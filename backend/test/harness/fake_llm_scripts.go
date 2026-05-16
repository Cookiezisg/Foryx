//go:build pipeline

package harness

import "time"

// ScriptText splits content across multiple SSE chunks for streaming-snapshot tests.
//
// ScriptText 把 content 分多帧发，给流式快照测试用。
func ScriptText(content string) Script {
	return Script{
		Actions:      splitTextActions(content, 3),
		FinishReason: "stop",
		InputTokens:  12,
		OutputTokens: 5,
	}
}

// ScriptSlowText splits content into chunks with chunkDelay between each for cancel tests.
//
// ScriptSlowText 多帧 + 帧间 chunkDelay，给 cancel 测试用。
func ScriptSlowText(content string, chunkDelay time.Duration) Script {
	chunks := splitTextActions(content, 8)
	actions := make([]ChunkAction, 0, len(chunks)*2)
	for _, c := range chunks {
		actions = append(actions, c)
		actions = append(actions, ChunkAction{Kind: "delay", Delay: chunkDelay})
	}
	return Script{
		Actions:      actions,
		FinishReason: "stop",
		InputTokens:  20,
		OutputTokens: 10,
	}
}

// ScriptSingleToolCall emits one tool call with finish_reason=tool_calls; argsJSON must be valid JSON.
//
// ScriptSingleToolCall 发一次 tool call，argsJSON 必须是合法 JSON。
func ScriptSingleToolCall(name, toolID, argsJSON string) Script {
	return Script{
		Actions: []ChunkAction{
			{Kind: "tool_call_start", Name: name, ToolID: toolID, Index: 0},
			{Kind: "tool_call_delta", Index: 0, Content: argsJSON},
		},
		FinishReason: "tool_calls",
		InputTokens:  15,
		OutputTokens: 8,
	}
}

// ScriptHTTPError makes the fake server return status immediately without streaming.
//
// ScriptHTTPError 让 fake server 直接返指定 HTTP 状态，不流式。
func ScriptHTTPError(status int) Script {
	return Script{HTTPStatus: status}
}

// ScriptRawJSON emits a single text chunk with payload verbatim.
//
// ScriptRawJSON 单帧发出 payload。
func ScriptRawJSON(payload string) Script {
	return Script{
		Actions:      []ChunkAction{{Kind: "text", Content: payload}},
		FinishReason: "stop",
		InputTokens:  5,
		OutputTokens: 3,
	}
}

// ToolCallSpec describes one tool call for ScriptParallelToolCalls.
//
// ToolCallSpec 描述 ScriptParallelToolCalls 里的一次 tool call。
type ToolCallSpec struct {
	Name     string
	ToolID   string
	ArgsJSON string
}

// ScriptParallelToolCalls emits multiple tool calls in one response with finish_reason=tool_calls.
//
// ScriptParallelToolCalls 单次响应发多个并行 tool call。
func ScriptParallelToolCalls(calls []ToolCallSpec) Script {
	actions := make([]ChunkAction, 0, len(calls)*2)
	for i, c := range calls {
		actions = append(actions,
			ChunkAction{Kind: "tool_call_start", Name: c.Name, ToolID: c.ToolID, Index: i},
			ChunkAction{Kind: "tool_call_delta", Index: i, Content: c.ArgsJSON},
		)
	}
	return Script{
		Actions:      actions,
		FinishReason: "tool_calls",
		InputTokens:  15,
		OutputTokens: 10,
	}
}

// splitTextActions divides content into up to n equal text chunk actions.
//
// splitTextActions 把 content 分成最多 n 个等长 text chunk action。
func splitTextActions(content string, n int) []ChunkAction {
	runes := []rune(content)
	total := len(runes)
	if total == 0 || n <= 1 {
		return []ChunkAction{{Kind: "text", Content: content}}
	}
	chunkSize := (total + n - 1) / n
	actions := make([]ChunkAction, 0, n)
	for i := 0; i < total; i += chunkSize {
		end := i + chunkSize
		if end > total {
			end = total
		}
		actions = append(actions, ChunkAction{Kind: "text", Content: string(runes[i:end])})
	}
	return actions
}
