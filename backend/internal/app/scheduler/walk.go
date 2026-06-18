package scheduler

import (
	"sort"

	flowrundomain "github.com/sunweilin/anselm/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/anselm/backend/internal/domain/workflow"
)

// nodeKey addresses one (graph node, loop iteration) — the unit the interpreter schedules and
// memoizes. iteration is 0 outside loops; a back edge bumps it by 1.
//
// nodeKey 寻址一个 (图节点, 循环轮次)——解释器调度与记忆化的单元。循环外 iteration=0；回边 +1。
type nodeKey struct {
	id   string
	iter int
}

// readyNode is a (node, iteration) the interpreter may run now.
//
// readyNode 是解释器现在可跑的一个 (节点, 轮次)。
type readyNode struct {
	node *workflowdomain.Node
	iter int
}

// walk is the per-advance derived view over the frozen graph + the run's memoized rows. It is
// rebuilt each loop turn (cheap, pure) so liveness/readiness always reflect the latest frn rows.
//
// walk 是每次 advance 在冻结图 + run 记忆化行上的派生视图。每轮重建（廉价、纯），使活跃/ready 总
// 反映最新 frn 行。
type walk struct {
	graph  *workflowdomain.Graph
	byID   map[string]*workflowdomain.Node
	declAt map[string]int // node id → declaration index (deterministic ordering)
	out    map[string][]*workflowdomain.Edge
	in     map[string][]*workflowdomain.Edge
	back   map[string]bool                               // edge id → is a back edge
	rows   map[string]map[int]*flowrundomain.FlowRunNode // node id → iteration → row
}

// newWalk indexes the graph (adjacency + back-edge classification via the SAME BackEdges the
// workflow module validates with) and the run's rows.
//
// newWalk 索引图（邻接 + 用与 workflow 模块校验同一个 BackEdges 分类回边）与 run 的行。
func newWalk(graph *workflowdomain.Graph, rows []*flowrundomain.FlowRunNode) *walk {
	w := &walk{
		graph:  graph,
		byID:   make(map[string]*workflowdomain.Node, len(graph.Nodes)),
		declAt: make(map[string]int, len(graph.Nodes)),
		out:    make(map[string][]*workflowdomain.Edge),
		in:     make(map[string][]*workflowdomain.Edge),
		back:   make(map[string]bool),
		rows:   make(map[string]map[int]*flowrundomain.FlowRunNode),
	}
	for i := range graph.Nodes {
		n := &graph.Nodes[i]
		w.byID[n.ID] = n
		w.declAt[n.ID] = i
	}
	for i := range graph.Edges {
		e := &graph.Edges[i]
		w.out[e.From] = append(w.out[e.From], e)
		w.in[e.To] = append(w.in[e.To], e)
	}
	for _, be := range workflowdomain.BackEdges(graph) {
		w.back[be.ID] = true
	}
	for _, r := range rows {
		if w.rows[r.NodeID] == nil {
			w.rows[r.NodeID] = make(map[int]*flowrundomain.FlowRunNode)
		}
		w.rows[r.NodeID][r.Iteration] = r
	}
	return w
}

// row returns the frn row for (nodeID, iter) or nil (reading a nil inner map is safe in Go).
//
// row 返 (nodeID, iter) 的 frn 行或 nil（Go 读 nil map 安全）。
func (w *walk) row(nodeID string, iter int) *flowrundomain.FlowRunNode {
	return w.rows[nodeID][iter]
}

func (w *walk) hasRow(nodeID string, iter int) bool { return w.row(nodeID, iter) != nil }

// completed reports the completed row for (nodeID, iter), if any (parked/failed are not completed).
//
// completed 报告 (nodeID, iter) 的 completed 行（parked/failed 不算 completed）。
func (w *walk) completed(nodeID string, iter int) (*flowrundomain.FlowRunNode, bool) {
	r := w.row(nodeID, iter)
	if r != nil && r.Status == flowrundomain.NodeCompleted {
		return r, true
	}
	return nil, false
}

func (w *walk) isCtlApf(nodeID string) bool {
	n := w.byID[nodeID]
	return n != nil && (n.Kind == workflowdomain.NodeKindControl || n.Kind == workflowdomain.NodeKindApproval)
}

// chosenPort returns which outgoing port a completed control/approval node selected: a control's
// result.port, an approval's result.decision (yes|no). "" for a non-resolved or non-routing node.
// This is what re-derives the active subgraph from already-recorded decisions — no skip-signal
// propagation.
//
// chosenPort 返回一个 completed control/approval 节点选了哪个出口 port：control 的 result.port、
// approval 的 result.decision。未决或非路由节点返 ""。这正是「从已落库决策重推活跃子图」——无 skip
// 信号传播。
func (w *walk) chosenPort(nodeID string, iter int) string {
	r, ok := w.completed(nodeID, iter)
	if !ok {
		return ""
	}
	switch w.byID[nodeID].Kind {
	case workflowdomain.NodeKindControl:
		return asString(r.Result[flowrundomain.ResultKeyPort])
	case workflowdomain.NodeKindApproval:
		return asString(r.Result[flowrundomain.ResultKeyDecision])
	}
	return ""
}

// edgePruned reports whether an edge from a node at srcIter is definitively dead: the source is a
// COMPLETED control/approval that chose a different port. A not-yet-completed control leaves all
// its forward edges tentatively open (resolved on a later advance turn).
//
// edgePruned 报告从 srcIter 处某节点出的边是否确定死：源是 completed 的 control/approval 且选了别的
// port。未决 control 让其所有前向边暂时在场（后续 advance 轮再定）。
func (w *walk) edgePruned(e *workflowdomain.Edge, srcIter int) bool {
	if !w.isCtlApf(e.From) {
		return false
	}
	if _, done := w.completed(e.From, srcIter); !done {
		return false
	}
	return w.chosenPort(e.From, srcIter) != e.FromPort
}

// computeReady derives the live subgraph from the recorded decisions, then returns the
// (node, iteration) pairs ready to run now. A node is ready ⟺ it is reached (some non-pruned active
// path from the seeded trigger), has no row yet, and every live incoming edge's source is completed
// (this single rule unifies AND-join for parallel fan-out and simple-merge after a control branch).
// overflow names a node whose iteration blew past MaxIterations (a runaway loop → caller fails the run).
//
// computeReady 从已落库决策推活跃子图，返回现在可跑的 (节点,轮次)。节点 ready ⟺ 它被 reached（从
// seed 的 trigger 有非剪活跃路径）、还没行、且每条 live 入边的源都 completed（这条规则统一了并行扇出
// 的 AND-join 与 control 分支后的 simple-merge）。overflow 命名 iteration 冲破 MaxIterations 的节点
// （失控循环 → 调用方失败该 run）。
func (w *walk) computeReady() (ready []readyNode, overflow string) {
	reached := make(map[nodeKey]bool)
	var queue []nodeKey

	// seed: each trigger node that has been seeded (its row exists at iteration 0).
	// seed：每个被 seed 的 trigger 节点（其行在 iteration 0 存在）。
	for i := range w.graph.Nodes {
		n := &w.graph.Nodes[i]
		if n.Kind == workflowdomain.NodeKindTrigger && w.hasRow(n.ID, 0) {
			k := nodeKey{n.ID, 0}
			if !reached[k] {
				reached[k] = true
				queue = append(queue, k)
			}
		}
	}

	// reachability BFS: forward edges propagate tentatively (a not-yet-resolved control opens all
	// its ports); a back edge is traversed ONLY when its source completed and selected that port
	// (so loops advance exactly one iteration per real decision — never tentatively, never infinitely).
	//
	// 可达 BFS：前向边暂时传播（未决 control 开所有 port）；回边仅在源 completed 且选了该 port 时走
	// （故循环每个真实决策恰进一轮——绝不暂时、绝不无限）。
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, e := range w.out[cur.id] {
			var iterB int
			if w.back[e.ID] {
				if _, done := w.completed(cur.id, cur.iter); !done {
					continue
				}
				if w.isCtlApf(cur.id) && w.chosenPort(cur.id, cur.iter) != e.FromPort {
					continue
				}
				iterB = cur.iter + 1
			} else {
				if w.edgePruned(e, cur.iter) {
					continue
				}
				iterB = cur.iter
			}
			if iterB > MaxIterations {
				return nil, e.To
			}
			k := nodeKey{e.To, iterB}
			if !reached[k] {
				reached[k] = true
				queue = append(queue, k)
			}
		}
	}

	// readiness over the reached set.
	// 在 reached 集上判 ready。
	for k := range reached {
		n := w.byID[k.id]
		if n == nil || n.Kind == workflowdomain.NodeKindTrigger {
			continue // trigger is seeded, never "ready"
		}
		if w.hasRow(k.id, k.iter) {
			continue
		}
		if w.predecessorsSatisfied(k.id, k.iter, reached) {
			ready = append(ready, readyNode{node: n, iter: k.iter})
		}
	}

	// deterministic order: by declaration index then iteration — makes replay byte-identical and
	// tests stable (the ready set itself is order-independent, but we walk it deterministically).
	//
	// 确定性序：按声明序再按 iteration——使重放逐字节一致、测试稳定。
	sort.Slice(ready, func(i, j int) bool {
		di, dj := w.declAt[ready[i].node.ID], w.declAt[ready[j].node.ID]
		if di != dj {
			return di < dj
		}
		return ready[i].iter < ready[j].iter
	})
	return ready, ""
}

// predecessorsSatisfied reports whether every LIVE incoming edge of (id, iter) has a completed
// source — and that at least one live incoming exists. A live incoming edge is one whose source is
// reached at the right iteration (forward: same iter; back: iter-1) and is not pruned. Pruned edges
// (a control branch not taken) are ignored — waiting on them would deadlock a simple-merge.
//
// predecessorsSatisfied 报告 (id, iter) 的每条 LIVE 入边是否都有 completed 源——且至少有一条 live
// 入边。live 入边 = 源在对的 iteration 被 reached（前向同 iter；回边 iter-1）且未剪。被剪边（control
// 未走的分支）忽略——等它们会让 simple-merge 死锁。
func (w *walk) predecessorsSatisfied(id string, iter int, reached map[nodeKey]bool) bool {
	hasLiveIncoming := false
	for _, e := range w.in[id] {
		srcIter := iter
		if w.back[e.ID] {
			srcIter = iter - 1
		}
		if srcIter < 0 {
			continue
		}
		if !reached[nodeKey{e.From, srcIter}] {
			continue // source not in play at that iteration
		}
		if w.edgePruned(e, srcIter) {
			continue // a control branch that was not taken
		}
		if _, done := w.completed(e.From, srcIter); !done {
			return false // a live predecessor has not completed → wait
		}
		hasLiveIncoming = true
	}
	return hasLiveIncoming
}

// scopeFor builds the model-B namespace for evaluating a node's Input at (·, iter): every node's
// completed result, addressed by node id, taken at the largest iteration ≤ iter that exists — so a
// loop-internal ancestor resolves to the current turn, a loop-external one to its fixed result.
// ctx carries the run id (the only true environment value).
//
// scopeFor 为在 (·, iter) 求值某节点 Input 构建 model-B 命名空间：每个节点的 completed result 按
// node id 寻址，取「iteration ≤ iter 中最大且存在」那个——故循环内祖先解析到当前轮、循环外到其固定
// result。ctx 携带 run id（唯一真·环境值）。
func (w *walk) scopeFor(runID string, iter int) map[string]any {
	scope := make(map[string]any, len(w.byID)+1)
	for nodeID, byIter := range w.rows {
		best := -1
		var bestRow *flowrundomain.FlowRunNode
		for it, r := range byIter {
			if it <= iter && it > best && r.Status == flowrundomain.NodeCompleted {
				best = it
				bestRow = r
			}
		}
		if bestRow != nil {
			scope[nodeID] = map[string]any(bestRow.Result)
		}
	}
	// Bind every other declared graph node id (no completed result at iteration ≤ iter — e.g. a
	// loop's back-edge predecessor on the first turn) to an EMPTY map, not absent. celScopedEnv
	// declares every node id as a CEL root, and cel-go hard-errors ("no such attribute(s)") on an
	// unbound declared root that an expression references — even inside has() — so an absent binding
	// makes the natural loop-state init `has(pred.x) ? pred.x : seed.x` un-evaluable on the first
	// turn. An empty map makes has(pred.x) cleanly false (the field is missing, the root is present),
	// so the seed branch is taken. Existing wiring is unaffected: it references completed ancestors
	// (bound to their result) and never an absent root, which cel-go only complains about when used.
	//
	// 把每个其它已声明图 node id（iteration ≤ iter 无 completed result——如循环回边前驱在首轮）绑成
	// 空 map、而非缺省。celScopedEnv 把每个 node id 声明为 CEL 根，cel-go 对表达式引用到的未绑已声明根
	// 硬报错（"no such attribute(s)"）、即便在 has() 内——故缺省绑定使自然的循环态初始化
	// `has(pred.x) ? pred.x : seed.x` 在首轮无法求值。空 map 使 has(pred.x) 干净地为 false（字段缺、根在），
	// 走 seed 分支。既有接线不受影响：它引用 completed 祖先（绑其 result）、从不引用缺省根（cel-go 只在被用到时抱怨）。
	for nodeID := range w.byID {
		if _, ok := scope[nodeID]; !ok {
			scope[nodeID] = map[string]any{}
		}
	}
	scope["ctx"] = map[string]any{"runId": runID}
	return scope
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
