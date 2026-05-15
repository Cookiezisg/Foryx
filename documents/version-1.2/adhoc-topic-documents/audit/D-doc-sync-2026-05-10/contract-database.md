# D3 — database-design.md ↔ code gap report

**Audited file**: `documents/version-1.2/service-contract-documents/database-design.md` (228 lines)
**Code source**: `backend/internal/domain/<name>/*.go` GORM struct tags + `infra/db/schema_extras.go`.

Method:
- Listed every doc table (12 active + 6 planned ⬜).
- Verified every field/index claim against actual GORM tag.
- Cross-checked schema_extras for partial-index claims.

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `sandbox_envs.running_pid INT default 0; index` | `domain/sandbox/sandbox.go:104` | **MED** — manifest column for Layer-B leak prevention (Bootstrap kills surviving processes). Not in doc field list (line 158). |
| `sandbox_envs.running_started_at DATETIME` | `domain/sandbox/sandbox.go:105` | **MED** — partner column to `running_pid`. Not in doc. |
| `sandbox_envs.status` DB CHECK constraint `('installing','ready','failed')` | `domain/sandbox/sandbox.go:89` | LOW — doc says "Status (installing/ready/failed)" string, but doesn't disclose **DB-level CHECK** (vs todo/model_configs which doc explicitly notes have **no** CHECK). |
| `sandbox_envs.owner_kind` DB CHECK constraint `('forge','mcp','skill','conversation')` | `domain/sandbox/sandbox.go:81` | LOW — doc lists 4 kinds but doesn't note CHECK is at DB layer. |
| `sandbox_envs.status` default `'ready'` | `domain/sandbox/sandbox.go:89` | LOW — doc reads "status (installing/ready/failed)" (line 158); doesn't say default. |
| `message_blocks.type` / `.status` GORM tag-declared CHECK constraints | `domain/chat/chat.go:116,119` | LOW — doc DOES say "CHECK(type IN (...))" (line 75) so OK; just confirming. |
| Sandbox `EnvSpec` has `Extras` field documented inline; doc does cover (line 158) | OK | — |

**Note**: doc lines 134-141 cover `todos` table — verified all columns + composite index `idx_td_conv_status` + soft-delete + status whitelist app-layer; all match code at `domain/todo/todo.go:20-33`.

---

## In doc but not in code (stale / unimplemented)

| Item | Doc location | Severity |
|---|---|---|
| `forge_executions` claim "保留最近 300 条/forge（合并上限，原 100+200）" | database-design.md:117 | **MED** — no eviction / trimming logic exists in `app/forge/` or `infra/store/forge/`. `grep -rn "DELETE FROM forge_executions\|TrimExecutions"` returns nothing. Doc claims a feature that's not implemented. Either implement trim or drop the claim. |
| `api_keys` doc claims "全索引 `(user_id)` + `(user_id, provider)` + `(deleted_at)`" | database-design.md:54 | **HIGH** — code at `domain/apikey/apikey.go:26-27` has `index:idx_api_keys_user_id` (single col on UserID) and `index:idx_api_keys_user_provider,priority:2` (single col on Provider, **missing `priority:1` for UserID**). The composite `(user_id, provider)` is **NOT created** because GORM needs both fields tagged with the same index name. Doc describes intended schema; code accidentally drops UserID from composite. Either fix code or doc. |
| `forge_test_cases` claim "`forge_id` 索引" | database-design.md:113 | LOW — code at `domain/forge/forge.go:163-171` has `forge_id` field tagged `index`; matches doc. |
| `forge_test_cases.user_id` field exists | code line 165 | LOW — doc field list (line 113) doesn't mention UserID — but it's a service-internal scoping field not customer-facing. |
| Sandbox `_runtimes` / `_envs` tables marked `📐` (planned) | database-design.md:150, 156 | LOW — code already has full GORM models + DB CHECK + indexes wired (`domain/sandbox/sandbox.go:50-108`). Status `📐` ("designed") was true at doc creation but tables are now live. |
| `sandbox_runtimes.kind` doc claims values include "browsers" / "static" | database-design.md:153 | LOW — code defines no `Kind` constants (free-form text); doc lists candidate values informally. Match OK. |

---

## Mismatched (different details)

| Item | Code | Doc | Severity |
|---|---|---|---|
| `api_keys` composite `(user_id, provider)` index | code creates only `idx_api_keys_user_provider` indexed on Provider alone (priority:1 absent) — bug | doc claims composite exists | **HIGH** — code-bug + doc-correct (doc shows desired state). Either fix index tag or drop the claim. |
| `messages.role` enum values | code: doc says `(user\|assistant)` and "tool 角色已移除" — code domain/chat/chat.go:77-79 only defines `RoleUser` / `RoleAssistant` constants | doc: "(user\|assistant，**tool 角色已移除**——tool result 变为 message_blocks 的 block)" line 65 | OK (matches) |
| `messages.status` 5-value enum | code: defines `StatusPending/Streaming/Completed/Error/Cancelled` (chat.go:81-87) | doc says "(pending\|streaming\|completed\|error\|cancelled)" | OK |
| `forge_executions` retention claim "300 条/forge" | code: no enforcement | doc: 300/forge | MED (above) |
| `messages` doc says "FTS5 已移除" | code schema_extras has comment confirming removal (`schema_extras.go:33-38`) | doc | OK |
| `message_blocks.seq` claim "per-conversation 全局单调" | code: GORM tag `uniqueIndex:idx_blocks_conv_seq,priority:1/2` enforces UNIQUE(conv_id, seq) | doc | OK |
| `message_blocks.parent_block_id` claim "嵌套指针" | code: indexed (chat.go:114) | doc | OK |
| `attachments` claim renamed from `chat_attachments` | code: `func (Attachment) TableName() string { return "attachments" }` (chat.go:196) | doc line 84 | OK |
| `attachments` soft-delete | code: `DeletedAt gorm.DeletedAt` (chat.go:193) | doc | OK |
| `forge_versions.env_id` indexed | code: `gorm:"...;index"` (forge.go:131) | doc says "env_id TEXT 索引" (line 105) | OK |
| `forge_versions.env_status` DB CHECK | code: doc says "5 值：pending/syncing/ready/failed/evicted，白名单 service 层校验" / 5-value app-layer; code at forge.go:140 has `default:'pending'` no CHECK | doc | OK (both say no CHECK) |
| `conversation.system_prompt` field | code: `SystemPrompt string` (conversation.go:25) | doc says "系统提示词" line 62 | OK |
| `conversation.auto_titled` field | code: `AutoTitled bool` (conversation.go:24) | doc | OK |
| Messages `error_code` / `error_message` fields | code: `ErrorCode/ErrorMessage` strings (chat.go:58-59) | doc | OK |
| `forges.active_version_id` field | code: `ActiveVersionID string` (forge.go:43) | doc | OK |
| `forges` partial unique index `WHERE deleted_at IS NULL` | code: `schema_extras.go:47-52` `CREATE UNIQUE INDEX idx_forges_user_name_active ON forges(user_id, name) WHERE deleted_at IS NULL` | doc | OK |
| schema_extras has only ONE entry (forges) | code: confirmed `schema_extras.go:39-54` | doc says "部分 UNIQUE 索引（`WHERE deleted_at IS NULL`，例如 tools `UNIQUE(user_id, name)`）" — but uses old "tools" name | LOW — doc still says "tools" instead of "forges" in the global section (line 40). Phase 1 rename leftover. |

---

## Sub-check

- **Total tables in code (active)**: 12 — `api_keys`, `model_configs`, `conversations`, `messages`, `message_blocks`, `attachments`, `forges`, `forge_versions`, `forge_test_cases`, `forge_executions`, `todos`, `sandbox_runtimes`, `sandbox_envs` (= 13 actually, + `todos` for Phase 5)
- **Total tables in doc (active ✅)**: 12, plus 2 `📐` for sandbox.
- **Aligned**: ~all tables present in doc match a code struct.
- **Gaps**: HIGH (1: api_keys composite index code-bug), MED (3: sandbox running_pid/running_started_at not documented + forge_executions retention claim unimplemented), LOW (5: minor description drift, status header lag).

---

## Severity rollup

- **HIGH**: 1
  - `api_keys` composite `(user_id, provider)` index: code accidentally drops UserID's `priority:1` tag, so the index that exists in production is just on Provider — not what doc describes. Either bug fix in code or doc retraction.
- **MED**: 3
  - `sandbox_envs.running_pid` + `running_started_at` columns absent from doc (Layer-B leak prevention — important for understanding boot semantics)
  - `forge_executions` "300 conditions/forge" eviction claim has no implementation
- **LOW**: 5
  - Sandbox tables marked `📐` despite being implemented + wired
  - `sandbox_envs` DB CHECK constraints not disclosed in doc
  - Default value for `sandbox_envs.status` not in doc
  - Doc still says "tools" in schema_extras global section (line 40) — should be "forges"
  - `forge_test_cases.user_id` field invisible in doc field list

---

## Reasoning

This file is the closest-to-aligned of the four contracts. Schema is mostly accurate. Two real bugs:

1. **`api_keys` composite index**: doc says `(user_id, provider)` exists. Code's GORM tag has `priority:2` on Provider but no `priority:1` on UserID — so GORM emits an index on Provider alone. This is a regression that's invisible from EXPLAIN traces unless someone benchmarks `WHERE user_id=? AND provider=?` queries. Fix is one tag edit; flagging as HIGH because doc accurately describes intended schema while code silently drops it.

2. **`forge_executions` 300/forge eviction**: pure doc claim with zero supporting code. Either implement (TRIGGER on INSERT or async cleanup goroutine) or strike the line.

Sandbox tables are fully wired but marked `📐` (designed-but-not-built). Status icons need to flip to `✅`. `running_pid` / `running_started_at` are non-trivial columns that participate in boot-time process kill — they should be disclosed in the table's column list.

Several minor description-drift items (the global "tools" reference in extras section is from before Phase 1 rename — pure leftover) — would clean up in passing.
