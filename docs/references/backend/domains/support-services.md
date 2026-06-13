---
id: DOC-030
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# 支撑服务十域 —— workspace · apikey · model · websearch · catalog · mention · notification · aispawn · humanloop · contextmgr · entitystream

> 十个微域合篇（各 100-900 行）。每节：定位 + 关键设计 + 契约引用。

## workspace —— 隔离根

唯一**没有** workspace_id 列的表（它就是 workspace 本身，全局表——这正是后台 `forEachWorkspace` 播种能在裸 ctx 列它的原因）。CRUD + 守"最后一个不能删"（`CANNOT_DELETE_LAST_WORKSPACE`）+ 语言校验 + auth 中间件的 `WorkspaceResolver` 端口（`Resolve`——校验 id 并返其 UI locale，使 **workspace.language 权威于 Accept-Language**：识别到 workspace 即覆盖头默认，assistant 按用户显式持久化语言回复，头仅作 onboarding 兜底）。**Delete 级联销毁**（Reaper 端口、bootstrap 后注入）：杀全部 workflow 自动化（摘监听+取消在途 run，连手动 run 一并）→ 停常驻 handler/mcp 进程 → 删盘上文件树（skills/memories）→ 删 ws 行（行消失即数据不可达+后台播种跳过；DB 实体行留作不可达遗留）。全程 Detached(目标 ws) ctx——删除请求可来自另一 workspace；best-effort。携带 per-workspace 模型默认（dialogue/utility/agent 三场景 ModelRef 列 + 默认搜索 key + `webFetchMode`——WebFetch 工具抓取方式，local=本机直 GET（默认，URL 不出本机）| jina=公共 reader（提取更好但 URL 发第三方）；PD-4 裁决 C，PATCH 设置、Service 经 `WebFetchMode(ctx)` 供 tool/web 读、读不到一律收敛 local）。码 `WORKSPACE_*` 7；ID `ws_`。

## apikey —— 加密凭据管理

凭据自身生命周期：存（AES-GCM 整密文）、probe 连通性测试、按 id 发放（`KeyProvider`/`ProbeReader` 端口）。**刻意零 provider 语义**——选哪个 key、key 隐含什么模型，是 model/websearch 的事。**删除守卫**：`RefScanner` 端口（boot 注册），Delete 询问每个 scanner、任一命中即拒（`API_KEY_IN_USE` 422）；真实引用来源二——workspace 的三 scenario 默认模型 / 默认搜索 key（`workspace.ReferencesAPIKey`）+ agent active 版本的 modelOverride（`agent.ReferencesAPIKey`），均结构满足端口、build_services 注入。probe 归档供 model 聚合解析。码 `API_KEY_*` 7；ID `aki_`。

## model —— 模型选择与能力

无存储：默认在 workspace 列、覆盖在实体字段。定义 `ModelRef` 值 + 三场景白名单（dialogue/utility/agent）+ **覆盖优先默认兜底**规则；`CapabilityService` 读 apikey 的 probe 归档、经各 provider 自描述的 `DescribeModels` 聚合模型目录（vision/native-docs 能力供 chat 附件门控）。码 `MODEL_*` 3。

## websearch —— 搜索配置词汇

最小 domain：provider 词汇（brave/serper/tavily/bocha）+ `SearchKeyPicker` 端口（workspace 选的搜索 key）。执行在 `tool/web`（BYOK 单 key 直连，无 provider 遍历）。

## catalog —— 能力总览（派生、不存）

按需聚合注册 source（function/handler/agent/skill/mcp/document…各自实现 `ListItems`）：`Summary`=注入 system prompt 的分组菜单文本；`Coverage`=结构化 source→ids 供 HTTP。**永不持久化、永不缓存**——每次现扫当前真相。容器实体（handler/mcp）带 `Members` 子单元列表。码 `CATALOG_ALL_SOURCES_FAILED`。

## mention —— @ 引用契约

纯契约包：`MentionType` 集 + `Resolver` 接口 + `Reference` 快照形状。**freeze-on-send**：发送时快照被 @ 实体内容进 user 回合 Attrs，实体后续变更不影响已发送语境。各实体的 mention_resolver.go 实现并 boot 注册。

## notification —— 通知中心

任何模块经 `Emitter` 端口发 `<domain>.<action>`；app 落 DB 行 **并**推 notifications 流 durable 信号（SSE 推送 best-effort，DB 行是真相）。前端列表 + 未读徽标（`WhereNull(read_at).Count`）。码 `NOTIFICATION_*` 2；ID `noti_`。

## aispawn —— AI 工作对话引擎（:iterate / :triage）

两个动词都归结为"开一个预 seed 上下文的对话，让正常 chat loop 接管"：`iterate` seed 实体（function/handler/agent/workflow/document 经 mention 快照注入）+ 编辑指引；`triage` seed 一次失败执行的诊断上下文。返回 `conversationId`（N5）。码 `EMPTY_ITERATE_REQUEST`/`UNTRIAGEABLE_EXECUTION`。

## humanloop —— 人在环 broker

进程内 broker：`Request(ctx, req)` **阻塞**挂起 waiter（按 toolCallID 键）直到 `Resolve` 送达人的决定或 ctx 取消；`Surface` 回调（chat 注入）把待决请求推成 ephemeral 流信号；**内存 pending 表是重连重同步的真相源**；`Allow/IsAllowed` = approve_always 的会话级白名单（与 active skill 的 allowed-tools 同为预授权来源）。经 ctx（`WithBroker/From`）流进嵌套 agent 运行。无表无码（纯运行时）。

## contextmgr —— 对话压缩引擎

生产侧（消费侧在 loop 投影 + chat.LoadHistory）。回合边界两步管线：① **demote**（免 LLM）——旧 tool_result 按新旧降 hot→warm（预览）→cold（占位），工具输出占 token 大头、常一步就够；② 仍超预算才 **summarize**——utility 模型摘最旧 span、增量并入 conversation.summary、推水位。触发用末回合**真实** InputTokens（非估算）达 input 预算 80%；最近 4 条 message 永不动（逐字底线）。**水位（summary_covers_up_to_seq）是幂等键**：崩溃在写 summary 与翻 archived 标记之间也不重复计数。写面最小：conversation 的 summary+水位 + blocks 的 ContextRole（投影、不改内容）。

## entitystream —— entities 流生产原语

SSE-C 的唯一生产 helper：向 Bridge 发实体锚定的节点（open→delta*→close 或点 Signal），scope = 实体（function/handler/agent/workflow/mcp/…）。所有实体面板的实时活动（run 终端/forge 镜像/fire 信号/节点进度）都经它——**一个原语、十处复用**。nil Bridge 全程容忍（无流不影响业务）。ctx 注入（WithBridge/WithRunScope）供 loop 镜像 forge 工具。
