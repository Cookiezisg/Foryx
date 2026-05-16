package ask

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	askapp "github.com/sunweilin/forgify/backend/internal/app/ask"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func newTestTool() (*AskUserQuestion, *askapp.Service) {
	svc := askapp.NewService()
	return &AskUserQuestion{svc: svc, timeout: 500 * time.Millisecond}, svc
}

func TestAskUserQuestion_Identity(t *testing.T) {
	tool, _ := newTestTool()
	if tool.Name() != "AskUserQuestion" {
		t.Errorf("Name = %q", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description empty")
	}
	if len(tool.Parameters()) == 0 {
		t.Error("Parameters empty")
	}
}

func TestAskUserQuestion_StaticMetadata(t *testing.T) {
	tool, _ := newTestTool()
	if !tool.IsReadOnly() {
		t.Error("AskUserQuestion should be read-only (does not mutate state)")
	}
	if tool.NeedsReadFirst() {
		t.Error("AskUserQuestion should not require Read first")
	}
	if tool.RequiresWorkspace() {
		t.Error("AskUserQuestion should not require workspace")
	}
}

func TestAskUserQuestion_Schema_HasExpectedFields(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(askSchema, &doc); err != nil {
		t.Fatalf("schema not valid JSON: %v", err)
	}
	props := doc["properties"].(map[string]any)
	for _, want := range []string{"question", "options"} {
		if _, ok := props[want]; !ok {
			t.Errorf("schema missing %q", want)
		}
	}
	required, _ := doc["required"].([]any)
	if len(required) != 1 || required[0] != "question" {
		t.Errorf("required = %v, want [question]", required)
	}
}

func TestAskTools_ReturnsOneTool(t *testing.T) {
	tools := AskTools(askapp.NewService())
	if len(tools) != 1 || tools[0].Name() != "AskUserQuestion" {
		t.Errorf("AskTools = %+v", tools)
	}
}

func TestAskUserQuestion_ValidateInput_RequiresQuestion(t *testing.T) {
	tool, _ := newTestTool()
	if err := tool.ValidateInput(json.RawMessage(`{}`)); !errors.Is(err, ErrEmptyQuestion) {
		t.Errorf("want ErrEmptyQuestion, got %v", err)
	}
	if err := tool.ValidateInput(json.RawMessage(`{"question":"  "}`)); !errors.Is(err, ErrEmptyQuestion) {
		t.Errorf("whitespace question should fail, got %v", err)
	}
}

func TestAskUserQuestion_ValidateInput_AcceptsValid(t *testing.T) {
	tool, _ := newTestTool()
	if err := tool.ValidateInput(json.RawMessage(`{"question":"yes?","options":["a","b"]}`)); err != nil {
		t.Errorf("unexpected: %v", err)
	}
}

func TestAskUserQuestion_Execute_NoToolCallID_FriendlyMessage(t *testing.T) {
	tool, _ := newTestTool()
	out, err := tool.Execute(context.Background(), `{"question":"hi"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "Cannot ask the user") {
		t.Errorf("expected runtime-not-initialized message, got: %q", out)
	}
}

func TestAskUserQuestion_Execute_AnswerArrivesBeforeTimeout(t *testing.T) {
	tool, svc := newTestTool()
	ctx := reqctxpkg.WithToolCallID(context.Background(), "call_xyz")

	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = svc.Resolve("call_xyz", "yes please")
	}()

	out, err := tool.Execute(ctx, `{"question":"go ahead?"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "yes please" {
		t.Errorf("answer = %q", out)
	}
}

func TestAskUserQuestion_Execute_TimeoutMessage(t *testing.T) {
	tool, _ := newTestTool()
	ctx := reqctxpkg.WithToolCallID(context.Background(), "call_timeout")
	start := time.Now()
	out, err := tool.Execute(ctx, `{"question":"any reply?"}`)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "did not respond") {
		t.Errorf("expected timeout message, got: %q", out)
	}
	if elapsed < 400*time.Millisecond || elapsed > 2*time.Second {
		t.Errorf("unexpected elapsed: %v (want ~500ms)", elapsed)
	}
}

func TestAskUserQuestion_Execute_CtxCancelMessage(t *testing.T) {
	tool, _ := newTestTool()
	ctx, cancel := context.WithCancel(reqctxpkg.WithToolCallID(context.Background(), "call_cancel"))
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	out, err := tool.Execute(ctx, `{"question":"hi"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "cancelled") {
		t.Errorf("expected cancellation message, got: %q", out)
	}
}
