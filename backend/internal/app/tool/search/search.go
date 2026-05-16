// Package search provides file/content search system tools (Grep + Glob).
//
// Package search 提供文件 / 内容检索 system tool（Grep + Glob）。
package search

import (
	"os/exec"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
)

// SearchTools constructs the search system tools wired with PathGuard.
//
// SearchTools 用 PathGuard 装配 search system tool。
func SearchTools(pathGuard pathguardpkg.PathGuard, log *zap.Logger) []toolapp.Tool {
	return []toolapp.Tool{
		newGrep(pathGuard, log),
		newGlob(pathGuard),
	}
}

func newGrep(pathGuard pathguardpkg.PathGuard, log *zap.Logger) *Grep {
	rgPath, _ := exec.LookPath("rg")
	return &Grep{pathGuard: pathGuard, rgPath: rgPath, log: log}
}

func newGlob(pathGuard pathguardpkg.PathGuard) *Glob {
	return &Glob{pathGuard: pathGuard}
}
