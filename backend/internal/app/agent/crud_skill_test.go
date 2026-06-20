package agent

import (
	"context"
	"errors"
	"testing"

	toolapp "github.com/sunweilin/anselm/backend/internal/app/tool"
	agentdomain "github.com/sunweilin/anselm/backend/internal/domain/agent"
)

// fakeSkillGuide resolves only the names it was told exist (mirrors skillapp.Guide returning a
// not-found error for an unknown name).
type fakeSkillGuide struct{ known map[string]bool }

func (f fakeSkillGuide) Guide(_ context.Context, name string) (string, error) {
	if f.known[name] {
		return "## guide for " + name, nil
	}
	return "", errors.New("skill not found")
}

// TestCreateEdit_RejectsDanglingSkill: round-3 skillagent lane — create_agent/edit_agent accepted a
// non-existent skill name and persisted a dangling ref that failed only on the first invoke
// ("skill not found"), building a dead-on-arrival agent. The mounted skill is now validated eagerly
// against the SAME SkillGuide invoke uses, so an accepted agent resolves its skill at invoke.
func TestCreateEdit_RejectsDanglingSkill(t *testing.T) {
	svc, ctx := newSvc(t)
	svc.SetInvokeDeps(InvokeDeps{Skill: fakeSkillGuide{known: map[string]bool{"real-skill": true}}})

	// A non-existent skill is rejected at CREATE (was: accepted, dead until first invoke).
	if _, _, err := svc.Create(ctx, CreateInput{Name: "ghost", Config: Config{Prompt: "p", Skill: "nope"}}); !errors.Is(err, agentdomain.ErrSkillNotFound) {
		t.Fatalf("create with a non-existent skill must reject ErrSkillNotFound, got: %v", err)
	}
	// An existing skill is accepted.
	a, _, err := svc.Create(ctx, CreateInput{Name: "poet", Config: Config{Prompt: "p", Skill: "real-skill"}})
	if err != nil {
		t.Fatalf("create with an existing skill must succeed, got: %v", err)
	}
	// EDIT to a non-existent skill is rejected too (symmetric with create).
	if _, err := svc.Edit(ctx, EditInput{ID: a.ID, Config: Config{Prompt: "p", Skill: "nope"}}); !errors.Is(err, agentdomain.ErrSkillNotFound) {
		t.Fatalf("edit to a non-existent skill must reject ErrSkillNotFound, got: %v", err)
	}
	// No skill at all stays valid (the field is optional).
	if _, _, err := svc.Create(ctx, CreateInput{Name: "plain", Config: Config{Prompt: "p"}}); err != nil {
		t.Fatalf("create with no skill must succeed, got: %v", err)
	}
}

// fakeKnowledgeGuide errors on any unknown doc id (mirroring BuildKnowledgePrefix after F98 surfaces
// GetBatch's silently-dropped missing ids).
type fakeKnowledgeGuide struct{ known map[string]bool }

func (f fakeKnowledgeGuide) BuildKnowledgePrefix(_ context.Context, ids []string) (string, error) {
	for _, id := range ids {
		if !f.known[id] {
			return "", agentdomain.ErrKnowledgeNotFound.WithDetails(map[string]any{"missing": []string{id}})
		}
	}
	return "knowledge", nil
}

// fakeMountResolver reports a mount unhealthy unless its ref is known (mirrors the real resolver
// CheckHealth for fn_/hd_/mcp existence).
type fakeMountResolver struct{ known map[string]bool }

func (fakeMountResolver) Resolve(context.Context, []agentdomain.ToolRef) ([]toolapp.Tool, error) {
	return nil, nil
}

func (f fakeMountResolver) CheckHealth(_ context.Context, refs []agentdomain.ToolRef) []agentdomain.MountHealth {
	out := make([]agentdomain.MountHealth, 0, len(refs))
	for _, r := range refs {
		h := agentdomain.MountHealth{Ref: r.Ref, Healthy: f.known[r.Ref]}
		if !h.Healthy {
			h.Error = "not found"
		}
		out = append(out, h)
	}
	return out
}

// TestCreateEdit_RejectsDanglingMounts: round-4 mountval lane — F96 (eager skill validation) was NOT
// generalized to the agent's other mount refs. A dangling KNOWLEDGE doc was accepted at create then
// SILENTLY dropped at invoke (status=ok, worse than F96's loud DOA); a dangling fn_/hd_ TOOL ref was
// accepted (ValidateTools only checks format) and failed dead-on-arrival at invoke. Both are now
// validated eagerly against the SAME resolvers invoke uses.
func TestCreateEdit_RejectsDanglingMounts(t *testing.T) {
	svc, ctx := newSvc(t)
	svc.SetInvokeDeps(InvokeDeps{
		Knowledge: fakeKnowledgeGuide{known: map[string]bool{"doc_real": true}},
		Mounts:    fakeMountResolver{known: map[string]bool{"fn_real": true}},
	})

	// Dangling knowledge doc → rejected at create (was: accepted, silently dropped at invoke).
	if _, _, err := svc.Create(ctx, CreateInput{Name: "gk", Config: Config{Prompt: "p", Knowledge: []string{"doc_nope"}}}); !errors.Is(err, agentdomain.ErrKnowledgeNotFound) {
		t.Fatalf("dangling knowledge must reject ErrKnowledgeNotFound, got: %v", err)
	}
	// Dangling tool ref → rejected at create (was: accepted, dead-on-arrival at invoke).
	if _, _, err := svc.Create(ctx, CreateInput{Name: "gt", Config: Config{Prompt: "p", Tools: []agentdomain.ToolRef{{Ref: "fn_nope"}}}}); !errors.Is(err, agentdomain.ErrMountInvalid) {
		t.Fatalf("dangling tool must reject ErrMountInvalid, got: %v", err)
	}
	// Valid knowledge + tools → accepted.
	a, _, err := svc.Create(ctx, CreateInput{Name: "ok", Config: Config{Prompt: "p", Knowledge: []string{"doc_real"}, Tools: []agentdomain.ToolRef{{Ref: "fn_real"}}}})
	if err != nil {
		t.Fatalf("valid mounts must succeed, got: %v", err)
	}
	// Edit to a dangling knowledge ref also rejected (symmetric with create).
	if _, err := svc.Edit(ctx, EditInput{ID: a.ID, Config: Config{Prompt: "p", Knowledge: []string{"doc_nope"}}}); !errors.Is(err, agentdomain.ErrKnowledgeNotFound) {
		t.Fatalf("edit to dangling knowledge must reject, got: %v", err)
	}
}
