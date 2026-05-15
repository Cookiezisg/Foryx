# Audit: backend/internal/transport/httpapi/middleware/recover.go

**LOC**: 48 (production); package-doc + `Recover` middleware.

## Purpose

Catch handler panics, emit 500 INTERNAL_ERROR envelope (best-effort if headers already flushed), log panic value + stack + method + path. Must be outermost in chain.

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | recover.go:29-44 | `defer func() { rec := recover(); if rec == nil { return }; log.Error("panic recovered", zap.Any("panic", rec), zap.String("stack", string(debug.Stack())), ...); responsehttpapi.Error(w, 500, "INTERNAL_ERROR", "internal server error", nil) }()` | A.1/A.2 | OK | §S3 — panic recovered, **always logged** at ERROR level with full stack + method + path. Never silently swallowed. Raw panic value not leaked to client (envelope hardcoded "internal server error"). The "best-effort 500" inline comment at L40-41 explains the WriteHeader-after-headers-flushed case is a fundamental Go limitation: at that point you can't change wire bytes that already left, so the only useful outcome is the log line. §S9 — Recover is async/middleware boundary, log call is the canonical "fire-and-forget must-log" form per §S10 ("异步或 fire-and-forget 必须打"). | — | — | — | — |
| 2 | recover.go:42-43 | `responsehttpapi.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", nil)` | A.5 | OK | §S17 — `INTERNAL_ERROR` wire code matches the errTable default for unmapped errors (errmap.go:260). No Go sentinel involved — Recover writes the envelope directly because the panic value isn't an error type traversal. Consistent with the convention that direct-emit codes mirror errmap defaults. | — | — | — | — |

## Sub-checks

```
A.1 §S3 错误吞没:
  - violations: not present
  - documented carve-out: "best-effort 500 / write fails silently if headers flushed" — inline comment explains the unfixable-by-design case (Go HTTP: can't unflush)
A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none (panic log + 500 envelope are not "terminal writes" in the §S9 DB-write sense; they're synchronous best-effort wire output)
  - 各自 ctx 来源: r.Context() (panic happens in request scope; logging uses zap directly without ctx)
  - violations: not present
A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package doesn't generate business IDs
A.4 §S16 错误 wrap 格式:
  - violations: not present (no fmt.Errorf calls)
A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: "INTERNAL_ERROR" wire code matches errmap.go:260 default — directly emitted, not via FromDomainError
  - missing: none
```

## Findings

**Clean** — no §S3/S9/S15/S16/S17 issues. The header-flushed write-fails-silently note is per-spec carve-out (§S3 example 5: defer Close() in cleanup paths). Panic always loud-logs with stack — meets §S10 fire-and-forget rule.
