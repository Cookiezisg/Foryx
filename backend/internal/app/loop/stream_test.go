package loop

import (
	"strings"
	"testing"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
)

var _ = chatdomain.Block{}

func makeAccums(triples ...string) map[int]*toolAccum {
	m := map[int]*toolAccum{}
	for i := 0; i+2 < len(triples); i += 3 {
		a := &toolAccum{id: triples[i], name: triples[i+1]}
		a.args.WriteString(triples[i+2])
		m[i/3] = a
	}
	return m
}

func TestAssemble_TextOnly(t *testing.T) {
	blocks := assembleBlocks("Hello world", "", nil)
	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(blocks))
	}
	if blocks[0].Type != eventlogdomain.BlockTypeText {
		t.Errorf("type = %q, want text", blocks[0].Type)
	}
	if blocks[0].Content != "Hello world" {
		t.Errorf("content = %q", blocks[0].Content)
	}
}

func TestAssemble_ReasoningThenText(t *testing.T) {
	blocks := assembleBlocks("The answer is 42.", "Let me think...", nil)
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Type != eventlogdomain.BlockTypeReasoning {
		t.Errorf("blocks[0] = %q, want reasoning", blocks[0].Type)
	}
	if blocks[1].Type != eventlogdomain.BlockTypeText {
		t.Errorf("blocks[1] = %q, want text", blocks[1].Type)
	}
}

func TestAssemble_TextThenToolCall(t *testing.T) {
	accums := makeAccums("call_1", "get_weather", `{"city":"Beijing"}`)
	blocks := assembleBlocks("Let me check the weather.", "", accums)
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks (text + tool_call), got %d", len(blocks))
	}
	if blocks[0].Type != eventlogdomain.BlockTypeText {
		t.Errorf("blocks[0] = %q, want text", blocks[0].Type)
	}
	if blocks[1].Type != eventlogdomain.BlockTypeToolCall {
		t.Errorf("blocks[1] = %q, want tool_call", blocks[1].Type)
	}
}

func TestAssemble_ToolCallOnly(t *testing.T) {
	accums := makeAccums("call_1", "get_weather", `{"summary":"Checking Beijing weather","city":"Beijing"}`)
	blocks := assembleBlocks("", "", accums)
	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(blocks))
	}
	if blocks[0].Type != eventlogdomain.BlockTypeToolCall {
		t.Errorf("type = %q, want tool_call", blocks[0].Type)
	}
	if blocks[0].ID != "call_1" {
		t.Errorf("id = %q, want call_1", blocks[0].ID)
	}
	if got, _ := blocks[0].Attrs["tool"].(string); got != "get_weather" {
		t.Errorf("attrs.tool = %q, want get_weather (full attrs: %#v)", got, blocks[0].Attrs)
	}
	if strings.Contains(blocks[0].Content, `"summary"`) {
		t.Errorf("Content should have summary stripped, got %q", blocks[0].Content)
	}
	if !strings.Contains(blocks[0].Content, `"city":"Beijing"`) {
		t.Errorf("Content missing city: %q", blocks[0].Content)
	}
}

func TestAssemble_ParallelToolCalls(t *testing.T) {
	accums := map[int]*toolAccum{}
	a0 := &toolAccum{id: "call_1", name: "get_weather"}
	a0.args.WriteString(`{"city":"Beijing"}`)
	a1 := &toolAccum{id: "call_2", name: "get_time"}
	a1.args.WriteString(`{"tz":"UTC"}`)
	accums[0] = a0
	accums[1] = a1

	blocks := assembleBlocks("", "", accums)
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(blocks))
	}
	if blocks[0].ID != "call_1" || blocks[1].ID != "call_2" {
		t.Errorf("ids = %q %q", blocks[0].ID, blocks[1].ID)
	}
}

func TestAssemble_FullReactStep(t *testing.T) {
	accums := makeAccums("call_1", "get_weather", `{"city":"Beijing"}`)
	blocks := assembleBlocks("Let me look that up.", "I'll check the weather first.", accums)
	if len(blocks) != 3 {
		t.Fatalf("want 3 blocks, got %d", len(blocks))
	}
	if blocks[0].Type != eventlogdomain.BlockTypeReasoning {
		t.Errorf("blocks[0] = %q, want reasoning", blocks[0].Type)
	}
	if blocks[1].Type != eventlogdomain.BlockTypeText {
		t.Errorf("blocks[1] = %q, want text", blocks[1].Type)
	}
	if blocks[2].Type != eventlogdomain.BlockTypeToolCall {
		t.Errorf("blocks[2] = %q, want tool_call", blocks[2].Type)
	}
}

func TestAssemble_Empty(t *testing.T) {
	blocks := assembleBlocks("", "", nil)
	if len(blocks) != 0 {
		t.Errorf("want 0 blocks, got %d", len(blocks))
	}
}

func TestParseToolArgs_WithAllStandardFields(t *testing.T) {
	fields, args := parseToolArgs(`{"summary":"doing X","destructive":true,"execution_group":3,"key":"val"}`)
	if fields.Summary != "doing X" {
		t.Errorf("Summary = %q", fields.Summary)
	}
	if !fields.Destructive {
		t.Errorf("Destructive = false, want true")
	}
	if fields.ExecutionGroup != 3 {
		t.Errorf("ExecutionGroup = %d, want 3", fields.ExecutionGroup)
	}
	if args["key"] != "val" {
		t.Errorf("key = %v", args["key"])
	}
	if _, ok := args["summary"]; ok {
		t.Error("summary should be stripped from args map")
	}
	if _, ok := args["destructive"]; ok {
		t.Error("destructive should be stripped from args map")
	}
	if _, ok := args["execution_group"]; ok {
		t.Error("execution_group should be stripped from args map")
	}
}

func TestParseToolArgs_NoStandardFields(t *testing.T) {
	fields, args := parseToolArgs(`{"key":"val"}`)
	if fields.Summary != "" {
		t.Errorf("Summary = %q, want empty", fields.Summary)
	}
	if fields.Destructive {
		t.Error("Destructive = true, want false (default when missing)")
	}
	if fields.ExecutionGroup != 0 {
		t.Errorf("ExecutionGroup = %d, want 0 (auto when missing)", fields.ExecutionGroup)
	}
	if args["key"] != "val" {
		t.Errorf("key = %v", args["key"])
	}
}

func TestParseToolArgs_MalformedJSON(t *testing.T) {
	fields, args := parseToolArgs(`not-json`)
	if fields.Summary != "" {
		t.Errorf("Summary = %q, want empty for bad JSON", fields.Summary)
	}
	if fields.Destructive {
		t.Error("Destructive = true, want false for bad JSON")
	}
	if fields.ExecutionGroup != 0 {
		t.Errorf("ExecutionGroup = %d, want 0 for bad JSON", fields.ExecutionGroup)
	}
	if args["raw"] != "not-json" {
		t.Errorf("fallback raw = %v", args["raw"])
	}
}

func TestCollectToolCalls_Mixed(t *testing.T) {
	accums := makeAccums("c1", "t1", `{"x":1}`)
	calls := collectToolCalls(accums)
	if len(calls) != 1 {
		t.Fatalf("want 1 tool call, got %d", len(calls))
	}
	if calls[0].ID != "c1" || calls[0].Name != "t1" {
		t.Errorf("call = %+v", calls[0])
	}
}

func TestCollectToolCalls_None(t *testing.T) {
	if calls := collectToolCalls(nil); len(calls) != 0 {
		t.Errorf("want 0 calls, got %d", len(calls))
	}
}

func TestMakeAccums_WritesArgs(t *testing.T) {
	accums := makeAccums("id1", "name1", `{"k":"v"}`)
	a, ok := accums[0]
	if !ok {
		t.Fatal("accums[0] not set")
	}
	if a.id != "id1" || a.name != "name1" {
		t.Errorf("id/name = %q/%q", a.id, a.name)
	}
	if got := a.args.String(); !strings.Contains(got, `"k"`) {
		t.Errorf("args = %q", got)
	}
}
