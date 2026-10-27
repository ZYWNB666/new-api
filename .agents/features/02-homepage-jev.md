# 02 · 首页重构与 Jev 决策支持

- **状态**：活跃
- **引入提交**：`70736a3d6`、`bac315d77`、`bb409e7f6`、`5ae8aa8f6`、`2bad79510`、`ce6ade566`、`46eecfa2c`
- **影响面**：后端 relay 新增 provider、前端首页与文档页

## 功能说明

两部分：

1. **Jev 决策支持**：新增 `/v1/decisions` 端点，透传到 Jev/typesafe 上游并按返回 usage 结算。
2. **首页重构**：Hero / 展示区 / 文档页重做，主题样式大幅调整。

## 实现要点

### Jev
- `relay/channel/jev/adaptor.go`：实现 `channel.Adaptor`，仅支持 `RelayModeJev`；URL 分流 typesafe.ai → `/v1/systemone`，否则 `/alpha/decisions`。
- `relay/channel/jev/constant.go`：`ModelList`（typesafe / jev-1.13）、`ChannelName="jev"`。
- `relay/jev_decision_handler.go`：`/v1/decisions` 透传上游 + 原样回写响应 + 从 `JevDecisionResponse.Usage` 结算。
- `relaykit/dto/jev_decision_request.go`：请求/响应/usage DTO（`RawBody` 透传未知字段）。
- `relaykit/types/relay_format.go`：`RelayFormatJev="jev"`。
- 注册：`relay/relay_adaptor.go`、`relay/constant/relay_mode.go`、`router/relay-router.go`。

### 首页
- `web/src/features/home/`：Hero、产品展示区、Jev 展示区、`use-home-api-url.ts`。
- `web/src/routes/docs.tsx` 文档页；`web/src/styles/index.css`、`theme.css` 主题重做。
- 移除 Hero 波纹动画（`ce6ade566`）。
- 顶导航链接逻辑 `web/src/hooks/use-top-nav-links.ts`（去掉 docs nav 与系统更新 action，见 `3817d7dae`）。

## 配置与开关

无独立开关。Jev 需要新建 channel 且 `type=jev`。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-09-26 | `70736a3d6` | Jev 请求处理与路由配置 |
| 2026-09-26 | `bac315d77` | Jev 展示区与首页布局 |
| 2026-09-26 | `bb409e7f6` | Jev 默认端点信息与示例请求构建 |
| 2026-10-21 | `5ae8aa8f6` | 文档页与主题样式 |
| 2026-10-21 | `2bad79510` | UI 细节调整 |
| 2026-10-21 | `ce6ade566` | 移除 Hero 波纹动画 |
| 2026-10-22 | `46eecfa2c` | ShellLayout / Hero 测试 |
| 2026-10-23 | `3817d7dae` | 顶导航去掉 docs nav 与系统更新 action |

## 注意事项

- Jev 走透传 + usage 结算，改计费前必读 `.agents/rules/billing.md`。
