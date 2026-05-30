// Package limits is the single source for user-tunable operational ceilings
// (agent steps, output cap, timeouts, tool result sizes). Consumers receive a
// func() Limits getter; in P0 it's limits.Default, in P3 it reads settings.json.
//
// Package limits 是用户可调运行上限的唯一来源（agent 步数 / 输出 cap / 超时 /
// 工具结果体量）。消费方拿 func() Limits getter；P0 = limits.Default，P3 改读 settings.json。
package limits

// Limits mirrors the settings.json "limits" block (see
// adhoc-topic-documents/limits-optimization/02). Zero value of any field means
// "use the Default()" — settings parsing fills zeros via WithDefaults.
//
// Limits 镜像 settings.json 的 "limits" 段。任一字段零值 = 用 Default()。
type Limits struct {
	Agent    AgentLimits    `json:"agent"`
	Output   OutputLimits   `json:"output"`
	Context  ContextLimits  `json:"context"`
	Timeout  TimeoutLimits  `json:"timeout"`
	Tools    ToolLimits     `json:"tools"`
	Workflow WorkflowLimits `json:"workflow"`
	Guards   GuardLimits    `json:"guards"`
}

type AgentLimits struct {
	// MaxSteps caps the chat ReAct loop. 0 = unbounded (rely on user stop).
	// MaxSteps 限聊天 ReAct 循环。0 = 无限（靠用户中断）。
	MaxSteps           int `json:"maxSteps"`
	MaxTurnDurationSec int `json:"maxTurnDurationSec"`
	SubagentTimeoutSec int `json:"subagentTimeoutSec"`
	SubagentMaxTurns   int `json:"subagentMaxTurns"`
}

type OutputLimits struct {
	// UnknownModelMaxTokens is the output cap for models absent from modelcaps.
	// UnknownModelMaxTokens 是 modelcaps 未知模型的输出兜底。
	UnknownModelMaxTokens int `json:"unknownModelMaxTokens"`
	// PerScenarioOverride is optional (P3); empty = use the model's real max.
	// PerScenarioOverride 可选（P3）；空 = 用模型真值。
	PerScenarioOverride map[string]int `json:"perScenarioOverride,omitempty"`
}

type ContextLimits struct {
	SoftRatio float64 `json:"softRatio"`
	HardRatio float64 `json:"hardRatio"`
}

type TimeoutLimits struct {
	// LLMIdleSec is the dead-socket detector, NOT a total wall-clock cap: it
	// resets on every streamed token, so a healthy long stream never trips it.
	// LLMIdleSec 是死连接探测，非总墙钟：每个流式 token 重置，健康长流永不触发。
	LLMIdleSec int `json:"llmIdleSec"`
	// MCPCallSec / BashDefaultTimeoutSec return control to the agent on timeout.
	// MCPCallSec / BashDefaultTimeoutSec 超时把控制权还给 agent。
	MCPCallSec            int `json:"mcpCallSec"`
	BashDefaultTimeoutSec int `json:"bashDefaultTimeoutSec"`
}

type ToolLimits struct {
	// SearchTopN is the uniform default for every search_* tool (hard max 50).
	// SearchTopN 是所有 search_* 的统一默认（硬上限 50）。
	SearchTopN       int `json:"searchTopN"`
	ReadDefaultLines int `json:"readDefaultLines"`
	BashOutputCapKB  int `json:"bashOutputCapKB"`
}

// WorkflowLimits is the unattended exception: these caps stay load-bearing
// because a triggered workflow has no human to stop it (decision #2).
//
// WorkflowLimits 是无人值守例外：触发型 workflow 无人能 stop，这些 cap 仍 load-bearing。
type WorkflowLimits struct {
	AgentNodeMaxTurns     int `json:"agentNodeMaxTurns"`
	AgentNodeMaxTurnsHard int `json:"agentNodeMaxTurnsHard"`
}

// GuardLimits are bucket-2 DoS/OOM guards whose values may be widened (P3).
//
// GuardLimits 是桶 2 的 DoS/OOM 防护，值可放开（P3）。
type GuardLimits struct {
	AttachmentMaxMB   int `json:"attachmentMaxMB"`
	HTTPNodeRespMaxMB int `json:"httpNodeRespMaxMB"`
	WebhookBodyMaxMB  int `json:"webhookBodyMaxMB"`
}

// Default returns the high-ceiling defaults (= 02 §4). Its signature is
// func() Limits, so it doubles as the P0 getter injected into every consumer.
//
// Default 返高 ceiling 默认值（= 02 §4）。签名即 func() Limits，直接当 P0 getter 注入。
func Default() Limits {
	return Limits{
		Agent: AgentLimits{
			MaxSteps:           150,
			MaxTurnDurationSec: 1800,
			SubagentTimeoutSec: 600,
			SubagentMaxTurns:   30,
		},
		Output: OutputLimits{UnknownModelMaxTokens: 64000},
		Context: ContextLimits{
			SoftRatio: 0.70,
			HardRatio: 0.85,
		},
		Timeout: TimeoutLimits{
			LLMIdleSec:            150,
			MCPCallSec:            180,
			BashDefaultTimeoutSec: 120,
		},
		Tools: ToolLimits{
			SearchTopN:       10,
			ReadDefaultLines: 2000,
			BashOutputCapKB:  256,
		},
		Workflow: WorkflowLimits{
			AgentNodeMaxTurns:     10,
			AgentNodeMaxTurnsHard: 50,
		},
		Guards: GuardLimits{
			AttachmentMaxMB:   50,
			HTTPNodeRespMaxMB: 10,
			WebhookBodyMaxMB:  10,
		},
	}
}

// MaxSearchTopN is the hard ceiling for search_* result counts; not tunable.
//
// MaxSearchTopN 是 search_* 返回数的硬上限，不可调。
const MaxSearchTopN = 50
