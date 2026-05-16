package scheduler

import (
	"context"
	"errors"
	"time"

	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

var defaultTimeouts = map[string]time.Duration{
	workflowdomain.NodeTypeFunction: 30 * time.Second,
	workflowdomain.NodeTypeHandler:  30 * time.Second,
	workflowdomain.NodeTypeMCP:      30 * time.Second,
	workflowdomain.NodeTypeSkill:    60 * time.Second,
	workflowdomain.NodeTypeLLM:      60 * time.Second,
	workflowdomain.NodeTypeHTTP:     30 * time.Second,
	workflowdomain.NodeTypeApproval: 7 * 24 * time.Hour,
}

func nodeTimeoutDuration(node workflowdomain.NodeSpec) time.Duration {
	if node.Timeout > 0 {
		return time.Duration(node.Timeout) * time.Millisecond
	}
	return defaultTimeouts[node.Type]
}

type retryAttemptFn func(ctx context.Context) DispatchOutput

// withRetry runs fn under NodeSpec.Retry; success or a fatal sentinel returns immediately.
//
// withRetry 按 NodeSpec.Retry 跑 fn；成功或 fatal sentinel 立即返回。
func withRetry(ctx context.Context, node workflowdomain.NodeSpec, execCtx *ExecutionContext, fn retryAttemptFn) DispatchOutput {
	retry := node.Retry
	maxAttempts := 1
	delay := time.Duration(0)
	backoff := ""
	if retry != nil {
		if retry.MaxAttempts > 1 {
			maxAttempts = retry.MaxAttempts
		}
		if retry.DelayMs > 0 {
			delay = time.Duration(retry.DelayMs) * time.Millisecond
		}
		backoff = retry.Backoff
	}

	var lastOut DispatchOutput
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return DispatchOutput{Error: err}
		}
		execCtx.Attempts[node.ID] = attempt
		lastOut = fn(ctx)
		if lastOut.Error == nil {
			return lastOut
		}
		if isFatalErr(lastOut.Error) {
			return lastOut
		}
		if attempt == maxAttempts {
			return lastOut
		}
		select {
		case <-ctx.Done():
			return DispatchOutput{Error: ctx.Err()}
		case <-time.After(delay):
		}
		delay = nextDelay(backoff, delay, retry)
	}
	return lastOut
}

func nextDelay(strategy string, current time.Duration, retry *workflowdomain.RetryConfig) time.Duration {
	switch strategy {
	case "exponential":
		if current <= 0 {
			return time.Second
		}
		return current * 2
	case "linear":
		if retry != nil && retry.DelayMs > 0 {
			return current + time.Duration(retry.DelayMs)*time.Millisecond
		}
		return current
	default:
		return current
	}
}

func isFatalErr(err error) bool {
	return errors.Is(err, ErrApprovalRequired) ||
		errors.Is(err, ErrLoopBodyNotSupported) ||
		errors.Is(err, ErrParallelBranchNotSupported)
}

func (s *Service) dispatchWithPolicies(ctx context.Context, node workflowdomain.NodeSpec, input map[string]any, execCtx *ExecutionContext) DispatchOutput {
	timeout := nodeTimeoutDuration(node)
	return withRetry(ctx, node, execCtx, func(rctx context.Context) DispatchOutput {
		callCtx := rctx
		if timeout > 0 {
			tctx, cancel := context.WithTimeout(rctx, timeout)
			defer cancel()
			callCtx = tctx
		}
		out := s.router.Dispatch(callCtx, DispatchInput{
			Node:    node,
			NodeIn:  input,
			ExecCtx: execCtx,
		})
		if out.Error == nil && callCtx.Err() == context.DeadlineExceeded {
			out.Error = context.DeadlineExceeded
		}
		return out
	})
}
