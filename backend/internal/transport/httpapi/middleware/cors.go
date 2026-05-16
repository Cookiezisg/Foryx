package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// CORSConfig configures the CORS middleware; AllowedOrigins is a strict whitelist.
//
// CORSConfig 配置 CORS;AllowedOrigins 严格白名单(不支持 "*")。
type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	MaxAge         time.Duration
}

// DefaultCORSConfig returns Forgify's standard dev/prod origins and methods.
//
// DefaultCORSConfig 返回 Forgify 标准 dev/prod origin 和方法。
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{
			"http://localhost:5173",
			"http://localhost:3000",
			"http://127.0.0.1:5173",
			"http://127.0.0.1:3000",
		},
		AllowedMethods: []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-Requested-With"},
		MaxAge:         24 * time.Hour,
	}
}

// CORS handles browser cross-origin requests; preflight returns 204.
//
// CORS 处理浏览器跨域请求;preflight 返 204。
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		allowed[o] = struct{}{}
	}
	allowMethods := strings.Join(cfg.AllowedMethods, ", ")
	allowHeaders := strings.Join(cfg.AllowedHeaders, ", ")
	maxAge := strconv.Itoa(int(cfg.MaxAge.Seconds()))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := allowed[origin]; !ok {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Add("Vary", "Origin")

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.Header().Set("Access-Control-Allow-Methods", allowMethods)
				w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				w.Header().Set("Access-Control-Max-Age", maxAge)
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
