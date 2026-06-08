package approval

import (
	"context"
	"strings"

	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
)

// AsCatalogSource exposes the approval-form library to the capability catalog (name +
// description only). approval forms are AI-facing work entities — strong name/description
// (the AI writes them) keeps the menu legible despite their per-workflow volume.
//
// AsCatalogSource 把审批表库暴露给能力 catalog（只 name + description）。审批表是面向 AI 的工作
// 实体——靠清晰的 name/description（AI 写）使菜单在 per-workflow 数量下仍可读。
func (s *Service) AsCatalogSource() catalogdomain.CatalogSource {
	return &approvalCatalogSource{svc: s}
}

type approvalCatalogSource struct{ svc *Service }

func (c *approvalCatalogSource) Name() string { return "approval" }

func (c *approvalCatalogSource) ListItems(ctx context.Context) ([]catalogdomain.Item, error) {
	forms, err := c.svc.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]catalogdomain.Item, 0, len(forms))
	for _, f := range forms {
		desc := strings.TrimSpace(f.Description)
		if desc == "" {
			desc = "(no description)"
		}
		items = append(items, catalogdomain.Item{Source: "approval", ID: f.ID, Name: f.Name, Description: desc})
	}
	return items, nil
}
