# Package audit summary: internal/transport/httpapi/router

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: router is pure assembly — no error paths to swallow. Nil-tolerant per-domain registration is documented design ("integration tests can stay narrow"; per Deps godoc lines 31-38), not silent fallback. The `_ = deps.SubagentService` placeholder has annotating godoc comment around it explaining the route removal — meets §S3 ritual.
- **§S9 detached ctx 终态写**: **N/A** — router never touches ctx; pure handler composition.
- **§S15 ID 生成**: **N/A** — no per-request ID generation.
- **§S16 错误 wrap 格式**: **N/A** — no `fmt.Errorf` calls or error returns.
- **§S17 errmap 单一事实源**: **N/A at this file level** — router invokes middleware/notfound.go (404 fallback) which directly emits `NOT_FOUND` wire code without traversing errmap (correct: no Go sentinel exists at that level). All domain handler error paths route through `responsehttpapi.FromDomainError` deep inside individual handlers, not at the router layer.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| deps.go | 217 | 1 | 1 | 0 | 0 | 0 |
| router.go | 102 | 5 | 5 | 0 | 0 | 0 |
| **TOTAL** | **319** | **6** | **6** | **0** | **0** | **0** |

## Severity breakdown

| Severity | Count | Status |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 0 | — |

**Net: 0 violations**.

## Cross-cutting

### Chain order documentation

router.go:13-19 / 24-27 contains the **source-of-truth** for middleware chain layering:

```
Recover → RequestLogger → CORS → InjectLocale → InjectUserID → mux
```

Rationale captured in godoc:
- Recover outermost catches inner panics
- RequestLogger second so access log captures 500s from Recover
- CORS / locale / userID innermost so preflight OPTIONS (terminates inside CORS) doesn't pay their cost

This is referenced by middleware/recover.go's "must be the OUTERMOST middleware" claim — the assertion is enforced **by router.go's `applyChain` ordering**, not by any code-level check. The audit verified `applyChain` order is correct (Recover applied last → outermost on the wire).

### Nil-tolerant registration

router.go:35-77 has a chain of `if deps.X != nil { ... }` registrations covering 12 domains. This is by design (documented in deps.go godoc) so:
- Test bootstraps can wire only the domains they exercise
- Production main.go always wires everything, so nothing silently disappears in shipped binaries
- Phase rollouts can add new domains without breaking earlier-Phase code that doesn't know about them

Not a §S3 silent-fallback violation — it's the standard nil-pattern DI seam.

### `_ = deps.SubagentService` (line 68)

A textbook §S3 example: bare `_ = X` IS annotated by surrounding godoc lines 62-67 explaining why the field is kept (no longer registers HTTP routes; sub-run data observed elsewhere). Annotation satisfies the ritual.

### errmap interaction

Router doesn't touch errmap directly. The 404 fallback at line 84 routes to `middleware.NotFound` which emits `NOT_FOUND` wire code via `responsehttpapi.Error` (not via `FromDomainError`). This is correct because there's no Go sentinel at the router layer — a path that doesn't match any registered route is purely a framework miss, not a domain error.

## Spot-check (random clean sites)

3 sites picked across both files:

1. **deps.go:#1** (struct fields): random sample of 5 fields verified nil-tolerant per godoc (APIKeyService line 42-43, ChatService line 58-59, EventLogBridge line 63-69 ("Phase 1 wiring: present alongside EventsBridge but no producer publishes to it yet"), AskService line 89-93, ShellManager line 207-215 ("Nil-tolerant: when unset DevHandler simply doesn't register the route")) — all match line 39's "service fields are nil-tolerant" claim.
2. **router.go:#1** (nil checks): verified `if deps.APIKeyService != nil { ... }` pattern repeats consistently for all 12 domain services.
3. **router.go:#5** (applyChain): verified order is exactly inverse of wire-order documented at lines 13-19 (innermost added first).

All 3 spot-checks confirmed mechanism, not rubber-stamping.

## Recommended fix priorities

**No fixes needed**. Package is §S3/S9/S15/S16/S17 textbook clean.

## Out-of-scope notes

1. The `Deps` struct has Phase-rollout artifacts (e.g., EventLogBridge "Phase 1 wiring" note line 62-69 — long since superseded by Phase 2 cuts). When the codebase periodically does docs-housekeeping, these dated comments should be refreshed to reflect current state, but it's not an §S3-S17 issue.
2. The dev-only fields cluster at the bottom of Deps (lines 152-215) is a cohesion concern — could be extracted to a separate `DevDeps` embedded struct for clarity. Not a Phase A audit concern.
