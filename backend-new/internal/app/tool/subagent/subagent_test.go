package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

type fakeRunner struct {
	result             string
	err                error
	gotType, gotPrompt string
}

func (r *fakeRunner) Spawn(_ context.Context, agentType, prompt string) (string, error) {
	r.gotType, r.gotPrompt = agentType, prompt
	return r.result, r.err
}

func newTool(rn Runner) *Tool {
	return New(rn, []string{"Explore", "Plan", "general-purpose"})
}

func TestTool_Execute(t *testing.T) {
	rn := &fakeRunner{result: "subagent answer"}
	out, err := newTool(rn).Execute(context.Background(), `{"subagent_type":"Explore","prompt":"find X"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if out != "subagent answer" || rn.gotType != "Explore" || rn.gotPrompt != "find X" {
		t.Fatalf("spawn args/result wrong: out=%q type=%q prompt=%q", out, rn.gotType, rn.gotPrompt)
	}
}

func TestTool_RecursionRefused(t *testing.T) {
	rn := &fakeRunner{result: "x"}
	ctx := reqctxpkg.SetSubagentID(context.Background(), "subagt_parent")
	if _, err := newTool(rn).Execute(ctx, `{"subagent_type":"Explore","prompt":"nested"}`); err == nil {
		t.Fatal("Subagent tool must refuse to run inside a subagent")
	}
	if rn.gotType != "" {
		t.Fatal("runner must not be called when recursion is refused")
	}
}

func TestTool_SpawnError(t *testing.T) {
	rn := &fakeRunner{err: errors.New("boom")}
	if _, err := newTool(rn).Execute(context.Background(), `{"subagent_type":"Plan","prompt":"p"}`); err == nil {
		t.Fatal("a spawn error must surface as an error (loop renders it as tool_result)")
	}
}

func TestTool_ValidateInput(t *testing.T) {
	tl := newTool(&fakeRunner{})
	bad := []json.RawMessage{
		json.RawMessage(`{"subagent_type":"Explore"}`),           // missing prompt
		json.RawMessage(`{"prompt":"p"}`),                        // missing type
		json.RawMessage(`{"subagent_type":"Nope","prompt":"p"}`), // unknown type
	}
	for _, b := range bad {
		if err := tl.ValidateInput(b); err == nil {
			t.Fatalf("expected validation error for %s", b)
		}
	}
	if err := tl.ValidateInput(json.RawMessage(`{"subagent_type":"general-purpose","prompt":"p"}`)); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
}
