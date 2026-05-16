// Package filesystem provides local-filesystem system tools (Read / Write / Edit) for the LLM.
//
// Package filesystem 提供本机文件操作的 LLM system tool（Read / Write / Edit）。
package filesystem

import (
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
)

// FilesystemTools constructs file-operation tools wired with their PathGuard.
//
// FilesystemTools 用 PathGuard 装配文件操作 tool。
func FilesystemTools(pathGuard pathguardpkg.PathGuard) []toolapp.Tool {
	return []toolapp.Tool{
		&Read{pathGuard: pathGuard},
		&Write{pathGuard: pathGuard},
		&Edit{pathGuard: pathGuard},
	}
}
