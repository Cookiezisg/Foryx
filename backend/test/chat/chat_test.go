//go:build pipeline

// Package chat runs chat domain pipeline tests; one Live_ test needs DEEPSEEK_API_KEY.
//
// Package chat 跑 chat 域 pipeline 测试，其中一个 Live_ 测试需 DEEPSEEK_API_KEY。
package chat

import (
	"net/http"
	"testing"
	"time"

	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// isTerminal reports whether status is a non-streaming terminal state.
//
// isTerminal 判断 status 是否非 streaming 终态。
func isTerminal(status string) bool {
	return status == chatdomain.StatusCompleted ||
		status == chatdomain.StatusError ||
		status == chatdomain.StatusCancelled
}

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

func TestChat_MissingModelConfig_ErrorCodePersisted(t *testing.T) {
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

func TestChat_ToolCall_SearchFunction(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"search_function", "call_fake_search_001",
		`{"query":"csv","summary":"searching functions for csv parsing"}`,
	))
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

func TestChat_CancelMidStream_StatusCancelled(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushDefault(th.ScriptSlowText(
		"Rivers form through erosion deposition watersheds tributaries channels gradients",
		30*time.Millisecond,
	))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "cancel")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Write a long essay about how rivers form.")

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

	var dbStat string
	if err := h.DB.Raw("SELECT status FROM messages WHERE id = ?", final.ID).Scan(&dbStat).Error; err != nil {
		t.Fatalf("query db: %v", err)
	}
	if dbStat != chatdomain.StatusCancelled {
		t.Errorf("db status=%q, want cancelled (terminal write must survive cancel)", dbStat)
	}
}

func TestChat_Live_ReasoningModel_BlocksSeparate(t *testing.T) {
	key := th.RequireDeepSeekKey(t)
	h := th.New(t)
	h.SeedDeepSeek(t, key)

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
	if textContent != "" && len(textContent) > 0 {
		t.Logf("model answer: %q", textContent)
	}
}

func TestChat_MissingAPIKey_ErrorCodePersisted(t *testing.T) {
	h := th.New(t)

	// Model.Upsert now requires a matching api-key to exist (green-save /
	// red-runtime guard), so we seed then delete to simulate the
	// "config drifted out from under the chat flow" path that
	// API_KEY_PROVIDER_NOT_FOUND is meant to catch.
	// Model.Upsert 现在要求有匹配 api-key（防绿保存红运行时），所以先
	// 种再删，模拟 API_KEY_PROVIDER_NOT_FOUND 想抓的"config 已飘走"路径。
	h.SeedDeepSeek(t, "test-key-soon-to-be-deleted")
	keys, _, err := h.APIKey.List(h.LocalCtx(), apikeydomain.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("list apikeys: %v", err)
	}
	for _, k := range keys {
		if err := h.APIKey.Delete(h.LocalCtx(), k.ID); err != nil {
			t.Fatalf("delete apikey: %v", err)
		}
	}

	conv := h.NewConversation(t, "no-key")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "anything")

	final := sub.WaitForAssistantTerminal(15 * time.Second)
	if final.Status != chatdomain.StatusError {
		t.Fatalf("status=%q, want error\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}
	if final.ErrorCode != "API_KEY_PROVIDER_NOT_FOUND" {
		t.Errorf("errorCode=%q, want API_KEY_PROVIDER_NOT_FOUND", final.ErrorCode)
	}

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

func TestChat_LLMStreamError_StatusError(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
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

func TestChat_CancelDuringSecondLLMCall_StatusCancelled(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
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

func TestChatReact_MultiStep_TwoToolRounds(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
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

	if fake.CallCount() != 3 {
		t.Errorf("fake LLM call count=%d, want 3", fake.CallCount())
	}
}

func TestChatReact_ParallelToolCalls_BothExecuted(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
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

	_, foundA := th.ExtractToolResultByCallID(final.Blocks, "call_parallel_a")
	_, foundB := th.ExtractToolResultByCallID(final.Blocks, "call_parallel_b")
	if !foundA {
		t.Error("no tool_result for call_parallel_a")
	}
	if !foundB {
		t.Error("no tool_result for call_parallel_b")
	}
}

func TestChatReact_HistoryRebuild_OrderCorrect(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptText("First reply"))
	fake.PushScript(th.ScriptText("Second reply"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")
	conv := h.NewConversation(t, "history-order")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "First question")
	first := sub.WaitForAssistantTerminal(30 * time.Second)
	if first.Status != chatdomain.StatusCompleted {
		t.Fatalf("first assistant status=%q", first.Status)
	}

	th.PostMessage(t, h, conv.ID, "Second question")
	second := sub.WaitForMessage(func(m *chatdomain.Message) bool {
		return m.Role == chatdomain.RoleAssistant &&
			m.ID != first.ID &&
			isTerminal(m.Status)
	}, 30*time.Second)
	if second.Status != chatdomain.StatusCompleted {
		t.Fatalf("second assistant status=%q", second.Status)
	}

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

func TestChatAutoTitle_EmptyTitle_TitleGenerated(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptText("Rivers form through erosion and deposition."))
	fake.PushScript(th.ScriptRawJSON("River Formation"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")

	conv := h.NewConversation(t, "")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "How do rivers form?")

	sub.WaitForAssistantTerminal(30 * time.Second)

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

func TestChatAutoTitle_ExplicitTitle_NotRegenerated(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptText("That is a great question."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")

	conv := h.NewConversation(t, "My Existing Title")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Hello")
	sub.WaitForAssistantTerminal(30 * time.Second)

	time.Sleep(100 * time.Millisecond)

	if c := sub.Conversation(); c != nil {
		t.Errorf("got unexpected conversation SSE: title=%q autoTitled=%v",
			c.Title, c.AutoTitled)
	}
}

func TestChatQueue_Full_Returns409(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	// 800ms initial delay so the worker stays busy long enough to fill the 5-slot queue.
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

	// Msg 1 → worker; 2–6 fill channel (cap 5); 7 must return 409.
	th.PostMessage(t, h, conv.ID, "message 1")
	for i := 2; i <= 6; i++ {
		th.PostMessage(t, h, conv.ID, "filler message")
	}

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

func TestChatAttachment_TextFile_UploadAndSend(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushDefault(th.ScriptText("I have noted the content of your file."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-key")
	conv := h.NewConversation(t, "attachment-test")
	sub := h.SubscribeSSE(t, conv.ID)

	content := []byte("name,age\nAlice,30\nBob,25")
	attID := th.UploadFile(t, h, "data.csv", "text/plain", content)
	if attID == "" {
		t.Fatal("upload: empty attachment id")
	}

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

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != "completed" {
		t.Fatalf("status=%q, want completed\nraw:\n%s",
			final.Status, sub.FormatRawEvents())
	}

	var msgList struct {
		Data []struct {
			ID    string         `json:"id"`
			Role  string         `json:"role"`
			Attrs map[string]any `json:"attrs"`
		} `json:"data"`
	}
	h.GetJSON("/api/v1/conversations/"+conv.ID+"/messages", &msgList)

	var sawAtt bool
	for _, m := range msgList.Data {
		if m.Role != "user" || len(m.Attrs) == 0 {
			continue
		}
		if atts, ok := m.Attrs["attachments"].([]any); ok && len(atts) > 0 {
			sawAtt = true
		}
	}
	if !sawAtt {
		t.Error("user message has no attachments in Message.Attrs; attachment was not linked")
	}
}
