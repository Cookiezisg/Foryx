package scheduler

import (
	"context"
	"fmt"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
)

// FunctionDispatcher bridges workflow function nodes to functionapp.
//
// FunctionDispatcher 把 workflow function 节点桥接到 functionapp。
type FunctionDispatcher struct {
	svc *functionapp.Service
}

// NewFunctionDispatcher constructs FunctionDispatcher with the function service.
//
// NewFunctionDispatcher 构造 FunctionDispatcher。
func NewFunctionDispatcher(svc *functionapp.Service) *FunctionDispatcher {
	return &FunctionDispatcher{svc: svc}
}

// Dispatch reads functionId + args from node.Config and runs the function.
// Accepts either `args` (canonical — same as handler dispatcher + HTTP `:run`
// endpoint + function tool surface) or legacy `input` (pre-trinity workflows).
//
// Dispatch 读 functionId + args 跑 function。`args` 是规范字段（与 handler
// dispatcher + HTTP `:run` 一致）；`input` 是 trinity 重构前遗留别名，保留兼容。
func (d *FunctionDispatcher) Dispatch(ctx context.Context, in DispatchInput) DispatchOutput {
	fnID, _ := in.Node.Config["functionId"].(string)
	if fnID == "" {
		return DispatchOutput{Error: fmt.Errorf("function node %q: functionId required", in.Node.ID)}
	}
	args, _ := in.Node.Config["args"].(map[string]any)
	if args == nil {
		args, _ = in.Node.Config["input"].(map[string]any)
	}
	versionID, _ := in.Node.Config["version"].(string)

	result, err := d.svc.RunFunction(ctx, functionapp.RunInput{
		FunctionID:  fnID,
		VersionID:   versionID,
		Input:       args,
		TriggeredBy: functiondomain.TriggeredByWorkflow,
	})
	if err != nil {
		return DispatchOutput{Error: err}
	}
	if result != nil && !result.OK {
		return DispatchOutput{Error: fmt.Errorf("function %q: %s", fnID, result.ErrorMsg)}
	}
	out := map[string]any{}
	if result != nil {
		out["out"] = result.Output
		out["elapsedMs"] = result.ElapsedMs
	}
	return DispatchOutput{Outputs: out}
}
