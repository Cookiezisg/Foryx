// Package llmparse extracts structured payloads from free-form LLM text.
// Shared by forge GenerateTestCases, search_forges, future intent agents.
//
// Package llmparse 从自由文本 LLM 响应中提取结构化载荷。
// 由 forge GenerateTestCases / search_forges / 未来意图 agent 共用。
package llmparse

import (
	"encoding/json"
	"strings"
)

// ExtractJSON pulls a JSON value out of LLM response s, handling plain JSON,
// markdown code fences (```json … ```), and prose-wrapped output (outer
// bracket fallback). Returns "", false when nothing parses. Fences are tried
// first (unambiguous); bracket fallback validates via json.Unmarshal to avoid
// stray-bracket false positives.
//
// ExtractJSON 从 LLM 响应 s 中提取 JSON：处理纯 JSON、markdown fence、
// 散文包裹（外层括号兜底）。无匹配返 "", false。优先 fence（无歧义），
// 外层括号兜底用 json.Unmarshal 验证防散文误匹配。
func ExtractJSON(s string) (string, bool) {
	s = strings.TrimSpace(s)

	for _, fence := range []string{"```json\n", "```json", "```\n", "```"} {
		if idx := strings.Index(s, fence); idx >= 0 {
			start := idx + len(fence)
			rest := s[start:]
			if end := strings.Index(rest, "```"); end >= 0 {
				candidate := strings.TrimSpace(rest[:end])
				if IsLikelyJSON(candidate) {
					return candidate, true
				}
			}
		}
	}

	for _, pair := range [][2]byte{{'[', ']'}, {'{', '}'}} {
		start := strings.IndexByte(s, pair[0])
		end := strings.LastIndexByte(s, pair[1])
		if start >= 0 && end > start {
			candidate := s[start : end+1]
			if IsLikelyJSON(candidate) {
				return candidate, true
			}
		}
	}
	return "", false
}

// IsLikelyJSON reports whether s parses as valid JSON. Exported for callers
// that want to validate without binding a specific schema.
//
// IsLikelyJSON 报告 s 是否合法 JSON。导出供不绑定 schema 的调用方验证。
func IsLikelyJSON(s string) bool {
	var v any
	return json.Unmarshal([]byte(s), &v) == nil
}
