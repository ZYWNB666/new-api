---
name: feature-tracker
description: >-
  Maintain the feature tracking directory for this fork. Covers adding, updating,
  and removing feature files under `.agents/features/`, and keeping the master
  index in sync. Use for any task that adds, changes, or removes a fork-specific
  feature (behavior not present in upstream `main`), when a commit introduces or
  alters user-visible functionality, when the user asks to track/document a
  feature, or when reviewing whether a change needs a feature entry. Always load
  and follow this skill before writing or editing files in `.agents/features/`.
---

# Feature Tracker Workflow

## Mandatory Preflight

- Read this entire `SKILL.md` before any work in `.agents/features/`.
- Determine the fork baseline first: the fork point is upstream commit `972aed197`. Only behavior that upstream `main` does **not** have counts as a fork feature. Upstream bugfixes, refactors, dependency bumps, and CI chores are **not** features.
- Use `git log --oneline 972aed197..HEAD` and `git show --stat <sha>` to ground every claim. Do not describe a feature from memory or from the user's phrasing alone.

## Scope Checklist

Treat the task as covered by this skill when it involves:

- `.agents/features/` (any read or write)
- A new commit that adds, changes, or removes fork-specific behavior
- Deciding whether a change warrants a feature entry
- The user asking "what features do we have", "track this", "document this feature"

Do not skip this workflow because the change is "just a small tweak" — small tweaks still update an existing feature file's changelog.

## Directory Layout

```
.agents/features/
├── README.md              # master index: the authoritative list of ALL current features
├── 01-<slug>.md           # one file per feature
├── 02-<slug>.md
└── NN-<slug>.md
```

- `NN` is a two-digit sequence number. **Numbers are never reused**, even after a feature is removed.
- `<slug>` is a short kebab-case name (`prompt-affinity`, `gateway-limit`).

## Feature File Format

Every feature file MUST use these sections in this order:

```markdown
# NN · <功能名>

- **状态**：活跃 | 已移除 | 已上游化
- **引入提交**：`<short-sha>`（<一句话说明>）
- **影响面**：<主要目录/文件>

## 功能说明
<它做什么、解决什么问题、与上游默认行为的差异。2-5 句。>

## 实现要点
<关键文件、核心函数/类型名、数据流、算法。用短列表。>

## 配置与开关
<环境变量、选项 key、管理后台路径；无开关就写"无开关"。>

## 变更记录
| 日期 | 提交 | 变更 |
|---|---|---|
| YYYY-MM-DD | `<short-sha>` | <改了什么> |

## 注意事项
<坑、安全/计费/数据库约束、与其他功能的关联、已知缺口。>
```

### Writing Rules

- Write in Chinese (matching the repo's dominant commit and doc language), except identifiers, file paths, and code symbols which stay verbatim.
- Every claim must be traceable to a commit or to code you actually read. Cite short SHAs.
- `实现要点` names real symbols (`automaticChannelAffinityKey`, `awaitFirstChunk`) — not vague descriptions.
- `注意事项` is mandatory. If there is genuinely nothing, state the invariant that must hold when the feature is modified.
- Keep each file focused on one feature. If a commit spans two unrelated capabilities, split it into two feature files.

## Update Procedures

### Adding a feature

1. Run `git log --oneline 972aed197..HEAD` and confirm the commit is fork-specific and not upstream-absorbed.
2. Pick the next unused `NN`.
3. Create `.agents/features/NN-<slug>.md` using the format above.
4. Add a row to the table in `README.md`.
5. Verify: `NN` unique, slug matches filename, SHA exists (`git cat-file -t <sha>`).

### Updating a feature

1. Locate the file from the `README.md` table (match by slug or by the SHA in 变更记录).
2. Append a row to `变更记录` with the date and short SHA.
3. Update `实现要点` / `配置与开关` / `注意事项` if behavior changed. Remove stale statements rather than appending contradictions.
4. If the change is substantial, add a short paragraph describing the new behavior under `功能说明`.
5. If status changed (removed / upstreamed), update the `状态` line and the `README.md` status column.

### Removing a feature

1. Set `状态` to `已移除`.
2. Append a `变更记录` row with the removing SHA and the reason.
3. Add a `## 移除说明` section: what replaced it, and any migration notes.
4. **Do not delete the file.** History must stay auditable.
5. Keep the row in `README.md` with status `已移除`.

### Upstream absorption

If a feature lands in upstream `main`:

1. Set `状态` to `已上游化`.
2. Record the upstream merge/SHA in `变更记录`.
3. Note in `注意事项` whether the fork copy can now be dropped on the next rebase.

## README.md Master Index Rules

`README.md` is the single authoritative list of **all current features**. It MUST contain:

1. The maintenance rules (add / update / remove / upstream).
2. A table with columns: `#`, `功能`, `状态`, `功能文件`, `引入提交`.
3. The upstream baseline section (fork point SHA, branch name).
4. A row for **every** feature file in the directory, including removed ones.

After any feature-file change, re-read `README.md` and confirm the table matches the directory listing exactly:

```bash
ls .agents/features/*.md
```

Every file except `README.md` must have exactly one table row, and every row must point to an existing file.

## Verification Checklist

Before finishing:

- [ ] Every new/changed claim is backed by a commit SHA or code you read.
- [ ] `README.md` table row count equals feature file count.
- [ ] `NN` numbers are unique and never reused.
- [ ] 变更记录 is append-only and chronologically ordered.
- [ ] No secrets, tokens, or credentials copied into feature files.
- [ ] If the feature touches billing, `注意事项` references `.agents/rules/billing.md`.
- [ ] If the feature touches `relaykit/`, note the `GOWORK=off go build ./...` requirement.
