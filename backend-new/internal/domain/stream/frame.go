package stream

// Frame is one operation on the rendering tree — a closed union of four verbs.
// Durable reports whether the frame enters the replay buffer: structure-bearing
// frames (open/close, and non-ephemeral signals) are durable so a reconnect can
// rebuild the tree; deltas (and ephemeral signals) are live-only and lossy. This
// tiering is why token-rate deltas never overflow the replay window.
//
// Frame 是对渲染树的一次操作——四动词封闭联合。Durable 报告该帧是否进 replay
// buffer：承载结构的帧（open/close、非 ephemeral 的 signal）durable，保证重连能重建
// 树；delta（与 ephemeral signal）实时可丢。正是这个分级让 token 级 delta 永不撑爆
// replay 窗口。
type Frame interface {
	frame()        // unexported marker keeps the union closed against outside types.
	Durable() bool // whether to buffer for replay.
}

// Close statuses — a node's terminal state. (A node is implicitly "streaming"
// between its open and close.)
//
// Close 终态——节点终结状态。（节点在 open 与 close 之间隐含为 streaming。）
const (
	StatusCompleted = "completed"
	StatusError     = "error"
	StatusCancelled = "cancelled"
)

// IsValidStatus reports whether s is a valid Close terminal status.
//
// IsValidStatus 报告 s 是否合法 Close 终态。
func IsValidStatus(s string) bool {
	switch s {
	case StatusCompleted, StatusError, StatusCancelled:
		return true
	}
	return false
}

// Open creates a node; ParentID empty = top-level, non-empty = nested mount point
// (e.g. a tool_call node whose children are the invoked agent's message subtree).
//
// Open 创建节点；ParentID 空 = 顶层，非空 = 嵌套挂载点（如 tool_call 下挂被调
// agent 的 message 子树）。
type Open struct {
	ParentID string `json:"parentId,omitempty"`
	Node     Node   `json:"node"`
}

// Delta appends a streaming chunk to an open node (token text / terminal output).
//
// Delta 给开着的节点追加流式 chunk（token 文本 / 终端输出）。
type Delta struct {
	Chunk string `json:"chunk"`
}

// Close terminates a node. Result, when present, carries the node's final content
// snapshot — the reconnect source of truth for streamed nodes (deltas are lossy, so
// the durable Close must be able to rebuild the content); nil for nodes that stream
// nothing. Error is set only on StatusError.
//
// Close 结束节点。Result 非 nil 时携带节点最终内容快照——流式节点的重连真相（delta
// 可丢，durable 的 Close 须能重建内容）；无流式内容的节点为 nil。Error 仅 StatusError 时非空。
type Close struct {
	Status string `json:"status"`
	Result *Node  `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Signal is a one-shot broadcast that builds no tree node (entity changed, flowrun
// tick). Ephemeral marks the lossy, no-backpressure class (ticks) vs the durable
// class (entity changes): a tick dropped on reconnect is fine, an entity change is not.
//
// Signal 是不建树节点的瞬时广播（实体变更、flowrun tick）。Ephemeral 标记可丢无背压
// 类（tick）与必达类（实体变更）：tick 重连时丢了无妨，实体变更不行。
type Signal struct {
	Node      Node `json:"node"`
	Ephemeral bool `json:"-"` // delivery semantics, not carried on the wire.
}

func (Open) frame()   {}
func (Delta) frame()  {}
func (Close) frame()  {}
func (Signal) frame() {}

func (Open) Durable() bool     { return true }
func (Delta) Durable() bool    { return false }
func (Close) Durable() bool    { return true }
func (s Signal) Durable() bool { return !s.Ephemeral }
