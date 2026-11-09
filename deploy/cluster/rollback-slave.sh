#!/usr/bin/env bash
# ============================================================================
# new-api slave 节点回滚
# ============================================================================
# 把 /opt/new-api/new-api 恢复为部署前的备份二进制，并重启服务。
# 备份由 deploy-slave.sh 在替换前创建：new-api.bak.<时间戳>
#
# 用法：./rollback-slave.sh <公网IP> [节点名]
# ============================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "$SCRIPT_DIR/common.sh"

IP="${1:?用法: rollback-slave.sh <公网IP> [节点名]}"
NODE="${2:-unknown}"

log "回滚 $NODE ($IP)"

# 回滚前先确认在途请求已排空（沿用集群发布的排空检查）
log "检查 HTTP 在途请求"
INFLIGHT="$(ssh_run "$IP" "curl -s http://127.0.0.1:9000/metrics 2>/dev/null \
  | grep '^new_api_generation_requests_in_flight' | grep -v '^#' | awk '{print \$2}'" || echo "")"
if [[ -n "${INFLIGHT:-}" && "$INFLIGHT" != "0" ]]; then
  warn "当前在途请求 $INFLIGHT，等待 30 秒"
  sleep 30
fi

# 无论回滚是否成功，都必须把服务拉回 RUNNING；否则一次失败会让节点离线。
set +e
ssh_run "$IP" 'cd /opt/new-api
supervisorctl stop new-api || true

BAK="$(ls -1t new-api.bak.* 2>/dev/null | head -1)"
if [[ -z "$BAK" ]]; then
  echo "没有可用备份，保持当前二进制并重启" >&2
else
  echo "恢复备份：$BAK"
  cp -a "$BAK" new-api.rollback-next
  chmod 0755 new-api.rollback-next
  chown root:root new-api.rollback-next
  mv -f new-api.rollback-next new-api
  sha256sum new-api
fi
supervisorctl start new-api'
RC=$?
set -e
[[ $RC -eq 0 ]] || warn "回滚命令返回非零（$RC），继续检查服务状态"

sleep 22
STATUS="$(ssh_run "$IP" 'supervisorctl status new-api' 2>/dev/null || true)"
echo "  $STATUS"
if ! echo "$STATUS" | grep -q RUNNING; then
  err "回滚后服务未 RUNNING，请人工检查：ssh root@$IP 'supervisorctl status new-api; tail -30 /var/log/new-api/supervisor.log'"
  exit 1
fi

ok "$NODE 已回滚"
