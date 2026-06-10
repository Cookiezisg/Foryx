package agent

import (
	"context"
	"encoding/json"
	"iter"
	"sync"
	"testing"

	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// scriptedLLMClient replays a different script per Stream call (one per ReAct step / run) — so a
// park step and its continuation can differ.
//
// scriptedLLMClient 每次 Stream 调用回放不同脚本（每 ReAct 步/运行一份）——使 park 步与续跑可不同。
type scriptedLLMClient struct {
	mu      sync.Mutex
	scripts [][]llminfra.StreamEvent
	call    int
}

func (c *scriptedLLMClient) Stream(_ context.Context, _ llminfra.Request) iter.Seq[llminfra.StreamEvent] {
	c.mu.Lock()
	idx := c.call
	c.call++
	c.mu.Unlock()
	return func(yield func(llminfra.StreamEvent) bool) {
		if idx >= len(c.scripts) {
			return
		}
		for _, ev := range c.scripts[idx] {
			if !yield(ev) {
				return
			}
		}
	}
}

// recTool records whether Execute ran (so a park test can assert non-execution before approval).
//
// recTool 记录 Execute 是否跑过（供 park 测试断言批准前未执行）。
type recTool struct {
	name string
	ran  *bool
}

func (t recTool) Name() string                        { return t.name }
func (t recTool) Description() string                 { return "deploy" }
func (t recTool) Parameters() json.RawMessage         { return json.RawMessage(`{"type":"object"}`) }
func (t recTool) ValidateInput(json.RawMessage) error { return nil }
func (t recTool) Execute(context.Context, string) (string, error) {
	*t.ran = true
	return "deployed", nil
}

// TestResumeExecution_DangerParkApprove: an interactively-invoked agent parks on a dangerous tool
// call (NOT run); ResumeExecution(approve) runs the tool, replays the transcript, and the run
// completes — the execution row advances parked → ok.
//
// TestResumeExecution_DangerParkApprove：交互调起的 agent 在危险工具调用处 park（不跑）；ResumeExecution(approve)
// 跑工具、重放 transcript、运行完成——execution 行从 parked → ok。
func TestResumeExecution_DangerParkApprove(t *testing.T) {
	svc, ctx := newSvc(t)
	ran := false
	svc.SetInvokeDeps(InvokeDeps{
		Resolver: fakeResolver{client: &scriptedLLMClient{scripts: [][]llminfra.StreamEvent{
			{ // step 1 → dangerous tool call → parks
				{Type: llminfra.EventToolStart, ToolIndex: 0, ToolID: "tc1", ToolName: "deploy"},
				{Type: llminfra.EventToolDelta, ToolIndex: 0, ArgsDelta: `{"danger":"dangerous","target":"prod"}`},
				{Type: llminfra.EventFinish, FinishReason: "tool_use", InputTokens: 5, OutputTokens: 3},
			},
			{ // continuation → final answer
				{Type: llminfra.EventText, Delta: "deployed successfully"},
				{Type: llminfra.EventFinish, InputTokens: 2, OutputTokens: 2},
			},
		}}},
		Tools:     func() []toolapp.Tool { return []toolapp.Tool{recTool{name: "deploy", ran: &ran}} },
		Knowledge: fakeKnowledge{},
	})
	a, _, err := svc.Create(ctx, CreateInput{Name: "deployer", Config: Config{
		Prompt: "deploy prod", Tools: []agentdomain.ToolRef{{Ref: "deploy"}},
	}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// manual invoke (interactive) → parks on the dangerous call
	res, err := svc.InvokeAgent(ctx, InvokeInput{AgentID: a.ID, TriggeredBy: agentdomain.TriggeredByManual})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !res.Parked || res.Status != agentdomain.ExecutionStatusParked {
		t.Fatalf("expected parked, got %+v", res)
	}
	if ran {
		t.Fatal("dangerous tool ran before approval")
	}

	// approve → executes the tool + replays + completes
	res2, err := svc.ResumeExecution(ctx, res.ExecutionID, "tc1", loopapp.ResolveApprove, "")
	if err != nil {
		t.Fatalf("ResumeExecution: %v", err)
	}
	if !ran {
		t.Fatal("approve must execute the gated tool")
	}
	if !res2.OK || res2.Output != "deployed successfully" {
		t.Fatalf("resumed run should complete with the continuation output, got %+v", res2)
	}
	exec, err := svc.repo.GetExecutionByID(ctx, res.ExecutionID)
	if err != nil || exec.Status != agentdomain.ExecutionStatusOK {
		t.Fatalf("execution should advance parked → ok, got status=%q err=%v", exec.Status, err)
	}
}

// TestResumeExecution_Deny: denying does NOT run the tool; the run still completes (the model
// re-routes off the denial).
//
// TestResumeExecution_Deny：拒绝不跑工具；运行仍完成（模型据拒绝改道）。
func TestResumeExecution_Deny(t *testing.T) {
	svc, ctx := newSvc(t)
	ran := false
	svc.SetInvokeDeps(InvokeDeps{
		Resolver: fakeResolver{client: &scriptedLLMClient{scripts: [][]llminfra.StreamEvent{
			{
				{Type: llminfra.EventToolStart, ToolIndex: 0, ToolID: "tc1", ToolName: "deploy"},
				{Type: llminfra.EventToolDelta, ToolIndex: 0, ArgsDelta: `{"danger":"dangerous"}`},
				{Type: llminfra.EventFinish, FinishReason: "tool_use", InputTokens: 5, OutputTokens: 3},
			},
			{
				{Type: llminfra.EventText, Delta: "okay, skipping"},
				{Type: llminfra.EventFinish, InputTokens: 2, OutputTokens: 2},
			},
		}}},
		Tools:     func() []toolapp.Tool { return []toolapp.Tool{recTool{name: "deploy", ran: &ran}} },
		Knowledge: fakeKnowledge{},
	})
	a, _, _ := svc.Create(ctx, CreateInput{Name: "deployer", Config: Config{
		Prompt: "deploy", Tools: []agentdomain.ToolRef{{Ref: "deploy"}},
	}})
	res, _ := svc.InvokeAgent(ctx, InvokeInput{AgentID: a.ID, TriggeredBy: agentdomain.TriggeredByManual})
	if !res.Parked {
		t.Fatalf("expected parked, got %+v", res)
	}
	if _, err := svc.ResumeExecution(ctx, res.ExecutionID, "tc1", loopapp.ResolveDeny, ""); err != nil {
		t.Fatalf("ResumeExecution deny: %v", err)
	}
	if ran {
		t.Fatal("deny must NOT run the tool")
	}
}

// TestInvoke_WorkflowNeverParks: a workflow-triggered run does NOT park on a dangerous call (no
// interactive approver) — it runs the tool (pure trust), matching the non-interactive contract.
//
// TestInvoke_WorkflowNeverParks：workflow 触发的运行不在危险调用处 park（无交互审批人）——照跑工具（纯信任），符合非交互契约。
func TestInvoke_WorkflowNeverParks(t *testing.T) {
	svc, ctx := newSvc(t)
	ran := false
	svc.SetInvokeDeps(InvokeDeps{
		Resolver: fakeResolver{client: &scriptedLLMClient{scripts: [][]llminfra.StreamEvent{
			{
				{Type: llminfra.EventToolStart, ToolIndex: 0, ToolID: "tc1", ToolName: "deploy"},
				{Type: llminfra.EventToolDelta, ToolIndex: 0, ArgsDelta: `{"danger":"dangerous"}`},
				{Type: llminfra.EventFinish, FinishReason: "tool_use", InputTokens: 5, OutputTokens: 3},
			},
			{
				{Type: llminfra.EventText, Delta: "done"},
				{Type: llminfra.EventFinish, InputTokens: 2, OutputTokens: 2},
			},
		}}},
		Tools:     func() []toolapp.Tool { return []toolapp.Tool{recTool{name: "deploy", ran: &ran}} },
		Knowledge: fakeKnowledge{},
	})
	a, _, _ := svc.Create(ctx, CreateInput{Name: "deployer", Config: Config{
		Prompt: "deploy", Tools: []agentdomain.ToolRef{{Ref: "deploy"}},
	}})
	res, err := svc.InvokeAgent(ctx, InvokeInput{AgentID: a.ID, TriggeredBy: agentdomain.TriggeredByWorkflow})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if res.Parked {
		t.Fatal("a workflow run must never park")
	}
	if !ran {
		t.Fatal("workflow run should execute the dangerous tool (pure trust)")
	}
}
