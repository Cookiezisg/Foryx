package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// toolResultContent is the tool_result node payload (the loop's slice of the messages
// vocabulary). A tool_result streams nothing — it is produced whole — so its content rides
// the open frame and the close carries only status/error.
//
// toolResultContent 是 tool_result 节点 payload（loop 那一份 messages 词表）。tool_result 无流式
// （一次性产出），故内容随 open 帧、close 只带 status/error。
type toolResultContent struct {
	Content string `json:"content,omitempty"`
}

// runTools executes calls in execution-group batches and returns tool_result blocks aligned
// to the input order. Same-group calls run concurrently (one WaitGroup per batch); groups
// run in ascending order. The result slice is index-aligned so a parallel batch's writes
// don't race on order.
//
// runTools 按 execution-group 分批执行 tool 调用，返回与输入同序的 tool_result block。同组并行
// （每批一个 WaitGroup）、组间按升序串行。结果切片按下标对齐，使并行批的写入不竞争顺序。
func runTools(
	ctx context.Context,
	calls []messagesdomain.ToolCallData,
	byName map[string]toolapp.Tool,
	log *zap.Logger,
) []messagesdomain.Block {
	if len(calls) == 0 {
		return nil
	}
	// Per-call block lists (progress* + tool_result), index-aligned so a parallel batch's writes
	// don't race on order; flattened in call order at the end.
	//
	// 每调用一组 block（progress* + tool_result），按下标对齐使并行批写入不竞争顺序；末尾按调用序拍平。
	perCall := make([][]messagesdomain.Block, len(calls))

	for _, batch := range partitionByExecutionGroup(calls) {
		if len(batch.items) == 1 {
			item := batch.items[0]
			perCall[item.idx] = runOneTool(ctx, byName[item.tc.Name], item.tc, log)
			continue
		}
		var wg sync.WaitGroup
		for _, item := range batch.items {
			wg.Add(1)
			go func(it indexedCall) {
				defer wg.Done()
				// Each goroutine writes its own pre-assigned index — no shared-slot race, no lock.
				//
				// 每个 goroutine 只写自己预分配的下标——无共享槽竞争、无需锁。
				perCall[it.idx] = runOneTool(ctx, byName[it.tc.Name], it.tc, log)
			}(item)
		}
		wg.Wait()
	}
	var blocks []messagesdomain.Block
	for _, bs := range perCall {
		blocks = append(blocks, bs...)
	}
	return blocks
}

// runOneTool executes one tool call and returns its tool_result block, live-pushing the
// block lifecycle. The danger level the LLM self-reported rode the tool_call node already
// (pure trust, M2.2: no gate here); a future approval pause for dangerous calls hooks in at
// the loop level once the ask channel exists (波次 6).
//
// runOneTool 执行一次 tool 调用、返回其 tool_result block，并实时推 block 生命周期。LLM 自报的
// danger 已随 tool_call 节点上行（纯信任，M2.2：此处无门控）；将来 dangerous 调用的确认暂停在
// loop 层接入（待 ask 通道就绪，波次 6）。
func runOneTool(ctx context.Context, t toolapp.Tool, tc messagesdomain.ToolCallData, log *zap.Logger) []messagesdomain.Block {
	argsJSON, _ := json.Marshal(tc.Arguments)
	// Seed this call's id so a tool can learn its own tool_call block id (the Subagent tool
	// anchors the subagent's message subtree under it, E3) and ToolProgress nests its progress
	// block under it. The capture lets a tool's live progress (bash output, env-fix log, …)
	// persist with the turn alongside the tool_result.
	//
	// 埋本次调用的 id，使工具能得知自己的 tool_call block id（Subagent 据此把 subagent message 子树锚其下，
	// E3；ToolProgress 据此把 progress 块嵌其下）。capture 使工具的实时进度（bash 输出、env-fix log…）随回合
	// 与 tool_result 一并持久化。
	ctx = reqctxpkg.SetToolCallID(ctx, tc.ID)
	pcap := &progressCapture{}
	ctx = withProgressCapture(ctx, pcap)
	output, errMsg, ok := executeTool(ctx, t, tc.Name, argsJSON, log)

	status := messagesdomain.StatusCompleted
	if !ok {
		status = messagesdomain.StatusError
	}

	em := newEmitter(ctx, log)
	blockID := idgenpkg.New("blk")
	em.open(ctx, blockID, tc.ID, messagesdomain.BlockTypeToolResult, streamdomain.JSONContent(toolResultContent{Content: output}))
	em.close(ctx, blockID, status, nil, errMsg)

	errVal := ""
	if !ok {
		errVal = errMsg
	}
	result := messagesdomain.Block{
		ID:            blockID,
		Type:          messagesdomain.BlockTypeToolResult,
		Content:       output,
		ParentBlockID: tc.ID,
		Error:         errVal,
		Attrs:         map[string]any{"tool": tc.Name},
	}
	// Progress blocks (emitted during Execute) precede the tool_result — chronological + correct
	// sibling order under the tool_call. Usually empty (most tools emit no progress).
	//
	// progress 块（Execute 期间发的）排在 tool_result 前——时序 + tool_call 下正确的兄弟序。通常为空
	// （多数工具不发进度）。
	return append(pcap.take(), result)
}

// executeTool runs ValidateInput then Execute and shapes the (output, errMsg, ok) tuple.
// There is no permission gate (M1.9 dissolved central gating) and no error rewriting: a
// tool owns the quality of its own error message (clean text, any next-step hint), so loop
// stays a neutral engine and just surfaces err.Error() to the LLM.
//
// executeTool 跑 ValidateInput 再 Execute，整形 (output, errMsg, ok) 三元组。无权限门控
// （M1.9 解散中央门控）、无错误改写：工具自负其 error message 质量（干净文本、必要的 next-step
// 提示），故 loop 保持中立引擎、只把 err.Error() 透传给 LLM。
func executeTool(ctx context.Context, t toolapp.Tool, name string, argsJSON []byte, log *zap.Logger) (output, errMsg string, ok bool) {
	if t == nil {
		// The LLM named a tool not in this turn's set — a wiring bug or a stale catalog.
		// LLM 点了本回合工具集外的工具——接线 bug 或过期 catalog。
		log.Warn("executeTool: tool not in registry — likely wiring bug or stale catalog", zap.String("tool", name))
		msg := fmt.Sprintf("tool %q not found", name)
		return msg, msg, false
	}

	if err := t.ValidateInput(argsJSON); err != nil {
		log.Warn("tool validate failed", zap.String("tool", name), zap.Error(err))
		return "input validation failed: " + err.Error(), err.Error(), false
	}

	output, err := t.Execute(ctx, string(argsJSON))
	if err != nil {
		log.Warn("tool execute failed", zap.String("tool", name), zap.Error(err))
		if output != "" {
			return output + "\n\n" + err.Error(), err.Error(), false
		}
		return err.Error(), err.Error(), false
	}
	return output, "", true
}

type indexedCall struct {
	idx int
	tc  messagesdomain.ToolCallData
}

type executionBatch struct {
	items []indexedCall
}

// autoGroupBase is where auto-assigned groups (calls with ExecutionGroup ≤ 0) start, kept
// above any plausible explicit group so they always sort after explicitly-grouped batches.
//
// autoGroupBase 是自动分组（ExecutionGroup ≤ 0 的调用）的起点，置于任何合理显式组之上，使其
// 总排在显式分组批之后。
const autoGroupBase = 1000

// partitionByExecutionGroup buckets calls by ExecutionGroup; ≤0 get sequential auto-groups
// (each its own batch) placed after the explicit ones. Same explicit group → one batch →
// concurrent; distinct groups → separate batches → ordered.
//
// partitionByExecutionGroup 按 ExecutionGroup 分桶；≤0 获顺序自动组（各自一批）排在显式组之后。
// 同一显式组 → 一批 → 并行；不同组 → 分批 → 有序。
func partitionByExecutionGroup(calls []messagesdomain.ToolCallData) []executionBatch {
	if len(calls) == 0 {
		return nil
	}

	maxExplicit := 0
	for _, tc := range calls {
		maxExplicit = max(maxExplicit, tc.ExecutionGroup)
	}
	nextAuto := max(maxExplicit+1, autoGroupBase)

	buckets := map[int][]indexedCall{}
	var groupNums []int
	for i, tc := range calls {
		g := tc.ExecutionGroup
		if g <= 0 {
			g = nextAuto
			nextAuto++
		}
		if _, ok := buckets[g]; !ok {
			groupNums = append(groupNums, g)
		}
		buckets[g] = append(buckets[g], indexedCall{idx: i, tc: tc})
	}

	sort.Ints(groupNums)
	out := make([]executionBatch, 0, len(groupNums))
	for _, g := range groupNums {
		out = append(out, executionBatch{items: buckets[g]})
	}
	return out
}
