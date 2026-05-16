package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"go.uber.org/zap/zaptest"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func TestSanitizeToolErr_StripsSinglePrefix(t *testing.T) {
	err := fmt.Errorf("Read.ValidateInput: %w", errors.New("file_path is required"))
	got := sanitizeToolErr(err)
	want := "file_path is required"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSanitizeToolErr_StripsMultiLayerChain(t *testing.T) {
	err := fmt.Errorf("subagent.Spawn: %w",
		fmt.Errorf("subagentapp.runReact: %w",
			fmt.Errorf("llm.Generate: %w",
				errors.New("deepseek api: 401 unauthorized"))))
	got := sanitizeToolErr(err)
	want := "deepseek api: 401 unauthorized"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSanitizeToolErr_NilReturnsEmpty(t *testing.T) {
	if got := sanitizeToolErr(nil); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestSanitizeToolErr_NoPrefixUnchanged(t *testing.T) {
	err := errors.New("permission denied")
	if got := sanitizeToolErr(err); got != "permission denied" {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestSanitizeToolErr_KeepsInnermostColons(t *testing.T) {
	err := fmt.Errorf("Tool.Execute: %w", errors.New("HTTP 502: upstream unreachable"))
	got := sanitizeToolErr(err)
	want := "HTTP 502: upstream unreachable"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

type alwaysDenyTool struct {
	name       string
	executed   bool
	permChecks int
}

func (t *alwaysDenyTool) Name() string                                 { return t.name }
func (t *alwaysDenyTool) Description() string                          { return "stub" }
func (t *alwaysDenyTool) Parameters() json.RawMessage                  { return json.RawMessage(`{"type":"object"}`) }
func (t *alwaysDenyTool) IsReadOnly() bool                             { return true }
func (t *alwaysDenyTool) NeedsReadFirst() bool                         { return false }
func (t *alwaysDenyTool) RequiresWorkspace() bool                      { return false }
func (t *alwaysDenyTool) ValidateInput(json.RawMessage) error          { return nil }
func (t *alwaysDenyTool) CheckPermissions(json.RawMessage, toolapp.PermissionMode) toolapp.PermissionResult {
	t.permChecks++
	return toolapp.PermissionDeny
}
func (t *alwaysDenyTool) Execute(_ context.Context, _ string) (string, error) {
	t.executed = true
	return "executed", nil
}

func TestExecuteTool_NoActiveSkill_HonorsCheckPermissions(t *testing.T) {
	stub := &alwaysDenyTool{name: "Bash"}
	log := zaptest.NewLogger(t)
	output, errMsg, ok := executeTool(context.Background(), stub, "Bash",
		json.RawMessage(`{"command":"git status"}`), log)

	if ok {
		t.Errorf("ok=true; expected false (CheckPermissions denied)")
	}
	if stub.executed {
		t.Errorf("Execute ran despite Deny")
	}
	if stub.permChecks != 1 {
		t.Errorf("permChecks = %d, want 1", stub.permChecks)
	}
	if errMsg != "permission denied" {
		t.Errorf("errMsg = %q, want %q", errMsg, "permission denied")
	}
	if output != "permission denied for this call" {
		t.Errorf("output = %q", output)
	}
}

func TestExecuteTool_ActiveSkillPreApproves_BypassesCheckPermissions(t *testing.T) {
	stub := &alwaysDenyTool{name: "Bash"}
	log := zaptest.NewLogger(t)

	state := &agentstatepkg.AgentState{}
	state.SetActiveSkill(&skilldomain.Skill{
		Name: "deploy",
		Frontmatter: skilldomain.Frontmatter{
			AllowedTools: []string{"Bash(git *)"},
		},
	})
	ctx := reqctxpkg.WithAgentState(context.Background(), state)

	output, errMsg, ok := executeTool(ctx, stub, "Bash",
		json.RawMessage(`{"command":"git status"}`), log)

	if !ok {
		t.Errorf("ok=false; expected true (pre-approved by active skill)")
	}
	if !stub.executed {
		t.Errorf("Execute did not run despite pre-approval")
	}
	if stub.permChecks != 0 {
		t.Errorf("permChecks = %d, want 0 (pre-approval must skip CheckPermissions entirely)",
			stub.permChecks)
	}
	if errMsg != "" {
		t.Errorf("errMsg = %q, want empty", errMsg)
	}
	if output != "executed" {
		t.Errorf("output = %q, want 'executed'", output)
	}
}

func TestExecuteTool_ActiveSkillNoMatch_FallsBackToCheckPermissions(t *testing.T) {
	stub := &alwaysDenyTool{name: "Read"}
	log := zaptest.NewLogger(t)

	state := &agentstatepkg.AgentState{}
	state.SetActiveSkill(&skilldomain.Skill{
		Name: "deploy",
		Frontmatter: skilldomain.Frontmatter{
			AllowedTools: []string{"Bash"},
		},
	})
	ctx := reqctxpkg.WithAgentState(context.Background(), state)

	_, _, ok := executeTool(ctx, stub, "Read", json.RawMessage(`{}`), log)
	if ok {
		t.Errorf("ok=true; expected CheckPermissions still gates non-listed tool")
	}
	if stub.permChecks != 1 {
		t.Errorf("permChecks = %d, want 1 (fallback to CheckPermissions)", stub.permChecks)
	}
}
