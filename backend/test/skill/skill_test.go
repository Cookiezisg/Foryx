//go:build pipeline

// Package skill runs pipeline tests for the Skill subsystem.
//
// Package skill 跑 Skill 子系统 pipeline 测试。
package skill

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	th "github.com/sunweilin/forgify/backend/test/harness"
)

// seedSkill writes a SKILL.md and triggers Scan so the in-memory cache reflects it.
//
// seedSkill 写 SKILL.md 并 Scan，让内存 cache 同步。
func seedSkill(t *testing.T, h *th.Harness, name, frontmatter, body string) {
	t.Helper()
	dir := filepath.Join(h.Skill.SkillsDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("seedSkill mkdir %s: %v", dir, err)
	}
	content := "---\n" + frontmatter + "\n---\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("seedSkill write %s/SKILL.md: %v", name, err)
	}
	if err := h.Skill.Scan(context.Background()); err != nil {
		t.Fatalf("seedSkill Scan: %v", err)
	}
}

func TestSkill_Activate_Inline_E2E(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"activate_skill", "call_act_1",
		`{"name":"pr-review","arguments":["1234"],"summary":"running pr-review skill"}`,
	))
	fake.PushScript(th.ScriptText("Skill loaded. Following the steps for PR #1234."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")

	seedSkill(t, h, "pr-review",
		`name: pr-review
description: Review a GitHub PR
arguments:
  - pr_number`,
		`# Review PR #$1
Step 1: gh pr view $1`)

	conv := h.NewConversation(t, "skill-activate-inline")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Review pull request 1234")

	final := sub.WaitForAssistantTerminal(60 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errCode=%q errMsg=%q\nraw:\n%s",
			final.Status, final.ErrorCode, final.ErrorMessage, sub.FormatRawEvents())
	}

	tcID, ok := th.ExtractToolCallByName(final.Blocks, "activate_skill")
	if !ok {
		t.Fatalf("no activate_skill tool_call in final blocks\nraw:\n%s", sub.FormatRawEvents())
	}
	resultData, ok := th.ExtractToolResultByCallID(final.Blocks, tcID)
	if !ok {
		t.Fatalf("no paired tool_result for activate_skill call %q", tcID)
	}
	if okFlag, _ := resultData["ok"].(bool); !okFlag {
		t.Errorf("activate_skill tool_result.ok=false; data=%v", resultData)
	}
	resultText, _ := resultData["result"].(string)
	if !strings.Contains(resultText, "Review PR #1234") {
		t.Errorf("activate_skill result lacks $1 substitution: %q", resultText)
	}
	if !strings.Contains(resultText, "gh pr view 1234") {
		t.Errorf("activate_skill result lacks step body: %q", resultText)
	}
}

func TestSkill_Search_Then_Activate_E2E(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"search_skills", "call_search_1",
		`{"query":"deploy","summary":"finding deploy skill"}`,
	))
	fake.PushScript(th.ScriptSingleToolCall(
		"activate_skill", "call_act_2",
		`{"name":"deploy","arguments":["staging"],"summary":"activating deploy"}`,
	))
	fake.PushScript(th.ScriptText("Deploy steps loaded for staging."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")

	seedSkill(t, h, "deploy",
		`name: deploy
description: Deploy via internal CI
arguments:
  - environment`,
		`# Deploy to $1
make deploy-$1`)

	conv := h.NewConversation(t, "skill-search-activate")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "I want to deploy")

	final := sub.WaitForAssistantTerminal(60 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errMsg=%q\nraw:\n%s", final.Status, final.ErrorMessage, sub.FormatRawEvents())
	}

	searchID, ok := th.ExtractToolCallByName(final.Blocks, "search_skills")
	if !ok {
		t.Fatalf("no search_skills tool_call in final blocks")
	}
	searchResult, ok := th.ExtractToolResultByCallID(final.Blocks, searchID)
	if !ok {
		t.Fatalf("no paired tool_result for search_skills")
	}
	searchText, _ := searchResult["result"].(string)
	var rows []map[string]any
	if err := json.Unmarshal([]byte(searchText), &rows); err != nil {
		t.Fatalf("search_skills result not JSON list: %v\nresult: %s", err, searchText)
	}
	if len(rows) < 1 || rows[0]["name"] != "deploy" {
		t.Errorf("search_skills did not surface 'deploy': %v", rows)
	}

	actID, ok := th.ExtractToolCallByName(final.Blocks, "activate_skill")
	if !ok {
		t.Fatalf("no activate_skill tool_call in final blocks")
	}
	actResult, ok := th.ExtractToolResultByCallID(final.Blocks, actID)
	if !ok {
		t.Fatalf("no paired tool_result for activate_skill")
	}
	actText, _ := actResult["result"].(string)
	if !strings.Contains(actText, "Deploy to staging") {
		t.Errorf("activate_skill result lacks $1 substitution: %q", actText)
	}
	if !strings.Contains(actText, "make deploy-staging") {
		t.Errorf("activate_skill result lacks body: %q", actText)
	}
}

func TestSkill_PreApproval_BashAfterActivate(t *testing.T) {
	fake := th.NewFakeLLMServer(t)
	fake.PushScript(th.ScriptSingleToolCall(
		"activate_skill", "call_act_3",
		`{"name":"hello-runner","summary":"loading hello-runner"}`,
	))
	fake.PushScript(th.ScriptSingleToolCall(
		"Bash", "call_bash_1",
		`{"command":"echo hello-from-skill","summary":"running echo step"}`,
	))
	fake.PushScript(th.ScriptText("All steps complete."))

	h := th.New(t, th.WithFakeLLMBaseURL(fake.URL()))
	h.SeedDeepSeek(t, "fake-test-key")

	seedSkill(t, h, "hello-runner",
		`name: hello-runner
description: Print hello via echo
allowed-tools:
  - Bash(echo *)`,
		`# Run echo
Just an echo demo.`)

	conv := h.NewConversation(t, "skill-preapproval")
	sub := h.SubscribeSSE(t, conv.ID)

	th.PostMessage(t, h, conv.ID, "Run hello-runner")

	final := sub.WaitForAssistantTerminal(60 * time.Second)
	if final.Status != chatdomain.StatusCompleted {
		t.Fatalf("status=%q errMsg=%q\nraw:\n%s", final.Status, final.ErrorMessage, sub.FormatRawEvents())
	}

	bashID, ok := th.ExtractToolCallByName(final.Blocks, "Bash")
	if !ok {
		t.Fatalf("no Bash tool_call in final blocks\nraw:\n%s", sub.FormatRawEvents())
	}
	bashResult, ok := th.ExtractToolResultByCallID(final.Blocks, bashID)
	if !ok {
		t.Fatalf("no paired tool_result for Bash")
	}
	if okFlag, _ := bashResult["ok"].(bool); !okFlag {
		t.Errorf("Bash tool_result.ok=false; pre-approval should have allowed echo. data=%v", bashResult)
	}
	bashOutput, _ := bashResult["result"].(string)
	if !strings.Contains(bashOutput, "hello-from-skill") {
		t.Errorf("Bash output lacks echoed text; pre-approval may not have actually allowed the run.\noutput: %q", bashOutput)
	}

	var runCount int64
	if err := h.DB.Raw(
		`SELECT COUNT(*) FROM messages
		 WHERE conversation_id = ? AND attrs != ''
		   AND json_extract(attrs, '$.kind') = 'subagent_run'`,
		conv.ID,
	).Scan(&runCount).Error; err != nil {
		t.Fatalf("query subagent runs: %v", err)
	}
	if runCount != 0 {
		t.Errorf("subagent runs = %d for non-fork skill activate; want 0", runCount)
	}
}
