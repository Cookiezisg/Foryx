package approval

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"go.uber.org/zap"

	approvalapp "github.com/sunweilin/forgify/backend/internal/app/approval"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	approvaldomain "github.com/sunweilin/forgify/backend/internal/domain/approval"
	approvalstore "github.com/sunweilin/forgify/backend/internal/infra/store/approval"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func newToolSvc(t *testing.T) (*approvalapp.Service, context.Context) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, stmt := range approvalstore.Schema {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	svc := approvalapp.NewService(approvalstore.New(ormpkg.Open(sqlDB)), nil, zap.NewNop())
	return svc, reqctxpkg.SetWorkspaceID(context.Background(), "ws_1")
}

func TestApprovalTools_Wiring(t *testing.T) {
	tools := ApprovalTools(nil) // nil svc OK: we only inspect Name() here
	want := map[string]bool{
		"search_approval": false, "get_approval": false, "create_approval": false,
		"edit_approval": false, "revert_approval": false, "delete_approval": false,
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

func TestApprovalTools_ValidateInput(t *testing.T) {
	cases := []struct {
		name    string
		tool    toolapp.Tool
		args    string
		wantErr bool
	}{
		{"create no name", &CreateApproval{}, `{"template":"ok?"}`, true},
		{"create no template", &CreateApproval{}, `{"name":"x"}`, true},
		{"create ok", &CreateApproval{}, `{"name":"x","template":"ok?"}`, false},
		{"edit no id", &EditApproval{}, `{"template":"ok?"}`, true},
		{"edit no template", &EditApproval{}, `{"approvalId":"apf_1"}`, true},
		{"edit ok", &EditApproval{}, `{"approvalId":"apf_1","template":"ok?"}`, false},
		{"revert no id", &RevertApproval{}, `{"version":1}`, true},
		{"revert bad version", &RevertApproval{}, `{"approvalId":"apf_1","version":0}`, true},
		{"revert ok", &RevertApproval{}, `{"approvalId":"apf_1","version":2}`, false},
		{"get no id", &GetApproval{}, `{}`, true},
		{"delete no id", &DeleteApproval{}, `{}`, true},
		{"search any", &SearchApproval{}, `{}`, false},
	}
	for _, c := range cases {
		err := c.tool.ValidateInput([]byte(c.args))
		if (err != nil) != c.wantErr {
			t.Errorf("%s: ValidateInput(%s) err=%v, wantErr=%v", c.name, c.args, err, c.wantErr)
		}
	}
}

func TestApprovalTools_RoundTrip(t *testing.T) {
	svc, ctx := newToolSvc(t)

	out, err := (&CreateApproval{svc: svc}).Execute(ctx,
		`{"name":"email","template":"发送给 {{ payload.to }}?","allowReason":true,"timeout":"30d","timeoutBehavior":"reject"}`)
	if err != nil {
		t.Fatalf("create execute: %v", err)
	}
	id := extractID(t, out)

	if _, err := (&GetApproval{svc: svc}).Execute(ctx, `{"approvalId":"`+id+`"}`); err != nil {
		t.Fatalf("get execute: %v", err)
	}
	if _, err := (&EditApproval{svc: svc}).Execute(ctx, `{"approvalId":"`+id+`","template":"改 {{ payload.x }}?"}`); err != nil {
		t.Fatalf("edit execute: %v", err)
	}
	if _, err := (&RevertApproval{svc: svc}).Execute(ctx, `{"approvalId":"`+id+`","version":1}`); err != nil {
		t.Fatalf("revert execute: %v", err)
	}
	sout, err := (&SearchApproval{svc: svc}).Execute(ctx, `{"query":"email"}`)
	if err != nil || !strings.Contains(sout, "email") {
		t.Fatalf("search execute: %v out=%s", err, sout)
	}
	if _, err := (&DeleteApproval{svc: svc}).Execute(ctx, `{"approvalId":"`+id+`"}`); err != nil {
		t.Fatalf("delete execute: %v", err)
	}
}

func TestCreateApproval_InvalidTemplate(t *testing.T) {
	svc, ctx := newToolSvc(t)
	_, err := (&CreateApproval{svc: svc}).Execute(ctx, `{"name":"bad","template":"bad {{ payload.( }}"}`)
	if !errors.Is(err, approvaldomain.ErrInvalidTemplate) {
		t.Fatalf("want ErrInvalidTemplate bubbled (framework softens at loop layer), got %v", err)
	}
}

func extractID(t *testing.T, jsonStr string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		t.Fatalf("unmarshal create out: %v", err)
	}
	id, _ := m["id"].(string)
	if id == "" {
		t.Fatalf("no id in create out: %s", jsonStr)
	}
	return id
}
