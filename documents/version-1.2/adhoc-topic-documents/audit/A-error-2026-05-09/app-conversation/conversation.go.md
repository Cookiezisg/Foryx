# Audit trace: backend/internal/app/conversation/conversation.go

**Phase A — §S3 / §S9 / §S15 / §S16 / §S17**
**LOC**: 133 (incl. package godoc + blank lines).
**Scope**: Service methods (Create / List / Get / Rename / Update / Delete) + newID helper + NewService constructor.

## Sites

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | conversation.go:38-40 | `if log == nil { panic("conversation.NewService: logger is nil") }` | A.3 / A.5 | OK | Constructor invariant; nil logger = boot-time programmer error. Sibling pattern verified across `apikey.NewService`, `model.NewService`, `todo.NewService`. | N-A | — | — | — |
| 2 | conversation.go:41-43 | `if notif == nil { notif = notificationspkg.New(nil, log) }` | A.1 | OK | Documented nil-fallback to no-op publisher (`notificationspkg.New(nil, ...)` returns `noopPublisher{}` per notifications.go:46-49). NOT silent error swallow — it's the documented "publisher optional" wiring (`pkg/notifications` allows nil bridge). Constructor-only fallback, no err path involved. | N-A | — | — | — |
| 3 | conversation.go:51-54 | `uid, err := reqctxpkg.RequireUserID(ctx); if err != nil { return nil, fmt.Errorf("conversation.Service.Create: %w", err) }` | A.4 / A.5 | OK | `<pkg>.<Method>: %w` literal + `%w`. `reqctxpkg.ErrMissingUserID` cross-cutting registered errmap.go:185 → 500 INTERNAL_ERROR. Chain unwraps. | N-A | — | — | — |
| 4 | conversation.go:57 | `ID: newID()` | A.3 | OK | `newID()` = `idgenpkg.New("cv")` (line 133). `cv_` prefix matches CLAUDE.md §S15 conversation entry. idgen.New panics on rand.Read fail (idgen.go:21-23) — caller doesn't repeat. crypto/rand source. | N-A | — | — | — |
| 5 | conversation.go:63-65 | `if err := s.repo.Save(ctx, c); err != nil { return nil, err }` | A.4 / A.9 | OK | Pass-through; store-layer err already wrapped per §S16. ctx propagated. **NOT a §S9 detached-ctx site** — Create is foreground POST `/api/v1/conversations`; if browser cancels, aborting in-flight write is correct (no orphan post-cancel state). | N-A | — | — | — |
| 6 | conversation.go:66-68 | `s.log.Info("conversation created", zap.String("conversation_id", c.ID), zap.String("user_id", uid))` | A.1 | OK | Structured log; success path; not error. §S10-compliant. | N-A | — | — | — |
| 7 | conversation.go:69 | `s.notif.Publish(ctx, "conversation", c.ID, c, c.ID)` | A.1 / A.9 | OK | Best-effort notification — `pkg/notifications.publisher.Publish` internally swallows bridge err with `log.Warn` (notifications.go:71-76). NOT silent: failure is logged. NOT terminal-state — DB row already saved at line 63; notification is downstream broadcast. ctx pass-through correct (caller cancels = "user left, no need to notify them"). | N-A | — | — | — |
| 8 | conversation.go:77 | `func (s *Service) List(...) (...) { return s.repo.List(ctx, filter) }` | A.4 / A.9 | OK | Pass-through to store; read path; ctx propagated. Store wraps its own `<pkg>.<Method>:` per §S16. NOT §S9 (read, not terminal write). | N-A | — | — | — |
| 9 | conversation.go:84 | `func (s *Service) Get(...) (...) { return s.repo.Get(ctx, id) }` | A.4 / A.5 / A.9 | OK | Pass-through. Returns `convdomain.ErrNotFound` (registered errmap.go:58 → 404 CONVERSATION_NOT_FOUND) when row missing — already-known sentinel per prompt. ctx propagated; read path. | N-A | — | — | — |
| 10 | conversation.go:91 | `func (s *Service) Rename(...) (...) { return s.Update(ctx, id, &title, nil) }` | A.4 | OK | Thin wrapper delegating to Update; no err handling needed (Update's err-handling covers it). | N-A | — | — | — |
| 11 | conversation.go:103-106 | `c, err := s.repo.Get(ctx, id); if err != nil { return nil, err }` | A.4 | OK | Pass-through. Get returns `ErrNotFound` (registered errmap.go:58); other err already store-wrapped per §S16. | N-A | — | — | — |
| 12 | conversation.go:114-116 | `if err := s.repo.Save(ctx, c); err != nil { return nil, err }` | A.4 / A.9 | OK | Pass-through; store wraps. **NOT a §S9 detached-ctx site** — Update is foreground PATCH `/api/v1/conversations/{id}`; if caller cancels, aborting in-flight write is correct semantic. | N-A | — | — | — |
| 13 | conversation.go:117 | `s.notif.Publish(ctx, "conversation", c.ID, c, c.ID)` | A.1 / A.9 | OK | Best-effort, same as site #7. Not terminal-state (DB already written line 114). Not silent (publisher logs Warn on bridge err). | N-A | — | — | — |
| 14 | conversation.go:124-127 | `if err := s.repo.Delete(ctx, id); err != nil { return err }` | A.4 / A.9 | OK | Pass-through; store wraps. **NOT §S9** — Delete is foreground DELETE `/api/v1/conversations/{id}`; caller cancel → abort soft-delete is correct (no post-cancel state to preserve). | N-A | — | — | — |
| 15 | conversation.go:128-129 | `s.notif.Publish(ctx, "conversation", id, map[string]any{"id": id, "deleted": true}, id)` | A.1 / A.9 | OK | Best-effort post-delete broadcast; same semantics as sites #7 / #13. NOT terminal — soft-delete already in DB. Caller-ctx cancel is OK (notification just won't fire; row stays deleted, frontend will discover on next List). | N-A | — | — | — |
| 16 | conversation.go:133 | `func newID() string { return idgenpkg.New("cv") }` | A.3 | OK | Single-line wrapper; `cv_` prefix correct per §S15 + CLAUDE.md ID list. idgen panics on rand fail. | N-A | — | — | — |

## Sub-checks

**A.1 §S3 错误吞没:**
  - violations: not present
  - All err paths return wrapped err / pass-through-store-wrapped err / sentinel; no `_ = err`, no `if err != nil { return nil }`. Notification publishes (sites #7 / #13 / #15) are best-effort by design — `pkg/notifications.publisher` internally `log.Warn`s on bridge err (notifications.go:72-76), the err-fire-and-forget is documented behavior, not silent swallow. Constructor's nil-notif fallback (site #2) is documented (godoc line 33-36) and not in error path.

**A.2 §S9 detached ctx 终态写:**
  - terminal-state writes identified: site #5 (Create Save), site #12 (Update Save), site #14 (Delete)
  - 各自 ctx 来源: pass-through caller ctx (HTTP r.Context() upstream)
  - violations: not present (N/A semantics)
  - **Reasoning**: All 3 mutations are foreground user actions invoked from REST handlers (POST/PATCH/DELETE on `/api/v1/conversations[/{id}]`). If user closes tab mid-request, aborting the in-flight DB write is the **correct** semantic — there's no post-cancel state to preserve. Contrast §S9 typical violation: chat's "write final assistant message after stream cancelled" (host.go:54 `saveCtx := reqctxpkg.SetUserID(context.Background(), h.uid)`) and chat's autoTitle (runner.go:145 `go s.autoTitle(context.Background(), ...)`) — both true post-cancel-must-persist. **autoTitle owns the conversation rename** but lives in `app/chat`, NOT here. This file's Update/Rename only fires for foreground user PATCH; chat's autoTitle takes a separate detached-ctx path, doesn't go through this Service. Cross-fork verified by grep `autoTitle\|AutoTitle` in `app/chat/runner.go:217-246` — autoTitle calls `s.repo.Save(saveCtx, conv)` directly on the conversation repo from chat's Service, bypassing this file. Site #5/#12/#14's pass-through ctx is correct for foreground.
  - Notification publishes (sites #7 / #13 / #15) are NOT terminal-state — the DB write already happened on the previous line; notification is downstream broadcast. Publisher internally `log.Warn`s on bridge err. Pass-through ctx is correct.

**A.3 §S15 ID 生成:**
  - ID generation calls: `newID()` at site #4 (Create); `newID()` defined at site #16 = `idgenpkg.New("cv")`
  - violations: not present
  - Prefix `cv_` matches CLAUDE.md §S15 conversation entry. crypto/rand panic-on-fail enforced inside `idgen.New` (idgen.go:21-23). No self-rolled rand, no math/rand, no time-based IDs.

**A.4 §S16 错误 wrap 格式:**
  - violations: not present
  - 1 explicit wrap at site #3 with literal `conversation.Service.Create: %w`. 5 pass-throughs (sites #5, #8, #9, #11, #12, #14) correctly relay store layer's already-prefixed err. 0 `errors.New(... + err.Error())`, 0 `%v`, 0 unprefixed `fmt.Errorf("%w", err)`. errors.Is chain unwraps cleanly at every layer.

**A.5 §S17 sentinel 登记 errmap:**
  - sentinels defined: none (this file defines no sentinels — they live in `domain/conversation/conversation.go:43`)
  - 已登记 errmap (consumed sentinels reaching handler):
    - `convdomain.ErrNotFound` → errmap.go:58 (404 CONVERSATION_NOT_FOUND) — already-known per prompt
    - `reqctxpkg.ErrMissingUserID` → errmap.go:185 (500 INTERNAL_ERROR cross-cutting)
  - missing: all registered
  - **The single domain sentinel `convdomain.ErrNotFound` is registered (prompt-confirmed)**; it surfaces from Get / Update / Delete via store. Cross-cutting `reqctxpkg.ErrMissingUserID` (wrapped at site #3) — registered.
