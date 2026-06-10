// Package function provides the LLM system tools for the user's function library:
// search / get / create / edit / revert / delete / run + execution-log inspection.
// These are lazy tools (Toolset.Lazy) — surfaced via search_tools, not resident.
//
// Env-fix progress (the AI dep-repair loop) is captured by a forgeSink and folded into
// the create/edit tool result, so the LLM sees the full self-heal narrative. Live
// streaming of each attempt is a chat-host seam (M5.2); the sink is that seam.
//
// Package function 提供操作用户 function 库的 LLM system tool。这些是懒加载工具
// （Toolset.Lazy）——经 search_tools 浮现，非常驻。env-fix 进度（AI 改依赖循环）由 forgeSink
// 收集并折进 create/edit 结果，使 LLM 看到完整自愈叙事。逐尝试 live 推流是 chat-host 接缝
// （M5.2）；sink 即该缝。
package function

import (
	"context"
	"encoding/json"
	"fmt"

	envfixapp "github.com/sunweilin/forgify/backend/internal/app/envfix"
	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

// FunctionTools constructs the function system tools over the app service.
//
// FunctionTools 基于 app service 构造 function system tool。
func FunctionTools(svc *functionapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchFunction{svc: svc},
		&GetFunction{svc: svc},
		&CreateFunction{svc: svc},
		&EditFunction{svc: svc},
		&RevertFunction{svc: svc},
		&DeleteFunction{svc: svc},
		&RunFunction{svc: svc},
		&SearchFunctionExecutions{svc: svc},
		&GetFunctionExecution{svc: svc},
	}
}

// forgeSink accumulates env-fix attempts (folded into the create/edit result so the LLM sees the
// full self-heal narrative) AND streams each install/repair step live as a `progress` block under
// the tool_call, so the user watches the env get fixed in real time. nil-safe off a streamed turn.
//
// forgeSink 累积 env-fix 尝试（折进 create/edit 结果使 LLM 见完整自愈叙事），并把每步装环境/修复实时
// 流成 tool_call 下的 `progress` 块，使用户实时看 env 被修好。非流式 turn 下 nil 安全。
type forgeSink struct {
	attempts []envfixapp.Attempt
	prog     *loopapp.ToolProgressWriter
}

// newForgeSink wires the env-fix sink to the live progress stream (from ctx).
//
// newForgeSink 把 env-fix sink 接到实时进度流（取自 ctx）。
func newForgeSink(ctx context.Context) *forgeSink {
	return &forgeSink{prog: loopapp.ToolProgress(ctx)}
}

func (s *forgeSink) OnAttempt(a envfixapp.Attempt) {
	s.attempts = append(s.attempts, a)
	if a.OK {
		s.prog.Print(fmt.Sprintf("✓ env ready (attempt %d)\n", a.Number))
	} else {
		s.prog.Print(fmt.Sprintf("✗ attempt %d failed: %s\n", a.Number, a.Error))
	}
}

func (s *forgeSink) OnFixing(attempt int) {
	s.prog.Print(fmt.Sprintf("↻ install failed — revising deps with an LLM (attempt %d)…\n", attempt))
}

// Close ends the progress block (no-op if nothing streamed); the create/edit tools defer it.
//
// Close 结束进度块（未流过则 no-op）；create/edit 工具 defer 它。
func (s *forgeSink) Close() { s.prog.Close() }

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
