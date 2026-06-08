package handler

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"go.uber.org/zap"

	envfixapp "github.com/sunweilin/forgify/backend/internal/app/envfix"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	handlerinfra "github.com/sunweilin/forgify/backend/internal/infra/handler"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	handlerstore "github.com/sunweilin/forgify/backend/internal/infra/store/handler"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// --- fakes -----------------------------------------------------------------

type okSandbox struct{}

func (okSandbox) EnsureEnv(_ context.Context, _ sandboxdomain.Owner, spec sandboxdomain.EnvSpec, _ sandboxdomain.ProgressFunc) (*sandboxdomain.Env, error) {
	return &sandboxdomain.Env{Status: sandboxdomain.EnvStatusReady, Deps: spec.Deps}, nil
}

type fakePicker struct{}

func (fakePicker) Pick(context.Context, string) (modeldomain.ModelRef, error) {
	return modeldomain.ModelRef{APIKeyID: "ak", ModelID: "m"}, nil
}

type fakeKeys struct{}

func (fakeKeys) ResolveCredentialsByID(context.Context, string) (apikeydomain.Credentials, error) {
	return apikeydomain.Credentials{Provider: "mock"}, nil
}
func (fakeKeys) MarkInvalidByID(context.Context, string, string) error { return nil }

type fakeEncryptor struct{}

func (fakeEncryptor) Encrypt(_ context.Context, pt []byte) ([]byte, error) { return pt, nil }
func (fakeEncryptor) Decrypt(_ context.Context, ct []byte) ([]byte, error) { return ct, nil }

type fakeHandle struct{ killed bool }

func (h *fakeHandle) Stdin() io.WriteCloser { return nopWC{} }
func (h *fakeHandle) Stdout() io.ReadCloser { return io.NopCloser(strings.NewReader("")) }
func (h *fakeHandle) Stderr() io.ReadCloser { return io.NopCloser(strings.NewReader("")) }
func (h *fakeHandle) Wait() error           { return nil }
func (h *fakeHandle) Kill() error           { h.killed = true; return nil }
func (h *fakeHandle) PID() int              { return 4321 }

type nopWC struct{}

func (nopWC) Write(p []byte) (int, error) { return len(p), nil }
func (nopWC) Close() error                { return nil }

type fakeRunner struct {
	spawns  int
	handles []*fakeHandle
}

func (r *fakeRunner) Ready() bool { return true }
func (r *fakeRunner) Spawn(_ context.Context, _ sandboxdomain.Owner, _, _, _ string) (sandboxdomain.LongLivedHandle, error) {
	r.spawns++
	h := &fakeHandle{}
	r.handles = append(r.handles, h)
	return h, nil
}
func (r *fakeRunner) Destroy(context.Context, string) error { return nil }

type fakeClient struct {
	calls   int
	crashed bool
	result  any
	callErr error
}

func (c *fakeClient) Init(context.Context, map[string]any) error { return nil }
func (c *fakeClient) Call(context.Context, string, map[string]any) (any, error) {
	c.calls++
	return c.result, c.callErr
}
func (c *fakeClient) StreamCall(ctx context.Context, m string, a map[string]any, _ func(any)) (any, error) {
	return c.Call(ctx, m, a)
}
func (c *fakeClient) Shutdown(context.Context) error { return nil }
func (c *fakeClient) Crashed() bool                  { return c.crashed }

// clientLog records every fake client the factory mints (one per spawn).
type clientLog struct{ clients []*fakeClient }

func (cl *clientLog) factory(io.WriteCloser, io.Reader, *zap.Logger) handlerinfra.Client {
	c := &fakeClient{result: "ok"}
	cl.clients = append(cl.clients, c)
	return c
}

// --- harness ---------------------------------------------------------------

func newSvc(t *testing.T) (*Service, *fakeRunner, *clientLog, context.Context) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, stmt := range handlerstore.Schema {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	repo := handlerstore.New(ormpkg.Open(sqlDB))
	prov := envfixapp.NewProvisioner(okSandbox{}, fakePicker{}, fakeKeys{}, llminfra.NewFactory(), zap.NewNop())
	runner := &fakeRunner{}
	cl := &clientLog{}
	svc := NewService(repo, prov, runner, fakeEncryptor{}, cl.factory, nil, zap.NewNop())
	return svc, runner, cl, reqctxpkg.SetWorkspaceID(context.Background(), "ws_1")
}

func createOps(t *testing.T, name string, reqArg bool) []Op {
	t.Helper()
	arr := `[{"op":"set_meta","name":"` + name + `","description":"d"},{"op":"add_method","method":{"name":"ping","args":[],"body":"return 1"}}`
	if reqArg {
		arr += `,{"op":"set_init_args_schema","args":[{"name":"api_key","type":"string","required":true,"sensitive":true}]}`
	}
	arr += `]`
	ops, err := ParseOps([]byte(arr))
	if err != nil {
		t.Fatalf("createOps: %v", err)
	}
	return ops
}

// editDepsOps is a non-conflicting edit (changes deps only) for version-bump tests.
func editDepsOps(t *testing.T) []Op {
	t.Helper()
	ops, err := ParseOps([]byte(`[{"op":"set_dependencies","dependencies":["requests"]}]`))
	if err != nil {
		t.Fatalf("editDepsOps: %v", err)
	}
	return ops
}

// --- tests -----------------------------------------------------------------

func TestCreate_NoEagerSpawn(t *testing.T) {
	svc, runner, _, ctx := newSvc(t)
	h, v, err := svc.Create(ctx, CreateInput{Ops: createOps(t, "alpha", false)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if v.Version != 1 || h.ActiveVersionID != v.ID {
		t.Fatalf("v1 not active: %+v", v)
	}
	if runner.spawns != 0 {
		t.Fatalf("create should not spawn an instance, got %d", runner.spawns)
	}
	if got := svc.manager.State(h.ID); got != handlerdomain.RuntimeStateStopped {
		t.Fatalf("runtime state = %q, want stopped", got)
	}
}

func TestCall_SpawnsRecordsReuses(t *testing.T) {
	svc, runner, cl, ctx := newSvc(t)
	h, _, _ := svc.Create(ctx, CreateInput{Ops: createOps(t, "a", false)})

	res, err := svc.Call(ctx, CallInput{HandlerID: h.ID, Method: "ping", Args: map[string]any{}})
	if err != nil || res != "ok" {
		t.Fatalf("call: res=%v err=%v", res, err)
	}
	if runner.spawns != 1 || svc.manager.State(h.ID) != handlerdomain.RuntimeStateRunning {
		t.Fatalf("first call should spawn + run: spawns=%d state=%s", runner.spawns, svc.manager.State(h.ID))
	}
	// second call reuses the resident instance
	if _, err := svc.Call(ctx, CallInput{HandlerID: h.ID, Method: "ping", Args: map[string]any{}}); err != nil {
		t.Fatalf("call 2: %v", err)
	}
	if runner.spawns != 1 {
		t.Fatalf("second call should reuse instance, spawns=%d", runner.spawns)
	}
	if cl.clients[0].calls != 2 {
		t.Fatalf("client should have served 2 calls, got %d", cl.clients[0].calls)
	}
	// the call was recorded
	page, _ := svc.SearchCalls(ctx, handlerdomain.CallFilter{HandlerID: h.ID})
	if page.Count != 2 || page.Aggregates.OKCount != 2 {
		t.Fatalf("calls not recorded: %+v", page)
	}
}

func TestRestart_StopsThenRespawns(t *testing.T) {
	svc, runner, cl, ctx := newSvc(t)
	h, _, _ := svc.Create(ctx, CreateInput{Ops: createOps(t, "a", false)})
	_, _ = svc.Call(ctx, CallInput{HandlerID: h.ID, Method: "ping", Args: map[string]any{}})

	state, err := svc.Restart(ctx, h.ID)
	if err != nil || state != handlerdomain.RuntimeStateRunning {
		t.Fatalf("restart: state=%s err=%v", state, err)
	}
	if runner.spawns != 2 {
		t.Fatalf("restart should respawn: spawns=%d", runner.spawns)
	}
	if !runner.handles[0].killed {
		t.Fatal("restart should have killed the old handle")
	}
	if len(cl.clients) != 2 {
		t.Fatalf("restart should mint a fresh client, got %d", len(cl.clients))
	}
}

func TestCrash_RespawnsOnNextCall(t *testing.T) {
	svc, runner, cl, ctx := newSvc(t)
	h, _, _ := svc.Create(ctx, CreateInput{Ops: createOps(t, "a", false)})
	_, _ = svc.Call(ctx, CallInput{HandlerID: h.ID, Method: "ping", Args: map[string]any{}})

	cl.clients[0].crashed = true // process died
	if got := svc.manager.State(h.ID); got != handlerdomain.RuntimeStateCrashed {
		t.Fatalf("state should be crashed, got %q", got)
	}
	if _, err := svc.Call(ctx, CallInput{HandlerID: h.ID, Method: "ping", Args: map[string]any{}}); err != nil {
		t.Fatalf("call after crash: %v", err)
	}
	if runner.spawns != 2 {
		t.Fatalf("crashed instance should respawn on next call: spawns=%d", runner.spawns)
	}
}

func TestEdit_BumpsVersionAndRestarts(t *testing.T) {
	svc, runner, _, ctx := newSvc(t)
	h, _, _ := svc.Create(ctx, CreateInput{Ops: createOps(t, "a", false)})
	_, _ = svc.Call(ctx, CallInput{HandlerID: h.ID, Method: "ping", Args: map[string]any{}}) // running (spawns=1)

	v2, err := svc.Edit(ctx, EditInput{ID: h.ID, Ops: editDepsOps(t)})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if v2.Version != 2 {
		t.Fatalf("edit version = %d, want 2", v2.Version)
	}
	if runner.spawns != 2 {
		t.Fatalf("edit should restart the resident instance: spawns=%d", runner.spawns)
	}
}

func TestRevert_PointerOnly(t *testing.T) {
	svc, _, _, ctx := newSvc(t)
	h, _, _ := svc.Create(ctx, CreateInput{Ops: createOps(t, "a", false)})
	if _, err := svc.Edit(ctx, EditInput{ID: h.ID, Ops: editDepsOps(t)}); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if _, err := svc.Revert(ctx, h.ID, 1); err != nil {
		t.Fatalf("revert: %v", err)
	}
	got, _ := svc.Get(ctx, h.ID)
	if got.ActiveVersion == nil || got.ActiveVersion.Version != 1 {
		t.Fatalf("active should be v1 after revert, got %+v", got.ActiveVersion)
	}
	if _, err := svc.GetVersionByNumber(ctx, h.ID, 2); err != nil {
		t.Fatalf("v2 must survive revert, got %v", err)
	}
}

func TestConfig_GatesSpawn(t *testing.T) {
	svc, runner, _, ctx := newSvc(t)
	h, _, _ := svc.Create(ctx, CreateInput{Ops: createOps(t, "a", true)}) // requires api_key

	// call before config → ErrConfigIncomplete, no spawn
	_, err := svc.Call(ctx, CallInput{HandlerID: h.ID, Method: "ping", Args: map[string]any{}})
	if !errors.Is(err, handlerdomain.ErrConfigIncomplete) {
		t.Fatalf("want ErrConfigIncomplete, got %v", err)
	}
	if runner.spawns != 0 {
		t.Fatalf("should not spawn without config, spawns=%d", runner.spawns)
	}

	// set config → UpdateConfig restarts → now running
	if err := svc.UpdateConfig(ctx, h.ID, map[string]any{"api_key": "secret"}); err != nil {
		t.Fatalf("update config: %v", err)
	}
	if svc.manager.State(h.ID) != handlerdomain.RuntimeStateRunning {
		t.Fatalf("config complete should start the instance, state=%s", svc.manager.State(h.ID))
	}
	if _, err := svc.Call(ctx, CallInput{HandlerID: h.ID, Method: "ping", Args: map[string]any{}}); err != nil {
		t.Fatalf("call after config: %v", err)
	}
}

func TestShutdown_StopsAll(t *testing.T) {
	svc, _, _, ctx := newSvc(t)
	h, _, _ := svc.Create(ctx, CreateInput{Ops: createOps(t, "a", false)})
	_, _ = svc.Call(ctx, CallInput{HandlerID: h.ID, Method: "ping", Args: map[string]any{}})
	svc.Shutdown(ctx)
	if got := svc.manager.State(h.ID); got != handlerdomain.RuntimeStateStopped {
		t.Fatalf("after shutdown state = %q, want stopped", got)
	}
}

func TestAssembleClass(t *testing.T) {
	d := &VersionDraft{
		Imports:        "import requests",
		InitBody:       "self.session = requests.Session()",
		ShutdownBody:   "self.session.close()",
		InitArgsSchema: []handlerdomain.InitArgSpec{{Name: "api_key", Type: "string", Required: true}},
		Methods:        []handlerdomain.MethodSpec{{Name: "fetch", Inputs: []schemapkg.Field{{Name: "url", Type: schemapkg.TypeString}}, Body: "return self.session.get(url).json()"}},
	}
	out := AssembleClass(d)
	for _, want := range []string{"class HandlerImpl:", "def __init__(self, api_key: str):", "def shutdown(self):", "def fetch(self, url: str):", "import requests"} {
		if !strings.Contains(out, want) {
			t.Fatalf("assembled class missing %q:\n%s", want, out)
		}
	}
}
