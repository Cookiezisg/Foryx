---
id: DOC-030
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
audience: [human, ai]
---

# 支撑服务十二域 —— workspace · apikey · freetier · model · websearch · catalog · mention · notification · aispawn · humanloop · contextmgr · entitystream

> 十二个微域合篇（各 100-900 行）。每节：定位 + 关键设计 + 契约引用。

## workspace —— 隔离根

唯一**没有** workspace_id 列的表（它就是 workspace 本身，全局表——这正是后台 `forEachWorkspace` 播种能在裸 ctx 列它的原因）。CRUD + 守"最后一个不能删"（`CANNOT_DELETE_LAST_WORKSPACE`）+ 语言校验 + auth 中间件的 `WorkspaceResolver` 端口（`Resolve`——校验 id 并返其 UI locale，使 **workspace.language 权威于 Accept-Language**：识别到 workspace 即覆盖头默认，assistant 按用户显式持久化语言回复，头仅作 onboarding 兜底）。**Delete 级联销毁**（Reaper 端口、bootstrap 后注入）：杀全部 workflow 自动化（摘监听+取消在途 run，连手动 run 一并）→ 停常驻 handler 实例 + 断开 mcp → 清搜索索引 → 删盘上文件树（skills/memories）→ 删 ws 行（行消失即数据不可达+后台播种跳过；DB 实体行留作不可达遗留）。全程 Detached(目标 ws) ctx——删除请求可来自另一 workspace；best-effort。携带 per-workspace 模型默认（dialogue/utility/agent 三场景 ModelRef 列 + 默认搜索 key + `webFetchMode`——WebFetch 工具抓取方式，local=本机直 GET（默认，URL 不出本机）| jina=公共 reader（提取更好但 URL 发第三方）；PATCH 设置、Service 经 `WebFetchMode(ctx)` 供 tool/web 读、读不到一律收敛 local）。码 7（`WORKSPACE_*` 6 + `CANNOT_DELETE_LAST_WORKSPACE`）；ID `ws_`。

## apikey —— 加密凭据管理

凭据自身生命周期：存（AES-GCM 整密文）、probe 连通性测试、按 id 发放（`KeyProvider`/`ProbeReader` 端口）。**刻意零 provider 语义**——选哪个 key、key 隐含什么模型，是 model/websearch 的事。**删除守卫**：`RefScanner` 端口（boot 注册，返 `[]apikeydomain.APIKeyRef`），Delete **聚合每个 scanner 的引用、非空即拒**（`API_KEY_IN_USE` 422，**`details.references` 带每个引用方 `{kind,id,name}`**——kind ∈ `scenario_default`/`search_default`/`agent_override`，前端据此指明去哪解引用，G4）；真实引用来源二——workspace 的三 scenario 默认模型 / 默认搜索 key（`workspace.ReferencesAPIKey`）+ agent active 版本的 modelOverride（`agent.ReferencesAPIKey`），均结构满足端口（仅为 ref 类型 import `domain/apikey`，不依赖 apikey app）、build_services 注入。probe 归档供 model 聚合解析。**旋转 key**（`PATCH` 带新 `key`）重置探测档案为 pending 后**自动重探一次**（有 tester 时，复用 `:test` 路径），200 带回解析后的 `testStatus`，免去「ok 但模型从选择器消失」的静默 pending；重探失败**不让 PATCH 失败**（旋转已成功，同 `CreateManaged` 脑裂取舍，G7）。**内置受管 provider（免费档网关 `anselm`）**：`ProviderMeta.Managed=true` 标记（`GET /providers` 暴露 `managed`，前端据此排除手动「添加 key」列表）；`CreateManaged` 直接播种探测档案（`test_status=ok` + 合成 `/models` body，**跳 live 探针**——否则「ok 但选择器无模型」死状态，且避开配额耗尽探针翻 key 的脑裂），凭证（`gwk_` install token）骑既有 AES-GCM 加密路径、provider=`anselm`、base=`api.anselm.website/v1`；`Update` 对受管行返 `API_KEY_IMMUTABLE`（422，不可编辑），删除仍由 `RefScanner` 守（作默认模型时）。**custom provider 的 `apiFormat`** 在 `validateCreate` 经白名单校验（`openai-compatible`/`anthropic-compatible` 二选一，非法 → `API_KEY_API_FORMAT_INVALID` 400）——堵掉任意串静默落 OpenAI-compat 默认、走错方言（G9）。码 `API_KEY_*` 10；ID `aki_`。

## freetier —— 内置免费档凭证 + 配额代理

把每 workspace 接入 Anselm 网关（DeepSeek 前置 OpenAI-wire 反代，`api.anselm.website/v1`）的内置免费档。**provisioner**（boot 的 `forEachWorkspace` + `workspace.SetOnCreated` 钩子覆盖 boot 后新建 ws，幂等 best-effort）首启铸 `gwk_` install token（POST 网关 `/install`，发**机器指纹 SHA-256**、绝不传裸序列号），落一条受管 `anselm` api_key 行（经 apikey `CreateManaged`，跳 live 探针）；刻意**不**设为默认模型——经网关路由 prompt 需前端显式同意。降级铁律：每个失败路径 log 并返 nil（无指纹 / install 失败 / 持久化冲突），免费档缺席绝不挂 boot/onboarding。**配额代理**（`QuotaReader`，`GET /freetier/quota`）：List 定位受管 anselm 行 → `ResolveCredentialsByID` 解密其 `gwk_` token → `QuotaClient` 以 `Authorization: Bearer <gwk_>` 调网关 `GET /v1/quota`（与 chat 同一 token / 同一 bearer 鉴权），返 `{limit,used,remaining,resetAt,available}`（`remaining=limit-used` 钳 ≥0；`available` 还折网关全局日预算，故 remaining>0 仍可能 false）。客户端**无法直读**——install token AES-GCM 加密存后端、永不出明文（连 `get_model_config` 都脱敏），故持明文的后端代理之；只读、每请求一次、绝不改凭证（失效 token 现为网关鉴权错误、非本地翻行）。无受管行 → `FREETIER_NOT_PROVISIONED`（404，设置页据此隐藏免费档仪表、不渲染误导清零，G1）；网关自身失败原样冒泡 `LLM_AUTH_FAILED`（401/403 token 失效/封禁）/`LLM_RATE_LIMITED`（429）/`LLM_PROVIDER_ERROR`（其余，经 `classifyHTTPError`）。码 `FREETIER_NOT_PROVISIONED` 1；无自有表无 ID（骑 apikey 的 `aki_` 受管行）。

## model —— 模型选择与能力

无存储：默认在 workspace 列、覆盖在实体字段。定义 `ModelRef` 值 + 三场景白名单（dialogue/utility/agent）+ **覆盖优先默认兜底**规则；`CapabilityService` 读 apikey 的 probe 归档、经各 provider 自描述的 `DescribeModels` 聚合模型目录（vision/native-docs 能力供 chat 附件门控）。**LLM 工具**：只读 `get_model_config`（`tool/model`，无参；投影三场景默认 ModelRef + 已配 key 的**脱敏**形（`KeyMasked`、绝不出明文）+ CapabilityService 可用模型）——使 agent 从真 workspace 配置答「我在用什么」、不必 grep 主机 FS（后者会泄露 `.env` 明文 key 并臆造假审计，F68）。码 `MODEL_*` 3。

## websearch —— 搜索配置词汇

最小 domain：provider 词汇（brave/serper/tavily/bocha）+ `SearchKeyPicker` 端口（workspace 选的搜索 key）。执行在 `tool/web`（BYOK 单 key 直连，无 provider 遍历）。

## catalog —— 能力总览（派生、不存）

按需聚合注册 source（function/handler/agent/skill/mcp/document/attachment…各自实现 `ListItems`）：`Summary`=注入 system prompt 的分组菜单文本；`Coverage`=结构化 source→ids 供 HTTP。**永不持久化、永不缓存**——每次现扫当前真相。容器实体（handler/mcp）带 `Members` 子单元列表。码 `CATALOG_ALL_SOURCES_FAILED`。

## mention —— @ 引用契约

纯契约包：`MentionType` 集 + `Resolver` 接口 + `Reference` 快照形状。**freeze-on-send**：发送时快照被 @ 实体内容进 user 回合 Attrs，实体后续变更不影响已发送语境。各实体的 mention_resolver.go 实现并 boot 注册。

## notification —— 通知中心

任何模块经 `Emitter` 端口发 `<domain>.<action>`；app 落 DB 行 **并**推 notifications 流 durable 信号（SSE 推送 best-effort，DB 行是真相）。前端列表 + 未读徽标（`WhereNull(read_at).Count`）。码 `NOTIFICATION_*` 2；ID `noti_`。

## aispawn —— AI 工作对话引擎（:iterate / :triage）

两个动词都归结为"开一个预 seed 上下文的对话，让正常 chat loop 接管"：`iterate` seed 实体（function/handler/agent/workflow/document 经 mention 快照注入）+ 编辑指引；`triage` seed 一次失败执行的诊断上下文。返回 `conversationId`（N5）。码 `EMPTY_ITERATE_REQUEST`/`UNTRIAGEABLE_EXECUTION`。

## humanloop —— 人在环 broker

进程内 broker：`Request(ctx, req)` **阻塞**挂起 waiter（按 toolCallID 键）直到 `Resolve` 送达人的决定或 ctx 取消；`Surface` 回调（chat 注入）把待决请求推成 ephemeral 流信号；**内存 pending 表是重连重同步的真相源**；`Allow/IsAllowed` = approve_always 的会话级白名单（与 active skill 的 allowed-tools 同为预授权来源）。经 ctx（`WithBroker/From`）流进嵌套 agent 运行。无表无码（纯运行时）。

## contextmgr —— 对话压缩引擎

生产侧（消费侧在 loop 投影 + chat.LoadHistory）。回合边界两步管线：① **demote**（免 LLM）——旧 tool_result 按新旧降 hot→warm（预览）→cold（占位），工具输出占 token 大头、常一步就够；② 仍超预算才 **summarize**——utility 模型摘最旧 span、增量并入 conversation.summary、推水位。触发用末回合**真实** InputTokens（非估算）达 input 预算 80%；最近 4 条 message 永不动（逐字底线）。**水位（summary_covers_up_to_seq）是幂等键**：崩溃在写 summary 与翻 archived 标记之间也不重复计数。写面最小：conversation 的 summary+水位 + blocks 的 ContextRole（投影、不改内容）。**demote 只动 tool_result 是刻意的**（F175-M9）：投影侧 `BlocksToAssistantLLM` 仅对 tool_result 施 warm/cold 截断、user/text 块恒全文——工具输出常不需逐字、用户的话是其意图，故大 user 粘贴**不** demote（截断用户原话比保真摘要更糟）、而是经 step ② summarize 处置。**summary 对大粘贴诚实**（F175-M8）：prompt 明示对大粘贴/引用（文档/日志/长代码）只记「是什么 + 要点 + 全文已不在上下文（用户可重发）」、不假装逐字保留——免 summary 记下不可满足的 recall 义务。

## entitystream —— entities 流生产原语

SSE-C 的唯一生产 helper：向 Bridge 发实体锚定的节点（open→delta*→close 或点 Signal），scope = 实体（function/handler/agent/workflow/mcp/…）。所有实体面板的实时活动（run 终端/build 镜像/fire 信号/节点进度）都经它——**一个原语、十处复用**。nil Bridge 全程容忍（无流不影响业务）。ctx 注入（WithBridge/WithRunScope）供 loop 镜像 build 工具。
