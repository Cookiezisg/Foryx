// interceptor.go — chat's loop.ToolInterceptor implementation: wires
// permissionsgate.Gate (rule eval + session ask cache) and hooksapp.Runner
// (PreToolUse / PostToolUse shell hooks) into runOneTool. Stored on
// agentCtx via loop.WithInterceptor before loop.Run; subagent runs
// automatically inherit because they get the parent ctx.
//
// interceptor.go ——chat 的 loop.ToolInterceptor 实现：把 permissionsgate.Gate
// + hooksapp.Runner 接到 runOneTool。runAgent 在 loop.Run 前装到
// agentCtx via loop.WithInterceptor；subagent 自动继承父 ctx。
package chat

import (
	"context"
	"encoding/json"
	"os"

	"go.uber.org/zap"

	hooksapp "github.com/sunweilin/forgify/backend/internal/app/hooks"
	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	permgate "github.com/sunweilin/forgify/backend/internal/app/tool/permissionsgate"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	permdomain "github.com/sunweilin/forgify/backend/internal/domain/permissions"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// toolInterceptor wires permissions gate + hooks into the loop.
//
// toolInterceptor 把 permissions gate + hooks 接进 loop。
type toolInterceptor struct {
	gate  *permgate.Gate
	hooks *hooksapp.Runner
	log   *zap.Logger
}

// newToolInterceptor wires the deps. Either / both may be nil → respective
// stages skipped (gate nil = no rule eval; hooks nil = no callbacks).
//
// newToolInterceptor 装配依赖。任一 / 全 nil → 对应阶段跳过。
func newToolInterceptor(gate *permgate.Gate, hooks *hooksapp.Runner, log *zap.Logger) *toolInterceptor {
	if log == nil {
		log = zap.NewNop()
	}
	return &toolInterceptor{gate: gate, hooks: hooks, log: log.Named("chat.interceptor")}
}

// BeforeCall: gate.Evaluate → (if Allow) PreToolUse hook. Returns
// denied=true on rule deny / hook deny; remembers ask approvals in
// session cache when the gate says Ask (V1.2 MVP: Ask is logged +
// auto-approved + session-cached — full AskUserQuestion ask flow is
// v2; the user's settings.deny is the primary control mechanism).
//
// BeforeCall：gate.Evaluate → 若 Allow 再跑 PreToolUse hook。规则 deny
// / hook deny 返 denied=true；gate 返 Ask 时（V1.2 MVP）log + 自动批准 +
// 缓存 session（完整 AskUserQuestion 流程 v2；用户主要靠 settings.deny
// 控制）。
func (it *toolInterceptor) BeforeCall(ctx context.Context, tc chatdomain.ToolCallData) (bool, string) {
	argsJSON, _ := json.Marshal(tc.Arguments)
	sessionID, _ := reqctxpkg.GetConversationID(ctx)

	// 1. Gate evaluation.
	// 1. Gate 评估。
	if it.gate != nil {
		dec := it.gate.Evaluate(sessionID, tc.Name, argsJSON, tc.Destructive)
		switch dec.Action {
		case permdomain.ActionDeny:
			return true, dec.Reason
		case permdomain.ActionAsk:
			// V1.2 MVP: auto-approve + remember. Loud log so users notice.
			// V1.2 MVP：自动批准 + 记住。响亮 log 让用户看见。
			it.log.Warn("tool needed user approval; auto-allowing for V1.2 MVP",
				zap.String("tool", tc.Name),
				zap.String("reason", dec.Reason),
				zap.Bool("destructive", tc.Destructive))
			it.gate.RememberApproval(sessionID, tc.Name, argsJSON)
		case permdomain.ActionAllow:
			// fall through
		}
	}

	// 2. PreToolUse shell hooks.
	// 2. PreToolUse shell hook。
	if it.hooks != nil {
		hookDec := it.hooks.FirePreToolUse(ctx, permdomain.HookInput{
			SessionID:      sessionID,
			ConversationID: sessionID,
			Cwd:            cwdSafe(),
			ToolName:       tc.Name,
			ToolInput:      argsJSON,
			ToolUseID:      tc.ID,
			DangerLevel:    permgate.LookupLevel(tc.Name, nil),
		})
		if hookDec.Action == permdomain.ActionDeny {
			return true, hookDec.Reason
		}
		if hookDec.Action == permdomain.ActionAsk {
			it.log.Warn("PreToolUse hook returned ask; auto-allowing for V1.2 MVP",
				zap.String("tool", tc.Name), zap.String("reason", hookDec.Reason))
			if it.gate != nil {
				it.gate.RememberApproval(sessionID, tc.Name, argsJSON)
			}
		}
	}
	return false, ""
}

// AfterCall fires PostToolUse hooks; injected text becomes a [hook]
// appendix on the tool_result content (runOneTool concatenates).
//
// AfterCall 跑 PostToolUse hook；注入文本作为 [hook] 附录拼到 tool_result
// content（runOneTool 串接）。
func (it *toolInterceptor) AfterCall(ctx context.Context, tc chatdomain.ToolCallData, output, errMsg string, ok bool) string {
	if it.hooks == nil {
		return ""
	}
	argsJSON, _ := json.Marshal(tc.Arguments)
	sessionID, _ := reqctxpkg.GetConversationID(ctx)
	status := "completed"
	if !ok {
		status = "error"
	}
	return it.hooks.FirePostToolUse(ctx, permdomain.HookInput{
		SessionID:      sessionID,
		ConversationID: sessionID,
		Cwd:            cwdSafe(),
		ToolName:       tc.Name,
		ToolInput:      argsJSON,
		ToolUseID:      tc.ID,
		DangerLevel:    permgate.LookupLevel(tc.Name, nil),
		ToolOutput:     output,
		ToolError:      errMsg,
		ToolStatus:     status,
	})
}

// cwdSafe returns os.Getwd or "" — hook input nice-to-have, never fatal.
//
// cwdSafe 返 os.Getwd 或 ""——hook 输入 nice-to-have，永不致命。
func cwdSafe() string {
	if d, err := os.Getwd(); err == nil {
		return d
	}
	return ""
}

// Compile-time check that *toolInterceptor implements loop.ToolInterceptor.
// 编译期检查。
var _ loopapp.ToolInterceptor = (*toolInterceptor)(nil)
