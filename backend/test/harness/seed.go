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

// LocalCtx returns a context stamped with DefaultLocalUserID.
//
// LocalCtx 返回打了 DefaultLocalUserID 的 ctx。
func (h *Harness) LocalCtx() context.Context {
	return reqctxpkg.SetUserID(context.Background(), reqctxpkg.DefaultLocalUserID)
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

	if _, err := h.APIKey.Create(ctx, apikeyapp.CreateInput{
		Provider:    ProviderDeepSeek,
		DisplayName: "pipeline-deepseek",
		Key:         apiKey,
		BaseURL:     h.fakeLLMBaseURL,
	}); err != nil {
		t.Fatalf("seed apikey: %v", err)
	}

	if _, err := h.Model.Upsert(ctx, modeldomain.ScenarioChat, modelapp.UpsertInput{
		Provider: ProviderDeepSeek,
		ModelID:  "deepseek-chat",
	}); err != nil {
		t.Fatalf("seed model config: %v", err)
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
