// Package tool defines the Tool interface system tools implement and the framework machinery for LLM tool-call handling.
//
// Package tool 定义 system tool 必须实现的 Tool 接口及框架层 LLM 工具调用处理设施。
package tool

import (
	"context"
	"encoding/json"
	"fmt"

	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// PermissionMode is the agent's permission mode for a turn.
//
// PermissionMode 是 agent 当前回合的权限模式。
type PermissionMode string

const (
	PermissionModeDefault     PermissionMode = "default"
	PermissionModeAcceptEdits PermissionMode = "acceptEdits"
	PermissionModePlan        PermissionMode = "plan"
	PermissionModeBypass      PermissionMode = "bypass"
)

// PermissionResult is what CheckPermissions returns.
//
// PermissionResult 是 CheckPermissions 的返回值。
type PermissionResult int

const (
	PermissionAllow PermissionResult = iota
	PermissionDeny
	PermissionAsk
)

// Tool is the contract every system tool must implement. All 9 methods are required.
//
// Tool 是每个 system tool 必须实现的契约，9 方法全必填。
type Tool interface {
	Name() string
	Description() string
	Parameters() json.RawMessage

	IsReadOnly() bool
	NeedsReadFirst() bool
	RequiresWorkspace() bool

	ValidateInput(args json.RawMessage) error
	CheckPermissions(args json.RawMessage, mode PermissionMode) PermissionResult

	Execute(ctx context.Context, argsJSON string) (string, error)
}

// ToLLMDef converts a Tool to ToolDef, injecting summary / destructive / execution_group into Parameters.
//
// ToLLMDef 把 Tool 转成 ToolDef，自动注入 summary / destructive / execution_group 字段。
func ToLLMDef(t Tool) llminfra.ToolDef {
	return llminfra.ToolDef{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  injectStandardFields(t.Parameters()),
	}
}

// ToLLMDefs batch-converts Tools to ToolDefs.
//
// ToLLMDefs 批量转换 Tool 为 ToolDef。
func ToLLMDefs(tools []Tool) []llminfra.ToolDef {
	defs := make([]llminfra.ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = ToLLMDef(t)
	}
	return defs
}

// injectStandardFields adds summary / destructive / execution_group to the tool's schema; panics on conflict.
//
// injectStandardFields 注入 summary / destructive / execution_group 到 schema，冲突时 panic。
func injectStandardFields(params json.RawMessage) json.RawMessage {
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(params, &schema); err != nil {
		panic(fmt.Sprintf("tool: parameters are not a valid JSON object: %v", err))
	}

	var props map[string]json.RawMessage
	if raw, ok := schema["properties"]; ok {
		if err := json.Unmarshal(raw, &props); err != nil {
			panic(fmt.Sprintf("tool: parameters.properties is not a valid JSON object: %v", err))
		}
		if _, conflict := props["summary"]; conflict {
			panic("tool: parameters already contain 'summary' field; rename to avoid conflict")
		}
		if _, conflict := props["destructive"]; conflict {
			panic("tool: parameters already contain 'destructive' field; rename to avoid conflict")
		}
		if _, conflict := props["execution_group"]; conflict {
			panic("tool: parameters already contain 'execution_group' field; rename to avoid conflict")
		}
	} else {
		props = map[string]json.RawMessage{}
	}

	props["summary"] = json.RawMessage(`{
		"type": "string",
		"description": "One sentence describing what you are doing and why. Required."
	}`)
	props["destructive"] = json.RawMessage(`{
		"type": "boolean",
		"description": "Set to true if this call may cause irreversible damage (rm -rf, DELETE FROM, git push --force, deleting forges, running forges that modify external state, etc.). The user will see a warning when true. Default false.",
		"default": false
	}`)
	props["execution_group"] = json.RawMessage(`{
		"type": "integer",
		"minimum": 1,
		"description": "Optional execution batch identifier. Tool calls with the same execution_group run in parallel; different groups run sequentially in ascending order. Set the same number on calls that have NO interdependence and NO shared mutable state (typical example: parallel git status + git diff + git log). If omitted, this call gets a unique sequential group — runs alone, after any explicit groups. When unsure, omit the field (sequential is always safe)."
	}`)

	propsRaw, err := json.Marshal(props)
	if err != nil {
		return params
	}
	schema["properties"] = propsRaw

	// "summary" must lead required so LLMs output it first; panic on bad required preserves author intent.
	// "summary" 排首位引导 LLM 优先输出；坏的 required 直接 panic 避免静默丢失必填项。
	var required []string
	if raw, ok := schema["required"]; ok {
		if err := json.Unmarshal(raw, &required); err != nil {
			panic(fmt.Sprintf("tool: parameters.required is not a valid JSON array of strings: %v", err))
		}
	}
	required = append([]string{"summary"}, required...)
	reqRaw, err := json.Marshal(required)
	if err != nil {
		return params
	}
	schema["required"] = reqRaw

	result, err := json.Marshal(schema)
	if err != nil {
		return params
	}
	return result
}

// StandardFields is the parsed form of the three framework-injected fields.
//
// StandardFields 是 StripStandardFields 提取的三个框架注入字段的解析结果。
type StandardFields struct {
	Summary        string
	Destructive    bool
	ExecutionGroup int
}

// StripStandardFields extracts summary / destructive / execution_group from argsJSON and returns the stripped JSON.
//
// StripStandardFields 从 argsJSON 提取三个注入字段并返回剥除后的 JSON。
func StripStandardFields(argsJSON string) (StandardFields, string) {
	var fields StandardFields
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return fields, argsJSON
	}
	// Wrong type from LLM stays zero — ValidateInput surfaces it back to LLM via retry.
	// LLM 类型错误时字段保持零值；ValidateInput 经重试反馈回 LLM。
	if raw, ok := m["summary"]; ok {
		_ = json.Unmarshal(raw, &fields.Summary)
		delete(m, "summary")
	}
	if raw, ok := m["destructive"]; ok {
		_ = json.Unmarshal(raw, &fields.Destructive)
		delete(m, "destructive")
	}
	if raw, ok := m["execution_group"]; ok {
		_ = json.Unmarshal(raw, &fields.ExecutionGroup)
		if fields.ExecutionGroup < 0 {
			fields.ExecutionGroup = 0
		}
		delete(m, "execution_group")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return fields, argsJSON
	}
	return fields, string(b)
}
