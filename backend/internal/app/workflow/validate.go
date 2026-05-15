// validate.go — final-state graph validation. Service.Create / Edit run
// this after ApplyOps to confirm the resulting graph is shippable. Single
// op application happens with permissive intermediate state (apply.go);
// the strict checks are batched here so a multi-op sequence can transiently
// invalidate the graph (e.g. add_node before add_edge referencing it).
//
// Rules (per 04-workflow.md §7.3):
//   - DAG no-cycle (top + each container body)
//   - node ID unique (global, including container body nodes)
//   - node type whitelist
//   - each edge from / to references an existing node + valid port
//   - at least one trigger node
//   - capability node references resolve (function / handler / skill exist;
//     mcp server installed)
//   - variable references in expressions reference declared variables
//   - container body subgraphs recursively validate
//
// validate.go —— graph 最终态校验。Service.Create / Edit 在 ApplyOps 后跑。
// 中间态宽松,严格检查在此批跑,允许 op 序列临时不合法。

package workflow

import (
	"context"
	"fmt"
	"strings"

	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

// CapabilityChecker queries external services to confirm node references
// resolve. Inject from Service layer with concrete implementations
// (function service / handler service / mcp service / skill service).
//
// CapabilityChecker 通过外部 service 验节点引用是否存在;Service 层注入
// 具体实现(function/handler/mcp/skill service)。
type CapabilityChecker interface {
	HasFunction(ctx context.Context, id string) (bool, error)
	HasHandler(ctx context.Context, name string) (bool, error)
	HasSkill(ctx context.Context, name string) (bool, error)
	HasMCPServer(ctx context.Context, name string) (bool, error)
}

// nopChecker is the zero-value CapabilityChecker that approves every ref.
// Used in unit tests where the external service deps are not wired.
//
// nopChecker 是 zero-value CapabilityChecker,全通过;单测无外部 service
// 依赖时用。
type nopChecker struct{}

func (nopChecker) HasFunction(context.Context, string) (bool, error)  { return true, nil }
func (nopChecker) HasHandler(context.Context, string) (bool, error)   { return true, nil }
func (nopChecker) HasSkill(context.Context, string) (bool, error)     { return true, nil }
func (nopChecker) HasMCPServer(context.Context, string) (bool, error) { return true, nil }

// NopChecker returns a CapabilityChecker that approves every reference.
// Used by tests and by Service when capability validation is not yet wired.
//
// NopChecker 返一个全通过的 CapabilityChecker;测试 / 未接外部 service 时用。
func NopChecker() CapabilityChecker { return nopChecker{} }

// ValidateGraph runs all final-state checks on g. checker is the external
// service-backed reference resolver (pass NopChecker() when unwired).
// Returns the first violation wrapped as a workflowdomain sentinel so
// errors.Is(err, workflowdomain.ErrDAGCycle) etc. work cleanly.
//
// ValidateGraph 跑所有最终态检查;checker 是外部 service-backed 引用解析
// 器(未接传 NopChecker())。返第一个违规,wrap 成 workflowdomain sentinel
// 以便 errors.Is 链。
func ValidateGraph(ctx context.Context, g *workflowdomain.Graph, checker CapabilityChecker) error {
	if g == nil {
		return fmt.Errorf("validate: nil graph")
	}
	if checker == nil {
		checker = NopChecker()
	}
	return validateSubgraph(ctx, g.Nodes, g.Edges, g.Variables, checker, true /*requireTrigger*/, 0)
}

// validateSubgraph runs the full rule set on (nodes, edges, variables).
// requireTrigger toggles the "≥1 trigger" rule — true at top level, false
// for container body subgraphs (loop / parallel bodies inherit their trigger
// from the containing graph).
//
// validateSubgraph 跑完整规则。requireTrigger 顶层 true,容器 body 子图 false
// (子图共享外层 trigger)。
func validateSubgraph(
	ctx context.Context,
	nodes []workflowdomain.NodeSpec,
	edges []workflowdomain.EdgeSpec,
	vars []workflowdomain.VariableSpec,
	checker CapabilityChecker,
	requireTrigger bool,
	depth int,
) error {
	if depth > 3 {
		return fmt.Errorf("%w: container nesting exceeds depth 3", workflowdomain.ErrOpInvalid)
	}

	// 1. Node ID unique within this subgraph + each node type in whitelist.
	seen := make(map[string]bool, len(nodes))
	triggerCount := 0
	for _, n := range nodes {
		if n.ID == "" {
			return fmt.Errorf("%w: node has empty id", workflowdomain.ErrOpInvalid)
		}
		if seen[n.ID] {
			return fmt.Errorf("%w: duplicate node id %q", workflowdomain.ErrOpInvalid, n.ID)
		}
		seen[n.ID] = true
		if !workflowdomain.IsValidNodeType(n.Type) {
			// #11: LLMs trained on n8n/Zapier/StepFunctions often try to add
			// an explicit terminal node. Catch the common attempts and tell
			// them why it's wrong instead of a generic "unknown type".
			// #11: LLM 习惯加显式 terminal,catch 常见尝试并告知原因。
			if isPseudoTerminalType(n.Type) {
				return fmt.Errorf("%w: node %q uses type %q — workflows have no terminal node; the DAG ends implicitly when no edges remain. Remove this node and let the last real node be the leaf",
					workflowdomain.ErrOpInvalid, n.ID, n.Type)
			}
			return fmt.Errorf("%w: node %q has unknown type %q", workflowdomain.ErrOpInvalid, n.ID, n.Type)
		}
		if n.Type == workflowdomain.NodeTypeTrigger {
			triggerCount++
		}
		if n.OnError != "" && !workflowdomain.IsValidOnError(n.OnError) {
			return fmt.Errorf("%w: node %q has unknown onError %q", workflowdomain.ErrOpInvalid, n.ID, n.OnError)
		}
	}

	// 2. At least one trigger node (top-level only).
	if requireTrigger && triggerCount == 0 {
		return fmt.Errorf("%w: graph has no trigger node", workflowdomain.ErrNoTrigger)
	}

	// 3. Variable name uniqueness + type whitelist.
	varSeen := make(map[string]bool, len(vars))
	declaredVars := make(map[string]bool, len(vars))
	for _, v := range vars {
		if v.Name == "" {
			return fmt.Errorf("%w: variable has empty name", workflowdomain.ErrOpInvalid)
		}
		if varSeen[v.Name] {
			return fmt.Errorf("%w: duplicate variable name %q", workflowdomain.ErrOpInvalid, v.Name)
		}
		varSeen[v.Name] = true
		declaredVars[v.Name] = true
		if !workflowdomain.IsValidVariableType(v.Type) {
			return fmt.Errorf("%w: variable %q has unknown type %q", workflowdomain.ErrOpInvalid, v.Name, v.Type)
		}
	}

	// 4. Each edge references an existing node + port consistency. From / To
	// are plain node IDs; FromPort selects an output port on branching
	// nodes (approval / condition / loop). Single-output nodes must have
	// empty FromPort. Self-loop is rejected as trivial cycle.
	nodeByID := make(map[string]workflowdomain.NodeSpec, len(nodes))
	for _, n := range nodes {
		nodeByID[n.ID] = n
	}
	for _, e := range edges {
		if e.ID == "" {
			return fmt.Errorf("%w: edge has empty id", workflowdomain.ErrOpInvalid)
		}
		if strings.Contains(e.From, ".") || strings.Contains(e.To, ".") {
			return fmt.Errorf("%w: edge %q uses legacy dotted node ID; from/to must be plain node ID, use fromPort/toPort for port routing",
				workflowdomain.ErrOpInvalid, e.ID)
		}
		if !seen[e.From] {
			return fmt.Errorf("%w: edge %q from references missing node %q", workflowdomain.ErrInvalidReference, e.ID, e.From)
		}
		if !seen[e.To] {
			return fmt.Errorf("%w: edge %q to references missing node %q", workflowdomain.ErrInvalidReference, e.ID, e.To)
		}
		if e.From == e.To {
			return fmt.Errorf("%w: edge %q is a self-loop on node %q", workflowdomain.ErrDAGCycle, e.ID, e.From)
		}
		// Port consistency: branching node ⇒ FromPort required + must be
		// one of the declared ports; single-output node ⇒ FromPort must
		// be empty. Approval/loop ports are static; condition reads from
		// node.Config["cases"].
		fromNode := nodeByID[e.From]
		if workflowdomain.IsBranchingNode(fromNode.Type) {
			if e.FromPort == "" {
				return fmt.Errorf("%w: edge %q: source node %q is %s (branching); fromPort required",
					workflowdomain.ErrOpInvalid, e.ID, e.From, fromNode.Type)
			}
			cases := extractConditionCases(fromNode)
			if !workflowdomain.IsValidBranchPort(fromNode.Type, e.FromPort, cases) {
				valid := workflowdomain.BranchOutputPorts[fromNode.Type]
				if fromNode.Type == workflowdomain.NodeTypeCondition {
					valid = cases
				}
				return fmt.Errorf("%w: edge %q: fromPort %q invalid for %s node; valid ports: %v",
					workflowdomain.ErrOpInvalid, e.ID, e.FromPort, fromNode.Type, valid)
			}
		} else if e.FromPort != "" {
			return fmt.Errorf("%w: edge %q: source node %q is %s (single-output); fromPort must be empty (got %q)",
				workflowdomain.ErrOpInvalid, e.ID, e.From, fromNode.Type, e.FromPort)
		}
	}

	// 5. DAG cycle check via Kahn's algorithm.
	if err := detectCycle(nodes, edges); err != nil {
		return err
	}

	// 6. Capability reference checks (function / handler / mcp / skill).
	for _, n := range nodes {
		if err := checkCapabilityRef(ctx, n, checker); err != nil {
			return err
		}
	}

	// 7. Variable references in node configs (best-effort substring scan
	// for {{ vars.X }}). Strict expression syntax check lives in W4
	// expression.go; here we just confirm each referenced variable was
	// declared in the variables[] block.
	if err := checkVariableRefs(nodes, declaredVars); err != nil {
		return err
	}

	// 8. Recurse into container bodies (loop / parallel).
	for _, n := range nodes {
		if n.Type != workflowdomain.NodeTypeLoop && n.Type != workflowdomain.NodeTypeParallel {
			continue
		}
		if err := validateContainerBody(ctx, n, checker, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// detectCycle runs Kahn's topological sort. Cycle = nodes left unvisited
// after queue drains.
//
// detectCycle 走 Kahn 拓扑排序;队列空后剩节点 = 有环。
func detectCycle(nodes []workflowdomain.NodeSpec, edges []workflowdomain.EdgeSpec) error {
	inDegree := make(map[string]int, len(nodes))
	adj := make(map[string][]string, len(nodes))
	for _, n := range nodes {
		inDegree[n.ID] = 0
	}
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
		inDegree[e.To]++
	}
	queue := make([]string, 0, len(nodes))
	for id, d := range inDegree {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, next := range adj[id] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if visited < len(nodes) {
		return fmt.Errorf("%w: %d nodes in cycle", workflowdomain.ErrDAGCycle, len(nodes)-visited)
	}
	return nil
}

// checkCapabilityRef confirms a capability-invocation node's reference
// resolves via the CapabilityChecker. Non-capability nodes return nil.
//
// checkCapabilityRef 通过 CapabilityChecker 验 capability 节点引用是否存在;
// 非 capability 节点直返 nil。
func checkCapabilityRef(ctx context.Context, n workflowdomain.NodeSpec, checker CapabilityChecker) error {
	switch n.Type {
	case workflowdomain.NodeTypeFunction:
		id := configString(n.Config, "functionId")
		if id == "" {
			return fmt.Errorf("%w: function node %q missing functionId", workflowdomain.ErrOpInvalid, n.ID)
		}
		ok, err := checker.HasFunction(ctx, id)
		if err != nil {
			return fmt.Errorf("function ref check: %w", err)
		}
		if !ok {
			return fmt.Errorf("%w: function %q not found (node %q)", workflowdomain.ErrCapabilityNotFound, id, n.ID)
		}
	case workflowdomain.NodeTypeHandler:
		name := configString(n.Config, "handlerName")
		if name == "" {
			return fmt.Errorf("%w: handler node %q missing handlerName", workflowdomain.ErrOpInvalid, n.ID)
		}
		ok, err := checker.HasHandler(ctx, name)
		if err != nil {
			return fmt.Errorf("handler ref check: %w", err)
		}
		if !ok {
			return fmt.Errorf("%w: handler %q not found (node %q)", workflowdomain.ErrCapabilityNotFound, name, n.ID)
		}
	case workflowdomain.NodeTypeSkill:
		name := configString(n.Config, "skillName")
		if name == "" {
			return fmt.Errorf("%w: skill node %q missing skillName", workflowdomain.ErrOpInvalid, n.ID)
		}
		ok, err := checker.HasSkill(ctx, name)
		if err != nil {
			return fmt.Errorf("skill ref check: %w", err)
		}
		if !ok {
			return fmt.Errorf("%w: skill %q not found (node %q)", workflowdomain.ErrCapabilityNotFound, name, n.ID)
		}
	case workflowdomain.NodeTypeMCP:
		serverName := configString(n.Config, "serverName")
		if serverName == "" {
			return fmt.Errorf("%w: mcp node %q missing serverName", workflowdomain.ErrOpInvalid, n.ID)
		}
		ok, err := checker.HasMCPServer(ctx, serverName)
		if err != nil {
			return fmt.Errorf("mcp ref check: %w", err)
		}
		if !ok {
			return fmt.Errorf("%w: mcp server %q (node %q)", workflowdomain.ErrMCPServerNotInstalled, serverName, n.ID)
		}
	}
	return nil
}

// checkVariableRefs scans node configs for {{ vars.NAME }} references and
// confirms each name was declared. Pure substring scan — strict syntax
// parse lives in expression.go (W4); this catches the common "typo'd
// variable name" mistake at validation time.
//
// checkVariableRefs 在 node config 里扫 {{ vars.NAME }} 引用,验各 name
// 已声明。子串扫即可,严格 syntax 解析在 expression.go(W4);本步抓
// "变量名打错"常见错。
func checkVariableRefs(nodes []workflowdomain.NodeSpec, declared map[string]bool) error {
	for _, n := range nodes {
		refs := scanVarRefs(stringifyConfig(n.Config))
		for _, name := range refs {
			if !declared[name] {
				return fmt.Errorf("%w: node %q references undeclared variable %q",
					workflowdomain.ErrInvalidReference, n.ID, name)
			}
		}
	}
	return nil
}

// validateContainerBody recursively validates a loop / parallel body sub-
// graph. Body lives in node.Config["body"] as {nodes:[...], edges:[...]}.
// parallel may use "branches" key (V1: same shape as body).
//
// validateContainerBody 递归校验 loop/parallel body 子图。
func validateContainerBody(
	ctx context.Context,
	container workflowdomain.NodeSpec,
	checker CapabilityChecker,
	depth int,
) error {
	bodyKey := "body"
	if container.Type == workflowdomain.NodeTypeParallel {
		// V1 parallel.body is a SubGraph; "branches" is reserved for V1.5
		// when each branch may diverge.
		// V1 parallel.body 是单 SubGraph;branches 留 V1.5。
		if _, ok := container.Config["branches"]; ok {
			bodyKey = "branches"
		}
	}
	rawBody, ok := container.Config[bodyKey].(map[string]any)
	if !ok || rawBody == nil {
		return nil // empty body — caller's responsibility to populate before accept
	}
	nodes, err := decodeSubgraphNodes(rawBody)
	if err != nil {
		return fmt.Errorf("%w: container %q body: %v", workflowdomain.ErrOpInvalid, container.ID, err)
	}
	edges, err := decodeSubgraphEdges(rawBody)
	if err != nil {
		return fmt.Errorf("%w: container %q body: %v", workflowdomain.ErrOpInvalid, container.ID, err)
	}
	// Bodies inherit the outer trigger — don't re-require one here.
	return validateSubgraph(ctx, nodes, edges, nil, checker, false, depth)
}

// ── small helpers ────────────────────────────────────────────────────────────

func configString(cfg map[string]any, key string) string {
	if cfg == nil {
		return ""
	}
	if v, ok := cfg[key].(string); ok {
		return v
	}
	return ""
}

func stringifyConfig(cfg map[string]any) string {
	if cfg == nil {
		return ""
	}
	var b strings.Builder
	for _, v := range cfg {
		writeAny(&b, v)
		b.WriteByte(' ')
	}
	return b.String()
}

func writeAny(b *strings.Builder, v any) {
	switch x := v.(type) {
	case string:
		b.WriteString(x)
	case map[string]any:
		for _, sub := range x {
			writeAny(b, sub)
			b.WriteByte(' ')
		}
	case []any:
		for _, sub := range x {
			writeAny(b, sub)
			b.WriteByte(' ')
		}
	}
}

// scanVarRefs extracts variable names referenced via {{ vars.NAME }} in s.
// Tolerates whitespace inside the braces; ignores other reference forms
// (in.x / nodes.x.output.y / etc.) — those are static-checked elsewhere
// or treated as runtime-only.
//
// scanVarRefs 抽 {{ vars.NAME }} 引用;只看 vars.* 子集。
func scanVarRefs(s string) []string {
	var out []string
	for {
		idx := strings.Index(s, "{{")
		if idx < 0 {
			break
		}
		end := strings.Index(s[idx:], "}}")
		if end < 0 {
			break
		}
		expr := strings.TrimSpace(s[idx+2 : idx+end])
		if strings.HasPrefix(expr, "vars.") {
			name := strings.SplitN(expr[len("vars."):], " ", 2)[0]
			name = strings.SplitN(name, ".", 2)[0]
			if name != "" {
				out = append(out, name)
			}
		}
		s = s[idx+end+2:]
	}
	return out
}

// decodeSubgraphNodes / Edges parse a container body map into typed nodes /
// edges. They handle the float64 → int coercion JSON does on numbers.
//
// decodeSubgraphNodes / Edges 把 container body map 解为 typed nodes/edges。
func decodeSubgraphNodes(body map[string]any) ([]workflowdomain.NodeSpec, error) {
	rawNodes, _ := body["nodes"].([]any)
	out := make([]workflowdomain.NodeSpec, 0, len(rawNodes))
	for _, item := range rawNodes {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("node entry is not an object")
		}
		var n workflowdomain.NodeSpec
		n.ID, _ = m["id"].(string)
		n.Type, _ = m["type"].(string)
		if cfg, ok := m["config"].(map[string]any); ok {
			n.Config = cfg
		}
		if onErr, ok := m["onError"].(string); ok {
			n.OnError = onErr
		}
		if t, ok := m["timeout"].(float64); ok {
			n.Timeout = int(t)
		}
		out = append(out, n)
	}
	return out, nil
}

func decodeSubgraphEdges(body map[string]any) ([]workflowdomain.EdgeSpec, error) {
	rawEdges, _ := body["edges"].([]any)
	out := make([]workflowdomain.EdgeSpec, 0, len(rawEdges))
	for _, item := range rawEdges {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("edge entry is not an object")
		}
		var e workflowdomain.EdgeSpec
		e.ID, _ = m["id"].(string)
		e.From, _ = m["from"].(string)
		e.FromPort, _ = m["fromPort"].(string)
		e.To, _ = m["to"].(string)
		e.ToPort, _ = m["toPort"].(string)
		out = append(out, e)
	}
	return out, nil
}

// extractConditionCases reads a condition node's declared case names from
// its Config["cases"] entry. Returns nil if not a condition node or no
// cases declared. Each case entry can be either a plain string (case
// name) or an object with a "name" field.
//
// isPseudoTerminalType reports whether a node type is one of the common
// "terminal sink" names LLMs invent (n8n/Zapier/StepFunctions habit) but
// that Forgify does not have. Used to surface a teachable error instead
// of a bare "unknown type" (#11 fix).
//
// isPseudoTerminalType 判断是否常见的"伪 terminal"类型(LLM 从 n8n/Zapier
// 习惯带来),让错误消息有教学性而非裸"unknown type"(#11 修)。
func isPseudoTerminalType(t string) bool {
	switch t {
	case "end", "output", "finish", "terminate", "stop", "return", "exit":
		return true
	}
	return false
}

// extractConditionCases 读 condition 节点的 case 名;非 condition 或未声明
// 返 nil。
func extractConditionCases(n workflowdomain.NodeSpec) []string {
	if n.Type != workflowdomain.NodeTypeCondition {
		return nil
	}
	rawCases, ok := n.Config["cases"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(rawCases))
	for _, c := range rawCases {
		switch x := c.(type) {
		case string:
			if x != "" {
				out = append(out, x)
			}
		case map[string]any:
			if name, ok := x["name"].(string); ok && name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}
