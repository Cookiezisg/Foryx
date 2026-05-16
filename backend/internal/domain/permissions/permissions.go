// Package permissions is the V1.2 §3 final-sweep domain layer for the
// settings-driven allow/ask/deny rules, danger levels, and hook
// configuration. No runtime evaluation here — that's app/tool/permissionsgate.
//
// Package permissions 是 V1.2 §3 final-sweep 的 domain 层：settings 驱动的
// allow/ask/deny 规则、危险等级、hook 配置。运行时评估在 app/tool/permissionsgate。
package permissions

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// DangerLevel classifies how risky a tool's execution is. Each tool
// gets a level assigned in app/tool/permissionsgate/levels.go — the
// framework hardcodes them rather than exposing a 10th method on the
// Tool interface (matches Claude Code's model: tool framework knows,
// tool itself doesn't).
//
// DangerLevel 标识 tool 执行的危险等级。每个 tool 在 levels.go 登记，
// 框架硬编码而非加 Tool 接口第 10 方法（抄 Claude Code 模式）。
type DangerLevel string

const (
	// LevelReadOnly: never prompts; pure read or sandboxed compute.
	LevelReadOnly DangerLevel = "read_only"

	// LevelWorkspaceWrite: first use of (tool, args) asks; remembered
	// for the rest of the session.
	LevelWorkspaceWrite DangerLevel = "workspace_write"

	// LevelDangerFullAccess: every use asks unless explicitly allowed
	// by settings.json.
	LevelDangerFullAccess DangerLevel = "danger_full_access"
)

// IsValidDangerLevel reports whether l is one of the 3 enumerated levels.
//
// IsValidDangerLevel 报告 l 是否 3 种枚举之一。
func IsValidDangerLevel(l DangerLevel) bool {
	switch l {
	case LevelReadOnly, LevelWorkspaceWrite, LevelDangerFullAccess:
		return true
	}
	return false
}

// Action is the outcome of evaluating a tool call against rules + hooks.
// Decided by the gate; consumed by chat/tools.go::runTools.
//
// Action 是 tool call 经规则 + hook 评估后的结果。gate 决定，
// chat/tools.go::runTools 消费。
type Action string

const (
	ActionAllow Action = "allow" // proceed without prompting
	ActionAsk   Action = "ask"   // surface AskUserQuestion; user decides
	ActionDeny  Action = "deny"  // refuse; return tool_result error
)

// Decision is what Gate.Evaluate returns. Reason is shown to the user
// (and to the LLM via tool_result when ActionDeny).
//
// Decision 是 Gate.Evaluate 的返回。Reason 给用户看（ActionDeny 时也经
// tool_result 给 LLM 看）。
type Decision struct {
	Action Action
	Reason string
}

// DefaultMode controls behaviour when no rule matches (allow / ask /
// deny mirror the 3 actions; bypass disables permission system but
// still honors protectedPaths).
//
// DefaultMode 控未匹配任何规则时的行为（allow/ask/deny 对应 3 actions；
// bypass 跳 permissions 但仍守 protectedPaths）。
type DefaultMode string

const (
	DefaultModeAsk    DefaultMode = "ask"     // default — prompt user
	DefaultModeAllow  DefaultMode = "allow"   // permissive
	DefaultModeDeny   DefaultMode = "deny"    // refuse everything not explicitly allowed
	DefaultModeBypass DefaultMode = "bypass"  // skip permissions entirely
)

// IsValidDefaultMode reports whether m is one of the 4 enumerated modes.
//
// IsValidDefaultMode 报告 m 是否 4 种枚举之一。
func IsValidDefaultMode(m DefaultMode) bool {
	switch m {
	case DefaultModeAsk, DefaultModeAllow, DefaultModeDeny, DefaultModeBypass:
		return true
	}
	return false
}

// Settings is the top-level shape of ~/.forgify/settings.json. Optional
// fields use zero values to mean "absent" (no rules → fall through to
// DefaultMode; no hooks → no callbacks).
//
// Settings 是 ~/.forgify/settings.json 的顶层形状。可选字段零值=缺省
// （无规则 → 走 DefaultMode；无 hook → 不回调）。
type Settings struct {
	Permissions    PermissionsBlock `json:"permissions"`
	Hooks          HooksBlock       `json:"hooks,omitempty"`
	ProtectedPaths ProtectedPaths   `json:"protectedPaths,omitempty"`
}

// PermissionsBlock holds the rule arrays + defaultMode.
//
// PermissionsBlock 持规则数组 + defaultMode。
type PermissionsBlock struct {
	DefaultMode DefaultMode `json:"defaultMode,omitempty"`
	Deny        []string    `json:"deny,omitempty"`
	Ask         []string    `json:"ask,omitempty"`
	Allow       []string    `json:"allow,omitempty"`
}

// HooksBlock maps each hook event name to an ordered list of hook
// configurations. Order matters: hooks run sequentially within an
// event, and a blocking exit (code 2) short-circuits the rest.
//
// HooksBlock 把每个 hook 事件名映射到有序 hook 配置列表。顺序重要：
// 同事件内顺序跑，blocking exit (code 2) 短路后续。
type HooksBlock struct {
	PreToolUse  []HookSpec `json:"PreToolUse,omitempty"`
	PostToolUse []HookSpec `json:"PostToolUse,omitempty"`
	Stop        []HookSpec `json:"Stop,omitempty"`
}

// HookSpec describes one hook. Matcher is a regex against tool name
// (Stop hooks ignore it). Command is the executable path. Args are
// the exec form args. Timeout is seconds (0 → 30s default). If is an
// optional glob filter against (tool_name, args) — only matching tool
// calls trigger the hook.
//
// HookSpec 描述一个 hook。Matcher 是 tool_name regex（Stop hook 忽略）。
// Command 可执行路径。Args exec form 参数。Timeout 秒（0 → 默认 30s）。
// If 可选 glob filter，仅匹配的 tool call 触发。
type HookSpec struct {
	Matcher string   `json:"matcher,omitempty"`
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Timeout int      `json:"timeout,omitempty"`
	If      string   `json:"if,omitempty"`
}

// ProtectedPaths holds user-extended write-protected globs. Defaults
// (.git/**, .env, .ssh/**, etc.) are merged in by pathguard at runtime
// — users can only ADD, not REMOVE defaults.
//
// ProtectedPaths 持用户扩展的写保护 glob。默认列表（.git/**, .env,
// .ssh/** 等）由 pathguard 运行时合并——用户只能增不能减。
type ProtectedPaths struct {
	DenyWrite []string `json:"denyWrite,omitempty"`
}

// HookInput is the JSON shipped to a hook on stdin. Common fields
// always present; PostToolUse adds tool_output / tool_error / tool_status
// / elapsed_ms.
//
// HookInput 是 stdin 给 hook 的 JSON。通用字段恒在；PostToolUse 加
// tool_output / tool_error / tool_status / elapsed_ms。
type HookInput struct {
	SessionID      string          `json:"session_id"`
	ConversationID string          `json:"conversation_id"`
	Cwd            string          `json:"cwd"`
	HookEventName  string          `json:"hook_event_name"`

	// Tool-event-only fields.
	ToolName    string          `json:"tool_name,omitempty"`
	ToolInput   json.RawMessage `json:"tool_input,omitempty"`
	ToolUseID   string          `json:"tool_use_id,omitempty"`
	DangerLevel DangerLevel     `json:"danger_level,omitempty"`

	// PostToolUse only.
	ToolOutput string `json:"tool_output,omitempty"`
	ToolError  string `json:"tool_error,omitempty"`
	ToolStatus string `json:"tool_status,omitempty"`
	ElapsedMs  int64  `json:"elapsed_ms,omitempty"`
}

// HookOutput is what a hook writes to stdout (on exit 0). All fields
// optional; unparseable stdout is treated as no-op.
//
// HookOutput 是 hook 在 exit 0 时写的 stdout。全部可选；解析失败当 no-op。
type HookOutput struct {
	Decision           Action `json:"decision,omitempty"`
	Reason             string `json:"reason,omitempty"`
	InjectIntoNextTurn string `json:"injectIntoNextTurn,omitempty"`
	SystemMessage      string `json:"systemMessage,omitempty"`
	SuppressOutput     bool   `json:"suppressOutput,omitempty"`
}

// Validate sanity-checks parsed Settings. Returns ErrInvalidSettings
// on schema violations. Empty Settings is valid (means "use defaults").
//
// Validate 对解析后的 Settings 做合理性检查。schema 违反返
// ErrInvalidSettings。空 Settings 合法（=用默认）。
func (s *Settings) Validate() error {
	if s.Permissions.DefaultMode != "" && !IsValidDefaultMode(s.Permissions.DefaultMode) {
		return fmt.Errorf("permissions.Validate: defaultMode %q: %w",
			s.Permissions.DefaultMode, ErrInvalidSettings)
	}
	for _, rs := range [][]string{s.Permissions.Deny, s.Permissions.Ask, s.Permissions.Allow} {
		for _, r := range rs {
			if strings.TrimSpace(r) == "" {
				return fmt.Errorf("permissions.Validate: empty rule string: %w", ErrInvalidSettings)
			}
		}
	}
	for _, evt := range [][]HookSpec{s.Hooks.PreToolUse, s.Hooks.PostToolUse, s.Hooks.Stop} {
		for i, h := range evt {
			if strings.TrimSpace(h.Command) == "" {
				return fmt.Errorf("permissions.Validate: hook[%d] command empty: %w", i, ErrInvalidSettings)
			}
			if h.Timeout < 0 {
				return fmt.Errorf("permissions.Validate: hook[%d] timeout negative: %w", i, ErrInvalidSettings)
			}
		}
	}
	return nil
}

// EffectiveDefaultMode returns DefaultModeAsk when not configured,
// keeping the safer default for fresh installs.
//
// EffectiveDefaultMode 未配时返 DefaultModeAsk，给全新安装更安全的默认。
func (s *Settings) EffectiveDefaultMode() DefaultMode {
	if s.Permissions.DefaultMode == "" {
		return DefaultModeAsk
	}
	return s.Permissions.DefaultMode
}

// Errors.

// ErrInvalidSettings: parsed settings.json has a schema violation.
// ErrInvalidSettings：解析后的 settings.json 违反 schema。
var ErrInvalidSettings = errors.New("permissions: invalid settings")

// ErrBlockedByRule: a tool call was denied by a permissions rule or by
// a PreToolUse hook returning decision=deny. Maps to 422 BLOCKED_BY_RULE.
//
// ErrBlockedByRule：tool call 被 permissions 规则或 PreToolUse hook
// decision=deny 拒绝。映射到 422 BLOCKED_BY_RULE。
var ErrBlockedByRule = errors.New("permissions: blocked by rule")
