// catalog_source.go — Handler implements catalogdomain.CatalogSource so
// app/catalog can include handlers in the system-prompt summary. Per D9-1
// the item.Description embeds the computed ConfigState (so the LLM knows
// whether the handler is callable or needs configuration first).
//
// Granularity is PerItem (generator may freely group "5 PG handlers" etc.).
// Category is "service" (vs function's "computation" — handlers tend to be
// service-shaped: DB, API client, queue worker).
//
// catalog_source.go —— Handler 实现 catalogdomain.CatalogSource。per D9-1
// 在 Description 嵌 ConfigState 让 LLM 知是否可直接调还是需先配置。

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
// AsCatalogSource 返 Service 的 CatalogSource port 适配器。
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

		// Append method overview (first 3 names) so the LLM knows what the
		// handler can do without fetching its full detail.
		//
		// 加 method 名概览(前 3 个)让 LLM 知 handler 能做啥不必抓详情。
		var methodNames []string
		var configState string
		if h.ActiveVersionID != "" {
			active, _ := c.svc.repo.GetVersion(ctx, h.ActiveVersionID)
			if active != nil {
				for _, m := range active.Methods {
					if strings.HasPrefix(m.Name, "_") {
						continue // private
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
