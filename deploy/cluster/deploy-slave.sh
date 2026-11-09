#!/usr/bin/env bash
# ============================================================================
# new-api slave 节点部署
# ============================================================================
# 从 master 拉取已校验的发布包与 env，逐台部署 slave。
#
# 每台流程：
#   1. prepare-node.sh   基础环境（supervisor / newapi 用户 / 目录）
#   2. 拉取发布包       从 master releases 目录经 SSH 管道传输到本地中转
#   3. 校验 SHA-256     与 master 运行中的二进制逐字节一致
#   4. 拉取 env         从 master 复制，仅改写 NODE_TYPE/NODE_NAME/METRICS_ADDR
#   5. 上传并安装       scp 上传二进制与配置，原子替换 + 权限修正
#   6. 启动            supervisorctl reread/update/start
#   7. verify-slave.sh  本地验证；失败则自动回滚到部署前二进制
#
# 用法：./deploy-slave.sh <all|主机名|公网IP> [--skip-verify]
# ============================================================================

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "$SCRIPT_DIR/common.sh"

TARGET="${1:?用法: deploy-slave.sh <all|主机名|公网IP>}"
SKIP_VERIFY=0
[[ "${2:-}" == "--skip-verify" ]] && SKIP_VERIFY=1

LOCAL_START="$SCRIPT_DIR/start.sh"
LOCAL_CONF="$SCRIPT_DIR/new-api.conf"
require_local_file "$LOCAL_START"
require_local_file "$LOCAL_CONF"

# ---- 解析目标 -------------------------------------------------------------
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

# ---- 本地中转目录 ---------------------------------------------------------
LOCAL_STAGE="$(mktemp -d /tmp/newapi-cluster.XXXXXX)"
cleanup() { rm -rf "$LOCAL_STAGE"; }
trap cleanup EXIT

# ---- 从 master 拉取发布包并校验 -------------------------------------------
log "从 master ($MASTER_PUBLIC_IP) 拉取发布包 $VERSION"

MASTER_SHA="$(ssh_run "$MASTER_PUBLIC_IP" \
  "sha256sum '$REMOTE_RELEASES/$VERSION/new-api-linux-amd64'" 2>/dev/null | awk '{print $1}')"
[[ -n "$MASTER_SHA" ]] || die "无法读取 master 发布包摘要"
[[ "$MASTER_SHA" == "$BINARY_SHA256" ]] \
  || die "master 二进制摘要 $MASTER_SHA 与预期 $BINARY_SHA256 不一致，中止"

ssh_run "$MASTER_PUBLIC_IP" \
  "tar czf - -C '$REMOTE_RELEASES/$VERSION' new-api-linux-amd64 build-info.json" \
  > "$LOCAL_STAGE/release.tar.gz"
tar xzf "$LOCAL_STAGE/release.tar.gz" -C "$LOCAL_STAGE"

LOCAL_BIN="$LOCAL_STAGE/new-api-linux-amd64"
require_local_file "$LOCAL_BIN"
LOCAL_SHA="$(sha256sum "$LOCAL_BIN" | awk '{print $1}')"
[[ "$LOCAL_SHA" == "$BINARY_SHA256" ]] \
  || die "中转二进制摘要 $LOCAL_SHA 与预期不一致，中止"
ok "发布包校验通过：$LOCAL_SHA"

# ---- 从 master 拉取 env 模板 ----------------------------------------------
# 凭据不经手：直接从 master 经 SSH 管道拉到本地中转，只做节点字段改写。
log "拉取 master env 模板"
ssh_run "$MASTER_PUBLIC_IP" "cat '$REMOTE_BASE/env'" > "$LOCAL_STAGE/env.template" 2>/dev/null
[[ -s "$LOCAL_STAGE/env.template" ]] || die "无法读取 master env 模板"
ok "env 模板已获取（$(wc -l < "$LOCAL_STAGE/env.template") 行）"

# ---- 逐台部署 -------------------------------------------------------------
FAILED=()
DEPLOYED=()

for i in "${TARGETS[@]}"; do
  NODE="$(slave_field "$i" 1)"
  IP="$(slave_field "$i" 2)"
  PRIV_IP="$(slave_field "$i" 3)"

  echo
  log "========== 部署 $NODE ($IP / $PRIV_IP) =========="

  if ! "$SCRIPT_DIR/prepare-node.sh" "$IP" "$NODE"; then
    err "$NODE 基础环境准备失败，跳过"
    FAILED+=("$NODE")
    continue
  fi

  # 生成该节点 env：只改节点身份与指标监听地址，其余与 master 完全一致
  ENV_OUT="$LOCAL_STAGE/env.$NODE"
  sed -e "s|^NODE_TYPE=.*|NODE_TYPE=slave|" \
      -e "s|^NODE_NAME=.*|NODE_NAME=$NODE|" \
      -e "s|^METRICS_ADDR=.*|METRICS_ADDR=$PRIV_IP:9000|" \
      "$LOCAL_STAGE/env.template" > "$ENV_OUT"

  # 关键项必须改写成功，且不能丢
  for expect in "^NODE_TYPE=slave$" "^NODE_NAME=$NODE$" "^METRICS_ADDR=$PRIV_IP:9000$" \
                "^SQL_DSN=" "^REDIS_CONN_STRING=" "^SESSION_SECRET=" "^CRYPTO_SECRET="; do
    grep -q "$expect" "$ENV_OUT" || { err "env 校验失败：缺少 $expect"; FAILED+=("$NODE"); continue 2; }
  done

  # 上传
  log "上传二进制与配置"
  scp_put "$LOCAL_BIN"   "root@$IP:$REMOTE_BASE/new-api.new"
  scp_put "$ENV_OUT"     "root@$IP:$REMOTE_BASE/env.new"
  scp_put "$LOCAL_START" "root@$IP:$REMOTE_BASE/start.sh.new"
  scp_put "$LOCAL_CONF"  "root@$IP:$SUPERVISOR_CONF"

  # 安装
  log "安装并修正权限"
  if ! ssh_run "$IP" "NODE='$NODE' VERSION='$VERSION' bash -s" <<'REMOTE'
set -euo pipefail
cd /opt/new-api

NEW_SHA="$(sha256sum new-api.new | awk '{print $1}')"
echo "uploaded sha=$NEW_SHA"

# 备份现有二进制（如有）
if [[ -f new-api ]]; then
  cp -a new-api "new-api.bak.$(date +%Y%m%d%H%M%S)"
fi

# 原子替换二进制
chmod 0755 new-api.new
chown root:root new-api.new
mv -f new-api.new new-api

# env：0600 属主 newapi（supervisor environment= 会解释 %，必须用 env 文件）
mv -f env.new env
chmod 0600 env
chown newapi:newapi env

# start.sh
mv -f start.sh.new start.sh
chmod 0755 start.sh
chown root:root start.sh

install -d -o newapi -g newapi -m 0755 "/opt/new-api/releases/$VERSION"
install -d -o newapi -g newapi -m 0700 /var/lib/new-api/log-exports
echo "installed node=$NODE version=$VERSION"
REMOTE
  then
    err "$NODE 安装失败"
    FAILED+=("$NODE")
    continue
  fi

  # 启动
  log "启动服务"
  ssh_run "$IP" 'supervisorctl reread && supervisorctl update' >/dev/null
  ssh_run "$IP" 'supervisorctl start new-api' >/dev/null 2>&1 || true

  # startsecs=15，等待进入 RUNNING
  log "等待服务稳定（startsecs=15）"
  sleep 22
  STATUS="$(ssh_run "$IP" 'supervisorctl status new-api' 2>/dev/null || true)"
  echo "  $STATUS"

  if ! echo "$STATUS" | grep -q RUNNING; then
    err "$NODE 未进入 RUNNING，抓取日志尾部"
    ssh_run "$IP" 'tail -20 /var/log/new-api/supervisor.log' 2>/dev/null || true
    FAILED+=("$NODE")
    continue
  fi

  # 验证
  if [[ "$SKIP_VERIFY" == "0" ]]; then
    if "$SCRIPT_DIR/verify-slave.sh" "$IP" "$NODE" "$PRIV_IP"; then
      DEPLOYED+=("$NODE")
    else
      err "$NODE 验证失败，回滚到部署前二进制"
      "$SCRIPT_DIR/rollback-slave.sh" "$IP" "$NODE" || true
      FAILED+=("$NODE")
      continue
    fi
  else
    DEPLOYED+=("$NODE")
  fi

  # 归档部署记录
  ssh_run "$IP" "NODE='$NODE' VERSION='$VERSION' SOURCE_COMMIT='$SOURCE_COMMIT' BINARY_SHA256='$BINARY_SHA256' bash -s" >/dev/null <<'REMOTE' || true
set -e
D="/opt/new-api/releases/$VERSION"
cat > "$D/deployment.json" <<JSON
{
  "node": "$NODE",
  "deployed_at": "$(date -Iseconds)",
  "version": "$VERSION",
  "source_commit": "$SOURCE_COMMIT",
  "binary_sha256": "$BINARY_SHA256",
  "supervisor": "$(supervisorctl status new-api)"
}
JSON
chmod 0644 "$D/deployment.json"
REMOTE

  ok "$NODE 部署完成"
done

# ---- 汇总 -----------------------------------------------------------------
echo
log "==================== 部署汇总 ===================="
if [[ ${#DEPLOYED[@]} -gt 0 ]]; then ok "成功：${DEPLOYED[*]}"; fi
if [[ ${#FAILED[@]} -gt 0 ]]; then err "失败：${FAILED[*]}"; exit 1; fi
