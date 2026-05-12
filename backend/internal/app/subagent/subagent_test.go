// subagent_test.go — pure-function unit tests for the registry, filter,
// and prompt composer. Service.Spawn end-to-end (LLM + bridge + DB) lives
// in test/subagent/ pipeline tests (D4-5).
//
// subagent_test.go ——registry / filter / prompt 合成的纯函数单测。
// Service.Spawn 端到端（LLM + bridge + DB）在 test/subagent/ pipeline 套（D4-5）。
package subagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ── Registry ─────────────────────────────────────────────────────────

func TestRegistry_Get_BuiltInTypes(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"Explore", "Plan", "general-purpose"} {
		typ, ok := r.Get(name)
		if !ok {
			t.Errorf("Get(%q) = false, want true", name)
			continue
		}
		if typ.Name != name {
			t.Errorf("Get(%q).Name = %q", name, typ.Name)
		}
		if typ.SystemPrompt == "" {
			t.Errorf("%s missing SystemPrompt", name)
		}
		if typ.DefaultMaxTurns <= 0 {
			t.Errorf("%s DefaultMaxTurns = %d, want > 0", name, typ.DefaultMaxTurns)
		}
	}
}

func TestRegistry_Get_UnknownReturnsFalse(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("Nonexistent"); ok {
		t.Error("Get of unknown type returned ok=true")
	}
}

func TestRegistry_List_StableAlphaOrder(t *testing.T) {
	r := NewRegistry()
	got := r.List()
	if len(got) != 3 {
		t.Fatalf("List len = %d, want 3", len(got))
	}
	want := []string{"Explore", "Plan", "general-purpose"}
	for i, expect := range want {
		if got[i].Name != expect {
			t.Errorf("List[%d] = %q, want %q", i, got[i].Name, expect)
		}
	}
}

func TestRegistry_GeneralPurpose_AllowedToolsNil(t *testing.T) {
	r := NewRegistry()
	gp, _ := r.Get("general-purpose")
	if gp.AllowedTools != nil {
		t.Errorf("general-purpose AllowedTools = %v, want nil (inherit-all marker)", gp.AllowedTools)
	}
}

func TestRegistry_Explore_WhitelistOnlyReadOnly(t *testing.T) {
	r := NewRegistry()
	exp, _ := r.Get("Explore")
	if len(exp.AllowedTools) == 0 {
		t.Fatal("Explore.AllowedTools empty — should whitelist read tools")
	}
	for _, mutator := range []string{"Write", "Edit", "Bash", "create_forge", "edit_forge"} {
		for _, tool := range exp.AllowedTools {
			if tool == mutator {
				t.Errorf("Explore whitelist contains mutator %q", mutator)
			}
		}
	}
}

// ── filterTools ──────────────────────────────────────────────────────

// fakeTool satisfies toolapp.Tool with only Name() doing useful work; the
// rest are no-op stubs sufficient to compile against the interface.
//
// fakeTool 满足 toolapp.Tool，只 Name() 有效，其余 no-op 足以编译。
type fakeTool struct{ name string }

func (f fakeTool) Name() string                                                              { return f.name }
func (f fakeTool) Description() string                                                       { return "" }
func (f fakeTool) Parameters() json.RawMessage                                               { return json.RawMessage(`{}`) }
func (f fakeTool) IsReadOnly() bool                                                          { return true }
func (f fakeTool) NeedsReadFirst() bool                                                      { return false }
func (f fakeTool) RequiresWorkspace() bool                                                   { return false }
func (f fakeTool) ValidateInput(_ json.RawMessage) error                                     { return nil }
func (f fakeTool) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}
func (f fakeTool) Execute(_ context.Context, _ string) (string, error) { return "", nil }

func makeTools(names ...string) []toolapp.Tool {
	out := make([]toolapp.Tool, len(names))
	for i, n := range names {
		out[i] = fakeTool{name: n}
	}
	return out
}

// TestFilterTools_DropsSubagentItself verifies the structural recursion
// defense (subagent.md §8 保险 1): regardless of whitelist, a tool named
// "Subagent" is removed from the per-spawn tool list.
//
// TestFilterTools_DropsSubagentItself 验证结构性防递归（§8 保险 1）：
// 不论白名单如何，名为 "Subagent" 的工具都被移除。
func TestFilterTools_DropsSubagentItself(t *testing.T) {
	s := &Service{tools: makeTools("Read", "Subagent", "Bash")}
	out := s.filterTools(subagentdomain.SubagentType{}) // empty whitelist = inherit
	for _, tt := range out {
		if tt.Name() == "Subagent" {
			t.Error("filterTools left Subagent in the registry")
		}
	}
	if len(out) != 2 {
		t.Errorf("len = %d, want 2 (Read + Bash)", len(out))
	}
}

func TestFilterTools_WhitelistApplied(t *testing.T) {
	s := &Service{tools: makeTools("Read", "Write", "Bash", "Grep")}
	out := s.filterTools(subagentdomain.SubagentType{
		AllowedTools: []string{"Read", "Grep"},
	})
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	names := map[string]bool{}
	for _, tt := range out {
		names[tt.Name()] = true
	}
	if !names["Read"] || !names["Grep"] {
		t.Errorf("missing whitelisted tool: got %v", names)
	}
}

func TestFilterTools_NilWhitelistInheritsAll(t *testing.T) {
	s := &Service{tools: makeTools("Read", "Write", "Bash")}
	out := s.filterTools(subagentdomain.SubagentType{AllowedTools: nil})
	if len(out) != 3 {
		t.Errorf("len = %d, want 3 (no whitelist = all)", len(out))
	}
}

func TestFilterTools_WhitelistedSubagentStillDropped(t *testing.T) {
	s := &Service{tools: makeTools("Read", "Subagent")}
	out := s.filterTools(subagentdomain.SubagentType{
		AllowedTools: []string{"Read", "Subagent"}, // pointless inclusion
	})
	for _, tt := range out {
		if tt.Name() == "Subagent" {
			t.Error("Subagent whitelisted but filter must drop it anyway (recursion defense)")
		}
	}
}

func TestFilterTools_EmptyRegistryReturnsNil(t *testing.T) {
	s := &Service{tools: nil}
	if got := s.filterTools(subagentdomain.SubagentType{}); got != nil {
		t.Errorf("empty registry should return nil, got %+v", got)
	}
}

// TestFilterTools_StripsWorkflowMutationOps verifies D21: sub-agents
// cannot see workflow create/edit/delete/revert/trigger tools. Workflow
// assembly + execution are the main agent's responsibility; sub-agents
// forge atoms (Function / Handler) only.
//
// TestFilterTools_StripsWorkflowMutationOps 验 D21:sub-agent 看不到
// workflow 突变 + 触发 tool。workflow 装配 + 执行是主 agent 职责。
func TestFilterTools_StripsWorkflowMutationOps(t *testing.T) {
	s := &Service{tools: makeTools(
		"create_function", "call_handler",
		"create_workflow", "edit_workflow",
		"delete_workflow", "revert_workflow", "trigger_workflow",
		"Subagent",
	)}
	out := s.filterTools(subagentdomain.SubagentType{})
	got := map[string]bool{}
	for _, tt := range out {
		got[tt.Name()] = true
	}
	for _, banned := range []string{
		"create_workflow", "edit_workflow", "delete_workflow",
		"revert_workflow", "trigger_workflow", "Subagent",
	} {
		if got[banned] {
			t.Errorf("D21 violation: %q must be stripped from sub-agent toolset", banned)
		}
	}
	for _, kept := range []string{"create_function", "call_handler"} {
		if !got[kept] {
			t.Errorf("forge atoms must stay available: %q stripped", kept)
		}
	}
}

// TestFilterTools_KeepsReadOnlyWorkflowTools — D21 carve-out: search +
// get for workflow stay (read-only, no side effect; forger sub-agents
// may reference existing workflow shape when authoring related entities).
//
// TestFilterTools_KeepsReadOnlyWorkflowTools D21 例外:read-only workflow
// tool 留(无副作用;forger 子 agent 可参考现有 workflow 形状)。
func TestFilterTools_KeepsReadOnlyWorkflowTools(t *testing.T) {
	s := &Service{tools: makeTools("search_workflow", "get_workflow")}
	out := s.filterTools(subagentdomain.SubagentType{})
	if len(out) != 2 {
		t.Errorf("read-only workflow tools dropped: got %d kept, want 2", len(out))
	}
}

// TestFilterTools_KeepsSelfTestTools — call_handler + run_function are
// how sub-agents test the entity they just forged. Stripping these would
// force every sub-agent to ship without self-validation, regressing
// quality.
//
// TestFilterTools_KeepsSelfTestTools call_handler/run_function 留 — 子
// agent 自测 forged entity 必需。
func TestFilterTools_KeepsSelfTestTools(t *testing.T) {
	s := &Service{tools: makeTools("call_handler", "run_function")}
	out := s.filterTools(subagentdomain.SubagentType{})
	if len(out) != 2 {
		t.Errorf("self-test tools dropped: got %d kept, want 2", len(out))
	}
}

// ── composeSystemPrompt ──────────────────────────────────────────────

func TestComposeSystemPrompt_PreambleAlwaysPresent(t *testing.T) {
	out := composeSystemPrompt("explore the world", reqctxpkg.LocaleEn)
	if !strings.Contains(out, "Forgify") {
		t.Error("preamble identity 'Forgify' missing")
	}
	if !strings.Contains(out, "subagent") {
		t.Error("preamble role 'subagent' missing")
	}
	if !strings.Contains(out, "explore the world") {
		t.Error("per-type prompt body missing")
	}
}

func TestComposeSystemPrompt_ZhCNAddsLocaleDirective(t *testing.T) {
	out := composeSystemPrompt("body", reqctxpkg.LocaleZhCN)
	if !strings.Contains(out, "Chinese") {
		t.Error("zh-CN locale should add Chinese-response directive")
	}
}

func TestComposeSystemPrompt_NonChineseLocaleNoDirective(t *testing.T) {
	out := composeSystemPrompt("body", reqctxpkg.LocaleEn)
	if strings.Contains(out, "Chinese") {
		t.Error("en locale should not add Chinese directive")
	}
}
