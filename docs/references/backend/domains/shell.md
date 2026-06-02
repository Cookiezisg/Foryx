---
id: DOC-121
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Shell Domain — 宿主机交互与 Bash 自动重定向原理

> **核心职责**：Shell 域是 Forgify 赋予 Agent 的 **“物理之手”**。它允许 LLM 通过 Bash 命令操作宿主机。为了安全，系统实现了一套精密的自动重定向机制，将所有的 Python/Node 操作“偷梁换柱”到沙箱环境中。

---

## 1. 核心原理 (Principles)

### 1.1 Command Interception (指令劫持)
这是 Shell 域最核心的技术细节。后端通过正则和轻量级词法分析，对 LLM 输入的 Bash 字符串进行预处理：
- **场景**：LLM 输入 `pip install pandas`。
- **劫持**：后端识别到 `pip` 关键词，自动查找当前对话关联的 `se_xxx` 沙箱。
- **改写**：物理执行的命令变为 `/Users/.../se_xxx/bin/python -m pip install pandas`。
- **效果**：LLM 感觉自己在操作全局环境，实则所有副作用都困在沙箱内。

### 1.2 Interactive-to-Batch (交互转批处理)
由于 LLM 生成是单向的，无法处理交互式 Shell (如 `vim`, `less`, `sudo` 提问)：
- **原理**：系统强制将子进程的 `stdin` 关闭或重定向到 `/dev/null`。
- **后果**：任何需要用户输入的命令都会立即以“Input required”错误结束。
- **例外**：未来计划通过 PTY 实现交互模式，当前 V1.2 为 **纯批处理 (Non-interactive)**。

### 1.3 Wall-clock Backstop (超时强杀)
- **物理阈值**：所有 `run_bash` 指令强制带有一个 **30 秒** 的 `context.WithTimeout`。
- **防止僵尸**：即便子进程内部陷入死循环，后端协程超时后会发送 `SIGKILL -PGID` 强杀整个进程组。

---

## 2. 生命周期 (Lifecycle)

1. **发起 (Emitting)**：LLM 发起 `run_bash(cmd)`。
2. **分析 (Analyzing)**：`shell.Service` 检查 cmd 是否包含非法字符或高危路径。
3. **路由 (Routing)**：Sandbox 模块计算当前环境下最合适的解释器路径（重定向）。
4. **拉起 (Spawning)**：`os/exec` 启动进程，劫持 `stdout/stderr`。
5. **归档 (Logging)**：命令及其输出被记录到审计日志，并以 `ProgressBlock` 形式流式推向 SSE。

---

## 3. 跨域集成 (Interactions)

- **Sandbox**：提供物理环境支撑。
- **Chat**：作为 Bash 工具的宿主。
- **Permissions**：受 `allow/deny` 命令清单约束。

---

## 4. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 场景 |
|---|---|---|---|
| `ErrCommandForbidden`| 403 | `PERM_DENIED` | 尝试运行 `rm -rf /` 等被拦截命令。 |
| `ErrSpawnFailed` | 502 | `SHELL_EXEC_ERROR` | 操作系统资源耗尽，无法开新进程。 |
| `ErrInteractiveRequired`| 422 | `SHELL_INPUT_REQUIRED`| 命令停在等待输入状态，被系统切断。 |
| `ErrTimeout` | 504 | `SHELL_TIMEOUT` | 运行超过 30s。 |
| `ErrInvalidCommand` | 400 | `INVALID_REQUEST` | 命令字符串语法错。 |
| `ErrSandboxMissing` | 422 | `SANDBOX_MISSING` | 试图跑 python 但对话还没分配沙箱。 |
