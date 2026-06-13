package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

type fakeResolver struct{ ok bool }

func (f fakeResolver) Resolve(_ context.Context, _ string) (reqctxpkg.Locale, error) {
	if f.ok {
		return reqctxpkg.LocaleEn, nil
	}
	return "", errors.New("unknown workspace")
}

func TestIdentifyWorkspaceFromHeader(t *testing.T) {
	var got string
	var present bool
	h := IdentifyWorkspace(fakeResolver{ok: true})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, present = reqctxpkg.GetWorkspaceID(r.Context())
	}))
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set(HeaderWorkspaceID, "ws1")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if !present || got != "ws1" {
		t.Errorf("workspace = %q present=%v, want ws1/true", got, present)
	}
}

func TestIdentifyWorkspaceInvalidDropped(t *testing.T) {
	var present bool
	h := IdentifyWorkspace(fakeResolver{ok: false})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, present = reqctxpkg.GetWorkspaceID(r.Context())
	}))
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set(HeaderWorkspaceID, "bad")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if present {
		t.Error("unknown workspace id should be dropped from ctx")
	}
}

func TestIdentifyWorkspaceFromQueryForSSE(t *testing.T) {
	var got string
	h := IdentifyWorkspace(nil)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = reqctxpkg.GetWorkspaceID(r.Context())
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/sse?workspaceID=ws2", nil))
	if got != "ws2" {
		t.Errorf("query workspace = %q, want ws2 (SSE can't set headers)", got)
	}
}

func TestRequireWorkspaceRejects(t *testing.T) {
	called := false
	h := RequireWorkspace(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/x", nil))
	if w.Code != http.StatusUnauthorized || called {
		t.Errorf("no workspace: status=%d called=%v, want 401 + not called", w.Code, called)
	}
	if !strings.Contains(w.Body.String(), "UNAUTH_NO_WORKSPACE") {
		t.Errorf("expected UNAUTH_NO_WORKSPACE code: %s", w.Body.String())
	}
}

func TestRequireWorkspacePasses(t *testing.T) {
	called := false
	h := RequireWorkspace(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	r := httptest.NewRequest("GET", "/x", nil).WithContext(reqctxpkg.SetWorkspaceID(context.Background(), "ws1"))
	h.ServeHTTP(httptest.NewRecorder(), r)
	if !called {
		t.Error("request with workspace should reach next handler")
	}
}
