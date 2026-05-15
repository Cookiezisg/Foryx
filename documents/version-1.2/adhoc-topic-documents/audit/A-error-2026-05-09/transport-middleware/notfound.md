# Audit: backend/internal/transport/httpapi/middleware/notfound.go

**LOC**: 20 (production); single function `NotFound` (router fallback handler).

## Purpose

Router's fallback handler for unmatched URLs. Emits N1-compliant envelope with code `NOT_FOUND` instead of Go's default plain text "404 page not found".

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | notfound.go:15-20 | `func NotFound(w http.ResponseWriter, r *http.Request) { responsehttpapi.Error(w, http.StatusNotFound, "NOT_FOUND", "route not found: "+r.URL.Path, nil) }` | A.1 | OK | §S3 — direct Error envelope write; no error to swallow. The `NOT_FOUND` wire code is not in `errmap.go::errTable` because there's no Go sentinel here — the 404 originates at the router level (no domain error chain), so it doesn't go through `FromDomainError`. This is the correct path. | — | — | — | — |

## Sub-checks

```
A.1 §S3 错误吞没:
  - violations: not present
A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A: middleware doesn't perform terminal writes
A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package doesn't generate business IDs
A.4 §S16 错误 wrap 格式:
  - violations: not present (no fmt.Errorf)
A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none (the "NOT_FOUND" string here is a wire code, not a Go sentinel)
  - 已登记 errmap: N/A — emitted directly via responsehttpapi.Error, bypasses errTable
  - missing: N/A: file defines no Go sentinels; wire code is fine to emit directly because no upstream error exists to translate
```

## Findings

**Clean** — no §S3/S9/S15/S16/S17 issues. The 404 path is router-originated (not a domain sentinel), so direct envelope write is correct and bypassing errmap is intentional.
