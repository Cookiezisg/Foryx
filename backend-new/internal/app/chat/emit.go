package chat

import (
	"context"

	"go.uber.org/zap"

	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
)

// nodeTypeMessage is the messages-stream node type for a whole conversation turn — the parent
// under which loop's block nodes (text / reasoning / tool_call / tool_result) nest. message_start
// is its Open, message_stop its Close (carrying the turn's terminal metadata + token accounting).
// loop emits the block vocabulary; chat emits this one message-level type.
//
// nodeTypeMessage 是一整个对话回合的 messages 流节点类型——loop 的 block 节点（text / reasoning /
// tool_call / tool_result）嵌在其下的父节点。message_start 是它的 Open、message_stop 是 Close
// （带回合终态元数据 + token 记账）。loop 发 block 词表；chat 发这一个 message 级类型。
const nodeTypeMessage = "message"

// messageOpenContent rides message_start: just the role, so the front end can render the right
// bubble before any block streams in.
//
// messageOpenContent 随 message_start：只带 role，使前端在任何 block 流入前就能渲对的气泡。
type messageOpenContent struct {
	Role string `json:"role"`
}

// messageUserContent is the user turn's close snapshot — the echoed text (+ attachment ids) the
// front end renders the user bubble from, without re-fetching.
//
// messageUserContent 是 user 回合的 close 快照——前端据此渲用户气泡的回显文本（+ 附件 id），无需回取。
type messageUserContent struct {
	Role          string   `json:"role"`
	Content       string   `json:"content"`
	AttachmentIDs []string `json:"attachmentIds,omitempty"`
}

// messageStopContent is the assistant turn's close snapshot: terminal status + stop reason +
// token accounting (turn metadata — deliberately NOT in any block snapshot). The front end ends
// the streaming bubble and shows the token cost from this.
//
// messageStopContent 是 assistant 回合的 close 快照：终态 + stop reason + token 记账（回合元数据
// ——刻意不进任何 block 快照）。前端据此结束流式气泡并显示 token 成本。
type messageStopContent struct {
	Role         string `json:"role"`
	Status       string `json:"status"`
	StopReason   string `json:"stopReason,omitempty"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	ErrorCode    string `json:"errorCode,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// emitUserMessage echoes a complete user turn as one message node (Open then Close) so every
// connected client sees it immediately. No-op when no bridge is wired (REST history still has it).
//
// emitUserMessage 把一个完整 user 回合作为一个 message 节点回显（Open 后 Close），使每个连接的
// 客户端立即看到。无 bridge 时 no-op（REST 历史仍有）。
func (s *Service) emitUserMessage(ctx context.Context, conversationID string, m *messagesdomain.Message, text string) {
	s.publishFrame(ctx, conversationID, m.ID, streamdomain.Open{
		Node: streamdomain.Node{Type: nodeTypeMessage, Content: streamdomain.JSONContent(messageOpenContent{Role: messagesdomain.RoleUser})},
	})
	s.publishFrame(ctx, conversationID, m.ID, streamdomain.Close{
		Status: messagesdomain.StatusCompleted,
		Result: &streamdomain.Node{
			Type:    nodeTypeMessage,
			Content: streamdomain.JSONContent(messageUserContent{Role: messagesdomain.RoleUser, Content: text, AttachmentIDs: attachmentIDsOf(m)}),
		},
	})
}

// emitMessageStart opens the assistant turn's message node; loop then nests its block nodes
// under msgID (their Open.ParentID = msgID).
//
// emitMessageStart 开 assistant 回合的 message 节点；loop 随后把 block 节点嵌在 msgID 下
// （其 Open.ParentID = msgID）。
func (s *Service) emitMessageStart(ctx context.Context, conversationID, msgID string) {
	s.publishFrame(ctx, conversationID, msgID, streamdomain.Open{
		Node: streamdomain.Node{Type: nodeTypeMessage, Content: streamdomain.JSONContent(messageOpenContent{Role: messagesdomain.RoleAssistant})},
	})
}

// emitMessageStop closes the assistant turn's message node with its terminal metadata — the
// final frame of a generation. The Close.Result snapshot is the reconnect truth for the turn's
// status + token cost.
//
// emitMessageStop 用终态元数据关 assistant 回合的 message 节点——一次生成的最后一帧。Close.Result
// 快照是回合状态 + token 成本的重连真相。
func (s *Service) emitMessageStop(ctx context.Context, conversationID string, m *messagesdomain.Message) {
	s.publishFrame(ctx, conversationID, m.ID, streamdomain.Close{
		Status: m.Status,
		Error:  m.ErrorMessage,
		Result: &streamdomain.Node{
			Type: nodeTypeMessage,
			Content: streamdomain.JSONContent(messageStopContent{
				Role:         messagesdomain.RoleAssistant,
				Status:       m.Status,
				StopReason:   m.StopReason,
				InputTokens:  m.InputTokens,
				OutputTokens: m.OutputTokens,
				ErrorCode:    m.ErrorCode,
				ErrorMessage: m.ErrorMessage,
			}),
		},
	})
}

// publishFrame pushes one frame for a node anchored at conversation:<id>. best-effort: no bridge
// → skip; a failed push is recovered by SSE replay + REST history, so it never fails the turn.
//
// publishFrame 推一帧到锚在 conversation:<id> 的节点。best-effort：无 bridge → 跳过；推送失败由
// SSE replay + REST 历史兜回，故绝不让回合失败。
func (s *Service) publishFrame(ctx context.Context, conversationID, nodeID string, frame streamdomain.Frame) {
	if s.deps.Bridge == nil {
		return
	}
	if _, err := s.deps.Bridge.Publish(ctx, streamdomain.Event{
		Scope: streamdomain.Scope{Kind: streamdomain.KindConversation, ID: conversationID},
		ID:    nodeID,
		Frame: frame,
	}); err != nil {
		s.log.Warn("chatapp: messages stream push failed", zap.String("nodeId", nodeID), zap.Error(err))
	}
}
