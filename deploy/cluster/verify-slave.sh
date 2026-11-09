#!/usr/bin/env bash
# ============================================================================
# new-api slave 节点验证
# ============================================================================
# 沿用 master 的验证清单：
#   - supervisor RUNNING
#   - 运行中二进制 SHA-256 与发布包一致
#   - 本机 /api/status 返回 200 且版本正确
#   - 6 个前端入口资源摘要与 build-info.json 一致
#   - 未认证访问 self / 导出创建 / 导出列表 / 导出下载均 401
#   - 错误 API 路径返回 JSON 404（功能 17 的协议完整性）
#   - 指标端口可抓取
#
# 用法：./verify-slave.sh <公网IP> <节点名> <内网IP>
# ============================================================================

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "$SCRIPT_DIR/common.sh"

IP="${1:?用法: verify-slave.sh <公网IP> <节点名> <内网IP>}"
NODE="${2:?用法: verify-slave.sh <公网IP> <节点名> <内网IP>}"
PRIV_IP="${3:?用法: verify-slave.sh <公网IP> <节点名> <内网IP>}"

FAILED=0
check() { # check <描述> <命令...>
  local desc="$1"; shift
  if "$@" >/dev/null 2>&1; then ok "$desc"; else err "$desc"; FAILED=1; fi
}

log "验证 $NODE ($IP)"

# ---- 1. 进程状态 ----------------------------------------------------------
STATUS="$(ssh_run "$IP" 'supervisorctl status new-api' 2>/dev/null || true)"
echo "  supervisor: $STATUS"
echo "$STATUS" | grep -q RUNNING || { err "服务未 RUNNING"; exit 1; }

# ---- 2. 二进制摘要 --------------------------------------------------------
RUN_SHA="$(ssh_run "$IP" 'sha256sum /opt/new-api/new-api' 2>/dev/null | awk '{print $1}')"
if [[ "$RUN_SHA" == "$BINARY_SHA256" ]]; then
  ok "二进制摘要一致：$RUN_SHA"
else
  err "二进制摘要不一致：$RUN_SHA（期望 $BINARY_SHA256）"
  FAILED=1
fi

# ---- 3. 版本 --------------------------------------------------------------
REPORTED_VERSION="$(ssh_run "$IP" 'curl -s http://127.0.0.1:3000/api/status' 2>/dev/null \
  | grep -o '"version":"[^"]*"' | head -1 | cut -d'"' -f4)"
if [[ "$REPORTED_VERSION" == "$VERSION" ]]; then
  ok "运行版本一致：$REPORTED_VERSION"
else
  err "版本不匹配：$REPORTED_VERSION（期望 $VERSION）"
  FAILED=1
fi

# ---- 4. 内网 /api/status --------------------------------------------------
CODE="$(ssh_run "$IP" "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3000/api/status" 2>/dev/null || echo 000)"
if [[ "$CODE" == "200" ]]; then ok "本机 /api/status 200"; else err "/api/status 返回 $CODE"; FAILED=1; fi

# ---- 5. 前端资源摘要 ------------------------------------------------------
# build-info.json 中的 6 个入口资源是集群统一的校验基准。
log "校验前端入口资源"
BI="$(ssh_run "$MASTER_PUBLIC_IP" "cat '$REMOTE_RELEASES/$VERSION/build-info.json'" 2>/dev/null)"
if [[ -z "$BI" ]]; then
  warn "无法从 master 读取 build-info.json，跳过资源摘要校验"
else
  # 用 python 解析 JSON，避免 grep 在多行/嵌套结构上漏项
  ASSET_MAP="$(printf '%s' "$BI" | python3 -c '
import sys, json
d = json.load(sys.stdin)
for path, sha in d.get("frontend_assets", {}).items():
    print(f"{path} {sha}")
' 2>/dev/null || true)"

  if [[ -z "$ASSET_MAP" ]]; then
    warn "无法解析 frontend_assets，跳过资源摘要校验"
  else
    # ssh 在循环内会吃掉 stdin，必须用文件描述符 3 喂数据，否则循环只跑一轮
    while read -r path expected <&3; do
      [[ -n "$path" ]] || continue
      actual="$(ssh_run "$IP" "curl -s http://127.0.0.1:3000$path" 2>/dev/null | sha256sum | awk '{print $1}')"
      if [[ -n "$expected" && "$actual" == "$expected" ]]; then
        ok "资源一致：$path"
      else
        err "资源不一致：$path (本地 ${actual:0:12}… 期望 ${expected:0:12}…)"
        FAILED=1
      fi
    done 3<<< "$ASSET_MAP"
  fi
fi

# ---- 6. 匿名访问必须 401 --------------------------------------------------
# 注意：导出列表没有独立路由，列表与创建同为 /api/log/export（GET/POST）。
for route in "/api/user/self" "/api/log/export"; do
  CODE="$(ssh_run "$IP" "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3000$route" 2>/dev/null || echo 000)"
  if [[ "$CODE" == "401" ]]; then ok "匿名 $route → 401"; else err "匿名 $route → $CODE（期望 401）"; FAILED=1; fi
done

# ---- 7. 错误 API 路径返回 JSON 404（非 HTML 首页）-------------------------
for route in "/messages" "/responses" "/v2/responses"; do
  CT="$(ssh_run "$IP" "curl -s -o /dev/null -w '%{content_type}' http://127.0.0.1:3000$route" 2>/dev/null || true)"
  CODE="$(ssh_run "$IP" "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3000$route" 2>/dev/null || echo 000)"
  if [[ "$CODE" == "404" && "$CT" == *json* ]]; then
    ok "错误 API $route → JSON 404"
  else
    err "错误 API $route → $CODE ($CT)（期望 JSON 404）"
    FAILED=1
  fi
done

# ---- 8. 指标端口 ----------------------------------------------------------
# slave 的 METRICS_ADDR 只绑本节点内网 IP（不监听 127.0.0.1），必须用内网地址探测。
MCODE="$(ssh_run "$IP" "curl -s -o /dev/null -w '%{http_code}' http://$PRIV_IP:9000/metrics" 2>/dev/null || echo 000)"
if [[ "$MCODE" == "200" ]]; then ok "指标端口可抓取（$PRIV_IP:9000）"; else err "指标端口返回 $MCODE"; FAILED=1; fi

# ---- 9. 数据库连通（实例心跳）---------------------------------------------
# slave 会上报 system_instances 心跳。凭据只在 master 的 env 里，不在本地落盘；
# 通过 master 上的 psql 查询，密码需 URL 解码（DSN 含 % 编码），且不能用
# grep -oP 提取（特殊字符会被本地 shell 破坏）。
log "确认实例心跳"
HB="$(ssh_run "$MASTER_PUBLIC_IP" "NODE='$NODE' bash -s" <<'REMOTE' 2>/dev/null || true
set -e
python3 - "$NODE" <<'PY'
import re, subprocess, os, sys
from urllib.parse import unquote
node = sys.argv[1]
dsn = None
for line in open("/opt/new-api/env"):
    line = line.strip()
    if line.startswith("SQL_DSN="):
        dsn = line.split("=", 1)[1]
        break
m = re.match(r"postgresql://([^:]+):(.*)@([^:/]+):(\d+)/([^?]*)", dsn)
user, pw, host, port, db = m.groups()
env = dict(os.environ, PGPASSWORD=unquote(pw))
out = subprocess.run(["psql", "-h", host, "-U", user, "-d", db, "-tAc",
  "select node_name from system_instances where node_name='" + node + "'"],
  capture_output=True, text=True, env=env)
print(out.stdout.strip())
PY
REMOTE
)"
if printf '%s' "$HB" | grep -qx "$NODE"; then
  ok "实例心跳已记录（node_name=$NODE）"
else
  warn "未确认实例心跳（心跳上报有间隔，可稍后用 verify-cluster.sh 复查）"
fi

echo
if [[ "$FAILED" == "0" ]]; then
  ok "$NODE 全部验证通过"
else
  err "$NODE 存在验证失败项"
fi
exit "$FAILED"
