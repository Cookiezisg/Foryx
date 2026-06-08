// Package approval provides the LLM system tools for the approval-form library:
// search / get / create / edit / revert / delete. Lazy tools (Toolset.Lazy) surfaced via
// search_tools. No run/executions — an approval form is rendered + parked by the workflow
// durable interpreter (波次 4), never invoked standalone.
//
// Package approval 提供操作审批表库的 LLM system tool：search/get/create/edit/revert/delete。
// 懒加载工具（Toolset.Lazy），经 search_tools 浮现。**无 run/executions**——审批表由 workflow
// durable 解释器（波次 4）渲染 + park，绝不独立调用。
package approval

import (
	"encoding/json"

	approvalapp "github.com/sunweilin/forgify/backend/internal/app/approval"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

// ApprovalTools constructs the approval-form system tools over the app service.
//
// ApprovalTools 基于 app service 构造审批表 system tool。
func ApprovalTools(svc *approvalapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchApproval{svc: svc},
		&GetApproval{svc: svc},
		&CreateApproval{svc: svc},
		&EditApproval{svc: svc},
		&RevertApproval{svc: svc},
		&DeleteApproval{svc: svc},
	}
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
