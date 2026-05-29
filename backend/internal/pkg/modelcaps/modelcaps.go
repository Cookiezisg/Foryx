// Package modelcaps is the per-(provider,model) capability catalog: context
// window + thinking-knob shape, as durable family-pattern rules. Supersedes
// the window-only modelmeta registry.
//
// Package modelcaps 是 per-(provider,model) 能力目录:上下文窗口 + thinking
// 形状,按抗漂移的家族前缀规则组织。取代只有窗口的 modelmeta。
package modelcaps

import "strings"

// SafetyBuffer reserves tokens for tool defs / system overhead / provider wrapping.
//
// SafetyBuffer 为 tool defs / system overhead / provider wrapping 预留 token。
const SafetyBuffer = 2000

type ThinkingShape int

const (
	ShapeNone ThinkingShape = iota
	ShapeEffort
	ShapeBudget
	ShapeToggle
)

// Cap is the resolved capability of one model.
//
// Cap 是单个模型解析后的能力。
type Cap struct {
	ContextWindow int
	MaxOutput     int
	Thinking      ThinkingShape
	EffortValues  []string
	BudgetMin     int
	BudgetMax     int
	ContextMode   string // "" | "qwen_max_input" | "ollama_num_ctx"
}

// UsableInput returns the input-side budget after reserving output + safety overhead.
//
// UsableInput 返扣除 output + safety 后的输入侧预算。
func (c Cap) UsableInput() int {
	u := c.ContextWindow - c.MaxOutput - SafetyBuffer
	if u < 1000 {
		u = 1000
	}
	return u
}

// CapOverride is a user-supplied partial override; only non-nil fields apply.
//
// CapOverride 是用户提供的部分覆盖;只有非 nil 字段生效。
type CapOverride struct {
	Thinking      *ThinkingShape
	ContextWindow *int
	MaxOutput     *int
}

// Apply overlays a CapOverride onto base; nil overlay or nil fields leave base intact.
//
// Apply 把 CapOverride 叠加到 base;overlay 或字段为 nil 时保留 base。
func Apply(base Cap, o *CapOverride) Cap {
	if o == nil {
		return base
	}
	if o.Thinking != nil {
		base.Thinking = *o.Thinking
	}
	if o.ContextWindow != nil {
		base.ContextWindow = *o.ContextWindow
	}
	if o.MaxOutput != nil {
		base.MaxOutput = *o.MaxOutput
	}
	return base
}

type rule struct {
	provider string
	prefix   string
	cap      Cap
}

// rules: most-specific prefix first; Lookup returns the first match. Numbers
// from 04-capability-catalog.md §1; family patterns survive model churn.
//
// rules:最具体前缀在前;数值见 04-capability-catalog.md §1,家族前缀抗漂移。
var rules = []rule{
	{"anthropic", "claude-opus-4-7", Cap{1_000_000, 128_000, ShapeEffort, []string{"low", "medium", "high"}, 0, 0, ""}},
	{"anthropic", "claude-opus-4-8", Cap{1_000_000, 128_000, ShapeEffort, []string{"low", "medium", "high"}, 0, 0, ""}},
	{"anthropic", "claude-opus-4-6", Cap{1_000_000, 128_000, ShapeEffort, []string{"low", "medium", "high"}, 0, 0, ""}},
	{"anthropic", "claude-sonnet-4-6", Cap{1_000_000, 64_000, ShapeEffort, []string{"low", "medium", "high"}, 0, 0, ""}},
	{"anthropic", "claude-sonnet-4", Cap{200_000, 64_000, ShapeBudget, nil, 1024, 64_000, ""}},
	{"anthropic", "claude-haiku-4", Cap{200_000, 64_000, ShapeBudget, nil, 1024, 64_000, ""}},
	{"anthropic", "claude-opus-4", Cap{200_000, 64_000, ShapeBudget, nil, 1024, 64_000, ""}},
	{"anthropic", "claude", Cap{200_000, 32_000, ShapeBudget, nil, 1024, 32_000, ""}},
	{"openai", "gpt-5.5", Cap{1_000_000, 128_000, ShapeEffort, []string{"none", "low", "medium", "high", "xhigh"}, 0, 0, ""}},
	{"openai", "gpt-5.2", Cap{400_000, 128_000, ShapeEffort, []string{"none", "low", "medium", "high", "xhigh"}, 0, 0, ""}},
	{"openai", "gpt-5.1", Cap{400_000, 128_000, ShapeEffort, []string{"none", "low", "medium", "high"}, 0, 0, ""}},
	{"openai", "gpt-5", Cap{400_000, 128_000, ShapeEffort, []string{"minimal", "low", "medium", "high"}, 0, 0, ""}},
	{"openai", "o", Cap{200_000, 100_000, ShapeEffort, []string{"low", "medium", "high"}, 0, 0, ""}},
	{"openai", "gpt-4", Cap{128_000, 16_000, ShapeNone, nil, 0, 0, ""}},
	{"deepseek", "deepseek-v4", Cap{1_000_000, 384_000, ShapeEffort, []string{"high", "max"}, 0, 0, ""}},
	{"deepseek", "deepseek-reasoner", Cap{128_000, 64_000, ShapeEffort, []string{"high", "max"}, 0, 0, ""}},
	{"deepseek", "deepseek", Cap{128_000, 64_000, ShapeNone, nil, 0, 0, ""}},
	{"google", "gemini-2.5-pro", Cap{1_048_576, 65_536, ShapeBudget, nil, 128, 32_768, ""}},
	{"google", "gemini-2.5-flash-lite", Cap{1_048_576, 65_536, ShapeBudget, nil, 0, 24_576, ""}},
	{"google", "gemini-2.5-flash", Cap{1_048_576, 65_536, ShapeBudget, nil, 0, 24_576, ""}},
	{"google", "gemini-2.5", Cap{1_048_576, 65_536, ShapeBudget, nil, 0, 32_768, ""}},
	{"google", "gemini-3", Cap{1_000_000, 64_000, ShapeEffort, []string{"minimal", "low", "medium", "high"}, 0, 0, ""}},
	{"google", "gemini", Cap{1_000_000, 64_000, ShapeEffort, []string{"minimal", "low", "medium", "high"}, 0, 0, ""}},
	{"qwen", "qwen3-max", Cap{262_144, 32_768, ShapeBudget, nil, 0, 81_920, ""}},
	{"qwen", "qwen-long", Cap{10_000_000, 32_768, ShapeNone, nil, 0, 0, ""}},
	{"qwen", "qwen-turbo", Cap{1_000_000, 16_384, ShapeBudget, nil, 0, 38_912, "qwen_max_input"}},
	{"qwen", "qwen", Cap{1_000_000, 32_768, ShapeBudget, nil, 0, 81_920, "qwen_max_input"}},
	{"zhipu", "glm-4.6", Cap{200_000, 128_000, ShapeToggle, nil, 0, 0, ""}},
	{"zhipu", "glm-4.5", Cap{131_072, 96_000, ShapeToggle, nil, 0, 0, ""}},
	{"zhipu", "glm", Cap{200_000, 128_000, ShapeToggle, nil, 0, 0, ""}},
	{"moonshot", "kimi-k2-thinking", Cap{262_144, 32_768, ShapeToggle, nil, 0, 0, ""}},
	{"moonshot", "kimi-k2", Cap{262_144, 32_768, ShapeToggle, nil, 0, 0, ""}},
	{"moonshot", "moonshot-v1-128k", Cap{131_072, 32_768, ShapeNone, nil, 0, 0, ""}},
	{"moonshot", "moonshot-v1-32k", Cap{32_768, 32_768, ShapeNone, nil, 0, 0, ""}},
	{"moonshot", "moonshot-v1", Cap{8_192, 32_768, ShapeNone, nil, 0, 0, ""}},
	{"doubao", "doubao-seed-1-8", Cap{256_000, 64_000, ShapeEffort, []string{"no_think", "low", "medium", "high"}, 0, 0, ""}},
	{"doubao", "doubao-seed-2", Cap{256_000, 64_000, ShapeEffort, []string{"no_think", "low", "medium", "high"}, 0, 0, ""}},
	{"doubao", "doubao-seed-1-6", Cap{256_000, 16_000, ShapeBudget, nil, 0, 32_768, ""}},
	{"doubao", "doubao", Cap{256_000, 16_000, ShapeBudget, nil, 0, 32_768, ""}},
	{"openrouter", "", Cap{128_000, 32_000, ShapeEffort, []string{"none", "low", "medium", "high"}, 0, 0, ""}},
	{"ollama", "", Cap{4_096, 0, ShapeEffort, []string{"none", "low", "medium", "high"}, 0, 0, "ollama_num_ctx"}},
}

var fallback = Cap{ContextWindow: 32_768, MaxOutput: 8_192, Thinking: ShapeNone}

// Lookup returns the capability for (provider, modelID) by most-specific
// prefix (a "" prefix matches any model of that provider); fallback on miss.
//
// Lookup 按最具体前缀返回能力(prefix="" 匹配该 provider 任意模型);未知给兜底。
func Lookup(provider, modelID string) Cap {
	id := strings.ToLower(strings.TrimSpace(modelID))
	p := strings.ToLower(strings.TrimSpace(provider))
	for _, r := range rules {
		if r.provider != p {
			continue
		}
		if r.prefix == "" || strings.HasPrefix(id, r.prefix) {
			return r.cap
		}
	}
	return fallback
}
