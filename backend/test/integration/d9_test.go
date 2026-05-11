//go:build pipeline

// d9_test.go — D9 cross-cutting integration / smoke coverage. Verifies
// the full Phase 4 准备件 stack (sandbox v2 + subagent + mcp + skill +
// catalog) actually composes when wired together at boot, and that
// the Capability Catalog summary propagates from Service.GetForSystemPrompt
// all the way onto the wire the LLM consumes.
//
// Three scenarios:
//
//  1. CatalogReachesLLM
//     Seed 1 forge + 1 skill → drive a chat turn through FakeLLM →
//     assert the role:system content the LLM received contains the
//     Catalog Summary's signature ('## Available capabilities' +
//     the seeded forge + skill names). Closes the gap that D8 unit
//     tests left open: they verified GetForSystemPrompt returns the
//     right text, but never that the chat runner propagates it onto
//     the wire.
//
//  2. DynamicSkillUpdate
//     Boot harness (catalog polling running) → write a SKILL.md to
//     disk → wait for fsnotify debounce + 1s catalog poll tick →
//     assert the new skill name appears in catalog Summary. Proves
//     the skill watcher → catalog regen chain works end-to-end.
//
//  3. BootSmoke
//     Bare boot of the harness → assert: all expected tool families
//     are registered, Catalog produces a non-nil Summary on demand,
//     all 5 service handles (Forge / Skill / MCP / Catalog / Sandbox)
//     are non-nil. This is the investor-demo confidence check —
//     nothing crashed during DI assembly.
//
// d9_test.go ——D9 跨切集成 / 烟雾覆盖。验全 Phase 4 准备件栈（sandbox
// v2 + subagent + mcp + skill + catalog）装配在一起真组合 OK，Capability
// Catalog summary 从 GetForSystemPrompt 真传到 LLM 看的 wire 上。
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

// ── 1. CatalogReachesLLM ─────────────────────────────────────────────

func TestD9_CatalogReachesLLM(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	// Single LLM turn: emit a brief text response so the conversation
	// terminates cleanly. We don't care about the response content —
	// the assertion is about what we sent INTO the LLM, not what it
	// emitted. Push a default script too because chat.runner fires a
	// second LLM call for auto-title generation.
	// 单次 LLM 轮：emit 简短 text 让对话干净终止。我们不关心响应内容
	// ——断言关于发给 LLM 的内容，不是它发出的。也设默认脚本，因
	// chat.runner 会再调一次 LLM 自动生成标题。
	fake.PushScript(th.ScriptText("OK, I see what's available."))
	fake.PushDefault(th.ScriptText("Title"))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")

	// Seed one function + one skill so the catalog has real content to
	// surface (and not a 'no capabilities' empty section).
	// 种一 function + 一 skill 让 catalog 有真内容（非 '无 capabilities' 空段）。
	h.NewFunction(t, "csv-clean", "def csv_clean(args):\n    return args\n")
	seedSkill(t, h, "deploy", "Deploy via internal CI")

	// Force an immediate catalog refresh so we don't depend on the 1s
	// poll tick happening before our chat call.
	// 强制立即 catalog refresh，不依赖 1s poll tick 在 chat 调用前发生。
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

	// Now inspect what the FakeLLM actually received.
	// 现在检查 FakeLLM 真收到啥。
	gotPrompt := fake.LastSystemPrompt()
	if gotPrompt == "" {
		t.Fatal("LastSystemPrompt is empty — chat runner did not send a system message")
	}

	// The catalog Summary always contains '## Available capabilities'
	// (both LLMGenerator's prompt template and mechanical fallback
	// produce this header).
	// catalog Summary 永含 '## Available capabilities'（LLMGenerator
	// 模板和 mechanical fallback 都产此头）。
	if !strings.Contains(gotPrompt, "## Available capabilities") {
		t.Errorf("system prompt missing catalog header.\nfull prompt:\n%s", gotPrompt)
	}
	if !strings.Contains(gotPrompt, "csv-clean") {
		t.Errorf("system prompt missing seeded forge name 'csv-clean'.\nfull prompt:\n%s", gotPrompt)
	}
	if !strings.Contains(gotPrompt, "deploy") {
		t.Errorf("system prompt missing seeded skill name 'deploy'.\nfull prompt:\n%s", gotPrompt)
	}

	// Sanity: also assert the base intro is still there. Catalog block
	// should be appended to the base prompt, not replace it.
	// 完整性：基础 intro 仍在。catalog 块附加而非替代。
	if !strings.Contains(gotPrompt, "You are Forgify") {
		t.Errorf("base intro lost from system prompt:\n%s", gotPrompt)
	}
}

// ── 2. DynamicSkillUpdate ────────────────────────────────────────────

func TestD9_DynamicSkillUpdate(t *testing.T) {
	h := th.New(t)

	// Initial Refresh — catalog reflects empty skill set.
	// 首次 Refresh——catalog 反映空 skill 集。
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

	// Drop a SKILL.md. The skill watcher is started by the harness, so
	// fsnotify should pick this up. To avoid a race where the watcher
	// hasn't yet added the new subdir's watch when SKILL.md gets
	// created (which would lose the inner Create event), we build the
	// skill outside the watched root then atomically rename it in.
	// The Rename triggers a dir-Create on the watched root → watcher
	// adds the new dir's watch + Service.Scan walks the disk fresh
	// (which finds SKILL.md regardless of whether its inner Create
	// event got delivered).
	//
	// 拖入 SKILL.md。watcher 由 harness 启，fsnotify 拾。为避免 watcher
	// 还没给新子目录加 watch 时 SKILL.md 已创建（丢内部 Create 事件）
	// 的竞态，先在 watched root 外建好 + atomic rename 进。Rename 触发
	// 根上 dir-Create → watcher 加新 dir watch + Service.Scan 走 disk
	// 找 SKILL.md（无论内部 Create 事件是否到位）。
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

	// Wait for fsnotify debounce + Skill.Scan to complete. Poll the
	// skill cache rather than sleeping a fixed amount — finishes as
	// soon as the watcher has caught up.
	// 等 fsnotify debounce + Skill.Scan 完成。轮询 skill cache 而非定额
	// 死睡——watcher 跟上即结束。
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

	// Now trigger the catalog Refresh — it should see the new skill
	// from the source and bump the fingerprint.
	// 现在触发 catalog Refresh——应从 source 看到新 skill + bump fp。
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

// ── 3. BootSmoke ─────────────────────────────────────────────────────

func TestD9_BootSmoke(t *testing.T) {
	// Pure boot. No seeding. Asserts the DI graph wires cleanly + every
	// promised service field is non-nil + every expected tool family is
	// registered. This is the test we'd run before an investor demo to
	// know nothing in the wiring graph regressed.
	// 纯 boot。无种。验 DI 图干净接 + 每个承诺的 service 字段非 nil + 每
	// 个预期 tool 家族注册。投资人 demo 前跑这个验线没回归。
	h := th.New(t)

	// Service handles non-nil.
	// service handle 非 nil。
	if h.Function == nil || h.Skill == nil || h.MCP == nil ||
		h.Catalog == nil || h.Sandbox == nil ||
		h.APIKey == nil || h.Model == nil || h.Conversation == nil ||
		h.Chat == nil {
		t.Fatalf("service field nil after harness boot:\n  Function=%v Skill=%v MCP=%v Catalog=%v Sandbox=%v APIKey=%v Model=%v Conv=%v Chat=%v",
			h.Function != nil, h.Skill != nil, h.MCP != nil, h.Catalog != nil, h.Sandbox != nil,
			h.APIKey != nil, h.Model != nil, h.Conversation != nil, h.Chat != nil)
	}

	// All expected tool families registered. Each system tool has a
	// stable Name(). We assert the set we expect contains the tools we
	// built — a missing tool here means main.go / harness wiring
	// regressed.
	// 全预期 tool 家族注册。每系统 tool 有稳定 Name()。我们断言期望
	// 集合含我们建的 tool——缺一个意味 main.go / harness 接线回归。
	gotNames := map[string]bool{}
	for _, tool := range h.Tools {
		gotNames[tool.Name()] = true
	}
	wantTools := []string{
		// function family (forge_redesign trinity)
		"search_function", "get_function", "create_function", "edit_function",
		"revert_function", "delete_function", "run_function",
		// filesystem family
		"Read", "Write", "Edit", "Glob", "Grep",
		// shell family
		"Bash", "BashOutput", "KillShell",
		// web family
		"WebFetch", "WebSearch",
		// todo family
		"TodoCreate", "TodoList", "TodoGet", "TodoUpdate",
		// ask family
		"AskUserQuestion",
		// subagent
		"Subagent",
		// mcp family
		"search_mcp_tools", "call_mcp_tool",
		// skill family
		"search_skills", "activate_skill",
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

	// Catalog produces a non-nil Summary on demand (mechanical fallback
	// fires since no LLM key is wired in this minimal boot).
	// catalog 按需产非 nil Summary（无 LLM key → mech fallback）。
	if err := h.Catalog.Refresh(context.Background()); err != nil {
		t.Fatalf("Catalog.Refresh: %v", err)
	}
	cat := h.Catalog.Get()
	if cat == nil || cat.Summary == "" {
		t.Errorf("Catalog Summary empty after Refresh; got %+v", cat)
	}

	// MCP service started cleanly with empty config. ListServers should
	// return an empty slice (not nil) so the catalog source's empty-
	// items contract is satisfied.
	// MCP 空配置干净启。ListServers 应返空 slice（非 nil）让 catalog
	// source 空 items 契约满足。
	if servers := h.MCP.ListServers(context.Background()); servers == nil {
		t.Errorf("MCP.ListServers returned nil; want empty slice")
	}
}

// ── helpers ──────────────────────────────────────────────────────────

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
