package reqctx

import (
	"context"

	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
)

// AgentState ctx ferry — stamped by chat/runner.go::processTask once per
// agent task; lifetime spans the conversation queue.

type agentStateKey struct{}

// WithAgentState returns a copy of ctx carrying s.
//
// WithAgentState 返回携带 s 的 ctx 拷贝。
func WithAgentState(ctx context.Context, s *agentstatepkg.AgentState) context.Context {
	return context.WithValue(ctx, agentStateKey{}, s)
}

// GetAgentState returns the AgentState pointer; ok=false when absent or nil.
// Caller decides whether to fail or defensively skip.
//
// GetAgentState 返回 AgentState 指针；缺失或 nil 时 ok=false，调用方自决 fail 或跳过。
func GetAgentState(ctx context.Context) (*agentstatepkg.AgentState, bool) {
	s, ok := ctx.Value(agentStateKey{}).(*agentstatepkg.AgentState)
	return s, ok && s != nil
}
