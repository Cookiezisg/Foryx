package scheduler

import (
	"context"
	"fmt"
	"time"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	celpkg "github.com/sunweilin/forgify/backend/internal/pkg/cel"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// runNode executes one ready (node, iteration) and writes its frn row, returning the resulting node
// status (completed | parked | failed). action/agent are dispatched via the port; control/approval
// are inline-evaluated. Any failure (input CEL, dispatch error, resolver error) fail-fasts: the node
// row is written failed AND the run is marked failed (caller stops advancing). The two CEL axes:
// node.Input is evaluated against the model-B scope (node-id roots); control when/emit + approval
// template read `input` (the node's resolved input map).
//
// runNode 执行一个 ready (节点,轮次) 并写其 frn 行，返回节点状态。action/agent 经端口 dispatch；
// control/approval 内联求值。任何失败 fail-fast：节点行写 failed + run 标 failed。CEL 双轴：node.Input
// 对 model-B scope（node-id 根）求值；control when/emit + approval template 读 `input`（节点解析出的
// input map）。
func (s *Service) runNode(ctx context.Context, run *flowrundomain.FlowRun, senv *celpkg.ScopedEnv, w *walk, rn readyNode) (string, error) {
	node := rn.node
	iter := rn.iter
	scope := w.scopeFor(run.ID, iter)

	input, err := evalInput(senv, node, scope)
	if err != nil {
		return s.failNode(ctx, run, node, iter, fmt.Sprintf("input eval: %v", err))
	}

	// Flowrun identity rides ctx into the execution entity (function/handler/mcp/agent), whose
	// audit recorder fills the flowrun_id / flowrun_node_id columns. The pinned version travels
	// as an explicit dispatch arg: it is execution semantics (which frozen version runs), not
	// ambient identity.
	//
	// Flowrun 身份经 ctx 进执行实体（function/handler/mcp/agent），由各实体审计记账填
	// flowrun_id / flowrun_node_id 列。pin 版本走显式派发参数：它是执行语义（跑哪个冻结版本），
	// 不是环境身份。
	ctx = reqctxpkg.SetFlowrunID(ctx, run.ID)
	ctx = reqctxpkg.SetFlowrunNodeID(ctx, node.ID)

	switch node.Kind {
	case workflowdomain.NodeKindAction:
		result, err := s.dispatch.RunAction(ctx, node.Ref, run.PinnedRefs[entityIDOf(node.Ref)], input)
		if err != nil {
			return s.failNode(ctx, run, node, iter, fmt.Sprintf("action %s: %v", node.Ref, err))
		}
		return s.writeNode(ctx, run, node, iter, flowrundomain.NodeCompleted, result, "")

	case workflowdomain.NodeKindAgent:
		result, err := s.dispatch.RunAgent(ctx, node.Ref, run.PinnedRefs[entityIDOf(node.Ref)], input)
		if err != nil {
			return s.failNode(ctx, run, node, iter, fmt.Sprintf("agent %s: %v", node.Ref, err))
		}
		return s.writeNode(ctx, run, node, iter, flowrundomain.NodeCompleted, result, "")

	case workflowdomain.NodeKindControl:
		port, emit, err := s.evalControl(ctx, run, node, input)
		if err != nil {
			return s.failNode(ctx, run, node, iter, err.Error())
		}
		return s.writeNode(ctx, run, node, iter, flowrundomain.NodeCompleted, flowrundomain.ControlResult(port, emit), "")

	case workflowdomain.NodeKindApproval:
		result, err := s.renderApproval(ctx, run, node, input)
		if err != nil {
			return s.failNode(ctx, run, node, iter, err.Error())
		}
		status, werr := s.writeNode(ctx, run, node, iter, flowrundomain.NodeParked, result, "")
		if werr == nil {
			// Summon the human: a parked approval blocks the run until someone decides — the
			// inbox alone only helps whoever already checks it. At-least-once (a crash between
			// write and advance may re-emit); a duplicate summons beats a silent stall.
			// 唤人：parked 审批把 run 堵到有人决策——光有收件箱只帮到主动去看的人。at-least-once
			// （写行与 advance 之间崩溃可能重发）；重复唤起好过静默卡死。
			s.notify(ctx, "workflow.approval_pending", map[string]any{
				"workflowId": run.WorkflowID, "flowrunId": run.ID, "nodeId": node.ID,
			})
		}
		return status, werr

	default:
		return s.failNode(ctx, run, node, iter, fmt.Sprintf("unschedulable node kind %q", node.Kind))
	}
}

// writeNode upserts the node's terminal/parked row (record-once, first-wins). completed/failed stamp
// completed_at; parked leaves it nil.
//
// writeNode upsert 节点的终态/parked 行（record-once，first-wins）。completed/failed 打 completed_at；
// parked 留 nil。
func (s *Service) writeNode(ctx context.Context, run *flowrundomain.FlowRun, node *workflowdomain.Node, iter int, status string, result map[string]any, errMsg string) (string, error) {
	n := &flowrundomain.FlowRunNode{
		FlowRunID: run.ID, NodeID: node.ID, Iteration: iter, Kind: node.Kind, Ref: node.Ref,
		Status: status, Result: result, Error: errMsg,
	}
	if status != flowrundomain.NodeParked {
		now := time.Now().UTC()
		n.CompletedAt = &now
	}
	if _, err := s.runs.InsertNodeResult(ctx, n); err != nil {
		return "", fmt.Errorf("schedulerapp: write node %s: %w", node.ID, err)
	}
	return status, nil
}

// failNode writes the failed node row then fail-fasts the whole run (doc 21 §4.4): completed sibling
// rows stay memoized; :replay clears the failed row and re-walks. Returns NodeFailed so the advance
// loop stops.
//
// failNode 写 failed 节点行后 fail-fast 整个 run（doc 21 §4.4）：completed 兄弟行留作记忆化；:replay
// 清 failed 行重走。返 NodeFailed 使 advance 循环停。
func (s *Service) failNode(ctx context.Context, run *flowrundomain.FlowRun, node *workflowdomain.Node, iter int, reason string) (string, error) {
	if _, err := s.writeNode(ctx, run, node, iter, flowrundomain.NodeFailed, nil, reason); err != nil {
		return "", err
	}
	if err := s.markRunTerminal(ctx, run, flowrundomain.StatusFailed, fmt.Sprintf("node %s: %s", node.ID, reason)); err != nil {
		return "", fmt.Errorf("schedulerapp: mark run failed: %w", err)
	}
	return flowrundomain.NodeFailed, nil
}

// evalInput evaluates each node.Input field's CEL against the model-B scope (ancestor results by
// node id + ctx.runId), producing the entity's input map.
//
// evalInput 对 model-B scope（按 node id 的祖先 result + ctx.runId）求值每个 node.Input 字段的 CEL，
// 产出实体的 input map。
func evalInput(senv *celpkg.ScopedEnv, node *workflowdomain.Node, scope map[string]any) (map[string]any, error) {
	input := make(map[string]any, len(node.Input))
	for field, expr := range node.Input {
		prog, err := senv.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("field %q (%q): %w", field, expr, err)
		}
		val, err := prog.Eval(scope)
		if err != nil {
			return nil, fmt.Errorf("field %q (%q): %w", field, expr, err)
		}
		input[field] = val
	}
	return input, nil
}

// evalControl resolves the pinned control logic and evaluates its branches first-true-wins over
// `input`, returning the chosen port + that branch's emitted data (empty Emit = pass input through).
// The last branch's When is "true" (enforced at author time) so a match is guaranteed.
//
// evalControl 解析 pin 的 control 逻辑、对 `input` first-true-wins 求值其 branches，返回选中的 port +
// 该分支 emit 数据（空 Emit = 透传 input）。末条 When="true"（编排时强制）故必有匹配。
func (s *Service) evalControl(ctx context.Context, run *flowrundomain.FlowRun, node *workflowdomain.Node, input map[string]any) (string, map[string]any, error) {
	versionID := run.PinnedRefs[entityIDOf(node.Ref)]
	branches, err := s.control.Resolve(ctx, node.Ref, versionID)
	if err != nil {
		return "", nil, fmt.Errorf("resolve control %s: %w", node.Ref, err)
	}
	vars := map[string]any{"input": input}
	for _, b := range branches {
		prog, err := celpkg.Compile(b.When)
		if err != nil {
			return "", nil, fmt.Errorf("control %s when %q: %w", node.Ref, b.When, err)
		}
		ok, err := prog.EvalBool(vars)
		if err != nil {
			return "", nil, fmt.Errorf("control %s when %q: %w", node.Ref, b.When, err)
		}
		if !ok {
			continue
		}
		if len(b.Emit) == 0 {
			return b.Port, input, nil // empty emit passes input through
		}
		emit := make(map[string]any, len(b.Emit))
		for f, expr := range b.Emit {
			ep, err := celpkg.Compile(expr)
			if err != nil {
				return "", nil, fmt.Errorf("control %s emit %q: %w", node.Ref, f, err)
			}
			v, err := ep.Eval(vars)
			if err != nil {
				return "", nil, fmt.Errorf("control %s emit %q: %w", node.Ref, f, err)
			}
			emit[f] = v
		}
		return b.Port, emit, nil
	}
	return "", nil, fmt.Errorf("control %s: no branch matched (missing catch-all when=\"true\")", node.Ref)
}

// renderApproval resolves the pinned approval form and renders its markdown template over `input`,
// returning the parked-state payload (the rendered prompt + whether a reason is allowed) for the
// inbox UI. The decision itself is written later by DecideApproval / a timeout.
//
// renderApproval 解析 pin 的 approval 表单、对 `input` 渲染其 markdown 模板，返回 parked 态 payload
// （渲染好的 prompt + 是否允许备注）供收件箱 UI。决策本身由 DecideApproval / 超时稍后写。
func (s *Service) renderApproval(ctx context.Context, run *flowrundomain.FlowRun, node *workflowdomain.Node, input map[string]any) (map[string]any, error) {
	versionID := run.PinnedRefs[entityIDOf(node.Ref)]
	form, err := s.approval.Resolve(ctx, node.Ref, versionID)
	if err != nil {
		return nil, fmt.Errorf("resolve approval %s: %w", node.Ref, err)
	}
	tmpl, err := celpkg.CompileTemplate(form.Template)
	if err != nil {
		return nil, fmt.Errorf("approval %s template: %w", node.Ref, err)
	}
	rendered, err := tmpl.Render(map[string]any{"input": input})
	if err != nil {
		return nil, fmt.Errorf("approval %s render: %w", node.Ref, err)
	}
	return map[string]any{
		flowrundomain.ResultKeyRendered: rendered,
		"allowReason":                   form.AllowReason,
	}, nil
}
