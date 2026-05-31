# audit: backend/internal/infra/sandbox/embed_mise_*.go (6 files combined)

LOC: 60 total across 6 files
Read: full file each (8 / 15 / 8 / 8 / 13 / 8 LOC)

## Files covered

| File | LOC | Build tag |
|---|---|---|
| embed_mise_darwin_amd64.go | 8 | `darwin && amd64` |
| embed_mise_darwin_arm64.go | 15 | `darwin && arm64` |
| embed_mise_linux_amd64.go | 8 | `linux && amd64` |
| embed_mise_linux_arm64.go | 8 | `linux && arm64` |
| embed_mise_unsupported.go | 13 | `!((darwin && (arm64 \|\| amd64)) \|\| (linux && (amd64 \|\| arm64)) \|\| (windows && amd64))` |
| embed_mise_windows_amd64.go | 8 | `windows && amd64` |

## Trace

These files contain only a `//go:embed` directive (or empty `var` for the unsupported fallback) and a single package-level variable declaration. No functions, no error sites, no logic.

| site# | file | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | embed_mise_*.go (5 supported variants) | `//go:embed mise/<goos>-<goarch>/mise` + `var miseBinary []byte` | A.1 | OK | compile-time embed; if the embed file is missing the build itself fails (go:embed contract). No runtime error path. The combined behavior with mise.go ExtractMiseBinary lines 65-68 (`len(miseBinary) == 0`) handles the unsupported-platform case gracefully. | N-A | — | — | — |
| 2 | embed_mise_unsupported.go | `var miseBinary []byte` (no embed directive) | A.1 | OK | empty fallback for non-supported platforms. mise.go:65-68 detects len==0 and returns ErrRuntimeInstallFailed with a platform-specific message. Service.Bootstrap then activates Degraded Mode per file-header doc comment lines 5-12. Documented intent + sentinel chain preserved upstream. | N-A | — | — | — |

## Sub-check

A.1 §S3 错误吞没:
  - violations: not present (files are pure declarations; the unsupported-platform fallback is documented + handled by mise.go upstream)

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: none
  - 各自 ctx 来源: N/A
  - violations: N/A — pure declarations, no runtime logic

A.3 §S15 ID 生成:
  - ID generation calls: none
  - violations: N/A — no logic

A.4 §S16 错误 wrap 格式:
  - violations: not present (no error returns)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A — files define no sentinels
