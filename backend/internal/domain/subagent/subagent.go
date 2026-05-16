// Package subagent is the domain layer for the Subagent system tool registry.
//
// Package subagent 是 Subagent system tool 的 domain 层（仅 type 注册表 + 防递归 sentinel）。
package subagent

import "errors"

// SubagentType is one registry entry; empty AllowedTools means inherit parent minus Subagent itself.
//
// SubagentType 是注册表一项；AllowedTools 为空表示继承父注册表（去掉 Subagent 自身防递归）。
type SubagentType struct {
	Name            string   `json:"name"`
	SystemPrompt    string   `json:"systemPrompt"`
	AllowedTools    []string `json:"allowedTools"`
	DefaultMaxTurns int      `json:"defaultMaxTurns"`
}

var ErrTypeNotFound = errors.New("subagent: type not found")

// ErrRecursionAttempt is returned when a sub-run tries to call Subagent again (depth check).
//
// ErrRecursionAttempt 在 sub-run 试图再调 Subagent 时返（防递归双保险）。
var ErrRecursionAttempt = errors.New("subagent: nested spawn not allowed")
