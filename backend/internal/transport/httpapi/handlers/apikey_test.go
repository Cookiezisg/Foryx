// apikey_test.go — E2E contract tests for the 5 /api/v1/api-keys/* endpoints.
// Each test builds a full stack: in-memory SQLite → real Store → real
// AES-GCM Encryptor → fake ConnectivityTester → Service → Handler, wrapped
// by InjectUserID middleware so reqctx.GetUserID works. Only the external
// LLM probe is faked — everything else is real code paths.
//
// apikey_test.go — /api/v1/api-keys/* 5 个端点的端到端契约测试。
// 每个 case 起完整栈：内存 SQLite → 真 Store → 真 AES-GCM Encryptor →
// fake ConnectivityTester → Service → Handler，用 InjectUserID 中间件包裹
// 让 reqctx.GetUserID 能工作。只 mock 上游 LLM 探测，其他全是真代码路径。

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"
	gormlogger "gorm.io/gorm/logger"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	cryptoinfra "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	apikeystore "github.com/sunweilin/forgify/backend/internal/infra/store/apikey"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

// fakeTester returns pre-canned TestResult / error.
//
// fakeTester 返回预设的 TestResult / error。
type fakeTester struct {
	result *apikeyapp.TestResult
	err    error
}

func (t *fakeTester) Test(ctx context.Context, provider, key, baseURL, apiFormat string) (*apikeyapp.TestResult, error) {
	return t.result, t.err
}

// newTestServer spins up a full stack as an httptest.Server.
//
// newTestServer 起完整栈的 httptest.Server。
func newTestServer(t *testing.T, tester apikeyapp.ConnectivityTester) *httptest.Server {
	t.Helper()

	gdb, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("dbinfra.Open: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(gdb) })
	if err := dbinfra.Migrate(gdb, &apikeydomain.APIKey{}); err != nil {
		t.Fatalf("dbinfra.Migrate: %v", err)
	}

	enc, err := cryptoinfra.NewAESGCMEncryptor(cryptoinfra.DeriveKey("handler-test-fixture"))
	if err != nil {
		t.Fatalf("NewAESGCMEncryptor: %v", err)
	}

	log := zaptest.NewLogger(t)
	svc := apikeyapp.NewService(apikeystore.New(gdb), enc, tester, log)
	h := NewAPIKeyHandler(svc, log)

	mux := http.NewServeMux()
	h.Register(mux)

	// Wrap with InjectUserID so reqctx.GetUserID returns DefaultLocalUserID.
	// 用 InjectUserID 包裹，让 reqctx.GetUserID 返回 DefaultLocalUserID。
	return httptest.NewServer(middlewarehttpapi.InjectUserID(mux))
}

// do is a small helper: serialize body, fire request, decode envelope.
//
// do 是小工具：序列化 body、发请求、解 envelope。
func do(t *testing.T, srv *httptest.Server, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, srv.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("content-type", "application/json")
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, nil
	}
	var env map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return resp.StatusCode, env
}

// dataMap extracts env["data"] as map (panics if unexpected shape).
//
// dataMap 提取 env["data"] 为 map（形状不对则 panic）。
func dataMap(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	d, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data is not a map: %+v", env)
	}
	return d
}

// dataSlice extracts env["data"] as slice (panics if unexpected shape).
// Tolerates nil (returns []any{}) so callers can len() unconditionally.
//
// dataSlice 提取 env["data"] 为 slice（形状不对则 panic）。
// 容忍 nil（返 []any{}）让调用方无脑 len()。
func dataSlice(t *testing.T, env map[string]any) []any {
	t.Helper()
	if env["data"] == nil {
		return []any{}
	}
	d, ok := env["data"].([]any)
	if !ok {
		t.Fatalf("data is not a slice: %+v", env)
	}
	return d
}

// errorCode extracts env["error"]["code"].
//
// errorCode 提取 env["error"]["code"]。
func errorCode(t *testing.T, env map[string]any) string {
	t.Helper()
	e, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error in envelope: %+v", env)
	}
	return e["code"].(string)
}

// ---- POST /api/v1/api-keys ----

func TestAPIKeyHandler_Create_Success(t *testing.T) {
	srv := newTestServer(t, &fakeTester{})
	defer srv.Close()

	status, env := do(t, srv, "POST", "/api/v1/api-keys", map[string]any{
		"provider":    "openai",
		"displayName": "Main",
		"key":         "sk-proj-abcdefg1234567890xyz",
	})

	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %+v", status, env)
	}
	d := dataMap(t, env)
	if got := d["keyMasked"].(string); got != "sk-proj...0xyz" {
		t.Errorf("keyMasked = %q, want sk-proj...0xyz", got)
	}
	if got := d["provider"].(string); got != "openai" {
		t.Errorf("provider = %q, want openai", got)
	}
	if got := d["testStatus"].(string); got != apikeydomain.TestStatusPending {
		t.Errorf("testStatus = %q, want pending", got)
	}
	// KeyEncrypted must not leak on the wire (json:"-" tag).
	// KeyEncrypted 不得泄漏到线上（json:"-" 标签）。
	if _, hasCT := d["keyEncrypted"]; hasCT {
		t.Error("keyEncrypted leaked into response — json:-\" tag broken")
	}
}

func TestAPIKeyHandler_Create_InvalidProvider(t *testing.T) {
	srv := newTestServer(t, &fakeTester{})
	defer srv.Close()

	status, env := do(t, srv, "POST", "/api/v1/api-keys", map[string]any{
		"provider": "notreal",
		"key":      "sk",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if code := errorCode(t, env); code != "INVALID_PROVIDER" {
		t.Errorf("code = %q, want INVALID_PROVIDER", code)
	}
}

func TestAPIKeyHandler_Create_MissingKey(t *testing.T) {
	srv := newTestServer(t, &fakeTester{})
	defer srv.Close()

	status, env := do(t, srv, "POST", "/api/v1/api-keys", map[string]any{
		"provider": "openai",
		"key":      "  ",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if code := errorCode(t, env); code != "KEY_REQUIRED" {
		t.Errorf("code = %q, want KEY_REQUIRED", code)
	}
}

func TestAPIKeyHandler_Create_MalformedJSON(t *testing.T) {
	srv := newTestServer(t, &fakeTester{})
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/api-keys",
		strings.NewReader("{not valid json"))
	req.Header.Set("content-type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	var env map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&env)
	if code := errorCode(t, env); code != "INVALID_REQUEST" {
		t.Errorf("code = %q, want INVALID_REQUEST", code)
	}
}

// ---- GET /api/v1/api-keys ----

func TestAPIKeyHandler_List_Paged(t *testing.T) {
	srv := newTestServer(t, &fakeTester{})
	defer srv.Close()

	// Seed two keys via the API.
	// 通过 API 种入两条 Key。
	for _, p := range []string{"openai", "anthropic"} {
		if st, env := do(t, srv, "POST", "/api/v1/api-keys", map[string]any{
			"provider": p, "key": "sk-" + p,
		}); st != 201 {
			t.Fatalf("seed %s: %d %+v", p, st, env)
		}
	}

	status, env := do(t, srv, "GET", "/api/v1/api-keys", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	items, ok := env["data"].([]any)
	if !ok {
		t.Fatalf("data is not an array: %+v", env["data"])
	}
	if len(items) != 2 {
		t.Errorf("len(data) = %d, want 2", len(items))
	}
	if _, has := env["hasMore"]; !has {
		t.Error("paged envelope missing hasMore field")
	}
}

func TestAPIKeyHandler_List_FilterByProvider(t *testing.T) {
	srv := newTestServer(t, &fakeTester{})
	defer srv.Close()

	for _, p := range []string{"openai", "anthropic", "deepseek"} {
		do(t, srv, "POST", "/api/v1/api-keys", map[string]any{"provider": p, "key": "sk"})
	}

	status, env := do(t, srv, "GET", "/api/v1/api-keys?provider=openai", nil)
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	items := env["data"].([]any)
	if len(items) != 1 {
		t.Errorf("filtered count = %d, want 1", len(items))
	}
	if p := items[0].(map[string]any)["provider"]; p != "openai" {
		t.Errorf("provider = %v, want openai", p)
	}
}

func TestAPIKeyHandler_List_InvalidLimit(t *testing.T) {
	srv := newTestServer(t, &fakeTester{})
	defer srv.Close()

	status, env := do(t, srv, "GET", "/api/v1/api-keys?limit=-1", nil)
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
	if code := errorCode(t, env); code != "INVALID_REQUEST" {
		t.Errorf("code = %q, want INVALID_REQUEST", code)
	}
}

// ---- PATCH /api/v1/api-keys/{id} ----

func TestAPIKeyHandler_Update_PartialFields(t *testing.T) {
	srv := newTestServer(t, &fakeTester{})
	defer srv.Close()

	_, env := do(t, srv, "POST", "/api/v1/api-keys", map[string]any{
		"provider": "openai", "displayName": "Old", "key": "sk",
	})
	id := dataMap(t, env)["id"].(string)

	status, env := do(t, srv, "PATCH", "/api/v1/api-keys/"+id, map[string]any{
		"displayName": "New Name",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	d := dataMap(t, env)
	if got := d["displayName"].(string); got != "New Name" {
		t.Errorf("displayName = %q, want New Name", got)
	}
}

func TestAPIKeyHandler_Update_NotFound(t *testing.T) {
	srv := newTestServer(t, &fakeTester{})
	defer srv.Close()

	name := "x"
	status, env := do(t, srv, "PATCH", "/api/v1/api-keys/nope", map[string]any{"displayName": name})
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if code := errorCode(t, env); code != "API_KEY_NOT_FOUND" {
		t.Errorf("code = %q, want API_KEY_NOT_FOUND", code)
	}
}

// ---- DELETE /api/v1/api-keys/{id} ----

func TestAPIKeyHandler_Delete_Success(t *testing.T) {
	srv := newTestServer(t, &fakeTester{})
	defer srv.Close()

	_, env := do(t, srv, "POST", "/api/v1/api-keys", map[string]any{
		"provider": "openai", "key": "sk",
	})
	id := dataMap(t, env)["id"].(string)

	status, _ := do(t, srv, "DELETE", "/api/v1/api-keys/"+id, nil)
	if status != http.StatusNoContent {
		t.Errorf("status = %d, want 204", status)
	}

	// Verify gone: second DELETE must 404.
	// 验证已删：第二次 DELETE 必须 404。
	status, env = do(t, srv, "DELETE", "/api/v1/api-keys/"+id, nil)
	if status != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404", status)
	}
	if code := errorCode(t, env); code != "API_KEY_NOT_FOUND" {
		t.Errorf("code = %q, want API_KEY_NOT_FOUND", code)
	}
}

// ---- POST /api/v1/api-keys/{id}:test ----

func TestAPIKeyHandler_Test_Success(t *testing.T) {
	tester := &fakeTester{result: &apikeyapp.TestResult{
		OK: true, Message: "connected", LatencyMs: 123,
		ModelsFound: []string{"gpt-4o", "gpt-3.5-turbo"},
	}}
	srv := newTestServer(t, tester)
	defer srv.Close()

	_, env := do(t, srv, "POST", "/api/v1/api-keys", map[string]any{
		"provider": "openai", "key": "sk",
	})
	id := dataMap(t, env)["id"].(string)

	status, env := do(t, srv, "POST", "/api/v1/api-keys/"+id+":test", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %+v", status, env)
	}
	d := dataMap(t, env)
	if ok := d["ok"].(bool); !ok {
		t.Errorf("ok = false, want true")
	}
	if lat := d["latencyMs"].(float64); lat != 123 {
		t.Errorf("latencyMs = %v, want 123", lat)
	}
	models := d["modelsFound"].([]any)
	if len(models) != 2 {
		t.Errorf("modelsFound = %v, want 2 entries", models)
	}
}

func TestAPIKeyHandler_Test_ConnectivityFails_Returns422(t *testing.T) {
	tester := &fakeTester{result: &apikeyapp.TestResult{
		OK: false, Message: "HTTP 401: invalid key", LatencyMs: 80,
	}}
	srv := newTestServer(t, tester)
	defer srv.Close()

	_, env := do(t, srv, "POST", "/api/v1/api-keys", map[string]any{
		"provider": "openai", "key": "sk",
	})
	id := dataMap(t, env)["id"].(string)

	status, env := do(t, srv, "POST", "/api/v1/api-keys/"+id+":test", nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", status)
	}
	if code := errorCode(t, env); code != "API_KEY_TEST_FAILED" {
		t.Errorf("code = %q, want API_KEY_TEST_FAILED", code)
	}
	// Details must carry latency for UI display.
	// details 必须带 latency 给 UI 展示。
	errObj := env["error"].(map[string]any)
	details, ok := errObj["details"].(map[string]any)
	if !ok {
		t.Fatalf("details missing: %+v", errObj)
	}
	if lat := details["latencyMs"].(float64); lat != 80 {
		t.Errorf("latencyMs = %v, want 80", lat)
	}
}

func TestAPIKeyHandler_Test_NotFound(t *testing.T) {
	srv := newTestServer(t, &fakeTester{})
	defer srv.Close()

	status, env := do(t, srv, "POST", "/api/v1/api-keys/nope:test", nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if code := errorCode(t, env); code != "API_KEY_NOT_FOUND" {
		t.Errorf("code = %q, want API_KEY_NOT_FOUND", code)
	}
}

func TestAPIKeyHandler_Test_UnknownAction_Returns404(t *testing.T) {
	srv := newTestServer(t, &fakeTester{})
	defer srv.Close()

	// POST /{id}:rotate is not defined.
	// POST /{id}:rotate 未定义。
	status, env := do(t, srv, "POST", "/api/v1/api-keys/anyid:rotate", nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if code := errorCode(t, env); code != "NOT_FOUND" {
		t.Errorf("code = %q, want NOT_FOUND", code)
	}
}

// ---- end-to-end roundtrip: create → test OK → verify test_status ----

func TestAPIKeyHandler_RoundTrip_CreateTestListReflectsNewStatus(t *testing.T) {
	tester := &fakeTester{result: &apikeyapp.TestResult{OK: true, Message: "ok"}}
	srv := newTestServer(t, tester)
	defer srv.Close()

	_, env := do(t, srv, "POST", "/api/v1/api-keys", map[string]any{
		"provider": "openai", "key": "sk",
	})
	id := dataMap(t, env)["id"].(string)

	if st, _ := do(t, srv, "POST", fmt.Sprintf("/api/v1/api-keys/%s:test", id), nil); st != 200 {
		t.Fatalf("test endpoint: %d", st)
	}

	_, env = do(t, srv, "GET", "/api/v1/api-keys", nil)
	items := env["data"].([]any)
	first := items[0].(map[string]any)
	if got := first["testStatus"].(string); got != apikeydomain.TestStatusOK {
		t.Errorf("testStatus after test = %q, want ok", got)
	}
}
