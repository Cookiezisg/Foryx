package document

import (
	"context"
	"strings"

	catalogdomain "github.com/sunweilin/forgify/backend/internal/domain/catalog"
)

// AsCatalogSource returns the CatalogSource port adapter for this Service.
//
// AsCatalogSource 返本 Service 的 CatalogSource port 适配器。
func (s *Service) AsCatalogSource() catalogdomain.CatalogSource {
	return &documentCatalogSource{svc: s}
}

type documentCatalogSource struct {
	svc *Service
}

func (c *documentCatalogSource) Name() string                           { return "document" }
func (c *documentCatalogSource) Granularity() catalogdomain.Granularity { return catalogdomain.PerItem }

// ListItems flattens every live doc into a catalog Item; Name = Path so the
// LLM sees tree position at a glance, Category = top-level folder so the
// Generator can group docs path-wise. UserID comes from ctx (chat runner
// sets it via reqctx; pipeline polling uses background ctx with DefaultLocalUserID).
//
// ListItems 把所有活跃文档摊平成 catalog Item;Name 用 Path 给 LLM 一眼
// 看树结构,Category 用顶层目录让 Generator 按 path 分组。UserID 从 ctx
// 取(chat runner 设;后台 polling 用 background ctx + DefaultLocalUserID)。
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
			Category:    topLevelSegment(d.Path),
		})
	}
	return items, nil
}

// topLevelSegment returns the first path component after the leading slash
// ("/Projects/Q1" → "Projects"); root-level docs ("/Notes") return "Notes"
// so each root doc forms its own one-item group.
//
// topLevelSegment 返 path 首段("/Projects/Q1" → "Projects");根级 doc
// ("/Notes") 返 "Notes" 自成一组。
func topLevelSegment(path string) string {
	p := strings.TrimPrefix(path, "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return p
}
