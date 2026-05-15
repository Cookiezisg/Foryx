// history.go — In-loop history extension. extendHistory is called after each
// tool-calling step. BlocksToAssistantLLM is exported so callers building
// historical history (e.g. chat.buildHistory loading from DB) reuse the same
// converter — there's only one source of truth for blocks → LLM wire shape.
//
// Block model (post-event-log-protocol unification):
//   - text/reasoning: Block.Content is the raw text (no JSON wrapper)
//   - tool_call: Block.ID is the LLM tool-call ID (tc_xxx); Block.Attrs
//     JSON has {tool: name}; Block.Content is the args JSON string
//   - tool_result: Block.ParentBlockID is the parent tool_call's ID
//     (= LLM tc_xxx); Block.Content is the result text; Block.Status
//     "error" means tool failed and Block.Error is the message
//
// history.go — 循环内历史扩展。extendHistory 在每个工具调用步骤后调用。
// BlocksToAssistantLLM 导出，让构建历史的调用方（如 chat.buildHistory 从 DB
// 加载）复用同一个转换器——blocks → LLM wire 形状只有一个事实源。
//
// Block 模型（事件日志协议统一后）：
//   - text/reasoning：Block.Content 是裸文本（无 JSON 包装）
//   - tool_call：Block.ID 是 LLM tool-call ID（tc_xxx）；Block.Attrs JSON
//     含 {tool: name}；Block.Content 是 args JSON 字符串
//   - tool_result：Block.ParentBlockID 是父 tool_call 的 ID（= LLM tc_xxx）；
//     Block.Content 是 result 文本；Block.Status "error" 表 tool 失败，
//     Block.Error 是错误信息
package loop

import (

	"go.uber.org/zap"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// extendHistory appends one ReAct step's contribution (assistant blocks +
// tool result blocks) to the running history.
//
// extendHistory 把一个 ReAct 步骤的贡献（assistant blocks + tool result blocks）
// 追加到运行中的历史。
func extendHistory(log *zap.Logger, history []llminfra.LLMMessage, aBlocks, rBlocks []chatdomain.Block) ([]llminfra.LLMMessage, error) {
	msgs, err := BlocksToAssistantLLM(log, append(aBlocks, rBlocks...))
	if err != nil {
		return nil, err
	}
	return append(history, msgs...), nil
}

// BlocksToAssistantLLM converts an assistant turn's blocks into LLM wire
// messages. A turn with tool calls expands to:
//
//	[assistant{text, reasoning, toolCalls}] + [N × role=tool messages]
//
// Used by both extendHistory (in-loop accumulation) and chat.buildHistory
// (DB-loaded historical messages) — single source of truth for the
// conversion.
//
// BlocksToAssistantLLM 把 assistant 回合的 blocks 转为 LLM 协议消息。
// 含工具调用的回合展开为：
//
//	[assistant{text, reasoning, toolCalls}] + [N 条 role=tool 消息]
//
// extendHistory（循环内累积）与 chat.buildHistory（从 DB 加载历史消息）共用
// ——转换器只有一个事实源。
func BlocksToAssistantLLM(log *zap.Logger, blocks []chatdomain.Block) ([]llminfra.LLMMessage, error) {
	assistant := llminfra.LLMMessage{Role: llminfra.RoleAssistant}
	var toolResults []llminfra.LLMMessage

	for _, b := range blocks {
		switch b.Type {
		case eventlogdomain.BlockTypeReasoning:
			assistant.ReasoningContent = b.Content

		case eventlogdomain.BlockTypeText:
			assistant.Content = b.Content

		case eventlogdomain.BlockTypeToolCall:
			// Tool name lives in Block.Attrs JSON {tool: name}; args
			// is Block.Content as raw JSON string. Block.ID is the
			// LLM tool-call ID (we use it directly as block id).
			//
			// Tool name 在 Block.Attrs JSON {tool: name}；args 是
			// Block.Content 裸 JSON 字符串。Block.ID 是 LLM tool-call ID
			// （直接复用作 block id）。
			// Attrs is now map[string]any (2026-05 serializer refactor) —
			// direct lookup, no JSON parse needed.
			// Attrs 2026-05 改 map[string]any,直接取键即可。
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
			// Tool-call ID = parent block ID (= LLM tc_id). Content
			// is the result text. Status="error" / Error field
			// signal tool failure but for LLM history we still emit
			// a role=tool message with the result content (LLM sees
			// the error as part of the result string).
			//
			// Tool-call ID = parent block ID（= LLM tc_id）。Content
			// 是 result 文本。Status="error" / Error 字段表 tool 失败，
			// 但给 LLM 历史仍发 role=tool 消息携 result 文本（LLM 在
			// result 串里看到错误）。
			content := b.Content
			if content == "" && b.Error != "" {
				content = b.Error
			}
			toolResults = append(toolResults, llminfra.LLMMessage{
				Role: llminfra.RoleTool, Content: content, ToolCallID: b.ParentBlockID,
			})
		}
	}

	// NOTE: TE-22's "reasoning_content → content fallback" lives in
	// infra/llm/openai.go::buildOpenAIAssistantMsg (wire-protocol
	// compliance is the wire client's job; doing it here would have
	// wrongly polluted the Anthropic path too).
	//
	// 注：TE-22 reasoning fallback 在 infra/llm/openai.go 内（OpenAI 协议
	// 合规归 OpenAI client 管）。本函数纯 schema 转换器。
	return append([]llminfra.LLMMessage{assistant}, toolResults...), nil
}

// ExtractTextContent returns the last text block's content from a block slice.
// Used by callers (chat for auto-titling; subagent as the tool_result string
// returned to the parent LLM).
//
// ExtractTextContent 从 block 列表返回最后一个 text block 的内容。供调用方
// 使用（chat 用作自动命名素材；subagent 用作返主 LLM 的 tool_result）。
func ExtractTextContent(blocks []chatdomain.Block) string {
	var last string
	for _, b := range blocks {
		if b.Type == eventlogdomain.BlockTypeText {
			last = b.Content
		}
	}
	return last
}
