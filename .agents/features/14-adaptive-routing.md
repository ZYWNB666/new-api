# 14 · 按用户启用的自适应渠道路由（缓存优先）

- **状态**：活跃
- **引入提交**：`a9732a0b4`（基于 `fbb04e5dd` 的自适应路由实现）
- **影响面**：`pkg/adaptiverouting/`、`model/channel_cache.go`、`model/ability.go`、`model/user.go`、`service/channel_select.go`、`pkg/metrics/generation.go`、`relay/channel/metrics_adaptor.go`、`relay/common/relay_info.go`、`relaykit/dto/user_settings.go`、管理员用户编辑

## 功能说明

管理员可以为指定用户开启自适应路由，按同一模型不同渠道的近期成功率、首字延迟和生成速度动态分配流量。性能按“渠道 ID × 客户端请求模型”隔离统计，不要求将供应商的多个模型拆成多个渠道。

已有会话绑定优先复用原供应商，以保护已预热的提示词缓存。普通亲和只有在限流、持续失败或有足够观测证明性能显著下降时才允许切换；新会话继续按动态权重分流。

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
- `SelectChannelForRequest()` 为普通亲和比较既有选择器产生的合法候选，由 `Store.ShouldSwitchAffinity()` 决定是否切换；普通波动、未知或过期指标保持原绑定。严格亲和和显式渠道绑定保留，`adaptiveChannelBackoff()` 防止绑定绕过 429 退避。
- 性能切换要求原渠道和候选在对应指标上各有至少十个有效样本，原渠道最近三次首字延迟均达到候选平滑值的两倍，或生成速度均不超过候选的一半，且原渠道平滑指标也达到门槛。候选近期成功率至少为 90%，最近没有失败或退避，且另一项已知指标不能恶化两倍。
- 429 退避、连续至少三次失败或至少十个样本后平滑成功率低于 80% 可直接允许故障转移，无须等待性能样本；候选仍遵守既有权限、分组、优先级、过滤和退避规则。成功后沿用既有 `RecordChannelAffinity()` 更新绑定。
- 开关通过既有 `users.setting` TEXT 存储，无新增数据库列。`User.EditWithTx()` 在事务内更新策略；`UpdateUserSetting()` 保留管理员策略，防止自助设置覆盖。`buildSelfUserData()` 过滤该字段，管理员详情单独返回。
- 管理员界面复用 `UsersMutateDrawer`、既有表单和 `Switch`，文案覆盖七种语言。

## 配置与开关

- 管理后台：用户管理 → 编辑用户 → 自适应路由，默认关闭。
- 管理端用户更新字段：`adaptive_routing`；持久化字段：`UserSetting.AdaptiveRouting`。
- 无 TTFT/TPS 绝对目标配置；评分系数、探测比例及会话切换门槛由 `pkg/adaptiverouting/routing.go` 定义。本次不新增配置项。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-10-27 | `a9732a0b4` | 初始实现：管理员按用户开启，成功率/TTFT/TPS 相对评分、恢复探测与限流退避 |
| 2026-10-28 | `8aa774789` | 增补自适应绕过亲和、严格绑定限流等诊断，保留键来源与原绑定渠道 |
| 2026-10-29 | `b9e1b3d2c` | 优先保留已预热会话，仅在明显持续性能下降、限流或持续失败时允许切换 |

## 注意事项

- 统计仅在单个网关进程内共享，重启后重新采样；没有跨实例汇总，也不推断供应商跨模型、跨密钥的共享限额。
- 该能力不保证上游容量；全部供应商真实限流时仍可能返回 429。严格绑定仍可能牺牲动态分流效果。
- 保护缓存依赖会话渠道亲和，不保证供应商实际缓存命中。性能比较基于渠道与模型的共享观测，并非每个会话的实际缓存状态；候选仍由动态加权选择器产生，不保证发现退化后的下一次请求一定抽中可替换渠道。
- 缺少可信 TPS 数据时只使用已知指标，不能把未知吞吐量视为零速。
- 输出用量仅作为路由观测，不修改计费数量；涉及用量和计费链路的后续改动须遵守 `.agents/rules/billing.md`。
- 修改 `relaykit/` 后必须执行 `cd relaykit && GOWORK=off go build ./...`，保持模块独立。
- 验证覆盖管理员读写和用户侧隔离、模型/分组隔离、相对性能尺度不变性、HTTP/SDK/流内 429、软冷却兜底、普通/严格亲和及关闭开关时的既有行为。

- 本次已于 2026-10-27 18:15（Asia/Shanghai）发布到 `netwebs.top`，版本 `v1.0.0-rc.39-local.20261027181252.adaptive.18775e31`；supervisor、本机/公网健康、metrics、前端资源校验通过，未登录管理接口返回 401。
- 发布前执行后端相关包测试、竞态检查、20 项前端测试、类型检查、lint、前端构建及 relaykit 独立构建。用户设置持久化和数据库选路已通过 SQLite 3.50.4、MySQL 8.0.46、PostgreSQL 16.15 实例验证，没有新增数据库列。
- 完整 controller 测试中的 `TestResponsesWebSocketReusesConnectionAndSettlesEachRequest`、`TestResponsesWebSocketInitialUpstreamRejectionRefundsReservation` 在基线 `fbb04e5dd` 同样失败；发布回归显式跳过这两组，未改动其断言或相关业务行为。
- 管理权限与用户侧隔离参考 OWASP ASVS 5.0.0、Authentication / Session Management / Authorization Cheat Sheets；本次不宣称整体合规。

- `b9e1b3d2c` 验证：`go test ./service ./pkg/adaptiverouting -count=1` 通过；`go test -race ./service ./pkg/adaptiverouting -run 'TestAdaptiveRouting|TestAutomaticChannelAffinity|TestRateLimit' -count=1` 通过；`go build ./...` 与 `git diff --check` 通过。回归覆盖冷启动、同等健康、普通波动、一次/两次尖峰、延迟恢复、持续首字或吞吐退化、样本不足、未知候选、缺失吞吐、指标过期、模型隔离、避免以另一项严重退化换性能、故障切换、严格绑定、关闭自适应及成功后更新亲和。
- 本次仅调整进程内健康观测和渠道选择，不改数据库结构、查询实现、计费数量或前端资源；服务层测试沿用既有 SQLite 夹具，不将其表述为新的三数据库兼容验证。

- `b9e1b3d2c` 已于 2026-10-29 17:32（Asia/Shanghai）发布到 `netwebs.top`，版本 `v1.0.0-rc.39-local.20261029173102.cache-affinity.6749f801`。构建关联工作区提交 `43318e14d`；Go 1.26.1、`GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath`，通过 ldflags 写入上述版本。二进制 SHA-256 为 `6e021f58681edac8da1f8dcca216bd07eb697a9a85bcc96e98723be51d2f4d00`。
- 上线执行 `python3 /tmp/newapi-cache-affinity-prepare-release.py` 与 `python3 /tmp/newapi-cache-affinity-publish.py`：发布文件校验全部通过，supervisor 为 RUNNING，本机及公网 `/api/status` 版本一致，运行中二进制摘要与发布包一致，metrics 与前端资源校验通过；未登录 `/api/option/request_policy` 返回 401。启动脚本摘要保持原值。旧二进制备份为 `/opt/new-api/releases/v1.0.0-rc.39-local.20261029173102.cache-affinity.6749f801/new-api-before-upgrade`，SHA-256 为 `21106876c59b06ac6385c6c3370c012b8921ec38a4503b146b6852bd3d9c8aa7`，发布脚本带失败自动回滚。
