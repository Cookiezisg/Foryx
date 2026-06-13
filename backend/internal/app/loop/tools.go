package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"go.uber.org/zap"

	humanloopapp "github.com/sunweilin/forgify/backend/internal/app/humanloop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	limitspkg "github.com/sunweilin/forgify/backend/internal/pkg/limits"
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

// runOneTool executes one tool call and returns its tool_result block, live-pushing the block
// lifecycle. A self-reported-dangerous call is gated for human approval first when a humanloop
// broker is in ctx (chat / nested agent); otherwise pure trust (M2.2) — the danger level rode the
// tool_call node and the call just runs.
//
// runOneTool 执行一次 tool 调用、返回其 tool_result block，并实时推 block 生命周期。当 ctx 里有 humanloop
// broker 时（chat / 嵌套 agent），自报 dangerous 的调用先门控到人批准；否则纯信任（M2.2）——danger 已随
// tool_call 节点上行、调用直接跑。
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
	output, errMsg, ok := dispatchWithGate(ctx, t, tc, argsJSON, log)

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

// dispatchWithGate runs the tool, gating a self-reported-dangerous call on human approval first
// when a humanloop broker is in ctx (chat / nested agent runs seed one; subagent / workflow do not
// → pure trust). It is interrupt-before-side-effect: a denied tool never executes — the denial is
// recorded as the result so the model re-routes; a cancelled ctx (the run aborted) records that.
// approve / approve_always fall through and execute (approve_always also session-whitelists, so the
// next dangerous call to this tool in this conversation skips the gate). The active skill's
// allowed-tools also pre-approve (R0040: a skill declares the tools it expects, so a dangerous call
// it intends skips the per-call confirmation) — see skillPreApproves.
//
// dispatchWithGate 跑工具，但当 ctx 里有 humanloop broker 时（chat / 嵌套 agent 运行 seed 之；subagent /
// workflow 不 → 纯信任），先把自报 dangerous 的调用门控到人批准。interrupt-before-side-effect：被拒的工具绝不
// 执行——拒绝记为结果使模型改道；ctx 取消（运行中止）记下之。approve / approve_always 落下去执行（approve_always
// 还会话白名单，使本对话下次对该工具的危险调用跳过门）。active skill 的 allowed-tools 同样预授权（R0040：skill
// 声明它期待的工具，故它有意的危险调用跳过逐次确认）——见 skillPreApproves。
func dispatchWithGate(ctx context.Context, t toolapp.Tool, tc messagesdomain.ToolCallData, argsJSON []byte, log *zap.Logger) (output, errMsg string, ok bool) {
	if b := humanloopapp.From(ctx); b != nil && tc.Danger == string(toolapp.DangerDangerous) {
		convID, _ := reqctxpkg.GetConversationID(ctx)
		if !b.IsAllowed(convID, tc.Name) && !skillPreApproves(ctx, tc.Name) {
			prompt, _ := json.Marshal(map[string]any{"summary": tc.Summary, "args": json.RawMessage(argsJSON)})
			resp, err := b.Request(ctx, humanloopapp.Request{
				ToolCallID:     tc.ID,
				Kind:           humanloopapp.KindDanger,
				Tool:           tc.Name,
				ConversationID: convID,
				Prompt:         prompt,
			})
			if err != nil { // ctx cancelled — the run is aborting
				return "The run was cancelled before this tool ran.", "", true
			}
			// Fail-safe: only an explicit approve runs the tool; deny / an unexpected action does NOT
			// (a malformed resolve must never execute a dangerous call).
			//
			// fail-safe：只有显式 approve 才跑工具；deny / 意外动作都不跑（畸形 resolve 绝不能执行危险调用）。
			if resp.Action != humanloopapp.DecisionApprove && resp.Action != humanloopapp.DecisionApproveAlways {
				return humanloopapp.DenyFeedback, "", true
			}
			// approve / approve_always → fall through and execute
		}
	}
	return executeTool(ctx, t, tc.Name, argsJSON, log)
}

// skillPreApproves reports whether the run's active skill declared this tool in its allowed-tools.
// A skill's allowed-tools are a PRE-AUTHORIZATION (R0040), not a restriction: a dangerous call the
// active skill expects skips the per-call confirmation. No agent state / no active skill → false
// (the gate stands). The active skill is recorded by skill activation (skill/activate.go).
//
// skillPreApproves 报告本次运行的 active skill 是否在其 allowed-tools 里声明了该工具。skill 的 allowed-tools
// 是**预授权**（R0040）、非限制：active skill 期待的危险调用跳过逐次确认。无 agent state / 无 active skill →
// false（门照常）。active skill 由 skill 激活记录（skill/activate.go）。
func skillPreApproves(ctx context.Context, toolName string) bool {
	st, ok := reqctxpkg.GetAgentState(ctx)
	return ok && st.IsToolPreApprovedBySkill(toolName)
}

// maxToolResultBytes() hard-caps any single tool_result. The result is persisted whole, rides a
// durable SSE open frame whole, and feeds the SAME turn's next LLM step whole (warm/cold
// projection only trims LATER turns) — so one unbounded result (a head_limit-less Grep over a
// big tree, a chatty MCP tool) would blow the provider request, the DB row, and the frontend
// frame all at once. 256 KiB matches the Bash tool's own cap.
//
// maxToolResultBytes() 硬限单个 tool_result。结果会整段落库、整段上 durable SSE open 帧、并整段进入
// **同一回合**下一步的 LLM 请求（warm/cold 投影只裁后续回合）——一个无界结果（不带 head_limit 的
// 大树 Grep、话痨 MCP 工具）会同时打爆 provider 请求、DB 行与前端帧。256 KiB 对齐 Bash 自身的 cap。

// maxToolResultBytes reads the live tool_result cap (limits.Tools.ToolResultCapKB).
//
// maxToolResultBytes 读活动 tool_result 上限（limits.Tools.ToolResultCapKB）。
func maxToolResultBytes() int { return limitspkg.Current().Tools.ToolResultCapKB << 10 }

// capToolResult truncates an oversized result (keeping the head — for search-style output the
// first matches are the useful ones) and tells the LLM how to narrow.
//
// capToolResult 截断超限结果（保头部——搜索类输出前面的命中才有用）并告诉 LLM 如何收窄。
func capToolResult(s string) string {
	if len(s) <= maxToolResultBytes() {
		return s
	}
	return s[:maxToolResultBytes()] + fmt.Sprintf(
		"\n...[tool result truncated: %d of %d bytes shown — narrow the query (filters / head_limit / pagination) to see the rest]",
		maxToolResultBytes(), len(s))
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
	output = capToolResult(output)
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
