package skill

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"

	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
)

// newTestSvc builds an empty skill Service rooted at t.TempDir(). Tests
// that need skills installed write their own SKILL.md files into the
// dir + call svc.Scan.
//
// newTestSvc 建空 skill Service 根 t.TempDir()。需 skill 的测试自己写
// SKILL.md + 调 svc.Scan。
func newTestSvc(t *testing.T) *skillapp.Service {
	t.Helper()
	return skillapp.New(t.TempDir(), nil, nil, nil, nil, nil, zaptest.NewLogger(t))
}

func TestSkillTools_FactoryReturnsBoth(t *testing.T) {
	tools := SkillTools(newTestSvc(t))
	if len(tools) != 2 {
		t.Fatalf("SkillTools returned %d tools, want 2", len(tools))
	}
	gotNames := map[string]bool{}
	for _, tool := range tools {
		gotNames[tool.Name()] = true
	}
	for _, want := range []string{"search_skills", "activate_skill"} {
		if !gotNames[want] {
			t.Errorf("factory missing %q (got %v)", want, gotNames)
		}
	}
}


func TestSearchSkills_Identity(t *testing.T) {
	tt := &SearchSkills{}
	if got := tt.Name(); got != "search_skills" {
		t.Errorf("Name = %q", got)
	}
	if !strings.Contains(tt.Description(), "search the user's installed skills") &&
		!strings.Contains(tt.Description(), "Search the user's installed skills") {
		t.Errorf("Description doesn't describe its purpose")
	}
	if !json.Valid(tt.Parameters()) {
		t.Errorf("Parameters() not valid JSON")
	}
}

func TestSearchSkills_StaticMetadata(t *testing.T) {
	tt := &SearchSkills{}
	if !tt.IsReadOnly() {
		t.Error("IsReadOnly = false; search_skills must be read-only")
	}
	if tt.NeedsReadFirst() || tt.RequiresWorkspace() {
		t.Error("NeedsReadFirst / RequiresWorkspace should be false")
	}
}

func TestSearchSkills_ValidateInput(t *testing.T) {
	tt := &SearchSkills{}
	cases := []struct {
		name  string
		args  string
		isErr bool
	}{
		{"valid", `{"query": "review pr"}`, false},
		{"empty-query", `{"query": ""}`, true},
		{"whitespace-query", `{"query": "   "}`, true},
		{"missing-query", `{}`, true},
		{"malformed-json", `{not json}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tt.ValidateInput(json.RawMessage(tc.args))
			if tc.isErr && err == nil {
				t.Errorf("want err, got nil")
			}
			if !tc.isErr && err != nil {
				t.Errorf("unexpected err: %v", err)
			}
		})
	}
}

func TestSearchSkills_CheckPermissions_AlwaysAllow(t *testing.T) {
	tt := &SearchSkills{}
	for _, mode := range []toolapp.PermissionMode{
		toolapp.PermissionModeDefault, toolapp.PermissionModeAcceptEdits,
		toolapp.PermissionModePlan, toolapp.PermissionModeBypass,
	} {
		if got := tt.CheckPermissions(json.RawMessage(`{}`), mode); got != toolapp.PermissionAllow {
			t.Errorf("mode=%q: got %v, want Allow", mode, got)
		}
	}
}

func TestSearchSkills_Execute_EmptyCatalog(t *testing.T) {
	svc := newTestSvc(t)
	tt := &SearchSkills{svc: svc}
	out, err := tt.Execute(context.Background(), `{"query": "anything"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "No skills installed") {
		t.Errorf("expected friendly empty-catalog message; got %q", out)
	}
}

func TestSearchSkills_Execute_ReturnsJSONList(t *testing.T) {
	svc := newTestSvc(t)
	// Seed two skills so Search returns them via the ≤topK short-circuit.
	// 种 2 skill 让 Search 经 ≤topK 短路返。
	seedSkill(t, svc, "alpha", "alpha description", false)
	seedSkill(t, svc, "beta", "beta description", true)
	if err := svc.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	tt := &SearchSkills{svc: svc}
	out, err := tt.Execute(context.Background(), `{"query": "anything", "top_k": 5}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var got []searchResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("Execute output not JSON list: %v\nout: %s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 results, got %d", len(got))
	}
	if got[0].Name != "alpha" || got[0].IsFork {
		t.Errorf("alpha row malformed: %+v", got[0])
	}
	if got[1].Name != "beta" || !got[1].IsFork {
		t.Errorf("beta row should be IsFork=true: %+v", got[1])
	}
}


func TestActivateSkill_Identity(t *testing.T) {
	tt := &ActivateSkill{}
	if got := tt.Name(); got != "activate_skill" {
		t.Errorf("Name = %q", got)
	}
	if !strings.Contains(tt.Description(), "skill") {
		t.Errorf("Description doesn't mention skill")
	}
	if !json.Valid(tt.Parameters()) {
		t.Errorf("Parameters() not valid JSON")
	}
}

func TestActivateSkill_StaticMetadata(t *testing.T) {
	tt := &ActivateSkill{}
	// IsReadOnly = false because ActiveSkill side-channel writes to AgentState.
	// IsReadOnly = false 因 ActiveSkill 旁路写 AgentState。
	if tt.IsReadOnly() {
		t.Error("IsReadOnly = true; activate_skill mutates AgentState side-channel")
	}
	if tt.NeedsReadFirst() || tt.RequiresWorkspace() {
		t.Error("NeedsReadFirst / RequiresWorkspace should be false")
	}
}

func TestActivateSkill_ValidateInput(t *testing.T) {
	tt := &ActivateSkill{}
	cases := []struct {
		name  string
		args  string
		isErr bool
	}{
		{"valid", `{"name": "deploy"}`, false},
		{"with-args", `{"name": "deploy", "arguments": ["staging"]}`, false},
		{"empty-name", `{"name": ""}`, true},
		{"missing-name", `{}`, true},
		{"malformed-json", `{not json}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tt.ValidateInput(json.RawMessage(tc.args))
			if tc.isErr && err == nil {
				t.Errorf("want err, got nil")
			}
			if !tc.isErr && err != nil {
				t.Errorf("unexpected err: %v", err)
			}
			if errors.Is(err, ErrEmptyName) && !strings.Contains(err.Error(), "name") {
				t.Errorf("ErrEmptyName message lacks 'name': %q", err.Error())
			}
		})
	}
}

func TestActivateSkill_Execute_FriendlyMissing(t *testing.T) {
	tt := &ActivateSkill{svc: newTestSvc(t)}
	out, err := tt.Execute(context.Background(), `{"name": "ghost"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("expected friendly not-found string; got %q", out)
	}
	if !strings.Contains(out, "search_skills") {
		t.Errorf("friendly message should suggest search_skills; got %q", out)
	}
}

func TestActivateSkill_Execute_ReturnsBody(t *testing.T) {
	svc := newTestSvc(t)
	seedSkill(t, svc, "deploy", "deploy CI", false)
	if err := svc.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	tt := &ActivateSkill{svc: svc}
	out, err := tt.Execute(context.Background(), `{"name": "deploy"}`)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(out, "deploy body content") {
		t.Errorf("Execute did not return body; got %q", out)
	}
}


func seedSkill(t *testing.T, svc *skillapp.Service, name, desc string, fork bool) {
	t.Helper()
	dir := filepath.Join(svc.SkillsDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	fmStr := "name: " + name + "\ndescription: " + desc
	if fork {
		fmStr += "\ncontext: fork\nagent: Explore"
	}
	body := "# " + name + " body content\nrun the steps."
	content := "---\n" + fmStr + "\n---\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}
