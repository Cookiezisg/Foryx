package agent

import errorspkg "github.com/sunweilin/anselm/backend/internal/pkg/errors"

// Input-validation sentinels shared across this package's tools (ValidateInput presence /
// range checks). One sentinel per distinct physical violation — tools reuse them, never
// re-declare per-tool copies (S20; the duplicate-wire-code guard enforces uniqueness).
//
// 本包各工具 ValidateInput 共享的输入校验 sentinel（必填 / 范围检查）。每种物理违例一个
// sentinel——工具复用、不逐工具重复声明（S20；撞码守卫兜唯一性）。

var (
	ErrAgentIDRequired     = errorspkg.New(errorspkg.KindInvalid, "AGENT_ID_REQUIRED", "agentId is required")
	ErrAgentInputRequired  = errorspkg.New(errorspkg.KindInvalid, "AGENT_INPUT_REQUIRED", "input is required (an object; pass {} if the agent's prompt is self-contained — there is no 'prompt' field)")
	ErrExecutionIDRequired = errorspkg.New(errorspkg.KindInvalid, "AGENT_EXECUTION_ID_REQUIRED", "executionId is required")
	ErrNamePromptRequired  = errorspkg.New(errorspkg.KindInvalid, "AGENT_NAME_PROMPT_REQUIRED", "name and prompt are required")
	ErrRevertArgsRequired  = errorspkg.New(errorspkg.KindInvalid, "AGENT_REVERT_ARGS_REQUIRED", "agentId and a positive version are required")
	ErrAgentMetaNotInEdit  = errorspkg.New(errorspkg.KindInvalid, "AGENT_META_NOT_IN_EDIT", "name/description/tags are not editable via edit_agent (they are not part of the versioned config) — use update_agent_meta to change them")
)
