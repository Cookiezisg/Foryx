// edge.go — EdgeSpec.
//
// edge.go —— EdgeSpec。

package workflow

// EdgeSpec is one directed graph edge connecting two node ports. From /
// To are dotted "<nodeId>.<portName>" strings; the port half is
// optional (defaults to "output" for from, "input" for to). ID is
// system-generated (edge_<random>) — the LLM never supplies it.
//
// V1 behaviour (per 04-workflow.md §5):
//   - data flows verbatim from upstream output to downstream input
//   - each input port at most 1 edge; each output port can fan out
//   - no inline transform / filter on the edge itself — semantics on the node
//
// EdgeSpec 一条有向图边连接两个节点端口。From/To "<nodeId>.<portName>";
// port 部分可省(from 默认 "output",to 默认 "input")。ID 系统生成
// (`edge_<random>`),LLM 不传。
type EdgeSpec struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}
