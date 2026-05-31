# Audit: backend/internal/app/todo/todo.go

**LOC**: 252
**Package**: `package todo` (alias `todoapp` per §S13)
**Role**: Service layer for LLM todo tool family — owns CRUD, conversation scoping, ID minting, SSE notification publication.

---

## 9-column Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | todo.go:48 | ``if log == nil { panic("todo.NewService: logger is nil") }`` (lines 47-49) | A.1 | OK | §S3 not applicable — wiring-time panic on nil logger surfaces a constructor misuse loudly, opposite of swallowing. Matches `apikey/forge/...` constructor pattern. | N-A | — | — | — |
| 2 | todo.go:50-52 | ``if notifications == nil { notifications = notificationspkg.From(context.Background()) }`` | A.1 | OK | §S3 not applicable — explicit no-op fallback by design (`notificationspkg.From` returns no-op publisher when ctx empty per pkg comment); not a silent error path. Documented in godoc line 42-45. | N-A | — | — | — |
| 3 | todo.go:88-91 | Create: ``convID, ok := reqctxpkg.GetConversationID(ctx); if !ok || convID == "" { return nil, fmt.Errorf("todo.Service.Create: %w", reqctxpkg.ErrMissingConversationID) }`` | A.4/A.5 | OK | §S16 ✓: `<pkg>.<Method>:` literal prefix + `%w`. §S17 ✓: `reqctxpkg.ErrMissingConversationID` is registered errmap.go:186 (cross-cutting). errors.Is chain intact through framework → handler. | N-A | — | — | — |
| 4 | todo.go:92-95 | Create subject validation: ``subject := strings.TrimSpace(in.Subject); if subject == "" { return nil, tododomain.ErrSubjectRequired }`` | A.5 | OK | §S17 ✓: `tododomain.ErrSubjectRequired` registered errmap.go:98 → 400 TODO_SUBJECT_REQUIRED. Sentinel returned at innermost layer (no wrap needed per §S16 example). | N-A | — | — | — |
| 5 | todo.go:97 | ``ID: newID()`` (in Create) | A.3 | OK | §S15 ✓: delegates to `newID()` (line 252) which is `idgenpkg.New("td")` — `td_` prefix matches CLAUDE.md ID prefix list (`td_` for conversation-level TODO, renamed from `tk_` 2026-05-05). Panic-on-rand-fail enforced inside `idgenpkg.New` (idgen.go:21-23). No self-rolled crypto. | N-A | — | — | — |
| 6 | todo.go:106-108 | ``if err := s.repo.Create(ctx, t); err != nil { return nil, err }`` | A.4 | OK | §S16: bare passthrough is acceptable — the store layer (per §S16 example: `apikeystore.List: %w`) is responsible for adding its own `<pkg>.<Method>:` prefix. Service relays without re-wrap because store err is already prefixed. errors.Is chain intact for `tododomain.ErrNotFound` etc. (Same pattern as `apikeyapp/Service.Get`.) | N-A | — | — | — |
| 7 | todo.go:109 | ``s.publish(ctx, t)`` (after successful Create) | A.1/A.2 | OK | §S3 ✓: `publish` (line 248-250) calls `notifications.Publish` which is documented as best-effort — failure goes to `p.log.Warn` inside `pkg/notifications` (notifications.go:72-76); not silently dropped. §S9 N/A: notifications are observability not terminal-state (godoc line 28-37 explicit). | N-A | — | — | — |
| 8 | todo.go:106 | ``s.repo.Create(ctx, t)`` — ctx source | A.2 | OK | §S9: `ctx` is pass-through caller ctx. **Not a §S9 terminal write violation** because there is no "post-cancel must-persist" semantic for Create — if caller cancels mid-create, aborting the insert is correct (no user-visible partial state). User receives normal err response; UI shows the operation didn't complete. (Mirrors app-tool-todo summary §S9 verdict.) | N-A | — | — | — |
| 9 | todo.go:124-128 | Get: ``convID, ok := reqctxpkg.GetConversationID(ctx); if !ok || convID == "" { return nil, fmt.Errorf("todo.Service.Get: %w", reqctxpkg.ErrMissingConversationID) }`` | A.4/A.5 | OK | §S16 ✓ + §S17 ✓ — same as site #3 with `Get` method tag. | N-A | — | — | — |
| 10 | todo.go:129-132 | ``t, err := s.repo.Get(ctx, id); if err != nil { return nil, err }`` | A.4 | OK | §S16: passthrough — same reasoning as site #6. `tododomain.ErrNotFound` from store unwraps cleanly. | N-A | — | — | — |
| 11 | todo.go:133-135 | ``if t.ConversationID != convID { return nil, tododomain.ErrNotFound }`` | A.1/A.5 | OK | §S3 deliberate **anti-leak translation**: cross-conversation mismatch returned as ErrNotFound to prevent existence leak (godoc line 117-120 explicit). §S17: ErrNotFound registered errmap.go:97. **NOT** error-swallowing — caller still gets a 404 + audit chain via store's err if the row truly didn't exist. The translation is the correct security semantic, not §S3 violation. | N-A | — | — | — |
| 12 | todo.go:142-148 | List: same pattern (require convID → return repo err) | A.4/A.5 | OK | §S16 ✓ + §S17 ✓ — same prefix style; no body validation needed (List has no input). | N-A | — | — | — |
| 13 | todo.go:156-160 | Update: convID gate (same as Create / Get) | A.4/A.5 | OK | §S16 ✓ + §S17 ✓. | N-A | — | — | — |
| 14 | todo.go:161-163 | ``t, err := s.repo.Get(ctx, id); if err != nil { return nil, err }`` | A.4 | OK | §S16 passthrough — same reasoning. | N-A | — | — | — |
| 15 | todo.go:165-169 | ``if t.ConversationID != convID { return nil, tododomain.ErrNotFound }`` (with comment "Same not-found semantics as Get") | A.1/A.5 | OK | Same anti-leak translation as site #11. Comment explicitly cross-references Get's semantics. Not §S3. | N-A | — | — | — |
| 16 | todo.go:170-176 | Update Subject branch: ``if in.Subject != nil { subject := strings.TrimSpace(*in.Subject); if subject == "" { return nil, tododomain.ErrSubjectRequired }; t.Subject = subject }`` | A.5 | OK | §S17 ✓ — same as site #4. | N-A | — | — | — |
| 17 | todo.go:183-188 | Update Status branch: ``if in.Status != nil { if !tododomain.IsValidStatus(*in.Status) { return nil, tododomain.ErrInvalidStatus }; t.Status = *in.Status }`` | A.5 | OK | §S17 ✓: `tododomain.ErrInvalidStatus` registered errmap.go:99 → 400 TODO_INVALID_STATUS. | N-A | — | — | — |
| 18 | todo.go:198-200 | ``if err := s.repo.Update(ctx, t); err != nil { return nil, err }`` | A.4 | OK | §S16 passthrough — same reasoning as site #6. | N-A | — | — | — |
| 19 | todo.go:198 | ``s.repo.Update(ctx, t)`` — ctx source | A.2 | OK | §S9: pass-through ctx, NOT a terminal-state violation. Update is a regular mutation step in an LLM turn; if caller cancels mid-update, aborting the write is the correct semantic (user resends if needed; no chat-final-message-after-cancel terminal semantic to preserve). Cross-fork confirmed: no detached ctx required. | N-A | — | — | — |
| 20 | todo.go:201 | ``s.publish(ctx, t)`` after Update | A.1/A.2 | OK | Same observability semantics as site #7. | N-A | — | — | — |
| 21 | todo.go:215-219 | Delete: convID gate | A.4/A.5 | OK | §S16 ✓ + §S17 ✓. | N-A | — | — | — |
| 22 | todo.go:220-223 | Delete: ``t, err := s.repo.Get(ctx, id); if err != nil { return err }`` | A.4 | OK | §S16 passthrough. | N-A | — | — | — |
| 23 | todo.go:224-226 | ``if t.ConversationID != convID { return tododomain.ErrNotFound }`` | A.1/A.5 | OK | Same anti-leak translation as sites #11/#15. | N-A | — | — | — |
| 24 | todo.go:227-229 | ``if err := s.repo.SoftDelete(ctx, id); err != nil { return err }`` | A.4 | OK | §S16 passthrough. | N-A | — | — | — |
| 25 | todo.go:227 | ``s.repo.SoftDelete(ctx, id)`` — ctx source | A.2 | OK | §S9: pass-through ctx. Same analysis as Update site #19 — Delete in todo flow is mid-LLM-turn step, not terminal. If caller cancels mid-delete, the row stays (transactional unit fails). User-visible state remains the pre-delete row, which is consistent with what user saw before the cancelled call. No detached ctx required. | N-A | — | — | — |
| 26 | todo.go:235 | ``s.publish(ctx, t)`` after Delete (post stamping `t.Status = StatusDeleted`) | A.1/A.2 | OK | Same observability semantics as site #7. **Note**: this publish is post-success, so even if ctx is cancelled here, the DB row is already soft-deleted; the notification merely doesn't reach UI subscribers — best-effort by design (notifications.go:72-76 logs Warn). UI eventually catches up via next poll/reconnect. | N-A | — | — | — |
| 27 | todo.go:248-250 | ``func (s *Service) publish(ctx context.Context, t *tododomain.Todo) { s.notifications.Publish(ctx, "todo", t.ID, t, t.ConversationID) }`` | A.1 | OK | §S3 ✓: `notifications.Publish` is best-effort, logs Warn on failure (verified pkg/notifications/notifications.go:72-76). Not silent. | N-A | — | — | — |
| 28 | todo.go:252 | ``func newID() string { return idgenpkg.New("td") }`` | A.3 | OK | §S15 ✓: `td_` prefix, `idgenpkg.New` panics on rand fail (idgen.go:21-23). 1-line wrapper for grep convenience; consistent with sibling packages (apikey/forge/conv etc.). | N-A | — | — | — |

**Total sites: 28 — 0 VIOLATION, 0 EDGE.**

---

## Sub-check Summary (per spec template)

### A.1 §S3 错误吞没
- **violations**: not present.
- All `err != nil` branches return the error or wrap with `%w`. No bare `_ = err`. The cross-conversation `ErrConversationMismatch → ErrNotFound` translation (sites #11, #15, #23) is intentional anti-existence-leak, not error-swallowing — caller still receives a 404 with audit trail. Notification publish is best-effort by documented design (notifications.go logs Warn on failure).

### A.2 §S9 detached ctx 终态写
- **terminal-state writes identified**: none (mid-LLM-turn writes only).
- **各自 ctx 来源**: pass-through caller ctx for `repo.Create` (site #8), `repo.Update` (site #19), `repo.SoftDelete` (site #25).
- **violations**: **N/A: package doesn't do terminal writes**. todo CRUD is invoked synchronously from LLM tool turn (TodoCreate / TodoUpdate / TodoDelete in `app/tool/todo`); if the caller (LLM-driven tool runner) cancels, aborting the mutation is the **correct** semantic — there's no post-cancel persistent state to preserve (unlike chat's "write final assistant message after stream cancelled"). Cross-fork verified against `app-tool-todo/_summary.md` §S9 N/A verdict: same conclusion holds at this layer.

### A.3 §S15 ID 生成
- **ID generation calls**: `newID() → idgenpkg.New("td")` at todo.go:252, called once at todo.go:97 (Create).
- **violations**: not present. Prefix `td_` matches CLAUDE.md ID convention (renamed 2026-05-05 from `tk_`). `idgenpkg.New` enforces panic-on-rand-fail (idgen.go:21-23) and 16-hex format. No self-rolled crypto, no math/rand, no time-based IDs.

### A.4 §S16 错误 wrap 格式
- **violations**: not present.
- 5 explicit wrap sites (sites #3, #9, #12, #13, #21) all use the canonical `fmt.Errorf("todo.Service.<Method>: %w", err)` form with `%w` (never `%v`/`%s`). 6 store-passthrough sites (#6, #10, #14, #18, #22, #24) correctly relay the store's already-prefixed err (per CLAUDE.md §S16 example pattern: store layer adds its own prefix, service doesn't double-wrap). 4 sentinel-direct returns (#4, #11, #15, #16, #17, #23) correctly skip wrap at the innermost producer per §S16 example `return apikeydomain.ErrNotFound`.

### A.5 §S17 sentinel 登记 errmap
- **sentinels defined**: none locally (this package defines no `var Err...`).
- **sentinels consumed** (cross-package, reach handler via FromDomainError):
  - `tododomain.ErrNotFound` → registered errmap.go:97 (`TODO_NOT_FOUND`, 404). Used at sites #11, #15, #23.
  - `tododomain.ErrSubjectRequired` → registered errmap.go:98 (`TODO_SUBJECT_REQUIRED`, 400). Used at sites #4, #16.
  - `tododomain.ErrInvalidStatus` → registered errmap.go:99 (`TODO_INVALID_STATUS`, 400). Used at site #17.
  - `reqctxpkg.ErrMissingConversationID` → registered errmap.go:186 (cross-cutting INTERNAL_ERROR 500). Used at sites #3, #9, #12, #13, #21.
- **all registered**.
- **intentionally absent**: `tododomain.ErrConversationMismatch` (4th sentinel, defined domain/todo/todo.go:74) — Service NEVER surfaces it (translates to ErrNotFound at sites #11, #15, #23 to prevent cross-conversation existence leak per godoc line 117-120). Correctly absent from errmap because it never reaches a handler. Cross-fork confirmed against app-tool-todo/_summary.md §S17 statement.

---

## Verdict

**0 HIGH / 0 MED / 0 LOW. Package is §S3/S9/S15/S16/S17 textbook-clean at the service layer.**

Strengths:
- Every wrap call uses `<pkg>.<Method>: %w` literal prefix; passthrough sites correctly relay store's prefix without double-wrap.
- Anti-existence-leak translation (`ErrConversationMismatch → ErrNotFound`) explicitly documented in godoc; not flagged as §S3 violation.
- ID generation delegates to `idgenpkg.New("td")` — single-line wrapper, no entropy concerns, prefix matches CLAUDE.md S15 list.
- Notification publish is best-effort by documented design; failures log Warn (not silent).
- §S9 not applicable: all writes are mid-LLM-turn, not terminal-state. Pass-through ctx is the correct semantic.
