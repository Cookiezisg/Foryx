# D4 — events-design.md ↔ code gap report

**Audited file**: `documents/version-1.2/service-contract-documents/events-design.md` (207 lines)
**Code source**:
- `domain/eventlog/eventlog.go` — 5 protocol events + 6 block types
- `domain/notifications/notifications.go` — generic envelope
- `pkg/notifications/notifications.go` — Publisher API + ctx wiring
- `infra/eventlog/bridge.go` / `infra/notifications/bridge.go` — Bridge implementations
- `transport/httpapi/handlers/eventlog.go` / `notifications.go` — HTTP / SSE endpoints

Method:
- Listed every event type / block type / Producer / SSE endpoint claimed in doc.
- Cross-checked code definitions and active publishers.

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `eventlog.ConversationUpdated` event type | `domain/eventlog/eventlog.go:234-242` (`EventType()` returns `"conversation_updated"`) | LOW (defined but no producers — see Mismatched) |
| `eventlog.TodoUpdated` event type | `domain/eventlog/eventlog.go:259-267` (`EventType()` returns `"todo_updated"`) | LOW (defined but no producers) |
| **Entire `domain/notifications` SSE protocol** + `GET /api/v1/notifications` endpoint | `domain/notifications/notifications.go`; `infra/notifications/bridge.go`; `transport/httpapi/handlers/notifications.go:61` | **HIGH** — events-design.md is titled "SSE 事件契约" but covers ONLY the eventlog protocol (5 events × 6 block types). The whole notifications protocol (1 generic envelope, 6 entity types live, global broadcast SSE) is absent. CLAUDE.md §E1 (top-level project guide) describes "双 SSE 协议" — this contract doc disagrees with the project guide by ignoring half of it. |
| Notification entity types in production: `conversation`, `todo`, `mcp_server`, `skill`, `catalog`, `sandbox_env` | producers at `app/conversation/conversation.go:69,117,128`; `app/todo/todo.go:249`; `app/mcp/mcp.go:326,379`; `app/skill/scan.go:106`; `app/catalog/polling.go:253`; `app/sandbox/sandbox.go:661,682` | **HIGH** — events-design.md never lists these notification types. CLAUDE.md §E1 itself only mentions 2 (`conversation` / `todo`) but real code has 6. |
| Notification Publisher API (`pkg/notifications.With/From/MustFrom`) | `pkg/notifications/notifications.go` | LOW — no doc coverage; CLAUDE.md §E1 mentions "Bridge pattern" but not the wrapper. |

---

## In doc but not in code (stale / unimplemented)

| Item | Doc location | Severity |
|---|---|---|
| `GET /api/v1/events?conversationId=xxx` (legacy SSE — claim still co-existing through Phase 4) | events-design.md:10 | **HIGH** — `/api/v1/events` endpoint was removed when `domain/events` was deleted. Code comment `chat.go:4-8` literally says "legacy /api/v1/events 端点随 domain/events 一起删了". Doc still tells callers it's there. |
| `domain/events/` (6 entity-snapshot events: `chat.message` / `forge` / `conversation` / `todo` / `mcp` / `skill`) | events-design.md:11, 180 | **HIGH** — `domain/events` directory does not exist (`ls` confirmed). All 6 legacy snapshot events were either dropped (chat.message) or migrated to `domain/notifications` (conversation/todo) or never existed in legacy form (forge/mcp/skill — they always lived elsewhere or are notifications-only). |
| §11 "Legacy events 共存（Phase 1-3 dual-write）" claim | events-design.md:178-184 | **HIGH** — entirely stale. `chat.go:4-8` confirms legacy endpoint is gone. The "dual-write" period is over. |
| §11 producer claim "chat 主管线：legacy chat.message **+** 新 5 events 都推" | events-design.md:182 | **HIGH** — chat producers only push the new 5 events; legacy chat.message is gone. |
| §11 producer claim "subagent: legacy chat.message (借 SubagentRun 壳)" | events-design.md:183 | HIGH — same reason; subagent only pushes new protocol. |
| §11 producer claim "forge / catalog / mcp / skill / todo / conversation：仍只推 legacy（**未接入新协议**）" | events-design.md:184 | **HIGH** — these domains push to `domain/notifications` (the global broadcast protocol), not the deleted `domain/events`. Doc is exactly wrong about which protocol they use. |
| §12 Phase Roadmap "Phase 4 ⬜ 等 V1.2 后端期结束" with milestones "前端 chat.js 切到新 bridge + 删 legacy events + drop subagent_runs/messages 表" | events-design.md:194 | **HIGH** — legacy events already deleted; subagent_runs/messages tables already deleted (per database-design.md note line 148 "schema 统一" and `dev_routes.go:121` "old /subagent-runs endpoints were retired"). Phase 4 milestones already done. |
| §13 Test reference "`domain/eventlog/eventlog_test.go` — ValidateEvent 各事件形状" | events-design.md:201 | **MED** — file does NOT exist. `find backend/internal/domain/eventlog -name '*_test*'` returns nothing. |
| §13 Test reference "`infra/store/chat/block_v2_test.go` (12 测) — BlockV2Store CRUD / CHECK / UNIQUE" | events-design.md:204 | **MED** — file does NOT exist. `find ... -name "block_v2*"` returns nothing. |
| §13 Test references for `infra/eventlog/bridge_test.go (10 测)` and `pkg/eventlog/eventlog_test.go (15 测)` | events-design.md:202-203 | LOW (files exist; test counts may have drifted — out of scope to verify) |

---

## Mismatched (different details)

| Item | Code | Doc | Severity |
|---|---|---|---|
| `ConversationUpdated` / `TodoUpdated` event types defined in `domain/eventlog` | code defines them but **no producer publishes them**; the actual conversation / todo updates publish via `domain/notifications` (generic envelope, type strings `"conversation"` / `"todo"`) | doc events-design.md doesn't list them at all (§4 schema only has 5 events) | **MED** — code has dead types in `domain/eventlog/eventlog.go:234-267`. Either: (a) wire producers to publish them through eventlog Bridge — but that contradicts the global-broadcast intent of CLAUDE.md §E1 and they really belong in notifications; (b) delete the dead types. Right now they're code-debt that pretends to be live API. |
| `domain/eventlog` event count — code defines **7** event types | code: `MessageStart` / `MessageStop` / `BlockStart` / `BlockDelta` / `BlockStop` / `ConversationUpdated` / `TodoUpdated` (= 7 with `EventType()` methods) | doc says "5 events × 6 block types" everywhere | MED — same root cause as above; the 5 vs 7 mismatch is from dead code. |
| §1 event count line "5 种事件 + 6 种 block 类型" | matches `IsValidBlockType` (6 values) and `IsValidStatus` (4 values) — these match | doc | OK on block / status |
| §3 Status enum 4 values | code: `streaming`/`completed`/`error`/`cancelled` (eventlog.go:96-99) | doc | OK |
| §4 BlockStart `id` shape note: `blk_<16hex>` or LLM-given `tc_<id>` | code: only constraint is `IsValidBlockType` + non-empty ID; no prefix enforcement | doc says §S15 prefix rule (line 75) | OK (doc-correct) |
| §6 Routing claim "客户端按 conversationId 订阅一条 SSE" | code: `eventlog.go:77` `GET /api/v1/eventlog` accepts `?conversationId=` query | doc | OK |
| §7 Subagent nesting example | code: subagent spawn does write `parent_block_id` chain matching doc | doc | OK |
| §8 Producer table | code: producers exist at the listed sites (chat/runner.go for processTask, etc.) | doc | OK overall — but the line "tool_call → tool_result child" isn't quite right because tool_result is sibling not child. Verified at runtime conventions. |
| §9 DB write table — `message_blocks` columns | code: `domain/chat/chat.go:110-123` matches column list including UNIQUE(conv_id, seq) | doc | OK |
| §10 Invariants (§S21) | code: enforced via DB UNIQUE + IsValidStatus + ValidateEvent at Bridge.Publish | doc | OK |

---

## Sub-check

- **Total events defined in code** (concrete `EventType()` impls):
  - `domain/eventlog`: 7 (MessageStart, MessageStop, BlockStart, BlockDelta, BlockStop, ConversationUpdated, TodoUpdated)
  - `domain/notifications`: 1 generic Event (Type discriminator)
- **Total events documented**: 5 (eventlog only); notifications protocol entirely absent.
- **Total notification entity types in production**: 6 (`conversation` / `todo` / `mcp_server` / `skill` / `catalog` / `sandbox_env`)
- **Total notification entity types documented in any contract doc**: 0 (CLAUDE.md §E1 mentions 2)
- **Aligned**: 5 (the eventlog protocol body)
- **Gaps**:
  - HIGH (8 stale claims about legacy events / Phase 4 roadmap)
  - HIGH (entire notifications protocol missing)
  - MED (2 dead event types in code; 2 missing test files)
  - LOW (notifications Publisher API undoc'd)

---

## Severity rollup

- **HIGH**: 2 categories
  1. **Entire notifications protocol absent**: CLAUDE.md §E1 mandates "双 SSE 协议" (eventlog + notifications); events-design.md only documents one. 6 entity types push notifications in production with zero documentation. Anyone reading events-design.md to plan Phase 4 frontend work will miss half the picture.
  2. **§11 + §12 stale legacy events claims** (8 sub-items): doc claims `domain/events` + `/api/v1/events` co-exist, dual-write is in progress, Phase 4 hasn't done the cutover. Reality: cutover already happened (commits per `git log` "domain/events deleted", "subagent-runs endpoints retired"). Doc telling readers "still in dual-write" is actively misleading them.
- **MED**: 3
  1. Dead `ConversationUpdated` / `TodoUpdated` types in `domain/eventlog` with no producers — should be deleted from code OR wired up.
  2. `domain/eventlog/eventlog_test.go` referenced in §13 doesn't exist
  3. `infra/store/chat/block_v2_test.go` referenced in §13 doesn't exist
- **LOW**: 1
  - Notification Publisher API (`pkg/notifications.With/From/MustFrom` ctx pattern) is undocumented.

---

## Reasoning

This file is the most stale of the four contract docs by far. It documents protocol surfaces that no longer exist (legacy events / Phase 4 dual-write narrative) while completely missing the entire `notifications` SSE protocol that's been live for several phases. The eventlog protocol body (sections 1-10) is mostly correct — but sections 11-13 (§Legacy / §Phase Roadmap / §Tests) are essentially fiction.

Two structural fixes the doc needs:

1. **Delete §11-§12** entirely (legacy is gone, roadmap items are done) and replace with a "current state" snapshot.
2. **Add a §1.5 or §2.5 covering the notifications protocol**: 1 envelope shape, 6 active entity types (`conversation`, `todo`, `mcp_server`, `skill`, `catalog`, `sandbox_env`), `GET /api/v1/notifications` endpoint, Publisher API (`pkg/notifications`).

Two code-side cleanups would resolve the MED items:

1. Delete `ConversationUpdated` / `TodoUpdated` from `domain/eventlog/eventlog.go` (lines 234-267) — they have no producers and their data flows correctly through `domain/notifications` instead.
2. Fix or remove the `block_v2_test.go` and `domain/eventlog/eventlog_test.go` references — they were planned but never written. (Note: `infra/store/chat` may have other test files that cover the BlockV2 contract; just not the one named in the doc.)
