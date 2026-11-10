#!/bin/sh
# ============================================================================
# new-api slave 启动脚本
# ============================================================================
# 与 master 的 /opt/new-api/start.sh 结构一致，差异仅一点：
#   指标监听绑定到本节点内网 IP（master 的 Prometheus 需要跨机抓取）；
#   master 绑定 127.0.0.1 是因为它的 Prometheus 同机。
#
# LOG_REQUEST_HEADERS 必须与 master 保持一致：客户端请求头日志是集群统一的
# 排查手段，ALB 会把请求打到任意节点，只有部分节点打印会让"某个模型没有
# 请求头"这类问题无法定位。它不在 env 里（master 也是如此），由本脚本导出。
#
# .env 放在工作目录，由本脚本 source 后 exec；不使用 supervisor 的
# environment= （会解释 %，含 URL 编码的 Redis 密码会被破坏）。
# ============================================================================

set -e
set -a
. /opt/new-api/env
set +a

export LOG_REQUEST_HEADERS=true

# 指标只绑内网：METRICS_ADDR 由部署脚本按节点内网 IP 写入 env
export METRICS_ADDR="${METRICS_ADDR:-127.0.0.1:9000}"

exec /opt/new-api/new-api --port "${PORT:-3000}" --log-dir ""
