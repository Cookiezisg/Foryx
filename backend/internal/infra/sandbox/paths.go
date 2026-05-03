// paths.go: pure path computation, EnvID hashing, and the per-forge mutex
// map. All exec / IO logic lives in sandbox.go / sync.go / preflight.go;
// keeping these helpers side-effect-free keeps them trivially unit-testable
// without spinning up uv or Python subprocesses.
//
// paths.go：纯路径计算、EnvID 哈希、per-forge 互斥 map。
// 所有 exec / IO 逻辑在 sandbox.go / sync.go / preflight.go 里；
// 保持这些 helper 无副作用让单测不必拉起 uv / Python 子进程。

package sandbox

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// ── Path helpers ──────────────────────────────────────────────────────────────

// envDir returns the venv directory for forge + EnvID:
//
//	<dataDir>/forges/<forgeID>/envs/<envID>/
//
// envDir 返回某 forge + EnvID 对应的 venv 目录。
func envDir(dataDir, forgeID, envID string) string {
	return filepath.Join(dataDir, "forges", forgeID, "envs", envID)
}

// versionDir returns the per-version code dir for forge + version:
//
//	<dataDir>/forges/<forgeID>/versions/<versionID>/
//
// versionDir 返回某 forge + version 对应的代码目录。
func versionDir(dataDir, forgeID, versionID string) string {
	return filepath.Join(dataDir, "forges", forgeID, "versions", versionID)
}

// forgeDir returns the top-level dir for a forge:
//
//	<dataDir>/forges/<forgeID>/
//
// forgeDir 返回某 forge 的顶层目录。
func forgeDir(dataDir, forgeID string) string {
	return filepath.Join(dataDir, "forges", forgeID)
}

// uvCacheDir returns the path used for UV_CACHE_DIR — isolated from the
// user's system uv cache (typically ~/.cache/uv on unix).
//
// uvCacheDir 返回 UV_CACHE_DIR 路径——隔离用户系统 uv cache（unix 通常
// ~/.cache/uv）。
func uvCacheDir(dataDir string) string {
	return filepath.Join(dataDir, "uv-cache")
}

// bundledPythonPath returns the absolute path to the bundled Python
// interpreter extracted by Bootstrap. python-build-standalone follows upstream
// Python packaging conventions, so the layout differs across platforms:
//
//   - mac/linux: <dataDir>/bin/python/bin/python3
//   - windows:   <dataDir>/bin/python/python.exe (no bin/ subdir)
//
// bundledPythonPath 返回 Bootstrap 解压的捆绑 Python 解释器绝对路径。
// python-build-standalone 跟随上游 Python 打包惯例，跨平台不同：
//
//   - mac/linux: <dataDir>/bin/python/bin/python3
//   - windows:   <dataDir>/bin/python/python.exe（直接在根目录，无 bin/）
func bundledPythonPath(dataDir string) string {
	base := filepath.Join(dataDir, "bin", "python")
	if runtime.GOOS == "windows" {
		return filepath.Join(base, "python.exe")
	}
	return filepath.Join(base, "bin", "python3")
}

// bundledUVPath returns the absolute path to the bundled uv binary
// (uv on unix, uv.exe on windows).
//
// bundledUVPath 返回捆绑 uv 二进制的绝对路径（unix uv / win uv.exe）。
func bundledUVPath(dataDir string) string {
	name := "uv"
	if runtime.GOOS == "windows" {
		name = "uv.exe"
	}
	return filepath.Join(dataDir, "bin", name)
}

// ── EnvID computation ─────────────────────────────────────────────────────────

// ComputeEnvID returns a stable identifier of the form "env_<12hex>" derived
// from a dependency set + Python version specifier. Multiple ForgeVersion
// rows that share the same deps + python end up with the same EnvID and
// therefore share one physical venv directory.
//
// Normalization (so order / casing differences don't fragment EnvIDs):
//   - trim leading/trailing whitespace from each specifier
//   - lowercase the leading package-name portion
//   - drop blank entries entirely
//   - sort the resulting list lexicographically
//   - trim whitespace on the python version too
//
// Specifier version constraint operators (>=, ==, ~=, etc.) and version digits
// are kept verbatim — "pandas>=2.0" and "pandas>=2.0.0" deliberately produce
// different EnvIDs. PEP 440 says they are equivalent, but matching that
// equivalence requires full version semantic parsing — out of scope. An
// extra venv with a few MB of metadata is cheap insurance.
//
// ComputeEnvID 返回形如 "env_<12hex>" 的稳定标识，由依赖集 + Python 版本
// 约束派生。同 deps + python 的多个 ForgeVersion 得到同 EnvID，共享同一个
// venv 目录。
//
// 标准化（避免顺序/大小写差异碎片化 EnvID）：
//   - 每个 specifier 去首尾空白
//   - 包名（前导标识符部分）小写
//   - 空字符串项整个丢弃
//   - 标准化后的列表字典序排序
//   - python 版本约束也去首尾空白
//
// 版本约束运算符（>=、==、~= 等）和版本号原样保留——`pandas>=2.0` 和
// `pandas>=2.0.0` 故意得到不同 EnvID（PEP 440 等价但语义合并需完整版本
// 解析，过度工程；多一个 venv 几 MB metadata 不算事）。
func ComputeEnvID(deps []string, pythonVersion string) string {
	normalized := make([]string, 0, len(deps))
	for _, d := range deps {
		if n := normalizeSpecifier(d); n != "" {
			normalized = append(normalized, n)
		}
	}
	sort.Strings(normalized)
	payload := strings.Join(normalized, "\n") + "\n" + strings.TrimSpace(pythonVersion)
	h := sha256.Sum256([]byte(payload))
	return "env_" + hex.EncodeToString(h[:6])
}

// normalizeSpecifier trims whitespace and lowercases the leading package-name
// portion (everything up to the first comparison operator or other separator).
// Returns "" for blank input.
//
// normalizeSpecifier 去首尾空白并把前导包名（首个比较符或分隔符前的部分）
// 小写。空白输入返 ""。
func normalizeSpecifier(spec string) string {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return ""
	}
	// Walk forward over the package-name characters: ASCII letter / digit /
	// underscore / hyphen / dot. Stop at the first version operator or other
	// punctuation (>, =, <, ~, !, [, space, etc.).
	i := 0
	for i < len(spec) {
		c := spec[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '-' || c == '.' {
			i++
			continue
		}
		break
	}
	return strings.ToLower(spec[:i]) + spec[i:]
}

// ── Per-forge mutex map ───────────────────────────────────────────────────────

// forgeMutexMap is a small concurrent map of per-forge mutexes. Sync calls
// for the same forge serialize through it; calls for different forges run
// in parallel. Run is NOT gated by this map — multiple Run invocations of
// the same forge are allowed (different files / read-only venv access).
//
// The map only grows — entries are never removed even after a forge is
// destroyed. Memory cost is trivial (a few thousand forges = a few KB of
// mutex headers); attempting to delete entries safely under contention adds
// race risk for negligible savings.
//
// forgeMutexMap 是 per-forge 互斥的小并发 map。同 forge 的 Sync 串行化；
// 不同 forge 的 Sync 并行。Run 不走此 map——同 forge 多个 Run 允许并发
// （动不同文件 / 只读用 venv）。
//
// map 仅增长——forge 销毁后条目不移除。内存代价微小（几千 forge =
// 几 KB mutex header）；并发删条目易引入 race，得不偿失。
type forgeMutexMap struct {
	mu sync.Mutex
	m  map[string]*sync.Mutex
}

func newForgeMutexMap() *forgeMutexMap {
	return &forgeMutexMap{m: make(map[string]*sync.Mutex)}
}

// Lock acquires the mutex for the given forge ID and returns an unlock func.
// Caller pattern:
//
//	unlock := m.Lock(forgeID)
//	defer unlock()
//
// Lock 获取指定 forge ID 的互斥锁并返回 unlock 函数。
func (m *forgeMutexMap) Lock(forgeID string) func() {
	m.mu.Lock()
	fm, ok := m.m[forgeID]
	if !ok {
		fm = &sync.Mutex{}
		m.m[forgeID] = fm
	}
	m.mu.Unlock()
	fm.Lock()
	return fm.Unlock
}
