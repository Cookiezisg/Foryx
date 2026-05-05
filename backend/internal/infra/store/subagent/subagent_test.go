// subagent_test.go — integration tests against in-memory SQLite per §T2.
// Covers run CRUD, AppendMessage seq monotonicity (incl. concurrency),
// per-conversation isolation, and replay-order Sort guarantees.
//
// subagent_test.go ——按 §T2 用 in-memory SQLite 做集成测试。覆盖 run CRUD、
// AppendMessage seq 单调（含并发）、跨对话隔离、replay 顺序保证。
package subagent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	gdb, err := dbinfra.Open(dbinfra.Config{DataDir: ""})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(gdb) })
	// SQLite :memory: gives each connection its own DB instance — pin to a
	// single conn so concurrent goroutines (TestAppendMessage_ConcurrentWithinRun)
	// see the same migrated tables.
	//
	// SQLite :memory: 每个连接独立实例——单连接让并发 goroutine 看到同一份
	// 迁移好的表。
	if sqlDB, err := gdb.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := dbinfra.Migrate(gdb,
		&subagentdomain.SubagentRun{},
		&subagentdomain.SubagentMessage{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(gdb)
}

func mkRun(id, convID, typ string) *subagentdomain.SubagentRun {
	now := time.Now().UTC()
	return &subagentdomain.SubagentRun{
		ID:                   id,
		ParentConversationID: convID,
		Type:                 typ,
		Prompt:               "find foo",
		Status:               subagentdomain.StatusRunning,
		StartedAt:            now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
}

func mkMsg(id, runID, role, text string) *subagentdomain.SubagentMessage {
	return &subagentdomain.SubagentMessage{
		ID:            id,
		SubagentRunID: runID,
		Role:          role,
		Blocks: []chatdomain.Block{
			{Type: chatdomain.BlockTypeText, Data: `{"text":"` + text + `"}`},
		},
		CreatedAt: time.Now().UTC(),
	}
}

// ── Run CRUD ─────────────────────────────────────────────────────────

func TestCreateAndGetRun(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := mkRun("sar_1", "cv_1", "Explore")
	if err := s.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	got, err := s.GetRun(ctx, "sar_1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if got.Type != "Explore" || got.ParentConversationID != "cv_1" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestGetRun_NotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetRun(context.Background(), "sar_missing")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("want gorm.ErrRecordNotFound, got %v", err)
	}
}

func TestUpdateRun_StatusTransition(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	r := mkRun("sar_2", "cv_1", "Plan")
	if err := s.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	end := time.Now().UTC()
	r.Status = subagentdomain.StatusCompleted
	r.EndedAt = &end
	r.TotalTokensIn = 1234
	r.TotalTokensOut = 567
	if err := s.UpdateRun(ctx, r); err != nil {
		t.Fatalf("UpdateRun: %v", err)
	}
	got, _ := s.GetRun(ctx, "sar_2")
	if got.Status != subagentdomain.StatusCompleted || got.TotalTokensIn != 1234 {
		t.Errorf("status/tokens not persisted: %+v", got)
	}
	if got.EndedAt == nil {
		t.Error("EndedAt not persisted")
	}
}

func TestListRunsByConversation_NewestFirst(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	older := mkRun("sar_old", "cv_x", "Explore")
	older.StartedAt = time.Now().Add(-time.Hour)
	newer := mkRun("sar_new", "cv_x", "Plan")
	newer.StartedAt = time.Now()
	other := mkRun("sar_other", "cv_y", "Explore") // different conv

	for _, r := range []*subagentdomain.SubagentRun{older, newer, other} {
		if err := s.CreateRun(ctx, r); err != nil {
			t.Fatalf("CreateRun: %v", err)
		}
	}

	rows, err := s.ListRunsByConversation(ctx, "cv_x")
	if err != nil {
		t.Fatalf("ListRunsByConversation: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows for cv_x, got %d", len(rows))
	}
	if rows[0].ID != "sar_new" || rows[1].ID != "sar_old" {
		t.Errorf("ordering wrong: %s, %s", rows[0].ID, rows[1].ID)
	}
}

func TestListRunsByConversation_Empty(t *testing.T) {
	s := newTestStore(t)
	rows, err := s.ListRunsByConversation(context.Background(), "cv_nothing")
	if err != nil {
		t.Fatalf("ListRunsByConversation: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}
}

func TestRun_TransientFields_NotPersisted(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := mkRun("sar_t", "cv_1", "Explore")
	r.LastToolCalled = "Grep"
	r.LastToolArgsBrief = `{"pattern":"foo"}`
	r.LastStepDurationMs = 421
	now := time.Now()
	r.LastStepAt = &now
	if err := s.CreateRun(ctx, r); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	got, _ := s.GetRun(ctx, "sar_t")
	if got.LastToolCalled != "" || got.LastToolArgsBrief != "" || got.LastStepDurationMs != 0 || got.LastStepAt != nil {
		t.Errorf("transient gorm:\"-\" fields leaked into DB: %+v", got)
	}
}

// ── Message CRUD + Seq ───────────────────────────────────────────────

func TestAppendMessage_SequentialSeq(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i, role := range []string{"user", "assistant", "tool"} {
		m := mkMsg("smm_"+string(rune('a'+i)), "sar_1", role, "msg")
		if err := s.AppendMessage(ctx, m); err != nil {
			t.Fatalf("AppendMessage[%d]: %v", i, err)
		}
		if m.Seq != i {
			t.Errorf("AppendMessage[%d] Seq = %d, want %d", i, m.Seq, i)
		}
	}
}

func TestAppendMessage_PerRunIsolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Two separate runs — each gets its own seq sequence starting at 0.
	// 两个独立 run——各自从 0 起 seq。
	for _, runID := range []string{"sar_a", "sar_b"} {
		for i := 0; i < 3; i++ {
			m := mkMsg("smm_"+runID+"_"+string(rune('0'+i)), runID, "user", "x")
			if err := s.AppendMessage(ctx, m); err != nil {
				t.Fatalf("AppendMessage: %v", err)
			}
			if m.Seq != i {
				t.Errorf("run %s msg %d seq = %d, want %d", runID, i, m.Seq, i)
			}
		}
	}
}

func TestAppendMessage_ConcurrentWithinRun(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	const N = 12
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			m := mkMsg("smm_c_"+string(rune('0'+i)), "sar_concurrent", "user", "x")
			if err := s.AppendMessage(ctx, m); err != nil {
				t.Errorf("concurrent AppendMessage: %v", err)
			}
		}(i)
	}
	wg.Wait()

	rows, err := s.ListMessagesByRun(ctx, "sar_concurrent")
	if err != nil {
		t.Fatalf("ListMessagesByRun: %v", err)
	}
	if len(rows) != N {
		t.Fatalf("want %d rows, got %d", N, len(rows))
	}
	// Each insert ran inside a transaction with SELECT MAX(seq)+1, so
	// after sorting by seq we must see 0..N-1 with no duplicates / gaps.
	// 每次插入在事务内 SELECT MAX(seq)+1，按 seq 排序后必须 0..N-1 无重无缺。
	seen := make(map[int]bool, N)
	for _, r := range rows {
		if seen[r.Seq] {
			t.Errorf("duplicate seq %d", r.Seq)
		}
		seen[r.Seq] = true
	}
	for i := 0; i < N; i++ {
		if !seen[i] {
			t.Errorf("missing seq %d", i)
		}
	}
}

func TestUpdateMessage_PreservesSeq_RewritesBlocks(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m := mkMsg("smm_u", "sar_u", "assistant", "first")
	if err := s.AppendMessage(ctx, m); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	originalSeq := m.Seq

	m.Blocks = []chatdomain.Block{
		{Type: chatdomain.BlockTypeText, Data: `{"text":"refined"}`},
		{Type: chatdomain.BlockTypeToolCall, Data: `{"id":"t1","name":"Grep"}`},
	}
	m.CompletionTokens = 42
	if err := s.UpdateMessage(ctx, m); err != nil {
		t.Fatalf("UpdateMessage: %v", err)
	}

	rows, _ := s.ListMessagesByRun(ctx, "sar_u")
	if len(rows) != 1 {
		t.Fatalf("want 1 row after update (no duplicate), got %d", len(rows))
	}
	if rows[0].Seq != originalSeq {
		t.Errorf("seq drifted: %d → %d", originalSeq, rows[0].Seq)
	}
	if len(rows[0].Blocks) != 2 || rows[0].CompletionTokens != 42 {
		t.Errorf("update did not persist: %+v", rows[0])
	}
}

func TestListMessagesByRun_OrderedBySeq(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		m := mkMsg("smm_o_"+string(rune('0'+i)), "sar_o", "user", "x")
		if err := s.AppendMessage(ctx, m); err != nil {
			t.Fatalf("AppendMessage: %v", err)
		}
	}
	rows, err := s.ListMessagesByRun(ctx, "sar_o")
	if err != nil {
		t.Fatalf("ListMessagesByRun: %v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("want 5 rows, got %d", len(rows))
	}
	for i, r := range rows {
		if r.Seq != i {
			t.Errorf("rows[%d].Seq = %d, want %d", i, r.Seq, i)
		}
	}
}

func TestListMessagesByRun_Empty(t *testing.T) {
	s := newTestStore(t)
	rows, err := s.ListMessagesByRun(context.Background(), "sar_nothing")
	if err != nil {
		t.Fatalf("ListMessagesByRun: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}
}

func TestListMessagesByRun_PerRunIsolation(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.AppendMessage(ctx, mkMsg("smm_a", "sar_a", "user", "x")); err != nil {
		t.Fatalf("AppendMessage a: %v", err)
	}
	if err := s.AppendMessage(ctx, mkMsg("smm_b", "sar_b", "user", "x")); err != nil {
		t.Fatalf("AppendMessage b: %v", err)
	}
	rowsA, _ := s.ListMessagesByRun(ctx, "sar_a")
	rowsB, _ := s.ListMessagesByRun(ctx, "sar_b")
	if len(rowsA) != 1 || rowsA[0].ID != "smm_a" {
		t.Errorf("rowsA wrong: %+v", rowsA)
	}
	if len(rowsB) != 1 || rowsB[0].ID != "smm_b" {
		t.Errorf("rowsB wrong: %+v", rowsB)
	}
}

func TestMessage_BlocksRoundTrip_ChatTypeReuse(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m := &subagentdomain.SubagentMessage{
		ID:            "smm_blocks",
		SubagentRunID: "sar_blocks",
		Role:          "assistant",
		Blocks: []chatdomain.Block{
			{Type: chatdomain.BlockTypeReasoning, Data: `{"text":"thinking..."}`},
			{Type: chatdomain.BlockTypeText, Data: `{"text":"answer"}`},
			{Type: chatdomain.BlockTypeToolCall, Data: `{"id":"t1","name":"Read","arguments":{}}`},
			{Type: chatdomain.BlockTypeToolResult, Data: `{"toolCallId":"t1","ok":true,"result":"data"}`},
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.AppendMessage(ctx, m); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	rows, _ := s.ListMessagesByRun(ctx, "sar_blocks")
	if len(rows) != 1 || len(rows[0].Blocks) != 4 {
		t.Fatalf("blocks not round-tripped: %+v", rows)
	}
	if rows[0].Blocks[0].Type != chatdomain.BlockTypeReasoning {
		t.Errorf("block[0] type = %q, want reasoning", rows[0].Blocks[0].Type)
	}
}
