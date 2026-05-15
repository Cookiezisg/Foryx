// history_test.go — unit tests for BlocksToAssistantLLM. Synthetic blocks
// exercise the converter shared by chat.buildHistory (DB-loaded historical
// messages) and loop.extendHistory (in-loop accumulation).
//
// New Block model (post event-log unification):
//   - text/reasoning: Block.Content is the raw text (no JSON wrapper)
//   - tool_call: Block.ID is the LLM tool-call ID; Block.Attrs JSON has
//     {tool: name}; Block.Content is the args JSON string
//   - tool_result: Block.ParentBlockID = LLM tool-call ID;
//     Block.Content = result text
//
// history_test.go ——BlocksToAssistantLLM 的单元测试。合成 block 演练
// chat.buildHistory（DB 加载历史）与 loop.extendHistory（循环内累积）
// 共享的转换器。
//
// 新 Block 模型（事件日志统一后）：
//   - text/reasoning：Block.Content 裸文本
//   - tool_call：Block.ID = LLM tool-call ID；Block.Attrs JSON 含
//     {tool: name}；Block.Content 是 args JSON
//   - tool_result：Block.ParentBlockID = LLM tool-call ID；
//     Block.Content = result 文本
package loop

import (
	"testing"

	"go.uber.org/zap"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

func textBlock(id, content string) chatdomain.Block {
	return chatdomain.Block{
		ID: id, Type: eventlogdomain.BlockTypeText, Content: content,
		Status: eventlogdomain.StatusCompleted,
	}
}

func reasoningBlock(id, content string) chatdomain.Block {
	return chatdomain.Block{
		ID: id, Type: eventlogdomain.BlockTypeReasoning, Content: content,
		Status: eventlogdomain.StatusCompleted,
	}
}

func toolCallBlock(id, name, argsJSON string) chatdomain.Block {
	return chatdomain.Block{
		ID: id, Type: eventlogdomain.BlockTypeToolCall, Content: argsJSON,
		Attrs:  map[string]any{"tool": name},
		Status: eventlogdomain.StatusCompleted,
	}
}

func toolResultBlock(id, parentID, result string) chatdomain.Block {
	return chatdomain.Block{
		ID: id, Type: eventlogdomain.BlockTypeToolResult, Content: result,
		ParentBlockID: parentID,
		Status:        eventlogdomain.StatusCompleted,
	}
}

func TestBuildAssistant_TextOnly(t *testing.T) {
	msgs, err := BlocksToAssistantLLM(zap.NewNop(), []chatdomain.Block{textBlock("b1", "Hello world")})
	if err != nil {
		t.Fatalf("BlocksToAssistantLLM: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != llminfra.RoleAssistant {
		t.Errorf("role = %q, want assistant", msgs[0].Role)
	}
	if msgs[0].Content != "Hello world" {
		t.Errorf("content = %q, want 'Hello world'", msgs[0].Content)
	}
}

func TestBuildAssistant_WithReasoning(t *testing.T) {
	msgs, err := BlocksToAssistantLLM(zap.NewNop(), []chatdomain.Block{
		reasoningBlock("b1", "Let me think"),
		textBlock("b2", "Answer"),
	})
	if err != nil {
		t.Fatalf("BlocksToAssistantLLM: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 assistant message, got %d", len(msgs))
	}
	if msgs[0].ReasoningContent != "Let me think" {
		t.Errorf("reasoning = %q", msgs[0].ReasoningContent)
	}
	if msgs[0].Content != "Answer" {
		t.Errorf("content = %q", msgs[0].Content)
	}
}

func TestBuildAssistant_WithToolCall(t *testing.T) {
	msgs, err := BlocksToAssistantLLM(zap.NewNop(), []chatdomain.Block{
		toolCallBlock("call_1", "get_weather", `{"city":"Beijing"}`),
		toolResultBlock("blk_r1", "call_1", "晴，25°C"),
	})
	if err != nil {
		t.Fatalf("BlocksToAssistantLLM: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	a := msgs[0]
	if a.Role != llminfra.RoleAssistant {
		t.Errorf("msgs[0] role = %q", a.Role)
	}
	if len(a.ToolCalls) != 1 {
		t.Fatalf("want 1 tool call, got %d", len(a.ToolCalls))
	}
	if a.ToolCalls[0].Name != "get_weather" || a.ToolCalls[0].ID != "call_1" {
		t.Errorf("tool call: %+v", a.ToolCalls[0])
	}
	if a.ToolCalls[0].Arguments != `{"city":"Beijing"}` {
		t.Errorf("args = %q", a.ToolCalls[0].Arguments)
	}
	tr := msgs[1]
	if tr.Role != llminfra.RoleTool || tr.ToolCallID != "call_1" || tr.Content != "晴，25°C" {
		t.Errorf("tool result = %+v", tr)
	}
}

func TestBuildAssistant_MultipleToolCalls(t *testing.T) {
	msgs, err := BlocksToAssistantLLM(zap.NewNop(), []chatdomain.Block{
		toolCallBlock("call_1", "t1", `{}`),
		toolCallBlock("call_2", "t2", `{}`),
		toolResultBlock("r1", "call_1", "r1"),
		toolResultBlock("r2", "call_2", "r2"),
	})
	if err != nil {
		t.Fatalf("BlocksToAssistantLLM: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("want 3 messages, got %d", len(msgs))
	}
	if len(msgs[0].ToolCalls) != 2 {
		t.Errorf("want 2 tool calls, got %d", len(msgs[0].ToolCalls))
	}
}

func TestBlocksToLLM_RoundTrip(t *testing.T) {
	input := []chatdomain.Block{
		reasoningBlock("b1", "thinking"),
		toolCallBlock("c1", "t1", `{"city":"Shanghai"}`),
		toolResultBlock("b3", "c1", "sunny"),
		textBlock("b4", "done"),
	}
	msgs, err := BlocksToAssistantLLM(zap.NewNop(), input)
	if err != nil {
		t.Fatalf("BlocksToAssistantLLM: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	a := msgs[0]
	if a.ReasoningContent != "thinking" {
		t.Errorf("reasoning = %q", a.ReasoningContent)
	}
	if a.Content != "done" {
		t.Errorf("content = %q", a.Content)
	}
	if len(a.ToolCalls) != 1 || a.ToolCalls[0].ID != "c1" {
		t.Errorf("tool calls = %+v", a.ToolCalls)
	}
	tr := msgs[1]
	if tr.Role != llminfra.RoleTool || tr.ToolCallID != "c1" || tr.Content != "sunny" {
		t.Errorf("tool result = %+v", tr)
	}
}

func TestBlocksToLLM_TextOnly(t *testing.T) {
	msgs, err := BlocksToAssistantLLM(zap.NewNop(), []chatdomain.Block{textBlock("b1", "hi")})
	if err != nil {
		t.Fatalf("BlocksToAssistantLLM: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "hi" {
		t.Errorf("content = %q", msgs[0].Content)
	}
}
