package scheduler

import (
	"context"

	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

// Dispatcher is the per-NodeType executor.
//
// Dispatcher 是 per-NodeType 执行器。
type Dispatcher interface {
	// Dispatch runs the node and returns its outputs by port name.
	//
	// Dispatch 跑节点并按 port 名返回输出。
	Dispatch(ctx context.Context, in DispatchInput) DispatchOutput
}

// DispatchInput is the per-call argument to Dispatcher.Dispatch.
//
// DispatchInput 是 Dispatcher.Dispatch 的入参。
type DispatchInput struct {
	Node    workflowdomain.NodeSpec
	NodeIn  map[string]any
	ExecCtx *ExecutionContext
}

// DispatchOutput is the response from Dispatcher.Dispatch.
//
// DispatchOutput 是 Dispatcher.Dispatch 的返回。
type DispatchOutput struct {
	Outputs  map[string]any
	NextPort string
	Error    error
}

// Router maps NodeType → Dispatcher; built once at startup, read-only afterwards.
//
// Router 映射 NodeType → Dispatcher，启动期建一次，后续只读。
type Router struct {
	dispatchers map[string]Dispatcher
}

// NewRouter constructs an empty Router.
//
// NewRouter 构造空 Router。
func NewRouter() *Router {
	return &Router{dispatchers: make(map[string]Dispatcher)}
}

// Set registers a Dispatcher for a NodeType, replacing any existing one.
//
// Set 注册某 NodeType 的 Dispatcher，已存在则替换。
func (r *Router) Set(nodeType string, d Dispatcher) {
	r.dispatchers[nodeType] = d
}

// Dispatch looks up and runs the registered Dispatcher; missing type returns ErrNoDispatcherForType.
//
// Dispatch 查并跑已注册的 Dispatcher；未注册时返 ErrNoDispatcherForType。
func (r *Router) Dispatch(ctx context.Context, in DispatchInput) DispatchOutput {
	d, ok := r.dispatchers[in.Node.Type]
	if !ok {
		return DispatchOutput{Error: ErrNoDispatcherForType{Type: in.Node.Type}}
	}
	return d.Dispatch(ctx, in)
}

// ErrNoDispatcherForType is returned when a node type has no registered Dispatcher.
//
// ErrNoDispatcherForType 是未注册 NodeType 时返的错误。
type ErrNoDispatcherForType struct{ Type string }

func (e ErrNoDispatcherForType) Error() string {
	return "scheduler: no dispatcher registered for node type " + e.Type
}

// DispatcherFunc adapts a plain function to the Dispatcher interface.
//
// DispatcherFunc 把普通函数适配为 Dispatcher 接口。
type DispatcherFunc func(ctx context.Context, in DispatchInput) DispatchOutput

// Dispatch delegates to the underlying function.
//
// Dispatch 委派给底层函数。
func (f DispatcherFunc) Dispatch(ctx context.Context, in DispatchInput) DispatchOutput {
	return f(ctx, in)
}
