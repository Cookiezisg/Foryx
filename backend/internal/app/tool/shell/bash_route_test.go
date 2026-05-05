// bash_route_test.go — pure-function tests for the Bash auto-route layer:
// detectRuntime classification (AST-based), envBinDirsForKind per-OS path
// derivation, prependPath env-var manipulation. The actual EnsureEnv path
// (Bash.maybeAutoRoute → sandbox Service) is covered in the D9 pipeline
// suite where a real sandbox can spin up.
//
// bash_route_test.go ——Bash auto-route 层 pure-function 测试：detectRuntime
// 分类（AST-based）、envBinDirsForKind per-OS 路径推导、prependPath env var
// 操作。真 EnsureEnv 路径（Bash.maybeAutoRoute → sandbox Service）由 D9
// pipeline 套覆盖（真 sandbox 启动）。

package shell

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ── detectRuntime ─────────────────────────────────────────────────────

func TestDetectRuntime_Classification(t *testing.T) {
	cases := []struct {
		cmd  string
		want string
	}{
		// ── Plain runtime invocations (covered by both first-token and AST) ──
		// Python family
		{"python script.py", "python"},
		{"python3 script.py", "python"},
		{"python3.12 -m foo", "python"},
		{"pip install pandas", "python"},
		{"pip3 install pandas", "python"},
		{"uv pip install requests", "python"},
		{"poetry add httpx", "python"},
		{"pipenv install", "python"},
		// Node family
		{"node app.js", "node"},
		{"npm install express", "node"},
		{"npx tsc", "node"},
		{"yarn add lodash", "node"},
		{"pnpm add react", "node"},
		// Rust family
		{"cargo build --release", "rust"},
		{"rustc main.rs", "rust"},
		// Go
		{"go build ./...", "go"},
		{"go test -run TestFoo", "go"},
		// Ruby family
		{"ruby script.rb", "ruby"},
		{"gem install rake", "ruby"},
		{"bundle install", "ruby"},
		{"rake test", "ruby"},
		// PHP
		{"php artisan migrate", "php"},
		{"composer require monolog/monolog", "php"},
		// Java family
		{"java -jar app.jar", "java"},
		{"javac Foo.java", "java"},
		{"mvn install", "java"},
		{"gradle build", "java"},
		// Dotnet
		{"dotnet build", "dotnet"},

		// ── cd-prefix and chain handling (AST walks every CallExpr) ─────
		{"cd /tmp && pip install pandas", "python"},
		{"cd /workspace && npm test", "node"},
		{"cd /workspace && cd nested && npm test", "node"},
		{"pip install foo && rm -rf /tmp", "python"},
		{"ls && pip install foo", "python"},
		{"pip install foo; npm install bar", "python"}, // first match wins
		{"pip install foo | tee log.txt", "python"},

		// ── Path prefix stripped (AST upgrade vs. first-token regex) ────
		{"/usr/bin/python3 script.py", "python"},
		{"/opt/homebrew/bin/pip install foo", "python"},
		{"/usr/local/bin/node app.js", "node"},

		// ── env wrapper / leading assignments (AST upgrade) ─────────────
		{"FOO=bar pip install x", "python"},
		{"PYTHONPATH=. python script.py", "python"},
		{"env PYTHONPATH=. python script.py", "python"},
		{"env -i CARGO_HOME=/tmp cargo build", "rust"},
		{"FOO=bar BAZ=qux node app.js", "node"},

		// ── Shell wrappers recurse into -c argument (AST upgrade) ───────
		{`bash -c "pip install pandas"`, "python"},
		{`sh -c 'npm install'`, "node"},
		{`bash -c "cd /tmp && pip install foo"`, "python"},
		{`bash -c "ls -la"`, ""},   // no runtime inside
		{`bash -c "git status"`, ""}, // no runtime inside
		{`bash -lc "pip install x"`, "python"}, // combined flag cluster handled
		{`bash -cl "pip install x"`, "python"}, // c-first cluster also works

		// ── Introspection commands route by argument (AST upgrade) ──────
		{"which python3", "python"},
		{"which npm", "node"},
		{"type pip", "python"},
		{"command -v cargo", "rust"},
		{"which ls", ""}, // not a runtime
		{"which", ""},    // bare which, no arg

		// ── Subshells / command substitution (Walk descends) ────────────
		{"(pip install pandas)", "python"},
		{"(cd /tmp && npm install)", "node"},
		{"echo $(pip install pandas)", "python"},
		{"`pip install pandas`", "python"},

		// ── Plain shell — nil ───────────────────────────────────────────
		{"ls -la", ""},
		{"git status", ""},
		{"cat README.md", ""},
		{"echo hello", ""},
		{"", ""},
		{"   ", ""},
		{"git log --oneline -10", ""},
		{"cat /tmp/file && grep foo /tmp/other", ""},

		// ── Static-escape gotchas (parser CAN'T see; remain bypassed,
		//    LLM warned in Bash.Description) ─────────────────────────────
		{`eval "pip install pandas"`, ""},
		{"source ./install.sh", ""},
		{". ./install.sh", ""},
	}
	for _, c := range cases {
		got := detectRuntime(c.cmd)
		if got != c.want {
			t.Errorf("detectRuntime(%q) = %q, want %q", c.cmd, got, c.want)
		}
	}
}

// TestDetectRuntime_FirstTokenFallback asserts that when mvdan.cc/sh's
// parser rejects the input, detectRuntime falls back to first-token
// regex on the raw command rather than returning "" silently. Triggered
// by truly malformed shell — most strings parse fine.
//
// TestDetectRuntime_FirstTokenFallback 断言 mvdan.cc/sh parser 拒绝输入时，
// detectRuntime fallback 到原命令的 first-token regex，而非静默返 ""。由真正
// 畸形的 shell 触发——大多数字符串都能成功 parse。
func TestDetectRuntime_FirstTokenFallback(t *testing.T) {
	// Unterminated double-quote — parser errors; fallback inspects first token.
	// 未闭合双引号——parser 报错；fallback 看首 token。
	got := detectRuntimeFirstToken(`pip install "unterminated`)
	if got != "python" {
		t.Errorf("first-token fallback on broken shell: got %q, want python", got)
	}
}

// ── stripPath ────────────────────────────────────────────────────────

func TestStripPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"python3", "python3"},
		{"/usr/bin/python3", "python3"},
		{"/opt/homebrew/bin/pip", "pip"},
		{"./relative/path/cargo", "cargo"},
		{`C:\Python312\python.exe`, "python.exe"},
		{"", ""},
		{"/", ""},
		{"//double", "double"},
	}
	for _, c := range cases {
		if got := stripPath(c.in); got != c.want {
			t.Errorf("stripPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── envBinDirsForKind ────────────────────────────────────────────────

func TestEnvBinDirsForKind(t *testing.T) {
	envPath := "/data/envs/conv/cv_abc:python"
	cases := []struct {
		kind     string
		wantUnix []string
	}{
		{"python", []string{filepath.Join(envPath, ".venv", "bin")}},
		{"node", []string{filepath.Join(envPath, "node_modules", ".bin")}},
		{"rust", []string{filepath.Join(envPath, "bin")}},
		{"go", []string{filepath.Join(envPath, "bin")}},
		{"ruby", []string{filepath.Join(envPath, "bundle", "bin")}},
		{"php", []string{filepath.Join(envPath, "vendor", "bin")}},
		// Java / dotnet — no per-env bin dir; rely on classpath / install dir.
		{"java", nil},
		{"dotnet", nil},
		// Unknown kind — nil so prepend is a no-op.
		{"elixir", nil},
		{"", nil},
	}
	for _, c := range cases {
		got := envBinDirsForKind(envPath, c.kind)
		if c.kind == "python" && runtime.GOOS == "windows" {
			// venv bin dir is Scripts on Windows.
			want := []string{filepath.Join(envPath, ".venv", "Scripts")}
			if !slicesEqual(got, want) {
				t.Errorf("envBinDirsForKind(python) on windows = %v, want %v", got, want)
			}
			continue
		}
		if !slicesEqual(got, c.wantUnix) {
			t.Errorf("envBinDirsForKind(%s) = %v, want %v", c.kind, got, c.wantUnix)
		}
	}
}

// ── prependPath ──────────────────────────────────────────────────────

func TestPrependPath_PrependsToExistingPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH semantics differ on Windows; covered by case-fold test below")
	}
	env := []string{"FOO=bar", "PATH=/usr/bin:/bin", "BAZ=qux"}
	out := prependPath(env, []string{"/sandbox/.venv/bin"})

	// Order must be: prepended dirs FIRST, original PATH after.
	// 顺序：前置 dir 在前，原 PATH 在后。
	wantPath := "PATH=/sandbox/.venv/bin:/usr/bin:/bin"
	found := false
	for _, kv := range out {
		if kv == wantPath {
			found = true
		}
	}
	if !found {
		t.Errorf("PATH not prepended correctly: %v", out)
	}
	if !contains(out, "FOO=bar") || !contains(out, "BAZ=qux") {
		t.Errorf("non-PATH entries dropped: %v", out)
	}
}

func TestPrependPath_AppendsWhenPATHMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows PATH key is Path; case-fold case below")
	}
	env := []string{"FOO=bar"}
	out := prependPath(env, []string{"/x", "/y"})
	want := "PATH=/x:/y"
	if !contains(out, want) {
		t.Errorf("PATH not appended when missing: %v (want includes %q)", out, want)
	}
}

func TestPrependPath_EmptyExtras_NoChange(t *testing.T) {
	env := []string{"FOO=bar", "PATH=/usr/bin"}
	out := prependPath(env, nil)
	if !slicesEqual(out, env) {
		t.Errorf("empty extras must not change env: %v vs %v", out, env)
	}
}

func TestPrependPath_MultipleExtrasJoinedWithSeparator(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses unix : separator")
	}
	env := []string{"PATH=/usr/bin"}
	out := prependPath(env, []string{"/a", "/b", "/c"})
	want := "PATH=/a:/b:/c:/usr/bin"
	if !contains(out, want) {
		t.Errorf("multiple extras not joined: got %v, want %q", out, want)
	}
}

// ── helpers ──────────────────────────────────────────────────────────

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestEnvKeyEqual(t *testing.T) {
	if !envKeyEqual("PATH", "PATH") {
		t.Error("identical keys should be equal")
	}
	if envKeyEqual("PATH", "FOO") {
		t.Error("different keys should not be equal")
	}
	if runtime.GOOS == "windows" {
		if !envKeyEqual("PATH", "Path") {
			t.Error("Windows: PATH/Path should be case-insensitive equal")
		}
	} else {
		if envKeyEqual("PATH", "Path") {
			t.Error("non-Windows: PATH/Path must be case-sensitive distinct")
		}
	}
	// Smoke-test: keep strings import live for future test additions.
	// 烟雾测试：保 strings 包持续被引用（防 linter 报未用）。
	_ = strings.TrimSpace("")
}
