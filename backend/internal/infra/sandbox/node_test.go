package sandbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

var _ sandboxdomain.EnvManager = (*NodeEnvManager)(nil)

func TestNodeEnvManager_Kind(t *testing.T) {
	nm := NewNodeEnvManager()
	if got := nm.Kind(); got != "node" {
		t.Errorf("Kind() = %q, want node", got)
	}
}

func TestNodeEnvManager_CreateEnv_WritesPackageJSON(t *testing.T) {
	nm := NewNodeEnvManager()
	envPath := filepath.Join(t.TempDir(), "envs", "mcp", "context7")
	if err := nm.CreateEnv(context.Background(), "/tmp/node", envPath); err != nil {
		t.Fatalf("CreateEnv: %v", err)
	}

	pkgPath := filepath.Join(envPath, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		t.Fatalf("read package.json: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest is not valid JSON: %v", err)
	}
	if manifest["private"] != true {
		t.Errorf("manifest must be private to prevent accidental publish, got %v", manifest["private"])
	}
	if name, _ := manifest["name"].(string); name != "forgify-env-context7" {
		t.Errorf("manifest name = %q, want forgify-env-context7", name)
	}
}

func TestNodeEnvManager_CreateEnv_Idempotent(t *testing.T) {
	nm := NewNodeEnvManager()
	envPath := filepath.Join(t.TempDir(), "envs", "mcp", "context7")
	if err := nm.CreateEnv(context.Background(), "/tmp/node", envPath); err != nil {
		t.Fatalf("first CreateEnv: %v", err)
	}
	pkgPath := filepath.Join(envPath, "package.json")
	st1, err := os.Stat(pkgPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if err := nm.CreateEnv(context.Background(), "/tmp/node", envPath); err != nil {
		t.Fatalf("second CreateEnv: %v", err)
	}
	st2, _ := os.Stat(pkgPath)
	if !st1.ModTime().Equal(st2.ModTime()) {
		t.Errorf("idempotency broken: package.json was rewritten on second call")
	}
}

func TestNodeEnvManager_EnvBin_PerOS(t *testing.T) {
	nm := NewNodeEnvManager()
	got := nm.EnvBin("/data/envs/mcp/context7", "tsc")

	var want string
	if runtime.GOOS == "windows" {
		want = "/data/envs/mcp/context7/node_modules/.bin/tsc.cmd"
	} else {
		want = "/data/envs/mcp/context7/node_modules/.bin/tsc"
	}
	if got != want {
		t.Errorf("EnvBin = %q, want %q", got, want)
	}
}

func TestNodeEnvManager_EnvBin_PreservesExplicitExtension(t *testing.T) {
	nm := NewNodeEnvManager()
	got := nm.EnvBin("/data/envs/mcp/context7", "tsc.cmd")
	want := "/data/envs/mcp/context7/node_modules/.bin/tsc.cmd"
	if got != want {
		t.Errorf("EnvBin = %q, want %q", got, want)
	}
}
