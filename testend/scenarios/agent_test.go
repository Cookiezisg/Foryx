// agent_test.go — R2（A4 Agent 整域，首轮零覆盖的补课）。
//
// PLAN A4 逐格：挂载三类（fn_ / hd_.method / mcp:server/tool）真合成专属绑定工具且真调通
// （工具宇宙=挂载、永不见系统工具）；改名按现名重解析、挂载物被删 invoke 大声失败（fail-fast）、
// ag_ 拒挂、合成撞名拒；invoke 三入口（HTTP :invoke / chat invoke_agent 嵌套块流 / workflow
// agent 节点）+ 执行台账溯源；transcript 自包含落库；prompt 组装（身份/skill 指南/outputs
// 硬约束/knowledge 前缀）；modelOverride 优先级以独立 mock 队列物理证明。
package scenarios

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

const agModel = "mock-agent" // agent 场景默认模型的独立队列。

// agentSetup 拉起 server+mock，配 dialogue（chat 入口用）与 agent 两场景默认模型。
func agentSetup(t *testing.T) (*harness.Client, *harness.LLMMock) {
	t.Helper()
	srv := harness.Start(t)
	mock := harness.NewLLMMock(t)
	c := srv.Client(t)
	wsID := c.POST("/api/v1/workspaces", map[string]any{"name": "agent-ws"}).Field(t, "id")
	wc := c.WS(wsID)
	keyID := wc.POST("/api/v1/api-keys", map[string]any{
		"provider": "openai", "displayName": "llmmock", "key": "sk-mock", "baseUrl": mock.URL(),
	}).Field(t, "id")
	wc.POST("/api/v1/api-keys/"+keyID+":test", nil).OK(t, nil)
	wc.PUT("/api/v1/workspaces/"+wsID+"/default-models/dialogue",
		map[string]any{"apiKeyId": keyID, "modelId": dlgModel}).OK(t, nil)
	wc.PUT("/api/v1/workspaces/"+wsID+"/default-models/agent",
		map[string]any{"apiKeyId": keyID, "modelId": agModel}).OK(t, nil)
	return wc, mock
}

// agCreate 建 agent（全量 Config 快照 = v1）返回 id。
func agCreate(t *testing.T, wc *harness.Client, body map[string]any) string {
	t.Helper()
	return nestedID(t, wc.POST("/api/v1/agents", body), "agent")
}

// invokeResult 是 :invoke 的线缆形状。
type invokeResult struct {
	ExecutionID string `json:"executionId"`
	OK          bool   `json:"ok"`
	Output      any    `json:"output"`
	Status      string `json:"status"`
	StopReason  string `json:"stopReason"`
	Steps       int    `json:"steps"`
	ErrorMsg    string `json:"errorMsg"`
}

func agInvoke(t *testing.T, wc *harness.Client, agID string, input map[string]any) invokeResult {
	t.Helper()
	var res invokeResult
	wc.POST("/api/v1/agents/"+agID+":invoke", map[string]any{"input": input}).OK(t, &res)
	return res
}

// fw 给脚本工具调用补 S18 框架字段（Framework 注入 schema、执行前剥离）。
func fw(args map[string]any) map[string]any {
	out := map[string]any{"summary": "scripted", "danger": "safe", "execution_group": 1}
	for k, v := range args {
		out[k] = v
	}
	return out
}

// TestAgentR2_MountSynthesisThreeKindsAndLedger: 核心格——三类挂载真合成绑定工具、
// agent 线缆工具集恰为挂载（无任何系统工具）、三工具真调通、四处台账（function 执行 /
// handler call / mcp call 各带 TriggeredBy=agent + agent 执行行带 transcript）。
func TestAgentR2_MountSynthesisThreeKindsAndLedger(t *testing.T) {
	wc, mock := agentSetup(t)

	fnID := fnCreate(t, wc, "tally_votes",
		"def tally_votes(n: int) -> dict:\n    return {\"tally\": n * 2}\n")
	hdID := hdCreate(t, wc, "greeter", map[string]any{
		"description": "stateful greeter", "initBody": "self.n = 0",
		"methods": []map[string]any{
			// 方法输入是具名参数（def hello(self, who)）。
			{"name": "hello", "body": "self.n += 1\nreturn {\"msg\": f\"hi {who}\", \"count\": self.n}",
				"description": "greet someone", "inputs": []map[string]any{{"name": "who", "type": "string"}}},
		},
	})
	script := writeScriptedMCP(t)
	wc.PUT("/api/v1/mcp-servers/agmcp", map[string]any{
		"description": "agent mount probe", "command": "python3", "args": []string{script},
	}).OK(t, nil)

	agID := agCreate(t, wc, map[string]any{
		"name": "Ops Worker", "description": "runs the mounted trio", "prompt": "Do the task with your tools.",
		"tools": []map[string]any{
			{"ref": fnID, "name": "tally_votes"},
			{"ref": hdID + ".hello", "name": "greeter hello"},
			{"ref": "mcp:agmcp/echo", "name": "agmcp echo"},
		},
	})

	mock.Enqueue(agModel,
		harness.LLMTurn{ToolCalls: []harness.MockToolCall{{Name: "tally_votes", Args: fw(map[string]any{"n": 21})}}},
		harness.LLMTurn{ToolCalls: []harness.MockToolCall{{Name: "greeter__hello", Args: fw(map[string]any{"who": "ada"})}}},
		harness.LLMTurn{ToolCalls: []harness.MockToolCall{{Name: "mcp__agmcp__echo", Args: fw(map[string]any{"text": "ping"})}}},
		harness.LLMTurn{Text: "all three done"},
	)
	res := agInvoke(t, wc, agID, map[string]any{"n": 21})
	if !res.OK || res.Status != "ok" {
		t.Fatalf("invoke must complete with ok status, got %+v", res)
	}
	if res.ExecutionID == "" || res.Steps < 3 {
		t.Fatalf("result must carry executionId and >=3 steps, got %+v", res)
	}

	// The model's-eye toolset is EXACTLY the three mounts — no system tools ever.
	// 模型视角工具集恰为三挂载——永无系统工具。
	dumps := mock.DumpsFor(agModel)
	if len(dumps) < 4 {
		t.Fatalf("want 4 agent requests, got %d", len(dumps))
	}
	want := map[string]bool{"tally_votes": true, "greeter__hello": true, "mcp__agmcp__echo": true}
	got := map[string]bool{}
	for _, name := range dumps[0].Tools {
		got[name] = true
	}
	for name := range want {
		if !got[name] {
			t.Fatalf("mounted tool %s missing from agent toolset %v", name, dumps[0].Tools)
		}
	}
	for _, sys := range []string{"run_function", "search_tools", "call_handler", "todo_write"} {
		if got[sys] {
			t.Fatalf("system tool %s leaked into agent toolset %v", sys, dumps[0].Tools)
		}
	}
	// Tool results fed back: function output / handler yield / mcp echo.
	// 工具结果回喂：function 输出 / handler 返回 / mcp echo。
	all := ""
	for _, d := range dumps[1:] {
		for _, m := range d.Messages {
			if m.Role == "tool" {
				all += m.Content + "\n"
			}
		}
	}
	for _, frag := range []string{`"tally":42`, "hi ada", "echo:ping"} {
		if !strings.Contains(all, frag) {
			t.Fatalf("tool feedback missing %q in: %s", frag, all)
		}
	}

	// Cross-domain ledgers: every target records TriggeredBy=agent.
	// 跨域台账：三个目标各记 TriggeredBy=agent。
	var fnPage struct {
		Executions []struct {
			TriggeredBy string `json:"triggeredBy"`
		} `json:"executions"`
	}
	wc.GET("/api/v1/functions/"+fnID+"/executions").OK(t, &fnPage)
	if len(fnPage.Executions) != 1 || fnPage.Executions[0].TriggeredBy != "agent" {
		t.Fatalf("function ledger must say agent: %+v", fnPage.Executions)
	}
	var hdPage struct {
		Calls []struct {
			TriggeredBy string `json:"triggeredBy"`
		} `json:"calls"`
	}
	wc.GET("/api/v1/handlers/"+hdID+"/calls").OK(t, &hdPage)
	if len(hdPage.Calls) != 1 || hdPage.Calls[0].TriggeredBy != "agent" {
		t.Fatalf("handler ledger must say agent: %+v", hdPage.Calls)
	}
	var mcpPage mcpCallsPage
	wc.GET("/api/v1/mcp-servers/agmcp/calls").OK(t, &mcpPage)
	if len(mcpPage.Calls) != 1 || mcpPage.Calls[0].TriggeredBy != "agent" {
		t.Fatalf("mcp ledger must say agent: %+v", mcpPage.Calls)
	}

	// Agent's own ledger: list row + detail carries the self-contained transcript.
	// agent 自己的台账：列表行 + 详情带自包含 transcript。
	var page struct {
		Executions []struct {
			ID          string `json:"id"`
			Status      string `json:"status"`
			TriggeredBy string `json:"triggeredBy"`
		} `json:"executions"`
	}
	wc.GET("/api/v1/agents/"+agID+"/executions").OK(t, &page)
	if len(page.Executions) != 1 || page.Executions[0].TriggeredBy != "manual" || page.Executions[0].Status != "ok" {
		t.Fatalf("agent execution row wrong: %+v", page.Executions)
	}
	var detail struct {
		Transcript json.RawMessage `json:"transcript"`
	}
	wc.GET("/api/v1/agent-executions/"+res.ExecutionID).OK(t, &detail)
	tr := string(detail.Transcript)
	if !strings.Contains(tr, "tool_call") || !strings.Contains(tr, "tally_votes") {
		t.Fatalf("transcript must carry the full block sequence, got: %.300s", tr)
	}
}

// TestAgentR2_RenameReresolutionAndFailFast: 运行时按现名重解析（改名后工具自动换名）、
// 挂载物被删 invoke 大声失败（绝不静默降级）、ag_ 拒挂（员工不调员工）、合成撞名拒。
func TestAgentR2_RenameReresolutionAndFailFast(t *testing.T) {
	wc, mock := agentSetup(t)

	fnID := fnCreate(t, wc, "old_name", "def old_name() -> dict:\n    return {\"v\": 1}\n")
	agID := agCreate(t, wc, map[string]any{
		"name": "Renamer", "description": "probe", "prompt": "do it",
		"tools": []map[string]any{{"ref": fnID, "name": "old_name"}},
	})

	mock.Enqueue(agModel, harness.LLMTurn{Text: "noop"})
	agInvoke(t, wc, agID, nil)
	dumps := mock.DumpsFor(agModel)
	if !hasTool(dumps[len(dumps)-1].Tools, "old_name") {
		t.Fatalf("toolset must carry the function's current name, got %v", dumps[len(dumps)-1].Tools)
	}

	// Rename the function → next invoke synthesizes under the NEW name (ToolRef.Name
	// is just a display snapshot). 改名 → 下次 invoke 以新名合成（ToolRef.Name 只是快照）。
	wc.PATCH("/api/v1/functions/"+fnID, map[string]any{"name": "new_name"}).OK(t, nil)
	mock.Enqueue(agModel, harness.LLMTurn{Text: "noop"})
	agInvoke(t, wc, agID, nil)
	dumps = mock.DumpsFor(agModel)
	last := dumps[len(dumps)-1].Tools
	if !hasTool(last, "new_name") || hasTool(last, "old_name") {
		t.Fatalf("rename must re-resolve to the live name, got %v", last)
	}

	// Delete the mounted function → the run fails loudly as a failed execution
	// (mount resolve is fail-fast; the failure is auditable, never a degraded run).
	// 删挂载物 → 运行落为 failed 执行（fail-fast；失败可审计、绝不降级跑）。
	wc.DELETE("/api/v1/functions/" + fnID).OK(t, nil)
	dangling := agInvoke(t, wc, agID, map[string]any{})
	if dangling.OK || dangling.Status != "failed" || !strings.Contains(dangling.ErrorMsg, "not found") {
		t.Fatalf("dangling mount must fail the run with the cause, got %+v", dangling)
	}

	// ag_ refs are rejected at write time (workers don't call workers).
	// ag_ 在写入时拒（员工不调员工）。
	r := wc.Do("POST", "/api/v1/agents", map[string]any{
		"name": "Boss", "description": "x", "prompt": "x",
		"tools": []map[string]any{{"ref": agID, "name": "subordinate"}},
	})
	if r.Status < 400 || r.Code == "" {
		t.Fatalf("ag_ mount must reject at create: %d %s", r.Status, r.Raw)
	}

	// Two mounts synthesizing one LLM tool name → mount-invalid, not silent last-write-wins.
	// 两挂载合成同名 → 拒，非静默覆盖。
	clashFn := fnCreate(t, wc, "greeter__hello", "def greeter__hello() -> dict:\n    return {}\n")
	clashHd := hdCreate(t, wc, "greeter", map[string]any{
		"description": "clash host", "initBody": "self.n = 0",
		"methods": []map[string]any{
			{"name": "hello", "body": "return {\"ok\": True}", "description": "hello"},
		},
	})
	clashAg := agCreate(t, wc, map[string]any{
		"name": "Clasher", "description": "x", "prompt": "x",
		"tools": []map[string]any{
			{"ref": clashFn, "name": "fn"},
			{"ref": clashHd + ".hello", "name": "hd"},
		},
	})
	clash := agInvoke(t, wc, clashAg, map[string]any{})
	if clash.OK || clash.Status != "failed" || !strings.Contains(clash.ErrorMsg, "collides") {
		t.Fatalf("name collision must fail the run citing the collision, got %+v", clash)
	}
}

func hasTool(tools []string, name string) bool {
	for _, n := range tools {
		if n == name {
			return true
		}
	}
	return false
}

// TestAgentR2_PromptAssembly: 模型视角的组装契约——身份段、skill 执行指南段（不激活不 fork）、
// outputs 硬约束（声明即「答案必须是恰含字段的单个 JSON」）、knowledge 前缀进 user 消息、
// Input JSON 块、且全程无 chat 主视角泄漏。
func TestAgentR2_PromptAssembly(t *testing.T) {
	wc, mock := agentSetup(t)

	docID := wc.POST("/api/v1/documents", map[string]any{
		"name": "ops manual", "content": "knowledgemark: rotate the key every monday.",
	}).Field(t, "id")
	wc.POST("/api/v1/skills", map[string]any{
		"name": "triage_steps", "description": "triage guide",
		"body": "guidemark: always bisect the failure window first.",
	}).OK(t, nil)

	agID := agCreate(t, wc, map[string]any{
		"name": "Auditor", "description": "audits things", "prompt": "Audit the input.",
		"skill":     "triage_steps",
		"knowledge": []string{docID},
		"outputs": []map[string]any{
			{"name": "verdict", "type": "string", "description": "pass or fail"},
		},
	})
	mock.Enqueue(agModel, harness.LLMTurn{Text: `{"verdict":"pass"}`})
	res := agInvoke(t, wc, agID, map[string]any{"target": "build 42"})
	if !res.OK {
		t.Fatalf("invoke must complete, got %+v", res)
	}

	d := mock.DumpsFor(agModel)[0]
	sys := d.System
	if !strings.Contains(sys, "Auditor") || !strings.Contains(sys, "audits things") {
		t.Fatalf("system must carry the worker identity, got: %.400s", sys)
	}
	if !strings.Contains(sys, "Execution guide") || !strings.Contains(sys, "guidemark") {
		t.Fatalf("skill guide section missing from system prompt: %.600s", sys)
	}
	if !strings.Contains(sys, "verdict") || !strings.Contains(sys, "JSON") {
		t.Fatalf("declared outputs must impose the JSON hard constraint: %.600s", sys)
	}
	// Chat-main-view isolation: none of the chat system sections may leak.
	// chat 主视角隔离：chat 专属段不得泄漏。
	for _, leak := range []string{"search_tools", "Forgify", "todo_write"} {
		if strings.Contains(sys, leak) {
			t.Fatalf("chat prompt leaked %q into agent view", leak)
		}
	}
	// Knowledge prefix + input ride the user message. knowledge 前缀 + 输入随 user 消息。
	user := ""
	for _, m := range d.Messages {
		if m.Role == "user" {
			user += m.Content
		}
	}
	if !strings.Contains(user, "knowledgemark") || !strings.Contains(user, "build 42") {
		t.Fatalf("user message must carry knowledge prefix + input JSON, got: %.500s", user)
	}
}

// TestAgentR2_ModelOverridePriority: modelOverride 优先级的物理证明——override 的请求落
// 在它自己的 mock 队列（独立 model id），默认 agent 队列分毫未动。
func TestAgentR2_ModelOverridePriority(t *testing.T) {
	wc, mock := agentSetup(t)

	// 取 setup 建的那把 key（裸数组、列表第一条）。
	var keys []struct {
		ID string `json:"id"`
	}
	wc.GET("/api/v1/api-keys").OK(t, &keys)
	if len(keys) == 0 {
		t.Fatal("setup key missing")
	}

	plain := agCreate(t, wc, map[string]any{
		"name": "Default Worker", "description": "x", "prompt": "x",
	})
	override := agCreate(t, wc, map[string]any{
		"name": "Special Worker", "description": "x", "prompt": "x",
		"modelOverride": map[string]any{"apiKeyId": keys[0].ID, "modelId": "mock-override"},
	})

	mock.Enqueue(agModel, harness.LLMTurn{Text: "from default"})
	mock.Enqueue("mock-override", harness.LLMTurn{Text: "from override"})
	if res := agInvoke(t, wc, plain, nil); !res.OK {
		t.Fatalf("default invoke failed: %+v", res)
	}
	if res := agInvoke(t, wc, override, nil); !res.OK {
		t.Fatalf("override invoke failed: %+v", res)
	}
	if n := len(mock.DumpsFor(agModel)); n != 1 {
		t.Fatalf("default queue must serve exactly the plain agent, got %d requests", n)
	}
	if n := len(mock.DumpsFor("mock-override")); n != 1 {
		t.Fatalf("override queue must serve exactly the override agent, got %d requests", n)
	}
}

// TestAgentR2_ChatEntryNestedStream: chat 入口——主 LLM 调 invoke_agent，子运行的流式块
// 经 parentBlockId 嵌套在 tool_call 之下（E3）、agent 结果回喂主对话、台账 TriggeredBy=chat
// 且带 conversationId。
func TestAgentR2_ChatEntryNestedStream(t *testing.T) {
	wc, mock := agentSetup(t)

	agID := agCreate(t, wc, map[string]any{
		"name": "Sub Worker", "description": "does sub work", "prompt": "Answer briefly.",
	})
	mock.Enqueue(agModel, harness.LLMTurn{Text: "sub-answer-token"})
	mock.Enqueue(dlgModel,
		harness.LLMTurn{ToolCalls: []harness.MockToolCall{{ID: "call_nest", Name: "invoke_agent",
			Args: fw(map[string]any{"agentId": agID, "input": map[string]any{"q": "go"}})}}},
		harness.LLMTurn{Text: "relayed: done"},
	)

	sse := wc.Subscribe(t, "messages")
	convID := convCreate(t, wc, "agent nesting")
	mid := sendMsg(t, wc, convID, "ask the worker")
	turn := waitTurn(t, wc, convID, mid, 30000)
	if turn.Status != "completed" {
		t.Fatalf("chat turn must complete, got %s %s", turn.Status, turn.ErrorMessage)
	}

	// E3 nesting, two steps: ① an open frame parents a text node under the PERSISTED
	// invoke_agent tool_call block id; ② that same node's close carries the agent's
	// full text snapshot. (Provider call ids are remapped to blk_ ids — AC-17 — so the
	// parent is the block id, not "call_nest".)
	// E3 嵌套两步：① 某 open 帧把 text 节点挂在**持久化的** invoke_agent tool_call 块 id
	// 下；② 同节点的 close 带 agent 完整文本快照。（provider call id 被重映射为 blk_ id
	// ——AC-17——父级是块 id 而非 "call_nest"。）
	turnNow := waitTurn(t, wc, convID, mid, 5000)
	tcBlockID := ""
	for _, b := range turnNow.Blocks {
		if b.Type == "tool_call" {
			tcBlockID = b.ID
		}
	}
	if tcBlockID == "" {
		t.Fatalf("persisted turn must carry the invoke_agent tool_call block: %+v", turnNow.Blocks)
	}
	nestedNodeID := ""
	for _, ev := range sse.Snapshot() {
		raw := string(ev.Data)
		if strings.Contains(raw, `"parentId":"`+tcBlockID+`"`) {
			var env struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(ev.Data, &env)
			nestedNodeID = env.ID
			break
		}
	}
	if nestedNodeID == "" {
		t.Fatalf("no streamed node nested under the tool_call block %s", tcBlockID)
	}
	found := false
	for _, ev := range sse.Snapshot() {
		raw := string(ev.Data)
		if strings.Contains(raw, `"id":"`+nestedNodeID+`"`) && strings.Contains(raw, "sub-answer-token") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("nested node %s must close with the agent's text snapshot", nestedNodeID)
	}

	// The agent's reply is fed back to the dialogue as the tool result.
	// agent 回复作为工具结果回喂主对话。
	dumps := mock.DumpsFor(dlgModel)
	fed := false
	for _, m := range dumps[len(dumps)-1].Messages {
		if m.Role == "tool" && strings.Contains(m.Content, "sub-answer-token") {
			fed = true
		}
	}
	if !fed {
		t.Fatalf("agent output must feed back to the dialogue, got %+v", dumps[len(dumps)-1].Messages)
	}

	// Ledger: TriggeredBy=chat with conversation provenance. 台账：chat 触发 + 对话溯源。
	var page struct {
		Executions []struct {
			TriggeredBy    string `json:"triggeredBy"`
			ConversationID string `json:"conversationId"`
		} `json:"executions"`
	}
	wc.GET("/api/v1/agents/"+agID+"/executions").OK(t, &page)
	if len(page.Executions) != 1 || page.Executions[0].TriggeredBy != "chat" || page.Executions[0].ConversationID != convID {
		t.Fatalf("chat-entry provenance wrong: %+v", page.Executions)
	}
}

// TestAgentR2_WorkflowEntryAndVersions: workflow 入口（agent 节点跑通、结果记忆化进 frn 行、
// 台账 TriggeredBy=workflow 带 flowrunId）+ 版本面（:edit 全量替换 → v2、versions 列表、
// :revert 回 v1 生效于下次 invoke）。
func TestAgentR2_WorkflowEntryAndVersions(t *testing.T) {
	wc, mock := agentSetup(t)

	agID := agCreate(t, wc, map[string]any{
		"name": "Pipeline Worker", "description": "v1 persona", "prompt": "Reply with your persona.",
	})

	// :edit = full Config replacement → v2 becomes active. :edit 全量替换 → v2 激活。
	wc.POST("/api/v1/agents/"+agID+":edit", map[string]any{
		"prompt": "V2-PERSONA reply.", "changeReason": "v2 persona",
	}).OK(t, nil)
	var vers []struct {
		Version int `json:"version"`
	}
	wc.GET("/api/v1/agents/"+agID+"/versions").OK(t, &vers)
	if len(vers) != 2 {
		t.Fatalf("want 2 versions after edit, got %+v", vers)
	}

	// Workflow agent node runs the ACTIVE (v2) config; result memoized on the node row.
	// workflow agent 节点跑 active（v2）配置；结果记忆化在节点行。
	wfID := wfCreate(t, wc, "agent_pipe", []map[string]any{
		{"op": "add_node", "node": map[string]any{"id": "start", "kind": "trigger", "ref": "trg_manual"}},
		{"op": "add_node", "node": map[string]any{"id": "worker", "kind": "agent", "ref": agID,
			"input": map[string]any{"task": "start.task"}}},
		{"op": "add_edge", "edge": map[string]any{"id": "e1", "from": "start", "to": "worker"}},
	})
	mock.Enqueue(agModel, harness.LLMTurn{Text: "wf-agent-output"})
	runID, status, nodes := runAndWait(t, wc, wfID, map[string]any{"task": "ship it"}, 30000)
	if status != "completed" {
		t.Fatalf("agent-node run must complete, got %s nodes=%s", status, nodes)
	}
	if !strings.Contains(string(nodes), "wf-agent-output") {
		t.Fatalf("agent result must be memoized on the node row: %s", nodes)
	}
	// The v2 prompt physically reached the model. v2 prompt 真到了模型。
	d := mock.DumpsFor(agModel)
	if !strings.Contains(string(d[len(d)-1].Raw), "V2-PERSONA") {
		t.Fatal("active v2 config must drive the workflow-entry invoke")
	}
	var page struct {
		Executions []struct {
			TriggeredBy string `json:"triggeredBy"`
			FlowrunID   string `json:"flowrunId"`
		} `json:"executions"`
	}
	wc.GET("/api/v1/agents/"+agID+"/executions").OK(t, &page)
	if len(page.Executions) != 1 || page.Executions[0].TriggeredBy != "workflow" || page.Executions[0].FlowrunID != runID {
		t.Fatalf("workflow-entry provenance wrong: %+v", page.Executions)
	}

	// :revert moves active back to v1 — effective on the NEXT invoke. :revert 回 v1，下次生效。
	wc.POST("/api/v1/agents/"+agID+":revert", map[string]any{"version": 1}).OK(t, nil)
	mock.Enqueue(agModel, harness.LLMTurn{Text: "ok"})
	agInvoke(t, wc, agID, nil)
	d = mock.DumpsFor(agModel)
	raw := string(d[len(d)-1].Raw)
	if strings.Contains(raw, "V2-PERSONA") || !strings.Contains(raw, "Reply with your persona") {
		t.Fatal("revert must restore the v1 prompt for subsequent invokes")
	}
}
