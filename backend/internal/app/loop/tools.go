package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"sync"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// pkgMethodPrefix matches the §S16 "<word>.<word>: " wrap-chain head used by sanitizeToolErr.
var pkgMethodPrefix = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*(?:\.[a-zA-Z][a-zA-Z0-9_]*)+:\s+`)

// sanitizeToolErr strips §S16 wrap-chain prefixes so the LLM sees only the innermost message.
//
// sanitizeToolErr 剥 §S16 wrap-chain 前缀，让 LLM 只看最里层消息。
func sanitizeToolErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	for pkgMethodPrefix.MatchString(msg) {
		msg = pkgMethodPrefix.ReplaceAllString(msg, "")
	}
	return msg
}

// runTools executes calls in execution-group batches and returns tool_result blocks.
//
// runTools 按 execution-group 分批执行 tool 调用，返 tool_result block 列表。
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

// runOneTool runs interceptor.BeforeCall → ValidateInput →
// CheckPermissions → Execute → interceptor.AfterCall and returns a
// tool_result block. BeforeCall denial short-circuits with a denial
// tool_result; AfterCall's injected feedback is appended to the
// tool_result content for the LLM to see on the next turn.
//
// runOneTool 跑 interceptor.BeforeCall → ValidateInput → CheckPermissions
// → Execute → interceptor.AfterCall，返 tool_result block。BeforeCall
// 拒绝短路返拒绝 tool_result；AfterCall 注入文本拼到 tool_result content
// 让 LLM 下轮看到。
func runOneTool(
	ctx context.Context,
	t toolapp.Tool,
	tc chatdomain.ToolCallData,
	seq int,
	log *zap.Logger,
) chatdomain.Block {
	argsJSON, _ := json.Marshal(tc.Arguments)

	toolCtx := reqctxpkg.WithToolCallID(ctx, tc.ID)
	toolCtx = reqctxpkg.WithParentBlockID(toolCtx, tc.ID)

	intr := interceptorFrom(ctx)
	var (
		output string
		errMsg string
		ok     bool
	)
	if denied, reason := intr.BeforeCall(toolCtx, tc); denied {
		log.Info("tool blocked by interceptor",
			zap.String("tool", tc.Name), zap.String("call_id", tc.ID), zap.String("reason", reason))
		output = "permission denied: " + reason
		errMsg = "BLOCKED_BY_RULE: " + reason
		ok = false
	} else {
		output, errMsg, ok = executeTool(toolCtx, t, tc.Name, argsJSON, log)
	}

	if inject := intr.AfterCall(toolCtx, tc, output, errMsg, ok); inject != "" {
		if output != "" {
			output += "\n\n[hook] " + inject
		} else {
			output = "[hook] " + inject
		}
	}

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

	errVal := ""
	if !ok {
		errVal = errMsg
	}
	return chatdomain.Block{
		ID:            resultBlockID,
		Type:          eventlogdomain.BlockTypeToolResult,
		Content:       output,
		ParentBlockID: tc.ID,
		Error:         errVal,
	}
}

type stringErr string

func (e stringErr) Error() string { return string(e) }

// executeTool runs pre-Execute hooks then Execute (PermissionModeDefault for now).
//
// executeTool 跑前置钩子再 Execute（当前固定 PermissionModeDefault）。
func executeTool(ctx context.Context, t toolapp.Tool, name string, argsJSON []byte, log *zap.Logger) (string, string, bool) {
	if t == nil {
		// LLM picked a tool not in registry — likely wiring bug / stale catalog.
		log.Warn("executeTool: tool not in registry — likely wiring bug or stale catalog",
			zap.String("tool", name))
		msg := fmt.Sprintf("tool %q not found", name)
		return msg, msg, false
	}

	if err := t.ValidateInput(argsJSON); err != nil {
		log.Warn("tool validate failed", zap.String("tool", name), zap.Error(err))
		// LLM sees sanitized inner reason; DB errMsg keeps the full §S16 wrap chain.
		clean := sanitizeToolErr(err)
		return "input validation failed: " + clean, err.Error(), false
	}

	// Skill pre-approval (skill.md §9): active skill's allowed-tools bypass CheckPermissions.
	if state, hasState := reqctxpkg.GetAgentState(ctx); hasState {
		if state.IsToolPreApprovedBySkill(name, argsJSON) {
			log.Debug("tool pre-approved by active skill",
				zap.String("tool", name))
			return executeAfterPermission(ctx, t, name, argsJSON, log)
		}
	}

	switch t.CheckPermissions(argsJSON, toolapp.PermissionModeDefault) {
	case toolapp.PermissionDeny:
		log.Warn("tool permission denied", zap.String("tool", name))
		return "permission denied for this call", "permission denied", false
	case toolapp.PermissionAsk:
		// Phase 3+: treat as Allow (single-user local has no real-time approval channel).
	}

	return executeAfterPermission(ctx, t, name, argsJSON, log)
}

// executeAfterPermission runs t.Execute and shapes the return tuple (shared by both permission paths).
//
// executeAfterPermission 跑 t.Execute 并整形返回三元组（两条权限路径共用）。
func executeAfterPermission(ctx context.Context, t toolapp.Tool, name string, argsJSON []byte, log *zap.Logger) (string, string, bool) {
	output, err := t.Execute(ctx, string(argsJSON))
	if err != nil {
		log.Warn("tool execute failed", zap.String("tool", name), zap.Error(err))
		clean := sanitizeToolErr(err)
		if output != "" {
			return output, err.Error(), false
		}
		return clean, err.Error(), false
	}
	return output, "", true
}

type indexedCall struct {
	idx int
	tc  chatdomain.ToolCallData
}

type executionBatch struct {
	items []indexedCall
}

const autoGroupBase = 1000

// partitionByExecutionGroup buckets calls by ExecutionGroup; ≤0 get auto-assigned groups after explicit ones.
//
// partitionByExecutionGroup 按 ExecutionGroup 分桶；≤0 获自动 group，排在显式 batch 之后。
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
