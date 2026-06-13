// Package golden holds the real-model LLM journeys (make evals): the same black-box harness,
// but the model is REAL (deepseek-v4-flash) — gated behind EVALS=1 so the suite never burns
// tokens by accident. These are 柱C of the acceptance program: prove the product's tool surface
// really drives a real model end to end. Assertions check OUTCOMES (entity created, function ran,
// memory recalled) not exact text — a real model is non-deterministic, so we judge "did it reach
// the goal state", never "did it say these words".
//
// Package golden 放真模型 LLM 旅程（make evals）：同一套黑盒 harness，但模型是真的
// （deepseek-v4-flash）——EVALS=1 门控，绝不意外烧钱。验收计划柱C：证明产品工具面真能端到端驱动真
// 模型。断言只看**结果状态**（实体建了、function 跑了、memory 记住了），不看逐字文本——真模型非
// 确定，只判"是否到达目标态"。
package golden

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

func TestMain(m *testing.M) {
	if os.Getenv("EVALS") == "" {
		os.Exit(0) // gated: only runs via make evals. 门控：仅 make evals 触发。
	}
	os.Exit(m.Run())
}

// realModel resolves the real-model wire config from the environment. EVALS_* win; otherwise
// fall back to deepseek (key from DEEPSEEK_API_KEY, the repo-root .env name). Empty key → skip.
//
// realModel 从环境解析真模型线缆配置。EVALS_* 优先；否则落 deepseek（key 取 DEEPSEEK_API_KEY，
// 仓库根 .env 的名字）。key 空 → skip。
func realModel(t *testing.T) (baseURL, model, key string) {
	t.Helper()
	key = firstNonEmpty(os.Getenv("EVALS_KEY"), os.Getenv("DEEPSEEK_API_KEY"))
	if key == "" {
		t.Skip("no real-model key (set DEEPSEEK_API_KEY or EVALS_KEY); make evals loads repo-root .env")
	}
	baseURL = firstNonEmpty(os.Getenv("EVALS_BASE_URL"), "https://api.deepseek.com")
	model = firstNonEmpty(os.Getenv("EVALS_MODEL"), "deepseek-v4-flash")
	return baseURL, model, key
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// evalWS boots a server, registers the real model as an openai-format key, probes it, and sets
// it as the default for the requested scenarios. Returns the workspace-bound client.
//
// evalWS 拉起 server、把真模型注册成 openai 格式 key、探活、设为所点 scenario 的默认，返回绑
// workspace 的 client。
func evalWS(t *testing.T, scenarios ...string) *harness.Client {
	t.Helper()
	baseURL, model, key := realModel(t)
	srv := harness.Start(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "eval-ws", "language": "en"}).Field(t, "id")
	wc := c.WS(wsID)
	// provider 用 "deepseek"（真实用户的选法）：窗口/能力静态表按 (provider, modelID)
	// 命中（1M/384k）——压缩等预算敏感链路才有已知 budget；openai+裸 baseURL 会查不到
	// 窗口、压缩按设计盲禁。
	keyID := wc.POST("/api/v1/api-keys", map[string]any{
		"provider": "deepseek", "displayName": "deepseek", "key": key, "baseUrl": baseURL,
	}).Field(t, "id")
	wc.POST("/api/v1/api-keys/"+keyID+":test", nil).OK(t, nil)
	if len(scenarios) == 0 {
		scenarios = []string{"dialogue", "utility", "agent"}
	}
	for _, sc := range scenarios {
		wc.PUT("/api/v1/workspaces/"+wsID+"/default-models/"+sc,
			map[string]any{"apiKeyId": keyID, "modelId": model}).OK(t, nil)
	}
	return wc
}

// evalMsg is one turn in GET /conversations/{id}/messages (minimal wire shape; golden is a
// separate package and shares no helpers with scenarios).
//
// evalMsg 是消息历史里一个回合（最小线缆形状；golden 独立包、不与 scenarios 共享 helper）。
type evalMsg struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	Status     string `json:"status"`
	StopReason string `json:"stopReason"`
	ErrorCode  string `json:"errorCode"`
	Blocks     []struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	} `json:"blocks"`
}

// drainInteractions auto-resolves any human-in-the-loop gate so eval journeys never hang: a real
// model may self-report a tool as dangerous (→ approve_always, whitelisting it for the rest of the
// run) or call ask_user (→ accept with a generic go-ahead). Best-effort; a resolve that 404s
// (already handled) is fine.
//
// drainInteractions 自动放行任何人在环门，使金标旅程不挂：真模型可能自报工具危险（→approve_always、
// 本次运行白名单它）或调 ask_user（→accept + 通用放行答复）。best-effort；resolve 404（已处理）无妨。
func drainInteractions(wc *harness.Client, convID string) {
	r, err := wc.Try("GET", "/api/v1/conversations/"+convID+"/interactions", nil)
	if err != nil || r.Status != 200 || len(r.Data) == 0 {
		return
	}
	var pend []struct {
		ToolCallID string `json:"toolCallId"`
		Kind       string `json:"kind"`
	}
	if json.Unmarshal(r.Data, &pend) != nil {
		return
	}
	for _, p := range pend {
		action, answer := "approve_always", ""
		if p.Kind == "ask" {
			action, answer = "accept", "Yes, proceed with a sensible default."
		}
		_, _ = wc.Try("POST", "/api/v1/conversations/"+convID+"/interactions/"+p.ToolCallID,
			map[string]any{"action": action, "answer": answer})
	}
}

// say sends a user message and waits for the assistant turn to reach a terminal state, returning
// the concatenated text of that turn. Real-model turns can take a while (multi-step tool loops),
// hence the generous timeout; human-loop gates are auto-resolved each poll so a danger/ask gate
// never stalls the journey.
//
// say 发一条用户消息并等 assistant 回合到终态，返回该回合文本拼接。真模型回合可能较久（多步工具
// 循环），故超时给得宽；每轮自动放行人在环门，使 danger/ask 门绝不卡住旅程。
func say(t *testing.T, wc *harness.Client, convID, content string, timeoutMS int) string {
	t.Helper()
	msgID := wc.POST("/api/v1/conversations/"+convID+"/messages",
		map[string]any{"content": content}).Field(t, "messageId")
	var text string
	harness.Eventually(t, timeoutMS, "assistant turn reaches terminal", func() bool {
		drainInteractions(wc, convID)
		var msgs []evalMsg
		wc.GET("/api/v1/conversations/" + convID + "/messages?limit=80").OK(t, &msgs)
		for _, m := range msgs {
			if m.ID != msgID {
				continue
			}
			if m.Status == "pending" || m.Status == "streaming" {
				return false
			}
			var b strings.Builder
			for _, blk := range m.Blocks {
				if blk.Type == "text" {
					b.WriteString(blk.Content)
				}
			}
			text = b.String()
			return true
		}
		return false
	})
	return text
}

func newConv(t *testing.T, wc *harness.Client, title string) string {
	t.Helper()
	return wc.POST("/api/v1/conversations", map[string]any{"title": title}).Field(t, "id")
}

// ── J1 自举引导：空 workspace 的第一句对话 ─────────────────────────────────────
// 真模型在零实体、有完整工具面的 workspace 里对一个开放问题给出连贯、非报错的引导。
func TestGolden_J1_Bootstrap(t *testing.T) {
	wc := evalWS(t)
	conv := newConv(t, wc, "getting started")
	out := say(t, wc, conv, "I'm new here. In one short paragraph, what can you help me build?", 90000)
	if strings.TrimSpace(out) == "" {
		t.Fatal("bootstrap turn produced no text")
	}
}

// ── J2 旗舰：从零建 function 并调通 ───────────────────────────────────────────
// 真模型必须 create_function 再 run_function——结果状态：functions 列出 ≥1，且最终答复报出和 5。
func TestGolden_J2_BuildAndRunFunction(t *testing.T) {
	wc := evalWS(t)
	conv := newConv(t, wc, "build add")
	out := say(t, wc, conv,
		"Create a Python function named add that takes two integers a and b and returns a+b. "+
			"Then run it with a=2 and b=3 and tell me the result.", 180000)

	var fns []json.RawMessage
	wc.GET("/api/v1/functions").OK(t, &fns)
	if len(fns) == 0 {
		t.Fatalf("model did not create any function (create_function not driven); answer was:\n%s", out)
	}
	if !strings.Contains(out, "5") {
		t.Errorf("model created a function but final answer lacks the result 5 (run_function may not have driven):\n%s", out)
	}
}

// ── J5 AI 自愈：埋雷 function 让模型诊断修好 ─────────────────────────────────
// 预置一个会抛错的 function，请模型修；结果状态：active 版本号前进（>1，说明 edit_function 真改了）。
func TestGolden_J5_DebugFunction(t *testing.T) {
	wc := evalWS(t)
	// 预置 bug：引用未定义变量。create 现返裸实体(MD1):data 顶层即 id。
	fnID := wc.POST("/api/v1/functions", map[string]any{
		"name": "buggy_double", "description": "double a number (has a bug)",
		"code": "def buggy_double(n: int) -> dict:\n    return {\"out\": n * undefined_factor}\n",
	}).Field(t, "id")

	conv := newConv(t, wc, "fix bug")
	say(t, wc, conv,
		"The function buggy_double is broken — it references an undefined variable. "+
			"Fix it so it returns n doubled (n*2), then verify it works on n=4.", 180000)

	// active 版本前进 = edit_function 真落了新版本。
	var versions []struct {
		Version int `json:"version"`
	}
	wc.GET("/api/v1/functions/" + fnID + "/versions").OK(t, &versions)
	maxV := 0
	for _, v := range versions {
		if v.Version > maxV {
			maxV = v.Version
		}
	}
	if maxV < 2 {
		t.Fatalf("model did not produce a new version (edit_function not driven); max version=%d", maxV)
	}
}

// ── J3 常驻服务：从零建 handler 并调其方法 ───────────────────────────────────
// 真模型 create_handler（有状态服务）再 call_handler；结果状态：handlers 列出 ≥1。
func TestGolden_J3_BuildAndCallHandler(t *testing.T) {
	wc := evalWS(t)
	conv := newConv(t, wc, "build handler")
	say(t, wc, conv,
		"Create a handler named Greeter with a method 'hello' that takes a name string and returns "+
			"a dict {\"msg\": \"Hello, <name>!\"}. Then call hello with name='Ada' and tell me what it returned.",
		240000)

	var handlers []json.RawMessage
	wc.GET("/api/v1/handlers").OK(t, &handlers)
	if len(handlers) == 0 {
		t.Fatal("model did not create any handler (create_handler not driven)")
	}
}

// ── J7 积木检索：搜到一个已锻造的 function ───────────────────────────────────
// 预置一个 function，请真模型用搜索找到它并报出确切名字（驱动 search_tools/search_blocks）。
func TestGolden_J7_SearchBuildingBlocks(t *testing.T) {
	wc := evalWS(t)
	wc.POST("/api/v1/functions", map[string]any{
		"name": "celsius_to_fahrenheit", "description": "convert a Celsius temperature to Fahrenheit",
		"code": "def celsius_to_fahrenheit(c: float) -> dict:\n    return {\"f\": c * 9 / 5 + 32}\n",
	}).OK(t, nil)

	conv := newConv(t, wc, "find block")
	out := say(t, wc, conv,
		"I forged a function earlier that converts Celsius to Fahrenheit but I forget its exact name. "+
			"Search my workspace and tell me its exact name.", 180000)
	if !strings.Contains(out, "celsius_to_fahrenheit") {
		t.Errorf("model did not find the forged function by search:\n%s", out)
	}
}

// ── J9 记忆：写入一条 memory，新对话里召回 ───────────────────────────────────
// 真模型在对话 A 写 memory（write_memory），对话 B（全新、靠 system prompt 注入的 memory）召回。
func TestGolden_J9_MemoryWriteRecall(t *testing.T) {
	wc := evalWS(t)
	a := newConv(t, wc, "tell")
	say(t, wc, a, "Please remember this for later: my project's deploy target is codename Polaris.", 120000)

	// memory 真落库（write_memory 驱动）。
	var mems []json.RawMessage
	wc.GET("/api/v1/memories").OK(t, &mems)
	if len(mems) == 0 {
		t.Fatal("model did not persist any memory (write_memory not driven)")
	}

	// 新对话召回（memory 经 system prompt 注入）。
	b := newConv(t, wc, "recall")
	out := say(t, wc, b, "What is my project's deploy target codename?", 120000)
	if !strings.Contains(strings.ToLower(out), "polaris") {
		t.Errorf("model did not recall the memory in a fresh conversation:\n%s", out)
	}
}

// ── J12 降级态：utility 未配，主对话链路（dialogue）仍完成 ───────────────────
// 只配 dialogue（不配 utility）——起标题/压缩静默缺席，但主问答照常完成、不报错。
func TestGolden_J12_DegradedMainPath(t *testing.T) {
	wc := evalWS(t, "dialogue") // 仅 dialogue
	conv := newConv(t, wc, "degraded")
	out := say(t, wc, conv, "In one sentence, what is durable workflow execution?", 90000)
	if strings.TrimSpace(out) == "" {
		t.Fatal("degraded main path produced no answer")
	}
}
