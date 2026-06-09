package subagent

import (
	"strings"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// subagentToolName is the Subagent (Task) tool's name — always stripped from a subagent's tool
// set so a subagent can never spawn another subagent (recursion guard, layer 1).
//
// subagentToolName 是 Subagent（Task）工具名——总从 subagent 的工具集剔除，使 subagent 永不能再派
// subagent（递归守卫第 1 层）。
const subagentToolName = "Subagent"

// Type is one built-in subagent kind: a system prompt + a tool allow-list + a default turn cap.
// AllowedTools empty means "everything the parent provides" (general-purpose); a non-empty list
// restricts the subagent to those tools (Explore / Plan). The Subagent tool itself is always
// stripped regardless (recursion).
//
// Type 是一个内置 subagent 种类：system prompt + 工具白名单 + 默认回合上限。AllowedTools 空 =
// 「父给的全部」（general-purpose）；非空则限定该子集（Explore / Plan）。Subagent 工具本身无论如何
// 都剔除（递归）。
type Type struct {
	Name            string
	SystemPrompt    string
	AllowedTools    []string
	DefaultMaxTurns int
}

// Built-in types. Hardcoded (not user entities, no table) — a subagent is a runtime mechanism,
// not a Quadrinity entity. Tool names are backend-new's actual tool Name()s.
//
// 内置类型。硬编码（非用户实体、无表）——subagent 是运行时机制、非 Quadrinity 实体。工具名是
// backend-new 的实际 Name()。
var builtInTypes = []Type{
	{
		Name: "Explore",
		SystemPrompt: "You are a code-reconnaissance subagent. Your job is to locate things on the " +
			"user's machine — files, definitions, usages, configuration — and report back concisely " +
			"with concrete paths and line references. Use LS/Glob/Grep/Read to search; do not modify " +
			"anything. Return a focused summary of what you found, not a transcript of your search.",
		AllowedTools:    []string{"Read", "LS", "Glob", "Grep"},
		DefaultMaxTurns: 30,
	},
	{
		Name: "Plan",
		SystemPrompt: "You are an architectural-planning subagent. Investigate the problem (read code, " +
			"search the web if useful) and produce a concrete, step-by-step implementation plan: which " +
			"files change, in what order, and the key decisions. Do not modify anything — output the plan.",
		AllowedTools:    []string{"Read", "LS", "Glob", "Grep", "WebFetch", "WebSearch"},
		DefaultMaxTurns: 25,
	},
	{
		Name: "general-purpose",
		SystemPrompt: "You are a general-purpose subagent spawned to carry out a focused task end to end. " +
			"You have the parent's tools (except spawning further subagents). Do the work and return a " +
			"concise result.",
		AllowedTools:    nil, // everything the parent provides (minus the Subagent tool)
		DefaultMaxTurns: 25,
	},
}

// Registry indexes the built-in subagent types by name.
//
// Registry 按名索引内置 subagent 类型。
type Registry struct{ byName map[string]Type }

// NewRegistry builds the registry from the built-in types.
//
// NewRegistry 由内置类型构建注册表。
func NewRegistry() *Registry {
	m := make(map[string]Type, len(builtInTypes))
	for _, t := range builtInTypes {
		m[t.Name] = t
	}
	return &Registry{byName: m}
}

// Get returns the type by name; ok=false for an unknown type.
//
// Get 按名返回类型；未知类型 ok=false。
func (r *Registry) Get(name string) (Type, bool) {
	t, ok := r.byName[name]
	return t, ok
}

// Names returns the built-in type names (for the Subagent tool's enum + the catalog).
//
// Names 返回内置类型名（供 Subagent 工具 enum + catalog）。
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.byName))
	for _, t := range builtInTypes {
		out = append(out, t.Name)
	}
	return out
}

// filterTools applies a type's allow-list to the parent's tool set and always strips the
// Subagent tool (recursion guard). An empty allow-list keeps everything (minus Subagent).
//
// filterTools 把类型的白名单套在父工具集上、并总剔除 Subagent 工具（递归守卫）。空白名单保留全部
// （减 Subagent）。
func filterTools(t Type, all []toolapp.Tool) []toolapp.Tool {
	var allow map[string]bool
	if len(t.AllowedTools) > 0 {
		allow = make(map[string]bool, len(t.AllowedTools))
		for _, n := range t.AllowedTools {
			allow[n] = true
		}
	}
	out := make([]toolapp.Tool, 0, len(all))
	for _, tool := range all {
		if tool.Name() == subagentToolName {
			continue
		}
		if allow != nil && !allow[tool.Name()] {
			continue
		}
		out = append(out, tool)
	}
	return out
}

// composeSystemPrompt prefixes a shared subagent preamble, appends the type's prompt, and adds a
// reply-language line for non-English workspaces.
//
// composeSystemPrompt 前置共享 subagent 序言、接类型 prompt、并为非英语工作区加回复语言行。
func composeSystemPrompt(t Type, locale reqctxpkg.Locale) string {
	var b strings.Builder
	b.WriteString("You are a Forgify subagent — a focused sub-task LLM spawned by the main conversation. " +
		"Work autonomously toward the task below and finish with a concise, self-contained answer (it " +
		"becomes the result handed back to the caller).\n\n")
	b.WriteString(t.SystemPrompt)
	if locale == reqctxpkg.LocaleZhCN {
		b.WriteString("\n\nReply in Chinese.")
	}
	return b.String()
}
