package skill

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	relationdomain "github.com/sunweilin/anselm/backend/internal/domain/relation"
	skilldomain "github.com/sunweilin/anselm/backend/internal/domain/skill"
	skillfs "github.com/sunweilin/anselm/backend/internal/infra/fs/skill"
	agentstatepkg "github.com/sunweilin/anselm/backend/internal/pkg/agentstate"
	reqctxpkg "github.com/sunweilin/anselm/backend/internal/pkg/reqctx"
)

func ctxWS(id string) context.Context { return reqctxpkg.SetWorkspaceID(context.Background(), id) }

// fakeRunner stands in for the subagent fork port.
type fakeRunner struct {
	gotType   string
	gotPrompt string
	result    string
}

func (f *fakeRunner) Spawn(_ context.Context, agentType, prompt string) (string, error) {
	f.gotType = agentType
	f.gotPrompt = prompt
	return f.result, nil
}

// fakeSyncer records relation sync calls.
type fakeSyncer struct {
	outEdges []relationdomain.SyncEdge
	inEdges  []relationdomain.SyncEdge
	purged   []string
}

func (f *fakeSyncer) SyncOutgoing(_ context.Context, _, _ string, _ []string, edges []relationdomain.SyncEdge) error {
	f.outEdges = edges
	return nil
}
func (f *fakeSyncer) SyncIncoming(_ context.Context, _, _ string, _ []string, edges []relationdomain.SyncEdge) error {
	f.inEdges = edges
	return nil
}
func (f *fakeSyncer) PurgeEntity(_ context.Context, _, id string) error {
	f.purged = append(f.purged, id)
	return nil
}

func newService(t *testing.T, runner skilldomain.SubagentRunner) *Service {
	t.Helper()
	return NewService(skillfs.New(t.TempDir()), runner, nil, zap.NewNop())
}

func TestCreate_RejectsConflict(t *testing.T) {
	svc := newService(t, nil)
	ctx := ctxWS("ws_1")
	if _, err := svc.Create(ctx, SaveInput{Name: "dup", Description: "d", Body: "b"}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := svc.Create(ctx, SaveInput{Name: "dup", Description: "d2", Body: "b2"}); !errors.Is(err, skilldomain.ErrNameConflict) {
		t.Fatalf("duplicate name should be ErrNameConflict, got %v", err)
	}
}

func TestCreate_ForkRequiresAgent(t *testing.T) {
	svc := newService(t, nil)
	ctx := ctxWS("ws_1")
	_, err := svc.Create(ctx, SaveInput{Name: "f", Description: "d", Body: "b", Context: skilldomain.ContextFork})
	if !errors.Is(err, skilldomain.ErrForkRequiresAgent) {
		t.Fatalf("fork without agent should be ErrForkRequiresAgent, got %v", err)
	}
}

func TestReplace_NotFound(t *testing.T) {
	svc := newService(t, nil)
	_, err := svc.Replace(ctxWS("ws_1"), SaveInput{Name: "ghost", Description: "d", Body: "b"})
	if !errors.Is(err, skilldomain.ErrNotFound) {
		t.Fatalf("replace missing should be ErrNotFound, got %v", err)
	}
}

func TestActivate_Inline_SubstitutesAndSetsActiveSkill(t *testing.T) {
	svc := newService(t, nil)
	state := agentstatepkg.New()
	ctx := reqctxpkg.WithAgentState(ctxWS("ws_1"), state)
	if _, err := svc.Create(ctx, SaveInput{
		Name: "greet", Description: "greet", Body: "Hello $1 and $ARGUMENTS",
		AllowedTools: []string{"Read"}, Context: skilldomain.ContextInline,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	out, err := svc.Activate(ctx, "greet", []string{"world", "everyone"})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if out != "Hello world and world everyone" {
		t.Fatalf("substitution wrong: %q", out)
	}
	if state.ActiveSkill() != "greet" {
		t.Fatalf("activeSkill should be set to greet, got %q", state.ActiveSkill())
	}
	if !state.IsToolPreApprovedBySkill("Read") {
		t.Fatal("Read should be pre-approved by the active skill")
	}
	if state.IsToolPreApprovedBySkill("Write") {
		t.Fatal("Write must NOT be pre-approved (not in allowed-tools)")
	}
}

func TestActivate_Fork_NoRunner_Degrades(t *testing.T) {
	svc := newService(t, nil) // subagent runner = nil
	ctx := ctxWS("ws_1")
	if _, err := svc.Create(ctx, SaveInput{Name: "f", Description: "d", Body: "do it", Context: skilldomain.ContextFork, Agent: "general-purpose"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err := svc.Activate(ctx, "f", nil)
	if !errors.Is(err, skilldomain.ErrSubagentUnavailable) {
		t.Fatalf("fork without runner should degrade to ErrSubagentUnavailable, got %v", err)
	}
}

func TestActivate_Fork_WithRunner(t *testing.T) {
	runner := &fakeRunner{result: "subagent did it"}
	svc := newService(t, runner)
	ctx := ctxWS("ws_1")
	if _, err := svc.Create(ctx, SaveInput{Name: "f", Description: "d", Body: "task $ARGUMENTS", Context: skilldomain.ContextFork, Agent: "explore"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	out, err := svc.Activate(ctx, "f", []string{"x"})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if out != "subagent did it" {
		t.Fatalf("fork should return runner result, got %q", out)
	}
	if runner.gotType != "explore" {
		t.Fatalf("runner should get agent type 'explore', got %q", runner.gotType)
	}
	if runner.gotPrompt != "task x" {
		t.Fatalf("runner should get substituted prompt, got %q", runner.gotPrompt)
	}
}

func TestCatalogSource_FiltersDisabledModelInvocation(t *testing.T) {
	svc := newService(t, nil)
	ctx := ctxWS("ws_1")
	_, _ = svc.Create(ctx, SaveInput{Name: "visible", Description: "d", Body: "b"})
	_, _ = svc.Create(ctx, SaveInput{Name: "hidden", Description: "d", Body: "b", DisableModelInvocation: true})

	items, err := svc.AsCatalogSource().ListItems(ctx)
	if err != nil {
		t.Fatalf("listitems: %v", err)
	}
	if len(items) != 1 || items[0].Name != "visible" {
		t.Fatalf("disable-model-invocation skill must be hidden from catalog, got %+v", items)
	}
	if items[0].Source != "skill" {
		t.Fatalf("catalog item source should be 'skill', got %q", items[0].Source)
	}
}

func TestRelations_EquipEdgesFromAllowedTools(t *testing.T) {
	svc := newService(t, nil)
	syncer := &fakeSyncer{}
	svc.SetRelationSyncer(syncer)
	ctx := ctxWS("ws_1")
	_, err := svc.Create(ctx, SaveInput{
		Name: "s", Description: "d", Body: "b",
		AllowedTools: []string{"Read", "fn_abc", "hd_xyz", "mcp:server/tool"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// 仅 fn_/hd_ 建 equip 边；内置 Read 与冒号形式 mcp: 跳过
	if len(syncer.outEdges) != 2 {
		t.Fatalf("expected 2 equip edges (fn_/hd_), got %d: %+v", len(syncer.outEdges), syncer.outEdges)
	}
	for _, e := range syncer.outEdges {
		if e.Kind != relationdomain.KindEquip {
			t.Fatalf("edge should be equip, got %q", e.Kind)
		}
	}
}

func TestDelete_PurgesRelations(t *testing.T) {
	svc := newService(t, nil)
	syncer := &fakeSyncer{}
	svc.SetRelationSyncer(syncer)
	ctx := ctxWS("ws_1")
	_, _ = svc.Create(ctx, SaveInput{Name: "s", Description: "d", Body: "b"})

	if err := svc.Delete(ctx, "s"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(syncer.purged) != 1 || syncer.purged[0] != "s" {
		t.Fatalf("delete should purge relations for 's', got %+v", syncer.purged)
	}
	if _, err := svc.Get(ctx, "s"); !errors.Is(err, skilldomain.ErrNotFound) {
		t.Fatalf("deleted skill should be NotFound, got %v", err)
	}
}

// TestCreate_RejectsBodyFrontmatter — round-12 skilluse lane: a skill body that itself opens with a
// YAML frontmatter block double-frontmattered the SKILL.md, silently dropping the body's allowedTools.
// The body must now be content-only (frontmatter comes from the args); a lone --- thematic break is fine.
func TestCreate_RejectsBodyFrontmatter(t *testing.T) {
	svc := newService(t, nil)
	ctx := ctxWS("ws_1")
	bodyFM := "---\nname: sneaky\nallowed-tools: [Bash]\n---\nDo the thing."
	if _, err := svc.Create(ctx, SaveInput{Name: "fm", Description: "d", Body: bodyFM}); !errors.Is(err, skilldomain.ErrInvalidFrontmatter) {
		t.Fatalf("a body opening with its own frontmatter block must be rejected, got %v", err)
	}
	if _, err := svc.Create(ctx, SaveInput{Name: "plain", Description: "d", Body: "Just the instructions."}); err != nil {
		t.Fatalf("a normal body must be accepted, got %v", err)
	}
	if _, err := svc.Create(ctx, SaveInput{Name: "rule", Description: "d", Body: "---\nA section after a thematic break, no closing fence."}); err != nil {
		t.Fatalf("a lone --- thematic break (no closing fence) must be accepted, got %v", err)
	}
}
