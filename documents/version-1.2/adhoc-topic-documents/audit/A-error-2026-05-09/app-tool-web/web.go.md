# audit: backend/internal/app/tool/web/web.go

LOC: 79
Read: full file (lines 1-79)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | web.go:38-49 | `WebTools(picker, keys, factory, mcpRouter, log) []toolapp.Tool { return []toolapp.Tool{ newWebFetch(...), newWebSearch(...) } }` | A.1/A.4 | OK | Pure wiring — no error production. Returns slice; nil checks deferred to per-tool ValidateInput / Execute. | N-A | — | — | — |
| 2 | web.go:54-64 | `newWebFetch(...) *WebFetch` constructor | A.1 | OK | Pure struct construction; no error path. | N-A | — | — | — |
| 3 | web.go:72-79 | `newWebSearch(keys, mcpRouter, log) *WebSearch { ... mcpRouter may be nil ... }` | A.1 | OK | Doc comment explicitly allows nil mcpRouter for tests; consumed at use sites with nil check (search.go line 223 `if t.mcpRouter != nil`). Documented intent. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — wiring file has no DB writes

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — wiring file generates no IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present (no fmt.Errorf calls in this file)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (constructors only)
