---
id: DOC-104
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-09
review-due: 2026-09-01
audience: [human, ai]
---
# Chat — 对话引擎 (The Conversation Engine)

> **核心地位**：`app/chat` 是波次 5 的枢纽——把用户消息变成持久化回合、在工作区工具上驱动 **ReAct 循环**（`app/loop`）、实时推 assistant 回合到 messages 流、落盘结果。它**拧合**已建的 conversation / messages / loop / tool / attachment / memory / document / catalog / todo / model，**但一个都不拥有**：每个依赖经端口（DIP）注入，故 chat 用 fake LLM 即可端到端测、真实装配留 M7。
>
> **职责边界**：对话线程**容器 + 配置**（CRUD）是 `conversation` 域（DOC-106）；回合**内容模型**（Message / Block / 词表 / 落盘）是 `messages` 域（DOC-301）。本文是**引擎**——怎么把一条用户消息跑成一个 assistant 回合。
>
> **交付分轮（chat 模块收官 🎉）**：**R0055 = 引擎核心**（chatHost + convQueue + Send + System Prompt + SSE message 节点 + model resolve）。**R0056 = 可用面 + mention**（HTTP handler〔Send 202 / List / Cancel 204〕 + `Service.Cancel` + mention 整套〔注册表 + `<mentions>` 渲染 + freeze-on-send + 补 workflow/agent resolver〕）。**R0057 = 收尾**（auto-title〔首回合 detached utility Generate + SetAutoTitle + 通知〕 + `GET /usage`〔tokensUsed〕 + `GET /system-prompt-preview`）。**砍**：export / llm-trace（投机占位、YAGNI）。**待 model 域补**：attachment caps 的 model 目录 vision/nativeDocs flag。

---

## 1. 落盘（持久化）

回合落盘是 `messages` 域（DOC-301 §6）：`messages`（`msg_`，回合记录）+ `message_blocks`（`blk_`，Block 树）两表、两段式写。chat 是其消费者，**不碰表**——经 `messages.Repository` 端口：

- **开 user 回合**：`CreateMessage(userMsg{role:user, status:completed, attrs:{attachments}}, [text block])`。
- **开 assistant 回合**：`CreateMessage(asstMsg{role:assistant, status:streaming}, nil)` → 拿 `msg_` id 作流锚点 + reqctx 种子。
- **收 assistant 回合**：`WriteFinalize` → `FinalizeMessage(asstMsg 终态, blocks)`（loop 产的 block 在此分配 seq 落盘）。

block seq 单调靠 **convQueue per-conversation 串行写**（§3）、非 DB 序列。

---

## 2. ReAct 循环（借 `app/loop`，不自己写）

chat **不重写循环**——它实现 `loop.Host` 接口（`chatHost`），把循环交给共享的 `app/loop.Run`。`chatHost` 是 `agentHost` 的**持久化对应物**，同三方法形状，两处改写：

| 方法 | agentHost | chatHost |
|---|---|---|
| `LoadHistory` | prompt（+ replay） | `messages.LoadThread` → LLM 历史（§4） |
| `Tools` | 静态白名单 | resident + search_tools + 本对话已 discovered 的 lazy（§5） |
| `WriteFinalize` | no-op（落 Execution） | `FinalizeMessage` + 推 message_stop（§6，Detached） |

+ 两个可选能力（loop type-assert）：
- **`AutoActivator.TryActivateForTool`**：LLM 点了某 lazy 工具但没先 `search_tools` → 在 lazy 集 `FindLazy` 命中即 `agentstate.MarkToolDiscovered` + 重算工具集。
- **`ReminderProvider.SystemReminders`**：每步前把 `todo.SystemReminder` 作临时 `<system-reminder>` 注入（live 清单顶在模型眼前，不污染持久历史）。
- **不实现 `StepRecorder`**（持久重放是 workflow-agent 的事）。

熔断 / 步数上限（`TOOL_ERROR_STORM` 连续 3 轮全失败、`MAX_STEPS_REACHED` 默认 **25** 步）由 loop 实现、chat 不复制（见 messages.md / loop）。

---

## 3. convQueue（per-conversation 串行）

每个对话一个 `convQueue`：单 goroutine 抽干小缓冲 channel，**同一时刻只跑一个 assistant 回合**（这使 block seq 分配无竞争）。

- **容量 5**：`Send` 投不进（缓冲满）→ `STREAM_IN_PROGRESS`（409）。
- **idle GC 5min**：无任务 5 分钟后 goroutine 自毁、从 `sync.Map` 注销，休眠对话零成本；新 `Send` 按需重建。拆卸期竞态进来的任务会重新注册保活。
- **agentState 挂 queue**：`SeenFiles` / `discoveredTools` 跨该对话的回合共享。
- **cancel 存储**：每回合 `processTask` 把 `context.CancelFunc` 存进 queue；`Service.Cancel`（DELETE stream 端点）取它触发运行回合 ctx Done（loop 流式中断 → WriteFinalize 落 cancelled）+ drain 积压回合逐个 `finalizeCancelled`（防 streaming 孤儿）；无队列优雅 no-op。

---

## 4. 上下文拼装

### 4.1 LoadHistory（喂 loop 的历史）
`messages.LoadThread(convID)` → 逐回合转 LLM 消息（最旧在前）：
- `conversation.Summary` 非空 → 前置一条 `<conversation_summary>` user 上下文块（被压缩的旧历史，原 block 已 archived）。
- **user 回合**：freeze 的 `<mentions>` 快照（§8，从 `Attrs` 渲染）前置 + text block → content；附件 id（落在 `Attrs`）→ `attachment.ToContentParts(ids, Capabilities{Vision,NativeDocs})` 渲成多模态 `Parts`（按当前模型能力门控；渲染失败降级纯文本）。
- **assistant 回合**：`loop.BlocksToAssistantLLM`（hot/warm/cold 投影，archived/compaction 丢）。
- **在飞的 assistant 回合**（本次生成、status=streaming 无 block）被跳过。
- **subagent sub-message**（`SubagentID != ""`，R0058）被跳过——subagent 的内部 trace 落在本对话（供 reload 树）但**不进父 LLM 历史**，父只见派它的 tool_call + 其 tool_result（subagent 最终答案）。

### 4.2 System Prompt（Section 容器）
每回合现拼，`<section name="...">` 包装，cache-friendly 顺序（不变静态在前、动态居中、规则殿后）：

| # | section | 来源 | 静态/动态 |
|---|---|---|---|
| 1 | `identity` | 重写常量 | 静态 |
| 2 | `how_to_work` | 重写常量 | 静态 |
| 3 | `tools` | 静态指引 + `Toolset.Overview()`（lazy 工具一行目录，使 LLM 知全集不盲搜） | 半动态 |
| 4 | `capabilities` | `catalog.GetForSystemPrompt` | 动态 |
| 5 | `memory` | `memory.ForSystemPrompt`（pinned 全文 + 目录） | 动态 |
| 6 | `documents` | `document.RenderAttached(conv.AttachedDocuments)`（XML） | 动态 |
| 7 | `user_system_prompt` | `conv.SystemPrompt` | 动态 |
| 8 | `environment` | 日期 + `reqctx.GetLocale` 回复语言 | 动态 |
| 9 | `architecture_rules` | 重写常量（Quadrinity / durable workflow 指引） | 静态 |
| 10 | `critical_rules` | 重写常量（殿后，末尾指令遵从最高） | 静态 |

静态段 **R0055 重写**（高密度 / 去产品 fluff / 去 safety theater / 不框死 agent），非照搬旧文案。可选 provider 为 nil 时该段降级为空。

---

## 5. 工具集（resident + 按需 lazy）

`chatHost.Tools(ctx)` 每步重算（loop 契约）= `Toolset.Resident` + `search_tools` + 本对话 `agentstate` 已 discovered 的 lazy 工具。lazy 工具默认只在 System Prompt §4.2#3 露一行概览；LLM 调 `search_tools`（chat 从 `Toolset.Lazy` 构造）拉某 lazy 工具完整 schema，标记 discovered，后续步即在工具集内。`search_tools` 是 chat 组装的（`Toolset` 文档明确「overview / search_tools / discovered 集由 chat 组装」）。

---

## 6. SSE 发射（message 级）+ Detached 终态

loop 只发 **block 级** node（open/delta/close 挂 msgID 下）；chat 发 **message 级** node（messages.md §3）：
- **message_start** = `Open{Node{type:"message", {role}}}`（Send 里、开 assistant 回合后）。
- **block 流**（loop 经 `WithBridge` 注入的同一 Bridge）。
- **message_stop** = `Close{Status, Result:{role,status,stopReason,inputTokens,outputTokens,errorCode?,errorMessage?}}`（`WriteFinalize` 里）。
- user 回合 echo = `Open` + `Close{Result:{role:user,content,attachmentIds?}}`（即时完整，使其他客户端立即看到）。

**Detached Context**：`WriteFinalize` / `failTurn` 在 `context.Background()`（重埋 workspace + conversation）上落盘 + 推 message_stop——上游 cancel（用户生成中关页）**绝不留永久 streaming 孤儿**，回合总抵达终态。

---

## 7. model resolve

`processTask` 经 `ModelResolver.ResolveChat(conv.ModelOverride)` 拿 `Bundle{Client, Request, Caps, Provider}`：M7 适配器做 `model.Resolve(ScenarioDialogue, override, workspace picker) → apikey.ResolveCredentialsByID → factory.Build`（对标 `agent` runLoop）。override nil → workspace dialogue 默认模型。`Provider`/`ModelID` 在 loop.Run 前设在 assistant message 上（溯源）。`Caps`（vision/nativeDocs）喂 §4.1 附件渲染（真实 flag 来自 model 目录，**待 model 域补 DescribeModels 字段**；现 M7 adapter 给保守默认）。

---

## 8. Send 流程

`Send(ctx, convID, SendInput{Content, AttachmentIDs}) (assistantMsgID, error)`：
1. 空内容 + 无附件 → `EMPTY_CONTENT`（400）。
2. `CreateMessage` user 回合（+ text block，附件 id 进 Attrs）+ emit user echo。
3. `CreateMessage` assistant 回合（streaming）拿 id + emit message_start。
4. 入队（携带 assistant msgID + workspace + locale，因队列 goroutine 脱离 Send ctx）→ 立即返回 assistant msgID（**202 语义**，回合经 messages SSE 流式）。
5. 入队失败（`STREAM_IN_PROGRESS`）→ 把 assistant 回合落 error，不留 streaming 孤儿。

**mention freeze-on-send（R0056）**：`SendInput.Mentions` 在 Send 时经注册表（`RegisterMentionResolver`，各域 M7 注册自己的 `AsMentionResolver`，5 类 document/function/handler/workflow/agent 全在）逐个 `Resolve` 抓 `Reference` 快照存进 `Attrs["mentions"]`——**发送瞬间定格内容**（后续不 re-resolve；resolver 缺失/失败 → stub `(unavailable)` 不阻断发送）；LoadHistory（§4.1）从快照渲 `<mentions>` 块前置到 user 文本。

---

## 9. 错误矩阵

| Wire Code | HTTP | 物理起因 |
|---|---|---|
| `EMPTY_CONTENT` | 400 | `Send` 无文本无附件 |
| `STREAM_IN_PROGRESS` | 409 | convQueue 缓冲满（对话已在跑） |
| `MESSAGE_NOT_FOUND` | 404 | `messages` 域（DOC-301） |
| `TOOL_ERROR_STORM` / `MAX_STEPS_REACHED` / `LLM_STREAM_ERROR` | — | loop 终态（落 message `error_code`，经 message_stop 上行；非 HTTP） |

---

## 10. 跨域端口（DIP）

`chatapp.Deps`：`Conversations`（Get）/ `Resolver`（ResolveChat）/ `Attachments`（ToContentParts）/ `Toolset` / `Memory` / `Catalog` / `Documents` / `Todo` / `Bridge`（messages 流实例）。可选 provider nil → 优雅降级。mention resolver 经 `RegisterMentionResolver` 后注入（各域 M7 注册）。真实现注入留 M7。

## 11. HTTP 端点

| 方法 | 路径 | 动作 | 响应 | 轮 |
|---|---|---|---|---|
| POST | `/api/v1/conversations/{id}/messages` | `Send`（body `{content, attachmentIds?, mentions?}`） | **202** `{messageId}`（回合经 messages SSE 流式） | R0056 |
| GET | `/api/v1/conversations/{id}/messages` | `ListMessages`（`?cursor&limit`，N4） | 200 Paged（最新在前，每条带 blocks） | R0056 |
| DELETE | `/api/v1/conversations/{id}/stream` | `Cancel`（停运行回合 + drain） | **204** | R0056 |
| GET | `/api/v1/conversations/{id}/system-prompt-preview` | `SystemPromptPreview`（复用 buildSystemPrompt，不解析模型） | 200 `{systemPrompt}` | R0057 |
| GET | `/api/v1/conversations/{id}/usage` | `Usage`（`messages.SumTokens` 透传） | 200 `{inputTokens, outputTokens, totalTokens}` | R0057 |

**auto-title（R0057）**：`Send` 首回合后 `processTask` 后台（detached + 10s + best-effort，s.wg 追踪）经 `ModelResolver.ResolveUtility` + `llm.Generate`（首 user + 首 assistant 摘要 → 5-10 词标题、清洗）→ `ConversationTitler.SetAutoTitle`（写 Title+AutoTitled、不覆盖用户标题）→ `conversation.auto_titled` 通知。非首回合 / 已 AutoTitled 不触发。

> `tokensUsed` 实现为专用 `GET /usage`（解耦 conversation←messages），非富化 conversation 的 `GET /{id}`（契约微调，记 contract #39）。`export` / `llm-trace` 判定为投机占位、**砍**（YAGNI）。
