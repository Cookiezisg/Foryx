package scheduler

import (
	"context"
	"errors"
	"fmt"
)

// ErrApprovalRequired signals the run reached an approval gate; executeRun pauses on it.
//
// ErrApprovalRequired 表示 run 抵达 approval 关卡，executeRun 据此暂停。
var ErrApprovalRequired = errors.New("scheduler: approval required")

// ApprovalDispatcher emits ErrApprovalRequired with prompt context.
//
// ApprovalDispatcher 返 ErrApprovalRequired 并附 prompt 信息。
type ApprovalDispatcher struct{}

// NewApprovalDispatcher constructs ApprovalDispatcher.
//
// NewApprovalDispatcher 构造 ApprovalDispatcher。
func NewApprovalDispatcher() *ApprovalDispatcher { return &ApprovalDispatcher{} }

// Dispatch reads prompt + timeout from node.Config and returns ErrApprovalRequired.
//
// Dispatch 读 prompt + timeout，返 ErrApprovalRequired。
func (d *ApprovalDispatcher) Dispatch(_ context.Context, in DispatchInput) DispatchOutput {
	prompt, _ := in.Node.Config["prompt"].(string)
	if prompt == "" {
		prompt = "Approval required"
	}
	return DispatchOutput{
		Error: fmt.Errorf("%w: node %q: %s", ErrApprovalRequired, in.Node.ID, prompt),
	}
}
