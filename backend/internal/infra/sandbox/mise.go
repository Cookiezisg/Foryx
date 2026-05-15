// mise.go — everything related to the mise universal version manager:
// extracting the embedded mise binary at boot (ExtractMiseBinary) and
// the generic RuntimeInstaller that wraps `mise install` / `mise where`
// (MiseInstaller).
//
// Layout the mise installer establishes per (kind, version):
//
//	<sandboxRoot>/mise-data/installs/<kind>/<resolved-version>/bin/<kind>
//
// All MiseInstaller instances share one MISE_DATA_DIR rooted at
// <sandboxRoot>/mise-data/, so mise's plugin manifest, version cache, and
// `mise where` lookups stay consistent across all installed runtimes.
//
// "resolved-version" matters when callers pass a partial spec like
// "3.12" — mise expands it to whichever patch is current at install
// time, and Install asks `mise where` for the actual path before
// deriving the relPath returned to the service.
//
// mise.go ——所有跟 mise 通用版本管理器相关的代码：启动时抽取 embed mise
// 二进制（ExtractMiseBinary）+ 包装 `mise install` / `mise where` 的通用
// RuntimeInstaller（MiseInstaller）。
//
// mise installer 按 (kind, version) 建立的布局：
//
//	<sandboxRoot>/mise-data/installs/<kind>/<resolved-version>/bin/<kind>
//
// 所有 MiseInstaller 实例共享单个 MISE_DATA_DIR（位于 <sandboxRoot>/mise-data/），
// 让 mise 的 plugin manifest / 版本缓存 / `mise where` 查询在所有装的 runtime
// 间保持一致。
//
// "resolved-version" 在调用方传部分约束（如 "3.12"）时有意义——mise 装机时
// 展开到当时该 minor 的最新 patch；Install 在 mise 装完后调 `mise where` 拿
// 真实路径再算 relPath 返给 service。

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

// ── Embed extraction ─────────────────────────────────────────────────────────

// ExtractMiseBinary writes the embedded mise binary to <sandboxRoot>/bin/mise
// (mise.exe on Windows), makes it executable, and on darwin runs ad-hoc
// codesign. Idempotent — subsequent calls with an unchanged embed return
// the existing path without re-writing. Returns the absolute path to the
// extracted mise binary on success.
//
// ExtractMiseBinary 把 embed mise 二进制写到 <sandboxRoot>/bin/mise
// （Windows 是 mise.exe），标记可执行，darwin 上 ad-hoc codesign。幂等——
// embed 不变的后续调用直接返已有路径不重写。成功返 mise 二进制绝对路径。
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

	// Idempotency: skip re-extract if both hash file matches AND binary on
	// disk exists (handles "user wiped sandbox/bin but kept .mise.hash").
	//
	// 幂等：仅当 hash 文件匹配 *且* 二进制存在时才跳过（处理"用户清了
	// sandbox/bin 但留了 .mise.hash"的情况）。
	if existing, err := os.ReadFile(hashPath); err == nil && string(existing) == wantHash {
		if _, statErr := os.Stat(binPath); statErr == nil {
			log.Debug("mise already extracted (hash match)", zap.String("path", binPath))
			return binPath, nil
		}
	}

	// Atomic write: tmp + rename so partial writes never leave a half-built
	// binary that subsequent runs would refuse to overwrite.
	//
	// 原子写：tmp + rename，半写永远不会留下后续运行拒绝覆盖的半成品。
	tmp := binPath + ".tmp"
	if err := os.WriteFile(tmp, miseBinary, 0o755); err != nil {
		return "", fmt.Errorf("sandbox.ExtractMiseBinary: write tmp: %w", err)
	}
	if err := os.Rename(tmp, binPath); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("sandbox.ExtractMiseBinary: rename: %w", err)
	}

	// darwin: ad-hoc codesign so Gatekeeper does not SIGKILL the binary on
	// first exec. macCodesign (codesign.go) operates on the single binary
	// path — V3 collapse removed the multi-file installers that needed
	// recursion (Python tarball / Playwright).
	//
	// darwin: ad-hoc codesign 让 Gatekeeper 首次 exec 时不 SIGKILL。
	// macCodesign（codesign.go）操作单二进制路径——V3 collapse 删除了需要
	// 递归的多文件 installer（Python tarball / Playwright）。
	if runtime.GOOS == "darwin" {
		if err := macCodesign(ctx, binPath, log); err != nil {
			return "", fmt.Errorf("sandbox.ExtractMiseBinary: codesign: %w", err)
		}
	}

	// Hash file write is best-effort — losing it on a crash just means the
	// next boot re-extracts (cheap, idempotent at the filesystem level).
	//
	// hash 文件写是 best-effort——崩溃丢失只是下次启动重抽（便宜，文件层
	// 仍幂等）。
	if err := os.WriteFile(hashPath, []byte(wantHash), 0o644); err != nil {
		log.Warn("mise hash file write failed (will re-extract next boot)", zap.Error(err))
	}

	// Write the mise global config that disables all attestation paths.
	// MiseInstaller.Install points MISE_GLOBAL_CONFIG_FILE at this file
	// so every install call inherits the same toggles.
	//
	// 写 mise 全局 config 把所有 attestation 路径关掉。MiseInstaller.Install
	// 把 MISE_GLOBAL_CONFIG_FILE 指过来，每次 install 都继承同一份开关。
	if err := writeMiseConfig(sandboxRoot, log); err != nil {
		log.Warn("mise config write failed (attestation may not be disabled)", zap.Error(err))
	}

	log.Info("mise extracted",
		zap.String("path", binPath),
		zap.Int("size_bytes", len(miseBinary)),
		zap.String("sha256", wantHash[:16]+"..."))
	return binPath, nil
}

// miseConfigName is the global mise config we drop next to the binary.
// MiseGlobalConfigPath returns its absolute path under <sandboxRoot>.
//
// miseConfigName 是落在 binary 旁边的 mise 全局 config 文件名。
// MiseGlobalConfigPath 返其在 <sandboxRoot> 下的绝对路径。
const miseConfigName = "mise.toml"

// MiseGlobalConfigPath returns the path MiseInstaller passes via
// MISE_GLOBAL_CONFIG_FILE so every `mise install` reads the same settings
// (attestation disabled — see writeMiseConfig).
//
// MiseGlobalConfigPath 返 MiseInstaller 通过 MISE_GLOBAL_CONFIG_FILE 传入的
// 路径，让每次 `mise install` 读同一份 settings（关 attestation——见
// writeMiseConfig）。
func MiseGlobalConfigPath(sandboxRoot string) string {
	return filepath.Join(sandboxRoot, miseConfigName)
}

// writeMiseConfig writes a mise.toml that disables attestation verification
// across every backend mise supports (core/python plugin, aqua-installed
// tools like uv/maven/pnpm, ubi-installed binaries). Rationale: mise's
// attestation calls hit GitHub's *unauthenticated* API which trips the
// 60 req/hr rate limit during pipeline test loops or active dev iteration,
// failing hard with "verification failed" rather than the actual cause.
// We're a local single-user app — the embedded mise binary is SHA256-pinned
// by us, and upstream tarballs are HTTPS + checksum-verified by mise itself.
// Attestation adds no real defense for this threat model. Idempotent — same
// content overwritten on each boot.
//
// writeMiseConfig 落一份 mise.toml，把 mise 支持的每个 backend（core/python
// plugin、aqua 装的工具如 uv/maven/pnpm、ubi 装的二进制）的 attestation 校验
// 全关。理由：mise 的 attestation 调用打 GitHub *未鉴权* API，pipeline 测试
// 循环或开发迭代密集时容易撞 60/小时上限，挂时表现"verification failed"而
// 不是真因。我们是本地单用户 app——embed mise 由我们 SHA256 钉死，上游
// tarball 走 HTTPS + mise 自带 checksum 校验。这层 attestation 对本场景威胁
// 模型无实际防御价值。幂等——每次启动同内容覆盖。
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

// miseExeName returns "mise.exe" on Windows, "mise" elsewhere.
//
// miseExeName Windows 上返 "mise.exe"，其他平台返 "mise"。
func miseExeName() string {
	if runtime.GOOS == "windows" {
		return "mise.exe"
	}
	return "mise"
}

// ── Generic RuntimeInstaller ─────────────────────────────────────────────────

// miseDataSubdir is the relative directory under sandboxRoot where mise
// keeps its data (installs, plugins, cache). Shared by all MiseInstaller
// instances so a single mise process state covers the whole sandbox.
//
// miseDataSubdir 是 mise 数据（installs / plugins / cache）相对 sandboxRoot
// 的目录。所有 MiseInstaller 实例共享，让单个 mise 进程状态覆盖整个 sandbox。
const miseDataSubdir = "mise-data"

// MiseInstaller is a generic RuntimeInstaller for any mise-supported tool
// (python / node / rust / java / go / ruby / php / 600+ via mise plugins).
//
// MiseInstaller 是任何 mise 支持工具的通用 RuntimeInstaller
// （python / node / rust / java / go / ruby / php / 通过 mise plugin 600+）。
type MiseInstaller struct {
	miseBin        string // absolute path to extracted mise binary (from ExtractMiseBinary)
	kind           string // mise plugin name + Runtime.Kind
	defaultVersion string // version returned by ResolveDefault when EnvSpec.Runtime.Version is empty
}

// NewMiseInstaller constructs a MiseInstaller. miseBin must be an absolute
// path to an executable mise binary (typically the value returned by
// ExtractMiseBinary). defaultVersion may be a partial spec (e.g. "3.12")
// that mise expands at install time.
//
// NewMiseInstaller 构造 MiseInstaller。miseBin 必须是已可执行 mise 二进制
// 的绝对路径（通常是 ExtractMiseBinary 的返回值）。defaultVersion 可以是
// 部分约束（如 "3.12"），mise 装机时自动展开。
func NewMiseInstaller(miseBin, kind, defaultVersion string) *MiseInstaller {
	return &MiseInstaller{miseBin: miseBin, kind: kind, defaultVersion: defaultVersion}
}

// Kind reports the mise plugin name this installer wraps.
//
// Kind 报告本 installer 包装的 mise plugin 名。
func (m *MiseInstaller) Kind() string { return m.kind }

// normalizeVersionForMise strips PEP 440 / semver range prefixes (`>=`, `~=`,
// `>`, `<=`, `<`, `==`) so the remaining concrete version is something mise
// accepts. mise's `install python@>=3.12` falls back to python-build (pyenv
// source compile, broken on this developer's mac); `install python@3.12`
// uses precompiled cpython artifacts and just works. Domain layer keeps
// LLM-emitted PEP 440 specs verbatim (audit / display), but the boundary
// to mise gets a concrete version.
//
// normalizeVersionForMise 剥掉 PEP 440 / semver 范围前缀(`>=`、`~=`、`>` 等)
// 留下具体版本号给 mise。mise `install python@>=3.12` 会退到 python-build 走
// 源码编译(在本机 mac 上挂了);`install python@3.12` 走预编译 cpython,稳。
// domain 层保留 LLM 写的 PEP 440 原文(审计/显示),只在 mise 边界做转换。
func normalizeVersionForMise(version string) string {
	v := version
	// Order matters: try longer prefixes first.
	// 优先匹配更长前缀。
	for _, prefix := range []string{">=", "<=", "~=", "==", ">", "<", "~", "^"} {
		if len(v) > len(prefix) && v[:len(prefix)] == prefix {
			v = v[len(prefix):]
			break
		}
	}
	// Trim surrounding whitespace LLM occasionally leaves in (`>= 3.12`).
	// 去 LLM 偶尔留的空白(`>= 3.12`)。
	for len(v) > 0 && (v[0] == ' ' || v[0] == '\t') {
		v = v[1:]
	}
	return v
}

// Install runs `mise install <kind>@<version>` against the shared
// MISE_DATA_DIR (<sandboxRoot>/mise-data/). After mise reports success,
// `mise where` resolves the actual install directory (handles partial
// version spec → concrete patch resolution); the returned relPath is the
// install dir relative to sandboxRoot — service layer stores it in
// Runtime.Path.
//
// Install 在共享 MISE_DATA_DIR（<sandboxRoot>/mise-data/）跑
// `mise install <kind>@<version>`。mise 报告成功后，`mise where` 解析实际
// install 目录（处理部分版本约束 → 具体 patch 的解析）；返回的 relPath
// 是 install dir 相对 sandboxRoot 的路径——service 层存入 Runtime.Path。
func (m *MiseInstaller) Install(ctx context.Context, version, sandboxRoot string, stream sandboxdomain.ProgressFunc) (string, error) {
	dataDir := filepath.Join(sandboxRoot, miseDataSubdir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", fmt.Errorf("sandbox.MiseInstaller.Install: mkdir mise data: %w", err)
	}

	miseVersion := normalizeVersionForMise(version)
	cmd := exec.CommandContext(ctx, m.miseBin, "install", "-y", m.kind+"@"+miseVersion)
	cmd.Env = append(os.Environ(),
		"MISE_DATA_DIR="+dataDir,
		"MISE_YES=1",   // skip interactive prompts
		"MISE_QUIET=1", // less chatty stdout (we parse stderr for progress)
		// Point at the global config written by ExtractMiseBinary so every
		// install reads the same attestation-disabled settings. See
		// writeMiseConfig for the rationale.
		//
		// 指向 ExtractMiseBinary 写的全局 config，让每次 install 读同一份
		// 关 attestation 的 settings。详见 writeMiseConfig。
		"MISE_GLOBAL_CONFIG_FILE="+MiseGlobalConfigPath(sandboxRoot),
	)

	if err := RunWithStderrCapture(cmd, stream,
		sandboxdomain.ErrRuntimeInstallFailed,
		fmt.Sprintf("sandbox.MiseInstaller.Install %s@%s", m.kind, version),
	); err != nil {
		return "", err
	}

	// Resolve actual install dir — mise may have expanded a partial spec
	// (e.g. "3.12" → "3.12.5"). Use absolute path then derive relPath.
	// Use the normalized version (matches what we actually installed above).
	//
	// 解析实际 install 目录——mise 可能展开了部分约束（如 "3.12" → "3.12.5"）。
	// 用绝对路径再算 relPath。用归一化后的版本(匹配上面的实际安装)。
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

// Locate returns the absolute path to the runtime's primary binary. mise
// install layouts vary per backend:
//   - core plugins (python/node/...) ship binaries under <installDir>/bin/<kind>
//   - aqua plugins (uv/pnpm/maven/...) extract tarballs whose contents
//     vary per upstream — uv flattens to a single binary named after the
//     tarball arch suffix (`uv-aarch64-apple-darwin`), pnpm puts it at
//     <installDir>/pnpm directly, etc.
//
// Strategy: try the fixed layouts first (cheap), then walk the install dir
// looking for an executable file matching the kind name (with arch suffix
// or not). `mise which` would be cleaner but only resolves the *active*
// version set via `mise use`, not version-pinned installs.
//
// Locate 返主 binary 绝对路径。mise install 布局按 backend 各异——core
// plugin（python/node）放 <installDir>/bin/<kind>；aqua plugin（uv/pnpm/
// maven）解 tarball，内容跟上游走（uv 是 flat binary 名带 arch 后缀
// `uv-aarch64-apple-darwin`，pnpm 平铺成 <installDir>/pnpm 等）。
// 策略：先试固定布局（快），再走 install 目录找匹配 kind 名的可执行文件
// （含 arch 后缀变体）。`mise which` 更简洁但只解 `mise use` 设置的 active
// 版本，版本钉死 install 不在其中。
func (m *MiseInstaller) Locate(version, sandboxRoot string) (string, error) {
	dataDir := filepath.Join(sandboxRoot, miseDataSubdir)
	// Normalize PEP 440 / semver prefixes before talking to mise; the install
	// happened under the normalized version so `mise where` must match.
	// 归一化版本号(与 Install 同步),否则 `mise where` 找不到。
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
	// Recursive search: aqua tarballs may extract to a per-arch subdir.
	// Match either exact binName, kind-arch-os pattern, or any file whose
	// name starts with kind and is executable.
	//
	// 递归查找：aqua tarball 可能解到 per-arch 子目录。匹配精确 binName、
	// kind-arch-os 模式，或任何以 kind 开头的可执行文件。
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

// walkForBinary descends installDir looking for an executable file whose
// name is `kind`, `kind.exe`, or starts with `kind` (catches aqua's
// `<kind>-<arch>-<os>` flat-binary pattern). Returns "" if none.
//
// walkForBinary 下行 installDir 找名为 kind / kind.exe / 或以 kind 起头的可
// 执行文件（覆盖 aqua 的 `<kind>-<arch>-<os>` flat binary 模式）。无则 ""。
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
		// Prefer exact match: stop walking once we have it.
		// 优先精确匹配：拿到就停止 walk。
		if name == exact || name == exactExe {
			return filepath.SkipAll
		}
		return nil
	})
	return hit
}

// isExecutable reports whether a file mode has any execute bit set.
// Windows: any regular file is considered executable here (file extension
// is the actual gate via .exe/.bat/etc. in PATHEXT — handled by the caller
// passing "<kind>.exe").
//
// isExecutable 判断 mode 是否有执行位。Windows：普通文件即视为可执行
// （真正按扩展名判断，调用方传 "<kind>.exe"）。
func isExecutable(mode os.FileMode) bool {
	if runtime.GOOS == "windows" {
		return mode.IsRegular()
	}
	return mode&0o111 != 0
}

// where invokes `mise where <kind>@<version>` against the given dataDir
// and returns the install path. Returns an error if the tool isn't
// installed at that version (caller chains it as install-failure context).
//
// where 在指定 dataDir 调 `mise where <kind>@<version>` 返 install 路径。
// 工具未装该版本返错（调用方串成 install-failure 上下文）。
func (m *MiseInstaller) where(ctx context.Context, dataDir, version string) (string, error) {
	cmd := exec.CommandContext(ctx, m.miseBin, "where", m.kind+"@"+version)
	// dataDir is <sandboxRoot>/<miseDataSubdir>; the global config sits at
	// <sandboxRoot>/mise.toml — derive sandboxRoot by going up one level.
	//
	// dataDir 是 <sandboxRoot>/<miseDataSubdir>；全局 config 在
	// <sandboxRoot>/mise.toml——上一层就是 sandboxRoot。
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
// May be a partial spec (e.g. "3.12"); mise resolves to the latest patch
// at install time.
//
// ResolveDefault 返构造时固化的默认版本。可以是部分约束（如 "3.12"）；
// mise 装机时解析到该 minor 最新 patch。
func (m *MiseInstaller) ResolveDefault(ctx context.Context) (string, error) {
	return m.defaultVersion, nil
}

// NormalizeVersion strips PEP 440 / semver range prefixes so two requests
// for the same install (e.g. `>=3.12` and `3.12`) share one runtime row
// (#17 fix). Mirrors the in-flight normalization Install / Locate already
// apply at the mise CLI boundary; doing the same at the registry-row layer
// makes sandbox_runtimes.version reflect the concrete installed version.
//
// NormalizeVersion 剥范围前缀,让 `>=3.12` / `3.12` 同 install 共用 runtime
// 行(#17 修)。与 Install/Locate 在 mise CLI 边界做的归一化同源,这里把
// 归一化拉到注册行层面,让 sandbox_runtimes.version 反映实装版本。
func (m *MiseInstaller) NormalizeVersion(version string) string {
	return normalizeVersionForMise(version)
}
