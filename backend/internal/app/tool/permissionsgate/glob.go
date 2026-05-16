// glob.go — rule pattern matching for V1.2 §3 final-sweep. Rules look
// like "Bash(rm -rf *)" or "Edit(./src/**)"; the leading verb is the
// tool name (regex/literal match) and the parens hold a doublestar
// glob against the formatted arguments string.
//
// glob.go ——V1.2 §3 规则模式匹配。规则形如 "Bash(rm -rf *)" 或
// "Edit(./src/**)"；动词部分是 tool 名（regex / 字面匹配），括号内是
// 对格式化 args 字符串做 doublestar glob。
package permissionsgate

import (
	"encoding/json"
	"regexp"
	"strings"
	"sync"
)

// MatchesRule reports whether the rule pattern matches a tool call.
// Rule shapes:
//
//	"Bash"                     — bare name, matches all Bash calls
//	"Bash(*)"                  — equivalent to bare
//	"Bash(rm *)"               — glob on formatted args (tool-specific
//	                             formatter; Bash = the command string)
//	"Edit(./src/**)"           — glob on the resolved file path (Edit /
//	                             Write / Read / Glob / Grep tools)
//	"WebFetch(domain:github.com)" — domain: prefix matches request host
//	"Bash(npm:*|yarn:*)"       — "|" inside the args pattern is OR
//
// Tool name part may itself be a "|"-separated regex (e.g. "Bash|Edit").
//
// MatchesRule 报告 rule 是否匹配一次 tool 调用。规则形态见上注释。
// Tool 名部分可以是 "|" 分隔的多匹配（如 "Bash|Edit"）。
func MatchesRule(rule string, toolName string, args json.RawMessage) bool {
	verb, argPattern, hasParen := splitRule(rule)
	if !matchToolName(verb, toolName) {
		return false
	}
	if !hasParen || argPattern == "" || argPattern == "*" {
		return true // bare name or "(*)" — any args match
	}
	formatted := formatArgsForMatch(toolName, args)
	return matchGlobAnyOf(argPattern, formatted)
}

// splitRule decomposes "Verb(pattern)" into ("Verb", "pattern", true)
// or "Verb" into ("Verb", "", false). Tolerates whitespace.
//
// splitRule 把 "Verb(pattern)" 拆成 ("Verb", "pattern", true) 或 "Verb"
// 拆成 ("Verb", "", false)。允许空白。
func splitRule(rule string) (verb, pattern string, hasParen bool) {
	r := strings.TrimSpace(rule)
	open := strings.IndexByte(r, '(')
	if open < 0 {
		return r, "", false
	}
	close := strings.LastIndexByte(r, ')')
	if close < open {
		return r, "", false // malformed; treat as bare
	}
	return strings.TrimSpace(r[:open]), strings.TrimSpace(r[open+1 : close]), true
}

// matchToolName handles "|"-separated alternatives in the verb position.
// Exact (case-sensitive) match; no regex metacharacters honored.
//
// matchToolName 处理 verb 位置的 "|" 分隔多选。精确匹配（区分大小写）；
// 不解释 regex 元字符。
func matchToolName(pattern, name string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	for _, alt := range strings.Split(pattern, "|") {
		if strings.TrimSpace(alt) == name {
			return true
		}
	}
	return false
}

// matchGlobAnyOf splits the pattern on "|" and returns true if any
// alternative matches. Each alternative is a shell-style glob translated
// to regex: `*` → `.*` (cross-separator OK — Bash commands like "rm -rf /"
// have `/` in the args), `?` → `.`, `**` → `.*` (equivalent in this
// non-path-aware mode), regex metacharacters escaped. Patterns cached.
//
// matchGlobAnyOf 按 "|" 拆 pattern 任一匹配返 true。每分支是 shell-style
// glob 翻译成 regex：`*` → `.*`（跨 separator——Bash 命令如 "rm -rf /"
// 含 `/`），`?` → `.`，`**` → `.*`（本非 path-aware 模式下等价），regex
// 元字符转义。pattern 缓存。
func matchGlobAnyOf(pattern, s string) bool {
	for _, alt := range strings.Split(pattern, "|") {
		alt = strings.TrimSpace(alt)
		if alt == "" {
			continue
		}
		re := compileGlob(alt)
		if re != nil && re.MatchString(s) {
			return true
		}
	}
	return false
}

// compileGlob translates a shell-style glob to a compiled regexp,
// memoised in globCache. Invalid patterns return nil (won't match).
//
// compileGlob 把 shell-style glob 翻译为编译后 regexp，memoise 在 globCache。
// 非法 pattern 返 nil（不匹配）。
func compileGlob(pattern string) *regexp.Regexp {
	globCacheMu.RLock()
	if re, ok := globCache[pattern]; ok {
		globCacheMu.RUnlock()
		return re
	}
	globCacheMu.RUnlock()
	re := buildGlobRegex(pattern)
	globCacheMu.Lock()
	globCache[pattern] = re
	globCacheMu.Unlock()
	return re
}

var (
	globCache   = map[string]*regexp.Regexp{}
	globCacheMu sync.RWMutex
)

func buildGlobRegex(pattern string) *regexp.Regexp {
	var sb strings.Builder
	sb.WriteByte('^')
	i := 0
	for i < len(pattern) {
		r := pattern[i]
		switch r {
		case '*':
			// Collapse "**" to a single ".*" — they mean the same thing
			// in this non-path-aware matcher.
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
			}
			sb.WriteString(".*")
		case '?':
			sb.WriteByte('.')
		case '.', '+', '(', ')', '[', ']', '{', '}', '^', '$', '\\':
			sb.WriteByte('\\')
			sb.WriteByte(r)
		default:
			sb.WriteByte(r)
		}
		i++
	}
	sb.WriteByte('$')
	re, err := regexp.Compile(sb.String())
	if err != nil {
		return nil
	}
	return re
}

// formatArgsForMatch normalises tool args into the string that rule
// globs are matched against. The shape is tool-aware:
//
//	Bash         → the command string
//	Edit / Write / Read / Glob / Grep / LS → the file/path argument
//	WebFetch     → "domain:host" plus "url:full-url" (caller can use either)
//	others       → "" (rule patterns only match the bare name)
//
// formatArgsForMatch 按 tool 把 args 归一化为 rule glob 匹配的字符串。
// Bash → command；Edit/Write 等 → 文件路径；WebFetch → domain:host；
// 其他 → "" （只能 bare 匹配）。
func formatArgsForMatch(toolName string, args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var raw map[string]any
	if err := json.Unmarshal(args, &raw); err != nil {
		return ""
	}
	switch toolName {
	case "Bash":
		if cmd, ok := raw["command"].(string); ok {
			return cmd
		}
	case "Edit", "Write", "Read":
		if p, ok := raw["file_path"].(string); ok {
			return p
		}
		if p, ok := raw["path"].(string); ok {
			return p
		}
	case "Glob", "Grep":
		if p, ok := raw["pattern"].(string); ok {
			return p
		}
		if p, ok := raw["path"].(string); ok {
			return p
		}
	case "WebFetch", "WebSearch":
		if u, ok := raw["url"].(string); ok {
			return "domain:" + extractHost(u)
		}
		if d, ok := raw["domain"].(string); ok {
			return "domain:" + d
		}
	}
	return ""
}

// extractHost pulls the hostname from a URL string with no allocation
// past what strings.* needs. Tolerates "https://example.com/x" /
// "example.com" / "//example.com".
//
// extractHost 从 URL 字符串抽 hostname。容忍 "https://..." / 裸 host / "//..."。
func extractHost(u string) string {
	u = strings.TrimSpace(u)
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	} else if strings.HasPrefix(u, "//") {
		u = u[2:]
	}
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	if i := strings.IndexByte(u, '@'); i >= 0 {
		u = u[i+1:]
	}
	if i := strings.IndexByte(u, ':'); i >= 0 {
		u = u[:i]
	}
	return strings.ToLower(u)
}
