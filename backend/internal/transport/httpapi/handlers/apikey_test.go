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

type fakeTester struct {
	result *apikeyapp.TestResult
	err    error
}

func (t *fakeTester) Test(ctx context.Context, provider, key, baseURL, apiFormat string) (*apikeyapp.TestResult, error) {
	return t.result, t.err
}

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

	return httptest.NewServer(middlewarehttpapi.InjectUserID(mux))
}

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

func dataMap(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	d, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("data is not a map: %+v", env)
	}
	return d
}

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

func errorCode(t *testing.T, env map[string]any) string {
	t.Helper()
	e, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error in envelope: %+v", env)
	}
	return e["code"].(string)
}


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


func TestAPIKeyHandler_List_Paged(t *testing.T) {
	srv := newTestServer(t, &fakeTester{})
	defer srv.Close()

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

	status, env = do(t, srv, "DELETE", "/api/v1/api-keys/"+id, nil)
	if status != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404", status)
	}
	if code := errorCode(t, env); code != "API_KEY_NOT_FOUND" {
		t.Errorf("code = %q, want API_KEY_NOT_FOUND", code)
	}
}


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

	status, env := do(t, srv, "POST", "/api/v1/api-keys/anyid:rotate", nil)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if code := errorCode(t, env); code != "NOT_FOUND" {
		t.Errorf("code = %q, want NOT_FOUND", code)
	}
}


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
