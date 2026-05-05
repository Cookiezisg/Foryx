// envmanager_rust_test.go — pure-function unit tests for RustEnvManager.
// Real `cargo install` belongs in the D9 pipeline suite.
//
// envmanager_rust_test.go ——RustEnvManager pure-function 单测。真
// `cargo install` 归 D9 pipeline 套。

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

var _ sandboxdomain.EnvManager = (*RustEnvManager)(nil)

func TestRustEnvManager_Kind(t *testing.T) {
	rm := NewRustEnvManager()
	if got := rm.Kind(); got != "rust" {
		t.Errorf("Kind() = %q, want rust", got)
	}
}

func TestRustEnvManager_CreateEnv_MakesCargoHome(t *testing.T) {
	rm := NewRustEnvManager()
	envPath := filepath.Join(t.TempDir(), "envs", "conv", "cv_abc:rust")
	if err := rm.CreateEnv(context.Background(), "/tmp/rust", envPath); err != nil {
		t.Fatalf("CreateEnv: %v", err)
	}
	if _, err := os.Stat(filepath.Join(envPath, ".cargo")); err != nil {
		t.Errorf(".cargo dir not created: %v", err)
	}
}

func TestRustEnvManager_EnvBin_PerOS(t *testing.T) {
	rm := NewRustEnvManager()
	got := rm.EnvBin("/data/envs/conv/cv:rust", "ripgrep")
	var want string
	if runtime.GOOS == "windows" {
		want = "/data/envs/conv/cv:rust/bin/ripgrep.exe"
	} else {
		want = "/data/envs/conv/cv:rust/bin/ripgrep"
	}
	if got != want {
		t.Errorf("EnvBin = %q, want %q", got, want)
	}
}

func TestRustEnvManager_EnvBin_PreservesExplicitExtension(t *testing.T) {
	rm := NewRustEnvManager()
	got := rm.EnvBin("/data/envs/conv/cv:rust", "tool.exe")
	want := "/data/envs/conv/cv:rust/bin/tool.exe"
	if got != want {
		t.Errorf("EnvBin = %q, want %q", got, want)
	}
}
