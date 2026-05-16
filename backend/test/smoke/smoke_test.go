//go:build pipeline

// Package smoke is the foundational harness sanity check; failure invalidates the rest of the suite.
//
// Package smoke 是 harness 基础 sanity check；失败则其他 pipeline 套件无效。
package smoke

import (
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

func TestHarness_Smoke(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushDefault(th.ScriptText("OK"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
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

	final := sub.WaitForAssistantTerminal(30 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status = %q (errorCode=%q errorMessage=%q), want completed\nraw events:\n%s",
			final.Status, final.ErrorCode, final.ErrorMessage, sub.FormatRawEvents())
	}

	var sawText bool
	for _, b := range final.Blocks {
		if b.Type == eventlogdomain.BlockTypeText {
			sawText = true
			break
		}
	}
	if !sawText {
		t.Fatalf("no text block in final message; blocks=%d\nraw events:\n%s",
			len(final.Blocks), sub.FormatRawEvents())
	}

	if final.InputTokens == 0 || final.OutputTokens == 0 {
		t.Errorf("token counts not populated: in=%d out=%d", final.InputTokens, final.OutputTokens)
	}

	eventLogCount := 0
	for _, e := range sub.RawEvents() {
		if e.Source == "eventlog" {
			eventLogCount++
		}
	}
	if eventLogCount < 5 {
		t.Errorf("expected ≥5 eventlog events, got %d", eventLogCount)
	}

	t.Logf("smoke ok: %d eventlog events, blocks=%d, tokens in=%d out=%d",
		eventLogCount, len(final.Blocks), final.InputTokens, final.OutputTokens)
}
