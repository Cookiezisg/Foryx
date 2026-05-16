// Package skill hosts the LLM-facing system tools for Anthropic Agent Skills (search + activate).
//
// Package skill 提供 Anthropic Agent Skills 的 LLM-facing 系统工具（search + activate）。
package skill

import (
	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
)

// SkillTools returns SearchSkills + ActivateSkill wired against svc.
//
// SkillTools 返回接到 svc 的 SearchSkills + ActivateSkill。
func SkillTools(svc *skillapp.Service) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchSkills{svc: svc},
		&ActivateSkill{svc: svc},
	}
}

// SkillExecutionTools constructs the skill execution-log tools wired with the Repository.
//
// SkillExecutionTools 装配 skill 执行日志工具。
func SkillExecutionTools(repo skilldomain.ExecutionRepository) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchSkillExecutions{repo: repo},
		&GetSkillExecution{repo: repo},
	}
}
