// Package fspath normalizes user-supplied file paths into clean absolute paths.
//
// It is the single physical enforcement point of Forgify's "always absolute,
// never a current directory" rule. A desktop agent has no project root and no
// cwd — it navigates the whole machine by absolute path the way a person clicks
// through Finder. So every file tool (Read/Write/Edit/LS/Glob/Grep) resolves its
// path here: expand a leading ~ (the user's home, which the backend process knows
// natively via os.UserHomeDir — the agent itself does not know whose home it is),
// then reject anything that isn't absolute, because there is no cwd to resolve a
// relative path against.
//
// Package fspath 把用户给的文件路径规范成干净的绝对路径。
//
// 它是 Forgify「永远绝对、没有当前目录」铁律的唯一物理执行点。桌面 agent 没有项目根、
// 没有 cwd——它像人点 Finder 一样用绝对路径在整台机器上导航。故每个文件工具
// (Read/Write/Edit/LS/Glob/Grep) 都在此解析路径:展开开头的 ~(用户 home,后端进程
// 经 os.UserHomeDir 天然知道——agent 自己并不知道这是谁的 home),再拒绝任何非绝对
// 路径,因为没有 cwd 可用来解析相对路径。
package fspath

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrEmptyPath: path is empty or whitespace-only.
	//
	// ErrEmptyPath:路径为空或纯空白。
	ErrEmptyPath = errors.New("path is required")

	// ErrNotAbsolute: path is not absolute after ~ expansion. The agent has no
	// working directory to resolve a relative path against — unrecoverable here.
	//
	// ErrNotAbsolute:展开 ~ 后仍非绝对。agent 无工作目录可解析相对路径——此处不可恢复。
	ErrNotAbsolute = errors.New("path must be absolute (the agent has no working directory; pass an absolute path or one starting with ~)")

	// ErrNoHome: path starts with ~ but the OS home directory is unknown.
	//
	// ErrNoHome:路径以 ~ 开头但系统 home 目录未知。
	ErrNoHome = errors.New("cannot expand ~: home directory is unknown")
)

// Expand turns a user-supplied path into a clean absolute path. A leading "~" or
// "~/" expands to the OS home dir; the result must then be absolute. Bare "~" and
// "~/rest" are supported — "~user" is NOT (no cross-user resolution; it falls
// through to the not-absolute rejection).
//
// Expand 把用户路径变成干净绝对路径。开头的 "~" 或 "~/" 展开为系统 home;展开后结果
// 必须绝对。支持 "~" 和 "~/rest"——不支持 "~user"(不跨用户解析;它会落到非绝对拒绝)。
func Expand(path string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", ErrEmptyPath
	}

	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", ErrNoHome
		}
		if p == "~" {
			p = home
		} else {
			p = filepath.Join(home, p[2:])
		}
	}

	if !filepath.IsAbs(p) {
		return "", ErrNotAbsolute
	}
	return filepath.Clean(p), nil
}
