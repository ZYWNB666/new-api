#!/usr/bin/env bash
# ============================================================================
# new-api 集群部署公共函数（master + slave）
# ============================================================================
# 约定：
#   - 通过 SSH 执行远端脚本；不使用内嵌 heredoc 传配置（多层引号会被本地
#     shell 破坏，历史事故见部署记录），统一用 scp 上传本地文件。
#   - 不在仓库或日志中保存任何凭据；env 文件只在节点间通过 SSH 传输。
# ============================================================================

set -euo pipefail

# ---- 集群拓扑 -------------------------------------------------------------
# master：139.224.194.237 / 172.16.0.75（已有，不重复部署）
MASTER_PUBLIC_IP="${MASTER_PUBLIC_IP:-139.224.194.237}"
MASTER_PRIVATE_IP="${MASTER_PRIVATE_IP:-172.16.0.75}"

# 待部署 slave：主机名|公网IP|内网IP
# 106.14.185.96 的内网地址 172.16.0.76 曾是旧 slave(newapi-2) 的地址，
# 该机器已下线，Prometheus 中仍残留 node: newapi-2 标签，接入监控时需修正。
SLAVES=(
  "newapi-2|47.100.175.171|172.16.0.77"
  "newapi-3|106.14.185.96|172.16.0.76"
  "newapi-4|139.196.74.136|172.16.0.78"
)

# ---- 部署布局（与 master 完全一致）-----------------------------------------
REMOTE_BASE=/opt/new-api
REMOTE_RELEASES="$REMOTE_BASE/releases"
REMOTE_LOG_DIR=/var/log/new-api
REMOTE_EXPORT_DIR=/var/lib/new-api/log-exports
SUPERVISOR_CONF=/etc/supervisor/conf.d/new-api.conf

# ---- 版本 -----------------------------------------------------------------
# 与 master 当前运行版本保持一致；发布包从 master 的 releases 目录取，
# 保证二进制逐字节一致（SHA-256 70b7452b...def42）。
VERSION="${VERSION:-v1.0.0-rc.39-local.20261105101241.protocol-contract.2896db5f9}"
SOURCE_COMMIT="${SOURCE_COMMIT:-2896db5f950cca76ca80da06f1b32e08544f5f22}"
BINARY_SHA256="${BINARY_SHA256:-70b7452b96fc69cea1875855a4e3df7df721874bf5b1b7556eee25f6972def42}"

# ---- SSH 选项 -------------------------------------------------------------
SSH_OPTS=(-o StrictHostKeyChecking=no -o ConnectTimeout=15 -o BatchMode=yes)

log()  { printf '\033[1;34m[%s]\033[0m %s\n' "$(date '+%F %T')" "$*"; }
ok()   { printf '\033[1;32m[ OK ]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[WARN]\033[0m %s\n' "$*"; }
err()  { printf '\033[1;31m[FAIL]\033[0m %s\n' "$*" >&2; }
die()  { err "$*"; exit 1; }

# ssh_run <ip> <命令...>
ssh_run() {
  local ip="$1"; shift
  ssh "${SSH_OPTS[@]}" "root@$ip" "$@"
}

# scp_put <本地文件> <ip>:<远端路径>
scp_put() {
  local src="$1" dst="$2"
  scp "${SSH_OPTS[@]}" -q "$src" "$dst"
}

# slave_field <索引> <字段序号：1=主机名 2=公网IP 3=内网IP>
slave_field() {
  local idx="$1" field="$2"
  echo "${SLAVES[$idx]}" | awk -F'|' -v f="$field" '{print $f}'
}

# 检查本地前置文件是否存在
require_local_file() {
  [[ -f "$1" ]] || die "缺少本地文件：$1"
}
