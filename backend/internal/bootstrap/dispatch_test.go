package bootstrap

import (
	"context"
	"encoding/json"
	"testing"

	agentapp "github.com/sunweilin/anselm/backend/internal/app/agent"
	functionapp "github.com/sunweilin/anselm/backend/internal/app/function"
	handlerapp "github.com/sunweilin/anselm/backend/internal/app/handler"
	functiondomain "github.com/sunweilin/anselm/backend/internal/domain/function"
)

// fakeCallables records what each port received so the routing + arg mapping can be asserted, and
// returns canned results to exercise the coercion.
type fakeCallables struct {
	fnIn   functionapp.RunInput
	fnRes  *functiondomain.ExecutionResult
	hdIn   handlerapp.CallInput
	hdOut  any
	mcpSrv string
	mcpTl  string
	mcpRaw json.RawMessage
	mcpOut string
	agIn   agentapp.InvokeInput
	agRes  *agentapp.InvokeResult
}

func (f *fakeCallables) RunFunction(_ context.Context, in functionapp.RunInput) (*functiondomain.ExecutionResult, error) {
	f.fnIn = in
	return f.fnRes, nil
}
func (f *fakeCallables) Call(_ context.Context, in handlerapp.CallInput) (any, error) {
	f.hdIn = in
	return f.hdOut, nil
}
func (f *fakeCallables) ResolveServerID(_ context.Context, token string) (string, error) {
	return token, nil // identity: the dispatch test feeds an id-form ref; name->id resolution is covered in mcp pkg
}
func (f *fakeCallables) CallTool(_ context.Context, serverID, tool string, args json.RawMessage, _ string) (string, error) {
	f.mcpSrv, f.mcpTl, f.mcpRaw = serverID, tool, args
	return f.mcpOut, nil
}
func (f *fakeCallables) InvokeAgent(_ context.Context, in agentapp.InvokeInput) (*agentapp.InvokeResult, error) {
	f.agIn = in
	return f.agRes, nil
}

func TestDispatcher_RoutesAndMapsArgs(t *testing.T) {
	ctx := context.Background()
	f := &fakeCallables{
		fnRes:  &functiondomain.ExecutionResult{OK: true, Output: map[string]any{"text": "drafted"}},
		hdOut:  map[string]any{"rows": 3},
		mcpOut: "tool said hi",
		agRes:  &agentapp.InvokeResult{OK: true, Output: map[string]any{"score": 0.9}},
	}
	d := NewDispatcher(f, f, f, f)

	// fn_ → RunFunction(FunctionID=ref, VersionID=pin, TriggeredBy=workflow); map output passes through flat.
	out, err := d.RunAction(ctx, "fn_abc", "fnv_pin1", map[string]any{"topic": "go"})
	if err != nil {
		t.Fatalf("RunAction fn: %v", err)
	}
	if f.fnIn.FunctionID != "fn_abc" || f.fnIn.VersionID != "fnv_pin1" || f.fnIn.TriggeredBy != functiondomain.TriggeredByWorkflow {
		t.Fatalf("fn input mis-mapped: %+v", f.fnIn)
	}
	if f.fnIn.Input["topic"] != "go" {
		t.Fatalf("fn args not forwarded: %+v", f.fnIn.Input)
	}
	if out["text"] != "drafted" {
		t.Fatalf("fn result not flat: %+v", out)
	}

	// hd_<id>.method → Call(HandlerID, Method split on first dot).
	if _, err := d.RunAction(ctx, "hd_xyz.doThing", "hdv_pin", map[string]any{"k": 1}); err != nil {
		t.Fatalf("RunAction hd: %v", err)
	}
	if f.hdIn.HandlerID != "hd_xyz" || f.hdIn.Method != "doThing" {
		t.Fatalf("hd ref mis-split: id=%q method=%q", f.hdIn.HandlerID, f.hdIn.Method)
	}

	// mcp:<serverId>/<tool> → CallTool(server, tool); args JSON-marshaled.
	if _, err := d.RunAction(ctx, "mcp:mcp_srv/search", "", map[string]any{"q": "x"}); err != nil {
		t.Fatalf("RunAction mcp: %v", err)
	}
	if f.mcpSrv != "mcp_srv" || f.mcpTl != "search" {
		t.Fatalf("mcp ref mis-split: server=%q tool=%q", f.mcpSrv, f.mcpTl)
	}
	var gotArgs map[string]any
	if err := json.Unmarshal(f.mcpRaw, &gotArgs); err != nil || gotArgs["q"] != "x" {
		t.Fatalf("mcp args not marshaled: %s (%v)", f.mcpRaw, err)
	}

	// ag_ → InvokeAgent(AgentID=ref, VersionID=pin, TriggeredBy=workflow); map output passes through.
	agOut, err := d.RunAgent(ctx, "ag_001", "agv_pin1", map[string]any{"task": "review"})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if f.agIn.AgentID != "ag_001" || f.agIn.VersionID != "agv_pin1" || f.agIn.TriggeredBy != "workflow" {
		t.Fatalf("agent input mis-mapped: %+v", f.agIn)
	}
	if agOut["score"] != 0.9 {
		t.Fatalf("agent result not flat: %+v", agOut)
	}
}

func TestDispatcher_CoercesScalarToText(t *testing.T) {
	ctx := context.Background()
	// Non-map returns (mcp text, a handler returning a scalar) wrap under "text".
	f := &fakeCallables{hdOut: "plain string", mcpOut: "raw text"}
	d := NewDispatcher(f, f, f, f)

	hd, err := d.RunAction(ctx, "hd_h.m", "", nil)
	if err != nil {
		t.Fatalf("hd: %v", err)
	}
	if hd["text"] != "plain string" {
		t.Fatalf("handler scalar not wrapped under text: %+v", hd)
	}

	mc, err := d.RunAction(ctx, "mcp:s/t", "", nil)
	if err != nil {
		t.Fatalf("mcp: %v", err)
	}
	if mc["text"] != "raw text" {
		t.Fatalf("mcp text not wrapped under text: %+v", mc)
	}
}

func TestDispatcher_FailuresAndBadRefs(t *testing.T) {
	ctx := context.Background()
	f := &fakeCallables{
		fnRes: &functiondomain.ExecutionResult{OK: false, ErrorMsg: "boom"},
		agRes: &agentapp.InvokeResult{OK: false, ErrorMsg: "agent boom"},
	}
	d := NewDispatcher(f, f, f, f)

	// OK=false fail-fasts as an error (node row → failed).
	if _, err := d.RunAction(ctx, "fn_x", "", nil); err == nil {
		t.Fatal("expected error for function OK=false")
	}
	if _, err := d.RunAgent(ctx, "ag_x", "", nil); err == nil {
		t.Fatal("expected error for agent OK=false")
	}

	// Malformed / unknown refs are rejected, not silently mis-routed.
	for _, ref := range []string{"hd_nomethod", "mcp:notool", "ctl_x", "weird"} {
		if _, err := d.RunAction(ctx, ref, "", nil); err == nil {
			t.Fatalf("expected error for bad action ref %q", ref)
		}
	}
	if _, err := d.RunAgent(ctx, "fn_notanagent", "", nil); err == nil {
		t.Fatal("expected error for non-ag_ ref in RunAgent")
	}
}
