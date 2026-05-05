# Shell Tools — V1.2 详设计

**Phase**：5（System Tool 第二代 shell 批次）
**状态**：✅ 实现完成（2026-05-04，B1-B2）
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../../../CLAUDE.md`](../../../CLAUDE.md) §S18 — Tool 接口规约
- [`./chat.md`](./chat.md) §4.4 — 系统工具完整目录
- [`./task.md`](./task.md) §10 — AgentState 跨 tool 共享状态生命周期
- 实现包：`backend/internal/app/tool/shell/`（5 文件：shell.go / manager.go / bash.go / output.go / kill.go）

---

## 1. 一句话

LLM 跑 shell 命令的三件套：**Bash**（前后台双模式 + cd 状态机）/ **BashOutput**（轮询后台进程新输出）/ **KillShell**（终止后台进程）。**故意**不带 banned-command 列表——单用户本地场景下 Bash 是用户日常命令的代理，banned-list 没意义。后台进程子系统：`ProcessManager` 注册表 + 256 KB 环形输出缓冲 + 读游标。

---

## 2. 端到端推演（设计原则 #5）

### Bash 前台路径

```
触发源：LLM 调 Bash(command, [timeout])
  → ValidateInput: command 非空 / timeout ∈ [0, 600000]
  → Execute:
      cmdText := strings.TrimSpace(args.Command)
      target, ok := parseCDOnly(cmdText)
      if ok → handleCD(target):
          路径解析（绝对 / 相对 cwd）+ Stat 验证目录
          state.SetCwd(target)         // 更新 AgentState.Cwd
          → "Changed working directory to <abs path>"
      else:
          cwd = resolveCwd(ctx)
          if Background → runBackground
          else → runForeground:
              context.WithTimeout(ctx, timeout) → exec.CommandContext("/bin/sh", "-c", cmd)
              cmd.Stdout = cmd.Stderr = &buf  // 合并捕获
              cmd.Run() 阻塞
              switch runCtx.Err():
                DeadlineExceeded  → "[command timed out after Xs]"
                Canceled          → "[cancelled]"  (batch 1 修的 UX bug)
                err is *ExitError → "[exit code: N]"
                err other         → "[exec failed: <msg>]"
              capOutput(buf, 256 KB)
  → tool_result：正文 + 状态尾注
```

### Bash 后台路径

```
LLM 调 Bash(command, run_in_background=true)
  → runBackground(ctx, cmdText, cwd):
      cmd := exec.CommandContext(context.Background(), ...)  // 故意 Background ctx
      stdout/stderr Pipe + cmd.Start
      proc := &BgProcess{ID=bsh_<16hex>, ConvID, Command, Cmd, StartedAt, status=Running}
      mgr.Register(proc)
      go pumpReader(stdout) → proc.appendOutput
      go pumpReader(stderr) → proc.appendOutput
      go reaper:
          pumpWG.Wait()
          err := cmd.Wait()
          → markFinished(StatusExited|Killed) / markErrored
  → tool_result：「Started background command (bash_id=bsh_xxx): <cmd>\n... 」
```

### BashOutput 路径

```
LLM 调 BashOutput(bash_id, [filter])
  → ValidateInput: bash_id 非空 / filter regex 合法
  → Execute:
      proc := mgr.Get(id) → ErrProcessNotFound 时返友好字符串
      newBytes, dropped, status, exitCode := proc.drainNew()  // 推进读游标
      filter regex 应用（可选）
      formatOutputResult: 正文 + ring overflow 提示 + 状态尾注
  → tool_result
```

### KillShell 路径

```
LLM 调 KillShell(shell_id)
  → ValidateInput: shell_id 非空
  → Execute:
      proc := mgr.Get(id) → 不存在时返友好字符串（**幂等**）
      proc.Cmd.Process.Kill()  // SIGKILL；尝试杀，失败也不报错
      mgr.Remove(id)
      → "Killed background shell <id>." / "Background shell <id> already finished; removed from registry."
```

**端到端跨 domain 依赖**：
- `pkg/agentstate.AgentState.Cwd` — Bash cd 状态机的存储；通过 `pkg/reqctx.WithAgentState` ctx 注入
- `pkg/reqctx.GetConversationID` — 后台进程注册时记 ConvID（仅信息性，无过滤逻辑）
- `pkg/idgen.New("bsh")` — 后台进程 ID 生成
- `os/exec` + `os.Kill` — Unix 系统调用
- 无 DB / Service / SSE / HTTP API
- **故意不依赖** PathGuard（Bash 的安全 trade-off，详 §6）

---

## 3. 关键决策

| 决策 | 选择 | 理由 |
|---|---|---|
| Banned-command 列表 | **不实现** | 本地单用户：Bash 是"用户本来就会在终端敲的命令"的代理；banned-list 无价值（`bash cat ~/.ssh/id_rsa` 能成功，挡住反而误伤合理 cmd）|
| PathGuard 集成 | **不集成**（RequiresWorkspace=false）| 同上 trade-off。PathGuard 是 file-tool 的护栏，不是安全边界（详 pathguard.go 包注释）|
| Shell 选择 | `/bin/sh -c "<cmd>"` Unix；`cmd /c` Windows（best-effort）| Unix 标准；Windows 非本期 v1 范围但 best-effort 不阻止 |
| cd 状态机 | **整命令 `cd <path>`** 短路更新 AgentState.Cwd；链式 `cd && other` **不追踪** | 链式跟"子 shell exit 后父 cwd 不变"语义一致；短路简化实现避免写 mini-parser |
| 前台 timeout | 默认 120s，硬上限 600s | 覆盖典型 build / test / install；真长跑走后台 |
| 后台 ctx | `context.Background()`（**不用** request ctx）| 让 conversation cancel 不杀后台子进程（按设计：bg 命令 outlive turn）|
| 后台 timeout | **无** | 用户主动 KillShell 或 backend shutdown 时 mgr.Stop() 杀 |
| 输出 cap | 前台 256 KB（截头）/ 后台环形 256 KB（环绕，丢头）| LLM context 不被失控命令撑爆；后台允许长流但有界 |
| Cancel UX | 父 ctx Canceled 时报 `[cancelled]` 而不是 `[exec failed: signal: killed]` | **Tool 自检 batch 1 修的 UX bug**——LLM 看到 "exec failed" 会错以为命令本身崩 |
| Cd 引号剥除 | 一对包裹 `"` 或 `'` 剥；不剥不平衡引号 | 让 `cd "/tmp/with space"` 顺手；不试图当完整 shell parser |
| 进程组 / SIGTERM 优雅 | **v1 仅 SIGKILL** | 原生 `cmd.Process.Kill()`；进程组 + SIGTERM-then-SIGKILL 是后续优化点 |
| Banned cmd metadata | IsReadOnly=false（Bash） / true（BashOutput） / false（KillShell） | 文档性，框架不强制（同 §S18 §8 对照表）|

---

## 4. 工具规约

### 4.1 Bash（`backend/internal/app/tool/shell/bash.go`）

**Args**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `command` | string | ✅ | shell 命令（POSIX sh）|
| `description` | string | | 一行人类可读描述（UI / log）|
| `run_in_background` | bool | | 默认 false；true 时立即返 bash_id 不等待 |
| `timeout` | number | | 前台超时（毫秒）；默认 120000；硬上限 600000 |

**返回**（前台）：合并 stdout+stderr 正文 + 空行 + 可选 [note] + `[exit code: N]` 尾注。

**返回**（后台）：`Started background command (bash_id=bsh_xxx): <cmd>\nUse BashOutput with this bash_id to poll new output, or KillShell to terminate.`

**返回**（cd）：`Changed working directory to <abs>` / `cd: <error>`

**前台 footer 示例**：
- 正常：`[exit code: 0]`
- 超时：`[command timed out after 200ms]\n[exit code: -1]`
- **取消**：`[cancelled]\n[exit code: -1]` ← batch 1 修
- 非零退出：`[exit code: 7]`
- 启动失败：`[exec failed: fork/exec /bin/sh: ...]\n[exit code: -1]`

**静态元数据**：`IsReadOnly=false` / `NeedsReadFirst=false` / `RequiresWorkspace=false`（**故意**——见 §6）

**ValidateInput** sentinels：
- `ErrEmptyCommand` — command 缺 / 空 / 仅空白
- `ErrInvalidTimeout` — timeout < 0 或 > maxTimeoutMS

### 4.2 BashOutput（`output.go`）

**Args**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `bash_id` | string | ✅ | Bash run_in_background:true 返的 ID |
| `filter` | string | | 正则；保留匹配行 |

**返回**：自上次轮询新增字节 + 状态尾注。

**返回**（特殊）：
- 不存在 → `Background shell process not found: <id>`（**不报错，幂等**）
- 无新增 → `(no new output since last poll)\n\n[status: running]`
- 环形溢出 → 正文头加 `[note: N bytes dropped from buffer head before this poll due to ring overflow]`

**状态尾注**：`[status: running]` / `[status: exited (code N)]` / `[status: killed]` / `[status: errored]` / `[status: unknown]`

**静态元数据**：`IsReadOnly=true` / `NeedsReadFirst=false` / `RequiresWorkspace=false`

**ValidateInput** sentinels：
- `ErrEmptyBashID` — bash_id 缺 / 空 / 仅空白
- filter regex 编译错 → `errors.New("BashOutput.ValidateInput: filter regex: <err>")`

### 4.3 KillShell（`kill.go`）

**Args**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `shell_id` | string | ✅ | bash_id |

**返回**：
- 存在且 running → `Killed background shell <id>.`
- 存在但已 finished → `Background shell <id> already finished; removed from registry.`
- 不存在 → `Background shell process not found: <id>`（**幂等**：杀两次返同结果）

**静态元数据**：`IsReadOnly=false` / `NeedsReadFirst=false` / `RequiresWorkspace=false`

**ValidateInput**：仅 shell_id 非空（无独立 sentinel）。

### 4.4 ShellTools 工厂

```go
// app/tool/shell/shell.go
type ShellTools struct {
    Manager *ProcessManager
    Tools   []toolapp.Tool
}

func NewShellTools() *ShellTools {
    mgr := NewProcessManager()
    return &ShellTools{
        Manager: mgr,  // 暴露给 main.go 用 defer mgr.Stop() 优雅关停
        Tools:   []toolapp.Tool{&Bash{mgr}, &BashOutput{mgr}, &KillShell{mgr}},
    }
}
```

调用方按 §S13 嵌套子包别名规则导入为 `shelltool`。

**main.go 装配**：

```go
shells := shelltool.NewShellTools()
defer shells.Manager.Stop()  // SIGKILL 所有 running 子进程
tools = append(tools, shells.Tools...)
```

---

## 5. 实现要点

### 5.1 cd 状态机（`parseCDOnly` + `handleCD`）

```go
func parseCDOnly(cmd string) (target string, ok bool) {
    trimmed := strings.TrimSpace(cmd)
    if trimmed == "cd"           { return "", true }       // cd 单字 → home
    if !strings.HasPrefix(trimmed, "cd ") &&
       !strings.HasPrefix(trimmed, "cd\t") { return "", false }
    rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "cd"))
    // 任何 shell 元字符（&|;<>$`）都说明这是链式 — 不当 cd-only 处理
    if strings.ContainsAny(rest, "&|;<>`$") { return "", false }
    // 一对包裹引号剥（手动写 mini-parser 太重）
    if len(rest) >= 2 && (rest[0] == '"' || rest[0] == '\'') &&
       rest[0] == rest[len(rest)-1] {
        rest = rest[1 : len(rest)-1]
    }
    return rest, true
}
```

**handleCD**：
- target 空 → `os.UserHomeDir()`
- 非绝对 → `filepath.Join(currentCwd, target)`
- `filepath.Clean`
- `os.Stat` 校验 `info.IsDir()`
- `state.SetCwd(target)` ← AgentState 缺失时**软错**（让 LLM 看到 wiring bug 警告但不致命）

**链式 `cd /tmp && ls`** 进入 `runForeground` 走 `/bin/sh -c "cd /tmp && ls"`——子 shell 内 cd 生效，pwd 输出 /tmp，但 AgentState.Cwd **不**变（与子 shell 退出后父 cwd 不变语义一致；这是有意决策）。

### 5.2 ProcessManager（`manager.go`）

**单一 mutex 守注册表**：

```go
type ProcessManager struct {
    mu    sync.Mutex
    procs map[string]*BgProcess
}
```

**每个 BgProcess 自带 mutex** 守 output buffer + 读游标：

```go
type BgProcess struct {
    ID, ConvID, Command string
    Cmd        *exec.Cmd
    StartedAt  time.Time

    mu         sync.Mutex
    buf        []byte    // 环形缓冲（cap bgBufferBytes = 256 KB）
    dropped    int64     // 累计被丢字节（informational）
    readCursor int       // BashOutput 已 drain 的位置
    status     Status
    exitCode   int
    finishedAt time.Time
    launchErr  error
}
```

**两层锁设计**：mgr.mu 只在 Register/Get/Remove/Stop 时短时间持有；BgProcess.mu 在 appendOutput / drainNew / markFinished 时持有。**并发 BashOutput 轮询不互相阻塞**（不同进程不同锁）。

### 5.3 环形输出缓冲（`appendOutput` / `drainNew`）

```go
func (p *BgProcess) appendOutput(b []byte) {
    p.mu.Lock(); defer p.mu.Unlock()
    p.buf = append(p.buf, b...)
    if len(p.buf) <= bgBufferBytes { return }
    overflow := len(p.buf) - bgBufferBytes
    p.dropped += int64(overflow)
    p.buf = p.buf[overflow:]      // 丢头
    p.readCursor -= overflow      // 游标向前贴
    if p.readCursor < 0 { p.readCursor = 0 }
}

func (p *BgProcess) drainNew() (newBytes []byte, dropped int64, status Status, exitCode int) {
    p.mu.Lock(); defer p.mu.Unlock()
    out := append([]byte(nil), p.buf[p.readCursor:]...)  // 拷贝（让调用方下游 regex 不持锁）
    p.readCursor = len(p.buf)
    return out, p.dropped, p.status, p.exitCode
}
```

**关键**：环形溢出时 `readCursor` 按比例回退——原本指在被丢区域的游标贴齐到剩余缓冲头（保证 BashOutput 看到的"新字节"流不会跳过 / 重复）。

### 5.4 Reaper goroutine（防 zombie）

```go
go func() {
    pumpWG.Wait()           // 两个 pumpReader（stdout + stderr）排空
    err := cmd.Wait()       // 真正 reap zombie + 拿 exit code
    switch {
    case err == nil:
        proc.markFinished(StatusExited, 0)
    case errors.As(err, &exitErr):
        if exitErr.ProcessState != nil && exitErr.ProcessState.Exited() {
            proc.markFinished(StatusExited, exitErr.ExitCode())
        } else {
            proc.markFinished(StatusKilled, -1)  // 信号杀
        }
    default:
        proc.markErrored(err)
    }
}()
```

**先 pumpWG.Wait 再 cmd.Wait**：保证 pipe 排空后才 reap，否则可能丢尾部输出。

### 5.5 graceful shutdown（`Manager.Stop`）

```go
func (m *ProcessManager) Stop() {
    m.mu.Lock()
    procs := make([]*BgProcess, 0, len(m.procs))
    for _, p := range m.procs { procs = append(procs, p) }
    m.mu.Unlock()
    for _, p := range procs {
        if p.Cmd != nil && p.Cmd.Process != nil {
            _ = p.Cmd.Process.Kill()  // best-effort SIGKILL
        }
    }
}
```

main.go 用 `defer shells.Manager.Stop()` 在 backend 关停时杀掉所有 running 后台子。**Best-effort**——失败 OS 会 reap 孤儿。

---

## 6. 安全边界

| 防线 | 覆盖 | 局限 |
|---|---|---|
| **故意不带 banned-command list** | n/a — 设计决策 | LLM 能跑用户身份允许的任何命令；包括 `cat ~/.ssh/id_rsa` |
| **不走 PathGuard** | n/a — 一致 trade-off | 见 pathguard.go 包注释 |
| **前台默认 / 硬上限 timeout** | 防失控命令卡 ReAct 循环 | 后台无 timeout（设计如此）|
| **256 KB 输出 cap**（前台） | 防输出灌爆 LLM context | 截头保留尾（最近的输出更可能是错误信息）|
| **256 KB 环形**（后台） | 防长跑命令耗尽内存 | 早期输出会丢；BashOutput 报 dropped 字节 |
| **cd 状态机 in-process**（不改进程 cwd） | 多对话 / 多 worker 不互相影响 | LLM 跑 `cd ... && other` 不更新 AgentState.Cwd（与子 shell 语义一致，但可能让 LLM 困惑——文档已说明）|
| **后台 ctx = Background**（不被 turn cancel 杀）| 让 build / test / dev server outlive 单次 turn | 长跑命令可能跨多个 turn 直到用户 KillShell；进程持续直到 backend shutdown |
| **idgen.New("bsh")** | 64 bit 熵防 ID 碰撞 | 在 mgr 内部唯一，跨 backend 重启 ID 重算（注册表清空也无所谓）|

---

## 7. 测试覆盖

| 层 | 文件 | 测试数 | 覆盖 |
|---|---|---|---|
| Bash | `backend/internal/app/tool/shell/bash_test.go` | 14 | identity / 静态 metadata / schema / Validate × 3 / parseCDOnly × 12 case 矩阵 / 整命令 cd 更新 AgentState / cd 拒不存在 / cd 拒文件 / cd 相对路径解析 / 前台 echo+exit code / 非零 exit / stderr 合并 / **timeout** / cwd 应用 / **父 ctx cancel 报 [cancelled]**（batch 1 加固）/ background 返 bash_id / background 输出捕获 / resolveCwd 优先级 / capOutput 边界 |
| BashOutput | `output_test.go` | 9 | identity / 静态 metadata / schema / Validate × 3 / unknown ID 友好 / 新字节 + 游标推进 / filter / 状态 footer × 2 / filterLines empty |
| KillShell | `kill_test.go` | 6 | identity / 静态 metadata / schema / Validate / unknown 幂等 / 杀 running + reaper wait / 杀已结束 / 双调用幂等 |
| ProcessManager | `manager_test.go` | 8 | Register 派 ID + 前缀 / Get unknown / Register-Get-Remove / appendOutput 不丢 / 环形溢出丢头 / drainNew 推进 / drain 溢出贴齐 / markFinished / Stop 空 + 多次 |
| Pipeline | `backend/test/shell/` | 3 场景 | LLM ↔ tool 端到端：BashEchoForeground / CdStateMachinePersistsAcrossCalls / BashOutputAndKillShellHandleUnknownID（19s）|

合计 **37 单测 + 3 pipeline 场景**。

---

## 8. 与其他 domain 的关系

| 关系 | 说明 |
|---|---|
| **agentstate** | Bash 读写 AgentState.Cwd；BashOutput / KillShell 不读 |
| **chat** | chat/runner.go::processTask 注入 AgentState；ProcessManager 是独立长生命周期对象，main.go 装一次 |
| **filesystem / search** | **不共享 PathGuard**（Bash 故意 RequiresWorkspace=false）—— 设计 trade-off |
| **forge** | 无直接耦合；但 LLM 通过 Bash 跑 `git` / `make` 等命令，某种意义补充 forge 沙箱跑用户函数 |
| **events / SSE** | 无 — 输出通过 chat.message tool_result block 推流 |
| **errmap** | 无登记 — 错误以友好字符串返 LLM |

---

## 9. 演化方向

- **进程组管理 + SIGTERM-then-SIGKILL 优雅终止**：当前 SIGKILL 单步杀；未来给子 shell 起 process group + 先 SIGTERM 等几秒再 SIGKILL，让 server 类命令有机会 cleanup
- **stderr 分流（不与 stdout 合并）**：当前合并捕获；未来可分开返 / 提供选项
- **Windows shell 适配**：当前 best-effort 走 cmd.exe；未来若桌面端发 Windows binary，正式适配 PowerShell（quoting / 命令行长度限制 / 进程模型差异）
- **交互式命令**：当前不支持 stdin（vim / less / 等）—— 未来加 `:run-interactive` 路径走 PTY
- **Per-conversation 后台进程清理**：当前 ProcessManager 全局；conversation 删除 / 5min idle GC 时**不**自动杀 bg child（设计如此，bg outlive turn）；未来若需要可加 hook
- **Resource 限制**（cgroup / rlimit）：当前 OS 无限制；未来若多用户场景再加
