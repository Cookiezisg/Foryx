# 03 — Shell Tools 深挖

> 02-tools-deep 系列第三篇——**最大的一篇**。
> ✅ **Bash** — Claude Code 标准 tool，description 由 **41 个子描述** 组合而成
> ⚠️ **Monitor** — v2.1.98 新增；Forgify P2，本篇仅设计走查不实现

## 信息源

- **主源**：Piebald-AI 的 41 个 `tool-description-bash-*.md` + 1 个 `tool-description-background-monitor-streaming-events.md`，每个文件 1–10 行，组合起来构成 LLM 看到的完整 Bash description
- **副源**：[code.claude.com tools-reference](https://code.claude.com/docs/en/tools-reference) `Bash tool behavior` / `PowerShell tool` / `Monitor tool` 三节
- **写作日期**：2026-05-03

⚠️ **关键认知**：CC 的 Bash description 不是一段连贯文本，而是**41 块拼图**——每条都是 Anthropic 在某次事故 / 用户反馈 / 模型 misuse 之后立的规矩。研读这些规矩本身就是 agent design 教材。Forgify 不必复刻全部，但需要**理解每条背后是什么 failure mode**。

---

## Bash

### 1. Description 拼装

LLM 看到的 Bash description 是这样组装的（按 ccVersion 升序简列）：

```
[overview]                                   ← "Executes a given bash command and returns its output."
[working-directory]                          ← "The working directory persists between commands, but shell state does not..."
[built-in-tools-note]                        ← 前言："built-in tools 比 Bash 等价物 UX 更好"
[prefer-dedicated-tools]                     ← "IMPORTANT: Avoid using Bash for read-only/searching commands when dedicated tools exist:"
  ├─ [bash-alternative-read-files]           ← "Read files: Use Read (NOT cat/head/tail)"
  ├─ [bash-alternative-edit-files]           ← "Edit files: Use Edit (NOT sed/awk)"
  ├─ [bash-alternative-write-files]          ← "Write files: Use Write (NOT echo >/cat <<EOF)"
  ├─ [bash-alternative-content-search]       ← "Content search: Use Grep (NOT grep or rg)"
  ├─ [bash-alternative-file-search]          ← "File search: Use Glob (NOT find or ls)"
  └─ [bash-alternative-communication]        ← "Communication: Output text directly (NOT echo/printf)"

[Instructions]
  ├─ [verify-parent-directory]               ← "ls 验证 parent 存在"
  ├─ [quote-file-paths]                      ← '路径含空格需要双引号'
  ├─ [maintain-cwd]                          ← '尽量绝对路径，避免 cd'
  ├─ [timeout]                               ← 'optional timeout up to ${MAX_MS}ms'
  ├─ [run_in_background]                     ← 描述参数（推测在 schema 里）
  └─ [Multiple commands]
      ├─ [parallel-commands]                 ← 'independent → 一个 message 多个 tool call'
      ├─ [sequential-commands]               ← 'dependent → 单 call 用 &&'
      ├─ [semicolon-usage]                   ← '只关心顺序不关心失败用 ;'
      └─ [no-newlines]                       ← '禁用 newline 分隔（quoted string 内可以）'

[sleep guidelines]
  ├─ [sleep-run-immediately]                 ← '能立即跑就不要 sleep'
  ├─ [sleep-no-polling-background-tasks]     ← 'run_in_background 自有通知，禁 poll'
  ├─ [sleep-keep-short]                      ← 'sleep 短一点'
  └─ [sleep-use-check-commands]              ← 'poll 外部进程用 check command 而非 sleep'

[git block]
  ├─ [git-prefer-new-commits]
  ├─ [git-avoid-destructive-ops]
  ├─ [git-never-skip-hooks]
  └─ [git-commit-and-pr-creation-instructions]   ← 大段（~150 行的 commit + PR 创建指南）

[sandbox block (~17 子描述)]                  ← 详见 §5
```

**Forgify 要不要复刻全部 41 块？** 不必。下面按主题分组复刻必要的、跳过不适用的。

### 2. JSON Schema（推断）

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "additionalProperties": false,
  "required": ["command"],
  "properties": {
    "command": {
      "type": "string",
      "description": "The shell command to execute"
    },
    "timeout": {
      "type": "number",
      "description": "Optional timeout in milliseconds (default 120000, max 600000)"
    },
    "description": {
      "type": "string",
      "description": "5-10 word description of what this command does (shown in UI / transcript)"
    },
    "run_in_background": {
      "type": "boolean",
      "default": false,
      "description": "Run command asynchronously; use BashOutput / Read on output file to retrieve later"
    }
  }
}
```

✅ 字段名 (`command` / `timeout` / `description` / `run_in_background`) 来自 wong2 gist + 官方 docs 交叉确认。

CC 实际 schema 可能还有：
- `dangerouslyDisableSandbox: boolean` —— v2.1.126 起被 policy 强制 false（"All commands MUST run in sandbox mode"），但 schema 字段仍存在
- `shell: string` —— v2.1.126 加入用于 PowerShell；CC 内部用，LLM 不直接传

Forgify 不做 sandbox，所以 `dangerouslyDisableSandbox` 字段不要；`shell` 也不要（一律用系统 sh）。

### 3. 子描述详解

#### 3.1 工作目录与状态（核心）

| 子描述 | 原文 |
|---|---|
| working-directory | The working directory persists between commands, but shell state does not. The shell environment is initialized from the user's profile (bash or zsh). |
| maintain-cwd | Try to maintain your current working directory throughout the session by using absolute paths and avoiding usage of `cd`. You may use `cd` if the User explicitly requests it. In particular, never prepend `cd <current-directory>` to a `git` command — `git` already operates on the current working tree, and the compound triggers a permission prompt. |

**关键约定**：
- **cwd 持久** ✅——一次 `cd /foo` 后，下条命令的 cwd 仍是 `/foo`
- **环境变量不持久** ✅——`export X=1` 在下条命令里 `$X` 是空。**这是 CC 已知限制 #2508/#20503，至今未修**
- **shell 初始化文件加载** ✅——首次启动 shell 会加载 `.bashrc` / `.zshrc`；子命令不重新加载
- LLM 应**用绝对路径** + 避免 cd（cd 改变 cwd 会触发 permission prompt）

**实现机制（推测）**：
- CC 维护一个 `inMemoryCwd` 字段
- 每条 Bash 命令实际执行：`cd <inMemoryCwd> && <command>`
- 命令前 parse 是否含 `cd ...`，如果是则更新 inMemoryCwd
- 不真正复用 shell 进程——每次都是 `bash -c "cd <cwd> && <cmd>"` 新进程
- 这就是为何 env 不持久——新进程不继承上一次的 export

#### 3.2 命令组合规则（4 条）

| 子描述 | 原文 | 何时用 |
|---|---|---|
| parallel-commands | If the commands are independent and can run in parallel, make multiple **${BASH_TOOL_NAME}** tool calls in a single message. Example: if you need to run "git status" and "git diff", send a single message with two **${BASH_TOOL_NAME}** tool calls in parallel. | 多个**独立**命令 |
| sequential-commands | If the commands depend on each other and must run sequentially, use a single **${BASH_TOOL_NAME}** call with '&&' to chain them together. | 多个有依赖的命令，前者失败则后者不跑 |
| semicolon-usage | Use ';' only when you need to run commands sequentially but don't care if earlier commands fail. | 顺序无所谓失败时 |
| no-newlines | DO NOT use newlines to separate commands (newlines are ok in quoted strings). | universal |

**Forgify 关注点**：parallel-commands 规矩在我们的实现里走的是**框架级 `execution_group` 设计**（不是 CC 的 `IsConcurrencySafe(args)` 反推）。LLM 自己给每个 tool call 标 group 号，runTools 按 group 分批。详见末节 §框架级变更。Bash 调用方所有 schema 注入由 framework 完成，本节不再讨论 IsConcurrencySafe。

#### 3.3 路径安全（2 条）

| 子描述 | 原文 |
|---|---|
| quote-file-paths | Always quote file paths that contain spaces with double quotes in your command (e.g., `cd "path with spaces/file.txt"`) |
| verify-parent-directory | If your command will create new directories or files, first use this tool to run `ls` to verify the parent directory exists and is the correct location. |

⚠️ verify-parent-directory 跟 §3.1 的 prefer-dedicated-tools 冲突（后者说"用 LS tool 而不是 Bash ls"，但 LS 已下线）。**实际 CC behavior**：用 `ls` Bash 命令，不用 dedicated tool（因为 LS tool v2.1.126 已下线，§02-search.md 已确认）。

#### 3.4 Timeout（1 条）

> You may specify an optional timeout in milliseconds (up to **${GET_MAX_TIMEOUT_MS()}**ms / **${GET_MAX_TIMEOUT_MS()/60000}** minutes). By default, your command will timeout after **${GET_DEFAULT_TIMEOUT_MS()}**ms (**${GET_DEFAULT_TIMEOUT_MS()/60000}** minutes).

| 占位符 | 典型值 |
|---|---|
| `GET_DEFAULT_TIMEOUT_MS` | **120000** (2 min) |
| `GET_MAX_TIMEOUT_MS` | **600000** (10 min) |

**Forgify 抄同样的数：120s 默认 / 600s 上限**。

#### 3.5 Sleep 哲学（4 条；agent design 金句）

| 子描述 | 原文 |
|---|---|
| sleep-run-immediately | Do not sleep between commands that can run immediately — just run them. |
| sleep-no-polling-background-tasks | If waiting for a background task you started with `run_in_background`, you will be notified when it completes — do not poll. |
| sleep-keep-short | If you must sleep, keep the duration short to avoid blocking the user. |
| sleep-use-check-commands | If you must poll an external process, use a check command (e.g. `gh run view`) rather than sleeping first. |

**4 条加起来的核心命题**：
> Agent 行为应该是**事件驱动 + 短轮询 check command**，禁用"sleep 再看"模式。

这条是 CC 在长任务体验上的核心设计——避免 LLM 用 sleep 占满 turn budget。Forgify chat 的 LLM prompt 里也应该塞同样的精神，不必逐字抄但要传达。

#### 3.6 Git 工作流（4 条 + 1 大段）

| 子描述 | 原文（核心） |
|---|---|
| git-prefer-new-commits | Prefer to create a new commit rather than amending an existing commit. |
| git-avoid-destructive-ops | Before running destructive operations (e.g., `git reset --hard`, `git push --force`, `git checkout --`), consider whether there is a safer alternative... Only use destructive operations when they are truly the best approach. |
| git-never-skip-hooks | Never skip hooks (`--no-verify`) or bypass signing (`--no-gpg-sign`, `-c commit.gpgsign=false`) unless the user has explicitly asked for it. If a hook fails, investigate and fix the underlying issue. |
| git-commit-and-pr-creation-instructions | 大段 ~150 行（含 Git Safety Protocol、commit step 1-4、HEREDOC 范例、PR 创建步骤、`gh pr create` 模板等） |

**Forgify 怎么处理？**
- Git 不是 Forgify 主线（用户用 Forgify 是建 forge / workflow，不是写 commit）
- **决策：完全不抄**——既不抄 4 短条，也不抄 commit safety protocol，也不抄 PR 创建指南
- 理由：靠 LLM 训练自带的 git 常识发挥；Bash description 保持清爽（~30 行省下来给真有价值的 cwd / sleep / tool 偏好提示）
- 风险接受：LLM 可能误跑 `git push --force` —— 这种灾难命令由本节 §6.4 `isDangerousCommand` 检测兜底（pattern match `git push --force` → 拒绝执行）

#### 3.7 Tool 偏好提示（已在 §01 §02 处理）

`prefer-dedicated-tools` + 6 个 `bash-alternative-*` 子描述告诉 LLM "有 dedicated tool 就别用 Bash"。Forgify 实现时把这段拼到 Bash description，列出我们有的 tool（Read/Write/Edit/Glob/Grep/LS）。

### 4. 算法行为汇总

按上面 §3 散点拢成一份执行步骤：

```
LLM 提交 tool call: {command, timeout?, description?, run_in_background?}

ValidateInput:
  - command 必填 + 非空
  - timeout 范围 [0, 600000]（0 视为默认）
  - description 长度 <= 100 字符（可选）

CheckPermissions（视 mode）:
  - default: 多数命令直接 Allow；含 destructive pattern（rm -rf / git push --force / etc）→ Ask
  - acceptEdits / bypass: 视配置

Execute:
  if run_in_background:
    起子进程，stdout/stderr 写到 /tmp/forgify-bg/{taskID}.log
    立即返回 "Background task started, ID: {taskID}"
    （后续靠 BashOutput / Read on log file 拿输出）
  else:
    用当前 cwd 拼 "cd <cwd> && <command>"
    ctx + timeout 包裹 exec.CommandContext
    捕 stdout + stderr (合并)
    跑完拿 exit code
    parse command 是否含 cd → 更新 in-memory cwd
    return formatted output
```

### 5. Sandbox 系统（CC 最复杂的子系统）

CC 在 v2.1.86 后引入 OS-level 沙箱（macOS 用 `sandbox-exec`，Linux 推测 `bwrap` / `firejail`）。Piebald 关于 sandbox 的 17 块拼图：

#### 5.1 强制策略（5 条）

| 子描述 | 原文 |
|---|---|
| sandbox-mandatory-mode | All commands MUST run in sandbox mode - the `dangerouslyDisableSandbox` parameter is disabled by policy. |
| sandbox-default-to-sandbox | You should always default to running commands within the sandbox. Do NOT attempt to set `dangerouslyDisableSandbox: true` unless: |
| sandbox-per-command | Treat each command you execute with `dangerouslyDisableSandbox: true` individually. Even if you have recently run a command with this setting, you should default to running future commands within the sandbox. |
| sandbox-no-exceptions | Commands cannot run outside the sandbox under any circumstances. |
| sandbox-no-sensitive-paths | Do not suggest adding sensitive paths like `~/.bashrc`, `~/.zshrc`, `~/.ssh/*`, or credential files to the sandbox allowlist. |

#### 5.2 失败诊断（5 条 evidence + 2 条结构）

LLM 看到失败时怎么判定"是沙箱阻挡的还是命令本身错"——CC 给了一个 evidence pattern 表：

> **Evidence of sandbox-caused failures includes:**
> - Access denied to specific paths outside allowed directories
> - "Operation not permitted" errors for file/network operations
> - Network connection failures to non-whitelisted hosts
> - Unix socket connection errors

**+ failure-evidence-condition**：
> A specific command just failed and you see evidence of sandbox restrictions causing the failure. Note that commands can fail for many reasons unrelated to the sandbox (missing files, wrong arguments, network issues, etc.).

#### 5.3 失败响应（5 条）

| 子描述 | 原文 |
|---|---|
| sandbox-response-header | When you see evidence of sandbox-caused failure: |
| sandbox-explain-restriction | Briefly explain what sandbox restriction likely caused the failure. Be sure to mention that the user can use the `/sandbox` command to manage restrictions. |
| sandbox-retry-without-sandbox | Immediately retry with `dangerouslyDisableSandbox: true` (don't ask, just do it) |
| sandbox-user-permission-prompt | This will prompt the user for permission |
| sandbox-adjust-settings | If a command fails due to sandbox restrictions, work with the user to adjust sandbox settings instead. |

⚠️ retry-without-sandbox 跟 mandatory-mode 表面冲突，实际是**两套 description 互斥版本**——根据安装模式（policy 锁死 vs 用户可选）下发不同集合。Forgify 简化版：不做 sandbox（详 §6.3）。

#### 5.4 TMPDIR 约定（1 条）

> For temporary files, always use the `$TMPDIR` environment variable. TMPDIR is automatically set to the correct sandbox-writable directory in sandbox mode. Do NOT use `/tmp` directly - use `$TMPDIR` instead.

**为何**：沙箱模式下 `/tmp` 不可写，`$TMPDIR` 由 CC 设到沙箱白名单内的临时目录。Forgify 不做沙箱时 `$TMPDIR` 走系统默认即可。

### 6. Forgify Go 实现要点

#### 6.1 Tool 接口（精简后 9 方法）

⚠️ **`IsConcurrencySafe(args)` 方法已移除**——见末节 §框架级变更。并行调度改由 LLM 自报的 `execution_group` 字段驱动，tool 不再参与判断。

```go
type Bash struct {
    workspace PathGuard
    state     *BashState
    bgStore   BackgroundStore
}

type BashState struct {
    mu  sync.RWMutex
    cwd string  // session-persistent working directory
}

func (t *Bash) Name() string                  { return "Bash" }
func (t *Bash) IsReadOnly() bool              { return false }
func (t *Bash) NeedsReadFirst() bool          { return false }
func (t *Bash) RequiresWorkspace() bool       { return true }   // Phase 5 cwd 必须在白名单
// 不再有 IsConcurrencySafe 方法
```

#### 6.2 cwd 持久化（核心）

```go
type BashState struct {
    mu  sync.RWMutex
    cwd string
}

func (s *BashState) Cwd() string {
    s.mu.RLock()
    defer s.mu.RUnlock()
    if s.cwd == "" {
        cwd, _ := os.Getwd()
        return cwd
    }
    return s.cwd
}

func (s *BashState) MaybeUpdateCwd(command string) {
    // 简版：只识别 "cd /abs/path" 形式
    // 进阶：需要真正解析 shell command（用 mvdan.cc/sh/v3/syntax）
    parts := strings.Fields(strings.TrimSpace(command))
    if len(parts) >= 2 && parts[0] == "cd" && filepath.IsAbs(parts[1]) {
        s.mu.Lock()
        s.cwd = parts[1]
        s.mu.Unlock()
    }
}
```

per-conversation 一份 BashState，挂在 chat.Service 上。**不真起 shell 进程**——每次 `bash -c "cd <cwd> && <cmd>"` 新进程。env 自然不持久（跟 CC 行为一致）。

#### 6.3 沙箱：**Forgify v1 不做 OS-level sandbox**

**理由**：
- 单用户本地桌面——威胁模型低
- macOS `sandbox-exec` deprecated 多年
- Linux `bwrap` 不通用，`firejail` 也是
- Windows 没等价物
- 追沙箱 ≈ 1 周以上工作 + 跨平台维护负担

**替代方案**：
- workspace whitelist（Phase 5）—— `cwd` 必须在白名单内
- destructive command warning —— Tool description 把 git destructive ops + `rm -rf /` 等模式列入禁用清单（让 LLM 自己规避）
- permission mode —— `CheckPermissions` 检测 `rm -rf` / `mkfs` / `dd if=/dev/zero of=/dev/sda` 等明显灾难命令 → 返 `Ask`，让用户手批
- Description 里复刻 sleep 哲学 + tool 偏好提示（git safety 不抄，靠 isDangerousCommand 兜底）

后续真要 sandbox，可考虑：
- macOS：`sandbox-exec -p '<profile>' /bin/bash -c '<cmd>'`
- Linux：`bwrap --ro-bind / / --bind <workspace> <workspace> --share-net /bin/bash -c '<cmd>'`

#### 6.4 Timeout 处理

```go
const (
    DefaultTimeoutMS = 120_000  // 2 min
    MaxTimeoutMS     = 600_000  // 10 min
)

func (t *Bash) Execute(ctx context.Context, argsJSON string) (string, error) {
    var args struct {
        Command           string `json:"command"`
        TimeoutMS         int    `json:"timeout"`
        Description       string `json:"description"`
        RunInBackground   bool   `json:"run_in_background"`
    }
    if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
        return "", fmt.Errorf("Bash.Execute: %w", err)
    }
    if args.TimeoutMS <= 0      { args.TimeoutMS = DefaultTimeoutMS }
    if args.TimeoutMS > MaxTimeoutMS { args.TimeoutMS = MaxTimeoutMS }

    if args.RunInBackground {
        return t.executeBackground(ctx, args)
    }
    return t.executeSync(ctx, args)
}

func (t *Bash) executeSync(ctx context.Context, args /*…*/) (string, error) {
    cwd := t.state.Cwd()
    if !t.workspace.Allowed(cwd) {
        return fmt.Sprintf("cwd outside allowed workspace: %s", cwd), nil
    }

    // permission gate (灾难命令)
    if isDangerousCommand(args.Command) {
        return "Refused: command matches destructive pattern. Ask user to confirm via UI.", nil
    }

    timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(args.TimeoutMS)*time.Millisecond)
    defer cancel()

    full := fmt.Sprintf("cd %s && %s", shellQuote(cwd), args.Command)
    cmd := exec.CommandContext(timeoutCtx, "sh", "-c", full)
    cmd.Env = os.Environ()  // 继承 backend 进程 env，但不传到下条命令
    out, err := cmd.CombinedOutput()

    t.state.MaybeUpdateCwd(args.Command)

    output := truncateOutput(string(out), 30_000)  // 30K 字符截断（CC 同款）
    if errors.Is(timeoutCtx.Err(), context.DeadlineExceeded) {
        return output + "\n\n[command timed out after " + strconv.Itoa(args.TimeoutMS) + "ms]", nil
    }
    if err != nil {
        var exitErr *exec.ExitError
        if errors.As(err, &exitErr) {
            return output + fmt.Sprintf("\n\n[exit code: %d]", exitErr.ExitCode()), nil
        }
        return "", fmt.Errorf("Bash.executeSync: %w", err)
    }
    return output, nil
}

func truncateOutput(s string, max int) string {
    if len(s) <= max { return s }
    return s[:max] + fmt.Sprintf("\n... [truncated to %d chars]", max)
}

func shellQuote(s string) string {
    return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func isDangerousCommand(cmd string) bool {
    cmd = strings.TrimSpace(cmd)
    patterns := []string{
        "rm -rf /", "rm -rf /*", "rm -rf ~", "rm -rf $HOME",
        "mkfs", "dd if=/dev/zero", "dd if=/dev/random",
        ":(){ :|:& };:",  // fork bomb
        "git push --force origin main", "git push --force origin master",
        "git reset --hard HEAD",
    }
    lc := strings.ToLower(cmd)
    for _, p := range patterns {
        if strings.Contains(lc, strings.ToLower(p)) { return true }
    }
    return false
}
```

#### 6.5 Background 模式

```go
type BackgroundStore interface {
    Start(ctx context.Context, cmd, cwd string) (taskID string, err error)
    GetOutputPath(taskID string) string
    Stop(taskID string) error
}

func (t *Bash) executeBackground(ctx context.Context, args /*…*/) (string, error) {
    cwd := t.state.Cwd()
    taskID, err := t.bgStore.Start(ctx, args.Command, cwd)
    if err != nil {
        return "", fmt.Errorf("Bash.executeBackground: %w", err)
    }
    return fmt.Sprintf("Background task started.\nTask ID: %s\nOutput file: %s\n\nUse Read on the output file to retrieve output.",
        taskID, t.bgStore.GetOutputPath(taskID)), nil
}
```

`BackgroundStore` 实现：
- 起 goroutine + `os.StartProcess`（不绑 ctx，独立 lifetime）
- stdout/stderr 重定向到 `$TMPDIR/forgify-bg-<taskID>.log`
- session 结束时清理 + 杀掉未结束的 background 进程
- 通过现有 SeenFiles 机制让 LLM 直接 Read 这个 log 文件

#### 6.6 Description 复刻策略

Description 里**必抄**的子段：
- ✅ overview
- ✅ working-directory
- ✅ maintain-cwd
- ✅ verify-parent-directory
- ✅ quote-file-paths
- ✅ timeout
- ✅ parallel-commands / sequential-commands / semicolon-usage / no-newlines
- ✅ sleep 4 条（agent behavior 教导）
- ✅ prefer-dedicated-tools + alternative（指 LLM 用 Read/Write/Edit/Glob/Grep）

**不抄**：
- ❌ 17 个 sandbox 子段（Forgify 不做 OS sandbox）
- ❌ git 4 条 + commit safety + PR 创建指南整段（**全不抄**——靠 LLM 训练自带 git 常识，灾难命令由 `isDangerousCommand` pattern match 兜底）

合计 description ~30 行 / ~900 字。

#### 6.7 Validate / Permission 错误码

| Sentinel | 触发 |
|---|---|
| `ErrEmptyCommand` | command == "" |
| `ErrTimeoutOutOfRange` | timeout < 0 或 > 600000 |
| `ErrCwdOutsideWorkspace` | Phase 5 |
| `ErrDangerousCommand` | matched destructive pattern (返 Ask 而非 Deny) |

#### 6.8 测试要点

- 普通命令（`ls /tmp`）成功
- cwd 持久（连续两次 `pwd` cwd 不变）
- env 不持久（`export X=1; ` 单 command 内有效，跨 command 失效）
- timeout 触发（`sleep 5` + timeout=100ms）
- exit code 非零（`false`）— 不报 Go error，输出含 `[exit code: 1]`
- background 模式（`echo hi & sleep 1`）— 立即返 task ID，后续 Read 拿 output
- 30K 字符截断
- destructive command（`rm -rf /`）→ Refused
- workspace 外的 cwd（v5）→ ErrCwdOutsideWorkspace
- 路径含空格（`ls "/tmp/has space"`）→ 成功
- 跨平台：linux + macOS 一致行为；windows 暂不支持（v1 用 sh, Windows fallback git bash）

### 7. 已知 bugs / edge cases

- **#2508 / #20503**: env vars 不持久——CC 自己也是。Forgify 同款行为，文档说明
- **复合命令权限旁路** (CC v2.1.98 修)：`echo hi; rm -rf /` 这类用 `;` 串接的命令早期版本 permission 检测只看第一段。Forgify 简版 destructive 检测是 substring match，能扫到整个 command 字符串
- **`/dev/tcp` 重定向**: CC v2.1.113 加了警惕，因为可以"用 Bash 起 TCP 连接"。Forgify v1 不防（无沙箱），文档警告

### 8. 输出格式给 LLM

```
<合并的 stdout + stderr>

[exit code: 0]
```

或截断：
```
<前 30000 字符>
... [truncated to 30000 chars]
[exit code: 0]
```

或超时：
```
<已捕获的部分输出>

[command timed out after 120000ms]
```

或 background：
```
Background task started.
Task ID: bg_a3f4...
Output file: /tmp/forgify-bg-a3f4....log

Use Read on the output file to retrieve output.
```

---

## Monitor（P2，仅设计走查）

### 1. Description 原文（Piebald v2.1.119）

> Start a background monitor that streams events from a long-running script. Each stdout line is an event — you keep working and notifications arrive in the chat. Events arrive on their own schedule and are not replies from the user, even if one lands while you're waiting for the user to answer a question.
>
> Pick by how many notifications you need:
> - **One** ("tell me when the server is ready / the build finishes") → use **Bash with `run_in_background`** and a command that exits when the condition is true, e.g. `until grep -q "Ready in" dev.log; do sleep 0.5; done`. You get a single completion notification when it exits.
> - **One per occurrence, indefinitely** ("tell me every time an ERROR line appears") → Monitor with an unbounded command (`tail -f`, `inotifywait -m`, `while true`).
> - **One per occurrence, until a known end** ("emit each CI step result, stop when the run completes") → Monitor with a command that emits lines and then exits.

（剩余约 100 行：脚本质量约束、coverage 约束、output volume 约束、persistent flag、exit 行为——内容高度浓缩，全文见 Piebald 原文）

### 2. JSON Schema（推断）

```json
{
  "type": "object",
  "required": ["command", "description"],
  "properties": {
    "command": {"type": "string", "description": "The watch script. stdout lines become events."},
    "description": {"type": "string", "description": "What is being watched (specific, e.g. 'errors in deploy.log')"},
    "persistent": {"type": "boolean", "default": false, "description": "Run for entire session (vs. exit on script completion)"}
  }
}
```

### 3. 行为要点

- 起子进程，stdout 逐行 → 每行变一条 chat notification
- stderr 不触发 notification，但写到 output file（用 `2>&1` merge 进 stdout 才能命中）
- **200ms 内的 stdout 行批合成一条**（多行单事件的天然分组）
- exit 自然停；timeout 杀；`TaskStop` 取消
- **持久标志 persistent**：true 时跑整个 session（如 PR 监控、log tail），false 时 script 退出就停

### 4. 三类典型用法

| 场景 | 工具选择 |
|---|---|
| 单次完成通知（"build 好了告诉我"） | Bash `run_in_background` + `until` 循环 |
| 持续事件流（"每次 ERROR 都告诉我"） | Monitor + `tail -f` |
| 有终态的事件流（"每个 CI step 结果"） | Monitor + 自定 polling 脚本，最后 break |

### 5. Forgify 决策

**P2，v1 不做**。理由：
- Forgify Phase 5 范围内还没有"长跑监听"用例（forge 跑都是 short-burst Python）
- Bash + `run_in_background` 已够覆盖"一次性等完成"
- Monitor 的事件流推送需要 SSE 改造（每行一个新 chat event 进对话历史）—— 与 chat.message snapshot 模型不直接兼容
- 等 Phase 4+ 工作流出来后，"workflow run 进度推送"是更值得做的功能，那时 Monitor 可作为子能力

如未来要做：
- SSE 引入新事件类型 `monitor.event`（每行一条）
- 让 background task 的 stdout 触发 publish
- chat 历史里以"system reminder"形式插入这些 event

---

## 跨 tool 共享

- `BashState` 跟 `AgentState.SeenFiles`（§01 file-ops 设计）平行，per-conversation 一份
- 都 Phase 5 RequiresWorkspace=true
- Bash 的 `description` 字段（5-10 字摘要）跟 framework 注入的 `summary` 字段在 SSE 中可以二者择一展示——Forgify 已经做 `summary` 了，所以 Bash 的 description 字段保留但权重降级

---

## 总结：本批实施估时

| 工具 | 估时 | 难点 |
|---|---|---|
| Bash | 1.5 天 | cwd 状态机 + dangerous command 检测 + background 子系统 + 跨平台（unix vs windows） |
| Monitor | 0 天（不做） | — |

**合计 1.5 天**——是 file-ops + search 加起来还多的工作量。

---

## 信任度总结

- ✅ **多源确认**：Bash schema 4 字段（command/timeout/description/run_in_background）/ cwd 持久 env 不持久 / 30K 输出截断 / 120s 默认 600s 上限 / 41 子描述全文（直接 fetch Piebald）/ Monitor description 原文 (v2.1.119)
- ⚠️ **单源 / 推测**：CC 内部 cwd 实现机制（`cd <cwd> && <cmd>` 拼接 vs 真持久 shell 进程）/ destructive command 实际触发的 permission policy 列表 / sandbox-exec 在 macOS 的具体 profile / linux sandbox 实现
- ❌ **无法验证**：CC source 中 dangerouslyDisableSandbox 字段当前是否仍在 schema / Monitor 200ms 批合成的精确算法 / persistent task 的 session 边界定义

deep-dive 期间补强 ⚠️ 项；实施时按 ✅ 项落地，⚠️ 项保留扩展位。

---

## 框架级变更：execution_group（来自 deep-dive 复审决策 D6）

> **背景**：原计划 Bash 用 `IsConcurrencySafe(args)` 让 server 端反推（参考 CC）。复审中改为 **LLM 自报 `execution_group: int` 字段**，框架按 group 分批。这是个**跨 tool 的框架级变更**，本节集中说明。

### 设计

新增第三个标准注入字段（与 `summary` / `destructive` 同款机制，所有 tool schema 自动加）：

```json
"execution_group": {
  "type": "integer",
  "minimum": 0,
  "description": "Execution batch identifier for this turn. Tool calls with the same execution_group run in parallel; different groups run sequentially in ascending order. Set the same number on calls that have NO interdependence and NO shared mutable state — typical example: parallel `git status` + `git diff`. If unsure, give each call its own unique number (sequential is always safe)."
}
```

### 调度语义

LLM 同 turn 发 5 个 tool call，标了 group：
```
[A:0, B:0, C:0, D:1, E:1]
→ 批 1: [A, B, C] 并行
→ 批 2: [D, E] 并行
```

更复杂 DAG：
```
[A:0, B:0, C:1, D:2, E:2]
→ 批 1: [A, B] 并行
→ 批 2: [C] 单跑
→ 批 3: [D, E] 并行
```

### 兜底机制

| 兜底 | 何时触发 | 行为 |
|---|---|---|
| **缺省值 = call index** | LLM 没填 `execution_group` | 每个 call 自动分配唯一 group → **退化为完全串行**（最安全） |
| **批内并发上限** | 单个 group 包含 >10 个 call | 截到 10，多余的强制串行（防 LLM 写出 100 并发） |
| **destructive 警示日志** | 同一 group 含 `destructive=true` 的 call | log warn + 仍按 LLM 意图并发（信任 LLM 但留痕） |

### 实现影响（跨多个文件）

| 文件 | 改动 |
|---|---|
| `internal/app/tool/tool.go::injectStandardFields` | 加注入第三个字段 `execution_group` |
| `internal/app/tool/tool.go::StripStandardFields` | 提取 `execution_group`，签名改返 struct（避免 4-tuple） |
| `internal/app/tool/tool.go::Tool` 接口 | **删 `IsConcurrencySafe(args)` 方法**——所有 tool 实现一并删（5 个 forge tool + 未来的 file-ops/search/shell/web/ux-tasks） |
| `internal/domain/chat/chat.go::ToolCallData` | 加 `ExecutionGroup int` 字段 |
| `internal/app/chat/stream.go::parseToolArgs` | 提取 `execution_group` 写入 `ToolCallData` |
| `internal/app/chat/tools.go::partitionByConcurrencySafety` | 重写为 `partitionByExecutionGroup`：bucket by group → 按 group 升序输出 batches |
| `internal/app/chat/tools.go::concurrencyBatch` | 简化为 `executionBatch`（不再有 safe bool 字段，只有 items；同 group 内全并行） |

### 兼容性

- 现存 5 个 forge tool 都返 `IsConcurrencySafe = IsReadOnly()`——改造后**全部删该方法**，并发行为完全靠 LLM 自报
- 旧 chat 历史中没有 `execution_group` 字段 → parseToolArgs 兜底自增（原回放行为不变）

### 文档同步

CLAUDE.md S18 §2「标准注入字段」节需要从"2 个字段"改为"3 个字段"。本节作为该改动的设计参考；CLAUDE.md 的实际更新等实施落地时一并做（避免规范与代码漂移）。

### 估时

加这一项框架改造：**~0.3 天**（接口+几个调用点的小重构 + 测试）。已加到 Bash 估时里（1.5 天 → 1.8 天）；其他 file-ops/search/web/ux-tasks tool 的实施天数不受影响（少实现一个 IsConcurrencySafe 方法 ≈ 抵消）。
