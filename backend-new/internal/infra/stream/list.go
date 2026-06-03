package stream

import (
	"context"
	"fmt"

	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// List returns up to limit durable envelopes with Seq > fromSeq for the ctx workspace;
// bool = hasMore. It reads the in-memory replay ring (the notifications stream has no DB
// persistence, so this is its only snapshot source). Ephemeral frames are absent by
// construction — they never enter the ring.
//
// List 返回 ctx workspace 下最多 limit 条 Seq > fromSeq 的 durable Envelope；bool =
// hasMore。读内存 replay 环（notifications 无 DB 落盘，这是其唯一快照源）。ephemeral 帧
// 天然不在环内。
func (b *Bus) List(ctx context.Context, fromSeq int64, limit int) ([]streamdomain.Envelope, bool, error) {
	wsID, err := reqctxpkg.RequireWorkspaceID(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("stream.Bus.List: %w", err)
	}

	b.mu.Lock()
	st, ok := b.spaces[wsID]
	b.mu.Unlock()
	if !ok {
		return nil, false, nil
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	var matched []streamdomain.Envelope
	for _, env := range st.buffer {
		if env.Seq > fromSeq {
			matched = append(matched, env)
		}
	}
	if limit <= 0 || len(matched) <= limit {
		return matched, false, nil
	}
	return matched[:limit], true, nil
}
