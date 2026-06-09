package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	schedulerapp "github.com/sunweilin/forgify/backend/internal/app/scheduler"
	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

// FunctionRunner / HandlerCaller / MCPCaller / AgentInvoker are the narrow slices of the four
// execution-unit Services the scheduler dispatches into. *functionapp.Service, *handlerapp.Service,
// *mcpapp.Service, *agentapp.Service satisfy them — bootstrap depends on the methods, not the
// concrete Services (testable + no accidental coupling).
//
// FunctionRunner / HandlerCaller / MCPCaller / AgentInvoker 是 scheduler 派发进去的四个执行单元
// Service 的窄切片。bootstrap 依赖方法、非具体 Service（可测 + 无意外耦合）。
type FunctionRunner interface {
	RunFunction(ctx context.Context, in functionapp.RunInput) (*functiondomain.ExecutionResult, error)
}
type HandlerCaller interface {
	Call(ctx context.Context, in handlerapp.CallInput) (any, error)
}
type MCPCaller interface {
	CallTool(ctx context.Context, serverID, tool string, args json.RawMessage) (string, error)
}
type AgentInvoker interface {
	InvokeAgent(ctx context.Context, in agentapp.InvokeInput) (*agentapp.InvokeResult, error)
}

// dispatcher adapts the four execution-unit Services to scheduler.Dispatcher: it parses a node
// ref's prefix and routes to fn :run / hd :call / mcp tool / ag :invoke, coercing each return into
// the flat node-result map model B reads (`node.field`). action = fn_/hd_/mcp:; agent = ag_.
//
// dispatcher 把四个执行单元 Service 适配成 scheduler.Dispatcher：按 node ref 前缀分流到 fn :run /
// hd :call / mcp tool / ag :invoke，把各自返回规整成 model B 读的扁平节点结果 map（`node.field`）。
type dispatcher struct {
	fn  FunctionRunner
	hd  HandlerCaller
	mcp MCPCaller
	ag  AgentInvoker
}

// NewDispatcher wires the four callables into scheduler.Dispatcher.
//
// NewDispatcher 把四个 callable 装成 scheduler.Dispatcher。
func NewDispatcher(fn FunctionRunner, hd HandlerCaller, mcp MCPCaller, ag AgentInvoker) schedulerapp.Dispatcher {
	return dispatcher{fn: fn, hd: hd, mcp: mcp, ag: ag}
}

var _ schedulerapp.Dispatcher = dispatcher{}

// RunAction dispatches an action node (fn_ / hd_<id>.method / mcp:<serverId>/<tool>). A callable
// that runs but reports failure (function/agent OK=false) fail-fasts as an error so the node row
// is written failed (doc 21 §4.4).
//
// RunAction 派发 action 节点。callable 跑了但自报失败（OK=false）走 error fail-fast，使节点行写 failed。
func (d dispatcher) RunAction(ctx context.Context, ref string, input map[string]any) (map[string]any, error) {
	switch {
	case strings.HasPrefix(ref, workflowdomain.RefPrefixFunction):
		res, err := d.fn.RunFunction(ctx, functionapp.RunInput{
			FunctionID:  ref,
			Input:       input,
			TriggeredBy: functiondomain.TriggeredByWorkflow,
		})
		if err != nil {
			return nil, err
		}
		if res == nil {
			return map[string]any{}, nil
		}
		if !res.OK {
			return nil, fmt.Errorf("function %s failed: %s", ref, res.ErrorMsg)
		}
		return toResultMap(res.Output), nil

	case strings.HasPrefix(ref, workflowdomain.RefPrefixHandler):
		// ref = hd_<id>.method — split off the method (entityIDOf mirrors this for the pin key).
		id, method, ok := strings.Cut(ref, ".")
		if !ok || method == "" {
			return nil, fmt.Errorf("handler ref %q: want hd_<id>.method", ref)
		}
		out, err := d.hd.Call(ctx, handlerapp.CallInput{
			HandlerID:   id,
			Method:      method,
			Args:        input,
			TriggeredBy: handlerdomain.TriggeredByWorkflow,
		})
		if err != nil {
			return nil, err
		}
		return toResultMap(out), nil

	case strings.HasPrefix(ref, workflowdomain.RefPrefixMCP):
		// ref = mcp:<serverId>/<tool> — the server token IS the mcp_ id (entityIDOf treats it as
		// the pin-closure entity id; CallTool takes the mcp_ id), so it passes straight through.
		server, tool, ok := strings.Cut(strings.TrimPrefix(ref, workflowdomain.RefPrefixMCP), "/")
		if !ok || tool == "" {
			return nil, fmt.Errorf("mcp ref %q: want mcp:<serverId>/<tool>", ref)
		}
		args, err := json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("mcp ref %q: marshal args: %w", ref, err)
		}
		out, err := d.mcp.CallTool(ctx, server, tool, args)
		if err != nil {
			return nil, err
		}
		return toResultMap(out), nil

	default:
		return nil, fmt.Errorf("action ref %q: want %s / %s / %s prefix", ref,
			workflowdomain.RefPrefixFunction, workflowdomain.RefPrefixHandler, workflowdomain.RefPrefixMCP)
	}
}

// RunAgent dispatches an agent node (ag_). Coarse-grained activity (flowrun.md §4): run the full
// ReAct loop, memoize only the final result; v1 has no sub-step replay, so the InvokeInput
// workflow-replay fields stay nil.
//
// RunAgent 派发 agent 节点（ag_）。粗粒度 activity：跑完整 ReAct loop、只记忆化最终 result；v1 无子步
// 重放，故 InvokeInput 的 workflow-replay 字段留空。
func (d dispatcher) RunAgent(ctx context.Context, ref string, input map[string]any) (map[string]any, error) {
	if !strings.HasPrefix(ref, workflowdomain.RefPrefixAgent) {
		return nil, fmt.Errorf("agent ref %q: want %s prefix", ref, workflowdomain.RefPrefixAgent)
	}
	res, err := d.ag.InvokeAgent(ctx, agentapp.InvokeInput{
		AgentID:     ref,
		Input:       input,
		TriggeredBy: agentdomain.TriggeredByWorkflow,
	})
	if err != nil {
		return nil, err
	}
	if res == nil {
		return map[string]any{}, nil
	}
	if !res.OK {
		return nil, fmt.Errorf("agent %s failed: %s", ref, res.ErrorMsg)
	}
	return toResultMap(res.Output), nil
}

// toResultMap coerces a callable's return into the flat node-result map model B reads
// (`node.field`, doc 20 §5.4): a JSON object passes through; nil → empty; any scalar / string /
// array wraps under "text" (the schema-less-output convention, doc 21 §4.2 — mcp text + agent
// free-text land here).
//
// toResultMap 把 callable 返回规整成 model B 读的扁平节点结果 map：JSON 对象直通；nil → 空；标量 /
// 字符串 / 数组包进 "text"（无 schema 输出约定——mcp 文本 + agent 自由文本走这）。
func toResultMap(v any) map[string]any {
	switch t := v.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return t
	default:
		return map[string]any{"text": t}
	}
}
