package agent

import (
	"context"
	"strings"

	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
)

// AsCatalogSource returns the CatalogSource port adapter for this Service.
//
// AsCatalogSource 返本 Service 的 CatalogSource port 适配器。
func (s *Service) AsCatalogSource() catalogdomain.CatalogSource {
	return &agentCatalogSource{svc: s}
}

type agentCatalogSource struct {
	svc *Service
}

func (c *agentCatalogSource) Name() string                           { return "agent" }
func (c *agentCatalogSource) Granularity() catalogdomain.Granularity { return catalogdomain.PerItem }

// InvokeTool: get_agent is the actionable handle from the menu (inspect an agent before referencing
// it as config.agentRef in a workflow agent node).
func (c *agentCatalogSource) InvokeTool() string { return "get_agent" }

// ListItems returns agents for catalog injection (LLM system prompt capability menu).
func (c *agentCatalogSource) ListItems(ctx context.Context) ([]catalogdomain.Item, error) {
	agents, _, err := c.svc.List(ctx, 200, "")
	if err != nil {
		return nil, err
	}
	items := make([]catalogdomain.Item, 0, len(agents))
	for _, a := range agents {
		desc := strings.TrimSpace(a.Description)
		if desc == "" {
			if joined := strings.Join(a.Tags, ", "); joined != "" {
				desc = joined
			} else {
				desc = "(no description)"
			}
		}
		items = append(items, catalogdomain.Item{
			Source:      "agent",
			ID:          a.ID,
			Name:        a.Name,
			Description: desc,
		})
	}
	return items, nil
}
