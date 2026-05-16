package sandbox

import (
	"runtime"
	"testing"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

var _ sandboxdomain.EnvManager = (*PythonEnvManager)(nil)

func TestPythonEnvManager_Kind(t *testing.T) {
	pm := NewPythonEnvManager(newFakeToolRegistry(map[string]string{"uv": "/tmp/uv"}))
	if got := pm.Kind(); got != "python" {
		t.Errorf("Kind() = %q, want python", got)
	}
}

func TestPythonEnvManager_EnvBin_PerOS(t *testing.T) {
	pm := NewPythonEnvManager(newFakeToolRegistry(map[string]string{"uv": "/tmp/uv"}))
	got := pm.EnvBin("/data/envs/forge/abc", "python")

	var want string
	if runtime.GOOS == "windows" {
		want = "/data/envs/forge/abc/.venv/Scripts/python.exe"
	} else {
		want = "/data/envs/forge/abc/.venv/bin/python"
	}
	if got != want {
		t.Errorf("EnvBin = %q, want %q", got, want)
	}
}

func TestPythonEnvManager_EnvBin_PreservesExplicitExtension(t *testing.T) {
	pm := NewPythonEnvManager(newFakeToolRegistry(map[string]string{"uv": "/tmp/uv"}))
	got := pm.EnvBin("/data/envs/forge/abc", "uvicorn.exe")

	var want string
	if runtime.GOOS == "windows" {
		want = "/data/envs/forge/abc/.venv/Scripts/uvicorn.exe"
	} else {
		want = "/data/envs/forge/abc/.venv/bin/uvicorn.exe"
	}
	if got != want {
		t.Errorf("EnvBin = %q, want %q", got, want)
	}
}

func TestPythonEnvManager_EnvDir_ReturnsInputUnchanged(t *testing.T) {
	pm := NewPythonEnvManager(newFakeToolRegistry(map[string]string{"uv": "/tmp/uv"}))
	if got := pm.EnvDir("/data/envs/conv/cv_abc_python"); got != "/data/envs/conv/cv_abc_python" {
		t.Errorf("EnvDir = %q, want input unchanged", got)
	}
}
