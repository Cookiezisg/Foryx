package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"go.uber.org/zap"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func ctxWS(id string) context.Context { return reqctxpkg.SetWorkspaceID(context.Background(), id) }

// --- fakes -----------------------------------------------------------------

type fakeRepo struct {
	byID  map[string]*mcpdomain.Server
	calls []*mcpdomain.Call
}

func newFakeRepo() *fakeRepo { return &fakeRepo{byID: map[string]*mcpdomain.Server{}} }

func (r *fakeRepo) Save(_ context.Context, s *mcpdomain.Server) error {
	cp := *s
	r.byID[s.ID] = &cp
	return nil
}
func (r *fakeRepo) GetByID(_ context.Context, id string) (*mcpdomain.Server, error) {
	if s, ok := r.byID[id]; ok {
		cp := *s
		return &cp, nil
	}
	return nil, mcpdomain.ErrServerNotFound
}
func (r *fakeRepo) GetByName(_ context.Context, name string) (*mcpdomain.Server, error) {
	for _, s := range r.byID {
		if s.Name == name {
			cp := *s
			return &cp, nil
		}
	}
	return nil, mcpdomain.ErrServerNotFound
}
func (r *fakeRepo) List(_ context.Context) ([]*mcpdomain.Server, error) {
	out := make([]*mcpdomain.Server, 0, len(r.byID))
	for _, s := range r.byID {
		cp := *s
		out = append(out, &cp)
	}
	return out, nil
}
func (r *fakeRepo) Delete(_ context.Context, id string) error {
	if _, ok := r.byID[id]; !ok {
		return mcpdomain.ErrServerNotFound
	}
	delete(r.byID, id)
	return nil
}
func (r *fakeRepo) SaveCall(_ context.Context, c *mcpdomain.Call) error {
	r.calls = append(r.calls, c)
	return nil
}
func (r *fakeRepo) GetCall(_ context.Context, id string) (*mcpdomain.Call, error) {
	for _, c := range r.calls {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, mcpdomain.ErrCallNotFound
}
func (r *fakeRepo) ListCalls(_ context.Context, _ mcpdomain.CallFilter) ([]*mcpdomain.Call, string, error) {
	return r.calls, "", nil
}

type fakeSandbox struct{ ensureErr error }

func (f *fakeSandbox) EnsureEnv(context.Context, sandboxdomain.Owner, sandboxdomain.EnvSpec, sandboxdomain.ProgressFunc) (*sandboxdomain.Env, error) {
	return &sandboxdomain.Env{}, f.ensureErr
}
func (f *fakeSandbox) SpawnLongLived(context.Context, sandboxdomain.Owner, sandboxdomain.SpawnOpts) (sandboxdomain.LongLivedHandle, error) {
	return &fakeHandle{}, nil
}

type fakeHandle struct{}

func (fakeHandle) Stdin() io.WriteCloser { return nopWC{} }
func (fakeHandle) Stdout() io.ReadCloser { return io.NopCloser(strings.NewReader("")) }
func (fakeHandle) Stderr() io.ReadCloser { return io.NopCloser(strings.NewReader("")) }
func (fakeHandle) Wait() error           { return nil }
func (fakeHandle) Kill() error           { return nil }
func (fakeHandle) PID() int              { return 1234 }

type nopWC struct{}

func (nopWC) Write(p []byte) (int, error) { return len(p), nil }
func (nopWC) Close() error                { return nil }

type fakeClient struct {
	tools      []mcpdomain.ToolDef
	callResult string
	initErr    error
	closed     bool
}

func (c *fakeClient) Initialize(context.Context) error { return c.initErr }
func (c *fakeClient) ListTools(context.Context) ([]mcpdomain.ToolDef, error) {
	return c.tools, nil
}
func (c *fakeClient) CallTool(context.Context, string, json.RawMessage) (string, error) {
	return c.callResult, nil
}
func (c *fakeClient) Close() error       { c.closed = true; return nil }
func (c *fakeClient) StderrTail() string { return "" }

type fakeRegistry struct{ entries []mcpdomain.RegistryEntry }

func (r *fakeRegistry) List(context.Context) ([]mcpdomain.RegistryEntry, error) {
	return r.entries, nil
}
func (r *fakeRegistry) Get(_ context.Context, name string) (*mcpdomain.RegistryEntry, error) {
	for i := range r.entries {
		if r.entries[i].Name == name {
			cp := r.entries[i]
			return &cp, nil
		}
	}
	return nil, mcpdomain.ErrRegistryEntryNotFound
}

// svcWith builds a Service with a fixed fake client (so CallTool reaches the same instance).
func svcWith(repo *fakeRepo, reg *fakeRegistry, fc *fakeClient) *Service {
	svc := New(repo, reg, &fakeSandbox{}, zap.NewNop())
	svc.SetClientFactory(func(mcpinfra.ClientSpec, *zap.Logger) mcpinfra.Client { return fc })
	return svc
}

func ctx7Registry() *fakeRegistry {
	return &fakeRegistry{entries: []mcpdomain.RegistryEntry{{
		Name:        "io.github.upstash/context7",
		Description: "Fetch latest library docs",
		Packages:    []mcpdomain.Package{{Name: "@upstash/context7-mcp", RuntimeHint: "npx"}},
	}}}
}

// --- tests -----------------------------------------------------------------

func TestInstall_ConnectsAndReportsTools(t *testing.T) {
	fc := &fakeClient{tools: []mcpdomain.ToolDef{{Name: "get-library-docs", Description: "..."}}}
	svc := svcWith(newFakeRepo(), ctx7Registry(), fc)
	st, err := svc.InstallFromRegistry(ctxWS("ws_1"), "io.github.upstash/context7", nil)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if st.Name != "context7" {
		t.Fatalf("want short name context7, got %q", st.Name)
	}
	if st.Status != mcpdomain.StatusReady {
		t.Fatalf("want ready, got %q", st.Status)
	}
	if len(st.Tools) != 1 || st.Tools[0].Name != "get-library-docs" {
		t.Fatalf("want 1 tool get-library-docs, got %v", st.Tools)
	}
}

func TestInstall_MissingEnv(t *testing.T) {
	reg := &fakeRegistry{entries: []mcpdomain.RegistryEntry{{
		Name:     "x/y",
		Packages: []mcpdomain.Package{{Name: "y-mcp", RuntimeHint: "npx", EnvVars: []mcpdomain.EnvVar{{Name: "API_KEY"}}}},
	}}}
	svc := svcWith(newFakeRepo(), reg, &fakeClient{})
	_, err := svc.InstallFromRegistry(ctxWS("ws_1"), "x/y", nil)
	if !errors.Is(err, mcpdomain.ErrEnvMissing) {
		t.Fatalf("want ErrEnvMissing, got %v", err)
	}
}

func TestCallTool_RoutesToClient(t *testing.T) {
	fc := &fakeClient{tools: []mcpdomain.ToolDef{{Name: "get-library-docs"}}, callResult: "DOCS"}
	repo := newFakeRepo()
	svc := svcWith(repo, ctx7Registry(), fc)
	ctx := ctxWS("ws_1")
	st, _ := svc.InstallFromRegistry(ctx, "io.github.upstash/context7", nil)
	res, err := svc.CallTool(ctx, st.ID, "get-library-docs", json.RawMessage(`{}`), "")
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res != "DOCS" {
		t.Fatalf("want DOCS, got %q", res)
	}
	// C4: every invocation records one mcp_calls audit row; "" derives chat off a plain ctx.
	// C4：每次调用记一行 mcp_calls 审计；"" 在裸 ctx 下推为 chat。
	if len(repo.calls) != 1 {
		t.Fatalf("want 1 recorded call, got %d", len(repo.calls))
	}
	c := repo.calls[0]
	if c.ServerID != st.ID || c.Tool != "get-library-docs" || c.Status != mcpdomain.CallStatusOK ||
		c.TriggeredBy != mcpdomain.CallTriggeredByChat || c.Output != "DOCS" {
		t.Fatalf("recorded call wrong: %+v", c)
	}
}

// TestCatalogSource_ReportsServerWithToolNames: catalog reports the server + ALL its tool
// names as Members (the container-entity contract).
//
// TestCatalogSource_ReportsServerWithToolNames：catalog 报 server + 它全部工具名为 Members（容器
// 实体契约）。
func TestCatalogSource_ReportsServerWithToolNames(t *testing.T) {
	fc := &fakeClient{tools: []mcpdomain.ToolDef{{Name: "get-library-docs"}, {Name: "resolve-id"}}}
	svc := svcWith(newFakeRepo(), ctx7Registry(), fc)
	ctx := ctxWS("ws_1")
	_, _ = svc.InstallFromRegistry(ctx, "io.github.upstash/context7", nil)

	items, err := svc.AsCatalogSource().ListItems(ctx)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 catalog item, got %d", len(items))
	}
	if items[0].Name != "context7" || items[0].Description != "Fetch latest library docs" {
		t.Fatalf("catalog name/desc: %+v", items[0])
	}
	if len(items[0].Members) != 2 || items[0].Members[0] != "get-library-docs" {
		t.Fatalf("want 2 tool-name Members, got %v", items[0].Members)
	}
}

func TestReconnect_RefreshesStatus(t *testing.T) {
	fc := &fakeClient{tools: []mcpdomain.ToolDef{{Name: "t"}}}
	svc := svcWith(newFakeRepo(), ctx7Registry(), fc)
	ctx := ctxWS("ws_1")
	_, _ = svc.InstallFromRegistry(ctx, "io.github.upstash/context7", nil)
	st, err := svc.Reconnect(ctx, "context7")
	if err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if st.Status != mcpdomain.StatusReady {
		t.Fatalf("want ready after reconnect, got %q", st.Status)
	}
}

func TestRemove_StopsAndDeletes(t *testing.T) {
	svc := svcWith(newFakeRepo(), ctx7Registry(), &fakeClient{})
	ctx := ctxWS("ws_1")
	_, _ = svc.InstallFromRegistry(ctx, "io.github.upstash/context7", nil)
	if err := svc.RemoveServer(ctx, "context7"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := svc.GetServer(ctx, "context7"); !errors.Is(err, mcpdomain.ErrServerNotFound) {
		t.Fatalf("removed server should be NotFound, got %v", err)
	}
}

func TestInstall_NameConflict(t *testing.T) {
	svc := svcWith(newFakeRepo(), ctx7Registry(), &fakeClient{})
	ctx := ctxWS("ws_1")
	_, _ = svc.InstallFromRegistry(ctx, "io.github.upstash/context7", nil)
	_, err := svc.InstallFromRegistry(ctx, "io.github.upstash/context7", nil)
	if !errors.Is(err, mcpdomain.ErrNameConflict) {
		t.Fatalf("want ErrNameConflict on re-install, got %v", err)
	}
}
