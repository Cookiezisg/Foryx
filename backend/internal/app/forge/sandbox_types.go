// sandbox_types.go — request value types for the forge.Sandbox port.
//
// These types used to live in infra/sandbox (the v1 forge-only sandbox
// implementation). Moving them into the forge app package removes the
// last forge → infra/sandbox direct type dependency, so D2-5b can delete
// the v1 sandbox files without forge needing to know.
//
// The shape is identical to v1's SyncRequest / RunRequest — adapter
// implementations (current: SandboxAdapter wrapping sandboxapp.Service)
// translate these into the v2 service's Owner / EnvSpec / SpawnOpts.
//
// sandbox_types.go ——forge.Sandbox 端口的请求值类型。
//
// 这些类型曾在 infra/sandbox（v1 forge-only sandbox 实现）。挪到 forge app
// 包消除 forge → infra/sandbox 直接类型依赖，D2-5b 删 v1 sandbox 文件时
// forge 不需知。
//
// 形状跟 v1 SyncRequest / RunRequest 一致——adapter 实现（当前：
// SandboxAdapter 包 sandboxapp.Service）翻译为 v2 service 的 Owner /
// EnvSpec / SpawnOpts。

package forge

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// SyncRequest is one materialize-this-EnvID order. The Sandbox
// implementation creates a venv keyed by EnvID under the forge's own
// dir, installs Dependencies via uv pip, and reports per-stage progress
// via OnProgress.
//
// SyncRequest 是一份"物化这个 EnvID"的指令。Sandbox 实现按 EnvID 在 forge
// 自己的 dir 下建 venv，通过 uv pip 装 Dependencies，per-stage 进度通过
// OnProgress 报。
type SyncRequest struct {
	ForgeID       string
	VersionID     string // for logging only — venv keyed by EnvID, not version
	EnvID         string
	Dependencies  []string
	PythonVersion string
	OnProgress    func(stage, detail string)
}

// RunRequest is one execute-this-forge order.
//
// RunRequest 是一份"执行这个 forge"的指令。
type RunRequest struct {
	ForgeID       string
	VersionID     string
	EnvID         string
	Code          string
	EntryFunction string // optional; sandbox falls back to first `def` if empty
	Input         map[string]any
}

// SyncError wraps a venv-build failure (e.g. uv pip stderr) so the forge
// service can errors.As + extract the captured stderr text into the
// ForgeVersion.EnvError column. Adapter implementations populate this
// when the underlying tool reports a failure.
//
// SyncError 包装 venv 构建失败（如 uv pip stderr），让 forge service 能
// errors.As + 把捕获的 stderr 文本提取到 ForgeVersion.EnvError 列。
// adapter 实现在底层工具报错时填这个。
type SyncError struct {
	Cause  error
	Stderr string
}

func (e *SyncError) Error() string { return e.Stderr }
func (e *SyncError) Unwrap() error { return e.Cause }

// ComputeEnvID returns a stable hash-derived id for the (deps, pythonVersion)
// pair: identical inputs produce identical EnvIDs across processes / boots.
//
// Normalization rules: dep names are lowercased up to the first version
// operator (so "Pandas" / "pandas" hash the same); blank entries dropped;
// list sorted lexically; pythonVersion stripped of surrounding whitespace.
// Version constraint operators (>=, ==, ~=) and version numbers are
// preserved verbatim — `pandas>=2.0` and `pandas>=2.0.0` deliberately
// produce different EnvIDs (PEP 440 equivalence requires a full version
// parser, overkill for "deduplicate envs"; one extra venv costs a few MB
// of metadata).
//
// ComputeEnvID 返 (deps, pythonVersion) 对的稳定 hash 派生 id：相同输入
// 跨进程/boot 产生相同 EnvID。
//
// 规范化规则：dep 名小写直到首个版本运算符（"Pandas" / "pandas" hash
// 相同）；空条目去掉；列表字典序排序；pythonVersion 去首尾空白。
// 版本运算符（>=、==、~=）和版本号原样保留——`pandas>=2.0` 与
// `pandas>=2.0.0` 故意不同 EnvID（PEP 440 等价需完整版本解析，过度
// 工程；多一个 venv 几 MB metadata 不算事）。
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
