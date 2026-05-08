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
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/gorm"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	askapp "github.com/sunweilin/forgify/backend/internal/app/ask"
	catalogapp "github.com/sunweilin/forgify/backend/internal/app/catalog"
	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	forgeapp "github.com/sunweilin/forgify/backend/internal/app/forge"
	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	asktool "github.com/sunweilin/forgify/backend/internal/app/tool/ask"
	fstool "github.com/sunweilin/forgify/backend/internal/app/tool/filesystem"
	forgetool "github.com/sunweilin/forgify/backend/internal/app/tool/forge"
	mcptool "github.com/sunweilin/forgify/backend/internal/app/tool/mcp"
	searchtool "github.com/sunweilin/forgify/backend/internal/app/tool/search"
	shelltool "github.com/sunweilin/forgify/backend/internal/app/tool/shell"
	skilltool "github.com/sunweilin/forgify/backend/internal/app/tool/skill"
	subagenttool "github.com/sunweilin/forgify/backend/internal/app/tool/subagent"
	todotool "github.com/sunweilin/forgify/backend/internal/app/tool/todo"
	webtool "github.com/sunweilin/forgify/backend/internal/app/tool/web"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
	cryptoinfra "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	eventloginfra "github.com/sunweilin/forgify/backend/internal/infra/eventlog"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	notificationsinfra "github.com/sunweilin/forgify/backend/internal/infra/notifications"
	sandboxinfra "github.com/sunweilin/forgify/backend/internal/infra/sandbox"
	apikeystore "github.com/sunweilin/forgify/backend/internal/infra/store/apikey"
	chatstore "github.com/sunweilin/forgify/backend/internal/infra/store/chat"
	convstore "github.com/sunweilin/forgify/backend/internal/infra/store/conversation"
	forgestore "github.com/sunweilin/forgify/backend/internal/infra/store/forge"
	modelstore "github.com/sunweilin/forgify/backend/internal/infra/store/model"
	sandboxstore "github.com/sunweilin/forgify/backend/internal/infra/store/sandbox"
	todostore "github.com/sunweilin/forgify/backend/internal/infra/store/todo"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
	routerhttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/router"
)

// Option configures the Harness at construction time.
//
// Option 在构造时配置 Harness。
type Option func(*options)

type options struct {
	fakeLLMBaseURL  string
	curatedRegistry bool
	sandboxDataDir  string
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

// WithCuratedRegistry swaps the in-memory test registry (everything +
// sqlite-test) for the production CuratedRegistrySource so curated
// marketplace pipeline tests can install / handshake any of the 21
// real entries by their slug.
//
// WithCuratedRegistry 把内存测试 registry 换成生产 CuratedRegistrySource，
// 让 curated 21 条 pipeline 测试能按 slug 走真装 / 真握手。
func WithCuratedRegistry() Option {
	return func(o *options) { o.curatedRegistry = true }
}

// WithSandboxDataDir overrides the default per-test t.TempDir() with a
// caller-provided directory. Lets pipeline runs share mise + npm + uv
// caches across multiple harness instances (production reality: one
// persistent ~/.forgify/sandbox/ warmed once); without it every test
// re-extracts mise (~65MB) and re-downloads node@22 (~50MB) — the
// 5-minute first-install cost dominates wall time.
//
// WithSandboxDataDir 用调用方提供的目录覆盖 per-test t.TempDir()。让
// pipeline 多个 harness 实例共享 mise + npm + uv 缓存（生产现实：单一
// ~/.forgify/sandbox/ 只 warmup 一次）。否则每个 test 重解 mise + 重下
// node@22，5min 冷启动主导墙钟。
func WithSandboxDataDir(dir string) Option {
	return func(o *options) { o.sandboxDataDir = dir }
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

	DB                  *gorm.DB
	EventLogBridge      *eventloginfra.Bridge
	NotificationsBridge *notificationsinfra.Bridge
	NotificationsPub    notificationspkg.Publisher
	ChatEmitter         eventlogpkg.Emitter
	Sandbox             *sandboxapp.Service
	MCP                 *mcpapp.Service
	Skill               *skillapp.Service
	Catalog             *catalogapp.Service

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
	var dataDir string
	if cfg.sandboxDataDir != "" {
		dataDir = cfg.sandboxDataDir
	} else {
		dataDir = t.TempDir()
	}
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
	eventLogBridge := eventloginfra.NewBridge(log)
	notificationsBridge := notificationsinfra.NewBridge(log)
	notificationsPub := notificationspkg.New(notificationsBridge, log)
	convService := convapp.NewService(convstore.New(gdb), notificationsPub, log)

	forgeService := forgeapp.NewService(
		forgestore.New(gdb),
		forgeapp.NewSandboxAdapter(sandboxSvc, dataDir),
		forgeLLM,
		log,
	)

	chatRepo := chatstore.New(gdb)
	chatEmitter := eventlogpkg.New(eventLogBridge, chatRepo, log)
	chatService := chatapp.NewService(
		chatRepo,
		convstore.New(gdb),
		modelService,
		apikeyService,
		llmFactory,
		chatEmitter,
		notificationsPub,
		dataDir,
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
	// WebTools wired without MCP router in pipeline harness — tests that
	// need MCP routing should construct WebSearch directly with a fake
	// MCPSearchRouter. nil router = no MCP tier (BYOK + Bing CN only).
	//
	// 测试 harness 不接 MCP router——需要 MCP 路由的测试自己构造 WebSearch
	// + fake router。nil 路由器 = 无 MCP 层（仅 BYOK + Bing CN）。
	tools = append(tools, webtool.WebTools(modelService, apikeyService, llmFactory, nil, log)...)
	shells := shelltool.NewShellTools(sandboxSvc)
	t.Cleanup(shells.Manager.Stop)
	tools = append(tools, shells.Tools...)
	todoService := todoapp.NewService(todostore.New(gdb), notificationsPub, log)
	tools = append(tools, todotool.TodoTools(todoService)...)
	askService := askapp.NewService()
	tools = append(tools, asktool.AskTools(askService)...)

	subagentService := subagentapp.New(
		chatRepo,
		subagentapp.NewRegistry(),
		modelService,
		apikeyService,
		llmFactory,
		log,
	)
	tools = append(tools, subagenttool.SubagentTools(subagentService)...)
	subagentService.SetTools(tools)

	// MCP: configPath inside the per-test tempdir so we never touch
	// real ~/.forgify/mcp.json. Service.Start with no config = no-op
	// (instant boot); pipeline tests that need MCP servers seed
	// mcp.json + register fakeClient via the test seam separately.
	//
	// MCP：configPath 在 per-test tempdir 内，永不动真 ~/.forgify/mcp.json。
	// 无配置时 Service.Start 是 no-op（瞬时启动）；需要 MCP server 的
	// pipeline 测试通过 test seam 单独灌 mcp.json + 注册 fakeClient。
	mcpConfigPath := filepath.Join(dataDir, "mcp.json")
	// In-memory test RegistrySource — production uses CuratedRegistrySource
	// (21 hand-picked servers); tests need predictable controllable entries
	// for install paths (the `everything` ref server + a forced-arg sample).
	//
	// 内存测试 RegistrySource——生产用 CuratedRegistrySource（21 条精选）；
	// 测试要可控固定 entry 跑 install 路径（`everything` 参考 server + 强制
	// 必填 arg 的样本）。
	var mcpRegistrySource mcpdomain.RegistrySource
	if cfg.curatedRegistry {
		mcpRegistrySource = mcpinfra.NewCuratedRegistrySource()
	} else {
		mcpRegistrySource = newTestRegistrySource(
			mcpdomain.RegistryEntry{
				Name:        "everything",
				Description: "MCP protocol reference test server (used by D9 pipeline tests).",
				Runtime:     "node",
				InstallCmd: mcpdomain.InstallCmd{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-everything"},
				},
				Category: "browser",
				Tier:     0,
			},
			mcpdomain.RegistryEntry{
				Name:        "sqlite-test",
				Description: "Sample server with a required arg, for install error-path tests.",
				Runtime:     "python",
				InstallCmd: mcpdomain.InstallCmd{
					Command: "uvx",
					Args:    []string{"mcp-server-sqlite", "--db-path", "${dbPath}"},
				},
				RequiredArgs: []mcpdomain.ArgRequirement{
					{Name: "dbPath", Description: "Path to the SQLite db file", Type: "path"},
				},
				Category: "database",
				Tier:     3,
			},
		)
	}
	mcpService := mcpapp.New(
		mcpConfigPath,
		mcpRegistrySource,
		sandboxSvc,
		modelService,
		apikeyService,
		llmFactory,
		notificationsPub,
		log,
	)
	if err := mcpService.Start(context.Background()); err != nil {
		t.Logf("mcp start: %v (continuing — pipeline tests that need it will skip)", err)
	}
	// Pass nils for LLM deps — harness tests don't exercise marketplace LLM rerank.
	// 传 nil LLM 依赖——harness 测试不跑 marketplace LLM 重排。
	tools = append(tools, mcptool.MCPTools(mcpService, nil, nil, nil)...)

	// Skill: per-test tempdir SkillsDir so we never touch real
	// ~/.forgify/skills/. Tests that need skills installed seed them
	// into h.Skill.SkillsDir() then either call h.Skill.Scan(ctx) for
	// immediate effect or rely on the 1s polling loop (D9 dynamic-update
	// pattern).
	//
	// Skill：per-test tempdir SkillsDir，永不动真 ~/.forgify/skills/。
	// 需 skill 的测试自己往 h.Skill.SkillsDir() 写 + 调 h.Skill.Scan
	// 立即生效，或靠 1s 轮询（D9 动态更新模式）。
	skillsDir := filepath.Join(dataDir, "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	skillService := skillapp.New(
		skillsDir,
		subagentService,
		modelService,
		apikeyService,
		llmFactory,
		notificationsPub,
		log,
	)
	if err := skillService.Start(context.Background()); err != nil {
		t.Logf("skill start: %v", err)
	}
	t.Cleanup(skillService.Stop)
	tools = append(tools, skilltool.SkillTools(skillService)...)

	// Capability Catalog: per-test tempdir for the cache file so we
	// never touch real ~/.forgify/.catalog.json. Tests that exercise
	// catalog drive it explicitly via h.Catalog.Refresh; the polling
	// loop is started so SSE-style timing tests are realistic.
	//
	// Capability Catalog：per-test tempdir cache 文件，永不动真
	// ~/.forgify/.catalog.json。需 catalog 的测试经 h.Catalog.Refresh
	// 显式驱动；polling loop 启 让 SSE 类时序测试逼真。
	catalogCachePath := filepath.Join(dataDir, ".catalog.json")
	catalogService := catalogapp.New(catalogCachePath, notificationsPub, log)
	// Deliberately NOT calling SetGenerator in the test harness:
	// production main.go wires LLMGenerator, but the FakeLLMServer's
	// FIFO script queue is a test-only abstraction (real LLM endpoints
	// handle concurrent requests fine). Background catalog regen
	// competing for queued scripts caused test/chat to regress after
	// D8. Solution: catalog uses mechanical-fallback only in pipelines
	// — content is still populated, Coverage map still complete, just
	// no LLM-generated 'Notes on choosing' prose. D9 + test/catalog
	// scenarios all assert against mechanical-fallback markers and
	// pass either way.
	//
	// 故意不在 harness 里 SetGenerator：生产 main.go 接 LLMGenerator，
	// 但 FakeLLMServer 的 FIFO 脚本队列是测试基础设施才有的限制（真
	// LLM endpoint 处理并发请求没问题）。后台 catalog regen 抢队列脚本
	// D8 后让 test/chat 回归。方案：pipeline 里 catalog 走 mechanical-
	// fallback——内容仍 populate、Coverage 仍全，只少 LLM 生成的"Notes
	// on choosing" prose。D9 + test/catalog 场景全针对 mechanical 标记
	// 断言，都通过。
	catalogService.RegisterSource(forgeService.AsCatalogSource())
	catalogService.RegisterSource(skillService.AsCatalogSource())
	catalogService.RegisterSource(mcpService.AsCatalogSource())
	if err := catalogService.Start(context.Background()); err != nil {
		t.Logf("catalog start: %v", err)
	}
	// catalogService.Stop blocks until the polling goroutine fully
	// drains — without this, a tick mid-saveToDisk could race with
	// t.TempDir's RemoveAll and fail tests with "directory not empty".
	//
	// catalogService.Stop 阻塞到 polling goroutine 完全 drain——没有
	// 它的话，mid-saveToDisk 的 tick 与 t.TempDir 的 RemoveAll 竞态会
	// 让测试以 "directory not empty" 失败。
	t.Cleanup(catalogService.Stop)
	chatService.SetSystemPromptProvider(catalogService)

	chatService.SetTools(tools)

	handler := routerhttpapi.New(routerhttpapi.Deps{
		Log:                 log,
		APIKeyService:       apikeyService,
		ModelService:        modelService,
		ConversationService: convService,
		ForgeService:        forgeService,
		ChatService:         chatService,
		EventLogBridge:      eventLogBridge,
		BlockV2Repo:         chatRepo,
		NotificationsBridge: notificationsBridge,
		AskService:          askService,
		SandboxService:      sandboxSvc,
		SubagentService:     subagentService,
		MCPService:          mcpService,
		SkillService:        skillService,
		CatalogService:      catalogService,
		Dev:                 false,
		Tools:               tools,
		LLMFactory:          llmFactory,
		ShellManager:        shells.Manager,
		DB:                  gdb,
		Port:                0,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &Harness{
		t:                   t,
		server:              srv,
		log:                 log,
		fakeLLMBaseURL:      cfg.fakeLLMBaseURL,
		DB:                  gdb,
		EventLogBridge:      eventLogBridge,
		NotificationsBridge: notificationsBridge,
		NotificationsPub:    notificationsPub,
		ChatEmitter:         chatEmitter,
		Sandbox:             sandboxSvc,
		MCP:                 mcpService,
		Skill:               skillService,
		Catalog:             catalogService,
		APIKey:              apikeyService,
		Model:               modelService,
		Conversation:        convService,
		Forge:               forgeService,
		Chat:                chatService,
		Tools:               tools,
	}
}

// registerSandboxStack mirrors cmd/server/main.go::registerSandboxStack so
// the harness wires the same v1 PluginSandbox runtime/env matrix. Mise-
// independent installers (dotnet/static) are registered up front; mise-
// managed ones are skipped if Bootstrap failed (degraded mode).
//
// registerSandboxStack 镜像 main.go 的同名 helper——curated marketplace 仅
// npm + pypi，故仅注册 python + node + uv runtime / python + node EnvManager。
func registerSandboxStack(svc *sandboxapp.Service) {
	miseBin := svc.MiseBin()
	if miseBin == "" {
		return
	}
	for kind, defaultVer := range map[string]string{
		"python": "3.12",
		"node":   "22",
		"uv":     "0.11.4",
	} {
		svc.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, kind, defaultVer))
	}
	svc.RegisterEnvManager(sandboxinfra.NewPythonEnvManager(svc))
	svc.RegisterEnvManager(sandboxinfra.NewNodeEnvManager())
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
