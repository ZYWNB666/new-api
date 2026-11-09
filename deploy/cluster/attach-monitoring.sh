#!/usr/bin/env bash
# ============================================================================
# new-api slave 接入 Prometheus 监控
# ============================================================================
# 在 master 上执行：
#   1. 备份 prometheus.yml
#   2. 为每个 slave 内网 IP 增加 9000 抓取目标，打上 node 标签
#   3. promtool check config 校验
#   4. SIGHUP 重载
#   5. 轮询确认所有 target UP
#
# 同时修正历史遗留：旧 slave(newapi-2) 的机器已下线，但其内网地址
# 172.16.0.76 现已分配给 newapi-3，原 target 的 node: newapi-2 标签
# 会造成监控数据归属错误，本次一并改为正确的节点名。
#
# 用法：./attach-monitoring.sh
# ============================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "$SCRIPT_DIR/common.sh"

PROM_CONFIG=/opt/monitoring/config/prometheus.yml
PROM_BIN=/opt/monitoring/bin/prometheus
PROM_URL=http://127.0.0.1:9090

log "在 master ($MASTER_PUBLIC_IP) 上接入 Prometheus 监控"

# ---- 生成新的 static_configs 块 -------------------------------------------
# master 自身 + 三个 slave，共 4 个目标
BLOCK="$(mktemp)"
trap 'rm -f "$BLOCK"' EXIT

{
  echo "  - job_name: new-api"
  echo "    sample_limit: 100000"
  echo "    static_configs:"
  echo "      - targets: ['127.0.0.1:9000']"
  echo "        labels:"
  echo "          application: new-api"
  echo "          environment: production"
  echo "          node: newapi-1"
  for i in "${!SLAVES[@]}"; do
    NODE="$(slave_field "$i" 1)"
    PRIV_IP="$(slave_field "$i" 3)"
    echo "      - targets: ['${PRIV_IP}:9000']"
    echo "        labels:"
    echo "          application: new-api"
    echo "          environment: production"
    echo "          node: $NODE"
  done
} > "$BLOCK"

log "将要写入的目标："
sed 's/^/    /' "$BLOCK"

# ---- 上传并在 master 上替换 job 块 ----------------------------------------
scp_put "$BLOCK" "root@$MASTER_PUBLIC_IP:/tmp/newapi-prom-block.yml"

ssh_run "$MASTER_PUBLIC_IP" "BLOCK=/tmp/newapi-prom-block.yml CONFIG=$PROM_CONFIG bash -s" <<'REMOTE'
set -euo pipefail

# 备份
cp -a "$CONFIG" "${CONFIG}.before-newapi-slaves-$(date +%Y%m%d%H%M%S)"
echo "已备份配置"

# 用 python 精确替换 new-api job 块（保留其余 job 不动）
python3 - "$CONFIG" "$BLOCK" <<'PY'
import sys, re
config_path, block_path = sys.argv[1], sys.argv[2]
with open(config_path) as f:
    lines = f.readlines()
with open(block_path) as f:
    block = f.read()

# 定位 "  - job_name: new-api" 到下一个同级 "  - job_name:" 之间
start = None
for idx, line in enumerate(lines):
    if re.match(r'^\s{2}- job_name:\s*new-api\s*$', line):
        start = idx
        break
if start is None:
    sys.exit("未找到 new-api job 块")

end = len(lines)
for idx in range(start + 1, len(lines)):
    if re.match(r'^\s{2}- job_name:', lines[idx]):
        end = idx
        break

new_lines = lines[:start] + [block if block.endswith("\n") else block + "\n"] + lines[end:]
with open(config_path, "w") as f:
    f.writelines(new_lines)
print(f"已替换 job 块：行 {start + 1} - {end}")
PY

# 校验
/opt/monitoring/bin/promtool check config "$CONFIG"
REMOTE

# ---- 重载并验证 -----------------------------------------------------------
# newapi-prometheus.service 没有 ExecReload，systemctl reload 会失败。
# 必须对 systemd 的 MainPID 发 SIGHUP（不能用 pgrep -f，可能命中多个进程，
# 历史上曾因此产生过一个孤儿 prometheus）。
log "重载 Prometheus"
MAINPID="$(ssh_run "$MASTER_PUBLIC_IP" 'systemctl show newapi-prometheus -p MainPID --value')"
[[ -n "$MAINPID" && "$MAINPID" != "0" ]] || die "无法获取 newapi-prometheus MainPID"
log "MainPID=$MAINPID，发送 SIGHUP"
ssh_run "$MASTER_PUBLIC_IP" "kill -HUP $MAINPID"

# 确认没有多出 prometheus 进程（SIGHUP 是原地重载，不应 fork 新进程）
# 注意：pgrep -f 会把执行检测命令自身的 bash -c 也匹配进来，必须排除自身 PID。
sleep 3
PROC_COUNT="$(ssh_run "$MASTER_PUBLIC_IP" 'pgrep -f "^/opt/monitoring/bin/prometheus" | wc -l')"
if [[ "$PROC_COUNT" != "1" ]]; then
  warn "检测到 $PROC_COUNT 个 prometheus 进程，请人工检查"
else
  ok "prometheus 进程数正常（1）"
fi

log "等待 target 上线（最多 60s）"
DEADLINE=$((SECONDS + 60))
ALL_UP=0
while [[ $SECONDS -lt $DEADLINE ]]; do
  RESULT="$(ssh_run "$MASTER_PUBLIC_IP" "curl -s $PROM_URL/api/v1/targets" 2>/dev/null || echo "")"
  [[ -z "$RESULT" ]] && { sleep 5; continue; }

  SUMMARY="$(printf '%s' "$RESULT" | python3 -c '
import sys, json
try:
    d = json.load(sys.stdin)
except Exception:
    sys.exit(1)
targets = [t for t in d["data"]["activeTargets"] if t["labels"].get("job") == "new-api"]
for t in targets:
    print(t["labels"].get("instance"), t["labels"].get("node", "-"), t["health"])
up = sum(1 for t in targets if t["health"] == "up")
sys.exit(0 if targets and up == len(targets) else 1)
' || true)"

  if [[ -n "$SUMMARY" ]]; then
    echo "$SUMMARY" | sed 's/^/    /'
    if printf '%s' "$SUMMARY" | grep -qv ' up$'; then
      sleep 5
      continue
    fi
    ALL_UP=1
    break
  fi
  sleep 5
done

echo
if [[ "$ALL_UP" == "1" ]]; then
  ok "所有 new-api target 均已 UP"
else
  err "仍有 target 未 UP，请检查 master 上 /opt/monitoring/config/prometheus.yml 与各节点指标端口"
  exit 1
fi
