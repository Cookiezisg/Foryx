package chat

import (
	"context"
	"database/sql"
	"errors"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"go.uber.org/zap"

	conversationdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	mentiondomain "github.com/sunweilin/forgify/backend/internal/domain/mention"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	streamdomain "github.com/sunweilin/forgify/backend/internal/domain/stream"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	messagesstore "github.com/sunweilin/forgify/backend/internal/infra/store/messages"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// --- fakes -----------------------------------------------------------------

// fakeClient replays one scripted StreamEvent slice per Stream call (one per ReAct step). An
// optional gate blocks every Stream call until released — used to pin a turn in-flight while a
// STREAM_IN_PROGRESS race is exercised.
//
// fakeClient 每次 Stream 调用回放一份脚本（每 ReAct 步一份）。可选 gate 阻塞每次 Stream 直到释放
// ——用于把一个回合钉在飞行中以触发 STREAM_IN_PROGRESS。
type fakeClient struct {
	script []llminfra.StreamEvent
	gate   chan struct{}
}

func (c *fakeClient) Stream(_ context.Context, _ llminfra.Request) iter.Seq[llminfra.StreamEvent] {
	return func(yield func(llminfra.StreamEvent) bool) {
		if c.gate != nil {
			<-c.gate
		}
		for _, ev := range c.script {
			if !yield(ev) {
				return
			}
		}
	}
}

// textTurn scripts a plain text answer with token accounting and no tool calls — the loop
// finalizes after one step with end_turn.
//
// textTurn 脚本一个带 token 记账、无工具调用的纯文本回答——loop 一步后以 end_turn 终态。
func textTurn() []llminfra.StreamEvent {
	return []llminfra.StreamEvent{
		{Type: llminfra.EventText, Delta: "Hello "},
		{Type: llminfra.EventText, Delta: "world"},
		{Type: llminfra.EventFinish, FinishReason: "stop", InputTokens: 10, OutputTokens: 5},
	}
}

// fakeResolver hands back a fixed bundle wrapping the fake client.
type fakeResolver struct{ client llminfra.Client }

func (r fakeResolver) ResolveChat(_ context.Context, _ *modeldomain.ModelRef) (Bundle, error) {
	return Bundle{
		Client:   r.client,
		Request:  llminfra.Request{ModelID: "fake-model"},
		Provider: "fake",
	}, nil
}

func (r fakeResolver) ResolveUtility(_ context.Context) (Bundle, error) {
	return Bundle{
		Client:   r.client,
		Request:  llminfra.Request{ModelID: "fake-utility"},
		Provider: "fake",
	}, nil
}

// fakeConvs returns one fixed conversation for any id.
type fakeConvs struct {
	conv *conversationdomain.Conversation
}

func (c fakeConvs) Get(_ context.Context, id string) (*conversationdomain.Conversation, error) {
	cp := *c.conv
	cp.ID = id
	return &cp, nil
}

// recordBridge captures every published event and signals each Close's node id on a buffered
// channel, so a test can wait for a specific turn's message_stop without polling.
//
// recordBridge 记录每个发布的事件，并在缓冲 channel 上报每个 Close 的节点 id，使测试无需轮询即可
// 等到某回合的 message_stop。
type recordBridge struct {
	mu     sync.Mutex
	events []streamdomain.Event
	closes chan string
}

func newRecordBridge() *recordBridge { return &recordBridge{closes: make(chan string, 64)} }

func (b *recordBridge) Publish(_ context.Context, e streamdomain.Event) (streamdomain.Envelope, error) {
	b.mu.Lock()
	b.events = append(b.events, e)
	b.mu.Unlock()
	if _, ok := e.Frame.(streamdomain.Close); ok {
		select {
		case b.closes <- e.ID:
		default:
		}
	}
	return streamdomain.Envelope{}, nil
}

func (b *recordBridge) Subscribe(context.Context, int64) (<-chan streamdomain.Envelope, func(), error) {
	return nil, func() {}, nil
}

func (b *recordBridge) frameFor(id string, want any) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range b.events {
		if e.ID != id {
			continue
		}
		switch want.(type) {
		case streamdomain.Open:
			if _, ok := e.Frame.(streamdomain.Open); ok {
				return true
			}
		case streamdomain.Close:
			if _, ok := e.Frame.(streamdomain.Close); ok {
				return true
			}
		}
	}
	return false
}

// --- harness ---------------------------------------------------------------

func newStore(t *testing.T) messagesdomain.Repository {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, stmt := range messagesstore.Schema {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	return messagesstore.New(ormpkg.Open(sqlDB))
}

func newSvc(t *testing.T, client llminfra.Client, bridge streamdomain.Bridge) (*Service, messagesdomain.Repository) {
	t.Helper()
	store := newStore(t)
	deps := Deps{
		Conversations: fakeConvs{conv: &conversationdomain.Conversation{SystemPrompt: "be concise"}},
		Resolver:      fakeResolver{client: client},
		Bridge:        bridge,
	}
	return New(store, deps, zap.NewNop()), store
}

func ctxWS(id string) context.Context {
	return reqctxpkg.SetWorkspaceID(context.Background(), id)
}

// waitClose drains the bridge's close signals until it sees msgID, or fails after timeout.
//
// waitClose 抽取 bridge 的 close 信号直到看到 msgID，超时则失败。
func waitClose(t *testing.T, b *recordBridge, msgID string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case id := <-b.closes:
			if id == msgID {
				return
			}
		case <-deadline:
			t.Fatalf("timed out waiting for message_stop on %s", msgID)
		}
	}
}

// --- tests -----------------------------------------------------------------

func TestSend_EndToEnd(t *testing.T) {
	bridge := newRecordBridge()
	svc, store := newSvc(t, &fakeClient{script: textTurn()}, bridge)
	ctx := ctxWS("ws_1")

	asstID, err := svc.Send(ctx, "cv_1", SendInput{Content: "hi there"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	waitClose(t, bridge, asstID)

	// Assistant turn persisted with terminal state, token accounting, provenance, and its text block.
	got, err := store.GetMessage(ctx, asstID)
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if got.Status != messagesdomain.StatusCompleted || got.StopReason != messagesdomain.StopReasonEndTurn {
		t.Fatalf("assistant terminal wrong: status=%q stop=%q", got.Status, got.StopReason)
	}
	if got.InputTokens != 10 || got.OutputTokens != 5 || got.Provider != "fake" || got.ModelID != "fake-model" {
		t.Fatalf("token/provenance wrong: %+v", got)
	}
	if len(got.Blocks) != 1 || got.Blocks[0].Type != messagesdomain.BlockTypeText || got.Blocks[0].Content != "Hello world" {
		t.Fatalf("assistant blocks wrong: %+v", got.Blocks)
	}

	// The user turn was persisted too (thread = user + assistant; find by role to avoid relying
	// on same-instant created_at tiebreaks).
	thread, err := store.LoadThread(ctx, "cv_1")
	if err != nil {
		t.Fatalf("LoadThread: %v", err)
	}
	if len(thread) != 2 {
		t.Fatalf("want 2 thread messages, got %d", len(thread))
	}
	var user *messagesdomain.Message
	for _, m := range thread {
		if m.Role == messagesdomain.RoleUser {
			user = m
		}
	}
	if user == nil || userText(user) != "hi there" {
		t.Fatalf("user turn wrong: %+v", thread)
	}

	// SSE: assistant message_start (Open) + message_stop (Close) both reached the bridge.
	if !bridge.frameFor(asstID, streamdomain.Open{}) || !bridge.frameFor(asstID, streamdomain.Close{}) {
		t.Fatalf("missing message_start/stop frames for %s", asstID)
	}
}

func TestSend_EmptyContent(t *testing.T) {
	svc, _ := newSvc(t, &fakeClient{script: textTurn()}, newRecordBridge())
	if _, err := svc.Send(ctxWS("ws_1"), "cv_1", SendInput{}); !errors.Is(err, ErrEmptyContent) {
		t.Fatalf("want ErrEmptyContent, got %v", err)
	}
}

func TestSend_StreamInProgress(t *testing.T) {
	gate := make(chan struct{})
	bridge := newRecordBridge()
	svc, _ := newSvc(t, &fakeClient{script: textTurn(), gate: gate}, bridge)
	ctx := ctxWS("ws_1")

	// First Send is picked up and blocks in Stream (gate). Subsequent Sends fill the buffer and
	// then get rejected — we only assert that the guard fires within a bounded number of attempts.
	//
	// 第一个 Send 被取走并阻塞在 Stream（gate）。后续 Send 填满缓冲后被拒——只断言 guard 在有界次数内触发。
	if _, err := svc.Send(ctx, "cv_1", SendInput{Content: "first"}); err != nil {
		t.Fatalf("first Send: %v", err)
	}
	gotInProgress := false
	for range queueCapacity + 3 {
		if _, err := svc.Send(ctx, "cv_1", SendInput{Content: "more"}); errors.Is(err, ErrStreamInProgress) {
			gotInProgress = true
			break
		}
	}
	close(gate) // release the blocked turn(s) so goroutines drain
	if !gotInProgress {
		t.Fatal("expected STREAM_IN_PROGRESS once a turn is in flight + buffer full")
	}
}

func TestBuildSystemPrompt_Sections(t *testing.T) {
	svc, _ := newSvc(t, &fakeClient{script: textTurn()}, newRecordBridge())
	conv := &conversationdomain.Conversation{SystemPrompt: "speak like a pirate"}
	prompt := svc.buildSystemPrompt(reqctxpkg.SetLocale(ctxWS("ws_1"), reqctxpkg.LocaleZhCN), conv)

	for _, want := range []string{
		`<section name="identity">`,
		`<section name="how_to_work">`,
		`<section name="tools">`,
		`<section name="user_system_prompt">`,
		"speak like a pirate",
		`<section name="environment">`,
		"Reply in Chinese.",
		`<section name="critical_rules">`,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("system prompt missing %q\n---\n%s", want, prompt)
		}
	}
}

func TestLoadHistory_Composition(t *testing.T) {
	bridge := newRecordBridge()
	svc, store := newSvc(t, &fakeClient{script: textTurn()}, bridge)
	ctx := ctxWS("ws_1")

	// Seed a prior completed user + assistant turn, then the in-flight assistant turn. Ordered
	// ids (msg_1 < msg_2 < msg_3) make LoadThread's (created_at, id) ordering deterministic for
	// same-instant creates.
	u := &messagesdomain.Message{ID: "msg_1", ConversationID: "cv_1", Role: messagesdomain.RoleUser, Status: messagesdomain.StatusCompleted}
	if err := store.CreateMessage(ctx, u, []messagesdomain.Block{{Type: messagesdomain.BlockTypeText, Content: "earlier q"}}); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	a := &messagesdomain.Message{ID: "msg_2", ConversationID: "cv_1", Role: messagesdomain.RoleAssistant, Status: messagesdomain.StatusCompleted}
	if err := store.CreateMessage(ctx, a, []messagesdomain.Block{{Type: messagesdomain.BlockTypeText, Content: "earlier answer"}}); err != nil {
		t.Fatalf("seed assistant: %v", err)
	}
	// The in-flight assistant turn (no blocks) that LoadHistory must skip.
	inflight := &messagesdomain.Message{ID: "msg_3", ConversationID: "cv_1", Role: messagesdomain.RoleAssistant, Status: messagesdomain.StatusStreaming}
	if err := store.CreateMessage(ctx, inflight, nil); err != nil {
		t.Fatalf("seed inflight: %v", err)
	}

	h := &chatHost{svc: svc, conversationID: "cv_1", assistantMsgID: "msg_3", summary: "older stuff was discussed"}
	hist, err := h.LoadHistory(ctx)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	// summary (1) + user (1) + assistant (1) = 3; the in-flight assistant turn is skipped.
	if len(hist) != 3 {
		t.Fatalf("want 3 history messages, got %d: %+v", len(hist), hist)
	}
	if !strings.Contains(hist[0].Content, "older stuff was discussed") {
		t.Fatalf("summary not prepended: %q", hist[0].Content)
	}
	if hist[1].Role != llminfra.RoleUser || hist[1].Content != "earlier q" {
		t.Fatalf("user message wrong: %+v", hist[1])
	}
	if hist[2].Role != llminfra.RoleAssistant || hist[2].Content != "earlier answer" {
		t.Fatalf("assistant message wrong: %+v", hist[2])
	}
}

// --- R0056: mention + cancel ----------------------------------------------

type fakeMentionResolver struct {
	typ mentiondomain.MentionType
	ref *mentiondomain.Reference
	err error
}

func (r fakeMentionResolver) Type() mentiondomain.MentionType { return r.typ }
func (r fakeMentionResolver) Resolve(context.Context, string) (*mentiondomain.Reference, error) {
	return r.ref, r.err
}

func TestMention_ResolveFreeze(t *testing.T) {
	svc, _ := newSvc(t, &fakeClient{script: textTurn()}, newRecordBridge())
	svc.RegisterMentionResolver(fakeMentionResolver{
		typ: mentiondomain.MentionDocument,
		ref: &mentiondomain.Reference{Name: "Doc X", Content: "doc body"},
	})

	snaps := svc.resolveMentions(ctxWS("ws_1"), []mentiondomain.MentionInput{
		{Type: mentiondomain.MentionDocument, ID: "doc_1"},
		{Type: mentiondomain.MentionFunction, ID: "fn_1"}, // no resolver registered → stub
	})
	if len(snaps) != 2 {
		t.Fatalf("want 2 snapshots, got %d", len(snaps))
	}
	if snaps[0]["name"] != "Doc X" || snaps[0]["content"] != "doc body" {
		t.Fatalf("resolved snapshot wrong: %+v", snaps[0])
	}
	if snaps[1]["name"] != "(unavailable)" {
		t.Fatalf("missing-resolver should stub, got: %+v", snaps[1])
	}
}

func TestMention_Render(t *testing.T) {
	m := &messagesdomain.Message{Attrs: map[string]any{attrMentions: []map[string]any{
		{"type": "document", "id": "doc_1", "name": "Doc X", "content": "doc body"},
	}}}
	out := renderMentions(m)
	for _, want := range []string{"<mentions>", `type="document"`, `name="Doc X"`, "doc body", "</mentions>"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
	if renderMentions(&messagesdomain.Message{}) != "" {
		t.Fatal("no mentions should render empty")
	}
}

func TestCancel_NoQueue(t *testing.T) {
	svc, _ := newSvc(t, &fakeClient{script: textTurn()}, newRecordBridge())
	if err := svc.Cancel(ctxWS("ws_1"), "cv_none"); err != nil {
		t.Fatalf("Cancel on a conversation with no active queue should be a no-op, got %v", err)
	}
}

func TestFinalizeCancelled(t *testing.T) {
	bridge := newRecordBridge()
	svc, store := newSvc(t, &fakeClient{script: textTurn()}, bridge)
	ctx := ctxWS("ws_1")

	// Seed a streaming assistant turn (as Send opens one before the loop runs).
	m := &messagesdomain.Message{ID: "msg_a", ConversationID: "cv_1", Role: messagesdomain.RoleAssistant, Status: messagesdomain.StatusStreaming}
	if err := store.CreateMessage(ctx, m, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc.finalizeCancelled("cv_1", "msg_a", "ws_1")

	got, err := store.GetMessage(ctx, "msg_a")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if got.Status != messagesdomain.StatusCancelled || got.StopReason != messagesdomain.StopReasonCancelled {
		t.Fatalf("turn not cancelled: %+v", got)
	}
	if !bridge.frameFor("msg_a", streamdomain.Close{}) {
		t.Fatal("cancelled turn must still push message_stop")
	}
}

// --- R0057: auto-title + usage + system-prompt-preview ---------------------

type fakeTitler struct{ called chan string }

func (f *fakeTitler) SetAutoTitle(_ context.Context, _, title string) error {
	select {
	case f.called <- title:
	default:
	}
	return nil
}

func titleTurn() []llminfra.StreamEvent {
	return []llminfra.StreamEvent{
		{Type: llminfra.EventText, Delta: "My Conversation Title"},
		{Type: llminfra.EventFinish, FinishReason: "stop", InputTokens: 3, OutputTokens: 4},
	}
}

func TestAutoTitle_FirstTurn(t *testing.T) {
	store := newStore(t)
	titler := &fakeTitler{called: make(chan string, 1)}
	svc := New(store, Deps{
		Conversations: fakeConvs{conv: &conversationdomain.Conversation{}}, // untitled
		Resolver:      fakeResolver{client: &fakeClient{script: titleTurn()}},
		Bridge:        newRecordBridge(),
		Titler:        titler,
	}, zap.NewNop())

	if _, err := svc.Send(ctxWS("ws_1"), "cv_1", SendInput{Content: "hi"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	select {
	case title := <-titler.called:
		if title != "My Conversation Title" {
			t.Fatalf("auto-title wrong: %q", title)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("auto-title was not invoked for the first turn")
	}
}

func TestUsage_SumTokens(t *testing.T) {
	bridge := newRecordBridge()
	svc, store := newSvc(t, &fakeClient{script: textTurn()}, bridge)
	ctx := ctxWS("ws_1")

	for i, id := range []string{"msg_1", "msg_2"} {
		m := &messagesdomain.Message{
			ID: id, ConversationID: "cv_1", Role: messagesdomain.RoleAssistant,
			Status: messagesdomain.StatusCompleted, InputTokens: 10 * (i + 1), OutputTokens: i + 1,
		}
		if err := store.CreateMessage(ctx, m, nil); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	in, out, err := svc.Usage(ctx, "cv_1")
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if in != 30 || out != 3 { // 10+20, 1+2
		t.Fatalf("token sum wrong: in=%d out=%d", in, out)
	}
}

func TestLoadHistory_ExcludesSubagent(t *testing.T) {
	bridge := newRecordBridge()
	svc, store := newSvc(t, &fakeClient{script: textTurn()}, bridge)
	ctx := ctxWS("ws_1")

	// A normal user turn + a subagent sub-message (SubagentID set) in the same conversation.
	u := &messagesdomain.Message{ID: "msg_1", ConversationID: "cv_1", Role: messagesdomain.RoleUser, Status: messagesdomain.StatusCompleted}
	if err := store.CreateMessage(ctx, u, []messagesdomain.Block{{Type: messagesdomain.BlockTypeText, Content: "parent question"}}); err != nil {
		t.Fatalf("user: %v", err)
	}
	sub := &messagesdomain.Message{ID: "msg_2", ConversationID: "cv_1", SubagentID: "subagt_x", Role: messagesdomain.RoleAssistant, Status: messagesdomain.StatusCompleted}
	if err := store.CreateMessage(ctx, sub, []messagesdomain.Block{{Type: messagesdomain.BlockTypeText, Content: "subagent internal trace"}}); err != nil {
		t.Fatalf("sub: %v", err)
	}

	h := &chatHost{svc: svc, conversationID: "cv_1", assistantMsgID: "msg_none"}
	hist, err := h.LoadHistory(ctx)
	if err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}
	// Only the parent user turn — the subagent sub-message is excluded from the parent's LLM history.
	if len(hist) != 1 || hist[0].Role != llminfra.RoleUser || hist[0].Content != "parent question" {
		t.Fatalf("history should contain only the parent turn, got %+v", hist)
	}
	for _, m := range hist {
		if strings.Contains(m.Content, "subagent internal") {
			t.Fatal("subagent trace leaked into the parent's LLM history")
		}
	}
}

func TestSystemPromptPreview(t *testing.T) {
	svc, _ := newSvc(t, &fakeClient{script: textTurn()}, newRecordBridge())
	prompt, err := svc.SystemPromptPreview(ctxWS("ws_1"), "cv_1")
	if err != nil {
		t.Fatalf("SystemPromptPreview: %v", err)
	}
	for _, want := range []string{`<section name="identity">`, `<section name="critical_rules">`, "be concise"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("preview missing %q:\n%s", want, prompt)
		}
	}
}
