// envmanager_static_test.go — pure-function unit tests for
// StaticBinaryEnvManager.
//
// envmanager_static_test.go ——StaticBinaryEnvManager pure-function 单测。

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

var _ sandboxdomain.EnvManager = (*StaticBinaryEnvManager)(nil)

func TestStaticBinaryEnvManager_Kind(t *testing.T) {
	sm := NewStaticBinaryEnvManager("github-mcp", "/data/sandbox")
	if got := sm.Kind(); got != "github-mcp" {
		t.Errorf("Kind() = %q, want github-mcp", got)
	}
}

func TestStaticBinaryEnvManager_CreateEnv_Mkdirs(t *testing.T) {
	sm := NewStaticBinaryEnvManager("github-mcp", "/tmp/sandbox")
	envPath := filepath.Join(t.TempDir(), "envs", "mcp", "github")
	if err := sm.CreateEnv(context.Background(), "/tmp/runtime", envPath); err != nil {
		t.Fatalf("CreateEnv: %v", err)
	}
	if _, err := os.Stat(envPath); err != nil {
		t.Errorf("env dir not created: %v", err)
	}
}

func TestStaticBinaryEnvManager_EnvBin_PointsAtSharedBinary(t *testing.T) {
	sm := NewStaticBinaryEnvManager("github-mcp", "/data/sandbox")
	got := sm.EnvBin("/data/envs/mcp/github", "github-mcp")
	want := filepath.Join("/data/sandbox", staticBinariesSubdir, "github-mcp", "github-mcp")
	if got != want {
		t.Errorf("EnvBin = %q, want %q (binary lives outside per-env dirs)", got, want)
	}
}

func TestStaticBinaryEnvManager_NoOps(t *testing.T) {
	sm := NewStaticBinaryEnvManager("github-mcp", "/data/sandbox")
	ctx := context.Background()
	if err := sm.InstallDeps(ctx, "/tmp/runtime", "/tmp/env", nil, nil); err != nil {
		t.Errorf("InstallDeps: want nil, got %v", err)
	}
	if err := sm.InstallExtras(ctx, "/tmp/runtime", "/tmp/env", nil, nil); err != nil {
		t.Errorf("InstallExtras: want nil, got %v", err)
	}
}
