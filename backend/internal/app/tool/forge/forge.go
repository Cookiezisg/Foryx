// Package forge provides the 5 system tools the LLM uses to interact with
// the user's forge library: search_forges, get_forge, create_forge,
// edit_forge, run_forge.
//
// Imported as `forgetool` per §S13 nested sub-package alias rule
// (`<sub><parent>` = forge + tool). Distinguish from `forgeapp` which is the
// app/forge service itself.
//
// Package forge 提供 5 个 system tool，让 LLM 与用户的 forge 库交互：
// search_forges / get_forge / create_forge / edit_forge / run_forge。
//
// 调用方按 §S13 嵌套子包别名规则导入 `forgetool`（`<子名><父名>` = forge + tool）。
// 区别于 `forgeapp`——后者是 app/forge service 本身。
package forge

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	forgeapp "github.com/sunweilin/forgify/backend/internal/app/forge"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	llmclientpkg "github.com/sunweilin/forgify/backend/internal/pkg/llmclient"
)

// ── ForgeTools factory ────────────────────────────────────────────────────────

// ForgeTools constructs the 5 forge system tools wired with their dependencies.
// Returns []toolapp.Tool because the chat ReAct loop consumes the abstract
// Tool interface; the concrete struct types (SearchForge, GetForge, etc.) are
// implementation details. Forge entity-state SSE events are published through
// svc.PublishSnapshot — bridge wiring lives on the Service, not each tool.
//
// ForgeTools 构造装配好依赖的 5 个 forge system tool。
// 返回 []toolapp.Tool——chat ReAct 循环消费的是抽象 Tool 接口；具体类型
// （SearchForge / GetForge 等）是实现细节。Forge entity-state SSE 事件
// 通过 svc.PublishSnapshot 发布——bridge 装在 Service 上，不再传给每个工具。
func ForgeTools(
	svc *forgeapp.Service,
	attachRepo chatdomain.Repository,
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
	log *zap.Logger,
) []toolapp.Tool {
	return []toolapp.Tool{
		&SearchForge{svc: svc, picker: picker, keys: keys, factory: factory, log: log},
		&GetForge{svc: svc},
		&CreateForge{svc: svc, picker: picker, keys: keys, factory: factory},
		&EditForge{svc: svc, picker: picker, keys: keys, factory: factory},
		&RunForge{svc: svc, attachRepo: attachRepo},
	}
}

// ── resolveAttachments ────────────────────────────────────────────────────────

// resolveAttachments walks top-level string fields in input and rewrites any
// "att_xxx" value to the attachment's storage path on disk. Non-string fields
// and strings without the att_ prefix are passed through unchanged.
//
// Limitation: only top-level string fields are inspected. Nested lists or
// maps containing att_xxx values are NOT expanded. Forge functions are
// expected to take a flat input shape (file_path: "att_xxx").
//
// resolveAttachments 遍历 input 的顶层 string 字段，把 "att_xxx" 重写成附件
// 在磁盘上的存储路径。非 string 字段或不带 att_ 前缀的 string 原样透传。
//
// 限制：仅检查顶层 string 字段。嵌套 list/map 中的 att_xxx 不展开。
// Forge 函数预期接收扁平 input（如 file_path: "att_xxx"）。
func resolveAttachments(ctx context.Context, repo chatdomain.Repository, input map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(input))
	for k, v := range input {
		s, ok := v.(string)
		if !ok || !strings.HasPrefix(s, "att_") {
			out[k] = v
			continue
		}
		att, err := repo.GetAttachment(ctx, s)
		if err != nil {
			return nil, fmt.Errorf("resolveAttachments: %w", err)
		}
		out[k] = att.StoragePath
	}
	return out, nil
}

// ── Streaming code generation ─────────────────────────────────────────────────

// streamCode calls the LLM to generate Python code, invoking onChunk after
// each text token with the fence-stripped accumulated code so the caller
// can publish snapshot events. Returns the final cleanly extracted code.
//
// onChunk receives the in-progress code (markdown fences best-effort stripped
// for live rendering); the returned code is the final post-stream stripped
// version. onChunk may be nil for non-streaming use.
//
// streamCode 调 LLM 生成 Python 代码，每个文本 token 后调用 onChunk（携带
// 已 trim fence 的累积代码），让调用方据此推快照。返回最终剥除 fence 的代码。
//
// onChunk 接收实时（已尽力剥 fence）代码；返回值是完整流结束后的最终结果。
// 不需要流式时可传 nil。
func streamCode(
	ctx context.Context,
	prompt string,
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
	onChunk func(accumulated string),
) (string, error) {
	bc, err := llmclientpkg.Resolve(ctx, picker, keys, factory)
	if err != nil {
		return "", fmt.Errorf("streamCode: %w", err)
	}

	req := llminfra.Request{
		ModelID:  bc.ModelID,
		Key:      bc.Key,
		BaseURL:  bc.BaseURL,
		Messages: []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: prompt}},
	}

	var buf strings.Builder
	for event := range bc.Client.Stream(ctx, req) {
		switch event.Type {
		case llminfra.EventText:
			buf.WriteString(event.Delta)
			if onChunk != nil {
				onChunk(extractCode(buf.String()))
			}
		case llminfra.EventError:
			return "", fmt.Errorf("streamCode: %w", event.Err)
		}
	}
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("streamCode: %w", err)
	}
	return extractCode(buf.String()), nil
}

// ── Code generation prompts ───────────────────────────────────────────────────

func buildCreatePrompt(name, description, instruction string) string {
	return fmt.Sprintf(`Write a Python function named %q.

Description: %s
Instruction: %s

Requirements:
- Single function with type annotations
- Google-style docstring with Args: and Returns: sections
- Return value must be JSON-serializable (str, int, float, bool, list, dict)
- Only output the function definition, no main block, no explanation

Output only the Python code.`, name, description, instruction)
}

func buildEditPrompt(currentCode, instruction string) string {
	return fmt.Sprintf(`Modify the following Python function according to the instruction.

Current code:
%s

Instruction: %s

Requirements:
- Keep it a single function with type annotations
- Maintain Google-style docstring
- Return value must be JSON-serializable
- Output only the complete modified function, no explanation

Output only the Python code.`, currentCode, instruction)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// extractCode strips markdown code fences (```python ... ``` etc.) from a raw
// LLM response. If no fence is present, returns the trimmed input unchanged.
//
// extractCode 剥除 LLM 响应中的 markdown 代码 fence（如 ```python ... ```）。
// 不含 fence 时原样返回 trim 后的输入。
func extractCode(raw string) string {
	raw = strings.TrimSpace(raw)
	for _, fence := range []string{"```python\n", "```\n", "```python", "```"} {
		if after, ok := strings.CutPrefix(raw, fence); ok {
			raw = after
			if idx := strings.LastIndex(raw, "```"); idx >= 0 {
				raw = raw[:idx]
			}
			return strings.TrimSpace(raw)
		}
	}
	return raw
}
