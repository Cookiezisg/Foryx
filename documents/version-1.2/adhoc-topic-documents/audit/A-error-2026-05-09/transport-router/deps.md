# Audit: backend/internal/transport/httpapi/router/deps.go

**LOC**: 217 (production); single struct `Deps` with package godoc.

## Purpose

Bundles every dependency the HTTP transport layer needs. Constructed once in `main.go`, handed to `router.New`. Per-domain service fields are nil-tolerant — `router.New` only registers a domain's routes when its service is non-nil (so integration tests can stay narrow).

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | deps.go:39-216 | `type Deps struct { Log *zap.Logger; APIKeyService *apikeyapp.Service; ... ShellManager *shelltool.ProcessManager }` | A.1/A.2/A.3/A.4/A.5 | OK | All — pure struct definition with godoc-only commentary. No code paths, no error handling, no ID generation, no sentinels. Field-level godoc is double-language per §S11. | — | — | — | — |

## Sub-checks

```
A.1 §S3 错误吞没:
  - violations: not present (file is data-only / no executable code)
A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A: deps file has no executable code; ctx never appears
A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A: package doesn't generate business IDs
A.4 §S16 错误 wrap 格式:
  - violations: not present (no fmt.Errorf or any error returns)
A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: none
  - missing: N/A: file defines no sentinels
```

## Findings

**Clean** — pure DI bundle struct, no executable code paths. Nothing to violate any of §S3/S9/S15/S16/S17.
