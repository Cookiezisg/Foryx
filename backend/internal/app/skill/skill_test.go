package skill

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"

	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	subagentapp "github.com/sunweilin/forgify/backend/internal/app/subagent"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

func writeSkill(t *testing.T, dir, name, frontmatter, body string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", skillDir, err)
	}
	content := "---\n" + frontmatter + "\n---\n" + body
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md for %s: %v", name, err)
	}
}

func newServiceWithDir(t *testing.T) *Service {
	t.Helper()
	return New(t.TempDir(), nil, nil, nil, nil, nil, zaptest.NewLogger(t))
}

type fakeSubagent struct {
	gotType     string
	gotPrompt   string
	gotOpts     subagentapp.SpawnOpts
	gotOverride *modeldomain.ModelRef
	returns     string
	returnErr   error
}

func (f *fakeSubagent) Spawn(_ context.Context, typeName, prompt string, opts subagentapp.SpawnOpts, parentModelOverride *modeldomain.ModelRef) (*subagentapp.SpawnResult, error) {
	f.gotType = typeName
	f.gotPrompt = prompt
	f.gotOpts = opts
	f.gotOverride = parentModelOverride
	if f.returnErr != nil {
		return nil, f.returnErr
	}
	return &subagentapp.SpawnResult{
		Type:   typeName,
		Status: subagentapp.StatusCompleted,
		Result: f.returns,
	}, nil
}

func TestScan_EmptyDir_OK(t *testing.T) {
	s := newServiceWithDir(t)
	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("Scan empty dir: %v", err)
	}
	if got := len(s.List(context.Background())); got != 0 {
		t.Errorf("List on empty dir = %d skills, want 0", got)
	}
}

func TestScan_MissingDir_ResetsCacheToEmpty(t *testing.T) {
	tmp := t.TempDir()
	s := New(filepath.Join(tmp, "does-not-exist"), nil, nil, nil, nil, nil, zaptest.NewLogger(t))
	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("missing dir should be benign; got: %v", err)
	}
	if got := len(s.List(context.Background())); got != 0 {
		t.Errorf("missing-dir scan must produce empty list; got %d", got)
	}
}

func TestScan_ParsesValidSkill(t *testing.T) {
	s := newServiceWithDir(t)
	writeSkill(t, s.skillsDir, "pr-review",
		`name: pr-review
description: Review a GitHub PR
allowed-tools:
  - Read
  - Bash(git *)
context: fork
agent: Explore
arguments:
  - pr_number`,
		`# Review PR #$1
Run: gh pr view $1`,
	)
	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	got, err := s.Get(context.Background(), "pr-review")
	if err != nil {
		t.Fatalf("Get pr-review: %v", err)
	}
	if got.Description != "Review a GitHub PR" {
		t.Errorf("Description = %q", got.Description)
	}
	if got.Frontmatter.Context != "fork" || got.Frontmatter.Agent != "Explore" {
		t.Errorf("fork directive lost: ctx=%q agent=%q",
			got.Frontmatter.Context, got.Frontmatter.Agent)
	}
	if len(got.Frontmatter.AllowedTools) != 2 {
		t.Errorf("AllowedTools = %v", got.Frontmatter.AllowedTools)
	}
	if got.Source != "user" {
		t.Errorf("Source = %q, want user", got.Source)
	}
}

func TestScan_RejectsBadFrontmatter(t *testing.T) {
	cases := []struct {
		name        string
		frontmatter string
	}{
		{"missing-description", `name: x`},
		{"empty-description", "name: x\ndescription: \"\""},
		{"fork-without-agent", "name: x\ndescription: y\ncontext: fork"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newServiceWithDir(t)
			writeSkill(t, s.skillsDir, tc.name, tc.frontmatter, "body")
			if err := s.Scan(context.Background()); err != nil {
				t.Fatalf("Scan top-level: %v", err)
			}
			if got := len(s.List(context.Background())); got != 0 {
				t.Errorf("bad skill loaded; want 0, got %d", got)
			}
		})
	}
}

func TestScan_RejectsTooLargeBody(t *testing.T) {
	s := newServiceWithDir(t)
	huge := strings.Repeat("X", skilldomain.MaxBodyBytes+1)
	writeSkill(t, s.skillsDir, "huge", `name: huge
description: oversized body`, huge)
	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got := len(s.List(context.Background())); got != 0 {
		t.Errorf("oversized skill loaded; cap is %d", skilldomain.MaxBodyBytes)
	}
}

func TestScan_DuplicateNameKeepsFirst(t *testing.T) {
	s := newServiceWithDir(t)
	writeSkill(t, s.skillsDir, "alpha-dir",
		"name: shared\ndescription: first instance", "body 1")
	writeSkill(t, s.skillsDir, "beta-dir",
		"name: shared\ndescription: second instance", "body 2")
	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	got, err := s.Get(context.Background(), "shared")
	if err != nil {
		t.Fatalf("Get shared: %v", err)
	}
	if got.Description != "first instance" && got.Description != "second instance" {
		t.Errorf("kept skill has unexpected description: %q", got.Description)
	}
}

func TestSplitFrontmatter_Modes(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		yaml    string
		body    string
		wantErr bool
	}{
		{
			"basic",
			"---\nname: x\ndescription: y\n---\n# Body\n",
			"name: x\ndescription: y",
			"# Body\n",
			false,
		},
		{
			"crlf-line-endings",
			"---\r\nname: x\r\ndescription: y\r\n---\r\nbody\r\n",
			"name: x\ndescription: y",
			"body\n",
			false,
		},
		{
			"utf8-bom-stripped",
			"\xEF\xBB\xBF---\nname: x\ndescription: y\n---\nbody",
			"name: x\ndescription: y",
			"body",
			false,
		},
		{
			"missing-opening-fence",
			"name: x\ndescription: y\n",
			"", "", true,
		},
		{
			"missing-closing-fence",
			"---\nname: x\ndescription: y\nbody continues with no end",
			"", "", true,
		},
		{
			"closing-at-eof-no-newline",
			"---\nname: x\ndescription: y\n---",
			"name: x\ndescription: y",
			"",
			false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			y, b, err := splitFrontmatter([]byte(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error; got yaml=%q body=%q", string(y), string(b))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got := string(y); got != tc.yaml {
				t.Errorf("yaml = %q, want %q", got, tc.yaml)
			}
			if got := string(b); got != tc.body {
				t.Errorf("body = %q, want %q", got, tc.body)
			}
		})
	}
}

func TestSubstitute_AllPlaceholders(t *testing.T) {
	body := `# Job for $1 with $ARGUMENTS
Working in ${CLAUDE_SKILL_DIR}
Session: ${CLAUDE_SESSION_ID}
Effort: ${CLAUDE_EFFORT}
PR: $pr_number`
	got := substitute(body, substituteVars{
		Arguments: []string{"1234", "verbose"},
		NamedArgs: []string{"pr_number", "mode"},
		SkillDir:  "/x/y/skills/pr-review",
		SessionID: "conv_abc",
		Effort:    "high",
	})
	wants := []string{
		"# Job for 1234 with 1234 verbose",
		"Working in /x/y/skills/pr-review",
		"Session: conv_abc",
		"Effort: high",
		"PR: 1234",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("substituted body missing %q\ngot:\n%s", w, got)
		}
	}
}

func TestSubstitute_ArgumentsLongerThanPositional(t *testing.T) {
	args := make([]string, 11)
	for i := range args {
		args[i] = "ARG" + string(rune('A'+i))
	}
	got := substitute("first=$1 tenth=$10 eleventh=$11", substituteVars{
		Arguments: args,
	})
	if !strings.Contains(got, "first=ARGA") {
		t.Errorf("$1 lost: %q", got)
	}
	if !strings.Contains(got, "tenth=ARGJ") {
		t.Errorf("$10 lost: %q", got)
	}
	if !strings.Contains(got, "eleventh=ARGK") {
		t.Errorf("$11 lost: %q", got)
	}
}

func TestActivate_NonFork_ReturnsBodyAndSetsActiveSkill(t *testing.T) {
	s := newServiceWithDir(t)
	writeSkill(t, s.skillsDir, "deploy",
		`name: deploy
description: deploy via CI
allowed-tools:
  - Bash(make deploy)`,
		"# Run\nmake deploy")
	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	state := &agentstatepkg.AgentState{}
	ctx := reqctxpkg.WithAgentState(context.Background(), state)

	out, err := s.Activate(ctx, "deploy", nil)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !strings.Contains(out, "make deploy") {
		t.Errorf("body lost in activate output: %q", out)
	}
	active := state.ActiveSkill()
	if active == nil || active.Name != "deploy" {
		t.Errorf("ActiveSkill not set; got %v", active)
	}
}

func TestActivate_Fork_DispatchesToSubagent(t *testing.T) {
	fake := &fakeSubagent{returns: "subagent done"}
	tmp := t.TempDir()
	s := New(tmp, fake, nil, nil, nil, nil, zaptest.NewLogger(t))
	writeSkill(t, s.skillsDir, "review",
		`name: review
description: review pr
context: fork
agent: Explore
arguments:
  - pr_number`,
		"# Review #$1\nLook at PR $1")
	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	out, err := s.Activate(context.Background(), "review", []string{"1234"})
	if err != nil {
		t.Fatalf("Activate fork: %v", err)
	}
	if out != "subagent done" {
		t.Errorf("Activate returned %q, want subagent's result", out)
	}
	if fake.gotType != "Explore" {
		t.Errorf("Spawn type = %q, want Explore", fake.gotType)
	}
	if !strings.Contains(fake.gotPrompt, "Review #1234") {
		t.Errorf("Spawn prompt missing $1 substitution: %q", fake.gotPrompt)
	}
}

// TestActivate_Fork_PropagatesParentModelOverride covers the chain-root override
// hand-off: chat.runner stashes conv.ModelOverride on ctx, fork-skill must read it
// and pass to Spawn so the subagent uses the same effective model as the parent.
//
// TestActivate_Fork_PropagatesParentModelOverride 守 override 链根透传:
// chat.runner 把 conv.ModelOverride 塞进 ctx, fork-skill 必须读出来传给 Spawn,
// 让 subagent 用与父对话同一 effective model。
func TestActivate_Fork_PropagatesParentModelOverride(t *testing.T) {
	fake := &fakeSubagent{returns: "ok"}
	tmp := t.TempDir()
	s := New(tmp, fake, nil, nil, nil, nil, zaptest.NewLogger(t))
	writeSkill(t, s.skillsDir, "review",
		"name: review\ndescription: r\ncontext: fork\nagent: Explore", "body")
	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	override := &modeldomain.ModelRef{APIKeyID: "aki_test", ModelID: "gpt-4o"}
	ctx := reqctxpkg.WithModelOverride(context.Background(), override)
	if _, err := s.Activate(ctx, "review", nil); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if fake.gotOverride == nil {
		t.Fatal("Spawn received nil override; expected the one stashed on ctx")
	}
	if fake.gotOverride.APIKeyID != "aki_test" || fake.gotOverride.ModelID != "gpt-4o" {
		t.Errorf("override = %+v, want {aki_test, gpt-4o}", fake.gotOverride)
	}
}

func TestActivate_Fork_NilOverride_WhenCtxHasNone(t *testing.T) {
	fake := &fakeSubagent{returns: "ok"}
	tmp := t.TempDir()
	s := New(tmp, fake, nil, nil, nil, nil, zaptest.NewLogger(t))
	writeSkill(t, s.skillsDir, "review",
		"name: review\ndescription: r\ncontext: fork\nagent: Explore", "body")
	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if _, err := s.Activate(context.Background(), "review", nil); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if fake.gotOverride != nil {
		t.Errorf("Spawn override = %+v, want nil when ctx unset", fake.gotOverride)
	}
}

func TestActivate_NestedFork_IgnoresForkDirective(t *testing.T) {
	fake := &fakeSubagent{returns: "should not be called"}
	tmp := t.TempDir()
	s := New(tmp, fake, nil, nil, nil, nil, zaptest.NewLogger(t))
	writeSkill(t, s.skillsDir, "fork-skill",
		`name: fork-skill
description: forks
context: fork
agent: Explore`,
		"# Inline body")
	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	ctx := reqctxpkg.WithSubagentDepth(context.Background(), 1)
	out, err := s.Activate(ctx, "fork-skill", nil)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if !strings.Contains(out, "Inline body") {
		t.Errorf("expected inline body, got %q", out)
	}
	if fake.gotType != "" {
		t.Errorf("subagent should NOT be spawned at depth ≥ 1; got type=%q", fake.gotType)
	}
}

func TestActivate_ForkWithoutSubagentService_FailsClean(t *testing.T) {
	tmp := t.TempDir()
	s := New(tmp, nil, nil, nil, nil, nil, zaptest.NewLogger(t))
	writeSkill(t, s.skillsDir, "k",
		"name: k\ndescription: forks\ncontext: fork\nagent: Explore", "x")
	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	_, err := s.Activate(context.Background(), "k", nil)
	if err == nil {
		t.Fatal("Activate fork with nil SubagentService should error")
	}
	if !strings.Contains(err.Error(), "SubagentService is nil") {
		t.Errorf("error message lacks diagnostic hint: %q", err.Error())
	}
}

func TestActivate_Missing_ReturnsErrSkillNotFound(t *testing.T) {
	s := newServiceWithDir(t)
	_, err := s.Activate(context.Background(), "ghost", nil)
	if !errors.Is(err, skilldomain.ErrSkillNotFound) {
		t.Errorf("err = %v, want wrap of ErrSkillNotFound", err)
	}
}

func TestSearch_LeqTopK_SkipsLLM(t *testing.T) {
	s := newServiceWithDir(t)
	writeSkill(t, s.skillsDir, "alpha", "name: alpha\ndescription: a", "x")
	writeSkill(t, s.skillsDir, "beta", "name: beta\ndescription: b", "x")
	if err := s.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	got, err := s.Search(context.Background(), "anything", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("Search returned %d, want 2", len(got))
	}
	if got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Errorf("Search order unexpected: %v", got)
	}
}

func TestSearch_EmptyCatalog(t *testing.T) {
	s := newServiceWithDir(t)
	got, err := s.Search(context.Background(), "x", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty catalog Search returned %d, want 0", len(got))
	}
}
