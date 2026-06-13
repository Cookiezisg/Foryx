package chat

import (
	"context"
	"encoding/json"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	humanloopapp "github.com/sunweilin/forgify/backend/internal/app/humanloop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	conversationdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// scriptedClient replays a distinct StreamEvent slice per Stream call (one per ReAct step), so a
// multi-step turn (a tool call, then a follow-up after the tool_result) can be scripted.
//
// scriptedClient 每次 Stream 调用回放不同的脚本（每 ReAct 步一份），使多步回合（一次工具调用、tool_result 后的
// 跟进）可脚本化。
type scriptedClient struct {
	mu      sync.Mutex
	scripts [][]llminfra.StreamEvent
	call    int
}

func (c *scriptedClient) Stream(_ context.Context, _ llminfra.Request) iter.Seq[llminfra.StreamEvent] {
	c.mu.Lock()
	i := c.call
	c.call++
	c.mu.Unlock()
	var s []llminfra.StreamEvent
	if i < len(c.scripts) {
		s = c.scripts[i]
	}
	return func(yield func(llminfra.StreamEvent) bool) {
		for _, ev := range s {
			if !yield(ev) {
				return
			}
		}
	}
}

// recordingTool records whether it ran (so a deny test can assert it did NOT).
//
// recordingTool 记录是否跑过（使 deny 测试能断言它**没**跑）。
type recordingTool struct {
	name string
	ran  *bool
}

func (t recordingTool) Name() string                      { return t.name }
func (recordingTool) Description() string                 { return "does a thing" }
func (recordingTool) Parameters() json.RawMessage         { return json.RawMessage(`{"type":"object"}`) }
func (recordingTool) ValidateInput(json.RawMessage) error { return nil }
func (t recordingTool) Execute(context.Context, string) (string, error) {
	*t.ran = true
	return "did the thing", nil
}

// dangerCall scripts one step that calls tool `name` self-reporting danger=dangerous (id tcID).
//
// dangerCall 脚本一步：调用工具 name 且自报 danger=dangerous（id tcID）。
func dangerCall(tcID, name string) []llminfra.StreamEvent {
	return []llminfra.StreamEvent{
		{Type: llminfra.EventToolStart, ToolIndex: 0, ToolID: tcID, ToolName: name},
		{Type: llminfra.EventToolDelta, ToolIndex: 0, ArgsDelta: `{"danger":"dangerous","target":"prod"}`},
		{Type: llminfra.EventFinish, FinishReason: "tool_use", InputTokens: 5, OutputTokens: 3},
	}
}

func newDangerSvc(t *testing.T, client llminfra.Client, bridge streamdomain.Bridge, tool recordingTool) (*Service, messagesdomain.Repository) {
	t.Helper()
	store := newStore(t)
	return New(store, Deps{
		// Title set so auto-title doesn't fire (it would consume a scripted Stream call and shift the
		// per-turn script indices in multi-turn tests).
		//
		// 设 Title 使 auto-title 不触发（否则它消耗一个脚本 Stream 调用、打乱多回合测试的每回合脚本索引）。
		Conversations: fakeConvs{conv: &conversationdomain.Conversation{SystemPrompt: "be concise", Title: "t"}},
		Resolver:      fakeResolver{client: client},
		Bridge:        bridge,
		Toolset:       toolapp.Toolset{Resident: []toolapp.Tool{tool}},
	}, zap.NewNop()), store
}

// waitPending polls until the conversation has n awaiting interactions, or fails after a timeout.
//
// waitPending 轮询直到对话有 n 条待决交互，超时则失败。
func waitPending(t *testing.T, svc *Service, conversationID string, n int) []humanloopapp.Request {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		p := svc.PendingInteractions(context.Background(), conversationID)
		if len(p) == n {
			return p
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d pending interaction(s), have %d", n, len(p))
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestDanger_ApproveRunsTool: a self-reported-dangerous call blocks for approval; approving runs
// the tool and the turn completes.
//
// TestDanger_ApproveRunsTool：自报 dangerous 的调用阻塞等批准；批准后跑工具、回合完成。
func TestDanger_ApproveRunsTool(t *testing.T) {
	ran := false
	bridge := newRecordBridge()
	client := &scriptedClient{scripts: [][]llminfra.StreamEvent{dangerCall("tc1", "deploy"), textTurn()}}
	svc, store := newDangerSvc(t, client, bridge, recordingTool{name: "deploy", ran: &ran})
	ctx := ctxWS("ws_1")

	asstID, err := svc.Send(ctx, "cv_1", SendInput{Content: "deploy"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	pending := waitPending(t, svc, "cv_1", 1)
	// The interaction id is the SERVER-minted block id (blk_), never the provider's wire id
	// — providers recycle ids, so the wire id cannot key anything durable.
	// 交互 id 是服务端铸造的块 id（blk_）、绝非 provider 线缆 id——provider 会复用 id，线缆 id
	// 不能作任何持久键。
	if pending[0].Kind != humanloopapp.KindDanger || pending[0].Tool != "deploy" || !strings.HasPrefix(pending[0].ToolCallID, "blk_") {
		t.Fatalf("unexpected pending interaction: %+v", pending[0])
	}
	if ran {
		t.Fatal("tool must NOT run before approval (interrupt-before-side-effect)")
	}

	if err := svc.ResolveInteraction(ctx, pending[0].ToolCallID, humanloopapp.DecisionApprove, ""); err != nil {
		t.Fatalf("ResolveInteraction: %v", err)
	}
	waitClose(t, bridge, asstID)
	if !ran {
		t.Fatal("tool should run after approval")
	}
	got, _ := store.GetMessage(ctx, asstID)
	if got.Status != messagesdomain.StatusCompleted {
		t.Fatalf("turn should complete, got %q", got.Status)
	}
}

// TestDanger_DenySkipsTool: denying records the denial as the tool_result and never runs the tool.
//
// TestDanger_DenySkipsTool：拒绝把拒绝记为 tool_result、绝不跑工具。
func TestDanger_DenySkipsTool(t *testing.T) {
	ran := false
	bridge := newRecordBridge()
	client := &scriptedClient{scripts: [][]llminfra.StreamEvent{dangerCall("tc1", "deploy"), textTurn()}}
	svc, store := newDangerSvc(t, client, bridge, recordingTool{name: "deploy", ran: &ran})
	ctx := ctxWS("ws_1")

	asstID, _ := svc.Send(ctx, "cv_1", SendInput{Content: "deploy"})
	pending := waitPending(t, svc, "cv_1", 1)
	if err := svc.ResolveInteraction(ctx, pending[0].ToolCallID, humanloopapp.DecisionDeny, ""); err != nil {
		t.Fatalf("ResolveInteraction: %v", err)
	}
	waitClose(t, bridge, asstID)
	if ran {
		t.Fatal("a denied tool must never run")
	}
	got, _ := store.GetMessage(ctx, asstID)
	if got.Status != messagesdomain.StatusCompleted {
		t.Fatalf("turn should complete after deny, got %q", got.Status)
	}
}

// TestDanger_ApproveAlwaysWhitelists: approve_always runs the tool AND session-whitelists it, so a
// later dangerous call to the same tool in the same conversation runs without blocking.
//
// TestDanger_ApproveAlwaysWhitelists：approve_always 跑工具**并**会话白名单它，使同对话后续对同一工具的危险调用
// 不阻塞直接跑。
func TestDanger_ApproveAlwaysWhitelists(t *testing.T) {
	ran := false
	bridge := newRecordBridge()
	client := &scriptedClient{scripts: [][]llminfra.StreamEvent{
		dangerCall("tc1", "deploy"), textTurn(), // turn 1: blocks → approve_always
		dangerCall("tc2", "deploy"), textTurn(), // turn 2: same tool, distinct id → must NOT block
	}}
	svc, store := newDangerSvc(t, client, bridge, recordingTool{name: "deploy", ran: &ran})
	ctx := ctxWS("ws_1")

	asst1, _ := svc.Send(ctx, "cv_1", SendInput{Content: "deploy"})
	pending := waitPending(t, svc, "cv_1", 1)
	if err := svc.ResolveInteraction(ctx, pending[0].ToolCallID, humanloopapp.DecisionApproveAlways, ""); err != nil {
		t.Fatalf("approve_always: %v", err)
	}
	waitClose(t, bridge, asst1)

	// turn 2: the whitelisted tool must run without ever blocking
	ran = false
	asst2, _ := svc.Send(ctx, "cv_1", SendInput{Content: "deploy again"})
	waitClose(t, bridge, asst2)
	if len(svc.PendingInteractions(ctx, "cv_1")) != 0 {
		t.Fatal("turn 2 must not block — the tool was session-whitelisted")
	}
	if !ran {
		t.Fatal("turn 2's whitelisted tool should run")
	}
	got, _ := store.GetMessage(ctx, asst2)
	if got.Status != messagesdomain.StatusCompleted {
		t.Fatalf("turn 2 should complete, got %q", got.Status)
	}
}
