# File trace: backend/internal/pkg/notifications/notifications.go

LOC: 118

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | notifications.go:46-54 | `func New(bridge notificationsdomain.Bridge, log *zap.Logger) Publisher { if bridge == nil { return noopPublisher{} } if log == nil { log = zap.NewNop() } ... }` | A.1 | OK | §S3: bridge nil → noop fallback is documented at godoc lines 40-45 ("bridge nil → returns a noop Publisher (so service constructors can safely default-fall-back to it without dereferencing)"). Documented design fallback. log nil → `zap.Nop()` is the standard Go idiom for "logger is optional". Not a swallow — it's a "feature off" pattern. | — | — | — | — |
| 2 | notifications.go:61-77 | `func (p *publisher) Publish(...) { ... if _, err := p.bridge.Publish(ctx, ...); err != nil { p.log.Warn("notification publish failed", zap.String("type", eventType), zap.String("id", id), zap.Error(err)) } }` | A.1/A.4/§S10 | OK | §S3: error from `bridge.Publish` is **logged at Warn level** with type/id/err — not silently dropped. §S10 (async fire-and-forget MUST log): satisfied — `Warn` includes structured fields for debugging. Publisher godoc lines 28-37 explicitly says "Best-effort: failures log but do not surface as errors (notifications are observability, not business)". §S16 doesn't apply: no error wrap (it's a sink, doesn't return errors). The log output is the entire error contract surface. **Note**: log is `Warn` not `Error`; rationale: notifications drop is recoverable degradation (UI doesn't auto-update, user can refetch), not invariant breach. Match between log level and impact. | — | — | — | — |
| 3 | notifications.go:86-100 | `func With(ctx, p) ctx { ... }; func From(ctx) Publisher { p, ok := ctx.Value(publisherKey{}).(Publisher); if !ok || p == nil { return noopPublisher{} }; return p }` | A.1 | OK | §S3: From returns no-op when ctx has no publisher — documented at godoc lines 90-93 ("No nil-checks needed at call sites"). Same pattern as `pkg/eventlog.From`. Not a swallow — it's a "ctx wiring" graceful default. | — | — | — | — |
| 4 | notifications.go:106-112 | `func MustFrom(ctx context.Context) Publisher { p, ok := ctx.Value(publisherKey{}).(Publisher); if !ok || p == nil { panic(fmt.Sprintf("notifications.MustFrom: no publisher in ctx")) }; return p }` | A.1 | OK | §S3: opposite of `From` — panic when missing publisher is unambiguously a wiring bug. Documented at godoc lines 102-105. Panic message has `<pkg>.<func>:` prefix consistent with §S16 spirit (this is a panic format, not error wrap). **Minor**: `fmt.Sprintf` with no format verbs (no `%s` etc.) — cosmetic; could be a literal string. Not a violation. | — | — | — | — |
| 5 | notifications.go:118 | `func (noopPublisher) Publish(context.Context, string, string, any, ...string) {}` | A.1 | OK | §S3: noop intentionally does nothing — no error to swallow. Used when bridge is nil at construction time, or no publisher in ctx via `From`. Documented design (godoc line 40-45 + 90-93). | — | — | — | — |

## File summary

5 sites total. 5 OK. 0 violations.

The publisher pattern is a textbook §S10 fire-and-forget: bridge.Publish error is logged at `Warn` with structured fields (type, id, err). Fail-open behaviour matches the Publisher godoc explicit contract ("Best-effort: failures log but do not surface as errors (notifications are observability, not business)"). The only sentinel-shape thing in the file is the panic message at line 109, and it's correctly formatted with `<pkg>.<func>:` prefix.

No errors are returned from any function in this package, so §S16 wrap format and §S17 errmap registration do not apply.

No business IDs generated (§S15 N/A). No terminal-state writes (§S9 N/A).
