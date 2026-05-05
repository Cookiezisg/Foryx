// envmanager_rust.go — cargo-backed EnvManager for Rust plugin envs.
//
// Per-env isolation strategy:
//
//   - CARGO_HOME=<envPath>/.cargo  → registry index, cached crate sources
//     and built artifacts all land inside the env. Each env has its own
//     copy of crate sources for installed deps; cargo's global store is
//     bypassed (no cross-env hardlinks like uv/pnpm — Cargo doesn't
//     have a content-addressable global store, so we lean on per-env
//     CARGO_HOME isolation rather than global sharing).
//   - `cargo install --root=<envPath>` puts compiled binaries under
//     <envPath>/bin/. Caller spawns from there.
//
// Trade-off vs sharing CARGO_HOME globally: more disk per env (each env
// downloads its own crate index ~200 MB on first install). v2 may
// consider sharing the registry index across envs while keeping per-env
// install/build dirs — but Cargo's design assumes the index belongs to
// one user, sharing across envs requires careful concurrent-write
// handling we won't tackle in v1.
//
// envmanager_rust.go ——基于 cargo 的 Rust plugin env EnvManager。
//
// 隔离策略：CARGO_HOME=<envPath>/.cargo（registry 索引、源码缓存、构建
// artifact 都在 env 内）+ `cargo install --root=<envPath>` 编译产物到
// <envPath>/bin/。每 env 独立 crate 源码 copy；不享全局 store
// （Cargo 无 content-addressable 全局 store 概念）。
//
// 跟"全局共享 CARGO_HOME"权衡：每 env 多用磁盘（首次 install 下 ~200 MB
// 索引）。v2 可考虑跨 env 共享索引但保 per-env install/build——Cargo 设计
// 假设索引属一个 user，跨 env 共享需小心并发写处理，v1 不碰。

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

// RustEnvManager satisfies sandboxdomain.EnvManager for Rust.
//
// RustEnvManager 满足 sandboxdomain.EnvManager 的 Rust 实现。
type RustEnvManager struct{}

// NewRustEnvManager constructs the manager. cargo binary path is
// resolved via the runtimePath argument at call time (no construction
// param) — every cargo invocation derives `<runtimePath>/cargo` so we
// stay aligned with whichever Rust runtime version the env was bound to.
//
// NewRustEnvManager 构造 manager。cargo 二进制路径在调用时通过
// runtimePath 参数解析（无构造参数）——每次 cargo 调用派生
// `<runtimePath>/cargo`，跟 env 绑的那个 Rust runtime 版本一致。
func NewRustEnvManager() *RustEnvManager { return &RustEnvManager{} }

// Kind reports the dispatch key.
//
// Kind 报告派发键。
func (r *RustEnvManager) Kind() string { return "rust" }

// CreateEnv mkdirs envPath + envPath/.cargo (CARGO_HOME). Idempotent.
//
// CreateEnv mkdir envPath + envPath/.cargo（CARGO_HOME）。幂等。
func (r *RustEnvManager) CreateEnv(ctx context.Context, runtimePath, envPath string) error {
	cargoHome := filepath.Join(envPath, ".cargo")
	if _, err := os.Stat(cargoHome); err == nil {
		return nil
	}
	if err := os.MkdirAll(cargoHome, 0o755); err != nil {
		return fmt.Errorf("sandbox.RustEnvManager.CreateEnv: mkdir cargo home: %w (env: %w)", err, sandboxdomain.ErrEnvCreateFailed)
	}
	return nil
}

// InstallDeps runs `cargo install --root=<envPath> <deps...>` for each
// crate. cargo install is meant for end-user binary tools (not library
// deps); the typical Forge/MCP plugin use case is "install some Rust
// CLI tool", which is exactly what this targets. Library deps (i.e.
// adding crates to a Cargo.toml workspace) are out of scope — that
// belongs in user-managed Cargo projects, not sandbox-managed envs.
//
// InstallDeps 对每个 crate 跑 `cargo install --root=<envPath> <deps...>`。
// cargo install 专给终端用户 binary 工具（非库 deps）；典型 Forge/MCP plugin
// 用例就是"装某 Rust CLI 工具"，正中靶心。库 deps（即给 Cargo.toml workspace
// 加 crate）超范围——属于用户管的 Cargo project，不归 sandbox 管的 env。
func (r *RustEnvManager) InstallDeps(ctx context.Context, runtimePath, envPath string, deps []string, stream sandboxdomain.ProgressFunc) error {
	if len(deps) == 0 {
		return nil
	}
	cargoBin := filepath.Join(runtimePath, "cargo"+exeSuffix())
	args := append([]string{"install", "--root=" + envPath}, deps...)
	cmd := exec.CommandContext(ctx, cargoBin, args...)
	cmd.Env = append(os.Environ(), "CARGO_HOME="+filepath.Join(envPath, ".cargo"))

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("sandbox.RustEnvManager.InstallDeps: stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("sandbox.RustEnvManager.InstallDeps: start: %w", err)
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
		return fmt.Errorf("sandbox.RustEnvManager.InstallDeps %v: %w", deps, sandboxdomain.ErrDepInstallFailed)
	}
	return nil
}

// InstallExtras is a no-op — Rust plugins don't have an extras concept
// (no equivalent to Playwright's browser binary download).
//
// InstallExtras no-op——Rust plugin 无 extras 概念（无 Playwright 类的浏览器
// 二进制下载）。
func (r *RustEnvManager) InstallExtras(ctx context.Context, runtimePath, envPath string, extras []string, stream sandboxdomain.ProgressFunc) error {
	return nil
}

// EnvBin returns the absolute path to a binary inside the env's bin/
// dir (cargo install --root puts compiled binaries there). On Windows
// the .exe suffix is added if the caller didn't include one.
//
// EnvBin 返 env 的 bin/ 目录中某 binary 绝对路径（cargo install --root
// 把编译产物放那）。Windows 上调用方未带后缀时加 .exe。
func (r *RustEnvManager) EnvBin(envPath, binName string) string {
	if runtime.GOOS == "windows" && filepath.Ext(binName) == "" {
		binName += ".exe"
	}
	return filepath.Join(envPath, "bin", binName)
}

// EnvDir returns the env root.
//
// EnvDir 返 env 根目录。
func (r *RustEnvManager) EnvDir(envPath string) string { return envPath }

// exeSuffix returns ".exe" on Windows, "" elsewhere — used to derive
// the cargo binary path from a runtimePath (mise installs put cargo at
// `<runtimePath>/cargo` on unix, `<runtimePath>/cargo.exe` on Windows).
//
// exeSuffix Windows 上返 ".exe"，其他返 ""——用来从 runtimePath 派生 cargo
// 二进制路径（mise install 在 unix 上把 cargo 放 `<runtimePath>/cargo`，
// Windows 是 `<runtimePath>/cargo.exe`）。
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
