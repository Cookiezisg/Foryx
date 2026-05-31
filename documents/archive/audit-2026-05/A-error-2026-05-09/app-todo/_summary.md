# Package audit summary: internal/app/todo

**Phase A — §S3 / §S9 / §S15 / §S16 / §S17**

## Spec understanding

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. This package returns or wraps every err it sees; no bare `_ = err`. The cross-conversation `tododomain.ErrConversationMismatch → tododomain.ErrNotFound` translation (3 sites) is intentional anti-existence-leak (godoc line 117-120 explicit), NOT error swallowing — the caller still receives a registered 404 sentinel. Notification publish is documented best-effort and logs Warn on failure (`pkg/notifications` notifications.go:72-76).
- **§S9 detached ctx 终态写**: terminal-state writes that MUST persist regardless of caller cancel use detached ctx. **N/A at this service layer**: all 3 mutating methods (Create / Update / Delete) run synchronously inside an LLM tool call; if the LLM-driven caller cancels, aborting the in-flight mutation is the **correct** semantic (no post-cancel state to preserve, unlike chat's "write final assistant message after stream cancelled"). Pass-through ctx is correct. Cross-fork verified against `app-tool-todo/_summary.md` §S9 N/A verdict — same conclusion holds at the service layer.
- **§S15 ID 生成**: `<prefix>_<16hex>` via `idgenpkg.New("td")`. Single-line wrapper at todo.go:252 (`newID()`), called once at Create (line 97). Prefix `td_` matches CLAUDE.md ID list (renamed 2026-05-05 from `tk_`). `idgenpkg.New` enforces panic-on-rand-fail at the implementation layer (idgen.go:21-23) — caller doesn't repeat the check. No self-rolled crypto, no math/rand, no time-based IDs.
- **§S16 错误 wrap 格式**: `fmt.Errorf("todo.Service.<Method>: %w", err)` literal prefix + `%w` at all 5 explicit-wrap sites. 6 store-passthrough sites correctly relay the store's already-prefixed err (per §S16 example: store layer owns its own `<pkg>.<Method>:` prefix; service doesn't double-wrap). 5 sentinel-direct returns correctly skip wrap at the innermost producer.
- **§S17 errmap 单一事实源**: every sentinel that can reach a handler must be in errmap.go. Package defines NO local sentinels. Consumes 4 cross-package sentinels — all 4 verified registered:
  - `tododomain.ErrNotFound` → errmap.go:97 (404 TODO_NOT_FOUND)
  - `tododomain.ErrSubjectRequired` → errmap.go:98 (400 TODO_SUBJECT_REQUIRED)
  - `tododomain.ErrInvalidStatus` → errmap.go:99 (400 TODO_INVALID_STATUS)
  - `reqctxpkg.ErrMissingConversationID` → errmap.go:186 (500 INTERNAL_ERROR — cross-cutting)
  - `tododomain.ErrConversationMismatch` (4th defined sentinel, domain/todo/todo.go:74) — **intentionally never surfaced** by Service (translated to `ErrNotFound` at the cross-conversation defense sites to prevent existence leak); correctly absent from errmap because it never reaches a handler.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| todo.go | 252 | 28 | 28 | 0 | 0 | 0 |
| **TOTAL** | **252** | **28** | **28** | **0** | **0** | **0** |

## Severity breakdown

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 0 | — | — |
| LOW | 0 | — | — |

**Net: 0 violations / 0 EDGE.**

## Cross-cutting

### Sentinel chain integrity (§S17)

- **Defined locally**: none (zero local sentinels — package consumes only `tododomain` + `reqctxpkg` sentinels).
- **Consumed from `tododomain`**: 3 of 4 sentinels reach handlers — all registered. 4th (ErrConversationMismatch) is intentionally never surfaced.
- **Consumed from `reqctxpkg`**: ErrMissingConversationID — registered as cross-cutting INTERNAL_ERROR.
- **No missing registrations.**

### Cross-conversation existence-leak defense (security note)

3 sites (#11 Get, #15 Update, #23 Delete) translate row-level `ConversationID != ctx convID` → `tododomain.ErrNotFound`. This is the **intentional security semantic** — without translation, `ErrConversationMismatch` as a distinct sentinel would let a curious caller distinguish "this todo doesn't exist" from "this todo exists but is in a different conversation", revealing existence across conversation boundaries. Godoc line 117-120 makes this explicit ("we don't want to leak existence across conversations"); cross-referenced at line 166 in Update with comment "Same not-found semantics as Get". Both `ErrNotFound` returns are §S17-clean (registered errmap.go:97).

### Detached ctx coverage (§S9) — N/A at this layer

All 3 mutation paths (Create / Update / Delete) use pass-through caller ctx, NOT detached ctx. This is the **correct** semantic for tool-driven CRUD:
- todo CRUD is invoked from `app/tool/todo` tools, which run synchronously inside an LLM turn.
- If the LLM caller (or upstream HTTP request) cancels mid-call, the in-flight DB write should abort — there's no post-cancel state to preserve.
- Contrast with chat's "write assistant final message after stream cancelled" (true terminal-state) → that needs detached ctx via `reqctxpkg.SetUserID(context.Background(), uid)`. todo has no analogous post-cancel-must-persist path.

The post-success `s.publish(ctx, t)` at sites #7 / #20 / #26 also uses pass-through ctx — best-effort by design (publish failure logs Warn in `pkg/notifications`, never blocks the success path). Even if ctx is cancelled at publish time, the DB is already written.

### Style consistency cross-check vs sibling app/* services

Stylistically consistent with `app/apikey`, `app/forge`, `app/conv`:
- Constructor panic on nil logger (sibling pattern verified at `apikey.NewService`, `forge.NewService` etc.).
- Constructor accepts nil notifications publisher → no-op fallback (`notificationspkg.From(context.Background())` at line 51).
- Single-line `newID() string { return idgenpkg.New("td") }` matches `apikeyapp.newID() = idgenpkg.New("aki")` etc.
- `<pkg>.<Method>: %w` wrap form at every cross-layer producer site.
- Sentinel-direct return at innermost validator (no wrap of bare sentinel).
- Cross-conversation defense uses `ErrNotFound` translation (security pattern; documented).

### Package structure (§S12 / §S13)

- **§S12**: `todo.go` is the main file matching package name; package godoc at top; single file at 252 LOC well under 500-line guideline. Compliant.
- **§S13**: Package declared `package todo`; consumers alias as `todoapp` per nested `<name><role>` rule (verified `app/tool/todo` imports use `todoapp`). Sibling `tododomain` (domain/todo) and `todostore` (infra/store/todo) follow same convention. Compliant.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

5 sites picked from `OK` set:

1. **Site #3** (Create convID gate): verified — `fmt.Errorf("todo.Service.Create: %w", reqctxpkg.ErrMissingConversationID)` literal prefix + `%w`. Sentinel registered errmap.go:186. errors.Is chain intact.
2. **Site #5** (newID call): verified — `newID()` is `idgenpkg.New("td")` (line 252); `td_` matches CLAUDE.md S15 list. `idgenpkg.New` panics on rand fail (idgen.go:22-23). No double-check needed at call site per §S15.
3. **Site #11** (Get cross-conv defense): verified — `if t.ConversationID != convID { return nil, tododomain.ErrNotFound }`. Godoc line 117-120 explicit about leak prevention. Returned sentinel is registered. NOT §S3 (caller still gets a registered 404).
4. **Site #17** (Update Status branch): verified — `if !tododomain.IsValidStatus(*in.Status) { return nil, tododomain.ErrInvalidStatus }`. Sentinel registered errmap.go:99 → 400 TODO_INVALID_STATUS. `IsValidStatus` is the canonical whitelist check (4 statuses per domain/todo/todo.go:50-57).
5. **Site #27** (publish helper): verified — `s.notifications.Publish(...)` is best-effort; notification.go:72-76 logs Warn on failure with type/id/error. NOT silent. §S3 ✓.

All 5 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping.

## Recommended fix priorities

**None.** Package is clean across all 5 sub-checks. No HIGH / MED / LOW found. No action items.

If future expansion adds:
1. **Post-cancel terminal-state writes** (e.g. "mark todo as auto-completed when conversation closes"), §S9 detached ctx will apply — adopt the chat-package pattern (`reqctxpkg.SetUserID(context.Background(), uid)`).
2. **A scenario where ErrConversationMismatch must surface** (e.g. internal admin tool that needs to distinguish "exists elsewhere" vs "doesn't exist"), register the sentinel in errmap.go and remove the translation — but only after carefully assessing the existence-leak risk.
3. **A new sentinel** in `tododomain` (e.g. `ErrBlockedByCycle` for circular dependencies), it must be added to errmap.go in the same commit per §S17.

## Out-of-scope notes (parent should verify if relevant)

1. **Store layer (`infra/store/todo/todo.go`)**: not audited in this fork. Spot-grep at todo.go header (file path: `infra/store/todo/todo.go:6,14`) confirms it documents `app/todo.Service` as the authority for conversation-scoping assertions — store-layer correctness requires separate audit.
2. **Domain layer (`domain/todo/todo.go`)**: not audited in this fork. Verified via Read that the 4 sentinels exist (lines 67-75), `IsValidStatus` whitelist matches 4 statuses, Repository interface matches Service consumption.
3. **`tododomain.ErrConversationMismatch`**: defined but never used by Service. Domain godoc line 73 calls it "defensive reject to prevent scope leak" — but Service translates it away before any caller sees it. Worth confirming no other consumer (skill / forge / mcp tools) uses todo Service directly and exposes the raw sentinel.
4. **`Metadata map[string]any`** at todo.go:64 / 80: free-form blob persisted via gorm `serializer:json` (verified domain/todo/todo.go:29). No size validation in Service.Create / Update — relies on app-level convention. If a future LLM tool starts injecting large blobs (>10KB), DB row bloat could become an issue. Out of §S3-S17 scope but worth tracking.
5. **`todo_test.go`**: per audit constraint, not read.
