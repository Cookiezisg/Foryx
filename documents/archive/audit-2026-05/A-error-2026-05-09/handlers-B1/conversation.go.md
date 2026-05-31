# conversation.go — audit trace

**Path**: `backend/internal/transport/httpapi/handlers/conversation.go`
**LOC**: 139
**Role**: `ConversationHandler` for 5 endpoints — `POST` (create) / `GET` (list, paged) / `GET /{id}` / `PATCH /{id}` (Rename, actually generic partial update) / `DELETE /{id}`. Pure §S6 thin handlers.

## 9-col trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | conversation.go:61-73 | `Create: var req createConvRequest; if err := decodeJSON(r, &req); err != nil { FromDomainError... }; c, err := h.svc.Create(r.Context(), req.Title); if err != nil { FromDomainError... }; Created(w, c)` | A.1/A.5 | OK | Textbook §S6 thin pattern. decodeJSON wraps malformed body to ErrInvalidRequest (errmap.go:44). svc.Create errors flow to errmap. Returns 201 per §N2. r.Context() correct for create — cancel-on-disconnect = correct (no value to write what user no longer wants). | N-A | — | — | — |
| 2 | conversation.go:78-93 | `List: p, err := paginationpkg.Parse(r); if err != nil { ... }; items, next, err := h.svc.List(r.Context(), convdomain.ListFilter{Cursor: p.Cursor, Limit: p.Limit}); ...; Paged(w, items, next, next != "")` | A.1/A.5 | OK | Pagination parse errors wrap `errorsdomain.ErrInvalidRequest` (pagination/cursor.go:55, 95, 98 — all registered errmap.go:44). svc.List paged result → §N4 envelope `{data, nextCursor, hasMore}`. Note: `next != ""` derives `hasMore` from the cursor token presence — semantically aligned with N4 paged contract, but worth noting if `next == ""` always means "last page" per the convapp.List contract (assumed correct from convdomain). | N-A | — | — | — |
| 3 | conversation.go:99-106 | `Get: c, err := h.svc.Get(r.Context(), r.PathValue("id")); if err != nil { FromDomainError... }; Success(w, http.StatusOK, c)` | A.1/A.5 | OK | Path-id passed unchecked (no empty-string guard) — service handles "not found / empty id" via convdomain.ErrNotFound (errmap.go:58 → 404). §6 反校验剧场 correctly applied: empty id triggers store NotFound naturally. | N-A | — | — | — |
| 4 | conversation.go:115-128 | `Rename: id := r.PathValue("id"); var req updateConvRequest; if err := decodeJSON(r, &req); err != nil { ... }; c, err := h.svc.Update(r.Context(), id, req.Title, req.SystemPrompt); ...; Success(w, http.StatusOK, c)` | A.1/A.5 | OK | Partial-update PATCH using `*string` ptr fields to distinguish absent / present-empty — godoc lines 47-56 explicitly document the choice. service Update errors flow to errmap (convdomain.ErrNotFound + ErrInvalidRequest if validation fails internally). r.Context() correct — single-step PATCH, cancel-on-disconnect is right. | N-A | — | — | — |
| 5 | conversation.go:133-139 | `Delete: if err := h.svc.Delete(r.Context(), r.PathValue("id")); err != nil { FromDomainError... }; NoContent(w)` | A.1/A.5 | OK | DELETE returns 204 per §N2. r.Context() correct. svc.Delete errors → errmap (ErrNotFound → 404). | N-A | — | — | — |

## Sub-checks

**A.1 §S3 错误吞没**:
- violations: not present (every error path goes through `responsehttpapi.FromDomainError` — handler has no `_` discards, no silent fallbacks)

**A.2 §S9 detached ctx 终态写**:
- terminal-state writes identified: site 1 Create / site 4 Update / site 5 Delete all write to DB
- 各自 ctx 来源: `r.Context()` for all
- violations: not present. Each is a single-step user CRUD operation — ctx-cancel mid-write = correct (no value to persist what user no longer wants). §S9 detached-ctx pattern targets **post-cancel terminal state** like writing assistant final message after stream cancel; one-shot CRUD does not match. No fire-and-forget goroutines started here.

**A.3 §S15 ID 生成**:
- ID generation calls: none in this file
- violations: N/A: handler does not mint conversation IDs (`cv_<16hex>` per §S15) — that responsibility lives in convapp.Service (transport is pure shell)

**A.4 §S16 错误 wrap 格式**:
- violations: not present in this file (handler does not wrap; forwards via `FromDomainError`)
- cross-cutting note: paginationpkg.cursor.go:55, 95, 98 wraps with `fmt.Errorf("limit must be a positive integer: %w", ...)` and `fmt.Errorf("decode cursor: %w", ...)` — these lack the `<pkg>.<Method>:` prefix per §S16, but that's a `pkg/pagination` audit concern, not this file. Flagged here only for visibility.

**A.5 §S17 sentinel 登记 errmap**:
- sentinels defined: none in this file
- 已登记 errmap (consumed transitively):
  - `errorsdomain.ErrInvalidRequest` — errmap.go:44 (via decodeJSON + paginationpkg.Parse)
  - `convdomain.ErrNotFound` — errmap.go:58
  - `reqctxpkg.ErrMissingUserID` — errmap.go:185 (when ctx lacks user, e.g., bypass auth middleware)
- missing: none — convapp.Service exposes only `convdomain.ErrNotFound` as a domain sentinel; any internal validation it might add (e.g., title length) currently piggybacks on ErrInvalidRequest

## Summary

- Sites: 5
- Violations: 0 (0 HIGH / 0 MED / 0 LOW)
- Verdict: textbook §S6 thin handler. 5 endpoints, all follow the canonical pattern (decode → service → envelope). All sentinels registered. Single cross-cutting noise: paginationpkg's wrap format lacks §S16 prefix — out of scope for this file but worth carrying to a paginationpkg audit.
