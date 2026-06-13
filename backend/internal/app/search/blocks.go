package search

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	searchdomain "github.com/sunweilin/forgify/backend/internal/domain/search"
	tokencountpkg "github.com/sunweilin/forgify/backend/internal/pkg/tokencount"
)

const (
	blocksDefaultLimit = 8
	blocksMaxLimit     = 20

	// siftBudgetTokens is the direct-feed threshold (§7.4): a catalog this small
	// goes WHOLE to the utility model — lossless, maximum precision. Measured in
	// serialized tokens, not row count; a constant, not a setting.
	// siftBudgetTokens 是直喂阈值（§7.4）：目录小于它就**整体**交给 utility 模型——
	// 无损、最大精度。按序列化 token 计、非条数；常量、非配置。
	siftBudgetTokens = 4000
	// siftRetrieveTopK is tier 2's candidate pool: index retrieval narrows, the
	// utility model picks — token-bounded with near-lossless precision.
	// siftRetrieveTopK 是第二档候选池：索引收窄、utility 精选——token 有界、精度近无损。
	siftRetrieveTopK = 50
)

// Sifter is the utility-LLM port behind the precision chain: given a query and
// numbered items, return the indexes of the best matches (best first). nil or
// erroring sifters drop SearchBlocks to plain index retrieval — tier 3.
//
// Sifter 是精度链背后的 utility LLM 端口：给定 query 与编号条目，返回最佳匹配的下标
// （最佳在前）。sifter 为 nil 或出错时 SearchBlocks 落到纯索引检索——第三档。
type Sifter interface {
	Sift(ctx context.Context, query string, items []string, topN int) ([]int, error)
}

// SetSifter wires the utility-model sifter (bootstrap).
//
// SetSifter 接入 utility 模型精选器（bootstrap）。
func (s *Service) SetSifter(f Sifter) { s.sifter = f }

// BlockHit is one wireable palette result: Ref drops straight into a workflow
// node (fn_<id> / hd_<id>.<method> / mcp:<server>/<tool> / agent / control /
// approval ids).
//
// BlockHit 是一个可接线的面板结果：Ref 直接可填 workflow 节点（fn_<id> /
// hd_<id>.<method> / mcp:<server>/<tool> / agent/control/approval id）。
type BlockHit struct {
	Ref      string `json:"ref"`
	Kind     string `json:"kind"`
	EntityID string `json:"entityId"`
	Name     string `json:"name"`
	Snippet  string `json:"snippet,omitempty"`
}

// SearchBlocks is the LLM palette query (§7.4): six block kinds only, folded
// per (entity, anchor) so each handler method / mcp tool is its own hit, and
// every result carries a wireable ref. Hits without one (an mcp server card)
// are dropped — un-wireable results are noise here.
//
// SearchBlocks 是 LLM 积木面板查询（§7.4）：仅六类积木、按 (entity, anchor) 折叠
// （每个 handler 方法 / mcp 工具各自成命中）、每条结果带可接线 ref。没有 ref 的
// 命中（mcp server 卡）丢弃——不可接线的结果在这里是噪声。
func (s *Service) SearchBlocks(ctx context.Context, query string, kinds []searchdomain.EntityType, limit int) ([]BlockHit, error) {
	if strings.TrimSpace(query) == "" {
		return nil, searchdomain.ErrQueryRequired
	}
	if len(kinds) == 0 {
		kinds = searchdomain.BlockEntityTypes
	}
	for _, k := range kinds {
		if !searchdomain.IsBlockEntityType(k) {
			return nil, searchdomain.ErrTypeInvalid
		}
	}
	if limit <= 0 {
		limit = blocksDefaultLimit
	}
	if limit > blocksMaxLimit {
		limit = blocksMaxLimit
	}
	// Three-tier precision chain (§7.4). Tier 1: a catalog under the token
	// budget goes whole to the utility model — nothing the index might miss.
	// 三段精度链（§7.4）。第一档：目录在 token 预算内就整体直喂 utility——索引可能
	// 漏的它不会漏。
	if s.sifter != nil {
		if rows, err := s.repo.BlockRows(ctx); err == nil {
			catalog := filterBlockRows(rows, kinds)
			if len(catalog) > 0 && catalogTokens(catalog) <= siftBudgetTokens {
				out, err := s.sift(ctx, query, catalog, limit)
				if err == nil {
					return out, nil
				}
				// Sifter failure falls through to index retrieval (tier 3 via tier 2);
				// the error must be visible or a dead utility wire looks like ranking.
				// 精选失败落到索引检索（经第二档到第三档）；错误必须可见，否则 utility
				// 断线会伪装成普通排序。
				s.log.Info("search_blocks: tier-1 sift failed", zap.Error(err))
			}
		}
	}

	hits, err := s.window(ctx, &searchdomain.Query{Q: query, Types: kinds, IncludeArchived: true}, false)
	if err != nil {
		return nil, err
	}
	wireable := make([]*searchdomain.Hit, 0, len(hits))
	for _, h := range hits {
		if h.RefHint != "" {
			wireable = append(wireable, h)
		}
	}
	// Tier 2: index narrows to top-50, the utility model picks — bounded tokens,
	// near-lossless precision. Tier 3 (no sifter / sift error): plain retrieval.
	// 第二档：索引收窄 top-50、utility 精选——token 有界、精度近无损。第三档
	//（无 sifter / 精选失败）：纯检索。
	if s.sifter != nil && len(wireable) > 0 {
		pool := wireable
		if len(pool) > siftRetrieveTopK {
			pool = pool[:siftRetrieveTopK]
		}
		candidates := make([]*searchdomain.DocHit, 0, len(pool))
		for _, h := range pool {
			candidates = append(candidates, &searchdomain.DocHit{
				EntityType: h.EntityType, EntityID: h.EntityID, Anchor: h.Anchor,
				Title: h.Name, Snippet: h.Snippet,
			})
		}
		out, err := s.sift(ctx, query, candidates, limit)
		if err == nil {
			return out, nil
		}
		s.log.Info("search_blocks: sift unavailable, returning index ranking", zap.Error(err))
	}
	out := make([]BlockHit, 0, limit)
	for _, h := range wireable {
		out = append(out, BlockHit{
			Ref:      h.RefHint,
			Kind:     string(h.EntityType),
			EntityID: h.EntityID,
			Name:     h.Name,
			Snippet:  h.Snippet,
		})
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

// filterBlockRows keeps wireable rows of the requested kinds (an mcp server
// card has no ref and is noise here).
//
// filterBlockRows 留下所请求 kind 的可接线行（mcp server 卡无 ref，在此是噪声）。
func filterBlockRows(rows []*searchdomain.DocHit, kinds []searchdomain.EntityType) []*searchdomain.DocHit {
	want := map[searchdomain.EntityType]bool{}
	for _, k := range kinds {
		want[k] = true
	}
	out := make([]*searchdomain.DocHit, 0, len(rows))
	for _, r := range rows {
		if want[r.EntityType] && searchdomain.RefHint(r.EntityType, r.EntityID, r.Anchor) != "" {
			out = append(out, r)
		}
	}
	return out
}

func catalogTokens(rows []*searchdomain.DocHit) int {
	total := 0
	for _, r := range rows {
		total += tokencountpkg.Estimate(renderBlockItem(r))
	}
	return total
}

func renderBlockItem(r *searchdomain.DocHit) string {
	ref := searchdomain.RefHint(r.EntityType, r.EntityID, r.Anchor)
	return fmt.Sprintf("[%s] %s (ref:%s) %s", r.EntityType, r.Title, ref, r.Snippet)
}

// sift asks the utility model to pick the best blocks from numbered items and
// maps the answer back to wireable hits.
//
// sift 让 utility 模型从编号条目中精选，并把答案映回可接线命中。
func (s *Service) sift(ctx context.Context, query string, rows []*searchdomain.DocHit, limit int) ([]BlockHit, error) {
	items := make([]string, len(rows))
	for i, r := range rows {
		items[i] = renderBlockItem(r)
	}
	picks, err := s.sifter.Sift(ctx, query, items, limit)
	if err != nil {
		return nil, err
	}
	out := make([]BlockHit, 0, limit)
	seen := map[int]bool{}
	for _, p := range picks {
		if p < 0 || p >= len(rows) || seen[p] {
			continue
		}
		seen[p] = true
		r := rows[p]
		out = append(out, BlockHit{
			Ref:      searchdomain.RefHint(r.EntityType, r.EntityID, r.Anchor),
			Kind:     string(r.EntityType),
			EntityID: r.EntityID,
			Name:     r.Title,
			Snippet:  r.Snippet,
		})
		if len(out) == limit {
			break
		}
	}
	return out, nil
}
