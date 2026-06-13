package agent

import (
	"context"
	"encoding/json"
	"iter"
	"sync"
	"testing"
	"time"

	humanloopapp "github.com/sunweilin/forgify/backend/internal/app/humanloop"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// multiClient replays a distinct script per Stream call (one per ReAct step).
//
// multiClient 每次 Stream 调用回放不同脚本（每 ReAct 步一份）。
type multiClient struct {
	mu      sync.Mutex
	scripts [][]llminfra.StreamEvent
	call    int
}

func (c *multiClient) Stream(_ context.Context, _ llminfra.Request) iter.Seq[llminfra.StreamEvent] {
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

type dangerTool struct{ ran *bool }

func (dangerTool) Name() string                        { return "deploy" }
func (dangerTool) Description() string                 { return "deploy something" }
func (dangerTool) Parameters() json.RawMessage         { return json.RawMessage(`{"type":"object"}`) }
func (dangerTool) ValidateInput(json.RawMessage) error { return nil }
func (t dangerTool) Execute(context.Context, string) (string, error) {
	*t.ran = true
	return "deployed", nil
}

// fakeMounts resolves any mounted refs to a fixed tool list (mount synthesis itself is covered
// by tool/mount's own tests; this test only needs A mounted tool inside the loop).
//
// fakeMounts 把任意挂载 ref 解析为固定工具列表（挂载合成本身由 tool/mount 自己的测试覆盖；
// 本测试只需要 loop 里**有**一个挂载工具）。
type fakeMounts struct{ tools []toolapp.Tool }

func (m fakeMounts) Resolve(context.Context, []agentdomain.ToolRef) ([]toolapp.Tool, error) {
	return m.tools, nil
}

// TestInvoke_DangerGateFromBrokerInCtx is the nested human-in-the-loop proof (R0064): an agent run
// given a humanloop broker via ctx (exactly how a chat turn's broker flows into an invoke_agent
// sub-run) blocks at the shared loop's danger gate before a dangerous tool executes, and a resolve
// — delivered to that same broker, keyed by the sub-run's tool_call id — unblocks it. No bubbling,
// no cascade: the blocked goroutine naturally holds the whole stack, and the run completes inline.
//
// TestInvoke_DangerGateFromBrokerInCtx 是嵌套人在环的证明（R0064）：经 ctx 拿到 humanloop broker 的 agent 运行
// （正是 chat 回合的 broker 流进 invoke_agent 子运行的方式）在危险工具执行前于共享 loop 的 danger 门阻塞，一个
// resolve——送给同一个 broker、按子运行的 tool_call id 键——解阻它。无冒泡、无级联：阻塞的 goroutine 天然 hold
// 住整个栈、运行就地完成。
func TestInvoke_DangerGateFromBrokerInCtx(t *testing.T) {
	svc, baseCtx := newSvc(t)
	ran := false
	svc.SetInvokeDeps(InvokeDeps{
		Resolver: fakeResolver{client: &multiClient{scripts: [][]llminfra.StreamEvent{
			{ // step 1: call deploy, self-reported dangerous
				{Type: llminfra.EventToolStart, ToolIndex: 0, ToolID: "tc1", ToolName: "deploy"},
				{Type: llminfra.EventToolDelta, ToolIndex: 0, ArgsDelta: `{"danger":"dangerous"}`},
				{Type: llminfra.EventFinish, FinishReason: "tool_use", InputTokens: 1, OutputTokens: 1},
			},
			{ // step 2: wrap up
				{Type: llminfra.EventText, Delta: "done"},
				{Type: llminfra.EventFinish, InputTokens: 1, OutputTokens: 1},
			},
		}}},
		Mounts:    fakeMounts{tools: []toolapp.Tool{dangerTool{ran: &ran}}},
		Knowledge: fakeKnowledge{},
	})
	a, _, err := svc.Create(baseCtx, CreateInput{Name: "deployer", Config: Config{Prompt: "deploy it", Tools: []agentdomain.ToolRef{{Ref: "fn_deploy"}}}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	broker := humanloopapp.New(nil)
	ctx := humanloopapp.WithBroker(reqctxpkg.SetConversationID(baseCtx, "c1"), broker)

	done := make(chan *InvokeResult, 1)
	go func() {
		res, _ := svc.InvokeAgent(ctx, InvokeInput{AgentID: a.ID, TriggeredBy: agentdomain.TriggeredByChat})
		done <- res
	}()

	// The sub-run blocks at the danger gate — pending, tool not yet run.
	waitForPending(t, broker, "c1", 1)
	if ran {
		t.Fatal("the dangerous tool must NOT run before approval, even nested in an agent")
	}
	select {
	case <-done:
		t.Fatal("InvokeAgent returned before the danger interaction was resolved")
	default:
	}

	// Resolve via the same broker, keyed by the sub-run's tool_call id — the SERVER-minted
	// block id read off the pending entry (never the provider's recyclable wire id).
	// 经同一 broker 决议，键为子运行的 tool_call id——从 pending 条目读到的服务端铸造块 id
	// （绝非 provider 可复用的线缆 id）。
	if !broker.Resolve(broker.Pending("c1")[0].ToolCallID, humanloopapp.Response{Action: humanloopapp.DecisionApprove}) {
		t.Fatal("Resolve should find the nested pending interaction")
	}
	res := <-done
	if !ran {
		t.Fatal("the tool should run after approval")
	}
	if res == nil || res.Status != agentdomain.ExecutionStatusOK {
		t.Fatalf("the sub-run should complete OK after approval, got %+v", res)
	}
}

func waitForPending(t *testing.T, b *humanloopapp.Broker, conversationID string, n int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for len(b.Pending(conversationID)) != n {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d nested pending interaction(s)", n)
		case <-time.After(5 * time.Millisecond):
		}
	}
}
