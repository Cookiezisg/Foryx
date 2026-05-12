// Package notifications defines the entity-update event bus. One generic
// envelope (Event) covers all entity types — `Type` is the discriminator
// string, `Data` carries the slim payload (per D-redo-6: just action +
// minimal IDs; UI does GET for full entity details).
//
// Per D-redo-3 (forge_redesign 2026-05-12) Bridge keys by user_id read
// from ctx — payload still carries ConversationID for client-side
// dispatch.
//
// Distinct from domain/eventlog: that protocol streams chat content
// (5 events × 6 block types). This protocol pushes entity state changes
// ("function X created", "todo Z updated"). Both share the same Bridge
// implementation pattern (per-user seq + replay buffer + Last-Event-ID
// reconnect) since the D-redo-2 + D-redo-3 unification.
//
// See documents/version-1.2/event-log-protocol.md and
// adhoc-topic-documents/forge_redesign/07-notifications-and-eventlog.md.
//
// Package notifications 定义 entity-update 事件总线。Event 通用 envelope
// 覆盖所有实体;Type 判别;Data 瘦身 payload(D-redo-6 仅 action + ID,UI
// 经 GET 取完整 entity)。
//
// 按 D-redo-3(forge_redesign 2026-05-12)Bridge 按 ctx 中 user_id key;
// payload 仍带 ConversationID 给 client dispatch。
//
// 跟 domain/eventlog 区别:那个流 chat 内容(5 events × 6 block);本协议
// 推 entity 状态变更。两者 D-redo-2 + D-redo-3 后共享 per-user Bridge 模式。
package notifications

import (
	"context"
	"errors"
	"fmt"
)

// Event is a single notification carrying an entity snapshot.
//
// Type discriminates entity kind: "conversation" / "todo" / future
// "mcp_server" / "skill" / "system_warning" etc. Subscribers (frontend
// UI) dispatch on Type to entity-specific renderers.
//
// ConversationID is set only when the entity is conversation-scoped
// (e.g. "todo" has a conversationId; "system_warning" doesn't).
// Frontends watching a specific conversation can filter by it; the
// global sidebar ignores it.
//
// Event 是单条 notification，携 entity 快照。
//
// Type 区分实体种类："conversation" / "todo" / 未来 "mcp_server" /
// "skill" / "system_warning" 等。订阅方（前端 UI）按 Type 分派到
// entity-specific renderer。
//
// ConversationID 仅当 entity 跟某对话相关时填（例：todo 有
// conversationId；system_warning 没有）。绑定到某对话的前端可按它
// 过滤；全局侧栏忽略。
type Event struct {
	Type           string `json:"type"`
	ID             string `json:"id"`
	Data           any    `json:"data"`
	ConversationID string `json:"conversationId,omitempty"`
}

// Envelope wraps an Event with its bridge-assigned sequence number.
//
// Envelope 给 Event 套上 bridge 分配的 seq。
type Envelope struct {
	Seq   int64
	Event Event
}

// Bridge dispatches notifications to per-user subscribers and assigns
// each event a user-monotonic sequence number. Implementations MUST be
// safe for concurrent Publish + Subscribe. user_id is read from ctx
// (reqctxpkg.RequireUserID); bridge stays out of caller signatures so
// producer / consumer decouple from auth wiring.
//
// Per D-redo-3 (forge_redesign 2026-05-12); subscribers filter client-
// side on Type / ConversationID within their per-user stream.
//
// Bridge 把通知分发给 per-user 订阅者,每事件分配 user-monotonic seq;
// user_id 经 ctx 读(D-redo-3,forge_redesign 2026-05-12)。订阅方按
// Type / ConversationID 在自家流内 client-side 过滤。
type Bridge interface {
	// Publish reads user_id from ctx, validates, assigns seq, dispatches
	// to that user's subscribers. Block-on-slow semantic (entity
	// snapshots can't be lost — UI relies on seeing every state change).
	// Returns ErrInvalidEvent for malformed payloads or the reqctx
	// user-id error when ctx is missing user_id.
	//
	// Publish 从 ctx 读 user_id,校验、分配 seq、扇出该用户订阅者。
	// 慢订阅者阻塞 publisher(快照不能丢)。形状错误返 ErrInvalidEvent,
	// ctx 缺 user_id 返 reqctx 错。
	Publish(ctx context.Context, e Event) (Envelope, error)

	// Subscribe reads user_id from ctx, registers a subscriber. fromSeq>0
	// replays buffered envelopes with seq > fromSeq before live;
	// ErrSeqTooOld if too old.
	//
	// Subscribe 从 ctx 读 user_id 注册订阅者。fromSeq>0 先 replay 缓存中
	// seq > fromSeq 再投递实时;过旧返 ErrSeqTooOld。
	Subscribe(ctx context.Context, fromSeq int64) (<-chan Envelope, func(), error)
}

// ErrSeqTooOld is returned by Bridge.Subscribe when fromSeq has been
// evicted from the replay buffer. Client should resubscribe with
// fromSeq=0 (live only) and re-fetch any state it cares about via REST.
//
// ErrSeqTooOld 由 Bridge.Subscribe 在 fromSeq 已被 replay buffer 淘汰
// 时返。客户端应重订 fromSeq=0（仅实时）再经 REST 取需要的状态。
var ErrSeqTooOld = errors.New("notifications: requested seq too old (evicted from replay buffer)")

// ErrInvalidEvent is returned for malformed events (empty Type / ID).
// Producer bug — caller should fix.
//
// ErrInvalidEvent 形状错误事件（空 Type / ID）返。Producer bug。
var ErrInvalidEvent = errors.New("notifications: invalid event")

// ValidateEvent runs minimal shape checks. Empty Type / ID fail; Data
// can be nil (rare — e.g. signaling event with no payload). Bridge
// implementations call this in Publish so violations surface at the
// producer boundary.
//
// ValidateEvent 跑最小形状检查。空 Type / ID 失败；Data 可空（罕见
// ——如纯信号事件无 payload）。Bridge 实现在 Publish 中调，让违规在
// producer 边界暴露。
func ValidateEvent(e Event) error {
	if e.Type == "" {
		return fmt.Errorf("%w: empty Type", ErrInvalidEvent)
	}
	if e.ID == "" {
		return fmt.Errorf("%w: empty ID", ErrInvalidEvent)
	}
	return nil
}
