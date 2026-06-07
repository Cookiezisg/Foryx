package sandbox

import (
	"context"
	"runtime"
	"testing"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// fakeToolRegistry resolves "uv" to a fixed path (uvx is derived beside it).
type fakeToolRegistry struct{ uvBin string }

func (f fakeToolRegistry) EnsureTool(_ context.Context, kind, _ string) (string, error) {
	if kind == "uv" {
		return f.uvBin, nil
	}
	return "", nil
}

// TestNodeResolveExec_RuntimeToolVsEnvDep: npx (bundled runner) resolves under the runtime
// install dir; a plain bare name resolves under the env's node_modules/.bin.
//
// TestNodeResolveExec_RuntimeToolVsEnvDep：npx（自带 runner）按 runtime install dir 解析；普通裸名
// 按 env 的 node_modules/.bin 解析。
func TestNodeResolveExec_RuntimeToolVsEnvDep(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path assertions are POSIX-shaped")
	}
	n := NewNodeEnvManager()
	cmd, args, _ := n.ResolveExec("/abs/node", "/env", sandboxdomain.SpawnOpts{Cmd: "npx", Args: []string{"-y", "@x/y"}})
	if cmd != "/abs/node/bin/npx" {
		t.Fatalf("npx should resolve to <runtime>/bin/npx, got %q", cmd)
	}
	if len(args) != 2 || args[0] != "-y" {
		t.Fatalf("args passed through, got %v", args)
	}
	cmd2, _, _ := n.ResolveExec("/abs/node", "/env", sandboxdomain.SpawnOpts{Cmd: "eslint"})
	if cmd2 != "/env/node_modules/.bin/eslint" {
		t.Fatalf("env dep should resolve to node_modules/.bin, got %q", cmd2)
	}
}

// TestPythonResolveExec_UvRunner: uvx resolves beside the aqua-installed uv (via ToolRegistry);
// a plain bare name resolves under the env's venv.
//
// TestPythonResolveExec_UvRunner：uvx 解析到 aqua 装的 uv 同目录（经 ToolRegistry）；普通裸名按
// env 的 venv 解析。
func TestPythonResolveExec_UvRunner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path assertions are POSIX-shaped")
	}
	p := NewPythonEnvManager(fakeToolRegistry{uvBin: "/aqua/uv/uv"})
	cmd, _, _ := p.ResolveExec("", "/env", sandboxdomain.SpawnOpts{Cmd: "uvx", Args: []string{"markitdown-mcp"}})
	if cmd != "/aqua/uv/uvx" {
		t.Fatalf("uvx should resolve beside uv, got %q", cmd)
	}
	cmd2, _, _ := p.ResolveExec("", "/env", sandboxdomain.SpawnOpts{Cmd: "black"})
	if cmd2 != "/env/.venv/bin/black" {
		t.Fatalf("env dep should resolve to venv/bin, got %q", cmd2)
	}
}

// TestDotnetResolveExec_Dnx: dnx resolves at the dotnet install dir's TOP LEVEL (not bin/).
//
// TestDotnetResolveExec_Dnx：dnx 解析到 dotnet install dir 顶层（不在 bin/）。
func TestDotnetResolveExec_Dnx(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path assertions are POSIX-shaped")
	}
	d := NewDotnetEnvManager()
	cmd, args, _ := d.ResolveExec("/abs/dotnet", "", sandboxdomain.SpawnOpts{Cmd: "dnx", Args: []string{"NuGet.Mcp.Server"}})
	if cmd != "/abs/dotnet/dnx" {
		t.Fatalf("dnx should resolve to <runtime>/dnx (top level), got %q", cmd)
	}
	if len(args) != 1 || args[0] != "NuGet.Mcp.Server" {
		t.Fatalf("args passed through, got %v", args)
	}
}
