// Package llmclient resolves the per-request LLM client via the canonical
// three-step dance shared by chat / subagent / forge / workflow callsites:
// picker.PickForX → keys.ResolveCredentialsByID → factory.Build.
//
// Package llmclient 通过 chat / subagent / forge / workflow 共享的三段式
// (picker.PickForX → keys.ResolveCredentialsByID → factory.Build) 解析 per-request LLM。
package llmclient

import (
	"context"
	"errors"
	"fmt"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// Step sentinels distinguish which resolve stage failed for stage-specific error mapping.
//
// Step sentinel 区分解析阶段错误,让调用方按阶段分发错误码。
var (
	ErrPickModel    = errors.New("llmclient: pick model failed")
	ErrResolveCreds = errors.New("llmclient: resolve credentials failed")
	ErrBuildClient  = errors.New("llmclient: build client failed")
)

// Bundle is the resolved per-request LLM bundle.
//
// Bundle 是单次请求解析后的 LLM 打包。
type Bundle struct {
	Client   llminfra.Client
	APIKeyID string // which api_key was used
	Provider string // derived from credentials.Provider, for logging
	ModelID  string
	Key      string
	BaseURL  string
}

// ResolveDialogueWithOverride resolves the dialogue-scenario LLM. If override
// is non-nil with both fields set, it wins; else falls back to picker.PickForDialogue.
// Used by chat main loop and subagent spawn.
//
// ResolveDialogueWithOverride 解析 dialogue scenario LLM。override 双字段齐时直接用,
// 否则 fallback 到 picker.PickForDialogue。chat 主循环和 subagent spawn 共用。
func ResolveDialogueWithOverride(
	ctx context.Context,
	override *modeldomain.ModelRef,
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
) (*Bundle, error) {
	if override != nil && override.APIKeyID != "" && override.ModelID != "" {
		return finishResolve(ctx, override.APIKeyID, override.ModelID, keys, factory)
	}
	apiKeyID, modelID, err := picker.PickForDialogue(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPickModel, err)
	}
	return finishResolve(ctx, apiKeyID, modelID, keys, factory)
}

// ResolveUtility resolves the utility-scenario LLM. No override — utility is
// tool-internal LLM work, not user-facing conversation.
//
// ResolveUtility 解析 utility scenario LLM。无 override —— utility 是 tool 内部
// LLM 活儿(autoTitle / compaction / rerank / env-fix / web 摘要),不参与 conv 选择。
func ResolveUtility(
	ctx context.Context,
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
) (*Bundle, error) {
	apiKeyID, modelID, err := picker.PickForUtility(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPickModel, err)
	}
	return finishResolve(ctx, apiKeyID, modelID, keys, factory)
}

// ResolveAgentWithOverride resolves the agent-scenario LLM. If override is
// non-nil with both fields set, it wins; else falls back to picker.PickForAgent.
// Used by workflow agent/llm node dispatchers (override = node.ModelOverride).
//
// ResolveAgentWithOverride 解析 agent scenario LLM。override 双字段齐时直接用,
// 否则 fallback 到 picker.PickForAgent。workflow agent/llm 节点 dispatcher 共用
// (override = node.ModelOverride)。
func ResolveAgentWithOverride(
	ctx context.Context,
	override *modeldomain.ModelRef,
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
) (*Bundle, error) {
	if override != nil && override.APIKeyID != "" && override.ModelID != "" {
		return finishResolve(ctx, override.APIKeyID, override.ModelID, keys, factory)
	}
	apiKeyID, modelID, err := picker.PickForAgent(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPickModel, err)
	}
	return finishResolve(ctx, apiKeyID, modelID, keys, factory)
}

// finishResolve looks up creds by api_key id (not provider), so multi-key-per-provider
// scenarios route to the exact key the user picked.
//
// finishResolve 按 api_key id 查 creds(不按 provider),保证多 key 同 provider
// 场景下精确落到用户选的那把。
func finishResolve(
	ctx context.Context,
	apiKeyID, modelID string,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
) (*Bundle, error) {
	creds, err := keys.ResolveCredentialsByID(ctx, apiKeyID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrResolveCreds, err)
	}
	client, baseURL, err := factory.Build(llminfra.Config{
		Provider: creds.Provider,
		ModelID:  modelID,
		Key:      creds.Key,
		BaseURL:  creds.BaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildClient, err)
	}
	return &Bundle{
		Client:   client,
		APIKeyID: apiKeyID,
		Provider: creds.Provider,
		ModelID:  modelID,
		Key:      creds.Key,
		BaseURL:  baseURL,
	}, nil
}
