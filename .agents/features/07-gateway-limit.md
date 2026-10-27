# 07 · 网关限流与缓存命中率

- **状态**：活跃
- **引入提交**：`4c9f881da`
- **影响面**：`pkg/gatewaylimit`、`controller/gateway_limit.go`、`model/gateway_limit.go`、Grafana

## 功能说明

在渠道级/用户级限流之外，新增可缓存的**分层网关准入限流**：按 RPM / TPM / 并发三个维度，支持全局、用户、令牌、渠道等多种 scope，并统计缓存命中率。

## 实现要点

- `pkg/gatewaylimit/`：
  - `policy.go`：`Rule` / `Config` / `Policy` 定义；`OptionKey = "GatewayRateLimits"`，`PolicyTopic = "newapi:gateway-limits:policy"`；上限 `MaxRules=2000`、`MaxValue=1000000000`。
  - `manager.go`：规则缓存与策略刷新（Redis 发布订阅同步多节点）。
  - `limit.lua`：Redis Lua 脚本实现原子计数（RPM/TPM/并发）。
- `controller/gateway_limit.go`：`GetGatewayLimits` / `UpdateGatewayLimits` / `GetGatewayLimitUsage` / `GetGatewayLimitTargets` 四个 API。targets 接口只返回元数据，API 凭证不外泄。
- `model/gateway_limit.go`：`SaveGatewayLimits`（事务 + `lockForUpdate` 乐观锁 revision 比对，冲突返回 `ErrGatewayLimitConflict`）、`applyGatewayLimits`（编译并 `gatewaylimit.Publish`）、`ListenGatewayLimitPolicies`（Redis 订阅广播）。
- `model/option.go`：`UpdateOption` 与 `updateOptionMap` 中按 `gatewaylimit.OptionKey` 分流到上述两个函数——**这就是选项 key 的注册路径**，没有独立的 setting 包注册。
- 路由：`router/api-router.go` 的 `optionRoute`（`RootAuth` 保护）注册 `/api/option/gateway_limits`、`/gateway_limits/usage`、`/gateway_limits/targets`。
- 准入链路：`relay/common/gateway_limit.go` 的 `AcquireRequestLimit` / `AcquireProviderLimit` / `ReportGatewayUsage` / `FinishGatewayLimits` / `MarkGatewaySent`；中间件 `GatewayLimitCleanup` 保证响应后（含同步任务）释放 lease。
- 重路由：`service/gateway_limit.go` 的 `RerouteGatewayLimit`——provider 维度被拒时跳过该供应商池，不消耗上游重试，且不覆盖渠道 pin 与 strict 会话亲和。
- 接入点：`controller/relay.go`（文本）、`controller/task.go`（异步任务）、`main.go`（启动监听）。
- `dto/channel_constraints.go`、`model/channel_constraint.go`：`FilterGatewayCapacity` 过滤 + 渠道集合限定。
- 缓存命中率统计写入 `pkg/metrics/generation.go` 与 Grafana 看板。
- 前端：`web/src/features/rate-limits/`（`api.ts` + `index.tsx` + `rule-dialog.tsx` + `target-picker.tsx`），路由 `/_authenticated/rate-limits/`，侧边栏入口 `/rate-limits`。
- 测试：`controller/token_test.go`（约 190 行）、`pkg/gatewaylimit/manager_test.go`、`web/src/features/rate-limits/__tests__/management.test.tsx`。

## 配置与开关

- **选项 key**：`GatewayRateLimits`（定义在 `pkg/gatewaylimit/policy.go` 的 `OptionKey`）。
- **管理后台路径**：`/rate-limits` 页面（Root 权限），对应 API `/api/option/gateway_limits`。
- **规则字段**：`id`、`name`、`scope`（`user`/`api_key`/`user_model`/`provider`）、`subject_id`、`model`、`channel_ids`、`rpm`、`tpm`、`concurrent`、`enabled`。
- **存储后端**：Redis 启用时为 `redis` 模式（多节点广播同步），否则 `memory`；`GetGatewayLimits` 返回 `backend` 与 `legacy_enabled`（`setting.ModelRequestRateLimitEnabled`）。
- **持久化**：存在 `Option` 表，与普通选项共用 `loadOptionsFromDatabase` 刷新路径作为恢复手段。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-10-23 | `4c9f881da` | 初始引入：限流规则、Lua 计数、缓存命中率、Grafana 看板 |
| 2026-10-27 | — | 补全文档：选项 key 注册路径、准入链路、重路由逻辑、持久化与约束说明 |

## 注意事项

- 限流是准入控制，拒绝路径必须早于计费预扣，避免先扣费再拒绝。
- Lua 脚本改动需保证多节点时钟不一致时仍不超发。
- `SaveGatewayLimits` 用 `Option` 行锁 + `revision` 乐观比对，冲突返回 409；**已存在规则的 `scope`/`subject_id`/`model`/`channel_ids` 不可变更**，只能新建规则（这是有意的防绕过设计，不要为了"方便修改"去掉）。
- 规则目标（user/api_key/channel）会在事务内校验存在性，删除对象前需先清理对应规则。
- Redis 广播只是加速传播，**恢复路径是数据库 option 刷新**；改广播协议时要保证旧节点仍能靠刷新收敛。
- `RerouteGatewayLimit` 刻意不覆盖渠道 pin 与 strict 亲和，改重试逻辑时不要破坏这个优先级。
- 涉及三数据库：`Option` 行锁必须走 `lockForUpdate(tx)`，改动须在 SQLite/MySQL/PostgreSQL 三库验证。
