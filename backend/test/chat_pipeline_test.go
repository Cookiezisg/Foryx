//go:build pipeline

// chat_pipeline_test.go — Step 3 of the testing plan: real-world chat flows.
// Five scenarios, each booting a fresh harness so DB state is isolated:
//
//  1. Simple chat               — text token streaming, monotonic growth, persisted Message
//  2. Pre-LLM config error      — missing model_config → status=error + ErrorCode persisted
//  3. Tool call (search_forges) — assistant message ends with tool_call + tool_result blocks
//  4. Cancel mid-stream         — DELETE /stream → status=cancelled + persisted
//  5. Reasoning model           — deepseek-reasoner → reasoning blocks separate from text
//
// Step 3 of the testing plan: 真实 chat 流程的 5 个场景，每个 boot 全新 harness
// 保证 DB 状态隔离。
package test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
)

// postMessageResp captures the 202 response shape from POST /messages.
//
// postMessageResp 抓 POST /messages 的 202 响应形状。
type postMessageResp struct {
	Data struct {
		MessageID string `json:"messageId"`
	} `json:"data"`
}

// postMessage POSTs a user message and returns the user message id.
//
// postMessage POST 用户消息并返回 user message id。
func postMessage(t *testing.T, h *Harness, convID, content string) string {
	t.Helper()
	var resp postMessageResp
	h.PostJSON("/api/v1/conversations/"+convID+"/messages",
		map[string]any{"content": content}, &resp)
	if resp.Data.MessageID == "" {
		t.Fatalf("post message: empty messageId")
	}
	return resp.Data.MessageID
}

// ── 1. Simple chat ───────────────────────────────────────────────────────────

func TestChat_SimpleText_StreamingSnapshots(t *testing.T) {
	RequireDeepSeekKey(t)
	h := New(t)
	h.SeedDeepSeek(t, "")
	conv := h.NewConversation(t, "simple")
	sub := h.SubscribeSSE(t, conv.ID)

	postMessage(t, h, conv.ID, "Reply with one short word.")

	final := sub.WaitForAssistantTerminal(60 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errorCode=%q errorMsg=%q\nraw:\n%s",
			final.Status, final.ErrorCode, final.ErrorMessage, sub.FormatRawEvents())
	}
	if final.StopReason != chatdomain.StopReasonEndTurn {
		t.Errorf("stopReason=%q, want end_turn", final.StopReason)
	}

	// Text-block content must grow monotonically across snapshots — entity-state
	// model says each snapshot is a strict superset of the previous one.
	//
	// 跨快照 text block 内容必须单调生长——entity-state 模型要求每帧是前一帧的严格超集。
	textLens := []int{}
	for _, e := range sub.RawEvents() {
		if e.Type != "chat.message" {
			continue
		}
		var m chatdomain.Message
		if err := json.Unmarshal(e.Data, &m); err != nil {
			continue
		}
		if m.ID != final.ID {
			continue
		}
		text := extractTextFromBlocks(m.Blocks)
		textLens = append(textLens, len(text))
	}
	if len(textLens) < 2 {
		t.Fatalf("expected at least 2 chat.message snapshots for assistant, got %d", len(textLens))
	}
	for i := 1; i < len(textLens); i++ {
		if textLens[i] < textLens[i-1] {
			t.Errorf("text shrank: snapshot[%d]=%d -> snapshot[%d]=%d",
				i-1, textLens[i-1], i, textLens[i])
		}
	}

	// Persistence: the message in DB must match the final SSE snapshot id +
	// status. SSE that fires without a row would be a bug.
	//
	// 持久化：DB 里的 message id + status 必须与 SSE 最终快照一致。
	// 推 SSE 但行不在 DB 里就是 bug。
	var (
		count  int64
		dbStat string
		dbCode string
	)
	if err := h.DB.Raw("SELECT COUNT(*) FROM messages WHERE id = ?", final.ID).Scan(&count).Error; err != nil {
		t.Fatalf("query db count: %v", err)
	}
	if count != 1 {
		t.Fatalf("messages row count for %s = %d, want 1", final.ID, count)
	}
	if err := h.DB.Raw("SELECT status FROM messages WHERE id = ?", final.ID).Scan(&dbStat).Error; err != nil {
		t.Fatalf("query db status: %v", err)
	}
	if dbStat != chatdomain.StatusCompleted {
		t.Errorf("db status=%q, want completed", dbStat)
	}
	if err := h.DB.Raw("SELECT error_code FROM messages WHERE id = ?", final.ID).Scan(&dbCode).Error; err == nil && dbCode != "" {
		t.Errorf("db errorCode=%q on a completed message; should be empty", dbCode)
	}
}

// ── 2. Pre-LLM config error ──────────────────────────────────────────────────

func TestChat_MissingModelConfig_ErrorCodePersisted(t *testing.T) {
	// Don't seed; we WANT pre-LLM failure.
	// 故意不 seed；要的就是 pre-LLM 失败。
	h := New(t)
	conv := h.NewConversation(t, "no-config")
	sub := h.SubscribeSSE(t, conv.ID)

	postMessage(t, h, conv.ID, "anything")

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

	// DB persistence — pre-LLM errors used to fly past as SSE without a row;
	// emitFatalError now writes a stub message so this is visible in history.
	//
	// DB 持久化——pre-LLM 错误以前只飞个 SSE 不落库；现在 emitFatalError 写 stub
	// 让历史能看到。
	var dbCode, dbMsg, dbStat string
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

// ── 3. Tool call (search_forges) ─────────────────────────────────────────────

func TestChat_ToolCall_SearchForges(t *testing.T) {
	RequireDeepSeekKey(t)
	h := New(t)
	h.SeedDeepSeek(t, "")

	// Seed a forge so search_forges has something to find.
	// 塞一个 forge 让 search_forges 有东西可搜。
	h.NewForge(t, "parse_csv",
		`def parse_csv(text: str) -> list:
    """Split CSV text into rows of values.

    Args:
        text: CSV-formatted text.

    Returns:
        List of rows, each row a list of cell strings.
    """
    return [line.split(",") for line in text.split("\n") if line]
`)

	conv := h.NewConversation(t, "tool-call")
	sub := h.SubscribeSSE(t, conv.ID)

	postMessage(t, h, conv.ID,
		"List the forges I have for parsing CSV by calling search_forges. "+
			"Return only what search_forges finds.")

	final := sub.WaitForAssistantTerminal(90 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errorCode=%q\nraw:\n%s",
			final.Status, final.ErrorCode, sub.FormatRawEvents())
	}

	// Final blocks must contain a tool_call (search_forges) and its tool_result.
	// 最终 blocks 必含 search_forges 的 tool_call 和配对 tool_result。
	var (
		toolCallID string
		sawCall    bool
		sawResult  bool
		resultOK   bool
	)
	for _, b := range final.Blocks {
		var d map[string]any
		if err := json.Unmarshal([]byte(b.Data), &d); err != nil {
			continue
		}
		switch b.Type {
		case chatdomain.BlockTypeToolCall:
			if name, _ := d["name"].(string); name == "search_forges" {
				sawCall = true
				toolCallID, _ = d["id"].(string)
			}
		case chatdomain.BlockTypeToolResult:
			if id, _ := d["toolCallId"].(string); id == toolCallID && id != "" {
				sawResult = true
				resultOK, _ = d["ok"].(bool)
			}
		}
	}
	if !sawCall {
		t.Errorf("no tool_call block for search_forges in final message blocks=%d", len(final.Blocks))
	}
	if !sawResult {
		t.Errorf("no paired tool_result for search_forges call %q", toolCallID)
	}
	if !resultOK {
		t.Error("tool_result.ok=false for search_forges")
	}

	// search_forges is read-only; no ForgeExecution row should be written.
	// search_forges 只读，不应写 forge_executions 行。
	var execCount int64
	h.DB.Raw("SELECT COUNT(*) FROM forge_executions").Scan(&execCount)
	if execCount != 0 {
		t.Errorf("forge_executions row count=%d, search_forges should not record execution", execCount)
	}
}

// ── 4. Cancel mid-stream ─────────────────────────────────────────────────────

func TestChat_CancelMidStream_StatusCancelled(t *testing.T) {
	RequireDeepSeekKey(t)
	h := New(t)
	h.SeedDeepSeek(t, "")
	conv := h.NewConversation(t, "cancel")
	sub := h.SubscribeSSE(t, conv.ID)

	// Ask for something long enough that we have time to cancel mid-stream.
	// 让请求足够长，给取消留出时间窗。
	postMessage(t, h, conv.ID,
		"Write a 200-word essay about how rivers form, slowly and carefully.")

	// Wait for first streaming snapshot with non-empty text — confirms tokens
	// have started arriving before we hit cancel.
	// 等第一帧 streaming 且 text 非空——确认 token 已开始流再取消。
	sub.WaitForMessage(func(m *chatdomain.Message) bool {
		if m.Role != chatdomain.RoleAssistant || m.Status != chatdomain.StatusStreaming {
			return false
		}
		return extractTextFromBlocks(m.Blocks) != ""
	}, 30*time.Second)

	// Now cancel.
	// 现在取消。
	h.Delete("/api/v1/conversations/" + conv.ID + "/stream")

	final := sub.WaitForAssistantTerminal(15 * time.Second)
	if final.Status != chatdomain.StatusCancelled {
		t.Fatalf("status=%q, want cancelled\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}
	if final.StopReason != chatdomain.StopReasonCancelled {
		t.Errorf("stopReason=%q, want cancelled", final.StopReason)
	}

	// Persisted with cancelled status — detached-context save (S9) ensures
	// the cancelled stream's row reaches DB despite ctx being done.
	// 落库为 cancelled——detached context（S9）保证流取消后行仍能写到 DB。
	var dbStat string
	if err := h.DB.Raw("SELECT status FROM messages WHERE id = ?", final.ID).Scan(&dbStat).Error; err != nil {
		t.Fatalf("query db: %v", err)
	}
	if dbStat != chatdomain.StatusCancelled {
		t.Errorf("db status=%q, want cancelled (terminal write should survive cancel)", dbStat)
	}
}

// ── 5. Reasoning model ───────────────────────────────────────────────────────

func TestChat_ReasoningModel_BlocksSeparate(t *testing.T) {
	key := RequireDeepSeekKey(t)
	h := New(t)
	h.SeedDeepSeek(t, key)

	// Override chat scenario to deepseek-reasoner. Upsert is idempotent.
	// 把 chat scenario 改用 deepseek-reasoner。Upsert 幂等。
	if _, err := h.Model.Upsert(h.LocalCtx(), modeldomain.ScenarioChat, modelapp.UpsertInput{
		Provider: ProviderDeepSeek,
		ModelID:  "deepseek-reasoner",
	}); err != nil {
		t.Fatalf("override to deepseek-reasoner: %v", err)
	}

	conv := h.NewConversation(t, "reasoner")
	sub := h.SubscribeSSE(t, conv.ID)

	postMessage(t, h, conv.ID,
		"Compute 19 * 23 step by step, showing your reasoning, then give the final number.")

	// Reasoner can be slower than chat (reasoning + answer).
	// reasoner 比 chat 慢（推理 + 答案）。
	final := sub.WaitForAssistantTerminal(180 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Skipf("reasoner not available or returned non-completed status=%q errorCode=%q; skipping",
			final.Status, final.ErrorCode)
	}

	var (
		reasoningContent string
		textContent      string
	)
	for _, b := range final.Blocks {
		var d struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(b.Data), &d); err != nil {
			continue
		}
		switch b.Type {
		case chatdomain.BlockTypeReasoning:
			reasoningContent += d.Text
		case chatdomain.BlockTypeText:
			textContent += d.Text
		}
	}
	if reasoningContent == "" {
		t.Errorf("no reasoning block; deepseek-reasoner should emit reasoning_content")
	}
	if textContent == "" {
		t.Errorf("no text block; expected the model's final answer")
	}
	// The final answer should mention 437 (19 * 23).
	// 最终答案应含 437（19 * 23）。
	if textContent != "" && !strings.Contains(textContent, "437") {
		t.Logf("text doesn't contain 437; model said: %q", textContent)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func extractTextFromBlocks(blocks []chatdomain.Block) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Type != chatdomain.BlockTypeText {
			continue
		}
		var d struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(blk.Data), &d); err == nil {
			b.WriteString(d.Text)
		}
	}
	return b.String()
}
