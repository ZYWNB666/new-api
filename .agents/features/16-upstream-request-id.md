# 16 · 上游请求 ID 自动识别

- **状态**：活跃
- **引入提交**：工作区（基于 `7d4bcc739`，尚未提交）
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
| 2026-11-04 | 工作区（基线 `7d4bcc739`） | 新增通用及供应商请求头自动识别，提前捕获错误/超时响应编号，清理重试间旧值 |

## 注意事项

- 只影响上线后的新请求，不补写历史日志中的空 ID；CDN 产生的请求编号是否可追到模型服务需要供应商确认。
- 识别范围为共享 HTTP 请求路径及通用响应复制路径；不解析响应正文、SSE 内容或独立 SDK 的私有元数据。
- 不改变计费、数据库查询/结构、认证或会话生命周期。128 字节上限与已有 `upstream_request_id` 存储约束一致。
- 回归集中在既有 `relay/channel/api_request_test.go`，覆盖用户提供的 CDN 响应头、标准与供应商头冲突、大小写、多值、非法值和长度边界、无响应头重试、首包超时、HTTP 错误、本地请求 ID 保留。
