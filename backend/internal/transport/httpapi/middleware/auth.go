package middleware

import (
	"net/http"

	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// InjectUserID stamps DefaultLocalUserID into ctx (simplified single-user auth).
//
// InjectUserID 给 ctx 塞 DefaultLocalUserID(简化单用户 auth)。
func InjectUserID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := reqctxpkg.SetUserID(r.Context(), reqctxpkg.DefaultLocalUserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
