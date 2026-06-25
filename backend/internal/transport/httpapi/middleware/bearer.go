package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	errorspkg "github.com/sunweilin/anselm/backend/internal/pkg/errors"
	responsehttpapi "github.com/sunweilin/anselm/backend/internal/transport/httpapi/response"
)

// RequireBearerToken enforces the per-launch loopback bearer token (ANSELM_AUTH_TOKEN) on every
// request — closing the localhost-server attack surface: even a malicious local process or a web
// page that reaches 127.0.0.1 cannot act without the random token the desktop parent minted and
// injected into the spawned server's env. The comparison is constant-time (crypto/subtle) so a
// wrong token leaks no timing signal.
//
// When expected is "" (dev `make server` / testend, which do not set ANSELM_AUTH_TOKEN) this is a
// NO-OP — dev-attach and the black-box harness keep working with zero auth. Exemptions when active:
//   - OPTIONS — CORS preflight carries no Authorization header
//   - /api/v1/webhooks/ — EXTERNAL callers (GitHub etc.) can't know the token; they self-auth via
//     their own HMAC secret (same rationale as the workspace-header exemption)
//
// /api/v1/health is NOT exempt: the health-gate is the same desktop process that minted the token,
// so it always has it; exempting health would leave one unauthenticated probe of the surface.
//
// RequireBearerToken 对每个请求强制每次启动的 loopback bearer token（ANSELM_AUTH_TOKEN），封住本地
// HTTP 服务的攻击面（本机恶意进程/网页即便够到 127.0.0.1，没有父 app 铸入子进程 env 的随机 token 也
// 无法动手）。常时比较（crypto/subtle）不漏时序。expected 为 ""（dev/testend 不设 token）时为 no-op，
// 保 dev-attach 与黑盒夹具零鉴权可用。激活时豁免:OPTIONS（CORS 预检无 Authorization）、/webhooks/
// （外部调用方不知 token、自带 HMAC）。/health 不豁免（门控它的就是铸 token 的同一进程）。
func RequireBearerToken(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if expected == "" {
			return next // no token configured (dev / testend) → no enforcement
		}
		want := []byte("Bearer " + expected)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions ||
				strings.HasPrefix(r.URL.Path, "/api/v1/webhooks/") {
				next.ServeHTTP(w, r)
				return
			}
			got := []byte(r.Header.Get("Authorization"))
			if subtle.ConstantTimeCompare(got, want) != 1 {
				responsehttpapi.FromDomainError(w, nil, errorspkg.ErrUnauthorizedBadToken)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
