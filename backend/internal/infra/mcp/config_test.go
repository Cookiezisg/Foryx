package mcp

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	mcpdomain "github.com/sunweilin/forgify/backend/internal/domain/mcp"
)

// claudeDesktopExample is verbatim from the mcp.md §5 sample so we
// know parsing is wire-compatible with what users would paste.
//
// claudeDesktopExample 取自 mcp.md §5 示例，证明解析与用户可能粘贴的
// Claude Desktop 配置 wire 兼容。
const claudeDesktopExample = `{
  "mcpServers": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_secret"
      }
    },
    "filesystem-extra": {
      "command": "/usr/local/bin/mcp-fs-extra",
      "args": ["--root", "/Users/me/projects"]
    }
  }
}`


func TestLoad_MissingFile_ReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.json")
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load missing file should succeed empty, got err=%v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestLoad_EmptyFile_ReturnsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(path, []byte(""), 0o600); err != nil {
		t.Fatalf("seed empty file: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load empty file: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestLoad_ClaudeDesktopExample(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte(claudeDesktopExample), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	gh, ok := got["github"]
	if !ok {
		t.Fatal("github entry missing")
	}
	if gh.Name != "github" {
		t.Errorf("github.Name = %q, want stamped from map key", gh.Name)
	}
	if gh.Command != "npx" {
		t.Errorf("github.Command = %q", gh.Command)
	}
	if len(gh.Args) != 2 || gh.Args[1] != "@modelcontextprotocol/server-github" {
		t.Errorf("github.Args = %v", gh.Args)
	}
	if gh.Env["GITHUB_PERSONAL_ACCESS_TOKEN"] != "ghp_secret" {
		t.Errorf("github.Env lost: %v", gh.Env)
	}

	fs, ok := got["filesystem-extra"]
	if !ok {
		t.Fatal("filesystem-extra entry missing")
	}
	if fs.Command != "/usr/local/bin/mcp-fs-extra" {
		t.Errorf("fs.Command = %q", fs.Command)
	}
}

func TestLoad_CorruptJSON_ReturnsErrConfigCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers": { broken`), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load corrupt should error")
	}
	if !errors.Is(err, ErrConfigCorrupt) {
		t.Errorf("err should be ErrConfigCorrupt; got %v", err)
	}
}

func TestLoad_TimeoutSecPreserved(t *testing.T) {
	body := `{"mcpServers":{"slow":{"command":"x","timeoutSec":120}}}`
	path := filepath.Join(t.TempDir(), "mcp.json")
	_ = os.WriteFile(path, []byte(body), 0o600)
	got, _ := Load(path)
	if got["slow"].TimeoutSec != 120 {
		t.Errorf("TimeoutSec lost: %d", got["slow"].TimeoutSec)
	}
}


func TestSave_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	in := map[string]mcpdomain.ServerConfig{
		"alpha": {
			Name:    "alpha",
			Command: "uvx",
			Args:    []string{"alpha-server"},
			Env:     map[string]string{"K": "v"},
		},
		"beta": {
			Name:       "beta",
			Command:    "npx",
			Args:       []string{"-y", "@scope/beta"},
			TimeoutSec: 60,
		},
	}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("round-trip len = %d, want 2", len(out))
	}
	if out["alpha"].Command != "uvx" || out["alpha"].Env["K"] != "v" {
		t.Errorf("alpha lost: %+v", out["alpha"])
	}
	if out["beta"].TimeoutSec != 60 {
		t.Errorf("beta.TimeoutSec lost: %d", out["beta"].TimeoutSec)
	}
}

func TestSave_KeysAlphaSorted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	// Intentionally insert in reverse alphabetical order.
	// 故意按反字母序插入。
	in := map[string]mcpdomain.ServerConfig{
		"zulu": {Command: "z"},
		"alpha": {Command: "a"},
		"mike":  {Command: "m"},
	}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The pretty-printed JSON should list keys in alpha order so that
	// hand-edited diffs / version control are stable across saves.
	// pretty-print 应按 alpha 列 key，让手编辑 diff / 版本控制跨次稳定。
	idxA := strings.Index(string(raw), `"alpha"`)
	idxM := strings.Index(string(raw), `"mike"`)
	idxZ := strings.Index(string(raw), `"zulu"`)
	if !(idxA < idxM && idxM < idxZ) {
		t.Errorf("keys not alpha-sorted in output\nbody: %s", raw)
	}
}

func TestSave_PrettyPrintedAndTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := Save(path, map[string]mcpdomain.ServerConfig{
		"x": {Command: "y"},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, _ := os.ReadFile(path)
	body := string(raw)
	if !strings.Contains(body, "\n") {
		t.Errorf("output should be pretty-printed (multi-line); got: %s", body)
	}
	if !strings.HasSuffix(body, "\n") {
		t.Errorf("output should have POSIX trailing newline; ends with %q", body[len(body)-1:])
	}
}

func TestSave_FileMode0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file mode bits don't translate cleanly on Windows")
	}
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := Save(path, map[string]mcpdomain.ServerConfig{"x": {Command: "y"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	mode := info.Mode().Perm()
	if mode != configFileMode {
		t.Errorf("file mode = %o, want %o (0600 — env values may carry credentials)",
			mode, configFileMode)
	}
}

func TestSave_CreatesParentDir(t *testing.T) {
	// Path under a not-yet-existing directory; Save should mkdir the parent.
	// 路径在尚不存在的目录下；Save 应自动 mkdir 父目录。
	path := filepath.Join(t.TempDir(), "nested", "more", "mcp.json")
	if err := Save(path, map[string]mcpdomain.ServerConfig{"x": {Command: "y"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

func TestSave_AtomicNoTmpLeftover(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := Save(path, map[string]mcpdomain.ServerConfig{"x": {Command: "y"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp should not be left over after successful save: stat err=%v", err)
	}
}

func TestSave_NameFieldNotDuplicatedInValue(t *testing.T) {
	// On disk the Name lives only in the map key. Stamping cfg.Name
	// inside the value would create an extra "name":"x" field — verify
	// it doesn't appear in the JSON.
	// 磁盘上 Name 只在 map key。值里加 "name":"x" 字段会冗余——验证 JSON
	// 中不出现。
	path := filepath.Join(t.TempDir(), "mcp.json")
	in := map[string]mcpdomain.ServerConfig{
		"github": {Name: "github", Command: "npx"},
	}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, _ := os.ReadFile(path)
	var generic map[string]map[string]map[string]any
	_ = json.Unmarshal(raw, &generic)
	entry := generic["mcpServers"]["github"]
	if _, present := entry["name"]; present {
		t.Errorf("disk shape should not duplicate name field in value; got %v", entry)
	}
}


func TestMerge_NewEntries_AllImported(t *testing.T) {
	existing := map[string]mcpdomain.ServerConfig{
		"alpha": {Command: "a"},
	}
	incoming := map[string]mcpdomain.ServerConfig{
		"beta":  {Command: "b"},
		"gamma": {Command: "g"},
	}
	out, res := Merge(existing, incoming, false)
	if len(out) != 3 {
		t.Errorf("merged len = %d, want 3", len(out))
	}
	if len(res.Conflicts) != 0 {
		t.Errorf("Conflicts = %v, want []", res.Conflicts)
	}
	if len(res.Imported) != 2 {
		t.Errorf("Imported = %v, want 2 entries", res.Imported)
	}
	// Names sorted alphabetically.
	if res.Imported[0] != "beta" || res.Imported[1] != "gamma" {
		t.Errorf("Imported not alpha-sorted: %v", res.Imported)
	}
}

func TestMerge_NoOverwrite_ConflictsRecordedOriginalKept(t *testing.T) {
	existing := map[string]mcpdomain.ServerConfig{
		"github": {Name: "github", Command: "old"},
	}
	incoming := map[string]mcpdomain.ServerConfig{
		"github":  {Name: "github", Command: "new"},
		"slack":   {Name: "slack", Command: "s"},
	}
	out, res := Merge(existing, incoming, false)
	if out["github"].Command != "old" {
		t.Errorf("no-overwrite path mutated existing github: %+v", out["github"])
	}
	if !contains(res.Conflicts, "github") {
		t.Errorf("Conflicts missing github: %v", res.Conflicts)
	}
	if !contains(res.Imported, "slack") {
		t.Errorf("Imported missing slack: %v", res.Imported)
	}
}

func TestMerge_Overwrite_ReplacesExisting(t *testing.T) {
	existing := map[string]mcpdomain.ServerConfig{
		"github": {Command: "old"},
	}
	incoming := map[string]mcpdomain.ServerConfig{
		"github": {Command: "new"},
	}
	out, res := Merge(existing, incoming, true)
	if out["github"].Command != "new" {
		t.Errorf("overwrite=true should replace; got %+v", out["github"])
	}
	if len(res.Conflicts) != 0 {
		t.Errorf("Conflicts should be empty when overwrite=true; got %v", res.Conflicts)
	}
	if !contains(res.Imported, "github") {
		t.Errorf("github should be in Imported on overwrite; got %v", res.Imported)
	}
}

func TestMerge_NilExisting_StartsFresh(t *testing.T) {
	incoming := map[string]mcpdomain.ServerConfig{
		"alpha": {Command: "a"},
	}
	out, res := Merge(nil, incoming, false)
	if len(out) != 1 || out["alpha"].Command != "a" {
		t.Errorf("nil-existing path failed: %+v", out)
	}
	if len(res.Imported) != 1 {
		t.Errorf("Imported = %v", res.Imported)
	}
}

func TestMerge_StampsNameFromMapKey(t *testing.T) {
	// Incoming may come from a Load() call where the file-schema
	// serverEntry didn't carry Name (it lives in the map key only).
	// Merge should stamp the canonical Name into the value so callers
	// reading from the merged map see Name populated.
	//
	// incoming 可能来自 Load()，file-schema 的 serverEntry 不带 Name
	// （只在 map key）。Merge 应把规范 Name 标进 value 让调用方读 merged
	// map 时看到 Name。
	incoming := map[string]mcpdomain.ServerConfig{
		"alpha": {Command: "a"}, // Name field intentionally empty
	}
	out, _ := Merge(nil, incoming, false)
	if out["alpha"].Name != "alpha" {
		t.Errorf("Name not stamped from map key: %+v", out["alpha"])
	}
}

// contains is a small string-slice helper used by the merge tests.
func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
