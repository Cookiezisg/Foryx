// Package mention is the domain contract for @-mention references in chat: when a
// user @-mentions an entity, its content is snapshotted at send time and injected
// into that message's LLM context (freeze-on-send — the snapshot stays fixed even
// if the entity later changes). This package is pure contract: the MentionType set,
// the wire input, the resolved Reference, and the Resolver interface each entity
// app implements. Resolution and rendering live in the consumers (per-domain
// resolvers + chat), not here.
//
// Package mention 是 chat 中 @ 引用的 domain 契约：用户 @ 一个实体时，其内容在发送时刻快照、
// 注入该消息的 LLM 上下文（freeze-on-send——快照定格，实体日后改了也不变）。本包是纯契约：
// MentionType 集合、前端 input、解析后的 Reference、各实体 app 实现的 Resolver 接口。解析与
// 渲染在消费方（各域 resolver + chat），不在此。
package mention

import "context"

// MentionType is the closed set of @-mentionable entity kinds: the Quadrinity plus
// document — entities the user forged that carry an injectable content snapshot.
// conversation/skill/mcp are NOT mentionable (no single content snapshot to inject).
//
// MentionType 是可被 @ 的实体类型封闭集：四件套 + document——用户锻造的、有可注入内容快照
// 的实体。conversation/skill/mcp 不可 @（无单一内容快照可注入）。
type MentionType string

const (
	MentionDocument MentionType = "document"
	MentionFunction MentionType = "function"
	MentionHandler  MentionType = "handler"
	MentionWorkflow MentionType = "workflow"
	MentionAgent    MentionType = "agent"
	// trigger / control / approval are forge entities too — mentionable so the AI :iterate verb
	// (R0065) can seed them by reference, exactly like the five above.
	//
	// trigger / control / approval 也是 forge 实体——可 @，使 AI :iterate（R0065）能像上面五个一样按引用种入它们。
	MentionTrigger  MentionType = "trigger"
	MentionControl  MentionType = "control"
	MentionApproval MentionType = "approval"
)

// IsValidMentionType reports whether t is one of the mentionable forge kinds. Consumers
// (chat) validate incoming MentionInput against it.
//
// IsValidMentionType 报告 t 是否可 @ 的 forge 类型之一。消费方（chat）据此校验 MentionInput。
func IsValidMentionType(t MentionType) bool {
	switch t {
	case MentionDocument, MentionFunction, MentionHandler, MentionWorkflow, MentionAgent,
		MentionTrigger, MentionControl, MentionApproval:
		return true
	}
	return false
}

// MentionInput is the per-mention wire shape the frontend sends: type + id only.
//
// MentionInput 是前端每个 mention 发来的形状：只 type + id。
type MentionInput struct {
	Type MentionType `json:"type"`
	ID   string      `json:"id"`
}

// Reference is the resolved snapshot stored on the message and rendered into the
// transcript. Content is the type-specific body (doc markdown / function code /
// handler methods / workflow graph / agent config), captured at send time.
//
// Reference 是已解析快照，存进消息并渲进 transcript。Content 是各类型自渲内文（doc markdown
// / function 代码 / handler 方法 / workflow 图 / agent 配置），发送时刻捕获。
type Reference struct {
	Type    MentionType `json:"type"`
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Content string      `json:"content"`
}

// Resolver is implemented by each capability app; chat holds a type→resolver
// registry and calls Resolve at send time to snapshot the mentioned entity.
//
// Resolver 由各能力 app 实现；chat 持 type→resolver 注册表，发送时调 Resolve 抓取被 @ 实体快照。
type Resolver interface {
	Type() MentionType
	Resolve(ctx context.Context, id string) (*Reference, error)
}
