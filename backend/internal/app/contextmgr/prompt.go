package contextmgr

import (
	"fmt"
	"strings"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
)

// CompactSystemPromptText exposes the compaction LLM system prompt to the §18 inventory endpoint.
//
// CompactSystemPromptText 把 compaction LLM 系统提示词暴露给 §18 总览端点。
func CompactSystemPromptText() string { return compactSystemPrompt }

const compactSystemPrompt = `Maintain a running summary of an ongoing conversation; older blocks are dropped to free context.

RULES:
1. PRESERVE all previous summary bullets. Strike (~~text~~) only if directly contradicted by new content.
2. APPEND new bullets from NEW CONTENT tagged [later] for chronology.
3. Sections (omit if empty): User request · Files touched · Tools called (high-level) · Errors & fixes · Decisions · Current state / next steps.
4. Keep summary under 1500 tokens.
5. Output the FULL updated summary. No commentary, no preamble.`

// buildCompactPrompt assembles previous summary + archiving blocks; per-block content is truncated.
//
// buildCompactPrompt 装配旧 summary + 待 archive blocks；每 block content 截断。
func buildCompactPrompt(previousSummary string, blocks []*chatdomain.Block, prevCoverSeq int64) string {
	var sb strings.Builder
	if strings.TrimSpace(previousSummary) == "" {
		sb.WriteString("PREVIOUS SUMMARY: (none yet — this is the first compaction)\n\n")
	} else {
		fmt.Fprintf(&sb, "PREVIOUS SUMMARY (covering blocks up to seq %d):\n%s\n\n",
			prevCoverSeq, previousSummary)
	}
	if len(blocks) == 0 {
		sb.WriteString("NEW CONTENT: (no new blocks; emit the previous summary verbatim)")
		return sb.String()
	}
	fmt.Fprintf(&sb, "NEW CONTENT (%d blocks, seq %d to %d):\n",
		len(blocks), blocks[0].Seq, blocks[len(blocks)-1].Seq)
	for _, b := range blocks {
		body := truncateForPrompt(b.Content, perBlockBudget)
		role := "assistant"
		switch b.Type {
		case eventlogdomain.BlockTypeToolCall:
			role = "tool_call"
		case eventlogdomain.BlockTypeToolResult:
			role = "tool_result"
		case eventlogdomain.BlockTypeReasoning:
			role = "reasoning"
		}
		fmt.Fprintf(&sb, "\n--- [%s seq=%d type=%s] ---\n%s\n", role, b.Seq, b.Type, body)
	}
	sb.WriteString("\nTASK: Output the FULL updated summary.")
	return sb.String()
}

const perBlockBudget = 1500

func truncateForPrompt(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...[truncated]"
}
