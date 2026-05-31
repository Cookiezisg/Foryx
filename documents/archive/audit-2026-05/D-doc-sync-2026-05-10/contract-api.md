# D2 — api-design.md ↔ code gap report

**Audited file**: `documents/version-1.2/service-contract-documents/api-design.md` (283 lines)
**Code source**: every `mux.HandleFunc(...)` registration in `backend/internal/transport/httpapi/handlers/` plus the `devRoutes` manifest at `handlers/dev_routes.go`.

Method:
- Listed every doc-claimed endpoint (~57 production rows).
- Listed every code-registered route (74 prod via mux.HandleFunc + ~17 /dev routes; the parameterized handlers like `{idAction}` fan out into multiple effective endpoints).
- Cross-referenced both directions.

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `POST /api/v1/forges/{id}:duplicate` | `forge.go:49` (via `postOnForge` switch) | **MED** — registered + listed in devRoutes manifest (line 75); api-design.md never lists it |
| `GET /api/v1/conversations/{id}` | `conversation.go:38` | **MED** — concrete Get endpoint exists; doc only lists Create/List/PATCH/DELETE under conversation (line 113-117) |
| `GET /api/v1/mcp-servers/{name}/stderr` | `mcp.go:78` | **MED** — debug-stderr endpoint exists; api-design.md mcp section (line 218-224) never lists it |
| `GET /api/v1/sandbox/runtimes` | `sandbox.go:68` | **HIGH** — entire sandbox HTTP surface (12 routes) absent from api-design.md |
| `GET /api/v1/sandbox/envs` | `sandbox.go:69` | HIGH (same as above) |
| `GET /api/v1/sandbox/envs/{id}` | `sandbox.go:70` | HIGH |
| `GET /api/v1/sandbox/disk-usage` | `sandbox.go:71` | HIGH |
| `GET /api/v1/sandbox/bootstrap-status` | `sandbox.go:72` | HIGH |
| `GET /api/v1/conversations/{id}/sandbox-envs` | `sandbox.go:73` | HIGH |
| `POST /api/v1/sandbox/envs/{id}:destroy` | `sandbox.go:78` (via `envAction`) | HIGH |
| `POST /api/v1/sandbox/runtimes/{id}:destroy` | `sandbox.go:79` (via `runtimeAction`) | HIGH |
| `POST /api/v1/sandbox/:gc` | `sandbox.go:80` (via `sandboxAction`) | HIGH |
| `POST /api/v1/sandbox/:retry-bootstrap` | `sandbox.go:80` (via `sandboxAction`) | HIGH |
| `POST /api/v1/sandbox/runtimes:install` | `sandbox.go:80` (via `sandboxAction` — namespaced) | HIGH |
| `POST /api/v1/conversations/{id}/sandbox-envs/{kind}:reset` | `sandbox.go:81` (via `convEnvKindAction`) | HIGH |
| `POST /api/v1/conversations/{id}/sandbox-envs:reset-all` | `sandbox.go:82` (via `convEnvsAction`) | HIGH |
| `POST /api/v1/skills/{name}:invoke` | `skills.go:94` (via `NameAction`) | LOW (devRoutes manifest line 104 has it; api-design.md skill table lists it line 249 ✅; reconfirm — it's actually present, mark NOT a gap) |

**Note on skill `:invoke`**: re-checked api-design.md line 249 — it's there. Removing from gap list.

**Sandbox absence**: api-design.md stops at `Phase 4 准备件` and lists subagent / mcp / skill / catalog. **Sandbox is fully missing** — neither under Phase 4 nor anywhere else. Code has 12 routes registered (sandbox.go:68-82) and the devRoutes manifest (lines 128-138) acknowledges them.

---

## In doc but not in code (stale)

| Item | Doc location | Severity |
|---|---|---|
| `GET /api/v1/events?conversationId=xxx` (legacy SSE) marked "✅ Phase 4 cutover 删" | api-design.md:81 | **HIGH** — legacy `/api/v1/events` endpoint was removed when domain/events was deleted (per `chat.go:4-8` "legacy /api/v1/events 端点随 domain/events 一起删了") and `eventlog.go:3` "ChatHandler.EventsSSE（/api/v1/events）共存" is itself a stale comment. Doc still lists it as ✅ |
| `GET /api/v1/conversations/{id}/subagent-runs` | api-design.md:206 | **HIGH** — route NOT registered. router.go:62-68 explicitly says "SubagentService no longer registers HTTP routes — sub-run data lives in the unified messages/message_blocks tables". |
| `GET /api/v1/subagent-runs/{id}` | api-design.md:207 | HIGH (same) |
| `GET /api/v1/subagent-runs/{id}/messages` | api-design.md:208 | HIGH (same) |
| `GET /api/v1/subagent-types` | api-design.md:209; **also in devRoutes manifest line 125** | **HIGH** — neither subagent.go handler file nor any HandleFunc(`/api/v1/subagent-types`...) call exists. devRoutes claims the route but it'll 404. |

---

## Mismatched (different details)

| Item | Code | Doc | Severity |
|---|---|---|---|
| `:duplicate` route on forge | `forge.go:49` registers via switch case `"duplicate"` returning 201 | api-design.md never lists `:duplicate` | MED (gap) |
| `GET /api/v1/conversations/{id}` Get endpoint | `conversation.go:38` registered, returns 200 with conversation | api-design.md (line 113-117) lists only POST/GET/PATCH/DELETE — no Get-by-id row | MED |
| Stale code comment `eventlog.go:3` claims "/api/v1/events" exists alongside | code reality: removed | doc reality: claims still exists | LOW (stale comments — out of scope but related to MED above) |
| api-design.md line 99 `GET /api/v1/providers` | code: exists at `providers.go:35`; lists ProviderMeta with `?category=llm/search` filter | doc claims `?category=llm 或 ?category=search`. Code: confirmed query params `category` accepted | OK (no gap) |
| Sandbox route shape: `POST /api/v1/sandbox/{action}` accepts 3 actions | code: `:gc` / `:retry-bootstrap` / `runtimes:install` | doc absent entirely | LOW — stylistic (single mux entry exposing 3 logical endpoints; testend manifest lists them as 3 rows) |
| `POST /api/v1/conversations/{id}/sandbox-envs/{kind}:reset` uses `_` separator internally | code (`sandbox.go:281` per snippets) joins owner.ID as `convID + "_" + kind` not `:` | doc absent | LOW — internal detail not normally surfaced |

---

## Sub-check

- **Total in code (HandleFunc + sub-action fan-out)**: ~74 production endpoints (counting each `case` branch in switch handlers, not bare `mux.HandleFunc` count of 60). Plus 17 `/dev` routes (3 of which are `--dev` only).
- **Total in doc (production)**: ~57 endpoint rows (excluding ⬜ placeholders).
- **Aligned**: ~52 production endpoints have both rows.
- **Net gap**:
  - 12 sandbox routes missing from doc (HIGH)
  - 4 subagent routes in doc but not in code (HIGH stale)
  - 1 `/api/v1/events` legacy listed in doc but not in code (HIGH stale)
  - 3 misc gaps (`:duplicate`, `/{id}` Get, `/stderr`) — MED
  - devRoutes manifest internally inconsistent: `subagent-types` row references nonexistent handler

---

## Severity rollup

- **HIGH**: 4 categories
  1. Sandbox 12 routes entirely missing from doc (one big gap; counts as 1 category but 12 line items)
  2. `/api/v1/events` legacy claimed ✅ but removed
  3. 3 `/api/v1/subagent-runs*` endpoints documented but not registered
  4. `/api/v1/subagent-types` documented + in devRoutes but no handler exists (will 404 if user calls it)
- **MED**: 3
  1. `POST /api/v1/forges/{id}:duplicate` not in api-design.md (registered)
  2. `GET /api/v1/conversations/{id}` not in api-design.md (registered)
  3. `GET /api/v1/mcp-servers/{name}/stderr` not in api-design.md (registered)
- **LOW**: minor stylistic / internal detail items

---

## Reasoning

The api-design.md file has fallen behind two recent restructurings:

1. **2026-04 schema unification**: subagent dedicated tables collapsed into `messages` rows. Subagent HTTP routes (`/api/v1/conversations/{id}/subagent-runs`, `/api/v1/subagent-runs/{id}`, `/api/v1/subagent-runs/{id}/messages`) were ripped out of the router; only `/api/v1/subagent-types` stayed in the devRoutes manifest (but no actual handler — bug to validate). Doc lists all 4 routes still as ✅.

2. **Phase 4 sandbox D2 work**: 12 sandbox routes wired up in `sandbox.go`. devRoutes manifest was updated, error-codes.md mostly tracks (sandbox sentinels documented), but api-design.md was not extended at all.

3. **`/api/v1/events` legacy endpoint**: when domain/events package was deleted, the route registration went too. api-design.md still claims it's a valid legacy endpoint. The eventlog.go file even has a stale comment claiming it co-exists ("ChatHandler.EventsSSE（/api/v1/events）共存") — out of scope for a doc audit but worth flagging the inconsistency between code comment and code reality.

4. **`/api/v1/subagent-types` ghost route**: This is the most insidious — the devRoutes manifest (testend Routes tab) advertises this endpoint to users, but no `mux.HandleFunc("GET /api/v1/subagent-types"...)` exists anywhere. Users calling it will get a generic 404 from the fallback handler. Either (a) implement the handler, (b) remove the devRoutes entry, or (c) admit it's been removed in api-design.md.
