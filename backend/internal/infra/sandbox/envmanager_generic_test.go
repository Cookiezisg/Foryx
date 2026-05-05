// envmanager_generic_test.go — pure-function unit tests for
// GenericEnvManager. Covers the no-op semantics + CreateEnv mkdir.
//
// envmanager_generic_test.go ——GenericEnvManager pure-function 单测。
// 覆盖 no-op 语义 + CreateEnv mkdir。

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

var _ sandboxdomain.EnvManager = (*GenericEnvManager)(nil)

func TestGenericEnvManager_KindMatchesConstructor(t *testing.T) {
	for _, kind := range []string{"elixir", "zig", "lua", "deno"} {
		gm := NewGenericEnvManager(kind)
		if got := gm.Kind(); got != kind {
			t.Errorf("Kind() = %q, want %q", got, kind)
		}
	}
}

func TestGenericEnvManager_CreateEnv_MkdirsAndIdempotent(t *testing.T) {
	gm := NewGenericEnvManager("elixir")
	envPath := filepath.Join(t.TempDir(), "envs", "conv", "cv_abc:elixir")

	if err := gm.CreateEnv(context.Background(), "/tmp/elixir", envPath); err != nil {
		t.Fatalf("first CreateEnv: %v", err)
	}
	if _, err := os.Stat(envPath); err != nil {
		t.Errorf("env dir not created: %v", err)
	}
	// Second call must not error (mkdir is idempotent via MkdirAll).
	if err := gm.CreateEnv(context.Background(), "/tmp/elixir", envPath); err != nil {
		t.Errorf("second CreateEnv: %v", err)
	}
}

// TestGenericEnvManager_NoOps confirms InstallDeps + InstallExtras
// silently succeed regardless of input. This is intentional — Generic
// EnvManager is a fallback and not expected to receive deps; the
// contract is "drop them" not "error".
//
// TestGenericEnvManager_NoOps 确认 InstallDeps + InstallExtras 不论入参
// 都静默成功。故意为之——Generic EnvManager 是兜底，不该收 deps；契约是
// "扔掉" 不是 "报错"。
func TestGenericEnvManager_NoOps(t *testing.T) {
	gm := NewGenericEnvManager("zig")
	ctx := context.Background()
	if err := gm.InstallDeps(ctx, "/tmp/zig", "/tmp/env", []string{"some-pkg"}, nil); err != nil {
		t.Errorf("InstallDeps with deps: want nil, got %v", err)
	}
	if err := gm.InstallExtras(ctx, "/tmp/zig", "/tmp/env", []string{"foo"}, nil); err != nil {
		t.Errorf("InstallExtras with extras: want nil, got %v", err)
	}
}

func TestGenericEnvManager_EnvBin_ReturnsBinNameOnly(t *testing.T) {
	gm := NewGenericEnvManager("zig")
	got := gm.EnvBin("/data/envs/conv/cv_abc:zig", "zig")
	if got != "zig" {
		t.Errorf("EnvBin = %q, want %q (no path prefix; caller resolves via PATH)", got, "zig")
	}
}
