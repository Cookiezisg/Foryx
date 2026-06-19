package model

import (
	"encoding/json"
	"testing"
)

// TestGetModelConfig_Contract — F68: lock the read-only config tool's contract (Execute itself is
// verified end-to-end on a live backend: the agent calls it + gets the real masked config, no FS grep).
func TestGetModelConfig_Contract(t *testing.T) {
	tools := ModelConfigTools(nil, nil, nil)
	if len(tools) != 1 {
		t.Fatalf("ModelConfigTools should return 1 tool, got %d", len(tools))
	}
	tool := tools[0]
	if tool.Name() != "get_model_config" {
		t.Fatalf("name = %q, want get_model_config", tool.Name())
	}
	var schema map[string]any
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Fatalf("Parameters not valid JSON: %v", err)
	}
	if _, hasRequired := schema["required"]; hasRequired {
		t.Errorf("get_model_config is a no-arg read tool; should declare no required params")
	}
	if err := tool.ValidateInput(json.RawMessage(`{}`)); err != nil {
		t.Errorf("ValidateInput({}) = %v, want nil", err)
	}
}
