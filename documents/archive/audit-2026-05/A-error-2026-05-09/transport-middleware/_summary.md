# Package audit summary: internal/transport/httpapi/middleware

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: middlewares decorate ctx (auth, locale) or wrap response (logger, cors, recover). Pass-through fallbacks (CORS disallowed origin, locale unsupported) are **explicit per-spec design**, fully documented in godoc — not silent swallowing of failures. The Recover middleware's "best-effort write fails silently if headers already flushed" is a per-spec carve-out (§S3 example 5: defer Close() cleanup is OK to swallow when fix is impossible). Recover always loud-logs panic + stack + method + path before attempting the 500 envelope.
- **§S9 detached ctx 终态写**: **N/A across the entire package** — middlewares only read/decorate ctx and write response wire bytes; they don't perform DB / persistent terminal writes. Logger emits zap log lines (§S9 spec line 51 explicitly excludes log writes from "terminal write" definition).
- **§S15 ID 生成**: **N/A across the entire package** — middlewares don't generate business IDs. `DefaultLocalUserID` is a package-level constant in `pkg/reqctx`, not a per-request generation.
- **§S16 错误 wrap 格式**: **N/A across the entire package** — no `fmt.Errorf` calls; only `zap.Error()` log usage which is not subject to wrapping rules.
- **§S17 errmap 单一事实源**: **N/A** — no Go sentinels defined. Wire codes emitted directly (NOT_FOUND in notfound.go, INTERNAL_ERROR in recover.go) are the correct path because they originate at framework level (router miss, panic), not from a domain error chain that would route via `FromDomainError`. They match the errTable default (`{500, "INTERNAL_ERROR"}` at errmap.go:260).

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| auth.go | 21 | 1 | 1 | 0 | 0 | 0 |
| cors.go | 89 | 2 | 2 | 0 | 0 | 0 |
| locale.go | 35 | 2 | 2 | 0 | 0 | 0 |
| logger.go | 83 | 3 | 3 | 0 | 0 | 0 |
| notfound.go | 20 | 1 | 1 | 0 | 0 | 0 |
| recover.go | 48 | 2 | 2 | 0 | 0 | 0 |
| **TOTAL** | **296** | **11** | **11** | **0** | **0** | **0** |

## Severity breakdown

| Severity | Count | Status |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 0 | — |

**Net: 0 violations**.

## Cross-cutting

### Auth simplification (Phase 2)

`InjectUserID` unconditionally stamps `DefaultLocalUserID` — Phase 2 simplified auth, godoc explicitly notes future JWT/session rewrite. Not a §S3 violation: there's no upstream signal that could fail (no real auth source yet). When Phase 5+ replaces this, the new code must surface auth failures (not silent default).

### Pass-through patterns

Three middlewares contain "pass through unchanged" branches; each is per-spec documented:

| Site | Pattern | Reason it's not §S3 violation |
|---|---|---|
| cors.go:66-69 | empty Origin → next | Same-origin or non-browser request; CORS irrelevant |
| cors.go:70-73 | disallowed Origin → next without CORS headers | Browser enforces, server has no opinion (per CORS spec) |
| locale.go:31-35 | unrecognized header → DefaultLocale | i18n best-effort, no error semantics; default is project's primary user language |

All three are explicit godoc-documented design choices, not silent error swallowing.

### Recover correctness

The single most §S3-relevant site: `recover.go:29-44`. Verified compliance:
1. Panic value never silently dropped — always `log.Error` with full stack + method + path before any response attempt.
2. Raw panic value never leaked to client (envelope is hardcoded "internal server error").
3. The "best-effort 500 fails silently if headers flushed" inline comment documents the unfixable Go-HTTP case (can't unflush bytes). Per §S3 carve-out for cleanup paths where no remediation is possible.
4. §S10 compliance: fire-and-forget recovery boundary always emits a log (asynchronous-equivalent context per `recover()` semantics).

### errmap.go connection

Wire codes emitted directly by middleware (not via `FromDomainError`):
- `NOT_FOUND` (404 in notfound.go) — router miss, no domain error chain to translate.
- `INTERNAL_ERROR` (500 in recover.go) — panic boundary; matches errTable default at errmap.go:260.

Both are **correct direct emissions** because they bypass the domain-error-translation path entirely. Adding them as Go sentinels would force all 404s and panics through fake-sentinel routing — anti-pattern.

## Spot-check (random clean sites)

5 sites picked across all 6 files:

1. **auth.go:#1** (InjectUserID): verified — unconditional `SetUserID(DefaultLocalUserID)` matches Phase 2 simplified auth doc; no error path possible.
2. **cors.go:#1** (origin pass-through): verified — godoc lines 44-46 / 51-53 spell out the four pass-through cases; behavior matches.
3. **logger.go:#1** (Write returns err): verified — `n, err := r.ResponseWriter.Write(b); ... return n, err` propagates error to caller; not swallowed.
4. **logger.go:#2** (Flush type-assert): verified — `if f, ok := r.ResponseWriter.(http.Flusher); ok { f.Flush() }` — assertion guard returns void if not flushable; no error to surface (interface signature `Flush()` returns nothing).
5. **recover.go:#1** (panic recovery): verified — log.Error always fires before envelope write; envelope strings hardcoded ("internal server error"); never leaks raw panic.

All 5 spot-checks confirmed mechanism, not rubber-stamping.

## Recommended fix priorities

**No fixes needed**. Package is §S3/S9/S15/S16/S17 textbook clean.

## Out-of-scope notes

1. Future JWT/session auth rewrite (Phase 5+) — when InjectUserID gains a real source, the failure path needs §S3 attention (return 401 envelope, not silent default).
2. Locale support expansion — if more locales are added, the simplified prefix match → x/text/language migration noted in godoc; doesn't affect §S3 stance (default fallback remains valid for unmatched).
