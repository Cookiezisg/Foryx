// bash_route.go — Bash auto-route to conversation sandbox env (sandbox.md
// §9.5). detectRuntime parses the shell command via mvdan.cc/sh/v3/syntax
// AST and walks every CallExpr looking for a runtime-bound command. This
// covers `bash -c "pip install ..."` (recurses into the -c argument),
// `env PYTHONPATH=. python ...` and `FOO=bar python ...` (env / leading
// assignments stripped), `/usr/bin/python3 ...` (path stripped), `cd &&
// ...` chains (every CallExpr in the tree is examined), command
// substitution / subshells (Walk descends into them), and `which python3`
// (introspection commands route based on their argument).
//
// Static escapes that no parser can see — `eval "..."`, `source
// ./script.sh`, `$(<dynamic-string>)` — remain best-effort and are called
// out in the Bash tool description so the LLM avoids them.
//
// bash_route.go ——Bash 自动路由到 conversation sandbox env（sandbox.md §9.5）。
// detectRuntime 通过 mvdan.cc/sh/v3/syntax AST 解析命令并 walk 每个 CallExpr
// 找 runtime 相关命令。覆盖 `bash -c "pip install ..."`（递归 `-c` 参数）、
// `env PYTHONPATH=. python ...` 与 `FOO=bar python ...`（env / 前导赋值剥除）、
// `/usr/bin/python3 ...`（路径剥除）、`cd && ...` 链（每个 CallExpr 都查）、
// command substitution / subshell（Walk 下钻）以及 `which python3`（自省命令
// 按 argument 路由）。
//
// 任何 parser 也看不到的静态逃逸——`eval "..."`、`source ./script.sh`、
// `$(<动态字符串>)`——仍是 best-effort，已在 Bash tool description 里
// 提示 LLM 避免这种写法。

package shell

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// runtimeDetector pairs a runtime kind with the regex that decides
// whether a bare command name "looks like" that runtime. Patterns are
// disjoint so order in the slice doesn't matter; first match wins.
//
// runtimeDetector 把 runtime kind 与决定裸命令名"看起来像"该 runtime 的
// regex 配对。pattern 不相交所以顺序无关，首次匹配胜。
type runtimeDetector struct {
	Kind    string
	Pattern *regexp.Regexp
}

// runtimeDetectors mirrors sandbox.md §9.5. Patterns are anchored against
// a *bare command name* (no path, no env-var prefix, no flags) — callers
// must normalise via stripPath / classifyCallExpr before matching. To
// extend, add one row here + one matching MiseInstaller registration in
// main.go's registerSandboxStack.
//
// runtimeDetectors 镜像 sandbox.md §9.5。pattern 锚定在*裸命令名*（无路径、
// 无 env var 前缀、无 flag）——调用方必须先经 stripPath / classifyCallExpr
// 规范化再匹配。扩展：加一行 + 在 main.go registerSandboxStack 加一个匹配
// MiseInstaller。
var runtimeDetectors = []runtimeDetector{
	{Kind: "python", Pattern: regexp.MustCompile(`^(?:python3?(?:\.\d+)?|pip3?|uv|virtualenv|pipenv|poetry)$`)},
	{Kind: "node", Pattern: regexp.MustCompile(`^(?:node|npm|npx|yarn|pnpm)$`)},
	{Kind: "rust", Pattern: regexp.MustCompile(`^(?:cargo|rustc|rustup)$`)},
	{Kind: "go", Pattern: regexp.MustCompile(`^go$`)},
	{Kind: "ruby", Pattern: regexp.MustCompile(`^(?:ruby|gem|bundle|bundler|rake)$`)},
	{Kind: "php", Pattern: regexp.MustCompile(`^(?:php|composer)$`)},
	{Kind: "java", Pattern: regexp.MustCompile(`^(?:java|javac|mvn|gradle)$`)},
	{Kind: "dotnet", Pattern: regexp.MustCompile(`^dotnet$`)},
}

// detectRuntime returns the runtime kind a shell command targets, or ""
// when no runtime-bound command is found anywhere in the parse tree.
// AST-based detection covers nested constructs that first-token regex
// cannot — see file header for the full list.
//
// Falls back to first-token regex when the parser rejects the input
// (rare; happens only on malformed shell or constructs sh.v3 can't
// handle).
//
// detectRuntime 返命令瞄准的 runtime kind；parse tree 任何位置都无 runtime
// 相关命令则返 ""。AST-based 检测覆盖 first-token regex 处理不到的嵌套构造
// （详见文件头）。parser 拒绝输入（罕见，仅畸形 shell 或 sh.v3 不支持的构造）
// 时 fallback 到 first-token regex。
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

// classifyCallExpr extracts the effective command name from a CallExpr —
// handling shell wrappers (`bash -c`), the `env` wrapper, leading
// assignments (already stripped by AST into call.Assigns), and
// introspection commands (`which X`) — and matches it against
// runtimeDetectors. Returns "" when the call is opaque (only dynamic
// expansions) or the command isn't runtime-bound.
//
// classifyCallExpr 从 CallExpr 提取有效命令名——处理 shell wrapper
// （`bash -c`）、`env` wrapper、前导赋值（AST 已剥到 call.Assigns）、自省命令
// （`which X`）——并匹配 runtimeDetectors。call 不透明（仅动态扩展）或命令非
// runtime 相关时返 ""。
func classifyCallExpr(call *syntax.CallExpr) string {
	args := callExprArgs(call)
	if len(args) == 0 {
		return ""
	}
	cmd := stripPath(args[0])

	// Shell wrapper: bash -c "..." / sh -c '...' — recurse into the -c
	// argument when it's a static literal.
	//
	// Shell wrapper：bash -c "..." / sh -c '...' ——`-c` 后参数为静态字面量
	// 时递归解析。
	if isShellWrapper(cmd) {
		if inner, ok := findDashCArg(args); ok && inner != "" {
			return detectRuntime(inner)
		}
		return ""
	}

	// env wrapper: env [KEY=VAL ...] [-flag ...] cmd args... — first
	// non-assignment non-flag arg is the real command.
	//
	// env wrapper：env [KEY=VAL ...] [-flag ...] cmd args... ——首个非赋值
	// 非 flag arg 是真命令。
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

	// Introspection: `which X` / `type X` / `command -v X` — route based
	// on what the LLM is looking up, so a conv-Python venv is reachable
	// even when the LLM asks "where is python".
	//
	// 自省：`which X` / `type X` / `command -v X` ——按 LLM 查的目标路由
	// （配了 conv-Python venv 的对话即使"问 python 在哪"也命中）。
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

// callExprArgs flattens a CallExpr's Args into plain strings. Words with
// dynamic parts (parameter expansion / command substitution) collapse to
// their literal portions only — opaque parts vanish, mirroring how
// shfmt's own consumers handle partially-static words.
//
// callExprArgs 把 CallExpr.Args 拍平成纯字符串。带动态部分（参数扩展 /
// command substitution）的 Word 仅取字面量部分——不透明部分丢弃，与 shfmt
// 自身消费者处理半静态 word 的方式一致。
func callExprArgs(call *syntax.CallExpr) []string {
	out := make([]string, 0, len(call.Args))
	for _, w := range call.Args {
		out = append(out, wordToString(w))
	}
	return out
}

// wordToString concatenates a Word's literal parts. Dynamic parts
// ($VAR, `cmd`, $(cmd)) contribute nothing — the result is the word's
// "static skeleton".
//
// wordToString 拼接 Word 的字面量部分。动态部分（$VAR / `cmd` / $(cmd)）
// 不贡献内容——返回 word 的"静态骨架"。
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

// matchDetector tests cmd against runtimeDetectors and returns the first
// matching kind, or "" when no detector matches.
//
// matchDetector 用 runtimeDetectors 测试 cmd 返首个匹配的 kind；无匹配返 ""。
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

// stripPath returns the basename of cmd (last `/` or `\` segment) so
// `/usr/bin/python3` matches the same detector as `python3`. Pure
// string operation; no filesystem access.
//
// stripPath 返 cmd 的 basename（最后一段路径），让 `/usr/bin/python3` 与
// `python3` 走同一 detector。纯字符串操作不访问 fs。
func stripPath(cmd string) string {
	if cmd == "" {
		return ""
	}
	if idx := strings.LastIndexAny(cmd, `/\`); idx >= 0 {
		return cmd[idx+1:]
	}
	return cmd
}

// isShellWrapper reports whether cmd is a known sub-shell launcher whose
// `-c <string>` argument carries the *real* command we want to classify.
//
// isShellWrapper 报告 cmd 是否已知 sub-shell 启动器（`-c <string>` 参数
// 携带我们真正想分类的命令）。
func isShellWrapper(cmd string) bool {
	switch cmd {
	case "bash", "sh", "dash", "zsh", "ksh", "ash":
		return true
	}
	return false
}

// findDashCArg returns the value following the first `-c` flag (or any
// short-flag cluster containing 'c', e.g. `-lc` / `-Bc`) in args, or
// ("", false) when no such flag is present or it has no value. POSIX
// shells let the user combine single-letter flags; bash's `-c` takes
// the command string as its argument regardless of cluster position.
//
// findDashCArg 返 args 中首个 `-c` flag（或包含 'c' 的短 flag cluster
// 如 `-lc` / `-Bc`）之后的值；无该 flag 或无值时返 ("", false)。POSIX
// shell 允许组合单字符 flag；bash 的 `-c` 不论在 cluster 哪个位置
// 都吃下一个 arg 当命令字符串。
func findDashCArg(args []string) (string, bool) {
	for i := 1; i+1 < len(args); i++ {
		a := args[i]
		if a == "-c" {
			return args[i+1], true
		}
		// Single-dash short-flag cluster (e.g. `-lc`, `-Bc`) — treat the
		// next arg as the -c value when the cluster contains 'c'. The
		// `len > 2` guard skips bare `--` and long `--flag` forms.
		//
		// 单短横短 flag cluster（如 `-lc` / `-Bc`）——cluster 含 'c'
		// 时把下一个 arg 当 -c 值。`len > 2` 守卫跳过裸 `--` 与长 `--flag`。
		if len(a) > 2 && a[0] == '-' && a[1] != '-' && strings.ContainsRune(a, 'c') {
			return args[i+1], true
		}
	}
	return "", false
}

// detectRuntimeFirstToken is the parse-failure fallback: takes the first
// whitespace-separated token of command and matches it against
// runtimeDetectors directly. Used only when mvdan.cc/sh's parser
// rejects the input (rare).
//
// detectRuntimeFirstToken 是 parse 失败的兜底：取命令首个空白分隔 token
// 直接匹配 runtimeDetectors。仅当 mvdan.cc/sh parser 拒绝输入时使用（罕见）。
func detectRuntimeFirstToken(command string) string {
	first := command
	if idx := strings.IndexAny(command, " \t"); idx > 0 {
		first = command[:idx]
	}
	return matchDetector(stripPath(first))
}

// envBinDirsForKind returns the directories under envPath that should
// prepend to PATH for the given runtime kind. Returns nil for kinds whose
// EnvManagers don't expose bin directories (Java uses classpath; Dotnet
// uses runtime PATH from the install dir, not from per-env scaffolding).
//
// envBinDirsForKind 返该 kind 应前置到 PATH 的 envPath 下目录。EnvManager
// 不暴露 bin 目录的 kind 返 nil（Java 用 classpath；Dotnet 从 install dir
// 的 runtime PATH 而非 per-env 脚手架）。
func envBinDirsForKind(envPath, kind string) []string {
	switch kind {
	case "python":
		// venv layout: bin/ on unix, Scripts/ on Windows.
		// venv 布局：unix bin/，Windows Scripts/。
		sub := "bin"
		if runtime.GOOS == "windows" {
			sub = "Scripts"
		}
		return []string{filepath.Join(envPath, ".venv", sub)}
	case "node":
		return []string{filepath.Join(envPath, "node_modules", ".bin")}
	case "rust", "go":
		return []string{filepath.Join(envPath, "bin")}
	case "ruby":
		return []string{filepath.Join(envPath, "bundle", "bin")}
	case "php":
		return []string{filepath.Join(envPath, "vendor", "bin")}
	default:
		return nil
	}
}

// prependPath returns env with PATH (or Path on Windows) updated so each
// dir in extras is prepended in order. Empty extras returns env unchanged.
//
// prependPath 返 env 把 PATH（Windows 上 Path）更新为 extras 中每个目录前置。
// extras 为空返 env 不变。
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

// envKeyEqual compares env-var keys case-insensitively on Windows
// (where PATH/Path/path all alias) and case-sensitively elsewhere.
//
// envKeyEqual 比较 env var key：Windows 大小写无关（PATH/Path/path 同义），
// 其他平台大小写敏感。
func envKeyEqual(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
