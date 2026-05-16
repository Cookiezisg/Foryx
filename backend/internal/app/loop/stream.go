package loop

import (
	"context"
	"encoding/json"
	"strings"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

type toolAccum struct {
	id, name string
	args     strings.Builder
}

// streamLLM runs one LLM call, real-time-emitting block lifecycle and returning blocks + tool calls.
//
// streamLLM 跑单次 LLM 调用，实时推 block 生命周期，返内存 blocks + tool calls。
func streamLLM(
	ctx context.Context,
	client llminfra.Client,
	req llminfra.Request,
) (blocks []chatdomain.Block, toolCalls []chatdomain.ToolCallData, stopReason string, errMsg string, inputTokens, outputTokens int) {
	var textBuf, reasonBuf strings.Builder
	accums := map[int]*toolAccum{}
	stopReason = chatdomain.StopReasonEndTurn

	em := eventlogpkg.From(ctx)
	msgID, _ := reqctxpkg.GetMessageID(ctx)
	var (
		textBlockID, reasonBlockID string
	)
	toolBlockIDs := make(map[int]string)

	closeText := func(status string) {
		if textBlockID != "" {
			em.StopBlock(ctx, textBlockID, status, nil)
			textBlockID = ""
		}
	}
	closeReason := func(status string) {
		if reasonBlockID != "" {
			em.StopBlock(ctx, reasonBlockID, status, nil)
			reasonBlockID = ""
		}
	}

	for event := range client.Stream(ctx, req) {
		switch event.Type {
		case llminfra.EventText:
			closeReason(eventlogdomain.StatusCompleted)
			if textBlockID == "" && msgID != "" {
				textBlockID = idgenpkg.New("blk")
				em.EmitBlockStart(ctx, textBlockID, msgID, msgID, eventlogdomain.BlockTypeText, nil)
			}
			if textBlockID != "" {
				em.DeltaBlock(ctx, textBlockID, event.Delta)
			}
			textBuf.WriteString(event.Delta)

		case llminfra.EventReasoning:
			closeText(eventlogdomain.StatusCompleted)
			if reasonBlockID == "" && msgID != "" {
				reasonBlockID = idgenpkg.New("blk")
				em.EmitBlockStart(ctx, reasonBlockID, msgID, msgID, eventlogdomain.BlockTypeReasoning, nil)
			}
			if reasonBlockID != "" {
				em.DeltaBlock(ctx, reasonBlockID, event.Delta)
			}
			reasonBuf.WriteString(event.Delta)

		case llminfra.EventToolStart:
			closeText(eventlogdomain.StatusCompleted)
			closeReason(eventlogdomain.StatusCompleted)
			accums[event.ToolIndex] = &toolAccum{id: event.ToolID, name: event.ToolName}
			if msgID != "" && event.ToolID != "" {
				toolBlockIDs[event.ToolIndex] = event.ToolID
				em.EmitBlockStart(ctx, event.ToolID, msgID, msgID,
					eventlogdomain.BlockTypeToolCall,
					map[string]any{"tool": event.ToolName})
			}

		case llminfra.EventToolDelta:
			if a := accums[event.ToolIndex]; a != nil {
				a.args.WriteString(event.ArgsDelta)
				if id := toolBlockIDs[event.ToolIndex]; id != "" {
					em.DeltaBlock(ctx, id, event.ArgsDelta)
				}
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

	// Promote silent ctx-cancel to StopReasonCancelled before computing closeStatus.
	// Some stream providers exit without an EventError, leaving stopReason=EndTurn
	// while ctx is actually done. If we don't fix it here, dangling blocks would
	// be closed with status=completed (§S21 violation) and the message-level
	// status=cancelled would not match its blocks.
	//
	// 静默 ctx-cancel 提升为 StopReasonCancelled，确保 closeStatus 正确。
	// 部分 stream provider 在 ctx 取消时直接关闭 channel 不发 EventError，
	// 留下 stopReason=EndTurn；不在这里修正，blocks 会被 completed 关闭
	// 而 message 是 cancelled，违反 §S21 invariant。
	if ctx.Err() != nil && stopReason == chatdomain.StopReasonEndTurn {
		stopReason = chatdomain.StopReasonCancelled
	}

	closeStatus := eventlogdomain.StatusCompleted
	switch stopReason {
	case chatdomain.StopReasonCancelled:
		closeStatus = eventlogdomain.StatusCancelled
	case chatdomain.StopReasonError:
		closeStatus = eventlogdomain.StatusError
	}
	closeText(closeStatus)
	closeReason(closeStatus)
	for _, id := range toolBlockIDs {
		em.StopBlock(ctx, id, closeStatus, nil)
	}

	blocks = assembleBlocks(textBuf.String(), reasonBuf.String(), accums)
	toolCalls = collectToolCalls(accums)
	return
}

// assembleBlocks builds the in-memory Block slice for history conversion (not persisted here).
//
// assembleBlocks 组装内存 Block 列表给 history 转换（不在此处落库）。
func assembleBlocks(text, reasoning string, accums map[int]*toolAccum) []chatdomain.Block {
	var blocks []chatdomain.Block

	if reasoning != "" {
		blocks = append(blocks, chatdomain.Block{
			Type:    eventlogdomain.BlockTypeReasoning,
			Content: reasoning,
		})
	}
	if text != "" {
		blocks = append(blocks, chatdomain.Block{
			Type:    eventlogdomain.BlockTypeText,
			Content: text,
		})
	}

	indices := make([]int, 0, len(accums))
	for i := range accums {
		indices = append(indices, i)
	}
	sortInts(indices)
	for _, i := range indices {
		a := accums[i]
		_, args := parseToolArgs(a.args.String())
		argsJSON, _ := json.Marshal(args)
		blocks = append(blocks, chatdomain.Block{
			ID:      a.id,
			Type:    eventlogdomain.BlockTypeToolCall,
			Content: string(argsJSON),
			Attrs:   map[string]any{"tool": a.name},
		})
	}
	return blocks
}


// collectToolCalls extracts ToolCallData straight from accumulators, ordered by LLM ToolIndex.
//
// collectToolCalls 直接从累加器取 ToolCallData，按 LLM ToolIndex 升序排列。
func collectToolCalls(accums map[int]*toolAccum) []chatdomain.ToolCallData {
	indices := make([]int, 0, len(accums))
	for i := range accums {
		indices = append(indices, i)
	}
	sortInts(indices)
	calls := make([]chatdomain.ToolCallData, 0, len(accums))
	for _, i := range indices {
		a := accums[i]
		fields, args := parseToolArgs(a.args.String())
		calls = append(calls, chatdomain.ToolCallData{
			ID:             a.id,
			Name:           a.name,
			Arguments:      args,
			Summary:        fields.Summary,
			Destructive:    fields.Destructive,
			ExecutionGroup: fields.ExecutionGroup,
		})
	}
	return calls
}

// parseToolArgs strips the 3 standard fields, surfacing malformed JSON as args["raw"].
//
// parseToolArgs 剥 3 个标准字段；JSON 坏时原文塞 args["raw"]。
func parseToolArgs(raw string) (toolapp.StandardFields, map[string]any) {
	if raw == "" {
		return toolapp.StandardFields{}, map[string]any{}
	}
	fields, stripped := toolapp.StripStandardFields(raw)
	var args map[string]any
	if err := json.Unmarshal([]byte(stripped), &args); err != nil || args == nil {
		return fields, map[string]any{"raw": raw}
	}
	return fields, args
}

func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}
