// Package llmclient resolves the per-request LLM client via the canonical
// three-step dance shared by chat / forge / WebFetch callsites:
// picker.Pick* → keys.ResolveCredentials → factory.Build.
//
// Package llmclient 通过 chat / forge / WebFetch 共享的三段式
// (picker.Pick* → keys.ResolveCredentials → factory.Build) 解析 per-request LLM 客户端。
package llmclient

import (
	"context"
	"errors"
	"fmt"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// Step sentinels distinguish which resolve stage failed, for callers that
// surface different user-facing error codes per stage.
//
// Step sentinel 标记三段解析中的失败步骤，让调用方按步骤分发不同的用户可见错误码。
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
	Provider string
	ModelID  string
	Key      string
	BaseURL  string
}

// Resolve walks picker → keys → factory for the chat scenario.
//
// Resolve 按 chat 场景走 picker → keys → factory。
func Resolve(
	ctx context.Context,
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
) (*Bundle, error) {
	provider, modelID, err := picker.PickForChat(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPickModel, err)
	}
	return finishResolve(ctx, provider, modelID, keys, factory)
}

// ResolveForWebSummary resolves the WebFetch summary LLM. Tries the
// web_summary scenario first; falls back to chat when unconfigured so
// summarisation works out of the box.
//
// ResolveForWebSummary 解析 WebFetch 摘要用 LLM。先 web_summary 场景，
// 未配置则 fallback 到 chat，保证开箱即用。
func ResolveForWebSummary(
	ctx context.Context,
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
) (*Bundle, error) {
	provider, modelID, err := picker.PickForWebSummary(ctx)
	if errors.Is(err, modeldomain.ErrNotConfigured) {
		provider, modelID, err = picker.PickForChat(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPickModel, err)
	}
	return finishResolve(ctx, provider, modelID, keys, factory)
}

func finishResolve(
	ctx context.Context,
	provider, modelID string,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
) (*Bundle, error) {
	creds, err := keys.ResolveCredentials(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrResolveCreds, err)
	}
	client, baseURL, err := factory.Build(llminfra.Config{
		Provider: provider,
		ModelID:  modelID,
		Key:      creds.Key,
		BaseURL:  creds.BaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBuildClient, err)
	}
	return &Bundle{
		Client:   client,
		Provider: provider,
		ModelID:  modelID,
		Key:      creds.Key,
		BaseURL:  baseURL,
	}, nil
}
