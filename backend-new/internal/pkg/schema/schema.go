// Package schema is the single shared I/O contract for every forge entity. Function,
// handler, agent, mcp, trigger, control and approval all describe what they consume and
// produce as a flat list of Fields — so a workflow node can uniformly read an entity's
// Inputs (what to feed) and Outputs (what to read downstream). It is deliberately minimal:
// a field is a name to reference, a coarse type for hints, and a description. Precise data
// shaping is CEL's job at runtime, not the schema's — so there are no nested fields, enums,
// required flags or JSON-Schema validators here.
//
// Package schema 是所有锻造实体共享的唯一 I/O 契约。function/handler/agent/mcp/trigger/
// control/approval 都把"吃什么、吐什么"声明成一串 Field——使 workflow 节点统一地读实体的
// Inputs（喂什么）和 Outputs（下游读什么）。刻意极简：字段 = 引用名 + 粗类型提示 + 描述。
// 精确的数据塑形是运行时 CEL 的事，不是 schema 的事——故无嵌套、无 enum、无 required、无 JSON-Schema 校验。
package schema

import "fmt"

// Field types. A coarse hint for the UI / authoring agent; runtime values flow through CEL,
// which is dynamically typed, so this is never enforced as a hard contract.
//
// Field 类型。给 UI / 编排 agent 的粗提示；运行时值走 CEL（动态类型），故从不作硬约束强制。
const (
	TypeString  = "string"
	TypeNumber  = "number"
	TypeBoolean = "boolean"
	TypeObject  = "object" // 不透明对象；嵌套字段用 CEL 在运行时往里取（x.user.name）
	TypeArray   = "array"
)

// Field describes one input or output field: a name to reference it by, a coarse type, and
// a human/AI-readable description. The same shape serves both directions everywhere.
//
// Field 描述一个输入或输出字段：引用名 + 粗类型 + 人/AI 可读描述。同一形状双向、处处通用。
type Field struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// IsValidType reports whether t is one of the known field types.
//
// IsValidType 报告 t 是否已知字段类型之一。
func IsValidType(t string) bool {
	switch t {
	case TypeString, TypeNumber, TypeBoolean, TypeObject, TypeArray:
		return true
	}
	return false
}

// ValidateFields checks a field list is structurally sound: every name non-empty + unique
// within the list, every type one of the known kinds. The single field-list validator every
// entity's create/edit calls (callers wrap the error in their own domain error).
//
// ValidateFields 校验一个字段列表结构合法：每个 name 非空且组内唯一、每个 type 是已知类型之一。
// 所有实体 create/edit 共用的唯一字段列表校验器（调用方把错误包成自己的 domain 错误）。
func ValidateFields(fields []Field) error {
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		if f.Name == "" {
			return fmt.Errorf("field has empty name")
		}
		if seen[f.Name] {
			return fmt.Errorf("duplicate field name: %q", f.Name)
		}
		seen[f.Name] = true
		if !IsValidType(f.Type) {
			return fmt.Errorf("field %q has invalid type %q", f.Name, f.Type)
		}
	}
	return nil
}
