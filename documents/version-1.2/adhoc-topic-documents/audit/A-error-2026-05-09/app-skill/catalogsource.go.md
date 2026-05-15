# audit: backend/internal/app/skill/catalogsource.go

LOC: 53
Read: full file (lines 1-53)

## Trace

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | catalogsource.go:30-32 | `func (s *Service) AsCatalogSource() catalogdomain.CatalogSource { return &skillCatalogSource{svc: s} }` | A.1 | OK | port adapter constructor; no error path. | N-A | — | — | — |
| 2 | catalogsource.go:38 | `func (c *skillCatalogSource) Name() string { return "skill" }` | A.1 | OK | constant string return. | N-A | — | — | — |
| 3 | catalogsource.go:39 | `func (c *skillCatalogSource) Granularity() catalogdomain.Granularity { return catalogdomain.PerItem }` | A.1 | OK | constant return; design noted in file header §12. | N-A | — | — | — |
| 4 | catalogsource.go:41-53 | `ListItems(ctx): skills := c.svc.List(ctx); items := make([]catalogdomain.Item, 0, len(skills)); for _, sk := range skills { items = append(items, catalogdomain.Item{...}) }; return items, nil` | A.1/A.4 | OK | List itself never errors (in-memory map iteration); ListItems signature has error return for `catalogdomain.CatalogSource` interface compliance only. Returning `(items, nil)` for all-success is canonical Go. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (no error paths in this file)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A (read-only listing)
  - violations: N/A — file is a read-side adapter

A.3 §S15 ID 生成:
  - ID generation calls: none — uses skill.Name as catalog Item.ID (per design comment line 47: "skill name is its stable identifier")
  - violations: N/A

A.4 §S16 错误 wrap 格式:
  - violations: not present (no fmt.Errorf calls)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — file defines no sentinels
