//go:build pipeline

// Package test is the whole-stack pipeline test harness. It boots the same DI graph as
// cmd/server/main.go (real Bridge, real LLM client, real Python sandbox) but
// with in-memory SQLite and an httptest server, and exposes helpers so test
// cases can drive the system through HTTP and observe SSE without ceremony.
//
// Build tag `pipeline` keeps these files out of the default `go test ./...` path —
// they hit real DeepSeek API and spawn Python subprocesses, so they belong to
// `make test-pipeline` (which sources .env and adds -tags=pipeline).
//
// Package test 是pipeline 测试脚手架。装配的 DI 图与 cmd/server/main.go 一致
// （真 Bridge / 真 LLM 客户端 / 真 Python sandbox），区别仅在于用内存 SQLite
// 和 httptest server，并提供 helper 让测试用例通过 HTTP 驱动 + 观测 SSE。
//
// `pipeline` build tag 让这些文件默认不进 `go test ./...`——它们调真 DeepSeek API
// 并起 Python 子进程，归 `make test-pipeline`（自动 source .env + 加 -tags=pipeline）使用。
package test

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
	chatapp "github.com/sunweilin/forgify/backend/internal/app/chat"
	convapp "github.com/sunweilin/forgify/backend/internal/app/conversation"
	forgeapp "github.com/sunweilin/forgify/backend/internal/app/forge"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	forgetool "github.com/sunweilin/forgify/backend/internal/app/tool/forge"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
	forgedomain "github.com/sunweilin/forgify/backend/internal/domain/forge"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
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
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
	routerhttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/router"
)

// Harness is a booted in-process backend ready to drive over HTTP.
//
// Harness 是 boot 完成、可通过 HTTP 驱动的 in-process 后端。
type Harness struct {
	t      *testing.T
	server *httptest.Server
	log    *zap.Logger

	DB     *gorm.DB
	Bridge eventsdomain.Bridge

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
// New 启动一个全新的 harness，底层是内存 SQLite。httptest server 启动、应用
// 与生产一致的迁移、DI 图与 main.go 对齐。清理（server 停 + DB 关）注册到 t。
func New(t *testing.T) *Harness {
	t.Helper()

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

	forgeLLM := &forgeLLMAdapter{picker: modelService, keys: apikeyService, factory: llmFactory}
	bridge := memoryinfra.NewBridge(log)
	forgeService := forgeapp.NewService(
		forgestore.New(gdb),
		sandboxinfra.New("python3"),
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

	tools := forgetool.ForgeTools(
		forgeService, chatRepo, modelService, apikeyService, llmFactory,
	)
	chatService.SetTools(tools)

	handler := routerhttpapi.New(routerhttpapi.Deps{
		Log:                 log,
		APIKeyService:       apikeyService,
		ModelService:        modelService,
		ConversationService: convService,
		ForgeService:        forgeService,
		ChatService:         chatService,
		EventsBridge:        bridge,
		Dev:                 false,
		Tools:               tools,
		DB:                  gdb,
		Port:                0,
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return &Harness{
		t:            t,
		server:       srv,
		log:          log,
		DB:           gdb,
		Bridge:       bridge,
		APIKey:       apikeyService,
		Model:        modelService,
		Conversation: convService,
		Forge:        forgeService,
		Chat:         chatService,
		Tools:        tools,
	}
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
