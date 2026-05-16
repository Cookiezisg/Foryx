// Package workflow provides system tools for the LLM to interact with the user's workflow library.
//
// Package workflow 提供操作用户 workflow 库的 system tool。
package workflow

import (
	"go.uber.org/zap"

	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	forgepkg "github.com/sunweilin/forgify/backend/internal/pkg/forge"
)

// WorkflowTools constructs workflow system tools; pass a noop forge Publisher to disable double-write in tests.
//
// WorkflowTools 构造 workflow system tool；测试 / 未接线时传 noop forge Publisher 关闭双写。
func WorkflowTools(svc *workflowapp.Service, forge forgepkg.Publisher, log *zap.Logger) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchWorkflow{svc: svc, log: log},
		&GetWorkflow{svc: svc},
		&CreateWorkflow{svc: svc, forge: forge},
		&EditWorkflow{svc: svc, forge: forge},
		&RevertWorkflow{svc: svc, forge: forge},
		&DeleteWorkflow{svc: svc, forge: forge},
	}
}

// WorkflowExecutionTools constructs execution-log tools wired with the flowrun Repository.
//
// WorkflowExecutionTools 用 flowrun Repository 装配执行日志工具。
func WorkflowExecutionTools(repo flowrundomain.Repository) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchWorkflowExecutions{repo: repo},
		&GetWorkflowExecution{repo: repo},
	}
}
