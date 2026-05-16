package mcp

import (
	"context"
	"fmt"
	"strings"

	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

const maxToolsInDescription = 3

// AsCatalogSource returns this Service's CatalogSource port adapter.
//
// AsCatalogSource 返回本 Service 的 CatalogSource 适配器。
func (s *Service) AsCatalogSource() catalogdomain.CatalogSource {
	return &mcpCatalogSource{svc: s}
}

type mcpCatalogSource struct {
	svc *Service
}

func (c *mcpCatalogSource) Name() string                           { return "mcp" }
func (c *mcpCatalogSource) Granularity() catalogdomain.Granularity { return catalogdomain.PerServer }

func (c *mcpCatalogSource) ListItems(ctx context.Context) ([]catalogdomain.Item, error) {
	servers := c.svc.ListServers(ctx)
	items := make([]catalogdomain.Item, 0, len(servers))
	for _, srv := range servers {
		if srv.Status != mcpdomain.StatusReady && srv.Status != mcpdomain.StatusDegraded {
			continue
		}
		items = append(items, catalogdomain.Item{
			Source:      "mcp",
			ID:          srv.Name,
			Name:        srv.Name,
			Description: synthesizeServerDescription(srv),
		})
	}
	return items, nil
}

// synthesizeServerDescription builds a per-server description from tools/list.
//
// synthesizeServerDescription 用 tools/list 合成 per-server 描述。
func synthesizeServerDescription(srv mcpdomain.ServerStatus) string {
	if len(srv.Tools) == 0 {
		return fmt.Sprintf("%s server (no tools exposed)", srv.Name)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d tool", len(srv.Tools))
	if len(srv.Tools) != 1 {
		sb.WriteByte('s')
	}
	sb.WriteString(" (e.g. ")
	for i, td := range srv.Tools {
		if i >= maxToolsInDescription {
			fmt.Fprintf(&sb, ", ...+%d more", len(srv.Tools)-i)
			break
		}
		if i > 0 {
			sb.WriteString("; ")
		}
		desc := td.Description
		if len(desc) > 60 {
			desc = desc[:60] + "..."
		}
		fmt.Fprintf(&sb, "%s: %s", td.Name, desc)
	}
	sb.WriteByte(')')
	return sb.String()
}
