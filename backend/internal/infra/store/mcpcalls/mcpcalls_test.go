// mcpcalls_test.go — integration tests for mcpcallstore.Store using
// in-memory SQLite.

package mcpcalls

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const userAlice = "u-alice"

func newStore(t *testing.T) *Store {
	t.Helper()
	gdb, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(gdb) })
	if err := dbinfra.Migrate(gdb, AutoMigrateModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(gdb)
}

func ctxFor(uid string) context.Context {
	return reqctxpkg.SetUserID(context.Background(), uid)
}

func mkCall(id, uid, server, tool, status string, ms int64) *mcpdomain.Call {
	now := time.Now().UTC()
	return &mcpdomain.Call{
		ID: id, UserID: uid, Status: status, TriggeredBy: "chat",
		Input: map[string]any{"k": "v"}, Output: "ok",
		ElapsedMs: ms,
		StartedAt: now, EndedAt: now,
		ServerName: server, ToolName: tool,
	}
}

func TestSaveAndGet(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	if err := s.SaveCall(ctx, mkCall("mcl1", userAlice, "everything", "echo", mcpdomain.CallStatusOK, 50)); err != nil {
		t.Fatalf("SaveCall: %v", err)
	}
	got, err := s.GetCallByID(ctx, "mcl1")
	if err != nil {
		t.Fatalf("GetCallByID: %v", err)
	}
	if got.ServerName != "everything" || got.ToolName != "echo" {
		t.Errorf("server/tool wrong: %+v", got)
	}
}

func TestGet_CrossUser_NotFound(t *testing.T) {
	s := newStore(t)
	_ = s.SaveCall(ctxFor(userAlice), mkCall("mcl1", userAlice, "x", "y", "ok", 1))

	_, err := s.GetCallByID(ctxFor("u-bob"), "mcl1")
	if !errors.Is(err, mcpdomain.ErrCallNotFound) {
		t.Errorf("expected ErrCallNotFound, got %v", err)
	}
}

func TestList_PaginationAndFilter(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	for i := 0; i < 5; i++ {
		_ = s.SaveCall(ctx, mkCall(fmt.Sprintf("mcl%d", i), userAlice, "everything", "tool"+fmt.Sprint(i%2), mcpdomain.CallStatusOK, int64(i)))
		time.Sleep(time.Millisecond)
	}
	rows, _, err := s.ListCalls(ctx, mcpdomain.CallFilter{Limit: 3})
	if err != nil {
		t.Fatalf("ListCalls: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("page len = %d, want 3", len(rows))
	}
	// Filter by tool name.
	rows, _, _ = s.ListCalls(ctx, mcpdomain.CallFilter{ToolName: "tool0"})
	for _, r := range rows {
		if r.ToolName != "tool0" {
			t.Errorf("filter leaked: %s", r.ToolName)
		}
	}
}

func TestComputeAggregates(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)
	_ = s.SaveCall(ctx, mkCall("mcl1", userAlice, "x", "t", mcpdomain.CallStatusOK, 100))
	_ = s.SaveCall(ctx, mkCall("mcl2", userAlice, "x", "t", mcpdomain.CallStatusFailed, 200))
	_ = s.SaveCall(ctx, mkCall("mcl3", userAlice, "x", "t", mcpdomain.CallStatusTimeout, 300))

	agg, err := s.ComputeAggregates(ctx, mcpdomain.CallFilter{ServerName: "x"})
	if err != nil {
		t.Fatalf("ComputeAggregates: %v", err)
	}
	if agg.OKCount != 1 || agg.FailedCount != 1 || agg.TimeoutCount != 1 {
		t.Errorf("counts wrong: %+v", agg)
	}
	if agg.AvgElapsedMs != 200 {
		t.Errorf("avg = %d, want 200", agg.AvgElapsedMs)
	}
}
