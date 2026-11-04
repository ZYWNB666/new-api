# 15 · 使用日志后台 CSV 导出

- **状态**：活跃
- **引入提交**：工作区变更，尚未提交（基于 `d8eab2dab`；该 SHA 是基线，不是功能提交）
- **影响面**：`model/log_export.go`、`service/log_export.go`、`controller/log_export.go`、`router/api-router.go`、`web/src/features/usage-logs/`

## 功能说明

使用日志的通用日志页支持按已生效的时间、类型、模型、令牌、分组、用户名、渠道和请求 ID 条件创建后台 CSV 导出。用户可单独勾选导出字段，管理员可额外选择用户 ID、用户名和渠道 ID；个人视图只导出当前用户的日志。任务存入既有 `system_tasks` 表，关闭页面后仍会继续执行，重新打开导出窗口可查看最近任务并下载完成的文件。

## 实现要点

- `model.OpenLogExportRows` 使用单条查询和数据库游标流式读取，无全量切片、重复 COUNT 或 OFFSET 分页；`model.LogExportColumns` 同时验证字段白名单和构造查询投影，不读取未选择的元数据大字段。
- `service.LogExportHandler` 复用 `EnqueueSystemTask` 的唯一活动键和任务租约，全集群只生成一个导出。每 500 条更新进度，生成 `.part` 后原子重命名，失败时删除半成品；`logExportWriter` 在写入边界限制字节数。
- `service.logExportCleanupHandler` 每小时清理过期 CSV 和崩溃遗留的临时文件；进程崩溃后的任务按既有租约过期机制标记失败，用户重新创建任务，不自动续传生成。
- `CreateLogExport`、`ListLogExports`、`DownloadLogExport` 使用既有 `UserAuth`；服务端校验管理员范围、字段权限和下载所有者。管理员降权后无法下载原先创建的全用户导出。不导出 `other`、请求头或权限诊断。
- CSV 使用 UTF-8 BOM、标准引号转义和公式注入防护，列名为稳定字段标识。时间为 UTC，耗时为秒，额度为原始整数额度单位，不改变计费值。
- `LogExportDialog` 复用共享 `Dialog`、`Checkbox`、`Button` 和 `LogsFilterToolbar.actionStart`。已有兑换码导出仅处理兑换码及 TXT/Markdown，不能承载日志筛选和后台任务，因此新增日志业务组件，不新增通用弹窗实现。
- `downloadLogExport` 在支持 File System Access 的浏览器里流式写入用户选择的文件；其他浏览器仅对不超过 128MiB 的文件使用 Blob 下载，较大的文件提示更换支持直接保存的浏览器。

## 配置与开关

- 使用入口：使用日志 → 通用日志 → 导出日志。
- `LOG_EXPORT_DIR`：默认 `./data/log-exports`。所有执行节点、下载节点必须挂载同一持久化目录；目录不应通过静态站点公开。
- `LOG_EXPORT_MAX_ROWS`：默认 10000000 行。
- `LOG_EXPORT_MAX_MB`：默认 1024MiB，代码限制配置在 1–10240MiB。
- 时间范围最多 7 天，未来结束时间在创建时截到当前时间；单任务最多执行 30 分钟。
- 下载有效期为任务创建后 24 小时，清理每小时执行。执行任务需要至少一个 master 节点。
- 最近任务窗口在集群最新 100 条导出记录中筛选当前所有者。并发生成受全集群活动任务约束，不需要新增表、迁移或依赖。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-11-04 | 工作区（基线 `d8eab2dab`） | 新增按筛选条件及选定字段生成后台 CSV、进度查询、权限校验和下载 |

## 注意事项

- 导出的额度字段遵循 `.agents/rules/billing.md`，原样读取，不参与预扣、结算、退款或额度换算。
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
