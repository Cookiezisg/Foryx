package eventlog

import "context"

// Bridge dispatches events per-user with monotonic seq; safe for concurrent Publish + Subscribe.
//
// Bridge 按 user 分发事件并分配单调 seq；实现须支持并发 Publish + Subscribe。
type Bridge interface {
	// Publish reads user_id from ctx, assigns seq, validates payload, dispatches.
	// Blocks slow subscribers so delta events are never dropped.
	//
	// Publish 从 ctx 读 user_id 分配 seq 后分发；阻塞慢订阅者，delta 不允许丢。
	Publish(ctx context.Context, e Event) (Envelope, error)

	// Subscribe registers a per-user subscriber; fromSeq>0 replays buffered events first.
	// Returned channel is not closed by the bridge; cancel is idempotent.
	//
	// Subscribe 注册 per-user 订阅者；fromSeq>0 先 replay 缓存。channel 不由 bridge 关，cancel 幂等。
	Subscribe(ctx context.Context, fromSeq int64) (<-chan Envelope, func(), error)
}
