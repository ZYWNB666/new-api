#!/usr/bin/env bash
# ============================================================================
# new-api slave 节点基础环境准备
# ============================================================================
# 在目标 slave 上执行：安装 supervisor / nfs-common，创建 newapi 用户与目录。
# 幂等：重复执行安全。
#
# 用法：./prepare-node.sh <公网IP> <节点名>
# ============================================================================

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "$SCRIPT_DIR/common.sh"

IP="${1:?用法: prepare-node.sh <公网IP> <节点名>}"
NODE="${2:?用法: prepare-node.sh <公网IP> <节点名>}"

log "准备节点 $NODE ($IP)"

# ---- 1. 安装软件包 --------------------------------------------------------
log "安装 supervisor / nfs-common"
ssh_run "$IP" 'export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq supervisor nfs-common
systemctl enable --now supervisor
supervisorctl version
command -v mount.nfs >/dev/null && echo "mount.nfs OK"'

# ---- 2. 创建 newapi 用户 --------------------------------------------------
# master 上 uid=996(newapi) gid=988(newapi)。gid 988 在 slave 上已被 docker
# 占用，因此只保证 uid=996 与用户名一致，gid 取系统分配值；NFS 的 all_squash
# 依赖 master 端 anonuid/anonuid，slave 本地 gid 不影响挂载。
log "创建 newapi 用户与目录"
ssh_run "$IP" 'set -e
if ! getent passwd 996 >/dev/null; then
  useradd --uid 996 --shell /usr/sbin/nologin --no-create-home newapi
fi
id newapi
install -d -o newapi -g newapi -m 0750 /opt/new-api
install -d -o newapi -g newapi -m 0750 /var/log/new-api
install -d -o newapi -g newapi -m 0700 /var/lib/new-api/log-exports
install -d -o newapi -g newapi -m 0750 /opt/new-api/releases'

ok "节点 $NODE 基础环境就绪"
