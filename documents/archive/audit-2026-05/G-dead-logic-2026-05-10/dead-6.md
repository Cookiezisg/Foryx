# dead-6 — events/notifications infra + small app domains

Audit fork G — 2026-05-10. Read end-to-end:

- `backend/internal/infra/eventlog/bridge.go`
- `backend/internal/infra/notifications/bridge.go`
- `backend/internal/pkg/notifications/notifications.go`
- `backend/internal/app/catalog/{catalog,disk,polling,generator,mechanical}.go`
- `backend/internal/app/skill/{skill,scan,activate,mutate,polling,search,import,catalogsource}.go`
- `backend/internal/app/conversation/conversation.go`
- `backend/internal/app/todo/todo.go`

Dead-logic = "code still runs but the work has lost meaning". Severity scale:

- **HIGH** — actively harmful (wrong errors, hides bugs, leaks resources, regression risk)
- **MED** — costs cycles or readers' attention with no upside
- **LOW** — clean-up tax; low priority but not zero
- **EDGE** — debatable / defensible / matter of taste, or out-of-scope cross-reference

---

## HIGH (none)

No actively harmful dead logic found. The bridges are still in active dual-protocol use post-D2-C; both producer paths (eventlog Emitter, notifications Publisher) feed live SSE streams consumed by `testend`.

---

## MED-1 — `pkg/notifications` ctx-wiring section is entirely dead

- **Location**: `backend/internal/pkg/notifications/notifications.go:79-112`
  - `publisherKey struct{}` (line 81)
  - `With(ctx, p) context.Context` (lines 86-88)
  - `From(ctx) Publisher` (lines 94-100)
  - `MustFrom(ctx) Publisher` (lines 106-112)
- **Claims to do**: ctx-injected wiring so service code can `Publish("conversation", id, snapshot)` without dragging the bridge through every call site (per package godoc lines 1-12).
- **Reality**:
  - `notificationspkg.With(...)` — **0 call sites** anywhere in `backend/` (incl. test/ and harness/).
  - `notificationspkg.MustFrom(...)` — **0 call sites** anywhere.
  - `notificationspkg.From(ctx)` — only 2 call sites and both pass `context.Background()` (an empty ctx) purely to obtain a no-op publisher: `app/chat/chat.go:142` and `app/todo/todo.go:51`. Both are equivalent to `notificationspkg.New(nil, log)` (cf. `conversation/conversation.go:42` which uses `New(nil, log)` for the same fallback). So `From` is being used as a clumsy noop-factory, not as ctx wiring.
  - Every actual consumer holds `notif notificationspkg.Publisher` as a struct field (catalog Service / chat Service / conversation Service / mcp Service / sandbox Service / skill Service / todo Service), constructor-injected. None inject through ctx.
- **Why it's dead**: the package godoc claims to mirror `pkg/eventlog`, but the `pkg/eventlog.With` pattern is genuinely needed there (3 call sites: `chat/runner.go:90`, `chat/chat.go:351`, `subagent/spawn.go:147`) because the emitter needs to thread through async tool execution where the producer doesn't want a struct field. For notifications nobody has that need — the publisher is a singleton-per-process whose injection pattern is "constructor + struct field," fully solved without ctx.
- **Severity**: MED. Costs ~35 lines + 1 panic-formatter alloc + 1 `fmt` import + cargo-culted package-doc paragraph. Misleads future readers ("oh, there's ctx wiring — must be a reason?"). No correctness damage.
- **Fix**: delete the four entities (`publisherKey`, `With`, `From`, `MustFrom`) plus the `import "fmt"` they justify. Replace the two `notificationspkg.From(context.Background())` call sites with `notificationspkg.New(nil, log)` (already the pattern used by conversation/skill/catalog/mcp/sandbox). Update godoc lines 5-6 / 11-12 to drop the "Mirrors pkg/eventlog (Emitter + With/From/MustFrom)" claim.
- **Risk**: none — purely subtractive.

---

## MED-2 — Bridge.log field set but never read (both bridges)

- **Location**:
  - `backend/internal/infra/notifications/bridge.go:47` (struct field), line 69-74 (constructor)
  - `backend/internal/infra/eventlog/bridge.go:48` (struct field), line 80-88 (constructor)
- **Claims to do**: the `*zap.Logger` field on `Bridge` is named `log` and the constructor wires `log.Named("notifications.bridge")` / `log.Named("eventlog.bridge")` — clearly intended for logging.
- **Reality**: grep for `b.log` in either bridge file: zero hits. Neither bridge ever logs. The `if log == nil { log = zap.NewNop() }` guard, the `log.Named(...)` annotation, and even the `log` parameter on `NewBridge` are vestigial.
- **Why it's dead**: presumably the author started with "I'll log slow-subscriber events / capacity warnings" then never followed through. The behavior is in fact **correct without logging** (slow-subscriber blocks publisher by design, ErrSeqTooOld is returned not logged, replay overflow returns wrapped error not log). So the field has nothing to do — but the wiring still pretends.
- **Severity**: MED. Wastes an allocation per `NewBridge` (named logger), forces every caller to pass a logger they don't need, and lies about behavior in the constructor signature.
- **Fix**: two options. (a) Remove the `log` field + constructor parameter from both `Bridge` types — rewire main.go + harness to call `NewBridge()` with no args; or (b) actually log the two not-currently-logged-but-could-be cases: replay-buffer overflow (currently returns error wrapped; logging would aid debugging) + slow-subscriber-cancel during fanout. Option (a) simpler given §S10 ("synchronous primitives don't self-log; let caller decide") — bridges fit "synchronous primitive" since they're called by services that already log on error.
- **Risk**: low — call sites (`cmd/server/main.go`, `test/harness/harness.go`) need updating. Compile-time errors will catch all of them.

---

## MED-3 — `bufferedEnvelope.at` field written but never read (both bridges)

- **Location**:
  - `backend/internal/infra/notifications/bridge.go:55-58` (struct), line 96 (write `at: time.Now()`)
  - `backend/internal/infra/eventlog/bridge.go:66-69` (struct), line 130 (write `at: time.Now()`)
- **Claims to do**: implicit — looking at the field name and the time.Now() write, it was meant for either time-window eviction (drop entries older than X minutes) or observability/debugging.
- **Reality**: `at` is never read. Eviction is purely size-based (`len(buffer) > replayBufferSize`); ErrSeqTooOld checks `buffer[0].env.Seq > fromSeq+1`, not `buffer[0].at`. No timestamp-based eviction or query exists in either bridge.
- **Why it's dead**: `time.Now()` is called on every Publish (cheap but not free — ~30ns). The struct gains 24 bytes per buffered entry (4096 × 24 = ~96 KB wasted in eventlog hot path × N conversations). The `time` import in both files exists **solely** to populate this never-read field.
- **Severity**: MED. Real cost in steady-state RAM (per-conversation 96 KB nobody reads). Mostly invisible but compounds with active-conversations count.
- **Fix**: remove the `at` field, the `time.Now()` write, and the `time` import in both bridges. Make `bufferedEnvelope` a thin wrapper or even fold to just `[]Envelope` — the `bufferedEnvelope` wrapper's only job was carrying `at`.
- **Risk**: none — purely subtractive, no behavior change.

---

## LOW-1 — `notifications.bridge.go:137` returns `ErrSeqTooOld` for replay overflow (semantically wrong, also unreachable)

- **Location**: `backend/internal/infra/notifications/bridge.go:131-139`
  ```go
  for _, be := range b.buffer {
      if be.env.Seq > fromSeq {
          select {
          case sub.ch <- be.env:
          default:
              b.mu.Unlock()
              return nil, nil, notificationsdomain.ErrSeqTooOld  // ← wrong error
          }
      }
  }
  ```
- **Claims to do**: defensive guard against pushing replay items into a full subscriber channel.
- **Reality**: two problems.
  1. **Unreachable in practice**: subscriber channel cap is `subscriberBufferSize = replayBufferSize + 256 = 1280`, and we hold `b.mu` so no concurrent writes; the channel is freshly-created and empty. We can push at most 1024 (replayBufferSize) items, well under cap. The `default` branch can never trigger unless cap math is wrong.
  2. **Wrong error code if it ever did fire**: `ErrSeqTooOld` semantically means "fromSeq has been evicted from the replay buffer" — sending it for a buffer-overflow situation would mislead the caller into refetching state when in fact the bridge has a sizing bug. Compare to the parallel eventlog branch (`bridge.go:194-198`) which returns a distinct `fmt.Errorf("eventlog: replay overflow (cap=%d)", subscriberBufferSize)` — explicit, debuggable.
- **Severity**: LOW. Currently unreachable so no live damage; but the "wrong error" is a latent bug if cap constants ever drift.
- **Fix**: mirror the eventlog form: `return nil, nil, fmt.Errorf("notifications: replay overflow (cap=%d)", subscriberBufferSize)`. Even if you also delete the `default` branch as truly dead, this brings the two bridges to symmetric form.
- **Risk**: none.

---

## LOW-2 — `app/conversation/conversation.go:128-129` Delete publishes a `data` snapshot frontend never reads

- **Location**: `backend/internal/app/conversation/conversation.go:124-131`
  ```go
  func (s *Service) Delete(ctx context.Context, id string) error {
      if err := s.repo.Delete(ctx, id); err != nil { return err }
      s.notif.Publish(ctx, "conversation", id,
          map[string]any{"id": id, "deleted": true}, id)
      return nil
  }
  ```
- **Claims to do**: notify subscribers with a snapshot indicating the deletion intent (the `deleted: true` flag).
- **Reality**: looking at the only consumer side (`testend/js/chat.js:798-805` and `testend/js/tab-notifications.js:116-117`):
  - `chat.js` only reacts when `n.data.title` is truthy (autoTitle / rename) — so delete event is ignored.
  - `tab-notifications.js._summarizeSSE` for `case 'conversation'`: returns `(d.title || '(untitled)') + ' · ' + (n.id || '')`. Delete event has no title → renders `(untitled) · cv_xxx` in feed. The `deleted: true` flag is **never** read by any consumer.
- **Why it's dead**: the flag was likely added "for completeness" but no UI uses it. UIs derive deletion from REST list refetch, not the SSE flag.
- **Severity**: LOW. The notification still has value (it lands in tab-notifications feed as evidence of activity). Just the `data` shape is dead. Compare with mcp/mcp.go:326 which uses `{"deleted": true}` and `tab-notifications.js:122-123` which DOES check `if (d.deleted) return n.id + ' · deleted'` — there the flag is consumed. Conversation case wasn't given the same render branch.
- **Fix**: two options. (a) Trim `data` to `nil` (frontend already handles `d || {}`) — minimum dead-data; or (b) match the mcp_server pattern in tab-notifications.js so the flag becomes consumed (`case 'conversation': if (d.deleted) return n.id + ' · deleted'; return ...`). (b) is cleaner and adds user-visible value.
- **Risk**: none for option (a). For (b) the testend change is independent.

---

## LOW-3 — `pkg/notifications.Publisher.Publish` variadic conversationID is over-engineered

- **Location**: `backend/internal/pkg/notifications/notifications.go:37`, signature:
  ```go
  Publish(ctx context.Context, eventType, id string, data any, conversationID ...string)
  ```
  Implementation lines 61-77 only ever reads the first variadic element.
- **Claims to do**: variadic encoding of "optional conversationID" so callers can omit it for non-conv events (e.g. future `mcp_server`, `system_warning`).
- **Reality**: every caller in the codebase passes either zero or one variadic arg, never two+. There's no semantic for multiple conversationIDs. The variadic form is purely a Go syntax trick to make the parameter "optional" — but it has caused at least one bug:
  - `app/chat/runner.go:251` (autoTitle) calls `s.notifications.Publish(titleCtx, "conversation", conv.ID, conv)` — **omits** the 5th arg, so the resulting Event has `ConversationID == ""`. Compare to `app/conversation/conversation.go:69/117` which always passes `c.ID` as 5th arg. Two paths emit `type="conversation"` events with **inconsistent** ConversationID population.
  - This isn't immediately broken in practice because `tab-notifications.js` filters by `n.id`, not by `n.conversationId`. But anyone in the future who adds a per-conversation notification listener would see autoTitle events get dropped.
- **Why it's dead-as-designed**: the variadic shape cannot prevent the bug — it lets the 5th arg silently disappear at the call site.
- **Severity**: LOW. The cross-reference (chat/runner.go) is out of audit scope, so the bug itself isn't this audit's call. The variadic-shaped signature, however, is. It encourages the bug pattern.
- **Fix**: change to `Publish(ctx context.Context, eventType, id string, data any, conversationID string)` — make it required, callers without a conv pass `""`. This forces the chat/runner.go site to be explicit and surfaces the inconsistency. Also matches `notificationsdomain.Event{ConversationID string}` shape (explicit, optional via empty-string convention).
- **Risk**: low. Mechanical update — every call site needs a 5th arg. Compiler catches all.

---

## LOW-4 — `app/catalog.Service.versionMu` is redundant single-flight serialization

- **Location**: `backend/internal/app/catalog/catalog.go:110-111` (declaration), `:235-240` (only consumer `nextVersion`).
- **Claims to do**: serialize concurrent `nextVersion` calls (per its godoc).
- **Reality**: `nextVersion` is called from exactly one site (`polling.go:244`) inside `Refresh`, which is itself gated single-flight by `tryRefresh`'s `busy.CompareAndSwap` (polling.go:132-136). So at most one Refresh runs at a time → at most one `nextVersion` runs at a time. `Start` (polling.go:50-52) also writes `s.version` directly under `versionMu.Lock`, but Start completes before pollLoop is launched (line 75-81), so there's no race.
- **Why it's dead**: defensive over-locking. The `busy` flag is the actual single-flight; `versionMu` is duplicate.
- **Severity**: LOW. Cheap mutex, no performance impact, defensible as "future-proofing." But adds reader cost (have to verify it's not protecting something subtle).
- **Fix**: optional. If kept, document that it's defensive against future call-site additions. If removed, replace `nextVersion()` with a one-liner `s.version++` inline.
- **Risk**: low — would only matter if someone adds a second concurrent caller of `nextVersion`, at which point the absence of mu would be a bug. Keep with a comment, or remove with a `// callers serialized via tryRefresh.busy` note.

---

## EDGE-1 — skill `Scan` always rebuilds map even when fingerprint unchanged

- **Location**: `backend/internal/app/skill/scan.go:91-96`, lines `s.mu.Lock(); s.skills = loaded; s.mu.Unlock()` runs **before** the fingerprint short-circuit at line 105.
- **Claims**: Scan parses every SKILL.md, then "publishes notification only when the fingerprint actually changes" (godoc line 38-40, comment line 98-101).
- **Reality**: only the SSE publish is short-circuited — the in-memory cache is always replaced. Since `loaded` is built from the same disk content with no varied state, the new map should be byte-identical to the old one in the no-change case. The `s.mu.Lock` + `s.skills = loaded` swap on every tick is pure heat (allocation + lock ping). With ~10 skills this is microseconds; not a hot-path issue.
- **Severity**: EDGE. Minor allocation churn at 1Hz. Could short-circuit the cache swap when `last == fp` to save the allocation, but adds a code branch.
- **Fix**: optional. Move `s.mu.Lock(); s.skills = loaded; s.mu.Unlock()` inside the `if last != fp` block. Net: fewer allocations during quiet periods.
- **Risk**: low — would need a careful read to ensure no consumer relies on the cache being "freshly assigned" each tick (it shouldn't — Get/List return copies of the entries, not the map ptr).

---

## EDGE-2 — skill notification `data: {"changed": true, "count": N}` consumed only as a JSON-dump preview

- **Location**: `backend/internal/app/skill/scan.go:106-108`, the published shape `{"changed": true, "count": len(loaded)}`.
- **Reality**: testend tab-notifications.js:126-129 handles this case as `JSON.stringify(d).slice(0, 60)` — i.e. dumps the JSON without parsing it. The `changed` bool and `count` int aren't extracted into structured fields. The fact that the notif fires at all is the real signal; the data shape is decoration.
- **Severity**: EDGE. Not dead; not entirely alive either. The `count` is mildly informative in a JSON-dump preview ("changed: true, count: 7" reads OK); could remain.
- **Fix**: none necessary. If the testend gets a real "skills changed" UI in the future, this is the natural shape. Keep.

---

## EDGE-3 — `app/todo.Service.NewService` falls back via `From(Background())` instead of `New(nil, log)`

- **Location**: `backend/internal/app/todo/todo.go:50-52`:
  ```go
  if notifications == nil {
      notifications = notificationspkg.From(context.Background())
  }
  ```
- **Reality**: equivalent to `notificationspkg.New(nil, log)`, which is what `app/conversation/conversation.go:41-43` uses for the same fallback. Three of seven services (catalog/conversation/mcp/skill/sandbox) use `New(nil, log)`; chat and todo use `From(Background())`. Inconsistent style.
- **Severity**: EDGE. After MED-1 lands (delete `From` / `With` / `MustFrom`), this disappears automatically — both call sites become `New(nil, log)`. Don't fix in isolation; bundle with MED-1.
- **Fix**: rolled up with MED-1.
- **Risk**: none.

---

## Summary

`dead-6 完工，3 死逻辑（0 HIGH / 3 MED / 4 LOW + 3 EDGE）`

**Headline finding**: `pkg/notifications` was modeled on `pkg/eventlog` but inherited the ctx-wiring (With/From/MustFrom) without the use case — none of the 7 producer services need ctx-injected publishers. The whole second half of `pkg/notifications/notifications.go` (lines 79-112) is cargo-cult dead code (MED-1).

**Secondary finding**: both bridges have an unread `log` field and an unread `bufferedEnvelope.at` timestamp (MED-2 / MED-3) — vestigial wiring from "I'll add logging / time-eviction later" decisions that never landed.

**Cross-reference (out of audit scope)**: the variadic-conversationID signature in `pkg/notifications.Publisher.Publish` (LOW-3) is enabling a real bug at `app/chat/runner.go:251` where autoTitle drops the 5th arg, producing `Event.ConversationID == ""` while every other "conversation" event has it populated. Worth raising in a follow-up audit.

**Active and healthy**: both bridges' core fanout / replay / cancel logic; catalog polling (the `s.notif.Publish(ctx, "catalog", ...)` is consumed by chat.js + tab-notifications.js); skill activate (HTTP path's no-AgentState branch is intentional); todo CRUD; conversation Create/Update; the `<-s.done` race between Publish-fanout and cancel goroutine in eventlog/bridge.go is correct and necessary, not dead.
