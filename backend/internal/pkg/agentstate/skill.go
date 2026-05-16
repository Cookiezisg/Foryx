package agentstate

import (
	"encoding/json"
	"strings"
	"sync/atomic"

	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
)

// SetActiveSkill records the skill as active for the current AgentState (last-write-wins).
//
// SetActiveSkill 把 skill 记为当前 AgentState 的 active（last-write-wins）。
func (s *AgentState) SetActiveSkill(skill *skilldomain.Skill) {
	s.activeSkill.Store(skill)
}

// ActiveSkill returns the currently-active skill or nil; treat returned pointer as read-only.
//
// ActiveSkill 返回当前 active skill 或 nil；返回指针视为只读。
func (s *AgentState) ActiveSkill() *skilldomain.Skill {
	return s.activeSkill.Load()
}

// ClearActiveSkillIfMatches clears the active skill only when its name matches (defer-cleanup safe).
//
// ClearActiveSkillIfMatches 仅当 name 匹配时清除 active skill（defer 清理安全）。
func (s *AgentState) ClearActiveSkillIfMatches(name string) {
	cur := s.activeSkill.Load()
	if cur != nil && cur.Name == name {
		s.activeSkill.CompareAndSwap(cur, nil)
	}
}

// IsToolPreApprovedBySkill reports whether active skill's allowed-tools grants toolName for given args.
//
// IsToolPreApprovedBySkill 报告 active skill 的 allowed-tools 是否在给定 args 下授权 toolName。
func (s *AgentState) IsToolPreApprovedBySkill(toolName string, argsJSON []byte) bool {
	skill := s.activeSkill.Load()
	if skill == nil {
		return false
	}
	for _, pattern := range skill.Frontmatter.AllowedTools {
		if matchAllowedTool(pattern, toolName, argsJSON) {
			return true
		}
	}
	return false
}

func matchAllowedTool(pattern, toolName string, argsJSON []byte) bool {
	open := strings.IndexByte(pattern, '(')
	if open < 0 {
		return pattern == toolName
	}
	close := strings.LastIndexByte(pattern, ')')
	if close <= open {
		return false
	}
	patternTool := pattern[:open]
	if patternTool != toolName {
		return false
	}
	spec := pattern[open+1 : close]

	primary := extractPrimaryArg(toolName, argsJSON)
	if primary == "" {
		return false
	}
	return wildcardMatch(spec, primary)
}

func extractPrimaryArg(toolName string, argsJSON []byte) string {
	switch toolName {
	case "Bash":
		var args struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return ""
		}
		return args.Command
	}
	return ""
}

// wildcardMatch tests pattern against subject with `*` glob semantics; anchored, no `?`/classes.
//
// wildcardMatch 用 `*` glob 语义测 pattern 对 subject；两端 anchor，无 `?`/字符类。
func wildcardMatch(pattern, subject string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == subject
	}
	if !strings.HasPrefix(subject, parts[0]) {
		return false
	}
	subject = subject[len(parts[0]):]
	last := parts[len(parts)-1]
	if !strings.HasSuffix(subject, last) {
		return false
	}
	subject = subject[:len(subject)-len(last)]
	for _, mid := range parts[1 : len(parts)-1] {
		idx := strings.Index(subject, mid)
		if idx < 0 {
			return false
		}
		subject = subject[idx+len(mid):]
	}
	return true
}

type activeSkillSlot = atomic.Pointer[skilldomain.Skill]
