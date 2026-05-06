// mcp_test.go — JSON round-trip + status helpers + sentinel uniqueness.
// Pure-function tests; runtime / Service tests live in app/mcp + infra/mcp.
//
// mcp_test.go ——JSON round-trip + status helper + sentinel 唯一性。
// 纯函数测试；runtime / Service 在 app/mcp + infra/mcp 测。
package mcp

import (
	"encoding/json"
	"testing"
	"time"
)

func TestServerConfig_JSONRoundTrip(t *testing.T) {
	in := ServerConfig{
		Name:       "github",
		Command:    "npx",
		Args:       []string{"-y", "@modelcontextprotocol/server-github"},
		Env:        map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_x"},
		TimeoutSec: 60,
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out ServerConfig
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Name != in.Name || out.Command != in.Command || out.TimeoutSec != in.TimeoutSec {
		t.Errorf("round-trip mismatch: in=%+v out=%+v", in, out)
	}
	if len(out.Args) != 2 || out.Args[0] != "-y" {
		t.Errorf("Args lost: %v", out.Args)
	}
	if out.Env["GITHUB_PERSONAL_ACCESS_TOKEN"] != "ghp_x" {
		t.Errorf("Env lost: %v", out.Env)
	}
}

func TestServerConfig_OmitEmpty(t *testing.T) {
	// Bare config (just Name + Command): args/env/timeoutSec should not
	// appear in the JSON, matching Claude Desktop's compact mcp.json.
	//
	// 裸配置（仅 Name + Command）：args/env/timeoutSec 不应出现，匹配
	// Claude Desktop 紧凑的 mcp.json。
	bare := ServerConfig{Name: "n", Command: "c"}
	b, _ := json.Marshal(bare)
	s := string(b)
	for _, k := range []string{`"args"`, `"env"`, `"timeoutSec"`} {
		if containsKey(s, k) {
			t.Errorf("bare config should omit %s, got %s", k, s)
		}
	}
}

func TestToolDef_JSONRoundTrip(t *testing.T) {
	in := ToolDef{
		ServerName:  "github",
		Name:        "create_pr",
		Description: "Open a pull request",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out ToolDef
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Name != in.Name || string(out.InputSchema) != string(in.InputSchema) {
		t.Errorf("round-trip mismatch: %+v", out)
	}
}

func TestServerStatus_JSONShapeForFrontend(t *testing.T) {
	now := time.Now().UTC()
	s := ServerStatus{
		Name:                "github",
		Status:              StatusReady,
		PID:                 12345,
		ConnectedAt:         &now,
		ConsecutiveFailures: 0,
		TotalCalls:          42,
		TotalFailures:       3,
		Tools: []ToolDef{
			{ServerName: "github", Name: "list_prs", Description: "list PRs", InputSchema: json.RawMessage(`{}`)},
		},
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	_ = json.Unmarshal(b, &got)
	if got["status"] != StatusReady {
		t.Errorf("status field = %v, want %q", got["status"], StatusReady)
	}
	tools, _ := got["tools"].([]any)
	if len(tools) != 1 {
		t.Errorf("tools[] len = %d, want 1", len(tools))
	}
}

func TestHealthResult_OmitErrorWhenHealthy(t *testing.T) {
	hr := HealthResult{ServerName: "x", Healthy: true, LatencyMs: 17, ToolCount: 5, CheckedAt: time.Now()}
	b, _ := json.Marshal(hr)
	if containsKey(string(b), `"error"`) {
		t.Errorf("healthy result should omit error, got %s", b)
	}
}

func TestIsCallable(t *testing.T) {
	cases := map[string]bool{
		StatusDisconnected: false,
		StatusConnecting:   false,
		StatusReady:        true,
		StatusDegraded:     true,
		StatusFailed:       false,
		"unknown":          false,
	}
	for status, want := range cases {
		if got := IsCallable(status); got != want {
			t.Errorf("IsCallable(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestSentinels_AllDistinct(t *testing.T) {
	all := []error{
		ErrServerNotFound, ErrServerNotConnected, ErrToolNotFound,
		ErrToolCallFailed, ErrToolCallTimeout,
		ErrRegistryEntryNotFound, ErrRuntimeMissing, ErrRequiredEnvMissing,
		ErrRequiredArgsMissing, ErrInstallFailed,
	}
	if want := 10; len(all) != want {
		t.Errorf("sentinel count = %d, want %d", len(all), want)
	}
	seen := make(map[string]bool, len(all))
	for _, e := range all {
		if e == nil {
			t.Fatal("nil sentinel in list")
		}
		if seen[e.Error()] {
			t.Errorf("duplicate sentinel message: %q", e.Error())
		}
		seen[e.Error()] = true
	}
}

// containsKey searches for a JSON object key (incl. its colon) so a stray
// occurrence in a value doesn't false-positive.
//
// containsKey 找 JSON 对象键（含冒号）避免值里出现误报。
func containsKey(haystack, needle string) bool {
	return contains(haystack, needle+":") || contains(haystack, needle+" :")
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
