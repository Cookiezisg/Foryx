package scheduler

import (
	"context"
	"fmt"
	"strings"

	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

// ToolDispatcher routes the unified "tool" node type to the appropriate sub-dispatcher
// based on the callable prefix (doc 00/03 §"tool 节点"):
//   - fn_xxx           → FunctionDispatcher (config.functionId = fn_xxx)
//   - hd_xxx.method    → HandlerDispatcher (config.handlerName = hd_xxx, config.method)
//   - ag_xxx           → AgentDispatcher (config.agentRef = ag_xxx)
//   - mcp:server/tool  → MCPDispatcher (config.serverName, config.tool)
//
// ToolDispatcher 按 callable 前缀把 "tool" 节点路由到对应的子 dispatcher。
type ToolDispatcher struct {
	router *Router // delegates to the already-registered per-type dispatchers
}

func NewToolDispatcher(router *Router) *ToolDispatcher { return &ToolDispatcher{router: router} }

func (d *ToolDispatcher) Dispatch(ctx context.Context, in DispatchInput) DispatchOutput {
	callable, _ := in.Node.Config["callable"].(string)
	if callable == "" {
		return DispatchOutput{Error: fmt.Errorf("tool node %q: callable required (fn_xxx / hd_xxx.method / ag_xxx / mcp:server/tool)", in.Node.ID)}
	}

	args, _ := in.Node.Config["args"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}

	// Synthesize a virtual NodeSpec with the appropriate type + config so the
	// sub-dispatcher sees the same shape it expects from its native node type.
	var synth workflowdomain.NodeSpec
	synth.ID = in.Node.ID
	synth.Retry = in.Node.Retry
	synth.Timeout = in.Node.Timeout

	switch {
	case strings.HasPrefix(callable, "fn_"):
		synth.Type = workflowdomain.NodeTypeFunction
		synth.Config = map[string]any{"functionId": callable, "args": args}

	case strings.HasPrefix(callable, "hd_"):
		// callable = "hd_xxx.method"
		parts := strings.SplitN(callable, ".", 2)
		handlerName := parts[0]
		method := ""
		if len(parts) > 1 {
			method = parts[1]
		}
		synth.Type = workflowdomain.NodeTypeHandler
		synth.Config = map[string]any{"handlerName": handlerName, "method": method, "args": args}

	case strings.HasPrefix(callable, "ag_"):
		synth.Type = workflowdomain.NodeTypeAgent
		synth.Config = map[string]any{"agentRef": callable}
		// Merge any extra node config (e.g. maxTurns override) into synth config.
		for k, v := range in.Node.Config {
			if k != "callable" && k != "args" {
				synth.Config[k] = v
			}
		}

	case strings.HasPrefix(callable, "mcp:"):
		// callable = "mcp:serverName/toolName"
		rest := strings.TrimPrefix(callable, "mcp:")
		parts := strings.SplitN(rest, "/", 2)
		serverName := parts[0]
		toolName := ""
		if len(parts) > 1 {
			toolName = parts[1]
		}
		synth.Type = workflowdomain.NodeTypeMCP
		synth.Config = map[string]any{"serverName": serverName, "tool": toolName, "args": args}

	default:
		return DispatchOutput{Error: fmt.Errorf("tool node %q: unrecognized callable prefix %q (must be fn_/hd_/ag_/mcp:)", in.Node.ID, callable)}
	}

	synthIn := in
	synthIn.Node = synth
	return d.router.Dispatch(ctx, synthIn)
}
