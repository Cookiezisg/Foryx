// Package subagent provides the Subagent system tool for spawning isolated sub-runners.
//
// Package subagent 提供 Subagent 系统工具，起隔离的 sub-runner。
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


var (
	// ErrEmptyPrompt: prompt missing or whitespace.
	//
	// ErrEmptyPrompt：prompt 缺失或全空白。
	ErrEmptyPrompt = errors.New("prompt is required and must be non-empty")

	// ErrEmptyType: subagent_type missing or empty.
	//
	// ErrEmptyType：subagent_type 缺失或为空。
	ErrEmptyType = errors.New("subagent_type is required and must be non-empty")
)


const subagentDescription = `Run a focused subtask in an isolated subagent — its own context window and a curated toolset, so your context stays clean. Use it for independent research/exploration, or to fan out several independent subtasks in parallel (e.g. forging several modules at once). Returns the subagent's final message. Set subagent_type in the schema (Explore / Plan / general-purpose).`

var subagentSchema = json.RawMessage(`{
	"type": "object",
	"required": ["subagent_type", "prompt"],
	"properties": {
		"subagent_type": {
			"type": "string",
			"enum": ["Explore", "Plan", "general-purpose"],
			"description": "Which subagent to spawn."
		},
		"prompt": {
			"type": "string",
			"description": "Self-contained task description for the subagent. The subagent does not see your conversation history."
		},
		"max_turns": {
			"type": "integer",
			"description": "Optional cap on the subagent's ReAct turns. Default per type (typically 25-30)."
		}
	}
}`)


// SubagentTool implements the Subagent system tool.
//
// SubagentTool 是 Subagent 系统工具的实现。
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

func (t *SubagentTool) Name() string                { return "Subagent" }
func (t *SubagentTool) Description() string         { return subagentDescription }
func (t *SubagentTool) Parameters() json.RawMessage { return subagentSchema }

func (t *SubagentTool) IsReadOnly() bool        { return false }
func (t *SubagentTool) NeedsReadFirst() bool    { return false }
func (t *SubagentTool) RequiresWorkspace() bool { return false }

// ValidateInput rejects empty subagent_type / prompt pre-Execute; type existence is checked in Service.Spawn.
//
// ValidateInput 在 Execute 前拒绝空 subagent_type / prompt；类型存在性由 Service.Spawn 检查。
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


// Execute checks recursion guard, calls Service.Spawn, and maps terminal status to a tool_result.
//
// Execute 查递归守卫 / 调 Service.Spawn / 按终态产出 tool_result。
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

	// Inherit effective ModelRef from ctx (set by chat.runner / parent Spawn).
	// Chain propagates without re-lookup at each level.
	//
	// 从 ctx 拿 effective ModelRef(由 chat.runner / 父 Spawn 写入);
	// 整条 spawn 链自动承袭,不在每层重查。
	parentOverride := reqctxpkg.GetModelOverride(ctx)
	res, err := t.svc.Spawn(ctx, args.SubagentType, args.Prompt, subagentapp.SpawnOpts{
		MaxTurns: args.MaxTurns,
	}, parentOverride)
	if err != nil {
		return "", err
	}

	switch res.Status {
	case subagentapp.StatusMaxTurns:
		return appendNote(res.Result, "subagent hit max turns"), nil
	case subagentapp.StatusCancelled:
		return appendNote(res.Result, "subagent was cancelled"), nil
	case subagentapp.StatusFailed:
		if strings.TrimSpace(res.Result) != "" {
			return appendNote(res.Result, fmt.Sprintf("subagent failed: %s", res.ErrorMsg)), nil
		}
		return fmt.Sprintf("Subagent %s failed: %s", res.Type, res.ErrorMsg), nil
	default:
		return res.Result, nil
	}
}

func appendNote(body, note string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Sprintf("[note: %s]", note)
	}
	return body + "\n\n[note: " + note + "]"
}


var _ toolapp.Tool = (*SubagentTool)(nil)
