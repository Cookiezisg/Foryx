package sandbox

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"go.uber.org/zap"
)

// TestSmokeBootstrap_FromDevResources verifies the full Bootstrap flow against
// real resources downloaded by `make download-resources`. Gated by
// FORGIFY_DEV_RESOURCES so unit-test runs / CI / offline machines skip it.
//
// TestSmokeBootstrap_FromDevResources 用 `make download-resources` 拉的真资源
// 跑完整 Bootstrap 流程。FORGIFY_DEV_RESOURCES 门控，CI / 离线机器跳过。
func TestSmokeBootstrap_FromDevResources(t *testing.T) {
	resourcesDir := os.Getenv("FORGIFY_DEV_RESOURCES")
	if resourcesDir == "" {
		t.Skip("FORGIFY_DEV_RESOURCES not set; run `make download-resources` first")
	}

	dataDir := t.TempDir()
	logger, _ := zap.NewDevelopment()
	s := New(Config{
		DataDir:       dataDir,
		DefaultPython: ">=3.12",
		Logger:        logger,
	})

	if err := s.Bootstrap(context.Background(), resourcesDir); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	pythonPath := s.PythonPath()
	if _, err := os.Stat(pythonPath); err != nil {
		t.Fatalf("PythonPath %q does not exist: %v", pythonPath, err)
	}

	out, err := exec.Command(pythonPath, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("run python: %v: %s", err, out)
	}
	t.Logf("PythonPath=%s   version=%s", pythonPath, out)
}
