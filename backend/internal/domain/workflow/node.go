package workflow

// NodeSpec is one graph node; retry/onError/timeout only meaningful for capability nodes.
//
// NodeSpec 是图中一个节点；retry/onError/timeout 仅对 capability 节点有意义。
type NodeSpec struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Position *Position      `json:"position,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
	Retry    *RetryConfig   `json:"retry,omitempty"`
	OnError  string         `json:"onError,omitempty"`
	Timeout  int            `json:"timeout,omitempty"`
	Notes    string         `json:"notes,omitempty"`
}

type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// RetryConfig is per-capability-node retry; MaxAttempts counts total attempts (1 = no retry).
//
// RetryConfig 单节点重试；MaxAttempts 总尝试次数（初次+重试），1 表不重试。
type RetryConfig struct {
	MaxAttempts int    `json:"maxAttempts"`
	Backoff     string `json:"backoff"`
	DelayMs     int    `json:"delay"`
}

// VariableSpec declares a workflow-level variable referenced via {{ vars.x }}.
//
// VariableSpec 声明 workflow 级变量；node 经 {{ vars.x }} 引用。
type VariableSpec struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Default any    `json:"default,omitempty"`
}

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

// IsCapabilityNode reports whether t accepts retry/onError/timeout config.
//
// IsCapabilityNode 报告 t 是否 capability 调用节点（可挂 retry/onError/timeout）。
func IsCapabilityNode(t string) bool {
	switch t {
	case NodeTypeFunction, NodeTypeHandler, NodeTypeMCP, NodeTypeSkill, NodeTypeLLM, NodeTypeHTTP:
		return true
	}
	return false
}

// BranchOutputPorts maps branching NodeType to its valid output ports; condition uses nil for dynamic.
//
// BranchOutputPorts 分叉节点 → 合法输出口名；condition 节点 ports 由 config 动态声明，故 nil。
var BranchOutputPorts = map[string][]string{
	NodeTypeApproval:  {"approved", "rejected"},
	NodeTypeLoop:      {"iterate", "done"},
	NodeTypeCondition: nil,
}

func IsBranchingNode(t string) bool {
	_, ok := BranchOutputPorts[t]
	return ok
}

// IsValidBranchPort reports whether port is valid for nodeType; pass declaredCases for condition.
//
// IsValidBranchPort 报告 port 是否 nodeType 合法输出口；condition 需传 declaredCases。
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

const (
	OnErrorStop     = "stop"
	OnErrorContinue = "continue"
	OnErrorBranch   = "branch"
)

func IsValidOnError(s string) bool {
	switch s {
	case OnErrorStop, OnErrorContinue, OnErrorBranch:
		return true
	}
	return false
}

const (
	VarTypeString  = "string"
	VarTypeNumber  = "number"
	VarTypeInteger = "integer"
	VarTypeBoolean = "boolean"
	VarTypeObject  = "object"
	VarTypeArray   = "array"
)

func IsValidVariableType(t string) bool {
	switch t {
	case VarTypeString, VarTypeNumber, VarTypeInteger, VarTypeBoolean, VarTypeObject, VarTypeArray:
		return true
	}
	return false
}

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
