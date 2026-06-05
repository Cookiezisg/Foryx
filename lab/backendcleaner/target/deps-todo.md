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
| `read/write/forget_memory` 工具 | 波次 2/3 | 包 app 的 Get/Upsert/Delete；LLM 自管记忆 |
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
