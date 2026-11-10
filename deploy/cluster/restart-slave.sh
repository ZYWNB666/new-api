#!/usr/bin/env bash
# ============================================================================
# new-api slave 滚动重启（仅更新 start.sh 等配置，不换二进制）
# ============================================================================
# 用于配置类变更（如补 LOG_REQUEST_HEADERS）：二进制不变，只重启进程。
#
# 每台流程：
#   1. 上传新的 start.sh
#   2. 排空检查：HTTP 在途为 0 且业务请求计数连续 30 秒不变
#   3. supervisorctl restart
#   4. 验证 RUNNING + 指标可抓取
#
# 用法：./restart-slave.sh <all|主机名|公网IP> [--no-drain]
#   --no-drain  跳过排空等待，直接重启（会中断当时在途的流式请求）
# ============================================================================

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "$SCRIPT_DIR/common.sh"

TARGET="${1:?用法: restart-slave.sh <all|主机名|公网IP> [--no-drain]}"
DRAIN=1
[[ "${2:-}" == "--no-drain" ]] && DRAIN=0

LOCAL_START="$SCRIPT_DIR/start.sh"
require_local_file "$LOCAL_START"

TARGETS=()
if [[ "$TARGET" == "all" ]]; then
  for i in "${!SLAVES[@]}"; do TARGETS+=("$i"); done
else
  for i in "${!SLAVES[@]}"; do
    if [[ "$TARGET" == "$(slave_field "$i" 1)" || "$TARGET" == "$(slave_field "$i" 2)" ]]; then
      TARGETS+=("$i")
    fi
  done
  [[ ${#TARGETS[@]} -gt 0 ]] || die "未找到目标节点：$TARGET"
fi

FAILED=()
DONE=()

for i in "${TARGETS[@]}"; do
  NODE="$(slave_field "$i" 1)"
  IP="$(slave_field "$i" 2)"
  PRIV_IP="$(slave_field "$i" 3)"

  echo
  log "========== 滚动重启 $NODE ($IP) =========="

  # 上传新 start.sh
  log "上传 start.sh"
  scp_put "$LOCAL_START" "root@$IP:$REMOTE_BASE/start.sh.new"
  ssh_run "$IP" 'set -e
cd /opt/new-api
chmod 0755 start.sh.new
chown root:root start.sh.new
mv -f start.sh.new start.sh
echo "start.sh sha256: $(sha256sum start.sh | cut -d" " -f1)"'

  # ---- 排空检查 -----------------------------------------------------------
  # 沿用集群发布的排空标准：在途请求为 0，且业务请求计数连续 30 秒不变。
  # --no-drain 时跳过：用户已明确接受中断在途流式请求的风险。
  if [[ "$DRAIN" == "0" ]]; then
    warn "跳过排空检查（--no-drain），直接重启"
  elif ! ssh_run "$IP" 'bash -s' <<'REMOTE'
set -e
INFLIGHT_METRIC='new_api_generation_requests_in_flight'
STARTED_METRIC='new_api_generation_started_total'

get_metric() {
  curl -s "http://127.0.0.1:${METRICS_ADDR##*:}/metrics" 2>/dev/null \
    | grep -E "^$1" | grep -v '^#' | awk '{s+=$2} END {print s+0}'
}

# 等待在途归零（最多 5 分钟）
for _ in $(seq 1 60); do
  V="$(get_metric "$INFLIGHT_METRIC")"
  if [[ "$V" == "0" ]]; then break; fi
  echo "  在途请求 $V，等待..."
  sleep 5
done

V="$(get_metric "$INFLIGHT_METRIC")"
if [[ "$V" != "0" ]]; then
  echo "在途请求未归零（$V），放弃重启" >&2
  exit 1
fi
echo "  在途请求已归零"

# 业务计数连续 30 秒不变
A="$(get_metric "$STARTED_METRIC")"
sleep 30
B="$(get_metric "$STARTED_METRIC")"
if [[ "$A" != "$B" ]]; then
  echo "业务请求计数仍在变化（$A -> $B），放弃重启" >&2
  exit 1
fi
echo "  业务计数稳定（$A）"
REMOTE
  then
    err "$NODE 排空检查未通过，跳过"
    FAILED+=("$NODE")
    continue
  fi

  # ---- 重启 ---------------------------------------------------------------
  log "重启服务"
  ssh_run "$IP" 'supervisorctl restart new-api'

  log "等待服务稳定（startsecs=15）"
  sleep 22
  STATUS="$(ssh_run "$IP" 'supervisorctl status new-api' 2>/dev/null || true)"
  echo "  $STATUS"
  if ! echo "$STATUS" | grep -q RUNNING; then
    err "$NODE 重启后未 RUNNING"
    ssh_run "$IP" 'tail -20 /var/log/new-api/supervisor.log' 2>/dev/null || true
    FAILED+=("$NODE")
    continue
  fi

  # ---- 验证 ---------------------------------------------------------------
  MCODE="$(ssh_run "$IP" "curl -s -o /dev/null -w '%{http_code}' http://$PRIV_IP:9000/metrics" 2>/dev/null || echo 000)"
  if [[ "$MCODE" == "200" ]]; then
    ok "$NODE 指标端口可抓取"
  else
    err "$NODE 指标端口返回 $MCODE"
    FAILED+=("$NODE")
    continue
  fi

  # 确认新的 start.sh 已生效：发一个本机 relay 请求，检查是否出现请求头日志。
  # 只有 relay 请求会触发 RouteTag("relay") 的请求头打印，管理接口不会。
  # grep -c 在匹配多个文件时会输出 "file:count" 多行，必须限定单文件并取数字。
  ssh_run "$IP" 'curl -s -o /dev/null -X POST http://127.0.0.1:3000/v1/chat/completions \
    -H "Content-Type: application/json" -d "{}"' >/dev/null 2>&1
  sleep 2
  HDR="$(ssh_run "$IP" 'grep -c "client request headers" /var/log/new-api/supervisor.log 2>/dev/null | head -1' 2>/dev/null || echo 0)"
  HDR="${HDR//[!0-9]/}"
  if [[ "${HDR:-0}" -ge 1 ]]; then
    ok "$NODE 请求头日志已生效（$HDR 行）"
    DONE+=("$NODE")
  else
    err "$NODE 请求头日志仍未出现"
    FAILED+=("$NODE")
  fi
done

echo
log "==================== 重启汇总 ===================="
if [[ ${#DONE[@]} -gt 0 ]]; then ok "成功：${DONE[*]}"; fi
if [[ ${#FAILED[@]} -gt 0 ]]; then err "失败：${FAILED[*]}"; exit 1; fi
