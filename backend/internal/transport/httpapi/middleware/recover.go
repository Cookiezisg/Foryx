// Package middleware — HTTP middlewares wrapping per-handler business logic.
//
// Package middleware — 包裹 handler 的 HTTP 中间件集合。
package middleware

import (
	"net/http"
	"runtime/debug"

	"go.uber.org/zap"

	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// Recover catches panics into 500 INTERNAL_ERROR; must be OUTERMOST.
//
// Recover 把 panic 捕获为 500 INTERNAL_ERROR;必须是最外层中间件。
func Recover(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				log.Error("panic recovered",
					zap.Any("panic", rec),
					zap.String("stack", string(debug.Stack())),
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
				)
				responsehttpapi.Error(w, http.StatusInternalServerError,
					"INTERNAL_ERROR", "internal server error", nil)
			}()
			next.ServeHTTP(w, r)
		})
	}
}
