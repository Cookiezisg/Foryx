package bootstrap

import (
	"context"

	schedulerapp "github.com/sunweilin/forgify/backend/internal/app/scheduler"
)

// runnerAdapter bridges workflowapp.Runner (primitive params, defined in the workflow package so it
// never imports the scheduler) onto the durable scheduler. StartRun maps (workflowID, payload) →
// schedulerapp.StartInput; KillWorkflow / CountRunning pass straight through. This is the D1
// execution-lifecycle wiring that lets the workflow service drive trigger / kill / drain-count.
//
// runnerAdapter 把 workflowapp.Runner（原生参数，定义在 workflow 包中故绝不 import 调度器）桥到 durable
// 调度器。StartRun 把 (workflowID, payload) 映射成 schedulerapp.StartInput；KillWorkflow / CountRunning 直通。
// 这是 D1 执行生命周期接线，使 workflow service 能驱动 trigger / kill / 排空计数。
type runnerAdapter struct{ sched *schedulerapp.Service }

func (a runnerAdapter) StartRun(ctx context.Context, workflowID string, payload map[string]any) (string, error) {
	return a.sched.StartRun(ctx, schedulerapp.StartInput{WorkflowID: workflowID, Payload: payload})
}

func (a runnerAdapter) KillWorkflow(ctx context.Context, workflowID string) (int, error) {
	return a.sched.KillWorkflow(ctx, workflowID)
}

func (a runnerAdapter) CountRunning(ctx context.Context, workflowID string) (int, error) {
	return a.sched.CountRunning(ctx, workflowID)
}
