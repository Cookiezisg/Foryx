package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"go.uber.org/zap"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

// ExtractMiseBinary writes the embedded mise binary into <sandboxRoot>/bin/mise; idempotent.
//
// ExtractMiseBinary 把 embed mise 二进制写到 <sandboxRoot>/bin/mise；幂等。
func ExtractMiseBinary(ctx context.Context, sandboxRoot string, log *zap.Logger) (string, error) {
	if len(miseBinary) == 0 {
		return "", fmt.Errorf("sandbox.ExtractMiseBinary: no mise binary embedded for %s/%s: %w",
			runtime.GOOS, runtime.GOARCH, sandboxdomain.ErrRuntimeInstallFailed)
	}

	binDir := filepath.Join(sandboxRoot, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("sandbox.ExtractMiseBinary: mkdir bin dir: %w", err)
	}

	binPath := filepath.Join(binDir, miseExeName())
	hashPath := filepath.Join(sandboxRoot, ".mise.hash")

	sum := sha256.Sum256(miseBinary)
	wantHash := hex.EncodeToString(sum[:])

	if existing, err := os.ReadFile(hashPath); err == nil && string(existing) == wantHash {
		if _, statErr := os.Stat(binPath); statErr == nil {
			log.Debug("mise already extracted (hash match)", zap.String("path", binPath))
			return binPath, nil
		}
	}

	tmp := binPath + ".tmp"
	if err := os.WriteFile(tmp, miseBinary, 0o755); err != nil {
		return "", fmt.Errorf("sandbox.ExtractMiseBinary: write tmp: %w", err)
	}
	if err := os.Rename(tmp, binPath); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("sandbox.ExtractMiseBinary: rename: %w", err)
	}

	if runtime.GOOS == "darwin" {
		if err := macCodesign(ctx, binPath, log); err != nil {
			return "", fmt.Errorf("sandbox.ExtractMiseBinary: codesign: %w", err)
		}
	}

	if err := os.WriteFile(hashPath, []byte(wantHash), 0o644); err != nil {
		log.Warn("mise hash file write failed (will re-extract next boot)", zap.Error(err))
	}

	if err := writeMiseConfig(sandboxRoot, log); err != nil {
		log.Warn("mise config write failed (attestation may not be disabled)", zap.Error(err))
	}

	log.Info("mise extracted",
		zap.String("path", binPath),
		zap.Int("size_bytes", len(miseBinary)),
		zap.String("sha256", wantHash[:16]+"..."))
	return binPath, nil
}

const miseConfigName = "mise.toml"

// MiseGlobalConfigPath returns the path injected as MISE_GLOBAL_CONFIG_FILE.
//
// MiseGlobalConfigPath 返回作为 MISE_GLOBAL_CONFIG_FILE 注入的路径。
func MiseGlobalConfigPath(sandboxRoot string) string {
	return filepath.Join(sandboxRoot, miseConfigName)
}

// writeMiseConfig drops a mise.toml that disables every attestation backend (avoid GitHub rate limits).
//
// writeMiseConfig 写 mise.toml 关掉所有 attestation 后端（避开 GitHub 限流）。
func writeMiseConfig(sandboxRoot string, log *zap.Logger) error {
	const body = `# Forgify-managed. Do not edit; rewritten on every backend boot
# by sandbox.ExtractMiseBinary. See sandbox/mise.go::writeMiseConfig.
# Forgify 管理。每次后端启动 sandbox.ExtractMiseBinary 重写——别动。

[settings]
# Disable every attestation path mise supports. These calls hit GitHub's
# unauthenticated API → 60 req/hr → trivial 403 under iteration. Embedded
# mise binary is SHA256-pinned by us; upstream tarballs go HTTPS + mise's
# own checksum check. See writeMiseConfig comment for full rationale.
#
# 关掉 mise 支持的所有 attestation 路径。这些调用打 GitHub 未鉴权 API
# → 60/小时 → 迭代时容易 403。embed mise 由我们 SHA256 钉死，上游
# tarball 走 HTTPS + mise 自带 checksum。详见 writeMiseConfig 注释。

# Python (core plugin)
python.github_attestations = false

# Aqua-installed tools (uv, pnpm, maven, ...)
aqua.cosign = false
aqua.slsa = false
aqua.minisign = false
aqua.github_attestations = false
`
	configPath := MiseGlobalConfigPath(sandboxRoot)
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		return fmt.Errorf("sandbox.writeMiseConfig: write mise.toml: %w", err)
	}
	log.Debug("mise config written", zap.String("path", configPath))
	return nil
}

func miseExeName() string {
	if runtime.GOOS == "windows" {
		return "mise.exe"
	}
	return "mise"
}

const miseDataSubdir = "mise-data"

// MiseInstaller is a generic RuntimeInstaller for any mise plugin.
//
// MiseInstaller 是任何 mise 插件的通用 RuntimeInstaller。
type MiseInstaller struct {
	miseBin        string
	kind           string
	defaultVersion string
}

// NewMiseInstaller constructs a MiseInstaller (defaultVersion may be a partial spec).
//
// NewMiseInstaller 构造 MiseInstaller（defaultVersion 可为部分约束）。
func NewMiseInstaller(miseBin, kind, defaultVersion string) *MiseInstaller {
	return &MiseInstaller{miseBin: miseBin, kind: kind, defaultVersion: defaultVersion}
}

func (m *MiseInstaller) Kind() string { return m.kind }

// normalizeVersionForMise strips PEP 440 / semver range prefixes for mise compatibility.
//
// normalizeVersionForMise 剥 PEP 440 / semver 范围前缀以适配 mise。
func normalizeVersionForMise(version string) string {
	v := version
	for _, prefix := range []string{">=", "<=", "~=", "==", ">", "<", "~", "^"} {
		if len(v) > len(prefix) && v[:len(prefix)] == prefix {
			v = v[len(prefix):]
			break
		}
	}
	for len(v) > 0 && (v[0] == ' ' || v[0] == '\t') {
		v = v[1:]
	}
	return v
}

// Install runs `mise install kind@version` and returns the install dir relative to sandboxRoot.
//
// Install 跑 `mise install kind@version` 并返回相对 sandboxRoot 的 install 路径。
func (m *MiseInstaller) Install(ctx context.Context, version, sandboxRoot string, stream sandboxdomain.ProgressFunc) (string, error) {
	dataDir := filepath.Join(sandboxRoot, miseDataSubdir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", fmt.Errorf("sandbox.MiseInstaller.Install: mkdir mise data: %w", err)
	}

	miseVersion := normalizeVersionForMise(version)
	cmd := exec.CommandContext(ctx, m.miseBin, "install", "-y", m.kind+"@"+miseVersion)
	cmd.Env = append(os.Environ(),
		"MISE_DATA_DIR="+dataDir,
		"MISE_YES=1",
		"MISE_QUIET=1",
		"MISE_GLOBAL_CONFIG_FILE="+MiseGlobalConfigPath(sandboxRoot),
	)

	if err := RunWithStderrCapture(cmd, stream,
		sandboxdomain.ErrRuntimeInstallFailed,
		fmt.Sprintf("sandbox.MiseInstaller.Install %s@%s", m.kind, version),
	); err != nil {
		return "", err
	}

	actual, err := m.where(ctx, dataDir, miseVersion)
	if err != nil {
		return "", fmt.Errorf("sandbox.MiseInstaller.Install: locate after install: %w", err)
	}
	rel, err := filepath.Rel(sandboxRoot, actual)
	if err != nil {
		return "", fmt.Errorf("sandbox.MiseInstaller.Install: rel path %q from %q: %w", actual, sandboxRoot, err)
	}
	return rel, nil
}

// Locate returns the absolute path to the runtime's primary binary (tries fixed paths, then walks).
//
// Locate 返主 binary 绝对路径（先试固定路径，再 walk 兜底）。
func (m *MiseInstaller) Locate(version, sandboxRoot string) (string, error) {
	dataDir := filepath.Join(sandboxRoot, miseDataSubdir)
	installDir, err := m.where(context.Background(), dataDir, normalizeVersionForMise(version))
	if err != nil {
		return "", fmt.Errorf("sandbox.MiseInstaller.Locate: %w", err)
	}
	binName := m.kind
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	for _, candidate := range []string{
		filepath.Join(installDir, "bin", binName),
		filepath.Join(installDir, binName),
	} {
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() && isExecutable(st.Mode()) {
			return candidate, nil
		}
	}
	if found := walkForBinary(installDir, m.kind); found != "" {
		return found, nil
	}
	entries, _ := os.ReadDir(installDir)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return "", fmt.Errorf("sandbox.MiseInstaller.Locate %s@%s: binary not found in %s (dir contains: %s)",
		m.kind, version, installDir, strings.Join(names, ", "))
}

func walkForBinary(installDir, kind string) string {
	exact := kind
	exactExe := kind + ".exe"
	var hit string
	_ = filepath.WalkDir(installDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if name != exact && name != exactExe && !strings.HasPrefix(name, kind) {
			return nil
		}
		st, err := d.Info()
		if err != nil || !isExecutable(st.Mode()) {
			return nil
		}
		hit = path
		if name == exact || name == exactExe {
			return filepath.SkipAll
		}
		return nil
	})
	return hit
}

func isExecutable(mode os.FileMode) bool {
	if runtime.GOOS == "windows" {
		return mode.IsRegular()
	}
	return mode&0o111 != 0
}

func (m *MiseInstaller) where(ctx context.Context, dataDir, version string) (string, error) {
	cmd := exec.CommandContext(ctx, m.miseBin, "where", m.kind+"@"+version)
	sandboxRoot := filepath.Dir(dataDir)
	cmd.Env = append(os.Environ(),
		"MISE_DATA_DIR="+dataDir,
		"MISE_GLOBAL_CONFIG_FILE="+MiseGlobalConfigPath(sandboxRoot),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("sandbox.MiseInstaller.where %s@%s: %w: %s",
			m.kind, version, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// ResolveDefault returns the default version baked at construction time.
//
// ResolveDefault 返回构造时固化的默认版本。
func (m *MiseInstaller) ResolveDefault(ctx context.Context) (string, error) {
	return m.defaultVersion, nil
}

// NormalizeVersion strips range prefixes so equivalent specs share one runtime row.
//
// NormalizeVersion 剥范围前缀，让等价 spec 共用一个 runtime 行。
func (m *MiseInstaller) NormalizeVersion(version string) string {
	return normalizeVersionForMise(version)
}
