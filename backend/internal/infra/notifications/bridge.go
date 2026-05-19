// Package notifications is the in-process Bridge for entity-update events (per-user keyed).
//
// Package notifications 是 entity 更新事件的进程内 Bridge（按 user_id 分流）。
package notifications

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	notificationsdomain "github.com/sunweilin/forgify/backend/internal/domain/notifications"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const (
	replayBufferSize     = 1024
	subscriberBufferSize = 1280
)

// Bridge is the thread-safe in-process notification dispatcher keyed by user_id.
//
// Bridge 是线程安全的进程内通知分发器，按 user_id 分流。
type Bridge struct {
	mu    sync.Mutex
	users map[string]*userState
}

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

// NewBridge constructs an empty Bridge; log is unused (synchronous primitive per §S10).
//
// NewBridge 构造空 Bridge；log 暂未使用（§S10 同步原语不自打日志）。
func NewBridge(_ *zap.Logger) *Bridge {
	return &Bridge{
		users: make(map[string]*userState),
	}
}

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

// Publish assigns per-user seq, buffers for replay, fans out; blocks on slow subscriber.
//
// Publish 分配 per-user seq、入 replay buffer、扇出订阅者；订阅者满时阻塞。
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
		case <-ctx.Done():
			return env, ctx.Err()
		}
	}
	return env, nil
}

// Subscribe registers a subscriber for ctx's user_id; fromSeq>0 replays buffered envelopes first.
//
// Subscribe 按 ctx 的 user_id 注册订阅者；fromSeq>0 先 replay 历史再上实时。
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

// List returns up to limit envelopes with Seq > fromSeq from the user's replay buffer.
//
// List 从 replay buffer 返最多 limit 条 Seq > fromSeq 的通知。
func (b *Bridge) List(ctx context.Context, fromSeq int64, limit int) ([]notificationsdomain.Envelope, bool, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("notifications.Bridge.List: %w", err)
	}

	b.mu.Lock()
	state, ok := b.users[uid]
	b.mu.Unlock()
	if !ok {
		return nil, false, nil
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	var matched []notificationsdomain.Envelope
	for _, env := range state.buffer {
		if env.Seq > fromSeq {
			matched = append(matched, env)
		}
	}
	if limit <= 0 || len(matched) <= limit {
		return matched, false, nil
	}
	return matched[:limit], true, nil
}

var _ notificationsdomain.Bridge = (*Bridge)(nil)
