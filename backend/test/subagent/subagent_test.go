//go:build pipeline

// Package subagent runs pipeline tests for the Subagent system tool.
//
// Package subagent 跑 Subagent 系统工具 pipeline 测试。
package subagent

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// findSubagentRuns returns messages flagged attrs.kind=subagent_run, decoded as maps.
//
// findSubagentRuns 返 attrs.kind=subagent_run 的 messages 行，解为 map。
func findSubagentRuns(t *testing.T, h *th.Harness, convID string) []map[string]any {
	t.Helper()
	type row struct {
		ID     string `gorm:"column:id"`
		Status string `gorm:"column:status"`
		Attrs  string `gorm:"column:attrs"`
	}
	var rows []row
	if err := h.DB.Raw(
		`SELECT id, status, attrs FROM messages
		 WHERE conversation_id = ? AND attrs != ''
		   AND json_extract(attrs, '$.kind') = 'subagent_run'`,
		convID,
	).Scan(&rows).Error; err != nil {
		t.Fatalf("query subagent runs: %v", err)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		var a map[string]any
		_ = json.Unmarshal([]byte(r.Attrs), &a)
		if a == nil {
			a = map[string]any{}
		}
		a["id"] = r.ID
		a["status"] = r.Status
		out = append(out, a)
	}
	return out
}

func TestSubagent_Spawn_EndToEnd(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"Subagent", "call_sub_1",
		`{"subagent_type":"general-purpose","prompt":"summarize the project","summary":"delegating to subagent"}`,
	))
	fake.PushScript(th.ScriptText("Forgify is a local-first agentic workflow platform built around a Go backend with sub-domains for chat, forge, and sandbox."))
	fake.PushScript(th.ScriptText("I delegated the question to a subagent — see its summary above."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "subagent-end-to-end")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "What is this project?")

	final := sub.WaitForAssistantTerminal(60 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("parent status=%q errCode=%q errMsg=%q\nraw:\n%s",
			final.Status, final.ErrorCode, final.ErrorMessage, sub.FormatRawEvents())
	}

	tcID, ok := th.ExtractToolCallByName(final.Blocks, "Subagent")
	if !ok {
		t.Fatalf("no Subagent tool_call block in parent final\nraw:\n%s", sub.FormatRawEvents())
	}
	resultData, ok := th.ExtractToolResultByCallID(final.Blocks, tcID)
	if !ok {
		t.Fatalf("no paired tool_result for Subagent call %q", tcID)
	}
	if okFlag, _ := resultData["ok"].(bool); !okFlag {
		t.Errorf("Subagent tool_result.ok=false; data=%v", resultData)
	}
	resultText, _ := resultData["result"].(string)
	if !strings.Contains(resultText, "Forgify") {
		t.Errorf("Subagent tool_result text doesn't echo sub-runner's message: %q", resultText)
	}

	runs := findSubagentRuns(t, h, conv.ID)
	if len(runs) != 1 {
		t.Fatalf("subagent run count = %d, want 1", len(runs))
	}
	if runs[0]["status"] != "completed" {
		t.Errorf("subagent run status = %v, want completed", runs[0]["status"])
	}
	if runs[0]["type"] != "general-purpose" {
		t.Errorf("subagent run type = %v, want general-purpose", runs[0]["type"])
	}

	runID, _ := runs[0]["id"].(string)
	var blockCount int64
	if err := h.DB.Raw(
		`SELECT COUNT(*) FROM message_blocks WHERE message_id = ?`, runID,
	).Scan(&blockCount).Error; err != nil {
		t.Fatalf("count message_blocks for sub-run: %v", err)
	}
	if blockCount < 1 {
		t.Errorf("message_blocks for sub-run = %d, want ≥ 1", blockCount)
	}
}

func TestSubagent_EventLog_CarriesSubagentRunMetadata(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"Subagent", "call_sub_2",
		`{"subagent_type":"general-purpose","prompt":"give me a one-line description","summary":"checking eventlog"}`,
	))
	fake.PushScript(th.ScriptText("A focused subagent that streams its work back through the eventlog."))
	fake.PushScript(th.ScriptText("Done."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "subagent-eventlog")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Describe yourself.")

	final := sub.WaitForAssistantTerminal(60 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("parent status=%q\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}

	var sawSubagentStart bool
	for _, ev := range sub.RawEvents() {
		if ev.Source != "eventlog" || ev.Type != "message_start" {
			continue
		}
		var payload struct {
			ConversationID string         `json:"conversationId"`
			Attrs          map[string]any `json:"attrs"`
		}
		if err := json.Unmarshal(ev.Data, &payload); err != nil {
			continue
		}
		if payload.Attrs["kind"] != "subagent_run" {
			continue
		}
		sawSubagentStart = true
		if payload.ConversationID != conv.ID {
			t.Errorf("subagent message_start conversationId = %q, want %s", payload.ConversationID, conv.ID)
		}
		if payload.Attrs["type"] != "general-purpose" {
			t.Errorf("subagent message_start attrs.type = %v, want general-purpose", payload.Attrs["type"])
		}
	}
	if !sawSubagentStart {
		t.Errorf("no message_start with attrs.kind=subagent_run during the run\nraw:\n%s",
			sub.FormatRawEvents())
	}
}

func TestSubagent_MaxTurns_Triggered(t *testing.T) {
	fake := th.NewFakeLLMServer(t)

	fake.PushScript(th.ScriptSingleToolCall(
		"Subagent", "call_sub_max",
		`{"subagent_type":"general-purpose","prompt":"loop forever","max_turns":1,"summary":"max-turns test"}`,
	))
	fake.PushScript(th.ScriptSingleToolCall(
		"NonexistentTool", "call_loop_x",
		`{"summary":"keep looping"}`,
	))
	fake.PushDefault(th.ScriptText("(should not be reached)"))

	fake.PushScript(th.ScriptText("Sub-run hit its max turns as expected."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")
	conv := h.NewConversation(t, "subagent-max-turns")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Try a loopy task.")

	final := sub.WaitForAssistantTerminal(60 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("parent status=%q\nraw:\n%s", final.Status, sub.FormatRawEvents())
	}

	tcID, ok := th.ExtractToolCallByName(final.Blocks, "Subagent")
	if !ok {
		t.Fatalf("no Subagent tool_call in parent final")
	}
	resultData, ok := th.ExtractToolResultByCallID(final.Blocks, tcID)
	if !ok {
		t.Fatalf("no paired tool_result for Subagent call")
	}
	resultText, _ := resultData["result"].(string)
	if !strings.Contains(resultText, "max turns") {
		t.Errorf("tool_result text missing max-turns note: %q", resultText)
	}

	runs := findSubagentRuns(t, h, conv.ID)
	if len(runs) != 1 {
		t.Fatalf("subagent run count = %d, want 1", len(runs))
	}
	if runs[0]["status"] != "max_turns" {
		t.Errorf("subagent run status = %v, want max_turns", runs[0]["status"])
	}
}
