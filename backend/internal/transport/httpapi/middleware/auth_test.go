package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

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
