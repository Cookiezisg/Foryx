# Audit: backend/internal/transport/httpapi/middleware/auth.go

**LOC**: 21 (production); single function `InjectUserID`.

## Purpose

Phase 2 simplified auth middleware — stamps `reqctxpkg.DefaultLocalUserID` into ctx. Eventual replacement: parse real credentials (JWT / session).

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | auth.go:15-20 | `func InjectUserID(next http.Handler) http.Handler { return http.HandlerFunc(func(w, r) { ctx := reqctxpkg.SetUserID(r.Context(), reqctxpkg.DefaultLocalUserID); next.ServeHTTP(w, r.WithContext(ctx)) }) }` | A.1 | OK | §S3 — no error path. Phase 2 design: every request unconditionally gets `DefaultLocalUserID`. Not a "silent fallback" because there's no upstream signal that could fail; this is the only auth source. Future JWT/session rewrite is documented in godoc as the explicit replacement plan. | — | — | — | — |

## Sub-checks

```
A.1 §S3 错误吞没:
  - violations: not present
A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A: middleware doesn't perform terminal writes (purely reads/decorates ctx)
A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package doesn't generate business IDs (DefaultLocalUserID is a package-level constant, not generated per-request)
A.4 §S16 错误 wrap 格式:
  - violations: not present (no error returns)
A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: none
  - missing: N/A: file defines no sentinels
```

## Findings

**Clean** — no §S3/S9/S15/S16/S17 issues. Middleware is intentionally trivial; the design doc note "Phase 2 simplified auth" makes the unconditional injection explicit (not a silent fallback masking failure).
