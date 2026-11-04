# 16 · 上游请求 ID 自动识别

- **状态**：活跃
- **引入提交**：`d65d2fc55`（自动识别供应商上游请求 ID）
- **影响面**：`common/constants.go`、`service/http.go`、`relay/channel/api_request.go`、`logger/logger.go`

## 功能说明

自动从上游 HTTP 响应头提取请求追踪编号，供已有使用日志筛选和 CSV 导出使用。兼容标准头以及供应商自定义的 `X-*-Request-Id`、`X-*-Requestid`，包括 `X-Ws-Request-Id`，无需逐个添加渠道配置。保留本地请求 ID 的原有语义，供应商/CDN 追踪编号不被解释为模型内部请求编号。

## 实现要点

- `service.CaptureUpstreamRequestID` 只读取响应头，按固定优先级选择一个编号：既有 `X-Nawei-Request-Id`、`X-Oneapi-Request-Id`，显式 `X-Upstream-Request-Id`，通用 `X-Request-Id` / `X-Requestid`，`Request-Id` / `Requestid`，最后才选择供应商扩展头。同级按不区分大小写的头名排序，大小写重复时再比较原始头名，避免 map 遍历顺序影响结果。
- 值去除首尾空白后，选择第一个非空、至多 128 字节、由可打印且无空白的 ASCII 字符构成的编号。非法或过长候选被跳过，不截断 ID。复用 `logger.IsSensitiveHTTPHeader` 的既有脱敏规则，不会将敏感头写入公开关联字段，也不会将普通会话 ID、`Traceparent` 或 `Cf-Ray` 当成请求 ID。
- `doRequest` 每次调用前清空旧编号，在收到响应头后、等待 SSE 首包前捕获，覆盖 HTTP 错误响应和有响应头但首包超时的请求。没有响应头的网络失败不继承前一次请求的编号。
- `common.UpstreamRequestIdHeaderKey` 在请求上下文记录来源头；开启既有 `LOG_REQUEST_HEADERS` 时，在原响应头日志行追加 `upstream_request_id_header=...`，不新增独立日志行。
- `IOCopyBytesGracefully` 对没有经过共享请求函数的响应也可提取 ID；复制响应头不再覆盖已经选定的编号，不覆盖客户端所见的本地 `X-Nawei-Request-Id`。

## 配置与开关

自动识别无开关。来源头日志沿用 `LOG_REQUEST_HEADERS=true`。使用日志和导出仍使用既有 `upstream_request_id` 字段，不增加表、字段或前端配置。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-11-04 | `d65d2fc55` | 新增通用及供应商请求头自动识别，提前捕获错误/超时响应编号，清理重试间旧值 |

## 注意事项

- 只影响上线后的新请求，不补写历史日志中的空 ID；CDN 产生的请求编号是否可追到模型服务需要供应商确认。
- 识别范围为共享 HTTP 请求路径及通用响应复制路径；不解析响应正文、SSE 内容或独立 SDK 的私有元数据。
- 不改变计费、数据库查询/结构、认证或会话生命周期。128 字节上限与已有 `upstream_request_id` 存储约束一致。
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
