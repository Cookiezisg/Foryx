// hooks_test.go — runs real shell hooks against /bin/sh -c.
//
// hooks_test.go ——经 /bin/sh -c 跑真实 shell hook。
package hooks

import (
	"context"
	"encoding/json"
	"testing"

	"go.uber.org/zap/zaptest"

	permdomain "github.com/sunweilin/forgify/backend/internal/domain/permissions"
)

type stubProvider struct{ s *permdomain.Settings }

func (p *stubProvider) GetRules() *permdomain.Settings { return p.s }

// shHook wraps a shell command into a HookSpec invoking /bin/sh -c.
//
// shHook 把 shell 命令包成调 /bin/sh -c 的 HookSpec。
func shHook(matcher, script string) permdomain.HookSpec {
	return permdomain.HookSpec{
		Matcher: matcher,
		Command: "/bin/sh",
		Args:    []string{"-c", script},
		Timeout: 5,
	}
}

func TestPreToolUse_AllowDecision(t *testing.T) {
	r := New(&stubProvider{s: &permdomain.Settings{
		Hooks: permdomain.HooksBlock{
			PreToolUse: []permdomain.HookSpec{
				shHook("Bash", `printf '{"decision":"allow","reason":"all good"}'`),
			},
		},
	}}, zaptest.NewLogger(t))
	d := r.FirePreToolUse(context.Background(), permdomain.HookInput{
		ToolName:  "Bash",
		ToolInput: json.RawMessage(`{"command":"ls"}`),
	})
	if d.Action != permdomain.ActionAllow {
		t.Errorf("decision = %+v, want allow", d)
	}
}

func TestPreToolUse_BlockingExitDeny(t *testing.T) {
	r := New(&stubProvider{s: &permdomain.Settings{
		Hooks: permdomain.HooksBlock{
			PreToolUse: []permdomain.HookSpec{
				shHook("Bash", `echo "dangerous command" >&2; exit 2`),
			},
		},
	}}, zaptest.NewLogger(t))
	d := r.FirePreToolUse(context.Background(), permdomain.HookInput{
		ToolName:  "Bash",
		ToolInput: json.RawMessage(`{"command":"rm -rf /"}`),
	})
	if d.Action != permdomain.ActionDeny {
		t.Errorf("decision = %+v, want deny", d)
	}
	if d.Reason != "dangerous command" {
		t.Errorf("reason = %q, want \"dangerous command\"", d.Reason)
	}
}

func TestPreToolUse_NonZeroNonBlocking(t *testing.T) {
	r := New(&stubProvider{s: &permdomain.Settings{
		Hooks: permdomain.HooksBlock{
			PreToolUse: []permdomain.HookSpec{
				shHook("Bash", `exit 1`),
				shHook("Bash", `printf '{"decision":"allow"}'`),
			},
		},
	}}, zaptest.NewLogger(t))
	d := r.FirePreToolUse(context.Background(), permdomain.HookInput{
		ToolName:  "Bash",
		ToolInput: json.RawMessage(`{"command":"ls"}`),
	})
	if d.Action != permdomain.ActionAllow {
		t.Errorf("first non-blocking error should not short-circuit; got %+v", d)
	}
}

func TestPreToolUse_MatcherFiltersOut(t *testing.T) {
	r := New(&stubProvider{s: &permdomain.Settings{
		Hooks: permdomain.HooksBlock{
			PreToolUse: []permdomain.HookSpec{
				shHook("Bash", `printf '{"decision":"deny"}'`),
			},
		},
	}}, zaptest.NewLogger(t))
	// Edit shouldn't match Bash matcher → no hook fires → empty decision.
	d := r.FirePreToolUse(context.Background(), permdomain.HookInput{
		ToolName:  "Edit",
		ToolInput: json.RawMessage(`{"file_path":"/tmp/x"}`),
	})
	if d.Action != "" {
		t.Errorf("non-matching matcher should yield empty decision; got %+v", d)
	}
}

func TestPreToolUse_MatcherRegexAlternatives(t *testing.T) {
	r := New(&stubProvider{s: &permdomain.Settings{
		Hooks: permdomain.HooksBlock{
			PreToolUse: []permdomain.HookSpec{
				shHook("Bash|Edit", `printf '{"decision":"deny","reason":"both blocked"}'`),
			},
		},
	}}, zaptest.NewLogger(t))
	d := r.FirePreToolUse(context.Background(), permdomain.HookInput{
		ToolName:  "Edit",
		ToolInput: json.RawMessage(`{"file_path":"/tmp/x"}`),
	})
	if d.Action != permdomain.ActionDeny {
		t.Errorf("Bash|Edit matcher should match Edit; got %+v", d)
	}
}

func TestPostToolUse_AggregatesInjects(t *testing.T) {
	r := New(&stubProvider{s: &permdomain.Settings{
		Hooks: permdomain.HooksBlock{
			PostToolUse: []permdomain.HookSpec{
				shHook("Edit", `printf '{"injectIntoNextTurn":"ran fmt"}'`),
				shHook("Edit", `printf '{"injectIntoNextTurn":"ran vet"}'`),
			},
		},
	}}, zaptest.NewLogger(t))
	got := r.FirePostToolUse(context.Background(), permdomain.HookInput{
		ToolName:  "Edit",
		ToolInput: json.RawMessage(`{"file_path":"/tmp/x"}`),
	})
	want := "ran fmt\nran vet"
	if got != want {
		t.Errorf("inject = %q, want %q", got, want)
	}
}

func TestStop_ContinueForcesAgentRetry(t *testing.T) {
	r := New(&stubProvider{s: &permdomain.Settings{
		Hooks: permdomain.HooksBlock{
			Stop: []permdomain.HookSpec{
				shHook("", `printf '{"decision":"continue","reason":"tests fail"}'`),
			},
		},
	}}, zaptest.NewLogger(t))
	cont, prompt := r.FireStop(context.Background(), permdomain.HookInput{})
	if !cont {
		t.Errorf("Stop hook with decision=continue should return true")
	}
	if prompt != "tests fail" {
		t.Errorf("prompt = %q, want \"tests fail\"", prompt)
	}
}

func TestStop_NoHooks_NoContinue(t *testing.T) {
	r := New(&stubProvider{s: &permdomain.Settings{}}, zaptest.NewLogger(t))
	cont, _ := r.FireStop(context.Background(), permdomain.HookInput{})
	if cont {
		t.Errorf("no Stop hooks should not continue")
	}
}
