package scheduler

import (
	"context"
	"fmt"
	"math"
	"strconv"

	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

// Interpreter is the durable execution engine (ADR-016). It walks the pinned graph as an agenda
// of ready (node, iteration_key) units from the trigger node:
//   - agent/tool node = an activity journaled node_started → node_completed/node_failed;
//   - case node = pure control flow journaled branch_taken; the chosen out-edge activates, the
//     others propagate a skip token so an active-branch join doesn't deadlock (A-1, 17 §3);
//   - a case back-edge to a loop header re-activates it at iteration_key+1 (ADR-017);
//   - a join (multi-in-edge) node fires once its in-edges have all arrived (active or skipped),
//     awaiting only the activated ones (AND-join when all are activated; active-branch otherwise).
//
// Run and Resume share the loop: both consult the journal first (copy-hit, ADR-019), so a
// crash-replay copies recorded results/decisions and only re-runs the first un-journaled unit.
//
// Interpreter 是 durable 执行引擎;agenda 驱动:活动记账、case 控制流 + skip token、回边 loop、
// join 等激活入边(active-branch / AND);Run/Resume 同一套,重放命中已记账抄、不重跑。
type Interpreter struct {
	journal   flowrundomain.JournalRepository
	dispatch  Dispatcher
	dryRun    bool                             // when set, side-effect nodes mock and approval auto-passes (yes)
	approvals flowrundomain.ApprovalRepository // optional: writes the approvals projection row on park (17 §9)
}

func New(journal flowrundomain.JournalRepository, dispatch Dispatcher) *Interpreter {
	return &Interpreter{journal: journal, dispatch: dispatch}
}

// WithDryRun returns the interpreter configured for a dry-run preview (no real side effects).
//
// WithDryRun 返回配置为 dry-run 预览的解释器(不产生真实副作用)。
func (in *Interpreter) WithDryRun(dry bool) *Interpreter { in.dryRun = dry; return in }

// WithApprovals wires the approvals projection so a park also writes the UI inbox/audit row.
//
// WithApprovals 接入 approvals 投影,使 park 同时写 UI inbox/审计行。
func (in *Interpreter) WithApprovals(a flowrundomain.ApprovalRepository) *Interpreter {
	in.approvals = a
	return in
}

// Run/Resume return parked=true when the flowrun suspended at an approval waiting for a signal
// (caller sets flowrun.status = awaiting_signal); false means it ran to a terminal.
func (in *Interpreter) Run(ctx context.Context, flowrunID string, g workflowdomain.Graph, input map[string]any) (bool, error) {
	return in.walk(ctx, flowrunID, g, input)
}
func (in *Interpreter) Resume(ctx context.Context, flowrunID string, g workflowdomain.Graph, input map[string]any) (bool, error) {
	return in.walk(ctx, flowrunID, g, input)
}

// ck is the per-iteration replay key: (nodeID, iteration_key) where iteration_key is the enclosing
// loop's back-edge ordinal (ADR-017); 0 outside any loop.
func ck(nodeID string, iter int) string { return nodeID + "#" + strconv.Itoa(iter) }

type agendaItem struct {
	node    string
	iter    int
	payload map[string]any
}

func (in *Interpreter) walk(ctx context.Context, flowrunID string, g workflowdomain.Graph, input map[string]any) (bool, error) {
	events, err := in.journal.LoadJournal(ctx, flowrunID)
	if err != nil {
		return false, fmt.Errorf("scheduler.walk load: %w", err)
	}
	completed := completedResults(events)
	branchTaken := branchResults(events)
	signalReceived := signalResults(events)
	parked := false

	byID := map[string]workflowdomain.NodeSpec{}
	for _, n := range g.Nodes {
		byID[n.ID] = n
	}
	backEdge := workflowdomain.BackEdges(g) // shared with the validator so authoring + execution agree
	fwdIn, backIn := inDegrees(g, backEdge)

	trigger := triggerNode(g)
	if trigger == nil {
		return false, fmt.Errorf("scheduler.walk: no trigger node in graph")
	}
	if input == nil {
		input = map[string]any{}
	}
	input = normalizeNumbers(input).(map[string]any) // match copy-hit number types (cel-safety-1)
	// ctxMap is the run-scoped read-only context CEL guards/emits read as `ctx` (17 §7: input =
	// payload + ctx). Replay-deterministic — runId + the original trigger payload are fixed for the
	// flowrun, so a guard on ctx.* evaluates identically on first run and replay (cel-safety-3).
	ctxMap := map[string]any{"runId": flowrunID, "trigger": input}

	agenda := []agendaItem{{node: trigger.ID, iter: 0, payload: input}}
	executed := map[string]bool{}
	skipped := map[string]bool{}
	arrivedActive := map[string]int{}
	arrivedTotal := map[string]int{}
	joinPayload := map[string]map[string]any{}

	// needed reports how many in-edges must arrive before (node,iter) can fire: forward in-edges
	// on the first iteration, back-edges on later iterations (single-entry loop, C6).
	needed := func(nodeID string, iter int) int {
		// Only a loop HEADER (a back-edge target) awaits its back-edge(s) at re-entry; an ordinary
		// body join always awaits its FORWARD in-degree, every iteration. The old code used back-edge
		// in-degree for every node at iter>0, so a body join (backIn=0) returned 1 and fired after a
		// single arrival — silently dropping a branch of an AND-join inside a loop (join-skip-1).
		if iter > 0 && backIn[nodeID] > 0 {
			return backIn[nodeID]
		}
		if fwdIn[nodeID] == 0 {
			return 1 // trigger / loop-entry head: one activation suffices
		}
		return fwdIn[nodeID]
	}

	var propagateSkip func(nodeID string, iter int)
	arrive := func(toID string, toIter int, payload map[string]any, active bool) {
		k := ck(toID, toIter)
		if executed[k] || skipped[k] {
			return
		}
		arrivedTotal[k]++
		if active {
			arrivedActive[k]++
			joinPayload[k] = mergeMaps(joinPayload[k], payload)
		}
		if arrivedTotal[k] < needed(toID, toIter) {
			return
		}
		if arrivedActive[k] > 0 {
			agenda = append(agenda, agendaItem{node: toID, iter: toIter, payload: joinPayload[k]})
		} else {
			propagateSkip(toID, toIter)
		}
	}
	propagateSkip = func(nodeID string, iter int) {
		k := ck(nodeID, iter)
		if executed[k] || skipped[k] {
			return
		}
		skipped[k] = true
		for _, e := range edgesFrom(g, nodeID) {
			arrive(e.To, iter, nil, false) // skip stays in the same iteration
		}
	}

	for len(agenda) > 0 {
		select {
		case <-ctx.Done():
			return parked, ctx.Err() // cancel/timeout surfaces distinctly, not as NODE_FAILED
		default:
		}
		it := agenda[0]
		agenda = agenda[1:]
		k := ck(it.node, it.iter)
		if executed[k] || skipped[k] {
			continue
		}
		executed[k] = true
		spec := byID[it.node]

		var activeTo, skipTo []string
		var out map[string]any

		switch spec.Type {
		case workflowdomain.NodeTypeTrigger:
			out = it.payload
			for _, e := range edgesFrom(g, it.node) {
				activeTo = append(activeTo, e.To)
			}
		case workflowdomain.NodeTypeCondition: // 5-node "case"
			selected, p, cErr := in.caseDecide(ctx, flowrunID, spec, it.iter, it.payload, ctxMap, branchTaken)
			if cErr != nil {
				return false, cErr
			}
			out = p
			for _, e := range edgesFrom(g, it.node) {
				if e.To == selected {
					activeTo = append(activeTo, e.To)
				} else {
					skipTo = append(skipTo, e.To)
				}
			}
		case workflowdomain.NodeTypeApproval: // durable wait for a yes/no signal
			decision := "yes" // dry-run auto-approves; otherwise the journaled signal decides
			if !in.dryRun {
				sig, ok := signalReceived[ck(it.node, it.iter)]
				if !ok {
					// park: journal signal_awaited once; the flowrun suspends (status awaiting_signal)
					// and re-walks after ResumeApproval journals the decision.
					if _, aErr := in.journal.AppendEvent(ctx, &flowrundomain.FlowRunEvent{
						FlowrunID: flowrunID, Type: flowrundomain.EventSignalAwaited, NodeID: it.node, IterationKey: it.iter,
					}); aErr != nil {
						return false, fmt.Errorf("scheduler.approval %s signal_awaited: %w", it.node, aErr)
					}
					// Write the approvals projection row (UI inbox + audit). Idempotent on replay via
					// UNIQUE(flowrun_id, node_id). Same DB as the journal write just above.
					if in.approvals != nil {
						prompt, _ := spec.Config["prompt"].(string)
						allowReason, _ := spec.Config["allowReason"].(bool)
						if pErr := in.approvals.Park(ctx, &flowrundomain.Approval{
							FlowrunID: flowrunID, NodeID: it.node, Prompt: prompt, AllowReason: allowReason, Payload: it.payload,
						}); pErr != nil {
							return false, fmt.Errorf("scheduler.approval %s park-row: %w", it.node, pErr)
						}
					}
					parked = true
					continue // do not propagate from a parked approval
				}
				decision, _ = sig["decision"].(string) // "yes" / "no"
			}
			out = it.payload
			for _, e := range edgesFrom(g, it.node) {
				if e.FromPort == decision {
					activeTo = append(activeTo, e.To)
				} else {
					skipTo = append(skipTo, e.To)
				}
			}
		default: // activity (function/handler/mcp/agent/...)
			p, aErr := in.activityRun(ctx, flowrunID, spec, it.iter, it.payload, completed)
			if aErr != nil {
				return false, aErr
			}
			out = p
			for _, e := range edgesFrom(g, it.node) {
				activeTo = append(activeTo, e.To)
			}
		}

		for _, to := range activeTo {
			toIter := it.iter
			if backEdge[it.node+">"+to] {
				toIter = it.iter + 1
			}
			arrive(to, toIter, out, true)
		}
		for _, to := range skipTo {
			propagateSkip(to, it.iter)
		}
	}
	return parked, nil
}

// caseDecide returns the chosen branch's `to` node + emitted payload, journaling branch_taken; on
// replay it copies the recorded decision. First-true-wins over per-branch CEL guards (fail-to-false G9).
func (in *Interpreter) caseDecide(ctx context.Context, flowrunID string, node workflowdomain.NodeSpec,
	iter int, payload, ctxMap map[string]any, branchTaken map[string]map[string]any) (string, map[string]any, error) {

	if bt, ok := branchTaken[ck(node.ID, iter)]; ok {
		to, _ := bt["to"].(string)
		out, _ := bt["payload"].(map[string]any)
		if out == nil {
			out = payload
		}
		return to, out, nil
	}
	specs, _ := node.Config["branches"].([]any)
	for _, b := range specs {
		bm, _ := b.(map[string]any)
		when, _ := bm["when"].(string)
		prg, err := workflowapp.CompileCEL(when)
		if err != nil {
			continue
		}
		match, evalErr := prg.EvalBool(payload, ctxMap)
		if evalErr != nil {
			match = false // G9 fail-to-false
		}
		if !match {
			continue
		}
		to, _ := bm["to"].(string)
		out := payload
		if emit, has := bm["emit"].(map[string]any); has {
			o, eErr := evalEmit(emit, payload, ctxMap)
			if eErr != nil {
				return "", nil, fmt.Errorf("scheduler.case %s: %w", node.ID, eErr)
			}
			out = o
		}
		if _, err := in.journal.AppendEvent(ctx, &flowrundomain.FlowRunEvent{
			FlowrunID: flowrunID, Type: flowrundomain.EventBranchTaken, NodeID: node.ID, IterationKey: iter,
			Result: map[string]any{"to": to, "payload": out},
		}); err != nil {
			return "", nil, fmt.Errorf("scheduler.case %s branch_taken: %w", node.ID, err)
		}
		return to, out, nil
	}
	return "", nil, fmt.Errorf("scheduler.case %s: no branch matched (missing final when:\"true\"?)", node.ID)
}

// activityRun journals an activity (node_started → Dispatch → node_completed/node_failed) or copies
// a recorded completion at this iteration on replay (ADR-019).
func (in *Interpreter) activityRun(ctx context.Context, flowrunID string, node workflowdomain.NodeSpec,
	iter int, payload map[string]any, completed map[string]map[string]any) (map[string]any, error) {

	if cached, ok := completed[ck(node.ID, iter)]; ok {
		return cached, nil
	}
	if _, err := in.journal.AppendEvent(ctx, &flowrundomain.FlowRunEvent{
		FlowrunID: flowrunID, Type: flowrundomain.EventNodeStarted, NodeID: node.ID, IterationKey: iter,
	}); err != nil {
		return nil, fmt.Errorf("scheduler.activity %s started: %w", node.ID, err)
	}
	// Dry-run: a side-effect node returns a synthetic output instead of really dispatching, so a
	// dryRun=true preview never invokes a real function/handler/mcp/http/agent (review R2 dryRun).
	var res DispatchOutput
	if in.dryRun && dryRunSideEffectNodes[node.Type] {
		res = dryRunMockOutput(node)
	} else {
		res = in.dispatch.Dispatch(ctx, DispatchInput{
			Node:   node,
			NodeIn: payload,
			ExecCtx: &ExecutionContext{
				Run:       &flowrundomain.FlowRun{ID: flowrunID},
				Variables: map[string]any{},
				Outputs:   map[string]map[string]any{},
			},
		})
	}
	if res.Error != nil {
		if _, err := in.journal.AppendEvent(ctx, &flowrundomain.FlowRunEvent{
			FlowrunID: flowrunID, Type: flowrundomain.EventNodeFailed, NodeID: node.ID, IterationKey: iter,
			Result: map[string]any{"error": res.Error.Error()},
		}); err != nil {
			return nil, fmt.Errorf("scheduler.activity %s failed-journal: %w", node.ID, err)
		}
		return nil, fmt.Errorf("scheduler.activity %s: %w", node.ID, res.Error)
	}
	// Normalize fresh activity output at the same boundary the copy-hit is normalized — a real
	// dispatcher returns JSON-decoded float64, so without this a fresh node_completed and its
	// replayed copy-hit would carry different number types (the asymmetric half of cel-safety-1).
	out := res.Outputs
	if out != nil {
		out = normalizeNumbers(out).(map[string]any)
	}
	if _, err := in.journal.AppendEvent(ctx, &flowrundomain.FlowRunEvent{
		FlowrunID: flowrunID, Type: flowrundomain.EventNodeCompleted, NodeID: node.ID, IterationKey: iter,
		Result: out,
	}); err != nil {
		return nil, fmt.Errorf("scheduler.activity %s completed: %w", node.ID, err)
	}
	return out, nil
}

// evalEmit evaluates each emit field as a bare CEL expression producing a typed value. A compile or
// eval error is RETURNED, not swallowed to nil — a bad emit is an authoring/data bug the operator
// must see (surfaced as a node failure, cel-safety-2). A non-string literal passes through.
//
// evalEmit 把每个 emit 字段当裸 CEL 求值;编译/求值错**返回**(不再吞成 nil)——坏 emit 是必须暴露的 bug。
func evalEmit(emit, payload, ctxMap map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(emit))
	for k, v := range emit {
		expr, ok := v.(string)
		if !ok {
			out[k] = v
			continue
		}
		prg, err := workflowapp.CompileCEL(expr)
		if err != nil {
			return nil, fmt.Errorf("emit %q: %w", k, err)
		}
		val, err := prg.Eval(payload, ctxMap)
		if err != nil {
			return nil, fmt.Errorf("emit %q: %w", k, err)
		}
		out[k] = val
	}
	return out, nil
}

// mergeMaps overlays b onto a copy of a (AND-join combines its activated in-edges' payloads).
func mergeMaps(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func completedResults(events []flowrundomain.FlowRunEvent) map[string]map[string]any {
	out := map[string]map[string]any{}
	for i := range events {
		if events[i].Type == flowrundomain.EventNodeCompleted {
			out[ck(events[i].NodeID, events[i].IterationKey)] = normalizeNumbers(asMap(events[i].Result)).(map[string]any)
		}
	}
	return out
}

func branchResults(events []flowrundomain.FlowRunEvent) map[string]map[string]any {
	out := map[string]map[string]any{}
	for i := range events {
		if events[i].Type == flowrundomain.EventBranchTaken {
			out[ck(events[i].NodeID, events[i].IterationKey)] = normalizeNumbers(asMap(events[i].Result)).(map[string]any)
		}
	}
	return out
}

// signalResults maps (approvalNodeID, iteration_key) → recorded signal_received result ({decision}).
func signalResults(events []flowrundomain.FlowRunEvent) map[string]map[string]any {
	out := map[string]map[string]any{}
	for i := range events {
		if events[i].Type == flowrundomain.EventSignalReceived {
			out[ck(events[i].NodeID, events[i].IterationKey)] = normalizeNumbers(asMap(events[i].Result)).(map[string]any)
		}
	}
	return out
}

func triggerNode(g workflowdomain.Graph) *workflowdomain.NodeSpec {
	for i := range g.Nodes {
		if g.Nodes[i].Type == workflowdomain.NodeTypeTrigger {
			return &g.Nodes[i]
		}
	}
	return nil
}

func edgesFrom(g workflowdomain.Graph, fromID string) []workflowdomain.EdgeSpec {
	var out []workflowdomain.EdgeSpec
	for _, e := range g.Edges {
		if e.From == fromID {
			out = append(out, e)
		}
	}
	return out
}

// Back-edge detection now lives in workflowdomain.BackEdges (shared with the validator so authoring
// and execution agree on exactly which edges are loops — review R1).

// inDegrees returns the forward (non-back-edge) and back-edge in-degree per node.
func inDegrees(g workflowdomain.Graph, backEdge map[string]bool) (fwd, back map[string]int) {
	fwd, back = map[string]int{}, map[string]int{}
	for _, e := range g.Edges {
		if backEdge[e.From+">"+e.To] {
			back[e.To]++
		} else {
			fwd[e.To]++
		}
	}
	return fwd, back
}

// normalizeNumbers folds whole-valued float64 back to int64 throughout a payload tree. JSON (the
// journal serializer) decodes every number as float64, so a copy-hit result carries float64 where a
// fresh in-memory run carries int64 — and CEL has no double+int arithmetic overload, so
// `payload.n + 1` over a replayed counter errors where the fresh run succeeds (cel-safety-1).
// Normalizing at the journal-read boundary makes fresh and replayed payloads type-identical;
// non-integral floats (3.14) are left as float64.
func normalizeNumbers(v any) any {
	switch x := v.(type) {
	case float64:
		// Only fold whole floats that fit int64; an out-of-range value (e.g. 1e19) stays float64
		// rather than saturating to MaxInt64 and silently corrupting it (round-2 number-2).
		if !math.IsInf(x, 0) && !math.IsNaN(x) && x == math.Trunc(x) && x >= -9.223372036854775e18 && x <= 9.223372036854775e18 {
			return int64(x)
		}
		return x
	case map[string]any:
		for k, e := range x {
			x[k] = normalizeNumbers(e)
		}
		return x
	case []any:
		for i, e := range x {
			x[i] = normalizeNumbers(e)
		}
		return x
	default:
		return v
	}
}

func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}
