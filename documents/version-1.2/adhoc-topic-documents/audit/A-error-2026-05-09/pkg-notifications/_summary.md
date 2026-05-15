# Package audit summary: internal/pkg/notifications

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: One `bridge.Publish` error path (line 71-76), correctly handled — logged at `Warn` with structured fields (type, id, err). The Publisher godoc lines 28-37 explicitly document this as "Best-effort: failures log but do not surface as errors (notifications are observability, not business)". Two nil-tolerant fallbacks (bridge nil → noop, ctx-publisher missing → noop) are documented design patterns, mirroring `pkg/eventlog`. All annotated.
- **§S9 detached ctx 终态写**: **N/A** — package is producer-side helper. `Publish` accepts `ctx` and threads it through `bridge.Publish`. No terminal-state writes inside this package; bridge implementation may persist (out of scope for this file).
- **§S15 ID 生成**: **N/A** — package doesn't generate business IDs. Caller passes the entity ID (`conv_xxx`, `td_xxx` etc.) to `Publish`.
- **§S16 错误 wrap 格式**: **N/A** — zero `fmt.Errorf` / `errors.New`. Package returns no errors. The single `fmt.Sprintf` at line 109 is a panic message with `<pkg>.<func>:` prefix consistent with §S16 spirit.
- **§S17 errmap 单一事实源**: **N/A** — no sentinels defined. Bridge errors are logged-and-dropped per godoc contract; never reach handler.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| notifications.go | 118 | 5 | 5 | 0 | 0 | 0 |
| **TOTAL** | **118** | **5** | **5** | **0** | **0** | **0** |

## Severity breakdown

| Severity | Count | Status |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 0 | — |

**Net: 0 violations**.

## Cross-cutting

### §S10 fire-and-forget compliance

Per CLAUDE.md §S10 ("异步或 fire-and-forget 必须打"), notifications is a quintessential fire-and-forget producer. The implementation correctly logs at `Warn` with structured fields:

```go
p.log.Warn("notification publish failed",
    zap.String("type", eventType),
    zap.String("id", id),
    zap.Error(err))
```

This is the canonical §S10 pattern. The `Warn` level (not `Error`) matches the impact: dropped notification = stale UI until next refetch, not data loss. If the level were `Error`, it would inflate the smoke alarm signal-to-noise ratio.

### Why no errmap entry

The package's only error path is `bridge.Publish` failure at line 66-77, and the contract is to **log + drop**. The error never returns to caller, never reaches `responsehttpapi.FromDomainError`. errmap registration correctly absent.

### MustFrom panic format

Line 109 panic message uses `fmt.Sprintf("notifications.MustFrom: no publisher in ctx")` — note: no format verbs (could be a literal). This is cosmetic, not a §S16 violation. Per CLAUDE.md §S16 the rule is for `fmt.Errorf` calls (error wrap), and panic format is governed by Go convention. The `<pkg>.<func>:` prefix is correct, matching the `apikeystore.List:` example in §S16.

If style cleanup were ever pursued: `panic("notifications.MustFrom: no publisher in ctx")` is equivalent. Not required.

### Twin-pattern with pkg/eventlog

The package godoc says: "Mirrors pkg/eventlog pattern (Emitter + With/From/MustFrom)." Verified by inspection — same shape:

| pkg/eventlog | pkg/notifications |
|---|---|
| `Emitter` interface | `Publisher` interface |
| `New(bridge, log)` | `New(bridge, log)` |
| `With(ctx, e)` | `With(ctx, p)` |
| `From(ctx)` → noop fallback | `From(ctx)` → noop fallback |
| `MustFrom(ctx)` → panic | `MustFrom(ctx)` → panic |

The pattern is consistent across both producer-side ctx-injected helpers in `pkg/`. If new SSE-like protocols are added, this is the proven shape.

## Spot-check (random clean sites)

3 sites picked across the file:

1. **notifications.go:1-12** (package doc): bilingual godoc explicitly references the eventlog twin pattern. Audited and confirmed accurate.
2. **notifications.go:61** (publish entry): variadic `conversationID ...string` matches the godoc line 28-32 contract ("conversationID is optional — pass `\"\"` for entity types that are not conversation-scoped"). Variadic is a Go-idiomatic way to express "optional with sensible default of empty". Confirmed.
3. **notifications.go:114-118** (noop): empty-method noop. Only role is "satisfy interface, do nothing". Used when bridge is nil at New() OR no publisher in ctx via From(). Both paths covered by godoc comments above. Mechanism aligned with intent.

All 3 spot-checks confirmed mechanism, not rubber-stamping.

## Recommended fix priorities

**No fixes needed**. Package is §S3/S9/S15/S16/S17 textbook clean and follows §S10 fire-and-forget pattern correctly.

## Out-of-scope notes

1. **Bridge implementation** (in `internal/infra/notifications/` or similar) is a separate audit concern. This package is the producer-side helper only.
2. **Subscriber-side SSE handlers** (in transport/) are also separate. The error contract here ("log and drop") only governs the producer side.
3. The Publisher contract is "no error returns" — meaning service code that calls Publish cannot retry, cannot detect failure for compensation logic. This is a deliberate trade-off (notifications are observability, not business). If a future feature needs guaranteed delivery (e.g., transactional outbox), it would need a different abstraction. Not a Phase A audit concern.
