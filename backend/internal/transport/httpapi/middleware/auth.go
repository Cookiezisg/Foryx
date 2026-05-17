package middleware

import (
	"context"
	"net/http"

	userdomain "github.com/sunweilin/forgify/backend/internal/domain/user"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// HeaderUserID is the per-request profile selector; client (testend / Wails) sets it from localStorage.
//
// HeaderUserID 是 per-request profile 选择 header；前端从 localStorage 读后填入。
const HeaderUserID = "X-Forgify-User-ID"

// UserResolver is the minimal port InjectUserID needs from userapp.Service.
//
// UserResolver 是 InjectUserID 所需 userapp.Service 端口的最小子集。
type UserResolver interface {
	Get(ctx context.Context, id string) (*userdomain.User, error)
	List(ctx context.Context) ([]*userdomain.User, error)
}

// InjectUserIDWith reads X-Forgify-User-ID and validates it against the user repo.
// Fallback chain: header user → first user in DB → DefaultLocalUserID (last-resort, when DB empty).
// nil resolver keeps legacy single-user behaviour (used by middleware unit tests + early boot).
//
// InjectUserIDWith 读 X-Forgify-User-ID 并按 user repo 校验。
// 回退链: header → DB 首个 user → DefaultLocalUserID（DB 空兜底）。
// nil resolver 走 legacy 单用户路径（middleware 单测 + 早期 boot 用）。
func InjectUserIDWith(resolver UserResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			uid := resolveUserID(r, resolver)
			ctx := reqctxpkg.SetUserID(r.Context(), uid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// InjectUserID is the legacy single-user middleware kept for callers without a resolver.
//
// InjectUserID 是不带 resolver 的 legacy 单用户中间件，保旧调用方。
func InjectUserID(next http.Handler) http.Handler {
	return InjectUserIDWith(nil)(next)
}

func resolveUserID(r *http.Request, resolver UserResolver) string {
	if resolver == nil {
		return reqctxpkg.DefaultLocalUserID
	}
	// Header preferred; fallback to query (?userID=) for SSE EventSource which
	// can't set custom headers in the browser API.
	// header 优先；fallback 到 query (?userID=) —— SSE EventSource 浏览器 API 不能自定义 header。
	uid := r.Header.Get(HeaderUserID)
	if uid == "" {
		uid = r.URL.Query().Get("userID")
	}
	if uid != "" {
		if _, err := resolver.Get(r.Context(), uid); err == nil {
			return uid
		}
		// Unknown user in header → fall through.
		// header 里 user 未知 → 走兜底。
	}
	// No / invalid header: use first user.
	// header 缺 / 无效：用首个 user。
	if users, err := resolver.List(r.Context()); err == nil && len(users) > 0 {
		return users[0].ID
	}
	return reqctxpkg.DefaultLocalUserID
}
