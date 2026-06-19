---
id: DOC-022
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# messages —— 回合内容模型（中立、被 chat/agent/subagent 共享）

## 1. 定位 + 心智模型

一个对话回合的**内容模型**：`Message`（回合：user 发言 / assistant 生成）拥有一棵 `Block` 树。刻意与 stream 分离——**stream 是传输（帧怎么到前端），messages 是内容（回合由什么组成）**；共享 ReAct 引擎（loop）产 Block 并依赖本包而非 chat，故 chat/agent/subagent 共享一个中立模型。无 app 层（store 直接被 host 消费）。

**Block 六型**：text / reasoning / tool_call / tool_result / compaction（压缩摘要标记）/ **progress**（工具中间过程——一等持久块：实时流在 tool_call 下 + 随回合落盘供刷新重放，但 **LLM 历史投影是类型白名单**、progress 永不回喂模型）。更深层级（subagent 子树）走 stream 的 `Open.ParentID`，不加块型。

**两段式写**（对应 loop.Host 契约）：`CreateMessage`（开回合，先 mint id 供流锚点；user 回合连 text block）→ `FinalizeMessage`（终态 + token/provider/model 溯源 + blocks，单事务、seq 落盘时分配）。两表 append-only（D1 内容日志永不删）。

**关键字段语义**：`SubagentID`（≠"" 的回合是 subagent 产出——LoadHistory 排除使父历史不被污染、ListMessages 保留使 reload 能重建子树；LLM 读路径见下）；`ContextRole`（hot/warm/cold/archived——压缩器对块的**投影**变更，落库 Content 永不改写）；`StopReason` 的 `max_steps` 是诚实的非成功终态（UI 给"继续"）。

## 2. 契约（引用）

表 `messages` / `message_blocks`（CHECK type/status/role/context_role）→ [database.md](../database.md) · 码 `MESSAGE_NOT_FOUND` → [error-codes.md](../error-codes.md) · ID：`msg_`/块 `blk_`。读面：`ListMessages`（keyset 分页）/ `LoadThread`（整线程，单用户内存可装）/ `SumTokens`（usage）。压缩器唯一写入口 = `UpdateBlocksContextRole`；boot 对账入口 = `SweepNonTerminal`（pending/streaming → cancelled，硬崩溃孤儿清扫）。

**LLM 读 subagent trace**：`get_subagent_trace`（lazy system tool，`app/tool/subagent`）经 `LoadThread` 拉本对话（ctx 取 `conversationId`）、按 `SubagentID != ""` 内存过滤——无参列出本对话各 subagent run（subagentRunId / status / finalText / blockCount / 派它的 tool_call 锚），带 `subagentRunId` 导出该 run 全 trace（块按 Seq 排）。只读、不加表/列/码：无对话 / 未知 id 降级为 tool-result 串（正常工具结局、非 HTTP 错）。补 Subagent 工具只回最终答案的盲区——subagent 内部回合（reasoning/tool_call/tool_result）落 sub-message 但不进父 LLM 历史，靠此读回。
