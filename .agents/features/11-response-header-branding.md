# 11 · 对外响应标识改写

- **状态**：活跃
- **引入提交**：`b079e605f`
- **影响面**：`common/response_headers.go`、`middleware/cors.go`、`middleware/utils.go`、`service/error.go`、`controller/relay.go`、`controller/plugin_protocol*.go`、`controller/billing.go`、`relaykit/types/error.go`

## 功能说明

把响应头中暴露的上游产品标识（`oneAPI` / `newAPI`）改写为自有品牌「Nawei」，避免向上游/客户端泄露网关实现。对外本地错误类型统一使用 `api_error`，覆盖令牌鉴权、渠道分配、未实现接口、任务协议和 Claude 包装等路径；旧上游错误类型在协议转换输出时归一化。

## 实现要点

- `common/constants.go`：品牌响应头常量。
- `common/response_headers.go`（新增）：响应头改写逻辑。
- `middleware/cors.go`：CORS 暴露头同步更新。
- `service/http.go`：HTTP 客户端侧处理。
- `controller/relay.go`、`relay/channel/api_request.go`：接入点。
- 前端 `web/src/features/docs/index.tsx` 与 i18n 同步文案。
- 测试：`middleware/logger_test.go`、`controller/relay_task_plugin_test.go`、`web/src/features/docs/__tests__/guide.test.tsx`。

- `relaykit/types.ErrorTypeAPIError` 是统一错误类型；`ErrorTypeNewAPIError` 保留为源码兼容别名。`ToOpenAIError()` / `ToClaudeError()` 把旧 `new_api_error` 类型归一化，OpenAI 转换也归一化同名旧错误码。
- API 契约回归集中在 `controller/relay_error_log_test.go`，覆盖多协议输出和缺失、无效、已过期令牌的统一 401、请求 ID 及终止转发；既有 `middleware/distributor_test.go` 验证无可用渠道时仍保留 `model_not_found`。

## 配置与开关

无开关，全局生效。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-10-25 | `b079e605f` | 初始引入：响应头 oneAPI/newAPI → Nawei |
| 2026-10-29 | `f4a33abfa` | 对外本地错误统一使用 api_error，兼容归一化旧上游错误类型 |

## 注意事项

- 新增会暴露实现身份的响应头时，需评估是否也要改写。

- 本次只归一化错误类型及同名旧错误码，业务错误码（如 `model_not_found`）、HTTP 状态和请求 ID 保持既有语义。客户端如按旧类型字符串分支，需改用 `api_error`。上游自由文本消息、任意元数据不做字符串擦写。
- 验证：`go test ./controller ./middleware ./service ./pkg/adaptiverouting -skip 'TestResponsesWebSocketReusesConnectionAndSettlesEachRequest|TestResponsesWebSocketInitialUpstreamRejectionRefundsReservation' -count=1` 通过，跳过的两组 WebSocket 测试为功能 14 已记录的既有基线问题；`go test ./controller ./middleware ./service -run 'TestPublicErrorsUseGenericType|TestTokenFailuresKeepGenericErrorAndAbort|TestDistributeHidesTaskPluginDetailsButLogsDiagnostics|TestRelayErrorHandler' -count=1` 通过。
- `go test -race ./controller -run 'TestPublicErrorsUseGenericType|TestTokenFailuresKeepGenericErrorAndAbort' -count=1`、根模块 `go build ./...`、`cd relaykit && GOWORK=off go build ./...` 与 `GOWORK=off go test ./... -count=1` 通过。涉及 relaykit 的后续修改必须继续独立构建。
- 鉴权错误呈现参考 [OWASP ASVS 5.0.0（v5.0.0-6.3.8）](https://github.com/OWASP/ASVS/blob/v5.0.0/5.0/en/0x15-V6-Authentication.md)、[Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html#authentication-and-error-messages) 和 [Session Management Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html)。适用控制为保持通用失败响应、服务端拒绝无效/过期凭据及不在错误响应暴露凭据；测试验证上述失败路径仍终止处理。本次未改变认证判定或会话生命周期，不宣称整个认证系统已通过 ASVS 验证。
- 未修改生产数据库查询、结构或计费逻辑；认证回归沿用 SQLite 测试夹具，不作为三数据库兼容验证。
