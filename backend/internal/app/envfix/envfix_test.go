package envfix

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/anselm/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/anselm/backend/internal/domain/model"
	sandboxdomain "github.com/sunweilin/anselm/backend/internal/domain/sandbox"
	llminfra "github.com/sunweilin/anselm/backend/internal/infra/llm"
)

// --- fakes -----------------------------------------------------------------

// fakeSandbox fails its first failFirst EnsureEnv calls, then succeeds; records the
// deps it was asked to install on each call so tests can assert the fix was applied.
type fakeSandbox struct {
	failFirst int
	calls     int
	seenDeps  [][]string
}

func (f *fakeSandbox) EnsureEnv(_ context.Context, _ sandboxdomain.Owner, spec sandboxdomain.EnvSpec, stream sandboxdomain.ProgressFunc) (*sandboxdomain.Env, error) {
	if stream != nil {
		stream("installing", "probe", -1) // envfix passes a non-nil no-op; exercise it
	}
	f.calls++
	f.seenDeps = append(f.seenDeps, append([]string(nil), spec.Deps...))
	if f.calls <= f.failFirst {
		return nil, fmt.Errorf("ERROR: no version satisfies %v", spec.Deps)
	}
	return &sandboxdomain.Env{Status: sandboxdomain.EnvStatusReady, Deps: spec.Deps}, nil
}

type fakePicker struct {
	ref modeldomain.ModelRef
	err error
}

func (f *fakePicker) Pick(_ context.Context, _ string) (modeldomain.ModelRef, error) {
	return f.ref, f.err
}

type fakeKeys struct {
	creds apikeydomain.Credentials
	err   error
}

func (f *fakeKeys) ResolveCredentialsByID(_ context.Context, _ string) (apikeydomain.Credentials, error) {
	return f.creds, f.err
}
func (f *fakeKeys) MarkInvalidByID(_ context.Context, _, _ string) error { return nil }

type recordingSink struct{ events []string }

func (r *recordingSink) OnAttempt(a Attempt) {
	r.events = append(r.events, fmt.Sprintf("attempt:%d:ok=%v", a.Number, a.OK))
}
func (r *recordingSink) OnFixing(n int) {
	r.events = append(r.events, fmt.Sprintf("fixing:%d", n))
}

// --- helpers ---------------------------------------------------------------

func depsScript(deps ...string) llminfra.MockScript {
	b, _ := json.Marshal(map[string]any{"deps": deps})
	return llminfra.MockScript{Events: []llminfra.StreamEvent{{Type: llminfra.EventText, Delta: string(b)}}}
}

// newProvisioner wires a Provisioner whose LLM path short-circuits to the mock
// client (creds.Provider == "mock").
func newProvisioner(t *testing.T, fs SandboxPort, picker modeldomain.ModelPicker, factory *llminfra.Factory) *Provisioner {
	t.Helper()
	return NewProvisioner(fs, picker, &fakeKeys{creds: apikeydomain.Credentials{Provider: "mock"}}, factory, zap.NewNop())
}

func okPicker() *fakePicker {
	return &fakePicker{ref: modeldomain.ModelRef{APIKeyID: "ak", ModelID: "m"}}
}

func baseRequest(deps []string, sink Sink) Request {
	return Request{
		Owner:   sandboxdomain.Owner{Kind: sandboxdomain.OwnerKindFunction, ID: "fn_x_fnenv_y"},
		Runtime: sandboxdomain.RuntimeSpec{Kind: "python"},
		Deps:    deps,
		Sink:    sink,
	}
}

// --- tests -----------------------------------------------------------------

func TestProvision_FirstTrySucceeds(t *testing.T) {
	fs := &fakeSandbox{failFirst: 0}
	factory := llminfra.NewFactory()
	p := newProvisioner(t, fs, okPicker(), factory)

	res := p.Provision(context.Background(), baseRequest([]string{"numpy"}, nil))

	if !res.OK || res.AttemptsUsed != 1 {
		t.Fatalf("want OK attempts=1, got OK=%v attempts=%d", res.OK, res.AttemptsUsed)
	}
	if !reflect.DeepEqual(res.FinalDeps, []string{"numpy"}) {
		t.Fatalf("FinalDeps = %v, want [numpy]", res.FinalDeps)
	}
	if factory.Mock().CallCount() != 0 {
		t.Fatalf("no LLM call expected on first-try success, got %d", factory.Mock().CallCount())
	}
}

func TestProvision_FixSucceeds(t *testing.T) {
	fs := &fakeSandbox{failFirst: 1}
	factory := llminfra.NewFactory()
	factory.Mock().PushScript(depsScript("pandas==2.0", "numpy"))
	p := newProvisioner(t, fs, okPicker(), factory)

	res := p.Provision(context.Background(), baseRequest([]string{"pandsa==2.0", "numpy"}, nil))

	if !res.OK || res.AttemptsUsed != 2 {
		t.Fatalf("want OK attempts=2, got OK=%v attempts=%d", res.OK, res.AttemptsUsed)
	}
	want := []string{"pandas==2.0", "numpy"}
	if !reflect.DeepEqual(res.FinalDeps, want) {
		t.Fatalf("FinalDeps = %v, want %v", res.FinalDeps, want)
	}
	// The second install must have used the LLM-corrected deps.
	if len(fs.seenDeps) != 2 || !reflect.DeepEqual(fs.seenDeps[1], want) {
		t.Fatalf("2nd install deps = %v, want %v", fs.seenDeps, want)
	}
	if len(res.History) != 2 || res.History[0].OK || !res.History[1].OK {
		t.Fatalf("History = %+v, want [fail, ok]", res.History)
	}
}

// TestProvision_RejectsDepDrop — F148: a "fix" that DROPS a declared package (shrinks below the
// original count) must be rejected. The retry must NOT install the shrunken set, the env stays failed
// (OK=false) with the real install error, and FinalDeps preserves what the user declared — never an
// empty, false-ready env that defers the failure to a runtime ModuleNotFoundError.
func TestProvision_RejectsDepDrop(t *testing.T) {
	fs := &fakeSandbox{failFirst: 1} // 1st install fails; an empty-set retry WOULD succeed
	factory := llminfra.NewFactory()
	factory.Mock().PushScript(depsScript()) // LLM "fixes" by dropping the package → {"deps":[]}
	p := newProvisioner(t, fs, okPicker(), factory)

	res := p.Provision(context.Background(), baseRequest([]string{"definitely-not-a-real-pkg"}, nil))

	if res.OK {
		t.Fatalf("dropping the declared package must NOT count as a fix (it would false-ready an env missing the pkg)")
	}
	if len(res.FinalDeps) != 1 || res.FinalDeps[0] != "definitely-not-a-real-pkg" {
		t.Fatalf("the declared dep must be PRESERVED, got %v", res.FinalDeps)
	}
	if fs.calls != 1 {
		t.Fatalf("the dep-dropping retry must never be installed, got %d install calls", fs.calls)
	}
}

func TestProvision_ExhaustsAttempts(t *testing.T) {
	fs := &fakeSandbox{failFirst: 3} // default max 3 → never succeeds
	factory := llminfra.NewFactory()
	factory.Mock().PushScript(depsScript("a"))
	factory.Mock().PushScript(depsScript("b"))
	p := newProvisioner(t, fs, okPicker(), factory)

	res := p.Provision(context.Background(), baseRequest([]string{"x"}, nil))

	if res.OK || res.AttemptsUsed != 3 {
		t.Fatalf("want fail attempts=3, got OK=%v attempts=%d", res.OK, res.AttemptsUsed)
	}
	if len(res.History) != 3 || fs.calls != 3 {
		t.Fatalf("History len=%d calls=%d, want 3/3", len(res.History), fs.calls)
	}
	// FinalDeps reflects the last tried (the second fix's "b").
	if !reflect.DeepEqual(res.FinalDeps, []string{"b"}) {
		t.Fatalf("FinalDeps = %v, want [b]", res.FinalDeps)
	}
}

func TestProvision_NoUtilityModelDegrades(t *testing.T) {
	fs := &fakeSandbox{failFirst: 3}
	factory := llminfra.NewFactory()
	// picker errors → suggestDeps fails before any LLM call → loop stops after 1 install.
	p := newProvisioner(t, fs, &fakePicker{err: modeldomain.ErrNotConfigured}, factory)

	res := p.Provision(context.Background(), baseRequest([]string{"x"}, nil))

	if res.OK || res.AttemptsUsed != 1 {
		t.Fatalf("want fail attempts=1 (no auto-fix), got OK=%v attempts=%d", res.OK, res.AttemptsUsed)
	}
	if fs.calls != 1 {
		t.Fatalf("sandbox calls = %d, want 1 (no retry without a fixer)", fs.calls)
	}
}

func TestProvision_SinkCallbackOrder(t *testing.T) {
	fs := &fakeSandbox{failFirst: 1}
	factory := llminfra.NewFactory()
	factory.Mock().PushScript(depsScript("pandas"))
	sink := &recordingSink{}
	p := newProvisioner(t, fs, okPicker(), factory)

	p.Provision(context.Background(), baseRequest([]string{"pandsa"}, sink))

	want := []string{"attempt:1:ok=false", "fixing:2", "attempt:2:ok=true"}
	if !reflect.DeepEqual(sink.events, want) {
		t.Fatalf("sink events = %v, want %v", sink.events, want)
	}
}

func TestProvision_NilSinkNoOp(t *testing.T) {
	fs := &fakeSandbox{failFirst: 1}
	factory := llminfra.NewFactory()
	factory.Mock().PushScript(depsScript("ok"))
	p := newProvisioner(t, fs, okPicker(), factory)

	// nil Sink must not panic.
	res := p.Provision(context.Background(), baseRequest([]string{"bad"}, nil))
	if !res.OK {
		t.Fatalf("want OK with nil sink, got %+v", res)
	}
}
