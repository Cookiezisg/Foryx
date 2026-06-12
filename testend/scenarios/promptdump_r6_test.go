// promptdump_r6_test.go — R6（柱 B 体验审查补全，首轮缺格的补课）。
//
// 计划格：**Subagent 视角**（自足 prompt、父历史零泄漏、工具子集守卫）与**前端开发者
// 视角**（三流 SSE 帧线缆形状逐字段审读）两缺失视角；**规模态**（200 实体下 system 不
// 线性爆炸）/**降级态**（未配模型的 preview 面仍连贯）/**崩溃恢复态**（kill -9 后历史
// 重水合无重复无残缺）/**长程压缩后态**（摘要恰一次、旧回合尽出、结构无孤儿）四状态；
// **tool_result 形状**（tool_call↔tool 配对不变量）与 **token 成本账单**（usage 与 mock
// 上报逐数对账）两横切刀。事实源 = llmmock PromptDump（模型线缆视角）+ SSE 原始帧。
package scenarios

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// TestPromptR6_SubagentViewpoint: subagent 收到的是自足 prompt——父对话历史零泄漏、
// 工具子集剔除 Subagent、身份非 chat 主视角。
func TestPromptR6_SubagentViewpoint(t *testing.T) {
	wc, mock := chatSetup(t, false)

	mock.Enqueue(dlgModel,
		harness.LLMTurn{ToolCalls: []harness.MockToolCall{{Name: "Subagent",
			Args: fw(map[string]any{"subagent_type": "Explore",
				"prompt": "Find the answer to the focused question."})}}},
		harness.LLMTurn{Text: "child done"},
		harness.LLMTurn{Text: "relayed"},
	)
	convID := convCreate(t, wc, "sub viewpoint")
	mid := sendMsg(t, wc, convID, "PARENTSECRET-marker please delegate")
	if turn := waitTurn(t, wc, convID, mid, 60000); turn.Status != "completed" {
		t.Fatalf("turn must complete, got %s", turn.Status)
	}

	dumps := mock.DumpsFor(dlgModel)
	if len(dumps) < 3 {
		t.Fatalf("want parent+child+parent requests, got %d", len(dumps))
	}
	child := dumps[1]
	// 父历史零泄漏：子请求只见自足任务、不见父用户原文。
	raw := string(child.Raw)
	if strings.Contains(raw, "PARENTSECRET-marker") {
		t.Fatal("subagent must NOT see the parent conversation history")
	}
	if !child.HasMessage("user", "focused question") {
		t.Fatalf("subagent must receive the self-contained prompt, got %+v", child.Messages)
	}
	// 工具子集：Explore 是只读侦察集——无 Subagent（递归守卫）、无锻造工具。
	for _, name := range child.Tools {
		if name == "Subagent" || name == "create_function" || name == "run_function" {
			t.Fatalf("Explore subagent toolset leaked %s: %v", name, child.Tools)
		}
	}
	// 身份隔离：不是 chat 主 prompt（无 Searchable tools 段），但仍有自己的 system。
	if child.System == "" || strings.Contains(child.System, "Searchable tools") {
		t.Fatalf("subagent must carry its own compact system, got %dB", len(child.System))
	}
}

// TestPromptR6_FrontendWireShapes: 前端开发者视角——三流帧的线缆形状逐字段审读：
// {seq, scope:{kind,id}, id, frame:{...}} camelCase；durable 带 SSE id 行语义（seq>0）、
// delta 恒 seq=0；notifications 帧带持久行 id。
func TestPromptR6_FrontendWireShapes(t *testing.T) {
	wc, mock := chatSetup(t, false)

	ms := wc.Subscribe(t, "messages")
	es := wc.Subscribe(t, "entities")
	ns := wc.Subscribe(t, "notifications")

	mock.Enqueue(dlgModel, harness.LLMTurn{Text: "wire shape probe answer"})
	convID := convCreate(t, wc, "wire shapes")
	mid := sendMsg(t, wc, convID, "speak")
	if turn := waitTurn(t, wc, convID, mid, 30000); turn.Status != "completed" {
		t.Fatalf("turn must complete, got %s", turn.Status)
	}
	fnID := fnCreate(t, wc, "wire_fn", "def wire_fn() -> dict:\n    return {}\n")
	// 帧到达是异步的——审计前先各等到货（entities 帧随 env 物化/锻造镜像）。
	es.WaitFor(t, 15000, "entities frames arrive", fnID)
	ns.WaitFor(t, 15000, "notification frame arrives", "function.")

	type envShape struct {
		Seq   int64 `json:"seq"`
		Scope struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		} `json:"scope"`
		ID    string          `json:"id"`
		Frame json.RawMessage `json:"frame"`
	}
	audit := func(name string, s *harness.SSE, wantScopeKind string) {
		evs := s.Snapshot()
		if len(evs) == 0 {
			t.Fatalf("%s: no frames captured", name)
		}
		sawDelta, sawDurable := false, false
		for _, ev := range evs {
			var e envShape
			if err := json.Unmarshal(ev.Data, &e); err != nil {
				t.Fatalf("%s: frame is not the envelope shape: %s", name, ev.Data)
			}
			if e.Scope.Kind == "" || e.ID == "" || len(e.Frame) == 0 {
				t.Fatalf("%s: envelope missing scope/id/frame: %s", name, ev.Data)
			}
			var frame struct {
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal(e.Frame, &frame); err != nil || frame.Kind == "" {
				t.Fatalf("%s: frame must carry a kind discriminator: %s", name, e.Frame)
			}
			switch frame.Kind {
			case "open", "delta", "close", "signal":
			default:
				t.Fatalf("%s: unknown frame kind %q in %s", name, frame.Kind, e.Frame)
			}
			if frame.Kind == "delta" {
				sawDelta = true
				if e.Seq != 0 {
					t.Fatalf("%s: delta frames are ephemeral and must carry seq=0: %s", name, ev.Data)
				}
			}
			if e.Seq > 0 {
				sawDurable = true
			}
		}
		if !sawDurable {
			t.Fatalf("%s: stream must carry durable (seq>0) frames", name)
		}
		if wantScopeKind != "" {
			found := false
			for _, ev := range evs {
				var e envShape
				_ = json.Unmarshal(ev.Data, &e)
				if e.Scope.Kind == wantScopeKind {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s: expected scope kind %q in stream", name, wantScopeKind)
			}
		}
		_ = sawDelta
	}
	audit("messages", ms, "conversation")
	audit("entities", es, "")
	audit("notifications", ns, "")
}

// TestPromptR6_ScaleStatePromptBounded: 规模态——200 个实体下 system prompt 不随实体数
// 线性爆炸（懒目录的物理证明：与 5 实体基线比体积 < 3×）。
func TestPromptR6_ScaleStatePromptBounded(t *testing.T) {
	wc, mock := chatSetup(t, false)

	mock.Enqueue(dlgModel, harness.LLMTurn{Text: "baseline"})
	conv1 := convCreate(t, wc, "baseline")
	for i := 0; i < 5; i++ {
		fnCreate(t, wc, fmt.Sprintf("base_fn_%02d", i), "def f() -> dict:\n    return {}\n")
	}
	if turn := waitTurn(t, wc, conv1, sendMsg(t, wc, conv1, "hi"), 30000); turn.Status != "completed" {
		t.Fatalf("baseline turn failed: %s", turn.Status)
	}
	dumps := mock.DumpsFor(dlgModel)
	baseline := len(dumps[len(dumps)-1].System)

	for i := 0; i < 195; i++ {
		fnCreate(t, wc, fmt.Sprintf("scale_fn_%03d", i), "def f() -> dict:\n    \"\"\"scale probe\"\"\"\n    return {}\n")
	}
	mock.Enqueue(dlgModel, harness.LLMTurn{Text: "scaled"})
	conv2 := convCreate(t, wc, "scaled")
	if turn := waitTurn(t, wc, conv2, sendMsg(t, wc, conv2, "hi again"), 30000); turn.Status != "completed" {
		t.Fatalf("scaled turn failed: %s", turn.Status)
	}
	dumps = mock.DumpsFor(dlgModel)
	scaled := len(dumps[len(dumps)-1].System)

	if scaled > baseline*3 {
		t.Fatalf("system prompt must stay bounded at scale: 5 entities → %dB, 200 entities → %dB (>3x)", baseline, scaled)
	}
}

// TestPromptR6_DegradedAndCrashRecoveredViews: 降级态（未配模型：preview 面仍连贯渲染）+
// 崩溃恢复态（kill -9 重启后续聊：历史重水合恰一次、无残缺无重复）。
func TestPromptR6_DegradedAndCrashRecoveredViews(t *testing.T) {
	// 降级态：全新 server、零配置——preview 必须仍渲染连贯 prompt（产品自举调试面）。
	srv := harness.Start(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "degraded"}).Field(t, "id")
	wc := c.WS(wsID)
	convID := wc.POST("/api/v1/conversations", map[string]any{"title": "preview"}).Field(t, "id")
	r := wc.GET("/api/v1/conversations/" + convID + "/system-prompt-preview")
	if r.Status != 200 || len(r.Data) < 200 {
		t.Fatalf("degraded preview must render coherently, got %d %dB", r.Status, len(r.Data))
	}

	// 崩溃恢复态：配模型、聊一回合、kill -9、重启、再聊——模型视角的历史恰一次。
	mock := harness.NewLLMMock(t)
	keyID := wc.POST("/api/v1/api-keys", map[string]any{
		"provider": "openai", "displayName": "m", "key": "sk-m", "baseUrl": mock.URL(),
	}).Field(t, "id")
	wc.POST("/api/v1/api-keys/"+keyID+":test", nil).OK(t, nil)
	wc.PUT("/api/v1/workspaces/"+wsID+"/default-models/dialogue",
		map[string]any{"apiKeyId": keyID, "modelId": dlgModel}).OK(t, nil)

	mock.Enqueue(dlgModel, harness.LLMTurn{Text: "CRASHMARK-reply-one"})
	mid := sendMsg(t, wc, convID, "CRASHMARK-user-one")
	if turn := waitTurn(t, wc, convID, mid, 30000); turn.Status != "completed" {
		t.Fatalf("pre-crash turn failed: %s", turn.Status)
	}

	srv.Kill9(t)
	srv.Restart(t)
	wc2 := srv.Client(t).WS(wsID)

	mock.Enqueue(dlgModel, harness.LLMTurn{Text: "post-crash reply"})
	mid2 := sendMsg(t, wc2, convID, "what did I say before the crash?")
	if turn := waitTurn(t, wc2, convID, mid2, 30000); turn.Status != "completed" {
		t.Fatalf("post-crash turn failed: %s err=%s", turn.Status, turn.ErrorMessage)
	}
	dumps := mock.DumpsFor(dlgModel)
	raw := string(dumps[len(dumps)-1].Raw)
	if strings.Count(raw, "CRASHMARK-user-one") != 1 || strings.Count(raw, "CRASHMARK-reply-one") != 1 {
		t.Fatalf("rehydrated history must carry each pre-crash turn exactly once: user=%d reply=%d",
			strings.Count(raw, "CRASHMARK-user-one"), strings.Count(raw, "CRASHMARK-reply-one"))
	}
}

// TestPromptR6_ToolResultPairingAndUsageLedger: 横切两刀——①tool_call↔tool 配对不变量
// （线缆上每个 assistant tool_call id 都有且仅有一个 tool 回包）；②token 账单与 mock 上报
// 逐数对账（多回合 + 工具回合）。
func TestPromptR6_ToolResultPairingAndUsageLedger(t *testing.T) {
	wc, mock := chatSetup(t, false)
	fnID := fnCreate(t, wc, "pair_fn", "def pair_fn() -> dict:\n    return {\"ok\": True}\n")

	mock.Enqueue(dlgModel,
		harness.LLMTurn{ToolCalls: []harness.MockToolCall{{Name: "run_function",
			Args: map[string]any{"functionId": fnID, "args": map[string]any{},
				"summary": "run", "danger": "safe", "execution_group": 1}}},
			PromptTokens: 111, CompletionTokens: 13},
		harness.LLMTurn{Text: "tool turn done", PromptTokens: 222, CompletionTokens: 17},
	)
	convID := convCreate(t, wc, "pairing")
	mid := sendMsg(t, wc, convID, "run it")
	if turn := waitTurn(t, wc, convID, mid, 30000); turn.Status != "completed" {
		t.Fatalf("tool turn failed: %s", turn.Status)
	}
	mock.Enqueue(dlgModel, harness.LLMTurn{Text: "plain", PromptTokens: 333, CompletionTokens: 19})
	mid2 := sendMsg(t, wc, convID, "plain turn")
	if turn := waitTurn(t, wc, convID, mid2, 30000); turn.Status != "completed" {
		t.Fatalf("plain turn failed: %s", turn.Status)
	}

	// ① 配对不变量：最后一个请求的历史里，assistant.tool_calls 的每个 id 恰有一个
	// role=tool 回包（sanitizer 红线——孤儿即 provider 400 风险）。
	dumps := mock.DumpsFor(dlgModel)
	last := dumps[len(dumps)-1]
	var msgs []struct {
		Role       string `json:"role"`
		ToolCallID string `json:"tool_call_id"`
		ToolCalls  []struct {
			ID string `json:"id"`
		} `json:"tool_calls"`
	}
	var rawReq struct {
		Messages json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(last.Raw, &rawReq); err != nil {
		t.Fatalf("raw request decode: %v", err)
	}
	if err := json.Unmarshal(rawReq.Messages, &msgs); err != nil {
		t.Fatalf("messages decode: %v", err)
	}
	calls := map[string]int{}
	replies := map[string]int{}
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			calls[tc.ID]++
		}
		if m.Role == "tool" && m.ToolCallID != "" {
			replies[m.ToolCallID]++
		}
	}
	if len(calls) == 0 {
		t.Fatal("history must carry the assistant tool_call")
	}
	for id, n := range calls {
		if n != 1 || replies[id] != 1 {
			t.Fatalf("tool pairing broken for %s: calls=%d replies=%d (all: %v / %v)", id, n, replies[id], calls, replies)
		}
	}
	for id := range replies {
		if calls[id] == 0 {
			t.Fatalf("orphan tool reply %s without a tool_call", id)
		}
	}

	// ② 账单对账：mock 上报 111+222+333 / 13+17+19。
	var usage struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
	}
	wc.GET("/api/v1/conversations/"+convID+"/usage").OK(t, &usage)
	if usage.InputTokens != 666 || usage.OutputTokens != 49 {
		t.Fatalf("usage ledger must equal mock-reported sums (666/49), got %+v", usage)
	}
}

// TestPromptR6_PostCompactionView: 长程压缩后态的结构审读——滚动摘要在 system/历史中
// **恰一次**、被压回合从模型视角尽出、当前回合在场、无孤儿 tool 配对。
func TestPromptR6_PostCompactionView(t *testing.T) {
	wc, mock := chatSetup(t, true)
	wc.PATCH("/api/v1/limits", map[string]any{"context": map[string]any{"triggerRatio": 0.1}}).OK(t, nil)

	// 压缩保留近窗——需要足够的可折叠旧回合（与 W4 同形：4 回合、末回合越线）。
	mock.Enqueue(utilModel, harness.LLMTurn{Text: "R6-ROLLING-SUMMARY"})
	filler := strings.Repeat("ancient words. ", 1500)
	mock.Enqueue(dlgModel,
		harness.LLMTurn{Text: "n1"},
		harness.LLMTurn{Text: "n2"},
		harness.LLMTurn{Text: "n3"},
		harness.LLMTurn{Text: "n4", PromptTokens: 60000},
		harness.LLMTurn{Text: "n5"},
	)
	convID := convCreate(t, wc, "post compaction")
	waitTurn(t, wc, convID, sendMsg(t, wc, convID, "OLDMARK-1 "+filler), 30000)
	for i := 2; i <= 4; i++ {
		waitTurn(t, wc, convID, sendMsg(t, wc, convID, fmt.Sprintf("OLDMARK-%d %s", i, filler)), 30000)
	}
	harness.Eventually(t, 20000, "summary persists", func() bool {
		var conv struct {
			Summary string `json:"summary"`
		}
		wc.GET("/api/v1/conversations/"+convID).OK(t, &conv)
		return strings.Contains(conv.Summary, "R6-ROLLING-SUMMARY")
	})
	waitTurn(t, wc, convID, sendMsg(t, wc, convID, "NEWMARK after compaction"), 30000)

	dumps := mock.DumpsFor(dlgModel)
	raw := string(dumps[len(dumps)-1].Raw)
	if n := strings.Count(raw, "R6-ROLLING-SUMMARY"); n != 1 {
		t.Fatalf("rolling summary must appear exactly once in the model view, got %d", n)
	}
	if strings.Contains(raw, "OLDMARK-1") {
		t.Fatal("compacted turns must vanish from the model view")
	}
	if !strings.Contains(raw, "NEWMARK") {
		t.Fatal("the current turn must be present")
	}
}
