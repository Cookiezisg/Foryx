package eventlog

import (
	"fmt"
	"strings"
)

// Scope identifies the recipient of an event stream as a (kind, id) tuple.
//
// Scope 标识事件流接收者，为 (kind, id) 二元组。
type Scope struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

const (
	KindConversation = "conversation"
	KindFlowRun      = "flowrun"
	KindFunction     = "function"
	KindHandler      = "handler"
	KindWorkflow     = "workflow"
	// New forge kinds (doc 11 §S2 CANON-X1): forge SSE extended to 6 kinds.
	// Agent = quadrinity 4th member; Document + Skill = right-pane subpage streaming.
	KindAgent    = "agent"
	KindDocument = "document"
	KindSkill    = "skill"
)

// String returns "<kind>:<id>", used as Bridge map key and HTTP ?scope= value.
//
// String 返 "<kind>:<id>"，作 Bridge map key 与 HTTP ?scope= 协议形式。
func (s Scope) String() string {
	return s.Kind + ":" + s.ID
}

// ParseScope parses "<kind>:<id>"; splits on the first ':' since id may contain ':'.
//
// ParseScope 解析 "<kind>:<id>"；只在首个 ':' 切，id 自身可含 ':'。
func ParseScope(raw string) (Scope, error) {
	i := strings.IndexByte(raw, ':')
	if i < 0 {
		return Scope{}, fmt.Errorf("eventlog.ParseScope: missing ':' in %q", raw)
	}
	kind := raw[:i]
	id := raw[i+1:]
	if kind == "" || id == "" {
		return Scope{}, fmt.Errorf("eventlog.ParseScope: empty kind or id in %q", raw)
	}
	return Scope{Kind: kind, ID: id}, nil
}

func IsValidKind(kind string) bool {
	switch kind {
	case KindConversation, KindFlowRun, KindFunction, KindHandler, KindWorkflow,
		KindAgent, KindDocument, KindSkill:
		return true
	}
	return false
}

// ConversationScope wraps a bare conversation id into a Scope.
//
// ConversationScope 把裸 conversation id 升级为 Scope。
func ConversationScope(convID string) Scope {
	return Scope{Kind: KindConversation, ID: convID}
}
