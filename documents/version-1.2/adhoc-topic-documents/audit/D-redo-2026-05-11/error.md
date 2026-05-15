# D-redo audit — error-codes.md (read-it-all)

**Scope**: doc `documents/version-1.2/service-contract-documents/error-codes.md` vs code
`backend/internal/transport/httpapi/response/errmap.go`, every `domain/<name>/<name>.go` sentinel,
`pkg/reqctx/`, `pkg/llmclient/`, `infra/crypto/`, `infra/llm/`, `app/tool/web/` (BYOK sentinels),
and `app/ask/` (handler-reachable). Excludes `_test.go` and forge subpackages.

**Date**: 2026-05-11. **Method**: end-to-end read of doc + errmap + every sentinel file; per
sentinel: (a) exists in code? (b) doc HTTP / wire match errmap? (c) errmap registration?
(d) at least one Go producer outside `_test.go` / declaration file / errmap.go?

**Summary**: 0 HIGH / 8 MED / 5 LOW gaps. errmap.go ↔ doc table align row-for-row on HTTP status,
wire code, and sentinel reference. Remaining issues split into (a) the SSE `SEQ_TOO_OLD` wire code
documented in three other contract files but missing from this doc's error-table, (b) eight
sentinels that are registered but produced nowhere in Go code, and (c) the doc's bare-alias
sentinel column drift vs §S13 import aliases.

---

## In code but not in doc

| Sentinel / wire | Code location | Severity |
|---|---|---|
| `eventlogdomain.ErrSeqTooOld` → wire `SEQ_TOO_OLD` 410 | `transport/httpapi/handlers/eventlog.go:107-113` (inline-translated via `responsehttpapi.Error`, **bypasses errmap by design**); sentinel at `domain/eventlog/eventlog.go:224` | LOW |
| `notificationsdomain.ErrSeqTooOld` → wire `SEQ_TOO_OLD` 410 | `transport/httpapi/handlers/notifications.go:77-82` (same pattern); sentinel at `domain/notifications/notifications.go:103` | LOW |

The `SEQ_TOO_OLD` wire code is part of the public SSE protocol (CLAUDE.md §N7, events-design.md
lines 118 / 228, api-design.md line 81, chat.md lines 754 / 972), but error-codes.md's tables
contain no row for it. Doc line 7 claims to be "全仓所有错误码、HTTP 状态、sentinel 一眼索引";
that's untrue for `SEQ_TOO_OLD` today. Reader looking up "what is 410 / SEQ_TOO_OLD" lands
empty-handed unless they already know to look elsewhere.

---

### Sentinels in code but intentionally not in doc — not gaps

These are sentinels that exist in code but are correctly omitted because they cannot reach a handler:

- `chatdomain.ErrBlockNotFound` (`domain/chat/chat.go:229`) — produced by `infra/store/chat`, consumed only by `pkg/eventlog/eventlog.go:372-376, 408-412` Emitter (warn-and-continue / best-effort write).
- `eventlogdomain.ErrInvalidEvent` (`domain/eventlog/eventlog.go:233`) — producer-bug sentinel; handler validates `conversationID` before calling `Subscribe`, so the `bridge.Subscribe` path that returns it (`infra/eventlog/bridge.go:154`) is unreachable; in-flight Publish errors logged in `pkg/eventlog/eventlog.go:179`.
- `notificationsdomain.ErrInvalidEvent` (`domain/notifications/notifications.go:109`) — swallowed by `publisher.Publish` warn-log (`pkg/notifications/notifications.go:62-72`).
- `cryptoinfra.ErrNoFingerprint` (`infra/crypto/fingerprint.go:24`) — boot-only via `cmd/server/main.go:148`; never request-path.
- `llmclientpkg.ErrPickModel` / `ErrResolveCreds` / `ErrBuildClient` (`pkg/llmclient/llmclient.go:24-26`) — `app/chat/runner.go:113-118` `errors.Is`-discriminates these to assign `Message.errorCode` strings; doc sub-table at lines 115-126 documents the resulting `errorCode` values as the public contract (these sentinels are routing-only, never become HTTP envelope codes).

---

## In doc but not in code (stale / ghost)

| Wire code / Sentinel | Doc location | Severity | Notes |
|---|---|---|---|
| (none) | — | — | Every doc-listed sentinel exists as a real Go `var Err...`. Stale-row issues only manifest as ghost sentinels (no producer), tracked separately below. |

---

## Semantic drift

| Sentinel | Doc HTTP / wire | Real | Severity |
|---|---|---|---|
| `errorsdomain.ErrInvalidRequest` | doc line 56 calls it `derrors.ErrInvalidRequest` | real alias is `errorsdomain.ErrInvalidRequest`; the alias `derrors` does not exist in the repo | LOW |
| All bare-alias sentinel-column references throughout the table — `apikey.Err*` / `chat.Err*` / `tool.Err*` / `model.Err*` / `conversation.Err*` / `todo.Err*` / `mcp.Err*` / `skill.Err*` / `sandbox.Err*` / `subagent.Err*` / `forge.Err*` | doc uses bare names | per §S13, every import in errmap.go uses `<name>domain` suffix: `apikeydomain` / `chatdomain` / `forgedomain` / `modeldomain` / `convdomain` / `tododomain` / `mcpdomain` / `skilldomain` / `sandboxdomain` / `subagentdomain` | LOW (cosmetic) |
| Forge sentinel-column (doc lines 136-150) | doc writes `tool.ErrNotFound` / `tool.ErrDuplicateName` / etc. | real Go sentinels live in `domain/forge/forge.go` as `forgedomain.Err*`; the wire codes are `TOOL_*` for legacy client compat per Phase 1 rename (per doc footnote line 152), but the sentinel-column itself is wrong | LOW |
| `tool/web` BYOK sentinels (doc lines 163-167) | doc table marks all three ✅ as if handler-reachable | the doc note line 158 correctly explains they exist only for `errors.Is` matching; handler-wise they never reach `FromDomainError` because `WebSearch.Execute` (`app/tool/web/search.go:221-260`) catches BYOK errors inside `tryBYOKProvider` and falls through silently | LOW |
| `sandbox.ErrDockerNotInstalled` / `ErrDockerDaemonDown` (doc lines 267-268) | doc table marks ✅ | errmap.go comment lines 110-118 explicitly says "0 current consumers; pre-registered so future docker-runtime integration won't trigger 'unmapped domain error' warnings on first touch" | LOW (acknowledged pre-registration) |

---

## Sentinel without producer (ghost)

"Ghost" = sentinel declared in `domain/<name>/*.go` and registered in `errmap.go`, but **zero Go
code outside `_test.go`, the declaration file, and errmap.go itself returns it**.

| Sentinel | Code defined | Producer | Severity |
|---|---|---|---|
| `chatdomain.ErrProviderUnavailable` | `domain/chat/chat.go:232` | **none** — only the errmap registration line (`errmap.go:61`); no Go path returns this | MED |
| `chatdomain.ErrVisionNotSupported` | `domain/chat/chat.go:236` | **none** — only errmap registration (`errmap.go:65`); no vision-discrimination path returns this today (provider 400 surfaces as `LLM_BAD_REQUEST` instead) | MED |
| `forgedomain.ErrSandboxUnavailable` | `domain/forge/forge.go:405` | **none** — only `_test.go` references at `infra/store/forge/forge_test.go:736` + doc-comment mentions at `cmd/resources/main.go:30,45` and `test/harness/harness.go:248`. The producer was removed during the D2-5 sandbox rewrite; the v2 sandbox path does not surface this | MED |
| `forgedomain.ErrDependencyResolution` | `domain/forge/forge.go:414` | **none** — only `_test.go` at `infra/store/forge/forge_test.go:737`. Godoc promises "uv stderr captured" but no Go path returns this sentinel today | MED |
| `mcpdomain.ErrRuntimeMissing` | `domain/mcp/mcp.go:177` | **none** — only `_test.go` at `domain/mcp/mcp_test.go:133` (used in an errors-list, not produced). Possibly missed during the V3 cleanup (doc line 231) that purged the other marketplace ghosts | MED |
| `mcpdomain.ErrUnsupportedRuntime` | `domain/mcp/registry.go:164` | **none** — declaration site only. "no supported runtime for entry" condition handled silently or never triggered | MED |
| `sandboxdomain.ErrDockerNotInstalled` | `domain/sandbox/sandbox.go:209` | **none** — errmap.go:110-118 acknowledges this as "Phase 5 pre-registration" | MED (acknowledged) |
| `sandboxdomain.ErrDockerDaemonDown` | `domain/sandbox/sandbox.go:216` | **none** — same acknowledged pre-registration | MED (acknowledged) |

---

## Sub-check

- Sentinels aligned (doc ↔ code): **yes** for handler-reachable rows. Every doc-listed sentinel has a real `var Err…`; every errmap registration appears in the doc table. Only the doc's sentinel-column aliases (bare `apikey.Err*` etc.) don't match §S13 import aliases.
- errmap rows complete: **yes** for every handler-reachable sentinel. `SEQ_TOO_OLD` wire code intentionally bypasses errmap (handler writes envelope directly); that's consistent code-side, but the doc index doesn't list it.
- HTTP status semantic correct: **yes** — verified row-by-row, no mismatches between doc and errmap HTTP status.
- No ghost sentinels: **no** — 8 ghosts (4 acknowledged via errmap / cmd comments + 4 silent).

---

## Cross-cutting findings

1. **Two design patterns inflate the "ghost" count**:
   - `SEQ_TOO_OLD` is **inline-translated by the handler** in `transport/httpapi/handlers/eventlog.go:107-113` and `notifications.go:77-82`; the sentinels are `errors.Is`-checked but the response is written manually, never via `FromDomainError`. So the wire code legitimately is not in errmap—but error-codes.md as the single-source-of-truth index still owes the reader a row.
   - The four "pre-registered for future" sentinels: `sandbox.ErrDockerNotInstalled` / `ErrDockerDaemonDown` are intentionally pre-registered (errmap.go:110-118 comment is explicit). `forge.ErrSandboxUnavailable` / `ErrDependencyResolution` are less clear: the godoc on each sentinel claims active behavior ("returned when ...") but the v1 paths that produced them were excised during D2-5, and the v2 sandbox doesn't yet wire them. **If fix-up were in scope**: either reinstate producers (sandbox bootstrap-failure check + uv parse-fail discrimination) or delete the sentinels + doc rows.

2. **Two chat sentinels are dead**: `chatdomain.ErrProviderUnavailable` and `chatdomain.ErrVisionNotSupported` are doc-listed (✅) and errmap-registered but produced nowhere. The 502 `LLM_PROVIDER_ERROR` path is now exclusively driven by `llminfra.ErrProviderError` (doc line 105 lists both sentinels OR'd, but only one half is real). Vision discrimination doesn't happen in our code today—a non-vision provider receiving an image part will return a 400, which surfaces as `LLM_BAD_REQUEST`.

3. **Two MCP sentinels are dead**: `mcpdomain.ErrRuntimeMissing` and `ErrUnsupportedRuntime`. Doc line 231 ("Marketplace V3 / V2 cleanup ... 相继移除") removed `MCP_HANDSHAKE_FAILED` / `MCP_QUERY_REQUIRED` / `MCP_MARKETPLACE_UNAVAILABLE` / `MCP_ALIAS_COLLISION`; these two may have been overlooked.

4. **Doc-vs-real package alias inconsistency** runs through the entire sentinel column. Mechanical fix (per §S13): `apikey.Err*` → `apikeydomain.Err*`, `chat.Err*` → `chatdomain.Err*`, `tool.Err*` (forge rows) → `forgedomain.Err*`, `model.Err*` → `modeldomain.Err*`, `conversation.Err*` → `convdomain.Err*`, `todo.Err*` → `tododomain.Err*`, `mcp.Err*` → `mcpdomain.Err*`, `skill.Err*` → `skilldomain.Err*`, `sandbox.Err*` → `sandboxdomain.Err*`, `subagent.Err*` → `subagentdomain.Err*`, `forge.Err*` (env / sandbox rows) → `forgedomain.Err*`, `derrors.Err*` (line 56) → `errorsdomain.Err*`. `ask.Err*` (lines 183-184) actually maps to `askapp.Err*`, which is correctly `app/ask` not `domain/ask`—those are fine. `webtool.Err*` (lines 165-167) also correct. `cryptoinfra.Err*` / `reqctxpkg.Err*` / `llminfra.Err*` aliases used in the cross-cutting INTERNAL_ERROR + LLM_* / WEBSEARCH_* rows are correct.

5. **No HIGH findings**. Every handler-reachable sentinel is properly registered (no "unmapped domain error" 500 + warning log risk). Both inline wire codes (`SEQ_TOO_OLD`, plus the `INVALID_REQUEST` envelopes that handlers emit directly for top-of-handler input checks) emit valid `{error: {code, message}}` shape; clients see the right thing.

---

## Final scorecard

| Category | Count |
|---|---|
| HIGH | 0 |
| MED | 8 ghost sentinels (4 acknowledged pre-registration + 4 silent ghosts) |
| LOW | 5 (1 doc missing `SEQ_TOO_OLD` index row [counts once as a root issue; manifests in both eventlog + notifications handlers], 4 cosmetic alias-drift items) |

**Recommended fix priority** (if a fix-up pass is later authorized):

1. **MED — delete or revive ghost sentinels.** For each of `chatdomain.Err{ProviderUnavailable,VisionNotSupported}`, `forgedomain.Err{SandboxUnavailable,DependencyResolution}`, `mcpdomain.Err{RuntimeMissing,UnsupportedRuntime}`: decide whether to add the missing producer or remove the sentinel + errmap row + doc row. The two `sandbox.ErrDocker*` are deliberate—leave with the existing comment.
2. **LOW — add `SEQ_TOO_OLD` row.** Add to the "通用 (Phase 1)" table (or a new "SSE" sub-section) with a footnote: "emitted inline by `/api/v1/eventlog` + `/api/v1/notifications` handlers; bypasses errmap by design (handlers write SSE wire shape directly)".
3. **LOW — cosmetic alias fix.** One mechanical pass to replace bare-alias sentinel-column entries with §S13 import aliases. Improves `grep`-from-doc-to-code traceability.
