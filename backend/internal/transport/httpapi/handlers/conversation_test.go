package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"
	gormlogger "gorm.io/gorm/logger"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	cryptoinfra "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	apikeystore "github.com/sunweilin/forgify/backend/internal/infra/store/apikey"
	convstore "github.com/sunweilin/forgify/backend/internal/infra/store/conversation"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

// convTestEnv bundles the test server plus an apikey service for tests
// exercising ModelOverride's F1 validation via PATCH.
//
// convTestEnv 携测试服务 + apikey service;PATCH ModelOverride 走 F1 校验。
type convTestEnv struct {
	srv       *httptest.Server
	apikeySvc *apikeyapp.Service
	apiKeyID  string
}

func newConvTestServer(t *testing.T) *convTestEnv {
	t.Helper()
	gdb, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("dbinfra.Open: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(gdb) })
	if err := dbinfra.Migrate(gdb, &convdomain.Conversation{}, &apikeydomain.APIKey{}); err != nil {
		t.Fatalf("dbinfra.Migrate: %v", err)
	}
	log := zaptest.NewLogger(t)

	enc, err := cryptoinfra.NewAESGCMEncryptor(cryptoinfra.DeriveKey("conv-handler-test"))
	if err != nil {
		t.Fatalf("NewAESGCMEncryptor: %v", err)
	}
	apikeySvc := apikeyapp.NewService(apikeystore.New(gdb), enc, &fakeTester{}, log)
	svc := convapp.NewService(convstore.New(gdb), nil, log)
	svc.SetKeyProvider(apikeySvc)
	h := NewConversationHandler(svc, nil, log)
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(middlewarehttpapi.InjectUserID(mux))

	// Seed one api_key for the InjectUserID-stamped user so ModelOverride
	// PATCH happy-paths can pass F1.
	//
	// 给 InjectUserID 用户种一把 api_key;让 PATCH 走 F1 校验通过。
	ctx := reqctxpkg.SetUserID(context.Background(), "test-user")
	key, err := apikeySvc.Create(ctx, apikeyapp.CreateInput{
		Provider: "openai", Key: "sk-test-fixture", DisplayName: "fixture",
	})
	if err != nil {
		t.Fatalf("seed api_key: %v", err)
	}
	return &convTestEnv{srv: srv, apikeySvc: apikeySvc, apiKeyID: key.ID}
}

func TestConvHandler_Create_Success(t *testing.T) {
	env := newConvTestServer(t)
	defer env.srv.Close()

	status, envBody := do(t, env.srv, "POST", "/api/v1/conversations", map[string]any{
		"title": "My First Chat",
	})
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %+v", status, envBody)
	}
	d := dataMap(t, envBody)
	if d["title"].(string) != "My First Chat" {
		t.Errorf("title = %q", d["title"])
	}
	if id := d["id"].(string); len(id) < 4 {
		t.Errorf("id = %q, too short", id)
	}
}

func TestConvHandler_Create_EmptyTitleAllowed(t *testing.T) {
	env := newConvTestServer(t)
	defer env.srv.Close()

	status, _ := do(t, env.srv, "POST", "/api/v1/conversations", map[string]any{"title": ""})
	if status != http.StatusCreated {
		t.Errorf("status = %d, want 201", status)
	}
}

func TestConvHandler_List_Paged(t *testing.T) {
	env := newConvTestServer(t)
	defer env.srv.Close()

	for _, title := range []string{"A", "B", "C"} {
		do(t, env.srv, "POST", "/api/v1/conversations", map[string]any{"title": title})
	}
	status, envBody := do(t, env.srv, "GET", "/api/v1/conversations", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	items := envBody["data"].([]any)
	if len(items) != 3 {
		t.Errorf("len(data) = %d, want 3", len(items))
	}
	if _, has := envBody["hasMore"]; !has {
		t.Error("paged envelope missing hasMore")
	}
}

func TestConvHandler_Rename_Success(t *testing.T) {
	env := newConvTestServer(t)
	defer env.srv.Close()

	_, envBody := do(t, env.srv, "POST", "/api/v1/conversations", map[string]any{"title": "Old"})
	id := dataMap(t, envBody)["id"].(string)

	status, envBody := do(t, env.srv, "PATCH", "/api/v1/conversations/"+id, map[string]any{"title": "New"})
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if got := dataMap(t, envBody)["title"].(string); got != "New" {
		t.Errorf("title = %q, want New", got)
	}
}

func TestConvHandler_Rename_NotFound(t *testing.T) {
	env := newConvTestServer(t)
	defer env.srv.Close()

	status, envBody := do(t, env.srv, "PATCH", "/api/v1/conversations/nope", map[string]any{"title": "x"})
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if code := errorCode(t, envBody); code != "CONVERSATION_NOT_FOUND" {
		t.Errorf("code = %q, want CONVERSATION_NOT_FOUND", code)
	}
}

func TestConvHandler_Delete_Success(t *testing.T) {
	env := newConvTestServer(t)
	defer env.srv.Close()

	_, envBody := do(t, env.srv, "POST", "/api/v1/conversations", map[string]any{"title": "test"})
	id := dataMap(t, envBody)["id"].(string)

	status, _ := do(t, env.srv, "DELETE", "/api/v1/conversations/"+id, nil)
	if status != http.StatusNoContent {
		t.Errorf("status = %d, want 204", status)
	}
	status, envBody = do(t, env.srv, "DELETE", "/api/v1/conversations/"+id, nil)
	if status != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404", status)
	}
	if code := errorCode(t, envBody); code != "CONVERSATION_NOT_FOUND" {
		t.Errorf("code = %q, want CONVERSATION_NOT_FOUND", code)
	}
}

func TestConvHandler_PatchModelOverride_Success(t *testing.T) {
	env := newConvTestServer(t)
	defer env.srv.Close()

	_, envBody := do(t, env.srv, "POST", "/api/v1/conversations", map[string]any{"title": "with-override"})
	id := dataMap(t, envBody)["id"].(string)

	status, envBody := do(t, env.srv, "PATCH", "/api/v1/conversations/"+id, map[string]any{
		"modelOverride": map[string]any{
			"apiKeyId": env.apiKeyID,
			"modelId":  "gpt-4o",
		},
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %+v", status, envBody)
	}
	d := dataMap(t, envBody)
	mo, ok := d["modelOverride"].(map[string]any)
	if !ok {
		t.Fatalf("modelOverride missing or wrong shape: %+v", d["modelOverride"])
	}
	if got := mo["apiKeyId"].(string); got != env.apiKeyID {
		t.Errorf("apiKeyId = %q, want %q", got, env.apiKeyID)
	}
	if got := mo["modelId"].(string); got != "gpt-4o" {
		t.Errorf("modelId = %q, want gpt-4o", got)
	}
}

func TestConvHandler_PatchModelOverride_APIKeyIDRequired(t *testing.T) {
	env := newConvTestServer(t)
	defer env.srv.Close()

	_, envBody := do(t, env.srv, "POST", "/api/v1/conversations", map[string]any{"title": "x"})
	id := dataMap(t, envBody)["id"].(string)

	status, envBody := do(t, env.srv, "PATCH", "/api/v1/conversations/"+id, map[string]any{
		"modelOverride": map[string]any{
			"apiKeyId": "",
			"modelId":  "gpt-4o",
		},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if code := errorCode(t, envBody); code != "API_KEY_ID_REQUIRED" {
		t.Errorf("code = %q, want API_KEY_ID_REQUIRED", code)
	}
}

func TestConvHandler_PatchModelOverride_APIKeyNotFound(t *testing.T) {
	env := newConvTestServer(t)
	defer env.srv.Close()

	_, envBody := do(t, env.srv, "POST", "/api/v1/conversations", map[string]any{"title": "x"})
	id := dataMap(t, envBody)["id"].(string)

	status, envBody := do(t, env.srv, "PATCH", "/api/v1/conversations/"+id, map[string]any{
		"modelOverride": map[string]any{
			"apiKeyId": "aki_nonexistent",
			"modelId":  "gpt-4o",
		},
	})
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if code := errorCode(t, envBody); code != "API_KEY_NOT_FOUND" {
		t.Errorf("code = %q, want API_KEY_NOT_FOUND", code)
	}
}

func TestConvHandler_PatchModelOverride_WithThinking_Persisted(t *testing.T) {
	env := newConvTestServer(t)
	defer env.srv.Close()

	_, envBody := do(t, env.srv, "POST", "/api/v1/conversations", map[string]any{"title": "thinking-test"})
	id := dataMap(t, envBody)["id"].(string)

	status, envBody := do(t, env.srv, "PATCH", "/api/v1/conversations/"+id, map[string]any{
		"modelOverride": map[string]any{
			"apiKeyId": env.apiKeyID,
			"modelId":  "claude-sonnet-4-5",
			"thinking": map[string]any{"mode": "on", "effort": "high"},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %+v", status, envBody)
	}
	d := dataMap(t, envBody)
	mo, ok := d["modelOverride"].(map[string]any)
	if !ok {
		t.Fatalf("modelOverride missing or wrong shape: %+v", d["modelOverride"])
	}
	thinking, ok := mo["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("modelOverride.thinking missing or wrong shape: %+v", mo["thinking"])
	}
	if got := thinking["mode"].(string); got != "on" {
		t.Errorf("thinking.mode = %q, want on", got)
	}
	if got := thinking["effort"].(string); got != "high" {
		t.Errorf("thinking.effort = %q, want high", got)
	}
}
