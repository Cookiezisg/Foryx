// envmanager_go_test.go — pure-function unit tests for GoEnvManager.
// Real `go install` belongs in the D9 pipeline suite.
//
// envmanager_go_test.go ——GoEnvManager pure-function 单测。真 `go install`
// 归 D9 pipeline 套。

package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

var _ sandboxdomain.EnvManager = (*GoEnvManager)(nil)

func TestGoEnvManager_Kind(t *testing.T) {
	gm := NewGoEnvManager()
	if got := gm.Kind(); got != "go" {
		t.Errorf("Kind() = %q, want go", got)
	}
}

func TestGoEnvManager_CreateEnv_MakesGopathAndBin(t *testing.T) {
	gm := NewGoEnvManager()
	envPath := filepath.Join(t.TempDir(), "envs", "conv", "cv_abc:go")
	if err := gm.CreateEnv(context.Background(), "/tmp/go", envPath); err != nil {
		t.Fatalf("CreateEnv: %v", err)
	}
	if _, err := os.Stat(filepath.Join(envPath, "gopath")); err != nil {
		t.Errorf("gopath dir not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(envPath, "bin")); err != nil {
		t.Errorf("bin dir not created: %v", err)
	}
}

func TestGoEnvManager_EnvBin_PerOS(t *testing.T) {
	gm := NewGoEnvManager()
	got := gm.EnvBin("/data/envs/conv/cv:go", "gopls")
	var want string
	if runtime.GOOS == "windows" {
		want = "/data/envs/conv/cv:go/bin/gopls.exe"
	} else {
		want = "/data/envs/conv/cv:go/bin/gopls"
	}
	if got != want {
		t.Errorf("EnvBin = %q, want %q", got, want)
	}
}
