# D-redo audit — database-design.md

Audited 2026-05-11. End-to-end read of `documents/version-1.2/service-contract-documents/database-design.md` against:
- `backend/internal/domain/*/<name>.go` (entity structs + GORM tags)
- `backend/internal/infra/db/schema_extras.go` + `db.go` + `migrate.go`
- `backend/internal/infra/store/*/` (verified field round-trip + index usage)
- `backend/cmd/server/main.go` migration list (13 tables)

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `messages.parent_block_id TEXT` column (with index) | `backend/internal/domain/chat/chat.go:54` | MED |
| `messages.attrs TEXT` column (JSON metadata) | `backend/internal/domain/chat/chat.go:62` | MED |
| `forge_test_cases.user_id` column | `backend/internal/domain/forge/forge.go:165` | LOW |
| `api_keys.display_name` / `base_url` / `api_format` / `test_status` / `test_error` / `last_tested_at` columns | `backend/internal/domain/apikey/apikey.go:28-35` | LOW (doc points to design doc for full list, but key feature columns like `test_status` not even mentioned in passing) |

The `messages.parent_block_id` + `attrs` gap is the load-bearing one: doc's "Phase 4 准备件" subagent section describes the runtime semantics ("一次 spawn 是 messages 表里的一行，attrs.kind=subagent_run + parent_block_id 指向…") but the `messages` schema section itself does NOT list these as columns. New readers scanning the `messages` row enumeration will think the columns don't exist.

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| `sandbox_envs` FK `runtime_id → sandbox_runtimes.id` claimed but no FK declared anywhere | line 158 ("FK `runtime_id → sandbox_runtimes.id`") | MED |
| `forge_executions` "无主动 eviction (TODO 300 条/forge 上限策略待实现)" — trimming IS implemented | line 117 | MED |
| Global D4 "外键显式声明" — no entity declares any `foreignKey` GORM tag anywhere | line 9 (D4 statement) | LOW (PRAGMA is on but there's nothing for it to enforce) |

Note on the FK gap: `infra/store/sandbox/sandbox.go:128-133` comment also asserts "FK; SQLite enforces with PRAGMA foreign_keys=ON" — that comment is consistent with the doc but also not borne out by the actual GORM tags. The entity has only `index` on `RuntimeID`, no `foreignKey:RuntimeID;references:ID`.

The `forge_executions` trimming gap: `app/forge/forge.go:1121-1132` in `recordExecution` does `CountExecutions` + `DeleteOldestExecution` when `n > MaxExecutionsPerForge`. Active retention, not TODO.

## Semantic drift

| Table.column | Doc says | Code does | Severity |
|---|---|---|---|
| `model_configs` index | "GORM 全唯一索引 `UNIQUE(user_id, scenario)`（partial UNIQUE 暂缓）" — correct | Entity GORM tag is `uniqueIndex:idx_mc_user_scenario` (full UNIQUE) — correct; but `domain/model/model.go:22-24` **godoc** misleadingly says "partial UNIQUE index in schema_extras.go" while no such partial exists. Code godoc lies; contract doc tells the truth. | LOW |
| `forge_executions.created_at` index annotation | "`idx_fe_forge_created` 复合 priority:2，`idx_fe_msg` 复合 with conversation_id+message_id" (read literally: created_at participates in idx_fe_msg too) | Entity puts `created_at` in `idx_fe_forge_created` only; `idx_fe_msg` is `(conversation_id, message_id)`, no `created_at`. Doc's later "复合索引 2 个" paragraph is correct; only the per-column annotation is misleadingly worded. | LOW |
| `api_keys` index list | "全索引 `(user_id)` + `(user_id, provider)` + `(deleted_at)`" — all three named as if equally needed | Entity defines exactly these three, but `idx_api_keys_user_id` (single col) is redundant with leading column of composite `idx_api_keys_user_provider`. Doc could note the redundancy or drop one; both code and doc currently keep both. Not a contract violation, just a footnote opportunity. | LOW |

## Sub-check

- Tables aligned: **yes** — 13 tables in migration call match 13 tables in doc (api_keys / model_configs / conversations / messages / message_blocks / attachments / forges / forge_versions / forge_test_cases / forge_executions / sandbox_runtimes / sandbox_envs / todos). No orphan tables in either direction.
- Columns aligned: **mostly** — only the two `messages` columns (`parent_block_id`, `attrs`) are real but undeclared in the doc's schema enumeration. The doc mentions them runtime-semantically in the subagent section but not in the `messages` row breakdown.
- Indexes aligned (GORM tag vs schema_extras): **yes** — only `forges (user_id, name) WHERE deleted_at IS NULL` lives in schema_extras (correct per D7); all other indexes are GORM tags, including the composites `idx_api_keys_user_provider`, `idx_mc_user_scenario`, `idx_blocks_conv_seq`, `idx_fe_forge_created`, `idx_fe_msg`, `idx_td_conv_status`, `idx_se_owner`, `uniq_sr_kind_version`, `uniq_se_owner`. D7 rule respected throughout.
- CHECK / UNIQUE constraints aligned: **yes** — `message_blocks.type` (6 values) + `message_blocks.status` (4 values) declared as GORM `check:` and match the doc; `sandbox_envs.owner_kind` + `sandbox_envs.status` declared as GORM `check:` and match the doc; `model_configs` / `apikey` Provider / TestStatus / Scenario all uncheckedd in DB per doc statement (whitelist deferred to app layer). UNIQUE constraints (kind, version) for sandbox_runtimes and (owner_kind, owner_id) for sandbox_envs match.
- Soft-delete patterns aligned: **yes** — every domain entity that doc claims soft-delete for has `DeletedAt gorm.DeletedAt` with `gorm:"index"`: api_keys, model_configs, conversations, messages, attachments, forges, todos. Tables doc explicitly excludes from soft-delete (`message_blocks`, `forge_versions`, `forge_test_cases`, `forge_executions`, `sandbox_runtimes`, `sandbox_envs`) have no `DeletedAt` field. Pattern fully consistent.

## Cross-cutting findings

Compared to the D1 grep-based audit, the end-to-end read uncovered three patterns:

1. **The doc-schema enumeration vs. doc-design-prose split is leaking.** For `messages`, the column list explicitly enumerates each column, but two real columns (`parent_block_id`, `attrs`) only appear in the "Phase 4 准备件 / subagent" prose three sections later. A reader who treats the schema section as authoritative will miss two real columns. Same pattern is what made `display_name` / `base_url` etc. invisible in the apikey section. Recommendation (for the next doc-sync pass): the schema-section enumeration should be definitive — if a column exists, list it there.

2. **The "FK explicitly declared" D4 standard is not what code actually does.** Zero entities have a `foreignKey` GORM tag anywhere in the tree. The PRAGMA `foreign_keys=on` is set in `db.go:92` and verified at startup, but there's nothing for it to enforce — every cross-table reference is "soft" (string column with an index, no constraint). The doc's D4 statement "外键显式声明 + PRAGMA foreign_keys=ON" reads like both halves are real; only the second half is. The sandbox FK claim (`runtime_id → sandbox_runtimes.id`) is the most prominent example — both the schema enumeration AND the store comment assert it, but the entity has only `index`. Either drop the D4 "显式声明" half or add `foreignKey:RuntimeID;references:ID` to the sandbox Env entity (and any others that should be enforced).

3. **`forge_executions` retention drift is a fix-not-applied case.** Doc still says "TODO: 300 条/forge 上限策略待实现 (当前由用户手动 GC)". `app/forge.Service.recordExecution` already implements the trim via `CountExecutions + DeleteOldestExecution` against `MaxExecutionsPerForge=300`. Doc needs to flip from TODO to "active retention; trim runs after every recordExecution". This is the kind of staleness that happens when implementation lands without a `[doc]` dev-log pass — exactly what §S14 is meant to prevent.

Sub-check totals: **6 gaps (0 HIGH / 3 MED / 3 LOW)** plus the 3 cross-cutting findings above.
