// Package model is the domain for model selection and the per-scenario picker contract. It owns no
// storage: defaults live as workspace columns, overrides as per-entity fields. This package only
// defines the shared ModelRef value, the scenario whitelist, and the override-then-default rule.
//
// Package model 是模型选择的 domain 与按 scenario 的 picker 契约。它不持有存储：默认选择是
// workspace 的列、override 是各实体的字段。本包只定义共享的 ModelRef 值、scenario 白名单与
// override 优先、否则默认的规则。
package model

import (
	"context"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// ModelRef is a reusable model selection: which key, which model, and the user's native option
// values (provider/model-native knob keys, e.g. {"reasoning_effort":"high"}). Provider is implicit
// via the api_key referenced by APIKeyID. Shared across workspace defaults and per-entity overrides
// (agent / conversation / node).
//
// ModelRef 是可复用的模型选择：用哪把 key、哪个 model、用户的原生选项值（provider/model 原生旋钮
// key，如 {"reasoning_effort":"high"}）。Provider 由 APIKeyID 引用的 api_key 隐含。被 workspace
// 默认与各实体 override（agent / conversation / node）共享。
type ModelRef struct {
	APIKeyID string            `json:"apiKeyId"`
	ModelID  string            `json:"modelId"`
	Options  map[string]string `json:"options,omitempty"`
}

// Validate reports whether a set selection carries both an api_key id and a model id. An empty ref
// means "unset" — callers test IsZero for that, Validate is for a ref that claims to be set.
//
// Validate 报告已设的选择是否同时带 api_key id 与 model id。空 ref 表示"未设"——caller 用 IsZero
// 判断，Validate 用于声称已设的 ref。
func (r ModelRef) Validate() error {
	if r.APIKeyID == "" || r.ModelID == "" {
		return ErrRefInvalid
	}
	return nil
}

// IsZero reports whether the ref is unset (no selection made).
//
// IsZero 报告 ref 是否未设（未做选择）。
func (r ModelRef) IsZero() bool {
	return r.APIKeyID == "" && r.ModelID == "" && len(r.Options) == 0
}

// Scenario is one of the fixed workspace-level default model slots.
//
// Scenario 是 workspace 级默认模型槽的固定集合之一。
const (
	ScenarioDialogue = "dialogue"
	ScenarioUtility  = "utility"
	ScenarioAgent    = "agent"
)

// IsValidScenario reports whether s is a recognised scenario.
//
// IsValidScenario 报告 s 是否为已知 scenario。
func IsValidScenario(s string) bool {
	switch s {
	case ScenarioDialogue, ScenarioUtility, ScenarioAgent:
		return true
	default:
		return false
	}
}

// ListScenarios returns every scenario in canonical order.
//
// ListScenarios 按规范顺序返回所有 scenario。
func ListScenarios() []string {
	return []string{ScenarioDialogue, ScenarioUtility, ScenarioAgent}
}

var (
	// ErrScenarioInvalid: unknown scenario name.
	// ErrScenarioInvalid：未知 scenario 名。
	ErrScenarioInvalid = errorsdomain.New(errorsdomain.KindInvalid, "MODEL_SCENARIO_INVALID", "unknown model scenario")

	// ErrNotConfigured: the workspace has no default model for this scenario — the caller surfaces
	// a "configure a model" prompt rather than failing opaquely.
	// ErrNotConfigured：该 workspace 在此 scenario 下无默认模型——caller 提示"去配置模型"而非晦涩报错。
	ErrNotConfigured = errorsdomain.New(errorsdomain.KindUnprocessable, "MODEL_NOT_CONFIGURED", "no model configured for scenario")

	// ErrRefInvalid: a model selection is missing apiKeyId or modelId.
	// ErrRefInvalid：模型选择缺 apiKeyId 或 modelId。
	ErrRefInvalid = errorsdomain.New(errorsdomain.KindInvalid, "MODEL_REF_INVALID", "model selection requires both apiKeyId and modelId")
)

// ModelPicker resolves a workspace's default ModelRef for a scenario. Implemented by app/workspace
// (defaults live in workspace columns); returns ErrNotConfigured when the scenario has no default.
//
// ModelPicker 解析 workspace 某 scenario 的默认 ModelRef。由 app/workspace 实现（默认在 workspace
// 列）；该 scenario 无默认时返 ErrNotConfigured。
type ModelPicker interface {
	Pick(ctx context.Context, scenario string) (ModelRef, error)
}

// Resolve applies the override-then-default rule: a set override wins; otherwise the picker's
// scenario default. This collapses the "check override, else default" branch that every LLM-using
// caller would otherwise repeat.
//
// Resolve 应用 override 优先、否则默认的规则：已设 override 胜出；否则取 picker 的 scenario 默认。
// 收口了每个用 LLM 的 caller 本要各自重复的「先看 override 再看默认」分支。
func Resolve(ctx context.Context, scenario string, override *ModelRef, picker ModelPicker) (ModelRef, error) {
	if override != nil && !override.IsZero() {
		return *override, override.Validate()
	}
	if !IsValidScenario(scenario) {
		return ModelRef{}, ErrScenarioInvalid
	}
	return picker.Pick(ctx, scenario)
}
