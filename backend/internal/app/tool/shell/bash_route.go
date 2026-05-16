package shell

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// runtimeDetector pairs a runtime kind with the regex matching its bare command names; patterns are disjoint.
//
// runtimeDetector 把 runtime kind 配对其裸命令名的 regex；pattern 互斥。
type runtimeDetector struct {
	Kind    string
	Pattern *regexp.Regexp
}

// runtimeDetectors only lists python + node — the runtimes the sandbox stack installs.
//
// runtimeDetectors 只列 python + node（sandbox stack 实装的 runtime）。
var runtimeDetectors = []runtimeDetector{
	{Kind: "python", Pattern: regexp.MustCompile(`^(?:python3?(?:\.\d+)?|pip3?|uv|virtualenv|pipenv|poetry)$`)},
	{Kind: "node", Pattern: regexp.MustCompile(`^(?:node|npm|npx|yarn|pnpm)$`)},
}

// detectRuntime returns the runtime kind targeted by command, or "" when none found in the parse tree.
//
// detectRuntime 返命令瞄准的 runtime kind；parse tree 无 runtime 命令则返 ""。
func detectRuntime(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	file, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		return detectRuntimeFirstToken(command)
	}
	var found string
	syntax.Walk(file, func(node syntax.Node) bool {
		if found != "" {
			return false
		}
		call, ok := node.(*syntax.CallExpr)
		if !ok {
			return true
		}
		if kind := classifyCallExpr(call); kind != "" {
			found = kind
			return false
		}
		return true
	})
	return found
}

// classifyCallExpr extracts the effective command name (unwrapping bash -c / env / which) and matches detectors.
//
// classifyCallExpr 提取有效命令名（解 bash -c / env / which 等 wrapper）并匹配 detector。
func classifyCallExpr(call *syntax.CallExpr) string {
	args := callExprArgs(call)
	if len(args) == 0 {
		return ""
	}
	cmd := stripPath(args[0])

	if isShellWrapper(cmd) {
		if inner, ok := findDashCArg(args); ok && inner != "" {
			return detectRuntime(inner)
		}
		return ""
	}

	if cmd == "env" {
		for _, arg := range args[1:] {
			if strings.HasPrefix(arg, "-") {
				continue
			}
			if eq := strings.IndexByte(arg, '='); eq > 0 {
				continue
			}
			return matchDetector(stripPath(arg))
		}
		return ""
	}

	// Introspection routes to the looked-up target so a conv venv is reachable for "which python".
	// 自省按查找目标路由，"which python" 仍可命中对话 venv。
	if cmd == "which" || cmd == "type" || cmd == "command" {
		for _, arg := range args[1:] {
			if strings.HasPrefix(arg, "-") {
				continue
			}
			if k := matchDetector(stripPath(arg)); k != "" {
				return k
			}
		}
		return ""
	}

	return matchDetector(cmd)
}

// callExprArgs flattens Args to strings; dynamic parts ($VAR / $(cmd)) drop to empty.
//
// callExprArgs 把 Args 拍平成字符串；动态部分（$VAR / $(cmd)）丢弃。
func callExprArgs(call *syntax.CallExpr) []string {
	out := make([]string, 0, len(call.Args))
	for _, w := range call.Args {
		out = append(out, wordToString(w))
	}
	return out
}

// wordToString concatenates a Word's literal parts; dynamic parts contribute nothing.
//
// wordToString 拼接 Word 的字面量部分，动态部分不贡献。
func wordToString(w *syntax.Word) string {
	var sb strings.Builder
	for _, p := range w.Parts {
		switch v := p.(type) {
		case *syntax.Lit:
			sb.WriteString(v.Value)
		case *syntax.SglQuoted:
			sb.WriteString(v.Value)
		case *syntax.DblQuoted:
			for _, pp := range v.Parts {
				if lit, ok := pp.(*syntax.Lit); ok {
					sb.WriteString(lit.Value)
				}
			}
		}
	}
	return sb.String()
}

func matchDetector(cmd string) string {
	if cmd == "" {
		return ""
	}
	for _, d := range runtimeDetectors {
		if d.Pattern.MatchString(cmd) {
			return d.Kind
		}
	}
	return ""
}

// stripPath returns cmd's basename so /usr/bin/python3 matches python3.
//
// stripPath 返 cmd basename，/usr/bin/python3 与 python3 同走 detector。
func stripPath(cmd string) string {
	if cmd == "" {
		return ""
	}
	if idx := strings.LastIndexAny(cmd, `/\`); idx >= 0 {
		return cmd[idx+1:]
	}
	return cmd
}

// isShellWrapper reports whether cmd is a sub-shell launcher carrying the real command in -c.
//
// isShellWrapper 报告 cmd 是否 sub-shell 启动器（真命令在 -c 后）。
func isShellWrapper(cmd string) bool {
	switch cmd {
	case "bash", "sh", "dash", "zsh", "ksh", "ash":
		return true
	}
	return false
}

// findDashCArg returns the value after the first `-c` flag (or short-cluster containing 'c') in args.
//
// findDashCArg 返 args 中首个 `-c` flag（或含 'c' 的短 cluster）后的值。
func findDashCArg(args []string) (string, bool) {
	for i := 1; i+1 < len(args); i++ {
		a := args[i]
		if a == "-c" {
			return args[i+1], true
		}
		if len(a) > 2 && a[0] == '-' && a[1] != '-' && strings.ContainsRune(a, 'c') {
			return args[i+1], true
		}
	}
	return "", false
}

// detectRuntimeFirstToken is the parse-failure fallback matching only the command's first token.
//
// detectRuntimeFirstToken 是 parse 失败的兜底，仅匹配命令首 token。
func detectRuntimeFirstToken(command string) string {
	first := command
	if idx := strings.IndexAny(command, " \t"); idx > 0 {
		first = command[:idx]
	}
	return matchDetector(stripPath(first))
}

// envBinDirsForKind returns PATH-prepend dirs for the runtime kind under envPath; nil for unknown kinds.
//
// envBinDirsForKind 返 kind 在 envPath 下应前置到 PATH 的目录；未知 kind 返 nil。
func envBinDirsForKind(envPath, kind string) []string {
	switch kind {
	case "python":
		sub := "bin"
		if runtime.GOOS == "windows" {
			sub = "Scripts"
		}
		return []string{filepath.Join(envPath, ".venv", sub)}
	case "node":
		return []string{filepath.Join(envPath, "node_modules", ".bin")}
	default:
		return nil
	}
}

// prependPath returns env with extras prepended to PATH (or Path on Windows); empty extras returns env unchanged.
//
// prependPath 返 env，把 extras 前置到 PATH（Windows 上是 Path）；extras 空时不变。
func prependPath(env []string, extras []string) []string {
	if len(extras) == 0 {
		return env
	}
	pathKey := "PATH"
	pathSep := ":"
	if runtime.GOOS == "windows" {
		pathKey = "Path"
		pathSep = ";"
	}
	prepend := strings.Join(extras, pathSep)
	out := make([]string, 0, len(env))
	replaced := false
	for _, kv := range env {
		if eq := strings.IndexByte(kv, '='); eq > 0 && envKeyEqual(kv[:eq], pathKey) {
			out = append(out, pathKey+"="+prepend+pathSep+kv[eq+1:])
			replaced = true
			continue
		}
		out = append(out, kv)
	}
	if !replaced {
		out = append(out, pathKey+"="+prepend)
	}
	return out
}

// envKeyEqual compares env-var keys case-insensitively on Windows, case-sensitively elsewhere.
//
// envKeyEqual 比较 env var key：Windows 大小写无关，其他平台敏感。
func envKeyEqual(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
