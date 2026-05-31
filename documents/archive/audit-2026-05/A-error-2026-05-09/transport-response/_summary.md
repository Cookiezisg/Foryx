# Package audit summary: internal/transport/httpapi/response

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: response writers swallow encode/wire errors after `WriteHeader` because the bytes are already on the wire (Go HTTP can't unflush). This is the established §S3 carve-out for "cleanup paths where no recovery is possible" (spec lines 21-26 / example 5). The package is functionally compliant; some sites are missing the inline `// _ = err — reason` comment that §S3 ritual recommends.
- **§S9 detached ctx 终态写**: **N/A across the entire package** — these are wire-write helpers (envelope.go, sse.go, errmap.go), not DB / persistent state mutators. ctx is read for shutdown only (sse.go:92), never used to scope a terminal write.
- **§S15 ID 生成**: **N/A across the entire package** — no per-request ID generation. `INTERNAL_ERROR` etc. are constants, not generated.
- **§S16 错误 wrap 格式**: **N/A across the entire package** — no `fmt.Errorf` calls in any of the 3 files; only zap.Error log usage. Errors propagated upstream via the lookup chain in errmap.go via `errors.Is`.
- **§S17 errmap 单一事实源**: this package IS the §S17 mechanism. Audit verified 75-entry coverage cross-checked against full grep of all `errors.New` declarations across `internal/{domain,pkg,infra,app}/`. **Every handler-reachable sentinel is registered**. Defensive entries (askapp.ErrAlreadyAnswered, askapp.ErrTimeout) are flagged LOW EDGE for clarity but are functionally harmless (errmap gracefully accepts orphan registrations).

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| envelope.go | 86 | 6 | 5 | 0 | 0 | 1 |
| errmap.go | 261 | 8 | 7 | 0 | 0 | 1 |
| sse.go | 96 | 5 | 3 | 0 | 0 | 2 |
| **TOTAL** | **443** | **19** | **15** | **0** | **0** | **4** |

## Severity breakdown (only VIOLATION + EDGE)

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 0 | — | — |
| LOW (silent error discard, missing inline `// _ = err — reason` per §S3 ritual) | 3 | envelope.go:#1 (writeJSON Encode); sse.go:#4 (onEvent err); sse.go:#5 (fmt.Fprint err) | FOUND |
| LOW (defensive errmap entry never actually reached in current code) | 1 | errmap.go:#6 (askapp.ErrAlreadyAnswered + askapp.ErrTimeout) | FOUND |

(Counts: 3 + 1 = 4; matches table.)

## Cross-cutting

### errmap coverage (the package's headline responsibility)

Audit ran a full grep of `var Err... = errors.New(...)` across `internal/{domain,pkg,infra,app}/`, then traced each sentinel via grep to determine whether it can flow through any `FromDomainError` call path. Outcome:

- **Reachable + registered**: all matched. 75 entries.
- **Defined but NOT reachable** (correctly absent from errmap):
  - `chatdomain.ErrBlockNotFound` — store-only consumer
  - `tododomain.ErrConversationMismatch` — explicitly never returned per ask.go:117-128 godoc
  - `eventlog/notifications.ErrInvalidEvent` — producer-side bug indicator, never reaches handler
  - `llmclient.ErrPickModel/ErrResolveCreds/ErrBuildClient` — translated inline at chat/runner.go:108-119 to wire codes via SSE emitFatalError, bypasses errmap
  - `sandbox.ErrDockerNotInstalled/ErrDockerDaemonDown` — declared but no callers (future-use)
  - `infra/crypto.ErrNoFingerprint` — internal to fingerprint helper
  - `infra/mcp.ErrConfigCorrupt` — internal to mcp.json loader
  - `app/mcp.ErrSearchServerUnavailable` — internal to searchrouter
  - `app/tool/{search,filesystem,shell,web,mcp,subagent,ask,skill}.ErrEmpty*` — tool framework consumed (returned via tool_result, not handler error path)
  - `chatdomain.ErrSeqTooOld` — handler at handlers/eventlog.go:107 + handlers/notifications.go:77 catches with errors.Is and emits SEQ_TOO_OLD/410 directly (bypasses errmap by design — different from default unmapped 500)
- **Reachable + registered defensively** (LOW EDGE site#6): askapp.ErrAlreadyAnswered (subsumed by ErrNoPendingQuestion post-refactor); askapp.ErrTimeout (tool-layer-only). Harmless registrations — recommend trim or comment.

### Direct-emit wire codes (NOT in errmap)

Handlers emit these wire codes directly via `responsehttpapi.Error` without going through errmap:

- `NOT_FOUND` (router 404 in middleware/notfound.go)
- `INTERNAL_ERROR` (panic in middleware/recover.go; matches errTable default at errmap.go:260)
- `SEQ_TOO_OLD` (eventlog/notifications stream restart at handlers/eventlog.go:110)
- `KIND_REQUIRED`, `OWNER_KIND_REQUIRED`, `MCP_COMMAND_REQUIRED`, `TRACER_DISABLED`, `UNKNOWN_ACTION` (handler-immediate validation responses)

All are **correct direct emissions** — they originate at framework / pre-domain layer (router miss, panic, immediate handler validation) where there's no upstream Go sentinel to translate. They don't violate §S17.

### §S3 ritual gap

Three sites silently discard error returns where the underlying behavior is correct (post-WriteHeader unrecoverable, client-disconnected) but the inline `// _ = err — reason` comment is missing:

| Site | Discard target | Rationale | Documented in godoc? |
|---|---|---|---|
| envelope.go:84 | `_ = json.NewEncoder(w).Encode(body)` | Header already flushed; can't change wire bytes | No |
| sse.go:87 | `_ = onEvent(w, item)` | Client disconnected; ctx will tear down loop | Yes (godoc lines 35-38) |
| sse.go:90 | `fmt.Fprint(w, ": keep-alive\n\n")` discards (n, err) | Same as above | No |

Recommendation: add a 1-line inline comment at each site (or extract `writeKeepAlive` helper). Functional behavior unchanged.

## Spot-check (random clean sites)

5 sites picked across all 3 files:

1. **envelope.go:#4** (NoContent): verified — `w.WriteHeader(http.StatusNoContent)` returns no error; cannot violate §S3.
2. **envelope.go:#5** (Paged): verified — pure construction + delegation; same writeJSON path.
3. **errmap.go:#1** (errTable contents): random sample of 8 entries cross-checked: `apikeydomain.ErrNotFound` → 404 / API_KEY_NOT_FOUND ✓; `chatdomain.ErrAttachmentTooLarge` → 413 / ATTACHMENT_TOO_LARGE ✓; `forgedomain.ErrPendingConflict` → 409 / TOOL_PENDING_CONFLICT ✓; `mcpdomain.ErrToolCallTimeout` → 504 / MCP_TOOL_CALL_TIMEOUT ✓; `sandboxdomain.ErrSpawnTimeout` → 504 / SANDBOX_SPAWN_TIMEOUT ✓; `webtool.ErrAuthFailed` → 401 / WEBSEARCH_AUTH_FAILED ✓; `context.Canceled` → 499 / CLIENT_CLOSED ✓; `cryptoinfra.ErrUnsupportedVersion` → 500 / INTERNAL_ERROR ✓.
4. **errmap.go:#3** (lookup function): verified — uses `stderrors.Is` (alias for errors.Is) so wrapped errors via `%w` properly traverse the chain.
5. **sse.go:#1** (flusher type-assertion fail): verified — emits 500 envelope explicitly; not a silent fallthrough.

All 5 confirmed mechanism not rubber-stamping.

## Recommended fix priorities

1. **§S3 ritual comments at 3 sites** (LOW × 3 — envelope.go:#1, sse.go:#4, sse.go:#5): single sweep commit, ~3 lines of inline justification. Style polish; functional behavior unchanged.
2. **Optional — askapp defensive registrations** (LOW × 1 — errmap.go:#6): either trim the two never-reached entries or add inline `// defensive — never reached post-2025 atomic-Resolve refactor`. Cosmetic.

**Net assessment**: package is **§S3/S9/S15/S16/S17 textbook clean** for its responsibilities. errmap.go specifically is the audit's source-of-truth subject and verified comprehensive — no missing sentinels reaching handlers. 4 LOW EDGE all stylistic/documentation.

## Out-of-scope notes

1. The `lookup` function does linear iteration over the 75-entry map — consider sorting/indexing if errmap grows past a few hundred entries (currently a non-issue at the actual sentinel count).
2. The `defensive registration` pattern at site#6 is a style choice — keeping orphans accessible to errors.Is doesn't break anything but does dilute the §S17 "single source of truth" assertion. A future cleanup pass could decide one way or the other; not blocking.
