# audit: backend/internal/app/apikey/providers.go

LOC: 145
Read: full file (lines 1-145)

**File character**: pure-data registry + 3 lookup helpers. No I/O, no DB, no IDs generated, no sentinels defined, no error returns.

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | providers.go:119-122 | `func GetProviderMeta(name string) (ProviderMeta, bool) { m, ok := providers[name]; return m, ok }` | A.1 | OK | bool ok return, not error; idiomatic map-lookup pattern | N-A | — | — | — |
| 2 | providers.go:127-130 | `func IsValidProvider(name string) bool { _, ok := providers[name]; return ok }` | A.1 | OK | `_` discards `ProviderMeta` value (not error); bool-returning predicate | N-A | — | — | — |
| 3 | providers.go:138-144 | `func ListProviders() []string { ... range providers { names = append(names, name) } ... }` | A.1 | OK | pure data flatten; cannot fail | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (no error-returning calls in this file; `_` at line 128 discards `ProviderMeta` value not error)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file is a stateless registry; no DB writes, no terminal state

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file generates no business IDs (data-only registry)

A.4 §S16 错误 wrap 格式:
  - violations: not present (file has no error-returning paths to wrap)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined in this file: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels
