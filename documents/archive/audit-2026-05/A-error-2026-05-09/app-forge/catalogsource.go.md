# audit: backend/internal/app/forge/catalogsource.go

LOC: 94
Read: full file (lines 1-94)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | catalogsource.go:43-47 | `func (c *forgeCatalogSource) ListItems(ctx context.Context) ([]catalogdomain.Item, error) { forges, err := c.svc.ListAll(ctx); if err != nil { return nil, err } }` | A.4 | EDGE | §S16: bare-return — `forges, err := c.svc.ListAll(ctx)` propagates ListAll's already-wrapped error directly. Sentinel preserved. Style inconsistency vs. wrap pattern in forge.go itself. | LOW | identical UX (sentinel reaches errmap); harder to grep call site | wrap: `return nil, fmt.Errorf("forgeapp.forgeCatalogSource.ListItems: %w", err)` for grep traceability | FOUND |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (only path involves bare-return; no silent skips, no `_ = err`)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — file is read-only adapter (svc.ListAll); no DB writes

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — file generates no business IDs

A.4 §S16 错误 wrap 格式:
  - violations: site #1 (LOW EDGE — bare-return inconsistent with wrap pattern; sentinel preserved)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels (only consumes c.svc.ListAll's chain)
