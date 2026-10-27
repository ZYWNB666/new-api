# 09 · 请求头日志（脱敏与截断）

- **状态**：活跃
- **引入提交**：`2b92efa58`
- **影响面**：`middleware/logger.go`

## 功能说明

把客户端请求头写入日志，用于排查上游/客户端问题；同时对敏感信息脱敏、对超长值截断，避免日志泄露凭证或被打爆。

## 实现要点

- `middleware/logger.go`：记录 `client request headers:` 行，输出为 JSON。
- 脱敏：`Authorization` 等敏感头替换为 `[REDACTED]`。
- 截断：单值超长截断为 `[TRUNCATED]`，多值只保留前若干项；头部数量过多时输出 `"_truncated":["too many headers"]`。
- 控制开关：环境变量 `LOG_REQUEST_HEADERS`（默认 false）。
- 日志行数受控：一条请求的头日志不会注入多行。
- 测试：`middleware/logger_test.go`。

## 配置与开关

`LOG_REQUEST_HEADERS=true` 开启。线上已开启（见功能 03）。

## 关联

- provider **响应**头日志在同一开关下由 `relay/channel/api_request.go` 打印（`provider response headers:` 行）。
- 该响应头日志在首包超时路径不生效，见功能 08「已知缺口」。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-10-22 | `2b92efa58` | 初始引入：请求头日志、脱敏、截断 |
| 2026-10-24 | `4ef72dbb7` | 同开关下新增 provider 响应头日志 |

## 注意事项

- 新增脱敏头类型时同步补测试；审计日志禁止出现可用令牌/密钥。
