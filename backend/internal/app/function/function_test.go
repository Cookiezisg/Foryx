package function

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"go.uber.org/zap"

	envfixapp "github.com/sunweilin/anselm/backend/internal/app/envfix"
	apikeydomain "github.com/sunweilin/anselm/backend/internal/domain/apikey"
	functiondomain "github.com/sunweilin/anselm/backend/internal/domain/function"
	modeldomain "github.com/sunweilin/anselm/backend/internal/domain/model"
	sandboxdomain "github.com/sunweilin/anselm/backend/internal/domain/sandbox"
	llminfra "github.com/sunweilin/anselm/backend/internal/infra/llm"
	functionstore "github.com/sunweilin/anselm/backend/internal/infra/store/function"
	ormpkg "github.com/sunweilin/anselm/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

// --- fakes -----------------------------------------------------------------

// okSandbox satisfies envfix.SandboxPort: every install succeeds immediately, so app
// tests focus on the version model (env-fix retry logic is covered in envfix_test).
type okSandbox struct{}

func (okSandbox) EnsureEnv(_ context.Context, _ sandboxdomain.Owner, spec sandboxdomain.EnvSpec, _ sandboxdomain.ProgressFunc) (*sandboxdomain.Env, error) {
	return &sandboxdomain.Env{Status: sandboxdomain.EnvStatusReady, Deps: spec.Deps}, nil
}

type fakeRunner struct {
	ran    int
	result *functiondomain.ExecutionResult
}

func (f *fakeRunner) Ready() bool { return true }
func (f *fakeRunner) Run(_ context.Context, _ sandboxdomain.Owner, _, _, _ string, _ map[string]any) (*functiondomain.ExecutionResult, error) {
	f.ran++
	if f.result != nil {
		return f.result, nil
	}
	return &functiondomain.ExecutionResult{OK: true, Output: "ok"}, nil
}
func (f *fakeRunner) Destroy(context.Context, string) error { return nil }

type fakePicker struct{}

func (fakePicker) Pick(context.Context, string) (modeldomain.ModelRef, error) {
	return modeldomain.ModelRef{APIKeyID: "ak", ModelID: "m"}, nil
}

type fakeKeys struct{}

func (fakeKeys) ResolveCredentialsByID(context.Context, string) (apikeydomain.Credentials, error) {
	return apikeydomain.Credentials{Provider: "mock"}, nil
}
func (fakeKeys) MarkInvalidByID(context.Context, string, string) error { return nil }

// --- harness ---------------------------------------------------------------

func newSvc(t *testing.T) (*Service, *fakeRunner, context.Context) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, stmt := range functionstore.Schema {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	repo := functionstore.New(ormpkg.Open(sqlDB))
	prov := envfixapp.NewProvisioner(okSandbox{}, fakePicker{}, fakeKeys{}, llminfra.NewFactory(), zap.NewNop())
	runner := &fakeRunner{}
	svc := NewService(repo, prov, runner, nil, zap.NewNop())
	return svc, runner, reqctxpkg.SetWorkspaceID(context.Background(), "ws_1")
}

func createOps(t *testing.T, name, code string, deps ...string) []Op {
	t.Helper()
	arr := fmt.Sprintf(`[{"op":"set_meta","name":%q,"description":"d"},{"op":"set_code","code":%q}`, name, code)
	if len(deps) > 0 {
		arr += `,{"op":"set_dependencies","dependencies":[`
		for i, d := range deps {
			if i > 0 {
				arr += ","
			}
			arr += fmt.Sprintf("%q", d)
		}
		arr += `]}`
	}
	arr += `]`
	ops, err := ParseOps([]byte(arr))
	if err != nil {
		t.Fatalf("createOps: %v", err)
	}
	return ops
}

const goodCode = "def main():\n    return 1"

// --- tests -----------------------------------------------------------------

func TestCreate_V1Active(t *testing.T) {
	svc, _, ctx := newSvc(t)
	f, v, err := svc.Create(ctx, CreateInput{Ops: createOps(t, "alpha", goodCode)})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if v.Version != 1 || f.ActiveVersionID != v.ID {
		t.Fatalf("v1 not active: version=%d active=%s vid=%s", v.Version, f.ActiveVersionID, v.ID)
	}
	if v.EnvStatus != functiondomain.EnvStatusReady {
		t.Fatalf("env not ready: %s", v.EnvStatus)
	}
}

func TestCreate_DuplicateName(t *testing.T) {
	svc, _, ctx := newSvc(t)
	if _, _, err := svc.Create(ctx, CreateInput{Ops: createOps(t, "dup", goodCode)}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, _, err := svc.Create(ctx, CreateInput{Ops: createOps(t, "dup", goodCode)}); !errors.Is(err, functiondomain.ErrDuplicateName) {
		t.Fatalf("want ErrDuplicateName, got %v", err)
	}
}

func TestCreate_InvalidCode(t *testing.T) {
	svc, _, ctx := newSvc(t)
	_, _, err := svc.Create(ctx, CreateInput{Ops: createOps(t, "bad", "x = 1  # no def")})
	if !errors.Is(err, functiondomain.ErrInvalidCode) {
		t.Fatalf("want ErrInvalidCode, got %v", err)
	}
}

func TestEdit_MovesPointerAndBumpsNumber(t *testing.T) {
	svc, _, ctx := newSvc(t)
	f, _, _ := svc.Create(ctx, CreateInput{Ops: createOps(t, "a", goodCode)})

	v2, err := svc.Edit(ctx, EditInput{ID: f.ID, Ops: createOps(t, "a", "def main():\n    return 2")})
	if err != nil || v2.Version != 2 {
		t.Fatalf("edit v2: version=%d err=%v", v2.Version, err)
	}
	got, _ := svc.Get(ctx, f.ID)
	if got.ActiveVersionID != v2.ID {
		t.Fatalf("active should be v2, got %s", got.ActiveVersionID)
	}
}

// TestRevert_PointerOnly_ForksFromActive is the core of version model A: revert moves
// the pointer without deleting newer versions, and a subsequent edit forks from the
// (reverted) active version with a fresh monotonic number.
func TestRevert_PointerOnly_ForksFromActive(t *testing.T) {
	svc, _, ctx := newSvc(t)
	// v1 deps=[x]
	f, _, _ := svc.Create(ctx, CreateInput{Ops: createOps(t, "a", goodCode, "x")})
	// v2 deps=[z] (active=v2)
	if _, err := svc.Edit(ctx, EditInput{ID: f.ID, Ops: createOps(t, "a", goodCode, "z")}); err != nil {
		t.Fatalf("edit v2: %v", err)
	}
	// revert to v1 (pointer only)
	if _, err := svc.Revert(ctx, f.ID, 1); err != nil {
		t.Fatalf("revert: %v", err)
	}
	got, _ := svc.Get(ctx, f.ID)
	if got.ActiveVersion == nil || got.ActiveVersion.Version != 1 {
		t.Fatalf("active should be v1 after revert, got %+v", got.ActiveVersion)
	}
	// v2 still exists (not deleted by revert)
	if _, err := svc.GetVersionByNumber(ctx, f.ID, 2); err != nil {
		t.Fatalf("v2 must survive revert, got %v", err)
	}
	// edit (set_code only) forks from active v1 → new version is 3, deps carried from v1 ([x] not [z])
	v3, err := svc.Edit(ctx, EditInput{ID: f.ID, Ops: editCodeOps(t, "def main():\n    return 9")})
	if err != nil {
		t.Fatalf("edit after revert: %v", err)
	}
	if v3.Version != 3 {
		t.Fatalf("post-revert edit should be v3, got %d", v3.Version)
	}
	if len(v3.Dependencies) != 1 || v3.Dependencies[0] != "x" {
		t.Fatalf("edit should fork from active v1 (deps [x]), got %v", v3.Dependencies)
	}
}

func TestEdit_EmptyOpsRebuildsEnv(t *testing.T) {
	svc, _, ctx := newSvc(t)
	f, v1, _ := svc.Create(ctx, CreateInput{Ops: createOps(t, "a", goodCode)})
	v, err := svc.Edit(ctx, EditInput{ID: f.ID, Ops: nil})
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if v.ID != v1.ID { // no new version, same active
		t.Fatalf("empty-ops edit should not create a new version: %s vs %s", v.ID, v1.ID)
	}
}

func TestRun_RecordsExecution(t *testing.T) {
	svc, runner, ctx := newSvc(t)
	f, _, _ := svc.Create(ctx, CreateInput{Ops: createOps(t, "a", goodCode)})

	res, err := svc.RunFunction(ctx, RunInput{FunctionID: f.ID, Input: map[string]any{"k": "v"}, TriggeredBy: functiondomain.TriggeredByChat})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !res.OK || runner.ran != 1 {
		t.Fatalf("run not executed: ok=%v ran=%d", res.OK, runner.ran)
	}
	page, err := svc.SearchExecutions(ctx, functiondomain.ExecutionFilter{FunctionID: f.ID})
	if err != nil || len(page.Executions) != 1 {
		t.Fatalf("execution not recorded: count=%d err=%v", len(page.Executions), err)
	}
	if page.Aggregates.OKCount != 1 {
		t.Fatalf("aggregates: %+v", page.Aggregates)
	}
}

func TestDelete_Removes(t *testing.T) {
	svc, _, ctx := newSvc(t)
	f, _, _ := svc.Create(ctx, CreateInput{Ops: createOps(t, "a", goodCode)})
	if err := svc.Delete(ctx, f.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(ctx, f.ID); !errors.Is(err, functiondomain.ErrNotFound) {
		t.Fatalf("deleted function should be NotFound, got %v", err)
	}
}

// TestEntryFuncName_TopLevelOnly — regression for F23 (iteration loop): the spawn driver calls
// entryFuncName's result by name, so it must pick the first COLUMN-0 def. An indented def (a class
// method / nested def) physically preceding the real entry must be skipped — otherwise the driver
// calls an indented name and the run dies with NameError. First-top-level-def-wins is preserved.
func TestEntryFuncName_TopLevelOnly(t *testing.T) {
	cases := []struct{ name, code, want string }{
		{"plain entry", "def main(x):\n    return x", "main"},
		{"class method before entry skipped", "class Calc:\n    def multiply(self, a, b):\n        return a * b\n\ndef process_order(items):\n    return {}", "process_order"},
		{"nested def before later body still picks outer", "def outer():\n    def inner():\n        pass\n    return inner", "outer"},
		{"first top-level helper still wins (entry must be first)", "def _avg(xs):\n    return sum(xs) / len(xs)\n\ndef summarize(scores):\n    return {}", "_avg"},
		{"no top-level def", "    def indented_only(self):\n        pass", ""},
	}
	for _, c := range cases {
		if got := entryFuncName(c.code); got != c.want {
			t.Errorf("%s: entryFuncName = %q, want %q", c.name, got, c.want)
		}
	}
}

func editCodeOps(t *testing.T, code string) []Op {
	t.Helper()
	ops, err := ParseOps([]byte(fmt.Sprintf(`[{"op":"set_code","code":%q}]`, code)))
	if err != nil {
		t.Fatalf("editCodeOps: %v", err)
	}
	return ops
}
