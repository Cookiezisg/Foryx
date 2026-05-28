package scheduler

import (
	"context"
	"fmt"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
)

// DefaultLLMCaller implements LLMCaller by routing every Generate call
// through llmclient.ResolveAgentWithOverride + llminfra.Generate. The optional
// per-call override comes from NodeSpec.ModelOverride at the dispatch layer.
//
// DefaultLLMCaller 实现 LLMCaller:每次 Generate 走
// llmclient.ResolveAgentWithOverride + llminfra.Generate。可选 override 来自
// dispatch 层的 NodeSpec.ModelOverride。
type DefaultLLMCaller struct {
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
}

// NewDefaultLLMCaller wires the three resolver deps; any nil causes Generate to err.
//
// NewDefaultLLMCaller 装配 3 个解析依赖;任一 nil 时 Generate 返错。
func NewDefaultLLMCaller(
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
) *DefaultLLMCaller {
	return &DefaultLLMCaller{picker: picker, keys: keys, factory: factory}
}

// Generate runs one single-shot completion for a workflow `llm` node.
// override is the node-level ModelOverride (may be nil → uses agent scenario default).
//
// Generate 跑一次单步补全(workflow llm 节点)。override 是节点级 ModelOverride
// (可能 nil → 走 agent scenario 默认)。
func (a *DefaultLLMCaller) Generate(ctx context.Context, override *modeldomain.ModelRef, prompt string, _ map[string]any) (string, error) {
	if a.picker == nil || a.keys == nil || a.factory == nil {
		return "", fmt.Errorf("DefaultLLMCaller: missing picker/keys/factory")
	}
	bundle, err := llmclientpkg.ResolveAgentWithOverride(ctx, override, a.picker, a.keys, a.factory)
	if err != nil {
		return "", fmt.Errorf("DefaultLLMCaller.Generate: %w", err)
	}
	req := llminfra.Request{
		ModelID: bundle.ModelID,
		Key:     bundle.Key,
		BaseURL: bundle.BaseURL,
		System:  "You are a workflow LLM step. Respond concisely.",
		Messages: []llminfra.LLMMessage{
			{Role: llminfra.RoleUser, Content: prompt},
		},
	}
	out, err := llminfra.Generate(ctx, bundle.Client, req)
	if err != nil {
		return "", fmt.Errorf("DefaultLLMCaller.Generate: %w", err)
	}
	return out, nil
}
