package tool

import (
	"encoding/json"
	"fmt"

	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	jsonrepairpkg "github.com/sunweilin/forgify/backend/internal/pkg/jsonrepair"
)

// The three framework-injected fields present on every tool call. Tools never declare
// them; ToLLMDef adds them to the schema and StripStandardFields removes them from args.
//
// 每个工具调用上 framework 注入的三个字段。工具从不声明；ToLLMDef 加进 schema、
// StripStandardFields 从 args 移除。
const (
	fieldSummary        = "summary"
	fieldDanger         = "danger"
	fieldExecutionGroup = "execution_group"
)

// StandardFields is the parsed form of the three injected fields, pulled off a tool
// call's args before Execute. Summary is shown to the user; Danger gates execution;
// ExecutionGroup batches parallel calls.
//
// StandardFields 是三个注入字段的解析结果，在 Execute 前从工具调用 args 摘下。Summary 给用户看；
// Danger 把守执行；ExecutionGroup 给并行调用分批。
type StandardFields struct {
	Summary        string
	Danger         DangerLevel
	ExecutionGroup int
}

// ToLLMDef converts a Tool to an llm.ToolDef, injecting the standard fields into the
// schema the LLM sees. The tool's own Parameters() is left untouched.
//
// ToLLMDef 把 Tool 转成 llm.ToolDef，把标准字段注入 LLM 所见 schema。工具自身 Parameters() 不动。
func ToLLMDef(t Tool) llminfra.ToolDef {
	return llminfra.ToolDef{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  injectStandardFields(t.Parameters()),
	}
}

// ToLLMDefs batch-converts tools.
//
// ToLLMDefs 批量转换。
func ToLLMDefs(tools []Tool) []llminfra.ToolDef {
	defs := make([]llminfra.ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = ToLLMDef(t)
	}
	return defs
}

// injectStandardFields adds summary / danger / execution_group to a tool's schema and
// makes summary + danger required (so the LLM declares both on every call). A tool whose
// own schema already names one of the three is a programming error → panic (caught at
// startup, never silently shadowed).
//
// injectStandardFields 给工具 schema 加 summary / danger / execution_group，并把 summary +
// danger 设为必填（使 LLM 每次都声明两者）。工具自身 schema 已占用三者之一 = 编程错误 → panic
// （启动期暴露，绝不静默覆盖）。
func injectStandardFields(params json.RawMessage) json.RawMessage {
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(params, &schema); err != nil {
		panic(fmt.Sprintf("tool: parameters are not a valid JSON object: %v", err))
	}

	props := map[string]json.RawMessage{}
	if raw, ok := schema["properties"]; ok {
		if err := json.Unmarshal(raw, &props); err != nil {
			panic(fmt.Sprintf("tool: parameters.properties is not a valid JSON object: %v", err))
		}
	}
	for _, f := range [...]string{fieldSummary, fieldDanger, fieldExecutionGroup} {
		if _, conflict := props[f]; conflict {
			panic(fmt.Sprintf("tool: parameters already contain %q; rename to avoid conflict", f))
		}
	}
	props[fieldSummary] = json.RawMessage(`{"type":"string","description":"One sentence: what you're doing and why."}`)
	props[fieldDanger] = json.RawMessage(`{"type":"string","enum":["safe","cautious","dangerous"],"description":"Risk of THIS call: safe=read-only or reversible; cautious=modifies recoverable state; dangerous=irreversible or external write (waits for user approval). Estimate conservatively."}`)
	props[fieldExecutionGroup] = json.RawMessage(`{"type":"integer","minimum":1,"description":"Parallel-batch id: calls sharing a group run together; groups run in order."}`)

	propsRaw, err := json.Marshal(props)
	if err != nil {
		return params
	}
	schema["properties"] = propsRaw

	// summary + danger lead required so the LLM always outputs them; a malformed existing
	// required is an author error → panic rather than silently drop their fields.
	//
	// summary + danger 排在 required 首位，使 LLM 必出；既有 required 畸形 = 作者错 → panic 而非静默丢字段。
	var required []string
	if raw, ok := schema["required"]; ok {
		if err := json.Unmarshal(raw, &required); err != nil {
			panic(fmt.Sprintf("tool: parameters.required is not a valid JSON array of strings: %v", err))
		}
	}
	required = append([]string{fieldSummary, fieldDanger}, required...)
	reqRaw, err := json.Marshal(required)
	if err != nil {
		return params
	}
	schema["required"] = reqRaw

	out, err := json.Marshal(schema)
	if err != nil {
		return params
	}
	return out
}

// StripStandardFields pulls summary / danger / execution_group off a tool call's raw args
// and returns the business-only args for Execute. It repairs first: LLMs emit ~4-8%
// malformed JSON (literal control chars, missing brackets) and repair recovers it; an
// unparseable body is returned verbatim with zero-value fields (ValidateInput then
// surfaces the real error to the LLM). A missing or invalid danger defaults to safe.
//
// StripStandardFields 从工具调用原始 args 摘下 summary / danger / execution_group，返回只含业务
// 参数的 args 给 Execute。先 repair：LLM ~4-8% 吐畸形 JSON（裸控制符、缺括号），repair 回收；
// 无法解析则原样返回、字段取零值（再由 ValidateInput 把真错反馈 LLM）。danger 缺失或非法 → safe。
func StripStandardFields(argsJSON string) (StandardFields, string) {
	fields := StandardFields{Danger: DangerSafe}
	argsJSON = jsonrepairpkg.Repair(argsJSON)
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return fields, argsJSON
	}
	if raw, ok := m[fieldSummary]; ok {
		_ = json.Unmarshal(raw, &fields.Summary)
		delete(m, fieldSummary)
	}
	if raw, ok := m[fieldDanger]; ok {
		var d string
		_ = json.Unmarshal(raw, &d)
		if IsValidDanger(d) {
			fields.Danger = DangerLevel(d)
		}
		delete(m, fieldDanger)
	}
	if raw, ok := m[fieldExecutionGroup]; ok {
		_ = json.Unmarshal(raw, &fields.ExecutionGroup)
		if fields.ExecutionGroup < 0 {
			fields.ExecutionGroup = 0
		}
		delete(m, fieldExecutionGroup)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return fields, argsJSON
	}
	return fields, string(b)
}
