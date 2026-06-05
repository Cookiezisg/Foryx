package sandbox

import (
	"path/filepath"
	"runtime"
	"testing"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

func TestPythonEnvManager_Kind(t *testing.T) {
	if got := NewPythonEnvManager(nil).Kind(); got != "python" {
		t.Errorf("Kind() = %q, want python", got)
	}
}

// TestPythonResolveExec_BareCommand: a plain binary name resolves to the venv's
// bin dir, with cwd = envPath and args passed through.
//
// 裸 binary 名解析到 venv 的 bin 目录，cwd = envPath，args 透传。
func TestPythonResolveExec_BareCommand(t *testing.T) {
	p := NewPythonEnvManager(nil)
	cmd, args, cwd := p.ResolveExec("ignored-runtime-ref", "/srv/env", sandboxdomain.SpawnOpts{
		Cmd:  "pytest",
		Args: []string{"-v", "tests/"},
	})
	want := filepath.Join("/srv/env", ".venv", venvBinSubdir(), pythonExeFor("pytest"))
	if cmd != want {
		t.Errorf("cmd = %q, want %q", cmd, want)
	}
	if cwd != "/srv/env" {
		t.Errorf("cwd = %q, want /srv/env", cwd)
	}
	if len(args) != 2 || args[0] != "-v" {
		t.Errorf("args = %v, want [-v tests/]", args)
	}
}

// pythonExeFor mirrors the .exe suffix binPath adds on Windows.
func pythonExeFor(bin string) string {
	if runtime.GOOS == "windows" {
		return bin + ".exe"
	}
	return bin
}

// TestPythonResolveExec_PathLikeCommand_PassThrough: an absolute or path-like cmd
// is not rewritten into the venv.
//
// 绝对/路径式 cmd 不被改写进 venv。
func TestPythonResolveExec_PathLikeCommand_PassThrough(t *testing.T) {
	p := NewPythonEnvManager(nil)
	for _, raw := range []string{"/usr/bin/python3", "./local.sh", "../rel/bin"} {
		cmd, _, _ := p.ResolveExec("", "/srv/env", sandboxdomain.SpawnOpts{Cmd: raw})
		if cmd != raw {
			t.Errorf("path-like %q rewritten to %q", raw, cmd)
		}
	}
}
