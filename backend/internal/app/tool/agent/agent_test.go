package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	agentapp "github.com/sunweilin/anselm/backend/internal/app/agent"
	agentdomain "github.com/sunweilin/anselm/backend/internal/domain/agent"
)

func TestAgentTools_NamesAndCount(t *testing.T) {
	tools := AgentTools(nil, nil, nil) // Name() does not deref svc
	want := map[string]bool{
		"search_agent": true, "get_agent": true, "create_agent": true, "edit_agent": true,
		"revert_agent": true, "delete_agent": true, "invoke_agent": true,
		"search_agent_executions": true, "get_agent_execution": true, "update_agent_meta": true,
	}
	if len(tools) != len(want) {
		t.Fatalf("want %d tools, got %d", len(want), len(tools))
	}
	for _, tl := range tools {
		if !want[tl.Name()] {
			t.Fatalf("unexpected tool %q", tl.Name())
		}
		delete(want, tl.Name())
	}
	if len(want) != 0 {
		t.Fatalf("missing tools: %v", want)
	}
}

func TestCreateAgent_ValidateInput(t *testing.T) {
	tl := &CreateAgent{}
	if err := tl.ValidateInput(json.RawMessage(`{"name":"judge","prompt":"p"}`)); err != nil {
		t.Fatalf("valid args rejected: %v", err)
	}
	if err := tl.ValidateInput(json.RawMessage(`{"name":"judge"}`)); err == nil {
		t.Fatal("missing prompt should fail")
	}
	if err := tl.ValidateInput(json.RawMessage(`{"prompt":"p"}`)); err == nil {
		t.Fatal("missing name should fail")
	}
}

func TestInvokeAgent_RequiresAgentID(t *testing.T) {
	tl := &InvokeAgent{}
	if err := tl.ValidateInput(json.RawMessage(`{}`)); err == nil {
		t.Fatal("missing agentId should fail")
	}
	// input is now required: a missing/misnamed task (e.g. a "prompt" key) must fail loudly rather
	// than run the agent with empty input and return a misleading ok:true. {} is allowed.
	if err := tl.ValidateInput(json.RawMessage(`{"agentId":"ag_1"}`)); err == nil {
		t.Fatal("missing input should fail")
	}
	if err := tl.ValidateInput(json.RawMessage(`{"agentId":"ag_1","input":{}}`)); err != nil {
		t.Fatalf("valid args (agentId + input) rejected: %v", err)
	}
}

// TestConfigProps_AgentChainRedirect — F31: forbidding ag_ refs must point the agent at the real
// composition path (a workflow agent node), so it doesn't burn a turn hand-rolling an HTTP wrapper.
func TestConfigProps_AgentChainRedirect(t *testing.T) {
	if !strings.Contains(configProps, "workflow with an agent node") {
		t.Fatalf("the tools-field desc must redirect agent-chaining to the workflow path")
	}
}

// TestEditAgent_ValidateInput_NoPromptRequired — edit_agent now MERGES, so a partial edit overlays
// only provided fields; agentId alone is valid (prompt no longer required), missing agentId still fails.
func TestEditAgent_ValidateInput_NoPromptRequired(t *testing.T) {
	tl := &EditAgent{}
	if err := tl.ValidateInput(json.RawMessage(`{"agentId":"ag_1","tools":[]}`)); err != nil {
		t.Fatalf("agentId-only partial edit must be valid: %v", err)
	}
	if err := tl.ValidateInput(json.RawMessage(`{"tools":[]}`)); err == nil {
		t.Fatal("missing agentId should fail")
	}
}

// TestEditAgent_RejectsMetaFields — F171: name/description/tags are NOT in edit_agent's versioned config
// (they live on the agent row, changed via update_agent_meta). edit_agent must REJECT them loudly with a
// pointer, not silently swallow them (it used to return success with the meta change lost).
func TestEditAgent_RejectsMetaFields(t *testing.T) {
	tl := &EditAgent{}
	for _, args := range []string{
		`{"agentId":"ag_1","tags":["demo"]}`,
		`{"agentId":"ag_1","name":"renamed"}`,
		`{"agentId":"ag_1","description":"new desc"}`,
	} {
		if err := tl.ValidateInput(json.RawMessage(args)); !errors.Is(err, ErrAgentMetaNotInEdit) {
			t.Fatalf("edit_agent must reject meta field in %s, got %v", args, err)
		}
	}
	// A real config edit (prompt) is still fine.
	if err := tl.ValidateInput(json.RawMessage(`{"agentId":"ag_1","prompt":"new prompt"}`)); err != nil {
		t.Fatalf("a config-only edit must pass, got %v", err)
	}
}

// TestMergeConfig_PreservesOmittedClearsExplicit — the heart of the edit_agent merge fix: a prompt-only
// edit must KEEP the agent's mounted skill/knowledge/tools (the old full-replace silently wiped them at a
// measured ~40% drop rate), while an explicitly-empty field still clears it.
func TestMergeConfig_PreservesOmittedClearsExplicit(t *testing.T) {
	current := agentapp.Config{
		Prompt:    "old prompt",
		Skill:     "reviewer",
		Knowledge: []string{"doc_1"},
		Tools:     []agentdomain.ToolRef{{Ref: "fn_1"}},
	}
	// Prompt-only edit: prompt changes, everything else PRESERVED.
	got := mergeConfig(current, []byte(`{"agentId":"ag_1","prompt":"new prompt","changeReason":"tweak"}`))
	if got.Prompt != "new prompt" {
		t.Fatalf("prompt must update, got %q", got.Prompt)
	}
	if got.Skill != "reviewer" || len(got.Knowledge) != 1 || len(got.Tools) != 1 {
		t.Fatalf("omitted skill/knowledge/tools must be PRESERVED, got %+v", got)
	}
	if got.ChangeReason != "tweak" {
		t.Fatalf("changeReason must apply, got %q", got.ChangeReason)
	}
	// An explicitly-empty field still clears it (a provided value wins, even when empty).
	cleared := mergeConfig(current, []byte(`{"agentId":"ag_1","tools":[]}`))
	if len(cleared.Tools) != 0 {
		t.Fatalf("explicit empty tools must clear, got %+v", cleared.Tools)
	}
	if cleared.Prompt != "old prompt" || len(cleared.Knowledge) != 1 {
		t.Fatalf("non-tools fields must stay preserved, got %+v", cleared)
	}
}
