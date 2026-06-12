// Package middleware holds the HTTP middlewares wrapping per-handler business logic.
//
// Package middleware 持有包裹 handler 业务逻辑的 HTTP 中间件集合。
package middleware

import (
	"context"
	"net/http"

	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// HeaderWorkspaceID is the per-request workspace selector; the client (Wails / browser)
// reads it from localStorage.activeWorkspaceId and sends it on every request.
//
// HeaderWorkspaceID 是 per-request workspace 选择 header；客户端从 localStorage 读后填入。
const HeaderWorkspaceID = "X-Forgify-Workspace-ID"

// WorkspaceResolver is the minimal port the auth middleware needs: validate a workspace id and
// return its UI locale (derived from the persisted language). A non-nil error means the id is
// unknown. Returning the locale lets the middleware make the workspace's language authoritative
// over Accept-Language (AC-PD-2): a user's explicit in-app language choice drives the assistant,
// not the browser header. Kept as a local interface so middleware imports no business domain; the
// workspace module implements it and is injected at wiring.
//
// WorkspaceResolver 是 auth 中间件所需的最小端口：校验 workspace id 并返回其 UI locale（由持久化
// language 派生）。非 nil 错 = 未知 id。返回 locale 使中间件让 workspace 语言压过 Accept-Language
// （AC-PD-2）：用户在 app 内的显式语言选择驱动 assistant，而非浏览器头。用本地接口使中间件不 import
// 业务 domain；workspace 模块实现并在装配时注入。
type WorkspaceResolver interface {
	Resolve(ctx context.Context, id string) (reqctxpkg.Locale, error)
}

// IdentifyWorkspace reads X-Forgify-Workspace-ID (or ?workspaceID= for SSE), validates it,
// and stamps ctx. Unknown / missing → ctx left empty; RequireWorkspace 401s if the route
// needs one. A nil resolver skips validation (pre-workspace-module wiring).
//
// IdentifyWorkspace 读 X-Forgify-Workspace-ID（SSE 用 ?workspaceID=），校验后写入 ctx；
// 不识别/缺失 → ctx 不带 workspace，由 RequireWorkspace 决定是否 401。resolver 为 nil 时跳过校验。
func IdentifyWorkspace(resolver WorkspaceResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get(HeaderWorkspaceID)
			if id == "" {
				id = r.URL.Query().Get("workspaceID")
			}
			ctx := r.Context()
			if id != "" && resolver != nil {
				loc, err := resolver.Resolve(ctx, id)
				if err != nil {
					id = "" // unknown id → treat as missing
				} else if loc.IsSupported() {
					// Workspace language is authoritative over Accept-Language (AC-PD-2): the user's
					// explicit, persisted choice wins; the header (set upstream by InjectLocale) is
					// only the pre-workspace (onboarding) fallback.
					// workspace 语言压过 Accept-Language（AC-PD-2）：用户显式持久化选择胜；头（上游
					// InjectLocale 设）仅作 pre-workspace（onboarding）兜底。
					ctx = reqctxpkg.SetLocale(ctx, loc)
				}
			}
			if id != "" {
				ctx = reqctxpkg.SetWorkspaceID(ctx, id)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireWorkspace rejects requests whose ctx has no workspace id (401 / UNAUTH_NO_WORKSPACE).
// Mount on every workspace-scoped route; exempt onboarding (/workspaces CRUD) and liveness.
//
// RequireWorkspace 拒绝 ctx 无 workspace 的请求（401 / UNAUTH_NO_WORKSPACE）；挂在所有按
// workspace 隔离的路由上，onboarding（/workspaces CRUD）与健康检查例外。
func RequireWorkspace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := reqctxpkg.GetWorkspaceID(r.Context()); !ok {
			responsehttpapi.Error(w, http.StatusUnauthorized, "UNAUTH_NO_WORKSPACE",
				"no valid workspace identifier; client should clear activeWorkspaceId and re-onboard", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// InjectWorkspaceID is a test-only middleware stamping a fixed workspace id for
// handler-level unit tests that don't need the full IdentifyWorkspace flow.
//
// InjectWorkspaceID 是 test-only 中间件，固定塞一个 workspace id，供不需要完整
// IdentifyWorkspace 流程的 handler 单测使用。
func InjectWorkspaceID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := reqctxpkg.SetWorkspaceID(r.Context(), "test-workspace")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
