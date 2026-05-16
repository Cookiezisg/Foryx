package loop

import (
	"fmt"

	"go.uber.org/zap"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// ContextRole values mirror contextmgr.ContextRole* (duplicated to keep loop free of app imports).
//
// ContextRole 与 contextmgr.ContextRole* 同名镜像；避免 loop 引 app 层。
const (
	contextRoleHot      = "hot"
	contextRoleWarm     = "warm"
	contextRoleCold     = "cold"
	contextRoleArchived = "archived"

	warmPreviewBytes = 200
)

// extendHistory appends one ReAct step (assistant blocks + tool results) to running history.
//
// extendHistory 把一个 ReAct 步骤（assistant + tool result）追加到运行历史。
func extendHistory(log *zap.Logger, history []llminfra.LLMMessage, aBlocks, rBlocks []chatdomain.Block) ([]llminfra.LLMMessage, error) {
	msgs, err := BlocksToAssistantLLM(log, append(aBlocks, rBlocks...))
	if err != nil {
		return nil, err
	}
	return append(history, msgs...), nil
}

// BlocksToAssistantLLM converts an assistant turn's blocks to [assistant + N×tool] LLM messages.
//
// BlocksToAssistantLLM 把 assistant 回合的 blocks 转为 [assistant + N×tool] LLM 消息。
func BlocksToAssistantLLM(log *zap.Logger, blocks []chatdomain.Block) ([]llminfra.LLMMessage, error) {
	assistant := llminfra.LLMMessage{Role: llminfra.RoleAssistant}
	var toolResults []llminfra.LLMMessage

	for _, b := range blocks {
		// archived + compaction blocks drop — content lives in conversation.summary.
		if b.ContextRole == contextRoleArchived {
			continue
		}
		if b.Type == eventlogdomain.BlockTypeCompaction {
			continue
		}
		switch b.Type {
		case eventlogdomain.BlockTypeReasoning:
			assistant.ReasoningContent = b.Content

		case eventlogdomain.BlockTypeText:
			assistant.Content = b.Content

		case eventlogdomain.BlockTypeToolCall:
			toolName := ""
			if b.Attrs != nil {
				if v, ok := b.Attrs["tool"].(string); ok {
					toolName = v
				}
			}
			assistant.ToolCalls = append(assistant.ToolCalls, llminfra.LLMToolCall{
				ID: b.ID, Name: toolName, Arguments: b.Content,
			})

		case eventlogdomain.BlockTypeToolResult:
			toolResults = append(toolResults, llminfra.LLMMessage{
				Role: llminfra.RoleTool, Content: projectToolResultContent(b), ToolCallID: b.ParentBlockID,
			})
		}
	}

	return append([]llminfra.LLMMessage{assistant}, toolResults...), nil
}

// projectToolResultContent renders tool_result per ContextRole (hot full / warm preview / cold omitted).
//
// projectToolResultContent 按 ContextRole 渲染 tool_result（hot 全文、warm preview、cold omitted）。
func projectToolResultContent(b chatdomain.Block) string {
	content := b.Content
	if content == "" && b.Error != "" {
		content = b.Error
	}
	switch b.ContextRole {
	case contextRoleWarm:
		if len(content) > warmPreviewBytes {
			return content[:warmPreviewBytes] +
				fmt.Sprintf("\n...[truncated, %d total bytes]", len(content))
		}
		return content
	case contextRoleCold:
		toolName := ""
		if b.Attrs != nil {
			if v, ok := b.Attrs["tool"].(string); ok {
				toolName = v
			}
		}
		if toolName == "" {
			return fmt.Sprintf("[tool_result omitted to save context (%d bytes)]", len(b.Content))
		}
		return fmt.Sprintf("[%s output omitted to save context (%d bytes)]",
			toolName, len(b.Content))
	default:
		return content
	}
}

// ExtractTextContent returns the last text block's content (used by autoTitle / subagent tool_result).
//
// ExtractTextContent 返回最后一个 text block 的内容（供 autoTitle / subagent tool_result 使用）。
func ExtractTextContent(blocks []chatdomain.Block) string {
	var last string
	for _, b := range blocks {
		if b.Type == eventlogdomain.BlockTypeText {
			last = b.Content
		}
	}
	return last
}
