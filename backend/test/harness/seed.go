//go:build pipeline

package harness

import (
	"context"
	"encoding/json"
	"testing"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
	functionapp "github.com/sunweilin/forgify/backend/internal/app/function"
	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	functiondomain "github.com/sunweilin/forgify/backend/internal/domain/function"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ProviderDeepSeek is the provider name string apikey/model expect.
//
// ProviderDeepSeek 是 apikey/model 期望的 provider 名。
const ProviderDeepSeek = "deepseek"

// SimpleFunctionCode is minimal valid Python for function pipeline tests.
//
// SimpleFunctionCode 是 function pipeline 测试用的最小 Python。
const SimpleFunctionCode = `def hello(name: str) -> str:
    """Greet someone.

    Args:
        name: Person's name.

    Returns:
        Greeting message.
    """
    return f"Hello, {name}!"
`

// SeedTestUserID is the fixture user id used by SeedCtx for pipeline tests.
//
// SeedTestUserID:pipeline 测试的固定 fixture user id。
const SeedTestUserID = "test-user"

// SeedCtx ensures a fixture user (id="test-user") exists in DB and returns
// a ctx stamped with that id. Replaces the old LocalCtx() which relied on
// the now-deleted DefaultLocalUserID seed.
//
// SeedCtx:保证 fixture user("test-user")存在,返回带 user 的 ctx。
// 替代旧的 LocalCtx(依赖已删除的 DefaultLocalUserID seed)。
func (h *Harness) SeedCtx(t *testing.T) context.Context {
	t.Helper()
	if _, err := h.User.EnsureExists(context.Background(), SeedTestUserID, "test"); err != nil {
		t.Fatalf("seed test user: %v", err)
	}
	return reqctxpkg.SetUserID(context.Background(), SeedTestUserID)
}

// LocalCtxAs seeds a user with the given id and returns a ctx stamped with it.
// For tests that need a fixture id different from SeedTestUserID.
//
// LocalCtxAs:用指定 id 建用户并返 ctx,给需要自定义 id 的测试用。
func (h *Harness) LocalCtxAs(t *testing.T, id string) context.Context {
	t.Helper()
	if _, err := h.User.EnsureExists(context.Background(), id, id); err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
	return reqctxpkg.SetUserID(context.Background(), id)
}

// LocalCtx is the legacy name retained for internal seed.go helpers; new
// tests should call SeedCtx(t) directly. Asserts on t being available via
// the harness — fatal if not, since DB-seed without t can't recover.
//
// LocalCtx:保留兼容老 helper;新测试用 SeedCtx(t)。
func (h *Harness) LocalCtx() context.Context {
	return h.SeedCtx(h.t)
}

// SeedDeepSeek inserts a DeepSeek API key + chat scenario; empty apiKey falls back to env.
//
// SeedDeepSeek 插入 DeepSeek API key + chat scenario，apiKey 空时走 env。
func (h *Harness) SeedDeepSeek(t *testing.T, apiKey string) {
	t.Helper()
	if apiKey == "" {
		apiKey = RequireDeepSeekKey(t)
	}
	ctx := h.LocalCtx()

	key, err := h.APIKey.Create(ctx, apikeyapp.CreateInput{
		Provider:    ProviderDeepSeek,
		DisplayName: "pipeline-deepseek",
		Key:         apiKey,
		BaseURL:     h.fakeLLMBaseURL,
	})
	if err != nil {
		t.Fatalf("seed apikey: %v", err)
	}

	// Seed all 3 scenarios (dialogue/utility/agent) pointing to this key so
	// any LLM callsite resolves cleanly in pipeline tests.
	//
	// 给 3 个 scenario 都种入这把 key,任何 LLM 调用点解析都能通。
	for _, scenario := range modeldomain.ListScenarios() {
		if _, err := h.Model.Upsert(ctx, scenario, modelapp.UpsertInput{
			APIKeyID: key.ID,
			ModelID:  "deepseek-chat",
		}); err != nil {
			t.Fatalf("seed model config %s: %v", scenario, err)
		}
	}
}

// NewConversation creates a fresh conversation via the service; returns entity with ID.
//
// NewConversation 通过 service 建对话，返带 ID 的 entity。
func (h *Harness) NewConversation(t *testing.T, title string) *convdomain.Conversation {
	t.Helper()
	c, err := h.Conversation.Create(h.LocalCtx(), title)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	return c
}

// NewFunction creates a function with name + Python code via set_meta + set_code ops.
//
// NewFunction 用 set_meta + set_code ops 建 function。
func (h *Harness) NewFunction(t *testing.T, name, code string) *functiondomain.Function {
	t.Helper()
	rawMeta, _ := json.Marshal(map[string]any{"name": name})
	rawCode, _ := json.Marshal(map[string]any{"code": code})
	ops := []functionapp.Op{
		{Type: "set_meta", Raw: rawMeta},
		{Type: "set_code", Raw: rawCode},
	}
	f, _, err := h.Function.Create(h.LocalCtx(), functionapp.CreateInput{Ops: ops})
	if err != nil {
		t.Fatalf("create function %q: %v", name, err)
	}
	return f
}

// RequireFunctionResources skips the test when v2 sandbox isn't ready (run `make resources`).
//
// RequireFunctionResources v2 sandbox 未 ready 则 skip。
func RequireFunctionResources(t *testing.T, h *Harness) {
	t.Helper()
	if !h.Sandbox.IsReady() {
		err := h.Sandbox.BootstrapError()
		t.Skipf("sandbox v2 not ready (run `make resources` to embed mise): %v", err)
	}
}
