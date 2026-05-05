// Package reqctx ferries identity and lifecycle metadata through ctx
// (user ID, locale, per-agent-run IDs). Each Set/With returns a ctx copy;
// keys are private empty structs to avoid string-key collisions.
//
// Package reqctx 通过 ctx 传递身份和生命周期元数据（user ID / locale /
// per-agent-run ID）。Set/With 返回 ctx 拷贝；私有 empty-struct key 避免冲突。
package reqctx

import (
	"context"
	"errors"
)

// ErrMissingUserID is returned by RequireUserID when ctx has no user ID
// (auth middleware didn't run). Treat as a wiring bug (500), not 401.
//
// ErrMissingUserID 由 RequireUserID 在 ctx 无 user ID 时返回（auth 中间件未跑）。
// 视为接线 bug（500），而非鉴权失败（401）。
var ErrMissingUserID = errors.New("reqctx: missing user id in context")

// DefaultLocalUserID is the hardcoded user ID for Phase 2 single-user mode.
//
// DefaultLocalUserID 是 Phase 2 单用户模式的硬编码 ID。
const DefaultLocalUserID = "local-user"

type userIDKey struct{}

// SetUserID returns a copy of ctx carrying id.
//
// SetUserID 返回携带 id 的 ctx 拷贝。
func SetUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

// GetUserID returns the user ID. ok=false when missing or empty —
// indicates a wiring bug (respond 500), not 401.
//
// GetUserID 取 user ID。缺失或空时 ok=false——属接线 bug，返 500 而非 401。
func GetUserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey{}).(string)
	return id, ok && id != ""
}

// RequireUserID is the (string, error) form of GetUserID for callers that
// want to bubble up ErrMissingUserID. All user-scoped store/app methods use this.
//
// RequireUserID 是 GetUserID 的 (string, error) 版本。按用户过滤的 store/app 方法都用它。
func RequireUserID(ctx context.Context) (string, error) {
	id, ok := GetUserID(ctx)
	if !ok {
		return "", ErrMissingUserID
	}
	return id, nil
}
