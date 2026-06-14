// Package conversation is the domain layer for chat-thread containers: the persistent,
// per-workspace thread entity (title, pin/archive, soft-delete) plus its thread-level
// config (system prompt, attached documents, model override). Messages are NOT here —
// they belong to chat; this package owns only the thread record + storage contract.
//
// Package conversation 是对话线程容器的 domain 层：按 workspace 持久化的线程实体（标题、
// 置顶/归档、软删）及其线程级配置（system prompt、挂载文档、模型覆盖）。消息**不在这里**——
// 归 chat；本包只持有线程记录 + 存储契约。
package conversation

import (
	"context"
	"time"

	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	errorspkg "github.com/sunweilin/forgify/backend/internal/pkg/errors"
)

// Conversation is a chat-thread container. The thread's messages live in chat's
// message_blocks; this record carries only the thread's identity, interaction state,
// and the config the chat runtime reads each turn. Summary / SummaryCoversUpToSeq are
// written by the compactor (app/contextmgr); AutoTitled is set by chat after it auto-names
// a turn-1 thread — all three are declared here (one coherent thread record) but kept off the
// PATCH surface. SystemPrompt / AttachedDocuments / ModelOverride are user-editable settings
// (this is conversation's job); chat merely consumes them at runtime.
//
// Conversation 是对话线程容器。线程消息在 chat 的 message_blocks；本记录只承载线程身份、
// 交互状态、chat 运行时每轮要读的配置。Summary / SummaryCoversUpToSeq 由压缩器（app/contextmgr）
// 写；AutoTitled 由 chat 给首轮线程自动命名后设——三者在此声明（一份内聚的线程记录）但
// 不进 PATCH 面。SystemPrompt / AttachedDocuments / ModelOverride 是用户可改的设置（conversation
// 的职责）；chat 仅在运行时消费。
type Conversation struct {
	ID                   string                            `db:"id,pk"                    json:"id"`
	WorkspaceID          string                            `db:"workspace_id,ws"          json:"-"`
	Title                string                            `db:"title"                    json:"title"`
	AutoTitled           bool                              `db:"auto_titled"              json:"autoTitled"`
	SystemPrompt         string                            `db:"system_prompt"            json:"systemPrompt,omitempty"`
	Summary              string                            `db:"summary"                  json:"summary,omitempty"`
	SummaryCoversUpToSeq int64                             `db:"summary_covers_up_to_seq" json:"summaryCoversUpToSeq,omitempty"`
	AttachedDocuments    []documentdomain.AttachedDocument `db:"attached_documents,json"  json:"attachedDocuments,omitempty"`
	Archived             bool                              `db:"archived"                 json:"archived"`
	Pinned               bool                              `db:"pinned"                   json:"pinned"`
	ModelOverride        *modeldomain.ModelRef             `db:"model_override,json"      json:"modelOverride,omitempty"`
	CreatedAt            time.Time                         `db:"created_at,created"       json:"createdAt"`
	UpdatedAt            time.Time                         `db:"updated_at,updated"       json:"updatedAt"`
	// LastMessageAt is the recency-sort key: the time of the most recent message added to the
	// thread (set at creation, bumped by chat on each user turn). It is a plain column — NOT the
	// ,updated tag — so pin/rename/model-override (which bump updated_at) never reorder the list.
	//
	// LastMessageAt 是最近活跃排序键：线程最后一条消息加入的时间（创建时设、chat 每个用户回合刷）。
	// 它是普通列、非 ,updated tag——故 pin/改名/换模型（刷 updated_at）不会重排列表。
	LastMessageAt time.Time  `db:"last_message_at" json:"lastMessageAt"`
	DeletedAt     *time.Time `db:"deleted_at,deleted" json:"-"`

	// IsGenerating is a derived runtime flag (NOT persisted, db:"-"): true when chat has an
	// in-flight assistant turn for this conversation. Filled per-row in the app layer from the
	// chat registry (GeneratingQuerier) so a freshly-connected client can cold-start its live
	// activity dots; read-only on the wire, never accepted in PATCH.
	//
	// IsGenerating 是派生运行时标志（不落库，db:"-"）：chat 有该对话在途 assistant 回合时为 true。
	// 由 app 层据 chat 登记（GeneratingQuerier）逐行填，使刚连上的客户端能冷启动活动圆点；线缆只读、
	// 不进 PATCH。
	IsGenerating bool `db:"-" json:"isGenerating"`
}

// ListFilter narrows the conversation list. Archived: nil = exclude archived (default),
// &true = archived only, &false = active only. Search is a case-insensitive title LIKE.
//
// ListFilter 收窄对话列表。Archived：nil = 排除已归档（默认），&true = 仅归档，&false = 仅活跃。
// Search 是标题大小写不敏感 LIKE。
type ListFilter struct {
	Cursor   string
	Limit    int
	Search   string
	Archived *bool
}

// UpdateInput is the PATCH payload; a nil field is left unchanged. ModelOverride is a
// pointer-to-pointer for tristate: nil = leave, &nil = clear, &(&ref) = set.
//
// UpdateInput 是 PATCH 载荷；nil 字段不动。ModelOverride 是指针的指针以表三态：nil = 不变、
// &nil = 清除、&(&ref) = 设置。
type UpdateInput struct {
	Title             *string
	SystemPrompt      *string
	AttachedDocuments *[]documentdomain.AttachedDocument
	Archived          *bool
	Pinned            *bool
	ModelOverride     **modeldomain.ModelRef
}

var (
	// ErrNotFound: get/update/delete on an unknown (or soft-deleted) conversation.
	// ErrNotFound：对未知（或已软删）对话 get/update/delete。
	ErrNotFound = errorspkg.New(errorspkg.KindNotFound, "CONVERSATION_NOT_FOUND", "conversation not found")

	// ErrInvalidModelOverride: a set modelOverride is missing apiKeyId or modelId. Mirrors
	// agent — structural only; key existence is resolved (and may fail gracefully) at chat time.
	// ErrInvalidModelOverride：已设的 modelOverride 缺 apiKeyId 或 modelId。照 agent——仅结构；
	// key 存在性在 chat 时解析（可优雅失败）。
	ErrInvalidModelOverride = errorspkg.New(errorspkg.KindUnprocessable, "CONVERSATION_INVALID_MODEL_OVERRIDE", "invalid modelOverride (apiKeyId and modelId both required)")
)

// Repository is the storage contract; workspace isolation + soft-delete are applied by the
// orm layer from ctx, so methods take no workspace id and List excludes tombstones.
//
// Repository 是存储契约；workspace 隔离 + 软删由 orm 层据 ctx 施加，故方法不带 workspace id、
// List 自动排除墓碑。
type Repository interface {
	Insert(ctx context.Context, c *Conversation) error
	Get(ctx context.Context, id string) (*Conversation, error)
	GetBatch(ctx context.Context, ids []string) ([]*Conversation, error)
	List(ctx context.Context, filter ListFilter) (items []*Conversation, next string, err error)
	Update(ctx context.Context, c *Conversation) error
	// TouchLastMessage sets last_message_at on one conversation (chat calls it when a message is
	// added) — a single cheap UPDATE; the ORM ,updated tag also bumps updated_at, as expected.
	//
	// TouchLastMessage 把某对话的 last_message_at 设为 t（chat 在消息加入时调）——一次廉价 UPDATE；
	// ORM ,updated tag 顺带刷 updated_at，符合预期。
	TouchLastMessage(ctx context.Context, id string, t time.Time) error
	SoftDelete(ctx context.Context, id string) error
}
