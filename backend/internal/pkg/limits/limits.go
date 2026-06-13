// Package limits is the single source for user-tunable operational ceilings. Every field
// here HAS a real consumer reading it through Current() — fields without a consumer do
// not exist (the schema is a projection of reality, not an aspiration). Startup loads
// <dataDir>/settings.json via app/settings and swaps the source with SetProvider; a
// PATCH /api/v1/limits hot-swaps it again, so consumers see new values on the next read.
//
// Package limits 是用户可调运行上限的唯一来源。这里每个字段都**有真实消费方**经 Current()
// 读取——没有消费方的字段不存在（schema 是现实的投影、不是愿景）。启动时 app/settings 读
// <dataDir>/settings.json 并经 SetProvider 换源；PATCH /api/v1/limits 再热换，消费方下一次
// 读取即见新值。
package limits

// Limits mirrors the settings.json "limits" block. Zero value of any field means
// "use the Default()" — WithDefaults fills zeros.
//
// Limits 镜像 settings.json 的 "limits" 段。任一字段零值 = 用 Default()——WithDefaults 补零。
type Limits struct {
	Agent   AgentLimits   `json:"agent"`
	Context ContextLimits `json:"context"`
	Timeout TimeoutLimits `json:"timeout"`
	Tools   ToolLimits    `json:"tools"`
	Guards  GuardLimits   `json:"guards"`
}

type AgentLimits struct {
	// MaxSteps caps the chat ReAct loop (consumer: chatapp).
	// MaxSteps 限聊天 ReAct 循环（消费方：chatapp）。
	MaxSteps int `json:"maxSteps"`
	// InvokeMaxTurns is the default turn cap for one agent invocation — chat invoke_agent,
	// HTTP :invoke and workflow agent nodes alike; an explicit per-call MaxTurns overrides
	// (consumer: agentapp).
	// InvokeMaxTurns 是一次 agent 调用的默认轮数上限——chat invoke_agent、HTTP :invoke、
	// workflow agent 节点同用；调用级显式 MaxTurns 可覆盖（消费方：agentapp）。
	InvokeMaxTurns int `json:"invokeMaxTurns"`
}

type ContextLimits struct {
	// TriggerRatio: compact when the last turn's input tokens reach this fraction of the
	// input budget (consumer: contextmgr).
	// TriggerRatio：末回合 input token 达 input 预算此比例时压缩（消费方：contextmgr）。
	TriggerRatio float64 `json:"triggerRatio"`
}

type TimeoutLimits struct {
	// LLMIdleSec resets per streamed token; fires only on a dead connection (consumer: infra/llm).
	// LLMIdleSec 每个流式 token 重置；只在死连接时触发（消费方：infra/llm）。
	LLMIdleSec int `json:"llmIdleSec"`
	// MCPCallSec bounds one MCP tool call (consumer: mcpapp).
	// MCPCallSec 限一次 MCP 工具调用（消费方：mcpapp）。
	MCPCallSec int `json:"mcpCallSec"`
	// BashDefaultTimeoutSec is the bash tool's default when the LLM passes none (consumer: shell).
	// BashDefaultTimeoutSec 是 LLM 未传超时时 bash 工具的默认（消费方：shell）。
	BashDefaultTimeoutSec int `json:"bashDefaultTimeoutSec"`
}

type ToolLimits struct {
	// ReadDefaultLines is the Read tool's default page (consumer: filesystem read).
	// ReadDefaultLines 是 Read 工具默认页大小（消费方：filesystem read）。
	ReadDefaultLines int `json:"readDefaultLines"`
	// BashOutputCapKB caps captured bash output (consumer: shell).
	// BashOutputCapKB 限 bash 捕获输出（消费方：shell）。
	BashOutputCapKB int `json:"bashOutputCapKB"`
	// ToolResultCapKB caps any tool_result fed back to the LLM (consumer: loop).
	// ToolResultCapKB 限回喂 LLM 的任何 tool_result（消费方：loop）。
	ToolResultCapKB int `json:"toolResultCapKB"`
}

type GuardLimits struct {
	// AttachmentMaxMB caps one uploaded file (consumers: attachmentapp + upload handler).
	// AttachmentMaxMB 限单个上传文件（消费方：attachmentapp + 上传 handler）。
	AttachmentMaxMB int `json:"attachmentMaxMB"`
	// WebhookBodyMaxMB caps an inbound webhook body (consumer: infra/trigger/webhook).
	// WebhookBodyMaxMB 限入站 webhook body（消费方：infra/trigger/webhook）。
	WebhookBodyMaxMB int `json:"webhookBodyMaxMB"`
}

// Default returns the defaults — aligned with what the code actually enforced before
// limits was wired (the previous hardcoded constants), so wiring changed no behavior.
//
// Default 返默认值——与接线前代码实际执行的硬编码常量对齐，接线不改变任何行为。
func Default() Limits {
	return Limits{
		Agent:   AgentLimits{MaxSteps: 25, InvokeMaxTurns: 10},
		Context: ContextLimits{TriggerRatio: 0.80},
		Timeout: TimeoutLimits{
			LLMIdleSec:            150,
			MCPCallSec:            180,
			BashDefaultTimeoutSec: 120,
		},
		Tools: ToolLimits{
			ReadDefaultLines: 2000,
			BashOutputCapKB:  256,
			ToolResultCapKB:  256,
		},
		Guards: GuardLimits{AttachmentMaxMB: 50, WebhookBodyMaxMB: 10},
	}
}

// WithDefaults fills every zero field from Default() — settings parsing tolerance:
// a partial settings.json tunes only what it names.
//
// WithDefaults 用 Default() 补全零值字段——settings 解析容差：部分 settings.json 只调它点名的。
func WithDefaults(l Limits) Limits {
	d := Default()
	if l.Agent.MaxSteps == 0 {
		l.Agent.MaxSteps = d.Agent.MaxSteps
	}
	if l.Agent.InvokeMaxTurns == 0 {
		l.Agent.InvokeMaxTurns = d.Agent.InvokeMaxTurns
	}
	if l.Context.TriggerRatio == 0 {
		l.Context.TriggerRatio = d.Context.TriggerRatio
	}
	if l.Timeout.LLMIdleSec == 0 {
		l.Timeout.LLMIdleSec = d.Timeout.LLMIdleSec
	}
	if l.Timeout.MCPCallSec == 0 {
		l.Timeout.MCPCallSec = d.Timeout.MCPCallSec
	}
	if l.Timeout.BashDefaultTimeoutSec == 0 {
		l.Timeout.BashDefaultTimeoutSec = d.Timeout.BashDefaultTimeoutSec
	}
	if l.Tools.ReadDefaultLines == 0 {
		l.Tools.ReadDefaultLines = d.Tools.ReadDefaultLines
	}
	if l.Tools.BashOutputCapKB == 0 {
		l.Tools.BashOutputCapKB = d.Tools.BashOutputCapKB
	}
	if l.Tools.ToolResultCapKB == 0 {
		l.Tools.ToolResultCapKB = d.Tools.ToolResultCapKB
	}
	if l.Guards.AttachmentMaxMB == 0 {
		l.Guards.AttachmentMaxMB = d.Guards.AttachmentMaxMB
	}
	if l.Guards.WebhookBodyMaxMB == 0 {
		l.Guards.WebhookBodyMaxMB = d.Guards.WebhookBodyMaxMB
	}
	return l
}

// current is the live limits source. Defaults to Default; app/settings swaps it at
// startup (and again on PATCH) via SetProvider. Provider swaps use a plain assignment
// guarded by the call sites' ordering (startup before serve; PATCH handler serialized) —
// consumers read a func value, which is safe to replace.
//
// current 是 limits 的活动来源。默认 Default；app/settings 启动时（与 PATCH 时）经
// SetProvider 换源。消费方读的是 func 值，替换安全。
var current = Default

// Current returns the live limits.
//
// Current 返活动 limits。
func Current() Limits { return current() }

// SetProvider swaps the live source (nil ignored).
//
// SetProvider 换活动来源（nil 忽略）。
func SetProvider(p func() Limits) {
	if p != nil {
		current = p
	}
}
