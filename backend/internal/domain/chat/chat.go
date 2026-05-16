// Package chat is the domain layer for conversation messaging: Message / Block / Attachment + Repository.
//
// Package chat 是对话消息 domain 层：Message / Block / Attachment 实体与 Repository 契约。
package chat

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Message is one conversation turn; content lives in Blocks loaded from message_blocks.
//
// Message 是对话的一个回合，内容在 Blocks（从 message_blocks 表加载）。
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
	Attrs          map[string]any `gorm:"type:text;serializer:json" json:"attrs,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`

	// Blocks is not a DB column; store layer fills via attachBlocks.
	//
	// Blocks 非 DB 列，store 层加载后填充。
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

// Block is one element of a Message's content tree, aligned with the recursive event-log protocol.
//
// Block 是 Message content 树的一个元素，与递归事件日志协议对齐。
type Block struct {
	ID             string         `gorm:"primaryKey;type:text" json:"id"`
	ConversationID string         `gorm:"not null;type:text;uniqueIndex:idx_blocks_conv_seq,priority:1;index:idx_mb_conv_role,priority:1" json:"conversationId"`
	MessageID      string         `gorm:"not null;type:text;index" json:"messageId"`
	ParentBlockID  string         `gorm:"type:text;index" json:"parentBlockId,omitempty"`
	Seq            int64          `gorm:"not null;uniqueIndex:idx_blocks_conv_seq,priority:2" json:"seq"`
	Type           string         `gorm:"not null;type:text;check:type IN ('text','reasoning','tool_call','tool_result','progress','message','compaction')" json:"type"`
	Attrs          map[string]any `gorm:"type:text;serializer:json" json:"attrs,omitempty"`
	Content        string         `gorm:"not null;type:text;default:''" json:"content"`
	Status         string         `gorm:"not null;type:text;check:status IN ('streaming','completed','error','cancelled')" json:"status"`
	Error          string         `gorm:"type:text" json:"error,omitempty"`
	// ContextRole projects how this block reaches LLM history: hot/warm/cold/archived.
	// ContextRole 控制本 block 投影到 LLM history 的形态；DB Content 永不改写。
	ContextRole string    `gorm:"type:text;not null;default:'hot';check:context_role IN ('hot','warm','cold','archived');index:idx_mb_conv_role,priority:2" json:"contextRole,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (Block) TableName() string { return "message_blocks" }

// ToolCallData is the in-memory parsed shape of an LLM tool call (not a DB shape).
//
// ToolCallData 是 LLM tool call 的内存解析形态（非 DB 形态）。
type ToolCallData struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Summary        string         `json:"summary"`
	Destructive    bool           `json:"destructive"`
	ExecutionGroup int            `json:"executionGroup"`
	Arguments      map[string]any `json:"arguments"`
}

// ToolResultData is the in-memory shape of a tool's return value with status metadata.
//
// ToolResultData 是 tool 返回值带状态元数据的内存形态。
type ToolResultData struct {
	ToolCallID string `json:"toolCallId"`
	OK         bool   `json:"ok"`
	Result     string `json:"result"`
	ErrorMsg   string `json:"errorMsg,omitempty"`
	ElapsedMs  int64  `json:"elapsedMs,omitempty"`
}

// AttachmentRef is the JSON shape inside a user message Attrs "attachments" array.
//
// AttachmentRef 是 user message Attrs 中 "attachments" 数组项的 JSON 形状。
type AttachmentRef struct {
	AttachmentID string `json:"attachmentId"`
	FileName     string `json:"fileName"`
	MimeType     string `json:"mimeType"`
}

// Attachment is a user-uploaded file; bytes live at StoragePath, row is metadata only.
//
// Attachment 是用户上传文件，字节在 StoragePath，行仅元数据。
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

const MaxAttachmentBytes = 50 * 1024 * 1024

type ListFilter struct {
	Cursor string
	Limit  int
}

// ReplayEnvelope is the wire shape returned by the eventlog HTTP refetch endpoint.
//
// ReplayEnvelope 是事件日志 HTTP refetch 端点返的 wire 形状。
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
	ErrAttachmentNotFound        = errors.New("chat: attachment not found")
	ErrEmptyContent              = errors.New("chat: message has empty content and no attachments")
)

// Repository is the storage contract for Message / Block / Attachment.
//
// Repository 是 Message / Block / Attachment 的存储契约。
type Repository interface {
	SaveMessage(ctx context.Context, m *Message) error
	GetMessage(ctx context.Context, id string) (*Message, error)
	ListMessagesByConversation(ctx context.Context, conversationID string, filter ListFilter) ([]*Message, string, error)

	SaveBlock(ctx context.Context, b *Block) error

	// AppendDelta atomically appends delta via SQL concat; ErrBlockNotFound when missing.
	//
	// AppendDelta 用 SQL 拼接原子追加 delta；行不存在返 ErrBlockNotFound。
	AppendDelta(ctx context.Context, blockID, delta string) error

	FinalizeStop(ctx context.Context, blockID, status, errStr string) error
	GetBlock(ctx context.Context, blockID string) (*Block, error)
	ListBlocksByConversation(ctx context.Context, conversationID string) ([]*Block, error)
	ListBlocksByMessage(ctx context.Context, messageID string) ([]*Block, error)
	UpdateBlockRole(ctx context.Context, blockID, role string) error

	// ReplayEventsAfter reconstructs the block-as-events stream for seq > fromSeq.
	//
	// ReplayEventsAfter 重构 seq > fromSeq 的 blocks-as-events 序列。
	ReplayEventsAfter(ctx context.Context, conversationID string, fromSeq int64) ([]ReplayEnvelope, error)

	SaveAttachment(ctx context.Context, a *Attachment) error
	GetAttachment(ctx context.Context, id string) (*Attachment, error)
}
