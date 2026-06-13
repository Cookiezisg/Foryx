// search_test.go — W3 集成域：统一搜索全况。
//
// 词法层（trigram + 中文短词 LIKE 回退 + 注入安全）、实体投影矩阵、综搜/垂搜/过滤、
// 排序手感（exact-name boost）、窗口分页 cursor、reindex、settings 三态（builtin/ollama/off）
// 与 RAG 真下载真嵌入（语义跨语种命中 = 向量真融合的物理证明）。
package scenarios

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// searchPage is the GET /search wire shape.
//
// searchPage 是 GET /search 的线缆形状。
type searchPage struct {
	Hits []struct {
		EntityType    string  `json:"entityType"`
		EntityID      string  `json:"entityId"`
		Name          string  `json:"name"`
		Snippet       string  `json:"snippet"`
		Anchor        string  `json:"anchor"`
		RefHint       string  `json:"refHint"`
		MatchedChunks int     `json:"matchedChunks"`
		Score         float64 `json:"score"`
		Archived      bool    `json:"archived"`
	} `json:"hits"`
	NextCursor string `json:"nextCursor"`
	Total      int    `json:"total"`
}

// searchQ runs GET /search with raw query params and decodes the page.
//
// searchQ 以原始 query 参数跑 GET /search 并解包。
func searchQ(t *testing.T, wc *harness.Client, params string) searchPage {
	t.Helper()
	var page searchPage
	r := wc.GET("/api/v1/search?" + params)
	r.OK(t, &page)
	page.NextCursor = r.NextCursor // 分页坐标在 envelope 顶层(Paged/MD2),不在 data
	return page
}

// hitTypes collects the distinct entityTypes in a page.
//
// hitTypes 收集一页里出现的实体类型集合。
func hitTypes(p searchPage) map[string]bool {
	out := map[string]bool{}
	for _, h := range p.Hits {
		out[h.EntityType] = true
	}
	return out
}

// TestSearch_ProjectionsLexicalAndFilters: A7 主链——8 类实体投影进同一索引、综搜跨类命中、
// 中文短词（<3 rune）LIKE 回退、exact-name 置顶、垂搜/标签/归档过滤、注入安全、<mark> 高亮。
func TestSearch_ProjectionsLexicalAndFilters(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "search-omni"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	// Seed one of each kind, all carrying the unique token "sweepmark" somewhere.
	// 每类播一个，都在某处带唯一 token "sweepmark"。
	fnCreate(t, wc, "sweepmark_fn", "def f() -> dict:\n    \"\"\"sweepmark function probe\"\"\"\n    return {}\n")
	hdCreate(t, wc, "sweepmark_hd", map[string]any{
		"description": "sweepmark handler probe",
		"initBody":    "self.n = 0",
		"methods": []map[string]any{
			{"name": "ping", "body": "return {\"ok\": True}", "description": "ping"},
		},
	})
	wc.POST("/api/v1/documents", map[string]any{
		"name": "猫咪手册", "description": "sweepmark document probe",
		"content": "明天的天气预报说有雨，记得关注。sweepmark 内容段。",
		"tags":    []string{"manual"},
	}).OK(t, nil)
	wc.PUT("/api/v1/memories/sweepmark-memory", map[string]any{
		"description": "sweepmark memory probe",
		"content":     "sweepmark memory probe — remember the umbrella",
		"source":      "user",
	}).OK(t, nil)
	wc.POST("/api/v1/skills", map[string]any{
		"name": "sweepmark_skill", "description": "sweepmark skill probe",
		"body": "Steps for the daily sweepmark report.",
	}).OK(t, nil)
	convID := wc.POST("/api/v1/conversations", map[string]any{"title": "sweepmark conversation probe"}).Field(t, "id")
	trgID := trgCreate(t, wc, "sweepmark_trigger", "webhook", map[string]any{
		"path": "sweepmark-in", "secret": "s3", "signatureAlgo": "hmac-sha256-hex",
	})
	wfWithTrigger(t, wc, "sweepmark_flow", trgID)

	// Omni search converges across kinds (indexing is async — poll).
	// 综搜跨类收敛（索引异步——轮询）。
	want := []string{"function", "handler", "document", "memory", "skill", "conversation", "trigger", "workflow"}
	harness.Eventually(t, 15000, "omni search covers all seeded kinds", func() bool {
		got := hitTypes(searchQ(t, wc, "q=sweepmark&limit=50"))
		for _, k := range want {
			if !got[k] {
				return false
			}
		}
		return true
	})

	// Chinese short word (2 runes → trigram blind → LIKE fallback) + 1-rune name hit.
	// 中文短词（2 rune → trigram 盲区 → LIKE 回退）+ 1 rune 名字命中。
	harness.Eventually(t, 10000, "2-rune query falls back to LIKE", func() bool {
		return hitTypes(searchQ(t, wc, "q="+url.QueryEscape("天气")))["document"]
	})
	if !hitTypes(searchQ(t, wc, "q="+url.QueryEscape("猫")))["document"] {
		t.Fatal("1-rune query must hit the document name via LIKE fallback")
	}

	// Exact-name boost: an entity named exactly like the query outranks body mentions.
	// exact-name 置顶：与查询同名的实体排在正文命中前。
	fnExact := fnCreate(t, wc, "sweepmark", "def f() -> dict:\n    return {}\n")
	harness.Eventually(t, 10000, "exact-name hit ranks first", func() bool {
		p := searchQ(t, wc, "q=sweepmark&limit=50")
		return len(p.Hits) > 0 && p.Hits[0].EntityID == fnExact
	})

	// Vertical filter + invalid params. 垂搜过滤 + 非法参数。
	p := searchQ(t, wc, "q=sweepmark&types=function&limit=50")
	for k := range hitTypes(p) {
		if k != "function" {
			t.Fatalf("types=function leaked %s", k)
		}
	}
	wc.Do("GET", "/api/v1/search?q=sweepmark&types=spaceship", nil).Fail(t, 400, "SEARCH_TYPE_INVALID")
	wc.Do("GET", "/api/v1/search", nil).Fail(t, 400, "SEARCH_QUERY_REQUIRED")

	// Tags filter. 标签过滤。
	if !hitTypes(searchQ(t, wc, "q=sweepmark&tags=manual"))["document"] {
		t.Fatal("tags=manual must keep the tagged document")
	}
	if n := len(searchQ(t, wc, "q=sweepmark&tags=nosuchtag").Hits); n != 0 {
		t.Fatalf("tags=nosuchtag must filter everything, got %d", n)
	}

	// Archived: default in, includeArchived=false out. 归档：默认含、=false 排除。
	wc.PATCH("/api/v1/conversations/"+convID, map[string]any{"archived": true}).OK(t, nil)
	harness.Eventually(t, 10000, "archived conversation filtered by includeArchived=false", func() bool {
		in := hitTypes(searchQ(t, wc, "q=sweepmark&limit=50"))["conversation"]
		out := hitTypes(searchQ(t, wc, "q=sweepmark&limit=50&includeArchived=false"))["conversation"]
		return in && !out
	})

	// Injection safety: FTS5 metacharacters and SQL fragments must never 500.
	// 注入安全：FTS5 元字符与 SQL 片段永不 500。
	for _, q := range []string{`"sweepmark`, `sweep" OR "1`, `(SELECT *`, `*`, `-`, `'); DROP TABLE search_docs;--`, `天 OR 气`} {
		r := wc.Do("GET", "/api/v1/search?q="+url.QueryEscape(q), nil)
		if r.Status >= 500 {
			t.Fatalf("query %q must not 500: %d %s", q, r.Status, r.Raw)
		}
	}

	// Snippet highlights with <mark>. 摘要带 <mark> 高亮。
	p = searchQ(t, wc, "q=sweepmark&limit=50")
	marked := false
	for _, h := range p.Hits {
		if strings.Contains(h.Snippet, "<mark>") {
			marked = true
			break
		}
	}
	if !marked {
		t.Fatal("at least one snippet must carry <mark> highlighting")
	}
}

// TestSearch_PaginationWindow: A7 分页——物化窗口 cursor 走全、total 一致、异查询 cursor 被拒。
func TestSearch_PaginationWindow(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "search-page"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	const n = 25
	for i := 0; i < n; i++ {
		fnCreate(t, wc, fmt.Sprintf("pagerfn_%02d", i), "def f() -> dict:\n    \"\"\"pagination probe target\"\"\"\n    return {}\n")
	}
	harness.Eventually(t, 20000, "all 25 functions indexed", func() bool {
		return searchQ(t, wc, "q=pagination&limit=50").Total >= n
	})

	// Walk the window with limit=10: 10+10+5, distinct ids, stable total.
	// limit=10 走窗口：10+10+5、id 不重、total 稳定。
	seen := map[string]bool{}
	cursor := ""
	pages := 0
	for {
		params := "q=pagination&limit=10"
		if cursor != "" {
			params += "&cursor=" + url.QueryEscape(cursor)
		}
		p := searchQ(t, wc, params)
		if p.Total != n {
			t.Fatalf("total must stay %d across pages, got %d", n, p.Total)
		}
		for _, h := range p.Hits {
			if seen[h.EntityID] {
				t.Fatalf("duplicate hit across pages: %s", h.EntityID)
			}
			seen[h.EntityID] = true
		}
		pages++
		if p.NextCursor == "" {
			break
		}
		cursor = p.NextCursor
		if pages > 5 {
			t.Fatal("pagination never terminates")
		}
	}
	if len(seen) != n || pages != 3 {
		t.Fatalf("want %d hits over 3 pages, got %d over %d", n, len(seen), pages)
	}

	// A cursor replayed under a different query → rejected, not silently re-windowed.
	// cursor 换查询重放 → 拒绝、不静默换窗。
	first := searchQ(t, wc, "q=pagination&limit=10")
	wc.Do("GET", "/api/v1/search?q=different&cursor="+url.QueryEscape(first.NextCursor), nil).
		Fail(t, 400, "SEARCH_CURSOR_INVALID")
	wc.Do("GET", "/api/v1/search?q=pagination&cursor=garbage", nil).
		Fail(t, 400, "SEARCH_CURSOR_INVALID")
}

// TestSearch_ReindexAndSettings: A7 重建 + 设置三态——:reindex 202 后命中恢复；settings 回显；
// 非法 embedder 拒；off 关引擎仍可词法搜；ollama 不可达降级软着陆；空串重置默认。
func TestSearch_ReindexAndSettings(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "search-admin"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	fnCreate(t, wc, "reindex_probe_fn", "def f() -> dict:\n    \"\"\"reindexable probe\"\"\"\n    return {}\n")
	harness.Eventually(t, 10000, "probe indexed", func() bool {
		return len(searchQ(t, wc, "q=reindexable").Hits) > 0
	})

	wc.POST("/api/v1/search:reindex", nil).OK(t, nil)
	harness.Eventually(t, 15000, "hits return after reindex", func() bool {
		return len(searchQ(t, wc, "q=reindexable").Hits) > 0
	})

	// Settings round-trip. 设置往返。
	type settings struct {
		Embedder      string `json:"embedder"`
		OllamaBaseURL string `json:"ollamaBaseUrl"`
		OllamaModel   string `json:"ollamaModel"`
		Engine        struct {
			Status    string `json:"status"`
			Model     string `json:"model"`
			LastError string `json:"lastError"`
		} `json:"engine"`
	}
	var s settings
	wc.GET("/api/v1/search/settings").OK(t, &s)
	if s.Embedder != "builtin" {
		t.Fatalf("default embedder must be builtin, got %q", s.Embedder)
	}
	if s.OllamaBaseURL == "" || s.OllamaModel == "" {
		t.Fatalf("ollama params must echo effective defaults, got %+v", s)
	}

	wc.Do("PATCH", "/api/v1/search/settings", map[string]any{"embedder": "bogus"}).
		Fail(t, 400, "SEARCH_EMBEDDER_INVALID")

	// off: engine reports off, lexical search keeps working. off：引擎报 off、词法照常。
	wc.PATCH("/api/v1/search/settings", map[string]any{"embedder": "off"}).OK(t, &s)
	if s.Engine.Status != "off" {
		t.Fatalf("off embedder must report engine off, got %+v", s.Engine)
	}
	if len(searchQ(t, wc, "q=reindexable").Hits) == 0 {
		t.Fatal("lexical search must survive embedder=off")
	}

	// ollama pointed at a dead port: settings apply, search degrades soft (never breaks).
	// ollama 指死端口：设置生效、搜索软降级（绝不坏）。
	wc.PATCH("/api/v1/search/settings", map[string]any{
		"embedder": "ollama", "ollamaBaseUrl": "http://127.0.0.1:9", "ollamaModel": "nope",
	}).OK(t, &s)
	if s.Embedder != "ollama" || s.OllamaBaseURL != "http://127.0.0.1:9" {
		t.Fatalf("ollama params must apply, got %+v", s)
	}
	if len(searchQ(t, wc, "q=reindexable").Hits) == 0 {
		t.Fatal("search must degrade to lexical with unreachable ollama")
	}

	// Empty string resets to the domain default. 空串重置回域默认。
	wc.PATCH("/api/v1/search/settings", map[string]any{"ollamaBaseUrl": "", "ollamaModel": ""}).OK(t, &s)
	if !strings.Contains(s.OllamaBaseURL, "127.0.0.1:11434") || s.OllamaModel != "embeddinggemma" {
		t.Fatalf("empty strings must reset ollama defaults, got %+v", s)
	}

	wc.PATCH("/api/v1/search/settings", map[string]any{"embedder": "builtin"}).OK(t, &s)
	if s.Embedder != "builtin" {
		t.Fatalf("switch back to builtin failed: %+v", s)
	}
}

// TestSearch_SemanticRAGBuiltin: A7 语义层真货——builtin 引擎真下载（llama-server + EmbeddingGemma
// GGUF）、真 spawn、真嵌入；跨语种零词法重叠命中 = RRF 向量融合的物理证明。首跑下载吃
// harness 缓存，之后秒回。
func TestSearch_SemanticRAGBuiltin(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "search-rag"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	// Chinese-only document; the query will be English-only — zero lexical overlap by design.
	// 纯中文文档；查询纯英文——刻意零词法重叠。
	wc.POST("/api/v1/documents", map[string]any{
		"name":    "出行备忘",
		"content": "广州明天大概率下雨，出门记得带伞，最好提前查看降雨概率。",
	}).OK(t, nil)

	// The embed worker kicks on index writes; poke it with cheap writes while the engine
	// installs (download → spawn → healthy). First run pays the model download.
	// embed worker 由索引写 kick；引擎安装期间用廉价写持续踢（下载 → spawn → 健康）。
	// 首跑买模型下载的单。
	poke := 0
	harness.Eventually(t, 600000, "builtin engine becomes ready (real download)", func() bool {
		var s struct {
			Engine struct {
				Status    string `json:"status"`
				LastError string `json:"lastError"`
			} `json:"engine"`
		}
		wc.GET("/api/v1/search/settings").OK(t, &s)
		if s.Engine.Status == "ready" {
			return true
		}
		poke++
		wc.PUT("/api/v1/memories/rag-poke", map[string]any{
			"description": "embed worker kick probe", "source": "user",
			"content": fmt.Sprintf("kick the embed worker %d", poke),
		})
		return false
	})

	// Cross-lingual semantic hit: English query finds the Chinese document via vectors.
	// 跨语种语义命中：英文查询经向量找到中文文档。
	harness.Eventually(t, 120000, "semantic cross-lingual hit lands", func() bool {
		p := searchQ(t, wc, "q="+url.QueryEscape("rain umbrella tomorrow forecast")+"&types=document&limit=10")
		for _, h := range p.Hits {
			if h.Name == "出行备忘" {
				return true
			}
		}
		// keep kicking — vectors may still be backfilling. 继续踢——向量可能还在补算。
		poke++
		wc.PUT("/api/v1/memories/rag-poke", map[string]any{
			"description": "embed worker kick probe", "source": "user",
			"content": fmt.Sprintf("kick the embed worker %d", poke),
		})
		return false
	})
}
