// search_lifecycle_test.go — R1（A7 高标准补全）：投影全周期 + 粒度 + 质量 + 同步自愈。
//
// PLAN A7 逐格：12 实体投影「建→搜到→改→新内容搜到&旧内容消失→删→搜不到」全周期；
// handler 方法 / mcp 工具锚点粒度（(entity,anchor) 即结果单元、refHint 可接线）；
// conversation 消息增量（DocAt）；排序三档（exact > prefix > 正文）与实体折叠
// （matchedChunks）；代码符号与中英混合查询；workspace 物理隔离；密文红线（加密
// 字段永不进投影）；杀进程丢事件 → boot 对账自愈。
package scenarios

import (
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// lcKind is one entity kind's lifecycle driver: create carrying tokenA, update
// swapping content to tokenB, delete. Queries assert per-phase index state.
//
// lcKind 是一类实体的全周期驱动：create 带 tokenA、update 换成 tokenB、delete。
// 查询逐相断言索引状态。
type lcKind struct {
	kind   string
	create func(t *testing.T, wc *harness.Client, tokenA string) (id string)
	update func(t *testing.T, wc *harness.Client, id, tokenB string)
	del    func(t *testing.T, wc *harness.Client, id string)
}

// nestedID decodes the {<entity>:{id}} create shape (AC-1 convention for
// versioned entities).
//
// nestedID 解 {<entity>:{id}} 创建形（版本实体的 AC-1 约定）。
func nestedID(t *testing.T, r *harness.Resp, entity string) string {
	t.Helper()
	// Create 现返裸实体(MD1):data 顶层即 id。entity 参数留作调用点自文档。
	_ = entity
	return r.Field(t, "id")
}

// TestSearchR1_ProjectionLifecycle12Kinds: 12 类实体逐个走 建→改→删 全周期，
// 每相都以唯一 token 查询钉死索引真随实体内容演化（增量 diff 投影非只 insert）。
func TestSearchR1_ProjectionLifecycle12Kinds(t *testing.T) {
	srv := harness.Start(t)
	mock := harness.NewLLMMock(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "lc-ws"}).Field(t, "id")
	wc := c.WS(wsID)
	script := writeScriptedMCP(t)

	kinds := []lcKind{
		{
			kind: "function",
			create: func(t *testing.T, wc *harness.Client, tok string) string {
				return fnCreate(t, wc, "lc_fn",
					"def f() -> dict:\n    \"\"\""+tok+" probe\"\"\"\n    return {}\n")
			},
			update: func(t *testing.T, wc *harness.Client, id, tok string) {
				wc.POST("/api/v1/functions/"+id+":edit", map[string]any{"ops": []map[string]any{
					{"op": "set_code", "code": "def f() -> dict:\n    \"\"\"" + tok + " probe\"\"\"\n    return {}\n"},
				}}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/functions/" + id).OK(t, nil)
			},
		},
		{
			kind: "handler",
			create: func(t *testing.T, wc *harness.Client, tok string) string {
				return hdCreate(t, wc, "lc_hd", map[string]any{
					"description": tok + " resident",
					"initBody":    "self.n = 0",
					"methods": []map[string]any{
						{"name": "ping", "body": "return {\"ok\": True}", "description": "ping"},
					},
				})
			},
			update: func(t *testing.T, wc *harness.Client, id, tok string) {
				wc.PATCH("/api/v1/handlers/"+id, map[string]any{"description": tok + " resident"}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/handlers/" + id).OK(t, nil)
			},
		},
		{
			kind: "agent",
			create: func(t *testing.T, wc *harness.Client, tok string) string {
				return nestedID(t, wc.POST("/api/v1/agents", map[string]any{
					"name": "lc_ag", "description": tok + " specialist", "prompt": "x",
				}), "agent")
			},
			update: func(t *testing.T, wc *harness.Client, id, tok string) {
				wc.PATCH("/api/v1/agents/"+id, map[string]any{"description": tok + " specialist"}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/agents/" + id).OK(t, nil)
			},
		},
		{
			kind: "control",
			create: func(t *testing.T, wc *harness.Client, tok string) string {
				return nestedID(t, wc.POST("/api/v1/controls", map[string]any{
					"name": "lc_ctl", "description": tok + " router",
					"inputs":   []map[string]any{{"name": "x", "type": "number"}},
					"branches": []map[string]any{{"port": "out", "when": "true"}},
				}), "control")
			},
			update: func(t *testing.T, wc *harness.Client, id, tok string) {
				wc.PATCH("/api/v1/controls/"+id, map[string]any{"description": tok + " router"}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/controls/" + id).OK(t, nil)
			},
		},
		{
			kind: "approval",
			create: func(t *testing.T, wc *harness.Client, tok string) string {
				return nestedID(t, wc.POST("/api/v1/approvals", map[string]any{
					"name": "lc_apf", "description": tok + " gate", "template": "approve?",
				}), "approval")
			},
			update: func(t *testing.T, wc *harness.Client, id, tok string) {
				wc.PATCH("/api/v1/approvals/"+id, map[string]any{"description": tok + " gate"}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/approvals/" + id).OK(t, nil)
			},
		},
		{
			kind: "workflow",
			create: func(t *testing.T, wc *harness.Client, tok string) string {
				return wc.POST("/api/v1/workflows", map[string]any{
					"name": "lc_wf", "description": tok + " pipeline",
					"ops": []map[string]any{
						{"op": "add_node", "node": map[string]any{"id": "t", "kind": "trigger", "ref": "trg_x"}},
						{"op": "add_node", "node": map[string]any{"id": "a", "kind": "action", "ref": "fn_x"}},
						{"op": "add_edge", "edge": map[string]any{"id": "e1", "from": "t", "to": "a"}},
					},
				}).Field(t, "id")
			},
			update: func(t *testing.T, wc *harness.Client, id, tok string) {
				wc.PATCH("/api/v1/workflows/"+id, map[string]any{"description": tok + " pipeline"}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/workflows/" + id).OK(t, nil)
			},
		},
		{
			kind: "trigger",
			create: func(t *testing.T, wc *harness.Client, tok string) string {
				return wc.POST("/api/v1/triggers", map[string]any{
					"name": "lc_trg", "description": tok + " listener", "kind": "webhook",
					"config": map[string]any{"path": "lc-in", "secret": "s1", "signatureAlgo": "hmac-sha256-hex"},
				}).Field(t, "id")
			},
			update: func(t *testing.T, wc *harness.Client, id, tok string) {
				wc.PATCH("/api/v1/triggers/"+id, map[string]any{"description": tok + " listener"}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/triggers/" + id).OK(t, nil)
			},
		},
		{
			kind: "document",
			create: func(t *testing.T, wc *harness.Client, tok string) string {
				return wc.POST("/api/v1/documents", map[string]any{
					"name": "lc_doc", "content": tok + " essay body",
				}).Field(t, "id")
			},
			update: func(t *testing.T, wc *harness.Client, id, tok string) {
				wc.PATCH("/api/v1/documents/"+id, map[string]any{"content": tok + " essay body"}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/documents/" + id).OK(t, nil)
			},
		},
		{
			kind: "memory",
			create: func(t *testing.T, wc *harness.Client, tok string) string {
				wc.PUT("/api/v1/memories/lc-mem", map[string]any{
					"description": "lifecycle probe", "content": tok + " remembered", "source": "user",
				}).OK(t, nil)
				return "lc-mem"
			},
			update: func(t *testing.T, wc *harness.Client, id, tok string) {
				wc.PUT("/api/v1/memories/"+id, map[string]any{
					"description": "lifecycle probe", "content": tok + " remembered", "source": "user",
				}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/memories/" + id).OK(t, nil)
			},
		},
		{
			kind: "skill", // name 即 id：PUT {name} 覆盖、无 PATCH。
			create: func(t *testing.T, wc *harness.Client, tok string) string {
				wc.POST("/api/v1/skills", map[string]any{
					"name": "lc_skill", "description": "lifecycle skill",
					"body": "Steps: use " + tok + " daily.",
				}).OK(t, nil)
				return "lc_skill"
			},
			update: func(t *testing.T, wc *harness.Client, id, tok string) {
				wc.PUT("/api/v1/skills/"+id, map[string]any{
					"description": "lifecycle skill",
					"body":        "Steps: use " + tok + " daily.",
				}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/skills/" + id).OK(t, nil)
			},
		},
		{
			kind: "conversation",
			create: func(t *testing.T, wc *harness.Client, tok string) string {
				return wc.POST("/api/v1/conversations", map[string]any{"title": tok + " chat"}).Field(t, "id")
			},
			update: func(t *testing.T, wc *harness.Client, id, tok string) {
				wc.PATCH("/api/v1/conversations/"+id, map[string]any{"title": tok + " chat"}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/conversations/" + id).OK(t, nil)
			},
		},
		{
			kind: "mcp",
			create: func(t *testing.T, wc *harness.Client, tok string) string {
				wc.PUT("/api/v1/mcp-servers/lcmcp", map[string]any{
					"description": tok + " external server",
					"command":     "python3", "args": []string{script},
				}).OK(t, nil)
				return "lcmcp"
			},
			update: func(t *testing.T, wc *harness.Client, id, tok string) {
				wc.PUT("/api/v1/mcp-servers/"+id, map[string]any{
					"description": tok + " external server",
					"command":     "python3", "args": []string{script},
				}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/mcp-servers/" + id).OK(t, nil)
			},
		},
	}
	_ = mock // mock 仅为真实环境形状（本测不驱动 LLM）。

	tokA := func(k string) string { return "lcalpha" + k }
	tokB := func(k string) string { return "lcbravo" + k }
	// hitKind: token 查询是否命中该 kind 的该实体。
	hitKind := func(kind, tok, id string) bool {
		p := searchQ(t, wc, "q="+url.QueryEscape(tok)+"&types="+kind+"&limit=20")
		for _, h := range p.Hits {
			if h.EntityID == id {
				return true
			}
		}
		return false
	}

	// Phase 1: create all with tokenA → all indexed. 相 1：全建 tokenA → 全可搜。
	ids := map[string]string{}
	for _, k := range kinds {
		ids[k.kind] = k.create(t, wc, tokA(k.kind))
	}
	for _, k := range kinds {
		k := k
		harness.Eventually(t, 20000, k.kind+" tokenA indexed after create", func() bool {
			return hitKind(k.kind, tokA(k.kind), ids[k.kind])
		})
	}

	// Phase 2: update all to tokenB → new content searchable, old token gone.
	// 相 2：全改 tokenB → 新内容可搜、旧 token 消失（diff 投影非只 insert）。
	for _, k := range kinds {
		k.update(t, wc, ids[k.kind], tokB(k.kind))
	}
	for _, k := range kinds {
		k := k
		harness.Eventually(t, 20000, k.kind+" tokenB indexed and tokenA evicted after update", func() bool {
			return hitKind(k.kind, tokB(k.kind), ids[k.kind]) && !hitKind(k.kind, tokA(k.kind), ids[k.kind])
		})
	}

	// Phase 3: delete all → gone from the index. 相 3：全删 → 索引除名。
	for _, k := range kinds {
		k.del(t, wc, ids[k.kind])
	}
	for _, k := range kinds {
		k := k
		harness.Eventually(t, 20000, k.kind+" evicted after delete", func() bool {
			return !hitKind(k.kind, tokB(k.kind), ids[k.kind])
		})
	}
}

// TestSearchR1_ConversationMessageIncremental: 对话消息增量投影（DocAt 单 message，
// anchor=message_id）——用户消息与 assistant 回复落库后各自可搜；删除对话全除名。
func TestSearchR1_ConversationMessageIncremental(t *testing.T) {
	wc, mock := chatSetup(t, false)
	mock.Enqueue(dlgModel, harness.LLMTurn{Text: "the assistant mentions glasswing here."})

	convID := convCreate(t, wc, "incremental probe")
	mid := sendMsg(t, wc, convID, "tell me about the term ironquill please")
	if turn := waitTurn(t, wc, convID, mid, 20000); turn.Status != "completed" {
		t.Fatalf("turn must complete, got %s", turn.Status)
	}

	// Both sides of the turn are individually indexed (incremental DocAt).
	// 回合两侧各自进索引（DocAt 增量）。
	for _, tok := range []string{"ironquill", "glasswing"} {
		tok := tok
		harness.Eventually(t, 20000, "message token "+tok+" searchable", func() bool {
			p := searchQ(t, wc, "q="+tok+"&types=conversation&limit=10")
			for _, h := range p.Hits {
				if h.EntityID == convID {
					return true
				}
			}
			return false
		})
	}

	// Snippets point into the conversation; anchors are message-level jump targets.
	// snippet 指回对话；anchor 是 message 级跳转锚。
	p := searchQ(t, wc, "q=ironquill&types=conversation&limit=10")
	if len(p.Hits) == 0 || p.Hits[0].Anchor == "" {
		t.Fatalf("conversation hit must carry a message anchor, got %+v", p.Hits)
	}

	wc.DELETE("/api/v1/conversations/" + convID).OK(t, nil)
	harness.Eventually(t, 20000, "deleted conversation evicted from index", func() bool {
		return len(searchQ(t, wc, "q=ironquill&types=conversation").Hits) == 0 &&
			len(searchQ(t, wc, "q=glasswing&types=conversation").Hits) == 0
	})
}

// TestSearchR1_GranularityAnchors: (entity, anchor) 粒度——handler 方法、mcp 工具各自
// 是结果单元，refHint 直接可填 workflow 节点（hd_<id>.<method> / mcp:<server>/<tool>）。
func TestSearchR1_GranularityAnchors(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "gran-ws"}).Field(t, "id")
	wc := c.WS(wsID)

	hdID := hdCreate(t, wc, "gran_hd", map[string]any{
		"description": "granularity host",
		"initBody":    "self.n = 0",
		"methods": []map[string]any{
			{"name": "ping", "body": "return {\"ok\": True}", "description": "granuping latency check"},
			{"name": "pong", "body": "return {\"ok\": True}", "description": "granupong echo back"},
		},
	})

	// Each handler method is its own hit with a wireable ref. 每个方法各自命中、ref 可接线。
	harness.Eventually(t, 20000, "handler method ping is its own anchored hit", func() bool {
		p := searchQ(t, wc, "q=granuping&types=handler&limit=10")
		for _, h := range p.Hits {
			if h.EntityID == hdID && h.Anchor == "ping" && h.RefHint == "hd_"+strings.TrimPrefix(hdID, "hd_")+".ping" {
				return true
			}
		}
		return false
	})
	p := searchQ(t, wc, "q=granupong&types=handler&limit=10")
	found := false
	for _, h := range p.Hits {
		if h.EntityID == hdID && h.Anchor == "pong" && strings.HasSuffix(h.RefHint, ".pong") {
			found = true
		}
	}
	if !found {
		t.Fatalf("method pong must be an anchored hit with wireable ref: %+v", p.Hits)
	}

	// MCP tools: each cached tool is an anchored hit with ref mcp:<server>/<tool>.
	// mcp 工具：缓存的每个工具各自命中、ref=mcp:<server>/<tool>。
	script := writeScriptedMCP(t)
	var st mcpStatus
	wc.PUT("/api/v1/mcp-servers/granmcp", map[string]any{
		"description": "granularity mcp", "command": "python3", "args": []string{script},
	}).OK(t, &st)
	if st.Status != "ready" || len(st.Tools) == 0 {
		t.Fatalf("scripted server must be ready with tools, got %s", st.Status)
	}
	toolName := st.Tools[0].Name
	harness.Eventually(t, 20000, "mcp tool is its own anchored hit with mcp ref", func() bool {
		p := searchQ(t, wc, "q="+url.QueryEscape(toolName)+"&types=mcp&limit=20")
		for _, h := range p.Hits {
			if h.Anchor == toolName && h.RefHint == "mcp:granmcp/"+toolName {
				return true
			}
		}
		return false
	})
}

// TestSearchR1_RankingPrefixAndFolding: 排序三档相对序（exact-name > name-prefix > 正文
// 命中）+ 综搜按实体折叠（多 chunk 命中合并为一条、matchedChunks 计数）。
func TestSearchR1_RankingPrefixAndFolding(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "rank-ws"}).Field(t, "id")
	wc := c.WS(wsID)

	// Three rank tiers for the same token. 同一 token 的三档。
	exactID := fnCreate(t, wc, "rankprobe", "def f() -> dict:\n    return {}\n")
	prefixID := fnCreate(t, wc, "rankprobe_extra", "def f() -> dict:\n    return {}\n")
	bodyID := wc.POST("/api/v1/documents", map[string]any{
		"name": "unrelated title", "content": "the body mentions rankprobe twice: rankprobe.",
	}).Field(t, "id")

	harness.Eventually(t, 20000, "exact > prefix > body relative order", func() bool {
		p := searchQ(t, wc, "q=rankprobe&limit=20")
		pos := map[string]int{}
		for i, h := range p.Hits {
			pos[h.EntityID] = i + 1 // 1-based; 0 = missing. 1 起；0=未命中。
		}
		return pos[exactID] != 0 && pos[prefixID] != 0 && pos[bodyID] != 0 &&
			pos[exactID] < pos[prefixID] && pos[prefixID] < pos[bodyID]
	})

	// Folding: a two-section markdown doc with the token in both sections is ONE hit
	// with matchedChunks ≥ 2. 折叠：双节 markdown 同 token → 单条命中、matchedChunks ≥ 2。
	filler := strings.Repeat("padding sentence to keep sections apart. ", 30)
	wc.POST("/api/v1/documents", map[string]any{
		"name":    "fold target",
		"content": "# part one\nchunkfold appears here. " + filler + "\n# part two\nchunkfold appears again. " + filler,
	}).OK(t, nil)
	harness.Eventually(t, 20000, "folded doc hit with matchedChunks>=2", func() bool {
		p := searchQ(t, wc, "q=chunkfold&types=document&limit=10")
		return len(p.Hits) == 1 && p.Hits[0].MatchedChunks >= 2
	})
}

// TestSearchR1_CodeSymbolAndMixedQuery: 代码符号子串（trigram 命中 snake_case 片段）+
// 中英混合查询（长 token MATCH 与短 CJK token LIKE 叠加、隐式 AND）。
func TestSearchR1_CodeSymbolAndMixedQuery(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "sym-ws"}).Field(t, "id")
	wc := c.WS(wsID)

	fnID := fnCreate(t, wc, "billing_rollup",
		"def billing_rollup(rows: list) -> dict:\n    total = compute_invoice_total(rows)\n    return {\"total\": total}\n")
	harness.Eventually(t, 20000, "code symbol substring hits the function", func() bool {
		p := searchQ(t, wc, "q=invoice_total&types=function&limit=10")
		for _, h := range p.Hits {
			if h.EntityID == fnID {
				return true
			}
		}
		return false
	})

	// Mixed-language query: long ascii token + 2-rune CJK token must BOTH match (AND).
	// 中英混合：长 ascii token + 2 rune CJK token 必须同时命中（AND）。
	wantID := wc.POST("/api/v1/documents", map[string]any{
		"name": "出行提示", "content": "明天的天气 forecast 显示有雨，预报建议带伞。",
	}).Field(t, "id")
	wc.POST("/api/v1/documents", map[string]any{
		"name": "decoy", "content": "this one only says forecast, no chinese weather word.",
	}).OK(t, nil)
	harness.Eventually(t, 20000, "mixed CJK+ascii query ANDs both tokens", func() bool {
		p := searchQ(t, wc, "q="+url.QueryEscape("forecast 预报")+"&types=document&limit=10")
		if len(p.Hits) != 1 {
			return false
		}
		return p.Hits[0].EntityID == wantID
	})
}

// TestSearchR1_WorkspaceIsolation: D2 物理隔离——两个 workspace 同 token，各自只见己方
// （infra/search 每条查询带显式 workspace 谓词的黑盒证明）。
func TestSearchR1_WorkspaceIsolation(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws1 := c.POST("/api/v1/workspaces", map[string]any{"name": "iso-1"}).Field(t, "id")
	ws2 := c.POST("/api/v1/workspaces", map[string]any{"name": "iso-2"}).Field(t, "id")
	wc1, wc2 := c.WS(ws1), c.WS(ws2)

	id1 := fnCreate(t, wc1, "isotoken_one", "def f() -> dict:\n    return {}\n")
	id2 := fnCreate(t, wc2, "isotoken_two", "def f() -> dict:\n    return {}\n")

	check := func(wc *harness.Client, wantID, intruderID string) func() bool {
		return func() bool {
			p := searchQ(t, wc, "q=isotoken&limit=20")
			seenWant := false
			for _, h := range p.Hits {
				if h.EntityID == intruderID {
					t.Fatalf("workspace isolation breached: foreign hit %s", intruderID)
				}
				if h.EntityID == wantID {
					seenWant = true
				}
			}
			return seenWant && p.Total == 1
		}
	}
	harness.Eventually(t, 20000, "ws1 sees only its own hit", check(wc1, id1, id2))
	harness.Eventually(t, 20000, "ws2 sees only its own hit", check(wc2, id2, id1))
}

// TestSearchR1_EncryptedRedline: 密文红线——经 Encryptor 落盘的字段（apikey 密文、trigger
// config、mcp env）永不进投影；明文名/描述照常可搜（正控）。
func TestSearchR1_EncryptedRedline(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "red-ws"}).Field(t, "id")
	wc := c.WS(wsID)
	script := writeScriptedMCP(t)

	wc.POST("/api/v1/api-keys", map[string]any{
		"provider": "openai", "displayName": "redline probe key", "key": "sk-redlinekeysecret",
	}).OK(t, nil)
	wc.POST("/api/v1/triggers", map[string]any{
		"name": "redline_hook", "kind": "webhook",
		"config": map[string]any{"path": "red-in", "secret": "redlinetrgsecret", "signatureAlgo": "hmac-sha256-hex"},
	}).OK(t, nil)
	wc.PUT("/api/v1/mcp-servers/redmcp", map[string]any{
		"description": "redline mcp probe", "command": "python3", "args": []string{script},
		"env": map[string]string{"TOKEN": "redlinemcpsecret"},
	}).OK(t, nil)

	// Positive control first: the trigger's plain name IS searchable — so "secret not
	// found" below means exclusion, not lag. 先正控：trigger 明文名可搜——下面的「密文
	// 搜不到」才是排除而非延迟。
	harness.Eventually(t, 20000, "plaintext trigger name searchable (positive control)", func() bool {
		return len(searchQ(t, wc, "q=redline_hook").Hits) > 0
	})
	harness.Eventually(t, 20000, "mcp description searchable (positive control)", func() bool {
		return len(searchQ(t, wc, "q=redmcp").Hits) > 0
	})

	for _, secret := range []string{"redlinekeysecret", "redlinetrgsecret", "redlinemcpsecret"} {
		if n := len(searchQ(t, wc, "q="+secret+"&limit=20").Hits); n != 0 {
			t.Fatalf("encrypted value %q leaked into the search index (%d hits)", secret, n)
		}
	}
}

// TestSearchR1_BootReconciliation: 写后通知是尽力而为（队满即丢）——杀进程窗口内的事件
// 可能永久丢失，boot 对账（stamps 比对）是唯一自愈。建实体后立刻 kill -9、重启，全部
// 必须最终可搜；重启后的新写照常入索（worker 活着）。
func TestSearchR1_BootReconciliation(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "boot-ws"}).Field(t, "id")
	wc := c.WS(wsID)

	// Burst-create then kill immediately — some Changed events die with the process.
	// 连发创建后立杀——部分 Changed 事件随进程死。
	var ids []string
	for i := 0; i < 5; i++ {
		ids = append(ids, fnCreate(t, wc, fmt.Sprintf("bootprobe_%d", i),
			"def f() -> dict:\n    \"\"\"bootreconcile target\"\"\"\n    return {}\n"))
	}
	srv.Kill9(t)
	srv.Restart(t)
	wc2 := srv.Client(t).WS(wsID)

	harness.Eventually(t, 30000, "boot reconciliation indexes all pre-crash entities", func() bool {
		p := searchPageOf(t, wc2, "q=bootreconcile&types=function&limit=20")
		seen := map[string]bool{}
		for _, h := range p.Hits {
			seen[h.EntityID] = true
		}
		for _, id := range ids {
			if !seen[id] {
				return false
			}
		}
		return true
	})

	// Post-restart writes flow normally. 重启后新写照常。
	wc2.POST("/api/v1/documents", map[string]any{"name": "afterboot", "content": "postcrash fresh doc"}).OK(t, nil)
	harness.Eventually(t, 20000, "post-restart write indexed", func() bool {
		return len(searchPageOf(t, wc2, "q=postcrash&types=document").Hits) > 0
	})
}

// searchPageOf mirrors searchQ for a non-default client (post-restart).
//
// searchPageOf 是 searchQ 的非默认 client 版（重启后用）。
func searchPageOf(t *testing.T, wc *harness.Client, params string) searchPage {
	t.Helper()
	var page searchPage
	r := wc.GET("/api/v1/search?" + params)
	r.OK(t, &page)
	page.NextCursor = r.NextCursor // 分页坐标在 envelope 顶层(Paged/MD2)
	return page
}
