package toolset

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// stubTool is a minimal Tool for test grouping without pulling in real deps.
type stubTool struct{ name string }

func (s *stubTool) Name() string                                                      { return s.name }
func (s *stubTool) Description() string                                               { return "" }
func (s *stubTool) Parameters() json.RawMessage                                       { return json.RawMessage(`{"type":"object","properties":{}}`) }
func (s *stubTool) IsReadOnly() bool                                                  { return true }
func (s *stubTool) NeedsReadFirst() bool                                              { return false }
func (s *stubTool) RequiresWorkspace() bool                                           { return false }
func (s *stubTool) ValidateInput(_ json.RawMessage) error                             { return nil }
func (s *stubTool) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}
func (s *stubTool) Execute(_ context.Context, _ string) (string, error) { return "", nil }

func makeTestToolset() toolapp.Toolset {
	return toolapp.Toolset{
		Resident: []toolapp.Tool{&stubTool{"Read"}},
		Lazy: map[string][]toolapp.Tool{
			"function": {&stubTool{"create_function"}, &stubTool{"edit_function"}},
			"handler":  {&stubTool{"create_handler"}},
		},
	}
}

func ctxWithState() (context.Context, *agentstatepkg.AgentState) {
	state := &agentstatepkg.AgentState{}
	ctx := reqctxpkg.WithAgentState(context.Background(), state)
	return ctx, state
}

func TestActivateTools_ValidateInput_ValidCategory(t *testing.T) {
	at := NewActivateTools(makeTestToolset())
	for _, cat := range ValidCategories {
		arg, _ := json.Marshal(map[string]string{"category": cat})
		if err := at.ValidateInput(arg); err != nil {
			t.Errorf("ValidateInput(%q) = %v, want nil", cat, err)
		}
	}
}

func TestActivateTools_ValidateInput_RejectsInvalidCategory(t *testing.T) {
	at := NewActivateTools(makeTestToolset())
	for _, bad := range []string{"", "unknown", "Function", "HANDLER", "all"} {
		arg, _ := json.Marshal(map[string]string{"category": bad})
		if err := at.ValidateInput(arg); err == nil {
			t.Errorf("ValidateInput(%q) = nil, want error", bad)
		}
	}
}

func TestActivateTools_Execute_ActivatesGroupInState(t *testing.T) {
	at := NewActivateTools(makeTestToolset())
	ctx, state := ctxWithState()

	args, _ := json.Marshal(map[string]string{"category": "function"})
	_, _ = at.Execute(ctx, string(args))

	groups := state.ActivatedGroups()
	if len(groups) != 1 || groups[0] != "function" {
		t.Errorf("AgentState.ActivatedGroups() = %v, want [function]", groups)
	}
}

func TestActivateTools_Execute_ReturnsToolNames(t *testing.T) {
	at := NewActivateTools(makeTestToolset())
	ctx, _ := ctxWithState()

	args, _ := json.Marshal(map[string]string{"category": "function"})
	out, err := at.Execute(ctx, string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "create_function") || !strings.Contains(out, "edit_function") {
		t.Errorf("Execute output %q missing expected tool names", out)
	}
	if !strings.HasPrefix(out, "Activated function:") {
		t.Errorf("Execute output %q missing 'Activated function:' prefix", out)
	}
}

func TestActivateTools_Execute_NoStateInCtx_DoesNotPanic(t *testing.T) {
	// Ctx without AgentState must not panic — Execute degrades gracefully.
	//
	// 无 AgentState 的 ctx 不得 panic，Execute 优雅降级。
	at := NewActivateTools(makeTestToolset())
	args, _ := json.Marshal(map[string]string{"category": "handler"})
	out, err := at.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Execute without state: %v", err)
	}
	if !strings.Contains(out, "handler") {
		t.Errorf("Execute output %q missing 'handler'", out)
	}
}

func TestHostToolsFullSet_ViaToolset(t *testing.T) {
	// Toolset.All() must include resident + all lazy tools (T8 unchanged guarantee).
	//
	// Toolset.All() 须包含 resident + 所有 lazy 工具（T8 前行为不变保证）。
	ts := makeTestToolset()
	all := ts.All()
	nameSet := make(map[string]bool, len(all))
	for _, tool := range all {
		nameSet[tool.Name()] = true
	}
	for _, want := range []string{"Read", "create_function", "edit_function", "create_handler"} {
		if !nameSet[want] {
			t.Errorf("Toolset.All() missing %q", want)
		}
	}
}
