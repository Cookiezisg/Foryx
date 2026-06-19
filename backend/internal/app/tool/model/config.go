// Package model provides the read-only LLM tool for inspecting THIS workspace's model
// configuration — the default model per scenario, the configured API keys (MASKED), and the
// available models — so the agent answers "what am I running on" from the real config instead of
// grepping the host filesystem (which leaks the plaintext key and confabulates a wrong audit, F68).
// Lazy tool, surfaced via search_tools.
//
// Package model 提供只读 LLM 工具查看本 workspace 的模型配置——各 scenario 默认模型、已配 API key
// （脱敏）、可用模型——使 agent 从真配置答「我在用什么」而非 grep 主机 FS（会泄露明文 key 并臆造，F68）。
package model

import (
	"context"
	"encoding/json"
	"fmt"

	apikeyapp "github.com/sunweilin/anselm/backend/internal/app/apikey"
	modelapp "github.com/sunweilin/anselm/backend/internal/app/model"
	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	workspaceapp "github.com/sunweilin/anselm/backend/internal/app/workspace"
	apikeydomain "github.com/sunweilin/anselm/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/anselm/backend/internal/domain/model"
)

// ModelConfigTools constructs the read-only model-config tool over the workspace / apikey /
// capability services.
//
// ModelConfigTools 在 workspace / apikey / capability 服务之上构造只读模型配置工具。
func ModelConfigTools(ws *workspaceapp.Service, keys *apikeyapp.Service, caps *modelapp.CapabilityService) []toolapp.Tool {
	return []toolapp.Tool{&GetModelConfig{ws: ws, keys: keys, caps: caps}}
}

// GetModelConfig answers "what model/keys am I configured with" from the real workspace state.
//
// GetModelConfig 从真实 workspace 状态回答「我配了什么模型/key」。
type GetModelConfig struct {
	ws   *workspaceapp.Service
	keys *apikeyapp.Service
	caps *modelapp.CapabilityService
}

func (t *GetModelConfig) Name() string { return "get_model_config" }

func (t *GetModelConfig) Description() string {
	return "Show THIS workspace's model configuration so you can answer 'what model am I running on' / 'which keys are configured' from the REAL config — never guess or read files for this. Returns: defaultModels (the model per scenario — dialogue=chat replies, utility=titles/summaries, agent=agent invokes), apiKeys (the configured keys, MASKED — the secret value is never exposed), and availableModels (what each key can serve). Read-only, no side effects."
}

func (t *GetModelConfig) Parameters() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}

func (t *GetModelConfig) ValidateInput(json.RawMessage) error { return nil }

func (t *GetModelConfig) Execute(ctx context.Context, _ string) (string, error) {
	// Default model per scenario; an unconfigured scenario reports so rather than erroring.
	// 各 scenario 的默认模型；未配的 scenario 明示而非报错。
	defaults := map[string]any{}
	for _, sc := range modeldomain.ListScenarios() {
		ref, err := t.ws.Pick(ctx, sc)
		if err != nil {
			defaults[sc] = "not configured"
			continue
		}
		defaults[sc] = map[string]string{"apiKeyId": ref.APIKeyID, "modelId": ref.ModelID}
	}
	// Configured keys — List returns the MASKED form only (key_encrypted is json:"-", never serialized).
	// 已配 key——List 只返脱敏形（key_encrypted 是 json:"-"、绝不序列化）。
	keyList, _, err := t.keys.List(ctx, apikeydomain.ListFilter{Limit: 200})
	if err != nil {
		return "", fmt.Errorf("get_model_config: list keys: %w", err)
	}
	apiKeys := make([]map[string]any, 0, len(keyList))
	for _, k := range keyList {
		apiKeys = append(apiKeys, map[string]any{
			"id": k.ID, "provider": k.Provider, "displayName": k.DisplayName,
			"keyMasked": k.KeyMasked, "baseUrl": k.BaseURL, "testStatus": k.TestStatus,
		})
	}
	// Available models per probed key (best-effort — a catalog read failure must not fail the tool).
	// 各已探 key 的可用模型（尽力——目录读失败不该使工具失败）。
	available := make([]map[string]any, 0)
	if views, err := t.caps.List(ctx); err == nil {
		for _, c := range views {
			available = append(available, map[string]any{
				"apiKeyId": c.APIKeyID, "provider": c.Provider, "modelId": c.ModelID,
				"displayName": c.DisplayName, "contextWindow": c.ContextWindow,
			})
		}
	}
	return toolapp.ToJSON(map[string]any{
		"defaultModels": defaults, "apiKeys": apiKeys, "availableModels": available,
	}), nil
}
