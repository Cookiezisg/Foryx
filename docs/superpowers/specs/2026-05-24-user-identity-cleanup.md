# User Identity Cleanup — Design Spec

Date: 2026-05-24
Status: Draft for review
Author: brainstorming session w/ Claude

## 1. Problem

The constant `DefaultLocalUserID = "local-user"` is a magic string overloaded with **three structurally different responsibilities**, and the resulting fallback chains mask bugs and produce unfixable UX issues.

| Concern | What `local-user` currently does | What it *should* do |
|---|---|---|
| **A. Pre-onboarding placeholder** | Pretends a user exists so all endpoints work even without an onboarded user. | Show onboarding. No user, no work. |
| **B. HTTP fallback for missing / invalid `X-Forgify-User-ID`** | Silently demote to `local-user` (or first user) so request still succeeds. | 401. Frontend self-heals. |
| **C. Background-task ownership** (catalog polling, scheduler rehydrate, mcp calltool, skill exec log, trigger) | Detached `context.Background()` is stamped with `DefaultLocalUserID`. | Iterate actual users; if zero users, no-op. Detached code that needs a specific user must receive it explicitly. |

The user-visible symptom is the one that prompted this work: backend logs are flooded with `record not found ... WHERE id = "u_650f0a64333679fa"` because the frontend's `localStorage.activeUserId` references a user that no longer exists in the wiped DB, yet middleware silently demotes the request to first-user / `local-user` instead of telling the frontend to clear and re-onboard.

The deeper problem is that **all three concerns share one fallback chain**, so fixing any one symptom (clearing stale id, adding 401, etc.) leaves the other two broken.

## 2. Goals

1. **Eliminate the magic string.** No code path treats `"local-user"` specially. Delete `DefaultLocalUserID`.
2. **Frontend self-heals from stale state.** A wiped backend + surviving `localStorage` produces an onboarding prompt within one render cycle, with no silent demotion.
3. **Background tasks are explicit about which user(s) they act for.** No hidden "I'll just pretend to be local-user" fallbacks.
4. **Fresh install detection is structural.** `users.length === 0`, not username sniffing.
5. **Existing single-user installs continue to work.** A user who has been running Forgify with an onboarded user (whether `local-user` row or a `u_<hex>` row) sees no disruption.

## 3. Non-goals

- Real authentication (passwords, sessions, cookies). Local-first single-user remains the model; this spec only fixes the identity *plumbing*.
- Multi-tenant or multi-device sync.
- New CLI / API for user management. Existing `POST /users`, `GET /users` keep current shape.
- Cosmetic onboarding redesign. We only touch onboarding where logic must change.

## 4. Architecture

### 4.1 Three layers, three responsibilities

```
                      ┌─────────────────────────────┐
HTTP Request ──────►  │  IdentifyUser middleware    │  ← sets ctx.userID or nil
                      └─────────────┬───────────────┘
                                    │
                      ┌─────────────▼───────────────┐
                      │  RequireUser middleware     │  ← 401 if nil
                      └─────────────┬───────────────┘  ← skipped for /users, /health
                                    │
                                  Handler
                                    │
                                    ▼
                              app / domain
                                    │
                                    ▼
                          ┌─────────────────┐
                          │  store + db     │
                          └─────────────────┘

Background scheduler  ──►  for u := range users.List() { do(ctx, u.ID) }
                           ── no fallback, no magic id
```

### 4.2 Middleware (IdentifyUser + RequireUser)

`IdentifyUser` (always on):
1. Read `X-Forgify-User-ID` header; if absent, read `?userID=` query (SSE compatibility).
2. If id is empty → ctx.userID = nil, continue.
3. If id present → call `userResolver.Get(ctx, id)`:
   - Found → set ctx.userID = id, continue.
   - Not found → ctx.userID = nil, continue. **Critically: no fall-through to "first user".**

`RequireUser` (mounted on most routes):
- ctx.userID == nil → return `401 {"error":{"code":"UNAUTH_NO_USER","message":"no valid user identifier"}}`.
- ctx.userID != nil → continue.

Routes that **must skip** `RequireUser`:
- `GET /users`, `POST /users` — needed by onboarding before any user exists.
- Any liveness / readiness endpoint if present (verify during implementation; add if exists).

SSE routes (`/api/v1/eventlog`, `/api/v1/notifications`, `/api/v1/forge`) do **not** skip RequireUser — see §4.5 for client-side behaviour.

### 4.3 Background tasks

Audit each detached-context site found in the explore phase and rewrite:

| File | Current | New |
|---|---|---|
| `app/catalog/polling.go` | `ctx = reqctxpkg.SetUserID(ctx, DefaultLocalUserID)` | Fetch users; for each user spawn or stamp per-user ctx. Zero users → no-op tick. |
| `app/scheduler/rehydrate.go` | Same | Iterate users in `RehydrateOnBoot`. |
| `app/mcp/calltool.go` | Two fallback sites + HealthSnapshot stamped `DefaultLocalUserID` | Require caller to pass user id. `HealthSnapshot.UserID` must come from real caller. |
| `app/skill/exec_log.go` | Fallback to default when ctx missing | Require ctx user; if not present, return error (this is upstream's bug). |
| `app/trigger/trigger.go` | Owner fallback | Require explicit owner on workflow spec; reject creation without one. |

Pattern for iterate-users:
```go
// users.List does not require a per-user context — it reads the users table directly.
users, err := userSvc.List(context.Background())
if err != nil { return err }
for _, u := range users {
    uctx := reqctxpkg.SetUserID(context.Background(), u.ID)
    if err := doWork(uctx, u.ID); err != nil {
        log.Warn("background work failed for user",
            zap.String("user", u.ID), zap.Error(err))
        // don't bail the whole loop on one user's error
    }
}
```

### 4.4 Bootstrap

- `cmd/server/main.go`: delete the `userService.EnsureDefault(...)` call.
- `app/user/user.go`: delete the `EnsureDefault` function entirely. Also delete the test `user_test.go::TestEnsureDefault`.
- `pkg/reqctxpkg`: delete the `DefaultLocalUserID` constant. Leave `SetUserID` / `UserID` / `Local` helpers.

Fresh-boot behaviour: backend starts with empty `users` table. All requests except `/users` and `/health` return 401 until a user is created via `POST /users`.

### 4.5 SSE

EventSource native limitation: on 401 the connection closes and **does not auto-reconnect**. We must handle this client-side.

Server side:
- `/api/v1/eventlog`, `/api/v1/notifications`, `/api/v1/forge` go through `IdentifyUser + RequireUser` (same as REST).
- Connection without `?userID=` → 401, EventSource closes.
- Connection with stale `?userID=` → 401, EventSource closes.

Client side has exactly 4 states (`frontend/src/sse/SSEProvider.jsx` and `frontend/src/sse/shared.js`):

| Frontend state | EventSource behaviour |
|---|---|
| `activeUserId === null` (pre-onboarding or just self-healed) | **Do not create a connection.** Stay idle. Opening with no id would 401 instantly — pointless. |
| `activeUserId` set, just mounted | Create one EventSource with `?userID=<id>`. |
| `activeUserId` changed (account switch or self-heal) | Close old, open new bound to new id. The connection is bound to a user at handshake; can't switch mid-stream. |
| Connection drops while `activeUserId` still set (backend restart / network) | The `error` event with `readyState === CLOSED` fires. If the id we connected with still equals current `activeUserId` → treat as auth failure; call self-heal. Otherwise no-op (the state-change watcher will handle it). |

Implementation: `useEffect` watching `useSettings(s => s.activeUserId)`. On change, close existing EventSource; if new value non-null, build a new one.

Steady-state: SSE is held open indefinitely. The open/close transitions only happen at the 4 events above.

**REST 401 is the primary self-heal trigger** (most user actions are REST). SSE-close self-heal is the secondary path — for the case where a user opens a tab and just watches without clicking, then the backend restarts: SSE drops first.

### 4.6 Frontend self-heal

`frontend/src/store/settings.js`:
- `activeUserId: null` stays as default. Comment updated: "null → user must onboard or pick".

`frontend/src/App.jsx`:
- Existing `useQuery({ queryKey: qk.users(), queryFn: () => apiFetch("/users") })` already runs at root.
- Add `useEffect`: when `usersQ.data` resolves, check `settings.activeUserId` against the returned list. If activeUserId is set but not in the list → `settings.set({ activeUserId: null })`.
- Fresh-install detection becomes: `!settings.activeUserId && (usersQ.data?.length ?? 0) === 0`. Onboarding overlay shows.
- If there are users but `activeUserId` is null → show a user picker (or auto-select the first if only one exists; see §5).

`frontend/src/api/client.js`:
- `apiFetch` on `res.status === 401` (specifically code `UNAUTH_NO_USER`):
  - Call `useSettings.getState().set({ activeUserId: null })`.
  - Call `queryClient.invalidateQueries({ queryKey: qk.users() })` so App.jsx re-renders into onboarding.
  - Throw the `ApiError` as usual so the caller knows the request failed.

## 5. Migration

Existing user data is **not** deleted. After upgrading to the new code:

1. If DB has any `users` row → those users remain. The literal `"local-user"` row (if present) is now a regular user. No special handling.
2. If `localStorage.activeUserId` matches a real user → unchanged, app works.
3. If `localStorage.activeUserId` references a stale id (the user-visible bug) → on next App.jsx render the self-heal effect clears it; user sees onboarding (or a one-user auto-select if exactly one onboarded user exists; see below).
4. If `localStorage.activeUserId` is null but DB has users → frontend auto-selects the only user if `users.length === 1`, else shows a picker.

No migration scripts. No data rewrites. The semantics change but the data shape doesn't.

**Auto-select rule** — when `activeUserId === null` (fresh boot OR just self-healed OR new browser), the UI's behaviour is dispatched by `users.length`:

| `users.length` | What UI does | Why |
|---|---|---|
| 0 | Show Onboarding overlay | No users exist; we need one |
| **1** | **Auto-select that user** (`settings.set({ activeUserId: users[0].id })`) | Forcing a picker or re-onboarding for the only existing user is dumb. Single-user is the common case; this is the no-friction path. |
| ≥2 | Show user picker | Genuine ambiguity; user must choose |

The auto-select branch is what makes the migration in §5 step 4 invisible to the dominant single-user case: localStorage gets cleared (browser data wipe, profile switch, our self-heal) → next render sees one user → silently picks them → user notices nothing.

## 6. Test fixture cleanup

15+ Go test files use the literal `"local-user"` as a fixture id. After this change, the string has no special meaning, so the simplest fix is **mechanical replacement** to `"test-user"` and switch them to use `harness.SeedUser(t, ...)` (or create the local helper if missing) so each test seeds its own user explicitly.

Files (from explore):
- `backend/test/document/document_test.go`
- `backend/test/document/workflow_attach_test.go`
- `backend/test/scheduler/approval_e2e_test.go`
- `backend/test/scheduler/scheduler_test.go`
- `backend/test/workflow/workflow_test.go`
- `backend/test/catalog/trinity_catalog_test.go`
- `backend/internal/transport/httpapi/middleware/auth_test.go` — rewrite to test the new semantics (401 on unknown, IdentifyUser/RequireUser layered behaviour)
- `backend/internal/app/user/user_test.go` — drop `TestEnsureDefault`, keep CRUD tests
- `backend/internal/domain/document/document_test.go`
- `backend/internal/infra/store/document/document_test.go`
- `backend/internal/pkg/userpath/userpath_test.go` (legacy migration tests; keep as-is — they test path conventions, not identity logic)
- `backend/internal/app/document/document_test.go`
- `backend/internal/app/tool/document/document_test.go`
- `backend/test/harness/seed.go` — `LocalCtx()` helper: rename to `SeedCtx(t)` returning ctx stamped with a freshly created user.

## 7. Documentation updates

Files that mention `local-user` or `DefaultLocalUserID` or the fallback chain and must be updated to match new semantics:

- `documents/version-1.2/backend-design.md` — section on Phase 2 multi-user
- `documents/version-1.2/service-design-documents/user.md`
- `documents/version-1.2/service-design-documents/catalog.md`
- `documents/version-1.2/service-design-documents/trigger.md`
- (any other service-design doc that mentions "default user" / `local-user`)
- `documents/version-1.2/frontend-prd.md` — §17 endpoints + §19 multi-user
- `documents/version-1.2/progress-record.md` — add a Phase entry describing the cleanup
- `CLAUDE.md` — S15 ID prefix table currently lacks a prefix for users; either add `u_` or note it explicitly

## 8. Verification matrix

Each must pass before merge:

| # | Scenario | Expected | How |
|---|---|---|---|
| 1 | Fresh install (empty DB) | Onboarding shows; `POST /users` works; all other routes 401 | Manual: rm `~/.forgify/forgify.db`, boot, open browser |
| 2 | DB wipe + localStorage kept (the original bug) | Within one render, `activeUserId` cleared; onboarding shows | Manual repro |
| 3 | Single onboarded user | App works normally; no picker | Manual |
| 4 | Two users, switch then delete the non-active one | No crash; active session continues | Manual |
| 5 | Two users, delete the active one | Self-heal: clears activeUserId; if other user exists, auto-select; else onboarding | Manual |
| 6 | Background catalog polling on 0 users | No errors, no log spam, no work | Boot fresh, watch logs for 30s |
| 7 | Background catalog polling on 2 users | Both refreshed each tick | Manual: seed 2 users, watch logs |
| 8 | SSE with stale `?userID=` | 401, EventSource closes, frontend triggers self-heal | curl test + browser repro |
| 9 | SSE reconnect after activeUserId change | New EventSource opens with new id, old closes | Browser devtools network tab |
| 10 | `make test-unit` | All green | CI |
| 11 | `make test-pipeline` (env-gated tests) | All green or skip cleanly | CI |
| 12 | `staticcheck ./...` | Clean | CI |

## 9. Rollback

If anything in §8 fails post-merge:

- Single revert of the integration commit returns to current behaviour (the only data-touching change is "stop calling `EnsureDefault`", which is forward-compatible — existing rows remain).
- No DB migration to undo.
- Frontend localStorage may end up cleared, but that just makes users re-onboard — not a data loss.

## 10. Out of scope (explicit)

- Renaming `users` table or any column.
- Audit log of who-did-what (still attributed to whatever user id stamped the action).
- Per-user encryption keys.
- Session expiry / token rotation.
- Web-multi-tab synchronization (`localStorage` change events) — nice-to-have but not required for this fix.

## 11. Open questions resolved during brainstorming

- **Should `POST /users` require auth?** No. Onboarding must work before any user exists. Once we have real auth, we'd gate this behind admin permission — out of scope.
- **Should we treat the existing `local-user` row as "the user" or as data?** Data. It just stays. The string has no special meaning anymore.
- **Should background tasks run when 0 users?** No. Skip tick silently.
- **Should middleware return 401 or 200+empty on missing header?** 401 with code `UNAUTH_NO_USER`. The empty case is "I called the wrong endpoint" — be explicit.
- **Should `IdentifyUser` + `RequireUser` be one middleware or two?** Two, because `/users` needs Identify but not Require. Keeping them separate makes the route table self-documenting.
