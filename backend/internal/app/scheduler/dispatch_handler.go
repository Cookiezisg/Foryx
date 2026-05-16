package scheduler

import (
	"context"
	"fmt"

	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
)

// HandlerDispatcher bridges workflow handler nodes to handlerapp.Service.Call.
//
// HandlerDispatcher 把 workflow handler 节点桥接到 handlerapp.Call。
type HandlerDispatcher struct {
	svc *handlerapp.Service
}

// NewHandlerDispatcher constructs HandlerDispatcher.
//
// NewHandlerDispatcher 构造 HandlerDispatcher。
func NewHandlerDispatcher(svc *handlerapp.Service) *HandlerDispatcher {
	return &HandlerDispatcher{svc: svc}
}

// Dispatch reads handlerName + method + args and returns the result on "out".
//
// Dispatch 读 handlerName/method/args 并把结果挂 "out" port。
func (d *HandlerDispatcher) Dispatch(ctx context.Context, in DispatchInput) DispatchOutput {
	name, _ := in.Node.Config["handlerName"].(string)
	method, _ := in.Node.Config["method"].(string)
	if name == "" {
		return DispatchOutput{Error: fmt.Errorf("handler node %q: handlerName required", in.Node.ID)}
	}
	if method == "" {
		return DispatchOutput{Error: fmt.Errorf("handler node %q: method required", in.Node.ID)}
	}
	args, _ := in.Node.Config["args"].(map[string]any)

	result, err := d.svc.Call(ctx, handlerapp.CallInput{
		HandlerName: name,
		Method:      method,
		Args:        args,
		Owner: handlerapp.Owner{
			Kind: "flowrun",
			ID:   in.ExecCtx.Run.ID,
		},
	})
	if err != nil {
		return DispatchOutput{Error: err}
	}
	return DispatchOutput{Outputs: map[string]any{"out": result}}
}
