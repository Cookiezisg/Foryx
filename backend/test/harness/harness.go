//go:build pipeline

// Package harness boots the production DI graph against in-memory SQLite + httptest for pipeline tests.
//
// Package harness 用内存 SQLite + httptest 启生产 DI 图给 pipeline 测试。
package harness

import (
	"context"
	"fmt"
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
	contextmgrapp "github.com/sunweilin/forgify/backend/internal/app/contextmgr"
	hooksapp "github.com/sunweilin/forgify/backend/internal/app/hooks"
	permgateapp "github.com/sunweilin/forgify/backend/internal/app/tool/permissionsgate"
	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	handlerapp "github.com/sunweilin/forgify/backend/internal/app/handler"
	mcpapp "github.com/sunweilin/forgify/backend/internal/app/mcp"
	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
	memoryapp "github.com/sunweilin/forgify/backend/internal/app/memory"
	askaiapp "github.com/sunweilin/forgify/backend/internal/app/askai"
	relationapp "github.com/sunweilin/forgify/backend/internal/app/relation"
	relationstore "github.com/sunweilin/forgify/backend/internal/infra/store/relation"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	todoapp "github.com/sunweilin/forgify/backend/internal/app/todo"
	userapp "github.com/sunweilin/forgify/backend/internal/app/user"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	asktool "github.com/sunweilin/forgify/backend/internal/app/tool/ask"
	fstool "github.com/sunweilin/forgify/backend/internal/app/tool/filesystem"
	functiontool "github.com/sunweilin/forgify/backend/internal/app/tool/function"
	handlertool "github.com/sunweilin/forgify/backend/internal/app/tool/handler"
	mcptool "github.com/sunweilin/forgify/backend/internal/app/tool/mcp"
	documenttool "github.com/sunweilin/forgify/backend/internal/app/tool/document"
	memorytool "github.com/sunweilin/forgify/backend/internal/app/tool/memory"
	searchtool "github.com/sunweilin/forgify/backend/internal/app/tool/search"
	shelltool "github.com/sunweilin/forgify/backend/internal/app/tool/shell"
	skilltool "github.com/sunweilin/forgify/backend/internal/app/tool/skill"
	subagenttool "github.com/sunweilin/forgify/backend/internal/app/tool/subagent"
	todotool "github.com/sunweilin/forgify/backend/internal/app/tool/todo"
	toolsettool "github.com/sunweilin/forgify/backend/internal/app/tool/toolset"
	webtool "github.com/sunweilin/forgify/backend/internal/app/tool/web"
	workflowtool "github.com/sunweilin/forgify/backend/internal/app/tool/workflow"
	workflowapp "github.com/sunweilin/forgify/backend/internal/app/workflow"
	schedulerapp "github.com/sunweilin/forgify/backend/internal/app/scheduler"
	triggerapp "github.com/sunweilin/forgify/backend/internal/app/trigger"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	handlerdomain "github.com/sunweilin/forgify/backend/internal/domain/handler"
	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	memorydomain "github.com/sunweilin/forgify/backend/internal/domain/memory"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	relationdomain "github.com/sunweilin/forgify/backend/internal/domain/relation"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	tododomain "github.com/sunweilin/forgify/backend/internal/domain/todo"
	userdomain "github.com/sunweilin/forgify/backend/internal/domain/user"
	workflowdomain "github.com/sunweilin/forgify/backend/internal/domain/workflow"
	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	cryptoinfra "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	eventloginfra "github.com/sunweilin/forgify/backend/internal/infra/eventlog"
	forgeinfra "github.com/sunweilin/forgify/backend/internal/infra/forge"
	mcpinfra "github.com/sunweilin/forgify/backend/internal/infra/mcp"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	settingsinfra "github.com/sunweilin/forgify/backend/internal/infra/settings"
	notificationsinfra "github.com/sunweilin/forgify/backend/internal/infra/notifications"
	sandboxinfra "github.com/sunweilin/forgify/backend/internal/infra/sandbox"
	apikeystore "github.com/sunweilin/forgify/backend/internal/infra/store/apikey"
	chatstore "github.com/sunweilin/forgify/backend/internal/infra/store/chat"
	convstore "github.com/sunweilin/forgify/backend/internal/infra/store/conversation"
	functionstore "github.com/sunweilin/forgify/backend/internal/infra/store/function"
	handlerstore "github.com/sunweilin/forgify/backend/internal/infra/store/handler"
	documentstore "github.com/sunweilin/forgify/backend/internal/infra/store/document"
	memorystore "github.com/sunweilin/forgify/backend/internal/infra/store/memory"
	modelcapoverridestore "github.com/sunweilin/forgify/backend/internal/infra/store/modelcapoverride"
	modelstore "github.com/sunweilin/forgify/backend/internal/infra/store/model"
	sandboxstore "github.com/sunweilin/forgify/backend/internal/infra/store/sandbox"
	todostore "github.com/sunweilin/forgify/backend/internal/infra/store/todo"
	userstore "github.com/sunweilin/forgify/backend/internal/infra/store/user"
	workflowstore "github.com/sunweilin/forgify/backend/internal/infra/store/workflow"
	flowrunstore "github.com/sunweilin/forgify/backend/internal/infra/store/flowrun"
	mcpcallstore "github.com/sunweilin/forgify/backend/internal/infra/store/mcpcalls"
	mcphealthstore "github.com/sunweilin/forgify/backend/internal/infra/store/mcphealth"
	skillexecstore "github.com/sunweilin/forgify/backend/internal/infra/store/skillexec"
	eventlogpkg "github.com/sunweilin/forgify/backend/internal/pkg/eventlog"
	forgepkg "github.com/sunweilin/forgify/backend/internal/pkg/forge"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	modelcapspkg "github.com/sunweilin/forgify/backend/internal/pkg/modelcaps"
	notificationspkg "github.com/sunweilin/forgify/backend/internal/pkg/notifications"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
	routerhttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/router"
)

// Option configures the Harness at construction time.
//
// Option 构造时配置 Harness。
type Option func(*options)

type options struct {
	fakeLLMBaseURL  string
	curatedRegistry bool
	sandboxDataDir  string
}

// WithFakeLLMBaseURL routes the injected apikey to a fake LLM server.
//
// WithFakeLLMBaseURL 把 apikey BaseURL 指向 fake LLM server。
func WithFakeLLMBaseURL(url string) Option {
	return func(o *options) { o.fakeLLMBaseURL = url }
}

// WithCuratedRegistry swaps the in-memory test registry for the production CuratedRegistrySource.
//
// WithCuratedRegistry 把测试 registry 换成生产 CuratedRegistrySource。
func WithCuratedRegistry() Option {
	return func(o *options) { o.curatedRegistry = true }
}

// WithSandboxDataDir overrides the per-test t.TempDir() to share mise/npm/uv caches across runs.
//
// WithSandboxDataDir 用指定目录共享 mise/npm/uv 缓存，避免每测重下载。
func WithSandboxDataDir(dir string) Option {
	return func(o *options) { o.sandboxDataDir = dir }
}

// Harness is a booted in-process backend driveable over HTTP.
//
// Harness 是可通过 HTTP 驱动的 in-process 后端。
type Harness struct {
	t      *testing.T
	server *httptest.Server
	log    *zap.Logger

	fakeLLMBaseURL string

	DB                  *gorm.DB
	EventLogBridge      *eventloginfra.Bridge
	NotificationsBridge *notificationsinfra.Bridge
	NotificationsPub    notificationspkg.Publisher
	ForgeBridge         *forgeinfra.Bridge
	ForgePub            forgepkg.Publisher
	ChatEmitter         eventlogpkg.Emitter
	Sandbox             *sandboxapp.Service
	MCP                 *mcpapp.Service
	Skill               *skillapp.Service
	Catalog             *catalogapp.Service
	Memory              *memoryapp.Service
	Document            *documentapp.Service
	Relation            *relationapp.Service
	ContextManager      *contextmgrapp.Manager
	Settings            *settingsinfra.Service
	SettingsPath        string // path to settings.json for tests to write rules
	PermGate            *permgateapp.Gate
	HookRunner          *hooksapp.Runner

	APIKey       *apikeyapp.Service
	Capability   *apikeyapp.CapabilityService
	Model        *modelapp.Service
	Conversation *convapp.Service
	Function     *functionapp.Service
	Handler      *handlerapp.Service
	Workflow     *workflowapp.Service
	Scheduler    *schedulerapp.Service
	Trigger      *triggerapp.Service
	FlowRunRepo  flowrundomain.Repository
	Chat         *chatapp.Service
	User         *userapp.Service
	Tools        []toolapp.Tool
}

// New boots a fresh harness with in-memory SQLite + httptest server; cleanup is registered on t.
//
// New 启动内存 SQLite + httptest 的 harness，清理挂到 t。
func New(t *testing.T, opts ...Option) *Harness {
	t.Helper()

	cfg := &options{}
	for _, o := range opts {
		o(cfg)
	}

	log := zaptest.NewLogger(t)

	gdb, err := dbinfra.Open(dbinfra.Config{DataDir: ""})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Deliberately NOT closing the :memory: DB at teardown. Production spawns
	// fire-and-forget goroutines (autoTitle, subagent reconcile/stop, scheduler
	// node exec) on detached context.Background() that legitimately outlive a
	// request; closing the shared conn races them → flaky "database is closed"
	// under -race. The :memory: DB is reclaimed when the *sql.DB is GC'd at
	// test-binary exit; each New(t) gets its own fresh isolated DB regardless.
	//
	// 故意不在 teardown 关 :memory: DB。生产用 detached context.Background() 起
	// autoTitle / subagent reconcile/stop / scheduler 节点执行等 fire-and-forget
	// goroutine,合法地比请求活得久;关共享连接会和它们抢时序 → -race 下偶发
	// "database is closed"。:memory: DB 在 *sql.DB 被 GC 时(测试二进制退出)自动
	// 回收;每个 New(t) 仍拿到各自全新隔离的 DB。

	// SQLite :memory: per-connection isolation; force single connection so goroutines share tables.
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
		&functiondomain.Function{},
		&functiondomain.Version{},
		&functiondomain.Execution{},
		&handlerdomain.Handler{},
		&handlerdomain.Version{},
		&handlerdomain.Call{},
		&workflowdomain.Workflow{},
		&workflowdomain.Version{},
		&flowrundomain.FlowRun{},
		&flowrundomain.Node{},
		&mcpdomain.Call{},
		&skilldomain.Execution{},
		&sandboxdomain.Runtime{},
		&sandboxdomain.Env{},
		&tododomain.Todo{},
		&memorydomain.Memory{},
		&documentdomain.Document{},
		&userdomain.User{},
		&relationdomain.Relation{},
		&mcpdomain.HealthSnapshot{},
		&modeldomain.ModelCapOverride{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Deterministic test fingerprint so harness can decrypt keys it inserted.
	encryptor, err := cryptoinfra.NewAESGCMEncryptor(
		cryptoinfra.DeriveKey("forgify-pipeline-test-fingerprint"),
	)
	if err != nil {
		t.Fatalf("build encryptor: %v", err)
	}

	// Store handles pulled out so apikey RefScanner setters can reach the same
	// instances used by the *Service constructors (mirrors main.go wiring).
	//
	// 抽出 store 句柄供 apikey RefScanner setter 复用,与 main.go 装配一致。
	modelStoreInst := modelstore.New(gdb)
	convStoreInst := convstore.New(gdb)
	workflowStoreInst := workflowstore.New(gdb)

	apikeyService := apikeyapp.NewService(
		apikeystore.New(gdb),
		encryptor,
		apikeyapp.NewHTTPTester(nil),
		log,
	)
	// Wire ref scanners so apikey.Service.Delete enforces RESTRICT against
	// model_configs / conv overrides / workflow node overrides (mirrors
	// main.go); pipeline tests that need to delete a referenced key bypass
	// via direct DB Exec.
	//
	// 装配 RefScanner 让 apikey.Service.Delete 强制 RESTRICT (镜像 main.go);
	// 需删除被引用 key 的 pipeline 测试通过直接 DB Exec 绕过。
	apikeyService.SetModelConfigRefScanner(modelStoreInst)
	apikeyService.SetConvOverrideRefScanner(convStoreInst)
	apikeyService.SetNodeOverrideRefScanner(workflowStoreInst)

	capabilityService := apikeyapp.NewCapabilityService(modelcapoverridestore.New(gdb))

	modelService := modelapp.NewService(modelStoreInst, apikeyService, log)

	llmFactory := llminfra.NewFactory()

	// PluginSandbox v2 rooted at per-test tempdir; Bootstrap failure → degraded mode.
	var dataDir string
	if cfg.sandboxDataDir != "" {
		dataDir = cfg.sandboxDataDir
	} else {
		dataDir = t.TempDir()
	}
	eventLogBridge := eventloginfra.NewBridge(log)
	notificationsBridge := notificationsinfra.NewBridge(log)
	notificationsPub := notificationspkg.New(notificationsBridge, log)
	forgeBridge := forgeinfra.NewBridge(log)
	forgePub := forgepkg.New(forgeBridge, log)

	sandboxRepo := sandboxstore.New(gdb)
	sandboxSvc := sandboxapp.New(sandboxRepo, dataDir, notificationsPub, log)
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

	convService := convapp.NewService(convStoreInst, notificationsPub, log)
	convService.SetKeyProvider(apikeyService) // §12.3

	functionService := functionapp.NewService(
		functionstore.New(gdb),
		functionapp.NewSandboxAdapter(sandboxSvc, dataDir),
		notificationsPub,
		log,
	)

	handlerService := handlerapp.NewService(
		handlerstore.New(gdb),
		handlerapp.NewSandboxAdapter(sandboxSvc, dataDir),
		handlerapp.DefaultClientFactory,
		encryptor,
		notificationsPub,
		log,
	)
	t.Cleanup(func() {
		handlerService.Shutdown(context.Background())
	})

	// Skill + MCP backfilled below once those services exist.
	workflowChecker := &workflowapp.ProductionChecker{
		Function: functionService,
		Handler:  handlerService,
	}
	workflowService := workflowapp.NewService(
		workflowStoreInst,
		workflowChecker,
		notificationsPub,
		log,
	)
	workflowService.SetKeyProvider(apikeyService) // enable F1 validation on node modelOverride

	chatRepo := chatstore.New(gdb)
	chatEmitter := eventlogpkg.New(eventLogBridge, chatRepo, log)
	chatService := chatapp.NewService(
		chatRepo,
		convStoreInst,
		modelService,
		apikeyService,
		llmFactory,
		chatEmitter,
		notificationsPub,
		dataDir,
		log,
	)

	pathGuard := pathguardpkg.NewDefault()

	tools := functiontool.FunctionTools(
		functionService, modelService, apikeyService, llmFactory, forgePub, log,
	)
	tools = append(tools, handlertool.HandlerTools(
		handlerService, modelService, apikeyService, llmFactory, forgePub, log,
	)...)
	tools = append(tools, workflowtool.WorkflowTools(
		workflowService, forgePub, log,
	)...)
	tools = append(tools, fstool.FilesystemTools(pathGuard)...)
	tools = append(tools, searchtool.SearchTools(pathGuard, log)...)
	// nil MCP router in harness = BYOK + Bing CN only; tests that need MCP construct WebSearch directly.
	tools = append(tools, webtool.WebTools(modelService, apikeyService, llmFactory, nil, log)...)
	shells := shelltool.NewShellTools(sandboxSvc)
	t.Cleanup(shells.Manager.Stop)
	tools = append(tools, shells.Tools...)
	todoService := todoapp.NewService(todostore.New(gdb), notificationsPub, log)
	userService := userapp.NewService(userstore.New(gdb), log)
	// Pipeline tests seed their own users via h.SeedCtx(t) / h.LocalCtxAs(t, id);
	// no boot-time seed needed now that EnsureDefault is deleted.
	//
	// pipeline 测试用 h.SeedCtx(t) 自助 seed,不再需要启动期种子。
	tools = append(tools, todotool.TodoTools(todoService)...)
	askService := askapp.NewService()
	tools = append(tools, asktool.AskTools(askService)...)

	memoryService := memoryapp.New(memorystore.New(gdb), notificationsPub, log)
	tools = append(tools, memorytool.MemoryTools(memoryService)...)

	documentService := documentapp.New(documentstore.New(gdb), notificationsPub, log)
	tools = append(tools, documenttool.DocumentTools(documentService)...)

	subagentRegistry := subagentapp.NewRegistry()
	subagentService := subagentapp.New(
		chatRepo,
		subagentRegistry,
		modelService,
		apikeyService,
		llmFactory,
		log,
	)
	tools = append(tools, subagenttool.SubagentTools(subagentService)...)
	subagentService.SetTools(tools)

	// per-test tempdir for mcp.json + in-memory registry (curated swapped in via WithCuratedRegistry).
	mcpConfigPath := filepath.Join(dataDir, "mcp.json")
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
	tools = append(tools, mcptool.MCPTools(mcpService)...)

	// per-test tempdir SkillsDir; tests seed and either call Scan or rely on 1s polling.
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

	// mechanical-only, on-demand catalog (no background poll / no LLM / no disk).
	catalogService := catalogapp.New(log)
	catalogService.RegisterSource(functionService.AsCatalogSource())
	catalogService.RegisterSource(handlerService.AsCatalogSource())
	catalogService.RegisterSource(skillService.AsCatalogSource())
	catalogService.RegisterSource(mcpService.AsCatalogSource())
	catalogService.RegisterSource(workflowService.AsCatalogSource())
	catalogService.RegisterSource(documentService.AsCatalogSource())
	chatService.SetSystemPromptProvider(catalogService)
	chatService.SetMemoryProvider(memoryService)
	chatService.SetDocumentResolver(documentService)

	// Relation domain wiring (mirrors cmd/server/main.go pattern).
	relationService := relationapp.NewService(relationapp.Config{
		Repo:               relationstore.New(gdb),
		FunctionReader:     functionService,
		HandlerReader:      handlerService,
		WorkflowReader:     workflowService,
		DocumentReader:     documentService,
		SkillReader:        skillService,
		McpReader:          mcpService,
		ConversationReader: convService,
		Log:                log,
	})
	workflowService.SetRelationSyncer(relationService)
	functionService.SetRelationSyncer(relationService)
	handlerService.SetRelationSyncer(relationService)
	documentService.SetRelationSyncer(relationService)
	convService.SetRelationSyncer(relationService)
	mcpService.SetRelationSyncer(relationService)
	skillService.SetRelationSyncer(relationService)

	askaiSpawner := askaiapp.New(convService, chatService, log)

	cheapLLMResolver := func(ctx context.Context) (llminfra.Client, string, string, string, *llminfra.ThinkingSpec, error) {
		bundle, err := llmclientpkg.ResolveUtility(ctx, modelService, apikeyService, llmFactory)
		if err != nil {
			return nil, "", "", "", nil, err
		}
		return bundle.Client, bundle.ModelID, bundle.Key, bundle.BaseURL, bundle.Thinking, nil
	}
	contextManager := contextmgrapp.New(
		chatRepo, convStoreInst, chatEmitter, notificationsPub, cheapLLMResolver, log)
	contextManager.SetCapabilityResolver(func(_ context.Context, provider, modelID string) modelcapspkg.Cap {
		return modelcapspkg.Lookup(provider, modelID)
	})
	chatService.SetContextCompactor(contextManager)
	// Drain detached goroutines (autoTitle) before the DB-close cleanup runs.
	// t.Cleanup is LIFO: registering Wait here (after the DB close registered above)
	// ensures Wait fires before DB close.
	//
	// autoTitle 等 detached goroutine 必须在 DB 关闭前排空；t.Cleanup LIFO 保证顺序。
	t.Cleanup(chatService.Wait)

	// V1.2 §3 final-sweep — permissions + hooks. Test harness points at
	// a per-test settings.json under t.TempDir() (created lazily); tests
	// write a custom file when they need rules.
	// V1.2 §3 ——permissions + hooks。test harness 指向 t.TempDir()/
	// settings.json（懒建）；测试需规则时自写文件。
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	settingsService := settingsinfra.New(settingsPath, log)
	if err := settingsService.Start(context.Background()); err != nil {
		t.Logf("settings start failed (continuing with empty defaults): %v", err)
	}
	t.Cleanup(settingsService.Close)
	permGate := permgateapp.New(settingsService)
	hookRunner := hooksapp.New(settingsService, log)
	chatService.SetPermissionsAndHooks(permGate, hookRunner)

	workflowChecker.Skill = skillService
	workflowChecker.MCP = mcpService
	workflowChecker.Document = documentService

	flowrunRepo := flowrunstore.New(gdb)
	mcpCallRepo := mcpcallstore.New(gdb)
	mcpHealthRepo := mcphealthstore.New(gdb)
	mcpService.SetHealthHistoryRepo(mcpHealthRepo)
	skillExecRepo := skillexecstore.New(gdb)
	mcpService.SetCallRepo(mcpCallRepo)
	skillService.SetExecRepo(skillExecRepo)

	httpMux := http.NewServeMux()
	triggerService := triggerapp.New(httpMux, log)
	schedulerService := schedulerapp.NewService(flowrunRepo, workflowService, notificationsPub, log)
	triggerService.SetScheduler(schedulerService)
	t.Cleanup(triggerService.Shutdown)

	router := schedulerapp.NewRouter()
	router.Set(workflowdomain.NodeTypeTrigger, schedulerapp.NewTriggerDispatcher())
	router.Set(workflowdomain.NodeTypeFunction, schedulerapp.NewFunctionDispatcher(functionService))
	router.Set(workflowdomain.NodeTypeHandler, schedulerapp.NewHandlerDispatcher(handlerService))
	router.Set(workflowdomain.NodeTypeMCP, schedulerapp.NewMCPDispatcher(mcpService))
	router.Set(workflowdomain.NodeTypeSkill, schedulerapp.NewSkillDispatcher(skillService))
	router.Set(workflowdomain.NodeTypeLLM, schedulerapp.NewLLMDispatcher(
		schedulerapp.NewDefaultLLMCaller(modelService, apikeyService, llmFactory),
		documentService,
	))
	router.Set(workflowdomain.NodeTypeAgent, schedulerapp.NewAgentDispatcher(
		modelService, apikeyService, llmFactory,
		documentService, func() []toolapp.Tool { return tools }, log,
	))
	router.Set(workflowdomain.NodeTypeHTTP, schedulerapp.NewHTTPDispatcher(nil))
	router.Set(workflowdomain.NodeTypeCondition, schedulerapp.NewConditionDispatcher())
	router.Set(workflowdomain.NodeTypeLoop, schedulerapp.NewLoopDispatcher(schedulerService))
	router.Set(workflowdomain.NodeTypeParallel, schedulerapp.NewParallelDispatcher())
	router.Set(workflowdomain.NodeTypeApproval, schedulerapp.NewApprovalDispatcher())
	router.Set(workflowdomain.NodeTypeWait, schedulerapp.NewWaitDispatcher())
	router.Set(workflowdomain.NodeTypeVariable, schedulerapp.NewVariableDispatcher())
	schedulerService.SetRouter(router)

	tools = append(tools, workflowtool.WorkflowExecutionTools(flowrunRepo)...)
	tools = append(tools, mcptool.MCPCallLogTools(mcpCallRepo)...)
	tools = append(tools, skilltool.SkillExecutionTools(skillExecRepo)...)

	// Mirrors cmd/server/main.go: partition tools, inject activate_tools as RESIDENT.
	// host.Tools(ctx) returns resident + activated lazy groups (on-demand); tools is
	// re-flattened to the full set here only for §18 inventory handlers (Deps.Tools).
	//
	// 镜像 main.go：拆分工具，activate_tools 注入为 RESIDENT。host.Tools(ctx) 返
	// resident + 已激活 lazy 组（按需）；这里把 tools 重新展平成全集仅供 §18 总览 handler。
	ts := buildHarnessToolset(tools)
	ts.Resident = append(ts.Resident, toolsettool.NewActivateTools(ts))
	chatService.SetToolset(ts)
	tools = ts.All()

	handler := routerhttpapi.New(routerhttpapi.Deps{
		Log:                 log,
		APIKeyService:       apikeyService,
		CapabilityService:   capabilityService,
		ModelService:        modelService,
		ConversationService: convService,
		FunctionService:     functionService,
		HandlerService:      handlerService,
		WorkflowService:     workflowService,
		FlowRunRepo:         flowrunRepo,
		SchedulerService:    schedulerService,
		TriggerService:      triggerService,
		Mux:                 httpMux,
		ChatService:         chatService,
		EventLogBridge:      eventLogBridge,
		BlockV2Repo:         chatRepo,
		NotificationsBridge: notificationsBridge,
		ForgeBridge:         forgeBridge,
		AskService:          askService,
		SandboxService:      sandboxSvc,
		SubagentService:     subagentService,
		MCPService:          mcpService,
		SkillService:        skillService,
		CatalogService:      catalogService,
		MemoryService:       memoryService,
		DocumentService:     documentService,
		RelationService:     relationService,
		AskAISpawner:        askaiSpawner,
		UserService:         userService,
		SettingsService:     settingsService,
		SettingsPath:        settingsPath,
		PermGate:            permGate,
		Dev:                 false,
		Tools:               tools,
		SubagentRegistry:    subagentRegistry,
		LLMFactory:          llmFactory,
		ShellManager:        shells.Manager,
		DB:                  gdb,
		Port:                0,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	// Auto-seed the canonical "test-user" so HTTP requests (which carry
	// X-Forgify-User-ID: test-user via requestJSON / DoRequest) clear the
	// IdentifyUser+RequireUser middleware pair. Tests calling LocalCtxAs(id)
	// for a different user still work — that method seeds its own user.
	//
	// 自动 seed 经典 "test-user",让 HTTP 请求(经 requestJSON / DoRequest 默认带
	// X-Forgify-User-ID: test-user)能通过 IdentifyUser+RequireUser 中间件。
	// 测试用 LocalCtxAs(id) 指定其他 user 时,该方法自己 seed。
	if _, err := userService.EnsureExists(context.Background(), SeedTestUserID, "test"); err != nil {
		t.Fatalf("auto-seed test user: %v", err)
	}

	return &Harness{
		t:                   t,
		server:              srv,
		log:                 log,
		fakeLLMBaseURL:      cfg.fakeLLMBaseURL,
		DB:                  gdb,
		EventLogBridge:      eventLogBridge,
		NotificationsBridge: notificationsBridge,
		NotificationsPub:    notificationsPub,
		ForgeBridge:         forgeBridge,
		ForgePub:            forgePub,
		ChatEmitter:         chatEmitter,
		Sandbox:             sandboxSvc,
		MCP:                 mcpService,
		Skill:               skillService,
		Catalog:             catalogService,
		APIKey:              apikeyService,
		Capability:          capabilityService,
		Model:               modelService,
		Conversation:        convService,
		Function:            functionService,
		Handler:             handlerService,
		Workflow:            workflowService,
		Scheduler:           schedulerService,
		Trigger:             triggerService,
		FlowRunRepo:         flowrunRepo,
		Chat:                chatService,
		User:                userService,
		Memory:              memoryService,
		Document:            documentService,
		Relation:            relationService,
		ContextManager:      contextManager,
		Settings:            settingsService,
		SettingsPath:        settingsPath,
		PermGate:            permGate,
		HookRunner:          hookRunner,
		Tools:               tools,
	}
}

// lazyGroupsHarness is the same name→category mapping as cmd/server/main.go::lazyGroups.
// Duplicated here because harness uses the pipeline build tag (not shared with cmd/server).
//
// lazyGroupsHarness 与 main.go::lazyGroups 相同，因 pipeline build tag 隔离而复制。
var lazyGroupsHarness = map[string]string{
	"create_function":            "function",
	"edit_function":              "function",
	"delete_function":            "function",
	"revert_function":            "function",
	"get_function":               "function",
	"get_function_execution":     "function",
	"search_function_executions": "function",
	"create_handler":             "handler",
	"edit_handler":               "handler",
	"delete_handler":             "handler",
	"revert_handler":             "handler",
	"get_handler":                "handler",
	"update_handler_config":      "handler",
	"get_handler_call":           "handler",
	"search_handler_calls":       "handler",
	"create_workflow":             "workflow",
	"edit_workflow":               "workflow",
	"delete_workflow":             "workflow",
	"revert_workflow":             "workflow",
	"get_workflow":                "workflow",
	"get_workflow_execution":      "workflow",
	"search_workflow_executions":  "workflow",
	// trigger_workflow is mapped here for consistency but WorkflowTriggerTool is not assembled in harness.
	"trigger_workflow":  "workflow",
	"call_mcp_tool":     "mcp",
	"install_mcp_server":   "mcp",
	"uninstall_mcp_server": "mcp",
	"list_mcp_marketplace": "mcp",
	"get_mcp_call":         "mcp",
	"search_mcp_calls":     "mcp",
	"create_document":  "document",
	"edit_document":    "document",
	"delete_document":  "document",
	"move_document":    "document",
	"read_document":    "document",
	"list_documents":   "document",
	"search_documents": "document",
	"get_skill_execution":    "skill",
	"search_skill_executions": "skill",
}

// residentToolNamesHarness mirrors cmd/server/main.go::residentToolNames.
// activate_tools is excluded: it is appended to ts.Resident after buildHarnessToolset returns.
//
// residentToolNamesHarness 镜像 main.go::residentToolNames；activate_tools 在函数返回后追加。
var residentToolNamesHarness = map[string]bool{
	"search_function":  true,
	"search_handler":   true,
	"search_workflow":  true,
	"search_skills":    true,
	"search_mcp_tools": true,
	"run_function":     true,
	"call_handler":     true,
	"Read":             true,
	"Write":            true,
	"Edit":             true,
	"Grep":             true,
	"Glob":             true,
	"Bash":             true,
	"BashOutput":       true,
	"KillShell":        true,
	"WebSearch":        true,
	"WebFetch":         true,
	"AskUserQuestion":  true,
	"TodoCreate":       true,
	"TodoUpdate":       true,
	"TodoList":         true,
	"TodoGet":          true,
	"read_memory":      true,
	"write_memory":     true,
	"forget_memory":    true,
	"activate_skill":   true,
	"Subagent":         true,
}

// buildHarnessToolset mirrors cmd/server/main.go::buildToolset with the same closed-mapping panic.
//
// buildHarnessToolset 镜像 main.go::buildToolset，使用相同的封闭映射+panic。
func buildHarnessToolset(all []toolapp.Tool) toolapp.Toolset {
	ts := toolapp.Toolset{
		Lazy: make(map[string][]toolapp.Tool),
	}
	for _, t := range all {
		name := t.Name()
		if cat, ok := lazyGroupsHarness[name]; ok {
			ts.Lazy[cat] = append(ts.Lazy[cat], t)
		} else if residentToolNamesHarness[name] {
			ts.Resident = append(ts.Resident, t)
		} else {
			panic(fmt.Sprintf("buildHarnessToolset: tool %q is not classified — add it to lazyGroupsHarness or residentToolNamesHarness", name))
		}
	}
	return ts
}

// registerSandboxStack mirrors cmd/server/main.go::registerSandboxStack.
//
// registerSandboxStack 镜像 main.go 的同名 helper。
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

// HTTP entry points (URL / HTTPClient / Post / Get / Patch / Delete /
// DoRequest / UploadFile) live in http.go.
// Live-mode env gates (RequireDeepSeekKey / RequireSandboxResources) live
// in live_gate.go. DB helpers live in db.go. Block / SSE / errcode
// assertions live in assertions.go.
//
// HTTP 入口、live 模式 env gate、DB helper、断言 helper 均已搬至同包对应文件。

