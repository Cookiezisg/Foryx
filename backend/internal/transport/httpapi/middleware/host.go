package middleware

import (
	"net"
	"net/http"

	errorspkg "github.com/sunweilin/anselm/backend/internal/pkg/errors"
	responsehttpapi "github.com/sunweilin/anselm/backend/internal/transport/httpapi/response"
)

// RequireLoopbackHost rejects any request whose Host header is not a loopback name — the canonical
// defense against DNS rebinding. Binding the listener to 127.0.0.1 alone is NOT enough: a browser
// (or any local agent) tricked into resolving an attacker-controlled domain to 127.0.0.1 will still
// connect to our port, but its Host header carries that domain. We accept only 127.0.0.1 / ::1 /
// localhost (any port) and 403 the rest. Always-on: it costs nothing under a loopback bind, and the
// dev `make server` + testend reach the server via exactly these names, so they pass.
//
// RequireLoopbackHost 拒绝 Host 头非 loopback 名的请求——防 DNS rebinding 的标准做法。仅绑 127.0.0.1
// 不够:被诱导把攻击者域名解析到 127.0.0.1 的浏览器/本机 agent 仍会连到我们的端口,但 Host 带的是那个
// 域名。只放行 127.0.0.1 / ::1 / localhost（任意端口),其余 403。常开（loopback 下零成本;dev/testend
// 经这些名访问、照常通过）。
func RequireLoopbackHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h // strip :port (and unwrap [::1])
		}
		switch host {
		case "127.0.0.1", "::1", "localhost":
			next.ServeHTTP(w, r)
		default:
			// S20: every transport error-write routes through FromDomainError with a sentinel.
			responsehttpapi.FromDomainError(w, nil, errorspkg.ErrForbiddenBadHost)
		}
	})
}
