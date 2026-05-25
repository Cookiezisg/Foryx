// Package mention is the domain layer for @-mention references injected into chat messages.
//
// Package mention 是 @ 引用的 domain 层：被引用实体解析成 Reference 烤进消息。
package mention

import "context"

// MentionType is the closed set of @-mentionable entity kinds.
//
// MentionType 是可被 @ 的实体类型（封闭集）。
type MentionType string

const (
	MentionDocument MentionType = "document"
	MentionFunction MentionType = "function"
	MentionHandler  MentionType = "handler"
	MentionWorkflow MentionType = "workflow"
)

// MentionInput is the wire shape the frontend sends per mention: type + id only.
//
// MentionInput 是前端每个 mention 发来的形状：只 type + id。
type MentionInput struct {
	Type MentionType `json:"type"`
	ID   string      `json:"id"`
}

// Reference is the resolved snapshot stored on the message + rendered into the transcript.
// Content is the type-specific body (doc markdown / function code / handler methods / workflow graph).
//
// Reference 是已解析快照，存进消息 + 渲进 transcript；Content 是各类型自渲的内文。
type Reference struct {
	Type    MentionType `json:"type"`
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Content string      `json:"content"`
}

// Resolver is implemented by each capability app; chat holds a type→resolver registry.
//
// Resolver 由各 app 实现；chat 持 type→resolver 注册表。
type Resolver interface {
	Type() MentionType
	Resolve(ctx context.Context, id string) (*Reference, error)
}
