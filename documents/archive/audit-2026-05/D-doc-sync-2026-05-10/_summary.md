# D — Contract Documents Sync Audit Summary (2026-05-10)

Phase D fork of the doc-sync audit. Audited the **4 contract documents** in `documents/version-1.2/service-contract-documents/` against actual backend code. **No code or doc edits in this phase** — only gap reports.

---

## Reports

| File | Audited doc | Code surface | Result |
|---|---|---|---|
| [`contract-error-codes.md`](contract-error-codes.md) | `error-codes.md` (287 lines) | every `var Err...` + `errmap.go::errTable` (65 rows) | 0 HIGH / 2 MED / 11 LOW |
| [`contract-api.md`](contract-api.md) | `api-design.md` (283 lines) | every `mux.HandleFunc(...)` + `devRoutes` manifest | 4 HIGH categories / 3 MED / minor LOW |
| [`contract-database.md`](contract-database.md) | `database-design.md` (228 lines) | every domain GORM struct + `schema_extras.go` | 1 HIGH / 3 MED / 5 LOW |
| [`contract-events.md`](contract-events.md) | `events-design.md` (207 lines) | `domain/eventlog` + `domain/notifications` + producers | 2 HIGH categories / 3 MED / 1 LOW |

---

## Top-line totals

- **HIGH**: 7 distinct issues (some have multiple sub-items, e.g. 12 sandbox routes count as 1 HIGH category)
- **MED**: 11
- **LOW**: ~20

---

## HIGH severity rollup (must-fix to align doc with reality)

| # | Category | File | Effect |
|---|---|---|---|
| 1 | All 12 sandbox HTTP routes missing from api-design.md | api-design.md | Backend has full sandbox surface (12 endpoints); doc has zero. Anyone planning frontend or testend work is blind to it. |
| 2 | `/api/v1/events` legacy endpoint listed as ✅ but removed | api-design.md:81 | Misleads readers — endpoint doesn't exist; calling it gets 404. |
| 3 | 3 `/api/v1/conversations/{id}/subagent-runs*` endpoints listed but not registered | api-design.md:206-208 | Schema-unification commit removed routes; doc still claims them. |
| 4 | `/api/v1/subagent-types` listed in api-design.md AND `devRoutes` manifest but no handler | api-design.md:209 + `dev_routes.go:125` | testend Routes tab advertises a 404. |
| 5 | `api_keys` composite `(user_id, provider)` index — code has only `priority:2` on Provider, missing `priority:1` on UserID | code bug + database-design.md:54 | Real index = single-col on Provider; doc says composite. Code-bug + doc-mirror. |
| 6 | Entire `domain/notifications` SSE protocol + `/api/v1/notifications` endpoint absent from events-design.md | events-design.md | 6 entity types (`conversation`/`todo`/`mcp_server`/`skill`/`catalog`/`sandbox_env`) push notifications in prod; doc is silent. CLAUDE.md §E1 mandates "双 SSE" — events-design covers half. |
| 7 | events-design.md §11-§12 (Legacy events / Phase 4 dual-write) entirely stale | events-design.md:178-194 | Legacy `domain/events` deleted; Phase 4 milestones already complete; doc tells readers dual-write is ongoing. |

---

## MED severity rollup

| # | Issue | File |
|---|---|---|
| 1 | `error-codes.md:197` `SUBAGENT_RUN_NOT_FOUND` row references deleted endpoint | error-codes.md |
| 2 | `error-codes.md` `LLM_PROVIDER_ERROR` wire code shared by two sentinels (`chat.ErrProviderUnavailable` + `llminfra.ErrProviderError`) — doc lists only one | error-codes.md:105, errmap.go:64+210 |
| 3 | `POST /forges/{id}:duplicate` registered but absent from api-design.md | api-design.md, forge.go:49 |
| 4 | `GET /api/v1/conversations/{id}` Get-by-id endpoint absent from api-design.md | api-design.md, conversation.go:38 |
| 5 | `GET /api/v1/mcp-servers/{name}/stderr` registered but absent from api-design.md | api-design.md, mcp.go:78 |
| 6 | `sandbox_envs.running_pid` + `running_started_at` columns absent from database-design.md | database-design.md:158, sandbox.go:104-105 |
| 7 | `forge_executions` "300 条/forge" eviction claim has no implementation | database-design.md:117 |
| 8 | Dead `ConversationUpdated` / `TodoUpdated` types in `domain/eventlog` — defined but no producers | events-design.md (mismatch), eventlog.go:234-267 |
| 9 | `domain/eventlog/eventlog_test.go` referenced in §13 doesn't exist | events-design.md:201 |
| 10 | `infra/store/chat/block_v2_test.go` referenced in §13 doesn't exist | events-design.md:204 |
| 11 | `forge.ErrEnvFailed` description vs `ErrDependencyResolution` overlap — scoping unclear | error-codes.md:143 |

---

## LOW severity rollup (~20 items spread across 4 reports)

- 8 newly-added sentinels not documented in error-codes.md (`llminfra.Err*` × 5, `webtool.Err*` × 3)
- `sandboxdomain.ErrInvalidOwnerID` / `ErrCmdRequired` not documented
- Sandbox table header status `📐` despite live entries (database-design.md)
- `sandbox_envs` DB CHECK constraints not disclosed
- `schema_extras` global section in database-design.md still says "tools" instead of "forges"
- Notification Publisher API (`pkg/notifications`) undocumented
- Misc description-drift items

---

## Pattern observations

1. **api-design.md and events-design.md are the most stale**. Both lag behind the schema-unification work (subagent_runs/messages tables → unified `messages` rows) and the Phase 5 sandbox D2 work.

2. **error-codes.md is mostly current** — Phase A (errmap audit) work apparently kept this doc in step. The 12 minor LOW items are 8 new sentinels + sandbox D2 sentinels that didn't make the doc round-trip.

3. **database-design.md is the closest-to-aligned**, with 1 actual code-bug (apikey composite index) and a few description drifts.

4. **`devRoutes` manifest as a third source of truth**: `handlers/dev_routes.go` is intended to mirror real registrations; it caught the `subagent-types` ghost route but contradicts itself (lists a path that no `mux.HandleFunc` registers). If kept, it's worth adding a CI check that compares it against actual registrations.

5. **CLAUDE.md §E1 vs events-design.md inconsistency**: project guide says "双 SSE 协议"; this contract doc only covers one. Either the contract doc needs to be expanded, or §E1's framing should clarify that events-design.md is scoped to the eventlog protocol only and notifications has a separate doc (which doesn't exist yet).

---

## Out of scope (flagged but not actioned)

- The `api_keys` composite index code-bug (HIGH #5) is a code-side fix, not just a doc fix.
- The `subagent-types` ghost route (HIGH #4) needs either a handler or removal from devRoutes — also code-side.
- Stale code comments in `handlers/eventlog.go:3` claiming `/api/v1/events` co-exists are misleading (code reality: removed) — this audit only listed it as context, not a primary finding.
