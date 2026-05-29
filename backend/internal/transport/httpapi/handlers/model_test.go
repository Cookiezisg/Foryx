package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"
	gormlogger "gorm.io/gorm/logger"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	cryptoinfra "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	apikeystore "github.com/sunweilin/forgify/backend/internal/infra/store/apikey"
	modelstore "github.com/sunweilin/forgify/backend/internal/infra/store/model"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

// modelTestEnv carries the test server plus the seed apikey service so tests
// can mint additional api_keys that satisfy Upsert F1's ResolveCredentialsByID.
//
// modelTestEnv 携测试服务 + apikey service;让测试种额外 api_key 满足 F1。
type modelTestEnv struct {
	srv       *httptest.Server
	apikeySvc *apikeyapp.Service
	apiKeyID  string
}

// newModelTestServer wires a real apikey Service as KeyProvider so model
// Upsert's F1 (api_key must exist + belong to user) reflects production.
//
// newModelTestServer 装配真 apikey Service 作 KeyProvider,让 F1 路径与生产一致。
func newModelTestServer(t *testing.T) *modelTestEnv {
	t.Helper()
	gdb, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("dbinfra.Open: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(gdb) })
	if err := dbinfra.Migrate(gdb, &modeldomain.ModelConfig{}, &apikeydomain.APIKey{}); err != nil {
		t.Fatalf("dbinfra.Migrate: %v", err)
	}
	log := zaptest.NewLogger(t)

	enc, err := cryptoinfra.NewAESGCMEncryptor(cryptoinfra.DeriveKey("model-handler-test"))
	if err != nil {
		t.Fatalf("NewAESGCMEncryptor: %v", err)
	}
	apikeySvc := apikeyapp.NewService(apikeystore.New(gdb), enc, &fakeTester{}, log)
	modelSvc := modelapp.NewService(modelstore.New(gdb), apikeySvc, log)
	h := NewModelConfigHandler(modelSvc, log)

	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(middlewarehttpapi.InjectUserID(mux))

	// Seed a usable api_key for the InjectUserID-stamped user; cross-user
	// isolation is covered in store unit tests.
	//
	// 给 InjectUserID 用户预置一把 api_key;跨用户隔离由 store 单测覆盖。
	ctx := reqctxpkg.SetUserID(context.Background(), "test-user")
	key, err := apikeySvc.Create(ctx, apikeyapp.CreateInput{
		Provider: "openai", Key: "sk-test-fixture", DisplayName: "fixture",
	})
	if err != nil {
		t.Fatalf("seed api_key: %v", err)
	}
	return &modelTestEnv{srv: srv, apikeySvc: apikeySvc, apiKeyID: key.ID}
}

// seedAnotherKey mints a second api_key for the same test user.
//
// seedAnotherKey 给同测试用户种第二把 api_key。
func (e *modelTestEnv) seedAnotherKey(t *testing.T, provider, displayName string) string {
	t.Helper()
	ctx := reqctxpkg.SetUserID(context.Background(), "test-user")
	k, err := e.apikeySvc.Create(ctx, apikeyapp.CreateInput{
		Provider: provider, Key: "sk-" + provider, DisplayName: displayName,
	})
	if err != nil {
		t.Fatalf("seed extra api_key: %v", err)
	}
	return k.ID
}

func TestModelHandler_List_EmptyReturnsArray(t *testing.T) {
	env := newModelTestServer(t)
	defer env.srv.Close()

	status, envBody := do(t, env.srv, "GET", "/api/v1/model-configs", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	items, ok := envBody["data"].([]any)
	if !ok {
		t.Fatalf("data is not an array: %+v", envBody["data"])
	}
	if len(items) != 0 {
		t.Errorf("len(data) = %d, want 0", len(items))
	}
}

func TestModelHandler_Upsert_Success(t *testing.T) {
	env := newModelTestServer(t)
	defer env.srv.Close()

	status, envBody := do(t, env.srv, "PUT", "/api/v1/model-configs/dialogue", map[string]any{
		"apiKeyId": env.apiKeyID,
		"modelId":  "gpt-4o",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %+v", status, envBody)
	}
	d := dataMap(t, envBody)
	if got := d["scenario"].(string); got != "dialogue" {
		t.Errorf("scenario = %q, want dialogue", got)
	}
	if got := d["apiKeyId"].(string); got != env.apiKeyID {
		t.Errorf("apiKeyId = %q, want %q", got, env.apiKeyID)
	}
	if got := d["modelId"].(string); got != "gpt-4o" {
		t.Errorf("modelId = %q, want gpt-4o", got)
	}
	if _, has := d["userId"]; has {
		t.Error("userId leaked into response")
	}
}

func TestModelHandler_Upsert_UpdateKeepsOneRow(t *testing.T) {
	env := newModelTestServer(t)
	defer env.srv.Close()
	second := env.seedAnotherKey(t, "anthropic", "second")

	do(t, env.srv, "PUT", "/api/v1/model-configs/dialogue", map[string]any{
		"apiKeyId": env.apiKeyID, "modelId": "gpt-4o",
	})
	do(t, env.srv, "PUT", "/api/v1/model-configs/dialogue", map[string]any{
		"apiKeyId": second, "modelId": "claude-3-5-sonnet-latest",
	})

	status, envBody := do(t, env.srv, "GET", "/api/v1/model-configs", nil)
	if status != http.StatusOK {
		t.Fatalf("GET status = %d", status)
	}
	items := envBody["data"].([]any)
	if len(items) != 1 {
		t.Errorf("len(data) = %d, want 1 after two PUTs on same scenario", len(items))
	}
	d := items[0].(map[string]any)
	if got := d["apiKeyId"].(string); got != second {
		t.Errorf("apiKeyId = %q, want %q (second PUT should win)", got, second)
	}
}

func TestModelHandler_Upsert_InvalidScenario(t *testing.T) {
	env := newModelTestServer(t)
	defer env.srv.Close()

	status, envBody := do(t, env.srv, "PUT", "/api/v1/model-configs/workflow_llm", map[string]any{
		"apiKeyId": env.apiKeyID, "modelId": "gpt-4o",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if code := errorCode(t, envBody); code != "INVALID_SCENARIO" {
		t.Errorf("code = %q, want INVALID_SCENARIO", code)
	}
}

func TestModelHandler_Upsert_APIKeyIDRequired(t *testing.T) {
	env := newModelTestServer(t)
	defer env.srv.Close()

	status, envBody := do(t, env.srv, "PUT", "/api/v1/model-configs/dialogue", map[string]any{
		"apiKeyId": "", "modelId": "gpt-4o",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if code := errorCode(t, envBody); code != "API_KEY_ID_REQUIRED" {
		t.Errorf("code = %q, want API_KEY_ID_REQUIRED", code)
	}
}

func TestModelHandler_Upsert_APIKeyNotFound(t *testing.T) {
	env := newModelTestServer(t)
	defer env.srv.Close()

	status, envBody := do(t, env.srv, "PUT", "/api/v1/model-configs/dialogue", map[string]any{
		"apiKeyId": "aki_nonexistent", "modelId": "gpt-4o",
	})
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if code := errorCode(t, envBody); code != "API_KEY_NOT_FOUND" {
		t.Errorf("code = %q, want API_KEY_NOT_FOUND", code)
	}
}

func TestModelHandler_Upsert_ModelIDRequired(t *testing.T) {
	env := newModelTestServer(t)
	defer env.srv.Close()

	status, envBody := do(t, env.srv, "PUT", "/api/v1/model-configs/dialogue", map[string]any{
		"apiKeyId": env.apiKeyID, "modelId": "",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if code := errorCode(t, envBody); code != "MODEL_ID_REQUIRED" {
		t.Errorf("code = %q, want MODEL_ID_REQUIRED", code)
	}
}

func TestModelHandler_Upsert_MalformedJSON(t *testing.T) {
	env := newModelTestServer(t)
	defer env.srv.Close()

	status, envBody := do(t, env.srv, "PUT", "/api/v1/model-configs/dialogue", "{bad json")
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if code := errorCode(t, envBody); code != "INVALID_REQUEST" {
		t.Errorf("code = %q, want INVALID_REQUEST", code)
	}
}

func TestModelHandler_Upsert_WithThinking_Persisted(t *testing.T) {
	env := newModelTestServer(t)
	defer env.srv.Close()

	status, envBody := do(t, env.srv, "PUT", "/api/v1/model-configs/dialogue", map[string]any{
		"apiKeyId": env.apiKeyID,
		"modelId":  "claude-sonnet-4-5",
		"thinking": map[string]any{"mode": "on", "effort": "high"},
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %+v", status, envBody)
	}
	d := dataMap(t, envBody)
	thinking, ok := d["thinking"].(map[string]any)
	if !ok {
		t.Fatalf("thinking missing or wrong shape: %+v", d["thinking"])
	}
	if got := thinking["mode"].(string); got != "on" {
		t.Errorf("thinking.mode = %q, want on", got)
	}
	if got := thinking["effort"].(string); got != "high" {
		t.Errorf("thinking.effort = %q, want high", got)
	}
}
