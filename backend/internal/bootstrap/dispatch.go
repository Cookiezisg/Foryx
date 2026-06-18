package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentapp "github.com/sunweilin/anselm/backend/internal/app/agent"
	functionapp "github.com/sunweilin/anselm/backend/internal/app/function"
	handlerapp "github.com/sunweilin/anselm/backend/internal/app/handler"
	schedulerapp "github.com/sunweilin/anselm/backend/internal/app/scheduler"
	agentdomain "github.com/sunweilin/anselm/backend/internal/domain/agent"
	functiondomain "github.com/sunweilin/anselm/backend/internal/domain/function"
	handlerdomain "github.com/sunweilin/anselm/backend/internal/domain/handler"
	mcpdomain "github.com/sunweilin/anselm/backend/internal/domain/mcp"
	workflowdomain "github.com/sunweilin/anselm/backend/internal/domain/workflow"
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
	// ResolveServerID maps the ref's server token (name or mcp_ id) to the canonical mcp_ id that
	// CallTool keys on — so a workflow node carrying the name-form ref (what search_blocks emits) dispatches.
	// ResolveServerID 把 ref 的 server 段（名或 mcp_ id）解析成 CallTool 键用的规范 mcp_ id——使带 name 形 ref 的节点可派发。
	ResolveServerID(ctx context.Context, token string) (string, error)
	CallTool(ctx context.Context, serverID, tool string, args json.RawMessage, triggeredBy string) (string, error)
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
// is written failed. function honors pinnedVersionID (frozen at run start); handler
// (resident instance always runs the active class code) and mcp (unversioned external server) are
// live-binding and ignore it.
//
// RunAction 派发 action 节点。callable 跑了但自报失败（OK=false）走 error fail-fast，使节点行写
// failed。function 执行 pinnedVersionID（run 启动时冻结）；handler（常驻实例永远跑 active 类代码）
// 与 mcp（无版本的外部 server）活态绑定、忽略之。
func (d dispatcher) RunAction(ctx context.Context, ref, pinnedVersionID string, input map[string]any) (map[string]any, error) {
	switch {
	case strings.HasPrefix(ref, workflowdomain.RefPrefixFunction):
		res, err := d.fn.RunFunction(ctx, functionapp.RunInput{
			FunctionID:  ref,
			VersionID:   pinnedVersionID,
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
		// ref = mcp:<server>/<tool> where <server> is the server NAME (what search_blocks emits +
		// the agent wires) or the mcp_ id — resolve it to the canonical mcp_ id CallTool keys on.
		// (Previously this passed the token straight through and a name-form ref hit ErrServerNotFound.)
		//
		// ref = mcp:<server>/<tool>，<server> 是 server 名（search_blocks 给的、agent 接的）或 mcp_ id——
		// 解析成 CallTool 键用的规范 mcp_ id。（此前直传 token、name 形会撞 ErrServerNotFound。）
		server, tool, ok := strings.Cut(strings.TrimPrefix(ref, workflowdomain.RefPrefixMCP), "/")
		if !ok || tool == "" {
			return nil, fmt.Errorf("mcp ref %q: want mcp:<server>/<tool>", ref)
		}
		serverID, err := d.mcp.ResolveServerID(ctx, server)
		if err != nil {
			return nil, fmt.Errorf("mcp ref %q: %w", ref, err)
		}
		args, err := json.Marshal(input)
		if err != nil {
			return nil, fmt.Errorf("mcp ref %q: marshal args: %w", ref, err)
		}
		out, err := d.mcp.CallTool(ctx, serverID, tool, args, mcpdomain.CallTriggeredByWorkflow)
		if err != nil {
			return nil, err
		}
		return toResultMap(out), nil

	default:
		return nil, fmt.Errorf("action ref %q: want %s / %s / %s prefix", ref,
			workflowdomain.RefPrefixFunction, workflowdomain.RefPrefixHandler, workflowdomain.RefPrefixMCP)
	}
}

// RunAgent dispatches an agent node (ag_). Coarse-grained activity (foundation/scheduler-flowrun.md):
// run the full ReAct loop against the pinned version (frozen at run start), memoize only the final
// result; v1 has no sub-step replay, so the InvokeInput workflow-replay fields stay nil.
//
// RunAgent 派发 agent 节点（ag_）。粗粒度 activity：对 pin 版本（run 启动时冻结）跑完整 ReAct loop、
// 只记忆化最终 result；v1 无子步重放，故 InvokeInput 的 workflow-replay 字段留空。
func (d dispatcher) RunAgent(ctx context.Context, ref, pinnedVersionID string, input map[string]any) (map[string]any, error) {
	if !strings.HasPrefix(ref, workflowdomain.RefPrefixAgent) {
		return nil, fmt.Errorf("agent ref %q: want %s prefix", ref, workflowdomain.RefPrefixAgent)
	}
	res, err := d.ag.InvokeAgent(ctx, agentapp.InvokeInput{
		AgentID:     ref,
		VersionID:   pinnedVersionID,
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
// (`node.field`): a JSON object passes through; nil → empty; any scalar / string /
// array wraps under "text" (the schema-less-output convention — mcp text + agent
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
