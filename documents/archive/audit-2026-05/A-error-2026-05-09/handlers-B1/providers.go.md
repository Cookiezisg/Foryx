# providers.go — audit trace

**Path**: `backend/internal/transport/httpapi/handlers/providers.go`
**LOC**: 96
**Role**: `ProvidersHandler` for `GET /api/v1/providers[?category=llm|search]`. Read-only exposure of the apikey package-level provider registry — lets testend / future Wails UI render the "Add API Key" dropdown without duplicating the whitelist client-side.

## 9-col trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | providers.go:60-63 | `wantCategory := r.URL.Query().Get("category"); names := apikeyapp.ListProviders()` | A.1 | OK | Pure registry read; no error path; query param is optional and unknown values gracefully filter to empty (per godoc line 54 "unknown category values return an empty list — the frontend already validated the choice locally — this is a thin wire"). §6 反校验剧场 explicitly applied + documented. | N-A | — | — | — |
| 2 | providers.go:65-72 | `for _, name := range names { meta, ok := apikeyapp.GetProviderMeta(name); if !ok { continue }; if wantCategory != "" && string(meta.Category) != wantCategory { continue }; ... }` | A.1 | EDGE | `if !ok { continue }` silently skips a registry-listed provider whose meta lookup fails. Per §S3 "silent fallback: upstream 失败后悄悄走 plan B 不告诉调用方" this is technically a silent skip — but in practice `ListProviders` + `GetProviderMeta` both query the same package-level registry, so `!ok` here means a **registry invariant violation** (a name was returned by List but its meta is missing). The current handler has no `log *zap.Logger` field (struct is empty `ProvidersHandler{}`), so even logging would require a constructor change. Provider-meta missing surfaces as a "dropdown missing one entry" — visible UX bug, not data loss. Not a real concern in practice. | LOW | If a registry bug ever desyncs List vs GetProviderMeta, the missing provider just disappears from the dropdown silently. Hard to debug without log. | (a) Inject `log *zap.Logger` into the handler so this branch can `log.Error("provider listed but meta missing")`; OR (b) add an inline comment justifying the silent skip as defensive against impossible invariant violation. Lowest priority. | FIXED-doc (this commit — 加内联注释说明 ListProviders+GetProviderMeta 同源 registry，desync 是 invariant 违反不可能；满足 §S3 "silent skip 必须带注释") |
| 3 | providers.go:73-85 | `out = append(out, providerInfo{...}); sortProviderInfos(out); responsehttpapi.Success(w, http.StatusOK, out)` | A.1 | OK | Pure data assembly + envelope. No error path. | N-A | — | — | — |
| 4 | providers.go:90-96 | `func sortProviderInfos(s []providerInfo) { for i := 1; i < len(s); i++ { for j := i; j > 0 && s[j-1].Name > s[j].Name; j-- { s[j-1], s[j] = s[j], s[j-1] } } }` | A.1 | OK | Pure-function insertion sort; no error path. (Side note: handwritten insertion sort instead of `sort.Slice` is a style choice, not an audit concern; presumably done for tiny n + no allocations.) | N-A | — | — | — |

## Sub-checks

**A.1 §S3 错误吞没**:
- violations: site 2 LOW (registry invariant silent-skip; no log injected, EDGE — defensive against impossible-in-practice case)

**A.2 §S9 detached ctx 终态写**:
- terminal-state writes identified: none
- 各自 ctx 来源: N/A — handler doesn't read `r.Context()` (read-only registry lookup, no IO)
- violations: N/A: read-only registry endpoint; no DB writes / no terminal state

**A.3 §S15 ID 生成**:
- ID generation calls: none
- violations: N/A: handler does not mint IDs (provider names are stable identifiers from the registry, not minted business IDs)

**A.4 §S16 错误 wrap 格式**:
- violations: not present (no `fmt.Errorf` / `errors.New` calls)

**A.5 §S17 sentinel 登记 errmap**:
- sentinels defined: none
- 已登记 errmap: N/A
- missing: N/A: file defines no sentinels and never calls `responsehttpapi.FromDomainError`

## Summary

- Sites: 4
- Violations: 1 LOW (site 2 silent-skip on registry invariant violation; EDGE-classified, suggested fix injects logger)
- Verdict: clean read-only handler. The single LOW concern is a hypothetical-only registry desync — practically harmless but masks a debug clue if it ever occurs.
