# 17 · API 路由与协议完整性

- **状态**：活跃
- **引入提交**：`2896db5f9`（API 错误路径、媒体转换及 Responses 终态处理）
- **影响面**：`router/web-router.go`、`router/main.go`、`relay/channel/openai/relay_responses.go`、`service/log_info_generate.go`、`relaykit/relayconvert/internal/`

## 功能说明

明确区分网站页面与错误 API 请求，避免 API 调用拿到 200 和 HTML 首页。媒体转换不能静默丢弃视频，无法保留内容的转换返回明确的客户端错误；原生或已有支持的路径继续保留输入。Responses 流必须有明确终态，异常断流向仍在线的客户端发出失败事件，保留供应商真正的失败结果。

## 实现要点

- `isAPIRequest` 识别版本前缀（包括 v2/v1beta）、Messages/Responses 等接口入口、非 GET/HEAD 方法、JSON 请求及 API 认证头。插件路由优先，错误 API 在静态资源及 SPA 首页处理之前返回不缓存的 JSON 404；外置前端重定向也执行相同判断。无 API 特征的浏览器页面仍按 SPA 规则处理，不把所有未知网页路径都当成 API。
- `media.ValidateClaudeVideo` 在 Claude → Chat Completions / Responses 转换前检查视频类型、video MIME 和嵌套 tool_result 内容，使用 `ConversionLossError` 让宿主返回 400 并跳过重试。Responses → Claude 不再把视频包装为 image；Responses → Gemini 对缺少媒体载荷、未知内容块明确报错。Responses 原生及已有 Responses → Chat 视频 URL 路径保留内容，不宣称供应商一定能理解视频。
- Claude → Chat 的图片来源复用 `claudeSourceURL`，保留远程 URL 和合法 data URL；缺少来源时明确拒绝，避免 `data:;base64,%!s(<nil>)` 和 nil 来源崩溃。
- `OaiResponsesStreamHandler` 对无效 JSON/缺少事件类型停止处理；保留真实 failed/incomplete，并把带确认结果的旧版 response.done 映射为对应标准终态，保留其他字段。收到终态后结束读取；缺少终态时发送 `response.failed`，错误码 `incomplete_stream`，不自动补 completed。客户端已断开或写入失败时不再补写。
- `appendStreamStatus` 将要求终态但没有完成、incomplete/cancelled 等结果显示为 error，已有日志页面的异常展示可直接识别，不需要新增前端文案。

## 配置与开关

无新增开关、环境变量、数据库列或迁移。行为默认生效；原有请求透传、渠道和模型配置不修改。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-11-05 | `2896db5f9` | 修复 API 首页兜底、视频丢弃、URL 图片损坏和 Responses 缺失/异常终态 |

## 注意事项

- 本次遵循 `.agents/rules/billing.md`。补发的网关失败事件在 `ResponsesUsageAccumulator.Finish` 之后生成，不当作供应商明确失败重新喂给计量器，避免清除中断前已完成的图片调用或已生成文本；不强行重试已输出内容的请求，也不把失败统一当作零费用。
- 客户端仍应解析成功终态，不能只看 HTTP 200。已经开始的 SSE 无法通过改 HTTP 状态表达后来发生的失败。上游直接返回 failed 的问题不会被伪装成成功，客户反馈的 29% 比例未作为本次已验证的事实。
- 修复的是网关的数据保真和错误反馈，不新增视频抽帧或视频理解模型能力。若原生供应商收到完整内容后仍静默忽略，需要供应商解决。
- relaykit 独立性检查必须执行 `cd relaykit && GOWORK=off go build ./...`。

### 验证记录

- `go test ./router ./relay/channel/openai ./service ./relay/helper -count=1` 通过；`go test -race ./router ./relay/channel/openai ./service -run 'TestWebFallback|TestFrontendRedirect|TestResponsesStreamTerminals|TestOaiResponsesStreamHandler|TestObserveResponsesOutcome' -count=1` 通过。
- `cd relaykit && GOWORK=off go test ./...`、`GOWORK=off go build ./...` 通过；根模块 `go build ./...` 通过。回归覆盖图片 URL/base64 保留、缺失来源、视频及嵌套视频拒绝、原生/Chat 视频内容保留、上游 failed、不完整终态、旧版 done、矛盾终态、EOF、无效事件及中断后的图片计量。
- `go test ./controller -run 'TestResponsesStreamOutcomesPreserveAccounting|TestSecurityAccountDeletionConcurrentRequestsHaveOneWinner' -count=1` 通过。HTTP SSE / WebSocket 的失败、截断与用量账务回归通过，只更新 incomplete 的日志状态期望，没有修改费用断言。
- `go test ./controller -skip 'TestResponsesWebSocketReusesConnectionAndSettlesEachRequest|TestResponsesWebSocketInitialUpstreamRejectionRefundsReservation' -count=1` 最终通过（SQLite 3.50.4）。这两个 WebSocket 基线失败说明见功能 15；初次完整运行另遇账户并发删除用例失败，当前和未修改基线的定向重跑均通过，最终完整重跑也通过，未修改该认证路径。
- `relay` 包中 `TestResponseModelComparisonAndLog`、`TestResponseModelLogOmitsUnchangedModel`、`TestResponseModelHandlersCaptureBeforeConversion` 在未修改的 `16265459b` worktree 中复现相同失败；跳过这三项后 relay 全包通过。本次不改无关模型比较功能。
- 本次没有数据库模型、驱动、SQL、连接配置、序列化实现或迁移改动；SQLite 回归写入并核对既有日志 JSON。没有把旧版 MySQL/PostgreSQL/ClickHouse 验证记录当作本次重新运行的矩阵。前端源码未改，发布包复用已有前端构建资源。

### 双节点发布（2026-11-05，Asia/Shanghai）

- 用户明确表示当前无流量、可直接发布后，逐台检查 HTTP 在途为 0 且业务请求计数连续 30 秒不变，再执行 `supervisorctl stop/start new-api`。没有修改 ALB 权重、环境文件或启动脚本。
- slave 10:16:37–10:16:53 更新；检查私网健康及双目标监控后，master 10:17:46–10:18:03 更新。两台均为 `v1.0.0-rc.39-local.20261105101241.protocol-contract.2896db5f9`，源码 `2896db5f9`，运行中二进制摘要 `70b7452b96fc69cea1875855a4e3df7df721874bf5b1b7556eee25f6972def42`。
- 两台 Supervisor RUNNING，6 个前端入口资源摘要分别校验通过，self/导出匿名访问保持 401，页面正常返回 HTML。公网连续版本检查通过，`GET /messages`、`GET /responses`、`GET /v2/responses`、`POST /missing-release-endpoint` 均返回 JSON 404；两台 Prometheus target 均 UP。
- 只通过合成数据、单元及集成回归验证媒体/流式错误，不额外调用付费模型，也未声称客户报告的 29% 上游失败率已消除。
- 两台发布记录为 `/opt/new-api/releases/v1.0.0-rc.39-local.20261105101241.protocol-contract.2896db5f9/deployment.json`、`cluster-verification.json`。若本机验证失败，发布脚本自动回滚；手动回滚各自节点的命令：`python3 /opt/new-api/releases/v1.0.0-rc.39-local.20261105101241.protocol-contract.2896db5f9/rollback.py`。备份是同目录的 `new-api-before-upgrade`，恢复上一版请求 ID 配置版本；回滚同样应先确认排空。
