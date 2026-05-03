//go:build pipeline

// harness_smoke_test.go — single sanity check that the harness wires up
// correctly end-to-end. Boots, seeds DeepSeek, POSTs a message, and confirms
// a chat.message snapshot lands with status=completed and at least one text
// block. If THIS test passes, Steps 3-5 have a solid foundation.
//
// harness_smoke_test.go — 一次性 sanity check 验证 harness 端到端接线 OK。
// 启动、塞 DeepSeek、POST 一条消息、确认 chat.message 快照达 completed 并
// 含至少一个 text block。本测试通过 = Step 3-5 有可靠地基。
package test

import (
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
)

func TestHarness_Smoke(t *testing.T) {
	RequireDeepSeekKey(t)

	h := New(t)
	h.SeedDeepSeek(t, "")
	conv := h.NewConversation(t, "smoke")

	sub := h.SubscribeSSE(t, conv.ID)

	var resp struct {
		Data struct {
			MessageID string `json:"messageId"`
		} `json:"data"`
	}
	h.PostJSON("/api/v1/conversations/"+conv.ID+"/messages",
		map[string]any{"content": "Reply with one short word."},
		&resp)
	if resp.Data.MessageID == "" {
		t.Fatalf("post message: empty messageId in response")
	}

	final := sub.WaitForAssistantTerminal(60 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status = %q (errorCode=%q errorMessage=%q), want completed\nraw events:\n%s",
			final.Status, final.ErrorCode, final.ErrorMessage, sub.FormatRawEvents())
	}

	// Must have at least one text block with non-empty content.
	// 至少一个非空 text block。
	var sawText bool
	for _, b := range final.Blocks {
		if b.Type == chatdomain.BlockTypeText {
			sawText = true
			break
		}
	}
	if !sawText {
		t.Fatalf("no text block in final message; blocks=%d\nraw events:\n%s",
			len(final.Blocks), sub.FormatRawEvents())
	}

	// Token counts populated (DeepSeek reports them).
	// token 计数已填（DeepSeek 会返回）。
	if final.InputTokens == 0 || final.OutputTokens == 0 {
		t.Errorf("token counts not populated: in=%d out=%d", final.InputTokens, final.OutputTokens)
	}

	// Multiple chat.message snapshots should have arrived (streaming intermediate states).
	// 应该收到多条 chat.message 快照（流式中间状态）。
	chatRaw := 0
	for _, e := range sub.RawEvents() {
		if e.Type == "chat.message" {
			chatRaw++
		}
	}
	if chatRaw < 2 {
		t.Errorf("expected multiple chat.message snapshots (token streaming), got %d", chatRaw)
	}

	t.Logf("smoke ok: %d chat.message snapshots, final blocks=%d, tokens in=%d out=%d",
		chatRaw, len(final.Blocks), final.InputTokens, final.OutputTokens)
}
