package middleware

import (
	"net/http"
	"strings"

	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// InjectLocale parses Accept-Language into ctx; unsupported falls back.
//
// InjectLocale 解析 Accept-Language 入 ctx,不支持则降级到默认。
func InjectLocale(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loc := parseAcceptLanguage(r.Header.Get("Accept-Language"))
		ctx := reqctxpkg.SetLocale(r.Context(), loc)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func parseAcceptLanguage(header string) reqctxpkg.Locale {
	header = strings.ToLower(strings.TrimSpace(header))
	if strings.HasPrefix(header, "en") {
		return reqctxpkg.LocaleEn
	}
	return reqctxpkg.LocaleZhCN
}
