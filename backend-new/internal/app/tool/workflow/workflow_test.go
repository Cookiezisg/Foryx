package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	workflowstore "github.com/sunweilin/forgify/backend/internal/infra/store/workflow"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// TestWorkflowTools_Wiring asserts all 12 tools are constructed with the expected names and
// each satisfies the 5-method Tool interface: 7 forge/query + 5 execution-lifecycle (D1).
func TestWorkflowTools_Wiring(t *testing.T) {
	tools := WorkflowTools(nil) // nil svc OK: we only inspect Name() here
	want := map[string]bool{
		"search_workflow": false, "get_workflow": false, "create_workflow": false,
		"edit_workflow": false, "revert_workflow": false, "delete_workflow": false,
		"capability_check_workflow": false,
		// execution lifecycle (D1)
		"trigger_workflow": false, "stage_workflow": false, "activate_workflow": false,
		"deactivate_workflow": false, "kill_workflow": false,
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
		{"create no name", &CreateWorkflow{}, `{"ops":[{"op":"add_node"}]}`, true},
		{"create no ops", &CreateWorkflow{}, `{"name":"a"}`, true},
		{"create ok", &CreateWorkflow{}, `{"name":"a","ops":[{"op":"add_node"}]}`, false},
		{"edit no id", &EditWorkflow{}, `{"ops":[{"op":"add_node"}]}`, true},
		{"edit no ops", &EditWorkflow{}, `{"workflowId":"wf_1","ops":[]}`, true},
		{"edit ok", &EditWorkflow{}, `{"workflowId":"wf_1","ops":[{"op":"add_node"}]}`, false},
		{"get no id", &GetWorkflow{}, `{}`, true},
		{"get ok", &GetWorkflow{}, `{"workflowId":"wf_1"}`, false},
		{"revert no id", &RevertWorkflow{}, `{"version":1}`, true},
		{"revert bad version", &RevertWorkflow{}, `{"workflowId":"wf_1","version":0}`, true},
		{"revert ok", &RevertWorkflow{}, `{"workflowId":"wf_1","version":2}`, false},
		{"delete no id", &DeleteWorkflow{}, `{}`, true},
		{"delete ok", &DeleteWorkflow{}, `{"workflowId":"wf_1"}`, false},
		{"capcheck no id", &CapabilityCheckWorkflow{}, `{}`, true},
		{"capcheck ok", &CapabilityCheckWorkflow{}, `{"workflowId":"wf_1"}`, false},
		{"search any", &SearchWorkflow{}, `{}`, false},
		// execution lifecycle (D1)
		{"trigger no id", &TriggerWorkflow{}, `{"payload":{}}`, true},
		{"trigger ok", &TriggerWorkflow{}, `{"workflowId":"wf_1","payload":{"x":1}}`, false},
		{"trigger ok no payload", &TriggerWorkflow{}, `{"workflowId":"wf_1"}`, false},
		{"stage no id", &StageWorkflow{}, `{}`, true},
		{"stage ok", &StageWorkflow{}, `{"workflowId":"wf_1"}`, false},
		{"activate no id", &ActivateWorkflow{}, `{}`, true},
		{"activate ok", &ActivateWorkflow{}, `{"workflowId":"wf_1"}`, false},
		{"deactivate no id", &DeactivateWorkflow{}, `{}`, true},
		{"deactivate ok", &DeactivateWorkflow{}, `{"workflowId":"wf_1"}`, false},
		{"kill no id", &KillWorkflow{}, `{}`, true},
		{"kill ok", &KillWorkflow{}, `{"workflowId":"wf_1"}`, false},
	}
	for _, c := range cases {
		err := c.tool.ValidateInput([]byte(c.args))
		if (err != nil) != c.wantErr {
			t.Errorf("%s: ValidateInput(%s) err=%v, wantErr=%v", c.name, c.args, err, c.wantErr)
		}
	}
}

func newSvc(t *testing.T) (*workflowapp.Service, context.Context) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, stmt := range workflowstore.Schema {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	svc := workflowapp.NewService(workflowstore.New(ormpkg.Open(sqlDB)), nil, nil, zap.NewNop())
	return svc, reqctxpkg.SetWorkspaceID(context.Background(), "ws_1")
}

// TestCreateGetEdit_HappyPath drives create → get → edit through the tools over a real
// Service + in-memory store, asserting the round-trip JSON carries the expected ids.
func TestCreateGetEdit_HappyPath(t *testing.T) {
	svc, ctx := newSvc(t)
	create := &CreateWorkflow{svc: svc}
	get := &GetWorkflow{svc: svc}
	edit := &EditWorkflow{svc: svc}

	createArgs := `{"name":"pipe","ops":[
		{"op":"add_node","node":{"id":"t","kind":"trigger","ref":"trg_a"}},
		{"op":"add_node","node":{"id":"a","kind":"action","ref":"fn_b","input":{"x":"t.v"}}},
		{"op":"add_edge","edge":{"id":"e1","from":"t","to":"a"}}
	]}`
	out, err := create.Execute(ctx, createArgs)
	if err != nil {
		t.Fatalf("create.Execute: %v", err)
	}
	var created struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
		Active  bool   `json:"active"`
	}
	if err := json.Unmarshal([]byte(out), &created); err != nil {
		t.Fatalf("create result: %v (%s)", err, out)
	}
	if created.ID == "" || created.Version != 1 || created.Active {
		t.Fatalf("create result wrong: %+v", created)
	}

	got, err := get.Execute(ctx, `{"workflowId":"`+created.ID+`"}`)
	if err != nil {
		t.Fatalf("get.Execute: %v", err)
	}
	if got == "" {
		t.Fatal("get returned empty")
	}

	editArgs := `{"workflowId":"` + created.ID + `","ops":[{"op":"delete_edge","id":"e1"},{"op":"delete_node","id":"a"}]}`
	editOut, err := edit.Execute(ctx, editArgs)
	if err != nil {
		t.Fatalf("edit.Execute: %v", err)
	}
	var edited struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal([]byte(editOut), &edited); err != nil {
		t.Fatalf("edit result: %v (%s)", err, editOut)
	}
	if edited.Version != 2 {
		t.Fatalf("edit should produce v2, got %d", edited.Version)
	}
}

func TestCapabilityCheck_Execute_StructuralOnly(t *testing.T) {
	svc, ctx := newSvc(t)
	create := &CreateWorkflow{svc: svc}
	out, err := create.Execute(ctx, `{"name":"cc","ops":[
		{"op":"add_node","node":{"id":"t","kind":"trigger","ref":"trg_a"}},
		{"op":"add_node","node":{"id":"a","kind":"action","ref":"fn_b","input":{"x":"t.v"}}},
		{"op":"add_edge","edge":{"id":"e1","from":"t","to":"a"}}
	]}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal([]byte(out), &created)

	cc := &CapabilityCheckWorkflow{svc: svc}
	res, err := cc.Execute(ctx, `{"workflowId":"`+created.ID+`"}`)
	if err != nil {
		t.Fatalf("capcheck.Execute: %v", err)
	}
	var rep struct {
		OK                bool `json:"ok"`
		StructurallyValid bool `json:"structurallyValid"`
		Resolved          bool `json:"resolved"`
	}
	if err := json.Unmarshal([]byte(res), &rep); err != nil {
		t.Fatalf("capcheck result: %v (%s)", err, res)
	}
	// No resolver wired → structural-only, valid, OK.
	if !rep.OK || !rep.StructurallyValid || rep.Resolved {
		t.Fatalf("structural-only capcheck wrong: %+v", rep)
	}
}
