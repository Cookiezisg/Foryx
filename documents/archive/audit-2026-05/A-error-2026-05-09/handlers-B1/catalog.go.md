# catalog.go — audit trace

**Path**: `backend/internal/transport/httpapi/handlers/catalog.go`
**LOC**: 85
**Role**: `CatalogHandler` — 2 endpoints: `GET /api/v1/catalog` (cached read) + `POST /api/v1/catalog:refresh` (force refresh, §N5 :action). Capability Catalog HTTP transport per catalog.md §9.

## 9-col trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | catalog.go:42-45 | `if log == nil { log = zap.NewNop() }; return &CatalogHandler{svc: svc, log: log.Named("handlers.catalog")}` | A.1 | OK | Defensive nil-logger guard at construction; not an error path. | N-A | — | — | — |
| 2 | catalog.go:65-67 | `func (h *CatalogHandler) Get(w http.ResponseWriter, _ *http.Request) { responsehttpapi.Success(w, http.StatusOK, h.svc.Get()) }` | A.1 | OK | `Get()` returns the cached Catalog (or nil); no error path. Comment explicitly documents the "null when no Refresh has produced one yet" semantics. | N-A | — | — | — |
| 3 | catalog.go:79-85 | `if err := h.svc.Refresh(r.Context()); err != nil { responsehttpapi.FromDomainError(w, h.log, err); return }; responsehttpapi.Success(w, http.StatusOK, h.svc.Get())` | A.1/A.5 | OK | `Refresh` errors flow to errmap. Per errmap.go:128-139, `ErrCoverageIncomplete` + `ErrGenerationFailed` are absorbed inside Service.Refresh (mechanical fallback) and never reach handler — only `ErrAllSourcesFailed` is registered (errmap.go:139). r.Context() correct: refresh is the user's operation and ctx-cancel mid-refresh = correct cancel semantics. | N-A | — | — | — |

## Sub-checks

**A.1 §S3 错误吞没**:
- violations: not present (every error path goes through `responsehttpapi.FromDomainError`; no `_` discards)

**A.2 §S9 detached ctx 终态写**:
- terminal-state writes identified: site 3 `Refresh` writes new Catalog to in-memory cache (and possibly DB-backed source list)
- 各自 ctx 来源: `r.Context()`
- violations: not present. Refresh IS the user operation; ctx-cancel mid-refresh = correct (no value to write what user no longer wants). §S9 detached-ctx pattern targets **post-cancel terminal state** like writing assistant final message after stream cancel; a single-shot refresh does not match. (Service.Refresh internally may run multiple LLM/source fetches and decide per source how to handle cancel — that's a service-layer concern, not handler.)

**A.3 §S15 ID 生成**:
- ID generation calls: none in this file
- violations: N/A: handler does not mint IDs (Catalog domain item IDs are produced by Service.Refresh)

**A.4 §S16 错误 wrap 格式**:
- violations: not present (handler does not wrap; forwards via `FromDomainError`)

**A.5 §S17 sentinel 登记 errmap**:
- sentinels defined: none in this file
- 已登记 errmap (consumed transitively):
  - `catalogdomain.ErrAllSourcesFailed` — errmap.go:139
- absorbed in service (per errmap.go:128-139 explicit comment, do NOT need errmap rows): `catalogdomain.ErrCoverageIncomplete`, `catalogdomain.ErrGenerationFailed`
- missing: none

## Summary

- Sites: 3
- Violations: 0 (0 HIGH / 0 MED / 0 LOW)
- Verdict: textbook §S6 thin handler. 2 endpoints, both follow canonical pattern. errmap registration is correct and explicitly documented as comprehensive in errmap.go itself (lines 128-139).
