# 15 · 使用日志后台 CSV 导出

- **状态**：活跃
- **引入提交**：`0d9b5847a`（使用日志后台 CSV 导出）
- **影响面**：`model/log_export.go`、`service/log_export.go`、`controller/log_export.go`、`router/api-router.go`、`web/src/features/usage-logs/`

## 功能说明

使用日志的通用日志页支持按已生效的时间、类型、模型、令牌、分组、用户名、渠道和请求 ID 条件创建后台 CSV 导出，可选择详细数据或统计数据。统计可按小时、天、月或整个区间汇总，叠加模型、用户 ID、渠道 ID、分组、令牌 ID/名称维度，按日志类型及扣费来源分别汇总。用户可单独勾选导出字段，管理员可额外选择用户 ID、用户名和渠道 ID；个人视图只导出当前用户的日志。任务存入既有 `system_tasks` 表，关闭页面后仍会继续执行，重新打开导出窗口可查看最近任务并下载完成的文件。

## 实现要点

- `model.OpenLogExportRows` 使用单条查询和数据库游标流式读取，无全量切片、重复 COUNT 或 OFFSET 分页；`model.LogExportColumns` 同时验证字段白名单和构造查询投影，仅当选择需要历史计费快照的字段时读取 `other`，对投影列去重。
- `service.LogExportHandler` 复用 `EnqueueSystemTask` 的唯一活动键和任务租约，全集群只生成一个导出。每 500 条更新进度，生成 `.part` 后原子重命名，失败时删除半成品；`logExportWriter` 在写入边界限制字节数。
- `service.logExportCleanupHandler` 每小时清理过期 CSV 和崩溃遗留的临时文件；进程崩溃后的任务按既有租约过期机制标记失败，用户重新创建任务，不自动续传生成。
- `CreateLogExport`、`ListLogExports`、`DownloadLogExport` 使用既有 `UserAuth`；服务端校验管理员范围、字段权限和下载所有者。管理员降权后无法下载原先创建的全用户导出。不导出完整 `other`、请求头或权限诊断；客户端 ID、计价用量、单价、表达式等只输出允许的公共字段，不输出完整元数据。
- CSV 使用 UTF-8 BOM、标准引号转义和公式注入防护，列名为稳定字段标识。时间为 UTC，耗时为秒，额度为原始整数额度单位，不改变计费值。新增金额列固定为 USD，使用日志中的 `billing_quota_per_unit` 换算；订阅日志优先使用 `subscription_consumed`，不使用当前价格或汇率补算历史。
- `LogExportDialog` 复用共享 `Dialog`、`Checkbox`、`Button` 和 `LogsFilterToolbar.actionStart`。已有兑换码导出仅处理兑换码及 TXT/Markdown，不能承载日志筛选和后台任务，因此新增日志业务组件，不新增通用弹窗实现。
- `downloadLogExport` 在支持 File System Access 的浏览器里流式写入用户选择的文件；其他浏览器仅对不超过 128MiB 的文件使用 Blob 下载，较大的文件提示更换支持直接保存的浏览器。

- `logExportBillingValues` 提取缓存读写（含 5m/1h）、图片/音频、计价输入输出、各项金额、单价和订阅消耗。`logExportExpressionPrices` 仅拆解历史表达式中的线性 token 项，用 `matched_tier` 与 `request_rules` 选择历史分支；不执行时钟或请求探测。按次、非线性或缺少必要历史依据的分项金额留空，原表达式仍可导出。每个导出缓存最多 256 组表达式价格结果。
- `addLogExportSummary` 在逐行读取时用 decimal 累计，不引入方言相关 JSON SQL 或全量日志切片；最多保留 10000 个汇总组合，超过时明确失败，不截断。`log_count` 为日志条数，不把多条失败/重试日志冒充独立请求数；未知数值使对应合计留空，`amount_missing_logs` 记录总金额未知的条数。
- 汇总时间使用所选 IANA 时区的日历边界；小时桶保留夏令时重复小时的不同 UTC 偏移。默认 UTC，前端可选浏览器时区及 Asia/Shanghai。各数值列有各自语义，原始用量、缓存子类别和计价用量不能直接相加作为总 token。

## 配置与开关

- 使用入口：使用日志 → 通用日志 → 导出日志。
- `LOG_EXPORT_DIR`：默认 `./data/log-exports`。所有执行节点、下载节点必须挂载同一持久化目录；目录不应通过静态站点公开。
- `LOG_EXPORT_MAX_ROWS`：默认 10000000 行。
- `LOG_EXPORT_MAX_MB`：默认 1024MiB，代码限制配置在 1–10240MiB。
- 明细时间范围最多 31 天，支持完整自然月对账；统计最多 366 天；都受 `LOG_EXPORT_MAX_ROWS` 的原始扫描行数上限限制。未来结束时间在创建时截到当前时间；单任务最多执行 30 分钟。
- 下载有效期为任务创建后 24 小时，清理每小时执行。执行任务需要至少一个 master 节点。
- 最近任务窗口在集群最新 100 条导出记录中筛选当前所有者。并发生成受全集群活动任务约束，不需要新增表、迁移或依赖。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-11-04 | `0d9b5847a` | 新增按筛选条件及选定字段生成后台 CSV、进度查询、权限校验和下载 |
| 2026-11-04 | `b8dd75220`（发布基线） | 发布到 netwebs.top，配置持久化导出目录并完成线上验证 |
| 2026-11-04 | `00d256998` | 新增可选客户端 ID 列，旧日志为空，保持元数据及管理员诊断不可导出 |
| 2026-11-04 | `4138bffd8`（发布基线） | 接入 slave 的只读共享 CSV 挂载，验证服务用户可读及写入被拒绝 |
| 2026-11-09 | 工作区（基于 `4e53ab720`，未提交） | 补齐 token/金额明细，新增可配置时间粒度、时区及维度的统计导出 |
| 2026-11-10 | 工作区（基于 `4e53ab720`，未提交） | 明细导出范围从 7 天放宽到 31 天，支持整月对账，同步七种语言提示及边界回归 |
| 2026-11-10 | 工作区发布（基于 `4e53ab720`） | 打包并完成四节点上线、NFS 挂载、公网验证及后台导出验证 |

## 注意事项

- 导出遵循 `.agents/rules/billing.md`：原始额度不变，USD 金额仅用历史快照换算，不修改预扣、结算、退款和实际计费值；缺失历史换算依据时留空，不借用当前 `QuotaPerUnit`。
- 多节点必须共享导出目录；仅共享数据库不能保证任意节点都能下载生成文件。浏览器直写磁盘能力通常需要 HTTPS 和受支持的浏览器。
- CSV 百万行可能超过部分电子表格的单工作表行数限制，文件本身不因此截断。
- 导出期间会占用一个日志库读连接；任务进度更新和租约续期还需要主库连接。主库与日志库共用连接池时，应至少保留额外连接。
- 以下验证使用隔离的真实数据库。未修改表结构或迁移；不把本地导出速度推断为线上网关 5000 RPM 容量。

### 验证记录

数据库定向回归命令在仓库根目录执行。DSN 使用独立临时库；下列变量代替本机测试凭据：

```bash
go test ./controller -run '^TestLogExport' -count=1
env LOG_EXPORT_TEST_MYSQL="$EXPORT_MYSQL_DSN" go test ./controller -run '^TestLogExport' -count=1
env LOG_EXPORT_TEST_POSTGRES="$EXPORT_POSTGRES_DSN" go test ./controller -run '^TestLogExport' -count=1
env LOG_EXPORT_TEST_CLICKHOUSE="$EXPORT_CLICKHOUSE_DSN" go test ./controller -run '^TestLogExport' -count=1
```

- SQLite 3.50.4、MySQL 8.0.46、PostgreSQL 17.11：定向导出回归通过。
- 独立 ClickHouse 24.8.14.39 日志库、SQLite 主库：定向导出回归通过。该版本运行仓库既有审计表初始化时需要在测试 DSN 启用 `allow_experimental_json_type=1`。
- 覆盖字段选择、组合筛选、CSV 转义和公式防护、所有者隔离、管理员降权、过期拒绝、任务去重、500 行进度边界、空结果、行数上限和单个超大字段文件上限。
- PostgreSQL 独立测试库实际生成并逐行核对 1000000 行、10 列 CSV，文件 89889007 字节；生成约 8.87 秒，测试进程峰值 RSS 39600KiB。测试数据为固定的 Kimi 模型名、令牌名、用量数字与递增请求 ID，不包含长文本。
- 前端使用日志模块：`cd web && bun run test src/features/usage-logs`，24 个文件、422 个测试通过；修改文件 lint、格式检查、`bun run typecheck`、`bun run build` 通过；`go build ./...` 通过。
- `go test ./controller ./service ./model -count=1` 中 service、model 通过；controller 的 `TestResponsesWebSocketReusesConnectionAndSettlesEachRequest` 与 `TestResponsesWebSocketInitialUpstreamRejectionRefundsReservation` 失败。使用独立、未修改的 `d8eab2dab` worktree 执行 `go test ./controller -run '^TestResponsesWebSocket(ReusesConnectionAndSettlesEachRequest|InitialUpstreamRejectionRefundsReservation)$' -count=1`，复现相同失败；本次未修改这些 WebSocket 计费路径。

### 上线记录（2026-11-04，Asia/Shanghai）

- 16:00 发布到 `netwebs.top`，版本 `v1.0.0-rc.39-local.20261104155419.log-export.b8dd75220`，功能提交 `0d9b5847a`，构建源码提交 `b8dd75220`。线上确认只有 `newapi-1` 一个活跃实例，主库 PostgreSQL 17.11。
- Go 1.26.1，前端 `bun run typecheck`、`bun run build` 通过；执行 `GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath`，通过 ldflags 设置上述版本并嵌入前端。发布前重新执行 `go test ./controller -run '^TestLogExport' -count=1`、`go test ./service ./model -count=1`，全部通过。
- 新二进制 SHA-256：`ecb3effacd2178f4b256c384325ae4f1842bf2bff6d0de936f80284405db14ce`。上传包及各文件校验通过；Supervisor RUNNING，进程实际二进制摘要、本机和公网版本一致，metrics 正常，6 个前端入口资源本机及公网摘要一致。
- 线上仅新增 `LOG_EXPORT_DIR=/var/lib/new-api/log-exports`、`LOG_EXPORT_MAX_ROWS=10000000`、`LOG_EXPORT_MAX_MB=1024`；未修改既有环境配置行和启动脚本。目录归 `newapi` 用户，权限 0700；生成文件权限 0600。增加实例时仍必须把所有执行、下载节点挂载到同一个导出目录。
- 使用独立、不匹配真实日志的模型筛选条件向既有任务队列加入一个验证任务，线上 worker 在 1.06 秒内完成，CSV 为 0 条数据、26 字节，列为 `created_at,model_name`。校验 UTF-8 BOM、内容和权限后，删除仅此测试任务及文件，未导出真实使用记录。
- 本机和公网对导出创建、列表、下载的匿名请求均返回 401。线上没有使用真实用户凭据进行已登录下载；所有者隔离和管理员降权由四种数据库回归覆盖。
- 发布执行 `python3 /tmp/newapi-export-prepare-remote.py`、`python3 /tmp/newapi-export-publish-remote.py` 和 `python3 /tmp/newapi-export-smoke-remote.py`（通过 SSH 的标准输入执行）。发布脚本包含健康、版本、摘要、静态资源和权限检查失败时自动回滚。
- 上一版本为 `v1.0.0-rc.39-local.20261029174244.generic-error.3f51797d`，二进制摘要 `adea8b2cbf53c150697478ae3303e6229684e7ee2b0abed06e0f1e6491aa0177`。回滚备份为 `/opt/new-api/releases/v1.0.0-rc.39-local.20261104155419.log-export.b8dd75220/new-api-before-upgrade` 与同目录下权限 0600 的 `env-before-upgrade`；手动回滚命令：`python3 /opt/new-api/releases/v1.0.0-rc.39-local.20261104155419.log-export.b8dd75220/rollback.py`。
- 发布结果保存在服务器 `/opt/new-api/releases/v1.0.0-rc.39-local.20261104155419.log-export.b8dd75220/deployment.json`、`export-verification.json`、`build-info.json`；启动脚本 SHA-256 保持 `38871d77d16c1d5c50c65c2dbf6ab313289cbd7c2519cb9487e0d5eb4f72fd76`。

### 双节点导出存储（2026-11-04）

`newapi-2` 已按只读 NFSv4.2 挂载 master 的导出目录，真实服务用户读取 0600 合成 CSV 成功、写入被拒绝；后台生成和清理仍由 master 执行，slave 可以下载完成文件。该存储依赖 master 可用，不能作为 master 故障时的导出高可用方案。完整部署、ALB 待办及回退记录见 [功能 03](03-deploy-ci.md)。

### 明细与统计导出验证（2026-11-09，工作区）

- 基于 `4e53ab720` 的未提交实现，无表结构、迁移、实际扣费或权限策略变更。复用既有 `LogExportDialog`、共享 `Dialog`、`NativeSelect`、`Checkbox`、`Button`；不新建通用交互组件。
- SQLite 3.50.4、MySQL 8.0.46-0ubuntu0.24.04.4、PostgreSQL 16.15、独立 ClickHouse 24.8.14.39 日志库（SQLite 主库）：全部 `TestLogExport*` 定向回归通过。均为隔离的真实数据库，MySQL/PostgreSQL 使用本地临时测试库，未连接线上数据库。
- 覆盖明细字段、历史金额与订阅用量、线性表达式的历史档位及请求倍率、非线性/按次/缺少历史条件时不伪造分项、汇总粒度/时区/维度、类型及来源隔离、未知金额统计、夏令时重复小时、空汇总和扫描上限。沿用既有所有者及字段权限、公式注入防护、任务去重、文件大小及过期测试。
- 执行命令（DSN 为本次隔离实例；MySQL 本地初始化无密码，PostgreSQL 仅回环地址 trust）：

```bash
go test ./controller -run '^TestLogExport' -count=1 -v
env LOG_EXPORT_TEST_MYSQL='root@tcp(127.0.0.1:23369)/export_v2?charset=utf8mb4&parseTime=true&loc=Local' go test ./controller -run '^TestLogExport' -count=1 -v
env LOG_EXPORT_TEST_POSTGRES='host=127.0.0.1 port=25439 user=postgres dbname=export_v2 sslmode=disable' go test ./controller -run '^TestLogExport' -count=1 -v
env LOG_EXPORT_TEST_CLICKHOUSE='clickhouse://127.0.0.1:29009/export_v2?allow_experimental_json_type=1' go test ./controller -run '^TestLogExport' -count=1 -v
go build ./...
cd web
bun run test src/features/usage-logs
bun run typecheck
bunx oxlint -c .oxlintrc.json src/features/usage-logs/lib/log-export.ts src/features/usage-logs/components/log-export-dialog.tsx src/features/usage-logs/components/__tests__/log-export.test.tsx src/i18n/static-keys.ts
bun run build
bun run i18n:sync
```

- 日志模块 24 个前端测试文件、426 个测试通过；最终导出相关 2 个测试文件、9 个测试通过。类型检查、涉及文件 lint/格式、前端生产构建、根 Go 模块构建及 `git diff --check` 通过。七种语言的新文案均已补齐。
- 明细最多 7 天、统计最多 366 天；统计仍受既有扫描行数、文件大小和 30 分钟执行限制，另设 10000 个汇总组合的内存边界。不将这些功能回归结果表述为生产集群的吞吐量压测。当时尚未部署，当前上线记录见下文。

### 明细整月对账范围调整（2026-11-10，工作区）

- `ValidateLogExport` 的明细时间范围上限从 7 天放宽到 31 天，统计仍为 366 天；同步 `LogExportDialog` 说明及七种语言。保留原有行数、文件大小和运行时长限制，超限明确失败，不静默截断。
- 先运行 `go test ./controller -run '^TestLogExportDetailMonthRange$' -count=1`，确认旧实现拒绝整月范围；调整后 `go test ./controller -run '^TestLogExport' -count=1` 通过，覆盖 28/29/30/31 天允许、超过 31 天拒绝以及原筛选、字段及权限回归。
- 本次只修改时间范围校验和提示文案，不修改数据库查询、投影、模型、迁移、金额计算或实际扣费；无需据此重新宣称数据库兼容性验证。新增前端断言保护 31 天提示。

### 四节点上线（2026-11-10，Asia/Shanghai）

- 10:58 构建、11:00–11:02 逐台发布到 `newapi-1`~`newapi-4`，版本 `v1.0.0-rc.39-local.20261110105838.log-export-month.9d85a8069`。二进制 SHA-256 `fe62451a40c0acc6b6113e2ab5a8b97d6d55511d9d713bda52e1f4e4471b208e`；四节点 Supervisor RUNNING、运行中二进制及版本一致，原 `env` 和 `start.sh` 摘要保持不变。
- Go 1.26.1，`GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w -X github.com/QuantumNous/new-api/common.Version=<上述版本>'`；发布前导出后端测试、9 项导出前端测试、前端类型检查及生产构建通过。
- 基于 `4e53ab720ae90a5d22e9fd0bbeb50ee6a830960b` 加未提交导出改动构建；不是纯 HEAD 二进制。源码清单、逐文件摘要和源码集合摘要 `9d85a8069` 前缀存于 `build-info.json`，已跟踪文件差异存于 `source-changes.patch`，两个新增 Go 文件另存 `.source`。打包未应用工作区的请求头启动脚本改动，不修改已有线上配置。
- 每台发布前检查在途 provider 请求为零，校验上传包 SHA-256，保存旧二进制，原子替换并重启；本机版本、运行中二进制、6 个入口资源、指标或匿名接口检查失败会自动恢复旧二进制并重启。旧版本 `v1.0.0-rc.39-local.20261105101241.protocol-contract.2896db5f9`，旧二进制 SHA-256 `70b7452b96fc69cea1875855a4e3df7df721874bf5b1b7556eee25f6972def42`。
- 公网 `https://netwebs.top` 连续 8 次健康版本检查通过；6 个入口资源及包含 31 天提示的日志导出异步资源摘要匹配；匿名导出创建/列表/下载均 401。PostgreSQL 17.11 中四实例心跳在线，Prometheus 四个目标均 UP。
- 线上只创建两个独立不存在模型筛选的临时后台任务，验证 31 天详细 CSV（72 字节）及 Asia/Shanghai 按天统计 CSV（547 字节），都成功且零数据行；没有导出真实使用记录，没有主动发起付费模型调用。测试任务及 CSV 已删除，不把此验证表述为真实月账单或吞吐量压测。
- 补齐导出所需共享存储：master 对 `172.16.0.76`、`.77`、`.78` 提供 NFSv4.2 只读导出，三个 slave 通过 fstab 与 systemd automount 挂载。真实 `newapi` 用户读取 master 的 0600 合成 CSV 摘要一致，写入被拒绝，探针文件已删除。NFS 客户端范围只限这些私网地址，未改数据库、Redis、应用环境文件、启动脚本或 ALB 配置。
- 各节点归档目录：`/opt/new-api/releases/v1.0.0-rc.39-local.20261110105838.log-export-month.9d85a8069/`，包含 `build-info.json`、`deployment.json`、`release-plan.json`、`new-api-before-upgrade`、`rollback-metadata.json`、`rollback.py`。手动回滚：在对应节点执行 `python3 /opt/new-api/releases/v1.0.0-rc.39-local.20261110105838.log-export-month.9d85a8069/rollback.py`。master 另存两份 `export-*-verification.json`、NFS 配置备份 `nfs-exports-before`；slave 保存 `fstab-before`。共享存储为本次功能所需，可独立保留，不随二进制回滚撤销。
- 本地发布包：`/tmp/new-api-release-v1.0.0-rc.39-local.20261110105838.log-export-month.9d85a8069.tar.gz`，包 SHA-256 `3365a1fda3f97f1364df1d1a5a9d6e24c53d3e520648b861fdd2353b65cf4b94`。本次部署脚本位于本机 `/tmp/newapi-month-{build,upload,stage,publish,final-verify}.py`；节点保留本次 `publish.py` 与回滚脚本，最终集群报告归档为 `cluster-verification.json`。
