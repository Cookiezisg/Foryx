package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func newTestDeps() Deps {
	return Deps{Log: zap.NewNop()}
}

func TestRouter_HealthEndpointReturnsEnvelope(t *testing.T) {
	h := New(newTestDeps())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rec.Code)
	}

	var env struct {
		Data struct {
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if env.Data.Status != "ok" {
		t.Errorf("status: got %q, want ok", env.Data.Status)
	}
}

func TestRouter_UnknownPathReturnsEnvelope404(t *testing.T) {
	h := New(newTestDeps())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/totally-nonexistent", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "404 page not found") {
		t.Errorf("leaked Go's default 404 body: %s", rec.Body.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if env.Error.Code != "NOT_FOUND" {
		t.Errorf("error code: got %q, want NOT_FOUND", env.Error.Code)
	}
}

func TestRouter_CORSPreflightWorks(t *testing.T) {
	h := New(newTestDeps())
	req := httptest.NewRequest("OPTIONS", "/api/v1/health", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "GET")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status: got %d, want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("CORS middleware not wired: missing Allow-Origin")
	}
}

func TestRouter_CORSHeaderPresentOnHealthRequest(t *testing.T) {
	h := New(newTestDeps())
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("missing Allow-Origin on passed-through request")
	}
}

func TestRouter_UserIDInjectedIntoHandlerContext(t *testing.T) {
	var gotID string
	var gotOK bool
	testHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotID, gotOK = reqctxpkg.GetUserID(r.Context())
	})

	h := applyChain(testHandler, newTestDeps())
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/anything", nil))

	if !gotOK {
		t.Fatalf("GetUserID ok: got false — InjectUserID not wired")
	}
	if gotID != reqctxpkg.DefaultLocalUserID {
		t.Errorf("userID: got %q, want %q", gotID, reqctxpkg.DefaultLocalUserID)
	}
}

func TestRouter_LocaleInjectedIntoHandlerContext(t *testing.T) {
	var gotLocale reqctxpkg.Locale
	testHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotLocale = reqctxpkg.GetLocale(r.Context())
	})

	h := applyChain(testHandler, newTestDeps())
	req := httptest.NewRequest("GET", "/anything", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if gotLocale != reqctxpkg.LocaleEn {
		t.Errorf("locale: got %q, want %q", gotLocale, reqctxpkg.LocaleEn)
	}
}
