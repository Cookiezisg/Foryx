// Package modelmeta holds a static registry of LLM model context-window meta for compaction thresholds.
//
// Package modelmeta 持静态 LLM 模型 context-window meta，供压缩阈值判断用。
package modelmeta

import "strings"

// SafetyBuffer reserves tokens for tool defs / system overhead / provider wrapping.
//
// SafetyBuffer 为 tool defs / system overhead / provider wrapping 预留 token。
const SafetyBuffer = 2000

// ModelMeta is one row in the registry (matching is lowercase + prefix-OK).
//
// ModelMeta 是 registry 的一行（小写匹配，允许前缀）。
type ModelMeta struct {
	Provider      string
	ModelID       string
	ContextWindow int
	MaxOutput     int
}

// UsableInput returns the input-side budget after reserving output + safety overhead.
//
// UsableInput 返扣除 output + safety 后的输入侧预算。
func (m ModelMeta) UsableInput() int {
	u := m.ContextWindow - m.MaxOutput - SafetyBuffer
	if u < 1000 {
		u = 1000
	}
	return u
}

var fallback = ModelMeta{
	Provider:      "unknown",
	ModelID:       "unknown",
	ContextWindow: 8000,
	MaxOutput:     2000,
}

// Lookup scans exact-match first, then prefix-match; put specific IDs before generic prefixes.
//
// Lookup 先精确匹配再前缀匹配；具体 ID 放在通用前缀之前。
var registry = []ModelMeta{
	{"deepseek", "deepseek-chat", 64000, 8192},
	{"deepseek", "deepseek-reasoner", 64000, 8192},
	{"deepseek", "deepseek-coder", 64000, 8192},

	{"anthropic", "claude-opus-4", 200000, 16384},
	{"anthropic", "claude-sonnet-4", 200000, 16384},
	{"anthropic", "claude-haiku-4", 200000, 16384},
	{"anthropic", "claude-3-5-sonnet", 200000, 8192},
	{"anthropic", "claude-3-5-haiku", 200000, 8192},
	{"anthropic", "claude-3-opus", 200000, 4096},

	{"openai", "gpt-4o", 128000, 16384},
	{"openai", "gpt-4o-mini", 128000, 16384},
	{"openai", "gpt-4.1", 1000000, 32768},
	{"openai", "gpt-4-turbo", 128000, 4096},
	{"openai", "o1", 200000, 32768},
	{"openai", "o3", 200000, 32768},

	{"ollama", "llama3", 8192, 2048},
	{"ollama", "qwen2", 32768, 4096},
	{"ollama", "mistral", 32768, 4096},
}

// Lookup finds a ModelMeta by (provider, modelID); case-insensitive, returns fallback on miss.
//
// Lookup 按 (provider, modelID) 查 ModelMeta；大小写不敏感，未命中返 fallback。
func Lookup(provider, modelID string) ModelMeta {
	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.ToLower(strings.TrimSpace(modelID))
	if p == "" || m == "" {
		return fallback
	}
	for _, r := range registry {
		if r.Provider == p && r.ModelID == m {
			return r
		}
	}
	for _, r := range registry {
		if r.Provider == p && strings.HasPrefix(m, r.ModelID) {
			return r
		}
	}
	return fallback
}
