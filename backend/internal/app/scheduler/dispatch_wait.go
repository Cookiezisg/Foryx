package scheduler

import (
	"context"
	"fmt"
	"time"
)

// WaitDispatcher sleeps until duration/until elapses or ctx is cancelled.
//
// WaitDispatcher 睡到 duration/until 到点或 ctx 取消。
type WaitDispatcher struct{}

// NewWaitDispatcher constructs WaitDispatcher.
//
// NewWaitDispatcher 构造 WaitDispatcher。
func NewWaitDispatcher() *WaitDispatcher { return &WaitDispatcher{} }

// Dispatch sleeps for the configured duration or until.
//
// Dispatch 按 duration 或 until 睡眠。
func (d *WaitDispatcher) Dispatch(ctx context.Context, in DispatchInput) DispatchOutput {
	var sleep time.Duration

	if durMs, ok := configInt(in.Node.Config["duration"]); ok && durMs > 0 {
		sleep = time.Duration(durMs) * time.Millisecond
	} else if untilStr, ok := in.Node.Config["until"].(string); ok && untilStr != "" {
		until, err := time.Parse(time.RFC3339, untilStr)
		if err != nil {
			return DispatchOutput{Error: fmt.Errorf("wait node %q: parse until: %w", in.Node.ID, err)}
		}
		sleep = time.Until(until)
	} else {
		return DispatchOutput{Error: fmt.Errorf("wait node %q: duration or until required", in.Node.ID)}
	}

	if sleep <= 0 {
		return DispatchOutput{Outputs: map[string]any{"out": "already past"}}
	}

	t := time.NewTimer(sleep)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return DispatchOutput{Error: ctx.Err()}
	case <-t.C:
		return DispatchOutput{Outputs: map[string]any{"out": "elapsed"}}
	}
}

func configInt(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	}
	return 0, false
}
