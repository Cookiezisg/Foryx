// search_llm_test.go — R1（A7 高标准补全）：搜索的 LLM 面。
//
// PLAN A7「LLM 口」整面：8 个 search_<entity> 垂搜工具真走统一内容引擎（正文/代码命中、
// 非只名字子串）；search_blocks 三段精度链逐档真触发（①目录整喂 utility ②超阈索引收窄
// top-50 再精选 ③utility 缺席落纯索引），含六类积木铁律（document/skill 永不出现）与
// (entity,anchor) 可接线 ref；search_conversations 回忆窗（snippet+id、绝不全文）。
// 一切断言以 llmmock 的 PromptDump（模型线缆视角）+ 工具回喂消息为物理证据。
package scenarios

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// driveTool 让 LLM 在一个新对话里调一次 name(args)，返回回喂给模型的 tool 消息内容
// （第二请求的 role=tool）。每次调用独立对话，互不污染队列。
func driveTool(t *testing.T, wc *harness.Client, mock *harness.LLMMock, name string, args map[string]any) string {
	t.Helper()
	full := map[string]any{"summary": "drive " + name, "danger": "safe", "execution_group": 1}
	for k, v := range args {
		full[k] = v
	}
	mock.Enqueue(dlgModel,
		harness.LLMTurn{ToolCalls: []harness.MockToolCall{{Name: name, Args: full}}},
		harness.LLMTurn{Text: "done."},
	)
	base := len(mock.DumpsFor(dlgModel))
	convID := convCreate(t, wc, "drive "+name)
	mid := sendMsg(t, wc, convID, "use the tool")
	turn := waitTurn(t, wc, convID, mid, 30000)
	if turn.Status != "completed" {
		t.Fatalf("%s turn must complete, got %s err=%s/%s", name, turn.Status, turn.ErrorCode, turn.ErrorMessage)
	}
	dumps := mock.DumpsFor(dlgModel)
	if len(dumps)-base < 2 {
		t.Fatalf("%s: expected tool roundtrip (2 requests), got %d", name, len(dumps)-base)
	}
	for _, m := range dumps[base+1].Messages {
		if m.Role == "tool" {
			return m.Content
		}
	}
	t.Fatalf("%s: no tool message fed back: %+v", name, dumps[base+1].Messages)
	return ""
}

// waitIndexed 等 token 在 HTTP 综搜可见（索引异步，先钉事实再驱动 LLM 防 flake）。
func waitIndexed(t *testing.T, wc *harness.Client, token, types string) {
	t.Helper()
	harness.Eventually(t, 20000, token+" indexed", func() bool {
		return len(searchQ(t, wc, "q="+token+"&types="+types+"&limit=10").Hits) > 0
	})
}

// TestSearchLLM_VerticalToolsContentEngine: 8 垂搜工具逐个真驱动——查询词只在正文/代码/
// 描述里出现（名字不含），命中即证明工具走统一内容引擎而非旧名字子串路径；slim 形状
// {count, <list>: [{id,name,description}]} 不变（保 schema 换引擎）。
func TestSearchLLM_VerticalToolsContentEngine(t *testing.T) {
	wc, mock := chatSetup(t, false)

	// Forge one entity per vertical, match-token strictly in body. 每垂一实体、token 只在正文。
	fnID := fnCreate(t, wc, "vt_fn",
		"def vt_fn(rows: list) -> dict:\n    \"\"\"ledgerreconcile quarterly books\"\"\"\n    return {}\n")
	hdID := hdCreate(t, wc, "vt_hd", map[string]any{
		"description": "parcelrouting dispatch core", "initBody": "self.n = 0",
		"methods": []map[string]any{{"name": "route", "body": "return {\"ok\": True}", "description": "route"}},
	})
	agID := nestedID(t, wc.POST("/api/v1/agents", map[string]any{
		"name": "vt_ag", "description": "sentimentdistill review expert", "prompt": "x",
	}), "agent")
	ctlID := nestedID(t, wc.POST("/api/v1/controls", map[string]any{
		"name": "vt_ctl", "description": "thresholdgate amount router",
		"inputs":   []map[string]any{{"name": "x", "type": "number"}},
		"branches": []map[string]any{{"port": "out", "when": "true"}},
	}), "control")
	apfID := nestedID(t, wc.POST("/api/v1/approvals", map[string]any{
		"name": "vt_apf", "description": "budgetsignoff spend gate", "template": "ok?",
	}), "approval")
	wfID := wc.POST("/api/v1/workflows", map[string]any{
		"name": "vt_wf", "description": "nightlydigest sync pipeline",
		"ops": []map[string]any{
			{"op": "add_node", "node": map[string]any{"id": "t", "kind": "trigger", "ref": "trg_x"}},
			{"op": "add_node", "node": map[string]any{"id": "a", "kind": "action", "ref": "fn_x"}},
			{"op": "add_edge", "edge": map[string]any{"id": "e1", "from": "t", "to": "a"}},
		},
	}).Field(t, "id")
	trgID := wc.POST("/api/v1/triggers", map[string]any{
		"name": "vt_trg", "description": "webhookpulse inbound listener", "kind": "webhook",
		"config": map[string]any{"path": "vt-in", "secret": "s1", "signatureAlgo": "hmac-sha256-hex"},
	}).Field(t, "id")
	docID := wc.POST("/api/v1/documents", map[string]any{
		"name": "vt_doc", "content": "an essay about the harborlight at dusk.",
	}).Field(t, "id")

	cases := []struct {
		tool, token, types, wantID string
		prose                      bool // search_documents 自有散文渲染（引擎同源）；其余 7 个 ContentSearch slim JSON。
	}{
		{"search_function", "ledgerreconcile", "function", fnID, false},
		{"search_handler", "parcelrouting", "handler", hdID, false},
		{"search_agent", "sentimentdistill", "agent", agID, false},
		{"search_control", "thresholdgate", "control", ctlID, false},
		{"search_approval", "budgetsignoff", "approval", apfID, false},
		{"search_workflow", "nightlydigest", "workflow", wfID, false},
		{"search_triggers", "webhookpulse", "trigger", trgID, false},
		{"search_documents", "harborlight", "document", docID, true},
	}
	for _, tc := range cases {
		waitIndexed(t, wc, tc.token, tc.types)
		out := driveTool(t, wc, mock, tc.tool, map[string]any{"query": tc.token})
		if !strings.Contains(out, tc.wantID) {
			t.Errorf("%s(%q) result must hit %s via content engine (body-only token), got: %s",
				tc.tool, tc.token, tc.wantID, out)
		}
		if !tc.prose && !strings.Contains(out, `"count"`) {
			t.Errorf("%s result must keep the slim {count, list} shape, got: %s", tc.tool, out)
		}
	}
}

// TestSearchLLM_BlocksTier1WholeCatalog: 精度链第一档——目录在 4k token 预算内时**整体**
// 直喂 utility（连与查询词法无关的积木也在场，这是索引检索做不到的），sifter 编号回选
// 驱动最终结果条数。
func TestSearchLLM_BlocksTier1WholeCatalog(t *testing.T) {
	wc, mock := chatSetup(t, true)

	fnCreate(t, wc, "metricflush",
		"def metricflush() -> dict:\n    \"\"\"flush collected metrics to disk\"\"\"\n    return {}\n")
	// Lexically UNRELATED to the query — tier 1's physical proof. 与查询词法无关——第一档物证。
	wc.POST("/api/v1/controls", map[string]any{
		"name": "oddgate", "description": "completely unrelated branch chooser",
		"inputs":   []map[string]any{{"name": "x", "type": "number"}},
		"branches": []map[string]any{{"port": "out", "when": "true"}},
	}).OK(t, nil)
	waitIndexed(t, wc, "metricflush", "function")
	waitIndexed(t, wc, "oddgate", "control")

	mock.Enqueue(utilModel, harness.LLMTurn{Text: "[1]"}) // sifter 回选第 1 条。
	out := driveTool(t, wc, mock, "search_blocks", map[string]any{"query": "flush metrics"})

	// The sifter prompt carried the WHOLE catalog — including the unrelated control.
	// sifter 收到的是整目录——含词法无关的 control。
	uds := mock.DumpsFor(utilModel)
	if len(uds) == 0 {
		t.Fatal("tier 1 must consult the utility sifter")
	}
	prompt := string(uds[len(uds)-1].Raw)
	if !strings.Contains(prompt, "You select workflow building blocks") {
		t.Fatalf("utility request is not the sifter prompt: %s", prompt)
	}
	if !strings.Contains(prompt, "metricflush") || !strings.Contains(prompt, "oddgate") {
		t.Fatalf("tier 1 must feed the WHOLE catalog (got metricflush=%v oddgate=%v):\n%s",
			strings.Contains(prompt, "metricflush"), strings.Contains(prompt, "oddgate"), prompt)
	}
	// Sifter picked exactly one — the tool result mirrors that. 精选 1 条 → 结果即 1 条。
	if !strings.Contains(out, `"count":1`) && !strings.Contains(out, `"count": 1`) {
		t.Fatalf("sifter pick [1] must yield exactly one block, got: %s", out)
	}
}

// TestSearchLLM_BlocksTier2IndexNarrowed: 第二档——目录撑破 4k token 预算后，先索引收窄
// （top-50、查询相关），utility 只见相关候选：词法无关的 oddgate 必须从 sifter prompt 消失。
func TestSearchLLM_BlocksTier2IndexNarrowed(t *testing.T) {
	wc, mock := chatSetup(t, true)

	fnCreate(t, wc, "metricflush",
		"def metricflush() -> dict:\n    \"\"\"flush collected metrics to disk\"\"\"\n    return {}\n")
	wc.POST("/api/v1/controls", map[string]any{
		"name": "oddgate", "description": "completely unrelated branch chooser",
		"inputs":   []map[string]any{{"name": "x", "type": "number"}},
		"branches": []map[string]any{{"port": "out", "when": "true"}},
	}).OK(t, nil)
	// Fatten the catalog past the 4k-token sift budget with query-irrelevant blocks.
	// Each catalog row is capped at substr(body,1,240) (~70 tokens incl. title+ref),
	// so ~70 rows clear the 4000-token budget with margin.
	// 用与查询无关的积木把目录撑过 4k token 精选预算。每行目录项被截到
	// substr(body,1,240)（连 title+ref 约 70 token），70 行带余量越过 4000。
	pad := strings.Repeat("verbose capability prose for catalog budget padding. ", 12)
	for i := 0; i < 70; i++ {
		fnCreate(t, wc, fmt.Sprintf("padfn_%02d", i),
			"def f() -> dict:\n    \"\"\"catalog filler "+pad+"\"\"\"\n    return {}\n")
	}
	waitIndexed(t, wc, "metricflush", "function")
	waitIndexed(t, wc, "oddgate", "control")

	mock.Enqueue(utilModel, harness.LLMTurn{Text: "[1]"})
	out := driveTool(t, wc, mock, "search_blocks", map[string]any{"query": "flush metrics"})

	uds := mock.DumpsFor(utilModel)
	if len(uds) == 0 {
		t.Fatal("tier 2 must still consult the utility sifter")
	}
	prompt := string(uds[len(uds)-1].Raw)
	if !strings.Contains(prompt, "metricflush") {
		t.Fatalf("tier 2 sifter prompt must contain the query-relevant block:\n%s", prompt)
	}
	if strings.Contains(prompt, "oddgate") {
		t.Fatalf("tier 2 must be index-narrowed — lexically unrelated block leaked into the sifter prompt:\n%s", prompt)
	}
	if !strings.Contains(out, "metricflush") {
		t.Fatalf("tier 2 result must carry the picked block, got: %s", out)
	}
}

// TestSearchLLM_BlocksTier3AndScope: 第三档（utility 未配 → sifter 解析失败 → 纯索引排序）
// + 积木铁律：六类之外（document/skill）即使同名也永不出现；ref 全部可接线（fn_ 直填、
// handler 方法带 .method）。
func TestSearchLLM_BlocksTier3AndScope(t *testing.T) {
	wc, mock := chatSetup(t, false) // 无 utility → 精度链落到第三档。

	fnID := fnCreate(t, wc, "metricflush",
		"def metricflush() -> dict:\n    \"\"\"flush collected metrics to disk\"\"\"\n    return {}\n")
	hdID := hdCreate(t, wc, "bufhost", map[string]any{
		"description": "buffer host", "initBody": "self.n = 0",
		"methods": []map[string]any{
			{"name": "flush", "body": "return {\"ok\": True}", "description": "flush metrics buffer now"},
		},
	})
	// Same-token decoys outside the six block kinds. 六类之外的同 token 诱饵。
	wc.POST("/api/v1/documents", map[string]any{
		"name": "metricflush essay", "content": "prose about metricflush flushing metrics",
	}).OK(t, nil)
	wc.POST("/api/v1/skills", map[string]any{
		"name": "metricflush_skill", "description": "skill about flush metrics", "body": "flush metrics steps",
	}).OK(t, nil)
	waitIndexed(t, wc, "metricflush", "function")
	waitIndexed(t, wc, "bufhost", "handler")

	out := driveTool(t, wc, mock, "search_blocks", map[string]any{"query": "flush metrics"})

	if !strings.Contains(out, fnID) {
		t.Fatalf("tier 3 (no utility) must still return index-ranked blocks, got: %s", out)
	}
	if !strings.Contains(out, "hd_"+strings.TrimPrefix(hdID, "hd_")+".flush") {
		t.Errorf("handler METHOD must be its own wireable ref (hd_<id>.flush), got: %s", out)
	}
	if strings.Contains(out, `"document"`) || strings.Contains(out, `"skill"`) ||
		strings.Contains(out, "essay") || strings.Contains(out, "metricflush_skill") {
		t.Errorf("blocks scope is six kinds ONLY — document/skill leaked: %s", out)
	}
}

// TestSearchLLM_SearchConversationsTool: 回忆窗——对话 A 落库一个独特词，对话 B 经
// search_conversations 找回：结果带 conversationId + messageId + snippet，绝不携带全文。
func TestSearchLLM_SearchConversationsTool(t *testing.T) {
	wc, mock := chatSetup(t, false)

	mock.Enqueue(dlgModel, harness.LLMTurn{Text: "Noted: the launch codename is blueharbor. Plus a private detail: the dock number is seventeen."})
	convA := convCreate(t, wc, "planning session")
	midA := sendMsg(t, wc, convA, "let's set the launch codename")
	if turn := waitTurn(t, wc, convA, midA, 20000); turn.Status != "completed" {
		t.Fatalf("conv A turn must complete, got %s", turn.Status)
	}
	waitIndexed(t, wc, "blueharbor", "conversation")

	out := driveTool(t, wc, mock, "search_conversations", map[string]any{"query": "blueharbor"})
	if !strings.Contains(out, convA) {
		t.Fatalf("search_conversations must return the source conversationId, got: %s", out)
	}
	if !strings.Contains(out, "messageId") && !strings.Contains(out, "msg_") {
		t.Errorf("hits must carry the matching messageId (recall is a pointer), got: %s", out)
	}
	// Pointer, not a context dump: the snippet window centers the match — distant
	// content from the same message must not ride along.
	// 指针而非倾倒：snippet 以命中为窗口——同消息里离得远的内容不得搭车。
	if strings.Contains(out, "seventeen") {
		t.Errorf("snippet must be a window, not the full transcript: %s", out)
	}
}
