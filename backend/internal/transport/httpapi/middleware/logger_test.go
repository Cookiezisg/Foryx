package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestLogger_NormalRequestLogs200(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := RequestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/health", nil))

	entries := obs.FilterMessage("http request").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	fields := entries[0].ContextMap()

	if fields["method"] != "GET" {
		t.Errorf("method: got %v, want GET", fields["method"])
	}
	if fields["path"] != "/api/v1/health" {
		t.Errorf("path: got %v, want /api/v1/health", fields["path"])
	}
	if fields["status"] != int64(200) {
		t.Errorf("status: got %v, want 200", fields["status"])
	}
	if fields["bytes"] != int64(5) {
		t.Errorf("bytes: got %v, want 5", fields["bytes"])
	}
}

func TestRequestLogger_ImplicitStatus200(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := RequestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("implicit"))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	fields := obs.All()[0].ContextMap()
	if fields["status"] != int64(200) {
		t.Errorf("status: got %v, want 200 (default)", fields["status"])
	}
}

func TestRequestLogger_404Status(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := RequestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/missing", nil))

	fields := obs.All()[0].ContextMap()
	if fields["status"] != int64(404) {
		t.Errorf("status: got %v, want 404", fields["status"])
	}
}

func TestRequestLogger_BytesAccumulate(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := RequestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("hello "))
		_, _ = w.Write([]byte("world!"))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	fields := obs.All()[0].ContextMap()
	if fields["bytes"] != int64(12) {
		t.Errorf("bytes: got %v, want 12", fields["bytes"])
	}
}

func TestRequestLogger_ElapsedIsNonNegative(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := RequestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	fields := obs.All()[0].ContextMap()
	elapsed, ok := fields["elapsed_ms"].(int64)
	if !ok {
		t.Fatalf("elapsed_ms not int64: %T", fields["elapsed_ms"])
	}
	if elapsed < 0 {
		t.Errorf("elapsed_ms is negative: %d", elapsed)
	}
	if elapsed < 1 {
		t.Errorf("elapsed_ms should be >= 1ms after 2ms sleep, got %d", elapsed)
	}
}

func TestRequestLogger_DoubleWriteHeaderRecordsFirst(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := RequestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		w.WriteHeader(http.StatusBadRequest)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	fields := obs.All()[0].ContextMap()
	if fields["status"] != int64(202) {
		t.Errorf("status: got %v, want 202 (first WriteHeader wins)", fields["status"])
	}
}
