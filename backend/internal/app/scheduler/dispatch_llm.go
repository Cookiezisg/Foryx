package scheduler

import (
	"context"
	"encoding/json"
	"fmt"

	documentapp "github.com/sunweilin/forgify/backend/internal/app/document"
	documentdomain "github.com/sunweilin/forgify/backend/internal/domain/document"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
)

// LLMCaller is the port LLMDispatcher consumes; main.go wires the concrete impl.
//
// LLMCaller 是 LLMDispatcher 消费的端口；main.go 装配具体实现。
type LLMCaller interface {
	// Generate calls the LLM with optional per-node ModelOverride + prompt + vars and returns the text.
	//
	// Generate 按节点级 ModelOverride 调 LLM(override 为 nil 时走 agent 默认),返生成文本。
	Generate(ctx context.Context, override *modeldomain.ModelRef, prompt string, vars map[string]any) (string, error)
}

// DocumentResolver is the port LLMDispatcher + AgentDispatcher consume to
// expand AttachedDocuments into full Documents at dispatch time
// (documentapp.Service satisfies). nil → no doc prepend, still functional.
//
// DocumentResolver 是 LLMDispatcher + AgentDispatcher 在 dispatch 时
// 把 AttachedDocuments 展开为完整 Document 的端口(documentapp.Service 实现);
// nil 时不前置 docs 段,其他逻辑不变。
type DocumentResolver interface {
	ResolveAttached(ctx context.Context, atts []documentdomain.AttachedDocument) ([]*documentdomain.Document, error)
}

// LLMDispatcher bridges workflow llm nodes to the LLMCaller port + optional
// document attach (knowledge-base-style prepend to the prompt).
//
// LLMDispatcher 把 workflow llm 节点桥接到 LLMCaller + 可选的文档挂载
// (知识库式 prompt 前缀)。
type LLMDispatcher struct {
	caller    LLMCaller
	documents DocumentResolver
}

// NewLLMDispatcher constructs LLMDispatcher; nil caller errors on dispatch,
// nil documents simply skips the docs prefix.
//
// NewLLMDispatcher 构造 LLMDispatcher；nil caller 时 dispatch 返错；
// nil documents 跳过 docs 前缀。
func NewLLMDispatcher(caller LLMCaller, documents DocumentResolver) *LLMDispatcher {
	return &LLMDispatcher{caller: caller, documents: documents}
}

// Dispatch reads prompt + attachedDocuments from node.Config and invokes the LLM.
//
// Dispatch 读 prompt + attachedDocuments,调 LLM。
func (d *LLMDispatcher) Dispatch(ctx context.Context, in DispatchInput) DispatchOutput {
	if d.caller == nil {
		return DispatchOutput{Error: fmt.Errorf("llm node %q: no LLMCaller wired", in.Node.ID)}
	}
	prompt, _ := in.Node.Config["prompt"].(string)
	if prompt == "" {
		return DispatchOutput{Error: fmt.Errorf("llm node %q: prompt required", in.Node.ID)}
	}

	atts, err := parseAttachedDocuments(in.Node.Config)
	if err != nil {
		return DispatchOutput{Error: fmt.Errorf("llm node %q: %w", in.Node.ID, err)}
	}
	if d.documents != nil && len(atts) > 0 {
		docs, err := d.documents.ResolveAttached(ctx, atts)
		if err != nil {
			return DispatchOutput{Error: fmt.Errorf("llm node %q: resolve attached: %w", in.Node.ID, err)}
		}
		if prefix := documentapp.RenderAttachedAsXML(docs); prefix != "" {
			prompt = prefix + "\n" + prompt
		}
	}

	// node.ModelOverride wired in Task 11; stub nil for now.
	//
	// node.ModelOverride 由 Task 11 接入,本任务先 nil 占位。
	var nodeModelOverride *modeldomain.ModelRef
	out, err := d.caller.Generate(ctx, nodeModelOverride, prompt, in.ExecCtx.Variables)
	if err != nil {
		return DispatchOutput{Error: err}
	}
	return DispatchOutput{Outputs: map[string]any{"out": out}}
}

// parseAttachedDocuments extracts and normalises the optional attachedDocuments
// field from a node config map. Accepts either the canonical struct shape or
// JSON-decoded map form (since node config is loaded as map[string]any).
//
// parseAttachedDocuments 从 node config 抽 attachedDocuments 字段;
// 兼容 struct 与 map (config 一般经 JSON 解到 map[string]any)。
func parseAttachedDocuments(cfg map[string]any) ([]documentdomain.AttachedDocument, error) {
	raw, ok := cfg["attachedDocuments"]
	if !ok || raw == nil {
		return nil, nil
	}
	// Already a typed slice (workflow validate may have promoted).
	if typed, ok := raw.([]documentdomain.AttachedDocument); ok {
		return typed, nil
	}
	// Fallback: serialize then deserialize through JSON.
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("attachedDocuments: marshal: %w", err)
	}
	var out []documentdomain.AttachedDocument
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, fmt.Errorf("attachedDocuments: unmarshal: %w", err)
	}
	return out, nil
}
