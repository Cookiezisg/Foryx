package workflow

// BackEdges returns the loop back-edges (keyed "from>to") of a reducible single-entry graph: an
// edge whose target is an ancestor on the DFS stack from the trigger (ADR-017). Shared by the
// validator (accept-time loop check) and the interpreter (iteration_key walk) so both agree on
// exactly which edges are loops — the divergence that left loops authored-but-rejected (review R1).
//
// BackEdges 返 reducible 单入口图的 loop 回边;validator 与 interpreter 共用,确保对"哪些是回边"一致。
func BackEdges(g Graph) map[string]bool {
	back := map[string]bool{}
	trigger := ""
	for i := range g.Nodes {
		if g.Nodes[i].Type == NodeTypeTrigger {
			trigger = g.Nodes[i].ID
			break
		}
	}
	if trigger == "" {
		return back
	}
	adj := map[string][]string{}
	for _, e := range g.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	onStack := map[string]bool{}
	visited := map[string]bool{}
	var dfs func(string)
	dfs = func(n string) {
		visited[n] = true
		onStack[n] = true
		for _, to := range adj[n] {
			if onStack[to] {
				back[n+">"+to] = true
				continue
			}
			if !visited[to] {
				dfs(to)
			}
		}
		onStack[n] = false
	}
	dfs(trigger)
	return back
}

// LoopHeaders returns the set of back-edge targets — the loop header nodes a back-edge re-enters.
//
// LoopHeaders 返回所有回边的目标节点(loop header)。
func LoopHeaders(back map[string]bool) map[string]bool {
	headers := map[string]bool{}
	for k := range back {
		for i := 0; i < len(k); i++ {
			if k[i] == '>' {
				headers[k[i+1:]] = true
				break
			}
		}
	}
	return headers
}
