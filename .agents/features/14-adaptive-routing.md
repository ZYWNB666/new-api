# 14 · 按用户启用的自适应渠道路由

- **状态**：活跃
- **引入提交**：`a9732a0b4`（基于 `fbb04e5dd` 的自适应路由实现）
- **影响面**：`pkg/adaptiverouting/`、`model/channel_cache.go`、`model/ability.go`、`model/user.go`、`service/channel_select.go`、`pkg/metrics/generation.go`、`relay/channel/metrics_adaptor.go`、`relay/common/relay_info.go`、`relaykit/dto/user_settings.go`、管理员用户编辑

## 功能说明

管理员可以为指定用户开启自适应路由，按同一模型不同渠道的近期成功率、首字延迟和生成速度动态分配流量。性能按“渠道 ID × 客户端请求模型”隔离统计，不要求将供应商的多个模型拆成多个渠道。

评分直接比较符合权限、请求约束和当前优先级的候选，不配置绝对 TTFT/TPS 目标。普通用户看不到该设置；未开启的用户继续使用原有渠道选择方式。

## 实现要点

- `adaptiverouting.Store.Record()` 记录平滑成功率、有效内容首字延迟及可信输出 token 对应的生成时长；成功率和性能观测使用 EWMA。统计在当前进程内跨用户共享，普通样本十分钟过期，最多保留 10000 个组合。
- `Store.Weights()` 从至少有十个近期样本的可用候选中找出最低 TTFT 和最高 TPS，分别计算 `min(1, bestTTFT / TTFT)`、`min(1, TPS / bestTPS)`。性能系数为两项分数的加权几何平均（0.6 / 0.4），最终评分使用性能系数平方与平滑成功率立方乘以基础权重；性能影响随前十个样本逐步增加。
- 有效流量权重由 95% 评分分配和 5% 基础权重探测组成，不改写数据库中的渠道权重。冷启动或缺失指标保持中性，过期样本不参与性能比较。
- 上游 429 使用有效 `Retry-After`，缺失或无效时采用 5/10/20/40/60 秒退避；全部候选真实限流时返回 429 和等待时间。连续失败的软冷却在有正常替代时避开，全部处于软冷却时保留兜底，不因单纯性能差清空候选。
- `metrics.ProviderAttempt` 复用有效生成内容的到达时间；Prometheus 未启用时仍采集路由观测。`MeteredAdaptor` 识别 HTTP、SDK 及 HTTP 200 内的协议失败；取消和业务拒绝不计入渠道失败。
- TPS 只使用可对应的已报告输出用量和内容时间间隔，不把 SSE chunk 当 token。估算、缺失或已知混合推理/多模态的用量不参与 TPS。
- `model.GetRandomSatisfiedChannel()` 和 `GetChannel()` 在原候选优先级内应用动态权重，覆盖内存缓存与数据库选路。自动分组可以跳过限流分组，尝试下一个有权使用的分组。
- 日志以 `binding_status = adaptive_bypass` 标明普通亲和让位于自适应比较，同时保留原 `cached_channel_id` 和内容指纹；不将缓存命中误报为实际采用绑定。
- `SelectChannelForRequest()` 让开启功能的普通亲和请求重新比较候选；严格亲和和显式渠道绑定保留。`adaptiveChannelBackoff()` 对绑定渠道执行限流等待，避免绑定绕过 429 退避。
- 开关通过既有 `users.setting` TEXT 存储，无新增数据库列。`User.EditWithTx()` 在事务内更新策略；`UpdateUserSetting()` 保留管理员策略，防止自助设置覆盖。`buildSelfUserData()` 过滤该字段，管理员详情单独返回。
- 管理员界面复用 `UsersMutateDrawer`、既有表单和 `Switch`，文案覆盖七种语言。

## 配置与开关

- 管理后台：用户管理 → 编辑用户 → 自适应路由，默认关闭。
- 管理端用户更新字段：`adaptive_routing`；持久化字段：`UserSetting.AdaptiveRouting`。
- 无 TTFT/TPS 绝对目标配置；评分系数及探测比例由 `pkg/adaptiverouting/routing.go` 定义。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-10-27 | `a9732a0b4` | 初始实现：管理员按用户开启，成功率/TTFT/TPS 相对评分、恢复探测与限流退避 |
| 2026-10-28 | `8aa774789` | 增补自适应绕过亲和、严格绑定限流等诊断，保留键来源与原绑定渠道 |

## 注意事项

- 统计仅在单个网关进程内共享，重启后重新采样；没有跨实例汇总，也不推断供应商跨模型、跨密钥的共享限额。
- 该能力不保证上游容量；全部供应商真实限流时仍可能返回 429。严格绑定仍可能牺牲动态分流效果。
- 缺少可信 TPS 数据时只使用已知指标，不能把未知吞吐量视为零速。
- 输出用量仅作为路由观测，不修改计费数量；涉及用量和计费链路的后续改动须遵守 `.agents/rules/billing.md`。
- 修改 `relaykit/` 后必须执行 `cd relaykit && GOWORK=off go build ./...`，保持模块独立。
- 验证覆盖管理员读写和用户侧隔离、模型/分组隔离、相对性能尺度不变性、HTTP/SDK/流内 429、软冷却兜底、普通/严格亲和及关闭开关时的既有行为。

- 本次已于 2026-10-27 18:15（Asia/Shanghai）发布到 `netwebs.top`，版本 `v1.0.0-rc.39-local.20261027181252.adaptive.18775e31`；supervisor、本机/公网健康、metrics、前端资源校验通过，未登录管理接口返回 401。
- 发布前执行后端相关包测试、竞态检查、20 项前端测试、类型检查、lint、前端构建及 relaykit 独立构建。用户设置持久化和数据库选路已通过 SQLite 3.50.4、MySQL 8.0.46、PostgreSQL 16.15 实例验证，没有新增数据库列。
- 完整 controller 测试中的 `TestResponsesWebSocketReusesConnectionAndSettlesEachRequest`、`TestResponsesWebSocketInitialUpstreamRejectionRefundsReservation` 在基线 `fbb04e5dd` 同样失败；发布回归显式跳过这两组，未改动其断言或相关业务行为。
- 管理权限与用户侧隔离参考 OWASP ASVS 5.0.0、Authentication / Session Management / Authorization Cheat Sheets；本次不宣称整体合规。
