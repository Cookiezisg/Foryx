# V1.2 Final Sweep — 完工 punch list

> **创建于**：2026-05-15
> **场景**：后端 + testend 已 95% 完工（Phase 0-4 全交付 + Phase 4 准备件 mcp/skill/subagent/catalog/sandbox 都到位）。本文档列出**剩余所有 gap**，不分 V1/V2——本轮全做。
> **不在范围**：前端 Wails 整体迁移（独立工程，留给下一轮）。
> **怎么用**：每个 gap 一个 `☐`，做完划掉。`§N.M` 编号方便引用 / git commit。
> **联动**：完工后同步 `progress-record.md` + 受影响的 `service-design-documents/<domain>.md` + `service-contract-documents/*.md`（§S14 文档同步纪律）。

---

## 索引

1. [长任务能力（Context Compaction）](#1-长任务能力context-compaction)
2. [记忆系统（Memory）](#2-记忆系统memory)
3. [权限 + Hooks](#3-权限--hooks)
4. [可观测性](#4-可观测性)
5. [Workflow 高阶能力](#5-workflow-高阶能力)
6. [MCP 高阶能力](#6-mcp-高阶能力)
7. [Skill 高阶能力](#7-skill-高阶能力)
8. [Subagent 高阶能力](#8-subagent-高阶能力)
9. [Catalog 高阶能力](#9-catalog-高阶能力)
10. [UX 工具完善](#10-ux-工具完善)
11. [Sandbox 收尾 + 修复](#11-sandbox-收尾--修复)
12. [HTTP / API 收尾](#12-http--api-收尾)
13. [可靠性 / 故障恢复](#13-可靠性--故障恢复)
14. [Phase 5 真正没做的](#14-phase-5-真正没做的)
15. [完美产品 UX](#15-完美产品-ux)
16. [桌面 app 准备（非 Wails UI 部分）](#16-桌面-app-准备非-wails-ui-部分)
17. [Burn-in 剩余 + 杂项](#17-burn-in-剩余--杂项)
18. [推荐 ordering](#18-推荐-ordering)

---

## 1. 长任务能力（Context Compaction）

**背景**：当前 `maxHistoryMessages=200` 硬截。复杂 forge 锻造 + 跑测 + 改 deps 重跑容易 50+ tool call，超过 200 条就静默丢早期消息——AI 突然"忘了在干啥"。Claude Code 对标的核心缺口。

- ☐ **§1.1 Token counter** —— 每次 LLM 调用后累积 `Conversation.InputTokens / OutputTokens`（已有 message-level 字段，聚合到 conversation 即可）。**Done when**：`GET /api/v1/conversations/{id}` 返 `tokensUsed: {input, output, totalSinceStart}`。
- ☐ **§1.2 MicroCompact**（Layer 1）—— 历史超过 N tokens 时，**删除旧 tool_result blocks 的 Content**（保留 metadata + tool name），只压最便宜的部分。**Done when**：context > 70% 阈值触发；保留最近 K turn 的完整 tool_result；旧 tool_result content 改为 `[compacted: 4.2 KB elided]`。
- ☐ **§1.3 AutoCompact**（Layer 4）—— context > 92% 时，把全历史发给 LLM 做摘要，保留架构决策 / 未解决 bug / 当前 task，丢重复输出。结果作为 first user message 注入新 context。**Done when**：阈值触发、摘要落 `compacted_messages` 表（可审查）、新 context 接续无中断。
- ☐ **§1.4 手动 `/compact [focus]` 命令** —— 用户输入触发 AutoCompact，可选 focus 字符串聚焦。**Done when**：testend 输入框打 `/compact auth-refactor` 立刻触发并 stream 进度。
- ☐ **§1.5 System prompt 缓存边界** —— `SYSTEM_PROMPT_DYNAMIC_BOUNDARY` 分割：静态段（tool defs / FORGIFY.md / catalog）+ 动态段（locale / now / git status）。**Done when**：静态前缀命中 Anthropic prompt cache，token 费用降 ~60% per turn。
- ☐ **§1.6 Tool result budget** —— `applyToolResultBudget` ——单次 tool_result 超 N tokens 自动截断 + 追加 `[output truncated; original size: X KB]`。**Done when**：`Bash` / `Read` 大输出自动截断；前端能查看完整版（`/api/v1/blocks/{id}` 已有）。
- ☐ **§1.7 `/context` 命令** —— 显示当前 context 占用分布（system prompt / history / catalog / each tool def）。**Done when**：testend 一键查看。

---

## 2. 记忆系统（Memory）

**背景**：当前 `conversation.SystemPrompt` 字段是用户手动写。**跨会话记忆完全没有**——用户每开新对话都得重新教"我用 Python / 不要写中文注释 / 忽略 .venv"。

- ☐ **§2.1 `FORGIFY.md` 自动注入** —— `~/.forgify/FORGIFY.md` + 工作目录向上递归 + project-level `.forgify/FORGIFY.md`（层级覆盖）。每次 chat turn build system prompt 时**重新从磁盘读**（compaction 后也是）。作为 user message 注入（不 system），永不被 compaction 删。**Done when**：`backend/internal/app/chat/runner.go::buildSystemPrompt` 读 FORGIFY.md 拼到 system prompt 顶部；编辑文件下次对话立刻生效。
- ☐ **§2.2 `~/.forgify/memory/` 目录 + `MEMORY.md` 索引** —— `MEMORY.md` 是 ≤200 行 index，每行 `- [Topic](file.md) — one-line hook`。Topic 文件按需加载（不进 system prompt，靠 read_memory tool 取）。**Done when**：`~/.forgify/memory/MEMORY.md` 自动在 system prompt 注入（200 行 cap）。
- ☐ **§2.3 `read_memory` tool** —— LLM 主动读 topic 文件。Args: `{name: "user_role"}` → 返 `~/.forgify/memory/user_role.md`。**Done when**：tool 接口 9 方法齐全 + 测试。
- ☐ **§2.4 `write_memory` tool** —— LLM 写新 memory。Args: `{name, content, metadata: {type, description}}`。自动更新 MEMORY.md index 一行。**Done when**：tool 接口齐全 + atomic 写（tmp + rename）+ MEMORY.md 自动更新。
- ☐ **§2.5 `forget_memory` tool** —— 删除 memory 文件 + 更新 index。**Done when**：tool 接口齐全 + 防误删（必须 LLM 主动调，user 不直接触发）。
- ☐ **§2.6 4 类 memory types** —— Claude Code 模式：`user.md` / `feedback.md` / `project.md` / `reference.md` 各自独立 frontmatter。read/write_memory 强制 type 选择。**Done when**：4 类标准化、frontmatter schema 定型。
- ☐ **§2.7 autoDream**（远期）—— idle 24h + 5+ session 后，后台 subagent 整合 memory（dedup / 合并近义 / prune 旧的）。**Done when**：optional Phase 5+ 功能；本轮可选先 skip。

---

## 3. 权限 + Hooks

**背景**：当前权限薄——PathGuard 守敏感路径（filesystem tools），Bash 故意不走 PathGuard（per CLAUDE.md decisión D5），没有 PermissionLevel / Stop Hook / 正式 Hook API。Phase 4 workflow 自动执行后风险升高。

- ☐ **§3.1 Tool 级 `PermissionLevel()` 方法** —— Tool 接口加第 10 方法 `PermissionLevel() Level`（ReadOnly / WorkspaceWrite / DangerFullAccess）。给未来 settings allow/deny 用。**Done when**：20 个现有 tool 各自声明 level；不改任何运行时行为。
- ☐ **§3.2 Protected paths** —— PathGuard 已有 deny list；扩展 deny **写**：`.git/` / `.env` / `.envrc` / `node_modules/` / 用户 `~/.ssh/` 等。**Done when**：Edit/Write tool 在写之前查 Protected paths，命中返友好错误。**注**：跟现有 PathGuard read deny list 复用同结构。
- ☐ **§3.3 Settings allow/deny 规则** —— `~/.forgify/settings.json` 配 glob：`{"deny": ["Bash(rm -rf *)", "Read(*.env)"], "allow": ["Bash(npm:*)"]}`。Tool dispatch 时第一匹配。**Done when**：第一匹配 deny > allow > 默认；settings 改后 1s polling 重读。
- ☐ **§3.4 Stop Hook** —— Agent 准备 finalPersist 前回调 hook；hook 返 `{continue: true, prompt: "tests didn't pass, fix first"}` 则强制继续。**Done when**：hook 接口 + 1 个 demo hook（"测试没过别停"）+ 测试。
- ☐ **§3.5 PreToolUse hook 正式化** —— `PreToolUse(toolName, args)` 返 `Allow / Deny / Modify / Ask`。当前没有正式 API。**Done when**：接口落 `app/hooks/`；3 种触发方式（shell 命令 / HTTP / LLM prompt）；settings 配置。
- ☐ **§3.6 PostToolUse hook 正式化** —— 现 `.claude/settings.local.json` 的 doc-sync hook 是临时配置。改正式 API：`PostToolUse(toolName, args, result)` 返 `nil / FeedbackToInject`。**Done when**：feedback 注入到下一轮 LLM context。
- ☐ **§3.7 SessionStart / SessionEnd hook** —— 对话开 / 关时回调。**Done when**：SessionStart 跑一次（init / 注入 context）；SessionEnd 跑一次（cleanup / log）。
- ☐ **§3.8 Permission Explainer** —— 高风险操作前单独 LLM 调用解释风险（"rm -rf 会删整个项目，确定？"）。**Done when**：destructive=true 的 tool call 前显式 ask user 通过 AskUserQuestion 风格 UI。
- ☐ **§3.9 Path traversal 防御加强** —— PathGuard 加 URL 解码 / Unicode normalization / backslash injection / 大小写 bypass 检测。**Done when**：覆盖 OWASP path traversal 常见 bypass。

---

## 4. 可观测性

**背景**：后端记录够全，但前端 / API 没暴露。"花了多少钱 / 找历史对话 / 调试 LLM 输出" 这些日常需求都没好用工具。

- ☐ **§4.1 Token / cost per-conversation 显示** —— Conversation entity 加聚合字段 `tokensUsed: {input, output}` + `costUsdEstimate`。**Done when**：GET conversation 返；testend 显示。
- ☐ **§4.2 总累计 cost 面板** —— 按 day/week/month 聚合所有 conversation 的 token + cost。`GET /api/v1/usage?period=day`。**Done when**：testend `/usage` 视图。
- ☐ **§4.3 Conversation 搜索** —— 按 title / message content / tool 名搜索。先 SQL LIKE，未来 FTS5。**Done when**：`GET /api/v1/conversations?search=email` 返匹配。
- ☐ **§4.4 Conversation export (markdown)** —— 导出对话为 markdown 文件 / JSON。**Done when**：`GET /api/v1/conversations/{id}/export?format=md|json` 返下载。
- ☐ **§4.5 Tool execution metrics dashboard** —— 慢 tool / 失败率 / p95 latency per-tool。基于现有 D22 execution log 聚合。**Done when**：`GET /api/v1/metrics/tools?since=7d` 返；testend 显示。
- ☐ **§4.6 LLM call trace export** —— 单次对话所有 LLM 调用 (req/resp/cost) 导出 JSON。debugging 用。**Done when**：`GET /api/v1/conversations/{id}/llm-trace` 返。
- ☐ **§4.7 Catalog version diff** —— "自上次看以来 catalog 加了什么"——持久化历史版本（当前只 cache 最新）。**Done when**：`catalog_history` 表 + UI 显示 diff。
- ☐ **§4.8 `/context` 命令** —— 当前 context 占用分布（system prompt / history / each tool def 各占多少 token）。**Done when**：testend 一键查看（跟 §1.7 是同件事）。
- ☐ **§4.9 `/usage` 命令** —— 当前对话 + 今日 + 本月 token / cost。**Done when**：testend 一键查看。
- ☐ **§4.10 LLM provider 健康指标** —— 各 provider 的 success rate / p95 latency 实时看。基于 `apikeys.last_tested_at` + LLM error 累计。**Done when**：testend `LLM` tab 显示。

---

## 5. Workflow 高阶能力

**背景**：Plan 04 + Plan 05 trinity authoring + execution plane 全完工，但 V1.5 标记了几个 deferred。本轮全做。

- ☐ **§5.1 Loop body subgraph** —— scheduler `dispatch_loop.go` 当前 V1 minimal（返 ErrLoopBodyNotSupported sentinel）。完整支持 for-each / while + body subgraph 递归执行。**Done when**：`loop` 节点能跑嵌套 body；3 层嵌套上限；测试覆盖。
- ☐ **§5.2 Parallel branches subgraph** —— scheduler `dispatch_parallel.go` 当前 pass-through（edges 自然并行）。完整支持 branches 子图（多个起点显式并行 + join）。**Done when**：`parallel` 节点能跑 N branches + join semantics；测试覆盖。
- ☐ **§5.3 `parallel(N)` concurrency** —— Workflow.Concurrency 当前只 `serial`。加 `parallel(N)`，允许同 workflow 同时跑 N 个 flowrun。**Done when**：CountRunning 检查改成 `< N`；schema 解析 `parallel(3)` 字符串。
- ☐ **§5.4 Per-trigger TZ override** —— 当前 cron 锁 `time.Local`。每个 trigger 加 `TimeZone` 字段。**Done when**：cron expression 旁可配 `"timeZone": "Asia/Tokyo"`。
- ☐ **§5.5 Cron missedPolicy `runAll` / `skip`** —— 当前只 `runOnce`（默认）。**Done when**：spec 字段 + 3 种 policy 都实现 + 测试。
- ☐ **§5.6 Webhook secret rotation** —— 当前 secret 单字段。加 rotation 支持（旧 + 新两段同时有效一段时间）。**Done when**：mcp.json 风格 `{"secret": "new", "secretOld": "old"}` 两者都通过。
- ☐ **§5.7 Workflow run-level timeout** —— 当前只有 per-node timeout。加整 run 上限（"超过 1h 整 run cancel"）。**Done when**：FlowRun 加 `timeoutSec` + 触达自动 cancel + status=failed (error_code=RUN_TIMEOUT)。
- ☐ **§5.8 Workflow safe mode** —— settings 加 `workflowSafeMode: true` 时，workflow 内的 Bash / function 节点强制走 AskUserQuestion 确认。**Done when**：设置生效 + 已运行的 workflow 不影响。
- ☐ **§5.9 EdgeSpec `ToPort`** —— 当前 V1 单输入，ToPort 字段保留未实现。加多输入端口（如 condition 节点的多 case 合并）。**Done when**：scheduler 解析 ToPort + 测试。
- ☐ **§5.10 Approval timeout** —— approval 节点默认 7d，当前固定。加 per-node `timeoutSec`。**Done when**：approval node config 加字段 + 超时 → run failed (NODE_TIMEOUT)。

---

## 6. MCP 高阶能力

**背景**：当前只 stdio + 21 条 curated marketplace。spec 还有 HTTP transport / Resources / Prompts / Sampling 等 primitives，几乎没人用——但 polished 产品该支持。

- ☐ **§6.1 HTTP / Streamable transport** —— 远程 MCP server 支持（spec 2025-11 加的）。OAuth 2.1 + PKCE。**Done when**：mcp.json 加 `"transport": "http", "url": "..."`；stdio + http 都通。
- ☐ **§6.2 Per-tool enable/disable** —— 当前粒度 server 级。加 per-tool 黑白名单。**Done when**：mcp.json 加 `"disabledTools": ["delete_repo"]`；search 时过滤。
- ☐ **§6.3 `mcp_call_history` 表持久化** —— 当前不存历史。加 D22 风格 table。**Done when**：每 `CallTool` 写一行 + 通用 16 字段 + mcp 专属（server_name / tool_name）+ 查询端点。
- ☐ **§6.4 远程注册表自动发现** —— 当前手编 mcp.json + 21 条 curated。加扫 `registry.modelcontextprotocol.io` 自动发现（带 LLM-rerank 过滤）。**Done when**：UI / LLM 可选"从远程 registry 找"。
- ☐ **§6.5 Resources primitive 支持** —— spec 的 Resources（数据源）。当前没用。**Done when**：mcpinfra Client 加 `ListResources` / `ReadResource`；新 system tool `read_mcp_resource`。
- ☐ **§6.6 Prompts primitive 支持** —— spec 的 Prompts（模板）。**Done when**：`ListPrompts` / `GetPrompt`；slash command `/mcp__<server>__<prompt>` 路径触发。
- ☐ **§6.7 Sampling 协议** —— server 主动调 LLM（"反向"调用）。极少人用。**Done when**：实现 spec sampling endpoint；接 Forgify modelpicker。
- ☐ **§6.8 Per-tool description override** —— 用户嫌某 tool description 太长 / 不准。配置 override。**Done when**：mcp.json 加 `"toolOverrides": {"create_pr": {"description": "..."}}`。
- ☐ **§6.9 Stderr filtering 细粒度** —— 当前 stderr 全转 zap Warn（启动横幅淹没）；2026-05-09 部分修过（WARNING/ERROR 才升 WARN）。加配置：per-server filter rules。**Done when**：mcp.json 加 `"stderrFilter": "ignore-banner"` 或 regex。

---

## 7. Skill 高阶能力

**背景**：D7 全交付（5 sentinels / Service / 2 tools / 9 HTTP / fsnotify → 1s polling）。Anthropic spec 还有 paths-trigger / shell-substitution / slash command 等高级特性，V1 解析但不消费。

- ☐ **§7.1 `paths`-glob auto-trigger** —— 用户编辑文件 match `paths` glob 时，自动注入 skill description hint 到下次 system prompt（"file looks like X, consider Y skill"）。**Done when**：watch 用户编辑 → 匹配 → 注入 hint。
- ☐ **§7.2 Slash command 注册** —— `/skill-name args` 走专用 chat 注入路径（不通过 LLM tool call）。**Done when**：testend 输入框打 `/pr-review 1234` 直接触发 Activate。
- ☐ **§7.3 `!`shell`` 预执行** —— spec 支持 frontmatter / body 内嵌 shell 命令预执行注入。**Done when**：SKILL.md body 内 `` !`gh pr list` `` 在 Activate 时先跑、把 stdout 替进 body。
- ☐ **§7.4 Skill registry 集成** —— 从 `anthropics/skills` repo 一键 install。**Done when**：`POST /api/v1/skills:install-from-registry` body `{repo, name}` → git clone + 校验 + 落 `~/.forgify/skills/`。
- ☐ **§7.5 Plugin 形态加载** —— 通过 plugin manager 动态加载第三方 skill bundle。**Done when**：plugin manifest schema + 加载机制 + CatalogSource 复用。
- ☐ **§7.6 Skill import from URL** —— `:import` 端点支持 URL（git / zip）。**Done when**：multipart 之外加 `{"url": "https://github.com/..."}`。
- ☐ **§7.7 Per-skill model override** —— frontmatter.Model 当前解析不消费。Activate 时给 fork subagent 用 override 的 model。**Done when**：frontmatter `model: claude-opus` 真生效；non-fork 也支持（覆盖 next turn 的 chat model）。

---

## 8. Subagent 高阶能力

**背景**：D4 全交付（3 内置 type + filterTools 防递归 + 双保险 + 5min 总超时 + multi-agent forging system prompt）。

- ☐ **§8.1 跨厂 subagent 定义** —— 从代码内置改文件加载（`~/.forgify/subagents/<name>.md` YAML frontmatter）。**Done when**：filesystem scan + 1s polling + Registry 改读文件源；3 内置类型迁文件作 default。
- ☐ **§8.2 Token budget 强约束** —— 基于 chatRepo 聚合 subagent 行 InputTokens/OutputTokens + 用户配的对话级上限触发拒绝。**Done when**：settings 加 `subagentBudgetPerConv: 100000`；超限 SpawnResult.Status=BudgetExceeded。
- ☐ **§8.3 Subagent 内嵌套（configurable MaxDepth）** —— V1 默认禁；加配置 `subagentMaxDepth: 1`（默认）/ `2` / etc.。**Done when**：filterTools 按 depth 决定是否剥 SubagentTool；测试 nested。
- ☐ **§8.4 Cancel HTTP 端点** —— 当前没有外部 cancel API。加 `POST /api/v1/subagent-runs/{id}:cancel`。**Done when**：端点存在 + Service.Cancel 接 activeRuns 注册表。
- ☐ **§8.5 Worktree-isolated subagent** —— `isolation: worktree` 模式，subagent 跑在 `git worktree add` 临时分支，完成自动清理。**Done when**：opts.Isolation 字段 + `infra/git/worktree` 包；测试隔离 + 清理。

---

## 9. Catalog 高阶能力

**背景**：D8 全交付（单次 attempt + mechanical fallback + 1s polling + fingerprint dedup + 3 sources forge/skill/mcp + slim notifications）。

- ☐ **§9.1 Knowledge source** —— Phase 5 knowledge domain 起，加 `knowledgeapp.AsCatalogSource()`。**Done when**：knowledge 实体（§14）出来后自动接 catalog 0 行修改。
- ☐ **§9.2 Workflow source** —— workflow 实体也进 catalog（让 LLM 知道有哪些 workflow 可触发）。**Done when**：`workflowapp.AsCatalogSource()` 实现 + register。
- ☐ **§9.3 Catalog 分页** —— 单 source 50+ items 时，generator 输出"data-processing 类: 12 items, call list_forges(category='data') for full list"，二次嵌套 progressive disclosure。**Done when**：超阈值时分组 + LLM 看到 hint 自己再查。
- ☐ **§9.4 Per-conversation catalog** —— 用户能"这个对话只看 forge X / Y / skill Z"。**Done when**：conversation 加 `catalogFilter: {forges: [...], skills: [...]}` 字段 + 拼 system prompt 时按 filter 过滤。
- ☐ **§9.5 Plugin 动态注册** —— plugin manager 运行时调 RegisterSource。**Done when**：sources slice mutex 已经 thread-safe（已有），加 Unregister 方法。
- ☐ **§9.6 Catalog version diff history** —— 持久化历史版本，UI 显示"自上次看以来加了什么"。**Done when**：`catalog_history` 表 + N 个版本保留 + diff endpoint。

---

## 10. UX 工具完善

**背景**：Edit/Write/Bash/Read 等 system tool 已交付，但缺一些"polished agent UX"特性。

- ☐ **§10.1 Checkpoint / Undo for Edit/Write** —— Edit/Write 执行前快照文件（sha256 + content cache）。`POST /api/v1/checkpoints/{id}:restore` 回滚。跨 session 持久化。**Done when**：checkpoints 表 + 每 Edit/Write 写一行 + 7d retention + UI 时间轴显示。
- ☐ **§10.2 File diff visualization** —— Edit tool 返 tool_result 当前是文本。改成结构化 `{oldHash, newHash, unifiedDiff}`，前端渲染 diff view。**Done when**：testend BlockView 渲染 diff（红 - / 绿 +）。
- ☐ **§10.3 Mid-stream tool execution** —— Claude Code 实现：SSE 解析中 tool_use arguments 完整即触发执行，不等整个 response。当前 Forgify 收完整流后才跑工具。**Done when**：infra/llm parser 检测到 tool_call args 完整 → emit EventToolReady → loop/tools.go 立刻 dispatch；测试。
- ☐ **§10.4 User Steer** —— Agent 执行中途用户注入新指令（"等等，先看这个"）。当前唯一选项是 cancel + 新开 turn。**Done when**：chat queue 接受 `steer` 类型 task，在当前 tool_result 处理完后自然融入下一轮 LLM。
- ☐ **§10.5 Slash command 系统**（`/compact` / `/tasks` / `/usage` / `/context` / `/skill-name` / `/mcp__server__prompt`）—— 当前只 chat normal message。**Done when**：runner 检测 `/<command>` 前缀 → route 到对应 handler；支持 §1.4 / §4.8-9 / §7.2 等。
- ☐ **§10.6 Stop reason 用户可见** —— 当前 stopReason 是后端字段（end_turn / max_tokens / cancelled / error），前端没显示。**Done when**：testend 在 message 终态显示 "(stopped: max_tokens)"。
- ☐ **§10.7 Tool retry button** —— tool_result error 时，UI 提供"重试"按钮 → 重新跑同 tool call 同 args。**Done when**：testend 错误 tool_result 旁有按钮。

---

## 11. Sandbox 收尾 + 修复

**背景**：D1-D2 全交付（mise embed + 11 EnvManager + 3 层 leak 防御）。但 burn-in #15-17 + D2 遗留有几个真 bug。

- ☐ **§11.1 env destroy 后 function lazy-rebuild**（burn-in #15）—— 当前 env destroy 后 next RunFunction 报错而不是自动重建。**Done when**：RunFunction 探测 env not ready 时自动 Sync；测试覆盖。
- ☐ **§11.2 ownerKind 非白名单显式拒**（burn-in #16）—— 当前传非法 ownerKind 静默吞。**Done when**：sandboxapp.EnsureEnv 入口校验 owner.Kind ∈ 白名单，否则 ErrInvalidOwnerKind。
- ☐ **§11.3 runtime 表 `3.12` vs `>=3.12` dedup**（burn-in #17）—— 两个版本说同一件事，但是 DB UNIQUE 让它们各占一行。**Done when**：Install 时解析 PEP 440 specifier → 取实际 install 版本（mise where）作 UNIQUE key；旧行迁移。
- ☐ **§11.4 Ruby / PHP EnvManager 修 bundler / composer**（D2 遗留）—— bundler / composer 不在 mise registry，Ruby/PHP EnvManager 调 EnsureTool 会失败。**Done when**：直接静态装 bundler / composer（github release 二进制）；EnvManager 用之；测试。
- ☐ **§11.5 env corruption 自动重装** —— `.venv/bin/python` 缺失（mise upgrade 把 install dir 改了）→ 自动 Sync 重建。**Done when**：Spawn 前 check 关键 binary 存在 → 不在则 Sync；测试。
- ☐ **§11.6 Docker runtime support** —— sandbox.md §4 提过留待 v1.5。**Done when**：`infra/sandbox/docker.go` Installer + EnvManager；mcp 等可选 docker runtime；ErrDockerNotInstalled / ErrDockerDaemonDown 已有 sentinel。
- ☐ **§11.7 Disk usage warn** —— sandbox 共享 cache 长跑后磁盘膨胀。`TotalSizeBytes > 5GB` 时 testend warn + 提示 `:gc`。**Done when**：可视化 + 阈值通知。

---

## 12. HTTP / API 收尾

**背景**：Plan 05 14 项 hardening 完成，但 burn-in #18 + 几个收尾还在。

- ☐ **§12.1 Pagination limit cap**（burn-in #18）—— 当前 `limit` 无上限，可传 `?limit=10000` 拉爆。**Done when**：`pkg/pagination.Parse` cap 到 200；超过返 400 INVALID_REQUEST。
- ☐ **§12.2 HTTP request retry (LLM client)** —— `infra/llm/openai.go` / `anthropic.go` 当前一次性发；429 / 529 / ECONNRESET 直接抛。加 exponential backoff（3 次重试上限）。**Done when**：`pkg/withretry` 通用；LLM client 接；测试。
- ☐ **§12.3 Per-conversation model override** —— 当前全局 model_configs；某对话想用别的 model 没接口。**Done when**：Conversation 加 `modelOverride: {provider, modelId}` 字段；PickForChat 检查 conv 字段优先。
- ☐ **§12.4 Webhook 多 HMAC 算法** —— 当前 secret 单字符串。GitHub / GitLab 等用 HMAC-SHA256 不同 header 名。**Done when**：spec 加 `"signatureAlgo": "hmac-sha256", "signatureHeader": "X-Hub-Signature-256"`。

---

## 13. 可靠性 / 故障恢复

- ☐ **§13.1 LLM client withRetry**（同 §12.2，单列因为这是可靠性核心）—— exponential backoff 3 attempts。**Done when**：测试 mock 429 → 3 次后成功。
- ☐ **§13.2 LLM provider auto-failover** —— 单 provider 持续失败 → 自动切到 fallback provider（用户配的）。**Done when**：apikey 配 `fallbackProvider` 字段；chat resolver 实现切换。
- ☐ **§13.3 SearXNG instance 健康度跟踪** —— `app/tool/web/search.go` 三层 fallback，但 SearXNG 池中坏实例不剔除。**Done when**：连续失败 ≥ 3 次的 instance 30 分钟内不用。
- ☐ **§13.4 BackgroundCtx 死亡检测** —— catalog / skill / mcp polling 用 background ctx；进程关 ctx 不取消（OS 回收）。但万一卡死 goroutine 不退。**Done when**：Stop() 方法 + main.go shutdown 显式调 + 测试。
- ☐ **§13.5 Conversation queue 长任务 worker timeout** —— `app/chat/runner.go` queue worker 没自身超时；卡死 goroutine 占资源。**Done when**：worker per-task 总超时 30 分钟（可配）；超时 cancel + log。

---

## 14. Phase 5 真正没做的

**背景**：愿景里 Phase 5 = knowledge / document + intent routing + chat 终极版。当前都 ⬜。

- ☐ **§14.1 sqlite-vec compatibility spike** —— modernc.org/sqlite 加载 sqlite-vec C 扩展可能不兼容。**Done when**：30 分钟 spike 跑通；不通则评估替代方案（chromem-go / 重新接 mattn）。
- ☐ **§14.2 Knowledge / Document domain** —— `domain/knowledge` + `domain/document`；entity + Repository + 1 sentinel。**Done when**：4 层都到位 + AutoMigrate。
- ☐ **§14.3 Document upload + chunking pipeline** —— `POST /api/v1/documents` 上传文件 → 切分 chunk → 向量化 → 落 sqlite-vec。**Done when**：UI 拖拽上传 + 进度 SSE + 几种格式（pdf / txt / md / docx）。
- ☐ **§14.4 `search_knowledge` system tool** —— 按 query 向量检索 + 返 top K chunks。**Done when**：tool 9 方法齐 + 测试 + catalog source（§9.1）。
- ☐ **§14.5 Knowledge attach 到 conversation / workflow** —— "这个对话只查 X / Y 知识库"。**Done when**：Conversation / Workflow.LLM 节点加 `knowledgeIds: []` 字段；search_knowledge tool 按 conv filter。
- ☐ **§14.6 Intent routing** —— 主 chat LLM 第一步识别用户意图（"创建工作流" vs "改工具" vs "纯问答"）→ 注入不同 system prompt sub-section。**Done when**：runner.buildSystemPrompt 加 intent 段；前 N turn 走 intent agent。
- ☐ **§14.7 Chat 终极版 — 工作流推荐 + 自动建草稿** —— 用户描述需求 → AI 推荐用哪个 workflow / forge / skill → 自动 draft 一个 workflow → ask user 确认。**Done when**：multi-agent forging system prompt 已经在 F2 教过；加 workflow draft 自动 spawn create_workflow 工具。

---

## 15. 完美产品 UX

**背景**："这些不做也能用，但没了感觉不像 polished v1.2"。

- ☐ **§15.1 First-run onboarding** —— 首次启动检测 `~/.forgify/` 不存在 → 引导：选语言 / 配 API key / 装一个 sample MCP / 装一个 sample skill。**Done when**：onboarding flow + skip 选项。
- ☐ **§15.2 Sample skills bundled** —— 装机自带 3-5 个示例 skill（如 `pr-review` / `csv-clean` / `code-review`），引导用户看怎么写。**Done when**：`internal/samples/skills/` go:embed + 首次启动复制到 `~/.forgify/skills/`。
- ☐ **§15.3 Sample forges bundled** —— 同上，3-5 个示例 function。**Done when**：装机后用户能立刻 run 一个 sample forge 看效果。
- ☐ **§15.4 Conversation starter prompts** —— 新对话有几个"试试这个"按钮（"帮我写一个 CSV 处理工具" / "解释这段代码"）。**Done when**：testend 新对话页有 starter 卡片。
- ☐ **§15.5 Settings preferences pane** —— 配置 model / api key / hooks / paths-deny / autoCompact 阈值 / language。**Done when**：testend `/settings` 视图 + backend `GET/PUT /api/v1/settings`。
- ☐ **§15.6 Conversation pinning / favorites** —— pin 几个常用对话置顶。**Done when**：Conversation 加 `isPinned: bool` 字段 + UI。
- ☐ **§15.7 Conversation tags / folders** —— `tags: []string`；按 tag 过滤 list。**Done when**：schema + UI。
- ☐ **§15.8 LLM provider health indicator** —— testend tray 持续显示各 provider 状态（绿 / 黄 / 红）。**Done when**：1 min polling apikey.last_tested_at + UI badge。
- ☐ **§15.9 Error message 本地化** —— errmap wire code 现在只英文 message。中文用户场景下应该本地化（前端按 wire code 翻译）。**Done when**：前端建 zh-CN 翻译表 + 按 errorCode 显中文友好语。
- ☐ **§15.10 Help / docs in-app** —— testend 内嵌 quick reference（每 tool 的简介 / 常用 prompt 模板 / 快捷键）。**Done when**：`/help` view + 搜索框。
- ☐ **§15.11 Backup / restore** —— `POST /api/v1/admin:backup` 打包 `~/.forgify/` + DB → zip 下载。`:restore` 反向。**Done when**：备份恢复都 work + 测试。
- ☐ **§15.12 Keyboard shortcuts** —— testend 加常用快捷键（Cmd+K 搜索 / Cmd+N 新对话 / Esc cancel / Cmd+/ slash command）。**Done when**：shortcut table + 帮助页。
- ☐ **§15.13 Migration import 完整** —— MCP import 已有；加 skill import / preferences import（从 Claude Desktop）。**Done when**：`:import-claude-desktop` endpoint 一键全套迁移。
- ☐ **§15.14 Telemetry / crash reports**（可选）—— 用户允许时发匿名 crash log。**Done when**：opt-in flow + Sentry-like 后端 / 或 disable。
- ☐ **§15.15 Privacy / data export** —— 用户随时导出全部数据（`~/.forgify/` zip）。**Done when**：跟 §15.11 backup 共用机制。
- ☐ **§15.16 Light / dark theme** —— testend 当前单一 theme。**Done when**：CSS variables + theme switcher + 跟系统 prefers-color-scheme。

---

## 16. 桌面 app 准备（非 Wails UI 部分）

**背景**：Wails 整体迁移不在本轮，但桌面壳需要的**后端 prep 工作**可以做。

- ☐ **§16.1 `Notifier` 接口** —— `domain/notification/Notifier`：`Notify(title, body, urgency)`。scheduler / approval / mcp_server failed 等触发。生产实现：Wails 桌面通知；dev / 测试：no-op。**Done when**：接口 + 注入 scheduler + 1 个 noop / 1 个 logger 实现。
- ☐ **§16.2 Scheduler 不退出** —— 关窗 ≠ 退出进程；scheduler / trigger / sandbox 继续跑。**Done when**：当前已经是 ctx.Background goroutine 模式；显式加 graceful-shutdown only on SIGTERM 测试。
- ☐ **§16.3 `cmd/desktop` 入口** —— Wails 用的入口（不含 UI 代码）；reuse `cmd/server` 装配但加 `internal/infra/desktop/` 桥接。**Done when**：`cmd/desktop/main.go` skeleton + 编译过。
- ☐ **§16.4 Preferences service** —— `Preferences{startOnLogin, missedTaskPolicy, defaultLocale, theme, ...}`；落 SQLite single-row。**Done when**：CRUD endpoint + schema。
- ☐ **§16.5 Binary 打包流程** —— `make build-all` 5 平台编 binary；`make package-darwin` / `package-linux` / `package-windows` 包 mise embed + 打 .tar.gz / .zip。**Done when**：CI 跑通 + artifact 存档。
- ☐ **§16.6 Auto-update mechanism** —— `cmd/server --check-update` 查 GitHub releases；testend 提示有新版本。**Done when**：endpoint + UI + manual update。
- ☐ **§16.7 Tray icon API（hooks）** —— Wails 接 tray 通过 HTTP API：`GET /api/v1/tray/badge` 返"待批准 3 / 报错 1"。**Done when**：endpoint 返结构化 + Wails 端 polling。

---

## 17. Burn-in 剩余 + 杂项

**背景**：burn-in 还有 #7 / #11 / #15-18 这几个；#15-18 已经在 §11-12 中拆出来；剩 #7 / #11。

- ☐ **§17.1 Burn-in #7 — LLM tool description 问题** —— 翻 progress-record 找具体描述（2026-05-15 burn-in 行）。**Done when**：定位 + 修。
- ☐ **§17.2 Burn-in #11 — NodeType 设计 issue** —— 同上。**Done when**：定位 + 修。
- ☐ **§17.3 Conversation queue full 时的 backpressure** —— `runner.go` 当前 buffered chan(5)；满了用户发新 message 会 block。加 explicit error 返"对话繁忙，请等等"。**Done when**：queue 满 → 立刻返 409 CHAT_BUSY + 前端 retry hint。
- ☐ **§17.4 Conversation 长期 idle 后清理** —— `agentstate` 跟 conversation 一起 GC 时机不清晰。**Done when**：30 天 idle conv 触发 agentstate 释放；测试。
- ☐ **§17.5 Bash AST walk 边界** —— `mvdan.cc/sh/v3` AST 覆盖 95%；`eval` / `source` / 动态 `$()` 仍可逃逸（已 Description 警告但 detect 漏）。**Done when**：扫描 AST 含 `EvalExpr` / `CmdSubst` / `SourceExpr` → fallback 走 plain shell（不 auto-route）+ warn。
- ☐ **§17.6 Catalog cache lock contention** —— `catalog.Refresh` 持 sourcesMu RWMutex + 后续 LLM 调用持续到完成。LLM 慢时 lock contention。**Done when**：拉数据后立刻 release lock，LLM 调用不持锁。
- ☐ **§17.7 MCP server 重启策略 review** —— 当前 fail-loud（不自动 restart）。但 transient crash（如 OOM）用户没法自动恢复；考虑加 `restartOnCrash: {maxAttempts: 3, backoffSec: 5}` 配置。**Done when**：mcp.json schema 加字段 + 实现 + 测试；保留默认 false 不破当前 fail-loud 哲学。
- ☐ **§17.8 Skill body cache TTL** —— 当前 Activate 时每次都 ReadFile。100ms ENOENT 重试已有但不缓存。可加 ETag-style cache（last-modified ≤ 1s 直接用 cache）。**Done when**：低优先，仅当 burn-in 撞到才做。
- ☐ **§17.9 Forge_executions retention** —— `function_executions` / `handler_calls` 等表当前无 retention。长跑后膨胀。**Done when**：scheduler.finalizeRun 后异步 trim oldest（按 conv / per-entity）+ 默认 keep N=1000 / per-entity。
- ☐ **§17.10 Sandbox env GC schedule** —— 当前手动触发 `:gc`。加 weekly auto-GC（lastUsedAt > 30 天）。**Done when**：cron-like internal scheduler + opt-out 配置。
- ☐ **§17.11 Sub-Message status drift 自检** —— `mapEventLogStatus` default 分支 Warn log。如果真触发说明 chatdomain 加新 Status 但 subagent 漏 sync。**Done when**：契约测试断言所有 chatdomain.Status* 都在 switch 覆盖内。
- ☐ **§17.12 Conversation 加 `archived: bool` 字段** —— 长对话归档（不删 + 不在主列表显示）。**Done when**：schema + endpoint + UI 切换。
- ☐ **§17.13 SearXNG 三层 fallback 失败后 LLM 不知道** —— 当前 web_search 三层都挂返 friendly error，但 LLM 看到不知道为啥（公网搜索全断？）。加更明确的 hint（"all 3 search backends failed; check network or use a BYOK provider"）。**Done when**：error 信息含 hint + testend "搜索健康度" 指示器联动。

---

## 18. 推荐 ordering

按 **用户感知 × ROI** 排，挑短的先做：

### Day 1（半天起步，立刻见效）

1. **§2.1 FORGIFY.md** + **§2.2 MEMORY.md index** + **§2.3-2.5 read/write/forget memory tools**（半天-1 天，UX 跃升）
2. **§4.1 Token 计数** + **§4.2 总累计 cost 面板**（半天）
3. **§12.1 limit cap** + **§11.2 ownerKind 白名单**（小坑，1 小时各）

### Day 2-3（关键能力）

4. **§1.1 Token counter** + **§1.2 MicroCompact** + **§1.6 Tool result budget**（1-2 天，长任务不再死）
5. **§17.3 queue 满 backpressure** + **§13.1 LLM withRetry** + **§13.5 worker timeout**（可靠性 1 天）
6. **§11.1 env lazy-rebuild** + **§11.3 runtime dedup** + **§11.4 Ruby/PHP**（sandbox 收尾，1 天）

### Day 4-7（结构性升级）

7. **§3.1 PermissionLevel** + **§3.2 Protected paths** + **§3.3 Settings allow/deny**（权限基线，2 天）
8. **§3.4-3.6 Hooks 正式化**（2-3 天）
9. **§10.1 Checkpoint / Undo** + **§10.2 File diff**（user trust 关键，2 天）
10. **§14.1 sqlite-vec spike**（半天 spike）→ pass: 进 **§14.2-14.5 Knowledge / RAG**

### Day 7+（高阶能力）

11. **§5.1 Loop body** + **§5.2 Parallel branches**（Workflow 完整，3-5 天）
12. **§14.6-14.7 Intent routing + chat 终极版**（Phase 5 收尾，3-5 天）
13. **§1.3 AutoCompact**（1-2 周，复杂）

### "polished" 项（穿插着做）

每天挑 1-2 个 §15.x（onboarding / sample bundles / settings pane / pinning / tags / themes / shortcuts / error i18n）。

### 远期 / 真留下次

- **§7.5 Plugin 形态加载**（生态成熟前别引）
- **§8.5 Worktree-isolated subagent**（无强需求）
- **§6.5-6.7 MCP Resources / Prompts / Sampling**（spec 有但几乎没人用）
- **§2.7 autoDream**（先看用户怎么用 memory）

---

## 完工标准

V1.2 ship 标准（前端 Wails 之外）：

- ☑ 后端 + testend 95%（已达成）
- ☐ §1（Compaction）+ §2（Memory）+ §3（Hooks 基线）—— "AI 真的不傻"基线
- ☐ §11-12（sandbox / API 收尾）+ §13（可靠性）—— 不易崩
- ☐ §14（Phase 5 智能化）—— 愿景核心
- ☐ §10.1（Checkpoint）+ §15.1-15.5（onboarding 基本套件）—— 用户敢用

剩下的 §5-9 / §15 / §17 大部分可以 **v1.2.1 / v1.2.2 patch release** 持续迭代。

---

> **维护**：做完一项划 `☑` + git commit message 引用 §N.M；新发现 gap 直接 append 到本文档同节末尾。每周 review 一次 ordering。
