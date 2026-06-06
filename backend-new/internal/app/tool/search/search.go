// Package search provides the file-navigation system tools (LS / Glob / Grep)
// for the LLM. They mirror how a person finds files on a machine: open a folder
// and look (LS), find by name pattern (Glob), find by content (Grep) — then
// drill in with an absolute path. There is no project root and no current
// directory: a desktop agent navigates the whole filesystem by absolute path,
// with ~ expanded by the tool layer (see pkg/fspath).
//
// Leaf tool adapter: no domain, no store, no handler. All three are read-only,
// share an injected PathGuard, and never touch AgentState.
//
// Package search 提供文件导航的 system tool（LS / Glob / Grep）。它们对应人在机器上
// 找文件的方式:打开文件夹看一眼(LS)/ 按名字找(Glob)/ 按内容找(Grep)——再用绝对
// 路径下钻。没有项目根、没有当前目录:桌面 agent 用绝对路径在整个文件系统导航,~ 由
// 工具层展开(见 pkg/fspath)。
//
// 叶子工具适配器:无 domain / store / handler。三者皆只读、共享注入的 PathGuard、永不碰 AgentState。
package search

import (
	"os/exec"

	"go.uber.org/zap"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	pathguardpkg "github.com/sunweilin/forgify/backend/internal/pkg/pathguard"
)

// SearchTools constructs the three navigation tools wired with their shared PathGuard.
//
// SearchTools 用共享 PathGuard 装配三件导航 tool。
func SearchTools(pathGuard pathguardpkg.PathGuard, log *zap.Logger) []toolapp.Tool {
	return []toolapp.Tool{
		&LS{pathGuard: pathGuard},
		&Glob{pathGuard: pathGuard},
		newGrep(pathGuard, log),
	}
}

// newGrep probes for ripgrep once at construction; an empty rgPath makes Grep
// fall back to the pure-Go stdlib backend (a desktop user may not have rg).
//
// newGrep 在构造时探测一次 ripgrep;rgPath 为空时 Grep 回落到纯 Go stdlib 后端
// （桌面用户可能没装 rg）。
func newGrep(pathGuard pathguardpkg.PathGuard, log *zap.Logger) *Grep {
	rgPath, _ := exec.LookPath("rg")
	return &Grep{pathGuard: pathGuard, rgPath: rgPath, log: log}
}
