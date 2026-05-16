package workflow

import (
	"context"
	"errors"
	"testing"

	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

func nodeT(id, typ string) workflowdomain.NodeSpec {
	return workflowdomain.NodeSpec{ID: id, Type: typ, Config: map[string]any{}}
}

func nodeFn(id, fnID string) workflowdomain.NodeSpec {
	return workflowdomain.NodeSpec{
		ID: id, Type: workflowdomain.NodeTypeFunction,
		Config: map[string]any{"functionId": fnID},
	}
}

func edge(id, from, to string) workflowdomain.EdgeSpec {
	return workflowdomain.EdgeSpec{ID: id, From: from, To: to}
}

func TestValidate_HappyPath(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			nodeT("trig", workflowdomain.NodeTypeTrigger),
			nodeFn("fn1", "fn_x"),
		},
		Edges: []workflowdomain.EdgeSpec{
			edge("e1", "trig", "fn1"),
		},
	}
	if err := ValidateGraph(context.Background(), g, NopChecker()); err != nil {
		t.Errorf("valid graph rejected: %v", err)
	}
}

// TestValidate_PseudoTerminalNodeType_TeachesLLM verifies #11: when an LLM
// adds an "end" / "output" / "finish" node, the error message explains
// implicit DAG termination instead of a generic "unknown type".
//
// TestValidate_PseudoTerminalNodeType_TeachesLLM 验 #11:LLM 加伪 terminal
// 节点时,错误消息教学 DAG 隐式结束。
func TestValidate_PseudoTerminalNodeType_TeachesLLM(t *testing.T) {
	for _, badType := range []string{"end", "output", "finish", "terminate", "exit"} {
		t.Run(badType, func(t *testing.T) {
			g := &workflowdomain.Graph{
				Nodes: []workflowdomain.NodeSpec{
					nodeT("trig", workflowdomain.NodeTypeTrigger),
					nodeFn("fn1", "fn_x"),
					nodeT("term", badType),
				},
				Edges: []workflowdomain.EdgeSpec{
					edge("e1", "trig", "fn1"),
					edge("e2", "fn1", "term"),
				},
			}
			err := ValidateGraph(context.Background(), g, NopChecker())
			if !errors.Is(err, workflowdomain.ErrOpInvalid) {
				t.Fatalf("want ErrOpInvalid, got %v", err)
			}
			msg := err.Error()
			if !contains(msg, "no terminal node") || !contains(msg, "DAG ends implicitly") {
				t.Errorf("error message should teach implicit termination; got: %s", msg)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestValidate_NoTrigger(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{nodeFn("fn1", "fn_x")},
	}
	err := ValidateGraph(context.Background(), g, NopChecker())
	if !errors.Is(err, workflowdomain.ErrNoTrigger) {
		t.Errorf("expected ErrNoTrigger, got %v", err)
	}
}

func TestValidate_DuplicateNodeID(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			nodeT("x", workflowdomain.NodeTypeTrigger),
			nodeT("x", workflowdomain.NodeTypeFunction),
		},
	}
	err := ValidateGraph(context.Background(), g, NopChecker())
	if !errors.Is(err, workflowdomain.ErrOpInvalid) {
		t.Errorf("expected ErrOpInvalid for dup id, got %v", err)
	}
}

func TestValidate_EdgeRefMissingNode(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{nodeT("trig", workflowdomain.NodeTypeTrigger)},
		Edges: []workflowdomain.EdgeSpec{edge("e1", "trig", "ghost")},
	}
	err := ValidateGraph(context.Background(), g, NopChecker())
	if !errors.Is(err, workflowdomain.ErrInvalidReference) {
		t.Errorf("expected ErrInvalidReference, got %v", err)
	}
}

func TestValidate_SelfLoop(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			nodeT("trig", workflowdomain.NodeTypeTrigger),
			nodeFn("fn1", "fn_x"),
		},
		Edges: []workflowdomain.EdgeSpec{
			edge("e1", "trig", "fn1"),
			edge("e2", "fn1", "fn1"),
		},
	}
	err := ValidateGraph(context.Background(), g, NopChecker())
	if !errors.Is(err, workflowdomain.ErrDAGCycle) {
		t.Errorf("expected ErrDAGCycle for self-loop, got %v", err)
	}
}

func TestValidate_Cycle(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			nodeT("trig", workflowdomain.NodeTypeTrigger),
			nodeFn("a", "fn_a"),
			nodeFn("b", "fn_b"),
			nodeFn("c", "fn_c"),
		},
		Edges: []workflowdomain.EdgeSpec{
			edge("e0", "trig", "a"),
			edge("e1", "a", "b"),
			edge("e2", "b", "c"),
			edge("e3", "c", "a"),
		},
	}
	err := ValidateGraph(context.Background(), g, NopChecker())
	if !errors.Is(err, workflowdomain.ErrDAGCycle) {
		t.Errorf("expected ErrDAGCycle, got %v", err)
	}
}

// Fake checker that selectively approves names.
//
// 选择性放行的 fake checker。
type fakeChecker struct {
	functions map[string]bool
	handlers  map[string]bool
	skills    map[string]bool
	mcpServers map[string]bool
}

func (f fakeChecker) HasFunction(_ context.Context, id string) (bool, error)  { return f.functions[id], nil }
func (f fakeChecker) HasHandler(_ context.Context, n string) (bool, error)    { return f.handlers[n], nil }
func (f fakeChecker) HasSkill(_ context.Context, n string) (bool, error)      { return f.skills[n], nil }
func (f fakeChecker) HasMCPServer(_ context.Context, n string) (bool, error)  { return f.mcpServers[n], nil }

func TestValidate_CapabilityNotFound(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			nodeT("trig", workflowdomain.NodeTypeTrigger),
			nodeFn("fn1", "fn_missing"),
		},
		Edges: []workflowdomain.EdgeSpec{edge("e1", "trig", "fn1")},
	}
	err := ValidateGraph(context.Background(), g, fakeChecker{functions: map[string]bool{"fn_x": true}})
	if !errors.Is(err, workflowdomain.ErrCapabilityNotFound) {
		t.Errorf("expected ErrCapabilityNotFound, got %v", err)
	}
}

func TestValidate_MCPServerNotInstalled(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			nodeT("trig", workflowdomain.NodeTypeTrigger),
			{ID: "mcp1", Type: workflowdomain.NodeTypeMCP, Config: map[string]any{"serverName": "filesystem", "toolName": "Read"}},
		},
		Edges: []workflowdomain.EdgeSpec{edge("e1", "trig", "mcp1")},
	}
	err := ValidateGraph(context.Background(), g, fakeChecker{mcpServers: map[string]bool{}})
	if !errors.Is(err, workflowdomain.ErrMCPServerNotInstalled) {
		t.Errorf("expected ErrMCPServerNotInstalled, got %v", err)
	}
}

func TestValidate_UndeclaredVariableRef(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			nodeT("trig", workflowdomain.NodeTypeTrigger),
			{ID: "fn1", Type: workflowdomain.NodeTypeFunction, Config: map[string]any{
				"functionId": "fn_x",
				"params":     "{{ vars.notDeclared }}",
			}},
		},
		Edges: []workflowdomain.EdgeSpec{edge("e1", "trig", "fn1")},
	}
	err := ValidateGraph(context.Background(), g, NopChecker())
	if !errors.Is(err, workflowdomain.ErrInvalidReference) {
		t.Errorf("expected ErrInvalidReference for undeclared var, got %v", err)
	}
}

func TestValidate_DeclaredVariableRefPasses(t *testing.T) {
	g := &workflowdomain.Graph{
		Variables: []workflowdomain.VariableSpec{
			{Name: "lastSeen", Type: workflowdomain.VarTypeString},
		},
		Nodes: []workflowdomain.NodeSpec{
			nodeT("trig", workflowdomain.NodeTypeTrigger),
			{ID: "fn1", Type: workflowdomain.NodeTypeFunction, Config: map[string]any{
				"functionId": "fn_x",
				"params":     "{{ vars.lastSeen }}",
			}},
		},
		Edges: []workflowdomain.EdgeSpec{edge("e1", "trig", "fn1")},
	}
	if err := ValidateGraph(context.Background(), g, NopChecker()); err != nil {
		t.Errorf("declared var ref should pass: %v", err)
	}
}

func TestValidate_RecursiveBody_CycleInLoopBody(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			nodeT("trig", workflowdomain.NodeTypeTrigger),
			{ID: "loop1", Type: workflowdomain.NodeTypeLoop, Config: map[string]any{
				"items": "{{ in.list }}",
				"body": map[string]any{
					"nodes": []any{
						map[string]any{"id": "a", "type": "function", "config": map[string]any{"functionId": "fn"}},
						map[string]any{"id": "b", "type": "function", "config": map[string]any{"functionId": "fn"}},
					},
					"edges": []any{
						map[string]any{"id": "be1", "from": "a", "to": "b"},
						map[string]any{"id": "be2", "from": "b", "to": "a"},
					},
				},
			}},
		},
		Edges: []workflowdomain.EdgeSpec{edge("e1", "trig", "loop1")},
	}
	err := ValidateGraph(context.Background(), g, NopChecker())
	if !errors.Is(err, workflowdomain.ErrDAGCycle) {
		t.Errorf("expected ErrDAGCycle in loop body, got %v", err)
	}
}

func TestValidate_DuplicateVariable(t *testing.T) {
	g := &workflowdomain.Graph{
		Variables: []workflowdomain.VariableSpec{
			{Name: "x", Type: workflowdomain.VarTypeString},
			{Name: "x", Type: workflowdomain.VarTypeInteger},
		},
		Nodes: []workflowdomain.NodeSpec{nodeT("trig", workflowdomain.NodeTypeTrigger)},
	}
	err := ValidateGraph(context.Background(), g, NopChecker())
	if !errors.Is(err, workflowdomain.ErrOpInvalid) {
		t.Errorf("expected ErrOpInvalid for dup var name, got %v", err)
	}
}

// ── Port routing contract (B-05 fix, 2026-05) ────────────────────────────────
//
// Branching nodes (approval/condition/loop) MUST have edges with FromPort
// declared and valid; non-branching nodes MUST leave FromPort empty.
// Reject at validate time so degenerate graphs never reach the scheduler.

func TestValidate_ApprovalEdgeRequiresFromPort(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			nodeT("trig", workflowdomain.NodeTypeTrigger),
			nodeT("a1", workflowdomain.NodeTypeApproval),
			nodeFn("fn1", "fn_x"),
		},
		Edges: []workflowdomain.EdgeSpec{
			edge("e1", "trig", "a1"),
			edge("e2", "a1", "fn1"), // ❌ missing FromPort on approval edge
		},
	}
	err := ValidateGraph(context.Background(), g, NopChecker())
	if !errors.Is(err, workflowdomain.ErrOpInvalid) {
		t.Errorf("expected ErrOpInvalid for missing FromPort on approval, got %v", err)
	}
}

func TestValidate_ApprovalEdgeInvalidPort(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			nodeT("trig", workflowdomain.NodeTypeTrigger),
			nodeT("a1", workflowdomain.NodeTypeApproval),
			nodeFn("fn1", "fn_x"),
		},
		Edges: []workflowdomain.EdgeSpec{
			edge("e1", "trig", "a1"),
			{ID: "e2", From: "a1", FromPort: "maybe", To: "fn1"}, // ❌ "maybe" not in {approved, rejected}
		},
	}
	err := ValidateGraph(context.Background(), g, NopChecker())
	if !errors.Is(err, workflowdomain.ErrOpInvalid) {
		t.Errorf("expected ErrOpInvalid for invalid FromPort, got %v", err)
	}
}

func TestValidate_ApprovalEdgeApprovedPasses(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			nodeT("trig", workflowdomain.NodeTypeTrigger),
			nodeT("a1", workflowdomain.NodeTypeApproval),
			nodeFn("fn1", "fn_x"),
		},
		Edges: []workflowdomain.EdgeSpec{
			edge("e1", "trig", "a1"),
			{ID: "e2", From: "a1", FromPort: "approved", To: "fn1"},
		},
	}
	if err := ValidateGraph(context.Background(), g, NopChecker()); err != nil {
		t.Errorf("approved-port edge should pass, got %v", err)
	}
}

func TestValidate_SingleOutputNodeRejectsFromPort(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			nodeT("trig", workflowdomain.NodeTypeTrigger),
			nodeFn("fn1", "fn_x"),
		},
		Edges: []workflowdomain.EdgeSpec{
			{ID: "e1", From: "trig", FromPort: "output", To: "fn1"}, // ❌ trigger is single-output
		},
	}
	err := ValidateGraph(context.Background(), g, NopChecker())
	if !errors.Is(err, workflowdomain.ErrOpInvalid) {
		t.Errorf("expected ErrOpInvalid for fromPort on single-output node, got %v", err)
	}
}

func TestValidate_LegacyDottedSyntaxRejected(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			nodeT("trig", workflowdomain.NodeTypeTrigger),
			nodeFn("fn1", "fn_x"),
		},
		Edges: []workflowdomain.EdgeSpec{
			{ID: "e1", From: "trig.next", To: "fn1.input"}, // ❌ legacy syntax
		},
	}
	err := ValidateGraph(context.Background(), g, NopChecker())
	if !errors.Is(err, workflowdomain.ErrOpInvalid) {
		t.Errorf("expected ErrOpInvalid for legacy dotted syntax, got %v", err)
	}
}

func TestValidate_ConditionEdgeRequiresValidCase(t *testing.T) {
	g := &workflowdomain.Graph{
		Nodes: []workflowdomain.NodeSpec{
			nodeT("trig", workflowdomain.NodeTypeTrigger),
			{ID: "c1", Type: workflowdomain.NodeTypeCondition, Config: map[string]any{
				"cases": []any{"x_gt_5", "default"},
			}},
			nodeFn("fn1", "fn_x"),
		},
		Edges: []workflowdomain.EdgeSpec{
			edge("e1", "trig", "c1"),
			{ID: "e2", From: "c1", FromPort: "notACase", To: "fn1"}, // ❌ not in declared cases
		},
	}
	err := ValidateGraph(context.Background(), g, NopChecker())
	if !errors.Is(err, workflowdomain.ErrOpInvalid) {
		t.Errorf("expected ErrOpInvalid for invalid condition case, got %v", err)
	}
}
