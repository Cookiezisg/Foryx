// logger_test.go — unit tests for the RequestLogger middleware.
//
// logger_test.go — RequestLogger 中间件的单元测试。
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
	// Handler only calls Write, never WriteHeader. Go implicitly sends 200.
	// The recorder must report 200 as well.
	//
	// Handler 只调 Write 不调 WriteHeader。Go 会隐式发送 200，recorder
	// 也应该记录 200。
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
	// Handler makes multiple Write calls; recorder must sum the bytes.
	//
	// Handler 多次 Write；recorder 应累加字节数。
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
	// elapsed_ms should be >= 0, never negative. We deliberately sleep a bit
	// to also make sure a positive elapsed is actually measured.
	//
	// elapsed_ms 应 >= 0，绝不为负。我们故意 sleep 一下，顺便验证正值
	// 能被测量到。
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
	// Calling WriteHeader twice is an app bug but shouldn't crash.
	// Recorder must report the FIRST status and ignore the second.
	//
	// 两次调 WriteHeader 是 bug 但不应 crash。recorder 应只记录**第一次**，
	// 忽略后续。
	log, obs := newObservedLogger(t)
	handler := RequestLogger(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)   // 202
		w.WriteHeader(http.StatusBadRequest) // 400 (ignored)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	fields := obs.All()[0].ContextMap()
	if fields["status"] != int64(202) {
		t.Errorf("status: got %v, want 202 (first WriteHeader wins)", fields["status"])
	}
}
