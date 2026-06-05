package document

import (
	"context"
	"strings"

	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
)

// AsCatalogSource returns the CatalogSource adapter for this Service: it reports
// every live doc as a name+description item so the LLM is aware the docs exist.
//
// AsCatalogSource 返本 Service 的 CatalogSource 适配器：把每篇活跃文档报成 name+description
// 条目，让 LLM 知道这些文档存在。
func (s *Service) AsCatalogSource() catalogdomain.CatalogSource {
	return &documentCatalogSource{svc: s}
}

type documentCatalogSource struct{ svc *Service }

var _ catalogdomain.CatalogSource = (*documentCatalogSource)(nil)

func (c *documentCatalogSource) Name() string { return "document" }

// ListItems flattens every live doc into a catalog Item; Name = Path so the LLM sees
// tree position at a glance. A blank description falls back to tags, then a placeholder.
//
// ListItems 把所有活跃文档摊平成 catalog Item；Name 用 Path 让 LLM 一眼看树结构。描述空时
// 回退到 tags，再回退到占位符。
func (c *documentCatalogSource) ListItems(ctx context.Context) ([]catalogdomain.Item, error) {
	docs, err := c.svc.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]catalogdomain.Item, 0, len(docs))
	for _, d := range docs {
		desc := strings.TrimSpace(d.Description)
		if desc == "" {
			if joined := strings.Join(d.Tags, ", "); joined != "" {
				desc = "tags: " + joined
			} else {
				desc = "(no description)"
			}
		}
		items = append(items, catalogdomain.Item{
			Source:      "document",
			ID:          d.ID,
			Name:        d.Path,
			Description: desc,
		})
	}
	return items, nil
}
