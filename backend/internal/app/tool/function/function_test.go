package function

import (
	"testing"

	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
)

// TestFunctionTools_Wiring asserts the 9 tools are constructed with the expected names.
func TestFunctionTools_Wiring(t *testing.T) {
	tools := FunctionTools(nil, nil, nil) // nil svc OK: we only inspect Name() here
	want := map[string]bool{
		"search_function": false, "get_function": false, "create_function": false,
		"edit_function": false, "revert_function": false, "delete_function": false,
		"run_function": false, "search_function_executions": false, "get_function_execution": false,
	}
	if len(tools) != len(want) {
		t.Fatalf("want %d tools, got %d", len(want), len(tools))
	}
	for _, tl := range tools {
		if _, ok := want[tl.Name()]; !ok {
			t.Fatalf("unexpected tool name %q", tl.Name())
		}
		want[tl.Name()] = true
		var _ toolapp.Tool = tl // every tool satisfies the 5-method interface
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("missing tool %q", name)
		}
	}
}

func TestValidateInput_RequiredFields(t *testing.T) {
	cases := []struct {
		name    string
		tool    toolapp.Tool
		args    string
		wantErr bool
	}{
		{"create empty ops", &CreateFunction{}, `{"ops":[]}`, true},
		{"create with ops", &CreateFunction{}, `{"ops":[{"op":"set_meta","name":"a"}]}`, false},
		{"edit no id", &EditFunction{}, `{"ops":[]}`, true},
		{"edit with id", &EditFunction{}, `{"functionId":"fn_1","ops":[]}`, false},
		{"get no id", &GetFunction{}, `{}`, true},
		{"get with id", &GetFunction{}, `{"functionId":"fn_1"}`, false},
		{"run no id", &RunFunction{}, `{"args":{}}`, true},
		{"run with id", &RunFunction{}, `{"functionId":"fn_1","args":{}}`, false},
		{"revert no id", &RevertFunction{}, `{"version":1}`, true},
		{"revert bad version", &RevertFunction{}, `{"functionId":"fn_1","version":0}`, true},
		{"revert ok", &RevertFunction{}, `{"functionId":"fn_1","version":2}`, false},
		{"delete no id", &DeleteFunction{}, `{}`, true},
		{"search exec no id", &SearchFunctionExecutions{}, `{}`, true},
		{"get exec no id", &GetFunctionExecution{}, `{}`, true},
		{"search any", &SearchFunction{}, `{}`, false},
	}
	for _, c := range cases {
		err := c.tool.ValidateInput([]byte(c.args))
		if (err != nil) != c.wantErr {
			t.Errorf("%s: ValidateInput(%s) err=%v, wantErr=%v", c.name, c.args, err, c.wantErr)
		}
	}
}
