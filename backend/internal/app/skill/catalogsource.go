package skill

import (
	"context"

	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
)

// AsCatalogSource returns this Service's CatalogSource port adapter.
//
// AsCatalogSource 返回本 Service 的 CatalogSource 适配器。
func (s *Service) AsCatalogSource() catalogdomain.CatalogSource {
	return &skillCatalogSource{svc: s}
}

type skillCatalogSource struct {
	svc *Service
}

func (c *skillCatalogSource) Name() string                           { return "skill" }
func (c *skillCatalogSource) Granularity() catalogdomain.Granularity { return catalogdomain.PerItem }

func (c *skillCatalogSource) ListItems(ctx context.Context) ([]catalogdomain.Item, error) {
	skills := c.svc.List(ctx)
	items := make([]catalogdomain.Item, 0, len(skills))
	for _, sk := range skills {
		items = append(items, catalogdomain.Item{
			Source:      "skill",
			ID:          sk.Name,
			Name:        sk.Name,
			Description: sk.Description,
		})
	}
	return items, nil
}
