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

- **该开关不在 env 文件里**，由各节点的 `start.sh` 用 `export LOG_REQUEST_HEADERS=true` 提供。新增节点或替换 `start.sh` 时必须保留这一行，否则该节点完全不打印客户端请求头；ALB 会把请求打到任意节点，表现为"某些请求/模型没有请求头"，容易误判为模型或协议问题。
- 只有 `RouteTag("relay")` 的请求会打印（`/v1/**`、`/v1beta/**`、`/pg`、`/mj`、任务与视频路由等）；管理接口（`/api/**`）和网页请求不会打印。
- 新增脱敏头类型时同步补测试；审计日志禁止出现可用令牌/密钥。

### 2026-11-09 slave 缺失请求头日志

- 现象：请求 `202611091212577059735608268d9d69e2c9zFe`（模型 `GLM-5.3-FlashX-Legal`，`POST /v1/chat/completions`，200）没有 `client request headers:` 行，用户怀疑只有该模型没有。
- 定位：该请求落在 slave `newapi-2`（47.100.175.171）。该节点日志中 `client request headers` 行数为 **0**，同节点其他 relay 请求同样没有，与模型无关。根因是部署 slave 时编写的 `start.sh` 刻意省略了 `export LOG_REQUEST_HEADERS=true`（当时认为是 master 专有），而 master 的该行在 `start.sh` 而非 `env` 中，复制 env 不会带上它。master 同期有 748 行请求头日志。
- 修复：把 `export LOG_REQUEST_HEADERS=true` 加回 `deploy/cluster/start.sh`，用 `deploy/cluster/restart-slave.sh all --no-drain` 逐台重启（用户选择跳过排空）。重启后三台进程环境变量均含 `LOG_REQUEST_HEADERS=true`，本机 relay 请求验证 `client request headers:` 正常输出。master 未重启（pid 867088 不变），重启窗口期无 relay 请求被中断。
