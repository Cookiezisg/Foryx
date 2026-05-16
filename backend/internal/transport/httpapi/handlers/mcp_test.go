package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

type fakeMCPClient struct {
	mu            sync.Mutex
	initErr       error
	listToolsResp []mcpdomain.ToolDef
	listToolsErr  error
	closed        bool
}

func (f *fakeMCPClient) Initialize(_ context.Context) error { return f.initErr }
func (f *fakeMCPClient) ListTools(_ context.Context) ([]mcpdomain.ToolDef, error) {
	return append([]mcpdomain.ToolDef(nil), f.listToolsResp...), f.listToolsErr
}
func (f *fakeMCPClient) CallTool(_ context.Context, _ string, _ json.RawMessage) (string, error) {
	return "", nil
}
func (f *fakeMCPClient) Close() error { f.mu.Lock(); defer f.mu.Unlock(); f.closed = true; return nil }
func (f *fakeMCPClient) StderrTail() string { return "" }

type mcpHandlerHarness struct {
	srv     *httptest.Server
	svc     *mcpapp.Service
	clients map[string]*fakeMCPClient
	mu      sync.Mutex
}

func newMCPTestServer(t *testing.T) *mcpHandlerHarness {
	t.Helper()
	log := zaptest.NewLogger(t)

	h := &mcpHandlerHarness{
		clients: map[string]*fakeMCPClient{},
	}
	source := newFakeRegistrySource(
		mcpdomain.RegistryEntry{
			Name:        "playwright",
			Description: "Browser automation reference entry for handler tests.",
			Runtime:     "node",
			InstallCmd:  mcpdomain.InstallCmd{Command: "npx", Args: []string{"-y", "@playwright/mcp"}},
			Category:    "browser",
			Tier:        0,
		},
		mcpdomain.RegistryEntry{
			Name:        "sqlite",
			Description: "SQLite reference entry for handler tests; has required dbPath.",
			Runtime:     "python",
			InstallCmd:  mcpdomain.InstallCmd{Command: "uvx", Args: []string{"mcp-server-sqlite", "--db-path", "${dbPath}"}},
			RequiredArgs: []mcpdomain.ArgRequirement{
				{Name: "dbPath", Description: "Path to the SQLite db file", Type: "path"},
			},
			Category: "database",
			Tier:     3,
		},
	)
	h.svc = mcpapp.New(
		filepath.Join(t.TempDir(), "mcp.json"),
		source,
		nil,
		nil, nil, nil, nil,
		log,
	)
	h.svc.SetClientFactory(func(cfg mcpdomain.ServerConfig, _ *zap.Logger) mcpinfra.Client {
		h.mu.Lock()
		defer h.mu.Unlock()
		fc, ok := h.clients[cfg.Name]
		if !ok {
			fc = &fakeMCPClient{}
			h.clients[cfg.Name] = fc
		}
		return fc
	})

	hd := NewMCPHandler(h.svc, log)
	mux := http.NewServeMux()
	hd.Register(mux)
	h.srv = httptest.NewServer(middlewarehttpapi.InjectUserID(mux))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *mcpHandlerHarness) registerClient(name string, c *fakeMCPClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[name] = c
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// envOf decodes {"data": ...} into a typed value for tests.
//
// envOf 解 {"data": ...} envelope 给测试用。
func envOf[T any](t *testing.T, body io.ReadCloser) T {
	t.Helper()
	defer body.Close()
	var env struct {
		Data T `json:"data"`
	}
	if err := json.NewDecoder(body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return env.Data
}


func TestMCP_ListServers_Empty(t *testing.T) {
	h := newMCPTestServer(t)
	resp, err := http.Get(h.srv.URL + "/api/v1/mcp-servers")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := envOf[[]mcpdomain.ServerStatus](t, resp.Body)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestMCP_ListServers_AfterAdd(t *testing.T) {
	h := newMCPTestServer(t)
	h.registerClient("alpha", &fakeMCPClient{
		listToolsResp: []mcpdomain.ToolDef{{ServerName: "alpha", Name: "ping"}},
	})
	if err := h.svc.AddServer(context.Background(), mcpdomain.ServerConfig{Name: "alpha", Command: "echo"}); err != nil {
		t.Fatalf("AddServer seed: %v", err)
	}
	resp, _ := http.Get(h.srv.URL + "/api/v1/mcp-servers")
	got := envOf[[]mcpdomain.ServerStatus](t, resp.Body)
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Errorf("got = %+v", got)
	}
}

func TestMCP_GetServer_NotFound(t *testing.T) {
	h := newMCPTestServer(t)
	resp, _ := http.Get(h.srv.URL + "/api/v1/mcp-servers/ghost")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}


func TestMCP_PutServer_Creates(t *testing.T) {
	h := newMCPTestServer(t)
	h.registerClient("github", &fakeMCPClient{})

	body := strings.NewReader(`{"command":"npx","args":["-y","@scope/pkg"],"env":{"K":"v"}}`)
	req, _ := http.NewRequest(http.MethodPut, h.srv.URL+"/api/v1/mcp-servers/github", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	st := envOf[mcpdomain.ServerStatus](t, resp.Body)
	if st.Name != "github" {
		t.Errorf("Name = %q", st.Name)
	}
}

func TestMCP_PutServer_RejectsEmptyCommand(t *testing.T) {
	h := newMCPTestServer(t)
	body := strings.NewReader(`{"command":""}`)
	req, _ := http.NewRequest(http.MethodPut, h.srv.URL+"/api/v1/mcp-servers/x", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestMCP_DeleteServer(t *testing.T) {
	h := newMCPTestServer(t)
	h.registerClient("toremove", &fakeMCPClient{})
	_ = h.svc.AddServer(context.Background(), mcpdomain.ServerConfig{Name: "toremove", Command: "x"})

	req, _ := http.NewRequest(http.MethodDelete, h.srv.URL+"/api/v1/mcp-servers/toremove", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}

func TestMCP_DeleteServer_NotFound(t *testing.T) {
	h := newMCPTestServer(t)
	req, _ := http.NewRequest(http.MethodDelete, h.srv.URL+"/api/v1/mcp-servers/ghost", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}


func TestMCP_Reconnect(t *testing.T) {
	h := newMCPTestServer(t)
	h.registerClient("a", &fakeMCPClient{})
	_ = h.svc.AddServer(context.Background(), mcpdomain.ServerConfig{Name: "a", Command: "x"})

	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/mcp-servers/a:reconnect", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestMCP_HealthCheck(t *testing.T) {
	h := newMCPTestServer(t)
	h.registerClient("a", &fakeMCPClient{
		listToolsResp: []mcpdomain.ToolDef{{Name: "x"}, {Name: "y"}},
	})
	_ = h.svc.AddServer(context.Background(), mcpdomain.ServerConfig{Name: "a", Command: "x"})

	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/mcp-servers/a:health-check", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := envOf[mcpdomain.HealthResult](t, resp.Body)
	if !got.Healthy {
		t.Errorf("Healthy = false; %+v", got)
	}
	if got.ToolCount != 2 {
		t.Errorf("ToolCount = %d, want 2", got.ToolCount)
	}
}

func TestMCP_NameAction_UnknownAction(t *testing.T) {
	h := newMCPTestServer(t)
	h.registerClient("a", &fakeMCPClient{})
	_ = h.svc.AddServer(context.Background(), mcpdomain.ServerConfig{Name: "a", Command: "x"})

	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/mcp-servers/a:nonsense", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}


func TestMCP_Import_JSONBody(t *testing.T) {
	h := newMCPTestServer(t)
	body := strings.NewReader(`{"mcpServers":{"github":{"command":"npx","args":["-y","@scope/gh"]}}}`)
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/mcp-servers:import", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	res := envOf[mcpinfra.MergeResult](t, resp.Body)
	if len(res.Imported) != 1 || res.Imported[0] != "github" {
		t.Errorf("Imported = %v, want [github]", res.Imported)
	}
}

func TestMCP_Import_ConflictNoOverwrite(t *testing.T) {
	h := newMCPTestServer(t)
	h.registerClient("github", &fakeMCPClient{})
	_ = h.svc.AddServer(context.Background(), mcpdomain.ServerConfig{Name: "github", Command: "old"})

	body := strings.NewReader(`{"mcpServers":{"github":{"command":"new"},"slack":{"command":"s"}}}`)
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/mcp-servers:import", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	res := envOf[mcpinfra.MergeResult](t, resp.Body)
	if !contains(res.Conflicts, "github") {
		t.Errorf("Conflicts missing github: %v", res.Conflicts)
	}
	if !contains(res.Imported, "slack") {
		t.Errorf("Imported missing slack: %v", res.Imported)
	}
}

func TestMCP_Import_OverwriteForce(t *testing.T) {
	h := newMCPTestServer(t)
	h.registerClient("github", &fakeMCPClient{})
	_ = h.svc.AddServer(context.Background(), mcpdomain.ServerConfig{Name: "github", Command: "old"})

	body := strings.NewReader(`{"mcpServers":{"github":{"command":"new"}}}`)
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/mcp-servers:import?overwrite=true", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	res := envOf[mcpinfra.MergeResult](t, resp.Body)
	if !contains(res.Imported, "github") {
		t.Errorf("Imported should include github with overwrite=true: %v", res.Imported)
	}
}

func TestMCP_Import_EmptyServersRejected(t *testing.T) {
	h := newMCPTestServer(t)
	body := strings.NewReader(`{"mcpServers":{}}`)
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/mcp-servers:import", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestMCP_Import_Multipart(t *testing.T) {
	h := newMCPTestServer(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("config", "mcp.json")
	_, _ = fw.Write([]byte(`{"mcpServers":{"alpha":{"command":"echo"}}}`))
	_ = mw.Close()

	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/mcp-servers:import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	res := envOf[mcpinfra.MergeResult](t, resp.Body)
	if len(res.Imported) != 1 || res.Imported[0] != "alpha" {
		t.Errorf("Imported = %v, want [alpha]", res.Imported)
	}
}


func TestMCP_ListRegistry_ReturnsAllEntries(t *testing.T) {
	h := newMCPTestServer(t)
	resp, _ := http.Get(h.srv.URL + "/api/v1/mcp-registry")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := envOf[[]mcpdomain.RegistryEntry](t, resp.Body)
	if len(got) != 2 {
		t.Errorf("got %d entries, want 2 (playwright + sqlite)", len(got))
	}
}

func TestMCP_GetRegistryEntry_Found(t *testing.T) {
	h := newMCPTestServer(t)
	resp, _ := http.Get(h.srv.URL + "/api/v1/mcp-registry/playwright")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := envOf[mcpdomain.RegistryEntry](t, resp.Body)
	if got.Name != "playwright" {
		t.Errorf("Name = %q", got.Name)
	}
}

func TestMCP_GetRegistryEntry_NotFound(t *testing.T) {
	h := newMCPTestServer(t)
	resp, _ := http.Get(h.srv.URL + "/api/v1/mcp-registry/no-such")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestMCP_Install_MissingRequiredArg(t *testing.T) {
	h := newMCPTestServer(t)
	body := strings.NewReader(`{}`)
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/mcp-registry/sqlite:install", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
}

func TestMCP_Install_UnknownEntry(t *testing.T) {
	h := newMCPTestServer(t)
	body := strings.NewReader(`{}`)
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/mcp-registry/no-such:install", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestMCP_Install_UnknownAction(t *testing.T) {
	h := newMCPTestServer(t)
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/api/v1/mcp-registry/playwright:nonsense", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}


type fakeRegistrySource struct {
	byName map[string]mcpdomain.RegistryEntry
	all    []mcpdomain.RegistryEntry
}

func newFakeRegistrySource(entries ...mcpdomain.RegistryEntry) *fakeRegistrySource {
	f := &fakeRegistrySource{byName: map[string]mcpdomain.RegistryEntry{}}
	for _, e := range entries {
		f.byName[e.Name] = e
		f.all = append(f.all, e)
	}
	return f
}

func (f *fakeRegistrySource) List(_ context.Context) ([]mcpdomain.RegistryEntry, error) {
	out := make([]mcpdomain.RegistryEntry, len(f.all))
	copy(out, f.all)
	return out, nil
}

func (f *fakeRegistrySource) Get(_ context.Context, name string) (*mcpdomain.RegistryEntry, error) {
	e, ok := f.byName[name]
	if !ok {
		return nil, mcpdomain.ErrRegistryEntryNotFound
	}
	cp := e
	return &cp, nil
}
