# 01 — Claude Code Agent 核心循环 / ReAct Pipeline

## 信息来源与局限

本研究基于 2026-03-31 `@anthropic-ai/claude-code` v2.1.88 source map 泄漏后公开的二手分析文章，以及 Anthropic 官方 docs 中已公开但被反编译者交叉验证过的部分。原始 cli.js（59.8 MB）/ cli.js.map（1900 文件、512K+ 行 TS）已被 DMCA 下架,本会话**不能直接读到泄漏源码本身**,所有"代码级"细节均来自社区反编译笔记与官方 docs。

主要参考(实际可访问):
- https://blog.promptlayer.com/claude-code-behind-the-scenes-of-the-master-agent-loop/
- https://github.com/VILA-Lab/Dive-into-Claude-Code (deepwiki)
- https://github.com/inematds/claudecode-manual (`04-query-engine.md`)
- https://kenhuangus.substack.com/p/chapter-3-the-query-agent-loop-claude
- https://deepwiki.com/myopicOracle/analysis_claude_code_in_English/2.2-message-queue-and-real-time-steering
- https://gerred.github.io/building-an-agentic-system/parallel-tool-execution.html
- https://code.claude.com/docs/en/how-claude-code-works (官方)

**置信度标注**：✅ 多源交叉验证；⚠️ 单源/版本相关；❌ 未在公开分析中找到。

---

## 1. 主循环结构

### 1.1 文件与函数链

✅ Claude Code 的 agent 主循环位于 `src/query.ts`（在 v2.1.88 快照中约 1729 行），核心函数是一个 **async generator**：

```ts
async function* queryLoop(state: State): AsyncGenerator<StreamEvent>
```

调用栈自上而下（每一层都是 `async function*`）：

```
submitMessage()        // QueryEngine 类的公开入口（每次用户回合一次）
  └─ query()           // 顶层 generator，做参数校验、系统 prompt 组装
       └─ queryLoop()  // 真正的 while(true) ReAct 循环 ← ★主体
            └─ queryModel()    // 单次 LLM streaming 调用，位于 src/claude.ts
                 └─ withRetry() // 10 次重试包装层
```

✅ `QueryEngine` 是**每会话一个、持有 abortController 和消息历史的有状态类**，每次 `submitMessage` 表示一次用户回合；公开表面（REPL、SDK、远程 CC）全部最终汇入同一个 `query()` generator。

### 1.2 State 对象

✅ 整个循环只有一个被 spread-rebuild 的 `State` 对象在迭代之间传递：

```ts
type State = {
  messages: Message[]
  toolUseContext: ToolUseContext           // 含 abortController.signal 等
  autoCompactTracking: AutoCompactTrackingState | undefined
  maxOutputTokensRecoveryCount: number     // 输出 token 限制重试次数
  hasAttemptedReactiveCompact: boolean     // 是否已经做过 reactive compaction
  pendingToolUseSummary: Promise<ToolUseSummaryMessage | null> | undefined
  stopHookActive: boolean | undefined      // 防 stop hook 死循环
  turnCount: number                        // 当前轮次
  transition: Continue | undefined         // 仅诊断用：上一轮为何继续
}
```

每次 continue 时写法是 `state = { ...state, /* 改动字段 */ }`，从不分散赋值——这让 React 风格的 reducer 思维可以应用到 agent 上，便于测试。

### 1.3 Continue 的 7 种原因 / Terminal 的 7 种退出

✅ **Continue（再绕一圈）** by `transition.reason`：

| reason | 何时触发 |
|---|---|
| `max_output_tokens_escalate` | 首次 8k cap 撞上 → 升到 64k |
| `max_output_tokens_recovery` | 输出限制再次触发 → 注入"继续输出"nudge（≤3 次） |
| `reactive_compact_retry` | 入参超 → 触发 reactive compaction → 重试 |
| `collapse_drain_retry` | 入参超 → 已用完 collapse 阶段 → drain 后重试 |
| `stop_hook_blocking` | stop hook 报错 → 把错误当下一条 user message 重新 query |
| `token_budget_continuation` | 本轮 token 数<90% 预算且仍有进展 → 注入 nudge |
| 检测到 tool_use blocks | 跑工具 → loop |

✅ **Terminal（退出循环）**：
`completed` / `blocking_limit` / `model_error` / `prompt_too_long` / `aborted_streaming` / `stop_hook_prevented` / `image_error`

### 1.4 9 步 Turn Pipeline

✅ 每一次循环迭代里有 9 个阶段（来自 VILA-Lab 分析）：

1. Settings resolution（resolve allow/deny/managed）
2. State initialization
3. Context assembly（拼 system prompt + history + 当前 user msg）
4. **5 个 pre-model context shapers**（详见报告 03）
5. Model invocation（调 queryModel → streaming 输出）
6. Tool dispatch（解析 tool_use blocks）
7. Permission gate evaluation（详见报告 08）
8. Tool execution（详见 §2.3）
9. Stop condition checking（决定是 continue 还是 terminal）

### 1.5 终止条件

✅ 直接终止条件：
- LLM 没产生 tool_use block（"completed"）→ 自然结束
- abortController.signal 触发（用户 ESC）→ "aborted_streaming"
- 撞到 prompt_too_long 且 collapse_drain 也救不了 → "prompt_too_long"
- API 返回不可恢复错误 → "model_error"
- stop hook 返回 prevent 且 stopHookActive 已为 true → "stop_hook_prevented"

Claude Code 没有显式的"maxSteps=20"硬上限——它靠 token 预算 + 8k/64k output cap + 重试次数（≤3 次输出 cap 升级）+ 用户中断 来自然收敛。

---

## 2. Streaming 与执行的配合

### 2.1 Streaming via Async Generator

✅ `queryLoop` 是 async generator，event 一边 yield 一边被消费（终端 UI 实时渲染、字符级"打字"效果）。整个栈都是 `async function*`，靠 `yield*` 自然组合，反压由 generator 的 pull-based 语义提供。

### 2.2 触发时机：StreamingToolExecutor vs runTools

✅/⚠️ Claude Code 有 **两套执行路径**：

- **`StreamingToolExecutor`**（feature gate `config.gates.streamingToolExecution`）：在 LLM 还在产生后续 token 时就开始执行已经流完 arguments 的 tool。一个 tool block 流完即可触发执行，**不必等 EventFinish**。极大降低多 tool 回合的延迟。
- **fallback `runTools`**：等整段 LLM response 收齐后再处理（按 §2.3 的批次规则执行）。

### 2.3 多 tool call 的并行/串行决策

✅ Claude Code 的核心洞察：**tool 不是一律并行，而是按 concurrency-safety 分组成 batch，每个 batch 内部要么全并行、要么全串行**。

逻辑（位于 `toolOrchestration.ts`）：

```ts
// 伪代码
function partition(toolUses: ToolUse[]): Batch[] {
  return toolUses.reduce((batches, tu) => {
    const safe = isConcurrencySafe(tu.name, tu.input)
    const last = batches[batches.length - 1]
    if (last && last.safe === safe && safe) {
      // 相邻安全的 tool 合并到同一个并行 batch
      last.items.push(tu)
    } else {
      // 否则起一个新 batch
      batches.push({ safe, items: [tu] })
    }
    return batches
  }, [])
}
```

`isConcurrencySafe()` 判定关键看 tool 上的 `readOnlyHint` —— Read/Grep/Glob 是 read-only，可并行；Edit/Write/Bash 默认非并行。例子（来自 ona.com 反编译笔记）：

```
[Read, Read, Write, Read, Bash(rm)]
→ batch 1: [Read, Read]   并行
→ batch 2: [Write]        串行
→ batch 3: [Read]          并行
→ batch 4: [Bash]          串行
```

### 2.4 retry 层 withRetry()

✅ 每次 API 调用都过 `withRetry()`（最多 10 次）：

- **529（overloaded）**：仅 foreground 重试；background 立即放弃
- **Opus fallback**：连续 3 次 529 → 抛 `FallbackTriggeredError`，自动降级到非 Opus
- **OAuth 401**：先刷新 token 再重试
- **400 context overflow**：解析 error 拿到准确 token 数，算 `maxTokensOverride`
- **ECONNRESET / EPIPE**：调 `disableKeepAlive()` 后重试
- **Persistent mode**：无限重试，30 分钟 backoff 上限，每 30 秒 yield 一个 heartbeat
- 退避：`min(BASE_DELAY × 2^(n-1), maxDelay) + 0~25% jitter`，遵守 `Retry-After` 头

---

## 3. 取消与 Steer 机制

### 3.1 取消（ESC / Ctrl+C）

✅ AbortController 存于 `QueryEngine`，传到 `toolUseContext.abortController.signal`。
- ESC（一次）= abort 当前 streaming → 状态标记 `aborted_streaming` → 写 tombstone message → 退出循环
- 工具执行中 abort：tool 自身需要 honor signal；某些 tool（如 Bash 子进程）会被 SIGTERM
- 已知 GitHub issue #6643：意外按 ESC 会立即中断且不要求确认 → 容易丢工作

⚠️ ESC 不是"先停下来等指令"——它是**销毁性中断**。要做"非破坏性 steer"目前社区方案是先 Ctrl+C 再输入新指令（issue #30492）。

### 3.2 h2A 队列 / Real-time Steering

✅ Claude Code 用 **h2A 异步双缓冲队列**（`#primary` / `#secondary` 两个 RingBuffer）连接 UI 和 agent loop。
- 实现 `Symbol.asyncIterator`，消费者用 `for await` 拉，`break` 时调可选 cleanup callback
- enqueue 时：有 reader 等待 → 立即 resolve；否则 push 到 primary buffer
- 吞吐 >10K msg/s（M2 Max + 512 MiB heap 实测）
- **支持 pause/resume，用户中途打字可以注入到下一个 turn boundary**——这是"steering"的基础

⚠️ 现实中（issue #36326）：用户在 Claude 工作时 Enter 提交的消息不会立刻打断当前 tool，而是**queue 到下一个 turn 边界**。所以"实时 steer"在 Claude Code 是 turn-level，不是 tool-level。

### 3.3 Stop Hook：阻止 agent 结束

✅ **Stop hook** 在 Claude 打算结束本轮回复时触发（每次回合结束都触发，不是任务完成才触发）：

- shell 形式：exit 2 + stderr 内容 = 阻止停止；agent 收到 stderr 当反馈继续干活
- JSON 形式：`{"decision":"block","reason":"测试没过，先跑测试"}` 或 prompt 形式 `{"ok":false,"reason":"..."}`
- 防死循环：hook 输入里带 `stop_hook_active: boolean`，hook 必须自查这个标志，否则会无限循环

`handleStopHooks()` 跑三类：
1. Stop Hooks（用户配置，并行）
2. TaskCompleted Hooks（teammate 模式）
3. TeammateIdle Hooks（agent team idle 转换）

死循环防御：先前一条 message 是 API error 时，hook 自动 skip（避免 hook 报错 → API 重试 → hook 再报错 的递归）。

---

## 4. 错误处理与恢复

### 4.1 Tool 失败

✅ Tool 执行失败的路径：
1. 抛异常 → 包装成 `tool_result` block，`is_error: true`，content = 错误文本
2. 作为下一轮 user message 的一部分喂回 LLM
3. LLM 自行决定如何调整（重试 / 换路 / 终止）

不会因 tool 失败终止主循环。

### 4.2 LLM API 错误

✅ withRetry() 处理 transient 错误（见 §2.4），不可恢复错误（如 invalid_request、超 1M tokens）抛出 → State 标记 `model_error` → terminal exit。

### 4.3 Checkpoint / 恢复

✅ Claude Code 在**每条 assistant 响应之后**自动对所有被 modify 的文件做 snapshot（详见报告 03 §4），存于 `~/.claude/projects/<session>/snapshots/<hash>@v<n>`。会话级也写入 JSONL 在 `~/.claude/projects/<id>.jsonl`，崩溃后能 `claude --resume` 恢复（但 in-flight tool 状态不能恢复，permissions 也要重新批）。

---

## 5. 并发与队列

### 5.1 多对话隔离

✅ 每个 `QueryEngine` 实例是独立的，绑定一个 sessionId / cwd。Claude Code CLI 一次只有一个 active session（同一 terminal）；多 session 用 worktree 或多终端。
- 在多终端共用同一 session_id 时（`claude --continue` 同时跑两个），都写同一 JSONL 文件，消息会交错（不会损坏，但会乱），官方推荐用 `--fork-session` 拆分。
- KAIROS 后台 agent（泄漏出现但未发布）会有真正的多 conversation 并发。

### 5.2 每会话队列

⚠️ 在主交互 loop 内**没有显式 per-conversation 排队队列**，h2A 是 producer/consumer 而非任务队列。Forgify 的"每对话 channel + worker goroutine"模型实际上比 Claude Code 当前 CLI 更显式。

---

## 6. 对 Forgify 的改进建议

> 现有实现：`backend/internal/app/chat/runner.go` 中 `agentRun` 单层 for+`maxSteps=20`，`stream.go:64` 已留 TODO(A1) 标记 mid-stream 触发；`tools.go:29` `runTools` 一律 goroutine 全并行；无 stop hook、无 retry、无 context shaper。

| # | 改进 | 现状 | 目标（Claude Code 做法） | 实施 | 影响文件 | 优先级 |
|---|---|---|---|---|---|---|
| 1 | **拆 State 对象** | 5 个独立局部变量 + sr 字符串 | 单一 `State` struct + `transition reason` 诊断字段 | 在 `runner.go` 顶部声明 `type agentState struct{Messages, TotalIn, TotalOut, StopReason, AttemptedCompact bool, MaxOutTokensRetry int, Transition string}` | `chat/runner.go` | P0（重构性，便于后续所有功能） |
| 2 | **Stop Hook 接口** | 无；LLM 一不调工具就立即 writeDB+break | 调用 `s.hooks.RunStop(ctx, state)`，返回 `block:true` 时把 reason 当 system message 注入 history、loop 再走一轮 | runner.go agentRun "无工具调用→writeDB" 分支前插入 `if shouldContinue, reason := s.runStopHooks(ctx, allBlocks); shouldContinue { history = append(history, llminfra.LLMMessage{Role:"user",Content:"<stop-hook>"+reason}); continue }`；新增 `chat/hooks.go` 定义 `type StopHook interface { Eval(ctx, state) (block bool, reason string, error) }` | `chat/runner.go:168`, 新文件 `chat/hooks.go` | P1 |
| 3 | **mid-stream tool 执行** | TODO(A1) 已留位置 | StreamingToolExecutor：`EventToolStart(N+1)` 到达即可启动 tool N | 在 `stream.go:64` 把"args 完整即推到执行池"做掉。改返回类型为 `(blocks, toolFutures map[int]<-chan blockResult, ...)`；`agentRun` 收 future 而不是同步 `runTools` | `chat/stream.go`, `chat/runner.go` | P2（收益大但实现复杂、容易错） |
| 4 | **isConcurrencySafe 分批** | 一律 goroutine 全并行 | Read/Grep/Glob 并行，Edit/Write/Bash 串行；相邻同性质合批 | Tool 接口加 `IsReadOnly() bool`；`runTools` 改为先 partition 再循环每个 batch（safe→`sync.WaitGroup` 全并行；unsafe→串行 for） | `agent/tool.go`, `chat/tools.go` | P0（安全性 + 一致性，写文件应该串行） |
| 5 | **withRetry 包装** | 无；网络抖一下就 publishError → terminal | `withRetry()` 10 次，`Retry-After`、jitter、ECONNRESET 处理 | `infra/llm` 增加 `RetryClient` decorator，包住 `client.Stream`。`runner.go` 收到 `EventError` 时区分 transient 和 fatal | `infra/llm/*`, `chat/runner.go:161` | P1 |
| 6 | **token budget continuation** | 无 | 本回合 output 数<90% 且非 diminishing → 注入 nudge "继续完成" | `agentRun` 终止前判 `if oT < 0.9*budget && !diminishing { history append nudge; continue }` | `chat/runner.go` | P3（先看用户是否有这个痛点） |
| 7 | **abort 行为更精细** | `cancel()` 立即终止 | abort 后等当前 tool batch 跑完再退、写 partial state | `agentRun` 收 `ctx.Done()` 时 `runTools` 不取消、setStopReason=Cancelled、再 break | `chat/runner.go` | P2 |
| 8 | **不变 maxSteps 上限** | 20 步硬上限 | 改成 token+turn 软上限，靠 stop reason 自然收敛 | 删除 maxSteps，加 `softTurnLimit=50` 但只在 transition 都是 same reason 重复 N 次时停 | `chat/runner.go:114` | P3（先观察 20 是否会撞到） |

最先做：**#1 + #4**（重构 + 并发安全性，是其他改进的基础）。

