# 07 — Claude Code 用户体验工具（AskUserQuestion / Task 系统）

## 信息来源与局限

主要参考：
- https://platform.claude.com/docs/en/agent-sdk/user-input (官方)
- https://github.com/Piebald-AI/claude-code-system-prompts/blob/main/system-prompts/tool-description-askuserquestion.md
- https://github.com/Piebald-AI/claude-code-system-prompts/blob/main/system-prompts/tool-description-todowrite.md
- https://www.atcyrus.com/stories/claude-code-ask-user-question-tool-guide
- https://platform.claude.com/docs/en/agent-sdk/todo-tracking
- https://oneryalcin.medium.com/when-claude-cant-ask-building-interactive-tools-for-the-agent-sdk-64ccc89558fa

---

## 1. AskUserQuestion Tool

### 1.1 Schema

✅（综合官方 docs + Piebald 收集 + Anthropic Agent SDK 反推）：

```ts
type AskUserQuestion = {
  questions: {
    question: string                 // 完整问题，问号结尾
    header: string                    // ≤12 字标签（chip 显示）
    options: {
      label: string                   // 1-5 字选项文字
      description: string             // 解释这个选项做什么
    }[]                               // 2-4 个选项
    multiSelect?: boolean             // 默认 false
  }[]                                 // 1-4 个问题
}
```

✅ 系统总会自动加一个"Other"选项让用户填自定义文本。返回结果：

```ts
type AskUserQuestionResult = {
  answers: {
    [question_text]: string           // 用户选的 label，或 "Other" + 自定义文本
  }
}
```

### 1.2 描述（描述就是约束）

✅ Tool description 里明文规定（来自 Piebald 收集）：
- 不要询问"plan 是否 ok"——那应该用 `ExitPlanMode`
- 不要在 plan mode 引用"the plan"——用户在 plan 被 ExitPlanMode 提交前看不到 plan
- 推荐选项放第一位 + 加 `(Recommended)` 后缀

### 1.3 Pause 实现机制 ⭐

✅ AskUserQuestion 内部依赖 Agent SDK 的 **canUseTool callback**：
1. LLM 调 `AskUserQuestion`
2. Tool 执行：触发 `canUseTool(toolName="AskUserQuestion", input)` callback
3. **callback 返回 Promise**（pending）→ tool 等待
4. callback 实现方（如 CLI）渲染 UI 让用户选；用户选完 callback resolve
5. tool 拿到结果 → `tool_result` block 注入 next loop iteration

✅ 这就是 SDK pattern：Agent loop 不需要"暂停"——是 tool 的 Promise 没 resolve 而已。

### 1.4 用户回答如何回传

✅ Anthropic Agent SDK 协议：
- CLI：直接通过 stdin 拿用户输入
- Web/Desktop：WebSocket 双向通信，server 推 question event，client 推 answer event；都通过同一个长连
- Agent SDK Python/TS：`canUseTool` callback 是开发者实现的——任何方式拿到 answer 后 resolve Promise

### 1.5 注入位置

✅ User answer 作为 `tool_result` block 注入到 next iteration 的 messages：

```
[user] 帮我搭个 React app
[assistant] (调 AskUserQuestion)
[tool_result] {"answers":{"使用 TypeScript 吗？":"是"}}
[assistant] (继续) 好的，那我们用 Vite + TypeScript ...
```

### 1.6 超时

✅ AskUserQuestion 默认 **60 秒超时**（来自 atcyrus 文章；超时后 tool 返回 timeout error，agent 收到后通常会 fall back 到默认选择或终止）。

### 1.7 限制

✅ **不能在 subagent 里调用** ——只主 agent 能 ask。subagent 想问只能在 final summary 里描述"我不确定 X，请用户决定"由父 agent 处理。

### 1.8 前端 UI

✅ Claude Code CLI：用 Ink（terminal React renderer）画一个 select 列表；箭头键导航；Enter 选；Esc 取消并 timeout。
✅ Web 版：在聊天流里 inline 渲染卡片，用户点击选项自动 submit。

---

## 2. TodoWrite / Task 系统

### 2.1 TodoWrite Schema（v1 / 旧）

✅（截至 v2.1.16 之前的版本）：

```ts
type TodoWrite = {
  todos: {
    id: string              // ≥3 字符
    content: string         // 任务描述（祈使式，"Run tests"）
    activeForm: string      // 现在分词式（"Running tests"）— UI 显示"正在做什么"
    status: "pending" | "in_progress" | "completed"
    priority?: "high" | "medium" | "low"
  }[]
}
```

✅ 关键约束（写在 description 里强制 LLM 遵守）：
- **同一时刻只能有一个 in_progress** todo
- 完成立刻 mark completed，**不要批量** mark
- 测试失败 / 实现部分 / 错误未解 → 保留 in_progress，不能 mark completed
- 任务粒度：3+ 步骤的任务才用 TodoWrite；单步任务不用

### 2.2 v2 / Task 系统（v2.1.16+ 替换）

⚠️ TodoWrite 在 v2.1.16 起被 **Task 系统**取代，新增能力：
- 依赖追踪（task A 必须在 B 完成后）
- 文件系统持久化（跨 session 保留）
- 多 agent 协作（teammate 可以 update 同一个 task）

新工具：`TaskCreate` / `TaskUpdate` / `TaskList` / `TaskStop`。schema 类似但带 `dependencies: string[]`、`assignee: string`（teammate 名）等字段。

❌ Task 系统的完整 schema 定义未在公开分析中找到完整版本。

### 2.3 UI 渲染

✅ TodoWrite 调用→ **特别的 SSE 事件**：
- 终端：每次 `TodoWrite` 调用，CLI 在 status line 上方画一个 checklist box
- Web 版：作为单独的 task widget 而非普通 tool_call 卡片渲染

✅ 状态更新：每次 LLM 调 TodoWrite 都是**整份 todos 重写**（不是 incremental update）。这降低了出 bug 概率（Claude 有时改一个 todo 同时弄乱别的）。

### 2.4 任务列表存储

✅ 旧版（TodoWrite）：仅存 LLM context（assistant message 里），session 结束随 transcript 走。
✅ 新版（Task 系统）：写到 `~/.claude/projects/<sessionId>/tasks.json`，跨 session 持久化。

---

## 3. Extended Thinking 可见性

### 3.1 Thinking Block 在 UI

✅ Anthropic API 返回的 `thinking` block 在 Claude Code UI 里：
- 默认**折叠**（"Thinking…" 占位 + 可展开）
- 展开时显示完整推理链
- 不计入 conversation main context（虽然计 token）

### 3.2 Effort Level

✅ Extended thinking 控制：
- 通过 `thinkingBudget` 参数（API：`thinking: { budget_tokens: N }`）
- Claude Code 在 settings.json：`"extendedThinking": { "budgetTokens": 16000 }`
- 用户可通过 `/think` 命令临时切换"思考预算" levels：low (4K) / medium (16K) / high (32K) / max (64K)
- LLM 在 thinking block 里"想完"后，正常输出 assistant response

### 3.3 Reasoning Token 提取

✅ Anthropic stream 协议：thinking 内容在 `content_block_delta` 事件中以 `delta: { type: "thinking_delta", thinking: "..." }` 增量返回。Claude Code 累积这些 delta 拼成 thinking block。

✅ Forgify 已经有 `chat.reasoning_token` SSE 事件（runner.go:54）——这点已经对齐。

---

## 4. Slash Commands

### 4.1 内置命令清单

✅ 主要内置 slash command（来自官方 cli-reference + 社区清单）：

| 命令 | 用途 |
|---|---|
| `/help` | 帮助 |
| `/init` | 引导创建 CLAUDE.md |
| `/compact [focus]` | 主动触发 compaction（可附 focus 字符串） |
| `/context` | 显示 token 用量分布 |
| `/usage` | 显示本月配额用量 |
| `/clear` | 清空当前 conversation |
| `/resume` | 恢复历史 session |
| `/model <name>` | 切换 model（sonnet/opus/haiku） |
| `/permissions` | 打开权限规则 UI |
| `/hooks` | 浏览所有 hooks |
| `/mcp` | MCP server 状态、OAuth |
| `/agents` | 配置自定义 subagent |
| `/skills` | 浏览/启用 skills |
| `/doctor` | 诊断安装/配置问题 |
| `/login` / `/logout` | 账户 |
| `/review [PR#]` | 内置 review skill |
| `/security-review` | 内置安全审查 |

约 30+ 内置 + skills 自动暴露的（每个 skill 1 个）+ 用户自定义的，总计**~85+** 是常见数字。

### 4.2 实现机制

✅ Slash command 在 prompt 提交前被解析：
- 优先内置命令（hardcoded routing）
- 然后 plugins 提供的命令
- 然后用户/项目自定义命令（`~/.claude/commands/<name>.md` 或 `<project>/.claude/commands/<name>.md`）
- 最后 skills 自动暴露的（`~/.claude/skills/<name>/SKILL.md` → `/<name>`）

✅ 用户自定义 slash command 文件格式：

```markdown
---
description: 给 LLM 看到的简介
allowed-tools: Read, Edit       # 可选限制
argument-hint: <file>            # 可选参数提示
---

请你做以下事情：
$ARGUMENTS               # 用户输入的参数会替换这个 placeholder
... 自由 prompt 模板 ...
```

✅ 用户输入 `/my-cmd foo bar` → markdown 模板里 `$ARGUMENTS` 替换为 `foo bar` → 整段作为 user message 提交给 LLM。

---

## 5. 对 Forgify 的改进建议

> 现状：
> - SSE 已有 `chat.reasoning_token`（已对齐）✅
> - 无 Task / Todo 系统
> - 无 AskUserQuestion——agent 想问只能等用户主动开口
> - SSE 事件齐全：`chat.token`/`chat.tool_call`/`chat.tool_result`/`chat.done`/`chat.error`

| # | 改进 | 优先级 | Go + 前端实施要点 |
|---|---|---|---|
| 1 | **AskUserQuestion tool（暂停 + 续跑）** | P0 | 后端：新文件 `agent/ask.go`，schema 同 §1.1。Execute 不立即返回——把问题写到 `domain/conversation` 的 pending question 表（DB），同时 publish 新 SSE 事件 `chat.question`{convID, msgID, questions[]}。然后**在 channel 上 block** 等用户答案。<br><br>新增 REST endpoint `POST /conversations/<id>/questions/<qid>/answer` body=`{answers:{...}}`：写答案到 DB，关闭 channel。<br><br>Tool Execute 拿到答案 → 返回 JSON tool_result。<br><br>Agent loop 看 tool result 自然续跑。<br><br>超时：60s timeout 用 `ctx.WithTimeout`。<br><br>前端：`chat.question` 事件触发渲染选择器组件，用户点选后 POST answer。</br></br></br></br></br></br></br>影响文件：`agent/ask.go`（新）、`chat/runner.go`（无需改，已经支持任意 tool）、`infra/eventbridge` 加事件类型、前端 chat view。| 
| 2 | **TodoWrite 系统** | P1 | 后端：`agent/todo.go` 实现 schema §2.1。Execute 写到 conversation 关联表 `todos(id, conv_id, content, active_form, status, priority, seq)`。每次调用**整组 replace**。Publish 新 SSE 事件 `chat.todo_update`{convID, todos[]}。<br><br>无需"持久化"复杂度——和 conversation 同生命周期。<br><br>前端：固定位置渲染 checklist；status 用 ✓/⏳/○ icon。 |
| 3 | **Extended thinking budget 控制** | P2 | 已有 reasoning_token 事件 ✅。配置侧：在 conversation 设置加 `thinking_budget: int`，默认 0 = 关闭，>0 用 Anthropic API thinking block。前端 setting panel 加滑块 |
| 4 | **基础 slash command 解析** | P2 | 在 chat.go Send 入口检查 message 首字符==`/`，路由到内置命令。内置先做：`/clear`（清 history）、`/context`（返回 token 分布）、`/compact`（触发 compaction，依赖报告 03 改进） |
| 5 | **自定义 slash command（远期）** | P3 | 扫描 `~/.forgify/commands/*.md`；解析 frontmatter；`$ARGUMENTS` 替换；作为 user message 提交 |

最先做：**#1（AskUserQuestion）+ #2（TodoWrite）**：
- AskUserQuestion 工作量约 1 天后端 + 0.5 天前端，但**用户体验跃升**
- TodoWrite 工作量约 1 天，让 agent 在做长任务时给用户清晰进度，主观感受"专业"+++

实现复杂度排序：**TodoWrite < AskUserQuestion < Slash Commands < Extended Thinking budget**。建议按这个顺序做。

