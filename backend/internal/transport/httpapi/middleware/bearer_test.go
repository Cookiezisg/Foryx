package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func bearerNext() (http.Handler, *bool) {
	called := false
	h := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	return h, &called
}

func TestRequireBearerToken_emptyIsNoop(t *testing.T) {
	next, called := bearerNext()
	h := RequireBearerToken("")(next)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/functions", nil) // no Authorization
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !*called || w.Code != http.StatusOK {
		t.Fatalf("empty token must be a no-op (dev/testend); called=%v code=%d", *called, w.Code)
	}
}

func TestRequireBearerToken_correctPasses(t *testing.T) {
	next, called := bearerNext()
	h := RequireBearerToken("s3cret")(next)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/functions", nil)
	r.Header.Set("Authorization", "Bearer s3cret")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !*called {
		t.Fatal("correct bearer token must pass")
	}
}

func TestRequireBearerToken_missingOrWrongRejected(t *testing.T) {
	for _, auth := range []string{"", "Bearer wrong", "s3cret", "Basic s3cret", "Bearer s3cret "} {
		next, called := bearerNext()
		h := RequireBearerToken("s3cret")(next)
		r := httptest.NewRequest(http.MethodGet, "/api/v1/functions", nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if *called {
			t.Fatalf("auth %q must be rejected", auth)
		}
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("auth %q: want 401, got %d", auth, w.Code)
		}
		if !strings.Contains(w.Body.String(), "UNAUTH_BAD_TOKEN") {
			t.Fatalf("auth %q: want UNAUTH_BAD_TOKEN envelope, got %s", auth, w.Body.String())
		}
	}
}

func TestRequireBearerToken_healthNotExempt(t *testing.T) {
	next, called := bearerNext()
	h := RequireBearerToken("s3cret")(next)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil) // no token
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if *called || w.Code != http.StatusUnauthorized {
		t.Fatalf("health must require the token; called=%v code=%d", *called, w.Code)
	}
}

func TestRequireBearerToken_optionsExempt(t *testing.T) {
	next, called := bearerNext()
	h := RequireBearerToken("s3cret")(next)
	r := httptest.NewRequest(http.MethodOptions, "/api/v1/functions", nil) // CORS preflight, no auth
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !*called {
		t.Fatal("OPTIONS preflight must be exempt")
	}
}

func TestRequireBearerToken_webhooksExempt(t *testing.T) {
	next, called := bearerNext()
	h := RequireBearerToken("s3cret")(next)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/gh/abc123", nil) // external caller, HMAC self-auth
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !*called {
		t.Fatal("/api/v1/webhooks/ must be exempt (external callers can't know the token)")
	}
}
