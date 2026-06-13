package loop

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"go.uber.org/zap"

	entitystreamapp "github.com/sunweilin/forgify/backend/internal/app/entitystream"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Node content shapes for the messages stream — the loop's slice of the vocabulary. open
// frames carry minimal metadata; the durable close frame carries the full snapshot so a
// reconnect that missed the lossy (ephemeral) deltas can rebuild the node. Front-end
// contract: see contract-changes / domains/messages.
//
// messages 流的 Node content 形状——loop 那一份词表。open 帧带最小元数据；durable 的 close
// 帧带完整快照，使错过可丢（ephemeral）delta 的重连能重建节点。前端契约见 contract-changes /
// domains/messages。
type (
	textContent struct {
		Content string `json:"content"`
	}
	reasoningContent struct {
		Content   string `json:"content"`
		Signature string `json:"signature,omitempty"`
	}
	toolCallContent struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments,omitempty"`
		// Summary / Danger are the LLM's self-reported intent + risk for THIS call. They
		// ride the tool_call node so the front end can show the one-liner and flag a
		// cautious / dangerous call — the visible half of "pure trust" danger handling.
		//
		// Summary / Danger 是 LLM 对本次调用自报的意图 + 风险。随 tool_call 节点上行，使前端
		// 能显示一句话摘要并标记 cautious / dangerous 调用——「纯信任」danger 处理的可见半边。
		Summary string `json:"summary,omitempty"`
		Danger  string `json:"danger,omitempty"`
	}
)

type toolAccum struct {
	id, name string
	args     strings.Builder
	// forge (non-nil for a ForgeTool call) mirrors this tool_call's arg delta onto the entities
	// stream so the entity panel fills in live (SSE-C). nil for non-forge calls.
	//
	// forge（ForgeTool 调用时非 nil）把本 tool_call 的 arg delta 镜像到 entities 流，使实体面板实时填充
	// （SSE-C）。非 forge 调用为 nil。
	forge *entitystreamapp.Writer
}

// forgeOpenContent is the entities-stream forge node's open-frame content. (Node type =
// entitystream.NodeForge; delta = raw arg chunks; close result = the final args.)
//
// forgeOpenContent 是 entities 流 forge 节点的 open 帧内容。（节点型 = entitystream.NodeForge；delta =
// 裸 arg chunk；close 结果 = 最终 args。）
type forgeOpenContent struct {
	Op string `json:"op"` // "create" | "edit"
}

// reasonAccum collects reasoning content and its accompanying Anthropic signature.
//
// reasonAccum 收集 reasoning content 及其 Anthropic 签名。
type reasonAccum struct {
	buf       strings.Builder
	signature string
}

// streamLLM runs one LLM call, live-pushing block lifecycle to the messages stream and
// returning the in-memory blocks + parsed tool calls. Blocks accumulate regardless of
// whether the stream is wired (the emitter no-ops when disabled), so a non-streaming run
// still produces a faithful history.
//
// streamLLM 跑单次 LLM 调用，实时推 block 生命周期到 messages 流，返内存 blocks + 解析的
// tool calls。无论流是否接线 block 都累加（emitter 禁用时 no-op），故非流式运行仍产出忠实历史。
func streamLLM(
	ctx context.Context,
	client llminfra.Client,
	req llminfra.Request,
	forgeOf func(toolName string) (toolapp.ForgeSpec, bool),
	log *zap.Logger,
) (blocks []messagesdomain.Block, toolCalls []messagesdomain.ToolCallData, stopReason, errMsg string, inputTokens, outputTokens int) {
	em := newEmitter(ctx, log)
	msgID, _ := reqctxpkg.GetMessageID(ctx)
	// entBridge (nil off a streamed chat turn) carries forge tool_call arg deltas onto the entities
	// stream, scoped to a forge session, so the entity panel fills in live.
	//
	// entBridge（不在流式 chat turn 则 nil）把 forge tool_call 的 arg delta 送上 entities 流、锚到一次 forge
	// 会话，使实体面板实时填充。
	entBridge := entitystreamapp.BridgeFrom(ctx)

	var textBuf strings.Builder
	var reason reasonAccum
	accums := map[int]*toolAccum{}
	stopReason = messagesdomain.StopReasonEndTurn

	var textBlockID, reasonBlockID string

	closeText := func(status string) {
		if textBlockID != "" {
			em.close(ctx, textBlockID, status, textSnapshot(textBuf.String()), "")
			textBlockID = ""
		}
	}
	closeReason := func(status string) {
		if reasonBlockID != "" {
			em.close(ctx, reasonBlockID, status, reasonSnapshot(reason), "")
			reasonBlockID = ""
		}
	}

	for event := range client.Stream(ctx, req) {
		switch event.Type {
		case llminfra.EventText:
			closeReason(messagesdomain.StatusCompleted)
			if textBlockID == "" {
				textBlockID = idgenpkg.New("blk")
				em.open(ctx, textBlockID, msgID, messagesdomain.BlockTypeText, nil)
			}
			em.delta(ctx, textBlockID, event.Delta)
			textBuf.WriteString(event.Delta)

		case llminfra.EventReasoning:
			closeText(messagesdomain.StatusCompleted)
			if event.Delta != "" {
				if reasonBlockID == "" {
					reasonBlockID = idgenpkg.New("blk")
					em.open(ctx, reasonBlockID, msgID, messagesdomain.BlockTypeReasoning, nil)
				}
				em.delta(ctx, reasonBlockID, event.Delta)
				reason.buf.WriteString(event.Delta)
			}
			// Signature arrives as a zero-Delta EventReasoning; capture it for the snapshot.
			// Signature 随 Delta 为空的 EventReasoning 到达，捕获供快照用。
			if event.Signature != "" {
				reason.signature = event.Signature
			}

		case llminfra.EventToolStart:
			closeText(messagesdomain.StatusCompleted)
			closeReason(messagesdomain.StatusCompleted)
			// The tool-call/block id is SERVER-minted, never the provider's call id: providers
			// recycle ids across steps and turns ("call_0"/"call_1" every response — index-style
			// providers do this), and message_blocks.id is a table-wide PK — reusing the wire id
			// loses ENTIRE turns to UNIQUE conflicts at finalize. The provider id is only an
			// in-response correlation handle (accums key by ToolIndex already); history round-trips
			// pair assistant tool_calls with tool results by THIS id, which providers accept.
			//
			// tool_call/块 id 一律服务端铸造、绝不用 provider 的 call id：provider 会跨步跨回合复用 id
			// （index 风格的家常发 "call_0"/"call_1"），而 message_blocks.id 是全表 PK——沿用线缆 id 会在
			// finalize 撞 UNIQUE、整回合丢失。provider id 只是响应内关联句柄（accums 本就按 ToolIndex
			// 键控）；历史回喂用本 id 配对 assistant tool_calls 与 tool 结果，provider 照单全收。
			a := &toolAccum{id: idgenpkg.New("blk"), name: event.ToolName}
			accums[event.ToolIndex] = a
			em.open(ctx, a.id, msgID, messagesdomain.BlockTypeToolCall,
				streamdomain.JSONContent(toolCallContent{Name: event.ToolName}))
			// SSE-C: a forge tool_call's args ARE an entity's content being written — mirror the
			// delta onto the entities stream (scope = a forge session keyed by the tool_call id;
			// the front end correlates to the entity via the streamed args + the tool_result).
			//
			// SSE-C：forge tool_call 的 args 本身就是某实体正被写出的内容——把 delta 镜像到 entities 流
			// （scope = 以 tool_call id 为键的 forge 会话；前端经流式 args + tool_result 关联到实体）。
			if spec, ok := forgeOf(event.ToolName); ok {
				a.forge = entitystreamapp.New(ctx, entBridge,
					streamdomain.Scope{Kind: spec.Kind, ID: a.id},
					entitystreamapp.NodeForge, streamdomain.JSONContent(forgeOpenContent{Op: spec.Op}))
			}

		case llminfra.EventToolDelta:
			if a := accums[event.ToolIndex]; a != nil {
				a.args.WriteString(event.ArgsDelta)
				if a.id != "" {
					em.delta(ctx, a.id, event.ArgsDelta)
				}
				if a.forge != nil {
					_, _ = a.forge.Write([]byte(event.ArgsDelta))
				}
			}

		case llminfra.EventFinish:
			if event.FinishReason == "length" {
				stopReason = messagesdomain.StopReasonMaxTokens
			}
			if event.InputTokens > 0 {
				inputTokens = event.InputTokens
			}
			if event.OutputTokens > 0 {
				outputTokens = event.OutputTokens
			}

		case llminfra.EventError:
			if ctx.Err() != nil {
				stopReason = messagesdomain.StopReasonCancelled
			} else {
				stopReason = messagesdomain.StopReasonError
				if event.Err != nil {
					errMsg = event.Err.Error()
				}
			}
		}
	}

	// Promote a silent ctx-cancel to StopReasonCancelled before computing closeStatus: some
	// providers close the stream without an EventError, leaving stopReason=EndTurn while ctx
	// is done — which would close dangling blocks as completed (a status mismatch with the
	// cancelled message).
	//
	// 静默 ctx-cancel 提升为 Cancelled，再算 closeStatus：部分 provider 在 ctx 取消时直接关流
	// 不发 EventError，留 stopReason=EndTurn——会把悬挂 block 以 completed 关闭（与 cancelled
	// 消息状态错配）。
	if ctx.Err() != nil && stopReason == messagesdomain.StopReasonEndTurn {
		stopReason = messagesdomain.StopReasonCancelled
	}

	closeStatus := messagesdomain.StatusCompleted
	switch stopReason {
	case messagesdomain.StopReasonCancelled:
		closeStatus = messagesdomain.StatusCancelled
	case messagesdomain.StopReasonError:
		closeStatus = messagesdomain.StatusError
	}
	closeText(closeStatus)
	closeReason(closeStatus)
	for _, a := range accums {
		if a.id != "" {
			em.close(ctx, a.id, closeStatus, toolCallSnapshot(a), "")
		}
		if a.forge != nil {
			// close the entities forge node with the final args as the reconnect snapshot.
			//
			// 用最终 args 作重连快照关 entities forge 节点。
			a.forge.Close(closeStatus, json.RawMessage(a.args.String()))
		}
	}

	blocks = assembleBlocks(textBuf.String(), reason, accums)
	toolCalls = collectToolCalls(accums)
	return
}

func textSnapshot(s string) *streamdomain.Node {
	return &streamdomain.Node{Type: messagesdomain.BlockTypeText, Content: streamdomain.JSONContent(textContent{Content: s})}
}

func reasonSnapshot(r reasonAccum) *streamdomain.Node {
	return &streamdomain.Node{
		Type:    messagesdomain.BlockTypeReasoning,
		Content: streamdomain.JSONContent(reasoningContent{Content: r.buf.String(), Signature: r.signature}),
	}
}

func toolCallSnapshot(a *toolAccum) *streamdomain.Node {
	fields, args := parseToolArgs(a.args.String())
	argsJSON, _ := json.Marshal(args)
	return &streamdomain.Node{
		Type: messagesdomain.BlockTypeToolCall,
		Content: streamdomain.JSONContent(toolCallContent{
			Name:      a.name,
			Arguments: string(argsJSON),
			Summary:   fields.Summary,
			Danger:    string(fields.Danger),
		}),
	}
}

// assembleBlocks builds the in-memory Block slice for history conversion (not persisted here).
//
// assembleBlocks 组装内存 Block 列表给 history 转换（不在此处落库）。
func assembleBlocks(text string, reason reasonAccum, accums map[int]*toolAccum) []messagesdomain.Block {
	var blocks []messagesdomain.Block

	if reasoning := reason.buf.String(); reasoning != "" {
		var attrs map[string]any
		if reason.signature != "" {
			attrs = map[string]any{"signature": reason.signature}
		}
		blocks = append(blocks, messagesdomain.Block{
			Type:    messagesdomain.BlockTypeReasoning,
			Content: reasoning,
			Attrs:   attrs,
		})
	}
	if text != "" {
		blocks = append(blocks, messagesdomain.Block{
			Type:    messagesdomain.BlockTypeText,
			Content: text,
		})
	}

	for _, i := range sortedAccumKeys(accums) {
		a := accums[i]
		fields, args := parseToolArgs(a.args.String())
		argsJSON, _ := json.Marshal(args)
		// tool / summary / danger persist on the block so a DB-rebuilt history (after replay
		// eviction) keeps the call's name and self-reported risk, matching the live snapshot.
		//
		// tool / summary / danger 落在 block 上，使 DB 重建的历史（replay 淘汰后）保留调用名与
		// 自报风险，与 live 快照一致。
		attrs := map[string]any{"tool": a.name}
		if fields.Summary != "" {
			attrs["summary"] = fields.Summary
		}
		if fields.Danger != "" {
			attrs["danger"] = string(fields.Danger)
		}
		blocks = append(blocks, messagesdomain.Block{
			ID:      a.id,
			Type:    messagesdomain.BlockTypeToolCall,
			Content: string(argsJSON),
			Attrs:   attrs,
		})
	}
	return blocks
}

// collectToolCalls extracts ToolCallData from accumulators, ordered by LLM ToolIndex. The
// self-reported Danger (tool.DangerLevel) is stored as a plain string — the app-layer
// crossing that keeps messages domain free of app/tool.
//
// collectToolCalls 从累加器取 ToolCallData，按 LLM ToolIndex 升序。自报的 Danger
// （tool.DangerLevel）存为纯字符串——使 messages domain 不沾 app/tool 的 app 层转换点。
func collectToolCalls(accums map[int]*toolAccum) []messagesdomain.ToolCallData {
	var calls []messagesdomain.ToolCallData
	for _, i := range sortedAccumKeys(accums) {
		a := accums[i]
		fields, args := parseToolArgs(a.args.String())
		calls = append(calls, messagesdomain.ToolCallData{
			ID:             a.id,
			Name:           a.name,
			Arguments:      args,
			Summary:        fields.Summary,
			Danger:         string(fields.Danger),
			ExecutionGroup: fields.ExecutionGroup,
		})
	}
	return calls
}

// parseToolArgs strips the 3 standard fields, surfacing malformed JSON as args["raw"] so a
// botched call still reaches the tool (which reports the real validation error).
//
// parseToolArgs 剥 3 个标准字段；JSON 坏时原文塞 args["raw"]，使畸形调用仍抵达工具（由工具报真校验错）。
func parseToolArgs(raw string) (toolapp.StandardFields, map[string]any) {
	if raw == "" {
		return toolapp.StandardFields{Danger: toolapp.DangerSafe}, map[string]any{}
	}
	fields, stripped := toolapp.StripStandardFields(raw)
	var args map[string]any
	if err := json.Unmarshal([]byte(stripped), &args); err != nil || args == nil {
		return fields, map[string]any{"raw": raw}
	}
	return fields, args
}

func sortedAccumKeys(m map[int]*toolAccum) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}
