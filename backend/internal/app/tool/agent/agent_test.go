package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentTools_NamesAndCount(t *testing.T) {
	tools := AgentTools(nil, nil, nil) // Name() does not deref svc
	want := map[string]bool{
		"search_agent": true, "get_agent": true, "create_agent": true, "edit_agent": true,
		"revert_agent": true, "delete_agent": true, "invoke_agent": true,
		"search_agent_executions": true, "get_agent_execution": true,
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
