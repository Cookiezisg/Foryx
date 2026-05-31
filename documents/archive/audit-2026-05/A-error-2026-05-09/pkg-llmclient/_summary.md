# Package audit summary: internal/pkg/llmclient

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: One documented silent fallback (`ResolveForWebSummary` → `PickForChat` when web_summary scenario unconfigured); annotated in godoc lines 56-61 as deliberate UX ("works out of the box"). Matches §S3's documented-fallback exception. No undocumented swallows.
- **§S9 detached ctx 终态写**: **N/A** — pure resolve helper. Receives `ctx` from caller and threads it through `picker.Pick*` / `keys.ResolveCredentials` / `factory.Build`. No terminal-state writes anywhere; all 3 inner calls are read/build operations whose failures should respect upstream cancel.
- **§S15 ID 生成**: **N/A** — package doesn't generate business IDs.
- **§S16 错误 wrap 格式**: 3 wrap sites all use `fmt.Errorf("%w: %v", <sentinel>, err)` pattern — outer sentinel preserved, inner err as string. Strict reading: violates "禁止 `%v`". Empirical reading: works because the 7 callsites only `errors.Is` on the outer sentinel; inner-sentinel preservation isn't depended on. Marked LOW × 3 EDGE; not a true violation.
- **§S17 errmap 单一事实源**: 3 sentinels defined (`ErrPickModel` / `ErrResolveCreds` / `ErrBuildClient`). **None reach `responsehttpapi.FromDomainError`**:
  - chat runner.go:108-117 manually `errors.Is` translates them into wire codes inside async agent loop, then `emitFatalError` writes a status=error assistant message to event log — never returns to handler
  - skill / mcp / forge tool callsites wrap with their own domain prefix; errors stay inside agent tool_result text path
  - catalog generator wraps to `catalogdomain.ErrGenerationFailed` which is "absorbed inside Service.Refresh (mechanical fallback)" per errmap.go comment
  - subagent spawn / web fetch tool: same agentic tool_result pattern
  Conclusion: errmap registration **correctly absent** for all 3 sentinels.

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| llmclient.go | 104 | 5 | 2 | 0 | 0 | 3 |
| **TOTAL** | **104** | **5** | **2** | **0** | **0** | **3** |

## Severity breakdown

| Severity | Count | Status |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 3 | FOUND (all §S16 `%w: %v` pattern; non-blocking) |

**Net: 0 strict violations; 3 LOW EDGE notes**.

## Cross-cutting

### `%w: %v` chained-sentinel pattern (sites 2/4/5)

All 3 wrap sites in `Resolve` / `ResolveForWebSummary` / `finishResolve` use the same shape:

```go
return nil, fmt.Errorf("%w: %v", ErrPickModel, err)
```

vs §S16 strict canonical:

```go
return nil, fmt.Errorf("llmclient.Resolve: %w", err)
```

Trade-off: the llmclient form preserves a **stage discriminator sentinel** (`ErrPickModel` vs `ErrResolveCreds` vs `ErrBuildClient`) that callers can `errors.Is` to branch on; the canonical form would force callers to `errors.Is` on whatever sentinel `picker.PickForChat` returned (e.g., `modeldomain.ErrNotConfigured`) — which is finer-grained but also leaks domain-internal sentinels into client code.

The empirical winner: `%w: %v` for chain stage discrimination. The **only** caller that branches by stage is chat runner.go:111-115, and it depends on the outer (stage) sentinel — not the inner (domain) one. Inner-sentinel loss is invisible.

**If §S16 enforcement tightens**, switch to Go 1.20+ multi-`%w`:

```go
return nil, fmt.Errorf("llmclient.Resolve: %w (%w)", ErrPickModel, err)
```

This preserves both. Backward-compatible at the `errors.Is` level. Not required today.

### `ErrBuildClient` unreferenced

`grep -rn "ErrBuildClient" backend/internal/` shows the sentinel defined at llmclient.go:26 but never `errors.Is`'d by any caller. It exists for **stage symmetry** (3 stages → 3 sentinels) but only `ErrPickModel` and `ErrResolveCreds` are actually discriminated. Either:

- Wire a callsite that needs it (chat runner could add `case errors.Is(err, ErrBuildClient): code = "LLM_PROVIDER_ERROR"`), or
- Document it as "reserved for future stage-discrimination"

Neither is urgent. The sentinel doesn't leak (also not in errmap) and doesn't block anything.

### Why no errmap entry for these 3 sentinels

The chat agent loop is **async** — `runAgent` runs in a goroutine kicked off by the SSE handler, and any LLM resolve failure becomes a `block` event with `status=error` (or a stub assistant message with status=error). The handler that started the SSE stream has long since returned 200 + `text/event-stream` headers. There is **no return path** from runner back to a `FromDomainError` call. Same logic applies to:

- catalog: `ErrGenerationFailed` is absorbed inside `Service.Refresh` (mechanical fallback)
- skill/mcp tool searches: failures become tool_result text
- subagent spawn: same tool_result path
- web fetch tool: same tool_result path
- forge tools (search/get/create/edit/run): same tool_result path

Pattern: **llmclient sentinels are agent-internal**, not user-facing HTTP errors. errmap correctly does not list them.

## Spot-check (random clean sites)

3 sites picked across the file:

1. **llmclient.go:1-7** (package doc): bilingual godoc declares the "picker → keys → factory" 3-stage dance. Matches code.
2. **llmclient.go:69** (web-summary fallback): `errors.Is(err, modeldomain.ErrNotConfigured)` is the **only** sentinel-`Is` check in the file (apart from the cascading `if err != nil`). Confirms web-summary fallback is documented + intentional.
3. **llmclient.go:97-103** (Bundle return): straight pass-through of resolved values; no error path here.

All 3 spot-checks confirmed mechanism, not rubber-stamping.

## Recommended fix priorities

**No blocking fixes**. Optional improvements:

1. (LOW) `%w: %v` → multi-`%w` (Go 1.20+) at sites 2/4/5 — preserves inner sentinel for future callers. Not required.
2. (LOW) Document or wire `ErrBuildClient` discrimination — clarify intent.

## Out-of-scope notes

1. The fact that chat runner.go translates llmclient sentinels to wire codes via a manual `switch errors.Is`-block (lines 111-115) is a **bypass** of errmap, not a violation. The architectural reason: errmap requires a synchronous handler return path, but the chat agent loop is async; emitting a fatal error event is the only way to deliver the error to the client. This pattern repeats for all chat/agent fatal errors. Audit fork for chat runner.go would surface the same observation.
2. `ResolveForWebSummary` is the only function that has provider-fallback logic. If new scenarios get added (e.g., `auto_title`, `notification_summary`), each may want different fallback behaviour. The current design (per-function) avoids a generic "scenario chain" abstraction that would be over-engineered for 2 cases. Not a Phase A audit concern.
