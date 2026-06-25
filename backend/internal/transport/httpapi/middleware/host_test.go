package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireLoopbackHost(t *testing.T) {
	cases := []struct {
		host string
		ok   bool
	}{
		{"127.0.0.1:8080", true},
		{"127.0.0.1", true},
		{"localhost:53121", true},
		{"localhost", true},
		{"[::1]:8080", true},
		{"evil.example.com", false}, // DNS-rebinding: resolves to 127.0.0.1 but Host carries the domain
		{"evil.example.com:8080", false},
		{"169.254.169.254", false}, // link-local metadata endpoint, not loopback
		{"0.0.0.0:8080", false},
	}
	for _, c := range cases {
		called := false
		h := RequireLoopbackHost(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
		r := httptest.NewRequest(http.MethodGet, "/api/v1/functions", nil)
		r.Host = c.host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if c.ok {
			if !called {
				t.Fatalf("host %q should pass", c.host)
			}
		} else {
			if called {
				t.Fatalf("host %q should be rejected (DNS-rebinding defense)", c.host)
			}
			if w.Code != http.StatusForbidden {
				t.Fatalf("host %q: want 403, got %d", c.host, w.Code)
			}
			if !strings.Contains(w.Body.String(), "FORBIDDEN_BAD_HOST") {
				t.Fatalf("host %q: want FORBIDDEN_BAD_HOST envelope, got %s", c.host, w.Body.String())
			}
		}
	}
}
