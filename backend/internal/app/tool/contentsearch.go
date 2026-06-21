package tool

import (
	"context"
	"strings"

	searchapp "github.com/sunweilin/anselm/backend/internal/app/search"
	searchdomain "github.com/sunweilin/anselm/backend/internal/domain/search"
)

// ContentSearch routes a vertical search tool's non-empty query through the
// unified content engine (FTS over name/description/tags AND body/code), and
// renders the tool's legacy slim list shape so the LLM-facing schema stays
// identical. ok=false (nil engine / empty query / engine error) tells the
// caller to fall back to its legacy substring path — the tool never breaks
// because the index is unavailable.
//
// ContentSearch 把垂搜工具的非空 query 路由到统一内容引擎（FTS 覆盖名/描述/tags
// **及正文/代码**），并渲染该工具原有的 slim 列表形状，LLM 所见 schema 不变。
// ok=false（引擎缺席/空 query/引擎出错）让调用方回退原子串路径——索引不可用时
// 工具绝不因此坏掉。
func ContentSearch(ctx context.Context, engine *searchapp.Service, t searchdomain.EntityType, query, listKey string) (string, bool) {
	if engine == nil || strings.TrimSpace(query) == "" {
		return "", false
	}
	page, err := engine.Search(ctx, &searchdomain.Query{
		Q: query, Types: []searchdomain.EntityType{t}, IncludeArchived: true, Limit: 20,
	})
	if err != nil {
		return "", false
	}
	out := make([]searchdomain.EntitySlim, 0, len(page.Hits))
	for _, h := range page.Hits {
		out = append(out, searchdomain.EntitySlim{ID: h.EntityID, Name: h.Name, Description: h.Snippet})
	}
	return ToJSON(SlimPageResult(len(out), page.Total, page.NextCursor, listKey, out)), true
}

// SlimPageResult renders a vertical search tool's slim list WITH truncation metadata: `count` is what
// was returned (≤ the engine page limit), `total` is the full match count, and nextCursor/hasMore
// signal that more results exist. Without total the LLM reads a 20-item list as "exactly 20 exist" —
// a silent false negative. The slim tool schemas take only `query`, so paging itself is REST-side;
// these fields just tell the agent the result was truncated. Centralized so every search tool
// discloses truncation identically (F175-M4).
//
// SlimPageResult 渲染垂搜工具的 slim 列表并带截断元数据：`count` 是本次返回数（≤引擎页上限），`total`
// 是全量匹配数，nextCursor/hasMore 示意还有更多。没 total，LLM 把 20 条读成「恰有 20 条」=静默假阴。
// slim 工具 schema 只收 query、翻页在 REST 侧；这些字段只告诉 agent 结果被截断了。集中一处，使每个搜索
// 工具的截断披露一致（F175-M4）。
func SlimPageResult(count, total int, nextCursor, listKey string, list any) map[string]any {
	res := map[string]any{"count": count, "total": total, listKey: list}
	if nextCursor != "" {
		res["nextCursor"] = nextCursor
		res["hasMore"] = true
	}
	return res
}
