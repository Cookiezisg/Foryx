package webhook

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
)

func newTestServer(t *testing.T) (*Listener, *httptest.Server, *atomic.Int32) {
	t.Helper()
	var fired atomic.Int32
	mux := http.NewServeMux()
	l := New(mux, zaptest.NewLogger(t), func(string, string, map[string]any) {
		fired.Add(1)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return l, srv, &fired
}

func TestRegister_AndFire(t *testing.T) {
	l, srv, fired := newTestServer(t)

	if err := l.Register(triggerdomain.Spec{
		WorkflowID: "wf_abc",
		NodeID:     "trig1",
		Kind:       triggerdomain.KindWebhook,
		Config:     map[string]any{"path": "hello"},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	resp, err := http.Post(srv.URL+"/api/v1/webhooks/wf_abc/hello",
		"application/json", bytes.NewBufferString(`{"k":"v"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want 202", resp.StatusCode)
	}

	// Wait briefly for async onFire.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if fired.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if fired.Load() != 1 {
		t.Errorf("fired count = %d, want 1", fired.Load())
	}
}

func TestPathConflict(t *testing.T) {
	l, _, _ := newTestServer(t)

	if err := l.Register(triggerdomain.Spec{
		WorkflowID: "wf_abc", NodeID: "trig1",
		Config: map[string]any{"path": "hello"},
	}); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	err := l.Register(triggerdomain.Spec{
		WorkflowID: "wf_abc", NodeID: "trig2", // different node, same path
		Config: map[string]any{"path": "hello"},
	})
	if !errors.Is(err, triggerdomain.ErrPathConflict) {
		t.Errorf("expected ErrPathConflict, got %v", err)
	}
}

func TestEmptyPath_Conflict(t *testing.T) {
	l, _, _ := newTestServer(t)
	err := l.Register(triggerdomain.Spec{
		WorkflowID: "wf_abc", NodeID: "trig1",
		Config: map[string]any{},
	})
	if !errors.Is(err, triggerdomain.ErrPathConflict) {
		t.Errorf("expected ErrPathConflict on empty path, got %v", err)
	}
}

func TestSecret_HeaderCheck(t *testing.T) {
	l, srv, fired := newTestServer(t)

	if err := l.Register(triggerdomain.Spec{
		WorkflowID: "wf_abc", NodeID: "trig1",
		Config: map[string]any{"path": "secure", "secret": "hunter2"},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Missing secret → 401.
	resp, _ := http.Post(srv.URL+"/api/v1/webhooks/wf_abc/secure",
		"application/json", bytes.NewBufferString("{}"))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no-secret status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	// Wrong secret → 401.
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/webhooks/wf_abc/secure",
		bytes.NewBufferString("{}"))
	req.Header.Set("X-Webhook-Secret", "wrong")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong-secret status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	// Correct secret → 202.
	req, _ = http.NewRequest("POST", srv.URL+"/api/v1/webhooks/wf_abc/secure",
		bytes.NewBufferString("{}"))
	req.Header.Set("X-Webhook-Secret", "hunter2")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("correct-secret status = %d, want 202", resp.StatusCode)
	}
	resp.Body.Close()

	// Wait for async onFire.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if fired.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if fired.Load() != 1 {
		t.Errorf("fired count = %d, want 1 (only correct-secret call)", fired.Load())
	}
}

func TestSecret_QueryTokenAlternative(t *testing.T) {
	l, srv, fired := newTestServer(t)
	_ = l.Register(triggerdomain.Spec{
		WorkflowID: "wf_abc", NodeID: "trig1",
		Config: map[string]any{"path": "tok", "secret": "abc"},
	})
	resp, _ := http.Post(srv.URL+"/api/v1/webhooks/wf_abc/tok?token=abc",
		"application/json", bytes.NewBufferString("{}"))
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want 202", resp.StatusCode)
	}
	resp.Body.Close()
	time.Sleep(100 * time.Millisecond)
	if fired.Load() != 1 {
		t.Errorf("fired = %d, want 1", fired.Load())
	}
}

func TestUnregister_Stops404(t *testing.T) {
	l, srv, _ := newTestServer(t)
	_ = l.Register(triggerdomain.Spec{
		WorkflowID: "wf_abc", NodeID: "trig1",
		Config: map[string]any{"path": "x"},
	})
	l.Unregister("wf_abc", "trig1")

	resp, _ := http.Post(srv.URL+"/api/v1/webhooks/wf_abc/x",
		"application/json", bytes.NewBufferString("{}"))
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("post-Unregister status = %d, want 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestMethodMismatch_405(t *testing.T) {
	l, srv, _ := newTestServer(t)
	_ = l.Register(triggerdomain.Spec{
		WorkflowID: "wf_abc", NodeID: "trig1",
		Config: map[string]any{"path": "x", "method": "POST"},
	})
	resp, _ := http.Get(srv.URL + "/api/v1/webhooks/wf_abc/x")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET status = %d, want 405", resp.StatusCode)
	}
	resp.Body.Close()
}
