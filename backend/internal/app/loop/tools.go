// tools.go — Tool call execution within the ReAct loop.
// Calls partition by LLM-supplied ExecutionGroup: same group = parallel batch;
// different groups = sequential ascending. Calls without an explicit group
// (≤ 0) get a unique auto-assigned group placed after all explicit ones, so
// the safe default is "run alone, sequentially."
//
// tools.go — ReAct 循环内的工具调用执行。按 LLM 提供的 ExecutionGroup
// 分组：同 group = 并行 batch；不同 group = 升序串行；无显式 group（≤ 0）
// 获得自动 group 排在所有显式 group 之后——安全默认是"独自运行，串行"。
package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// runTools executes all tool calls in execution-group batches. Per-tool
// emit (tool_result block_start/delta/stop) fires real-time inside
// runOneTool — there is no snapshot publish path. Returns the
// in-memory tool_result block slice for in-loop history extension.
//
// runTools 按 execution-group 分批执行所有 tool 调用。每个 tool 在
// runOneTool 内部实时 emit (tool_result block_start/delta/stop)
// ——无快照推送。返回内存 tool_result block 列表给循环内 history 扩展。
func runTools(
	ctx context.Context,
	calls []chatdomain.ToolCallData,
	byName map[string]toolapp.Tool,
	log *zap.Logger,
) []chatdomain.Block {
	if len(calls) == 0 {
		return nil
	}
	batches := partitionByExecutionGroup(calls)
	blocks := make([]chatdomain.Block, len(calls))

	var mu sync.Mutex
	for _, b := range batches {
		if len(b.items) > 1 {
			var wg sync.WaitGroup
			for _, item := range b.items {
				wg.Add(1)
				go func(it indexedCall) {
					defer wg.Done()
					blk := runOneTool(ctx, byName[it.tc.Name], it.tc, it.idx, log)
					mu.Lock()
					blocks[it.idx] = blk
					mu.Unlock()
				}(item)
			}
			wg.Wait()
		} else {
			item := b.items[0]
			blk := runOneTool(ctx, byName[item.tc.Name], item.tc, item.idx, log)
			mu.Lock()
			blocks[item.idx] = blk
			mu.Unlock()
		}
	}
	return blocks
}

// runOneTool executes a single tool call: ValidateInput → CheckPermissions →
// Execute, returning a tool_result block. Never errors — failures become
// ok=false results so the LLM can react.
//
// runOneTool 执行单个 tool 调用：ValidateInput → CheckPermissions → Execute，
// 返回 tool_result block。永不返 error——失败以 ok=false 呈现让 LLM 可响应。
//
// 事件日志 dual-write：用 WithParentBlockID 包工具 Execute 的 ctx，让工具
// 内部 emit（progress / 嵌套 LLM 文本）自动挂 tool_call block 下。Execute
// 返回后 emit 一个直挂 tool_call 下的 tool_result block。
func runOneTool(
	ctx context.Context,
	t toolapp.Tool,
	tc chatdomain.ToolCallData,
	seq int,
	log *zap.Logger,
) chatdomain.Block {
	// tc.Arguments is built by parseToolArgs from a map of basic types
	// (string / number / bool / nested basic-type maps); Marshal cannot
	// fail at runtime — discard err.
	// tc.Arguments 由 parseToolArgs 从基本类型 map（字串/数字/布尔/嵌套
	// 基本类型 map）构造，Marshal 运行时不可能失败——忽略 err。
	argsJSON, _ := json.Marshal(tc.Arguments)

	// reqctx for tool internals: ToolCallID + ParentBlockID (= tool_call's
	// block ID, which is the LLM tool-call ID per stream.go convention).
	//
	// 给工具内部用的 reqctx：ToolCallID + ParentBlockID（= tool_call 的
	// block ID，按 stream.go 约定即 LLM 的 tool-call ID）。
	toolCtx := reqctxpkg.WithToolCallID(ctx, tc.ID)
	toolCtx = reqctxpkg.WithParentBlockID(toolCtx, tc.ID)

	start := time.Now()
	output, errMsg, ok := executeTool(toolCtx, t, tc.Name, argsJSON, log)
	elapsedMs := time.Since(start).Milliseconds()

	// Event-log emit: tool_result is a child of tool_call.
	// 事件日志 emit：tool_result 是 tool_call 的子。
	em := eventlogpkg.From(ctx)
	msgID, _ := reqctxpkg.GetMessageID(ctx)
	resultBlockID := idgenpkg.New("blk")
	if msgID != "" && tc.ID != "" {
		em.EmitBlockStart(ctx, resultBlockID, tc.ID, msgID, eventlogdomain.BlockTypeToolResult, nil)
		if output != "" {
			em.DeltaBlock(ctx, resultBlockID, output)
		}
		status := eventlogdomain.StatusCompleted
		var stopErr error
		if !ok {
			status = eventlogdomain.StatusError
			if errMsg != "" {
				stopErr = stringErr(errMsg)
			}
		}
		em.StopBlock(ctx, resultBlockID, status, stopErr)
	}

	// In-memory block for in-loop history conversion. Content = result
	// text; ParentBlockID = tc.ID lets BlocksToAssistantLLM recover the
	// LLM tool-call ID for the role=tool message.
	//
	// 内存 block 给循环内 history 转换。Content = 结果文本；
	// ParentBlockID = tc.ID 让 BlocksToAssistantLLM 取回 LLM tool-call
	// ID 给 role=tool 消息。
	statusVal := eventlogdomain.StatusCompleted
	errVal := ""
	if !ok {
		statusVal = eventlogdomain.StatusError
		errVal = errMsg
	}
	_ = elapsedMs // legacy elapsedMs no longer carried in Block (UI gets it via DB row updated_at - created_at)
	return chatdomain.Block{
		ID:            resultBlockID,
		Type:          eventlogdomain.BlockTypeToolResult,
		Content:       output,
		ParentBlockID: tc.ID,
		Status:        statusVal,
		Error:         errVal,
		CreatedAt:     time.Now().UTC(),
	}
}

// stringErr is a tiny error wrapper that lets us pass a string through
// the (error)-typed StopBlock parameter without importing errors here.
//
// stringErr 是 string → error 的小包装，让我们能把字符串透到 StopBlock
// 的 (error) 参数，免去本包 import errors。
type stringErr string

func (e stringErr) Error() string { return string(e) }

// executeTool runs the pre-Execute hooks then Execute. Phase 3+ uses
// PermissionModeDefault; Phase 4+ scheduler will pass real modes.
//
// executeTool 跑前置钩子再 Execute。Phase 3+ 用 PermissionModeDefault；
// Phase 4+ scheduler 会传真实 mode。
func executeTool(ctx context.Context, t toolapp.Tool, name string, argsJSON []byte, log *zap.Logger) (string, string, bool) {
	if t == nil {
		// LLM picked a tool name not in the registry — wiring bug
		// (catalog generated stale name / tool removed but catalog
		// not refreshed). Log loudly so operator sees the misconfig
		// at run-time; the LLM-facing msg + DB errMsg path stays
		// unchanged so the conversation continues with a sensible
		// "tool not found" hint.
		//
		// LLM 选了不在 registry 的 tool 名——wiring bug（catalog 生成
		// 了陈旧名字 / tool 已删但 catalog 没刷新）。高声 log 让
		// operator 运行时看到 misconfig；LLM 看的 msg + DB errMsg 不变
		// 让对话能带着合理的"tool not found"提示继续。
		log.Warn("executeTool: tool not in registry — likely wiring bug or stale catalog",
			zap.String("tool", name))
		msg := fmt.Sprintf("tool %q not found", name)
		return msg, msg, false
	}

	if err := t.ValidateInput(argsJSON); err != nil {
		log.Warn("tool validate failed", zap.String("tool", name), zap.Error(err))
		return fmt.Sprintf("input validation failed: %s", err.Error()), err.Error(), false
	}

	// Skill pre-approval (skill.md §9): if an active skill on this
	// conversation lists this tool in its allowed-tools, skip the
	// per-tool CheckPermissions and treat as Allow. The check is
	// centralized here so each Tool implementation doesn't repeat
	// (per skill.md §9 line 385: "central in framework dispatch beats
	// per-tool changes"). Bare-name patterns ('Read', 'Bash') and
	// paren-form patterns ('Bash(git *)') are both honored — see
	// pkg/agentstate/skill.go::matchAllowedTool.
	//
	// Skill 预授权（skill.md §9）：本对话有 active skill 且其 allowed-
	// tools 列了本 tool → 跳过 per-tool CheckPermissions 当 Allow。集中
	// 在 framework dispatch 比改每个 Tool 划算（§9 line 385）。bare-name
	// 与 paren-form pattern 均支持——见 matchAllowedTool。
	if state, hasState := reqctxpkg.GetAgentState(ctx); hasState {
		if state.IsToolPreApprovedBySkill(name, argsJSON) {
			log.Debug("tool pre-approved by active skill",
				zap.String("tool", name))
			// Skip CheckPermissions entirely; proceed to Execute.
			// 整个跳过 CheckPermissions；直接 Execute。
			return executeAfterPermission(ctx, t, name, argsJSON, log)
		}
	}

	switch t.CheckPermissions(argsJSON, toolapp.PermissionModeDefault) {
	case toolapp.PermissionDeny:
		log.Warn("tool permission denied", zap.String("tool", name))
		return "permission denied for this call", "permission denied", false
	case toolapp.PermissionAsk:
		// Phase 4+ user-gating UI will treat Ask as a real suspension. Phase 3+
		// falls through (treat as Allow) — single-user local desktop has nobody
		// to ask in real time anyway.
		//
		// Phase 4+ 带审批 UI 的 scheduler 会把 Ask 当真挂起。Phase 3+ 落到 Allow
		// ——单用户本地桌面没真实询问通道。
	}

	return executeAfterPermission(ctx, t, name, argsJSON, log)
}

// executeAfterPermission runs t.Execute and shapes the return tuple.
// Extracted so the skill-preapproval path and the normal CheckPermissions
// path can share the post-permission codepath without duplication.
//
// executeAfterPermission 跑 t.Execute + 整形返回三元组。抽出让 skill 预
// 授权路径与正常 CheckPermissions 路径共用 post-permission 代码不重复。
func executeAfterPermission(ctx context.Context, t toolapp.Tool, name string, argsJSON []byte, log *zap.Logger) (string, string, bool) {
	output, err := t.Execute(ctx, string(argsJSON))
	if err != nil {
		log.Warn("tool execute failed", zap.String("tool", name), zap.Error(err))
		if output != "" {
			return output, err.Error(), false
		}
		return err.Error(), err.Error(), false
	}
	return output, "", true
}

// indexedCall pairs a tool call with its original index so block ordering
// survives parallel scheduling.
//
// indexedCall 把 tool 调用与原索引绑定，让 block 顺序在并行调度后还能复原。
type indexedCall struct {
	idx int
	tc  chatdomain.ToolCallData
}

// executionBatch is one set of calls that runs in parallel. Distinct batches
// run sequentially in ascending group-number order.
//
// executionBatch 是一组并行调用。不同 batch 之间按 group 号升序串行。
type executionBatch struct {
	items []indexedCall
}

// autoGroupBase keeps auto-assigned groups visibly higher than typical
// LLM-supplied numbers in logs while preserving sort order.
//
// autoGroupBase 让自动 group 在日志里显著高于 LLM 典型值，同时保持排序正确。
const autoGroupBase = 1000

// partitionByExecutionGroup buckets calls by ExecutionGroup. Calls with
// ExecutionGroup ≤ 0 get unique auto-assigned groups starting at
// max(maxExplicit+1, autoGroupBase) so unspecified calls run alone after
// all explicit batches.
//
// partitionByExecutionGroup 按 ExecutionGroup 分桶。≤ 0 的调用获得唯一
// 自动 group，从 max(maxExplicit+1, autoGroupBase) 起，让未指定的调用独自
// 运行且都排在显式 batch 之后。
func partitionByExecutionGroup(calls []chatdomain.ToolCallData) []executionBatch {
	if len(calls) == 0 {
		return nil
	}

	maxExplicit := 0
	for _, tc := range calls {
		if tc.ExecutionGroup > maxExplicit {
			maxExplicit = tc.ExecutionGroup
		}
	}
	nextAuto := maxExplicit + 1
	if nextAuto < autoGroupBase {
		nextAuto = autoGroupBase
	}

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
