// stream.go — One LLM call: consume stream events, emit
// block_start/delta/stop on the event-log Bridge, assemble in-memory
// Blocks for in-loop history conversion. Block rows persist real-time
// inside the Emitter (pkg/eventlog); loop.Run only writes the final
// messages row via host.WriteFinalize.
//
// stream.go — 单次 LLM 调用：消费流事件、给事件日志 Bridge 发
// block_start/delta/stop、组装内存 Block 给循环内 history 转换。Block 行
// 由 Emitter（pkg/eventlog）实时写；loop.Run 只经 host.WriteFinalize 写
// 终态 messages 行。
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

// toolAccum accumulates streaming fragments for one tool call.
// toolAccum 累积单个 tool call 的流式片段。
type toolAccum struct {
	id, name string
	args     strings.Builder
}

// streamLLM executes one LLM call. Per-event emit fires real-time
// block_start / block_delta / block_stop on the eventlog Bridge — no
// snapshot publish path; UI sees deltas as they arrive. text + reasoning
// blocks mint fresh blk_<id>; tool_call blocks reuse the LLM's tool-call
// ID as the block ID (per event-log-protocol.md §3). On stream end /
// transition all open blocks get closed with appropriate status. Returns
// the in-memory block list for in-loop history extension and tool calls
// for runTools dispatch.
//
// streamLLM 执行单次 LLM 调用。每事件实时给事件日志 Bridge 推
// block_start / block_delta / block_stop——无快照路径；UI 边到边看 delta。
// text + reasoning 铸新 blk_<id>；tool_call 复用 LLM 的 tool-call ID 作
// block ID（详 event-log-protocol.md §3）。流结束 / 切换时关闭所有 open
// block。返内存 block 列表给循环内 history 扩展、tool calls 给 runTools
// 派发。
func streamLLM(
	ctx context.Context,
	client llminfra.Client,
	req llminfra.Request,
) (blocks []chatdomain.Block, toolCalls []chatdomain.ToolCallData, stopReason string, errMsg string, inputTokens, outputTokens int) {
	var textBuf, reasonBuf strings.Builder
	accums := map[int]*toolAccum{}
	stopReason = chatdomain.StopReasonEndTurn

	// Event-log emit state. Block IDs persist across stream events so
	// successive deltas reference the same block. Real-time emit is
	// the only push path (no legacy snapshot publish).
	//
	// 事件日志 emit 状态。Block ID 跨流事件持续，让连续 delta 引同一 block。
	// 实时 emit 是唯一推送路径（无 legacy 快照 publish）。
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
			// Transition out of reasoning if it was open.
			// 转出 reasoning（若开着）。
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
			// Tool start is a low-frequency milestone (one per tool call,
			// not per token) — push immediately so the UI can render the
			// "running…" pill without waiting up to 16ms.
			//
			// tool_start 是低频里程碑（每 tool 调用一次，非每 token），
			// 立即推，UI "running…" 无需等 16ms。
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

	// Close any still-open event-log blocks before returning. The status
	// follows the stream's stopReason: cancelled → cancelled, error →
	// error, otherwise → completed (which covers normal end_turn / tool
	// transitions where the LLM finished delivering args).
	//
	// 返前关掉所有仍 open 的事件日志 block。状态跟随流 stopReason：取消 →
	// cancelled，error → error，其他 → completed（覆盖正常 end_turn / tool
	// 切换 LLM 完成 args 派发的情况）。
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

	if ctx.Err() != nil && stopReason == chatdomain.StopReasonEndTurn {
		stopReason = chatdomain.StopReasonCancelled
	}

	blocks = assembleBlocks(textBuf.String(), reasonBuf.String(), accums)
	toolCalls = collectToolCalls(accums)
	return
}

// assembleBlocks builds the in-memory Block slice for in-loop history
// conversion (BlocksToAssistantLLM) and the loop.Result.Blocks return.
// Order: reasoning → text → tool_calls (by ToolIndex).
//
// These blocks are NOT persisted from here — emit (in stream loop) is
// the sole DB write path. assembleBlocks fills only the fields
// BlocksToAssistantLLM consumes: Type + Content for text/reasoning;
// ID + Type + Content + Attrs for tool_call (ID needed because the LLM
// tool-call ID flows through here to extendHistory).
//
// assembleBlocks 组装内存 Block 列表给循环内 history 转换
// （BlocksToAssistantLLM）和 loop.Result.Blocks 返回。顺序：reasoning →
// text → tool_calls（按 ToolIndex）。
//
// 这些 block 不在此处持久化——emit（在 stream 循环里）是唯一 DB 写入
// 路径。assembleBlocks 只填 BlocksToAssistantLLM 真正会读的字段：
// text/reasoning 用 Type + Content；tool_call 用 ID + Type + Content +
// Attrs（ID 必填——LLM tool-call ID 经此传给 extendHistory）。
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
		// args is JSON-marshaled into Block.Content (a string column). attrs
		// goes directly as map (GORM serializer:json handles column store).
		// 2026-05: Attrs 改 map[string]any,无需再外层 Marshal。
		argsJSON, _ := json.Marshal(args)
		blocks = append(blocks, chatdomain.Block{
			ID:      a.id, // LLM tc_id reused as block id
			Type:    eventlogdomain.BlockTypeToolCall,
			Content: string(argsJSON),
			Attrs:   map[string]any{"tool": a.name},
		})
	}
	return blocks
}


// collectToolCalls returns ToolCallData parsed directly from the
// streaming accumulators (no Block intermediary). Order matches
// LLM ToolIndex (sorted ascending).
//
// collectToolCalls 直接从流式累加器返回 ToolCallData（不经 Block）。
// 顺序按 LLM ToolIndex（升序）。
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

// parseToolArgs strips the three standard fields from raw JSON args via the
// canonical toolapp.StripStandardFields, surfacing malformed JSON as
// args["raw"] so the LLM can still see what it sent.
//
// parseToolArgs 用 toolapp.StripStandardFields 剥三个标准字段；JSON 损坏时
// 把原文塞 args["raw"] 让 LLM 仍能看到自己发了什么。
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

// sortInts is a tiny in-place ascending int sort.
// sortInts 是一个就地升序整数排序。
func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}
