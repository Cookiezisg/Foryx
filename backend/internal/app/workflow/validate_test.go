// validate_test.go — unit tests for ValidateGraph. Covers each rule in
// 04-workflow.md §7.3 + the recursive container body path.
//
// validate_test.go —— ValidateGraph 单测;覆盖 §7.3 各规则 + 容器 body 递归。

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
			edge("e1", "trig.next", "fn1.input"),
		},
	}
	if err := ValidateGraph(context.Background(), g, NopChecker()); err != nil {
		t.Errorf("valid graph rejected: %v", err)
	}
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
		Edges: []workflowdomain.EdgeSpec{edge("e1", "trig.next", "ghost.input")},
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
			edge("e1", "trig.next", "fn1.input"),
			edge("e2", "fn1.output", "fn1.input"),
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
			edge("e0", "trig.next", "a.input"),
			edge("e1", "a.output", "b.input"),
			edge("e2", "b.output", "c.input"),
			edge("e3", "c.output", "a.input"),
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
		Edges: []workflowdomain.EdgeSpec{edge("e1", "trig.next", "fn1.input")},
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
		Edges: []workflowdomain.EdgeSpec{edge("e1", "trig.next", "mcp1.input")},
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
		Edges: []workflowdomain.EdgeSpec{edge("e1", "trig.next", "fn1.input")},
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
		Edges: []workflowdomain.EdgeSpec{edge("e1", "trig.next", "fn1.input")},
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
						map[string]any{"id": "be1", "from": "a.output", "to": "b.input"},
						map[string]any{"id": "be2", "from": "b.output", "to": "a.input"},
					},
				},
			}},
		},
		Edges: []workflowdomain.EdgeSpec{edge("e1", "trig.next", "loop1.input")},
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
