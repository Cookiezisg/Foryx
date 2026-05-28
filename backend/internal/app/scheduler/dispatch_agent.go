package scheduler

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
	loopapp "github.com/sunweilin/forgify/backend/internal/app/loop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
)

// AgentDispatcher runs the workflow `agent` node — an agentic ReAct loop
// (multi-turn, full system-tool registry) wrapped around app/loop.Run.
// Distinct from LLMDispatcher which is single-shot non-streaming.
//
// AgentDispatcher 跑 workflow `agent` 节点——基于 app/loop.Run 的 agentic
// ReAct 循环(多轮 + 完整 system tool 注入);跟 LLMDispatcher 单次非流式区分。
type AgentDispatcher struct {
	picker    modeldomain.ModelPicker
	keys      apikeydomain.KeyProvider
	factory   *llminfra.Factory
	documents DocumentResolver
	toolsFn   func() []toolapp.Tool
	log       *zap.Logger
}

// NewAgentDispatcher wires deps. nil picker/keys/factory → dispatch errs;
// nil documents simply skips attach prefix; toolsFn returns the tool slice
// at dispatch time (so registrations that append AFTER wire-up still take
// effect — common when D22 read-only tools are added later in main.go boot).
//
// NewAgentDispatcher 装配依赖。nil picker/keys/factory 时 dispatch 返错;
// nil documents 跳过 attach 前缀;toolsFn 在 dispatch 时读取(支持装配后
// append——main.go boot 末尾追加 D22 只读工具是常见情况)。
func NewAgentDispatcher(
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
	documents DocumentResolver,
	toolsFn func() []toolapp.Tool,
	log *zap.Logger,
) *AgentDispatcher {
	if log == nil {
		log = zap.NewNop()
	}
	return &AgentDispatcher{
		picker:    picker,
		keys:      keys,
		factory:   factory,
		documents: documents,
		toolsFn:   toolsFn,
		log:       log,
	}
}

// agentMaxTurnsDefault caps agent dispatch to keep cost bounded; config can
// override via maxTurns (clamped to [1, agentMaxTurnsHardLimit]).
//
// agentMaxTurnsDefault 默认轮次上限,config maxTurns 可覆盖(钳到 [1, hard])。
const (
	agentMaxTurnsDefault   = 10
	agentMaxTurnsHardLimit = 50
)

func (d *AgentDispatcher) Dispatch(ctx context.Context, in DispatchInput) DispatchOutput {
	if d.picker == nil || d.keys == nil || d.factory == nil {
		return DispatchOutput{Error: fmt.Errorf("agent node %q: missing picker/keys/factory", in.Node.ID)}
	}

	cfg := in.Node.Config
	prompt, _ := cfg["prompt"].(string)
	if prompt == "" {
		return DispatchOutput{Error: fmt.Errorf("agent node %q: prompt required", in.Node.ID)}
	}

	maxTurns := agentMaxTurnsDefault
	if v, ok := cfg["maxTurns"]; ok {
		switch n := v.(type) {
		case int:
			maxTurns = n
		case float64:
			maxTurns = int(n)
		}
	}
	if maxTurns < 1 {
		maxTurns = 1
	}
	if maxTurns > agentMaxTurnsHardLimit {
		maxTurns = agentMaxTurnsHardLimit
	}

	atts, err := parseAttachedDocuments(cfg)
	if err != nil {
		return DispatchOutput{Error: fmt.Errorf("agent node %q: %w", in.Node.ID, err)}
	}
	docPrefix := ""
	if d.documents != nil && len(atts) > 0 {
		docs, err := d.documents.ResolveAttached(ctx, atts)
		if err != nil {
			return DispatchOutput{Error: fmt.Errorf("agent node %q: resolve attached: %w", in.Node.ID, err)}
		}
		docPrefix = documentapp.RenderAttachedAsXML(docs)
	}

	enabled, _ := parseEnabledTools(cfg)
	var allTools []toolapp.Tool
	if d.toolsFn != nil {
		allTools = d.toolsFn()
	}
	tools := filterToolsByWhitelist(allTools, enabled)

	// node.ModelOverride lets per-node override the agent scenario default;
	// Task 11 wires it from NodeSpec.ModelOverride. For now stub nil.
	//
	// node.ModelOverride 让每个节点单独 override agent scenario 默认;
	// Task 11 接入 NodeSpec.ModelOverride,本任务先 nil 占位。
	var nodeModelOverride *modeldomain.ModelRef // wired in Task 11
	bundle, err := llmclientpkg.ResolveAgentWithOverride(ctx, nodeModelOverride, d.picker, d.keys, d.factory)
	if err != nil {
		return DispatchOutput{Error: fmt.Errorf("agent node %q: resolve LLM: %w", in.Node.ID, err)}
	}

	host := &agentHost{
		userPrompt: docPrefix + prompt,
		tools:      tools,
		captured:   &agentResult{},
	}
	baseReq := llminfra.Request{
		ModelID: bundle.ModelID,
		Key:     bundle.Key,
		BaseURL: bundle.BaseURL,
		System:  "You are a workflow agent. Use available tools as needed; respond concisely when finished.",
	}
	result := loopapp.Run(ctx, host, bundle.Client, baseReq, maxTurns, d.log)

	if result.Status == chatdomain.StatusError {
		return DispatchOutput{Error: fmt.Errorf("agent node %q: agent loop error", in.Node.ID)}
	}

	return DispatchOutput{Outputs: map[string]any{
		"out":        result.LastMessage,
		"status":     result.Status,
		"stopReason": result.StopReason,
		"steps":      result.Steps,
		"tokensIn":   result.TokensIn,
		"tokensOut":  result.TokensOut,
	}}
}

// agentHost is the per-dispatch loop.Host: history is a single user message
// (the prompt), Tools come from the dispatcher's pre-filtered slice,
// WriteFinalize is no-op (workflow doesn't persist agent chat history).
//
// agentHost 是单次 dispatch 的 loop.Host:history 仅含一条 user message(prompt),
// Tools 取自 dispatcher 预过滤切片,WriteFinalize no-op(workflow 不持久化 agent 历史)。
type agentHost struct {
	userPrompt string
	tools      []toolapp.Tool
	captured   *agentResult
}

type agentResult struct {
	blocks []chatdomain.Block
	status string
}

func (h *agentHost) LoadHistory(_ context.Context) ([]llminfra.LLMMessage, error) {
	return []llminfra.LLMMessage{
		{Role: llminfra.RoleUser, Content: h.userPrompt},
	}, nil
}

// Tools ignores ctx: workflow agent dispatch uses a fixed pre-filtered slice
// (no lazy groups / activate_tools).
//
// Tools 忽略 ctx：workflow agent dispatch 用固定预过滤切片（无 lazy 组 / activate_tools）。
func (h *agentHost) Tools(_ context.Context) []toolapp.Tool {
	return h.tools
}

func (h *agentHost) WriteFinalize(_ context.Context, blocks []chatdomain.Block, status, _, _, _ string, _, _ int) {
	h.captured.blocks = blocks
	h.captured.status = status
}

// parseEnabledTools accepts a string slice naming whitelisted tool names.
// Empty / missing → no filter (all tools available).
//
// parseEnabledTools 解析白名单(string 切片)。空/缺失 → 不过滤(全部可用)。
func parseEnabledTools(cfg map[string]any) ([]string, error) {
	raw, ok := cfg["enabledTools"]
	if !ok || raw == nil {
		return nil, nil
	}
	if typed, ok := raw.([]string); ok {
		return typed, nil
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var out []string
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func filterToolsByWhitelist(all []toolapp.Tool, whitelist []string) []toolapp.Tool {
	if len(whitelist) == 0 {
		return all
	}
	allowed := make(map[string]bool, len(whitelist))
	for _, n := range whitelist {
		allowed[n] = true
	}
	out := make([]toolapp.Tool, 0, len(whitelist))
	for _, t := range all {
		if allowed[t.Name()] {
			out = append(out, t)
		}
	}
	return out
}
