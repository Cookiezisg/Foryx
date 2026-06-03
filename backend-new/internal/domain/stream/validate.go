package stream

import (
	"fmt"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// ErrInvalidEvent marks a malformed event — a producer bug, surfaced at Publish so it
// fails at the boundary instead of on the wire. KindInternal (500) is the right class
// if it ever does escape: a producer emitting a malformed event is a server-side bug.
//
// ErrInvalidEvent 标记形状错误事件——producer bug，在 Publish 暴露，让它在边界失败而非
// 流到线缆。万一真的逃逸，KindInternal(500) 是对的类：producer 发畸形事件是服务端 bug。
var ErrInvalidEvent = errorsdomain.New(errorsdomain.KindInternal, "STREAM_INVALID_EVENT",
	"stream: invalid event")

// ValidateEvent runs the protocol's universal shape invariants: valid scope kind,
// node ID present, and frame-internal consistency (a node must carry a Type; a close
// must carry a terminal status). domain does NOT inspect Node.Content — that is the
// producing module's concern (#6 反校验剧场).
//
// ValidateEvent 跑协议通用形状不变量：scope kind 合法、节点 ID 非空、frame 内部一致
// （node 必须有 Type；close 必须有终态）。domain **不**检查 Node.Content——那是
// producer 模块的事（#6 反校验剧场）。
func ValidateEvent(e Event) error {
	if !IsValidKind(e.Scope.Kind) {
		return fmt.Errorf("%w: invalid scope kind %q", ErrInvalidEvent, e.Scope.Kind)
	}
	if e.ID == "" {
		return fmt.Errorf("%w: empty node ID", ErrInvalidEvent)
	}
	if e.Frame == nil {
		return fmt.Errorf("%w: nil frame", ErrInvalidEvent)
	}
	switch f := e.Frame.(type) {
	case Open:
		if f.Node.Type == "" {
			return fmt.Errorf("%w: open frame with empty node type", ErrInvalidEvent)
		}
	case Delta:
		// An empty chunk is a harmless no-op; nothing to check.
		// 空 chunk 是无害 no-op，无需校验。
	case Close:
		if !IsValidStatus(f.Status) {
			return fmt.Errorf("%w: close frame with invalid status %q", ErrInvalidEvent, f.Status)
		}
		if f.Result != nil && f.Result.Type == "" {
			return fmt.Errorf("%w: close frame result with empty node type", ErrInvalidEvent)
		}
	case Signal:
		if f.Node.Type == "" {
			return fmt.Errorf("%w: signal frame with empty node type", ErrInvalidEvent)
		}
	default:
		return fmt.Errorf("%w: unknown frame type %T", ErrInvalidEvent, e.Frame)
	}
	return nil
}
