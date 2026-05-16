package handler

import (
	"context"
	"fmt"
	"strings"

	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
)

// AsCatalogSource returns the CatalogSource port adapter for this Service.
//
// AsCatalogSource 返回 Service 的 CatalogSource port 适配器。
func (s *Service) AsCatalogSource() catalogdomain.CatalogSource {
	return &handlerCatalogSource{svc: s}
}

type handlerCatalogSource struct {
	svc *Service
}

func (c *handlerCatalogSource) Name() string                           { return "handler" }
func (c *handlerCatalogSource) Granularity() catalogdomain.Granularity { return catalogdomain.PerItem }

func (c *handlerCatalogSource) ListItems(ctx context.Context) ([]catalogdomain.Item, error) {
	hs, err := c.svc.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]catalogdomain.Item, 0, len(hs))
	for _, h := range hs {
		desc := strings.TrimSpace(h.Description)
		if desc == "" {
			if joined := strings.Join(h.Tags, ", "); joined != "" {
				desc = joined
			} else {
				desc = "(no description)"
			}
		}

		var methodNames []string
		var configState string
		if h.ActiveVersionID != "" {
			active, _ := c.svc.repo.GetVersion(ctx, h.ActiveVersionID)
			if active != nil {
				for _, m := range active.Methods {
					if strings.HasPrefix(m.Name, "_") {
						continue
					}
					methodNames = append(methodNames, m.Name)
					if len(methodNames) >= 3 {
						break
					}
				}
				state, _, _ := c.svc.ComputeConfigState(ctx, h.ID, active.InitArgsSchema)
				configState = state
			}
		}
		if len(methodNames) > 0 {
			desc += fmt.Sprintf(" Methods: %s.", strings.Join(methodNames, ", "))
		}
		if configState != "" && configState != handlerdomain.ConfigStateReady {
			desc += fmt.Sprintf(" configState: %s.", configState)
		}

		items = append(items, catalogdomain.Item{
			Source:      "handler",
			ID:          h.ID,
			Name:        h.Name,
			Description: desc,
			Category:    "service",
		})
	}
	return items, nil
}
