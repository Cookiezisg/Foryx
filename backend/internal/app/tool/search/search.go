// Package search provides the file/content search system tools the LLM uses
// to discover code: Grep / Glob (Phase 5 search batch).
//
// Imported as `searchtool` per §S13 nested sub-package alias rule.
//
// Path safety: every tool routes paths through `pkg/pathguard` to deny
// known-sensitive locations (~/.ssh, ~/.aws, /etc/, Forgify state dir).
// See decision D5 in 02-tools-deep/03-shell.md.
//
// Backend strategy for Grep: shell out to ripgrep (`rg`) when available
// for speed; fall back to a stdlib bufio+regexp implementation when rg
// is missing. Glob uses `bmatcuk/doublestar` for `**` support and a
// JSON output enriched with type/size/mtime per Forgify's "Glob covers LS"
// design (decision D3).
//
// Package search 提供 LLM 用于发现代码的检索 system tool：Grep / Glob
// （Phase 5 search 批次）。
//
// 调用方按 §S13 嵌套子包别名规则导入为 `searchtool`。
//
// 路径安全：每个 tool 都经 `pkg/pathguard` 守卫敏感路径（~/.ssh、~/.aws、
// /etc/、Forgify 状态目录），见 02-tools-deep/03-shell.md 决策 D5。
//
// Grep 后端策略：检测到 ripgrep（`rg`）时 shell out 求速；缺时 fallback
// 到 stdlib bufio+regexp 实现。Glob 用 `bmatcuk/doublestar` 支持 `**`，
// 输出 JSON 含 type/size/mtime（Forgify "Glob 覆盖 LS" 设计，决策 D3）。
package search

import (
	"os/exec"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
)

// SearchTools constructs the search system tools wired with their
// dependencies. Returns []toolapp.Tool because the chat ReAct loop consumes
// the abstract Tool interface.
//
// SearchTools 构造装配好依赖的 search system tool。返回 []toolapp.Tool。
func SearchTools(pathGuard pathguardpkg.PathGuard, log *zap.Logger) []toolapp.Tool {
	return []toolapp.Tool{
		newGrep(pathGuard, log),
		newGlob(pathGuard),
	}
}

// newGrep constructs a Grep with rg auto-detected from PATH.
// Empty rgPath means the stdlib fallback will be used.
//
// newGrep 构造 Grep，PATH 上自动检测 rg；空 rgPath 意味着用 stdlib fallback。
func newGrep(pathGuard pathguardpkg.PathGuard, log *zap.Logger) *Grep {
	rgPath, _ := exec.LookPath("rg") // err = not in PATH; treat as fallback
	return &Grep{pathGuard: pathGuard, rgPath: rgPath, log: log}
}

// newGlob constructs a Glob.
//
// newGlob 构造 Glob。
func newGlob(pathGuard pathguardpkg.PathGuard) *Glob {
	return &Glob{pathGuard: pathGuard}
}
