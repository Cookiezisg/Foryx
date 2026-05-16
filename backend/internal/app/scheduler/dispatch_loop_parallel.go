package scheduler

import (
	"context"
	"errors"
	"fmt"
)

// ErrLoopBodyNotSupported is returned when loop.config.body is non-empty.
//
// ErrLoopBodyNotSupported 在 loop.config.body 非空时返回。
var ErrLoopBodyNotSupported = errors.New("scheduler: loop body subgraph not supported in V1")

// LoopDispatcher iterates over config.items and emits them on "out".
//
// LoopDispatcher 遍历 config.items 并挂 "out" 输出。
type LoopDispatcher struct{}

// NewLoopDispatcher constructs LoopDispatcher.
//
// NewLoopDispatcher 构造 LoopDispatcher。
func NewLoopDispatcher() *LoopDispatcher { return &LoopDispatcher{} }

// Dispatch reads config.items; errors on non-empty config.body.
//
// Dispatch 读 config.items；config.body 非空时返错。
func (d *LoopDispatcher) Dispatch(_ context.Context, in DispatchInput) DispatchOutput {
	if body, ok := in.Node.Config["body"]; ok && body != nil {
		if arr, isArr := body.([]any); isArr && len(arr) > 0 {
			return DispatchOutput{
				Error: fmt.Errorf("loop node %q: %w", in.Node.ID, ErrLoopBodyNotSupported),
			}
		}
	}
	items, _ := in.Node.Config["items"].([]any)
	return DispatchOutput{
		Outputs: map[string]any{"out": items, "count": len(items)},
	}
}

// ErrParallelBranchNotSupported is returned when parallel.config.branches is non-empty.
//
// ErrParallelBranchNotSupported 在 parallel.config.branches 非空时返回。
var ErrParallelBranchNotSupported = errors.New("scheduler: parallel branch subgraph not supported in V1")

// ParallelDispatcher is a pass-through; natural parallel edges run concurrently in executeRun.
//
// ParallelDispatcher 是 pass-through；天然并行边由 executeRun 并发跑。
type ParallelDispatcher struct{}

// NewParallelDispatcher constructs ParallelDispatcher.
//
// NewParallelDispatcher 构造 ParallelDispatcher。
func NewParallelDispatcher() *ParallelDispatcher { return &ParallelDispatcher{} }

// Dispatch passes through; errors on non-empty config.branches.
//
// Dispatch pass-through；branches 非空时返错。
func (d *ParallelDispatcher) Dispatch(_ context.Context, in DispatchInput) DispatchOutput {
	if branches, ok := in.Node.Config["branches"]; ok && branches != nil {
		if arr, isArr := branches.([]any); isArr && len(arr) > 0 {
			return DispatchOutput{
				Error: fmt.Errorf("parallel node %q: %w", in.Node.ID, ErrParallelBranchNotSupported),
			}
		}
	}
	return DispatchOutput{Outputs: map[string]any{"out": "passthrough"}}
}
