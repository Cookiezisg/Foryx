package sandbox

import (
	"context"
	"reflect"
	"testing"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

func TestDockerInstaller_PureContract(t *testing.T) {
	d := NewDockerInstaller()
	if d.Kind() != "docker" {
		t.Errorf("Kind() = %q", d.Kind())
	}
	// The image ref is already canonical and is its own locator — no host binary.
	// 镜像 ref 已是规范形、本身即定位符——无宿主 binary。
	const ref = "ghcr.io/github/github-mcp-server:1.1.2"
	if got := d.NormalizeVersion(ref); got != ref {
		t.Errorf("NormalizeVersion mutated ref: %q", got)
	}
	if got, _ := d.Locate(ref, "/ignored"); got != ref {
		t.Errorf("Locate = %q, want the image ref", got)
	}
	if got, _ := d.ResolveDefault(context.Background()); got != "" {
		t.Errorf("ResolveDefault = %q, want empty (docker has no implicit default)", got)
	}
}

// TestDockerResolveExec_WrapsRun: opts.Cmd is wrapped in `docker run --rm -i
// <image> <cmd> <args>`; cwd is empty (the workdir lives in the container).
//
// opts.Cmd 被包进 `docker run --rm -i <image> <cmd> <args>`；cwd 空（工作目录在容器内）。
func TestDockerResolveExec_WrapsRun(t *testing.T) {
	d := NewDockerEnvManager()
	cmd, args, cwd := d.ResolveExec("ghcr.io/org/img:1.0", "/ignored-env", sandboxdomain.SpawnOpts{
		Cmd:  "mcp-server",
		Args: []string{"--port", "8080"},
	})
	if cmd != "docker" {
		t.Errorf("cmd = %q, want docker", cmd)
	}
	want := []string{"run", "--rm", "-i", "ghcr.io/org/img:1.0", "mcp-server", "--port", "8080"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("args = %v, want %v", args, want)
	}
	if cwd != "" {
		t.Errorf("cwd = %q, want empty", cwd)
	}
}

// TestDockerResolveExec_EnvAsSortedFlags: opts.Env becomes -e flags in sorted
// key order (a host process env would not reach inside the container).
//
// opts.Env 变成按 key 排序的 -e flag（宿主进程 env 进不了容器）。
func TestDockerResolveExec_EnvAsSortedFlags(t *testing.T) {
	d := NewDockerEnvManager()
	_, args, _ := d.ResolveExec("img:latest", "", sandboxdomain.SpawnOpts{
		Cmd: "x",
		Env: map[string]string{"ZED": "9", "ALPHA": "1", "MID": "5"},
	})
	want := []string{"run", "--rm", "-i", "-e", "ALPHA=1", "-e", "MID=5", "-e", "ZED=9", "img:latest", "x"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("env flags not sorted/placed correctly:\n got %v\nwant %v", args, want)
	}
}
