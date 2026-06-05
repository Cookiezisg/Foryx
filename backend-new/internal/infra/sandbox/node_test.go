package sandbox

import (
	"path/filepath"
	"runtime"
	"testing"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

func TestNodeEnvManager_Kind(t *testing.T) {
	if got := NewNodeEnvManager().Kind(); got != "node" {
		t.Errorf("Kind() = %q, want node", got)
	}
}

// TestNodeResolveExec_BareCommand: a plain binary resolves into node_modules/.bin.
//
// 裸 binary 解析到 node_modules/.bin。
func TestNodeResolveExec_BareCommand(t *testing.T) {
	n := NewNodeEnvManager()
	cmd, args, cwd := n.ResolveExec("ignored", "/srv/env", sandboxdomain.SpawnOpts{
		Cmd:  "tsc",
		Args: []string{"--noEmit"},
	})
	want := filepath.Join("/srv/env", "node_modules", ".bin", nodeBinFor("tsc"))
	if cmd != want {
		t.Errorf("cmd = %q, want %q", cmd, want)
	}
	if cwd != "/srv/env" {
		t.Errorf("cwd = %q, want /srv/env", cwd)
	}
	if len(args) != 1 || args[0] != "--noEmit" {
		t.Errorf("args = %v", args)
	}
}

func nodeBinFor(bin string) string {
	if runtime.GOOS == "windows" {
		return bin + ".cmd"
	}
	return bin
}

func TestNodeResolveExec_PathLikeCommand_PassThrough(t *testing.T) {
	n := NewNodeEnvManager()
	cmd, _, _ := n.ResolveExec("", "/srv/env", sandboxdomain.SpawnOpts{Cmd: "/usr/local/bin/node"})
	if cmd != "/usr/local/bin/node" {
		t.Errorf("absolute cmd rewritten: %q", cmd)
	}
}
