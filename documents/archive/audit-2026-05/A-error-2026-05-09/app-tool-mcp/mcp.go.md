# audit: backend/internal/app/tool/mcp/mcp.go

LOC: 56
Read: full file (lines 1-56)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | mcp.go:47-55 | `func MCPTools(svc) []toolapp.Tool { return []toolapp.Tool{ &SearchMCP{svc:svc}, &CallMCP{svc:svc}, &ListMCPMarketplace{svc:svc}, &InstallMCPServer{svc:svc}, &UninstallMCPServer{svc:svc} } }` | A.4 | OK | factory only — no error paths; struct literal init. §S16 N/A here. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (file has no error-handling sites)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — package factory only, no DB writes

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present (no fmt.Errorf calls)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none in this file
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels
