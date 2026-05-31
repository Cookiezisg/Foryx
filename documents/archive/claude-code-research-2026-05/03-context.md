# 03 — Claude Code Context 管理 / Compaction

## 信息来源与局限

主要参考：
- https://finisky.github.io/en/claude-code-context-compaction/ (5 layer cascade 详解)
- https://justin3go.com/en/posts/2026/04/09-context-compaction-in-codex-claude-code-and-opencode (跨 IDE 对比)
- https://kenhuangus.substack.com/p/chapter-3-the-query-agent-loop-claude (queryLoop)
- https://gist.github.com/badlogic/cd2ef65b0697c4dbe2d13fbecb0a0a5f (compaction prompts 部分摘录)
- https://www.morphllm.com/claude-code-auto-compact
- https://hyperdev.matsuoka.com/p/how-claude-code-got-better-by-protecting
- https://code.claude.com/docs/en/checkpointing (官方)
- https://claude-code-explain.helmcode.com/system-prompt/

---

## 1. Token 计数与触发机制

### 1.1 实时追踪

✅ Claude Code 在 `State.autoCompactTracking` 里跟踪累计 token。每次 `queryModel` 调用 Anthropic API 返回 `usage` 字段（`input_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`, `output_tokens`），追加到 tracking。

⚠️ context 占用百分比 = `usage.input_tokens + usage.cache_read_input_tokens` / model_context_window（Sonnet 200K 或 1M、Opus 200K）。**不是简单字符数估算**。

### 1.2 触发阈值

✅ **Claude Code CLI** 默认 95%（25% remaining）触发 auto-compact；
⚠️ **VS Code 扩展** 在 ~75% 用量（25% remaining）就触发，给 compaction 自身留 ~20% 预算；
⚠️ 部分二手分析提到 92% 阈值（如 wU2 文章），**官方 docs 写 95%**——以 95% 为准。可通过 CLAUDE.md `Compact Instructions` 影响行为，但**阈值本身不可配置**（issue #11819 是 feature request）。

### 1.3 触发后的同步性

✅ 触发后**同步阻塞**：在下一次 `queryModel` 调用之前，先 fork 一个 subagent 跑 `summarizePrompt` → 生成摘要 → 替换历史 → 用新历史发起原本的 LLM call。这就是 `transition.reason = reactive_compact_retry`。

---

## 2. 五层 Compaction Pipeline ⭐

✅ 这是 Claude Code 的核心设计。5 个 shaper 在每次 LLM 调用前**按顺序**作用于 `messagesForQuery`：

```
Layer 0: applyToolResultBudget()   ← 每条 tool result 单独限大小
Layer 1: snipCompact (HISTORY_SNIP) ← 去掉中段 N 条 message
Layer 2: microcompact              ← 合并连续 tool-result pair
Layer 3: contextCollapse (CTX_COLLAPSE) ← 读取时投影旧消息
Layer 4: autoCompact               ← 最后兜底：LLM 摘要
```

设计哲学（来自 Finisky Garden 文章）："**避免压缩；非要压则廉价压；最后实在不行才调 LLM 摘要**"。

### 2.1 Tier 1 / Layer 0：applyToolResultBudget

✅ 对每条历史 `tool_result` block 单独检查大小：
- 超过 `TOOL_RESULT_MAX_TOKENS`（约 25K tokens）的 oversized output 被替换成 placeholder：`<tool_use was here. Output was too large; reference id=tu_xxx>`
- 替换的 reference 不会影响 LLM 行为（LLM 看到 placeholder 就知道之前调过）
- 不丢 message 结构、不丢小输出
- 每次 LLM call 前都跑（廉价、O(n)）

✅ **保留判断**：保留小 output（<25K）和**最近的几条**（位置感知）。具体哪些算 "原始 tool output 该被移除" 的判断标准未在公开分析中找到精确阈值。

### 2.2 Tier 1 / Layer 1：snipCompact（HISTORY_SNIP）

✅ 删除 history **中段**消息（保留首尾）。触发条件：input_tokens 超过某软阈值（例如 60% 容量）。
- 删除策略：保留最早 K 条（user request、关键 prompt）+ 最新 N 条（recent context）；中间删
- 删除的位置插入 `<snip: removed M messages from this point>` placeholder
- **不调 LLM**，纯结构层操作

### 2.3 Tier 1 / Layer 2：microcompact

✅ 合并连续的 tool-call/tool-result pair。例如 5 个连续的 Read tool（同一个 LLM 回合）会被合并成单条 summary。
- 触发：cache 命中率下降时（cache 失效会让 input tokens 翻倍）
- 不调 LLM，按规则做

### 2.4 Tier 2 / Layer 3：contextCollapse

✅ "读时投影"：在读历史构建 messagesForQuery 时，把更老的消息**用一个紧凑的 representation 替代**，但**原始 message 仍然在 State 中保留**——所以是"projection"，不是 destructive。
- 类似 LRU 但保留全文，只是 query 时不发
- 当再 collapse_drain 重试时，逐步增加 collapse 强度

### 2.5 Tier 3 / Layer 4：autoCompact ⭐

✅ 最后兜底：把 history 整个发给 LLM 让它产 summary。
- **作为 forked subagent 跑**（独立 context，不污染主对话）
- subagent 用一个**特定 system prompt**（社区抓到部分原文，下面是关键骨架）：

```
Your task is to create a detailed summary of the conversation so far,
paying close attention to the user's explicit requests and your previous
actions. This summary will be used as context when continuing the
conversation, so preserve critical information including:
- What was accomplished
- Current work in progress
- Files involved
- Next steps
- Key user requests or constraints
- Important technical decisions / architectural choices
- Unresolved bugs / known issues
```

⚠️ 公开 gist 提到的"8 sections"具体名字未完全公开，上面是经验性骨架。

✅ subagent 输出后：
- 替换主 history 为 [system prompt + summary message + 最新 user message]
- **重新读 CLAUDE.md / MEMORY.md / 已激活的 skills 和 rules**（避免被 compaction 丢掉）
- 写一个 marker 让用户/UI 知道"compaction happened"

✅ 60-80% 压缩率：典型示例（来自 Morph 文章）"5-ticket workflow 从 204K → 82K"=58.6%，跨多次 compaction 累积可达 70%+。

### 2.6 Tier 3 / `/compact [focus]`

✅ 用户主动调 `/compact "focus on auth refactor"`：把 focus 字符串注入 summarize subagent 的 system prompt，作为"特别保留这部分"的指令。本质和 autoCompact 同一条路径，只是 focus 是用户给的。

---

## 3. System Prompt 缓存分割

### 3.1 SYSTEM_PROMPT_DYNAMIC_BOUNDARY

✅ Claude Code 用一个特殊 marker `__SYSTEM_PROMPT_DYNAMIC_BOUNDARY__` 把 system prompt 切两半：
- **boundary 之前（静态部分）**：tool definitions、CLAUDE.md（merge 之后的）、global preferences、tool-use rules——**全局可缓存**（跨 organization、跨 session 都能命中）
- **boundary 之后（动态部分）**：MCP instructions、loaded skills 描述、当前 session 的 cwd、当前时间、git status 摘要——**session 特定，不全局缓存**

⚠️ marker 的具体文件位置（在 system prompt 第几行）未在公开分析中找到精确数字，但顺序是"static → DYNAMIC_BOUNDARY → dynamic"。

✅ MCP instructions 用 `DANGEROUS_uncachedSystemPromptSection()` 包裹——因为 MCP server 可能在 turn 之间断开/重连，schema 会变，缓存会失效。

### 3.2 cache_control 用法

✅ Anthropic API 的 `cache_control` 标记位置：
- 标在 tools 末尾（cache 整个 tools block）
- 标在 system prompt 静态段末尾（cache static system）
- 标在 messages 列表的"上一轮 assistant + tool_result"对（**滑动 breakpoint**：每轮往前推一格）

每次 turn 最多 4 个 cache breakpoints。Claude Code 用满 4 个是常见的。

### 3.3 跨 session 缓存复用

✅ 静态部分缓存 5 分钟 TTL（Anthropic 默认）。同用户在 5 分钟内开新 session，static system 部分 cache hit；超过 5 分钟则 cache_creation 一次（费用 1.25× 普通 input tokens），后续仍可复用。

---

## 4. Checkpoint 系统

### 4.1 文件快照

✅ Edit/Write/NotebookEdit 执行**前**，Claude Code 自动对目标文件做 snapshot：
- 存放：`~/.claude/projects/<sessionId>/snapshots/<fileHash>@v<n>`
- 命名：`abc123def4567890@v1`、`@v2`...（hash 是 path 的稳定 id）
- 文件内容是 raw bytes，不做 diff 压缩

✅ 持久 30 天后自动清理。

### 4.2 跨 session 持久化

✅ 同一 session 内回滚：`Esc Esc` 双击 ESC 调出 checkpoint 选择器；选某点恢复后，会话历史也会回退到那个点（fork-style）。

✅ 跨 session：`claude --resume` 拿回 JSONL 历史，但 snapshot 仍可用（文件没被清理）。

### 4.3 已知限制

✅ **只追 Claude 调用 Edit/Write/NotebookEdit 的修改**。Bash 跑 `rm`/`mv`/`sed -i` 这种**不被追**——所以 Claude Code 在 Bash 调用前**特别要求权限确认**。

---

## 5. Context 可见性 `/context`

✅ `/context` 命令展示 token 使用分布的可视化。典型示例（VS Code 扩展）：

```
Context window: 125,432 / 200,000 tokens (62.7%)
├─ System prompt:        4,210 tokens
├─ Tools definitions:   12,800 tokens
├─ CLAUDE.md:            1,560 tokens
├─ MEMORY.md (loaded):     820 tokens
├─ Skills active:        2,400 tokens (3 skills)
├─ MCP tool descs:         180 tokens (5 servers, lazy)
├─ Conversation:        98,720 tokens
│   ├─ User messages:    8,200
│   ├─ Assistant:       12,600
│   ├─ Tool calls:      78,920 ← 主要占用
└─ Auto-compact at:    180,000 tokens (90%)
```

`/mcp` 单独查每个 MCP server 的 token 成本。

---

## 6. 对 Forgify 的改进建议

> 现状（`backend/internal/app/chat/history.go`）：
> - `maxHistoryMessages=200` 硬截断
> - 无 token 计数（不知道用了多少）
> - 无任何 compaction 层
> - 无 prompt cache 利用（每次 system prompt 都按全量算 input）
> - 无 checkpoint

| # | 改进 | 优先级 | 实施要点 |
|---|---|---|---|
| 1 | **token 计数** | P0 | 把 LLM API 返回的 `usage.input_tokens` / `output_tokens` 累积到 conversation 状态。`stream.go` 已经在 `EventFinish` 拿到 InputTokens/OutputTokens（行 78-83）——把它聚合。新增 `domain/conversation` 字段 `tokenUsage` 存最近 N turn 历史 |
| 2 | **Layer 0 applyToolResultBudget** | P0 | `history.go` `blocksToAssistantLLM` 里循环 tool_result block，超 25K 字符替换成 `<truncated tool_result id=xx, original size=N bytes>`。**最容易做、收益最大**。25K 是个保守阈值；可放到 `chat/config.go` 配 |
| 3 | **Layer 1 snipCompact** | P1 | history.go `buildHistory` 在已知 token 用量后，若 >= 60% 容量，删除中段（保留首 K=10 + 末 N=20），插入 `<snip: removed M messages>` 占位 |
| 4 | **Layer 4 autoCompact (subagent)** | P1 | 当 token >= 90% 容量，触发 forked subagent 调 LLM summary，使用类似 §2.5 的 prompt。需要先有 subagent 能力（见报告 05）。简化版：单次 LLM call，不 fork |
| 5 | **System prompt 静态/动态拆分** | P2 | runner.go `buildSystemPrompt` 拆两部分：static（Forgify 描述、tool rules）+ dynamic（locale、cwd、time）。配合 LLM provider（Anthropic）的 cache_control 使用——若 provider 支持 |
| 6 | **/context 命令** | P3 | 新增 service method 返回 token 分布；前端 chat header 加按钮 |
| 7 | **Checkpoint** | P3 | write_file/edit 前把原文件 SHA + bytes 存到 `<storage>/snapshots/<convId>/<fileHash>@v<n>`。30 天后清。`Esc Esc` 等价的 UI 是 conversation timeline 上"恢复到此点" |

最先做：**#1 + #2**（token 计数 + tool result budget），这俩做好可让现有"200 条硬截断"立即变成更智能的预算管理，工作量小、收益大。


