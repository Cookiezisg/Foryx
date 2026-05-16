// Package notifications defines the entity-update event bus (one generic Event envelope).
//
// Package notifications 定义 entity-update 事件总线（一个通用 Event envelope）。
package notifications

import (
	"context"
	"errors"
	"fmt"
)

// Event is a single notification; Type discriminates entity kind, Data is the slim payload.
//
// Event 是单条 notification；Type 判别实体种类，Data 为瘦身 payload。
type Event struct {
	Type           string `json:"type"`
	ID             string `json:"id"`
	Data           any    `json:"data"`
	ConversationID string `json:"conversationId,omitempty"`
}

// Envelope wraps an Event with the bridge-assigned seq.
//
// Envelope 给 Event 套上 bridge 分配的 seq。
type Envelope struct {
	Seq   int64
	Event Event
}

// Bridge dispatches notifications per-user; user_id read from ctx via reqctxpkg.
//
// Bridge 把通知分发给 per-user 订阅者；user_id 从 ctx 读。
type Bridge interface {
	// Publish validates, assigns seq, dispatches; blocks slow subscribers so snapshots aren't lost.
	//
	// Publish 校验后分配 seq 并扇出；阻塞慢订阅者，快照不能丢。
	Publish(ctx context.Context, e Event) (Envelope, error)

	// Subscribe registers a per-user subscriber; fromSeq>0 replays first, too old returns ErrSeqTooOld.
	//
	// Subscribe 注册订阅者；fromSeq>0 先 replay；过旧返 ErrSeqTooOld。
	Subscribe(ctx context.Context, fromSeq int64) (<-chan Envelope, func(), error)
}

// ErrSeqTooOld is returned when fromSeq has been evicted; client should resubscribe live + REST refetch.
//
// ErrSeqTooOld fromSeq 已被淘汰；client 应重订 live 并 REST 取状态。
var ErrSeqTooOld = errors.New("notifications: requested seq too old (evicted from replay buffer)")

// ErrInvalidEvent is returned for malformed events (producer bug).
//
// ErrInvalidEvent 形状错误事件返（Producer bug）。
var ErrInvalidEvent = errors.New("notifications: invalid event")

// ValidateEvent runs minimal shape checks; Bridge calls this in Publish.
//
// ValidateEvent 跑最小形状检查；Bridge 在 Publish 时调用。
func ValidateEvent(e Event) error {
	if e.Type == "" {
		return fmt.Errorf("%w: empty Type", ErrInvalidEvent)
	}
	if e.ID == "" {
		return fmt.Errorf("%w: empty ID", ErrInvalidEvent)
	}
	return nil
}
