// Package skill provides the skill system tools (lazy, surfaced via the catalog overview).
// Five tools: activate (the core action) + get + create/edit/delete (authoring). No search
// tool (catalog overview already exposes every skill) and no execution-query tools (skill
// activation is not a tracked execution).
//
// Package skill 提供 skill system tool（懒加载，经 catalog 概览浮现）。五个：activate（核心）
// + get + create/edit/delete（创作）。无 search 工具（catalog 概览已曝光全部 skill）、无执行
// 查询工具（skill 激活非受追踪的执行）。
package skill

import (
	skillapp "github.com/sunweilin/anselm/backend/internal/app/skill"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
)

// SkillTools constructs the skill system tools over the app service.
//
// SkillTools 在 app service 之上构造 skill system tool。
func SkillTools(svc *skillapp.Service, deps toolapp.DependentCounter) []toolapp.Tool {
	return []toolapp.Tool{
		&ActivateSkill{svc: svc},
		&GetSkill{svc: svc},
		&CreateSkill{svc: svc},
		&EditSkill{svc: svc},
		&DeleteSkill{svc: svc, deps: deps},
	}
}
