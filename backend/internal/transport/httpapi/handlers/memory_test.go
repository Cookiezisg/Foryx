package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"
	gormlogger "gorm.io/gorm/logger"

	memoryapp "github.com/sunweilin/forgify/backend/internal/app/memory"
	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	memorystore "github.com/sunweilin/forgify/backend/internal/infra/store/memory"
)

func newMemoryTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	gdb, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("dbinfra.Open: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(gdb) })
	if err := dbinfra.Migrate(gdb, memorystore.AutoMigrateModels()...); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	log := zaptest.NewLogger(t)
	svc := memoryapp.New(memorystore.New(gdb), nil, log)
	h := NewMemoryHandler(svc, log)
	mux := http.NewServeMux()
	h.Register(mux)
	return httptest.NewServer(mux)
}

func memCreateBody(name, ty, desc, content string, pinned bool) map[string]any {
	body := map[string]any{
		"name":        name,
		"type":        ty,
		"description": desc,
		"content":     content,
	}
	if pinned {
		body["pinned"] = true
	}
	return body
}

func TestMemoryHandler_Create_Success(t *testing.T) {
	srv := newMemoryTestServer(t)
	defer srv.Close()

	status, env := do(t, srv, "POST", "/api/v1/memories",
		memCreateBody("user_role", memorydomain.TypeUser, "Go engineer", "User is a Go engineer.", false))
	if status != http.StatusCreated {
		t.Fatalf("status = %d, env=%+v", status, env)
	}
	d := dataMap(t, env)
	if d["name"].(string) != "user_role" {
		t.Errorf("name = %q", d["name"])
	}
	if d["source"].(string) != memorydomain.SourceUser {
		t.Errorf("source = %q, want user", d["source"])
	}
}

func TestMemoryHandler_Create_DuplicateName_409(t *testing.T) {
	srv := newMemoryTestServer(t)
	defer srv.Close()
	body := memCreateBody("x", memorydomain.TypeUser, "d", "c", false)
	do(t, srv, "POST", "/api/v1/memories", body)
	status, env := do(t, srv, "POST", "/api/v1/memories", body)
	if status != http.StatusConflict {
		t.Errorf("status = %d, want 409: %+v", status, env)
	}
}

func TestMemoryHandler_Create_InvalidName_400(t *testing.T) {
	srv := newMemoryTestServer(t)
	defer srv.Close()
	status, _ := do(t, srv, "POST", "/api/v1/memories",
		memCreateBody("Bad Name!", memorydomain.TypeUser, "d", "c", false))
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}

func TestMemoryHandler_Get_NotFound(t *testing.T) {
	srv := newMemoryTestServer(t)
	defer srv.Close()
	status, _ := do(t, srv, "GET", "/api/v1/memories/nope", nil)
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

func TestMemoryHandler_List_FilterByType(t *testing.T) {
	srv := newMemoryTestServer(t)
	defer srv.Close()
	do(t, srv, "POST", "/api/v1/memories",
		memCreateBody("a", memorydomain.TypeUser, "d", "c", false))
	do(t, srv, "POST", "/api/v1/memories",
		memCreateBody("b", memorydomain.TypeFeedback, "d", "c", false))

	status, env := do(t, srv, "GET", "/api/v1/memories?type=feedback", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	items := env["data"].([]any)
	if len(items) != 1 {
		t.Errorf("len = %d, want 1", len(items))
	}
}

func TestMemoryHandler_Update_Partial(t *testing.T) {
	srv := newMemoryTestServer(t)
	defer srv.Close()
	do(t, srv, "POST", "/api/v1/memories",
		memCreateBody("x", memorydomain.TypeUser, "old desc", "old content", false))

	status, env := do(t, srv, "PATCH", "/api/v1/memories/x", map[string]any{
		"description": "new desc",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	d := dataMap(t, env)
	if d["description"].(string) != "new desc" {
		t.Errorf("description not updated: %v", d)
	}
	if d["content"].(string) != "old content" {
		t.Errorf("content should preserve: %v", d)
	}
}

func TestMemoryHandler_PinUnpin(t *testing.T) {
	srv := newMemoryTestServer(t)
	defer srv.Close()
	do(t, srv, "POST", "/api/v1/memories",
		memCreateBody("x", memorydomain.TypeUser, "d", "c", false))

	status, env := do(t, srv, "POST", "/api/v1/memories/x:pin", nil)
	if status != http.StatusOK {
		t.Fatalf("pin status = %d", status)
	}
	if !dataMap(t, env)["pinned"].(bool) {
		t.Errorf("after pin, pinned should be true")
	}

	status, env = do(t, srv, "POST", "/api/v1/memories/x:unpin", nil)
	if status != http.StatusOK {
		t.Fatalf("unpin status = %d", status)
	}
	if dataMap(t, env)["pinned"].(bool) {
		t.Errorf("after unpin, pinned should be false")
	}
}

func TestMemoryHandler_PinUnknown_404(t *testing.T) {
	srv := newMemoryTestServer(t)
	defer srv.Close()
	status, _ := do(t, srv, "POST", "/api/v1/memories/nope:pin", nil)
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

func TestMemoryHandler_UnknownAction_404(t *testing.T) {
	srv := newMemoryTestServer(t)
	defer srv.Close()
	do(t, srv, "POST", "/api/v1/memories",
		memCreateBody("x", memorydomain.TypeUser, "d", "c", false))
	status, _ := do(t, srv, "POST", "/api/v1/memories/x:weird", nil)
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

func TestMemoryHandler_Delete_204(t *testing.T) {
	srv := newMemoryTestServer(t)
	defer srv.Close()
	do(t, srv, "POST", "/api/v1/memories",
		memCreateBody("x", memorydomain.TypeUser, "d", "c", false))
	status, _ := do(t, srv, "DELETE", "/api/v1/memories/x", nil)
	if status != http.StatusNoContent {
		t.Errorf("status = %d, want 204", status)
	}
	status, _ = do(t, srv, "GET", "/api/v1/memories/x", nil)
	if status != http.StatusNotFound {
		t.Errorf("after delete, GET status = %d, want 404", status)
	}
}
