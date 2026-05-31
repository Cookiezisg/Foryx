# Audit trace: backend/internal/app/model/model.go

**Phase A — §S3 / §S9 / §S15 / §S16 / §S17**
**LOC**: 135 (incl. package godoc + blank lines).
**Scope**: Service methods (List / Upsert / PickForChat / PickForWebSummary) + newID helper + NewService constructor + ModelPicker compile-time guard.

## Sites

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | model.go:39-41 | `if log == nil { panic("model.NewService: logger is nil") }` | A.3 / A.5 | OK | Constructor invariant; nil logger = boot-time programmer error. Not error path. Sibling pattern verified in app/todo, app/apikey constructors. | N-A | — | — | — |
| 2 | model.go:62 | `func (s *Service) List(ctx context.Context) (...) { return s.repo.List(ctx) }` | A.4 | OK | Pass-through to store. §S16 example "store layer owns its own `<pkg>.<Method>:` prefix; service doesn't double-wrap" applies — repo returns errs already prefixed. ctx is propagated correctly per §S9 (read path, not terminal write). | N-A | — | — | — |
| 3 | model.go:69-71 | `if !modeldomain.IsValidScenario(scenario) { return nil, modeldomain.ErrInvalidScenario }` | A.4 / A.5 | OK | Innermost validator returns bare sentinel — §S16 explicit "直接返 sentinel（最里层无需 wrap）". Sentinel registered errmap.go:72 → 400 INVALID_SCENARIO. errors.Is chain intact. | N-A | — | — | — |
| 4 | model.go:72-74 | `if strings.TrimSpace(in.Provider) == "" { return nil, modeldomain.ErrProviderRequired }` | A.4 / A.5 | OK | Bare sentinel innermost return. Registered errmap.go:73 → 400 PROVIDER_REQUIRED. | N-A | — | — | — |
| 5 | model.go:75-77 | `if strings.TrimSpace(in.ModelID) == "" { return nil, modeldomain.ErrModelIDRequired }` | A.4 / A.5 | OK | Bare sentinel innermost return. Registered errmap.go:74 → 400 MODEL_ID_REQUIRED. | N-A | — | — | — |
| 6 | model.go:78-81 | `uid, err := reqctxpkg.RequireUserID(ctx); if err != nil { return nil, fmt.Errorf("model.Service.Upsert: %w", err) }` | A.4 / A.5 | OK | `<pkg>.<Method>: %w` literal + `%w`. `reqctxpkg.ErrMissingUserID` cross-cutting registered errmap.go:185 → 500 INTERNAL_ERROR. Chain unwraps. | N-A | — | — | — |
| 7 | model.go:82-85 | `m, err := s.repo.GetByScenario(ctx, scenario); if err != nil && !errors.Is(err, modeldomain.ErrNotConfigured) { return nil, err }` | A.1 / A.4 | OK | NOT swallowing — ErrNotConfigured is the "first time set up" branch and intentionally falls through to construct a fresh ModelConfig (lines 86-92). Other err types pass through with original wrap from store layer. Pass-through (no double-wrap) per §S16. | N-A | — | — | — |
| 8 | model.go:88 | `m = &modeldomain.ModelConfig{ID: newID(), UserID: uid, Scenario: scenario}` | A.3 | OK | `newID()` = `idgenpkg.New("mc")` (line 135). `mc_` prefix matches CLAUDE.md S15 list (model config). idgen.New panics on rand.Read fail (idgen.go:21-23) — caller doesn't repeat. crypto/rand source. | N-A | — | — | — |
| 9 | model.go:95-97 | `if err := s.repo.Upsert(ctx, m); err != nil { return nil, err }` | A.4 / A.9 | OK | Pass-through; store-layer err already wrapped per §S16. ctx propagated. **NOT a §S9 detached-ctx site** — Upsert is the user's foreground action: if HTTP request cancels, aborting the write is the correct semantic (no orphan post-cancel state to preserve, unlike chat's "write final assistant message after stream cancelled" or apikey.Test's "record probe result after probe finishes"). | N-A | — | — | — |
| 10 | model.go:98-102 | `s.log.Info("model config upserted", zap.String("user_id", uid), ...)` | A.1 | OK | Structured log; not error path. §S10 pattern-compliant for the success branch. | N-A | — | — | — |
| 11 | model.go:111-116 | `func (s *Service) PickForChat(ctx context.Context) ... { m, err := s.repo.GetByScenario(ctx, modeldomain.ScenarioChat); if err != nil { return "", "", err }; return m.Provider, m.ModelID, nil }` | A.1 / A.4 | OK | Read path; pass-through err with store wrap. ErrNotConfigured registered errmap.go:71 → 422 MODEL_NOT_CONFIGURED. NOT §S3 violation 7 ("zero value + err but no wrap") — `"", "", err` is correct (returning the wrapped err from store + zero strings is the canonical "failure" form). | N-A | — | — | — |
| 12 | model.go:127-133 | `func (s *Service) PickForWebSummary(ctx context.Context) ... { m, err := s.repo.GetByScenario(ctx, modeldomain.ScenarioWebSummary); if err != nil { return "", "", err }; return m.Provider, m.ModelID, nil }` | A.1 / A.4 | OK | Same pattern as #11; ErrNotConfigured propagation lets WebFetch caller errors.Is check + fallback to PickForChat (godoc lines 119-126 explicit). NOT §S3 silent fallback (caller, not service, decides fallback). | N-A | — | — | — |
| 13 | model.go:135 | `func newID() string { return idgenpkg.New("mc") }` | A.3 | OK | Single-line wrapper; `mc_` prefix correct per §S15 + CLAUDE.md ID list. idgen panics on rand fail. | N-A | — | — | — |

## Sub-checks

**A.1 §S3 错误吞没:**
  - violations: not present
  - All err paths return wrapped err / pass-through-store-wrapped err / sentinel; the only `errors.Is(err, ErrNotConfigured)` discrimination at site #7 is intentional Upsert "first-time setup" branch (godoc line 65-67 implicit; cleanup confirmed by following lines 86-92 reconstructing `m`). No `_ = err`, no `if err != nil { return nil }`, no silent fallback in service layer. The PickForWebSummary doc-stated WebFetch fallback (sites #11/#12) is a **caller** responsibility, not a service-layer silent swallow.

**A.2 §S9 detached ctx 终态写:**
  - terminal-state writes identified: site #9 (Upsert s.repo.Upsert)
  - 各自 ctx 来源: pass-through caller ctx (HTTP r.Context() upstream)
  - violations: not present (N/A semantics)
  - **Reasoning**: Upsert is foreground user action — user clicks "save", PUT /api/v1/model-configs/{scenario} hits handler, repo write completes synchronously inside the request. If user closes tab mid-request, aborting the write is correct (DB stays consistent with last-saved state; user retries). Contrast §S9 typical violation: "user starts long-running stream, cancels, but final assistant message MUST be persisted" — model config has no analogous post-cancel-must-persist requirement. No background goroutines, no fire-and-forget writes, no "stream finished after caller left" semantics in this file. Site #9's pass-through ctx is correct.

**A.3 §S15 ID 生成:**
  - ID generation calls: `newID()` at site #8 (Upsert new-config branch); `newID()` defined at site #13 = `idgenpkg.New("mc")`
  - violations: not present
  - Prefix `mc_` matches CLAUDE.md §S15 model config entry. crypto/rand panic-on-fail enforced inside `idgen.New` (idgen.go:21-23). No self-rolled rand, no math/rand, no time-based IDs.

**A.4 §S16 错误 wrap 格式:**
  - violations: not present
  - 1 explicit wrap at site #6 with literal `model.Service.Upsert: %w`. 4 pass-throughs (sites #2, #7, #9, #11, #12) correctly relay store layer's already-prefixed err. 3 sentinel-direct returns (sites #3, #4, #5) at innermost validator. 0 `errors.New(... + err.Error())`, 0 `%v`, 0 unprefixed `fmt.Errorf("%w", err)`. errors.Is chain unwraps cleanly at every layer.

**A.5 §S17 sentinel 登记 errmap:**
  - sentinels defined: none (this file defines no sentinels — they live in `domain/model/model.go:70-73`)
  - 已登记 errmap (consumed sentinels reaching handler):
    - `modeldomain.ErrInvalidScenario` → errmap.go:72 (400 INVALID_SCENARIO)
    - `modeldomain.ErrProviderRequired` → errmap.go:73 (400 PROVIDER_REQUIRED)
    - `modeldomain.ErrModelIDRequired` → errmap.go:74 (400 MODEL_ID_REQUIRED)
    - `modeldomain.ErrNotConfigured` → errmap.go:71 (422 MODEL_NOT_CONFIGURED)
    - `reqctxpkg.ErrMissingUserID` → errmap.go:185 (500 INTERNAL_ERROR cross-cutting)
  - missing: all registered
  - **All 4 modeldomain sentinels reach handlers** (Upsert returns each; PickFor* returns ErrNotConfigured) — all 4 registered. Cross-cutting `reqctxpkg.ErrMissingUserID` (wrapped via site #6) — registered.
