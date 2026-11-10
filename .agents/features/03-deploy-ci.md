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
| 2026-11-05 | `2896db5f9` | 双节点逐台发布 API 协议完整性修复，排空检查与自动回滚，配置保持不变 |
| 2026-11-10 | 工作区（基于 `4e53ab720`） | 四节点部署日志明细/统计导出，补齐只读共享存储并更新默认发布版本 |

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

- 2026-11-05 两台更新至 `v1.0.0-rc.39-local.20261105101241.protocol-contract.2896db5f9`；当前部署、验证和回滚详情见 [功能 17](17-api-protocol-integrity.md)。

### 四节点部署记录（2026-11-09，Asia/Shanghai）

- 新增三台 slave，与 master 同 VPC（`vpc-uf6bhmtb3d7ouef4e0v1t`）、同 region（cn-shanghai），均可直连 RDS PG `172.16.3.90:5432` 与 Redis `172.16.0.71:6379`。三台原先都是干净 Ubuntu 24.04.5（无 supervisor、无 new-api），8 核 16GB，磁盘 40G。

| 节点 | 公网 IP | 内网 IP | ECS | 主机名 |
|---|---|---|---|---|
| newapi-2 | 47.100.175.171 | 172.16.0.77 | i-uf608xbyhu0ngkzmk2e0 | newapi-2 |
| newapi-3 | 106.14.185.96 | 172.16.0.76 | i-uf608xbyhu0ngkzmk2e1 | newapi-3 |
| newapi-4 | 139.196.74.136 | 172.16.0.78 | i-uf608xbyhu0ngkzmk2e2 | newapi-4 |

- **IP 复用注意**：`172.16.0.76` 曾是 2026-11-04 那台 slave（newapi-2，`106.14.86.71`）的地址，该机器已下线（Prometheus target 长期 down），IP 现分配给 newapi-3。接入监控时把原 `node: newapi-2` 标签改为 `node: newapi-3`，避免指标归属错误。
- 四台版本统一为 `v1.0.0-rc.39-local.20261105101241.protocol-contract.2896db5f9`，二进制 SHA-256 均为 `70b7452b96fc69cea1875855a4e3df7df721874bf5b1b7556eee25f6972def42`。发布包从 master 的 `releases/` 目录经 SSH 管道拉取，部署前后各校验一次摘要；未改代码、依赖、DSN、表结构或迁移逻辑，**没有重启 master**（master 进程 uptime 连续，pid 867088 未变）。
- 部署脚本落在 `deploy/cluster/`（`common.sh`、`prepare-node.sh`、`deploy-slave.sh`、`verify-slave.sh`、`verify-cluster.sh`、`attach-monitoring.sh`、`rollback-slave.sh`、`start.sh`、`new-api.conf`），详见该目录 README。沿用既有约定：凭据不经手（env 从 master 管道拉取后只改 `NODE_TYPE`/`NODE_NAME`/`METRICS_ADDR` 三行）、不用 heredoc 传配置（改 scp）、不用 supervisor `environment=`（`%` 会破坏 Redis 密码的 URL 编码）。
- 目录布局、supervisor 参数与 master 完全一致：`/opt/new-api`、`/var/log/new-api`、user=newapi（UID 996，gid 取系统分配值 1000——slave 上 988 已被 docker 占用，NFS `all_squash` 依赖 master 端 anonuid/anongid，本地 gid 不影响挂载）、`env` 0600、`startsecs=15`、`stopwaitsecs=180`。
- 指标监听从 master 的 `127.0.0.1:9000` 改为各 slave 自己的内网 IP（master 的 Prometheus 同机，slave 需要跨机抓取），未对公网开放。master Prometheus 的 `new-api` job 现为 4 个目标，`promtool check config` 通过后对 systemd MainPID 发 SIGHUP 重载，四个 target 均 UP。配置备份 `/opt/monitoring/config/prometheus.yml.before-newapi-slaves-20261109140206`。
- 验证：四台 supervisor 均 RUNNING；二进制摘要、运行时版本一致；各 slave 本机 `/api/status` 200；6 个前端入口资源摘要逐一匹配；匿名访问 self 与导出创建均 401；`/messages`、`/responses`、`/v2/responses` 均返回 JSON 404（非 HTML 首页）；`system_instances` 表中 `newapi-1`~`newapi-4` 四个心跳均在更新；公网 `netwebs.top` 健康与版本正常；三台 slave 日志 0 error/panic/fatal、0 个 429。
- **流量接入状态**：三台的 3000 端口均已出现来自 ALB（`172.16.1.94`，与 master 相同的连接模式）的 established 连接，`newapi-2`/`newapi-3` 已收到真实 `POST /v1/responses` 并返回 200。即用户已在 ALB 服务器组加入这三台，本次部署是在流量已进入的情况下补齐服务。`GLOBAL_WEB_RATE_LIMIT=600/60s` 与 master 一致，健康检查未被 429 误杀。
- 本次未修改 ALB 权重、环境文件既有行、启动脚本结构、渠道限额或计费配置；未使用真实用户凭据登录，未主动发起付费模型调用，也未执行 5000 RPM 压测。

### 待办（四节点后）

- **NFS 共享导出目录已完成（2026-11-10）**：master 已为三台 slave 配置只读导出，slave 通过持久化 NFSv4.2/systemd automount 挂载；真实服务用户读取 0600 合成 CSV 成功、写入被拒绝。部署和回退记录见 [功能 15](15-usage-log-export.md)。
- **master 指标防火墙单元已丢失**：`newapi-metrics-firewall.service` 与 `/etc/newapi-metrics.nft` 在 master 上均已不存在，nftables 现无 9000 端口访问控制，四台的指标端口实际只依赖安全组。需要重建该单元，仅放行 master 与本机访问 9000。

### 四节点日志导出版本发布（2026-11-10，Asia/Shanghai）

- 四台统一发布 `v1.0.0-rc.39-local.20261110105838.log-export-month.9d85a8069`，运行中二进制 SHA-256 `fe62451a40c0acc6b6113e2ab5a8b97d6d55511d9d713bda52e1f4e4471b208e`。逐台备份、原子替换、重启及校验，失败自动回滚；应用环境文件、启动脚本和 ALB 配置保持原值。
- 补齐三台 slave 只读 NFS 导出存储；四节点版本、摘要、心跳、Prometheus、公网资源和匿名导出接口检查通过。临时导出任务及合成 CSV 探针已清理。完整包、源码来源、验证及回滚路径见 [功能 15](15-usage-log-export.md)。
- `deploy/cluster/common.sh` 默认版本/摘要更新到本次发布；SOURCE_COMMIT 仅表示构建基线，精确源码以发布包的工作区清单和差异为准。
