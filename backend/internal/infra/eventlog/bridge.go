// Package eventlog is the in-process Bridge for the recursive event-log protocol (per-user keyed).
//
// Package eventlog 是递归事件日志协议的进程内 Bridge（按 user_id 分流）。
package eventlog

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const (
	replayBufferSize     = 4096
	subscriberBufferSize = 4352 // = replayBufferSize + 256 live headroom
)

// Bridge is a thread-safe in-process eventlog dispatcher keyed by user_id.
//
// Bridge 是线程安全的进程内 eventlog 分发器，按 user_id 分流。
type Bridge struct {
	mu    sync.Mutex
	users map[string]*userState
}

type userState struct {
	mu     sync.Mutex
	seq    int64
	buffer []eventlogdomain.Envelope
	subs   []*subscription
}

type subscription struct {
	ch     chan eventlogdomain.Envelope
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
func (b *Bridge) Publish(ctx context.Context, e eventlogdomain.Event) (eventlogdomain.Envelope, error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return eventlogdomain.Envelope{}, fmt.Errorf("eventlog.Bridge.Publish: %w", err)
	}
	if err := eventlogdomain.ValidateEvent(e); err != nil {
		return eventlogdomain.Envelope{}, err
	}

	state := b.ensureUser(uid)
	state.mu.Lock()
	defer state.mu.Unlock()

	state.seq++
	env := eventlogdomain.Envelope{Seq: state.seq, Event: e}

	state.buffer = append(state.buffer, env)
	if len(state.buffer) > replayBufferSize {
		state.buffer = state.buffer[len(state.buffer)-replayBufferSize:]
	}

	// Fan out under state.mu so seq order matches delivery order.
	// 在 state.mu 内扇出，保证 seq 顺序与投递顺序一致。
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
func (b *Bridge) Subscribe(ctx context.Context, fromSeq int64) (<-chan eventlogdomain.Envelope, func(), error) {
	uid, err := reqctxpkg.RequireUserID(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("eventlog.Bridge.Subscribe: %w", err)
	}

	state := b.ensureUser(uid)
	sub := &subscription{
		ch:   make(chan eventlogdomain.Envelope, subscriberBufferSize),
		done: make(chan struct{}),
	}

	state.mu.Lock()
	if fromSeq > 0 && fromSeq < state.seq {
		// fromSeq evicted from buffer → caller must refetch.
		// fromSeq 已被 buffer 淘汰 → 调用方需 refetch。
		if len(state.buffer) > 0 && state.buffer[0].Seq > fromSeq+1 {
			state.mu.Unlock()
			return nil, nil, eventlogdomain.ErrSeqTooOld
		}
		for _, env := range state.buffer {
			if env.Seq > fromSeq {
				select {
				case sub.ch <- env:
				default:
					state.mu.Unlock()
					return nil, nil, fmt.Errorf("eventlog: replay overflow (cap=%d)", subscriberBufferSize)
				}
			}
		}
	}

	state.subs = append(state.subs, sub)
	state.mu.Unlock()

	cancel := func() {
		sub.closed.Do(func() {
			close(sub.done)
		})
		// Close done first so blocked Publish unblocks via <-s.done before we grab mu.
		// 先 close(done) 让阻塞中的 Publish 通过 <-s.done 解锁，再争 mu。
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

var _ eventlogdomain.Bridge = (*Bridge)(nil)
