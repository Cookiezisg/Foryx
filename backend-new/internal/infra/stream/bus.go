// Package stream is the in-process Bus backing the three SSE streams (messages /
// entities / notifications). One Bus type, instantiated once per stream; each keeps its
// own per-workspace seq + replay ring. Frame durability decides buffering: durable
// frames (open/close, non-ephemeral signals) get a seq and enter the ring for replay;
// ephemeral frames (delta, tick) fan out live with seq 0, are never buffered, and are
// dropped on a full subscriber — so token-rate deltas never overflow the replay window
// nor stall the producer. v1 keys by workspace only (全量推); per-scope subscription is
// a future extension. See lab/backendcleaner/target/stream-protocol.md §5.
//
// Package stream 是支撑三条 SSE 流的进程内 Bus。一个 Bus 类型、每条流实例化一次；各自
// 持有 per-workspace seq + replay 环。可丢性由 frame 决定：durable 帧（open/close、非
// ephemeral signal）分配 seq 并入环供 replay；ephemeral 帧（delta、tick）实时扇出、seq 0、
// 不入环、满则丢——token 级 delta 永不撑爆 replay 窗口、永不卡 producer。v1 仅按 workspace
// 分流（全量推），scope 级订阅是未来扩展。见 stream-protocol.md §5。
package stream

import (
	"context"
	"fmt"
	"sync"

	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Bus dispatches one stream's events per workspace; safe for concurrent Publish + Subscribe.
//
// Bus 按 workspace 分发单条流的事件；支持并发 Publish + Subscribe。
type Bus struct {
	mu      sync.Mutex
	spaces  map[string]*workspaceState
	bufSize int
}

type workspaceState struct {
	mu     sync.Mutex
	seq    int64
	buffer []streamdomain.Envelope
	subs   []*subscription
}

type subscription struct {
	ch     chan streamdomain.Envelope
	done   chan struct{}
	closed sync.Once
}

// New constructs an empty Bus whose replay ring holds the last bufSize durable frames.
//
// New 构造空 Bus，replay 环保留最近 bufSize 条 durable 帧。
func New(bufSize int) *Bus {
	return &Bus{spaces: make(map[string]*workspaceState), bufSize: bufSize}
}

func (b *Bus) ensureWorkspace(id string) *workspaceState {
	b.mu.Lock()
	defer b.mu.Unlock()
	st, ok := b.spaces[id]
	if !ok {
		st = &workspaceState{}
		b.spaces[id] = st
	}
	return st
}

// Publish validates e, then dispatches by frame durability. Durable: monotonic seq +
// replay ring + blocking ordered fan-out (never dropped). Ephemeral: seq 0, skips the
// ring, dropped on a full subscriber (never blocks the producer). Reads workspace_id from ctx.
//
// Publish 校验 e 后按 frame 可丢性分发。durable：单调 seq + replay 环 + 锁内保序阻塞扇出
// （不丢）。ephemeral：seq 0、不入环、满则丢（不卡 producer）。workspace_id 从 ctx 读。
func (b *Bus) Publish(ctx context.Context, e streamdomain.Event) (streamdomain.Envelope, error) {
	wsID, err := reqctxpkg.RequireWorkspaceID(ctx)
	if err != nil {
		return streamdomain.Envelope{}, fmt.Errorf("stream.Bus.Publish: %w", err)
	}
	if err := streamdomain.ValidateEvent(e); err != nil {
		return streamdomain.Envelope{}, err
	}

	st := b.ensureWorkspace(wsID)
	st.mu.Lock()
	defer st.mu.Unlock()

	if !e.Frame.Durable() {
		// Ephemeral: live-only, seq 0, no ring, drop on full — must never stall the producer.
		// ephemeral：实时、seq 0、不入环、满则丢——绝不卡 producer。
		env := streamdomain.Envelope{Seq: 0, Event: e}
		for _, s := range st.subs {
			select {
			case s.ch <- env:
			default:
			}
		}
		return env, nil
	}

	// Durable: assign seq, ring-buffer, fan out under the lock so seq order == delivery order.
	// durable：分配 seq、入环、锁内扇出保证 seq 顺序 == 投递顺序。
	st.seq++
	env := streamdomain.Envelope{Seq: st.seq, Event: e}
	st.buffer = append(st.buffer, env)
	if len(st.buffer) > b.bufSize {
		st.buffer = st.buffer[len(st.buffer)-b.bufSize:]
	}
	for _, s := range st.subs {
		select {
		case s.ch <- env:
		case <-s.done:
		case <-ctx.Done():
			return env, ctx.Err()
		}
	}
	return env, nil
}

// Bus implements ListReader (and therefore Bridge); the consumer is wired with whichever fits.
//
// Bus 实现 ListReader（因而也实现 Bridge）；消费方按所需接口接线。
var _ streamdomain.ListReader = (*Bus)(nil)
