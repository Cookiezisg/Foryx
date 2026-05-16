package scheduler

import (
	"context"
)

// TriggerDispatcher passes the run's trigger input through the "out" port.
//
// TriggerDispatcher 把 run 的 trigger input 透传到 "out" port。
type TriggerDispatcher struct{}

// NewTriggerDispatcher constructs the no-op trigger dispatcher.
//
// NewTriggerDispatcher 构造 no-op trigger dispatcher。
func NewTriggerDispatcher() *TriggerDispatcher { return &TriggerDispatcher{} }

// Dispatch returns the run's TriggerInput as the default output port.
//
// Dispatch 把 run.TriggerInput 当默认 out port 返。
func (d *TriggerDispatcher) Dispatch(_ context.Context, in DispatchInput) DispatchOutput {
	return DispatchOutput{
		Outputs: map[string]any{
			"out": in.ExecCtx.Run.TriggerInput,
		},
	}
}
