// sync.go: Sandbox.Sync materializes a per-EnvID venv directory by running
// `uv sync` with stderr piped through scanProgress (recognized stage lines
// dispatched to OnProgress callback; everything else buffered into errBuf
// for SyncError on failure).
//
// Sync is idempotent: if `<envDir>/.venv/` already exists, it returns nil
// without touching anything. Half-built envs surface naturally on the next
// Run when uv reports a missing package — punted to the LLM via tool_result
// (sandbox iter doc §11.1).
//
// sync.go：Sandbox.Sync 通过 `uv sync` 物化 EnvID 对应的 venv 目录，
// stderr 经 scanProgress 双路分流（识别行调 OnProgress，其他行缓存到
// errBuf 失败时透传 SyncError）。
//
// 幂等：`<envDir>/.venv/` 已存在直接返 nil 不动。半成品 venv 下次 Run 时
// 由 uv 自然报"包找不到"——punt 给 LLM 通过 tool_result 看到错误自救
// （沙箱迭代 §11.1）。

package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// SyncRequest is one materialize-this-EnvID order.
//
// SyncRequest 是一份"物化这个 EnvID"的指令。
type SyncRequest struct {
	ForgeID       string
	VersionID     string // for logging only — venv keyed by EnvID, not version
	EnvID         string
	Dependencies  []string
	PythonVersion string

	// OnProgress is invoked for each recognized uv stage line during sync
	// (resolving / preparing / installing). May be nil. The forgeapp layer
	// wires this to UpdateVersionEnvProgress + publishForgeSnapshot so the
	// frontend gets per-stage forge entity-state events (sandbox iter §5).
	//
	// OnProgress 在 sync 期间每识别到一个 uv stage 行就调用（resolving /
	// preparing / installing）。可为 nil。forgeapp 层把它接到
	// UpdateVersionEnvProgress + publishForgeSnapshot，让前端拿到每阶段
	// forge entity-state 事件（沙箱迭代 §5）。
	OnProgress func(stage, detail string)
}

// SyncError wraps a uv sync failure with the captured stderr text. The
// forgeapp layer reads SyncError.Stderr into ForgeVersion.EnvError so the
// LLM sees the actual resolver / network / build error and can call
// edit_forge to self-correct. errors.Is(err, target) walks Cause.
//
// SyncError 包装 uv sync 失败 + 捕获 stderr 文本。forgeapp 层把
// SyncError.Stderr 读进 ForgeVersion.EnvError，让 LLM 看到真实
// resolver / 网络 / 构建错误，调 edit_forge 自救。errors.Is(err, target)
// 走 Cause。
type SyncError struct {
	Cause  error
	Stderr string
}

func (e *SyncError) Error() string { return e.Stderr }
func (e *SyncError) Unwrap() error { return e.Cause }

// Sync materializes the venv directory for the given EnvID. Idempotent —
// when the .venv already exists it returns nil immediately without
// re-running uv. Failed syncs return *SyncError carrying both Cause and the
// captured stderr.
//
// Sync 物化指定 EnvID 的 venv 目录。幂等——.venv 已存在直接返 nil 不重跑
// uv。失败返 *SyncError 含 Cause 和捕获的 stderr。
func (s *Sandbox) Sync(ctx context.Context, req SyncRequest) error {
	if err := s.ensureReady(); err != nil {
		return err
	}

	unlock := s.syncMu.Lock(req.ForgeID)
	defer unlock()

	dir := envDir(s.cfg.DataDir, req.ForgeID, req.EnvID)

	// Idempotency: skip if .venv already there.
	// 幂等：.venv 已在则跳过。
	if _, err := os.Stat(filepath.Join(dir, ".venv")); err == nil {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir env dir: %w", err)
	}

	pyproject := renderPyproject(req.ForgeID, req.Dependencies, req.PythonVersion, s.cfg.DefaultPython)
	if err := writeAtomic(filepath.Join(dir, "pyproject.toml"), []byte(pyproject), 0o644); err != nil {
		return fmt.Errorf("write pyproject.toml: %w", err)
	}

	// `uv sync --project <dir>` — the bundled Python is selected via
	// UV_PYTHON env var (set by withUVEnv), not via the --python flag, so
	// every Sandbox subprocess uses the same Python uniformly.
	//
	// `uv sync --project <dir>` ——通过 UV_PYTHON env var（由 withUVEnv 设）
	// 选捆绑 Python，不用 --python flag；让 Sandbox 所有子进程统一用同一个
	// Python。
	cmd := exec.CommandContext(ctx, s.UVPath(), "sync", "--project", dir, "--no-progress")
	cmd.Env = s.withUVEnv()

	setupProcessGroup(cmd)
	cmd.Cancel = func() error { return killProcessGroup(cmd) }

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start uv sync: %w", err)
	}

	// scanProgress reads stderrPipe to EOF — uv closes it when done.
	// Recognized lines → OnProgress; everything else → errBuf.
	//
	// scanProgress 读 stderrPipe 到 EOF——uv 完成时关闭。识别行 →
	// OnProgress；其他 → errBuf。
	var errBuf bytes.Buffer
	scanProgress(stderrPipe, req.OnProgress, &errBuf)

	if err := cmd.Wait(); err != nil {
		return &SyncError{Cause: err, Stderr: errBuf.String()}
	}
	return nil
}

// writeAtomic writes data to path via tmp + rename — readers never see a
// half-written file.
//
// writeAtomic 通过 tmp + rename 写文件——读取方永远看不到半成品。
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
