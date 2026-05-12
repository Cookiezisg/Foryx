// Package notifications provides the in-process Bridge for entity-
// update notification events. Per D-redo-3 (forge_redesign 2026-05-12)
// keying is per-user (read from ctx) — clients filter on payload Type /
// ConversationID. Per-user seq monotonic; replay buffer + Last-Event-ID
// reconnect; block-on-slow-subscriber semantic (entity snapshots
// matter — losing them leaves UI showing stale state).
//
// Mirrors infra/eventlog/bridge.go pattern (post-D-redo-2 both are
// per-user); differences are notif payload type + smaller default
// buffer (entity-state changes are less frequent than chat tokens).
//
// Package notifications 提供 entity-update 通知事件的进程内 Bridge。
// 按 D-redo-3(forge_redesign 2026-05-12)key 改 per-user(从 ctx 读),
// client 按 payload Type / ConversationID 过滤。per-user 单调 seq +
// replay buffer + Last-Event-ID 重连 + 慢订阅者阻塞 publisher。
//
// 镜像 infra/eventlog/bridge.go(D-redo-2 后两边都 per-user);区别仅
// payload type + 较小默认 buffer(entity 状态变化比 chat token 频率低)。
package notifications

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	notificationsdomain "github.com/sunweilin/forgify/backend/internal/domain/notifications"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Tunables. Sized for single-user desktop:
//   - replayBufferSize: keeps last N events for reconnect. 1024 is
//     generous; entity state changes are infrequent.
//   - subscriberBufferSize: buffer per subscribe channel. Wide enough
//     to hold a full replay burst plus live headroom.
//
// 调参,按单用户桌面;1024 足量;subscriber 容下完整 replay + 实时余量。
const (
	replayBufferSize     = 1024
	subscriberBufferSize = 1280 // = replayBufferSize + 256 live headroom
)

// Bridge is the thread-safe in-process notification dispatcher keyed by
// user_id (read from ctx).
//
// Bridge 是线程安全的进程内通知分发器,按 user_id key(从 ctx 读)。
type Bridge struct {
	mu    sync.Mutex
	users map[string]*userState
}

// userState holds per-user seq counter + replay buffer + subs.
//
// userState 持有 per-user 的 seq 计数器 + replay buffer + sub。
type userState struct {
	mu     sync.Mutex
	seq    int64
	buffer []notificationsdomain.Envelope
	subs   []*subscription
}

type subscription struct {
	ch     chan notificationsdomain.Envelope
	done   chan struct{}
	closed sync.Once
}

// NewBridge constructs an empty Bridge. The log parameter is accepted
// for API symmetry with eventlog.NewBridge but is currently unused —
// the bridge follows §S10's "synchronous primitive" rule (don't
// self-log; let callers decide).
//
// NewBridge 构造空 Bridge。log 参数为 API 对称(与 eventlog.NewBridge 一致)
// 保留,目前未用——bridge 按 §S10 同步原语原则不自打日志。
func NewBridge(_ *zap.Logger) *Bridge {
	return &Bridge{
		users: make(map[string]*userState),
	}
}

// ensureUser returns the userState for id, creating it on first touch.
//
// ensureUser 返 id 的 userState;首次访问时创建。
func (b *Bridge) ensureUser(id string) *userState {
	b.mu.Lock()
	defer b.mu.Unlock()
	state, ok := b.users[id]
	if !ok {
		state = &userState{}
		b.users[id] = state
	}
	return state
}

// Publish reads user_id from ctx, validates, assigns seq (per-user
// monotonic), appends to replay buffer, and fans out to that user's
// subscribers. Blocks if any subscriber buffer is full (intentional —
// snapshots must not be lost).
//
// Publish 从 ctx 读 user_id,校验、分配 per-user 单调 seq、追加 replay buffer、
// 扇出给该用户订阅者。订阅者 buffer 满时阻塞(故意——快照不能丢)。
func (b *Bridge) Publish(ctx context.Context, e notificationsdomain.Event) (notificationsdomain.Envelope, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return notificationsdomain.Envelope{}, fmt.Errorf("notifications.Bridge.Publish: %w", err)
	}
	if err := notificationsdomain.ValidateEvent(e); err != nil {
		return notificationsdomain.Envelope{}, err
	}

	state := b.ensureUser(uid)
	state.mu.Lock()
	defer state.mu.Unlock()

	state.seq++
	env := notificationsdomain.Envelope{Seq: state.seq, Event: e}

	state.buffer = append(state.buffer, env)
	if len(state.buffer) > replayBufferSize {
		state.buffer = state.buffer[len(state.buffer)-replayBufferSize:]
	}

	for _, s := range state.subs {
		select {
		case s.ch <- env:
		case <-s.done:
			// subscriber cancelled — skip
		case <-ctx.Done():
			return env, ctx.Err()
		}
	}
	return env, nil
}

// Subscribe registers a subscriber for the user_id read from ctx.
// fromSeq>0 replays buffered envelopes with seq > fromSeq before live;
// ErrSeqTooOld if fromSeq is older than the buffer's oldest entry.
//
// Subscribe 按 ctx 中 user_id 注册订阅者。fromSeq>0 先 replay seq > fromSeq
// 再投递实时;fromSeq 比 buffer 最旧还旧返 ErrSeqTooOld。
func (b *Bridge) Subscribe(ctx context.Context, fromSeq int64) (<-chan notificationsdomain.Envelope, func(), error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("notifications.Bridge.Subscribe: %w", err)
	}

	state := b.ensureUser(uid)
	sub := &subscription{
		ch:   make(chan notificationsdomain.Envelope, subscriberBufferSize),
		done: make(chan struct{}),
	}

	state.mu.Lock()
	if fromSeq > 0 && fromSeq < state.seq {
		if len(state.buffer) > 0 && state.buffer[0].Seq > fromSeq+1 {
			state.mu.Unlock()
			return nil, nil, notificationsdomain.ErrSeqTooOld
		}
		for _, env := range state.buffer {
			if env.Seq > fromSeq {
				// Non-blocking push — channel cap >= replayBufferSize
				// guarantees this fits. Defensive default: if cap math
				// ever drifts, surface as a distinct error (not
				// ErrSeqTooOld which means "evicted from buffer" — wrong
				// semantic for a buffer-overflow situation).
				//
				// 非阻塞 push;cap 保证装得下。防御 default:cap 计算出错
				// 时用独立错误(不是 ErrSeqTooOld,那是被淘汰)。
				select {
				case sub.ch <- env:
				default:
					state.mu.Unlock()
					return nil, nil, fmt.Errorf("notifications: replay overflow (cap=%d)", subscriberBufferSize)
				}
			}
		}
	}
	state.subs = append(state.subs, sub)
	state.mu.Unlock()

	cancel := func() {
		sub.closed.Do(func() { close(sub.done) })
		state.mu.Lock()
		for i, s := range state.subs {
			if s == sub {
				state.subs = append(state.subs[:i], state.subs[i+1:]...)
				break
			}
		}
		state.mu.Unlock()
	}

	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-sub.done:
		}
	}()

	return sub.ch, cancel, nil
}

// Compile-time check.
//
// 编译期检查。
var _ notificationsdomain.Bridge = (*Bridge)(nil)
