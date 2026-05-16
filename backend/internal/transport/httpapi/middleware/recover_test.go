package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newObservedLogger(t *testing.T) (*zap.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, obs := observer.New(zap.DebugLevel)
	return zap.New(core), obs
}

func handlerThatPanics(v any) http.Handler {
	return http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(v)
	})
}

func TestRecover_NormalRequestPassesThrough(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := Recover(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("brew"))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if rec.Code != http.StatusTeapot {
		t.Errorf("status: got %d, want 418", rec.Code)
	}
	if rec.Body.String() != "brew" {
		t.Errorf("body: got %q, want %q", rec.Body.String(), "brew")
	}
	if obs.Len() != 0 {
		t.Errorf("expected no log entries on normal request, got %d", obs.Len())
	}
}

func TestRecover_StringPanic(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := Recover(log)(handlerThatPanics("boom"))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	assertEnvelope500(t, rec)

	entries := obs.FilterMessage("panic recovered").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 panic log entry, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["panic"] != "boom" {
		t.Errorf("panic field: got %v, want \"boom\"", fields["panic"])
	}
	if !strings.Contains(fields["stack"].(string), "runtime/debug.Stack") &&
		!strings.Contains(fields["stack"].(string), "recover_test.go") {
		t.Errorf("stack field does not look like a real stack trace: %v", fields["stack"])
	}
}

func TestRecover_ErrorPanic(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := Recover(log)(handlerThatPanics(errors.New("db died")))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	assertEnvelope500(t, rec)

	entries := obs.FilterMessage("panic recovered").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 panic log entry, got %d", len(entries))
	}
	panicVal := entries[0].ContextMap()["panic"]
	if s, ok := panicVal.(string); !ok || !strings.Contains(s, "db died") {
		if e, ok := panicVal.(error); !ok || !strings.Contains(e.Error(), "db died") {
			t.Errorf("panic value does not carry 'db died': %v (%T)", panicVal, panicVal)
		}
	}
}

func TestRecover_RuntimePanic(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := Recover(log)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		xs := []int{}
		_ = xs[10]
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	assertEnvelope500(t, rec)

	if got := obs.FilterMessage("panic recovered").Len(); got != 1 {
		t.Errorf("expected 1 panic log, got %d", got)
	}
}

func TestRecover_DoesNotLeakPanicValueToClient(t *testing.T) {
	log, _ := newObservedLogger(t)
	secret := "SECRET_DATABASE_PASSWORD=hunter2"
	handler := Recover(log)(handlerThatPanics(secret))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("response leaks panic value to client: %s", rec.Body.String())
	}

	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if env.Error.Code != "INTERNAL_ERROR" {
		t.Errorf("error.code: got %q, want INTERNAL_ERROR", env.Error.Code)
	}
	if env.Error.Message != "internal server error" {
		t.Errorf("error.message: got %q, want generic message", env.Error.Message)
	}
}

func TestRecover_LogsRequestContext(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := Recover(log)(handlerThatPanics("x"))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/tools/123:run", nil))

	entries := obs.FilterMessage("panic recovered").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 panic log, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["method"] != "POST" {
		t.Errorf("method: got %v, want POST", fields["method"])
	}
	if fields["path"] != "/api/v1/tools/123:run" {
		t.Errorf("path: got %v, want /api/v1/tools/123:run", fields["path"])
	}
}

func assertEnvelope500(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type: got %q, want application/json", ct)
	}
	if !strings.Contains(rec.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Errorf("body missing INTERNAL_ERROR code: %s", rec.Body.String())
	}
}
