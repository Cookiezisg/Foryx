# Package audit summary: internal/app/conversation

**Phase A — §S3 / §S9 / §S15 / §S16 / §S17**

## Spec understanding

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. This package returns or wraps every err it sees; no bare `_ = err`. Three notification publish sites (#7 / #13 / #15) are best-effort by design — `pkg/notifications.publisher.Publish` internally swallows bridge err with `log.Warn` (notifications.go:71-76); failure is logged + non-blocking, documented as the publisher's contract. The constructor's nil-notif fallback (site #2) is documented at godoc line 33-36 and isn't in an error path. No silent fallback at the service layer.
- **§S9 detached ctx 终态写**: terminal-state writes that MUST persist regardless of caller cancel use detached ctx. **N/A at this service**: all 3 mutations (Create / Update / Delete) are foreground user actions invoked from REST handlers (POST `/api/v1/conversations`, PATCH/DELETE `/api/v1/conversations/{id}`). If the caller cancels mid-request (browser close, network drop), aborting the in-flight DB write is the **correct** semantic — there's no post-cancel state to preserve. **autoTitle is in `app/chat`, NOT here** (verified `runner.go:145` `go s.autoTitle(context.Background(), ...)` + runner.go:164 `saveCtx := reqctxpkg.SetUserID(context.Background(), uid)`); chat's Service writes directly to the conversation repo from its detached-ctx path, bypassing this Service entirely. Pass-through ctx is correct at every site here.
- **§S15 ID 生成**: `<prefix>_<16hex>` via `idgenpkg.New("cv")`. Single-line wrapper at conversation.go:133 (`newID()`), called once at Create (line 57). Prefix `cv_` matches CLAUDE.md §S15 conversation entry. `idgenpkg.New` enforces panic-on-rand-fail at the implementation layer (idgen.go:21-23) — caller doesn't repeat. No self-rolled crypto, no math/rand, no time-based IDs.
- **§S16 错误 wrap 格式**: `fmt.Errorf("conversation.Service.Create: %w", err)` literal prefix + `%w` at the single explicit-wrap site (line 53). 6 pass-through sites correctly relay the store / repo's already-prefixed err (per §S16 example: store layer owns its own `<pkg>.<Method>:` prefix; service doesn't double-wrap). 0 `errors.New(... + err.Error())`, 0 `%v`, 0 unprefixed `fmt.Errorf("%w", err)`.
- **§S17 errmap 单一事实源**: every sentinel that can reach a handler must be in errmap.go. Package defines NO local sentinels. Consumes 2 cross-package sentinels — both verified registered:
  - `convdomain.ErrNotFound` → errmap.go:58 (404 CONVERSATION_NOT_FOUND) — already-known per prompt
  - `reqctxpkg.ErrMissingUserID` → errmap.go:185 (500 INTERNAL_ERROR — cross-cutting)

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| conversation.go | 133 | 16 | 16 | 0 | 0 | 0 |
| **TOTAL** | **133** | **16** | **16** | **0** | **0** | **0** |

## Severity breakdown

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 0 | — | — |
| LOW | 0 | — | — |

**Net: 0 violations / 0 EDGE.**

## Cross-cutting

### Sentinel chain integrity (§S17)

- **Defined locally**: none (zero local sentinels — package consumes only `convdomain` + `reqctxpkg`).
- **Consumed from `convdomain`**: 1 sentinel (ErrNotFound) — surfaces from Get / Update / Delete via store; registered.
- **Consumed from `reqctxpkg`**: ErrMissingUserID (wrapped at line 53) — registered as cross-cutting INTERNAL_ERROR.
- **No missing registrations.**

### Detached ctx coverage (§S9) — N/A at this layer (autoTitle lives in chat)

The 3 mutating methods (Create / Update / Delete) all use pass-through caller ctx, NOT detached ctx. This is the **correct** semantic for foreground REST CRUD:
- Each is invoked from `transport/httpapi` POST/PATCH/DELETE handler, runs synchronously inside the request lifecycle.
- If HTTP request cancels (browser close, network drop), the in-flight DB write should abort — there's no post-cancel state to preserve.

**Where IS the detached-ctx semantic in conversation flow?** In `app/chat`:
- `runner.go:144-145`: `if task.conv.Title == "" && !task.conv.AutoTitled { go s.autoTitle(context.Background(), task.conv, task.uid, result.LastMessage) }` — autoTitle fires post-stream-completion as a goroutine with a fresh `context.Background()`.
- `runner.go:164`: chat's stream-completion path uses `saveCtx := reqctxpkg.SetUserID(context.Background(), uid)` for assistant-message terminal writes.
- `runner.go:224-246`: `s.autoTitle(...)` calls `s.repo.Save(saveCtx, conv)` directly on the conversation repo from chat's Service — **bypasses this file's Service.Update**.

So the detached-ctx semantic is correctly placed in `app/chat` (where the post-stream-cancel terminal-write requirement lives); `app/conversation` itself is foreground-only and doesn't need detached ctx. This is a clean separation — the conversation package owns user-driven CRUD; chat's Service owns post-cancel autoTitle.

### Notification publish best-effort pattern (§S3 ✓)

3 notification sites (#7 Create, #13 Update, #15 Delete) call `s.notif.Publish(ctx, "conversation", ...)`. This pattern is best-effort by design:
- Inside `pkg/notifications.publisher.Publish` (notifications.go:61-77): if `bridge.Publish` returns err, the publisher logs Warn with type/id/error AND **does not return the err** (return type is void). The Publisher interface signature `Publish(ctx, eventType, id, data, conversationID...)` returns nothing.
- This is documented in `pkg/notifications` and matches the global notifications channel design (broadcast best-effort; consumers rebuild state via REST refetch on reconnect).
- NOT §S3 violation — the failure is logged structurally, not silently swallowed; the contract explicitly says "publish never blocks user action".

The constructor nil-notif fallback (site #2) `notificationspkg.New(nil, log)` returns `noopPublisher{}` (notifications.go:46-49); it's documented and matches the sibling pattern in `app/todo.NewService` (where `notif` is also nil-fallbacked).

### Style consistency cross-check vs sibling app/* services

Stylistically consistent with `app/apikey`, `app/forge`, `app/model`, `app/todo`:
- Constructor panic on nil logger (sibling pattern verified at `apikey.NewService`, `forge.NewService`, `model.NewService`, `todo.NewService`).
- Constructor accepts nil notifications publisher → no-op fallback (matches `todo.NewService` pattern).
- Single-line `newID() string { return idgenpkg.New("cv") }` matches `apikeyapp.newID() = idgenpkg.New("aki")`, `modelapp.newID() = idgenpkg.New("mc")`, `todoapp.newID() = idgenpkg.New("td")`.
- `<pkg>.<Method>: %w` wrap form at the cross-layer producer site (line 53).
- Pass-through err with no double-wrap at every store-edge site.
- No detached-ctx use cases — all mutations are foreground.

### Package structure (§S12 / §S13)

- **§S12**: `conversation.go` is the main file matching package name; package godoc at top covering all 3 layers' naming convention (line 1-7); single file at 133 LOC well under 500-line guideline. Compliant.
- **§S13**: Package declared `package conversation`; consumers alias as `convapp` per `<name><role>` rule with the canonical `conv` shortform (per CLAUDE.md §S13 example). Sibling `convdomain` (domain/conversation) and `convstore` (infra/store/conversation) follow same convention. Compliant.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

5 sites picked from `OK` set:

1. **Site #3** (Create RequireUserID wrap): verified — `fmt.Errorf("conversation.Service.Create: %w", err)` literal prefix + `%w`. Sentinel `reqctxpkg.ErrMissingUserID` registered errmap.go:185. errors.Is chain intact.
2. **Site #4** (newID call in Create): verified — `newID()` is `idgenpkg.New("cv")` (line 133); `cv_` matches CLAUDE.md S15 list. `idgenpkg.New` panics on rand fail (idgen.go:22-23). No double-check needed at call site per §S15.
3. **Site #7** (Create publish): verified — `s.notif.Publish(...)` is best-effort; `pkg/notifications/notifications.go:72-76` logs Warn on bridge err with `type` / `id` / `error` zap fields. NOT silent. NOT terminal-state (DB already saved at line 63). §S3 ✓.
4. **Site #9** (Get pass-through): verified — bare `s.repo.Get(ctx, id)` return; store layer owns its own `<pkg>.<Method>:` wrap per §S16 example; `convdomain.ErrNotFound` flows out registered errmap.go:58 → 404 CONVERSATION_NOT_FOUND. ctx propagated correctly per §S9 (read path, not terminal write).
5. **Site #16** (newID definition): verified — single-line. Prefix `cv` matches CLAUDE.md §S15 conversation entry. `idgenpkg.New` panics on rand fail (idgen.go:21-23). No 16-hex / no math/rand / no time-based ID.

All 5 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping.

## Recommended fix priorities

**None.** Package is clean across all 5 sub-checks. No HIGH / MED / LOW found. No action items.

If future expansion adds:
1. **A scheduled "auto-archive idle conversation" path** (e.g. cron job that soft-deletes 30-day-old convs), the cron worker should already use background ctx (not request-derived) — that's the natural background pattern, not the §S9 detached-ctx pattern.
2. **A "mark as read" or "last-seen-at" terminal-state update fired post-message-stream** that MUST persist after stream cancel, §S9 detached ctx will apply — adopt the chat-package pattern (`reqctxpkg.SetUserID(context.Background(), uid)`). Confirm the call site is in chat's runner not here.
3. **A new sentinel** in `convdomain` (e.g. `ErrTitleTooLong` for max-length validation), it must be added to errmap.go in the same commit per §S17.
4. **A multi-user / sharing model** (currently single-user per CLAUDE.md "项目特殊性"), Get / List / Update / Delete will need explicit `conv.UserID == uid` checks like `app/todo`'s cross-conversation defense pattern (translate ErrConversationMismatch → ErrNotFound to prevent existence leak). Currently the store presumably scopes by ctx user (verified via `reqctxpkg.RequireUserID` upstream + store layer); single-user simplification means no leak risk today.

## Out-of-scope notes (parent should verify if relevant)

1. **Store layer (`infra/store/conversation/...`)**: not audited in this fork. Sentinel registration coverage assumes store wraps with its own `<pkg>.<Method>: %w` prefix per §S16 (matches sibling stores' pattern); store-layer correctness requires separate audit.
2. **Domain layer (`domain/conversation/conversation.go`)**: not audited in this fork. Verified via grep that the single sentinel `ErrNotFound` exists (line 43); Repository interface presumably matches Service consumption.
3. **autoTitle in `app/chat/runner.go`**: not audited in this fork — referenced for §S9 N/A reasoning. Verified the call goes through chat's Service to the conversation repo directly, bypassing `app/conversation/Service.Update`. If a future refactor routes autoTitle through `Service.Update` (instead of repo direct), §S9 detached ctx semantics need to be added at the entry point — currently safe because chat already detaches before calling repo.
4. **Notification publish failure observability**: best-effort pattern means transient bridge errors only show up as `notifications.publisher` Warn logs. If autoTitle's Update succeeds in DB but the notification fails, frontend won't see the title change until manual refresh / List refetch. Documented contract; not a §S3 violation but worth tracking if user-facing autoTitle latency complaints arise.
5. **`conversation_test.go`**: per audit constraint, not read.
