# 11 · 响应头品牌标识改写

- **状态**：活跃
- **引入提交**：`b079e605f`
- **影响面**：`common/response_headers.go`、`middleware/cors.go`、`service/http.go`

## 功能说明

把响应头中暴露的上游产品标识（`oneAPI` / `newAPI`）改写为自有品牌「Nawei」，避免向上游/客户端泄露网关实现。

## 实现要点

- `common/constants.go`：品牌响应头常量。
- `common/response_headers.go`（新增）：响应头改写逻辑。
- `middleware/cors.go`：CORS 暴露头同步更新。
- `service/http.go`：HTTP 客户端侧处理。
- `controller/relay.go`、`relay/channel/api_request.go`：接入点。
- 前端 `web/src/features/docs/index.tsx` 与 i18n 同步文案。
- 测试：`middleware/logger_test.go`、`controller/relay_task_plugin_test.go`、`web/src/features/docs/__tests__/guide.test.tsx`。

## 配置与开关

无开关，全局生效。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-10-25 | `b079e605f` | 初始引入：响应头 oneAPI/newAPI → Nawei |

## 注意事项

- 新增会暴露实现身份的响应头时，需评估是否也要改写。
