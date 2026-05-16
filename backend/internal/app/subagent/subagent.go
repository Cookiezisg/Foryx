// Package subagent (app layer) implements the Subagent system tool's Spawn → loop.Run → terminal-write lifecycle.
//
// Package subagent（app 层）实现 Subagent 系统工具的 Spawn → loop.Run → 终态写生命周期。
package subagent

import (
	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Service ties the registry, chat repo, and shared infra together; Spawn is the only mutating entry.
//
// Service 把 registry、chat repo 与共享 infra 绑定，Spawn 是唯一变更入口。
type Service struct {
	chatRepo    chatdomain.Repository
	registry    *Registry
	tools       []toolapp.Tool
	modelPicker modeldomain.ModelPicker
	keyProvider apikeydomain.KeyProvider
	llmFactory  *llminfra.Factory
	log         *zap.Logger
}

// New constructs a Service; tools may be nil and supplied later via SetTools.
//
// New 构造 Service；tools 可暂为 nil，事后用 SetTools 注入。
func New(
	chatRepo chatdomain.Repository,
	registry *Registry,
	modelPicker modeldomain.ModelPicker,
	keyProvider apikeydomain.KeyProvider,
	llmFactory *llminfra.Factory,
	log *zap.Logger,
) *Service {
	if log == nil {
		panic("subagent.New: logger is nil")
	}
	return &Service{
		chatRepo:    chatRepo,
		registry:    registry,
		modelPicker: modelPicker,
		keyProvider: keyProvider,
		llmFactory:  llmFactory,
		log:         log,
	}
}

// SetTools injects the registered global tool list.
//
// SetTools 注入全局 tool 列表。
func (s *Service) SetTools(tools []toolapp.Tool) {
	s.tools = tools
}

// subagentStrippedTools is the closed deny-list of tool names sub-agents must never see.
//
// subagentStrippedTools 是 sub-agent 不可见的封闭黑名单。
var subagentStrippedTools = map[string]bool{
	"Subagent":         true,
	"create_workflow":  true,
	"edit_workflow":    true,
	"delete_workflow":  true,
	"revert_workflow":  true,
	"trigger_workflow": true,
}

// filterTools drops stripped tools and any tools not in typ.AllowedTools (when set).
//
// filterTools 过滤掉黑名单 tool 与 typ.AllowedTools（若设）外的 tool。
func (s *Service) filterTools(typ subagentdomain.SubagentType) []toolapp.Tool {
	if len(s.tools) == 0 {
		return nil
	}
	var allowed map[string]struct{}
	if len(typ.AllowedTools) > 0 {
		allowed = make(map[string]struct{}, len(typ.AllowedTools))
		for _, name := range typ.AllowedTools {
			allowed[name] = struct{}{}
		}
	}
	out := make([]toolapp.Tool, 0, len(s.tools))
	for _, t := range s.tools {
		if subagentStrippedTools[t.Name()] {
			continue
		}
		if allowed != nil {
			if _, ok := allowed[t.Name()]; !ok {
				continue
			}
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// composeSystemPrompt prepends the Forgify subagent preamble and appends a zh-CN locale hint.
//
// composeSystemPrompt 给 type system prompt 前置 Forgify 序文，并按 zh-CN locale 追加提示。
func composeSystemPrompt(typeSystemPrompt string, locale reqctxpkg.Locale) string {
	const preamble = "You are a Forgify subagent — a focused sub-task LLM spawned by the main conversation. " +
		"Stay narrowly on your assigned task; return a concise summary suitable for the parent LLM."
	out := preamble + "\n\n" + typeSystemPrompt
	if locale == reqctxpkg.LocaleZhCN {
		out += "\n\nPlease respond in Chinese (Simplified)."
	}
	return out
}
