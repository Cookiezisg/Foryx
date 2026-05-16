package llm

// SanitizeMessages enforces assistant.tool_calls ↔ tool message pairing; returns a new slice.
//
// SanitizeMessages 守 assistant.tool_calls ↔ tool message 配对，返回新 slice。
func SanitizeMessages(msgs []LLMMessage) []LLMMessage {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]LLMMessage, 0, len(msgs))
	i := 0
	for i < len(msgs) {
		m := msgs[i]
		i++

		if m.Role == RoleTool {
			continue
		}

		out = append(out, m)
		if m.Role != RoleAssistant || len(m.ToolCalls) == 0 {
			continue
		}

		expected := make(map[string]bool, len(m.ToolCalls))
		for _, tc := range m.ToolCalls {
			expected[tc.ID] = true
		}
		for i < len(msgs) && msgs[i].Role == RoleTool {
			t := msgs[i]
			i++
			if t.ToolCallID == "" || !expected[t.ToolCallID] {
				continue
			}
			out = append(out, t)
			delete(expected, t.ToolCallID)
		}

		// Stub missing IDs so the LLM sees the interruption.
		// 给未配对的 ID 合成 stub，让 LLM 知道是被打断。
		for _, tc := range m.ToolCalls {
			if expected[tc.ID] {
				out = append(out, LLMMessage{
					Role:       RoleTool,
					ToolCallID: tc.ID,
					Content:    "[interrupted: tool call did not complete]",
				})
			}
		}
	}
	return out
}
