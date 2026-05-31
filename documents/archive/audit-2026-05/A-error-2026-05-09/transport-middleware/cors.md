# Audit: backend/internal/transport/httpapi/middleware/cors.go

**LOC**: 89 (production); types `CORSConfig`, `DefaultCORSConfig`, `CORS` factory.

## Purpose

Browser CORS handling. Strict origin whitelist (no `*` to stay credential-compatible). Preflight returns 204 with CORS headers; disallowed origins pass through with no CORS headers (browser blocks per spec).

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | cors.go:65-72 | `origin := r.Header.Get("Origin"); if origin == "" { next.ServeHTTP(w, r); return }; if _, ok := allowed[origin]; !ok { next.ServeHTTP(w, r); return }` | A.1 | OK | §S3 — disallowed-origin pass-through is **documented design** (godoc lines 44-46 / 51-52: "no CORS headers; passes through (browser blocks)"). Not a silent fallback hiding failure: behavior is per-spec — only the browser enforces CORS, server has no opinion. Same for empty-Origin (pure same-origin/server-to-server request, CORS headers irrelevant). | — | — | — | — |
| 2 | cors.go:75-85 | `w.Header().Set("Access-Control-Allow-Origin", origin); ... if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" { ... w.WriteHeader(http.StatusNoContent); return }` | A.1 | OK | §S3 — pure header-write path; `http.ResponseWriter.Header().Set` doesn't return error. `w.WriteHeader` doesn't either. No error opportunities to swallow. | — | — | — | — |

## Sub-checks

```
A.1 §S3 错误吞没:
  - violations: not present
A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A: middleware doesn't perform terminal writes (header-only)
A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package doesn't generate business IDs
A.4 §S16 错误 wrap 格式:
  - violations: not present (no error returns)
A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: none
  - missing: N/A: file defines no sentinels
```

## Findings

**Clean** — no §S3/S9/S15/S16/S17 issues. Pass-through behavior on disallowed origin / empty Origin is per-spec design, fully documented in godoc, not a silent fallback.
