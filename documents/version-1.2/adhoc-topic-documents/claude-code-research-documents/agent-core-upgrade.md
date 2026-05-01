# Claude Code vs. Forgify — 全面对标分析

**创建于**：2026-04-28
**目的**：对标 Claude Code（2026-04 泄漏源码分析），找出 Forgify 的系统性差距，制定补强路线图。
**参考来源**：Claude Code npm source map 泄漏（512K 行 TypeScript，1906 文件，2026-03-31）+ 多篇深度技术分析。

---

## 结论摘要

Forgify 的**核心 ReAct 循环骨架是正确的**。但 Claude Code 之所以能做复杂自主任务，靠的不是那 1.6% 的 AI 决策逻辑，而是另外 98.4% 的确定性工程基础设施。差距集中在：Context Compaction、工具完整度、记忆系统、Subagent、Hooks、用户体验工具（AskUserQuestion / Task 系统）。

---

## A. 核心 Agent Loop

### Claude Code 实现

- **Streaming-first**：SSE 解析中，tool_use block 的 arguments 一旦完整即触发执行，不等整个 response 结束
- 单次 LLM 响应可包含**多个 tool_use block**，按序执行
- 终止条件：model 输出纯文本（无 tool call）即停
- 典型任务需要 **5~50 次循环**
- **h2A 异步队列**：用户可在 agent 执行中途注入新指令，Claude 在当前 tool result 处理完后自然融入，无需重启
- **Stop Hook**：可以在 Claude 打算结束时强制继续（比如：测试没过，不允许停）

### Forgify 现状

- `consumeStream` 收完整个流 → `builder.finalize()` → 再执行工具，**工具调用存在明显延迟**
- 取消 = context cancel，无法中途转向
- 没有 Stop Hook 等价物，agent 决定停就停

### 差距 & 改进

| # | 改进项 | 优先级 |
|---|---|---|
| A1 | **Mid-stream 工具执行**：arguments 完整即触发，不等 EventFinish | 高 |
| A2 | **用户 Steer 机制**：queue 接受 "steer" 类型任务，注入到当前循环 | 中 |
| A3 | **Stop Hook**：在 finalPersist 前回调一个可以 veto 的 hook | 中 |

---

## B. 工具系统

### Claude Code 完整工具清单（40+ 个）

#### 文件操作类

| 工具 | 说明 |
|---|---|
| `Read` | 读文件，支持图片 / PDF / Jupyter notebook |
| `Write` | 写文件（整体替换） |
| `Edit` | **字符串精确替换**（首选，比 Write 更透明，有 diff） |
| `MultiEdit` | 原子多文件编辑 |
| `Glob` | 模式匹配找文件（按 modification time 排序） |
| `Grep` | ripgrep 正则搜索 |
| `LS` | 目录列表 |

#### 执行类

| 工具 | 说明 |
|---|---|
| `Bash` | shell 执行，working dir 持久化，env var **不持久** |
| `PowerShell` | Windows 支持 |
| `REPL` | 交互式 REPL |

#### Web 类

| 工具 | 说明 |
|---|---|
| `WebFetch` | 抓网页，**独立 context window** 防 prompt injection，15 min 缓存 |
| `WebSearch` | 搜索，支持 domain 过滤 |
| `WebBrowser` | 完整浏览器操作 |

#### 代码智能类（LSP）

| 工具 | 说明 |
|---|---|
| `LSP` | 跳转到定义、查引用、inspect 类型、列 symbols、追 call hierarchy |

#### Agent 编排类

| 工具 | 说明 |
|---|---|
| `Agent` / `Task` | **生成子 agent**，独立 context，执行完返回 summary |
| `TaskCreate` | 创建 todo 任务（UI 渲染为 checklist） |
| `TaskList` | 列出当前任务 |
| `TaskUpdate` | 更新任务状态（in_progress / completed） |
| `TaskStop` | 停止任务 |
| `SendMessage` | subagent 间通信（mailbox 机制） |
| `EnterWorktree` / `ExitWorktree` | 进入 / 退出 git worktree 隔离环境 |
| `Monitor` | 监控 MCP server |

#### 用户交互类

| 工具 | 说明 |
|---|---|
| `AskUserQuestion` | **主动暂停并向用户提问**（不是被动等，是主动问） |
| `TodoWrite` | 写 TODO list，UI 渲染为交互 checklist |

#### MCP 类

| 工具 | 说明 |
|---|---|
| `MCPTool` | 调用任意 MCP server 的工具（`mcp__<server>__<tool>` 命名） |

#### Skill 类

| 工具 | 说明 |
|---|---|
| `Skill` | 执行自定义 skill 命令（Markdown 文件定义） |

#### 其他

| 工具 | 说明 |
|---|---|
| `NotebookEdit` | Jupyter notebook 编辑 |

---

### Forgify 现状工具清单（13 个）

| 分类 | 工具 |
|---|---|
| System Tools（6） | read_file, write_file, list_dir, run_shell, run_python, datetime |
| Web Tools（2） | web_search, fetch_url |
| Forge Tools（5） | search_tools, get_tool, create_tool, edit_tool, run_tool |

### 关键缺失工具分析

| 缺失工具 | 为什么重要 | 优先级 |
|---|---|---|
| `Edit`（字符串替换） | write_file 是全覆盖，edit 是精确替换 + diff，对代码改动更安全可追溯 | 高 |
| `Glob` + `Grep` | 代码搜索是 agent 能力基石，比 read_file+list_dir 组合强得多 | 高 |
| `AskUserQuestion` | agent **主动暂停问用户**，不猜测自己决定，复杂任务必备 | 高 |
| `TaskCreate/Update` | agent 自管任务列表，用户实时看进度，长任务必备 | 高 |
| `Agent`（子 agent） | 并行/隔离执行，见 E 节 | 高 |
| `LSP` | 语义级代码导航（跳定义/查引用），forge 场景极有价值 | 中 |
| `MultiEdit` | 原子多文件编辑 | 中 |
| `WebBrowser` | 完整浏览器操作 | 低 |

---

## C. Context 管理

> **这是 Phase 4+ 最关键的工程硬约束。没有 Compaction，workflow 这种天生很长的任务会撞墙。**

### Claude Code 实现

**触发阈值**：context 使用达到 **92~95%** 时自动触发。

**三层压缩 pipeline**：

| 层 | 名称 | 做什么 |
|---|---|---|
| Tier 1 | MicroCompact | 只移除旧消息中的原始 tool output，低风险，最快 |
| Tier 2 | AutoCompact | 全量历史传给 model 做摘要，保留架构决策/未解决 bug/实现细节，丢弃重复输出和过时中间结果。典型压缩率 **60~80%** |
| Tier 3 | 手动 `/compact [focus]` | 用户可指定"聚焦 auth 重构部分"，定向保留 |

**关键细节**：
- CLAUDE.md 在每次 compaction 后**重新从磁盘读**，保证永远最新
- 系统提示按 `SYSTEM_PROMPT_DYNAMIC_BOUNDARY` 分割：
  - Boundary 前（tool 定义、基础指令）→ **全组织级缓存**，跨 session 复用
  - Boundary 后（git 状态、项目配置、时间戳）→ session-specific，每次重算
- 提示缓存：同一 session 后续请求 system prompt prefix 命中缓存，**token 费用大幅降低**

**Checkpoint 系统**：
- 每次 Edit/Write 工具执行前快照文件状态
- 跨 session 持久化，支持回滚
- 注意：Bash 的 rm/mv/cp 不在 checkpoint 范围内

**典型 session 成本**：
- 简单 bug fix：30,000~50,000 tokens
- 复杂重构：150,000~200,000 tokens（触发 compaction 前）
- 开发者平均费用：约 $6/天

### Forgify 现状

```go
const maxHistoryMessages = 200
// buildLLMHistory 硬截取最新 200 条，超出的旧消息静默丢弃
```

无 compaction，无缓存分割，无 checkpoint，无 token 计数。

### 改进建议

| # | 改进项 | 说明 | 优先级 |
|---|---|---|---|
| C1 | **三层 Compaction** | 在 Service 里实现 token 计数 + 触发压缩逻辑，Tier 1 最先做 | **最高，Phase 4 前必须** |
| C2 | **System prompt 缓存分割** | 静态部分（tool defs）vs 动态部分（git 状态、conversation.systemPrompt）分开 | 高 |
| C3 | **Checkpoint** | Edit/Write tool 执行前快照文件 | 中 |
| C4 | **Token 计数器** | 每次 LLM 调用后累积 inputTokens，接近阈值触发 compaction | 高 |

---

## D. 记忆系统

### Claude Code 实现

**三层记忆架构**：

```
Layer 1: MEMORY.md（始终加载，≤200行 / ≤25KB）
         └── 仅存 pointer：[topic](file.md) + 一行描述

Layer 2: Topic 文件（按需加载）
         └── user.md / feedback.md / project.md / reference.md

Layer 3: Session 日志（append-only，靶向检索）
```

**加载机制（session 启动时）**：
1. 从当前目录向上遍历，逐级加载 `CLAUDE.md`（层级覆盖：项目 > 用户 > 组织）
2. MEMORY.md 前 200 行自动加载（index）
3. Topic 文件在 agent 判断相关时 on-demand 读取
4. Rules files：`.claude/rules/*.md`（无条件加载 + path-scoped 按需）
5. Skills：`~/.claude/skills/` + `.claude/skills/`（自定义 slash command）

**CLAUDE.md 注入方式**：作为 **user message** 注入（不是 system prompt），compaction 后重新读盘，**永不丢失**

**autoDream（后台记忆整合）**：
- 触发条件：24 小时后 + 5+ session + 获取 lock
- 四阶段：Orient（扫描记忆）→ Gather（收集新信号）→ Consolidate（写/更新）→ Prune（维持 <200 行）
- 以独立 subagent 运行

### Forgify 现状

- `conversation.systemPrompt`：用户手动写，不是 agent 学到的
- 没有跨会话记忆
- 没有 CLAUDE.md 等价物

### 改进建议

| # | 改进项 | 说明 | 优先级 |
|---|---|---|---|
| D1 | **项目指令文件（FORGIFY.md）** | 从项目目录向上遍历，注入为 user message，compaction 后重读 | 高 |
| D2 | **Auto Memory** | agent 可以读写记忆文件（新增 write_memory / read_memory tool），session 启动时自动加载 | 高 |
| D3 | **Memory 分层** | index 文件（指针）+ topic 文件（内容），≤200 行 index | 中 |
| D4 | **autoDream** | 后台 goroutine，idle 一段时间后触发记忆整合 | 低（Phase 5） |

---

## E. Subagent 系统

### Claude Code 实现

**内置类型**：

| 类型 | Model | Tools | 用途 |
|---|---|---|---|
| Explore | Haiku（快速轻量） | 只读（Read/Grep/Glob） | 发现/调研 |
| Plan | 继承父 agent | 只读 | 规划 |
| general-purpose | 继承父 agent | 完整访问 | 通用执行 |

**关键约束**：
- 深度限制 **1 层**（subagent 不能再生 subagent）
- subagent 完成后只返回 **1000~2000 token 的 summary**，不管内部用了多少 token
- 支持自定义：system prompt / tool allowlist+denylist / model / permission mode / isolation 设置

**Worktree 隔离**：`isolation: worktree` 模式让 subagent 运行在临时 git worktree，对主库零影响，完成后自动清理

**Teams 模式（并行）**：多个 agent 通过 tmux pane + shared message bus 真正并行，各有角色（researcher / implementer / verifier），使用 Unix Domain Socket mailbox 通信

### Forgify 现状

无 subagent，所有工具调用在同一 ReAct 循环同一 context。

### 改进建议

| # | 改进项 | 说明 | 优先级 |
|---|---|---|---|
| E1 | **Agent Tool** | `spawn_agent(prompt, tools, model)` → 独立 Service 实例 → 返回 summary | 高 |
| E2 | **深度限制** | subagent ctx 里标记 depth，>1 时禁止再 spawn | 高（安全） |
| E3 | **专用类型** | Explore（只读，快模型）/ Plan（只读）/ general-purpose | 中 |
| E4 | **Worktree 隔离** | subagent 在临时 git worktree 运行（`git worktree add`），完成后清理 | 中 |

---

## F. 权限与安全系统

### Claude Code 实现

**5 层权限 cascade**（从低到高，第一匹配优先）：

| 层 | 机制 |
|---|---|
| 1 | Tool 自身的 `checkPermissions()`（如 Bash 拦截危险命令） |
| 2 | Settings allowlist/denylist（glob 模式：`Bash(npm:*)`, `Read(./.env)`, `WebFetch(domain:x.com)`） |
| 3 | OS 级沙箱（macOS: Seatbelt / Linux: bubblewrap，子进程继承） |
| 4 | Permission mode（default / acceptEdits / plan / bypassPermissions / auto） |
| 5 | Hook 覆盖（PreToolUse hook 可 allow / deny / modify） |

**第一匹配原则**：deny 优先，ask 次之，allow 最后，无歧义

**ML 分类器（"YOLO classifier"）**：auto 模式下判断 HIGH / MEDIUM / LOW 风险，低风险自动批准

**Protected files**：`.gitconfig`, `.bashrc`, `.zshrc`, `.mcp.json`, `.claude.json` 自动保护，不可自动编辑

**Permission Explainer**：高风险操作前单独调一次 LLM 解释风险，让用户看清楚再决定

**Path traversal 防御**：URL 编码、Unicode 归一化、backslash injection、大小写 bypass 检测

### Forgify 现状

无权限系统。当前单用户桌面影响有限，但 Phase 4 workflow 自动执行时风险上升。

### 改进建议

| # | 改进项 | 说明 | 优先级 |
|---|---|---|---|
| F1 | **Tool 级权限声明** | Tool 接口加 `PermissionLevel()` 方法（ReadOnly / WorkspaceWrite / DangerFullAccess） | 高 |
| F2 | **Protected paths** | 写文件前检查是否是 .git / .env 等敏感路径 | 高（安全） |
| F3 | **Settings 规则** | allow/deny glob pattern 配置（`Bash(rm -rf *)` 之类） | 中 |
| F4 | **沙箱加固** | Bash tool 加 timeout + 危险命令黑名单 | 中 |

---

## G. Hooks 系统

### Claude Code 实现

**Hook 类型**：

| Hook | 触发时机 | 能做什么 |
|---|---|---|
| `PreToolUse` | tool 参数生成后、执行前 | allow / deny / modify input / 追加 context |
| `PermissionRequest` | 权限对话框出现前 | allow / deny 代替用户 |
| `PostToolUse` | tool 执行成功后 | 提供纠正反馈（注入到下一轮） |
| `Stop` | Claude 打算结束时 | **强制继续**（质量门控：测试没过不许停） |
| `SessionStart` / `SessionEnd` | session 边界 | 初始化 / 清理 |
| `ConfigChange` | 配置变更时 | 追踪变更 |

**Hook 实现方式**：shell 命令 / HTTP endpoint / LLM prompt（yes/no 评估）/ subagent（Read+Grep+Glob）

**MCP 工具也走同一 Hook 系统**：`mcp__<server>__<tool>` 命名，统一进 PreToolUse

### Forgify 现状

`settings.local.json` 里已有 PostToolUse hook（编辑 backend/ 时注入文档同步提醒）。好起点，但 hook 系统本身没有正式 API，是临时配置。

### 改进建议

| # | 改进项 | 说明 | 优先级 |
|---|---|---|---|
| G1 | **正式化 Hook 接口** | `type Hook interface { Before(ctx, toolName, args) HookResult; After(ctx, toolName, result) }` | 高 |
| G2 | **Stop Hook** | agent finalPersist 前回调，可拦截并继续（如：让 agent 先跑测试） | 中 |
| G3 | **Hook 配置文件** | 从 settings 读取 hook 规则，支持 shell 命令 / HTTP endpoint | 中 |

---

## H. MCP 集成

### Claude Code 实现

- MCP 工具**延迟加载**：session 启动只加载 tool name（节省 context），实际调用时才加载完整 schema
- 多 scope：project / user / local / enterprise / 插件 server，各有独立配置
- MCP tool 统一走 permission + hook 系统，`mcp__<server>__<tool>` 命名规范
- 支持 MCP resources（数据资源）和 MCP prompts（提示模板）
- OAuth 2.0 HTTP transport 认证（Protected Resource Metadata discovery）
- Trust model：Anthropic 不审计 MCP server，用户自己负责 allowlist

### Forgify 现状

计划 Phase 5，尚未实现。

### 改进建议（Phase 5 时采用）

| # | 改进项 | 优先级 |
|---|---|---|
| H1 | 延迟加载 schema（只在首次调用时拉完整定义） | 高 |
| H2 | MCP tool 注册到统一 Tool 接口，走同一 permission + hook | 高 |
| H3 | 优先实现 stdio transport（本地进程），HTTP 次之 | 中 |

---

## I. 用户体验工具

### Claude Code 实现

**Task/Todo 系统**：
- `TodoWrite` 创建结构化任务列表（ID / content / status / priority）
- UI 渲染为可交互 checklist，用户实时看 agent 进度
- agent 主动更新（in_progress → completed），不需要用户触发

**AskUserQuestion Tool**：
- agent **主动暂停**问用户，不是被动等待
- 用户回答后 agent 继续，输入已作为 context
- 复杂任务中的关键：不确定时问，不瞎猜

**Slash Commands（约 85 个）**：
| 命令 | 用途 |
|---|---|
| `/compact [focus]` | 手动触发摘要，可指定保留焦点 |
| `/tasks` | 查看任务列表 |
| `/context` | 查看当前 context 用量分布 |
| `/usage` | token 用量和费用统计 |
| `/mcp__<server>__<prompt>` | 动态 MCP 提示 |

**Skills 系统**：
- `~/.claude/skills/` + `.claude/skills/` 目录
- 每个 Markdown 文件 = 一个自定义 slash command（描述 + 使用方式）
- `Skill` tool 在 agent 循环中执行

**Extended Thinking 可见性**：
- 默认开启
- effort level 控制（low / medium / high / max）
- thinking block 折叠展示在 UI

### Forgify 现状

- `chat.reasoning_token` SSE + testend 折叠展示 ✅
- 无 Task/Todo 系统
- 无 AskUserQuestion
- 无 Slash Commands（testend 有手动 collections，不是 agent 层的）
- 无 Skills 系统

### 改进建议

| # | 改进项 | 说明 | 优先级 |
|---|---|---|---|
| I1 | **AskUserQuestion Tool** | `execute()` 返回 `WAITING_FOR_USER`，SSE 推 `chat.question`，前端展示输入框，用户回答后恢复 agent | **最高** |
| I2 | **Task 系统** | `task_create/list/update` tool → SSE 推送 → 前端渲染 checklist | 高 |
| I3 | **Skills 系统** | `.forgify/skills/` 目录，Markdown 文件 = 自定义指令 | 中 |
| I4 | **`/context` 命令** | 返回当前 token 用量分布，用户知道快满了 | 中 |

---

## J. 生产级工程特性

| 特性 | Claude Code | Forgify 现状 | 改进建议 | 优先级 |
|---|---|---|---|---|
| **Prompt 缓存分割** | SYSTEM_PROMPT_DYNAMIC_BOUNDARY，静态部分组织级缓存 | 无 | 降低 token 费用 | 高 |
| **LLM Retry with backoff** | 429/5xx 自动重试 | 无 | 稳定性必备 | 高 |
| **WebFetch 独立 context** | 用单独 context window，防 prompt injection | 同一 context | subagent 实现后顺手做 | 中 |
| **Session 成本追踪** | `/usage` 实时显示 token/费用 | 无 | `/context` 命令 | 中 |
| **Extended Thinking effort 控制** | low/medium/high/max | 无法控制 | 加 API 参数透传 | 中 |
| **OS 级沙箱** | macOS Seatbelt / Linux bubblewrap | subprocess timeout | 单用户桌面可推迟 | 低 |
| **Undercover mode** | 公开 repo 时抑制内部信息 | 无 | 推迟 | 低 |

---

## K. 全面优先级路线图

### 立刻做（Phase 4 开工前）

```
C1  Context Compaction（3 层，token 计数触发，60~80% 压缩率）
A1  Mid-stream 工具执行（arguments 完整即触发，不等 EventFinish）
I1  AskUserQuestion Tool（agent 主动暂停问用户）
I2  Task/Todo 系统（进度 checklist 可视化）
B1  Edit Tool（字符串精确替换 + diff，替代 write_file）
B2  Grep + Glob Tool（代码搜索基础设施）
J2  LLM Retry with backoff（429/5xx 稳定性）
```

### Phase 4 开始时做

```
E1  Agent Tool（Subagent，独立 context，1 层深度限制）
D1  项目指令文件 FORGIFY.md（目录向上遍历加载，compaction 后重读）
D2  Auto Memory（agent 读写记忆文件，跨 session 学习）
F1  Tool 权限声明（PermissionLevel 接口）
F2  Protected paths（.git / .env 保护）
G1  正式化 Hook 接口（PreToolUse / PostToolUse / Stop）
C2  System prompt 缓存分割（静态 vs 动态）
```

### Phase 5 及以后

```
H1  MCP 延迟加载
E2  Worktree 隔离（subagent 跑在临时 git worktree）
D4  autoDream 后台记忆整合
I3  Skills 系统（.forgify/skills/）
C3  Checkpoint（Edit/Write 前快照）
```

---

## L. 核心差距可视化

```
                    Claude Code          Forgify 现状
                    ────────────         ────────────
核心 ReAct 循环     ████████████         ████████████  ← 已对齐
Streaming 体验      ████████████         ████████░░░░  ← mid-stream 缺失
工具完整度          ████████████         ████░░░░░░░░  ← 13 vs 40+
Context 管理        ████████████         ██░░░░░░░░░░  ← 最大差距
记忆系统            ████████████         ░░░░░░░░░░░░  ← 从零开始
Subagent            ████████████         ░░░░░░░░░░░░  ← 从零开始
Hooks 系统          ████████████         ██░░░░░░░░░░  ← 雏形
用户体验工具        ████████████         ████░░░░░░░░  ← AskUser/Task 缺失
权限安全            ████████████         ░░░░░░░░░░░░  ← 从零开始
MCP 集成            ████████████         ░░░░░░░░░░░░  ← Phase 5
```

---

*文档创建于 2026-04-28，基于 Claude Code npm source map 泄漏（2026-03-31）的多篇深度分析。*

---

## 开发日志

### 2026-04-28 — [refactor] app/chat 管线重构

**动机**：现有 pipeline.go 有 4 层嵌套（runReactLoop → runStep → consumeStream → finalize），历史构建分两条路径（循环内内存 `current` + 跨轮 DB 重建），`allBlocks` 跨步骤累积配合 `persistMsg` 反复 upsert 同一行，逻辑难以追踪和扩展。

**目标**：单层 for 循环 + 三个职责单一的函数，历史统一一条路径，DB 写入时机明确，预留 context compaction 钩子位置。

**文件变动**：

| 文件 | 操作 | 说明 |
|---|---|---|
| `pipeline.go` | 删除内容（保留空 package 声明） | 所有逻辑迁移到 runner.go |
| `runner.go` | 新建 | 队列管理 + `processTask` + `agentRun` + `writeDB` |
| `stream.go` | 重写 | `streamLLM`（单遍事件处理）+ `assembleBlocks` + `extractToolCalls` |
| `history.go` | 重写 | `buildHistory` + `extendHistory`（统一路径） + `blocksToLLM` |
| `tools.go` | 小改 | `executeToolCalls` → `runTools` |
| `stream_test.go` | 重写 | 适配新 `assembleBlocks` API |

**关键决策**：
- 推流时机：不变（EventText/Reasoning/ToolStart 立即推 SSE）
- 写表时机：不变（有 tool call 时写 streaming checkpoint，结束时写终态）
- 本轮**不做** A1（mid-stream 工具执行），`// TODO: context compaction` 注释占位
- `extendHistory` 是循环内历史增长的唯一入口，与 `buildHistory` 共用 `blocksToAssistantLLM`
- `writeDB(fatal bool)` 替代 `persistMsg` + `finalPersist` 两个函数，语义通过参数区分

**结果**：`go build ./...` 零错误，`go test -race ./internal/app/chat/...` 全绿（1.6s）。全量测试除 `infra/llm` 集成测试（需要真实 API key，与本次改动无关）外全部通过。
