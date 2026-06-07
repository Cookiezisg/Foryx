package sandbox

import (
	"context"
	"path/filepath"
	"runtime"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// DotnetEnvManager runs .NET MCP servers via dnx — .NET 10's "package runner" (pulls a NuGet
// package and runs it, like npx/uvx). dnx sits at the runtime install dir's TOP LEVEL
// (<install>/dnx — verified on a real machine via `mise install dotnet@10.0.300`), not under
// bin/. dnx manages its own package cache, so there's no per-owner env: CreateEnv / InstallDeps
// are no-ops. Paired with a mise dotnet RuntimeInstaller (registered in cmd/server, M7).
//
// DotnetEnvManager 经 dnx 跑 .NET MCP server——.NET 10 的「包运行器」（拉 NuGet 包即跑，像
// npx/uvx）。dnx 在 runtime install dir 顶层（<install>/dnx——经 `mise install dotnet@10.0.300`
// 真机验证），不在 bin/。dnx 自管包缓存，故无 per-owner env：CreateEnv / InstallDeps 为 no-op。
// 与 mise dotnet RuntimeInstaller 配对（在 cmd/server 注册，M7）。
type DotnetEnvManager struct{}

// NewDotnetEnvManager constructs the manager.
//
// NewDotnetEnvManager 构造 manager。
func NewDotnetEnvManager() *DotnetEnvManager { return &DotnetEnvManager{} }

var _ sandboxdomain.EnvManager = (*DotnetEnvManager)(nil)

func (*DotnetEnvManager) Kind() string { return "dotnet" }

// CreateEnv is a no-op — dnx needs no per-owner isolation dir.
//
// CreateEnv 为 no-op——dnx 不需要 per-owner 隔离目录。
func (*DotnetEnvManager) CreateEnv(context.Context, string, string) error { return nil }

// InstallDeps is a no-op — dnx pulls the package at run time.
//
// InstallDeps 为 no-op——dnx 运行时才拉包。
func (*DotnetEnvManager) InstallDeps(context.Context, string, string, []string, sandboxdomain.ProgressFunc) error {
	return nil
}

// ResolveExec resolves the dnx runner against the ABSOLUTE dotnet install dir (runtimeRef);
// dnx is at the top level, not bin/. A path-like cmd passes through.
//
// ResolveExec 把 dnx runner 按绝对 dotnet install dir（runtimeRef）解析；dnx 在顶层、不在 bin/。
// 路径式 cmd 原样透传。
func (*DotnetEnvManager) ResolveExec(runtimeRef string, _ string, opts sandboxdomain.SpawnOpts) (cmd string, args []string, cwd string) {
	cmd = opts.Cmd
	if cmd == "dnx" || cmd == "dotnet" {
		if runtime.GOOS == "windows" {
			cmd += ".exe"
		}
		cmd = filepath.Join(runtimeRef, cmd)
	}
	return cmd, opts.Args, ""
}
