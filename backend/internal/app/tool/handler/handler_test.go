package handler

import (
	"testing"

	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	handlerdomain "github.com/sunweilin/anselm/backend/internal/domain/handler"
)

// TestBuildOutput_SurfacesRuntimeState — F-handler-broken-init-outage (round-8): edit_handler must
// surface the post-edit runtimeState (+ a warning when not running) so a broken __init__ that builds
// the env fine but fails to spawn doesn't read as a clean "successful" edit. Create (runtimeState="")
// stays silent — a fresh handler not running is expected, not a bricking.
func TestBuildOutput_SurfacesRuntimeState(t *testing.T) {
	v := &handlerdomain.Version{ID: "hdv_1", Version: 2, EnvStatus: "ready"}

	running := buildOutput("hd_1", v, 1, nil, handlerdomain.RuntimeStateRunning, false)
	if running["runtimeState"] != handlerdomain.RuntimeStateRunning {
		t.Fatalf("running edit must report runtimeState, got %+v", running)
	}
	if _, hasWarn := running["runtimeWarning"]; hasWarn {
		t.Fatalf("a running instance must NOT carry a warning, got %+v", running)
	}

	broken := buildOutput("hd_1", v, 1, nil, handlerdomain.RuntimeStateStopped, false)
	if broken["runtimeState"] != handlerdomain.RuntimeStateStopped {
		t.Fatalf("broken edit must report runtimeState=stopped, got %+v", broken)
	}
	if _, hasWarn := broken["runtimeWarning"]; !hasWarn {
		t.Fatalf("a not-running instance after edit MUST carry a warning (else the brick is silent), got %+v", broken)
	}

	created := buildOutput("hd_1", v, 1, nil, "", false)
	if _, has := created["runtimeState"]; has {
		t.Fatalf("create (runtimeState=\"\") must stay silent on runtime state, got %+v", created)
	}
}

// TestBuildOutput_EmptyOpsRestartIsVisible — F140: an empty-ops edit_handler rebuilds the env and
// restarts the resident instance (wiping in-memory state) but applies no ops and mints no version —
// it must NOT read as a no-op. The result carries restarted:true + a note so the state wipe is visible.
func TestBuildOutput_EmptyOpsRestartIsVisible(t *testing.T) {
	v := &handlerdomain.Version{ID: "hdv_1", Version: 2, EnvStatus: "ready"}

	restarted := buildOutput("hd_1", v, 0, nil, handlerdomain.RuntimeStateRunning, true)
	if restarted["restarted"] != true {
		t.Fatalf("empty-ops restart must surface restarted:true (else it reads as a no-op), got %+v", restarted)
	}
	if _, has := restarted["restartNote"]; !has {
		t.Fatalf("a restart must carry a note that in-memory state was wiped, got %+v", restarted)
	}

	normal := buildOutput("hd_1", v, 2, nil, handlerdomain.RuntimeStateRunning, false)
	if _, has := normal["restarted"]; has {
		t.Fatalf("a normal op-applying edit must NOT flag restarted (the version bump already signals change), got %+v", normal)
	}
}

func TestHandlerTools_Wiring(t *testing.T) {
	tools := HandlerTools(nil, nil, nil)
	want := map[string]bool{
		"search_handler": false, "get_handler": false, "create_handler": false,
		"edit_handler": false, "revert_handler": false, "delete_handler": false,
		"call_handler": false, "update_handler_config": false, "restart_handler": false,
		"search_handler_calls": false, "get_handler_call": false, "update_handler_meta": false,
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
