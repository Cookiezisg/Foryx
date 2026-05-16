package envfix

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"testing"

	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
)

type fakeLLM struct {
	responses []string
	errs      []error
	idx       int
}

func (f *fakeLLM) Stream(ctx context.Context, req llminfra.Request) iter.Seq[llminfra.StreamEvent] {
	return func(yield func(llminfra.StreamEvent) bool) {
		if f.idx >= len(f.responses) {
			return
		}
		i := f.idx
		f.idx++
		if i < len(f.errs) && f.errs[i] != nil {
			yield(llminfra.StreamEvent{Type: llminfra.EventError, Err: f.errs[i]})
			return
		}
		yield(llminfra.StreamEvent{Type: llminfra.EventText, Delta: f.responses[i]})
	}
}

func bundle(c llminfra.Client) *llmclientpkg.Bundle {
	return &llmclientpkg.Bundle{Client: c, ModelID: "test", Key: "k", BaseURL: ""}
}

func TestFakeLLM_GenerateRoundtrip(t *testing.T) {
	llm := &fakeLLM{responses: []string{`{"deps":["v1"]}`}}
	out, err := llminfra.Generate(context.Background(), llm, llminfra.Request{ModelID: "x"})
	if err != nil {
		t.Fatalf("Generate err: %v", err)
	}
	if out != `{"deps":["v1"]}` {
		t.Errorf("Generate returned %q, want json", out)
	}
}

func TestRunLoop_InitialReadyReturnsImmediately(t *testing.T) {
	calls := 0
	r := RunLoop(context.Background(), Options{
		Bundle:         bundle(&fakeLLM{}),
		InitialAttempt: Attempt{Number: 1, Deps: []string{"x"}, EnvStatus: "ready"},
		ApplyDeps: func(context.Context, []string) (string, string, error) {
			calls++
			return "", "", nil
		},
	})
	if r.FinalEnvStatus != "ready" {
		t.Errorf("want ready, got %s", r.FinalEnvStatus)
	}
	if r.AttemptsUsed != 1 {
		t.Errorf("want 1 attempt, got %d", r.AttemptsUsed)
	}
	if calls != 0 {
		t.Errorf("ApplyDeps should not be called when initial=ready, got %d calls", calls)
	}
}

func TestRunLoop_RetrySucceedsOnAttempt2(t *testing.T) {
	llm := &fakeLLM{responses: []string{`{"deps":["pandas>=2.0"]}`}}
	applied := [][]string{}
	r := RunLoop(context.Background(), Options{
		Bundle:         bundle(llm),
		InitialAttempt: Attempt{Number: 1, Deps: []string{"pandass"}, EnvStatus: "failed", EnvError: "No matching distribution found for pandass"},
		ApplyDeps: func(_ context.Context, deps []string) (string, string, error) {
			applied = append(applied, deps)
			return "ready", "", nil
		},
	})
	if r.FinalEnvStatus != "ready" {
		t.Errorf("want ready, got %s", r.FinalEnvStatus)
	}
	if r.AttemptsUsed != 2 {
		t.Errorf("want 2 attempts, got %d", r.AttemptsUsed)
	}
	if len(applied) != 1 || applied[0][0] != "pandas>=2.0" {
		t.Errorf("ApplyDeps got unexpected deps: %v", applied)
	}
	if len(r.History) != 2 {
		t.Errorf("want 2 history entries, got %d", len(r.History))
	}
}

func TestRunLoop_ExhaustsMaxAttempts(t *testing.T) {
	llm := &fakeLLM{responses: []string{
		`{"deps":["v1"]}`,
		`{"deps":["v2"]}`,
	}}
	r := RunLoop(context.Background(), Options{
		Bundle:         bundle(llm),
		InitialAttempt: Attempt{Number: 1, Deps: []string{"v0"}, EnvStatus: "failed", EnvError: "err0"},
		MaxAttempts:    3,
		ApplyDeps: func(_ context.Context, deps []string) (string, string, error) {
			return "failed", "err-" + deps[0], nil
		},
	})
	if r.FinalEnvStatus != "failed" {
		t.Errorf("want failed, got %s", r.FinalEnvStatus)
	}
	if r.AttemptsUsed != 3 {
		t.Errorf("want 3 attempts, got %d", r.AttemptsUsed)
	}
	if r.FinalEnvError != "err-v2" {
		t.Errorf("want last err, got %q", r.FinalEnvError)
	}
}

func TestRunLoop_LLMCallFailureTreatedAsInstallFail(t *testing.T) {
	llm := &fakeLLM{
		responses: []string{""},
		errs:      []error{errors.New("network blip")},
	}
	r := RunLoop(context.Background(), Options{
		Bundle:         bundle(llm),
		InitialAttempt: Attempt{Number: 1, Deps: []string{"x"}, EnvStatus: "failed", EnvError: "boom"},
		ApplyDeps: func(context.Context, []string) (string, string, error) {
			t.Fatalf("ApplyDeps should not run after LLM failure")
			return "", "", nil
		},
	})
	if r.FinalEnvStatus != "failed" {
		t.Errorf("want failed, got %s", r.FinalEnvStatus)
	}
	if r.AttemptsUsed != 1 {
		t.Errorf("want attemptsUsed=1, got %d", r.AttemptsUsed)
	}
	if !strings.Contains(r.FinalEnvError, "env-fix LLM call failed") {
		t.Errorf("want LLM-failed reason in error, got %q", r.FinalEnvError)
	}
}

func TestRunLoop_LLMReturnsMalformedJSON(t *testing.T) {
	llm := &fakeLLM{responses: []string{"not json at all"}}
	r := RunLoop(context.Background(), Options{
		Bundle:         bundle(llm),
		InitialAttempt: Attempt{Number: 1, Deps: []string{"x"}, EnvStatus: "failed", EnvError: "boom"},
		ApplyDeps:      func(context.Context, []string) (string, string, error) { return "", "", nil },
	})
	if r.FinalEnvStatus != "failed" {
		t.Errorf("want failed, got %s", r.FinalEnvStatus)
	}
	if r.AttemptsUsed != 1 {
		t.Errorf("want attemptsUsed=1, got %d", r.AttemptsUsed)
	}
}

func TestRunLoop_ApplyDepsFatalError(t *testing.T) {
	llm := &fakeLLM{responses: []string{`{"deps":["v1"]}`}}
	fatal := errors.New("sandbox down")
	r := RunLoop(context.Background(), Options{
		Bundle:         bundle(llm),
		InitialAttempt: Attempt{Number: 1, Deps: []string{"v0"}, EnvStatus: "failed", EnvError: "err0"},
		ApplyDeps: func(context.Context, []string) (string, string, error) {
			return "", "", fatal
		},
	})
	if r.FinalEnvStatus != "failed" {
		t.Errorf("want failed, got %s", r.FinalEnvStatus)
	}
	if !errors.Is(r.FatalErr, fatal) {
		t.Errorf("want FatalErr=sandbox down, got %v", r.FatalErr)
	}
}

func TestRunLoop_NilBundle(t *testing.T) {
	r := RunLoop(context.Background(), Options{
		Bundle:         nil,
		InitialAttempt: Attempt{Number: 1, Deps: []string{"x"}, EnvStatus: "failed", EnvError: "boom"},
	})
	if !errors.Is(r.FatalErr, ErrNoBundle) {
		t.Errorf("want FatalErr=ErrNoBundle, got %v", r.FatalErr)
	}
	if r.AttemptsUsed != 1 {
		t.Errorf("want attemptsUsed=1, got %d", r.AttemptsUsed)
	}
}

func TestRunLoop_HooksFiredCorrectly(t *testing.T) {
	llm := &fakeLLM{responses: []string{
		`{"deps":["v1"]}`,
		`{"deps":["v2"]}`,
	}}
	results := []int{}
	fixings := []int{}
	r := RunLoop(context.Background(), Options{
		Bundle:         bundle(llm),
		InitialAttempt: Attempt{Number: 1, Deps: []string{"v0"}, EnvStatus: "failed"},
		MaxAttempts:    3,
		ApplyDeps: func(_ context.Context, deps []string) (string, string, error) {
			if deps[0] == "v2" {
				return "ready", "", nil
			}
			return "failed", "still bad", nil
		},
		Hooks: LoopHooks{
			OnAttemptResult: func(_ context.Context, a Attempt) { results = append(results, a.Number) },
			OnFixing:        func(_ context.Context, n int) { fixings = append(fixings, n) },
		},
	})
	if r.FinalEnvStatus != "ready" {
		t.Errorf("want ready, got %s", r.FinalEnvStatus)
	}
	if fmt.Sprint(results) != "[1 2 3]" {
		t.Errorf("OnAttemptResult fired for: %v, want [1 2 3]", results)
	}
	if fmt.Sprint(fixings) != "[2 3]" {
		t.Errorf("OnFixing fired for: %v, want [2 3]", fixings)
	}
}

func TestRunLoop_MaxAttemptsClamping(t *testing.T) {
	llm := &fakeLLM{responses: []string{
		`{"deps":["v1"]}`,
		`{"deps":["v2"]}`,
	}}
	r := RunLoop(context.Background(), Options{
		Bundle:         bundle(llm),
		InitialAttempt: Attempt{Number: 1, Deps: []string{"v0"}, EnvStatus: "failed"},
		MaxAttempts:    0,
		ApplyDeps: func(context.Context, []string) (string, string, error) {
			return "failed", "still bad", nil
		},
	})
	if r.AttemptsUsed != DefaultMaxAttempts {
		t.Errorf("want %d attempts, got %d", DefaultMaxAttempts, r.AttemptsUsed)
	}
}

func TestBuildPrompt_Shape(t *testing.T) {
	p := buildPrompt([]string{"pandas>=2.0"}, "ImportError", []Attempt{
		{Number: 1, Deps: []string{"pandass"}, EnvStatus: "failed", EnvError: "typo"},
		{Number: 2, Deps: []string{"pandas"}, EnvStatus: "failed", EnvError: "version conflict"},
	})
	mustContain := []string{
		"Current dependencies",
		"pandas>=2.0",
		"Last install error",
		"ImportError",
		"Prior attempts",
		"attempt 1",
		"Return JSON only",
		`{"deps":`,
	}
	for _, s := range mustContain {
		if !strings.Contains(p, s) {
			t.Errorf("prompt missing %q\nprompt:\n%s", s, p)
		}
	}
}
