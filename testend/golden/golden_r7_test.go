// golden_r7_test.go — R7（柱 C 金标补全：计划 12 旅程的后 5 条，真模型 deepseek-v4-flash）。
//
// J4 搓 workflow 真触发到 parked / J6 MCP 工具真发现真调 / J8 跨对话回忆（search_conversations）
// / J10 激活 skill 干活 / J11 跨压缩边界长任务。与首批 7 条同范式：结果状态断言
// （实体建了/run parked 了/调用记账了/摘要落了），不赌逐字；drainInteractions 自动放行人在环。
package golden

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/testend/harness"
)

// echoMCP 是零依赖单工具（echo）MCP stdio server——J6 的本地真服务（golden 与 scenarios
// 不共享 helper，自带最小版）。
const echoMCP = `import sys, json

def send(obj):
    sys.stdout.write(json.dumps(obj) + "\n")
    sys.stdout.flush()

for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    req = json.loads(line)
    mid = req.get("id")
    method = req.get("method")
    if method == "initialize":
        send({"jsonrpc": "2.0", "id": mid, "result": {
            "protocolVersion": "2024-11-05", "capabilities": {"tools": {}},
            "serverInfo": {"name": "goldecho", "version": "1.0.0"}}})
    elif method == "notifications/initialized":
        continue
    elif method == "tools/list":
        send({"jsonrpc": "2.0", "id": mid, "result": {"tools": [
            {"name": "echo", "description": "echo text back",
             "inputSchema": {"type": "object", "properties": {"text": {"type": "string"}}, "required": ["text"]}}]}})
    elif method == "tools/call":
        text = (req.get("params") or {}).get("arguments", {}).get("text", "")
        send({"jsonrpc": "2.0", "id": mid, "result": {"content": [{"type": "text", "text": "echo:" + text}]}})
    elif mid is not None:
        send({"jsonrpc": "2.0", "id": mid, "result": {}})
`

// TestGolden_J4_WorkflowToParked: 预置 function + approval 积木，真模型搓三节点
// workflow（trigger→approval→fn）并触发一次 run——结果状态：flowrun 真挂在 parked。
func TestGolden_J4_WorkflowToParked(t *testing.T) {
	wc := evalWS(t)

	fnID := ""
	{
		var created struct {
			Function struct {
				ID string `json:"id"`
			} `json:"function"`
		}
		wc.POST("/api/v1/functions", map[string]any{
			"name": "publish_report", "description": "publishes the report",
			"code": "def publish_report() -> dict:\n    return {\"published\": True}\n",
		}).OK(t, &created)
		fnID = created.Function.ID
	}
	var apf struct {
		Approval struct {
			ID string `json:"id"`
		} `json:"approval"`
	}
	wc.POST("/api/v1/approvals", map[string]any{
		"name": "publish_gate", "description": "human gate before publishing",
		"template": "Publish the report?",
	}).OK(t, &apf)

	conv := newConv(t, wc, "build workflow")
	say(t, wc, conv,
		"Create a workflow named publish_pipeline with three nodes: a manual trigger, then the existing "+
			"approval "+apf.Approval.ID+" (a human gate), then the existing function "+fnID+". "+
			"Wire trigger→approval, and approval's yes port→function. Then start one run of it "+
			"(trigger it manually) and tell me the run id.", 300000)

	// 结果状态：存在一个 parked 的 flowrun（approval 节点真把 run 挂住）。
	var wfs []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	wc.GET("/api/v1/workflows").OK(t, &wfs)
	if len(wfs) == 0 {
		t.Fatal("the model must have created a workflow")
	}
	harness.Eventually(t, 120000, "a flowrun parks on the approval gate", func() bool {
		for _, wf := range wfs {
			r := wc.GET("/api/v1/flowruns?workflowId=" + wf.ID)
			if r.Status == 200 && strings.Contains(string(r.Data), `"parked"`) {
				return true
			}
		}
		return false
	})
}

// TestGolden_J6_MCPDiscoverAndCall: 预装本地 echo MCP server，真模型发现动态工具并真调
// ——结果状态：mcp_calls 台账一条 ok、triggeredBy=chat。
func TestGolden_J6_MCPDiscoverAndCall(t *testing.T) {
	wc := evalWS(t)

	script := filepath.Join(t.TempDir(), "echo_mcp.py")
	if err := os.WriteFile(script, []byte(echoMCP), 0o644); err != nil {
		t.Fatalf("write mcp script: %v", err)
	}
	var st struct {
		Status string `json:"status"`
	}
	wc.PUT("/api/v1/mcp-servers/goldecho", map[string]any{
		"description": "local echo server", "command": "python3", "args": []string{script},
	}).OK(t, &st)
	if st.Status != "ready" {
		t.Fatalf("echo server must be ready, got %s", st.Status)
	}

	conv := newConv(t, wc, "mcp call")
	say(t, wc, conv,
		"I installed an MCP server named goldecho with one tool. Use its echo tool to echo back "+
			"the exact text GOLDENPING and tell me what it returned.", 240000)

	var calls struct {
		Calls []struct {
			Tool        string `json:"tool"`
			Status      string `json:"status"`
			TriggeredBy string `json:"triggeredBy"`
		} `json:"calls"`
	}
	wc.GET("/api/v1/mcp-servers/goldecho/calls").OK(t, &calls)
	ok := false
	for _, c := range calls.Calls {
		if c.Tool == "echo" && c.Status == "ok" && c.TriggeredBy == "chat" {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("the model must really call the mcp echo tool (ledger: %+v)", calls.Calls)
	}
}

// TestGolden_J8_RecallPastConversation: 对话 A 落一个独特事实；对话 B 问"我们之前聊过什么"
// ——真模型经 search_conversations 找回并在答复中报出该事实。
func TestGolden_J8_RecallPastConversation(t *testing.T) {
	wc := evalWS(t)

	convA := newConv(t, wc, "planning session")
	say(t, wc, convA, "Note this down in our chat: the secret launch harbor is called GOLDHARBOR-7. Just acknowledge.", 120000)

	// 索引异步——先等综搜可见再开第二段对话。
	harness.Eventually(t, 30000, "conversation indexed", func() bool {
		r := wc.GET("/api/v1/search?q=GOLDHARBOR&types=conversation")
		return r.Status == 200 && strings.Contains(string(r.Data), convA)
	})

	convB := newConv(t, wc, "recall session")
	answer := say(t, wc, convB,
		"In an earlier conversation we discussed a secret launch harbor. Search our past conversations "+
			"and tell me its exact name.", 240000)
	if !strings.Contains(answer, "GOLDHARBOR-7") {
		t.Fatalf("the model must recall the fact from the past conversation, got: %s", answer)
	}
}

// TestGolden_J10_SkillActivation: 预置带独特指令的 skill，真模型按用户要求激活并遵循
// ——结果状态：答复带 skill 指定的暗号词。
func TestGolden_J10_SkillActivation(t *testing.T) {
	wc := evalWS(t)

	wc.POST("/api/v1/skills", map[string]any{
		"name": "release_checklist", "description": "the team's release checklist routine",
		"body": "When asked about releasing, ALWAYS begin your final answer with the word SKILLSTAMP " +
			"followed by a three-step checklist.",
	}).OK(t, nil)

	conv := newConv(t, wc, "use the skill")
	answer := say(t, wc, conv,
		"Activate my release_checklist skill and follow it to tell me how to release.", 240000)
	if !strings.Contains(answer, "SKILLSTAMP") {
		t.Fatalf("the model must activate and FOLLOW the skill instruction, got: %s", answer)
	}
}

// TestGolden_J12b_CrossCompactionTask: 跨压缩边界长任务——压低触发线，多回合后压缩真发生
// （摘要落 conversation 行），随后对话照常推进且能引用早期事实（摘要语义兜底）。
func TestGolden_J12b_CrossCompactionTask(t *testing.T) {
	wc := evalWS(t)
	wc.PATCH("/api/v1/limits", map[string]any{"context": map[string]any{"triggerRatio": 0.01}}).OK(t, nil)

	conv := newConv(t, wc, "long task")
	say(t, wc, conv, "Remember: the project codename is GOLDCOMPACT-11. Acknowledge briefly.", 120000)
	say(t, wc, conv, "Now list three colors, briefly.", 120000)
	say(t, wc, conv, "And three animals, briefly.", 120000)

	// 压缩真发生：滚动摘要落到 conversation 行。
	harness.Eventually(t, 60000, "rolling summary persists", func() bool {
		var conv2 struct {
			Summary string `json:"summary"`
		}
		r := wc.GET("/api/v1/conversations/" + conv)
		if r.Status != 200 {
			return false
		}
		_ = json.Unmarshal(r.Data, &conv2)
		return conv2.Summary != ""
	})

	// 压缩后继续推进——回合不报错；语义兜底：模型仍能答出 codename（来自摘要或近窗）。
	answer := say(t, wc, conv, "What is the project codename I told you at the start?", 240000)
	if !strings.Contains(answer, "GOLDCOMPACT-11") {
		t.Fatalf("the task must survive the compaction boundary, got: %s", answer)
	}
}
