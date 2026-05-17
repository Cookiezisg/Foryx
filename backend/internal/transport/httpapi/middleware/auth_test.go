package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	userdomain "github.com/sunweilin/forgify/backend/internal/domain/user"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// fakeResolver in-memory UserResolver for header-routing tests.
//
// fakeResolver 内存实现，仅用于 header-routing 测试。
type fakeResolver struct {
	users map[string]*userdomain.User
	order []string // List order
}

func (r *fakeResolver) Get(_ context.Context, id string) (*userdomain.User, error) {
	if u, ok := r.users[id]; ok {
		return u, nil
	}
	return nil, errors.New("not found")
}

func (r *fakeResolver) List(_ context.Context) ([]*userdomain.User, error) {
	out := make([]*userdomain.User, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.users[id])
	}
	return out, nil
}

func TestInjectUserID_StampsDefaultIntoContext(t *testing.T) {
	var gotID string
	var gotOK bool
	handler := InjectUserID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotID, gotOK = reqctxpkg.GetUserID(r.Context())
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))

	if !gotOK {
		t.Fatalf("userID not set by middleware")
	}
	if gotID != reqctxpkg.DefaultLocalUserID {
		t.Errorf("userID: got %q, want %q", gotID, reqctxpkg.DefaultLocalUserID)
	}
}

// Multi-user header routing tests.

func TestInjectUserIDWith_HeaderResolvesValidUser(t *testing.T) {
	resolver := &fakeResolver{
		users: map[string]*userdomain.User{
			"u_alice": {ID: "u_alice", Username: "alice"},
			"u_bob":   {ID: "u_bob", Username: "bob"},
		},
		order: []string{"u_alice", "u_bob"},
	}
	var gotID string
	handler := InjectUserIDWith(resolver)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotID, _ = reqctxpkg.GetUserID(r.Context())
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set(HeaderUserID, "u_bob")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if gotID != "u_bob" {
		t.Errorf("got %q, want u_bob", gotID)
	}
}

func TestInjectUserIDWith_UnknownHeaderFallsBackToFirst(t *testing.T) {
	resolver := &fakeResolver{
		users: map[string]*userdomain.User{
			"u_alice": {ID: "u_alice"},
		},
		order: []string{"u_alice"},
	}
	var gotID string
	handler := InjectUserIDWith(resolver)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotID, _ = reqctxpkg.GetUserID(r.Context())
	}))
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set(HeaderUserID, "u_nope")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if gotID != "u_alice" {
		t.Errorf("got %q, want u_alice (first user fallback)", gotID)
	}
}

func TestInjectUserIDWith_NoUsers_FallsBackToLocalDefault(t *testing.T) {
	resolver := &fakeResolver{users: map[string]*userdomain.User{}, order: nil}
	var gotID string
	handler := InjectUserIDWith(resolver)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotID, _ = reqctxpkg.GetUserID(r.Context())
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	if gotID != reqctxpkg.DefaultLocalUserID {
		t.Errorf("got %q, want %q", gotID, reqctxpkg.DefaultLocalUserID)
	}
}

func TestInjectUserID_DoesNotAffectResponse(t *testing.T) {
	handler := InjectUserID(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("brew"))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if rec.Code != http.StatusTeapot {
		t.Errorf("status: got %d, want 418", rec.Code)
	}
	if rec.Body.String() != "brew" {
		t.Errorf("body: got %q, want \"brew\"", rec.Body.String())
	}
}
