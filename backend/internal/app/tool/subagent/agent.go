// Package subagent provides the Subagent system tool — the LLM-facing
// entry point that lets the parent LLM spawn an isolated sub-runner with
// its own context window and a curated tool list, and receive the
// sub-runner's last message as the tool_result.
//
// Imported as `subagenttool` per §S13 nested sub-package alias rule.
//
// Recursion defense is two-layered (subagent.md §8):
//
//  1. structural — Service.Spawn filters the tool list to drop "Subagent"
//     itself before calling loop.Run, so the sub-LLM physically cannot
//     see the tool name. This is the primary defense.
//  2. runtime — Execute checks reqctxpkg.GetSubagentDepth(ctx) ≥ 1 and
//     refuses with ErrRecursionAttempt. Belt-and-suspenders catch in
//     case a future bridge bug or test path leaks Subagent into a
//     sub-runner's tool list.
//
// Failure paths follow the §S18 / ask.go pattern: max-turns / cancelled
// terminations are converted to friendly tool_result strings so the
// parent LLM can read the situation and decide how to proceed; only
// hard sentinels (recursion / unknown type) escape as Go errors.
//
// Package subagent 提供 Subagent 系统工具——LLM 入口，父 LLM 起一个隔离
// sub-runner（独立 context window + 精选 tool 列表），sub-runner 的最后
// 一条 message 作为 tool_result 返。
//
// 双保险防递归：结构性（Service.Spawn 过滤 tool 列表）+ 运行时
// （Execute 查 ctx depth）。失败路径按 §S18 / ask.go：max-turns / cancelled
// 转友好 tool_result 字符串；hard sentinel（recursion / 未知类型）走 Go err。
package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ── Validation sentinels ─────────────────────────────────────────────

var (
	// ErrEmptyPrompt — `prompt` arg missing or whitespace.
	// ErrEmptyPrompt：prompt 缺失或全空白。
	ErrEmptyPrompt = errors.New("prompt is required and must be non-empty")

	// ErrEmptyType — `subagent_type` arg missing or empty.
	// ErrEmptyType：subagent_type 缺失或为空。
	ErrEmptyType = errors.New("subagent_type is required and must be non-empty")
)

// ── Description & schema ─────────────────────────────────────────────

const subagentDescription = `Spawn a specialized subagent to handle a focused subtask in isolation.
The subagent has its own context window and a curated tool list — your
own context is not consumed. Returns the subagent's final message as a string.

Use for:
- searching large codebases (subagent_type="Explore")
- planning multi-step work (subagent_type="Plan")
- any task where isolating context from your main conversation is valuable
  (subagent_type="general-purpose")

Be specific in ` + "`prompt`" + ` — the subagent does not see your conversation.`

var subagentSchema = json.RawMessage(`{
	"type": "object",
	"required": ["subagent_type", "prompt"],
	"properties": {
		"subagent_type": {
			"type": "string",
			"description": "Which subagent to spawn. Available: Explore, Plan, general-purpose."
		},
		"prompt": {
			"type": "string",
			"description": "Task description for the subagent. Be specific — the subagent does not see your conversation history."
		},
		"max_turns": {
			"type": "integer",
			"description": "Optional cap on the subagent's ReAct turns. Default per type (typically 25-30)."
		}
	}
}`)

// ── Tool struct & 9 methods ──────────────────────────────────────────

// SubagentTool implements the Subagent system tool.
//
// SubagentTool struct 是 Subagent 系统工具。
type SubagentTool struct {
	svc *subagentapp.Service
}

// SubagentTools constructs the subagent system tools sharing one Service.
//
// SubagentTools 用一个 Service 构造 subagent 系统工具。
func SubagentTools(svc *subagentapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&SubagentTool{svc: svc},
	}
}

// Identity --------------------------------------------------------------------

func (t *SubagentTool) Name() string                { return "Subagent" }
func (t *SubagentTool) Description() string         { return subagentDescription }
func (t *SubagentTool) Parameters() json.RawMessage { return subagentSchema }

// Static metadata -------------------------------------------------------------

// IsReadOnly is conservatively false because a sub-runner can invoke
// Write/Edit/Bash etc. (general-purpose inherits the full registry).
//
// IsReadOnly 保守取 false——sub-runner 可调 Write/Edit/Bash 等
// （general-purpose 继承全注册表）。
func (t *SubagentTool) IsReadOnly() bool        { return false }
func (t *SubagentTool) NeedsReadFirst() bool    { return false }
func (t *SubagentTool) RequiresWorkspace() bool { return false }

// ── Args-dependent hooks ─────────────────────────────────────────────

// ValidateInput rejects empty subagent_type / prompt pre-Execute. Type
// existence is checked inside Service.Spawn (returns ErrTypeNotFound).
//
// ValidateInput 在 Execute 前拒绝空 subagent_type / prompt。类型存在性
// 由 Service.Spawn 检查（返 ErrTypeNotFound）。
func (t *SubagentTool) ValidateInput(args json.RawMessage) error {
	var a struct {
		SubagentType string `json:"subagent_type"`
		Prompt       string `json:"prompt"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("SubagentTool.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.SubagentType) == "" {
		return ErrEmptyType
	}
	if strings.TrimSpace(a.Prompt) == "" {
		return ErrEmptyPrompt
	}
	return nil
}

func (t *SubagentTool) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// ── Execute ──────────────────────────────────────────────────────────

// Execute checks the runtime recursion guard, parses the args, calls
// Service.Spawn, and converts terminal status into the right tool_result
// shape:
//
//   - completed → return run.Result (the last assistant message text)
//   - max_turns → return run.Result + "\n\n[note: subagent hit max turns]"
//   - cancelled → return run.Result + "\n\n[note: subagent was cancelled]"
//   - failed    → return Go err (LLM sees "tool failed" instead of empty)
//   - recursion → return ErrRecursionAttempt as Go err so the chat layer
//                 surfaces "permission denied" tool_result text
//   - unknown type → return ErrTypeNotFound (LLM sees clear "type not found")
//
// Execute 检查运行时递归守卫，解析 args，调 Service.Spawn，按终态产出
// tool_result 形状：completed 直接返；max_turns/cancelled 加注脚；failed
// 走 Go err；recursion 走 Go err 让 chat 层显示 permission denied；未知
// 类型走 Go err 让 LLM 看到清晰提示。
func (t *SubagentTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	if depth := reqctxpkg.GetSubagentDepth(ctx); depth >= 1 {
		return "", fmt.Errorf("SubagentTool.Execute: %w (depth=%d)",
			subagentdomain.ErrRecursionAttempt, depth)
	}

	var args struct {
		SubagentType string `json:"subagent_type"`
		Prompt       string `json:"prompt"`
		MaxTurns     int    `json:"max_turns"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("SubagentTool.Execute: parse args: %w", err)
	}

	res, err := t.svc.Spawn(ctx, args.SubagentType, args.Prompt, subagentapp.SpawnOpts{
		MaxTurns: args.MaxTurns,
	})
	if err != nil {
		// Hard errors: type not found, persist failure, LLM resolve failure.
		// Spawn already wraps with %w so errmap can match the sentinel.
		// Hard error：未知类型 / 持久化失败 / LLM 解析失败。Spawn 已用 %w
		// 包好让 errmap 匹配 sentinel。
		return "", err
	}

	// Friendly status notes — LLM gets the result body plus a hint about
	// non-completed terminations, so it can decide whether to re-spawn,
	// pivot, or summarize what we have.
	//
	// 友好状态注脚——LLM 拿到结果正文 + 非 completed 终态提示，自行决定
	// 是否重起、转向或就此总结。
	switch res.Run.Status {
	case subagentdomain.StatusMaxTurns:
		return appendNote(res.Result, "subagent hit max turns; consider re-spawning with more turns or refining the prompt"), nil
	case subagentdomain.StatusCancelled:
		return appendNote(res.Result, "subagent was cancelled"), nil
	case subagentdomain.StatusFailed:
		// Failed runs may still have produced partial Result text; if so
		// surface it; if not, return a clear failure message.
		// 失败 run 可能仍产出部分文本；有则返；无则给清晰失败消息。
		if strings.TrimSpace(res.Result) != "" {
			return appendNote(res.Result, fmt.Sprintf("subagent failed: %s", res.Run.ErrorMsg)), nil
		}
		return fmt.Sprintf("Subagent %s failed: %s", res.Run.Type, res.Run.ErrorMsg), nil
	default:
		return res.Result, nil
	}
}

// appendNote tacks a "[note: …]" line onto the body, separated by a
// blank line so the LLM clearly distinguishes its own assistant text
// from our framework annotation.
//
// appendNote 在正文后加 "[note: …]" 行，空行分隔让 LLM 清楚区分自身
// assistant 文本与框架注释。
func appendNote(body, note string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Sprintf("[note: %s]", note)
	}
	return body + "\n\n[note: " + note + "]"
}

// ── Compile-time checks ──────────────────────────────────────────────

var _ toolapp.Tool = (*SubagentTool)(nil)
