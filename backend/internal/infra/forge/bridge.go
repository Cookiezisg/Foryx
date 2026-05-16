// Package forge is the in-process Bridge for the trinity-forging SSE protocol (per-user keyed).
//
// Package forge 是 trinity 锻造 SSE 协议的进程内 Bridge（按 user_id 分流）。
package forge

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const (
	replayBufferSize     = 1024
	subscriberBufferSize = 1280 // = replayBufferSize + 256 live headroom
)

// Bridge is the thread-safe in-process forge dispatcher.
//
// Bridge 是线程安全的进程内 forge 分发器。
type Bridge struct {
	mu    sync.Mutex
	users map[string]*userState
}

type userState struct {
	mu     sync.Mutex
	seq    int64
	buffer []forgedomain.Envelope
	subs   []*subscription
}

type subscription struct {
	ch     chan forgedomain.Envelope
	done   chan struct{}
	closed sync.Once
}

// NewBridge constructs an empty Bridge; log is unused (synchronous primitive per §S10).
//
// NewBridge 构造空 Bridge；log 暂未使用（§S10 同步原语不自打日志）。
func NewBridge(_ *zap.Logger) *Bridge {
	return &Bridge{users: make(map[string]*userState)}
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
func (b *Bridge) Publish(ctx context.Context, e forgedomain.Event) (forgedomain.Envelope, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return forgedomain.Envelope{}, fmt.Errorf("forge.Bridge.Publish: %w", err)
	}
	if err := forgedomain.ValidateEvent(e); err != nil {
		return forgedomain.Envelope{}, err
	}

	state := b.ensureUser(uid)
	state.mu.Lock()
	defer state.mu.Unlock()

	state.seq++
	env := forgedomain.Envelope{Seq: state.seq, Event: e}

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
func (b *Bridge) Subscribe(ctx context.Context, fromSeq int64) (<-chan forgedomain.Envelope, func(), error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("forge.Bridge.Subscribe: %w", err)
	}

	state := b.ensureUser(uid)
	sub := &subscription{
		ch:   make(chan forgedomain.Envelope, subscriberBufferSize),
		done: make(chan struct{}),
	}

	state.mu.Lock()
	if fromSeq > 0 && fromSeq < state.seq {
		if len(state.buffer) > 0 && state.buffer[0].Seq > fromSeq+1 {
			state.mu.Unlock()
			return nil, nil, forgedomain.ErrSeqTooOld
		}
		for _, env := range state.buffer {
			if env.Seq > fromSeq {
				select {
				case sub.ch <- env:
				default:
					state.mu.Unlock()
					return nil, nil, fmt.Errorf("forge: replay overflow (cap=%d)", subscriberBufferSize)
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

var _ forgedomain.Bridge = (*Bridge)(nil)
