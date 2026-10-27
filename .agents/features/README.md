# 功能跟踪总目录（codex-fork 分支）

本目录记录 `codex-fork` 分支**相对上游 `main` 的全部自研功能**。每个功能一个文件，命名 `NN-<slug>.md`，`NN` 为两位序号（分配后不复用）。

## 维护规则

- **新增功能**：追加一个功能文件，并在本目录表格中新增一行。
- **修改功能**：更新对应功能文件的「变更记录」，必要时更新本目录的「状态」列。
- **删除功能**：把功能文件状态改为「已移除」，注明移除提交与原因，**不要删除文件**（保留历史可追溯）。
- 每次改动必须同步更新本目录，保证这里是「当前全部功能」的唯一权威清单。
- 功能判定以「上游 `main` 没有的行为」为准；上游已有的 bugfix/重构不算新功能。
- 详细的维护流程见 `.agents/skills/feature-tracker/SKILL.md`。

## 当前功能清单

| # | 功能 | 状态 | 功能文件 | 引入提交 |
|---|---|---|---|---|
| 01 | 品牌本土化与法律文档 | 活跃 | [01-branding-legal.md](01-branding-legal.md) | `539045224` `41bdb07d0` |
| 02 | 首页重构与 Jev 决策支持 | 活跃 | [02-homepage-jev.md](02-homepage-jev.md) | `70736a3d6` `bac315d77` `bb409e7f6` `5ae8aa8f6` `2bad79510` `ce6ade566` `46eecfa2c` |
| 03 | 部署与 CI（supervisor / Grafana / GHCR） | 活跃 | [03-deploy-ci.md](03-deploy-ci.md) | `3996b5642` `3817d7dae` `03d2ff99d` |
| 04 | 支付宝支付 | 活跃 | [04-alipay.md](04-alipay.md) | `9c727251b` `6288aec2f` |
| 05 | OAuth 代理支持 | 活跃 | [05-oauth-proxy.md](05-oauth-proxy.md) | `cf0ef5fd7` |
| 06 | 可观测性指标（Prometheus / Grafana） | 活跃 | [06-metrics.md](06-metrics.md) | `10483148d` `c52ce1ac7` |
| 07 | 网关限流与缓存命中率 | 活跃 | [07-gateway-limit.md](07-gateway-limit.md) | `4c9f881da` |
| 08 | TTFT 首包超时与 499 重试 | 活跃 | [08-ttft-timeout.md](08-ttft-timeout.md) | `4ef72dbb7` `e74fa8092` |
| 09 | 请求头日志（脱敏与截断） | 活跃 | [09-request-header-logging.md](09-request-header-logging.md) | `2b92efa58` |
| 10 | 使用日志计费明细 | 活跃 | [10-charge-breakdown.md](10-charge-breakdown.md) | `3414e7482` |
| 11 | 响应头品牌标识改写 | 活跃 | [11-response-header-branding.md](11-response-header-branding.md) | `b079e605f` |
| 12 | Prompt 前缀亲和键（automatic session） | 活跃 | [12-prompt-affinity.md](12-prompt-affinity.md) | `fbb04e5dd` |
| 13 | `prompt_cache_options` 字段支持 | 活跃 | [13-prompt-cache-options.md](13-prompt-cache-options.md) | `cf690561a` |
| 14 | 按用户启用的自适应渠道路由 | 活跃 | [14-adaptive-routing.md](14-adaptive-routing.md) | `a9732a0b4` |

## 上游基线

- 分叉点：`972aed197`（`fix(log): derive response model mismatch from names instead of a stored flag (#7464)`）
- 分支：`codex-fork`；与 `origin/codex-fork` 的同步状态以 `git status -sb` 为准
- 上游 `main` 的新提交若被合入，需重新核对上述功能是否已被上游吸收；若已吸收，把对应功能状态改为「已上游化」并注明合入提交。
