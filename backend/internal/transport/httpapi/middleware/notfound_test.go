package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type envelopeShape struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestNotFound_ReturnsEnvelope404(t *testing.T) {
	rec := httptest.NewRecorder()
	NotFound(rec, httptest.NewRequest("GET", "/api/v1/missing", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type: got %q, want application/json", ct)
	}

	var env envelopeShape
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not valid JSON: %v", err)
	}
	if env.Error.Code != "NOT_FOUND" {
		t.Errorf("error.code: got %q, want NOT_FOUND", env.Error.Code)
	}
}

func TestNotFound_IncludesPathInMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	NotFound(rec, httptest.NewRequest("GET", "/api/v1/totally-made-up", nil))

	var env envelopeShape
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if !strings.Contains(env.Error.Message, "/api/v1/totally-made-up") {
		t.Errorf("message should contain path, got: %q", env.Error.Message)
	}
}

func TestNotFound_WorksForAnyMethod(t *testing.T) {
	methods := []string{"GET", "POST", "PATCH", "PUT", "DELETE"}
	for _, m := range methods {
		t.Run(m, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NotFound(rec, httptest.NewRequest(m, "/missing", nil))

			if rec.Code != http.StatusNotFound {
				t.Errorf("%s: status got %d, want 404", m, rec.Code)
			}
			var env envelopeShape
			_ = json.Unmarshal(rec.Body.Bytes(), &env)
			if env.Error.Code != "NOT_FOUND" {
				t.Errorf("%s: code got %q, want NOT_FOUND", m, env.Error.Code)
			}
		})
	}
}

func TestNotFound_DoesNotLeakGoDefault(t *testing.T) {
	rec := httptest.NewRecorder()
	NotFound(rec, httptest.NewRequest("GET", "/x", nil))

	body := rec.Body.String()
	if strings.Contains(body, "404 page not found") {
		t.Errorf("response contains Go's default 404 text, envelope not applied: %q", body)
	}
	if !strings.HasPrefix(strings.TrimSpace(body), "{") {
		t.Errorf("response is not JSON: %q", body)
	}
}
