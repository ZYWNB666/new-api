# 01 · 品牌本土化与法律文档

- **状态**：活跃
- **引入提交**：`539045224`（品牌与版权）、`41bdb07d0`（法律文档）
- **影响面**：前端全局、法律路由

## 功能说明

把系统身份从上游默认品牌改为「维流 Token 平台」及公司版权信息，并新增隐私政策与用户服务协议两个法律文档页面。

## 实现要点

- 系统名称与公司版权信息集中在 `web/src/lib/constants.ts`。
- `web/public/favicon.ico`、`web/rsbuild.config.ts` 指向 `logo-flow-ai.png`（见 `1e4bc7589`）。
- 法律文档正文：`docs/legal/privacy-policy.zh-CN.md`、`docs/legal/user-agreement.zh-CN.md`。
- 前端路由 `privacy-policy` / `user-agreement`，位于 `web/src/features/legal/`。
- `web/src/lib/content-format.ts` 增加文档内容的格式检测与测试。

## 配置与开关

无独立开关，随前端构建生效。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-09-23 | `539045224` | 初始引入：系统名称、版权、样式 |
| 2026-09-23 | `41bdb07d0` | 新增法律文档与内容格式检测 |
| 2026-09-26 | `1e4bc7589` | favicon 切换到 logo-flow-ai.png |

## 注意事项

- 项目治理规则（`AGENTS.md`）保护 `new-api` / `QuantumNous` 标识，本功能只改面向终端用户的品牌展示，不涉及上游归属信息。
