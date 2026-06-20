// Package control provides the LLM system tools for the control-logic library:
// search / get / create / edit / revert / delete. These are lazy tools (Toolset.Lazy)
// — surfaced via search_tools, not resident. There is NO run/executions tool: a control
// logic is evaluated by the workflow durable interpreter, never invoked
// standalone.
//
// Package control 提供操作 control 逻辑库的 LLM system tool：search/get/create/edit/revert/
// delete。懒加载工具（Toolset.Lazy）——经 search_tools 浮现，非常驻。**无 run/executions 工具**：
// control 逻辑由 workflow durable 解释器求值，绝不独立调用。
package control

import (
	controlapp "github.com/sunweilin/anselm/backend/internal/app/control"
	searchapp "github.com/sunweilin/anselm/backend/internal/app/search"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	controldomain "github.com/sunweilin/anselm/backend/internal/domain/control"
)

// ControlTools constructs the control-logic system tools over the app service.
//
// ControlTools 基于 app service 构造 control 逻辑 system tool。
func ControlTools(svc *controlapp.Service, content *searchapp.Service, deps toolapp.DependentCounter) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchControl{svc: svc, content: content},
		&GetControl{svc: svc},
		&CreateControl{svc: svc},
		&EditControl{svc: svc},
		&RevertControl{svc: svc},
		&DeleteControl{svc: svc, deps: deps},
	}
}

// branchArg is the JSON shape of one routing branch in create/edit tool args.
//
// branchArg 是 create/edit 工具参数里一条路由分支的 JSON 形状。
type branchArg struct {
	Port string            `json:"port"`
	When string            `json:"when"`
	Emit map[string]string `json:"emit"`
}

func toBranches(in []branchArg) []controldomain.Branch {
	out := make([]controldomain.Branch, len(in))
	for i, b := range in {
		out[i] = controldomain.Branch{Port: b.Port, When: b.When, Emit: b.Emit}
	}
	return out
}
