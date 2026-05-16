package scheduler

import (
	"context"
	"fmt"
	"strings"

	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
)

// ConditionDispatcher evaluates condition expressions for branching.
//
// ConditionDispatcher 评估条件表达式做分支。
type ConditionDispatcher struct{}

// NewConditionDispatcher constructs ConditionDispatcher.
//
// NewConditionDispatcher 构造 ConditionDispatcher。
func NewConditionDispatcher() *ConditionDispatcher { return &ConditionDispatcher{} }

// Dispatch evaluates the condition and routes to the "true" or "false" port.
//
// Dispatch 评估 condition 并走 true / false port。
func (d *ConditionDispatcher) Dispatch(_ context.Context, in DispatchInput) DispatchOutput {
	exprSrc, _ := in.Node.Config["condition"].(string)
	if exprSrc == "" {
		return DispatchOutput{Error: fmt.Errorf("condition node %q: condition required", in.Node.ID)}
	}

	tmpl, err := workflowapp.Compile(exprSrc)
	if err != nil {
		return DispatchOutput{Error: fmt.Errorf("condition node %q: compile: %w", in.Node.ID, err)}
	}
	ctx := workflowapp.EvalContext{
		Vars:     in.ExecCtx.Variables,
		In:       in.NodeIn,
		NodesOut: in.ExecCtx.Outputs,
		Run: workflowapp.RunContext{
			ID:        in.ExecCtx.Run.ID,
			StartedAt: in.ExecCtx.Run.StartedAt.Format("2006-01-02T15:04:05Z07:00"),
		},
	}
	out, err := workflowapp.Execute(tmpl, ctx, exprSrc)
	if err != nil {
		return DispatchOutput{Error: fmt.Errorf("condition node %q: eval: %w", in.Node.ID, err)}
	}
	port := "false"
	if isTruthy(out) {
		port = "true"
	}
	return DispatchOutput{
		Outputs:  map[string]any{"out": out, "branch": port},
		NextPort: port,
	}
}

func isTruthy(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "false" || s == "0" || s == "no" || s == "null" {
		return false
	}
	return true
}
