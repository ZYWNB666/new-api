# 13 · `prompt_cache_options` 字段支持

- **状态**：活跃
- **引入提交**：`cf690561a`
- **影响面**：`relaykit/dto/openai_request.go`、Moonshot 适配器

## 功能说明

在 OpenAI 兼容请求 DTO 中支持 `prompt_cache_options` 字段，使 Moonshot 等上游的缓存选项可以透传。

## 实现要点

- `relaykit/dto/openai_request.go`：新增字段（指针类型 + `omitempty`，遵循可选标量字段规范）。
- `relay/channel/moonshot/adaptor_test.go`：+30 行覆盖新字段。
- 注意 `relaykit/` 是独立 Go 模块，验证用 `cd relaykit && GOWORK=off go build ./...`。

## 配置与开关

无开关，客户端传字段即透传。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-10-24 | `cf690561a` | 初始引入：`prompt_cache_options` 字段与测试 |

## 注意事项

- 该字段若参与计费（缓存折扣），改动前必读 `.agents/rules/billing.md`。
