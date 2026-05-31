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
- ☑ **§4.3 Conversation 搜索** ✅ 2026-05-17 —— `conversation.ListFilter.Search` 字段 + SQL LIKE on `title`(V1;message content / tool 名等 FTS5 后续);handler 接 `?search=foo` 参数;testend `conv.refresh()` 自动带 filter。
- ☑ **§4.4 Conversation export (md / json)** ✅ 2026-05-17 —— `GET /api/v1/conversations/{id}/export?format=md|json`。md 渲染 user-readable(per-message heading + 6 block 类型差异化渲染 + token 注脚);json dump 完整 entity 数组。`Content-Disposition: attachment` headers。
- ☑ **§4.5 Tool execution metrics** ✅ 2026-05-17 —— `GET /api/v1/metrics/tools?since=7d` 聚合 4 张 D22 表(function_executions / handler_calls / mcp_calls / skill_executions),per-bucket 返 ok/failed/cancelled/timeout count + 成功率 + avg/p95 latency。`MetricsHandler` 新文件 + 4 repos 注入 Deps。`mcp.CallFilter` + `skill.ExecutionFilter` 加 `Since` / `Until` 字段(propagated to store applyFilter)。per-tool group-by 留 V1.5。
- ☑ **§4.6 LLM call trace export** ✅ 2026-05-17 —— `GET /api/v1/conversations/{id}/llm-trace` 列每个 assistant message 的 LLM 调用元数据(provider / modelId / token in/out / status / stopReason / errorCode / elapsedMs)。**纯派生 messages 表**,无新持久化。返 `{calls, totals}` 含全对话累计。
- ☑ **§4.7 Catalog version diff** ✅ 2026-05-17 —— 新 `catalog.HistoryEntry` domain + `catalog_history` 表(GORM AutoMigrate)+ `infra/store/cataloghistory` 新包(Save 含 trim-to-N=50 cap)。catalog Service `SetHistoryRepo` 接入,每次成功 Refresh 写一行。**2 新端点**:`GET /catalog/history?limit=N` 列近期版本 / `GET /catalog/diff?from=X&to=Y` 返 per-source added/removed item IDs。
- ☑ **§4.8 `/context` 端点** ✅ 2026-05-17 —— `GET /api/v1/conversations/{id}/context-stats` 估算每段 token 占用:catalogSummary / memorySection / conversationSystemPrompt / attachedDocuments(经 documentSvc.ResolveAttached + RenderAttachedAsXML 算入)+ history 总量(SumTokensForConversation)。`pkg/tokencount.Estimate` 字符级估算(CJK 1tok/char,ascii bytes/4)。
- ☑ **§4.9 `/usage` testend 视图** ✅ 2026-05-17 —— testend `/observe/usage` 视图,period 切 day/week/month/all + per-model 表 + cost 估算。包装现有 `/api/v1/usage`,新增 `usageAPI` client + nav 入口。
- ☑ **§4.10 LLM provider 健康指标** ✅ 2026-05-17 —— testend `/config/llm-health` 视图,按 provider group apikeys + worstStatus 显示 + 一键 test。`testStatus` 三态(ok / error / pending),lastTestedAt 显示 timeAgo。新增 `metricsAPI` / `catalogHistoryAPI` / `contextStatsAPI` clients 待相应 testend 视图复用(未来 polish)。

---

## 5. Workflow 高阶能力

**背景**：Plan 04 + Plan 05 trinity authoring + execution plane 全完工，但 V1.5 标记了几个 deferred。本轮全做。

- ☑ **§5.1 Loop body subgraph** ✅ 2026-05-17 —— scheduler 真 for-each + body 子图：`runReadyLoop` 抽出（与 finalizeRun 解耦）+ `ExecuteSubDAG` per-iteration 入口 + `SubDAGFromBody` map→Graph decoder + `SubstituteLoopTemplates` deep-walk 模板替换（`{{ .loop.item }}` / `{{ .loop.index }}` 全 config 树生效）+ `concurrentRun` 并发 helper。`LoopDispatcher` 真循环：sequential 默认 / `parallel: true` + `concurrency: N`（缺省 cap 5）/ `onError: stop` 默认 / `continue` opt-in（返 successes + failures 列表）。`flowrun_nodes` 表加 `parent_loop_node TEXT` + `iteration_index INT` 列；recordNode 自动 propagate。`ExecutionContext` 加 `Loop / ParentLoopNodeID / IterationIndex` 字段。Approval 节点在 body 内拒（V1 不支持中途暂停）。3 层嵌套上限沿用 validate.go 早有的 container-body 递归校验。**5 单测**：simple foreach sequential / fail-fast / continue / approval rejected / SubstituteLoopTemplates 嵌套。create_workflow tool description 加 loop body 完整例。同步 trigger.md / workflow.md。
- ☐ **§5.2 Parallel branches subgraph** —— 大部分场景已被 §5.1 fan-out + onError continue 覆盖；named branches 真增量是 per-branch retry/timeout（多用户场景才必要）。defer V1.5。
- ☐ **§5.3 `parallel(N)` concurrency** —— Workflow.Concurrency 当前只 `serial`。加 `parallel(N)`，允许同 workflow 同时跑 N 个 flowrun。**Done when**：CountRunning 检查改成 `< N`；schema 解析 `parallel(3)` 字符串。
- ☐ **§5.4 Per-trigger TZ override** —— 当前 cron 锁 `time.Local`。每个 trigger 加 `TimeZone` 字段。**Done when**：cron expression 旁可配 `"timeZone": "Asia/Tokyo"`。
- ☐ **§5.5 Cron missedPolicy `runAll` / `skip`** —— 当前只 `runOnce`（默认）。**Done when**：spec 字段 + 3 种 policy 都实现 + 测试。
- ☐ **§5.6 Webhook secret rotation** —— 当前 secret 单字段。加 rotation 支持（旧 + 新两段同时有效一段时间）。**Done when**：mcp.json 风格 `{"secret": "new", "secretOld": "old"}` 两者都通过。
- ☑ **§5.7 Workflow run-level timeout** ✅ 2026-05-17 —— `Workflow.TimeoutSec int`（0 = unlimited）+ `StartRun` 改用 `context.WithTimeout` + `runReadyLoop` 检测 `ctx.Err == context.DeadlineExceeded` → status=failed + error_code=`RUN_TIMEOUT`（与显式 cancel 区分）+ 2 单测。
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

- ☑ **§11.1 env destroy 后 function lazy-rebuild**（burn-in #15）✅ 2026-05-15 —— P3 batch 已修：`app/function/run.go::RunFunction` 在 `sandbox.Run` 返 `ErrEnvNotFound` 时按 stored spec 走 `syncEnvSync` 重建并重试一次。**tracker 滞后,2026-05-17 审计补上 ☑**。
- ☑ **§11.2 ownerKind 非白名单显式拒**（burn-in #16）✅ 2026-05-15 —— P3 batch 顺手修：`validOwnerKinds` 5 值白名单 + 400 `INVALID_OWNER_KIND`，告别"空 list 当无数据"误读。
- ☑ **§11.3 runtime 表 `3.12` vs `>=3.12` dedup**（burn-in #17）✅ 2026-05-15 —— P3 batch 已修：`RuntimeInstaller` 接口加 `NormalizeVersion(version) string`，`MiseInstaller` 复用 B-01 的 `normalizeVersionForMise`；`EnsureRuntime` lookup/insert 前归一化让 `>=3.12` / `3.12` 共用一行。**tracker 滞后,2026-05-17 审计补上 ☑**。
- ~~§11.4 Ruby / PHP EnvManager~~ ❌ **2026-05-17 删除——前提不成立**。`sandbox.md` §1 写明 Marketplace V3 collapse(2026-05-08)已删 Ruby/PHP/Rust/Go/Java EnvManager(无消费方)。当前 `cmd/server/main.go::registerSandboxStack` 只注册 python+node+uv 3 runtime + Python/Node 2 EnvManager。此 sweep 项是 bitrot,基于已删除的 EnvManager 矩阵写的。**若未来真有 Ruby/PHP plugin 需求,重做 Ruby/PHP EnvManager 再处理 bundler/composer，远期不规划**。
- ☑ **§11.5 env corruption 自动重装** ✅ 2026-05-17 —— `infra/sandbox/spawn.go::SpawnOnce` + `SpawnLongLived` 在 exec 前对绝对路径 `opts.Cmd` 走 `os.Stat`,缺失则返 `ErrEnvNotFound`,复用 §11.1 lazy rebuild 路径(零额外代码路径)。**Done**:dangling symlink 单测覆盖；生产 mise 升级后第一次 run 不再炸 cryptic ENOENT,自动重建 venv 重试。
- ⏸ **§11.6 Docker runtime support** —— Marketplace V3 collapse 已删 Docker installer；当前生产无 docker runtime/EnvManager 装配。**V1.5 真 defer**：要做就等真撞需求(mcp / function 需要 docker 隔离),sentinel `ErrDockerNotInstalled` / `ErrDockerDaemonDown` 保留为前瞻位。
- ❌ **§11.7 Disk usage warn** —— **2026-05-17 用户判定不做**。`/api/v1/sandbox/disk-usage` 已返 `totalBytes`,用户主动看就够；threshold warn + testend banner 是非开发用户场景的 polish,当前 dogfood 阶段不优先。**重开条件**：撞到磁盘满 / sandbox 缓存过大影响 dev 时再加。

---

## 12. HTTP / API 收尾

**背景**：Plan 05 14 项 hardening 完成，但 burn-in #18 + 几个收尾还在。

- ☑ **§12.1 Pagination limit cap**（burn-in #18）✅ 2026-05-15 —— 审计发现 `pkg/pagination.Parse` 早已 cap 到 `MaxLimit=200`；负数/非数字 400 INVALID_REQUEST；handler 全经 Parse 单入口，无直读 `?limit`。**实现到位**（误判为遗留）。
- ☑ **§12.2 HTTP request retry (LLM client)** ✅ 2026-05-16 —— `infra/llm.Generate` 套 `withRetry`（3 attempts，500ms → 1.5s 指数退避）；`isRetryable` 白名单 `ErrRateLimited` / `ErrProviderError` / `context.DeadlineExceeded`；`ErrAuthFailed` / `ErrBadRequest` / `context.Canceled` 不重试。**注**：仅 `Generate`（非流式）有 retry —— `Stream`（chat 主路径）不重试，避免 mid-stream retry 丢已见内容；7 单测覆盖。
- ☑ **§12.3 Per-conversation model override** ✅ 2026-05-17 —— `modeldomain.ModelRef`（跨 domain shared）+ `Conversation.ModelOverride *ModelRef`（GORM serializer:json）+ `UpdateInput.ModelOverride **ModelRef`（三态：nil 跳 / &nil 清 / &{...} 设）+ handler `updateConvRequest.HasModelOverride` 区分 absent vs explicit null + Service.SetKeyProvider 启用 F1 422 PROVIDER_HAS_NO_KEY 校验 + `llmclient.ResolveWithOverride` override-first + chat runner 用新 API + main / harness 注入 KeyProvider + testend types/api/store + `ModelOverrideEditor.vue` 弹窗（list user 已 tested-ok apikeys' modelsFound × providers）+ chat header `⚙ <model>` 按钮 active 时高亮。autoTitle 仍用全局 picker（不 override）。5 新 conv app 单测全绿。
- ☑ **§12.4 Webhook 多 HMAC 算法** ✅ 2026-05-17 —— webhook spec 接 `signatureAlgo: "hmac-sha256-hex"`（A2 + 智能默认）+ `signatureHeader?`（缺省 `X-Hub-Signature-256`，GitHub 兼容）。`registration` 加两字段，Register 时校验：未知 algo / algo 缺 secret 都返 `ErrInvalidConfig`。handler 把 secret 校验移到 body 读后，按 algo 分支：HMAC 走 `verifyHMACSHA256Hex(body, secret, headerVal)`（constant-time + 自动剥 `sha256=` 前缀），plain 兜底走原 `X-Webhook-Secret` / `?token=` 比对。5 新 webhook 单测全绿（invalid algo / algo-no-secret / default-header / custom-header / bare-hex）。

---

## 13. 可靠性 / 故障恢复

- ☑ **§13.1 LLM client withRetry**（同 §12.2，单列因为这是可靠性核心）✅ 2026-05-16 —— 见 §12.2。
- ☐ **§13.2 LLM provider auto-failover** —— 单 provider 持续失败 → 自动切到 fallback provider（用户配的）。**Done when**：apikey 配 `fallbackProvider` 字段；chat resolver 实现切换。
- ☐ **§13.3 SearXNG instance 健康度跟踪** —— `app/tool/web/search.go` 三层 fallback，但 SearXNG 池中坏实例不剔除。**Done when**：连续失败 ≥ 3 次的 instance 30 分钟内不用。
- ☑ **§13.4 BackgroundCtx 死亡检测** ✅ 2026-05-17 —— `catalogService.Stop()` + `skillService.Stop()` + `mcpService.Stop(shutdownCtx)` 全部接到 `cmd/server/main.go` SIGTERM 路径（`srv.Shutdown` 之后 / `handlerService.Shutdown` 之前）。polling goroutine 不再泄漏到 OS 回收。
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
- ☑ **§14.5 Workflow + Conversation 接入 + testend Notion UI** ✅ 2026-05-17 —— 4 子件全交付（设计调整 2026-05-16 拆 4 个：5a llm attach / 5b agent 节点 / 5c Conv attach / 5d testend UI）。
  - ☑ **§14.5a `llm` 节点 AttachedDocuments** ✅ —— `LLMNodeConfig.AttachedDocuments: []AttachedDocument{DocumentID, IncludeSubtree}` + `documentapp.ResolveAttached`（live-resolve subtree，调新 public `Repository.ListSubtreeIDs`）+ `dispatch_llm.go` 加 attach 解析 + 前置 `<documents>` 段 + 新 `DefaultLLMCaller` adapter 用 llmclient.Resolve 替原 TODO nil（workflow LLM 节点首次真正可跑）。
  - ☑ **§14.5b 新增 `agent` 节点（14th NodeType）** ✅ —— `domain/workflow/node.go` 加 `NodeTypeAgent` + `IsCapabilityNode` + `dispatch_agent.go` 用 `app/loop.Run` + lazy tool registry（toolsFn closure 避免装配序问题）+ AgentNodeConfig（prompt / attachedDocuments / enabledTools / maxTurns 默认 10、硬上限 50）+ scheduler.Router 注册。
  - ☑ **§14.5c Conversation.AttachedDocuments + chat prepend** ✅ —— `convdomain.Conversation` 加 `AttachedDocuments` 字段（GORM serializer:json）+ `convapp.Service.Update` 改 `UpdateInput` struct（含 AttachedDocuments）+ HTTP handler 接 PATCH + `chat.Service.SetDocumentResolver` 端口 + `chat/runner.buildSystemPrompt` 调 ResolveAttached 前置 `<documents>` 段（cache-friendly 静态层，跟 memory pinned 同位置）+ `documentapp.RenderAttachedAsXML` 共用渲染器。
  - ☑ **§14.5d testend UI** ✅ —— `Documents.vue` 扁平表 CRUD 已在 §14.2 烟雾层就位（含 path 缩进显示树状）；新增 `AttachedDocsEditor.vue` 对话头部 📎 按钮 → 多选挂载 + 含子树 toggle + 文件大小估算；`api/conversations.ts.setAttachedDocuments` + `stores/conv.ts.setAttachedDocuments` + `types/domain.Conversation.attachedDocuments`。**Notion 树（折叠/拖拽）+ Monaco 编辑器留 polish 后续**——当前扁平 + textarea 够 dogfood。
  - **测试**：22 doc tool 单测 + 13 conv handler + scheduler dispatch 单测 + 6 新 pipeline test（agent 写 doc E2E / llm 节点 attach 真到 prompt / Conv 挂载单文档 / Conv 挂载子树 / Conv 空挂载 / workflow validate 拒绝缺失 doc）全绿；make test-unit + Windows 跨平台 + staticcheck（dispatch_agent / dispatch_llm / llm_adapter 干净）全过。

> **设计 pivot 2026-05-16**：(1) 弃原"不新增节点类型"承诺——拆 `llm`（单次 + 挂知识库）vs `agent`（agentic + tools）让成本可见性 first-class。(2) AttachedDocuments live-resolve subtree：用户挂"项目根 + 子树"后续加新文档自动跟上，不用回头编辑 workflow / Conv。详 [`service-design-documents/document.md`](./service-design-documents/document.md) §9。
- 🔄 **§14.6 Intent routing** —— **collapse 到 §18 Prompt Governance**。本质是 prompt 工程不是 feature；multi-agent forging section 已 cover 大部分；剩"按 intent 分支注入子段"属 prompt polish，dogfood 撞到 prompt 缺位再调。
- 🔄 **§14.7 Chat 终极版 — 工作流推荐 + 自动建草稿** —— **collapse 到 §18 Prompt Governance**。F2 multi-agent forging 教学已 ≥80% cover；"推荐 + draft"语义当前 prompt + AskUserQuestion + create_workflow pending v1 已能做。剩纯 polish 留 dogfood 调。

---

## 15. 完美产品 UX

**背景**："这些不做也能用，但没了感觉不像 polished v1.2"。

- ☐ **§15.1 First-run onboarding** —— 首次启动检测 `~/.forgify/` 不存在 → 引导：选语言 / 配 API key / 装一个 sample MCP / 装一个 sample skill。**Done when**：onboarding flow + skip 选项。
- ☐ **§15.2 Sample skills bundled** —— 装机自带 3-5 个示例 skill（如 `pr-review` / `csv-clean` / `code-review`），引导用户看怎么写。**Done when**：`internal/samples/skills/` go:embed + 首次启动复制到 `~/.forgify/skills/`。
- ☐ **§15.3 Sample forges bundled** —— 同上，3-5 个示例 function。**Done when**：装机后用户能立刻 run 一个 sample forge 看效果。
- ☐ **§15.4 Conversation starter prompts** —— 新对话有几个"试试这个"按钮（"帮我写一个 CSV 处理工具" / "解释这段代码"）。**Done when**：testend 新对话页有 starter 卡片。
- ☐ **§15.5 Settings preferences pane** —— 配置 model / api key / hooks / paths-deny / autoCompact 阈值 / language。**Done when**：testend `/settings` 视图 + backend `GET/PUT /api/v1/settings`。
- ☑ **§15.6 Conversation pinning / favorites** ✅ 2026-05-17 —— `Conversation.Pinned bool` + `UpdateInput.Pinned *bool` + PATCH `pinned` field + slim notif `action: pinned/unpinned` + store `setPinned` + sidebar 右键菜单 + pinned conv 加 📌 标识 + bg highlight + sort `pinned DESC, updated DESC` 客户端 + DB ORDER BY `pinned DESC, created_at DESC, id DESC`。无新错误码。
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

## 18. Prompt Governance（吸收 §14.6 / §14.7 / §17.1）

**背景**：LLM-facing prompt 散养在 33 个 tool description + 9 个系统段 + ~6 个内部 LLM 提示词。今天 audit 要 grep 整仓；前端用户也看不到实际发给 LLM 的 system prompt 是啥；没 lint 规约。collapse 原 §14.6 intent routing + §14.7 chat 终极版 + §17.1 tool description polish 到这里——本质都是 prompt 工程问题。

- ☑ **§18.1 Prompt inventory endpoint + testend viewer** ✅ 2026-05-17 —— `GET /api/v1/dev/prompts` (dev-only) 罗列所有 prompt 源：33 个 tool descriptions（自动从 `deps.Tools` 抽 Name()+Description()）+ 2 个 chat-system 静态段（base / multi-agent forging）+ 3 个 internal-llm（contextmgr.compact / catalog.generator / web.summary template）+ 3 个 subagent（Explore / Plan / general-purpose）= **41 条**。每条返 `{name, category, description, content, length, tokensEst, source}`。testend `/dev/prompts` 搜索 + 按 category 过滤 + 点击展开 + length 颜色告警（<50 黄 / >800 红）+ category-级统计板（count + total tokens）。
- ☑ **§18.2 System prompt composition preview** ✅ 2026-05-17 —— `chat.Service` 加 `SystemPromptSections(ctx, conv) []PromptSection` 返按 cache-friendly 顺序的命名段（base → multi_agent_forging → catalog → memory → documents → user_systemPrompt → locale_hint）。`AssemblePromptSections` 把每段用 `<section name="...">` XML marker 包起来——LLM 能识别边界 + UI 能渲染。`GET /api/v1/conversations/{id}/system-prompt-preview` 返完整 sections + assembled + 长度/token 估算。testend chat header 加 **📋 prompt** 按钮 → 全屏 modal（每段折叠 + raw 切换）。`buildSystemPrompt` 重构为 `AssemblePromptSections(SystemPromptSections(...))`——单一事实源。
- ☑ **§18.3 Prompt lint + section markers** ✅ 2026-05-17 —— `cmd/lintprompts` 走 5 个 prompt-bearing dir，regex 抽 `const \w*[Pp]rompt|\w*[Dd]escription = \`...\`` 共 34 条，对每条跑 4 条规则：(1) length [50, 800] 外，(2) 第一人称 "I will / I'll / I am / I need to"，(3) weasel "be careful / try to / when in doubt / as much as possible"，(4) emoji。**当前基线 1 violation**（contextmgr.compactSystemPrompt 890 char 略超，可接受指导意义）。`make lint-prompts` Makefile target 集成。
- ☑ **§18.4 Prompt principles 文档** ✅ 2026-05-17 —— `documents/version-1.2/prompt-principles.md` 6 条原则（examples-beat-rules / what-NOT / static-first / no-first-person / no-weasel / 50-800 sweet spot）+ anti-pattern 表 + section markers 设计 + "新写 prompt 流程"5 步。lint 规则 1:1 对照原则 4/5/6。

**§18 完工标准**：4/4 全 ☑。新增 prompt 的工程纪律落地——audit + preview + lint + principles 四件套就位，未来 prompt 修改有 dev tool + 守门 + 原则书。

---

## 20. 多用户 / Profile 切换（V1.2 minimal）

**背景**：单机本地 + 一个用户多账号场景（个人 vs 工作 vs 副业），需要类 Slack workspace 切换。无 auth、无密码——单机谁拿 laptop 谁控制。

- ☑ **§20.1 User domain + service + HTTP** ✅ 2026-05-17 —— `domain/user` + `app/user` + `infra/store/user` + `handlers/users.go`（GET list / POST / GET id / DELETE / POST :activate）+ 5 sentinel + errmap。`EnsureDefault` 启动时给空表创默认 `ID="local-user"` user 让现有数据自然 surface（数据库已经按 user_id scope）。8 单测。
- ☑ **§20.2 Middleware + session 机制** ✅ 2026-05-17 —— `InjectUserIDWith(resolver)` 读 `X-Forgify-User-ID` header（SSE EventSource 浏览器 API 限制 → query `?userID=` 兜底），验证 user 存在 → ctx。回退链：unknown header / 缺 → 首个 user → DefaultLocalUserID（DB 空兜底）。3 新单测。
- ☑ **§20.3 Per-user filesystem roots** ✅ 2026-05-17 —— 新 `pkg/userpath`：`UserHome(homeRoot, uid)` → `homeRoot/users/<uid>/`；`MigrateLegacy` 把老 `homeRoot/{mcp.json,skills,.catalog.json,settings.json}` 平迁到 `homeRoot/users/local-user/`（target 存在 → skip）。main.go 4 处装配切到 `defaultUserHome`。4 单测。
- ☑ **§20.4 Frontend 切换 UI** ✅ 2026-05-17 —— `api/users.ts` + `stores/users.ts`（active 持久化 localStorage / 切换 reload 清内存 state）+ `UserPicker.vue` startup 选择屏（≥2 profile 且无 active 时显示）+ `UserSwitcher.vue` header 右上 avatar dropdown + `/config/profile` 管理页 + `api/client.ts` 注入 `X-Forgify-User-ID` header + `api/sse.ts` 给 EventSource URL append `?userID=`。

**V1.2 限制**（用户已确认接受）：
- 后台任务（catalog / skill / mcp polling、scheduler、trigger）仍跑在默认 user 上下文；非 active user 的 cron workflow **会** fire 但用 owner user_id 入 flowrun。
- 无密码 / 锁。
- 不区分 user 的 mcp.json / skills 目录——切 user 时仍读默认桶。运行时按 user 重建 service 留 V1.5。
- 真"per-user 后台 poller + 隔离 service tree" → V1.5。

**§20 完工 4/4 ✅**——单机多 profile 体验跟 Slack workspace 切换齐平，5 天工程量浓缩在 1 天交付。

---

## 17. Burn-in 剩余 + 杂项

**背景**：burn-in 还有 #7 / #11 / #15-18 这几个；#15-18 已经在 §11-12 中拆出来；剩 #7 / #11。

- ☑ **§17.1 Burn-in v1 #7 — LLM tool description** ✅ 2026-05-17 —— 2026-05-11 + 2026-05-14 主体修；§18 Prompt Governance 上线后，新的 tool description 改动均经 `make lint-prompts` 守门（长度 / 第一人称 / weasel / emoji 4 类自动告警）+ `/dev/prompts` 一站式 audit。原"独立 prompt tuning round"诉求由 §18 工具链满足。
- 🔄 **§17.2 Burn-in v1 #11 — NodeType "end" 别名** ✅ 主体 2026-05-14 —— apply.go::applyAddNode 现走 validate teach error（v1#11 的"unknown node type"消息，引导 LLM 走 13 NodeTypes 词汇表），LLM 试 `end` 时不再静默失败。"加 `end` synonym alias"属低优先 nice-to-have，v1.2.x polish。
- ☑ **§17.3 Conversation queue full 时的 backpressure** ✅ 2026-05-16 —— 审计验证：现状 `select` + `default` 已经直接返 409 `STREAM_IN_PROGRESS`（buffered chan(5) 满时），无 block / panic 风险。**实现到位**（误判为遗留；名字 STREAM_IN_PROGRESS 与"queue full"语义稍有重叠，但 V1.2 单用户本地几乎撞不到 6+1=7 队，过度区分是反校验剧场）。
- ⏳ **§17.4 Conversation 长期 idle 后清理** —— 现状审计 2026-05-17：`runQueue` 已有 5 min idleTimeout GC（convQueue + agentstate 一起释），覆盖单用户活跃场景。"30 天 idle conv 整体 archive + agentstate 释放"属 V1.5 多对话归档场景（与 §17.12 archived 字段联动），本轮 defer。无活跃 bug 风险。
- ⏳ **§17.5 Bash AST walk 边界** —— defer v1.2.x：单用户本地 dev 自己写 `eval`/`source`/`$()` 时立刻发现 conv venv 没前置 PATH（手动 export），属体验改进非 bug。auto-route 失败 = 用户 fall back 到 plain shell 行为，不影响安全/正确。
- ☑ **§17.6 Catalog cache lock contention** ✅ 2026-05-17 —— 审计验证 bitrot：`catalog.polling.go::Refresh` 调 `snapshotSources()`（RLock → copy slice → defer RUnlock 立即释），LLM categorization 调用阶段不持锁；Set 写入用 `sourcesMu.Lock` 短临界区。设计已到位。
- ⏳ **§17.7 MCP server restartOnCrash** —— defer V1.5：单用户本地 MCP 挂时用户自己重启即可；auto-restart + backoff 是多用户/24x7 部署场景才必要。当前 fail-loud 哲学正确，不引复杂度。
- ⏳ **§17.8 Skill body cache TTL** —— defer v1.2.x，原 spec 已标低优先。
- ⏳ **§17.9 Forge_executions retention** —— 用户 2026-05-17 明确不做（"retention 我觉得不要修"）。单用户本地数据量小，膨胀风险低。defer V1.5。
- ⏳ **§17.10 Sandbox env GC schedule** —— defer v1.2.x：手动 `:gc` 对单用户够用，cron-like scheduler 是多用户场景。
- ☑ **§17.11 Sub-Message status drift 自检** ✅ 2026-05-17 —— `chatdomain.AllStatuses` 单一事实源 + 两个 pure switch `chatStatusToEventLog` / `subagentStatusToEventLog` 返 `(out, known bool)` + 两个 host wrapper 仅在 unknown 时 Warn + 两个契约测试遍历 AllStatuses 断言每个 Status* 都不落 default 分支。两测试全绿。
- ☑ **§17.12 Conversation 加 `archived: bool` 字段** ✅ 2026-05-17 —— `convdomain.Conversation.Archived bool`（GORM not null default false + index）+ `ListFilter.Archived *bool`（nil = 默认排除归档；true / false = 显式过滤）+ `convapp.UpdateInput.Archived` + handler 接 `?archived=` query 与 PATCH `archived` field + slim notif 切换时 action 改 `archived`/`unarchived` + testend ConvSidebar 📁 toggle 按钮 + 右键菜单"归档/取消归档"+ store `setArchived` / `toggleShowArchived` + types/api/store 三处类型同步。无新错误码。
- ☑ **§17.13 SearXNG 三层 fallback hint** ✅ 2026-05-17 —— bitrot：search.go:42 的 `searchDescription` 现写 "Routes to the first available source: a configured BYOK provider (Brave / Serper / Tavily / Bocha), then the duckduckgo-search MCP server (if installed). When neither is available the result includes a hint about how to enable one." chain 已重写为 BYOK → DDG MCP（SearXNG → Bing → Bing CN 是过时描述），hint 早已加。

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
| §4 | 可观测性 | 10 / 10 | ✅ 完工 | 2026-05-17 全清——`/conversations?search=` + export md/json + metrics dashboard + llm-trace + catalog history/diff + context-stats + testend `/observe/usage` + `/config/llm-health` 全到位 |
| §5 | Workflow 高阶 | 2 / 10 | 🟢 主体推进 | §5.1 Loop body ✅ + §5.7 Run-level timeout ✅ 2026-05-17；§5.2 defer；§5.3-§5.6/§5.8-§5.10 V1.5 territory |
| §19 | 工作流可用性 | 2 / 2 | ✅ 完工 2026-05-17 | Dry-run 模式 + Live progress UI（轮询 + 通知响应 + 状态徽章）|
| §20 | 多用户 / Profile 切换 | 4 / 4 | ✅ 完工 2026-05-17 | User domain + middleware header + per-user fs roots + frontend picker/switcher；后台 polling 仍走默认 user，per-user poller 留 V1.5 |
| §21 | 多语言 i18n (中/英) | 4 / 4 | ✅ 完工 2026-05-17 | User.Language + vue-i18n + locale 切换 UI；门面层翻译完，深层 panel V1.5 |
| §6 | MCP 高阶 | 0 / 7 | ⬜ 未起 | |
| §7 | Skill 高阶 | 0 / 5 | ⬜ 未起 | |
| §8 | Subagent 高阶 | 0 / 5 | ⬜ 未起 | |
| §9 | Catalog 高阶 | 0 / 5 | ⬜ 未起 | |
| §10 | UX 工具完善 | 0 / 7 | ⬜ 未起 | §10.1 Checkpoint/Undo 用户判定不做（trinity 已 version control + 本地文件 ops 单独建 checkpoint 基建 ROI 不够）|
| §11 | Sandbox 收尾 + 修复 | 5 / 6 | 🟢 主体完工 | §11.1+11.2+11.3+11.5 ☑（最后一件 §11.5 2026-05-17）；§11.4 删除(bitrot,Marketplace V3 collapse 已删 Ruby/PHP installer 前提不存在)；§11.6 Docker V1.5 真 defer；§11.7 用户判定不做。剩 §11.6 (defer) 不计入活跃 sweep |
| §12 | HTTP / API 收尾 | 4 / 4 | ✅ 完工 | §12.1+§12.2 早完；§12.3+§12.4 ✅ 2026-05-17 |
| §13 | 可靠性 / 故障恢复 | 3 / 5 | 🟢 主体完工 | retry + timeout + §13.4 BackgroundCtx 收尾 ☑；§13.2 provider failover / §13.3 SearXNG (bitrot) 留 / defer |
| §14 | Phase 5 真正没做的 | 5 / 5 | ✅ 完工 | §14.1-§14.5 全交付（5/5）；原 §14.6 + §14.7 collapse 到 §18 Prompt Governance（本质 prompt 工程不是 feature）|
| §18 | Prompt Governance | 4 / 4 | ✅ 完工 2026-05-17 | inventory endpoint + testend viewer + composition preview + lint + principles doc 全套；吸收 §14.6 / §14.7 / §17.1 polish 诉求 |
| §15 | 完美产品 UX | 1 / 16 | 🟡 起步 | §15.6 pinning ☑ 2026-05-17；其余 onboarding / settings pane / themes / 等待穿插着做 |
| §16 | 桌面 app 准备 | 0 / 7 | ⬜ 未起 | Wails 主迁移前的后端 prep |
| §17 | Burn-in 剩余 + 杂项 | 8 / 13 | 🟢 主体完工 | §17.3+§17.6+§17.13+§17.1 ✅；§17.2 🔄 主体完工(end alias polish 留)；§17.4+§17.5+§17.7-10 ⏳ defer V1.5 / v1.2.x；§17.11+§17.12 ✅ 2026-05-17 |

**Ship gate 进度（`完工标准` 节）**：

- ☑ §1 + §2 + §3 — **"AI 真的不傻"基线**（✅ 2026-05-16 全过）
- 🟢 §11-12 (sandbox / API 收尾) + §13 (可靠性) — **不易崩**（主体过，残留低优）
- ⬜ §14 (Phase 5 智能化) — **愿景核心**
- ⬜ §10.1 (Checkpoint) + §15.1-15.5 (onboarding 基本套件) — **用户敢用**

### 19.2 日志

| 日期 | 范围 | 摘要 |
|---|---|---|
| 2026-05-17 | **[doc-fix] §S14 同步补课** | 多个 feat batch 只更新 sweep + progress-record，漏了 design / contract 文档同步。一次性补：(1) 新 `service-design-documents/user.md` 13 节完整设计 doc；(2) `api-design.md` 加 users 5 endpoints + `/dev/prompts` + `/system-prompt-preview` + workflows `?dryRun=true` query + 多用户 session 段 + 多语言段；(3) `error-codes.md` 加 user 6 sentinel + RUN_TIMEOUT + trigger INVALID_CONFIG；(4) `database-design.md` 加 `users` 表 + `workflows.timeout_sec` + `flowruns.dry_run` + `flowrun_nodes.parent_loop_node` + `iteration_index`；(5) `workflow.md` 14 节点 + loop body config 详；(6) `scheduler.md` 新增 §6.4 Sub-DAG / §6.5 run-level timeout / §6.6 dry-run + §6.3 RehydrateOnBoot 多 user 注释；(7) `trigger.md` Spec.UserID 段。**反思**：feat 期间应与代码一同步契约文档，事后补效率低且易漏。下次纪律 = contract docs 与 commit 一起更新。 |
| 2026-05-17 | **§21 多语言 i18n（中/英）** | 后端 `User.Language string`（zh-CN / en；DB CHECK 约束）+ `Update / Create input` 字段 + PATCH endpoint + `ErrLanguageInvalid` errmap。前端 `vue-i18n@10` + `src/i18n.ts`（zh-CN 默认，localStorage `forgify:locale` + 浏览器嗅探）+ `locales/{zh-CN,en}.json` 骨架（common / topbar / nav / convs / chat / users 6 节）+ `client.ts` Accept-Language header 注入 + `usersAPI.update` + `users.setLanguage` 双写 vue-i18n。**门面层翻译**：TopBar / TabNav / ConvSidebar / UserPicker / UserSwitcher / Profile.vue 全切 `t()`。Profile 页加语言切换 select。**深层 panel（FlowRunDetail / WorkflowDetail / 等 60 个）保留原文**，dogfood 撞到再补。test-unit + vet + vue-tsc 全绿。 |
| 2026-05-17 | **§20 多用户 / Profile 切换 V1.2 minimal** | **§20.1** `domain/user` + `app/user`（Create/Get/List/Delete/EnsureDefault/TouchLastUsed）+ `infra/store/user` + `handlers/users.go`（GET list / POST / GET id / DELETE / POST :activate）+ 5 sentinel + errmap 5 行（USER_NOT_FOUND / USERNAME_REQUIRED / USERNAME_CONFLICT / USERNAME_INVALID / CANNOT_DELETE_LAST_USER）+ 8 单测。**§20.2** `InjectUserIDWith(resolver)` 读 `X-Forgify-User-ID` header；fallback 链 unknown → 首个 user → DefaultLocalUserID（SSE 用 `?userID=` query 兜底，因 EventSource 不能自定义 header）+ 3 单测。**§20.3** 新 `pkg/userpath`：`UserHome` + `MigrateLegacy`（target 存在 skip）；main.go 4 装配位置切到 `defaultUserHome`（mcp.json / skills / .catalog.json / settings.json）+ 4 单测。**§20.4** `api/users.ts` + `stores/users.ts`（localStorage 持久化 active；switchTo reload 整页清 state）+ `UserPicker.vue` 启动选择屏（≥2 profile + 无 active 显示）+ `UserSwitcher.vue` header avatar dropdown + `/config/profile` 管理页 + `client.ts` X-Forgify-User-ID header 注入 + `sse.ts` URL `?userID=` append。**EnsureDefault** 启动期把空表迁出 `ID="local-user"` 默认 user 让现有 row（已是该 user_id）自然 surface；`MigrateLegacy` 同步把单用户期文件路径平迁到 `users/local-user/`。**V1.2 限制接受**：后台 polling 走默认 user / 无密码 / 不区分 user 的 service tree（运行时重建留 V1.5）。**§20 4/4 ✅**。test-unit + vet + vue-tsc 全绿。 |
| 2026-05-17 | **§5.7 Run-level timeout + Dry-run + Live UI 三连发** | **§5.7** `Workflow.TimeoutSec int`（0 = unlimited）→ `StartRun` 用 `context.WithTimeout`；`runReadyLoop` 检测 `ctx.Err == DeadlineExceeded` 区分 → status=failed + RUN_TIMEOUT。**Dry-run** `FlowRun.DryRun bool` + `StartRunWithOptions(opts StartRunOptions{DryRun})` + `ExecutionContext.DryRun` propagate（含子图）+ `dispatchWithPolicies` 拦截 8 个 side-effect NodeType（function/handler/mcp/skill/llm/agent/http/approval/wait）返合成 `{out: "[DRY RUN]", _dryRun: true}`；approval 自动 NextPort=approved；HTTP `POST /workflows/{id}:trigger?dryRun=true` 接入（bypasses trigger.FireManual）。**Live progress UI** `FlowRunDetail.vue` 加 2s polling（running/paused 启动；终态停）+ notifications store watch（type=flowrun match id 即时 refresh）+ 头部 `👁 DRY RUN` / `⏱ RUN_TIMEOUT` 徽章 + 🟢 live-pulse 闪烁。workflow trigger UI 加 👁 Dry-run 按钮。**4 新单测**（DeadlineExceeded→RUN_TIMEOUT / 显式 cancel→cancelled / 函数 dry-run mock / pure logic 透传 / approval 自动 approved）。test-unit + vet + vue-tsc 全绿。 |
| 2026-05-17 | **§5.1 Workflow Loop body subgraph 完整版** | scheduler 真正 for-each：(1) `pause.go::driveLoop` 抽 `runReadyLoop` 与 finalizeRun 解耦——让子图复用 ready-set 主循环不写终态。(2) 新 `subdag.go`：`ExecuteSubDAG(req)` per-iteration 入口 + `SubDAGFromBody(map)→Graph` decoder + `SubstituteLoopTemplates` 深度模板替换（`{{ .loop.item }}` / `{{ .loop.index }}` 在 string / nested map / list 叶节点全生效）+ `concurrentRun` 并发 helper（默认 cap 5）。(3) `LoopDispatcher` 重写：sequential 默认 / `parallel: true` + `concurrency: N` 并行 / `onError: stop` 默认 vs `continue` opt-in（返 `{successes, failures: [{index, error}]}`）。(4) `flowrun_nodes` 加 `parent_loop_node TEXT` + `iteration_index INT` 列；`recordNode` 自动 propagate。(5) `ExecutionContext` 加 `Loop / ParentLoopNodeID / IterationIndex`；`dispatch_condition.go` EvalContext 转发 Loop。(6) body 含 approval 节点拒（V1 不支持中途暂停）。3 层嵌套上限沿用 validate.go 早有 container-body 递归校验。**5 单测**：simple foreach sequential / fail-fast / continue / approval rejected / SubstituteLoopTemplates 嵌套。**create_workflow tool description 加 loop body 完整例**（含 parallel / onError 字段说明）。**§5 状态 0/7 → 1/10**（§5.2 标 defer V1.5）。test-unit + vet + Windows cross 全绿。 |
| 2026-05-17 | **§18 Prompt Governance 全批 — 吸收 §14.6 / §14.7 / §17.1** | **§18.1** `GET /api/v1/dev/prompts` (dev-only) 罗列 41 条 prompt（33 tool desc + 2 chat-system 静态段 + 3 internal-llm + 3 subagent）每条返 {name, category, content, length, tokensEst, source}。testend `/dev/prompts` 搜索/过滤/折叠/length 颜色告警 + category 统计板。**§18.2** `chat.Service.SystemPromptSections` 返按 cache-friendly 顺序的命名段；`AssemblePromptSections` 加 `<section name="...">` XML markers；`GET /api/v1/conversations/{id}/system-prompt-preview` 端点 + testend chat header 📋 prompt 按钮全屏 modal（每段折叠 + raw 切换）。`buildSystemPrompt` 重构为单一事实源（sections → assemble）。**§18.3** `cmd/lintprompts` 扫 5 个 prompt-bearing dir × 34 个 prompt 常量 × 4 规则（length 50-800 / 第一人称 / weasel / emoji）；当前基线 1 violation（compactSystemPrompt 890 char）。`make lint-prompts` Makefile target。**§18.4** `prompt-principles.md` 6 条原则（examples-beat-rules / what-NOT / static-first / no-first-person / no-weasel / 50-800 sweet spot）+ anti-pattern 表 + section markers 设计 + "新写 prompt 流程"5 步。**Collapse**：§14.6 + §14.7 + §17.1 全部归 §18 ✅（本质 prompt 工程不是 feature）。**§18 完工 4/4；§14 状态 5/7→5/5 完工**。test-unit + vue-tsc + Windows cross 全绿。 |
| 2026-05-17 | **§12.3 + §12.4 中等批 — Per-conv model + Webhook HMAC** | **§12.3** `modeldomain.ModelRef` 共享 struct + `Conversation.ModelOverride *ModelRef`（GORM serializer:json）+ `UpdateInput.ModelOverride **ModelRef` 三态 + handler `HasModelOverride` flag 区分 absent vs explicit null + `Service.SetKeyProvider` 启 F1 422 校验（复用 burn-in #5 `ErrProviderHasNoKey` / 已 errmap）+ `llmclient.ResolveWithOverride` override-first + `chat/runner` 切到新 API + main / harness 注入 KeyProvider。testend：types + `setModelOverride` API/store + 新 `ModelOverrideEditor.vue` 弹窗（按 user 已 tested-ok apikeys' modelsFound × providers）+ chat header `⚙ <model>` 按钮 active 时 accent-bg。5 新 conv unit test 全绿。**§12.4** webhook spec 接 `signatureAlgo: "hmac-sha256-hex"`（A2 + 智能默认 header）+ `signatureHeader?`（缺省 `X-Hub-Signature-256`）。`registration` 加 2 字段，Register 验 algo 白名单 + algo 缺 secret → `ErrInvalidConfig`。handler 把 auth 块移到 body 读后，按 algo 分支：HMAC 走 `verifyHMACSHA256Hex`（constant-time `hmac.Equal` + 自动剥 `sha256=` 前缀），plain 兜底走原 `X-Webhook-Secret` / `?token=` 等值比对。5 新 webhook unit test。**§12 状态 2/4 → 4/4 完工**。同步 conversation.md / trigger.md / api-design / database-design。test-unit + vue-tsc 全绿。 |
| 2026-05-17 | **§13.4 + §15.6 + §17.12 test 小修批** | **§13.4** `cmd/server/main.go` SIGTERM 路径加 `catalogService.Stop()` + `skillService.Stop()` + `mcpService.Stop(shutdownCtx)`，polling goroutine 不再泄漏。**§17.12** `TestList_ArchivedFilter` 补 3 个 subtest（nil/false/true 三态过滤）。**§15.6** Conversation pinning：domain `Pinned bool` + UpdateInput + PATCH `pinned` + slim notif `action: pinned/unpinned` + DB ORDER BY `pinned DESC, created_at DESC, id DESC` + testend setPinned / 右键置顶 / 📌 标识 / bg highlight / 客户端同 sort。docs：conversation.md（struct + DB schema + §7.2/7.3）+ api-design + database-design。**§13 状态 2/5→3/5；§15 状态 0/16→1/16**。test-unit + vue-tsc 全绿。 |
| 2026-05-17 | **§17.11 + §17.12 真活 + §17 tracker 大整理** | **§17.11** chatdomain Status drift 契约测试——加 `chatdomain.AllStatuses` 单一事实源 + 两个 pure switch `chatStatusToEventLog` / `subagentStatusToEventLog` 返 `(out, known bool)` + 两 host wrapper unknown 时才 Warn + 两契约测试遍历 AllStatuses 断言无 default 分支命中。**§17.12** Conversation `archived` 字段——domain `Archived bool` + GORM index + `ListFilter.Archived *bool`（缺省排除已归档）+ `convapp.UpdateInput.Archived` + handler `?archived=` query + PATCH 接 `archived` field + slim notif `action: archived/unarchived` + testend types/api/store 同步 + ConvSidebar 📁 toggle + 右键菜单归档/取消归档。**§17 tracker bitrot/defer 大扫除**——§17.6（catalog lock，`snapshotSources` RLock 立即释 LLM 不持锁）+ §17.13（SearXNG hint，chain 重写 BYOK → DDG MCP，description 已含 hint）核实 bitrot ☑；§17.1 + §17.2 2026-05-11/14 已主体覆盖（13 tool description 瘦身 + workflow op cheatsheet + apply.go teach errors），剩独立 prompt tuning 标 v1.2.x polish 🔄；§17.4 + §17.5 + §17.7-10 ⏳ defer V1.5/v1.2.x（单用户本地场景不必要的 multi-user 防御）。**§17 状态 1/13 → 7/13**。同步 conversation.md（struct + DB schema + API §7.2/7.3 含 archived）+ 本表 + tracker 行。backend test-unit + vue-tsc 全绿。 |
| 2026-05-16 | **§14.5 设计调整 #3 — 拆 `llm`/`agent` 节点 + subtree live-resolve** | (1) **拆 LLM 节点**为两种——`llm`（单次 + 挂知识库 = 原节点扩 AttachedDocuments）+ **新增 14th NodeType `agent`**（agentic loop with full system tool registry，含 7 个 document tool + filesystem + bash + web + MCP + skill）。**理由**：成本可见性——`llm` ≈ 1 LLM call / $0.001-0.01，`agent` ≈ 1-N call / $0.01-0.10+。写 doc 必走 `agent` 节点。**架构现实校验**：grep `dispatch_llm.go` 当前是 `LLMCaller.Generate` 单次,no tool,backend-design L209 那句"workflow LLM 是 app/loop 调用方"是设计意图未实现——故 `agent` 节点而非扩 `llm` 节点是正确路径。(2) **AttachedDocuments live-resolve subtree**：schema 改 `{documentId, includeSubtree?}` 替 flat ID list；用户挂"整 Notebook"后续加新 doc 自动跟上。共用 `documentapp.ResolveAttached` resolver；Repository expose public `ListSubtreeIDs`。(3) **Conversation 复用同 struct**——chat / workflow `llm` / workflow `agent` 三入口共享 schema。**§14.5 拆 4 子件**：5a (`llm` AttachedDocs, 0.5d) / 5b (`agent` 节点, 1d) / 5c (Conv attach, 0.5d) / 5d (testend Notion UI, 1.2d) = ~3 天（原 1.5 天因加 agent 节点膨胀但换 first-class agentic 能力 + 写 doc 官方路径）。**同步**：final-sweep §14.5 子件清单 + document.md §9 重写（含 LLM vs agent 对比 + ResolveAttached + dispatch_agent.go 设计）+ §10 Conv schema + §14 实施顺序表 + §15 不变量。**纯设计 pivot 未写代码,§14.3 下一步开工**。|
| 2026-05-16 | **§14 设计改向 #2 — Notion-style 树状** | 在 no-RAG 决策之上进一步精化 document 数据模型：从"flat doc + 可选 section 子表"改为 **Notion-style 树状嵌套**（单表自引用 + ParentID + Position + Path 冗余字段）。**理由**：(1) 用户实际心智模型是"大文档套小文档"（项目笔记 / API 文档树 / 日报树）不是"PDF 切章节"；(2) 单表自引用比独立 sections 子表语义更清晰；(3) AI 能用 create / move / delete 真正帮用户组织文档不止读。**系统工具 2 个 → 7 个**：`search_documents` / `list_documents` / `read_document` / `create_document` / `edit_document` / `move_document` / `delete_document`；后 4 个 WorkspaceWrite，`delete_document` 自动 destructive=true 走 §3 permissions ask 路径。**Workflow 接入改向**：原计划新增第 14 种 `document` 节点类型 → 改为给现有 `llm` 节点 config 加 `AttachedDocumentIds: []string`，节点数仍 13 不爆炸；`Conversation` 同样加 `AttachedDocumentIDs` 让对话能挂文档库；前者 `dispatch_llm.go` 拼 `<documents>` 段前置 input，后者 `runner.buildSystemPrompt` prepend（跟 memory pinned 同一 cache-friendly 层）。**Catalog 接入**：按 path 分组（`- /Projects/2026/Q1 — Q1 计划`），>50 docs 自动 progressive disclosure。**新建 `service-design-documents/document.md`** 详设计 doc。**同步**：final-sweep §14 5 子项重写 + §19.1 tracker + backend-design 协作图（document subtree → 子节点 tree）+ progress-record §2 dev log。**纯设计 pivot，未写代码**。|
| 2026-05-16 | **§14 设计改向 (no-RAG)** | 弃 RAG / sqlite-vec / chunking / 向量检索 → 改 **LLM-ranked document attach** 模式（抄 forge / skill / mcp catalog 套路）。**理由**：(1) 本地单用户文档量人类规模（几十到几百），向量索引过度工程；(2) 2026-04-26 已有先例（progress-record 行 100，tool search 从 chromem-go 切到 LLM 排序）同一推理成立；(3) 大 context（Sonnet 4.6/4.7=200K，Opus 4.7=1M）+ Anthropic prompt cache（5min TTL，命中省 90%）让"塞全文"反超 RAG；(4) 用户场景是"工作流决定 attach 哪个 doc"——deterministic routing 不是 similarity search。**结果**：§14.1 sqlite-vec spike 取消（modernc 不再需要加载 C 扩展），§14.1-14.5 重设计为 `document` domain + 2 system tools（`search_documents` LLM-ranked + `read_document`）+ catalog 第 4 source + workflow `document` 节点类型，工程量减半，跨平台编译一行命令保住。**未来扩展**：真撞上"全公司 wiki 几千篇 / GitHub repo 自动索引代码 chunk"再加 `embedding` 列 + 向量库当二进制工具，平滑长出来。**同步**：final-sweep §14 全部 7 子项重写 + §18 ordering + §19.1 tracker；backend-design.md 能力清单 #4 + Phase 5 描述 + 跨 domain 协作图 + domain tree；progress-record §2 dev log。**纯设计 pivot，未写代码**。|
| 2026-05-16 | **§3 permissions + hooks + §4.1+4.2 + §12.2 + §13.1 + §13.5 + §17.3** | V1.2 ship gate 第三栏（"AI 真的不傻"）完工。新增 4 包（`domain/permissions` + `infra/settings` + `app/tool/permissionsgate` + `app/hooks`）+ 56 tool 危险等级登记 + glob→regex 翻译器 + shell hook stdin/stdout JSON 协议 + `pathguard.AllowWrite`（读写 deny 分离）+ 5 HTTP endpoints + testend `/config/permissions` 3 tab。30+ 新单测 + 3 pipeline test 全绿。同步：`service-design-documents/permissions.md`（18 节设计 doc）+ api-design + error-codes（INVALID_SETTINGS / BLOCKED_BY_RULE）+ progress-record dev log。**附带**：Token counter（§4.1）+ Usage 端点（§4.2，按 model 拆 + cost 估算 via 新 `pkg/llmcost` 16-model registry）+ LLM withRetry（§12.2 + §13.1，仅 Generate；Stream 不重试避免 mid-stream 丢内容）+ chat worker 10 min timeout（§13.5）+ queue backpressure 现状审计（§17.3 已实现到位）。|
| 2026-05-16 | **§1 compaction + §2 memory** | V1.2 ship gate 第一栏（"AI 真的不傻"基础）完工。新增 `domain/permissions` 仍待 §3；但 §1 + §2 两大跨对话能力一气落地：`pkg/modelmeta` + `pkg/tokencount` + `app/contextmgr.Manager`（3 路径：< Soft / Soft 降级 / Hard fullCompact）+ `conversations.summary` + `summary_covers_up_to_seq` 2 列 + `message_blocks.context_role` 1 列 + 新 block type `compaction`（eventlog 协议 6→7 种）+ `loop/history.BlocksToAssistantLLM` 按 role 投影 + chat.buildHistory 前置 `<conversation_summary>` wrapper；Memory 新 `domain/memory` + `app/memory` + 3 system tools + 7 HTTP endpoints + 3 errmap sentinel + testend `/config/memory` + `/current/compaction` 2 视图。4 + 3 = 7 pipeline tests 全绿。设计 doc：`service-design-documents/memory.md` + `compaction.md`。附带修旧债：harness Attrs migration / scheduler dotted edge / handler Python kwargs / cancel 422 接受 / chat_test seed-delete 模式。|
| 2026-05-15 | **§11.2 ownerKind 白名单 + §12.1 limit cap（审计）** | burn-in P3 batch 顺手修：`validOwnerKinds` 5 值白名单 + 400 INVALID_OWNER_KIND。同期审计 §12.1 发现 `pkg/pagination.Parse` 早已 cap 到 200，无 handler 直读 `?limit`——实现到位（误判为遗留）。|

> **下条记录待写**：sqlite-vec 闸门 2026-05-16 设计改向后已弃，下一站建议 **§10.1 Checkpoint/Undo**（~1 天 ship gate "用户敢用"）或直接进 **§14.1 Document domain**（Phase 5 愿景核心，LLM-ranked attach 模式，2-3 天）。§11 sandbox 残留低优，穿插着做。
