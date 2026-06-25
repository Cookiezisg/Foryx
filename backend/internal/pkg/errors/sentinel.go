package errors

// Cross-domain sentinels. Per-domain sentinels live in their own packages.
//
// 跨 domain sentinel；按 domain 的 sentinel 放各自包内。
var (
	// ErrInvalidRequest: malformed / semantically invalid request before domain logic.
	// ErrInvalidRequest：domain 逻辑前发现的格式错误或语义无效。
	ErrInvalidRequest = New(KindInvalid, "INVALID_REQUEST", "invalid request")

	// ErrUnauthorizedNoWorkspace: a workspace-scoped route was hit without a valid
	// workspace id. Frontend clears the active workspace and re-selects.
	//
	// ErrUnauthorizedNoWorkspace：按 workspace 隔离的路由未携带有效 workspace id；
	// 前端据此清除当前 workspace 并重新选择。
	ErrUnauthorizedNoWorkspace = New(KindUnauthorized, "UNAUTH_NO_WORKSPACE", "unauthorized: no valid workspace id")

	// ErrUnauthorizedBadToken: a request to the loopback server carried a missing/wrong
	// per-launch bearer token (ANSELM_AUTH_TOKEN; loopback hardening). The sidecar is
	// misconfigured, NOT a workspace problem — the desktop client shows a restart-backend
	// banner, never clears the workspace. Enforced only when the server has a token set.
	//
	// ErrUnauthorizedBadToken：到 loopback server 的请求缺/错每次启动的 bearer token
	// （ANSELM_AUTH_TOKEN；loopback 加固）。是 sidecar 配置问题、非 workspace 问题——客户端显示
	// 重启后端横幅、绝不清 workspace。仅当 server 设了 token 时强制。
	ErrUnauthorizedBadToken = New(KindUnauthorized, "UNAUTH_BAD_TOKEN", "unauthorized: invalid or missing bearer token")

	// ErrForbiddenBadHost: a request reached the loopback server with a non-loopback Host header
	// (DNS-rebinding defense; loopback hardening). 403 — the request is understood but refused.
	//
	// ErrForbiddenBadHost：到 loopback server 的请求带了非 loopback 的 Host 头（防 DNS rebinding；
	// loopback 加固）。403——请求可解但拒绝。
	ErrForbiddenBadHost = New(KindForbidden, "FORBIDDEN_BAD_HOST", "forbidden: request host is not loopback")

	// ErrNotFound: a route / sub-action / resource the router or a handler's :action
	// dispatcher can't resolve. Every unmatched dispatch + unknown :action lands here
	// (404) — transport never hand-codes a 404 (S20).
	//
	// ErrNotFound：路由 / 子动作 / 资源解析不出（router 兜底 + handler 的 :action 派发器未
	// 命中 + 未知 :action 全落此,404）——transport 不再手编 404（S20）。
	ErrNotFound = New(KindNotFound, "NOT_FOUND", "not found")

	// ErrInternal: an unexpected server fault (recovered panic). Original detail is logged,
	// never sent on the wire.
	//
	// ErrInternal：未预期的服务端故障（recover 的 panic）。原始细节记日志、绝不上线缆。
	ErrInternal = New(KindInternal, "INTERNAL_ERROR", "internal error")

	// ErrStreamingUnsupported: the ResponseWriter can't stream (no http.Flusher) — an SSE
	// endpoint hit a non-streaming transport.
	//
	// ErrStreamingUnsupported：ResponseWriter 不支持流式（无 http.Flusher）——SSE 端点遇非
	// 流式传输。
	ErrStreamingUnsupported = New(KindInternal, "STREAMING_UNSUPPORTED", "streaming not supported")
)
