package stream

import (
	"context"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// Bridge is the per-workspace dispatch port for one stream: assign seq, buffer
// durable frames for replay, fan out to subscribers. Implemented in infra/stream
// (a single Bus instantiated three times). Each stream re-declares a thin Bridge
// (messages.Bridge / entities.Bridge / notifications.Bridge) for typed injection.
//
// Bridge 是单条流的 per-workspace 分发端口：分配 seq、把 durable 帧入 buffer 供
// replay、扇出订阅者。实现在 infra/stream（单一 Bus 实例化三次）。各流再声明 thin
// Bridge 供强类型注入。
type Bridge interface {
	// Publish validates e, stamps a seq (0 for ephemeral), buffers durable frames, fans out.
	//
	// Publish 校验 e、盖 seq（ephemeral 为 0）、durable 帧入 buffer、扇出。
	Publish(ctx context.Context, e Event) (Envelope, error)

	// Subscribe registers a subscriber; fromSeq>0 replays buffered durable frames first.
	// The channel is not closed by the bridge; cancel is idempotent. Too old → ErrSeqTooOld.
	//
	// Subscribe 注册订阅者；fromSeq>0 先 replay 缓存的 durable 帧。channel 不由 bridge
	// 关，cancel 幂等。过旧 → ErrSeqTooOld。
	Subscribe(ctx context.Context, fromSeq int64) (<-chan Envelope, func(), error)
}

// ListReader extends Bridge with a REST snapshot pull. The notifications stream uses
// it (no DB persistence — List reads the in-memory replay buffer); messages/entities
// inject the plain Bridge. The single Bus implements ListReader, so it satisfies both
// — the distinction is which interface a consumer is wired with.
//
// ListReader 在 Bridge 上加 REST 快照拉取。notifications 流用它（无 DB 落盘，List 读
// 内存 replay buffer）；messages/entities 注入纯 Bridge。单一 Bus 实现 ListReader，故
// 两者皆满足——区别在消费方按哪个接口接线。
type ListReader interface {
	Bridge
	List(ctx context.Context, fromSeq int64, limit int) ([]Envelope, bool, error)
}

// ErrSeqTooOld is returned when fromSeq has been evicted from the replay buffer; the
// client must refetch full state (messages: DB history; entities/notifications: resubscribe).
// It is a structured domain error (KindGone → HTTP 410) so transport maps it via
// statusForKind with no special case — this error reaches the wire.
//
// ErrSeqTooOld 在 fromSeq 已被 replay buffer 淘汰时返回；客户端须全量重取（messages 走
// DB 历史；entities/notifications 重订阅）。它是结构化 domain 错误（KindGone → HTTP
// 410），transport 经 statusForKind 映射、零特例——这个错误会上线缆。
var ErrSeqTooOld = errorsdomain.New(errorsdomain.KindGone, "SEQ_TOO_OLD",
	"requested seq too old (evicted from replay buffer)")
