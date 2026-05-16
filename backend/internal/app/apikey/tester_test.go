package apikey

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
)

func newTester() *HTTPTester {
	return NewHTTPTester(&http.Client{Timeout: 2 * time.Second})
}

func TestHTTPTester_OpenAICompatible_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/models" {
			t.Errorf("path = %q, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization = %q, want Bearer sk-test", got)
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]string{{"id": "gpt-4o"}, {"id": "gpt-3.5-turbo"}},
		})
	}))
	defer srv.Close()

	res, err := newTester().Test(context.Background(), "openai", "sk-test", srv.URL, "")
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !res.OK {
		t.Fatalf("OK=false, want true: msg=%q", res.Message)
	}
	if len(res.ModelsFound) != 2 {
		t.Errorf("ModelsFound = %v, want 2 entries", res.ModelsFound)
	}
	if res.LatencyMs < 0 {
		t.Errorf("LatencyMs = %d, want >= 0", res.LatencyMs)
	}
}

func TestHTTPTester_OpenAICompatible_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	res, err := newTester().Test(context.Background(), "openai", "sk-bad", srv.URL, "")
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if res.OK {
		t.Fatalf("OK=true, want false")
	}
	if !strings.Contains(res.Message, "401") {
		t.Errorf("Message = %q, want to contain 401", res.Message)
	}
}

func TestHTTPTester_OpenAICompatible_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	res, err := newTester().Test(context.Background(), "openai", "sk", srv.URL, "")
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if res.OK {
		t.Errorf("OK=true on 500, want false")
	}
	if !strings.Contains(res.Message, "500") {
		t.Errorf("Message = %q, want to contain 500", res.Message)
	}
}

func TestHTTPTester_Anthropic_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
			t.Errorf("want POST /v1/messages, got %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "sk-ant-test" {
			t.Errorf("x-api-key = %q, want sk-ant-test", got)
		}
		if got := r.Header.Get("anthropic-version"); got == "" {
			t.Error("anthropic-version header missing")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg_abc","content":[{"type":"text","text":"."}]}`))
	}))
	defer srv.Close()

	res, err := newTester().Test(context.Background(), "anthropic", "sk-ant-test", srv.URL, "")
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !res.OK {
		t.Fatalf("OK=false, want true: msg=%q", res.Message)
	}
	if len(res.ModelsFound) != 0 {
		t.Errorf("Anthropic ping returns no model list, got %v", res.ModelsFound)
	}
}

func TestHTTPTester_Anthropic_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	res, err := newTester().Test(context.Background(), "anthropic", "sk-ant-bad", srv.URL, "")
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if res.OK {
		t.Errorf("OK=true, want false")
	}
	if !strings.Contains(res.Message, "401") {
		t.Errorf("Message = %q, want to contain 401", res.Message)
	}
}

func TestHTTPTester_Google_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models" {
			t.Errorf("path = %q, want /v1beta/models", r.URL.Path)
		}
		if got := r.URL.Query().Get("key"); got != "gk-test" {
			t.Errorf("query key = %q, want gk-test", got)
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Google uses query param, Authorization header must be empty, got %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-pro"},{"name":"models/gemini-1.5-flash"}]}`))
	}))
	defer srv.Close()

	res, err := newTester().Test(context.Background(), "google", "gk-test", srv.URL, "")
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !res.OK {
		t.Fatalf("OK=false: %q", res.Message)
	}
	if len(res.ModelsFound) != 2 {
		t.Errorf("ModelsFound = %v, want 2", res.ModelsFound)
	}
}

func TestHTTPTester_Google_EscapesKeyInQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("key"); got != "a/b=c&d" {
			t.Errorf("server got key=%q, want a/b=c&d (escaping broke round-trip)", got)
		}
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	if _, err := newTester().Test(context.Background(), "google", "a/b=c&d", srv.URL, ""); err != nil {
		t.Fatalf("Test: %v", err)
	}
}

func TestHTTPTester_Ollama_Success_NoAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("path = %q, want /api/tags", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Ollama must not send auth, got %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3:latest"},{"name":"mistral"}]}`))
	}))
	defer srv.Close()

	res, err := newTester().Test(context.Background(), "ollama", "", srv.URL, "")
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !res.OK {
		t.Fatalf("OK=false: %q", res.Message)
	}
	if len(res.ModelsFound) != 2 {
		t.Errorf("ModelsFound = %v, want 2", res.ModelsFound)
	}
}

func TestHTTPTester_Custom_RoutesByAPIFormat(t *testing.T) {
	t.Run("anthropic-compatible → POST /v1/messages", func(t *testing.T) {
		hit := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/v1/messages" {
				hit = true
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer srv.Close()

		if _, err := newTester().Test(context.Background(), "custom", "k", srv.URL, apikeydomain.APIFormatAnthropicCompatible); err != nil {
			t.Fatalf("Test: %v", err)
		}
		if !hit {
			t.Error("expected POST /v1/messages for anthropic-compatible custom")
		}
	})

	t.Run("openai-compatible → GET /models", func(t *testing.T) {
		hit := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/models" {
				hit = true
			}
			_, _ = w.Write([]byte(`{"data":[]}`))
		}))
		defer srv.Close()

		if _, err := newTester().Test(context.Background(), "custom", "k", srv.URL, "openai-compatible"); err != nil {
			t.Fatalf("Test: %v", err)
		}
		if !hit {
			t.Error("expected GET /models for openai-compatible custom")
		}
	})

	t.Run("empty apiFormat defaults to openai-compatible", func(t *testing.T) {
		hit := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/models" {
				hit = true
			}
			_, _ = w.Write([]byte(`{"data":[]}`))
		}))
		defer srv.Close()

		if _, err := newTester().Test(context.Background(), "custom", "k", srv.URL, ""); err != nil {
			t.Fatalf("Test: %v", err)
		}
		if !hit {
			t.Error("empty apiFormat should default to openai-compatible (GET /models)")
		}
	})
}

func TestHTTPTester_NetworkError_SurfacesInResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	res, err := newTester().Test(context.Background(), "openai", "k", srv.URL, "")
	if err != nil {
		t.Fatalf("Test: %v (network failure must surface in TestResult, not err)", err)
	}
	if res.OK {
		t.Fatalf("OK=true against closed server, want false")
	}
	if !strings.Contains(res.Message, "connect") {
		t.Errorf("Message = %q, want to mention 'connect'", res.Message)
	}
}

func TestHTTPTester_ContextCancel_SurfacesInResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	res, err := newTester().Test(ctx, "openai", "k", srv.URL, "")
	if err != nil {
		t.Fatalf("Test: %v (ctx cancel must surface in TestResult, not err)", err)
	}
	if res.OK {
		t.Errorf("OK=true on cancelled ctx, want false")
	}
}

func TestHTTPTester_200_WithMalformedJSON_IsStillOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	res, err := newTester().Test(context.Background(), "openai", "k", srv.URL, "")
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !res.OK {
		t.Errorf("OK=false on 200+garbage, want true (connectivity still worked)")
	}
	if len(res.ModelsFound) != 0 {
		t.Errorf("ModelsFound = %v, want empty on malformed JSON", res.ModelsFound)
	}
}

func TestHTTPTester_BaseURLTrailingSlash_DoesNotDoubleUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path = %q, want exactly /models (no double slash)", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	if _, err := newTester().Test(context.Background(), "openai", "k", srv.URL+"/", ""); err != nil {
		t.Fatalf("Test: %v", err)
	}
}

func TestHTTPTester_UnknownProvider_ReturnsErrInvalidProvider(t *testing.T) {
	_, err := newTester().Test(context.Background(), "not-a-provider", "k", "http://x", "")
	if err == nil {
		t.Fatal("want error for unknown provider")
	}
	if !errors.Is(err, apikeydomain.ErrInvalidProvider) {
		t.Errorf("err = %v, want to wrap ErrInvalidProvider", err)
	}
}

func TestHTTPTester_OllamaMissingBaseURL_ReturnsErrBaseURLRequired(t *testing.T) {
	_, err := newTester().Test(context.Background(), "ollama", "", "", "")
	if err == nil {
		t.Fatal("want error when Ollama baseURL is missing")
	}
	if !errors.Is(err, apikeydomain.ErrBaseURLRequired) {
		t.Errorf("err = %v, want to wrap ErrBaseURLRequired", err)
	}
}

func TestHTTPTester_CustomMissingBaseURL_ReturnsErrBaseURLRequired(t *testing.T) {
	_, err := newTester().Test(context.Background(), "custom", "k", "", "")
	if err == nil {
		t.Fatal("want error when custom baseURL is missing")
	}
	if !errors.Is(err, apikeydomain.ErrBaseURLRequired) {
		t.Errorf("err = %v, want to wrap ErrBaseURLRequired", err)
	}
}

func TestHTTPTester_ProviderDefaultBaseURL_UsedWhenUserEmpty(t *testing.T) {
	tester := NewHTTPTester(&http.Client{Timeout: 1 * time.Millisecond})
	res, err := tester.Test(context.Background(), "openai", "k", "", "")
	if err != nil {
		t.Fatalf("Test: got err %v, want TestResult (means default-baseURL branch wasn't reached)", err)
	}
	if res.OK {
		t.Fatalf("OK=true on 1ms timeout, want false")
	}
}
