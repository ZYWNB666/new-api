# 16 · 上下游请求 ID 识别与配置

- **状态**：活跃
- **引入提交**：`d65d2fc55`（自动识别供应商上游请求 ID）
- **影响面**：`common/constants.go`、`service/http.go`、`relay/channel/api_request.go`、`logger/logger.go`、`relaykit/dto/request_id.go`、`model/user.go`、`model/log.go`、`middleware/auth.go`、渠道/用户编辑及日志前端

## 功能说明

自动从上游 HTTP 响应头提取请求追踪编号，供已有使用日志筛选和 CSV 导出使用。兼容标准头以及供应商自定义的 `X-*-Request-Id`、`X-*-Requestid`，包括 `X-Ws-Request-Id`，默认无需逐个添加渠道配置。渠道可覆盖自动规则；管理员也可按用户指定入站客户端 ID 头，分别记录本地、供应商和客户端 ID。保留本地请求 ID 的原有语义，供应商/CDN 追踪编号不被解释为模型内部请求编号。

## 实现要点

- `service.CaptureUpstreamRequestID` 只读取响应头，按固定优先级选择一个编号：既有 `X-Nawei-Request-Id`、`X-Oneapi-Request-Id`，显式 `X-Upstream-Request-Id`，通用 `X-Request-Id` / `X-Requestid`，`Request-Id` / `Requestid`，最后才选择供应商扩展头。同级按不区分大小写的头名排序，大小写重复时再比较原始头名，避免 map 遍历顺序影响结果。
- 值去除首尾空白后，选择第一个非空、至多 128 字节、由可打印且无空白的 ASCII 字符构成的编号。非法或过长候选被跳过，不截断 ID。复用 `logger.IsSensitiveHTTPHeader` 的既有脱敏规则，不会将敏感头写入公开关联字段，也不会将普通会话 ID、`Traceparent` 或 `Cf-Ray` 当成请求 ID。
- `doRequest` 每次调用前清空旧编号，在收到响应头后、等待 SSE 首包前捕获，覆盖 HTTP 错误响应和有响应头但首包超时的请求。没有响应头的网络失败不继承前一次请求的编号。
- `common.UpstreamRequestIdHeaderKey` 在请求上下文记录来源头；开启既有 `LOG_REQUEST_HEADERS` 时，在原响应头日志行追加 `upstream_request_id_header=...`，不新增独立日志行。
- `IOCopyBytesGracefully` 对没有经过共享请求函数的响应也可提取 ID；复制响应头不再覆盖已经选定的编号，不覆盖客户端所见的本地 `X-Nawei-Request-Id`。

- `dto.CompileRequestIDHeaderRule` 在服务端验证并缓存正则（最多 1024 个规则），不依赖宿主模块。非法历史规则不采集 ID；敏感头即使显式指定也跳过。
- `CaptureClientRequestID` 在 `TokenAuth` 已认证并加载启用用户的设置后采集。`appendClientRequestID` 在成功/错误日志的既有 `Other` 写入公开 `client_request_id` 与 `client_request_id_header`。不替换本地 ID 或供应商 ID。
- `User.EditWithTx` 合并管理员策略，`UpdateUserSetting` 与自助资料更新保留原策略；GetSelf 不公开策略，管理员 GetUser 返回独立编辑字段。既有事务、权限检查和缓存发布继续使用。
- 前端复用现有渠道额外配置表单、用户编辑表单、详情 `DetailRow` 与 CSV 字段选择，不引入通用 UI 替代组件。七种语言已同步。

## 配置与开关

- 渠道编辑 → 其他设置 → 渠道额外设置 → 上游请求 ID 请求头：`setting.upstream_request_id_header`。留空自动识别；例如供应商头 `X-Ws-Request-Id`。
- 用户管理 → 编辑用户 → 客户端请求 ID 请求头：`client_request_id_header`，保存到既有用户 setting。留空禁用采集；例 `X-Client-Request-Id`。
- 两者支持精确头名或 `re:` 前缀的 Go/RE2 正则，例如 `re:^X-(Client|Customer)-Request-Id$`；默认忽略大小写、匹配整个头名称，取完整有效值，不截取正则分组。规则最多 256 字节。显式规则未匹配时为空，不回退自动识别。多个匹配头按头名排序，取第一个有效候选。
- 来源头日志沿用 `LOG_REQUEST_HEADERS=true`。日志详情新增客户端 ID，后台 CSV 可选择 `client_request_id`；未新增客户端 ID 查询索引或筛选接口。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-11-04 | `d65d2fc55` | 新增通用及供应商请求头自动识别，提前捕获错误/超时响应编号，清理重试间旧值 |
| 2026-11-04 | `00d256998` | 新增渠道/用户精确头名及正则配置，客户端 ID 独立记录、详情展示与 CSV 导出 |

## 注意事项

- 只影响上线后的新请求，不补写历史日志中的空 ID；CDN 产生的请求编号是否可追到模型服务需要供应商确认。
- 识别范围为共享 HTTP 请求路径及通用响应复制路径；不解析响应正文、SSE 内容或独立 SDK 的私有元数据。
- 不改变计费值、表结构、认证或会话生命周期；日志字段遵循 `.agents/rules/billing.md`，只追加追踪元数据。CSV 选择客户端 ID 时读取既有 `other`，只输出客户端 ID，不输出整个元数据或管理员诊断。128 字节上限与已有 `upstream_request_id` 存储约束一致。
- 客户端 ID 来自用户输入，仅用于追踪，不能作为身份、权限或去重凭据。禁用采集不清除已有日志。只覆盖共享 HTTP/TokenAuth 路径，不承诺独立 SDK 私有元数据或未认证请求采集。
- 修改 relaykit 后必须执行 `cd relaykit && GOWORK=off go build ./...`。
- 回归集中在既有 `relay/channel/api_request_test.go`，覆盖用户提供的 CDN 响应头、标准与供应商头冲突、大小写、多值、非法值和长度边界、无响应头重试、首包超时、HTTP 错误、本地请求 ID 保留。


### 验证及上线记录（2026-11-04，Asia/Shanghai）

- `go test ./relay/channel ./middleware ./service ./logger -count=1`、`go test -race ./relay/channel -run 'TestUpstreamRequestIDRecognition|TestDoRequestRetryDoesNotReuseUpstreamRequestID|TestDoRequestProviderResponseHeaders' -count=1` 通过。
- `go test ./controller -run 'TestLogExport|TestRelayErrorHandler' -count=1`、根模块 `go build ./...` 通过。未修改前端、relaykit 或数据库模型/查询，不涉及新表、迁移或计费值。
- 16:28 发布至 `netwebs.top`，版本 `v1.0.0-rc.39-local.20261104162549.request-id.d65d2fc55`，源码提交 `d65d2fc55`，Go 1.26.1；`GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath`，通过 ldflags 设置版本。
- 二进制 SHA-256：`4f4092502b73b630996cd5b1ce5fd7b4a755deeb05e4fe44f7b2ad60b0b50fcb`。运行中进程摘要匹配发布包，Supervisor RUNNING，本机、公网及独立客户端健康和版本检查通过，metrics 正常，6 个前端入口资源摘要一致，导出创建、列表、下载的匿名请求仍为 401。
- 线上环境配置和启动脚本内容摘要均与发布前一致，既有 CSV 导出目录仍为 `/var/lib/new-api/log-exports`。发布通过 SSH 的 `python3 -` 依次执行本地 `/tmp/newapi-request-id-prepare-remote.py` 与 `/tmp/newapi-request-id-publish-remote.py`；失败自动回滚。
- 发布后对最近 2MiB 日志进行只读检查，观察到 3 条使用 `upstream_request_id_header=X-Request-Id` 的真实 provider 响应日志。检查时渠道 13 尚无新样本；`X-Ws-Request-Id` 使用用户给出的原始样本回归验证，没有额外发起供应商调用或回填历史记录。
- 上一版 `v1.0.0-rc.39-local.20261104155419.log-export.b8dd75220`，摘要 `ecb3effacd2178f4b256c384325ae4f1842bf2bff6d0de936f80284405db14ce`。备份位于 `/opt/new-api/releases/v1.0.0-rc.39-local.20261104162549.request-id.d65d2fc55/new-api-before-upgrade`；手动回滚：`python3 /opt/new-api/releases/v1.0.0-rc.39-local.20261104162549.request-id.d65d2fc55/rollback.py`。
- 发布结果与只读观察结果保存在服务器 `/opt/new-api/releases/v1.0.0-rc.39-local.20261104162549.request-id.d65d2fc55/deployment.json` 和 `request-id-observation.json`。

### 可配置规则验证（2026-11-04）

- `go test ./controller ./relay/channel -run 'TestRequestIDHeaderPolicies|TestLogExportFiltersFieldsAndPermissions|TestUpstreamRequestIDRecognition|TestDoRequestRetryDoesNotReuseUpstreamRequestID' -count=1` 通过；规则保存/清空/拒绝非法配置、自助更新不可伪造、普通用户权限拒绝、三种 ID 隔离及成功/错误日志均覆盖。
- SQLite 3.50.4、MySQL 8.0.46、PostgreSQL 17.11 真实实例通过策略持久化与日志写入测试；MySQL/PostgreSQL 使用独立主库及日志库。CSV 新字段在上述三种数据库及独立 ClickHouse 24.8.14.39 日志库均通过，旧记录缺失客户端元数据时为空。不新增迁移或数据库列，用户编辑辅助字段 `gorm:"-:all"`。
- 根目录数据库命令（变量表示隔离测试 DSN，不含凭据）：
  ```bash
  env TEST_MANAGE_USER_DIALECT=mysql TEST_MYSQL_DSN="$MYSQL_DSN" TEST_MANAGE_USER_SEPARATE_LOG_DB=1 go test ./controller -run '^TestRequestIDHeaderPolicies$' -count=1 -v
  env TEST_MANAGE_USER_DIALECT=postgres TEST_POSTGRES_DSN="$POSTGRES_DSN" TEST_MANAGE_USER_SEPARATE_LOG_DB=1 go test ./controller -run '^TestRequestIDHeaderPolicies$' -count=1 -v
  env LOG_EXPORT_TEST_MYSQL="$MYSQL_DSN" go test ./controller -run '^TestLogExportFiltersFieldsAndPermissions$' -count=1
  env LOG_EXPORT_TEST_POSTGRES="$POSTGRES_DSN" go test ./controller -run '^TestLogExportFiltersFieldsAndPermissions$' -count=1
  env LOG_EXPORT_TEST_CLICKHOUSE="$CLICKHOUSE_DSN" go test ./controller -run '^TestLogExportFiltersFieldsAndPermissions$' -count=1
  ```
- `go test ./relay/channel ./middleware ./service ./model -count=1`、`go test -race ./relay/channel -run 'TestUpstreamRequestIDRecognition|TestDoRequestRetryDoesNotReuseUpstreamRequestID' -count=1` 通过。`cd relaykit && GOWORK=off go build ./...` 通过。
- 前端四个直接修改的测试文件共 119 项通过；修改文件 oxlint、oxfmt、`bun run build:check`（类型检查及生产构建）通过。
- 安全范围参考 [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html)、[Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) 及 [ASVS 稳定版 5.0.0](https://owasp.org/www-project-application-security-verification-standard/)。适用控制为服务端管理员权限校验、自助更新不可覆盖管理员策略、身份鉴别与不可信追踪值隔离、敏感认证头不进入公开追踪日志。回归覆盖普通用户拒绝、自助字段伪造、敏感头跳过与 AuthVersion 保留；不声明整个系统 ASVS 合规。
- 完整前端范围 `cd web && bun run test src/features/channels src/features/users src/features/usage-logs`：48 文件、726 项通过；根模块 `go build ./...` 通过。完整 controller 回归跳过已在原始基线复现失败的两个 WebSocket 用例，其他用例全部通过，命令：`go test ./controller -skip 'TestResponsesWebSocketReusesConnectionAndSettlesEachRequest|TestResponsesWebSocketInitialUpstreamRejectionRefundsReservation' -count=1`。既有失败说明见功能 15，不修改其计费行为。

### 可配置规则上线（2026-11-04，Asia/Shanghai）

- 17:08 发布至 `netwebs.top`，源码 `4138bffd8`（功能提交 `00d256998`），版本 `v1.0.0-rc.39-local.20261104170732.request-id-config.4138bffd8`。二进制 SHA-256：`59c85bc2ab0db38d1b4633028dae6f5bbb80baed812e1ed59f73dc088deca008`。
- `newapi-1` 上 Supervisor RUNNING，运行中进程二进制摘要匹配发布包；本机、公网健康及版本检查通过；metrics 正常，6 个前端入口资源本机及公网摘要匹配。导出创建、列表、下载的匿名请求仍全部 401。未使用真实用户凭据修改业务配置或调用供应商，客户端采集保持默认关闭。
- 环境和启动脚本摘要与发布前一致，不新增迁移；CSV 持久化目录保持 `/var/lib/new-api/log-exports`。发布使用本地 `/tmp/newapi-request-id-prepare-remote.py` 和 `/tmp/newapi-request-id-publish-remote.py` 经 SSH `python3 -` 执行，后者包含失败自动回滚。
- 上一版本 `v1.0.0-rc.39-local.20261104162549.request-id.d65d2fc55`，摘要 `4f4092502b73b630996cd5b1ce5fd7b4a755deeb05e4fe44f7b2ad60b0b50fcb`。备份及发布结果：`/opt/new-api/releases/v1.0.0-rc.39-local.20261104170732.request-id-config.4138bffd8/new-api-before-upgrade`、`deployment.json`、`build-info.json`；手动回滚命令：`python3 /opt/new-api/releases/v1.0.0-rc.39-local.20261104170732.request-id-config.4138bffd8/rollback.py`。
