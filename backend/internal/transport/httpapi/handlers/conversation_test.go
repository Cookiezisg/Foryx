package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"
	gormlogger "gorm.io/gorm/logger"

	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	convstore "github.com/sunweilin/forgify/backend/internal/infra/store/conversation"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

func newConvTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	gdb, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("dbinfra.Open: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(gdb) })
	if err := dbinfra.Migrate(gdb, &convdomain.Conversation{}); err != nil {
		t.Fatalf("dbinfra.Migrate: %v", err)
	}
	log := zaptest.NewLogger(t)
	svc := convapp.NewService(convstore.New(gdb), nil, log)
	h := NewConversationHandler(svc, nil, log)
	mux := http.NewServeMux()
	h.Register(mux)
	return httptest.NewServer(middlewarehttpapi.InjectUserID(mux))
}

func TestConvHandler_Create_Success(t *testing.T) {
	srv := newConvTestServer(t)
	defer srv.Close()

	status, env := do(t, srv, "POST", "/api/v1/conversations", map[string]any{
		"title": "My First Chat",
	})
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %+v", status, env)
	}
	d := dataMap(t, env)
	if d["title"].(string) != "My First Chat" {
		t.Errorf("title = %q", d["title"])
	}
	if id := d["id"].(string); len(id) < 4 {
		t.Errorf("id = %q, too short", id)
	}
}

func TestConvHandler_Create_EmptyTitleAllowed(t *testing.T) {
	srv := newConvTestServer(t)
	defer srv.Close()

	status, _ := do(t, srv, "POST", "/api/v1/conversations", map[string]any{"title": ""})
	if status != http.StatusCreated {
		t.Errorf("status = %d, want 201", status)
	}
}

func TestConvHandler_List_Paged(t *testing.T) {
	srv := newConvTestServer(t)
	defer srv.Close()

	for _, title := range []string{"A", "B", "C"} {
		do(t, srv, "POST", "/api/v1/conversations", map[string]any{"title": title})
	}
	status, env := do(t, srv, "GET", "/api/v1/conversations", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	items := env["data"].([]any)
	if len(items) != 3 {
		t.Errorf("len(data) = %d, want 3", len(items))
	}
	if _, has := env["hasMore"]; !has {
		t.Error("paged envelope missing hasMore")
	}
}

func TestConvHandler_Rename_Success(t *testing.T) {
	srv := newConvTestServer(t)
	defer srv.Close()

	_, env := do(t, srv, "POST", "/api/v1/conversations", map[string]any{"title": "Old"})
	id := dataMap(t, env)["id"].(string)

	status, env := do(t, srv, "PATCH", "/api/v1/conversations/"+id, map[string]any{"title": "New"})
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if got := dataMap(t, env)["title"].(string); got != "New" {
		t.Errorf("title = %q, want New", got)
	}
}

func TestConvHandler_Rename_NotFound(t *testing.T) {
	srv := newConvTestServer(t)
	defer srv.Close()

	status, env := do(t, srv, "PATCH", "/api/v1/conversations/nope", map[string]any{"title": "x"})
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if code := errorCode(t, env); code != "CONVERSATION_NOT_FOUND" {
		t.Errorf("code = %q, want CONVERSATION_NOT_FOUND", code)
	}
}

func TestConvHandler_Delete_Success(t *testing.T) {
	srv := newConvTestServer(t)
	defer srv.Close()

	_, env := do(t, srv, "POST", "/api/v1/conversations", map[string]any{"title": "test"})
	id := dataMap(t, env)["id"].(string)

	status, _ := do(t, srv, "DELETE", "/api/v1/conversations/"+id, nil)
	if status != http.StatusNoContent {
		t.Errorf("status = %d, want 204", status)
	}
	status, env = do(t, srv, "DELETE", "/api/v1/conversations/"+id, nil)
	if status != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404", status)
	}
	if code := errorCode(t, env); code != "CONVERSATION_NOT_FOUND" {
		t.Errorf("code = %q, want CONVERSATION_NOT_FOUND", code)
	}
}
