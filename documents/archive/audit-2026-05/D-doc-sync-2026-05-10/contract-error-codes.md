# D1 — error-codes.md ↔ code gap report

**Audited file**: `documents/version-1.2/service-contract-documents/error-codes.md` (287 lines)
**Code source**: `backend/internal/transport/httpapi/response/errmap.go::errTable` + every `var Err... = errors.New(...)` in `internal/`.

Method:
- Compared every doc-listed sentinel row against (a) sentinel definition file and (b) errmap.go entry.
- Cross-checked errmap.go has all sentinels claimed in doc.
- Listed sentinels code defines but doc omits (LOW unless they reach handler).

---

## In code but not in doc

| Item | Code location | Severity |
|---|---|---|
| `llminfra.ErrAuthFailed` (LLM_AUTH_FAILED 401) | `backend/internal/infra/llm/llm.go:29`; errmap row `errmap.go:206` | LOW (errmap registered, doc never lists wire code) |
| `llminfra.ErrRateLimited` (LLM_RATE_LIMITED 429) | `backend/internal/infra/llm/llm.go:30`; errmap row `errmap.go:207` | LOW |
| `llminfra.ErrBadRequest` (LLM_BAD_REQUEST 400) | `backend/internal/infra/llm/llm.go:31`; errmap row `errmap.go:208` | LOW |
| `llminfra.ErrModelNotFound` (LLM_MODEL_NOT_FOUND 404) | `backend/internal/infra/llm/llm.go:32`; errmap row `errmap.go:209` | LOW |
| `llminfra.ErrProviderError` (LLM_PROVIDER_ERROR 502) | `backend/internal/infra/llm/llm.go:33`; errmap row `errmap.go:210` | LOW (note: same wire code `LLM_PROVIDER_ERROR` already on doc for `chat.ErrProviderUnavailable` 502 — collision risk) |
| `webtool.ErrAuthFailed` (WEBSEARCH_AUTH_FAILED 401) | `backend/internal/app/tool/web/search.go:63`; errmap row `errmap.go:221` | LOW (Phase A new sentinel — explicit task target) |
| `webtool.ErrRateLimited` (WEBSEARCH_RATE_LIMITED 429) | `backend/internal/app/tool/web/search.go:64`; errmap row `errmap.go:222` | LOW |
| `webtool.ErrUpstreamHTTP` (WEBSEARCH_UPSTREAM_HTTP 502) | `backend/internal/app/tool/web/search.go:65`; errmap row `errmap.go:223` | LOW |
| `sandboxdomain.ErrInvalidOwnerID` (SANDBOX_INVALID_OWNER_ID 400) | `backend/internal/domain/sandbox/sandbox.go:191`; errmap row `errmap.go:111` | LOW (Phase A new — sandbox D2 tightening; doc table never updated) |
| `sandboxdomain.ErrCmdRequired` (SANDBOX_CMD_REQUIRED 400) | `backend/internal/domain/sandbox/sandbox.go:199`; errmap row `errmap.go:112` | LOW |
| `context.Canceled` (CLIENT_CLOSED 499) | stdlib; errmap row `errmap.go:237` | LOW (deliberately registered to suppress unmapped warning; doc could mention) |
| `context.DeadlineExceeded` (REQUEST_TIMEOUT 504) | stdlib; errmap row `errmap.go:238` | LOW |

**Notes**:
- The 5 `llminfra.Err*` + 3 `webtool.Err*` are post-Phase-A additions (commit `363b084 fix(llm)` and `7dba737 fix(tool/web)`). Doc was never extended.
- `sandboxdomain.ErrInvalidOwnerID` / `ErrCmdRequired` are listed nowhere in the doc, even under sandbox table; only `ErrDocker*` two were documented (lines 250-251).

---

## In doc but not in code (stale / unused)

| Item | Doc location | Severity |
|---|---|---|
| `tool.ErrImportConflict` (TOOL_IMPORT_CONFLICT 409) marked ⬜ | error-codes.md:140 | LOW (status ⬜ honest — sentinel doesn't exist yet; row is forward-looking placeholder) |
| `subagentdomain.ErrRunNotFound` (`SUBAGENT_RUN_NOT_FOUND` 404) row | error-codes.md:197 | **MED** — doc claims `gorm.ErrRecordNotFound` is mapped via "handler 内映射", but `/subagent-runs/{id}` route was removed (per `handlers/dev_routes.go:121-124` "old /subagent-runs endpoints were retired with schema unification — only list types remains"). Row references a dead endpoint. |
| `forgedomain.ErrImportConflict` (`TOOL_IMPORT_CONFLICT` 409) | error-codes.md:140 | LOW (planned ⬜) |
| `intent.ErrAmbiguous` (INTENT_AMBIGUOUS 422) | error-codes.md:287 | LOW (Phase 5 placeholder ⬜) |
| Workflow domain (`workflow.ErrNotFound` etc.) | error-codes.md:181-186 | LOW (planned Phase 4) |
| Knowledge / flowrun / scheduler placeholder rows | error-codes.md:255-265, 273-277 | LOW (placeholder ⬜) |

---

## Mismatched (different details)

| Item | Code | Doc | Severity |
|---|---|---|---|
| `LLM_PROVIDER_ERROR` wire code collision | errmap.go:64 maps `chatdomain.ErrProviderUnavailable` → 502 LLM_PROVIDER_ERROR; errmap.go:210 also maps `llminfra.ErrProviderError` → 502 LLM_PROVIDER_ERROR | error-codes.md:105 lists code only against `chat.ErrProviderUnavailable`; doesn't disclose the second sentinel sharing the wire code | **MED** — same wire code returned for two sentinels means clients can't disambiguate. Doc must either list both or note the alias. |
| Phase 4 sandbox table — header status `📐` (planned) but rows have `✅` for `ErrInvalidOwnerID` / `ErrCmdRequired` / `ErrDockerNotInstalled` / `ErrDockerDaemonDown` | errmap rows lines 111-112, 120-121 — all 4 registered live | error-codes.md:240-251 lists 8 ⏳ rows + 2 ✅ rows under "sandbox 📐" header | LOW — header `📐` (designed) is misleading: 4 sandbox sentinels are already wired into errmap and Phase A explicitly added 2 of them. |
| `ASK_TIMEOUT` wire description note "实际不到 handler" | doc says "保留登记便于将来若改语义" | code (`tool/ask/ask.go`) does convert timeout to friendly text — true | LOW — doc note correct; just confirming sync. |
| `forge.ErrEnvFailed` 422 description includes "Sync 失败（含 deps 解析失败 / Python 包冲突）" | But code has separate `forgedomain.ErrDependencyResolution` for that exact case | error-codes.md:143 description overlaps with `FORGE_DEPENDENCY_RESOLUTION` row at line 145 | LOW — description text needs scope clarification (deps-only failures should belong to `ErrDependencyResolution`, env env failures stay on `ErrEnvFailed`). |

---

## Sentinels code defines but never registered (handler-unreachable — OK by design)

These are sentinel definitions that **never reach handler / errmap**, so absence from doc is expected. Listing for completeness so reviewer can verify nothing slipped:

| Sentinel | File:line | Reason not in errmap |
|---|---|---|
| `infra/crypto.ErrNoFingerprint` | `fingerprint.go:24` | Internal — wraps boot-time machine ID failure; not a request error |
| `infra/mcp.ErrConfigCorrupt` | `infra/mcp/config.go:53` | Boot-time only; mcp.config.Load called once at startup |
| `app/mcp.ErrSearchServerUnavailable` | `app/mcp/searchrouter.go:52` | Internal use only by web tool fallback |
| `app/tool/web.ErrMCPSearchUnavailable` | `app/tool/web/search_mcp.go:23` | Tool-internal; converted to friendly text |
| `app/tool/skill.ErrEmptyName` / `.ErrEmptyQuery` | `tool/skill/{activate,search}.go` | Tool ValidateInput → tool_result text |
| `app/tool/ask.ErrEmptyQuestion` | `tool/ask/ask.go:54` | Tool ValidateInput |
| `app/tool/shell.ErrEmptyBashID` / `.ErrEmptyCommand` / `.ErrInvalidTimeout` / `.ErrProcessNotFound` | `tool/shell/*.go` | Tool ValidateInput / runtime |
| `app/tool/web.ErrEmptyQuery` / `.ErrEmptyURL` / `.ErrEmptyPrompt` / `.ErrUnsupportedScheme` | `tool/web/{fetch,search}.go` | Tool ValidateInput |
| `app/tool/filesystem.ErrEmptyOldString` / `.ErrEditNoOp` / `.ErrEmptyFilePath` / `.ErrPathNotAbsolute` / `.ErrNegativeOffset` / `.ErrNegativeLimit` | `tool/filesystem/{edit,read}.go` | Tool ValidateInput |
| `app/tool/mcp.ErrEmptyServer` / `.ErrEmptyTool` / `.ErrEmptyQuery` | `tool/mcp/{call,search}.go` | Tool ValidateInput |
| `app/tool/search.ErrEmptyPattern` / `.ErrInvalidOutputMode` | `tool/search/grep.go` | Tool ValidateInput |
| `app/tool/subagent.ErrEmptyPrompt` / `.ErrEmptyType` | `tool/subagent/agent.go` | Tool ValidateInput |
| `domain/eventlog.ErrSeqTooOld` / `.ErrInvalidEvent` | `domain/eventlog/eventlog.go:277,286` | SSE handler at `transport/httpapi/handlers/eventlog.go:107` does direct `errors.Is` + custom 410 response; doesn't go through FromDomainError |
| `domain/notifications.ErrSeqTooOld` / `.ErrInvalidEvent` | `domain/notifications/notifications.go:103,109` | Same SSE pattern — `handlers/notifications.go:77` |
| `domain/chat.ErrBlockNotFound` | `domain/chat/chat.go:229` | Internal store layer; `GetBlock` rarely called from handler path |
| `domain/todo.ErrConversationMismatch` | `domain/todo/todo.go:74` | Per doc note: collapsed into `ErrNotFound` to avoid existence leak (line 160) — never reaches handler as itself |
| `domain/forge.ErrConfigCorrupt` (none — `infra/mcp.ErrConfigCorrupt` only) | — | (alias) |
| `pkg/llmclient.ErrPickModel` / `.ErrResolveCreds` / `.ErrBuildClient` | `pkg/llmclient/llmclient.go:24-26` | Used in `app/chat/runner.go:112-114` to set `Message.errorCode` field, NOT to bubble to handler. **Note**: error-codes.md:115-117 documents the `Message.errorCode` values, but doesn't link these to the `pkg/llmclient.Err*` sentinels — see Mismatched table for related |
| `domain/catalog.ErrCoverageIncomplete` / `.ErrGenerationFailed` | `domain/catalog/catalog.go:124,132` | Absorbed by mechanical fallback per doc line 236 ✅ |
| `domain/subagent.ErrCancelled` / `.ErrMaxTurnsExceeded` | (don't exist as Go sentinels — see doc line 199 note "as `subagentapp.StatusMaxTurns` / `StatusCancelled` 字符串常量") | Confirmed doc-correct |

---

## Sub-check

- **Total registered errmap rows**: 65 (counting all entries in `errTable` map literal, including `errInvalid` / `errInternal`, but excluding stdlib ctx Canceled+DeadlineExceeded = 67 grand total)
- **Total sentinel definitions in code** (excluding pure tool ValidateInput variants): ~78
- **Total rows in error-codes.md** (excl. column headers, status legend, Phase 4/5 ⬜ placeholders): ~50 production rows
- **Aligned**: ~50 (every sentinel currently registered in errmap is also listed in error-codes.md, modulo the In-code-not-in-doc table above)
- **Gaps**: 12 in-code-but-not-in-doc rows + 1 stale/MED `SUBAGENT_RUN_NOT_FOUND` row + 1 MED collision (`LLM_PROVIDER_ERROR`) + minor description drift on `ErrEnvFailed`

---

## Severity rollup

- **HIGH**: 0
- **MED**: 2
  1. `SUBAGENT_RUN_NOT_FOUND` row pointing at deleted `/subagent-runs/{id}` endpoint (error-codes.md:197)
  2. `LLM_PROVIDER_ERROR` wire code shared by `chat.ErrProviderUnavailable` + `llminfra.ErrProviderError` — doc lists only one
- **LOW**: 11
  - 8 newly-added sentinels not documented (5× llminfra + 3× webtool)
  - 2 sandbox sentinels not documented (`ErrInvalidOwnerID`, `ErrCmdRequired`)
  - sandbox header status `📐` despite live entries
  - `ErrEnvFailed` vs `ErrDependencyResolution` description overlap
  - 2 stdlib ctx errors not mentioned as registered

---

## Reasoning

These gaps are pure documentation lag — every new sentinel went into errmap (otherwise audit B / C would have caught the unmapped warning) but didn't make the round trip to `error-codes.md`. The `SUBAGENT_RUN_NOT_FOUND` row is the only real bug because it claims an endpoint that doesn't exist anymore. The `LLM_PROVIDER_ERROR` collision is observable to clients and should be disclosed (or one of the two sentinels should switch to a distinct wire code).
