package subagent

import (
	"context"

	"go.uber.org/zap"

	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
)

// nodeTypeMessage is the messages-stream node type for a subagent turn — same "message" type
// chat uses, but its Open.ParentID is the spawning tool_call's block id (E3 nesting) so the
// front end renders the subagent as a live subtree under the Subagent tool_call. loop nests the
// subagent's block nodes under this message node (their Open.ParentID = the sub-message id).
//
// nodeTypeMessage 是 subagent 回合的 messages 流节点类型——与 chat 同的 "message" 型，但其
// Open.ParentID 是派它的 tool_call block id（E3 嵌套），使前端把 subagent 渲成 Subagent tool_call
// 下的实时子树。loop 把 subagent 的 block 节点嵌在此 message 节点下（其 Open.ParentID = sub-message id）。
const nodeTypeMessage = "message"

type messageOpenContent struct {
	Role     string `json:"role"`
	Subagent bool   `json:"subagent"` // marks this turn as a subagent run (front-end groups it)
}

type messageStopContent struct {
	Role         string `json:"role"`
	Status       string `json:"status"`
	StopReason   string `json:"stopReason,omitempty"`
	InputTokens  int    `json:"inputTokens"`
	OutputTokens int    `json:"outputTokens"`
	ErrorCode    string `json:"errorCode,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// emitMessageStart opens the subagent's message node nested under the spawning tool_call
// (parentID). parentID empty (a fork skill not under a tool_call) → it anchors at the
// conversation root, still a valid node.
//
// emitMessageStart 在派它的 tool_call 下（parentID）开 subagent 的 message 节点。parentID 空
// （不在 tool_call 下的 fork skill）→ 锚在 conversation 根，仍是合法节点。
func (s *Service) emitMessageStart(ctx context.Context, conversationID, msgID, parentID string) {
	s.publishFrame(ctx, conversationID, msgID, streamdomain.Open{
		ParentID: parentID,
		Node:     streamdomain.Node{Type: nodeTypeMessage, Content: streamdomain.JSONContent(messageOpenContent{Role: messagesdomain.RoleAssistant, Subagent: true})},
	})
}

// emitMessageStop closes the subagent's message node with its terminal metadata.
//
// emitMessageStop 用终态元数据关 subagent 的 message 节点。
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

// publishFrame pushes one frame anchored at conversation:<id>; best-effort (no bridge → skip,
// recovered by REST history).
//
// publishFrame 推一帧、锚 conversation:<id>；best-effort（无 bridge → 跳过，由 REST 历史兜回）。
func (s *Service) publishFrame(ctx context.Context, conversationID, nodeID string, frame streamdomain.Frame) {
	if s.deps.Bridge == nil {
		return
	}
	if _, err := s.deps.Bridge.Publish(ctx, streamdomain.Event{
		Scope: streamdomain.Scope{Kind: streamdomain.KindConversation, ID: conversationID},
		ID:    nodeID,
		Frame: frame,
	}); err != nil {
		s.log.Warn("subagentapp: messages stream push failed", zap.String("nodeId", nodeID), zap.Error(err))
	}
}
