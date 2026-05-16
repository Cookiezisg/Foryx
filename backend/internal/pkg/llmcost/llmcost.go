// Package llmcost holds a tiny rate registry for LLM cost estimation
// (V1.2 §4.2). Rates are USD per 1M tokens, hand-curated from each
// provider's public pricing page. Unknown (provider, model) returns
// zero cost — caller surfaces it under the "(unknown)" bucket.
//
// The registry is intentionally small and unauthoritative — users
// should treat /api/v1/usage cost numbers as "rough ballpark", and we
// say so in the API response.
//
// Package llmcost 给 LLM cost 估算的小 rate registry（V1.2 §4.2）。
// 单价 USD per 1M tokens，从各 provider 公开价格页手动整理。未知
// (provider, model) 返 0 cost——调用方归到 "(unknown)" 桶。
//
// registry 故意小且非权威——/api/v1/usage 给的成本是"大致估算"，API
// 响应也明说。
package llmcost

import "strings"

// Rate is per-1M-tokens USD pricing for one model.
//
// Rate 是单 model 的 per-1M-tokens USD 单价。
type Rate struct {
	Provider     string
	ModelID      string
	InputPerMTok float64
	OutputPerMTok float64
}

// registry lists known prices. Match is case-insensitive (provider
// exact + modelID prefix). Update opportunistically; missing entries
// default to zero cost (no panic, no error).
//
// registry 列已知单价。匹配大小写不敏感（provider 精确 + modelID 前缀）。
// 按需更新；缺失条目默认 0（不 panic 不报错）。
var registry = []Rate{
	// DeepSeek (2026-05 prices; standard cache-miss rates)
	{"deepseek", "deepseek-chat", 0.27, 1.10},
	{"deepseek", "deepseek-reasoner", 0.55, 2.19},
	{"deepseek", "deepseek-coder", 0.27, 1.10},

	// Anthropic (2026 prices)
	{"anthropic", "claude-opus-4", 15.00, 75.00},
	{"anthropic", "claude-sonnet-4", 3.00, 15.00},
	{"anthropic", "claude-haiku-4", 0.80, 4.00},
	{"anthropic", "claude-3-5-sonnet", 3.00, 15.00},
	{"anthropic", "claude-3-5-haiku", 0.80, 4.00},
	{"anthropic", "claude-3-opus", 15.00, 75.00},

	// OpenAI (2026 prices)
	{"openai", "gpt-4o", 2.50, 10.00},
	{"openai", "gpt-4o-mini", 0.15, 0.60},
	{"openai", "gpt-4.1", 2.00, 8.00},
	{"openai", "gpt-4-turbo", 10.00, 30.00},
	{"openai", "o1", 15.00, 60.00},
	{"openai", "o3", 2.00, 8.00},

	// Ollama / generic local — zero cost, listed so they appear in usage
	// breakdown without "(unknown)" tag.
	// Ollama / generic local —— 0 cost，列出来让 usage 拆分时不带 "(unknown)"。
	{"ollama", "", 0.0, 0.0},
}

// Estimate returns USD cost for (provider, modelID) at the given input
// + output token counts. Unknown combos return 0 (caller can detect
// + display "$0.00 (rates unknown)").
//
// Estimate 返 (provider, modelID) 在给定 input + output token 下的 USD
// 成本。未知组合返 0（caller 可识别 + 显示 "$0.00 (rates unknown)"）。
func Estimate(provider, modelID string, inputTokens, outputTokens int) float64 {
	r, ok := Lookup(provider, modelID)
	if !ok {
		return 0
	}
	return (float64(inputTokens)/1_000_000.0)*r.InputPerMTok +
		(float64(outputTokens)/1_000_000.0)*r.OutputPerMTok
}

// Lookup finds the Rate for (provider, modelID). Match: provider exact
// (lowercase) + modelID empty match (ollama-style "any model under
// provider") OR exact OR prefix. Returns (Rate{}, false) when no match.
//
// Lookup 查 (provider, modelID) 的 Rate。匹配：provider 精确（小写）+
// modelID 空匹配（ollama "任意 model"）/ 精确 / 前缀。
func Lookup(provider, modelID string) (Rate, bool) {
	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.ToLower(strings.TrimSpace(modelID))
	if p == "" {
		return Rate{}, false
	}
	// Exact provider + model
	for _, r := range registry {
		if r.Provider == p && r.ModelID == m {
			return r, true
		}
	}
	// Prefix match on model (handles claude-opus-4 → claude-opus-4-7)
	for _, r := range registry {
		if r.Provider == p && r.ModelID != "" && strings.HasPrefix(m, r.ModelID) {
			return r, true
		}
	}
	// Empty-modelID catch-all (e.g. ollama)
	for _, r := range registry {
		if r.Provider == p && r.ModelID == "" {
			return r, true
		}
	}
	return Rate{}, false
}
