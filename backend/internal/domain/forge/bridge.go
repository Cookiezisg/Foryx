package forge

import "context"

// Bridge dispatches forge events per-user; user_id read from ctx via reqctxpkg.
//
// Bridge 把 forge 事件分发给 per-user 订阅者；user_id 从 ctx 读。
type Bridge interface {
	// Publish validates, assigns seq, dispatches; blocks slow subscribers.
	//
	// Publish 校验后分配 seq 并扇出；阻塞慢订阅者，forge 事件不允许丢。
	Publish(ctx context.Context, e Event) (Envelope, error)

	// Subscribe registers a per-user subscriber; fromSeq>0 replays first; old returns ErrSeqTooOld.
	//
	// Subscribe 注册订阅者；fromSeq>0 先 replay；过旧返 ErrSeqTooOld。
	Subscribe(ctx context.Context, fromSeq int64) (<-chan Envelope, func(), error)
}
