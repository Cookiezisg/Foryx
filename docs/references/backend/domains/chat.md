---
id: DOC-021
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# chat —— 对话引擎

## 1. 定位

把一条用户消息变成持久化回合、在工作区工具上驱动 ReAct 循环（`app/loop`）、实时推流、落盘结果。**它是枢纽但一无所有**：conversation/messages/loop/tool/attachment/memory/document/catalog/todo/model 全部经 DIP 端口注入——chat 用 fake LLM 即可整测。chat 无 domain 包（[messages](messages.md) 是中立内容模型）；自己的 HTTP 错误（`EMPTY_CONTENT`/`STREAM_IN_PROGRESS`/`NO_PENDING_INTERACTION`）就地用 errorspkg 定义。

## 2. 心智模型：每对话一条串行队列

`Send` 是**两段式**（头部先验对话存在——404 早退不落孤儿行；归档对话**自动解档**后照常接收，软失败不挡消息）：① 同步落 user 回合（text block + 附件 id/[@提及快照](#5-freeze-on-send)进 Attrs）+ 开 assistant 回合（streaming、无 block——先 mint id 作流锚点）+ 发 message_start；② 任务入该对话的 `convQueue`。**每对话一个抽取 goroutine 串行生成**（同时一个 assistant 回合 → block seq 分配天然无竞争）；生成中（`q.running`，至 finalize 放行）再 Send 直接 409 `STREAM_IN_PROGRESS`（不排队）；回合收尾活（同步压缩检查，可达秒级真 LLM 调用）期间的 Send 落进**单槽缓冲**、紧随其后被服务，槽满仍 409。队列 **5 分钟无任务自毁**（休眠对话零成本），新 Send 按需重建；拆卸与投递在 q.mu 下原子互斥（task 不可能滞留死 channel）。**Shutdown 即时**：cancel 全部在跑回合 + stop 信号短路每个队列（不等 idle timer）。

**processTask 的 ctx 装配**（Send ctx 早已消失，全部重建）：`Detached(ws)` + locale + conversationID + messageID + AgentState + messages 流桥 + entities 流桥（forge 镜像）+ humanloop broker + cancel（Cancel 端点可触发）。

## 3. 回合生命周期

`chatHost` 实现 loop.Host（agentHost 的持久化对应物）：
- **LoadHistory**：整线程载入 → 排除 `SubagentID != ""` 的回合（subagent 内部 trace 不进父历史）→ **压缩水位线投影**（seq ≤ `summaryCoversUpToSeq` 的块已并入 conversation.summary、从历史丢弃，summary 前置）→ user 回合按模型能力渲染多模态附件。
- **Tools 每步重算**：resident + `search_tools` + 本对话已 discovered 的 lazy 工具（记在 AgentState）；**AutoActivator**——LLM 直接点名 lazy 工具时自动标记 discovered（免先跑 search_tools）。
- **ReminderProvider**：每步前注入 live todo 清单为临时 `<system-reminder>`（不污染持久历史）。
- **WriteFinalize 在 Detached ctx**：用户中途关页也绝不留永久 streaming 孤儿；**硬崩溃**（kill -9）的孤儿由 boot 对账兜底（`SweepOrphans`——每 workspace 把 pending/streaming 行扫成 cancelled，messages 版 scheduler.Recover）。
- 回合后（仍在队列槽内防竞态）：首回合自动起标题（utility 模型、best-effort）+ 同步触发上下文压缩检查（contextmgr）。**utility 未配时的全降级面**：起标题静默缺席、压缩跳过、WebFetch 摘要回退原文、search_blocks 精选落纯索引——对话主链路（dialogue 模型）不受影响，但这些静默缺席的归因口前端应在设置页提示「未配 utility 模型」。
- maxSteps=25（高于 agent 的 10——交互对话合理串更多步）；触顶诚实报 `max_steps` + "继续"提示。

## 4. 人在环（R0064）

危险工具（LLM 自报 dangerous）/ `ask_user` 在 loop 内**阻塞**于 humanloop broker。chat 注入的 Surface 把待决交互推成 messages 流的 **ephemeral** `interaction` 信号（即时弹出）；**broker 内存 pending 表是真相源**——重连客户端走 `GET .../interactions` 重新同步。`ResolveInteraction` 把人的决定交给 broker（approve 跑 / deny 反馈 / approve_always 加会话白名单——active skill 的 allowed-tools 也是预授权来源）；重复 POST 安全（`NO_PENDING_INTERACTION` 404）。broker 经 ctx 流入嵌套 agent 运行（嵌套不冒泡，阻塞的 goroutine hold 整栈）。

## 5. freeze-on-send（@提及）

发送时**快照**被 @ 实体的内容（function 代码/handler 类/agent 描述/文档正文…经 mention registry 解析）进 user 回合 Attrs——之后实体再改不影响已发送回合的语境。渲染进 LLM 历史时从快照读。

## 6. 契约（引用）

端点（send/cancel/interactions/usage/system-prompt-preview）→ [api.md](../api.md) · 码 3 个（`EMPTY_CONTENT` 400 / `STREAM_IN_PROGRESS` 409 / `NO_PENDING_INTERACTION` 404）→ [error-codes.md](../error-codes.md)。注：message 行的 `error_code` 字段（如 `LLM_RESOLVE_ERROR`/`MAX_STEPS_REACHED`）是**回合级错误码**（前端展示），与 HTTP wire code 是两个命名空间。

## 7. 跨域集成

消费：conversation（线程配置）/ messages（持久化）/ loop（引擎）/ toolset（resident+lazy）/ attachment（多模态渲染，按模型 caps 门控）/ memory+catalog+document（system prompt 三段）/ todo（reminder）/ model（resolve）/ contextmgr（压缩）/ humanloop（broker）。被消费：`invoke_agent` 嵌套呈现（E3）、subagent 落 sub-message、aispawn 的 `:iterate`/`:triage` 开对话。
