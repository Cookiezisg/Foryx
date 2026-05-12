// Package eventlog provides the in-process Bridge implementation for
// the recursive event-log protocol. Per-user monotonic seq, replay
// buffer for Last-Event-ID reconnect, block-on-slow-subscriber
// semantic (delta events are append-only — losing them corrupts the
// wire stream).
//
// Per D-redo-2 (forge_redesign 2026-05-12) keying is per-user: each
// user owns one wire stream covering ALL their conversations; clients
// read payload.conversationId to dispatch into the right UI panel.
// Single backing implementation: no redis / disk variant on the
// roadmap for this single-user local desktop app.
//
// Package eventlog 提供递归事件日志协议的进程内 Bridge。
// per-user 单调 seq、Last-Event-ID 重连用 replay buffer、慢订阅者阻塞
// publisher 语义(delta 事件 append-only)。
//
// 按 D-redo-2(forge_redesign 2026-05-12),Bridge 按 user_id key:每用户
// 一条流覆盖全部对话事件,客户端按 payload.conversationId 分派 panel。
package eventlog

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Tunables. Sized for single-user desktop load. The per-user stream
// covers all of that user's active conversations so the buffer needs
// some headroom over the per-conversation pre-D-redo-2 sizing.
//
// 调参,按单用户桌面负载。per-user 流覆盖全部对话所以 buffer 比 pre-D-redo-2
// per-conversation 时段大一点。
const (
	replayBufferSize     = 4096
	subscriberBufferSize = 4352 // = replayBufferSize + 256 live headroom
)

// Bridge is a thread-safe, in-process eventlog dispatcher keyed by user_id.
//
// Bridge 是线程安全的进程内 eventlog 分发器,按 user_id key。
type Bridge struct {
	mu    sync.Mutex
	users map[string]*userState
}

// userState holds per-user seq counter + replay buffer + subs.
// All fields guarded by mu.
//
// userState 持有 per-user 的 seq 计数器 + replay buffer + sub。
// 所有字段由 mu 守护。
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

// NewBridge constructs an empty Bridge. The log parameter is accepted
// for API symmetry with notifications.NewBridge / future variants but
// is currently unused — the bridge follows §S10's "synchronous primitive"
// rule (don't self-log; let callers decide).
//
// NewBridge 构造空 Bridge。log 参数为 API 对称保留,目前未用——bridge 按
// §S10 同步原语原则不自打日志。
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

// Publish validates, assigns seq (per-user monotonic), appends to replay
// buffer, and fans out to that user's subscribers. Blocks if any
// subscriber buffer is full (intentional — delta events must not be
// lost). Returns ctx.Err if ctx cancelled mid-fanout (the event is
// still recorded in the replay buffer so future subscribers can pick
// it up).
//
// Publish 校验、分配 per-user 单调 seq、追加 replay buffer、扇出给该用户的
// 订阅者。订阅者 buffer 满时阻塞(故意——delta 不能丢)。扇出途中 ctx 取消
// 返 ctx.Err(事件已进 replay buffer,未来订阅者仍能取)。
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

	// Append to replay buffer; trim oldest when full.
	// 追加到 replay buffer;满时丢最旧。
	state.buffer = append(state.buffer, env)
	if len(state.buffer) > replayBufferSize {
		state.buffer = state.buffer[len(state.buffer)-replayBufferSize:]
	}

	// Fan out under state.mu so seq order matches send order.
	// 在 state.mu 下扇出,保证 seq 顺序与 send 顺序一致。
	for _, s := range state.subs {
		select {
		case s.ch <- env:
			// delivered / 已投递
		case <-s.done:
			// subscriber cancelled — skip without blocking
			// 订阅者已取消——跳过不阻塞
		case <-ctx.Done():
			return env, ctx.Err()
		}
	}
	return env, nil
}

// Subscribe registers a subscriber for the user_id read from ctx.
// fromSeq>0 replays buffered envelopes with seq > fromSeq before live
// delivery; returns ErrSeqTooOld if fromSeq is older than the buffer's
// oldest entry.
//
// Subscribe 按 ctx 中 user_id 注册订阅者。fromSeq>0 先 replay seq > fromSeq
// 的 buffer 项再投递实时;fromSeq 比 buffer 最旧还旧返 ErrSeqTooOld。
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
	// Replay logic: only when caller wants resume (fromSeq>0).
	// fromSeq=0 means "live only, no history". fromSeq>=current means
	// "I already have everything", no replay needed.
	//
	// Replay 逻辑:仅当调用方要 resume(fromSeq>0)。
	// fromSeq=0 = 只要实时不要历史。fromSeq>=current = 我都有了,无 replay。
	if fromSeq > 0 && fromSeq < state.seq {
		// Check if fromSeq has been evicted: oldest buffer entry > fromSeq+1
		// means events fromSeq+1..oldest-1 are gone.
		//
		// 检查 fromSeq 是否被淘汰:最旧 buffer 项 > fromSeq+1 表示
		// fromSeq+1..oldest-1 段已丢。
		if len(state.buffer) > 0 && state.buffer[0].Seq > fromSeq+1 {
			state.mu.Unlock()
			return nil, nil, eventlogdomain.ErrSeqTooOld
		}
		for _, env := range state.buffer {
			if env.Seq > fromSeq {
				// Non-blocking push — channel cap >= replayBufferSize
				// guarantees this fits.
				// 非阻塞 push——channel cap >= replayBufferSize 保证装得下。
				select {
				case sub.ch <- env:
				default:
					// Defensive: should be unreachable given cap.
					// 防御:cap 保证不该到这里。
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
		// Remove from state.subs (separate from close(done) so Publish
		// can unblock via <-s.done before we wait for state.mu).
		//
		// 从 state.subs 移除(与 close(done) 分开,让 Publish 通过
		// <-s.done 解阻塞,再等 state.mu)。
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

// Compile-time check that *Bridge satisfies eventlogdomain.Bridge.
//
// 编译期确认 *Bridge 满足 eventlogdomain.Bridge。
var _ eventlogdomain.Bridge = (*Bridge)(nil)
