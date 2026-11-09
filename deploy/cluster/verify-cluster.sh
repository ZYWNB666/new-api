#!/usr/bin/env bash
# ============================================================================
# new-api 全集群验证
# ============================================================================
# 部署完成后运行，确认：
#   - 各节点 supervisor RUNNING、二进制摘要一致、版本一致
#   - 数据库 system_instances 表中心跳节点数与实际一致
#   - Prometheus 所有 target UP
#   - 公网健康检查未受影响（master 仍通过 ALB 对外服务）
#
# 用法：./verify-cluster.sh
# ============================================================================

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "$SCRIPT_DIR/common.sh"

FAILED=0

log "========== 全集群验证 =========="

# ---- 1. master ------------------------------------------------------------
log "master ($MASTER_PUBLIC_IP)"
M_STATUS="$(ssh_run "$MASTER_PUBLIC_IP" 'supervisorctl status new-api' 2>/dev/null || true)"
echo "  $M_STATUS"
echo "$M_STATUS" | grep -q RUNNING || { err "master 未 RUNNING"; FAILED=1; }

M_SHA="$(ssh_run "$MASTER_PUBLIC_IP" 'sha256sum /opt/new-api/new-api' 2>/dev/null | awk '{print $1}')"
if [[ "$M_SHA" == "$BINARY_SHA256" ]]; then
  ok "master 二进制摘要一致"
else
  err "master 二进制摘要不一致：$M_SHA"
  FAILED=1
fi

# ---- 2. 各 slave ----------------------------------------------------------
for i in "${!SLAVES[@]}"; do
  NODE="$(slave_field "$i" 1)"
  IP="$(slave_field "$i" 2)"
  PRIV_IP="$(slave_field "$i" 3)"

  echo
  log "$NODE ($IP / $PRIV_IP)"

  S_STATUS="$(ssh_run "$IP" 'supervisorctl status new-api' 2>/dev/null || true)"
  echo "  $S_STATUS"
  echo "$S_STATUS" | grep -q RUNNING || { err "$NODE 未 RUNNING"; FAILED=1; continue; }

  S_SHA="$(ssh_run "$IP" 'sha256sum /opt/new-api/new-api' 2>/dev/null | awk '{print $1}')"
  if [[ "$S_SHA" == "$BINARY_SHA256" ]]; then
    ok "$NODE 二进制摘要一致"
  else
    err "$NODE 二进制摘要不一致：$S_SHA"
    FAILED=1
  fi

  S_VER="$(ssh_run "$IP" 'curl -s http://127.0.0.1:3000/api/status' 2>/dev/null \
    | grep -o '"version":"[^"]*"' | head -1 | cut -d'"' -f4)"
  if [[ "$S_VER" == "$VERSION" ]]; then
    ok "$NODE 版本一致：$S_VER"
  else
    err "$NODE 版本不匹配：$S_VER"
    FAILED=1
  fi

  # 指标端口（从 master 侧探测，验证防火墙与监听）
  M_OK="$(ssh_run "$MASTER_PUBLIC_IP" "curl -s -o /dev/null -w '%{http_code}' --max-time 5 http://$PRIV_IP:9000/metrics" 2>/dev/null || echo 000)"
  if [[ "$M_OK" == "200" ]]; then
    ok "$NODE 指标端口可从 master 抓取"
  else
    err "$NODE 指标端口不可抓取（$M_OK）"
    FAILED=1
  fi
done

# ---- 3. 数据库实例心跳 ----------------------------------------------------
echo
log "数据库实例心跳"
# 通过 master 上的 psql 查询（凭据只在 master 的 env 中，密码需 URL 解码）
NAMES="$(ssh_run "$MASTER_PUBLIC_IP" 'bash -s' <<'REMOTE' 2>/dev/null || true
set -e
python3 <<'PY'
import re, subprocess, os
from urllib.parse import unquote
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
  "select node_name from system_instances order by node_name"],
  capture_output=True, text=True, env=env)
print(out.stdout.strip())
PY
REMOTE
)"
if [[ -n "$NAMES" ]]; then
  echo "  心跳节点：$(echo "$NAMES" | tr '\n' ' ')"
  for expect in newapi-1 newapi-2 newapi-3 newapi-4; do
    if printf '%s' "$NAMES" | grep -qx "$expect"; then
      ok "心跳包含 $expect"
    else
      warn "心跳未见 $expect"
      FAILED=1
    fi
  done
else
  warn "无法查询实例心跳表"
  FAILED=1
fi

# ---- 4. Prometheus targets ------------------------------------------------
echo
log "Prometheus targets"
T_RESULT="$(ssh_run "$MASTER_PUBLIC_IP" 'curl -s http://127.0.0.1:9090/api/v1/targets' 2>/dev/null || echo "")"
if [[ -n "$T_RESULT" ]]; then
  # 用 python 的 -c 单行脚本时避免在 f-string 里用反斜杠转义引号，
  # 改用 % 格式化，防止 SSH 多层引号把代码破坏。
  printf '%s' "$T_RESULT" | python3 -c '
import sys, json
d = json.load(sys.stdin)
targets = [t for t in d["data"]["activeTargets"] if t["labels"].get("job") == "new-api"]
for t in targets:
    inst = t["labels"].get("instance", "-")
    node = t["labels"].get("node", "-")
    print("    %-22s node=%-10s %s" % (inst, node, t["health"]))
up = sum(1 for t in targets if t["health"] == "up")
sys.exit(0 if targets and up == len(targets) else 1)
' || { err "存在未 UP 的 target"; FAILED=1; }
else
  err "无法读取 Prometheus targets"
  FAILED=1
fi

# ---- 5. 公网健康 ----------------------------------------------------------
echo
log "公网健康检查"
PUB_CODE="$(curl -s -o /dev/null -w '%{http_code}' --max-time 10 https://netwebs.top/api/status 2>/dev/null || echo 000)"
if [[ "$PUB_CODE" == "200" ]]; then
  ok "公网 /api/status 200"
else
  err "公网 /api/status 返回 $PUB_CODE"
  FAILED=1
fi

PUB_VER="$(curl -s --max-time 10 https://netwebs.top/api/status 2>/dev/null | grep -o '"version":"[^"]*"' | head -1 | cut -d'"' -f4)"
if [[ "$PUB_VER" == "$VERSION" ]]; then
  ok "公网版本一致：$PUB_VER"
else
  err "公网版本不匹配：$PUB_VER"
  FAILED=1
fi

# ---- 汇总 -----------------------------------------------------------------
echo
log "========== 验证汇总 =========="
if [[ "$FAILED" == "0" ]]; then
  ok "全集群验证通过"
else
  err "存在验证失败项"
fi
exit "$FAILED"
