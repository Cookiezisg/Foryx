// Package tool defines the Tool contract every system tool implements and the framework
// machinery for LLM tool-call handling: standard-field injection/stripping (summary /
// danger / execution_group) and the resident/lazy Toolset partition. It is a pure
// app-layer interface module — no domain, no store, no persistence — depended on by the
// loop, every tool adapter, and chat.
//
// Package tool 定义每个 system tool 实现的 Tool 契约，以及 LLM 工具调用处理的 framework 设施：
// 标准字段注入/剥离（summary / danger / execution_group）+ resident/lazy 的 Toolset 分组。
// 纯 app 层接口模块——无 domain、无 store、无持久化——被 loop、每个工具适配器、chat 依赖。
package tool

import (
	"context"
	"encoding/json"
)

// DangerLevel is the LLM's self-declared risk for one tool call (the injected `danger`
// field). Pure trust: tools set no floor — the LLM must declare honestly on every call,
// guided by the field description. The loop gates on it: dangerous blocks for user
// approval, cautious is surfaced but not blocked, safe runs silently.
//
// DangerLevel 是 LLM 对一次工具调用自报的危险度（注入的 `danger` 字段）。纯信任：工具不设下限
// ——LLM 须每次诚实自报、由字段描述引导。loop 据此设闸：dangerous 阻塞等用户同意、cautious 标记
// 不阻塞、safe 静默执行。
type DangerLevel string

const (
	// DangerSafe — read-only or trivially reversible; runs silently.
	// DangerSafe —— 只读或可逆；静默执行。
	DangerSafe DangerLevel = "safe"
	// DangerCautious — modifies recoverable state; runs but is surfaced prominently, no block.
	// DangerCautious —— 改可恢复状态；执行但前端显著标记、不阻塞。
	DangerCautious DangerLevel = "cautious"
	// DangerDangerous — irreversible or external write; blocks for user approval.
	// DangerDangerous —— 不可逆或外部写；阻塞等用户同意。
	DangerDangerous DangerLevel = "dangerous"
)

// IsValidDanger reports whether s is one of the three levels.
//
// IsValidDanger 报告 s 是否三级之一。
func IsValidDanger(s string) bool {
	switch DangerLevel(s) {
	case DangerSafe, DangerCautious, DangerDangerous:
		return true
	}
	return false
}

// Tool is the contract every system tool implements — exactly these five methods. The
// three standard fields (summary / danger / execution_group) are NOT methods: the
// framework injects them into the schema (ToLLMDef) and strips them from args
// (StripStandardFields), so a tool only ever declares and receives its own business
// parameters.
//
// Tool 是每个 system tool 实现的契约——恰这五个方法。三个标准字段（summary / danger /
// execution_group）**不是方法**：framework 注入进 schema（ToLLMDef）、从 args 剥离
// （StripStandardFields），故工具只声明、只收到自己的业务参数。
type Tool interface {
	// Name is the tool's call name (e.g. "Read", "run_function").
	//
	// Name 是工具调用名（如 "Read"、"run_function"）。
	Name() string

	// Description is the LLM-facing usage doc; may be large — the reason a Toolset can
	// load it lazily.
	//
	// Description 是给 LLM 的用法说明；可能很大——正是 Toolset 可懒加载它的理由。
	Description() string

	// Parameters is the JSON Schema of the tool's business args, WITHOUT the standard fields.
	//
	// Parameters 是工具业务参数的 JSON Schema，**不含**标准字段。
	Parameters() json.RawMessage

	// ValidateInput cheaply rejects bad business args pre-Execute; the error is surfaced
	// to the LLM for retry.
	//
	// ValidateInput 在 Execute 前廉价拒绝坏业务参数；错误反馈给 LLM 重试。
	ValidateInput(args json.RawMessage) error

	// Execute runs the tool against the stripped (business-only) args and returns the
	// result text shown back to the LLM.
	//
	// Execute 以剥离后（只含业务）的 args 运行工具，返回回显给 LLM 的结果文本。
	Execute(ctx context.Context, argsJSON string) (string, error)
}
