# model.go — audit trace

**Path**: `backend/internal/transport/httpapi/handlers/model.go`
**LOC**: 76
**Role**: `ModelConfigHandler` for 2 endpoints — `GET /api/v1/model-configs` (list) + `PUT /api/v1/model-configs/{scenario}` (upsert per §N6 200). Pure §S6 thin: decode → service → envelope.

## 9-col trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | model.go:48-55 | `items, err := h.svc.List(r.Context()); if err != nil { responsehttpapi.FromDomainError(w, h.log, err); return }; responsehttpapi.Success(w, http.StatusOK, items)` | A.1/A.5 | OK | Textbook §S6 thin handler. service err → `FromDomainError` (errmap covers `reqctxpkg.ErrMissingUserID` for missing-uid + any modelapp errors). r.Context() is the right ctx for a query (cancel on disconnect = correct behavior). | N-A | — | — | — |
| 2 | model.go:60-66 | `scenario := r.PathValue("scenario"); var req upsertModelRequest; if err := decodeJSON(r, &req); err != nil { responsehttpapi.FromDomainError(w, h.log, err); return }` | A.1 | OK | `decodeJSON` (apikey.go:217) wraps malformed JSON via `errorsdomain.ErrInvalidRequest` → 400 INVALID_REQUEST in errmap. Path-value `scenario` passes through unchecked to service — modelapp.Service is the right place for the whitelist check (`modeldomain.ErrInvalidScenario`, errmap.go:72). §6 反校验剧场: handler doesn't pre-validate scenario, defers to service (correct). | N-A | — | — | — |
| 3 | model.go:67-76 | `m, err := h.svc.Upsert(r.Context(), scenario, modelapp.UpsertInput{...}); if err != nil { responsehttpapi.FromDomainError(...) }; responsehttpapi.Success(w, http.StatusOK, m)` | A.1/A.5 | OK | service errors flow to errmap (modeldomain sentinels all registered: `ErrNotConfigured`, `ErrInvalidScenario`, `ErrProviderRequired`, `ErrModelIDRequired` — errmap.go:71-74). Returns 200 per §N6 (PUT upsert idempotent). r.Context() OK for upsert path: ctx-cancel mid-write → service may roll back via SQLite tx, no terminal-state-write concern at handler layer (the write itself is the user operation, not a post-stream finalization). | N-A | — | — | — |

## Sub-checks

**A.1 §S3 错误吞没**:
- violations: not present (every error path goes through `responsehttpapi.FromDomainError` → envelope; no `_` discards, no silent fallbacks)

**A.2 §S9 detached ctx 终态写**:
- terminal-state writes identified: site 3 `Upsert` writes config to DB
- 各自 ctx 来源: `r.Context()` (HTTP request scope)
- violations: not present. The Upsert IS the user operation — if client disconnects mid-PUT, the cancel is correct (no value to write what user no longer wants). §S9 detached-ctx pattern targets **post-cancel terminal state** like writing assistant final message after stream cancel; a single-step PUT does not match. Service layer may still need detached ctx if it kicks off async invariants, but at the handler layer r.Context() is correct.

**A.3 §S15 ID 生成**:
- ID generation calls: none in this file
- violations: N/A — handler does not mint IDs; modelapp.Service does (model config IDs `mc_<16hex>` per §S15) and the file under audit is purely the transport shell

**A.4 §S16 错误 wrap 格式**:
- violations: not present (handler does not wrap — it forwards via `FromDomainError`. The only wrap-adjacent call is `decodeJSON` itself in apikey.go:221 which uses `fmt.Errorf("decode body: %w", ...)` — that's out of scope for this file but worth noting it lacks the `<pkg>.<Method>:` prefix per §S16; will be flagged in apikey.go audit if covered)

**A.5 §S17 sentinel 登记 errmap**:
- sentinels defined: none in this file
- 已登记 errmap (consumed transitively):
  - `errorsdomain.ErrInvalidRequest` — errmap.go:44
  - `modeldomain.ErrNotConfigured` — errmap.go:71
  - `modeldomain.ErrInvalidScenario` — errmap.go:72
  - `modeldomain.ErrProviderRequired` — errmap.go:73
  - `modeldomain.ErrModelIDRequired` — errmap.go:74
  - `reqctxpkg.ErrMissingUserID` — errmap.go:185 (when ctx lacks user)
- missing: none — all model-domain sentinels surfaced via `FromDomainError` are registered

## Summary

- Sites: 3
- Violations: 0 (0 HIGH / 0 MED / 0 LOW)
- Verdict: textbook §S6 thin handler. Both endpoints follow the canonical pattern (decode → service → envelope). All sentinels registered. r.Context() correct everywhere — no terminal-state-write footprint at handler layer.
