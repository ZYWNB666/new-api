# 03 · 部署与 CI（supervisor / 集群 / Grafana / GHCR）

- **状态**：活跃
- **引入提交**：`3996b5642`（supervisor）、`3817d7dae`（GHCR CI）、`03d2ff99d`（CI 修复）
- **影响面**：部署目录、CI workflow

## 功能说明

补齐三种部署/运维能力：supervisor 进程托管配置、GHCR 镜像构建 CI、Grafana 看板（与功能 06 配套）。

## 实现要点

- `deploy/supervisor/new-api.conf`：supervisor program 定义（`/opt/new-api/start.sh`，user=newapi）。
- `deploy/supervisor/new-api.env.example`：160 行环境变量示例，含 `LOG_REQUEST_HEADERS`、`ERROR_LOG_ENABLED`、`RELAY_TTFT_TIMEOUT` 等。
- `.github/workflows/ghcr-build.yml`：构建并推送 GHCR 镜像；`03d2ff99d` 修复 upload-artifact 的 `VERSION` 变量未展开。
- Grafana 看板 JSON 见 `deploy/grafana/`（`newapi-overview.json`、`newapi-tenants.json`，随功能 06/07 引入）。
- `docker-compose.yml` 增加指标相关服务编排。

## 配置与开关

环境变量示例见 `deploy/supervisor/new-api.env.example`。线上实际生效值（截至 2026-10-27）：

| 变量 | 线上值 | 说明 |
|---|---|---|
| `LOG_REQUEST_HEADERS` | `true` | 打印 provider 响应头 |
| `ERROR_LOG_ENABLED` | `true` | 错误日志 |
| `RELAY_TTFT_TIMEOUT` | 60（默认 30） | 流式首包超时秒数 |

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-10-21 | `3996b5642` | supervisor 配置与环境变量示例 |
| 2026-10-23 | `3817d7dae` | GHCR 镜像 workflow |
| 2026-10-23 | `03d2ff99d` | 修复 upload-artifact 路径 VERSION 未展开 |
| 2026-11-04 | `4138bffd8`（发布基线） | 部署 newapi-2 slave，共享数据库/Redis/密钥、只读 CSV 存储和监控；等待 ALB 加入 |
| 2026-11-04 | `370617d22`（部署记录基线） | 用户接入 ALB 后验证真实模型请求与成功返回，更新双机部署状态 |

## 注意事项

- 日志目录线上为 `/var/log/new-api/`，由 supervisor 轮转（`supervisor.log.N[.gz]`）。

### 双节点部署记录（2026-11-04，Asia/Shanghai）

- 现有 `newapi-1`：`139.224.194.237` / `172.16.0.75`，ECS `i-uf6dpvx9881exf2kb3jc`，`NODE_TYPE=master`。新增 `newapi-2`：`106.14.86.71` / `172.16.0.76`，ECS `i-uf6ei26gkgsvzs065r82`，Ubuntu 24.04.5 LTS、x86_64、8 核 16GB，`NODE_TYPE=slave`。使用 `NODE_NAME` 区分实例，不更改 ECS 主机名。
- 两台版本均为 `v1.0.0-rc.39-local.20261104170732.request-id-config.4138bffd8`；二进制 SHA-256 均为 `59c85bc2ab0db38d1b4633028dae6f5bbb80baed812e1ed59f73dc088deca008`。复用已校验发布包，未改代码、依赖、SQL/DSN、表结构或迁移逻辑，没有重启 master 应用。
- 配置通过 SSH 在两台之间复制，不在仓库或日志保存凭据。运行时比对确认主库、日志库设置、Redis、SESSION_SECRET、CRYPTO_SECRET、安全 Cookie 配置及可信代理一致；仅覆盖节点角色/名称及指标监听地址。主库 PostgreSQL 17.11，slave 实际连通并报告实例心跳。
- 新节点采用现有目录布局 `/opt/new-api`、`/var/log/new-api`，Supervisor 托管 `new-api`、用户 `newapi`（UID 996），`env` 权限 0600。`startsecs=15`、`stopwaitsecs=180`；systemd supervisor 的 `TimeoutStopSec=210`，保留应用默认 120 秒排空窗口。Supervisor、指标防火墙开机启用，Supervisor 已实际重启并恢复 RUNNING；未重启整台 ECS。
- master 的 `/var/lib/new-api/log-exports` 通过仅 NFSv4/TCP 的导出供 `172.16.0.76` 只读访问，使用 `all_squash,anonuid=996,anongid=988`。配置 `/etc/exports.d/newapi.exports`、`/etc/nfs.conf.d/newapi-exports.conf`；NFS 监听 `172.16.0.75:2049`，rpcbind/statd 停用，mountd 使用 `--no-tcp --no-udp`，不开放额外 RPC 监听。
- slave 停用不需要的 rpcbind/statd，挂载使用 NFSv4.2；其 `/etc/fstab` 使用 `_netdev,nofail,x-systemd.automount,x-systemd.mount-timeout=15s`，挂载同名导出目录；Supervisor 依赖 automount 单元，不依赖远端挂载成功才启动。以 `newapi` 用户实际读取 master 创建的 0600 合成 CSV 成功，写入返回只读错误；只删除该探针文件，不读取或导出现有用户日志。
- slave 指标监听 `172.16.0.76:9000`，由 `newapi-metrics-firewall.service` 加载 `/etc/newapi-metrics.nft`，仅允许 master 和本机访问该端口，不修改其他端口规则。master Prometheus 的 `new-api` job 增加目标，`promtool check config` 通过后 SIGHUP 重载，两个目标均 UP。配置备份 `/opt/monitoring/config/prometheus.yml.before-newapi-2-20261104203544`。
- 验证：两台运行时版本及二进制一致；slave 内网 `/api/status` 正常，6 个前端入口资源摘要匹配；未认证访问 self、导出创建/列表/下载均 401；公网 `netwebs.top` 健康及版本正常。共享实例表中只有 `newapi-1=true`、`newapi-2=false` 两个近期心跳。未使用真实用户凭据或发起模型生成；不据此声称已完成 5000 RPM 压测或跨节点登录实测。
- **流量接入已完成**：用户在现有 ALB 服务器组添加了 ECS `i-uf6ei26gkgsvzs065r82` / `172.16.0.76:3000`。20:43–20:44 验证新节点已经收到真实 `/v1/chat/completions` 请求并有 200 返回，公网健康检查也落到新节点；两台 Prometheus target 均 UP。云管理操作由用户完成，没有使用云 API 凭据；未为直连公网开放端口。
- 部署报告存于两台 `/opt/new-api/releases/v1.0.0-rc.39-local.20261104170732.request-id-config.4138bffd8/slave-deployment.json`。如撤下节点，先从 ALB 摘除（如已加入），再在 slave 执行 `supervisorctl stop new-api`，不把它改成 master；按上述备份回退 Prometheus、删除新增 NFS export 并 `exportfs -r` 即可解除共享。master 发布包及原有环境均保留。

- ALB 接入后的只读观测样本：新节点供应商响应中渠道 5 有 2 次 200，渠道 9 有 3 次 200、10 次 429；采样窗口为进程最近 1MiB 日志，统计随流量变化。429 已明确来自供应商响应，不能都归因于本机承载能力；本次不修改渠道限额、模型重试或计费配置。此次只观察既有实际请求，没有额外发起付费生成，也未执行 5000 RPM 压测。
