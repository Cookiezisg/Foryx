# 总结：9 篇报告全局回顾

## 改进优先级总表

按"短期收益最大 + 技术风险最小"排序，跨所有 9 个方向：

### 🚀 P0 第一批（建议本月做）

| 来自 | 改进 | 理由 |
|---|---|---|
| 02 #1 | **Edit tool（精确替换）** | 最大缺口；让 LLM 改代码不再靠"覆盖"或"shell sed" |
| 02 #5 | **Tool 接口加 IsReadOnly** | 是后续并发安全 / 权限分级的前提 |
| 03 #1+#2 | **token 计数 + applyToolResultBudget** | 立刻把"200 条硬截断"变成预算驱动；工作量小 |
| 04 #1+#2 | **FORGIFY.md + read/write_memory tool** | 跨 session 累积知识；约 1-2 天 |
| 06 #1-3+5-6 | **PreToolUse/PostToolUse Hook 链路** | 替换现有"手写 hook"为正式接口 |
| 07 #1+#2 | **AskUserQuestion + TodoWrite** | UX 跃升；用户感知最强 |
| 08 #1+#2+#3 | **PermissionLevel / 保护路径 / Bash 黑名单** | 半天工作量挡 90% 事故 |
| 09 #1+#2+#6 | **MCP stdio 接入 + adapter** | Phase 5 必做；约 2-3 天 |

### 🎯 P1 第二批

| 来自 | 改进 |
|---|---|
| 01 #2 | Stop hook 接口 |
| 01 #4 | isConcurrencySafe 分批执行（替换全并行） |
| 01 #5 | withRetry 包装（429/529/ECONNRESET） |
| 02 #2-3 | Glob / Grep tool |
| 02 #6-7 | Bash 持久 cwd + timeout 配置 + run_in_background |
| 03 #3 | snipCompact (Layer 1) |
| 03 #4 | autoCompact (Layer 4) |
| 05 #1-3 | 基础 Subagent + Explore 类型 |
| 04 #5 | Rules 系统 |
| 06 #4 | Stop hook 完整 |
| 08 #4+#6 | Settings allow/deny + Workflow 安全模式 |
| 09 #4 | HTTP transport |

### 🌱 P2/P3 远期

01 mid-stream tool 执行、05 worktree 隔离、05 Teams 模式、09 OAuth 2.0、04 autoDream、Skills 系统、Slash command 系统、Extended thinking budget UI 等。

---

## 跨报告的设计一致性

✅ Claude Code 在多个层面上做了**统一抽象**，Forgify 应保持一致：

1. **`mcp__` / `Agent(<type>)` / 普通 tool 走同一权限/hook 系统** ← Tool 接口统一是关键
2. **Subagent / autoDream / autoCompact 都用 forked subagent 跑** ← 一套 subagent 框架可复用三处
3. **CLAUDE.md / rules / skills / system prompt 静态段都走 prompt cache** ← 大幅省 token
4. **5 层 compaction cascade** = 廉价的先做、贵的（LLM 摘要）兜底
5. **deny-first 权限优先级** + **hook 能 tighten 不能 loosen**

---

## 信息源置信度声明（再次明示）

本系列 9 份报告基于：
- ✅ Anthropic 官方 Claude Code docs（高置信，已对齐 v2.1.x）
- ✅ 多源交叉验证的二手反编译/分析文章（中-高置信）
- ⚠️ 单源/版本依赖的具体函数名 / token 阈值数字（中置信）
- ❌ 公开分析未触及的内部实现细节（YOLO classifier 模型架构、Seatbelt profile 全文、autoCompact 完整 prompt 等）已明示标记

**原始泄漏 cli.js / cli.js.map 已被 DMCA 下架，本研究不直接引用泄漏源码本身**——所有"代码级"细节均来自社区反编译笔记 + 官方 docs。引用具体函数/类名（nO/h2A/wU2/tUM 等）按反编译者公开命名沿用，可能与未来版本不一致。

