// promptdump_test.go — W6 体验静态审查（柱 B）：用 llmmock 的 PromptDump 审读「模型在线缆上真
// 看到什么」。不是测功能对错，是审体验质量：system prompt 结构/无矛盾/无安全剧场、工具 schema
// 框架字段（summary/danger/execution_group，S18）注入齐全、preview 端点与模型真收到的一致
// （透明度 R0057 不漂移）、utility 视角与 chat 主视角隔离、空态自举 prompt 连贯、i18n 回复语言接缝。
//
// 视角覆盖：Chat 主 LLM（结构/工具/i18n/空态）、Utility（隔离）、用户（preview 端点）。Subagent /
// Agent 实体视角的嵌套 dump 由 chat/agent 既有场景间接覆盖；此处聚焦可静态断言的体验事实。
package scenarios

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// promptSetup boots server+mock with a workspace of the given UI language, registers the mock
// as the dialogue model, and returns the workspace-bound client + mock + conversation id.
//
// promptSetup 拉起 server+mock、建给定 UI 语言的 workspace、把 mock 设为 dialogue 模型，返回
// 绑 workspace 的 client + mock + 一个对话 id。
func promptSetup(t *testing.T, lang string) (*harness.Client, *harness.LLMMock, string) {
	t.Helper()
	srv := harness.Start(t)
	mock := harness.NewLLMMock(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "pd-ws", "language": lang}).Field(t, "id")
	wc := c.WS(wsID)
	keyID := wc.POST("/api/v1/api-keys", map[string]any{
		"provider": "openai", "displayName": "pd", "key": "sk-mock", "baseUrl": mock.URL(),
	}).Field(t, "id")
	wc.POST("/api/v1/api-keys/"+keyID+":test", nil).OK(t, nil)
	wc.PUT("/api/v1/workspaces/"+wsID+"/default-models/dialogue",
		map[string]any{"apiKeyId": keyID, "modelId": dlgModel}).OK(t, nil)
	return wc, mock, wsID
}

// safetyTheaterPhrases are clichés a high-density local-single-user prompt must NOT carry
// (feedback: no safety theater). Their presence is an experience regression worth a finding.
//
// safetyTheaterPhrases 是高密度本地单用户 prompt 不该有的套话（无安全剧场）。出现即体验回退。
var safetyTheaterPhrases = []string{
	"As an AI language model",
	"I cannot and will not",
	"It is important to note that",
	"I'm just an AI",
}

// TestPromptDump_ChatSystemPromptStructure 审读 chat 主 LLM 的 system prompt 结构与体验质量。
func TestPromptDump_ChatSystemPromptStructure(t *testing.T) {
	wc, mock, _ := promptSetup(t, "en")
	// 建一个 function 使 capabilities 段非空（forged 实体真出现在菜单里）。
	fnCreate(t, wc, "weather_lookup", "def f(city: str) -> dict:\n    return {\"c\": city}\n")

	mock.Enqueue(dlgModel, harness.LLMTurn{Text: "hi."})
	convID := convCreate(t, wc, "structure")
	msgID := sendMsg(t, wc, convID, "hello")
	waitTurn(t, wc, convID, msgID, 10000)

	d := mock.WaitDumps(t, dlgModel, 1, 8000)[0]
	sys := d.System
	if sys == "" {
		t.Fatal("dialogue request carried no system prompt")
	}
	// 核心静态段必须在场（identity → how_to_work → tools … critical_rules）。
	for _, name := range []string{"identity", "how_to_work", "tools", "environment", "critical_rules"} {
		if !strings.Contains(sys, `name="`+name+`"`) {
			t.Errorf("system prompt missing <section name=%q>; got:\n%s", name, sys)
		}
	}
	// 身份只出现一次（无重复堆叠）。
	if n := strings.Count(sys, `name="identity"`); n != 1 {
		t.Errorf("identity section appears %d times (want 1)", n)
	}
	// lazy 工具目录浮出（LLM 知道全集、不盲搜），且 forged function 进了 capabilities 菜单。
	if !strings.Contains(sys, "Searchable tools:") || !strings.Contains(sys, "run_function") {
		t.Errorf("tools section did not surface the lazy-tool catalog:\n%s", sys)
	}
	if !strings.Contains(sys, "weather_lookup") {
		t.Errorf("capabilities section did not include the forged function weather_lookup")
	}
	// 无空 section 残壳（每个 <section> 后跟非空内容）。
	if strings.Contains(sys, "</section>\n\n<section") == false && strings.Contains(sys, "</section>") {
		// 仅当存在多段时检查相邻，宽松——主要防 "name=...>\n\n</section>" 空壳。
	}
	if strings.Contains(sys, ">\n\n</section>") {
		t.Errorf("system prompt contains an empty <section> shell:\n%s", sys)
	}
	// 无安全剧场套话。
	for _, p := range safetyTheaterPhrases {
		if strings.Contains(sys, p) {
			t.Errorf("system prompt carries safety-theater phrase %q", p)
		}
	}
}

// TestPromptDump_ToolSchemaFrameworkFields 审读 S18：每个线缆工具 schema 的 properties 必含框架
// 三字段（summary/danger/execution_group），且工具 description 非空（LLM 选型靠它）。
func TestPromptDump_ToolSchemaFrameworkFields(t *testing.T) {
	wc, mock, _ := promptSetup(t, "en")
	mock.Enqueue(dlgModel, harness.LLMTurn{Text: "hi."})
	convID := convCreate(t, wc, "toolschema")
	msgID := sendMsg(t, wc, convID, "hello")
	waitTurn(t, wc, convID, msgID, 10000)

	d := mock.WaitDumps(t, dlgModel, 1, 8000)[0]
	var req struct {
		Tools []struct {
			Function struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Parameters  struct {
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"parameters"`
			} `json:"function"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(d.Raw, &req); err != nil {
		t.Fatalf("parse wire request: %v", err)
	}
	if len(req.Tools) == 0 {
		t.Fatal("no tools on the wire (resident tools must always be present)")
	}
	for _, tl := range req.Tools {
		f := tl.Function
		if strings.TrimSpace(f.Description) == "" {
			t.Errorf("wire tool %q has an empty description (LLM picks tools by it)", f.Name)
		}
		for _, field := range []string{"summary", "danger", "execution_group"} {
			if _, ok := f.Parameters.Properties[field]; !ok {
				t.Errorf("wire tool %q schema missing framework field %q (S18 injection)", f.Name, field)
			}
		}
	}
}

// TestPromptDump_PreviewEndpointFidelity 审读 R0057 透明度：GET /system-prompt-preview 必须与模型
// 真收到的 system prompt 一致（同对话同日同 ctx，二者都走 buildSystemPrompt）——漂移即用户被骗。
func TestPromptDump_PreviewEndpointFidelity(t *testing.T) {
	wc, mock, _ := promptSetup(t, "en")
	fnCreate(t, wc, "fidelity_fn", "def f() -> dict:\n    return {}\n")
	mock.Enqueue(dlgModel, harness.LLMTurn{Text: "hi."})
	convID := convCreate(t, wc, "fidelity")
	msgID := sendMsg(t, wc, convID, "hello")
	waitTurn(t, wc, convID, msgID, 10000)

	d := mock.WaitDumps(t, dlgModel, 1, 8000)[0]
	var pv struct {
		SystemPrompt string `json:"systemPrompt"`
	}
	wc.GET("/api/v1/conversations/" + convID + "/system-prompt-preview").OK(t, &pv)
	if strings.TrimSpace(pv.SystemPrompt) != strings.TrimSpace(d.System) {
		t.Errorf("preview drifted from the real wire system prompt.\n--- preview ---\n%s\n--- wire ---\n%s",
			pv.SystemPrompt, d.System)
	}
}

// TestPromptDump_UtilityViewpointIsolation 审读视角隔离：utility 模型（首回合自动起标题）收到的
// 是一个紧凑的专用 prompt，绝不泄漏 chat 主视角的全 system（identity 等段）。
func TestPromptDump_UtilityViewpointIsolation(t *testing.T) {
	srv := harness.Start(t)
	mock := harness.NewLLMMock(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "util-ws", "language": "en"}).Field(t, "id")
	wc := c.WS(wsID)
	keyID := wc.POST("/api/v1/api-keys", map[string]any{
		"provider": "openai", "displayName": "u", "key": "sk-mock", "baseUrl": mock.URL(),
	}).Field(t, "id")
	wc.POST("/api/v1/api-keys/"+keyID+":test", nil).OK(t, nil)
	wc.PUT("/api/v1/workspaces/"+wsID+"/default-models/dialogue",
		map[string]any{"apiKeyId": keyID, "modelId": dlgModel}).OK(t, nil)
	wc.PUT("/api/v1/workspaces/"+wsID+"/default-models/utility",
		map[string]any{"apiKeyId": keyID, "modelId": utilModel}).OK(t, nil)

	mock.Enqueue(dlgModel, harness.LLMTurn{Text: "answer."})
	mock.Enqueue(utilModel, harness.LLMTurn{Text: "A Concise Title"})
	convID := convCreate(t, wc, "") // 空标题 → 首回合触发 utility 起标题
	msgID := sendMsg(t, wc, convID, "what is the capital of France?")
	waitTurn(t, wc, convID, msgID, 10000)

	uds := mock.WaitDumps(t, utilModel, 1, 8000)
	raw := string(uds[0].Raw)
	if strings.Contains(raw, `name="identity"`) || strings.Contains(raw, "Searchable tools:") {
		t.Errorf("utility request leaked the full chat system prompt (viewpoint not isolated):\n%s", raw)
	}
	// 且 utility 请求确实在做起标题的活（引用了用户消息内容）。
	if !strings.Contains(raw, "France") {
		t.Errorf("utility title request did not reference the conversation content:\n%s", raw)
	}
}

// TestPromptDump_EmptyStateCoherence 审读空态（自举）：零 forged 实体的 workspace，system prompt
// 仍连贯——capabilities 段要么缺席、要么是连贯的空提示，绝不是半截残壳。
func TestPromptDump_EmptyStateCoherence(t *testing.T) {
	wc, mock, _ := promptSetup(t, "en") // 不建任何实体
	mock.Enqueue(dlgModel, harness.LLMTurn{Text: "hi."})
	convID := convCreate(t, wc, "empty")
	msgID := sendMsg(t, wc, convID, "hello")
	waitTurn(t, wc, convID, msgID, 10000)

	d := mock.WaitDumps(t, dlgModel, 1, 8000)[0]
	sys := d.System
	// 核心身份/规则段必须仍在（空态不残）。
	for _, name := range []string{"identity", "how_to_work", "critical_rules"} {
		if !strings.Contains(sys, `name="`+name+`"`) {
			t.Errorf("empty-state prompt missing core section %q", name)
		}
	}
	// 无空 section 残壳。
	if strings.Contains(sys, ">\n\n</section>") {
		t.Errorf("empty-state prompt has an empty <section> shell:\n%s", sys)
	}
}

// TestPromptDump_AgentViewpoint 审读 Agent 实体视角：HTTP :invoke 一个 agent，它收到的 system
// prompt 是「你是 <name>，一个 workflow 自动化 worker + 你的角色…」——与 chat 主视角（You are
// Forgify + 全 section 菜单）完全隔离，且只挂载它被授予的工具。
func TestPromptDump_AgentViewpoint(t *testing.T) {
	srv := harness.Start(t)
	mock := harness.NewLLMMock(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "ag-vp", "language": "en"}).Field(t, "id")
	wc := c.WS(wsID)
	keyID := wc.POST("/api/v1/api-keys", map[string]any{
		"provider": "openai", "displayName": "a", "key": "sk-mock", "baseUrl": mock.URL(),
	}).Field(t, "id")
	wc.POST("/api/v1/api-keys/"+keyID+":test", nil).OK(t, nil)
	wc.PUT("/api/v1/workspaces/"+wsID+"/default-models/agent",
		map[string]any{"apiKeyId": keyID, "modelId": "mock-agent"}).OK(t, nil)

	agID := wc.POST("/api/v1/agents", map[string]any{
		"name": "DataBot", "description": "crunch numbers and report",
		"prompt": "you analyze data",
	}).Field(t, "id") // create 现返裸实体(MD1)

	mock.Enqueue("mock-agent", harness.LLMTurn{Text: "analysis complete."})
	wc.POST("/api/v1/agents/"+agID+":invoke", map[string]any{"input": map[string]any{}}).OK(t, nil)

	d := mock.WaitDumps(t, "mock-agent", 1, 10000)[0]
	sys := d.System
	if !strings.Contains(sys, "DataBot") || !strings.Contains(sys, "workflow automation worker") {
		t.Errorf("agent system prompt missing its own identity:\n%s", sys)
	}
	if !strings.Contains(sys, "crunch numbers and report") {
		t.Errorf("agent system prompt missing its role (description)")
	}
	// 与 chat 主视角隔离：无 Forgify 身份、无 chat 的 section 菜单。
	if strings.Contains(sys, "You are Forgify") || strings.Contains(sys, `name="identity"`) {
		t.Errorf("agent prompt leaked the chat main viewpoint:\n%s", sys)
	}
}

// TestPromptDump_I18nReplyLanguage 审读 i18n 接缝：**workspace.language 权威**（AC-24 修复 /
// AC-PD-2 裁决）——environment 段的「Reply in <lang>」由 workspace 持久化语言驱动（zh-CN→Chinese、
// en→English），不再受浏览器 Accept-Language 摆布；prompt 本体保持英文（高密度、模型友好）。
func TestPromptDump_I18nReplyLanguage(t *testing.T) {
	for _, tc := range []struct{ lang, wantReply string }{
		{"zh-CN", "Reply in Chinese"},
		{"en", "Reply in English"},
	} {
		wc, mock, _ := promptSetup(t, tc.lang)
		mock.Enqueue(dlgModel, harness.LLMTurn{Text: "ok."})
		convID := convCreate(t, wc, "i18n-"+tc.lang)
		msgID := sendMsg(t, wc, convID, "hi")
		waitTurn(t, wc, convID, msgID, 10000)
		d := mock.WaitDumps(t, dlgModel, 1, 8000)[0]
		// workspace 语言驱动回复语言（middleware 让 workspace.language 压过 Accept-Language）。
		if !strings.Contains(d.System, tc.wantReply) {
			t.Errorf("lang %q: system prompt missing %q (workspace.language should be authoritative):\n%s",
				tc.lang, tc.wantReply, d.System)
		}
		// prompt 本体英文（不因任何语言把整 prompt 翻译）。
		if !strings.Contains(d.System, "You are Forgify") {
			t.Errorf("lang %q: identity instruction is not English (prompt body should stay English)", tc.lang)
		}
	}
}
