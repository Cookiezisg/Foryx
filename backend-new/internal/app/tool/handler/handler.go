// Package handler provides the LLM system tools for the user's handler library:
// search / get / create / edit / revert / delete / call / update_config / restart +
// call-log inspection. These are lazy tools (Toolset.Lazy), surfaced via search_tools.
//
// restart_handler is the conversational "this handler is broken, restart it" path; the
// HTTP :restart endpoint is the editor-button path. Both reset the resident instance.
//
// Package handler 提供操作用户 handler 库的 LLM system tool。这些是懒加载工具，经 search_tools
// 浮现。restart_handler 是对话内"这个 handler 坏了，重启它"路径；HTTP :restart 是编辑器按钮路径。
package handler

import (
	"context"
	"encoding/json"
	"fmt"

	envfixapp "github.com/sunweilin/forgify/backend/internal/app/envfix"
	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

// HandlerTools constructs the handler system tools over the app service.
func HandlerTools(svc *handlerapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchHandler{svc: svc},
		&GetHandler{svc: svc},
		&CreateHandler{svc: svc},
		&EditHandler{svc: svc},
		&RevertHandler{svc: svc},
		&DeleteHandler{svc: svc},
		&CallHandler{svc: svc},
		&UpdateHandlerConfig{svc: svc},
		&RestartHandler{svc: svc},
		&SearchHandlerCalls{svc: svc},
		&GetHandlerCall{svc: svc},
	}
}

// forgeSink accumulates env-fix attempts (folded into the create/edit result for the LLM) AND
// streams each install/repair step live as a `progress` block under the tool_call, so the user
// watches the handler's env get fixed in real time. nil-safe off a streamed turn.
//
// forgeSink 累积 env-fix 尝试（折进 create/edit 结果给 LLM），并把每步装环境/修复实时流成 tool_call 下
// 的 `progress` 块，使用户实时看 handler 的 env 被修好。非流式 turn 下 nil 安全。
type forgeSink struct {
	attempts []envfixapp.Attempt
	prog     *loopapp.ToolProgressWriter
}

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

// Close ends the progress block (no-op if nothing streamed); create/edit defer it.
//
// Close 结束进度块（未流过则 no-op）；create/edit defer 它。
func (s *forgeSink) Close() { s.prog.Close() }

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
