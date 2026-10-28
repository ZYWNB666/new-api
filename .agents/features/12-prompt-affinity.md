# 12 · Prompt 前缀亲和键（automatic session）

- **状态**：活跃
- **引入提交**：`fbb04e5dd`
- **影响面**：`service/channel_affinity_auto.go`、`service/channel_affinity.go`、`README.zh_CN.md`、前端请求策略、使用日志渠道提示框

## 功能说明

在没有显式 session ID 时，从请求内容自动生成一个**稳定的会话 ID**，用它保持供应商亲和——同一段对话的后续轮次会优先复用上次成功的渠道，提高上游缓存命中率。

上游 `main` 只有按显式 key（如 `prompt_cache_key`）的亲和；本功能新增内容指纹兜底。

## 实现要点

- `service/channel_affinity_auto.go`：
  - `const automaticAffinityRuleName = "automatic session"`。
  - `channelAffinityRules()` 把该内置规则**追加到用户配置的规则列表末尾**，因此手工规则优先。
  - 规则定义：`ModelRegex: [".+"]`、`PathRegex` 覆盖 chat/completions、completions、responses、messages 与 Gemini generateContent、`KeySources: [{Type:"automatic"}]`、`SessionMode: "inherit"`、`TTLSeconds: max(3600, DefaultTTLSeconds)`、`IncludeUsingGroup/IncludeModelName/IncludeRuleName: true`。
  - `automaticChannelAffinityKey()` 提取优先级：
    1. 请求头 `X-Session-Id`、`X-Conversation-Id`、`Session-Id`、`Session_id`、`Thread-Id`、`Thread_id`
    2. Body `prompt_cache_key`、`metadata.session_id`、`metadata.conversation_id`、`session_id`、`conversation.id`、`conversation`
    3. 都没有时，对 system/developer 提示词 + `tools` + 第一条用户消息生成 SHA-256 路由指纹
  - 指纹输入为规范化 JSON（`["automatic-affinity-v1", userID, modelName, usingGroup, identity]`），对象键顺序不影响结果；多模态首条消息的图片等内容也参与。
- 隔离维度：用户、请求模型、分组相互隔离。
- 隐私：原始提示词与会话标识**不写入**自动亲和缓存键或日志；日志只保留 `key_fp` 指纹。
- 不生成绑定的情形：无首条用户内容且无显式标识的请求、独立任务、WebSocket。
- 日志字段：`channel_affinity.rule_name = "automatic session"`、内容指纹使用 `key_source = "prompt_hash"`；显式头部或 Body 标识仍使用 `key_source = "automatic"`；`key_fp`（`affinityFingerprint()` 对路由键计算 SHA-1 后取前 8 位，仅用于日志关联）。
- `recordChannelAffinityDecision()` 在亲和键生成后记录来源与指纹，不要求实际采用绑定；新增 `cache_hit`、`cached_channel_id`、`session_mode`、`adaptive_routing`、`binding_status`，分别表示缓存查询、原绑定渠道和最终绑定决策。关闭亲和或缓存查询失败时不伪报缓存未命中。
- `SelectChannelForRequest()` 记录自适应优先、渠道不可用、严格绑定限流和重试重新选路；实际最终渠道继续由日志渠道和重试链表示，命中缓存不等于采用绑定。
- 管理员日志提示框复用既有 Tooltip 和 `policyLabel()`，显示键来源、会话模式、缓存命中、绑定渠道、绑定决策及自适应开关；隐藏敏感信息时同时遮盖指纹和绑定渠道。
- 日志渠道提示框：仅当 `key_source = "prompt_hash"` 且存在 `key_fp` 时，在规则名后显示 `(PromptHash: 指纹)`；隐藏敏感数据时遮盖指纹，不展示原始提示词。
- 前端：`web/src/features/system-settings/request-policies/routing-section.tsx` 展示说明；i18n 7 语言同步。
- 测试：`service/channel_affinity_template_test.go` 与 `web/src/features/usage-logs/components/__tests__/channel-affinity.test.tsx`。
- 历史日志未区分内容指纹和显式会话来源，不猜测或补标 PromptHash。

## 配置与开关

- 全局「会话与重试 → 会话亲和」的 `SessionMode` 设为 `off` 可整体关闭（内置规则为 `inherit`，跟随全局）。
- 也可加一条优先匹配的规则，例如 `ModelRegex: ["^GLM"]` + `SessionMode: "off"`，只对特定模型关闭。
- 缓存 TTL 至少 1 小时。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-10-25 | `fbb04e5dd` | 初始引入：prompt 前缀生成亲和键 |
| 2026-10-27 | `a9732a0b4` | 区分内容指纹与显式会话来源，在日志规则后显示 PromptHash 指纹 |
| 2026-10-28 | `8aa774789` | 独立记录亲和键生成、缓存查询和绑定决策，自适应绕过绑定或缓存未命中仍可展示 PromptHash |

## 注意事项

- 默认情况下，相同 prompt 会优先复用同一渠道；开启功能 14 的用户在普通亲和模式下重新比较候选，严格绑定保留。
- 指纹是路由用途，**不是** provider 缓存参数，不要把它当 `prompt_cache_key` 上报上游。
- 后续追加轮次不改变指纹（只取第一条用户消息）。

- 新诊断字段仅保存在既有 `admin_info.channel_affinity` 中，不写原始提示词或会话 ID，不增加表或列；历史日志无法补回未保存的键来源与指纹。本次没有改变选路、重试和计费行为。

- `8aa774789` 已于 2026-10-28 17:28（Asia/Shanghai）发布到 `netwebs.top`，版本 `v1.0.0-rc.39-local.20261028172715.affinity-log.505c0637`；supervisor、本机与公网健康、metrics、前端静态资源校验通过，未登录管理接口仍返回 401。
- 验证：`go test ./service ./pkg/adaptiverouting -count=1`、亲和与自适应相关竞态测试、根模块构建、七项日志页面测试、前端类型检查、涉及文件 lint/格式检查及生产构建通过。复用已有渠道 Tooltip 与 `policyLabel()`，没有新建通用交互组件。
