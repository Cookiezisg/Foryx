# audit: backend/internal/app/mcp/catalogsource.go

LOC: 123
Read: full file (lines 1-123)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | catalogsource.go:67-85 | `func (c *mcpCatalogSource) ListItems(ctx context.Context) ([]catalogdomain.Item, error) { servers := c.svc.ListServers(ctx); ... return items, nil }` | A.1 | OK | always returns nil err — ListServers is in-memory snapshot from s.states map, no I/O can fail; design intent per catalog.md §12 (CatalogSource V1 contract: only return ready/degraded). Function signature includes error return for interface compliance + future I/O extension | N-A | — | — | — |
| 2 | catalogsource.go:74-76 | `if srv.Status != mcpdomain.StatusReady && srv.Status != mcpdomain.StatusDegraded { continue }` | A.1 | OK | non-error filter per CatalogSource V1 contract documented in lines 13-17 + 71-73 (skip half-loaded; catalog will pick up on next 1s tick) | N-A | — | — | — |
| 3 | catalogsource.go:97-123 | `synthesizeServerDescription` — pure string formatting; no errors | A.1 | OK | pure helper, no error paths | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present
  - Note: ListItems intentionally returns nil err (CatalogSource interface contract); skip-by-status (site #2) is documented design (catalog.md §12 V1 contract)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file is read-only catalog adapter; no DB writes, no terminal-state operations

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file uses `srv.Name` (string from mcp.json) as `Item.ID`; no business ID generation

A.4 §S16 错误 wrap 格式:
  - violations: not present (no fmt.Errorf calls in file)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none (file consumes mcpdomain status constants which are non-error consts, not sentinels)
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels
