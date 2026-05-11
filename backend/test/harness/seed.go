//go:build pipeline

// seed.go — quick fixture helpers built on top of the harness Service layer.
// Use these at the start of a pipeline test to get to "ready to chat" in 1-2 lines.
//
// seed.go — 基于 harness Service 层的 fixture helper。pipeline 测试开头几行就能
// 走到"准备聊天"状态。
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

// ProviderDeepSeek is the provider name string the apikey/model layers expect.
// Lives here (not in domain) since the domain treats provider as a free string.
//
// ProviderDeepSeek 是 apikey/model 层期望的 provider 名字字符串。放这里
// （不放 domain）因为 domain 把 provider 当自由字符串。
const ProviderDeepSeek = "deepseek"

// SimpleFunctionCode is minimal valid Python for function pipeline tests.
//
// SimpleFunctionCode 是 function pipeline 测试用的最小可用 Python。
const SimpleFunctionCode = `def hello(name: str) -> str:
    """Greet someone.

    Args:
        name: Person's name.

    Returns:
        Greeting message.
    """
    return f"Hello, {name}!"
`

// LocalCtx returns a context stamped with the default local user — the same
// user the InjectUserID middleware stamps for HTTP requests. Use this for
// service-layer calls that bypass HTTP.
//
// LocalCtx 返回打了默认本地用户的 ctx——与 InjectUserID 中间件给 HTTP 请求
// 打的 user 一致。绕过 HTTP 直接调 service 层时用这个。
func (h *Harness) LocalCtx() context.Context {
	return reqctxpkg.SetUserID(context.Background(), reqctxpkg.DefaultLocalUserID)
}

// SeedDeepSeek inserts a DeepSeek API key + chat scenario model config so
// chat flows can resolve credentials. apiKey defaults to env DEEPSEEK_API_KEY
// when empty (use RequireDeepSeekKey to fail-skip on missing).
//
// SeedDeepSeek 插入 DeepSeek API key + chat scenario 模型配置，让 chat 流能
// 解出 credentials。apiKey 为空时用环境 DEEPSEEK_API_KEY（缺时用
// RequireDeepSeekKey 让 test skip）。
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
		BaseURL:     h.fakeLLMBaseURL, // non-empty → routes calls to FakeLLMServer
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

// NewConversation creates a fresh conversation via the conversation service.
// Returns the entity (with allocated ID) for further operations.
//
// NewConversation 通过 conversation service 新建一个对话，返回带分配 ID 的 entity。
func (h *Harness) NewConversation(t *testing.T, title string) *convdomain.Conversation {
	t.Helper()
	c, err := h.Conversation.Create(h.LocalCtx(), title)
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	return c
}

// NewFunction creates a function with the given name + Python code via the
// function service. Builds a minimal ops sequence (set_meta + set_code) so
// validation passes; tests that need parameters / dependencies / etc construct
// their own DirectCreateInput.
//
// NewFunction 通过 function service 新建一个 function。仅 set_meta + set_code
// 让 final 校验通过;需要 parameters/deps 的测试自构 DirectCreateInput。
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

// RequireFunctionResources skips the test when the v2 sandbox isn't ready —
// either the mise binary wasn't embedded for the current platform (run
// `make resources`) or Bootstrap failed for some other reason. All function-
// sandbox pipeline tests should call this at the top.
//
// RequireFunctionResources 在 v2 sandbox 未 ready 时 skip。
func RequireFunctionResources(t *testing.T, h *Harness) {
	t.Helper()
	if !h.Sandbox.IsReady() {
		err := h.Sandbox.BootstrapError()
		t.Skipf("sandbox v2 not ready (run `make resources` to embed mise): %v", err)
	}
}
