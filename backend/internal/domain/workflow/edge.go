// edge.go — EdgeSpec.
//
// edge.go —— EdgeSpec。

package workflow

// EdgeSpec is one directed graph edge connecting two nodes. From / To are
// plain node IDs (no port suffix). For branching nodes (approval /
// condition / loop) that emit different outputs depending on runtime
// decision, FromPort selects which output port the edge consumes:
//
//   - approval node:  FromPort = "approved" | "rejected"
//   - condition node: FromPort = case name from node config
//   - loop node:      FromPort = "iterate" | "done"
//
// Single-output nodes (trigger / function / handler / mcp / skill / llm /
// http / wait / variable / parallel) leave FromPort empty.
//
// ToPort is reserved for V1.5 when a node may have multiple input ports.
// V1 nodes accept all incoming edges on a single implicit input.
//
// ID is system-generated (`edge_<random>`) — the LLM never supplies it.
//
// EdgeSpec 是有向边。From/To 纯 node ID(不带 port 后缀)。分叉节点
// (approval/condition/loop)需 FromPort 选输出口;单输出节点 FromPort 空。
// ToPort 留 V1.5,V1 单输入。ID 系统生成。
type EdgeSpec struct {
	ID       string `json:"id"`
	From     string `json:"from"`
	FromPort string `json:"fromPort,omitempty"`
	To       string `json:"to"`
	ToPort   string `json:"toPort,omitempty"`
}
