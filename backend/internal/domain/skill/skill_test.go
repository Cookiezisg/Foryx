package skill

import (
	"encoding/json"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFrontmatter_YAMLParse_FullSpec(t *testing.T) {
	src := `name: pr-review
description: |
  Review a GitHub PR for code quality, tests, and security.
  Use when the user asks to "review PR" or "check this pull request".
allowed-tools:
  - Read
  - Grep
  - Bash(git *)
  - Bash(gh pr view *)
disable-model-invocation: false
user-invocable: true
paths:
  - "**/*.go"
  - "**/*.py"
context: fork
agent: Explore
arguments:
  - pr_number
argument-hint: "<pr_number>"
model: claude-sonnet-4-6
effort: medium
when_to_use: When user asks to review a PR
`
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(src), &fm); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if fm.Name != "pr-review" {
		t.Errorf("Name = %q, want pr-review", fm.Name)
	}
	if !strings.Contains(fm.Description, "GitHub PR") {
		t.Errorf("Description missing expected text: %q", fm.Description)
	}
	if got, want := len(fm.AllowedTools), 4; got != want {
		t.Fatalf("AllowedTools len = %d, want %d", got, want)
	}
	if fm.AllowedTools[2] != "Bash(git *)" {
		t.Errorf("AllowedTools[2] = %q, want Bash(git *)", fm.AllowedTools[2])
	}
	if fm.Context != "fork" || fm.Agent != "Explore" {
		t.Errorf("Context/Agent = %q/%q, want fork/Explore", fm.Context, fm.Agent)
	}
	if len(fm.Arguments) != 1 || fm.Arguments[0] != "pr_number" {
		t.Errorf("Arguments = %v, want [pr_number]", fm.Arguments)
	}
	if fm.WhenToUse == "" || fm.Model == "" || fm.Effort == "" {
		t.Errorf("V1-parsed-not-consumed fields lost: when_to_use=%q model=%q effort=%q",
			fm.WhenToUse, fm.Model, fm.Effort)
	}
}

func TestFrontmatter_YAMLParse_Minimal(t *testing.T) {
	// Anthropic spec only requires name + description; everything else opt-in.
	// Anthropic spec 仅 name + description 必填；其他全可选。
	src := `name: csv-clean
description: Strip BOMs and normalize line endings.
`
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(src), &fm); err != nil {
		t.Fatalf("minimal frontmatter parse: %v", err)
	}
	if fm.Name != "csv-clean" {
		t.Errorf("Name = %q", fm.Name)
	}
	if fm.Context != "" {
		t.Errorf("Context = %q, want empty (no fork directive)", fm.Context)
	}
	if len(fm.AllowedTools) != 0 {
		t.Errorf("AllowedTools = %v, want empty", fm.AllowedTools)
	}
}

func TestSkill_JSONRoundTrip_CamelCase(t *testing.T) {
	// Wire shape per CLAUDE.md N3: API JSON uses camelCase (allowedTools
	// not allowed-tools). The YAML tag is for SKILL.md disk format; the
	// JSON tag is for HTTP API responses. They differ on purpose.
	// 线 N3：API JSON 用 camelCase（allowedTools 而非 allowed-tools）。YAML
	// tag 为磁盘 SKILL.md 格式；JSON tag 为 HTTP 响应。差别有意。
	in := Skill{
		Name:        "deploy",
		Source:      "user",
		DirPath:     "/home/u/.forgify/skills/deploy",
		BodyPath:    "/home/u/.forgify/skills/deploy/SKILL.md",
		Description: "Deploy via internal CI",
		Frontmatter: Frontmatter{
			Name:         "deploy",
			Description:  "Deploy via internal CI",
			AllowedTools: []string{"Bash(make deploy)"},
			Context:      "fork",
			Agent:        "general-purpose",
		},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	wire := string(b)
	for _, want := range []string{
		`"dirPath":`, `"bodyPath":`, `"frontmatter":`, `"allowedTools":`,
	} {
		if !strings.Contains(wire, want) {
			t.Errorf("wire missing camelCase key %s\nwire: %s", want, wire)
		}
	}
	for _, banned := range []string{`"dir_path"`, `"body_path"`, `"allowed-tools":`} {
		if strings.Contains(wire, banned) {
			t.Errorf("wire contains non-camelCase token %q", banned)
		}
	}

	var out Skill
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Name != in.Name || out.Frontmatter.Context != "fork" {
		t.Errorf("round-trip lost data: %+v", out)
	}
}

func TestSentinels_Unique(t *testing.T) {
	all := []error{
		ErrSkillNotFound,
		ErrInvalidFrontmatter,
		ErrBodyTooLarge,
		ErrNameConflict,
		ErrInvalidName,
	}
	seen := map[string]bool{}
	for _, e := range all {
		if e == nil {
			t.Fatal("nil sentinel in list")
		}
		msg := e.Error()
		if seen[msg] {
			t.Errorf("duplicate sentinel message %q (errors.Is breaks at transport)", msg)
		}
		seen[msg] = true
	}
}

func TestSentinels_AllPrefixed(t *testing.T) {
	// Every sentinel message must start with "skill: " — readers grep error
	// logs by domain prefix to triage which subsystem is failing.
	// 每条 sentinel 必须 "skill: " 前缀——读日志按 domain 前缀分流。
	for _, e := range []error{
		ErrSkillNotFound, ErrInvalidFrontmatter, ErrBodyTooLarge,
		ErrNameConflict, ErrInvalidName,
	} {
		if !strings.HasPrefix(e.Error(), "skill: ") {
			t.Errorf("sentinel %q lacks 'skill: ' prefix", e.Error())
		}
	}
}

func TestConstants(t *testing.T) {
	if MaxBodyBytes != 32*1024 {
		t.Errorf("MaxBodyBytes = %d, want 32 KiB (Anthropic spec §10.6)", MaxBodyBytes)
	}
	if MaxDescriptionChars != 1536 {
		t.Errorf("MaxDescriptionChars = %d, want 1536 (per spec)", MaxDescriptionChars)
	}
}
