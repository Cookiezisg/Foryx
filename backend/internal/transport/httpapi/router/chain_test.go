package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// TestChainExemptVsGuarded verifies the workspace gate: with a nil resolver no workspace is
// ever stamped, so guarded routes 401 while exempt (onboarding/liveness/static) routes pass.
//
// TestChainExemptVsGuarded 验证 workspace 门：resolver 为 nil 时永不写入 workspace，故受守
// 路由 401，而豁免（onboarding/健康检查/静态）路由放过。
func TestChainExemptVsGuarded(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := Chain(inner, zap.NewNop(), nil, "") // "" token → bearer gate off for this workspace-gate test

	cases := []struct {
		path string
		want int
	}{
		{"/api/v1/health", http.StatusOK},                  // liveness — exempt
		{"/api/v1/workspaces", http.StatusOK},              // onboarding — exempt
		{"/api/v1/providers", http.StatusOK},               // static metadata — exempt
		{"/api/v1/scenarios", http.StatusOK},               // static metadata — exempt
		{"/api/v1/conversations", http.StatusUnauthorized}, // guarded, no workspace → 401
		{"/api/v1/webhooks/trg_x/push", http.StatusOK},     // external webhook — exempt (own secret/HMAC auth)
		{"/healthz", http.StatusOK},                        // non-/api/v1 passes through to inner
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, loopback("GET", c.path))
		if w.Code != c.want {
			t.Errorf("%s → %d, want %d", c.path, w.Code, c.want)
		}
	}
}

// TestChain_MuxErrorsEnveloped — F172: the stdlib ServeMux's plain-text 404 (unknown route) / 405 (wrong
// method) on /api/v1/* must be rewritten to the N1 error envelope, so a client that always parses
// {"error":{code,message}} doesn't hit a JSON parse error. Non-/api/v1 paths stay stdlib plaintext.
func TestChain_MuxErrorsEnveloped(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/workspaces", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := Chain(mux, zap.NewNop(), nil, "") // /api/v1/workspaces* exempt → no 401 first; "" token → no bearer gate

	// 404: unknown route under /api/v1/* → N1 envelope, not "404 page not found" plaintext.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, loopback("GET", "/api/v1/workspaces/does-not-exist"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown /api/v1 route → %d, want 404", w.Code)
	}
	assertErrorEnvelope(t, w.Body.Bytes(), "ROUTE_NOT_FOUND")

	// 405: known path, wrong method → N1 envelope.
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, loopback("DELETE", "/api/v1/workspaces"))
	if w2.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method on /api/v1 path → %d, want 405", w2.Code)
	}
	assertErrorEnvelope(t, w2.Body.Bytes(), "METHOD_NOT_ALLOWED")

	// A non-/api/v1 404 must stay stdlib plaintext (the contract only governs /api/v1/*).
	w3 := httptest.NewRecorder()
	h.ServeHTTP(w3, loopback("GET", "/nope"))
	if w3.Code != http.StatusNotFound || strings.Contains(w3.Body.String(), "ROUTE_NOT_FOUND") {
		t.Fatalf("non-/api/v1 404 must stay stdlib plaintext, got %d %q", w3.Code, w3.Body.String())
	}
}

// loopback builds a request whose Host passes RequireLoopbackHost — httptest's default Host is
// "example.com", which the loopback gate (correctly) 403s. 让请求带 loopback Host(否则被 host 门 403)。
func loopback(method, target string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.Host = "127.0.0.1"
	return r
}

func assertErrorEnvelope(t *testing.T, body []byte, wantCode string) {
	t.Helper()
	var env struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("response must be the N1 JSON envelope, got non-JSON %q (err %v)", body, err)
	}
	if env.Error == nil || env.Error.Code != wantCode {
		t.Fatalf("envelope error code = %+v, want %q", env.Error, wantCode)
	}
}
