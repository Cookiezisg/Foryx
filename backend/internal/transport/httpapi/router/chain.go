package router

import (
	"net/http"
	"strings"

	"go.uber.org/zap"

	middlewarehttpapi "github.com/sunweilin/anselm/backend/internal/transport/httpapi/middleware"
	responsehttpapi "github.com/sunweilin/anselm/backend/internal/transport/httpapi/response"
)

// Chain wraps h with the standard middleware stack, outermost first:
//
//	Recover → RequestLogger → RequireLoopbackHost → RequireBearerToken → CORS → InjectLocale →
//	IdentifyWorkspace → RequireWorkspace(exempt)
//
// resolver may be nil (validation skipped) before the workspace module is wired. authToken is the
// per-launch loopback bearer secret (ANSELM_AUTH_TOKEN); "" disables bearer enforcement (dev /
// testend). The two loopback-hardening gates sit right after logging (so attacks are recorded) and
// before CORS/business: Host validation defeats DNS rebinding, the bearer token closes the
// localhost-server attack surface. bootstrap builds the mux, registers every handler, then Chains it.
//
// Chain 用标准中间件栈包裹 h（最外层在前）：Recover → RequestLogger → RequireLoopbackHost →
// RequireBearerToken → CORS → InjectLocale → IdentifyWorkspace → RequireWorkspace(豁免)。resolver
// 接线前可 nil；authToken 是每次启动的 loopback bearer 密钥（ANSELM_AUTH_TOKEN），"" 关闭 bearer 强制
// （dev/testend）。两道 loopback 加固门在日志之后（记录攻击）、CORS/业务之前:Host 校验防 DNS rebinding，
// bearer token 封住本地服务攻击面。
func Chain(h http.Handler, log *zap.Logger, resolver middlewarehttpapi.WorkspaceResolver, authToken string) http.Handler {
	h = envelopeMuxErrors(h) // innermost: rewrite the mux's plain-text 404/405 into the N1 envelope
	h = requireWorkspaceExempt(h)
	h = middlewarehttpapi.IdentifyWorkspace(resolver)(h)
	h = middlewarehttpapi.InjectLocale(h)
	h = middlewarehttpapi.CORS(middlewarehttpapi.DefaultCORSConfig())(h)
	h = middlewarehttpapi.RequireBearerToken(authToken)(h) // loopback hardening: per-launch token
	h = middlewarehttpapi.RequireLoopbackHost(h)           // loopback hardening: anti-DNS-rebinding
	h = middlewarehttpapi.RequestLogger(log)(h)
	h = middlewarehttpapi.Recover(log)(h)
	return h
}

// requireWorkspaceExempt applies RequireWorkspace to all /api/v1/* routes EXCEPT the ones
// that must work without a workspace header:
//   - /api/v1/workspaces — onboarding must create a workspace first
//   - /api/v1/health — liveness probe
//   - /api/v1/providers, /api/v1/scenarios — static metadata the onboarding UI reads
//   - /api/v1/webhooks/ — EXTERNAL callers (GitHub etc.) can never send the workspace
//     header; the webhook listener authenticates with its own secret/HMAC and the trigger
//     app resolves the workspace from the trigger's registration at report time
//
// Non-/api/v1/* paths pass through (mux handles NotFound / static assets).
//
// requireWorkspaceExempt 给所有 /api/v1/* 套 RequireWorkspace，但豁免无法带 workspace header 的：
// /workspaces（创建工作区）、/health（健康检查）、/providers + /scenarios（静态元数据）、
// /webhooks/（**外部**调用方如 GitHub 不可能带 header；webhook 监听器自带 secret/HMAC 鉴权，
// workspace 由 trigger app 在 report 时从注册表解析）。非 /api/v1/* 路径放过。
func requireWorkspaceExempt(next http.Handler) http.Handler {
	guarded := middlewarehttpapi.RequireWorkspace(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if !strings.HasPrefix(p, "/api/v1/") ||
			strings.HasPrefix(p, "/api/v1/workspaces") ||
			strings.HasPrefix(p, "/api/v1/webhooks/") ||
			p == "/api/v1/health" ||
			p == "/api/v1/providers" ||
			p == "/api/v1/scenarios" {
			next.ServeHTTP(w, r)
			return
		}
		guarded.ServeHTTP(w, r)
	})
}

// envelopeMuxErrors rewrites the stdlib ServeMux's plain text/plain 404 (no route matched) and 405
// (method not allowed) into the N1 error envelope for /api/v1/* paths. Without it those bypass the N1
// contract (failure MUST be {"error":{code,message,details}} JSON) — a client that always parses the
// error envelope hits a JSON parse error on every unknown route / wrong method (F172). It intercepts
// ONLY a 404/405 status on /api/v1/*; every other response (incl. large bodies) passes through with no
// buffering. The 405's Allow header set by the mux is preserved (response.Error does not touch it).
//
// envelopeMuxErrors 把标准库 ServeMux 的纯 text/plain 404（无路由匹配）/405（方法不允许）在 /api/v1/* 上改写成
// N1 错误 envelope。否则它们绕过 N1 契约（失败须 `{"error":{code,message,details}}` JSON）——总解析错误 envelope
// 的客户端会在每个未知路由/错误方法上 JSON 解析失败（F172）。**仅当 mux 未匹配任何路由时**才套 muxErrorWriter：
// 用 `mux.Handler(r)`（pattern=="" ⟺ 无匹配，404 与 405 皆然）判定。匹配到的 handler——它自己返的 404
// （FUNCTION_NOT_FOUND/WORKSPACE_NOT_FOUND…）或 SSE 流——拿到**裸 w 原样透传**。
//
// （原实现包裹**每个** /api/v1 响应：把所有 handler 的 404 clobber 成 ROUTE_NOT_FOUND，并因 muxErrorWriter
// 不转发 Flusher 而让三条 SSE 流全 500——本修复用"仅未匹配才包"消除两者。）mux 的 405 Allow header 保留。
func envelopeMuxErrors(next http.Handler) http.Handler {
	mux, isMux := next.(*http.ServeMux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isMux && strings.HasPrefix(r.URL.Path, "/api/v1/") {
			if _, pattern := mux.Handler(r); pattern == "" {
				// no route matched → only the mux's own plaintext 404/405 will be written; rewrite it.
				next.ServeHTTP(&muxErrorWriter{ResponseWriter: w}, r)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// muxErrorWriter intercepts a 404/405 WriteHeader and writes the N1 envelope instead, swallowing the
// stdlib's subsequent plain-text body. Any other status passes through untouched.
type muxErrorWriter struct {
	http.ResponseWriter
	intercepted bool
}

func (e *muxErrorWriter) WriteHeader(code int) {
	switch code {
	case http.StatusNotFound:
		e.intercepted = true
		responsehttpapi.Error(e.ResponseWriter, code, "ROUTE_NOT_FOUND", "no route matches this path", nil)
	case http.StatusMethodNotAllowed:
		e.intercepted = true
		responsehttpapi.Error(e.ResponseWriter, code, "METHOD_NOT_ALLOWED", "this method is not allowed for this path", nil)
	default:
		e.ResponseWriter.WriteHeader(code)
	}
}

func (e *muxErrorWriter) Write(b []byte) (int, error) {
	if e.intercepted {
		return len(b), nil // the stdlib's "404 page not found" / "Method Not Allowed" body — already replaced
	}
	return e.ResponseWriter.Write(b)
}

// Flush delegates to the underlying ResponseWriter so SSE streaming SURVIVES this wrapper. Without it,
// muxErrorWriter — which wraps EVERY /api/v1 response (envelopeMuxErrors) — would not satisfy
// http.Flusher, and StreamSSE's `w.(http.Flusher)` check would fail → all three SSE streams 500 with
// STREAMING_UNSUPPORTED. (Embedding http.ResponseWriter does NOT promote Flush, since that interface
// doesn't declare it.) 委托 Flush:本 wrapper 包了每个 /api/v1 响应,不转发则吃掉 Flusher → 三条 SSE 全 500。
func (e *muxErrorWriter) Flush() {
	if f, ok := e.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
