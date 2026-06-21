// Package agent provides the LLM system tools for the user's agent library:
// search / get / create / edit / revert / delete / invoke + execution-log inspection.
// These are lazy tools (Toolset.Lazy) — surfaced via search_tools, not resident. There is no
// accept tool: create/edit take effect immediately (no pending/accept), same as function.
//
// Package agent 提供操作用户 agent 库的 LLM system tool：search / get / create / edit / revert /
// delete / invoke + execution-log 查看。懒加载工具（Toolset.Lazy）——经 search_tools 浮现、非常驻。
// 无 accept 工具：create/edit 立即生效（无 pending/accept），同 function。
package agent

import (
	agentapp "github.com/sunweilin/anselm/backend/internal/app/agent"
	searchapp "github.com/sunweilin/anselm/backend/internal/app/search"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
)

// AgentTools constructs the agent system tools over the app service.
//
// AgentTools 基于 app service 构造 agent system tool。
func AgentTools(svc *agentapp.Service, content *searchapp.Service, deps toolapp.DependentCounter) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchAgent{svc: svc, content: content},
		&GetAgent{svc: svc},
		&CreateAgent{svc: svc},
		&EditAgent{svc: svc},
		&RevertAgent{svc: svc},
		&DeleteAgent{svc: svc, deps: deps},
		&UpdateAgentMeta{svc: svc},
		&InvokeAgent{svc: svc},
		&SearchAgentExecutions{svc: svc},
		&GetAgentExecution{svc: svc},
	}
}
