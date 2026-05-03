// stream.go — One LLM call: consume stream events, publish ChatMessage
// snapshots, assemble Blocks. No database writes happen here. The caller
// (agentRun) owns persistence.
//
// stream.go — 单次 LLM 调用：消费流事件、推 ChatMessage 快照、组装 Block。
// 不写 DB——持久化由调用方 agentRun 负责。
package chat

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// toolAccum accumulates streaming fragments for one tool call.
//
// toolAccum 累积单个 tool call 的流式片段。
type toolAccum struct {
	id, name string
	args     strings.Builder
}

// streamLLM executes one LLM call. As each stream event arrives it rebuilds
// the in-progress Message snapshot (parentBlocks + this step's freshly
// assembled blocks) and publishes a chat.message event. errMsg is "" on
// success; on EventError it carries the upstream error text so the caller
// can stamp it onto the persisted Message.
//
// parentBlocks are the blocks already accumulated from earlier ReAct steps —
// per-token snapshots prepend them so subscribers always see the full
// message-so-far.
//
// streamLLM 执行一次 LLM 调用。每个流事件到达时重建当前 Message 快照
// （parentBlocks + 本步骤刚组装的 blocks），并推送 chat.message 事件。
// errMsg 在成功时为 ""；EventError 触发时携带上游错误文本，供调用方写回 Message。
//
// parentBlocks 是 ReAct 之前步骤累积的 blocks——每 token 快照把它们前置，
// 让订阅者始终看到完整 message-so-far。
func (s *Service) streamLLM(
	ctx context.Context,
	client llminfra.Client,
	req llminfra.Request,
	convID, msgID, uid string,
	parentBlocks []chatdomain.Block,
) (blocks []chatdomain.Block, toolCalls []chatdomain.ToolCallData, stopReason string, errMsg string, inputTokens, outputTokens int) {
	var textBuf, reasonBuf strings.Builder
	accums := map[int]*toolAccum{}
	stopReason = chatdomain.StopReasonEndTurn

	publish := func() {
		current := assembleBlocks(textBuf.String(), reasonBuf.String(), accums)
		s.publishMessageSnapshot(ctx, msgID, convID, uid,
			joinBlocks(parentBlocks, current),
			chatdomain.StatusStreaming, "", "", "",
			inputTokens, outputTokens)
	}

	for event := range client.Stream(ctx, req) {
		switch event.Type {
		case llminfra.EventText:
			textBuf.WriteString(event.Delta)
			publish()

		case llminfra.EventReasoning:
			reasonBuf.WriteString(event.Delta)
			publish()

		case llminfra.EventToolStart:
			accums[event.ToolIndex] = &toolAccum{id: event.ToolID, name: event.ToolName}
			publish()

		case llminfra.EventToolDelta:
			if a := accums[event.ToolIndex]; a != nil {
				a.args.WriteString(event.ArgsDelta)
				publish()
			}

		case llminfra.EventFinish:
			if event.FinishReason == "length" {
				stopReason = chatdomain.StopReasonMaxTokens
			}
			if event.InputTokens > 0 {
				inputTokens = event.InputTokens
			}
			if event.OutputTokens > 0 {
				outputTokens = event.OutputTokens
			}

		case llminfra.EventError:
			if ctx.Err() != nil {
				stopReason = chatdomain.StopReasonCancelled
			} else {
				stopReason = chatdomain.StopReasonError
				if event.Err != nil {
					errMsg = event.Err.Error()
				}
			}
		}
	}

	if ctx.Err() != nil && stopReason == chatdomain.StopReasonEndTurn {
		stopReason = chatdomain.StopReasonCancelled
	}

	blocks = assembleBlocks(textBuf.String(), reasonBuf.String(), accums)
	toolCalls = extractToolCalls(blocks)
	return
}

// assembleBlocks builds the final Block slice from accumulated stream buffers.
// Order: reasoning → text → tool_calls (by ToolIndex). Seq is stamped locally
// here and overwritten globally by stampBlocks when written to the database.
//
// assembleBlocks 从流缓冲组装最终的 Block 列表。
// 顺序：reasoning → text → tool_calls（按 ToolIndex）。
// Seq 在此打本地值，写 DB 时由 stampBlocks 覆盖为全局值。
func assembleBlocks(text, reasoning string, accums map[int]*toolAccum) []chatdomain.Block {
	var blocks []chatdomain.Block
	seq := 0

	if reasoning != "" {
		d, _ := json.Marshal(chatdomain.TextData{Text: reasoning})
		blocks = append(blocks, chatdomain.Block{
			ID: newBlockID(), Seq: seq, Type: chatdomain.BlockTypeReasoning,
			Data: string(d), CreatedAt: time.Now().UTC(),
		})
		seq++
	}
	if text != "" {
		d, _ := json.Marshal(chatdomain.TextData{Text: text})
		blocks = append(blocks, chatdomain.Block{
			ID: newBlockID(), Seq: seq, Type: chatdomain.BlockTypeText,
			Data: string(d), CreatedAt: time.Now().UTC(),
		})
		seq++
	}

	// Tool calls in deterministic order: by ToolIndex.
	// Tool calls 按确定顺序：按 ToolIndex。
	indices := make([]int, 0, len(accums))
	for i := range accums {
		indices = append(indices, i)
	}
	sortInts(indices)
	for _, i := range indices {
		a := accums[i]
		summary, destructive, args := parseToolArgs(a.args.String())
		td := chatdomain.ToolCallData{
			ID:          a.id,
			Name:        a.name,
			Arguments:   args,
			Summary:     summary,
			Destructive: destructive,
		}
		d, _ := json.Marshal(td)
		blocks = append(blocks, chatdomain.Block{
			ID: newBlockID(), Seq: seq, Type: chatdomain.BlockTypeToolCall,
			Data: string(d), CreatedAt: time.Now().UTC(),
		})
		seq++
	}
	return blocks
}

// joinBlocks concatenates two block slices into a fresh slice (avoids
// mutating either input). Used so streaming snapshots can include earlier
// ReAct-step blocks without aliasing the agentRun-owned accumulator.
//
// joinBlocks 把两段 block 切片拼到新切片（不修改任何输入）。让流式快照能
// 拼上前面 ReAct 步骤的 blocks，又不和 agentRun 维护的累加切片别名共享。
func joinBlocks(a, b []chatdomain.Block) []chatdomain.Block {
	out := make([]chatdomain.Block, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}

// extractToolCalls walks blocks and returns every tool_call's ToolCallData.
// extractToolCalls 遍历 blocks，返回所有 tool_call 的 ToolCallData。
func extractToolCalls(blocks []chatdomain.Block) []chatdomain.ToolCallData {
	var calls []chatdomain.ToolCallData
	for _, b := range blocks {
		if b.Type != chatdomain.BlockTypeToolCall {
			continue
		}
		var tc chatdomain.ToolCallData
		if json.Unmarshal([]byte(b.Data), &tc) == nil {
			calls = append(calls, tc)
		}
	}
	return calls
}

// parseToolArgs extracts summary / destructive from raw JSON args and returns
// the remaining args as a map for assembly into ToolCallData. Delegates to the
// canonical toolapp.StripStandardFields and only adds the chat-side fallback
// of surfacing malformed JSON as args["raw"] — that way the LLM still sees
// what it sent and the tool's ValidateInput can reject with a retry signal.
//
// parseToolArgs 从原始 JSON args 中提取 summary / destructive，把剩余字段
// 装回 map 供 ToolCallData 使用。直接复用 toolapp.StripStandardFields，
// 仅追加 chat 侧的兜底：JSON 损坏时塞 args["raw"]——让 LLM 至少能看到自己发了
// 什么，工具 ValidateInput 据此报错让 LLM 重试。
func parseToolArgs(raw string) (summary string, destructive bool, args map[string]any) {
	if raw == "" {
		return "", false, map[string]any{}
	}
	summary, destructive, stripped := toolapp.StripStandardFields(raw)
	if err := json.Unmarshal([]byte(stripped), &args); err != nil || args == nil {
		return summary, destructive, map[string]any{"raw": raw}
	}
	return summary, destructive, args
}

// sortInts is a tiny in-place ascending int sort (stdlib's sort.Ints adds
// import weight for one call site).
//
// sortInts 是一个就地升序整数排序（用 stdlib sort.Ints 仅一处用就太重）。
func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}
