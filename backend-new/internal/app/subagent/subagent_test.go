package subagent

import (
	"context"
	"database/sql"
	"encoding/json"
	"iter"
	"slices"
	"testing"

	_ "github.com/glebarez/go-sqlite"
	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	messagesdomain "github.com/sunweilin/forgify/backend/internal/domain/messages"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	messagesstore "github.com/sunweilin/forgify/backend/internal/infra/store/messages"
	ormpkg "github.com/sunweilin/forgify/backend/internal/pkg/orm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// --- fakes -----------------------------------------------------------------

type fakeClient struct{ script []llminfra.StreamEvent }

func (c *fakeClient) Stream(_ context.Context, _ llminfra.Request) iter.Seq[llminfra.StreamEvent] {
	return func(yield func(llminfra.StreamEvent) bool) {
		for _, ev := range c.script {
			if !yield(ev) {
				return
			}
		}
	}
}

func textTurn(text string) []llminfra.StreamEvent {
	return []llminfra.StreamEvent{
		{Type: llminfra.EventText, Delta: text},
		{Type: llminfra.EventFinish, FinishReason: "stop", InputTokens: 7, OutputTokens: 9},
	}
}

type fakeResolver struct{ client llminfra.Client }

func (r fakeResolver) Resolve(_ context.Context) (Bundle, error) {
	return Bundle{Client: r.client, Request: llminfra.Request{ModelID: "fake-model"}, Provider: "fake"}, nil
}

type fakeTool struct{ name string }

func (f fakeTool) Name() string                                    { return f.name }
func (f fakeTool) Description() string                             { return f.name }
func (f fakeTool) Parameters() json.RawMessage                     { return json.RawMessage(`{"type":"object"}`) }
func (f fakeTool) ValidateInput(json.RawMessage) error             { return nil }
func (f fakeTool) Execute(context.Context, string) (string, error) { return "", nil }

type fakeTools struct{ tools []toolapp.Tool }

func (f fakeTools) Tools() []toolapp.Tool { return f.tools }

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

func ctxWS(id string) context.Context { return reqctxpkg.SetWorkspaceID(context.Background(), id) }

// --- registry --------------------------------------------------------------

func TestRegistry_BuiltInTypes(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"Explore", "Plan", "general-purpose"} {
		if _, ok := r.Get(name); !ok {
			t.Fatalf("missing built-in type %q", name)
		}
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatal("unknown type should not resolve")
	}
}

func TestFilterTools(t *testing.T) {
	all := []toolapp.Tool{fakeTool{"Read"}, fakeTool{"Grep"}, fakeTool{"Write"}, fakeTool{"Subagent"}}

	explore, _ := NewRegistry().Get("Explore")
	got := names(filterTools(explore, all))
	// Explore whitelist keeps Read + Grep, drops Write (not whitelisted) and Subagent (recursion).
	if len(got) != 2 || !has(got, "Read") || !has(got, "Grep") || has(got, "Subagent") || has(got, "Write") {
		t.Fatalf("Explore filter wrong: %v", got)
	}

	gp, _ := NewRegistry().Get("general-purpose")
	got = names(filterTools(gp, all))
	// general-purpose keeps everything EXCEPT Subagent (recursion guard).
	if len(got) != 3 || has(got, "Subagent") {
		t.Fatalf("general-purpose filter wrong: %v", got)
	}
}

// --- Spawn end-to-end ------------------------------------------------------

func TestSpawn_PersistsSubMessage(t *testing.T) {
	store := newStore(t)
	svc := New(Deps{
		Messages: store,
		Resolver: fakeResolver{client: &fakeClient{script: textTurn("explored: found it")}},
		Tools:    fakeTools{tools: []toolapp.Tool{fakeTool{"Read"}}},
	}, zap.NewNop())

	ctx := reqctxpkg.SetConversationID(ctxWS("ws_1"), "cv_1")
	ctx = reqctxpkg.SetToolCallID(ctx, "blk_tc") // simulates loop seeding the spawning tool_call id

	result, err := svc.Spawn(ctx, "Explore", "find the thing")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if result != "explored: found it" {
		t.Fatalf("result wrong: %q", result)
	}

	thread, err := store.LoadThread(ctx, "cv_1")
	if err != nil {
		t.Fatalf("LoadThread: %v", err)
	}
	if len(thread) != 1 {
		t.Fatalf("want 1 sub-message, got %d", len(thread))
	}
	m := thread[0]
	if m.SubagentID == "" {
		t.Fatalf("sub-message must be tagged with a SubagentID: %+v", m)
	}
	if m.Role != messagesdomain.RoleAssistant || m.Status != messagesdomain.StatusCompleted {
		t.Fatalf("sub-message terminal wrong: %+v", m)
	}
	if anchor, _ := m.Attrs[attrParentBlockID].(string); anchor != "blk_tc" {
		t.Fatalf("sub-message must anchor at the spawning tool_call: %v", m.Attrs)
	}
	if len(m.Blocks) != 1 || m.Blocks[0].Content != "explored: found it" {
		t.Fatalf("sub-message blocks wrong: %+v", m.Blocks)
	}
}

func TestSpawn_RecursionRefused(t *testing.T) {
	svc := New(Deps{
		Messages: newStore(t),
		Resolver: fakeResolver{client: &fakeClient{script: textTurn("x")}},
		Tools:    fakeTools{},
	}, zap.NewNop())

	// A ctx already inside a subagent run → spawning is refused (depth 1).
	ctx := reqctxpkg.SetSubagentID(reqctxpkg.SetConversationID(ctxWS("ws_1"), "cv_1"), "subagt_parent")
	if _, err := svc.Spawn(ctx, "Explore", "nested"); err == nil {
		t.Fatal("a subagent must not be able to spawn another subagent")
	}
}

func TestSpawn_UnknownType(t *testing.T) {
	svc := New(Deps{Messages: newStore(t), Resolver: fakeResolver{client: &fakeClient{}}, Tools: fakeTools{}}, zap.NewNop())
	if _, err := svc.Spawn(reqctxpkg.SetConversationID(ctxWS("ws_1"), "cv_1"), "Nope", "x"); err == nil {
		t.Fatal("unknown subagent type should error")
	}
}

func names(tools []toolapp.Tool) []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.Name()
	}
	return out
}

func has(ss []string, s string) bool { return slices.Contains(ss, s) }
