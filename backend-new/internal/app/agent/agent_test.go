package agent

import (
	"context"
	"database/sql"
	"errors"
	"iter"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	agentdomain "github.com/sunweilin/forgify/backend/internal/domain/agent"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	agentstore "github.com/sunweilin/forgify/backend/internal/infra/store/agent"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
	schemapkg "github.com/sunweilin/forgify/backend/internal/pkg/schema"
)

// fakeLLMClient replays one scripted step of stream events.
type fakeLLMClient struct{ events []llminfra.StreamEvent }

func (c *fakeLLMClient) Stream(_ context.Context, _ llminfra.Request) iter.Seq[llminfra.StreamEvent] {
	return func(yield func(llminfra.StreamEvent) bool) {
		for _, ev := range c.events {
			if !yield(ev) {
				return
			}
		}
	}
}

type fakeResolver struct{ client llminfra.Client }

func (r fakeResolver) ResolveAgent(context.Context, *modeldomain.ModelRef) (LLMBundle, error) {
	return LLMBundle{Client: r.client, Request: llminfra.Request{ModelID: "test-model"}}, nil
}

type fakeKnowledge struct{}

func (fakeKnowledge) BuildKnowledgePrefix(context.Context, []string) (string, error) { return "", nil }

func newSvc(t *testing.T) (*Service, context.Context) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, stmt := range agentstore.Schema {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	svc := NewService(agentstore.New(ormpkg.Open(sqlDB)), nil, zap.NewNop())
	return svc, reqctxpkg.SetWorkspaceID(context.Background(), "ws_1")
}

func TestService_CreateEditRevert(t *testing.T) {
	svc, ctx := newSvc(t)

	a, v1, err := svc.Create(ctx, CreateInput{Name: "alpha", Config: Config{Prompt: "p1"}})
	if err != nil || v1.Version != 1 {
		t.Fatalf("create: %v v%d", err, v1.Version)
	}
	v2, err := svc.Edit(ctx, EditInput{ID: a.ID, Config: Config{Prompt: "p2"}})
	if err != nil || v2.Version != 2 {
		t.Fatalf("edit: %v v%d", err, v2.Version)
	}
	if got, _ := svc.Get(ctx, a.ID); got.ActiveVersionID != v2.ID {
		t.Fatalf("active should be v2 after edit")
	}
	rv, err := svc.Revert(ctx, a.ID, 1)
	if err != nil || rv.ID != v1.ID {
		t.Fatalf("revert: %v", err)
	}
	if got, _ := svc.Get(ctx, a.ID); got.ActiveVersionID != v1.ID {
		t.Fatalf("active should be v1 after revert")
	}
}

func TestService_CreateRejectsAgentRef(t *testing.T) {
	svc, ctx := newSvc(t)
	_, _, err := svc.Create(ctx, CreateInput{Name: "x", Config: Config{
		Prompt: "p", Tools: []agentdomain.ToolRef{{Ref: "ag_other"}},
	}})
	if !errors.Is(err, agentdomain.ErrToolsAgentRef) {
		t.Fatalf("want ErrToolsAgentRef, got %v", err)
	}
}

// TestService_InvokeRunsLoopAndRecords: with a fake LLM (no real network), InvokeAgent runs the
// real ReAct loop, returns the final output, and records one execution. This is the whole
// invoke-with-loop surface, fake-tested.
//
// TestService_InvokeRunsLoopAndRecords：假 LLM（无网络）下 InvokeAgent 跑真 ReAct loop、返回最终
// 输出、落一条 execution。这是 invoke-接-loop 全面，fake 测。
func TestService_InvokeRunsLoopAndRecords(t *testing.T) {
	svc, ctx := newSvc(t)
	svc.SetInvokeDeps(InvokeDeps{
		Resolver: fakeResolver{client: &fakeLLMClient{events: []llminfra.StreamEvent{
			{Type: llminfra.EventText, Delta: "approve"},
			{Type: llminfra.EventFinish, InputTokens: 10, OutputTokens: 5},
		}}},
		Tools:     func() []toolapp.Tool { return nil },
		Knowledge: fakeKnowledge{},
	})

	a, _, err := svc.Create(ctx, CreateInput{
		Name: "judge",
		Config: Config{
			Prompt:       "judge the PR",
			Outputs:      []schemapkg.Field{{Name: "decision", Type: schemapkg.TypeString, Description: "one of: approve, reject"}},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	res, err := svc.InvokeAgent(ctx, InvokeInput{AgentID: a.ID, TriggeredBy: agentdomain.TriggeredByChat})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK, got %+v", res)
	}
	if res.Output != "approve" {
		t.Fatalf("expected output 'approve', got %v", res.Output)
	}
	if res.ExecutionID == "" {
		t.Fatalf("expected an execution to be recorded")
	}

	sr, err := svc.SearchExecutions(ctx, agentdomain.ExecutionFilter{AgentID: a.ID})
	if err != nil || sr.Count != 1 || sr.Aggregates.OKCount != 1 {
		t.Fatalf("execution not recorded as ok: %v %+v", err, sr)
	}
}

func TestService_InvokeWithoutDepsFails(t *testing.T) {
	svc, ctx := newSvc(t)
	a, _, _ := svc.Create(ctx, CreateInput{Name: "x", Config: Config{Prompt: "p"}})
	if _, err := svc.InvokeAgent(ctx, InvokeInput{AgentID: a.ID}); err == nil {
		t.Fatal("expected error when invoke deps not configured")
	}
}
