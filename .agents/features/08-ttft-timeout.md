# 08 · TTFT 首包超时与 499 重试

- **状态**：活跃
- **引入提交**：`4ef72dbb7`（超时与重试）、`e74fa8092`（全请求耗时计时）
- **影响面**：`relay/channel`、`model/request_policy.go`、`common/constants.go`、前端请求策略设置

## 功能说明

流式请求增加**首包超时（TTFT, Time To First Token）**保护：上游在限定时间内没有返回第一个字节就取消并返回内部状态码 `499`，由既有重试策略决定是否换渠道重试。同时把耗时统计从「连接成功后」扩展到「从连接建立开始」。

## 实现要点

- `relay/channel/first_chunk.go`：`awaitFirstChunk` 在协议处理器提交响应头或发 ping **之前**持有响应，只缓冲 1 字节；超时通过 `context.CancelCause` 取消，取消原因 `errFirstChunkTimeout`。
- `relay/channel/request_timing.go`：`upstreamTiming` 用 `httptrace.ClientTrace` 分阶段计时（`connection_acquire`/`dns`/`tcp_connect`/`tls_handshake`/`request_write`/`response_headers`/`first_body_wait`），输出 `upstream timing:` JSON 日志。
- `relay/channel/api_request.go`：按 `IsStream` 与 `RelayMode` 决定是否启用 TTFT；超时错误映射为 `ErrorCodeUpstreamFirstChunkTimeout`（499）。
- `relaykit/types/error.go`：`ErrorCodeUpstreamFirstChunkTimeout = "upstream_first_chunk_timeout"`。
- 重试豁免：客户端取消、响应已提交、Responses 有状态会话/工具副作用（`previous_response_id`、`conversation`、非 function 类型工具）不重试。
- 适用入口：Chat Completions、Completions、Messages、Responses、Gemini 流式生成。**不适用**：非流式、WebSocket、异步任务、音视频/图片生成、独立 SDK 请求路径。
- 配置：`RelayTTFTTimeout`（秒，默认 30，环境变量 `RELAY_TTFT_TIMEOUT`）、`RelayTTFTModelTimeouts`（JSON，按模型覆盖）。
- 前端：`web/src/features/system-settings/request-policies/` 的 retry 区块可配。

## 已知缺口（排查痛点）

**首包超时发生时不会打印 provider 响应头**，导致无法用上游 `X-Request-Id` 对账。原因：`api_request.go` 中响应头日志的条件是 `response != nil`，而 `first_chunk_timeout` 路径下 `relayClient.Do()` 因 context 取消返回 `err != nil` 且 `response == nil`，该日志行被结构性跳过。

线上实证（2026-10-27，渠道 #5，GLM-5.3-Flash，1.6MB prompt）：
- 14 次 `first_chunk_timeout`，13 次卡在 `response_headers` 阶段，1 次卡在 TLS。
- 全部无 `provider response headers` 日志；而同渠道成功请求有 `status=200` 响应头。
- 说明上游**能**返回响应头，只是大 payload 下首包超过 60 秒。

**待办**：在 `send()` 的失败路径（`response == nil`）也记录渠道 ID、attempt、错误类型与耗时，或把响应头日志改为 defer 保证执行。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-10-24 | `4ef72dbb7` | TTFT 超时、499 重试、下游响应头日志 |
| 2026-10-25 | `e74fa8092` | 耗时统计扩展到连接建立阶段，新增分阶段 timing 日志 |

## 注意事项

- 改超时逻辑必须覆盖：TLS 阶段取消、无响应头、有心跳无正文、客户端取消、其他网络超时（已有测试在 `relay/channel/api_request_test.go`）。
- 取消不保证上游停止计算或免除上游费用。
