package bootstrap

import (
	"context"
	stderrors "errors"
	"testing"

	agentdomain "github.com/sunweilin/anselm/backend/internal/domain/agent"
	approvaldomain "github.com/sunweilin/anselm/backend/internal/domain/approval"
	controldomain "github.com/sunweilin/anselm/backend/internal/domain/control"
	functiondomain "github.com/sunweilin/anselm/backend/internal/domain/function"
	handlerdomain "github.com/sunweilin/anselm/backend/internal/domain/handler"
	relationdomain "github.com/sunweilin/anselm/backend/internal/domain/relation"
	triggerdomain "github.com/sunweilin/anselm/backend/internal/domain/trigger"
	workflowdomain "github.com/sunweilin/anselm/backend/internal/domain/workflow"
)

// fakeReaders implements all seven entity read ports with canned data; missing=true makes every
// Get return that entity's NotFound sentinel so the not-found mapping can be exercised.
type fakeReaders struct{ missing bool }

func (f fakeReaders) Get(_ context.Context, _ string) (*functiondomain.Function, error) {
	if f.missing {
		return nil, functiondomain.ErrNotFound
	}
	return &functiondomain.Function{ActiveVersionID: "fnv_1"}, nil
}

type fakeHandler struct{ fakeReaders }

func (fakeHandler) Get(_ context.Context, _ string) (*handlerdomain.Handler, error) {
	return &handlerdomain.Handler{ActiveVersionID: "hdv_1"}, nil
}
func (fakeHandler) GetVersion(_ context.Context, _ string) (*handlerdomain.Version, error) {
	return &handlerdomain.Version{Methods: []handlerdomain.MethodSpec{{Name: "doThing"}, {Name: "other"}}}, nil
}

type fakeAgent struct{ fakeReaders }

func (fakeAgent) Get(_ context.Context, _ string) (*agentdomain.Agent, error) {
	return &agentdomain.Agent{ActiveVersionID: "agv_1"}, nil
}
func (fakeAgent) GetVersion(_ context.Context, _ string) (*agentdomain.Version, error) {
	return &agentdomain.Version{Tools: []agentdomain.ToolRef{{Ref: "fn_mounted"}, {Ref: "hd_mounted.m"}}}, nil
}

type fakeControl struct{ fakeReaders }

func (fakeControl) Get(_ context.Context, _ string) (*controldomain.ControlLogic, error) {
	return &controldomain.ControlLogic{
		ActiveVersionID: "ctlv_1",
		ActiveVersion:   &controldomain.Version{Branches: []controldomain.Branch{{Port: "pass", When: "input.ok"}, {Port: "else", When: "true"}}},
	}, nil
}

type fakeApproval struct{ fakeReaders }

func (fakeApproval) Get(_ context.Context, _ string) (*approvaldomain.ApprovalForm, error) {
	return &approvaldomain.ApprovalForm{ActiveVersionID: "apfv_1"}, nil
}

type fakeTrigger struct{ fakeReaders }

func (fakeTrigger) Get(_ context.Context, _ string) (*triggerdomain.Trigger, error) {
	return &triggerdomain.Trigger{ID: "trg_1"}, nil
}

type fakeMCP struct {
	present bool
	tools   []string // tool names the resolved server exposes (F51)
}

func (f fakeMCP) ResolveServerID(_ context.Context, token string) (string, error) {
	if !f.present {
		return "", stderrors.New("mcp server not found")
	}
	return "mcp_resolved", nil // resolves any token — name OR id — when the server is present
}

func (f fakeMCP) ServerToolNames(_ context.Context, _ string) ([]string, error) { return f.tools, nil }

func TestRefResolver_ResolvesEachKind(t *testing.T) {
	ctx := context.Background()
	r := NewRefResolver(fakeReaders{}, fakeHandler{}, fakeAgent{}, fakeControl{}, fakeApproval{}, fakeTrigger{}, fakeMCP{present: true})

	// fn_ → function + active version.
	if info, err := r.Resolve(ctx, "fn_abc"); err != nil || info.Kind != relationdomain.EntityKindFunction || info.ActiveVersionID != "fnv_1" || !info.HasActiveVersion {
		t.Fatalf("fn resolve: %+v err=%v", info, err)
	}

	// hd_<id>.method → handler; MethodNames from the active version; entity id drops .method.
	info, err := r.Resolve(ctx, "hd_xyz.doThing")
	if err != nil || info.Kind != relationdomain.EntityKindHandler || info.ActiveVersionID != "hdv_1" {
		t.Fatalf("hd resolve: %+v err=%v", info, err)
	}
	if len(info.MethodNames) != 2 || info.MethodNames[0] != "doThing" {
		t.Fatalf("hd method names: %+v", info.MethodNames)
	}

	// ag_ → agent; AgentCallables = its mounted fn_/hd_ refs (for pin recursion).
	info, err = r.Resolve(ctx, "ag_1")
	if err != nil || info.Kind != relationdomain.EntityKindAgent {
		t.Fatalf("ag resolve: %+v err=%v", info, err)
	}
	if len(info.AgentCallables) != 2 || info.AgentCallables[0] != "fn_mounted" {
		t.Fatalf("agent callables: %+v", info.AgentCallables)
	}

	// ctl_ → control; BranchPorts from the active version's branches.
	info, err = r.Resolve(ctx, "ctl_1")
	if err != nil || info.Kind != relationdomain.EntityKindControl {
		t.Fatalf("ctl resolve: %+v err=%v", info, err)
	}
	if len(info.BranchPorts) != 2 || info.BranchPorts[0] != "pass" || info.BranchPorts[1] != "else" {
		t.Fatalf("control branch ports: %+v", info.BranchPorts)
	}

	// apf_ → approval.
	if info, err := r.Resolve(ctx, "apf_1"); err != nil || info.Kind != relationdomain.EntityKindApproval || info.ActiveVersionID != "apfv_1" {
		t.Fatalf("apf resolve: %+v err=%v", info, err)
	}

	// trg_ → trigger: version-less, existence = usable (HasActiveVersion true, empty version).
	if info, err := r.Resolve(ctx, "trg_1"); err != nil || info.Kind != relationdomain.EntityKindTrigger || !info.HasActiveVersion || info.ActiveVersionID != "" {
		t.Fatalf("trg resolve: %+v err=%v", info, err)
	}

	// mcp:<id>/<tool> → mcp: version-less, existence = usable; entity id drops /tool.
	if info, err := r.Resolve(ctx, "mcp:mcp_srv/search"); err != nil || info.Kind != relationdomain.EntityKindMCP || !info.HasActiveVersion {
		t.Fatalf("mcp resolve: %+v err=%v", info, err)
	}
	// F23 regression: the NAME-form ref (what search_blocks/RefHint advertises) must resolve too —
	// ResolveServerID accepts name or id, so the form mounts use no longer fails in workflows.
	if info, err := r.Resolve(ctx, "mcp:markitdown/convert_to_markdown"); err != nil || info.Kind != relationdomain.EntityKindMCP || !info.HasActiveVersion {
		t.Fatalf("mcp name-form resolve: %+v err=%v", info, err)
	}
}

func TestRefResolver_NotFoundAndUnknown(t *testing.T) {
	ctx := context.Background()

	// A missing function maps its NotFound sentinel to ErrRefNotFound (uniform for callers).
	r := NewRefResolver(fakeReaders{missing: true}, fakeHandler{}, fakeAgent{}, fakeControl{}, fakeApproval{}, fakeTrigger{}, fakeMCP{present: true})
	if _, err := r.Resolve(ctx, "fn_gone"); !stderrors.Is(err, workflowdomain.ErrRefNotFound) {
		t.Fatalf("missing fn: want ErrRefNotFound, got %v", err)
	}

	// A missing mcp server (NamesByIDs returns nothing for the id) is also ErrRefNotFound.
	r2 := NewRefResolver(fakeReaders{}, fakeHandler{}, fakeAgent{}, fakeControl{}, fakeApproval{}, fakeTrigger{}, fakeMCP{present: false})
	if _, err := r2.Resolve(ctx, "mcp:mcp_absent/x"); !stderrors.Is(err, workflowdomain.ErrRefNotFound) {
		t.Fatalf("missing mcp: want ErrRefNotFound, got %v", err)
	}

	// An unrecognized prefix is ErrRefNotFound, not a panic.
	if _, err := r2.Resolve(ctx, "wat_123"); !stderrors.Is(err, workflowdomain.ErrRefNotFound) {
		t.Fatalf("unknown prefix: want ErrRefNotFound, got %v", err)
	}
}
