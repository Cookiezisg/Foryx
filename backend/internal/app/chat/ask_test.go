package chat

import (
	"testing"

	"go.uber.org/zap"

	humanloopapp "github.com/sunweilin/forgify/backend/internal/app/humanloop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	asktool "github.com/sunweilin/forgify/backend/internal/app/tool/ask"
	conversationdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// askCall scripts one step that calls ask_user (self-reported safe — it routes to the ask block,
// not the danger gate).
//
// askCall 脚本一步：调用 ask_user（自报 safe——走 ask 阻塞而非 danger 门）。
func askCall(tcID string) []llminfra.StreamEvent {
	return []llminfra.StreamEvent{
		{Type: llminfra.EventToolStart, ToolIndex: 0, ToolID: tcID, ToolName: "ask_user"},
		{Type: llminfra.EventToolDelta, ToolIndex: 0, ArgsDelta: `{"danger":"safe","message":"Which environment?","options":["staging","prod"]}`},
		{Type: llminfra.EventFinish, FinishReason: "tool_use", InputTokens: 5, OutputTokens: 3},
	}
}

func newAskSvc(t *testing.T, client llminfra.Client, bridge streamdomain.Bridge) (*Service, messagesdomain.Repository) {
	t.Helper()
	store := newStore(t)
	return New(store, Deps{
		Conversations: fakeConvs{conv: &conversationdomain.Conversation{SystemPrompt: "be concise", Title: "t"}},
		Resolver:      fakeResolver{client: client},
		Bridge:        bridge,
		Toolset:       toolapp.Toolset{Resident: []toolapp.Tool{asktool.New()}},
	}, zap.NewNop()), store
}

// TestAsk_AcceptReturnsAnswer: ask_user blocks for the human; accepting feeds the answer back as
// the tool_result and the turn completes.
//
// TestAsk_AcceptReturnsAnswer：ask_user 阻塞等人；accept 把答案当 tool_result 反馈、回合完成。
func TestAsk_AcceptReturnsAnswer(t *testing.T) {
	bridge := newRecordBridge()
	client := &scriptedClient{scripts: [][]llminfra.StreamEvent{askCall("tc1"), textTurn()}}
	svc, store := newAskSvc(t, client, bridge)
	ctx := ctxWS("ws_1")

	asstID, err := svc.Send(ctx, "cv_1", SendInput{Content: "deploy somewhere"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	pending := waitPending(t, svc, "cv_1", 1)
	if pending[0].Kind != humanloopapp.KindAsk || pending[0].Tool != "ask_user" {
		t.Fatalf("unexpected pending interaction: %+v", pending[0])
	}

	if err := svc.ResolveInteraction(ctx, pending[0].ToolCallID, humanloopapp.DecisionAccept, "staging"); err != nil {
		t.Fatalf("ResolveInteraction: %v", err)
	}
	waitClose(t, bridge, asstID)

	got, _ := store.GetMessage(ctx, asstID)
	if got.Status != messagesdomain.StatusCompleted {
		t.Fatalf("turn should complete, got %q", got.Status)
	}
	tr := toolResultUnder(got, pending[0].ToolCallID)
	if tr == nil || tr.Content != "staging" {
		t.Fatalf("ask_user tool_result should hold the answer, got %+v", tr)
	}
}

// TestAsk_Decline: declining feeds the re-route hint back as the tool_result.
//
// TestAsk_Decline：decline 把改道提示当 tool_result 反馈。
func TestAsk_Decline(t *testing.T) {
	bridge := newRecordBridge()
	client := &scriptedClient{scripts: [][]llminfra.StreamEvent{askCall("tc1"), textTurn()}}
	svc, store := newAskSvc(t, client, bridge)
	ctx := ctxWS("ws_1")

	asstID, _ := svc.Send(ctx, "cv_1", SendInput{Content: "deploy"})
	pending := waitPending(t, svc, "cv_1", 1)
	if err := svc.ResolveInteraction(ctx, pending[0].ToolCallID, humanloopapp.DecisionDecline, ""); err != nil {
		t.Fatalf("ResolveInteraction: %v", err)
	}
	waitClose(t, bridge, asstID)

	got, _ := store.GetMessage(ctx, asstID)
	tr := toolResultUnder(got, pending[0].ToolCallID)
	if tr == nil || tr.Content != humanloopapp.DeclineFeedback {
		t.Fatalf("decline should feed the re-route hint, got %+v", tr)
	}
}

// toolResultUnder finds the tool_result block whose parent is the given tool_call id.
//
// toolResultUnder 找父为给定 tool_call id 的 tool_result 块。
func toolResultUnder(m *messagesdomain.Message, toolCallID string) *messagesdomain.Block {
	for i := range m.Blocks {
		b := &m.Blocks[i]
		if b.ParentBlockID == toolCallID && b.Type == messagesdomain.BlockTypeToolResult {
			return b
		}
	}
	return nil
}
