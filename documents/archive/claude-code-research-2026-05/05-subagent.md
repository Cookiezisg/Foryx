# 05 — Claude Code Subagent 系统

## 信息来源与局限

主要参考：
- https://code.claude.com/docs/en/sub-agents (官方)
- https://platform.claude.com/docs/en/agent-sdk/subagents (Agent SDK)
- https://claude.nagdy.me/learn/subagents/
- https://www.verdent.ai/guides/claude-code-worktree-setup-guide
- https://claudefa.st/blog/guide/development/worktree-guide
- https://github.com/anthropics/claude-code/issues/4182 (depth limit)
- https://www.sabrina.dev/p/reverse-engineering-claude-code-using

---

## 1. Agent / Task Tool 实现

### 1.1 Tool 签名

✅ `Agent`（v2.1+，原名 `Task`）的参数 schema 大致：

```ts
{
  description: string,           // 3-5 字短描述（status line 显示）
  subagent_type: string,         // "general-purpose" | "Explore" | "Plan" | <custom-name>
  prompt: string,                // 给 subagent 的完整 brief（self-contained）
  isolation?: "worktree",        // 可选：开 git worktree 隔离
  run_in_background?: boolean,   // 可选：异步跑，主 agent 收 future
  model?: "haiku" | "sonnet" | "opus"  // 可选：覆盖 subagent default model
}
```

返回：subagent 完成后唯一的 final message 字符串（1000-2000 token 推荐）。中间 tool call 和 result **不进父 agent context**——这是 subagent 最大价值。

### 1.2 Spawn 流程

✅（综合多个分析）spawn 一个 subagent：

```
1. 父 agent 调 Agent tool, 参数验证
2. Agent tool 内部:
   a. 创建独立 ToolUseContext (新 abortController, 新 messages, 新 messageId)
   b. 加载 subagent 配置 (system prompt + allowed tools list)
   c. 把 prompt 当首个 user message
   d. 调 query() → queryLoop() → 跑独立 ReAct 循环
      ├─ subagent 自己的 tool 调用都在自己 context 内
      ├─ subagent 的 LLM stream 不 yield 到父
      └─ 父 agent 这时候 await
   e. subagent 终止 → 取最后 assistant 文本作为 final message
3. Agent tool 把 final message 包成 tool_result block 返父 agent
4. 父 agent 看到 tool_result，next loop iteration
```

异步（`run_in_background: true`）：步骤 2.d 立即返回一个 task handle；父 agent 继续干别的；后续用 `SendMessage` 或 `Monitor` tool 查 / 通信。

### 1.3 Final Message 生成

✅ subagent 最后一个 assistant 文本就是 summary。subagent 的 system prompt 里会强调"end with a comprehensive summary"——并不是单独 LLM 调用做摘要。

---

## 2. 深度限制（重点）

### 2.1 当前实现（v2.x）

✅ Claude Code **设计上禁止 nested subagent**：subagent 拿到的 tool 列表里**不包含 `Agent`/`Task` tool 本身**（issue #4182 确认）。

✅ 这是**靠 tool registry 过滤**实现，不是通过 ctx 标记或全局计数器。

### 2.2 为什么不让

✅ 防递归爆炸 + token 失控（一个 subagent fork 5 个，每个再 fork 5 个 = 25 个并行 LLM call）。

### 2.3 绕过 / Workaround

✅ 用户已发现：subagent 可以在 Bash 里调 `claude -p "<prompt>"` 启动一个 non-interactive Claude 实例。这绕过了 tool-level 限制——但被官方视为 bad practice（observability 差、并发 cost 没限制、recursion 仍有可能）。

⚠️ Codex 等同类项目允许 nested 但 cap 深度 5；Claude Code 选择**硬禁止深度>1**。

---

## 3. 内置 Subagent 类型

### 3.1 general-purpose

✅ 继承父 agent 的**全部 tool 列表**，包括 Bash/Edit/Write。除了不给 Agent tool 本身。模型沿用主 session 的 model（默认 Sonnet）。

### 3.2 Explore（只读 + Haiku）

✅ 限制：
- 只能用 `Glob`, `Grep`, `Read`, `Bash`（**注**：Bash 能用，但 Bash 自己有 read-only 命令白名单——`ls/cat/find/wc` 这些；Edit/Write 通过 Bash 也会被 sandbox 拦）
- 默认模型：Haiku（快、便宜）
- system prompt 里强调"快速 survey、不要深 read、最后给 summary"
- 参数 `breadth: "quick" | "medium" | "very thorough"`：影响 system prompt 措辞

✅ 用途：父 agent 让 Explore 在 monorepo 里 grep/glob 找东西，summary 拿回来，**不污染父 context**——这是 subagent 最常见用法。

### 3.3 Plan

✅ 只读 + 可调用 ExitPlanMode；不能 Bash/Edit/Write/NotebookEdit。模型：Sonnet（推理强）。
✅ 用途：在 plan mode 里被父 agent 调起来设计实现方案。

### 3.4 自定义类型

✅ 文件 `~/.claude/agents/<name>.md` 或 `<project>/.claude/agents/<name>.md`：

```markdown
---
name: code-reviewer
description: Review pending code changes for security and bugs
tools: [Read, Grep, Glob, Bash(git:*)]
model: sonnet
---

You are a security-focused code reviewer. ...

(系统 prompt 全文写这里)
```

✅ 也支持通过 `--agents` CLI 标志临时引入一个 session：

```bash
claude --agents path/to/my-agents.json
```

---

## 4. Worktree 隔离

### 4.1 实现机制

✅ `isolation: "worktree"` 触发：
1. 父 agent 调 Agent tool with `isolation: "worktree"`
2. Agent tool 调用 `git worktree add <tmp_path> -b <new_branch>`（自动生成临时 branch 名）
3. subagent 的 ToolUseContext 把 cwd 设为 `<tmp_path>`
4. subagent 所有 file ops 都在 worktree 内，不动主 working tree
5. subagent 完成时通过 `WorktreeRemove` 钩子做清理：
   - 若 subagent **没改任何文件**：自动 `git worktree remove <tmp_path>` 并删 branch
   - 若**有改**：保留 worktree 和 branch，把路径和 branch 名作为 result 返回，让用户自己 review/merge

### 4.2 配置

✅ session 级也可在启动时打开默认 worktree 模式：`--worktree` flag 或 settings `worktree: true`。

⚠️ 已知 issue（#47548）：`isolation: "worktree"` 在某些 git 配置下会切父 worktree 的 branch 而不是建新 worktree——是 bug，待修。

### 4.3 真正的并发隔离

✅ Agent A 改 `src/auth.ts`、Agent B 改 `src/payments.ts` 在各自 worktree 里跑，**不会冲突**。完成后两个 branch，用户决定合并顺序。

---

## 5. Teams / Parallel Mode

### 5.1 多 agent 真正并行

✅ Claude Code 的 "Agent Teams" 模式（claude-code Teams）：
- 多个 agent 在**独立的 tmux pane** 里跑，每个 pane 是独立 claude 进程
- 通过 **Unix Domain Socket mailbox** 互发消息（`SendMessage` tool）
- 每个 agent 有自己的 role（researcher / implementer / verifier），role 是用户在 `/teammates` 配置的

### 5.2 SendMessage 实现

✅ `SendMessage(to: <teammate-name>, message: string)` 写到目标 mailbox 的 UDS；目标 agent 的 h2A queue 收到 → 打断当前正在等的 LLM stream（实际是塞到下一个 turn 边界）。

### 5.3 角色分配

✅ Hardcoded 三个推荐 role（researcher / implementer / verifier）；也支持自定义 role file。每个 role 是一个 subagent 配置 + 启动指令。

### 5.4 同步等待

✅ Manager agent 用 `WaitFor(teammate, condition?)` tool 等下游完成。条件可以是 timeout / 某 file 出现 / 某 agent idle。

❌ 公开分析中 mailbox 协议的具体二进制格式未找到。

---

## 6. 对 Forgify 的改进建议

> 现状：所有 tool 调用在同一 ReAct 循环、同一 context 内；Forgify Service.Send（chat.go）是单 conversation 入口，没有 subagent。

| # | 改进 | 优先级 | Go 实施要点 |
|---|---|---|---|
| 1 | **基本 Agent tool（独立 context）** | P0 | 新文件 `agent/subagent.go`：定义 `Agent` tool。Execute 做：构建一个 in-memory `subSession` 含独立 messages slice、复用 Service 的 LLM client+tools registry（按白名单过滤）；跑一个简化版 agentRun（不 publish SSE 事件 / 不写 DB），收集所有 assistant text，最后取 last text 作为 result。**关键**：不写 DB——subagent 是 ephemeral 的 |
| 2 | **深度限制** | P0 | `WithSubagentDepth(ctx, n)` 在 ctx 中存深度。Agent tool 注册的 tool list **物理上不含自己** 是更可靠的做法（沿用 Claude Code）。即在 spawn subagent 时，过滤 tools = parent.tools 减去 Agent 工具本身 |
| 3 | **Explore 类型（只读+快模型）** | P1 | 配置 `subagentTypes` map：`Explore: { tools: ["read_file","list_dir","web_search","grep","glob"], model: "haiku-or-equivalent", systemPrompt: "..." }`。spawn 时按 type 过滤 tools。**模型**：通过 Forgify 的 ModelPicker 拿一个"fast"模型 |
| 4 | **Plan 类型** | P2 | 同 Explore，工具集只读，但模型用最强；system prompt 强调"产出 plan，不动代码" |
| 5 | **Worktree 隔离（高级）** | P3 | spawn 时 `git worktree add <tmpdir> -b <branch>`；subagent 的 cwd 切到 tmpdir（通过 ctx 传 cwd 给 system tools）；完成时若 `git status --porcelain` 空则 remove 全清，否则保留 + 把 branch 名加到 result。**前置**：所有 file tool 必须 honor "cwd from ctx" |
| 6 | **异步 / background** | P3 | 用 goroutine + Forgify 现有 SSE 事件机制广播 subagent 进度。`run_in_background=true` 时 Agent tool 立即返 task_id，父 agent 继续；新增 `task_status` tool 查 |

最先做：**#1 + #2 + #3**（最小可用 subagent + Explore 类型），约 2-3 天工作量。Explore 给到主 agent 后能立即把"在大仓库 grep/搜索"这种重活外包出去，对 context window 是巨大释放。

