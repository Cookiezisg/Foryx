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
)
