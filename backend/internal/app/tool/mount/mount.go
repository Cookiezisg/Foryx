// Package mount synthesizes an agent's mounted ToolRefs (fn_ / hd_…method / mcp:server/tool)
// into bound, callable tools — one tool per mount, named after the target entity, carrying the
// target's own description + input schema, executing through the entity's standard execution
// method (RunFunction / Call / CallTool) with TriggeredBy=agent.
//
// This is the OPPOSITE of handing the agent the generic system-tool registry: an agent never
// sees run_function / call_handler / Read / Bash — it sees exactly the capabilities mounted on
// its version, each pre-bound to its target (no free id parameter for the LLM to wander with).
// Resolution happens per invoke against live entities, so a renamed function surfaces its
// current name and a deleted one fails the invoke loudly (mount resolve is fail-fast — a worker
// missing a declared capability must not run degraded silently).
//
// Package mount 把 agent 挂载的 ToolRef（fn_ / hd_…method / mcp:server/tool）合成为绑定的可调工具
// ——每个挂载一个工具：以目标实体命名、带目标自己的描述 + 输入 schema、经实体标准执行方法
// （RunFunction / Call / CallTool）执行、TriggeredBy=agent。
//
// 这与把通用系统工具表给 agent **相反**：agent 永远看不到 run_function / call_handler / Read /
// Bash——它看到的恰是其版本上挂载的能力，且各自已绑定目标（不留自由 id 参数让 LLM 乱走）。
// 解析在每次 invoke 时对活实体进行：改名的 function 以现名出现、被删的让 invoke 大声失败
// （mount 解析 fail-fast——worker 缺声明能力绝不静默降级运行）。
package mount

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// FunctionPort / HandlerPort / MCPPort are the narrow slices of the three execution services the
// resolver needs (DIP — satisfied by the concrete app Services, fakeable in tests).
//
// FunctionPort / HandlerPort / MCPPort 是 resolver 所需三个执行服务的窄切片（DIP——由具体 app
// Service 满足、测试可 fake）。
type (
	FunctionPort interface {
		Get(ctx context.Context, id string) (*functiondomain.Function, error)
		RunFunction(ctx context.Context, in functionapp.RunInput) (*functiondomain.ExecutionResult, error)
	}
	HandlerPort interface {
		Get(ctx context.Context, id string) (*handlerdomain.Handler, error)
		Call(ctx context.Context, in handlerapp.CallInput) (any, error)
	}
	MCPPort interface {
		ListServers(ctx context.Context) ([]mcpdomain.ServerStatus, error)
		ListTools(ctx context.Context) ([]mcpdomain.ToolDef, error)
		CallTool(ctx context.Context, serverID, tool string, args json.RawMessage, triggeredBy string) (string, error)
	}
)

// Resolver resolves an agent version's mounts into bound tools.
//
// Resolver 把 agent 版本的挂载解析成绑定工具。
type Resolver struct {
	fn  FunctionPort
	hd  HandlerPort
	mcp MCPPort
}

// NewResolver wires the three execution ports; a nil port disables that mount kind (resolving
// such a ref then fails as unresolvable — a wiring gap surfaces loudly, not as a missing tool).
//
// NewResolver 接三个执行端口；nil 端口禁用对应挂载类（解析该类 ref 即失败——装配缺口大声暴露、
// 而非工具悄悄缺席）。
func NewResolver(fn FunctionPort, hd HandlerPort, mcp MCPPort) *Resolver {
	return &Resolver{fn: fn, hd: hd, mcp: mcp}
}

// Resolve turns every mounted ref into a bound tool, fail-fast on the first unresolvable one.
// Synthesized names must be unique — two mounts colliding on one LLM tool name is a config
// error, not something to silently last-write-win.
//
// Resolve 把每个挂载 ref 解析成绑定工具，遇第一个不可解析即失败。合成名必须唯一——两个挂载撞
// 同一 LLM 工具名是配置错误，不做静默覆盖。
func (r *Resolver) Resolve(ctx context.Context, refs []agentdomain.ToolRef) ([]toolapp.Tool, error) {
	tools := make([]toolapp.Tool, 0, len(refs))
	seen := make(map[string]string, len(refs)) // tool name → ref（撞名报错用）
	for _, tr := range refs {
		ref := strings.TrimSpace(tr.Ref)
		var (
			t   toolapp.Tool
			err error
		)
		switch {
		case strings.HasPrefix(ref, "fn_"):
			t, err = r.functionTool(ctx, ref)
		case strings.HasPrefix(ref, "hd_"):
			t, err = r.handlerTool(ctx, ref)
		case strings.HasPrefix(ref, "mcp:"):
			t, err = r.mcpTool(ctx, ref)
		default:
			err = fmt.Errorf("unknown ref scheme: %w", agentdomain.ErrMountInvalid)
		}
		if err != nil {
			return nil, fmt.Errorf("mount %q: %w", ref, err)
		}
		if prev, dup := seen[t.Name()]; dup {
			return nil, fmt.Errorf("mount %q: tool name %q collides with mount %q: %w",
				ref, t.Name(), prev, agentdomain.ErrMountInvalid)
		}
		seen[t.Name()] = ref
		tools = append(tools, t)
	}
	return tools, nil
}

// CheckHealth resolves each mount INDEPENDENTLY (no fail-fast) and reports per-mount status — the
// on-demand counterpart to Resolve, for an agent's mount-health precheck. Reuses the same per-ref
// resolvers (so a broken mount here is exactly what would fail an invoke), but collects every
// result instead of stopping at the first failure.
//
// CheckHealth 独立解析每个挂载（不 fail-fast）并报告逐挂载状态——Resolve 的按需对应物，给 agent 挂载
// 健康预检。复用同一批 per-ref 解析器（故此处坏的挂载正是 invoke 会失败的那个），但收集每条结果而非
// 遇首个失败即停。
func (r *Resolver) CheckHealth(ctx context.Context, refs []agentdomain.ToolRef) []agentdomain.MountHealth {
	out := make([]agentdomain.MountHealth, 0, len(refs))
	for _, tr := range refs {
		ref := strings.TrimSpace(tr.Ref)
		var (
			t   toolapp.Tool
			err error
		)
		switch {
		case strings.HasPrefix(ref, "fn_"):
			t, err = r.functionTool(ctx, ref)
		case strings.HasPrefix(ref, "hd_"):
			t, err = r.handlerTool(ctx, ref)
		case strings.HasPrefix(ref, "mcp:"):
			t, err = r.mcpTool(ctx, ref)
		default:
			err = fmt.Errorf("unknown ref scheme: %w", agentdomain.ErrMountInvalid)
		}
		h := agentdomain.MountHealth{Ref: ref, Healthy: err == nil}
		if err != nil {
			h.Error = err.Error()
		} else {
			h.Name = t.Name()
		}
		out = append(out, h)
	}
	return out
}

// --- function mount (fn_<id>) ------------------------------------------------

func (r *Resolver) functionTool(ctx context.Context, ref string) (toolapp.Tool, error) {
	if r.fn == nil {
		return nil, fmt.Errorf("function port not wired: %w", agentdomain.ErrMountInvalid)
	}
	f, err := r.fn.Get(ctx, ref)
	if err != nil {
		return nil, err
	}
	if f.ActiveVersion == nil {
		return nil, functiondomain.ErrNoActiveVersion
	}
	desc := f.Description
	if desc == "" {
		desc = "Run the " + f.Name + " function."
	}
	return &functionTool{
		fn: r.fn, functionID: f.ID, name: f.Name,
		description: desc, params: schemapkg.ToJSONSchema(f.ActiveVersion.Inputs),
	}, nil
}

// functionTool is one mounted function, bound to its id — the LLM passes only the function's
// own declared inputs.
//
// functionTool 是一个挂载的 function，绑定其 id——LLM 只传该 function 自己声明的输入。
type functionTool struct {
	fn          FunctionPort
	functionID  string
	name        string
	description string
	params      json.RawMessage
}

func (t *functionTool) Name() string                        { return t.name }
func (t *functionTool) Description() string                 { return t.description }
func (t *functionTool) Parameters() json.RawMessage         { return t.params }
func (t *functionTool) ValidateInput(json.RawMessage) error { return nil } // 实体执行路径自校验

func (t *functionTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("%s: bad args: %w", t.name, err)
	}
	res, err := t.fn.RunFunction(ctx, functionapp.RunInput{
		FunctionID:  t.functionID,
		Input:       args,
		TriggeredBy: functiondomain.TriggeredByAgent,
	})
	if err != nil {
		return "", fmt.Errorf("%s: %w", t.name, err)
	}
	return toolapp.ToJSON(res), nil
}

// --- handler mount (hd_<id>.method) -------------------------------------------

func (r *Resolver) handlerTool(ctx context.Context, ref string) (toolapp.Tool, error) {
	if r.hd == nil {
		return nil, fmt.Errorf("handler port not wired: %w", agentdomain.ErrMountInvalid)
	}
	id, method, ok := strings.Cut(ref, ".")
	if !ok || method == "" {
		return nil, fmt.Errorf("want hd_<id>.<method>: %w", agentdomain.ErrMountInvalid)
	}
	h, err := r.hd.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if h.ActiveVersion == nil {
		return nil, handlerdomain.ErrNoActiveVersion
	}
	var spec *handlerdomain.MethodSpec
	for i := range h.ActiveVersion.Methods {
		if h.ActiveVersion.Methods[i].Name == method {
			spec = &h.ActiveVersion.Methods[i]
			break
		}
	}
	if spec == nil {
		return nil, handlerdomain.ErrMethodNotFound
	}
	desc := spec.Description
	if desc == "" {
		desc = "Call the " + method + " method of the " + h.Name + " handler."
	}
	return &handlerTool{
		hd: r.hd, handlerID: h.ID, method: method,
		name:        h.Name + "__" + method, // mcp__ 风格：LLM 工具名不许 '.'
		description: desc, params: schemapkg.ToJSONSchema(spec.Inputs),
	}, nil
}

// handlerTool is one mounted handler method, bound to handler id + method.
//
// handlerTool 是一个挂载的 handler 方法，绑定 handler id + method。
type handlerTool struct {
	hd          HandlerPort
	handlerID   string
	method      string
	name        string
	description string
	params      json.RawMessage
}

func (t *handlerTool) Name() string                        { return t.name }
func (t *handlerTool) Description() string                 { return t.description }
func (t *handlerTool) Parameters() json.RawMessage         { return t.params }
func (t *handlerTool) ValidateInput(json.RawMessage) error { return nil }

func (t *handlerTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("%s: bad args: %w", t.name, err)
	}
	// Stream the method's yields live under this tool_call (same bridge as call_handler).
	// 把 method 的 yield 实时流在本 tool_call 下（与 call_handler 同桥）。
	prog := loopapp.ToolProgress(ctx)
	defer prog.Close()
	res, err := t.hd.Call(ctx, handlerapp.CallInput{
		HandlerID: t.handlerID, Method: t.method, Args: args,
		TriggeredBy: handlerdomain.TriggeredByAgent,
		OnProgress: func(v any) {
			if s, ok := v.(string); ok {
				prog.Print(s + "\n")
				return
			}
			b, _ := json.Marshal(v)
			prog.Print(string(b) + "\n")
		},
	})
	if err != nil {
		return "", fmt.Errorf("%s: %w", t.name, err)
	}
	return toolapp.ToJSON(map[string]any{"result": res}), nil
}

// --- mcp mount (mcp:server/tool) ------------------------------------------------

func (r *Resolver) mcpTool(ctx context.Context, ref string) (toolapp.Tool, error) {
	if r.mcp == nil {
		return nil, fmt.Errorf("mcp port not wired: %w", agentdomain.ErrMountInvalid)
	}
	server, tool, ok := strings.Cut(strings.TrimPrefix(ref, "mcp:"), "/")
	if !ok || server == "" || tool == "" {
		return nil, fmt.Errorf("want mcp:<server>/<tool>: %w", agentdomain.ErrMountInvalid)
	}
	servers, err := r.mcp.ListServers(ctx)
	if err != nil {
		return nil, err
	}
	serverID := ""
	for _, s := range servers {
		if s.Name == server {
			serverID = s.ID
			break
		}
	}
	if serverID == "" {
		return nil, mcpdomain.ErrServerNotFound
	}
	defs, err := r.mcp.ListTools(ctx) // 只含 callable server 的工具——离线 server 即解析失败（fail-fast）
	if err != nil {
		return nil, err
	}
	for _, d := range defs {
		if d.ServerName == server && d.Name == tool {
			return &mcpTool{
				mcp: r.mcp, serverID: serverID, serverName: server, toolName: tool,
				description: d.Description, params: d.InputSchema,
			}, nil
		}
	}
	return nil, mcpdomain.ErrToolNotFound
}

// mcpTool is one mounted MCP tool, bound to its server id (mirrors tool/mcp's dynamicTool —
// same name shape, same progress bridge — but resolved from a mount ref instead of the registry).
//
// mcpTool 是一个挂载的 MCP 工具，绑定其 server id（对齐 tool/mcp 的 dynamicTool——同名形、同进度
// 桥——只是从挂载 ref 解析而非注册表）。
type mcpTool struct {
	mcp         MCPPort
	serverID    string
	serverName  string
	toolName    string
	description string
	params      json.RawMessage
}

func (t *mcpTool) Name() string                        { return "mcp__" + t.serverName + "__" + t.toolName }
func (t *mcpTool) Description() string                 { return t.description }
func (t *mcpTool) Parameters() json.RawMessage         { return t.params }
func (t *mcpTool) ValidateInput(json.RawMessage) error { return nil } // MCP server 自校验

func (t *mcpTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	prog := loopapp.ToolProgress(ctx)
	defer prog.Close()
	ctx = mcpinfra.WithProgress(ctx, prog.Print)
	return t.mcp.CallTool(ctx, t.serverID, t.toolName, json.RawMessage(argsJSON), mcpdomain.CallTriggeredByAgent)
}
