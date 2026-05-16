// interceptor.go — the loop's tool-dispatch hook seam. chat.Service
// installs a ToolInterceptor (permissions gate + hook runner) on the
// agentCtx before loop.Run; runOneTool consumes it via ctx so subagent
// runs naturally inherit the parent's permission policy.
//
// interceptor.go ——loop 的 tool 派发 hook 缝。chat.Service 在调
// loop.Run 前把 ToolInterceptor（permissions gate + hook runner）装到
// agentCtx；runOneTool 经 ctx 消费，subagent 自然继承父规则。
package loop

import (
	"context"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
)

// ToolInterceptor is the per-tool-call hook surface. Implementations
// orchestrate permissions evaluation + PreToolUse/PostToolUse hook
// firing. Empty interface methods means "no-op" (default behavior).
//
// ToolInterceptor 是 per-tool-call hook 接口。实现 orchestrate 权限评估
// + PreToolUse/PostToolUse hook fire。空方法实现 = no-op（默认行为）。
type ToolInterceptor interface {
	// BeforeCall fires before Tool.Execute. Returning denied=true skips
	// Execute and emits a tool_result error with denyReason. Returning
	// (false, "") lets the call proceed.
	//
	// BeforeCall 在 Tool.Execute 前触发。denied=true 跳 Execute 并发
	// tool_result error 带 denyReason。(false, "") 放行。
	BeforeCall(ctx context.Context, tc chatdomain.ToolCallData) (denied bool, denyReason string)

	// AfterCall fires after Tool.Execute completes (success or fail).
	// The injected string (if non-empty) is appended to the next LLM
	// turn's tool_result so PostToolUse hooks can give the LLM feedback
	// ("ran fmt", "tests still failing").
	//
	// AfterCall 在 Tool.Execute 完成（成败均）后触发。返回非空字符串
	// 拼到下轮 tool_result，让 PostToolUse hook 给 LLM 反馈。
	AfterCall(ctx context.Context, tc chatdomain.ToolCallData, output, errMsg string, ok bool) (injectIntoNextTurn string)
}

type interceptorCtxKey struct{}

// WithInterceptor returns a ctx that carries i. Nil i = no-op (caller
// can pass nil unconditionally).
//
// WithInterceptor 返载 i 的 ctx。nil i = no-op（caller 可无条件传 nil）。
func WithInterceptor(ctx context.Context, i ToolInterceptor) context.Context {
	if i == nil {
		return ctx
	}
	return context.WithValue(ctx, interceptorCtxKey{}, i)
}

// interceptorFrom returns the ctx interceptor or a noop fallback.
//
// interceptorFrom 返 ctx 中的 interceptor 或 noop 兜底。
func interceptorFrom(ctx context.Context) ToolInterceptor {
	if v, ok := ctx.Value(interceptorCtxKey{}).(ToolInterceptor); ok && v != nil {
		return v
	}
	return noopInterceptor{}
}

type noopInterceptor struct{}

func (noopInterceptor) BeforeCall(context.Context, chatdomain.ToolCallData) (bool, string) {
	return false, ""
}
func (noopInterceptor) AfterCall(context.Context, chatdomain.ToolCallData, string, string, bool) string {
	return ""
}
