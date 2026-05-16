package workflow

// EdgeSpec is one directed graph edge; branching nodes set FromPort to select the output.
//
// EdgeSpec 是有向边；分叉节点（approval/condition/loop）经 FromPort 选输出口，单输出节点 FromPort 空。
type EdgeSpec struct {
	ID       string `json:"id"`
	From     string `json:"from"`
	FromPort string `json:"fromPort,omitempty"`
	To       string `json:"to"`
	ToPort   string `json:"toPort,omitempty"`
}
