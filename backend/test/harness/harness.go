//go:build pipeline

// Package harness is the whole-stack pipeline test harness. It boots the same
// DI graph as cmd/server/main.go (real Bridge, real LLM client, real Python
// sandbox) but with in-memory SQLite and an httptest server, so tests can drive
// the system through HTTP and observe SSE without ceremony.
//
// By default, tests use FakeLLMServer (no external network). Pass
// WithFakeLLMBaseURL to route the injected apikey's BaseURL to the fake server.
// Tests that need a real provider use RequireDeepSeekKey and the "Live_" naming
// prefix.
//
// Package harness 是 pipeline 测试脚手架。DI 图与 cmd/server/main.go 一致
// （真 Bridge / 真 LLM 客户端 / 真 Python sandbox），区别在于内存 SQLite +
// httptest server。默认走 FakeLLMServer（无外网）；需真实 provider 的测试
// 用 RequireDeepSeekKey + "Live_" 前缀命名。
package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/gorm"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	askapp "github.com/sunweilin/forgify/backend/internal/app/ask"
	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	forgeapp "github.com/sunweilin/forgify/backend/internal/app/forge"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	asktool "github.com/sunweilin/forgify/backend/internal/app/tool/ask"
	fstool "github.com/sunweilin/forgify/backend/internal/app/tool/filesystem"
	forgetool "github.com/sunweilin/forgify/backend/internal/app/tool/forge"
	searchtool "github.com/sunweilin/forgify/backend/internal/app/tool/search"
	shelltool "github.com/sunweilin/forgify/backend/internal/app/tool/shell"
	subagenttool "github.com/sunweilin/forgify/backend/internal/app/tool/subagent"
	todotool "github.com/sunweilin/forgify/backend/internal/app/tool/todo"
	webtool "github.com/sunweilin/forgify/backend/internal/app/tool/web"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
	cryptoinfra "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	memoryinfra "github.com/sunweilin/forgify/backend/internal/infra/events/memory"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	sandboxinfra "github.com/sunweilin/forgify/backend/internal/infra/sandbox"
	apikeystore "github.com/sunweilin/forgify/backend/internal/infra/store/apikey"
	chatstore "github.com/sunweilin/forgify/backend/internal/infra/store/chat"
	convstore "github.com/sunweilin/forgify/backend/internal/infra/store/conversation"
	forgestore "github.com/sunweilin/forgify/backend/internal/infra/store/forge"
	modelstore "github.com/sunweilin/forgify/backend/internal/infra/store/model"
	sandboxstore "github.com/sunweilin/forgify/backend/internal/infra/store/sandbox"
	subagentstore "github.com/sunweilin/forgify/backend/internal/infra/store/subagent"
	todostore "github.com/sunweilin/forgify/backend/internal/infra/store/todo"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
	routerhttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/router"
)

// Option configures the Harness at construction time.
//
// Option 在构造时配置 Harness。
type Option func(*options)

type options struct {
	fakeLLMBaseURL string
}

// WithFakeLLMBaseURL routes the injected DeepSeek apikey's BaseURL to the
// given fake server URL instead of the real provider. Use together with
// NewFakeLLMServer to test chat flows without network calls.
//
// WithFakeLLMBaseURL 把注入的 DeepSeek apikey 的 BaseURL 指向 fake server
// 而非真实 provider。配合 NewFakeLLMServer 做无网络 chat 测试。
func WithFakeLLMBaseURL(url string) Option {
	return func(o *options) { o.fakeLLMBaseURL = url }
}

// Harness is a booted in-process backend ready to drive over HTTP.
//
// Harness 是 boot 完成、可通过 HTTP 驱动的 in-process 后端。
type Harness struct {
	t      *testing.T
	server *httptest.Server
	log    *zap.Logger

	// fakeLLMBaseURL is stored so SeedDeepSeek can inject it into the apikey.
	// fakeLLMBaseURL 存这里让 SeedDeepSeek 注入到 apikey 里。
	fakeLLMBaseURL string

	DB      *gorm.DB
	Bridge  eventsdomain.Bridge
	Sandbox *sandboxapp.Service

	APIKey       *apikeyapp.Service
	Model        *modelapp.Service
	Conversation *convapp.Service
	Forge        *forgeapp.Service
	Chat         *chatapp.Service
	Tools        []toolapp.Tool
}

// New boots a fresh harness backed by an in-memory SQLite. The httptest server
// is started, the same migrations as production are applied, and the DI graph
// matches main.go. Cleanup (server stop + DB close) is registered on t.
//
// Pass WithFakeLLMBaseURL to redirect LLM calls to a FakeLLMServer so tests
// run without external network access. Without it the harness is wired for
// real provider calls (requires SeedDeepSeek with a live key).
//
// New 启动内存 SQLite + httptest server 的全新 harness，迁移 + DI 图与
// main.go 对齐，清理注册到 t。传 WithFakeLLMBaseURL 把 LLM 调用路由到
// FakeLLMServer，无需外网；不传则需真实 apikey。
func New(t *testing.T, opts ...Option) *Harness {
	t.Helper()

	cfg := &options{}
	for _, o := range opts {
		o(cfg)
	}

	log := zaptest.NewLogger(t)

	gdb, err := dbinfra.Open(dbinfra.Config{DataDir: ""}) // in-memory
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if err := dbinfra.Close(gdb); err != nil {
			t.Logf("close db: %v", err)
		}
	})

	// SQLite :memory: gives each connection its own independent DB instance.
	// Force a single connection so all goroutines share the migrated tables.
	//
	// SQLite :memory: 每个连接独立实例；强制单连接让所有 goroutine 共享迁移后的表。
	if sqlDB, err := gdb.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}

	if err := dbinfra.Migrate(gdb,
		&apikeydomain.APIKey{},
		&modeldomain.ModelConfig{},
		&convdomain.Conversation{},
		&chatdomain.Message{},
		&chatdomain.Block{},
		&chatdomain.Attachment{},
		&forgedomain.Forge{},
		&forgedomain.ForgeVersion{},
		&forgedomain.ForgeTestCase{},
		&forgedomain.ForgeExecution{},
		&sandboxdomain.Runtime{},
		&sandboxdomain.Env{},
		&subagentdomain.SubagentRun{},
		&subagentdomain.SubagentMessage{},
		&tododomain.Todo{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Crypto: the encryptor uses a deterministic test fingerprint so each
	// harness instance can decrypt keys it inserted itself. Production
	// derives from MachineFingerprint (os/hardware identity).
	//
	// crypto：用确定性测试指纹，让 harness 实例能解开自己插入的 key。
	// 生产从 MachineFingerprint（OS/硬件身份）派生。
	encryptor, err := cryptoinfra.NewAESGCMEncryptor(
		cryptoinfra.DeriveKey("forgify-pipeline-test-fingerprint"),
	)
	if err != nil {
		t.Fatalf("build encryptor: %v", err)
	}

	apikeyService := apikeyapp.NewService(
		apikeystore.New(gdb),
		encryptor,
		apikeyapp.NewHTTPTester(nil),
		log,
	)
	modelService := modelapp.NewService(modelstore.New(gdb), log)
	convService := convapp.NewService(convstore.New(gdb), log)

	llmFactory := llminfra.NewFactory()

	// PluginSandbox v2 — same DI as cmd/server/main.go but rooted at a per-test
	// tempdir. Bootstrap extracts the embedded mise binary; if unavailable for
	// the host platform (e.g. CI without `make resources`), bootstrap fails →
	// degraded mode → runtime ops return ErrSandboxUnavailable. Tests that
	// need a working sandbox should gate themselves with sandboxapp.Service
	// .IsReady().
	//
	// PluginSandbox v2 ——与 main.go 同 DI 图但根目录是 per-test tempdir。
	// Bootstrap 解 embed mise；当前平台没有就 degraded mode；需要 sandbox 的
	// 测试自己用 IsReady() 守卫。
	dataDir := t.TempDir()
	sandboxRepo := sandboxstore.New(gdb)
	sandboxSvc := sandboxapp.New(sandboxRepo, dataDir, log)
	if err := sandboxSvc.Bootstrap(context.Background()); err != nil {
		t.Logf("sandbox v2 bootstrap failed: %v (degraded mode active; runtime ops will fail)", err)
	}
	registerSandboxStack(sandboxSvc)
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := sandboxSvc.Shutdown(shutdownCtx); err != nil {
			t.Logf("sandbox shutdown: %v", err)
		}
	})

	forgeLLM := &forgeLLMAdapter{picker: modelService, keys: apikeyService, factory: llmFactory}
	bridge := memoryinfra.NewBridge(log)
	forgeService := forgeapp.NewService(
		forgestore.New(gdb),
		forgeapp.NewSandboxAdapter(sandboxSvc, dataDir),
		forgeLLM,
		bridge,
		log,
	)

	chatRepo := chatstore.New(gdb)
	chatService := chatapp.NewService(
		chatRepo,
		convstore.New(gdb),
		modelService,
		apikeyService,
		llmFactory,
		bridge,
		"", // dataDir empty: tests don't write attachment files
		log,
	)

	// PathGuard for filesystem tools — NewDefault deny-list is fine for tests
	// (tests use t.TempDir paths which aren't on the deny list).
	// 文件系统 tool 的 PathGuard——NewDefault 黑名单足够测试用（t.TempDir 路径
	// 不在黑名单里）。
	pathGuard := pathguardpkg.NewDefault()

	tools := forgetool.ForgeTools(
		forgeService, chatRepo, modelService, apikeyService, llmFactory,
	)
	tools = append(tools, fstool.FilesystemTools(pathGuard)...)
	tools = append(tools, searchtool.SearchTools(pathGuard)...)
	tools = append(tools, webtool.WebTools(modelService, apikeyService, llmFactory)...)
	shells := shelltool.NewShellTools(sandboxSvc)
	t.Cleanup(shells.Manager.Stop)
	tools = append(tools, shells.Tools...)
	todoService := todoapp.NewService(todostore.New(gdb), bridge, log)
	tools = append(tools, todotool.TodoTools(todoService)...)
	askService := askapp.NewService()
	tools = append(tools, asktool.AskTools(askService)...)

	subagentService := subagentapp.New(
		subagentstore.New(gdb),
		subagentapp.NewRegistry(),
		bridge,
		modelService,
		apikeyService,
		llmFactory,
		log,
	)
	tools = append(tools, subagenttool.SubagentTools(subagentService)...)
	subagentService.SetTools(tools)
	chatService.SetTools(tools)

	handler := routerhttpapi.New(routerhttpapi.Deps{
		Log:                 log,
		APIKeyService:       apikeyService,
		ModelService:        modelService,
		ConversationService: convService,
		ForgeService:        forgeService,
		ChatService:         chatService,
		EventsBridge:        bridge,
		AskService:          askService,
		SandboxService:      sandboxSvc,
		SubagentService:     subagentService,
		Dev:                 false,
		Tools:               tools,
		DB:                  gdb,
		Port:                0,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &Harness{
		t:              t,
		server:         srv,
		log:            log,
		fakeLLMBaseURL: cfg.fakeLLMBaseURL,
		DB:             gdb,
		Bridge:         bridge,
		Sandbox:        sandboxSvc,
		APIKey:         apikeyService,
		Model:          modelService,
		Conversation:   convService,
		Forge:          forgeService,
		Chat:           chatService,
		Tools:          tools,
	}
}

// registerSandboxStack mirrors cmd/server/main.go::registerSandboxStack so
// the harness wires the same v1 PluginSandbox runtime/env matrix. Mise-
// independent installers (dotnet/static) are registered up front; mise-
// managed ones are skipped if Bootstrap failed (degraded mode).
//
// registerSandboxStack 镜像 main.go 的同名 helper，让 harness 注册同一份 v1
// PluginSandbox runtime/env 矩阵。Mise 无关的 installer（dotnet/static）先
// 注册；mise 管理的 installer 在 Bootstrap 失败（degraded mode）时跳过。
func registerSandboxStack(svc *sandboxapp.Service) {
	miseBin := svc.MiseBin()
	if miseBin == "" {
		return
	}
	for kind, defaultVer := range map[string]string{
		"python": "3.12",
		"node":   "22",
		"rust":   "stable",
		"java":   "21",
		"go":     "1.22",
		"ruby":   "3.3",
		"php":    "8.3",
		// Mirrors main.go pin set. bundler + composer NOT registered (not
		// in mise registry — see main.go comment for the details).
		// 镜像 main.go 的 pin。bundler + composer 不注册（不在 mise registry
		// ——详见 main.go 同段注释）。
		"uv":    "0.11.4",
		"pnpm":  "9.15.4",
		"maven": "3.9.9",
	} {
		svc.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, kind, defaultVer))
	}
	svc.RegisterInstaller(sandboxinfra.NewDotnetInstaller("8.0"))

	svc.RegisterEnvManager(sandboxinfra.NewPythonEnvManager(svc))
	svc.RegisterEnvManager(sandboxinfra.NewNodeEnvManager(svc))
	svc.RegisterEnvManager(sandboxinfra.NewRustEnvManager())
	svc.RegisterEnvManager(sandboxinfra.NewGoEnvManager())
	svc.RegisterEnvManager(sandboxinfra.NewJavaEnvManager(svc))
	svc.RegisterEnvManager(sandboxinfra.NewRubyEnvManager(svc))
	svc.RegisterEnvManager(sandboxinfra.NewPHPEnvManager(svc))
	svc.RegisterEnvManager(sandboxinfra.NewDotnetEnvManager())
	svc.RegisterEnvManager(sandboxinfra.NewPlaywrightEnvManager(
		sandboxinfra.NewNodeEnvManager(svc), svc.SandboxRoot()))
}

// URL returns the test server's base URL (e.g. "http://127.0.0.1:54321").
//
// URL 返回 test server 的 base URL。
func (h *Harness) URL() string { return h.server.URL }

// HTTPClient returns a client suitable for short-lived requests against the
// harness. SSE long-lived streams should construct their own request via
// SubscribeSSE.
//
// HTTPClient 返回适合短请求的 client；SSE 长连接走 SubscribeSSE。
func (h *Harness) HTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// PostJSON POSTs body as JSON to path (relative to URL()) and decodes the
// response into out. Fails the test on non-2xx or transport errors.
//
// PostJSON 把 body 编为 JSON POST 到 path（相对 URL()），结果解到 out。
// 非 2xx / 传输错误直接 fail。
func (h *Harness) PostJSON(path string, body, out any) *http.Response {
	h.t.Helper()
	return h.requestJSON("POST", path, body, out)
}

// GetJSON GETs path and decodes into out.
// GetJSON 取 path 并解到 out。
func (h *Harness) GetJSON(path string, out any) *http.Response {
	h.t.Helper()
	return h.requestJSON("GET", path, nil, out)
}

// PatchJSON PATCH's body to path and decodes into out.
// PatchJSON 把 body PATCH 到 path 并解到 out。
func (h *Harness) PatchJSON(path string, body, out any) *http.Response {
	h.t.Helper()
	return h.requestJSON("PATCH", path, body, out)
}

// Delete DELETEs path. Fails the test on non-2xx or transport errors.
// Delete DELETE 请求；非 2xx / 传输错误 fail。
func (h *Harness) Delete(path string) *http.Response {
	h.t.Helper()
	return h.requestJSON("DELETE", path, nil, nil)
}

func (h *Harness) requestJSON(method, path string, body, out any) *http.Response {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal %s %s body: %v", method, path, err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, h.server.URL+path, rdr)
	if err != nil {
		h.t.Fatalf("build %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.HTTPClient().Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		h.t.Fatalf("%s %s: status %d: %s", method, path, resp.StatusCode, raw)
	}
	if out != nil {
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			h.t.Fatalf("%s %s: decode response: %v", method, path, err)
		}
	} else {
		_ = resp.Body.Close()
	}
	return resp
}

// RequireDeepSeekKey returns the DeepSeek API key from env or skips the test.
// All chat-touching pipeline tests should call this at the top so missing keys
// fail clean rather than mid-run.
//
// RequireDeepSeekKey 返回环境里的 DEEPSEEK_API_KEY，缺则 skip。
// 所有触 chat 的 pipeline 测试在开头调它，让缺 key 时直接 skip 而非中途失败。
func RequireDeepSeekKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("DEEPSEEK_API_KEY")
	if key == "" {
		t.Skip("DEEPSEEK_API_KEY not set; skipping (run via `make test-pipeline` to load .env)")
	}
	return key
}

// ── forgeLLMAdapter (mirrors main.go) ─────────────────────────────────────────

type forgeLLMAdapter struct {
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
}

func (c *forgeLLMAdapter) Generate(ctx context.Context, prompt string) (string, error) {
	bc, err := llmclientpkg.Resolve(ctx, c.picker, c.keys, c.factory)
	if err != nil {
		return "", fmt.Errorf("forgeLLMAdapter: %w", err)
	}
	return llminfra.Generate(ctx, bc.Client, llminfra.Request{
		ModelID:  bc.ModelID,
		Key:      bc.Key,
		BaseURL:  bc.BaseURL,
		Messages: []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: prompt}},
	})
}
