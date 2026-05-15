// node.go — NodeSpec + Position + RetryConfig + VariableSpec + 13 NodeType
// enum + 3 OnError values + 6 VariableType values.
//
// node.go —— NodeSpec 等;13 NodeType 封闭枚举。

package workflow

// NodeSpec is one graph node. type is the discriminator; config is the
// type-specific JSON object (each NodeType has its own config schema —
// see 04-workflow.md §2). retry / onError / timeout are optional and
// only meaningful for capability nodes (function / handler / mcp /
// skill / llm / http).
//
// NodeSpec 是图中一个节点。type 判别;config 类型特定 JSON object;
// retry/onError/timeout 仅 capability 节点有意义。
type NodeSpec struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Position *Position      `json:"position,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
	Retry    *RetryConfig   `json:"retry,omitempty"`
	OnError  string         `json:"onError,omitempty"`
	Timeout  int            `json:"timeout,omitempty"` // ms
	Notes    string         `json:"notes,omitempty"`
}

// Position is the (x, y) coordinate for graph editor layout. Optional —
// when omitted the editor auto-lays out. Stored as part of the node so
// the visual layout survives revert.
//
// Position 是图编辑器的 (x, y) 坐标。可省;省时编辑器 auto-layout。
type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// RetryConfig is the per-capability-node retry knob. MaxAttempts counts
// total attempts (initial + retries); a value of 1 means "no retry".
// Backoff is one of "exponential" / "linear" / "fixed". DelayMs is the
// initial delay between attempts (exponential doubles each round).
//
// RetryConfig 单节点重试。MaxAttempts 总尝试次数(初次+重试);1=不重试。
// Backoff 三选一;DelayMs 初始延迟(exponential 每轮翻倍)。
type RetryConfig struct {
	MaxAttempts int    `json:"maxAttempts"`
	Backoff     string `json:"backoff"`
	DelayMs     int    `json:"delay"`
}

// VariableSpec declares a workflow-level variable referenced by node
// configs via {{ vars.x }} expressions. Default is optional; when absent
// and no node sets the variable, expression reads return the zero value
// of the declared type.
//
// VariableSpec 声明 workflow 级变量;node 经 {{ vars.x }} 引用。
// Default 可省,省时引用返类型零值。
type VariableSpec struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Default any    `json:"default,omitempty"`
}

// Node type constants — closed enumeration of 13. New types require:
//   - protocol doc update (04-workflow.md §2)
//   - validator update (workflow/validate.go)
//   - executor support (scheduler / Plan 05)
//   - DB CHECK if added (currently service-level whitelist)
//
// Node type 常量,封闭 13 种。新增需同 PR 改协议文档 + 校验器 + 执行器。
const (
	NodeTypeTrigger   = "trigger"
	NodeTypeFunction  = "function"
	NodeTypeHandler   = "handler"
	NodeTypeMCP       = "mcp"
	NodeTypeSkill     = "skill"
	NodeTypeLLM       = "llm"
	NodeTypeHTTP      = "http"
	NodeTypeCondition = "condition"
	NodeTypeLoop      = "loop"
	NodeTypeParallel  = "parallel"
	NodeTypeApproval  = "approval"
	NodeTypeWait      = "wait"
	NodeTypeVariable  = "variable"
)

// IsValidNodeType reports whether t is a known V1 node type.
//
// IsValidNodeType 报告 t 是否合法 V1 节点 type。
func IsValidNodeType(t string) bool {
	switch t {
	case NodeTypeTrigger, NodeTypeFunction, NodeTypeHandler, NodeTypeMCP,
		NodeTypeSkill, NodeTypeLLM, NodeTypeHTTP, NodeTypeCondition,
		NodeTypeLoop, NodeTypeParallel, NodeTypeApproval, NodeTypeWait,
		NodeTypeVariable:
		return true
	}
	return false
}

// IsCapabilityNode reports whether t is a capability-invocation node
// type (function / handler / mcp / skill / llm / http). These accept
// retry / onError / timeout config; non-capability nodes (trigger /
// condition / loop / parallel / approval / wait / variable) don't.
//
// IsCapabilityNode 报告 t 是否 capability 调用节点(可挂 retry/onError/
// timeout)。
func IsCapabilityNode(t string) bool {
	switch t {
	case NodeTypeFunction, NodeTypeHandler, NodeTypeMCP, NodeTypeSkill, NodeTypeLLM, NodeTypeHTTP:
		return true
	}
	return false
}

// BranchOutputPorts maps each branching NodeType to its valid output
// ports. Edges leaving these nodes MUST set EdgeSpec.FromPort to one of
// these values; edges from any other (single-output) node MUST leave
// FromPort empty. Validated at workflow create/edit time.
//
// condition's case names are dynamic (declared in node.Config.cases) —
// it's listed here as a sentinel value (nil slice) so the validator
// knows "this is branching but ports come from config" and defers to a
// runtime port-name check.
//
// BranchOutputPorts 分叉节点 → 合法输出口名。条件节点 cases 动态(在
// node.Config 里声明),用 nil slice 标"分叉但 port 来自 config"。
var BranchOutputPorts = map[string][]string{
	NodeTypeApproval:  {"approved", "rejected"},
	NodeTypeLoop:      {"iterate", "done"},
	NodeTypeCondition: nil, // dynamic — read from node.Config["cases"]
}

// IsBranchingNode reports whether t emits multiple named output ports
// at runtime (and therefore edges from it must declare FromPort).
//
// IsBranchingNode 报告 t 是否运行时有多个命名输出口(其出边必带 FromPort)。
func IsBranchingNode(t string) bool {
	_, ok := BranchOutputPorts[t]
	return ok
}

// IsValidBranchPort reports whether port is a valid output port for a
// node of type nodeType. For condition nodes (dynamic), declaredCases
// is the slice of case names extracted from node.Config["cases"]; pass
// nil for non-condition types.
//
// IsValidBranchPort 报告 port 是否 nodeType 的合法输出口。condition 节点
// 动态,declaredCases 是 config["cases"] 里的 case 名;其他类型传 nil。
func IsValidBranchPort(nodeType, port string, declaredCases []string) bool {
	ports, ok := BranchOutputPorts[nodeType]
	if !ok {
		return false
	}
	if nodeType == NodeTypeCondition {
		for _, c := range declaredCases {
			if c == port {
				return true
			}
		}
		return false
	}
	for _, p := range ports {
		if p == port {
			return true
		}
	}
	return false
}

// OnError values for capability nodes. stop fails the whole run;
// continue lets execution flow past with nil output; branch sends to
// the node's "error" output port (only valid when an edge exists from
// that port).
//
// OnError 值;stop 整 run 失败,continue 继续,branch 走 "error" 端口。
const (
	OnErrorStop     = "stop"
	OnErrorContinue = "continue"
	OnErrorBranch   = "branch"
)

// IsValidOnError reports whether s is a known OnError value.
//
// IsValidOnError 报告 s 是否合法 OnError 值。
func IsValidOnError(s string) bool {
	switch s {
	case OnErrorStop, OnErrorContinue, OnErrorBranch:
		return true
	}
	return false
}

// VariableType values for VariableSpec.Type. Same vocabulary as
// function.ParameterSpec types so the LLM uses one vocabulary across
// trinity entities.
//
// VariableType 值;跟 function.ParameterSpec 同词表。
const (
	VarTypeString  = "string"
	VarTypeNumber  = "number"
	VarTypeInteger = "integer"
	VarTypeBoolean = "boolean"
	VarTypeObject  = "object"
	VarTypeArray   = "array"
)

// IsValidVariableType reports whether t is a known variable type.
//
// IsValidVariableType 报告 t 是否合法变量类型。
func IsValidVariableType(t string) bool {
	switch t {
	case VarTypeString, VarTypeNumber, VarTypeInteger, VarTypeBoolean, VarTypeObject, VarTypeArray:
		return true
	}
	return false
}

// Op constants — 9 ops the LLM uses to mutate a workflow graph.
//
// Op 常量,LLM 改 workflow 图的 9 个 op。
const (
	OpSetMeta       = "set_meta"
	OpAddNode       = "add_node"
	OpUpdateNode    = "update_node"
	OpDeleteNode    = "delete_node"
	OpAddEdge       = "add_edge"
	OpUpdateEdge    = "update_edge"
	OpDeleteEdge    = "delete_edge"
	OpSetVariable   = "set_variable"
	OpUnsetVariable = "unset_variable"
)
