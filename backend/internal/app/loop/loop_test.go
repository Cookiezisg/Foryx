package loop

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"strings"
	"testing"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	messagesdomain "github.com/sunweilin/anselm/backend/internal/domain/messages"
	llminfra "github.com/sunweilin/anselm/backend/internal/infra/llm"
)

// --- fakes -----------------------------------------------------------------

// fakeClient replays one scripted StreamEvent slice per Stream call (one per ReAct step)
// and captures the messages it was handed, so tests can assert reminder injection.
//
// fakeClient 每次 Stream 调用回放一份脚本（每 ReAct 步一份），并记录收到的 messages，供测试断言
// reminder 注入。
type fakeClient struct {
	scripts  [][]llminfra.StreamEvent
	calls    int
	captured [][]llminfra.LLMMessage
}

func (c *fakeClient) Stream(_ context.Context, req llminfra.Request) iter.Seq[llminfra.StreamEvent] {
	c.captured = append(c.captured, req.Messages)
	idx := c.calls
	c.calls++
	return func(yield func(llminfra.StreamEvent) bool) {
		if idx >= len(c.scripts) {
			return
		}
		for _, ev := range c.scripts[idx] {
			if !yield(ev) {
				return
			}
		}
	}
}

type finalizeCapture struct {
	blocks                              []messagesdomain.Block
	status, stopReason, errCode, errMsg string
	in, out                             int
	called                              int
}

// fakeHost implements only Host — the minimal surface. reminderHost / autoActivateHost embed
// it to add the optional capabilities.
//
// fakeHost 只实现 Host——最小面。reminderHost / autoActivateHost 嵌入它以加可选能力。
type fakeHost struct {
	history []llminfra.LLMMessage
	tools   []toolapp.Tool
	fin     finalizeCapture
}

func (h *fakeHost) LoadHistory(context.Context) ([]llminfra.LLMMessage, error) { return h.history, nil }
func (h *fakeHost) Tools(context.Context) []toolapp.Tool                       { return h.tools }
func (h *fakeHost) WriteFinalize(_ context.Context, blocks []messagesdomain.Block, status, stopReason, errCode, errMsg string, in, out int) {
	h.fin.blocks = blocks
	h.fin.status = status
	h.fin.stopReason = stopReason
	h.fin.errCode = errCode
	h.fin.errMsg = errMsg
	h.fin.in = in
	h.fin.out = out
	h.fin.called++
}

type errHistoryHost struct{ fakeHost }

func (errHistoryHost) LoadHistory(context.Context) ([]llminfra.LLMMessage, error) {
	return nil, errors.New("db down")
}

type reminderHost struct {
	*fakeHost
	reminders []string
}

func (h reminderHost) SystemReminders(context.Context) []string { return h.reminders }

type autoActivateHost struct {
	*fakeHost
	lazy []toolapp.Tool // activated when a tool not in the base set is requested
}

func (h *autoActivateHost) TryActivateForTool(_ context.Context, name string) []toolapp.Tool {
	for _, t := range h.lazy {
		if t.Name() == name {
			h.tools = append(h.tools, h.lazy...)
			return h.tools
		}
	}
	return nil
}

// fakeTool implements toolapp.Tool. record receives the call's stripped args; err makes
// Execute fail (for the error-storm path).
//
// fakeTool 实现 toolapp.Tool。record 收到调用的剥离后 args；err 让 Execute 失败（错误风暴路径）。
type fakeTool struct {
	name   string
	result string
	err    error
}

func (t fakeTool) Name() string                        { return t.name }
func (t fakeTool) Description() string                 { return "fake tool" }
func (t fakeTool) Parameters() json.RawMessage         { return json.RawMessage(`{"type":"object"}`) }
func (t fakeTool) ValidateInput(json.RawMessage) error { return nil }
func (t fakeTool) Execute(_ context.Context, _ string) (string, error) {
	if t.err != nil {
		return "", t.err
	}
	return t.result, nil
}

var _ toolapp.Tool = fakeTool{}

// --- event builders --------------------------------------------------------

func textEv(s string) llminfra.StreamEvent {
	return llminfra.StreamEvent{Type: llminfra.EventText, Delta: s}
}
func toolStartEv(idx int, id, name string) llminfra.StreamEvent {
	return llminfra.StreamEvent{Type: llminfra.EventToolStart, ToolIndex: idx, ToolID: id, ToolName: name}
}
func toolDeltaEv(idx int, args string) llminfra.StreamEvent {
	return llminfra.StreamEvent{Type: llminfra.EventToolDelta, ToolIndex: idx, ArgsDelta: args}
}
func finishEv() llminfra.StreamEvent {
	return llminfra.StreamEvent{Type: llminfra.EventFinish, InputTokens: 10, OutputTokens: 5}
}

// --- Run -------------------------------------------------------------------

func TestRun_SingleTextTurn(t *testing.T) {
	client := &fakeClient{scripts: [][]llminfra.StreamEvent{{textEv("hello world"), finishEv()}}}
	host := &fakeHost{history: []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: "hi"}}}

	res := Run(context.Background(), host, client, llminfra.Request{}, 5, nil)

	if host.fin.called != 1 {
		t.Fatalf("WriteFinalize called %d times, want 1", host.fin.called)
	}
	if host.fin.status != messagesdomain.StatusCompleted || host.fin.stopReason != messagesdomain.StopReasonEndTurn {
		t.Fatalf("status=%q stopReason=%q, want completed/end_turn", host.fin.status, host.fin.stopReason)
	}
	if res.LastMessage != "hello world" {
		t.Fatalf("LastMessage=%q, want %q", res.LastMessage, "hello world")
	}
	if res.Steps != 1 || res.TokensIn != 10 || res.TokensOut != 5 {
		t.Fatalf("steps=%d in=%d out=%d, want 1/10/5", res.Steps, res.TokensIn, res.TokensOut)
	}
}

func TestRun_ToolThenText(t *testing.T) {
	client := &fakeClient{scripts: [][]llminfra.StreamEvent{
		{toolStartEv(0, "tc_1", "echo"), toolDeltaEv(0, `{"summary":"echoing","danger":"safe","msg":"x"}`), finishEv()},
		{textEv("done"), finishEv()},
	}}
	host := &fakeHost{
		history: []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: "go"}},
		tools:   []toolapp.Tool{fakeTool{name: "echo", result: "echoed!"}},
	}

	res := Run(context.Background(), host, client, llminfra.Request{}, 5, nil)

	if res.Steps != 2 {
		t.Fatalf("steps=%d, want 2", res.Steps)
	}
	if host.fin.status != messagesdomain.StatusCompleted {
		t.Fatalf("status=%q, want completed", host.fin.status)
	}
	// allBlocks = tool_call + tool_result (step1) + text (step2).
	var sawToolResult, sawText bool
	for _, b := range host.fin.blocks {
		if b.Type == messagesdomain.BlockTypeToolResult && b.Content == "echoed!" {
			sawToolResult = true
		}
		if b.Type == messagesdomain.BlockTypeText && b.Content == "done" {
			sawText = true
		}
	}
	if !sawToolResult || !sawText {
		t.Fatalf("blocks missing pieces: toolResult=%v text=%v (%+v)", sawToolResult, sawText, host.fin.blocks)
	}
}

func TestRun_MaxStepsReached(t *testing.T) {
	// Every step returns a tool call → the loop never naturally ends.
	loopStep := []llminfra.StreamEvent{toolStartEv(0, "tc_1", "echo"), toolDeltaEv(0, `{"summary":"s","danger":"safe"}`), finishEv()}
	client := &fakeClient{scripts: [][]llminfra.StreamEvent{loopStep, loopStep, loopStep}}
	host := &fakeHost{tools: []toolapp.Tool{fakeTool{name: "echo", result: "ok"}}}

	res := Run(context.Background(), host, client, llminfra.Request{}, 2, nil)

	if host.fin.stopReason != messagesdomain.StopReasonMaxSteps || host.fin.errCode != "MAX_STEPS_REACHED" {
		t.Fatalf("stopReason=%q errCode=%q, want max_steps/MAX_STEPS_REACHED", host.fin.stopReason, host.fin.errCode)
	}
	if host.fin.status != messagesdomain.StatusError || res.Steps != 2 {
		t.Fatalf("status=%q steps=%d, want error/2", host.fin.status, res.Steps)
	}
	// F66: the returned Result must carry the real terminal cause (not just WriteFinalize) so the
	// agent execution record surfaces it instead of a generic "agent loop error".
	if res.ErrCode != "MAX_STEPS_REACHED" || res.ErrMsg == "" {
		t.Fatalf("Result should carry ErrCode/ErrMsg; got code=%q msg=%q", res.ErrCode, res.ErrMsg)
	}
}

func TestRun_ToolErrorStorm(t *testing.T) {
	loopStep := []llminfra.StreamEvent{toolStartEv(0, "tc_1", "boom"), toolDeltaEv(0, `{"summary":"s","danger":"safe"}`), finishEv()}
	scripts := make([][]llminfra.StreamEvent, 5)
	for i := range scripts {
		scripts[i] = loopStep
	}
	client := &fakeClient{scripts: scripts}
	host := &fakeHost{tools: []toolapp.Tool{fakeTool{name: "boom", err: errors.New("kaboom")}}}

	Run(context.Background(), host, client, llminfra.Request{}, 10, nil)

	if host.fin.errCode != "TOOL_ERROR_STORM" {
		t.Fatalf("errCode=%q, want TOOL_ERROR_STORM", host.fin.errCode)
	}
	// 3 consecutive all-fail turns is the cap.
	if !strings.Contains(host.fin.errMsg, "3 consecutive") {
		t.Fatalf("errMsg=%q, want mention of 3 consecutive", host.fin.errMsg)
	}
}

func TestRun_LoadHistoryError(t *testing.T) {
	host := &errHistoryHost{}
	client := &fakeClient{}

	res := Run(context.Background(), host, client, llminfra.Request{}, 5, nil)

	if res.Status != messagesdomain.StatusError || host.fin.errCode != "INTERNAL_ERROR" {
		t.Fatalf("status=%q errCode=%q, want error/INTERNAL_ERROR", res.Status, host.fin.errCode)
	}
	if client.calls != 0 {
		t.Fatalf("client called %d times, want 0 (aborted before stream)", client.calls)
	}
}

func TestRun_RemindersInjectedEachStep(t *testing.T) {
	client := &fakeClient{scripts: [][]llminfra.StreamEvent{{textEv("ok"), finishEv()}}}
	base := &fakeHost{history: []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: "hi"}}}
	host := reminderHost{fakeHost: base, reminders: []string{"todo: ship it"}}

	Run(context.Background(), host, client, llminfra.Request{}, 5, nil)

	if len(client.captured) != 1 {
		t.Fatalf("captured %d requests, want 1", len(client.captured))
	}
	msgs := client.captured[0]
	last := msgs[len(msgs)-1]
	if last.Role != llminfra.RoleUser || !strings.Contains(last.Content, "<system-reminder>") || !strings.Contains(last.Content, "ship it") {
		t.Fatalf("last message not the injected reminder: %+v", last)
	}
	// Persisted history (base.history) must stay clean — reminder is transient.
	if len(base.history) != 1 {
		t.Fatalf("base history mutated to len %d, want 1", len(base.history))
	}
}

func TestRun_AutoActivateLazyTool(t *testing.T) {
	client := &fakeClient{scripts: [][]llminfra.StreamEvent{
		{toolStartEv(0, "tc_1", "lazy_tool"), toolDeltaEv(0, `{"summary":"s","danger":"safe"}`), finishEv()},
		{textEv("activated and ran"), finishEv()},
	}}
	base := &fakeHost{} // base tool set is EMPTY — lazy_tool only reachable via activation
	host := &autoActivateHost{fakeHost: base, lazy: []toolapp.Tool{fakeTool{name: "lazy_tool", result: "lazy ran"}}}

	Run(context.Background(), host, client, llminfra.Request{}, 5, nil)

	var ran bool
	for _, b := range host.fin.blocks {
		if b.Type == messagesdomain.BlockTypeToolResult && b.Content == "lazy ran" {
			ran = true
		}
	}
	if !ran {
		t.Fatalf("lazy tool was not auto-activated + executed: %+v", host.fin.blocks)
	}
}

// --- tool dispatch ---------------------------------------------------------

func TestPartitionByExecutionGroup(t *testing.T) {
	calls := []messagesdomain.ToolCallData{
		{ID: "a", ExecutionGroup: 1},
		{ID: "b", ExecutionGroup: 1},
		{ID: "c", ExecutionGroup: 0}, // auto-grouped, sorts after explicit
		{ID: "d", ExecutionGroup: 2},
	}
	batches := partitionByExecutionGroup(calls)
	// groups: 1 -> [a,b], 2 -> [d], auto(1000) -> [c]. Order: 1, 2, 1000.
	if len(batches) != 3 {
		t.Fatalf("got %d batches, want 3", len(batches))
	}
	if len(batches[0].items) != 2 || batches[0].items[0].tc.ID != "a" {
		t.Fatalf("batch0 wrong: %+v", batches[0].items)
	}
	if len(batches[1].items) != 1 || batches[1].items[0].tc.ID != "d" {
		t.Fatalf("batch1 wrong: %+v", batches[1].items)
	}
	if len(batches[2].items) != 1 || batches[2].items[0].tc.ID != "c" {
		t.Fatalf("batch2 (auto) wrong: %+v", batches[2].items)
	}
}

func TestRunTools_ResultsIndexAligned(t *testing.T) {
	// Two calls in the same execution group run concurrently; results must map back to input order.
	calls := []messagesdomain.ToolCallData{
		{ID: "tc_a", Name: "a", ExecutionGroup: 1},
		{ID: "tc_b", Name: "b", ExecutionGroup: 1},
	}
	byName := map[string]toolapp.Tool{
		"a": fakeTool{name: "a", result: "result-a"},
		"b": fakeTool{name: "b", result: "result-b"},
	}
	blocks := runTools(context.Background(), calls, byName, zap.NewNop())
	if len(blocks) != 2 {
		t.Fatalf("got %d blocks, want 2", len(blocks))
	}
	if blocks[0].Content != "result-a" || blocks[0].ParentBlockID != "tc_a" {
		t.Fatalf("block0 misaligned: %+v", blocks[0])
	}
	if blocks[1].Content != "result-b" || blocks[1].ParentBlockID != "tc_b" {
		t.Fatalf("block1 misaligned: %+v", blocks[1])
	}
}

func TestExecuteTool_NotFound(t *testing.T) {
	out, errMsg, ok := executeTool(context.Background(), nil, "ghost", []byte(`{}`), zap.NewNop())
	if ok || !strings.Contains(out, "not found") || errMsg == "" {
		t.Fatalf("nil tool: out=%q errMsg=%q ok=%v", out, errMsg, ok)
	}
}

// --- stream assembly + danger ---------------------------------------------

func TestStreamLLM_AssemblesBlocksAndDanger(t *testing.T) {
	client := &fakeClient{scripts: [][]llminfra.StreamEvent{{
		llminfra.StreamEvent{Type: llminfra.EventReasoning, Delta: "thinking"},
		textEv("answer"),
		toolStartEv(0, "tc_1", "writer"),
		toolDeltaEv(0, `{"summary":"writing","danger":"dangerous","path":"/x"}`),
		finishEv(),
	}}}

	noBuild := func(string) (toolapp.BuildSpec, bool) { return toolapp.BuildSpec{}, false }
	blocks, calls, stop, _, in, out := streamLLM(context.Background(), client, llminfra.Request{}, noBuild, nil)

	if stop != messagesdomain.StopReasonEndTurn || in != 10 || out != 5 {
		t.Fatalf("stop=%q in=%d out=%d", stop, in, out)
	}
	if len(calls) != 1 || calls[0].Danger != "dangerous" || calls[0].Summary != "writing" {
		t.Fatalf("tool call danger/summary not parsed: %+v", calls)
	}
	// Business args must have the standard fields stripped.
	if _, hasDanger := calls[0].Arguments["danger"]; hasDanger {
		t.Fatalf("danger leaked into business args: %+v", calls[0].Arguments)
	}
	if calls[0].Arguments["path"] != "/x" {
		t.Fatalf("business arg missing: %+v", calls[0].Arguments)
	}
	// reasoning + text + tool_call blocks; tool_call carries danger in attrs.
	var toolCallBlk *messagesdomain.Block
	for i := range blocks {
		if blocks[i].Type == messagesdomain.BlockTypeToolCall {
			toolCallBlk = &blocks[i]
		}
	}
	if toolCallBlk == nil || toolCallBlk.Attrs["danger"] != "dangerous" {
		t.Fatalf("tool_call block missing danger attr: %+v", blocks)
	}
}

// --- history transform -----------------------------------------------------

func TestBlocksToAssistantLLM(t *testing.T) {
	blocks := []messagesdomain.Block{
		{Type: messagesdomain.BlockTypeReasoning, Content: "hmm", Attrs: map[string]any{"signature": "sig1"}},
		{Type: messagesdomain.BlockTypeText, Content: "hi"},
		{ID: "tc_1", Type: messagesdomain.BlockTypeToolCall, Content: `{"x":1}`, Attrs: map[string]any{"tool": "echo"}},
		{Type: messagesdomain.BlockTypeToolResult, Content: "out", ParentBlockID: "tc_1"},
		{Type: messagesdomain.BlockTypeCompaction, Content: "dropme"},
	}
	msgs := BlocksToAssistantLLM(blocks)
	if len(msgs) != 2 {
		t.Fatalf("got %d msgs, want 2 (assistant + tool)", len(msgs))
	}
	a := msgs[0]
	if a.Role != llminfra.RoleAssistant || a.Content != "hi" || a.ReasoningContent != "hmm" || a.ReasoningSignature != "sig1" {
		t.Fatalf("assistant msg wrong: %+v", a)
	}
	if len(a.ToolCalls) != 1 || a.ToolCalls[0].Name != "echo" || a.ToolCalls[0].ID != "tc_1" {
		t.Fatalf("assistant tool calls wrong: %+v", a.ToolCalls)
	}
	if msgs[1].Role != llminfra.RoleTool || msgs[1].Content != "out" || msgs[1].ToolCallID != "tc_1" {
		t.Fatalf("tool msg wrong: %+v", msgs[1])
	}
}

func TestProjectToolResultContent_ContextRole(t *testing.T) {
	long := strings.Repeat("x", 500)
	cases := []struct {
		role string
		want string // substring expected
	}{
		{messagesdomain.ContextRoleHot, long},
		{messagesdomain.ContextRoleWarm, "truncated, 500 total bytes"},
		{messagesdomain.ContextRoleCold, "output omitted to save context"},
	}
	for _, c := range cases {
		b := messagesdomain.Block{
			Type: messagesdomain.BlockTypeToolResult, Content: long,
			ContextRole: c.role, Attrs: map[string]any{"tool": "reader"},
		}
		got := projectToolResultContent(b)
		if !strings.Contains(got, c.want) {
			t.Fatalf("role %q: got %q, want substring %q", c.role, truncate(got), c.want)
		}
	}
}

func TestInjectReminders_NoProviderUnchanged(t *testing.T) {
	host := &fakeHost{}
	history := []llminfra.LLMMessage{{Role: llminfra.RoleUser, Content: "hi"}}
	got := injectReminders(context.Background(), host, history)
	if len(got) != 1 {
		t.Fatalf("non-provider host should pass history through, got len %d", len(got))
	}
}

// --- helpers ---------------------------------------------------------------

func truncate(s string) string {
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}
