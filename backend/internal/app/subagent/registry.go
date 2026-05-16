package subagent

import (
	"sort"
	"sync"

	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
)

// defaultMaxTurns is the fallback when SubagentType.DefaultMaxTurns is zero.
//
// defaultMaxTurns 是 SubagentType.DefaultMaxTurns 为 0 时的兜底。
const defaultMaxTurns = 25

var builtInTypes = []subagentdomain.SubagentType{
	{
		Name:            "Explore",
		SystemPrompt:    "You are Explore, a code reconnaissance agent. Your job is to locate files, definitions, and references quickly. Use Read / Glob / Grep / LS to navigate. Return a concise summary of what you found (paths, line numbers, brief snippets). Do NOT propose changes or analysis — your role is purely to locate.",
		AllowedTools:    []string{"Read", "Glob", "Grep", "LS", "search_forges"},
		DefaultMaxTurns: 30,
	},
	{
		Name:            "Plan",
		SystemPrompt:    "You are Plan, an architectural advisor. Your job is to produce a concrete implementation plan. Use Read / Glob / Grep / LS to inspect the existing code; use WebFetch / WebSearch when external context helps. Return a step-by-step plan, the critical files involved, and the main trade-offs. Do NOT modify any files — your role is strategy only.",
		AllowedTools:    []string{"Read", "Glob", "Grep", "LS", "WebFetch", "WebSearch"},
		DefaultMaxTurns: 25,
	},
	{
		Name:            "general-purpose",
		SystemPrompt:    "You are a general-purpose subagent. You inherit the parent agent's full tool registry minus Subagent itself, so you can read, search, edit, run shells, and more. Focus on completing the focused subtask the parent delegated to you, then return a concise summary.",
		AllowedTools:    nil,
		DefaultMaxTurns: 25,
	},
}

// Registry indexes SubagentType by Name; read-only after construction.
//
// Registry 按 Name 索引 SubagentType，构造后只读。
type Registry struct {
	once sync.Once
	idx  map[string]subagentdomain.SubagentType
}

// NewRegistry builds the registry from the built-in types.
//
// NewRegistry 用内置类型构建注册表。
func NewRegistry() *Registry {
	return &Registry{}
}

func (r *Registry) ensureIndexed() {
	r.once.Do(func() {
		r.idx = make(map[string]subagentdomain.SubagentType, len(builtInTypes))
		for _, t := range builtInTypes {
			if t.DefaultMaxTurns <= 0 {
				t.DefaultMaxTurns = defaultMaxTurns
			}
			r.idx[t.Name] = t
		}
	})
}

// Get returns the SubagentType matching name; ok=false when absent.
//
// Get 按 name 取 SubagentType，不存在时 ok=false。
func (r *Registry) Get(name string) (subagentdomain.SubagentType, bool) {
	r.ensureIndexed()
	t, ok := r.idx[name]
	return t, ok
}

// List returns all registered types in stable Name order.
//
// List 返回所有注册类型，按 Name 字母序稳定输出。
func (r *Registry) List() []subagentdomain.SubagentType {
	r.ensureIndexed()
	out := make([]subagentdomain.SubagentType, 0, len(r.idx))
	for _, t := range r.idx {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
