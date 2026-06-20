package control

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"go.uber.org/zap"

	controlapp "github.com/sunweilin/anselm/backend/internal/app/control"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	controldomain "github.com/sunweilin/anselm/backend/internal/domain/control"
	controlstore "github.com/sunweilin/anselm/backend/internal/infra/store/control"
	ormpkg "github.com/sunweilin/anselm/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

func newToolSvc(t *testing.T) (*controlapp.Service, context.Context) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, stmt := range controlstore.Schema {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	svc := controlapp.NewService(controlstore.New(ormpkg.Open(sqlDB)), nil, zap.NewNop())
	return svc, reqctxpkg.SetWorkspaceID(context.Background(), "ws_1")
}

func TestControlTools_Wiring(t *testing.T) {
	tools := ControlTools(nil, nil, nil) // nil svc OK: we only inspect Name() here
	want := map[string]bool{
		"search_control": false, "get_control": false, "create_control": false,
		"edit_control": false, "revert_control": false, "delete_control": false,
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

func TestControlTools_ValidateInput(t *testing.T) {
	cases := []struct {
		name    string
		tool    toolapp.Tool
		args    string
		wantErr bool
	}{
		{"create no name", &CreateControl{}, `{"branches":[{"port":"a","when":"true"}]}`, true},
		{"create no branches", &CreateControl{}, `{"name":"x"}`, true},
		{"create ok", &CreateControl{}, `{"name":"x","branches":[{"port":"a","when":"true"}]}`, false},
		{"edit no id", &EditControl{}, `{"branches":[{"port":"a","when":"true"}]}`, true},
		{"edit no branches", &EditControl{}, `{"controlId":"ctl_1"}`, true},
		{"edit ok", &EditControl{}, `{"controlId":"ctl_1","branches":[{"port":"a","when":"true"}]}`, false},
		{"revert no id", &RevertControl{}, `{"version":1}`, true},
		{"revert bad version", &RevertControl{}, `{"controlId":"ctl_1","version":0}`, true},
		{"revert ok", &RevertControl{}, `{"controlId":"ctl_1","version":2}`, false},
		{"get no id", &GetControl{}, `{}`, true},
		{"delete no id", &DeleteControl{}, `{}`, true},
		{"search any", &SearchControl{}, `{}`, false},
	}
	for _, c := range cases {
		err := c.tool.ValidateInput([]byte(c.args))
		if (err != nil) != c.wantErr {
			t.Errorf("%s: ValidateInput(%s) err=%v, wantErr=%v", c.name, c.args, err, c.wantErr)
		}
	}
}

func TestControlTools_RoundTrip(t *testing.T) {
	svc, ctx := newToolSvc(t)

	out, err := (&CreateControl{svc: svc}).Execute(ctx,
		`{"name":"router","description":"d","branches":[{"port":"pass","when":"input.score >= 0.9","emit":{"n":"input.n + 1"}},{"port":"else","when":"true"}]}`)
	if err != nil {
		t.Fatalf("create execute: %v", err)
	}
	id := extractID(t, out)

	if _, err := (&GetControl{svc: svc}).Execute(ctx, `{"controlId":"`+id+`"}`); err != nil {
		t.Fatalf("get execute: %v", err)
	}
	if _, err := (&EditControl{svc: svc}).Execute(ctx, `{"controlId":"`+id+`","branches":[{"port":"only","when":"true"}]}`); err != nil {
		t.Fatalf("edit execute: %v", err)
	}
	if _, err := (&RevertControl{svc: svc}).Execute(ctx, `{"controlId":"`+id+`","version":1}`); err != nil {
		t.Fatalf("revert execute: %v", err)
	}
	sout, err := (&SearchControl{svc: svc}).Execute(ctx, `{"query":"router"}`)
	if err != nil || !strings.Contains(sout, "router") {
		t.Fatalf("search execute: %v out=%s", err, sout)
	}
	if _, err := (&DeleteControl{svc: svc}).Execute(ctx, `{"controlId":"`+id+`"}`); err != nil {
		t.Fatalf("delete execute: %v", err)
	}
}

func TestCreateControl_InvalidCEL(t *testing.T) {
	svc, ctx := newToolSvc(t)
	_, err := (&CreateControl{svc: svc}).Execute(ctx,
		`{"name":"bad","branches":[{"port":"x","when":"input.("},{"port":"y","when":"true"}]}`)
	if !errors.Is(err, controldomain.ErrInvalidCEL) {
		t.Fatalf("want ErrInvalidCEL bubbled (framework softens at loop layer), got %v", err)
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
