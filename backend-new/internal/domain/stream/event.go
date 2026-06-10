// Package stream defines the unified streaming-tree protocol shared by the three SSE
// streams (messages / entities / notifications). The design separates transport from
// semantics: all three share one envelope + four tree-operation verbs (Frame) + a
// generic Node ({type, content}); the node vocabulary is owned by each producer
// business module, not by domain. See lab/backendcleaner/target/stream-protocol.md.
//
// Package stream 定义三条 SSE 流（messages / entities / notifications）共享的统一
// 「流式树」协议。设计是传输与语义正交：三流共享一个信封 + 四个树操作动词（Frame）+
// 一个通用 Node（{type, content}）；node 词表归各 producer 业务模块，不归 domain。
// 见 stream-protocol.md。
package stream

import "encoding/json"

// Event is what a producer emits — an unsequenced draft. The producer supplies the
// target Scope, the node ID it operates on, and the Frame; it does not know the seq
// (that is the bus's job — keeping the Event/Envelope split honest at the type level).
//
// Event 是 producer 要发的内容——未编号草稿。producer 提供目标 Scope、所操作的节点
// ID、Frame；它不知道 seq（seq 是 bus 的职责——用类型把"草稿/成品"边界划清）。
type Event struct {
	Scope Scope  `json:"scope"`
	ID    string `json:"id"`
	Frame Frame  `json:"frame"`
}

// Envelope is an Event stamped with the bus-assigned seq (the delivered form).
// Seq is monotonic per stream; ephemeral frames carry Seq 0 (no replay, no id: line).
//
// Envelope 是被 bus 盖了 seq 章的 Event（投递形态）。Seq 每流单调；ephemeral 帧
// Seq 为 0（不 replay、无 id: 行）。
type Envelope struct {
	Seq int64 `json:"seq"`
	Event
}

// Node is a frame's payload: a Type discriminant + an opaque Content JSON. The protocol
// deliberately does NOT enumerate node types or their fields — that vocabulary belongs
// to each producing business module (chat defines its message-node types, entities
// producers define theirs). Type is a free string that may encode hierarchy (e.g.
// "tool_call" or "tool_call/read_file"); Content is whatever JSON that type carries.
// domain stays out of the semantics and never inspects Content (#6 反校验剧场).
//
// Node 是帧的 payload：一个 Type 判别 + 一坨不透明的 Content JSON。协议**刻意不**枚举
// node 类型或其字段——那套词表归属各 producer 业务模块（chat 定义它的 message 节点类型，
// entities 的 producer 定义它们的）。Type 是自由字符串、可编码层级（如 "tool_call" 或
// "tool_call/read_file"）；Content 是该 type 携带的任意 JSON。domain 不碰语义、从不检查
// Content（#6 反校验剧场）。
type Node struct {
	Type    string          `json:"type"`
	Content json.RawMessage `json:"content,omitempty"`
}

// JSONContent marshals v into a Node.Content payload, degrading to nil on the never-expected
// failure (these are flat producer structs) rather than aborting a stream. The one shared helper
// every producer (loop / chat / subagent / mcp / trigger …) uses to build node content.
//
// JSONContent 把 v marshal 成 Node.Content payload，（这些都是扁平 producer 结构）不应失败、失败降级
// nil 而非中断流。所有 producer（loop / chat / subagent / mcp / trigger…）造节点内容共用的唯一 helper。
func JSONContent(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
