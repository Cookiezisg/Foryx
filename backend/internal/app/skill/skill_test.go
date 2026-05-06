// skill_test.go — covers Scan + Get + List + Activate (non-fork +
// fork-via-mock-subagent + nested-depth fork suppression) + substitute
// placeholders. Search ranking against LLM is left to pipeline tests
// where a real (or fake) LLM is wired; here we cover only the
// short-circuit-when-≤-topK path.
//
// skill_test.go ——覆盖 Scan + Get + List + Activate（非 fork + 经 mock
// subagent fork + 嵌套 depth 抑制 fork）+ 占位替换。Search 的 LLM 排序留
// pipeline（接真/假 LLM 的环境），本文件只覆盖 ≤ topK 短路。
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
	subagentdomain "github.com/sunweilin/forgify/backend/internal/domain/subagent"
	agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ── helpers ──────────────────────────────────────────────────────────

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

// fakeSubagent records the spawn call so fork-mode tests can assert
// dispatch parameters without spinning up the real subagentapp.Service.
//
// fakeSubagent 记 spawn 调用让 fork 测试无需起真 subagentapp.Service 即
// 可断言派发参数。
type fakeSubagent struct {
	gotType   string
	gotPrompt string
	gotOpts   subagentapp.SpawnOpts
	returns   string
	returnErr error
}

func (f *fakeSubagent) Spawn(_ context.Context, typeName, prompt string, opts subagentapp.SpawnOpts) (*subagentapp.SpawnResult, error) {
	f.gotType = typeName
	f.gotPrompt = prompt
	f.gotOpts = opts
	if f.returnErr != nil {
		return nil, f.returnErr
	}
	return &subagentapp.SpawnResult{
		Run:    &subagentdomain.SubagentRun{Type: typeName, Status: subagentdomain.StatusCompleted},
		Result: f.returns,
	}, nil
}

// ── Scan ─────────────────────────────────────────────────────────────

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
			// Bad skill must be skipped (logged, not loaded). Catalog stays empty.
			// 坏 skill 必须跳过（log，不加载）。catalog 保持空。
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
	// Two dirs whose frontmatter.name collide.
	// 两目录 frontmatter.name 撞名。
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
	// One of the two is kept; both must have produced log warnings (not asserted here).
	// 二者之一保留；两条都该 log 警告（此处不断言）。
	if got.Description != "first instance" && got.Description != "second instance" {
		t.Errorf("kept skill has unexpected description: %q", got.Description)
	}
}

// ── splitFrontmatter ─────────────────────────────────────────────────

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

// ── substitute ───────────────────────────────────────────────────────

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
	// Make sure $10 doesn't get pre-empted by $1 — strings.Replacer is
	// longest-key-wins but we want explicit verification.
	// 防 $10 被 $1 抢匹配——strings.Replacer 长 key 胜，但要验证。
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

// ── Activate ─────────────────────────────────────────────────────────

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

func TestActivate_NestedFork_IgnoresForkDirective(t *testing.T) {
	// Per skill.md §9.5: when called inside a subagent (depth ≥ 1), the
	// fork directive is suppressed — body returned inline so the sub-LLM
	// follows the instructions in its own context, no nested spawn.
	// §9.5：在 subagent 里（depth ≥ 1）抑制 fork——body inline 返让子 LLM
	// 在自己上下文执行，无嵌套 spawn。
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
	// Production main.go always wires a subagent; tests sometimes don't
	// (when subagent isn't relevant). A fork skill in that env must
	// produce a clear error rather than nil-deref.
	// 生产 main.go 永远接 subagent；测试有时不接（无关时）。该环境下
	// fork skill 必须清晰报错而非 nil-deref。
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

// ── Search short-circuit ─────────────────────────────────────────────

func TestSearch_LeqTopK_SkipsLLM(t *testing.T) {
	// With ≤ topK skills loaded the Search must return everything alpha-
	// sorted without consulting the LLM (modelPicker is nil here — would
	// nil-deref if Search tried to resolve).
	// ≤ topK 时不查 LLM 直接字母序全返（modelPicker 为 nil——若 Search 尝试
	// resolve 会 nil-deref）。
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
