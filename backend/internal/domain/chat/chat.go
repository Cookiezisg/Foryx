// Package chat is the domain layer for conversation messaging: Message
// / Block / Attachment entities, sentinels, Repository contract. No LLM
// orchestration here — that's in app/chat.
//
// Block model: aligned 1:1 with the recursive event-log protocol
// (domain/eventlog) — same 6 type enumeration; per-conversation
// monotonic seq; parent_block_id for nested blocks; append-only
// content. Blocks are written ONLY through pkg/eventlog.Emitter →
// chat Repository (no other write path).
//
// Package chat 是对话消息 domain 层：Message / Block / Attachment 实体
// + sentinel + Repository 契约。LLM 编排在 app/chat。
//
// Block 模型与递归事件日志协议（domain/eventlog）1:1 对齐——同 6 类
// 枚举；per-conversation 单调 seq；parent_block_id 嵌套；append-only
// content。Block 写入唯一路径：pkg/eventlog.Emitter → chat Repository。
package chat

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Message is one conversation turn. Content lives in Blocks (loaded
// from the message_blocks table). ErrorCode / ErrorMessage are
// populated only when Status="error".
//
// ParentBlockID is set ONLY when this message is nested under a block
// of another message — used for subagent sub-runs (parent_block_id
// points to a block of type=message in the parent conv message).
// Empty for top-level user / assistant messages.
//
// Attrs is JSON metadata. Free-form sparse map; common keys:
//   - attachments: []{attachmentId, fileName, mimeType}  (user msg)
//   - kind: "subagent_run", type, maxTurns                (sub-run msg)
//
// Message 是对话的一个回合。内容在 Blocks（从 message_blocks 表加载）。
// ErrorCode / ErrorMessage 仅 Status="error" 时填。
//
// ParentBlockID 仅当本消息嵌套在另一消息的某 block 下时设——subagent
// sub-run 用（parent_block_id 指向父对话消息中 type=message 的 block）。
// 顶层 user / assistant 消息为空。
//
// Attrs 是 JSON 元数据。自由稀疏 map，常见键：
//   - attachments: 用户消息附件列表
//   - kind: "subagent_run", type, maxTurns 等（subagent sub-run 消息）
type Message struct {
	ID             string         `gorm:"primaryKey;type:text" json:"id"`
	ConversationID string         `gorm:"not null;index;type:text" json:"conversationId"`
	UserID         string         `gorm:"not null;type:text" json:"-"`
	ParentBlockID  string         `gorm:"type:text;index" json:"parentBlockId,omitempty"`
	Role           string         `gorm:"not null;type:text" json:"role"`
	Status         string         `gorm:"not null;type:text;default:'completed'" json:"status"`
	StopReason     string         `gorm:"type:text;default:''" json:"stopReason,omitempty"`
	ErrorCode      string         `gorm:"type:text;default:''" json:"errorCode,omitempty"`
	ErrorMessage   string         `gorm:"type:text;default:''" json:"errorMessage,omitempty"`
	InputTokens    int            `gorm:"default:0" json:"inputTokens,omitempty"`
	OutputTokens   int            `gorm:"default:0" json:"outputTokens,omitempty"`
	// Attrs is JSON metadata (see godoc above for typical keys). GORM
	// `serializer:json` handles marshal/unmarshal at the storage boundary
	// so callers always see a typed map (matching the SSE wire shape).
	// 2026-05 重构:从 `string` 改 `map[string]any`,GORM serializer 透明处理
	// 列读写,REST 出口跟 SSE 一致都是对象(原来 REST 是 JSON string)。
	Attrs          map[string]any `gorm:"type:text;serializer:json" json:"attrs,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`

	// Blocks is not a DB column — store layer fills via attachBlocks
	// after loading the message row.
	//
	// Blocks 非 DB 列——store 层加载 message 行后经 attachBlocks 填充。
	Blocks []Block `gorm:"-" json:"blocks"`
}

func (Message) TableName() string { return "messages" }

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

const (
	StatusPending   = "pending"
	StatusStreaming = "streaming"
	StatusCompleted = "completed"
	StatusError     = "error"
	StatusCancelled = "cancelled"
)

const (
	StopReasonEndTurn   = "end_turn"
	StopReasonMaxTokens = "max_tokens"
	StopReasonCancelled = "cancelled"
	StopReasonError     = "error"
)

// Block is one element of a Message's content tree, aligned with the
// recursive event-log protocol. The shape mirrors the Bridge wire
// format — every emit (block_start / block_delta / block_stop) writes
// or updates one row.
//
// CHECK constraints on Type / Status are GORM-tag declared to enforce
// the 6+4 closed enumerations defined in domain/eventlog.
//
// Block 是 Message content 树的一个元素，与递归事件日志协议对齐。
// 形状镜像 Bridge wire format——每次 emit (block_start / block_delta /
// block_stop) 写或更新一行。
//
// Type / Status 的 CHECK 约束经 GORM tag 声明，强制 domain/eventlog
// 定义的 6+4 封闭枚举。
type Block struct {
	ID             string    `gorm:"primaryKey;type:text" json:"id"`
	ConversationID string    `gorm:"not null;type:text;uniqueIndex:idx_blocks_conv_seq,priority:1" json:"conversationId"`
	MessageID      string    `gorm:"not null;type:text;index" json:"messageId"`
	ParentBlockID  string    `gorm:"type:text;index" json:"parentBlockId,omitempty"`
	Seq            int64     `gorm:"not null;uniqueIndex:idx_blocks_conv_seq,priority:2" json:"seq"`
	Type           string    `gorm:"not null;type:text;check:type IN ('text','reasoning','tool_call','tool_result','progress','message')" json:"type"`
	// Attrs is JSON metadata. GORM serializer:json transparently handles
	// the text column ↔ map conversion so REST output matches the SSE wire
	// shape (both: object). 2026-05 重构。
	Attrs          map[string]any `gorm:"type:text;serializer:json" json:"attrs,omitempty"`
	Content        string    `gorm:"not null;type:text;default:''" json:"content"`
	Status         string    `gorm:"not null;type:text;check:status IN ('streaming','completed','error','cancelled')" json:"status"`
	Error          string    `gorm:"type:text" json:"error,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func (Block) TableName() string { return "message_blocks" }

// ToolCallData is the in-memory parsed shape of an LLM tool call. Not
// a DB shape — block_call rows store args JSON in Block.Content; this
// is the runtime parsed form passed to runTools dispatch.
//
// Arguments excludes the three framework-injected standard fields
// (summary / destructive / execution_group); those are first-class
// fields on this struct.
//
// ToolCallData 是 LLM tool call 的内存解析形态。非 DB 形态——tool_call
// 行 args JSON 存 Block.Content；这里是 runtime 解析形给 runTools 派发用。
//
// Arguments 不含三个框架注入标准字段（summary / destructive /
// execution_group）——三者作为本 struct 一等字段。
type ToolCallData struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Summary        string         `json:"summary"`
	Destructive    bool           `json:"destructive"`
	ExecutionGroup int            `json:"executionGroup"`
	Arguments      map[string]any `json:"arguments"`
}

// ToolResultData is the in-memory shape of a tool's return value with
// status metadata. Not a DB shape — tool_result rows store the output
// string in Block.Content; this struct is used at runtime + over the
// LLM history wire (extendHistory marshals it back).
//
// ToolResultData 是 tool 返回值带状态元数据的内存形态。非 DB 形态
// ——tool_result 行 output 存 Block.Content；本 struct 用于 runtime +
// LLM 历史 wire（extendHistory marshal 回去）。
type ToolResultData struct {
	ToolCallID string `json:"toolCallId"`
	OK         bool   `json:"ok"`
	Result     string `json:"result"`
	ErrorMsg   string `json:"errorMsg,omitempty"`
	ElapsedMs  int64  `json:"elapsedMs,omitempty"`
}

// AttachmentRef is the JSON shape used when a user message's Attrs
// JSON contains an "attachments" array. Convention only — Message.Attrs
// is a free-form JSON string; this struct documents the shape callers
// agree on.
//
// AttachmentRef 是 user message 的 Attrs JSON 含 "attachments" 数组时
// 的 JSON 形状。仅约定——Message.Attrs 是自由 JSON 字符串；本 struct
// 记录调用方约定的形状。
type AttachmentRef struct {
	AttachmentID string `json:"attachmentId"`
	FileName     string `json:"fileName"`
	MimeType     string `json:"mimeType"`
}

// Attachment is a user-uploaded file. Bytes at StoragePath; row is
// metadata only. Soft-deletable so older conversations don't lose refs.
//
// Attachment 是用户上传文件。字节在 StoragePath；行仅元数据。
// 软删除——避免旧对话失去引用。
type Attachment struct {
	ID          string         `gorm:"primaryKey;type:text" json:"id"`
	UserID      string         `gorm:"not null;index;type:text" json:"-"`
	FileName    string         `gorm:"not null;type:text" json:"fileName"`
	MimeType    string         `gorm:"not null;type:text" json:"mimeType"`
	SizeBytes   int64          `gorm:"not null" json:"sizeBytes"`
	StoragePath string         `gorm:"not null;type:text" json:"-"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Attachment) TableName() string { return "attachments" }

// MaxAttachmentBytes is the upload size limit (50 MB).
//
// MaxAttachmentBytes 上传大小限制（50 MB）。
const MaxAttachmentBytes = 50 * 1024 * 1024

// ListFilter is the query shape for paginated message listing.
//
// ListFilter 分页消息列表的查询形状。
type ListFilter struct {
	Cursor string
	Limit  int
}

// ReplayEnvelope is the wire shape returned by the
// /api/v1/conversations/{id}/eventlog?from=N HTTP refetch endpoint.
// Mirrors eventlog.Envelope but flattens type + seq into the JSON body
// so HTTP clients see one uniform shape (live SSE puts type/seq in
// headers; HTTP refetch puts them in body).
//
// ReplayEnvelope 是 /api/v1/conversations/{id}/eventlog?from=N HTTP
// refetch 端点返的 wire 形状。镜像 eventlog.Envelope 但把 type + seq
// 拍到 JSON body——HTTP 客户端拿到统一形状（live SSE 把 type/seq 放
// header；HTTP refetch 放 body）。
type ReplayEnvelope struct {
	Type    string         `json:"type"`
	Seq     int64          `json:"seq"`
	Payload map[string]any `json:"payload"`
}

var (
	ErrMessageNotFound           = errors.New("chat: message not found")
	ErrBlockNotFound             = errors.New("chat: block not found")
	ErrStreamNotFound            = errors.New("chat: no active stream for conversation")
	ErrStreamInProgress          = errors.New("chat: stream already in progress")
	ErrAttachmentTooLarge        = errors.New("chat: attachment exceeds 50 MB limit")
	ErrAttachmentTypeUnsupported = errors.New("chat: attachment type not supported")
	ErrAttachmentParseFailed     = errors.New("chat: attachment parse failed")
)

// Repository is the storage contract for Message / Block / Attachment.
// Message methods are scoped to ctx user; Block methods are scoped by
// conversation_id (auth lives on the parent message — block writes
// happen server-side trusted via emitter, no per-block user filter).
//
// Repository 是 Message / Block / Attachment 存储契约。Message 方法按
// ctx 用户过滤；Block 方法按 conversation_id 过滤（auth 在父 message
// 上——block 写由 emitter server-side 可信，不做 per-block user 过滤）。
type Repository interface {
	// ── Message ────────────────────────────────────────────────────

	// SaveMessage upserts a Message row (no block writes — those go
	// through SaveBlock / AppendDelta / FinalizeStop).
	//
	// SaveMessage upsert message 行（不写 block——那些走 SaveBlock /
	// AppendDelta / FinalizeStop）。
	SaveMessage(ctx context.Context, m *Message) error

	// GetMessage fetches by id (with Blocks attached), scoped to
	// ctx user; ErrMessageNotFound when absent.
	//
	// GetMessage 按 id 取（含 Blocks），按 ctx 用户过滤；缺失返
	// ErrMessageNotFound。
	GetMessage(ctx context.Context, id string) (*Message, error)

	// ListMessagesByConversation returns cursor-paginated messages
	// (with Blocks), ordered by created_at ASC.
	//
	// ListMessagesByConversation 返带 Blocks 的 cursor 分页 message，
	// created_at ASC 排序。
	ListMessagesByConversation(ctx context.Context, conversationID string, filter ListFilter) ([]*Message, string, error)

	// ── Block ──────────────────────────────────────────────────────

	// SaveBlock upserts a Block row (used by emitter at block_start
	// + block_stop). Concurrent appends to content go through
	// AppendDelta, not Save (avoids read-modify-write race).
	//
	// SaveBlock upsert block 行（emitter 在 block_start + block_stop
	// 时调）。content 并发追加走 AppendDelta，不走 Save。
	SaveBlock(ctx context.Context, b *Block) error

	// AppendDelta atomically appends delta to blockID's content via
	// SQL string concat. ErrBlockNotFound if the row doesn't exist.
	//
	// AppendDelta 经 SQL 字符串拼接原子追加 delta。行不存在返
	// ErrBlockNotFound。
	AppendDelta(ctx context.Context, blockID, delta string) error

	// FinalizeStop sets terminal status + error on blockID.
	// ErrBlockNotFound if the row doesn't exist.
	//
	// FinalizeStop 给 blockID 设终态 status + error。行不存在返
	// ErrBlockNotFound。
	FinalizeStop(ctx context.Context, blockID, status, errStr string) error

	// GetBlock returns blockID's row. ErrBlockNotFound when absent.
	//
	// GetBlock 返 blockID 的行。缺失返 ErrBlockNotFound。
	GetBlock(ctx context.Context, blockID string) (*Block, error)

	// ListBlocksByConversation returns all blocks of conversationID
	// ordered by seq ASC. Used by history replay endpoint.
	//
	// ListBlocksByConversation 返 conversationID 所有 block，seq ASC。
	// 历史 replay 端点用。
	ListBlocksByConversation(ctx context.Context, conversationID string) ([]*Block, error)

	// ListBlocksByMessage returns all blocks of messageID, seq ASC.
	//
	// ListBlocksByMessage 返 messageID 所有 block，seq ASC。
	ListBlocksByMessage(ctx context.Context, messageID string) ([]*Block, error)

	// ReplayEventsAfter reconstructs the block-as-events stream for
	// conversationID with seq > fromSeq. Each row produces 3 envelopes
	// (block_start + block_delta(content) + block_stop), all sharing
	// the row's seq.
	//
	// ReplayEventsAfter 重构 conversationID 中 seq > fromSeq 的 blocks-
	// as-events 序列。每行产 3 envelope（block_start + block_delta(content)
	// + block_stop），共享行 seq。
	ReplayEventsAfter(ctx context.Context, conversationID string, fromSeq int64) ([]ReplayEnvelope, error)

	// ── Attachment ─────────────────────────────────────────────────

	// SaveAttachment inserts (write-once).
	//
	// SaveAttachment 插入（一次写）。
	SaveAttachment(ctx context.Context, a *Attachment) error

	// GetAttachment fetches by id, scoped to ctx user.
	//
	// GetAttachment 按 id 取，按 ctx 用户过滤。
	GetAttachment(ctx context.Context, id string) (*Attachment, error)
}
