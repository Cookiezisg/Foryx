package eventlog

import (
	"context"
	"errors"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	eventloginfra "github.com/sunweilin/forgify/backend/internal/infra/eventlog"
	chatstore "github.com/sunweilin/forgify/backend/internal/infra/store/chat"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func setupCtx(t *testing.T) (context.Context, *eventloginfra.Bridge, Emitter) {
	t.Helper()
	br := eventloginfra.NewBridge(nil)
	em := New(br, nil, nil)
	ctx := context.Background()
	ctx = reqctxpkg.SetUserID(ctx, "u_test")
	ctx = reqctxpkg.WithConversationID(ctx, "cv_test")
	ctx = reqctxpkg.WithMessageID(ctx, "msg_test")
	ctx = With(ctx, em)
	return ctx, br, em
}

func subCtxFor(parent context.Context) context.Context {
	return reqctxpkg.SetUserID(parent, "u_test")
}

func TestEmitter_StartBlockReadsParentFromCtx(t *testing.T) {
	ctx, br, em := setupCtx(t)

	subCtx, cancel := context.WithCancel(subCtxFor(context.Background()))
	defer cancel()
	ch, cancelSub, _ := br.Subscribe(subCtx, 0)
	defer cancelSub()

	parentBlockID := "blk_parent"
	scoped := reqctxpkg.WithParentBlockID(ctx, parentBlockID)
	blockID := em.StartBlock(scoped, eventlogdomain.BlockTypeText, nil)
	if blockID == "" {
		t.Fatal("expected minted blockID, got empty")
	}

	env := <-ch
	bs, ok := env.Event.(eventlogdomain.BlockStart)
	if !ok {
		t.Fatalf("expected BlockStart, got %T", env.Event)
	}
	if bs.ParentID != parentBlockID {
		t.Errorf("ParentID: got %q, want %q", bs.ParentID, parentBlockID)
	}
	if bs.MessageID != "msg_test" {
		t.Errorf("MessageID: got %q, want msg_test", bs.MessageID)
	}
}

func TestEmitter_StartBlockFallsBackToMessageID(t *testing.T) {
	ctx, br, em := setupCtx(t)

	subCtx, cancel := context.WithCancel(subCtxFor(context.Background()))
	defer cancel()
	ch, cancelSub, _ := br.Subscribe(subCtx, 0)
	defer cancelSub()

	blockID := em.StartBlock(ctx, eventlogdomain.BlockTypeText, nil)
	if blockID == "" {
		t.Fatal("expected minted blockID")
	}

	env := <-ch
	bs := env.Event.(eventlogdomain.BlockStart)
	if bs.ParentID != "msg_test" {
		t.Errorf("ParentID fallback: got %q, want msg_test", bs.ParentID)
	}
}

func TestEmitter_DeltaAndStopBlock(t *testing.T) {
	ctx, br, em := setupCtx(t)

	subCtx, cancel := context.WithCancel(subCtxFor(context.Background()))
	defer cancel()
	ch, cancelSub, _ := br.Subscribe(subCtx, 0)
	defer cancelSub()

	blockID := em.StartBlock(ctx, eventlogdomain.BlockTypeText, nil)
	em.DeltaBlock(ctx, blockID, "hello")
	em.DeltaBlock(ctx, blockID, " world")
	em.StopBlock(ctx, blockID, eventlogdomain.StatusCompleted, nil)

	want := []string{"block_start", "block_delta", "block_delta", "block_stop"}
	for i, w := range want {
		env := <-ch
		if env.Event.EventType() != w {
			t.Errorf("event %d: got %s, want %s", i, env.Event.EventType(), w)
		}
	}
}

func TestEmitter_StopBlockWithError(t *testing.T) {
	ctx, br, em := setupCtx(t)
	subCtx, cancel := context.WithCancel(subCtxFor(context.Background()))
	defer cancel()
	ch, cancelSub, _ := br.Subscribe(subCtx, 0)
	defer cancelSub()

	blockID := em.StartBlock(ctx, eventlogdomain.BlockTypeText, nil)
	<-ch
	em.StopBlock(ctx, blockID, eventlogdomain.StatusError, errors.New("boom"))
	env := <-ch
	bs := env.Event.(eventlogdomain.BlockStop)
	if bs.Error != "boom" {
		t.Errorf("Error: got %q, want %q", bs.Error, "boom")
	}
	if bs.Status != eventlogdomain.StatusError {
		t.Errorf("Status: got %q, want error", bs.Status)
	}
}

func TestEmitter_MissingConversationIDSkipsEmit(t *testing.T) {
	br := eventloginfra.NewBridge(nil)
	em := New(br, nil, nil)
	ctx := context.Background()
	em.EmitBlockStart(ctx, "blk_t1", "msg_t1", "msg_t1", eventlogdomain.BlockTypeText, nil)
	_ = br
}

func TestFrom_ReturnsNoopWhenAbsent(t *testing.T) {
	em := From(context.Background())
	em.DeltaBlock(context.Background(), "blk_x", "ignored")
	em.StopBlock(context.Background(), "blk_x", eventlogdomain.StatusCompleted, nil)
	em.StopMessage(context.Background(), "msg_x", eventlogdomain.StatusCompleted, "", "", "", 0, 0)
}

func setupDBCtx(t *testing.T) (context.Context, *chatstore.Store, Emitter) {
	t.Helper()
	database, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(database) })
	if err := dbinfra.Migrate(database, &chatdomain.Block{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := chatstore.New(database)

	br := eventloginfra.NewBridge(nil)
	em := New(br, repo, nil)
	ctx := context.Background()
	ctx = reqctxpkg.SetUserID(ctx, "u_db")
	ctx = reqctxpkg.WithConversationID(ctx, "cv_db")
	ctx = reqctxpkg.WithMessageID(ctx, "msg_db")
	ctx = With(ctx, em)
	return ctx, repo, em
}

func TestEmitBlockStart_DualWritesToDB(t *testing.T) {
	ctx, repo, em := setupDBCtx(t)

	em.EmitBlockStart(ctx, "blk_t1", "msg_db", "msg_db", eventlogdomain.BlockTypeText, nil)

	got, err := repo.GetBlock(ctx, "blk_t1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ConversationID != "cv_db" {
		t.Errorf("conversationID: got %q, want cv_db", got.ConversationID)
	}
	if got.MessageID != "msg_db" {
		t.Errorf("messageID: got %q, want msg_db", got.MessageID)
	}
	if got.ParentBlockID != "" {
		t.Errorf("parentBlockID: got %q, want empty (top-level)", got.ParentBlockID)
	}
	if got.Type != eventlogdomain.BlockTypeText {
		t.Errorf("type: got %q, want text", got.Type)
	}
	if got.Status != eventlogdomain.StatusStreaming {
		t.Errorf("status: got %q, want streaming", got.Status)
	}
	if got.Seq != 1 {
		t.Errorf("seq: got %d, want 1", got.Seq)
	}
}

func TestEmitBlockStart_DualWritesNestedParent(t *testing.T) {
	ctx, repo, em := setupDBCtx(t)

	em.EmitBlockStart(ctx, "blk_parent", "msg_db", "msg_db", eventlogdomain.BlockTypeToolCall, nil)
	em.EmitBlockStart(ctx, "blk_child", "blk_parent", "msg_db", eventlogdomain.BlockTypeProgress, nil)

	child, _ := repo.GetBlock(ctx, "blk_child")
	if child.ParentBlockID != "blk_parent" {
		t.Errorf("nested parent: got %q, want blk_parent", child.ParentBlockID)
	}
}

func TestDeltaBlock_DualWritesAppend(t *testing.T) {
	ctx, repo, em := setupDBCtx(t)

	em.EmitBlockStart(ctx, "blk_t1", "msg_db", "msg_db", eventlogdomain.BlockTypeText, nil)
	em.DeltaBlock(ctx, "blk_t1", "hello")
	em.DeltaBlock(ctx, "blk_t1", " world")

	got, _ := repo.GetBlock(ctx, "blk_t1")
	if got.Content != "hello world" {
		t.Errorf("content: got %q, want %q", got.Content, "hello world")
	}
}

func TestStopBlock_DualWritesFinalize(t *testing.T) {
	ctx, repo, em := setupDBCtx(t)

	em.EmitBlockStart(ctx, "blk_t1", "msg_db", "msg_db", eventlogdomain.BlockTypeText, nil)
	em.DeltaBlock(ctx, "blk_t1", "all done")
	em.StopBlock(ctx, "blk_t1", eventlogdomain.StatusCompleted, nil)

	got, _ := repo.GetBlock(ctx, "blk_t1")
	if got.Status != eventlogdomain.StatusCompleted {
		t.Errorf("status: got %q, want completed", got.Status)
	}
	if got.Error != "" {
		t.Errorf("error: got %q, want empty", got.Error)
	}
}

func TestStopBlock_DualWritesError(t *testing.T) {
	ctx, repo, em := setupDBCtx(t)

	em.EmitBlockStart(ctx, "blk_t1", "msg_db", "msg_db", eventlogdomain.BlockTypeText, nil)
	em.StopBlock(ctx, "blk_t1", eventlogdomain.StatusError, errors.New("boom"))

	got, _ := repo.GetBlock(ctx, "blk_t1")
	if got.Status != eventlogdomain.StatusError {
		t.Errorf("status: got %q, want error", got.Status)
	}
	if got.Error != "boom" {
		t.Errorf("error: got %q, want boom", got.Error)
	}
}

func TestEmitter_AttrsJSONMarshalled(t *testing.T) {
	ctx, repo, em := setupDBCtx(t)

	em.EmitBlockStart(ctx, "blk_t1", "msg_db", "msg_db", eventlogdomain.BlockTypeToolCall,
		map[string]any{"tool": "Read", "summary": "fetching"})

	got, _ := repo.GetBlock(ctx, "blk_t1")
	if tool, _ := got.Attrs["tool"].(string); tool != "Read" {
		t.Errorf("attrs.tool = %q, want Read (full attrs: %#v)", tool, got.Attrs)
	}
}

func TestProtocolContract_ChatRoundtrip(t *testing.T) {
	ctx, repo, em := setupDBCtx(t)

	br := em.(*emitter).bridge
	subCtx, cancel := context.WithCancel(reqctxpkg.SetUserID(context.Background(), "u_db"))
	defer cancel()
	ch, cancelSub, err := br.Subscribe(subCtx, 0)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer cancelSub()

	em.EmitMessageStart(ctx, "msg_db", "assistant", "", nil)

	textID := "blk_text_1"
	em.EmitBlockStart(ctx, textID, "msg_db", "msg_db", eventlogdomain.BlockTypeText, nil)
	em.DeltaBlock(ctx, textID, "Hello, ")
	em.DeltaBlock(ctx, textID, "world.")
	em.StopBlock(ctx, textID, eventlogdomain.StatusCompleted, nil)

	tcID := "tc_abc123"
	em.EmitBlockStart(ctx, tcID, "msg_db", "msg_db", eventlogdomain.BlockTypeToolCall,
		map[string]any{"tool": "Read"})
	em.DeltaBlock(ctx, tcID, `{"path":"/etc/hosts"}`)
	em.StopBlock(ctx, tcID, eventlogdomain.StatusCompleted, nil)

	resultID := "blk_result_1"
	em.EmitBlockStart(ctx, resultID, tcID, "msg_db", eventlogdomain.BlockTypeToolResult, nil)
	em.DeltaBlock(ctx, resultID, "127.0.0.1 localhost\n")
	em.StopBlock(ctx, resultID, eventlogdomain.StatusCompleted, nil)

	em.StopMessage(ctx, "msg_db", eventlogdomain.StatusCompleted, "end_turn", "", "", 100, 200)

	expected := 12
	got := make([]eventlogdomain.Envelope, 0, expected)
	for i := 0; i < expected; i++ {
		select {
		case env := <-ch:
			got = append(got, env)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for envelope #%d (got %d)", i+1, len(got))
		}
	}

	for i, env := range got {
		want := int64(i + 1)
		if env.Seq != want {
			t.Errorf("env[%d].Seq: got %d, want %d", i, env.Seq, want)
		}
	}

	known := map[string]bool{}
	for i, env := range got {
		switch e := env.Event.(type) {
		case eventlogdomain.MessageStart:
			known[e.ID] = true
		case eventlogdomain.BlockStart:
			if !known[e.ParentID] {
				t.Errorf("env[%d] BlockStart parent %q referenced before it existed",
					i, e.ParentID)
			}
			if !known[e.MessageID] {
				t.Errorf("env[%d] BlockStart messageId %q referenced before it existed",
					i, e.MessageID)
			}
			known[e.ID] = true
		case eventlogdomain.BlockDelta:
			if !known[e.ID] {
				t.Errorf("env[%d] BlockDelta id %q has no prior block_start", i, e.ID)
			}
		case eventlogdomain.BlockStop:
			if !known[e.ID] {
				t.Errorf("env[%d] BlockStop id %q has no prior block_start", i, e.ID)
			}
		case eventlogdomain.MessageStop:
			if !known[e.ID] {
				t.Errorf("env[%d] MessageStop id %q has no prior message_start", i, e.ID)
			}
		}
	}

	var foundToolCallStart bool
	for _, env := range got {
		if bs, ok := env.Event.(eventlogdomain.BlockStart); ok &&
			bs.BlockType == eventlogdomain.BlockTypeToolCall {
			if bs.ID != "tc_abc123" {
				t.Errorf("tool_call BlockStart ID: got %q, want tc_abc123 (LLM-supplied)", bs.ID)
			}
			foundToolCallStart = true
		}
	}
	if !foundToolCallStart {
		t.Error("never saw tool_call BlockStart")
	}

	for _, env := range got {
		if bs, ok := env.Event.(eventlogdomain.BlockStart); ok &&
			bs.BlockType == eventlogdomain.BlockTypeToolResult {
			if bs.ParentID != "tc_abc123" {
				t.Errorf("tool_result parent: got %q, want tc_abc123", bs.ParentID)
			}
		}
	}

	textRow, err := repo.GetBlock(ctx, textID)
	if err != nil {
		t.Fatalf("get text block: %v", err)
	}
	if textRow.Content != "Hello, world." {
		t.Errorf("text content: got %q, want %q", textRow.Content, "Hello, world.")
	}
	if textRow.Status != eventlogdomain.StatusCompleted {
		t.Errorf("text status: got %q, want completed", textRow.Status)
	}
	if textRow.ParentBlockID != "" {
		t.Errorf("text parent_block_id: got %q, want empty (top-level)", textRow.ParentBlockID)
	}

	tcRow, _ := repo.GetBlock(ctx, tcID)
	if tcRow.Content != `{"path":"/etc/hosts"}` {
		t.Errorf("tool_call content: got %q, want JSON args", tcRow.Content)
	}
	if tool, _ := tcRow.Attrs["tool"].(string); tool != "Read" {
		t.Errorf("tool_call attrs.tool = %q, want Read (full: %#v)", tool, tcRow.Attrs)
	}

	resultRow, _ := repo.GetBlock(ctx, resultID)
	if resultRow.ParentBlockID != tcID {
		t.Errorf("tool_result parent: got %q, want %q (nested under tool_call)", resultRow.ParentBlockID, tcID)
	}
}

