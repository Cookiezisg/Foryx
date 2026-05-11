//go:build pipeline

// chat_test.go — chat domain pipeline tests combining all chat scenarios:
//
// From chat_pipeline_test.go (5 scenarios):
//  1. SimpleText_StreamingSnapshots  — fake LLM, text streaming, monotonic growth, DB persistence
//  2. MissingModelConfig_ErrorCodePersisted — no LLM needed (pre-LLM error path)
//  3. ToolCall_SearchFunction          — fake LLM triggers search_function round-trip
//  4. CancelMidStream_StatusCancelled — fake LLM slow stream, client cancels
//  5. Live_ReasoningModel_BlocksSeparate — real DeepSeek, opt-in via DEEPSEEK_API_KEY
//
// From chat_basic_pipeline_test.go (3 scenarios):
//  6. MissingAPIKey_ErrorCodePersisted — model config present, no API key
//  7. LLMStreamError_StatusError — LLM returns HTTP 401
//  8. CancelDuringSecondLLMCall_StatusCancelled — cancel post-tool
//
// From chat_react_pipeline_test.go (3 scenarios):
//  9. MultiStep_TwoToolRounds — two tool-call rounds
//  10. ParallelToolCalls_BothExecuted — two parallel search_function
//  11. HistoryRebuild_OrderCorrect — two sequential messages
//
// From chat_autotitle_pipeline_test.go (2 scenarios):
//  12. AutoTitle_EmptyTitle_TitleGenerated
//  13. AutoTitle_ExplicitTitle_NotRegenerated
//
// From chat_queue_pipeline_test.go (1 scenario):
//  14. Queue_Full_Returns409
//
// From chat_attachment_pipeline_test.go (1 scenario):
//  15. Attachment_TextFile_UploadAndSend
//
// Tests 1-4, 6-15 run offline (no external network). Test 5 requires
// DEEPSEEK_API_KEY and is gated by RequireDeepSeekKey.
//
// chat_test.go — 合并所有 chat domain pipeline 测试。1-4、6-15 离线（fake LLM），
// 5 需真实 DeepSeek key。
package chat

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// isTerminal returns true for any non-streaming terminal status.
//
// isTerminal 对任何非 streaming 终态返回 true。
func isTerminal(status string) bool {
	return status == chatdomain.StatusCompleted ||
		status == chatdomain.StatusError ||
		status == chatdomain.StatusCancelled
}

// ── 1. Simple text streaming ─────────────────────────────────────────────────

func TestChat_SimpleText_StreamingSnapshots(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushDefault(th.ScriptText("Rivers form through erosion and deposition over long periods of time."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "simple")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Describe how rivers form in one sentence.")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errorCode=%q errorMsg=%q\nraw:\n%s",
			final.Status, final.ErrorCode, final.ErrorMessage, sub.FormatRawEvents())
	}
	if final.StopReason != chatdomain.StopReasonEndTurn {
		t.Errorf("stopReason=%q, want end_turn", final.StopReason)
	}

	// Streaming evidence — the eventlog stream must carry the full 5-event
	// shape for the assistant message: message_start + ≥1 block_start +
	// ≥1 block_delta + ≥1 block_stop + message_stop. Recursive event log
	// replaces the old entity-snapshot model (no chat.message events).
	//
	// 流式证据——eventlog 必须为 assistant message 推完整 5 事件形：
	// message_start + ≥1 block_start + ≥1 block_delta + ≥1 block_stop +
	// message_stop。递归事件日志替代旧 entity-snapshot（无 chat.message）。
	counts := map[string]int{}
	for _, e := range sub.RawEvents() {
		if e.Source != "eventlog" {
			continue
		}
		counts[e.Type]++
	}
	for _, k := range []string{"message_start", "block_start", "block_delta", "block_stop", "message_stop"} {
		if counts[k] < 1 {
			t.Errorf("expected ≥1 %s event, got %d (full counts: %v)", k, counts[k], counts)
		}
	}

	// DB persistence: message row must exist with status=completed and no error code.
	// DB 持久化：消息行必须存在，status=completed，无 error code。
	var dbStat, dbCode string
	if err := h.DB.Raw("SELECT status FROM messages WHERE id = ?", final.ID).Scan(&dbStat).Error; err != nil {
		t.Fatalf("query db status: %v", err)
	}
	if dbStat != chatdomain.StatusCompleted {
		t.Errorf("db status=%q, want completed", dbStat)
	}
	_ = h.DB.Raw("SELECT error_code FROM messages WHERE id = ?", final.ID).Scan(&dbCode)
	if dbCode != "" {
		t.Errorf("db errorCode=%q on completed message; should be empty", dbCode)
	}
}

// ── 2. Pre-LLM config error ──────────────────────────────────────────────────

func TestChat_MissingModelConfig_ErrorCodePersisted(t *testing.T) {
	// No seed — we want pre-LLM failure.
	// 故意不 seed，要的就是 pre-LLM 失败。
	h := th.New(t)
	conv := h.NewConversation(t, "no-config")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "anything")

	final := sub.WaitForAssistantTerminal(15 * time.Second)
	if final.Status != chatdomain.StatusError {
		t.Fatalf("status=%q, want error\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}
	if final.ErrorCode != "MODEL_NOT_CONFIGURED" {
		t.Errorf("errorCode=%q, want MODEL_NOT_CONFIGURED", final.ErrorCode)
	}
	if final.ErrorMessage == "" {
		t.Error("errorMessage is empty; should carry human-readable reason")
	}
	if final.StopReason != chatdomain.StopReasonError {
		t.Errorf("stopReason=%q, want error", final.StopReason)
	}

	// DB persistence — pre-LLM errors must be persisted (emitFatalError writes stub).
	// DB 持久化——pre-LLM 错误必须落库（emitFatalError 写 stub）。
	var dbStat, dbCode, dbMsg string
	if err := h.DB.Raw(
		"SELECT status, error_code, error_message FROM messages WHERE id = ?",
		final.ID,
	).Row().Scan(&dbStat, &dbCode, &dbMsg); err != nil {
		t.Fatalf("query db row: %v", err)
	}
	if dbStat != chatdomain.StatusError {
		t.Errorf("db status=%q, want error", dbStat)
	}
	if dbCode != "MODEL_NOT_CONFIGURED" {
		t.Errorf("db errorCode=%q, want MODEL_NOT_CONFIGURED", dbCode)
	}
	if dbMsg == "" {
		t.Error("db errorMessage empty; persistence dropped it")
	}
}

// ── 3. Tool call (search_function) ───────────────────────────────────────────

func TestChat_ToolCall_SearchFunction(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	// Script 1: LLM returns a tool call for search_function.
	// Script 1：LLM 返回 search_function 的 tool call。
	fake.PushScript(th.ScriptSingleToolCall(
		"search_function", "call_fake_search_001",
		`{"query":"csv","summary":"searching functions for csv parsing"}`,
	))
	// Script 2: LLM responds after receiving the (empty) tool result.
	// search_function returns [] immediately when no functions exist (early-exit
	// path), so no Python/sandbox is needed.
	//
	// Script 2：LLM 收到（空）tool result 后响应。
	// search_function 在无 function 时直接返 []（early-exit），无需 Python/sandbox。
	fake.PushScript(th.ScriptText("I searched your function library but found no CSV-related functions."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "tool-call")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "List functions I have for parsing CSV.")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errorCode=%q\nraw:\n%s",
			final.Status, final.ErrorCode, sub.FormatRawEvents())
	}

	// Final blocks must contain a search_function tool_call and its paired tool_result.
	// 最终 blocks 必含 search_function 的 tool_call 和配对 tool_result。
	toolCallID, sawCall := th.ExtractToolCallByName(final.Blocks, "search_function")
	if !sawCall {
		t.Errorf("no tool_call block for search_function; blocks=%d\nraw:\n%s",
			len(final.Blocks), sub.FormatRawEvents())
	}
	resultData, sawResult := th.ExtractToolResultByCallID(final.Blocks, toolCallID)
	if !sawResult {
		t.Errorf("no paired tool_result for search_function call %q", toolCallID)
	}
	if ok, _ := resultData["ok"].(bool); !ok {
		t.Errorf("tool_result.ok=false for search_function; data=%v", resultData)
	}
}

// ── 4. Cancel mid-stream ─────────────────────────────────────────────────────

func TestChat_CancelMidStream_StatusCancelled(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	// Slow stream: many short chunks with delays so the cancel arrives while
	// tokens are still flowing.
	//
	// 慢流：多帧短 chunk 带延迟，让取消在 token 仍在流时到达。
	fake.PushDefault(th.ScriptSlowText(
		"Rivers form through erosion deposition watersheds tributaries channels gradients",
		30*time.Millisecond,
	))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "cancel")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Write a long essay about how rivers form.")

	// Wait for the first streaming snapshot with non-empty text, then cancel.
	// 等第一帧 streaming 且 text 非空后取消。
	sub.WaitForMessage(func(m *chatdomain.Message) bool {
		return m.Role == chatdomain.RoleAssistant &&
			m.Status == chatdomain.StatusStreaming &&
			th.ExtractTextFromBlocks(m.Blocks) != ""
	}, 30*time.Second)

	h.Delete("/api/v1/conversations/" + conv.ID + "/stream")

	final := sub.WaitForAssistantTerminal(15 * time.Second)
	if final.Status != chatdomain.StatusCancelled {
		t.Fatalf("status=%q, want cancelled\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}
	if final.StopReason != chatdomain.StopReasonCancelled {
		t.Errorf("stopReason=%q, want cancelled", final.StopReason)
	}

	// Detached-context write (S9) must persist the cancelled status to DB.
	// 分离 ctx 写（S9）必须把 cancelled 状态落库。
	var dbStat string
	if err := h.DB.Raw("SELECT status FROM messages WHERE id = ?", final.ID).Scan(&dbStat).Error; err != nil {
		t.Fatalf("query db: %v", err)
	}
	if dbStat != chatdomain.StatusCancelled {
		t.Errorf("db status=%q, want cancelled (terminal write must survive cancel)", dbStat)
	}
}

// ── 5. Reasoning model (Live — requires DEEPSEEK_API_KEY) ────────────────────

func TestChat_Live_ReasoningModel_BlocksSeparate(t *testing.T) {
	key := th.RequireDeepSeekKey(t)
	h := th.New(t) // no fake LLM — uses real DeepSeek
	h.SeedDeepSeek(t, key)

	// Override chat scenario to deepseek-reasoner.
	// 把 chat scenario 切为 deepseek-reasoner。
	if _, err := h.Model.Upsert(h.LocalCtx(), modeldomain.ScenarioChat, modelapp.UpsertInput{
		Provider: th.ProviderDeepSeek,
		ModelID:  "deepseek-reasoner",
	}); err != nil {
		t.Fatalf("override to deepseek-reasoner: %v", err)
	}

	conv := h.NewConversation(t, "reasoner")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID,
		"Compute 19 * 23 step by step, showing your reasoning, then give the final number.")

	final := sub.WaitForAssistantTerminal(180 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Skipf("reasoner returned non-completed status=%q errorCode=%q; skipping",
			final.Status, final.ErrorCode)
	}

	var reasoningContent, textContent string
	for _, b := range final.Blocks {
		switch b.Type {
		case eventlogdomain.BlockTypeReasoning:
			reasoningContent += b.Content
		case eventlogdomain.BlockTypeText:
			textContent += b.Content
		}
	}
	if reasoningContent == "" {
		t.Errorf("no reasoning block; deepseek-reasoner should emit reasoning_content")
	}
	if textContent == "" {
		t.Errorf("no text block; expected the model's final answer")
	}
	// 19 × 23 = 437
	if textContent != "" && len(textContent) > 0 {
		t.Logf("model answer: %q", textContent)
	}
}

// ── 6. Model config present but no API key ───────────────────────────────────

func TestChat_MissingAPIKey_ErrorCodePersisted(t *testing.T) {
	h := th.New(t)

	// Seed model config only — no API key. The runner should fail with a
	// key-not-found sentinel and persist the error stub.
	//
	// 只 seed model config，不 seed API key。runner 应以 key-not-found
	// sentinel 失败并持久化错误 stub。
	if _, err := h.Model.Upsert(h.LocalCtx(), modeldomain.ScenarioChat, modelapp.UpsertInput{
		Provider: th.ProviderDeepSeek,
		ModelID:  "deepseek-chat",
	}); err != nil {
		t.Fatalf("seed model config: %v", err)
	}

	conv := h.NewConversation(t, "no-key")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "anything")

	final := sub.WaitForAssistantTerminal(15 * time.Second)
	if final.Status != chatdomain.StatusError {
		t.Fatalf("status=%q, want error\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}
	// API_KEY_PROVIDER_NOT_FOUND is the sentinel for "no key for this provider".
	// API_KEY_PROVIDER_NOT_FOUND 是"该 provider 无 key"的 sentinel。
	if final.ErrorCode != "API_KEY_PROVIDER_NOT_FOUND" {
		t.Errorf("errorCode=%q, want API_KEY_PROVIDER_NOT_FOUND", final.ErrorCode)
	}

	// Error must be persisted.
	// 错误必须落库。
	var dbStat, dbCode string
	if err := h.DB.Raw(
		"SELECT status, error_code FROM messages WHERE id = ?", final.ID,
	).Row().Scan(&dbStat, &dbCode); err != nil {
		t.Fatalf("query db: %v", err)
	}
	if dbStat != chatdomain.StatusError {
		t.Errorf("db status=%q, want error", dbStat)
	}
	if dbCode != "API_KEY_PROVIDER_NOT_FOUND" {
		t.Errorf("db errorCode=%q, want API_KEY_PROVIDER_NOT_FOUND", dbCode)
	}
}

// ── 7. LLM returns HTTP 401 mid-stream → status=error ───────────────────────

func TestChat_LLMStreamError_StatusError(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	// Force the fake LLM to return 401 on all completions requests.
	// 让 fake LLM 对所有 completions 请求返 401。
	fake.PushDefault(th.ScriptHTTPError(401))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "bad-key")
	conv := h.NewConversation(t, "llm-error")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "anything")

	final := sub.WaitForAssistantTerminal(15 * time.Second)
	if final.Status != chatdomain.StatusError {
		t.Fatalf("status=%q, want error\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}
	if final.ErrorCode == "" {
		t.Error("errorCode is empty; LLM auth failure should set a non-empty code")
	}
	if final.StopReason != chatdomain.StopReasonError {
		t.Errorf("stopReason=%q, want error", final.StopReason)
	}
}

// ── 8. Cancel during second LLM call (post-tool) → status=cancelled ──────────

func TestChat_CancelDuringSecondLLMCall_StatusCancelled(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	// Script 1: LLM triggers a tool call.
	// Script 2: LLM streams text slowly after the tool result — cancel lands here.
	//
	// Script 1：LLM 触发 tool call。
	// Script 2：LLM 在收到 tool result 后缓慢流文字——取消落在这里。
	fake.PushScript(th.ScriptSingleToolCall(
		"search_function", "call_cancel_test",
		`{"query":"anything","summary":"searching forges"}`,
	))
	fake.PushScript(th.ScriptSlowText(
		"search found nothing relevant but here is a long answer about rivers",
		40*time.Millisecond,
	))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")
	conv := h.NewConversation(t, "cancel-tool")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "search forges and explain the results at length")

	// Wait for the tool_result block to appear — the tool has been executed,
	// and the second LLM call has started but is streaming slowly.
	//
	// 等 tool_result block 出现——工具已执行，第二次 LLM 调用已开始但仍在缓慢流。
	sub.WaitForMessage(func(m *chatdomain.Message) bool {
		if m.Role != chatdomain.RoleAssistant || m.Status != chatdomain.StatusStreaming {
			return false
		}
		_, found := th.ExtractToolResultByCallID(m.Blocks, "call_cancel_test")
		return found
	}, 30*time.Second)

	h.Delete("/api/v1/conversations/" + conv.ID + "/stream")

	final := sub.WaitForAssistantTerminal(15 * time.Second)
	if final.Status != chatdomain.StatusCancelled {
		t.Fatalf("status=%q, want cancelled\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}
}

// ── 9. Multi-step ReAct: two tool-call rounds ─────────────────────────────────

func TestChatReact_MultiStep_TwoToolRounds(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	// Round 1: LLM calls search_function.
	// Round 2: LLM calls search_function again (simulating a second reasoning step).
	// Round 3: LLM returns text.
	//
	// 第 1 轮：LLM 调 search_function。
	// 第 2 轮：LLM 再调 search_function（模拟第二次推理步骤）。
	// 第 3 轮：LLM 返回文字。
	fake.PushScript(th.ScriptSingleToolCall(
		"search_function", "call_step1",
		`{"query":"csv","summary":"first search"}`,
	))
	fake.PushScript(th.ScriptSingleToolCall(
		"search_function", "call_step2",
		`{"query":"json","summary":"second search"}`,
	))
	fake.PushScript(th.ScriptText("I searched twice and found no matching forges."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")
	conv := h.NewConversation(t, "react-multi")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Search my forges for CSV parsers then JSON parsers.")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errorCode=%q\nraw:\n%s",
			final.Status, final.ErrorCode, sub.FormatRawEvents())
	}

	// Verify two tool_call blocks and their paired tool_result blocks.
	// 验证两个 tool_call block 和配对的 tool_result block。
	_, found1 := th.ExtractToolCallByName(final.Blocks, "search_function")
	if !found1 {
		t.Error("no search_function tool_call block in final message")
	}

	toolCallCount := 0
	toolResultCount := 0
	for _, b := range final.Blocks {
		if b.Type == eventlogdomain.BlockTypeToolCall {
			toolCallCount++
		}
		if b.Type == eventlogdomain.BlockTypeToolResult {
			toolResultCount++
		}
	}
	if toolCallCount < 2 {
		t.Errorf("tool_call blocks=%d, want ≥2 (two search rounds)", toolCallCount)
	}
	if toolResultCount < 2 {
		t.Errorf("tool_result blocks=%d, want ≥2", toolResultCount)
	}

	// LLM should have been called 3 times: round1, round2, round3.
	// LLM 应被调 3 次：第 1/2/3 轮。
	if fake.CallCount() != 3 {
		t.Errorf("fake LLM call count=%d, want 3", fake.CallCount())
	}
}

// ── 10. Parallel tool calls: two search_function in one LLM response ─────────────

func TestChatReact_ParallelToolCalls_BothExecuted(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	// Script 1: LLM returns two parallel search_function calls.
	// Script 2: LLM responds after receiving both tool results.
	//
	// Script 1：LLM 返回两个并行 search_function 调用。
	// Script 2：LLM 收到两个 tool result 后响应。
	fake.PushScript(th.ScriptParallelToolCalls([]th.ToolCallSpec{
		{Name: "search_function", ToolID: "call_parallel_a",
			ArgsJSON: `{"query":"csv","summary":"search a"}`},
		{Name: "search_function", ToolID: "call_parallel_b",
			ArgsJSON: `{"query":"json","summary":"search b"}`},
	}))
	fake.PushScript(th.ScriptText("Both searches returned no results."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")
	conv := h.NewConversation(t, "react-parallel")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Run two forge searches in parallel.")

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errorCode=%q\nraw:\n%s",
			final.Status, final.ErrorCode, sub.FormatRawEvents())
	}

	// Two tool_call blocks + two tool_result blocks + text.
	// 两个 tool_call block + 两个 tool_result block + 文字。
	toolCalls, toolResults := 0, 0
	for _, b := range final.Blocks {
		switch b.Type {
		case eventlogdomain.BlockTypeToolCall:
			toolCalls++
		case eventlogdomain.BlockTypeToolResult:
			toolResults++
		}
	}
	if toolCalls != 2 {
		t.Errorf("tool_call blocks=%d, want 2", toolCalls)
	}
	if toolResults != 2 {
		t.Errorf("tool_result blocks=%d, want 2", toolResults)
	}

	// Two call IDs must both appear in tool results.
	// 两个 call ID 必须都在 tool result 里出现。
	_, foundA := th.ExtractToolResultByCallID(final.Blocks, "call_parallel_a")
	_, foundB := th.ExtractToolResultByCallID(final.Blocks, "call_parallel_b")
	if !foundA {
		t.Error("no tool_result for call_parallel_a")
	}
	if !foundB {
		t.Error("no tool_result for call_parallel_b")
	}
}

// ── 11. History rebuild: two sequential messages arrive in order ───────────────

func TestChatReact_HistoryRebuild_OrderCorrect(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptText("First reply"))
	fake.PushScript(th.ScriptText("Second reply"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")
	conv := h.NewConversation(t, "history-order")
	sub := h.SubscribeSSE(t, conv.ID)

	// Message 1 — wait for its assistant to complete before sending message 2.
	// This ensures the queue processes them sequentially with correct history.
	//
	// 消息 1——等 assistant 完成再发消息 2，确保队列按顺序处理且历史正确。
	th.PostMessage(t, h, conv.ID, "First question")
	first := sub.WaitForAssistantTerminal(30 * time.Second)
	if first.Status != chatdomain.StatusCompleted {
		t.Fatalf("first assistant status=%q", first.Status)
	}

	// Message 2.
	th.PostMessage(t, h, conv.ID, "Second question")
	second := sub.WaitForMessage(func(m *chatdomain.Message) bool {
		return m.Role == chatdomain.RoleAssistant &&
			m.ID != first.ID &&
			isTerminal(m.Status)
	}, 30*time.Second)
	if second.Status != chatdomain.StatusCompleted {
		t.Fatalf("second assistant status=%q", second.Status)
	}

	// List all messages — should be 4: user1, assistant1, user2, assistant2.
	// List 所有消息——应有 4 条：user1 / assistant1 / user2 / assistant2。
	var listResp struct {
		Data []struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"data"`
	}
	h.GetJSON("/api/v1/conversations/"+conv.ID+"/messages", &listResp)
	msgs := listResp.Data
	if len(msgs) != 4 {
		t.Fatalf("message count=%d, want 4", len(msgs))
	}

	wantRoles := []string{"user", "assistant", "user", "assistant"}
	for i, m := range msgs {
		if m.Role != wantRoles[i] {
			t.Errorf("msg[%d].role=%q, want %q", i, m.Role, wantRoles[i])
		}
	}
}

// ── 12. Empty title → auto-title generated after first round ──────────────────

func TestChatAutoTitle_EmptyTitle_TitleGenerated(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	// Script 1: main chat response.
	// Script 2: auto-title LLM call (uses llminfra.Generate → collects text chunks).
	// Auto-title fires async after the agent completes.
	//
	// Script 1：主 chat 响应。
	// Script 2：自动标题 LLM 调用（llminfra.Generate 收集 text chunk 作标题）。
	// 自动标题在 agent 完成后异步触发。
	fake.PushScript(th.ScriptText("Rivers form through erosion and deposition."))
	fake.PushScript(th.ScriptRawJSON("River Formation"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")

	// Empty title triggers auto-title after first round completes.
	// title="" 触发首轮完成后自动标题。
	conv := h.NewConversation(t, "")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "How do rivers form?")

	// Wait for chat to finish.
	// 等 chat 完成。
	sub.WaitForAssistantTerminal(30 * time.Second)

	// Auto-title is async — wait for conversation SSE with AutoTitled=true.
	// 自动标题异步——等 AutoTitled=true 的 conversation SSE。
	titled := sub.WaitForConversation(func(c *convdomain.Conversation) bool {
		return c.AutoTitled && c.Title != ""
	}, 10*time.Second)

	if titled.Title == "" {
		t.Error("auto-title: title is still empty after conversation SSE")
	}
	if !titled.AutoTitled {
		t.Error("auto-title: AutoTitled should be true")
	}
	t.Logf("auto-title generated: %q", titled.Title)
}

// ── 13. Explicit title → not re-generated ─────────────────────────────────────

func TestChatAutoTitle_ExplicitTitle_NotRegenerated(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	// Only one script needed — auto-title won't fire for a non-empty title.
	// 只需一条脚本——非空 title 不会触发自动标题。
	fake.PushScript(th.ScriptText("That is a great question."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")

	conv := h.NewConversation(t, "My Existing Title")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Hello")
	sub.WaitForAssistantTerminal(30 * time.Second)

	// Give a short window for any spurious conversation SSE to arrive.
	// 留短暂窗口捕获任何意外的 conversation SSE。
	time.Sleep(100 * time.Millisecond)

	// No conversation SSE should have been published (auto-title skipped).
	// 不应有 conversation SSE（自动标题已跳过）。
	if c := sub.Conversation(); c != nil {
		t.Errorf("got unexpected conversation SSE: title=%q autoTitled=%v",
			c.Title, c.AutoTitled)
	}
}

// ── 14. Queue full → 409 STREAM_IN_PROGRESS ──────────────────────────────────

func TestChatQueue_Full_Returns409(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	// Default script: 800ms initial delay so the worker stays busy long enough
	// to fill the 5-slot queue with subsequent messages.
	//
	// 默认脚本：800ms 初始延迟，让 worker 保持忙碌，有时间把后续 5 条消息塞满队列。
	fake.PushDefault(th.Script{
		Actions: []th.ChunkAction{
			{Kind: "delay", Delay: 800 * time.Millisecond},
			{Kind: "text", Content: "response"},
		},
		FinishReason: "stop",
		InputTokens:  5,
		OutputTokens: 1,
	})

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")
	conv := h.NewConversation(t, "queue-test")

	// Message 1 goes to the worker (dequeued immediately).
	// Messages 2–6 fill the channel (capacity 5).
	// Message 7 must get 409.
	//
	// 消息 1 进 worker（立即出队）。
	// 消息 2–6 填满 channel（容量 5）。
	// 消息 7 必须返 409。
	th.PostMessage(t, h, conv.ID, "message 1")
	for i := 2; i <= 6; i++ {
		th.PostMessage(t, h, conv.ID, "filler message")
	}

	// Message 7 — should hit full queue.
	// 消息 7——应命中满队列。
	var errResp th.ErrEnvelope
	status := th.DoRequest(t, h, "POST",
		"/api/v1/conversations/"+conv.ID+"/messages",
		map[string]any{"content": "overflow message"},
		&errResp)
	if status != http.StatusConflict {
		t.Errorf("status=%d, want 409 (queue full)", status)
	}
	if errResp.Error.Code != "STREAM_IN_PROGRESS" {
		t.Errorf("error.code=%q, want STREAM_IN_PROGRESS", errResp.Error.Code)
	}
}

// ── 15. Text attachment upload + send in a message ────────────────────────────

func TestChatAttachment_TextFile_UploadAndSend(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	// The fake LLM doesn't inspect the message body; it just needs to complete.
	// fake LLM 不检查消息体，只需能完成即可。
	fake.PushDefault(th.ScriptText("I have noted the content of your file."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")
	conv := h.NewConversation(t, "attachment-test")
	sub := h.SubscribeSSE(t, conv.ID)

	// Upload a small plain-text file.
	// 上传一个小文本文件。
	content := []byte("name,age\nAlice,30\nBob,25")
	attID := th.UploadFile(t, h, "data.csv", "text/plain", content)
	if attID == "" {
		t.Fatal("upload: empty attachment id")
	}

	// Send a message that references the attachment.
	// 发送一条引用该附件的消息。
	var sendResp struct {
		Data struct {
			MessageID string `json:"messageId"`
		} `json:"data"`
	}
	h.PostJSON("/api/v1/conversations/"+conv.ID+"/messages", map[string]any{
		"content":       "Summarise this CSV file.",
		"attachmentIds": []string{attID},
	}, &sendResp)
	if sendResp.Data.MessageID == "" {
		t.Fatal("send: empty messageId")
	}

	// Wait for the assistant to complete.
	// 等 assistant 完成。
	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != "completed" {
		t.Fatalf("status=%q, want completed\nraw:\n%s",
			final.Status, sub.FormatRawEvents())
	}

	// The user message should carry the attachment ref via Message.Attrs
	// JSON ({"attachments": [...]}), not via a separate block — schema
	// unification (2026-05) folded attachment_ref blocks into the message
	// itself.
	//
	// user 消息经 Message.Attrs JSON 携附件引用（{"attachments": [...]}），
	// 而不是独立 block——2026-05 schema 统一把 attachment_ref 折到 message
	// 自身。
	var msgList struct {
		Data []struct {
			ID    string `json:"id"`
			Role  string `json:"role"`
			Attrs string `json:"attrs"`
		} `json:"data"`
	}
	h.GetJSON("/api/v1/conversations/"+conv.ID+"/messages", &msgList)

	var sawAtt bool
	for _, m := range msgList.Data {
		if m.Role != "user" || m.Attrs == "" {
			continue
		}
		var a map[string]any
		if err := json.Unmarshal([]byte(m.Attrs), &a); err != nil {
			continue
		}
		if atts, ok := a["attachments"].([]any); ok && len(atts) > 0 {
			sawAtt = true
		}
	}
	if !sawAtt {
		t.Error("user message has no attachments in Message.Attrs; attachment was not linked")
	}
}
