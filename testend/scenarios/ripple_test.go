// ripple_test.go — R5（A10 跨域涟漪矩阵机械表）。
//
// PLAN A10：{创建/改名/删除} × 12 实体 → {搜索索引/关系图/catalog/通知/挂载方/引用方}
// 六涟漪面。本文件机械扫三个**逐实体全扫面**（搜索 exact-name / 通知三族 / catalog 进出与
// 跟名）+ 两个**代表性深面**（关系图边集与水化跟名、引用方 capability-check 报缺）；
// name 即 id 的三类（skill/memory/mcp）改名列 N/A；挂载方跟名已在 R2（agent 重解析）实证。
// 完整 12×3×6 格台账（每格 测于哪/为何 N/A）见 findings.md R5 节。
package scenarios

import (
	"net/url"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// rpKind 是矩阵一行：建（rp1 名）、改名（rp2 名）、删，及该实体的通知域前缀。
type rpKind struct {
	kind   string
	domain string // 通知 type 前缀（<domain>.）。
	create func(t *testing.T, wc *harness.Client, name string) (id string)
	rename func(t *testing.T, wc *harness.Client, id, name string)
	del    func(t *testing.T, wc *harness.Client, id string)
}

// TestRippleR5_CreateRenameDeleteMatrix: 9 个可改名实体 × 3 操作 × 3 全扫面
// （搜索 exact-name 跟名、catalog 进/跟/出、通知 created/updated 族/deleted）。
func TestRippleR5_CreateRenameDeleteMatrix(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "ripple-ws"}).Field(t, "id")
	wc := c.WS(wsID)

	kinds := []rpKind{
		{
			kind: "function", domain: "function.",
			create: func(t *testing.T, wc *harness.Client, name string) string {
				return fnCreate(t, wc, name, "def "+name+"() -> dict:\n    return {}\n")
			},
			rename: func(t *testing.T, wc *harness.Client, id, name string) {
				wc.PATCH("/api/v1/functions/"+id, map[string]any{"name": name}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/functions/" + id).OK(t, nil)
			},
		},
		{
			kind: "handler", domain: "handler.",
			create: func(t *testing.T, wc *harness.Client, name string) string {
				return hdCreate(t, wc, name, map[string]any{
					"description": "ripple host", "initBody": "self.n = 0",
					"methods": []map[string]any{{"name": "m", "body": "return {\"ok\": True}", "description": "m"}},
				})
			},
			rename: func(t *testing.T, wc *harness.Client, id, name string) {
				wc.PATCH("/api/v1/handlers/"+id, map[string]any{"name": name}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/handlers/" + id).OK(t, nil)
			},
		},
		{
			kind: "agent", domain: "agent.",
			create: func(t *testing.T, wc *harness.Client, name string) string {
				return agCreate(t, wc, map[string]any{"name": name, "description": "rp", "prompt": "p"})
			},
			rename: func(t *testing.T, wc *harness.Client, id, name string) {
				wc.PATCH("/api/v1/agents/"+id, map[string]any{"name": name}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/agents/" + id).OK(t, nil)
			},
		},
		{
			kind: "control", domain: "control.",
			create: func(t *testing.T, wc *harness.Client, name string) string {
				return nestedID(t, wc.POST("/api/v1/controls", map[string]any{
					"name": name, "description": "rp",
					"inputs":   []map[string]any{{"name": "x", "type": "number"}},
					"branches": []map[string]any{{"port": "out", "when": "true"}},
				}), "control")
			},
			rename: func(t *testing.T, wc *harness.Client, id, name string) {
				wc.PATCH("/api/v1/controls/"+id, map[string]any{"name": name}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/controls/" + id).OK(t, nil)
			},
		},
		{
			kind: "approval", domain: "approval.",
			create: func(t *testing.T, wc *harness.Client, name string) string {
				return nestedID(t, wc.POST("/api/v1/approvals", map[string]any{
					"name": name, "description": "rp", "template": "ok?",
				}), "approval")
			},
			rename: func(t *testing.T, wc *harness.Client, id, name string) {
				wc.PATCH("/api/v1/approvals/"+id, map[string]any{"name": name}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/approvals/" + id).OK(t, nil)
			},
		},
		{
			kind: "workflow", domain: "workflow.",
			create: func(t *testing.T, wc *harness.Client, name string) string {
				return wfCreate(t, wc, name, []map[string]any{
					{"op": "add_node", "node": map[string]any{"id": "t", "kind": "trigger", "ref": "trg_x"}},
					{"op": "add_node", "node": map[string]any{"id": "a", "kind": "action", "ref": "fn_x"}},
					{"op": "add_edge", "edge": map[string]any{"id": "e1", "from": "t", "to": "a"}},
				})
			},
			rename: func(t *testing.T, wc *harness.Client, id, name string) {
				wc.PATCH("/api/v1/workflows/"+id, map[string]any{"name": name}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/workflows/" + id).OK(t, nil)
			},
		},
		{
			kind: "trigger", domain: "", // trigger 无生命周期通知（events.md 言明）——通知面 N/A。
			create: func(t *testing.T, wc *harness.Client, name string) string {
				return trgCreate(t, wc, name, "webhook", map[string]any{
					"path": "rp-" + name, "secret": "s", "signatureAlgo": "hmac-sha256-hex",
				})
			},
			rename: func(t *testing.T, wc *harness.Client, id, name string) {
				wc.PATCH("/api/v1/triggers/"+id, map[string]any{"name": name}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/triggers/" + id).OK(t, nil)
			},
		},
		{
			kind: "document", domain: "document.",
			create: func(t *testing.T, wc *harness.Client, name string) string {
				return wc.POST("/api/v1/documents", map[string]any{"name": name, "content": "ripple body"}).Field(t, "id")
			},
			rename: func(t *testing.T, wc *harness.Client, id, name string) {
				wc.PATCH("/api/v1/documents/"+id, map[string]any{"name": name}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/documents/" + id).OK(t, nil)
			},
		},
		{
			kind: "conversation", domain: "conversation.",
			create: func(t *testing.T, wc *harness.Client, name string) string {
				return wc.POST("/api/v1/conversations", map[string]any{"title": name}).Field(t, "id")
			},
			rename: func(t *testing.T, wc *harness.Client, id, name string) {
				wc.PATCH("/api/v1/conversations/"+id, map[string]any{"title": name}).OK(t, nil)
			},
			del: func(t *testing.T, wc *harness.Client, id string) {
				wc.DELETE("/api/v1/conversations/" + id).OK(t, nil)
			},
		},
	}

	// catalogHas: coverage 任一 source 含 id。conversation/document 不入 catalog（能力菜单
	// 只收积木/内容工具面）——按 kind 决定断言。
	type catalogShape struct {
		Summary  string              `json:"summary"`
		Coverage map[string][]string `json:"coverage"`
	}
	catalogHas := func(id string) bool {
		var cat catalogShape
		wc.GET("/api/v1/catalog").OK(t, &cat)
		for _, ids := range cat.Coverage {
			for _, x := range ids {
				if x == id {
					return true
				}
			}
		}
		return false
	}
	catalogSummaryHas := func(name string) bool {
		var cat catalogShape
		wc.GET("/api/v1/catalog").OK(t, &cat)
		return strings.Contains(cat.Summary, name)
	}
	searchExact := func(name, kind, id string) bool {
		p := searchQ(t, wc, "q="+url.QueryEscape(name)+"&types="+kind+"&limit=10")
		for _, h := range p.Hits {
			if h.EntityID == id {
				return true
			}
		}
		return false
	}
	notifHas := func(prefix, suffix string) bool {
		var rows []struct {
			Type string `json:"type"`
		}
		wc.GET("/api/v1/notifications?limit=200").OK(t, &rows)
		for _, n := range rows {
			if strings.HasPrefix(n.Type, prefix) && strings.HasSuffix(n.Type, suffix) {
				return true
			}
		}
		return false
	}

	for _, k := range kinds {
		k := k
		name1, name2 := "rp1"+k.kind, "rp2"+k.kind
		id := k.create(t, wc, name1)
		inCatalog := k.kind != "document" && k.kind != "conversation" && k.kind != "trigger"
		// function/handler 的旧名随代码体常驻（def/类名，by design）——旧名清除只对
		// 名字不进正文的实体断言。
		nameOnlyInTitle := k.kind != "function" && k.kind != "handler"

		// 建：索引 exact-name + catalog 进 + <域>.created。
		harness.Eventually(t, 20000, k.kind+" create ripples (search)", func() bool {
			return searchExact(name1, k.kind, id)
		})
		if inCatalog && !catalogHas(id) {
			t.Errorf("%s: created entity must enter the catalog coverage", k.kind)
		}
		if k.domain != "" {
			harness.Eventually(t, 10000, k.kind+" created notification", func() bool {
				return notifHas(k.domain, "created")
			})
		}

		// 改名：索引跟名（标题面新名命中；纯标题实体旧名同时出局）+ catalog summary 跟名。
		k.rename(t, wc, id, name2)
		harness.Eventually(t, 20000, k.kind+" rename ripples (search follows)", func() bool {
			if !searchExact(name2, k.kind, id) {
				return false
			}
			if nameOnlyInTitle && searchExact(name1, k.kind, id) {
				return false
			}
			return true
		})
		if inCatalog {
			harness.Eventually(t, 10000, k.kind+" rename ripples (catalog follows)", func() bool {
				return catalogSummaryHas(name2)
			})
		}

		// 删：索引出 + catalog 出 + <域>.deleted。
		k.del(t, wc, id)
		harness.Eventually(t, 20000, k.kind+" delete ripples (search evicts)", func() bool {
			return !searchExact(name2, k.kind, id)
		})
		if inCatalog && catalogHas(id) {
			t.Errorf("%s: deleted entity must leave the catalog", k.kind)
		}
		if k.domain != "" {
			harness.Eventually(t, 10000, k.kind+" deleted notification", func() bool {
				return notifHas(k.domain, "deleted")
			})
		}
	}
}

// TestRippleR5_RelationGraphFaces: 关系图面——agent 五类挂载出边齐、trigger↔workflow 绑定边、
// conversation @mention 边、document wikilink 边；改名水化跟随（图存 id 名字读时取）；
// 删除中心实体边集级联清。
func TestRippleR5_RelationGraphFaces(t *testing.T) {
	srv := harness.Start(t)
	mock := harness.NewLLMMock(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "rel-ws"}).Field(t, "id")
	wc := c.WS(wsID)
	script := writeScriptedMCP(t)

	// 素材：fn / hd / mcp / doc / skill，全挂到一个 agent 上。
	fnID := fnCreate(t, wc, "rel_fn", "def rel_fn() -> dict:\n    return {}\n")
	hdID := hdCreate(t, wc, "rel_hd", map[string]any{
		"description": "d", "initBody": "self.n = 0",
		"methods": []map[string]any{{"name": "m", "body": "return {\"ok\": True}", "description": "m"}},
	})
	wc.PUT("/api/v1/mcp-servers/relmcp", map[string]any{
		"description": "d", "command": "python3", "args": []string{script},
	}).OK(t, nil)
	var st mcpStatus
	wc.GET("/api/v1/mcp-servers/relmcp").OK(t, &st)
	docID := wc.POST("/api/v1/documents", map[string]any{"name": "rel_doc", "content": "knowledge"}).Field(t, "id")
	wc.POST("/api/v1/skills", map[string]any{"name": "rel_skill", "description": "d", "body": "b"}).OK(t, nil)

	agID := agCreate(t, wc, map[string]any{
		"name": "Rel Worker", "description": "d", "prompt": "p",
		"skill": "rel_skill", "knowledge": []string{docID},
		"tools": []map[string]any{
			{"ref": fnID, "name": "fn"},
			{"ref": hdID + ".m", "name": "hd"},
			{"ref": "mcp:relmcp/" + st.Tools[0].Name, "name": "mcp"},
		},
	})

	neighborhood := func(kind, id string) string {
		r := wc.GET("/api/v1/relations/neighborhood?kind=" + kind + "&id=" + url.QueryEscape(id) + "&depth=1")
		if r.Status != 200 {
			t.Fatalf("neighborhood %s/%s: %d %s", kind, id, r.Status, r.Raw)
		}
		return string(r.Data)
	}

	// agent 出边：五类挂载全在邻域（hd 剥 .method、mcp 剥 /tool 归容器实体）。
	harness.Eventually(t, 15000, "agent equip edges complete", func() bool {
		n := neighborhood("agent", agID)
		return strings.Contains(n, fnID) && strings.Contains(n, hdID) &&
			strings.Contains(n, "relmcp") && strings.Contains(n, docID) && strings.Contains(n, "rel_skill")
	})

	// 水化跟名：改 fn 名 → 邻域显示新名（图存 id、名字读时取）。
	wc.PATCH("/api/v1/functions/"+fnID, map[string]any{"name": "rel_fn_renamed"}).OK(t, nil)
	harness.Eventually(t, 10000, "neighborhood hydrates the live name", func() bool {
		return strings.Contains(neighborhood("agent", agID), "rel_fn_renamed")
	})

	// trigger↔workflow 绑定边。
	trgID := trgCreate(t, wc, "rel_trg", "webhook", map[string]any{
		"path": "rel-in", "secret": "s", "signatureAlgo": "hmac-sha256-hex",
	})
	wfID, _ := wfWithTrigger(t, wc, "rel_wf", trgID)
	harness.Eventually(t, 15000, "trigger-workflow binding edge", func() bool {
		return strings.Contains(neighborhood("trigger", trgID), wfID)
	})

	// （@mention 不产 relation 边——快照非引用，by design：conversation 的 relations
	// 只有 purge+Namer。该格记 N/A，见 findings R5 矩阵。）
	_ = mock
	_ = wsID

	// document wikilink 边（按 id 链接）：A [[<docID>]] → A→doc 的 link 边。
	aID := wc.POST("/api/v1/documents", map[string]any{
		"name": "rel_linker", "content": "see [[" + docID + "]] for details",
	}).Field(t, "id")
	harness.Eventually(t, 15000, "document wikilink edge", func() bool {
		return strings.Contains(neighborhood("document", aID), docID)
	})

	// 删除中心实体 → 其边集级联清（agent 邻域消失）。
	wc.DELETE("/api/v1/agents/" + agID).OK(t, nil)
	harness.Eventually(t, 15000, "deleted agent's edges purge", func() bool {
		n := neighborhood("function", fnID)
		return !strings.Contains(n, agID)
	})
}

// TestRippleR5_ReferenceRipples: 引用方涟漪——workflow 引用的 fn 被删后 :capability-check
// 报缺（图不嘴硬）；恢复（重建同名新实体不救——ref 按 id）。
func TestRippleR5_ReferenceRipples(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "refrip-ws"}).Field(t, "id")
	wc := c.WS(wsID)

	fnID := fnCreate(t, wc, "ref_fn", "def ref_fn() -> dict:\n    return {}\n")
	wfID := wfCreate(t, wc, "ref_wf", []map[string]any{
		{"op": "add_node", "node": map[string]any{"id": "t", "kind": "trigger", "ref": "trg_manual"}},
		{"op": "add_node", "node": map[string]any{"id": "a", "kind": "action", "ref": fnID}},
		{"op": "add_edge", "edge": map[string]any{"id": "e1", "from": "t", "to": "a"}},
	})

	// 健康时体检干净。
	r := wc.POST("/api/v1/workflows/"+wfID+":capability-check", nil)
	if r.Status != 200 {
		t.Fatalf("capability-check: %d %s", r.Status, r.Raw)
	}
	healthy := string(r.Data)

	// 删被引用的 fn → 体检报缺（且含该节点/ref 线索）。
	wc.DELETE("/api/v1/functions/" + fnID).OK(t, nil)
	r = wc.POST("/api/v1/workflows/"+wfID+":capability-check", nil)
	if r.Status != 200 {
		t.Fatalf("capability-check after delete: %d %s", r.Status, r.Raw)
	}
	broken := string(r.Data)
	if broken == healthy || !strings.Contains(broken, fnID) {
		t.Fatalf("capability-check must report the dangling ref %s: %s", fnID, broken)
	}

	// 同名重建是新 id —— ref 按 id，体检仍缺（防"换皮自愈"假象）。
	fnNew := fnCreate(t, wc, "ref_fn", "def ref_fn() -> dict:\n    return {}\n")
	if fnNew == fnID {
		t.Fatal("recreated function must mint a new id")
	}
	r = wc.POST("/api/v1/workflows/"+wfID+":capability-check", nil)
	if !strings.Contains(string(r.Data), fnID) {
		t.Fatalf("same-name recreation must NOT satisfy the old ref: %s", r.Data)
	}
}
