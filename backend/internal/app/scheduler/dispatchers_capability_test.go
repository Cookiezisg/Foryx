package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
)

func contains(s, sub string) bool { return strings.Contains(s, sub) }

func mkInput(node workflowdomain.NodeSpec, run *flowrundomain.FlowRun) DispatchInput {
	return DispatchInput{
		Node:   node,
		NodeIn: map[string]any{},
		ExecCtx: &ExecutionContext{
			Run: run,
			Variables: map[string]any{
				"trigger": run.TriggerInput,
			},
		},
	}
}

func TestTriggerDispatcher_PassesTriggerInput(t *testing.T) {
	d := NewTriggerDispatcher()
	run := &flowrundomain.FlowRun{ID: "fr1", TriggerInput: map[string]any{"k": "v"}}
	in := mkInput(workflowdomain.NodeSpec{ID: "trig", Type: workflowdomain.NodeTypeTrigger}, run)

	out := d.Dispatch(context.Background(), in)
	if out.Error != nil {
		t.Fatalf("Error: %v", out.Error)
	}
	got, _ := out.Outputs["out"].(map[string]any)
	if got["k"] != "v" {
		t.Errorf("trigger payload lost: %+v", out.Outputs)
	}
}

func TestFunctionDispatcher_MissingFunctionId(t *testing.T) {
	d := NewFunctionDispatcher(nil)
	in := mkInput(workflowdomain.NodeSpec{ID: "fn", Type: workflowdomain.NodeTypeFunction},
		&flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if out.Error == nil {
		t.Fatalf("expected error for missing functionId")
	}
	if !contains(out.Error.Error(), "functionId required") {
		t.Errorf("error text = %q", out.Error.Error())
	}
}

func TestHandlerDispatcher_MissingHandlerName(t *testing.T) {
	d := NewHandlerDispatcher(nil)
	in := mkInput(workflowdomain.NodeSpec{ID: "h", Type: workflowdomain.NodeTypeHandler,
		Config: map[string]any{"method": "ping"}}, &flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if out.Error == nil || !contains(out.Error.Error(), "handlerName required") {
		t.Errorf("expected handlerName-required error, got %v", out.Error)
	}
}

func TestHandlerDispatcher_MissingMethod(t *testing.T) {
	d := NewHandlerDispatcher(nil)
	in := mkInput(workflowdomain.NodeSpec{ID: "h", Type: workflowdomain.NodeTypeHandler,
		Config: map[string]any{"handlerName": "x"}}, &flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if out.Error == nil || !contains(out.Error.Error(), "method required") {
		t.Errorf("expected method-required error, got %v", out.Error)
	}
}

func TestMCPDispatcher_MissingServerName(t *testing.T) {
	d := NewMCPDispatcher(nil)
	in := mkInput(workflowdomain.NodeSpec{ID: "m", Type: workflowdomain.NodeTypeMCP,
		Config: map[string]any{"tool": "x"}}, &flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if out.Error == nil || !contains(out.Error.Error(), "serverName required") {
		t.Errorf("expected serverName-required error, got %v", out.Error)
	}
}

func TestMCPDispatcher_MissingTool(t *testing.T) {
	d := NewMCPDispatcher(nil)
	in := mkInput(workflowdomain.NodeSpec{ID: "m", Type: workflowdomain.NodeTypeMCP,
		Config: map[string]any{"serverName": "x"}}, &flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if out.Error == nil || !contains(out.Error.Error(), "tool required") {
		t.Errorf("expected tool-required error, got %v", out.Error)
	}
}

func TestSkillDispatcher_MissingSkillName(t *testing.T) {
	d := NewSkillDispatcher(nil)
	in := mkInput(workflowdomain.NodeSpec{ID: "s", Type: workflowdomain.NodeTypeSkill},
		&flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if out.Error == nil || !contains(out.Error.Error(), "skillName required") {
		t.Errorf("expected skillName-required error, got %v", out.Error)
	}
}

type fakeLLMCaller struct {
	gotScenario string
	gotPrompt   string
	gotVars     map[string]any
	resp        string
	err         error
}

func (f *fakeLLMCaller) Generate(_ context.Context, scenario, prompt string, vars map[string]any) (string, error) {
	f.gotScenario, f.gotPrompt, f.gotVars = scenario, prompt, vars
	return f.resp, f.err
}

func TestLLMDispatcher_NoCaller_ReturnsError(t *testing.T) {
	d := NewLLMDispatcher(nil)
	in := mkInput(workflowdomain.NodeSpec{ID: "l", Type: workflowdomain.NodeTypeLLM,
		Config: map[string]any{"prompt": "hi"}}, &flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if out.Error == nil || !contains(out.Error.Error(), "no LLMCaller wired") {
		t.Errorf("expected nil-caller error, got %v", out.Error)
	}
}

func TestLLMDispatcher_MissingPrompt(t *testing.T) {
	d := NewLLMDispatcher(&fakeLLMCaller{})
	in := mkInput(workflowdomain.NodeSpec{ID: "l", Type: workflowdomain.NodeTypeLLM,
		Config: map[string]any{}}, &flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if out.Error == nil || !contains(out.Error.Error(), "prompt required") {
		t.Errorf("expected prompt-required error, got %v", out.Error)
	}
}

func TestLLMDispatcher_DefaultScenarioIsChat(t *testing.T) {
	caller := &fakeLLMCaller{resp: "hello world"}
	d := NewLLMDispatcher(caller)
	in := mkInput(workflowdomain.NodeSpec{ID: "l", Type: workflowdomain.NodeTypeLLM,
		Config: map[string]any{"prompt": "say hi"}}, &flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if out.Error != nil {
		t.Fatalf("unexpected error: %v", out.Error)
	}
	if caller.gotScenario != "chat" {
		t.Errorf("default scenario = %q, want chat", caller.gotScenario)
	}
	if out.Outputs["out"] != "hello world" {
		t.Errorf("output = %v, want hello world", out.Outputs)
	}
}

func TestLLMDispatcher_CallerErrorPropagates(t *testing.T) {
	bad := errors.New("upstream LLM 500")
	d := NewLLMDispatcher(&fakeLLMCaller{err: bad})
	in := mkInput(workflowdomain.NodeSpec{ID: "l", Type: workflowdomain.NodeTypeLLM,
		Config: map[string]any{"prompt": "x"}}, &flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if !errors.Is(out.Error, bad) {
		t.Errorf("expected wrapped upstream error, got %v", out.Error)
	}
}
