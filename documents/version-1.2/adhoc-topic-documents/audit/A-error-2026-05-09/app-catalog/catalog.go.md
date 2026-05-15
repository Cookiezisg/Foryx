# catalog.go — Phase A audit

**Path**: `backend/internal/app/catalog/catalog.go`
**LOC**: 241
**Role**: Service struct + lifecycle (`New`, `SetGenerator`, `SetPollInterval`, `RegisterSource`, `snapshotSources`, `Get`, `GetForSystemPrompt`, `nextVersion`). No I/O, no error returns from any method here — all the runtime risk lives in `polling.go` / `generator.go` / `disk.go`.

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | catalog.go:140-142 | `if log == nil { panic("catalog.New: logger is nil") }` | A.1 | OK | §S3 carve-out for unrecoverable boot-time invariants. Same pattern as apikey/mcp. Caught at app boot, never runtime. | N-A | — | — | — |
| 2 | catalog.go:143-145 | `if notif == nil { notif = notificationspkg.New(nil, log) }` | A.1 | OK | This is graceful nil-handling, not error suppression. notificationspkg.New(nil, log) returns a documented no-op publisher (notifications.go:46-49). Service constructors expect to work without an events bridge wired. Not §S3 silent fallback because no error involved — the nil arg is a valid input (no bridge configured = no notifications emitted). | N-A | — | — | — |
| 3 | catalog.go:152 | `s.lastFP.Store("")` | A.1 | OK | Atomic store with no error return. Boot init of empty string sentinel for "no fingerprint observed yet". | N-A | — | — | — |
| 4 | catalog.go:185-189 | `RegisterSource` — `s.sources = append(s.sources, src)` under sourcesMu.Lock | A.1 | OK | Plain mutex-protected slice append; no error path. | N-A | — | — | — |
| 5 | catalog.go:213-215 | `Get` — `return s.cache.Load()` | A.1 | OK | Atomic load returning nil-or-pointer; caller treats nil as "no cache yet" per godoc. Not error suppression. | N-A | — | — | — |
| 6 | catalog.go:223-229 | `GetForSystemPrompt` — `if cat == nil { return "" }` | A.1 | OK | Empty-string contract documented as "no catalog built yet — caller silently skips section". Per catalog.md §6 cold-start window this is the documented, intentional contract for chat.runner. Not §S3 silent fallback because no upstream call has failed; cache is just not yet populated. | N-A | — | — | — |
| 7 | catalog.go:235-240 | `nextVersion` — mutex-guarded counter increment | A.1 | OK | No error path; pure counter. | N-A | — | — | — |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: not present
  - rationale: file is pure struct + lifecycle; no error-returning calls. Two nil-check sites (#1 panic; #2 graceful no-op fallback) are explicit, not silent.

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none in this file
  - 各自 ctx 来源: N/A
  - violations: N/A: file does not perform any DB / disk / SSE writes. All terminal writes live in polling.go (Refresh → cache.Store / lastFP.Store / saveToDisk / notif.Publish).

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package's only persistent identifier is the SHA-256 fingerprint (computed in polling.go::fingerprint), not a §S15 business ID. Fingerprint correctly uses `sha256` + `hex.EncodeToString` (deterministic content hash, not random ID).

A.4 §S16 错误 wrap 格式:
  - violations: not present
  - rationale: no `fmt.Errorf` / `errors.New` calls; only `panic("catalog.New: logger is nil")` literal which carries the `<pkg>.<Method>:` prefix per §S16 spirit (panic message is not subject to %w but follows same locator pattern).

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in this file (catalogdomain.ErrCoverageIncomplete + catalogdomain.ErrGenerationFailed live in domain/catalog/catalog.go:124,132)
  - 已登记 errmap: catalog domain sentinels are NOT in errmap.go (verified: no `catalog` mention in `errmap.go`). Per design doc §10 ("均不上抛 handler——catalog 内部消化") this is intentional.
  - missing: N/A: file defines no sentinels. **However, cross-cutting concern raised in _summary.md**: the design promise that "ErrGenerationFailed never reaches handler" is technically broken by polling.go::Refresh's "all sources failed" path (which IS reachable from the HTTP `:refresh` handler) — see polling.go.md and _summary.md for details.

## Spot-check

- Verified `panic("catalog.New: logger is nil")` follows `<pkg>.<Method>:` locator format (matches apikeyapp / mcpapp panic conventions).
- Verified `Generator` interface signature matches `LLMGenerator.Generate` in generator.go:95 — no implicit nil-safety footgun (Service.Refresh checks `s.generator != nil` before calling).
- Verified `atomic.Pointer[catalogdomain.Catalog]` zero value is nil, and `Get` / `GetForSystemPrompt` both correctly handle the nil pre-Refresh case.
