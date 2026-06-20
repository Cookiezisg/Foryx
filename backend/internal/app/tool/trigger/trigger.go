// Package trigger provides the LLM system tools for the user's trigger library:
// search / get / create / edit / delete / fire + activation-log inspection. These are lazy
// tools (Toolset.Lazy), surfaced via search_tools. A trigger is a signal source (cron /
// webhook / fsnotify / sensor); it fires the workflows that listen to it.
//
// Package trigger 提供操作用户 trigger 库的 LLM system tool（懒加载，经 search_tools 浮现）。
// trigger 是信号源（cron/webhook/fsnotify/sensor），触发监听它的 workflow。
package trigger

import (
	searchapp "github.com/sunweilin/anselm/backend/internal/app/search"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	triggerapp "github.com/sunweilin/anselm/backend/internal/app/trigger"
)

// TriggerTools constructs the trigger system tools over the app service.
//
// TriggerTools 在 app service 之上构造 trigger system tool。
func TriggerTools(svc *triggerapp.Service, content *searchapp.Service, deps toolapp.DependentCounter) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchTriggers{svc: svc, content: content},
		&GetTrigger{svc: svc},
		&CreateTrigger{svc: svc},
		&EditTrigger{svc: svc},
		&DeleteTrigger{svc: svc, deps: deps},
		&FireTrigger{svc: svc},
		&SearchActivations{svc: svc},
		&GetActivation{svc: svc},
	}
}
