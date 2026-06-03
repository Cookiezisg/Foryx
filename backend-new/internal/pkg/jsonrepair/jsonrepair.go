// Package jsonrepair provides a best-effort JSON repair that tolerates the most
// common malformed outputs from LLMs before strict json.Unmarshal parsing.
//
// LLMs (especially deepseek-v4-flash) produce ~4-8% malformed JSON in complex
// tool calls (complex nested ops push this to ~17%). Two dominant failure modes:
//  1. Literal control characters (newlines, tabs) inside JSON strings, unescaped.
//  2. Bracket/brace imbalance — trailing } or ] missing.
//
// This repair handles both. It is NOT a full JSON parser — it only fixes the
// patterns empirically observed in LLM output. If repair still fails to produce
// valid JSON, the original string is returned unchanged so the caller's
// json.Unmarshal surfaces the real error.
//
// Package jsonrepair 对 LLM 产出的畸形 JSON 做 best-effort 修复（不重新解析,
// 只处理两类常见模式），失败时返原串让调用方正常报错。
package jsonrepair

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Repair attempts to fix common LLM JSON issues and returns valid JSON.
// If the input is already valid or repair fails, returns the input unchanged.
//
// Repair 尝试修复常见 LLM JSON 缺陷；若已合法或修复仍失败，返回原串。
func Repair(s string) string {
	if s == "" {
		return s
	}
	// Fast path: already valid.
	if json.Valid([]byte(s)) {
		return s
	}

	fixed := s
	// Pass 1: escape unescaped literal control characters inside strings.
	fixed = escapeControlChars(fixed)
	if json.Valid([]byte(fixed)) {
		return fixed
	}

	// Pass 2: balance brackets and braces.
	fixed = balanceBrackets(fixed)
	if json.Valid([]byte(fixed)) {
		return fixed
	}

	// Pass 3: both passes together on original.
	combined := balanceBrackets(escapeControlChars(s))
	if json.Valid([]byte(combined)) {
		return combined
	}

	// Could not repair; return original so caller gets the real parse error.
	return s
}

// RepairBytes is the []byte variant of Repair.
func RepairBytes(b []byte) []byte {
	return []byte(Repair(string(b)))
}

// escapeControlChars walks the JSON string byte-by-byte and escapes any literal
// ASCII control characters (0x00-0x1F) that appear inside quoted strings but
// are not already part of a valid escape sequence. This is the single most
// common LLM failure mode: multi-line prompt strings with literal \n chars.
func escapeControlChars(s string) string {
	var buf bytes.Buffer
	buf.Grow(len(s) + 32)
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			buf.WriteByte(c)
			escaped = false
			continue
		}
		if c == '\\' && inString {
			buf.WriteByte(c)
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			buf.WriteByte(c)
			continue
		}
		if inString && c < 0x20 {
			// Unescaped control character inside string — escape it.
			switch c {
			case '\n':
				buf.WriteString(`\n`)
			case '\r':
				buf.WriteString(`\r`)
			case '\t':
				buf.WriteString(`\t`)
			default:
				buf.WriteString(`\u00`)
				const hexChars = "0123456789abcdef"
				buf.WriteByte(hexChars[c>>4])
				buf.WriteByte(hexChars[c&0xf])
			}
			continue
		}
		buf.WriteByte(c)
	}
	return buf.String()
}

// balanceBrackets appends any missing closing brackets/braces at the end of
// the JSON string. It counts open { [ that are not inside strings and appends
// the missing } ] in reverse order of opening.
func balanceBrackets(s string) string {
	s = strings.TrimRight(s, " \t\r\n")
	var stack []byte
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch c {
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) > 0 && stack[len(stack)-1] == c {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if len(stack) == 0 {
		return s
	}
	var buf bytes.Buffer
	buf.WriteString(s)
	for i := len(stack) - 1; i >= 0; i-- {
		buf.WriteByte(stack[i])
	}
	return buf.String()
}
