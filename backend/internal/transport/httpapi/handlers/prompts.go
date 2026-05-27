package handlers

import (
	"net/http"
	"sort"

	"go.uber.org/zap"

	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	contextmgrapp "github.com/sunweilin/forgify/backend/internal/app/contextmgr"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	webtool "github.com/sunweilin/forgify/backend/internal/app/tool/web"
	tokencountpkg "github.com/sunweilin/forgify/backend/internal/pkg/tokencount"
	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// PromptsHandler exposes §18 inventory — every LLM-facing prompt the backend ships, in one place.
//
// PromptsHandler 提供 §18 prompt 总览——后端所有 LLM-facing prompt 一站式 audit。
type PromptsHandler struct {
	tools             []toolapp.Tool
	subagentRegistry  *subagentapp.Registry
	log               *zap.Logger
}

// NewPromptsHandler wires the dev-only prompts inventory; nil tools or registry just yields an empty section.
//
// NewPromptsHandler 装配 dev-only 总览；nil 入参对应段为空。
func NewPromptsHandler(tools []toolapp.Tool, subagentRegistry *subagentapp.Registry, log *zap.Logger) *PromptsHandler {
	if log == nil {
		log = zap.NewNop()
	}
	return &PromptsHandler{tools: tools, subagentRegistry: subagentRegistry, log: log.Named("handlers.prompts")}
}

func (h *PromptsHandler) Register(mux Registrar) {
	mux.HandleFunc("GET /api/v1/dev/prompts", h.List)
}

// promptEntry is one inventory row in the §18 listing.
//
// promptEntry 是 §18 总览中的一行。
type promptEntry struct {
	Name        string `json:"name"`
	Category    string `json:"category"`     // "tool" / "chat-system" / "subagent" / "internal-llm"
	Description string `json:"description"`  // short hint about role / when used
	Content     string `json:"content"`      // the literal prompt string
	Length      int    `json:"length"`       // len(Content) bytes
	TokensEst   int    `json:"tokensEst"`    // token estimate (tokencount.Estimate)
	Source      string `json:"source"`       // file:line hint for finding the source
}

// List returns the full prompt inventory grouped by category.
//
// List 返完整 prompt 总览，按 category 分组。
func (h *PromptsHandler) List(w http.ResponseWriter, _ *http.Request) {
	entries := make([]promptEntry, 0, 50)

	// Chat-facing static sections.
	// chat 端静态段。
	entries = append(entries, mkEntry("chat.identity", "chat-system",
		"Identity line opening every chat system prompt",
		chatapp.IdentityText(),
		"backend/internal/app/chat/runner.go::identitySection"))
	entries = append(entries, mkEntry("chat.how_to_work", "chat-system",
		"Operating principles (reuse / verify / care / ask / concise / parallel)",
		chatapp.HowToWorkText(),
		"backend/internal/app/chat/runner.go::howToWorkSection"))
	entries = append(entries, mkEntry("chat.tools", "chat-system",
		"Tool model + the three standard fields (summary / destructive / execution_group)",
		chatapp.ToolsText(),
		"backend/internal/app/chat/runner.go::toolsSection"))

	// Internal LLM prompts.
	// 内部 LLM 用提示词。
	entries = append(entries, mkEntry("contextmgr.compact", "internal-llm",
		"Compaction summary LLM system prompt (V1.2 §1)",
		contextmgrapp.CompactSystemPromptText(),
		"backend/internal/app/contextmgr/prompt.go::compactSystemPrompt"))
	entries = append(entries, mkEntry("web.summary", "internal-llm",
		"WebFetch summarisation prompt (template with placeholders)",
		webtool.SummaryPromptTemplate(),
		"backend/internal/app/tool/web/fetch.go::buildSummaryPrompt"))

	// Subagent system prompts.
	// Subagent 系统提示词。
	if h.subagentRegistry != nil {
		for _, t := range h.subagentRegistry.List() {
			entries = append(entries, mkEntry("subagent."+t.Name, "subagent",
				"Identity + scope for `"+t.Name+"` subagent type",
				t.SystemPrompt,
				"backend/internal/app/subagent/registry.go"))
		}
	}

	// Tool descriptions.
	// 系统 tool 描述。
	for _, tool := range h.tools {
		entries = append(entries, mkEntry("tool."+tool.Name(), "tool",
			"LLM-facing tool description",
			tool.Description(),
			"backend/internal/app/tool/*/"))
	}

	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Category != entries[j].Category {
			return entries[i].Category < entries[j].Category
		}
		return entries[i].Name < entries[j].Name
	})

	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"count":   len(entries),
		"entries": entries,
	})
}

func mkEntry(name, category, description, content, source string) promptEntry {
	return promptEntry{
		Name:        name,
		Category:    category,
		Description: description,
		Content:     content,
		Length:      len(content),
		TokensEst:   tokencountpkg.Estimate(content),
		Source:      source,
	}
}
