# Package audit summary: internal/app/model

**Phase A — §S3 / §S9 / §S15 / §S16 / §S17**

## Spec understanding

- **§S3 错误不吞**: error suppression that hides user-visible failure / data loss / config drift is forbidden. Service has only one `errors.Is`-based branch (`ErrNotConfigured` discrimination at line 83-86) — that's the intentional "first-time set up" branch in Upsert, not silent swallow (the err is consumed only when the next code path reconstructs a fresh `ModelConfig`). No `_ = err`, no `if err != nil { return nil }`. Read paths (`List` / `PickForChat` / `PickForWebSummary`) propagate err with store-layer wrap intact. The PickForWebSummary godoc-mentioned "caller falls back to PickForChat" is a **caller** responsibility — service returns `ErrNotConfigured` faithfully, caller (WebFetch tool) does the fallback.
- **§S9 detached ctx 终态写**: terminal-state writes that MUST persist regardless of caller cancel use detached ctx. **N/A at this service**: Upsert is the user's foreground PUT action (`PUT /api/v1/model-configs/{scenario}`); no post-cancel-must-persist scenario, no fire-and-forget writes, no streaming. If user closes tab mid-PUT, aborting the in-flight write is correct (DB stays consistent with last saved value; user retries). Contrast: chat's "final assistant message after stream cancelled" or apikey's "test result after probe completes" — model config has no such side-effect requiring detached ctx.
- **§S15 ID 生成**: `<prefix>_<16hex>` via `idgenpkg.New("mc")`. Single-line wrapper at model.go:135 (`newID()`), called once at Upsert (line 88, new-config branch). Prefix `mc_` matches CLAUDE.md §S15 model-config entry. `idgenpkg.New` enforces panic-on-rand-fail at the implementation layer (idgen.go:21-23) — caller doesn't repeat. No self-rolled crypto, no math/rand, no time-based IDs.
- **§S16 错误 wrap 格式**: `fmt.Errorf("model.Service.Upsert: %w", err)` literal prefix + `%w` at the single explicit-wrap site (line 80). 5 pass-through sites correctly relay the store / repo's already-prefixed err (per §S16 example: store layer owns its own `<pkg>.<Method>:` prefix; service doesn't double-wrap). 3 sentinel-direct returns at innermost validator (`ErrInvalidScenario` / `ErrProviderRequired` / `ErrModelIDRequired`) skip wrap correctly.
- **§S17 errmap 单一事实源**: every sentinel that can reach a handler must be in errmap.go. Package defines NO local sentinels. Consumes 5 cross-package sentinels — all 5 verified registered:
  - `modeldomain.ErrInvalidScenario` → errmap.go:72 (400 INVALID_SCENARIO)
  - `modeldomain.ErrProviderRequired` → errmap.go:73 (400 PROVIDER_REQUIRED)
  - `modeldomain.ErrModelIDRequired` → errmap.go:74 (400 MODEL_ID_REQUIRED)
  - `modeldomain.ErrNotConfigured` → errmap.go:71 (422 MODEL_NOT_CONFIGURED)
  - `reqctxpkg.ErrMissingUserID` → errmap.go:185 (500 INTERNAL_ERROR — cross-cutting)

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| model.go | 135 | 13 | 13 | 0 | 0 | 0 |
| **TOTAL** | **135** | **13** | **13** | **0** | **0** | **0** |

## Severity breakdown

| Severity | Count | Sites | Status |
|---|---|---|---|
| HIGH | 0 | — | — |
| MED | 0 | — | — |
| LOW | 0 | — | — |

**Net: 0 violations / 0 EDGE.**

## Cross-cutting

### Sentinel chain integrity (§S17)

- **Defined locally**: none (zero local sentinels — package consumes only `modeldomain` + `reqctxpkg`).
- **Consumed from `modeldomain`**: 4 sentinels (ErrNotConfigured / ErrInvalidScenario / ErrProviderRequired / ErrModelIDRequired); all reach handlers via Upsert / PickForChat / PickForWebSummary; all 4 registered.
- **Consumed from `reqctxpkg`**: ErrMissingUserID (wrapped at line 80) — registered as cross-cutting INTERNAL_ERROR.
- **No missing registrations.**

### Detached ctx coverage (§S9) — N/A at this layer

The single mutating method (Upsert) uses pass-through caller ctx. This is the **correct** semantic for foreground user-driven PUT:
- Upsert is invoked from `transport/httpapi` PUT handler, runs synchronously inside the request lifecycle.
- If HTTP request cancels (browser close, network drop), the in-flight DB write should abort — there's no post-cancel state to preserve.
- Contrast with chat's "write assistant final message after stream cancelled" (true terminal-state) → that needs detached ctx via `reqctxpkg.SetUserID(context.Background(), uid)`. model config has no analogous post-cancel-must-persist path.

The post-success `s.log.Info(...)` at line 98-102 is structured logging, not state write — N/A for §S9.

### ModelPicker compile-time guard (line 56)

`var _ modeldomain.ModelPicker = (*Service)(nil)` enforces that *Service satisfies the ModelPicker port interface (consumed by chat / WebFetch tool / etc. to look up scenario → provider/modelID). The two PickFor* methods are the interface implementation. This is §S13-style port hygiene — `app/model.Service` implements the port; consumers depend on the abstract interface, not the concrete struct. No audit issue, included here to confirm the cross-domain wiring is by-interface.

### Style consistency cross-check vs sibling app/* services

Stylistically consistent with `app/apikey`, `app/forge`, `app/conv`, `app/todo`:
- Constructor panic on nil logger (sibling pattern verified at `apikey.NewService`, `forge.NewService`, `todo.NewService`).
- Single-line `newID() string { return idgenpkg.New("mc") }` matches `apikeyapp.newID() = idgenpkg.New("aki")`, `todoapp.newID() = idgenpkg.New("td")` etc.
- `<pkg>.<Method>: %w` wrap form at every cross-layer producer site (line 80).
- Sentinel-direct return at innermost validator (no wrap of bare sentinel) at lines 70/73/76.
- Pass-through ctx at all sites — package has no detached-ctx use cases.

### Package structure (§S12 / §S13)

- **§S12**: `model.go` is the main file matching package name; package godoc at top covering all 3 layers' naming convention (line 1-12); single file at 135 LOC well under 500-line guideline. Compliant.
- **§S13**: Package declared `package model`; consumers alias as `modelapp` per `<name><role>` rule (verified errmap.go imports `modeldomain`, sibling stores import `modelapp` etc.). Sibling `modeldomain` (domain/model) and `modelstore` (infra/store/model) follow same convention. Compliant.

## Spot-check (random clean sites — verify mechanism not rubber-stamping)

5 sites picked from `OK` set:

1. **Site #6** (Upsert RequireUserID wrap): verified — `fmt.Errorf("model.Service.Upsert: %w", err)` literal prefix + `%w`. Sentinel `reqctxpkg.ErrMissingUserID` registered errmap.go:185. errors.Is chain intact (handler → response.FromDomainError → lookup → Is unwraps to bare sentinel).
2. **Site #7** (errors.Is ErrNotConfigured branch): verified NOT a §S3 violation — the err is consumed because the next branch reconstructs `m` (line 86-92). Without the discrimination, every first-time Upsert would 404 instead of creating the row. Sentinel `ErrNotConfigured` registered errmap.go:71 (caller of List / PickFor* still surfaces it correctly when m is nil).
3. **Site #8** (newID on new-config branch): verified — `newID()` is `idgenpkg.New("mc")` (line 135); `mc_` matches CLAUDE.md S15 list. `idgenpkg.New` panics on rand fail (idgen.go:22-23). No double-check at call site per §S15.
4. **Site #11** (PickForChat): verified — pass-through err with store wrap. ErrNotConfigured registered errmap.go:71 → 422 MODEL_NOT_CONFIGURED. Returning `"", "", err` is canonical "failure" form (not §S3 violation 7 — the err IS wrapped, just by the store layer not the service).
5. **Site #13** (newID definition): verified — single-line. Prefix `mc` matches CLAUDE.md §S15 model-config entry. `idgenpkg.New` panics on rand fail (idgen.go:21-23). No 16-hex / no math/rand / no time-based ID.

All 5 spot-checks confirmed correct classification — mechanism functioning, not rubber-stamping.

## Recommended fix priorities

**None.** Package is clean across all 5 sub-checks. No HIGH / MED / LOW found. No action items.

If future expansion adds:
1. **A scenario-default broadcast** (e.g. "when user changes Chat scenario, fire notification to all open conversations"), §S9 detached ctx may apply — adopt the chat-package pattern (`reqctxpkg.SetUserID(context.Background(), uid)`).
2. **A new sentinel** in `modeldomain` (e.g. `ErrModelDeprecated` for retired model IDs), it must be added to errmap.go in the same commit per §S17.
3. **A delete endpoint** (currently no Delete in Service), the same pass-through-ctx semantic applies — only adopt detached ctx if there's a follow-up "broadcast deletion" or "purge dependent rows" tail that must persist.

## Out-of-scope notes (parent should verify if relevant)

1. **Store layer (`infra/store/model/...`)**: not audited in this fork. Sentinel registration coverage assumes store wraps with its own `<pkg>.<Method>: %w` prefix per §S16 (matches sibling stores' pattern); store-layer correctness requires separate audit.
2. **Domain layer (`domain/model/model.go`)**: not audited in this fork. Verified via grep that the 4 sentinels exist (lines 70-73), `IsValidScenario` whitelist + `ScenarioChat` / `ScenarioWebSummary` constants exist, ModelPicker interface matches Service consumption. No audit issue at the boundary.
3. **`ScenarioWebSummary` fallback**: PickForWebSummary godoc line 119-126 promises "WebFetch tool falls back to PickForChat". This requires the WebFetch tool's caller to do the fallback — out of scope for this audit, but worth confirming `app/tool/web/...` actually does `errors.Is(err, modeldomain.ErrNotConfigured)` + retry with PickForChat. If not, the documentation is a lie (silent failure: WebFetch summarisation breaks for users without ScenarioWebSummary set, not §S3 here but potential audit issue downstream).
4. **`model_test.go`**: per audit constraint, not read.
