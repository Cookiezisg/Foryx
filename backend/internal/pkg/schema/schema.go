// Package schema is the single shared I/O contract for every build entity. Function,
// handler, agent, mcp, trigger, control and approval all describe what they consume and
// produce as a flat list of Fields — so a workflow node can uniformly read an entity's
// Inputs (what to feed) and Outputs (what to read downstream). It is deliberately minimal:
// a field is a name to reference, a coarse type for hints, and a description. Precise data
// shaping is CEL's job at runtime, not the schema's — so there are no nested fields, enums,
// required flags or JSON-Schema validators here.
//
// Package schema 是所有构建实体共享的唯一 I/O 契约。function/handler/agent/mcp/trigger/
// control/approval 都把"吃什么、吐什么"声明成一串 Field——使 workflow 节点统一地读实体的
// Inputs（喂什么）和 Outputs（下游读什么）。刻意极简：字段 = 引用名 + 粗类型提示 + 描述。
// 精确的数据塑形是运行时 CEL 的事，不是 schema 的事——故无嵌套、无 enum、无 required、无 JSON-Schema 校验。
package schema

import (
	"encoding/json"
	"fmt"
	"sort"
)

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
	for i := range fields {
		f := &fields[i]
		if f.Name == "" {
			return fmt.Errorf("field has empty name")
		}
		if seen[f.Name] {
			return fmt.Errorf("duplicate field name: %q", f.Name)
		}
		seen[f.Name] = true
		f.Type = canonicalType(f.Type) // normalize authoring aliases (integer→number, str→string, …) in place
		if !IsValidType(f.Type) {
			return fmt.Errorf("field %q has invalid type %q (use one of: string, number, boolean, object, array)", f.Name, f.Type)
		}
	}
	return nil
}

// canonicalType maps common authoring aliases to the coarse canonical types, so an agent's natural
// type vocabulary (integer/int/float, str, bool, dict, list) is accepted and normalized instead of
// bouncing with "invalid type integer" — runtime values are CEL-dynamic anyway. Unknown → unchanged.
//
// canonicalType 把常见编写别名归一到粗类型，使 agent 的自然类型词汇（integer/int/float、str、bool、dict、
// list）被接受并归一，而非以 "invalid type integer" 弹回——运行时值本就 CEL 动态。未知 → 原样返回。
func canonicalType(t string) string {
	switch t {
	case "integer", "int", "float", "double", "long":
		return TypeNumber
	case "str", "text":
		return TypeString
	case "bool":
		return TypeBoolean
	case "dict", "map":
		return TypeObject
	case "list":
		return TypeArray
	}
	return t
}

// FromJSONSchema converts a JSON Schema object's top-level properties into a flat []Field —
// used to expose an MCP tool's server-provided inputSchema as the uniform Field view for
// workflow wiring. Name comes from the property key, Type from its "type" (default "object"),
// Description from its "description". Nested/exotic schema features are intentionally dropped
// (workflow wiring only needs top-level field names + coarse types). Invalid/empty input → nil.
//
// FromJSONSchema 把一个 JSON Schema object 的顶层 properties 转成扁平 []Field——用于把 MCP 工具
// 服务端给的 inputSchema 暴露成统一 Field 视图供 workflow 接线。Name=属性键，Type=其 "type"
// （缺省 "object"），Description=其 "description"。嵌套/异类特性刻意丢弃（接线只需顶层名+粗类型）。
// 非法/空输入 → nil。
func FromJSONSchema(raw json.RawMessage) []Field {
	var doc struct {
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil || len(doc.Properties) == 0 {
		return nil
	}
	fields := make([]Field, 0, len(doc.Properties))
	for name, p := range doc.Properties {
		typ := canonicalType(p.Type)
		if !IsValidType(typ) {
			typ = TypeObject
		}
		fields = append(fields, Field{Name: name, Type: typ, Description: p.Description})
	}
	// Map iteration is unordered; sort by name so the result is stable across calls.
	//
	// map 遍历无序；按 name 排序使结果跨调用稳定。
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	return fields
}

// ToJSONSchema renders a flat []Field as a JSON Schema object — the inverse of FromJSONSchema.
// Every declared field is required (a Field list IS the call contract; there is no optional
// marker in the coarse model). Empty fields → a schema accepting any object.
//
// ToJSONSchema 把扁平 []Field 渲成 JSON Schema object——FromJSONSchema 的逆向。每个声明字段都
// required（Field 列表即调用契约；粗模型无可选标记）。空列表 → 接受任意对象的 schema。
func ToJSONSchema(fields []Field) json.RawMessage {
	type prop struct {
		Type        string `json:"type"`
		Description string `json:"description,omitempty"`
	}
	doc := struct {
		Type       string          `json:"type"`
		Properties map[string]prop `json:"properties"`
		Required   []string        `json:"required,omitempty"`
	}{Type: "object", Properties: map[string]prop{}}
	for _, f := range fields {
		doc.Properties[f.Name] = prop{Type: f.Type, Description: f.Description}
		doc.Required = append(doc.Required, f.Name)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return json.RawMessage(`{"type":"object"}`)
	}
	return b
}
