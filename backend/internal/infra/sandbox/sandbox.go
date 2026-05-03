// Package sandbox executes user forge code in per-version Python venvs managed
// by uv. Sandbox is the only struct callers interact with — it owns the
// bundled uv binary path, the bundled Python interpreter path, the per-forge
// sync mutex, and implements the forgeapp.Sandbox interface (Bootstrap / Sync /
// Run / Destroy / DestroyEnv / WriteCodeFile).
//
// File layout under <dataDir>:
//
//	bin/uv                                   ← bundled uv binary (Bootstrap)
//	bin/python/...                           ← bundled python-build-standalone (Bootstrap)
//	uv-cache/                                ← UV_CACHE_DIR; isolated from user's ~/.cache/uv
//	forges/<forgeID>/envs/<envID>/           ← venv keyed by deps hash; multi versions share
//	forges/<forgeID>/versions/<vID>/main.py  ← per-version code file
//
// uv binary path and Python interpreter path are derived purely from DataDir
// (see paths.go::bundledUVPath / bundledPythonPath); callers don't pass them
// — Bootstrap only has to extract resources to those well-known locations.
//
// Bootstrap must run successfully before any Sync / Run; until then, Sync /
// Run return errBootstrapPending.
//
// Package sandbox 在每版本独立的 uv 管理 venv 中执行 forge 代码。
// Sandbox 是唯一对外类型，持有捆绑 uv 二进制路径、捆绑 Python 解释器路径、
// per-forge sync 互斥，实现 forgeapp.Sandbox 接口。
//
// 磁盘布局见上方英文段。uv 二进制路径和 Python 解释器路径都由 DataDir 推导
// （见 paths.go::bundledUVPath / bundledPythonPath）——调用方不传，
// Bootstrap 只负责把资源解压到这俩约定路径。
//
// Bootstrap 必须先成功执行；之前调 Sync / Run 返 errBootstrapPending。
package sandbox

import (
	"errors"
	"os"

	"go.uber.org/zap"
)

// Config wires Sandbox with the data dir + default Python spec + logger.
// uv / Python paths are derived from DataDir, not configured separately.
//
// Config 装配 Sandbox 的数据目录 + 默认 Python 约束 + logger。
// uv / Python 路径由 DataDir 推导，不单独配置。
type Config struct {
	// DataDir is the root directory; all sandbox files live under it.
	// DataDir 是根目录；所有沙箱文件都在它下面。
	DataDir string

	// DefaultPython is the spec used when ForgeVersion.PythonVersion is empty,
	// e.g. ">=3.12". Embedded in the rendered pyproject.toml.
	//
	// DefaultPython 是 ForgeVersion.PythonVersion 为空时的默认版本约束，
	// 如 ">=3.12"。会嵌入渲染出的 pyproject.toml。
	DefaultPython string

	// Logger is required; New panics on nil.
	// Logger 必填；New 在 nil 时 panic。
	Logger *zap.Logger
}

// Sandbox is the entry point for all sandbox operations. Field access is
// package-internal — callers only see the methods (Bootstrap / Sync / Run /
// Destroy / DestroyEnv / WriteCodeFile) which together implement the
// forgeapp.Sandbox port.
//
// Sandbox 是所有沙箱操作的入口。字段为包内访问——调用方只看到方法集，
// 它们共同实现 forgeapp.Sandbox 端口。
type Sandbox struct {
	cfg    Config
	log    *zap.Logger
	syncMu *forgeMutexMap // per-forge sync 互斥；Run 不走此 map

	// bootstrapped is set true after Bootstrap returns nil. Sync / Run
	// guard against ops before it's true.
	//
	// bootstrapped 在 Bootstrap 成功返回后置 true。Sync / Run 前置检查。
	bootstrapped bool
}

// errBootstrapPending is returned by Sync / Run if called before Bootstrap
// has succeeded. The forgeapp layer maps this to forgedomain.ErrSandboxUnavailable.
//
// errBootstrapPending 在 Bootstrap 未成功前调用 Sync / Run 时返。
// forgeapp 层映射到 forgedomain.ErrSandboxUnavailable。
var errBootstrapPending = errors.New("sandbox: bootstrap not yet completed")

// New constructs a Sandbox. Logger is required (panics on nil). Sync / Run
// must not be called before Bootstrap has returned nil.
//
// New 构造 Sandbox。Logger 必填（nil 触发 panic）。Bootstrap 返回 nil 前
// 切勿调用 Sync / Run。
func New(cfg Config) *Sandbox {
	if cfg.Logger == nil {
		panic("sandbox.New: Logger is nil")
	}
	return &Sandbox{
		cfg:    cfg,
		log:    cfg.Logger,
		syncMu: newForgeMutexMap(),
	}
}

// UVPath returns the absolute path to the bundled uv binary. The file exists
// only after Bootstrap has succeeded.
//
// UVPath 返回捆绑 uv 二进制的绝对路径。文件仅在 Bootstrap 成功后存在。
func (s *Sandbox) UVPath() string {
	return bundledUVPath(s.cfg.DataDir)
}

// PythonPath returns the absolute path to the bundled Python interpreter
// (raw, not inside any venv). Used by uv as `--python` target and exposed
// to ASTParser (which uses Python stdlib only, no venv needed).
//
// PythonPath 返回捆绑 Python 解释器（raw，不在任何 venv 里）的绝对路径。
// uv 用它作 `--python` 目标；ASTParser 也用它（仅 stdlib，不需 venv）。
func (s *Sandbox) PythonPath() string {
	return bundledPythonPath(s.cfg.DataDir)
}

// ensureReady returns errBootstrapPending if Bootstrap hasn't succeeded.
// Call from every entry method (Sync / Run / Destroy / etc) before doing
// any IO.
//
// ensureReady 在 Bootstrap 未成功前返 errBootstrapPending。
// 每个入口方法在做 IO 前先调它。
func (s *Sandbox) ensureReady() error {
	if !s.bootstrapped {
		return errBootstrapPending
	}
	return nil
}

// withUVEnv returns os.Environ() plus the UV_* environment variables that
// isolate Sandbox subprocess state from the user's system uv config / cache:
//
//   - UV_CACHE_DIR points at <dataDir>/uv-cache, not the user's ~/.cache/uv
//   - UV_NO_CONFIG=1 ignores user-global ~/.config/uv settings
//   - UV_NO_PROGRESS=1 keeps stderr free of fancy spinner output (we parse it)
//   - UV_PYTHON forces uv to use the bundled Python (no auto-download)
//   - PYTHONDONTWRITEBYTECODE=1 prevents __pycache__ pollution in forge dirs
//
// Used by sync.go / run.go when spawning uv subprocesses; tests (or callers
// that don't want subprocess isolation) bypass this and build env directly.
//
// withUVEnv 返回 os.Environ() + 一组 UV_* 环境变量，把 Sandbox 子进程状态
// 跟用户系统 uv 配置 / 缓存隔离：
//
//   - UV_CACHE_DIR 指向 <dataDir>/uv-cache，不动用户 ~/.cache/uv
//   - UV_NO_CONFIG=1 忽略用户全局 ~/.config/uv
//   - UV_NO_PROGRESS=1 让 stderr 干净不含 spinner（我们要解析它）
//   - UV_PYTHON 强制 uv 用捆绑 Python（不去自动下载）
//   - PYTHONDONTWRITEBYTECODE=1 防 __pycache__ 污染 forge 目录
//
// sync.go / run.go 起 uv 子进程时调；测试或不需要隔离的调用方可绕过自构造 env。
func (s *Sandbox) withUVEnv() []string {
	base := os.Environ()
	overlay := []string{
		"UV_CACHE_DIR=" + uvCacheDir(s.cfg.DataDir),
		"UV_NO_CONFIG=1",
		"UV_NO_PROGRESS=1",
		"UV_PYTHON=" + s.PythonPath(),
		"PYTHONDONTWRITEBYTECODE=1",
	}
	out := make([]string, 0, len(base)+len(overlay))
	out = append(out, base...)
	out = append(out, overlay...)
	return out
}
