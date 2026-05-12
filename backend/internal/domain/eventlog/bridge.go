package eventlog

import "context"

// Bridge dispatches events to per-user subscribers and assigns each event
// a user-monotonic sequence number. Implementations MUST be safe for
// concurrent Publish + Subscribe. The user_id is read from ctx (via
// reqctxpkg.RequireUserID); the bridge does not appear in any callsite
// signature so producers / consumers stay decoupled from auth.
//
// Per D-redo-2 (forge_redesign 2026-05-12) the wire stream is per-user;
// payload still carries conversationID and clients demux at the panel
// level. Prior per-conversation keying was removed because trinity-side
// detail panels + multi-conversation test consoles were hitting the
// HTTP/1.1 6-connection-per-origin browser cap.
//
// Bridge 把事件分发给 per-user 订阅者,每个事件分配 user-monotonic seq。
// 实现必须支持并发 Publish + Subscribe。user_id 经 reqctxpkg.RequireUserID
// 从 ctx 读;Bridge 在 caller 签名中不出现 user_id 让 producer/consumer 跟
// auth 解耦。
//
// 按 D-redo-2(forge_redesign 2026-05-12),wire 流 per-user;payload 仍带
// conversationID,client 在 panel 层 demux。旧 per-conversation keying 被
// 去除,因为 trinity 详情面板 + 多对话 testend 撞 HTTP/1.1 浏览器 6-conn 限制。
type Bridge interface {
	// Publish reads user_id from ctx, assigns a seq, validates payload,
	// and dispatches to that user's subscribers. Returns the assigned
	// envelope.
	//
	// Semantic for slow subscribers: BLOCK the publisher (delta events
	// must not be lost — append-only semantic relies on no gaps). Each
	// subscriber buffer is small (~256 over replay) so blocking happens
	// promptly.
	//
	// Returns ErrInvalidEvent for malformed payloads (caller bug, not
	// recoverable). Returns ctx.Err() if ctx cancelled. Returns the
	// reqctx user-id error when no user_id is in ctx (wiring bug — every
	// HTTP request runs the InjectUserID middleware and detached writes
	// re-stamp it via reqctxpkg.SetUserID).
	//
	// Publish 从 ctx 读 user_id,分配 seq,校验 payload,分发给该用户订阅者。
	// 慢订阅者阻塞 publisher(delta 不允许丢)。形状错返 ErrInvalidEvent;
	// ctx 取消返 ctx.Err()。ctx 缺 user_id 返 reqctx 错(接线 bug,
	// HTTP 入口的 InjectUserID 中间件 + detached 写时 SetUserID 应保证)。
	Publish(ctx context.Context, e Event) (Envelope, error)

	// Subscribe reads user_id from ctx and registers a subscriber for
	// that user. fromSeq>0 triggers replay of buffered envelopes with
	// seq > fromSeq before live delivery. fromSeq=0 starts at live
	// (no replay). Returns ErrSeqTooOld if fromSeq is older than the
	// buffer's oldest entry.
	//
	// The returned channel is never closed by the bridge; callers stop
	// by ctx.Done() or invoking cancel. cancel is idempotent.
	//
	// Subscribe 从 ctx 读 user_id 注册订阅者。fromSeq>0 先 replay 缓存
	// seq > fromSeq 再投递实时;fromSeq=0 直接实时(无 replay)。
	// fromSeq 比 buffer 最旧还旧返 ErrSeqTooOld。
	//
	// 返 channel 不会被 bridge 关闭;调用方靠 ctx.Done() 或 cancel 停止;
	// cancel 幂等。
	Subscribe(ctx context.Context, fromSeq int64) (<-chan Envelope, func(), error)
}
