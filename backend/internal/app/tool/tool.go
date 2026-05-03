// Package tool defines the Tool interface that every system tool implements
// and the framework-level machinery for LLM tool-call handling.
//
// Architecture:
//
//   - Tool interface: 10 required methods covering identity, static metadata,
//     args-dependent hooks, and main execution.
//   - Standard injected fields: every tool's Parameters schema is augmented
//     with two LLM-facing fields, "summary" (required string) and
//     "destructive" (optional bool). The framework strips them before passing
//     args to Execute. They are stored as first-class fields on ToolCallData.
//   - Sub-packages by tool family: tool/forge/ for user-forged-tool tools,
//     plus future tool/filesystem/, tool/shell/, tool/web/ (Phase 5).
//     §S12 example position; alias `<sub><parent>` per §S13.
//
// Package tool 定义每个 system tool 必须实现的 Tool 接口及框架层 LLM 工具调用处理设施。
//
// 架构：
//   - Tool 接口：10 个必须方法，涵盖 identity / 静态元数据 / args-dependent 钩子 / 主入口
//   - 标准注入字段：每个 tool 的 Parameters schema 自动加上两个 LLM-facing 字段——
//     "summary"（必填 string）和 "destructive"（可选 bool）。框架在传给 Execute 前剥除。
//     二者作为 ToolCallData 的一等字段独立存储。
//   - 按 tool 家族分子包：tool/forge/、tool/filesystem/、tool/shell/、tool/web/
//     （§S12 例外位置，§S13 别名规则 `<sub><parent>`）
package tool

import (
	"context"
	"encoding/json"
	"fmt"

	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// ── Permission types ──────────────────────────────────────────────────────────

// PermissionMode is the agent's current permission mode for a turn.
// Phase 3 (current): only PermissionModeDefault is wired; runTools always
// passes Default. Reserved for Phase 4+ workflow scheduler / acceptEdits UI.
//
// PermissionMode 是 agent 当前回合的权限模式。
// Phase 3（当下）只串通 PermissionModeDefault；runTools 一律传 Default。
// 保留接口位给 Phase 4+ workflow scheduler / acceptEdits UI 用。
type PermissionMode string

const (
	PermissionModeDefault     PermissionMode = "default"
	PermissionModeAcceptEdits PermissionMode = "acceptEdits"
	PermissionModePlan        PermissionMode = "plan"
	PermissionModeBypass      PermissionMode = "bypass"
)

// PermissionResult is what CheckPermissions returns.
//
// PermissionResult 是 CheckPermissions 的返回值。
type PermissionResult int

const (
	PermissionAllow PermissionResult = iota // 允许执行
	PermissionDeny                          // 拒绝执行（返错给 LLM）
	PermissionAsk                           // 问用户（Phase 4+，当前等价 Allow）
)

// ── Tool interface ────────────────────────────────────────────────────────────

// Tool is the contract every system tool must implement.
// All 10 methods are required — there is no BaseTool to inherit defaults from.
// This is intentional: each tool's metadata is explicit and greppable.
//
// Tool 是每个 system tool 必须实现的契约。
// 10 个方法全部必须实现——不通过 BaseTool 提供默认。这是有意为之：
// 每个 tool 的元数据都显式且可 grep。
type Tool interface {
	// ── Identity ──────────────────────────────────────────────────────────

	// Name returns the LLM-facing tool name (e.g. "search_forges").
	// Name 返回 LLM 看到的工具名（如 "search_forges"）。
	Name() string

	// Description tells the LLM what the tool does and when to use it.
	// Description 告诉 LLM 工具的作用和何时使用。
	Description() string

	// Parameters returns the JSON Schema describing the tool's input shape.
	// MUST NOT include "summary" or "destructive" — the framework injects them.
	//
	// Parameters 返回描述工具输入的 JSON Schema。
	// 不得包含 "summary" 或 "destructive"——框架自动注入。
	Parameters() json.RawMessage

	// ── Static metadata: properties of the tool itself ────────────────────

	// IsReadOnly reports whether this tool only reads state (no side effects).
	// True → safe to run concurrently with other read-only tools.
	//
	// IsReadOnly 报告本 tool 是否纯读（无副作用）。
	// true → 可与其他 read-only tool 并发。
	IsReadOnly() bool

	// NeedsReadFirst reports whether the file this tool operates on must have
	// been Read in this session before the tool can be invoked. Phase 5 Edit/Write.
	//
	// NeedsReadFirst 报告本 tool 操作的文件是否必须在 session 内被 Read 过。
	// Phase 5 Edit/Write 用。
	NeedsReadFirst() bool

	// RequiresWorkspace reports whether the tool's cwd must be inside the
	// user-configured workspace whitelist. Phase 5 Bash/Edit/Write.
	//
	// RequiresWorkspace 报告本 tool 的 cwd 是否必须在用户 workspace 白名单内。
	// Phase 5 Bash/Edit/Write 用。
	RequiresWorkspace() bool

	// ── Args-dependent hooks ──────────────────────────────────────────────

	// IsConcurrencySafe decides if a specific call can run in parallel with
	// other calls. For most tools this matches IsReadOnly(); for Bash and
	// similar it depends on the actual command.
	//
	// IsConcurrencySafe 决定此次具体调用能否与其他并行。多数 tool 与 IsReadOnly() 一致；
	// Bash 这种 args 决定（"ls" 安全 / "rm" 不安全）。
	IsConcurrencySafe(args json.RawMessage) bool

	// ValidateInput performs pre-Execute parameter validation. Return nil if
	// input is valid; an error halts the call before Execute (the error text
	// becomes the tool_result, fed back to the LLM).
	//
	// ValidateInput 在 Execute 前做参数级校验。返回 nil 表示通过；
	// 返错则不进 Execute，错误文本作为 tool_result 喂回 LLM。
	ValidateInput(args json.RawMessage) error

	// CheckPermissions decides if a call is allowed under the current mode.
	// Returns Allow / Deny / Ask. Forgify Phase 3 always passes mode=Default
	// and most tools return Allow; reserved for Phase 4+ workflow scheduler.
	//
	// CheckPermissions 决定当前 mode 下是否允许调用。返回 Allow / Deny / Ask。
	// Forgify Phase 3 一律传 mode=Default，多数 tool 返 Allow；
	// 保留位给 Phase 4+ workflow scheduler。
	CheckPermissions(args json.RawMessage, mode PermissionMode) PermissionResult

	// ── Main entry ────────────────────────────────────────────────────────

	// Execute runs the tool with stripped args (no "summary" / "destructive").
	// Returns the result string (fed back to LLM as tool_result) and an error.
	// If err != nil, the framework converts it to a failure tool_result.
	//
	// Execute 用剥除标准字段（"summary" / "destructive"）的 args 执行。
	// 返回结果字符串（作为 tool_result 喂回 LLM）和 error。
	// err != nil 时框架转成失败 tool_result。
	Execute(ctx context.Context, argsJSON string) (string, error)
}

// ── LLM def conversion ────────────────────────────────────────────────────────

// ToLLMDef converts a Tool to the ToolDef sent to the LLM, automatically
// injecting "summary" and "destructive" fields into the Parameters schema.
//
// ToLLMDef 把 Tool 转成发给 LLM 的 ToolDef，自动注入 "summary" 和 "destructive" 字段。
func ToLLMDef(t Tool) llminfra.ToolDef {
	return llminfra.ToolDef{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  injectStandardFields(t.Parameters()),
	}
}

// ToLLMDefs batch-converts a slice of Tools to ToolDefs.
//
// ToLLMDefs 批量转换 Tool 为 ToolDef。
func ToLLMDefs(tools []Tool) []llminfra.ToolDef {
	defs := make([]llminfra.ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = ToLLMDef(t)
	}
	return defs
}

// ── injectStandardFields ──────────────────────────────────────────────────────

// injectStandardFields adds "summary" (required string) and "destructive"
// (optional bool, default false) to the tool's Parameters schema.
// If either field name is already present (implementation bug), it panics
// to fail fast at development time.
//
// injectStandardFields 向 tool 参数 schema 注入 "summary"（必填 string）
// 和 "destructive"（可选 bool，默认 false）。
// 若任一字段名已被占用（实现 bug）直接 panic——开发期快速失败。
func injectStandardFields(params json.RawMessage) json.RawMessage {
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(params, &schema); err != nil {
		panic(fmt.Sprintf("tool: parameters are not a valid JSON object: %v", err))
	}

	var props map[string]json.RawMessage
	if raw, ok := schema["properties"]; ok {
		if err := json.Unmarshal(raw, &props); err != nil {
			panic(fmt.Sprintf("tool: parameters.properties is not a valid JSON object: %v", err))
		}
		if _, conflict := props["summary"]; conflict {
			panic("tool: parameters already contain 'summary' field; rename to avoid conflict")
		}
		if _, conflict := props["destructive"]; conflict {
			panic("tool: parameters already contain 'destructive' field; rename to avoid conflict")
		}
	} else {
		props = map[string]json.RawMessage{}
	}

	props["summary"] = json.RawMessage(`{
		"type": "string",
		"description": "One sentence describing what you are doing and why. Required."
	}`)
	props["destructive"] = json.RawMessage(`{
		"type": "boolean",
		"description": "Set to true if this call may cause irreversible damage (rm -rf, DELETE FROM, git push --force, deleting forges, running forges that modify external state, etc.). The user will see a warning when true. Default false.",
		"default": false
	}`)

	propsRaw, err := json.Marshal(props)
	if err != nil {
		return params
	}
	schema["properties"] = propsRaw

	// Prepend "summary" to required so most LLMs output it first.
	// "destructive" stays optional (default false handles it).
	// Silent-parse of an existing malformed `required` would drop the tool
	// author's required field list and let the LLM skip required args —
	// match the surrounding panic-on-bad-schema policy at line 191/196/200.
	//
	// "summary" 排在 required 首位引导 LLM 优先输出；"destructive" 不必填，
	// 缺省 false。静默解析坏掉的现有 `required` 会丢失工具作者的必填字段表，
	// 让 LLM 跳过必填项——与 191/196/200 行的 panic-on-bad-schema 策略保持一致。
	var required []string
	if raw, ok := schema["required"]; ok {
		if err := json.Unmarshal(raw, &required); err != nil {
			panic(fmt.Sprintf("tool: parameters.required is not a valid JSON array of strings: %v", err))
		}
	}
	required = append([]string{"summary"}, required...)
	reqRaw, err := json.Marshal(required)
	if err != nil {
		return params
	}
	schema["required"] = reqRaw

	result, err := json.Marshal(schema)
	if err != nil {
		return params
	}
	return result
}

// ── StripStandardFields ───────────────────────────────────────────────────────

// StripStandardFields extracts "summary" and "destructive" from argsJSON and
// returns them along with the JSON with both fields removed.
// Missing fields default to zero values (empty summary, false destructive).
// Invalid JSON returns zero values and the original argsJSON unchanged.
//
// StripStandardFields 从 argsJSON 中提取 "summary" 和 "destructive"，
// 返回二者和剥除后的 JSON。字段缺失则取零值（summary 空、destructive false）。
// JSON 不合法时返回零值和原始 argsJSON。
func StripStandardFields(argsJSON string) (summary string, destructive bool, strippedJSON string) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return "", false, argsJSON
	}
	// LLM-produced args: if the LLM emits the wrong type (e.g. summary as
	// an int) the field stays zero. We deliberately don't return / log
	// here — the tool's ValidateInput will reject the malformed call with
	// a retry signal that propagates back to the LLM, which IS the surface.
	//
	// LLM 产出的 args：类型不对（如 summary 给成 int）字段保持零值。
	// 此处刻意不返错 / 不打日志——下游 tool 的 ValidateInput 会拒绝并以
	// 重试信号回到 LLM，那才是真正的暴露面。
	if raw, ok := m["summary"]; ok {
		_ = json.Unmarshal(raw, &summary)
		delete(m, "summary")
	}
	if raw, ok := m["destructive"]; ok {
		_ = json.Unmarshal(raw, &destructive)
		delete(m, "destructive")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return summary, destructive, argsJSON
	}
	return summary, destructive, string(b)
}
