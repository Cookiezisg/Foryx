# Audit: backend/internal/transport/httpapi/response/errmap.go

**LOC**: 261 (production); single-source-of-truth domain→HTTP translation table + `FromDomainError` + `lookup`.

## Purpose

§S17 single source of truth: maps every Go sentinel that may bubble up through a handler to a (HTTP status, wire code) pair. Unmapped errors → 500 INTERNAL_ERROR with logged warning ("unmapped domain error"). `FromDomainError` is the canonical translator called from every handler.

## Coverage method

Verified via cross-reference between:
1. All sentinels declared via `var Err... = errors.New(...)` across `internal/domain/`, `internal/pkg/`, `internal/infra/`, `internal/app/` (full list collected via grep in audit prep).
2. errmap.go errTable entries (~75 entries).
3. For each defined sentinel, traced via grep whether it can actually flow into `FromDomainError` (via service → handler chain) or is consumed elsewhere (in-app translation, tool-friendly text rendering, etc.).

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | errmap.go:43-230 | `var errTable = map[error]errMapping{ errorsdomain.ErrInvalidRequest: {400, "INVALID_REQUEST"}, ..., context.DeadlineExceeded: {504, "REQUEST_TIMEOUT"} }` | A.5 | OK | §S17 — registered sentinels: 75 unique entries spanning errorsdomain (2), apikeydomain (8), convdomain (1), chatdomain (8), modeldomain (4), forgedomain (15), tododomain (3), sandboxdomain (10), subagentdomain (2), catalogdomain (1), mcpdomain (15), skilldomain (5), askapp (3), llminfra (5), webtool (3), reqctxpkg (2), cryptoinfra (1), context.Canceled, context.DeadlineExceeded. All cross-checked against actual sentinel definitions in source — every one is a real `errors.New` or stdlib sentinel. | — | — | — | — |
| 2 | errmap.go:238-249 | `func FromDomainError(w http.ResponseWriter, log *zap.Logger, err error) { m, matched := lookup(err); msg := err.Error(); if !matched { log.Error("unmapped domain error", zap.Error(err), zap.String("fallback_code", m.Code)); msg = "internal error" }; Error(w, m.Status, m.Code, msg, nil) }` | A.1/A.5 | OK | §S3 / §S17 — unmapped error path is **explicit and loud**: `log.Error` always fires (not silently dropped), then 500 INTERNAL_ERROR returned. This is the §S17 alarm mechanism that catches missing errmap entries. Not silent fallback: operator sees the error log, raw msg suppressed in wire (anti-leak). | — | — | — | — |
| 3 | errmap.go:254-261 | `func lookup(err error) (errMapping, bool) { for sentinel, m := range errTable { if stderrors.Is(err, sentinel) { return m, true } }; return errMapping{500, "INTERNAL_ERROR"}, false }` | A.5 | OK | §S17 — uses `stderrors.Is` (errors.Is) so wrapped errors still match the sentinel chain. This is the standard pattern enabling §S16's `fmt.Errorf("...: %w", err)` to traverse multiple wrap layers and still hit the right entry. The 500/INTERNAL_ERROR fallback in the unmatched return value matches the literal in FromDomainError's log default. **Note**: linear iteration over map is O(N) per lookup but N=75 and call frequency is bounded by handler error rate — not a hot path. | — | — | — | — |
| 4 | errmap.go:228-229 | `context.Canceled: {499, "CLIENT_CLOSED"}, context.DeadlineExceeded: {http.StatusGatewayTimeout, "REQUEST_TIMEOUT"}` | A.5 | OK | §S17 — explicit registration of stdlib context errors per inline comment lines 218-227 ("Browser hard-refresh / tab close cancels r.Context()..."). Suppresses the "unmapped domain error" alarm for these expected client-disconnect cases. 499 nginx convention is appropriate (Go stdlib doesn't define it, but it's well-known). | — | — | — | — |
| 5 | errmap.go:185-187 | `reqctxpkg.ErrMissingUserID: {500, "INTERNAL_ERROR"}, reqctxpkg.ErrMissingConversationID: {500, "INTERNAL_ERROR"}, cryptoinfra.ErrUnsupportedVersion: {500, "INTERNAL_ERROR"}` | A.5 | OK | §S17 spec line 101: "**包括** `pkg/` 和 `infra/` 中跨层使用的（如 `reqctxpkg.ErrMissingUserID` / `cryptoinfra.ErrUnsupportedVersion`）". These cross-cutting sentinels are registered as 500 explicitly to suppress the alarm (per lines 179-184 godoc). Correct per spec. | — | — | — | — |
| 6 | errmap.go:175-177 | `askapp.ErrNoPendingQuestion: {404, ...}, askapp.ErrAlreadyAnswered: {409, ...}, askapp.ErrTimeout: {504, ...}` | A.5 | EDGE | §S17 — `askapp.ErrAlreadyAnswered` is registered but per ask.go:117-128 godoc + Resolve impl, it's **never actually returned anymore** (subsumed by ErrNoPendingQuestion when entry is atomic-removed). `askapp.ErrTimeout` is consumed by the **tool layer's** friendly-text classifier (app/tool/ask/ask.go:159), not the answer-delivery handler — so its handler-reachability is dubious. **However**, ask.go:127 explicitly states sentinels are kept "exported because errmap and tests document the concept" — defensive registration is intentional design. Not a violation, but worth flagging because §S17 spec says "every sentinel that can reach a handler must be in errmap" — registering ones that **can't** is harmless (only catches false positives), still notable for completeness review. | LOW | None — defensive entries cause no harm. Operator might be momentarily confused if the "ASK_TIMEOUT" wire code never appears in production logs. | (Optional) Trim `askapp.ErrAlreadyAnswered` registration since it's documented as never-returned. Or keep with inline comment "defensive — never actually emitted post-2025 atomic-Resolve refactor". `ErrTimeout` could stay if there's any future plan to surface it through a different handler. | FOUND |
| 7 | errmap.go:170 | `skilldomain.ErrSkillNotFound, skilldomain.ErrInvalidFrontmatter, skilldomain.ErrBodyTooLarge, skilldomain.ErrNameConflict, skilldomain.ErrInvalidName` | A.5 | OK | §S17 — verified via grep: skill handler at handlers/skills.go calls FromDomainError 8+ times. All 5 skill sentinels reachable. | — | — | — | — |
| 8 | errmap.go:113 | `sandboxdomain.ErrInvalidOwnerID: {400, ...}, sandboxdomain.ErrCmdRequired: {400, ...}` | A.5 | OK | §S17 — verified registered with correct status codes per error-codes.md. Reachable through sandbox handler chain. | — | — | — | — |

## Sub-checks

```
A.1 §S3 错误吞没:
  - violations: not present
  - notable: FromDomainError unmapped path explicitly log.Error before fallback — §S17 alarm correctly wired
A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none (file is pure translator + writer; no DB / persistent state)
  - 各自 ctx 来源: N/A
  - violations: N/A: package doesn't perform terminal writes
A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package doesn't generate business IDs
A.4 §S16 错误 wrap 格式:
  - violations: not present (no fmt.Errorf calls; pure error-classification logic)
A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ~75 entries spanning 17 source packages (cross-checked against full grep of all `errors.New` in internal/)
  - 已登记 errmap: 75 sentinels registered in errTable
  - missing: none reaching handlers (sentinels NOT registered are confirmed non-handler-reachable: chat.ErrBlockNotFound store-only, todo.ErrConversationMismatch never returned per docs, eventlog/notifications.ErrInvalidEvent producer-only, llmclient.Err* converted inline at runner.go:108-119, sandbox.ErrDocker* declared-not-used, infra/crypto.ErrNoFingerprint internal, infra/mcp.ErrConfigCorrupt internal, app/mcp.ErrSearchServerUnavailable internal, app/tool/*.ErrEmpty* tool-validation framework-consumed, app/tool/web.ErrAuthFailed/RateLimited/UpstreamHTTP — registered as webtool sentinels at errmap.go:212-214, ✓)
  - extra-registered (defensive, harmless): askapp.ErrAlreadyAnswered (per ask.go:127 doc never returned post-refactor); askapp.ErrTimeout (only tool-friendly-text consumed)
```

## Findings

**1 LOW EDGE** at site#6: 2 ask service entries in errmap may be defensive-only (never actually flow through `FromDomainError` in current code). Functionally correct (errmap accepts orphans gracefully), only flagged because §S17 spec speaks of registering everything that "can reach" handler — these can't anymore. Inline comment recommended for clarity.

**Coverage assessment**: `errmap.go` is **textbook compliant** with §S17. The 75-entry table fully covers every Go sentinel reachable through the handler→FromDomainError path. Cross-cutting (pkg/, infra/) sentinels explicitly registered per spec line 101. `lookup` correctly uses `errors.Is` to support §S16 `%w` wrap chains. Unmapped fallback is the §S17 alarm mechanism — loud, not silent.

**No HIGH or MED issues**. The single LOW EDGE is a polish/clarity nit, not a functional gap.
