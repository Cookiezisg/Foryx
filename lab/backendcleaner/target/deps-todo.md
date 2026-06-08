# 跨模块待办（从已重写模块移出、待目标模块建立的关注点）

> 重写某模块时，把不属于它的关注点移走后记在这里；到目标模块那一轮去建立或判定，确保不丢。

## 来自波次 0 · M0.1 第一轮（reqctx / idgen / pagination）

| 移出内容 | 原位置（问题） | 去向 | 备注 |
|---|---|---|---|
| model override ctx | `reqctx/modeloverride.go`（🔴 曾让 reqctx → `domain/model` 反向依赖） | model（M1.3） | `WithModelOverride`/`GetModelOverride`；在 model 模块重建其 ctx 透传 |
| agent state ctx | `reqctx/agentstate.go` | agent/loop（M2.2/M3.4） | `WithAgentState`/`GetAgentState` + `pkg/agentstate` 去留判定 |
| 对话/执行标识 ctx | `reqctx/agentrun.go` | chat/loop/messages（M2.2/M5.2） | conversationID·messageID·toolCallID·parentBlockID·subagentDepth；服务 messages 流递归(`Open.ParentID` 嵌套)；判定是否仍走 ctx 透传、放哪一层 |
| ID 前缀 → 实体类型 ✅ R0021 | `idgen/prefix.go` | relation（M1.4，已落地） | `relation.KindForID` 8 条前缀(补 agent + 定 sk_/mcp_ 规矩)；值 = `relationdomain.EntityKind*` |
| HTTP 分页解析 | `pagination`（曾 import `net/http` + `domain/errors`） | transport 框架（M0.7） | `Parse(*http.Request)` + `DefaultLimit`/`MaxLimit`；把 `pagination.ErrMalformedCursor` 映射到 `domain/errors.ErrInvalidRequest` |

## 来自波次 0 · M0.1（userpath 判定删除 R0004）

`userpath` 整包删除（多用户文件分桶 + 历史迁移，新架构不存在）。其能力与连带清理：

| 移出内容 | 原位置（问题） | 去向 | 备注 |
|---|---|---|---|
| app 资源文件根布局 | `userpath.UserHome` → `~/.forgify/users/<uid>/` | workspace（M1.1） | 重定 `~/.forgify/` 下 mcp.json/skills/settings.json/catalog 布局；**删 users/local-user 层**；是否按 workspace 分桶由 workspace 物理模型定 |
| 历史迁移 | `userpath.MigrateLegacy`（迁 mcp.json/skills/.catalog.json/settings.json） | 删，无去向 | 项目未上线 + 无数据保留 → 无 legacy 可迁 |
| cmd/server 装配残留 | `main.go`：`legacyDefaultUserDir="local-user"`、`MigrateLegacy` 调用、"切换 user/V1.5 按 user 重建"注释 | cmd/server（M7.1） | 全删；`SetUserID(ctx,"local-user")`→ boot workspace；mcp/skill/settings 路径改走 M1.1 布局；清 `V1.2 §3` 注释 |

## 来自波次 0 · M0.1（wikilink 剥成纯抽取 R0005）

`wikilink.Parse` 去掉 Kind 解析（`[]ParsedRef{Kind,ID,Count}` → `[]Ref{ID,Count}`）、去 idgen 依赖，变纯文本抽取。是**内部 Go API** 变更，影响 document（M1.10）内部依赖，**不进 contract-changes.md**。连带：

| 移出内容 | 原位置 | 去向 | 备注 |
|---|---|---|---|
| 前缀 → EntityKind 映射 + `KindForID` ✅ R0021 | wikilink | relation domain（M1.4，已落地） | `entitykind.go` 持 8 EntityKind + prefixKind 8 条 + `KindForID` |
| 未知前缀过滤 + Kind 解析 | `wikilink.Parse` | document（M1.10） | document 拿 wikilink 的 ID → `relation.KindForID` 解析 Kind + 过滤 + 跳过自链，再建 `SyncEdge` |
| Kind 映射测试用例 ✅ R0021 | wikilink_test | relation（M1.4，已落地）测试 | `entitykind_test.go` 验 8 前缀 + 未知/执行流水/名字形态返 false |

## 来自波次 1 · M1.4（relation 建立 R0021）

relation 本体（domain/store/app/handler + KindForID + 读时 hydrate）已建。消费侧与未来归一登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| skill/mcp 归一 id 体系 | 波次 3（建 `skills`/`mcps` 实体） | 前缀 `sk_`/`mcp_` 规矩已定、`relation.KindForID` 已识别、wikilink 可抓；届时建表 + 生成器发 id + 各自实现 `Namer`，**零改动**接入 hydrate/前缀映射 |
| `Namer.NamesByIDs` 实现 + 注入 | 各实体域（波次 2/3/5）+ M7 装配 | function/handler/workflow/agent/document/conversation 各实现一句 `WHERE id IN … 取 name`，装配时注入 relation Service；某 kind 缺 namer 则其边显示 id |
| 各实体 sync 胶水 | 波次 2/3/5 | workflow/agent 锻造后 `SyncOutgoing`(equip)+`SyncIncoming`(create/edit)、删除 `PurgeEntity`；document 解析 wikilink → `SyncOutgoing`(link) |
| relation handler 路由装配 | M7 | `NewRelationHandler(...).Register(mux)` 接入总 router（同其他 handler）|

## 来自波次 1 · M1.5（catalog 建立 R0022）

catalog 本体（domain/app/handler，无 store）已建。消费侧与连带登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| 7 个 `AsCatalogSource()` 实现 + `RegisterSource` | 各实体域（波次 3）+ boot 装配 | function/handler/workflow/agent/skill/mcp/document 各实现 `CatalogSource{Name,ListItems}`，交「名字+描述」；随各域建好接入 |
| 实体强制 name + description | 各实体域创建校验（波次 3） | catalog 信任非空、不做回退兜底；creation 必填 name+description |
| 搜索工具（概览的下游） | `tool/search`（波次 2） | LLM 看完 catalog 概览 → 调 `search_<kind>` 拿精确实体(id/详情)再用；catalog 不管调用 |
| catalog handler + chat 注入装配 | M7 / 波次 5 | `NewCatalogHandler(...).Register(mux)`；chat runner 经 `SystemPromptProvider` 注入 |

## 来自波次 1 · M1.6（mention 建立 R0023）

mention 纯 domain 契约已建。消费侧登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| 5 个 `AsMentionResolver()` 实现 | 各实体域（波次 3） | function/handler/workflow/agent/document 各实现 `Resolver{Type,Resolve}`，抓内容快照成 `Reference` |
| chat 注册表 + `<mentions>` 渲染 + 错误处理 | chat（波次 5） | type→resolver 注册表；发送时解析；统一 `<mention>` 标签 + snapshot 标记；resolver 未注册/解析失败的处理（回退 stub 不中断）|
| `Reference` 快照持久化 | chat / messages（波次 5） | 存进 `messages.attrs` 的 mentions 数组 |

## 来自 R0024（notification 建立 + stream 清理 + R0018 翻转）

| 关注点 | 去向 | 备注 |
|---|---|---|
| notification app 注入 notifications Bridge + SSE 端点装配 | M7 | `NewService(repo, bridge, log)` 的 bridge = Bus notifications 实例；handler SSE 端点接 router |
| 各 producer 注入 `notification.Emitter` | 各模块（波次 1+/3/5） | memory（下一步）、sandbox/apikey/conversation… 发通知 = `Emitter.Emit(type, payload)` |
| `~/.forgify/workspaces/<wsID>/` 分桶布局落地 | M7 | R0018→R0024 翻转：memory/skills/settings/mcp 各 workspace 一份；boot 路径 |
| scope-relation EntityKind 收口 | 单独评估 | stream scope 实体 kind 与 relation.EntityKind 重叠；实体 kind 词表归一（依赖方向待定）|
| 通知自动清理（保留 N 条/N 天） | 回头 | 当前只增不删；用户手动删/自动清理延后 |

## 来自波次 1 · M1.7（memory 文件式 R0025）

memory 文件式 store + app + handler 已建。消费侧 / 装配登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| ~~`read/write/forget_memory` 工具~~ **✅ R0044** | 波次 3（M3.7） | `app/tool/memory` 包 Get/Upsert/Delete；write **无 type**、`source=ai` 内定、不暴露 `pinned`。装入 `Toolset.Lazy` 留 M7 |
| chat 注入 memory 段 | 波次 5 | chat runner 经 `SystemPromptProvider.ForSystemPrompt` 注入 |
| `~/.forgify` base 路径 + `notification.Emitter` 注入 | M7 / boot | fs store `New(base)`；app `NewService(repo, emitter, log)` |
| skills 复用文件 store 范式 | 波次 3 | skills 也是 frontmatter md 文件，复用 infra/fs 模式 |

## 来自波次 0 · M0.3（logger broadcast 删除 R0010）

`LogBroadcaster`（日志 SSE 流，违反 E1 三流）已判删。连带清理（M7 wiring）：

| 移出内容 | 原位置 | 去向 | 备注 |
|---|---|---|---|
| dev 日志流 SSE 端点 | `handlers/dev.go` | cmd/server（M7） | 随 broadcaster 删；dev 端点整体去留也在 M7 判定 |
| broadcaster 接线 | `main.go`（`NewLogBroadcaster` + 作 extra core 传 `logger.New`） | cmd/server（M7） | `logger.New` 已简化为 `New(dev)`，无 extras |
| broadcaster 注入 | `router/deps.go` | transport（M7） | 去除注入 |

## 来自波次 0 · M0.3（crypto 迁移 R0011）

| 待办 | 位置 | 去向 | 备注 |
|---|---|---|---|
| encryptor 构造 | `main.go` | cmd/server（M7） | 用 `crypto.NewAESGCMEncryptor(crypto.DeriveKey(crypto.MachineFingerprint()))` 现场派生；判定旧 `~/.forgify/encryption-key` 文件是否残留（机器指纹方案无需存 key 文件） |

## 来自波次 0 · M0.4（errors 强化 R0012）

| 待办 | 位置 | 去向 | 备注 |
|---|---|---|---|
| errTable 集中映射（293 行 + 27 import） | `transport/errmap.go` | transport（M0.7） | 塌缩成 `statusForKind(Kind)` + `errors.As(*Error)`；零 domain import；`context.Canceled`/`DeadlineExceeded` 等 stdlib 特例单列 |
| 各 domain error 改造 | 各 domain（M1.x+） | 各模块轮 | `errors.New(msg)` → `New(kind, code, msg)`；保留原 wire code（对齐 error-codes.md） |
| 错误码对账测试 | 待建 | M0.7 / 覆盖阶段 | 扫所有 `Error{Code}` 校验唯一 + 对齐 error-codes.md（取代人肉维护 293 行大表） |

## 来自波次 0 · M0.4（SSE 三流统一协议 R0013）

三流改名 + 统一「流式树」协议（见 `stream-protocol.md`），下游 ~20 目录全 app 层按新协议重写：

| 待办 | 原位置 | 去向 | 备注 |
|---|---|---|---|
| producer 辅助统一 | `pkg/{eventlog,forge,notifications}`（Emitter/Publisher 三套） | `pkg/streamemit`（含 `EmitBoth` 双输出） | forge/notif producer 薄，随 M0.5；messages 的 chat 双写依赖随 chat（M5.2） |
| type 常量下沉 | 旧三流 node 词表常量（text/tool_call/forge/run/entity_changed/...） | 各 producer 业务模块 | Node 通用化连带：domain 不持词表，由发它的业务定义登记；前端契约靠 events.md（覆盖阶段）+ TS 类型 |
| messages DB 落盘 + History | `pkg/eventlog.Emitter` 双写 chat blocks；`GET /conversations/{id}/eventlog` | chat（M5.2） | 落盘只 messages 有；端点改 `/conversations/{id}/messages`；供 410 后全量重放 |
| scope 级订阅判定 | infra bus 订阅模式 | M0.5 | `?scope=agent:ag_x` 精准订阅 vs workspace 全量推前端过滤；v1 倾向全量推、buffer 留 scope 过滤扩展点 |
| 各 app 模块 emit 改造 | loop·chat·scheduler·subagent·contextmgr·tool/{workflow,handler,function,agent}·workflow·handler·function·mcp·skill·ask·todo·sandbox·memory·document·conversation（~20） | 各自波次 | 旧 `eventlog.Emitter`/`forge.Publisher`/`notif.Publish` 调用 → 新 `streamemit` + 统一 `Event{scope,id,frame}` |
| installprogress→notif | `pkg/installprogress`（依赖旧 notifications） | M0.5 / 相关波次 | 判定 installprogress 去留 + 改用 streamemit signal |
| 对外契约重写 | `events.md`（旧三流全量事件表） | 覆盖阶段 | 按新协议重写 events.md；端点改名；前端/testend 改（见 contract-changes #2） |

## 来自波次 0 · M0.5（infra/stream R0014）

| 待办 | 原位置 | 去向 | 备注 |
|---|---|---|---|
| infra/chat extractor | `infra/chat/extractor.go`（import chatdomain） | chat（M5.2） | 依赖 chat domain，M0.5 做不了；随 chat 那轮重写 |
| 三流 Bus 实例化 + 注入 | — | M0.7 / cmd | messages/entities 按 `stream.Bridge` 注入、notif 按 `stream.ListReader`；buffer 大小 wiring 定（旧 messages 4096 / entities·notif 1024）|
| SSE 线缆 marshal | — | M0.7 | handler 把 `stream.Envelope` marshal 成 SSE（frame kind + node type 判别字段注入；ephemeral seq0 省 `id:` 行）；线缆形状见 stream-protocol §1-3 |

## 来自波次 0 · M0.6（infra/llm 框架 R0015）

| 待办 | 原位置 | 去向 | 备注 |
|---|---|---|---|
| trace（LLM 调用跟踪） | `infra/llm/trace.go`（`recordingClient` 依赖 `reqctx.GetConversationID`） | chat/loop（M5.2）+ dev（M7.2） | conv ctx 随 chat 重建后才能搬；dev tracing 去留 M7.2 判；factory 已去 tracer 钩子 |
| ~~其余 provider~~ ✅ R0016 完成 | — | — | 11 家全部移植（含 7 家 workflow 并行）、自包含 + -race + 合规绿 |

## 来自波次 0 · M0.7（transport 框架 R0017）

| 待办 | 原位置 | 去向 | 备注 |
|---|---|---|---|
| 完整 `router.New` + Deps 容器 | 旧 router.go 装配所有 handler + deps.go 容器 | cmd/server（M7.1） | 依赖整个 app；M0.7 只建 `Chain`+`Recorder` 框架；M7 构造 mux + 注册所有 handler + 过 Chain |
| auth `WorkspaceResolver` 实现 + 注入 | `middleware.WorkspaceResolver`(本地接口) | workspace（M1.1）实现 + M7 注入 | M1.1 workspace service 实现 `Validate(ctx,id)error`；M7 wiring 把它注入 `IdentifyWorkspace` |
| health handler | 旧 handlers/health | M7 / workspace 轮 | `GET /api/v1/health`；框架就绪，handler 随 M7 |
| stream handler（messages/entities/notifications） | 旧 handlers/{eventlog,forge,notifications} | 各业务（chat M5.2 / 实体各轮 / 通知） | 各 SSE 端点 handler 用 `StreamSSE` + `WriteStreamEnvelope`；随业务垂直切片 |
| 信息端点(providers/scenarios/capabilities/usage/...) | 旧 handlers | M7.2 逐个判定 | AI 加的信息端点去留 |
| `Request.MaxTokens` 由 caller 填 | provider 不读 catalog | caller（app/loop/chat 接线时） | anthropic `max_tokens` 用；caller 从 model catalog（M1.5）查 MaxOutput 填 `Request.MaxTokens`；0 → provider 默认。去除了 infra/llm → modelcatalog 依赖 |
| pkg llmclient/llmcost/llmparse | `pkg/{llmclient,llmcost,llmparse}` | 随 llm 完成 / 相关业务轮 | llmclient(解析 client 配置)·llmcost(费用估算)·llmparse(抽 JSON)；判保留 + 改用 backend-new llm |

## 来自波次 1 · M1.1（workspace 正名 R0018）

| 待办 | 原位置 | 去向 | 备注 |
|---|---|---|---|
| boot 默认 workspace | cmd/server | M7.1 | `Count()==0 → Create("默认工作区")`；前端 onboarding 经 List 拿到（**无固定 id**；EnsureExists 已删）|
| `WorkspaceResolver` 注入 | `app/workspace.Service.Validate`（已实现） | M7.1 | wiring 把 Service 注入 `middleware.IdentifyWorkspace`（M0.7 留的端口）|
| `~/.forgify/` 共享资源布局 | 旧 userpath（已删） | M7.1 | mcp.json/skills/settings/catalog **共享一份不分桶**（workspace=数据边界非文件边界，R0018 决策）；落地 boot 路径 |
| workspaces DDL 收集 | `infra/store/workspace.Schema`（`[]string`） | M7.1 | cmd/server 收集各模块 `Schema` 传 `db.Migrate` |
| `settings.json` ↔ workspace 偏好边界 | `settingsinfra` | settings 模块轮 | 判哪些偏好归 workspace 行、哪些留文件；`Language` 已进 workspace（第一个）|
| 通用 `validate` 包（观察点，**未建**） | — | ≥2 模块共享格式校验时 | 现 0 跨模块校验需求（wikilink 正则是私有实现）；YAGNI，勿提前建、勿污染 wikilink 语义 |

## 来自波次 1 · M1.2（apikey 收窄，设计敲定 2026-06-04，待重写）

> apikey 收窄为「加密保险箱 + 哑探针 + 按 id 发钥匙」，大量职责下放。详见 `contracts/apikey.md`。

| 待办 | 去向 | 备注 |
|---|---|---|
| ✅ R0020 模型解析 | 各家 `infra/llm` provider `DescribeModels` | 落地为**各家 provider 自解析自家 /models**（非共享 parseOpenAIModels）；apikey 探针存 `test_response`，model 经 `llm.DescribeModels(provider, raw)` 取 |
| ✅ R0020 模型目录 | 各家 provider 静态目录 + 富 /models 解析 | `modelcatalog` **弃用不迁**；知识(窗口/上限/旋钮)下沉各家 provider 自包含；删 `ThinkingSpec`/compile、各家原生 `Options`（静态数值随软件更新人工对账）|
| ✅ R0020 capabilities | `app/model.CapabilityService` + handler | `GET /model-capabilities` 聚合各 key `DescribeModels`（去 modelsFound、返原生 knobs）|
| 选搜索 key：`IsDefault`/`DefaultProvider`/`SearchProviderPriority`/`GetByProvider` 启发式 | **未来搜索配置模块** | 像 model 一样让用户显式配 api_key_id，**不启发式**（防乱烧钱）；随 WebSearch（波次 2）配套建 |
| `RefScanner` 注入（各实体 override 引用检查）| conversation/workflow/agent 各轮（波次 2/3/5）+ M7 | model_config 已废（默认搬 workspace 列）；各持 `*ModelRef` override 的实体各自实现 scanner 扫自身列/字段；Delete → ErrInUse；DIP 端口 apikey 这轮留 |
| `KeyProvider` 注入 | M7 | 28 模块按 id 取钥匙（`ResolveCredentialsByID`/`MarkInvalidByID`）|
| ✅ R0020 `pkg/modelcatalog` 错位 | 弃用不迁 | 知识下沉各家 `infra/llm` provider，无跨家 pkg 目录、无 model domain 依赖错位 |

## 来自波次 1 · M1.3（model 重写 R0020）

> model 退化为聚合薄层；override 消费、静态目录维护、连带 doc-fix 移走如下。

| 待办 | 去向 | 备注 |
|---|---|---|
| override 消费者 | chat / agent / workflow-node 各轮（波次 2/3/5）+ M7 | 各自读自身 `*ModelRef` override 列/字段 + 调 `model.Resolve(scenario, override, picker)`（override 优先否则默认）；各自实现 apikey `RefScanner` 扫自身 override 列/字段（弱引用，key 没了运行时 `MODEL_NOT_CONFIGURED`，不做删除时保护）|
| 静态模型目录数值对账 | 随软件更新人工 | 各家 `infra/llm/<家>.go` 静态目录（窗口/上限/旋钮）随厂商迭代**人工对账**，OpenAI 迭代尤快；非自动同步 |
| `pkg/limits/limits.go` modelcatalog 注释 | ✅ 已 doc-fix R0020 | 注释提已弃用的 `modelcatalog` → 改述（无遗留）|
| M1.2 apikey anthropic 探针 | ✅ 已改 R0020 | 探针从 `anthropic_ping` 改打 `/v1/models`（`anthropic_models`）；envelope 不变（doc-fix 历史断言"无 list-models 端点"）|

## 来自波次 1 · M1.8（sandbox 三 runtime R0026）

sandbox 三 runtime 隔离运行时（domain/store/infra/app/handler + 25 测试）已建。消费侧 / 装配 / Docker 精细化登记：

| 待办 | 去向 | 备注 |
|---|---|---|
| **Docker 容器生命周期精细化** | M3.6（mcp 那轮） | 优雅停止（`docker stop` + container-id 追踪，而非 kill 进程组留孤儿）、孤儿容器回收、stdio MCP 长连接 e2e——docker spawn 真正被消费验证处；本轮只基础 `docker run --rm -i` |
| function/handler 的 `Sandbox` 适配器 | 波次 3（M3.1/M3.2） | 消费者侧把 Service 包成各自 `Sandbox` 接口（function 一次性 Spawn、handler 常驻 SpawnLongLived + lazy rebuild on ErrEnvNotFound）；本模块只提供 Service |
| mcp docker 拉取接入 | 波次 3（M3.6） | mcp 镜像型 server 经 `EnsureEnv`（owner=mcp）+ `docker run` 起 stdio；docker `-e` env 注入已就位 |
| chat 对话 scratch env | 波次 5 | bash 工具自动路由 per-conversation python/node env（owner=conversation `<convID>_<kind>`）|
| 注册三 installer/envmanager + boot | M7 | `RegisterInstaller`(MiseInstaller python/node/uv + DockerInstaller) + `RegisterEnvManager`(Python/Node/Docker)；`New(repo, dataDir, emitter, log)` 注入 `~/.forgify` base + `notification.Emitter`；`Bootstrap`+`Shutdown` 生命周期钩子 |
| `make fetch-mise` cmd | M7.2 | embed mise 二进制按平台生成（`cmd/fetch-mise`，git 不入库、SHA256 钉）；本地已有 darwin-arm64 |
| sandbox handler 路由装配 | M7 | `NewSandboxHandler(svc, log).Register(mux)` 接入总 router |
| sandbox DDL 收集 | M7 | `infra/store/sandbox.Schema` 交 cmd/server `db.Migrate` |

## 来自波次 1 · M1.9（permissions/hooks/settings 判定解散 R0027）

permissions domain + app/hooks + infra/settings 整个不迁。碎片去向：

| 关注点 | 去向 | 备注 |
|---|---|---|
| 危险控制（灾难命令拦截） | 波次 2 工具（tool/shell 等） | **不做中央门控**；bash 等工具内置极少数硬拦截（`rm -rf /`·`sudo`…）防无人值守闯祸；不做 allow/ask/deny 配置系统、不做 ask 交互 |
| protectedPaths（写保护） | 波次 2 tool/filesystem | `pathguard`（M0.1）已有默认禁区（.git/.env/.ssh）；filesystem 工具直接用，不要用户可配 |
| limits 装配 | M7 | `pkg/limits.Current()` 走 `Default()`；**删** settings-backed `SetProvider`（旧 main.go 把 `settings.Limits` 接进去）——不接 settings |
| M5.4 tool/permissionsgate | 删 | 随 permissions 解散；chat（M5.2）危险控制改由工具自管 |
| 前端 settings/permissions UI | 覆盖阶段（contract #7） | testend Permissions.tsx + `/settings`·`/permissions` 调用全拆 |

## 来自波次 1 · M1.10（document 建立 R0028）

document Notion 树 + 4 适配器已建。消费侧 / 注入登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| 4 适配器注入 | M7 | catalog `RegisterSource(doc.AsCatalogSource())`、chat mention 注册 `doc.AsMentionResolver()`、`doc.SetRelationSyncer(relationSvc)`、relation `Config.Namers["document"]=docSvc`；双向（doc↔relation）注入避 init 环 |
| attach 消费（`ResolveAttached` + `RenderAttachedAsXML`） | 波次 4/5 | chat runner system prompt + workflow llm/agent dispatcher 注入挂载文档；AttachedDocument 已去 IncludeSubtree（只单篇） |
| AttachedDocument 去 IncludeSubtree 连带 | conversation（波次5）/ workflow node（波次4） | 那俩持 `[]AttachedDocument` 字段；去 includeSubtree；前端挂载 UI 去子树选项（contract #8） |
| `:iterate`（askai 编辑） | 波次 6 | document handler 的 `:iterate` + `BuildDocumentContext` |
| ~~`app/tool/document`（LLM 工具）~~ **✅ R0044** | 波次 3（M3.7） | 7 工具 search/list/read/create/edit/move/delete_document；`edit`→Service.`Update`、`delete` 砍 destructive、errorsdomain 软失败。装入 `Toolset.Lazy` 留 M7 |
| document handler 路由装配 + DDL 收集 | M7 | `NewDocumentHandler(...).Register(mux)` + `documentstore.Schema` 交 `db.Migrate` |

## 来自波次 1 · M1.11（todo 重铸 R0029）

todo TodoWrite 式重铸（reqctx 双种子 + domain/store/app/handler + 11 测试）已建。跨波次接线登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| **`TodoWrite` 工具**（唯一写入面） | 波次 2/3（app/tool 建后） | 薄包 `Service.Write(ctx, items)`；单工具整列替换；S18 九方法；description 教 LLM「整列发、恰一项 in_progress、做完即标」；app 已就绪 |
| **loop 每轮注入 `SystemReminder`** | M2.2（loop） | loop 每迭代调 `todoSvc.SystemReminder(ctx)` → 非空把未完成清单作 system-reminder 注入；这是「LLM 真用」的持续可见层 |
| **subagent loop 埋 `SetSubagentID`** | 波次 3（subagent） | subagent run 起 loop 时 `ctx = reqctx.SetSubagentID(ctx, subagentRunID)`；种子已埋、写入方那轮接 |
| **messages-bridge 实接 broadcast** | M7 boot | `todoapp.New(repo, bridge, log)` 的 bridge = Bus **messages** 实例（非 notifications）；nil 时只持久不推 |
| **TodoHandler 注册 + DDL 收集** | M7 | `NewTodoHandler(svc, log).Register(mux)`；`todostore.Schema` 交 `db.Migrate` |
| **前端真任务看板** | 覆盖回 backend/ 后前端兼容期 | 删旧（无真消费：`todo` 通知 handler 返 `[]`、误名 TodoTab 实为 Ask tab）；建真看板：`GET /conversations/{id}/todos` 初值 + 订 messages `todo` signal 实时更新 + 按 `subagentId` 嵌子树；testend 死链 `/todos` 改打新只读端点（contract #9）|
| **对话删除级联清理 todo 清单** | conversation（波次 5）/ 后波次 | 对话删 → `Query(conversation_id)` 软删其主清单 + 所有 subagent 清单；store 已留 `idx_todos_ws_conversation` 索引 |
| reqctx 双种子已埋（conversation_id + subagent_id）；旧 `agentrun.go` 余项（messageID/toolCallID/parentBlockID）仍待 | M2.2/M5.2 | 见本文件顶「对话/执行标识 ctx」行——todo 只需 conv+subagent，已落；其余随 chat/loop 那轮 |

## 来自波次 2 · M2.1（tool 基础接口 R0030）

tool 5 方法接口 + 三字段注入剥离 + Toolset 懒加载已建。跨波次接线登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| **danger 确认流** | loop M2.2 / chat M5.2 | loop 跑 `StripStandardFields` 拿 `Danger`；`dangerous` → 暂停弹用户确认（走 ask/answers 机制）再 Execute；`safe`/`cautious` 直接放行（cautious 前端标记）。无人值守 workflow 的 `dangerous` 策略（自动放行/锻造预批）波次 4 定 |
| **execution_group 并行批** | loop M2.2 | loop 按 `ExecutionGroup` 分桶：同组并行、组间串行；≤0 自动分组排显式之后 |
| **activate_tools 工具 + 激活状态 + lazy 类 prompt + host.Tools 组装** | chat M5.2 | `activate_tools(category)` 工具；每对话激活了哪些 category 的状态；system prompt 报「有哪些 lazy 类」；`host.Tools(ctx)` = Resident + 已激活 Lazy 组 → `ToLLMDefs` 喂 LLM |
| **工具适配器全景** | 各波次 | ✅ 已建：filesystem/search/web/toolset（2.3）· function/handler/trigger/agent/skill/mcp（波次 3 各实体域）· **memory/document/shell（M3.7 R0044）** · **control（R0045）· approval（R0046）**（波次4前置，各 create/edit/revert/search/get/delete 共 6 工具、无 run、Lazy）。⏸ 待后续波次：todo（chat M5.2，SystemReminder 耦合）· subagent（波次 5）· workflow（M4.5）· ask（波次 6）。**shell 进 `Toolset.Resident`**（同 filesystem/search，chat M5.2 host 装）；其余实体工具进 `Toolset.Lazy` |
| workflow/subagent 的 host.Tools | 波次 3/4 | 固定预过滤切片（无 lazy / activate_tools）——非交互场景全量给 |
| Resident/Lazy 分类表 | chat M5.2 / M7 boot | 旧 main.go `residentToolNames`+`lazyGroups` 两张封闭表；装配时定哪些常驻、哪类懒加载 |
| 工具总览 handler（`Toolset.All`） | M7（判定） | 若要 `GET /tools` 巡检端点（旧"§18 inventory"），去留 M7 定 |

## 来自波次 2 · M2.2（loop ReAct 引擎 + messages domain R0031）

loop 6 文件 + `domain/messages` 类型契约 + reqctx messageID 种子已建。跨波次接线登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| **agentstate 重建**（4 块职责，loop 本身零依赖） | filesystem/shell（2.3）· chat（M5.2）· skill（M3.5） | SeenFiles(写前必读)→filesystem、cwd→shell、activatedGroups(懒加载激活账本)→chat host、activeSkill→skill；**创建者 chat**（每对话起一个 AgentState 挂 ctx）；skill pre-approval 随 CheckPermissions 删而失靶、不重建 |
| **message_blocks store/落盘/History + Message 实体 + 隔离列** | chat M5.2 | `Block` 带 db tag 已立（`blk_` 前缀已登记），但表 DDL/CRUD/workspace_id 隔离/Seq 分配/History 查询留 M5.2；loop 经 `host.WriteFinalize` 外包落盘、自身不碰表 |
| **WithBridge 注入 + messageID 种子写入** | chat/agent host（M5.2/M3.4）+ M7 | host 在 `loop.Run` 前 `WithBridge(ctx, bus.messages)` + `reqctx.SetMessageID(ctx, msgID)`；workflow-agent **不注入**（非流式，emitter 自禁用）；subagent host 还 `SetSubagentID` |
| **danger 阻塞确认**（dangerous → 暂停等用户同意） | 波次 6（ask 通道） | M2.2 纯标记（tool_call 节点带 danger 前端标记、不阻塞）；loop 留接口位，ask/answers 就绪后接入；无人值守 workflow 的 dangerous 策略波次 4 定 |
| **StepRecorder（ADR-010 子步重放）** | workflow-agent（波次 4） | 可选 Host 能力；chat host 不实现；workflow flowrun :replay 用 journal 重建历史、跳过已完成步 |
| **chat host 实现 Host 接口** | chat M5.2 | LoadHistory（用 `BlocksToAssistantLLM` 把 DB blocks 转 LLMMessage）/Tools（resident+激活 lazy）/WriteFinalize/`ReminderProvider`（注入 todo `SystemReminder`）/`AutoActivator`（用 activatedGroups） |
| **node content 词表 + events.md 全量重写 + 前端 messages 流重渲** | 覆盖阶段（contract #2/#11） | text/reasoning/tool_call/tool_result 的 content 形状（loop 那份词表）；events.md 按新协议重写；前端从 eventlog block 改 messages 流 frame+node.type 判别 |

## 来自波次 2 · M2.3#1（tool/filesystem + 首建 pkg/agentstate R0032）

filesystem 三件套 + `pkg/agentstate{SeenFiles}` + reqctx `WithAgentState/GetAgentState` 种子已建。跨波次接线登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| ~~`agentstate.cwd` 字段 + Bash `cd` 追踪~~ **废弃 R0033** | — | **cwd 概念全局取消**:桌面 agent 无项目根 / 无当前目录、永远绝对路径(`~` 由 `fspath` 展开)。`pkg/agentstate` 永不加 cwd 字段;shell M3.7 也无 cwd 持久化——Bash 命令若需工作目录,命令内显式 `cd /abs &&`,不跨调用记忆 |
| **`agentstate.activeSkill` 字段 + skill 预授权域** | skill M3.5 | 在 `pkg/agentstate` 追加 `activeSkill activeSkillSlot`（slot 含 skill id + 允许的工具白名单）；skill 调用时把白名单内工具标"已预授权"避免每次确认；M1.9 砍 CheckPermissions 后失靶——skill 这一轮重新设计预授权语义（可能就是"工具调用不弹 dangerous 确认"） |
| **`agentstate.activatedGroups` 字段 + Toolset lazy 激活账本** | toolset M2.3 后续 + chat M5.2 host | 在 `pkg/agentstate` 追加 `activatedGroups map[string]bool + sync.Mutex`；`activate_tools(category)` 工具更新；chat host 的 `AutoActivator` 钩子（loop 调）读它决定 `host.Tools(ctx)` 该不该露 lazy 组 |
| **`WithAgentState` 调用方接线** | chat M5.2 / subagent M3.3 / scheduler M4.3 | 每对话 host 起一个 AgentState 挂 ctx；subagent 继承父 ctx 还是独立新建（防 SeenFiles 污染父）波次 3 定；scheduler 跑 workflow 内 agent 时是否需要独立 state 也波次 4 定 |
| **三工具装入 `Toolset.Resident`** | chat M5.2 host 组装 | filesystem 是常驻工具典型例（巨大 description 不需 lazy）；`FilesystemTools(pathGuard)` 返切片直接进 Resident |
| **`PathGuard` 实例** | server boot M7 | `pathguardpkg.NewDefault()` 拿 `DefaultDenyList ∪ DefaultWriteOnlyExtras`；本轮已用、boot 装配即可 |
| **testend 工具断言改名 `fs_*`→`Read/Write/Edit`** | 覆盖阶段（contract #12） | 旧文档错把工具名写成 `fs_read/fs_write/fs_edit`；若 testend 真按假名断言（疑似——文档腐烂的常见连锁）改首字母大写 |

## 来自波次 2 · M2.3#2（tool/search + 新建 pkg/fspath + cwd 全局废弃 R0033）

search 三件套(LS/Glob/Grep) + `pkg/fspath.Expand` + filesystem 回溯补 `~` 已建。跨波次接线登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| **cwd 概念全局废弃** | shell M3.7 + 一切文件工具 | 桌面 agent 全电脑范围、靠交互定位,无项目根/无 cwd。shell 的 Bash 也无 cwd 持久化(命令内显式 `cd /abs &&`,不跨调用记忆);`agentstate` 永不加 cwd 字段。**R0032 的"cwd→shell M3.7"已撤销** |
| **search 三工具装入 `Toolset.Resident`** | chat M5.2 host 组装 | `SearchTools(pathGuard, log)` 返切片直接进 Resident(同 filesystem) |
| **`rg` 二进制** | 不代装 | `exec.LookPath("rg")` 探测,无则 stdlib 兜底;同 sandbox docker"不代装"决策 |
| **`fspath.Expand` 共用** | 所有文件工具 | filesystem 3 + search 3 工具 path 的唯一解析点(展开 `~` + 必绝对) |
| **testend search 工具断言** | 覆盖阶段(contract #13) | 旧假名 `grep_search`/`glob` → `Grep`/`Glob`;新增 `LS`;path 参数断言改"必填、绝对或 `~`" |

## 来自波次 2 · M2.3#3 前置（搜索配置 domain/websearch + workspace 列 R0034）

`domain/websearch`(Provider 词表 + SearchKeyPicker 接口)+ workspace `default_search_key_id` 列已建。跨波次接线登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| **WebSearch 消费 SearchKeyPicker + Provider 常量** | ✅ tool/web（R0035 已实接） | `picker.DefaultSearchKeyID(ctx)` → `keys.ResolveCredentialsByID(id)` → `websearch.IsProvider(creds.Provider)` ? switch → searchBrave/Serper/Tavily/Bocha;`workspace.Service` 已实现 picker(boot M7 注入 web) |
| ~~**WebSearch MCP tier**~~ **废弃 R0035** | — | 改判删除:MCP 搜索经 `tool/mcp`(M3.7)**平级暴露**给 LLM、直接调,WebSearch 不代理。web 零 mcp 依赖。引导文案提"装 duckduckgo MCP"是纯文字、零代码耦合 |
| **前端"默认搜索 key"选择器** | 覆盖阶段（contract #14） | Settings 从 `category=search` 的 apikey 单选 → PUT/DELETE default-search;读 workspace 响应 `defaultSearchKeyId` |
| **boot 注入 web 的 SearchKeyPicker** | server boot M7 | `workspace.Service` 实例传给 `WebTools(...)`（同 model.ModelPicker 的注入） |

## 来自波次 3 · M3.1（function 重写 + 抽 app/envfix R0037）

function（domain/store/app/tool/handler）+ 共享 `app/envfix` 已建。消费侧 / 装配登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| **`app/envfix` 给 handler 复用** | M3.2 handler | handler 也装 env（有 deps）→ 复用 `envfix.Provisioner`（注入 sandboxapp.Service + picker + keys + factory）；handler 的 SandboxRunner 用 `SpawnLongLived`（常驻）而非一次性 Spawn |
| **`app/envfix` 给轮询触发源复用** | 「单独一种」那轮 | 轮询源（用户后面单独算的触发概念）若需 sandbox env 同样复用 envfix |
| **`envfix.Sink` live 推流** | chat host M5.2 | 当前 create/edit 工具用累积 `forgeSink` 把尝试折进结果；M5.2 建「tool-progress 流缝」后，Sink.OnAttempt/OnFixing 同时推 messages/forge 流 |
| **boot 装配 envfix.Provisioner + function SandboxRunner** | M7 | `envfix.NewProvisioner(sandboxapp.Service, picker, keys, factory, log)` + `functionapp.NewSandboxAdapter(sandboxapp.Service, dataDir)` 注入 `functionapp.NewService` |
| **function 3 适配器注入** | M7 | catalog `RegisterSource(fnSvc.AsCatalogSource())`、chat mention 注册 `fnSvc.AsMentionResolver()`、`fnSvc.SetRelationSyncer(relationSvc)`、relation `Config.Namers["function"]=fnSvc`（fnSvc 实现 `NamesByIDs`）|
| **function 9 工具进 `Toolset.Lazy`** | M7 boot / chat host | `functiontool.FunctionTools(fnSvc)` 返 9 工具 → 加入 Toolset.Lazy（懒加载实体工具，经 search_tools 浮现）|
| **function handler 路由 + DDL 收集** | M7 | `NewFunctionHandler(fnSvc, log).Register(mux)`；`functionstore.Schema`（3 表）交 `db.Migrate` |
| **`triggered_by` 写入方接线** | agent M3.4 / workflow M4 / chat M5.2 | tool 已按 reqctx 有无 subagent 区分 chat/agent；agent host 显式 set agent、workflow dispatcher 调 `Service.RunFunction` set workflow、HTTP 手动已 set manual |
| **`:iterate`（askai AI 编辑）** | 波次 6 | function handler 的 `:iterate` + `BuildFunctionContext` + `SetSpawner`（askai 那轮加，本轮端点不挂）|
| **execution `tool_call_id` 种子** | chat M5.2 | reqctx 暂无 `GetToolCallID`（旧 agentrun.go 余项随 chat 那轮）；Execution.ToolCallID 当前留空，M5.2 接 |

## 来自波次 3 · M3.2（handler MCP 式单例常驻 R0038）

handler（domain/store/app/tool/handler + infra/handler client + 单例进程管理器）已建。消费侧 / 装配登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| **`Service.Boot(ctx)` / `Shutdown(ctx)` 注入生命周期** | server boot/退出 M7 | boot 调 `handlerSvc.Boot(ctx)` 开局起常驻实例、进程退出调 `Shutdown(ctx)` 优雅关全部。**多 workspace boot 编排 + workspace 切换时起停**（切走停旧 ws、切入起新 ws，还是全程常驻？）留 M7 定——manager 按 handlerID 全局键，跨 workspace 实例可共存 |
| **boot 装配 envfix.Provisioner + SandboxRunner + encryptor + clientFact** | M7 | `envfix.NewProvisioner(sandboxapp.Service,...)` + `handlerapp.NewSandboxAdapter(sandboxapp.Service, dataDir)` + `crypto` 注入 `handlerapp.NewService`；clientFact 默认 `DefaultClientFactory` |
| **handler 3 适配器注入** | M7 | catalog `RegisterSource`、mention 注册 `AsMentionResolver`、`SetRelationSyncer`、relation `Config.Namers["handler"]=handlerSvc` |
| **handler 11 工具进 `Toolset.Lazy`** | M7 / chat host | `handlertool.HandlerTools(handlerSvc)` → Toolset.Lazy |
| **handler handler 路由 + DDL 收集** | M7 | `NewHandlerHandler(svc, log).Register(mux)`；`handlerstore.Schema`（3 表）交 `db.Migrate` |
| **workflow tool 节点 kind=handler 调方法** | M4 | dispatcher 调 `handlerSvc.Call`（TriggeredBy=workflow）|
| **`triggered_by` 写入方 + StreamCall 进度推流** | agent M3.4 / chat M5.2 | tool 已按 ctx subagent 区分 chat/agent；StreamCall 的 OnProgress 推 messages 流（同 function env-fix sink，M5.2 tool-progress 流缝）|
| **`:iterate`（askai AI 编辑）** | 波次 6 | handler handler 的 `:iterate` + BuildHandlerContext（那轮加，本轮端点不挂）|
| **call `tool_call_id` 种子** | chat M5.2 | 同 function——reqctx 暂无 GetToolCallID，Call.ToolCallID 留空 M5.2 接 |

## 来自波次 3 · M3.3（trigger 独立实体 R0039）

trigger（domain/store/app/tool/handler + infra/trigger 4 listener + 新建 `pkg/cel`）已建。消费侧 / 装配登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| **Firing claim → 建 flowrun**（消费收件箱） | scheduler M4.3 | scheduler drain `ListPendingFirings` → `triggerstore.ClaimFiring(firingID, create)`（单事务 ADR-021，create 在同 tx 内建 flowrun——store 已备此具体方法）；trigger 这轮只写 Firing |
| **workflow 开关 = Attach/Detach** | workflow + scheduler M4 | workflow `:activate` → 对其引用的每个 trigger 调 `triggerSvc.Attach(triggerID, workflowID)`；`:deactivate`/删 → Detach。**active = 永久监听 / 手动跑一次 = 监听额度 1**（arm-once：收一个信号即 Detach；"等下一刻度还是立刻跑"那轮定）|
| **boot 重建 Attach + Start/Shutdown** | server boot M7 | boot 调 `triggerSvc.Start()`（启所有 listener）+ 遍历 active workflow 重新 `Attach`（内存引用计数的持久真相在 workflow 侧）；进程退出调 `Shutdown()` |
| **注入 SensorInvoker（function/handler 适配器）** | M7 | sensor 探测靠 `sensorinfra.SensorInvoker.Invoke(ctx, kind, id, method)`；function app / handler app 各包一个适配器（`function.Run` / `handler.Call`），boot 注入 `triggerapp.NewService` 的 invoker 参数 |
| **boot 装配 + 路由 + DDL 收集** | M7 | `triggerapp.NewService(repo, mux, invoker, log)` + `SetRelationSyncer`；`NewTriggerHandler(svc, log).Register(mux)`；`triggerstore.Schema`（3 表）交 `db.Migrate`；webhook listener 与 HTTP server 共享同一 `mux` |
| **trigger 适配器注入** | M7 | catalog `RegisterSource(triggerSvc.AsCatalogSource())`；relation `Config.Namers["trigger"]=triggerSvc` + `SetRelationSyncer`（sensor `equip` 边 + 对话 `create` 边）|
| **trigger 8 工具进 `Toolset.Lazy`** | M7 / chat host | `triggertool.TriggerTools(triggerSvc)` → Toolset.Lazy（懒加载实体工具，经 search_tools 浮现）|
| **`pkg/cel` 给 workflow 节点控制复用** | workflow M4 | `pkg/cel.Compile/Eval/EvalBool`（读 `payload`/`ctx`、无 `now()`）已建；workflow case.when / emit / tool.args 的 CEL 直接复用，不再各写一份（是否还需扩 env 变量那轮定）|
| **workflow→trigger 监听边 + 删旧 workflow 内嵌 trigger 节点** | workflow M4 | workflow 不再有 trigger 节点；改为引用独立 trigger（产 `workflow → trigger` 监听边）；旧 `GET /workflows/{id}/triggers` 端点语义改为"列出该 workflow 引用的 trigger"|
| **missed-tick cron 补跑** | 择机 | 这轮 cron 不做跨重启补漏（旧靠 `schedule.lastFire` 持久化，已随 schedule 表砍）；要补可用 Activation 日志的最后 fired 时间重建 |
| **`:iterate`（askai AI 编辑）** | 波次 6 | trigger handler 的 `:iterate`（那轮加，本轮端点不挂）|

## 来自波次 3 · M3.4 考古（subagent 后移波次 5）

考古 subagent 后判定**后移波次 5（贴 chat）**——非因乱，而是 subagent 两个核心行为是 chat host 子集，波次 3 做只能 stub：

| 关注点 | 去向 | 备注 |
|---|---|---|
| **subagent 落盘** | 波次 5 chat | subagent 无自己的表，写**父对话 messages 表**（sub-message + parentBlock 锚点 E3 递归）；chat 表波次 5 → 与 chat host 共享落盘端口 |
| **subagent model 承袭** | 波次 5 chat | subagent 无 model 配置、承袭父 effective model；reqctx 不带 override、只 chat 知父 override → 与 chat 同轮解析 |
| **skill fork 端口（M3.5 这轮留空）** | M3.5 skill | skill 定 `SubagentService` 端口 + `subagent==nil` 优雅降级（旧代码已有）；subagent 波次 5 就绪后 boot 注入、fork 才生效。**非 fork 模式照常做** |
| **防递归** | 波次 5 | 旧 `subagentDepth`(int) → 新 `SubagentID`(string) 存在性：子 run `SetSubagentID` 后再 spawn 即拒（种子 R0029 已埋，本为 todo 作用域）；限 1 层 |
| **agentstate 隔离** | 波次 5 | 子 run **独立新建 AgentState**（否则 SeenFiles 写前必读账本污染父对话）|
| **3 内置类型** | 波次 5 | Explore/Plan/general-purpose（借 Claude Code）；AllowedTools 对齐新 Toolset（Resident 安全子集 + 新 search_* 命名 + 加 LS）|
| **`tool/subagent` 工具** | 波次 5（随 subagent）| SubagentTool 9→5 方法；防递归守卫；属 chat 场景分治工具 |
| **契约 DOC-123 整篇重写** | 波次 5 | 旧文档严重腐烂（虚构 cv_xxx 子对话 / 深度限 2 实为 1 / AgentState 沙箱 / 4 个虚构错误码 ErrRecursionTooDeep·ErrSubagentCrash·ErrTaskAmbiguous·ErrToolAccessDenied 代码全无）|

**附：agent 顺序后移（同次考古连带）**——agent 挂载 skill/mcp/doc/fn/hd/model，现做挂载只能 stub。波次 3 顺序调成 **skill(M3.4) → mcp(M3.5) → agent(M3.6 压轴)**；agent 挂载件齐全后一次做完整。agent 重写跟进方案 A **砍 pending/accept**（孪生 function/handler）、**不需 sandbox/envfix**（不跑代码）、execution 面对齐 function。skill `polling.go` / mcp `searchrouter.go` 去留考古时判（疑分别与 trigger/sensor、R0035 删 MCP tier 重叠）。

## 来自波次 3 · M3.4（skill 文件式 R0040）

skill（domain/skill + infra/fs/skill + app/skill + 5 工具 + handler + agentstate activeSkill）已建。`polling.go` 判定=**文件热重载非 trigger/sensor**，已砍（改纯按需扫描）。跨波次接线登记：

| 关注点 | 去向 | 备注 |
|---|---|---|
| **fork `SubagentRunner` 注入** | subagent 波次 5 | `skillapp.NewService(repo, runner, ...)` 的 runner = subagent 适配器（实现 `skilldomain.SubagentRunner.Spawn(ctx, agentType, prompt)→result`，包 subagent.Spawn 把 agentType 映射到内置类型）；波次 5 前 nil → fork 返 `ErrSubagentUnavailable`、inline 完整可用 |
| **allowed-tools 预授权消费** | ask 波次 6 | danger 确认流查 `agentstate.IsToolPreApprovedBySkill(tool)` → 命中免逐次确认；这轮只在 activate 时 `SetActiveSkill` 存字段 |
| **`${CLAUDE_SKILL_DIR}` + L3 附加文件** | 择机 | 目录式已留结构（`skills/<name>/`）；附加 references/scripts 按需读 + `${CLAUDE_SKILL_DIR}` 占位那轮加 |
| **boot 装配** | M7 | `skillfs.New(~/.forgify)` → `skillapp.NewService(repo, subagentRunner, emitter, log)` + `SetRelationSyncer` → catalog `RegisterSource(svc.AsCatalogSource())` + relation `Config.Namers["skill"]=svc` → `SkillTools(svc)` 进 `Toolset.Lazy` + `NewSkillHandler(svc, log).Register(mux)` |
| **user-invocable 前端 slash 入口** | 前端覆盖期 | frontmatter `user-invocable` 已解析存；前端 slash command 触发 UI 那轮接 |

## 来自波次 3 · M3.5（mcp 市场对接 + 加密表 + 双 transport R0041）

mcp 模块逻辑完整 + 测试绿（fake sandbox/client/repo/registry，全离线）。**3 处留口已全部补完（R0042）**——含 sandbox 物理 runtime-tool **真机端到端跑通**。仅 boot 装配留 M7：

| 关注点 | 去向 | 备注 |
|---|---|---|
| **sandbox 物理 runtime-tool** | ✅ R0042 真机跑通 | mcp 用 `npx/uvx/dnx`（runtime 自带「拉包即跑」工具）。补了 5 点：① node ResolveExec 认 `npx/npm/node` → `<runtimeRef>/bin/<cmd>`（runtimeRef 现为**绝对** install dir）② python ResolveExec 认 `uvx/uv` → `tools.EnsureTool("uv")` 同目录（**uv 早就在**——sandbox python 是 uv-backed `uv venv`/`uv pip`，uvx 随 uv 自带；先前「uvx 需装 uv」是错判，已纠正）③ `dotnet.go` 新 `DotnetEnvManager`（dnx 拉包即跑，CreateEnv/InstallDeps no-op，dnx=`<rt>/dnx` **顶层**）④ spawn.go 空-Cmd 检查移到 runtime 查之后 + docker 放行（image entrypoint）+ 非-docker 传绝对 runtimeRef ⑤ spawn.go `prependPATH`（npx shebang `#!/usr/bin/env node` 需 `<rt>/bin` 在 PATH——端到端逼出的硬集成点）。**真机验证（darwin/arm64）**：`mise install dotnet@10.0.300` 真装成功、dnx 在 install 顶层；e2e（`infra/mcp/e2e_test.go` tag `e2e`）embed mise → node 22.22.3 → npx → go-sdk → **context7 v3.1.0 跑通 2 工具**，PASS 17.91s。单测 `resolveexec_test.go` + `path_test.go`。**注册留 M7**：dotnet EnvManager/installer + node/python/docker/dotnet 全注册随 cmd/server 装配（现都没注册）。 |
| **handler catalog 列方法名** | ✅ R0042 已做 | handler catalog_source 填 `Members` = active 版本方法名（`repo.GetVersion(ActiveVersionID).Methods[].Name`）；catalog mechanical 渲染 `memberLabel("handler")="methods"`；测试 `TestCatalogSource_ListsMethodNames`。 |
| **trigger sensor 绑 mcp.tool** | ✅ R0042（domain+relation）；Invoke 路由 → M7 | sensor target 加 `SensorTargetMCP`（TargetID=server / Method=tool）+ 校验 + relation `equip` 边（trigger→mcp）已加；测试 `TestValidateConfig_SensorTargets`。**实际 Invoke 路由**（周期 `CallTool` 喂 CEL）随 M7：`SensorInvoker` 的实现整体（function/handler/mcp 三路由）尚未写——这轮 sensor Invoke function/handler 也都是 M7 装配，mcp 同款 case 一起加。 |
| **boot 装配** | M7 | `mcpinfra.NewGitHubRegistrySource(~/.forgify/cache, log)` + `mcpstore.New(db, encryptor)` + `mcpapp.New(repo, registry, sandboxSvc, log)` + `SetRelationSyncer` → `svc.Boot(ctx)`（per-workspace 起常驻）+ `svc.Shutdown` 接退出；catalog `RegisterSource(svc.AsCatalogSource())` + relation `Namers["mcp"]=svc` + `MCPTools(svc)` 进 resident、`DynamicTools(ctx,svc)` 进 `search_tools` 检索池（host 组装，M5.2）+ `NewMCPHandler(svc, log).Register(mux)`；db.Migrate 收 `mcpstore.Schema` |
| **mcp.json export（互操作）** | 择机 | `ParseImport` 已做（Claude Desktop mcp.json → 加密表）；反向 export（表 → mcp.json，secret 占位）用户需要再加 |

## 来自波次 3 · M3.6（agent 配置好的 LLM worker R0043）

agent 模块逻辑完整 + 测试绿（store/app invoke fake LLM/tool 全离线）。boot 装配 + invoke 真实依赖注入：

| 关注点 | 去向 | 备注 |
|---|---|---|
| **boot 装配** | M7 | `agentstore.New(db)` + `agentapp.NewService(repo, notif, log)` + `SetRelationSyncer` → catalog `RegisterSource(svc.AsCatalogSource())` + relation `Namers["agent"]=svc` + `AgentTools(svc)` 进 `Toolset.Lazy` + `NewAgentHandler(svc, log).Register(mux)`；`db.Migrate` 收 `agentstore.Schema` |
| **SetInvokeDeps 三端口注入** | M7（invoke 真跑前提）| `LLMResolver`（model picker + apikey + llm factory → `LLMBundle`，agent 不拥有）/ `ToolsProvider`（全局工具池组装，host 装配层提供、chat 也用）/ `KnowledgeProvider`（document service 渲染 doc → 前缀）。未注入前 invoke 报错、CRUD 不依赖。逻辑 + fake 测试已覆盖（`TestService_InvokeRunsLoopAndRecords` fake LLM 跑通真 loop + execution 落表）；**真 LLM/工具/doc 跑通随 M7 装配**——比 mcp 轻（无需 sandbox 物理，loop + fake 已测完整逻辑）。 |
| **:iterate（AI 编辑对话）** | 波次 6（askai）| HTTP `:iterate` 经 askai spawner 开对话编辑 agent；askai 在波次 6，那轮加端点。 |
| **workflow agent 节点 invoke 路由** | 波次 4（workflow/flowrun）| workflow agent 节点经 `InvokeAgent(InvokeInput{TriggeredBy:workflow, FlowrunID, ReplaySteps, Recorder})` 调；ADR-010 子步重放（`RecordedStep`/`StepRecorder`）已在 `InvokeInput` 就位，flowrun `:replay` 接它。 |

