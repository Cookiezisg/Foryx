package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func collectAnthropicEvents(sseText string) []StreamEvent {
	var events []StreamEvent
	r := strings.NewReader(sseText)
	parseAnthropicSSE(context.Background(), r, func(e StreamEvent) bool {
		events = append(events, e)
		return true
	})
	return events
}

func TestAnthropicParseSSE_TextOnly(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":10}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello "}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"world"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}

event: message_stop
data: {"type":"message_stop"}
`
	events := collectAnthropicEvents(sse)

	texts := filterType(events, EventText)
	if len(texts) != 2 {
		t.Fatalf("want 2 EventText, got %d", len(texts))
	}
	combined := texts[0].Delta + texts[1].Delta
	if combined != "Hello world" {
		t.Errorf("combined text = %q, want %q", combined, "Hello world")
	}

	finishes := filterType(events, EventFinish)
	if len(finishes) != 1 {
		t.Fatalf("want 1 EventFinish, got %d", len(finishes))
	}
	if finishes[0].FinishReason != "end_turn" {
		t.Errorf("finish_reason = %q, want end_turn", finishes[0].FinishReason)
	}
	if finishes[0].InputTokens != 10 || finishes[0].OutputTokens != 5 {
		t.Errorf("tokens = in:%d out:%d, want in:10 out:5",
			finishes[0].InputTokens, finishes[0].OutputTokens)
	}
}

func TestAnthropicParseSSE_ToolCall(t *testing.T) {
	sse := `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"Beijing\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":20}}
`
	events := collectAnthropicEvents(sse)

	starts := filterType(events, EventToolStart)
	if len(starts) != 1 {
		t.Fatalf("want 1 EventToolStart, got %d", len(starts))
	}
	if starts[0].ToolName != "get_weather" || starts[0].ToolID != "toolu_01" {
		t.Errorf("tool start: name=%q id=%q", starts[0].ToolName, starts[0].ToolID)
	}
	if starts[0].ToolIndex != 0 {
		t.Errorf("tool index = %d, want 0", starts[0].ToolIndex)
	}

	deltas := filterType(events, EventToolDelta)
	assembled := ""
	for _, d := range deltas {
		assembled += d.ArgsDelta
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(assembled), &args); err != nil {
		t.Errorf("assembled args not valid JSON: %q", assembled)
	}
	if args["city"] != "Beijing" {
		t.Errorf("city = %v, want Beijing", args["city"])
	}
}

func TestAnthropicParseSSE_ThinkingBlock(t *testing.T) {
	sse := `event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think..."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Answer"}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":10}}
`
	events := collectAnthropicEvents(sse)

	reasoning := filterType(events, EventReasoning)
	if len(reasoning) != 1 || reasoning[0].Delta != "Let me think..." {
		t.Errorf("reasoning events = %+v", reasoning)
	}
	texts := filterType(events, EventText)
	if len(texts) != 1 || texts[0].Delta != "Answer" {
		t.Errorf("text events = %+v", texts)
	}
}

func TestBuildAnthropicBody_SystemField(t *testing.T) {
	req := Request{
		ModelID: "claude-3-5-sonnet-20241022",
		System:  "You are helpful.",
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "Hello"},
		},
	}
	body, err := buildAnthropicBody(req)
	if err != nil {
		t.Fatalf("buildAnthropicBody: %v", err)
	}
	var out anthropicRequest
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.System != "You are helpful." {
		t.Errorf("system = %q, want 'You are helpful.'", out.System)
	}
	// System is NOT a message — messages should only have the user turn.
	// system 不是 message——messages 只应有 user 回合。
	if len(out.Messages) != 1 || out.Messages[0].Role != "user" {
		t.Errorf("messages = %+v", out.Messages)
	}
}

func TestBuildAnthropicBody_ToolResultGrouped(t *testing.T) {
	// Two consecutive RoleTool messages should be grouped into one user message.
	// 两条连续的 RoleTool 消息应合并为一条 user 消息。
	req := Request{
		ModelID: "claude-3-5-sonnet-20241022",
		Messages: []LLMMessage{
			{
				Role: RoleAssistant,
				ToolCalls: []LLMToolCall{
					{ID: "call_1", Name: "t1", Arguments: "{}"},
					{ID: "call_2", Name: "t2", Arguments: "{}"},
				},
			},
			{Role: RoleTool, Content: "result1", ToolCallID: "call_1"},
			{Role: RoleTool, Content: "result2", ToolCallID: "call_2"},
		},
	}
	body, err := buildAnthropicBody(req)
	if err != nil {
		t.Fatalf("buildAnthropicBody: %v", err)
	}
	var out anthropicRequest
	json.Unmarshal(body, &out)

	// Should be: [assistant, user(tool_results)]
	// 应为：[assistant, user(tool_results)]
	if len(out.Messages) != 2 {
		t.Fatalf("want 2 messages, got %d: %+v", len(out.Messages), out.Messages)
	}
	if out.Messages[1].Role != "user" {
		t.Errorf("second message role = %q, want user", out.Messages[1].Role)
	}
	if len(out.Messages[1].Content) != 2 {
		t.Errorf("tool result content blocks = %d, want 2", len(out.Messages[1].Content))
	}
	for _, blk := range out.Messages[1].Content {
		if blk.Type != "tool_result" {
			t.Errorf("content block type = %q, want tool_result", blk.Type)
		}
	}
}

func TestBuildAnthropicBody_ToolDefinition(t *testing.T) {
	req := Request{
		ModelID:  "claude-3-5-sonnet-20241022",
		Messages: []LLMMessage{{Role: RoleUser, Content: "hi"}},
		Tools: []ToolDef{{
			Name:        "get_weather",
			Description: "Get weather",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"city":{"type":"string"}}}`),
		}},
	}
	body, _ := buildAnthropicBody(req)
	var out anthropicRequest
	json.Unmarshal(body, &out)

	if len(out.Tools) != 1 {
		t.Fatalf("want 1 tool, got %d", len(out.Tools))
	}
	// Anthropic uses "input_schema" not "parameters".
	// Anthropic 用 "input_schema" 而非 "parameters"。
	if string(out.Tools[0].InputSchema) == "" {
		t.Error("input_schema should not be empty")
	}
}

func TestExtractBase64Data(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"data:image/png;base64,abc123", "abc123"},
		{"data:image/jpeg;base64,xyz", "xyz"},
		{"abc123", "abc123"}, // non data URL passes through
	}
	for _, c := range cases {
		got := extractBase64Data(c.input)
		if got != c.want {
			t.Errorf("extractBase64Data(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestExtractMediaType(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"data:image/png;base64,abc", "image/png"},
		{"data:image/jpeg;base64,abc", "image/jpeg"},
		{"https://example.com/img.jpg", "image/jpeg"}, // fallback
	}
	for _, c := range cases {
		got := extractMediaType(c.input)
		if got != c.want {
			t.Errorf("extractMediaType(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
