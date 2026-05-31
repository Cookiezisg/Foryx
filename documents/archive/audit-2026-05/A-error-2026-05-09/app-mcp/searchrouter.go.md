# audit: backend/internal/app/mcp/searchrouter.go

LOC: 92
Read: full file (lines 1-92)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | searchrouter.go:52 | `var ErrSearchServerUnavailable = errors.New("mcp: duckduckgo-search server not configured or not connected")` | A.5 | OK | sentinel defined in app layer (not mcpdomain). Per §S17 judgement criteria: this sentinel is **internal to mcp/web layer** — explicit doc comment lines 44-51 says "web tool's MCPSearchRouter port wraps this into its own ErrMCPSearchUnavailable so web doesn't import mcp domain types". So it's wrapped before reaching handlers; never directly hits errmap.FromDomainError. Per §S17 spec: "完全包内 / 跨包但只在 service 层消费、handler 层翻译成别的 sentinel 的，不需要登记". | N-A | — | — | — |
| 2 | searchrouter.go:78-83 | `st, err := r.svc.GetServer(ctx, V1SearchServerName); if err != nil { return "", ErrSearchServerUnavailable }` | A.4 | EDGE | §S16: this is a **deliberate sentinel translation** — GetServer returns ErrServerNotFound (registered) wrapped with mcpapp.GetServer prefix; here we transform that into ErrSearchServerUnavailable. The doc comment at line 80-81 states the intent ("ErrServerNotFound = not configured at all → translate to availability error"). However, the **original err is dropped** — caller gets only the translated sentinel without underlying cause. This is acceptable per §S16 ("sentinel 在最里层" — translation at boundary is fine), but loses debug context (was it not-configured, or some unexpected GetServer failure?). | LOW | search-not-available reported but root cause obscured if it's a non-not-found GetServer failure (theoretical — GetServer only returns ErrServerNotFound or success, so currently no information loss; future-proofing concern only) | optional: log original at Debug for diagnosis: `s.svc.log.Debug("search router: GetServer failed; routing to ErrSearchServerUnavailable", zap.Error(err))` — but probably not worth it for current GetServer impl | **WAIVED 2026-05-10** — GetServer only returns ErrServerNotFound or success, no info loss in current impl; future-proofing concern only. Audit-recommended waive. |
| 3 | searchrouter.go:84-86 | `if !mcpdomain.IsCallable(st.Status) { return "", fmt.Errorf("%w (status=%s)", ErrSearchServerUnavailable, st.Status) }` | A.4 | EDGE | §S16: prefix is `(status=%s)` not canonical `<pkg>.<Method>:` — sentinel `ErrSearchServerUnavailable` wrapped with %w correctly, but no pkg.Method context. errors.Is unwraps correctly to the sentinel; no functional issue. | LOW | error message reads "mcp: duckduckgo-search server not configured or not connected (status=connecting)" — readable but lacks call-site loc | optional: `fmt.Errorf("mcpapp.SearchRouter.CallSearchTool: %w (status=%s)", ErrSearchServerUnavailable, st.Status)` | **FIXED 2026-05-09 505d6e3** |
| 4 | searchrouter.go:87-90 | `args, _ := json.Marshal(map[string]any{ "query": query, "max_results": limit })` | A.1 | EDGE | §S3: `_` discards json.Marshal error without inline justification comment per §S3 spec example. **Functionally**: marshaling `map[string]any` with only string + int values cannot fail in Go's encoding/json (no NaN/Inf/cyclic possible from these types). So this is the same class as the §S3 example exception "panic 路径里的 cleanup" — error is provably impossible. **However**, §S3 spec is explicit that `_ = err` requires inline justification; this site has none. | LOW | zero — Marshal of plain types is a Go invariant; never fails in practice. Audit-trail concern only: future maintainer changing the args map shape might add a non-marshalable type | add inline comment: `args, _ := json.Marshal(...) // _ = err — Marshal of {string,int} map is unfailable per encoding/json invariant` | **FIXED 2026-05-09 505d6e3** |
| 5 | searchrouter.go:91 | `return r.svc.CallTool(ctx, V1SearchServerName, V1SearchToolName, args)` | A.4 | OK | bare passthrough of CallTool's (result, err) — calltool.go wraps internally with mcpapp.CallTool prefix + sentinels, so no double-wrap needed | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: 1 EDGE LOW (site #4 — `json.Marshal` of plain-type map; functionally unfailable but lacks the §S3-mandated inline justification comment)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file is a thin routing adapter; CallTool does not write any DB state (mcp ServerStatus is in-memory only per mcp.md §5.6)

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file does not generate IDs

A.4 §S16 错误 wrap 格式:
  - violations: 2 EDGE LOW
    - site #2 (translates GetServer err to ErrSearchServerUnavailable, drops original — not strict §S16 violation since translation is documented intent)
    - site #3 (`fmt.Errorf("%w (status=%s)", ...)` no pkg.Method prefix)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: `ErrSearchServerUnavailable` (line 52)
  - 已登记 errmap: NOT registered, by design — explicit doc comment at lines 44-51 confirms web layer wraps this into its own ErrMCPSearchUnavailable before any handler call. Internal-to-app-layer sentinel; never reaches errmap.FromDomainError directly.
  - missing: N/A — sentinel is intentionally not in errmap (per §S17 judgement: "完全包内 / 跨包但只在 service 层消费的，不需要登记")
  - **Verification needed**: confirm app/tool/web indeed wraps this sentinel and never propagates raw to handler. If a handler ever calls SearchRouter.CallSearchTool directly (skipping web tool), this sentinel becomes unmapped. Cross-fork concern; should be checked when app/tool/web fork audit completes.
