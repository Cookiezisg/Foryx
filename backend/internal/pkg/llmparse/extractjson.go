// Package llmparse provides shared helpers for pulling structured payloads
// out of free-form LLM text responses. Centralised here so every site (forge
// service GenerateTestCases, search_forges tool, future intent agents) uses
// the same fence-stripping and bracket-fallback rules.
//
// Package llmparse 提供从自由文本 LLM 响应中提取结构化载荷的共享 helper。
// 集中在此包让各处（forge service GenerateTestCases、search_forges 工具、
// 未来的意图 agent）使用同一套 fence 剥除和外层括号兜底规则。
package llmparse

import (
	"encoding/json"
	"strings"
)

// ExtractJSON pulls a JSON value out of an LLM response, handling several
// common shapes the LLM may use:
//
//  1. Plain JSON ("[...]" or "{...}") — returned as-is.
//  2. Markdown code fence with json language hint (```json ... ``` or ``` ... ```).
//  3. Surrounding prose ("Here's the answer: [...]") — fallback to outer
//     bracket matching, less reliable.
//
// Returns the JSON substring and true when a candidate parses cleanly;
// "", false when nothing matches. Markdown fences are tried first because
// they are unambiguous; bracket fallback is validated with json.Unmarshal so
// stray "[" / "]" inside prose don't produce a false positive.
//
// ExtractJSON 从 LLM 响应中提取 JSON 值，处理几种常见情况：
//
//  1. 纯 JSON（"[...]" 或 "{...}"）原样返回
//  2. Markdown 代码 fence（```json ... ``` 或 ``` ... ```）
//  3. 周围有散文（"Here's the answer: [...]"）—— 兜底用外层括号匹配
//
// 找到合法候选时返回 (substring, true)；都没匹配返 ("", false)。
// 优先试 markdown fence（无歧义）；外层括号兜底用 json.Unmarshal 验证，
// 避免散文里的方括号造成误匹配。
func ExtractJSON(s string) (string, bool) {
	s = strings.TrimSpace(s)

	// Markdown fences first (unambiguous).
	// 优先 markdown fence（无歧义）。
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

	// Fallback: outer-most bracket pair, validated.
	// 兜底：外层括号匹配，验证。
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

// IsLikelyJSON cheaply checks if s parses as valid JSON. Used by ExtractJSON
// to disambiguate between candidate substrings; exported because callers may
// want to validate post-extraction strings without forcing a specific schema.
//
// IsLikelyJSON 廉价检查 s 是否合法 JSON。供 ExtractJSON 在多个候选间挑选；
// 导出供调用方在不绑定具体 schema 时验证提取后的字符串。
func IsLikelyJSON(s string) bool {
	var v any
	return json.Unmarshal([]byte(s), &v) == nil
}
