package stream

import "encoding/json"

// Node is a stream node's payload: a Type discriminant + an opaque Content JSON. The
// protocol deliberately does NOT enumerate node types or their fields — that vocabulary
// belongs to each producing business module (chat defines its message-node types,
// entities producers define theirs). Type is a free string that may encode hierarchy
// (e.g. "tool_call" or "tool_call/read_file"); Content is whatever JSON that type
// carries. domain stays out of the semantics and never inspects Content (#6 反校验剧场).
//
// Node 是流节点的 payload：一个 Type 判别 + 一坨不透明的 Content JSON。协议**刻意不**
// 枚举 node 类型或其字段——那套词表归属各 producer 业务模块（chat 定义它的 message
// 节点类型，entities 的 producer 定义它们的）。Type 是自由字符串、可编码层级（如
// "tool_call" 或 "tool_call/read_file"）；Content 是该 type 携带的任意 JSON。domain
// 不碰语义、从不检查 Content（#6 反校验剧场）。
type Node struct {
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content,omitempty"`
}
