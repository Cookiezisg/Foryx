package handler

import (
	"testing"

	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
)

func TestHandlerTools_Wiring(t *testing.T) {
	tools := HandlerTools(nil, nil, nil)
	want := map[string]bool{
		"search_handler": false, "get_handler": false, "create_handler": false,
		"edit_handler": false, "revert_handler": false, "delete_handler": false,
		"call_handler": false, "update_handler_config": false, "restart_handler": false,
		"search_handler_calls": false, "get_handler_call": false,
	}
	if len(tools) != len(want) {
		t.Fatalf("want %d tools, got %d", len(want), len(tools))
	}
	for _, tl := range tools {
		if _, ok := want[tl.Name()]; !ok {
			t.Fatalf("unexpected tool name %q", tl.Name())
		}
		want[tl.Name()] = true
		var _ toolapp.Tool = tl
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
		{"create empty ops", &CreateHandler{}, `{"ops":[]}`, true},
		{"create with ops", &CreateHandler{}, `{"ops":[{"op":"set_meta","name":"a"}]}`, false},
		{"edit no id", &EditHandler{}, `{"ops":[]}`, true},
		{"get no id", &GetHandler{}, `{}`, true},
		{"call no id", &CallHandler{}, `{"method":"m","args":{}}`, true},
		{"call no method", &CallHandler{}, `{"handlerId":"hd_1","args":{}}`, true},
		{"call ok", &CallHandler{}, `{"handlerId":"hd_1","method":"m","args":{}}`, false},
		{"revert bad version", &RevertHandler{}, `{"handlerId":"hd_1","version":0}`, true},
		{"revert ok", &RevertHandler{}, `{"handlerId":"hd_1","version":2}`, false},
		{"delete no id", &DeleteHandler{}, `{}`, true},
		{"restart no id", &RestartHandler{}, `{}`, true},
		{"restart ok", &RestartHandler{}, `{"handlerId":"hd_1"}`, false},
		{"update_config no id", &UpdateHandlerConfig{}, `{"config":{}}`, true},
		{"search_calls no id", &SearchHandlerCalls{}, `{}`, true},
		{"get_call no id", &GetHandlerCall{}, `{}`, true},
		{"search any", &SearchHandler{}, `{}`, false},
	}
	for _, c := range cases {
		err := c.tool.ValidateInput([]byte(c.args))
		if (err != nil) != c.wantErr {
			t.Errorf("%s: ValidateInput(%s) err=%v, wantErr=%v", c.name, c.args, err, c.wantErr)
		}
	}
}
