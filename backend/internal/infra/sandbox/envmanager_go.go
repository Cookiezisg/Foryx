// envmanager_go.go — Go-toolchain-backed EnvManager for Go plugin envs.
//
// Per-env isolation strategy:
//
//   - GOPATH=<envPath>/gopath  → Go's module cache, source proxies, and
//     `go install` output all land inside the env. Each env has its own
//     module download cache; Go's global cache (default ~/go) is bypassed.
//   - GOBIN=<envPath>/bin (derived from GOPATH/bin) — `go install` puts
//     compiled binaries here.
//   - GOMODCACHE=<envPath>/gopath/pkg/mod (the standard layout under
//     GOPATH; we don't override it).
//
// As with Rust, this means each env redownloads modules even if they
// overlap with another env. v2 may share GOMODCACHE across envs (Go's
// module cache is content-addressable so it would actually work cleanly
// — unlike Cargo). Filed as future work; v1 keeps full isolation for
// simplicity.
//
// envmanager_go.go ——基于 Go toolchain 的 Go plugin env EnvManager。
//
// 隔离策略：GOPATH=<envPath>/gopath（Go 的 module cache、源 proxy、
// `go install` 输出都落在 env 里）+ GOBIN=GOPATH/bin。每 env 重下 modules；
// Go 全局 cache（默认 ~/go）被绕开。
//
// 跟 Rust 类似——v2 可考虑跨 env 共享 GOMODCACHE（Go 的 module cache 是
// content-addressable 实际可干净共享，跟 Cargo 不一样）。v1 全隔离图简单。

package sandbox

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// GoEnvManager satisfies sandboxdomain.EnvManager for Go.
//
// GoEnvManager 满足 sandboxdomain.EnvManager 的 Go 实现。
type GoEnvManager struct{}

// NewGoEnvManager constructs the manager. As with Rust, no construction
// param — go binary path derived from runtimePath at call time.
//
// NewGoEnvManager 构造 manager。同 Rust，无构造参数——go 二进制路径调用时
// 从 runtimePath 派生。
func NewGoEnvManager() *GoEnvManager { return &GoEnvManager{} }

// Kind reports the dispatch key.
//
// Kind 报告派发键。
func (g *GoEnvManager) Kind() string { return "go" }

// CreateEnv mkdirs envPath/gopath (GOPATH) and the bin/ subdir. Idempotent.
//
// CreateEnv mkdir envPath/gopath（GOPATH）和 bin/ 子目录。幂等。
func (g *GoEnvManager) CreateEnv(ctx context.Context, runtimePath, envPath string) error {
	gopath := filepath.Join(envPath, "gopath")
	binDir := filepath.Join(envPath, "bin")
	if _, err := os.Stat(gopath); err == nil {
		return nil
	}
	if err := os.MkdirAll(gopath, 0o755); err != nil {
		return fmt.Errorf("sandbox.GoEnvManager.CreateEnv: mkdir gopath: %w (env: %w)", err, sandboxdomain.ErrEnvCreateFailed)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("sandbox.GoEnvManager.CreateEnv: mkdir bin: %w (env: %w)", err, sandboxdomain.ErrEnvCreateFailed)
	}
	return nil
}

// InstallDeps runs `go install <pkg@version>...` per dep with GOPATH +
// GOBIN pinned to the env. deps are Go-style import paths with optional
// @version (e.g. "github.com/example/cli@latest"). Library deps for Go
// modules live in user-managed go.mod files, not sandbox envs — same
// boundary as Rust.
//
// InstallDeps 对每个 dep 跑 `go install <pkg@version>` + GOPATH/GOBIN 钉
// env。deps 是 Go 风格 import path 可带 @version（如
// "github.com/example/cli@latest"）。库 deps 走用户管的 go.mod 文件，不归
// sandbox env——跟 Rust 同边界。
func (g *GoEnvManager) InstallDeps(ctx context.Context, runtimePath, envPath string, deps []string, stream sandboxdomain.ProgressFunc) error {
	if len(deps) == 0 {
		return nil
	}
	goBin := filepath.Join(runtimePath, "go"+exeSuffix())
	gopath := filepath.Join(envPath, "gopath")
	gobin := filepath.Join(envPath, "bin")

	for _, dep := range deps {
		cmd := exec.CommandContext(ctx, goBin, "install", dep)
		cmd.Env = append(os.Environ(),
			"GOPATH="+gopath,
			"GOBIN="+gobin,
		)

		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("sandbox.GoEnvManager.InstallDeps: stderr pipe %s: %w", dep, err)
		}
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("sandbox.GoEnvManager.InstallDeps: start %s: %w", dep, err)
		}

		if stream != nil {
			scanner := bufio.NewScanner(stderrPipe)
			for scanner.Scan() {
				stream("installing-deps", scanner.Text(), -1)
			}
		} else {
			_, _ = io.Copy(io.Discard, stderrPipe)
		}

		if err := cmd.Wait(); err != nil {
			return fmt.Errorf("sandbox.GoEnvManager.InstallDeps %s: %w", dep, sandboxdomain.ErrDepInstallFailed)
		}
	}
	return nil
}

// InstallExtras is a no-op — Go plugins don't have an extras concept.
//
// InstallExtras no-op——Go plugin 无 extras 概念。
func (g *GoEnvManager) InstallExtras(ctx context.Context, runtimePath, envPath string, extras []string, stream sandboxdomain.ProgressFunc) error {
	return nil
}

// EnvBin returns the absolute path to a binary inside the env's bin/
// dir (where GOBIN points). Adds .exe on Windows when caller didn't.
//
// EnvBin 返 env 的 bin/ 目录中某 binary 绝对路径（GOBIN 指向那里）。
// Windows 上调用方未带时加 .exe。
func (g *GoEnvManager) EnvBin(envPath, binName string) string {
	if runtime.GOOS == "windows" && filepath.Ext(binName) == "" {
		binName += ".exe"
	}
	return filepath.Join(envPath, "bin", binName)
}

// EnvDir returns the env root.
//
// EnvDir 返 env 根目录。
func (g *GoEnvManager) EnvDir(envPath string) string { return envPath }
