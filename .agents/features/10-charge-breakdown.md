# 10 · 使用日志计费明细

- **状态**：活跃
- **引入提交**：`3414e7482`
- **影响面**：`service/text_quota.go`、`relay/common/relay_info.go`、`web/src/features/usage-logs/`

## 功能说明

在使用日志中展示**本次请求实际的计费明细**：各计费项（prompt / completion / cache read / cache write 及其 5m/1h 分档）的用量与生效单价，让用户能核对账单构成。

## 实现要点

- `service/text_quota.go`：`textQuotaSummary` 新增 `BillingTokens` / `BillingPrices`（键 `p`/`c`/`cr`/`cc`/`cc5m`/`cc1h`），记录本次运行的精确用量与有效价格。
- `service/text_quota.go`：`PostTextConsumeQuota` 写入 `billing_charge_tokens`、`billing_charge_prices`、`billing_quota_per_unit`，并公开 `cache_creation_tokens_5m` / `cache_creation_tokens_1h`。
- `relay/common/relay_info.go`：补充缓存写入分档字段。
- 前端：
  - `web/src/features/usage-logs/lib/charge-breakdown.ts`：明细计算（复用 `getTieredBillingSummary`）。
  - `web/src/features/usage-logs/components/dialogs/charge-breakdown.tsx`：明细弹窗。
  - `details-dialog.tsx` 增加入口；`types.ts` 增加字段。
- i18n：7 语言各 +15 键。
- 测试：`service/text_quota_test.go`、`web/.../__tests__/usage-facts.test.tsx`。

## 配置与开关

无开关，随日志详情展示。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-10-25 | `3414e7482` | 初始引入：计费明细后端快照 + 前端弹窗 |

## 注意事项

- **改动前必读 `.agents/rules/billing.md` 全文**（本功能触及 `service/text_quota.go` 与 `relay/common/relay_info.go` 计费字段）。
- `BillingTokens`/`BillingPrices` 是**展示用快照**，不参与结算；不要把它接进结算链路。
