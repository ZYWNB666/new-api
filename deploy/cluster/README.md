# new-api 集群部署（master + slave）

多节点共享同一套 PostgreSQL / Redis / 会话密钥，由 `NODE_TYPE` 区分角色。
master 负责数据库迁移、后台任务调度、CSV 导出生成；slave 只承接 relay 流量。

## 拓扑

| 节点 | 公网 IP | 内网 IP | 角色 | ECS |
|---|---|---|---|---|
| newapi-1 | 139.224.194.237 | 172.16.0.75 | master | i-uf6dpvx9881exf2kb3jc |
| newapi-2 | 47.100.175.171 | 172.16.0.77 | slave | i-uf608xbyhu0ngkzmk2e0 |
| newapi-3 | 106.14.185.96 | 172.16.0.76 | slave | i-uf608xbyhu0ngkzmk2e1 |
| newapi-4 | 139.196.74.136 | 172.16.0.78 | slave | i-uf608xbyhu0ngkzmk2e2 |

> 注意：`172.16.0.76` 曾是旧 slave（newapi-2）的地址，该机器已下线，IP 现分配给
> newapi-3。Prometheus 中原 `node: newapi-2` 标签需在接入监控时修正。

## 文件

| 文件 | 用途 |
|---|---|
| `common.sh` | 集群拓扑、版本常量、SSH/scp 封装 |
| `prepare-node.sh` | slave 基础环境：supervisor、nfs-common、newapi 用户、目录 |
| `deploy-slave.sh` | 主部署流程：拉包 → 校验 → 装配置 → 启动 → 验证 → 失败回滚 |
| `restart-slave.sh` | 滚动重启（配置类变更，不换二进制）：上传 start.sh → 排空 → 重启 → 验证 |
| `verify-slave.sh` | 单节点验证（进程/摘要/版本/资源/401/JSON 404/指标/心跳） |
| `verify-cluster.sh` | 全集群验证（含 Prometheus targets 与公网健康） |
| `attach-monitoring.sh` | 把 slave 指标接入 master 的 Prometheus |
| `rollback-slave.sh` | 单节点回滚到部署前二进制 |
| `start.sh` | slave 启动脚本（source env 后 exec） |
| `new-api.conf` | supervisord 进程配置（与 master 一致） |

## 使用

```bash
# 1. 部署全部 slave
./deploy-slave.sh all

# 2. 部署单台
./deploy-slave.sh newapi-2
./deploy-slave.sh 47.100.175.171

# 3. 接入监控（在 master 上改 prometheus.yml 并重载）
./attach-monitoring.sh

# 4. 全集群验证
./verify-cluster.sh

# 配置类变更后滚动重启（不换二进制）
./restart-slave.sh all              # 先排空再逐台重启
./restart-slave.sh all --no-drain   # 跳过排空，直接重启（会中断在途流式请求）

# 回滚单台
./rollback-slave.sh 47.100.175.171 newapi-2
```

## start.sh 必须与 master 一致的行

`start.sh` 里有三处容易在新增节点时漏掉，漏掉不会报错，只会静默丢功能：

| 行 | 作用 | 漏掉的后果 |
|---|---|---|
| `export LOG_REQUEST_HEADERS=true` | 客户端请求头日志 | 该节点完全不打印请求头；ALB 分流后表现为"某些请求/模型没有请求头" |
| `export METRICS_ADDR=...` | 指标监听地址 | slave 需绑自己内网 IP，否则 master 抓不到该节点指标 |
| `. /opt/new-api/env` | 加载 DSN/Redis/密钥 | 服务直接起不来 |

这些行来自 master 的 `start.sh`，不在 `env` 文件里，复制 env 不会带上。

## 关键约定

- **凭据不经手**：`env` 从 master 经 SSH 管道拉到本地中转，只改写
  `NODE_TYPE` / `NODE_NAME` / `METRICS_ADDR` 三行，其余与 master 逐字节一致。
- **不用 heredoc 传配置**：多层引号会被本地 shell 破坏（历史事故），
  统一 `scp` 上传本地文件。
- **不用 supervisor `environment=`**：它会解释 `%`，Redis 密码含 URL 编码
  （`%24` `%26`）会被破坏；必须用 env 文件 + 包装脚本。
- **二进制必须一致**：从 master 的 `releases/` 目录取，部署前后各校验一次
  SHA-256，保证集群内逐字节相同。
- **指标只绑内网**：slave 的 `METRICS_ADDR` 绑本节点内网 IP，不监听公网。

## 角色差异（代码行为）

`common/init.go`：`IsMasterNode = os.Getenv("NODE_TYPE") != "slave"`

master 专属（slave 不执行）：

- `model.InitDB` / `InitLogDB` 的 `migrateDB()` / `migrateLOGDB()`
- `model.MigrateRetiredFrontendOptions()`
- `service.StartSystemTaskRunner()`（后台任务调度）
- `service` 的 auth 清理、codex 凭据刷新、订阅重置等定时任务
- `router.SetRouter` 中 `FRONTEND_BASE_URL` 被忽略（master 强制本地前端）

slave 与 master 相同：

- relay 全链路、渠道缓存同步、Redis 订阅、指标上报、实例心跳

## 待办

- **NFS 共享导出目录已完成（2026-11-10）**：三个 slave 均已通过 NFSv4.2 只读挂载 master 的 `/var/lib/new-api/log-exports`，通过 fstab/systemd automount 持久化。真实 `newapi` 用户读取 0600 合成 CSV 成功、写入被拒绝；不再需要新增挂载。
- **ALB 接入**：在阿里云控制台把三个 ECS 加入服务器组
  `sgp-p9hx08fv66eydekk7h`（HTTP, Wrr）才会分流真实流量。加入前确认
  `GLOBAL_WEB_RATE_LIMIT` 已放宽（master 为 600/60s），否则健康检查会被 429
  判不健康。
- **master 防火墙**：`newapi-metrics-firewall.service` 单元文件已丢失，
  nftables 现无 9000 端口访问控制，需重建。

## 当前发布（2026-11-10，Asia/Shanghai）

- 四节点版本统一为 `v1.0.0-rc.39-local.20261110105838.log-export-month.9d85a8069`，二进制 SHA-256 `fe62451a40c0acc6b6113e2ab5a8b97d6d55511d9d713bda52e1f4e4471b208e`。
- 发布包包含 31 天明细及 366 天统计导出；各节点保留原环境文件和启动脚本。完整验证及回退记录见 `.agents/features/15-usage-log-export.md`。
- 本次回滚使用每个节点 `/opt/new-api/releases/v1.0.0-rc.39-local.20261110105838.log-export-month.9d85a8069/rollback.py`，备份为同目录 `new-api-before-upgrade`；不要用仅搜索 `new-api.bak.*` 的旧回滚脚本替代本次发布回退。
