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

- ☑ **§1.1 Token counter** ✅ 2026-05-16 —— 经 §4.1 落地：`GET /api/v1/conversations/{id}` 返 `tokensUsed: {input, output, total}`，`chatdomain.Repository.SumTokensByConversation` SQL SUM。
- ☑ **§1.2 MicroCompact**（Layer 1）✅ 2026-05-16 —— `app/contextmgr.demoteOldBlocks` Soft 阈 0.70 触发；老 tool_result `context_role` 标 `warm`（200B preview）/ `cold`（仅元数据占位）；DB Content 永不改写，只投影换形态。
- ☑ **§1.3 AutoCompact**（Layer 4）✅ 2026-05-16 —— `app/contextmgr.fullCompact` Hard 阈 0.85 触发；cheap LLM 生成 anchored-merge 摘要 → `conversations.summary` 持续累加；候选 block 标 `archived`。摘要落 `conversation.summary` 列（非 `compacted_messages` 表）+ 新 block type `compaction` 留 audit。
- ☐ **§1.4 手动 `/compact [focus]` 命令** —— 用户输入触发 AutoCompact，可选 focus 字符串聚焦。**Done when**：testend 输入框打 `/compact auth-refactor` 立刻触发并 stream 进度。**注**：`Manager.ForceCompact` 已实现，仅缺 HTTP 端点 + slash command 路由。
- ☐ **§1.5 System prompt 缓存边界** —— `SYSTEM_PROMPT_DYNAMIC_BOUNDARY` 分割：静态段（tool defs / FORGIFY.md / catalog）+ 动态段（locale / now / git status）。**Done when**：静态前缀命中 Anthropic prompt cache，token 费用降 ~60% per turn。
- ☐ **§1.6 Tool result budget** —— `applyToolResultBudget` ——单次 tool_result 超 N tokens 自动截断 + 追加 `[output truncated; original size: X KB]`。**Done when**：`Bash` / `Read` 大输出自动截断；前端能查看完整版（`/api/v1/blocks/{id}` 已有）。
- ☐ **§1.7 `/context` 命令** —— 显示当前 context 占用分布（system prompt / history / catalog / each tool def）。**Done when**：testend 一键查看。

---

## 2. 记忆系统（Memory）

**背景**：当前 `conversation.SystemPrompt` 字段是用户手动写。**跨会话记忆完全没有**——用户每开新对话都得重新教"我用 Python / 不要写中文注释 / 忽略 .venv"。

- 🔄 **§2.1 `FORGIFY.md` 自动注入** ✅ 2026-05-16（**设计改向：DB-backed memories 替代磁盘文件**）—— Pinned memories（type=user / feedback）功能等价 FORGIFY.md，但状态在 `memories` 表而非 `~/.forgify/FORGIFY.md`，所有写入经 service 而非直接编辑文件；用户改动经 testend 立刻生效。**取舍**：DB 给 audit / CRUD / per-user 隔离更好；不再需要磁盘 watch。
- 🔄 **§2.2 `~/.forgify/memory/` 目录 + `MEMORY.md` 索引** ✅ 2026-05-16（**同上替为 DB**）—— Non-pinned memories 等价 MEMORY.md index：`memoryapp.ForSystemPrompt` 渲染 200 行 markdown 索引段。
- ☑ **§2.3 `read_memory` tool** ✅ 2026-05-16 —— `app/tool/memory/read.go` tool 接口 9 方法齐全 + 测试。
- ☑ **§2.4 `write_memory` tool** ✅ 2026-05-16 —— `app/tool/memory/write.go` 走 service Upsert（atomic DB write），自动 publish `memory` notif。
- ☑ **§2.5 `forget_memory` tool** ✅ 2026-05-16 —— `app/tool/memory/forget.go` 软删 + 通知。LLM 主动调（user 经 testend UI 也可）。
- ☑ **§2.6 4 类 memory types** ✅ 2026-05-16 —— `user` / `feedback` / `project` / `reference` 4 值 CHECK 约束（DB 层强制）；schema 见 `service-design-documents/memory.md`。
- ☐ **§2.7 autoDream**（远期）—— idle 24h + 5+ session 后，后台 subagent 整合 memory（dedup / 合并近义 / prune 旧的）。**Done when**：optional Phase 5+ 功能；本轮可选先 skip。

---

## 3. 权限 + Hooks

**背景**：当前权限薄——PathGuard 守敏感路径（filesystem tools），Bash 故意不走 PathGuard（per CLAUDE.md decisión D5），没有 PermissionLevel / Stop Hook / 正式 Hook API。Phase 4 workflow 自动执行后风险升高。

- ☑ **§3.1 Tool 级 `PermissionLevel()` 方法** ✅ 2026-05-16 —— **设计改向：框架硬编码而非 Tool 接口第 10 方法**（抄 Claude Code 模式）。`app/tool/permissionsgate/levels.go` 静态 map 登记 56 tool；`LookupLevel` 兜底 `Tool.IsReadOnly()`。
- ☑ **§3.2 Protected paths** ✅ 2026-05-16 —— `pkg/pathguard` 加 `AllowWrite` 方法 + `DefaultWriteOnlyExtras`（`.git/` / `.env*` / `.envrc` / `node_modules/` / `.venv/` / `~/.ssh/`）。读 deny 与写 deny 分离，写=读∪写专属。
- ☑ **§3.3 Settings allow/deny 规则** ✅ 2026-05-16 —— `~/.forgify/settings.json` permissions.deny/ask/allow + defaultMode（ask/allow/deny/bypass）；deny→ask→allow→default 第一匹配。`infra/settings` 经 fsnotify watch + 5s poll 兜底热加载（**比原计划 1s polling 更快/更准**）。
- ☑ **§3.4 Stop Hook** ✅ 2026-05-16 —— `hooks.Runner.FireStop` + Stop 时机；hook 返 `decision: "continue"` + Reason 注入下轮 prompt。**注**：chat runner 集成 Stop 触发点仍可补强（当前接口已就位，自动 fire 路径未接 finalPersist）。
- ☑ **§3.5 PreToolUse hook 正式化** ✅ 2026-05-16 —— `app/hooks` shell exec + stdin/stdout JSON 协议；exit 0 解析 / exit 2 blocking deny / 其他 nonblocking。**注**：HTTP / MCP form 留 v2（设计 doc §6.6 已说明）。
- ☑ **§3.6 PostToolUse hook 正式化** ✅ 2026-05-16 —— 同上 `FirePostToolUse`；hook 输出 `injectIntoNextTurn` 拼到 tool_result 当 `[hook] <hint>` 附录，下轮 LLM 看到。
- ☐ **§3.7 SessionStart / SessionEnd hook** —— 对话开 / 关时回调。**Done when**：SessionStart 跑一次（init / 注入 context）；SessionEnd 跑一次（cleanup / log）。
- ☐ **§3.8 Permission Explainer** —— 高风险操作前单独 LLM 调用解释风险（"rm -rf 会删整个项目，确定？"）。**Done when**：destructive=true 的 tool call 前显式 ask user 通过 AskUserQuestion 风格 UI。**部分**：destructive=true 当前强制走 ask 路径（V1.2 MVP 自动批准 + 日志），但 LLM explainer 风格未做。
- ☐ **§3.9 Path traversal 防御加强** —— PathGuard 加 URL 解码 / Unicode normalization / backslash injection / 大小写 bypass 检测。**Done when**：覆盖 OWASP path traversal 常见 bypass。

---

## 4. 可观测性

**背景**：后端记录够全，但前端 / API 没暴露。"花了多少钱 / 找历史对话 / 调试 LLM 输出" 这些日常需求都没好用工具。

- ☑ **§4.1 Token / cost per-conversation 显示** ✅ 2026-05-16 —— `GET /api/v1/conversations/{id}` 响应附 `tokensUsed: {input, output, total}`；`SumTokensByConversation` 单 SQL SUM 聚合。**注**：每 conv 的 cost 估算需要 model breakdown，走 §4.2 端点（per-conv 模式无 cost 字段，仅 totals）。
- ☑ **§4.2 总累计 cost 面板** ✅ 2026-05-16 —— `GET /api/v1/usage?conversationId=…\|period=day\|week\|month\|all`，按 (provider, modelId) 拆 + cost 估算；新 `pkg/llmcost` 16-model 单价 registry。**未做**：testend `/usage` 视图（仅后端端点，UI 留下次）。
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

- ☐ **§9.1 Document source** —— Phase 5 document domain 起，加 `documentapp.AsCatalogSource()`（同 §14.4，从 catalog 侧视角列在此供索引；实际工作随 §14.4 一起做）。**Done when**：document 实体（§14.1）出来后自动接 catalog，0 行修改。
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
- ☑ **§11.2 ownerKind 非白名单显式拒**（burn-in #16）✅ 2026-05-15 —— P3 batch 顺手修：`validOwnerKinds` 5 值白名单 + 400 `INVALID_OWNER_KIND`，告别"空 list 当无数据"误读。
- ☐ **§11.3 runtime 表 `3.12` vs `>=3.12` dedup**（burn-in #17）—— 两个版本说同一件事，但是 DB UNIQUE 让它们各占一行。**Done when**：Install 时解析 PEP 440 specifier → 取实际 install 版本（mise where）作 UNIQUE key；旧行迁移。
- ☐ **§11.4 Ruby / PHP EnvManager 修 bundler / composer**（D2 遗留）—— bundler / composer 不在 mise registry，Ruby/PHP EnvManager 调 EnsureTool 会失败。**Done when**：直接静态装 bundler / composer（github release 二进制）；EnvManager 用之；测试。
- ☐ **§11.5 env corruption 自动重装** —— `.venv/bin/python` 缺失（mise upgrade 把 install dir 改了）→ 自动 Sync 重建。**Done when**：Spawn 前 check 关键 binary 存在 → 不在则 Sync；测试。
- ☐ **§11.6 Docker runtime support** —— sandbox.md §4 提过留待 v1.5。**Done when**：`infra/sandbox/docker.go` Installer + EnvManager；mcp 等可选 docker runtime；ErrDockerNotInstalled / ErrDockerDaemonDown 已有 sentinel。
- ☐ **§11.7 Disk usage warn** —— sandbox 共享 cache 长跑后磁盘膨胀。`TotalSizeBytes > 5GB` 时 testend warn + 提示 `:gc`。**Done when**：可视化 + 阈值通知。

---

## 12. HTTP / API 收尾

**背景**：Plan 05 14 项 hardening 完成，但 burn-in #18 + 几个收尾还在。

- ☑ **§12.1 Pagination limit cap**（burn-in #18）✅ 2026-05-15 —— 审计发现 `pkg/pagination.Parse` 早已 cap 到 `MaxLimit=200`；负数/非数字 400 INVALID_REQUEST；handler 全经 Parse 单入口，无直读 `?limit`。**实现到位**（误判为遗留）。
- ☑ **§12.2 HTTP request retry (LLM client)** ✅ 2026-05-16 —— `infra/llm.Generate` 套 `withRetry`（3 attempts，500ms → 1.5s 指数退避）；`isRetryable` 白名单 `ErrRateLimited` / `ErrProviderError` / `context.DeadlineExceeded`；`ErrAuthFailed` / `ErrBadRequest` / `context.Canceled` 不重试。**注**：仅 `Generate`（非流式）有 retry —— `Stream`（chat 主路径）不重试，避免 mid-stream retry 丢已见内容；7 单测覆盖。
- ☐ **§12.3 Per-conversation model override** —— 当前全局 model_configs；某对话想用别的 model 没接口。**Done when**：Conversation 加 `modelOverride: {provider, modelId}` 字段；PickForChat 检查 conv 字段优先。
- ☐ **§12.4 Webhook 多 HMAC 算法** —— 当前 secret 单字符串。GitHub / GitLab 等用 HMAC-SHA256 不同 header 名。**Done when**：spec 加 `"signatureAlgo": "hmac-sha256", "signatureHeader": "X-Hub-Signature-256"`。

---

## 13. 可靠性 / 故障恢复

- ☑ **§13.1 LLM client withRetry**（同 §12.2，单列因为这是可靠性核心）✅ 2026-05-16 —— 见 §12.2。
- ☐ **§13.2 LLM provider auto-failover** —— 单 provider 持续失败 → 自动切到 fallback provider（用户配的）。**Done when**：apikey 配 `fallbackProvider` 字段；chat resolver 实现切换。
- ☐ **§13.3 SearXNG instance 健康度跟踪** —— `app/tool/web/search.go` 三层 fallback，但 SearXNG 池中坏实例不剔除。**Done when**：连续失败 ≥ 3 次的 instance 30 分钟内不用。
- ☐ **§13.4 BackgroundCtx 死亡检测** —— catalog / skill / mcp polling 用 background ctx；进程关 ctx 不取消（OS 回收）。但万一卡死 goroutine 不退。**Done when**：Stop() 方法 + main.go shutdown 显式调 + 测试。
- ☑ **§13.5 Conversation queue 长任务 worker timeout** ✅ 2026-05-16 —— `chat.runner.processTask` 加 `context.WithTimeout(maxTurnDuration=10min)`；超时 loop.Run 下步退，`host.WriteFinalize` 落 `cancelled/timeout` 终态。**注**：用 10 min（非原计划 30 min）—— 单 chat turn 10 min 已经够大；超此基本是 bug 而非用户期望。

---

## 14. Phase 5 真正没做的

**背景**：愿景里 Phase 5 = 文档库 + intent routing + chat 终极版。当前都 ⬜。

**设计改向 2026-05-16 — 弃 RAG / sqlite-vec，改 Notion-style 树状文档库 + LLM-ranked attach**：抄 forge / skill / mcp catalog 套路，不引向量检索；文档组织抄 Notion（单表自引用 + 树状嵌套 + 全 markdown）。五条理由：

1. **本地单用户文档量是人类规模**（几十到几百份），向量索引过度工程。同体量 50-200 份"能力载体"在 forge / skill / mcp 三个 domain 已经用 LLM 排序跑得很好。
2. **2026-04-26 已有先例**（progress-record 行 100）：把 tool search 从 chromem-go 向量库切到 LLM 排序，删了 `infra/vectordb/`。同一推理用在 document 上完全成立。
3. **现代大 context + prompt cache 让"塞全文"反超 RAG**：Sonnet 4.6/4.7 = 200K，Opus 4.7 = 1M；Anthropic prompt cache 5min TTL，cache 命中省 90%。50K 文档第一轮贵，之后几乎免费。RAG 的"省 token"优势在 cache 时代缩水严重。
4. **用户实际场景是 deterministic routing 不是 similarity search**：用户描述的是"工作流决定 attach 哪个 doc"（规则路由）+ "agent 在 catalog 看可选项自己挑"（LLM 排序），两者都不需要向量。
5. **组织形态选 Notion-tree 而非 flat-with-sections**：用户实际心智模型是"大文档套小文档"（项目笔记 / API 文档树 / 日报树），不是"一份长 PDF 切章节"。单表自引用比独立 sections 子表语义更清晰；AI 也能用 create/move/delete 真正帮用户组织文档，不止读。

**结果**：sqlite-vec 闸门取消（modernc 不再需要加载 C 扩展），Phase 5 文档库工程量从 RAG 版的"上传切片向量化"减为"树形 CRUD + LLM 排序"，跨平台编译保住一行命令。详 [`service-design-documents/document.md`](./service-design-documents/document.md)。**未来场景**：如果真撞上"全公司 wiki 几千篇" / "GitHub repo 自动索引代码 chunk"这类**真正大规模 + 跨文档模糊查询**，再加向量层；document 表加 `embedding` 列、引向量库当二进制工具是可以平滑长出来的。

- ☑ **§14.1 `domain/document` 4 层 + 树状 DB schema** ✅ 2026-05-16 —— Document entity 4 层全到位（domain + store + app + AutoMigrate in main.go + harness.go）。Repository 含 ListByParent / GetBatch / Search / IsAncestor / SoftDeleteSubtree / CountChildren / CountDescendants / MaxSiblingPosition / UpdateBatch 树操作方法；Service 含 Create / Get / List / Update（rename 时 path 子树级联）/ Move（成环检测 + parent path cascade）/ Delete（递归软删）/ Search。6 sentinel（`ErrNotFound` / `ErrInvalidParent` / `ErrNameConflict` / `ErrContentTooLarge` / `ErrInvalidName` / `ErrParentNotFound`）。schema_extras partial UNIQUE `(user_id, COALESCE(parent_id, ''), name) WHERE deleted_at IS NULL`（COALESCE 让 root 级同名也撞 UNIQUE）。**测试 19 + 5 + 4 全绿**（store 19 / app 13 / domain 4）；全量 `make test-unit` 不影响其他 domain。
- 🟡 **§14.2 HTTP API + testend Notion-style 侧边栏 UI** —— **后端 7 端点 ✅ 2026-05-16**（GET list / tree / id, POST create / `:move`, PATCH, DELETE 全到位 + 6 errmap sentinel + 13 httptest 全绿）。**testend 烟雾层 ✅ 2026-05-16**（`api/resources.ts::documentAPI` 7 方法 + `Document` 类型 + `'doc'` IDPrefix + `views/config/Documents.vue` 扁平表 with create/edit/move/delete + `/config/documents` 路由 + nav 入口；vue-tsc 干净）。**Notion 树 + Monaco 编辑器 + 拖拽 reorganize 留 §14.5**。
- ☑ **§14.3 7 个 system tool**（让 AI 真能组织文档）✅ 2026-05-17 —— `app/tool/document/` 8 文件（document.go 工厂 + 7 个 tool 文件）。3 ReadOnly（search/list/read）+ 4 WorkspaceWrite（create/edit/move/delete）；每个 tool 接口 9 方法齐 + permissionsgate `toolLevels` 7 项登记 + chat agent ToolRegistry 注入（main.go + harness.go 同步）。Tool 内部 graceful 把 domain sentinel 翻译成给 LLM 看的友好文本（ErrNotFound / ErrNameConflict / ErrInvalidParent / ErrContentTooLarge）不直接抛错——LLM 拿到可解释的 tool_result 自我纠错。**测试 22 单测 + 2 pipeline test（agent create_document 真落库 + agent read_document → edit_document 真持久化）全绿**；staticcheck 干净；make test-unit 通。**注**：search_documents 暂用 SQL LIKE 实现（per Service.Search），design doc 提的"LLM-ranked"模式留下次（V1 LIKE 够用，撞性能问题再加 LLM rerank 层）。`delete_document` destructive 标记由 LLM 自报 + §3 permissions destructive=true 自动 ask 路径处理（tool 自身静态 ReadOnly=false，destructive 是 per-call LLM-supplied standard field）。
- ☑ **§14.4 Catalog 第 4 source — `documentapp.AsCatalogSource()`** ✅ 2026-05-17 —— `app/document/catalog_source.go` 50 行；CatalogSource impl 返 Source=`"document"` / Name=`Path`（让 LLM 一眼看树位置）/ Description（fallback 到 tags → "(no description)"）/ **Category=top-level segment**（让 mechanical / LLM generator 按 path 分组）。已注册到 main.go + harness.go 的 catalog Service（3 source → 4 source）。**Notification → invalidate hook 不需要做**：catalog 已用 1s polling + fingerprint dedup，document Create/Edit/Move/Delete 改动会被下一次 polling cycle 自动捡到——hook 是 design doc 笔误，polling 路径已够用。**4 单测 + 1 pipeline test（document_catalog_test.go：seed 树 → Refresh → 验 Coverage["document"] + Summary 含 path）全绿**；catalog 全 suite 47s 通；make test-unit + staticcheck + Windows 交叉编译都干净。
- ☐ **§14.5 Workflow + Conversation 接入 + testend Notion UI**（设计调整 2026-05-16，**拆 4 子件**——见下方）。**核心改动**：(1) 增加第 14 种节点类型 `agent`（agentic loop with tools，跟 `llm` 单次区分成本心智）；(2) `AttachedDocuments` schema 用 `{documentId, includeSubtree?}` 而非 flat ID list——挂"整 Notebook"live-resolve 跟随用户加新 doc 自动包含。
  - ☐ **§14.5a `llm` 节点 AttachedDocuments**（0.5 天）—— `LLMNodeConfig.AttachedDocuments: []AttachedDocument{DocumentID, IncludeSubtree}` + `documentapp.ResolveAttached`（live-resolve subtree，调 Repository.ListSubtreeIDs）+ dispatch_llm.go buildDocsPrefix + workflow.validate capability check。
  - ☐ **§14.5b 新增 `agent` 节点（14th NodeType）**（1 天）—— domain.NodeType 加 `agent` + `dispatch_agent.go` 用 `app/loop.Run` + 全套 system tool registry 注入（含 7 个 document tool + filesystem + bash + web + MCP + skill）+ AgentNodeConfig（Prompt / AttachedDocuments / EnabledTools / MaxTurns 默认 10）+ scheduler.Router 注册 + 6 单测 + 1 pipeline test（workflow `agent` 节点创/编辑 doc 端到端）。**这是写 doc 的官方路径**——dispatch_llm 仍 single-shot 无 tool。
  - ☐ **§14.5c Conversation.AttachedDocuments + chat prepend**（0.5 天）—— Conversation schema 同 `AttachedDocument` struct + chat runner.buildSystemPrompt 调 ResolveAttached 前置 `<documents>` 段 + cross-domain `CONVERSATION_DOCUMENT_NOT_FOUND`。
  - ☐ **§14.5d testend Notion 树 + Monaco + Conv 挂载下拉**（1.2 天）—— 侧边栏树状 UI（拖拽 reorganize + 右键 menu）+ Monaco/CodeMirror markdown 编辑器 + 对话视图"挂载文档"面板（subtree toggle + 子节点 preview 灰显 + token 估算）+ workflow 节点 attach UI。**Done when**：4 子件全到，5 pipeline test（`llm` attached / `agent` 写 doc / chat 挂载 / 跨对话切换 / 工作流 capability check 拒绝缺失 doc）。

> **设计 pivot 2026-05-16**：(1) 弃原"不新增节点类型"承诺——拆 `llm`（单次 + 挂知识库）vs `agent`（agentic + tools）让成本可见性 first-class。(2) AttachedDocuments live-resolve subtree：用户挂"项目根 + 子树"后续加新文档自动跟上，不用回头编辑 workflow / Conv。详 [`service-design-documents/document.md`](./service-design-documents/document.md) §9。
- ☐ **§14.6 Intent routing** —— 主 chat LLM 第一步识别用户意图（"创建工作流" vs "改工具" vs "查文档" vs "纯问答"）→ 注入不同 system prompt sub-section。**Done when**：runner.buildSystemPrompt 加 intent 段；前 N turn 走 intent agent。
- ☐ **§14.7 Chat 终极版 — 工作流推荐 + 自动建草稿** —— 用户描述需求 → AI 推荐用哪个 workflow / forge / skill / document → 自动 draft 一个 workflow → ask user 确认。**Done when**：multi-agent forging system prompt 已经在 F2 教过；加 workflow draft 自动 spawn create_workflow 工具。

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
- ☑ **§17.3 Conversation queue full 时的 backpressure** ✅ 2026-05-16 —— 审计验证：现状 `select` + `default` 已经直接返 409 `STREAM_IN_PROGRESS`（buffered chan(5) 满时），无 block / panic 风险。**实现到位**（误判为遗留；名字 STREAM_IN_PROGRESS 与"queue full"语义稍有重叠，但 V1.2 单用户本地几乎撞不到 6+1=7 队，过度区分是反校验剧场）。
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
10. **§14.1-14.5 Document domain + tools + workflow 节点**（LLM-ranked attach 模式，无向量库 / 无 sqlite-vec / 无 chunking pipeline；2-3 天）

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

---

## 19. Tracker（进度看板）

**用法**：
- 每完成一批，**就来这里**：(a) 上面把对应 ☐ 翻成 ☑（带 ✅ 日期）；(b) 总览表更新数字；(c) "日志" 追一条。
- ☐ 未做 / 🔄 设计改向已等价完成 / 部分 = 主体完工有遗留 / ☑ 完工。

### 19.1 总览（按节聚合）

| § | 节标题 | 已完 / 总数 | 状态 | 备注 |
|---|---|---|---|---|
| §1 | 长任务能力（Compaction）| 3 / 7 | 🟢 主体完工 | 核心 3 件（counter / Micro / Auto）做完；slash / cache 边界 / tool budget 留下次 |
| §2 | 记忆系统（Memory）| 6 / 7 | 🟢 主体完工 | §2.1+§2.2 设计改向：DB-backed 替代磁盘 FORGIFY.md / MEMORY.md（pinned + index 同语义）；§2.7 autoDream "本轮 skip" 远期 |
| §3 | 权限 + Hooks | 6 / 9 | 🟢 主体完工 | §3.7 Session hooks / §3.8 Explainer / §3.9 path traversal 加固 留 v2 |
| §4 | 可观测性 | 2 / 10 | 🟡 起步 | §4.1 + §4.2 后端就位；testend `/usage` UI 留下次 |
| §5 | Workflow 高阶 | 0 / 7 | ⬜ 未起 | Phase 4 主体已完，本节是 V1.5 deferred 全做 |
| §6 | MCP 高阶 | 0 / 7 | ⬜ 未起 | |
| §7 | Skill 高阶 | 0 / 5 | ⬜ 未起 | |
| §8 | Subagent 高阶 | 0 / 5 | ⬜ 未起 | |
| §9 | Catalog 高阶 | 0 / 5 | ⬜ 未起 | |
| §10 | UX 工具完善 | 0 / 7 | ⬜ 未起 | §10.1 Checkpoint/Undo 是 ship gate "用户敢用"关键 |
| §11 | Sandbox 收尾 + 修复 | 1 / 7 | 🟡 起步 | §11.2 已修；§11.1 / §11.3-7 待 |
| §12 | HTTP / API 收尾 | 2 / 4 | 🟢 主体完工 | §12.3 + §12.4 留下次 |
| §13 | 可靠性 / 故障恢复 | 2 / 5 | 🟢 主体完工 | retry + timeout 两件核心已完；failover / SearXNG health / BackgroundCtx 留 |
| §14 | Phase 5 真正没做的 | 4 / 7 | 🟡 主体推进中 | 愿景核心；§14.1+§14.2+§14.3+§14.4 全交付（domain/store/HTTP/testend smoke + 7 tools + catalog 4th source，~80 测试全绿）；剩 §14.5 拆 4 子件 ~3 天（llm 节点 + agent 节点 + Conv attach + testend Notion 树 UI），§14.6-7 是 Phase 5 智能化收尾。详 [`service-design-documents/document.md`](./service-design-documents/document.md) |
| §15 | 完美产品 UX | 0 / 16 | ⬜ 未起 | onboarding / settings pane / themes / 等等；穿插着做 |
| §16 | 桌面 app 准备 | 0 / 7 | ⬜ 未起 | Wails 主迁移前的后端 prep |
| §17 | Burn-in 剩余 + 杂项 | 1 / 13 | 🟡 起步 | §17.3 已审计；§17.1+§17.2 burn-in #7/#11 + §17.4-13 待 |

**Ship gate 进度（`完工标准` 节）**：

- ☑ §1 + §2 + §3 — **"AI 真的不傻"基线**（✅ 2026-05-16 全过）
- 🟢 §11-12 (sandbox / API 收尾) + §13 (可靠性) — **不易崩**（主体过，残留低优）
- ⬜ §14 (Phase 5 智能化) — **愿景核心**
- ⬜ §10.1 (Checkpoint) + §15.1-15.5 (onboarding 基本套件) — **用户敢用**

### 19.2 日志

| 日期 | 范围 | 摘要 |
|---|---|---|
| 2026-05-16 | **§14.5 设计调整 #3 — 拆 `llm`/`agent` 节点 + subtree live-resolve** | (1) **拆 LLM 节点**为两种——`llm`（单次 + 挂知识库 = 原节点扩 AttachedDocuments）+ **新增 14th NodeType `agent`**（agentic loop with full system tool registry，含 7 个 document tool + filesystem + bash + web + MCP + skill）。**理由**：成本可见性——`llm` ≈ 1 LLM call / $0.001-0.01，`agent` ≈ 1-N call / $0.01-0.10+。写 doc 必走 `agent` 节点。**架构现实校验**：grep `dispatch_llm.go` 当前是 `LLMCaller.Generate` 单次,no tool,backend-design L209 那句"workflow LLM 是 app/loop 调用方"是设计意图未实现——故 `agent` 节点而非扩 `llm` 节点是正确路径。(2) **AttachedDocuments live-resolve subtree**：schema 改 `{documentId, includeSubtree?}` 替 flat ID list；用户挂"整 Notebook"后续加新 doc 自动跟上。共用 `documentapp.ResolveAttached` resolver；Repository expose public `ListSubtreeIDs`。(3) **Conversation 复用同 struct**——chat / workflow `llm` / workflow `agent` 三入口共享 schema。**§14.5 拆 4 子件**：5a (`llm` AttachedDocs, 0.5d) / 5b (`agent` 节点, 1d) / 5c (Conv attach, 0.5d) / 5d (testend Notion UI, 1.2d) = ~3 天（原 1.5 天因加 agent 节点膨胀但换 first-class agentic 能力 + 写 doc 官方路径）。**同步**：final-sweep §14.5 子件清单 + document.md §9 重写（含 LLM vs agent 对比 + ResolveAttached + dispatch_agent.go 设计）+ §10 Conv schema + §14 实施顺序表 + §15 不变量。**纯设计 pivot 未写代码,§14.3 下一步开工**。|
| 2026-05-16 | **§14 设计改向 #2 — Notion-style 树状** | 在 no-RAG 决策之上进一步精化 document 数据模型：从"flat doc + 可选 section 子表"改为 **Notion-style 树状嵌套**（单表自引用 + ParentID + Position + Path 冗余字段）。**理由**：(1) 用户实际心智模型是"大文档套小文档"（项目笔记 / API 文档树 / 日报树）不是"PDF 切章节"；(2) 单表自引用比独立 sections 子表语义更清晰；(3) AI 能用 create / move / delete 真正帮用户组织文档不止读。**系统工具 2 个 → 7 个**：`search_documents` / `list_documents` / `read_document` / `create_document` / `edit_document` / `move_document` / `delete_document`；后 4 个 WorkspaceWrite，`delete_document` 自动 destructive=true 走 §3 permissions ask 路径。**Workflow 接入改向**：原计划新增第 14 种 `document` 节点类型 → 改为给现有 `llm` 节点 config 加 `AttachedDocumentIds: []string`，节点数仍 13 不爆炸；`Conversation` 同样加 `AttachedDocumentIDs` 让对话能挂文档库；前者 `dispatch_llm.go` 拼 `<documents>` 段前置 input，后者 `runner.buildSystemPrompt` prepend（跟 memory pinned 同一 cache-friendly 层）。**Catalog 接入**：按 path 分组（`- /Projects/2026/Q1 — Q1 计划`），>50 docs 自动 progressive disclosure。**新建 `service-design-documents/document.md`** 详设计 doc。**同步**：final-sweep §14 5 子项重写 + §19.1 tracker + backend-design 协作图（document subtree → 子节点 tree）+ progress-record §2 dev log。**纯设计 pivot，未写代码**。|
| 2026-05-16 | **§14 设计改向 (no-RAG)** | 弃 RAG / sqlite-vec / chunking / 向量检索 → 改 **LLM-ranked document attach** 模式（抄 forge / skill / mcp catalog 套路）。**理由**：(1) 本地单用户文档量人类规模（几十到几百），向量索引过度工程；(2) 2026-04-26 已有先例（progress-record 行 100，tool search 从 chromem-go 切到 LLM 排序）同一推理成立；(3) 大 context（Sonnet 4.6/4.7=200K，Opus 4.7=1M）+ Anthropic prompt cache（5min TTL，命中省 90%）让"塞全文"反超 RAG；(4) 用户场景是"工作流决定 attach 哪个 doc"——deterministic routing 不是 similarity search。**结果**：§14.1 sqlite-vec spike 取消（modernc 不再需要加载 C 扩展），§14.1-14.5 重设计为 `document` domain + 2 system tools（`search_documents` LLM-ranked + `read_document`）+ catalog 第 4 source + workflow `document` 节点类型，工程量减半，跨平台编译一行命令保住。**未来扩展**：真撞上"全公司 wiki 几千篇 / GitHub repo 自动索引代码 chunk"再加 `embedding` 列 + 向量库当二进制工具，平滑长出来。**同步**：final-sweep §14 全部 7 子项重写 + §18 ordering + §19.1 tracker；backend-design.md 能力清单 #4 + Phase 5 描述 + 跨 domain 协作图 + domain tree；progress-record §2 dev log。**纯设计 pivot，未写代码**。|
| 2026-05-16 | **§3 permissions + hooks + §4.1+4.2 + §12.2 + §13.1 + §13.5 + §17.3** | V1.2 ship gate 第三栏（"AI 真的不傻"）完工。新增 4 包（`domain/permissions` + `infra/settings` + `app/tool/permissionsgate` + `app/hooks`）+ 56 tool 危险等级登记 + glob→regex 翻译器 + shell hook stdin/stdout JSON 协议 + `pathguard.AllowWrite`（读写 deny 分离）+ 5 HTTP endpoints + testend `/config/permissions` 3 tab。30+ 新单测 + 3 pipeline test 全绿。同步：`service-design-documents/permissions.md`（18 节设计 doc）+ api-design + error-codes（INVALID_SETTINGS / BLOCKED_BY_RULE）+ progress-record dev log。**附带**：Token counter（§4.1）+ Usage 端点（§4.2，按 model 拆 + cost 估算 via 新 `pkg/llmcost` 16-model registry）+ LLM withRetry（§12.2 + §13.1，仅 Generate；Stream 不重试避免 mid-stream 丢内容）+ chat worker 10 min timeout（§13.5）+ queue backpressure 现状审计（§17.3 已实现到位）。|
| 2026-05-16 | **§1 compaction + §2 memory** | V1.2 ship gate 第一栏（"AI 真的不傻"基础）完工。新增 `domain/permissions` 仍待 §3；但 §1 + §2 两大跨对话能力一气落地：`pkg/modelmeta` + `pkg/tokencount` + `app/contextmgr.Manager`（3 路径：< Soft / Soft 降级 / Hard fullCompact）+ `conversations.summary` + `summary_covers_up_to_seq` 2 列 + `message_blocks.context_role` 1 列 + 新 block type `compaction`（eventlog 协议 6→7 种）+ `loop/history.BlocksToAssistantLLM` 按 role 投影 + chat.buildHistory 前置 `<conversation_summary>` wrapper；Memory 新 `domain/memory` + `app/memory` + 3 system tools + 7 HTTP endpoints + 3 errmap sentinel + testend `/config/memory` + `/current/compaction` 2 视图。4 + 3 = 7 pipeline tests 全绿。设计 doc：`service-design-documents/memory.md` + `compaction.md`。附带修旧债：harness Attrs migration / scheduler dotted edge / handler Python kwargs / cancel 422 接受 / chat_test seed-delete 模式。|
| 2026-05-15 | **§11.2 ownerKind 白名单 + §12.1 limit cap（审计）** | burn-in P3 batch 顺手修：`validOwnerKinds` 5 值白名单 + 400 INVALID_OWNER_KIND。同期审计 §12.1 发现 `pkg/pagination.Parse` 早已 cap 到 200，无 handler 直读 `?limit`——实现到位（误判为遗留）。|

> **下条记录待写**：sqlite-vec 闸门 2026-05-16 设计改向后已弃，下一站建议 **§10.1 Checkpoint/Undo**（~1 天 ship gate "用户敢用"）或直接进 **§14.1 Document domain**（Phase 5 愿景核心，LLM-ranked attach 模式，2-3 天）。§11 sandbox 残留低优，穿插着做。
