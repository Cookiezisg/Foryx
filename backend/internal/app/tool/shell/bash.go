// bash.go — Bash system tool: run a shell command in the user's
// environment. Two modes:
//
//   - Foreground (default): exec the command with the conversation cwd,
//     wait up to `timeout` ms (default 120s, hard max 600s), capture
//     combined stdout+stderr, return the result string with status footer.
//   - Background (`run_in_background: true`): spawn the command, register
//     in ProcessManager, return immediately with the bash_id the LLM
//     uses to poll BashOutput / KillShell. No timeout in background mode.
//
// cwd state machine: when the entire command (after Trim) matches
// `cd <path>`, we update AgentState's cwd and return success without
// invoking a subshell. All other commands run via `sh -c "<command>"`
// with that cwd. Chained `cd && other` is NOT tracked — same limitation
// a real terminal would show ("subshell exited; parent cwd unchanged").
//
// bash.go — Bash 系统工具：在用户环境跑 shell 命令。两模式：
//   - 前台（默认）：用对话 cwd 执行，至多等 timeout 毫秒（默认 120s 硬上限 600s），
//     捕获合并 stdout+stderr，返结果字符串（含状态尾注）。
//   - 后台（`run_in_background: true`）：spawn + 注册进 ProcessManager，
//     立即返 bash_id 让 LLM 用于 BashOutput / KillShell 轮询。后台无超时。
//
// cwd 状态机：整条命令（trim 后）匹配 `cd <path>` 时更新 AgentState cwd 并直接
// 返成功，不下到 subshell。其他命令用 `sh -c "<command>"` + 该 cwd 跑。
// 链式 `cd && other` 不追踪——与真实终端一致（"子 shell 退出后父 cwd 不变"）。
package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	sandboxapp "github.com/sunweilin/forgify/backend/internal/app/sandbox"
	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
	installprogresspkg "github.com/sunweilin/forgify/backend/internal/pkg/installprogress"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ── Limits & defaults ─────────────────────────────────────────────────────────

const (
	// defaultTimeoutMS is the foreground timeout when LLM doesn't supply.
	// 2 minutes covers typical build / test / install workflows.
	//
	// defaultTimeoutMS 是 LLM 未传 timeout 时的前台超时；2 分钟覆盖典型
	// build / test / install。
	defaultTimeoutMS = 120_000

	// maxTimeoutMS hard cap on a single foreground run. Anything truly
	// long-lived should go background.
	//
	// maxTimeoutMS 单次前台运行硬上限；真长跑应走 background。
	maxTimeoutMS = 600_000

	// outputCapBytes truncates the foreground result body. Same 256 KB
	// budget as bgBufferBytes so the LLM never gets a 10 MB blob.
	//
	// outputCapBytes 截断前台结果正文；与 bgBufferBytes 同 256 KB 预算。
	outputCapBytes = 256 * 1024
)

// ── Validation sentinels ──────────────────────────────────────────────────────

var (
	// ErrEmptyCommand: command missing or empty.
	// ErrEmptyCommand：command 缺失或为空。
	ErrEmptyCommand = errors.New("command is required and must be non-empty")

	// ErrInvalidTimeout: timeout outside [0, maxTimeoutMS]. 0 = use default.
	// ErrInvalidTimeout：timeout 不在 [0, maxTimeoutMS]；0 = 用默认。
	ErrInvalidTimeout = fmt.Errorf("timeout must be between 0 and %d ms", maxTimeoutMS)
)

// ── Description & schema ──────────────────────────────────────────────────────

const bashDescription = `Run a shell command on the user's machine.

Usage:
- ` + "`command`" + ` is the shell command. On macOS/Linux it runs via ` + "`/bin/sh -c`" + `; on Windows it runs via ` + "`cmd.exe /c`" + `. Use shell-portable syntax when possible. Examples: "ls -la" (unix) / "dir" (windows), "git status", "go test ./...".
- ` + "`description`" + ` is a one-line note for the human reader (e.g. "List repo files").
- ` + "`run_in_background: true`" + ` spawns the command without waiting and returns a bash_id; use BashOutput to poll for new output and KillShell to terminate.
- ` + "`timeout`" + ` (milliseconds, foreground only) defaults to 120000 (2 min); hard max 600000 (10 min). For longer-running tasks use background mode.
- The conversation has a tracked working directory: ` + "`cd <path>`" + ` as the entire command updates it; subsequent commands run there. Chained 'cd ... && ...' does not update the tracked cwd (matches normal subshell semantics).
- Combined stdout+stderr is returned, capped at 256 KB. Exit code appears in a status footer.
- This is a local single-user app — there is no banned-command list. Be careful with destructive commands; the user sees what you propose to run.

Sandbox auto-routing (packages do not pollute the host system):
- Commands that invoke a managed language runtime (` + "`pip`" + `, ` + "`python`" + `, ` + "`uv`" + `, ` + "`node`" + `, ` + "`npm`" + `, ` + "`npx`" + `, ` + "`pnpm`" + `, ` + "`cargo`" + `, ` + "`go`" + `, ` + "`gem`" + `, ` + "`bundle`" + `, ` + "`mvn`" + `, ` + "`gradle`" + `, ` + "`composer`" + `, ` + "`dotnet`" + `, etc.) automatically execute inside a per-conversation isolated environment. Detection covers nested forms — ` + "`bash -c \"pip install ...\"`" + `, ` + "`env VAR=val python ...`" + `, ` + "`/usr/bin/python3 ...`" + `, ` + "`cd /tmp && python ...`" + ` chains, subshells, and ` + "`which python3`" + `.
- The router cannot see through ` + "`eval \"...\"`" + `, ` + "`source ./script.sh`" + `, or commands hidden inside ` + "`$(<dynamic-string>)`" + ` substitutions — those run on the host system and pollute it. When installing packages or running scripts, write the runtime command directly (e.g. ` + "`pip install pandas`" + `, not ` + "`eval \"pip install pandas\"`" + `).`

var bashSchema = json.RawMessage(`{
	"type": "object",
	"required": ["command"],
	"properties": {
		"command": {
			"type": "string",
			"description": "Shell command to execute (POSIX sh)."
		},
		"description": {
			"type": "string",
			"description": "One-line human-readable description of what this command does."
		},
		"run_in_background": {
			"type": "boolean",
			"default": false,
			"description": "If true, spawn without waiting and return a bash_id for BashOutput / KillShell."
		},
		"timeout": {
			"type": "number",
			"description": "Foreground timeout in milliseconds (default 120000, hard max 600000). Ignored in background mode."
		}
	}
}`)

// ── Args ──────────────────────────────────────────────────────────────────────

type bashArgs struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	Background  bool   `json:"run_in_background"`
	Timeout     int    `json:"timeout"`
}

// ── Tool struct & 9 methods ───────────────────────────────────────────────────

// Bash implements the Bash system tool. mgr is shared with BashOutput +
// KillShell so all three operate on the same registry. sandbox is the
// optional auto-route target — when non-nil and ready, runtime-bound
// commands (`pip install`, `python ...`, `npm ...`, etc.) execute with
// PATH augmented by a per-conversation scratch env (sandbox.md §9.5).
// nil sandbox or a degraded service falls through to plain system shell.
//
// Bash struct 是 Bash 系统工具。mgr 与 BashOutput + KillShell 共享。sandbox
// 是可选自动路由目标——非 nil 且 ready 时，runtime 相关命令
// （`pip install`、`python ...`、`npm ...` 等）以 per-conversation scratch
// env 增强 PATH 执行（sandbox.md §9.5）。nil 或 degraded service 落到 plain
// system shell。
type Bash struct {
	mgr     *ProcessManager
	sandbox *sandboxapp.Service // optional; nil → no auto-route
}

// Identity --------------------------------------------------------------------

func (t *Bash) Name() string                { return "Bash" }
func (t *Bash) Description() string         { return bashDescription }
func (t *Bash) Parameters() json.RawMessage { return bashSchema }

// Static metadata -------------------------------------------------------------

func (t *Bash) IsReadOnly() bool        { return false }
func (t *Bash) NeedsReadFirst() bool    { return false }
func (t *Bash) RequiresWorkspace() bool { return false } // PathGuard intentionally NOT applied — see package doc

// Args-dependent hooks --------------------------------------------------------

// ValidateInput rejects empty commands and timeouts outside the allowed
// range.
//
// ValidateInput 拒绝空命令和越界 timeout。
func (t *Bash) ValidateInput(args json.RawMessage) error {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return fmt.Errorf("Bash.ValidateInput: %w", err)
	}
	if strings.TrimSpace(a.Command) == "" {
		return ErrEmptyCommand
	}
	if a.Timeout < 0 || a.Timeout > maxTimeoutMS {
		return ErrInvalidTimeout
	}
	return nil
}

func (t *Bash) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
	return toolapp.PermissionAllow
}

// ── Execute ───────────────────────────────────────────────────────────────────

// Execute dispatches:
//   - cd-only command → update AgentState.Cwd, return new cwd.
//   - background → spawn + register, return bash_id immediately.
//   - foreground → run with timeout, return stdout/stderr + exit code.
//
// Filesystem / OS errors are returned as LLM-friendly strings; only truly
// internal failures (post-validation JSON unmarshal) bubble up as Go err.
//
// Execute 分派：cd 单命令 → 更新 cwd 返新值；background → spawn + 注册返
// bash_id；foreground → 超时下跑、返输出 + exit code。
//
// 文件系统 / OS 错误返友好字符串；仅真正内部失败上抛 Go err。
func (t *Bash) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args bashArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("Bash.Execute: %w", err)
	}
	cmdText := strings.TrimSpace(args.Command)

	// cd-only short-circuit: update state and return without subshell.
	// cd-only 短路：更新 state 后直接返，不下 subshell。
	if target, ok := parseCDOnly(cmdText); ok {
		return t.handleCD(ctx, target)
	}

	cwd := resolveCwd(ctx)

	// Conversation auto-route: detect runtime kind, lazily create the
	// scratch env, derive PATH-prepend dirs. When the command targets
	// a sandboxed runtime (python/node/etc) and sandbox preparation
	// fails, surface the failure to the LLM rather than silently
	// falling through to system shell — running on /usr/bin/python3
	// would give the LLM stale facts (e.g. "macOS ships Python 3.9.6")
	// and violate §S3 "errors must not be swallowed".
	//
	// 对话自动路由：检测 runtime kind，懒建 scratch env，派生 PATH 前置
	// 目录。命令瞄准 sandboxed runtime（python/node 等）但 sandbox 准备
	// 失败时把错抛给 LLM，绝不静默降到系统 shell——/usr/bin/python3 给
	// LLM 假事实（"macOS 自带 Python 3.9.6"），违 §S3 "错误不吞"。
	extraPath, autoRouteErr := t.maybeAutoRoute(ctx, cmdText)
	if autoRouteErr != nil {
		return formatAutoRouteError(autoRouteErr), nil
	}

	if args.Background {
		return t.runBackground(ctx, cmdText, cwd, extraPath)
	}

	timeoutMS := args.Timeout
	if timeoutMS == 0 {
		timeoutMS = defaultTimeoutMS
	}
	return t.runForeground(ctx, cmdText, cwd, extraPath, time.Duration(timeoutMS)*time.Millisecond)
}

// formatAutoRouteError turns a sandbox-prep failure into a tool result
// the LLM can read and react to. The body explains what failed + the
// (likely actionable) reason, the footer marks exit -1 + a "[sandbox
// auto-route failed]" note so the LLM doesn't confuse it for a
// command-side error. The wrapper returns (string, nil) so the tool
// framework treats this as a normal tool result with explanatory body
// rather than retrying / hiding it as a tool-framework error.
//
// formatAutoRouteError 把 sandbox 准备失败转成 LLM 可读可响应的 tool
// result。body 说明哪步失败+原因，footer 加 "[sandbox auto-route failed]"
// 标 exit -1 让 LLM 不混淆为命令端错误。返 (string, nil) 让框架当成
// 普通 tool result（带解释 body）而非 tool 框架错误重试 / 隐藏。
func formatAutoRouteError(err error) string {
	body := "Sandbox auto-route could not prepare the runtime for this command. " +
		"The command was NOT executed (running on the system shell would " +
		"return misleading data — e.g. system Python 3.9.6 instead of " +
		"the conversation's isolated 3.12 venv). Please retry, or have " +
		"the user check the sandbox status in testend.\n\n" +
		"Reason: " + err.Error() + "\n"
	return formatForegroundResult(body, -1, "sandbox auto-route failed")
}

// maybeAutoRoute returns the bin directories to prepend onto PATH for
// a sandboxed-runtime command, plus an error when sandbox preparation
// failed for a runtime-bound command. Returning nil/nil means the
// command isn't runtime-bound (vanilla `ls` / `git status` etc.) — pass
// through to system shell with no PATH change. Returning extraPath/nil
// means we successfully prepared a conversation env. Returning nil/err
// means a runtime-bound command found sandbox unavailable; caller must
// surface the err to the LLM rather than fall through (running system
// /usr/bin/python3 would mislead the LLM, violating §S3).
//
// maybeAutoRoute 返 sandboxed-runtime 命令该前置到 PATH 的 bin 目录 +
// runtime 命令 sandbox 准备失败时的 err。nil/nil 表示非 runtime 命令
// （裸 `ls` / `git status`），按系统 shell 直传不改 PATH。extraPath/nil
// 是成功准备好 conv env。nil/err 是 runtime 命令撞 sandbox 不可用——
// 调用方必须把 err 抛给 LLM 不能 fallthrough（系统 /usr/bin/python3
// 会给 LLM 假事实，违 §S3）。
func (t *Bash) maybeAutoRoute(ctx context.Context, command string) ([]string, error) {
	kind := detectRuntime(command)
	if kind == "" {
		return nil, nil // non-runtime command — system shell is fine
	}
	if t.sandbox == nil {
		return nil, fmt.Errorf("shelltool.Bash.maybeAutoRoute: sandbox service not wired (this is a server build / config issue — please report)")
	}
	if !t.sandbox.IsReady() {
		bootErr := t.sandbox.BootstrapError()
		reason := "bootstrap incomplete"
		if bootErr != nil {
			reason = "bootstrap failed: " + bootErr.Error()
		}
		return nil, fmt.Errorf("shelltool.Bash.maybeAutoRoute: sandbox not ready (%s) — %s commands cannot run safely on the system shell", reason, kind)
	}
	convID, ok := reqctxpkg.GetConversationID(ctx)
	if !ok || convID == "" {
		return nil, fmt.Errorf("shelltool.Bash.maybeAutoRoute: no conversation context — %s commands need a conversation-scoped sandbox env", kind)
	}
	// owner.ID joins convID + runtimeKind with "_" (NOT ":"): owner.ID
	// becomes a literal directory name (sandbox.go:478) that prepends
	// to PATH at run time. PATH uses ":" as segment separator on POSIX,
	// so any ":" inside the path makes shell split it into nonexistent
	// sibling dirs and fall through to /usr/bin (running system Python
	// instead of mise's). convID is fixed-length "cv_<16hex>" (19
	// chars), runtime kinds are pure alphanumeric, so a single "_" is
	// unambiguous despite cv_ already containing one.
	//
	// owner.ID 用 "_" 而非 ":" 拼 convID 与 runtimeKind：owner.ID 直接
	// 当目录名（sandbox.go:478），目录前置到 PATH。POSIX PATH 用 ":"
	// 分隔，路径含 ":" 会被 shell 切成不存在的兄弟目录 → 落到 /usr/bin
	// 用系统 Python。convID 固定 "cv_<16hex>" 19 字符，runtime kind
	// 纯字母数字——单个 "_" 即便与 cv_ 中的下划线共存也无歧义。
	owner := sandboxdomain.Owner{
		Kind: sandboxdomain.OwnerKindConversation,
		ID:   convID + "_" + kind,
		Name: fmt.Sprintf("Conv %s scratch (%s)", convID, kind),
	}
	// Wrap EnsureEnv with installprogresspkg.Run so the install progress
	// streams as a progress block under the in-flight Bash tool_call
	// (parent comes from ctx — runOneTool stamps WithParentBlockID(tc.ID)
	// before invoking each tool). When ctx isn't a chat flow (e.g. test
	// harness without an active tool_call), the helper's callback no-ops
	// — sandbox_env notification still fires from the sandbox service
	// itself per the "always publish on state change" rule.
	//
	// 用 installprogresspkg.Run 包 EnsureEnv，让装包进度作为 progress
	// block 流式挂在当前 Bash tool_call 父下（runOneTool 调每个 tool 前
	// 已塞 WithParentBlockID(tc.ID)）。非 chat flow ctx（如测试 harness
	// 无活动 tool_call）下 helper 回调 no-op——sandbox_env notification
	// 仍由 sandbox service 按"状态变化必发"规则发。
	env, err := installprogresspkg.Run(ctx,
		map[string]any{
			"stage":   "preparing-runtime",
			"runtime": kind,
			"owner":   owner.ID,
		},
		func(progress sandboxdomain.ProgressFunc) (*sandboxdomain.Env, error) {
			return t.sandbox.EnsureEnv(ctx, owner, sandboxdomain.EnvSpec{
				Runtime: sandboxdomain.RuntimeSpec{Kind: kind},
			}, progress)
		})
	if err != nil {
		return nil, fmt.Errorf("shelltool.Bash.maybeAutoRoute: sandbox env install failed (%s for %s): %w", kind, convID, err)
	}
	envPath := filepath.Join(t.sandbox.SandboxRoot(), env.Path)
	return envBinDirsForKind(envPath, kind), nil
}

// ── cd state machine ──────────────────────────────────────────────────────────

// parseCDOnly returns (target, true) when cmd is exactly `cd` or
// `cd <path>` (no &&, no ;, no |). target may be empty meaning "home".
//
// parseCDOnly 在 cmd 恰为 `cd` 或 `cd <path>`（无 && / ; / |）时返
// (target, true)；target 为空意味着 home。
func parseCDOnly(cmd string) (string, bool) {
	trimmed := strings.TrimSpace(cmd)
	if trimmed == "cd" {
		return "", true
	}
	if !strings.HasPrefix(trimmed, "cd ") && !strings.HasPrefix(trimmed, "cd\t") {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "cd"))
	// Reject anything that contains shell metachars suggesting a chain.
	// 拒绝任何看起来像链式的 shell 元字符。
	if strings.ContainsAny(rest, "&|;<>`$") {
		return "", false
	}
	// Strip a single layer of surrounding quotes for ergonomics.
	// 去一层包裹引号，体验更顺。
	if len(rest) >= 2 && (rest[0] == '"' || rest[0] == '\'') && rest[0] == rest[len(rest)-1] {
		rest = rest[1 : len(rest)-1]
	}
	return rest, true
}

// handleCD validates the target dir, updates AgentState, and returns a
// confirmation. AgentState absent (chat layer didn't wire it) is reported
// as a soft error so the LLM can recover rather than being told nothing.
//
// handleCD 校验目标目录、更新 AgentState、返确认。AgentState 缺失（chat
// 层未接线）报软错让 LLM 可恢复。
func (t *Bash) handleCD(ctx context.Context, target string) (string, error) {
	if target == "" {
		if h, err := os.UserHomeDir(); err == nil {
			target = h
		} else {
			return "Cannot resolve home directory: " + err.Error(), nil
		}
	}
	// Resolve relative to current cwd.
	// 对 current cwd 解析相对路径。
	current := resolveCwd(ctx)
	if !filepath.IsAbs(target) {
		target = filepath.Join(current, target)
	}
	target = filepath.Clean(target)

	info, err := os.Stat(target)
	if err != nil {
		return fmt.Sprintf("cd: %s: %v", target, err), nil
	}
	if !info.IsDir() {
		return fmt.Sprintf("cd: not a directory: %s", target), nil
	}

	state, ok := reqctxpkg.GetAgentState(ctx)
	if !ok {
		return "cd: agent state missing — cwd not persisted across calls. Subsequent commands will use the process default cwd.", nil
	}
	state.SetCwd(target)
	return "Changed working directory to " + target, nil
}

// resolveCwd returns the conversation's tracked cwd, falling back to the
// process cwd when AgentState is missing or empty.
//
// resolveCwd 返对话级 cwd；AgentState 缺失或空时回落到进程 cwd。
func resolveCwd(ctx context.Context) string {
	if state, ok := reqctxpkg.GetAgentState(ctx); ok {
		if c := state.Cwd(); c != "" {
			return c
		}
	}
	if c, err := os.Getwd(); err == nil {
		return c
	}
	return "/"
}

// ── Foreground run ────────────────────────────────────────────────────────────

// runForeground execs command with a hard wall-clock timeout, captures
// combined stdout+stderr (capped), and returns a formatted result.
//
// runForeground 带硬墙钟超时执行 command，捕获合并 stdout+stderr（截断），
// 返格式化结果。
func (t *Bash) runForeground(ctx context.Context, command, cwd string, extraPath []string, timeout time.Duration) (string, error) {
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := buildShellCmd(runCtx, command, cwd, extraPath)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := capOutput(buf.Bytes())

	switch {
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		return formatForegroundResult(output, -1, fmt.Sprintf("command timed out after %s", timeout)), nil
	case errors.Is(runCtx.Err(), context.Canceled):
		// Parent ctx cancelled (user hit Cancel on the conversation, or the
		// chat agent loop ended). exec.CommandContext sent SIGKILL, so cmd.Run
		// returned an *exec.ExitError; without this branch we'd report it as
		// "exec failed: signal: killed" and confuse the LLM into thinking the
		// command itself crashed.
		//
		// 父 ctx 被取消（用户点了对话的 Cancel，或 chat agent 循环结束）。
		// exec.CommandContext 已发 SIGKILL，cmd.Run 返回 *exec.ExitError；
		// 不加这支会报 "exec failed: signal: killed"，误导 LLM 以为命令自己崩了。
		return formatForegroundResult(output, -1, "cancelled"), nil
	case err != nil:
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return formatForegroundResult(output, exitErr.ExitCode(), ""), nil
		}
		return formatForegroundResult(output, -1, "exec failed: "+err.Error()), nil
	}
	return formatForegroundResult(output, 0, ""), nil
}

// formatForegroundResult assembles the body + status footer the LLM sees.
// Output-empty case still includes the footer so the LLM never has to
// guess "did anything happen?".
//
// formatForegroundResult 拼正文 + 状态尾注；输出为空也带尾注，让 LLM 不必
// 猜"有没有跑成功"。
func formatForegroundResult(output string, exitCode int, note string) string {
	var sb strings.Builder
	sb.WriteString(output)
	if !strings.HasSuffix(output, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	if note != "" {
		fmt.Fprintf(&sb, "[%s]\n", note)
	}
	fmt.Fprintf(&sb, "[exit code: %d]", exitCode)
	return sb.String()
}

// capOutput trims output to outputCapBytes, indicating truncation when
// it had to cut.
//
// capOutput 截到 outputCapBytes，截断时附标注。
func capOutput(b []byte) string {
	if len(b) <= outputCapBytes {
		return string(b)
	}
	dropped := len(b) - outputCapBytes
	return fmt.Sprintf("...[truncated %d bytes from start]\n", dropped) + string(b[len(b)-outputCapBytes:])
}

// ── Background spawn ──────────────────────────────────────────────────────────

// runBackground starts the command without waiting, registers a BgProcess
// in the manager, and returns the assigned bash_id. The Wait+output pump
// run in a child goroutine that updates the BgProcess on completion.
//
// runBackground 不等待启动 command，注册 BgProcess 并返 bash_id；Wait + 输出
// pump 在子 goroutine 跑，结束时更新 BgProcess。
func (t *Bash) runBackground(ctx context.Context, command, cwd string, extraPath []string) (string, error) {
	convID, _ := reqctxpkg.GetConversationID(ctx)
	// Detached ctx — by design: background children outlive a single chat
	// turn. Using the request ctx would let conversation cancel kill running
	// `make build` / `npm run dev` / etc., which contradicts the whole point
	// of run_in_background:true. Final cleanup happens via KillShell or
	// ProcessManager.Stop() at backend shutdown.
	//
	// 切断 ctx——按设计：后台子进程 outlive 单次 chat turn。用请求 ctx 会让
	// 对话取消把跑着的 `make build` / `npm run dev` 等都杀掉，违背
	// run_in_background:true 的整个意图。最终清理走 KillShell 或 backend
	// 关停时的 ProcessManager.Stop()。
	cmd := buildShellCmd(context.Background(), command, cwd, extraPath)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Sprintf("Failed to open stdout pipe: %v", err), nil
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Sprintf("Failed to open stderr pipe: %v", err), nil
	}

	if err := cmd.Start(); err != nil {
		return fmt.Sprintf("Failed to start background command: %v", err), nil
	}

	proc := &BgProcess{
		ConvID:    convID,
		Command:   command,
		Cmd:       cmd,
		StartedAt: time.Now(),
		status:    StatusRunning,
	}
	t.mgr.Register(proc)

	// Pump stdout + stderr into the ring buffer concurrently so a noisy
	// process doesn't deadlock by filling one pipe.
	// 并发把 stdout + stderr 灌进环形缓冲，防一根管满死锁。
	var pumpWG sync.WaitGroup
	pumpWG.Add(2)
	go pumpReader(&pumpWG, proc, stdout)
	go pumpReader(&pumpWG, proc, stderr)

	// Reaper goroutine: wait for pumps to drain, then Wait the child to
	// reap zombies + capture exit code.
	// reaper goroutine：等 pump 排空再 Wait 子进程，reap 僵尸 + 抓 exit code。
	go func() {
		pumpWG.Wait()
		err := cmd.Wait()
		switch {
		case err == nil:
			proc.markFinished(StatusExited, 0)
		default:
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				if exitErr.ProcessState != nil && exitErr.ProcessState.Exited() {
					proc.markFinished(StatusExited, exitErr.ExitCode())
				} else {
					// Killed by signal — exit code is -1 in Go's model.
					// 被信号杀——Go 模型中 exit code 是 -1。
					proc.markFinished(StatusKilled, -1)
				}
			} else {
				proc.markErrored(err)
			}
		}
	}()

	return fmt.Sprintf(
		"Started background command (bash_id=%s): %s\nUse BashOutput with this bash_id to poll new output, or KillShell to terminate.",
		proc.ID, command,
	), nil
}

// pumpReader copies r into proc.appendOutput until EOF.
//
// pumpReader 把 r 拷贝到 proc.appendOutput 直至 EOF。
func pumpReader(wg *sync.WaitGroup, proc *BgProcess, r io.Reader) {
	defer wg.Done()
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			proc.appendOutput(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// ── Shell command builder ─────────────────────────────────────────────────────

// buildShellCmd creates the *exec.Cmd to run the user's command.
// Per-platform shell choice:
//   - Unix (macOS/Linux): /bin/sh -c "<cmd>"   (POSIX-conformant)
//   - Windows:            cmd.exe /c "<cmd>"   (Win32 builtin; always present)
//
// PowerShell is intentionally NOT used on Windows: cmd.exe is universally
// available + has predictable quoting, while PowerShell execution policy
// can block scripted invocations on locked-down corporate machines.
// Users wanting PowerShell can prefix their command (`powershell -Command "..."`).
//
// buildShellCmd 构造 *exec.Cmd。per-platform shell 选择：Unix（macOS/Linux）
// /bin/sh -c；Windows cmd.exe /c。故意不用 PowerShell——cmd.exe 永远在
// 且引号行为可预测；PowerShell execution policy 在锁定企业机可能拦脚本式
// 调用。要 PowerShell 的用户自己 `powershell -Command "..."` 前缀。
func buildShellCmd(ctx context.Context, command, cwd string, extraPath []string) *exec.Cmd {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	cmd.Dir = cwd
	cmd.Env = prependPath(os.Environ(), extraPath)
	return cmd
}

// ── Compile-time checks ───────────────────────────────────────────────────────

var _ toolapp.Tool = (*Bash)(nil)
