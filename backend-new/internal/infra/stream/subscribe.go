package stream

import (
	"context"
	"fmt"

	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// subscriberHeadroom is the extra channel capacity above bufSize, so a full replay
// burst plus a few live frames fit without blocking the registration path.
//
// subscriberHeadroom 是 channel 在 bufSize 之上的余量，让一次满 replay 加几条实时帧
// 不阻塞注册路径。
const subscriberHeadroom = 256

// Subscribe registers a per-workspace subscriber; fromSeq>0 replays buffered durable
// envelopes (Seq>fromSeq) first, then live. The channel is not closed by the bus; cancel
// is idempotent. If fromSeq has been evicted from the ring → ErrSeqTooOld.
//
// Subscribe 按 ctx workspace 注册订阅者；fromSeq>0 先 replay 环内 Seq>fromSeq 的 durable
// 帧再上实时。channel 不由 bus 关；cancel 幂等。fromSeq 已被淘汰 → ErrSeqTooOld。
func (b *Bus) Subscribe(ctx context.Context, fromSeq int64) (<-chan streamdomain.Envelope, func(), error) {
	wsID, err := reqctxpkg.RequireWorkspaceID(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("stream.Bus.Subscribe: %w", err)
	}

	st := b.ensureWorkspace(wsID)
	sub := &subscription{
		ch:   make(chan streamdomain.Envelope, b.bufSize+subscriberHeadroom),
		done: make(chan struct{}),
	}

	st.mu.Lock()
	if fromSeq > 0 && fromSeq < st.seq {
		// If the oldest buffered seq is already past fromSeq+1, the gap was evicted → refetch.
		// 若环内最旧 seq 已越过 fromSeq+1，缺口被淘汰 → 须 refetch。
		if len(st.buffer) > 0 && st.buffer[0].Seq > fromSeq+1 {
			st.mu.Unlock()
			return nil, nil, streamdomain.ErrSeqTooOld
		}
		for _, env := range st.buffer {
			if env.Seq > fromSeq {
				select {
				case sub.ch <- env:
				default:
					st.mu.Unlock()
					return nil, nil, fmt.Errorf("stream: replay overflow (cap=%d)", cap(sub.ch))
				}
			}
		}
	}
	st.subs = append(st.subs, sub)
	st.mu.Unlock()

	cancel := func() {
		// Close done first so a blocked durable Publish unblocks via <-s.done before we grab mu.
		// 先 close(done) 让阻塞中的 durable Publish 经 <-s.done 解锁，再争 mu。
		sub.closed.Do(func() { close(sub.done) })
		st.mu.Lock()
		for i, s := range st.subs {
			if s == sub {
				st.subs = append(st.subs[:i], st.subs[i+1:]...)
				break
			}
		}
		st.mu.Unlock()
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
