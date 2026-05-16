package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"
	gormlogger "gorm.io/gorm/logger"

	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	workflowstore "github.com/sunweilin/forgify/backend/internal/infra/store/workflow"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

func newWorkflowTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	gdb, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("dbinfra.Open: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(gdb) })
	if err := dbinfra.Migrate(gdb, workflowstore.AutoMigrateModels()...); err != nil {
		t.Fatalf("dbinfra.Migrate: %v", err)
	}
	log := zaptest.NewLogger(t)
	svc := workflowapp.NewService(workflowstore.New(gdb), nil, notificationspkg.New(nil, log), log)
	h := NewWorkflowHandler(svc, log)
	mux := http.NewServeMux()
	h.Register(mux)
	return httptest.NewServer(middlewarehttpapi.InjectUserID(mux))
}

func happyCreateOps(name string) []map[string]any {
	return []map[string]any{
		{"op": "set_meta", "name": name, "description": "test workflow"},
		{"op": "add_node", "node": map[string]any{
			"id":   "n1",
			"type": "trigger",
			"name": "manual",
			"config": map[string]any{
				"triggerType": "manual",
			},
		}},
	}
}

func TestWorkflowHandler_Create_Success(t *testing.T) {
	srv := newWorkflowTestServer(t)
	defer srv.Close()

	status, env := do(t, srv, "POST", "/api/v1/workflows", map[string]any{
		"ops":          happyCreateOps("hello-wf"),
		"changeReason": "first version",
	})
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %+v", status, env)
	}
	d := dataMap(t, env)
	wf, ok := d["workflow"].(map[string]any)
	if !ok {
		t.Fatalf("workflow missing: %+v", d)
	}
	if got := wf["name"].(string); got != "hello-wf" {
		t.Errorf("name = %q, want hello-wf", got)
	}
	if id := wf["id"].(string); len(id) < 4 {
		t.Errorf("id = %q, too short", id)
	}
	v, ok := d["version"].(map[string]any)
	if !ok {
		t.Fatalf("version missing")
	}
	if got := v["status"].(string); got != workflowdomain.StatusAccepted {
		t.Errorf("version status = %q, want accepted", got)
	}
}

func TestWorkflowHandler_Create_DuplicateName(t *testing.T) {
	srv := newWorkflowTestServer(t)
	defer srv.Close()

	body := map[string]any{"ops": happyCreateOps("dup")}
	do(t, srv, "POST", "/api/v1/workflows", body)
	status, env := do(t, srv, "POST", "/api/v1/workflows", body)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %+v", status, env)
	}
	if code := errorCode(t, env); code != "WORKFLOW_NAME_DUPLICATE" {
		t.Errorf("code = %q, want WORKFLOW_NAME_DUPLICATE", code)
	}
}

func TestWorkflowHandler_Create_NoTrigger(t *testing.T) {
	srv := newWorkflowTestServer(t)
	defer srv.Close()

	status, env := do(t, srv, "POST", "/api/v1/workflows", map[string]any{
		"ops": []map[string]any{
			{"op": "set_meta", "name": "no-trigger", "description": "x"},
		},
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %+v", status, env)
	}
	if code := errorCode(t, env); code != "WORKFLOW_NO_TRIGGER" {
		t.Errorf("code = %q, want WORKFLOW_NO_TRIGGER", code)
	}
}

func TestWorkflowHandler_List_Paged(t *testing.T) {
	srv := newWorkflowTestServer(t)
	defer srv.Close()

	for _, n := range []string{"a-wf", "b-wf", "c-wf"} {
		do(t, srv, "POST", "/api/v1/workflows", map[string]any{"ops": happyCreateOps(n)})
	}
	status, env := do(t, srv, "GET", "/api/v1/workflows", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	items := dataSlice(t, env)
	if len(items) != 3 {
		t.Errorf("len(data) = %d, want 3", len(items))
	}
	if _, has := env["hasMore"]; !has {
		t.Error("paged envelope missing hasMore")
	}
}

func TestWorkflowHandler_List_EnabledFilter(t *testing.T) {
	srv := newWorkflowTestServer(t)
	defer srv.Close()

	for _, n := range []string{"on-1", "on-2"} {
		do(t, srv, "POST", "/api/v1/workflows", map[string]any{"ops": happyCreateOps(n)})
	}
	_, env := do(t, srv, "GET", "/api/v1/workflows", nil)
	for _, raw := range dataSlice(t, env) {
		wf := raw.(map[string]any)
		if wf["name"].(string) == "on-2" {
			id := wf["id"].(string)
			do(t, srv, "PATCH", "/api/v1/workflows/"+id, map[string]any{"enabled": false})
		}
	}
	status, env := do(t, srv, "GET", "/api/v1/workflows?enabled=true", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if got := len(dataSlice(t, env)); got != 1 {
		t.Errorf("enabled-only count = %d, want 1", got)
	}
}

func TestWorkflowHandler_Get_NotFound(t *testing.T) {
	srv := newWorkflowTestServer(t)
	defer srv.Close()

	status, env := do(t, srv, "GET", "/api/v1/workflows/wf_missing", nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %+v", status, env)
	}
	if code := errorCode(t, env); code != "WORKFLOW_NOT_FOUND" {
		t.Errorf("code = %q, want WORKFLOW_NOT_FOUND", code)
	}
}

func TestWorkflowHandler_UpdateMeta_Success(t *testing.T) {
	srv := newWorkflowTestServer(t)
	defer srv.Close()

	_, env := do(t, srv, "POST", "/api/v1/workflows", map[string]any{"ops": happyCreateOps("orig-name")})
	id := dataMap(t, env)["workflow"].(map[string]any)["id"].(string)

	newDesc := "updated description"
	status, env := do(t, srv, "PATCH", "/api/v1/workflows/"+id, map[string]any{
		"description": newDesc,
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %+v", status, env)
	}
	if got := dataMap(t, env)["description"].(string); got != newDesc {
		t.Errorf("description = %q, want %q", got, newDesc)
	}
}

func TestWorkflowHandler_Delete_Success(t *testing.T) {
	srv := newWorkflowTestServer(t)
	defer srv.Close()

	_, env := do(t, srv, "POST", "/api/v1/workflows", map[string]any{"ops": happyCreateOps("to-delete")})
	id := dataMap(t, env)["workflow"].(map[string]any)["id"].(string)

	status, _ := do(t, srv, "DELETE", "/api/v1/workflows/"+id, nil)
	if status != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", status)
	}
	status, env = do(t, srv, "GET", "/api/v1/workflows/"+id, nil)
	if status != http.StatusNotFound {
		t.Fatalf("post-delete GET status = %d, want 404: %+v", status, env)
	}
}

func TestWorkflowHandler_Versions_Listing(t *testing.T) {
	srv := newWorkflowTestServer(t)
	defer srv.Close()

	_, env := do(t, srv, "POST", "/api/v1/workflows", map[string]any{"ops": happyCreateOps("versioned")})
	id := dataMap(t, env)["workflow"].(map[string]any)["id"].(string)

	status, env := do(t, srv, "GET", "/api/v1/workflows/"+id+"/versions", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	rows := dataSlice(t, env)
	if len(rows) != 1 {
		t.Errorf("len(versions) = %d, want 1", len(rows))
	}
	status, env = do(t, srv, "GET", "/api/v1/workflows/"+id+"/versions/1", nil)
	if status != http.StatusOK {
		t.Fatalf("version detail status = %d", status)
	}
	v := dataMap(t, env)
	if got := v["status"].(string); got != workflowdomain.StatusAccepted {
		t.Errorf("version status = %q", got)
	}
}

func TestWorkflowHandler_Pending_NotFound(t *testing.T) {
	srv := newWorkflowTestServer(t)
	defer srv.Close()

	_, env := do(t, srv, "POST", "/api/v1/workflows", map[string]any{"ops": happyCreateOps("no-pending")})
	id := dataMap(t, env)["workflow"].(map[string]any)["id"].(string)

	status, env := do(t, srv, "GET", "/api/v1/workflows/"+id+"/pending", nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %+v", status, env)
	}
	if code := errorCode(t, env); code != "WORKFLOW_PENDING_NOT_FOUND" {
		t.Errorf("code = %q, want WORKFLOW_PENDING_NOT_FOUND", code)
	}
}

func TestWorkflowHandler_Revert_Success(t *testing.T) {
	srv := newWorkflowTestServer(t)
	defer srv.Close()

	_, env := do(t, srv, "POST", "/api/v1/workflows", map[string]any{"ops": happyCreateOps("reverting")})
	id := dataMap(t, env)["workflow"].(map[string]any)["id"].(string)

	status, env := do(t, srv, "POST", "/api/v1/workflows/"+id+":revert", map[string]any{
		"targetVersion": 1,
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d: %+v", status, env)
	}
	v := dataMap(t, env)
	if got := v["status"].(string); got != workflowdomain.StatusAccepted {
		t.Errorf("status = %q, want accepted", got)
	}
}

func TestWorkflowHandler_PostUnknownAction_NotFound(t *testing.T) {
	srv := newWorkflowTestServer(t)
	defer srv.Close()

	_, env := do(t, srv, "POST", "/api/v1/workflows", map[string]any{"ops": happyCreateOps("unknown-action")})
	id := dataMap(t, env)["workflow"].(map[string]any)["id"].(string)

	resp, err := srv.Client().Post(srv.URL+"/api/v1/workflows/"+id+":bogus", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown action status = %d, want 404", resp.StatusCode)
	}
}

func TestWorkflowHandler_Trigger_NoFlowRunHandlerReturns503(t *testing.T) {
	srv := newWorkflowTestServer(t)
	defer srv.Close()

	_, env := do(t, srv, "POST", "/api/v1/workflows", map[string]any{"ops": happyCreateOps("trigger-test")})
	id := dataMap(t, env)["workflow"].(map[string]any)["id"].(string)

	status, env := do(t, srv, "POST", "/api/v1/workflows/"+id+":trigger",
		map[string]any{"input": map[string]any{}})
	if status != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", status)
	}
	if code := errorCode(t, env); code != "SCHEDULER_NOT_AVAILABLE" {
		t.Errorf("code = %q, want SCHEDULER_NOT_AVAILABLE", code)
	}
}

func TestWorkflowHandler_Create_BadJSON(t *testing.T) {
	srv := newWorkflowTestServer(t)
	defer srv.Close()

	status, env := do(t, srv, "POST", "/api/v1/workflows", map[string]any{
		"ops":           happyCreateOps("bad"),
		"unknown_field": "x",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %+v", status, env)
	}
	if code := errorCode(t, env); code != "INVALID_REQUEST" {
		t.Errorf("code = %q, want INVALID_REQUEST", code)
	}
}
