package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func TestInjectLocale_ParsesAcceptLanguage(t *testing.T) {
	cases := []struct {
		header string
		want   reqctxpkg.Locale
	}{
		{"", reqctxpkg.LocaleZhCN},
		{"zh-CN", reqctxpkg.LocaleZhCN},
		{"zh-CN,en;q=0.9", reqctxpkg.LocaleZhCN},
		{"en", reqctxpkg.LocaleEn},
		{"en-US", reqctxpkg.LocaleEn},
		{"en-US,en;q=0.9,zh;q=0.8", reqctxpkg.LocaleEn},
		{"EN", reqctxpkg.LocaleEn},
		{"fr-FR", reqctxpkg.LocaleZhCN},
		{"de", reqctxpkg.LocaleZhCN},
	}

	for _, c := range cases {
		t.Run(c.header, func(t *testing.T) {
			var got reqctxpkg.Locale
			handler := InjectLocale(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = reqctxpkg.GetLocale(r.Context())
			}))

			req := httptest.NewRequest("GET", "/x", nil)
			if c.header != "" {
				req.Header.Set("Accept-Language", c.header)
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)

			if got != c.want {
				t.Errorf("Accept-Language %q: got %q, want %q", c.header, got, c.want)
			}
		})
	}
}

func TestInjectLocale_DoesNotAffectResponse(t *testing.T) {
	handler := InjectLocale(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
