package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"

	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
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
	gotOverride *modeldomain.ModelRef
	gotPrompt   string
	gotVars     map[string]any
	resp        string
	err         error
}

func (f *fakeLLMCaller) Generate(_ context.Context, override *modeldomain.ModelRef, prompt string, vars map[string]any) (string, error) {
	f.gotOverride, f.gotPrompt, f.gotVars = override, prompt, vars
	return f.resp, f.err
}

func TestLLMDispatcher_NoCaller_ReturnsError(t *testing.T) {
	d := NewLLMDispatcher(nil, nil)
	in := mkInput(workflowdomain.NodeSpec{ID: "l", Type: workflowdomain.NodeTypeLLM,
		Config: map[string]any{"prompt": "hi"}}, &flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if out.Error == nil || !contains(out.Error.Error(), "no LLMCaller wired") {
		t.Errorf("expected nil-caller error, got %v", out.Error)
	}
}

func TestLLMDispatcher_MissingPrompt(t *testing.T) {
	d := NewLLMDispatcher(&fakeLLMCaller{}, nil)
	in := mkInput(workflowdomain.NodeSpec{ID: "l", Type: workflowdomain.NodeTypeLLM,
		Config: map[string]any{}}, &flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if out.Error == nil || !contains(out.Error.Error(), "prompt required") {
		t.Errorf("expected prompt-required error, got %v", out.Error)
	}
}

func TestLLMDispatcher_NilOverrideByDefault(t *testing.T) {
	caller := &fakeLLMCaller{resp: "hello world"}
	d := NewLLMDispatcher(caller, nil)
	in := mkInput(workflowdomain.NodeSpec{ID: "l", Type: workflowdomain.NodeTypeLLM,
		Config: map[string]any{"prompt": "say hi"}}, &flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if out.Error != nil {
		t.Fatalf("unexpected error: %v", out.Error)
	}
	// Task 11 will wire NodeSpec.ModelOverride; until then dispatcher always stubs nil.
	if caller.gotOverride != nil {
		t.Errorf("default override = %+v, want nil", caller.gotOverride)
	}
	if out.Outputs["out"] != "hello world" {
		t.Errorf("output = %v, want hello world", out.Outputs)
	}
}

func TestLLMDispatcher_CallerErrorPropagates(t *testing.T) {
	bad := errors.New("upstream LLM 500")
	d := NewLLMDispatcher(&fakeLLMCaller{err: bad}, nil)
	in := mkInput(workflowdomain.NodeSpec{ID: "l", Type: workflowdomain.NodeTypeLLM,
		Config: map[string]any{"prompt": "x"}}, &flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if !errors.Is(out.Error, bad) {
		t.Errorf("expected wrapped upstream error, got %v", out.Error)
	}
}

// fakeDocResolver returns the supplied docs verbatim regardless of inputs.
//
// fakeDocResolver 不管输入,返预设 docs。
type fakeDocResolver struct {
	docs []*documentdomain.Document
	err  error
}

func (f *fakeDocResolver) ResolveAttached(_ context.Context, _ []documentdomain.AttachedDocument) ([]*documentdomain.Document, error) {
	return f.docs, f.err
}

func TestLLMDispatcher_AttachedDocuments_PrependedToPrompt(t *testing.T) {
	caller := &fakeLLMCaller{resp: "ok"}
	resolver := &fakeDocResolver{
		docs: []*documentdomain.Document{
			{ID: "doc_1", Path: "/spec", Content: "# spec contentBODY"},
		},
	}
	d := NewLLMDispatcher(caller, resolver)

	in := mkInput(workflowdomain.NodeSpec{
		ID:   "l",
		Type: workflowdomain.NodeTypeLLM,
		Config: map[string]any{
			"prompt": "summarise",
			"attachedDocuments": []map[string]any{
				{"documentId": "doc_1"},
			},
		},
	}, &flowrundomain.FlowRun{ID: "fr1"})

	out := d.Dispatch(context.Background(), in)
	if out.Error != nil {
		t.Fatalf("dispatch err: %v", out.Error)
	}
	if !contains(caller.gotPrompt, "<documents>") {
		t.Errorf("prompt missing docs prefix:\n%s", caller.gotPrompt)
	}
	if !contains(caller.gotPrompt, "spec contentBODY") {
		t.Errorf("prompt missing doc content:\n%s", caller.gotPrompt)
	}
	if !contains(caller.gotPrompt, "summarise") {
		t.Errorf("prompt missing original instruction:\n%s", caller.gotPrompt)
	}
}

func TestLLMDispatcher_NoResolver_NoPrefix(t *testing.T) {
	caller := &fakeLLMCaller{resp: "ok"}
	// No resolver, but attachedDocuments set in config — should be silently skipped.
	d := NewLLMDispatcher(caller, nil)
	in := mkInput(workflowdomain.NodeSpec{
		ID:   "l",
		Type: workflowdomain.NodeTypeLLM,
		Config: map[string]any{
			"prompt":            "say hi",
			"attachedDocuments": []map[string]any{{"documentId": "doc_x"}},
		},
	}, &flowrundomain.FlowRun{ID: "fr1"})
	out := d.Dispatch(context.Background(), in)
	if out.Error != nil {
		t.Fatalf("err: %v", out.Error)
	}
	if contains(caller.gotPrompt, "<documents>") {
		t.Errorf("prompt should NOT have docs prefix without resolver:\n%s", caller.gotPrompt)
	}
	if caller.gotPrompt != "say hi" {
		t.Errorf("prompt should equal original input; got %q", caller.gotPrompt)
	}
}
