package scenarios

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/sunweilin/forgify/testend/harness"
)

// wfCreate forges a workflow from ops and returns its id.
//
// wfCreate 从 ops 锻造 workflow 并返回 id。
func wfCreate(t *testing.T, wc *harness.Client, name string, ops []map[string]any) string {
	t.Helper()
	var created struct {
		Workflow struct {
			ID string `json:"id"`
		} `json:"workflow"`
	}
	wc.POST("/api/v1/workflows", map[string]any{"name": name, "ops": ops}).OK(t, &created)
	if created.Workflow.ID == "" {
		t.Fatal("create returned no workflow.id")
	}
	return created.Workflow.ID
}

// runAndWait starts a manual run and returns (flowrunId, terminal status, nodes raw).
//
// runAndWait 手动起一个 run，返回（flowrunId、终态、节点原始 JSON）。
func runAndWait(t *testing.T, wc *harness.Client, workflowID string, payload map[string]any, timeoutMS int) (string, string, json.RawMessage) {
	t.Helper()
	var started struct {
		Flowrun struct {
			ID string `json:"id"`
		} `json:"flowrun"`
	}
	wc.POST("/api/v1/flowruns", map[string]any{"workflowId": workflowID, "payload": payload}).OK(t, &started)
	runID := started.Flowrun.ID
	var status string
	var nodes json.RawMessage
	harness.Eventually(t, timeoutMS, "run reaches a terminal state", func() bool {
		var got struct {
			Flowrun struct {
				Status string `json:"status"`
			} `json:"flowrun"`
			Nodes json.RawMessage `json:"nodes"`
		}
		r := wc.GET("/api/v1/flowruns/" + runID)
		if r.Status != 200 {
			return false
		}
		if err := json.Unmarshal(r.Data, &got); err != nil {
			return false
		}
		status, nodes = got.Flowrun.Status, got.Nodes
		return status == "completed" || status == "failed" || status == "cancelled"
	})
	return runID, status, nodes
}

// TestWorkflow_GraphValidationRejections: A5 图校验出错列——无 trigger / 孤儿节点。
func TestWorkflow_GraphValidationRejections(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "wf-rejects"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	// no trigger node. 无 trigger 节点。
	r := wc.POST("/api/v1/workflows", map[string]any{"name": "no_trigger", "ops": []map[string]any{
		{"op": "add_node", "node": map[string]any{"id": "a", "kind": "action", "ref": "fn_x"}},
	}})
	if r.Status < 400 || r.Code == "" {
		t.Fatalf("trigger-less graph must reject: %d %s", r.Status, r.Raw)
	}
	// orphan node (unreachable from trigger). 孤儿节点。
	r = wc.POST("/api/v1/workflows", map[string]any{"name": "orphan", "ops": []map[string]any{
		{"op": "add_node", "node": map[string]any{"id": "t", "kind": "trigger", "ref": "trg_x"}},
		{"op": "add_node", "node": map[string]any{"id": "lost", "kind": "action", "ref": "fn_x"}},
	}})
	if r.Status < 400 || r.Code == "" {
		t.Fatalf("orphan node must reject: %d %s", r.Status, r.Raw)
	}
}

// TestWorkflow_SetMetaProjection pins the AC-10 fix: a set_meta op actually lands on the
// header (it used to be a silent no-op), including the concurrency policy (AC-9).
//
// TestWorkflow_SetMetaProjection 钉死 AC-10 修复：set_meta op 真落头部（曾是静默 no-op），
// 含并发政策（AC-9）。
func TestWorkflow_SetMetaProjection(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "wf-meta"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	wfID := wfCreate(t, wc, "meta_probe", []map[string]any{
		{"op": "set_meta", "concurrency": "allow_all"},
		{"op": "add_node", "node": map[string]any{"id": "t", "kind": "trigger", "ref": "trg_x"}},
		{"op": "add_node", "node": map[string]any{"id": "a", "kind": "action", "ref": "fn_x", "input": map[string]any{"x": "t.v"}}},
		{"op": "add_edge", "edge": map[string]any{"id": "e1", "from": "t", "to": "a"}},
	})
	var detail struct {
		Concurrency string `json:"concurrency"`
		Name        string `json:"name"`
	}
	wc.GET("/api/v1/workflows/"+wfID).OK(t, &detail)
	if detail.Concurrency != "allow_all" {
		t.Fatalf("set_meta concurrency must land on header, got %q", detail.Concurrency)
	}

	// edit rename via set_meta — the historic silent no-op. 经 set_meta 改名——历史静默 no-op。
	wc.POST("/api/v1/workflows/"+wfID+":edit", map[string]any{"ops": []map[string]any{
		{"op": "set_meta", "name": "meta_probe_renamed", "concurrency": "skip"},
		{"op": "update_node", "id": "a", "patch": map[string]any{"ref": "fn_y"}},
	}}).OK(t, nil)
	wc.GET("/api/v1/workflows/"+wfID).OK(t, &detail)
	if detail.Name != "meta_probe_renamed" || detail.Concurrency != "skip" {
		t.Fatalf("edit set_meta must project: %+v", detail)
	}

	// PATCH concurrency knob (AC-9 HTTP face) + invalid value rejects.
	// PATCH 并发旋钮（AC-9 HTTP 面）+ 非法值拒。
	wc.PATCH("/api/v1/workflows/"+wfID, map[string]any{"concurrency": "buffer_one"}).OK(t, nil)
	wc.GET("/api/v1/workflows/"+wfID).OK(t, &detail)
	if detail.Concurrency != "buffer_one" {
		t.Fatalf("PATCH concurrency must land, got %q", detail.Concurrency)
	}
	r := wc.PATCH("/api/v1/workflows/"+wfID, map[string]any{"concurrency": "yolo"})
	if r.Status < 400 || r.Code == "" {
		t.Fatalf("invalid concurrency must reject: %d %s", r.Status, r.Raw)
	}
}

// TestWorkflow_LinearRunCELAddressing: 线性 run——payload CEL 寻址、节点记忆化、执行溯源。
func TestWorkflow_LinearRunCELAddressing(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "wf-linear"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	fnID := fnCreate(t, wc, "echo_step", "def f(x: str) -> dict:\n    print(f\"step got {x}\")\n    return {\"out\": x + \"!\"}\n")
	wfID := wfCreate(t, wc, "linear_pipe", []map[string]any{
		{"op": "add_node", "node": map[string]any{"id": "start", "kind": "trigger", "ref": "trg_manual"}},
		{"op": "add_node", "node": map[string]any{"id": "step", "kind": "action", "ref": fnID, "input": map[string]any{"x": "start.v"}}},
		{"op": "add_edge", "edge": map[string]any{"id": "e1", "from": "start", "to": "step"}},
	})

	runID, status, nodes := runAndWait(t, wc, wfID, map[string]any{"v": "ping"}, 30000)
	if status != "completed" {
		t.Fatalf("run must complete, got %s nodes=%s", status, nodes)
	}
	// memoized node result carries the function output (CEL addressed the payload).
	// 记忆化节点结果带函数输出（CEL 寻址到了 payload）。
	if !strings.Contains(string(nodes), `"out":"ping!"`) {
		t.Fatalf("step result missing CEL-addressed output: %s", nodes)
	}
	// execution ledger provenance: workflow-triggered + flowrun ids attached.
	// 执行台账溯源：workflow 触发 + flowrun 双列。
	var page struct {
		Executions []struct {
			TriggeredBy string `json:"triggeredBy"`
			FlowrunID   string `json:"flowrunId"`
		} `json:"executions"`
	}
	wc.GET("/api/v1/functions/"+fnID+"/executions").OK(t, &page)
	if len(page.Executions) != 1 || page.Executions[0].TriggeredBy != "workflow" || page.Executions[0].FlowrunID != runID {
		t.Fatalf("provenance wrong: %+v", page.Executions)
	}
}

// TestWorkflow_ControlRoutingAndEmit: control 真路由——选边、emit 字段下游可读、未选边不跑。
func TestWorkflow_ControlRoutingAndEmit(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "wf-ctl"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))

	bigFn := fnCreate(t, wc, "big_path", "def f(tier: str) -> dict:\n    return {\"ran\": \"big\", \"tier\": tier}\n")
	smallFn := fnCreate(t, wc, "small_path", "def f() -> dict:\n    return {\"ran\": \"small\"}\n")
	var ctl struct {
		Control struct {
			ID string `json:"id"`
		} `json:"control"`
	}
	wc.POST("/api/v1/controls", map[string]any{
		"name":   "amount_router",
		"inputs": []map[string]any{{"name": "amount", "type": "number"}},
		"branches": []map[string]any{
			{"port": "big", "when": "input.amount > 100.0", "emit": map[string]string{"tier": "'vip'"}},
			{"port": "small", "when": "true"},
		},
	}).OK(t, &ctl)

	wfID := wfCreate(t, wc, "routed_pipe", []map[string]any{
		{"op": "add_node", "node": map[string]any{"id": "start", "kind": "trigger", "ref": "trg_manual"}},
		{"op": "add_node", "node": map[string]any{"id": "gate", "kind": "control", "ref": ctl.Control.ID, "input": map[string]any{"amount": "start.amount"}}},
		{"op": "add_node", "node": map[string]any{"id": "big", "kind": "action", "ref": bigFn, "input": map[string]any{"tier": "gate.tier"}}},
		{"op": "add_node", "node": map[string]any{"id": "small", "kind": "action", "ref": smallFn}},
		{"op": "add_edge", "edge": map[string]any{"id": "e1", "from": "start", "to": "gate"}},
		{"op": "add_edge", "edge": map[string]any{"id": "e2", "from": "gate", "to": "big", "fromPort": "big"}},
		{"op": "add_edge", "edge": map[string]any{"id": "e3", "from": "gate", "to": "small", "fromPort": "small"}},
	})

	_, status, nodes := runAndWait(t, wc, wfID, map[string]any{"amount": 500}, 30000)
	if status != "completed" {
		t.Fatalf("routed run must complete: %s %s", status, nodes)
	}
	s := string(nodes)
	if !strings.Contains(s, `"ran":"big"`) || !strings.Contains(s, `"tier":"vip"`) {
		t.Fatalf("big branch must run with emitted tier: %s", s)
	}
	if strings.Contains(s, `"ran":"small"`) {
		t.Fatalf("unchosen branch must NOT run: %s", s)
	}
}

// TestWorkflow_ApprovalParkDecideResume: approval 人在环——park、收件箱、唤回通知、决策续跑。
func TestWorkflow_ApprovalParkDecideResume(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "wf-apf"}).OK(t, nil)
	wc := c.WS(ws.Field(t, "id"))
	ns := wc.Subscribe(t, "notifications")

	pubFn := fnCreate(t, wc, "publish_step", "def f(decision: str) -> dict:\n    return {\"published\": decision}\n")
	var apf struct {
		Approval struct {
			ID string `json:"id"`
		} `json:"approval"`
	}
	wc.POST("/api/v1/approvals", map[string]any{
		"name": "spend_gate", "template": "approve {{ input.amt }}?", "allowReason": true,
	}).OK(t, &apf)

	wfID := wfCreate(t, wc, "approval_pipe", []map[string]any{
		{"op": "add_node", "node": map[string]any{"id": "start", "kind": "trigger", "ref": "trg_manual"}},
		{"op": "add_node", "node": map[string]any{"id": "human", "kind": "approval", "ref": apf.Approval.ID, "input": map[string]any{"amt": "start.amt"}}},
		{"op": "add_node", "node": map[string]any{"id": "pub", "kind": "action", "ref": pubFn, "input": map[string]any{"decision": "human.decision"}}},
		{"op": "add_edge", "edge": map[string]any{"id": "e1", "from": "start", "to": "human"}},
		{"op": "add_edge", "edge": map[string]any{"id": "e2", "from": "human", "to": "pub", "fromPort": "yes"}},
	})

	// start → parks. 起跑 → 挂起。
	var started struct {
		Flowrun struct {
			ID string `json:"id"`
		} `json:"flowrun"`
		Nodes json.RawMessage `json:"nodes"`
	}
	wc.POST("/api/v1/flowruns", map[string]any{"workflowId": wfID, "payload": map[string]any{"amt": "99"}}).OK(t, &started)
	runID := started.Flowrun.ID
	if !strings.Contains(string(started.Nodes), `"parked"`) {
		t.Fatalf("approval must park: %s", started.Nodes)
	}
	// summons + inbox. 唤回 + 收件箱。
	ns.WaitFor(t, 8000, "approval_pending summons the human", "workflow.approval_pending", runID)
	var inbox struct {
		Parked []struct {
			FlowRunID string `json:"flowrunId"`
			NodeID    string `json:"nodeId"`
		} `json:"parked"`
	}
	wc.GET("/api/v1/flowrun-inbox").OK(t, &inbox)
	found := false
	for _, p := range inbox.Parked {
		if p.FlowRunID == runID && p.NodeID == "human" {
			found = true
		}
	}
	if !found {
		t.Fatalf("inbox must list the parked node: %+v", inbox.Parked)
	}

	// decide yes → resumes through the yes edge and completes.
	// 决策 yes → 走 yes 边续跑至完成。
	wc.POST("/api/v1/flowruns/"+runID+"/approvals/human:decide", map[string]any{"decision": "yes", "reason": "fine"}).OK(t, nil)
	harness.Eventually(t, 20000, "run completes after decision", func() bool {
		var got struct {
			Flowrun struct {
				Status string `json:"status"`
			} `json:"flowrun"`
			Nodes json.RawMessage `json:"nodes"`
		}
		r := wc.GET("/api/v1/flowruns/" + runID)
		if r.Status != 200 {
			return false
		}
		_ = json.Unmarshal(r.Data, &got)
		return got.Flowrun.Status == "completed" && strings.Contains(string(got.Nodes), `"published":"yes"`)
	})
}

// TestWorkflow_CrashRecovery is durable execution's final exam: kill -9 mid-run, restart
// on the same data dir, and the run must finish via boot Recover (at-least-once re-run).
//
// TestWorkflow_CrashRecovery 是 durable 执行的终极考试：run 进行中 kill -9、同数据目录重启，
// boot Recover 必须把 run 跑完（at-least-once 重跑）。
func TestWorkflow_CrashRecovery(t *testing.T) {
	srv := harness.Start(t)
	c := srv.Client(t)
	ws := c.POST("/api/v1/workspaces", map[string]any{"name": "wf-crash"}).OK(t, nil)
	wsID := ws.Field(t, "id")
	wc := c.WS(wsID)

	slowFn := fnCreate(t, wc, "slow_step", "import time\ndef f() -> dict:\n    time.sleep(6)\n    return {\"survived\": True}\n")
	wfID := wfCreate(t, wc, "crash_pipe", []map[string]any{
		{"op": "add_node", "node": map[string]any{"id": "start", "kind": "trigger", "ref": "trg_manual"}},
		{"op": "add_node", "node": map[string]any{"id": "slow", "kind": "action", "ref": slowFn}},
		{"op": "add_edge", "edge": map[string]any{"id": "e1", "from": "start", "to": "slow"}},
	})

	// fire the run without waiting — the request may die with the process (that IS the
	// crash); Try tolerates it. 不等结果起跑——请求可能随进程夭折（这就是崩溃）；Try 容忍。
	go func() {
		_, _ = wc.Try("POST", "/api/v1/flowruns", map[string]any{"workflowId": wfID, "payload": map[string]any{}})
	}()
	time.Sleep(2 * time.Second) // run is inside the 6s sleep. run 正睡在 6s 里。

	srv.Kill9(t)
	srv.Restart(t)
	wc2 := srv.Client(t).WS(wsID)

	// Recover must finish the run (slow node re-executed — at-least-once).
	// Recover 必须把 run 跑完（slow 节点重跑——at-least-once）。
	harness.Eventually(t, 40000, "crashed run recovers to completed", func() bool {
		var page struct {
			Items []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"items"`
		}
		r := wc2.GET("/api/v1/flowruns?workflowId=" + wfID)
		if r.Status != 200 {
			return false
		}
		_ = json.Unmarshal(r.Data, &page)
		raw := string(r.Data)
		_ = page
		return strings.Contains(raw, `"status":"completed"`)
	})
}
