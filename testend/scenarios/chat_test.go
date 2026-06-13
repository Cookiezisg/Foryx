// chat_test.go — W4 对话域：llmmock 驱动的 chat 全链。
//
// 发送/流式/落盘、工具往返（LLM 面真驱动 + 跨域溯源涟漪）、人在环危险门（approve/deny/重复决议）、
// Cancel 与 STREAM_IN_PROGRESS、todo reminder 注入与首回合起标题、压缩水位线投影、错误路径
// （EMPTY_CONTENT / 未配模型 / 供应商 5xx / max_steps 触顶）。promptdump = 模型在线缆上真看到
// 什么，是体验断言的事实源。
package scenarios

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

const (
	dlgModel  = "gpt-4o"       // catalog-known: window 128k → compaction budget works. 目录已知：压缩预算可算。
	utilModel = "mock-utility" // separate queue — title/compaction never race dialogue turns. 独立队列。
)

// chatSetup boots a server+mock, binds a workspace, registers the mock as an openai key,
// and sets the dialogue (+optionally utility) default model.
//
// chatSetup 拉起 server+mock、绑 workspace、把 mock 注册成 openai key、设 dialogue
// （可选 utility）默认模型。
func chatSetup(t *testing.T, withUtility bool) (*harness.Client, *harness.LLMMock) {
	t.Helper()
	srv := harness.Start(t)
	mock := harness.NewLLMMock(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "chat-ws"}).OK(t, nil)
	wsID := ws.Field(t, "id")
	wc := c.WS(wsID)

	keyID := wc.POST("/api/v1/api-keys", map[string]any{
		"provider": "openai", "displayName": "llmmock", "key": "sk-mock", "baseUrl": mock.URL(),
	}).Field(t, "id")
	// Probe the key: the capability catalog is built from probe archives — without :test the
	// model window is unknown (compaction silently disabled, attachments rendered conservatively).
	// 探测 key：能力目录来自探测档案——不跑 :test 模型窗口未知（压缩静默禁用、附件保守渲染）。
	wc.POST("/api/v1/api-keys/"+keyID+":test", nil).OK(t, nil)
	wc.PUT("/api/v1/workspaces/"+wsID+"/default-models/dialogue",
		map[string]any{"apiKeyId": keyID, "modelId": dlgModel}).OK(t, nil)
	if withUtility {
		wc.PUT("/api/v1/workspaces/"+wsID+"/default-models/utility",
			map[string]any{"apiKeyId": keyID, "modelId": utilModel}).OK(t, nil)
	}
	return wc, mock
}

func convCreate(t *testing.T, wc *harness.Client, title string) string {
	t.Helper()
	return wc.POST("/api/v1/conversations", map[string]any{"title": title}).Field(t, "id")
}

func sendMsg(t *testing.T, wc *harness.Client, convID, content string) string {
	t.Helper()
	return wc.POST("/api/v1/conversations/"+convID+"/messages",
		map[string]any{"content": content}).Field(t, "messageId")
}

// chatMsg is the wire shape of one turn in GET /conversations/{id}/messages.
//
// chatMsg 是消息历史里一个回合的线缆形状。
type chatMsg struct {
	ID           string `json:"id"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	StopReason   string `json:"stopReason"`
	ErrorCode    string `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	Blocks       []struct {
		ID      string         `json:"id"`
		Type    string         `json:"type"`
		Content string         `json:"content"`
		Attrs   map[string]any `json:"attrs"`
	} `json:"blocks"`
}

func listMsgs(t *testing.T, wc *harness.Client, convID string) []chatMsg {
	t.Helper()
	var msgs []chatMsg
	wc.GET("/api/v1/conversations/"+convID+"/messages?limit=50").OK(t, &msgs)
	return msgs
}

// waitTurn polls until THE assistant turn (by the id Send returned) reaches a terminal
// status and returns it — list order independent.
//
// waitTurn 轮询到指定 id 的 assistant 回合（Send 返回的）达终态并返回——与列表排序无关。
func waitTurn(t *testing.T, wc *harness.Client, convID, msgID string, timeoutMS int) chatMsg {
	t.Helper()
	var last chatMsg
	harness.Eventually(t, timeoutMS, "assistant turn "+msgID+" reaches a terminal status", func() bool {
		for _, m := range listMsgs(t, wc, convID) {
			if m.ID == msgID {
				last = m
				return m.Status != "pending" && m.Status != "streaming"
			}
		}
		return false
	})
	return last
}

// blockOfType returns the first block of the given type (nil-safe via ok flag).
//
// blockOfType 返回首个给定类型的块。
func blockOfType(m chatMsg, typ string) (content string, ok bool) {
	for _, b := range m.Blocks {
		if b.Type == typ {
			return b.Content, true
		}
	}
	return "", false
}

// TestChat_SendStreamToolRoundtrip: A8 主链——发送→流式→reasoning/tool_call/text 块落盘、
// 工具真执行（function 执行台账带 chat 溯源）、第二请求回喂工具结果、usage 聚合、SSE 帧到达。
func TestChat_SendStreamToolRoundtrip(t *testing.T) {
	wc, mock := chatSetup(t, false)
	fnID := fnCreate(t, wc, "chat_probe",
		"def probe(mode: str) -> dict:\n    return {\"echo\": mode, \"proof\": \"fn-ran-for-chat\"}\n")

	mock.Enqueue(dlgModel,
		harness.LLMTurn{
			Reasoning: "I should run the probe function.",
			ToolCalls: []harness.MockToolCall{{Name: "run_function", Args: map[string]any{
				"functionId": fnID, "args": map[string]any{"mode": "via-chat"},
				"summary": "Run the probe", "danger": "safe", "execution_group": 1,
			}}},
			PromptTokens: 120, CompletionTokens: 15,
		},
		harness.LLMTurn{Text: "Probe finished: fn-ran-for-chat.", PromptTokens: 180, CompletionTokens: 25},
	)

	sse := wc.Subscribe(t, "messages")
	convID := convCreate(t, wc, "tool roundtrip")
	mid := sendMsg(t, wc, convID, "run my probe with mode via-chat")

	turn := waitTurn(t, wc, convID, mid, 30000)
	if turn.Status != "completed" {
		t.Fatalf("turn must complete, got %s err=%s/%s", turn.Status, turn.ErrorCode, turn.ErrorMessage)
	}
	if txt, ok := blockOfType(turn, "text"); !ok || !strings.Contains(txt, "Probe finished") {
		t.Fatalf("final text block missing/wrong: %+v", turn.Blocks)
	}
	if _, ok := blockOfType(turn, "reasoning"); !ok {
		t.Fatalf("reasoning block must persist, got %+v", turn.Blocks)
	}
	if _, ok := blockOfType(turn, "tool_call"); !ok {
		t.Fatalf("tool_call block must persist, got %+v", turn.Blocks)
	}

	// The model's-eye view: request 1 carries system prompt + user text + the tool schema;
	// request 2 feeds the tool result back.
	// 模型视角：请求 1 带 system + 用户文本 + 工具 schema；请求 2 回喂工具结果。
	dumps := mock.WaitDumps(t, dlgModel, 2, 5000)
	if dumps[0].System == "" || !dumps[0].HasMessage("user", "run my probe") {
		t.Fatalf("request 1 must carry system+user, got system=%dB msgs=%+v", len(dumps[0].System), dumps[0].Messages)
	}
	// Lazy-tool auto-discovery: run_function is NOT resident on request 1 (search_tools is),
	// yet the model calling it by name works — AutoActivator marks it discovered, and it
	// joins the wire toolset from request 2 on.
	// 懒加载自动发现：请求 1 的线缆工具集没有 run_function（有 search_tools），但模型直接点名
	// 它照样跑——AutoActivator 标记 discovered，请求 2 起它进入线缆工具集。
	has := func(tools []string, name string) bool {
		for _, n := range tools {
			if n == name {
				return true
			}
		}
		return false
	}
	if has(dumps[0].Tools, "run_function") || !has(dumps[0].Tools, "search_tools") {
		t.Fatalf("request 1 toolset must be resident-only (search_tools yes, run_function no), got %v", dumps[0].Tools)
	}
	if !has(dumps[1].Tools, "run_function") {
		t.Fatalf("auto-activated lazy tool must join request 2's toolset, got %v", dumps[1].Tools)
	}
	if !dumps[1].HasMessage("tool", "fn-ran-for-chat") {
		t.Fatalf("request 2 must feed the tool result back, got %+v", dumps[1].Messages)
	}

	// Cross-domain ripple: the function's execution ledger carries chat provenance.
	// 跨域涟漪：function 执行台账带 chat 溯源。
	var page struct {
		Executions []struct {
			TriggeredBy    string `json:"triggeredBy"`
			ConversationID string `json:"conversationId"`
		} `json:"executions"`
	}
	wc.GET("/api/v1/functions/"+fnID+"/executions").OK(t, &page)
	if len(page.Executions) != 1 || page.Executions[0].TriggeredBy != "chat" || page.Executions[0].ConversationID != convID {
		t.Fatalf("execution provenance wrong: %+v", page.Executions)
	}

	// usage = mock-controlled exact sums. usage = mock 控制的精确和。
	var usage struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
	}
	wc.GET("/api/v1/conversations/"+convID+"/usage").OK(t, &usage)
	if usage.InputTokens != 300 || usage.OutputTokens != 40 {
		t.Fatalf("usage must sum mock-reported tokens (300/40), got %+v", usage)
	}

	// The stream carried frames for this conversation. 流上有这个对话的帧。
	sse.WaitFor(t, 5000, "messages stream frames for the turn", convID)

	// system prompt preview is a living debug surface. 系统提示预览是活的调试面。
	if r := wc.GET("/api/v1/conversations/" + convID + "/system-prompt-preview"); len(r.Data) < 100 {
		t.Fatalf("system-prompt-preview suspiciously empty: %s", r.Data)
	}
}

// TestChat_HumanLoopDangerGate: A8 人在环——LLM 自报 dangerous → broker 阻塞 → interactions
// 重同步 → approve 真跑 / deny 不跑且把拒绝回喂模型 → 重复决议 404。
func TestChat_HumanLoopDangerGate(t *testing.T) {
	wc, mock := chatSetup(t, false)
	fnID := fnCreate(t, wc, "danger_probe", "def go() -> dict:\n    return {\"did\": \"it\"}\n")

	dangerCall := func(id string) harness.MockToolCall {
		return harness.MockToolCall{ID: id, Name: "run_function", Args: map[string]any{
			"functionId": fnID, "args": map[string]any{},
			"summary": "Run the dangerous probe", "danger": "dangerous", "execution_group": 1,
		}}
	}

	// ── approve path ──────────────────────────────────────────────────────────
	mock.Enqueue(dlgModel,
		harness.LLMTurn{ToolCalls: []harness.MockToolCall{dangerCall("call_ap")}},
		harness.LLMTurn{Text: "did it after approval"},
	)
	conv1 := convCreate(t, wc, "approve")
	mid1 := sendMsg(t, wc, conv1, "do the dangerous thing")

	var pending []struct {
		ToolCallID string `json:"toolCallId"`
		Kind       string `json:"kind"`
		Tool       string `json:"tool"`
	}
	harness.Eventually(t, 15000, "danger interaction pends", func() bool {
		pending = nil
		wc.GET("/api/v1/conversations/"+conv1+"/interactions").OK(t, &pending)
		return len(pending) == 1
	})
	if pending[0].Kind != "danger" || pending[0].Tool != "run_function" {
		t.Fatalf("pending interaction wrong: %+v", pending[0])
	}
	wc.POST("/api/v1/conversations/"+conv1+"/interactions/"+pending[0].ToolCallID,
		map[string]any{"action": "approve"}).OK(t, nil)
	if turn := waitTurn(t, wc, conv1, mid1, 20000); turn.Status != "completed" {
		t.Fatalf("approved turn must complete, got %s %s", turn.Status, turn.ErrorMessage)
	}
	var page struct {
		Aggregates struct {
			OKCount int `json:"okCount"`
		} `json:"aggregates"`
	}
	wc.GET("/api/v1/functions/"+fnID+"/executions").OK(t, &page)
	if page.Aggregates.OKCount != 1 {
		t.Fatalf("approve must actually run the tool, executions=%+v", page.Aggregates)
	}

	// duplicate resolve → gone. 重复决议 → 404。
	wc.Do("POST", "/api/v1/conversations/"+conv1+"/interactions/"+pending[0].ToolCallID,
		map[string]any{"action": "approve"}).Fail(t, 404, "NO_PENDING_INTERACTION")

	// ── deny path ─────────────────────────────────────────────────────────────
	mock.Enqueue(dlgModel,
		harness.LLMTurn{ToolCalls: []harness.MockToolCall{dangerCall("call_dn")}},
		harness.LLMTurn{Text: "understood, not doing it"},
	)
	conv2 := convCreate(t, wc, "deny")
	mid2 := sendMsg(t, wc, conv2, "do it again")
	harness.Eventually(t, 15000, "second danger interaction pends", func() bool {
		pending = nil
		wc.GET("/api/v1/conversations/"+conv2+"/interactions").OK(t, &pending)
		return len(pending) == 1
	})
	wc.POST("/api/v1/conversations/"+conv2+"/interactions/"+pending[0].ToolCallID,
		map[string]any{"action": "deny"}).OK(t, nil)
	if turn := waitTurn(t, wc, conv2, mid2, 20000); turn.Status != "completed" {
		t.Fatalf("denied turn must still complete, got %s", turn.Status)
	}
	wc.GET("/api/v1/functions/"+fnID+"/executions").OK(t, &page)
	if page.Aggregates.OKCount != 1 {
		t.Fatalf("deny must NOT run the tool, executions=%+v", page.Aggregates)
	}
	// The denial is fed back to the model as the tool outcome. 拒绝作为工具结果回喂模型。
	dumps := mock.DumpsFor(dlgModel)
	last := dumps[len(dumps)-1]
	denied := false
	for _, m := range last.Messages {
		if m.Role == "tool" && (strings.Contains(strings.ToLower(m.Content), "den") || strings.Contains(m.Content, "拒")) {
			denied = true
			break
		}
	}
	if !denied {
		t.Fatalf("denial feedback missing from model view: %+v", last.Messages)
	}
}

// TestChat_CancelAndStreamConflict: A8 在途控制——流式中再 Send 409；Cancel 即收尾、
// 回合落 cancelled 不留 streaming 孤儿。
func TestChat_CancelAndStreamConflict(t *testing.T) {
	wc, mock := chatSetup(t, false)
	mock.Enqueue(dlgModel, harness.LLMTurn{Text: "long answer coming......", StallMS: 8000})

	sse := wc.Subscribe(t, "messages")
	convID := convCreate(t, wc, "cancel")
	mid := sendMsg(t, wc, convID, "talk slowly")

	// In-flight proof: the live stream carries the first half of the stalled reply
	// (the DB row stays "pending" until finalize — the stream is the live truth).
	// 在飞证明：活流带出 stalled 回复的前半（DB 行到 finalize 前保持 "pending"——流才是实时事实源）。
	sse.WaitFor(t, 10000, "first text delta streams", "long answer")
	wc.Do("POST", "/api/v1/conversations/"+convID+"/messages",
		map[string]any{"content": "impatient second send"}).Fail(t, 409, "STREAM_IN_PROGRESS")

	wc.DELETE("/api/v1/conversations/" + convID + "/stream")
	turn := waitTurn(t, wc, convID, mid, 15000)
	if turn.Status != "cancelled" {
		t.Fatalf("cancelled turn must persist as cancelled, got %s", turn.Status)
	}
}

// TestChat_TodoReminderAndTitle: A8 工作清单 + 起标题——todo_write 落库可查、下一步的模型视角
// 出现 live 清单 reminder（不污染持久历史）、首回合 utility 自动起标题。
func TestChat_TodoReminderAndTitle(t *testing.T) {
	wc, mock := chatSetup(t, true)
	mock.Enqueue(utilModel, harness.LLMTurn{Text: "Mock Title"})
	mock.Enqueue(dlgModel,
		harness.LLMTurn{ToolCalls: []harness.MockToolCall{{Name: "todo_write", Args: map[string]any{
			"items": []map[string]any{
				{"content": "step one of the plan", "status": "in_progress", "activeForm": "Doing step one"},
				{"content": "step two of the plan", "status": "pending"},
			},
			"summary": "Write the plan", "danger": "safe", "execution_group": 1,
		}}}},
		harness.LLMTurn{Text: "plan written"},
	)

	convID := convCreate(t, wc, "")
	mid := sendMsg(t, wc, convID, "make a plan")
	if turn := waitTurn(t, wc, convID, mid, 30000); turn.Status != "completed" {
		t.Fatalf("turn must complete, got %s %s", turn.Status, turn.ErrorMessage)
	}

	var todos struct {
		Todos []struct {
			Content string `json:"content"`
			Status  string `json:"status"`
		} `json:"todos"`
	}
	wc.GET("/api/v1/conversations/"+convID+"/todos").OK(t, &todos)
	if len(todos.Todos) != 2 || todos.Todos[0].Content != "step one of the plan" {
		t.Fatalf("todo list wrong: %+v", todos.Todos)
	}

	// The live checklist rides the NEXT model request as an ephemeral reminder.
	// live 清单作为临时 reminder 出现在下一次模型请求里。
	dumps := mock.DumpsFor(dlgModel)
	last := dumps[len(dumps)-1]
	if !strings.Contains(string(last.Raw), "step one of the plan") {
		t.Fatal("todo reminder missing from the model view")
	}

	// First turn auto-titles via utility (best-effort, async). 首回合 utility 起标题。
	harness.Eventually(t, 10000, "conversation auto-titled", func() bool {
		var conv struct {
			Title string `json:"title"`
		}
		wc.GET("/api/v1/conversations/"+convID).OK(t, &conv)
		return strings.Contains(conv.Title, "Mock Title")
	})
}

// TestChat_CompactionWatermark: A8 压缩长程——真实 input token 越线 → utility 摘要 → 水位线
// 投影：下一请求带摘要、旧回合从模型视角消失（产品逻辑列的物理证明）。
func TestChat_CompactionWatermark(t *testing.T) {
	wc, mock := chatSetup(t, true)
	wc.PATCH("/api/v1/limits", map[string]any{"context": map[string]any{"triggerRatio": 0.1}}).OK(t, nil)

	// The conversation is user-titled → no autotitle call; the utility queue serves ONLY the
	// compaction summary.
	// 对话用户已命名 → 不触发起标题；utility 队列只出压缩摘要。
	mock.Enqueue(utilModel, harness.LLMTurn{Text: "MOCK-SUMMARY-OF-OLDER-TURNS"})
	filler := strings.Repeat("filler words about the ancient topic. ", 800) // ~30KB/turn
	mock.Enqueue(dlgModel,
		harness.LLMTurn{Text: "noted 1"},
		harness.LLMTurn{Text: "noted 2"},
		harness.LLMTurn{Text: "noted 3"},
		// Turn 4 reports real input tokens over the (lowered) trigger line.
		// 第 4 回合上报的真实 input token 越过（调低后的）触发线。
		harness.LLMTurn{Text: "noted 4", PromptTokens: 60000},
		harness.LLMTurn{Text: "answer about recall"},
	)

	convID := convCreate(t, wc, "compaction")
	mid := sendMsg(t, wc, convID, "TURN1-ANCIENT-MARKER "+filler)
	waitTurn(t, wc, convID, mid, 30000)
	for i := 2; i <= 4; i++ {
		mid = sendMsg(t, wc, convID, fmt.Sprintf("TURN%d-MARKER %s", i, filler))
		waitTurn(t, wc, convID, mid, 30000)
	}

	// Compaction runs synchronously at turn 4's tail — wait for the rolling summary + watermark
	// to actually PERSIST on the conversation (the utility request arriving at the mock is not
	// yet the write).
	// 压缩在第 4 回合尾同步跑——等滚动摘要 + 水位线真落到 conversation 行（utility 请求到达 mock
	// 还不等于写完成）。
	harness.Eventually(t, 20000, "rolling summary persists on the conversation", func() bool {
		var conv struct {
			Summary              string `json:"summary"`
			SummaryCoversUpToSeq int64  `json:"summaryCoversUpToSeq"`
		}
		wc.GET("/api/v1/conversations/"+convID).OK(t, &conv)
		return strings.Contains(conv.Summary, "MOCK-SUMMARY-OF-OLDER-TURNS") && conv.SummaryCoversUpToSeq > 0
	})

	mid = sendMsg(t, wc, convID, "what did we discuss at the very beginning?")
	waitTurn(t, wc, convID, mid, 30000)

	dumps := mock.DumpsFor(dlgModel)
	last := dumps[len(dumps)-1]
	raw := string(last.Raw)
	if !strings.Contains(raw, "MOCK-SUMMARY-OF-OLDER-TURNS") {
		t.Fatal("post-compaction request must carry the rolling summary")
	}
	if strings.Contains(raw, "TURN1-ANCIENT-MARKER") {
		t.Fatal("compacted turns must vanish from the model view (watermark projection)")
	}
	if !strings.Contains(raw, "what did we discuss") {
		t.Fatal("the current user turn must of course still be present")
	}
}

// TestChat_ErrorPaths: A8 出错列——空内容 400、未配模型的回合级错误码、供应商连环 5xx 落
// error 回合、25 步触顶诚实报 max_steps。
func TestChat_ErrorPaths(t *testing.T) {
	wc, mock := chatSetup(t, false)

	convID := convCreate(t, wc, "errors")
	wc.Do("POST", "/api/v1/conversations/"+convID+"/messages",
		map[string]any{"content": "   "}).Fail(t, 400, "EMPTY_CONTENT")
	wc.Do("POST", "/api/v1/conversations/conv_nonexistent/messages",
		map[string]any{"content": "hi"}).Fail(t, 404, "CONVERSATION_NOT_FOUND")

	// Provider hard-fails every retry → the turn lands as an error row, honestly coded.
	// 供应商每次重试都硬失败 → 回合落 error 行、错误码诚实。
	for i := 0; i < 8; i++ {
		mock.Enqueue(dlgModel, harness.LLMTurn{Status: 500})
	}
	mid := sendMsg(t, wc, convID, "trigger provider failure")
	turn := waitTurn(t, wc, convID, mid, 60000)
	if turn.Status == "completed" || turn.ErrorCode == "" {
		t.Fatalf("provider 5xx must surface an error turn with a code, got %s code=%q msg=%q",
			turn.Status, turn.ErrorCode, turn.ErrorMessage)
	}

	// max_steps: a model that never stops calling tools is cut at the ceiling and says so.
	// (Clear first: the retry chain consumed fewer than the 8 scripted failures above —
	// leftovers must not poison this sub-scenario.)
	// max_steps：永远在调工具的模型在天花板被切，并明说。（先 Clear：上面 8 个故障帧重试链
	// 没用完，残留不得毒到本子场景。）
	mock.Clear(dlgModel)
	loopCall := harness.MockToolCall{Name: "search_function", Args: map[string]any{
		"query": "anything", "summary": "Search again", "danger": "safe", "execution_group": 1,
	}}
	for i := 0; i < 30; i++ {
		mock.Enqueue(dlgModel, harness.LLMTurn{ToolCalls: []harness.MockToolCall{loopCall}})
	}
	conv2 := convCreate(t, wc, "maxsteps")
	mid2 := sendMsg(t, wc, conv2, "loop forever")
	turn = waitTurn(t, wc, conv2, mid2, 120000)
	if !strings.Contains(strings.ToLower(turn.ErrorCode+turn.StopReason), "max_steps") {
		t.Fatalf("ceiling must be honestly reported, got status=%s stop=%q code=%q",
			turn.Status, turn.StopReason, turn.ErrorCode)
	}

	// A fresh workspace with NO dialogue model: Send is accepted, the turn errors with a
	// configuration code — the product's "go configure a model" moment.
	// 全新未配模型 workspace：Send 被接受、回合以配置类错误码落地——产品的「去配模型」时刻。
	srv2 := harness.Start(t)
	c2 := srv2.Client(t)
	ws2 := c2.POST("/api/v1/workspaces", map[string]any{"name": "no-model"}).OK(t, nil)
	wc2 := c2.WS(ws2.Field(t, "id"))
	conv3 := convCreate(t, wc2, "unconfigured")
	mid3 := sendMsg(t, wc2, conv3, "hello?")
	turn = waitTurn(t, wc2, conv3, mid3, 20000)
	if turn.Status == "completed" || turn.ErrorCode == "" {
		t.Fatalf("unconfigured model must error the turn with a code, got %s code=%q", turn.Status, turn.ErrorCode)
	}
}
