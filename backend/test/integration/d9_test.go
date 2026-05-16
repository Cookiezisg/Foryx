//go:build pipeline

// Package integration runs D9 cross-cutting integration smoke tests.
//
// Package integration 跑 D9 跨切集成烟雾测试。
package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

func TestD9_CatalogReachesLLM(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptText("OK, I see what's available."))
	fake.PushDefault(th.ScriptText("Title"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")

	h.NewFunction(t, "csv_clean", "def csv_clean(args):\n    return args\n")
	seedSkill(t, h, "deploy", "Deploy via internal CI")

	if err := h.Catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Catalog.Refresh: %v", err)
	}

	conv := h.NewConversation(t, "d9-catalog-reaches-llm")
	sub := h.SubscribeSSE(t, conv.ID)
	th.PostMessage(t, h, conv.ID, "What can you do?")

	final := sub.WaitForAssistantTerminal(60 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errMsg=%q\nraw:\n%s", final.Status, final.ErrorMessage, sub.FormatRawEvents())
	}

	gotPrompt := fake.LastSystemPrompt()
	if gotPrompt == "" {
		t.Fatal("LastSystemPrompt is empty — chat runner did not send a system message")
	}

	if !strings.Contains(gotPrompt, "## Available capabilities") {
		t.Errorf("system prompt missing catalog header.\nfull prompt:\n%s", gotPrompt)
	}
	if !strings.Contains(gotPrompt, "csv_clean") {
		t.Errorf("system prompt missing seeded forge name 'csv-clean'.\nfull prompt:\n%s", gotPrompt)
	}
	if !strings.Contains(gotPrompt, "deploy") {
		t.Errorf("system prompt missing seeded skill name 'deploy'.\nfull prompt:\n%s", gotPrompt)
	}

	if !strings.Contains(gotPrompt, "You are Forgify") {
		t.Errorf("base intro lost from system prompt:\n%s", gotPrompt)
	}
}

func TestD9_DynamicSkillUpdate(t *testing.T) {
	h := th.New(t)

	if err := h.Catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("initial Refresh: %v", err)
	}
	initial := h.Catalog.Get()
	if initial == nil {
		t.Fatal("Catalog nil after initial Refresh")
	}
	if strings.Contains(initial.Summary, "dropped-skill") {
		t.Fatalf("initial Summary already contains the test skill — leftover from prior test?\n%s", initial.Summary)
	}

	// Build outside watched root then atomic rename in to avoid fsnotify race on new subdir.
	stage := filepath.Join(t.TempDir(), "dropped-skill")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		t.Fatalf("mkdir stage: %v", err)
	}
	skillContent := "---\nname: dropped-skill\ndescription: dropped at runtime\n---\n# body\n"
	if err := os.WriteFile(filepath.Join(stage, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	target := filepath.Join(h.Skill.SkillsDir(), "dropped-skill")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir target parent: %v", err)
	}
	if err := os.Rename(stage, target); err != nil {
		t.Fatalf("rename into watched dir: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if sk, err := h.Skill.Get(context.Background(), "dropped-skill"); err == nil && sk != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	sk, err := h.Skill.Get(context.Background(), "dropped-skill")
	if err != nil || sk == nil {
		t.Fatalf("skill watcher did not pick up dropped SKILL.md within 5s")
	}

	if err := h.Catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("post-drop Refresh: %v", err)
	}
	updated := h.Catalog.Get()
	if updated.Fingerprint == initial.Fingerprint {
		t.Errorf("Fingerprint unchanged after skill drop; chain didn't propagate")
	}
	if !strings.Contains(updated.Summary, "dropped-skill") {
		t.Errorf("Summary missing dropped-skill after fsnotify → catalog regen chain:\n%s", updated.Summary)
	}
}

func TestD9_BootSmoke(t *testing.T) {
	h := th.New(t)

	if h.Function == nil || h.Skill == nil || h.MCP == nil ||
		h.Catalog == nil || h.Sandbox == nil ||
		h.APIKey == nil || h.Model == nil || h.Conversation == nil ||
		h.Chat == nil {
		t.Fatalf("service field nil after harness boot:\n  Function=%v Skill=%v MCP=%v Catalog=%v Sandbox=%v APIKey=%v Model=%v Conv=%v Chat=%v",
			h.Function != nil, h.Skill != nil, h.MCP != nil, h.Catalog != nil, h.Sandbox != nil,
			h.APIKey != nil, h.Model != nil, h.Conversation != nil, h.Chat != nil)
	}
	if h.Workflow == nil || h.Handler == nil {
		t.Fatalf("trinity service nil: Workflow=%v Handler=%v", h.Workflow != nil, h.Handler != nil)
	}
	if h.Scheduler == nil || h.Trigger == nil || h.FlowRunRepo == nil {
		t.Fatalf("Plan 05 execution-plane services nil: Scheduler=%v Trigger=%v FlowRunRepo=%v",
			h.Scheduler != nil, h.Trigger != nil, h.FlowRunRepo != nil)
	}

	gotNames := map[string]bool{}
	for _, tool := range h.Tools {
		gotNames[tool.Name()] = true
	}
	wantTools := []string{
		"search_function", "get_function", "create_function", "edit_function",
		"revert_function", "delete_function", "run_function",
		"search_function_executions", "get_function_execution",
		"search_handler", "get_handler", "create_handler", "edit_handler",
		"revert_handler", "delete_handler", "call_handler", "update_handler_config",
		"search_handler_calls", "get_handler_call",
		"search_workflow", "get_workflow", "create_workflow", "edit_workflow",
		"revert_workflow", "delete_workflow",
		"search_workflow_executions", "get_workflow_execution",
		"Read", "Write", "Edit", "Glob", "Grep",
		"Bash", "BashOutput", "KillShell",
		"WebFetch", "WebSearch",
		"TodoCreate", "TodoList", "TodoGet", "TodoUpdate",
		"AskUserQuestion",
		"Subagent",
		"search_mcp_tools", "call_mcp_tool",
		"search_mcp_calls", "get_mcp_call",
		"search_skills", "activate_skill",
		"search_skill_executions", "get_skill_execution",
	}
	missing := []string{}
	for _, w := range wantTools {
		if !gotNames[w] {
			missing = append(missing, w)
		}
	}
	if len(missing) > 0 {
		t.Errorf("expected tool families missing from registry: %v\ngot: %v", missing, gotNames)
	}

	if err := h.Catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Catalog.Refresh: %v", err)
	}
	cat := h.Catalog.Get()
	if cat == nil {
		t.Errorf("Catalog.Get returned nil after Refresh")
	}
	// Empty Summary is fine when no functions/handlers/skills/mcps are
	// registered — mechanical fallback intentionally outputs "" for an
	// empty library (catalog/mechanical.go: avoids "## Available
	// capabilities" header followed by nothing).
	// 空 Summary 在无功能 / 无 handler / 无 skill / 无 mcp 时是合理的——
	// mechanical fallback 故意为空库返 ""（避免 "## Available capabilities"
	// 头后空白怪态）。

	if servers := h.MCP.ListServers(context.Background()); servers == nil {
		t.Errorf("MCP.ListServers returned nil; want empty slice")
	}
}

func seedSkill(t *testing.T, h *th.Harness, name, desc string) {
	t.Helper()
	dir := filepath.Join(h.Skill.SkillsDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n# body\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := h.Skill.Scan(context.Background()); err != nil {
		t.Fatalf("Skill.Scan: %v", err)
	}
}
