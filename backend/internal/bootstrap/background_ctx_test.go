package bootstrap

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/glebarez/go-sqlite"

	flowrundomain "github.com/sunweilin/forgify/backend/internal/domain/flowrun"
	triggerdomain "github.com/sunweilin/forgify/backend/internal/domain/trigger"
	flowrunstore "github.com/sunweilin/forgify/backend/internal/infra/store/flowrun"
	triggerstore "github.com/sunweilin/forgify/backend/internal/infra/store/trigger"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// TestBackgroundPaths_RequireWorkspaceSeeding locks the P3-1 regression: the background entry
// points (drainLoop's DrainFirings/CheckTimeouts, Boot's handler/mcp/ReattachActive) read
// workspace-scoped tables, so they MUST run under a per-workspace seeded ctx — a bare
// context.Background() fails with MISSING_WORKSPACE_ID and the whole automation path is dead
// while looking "merely degraded" in logs. This test proves both halves at the store layer:
// the drain queries fail on a bare ctx and succeed (returning the seeded rows) on a
// Detached(ws) ctx — exactly the contract forEachWorkspace provides.
//
// TestBackgroundPaths_RequireWorkspaceSeeding 锁定 P3-1 回归：后台入口（drainLoop 的
// DrainFirings/CheckTimeouts、Boot 的 handler/mcp/ReattachActive）读 workspace 隔离表，必须在
// 逐 workspace 播种的 ctx 下跑——裸 context.Background() 会以 MISSING_WORKSPACE_ID 失败，整条
// 自动化链路死掉、日志里却只像"轻微降级"。本测试在 store 层证明两半：drain 查询在裸 ctx 失败、
// 在 Detached(ws) ctx 成功（读到种入的行）——正是 forEachWorkspace 提供的契约。
func TestBackgroundPaths_RequireWorkspaceSeeding(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, schema := range [][]string{flowrunstore.Schema, triggerstore.Schema} {
		for _, stmt := range schema {
			if _, err := sqlDB.Exec(stmt); err != nil {
				t.Fatalf("schema: %v", err)
			}
		}
	}
	db := ormpkg.Open(sqlDB)
	frs := flowrunstore.New(db)
	trs := triggerstore.New(db)

	wsCtx := reqctxpkg.Detached("ws_1")

	// Seed one pending firing + one parked node under ws_1.
	// 在 ws_1 下种一条 pending firing + 一条 parked 节点。
	if _, err := trs.AppendFiring(wsCtx, &triggerdomain.Firing{
		TriggerID: "trg_1", WorkflowID: "wf_1", ActivationID: "tra_1",
		Payload: map[string]any{}, DedupKey: "k1",
	}); err != nil {
		t.Fatalf("seed firing: %v", err)
	}
	if _, err := frs.InsertNodeResult(wsCtx, &flowrundomain.FlowRunNode{
		FlowRunID: "fr_1", NodeID: "gate", Iteration: 0,
		Kind: "approval", Status: flowrundomain.NodeParked, Result: map[string]any{},
	}); err != nil {
		t.Fatalf("seed parked: %v", err)
	}

	// Bare Background ctx (the old drainLoop wiring) MUST fail — this failing is the alarm
	// that keeps anyone from wiring a background entry point without workspace seeding.
	// 裸 Background ctx（旧 drainLoop 接法）必须失败——这条失败就是警报，防止任何人再不播种就接后台入口。
	if _, err := trs.ListPendingFirings(context.Background(), 10); err == nil {
		t.Fatal("ListPendingFirings(Background) should fail without a workspace ctx")
	}
	if _, err := frs.ListParkedNodes(context.Background()); err == nil {
		t.Fatal("ListParkedNodes(Background) should fail without a workspace ctx")
	}

	// Detached(ws) ctx (the forEachWorkspace contract) sees the seeded rows.
	// Detached(ws) ctx（forEachWorkspace 的契约）读到种入的行。
	firings, err := trs.ListPendingFirings(wsCtx, 10)
	if err != nil || len(firings) != 1 {
		t.Fatalf("ListPendingFirings(Detached) = %d rows, err %v; want 1", len(firings), err)
	}
	parked, err := frs.ListParkedNodes(wsCtx)
	if err != nil || len(parked) != 1 {
		t.Fatalf("ListParkedNodes(Detached) = %d rows, err %v; want 1", len(parked), err)
	}
}
