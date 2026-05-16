package function

import (
	"context"
	"strings"

	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
)

// AsCatalogSource returns the CatalogSource port adapter for this Service.
//
// AsCatalogSource 返本 Service 的 CatalogSource port 适配器。
func (s *Service) AsCatalogSource() catalogdomain.CatalogSource {
	return &functionCatalogSource{svc: s}
}

type functionCatalogSource struct {
	svc *Service
}

func (c *functionCatalogSource) Name() string                           { return "function" }
func (c *functionCatalogSource) Granularity() catalogdomain.Granularity { return catalogdomain.PerItem }

func (c *functionCatalogSource) ListItems(ctx context.Context) ([]catalogdomain.Item, error) {
	fns, err := c.svc.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]catalogdomain.Item, 0, len(fns))
	for _, f := range fns {
		desc := strings.TrimSpace(f.Description)
		if desc == "" {
			if joined := strings.Join(f.Tags, ", "); joined != "" {
				desc = joined
			} else {
				desc = "(no description)"
			}
		}
		items = append(items, catalogdomain.Item{
			Source:      "function",
			ID:          f.ID,
			Name:        f.Name,
			Description: desc,
		})
	}
	return items, nil
}
