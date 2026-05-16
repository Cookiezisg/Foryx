// Package eventlog defines the recursive event-log protocol (5 events x 6 block types).
//
// Package eventlog 定义递归事件日志协议（5 事件 x 6 block 类型）。
package eventlog

import (
	"errors"
	"fmt"
)

// Event is one unit of the protocol; concrete types are exhaustive (5).
//
// Event 是协议的一个单位；具体类型穷举共 5 种。
type Event interface {
	EventType() string
}

// Envelope wraps an Event with the bridge-assigned sequence number.
//
// Envelope 给 Event 套上 bridge 分配的 sequence 号。
type Envelope struct {
	Seq   int64
	Event Event
}

const (
	BlockTypeText       = "text"
	BlockTypeReasoning  = "reasoning"
	BlockTypeToolCall   = "tool_call"
	BlockTypeToolResult = "tool_result"
	BlockTypeProgress   = "progress"
	BlockTypeMessage    = "message"
	BlockTypeCompaction = "compaction"
)

// IsValidBlockType reports whether t is one of the 7 enumerated block types.
//
// IsValidBlockType 报告 t 是否 7 种枚举之一。
func IsValidBlockType(t string) bool {
	switch t {
	case BlockTypeText, BlockTypeReasoning, BlockTypeToolCall,
		BlockTypeToolResult, BlockTypeProgress, BlockTypeMessage,
		BlockTypeCompaction:
		return true
	}
	return false
}

const (
	StatusStreaming = "streaming"
	StatusCompleted = "completed"
	StatusError     = "error"
	StatusCancelled = "cancelled"
)

func IsValidStatus(s string) bool {
	switch s {
	case StatusStreaming, StatusCompleted, StatusError, StatusCancelled:
		return true
	}
	return false
}

// MessageStart opens a new message; nested messages point ParentBlockID at the triggering tool_call.
//
// MessageStart 开新 message；嵌套 message 的 ParentBlockID 指向触发它的 tool_call block。
type MessageStart struct {
	ConversationID string         `json:"conversationId"`
	ID             string         `json:"id"`
	ParentBlockID  string         `json:"parentBlockId,omitempty"`
	Role           string         `json:"role"`
	Attrs          map[string]any `json:"attrs,omitempty"`
}

func (MessageStart) EventType() string { return "message_start" }

// MessageStop closes a message with terminal status and optional metadata.
//
// MessageStop 关闭 message，终态加可选元数据。
type MessageStop struct {
	ConversationID string `json:"conversationId"`
	ID             string `json:"id"`
	Status         string `json:"status"`
	StopReason     string `json:"stopReason,omitempty"`
	ErrorCode      string `json:"errorCode,omitempty"`
	ErrorMessage   string `json:"errorMessage,omitempty"`
	InputTokens    int    `json:"inputTokens,omitempty"`
	OutputTokens   int    `json:"outputTokens,omitempty"`
}

func (MessageStop) EventType() string { return "message_stop" }

// BlockStart opens a block under ParentID (a message ID or another block ID).
//
// BlockStart 在 ParentID 下开 block；ParentID 可为 message ID 或另一个 block ID。
type BlockStart struct {
	ConversationID string         `json:"conversationId"`
	ID             string         `json:"id"`
	ParentID       string         `json:"parentId"`
	MessageID      string         `json:"messageId"`
	BlockType      string         `json:"blockType"`
	Attrs          map[string]any `json:"attrs,omitempty"`
}

func (BlockStart) EventType() string { return "block_start" }

// BlockDelta appends Delta to an open block; append-only, never overwritten or reordered.
//
// BlockDelta 给开着的 block 追加 Delta；纯 append，前端不重写不重排。
type BlockDelta struct {
	ConversationID string `json:"conversationId"`
	ID             string `json:"id"`
	Delta          string `json:"delta"`
}

func (BlockDelta) EventType() string { return "block_delta" }

// BlockStop closes a block; Error is non-empty only when Status == StatusError.
//
// BlockStop 关闭 block；Error 仅 Status == StatusError 时非空。
type BlockStop struct {
	ConversationID string `json:"conversationId"`
	ID             string `json:"id"`
	Status         string `json:"status"`
	Error          string `json:"error,omitempty"`
}

func (BlockStop) EventType() string { return "block_stop" }

// ErrSeqTooOld is returned when fromSeq has been evicted from the replay buffer.
//
// ErrSeqTooOld 在 fromSeq 已被 replay buffer 淘汰时返回，客户端须 HTTP refetch 全态。
var ErrSeqTooOld = errors.New("eventlog: requested seq too old (evicted from replay buffer)")

// ErrInvalidEvent is returned when a payload fails minimal shape checks.
//
// ErrInvalidEvent 在 payload 最小形状检查失败时返回；Bridge 实现不得静默丢。
var ErrInvalidEvent = errors.New("eventlog: invalid event")

// ValidateEvent runs minimal shape checks on e; called by Bridge implementations in Publish.
//
// ValidateEvent 跑 e 的最小形状检查；Bridge 实现在 Publish 时调用。
func ValidateEvent(e Event) error {
	switch ev := e.(type) {
	case MessageStart:
		if ev.ConversationID == "" {
			return fmt.Errorf("%w: MessageStart.ConversationID empty", ErrInvalidEvent)
		}
		if ev.ID == "" {
			return fmt.Errorf("%w: MessageStart.ID empty", ErrInvalidEvent)
		}
		if ev.Role == "" {
			return fmt.Errorf("%w: MessageStart.Role empty", ErrInvalidEvent)
		}
	case MessageStop:
		if ev.ConversationID == "" {
			return fmt.Errorf("%w: MessageStop.ConversationID empty", ErrInvalidEvent)
		}
		if ev.ID == "" {
			return fmt.Errorf("%w: MessageStop.ID empty", ErrInvalidEvent)
		}
		if !IsValidStatus(ev.Status) {
			return fmt.Errorf("%w: MessageStop.Status=%q", ErrInvalidEvent, ev.Status)
		}
	case BlockStart:
		if ev.ConversationID == "" {
			return fmt.Errorf("%w: BlockStart.ConversationID empty", ErrInvalidEvent)
		}
		if ev.ID == "" {
			return fmt.Errorf("%w: BlockStart.ID empty", ErrInvalidEvent)
		}
		if ev.ParentID == "" {
			return fmt.Errorf("%w: BlockStart.ParentID empty", ErrInvalidEvent)
		}
		if ev.MessageID == "" {
			return fmt.Errorf("%w: BlockStart.MessageID empty", ErrInvalidEvent)
		}
		if !IsValidBlockType(ev.BlockType) {
			return fmt.Errorf("%w: BlockStart.BlockType=%q", ErrInvalidEvent, ev.BlockType)
		}
	case BlockDelta:
		if ev.ConversationID == "" {
			return fmt.Errorf("%w: BlockDelta.ConversationID empty", ErrInvalidEvent)
		}
		if ev.ID == "" {
			return fmt.Errorf("%w: BlockDelta.ID empty", ErrInvalidEvent)
		}
	case BlockStop:
		if ev.ConversationID == "" {
			return fmt.Errorf("%w: BlockStop.ConversationID empty", ErrInvalidEvent)
		}
		if ev.ID == "" {
			return fmt.Errorf("%w: BlockStop.ID empty", ErrInvalidEvent)
		}
		if !IsValidStatus(ev.Status) {
			return fmt.Errorf("%w: BlockStop.Status=%q", ErrInvalidEvent, ev.Status)
		}
	default:
		return fmt.Errorf("%w: unknown event type %T", ErrInvalidEvent, e)
	}
	return nil
}
