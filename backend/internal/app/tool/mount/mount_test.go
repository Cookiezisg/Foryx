package mount

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// --- fakes ------------------------------------------------------------------

type fakeFn struct {
	f      *functiondomain.Function
	gotRun functionapp.RunInput
}

func (m *fakeFn) Get(_ context.Context, id string) (*functiondomain.Function, error) {
	if m.f == nil || m.f.ID != id {
		return nil, functiondomain.ErrNotFound
	}
	return m.f, nil
}

func (m *fakeFn) RunFunction(_ context.Context, in functionapp.RunInput) (*functiondomain.ExecutionResult, error) {
	m.gotRun = in
	return &functiondomain.ExecutionResult{OK: true, Output: map[string]any{"sum": 3}}, nil
}

type fakeHd struct {
	h       *handlerdomain.Handler
	gotCall handlerapp.CallInput
}

func (m *fakeHd) Get(_ context.Context, id string) (*handlerdomain.Handler, error) {
	if m.h == nil || m.h.ID != id {
		return nil, handlerdomain.ErrNotFound
	}
	return m.h, nil
}

func (m *fakeHd) Call(_ context.Context, in handlerapp.CallInput) (any, error) {
	m.gotCall = in
	return map[string]any{"sent": true}, nil
}

type fakeMCP struct {
	servers []mcpdomain.ServerStatus
	tools   []mcpdomain.ToolDef
	gotCall struct{ serverID, tool, triggeredBy string }
}

func (m *fakeMCP) ListServers(context.Context) ([]mcpdomain.ServerStatus, error) {
	return m.servers, nil
}
func (m *fakeMCP) ListTools(context.Context) ([]mcpdomain.ToolDef, error) { return m.tools, nil }
func (m *fakeMCP) CallTool(_ context.Context, serverID, tool string, _ json.RawMessage, triggeredBy string) (string, error) {
	m.gotCall = struct{ serverID, tool, triggeredBy string }{serverID, tool, triggeredBy}
	return "mcp ok", nil
}

func fixtureFn() *functiondomain.Function {
	return &functiondomain.Function{
		ID: "fn_1", Name: "add_numbers", Description: "Adds two numbers.",
		ActiveVersion: &functiondomain.Version{Inputs: []schemapkg.Field{
			{Name: "a", Type: "number"}, {Name: "b", Type: "number"},
		}},
	}
}

func fixtureHd() *handlerdomain.Handler {
	return &handlerdomain.Handler{
		ID: "hd_1", Name: "mailer",
		ActiveVersion: &handlerdomain.Version{Methods: []handlerdomain.MethodSpec{{
			Name: "send", Description: "Send an email.",
			Inputs: []schemapkg.Field{{Name: "to", Type: "string"}},
		}}},
	}
}

// --- tests ------------------------------------------------------------------

// TestResolve_FunctionMount: an fn_ mount becomes a tool named after the function, carrying its
// declared inputs as schema, executing RunFunction bound to its id with TriggeredBy=agent.
//
// TestResolve_FunctionMount：fn_ 挂载成为以 function 命名的工具，带其声明输入作 schema，执行绑定
// 其 id 的 RunFunction、TriggeredBy=agent。
func TestResolve_FunctionMount(t *testing.T) {
	fn := &fakeFn{f: fixtureFn()}
	r := NewResolver(fn, nil, nil)

	tools, err := r.Resolve(context.Background(), []agentdomain.ToolRef{{Ref: "fn_1"}})
	if err != nil || len(tools) != 1 {
		t.Fatalf("resolve: %v (%d tools)", err, len(tools))
	}
	tool := tools[0]
	if tool.Name() != "add_numbers" || tool.Description() != "Adds two numbers." {
		t.Fatalf("identity = %q / %q", tool.Name(), tool.Description())
	}
	var schema struct {
		Properties map[string]struct{ Type string } `json:"properties"`
		Required   []string                         `json:"required"`
	}
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if schema.Properties["a"].Type != "number" || len(schema.Required) != 2 {
		t.Fatalf("schema = %s", tool.Parameters())
	}

	out, err := tool.Execute(context.Background(), `{"a":1,"b":2}`)
	if err != nil || !strings.Contains(out, `"sum":3`) {
		t.Fatalf("execute: %v out=%s", err, out)
	}
	if fn.gotRun.FunctionID != "fn_1" || fn.gotRun.TriggeredBy != functiondomain.TriggeredByAgent {
		t.Fatalf("run input = %+v", fn.gotRun)
	}
	if fn.gotRun.Input["a"] != float64(1) {
		t.Fatalf("args not forwarded: %+v", fn.gotRun.Input)
	}
}

// TestCheckHealth_MixedMounts: per-mount status, no fail-fast — a healthy mount carries its name,
// a deleted target / unknown scheme is reported broken (the rest still evaluated).
func TestCheckHealth_MixedMounts(t *testing.T) {
	fn := &fakeFn{f: fixtureFn()} // fn_1 exists
	r := NewResolver(fn, nil, nil)
	health := r.CheckHealth(context.Background(), []agentdomain.ToolRef{
		{Ref: "fn_1"},     // resolvable
		{Ref: "fn_ghost"}, // deleted/missing target → broken
		{Ref: "xyz_bad"},  // unknown ref scheme → broken
	})
	if len(health) != 3 {
		t.Fatalf("want 3 results (no fail-fast), got %d", len(health))
	}
	if !health[0].Healthy || health[0].Ref != "fn_1" || health[0].Name != "add_numbers" {
		t.Errorf("mount 0 = %+v, want healthy add_numbers", health[0])
	}
	if health[1].Healthy || health[1].Error == "" {
		t.Errorf("mount 1 (missing fn) must be unhealthy with an error: %+v", health[1])
	}
	if health[2].Healthy || health[2].Error == "" {
		t.Errorf("mount 2 (unknown scheme) must be unhealthy with an error: %+v", health[2])
	}
}

// TestResolve_HandlerMount: hd_<id>.<method> becomes <handlerName>__<method> bound to that
// method's spec, calling with TriggeredBy=agent.
//
// TestResolve_HandlerMount：hd_<id>.<method> 成为 <handlerName>__<method>，绑定该 method 的 spec，
// 以 TriggeredBy=agent 调用。
func TestResolve_HandlerMount(t *testing.T) {
	hd := &fakeHd{h: fixtureHd()}
	r := NewResolver(nil, hd, nil)

	tools, err := r.Resolve(context.Background(), []agentdomain.ToolRef{{Ref: "hd_1.send"}})
	if err != nil || len(tools) != 1 {
		t.Fatalf("resolve: %v", err)
	}
	tool := tools[0]
	if tool.Name() != "mailer__send" || tool.Description() != "Send an email." {
		t.Fatalf("identity = %q / %q", tool.Name(), tool.Description())
	}

	out, err := tool.Execute(context.Background(), `{"to":"x@y.z"}`)
	if err != nil || !strings.Contains(out, `"sent":true`) {
		t.Fatalf("execute: %v out=%s", err, out)
	}
	if hd.gotCall.HandlerID != "hd_1" || hd.gotCall.Method != "send" ||
		hd.gotCall.TriggeredBy != handlerdomain.TriggeredByAgent {
		t.Fatalf("call input = %+v", hd.gotCall)
	}

	// 缺 method → HANDLER_METHOD_NOT_FOUND（已有具体码，不造新码）。
	if _, err := r.Resolve(context.Background(), []agentdomain.ToolRef{{Ref: "hd_1.nope"}}); !errors.Is(err, handlerdomain.ErrMethodNotFound) {
		t.Fatalf("missing method err = %v", err)
	}
}

// TestResolve_MCPMount: mcp:server/tool resolves through the live server list to a
// mcp__server__tool (dynamicTool name shape) bound to the server's id.
//
// TestResolve_MCPMount：mcp:server/tool 经在线 server 列表解析为 mcp__server__tool（dynamicTool
// 同名形），绑定 server id。
func TestResolve_MCPMount(t *testing.T) {
	mcp := &fakeMCP{
		servers: []mcpdomain.ServerStatus{{ID: "mcp_1", Name: "search"}},
		tools: []mcpdomain.ToolDef{{
			ServerName: "search", Name: "query", Description: "Search the web.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`),
		}},
	}
	r := NewResolver(nil, nil, mcp)

	tools, err := r.Resolve(context.Background(), []agentdomain.ToolRef{{Ref: "mcp:search/query"}})
	if err != nil || len(tools) != 1 {
		t.Fatalf("resolve: %v", err)
	}
	tool := tools[0]
	if tool.Name() != "mcp__search__query" {
		t.Fatalf("name = %q", tool.Name())
	}
	if _, err := tool.Execute(context.Background(), `{"q":"hi"}`); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if mcp.gotCall.serverID != "mcp_1" || mcp.gotCall.triggeredBy != mcpdomain.CallTriggeredByAgent {
		t.Fatalf("call = %+v", mcp.gotCall)
	}

	// 不在线/不存在的 server → 具体错误，fail-fast。
	if _, err := r.Resolve(context.Background(), []agentdomain.ToolRef{{Ref: "mcp:gone/x"}}); !errors.Is(err, mcpdomain.ErrServerNotFound) {
		t.Fatalf("missing server err = %v", err)
	}
}

// TestResolve_InvalidAndCollision: unknown schemes / malformed refs / synthesized-name
// collisions all fail with AGENT_MOUNT_INVALID — never a silent skip.
//
// TestResolve_InvalidAndCollision：未知 scheme / 坏格式 ref / 合成名撞名，一律 AGENT_MOUNT_INVALID
// 失败——绝不静默跳过。
func TestResolve_InvalidAndCollision(t *testing.T) {
	fn := &fakeFn{f: fixtureFn()}
	r := NewResolver(fn, nil, nil)

	for _, ref := range []string{"weird_1", "hd_1", "mcp:noslash"} {
		if _, err := NewResolver(fn, &fakeHd{h: fixtureHd()}, nil).Resolve(
			context.Background(), []agentdomain.ToolRef{{Ref: ref}}); !errors.Is(err, agentdomain.ErrMountInvalid) {
			t.Fatalf("ref %q err = %v, want ErrMountInvalid", ref, err)
		}
	}

	// 同一 function 挂两次 → 同名工具撞名。
	if _, err := r.Resolve(context.Background(), []agentdomain.ToolRef{{Ref: "fn_1"}, {Ref: "fn_1"}}); !errors.Is(err, agentdomain.ErrMountInvalid) {
		t.Fatalf("collision err = %v, want ErrMountInvalid", err)
	}

	// 目标实体没了 → 具体的 FUNCTION_NOT_FOUND 冒泡（invoke fail-fast）。
	if _, err := r.Resolve(context.Background(), []agentdomain.ToolRef{{Ref: "fn_gone"}}); !errors.Is(err, functiondomain.ErrNotFound) {
		t.Fatalf("gone target err = %v, want functiondomain.ErrNotFound", err)
	}
}
