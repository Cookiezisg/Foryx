# Audit: backend/internal/transport/httpapi/router/router.go

**LOC**: 102 (production); function `New` (handler/middleware assembly) + `applyChain`.

## Purpose

Build complete HTTP handler: routes + middleware chain + 404 fallback. Each domain's handler self-registers when service is non-nil. Chain order (outermost-first): Recover → RequestLogger → CORS → InjectLocale → InjectUserID → mux.

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | router.go:35-77 | `if deps.APIKeyService != nil { handlershttpapi.NewAPIKeyHandler(deps.APIKeyService, deps.Log).Register(mux) }; ... if deps.MCPService != nil { ... }; ...` | A.1 | OK | §S3 — nil-tolerant registration is a **design feature** documented in deps.go:31-38 godoc ("integration tests can stay narrow"). Not a silent fallback masking failure: in production main.go always wires every required service; nil only happens in narrow test bootstraps where the test author has explicitly chosen not to wire that domain. | — | — | — | — |
| 2 | router.go:62-68 | `// SubagentService no longer registers HTTP routes... _ = deps.SubagentService` | A.1 | OK | §S3 — the `_ = deps.SubagentService` is **annotated by the surrounding comment** explaining why: "sub-run data lives in the unified messages/message_blocks tables and is observed via the eventlog SSE stream + standard chat message endpoints." Comment satisfies §S3 example 1 ritual ("must come with comment"). Field is kept on Deps for future re-introduction; dropping the assignment would leave the struct field unused (compile-time warning if it had no usage). | — | — | — | — |
| 3 | router.go:78-80 | `if deps.Dev { handlershttpapi.NewDevHandler(deps.DB, deps.LogBroadcaster, deps.CollectionsDir, deps.IntegrationDir, deps.ForgifyHome, deps.Port, deps.Tools, deps.LLMFactory, deps.ShellManager, deps.Log).Register(mux) }` | A.1 | OK | §S3 — dev-only routes only register when `deps.Dev=true`. No silent skip: production simply doesn't expose dev endpoints. Documented per Deps godoc lines 152-215. | — | — | — | — |
| 4 | router.go:84 | `mux.HandleFunc("/", middlewarehttpapi.NotFound)` | A.5 | OK | §S5/§S17 — 404 fallback for unmatched paths. NotFound emits `NOT_FOUND` wire code directly (not via errmap). This is correct per audit of `middleware/notfound.go` — no Go sentinel involved at this level. | — | — | — | — |
| 5 | router.go:94-101 | `func applyChain(h http.Handler, deps Deps) http.Handler { h = middlewarehttpapi.InjectUserID(h); h = middlewarehttpapi.InjectLocale(h); h = middlewarehttpapi.CORS(middlewarehttpapi.DefaultCORSConfig())(h); h = middlewarehttpapi.RequestLogger(deps.Log)(h); h = middlewarehttpapi.Recover(deps.Log)(h); return h }` | A.1 | OK | §S3 — pure middleware composition. No error paths in chain wrapping itself. The chain order is documented in godoc lines 13-19 / 24-27 with rationale (Recover outermost catches panics; Logger next so 500s show up; CORS+locale+userID innermost so OPTIONS terminates early). | — | — | — | — |

## Sub-checks

```
A.1 §S3 错误吞没:
  - violations: not present
  - notable: `_ = deps.SubagentService` (line 68) IS annotated by surrounding godoc comment per §S3 ritual
A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none (router is pure assembly; ctx never appears)
  - 各自 ctx 来源: N/A
  - violations: N/A: package doesn't perform DB / persistent terminal writes
A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package doesn't generate business IDs
A.4 §S16 错误 wrap 格式:
  - violations: not present (no fmt.Errorf calls or error returns)
A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: none — 404 wire code emitted at middleware layer (notfound.go), not via FromDomainError
  - missing: N/A: file defines no Go sentinels
```

## Findings

**Clean** — no §S3/S9/S15/S16/S17 issues. Nil-tolerant service registration is documented design feature, not silent fallback. The single `_ =` (subagent placeholder) has properly justifying surrounding godoc per §S3 ritual.

The chain order documentation at lines 13-19 / 24-27 is the **source-of-truth** for Recover/Logger/CORS/locale/userID layering — referenced by middleware/recover.go for "must be outermost" claim.
