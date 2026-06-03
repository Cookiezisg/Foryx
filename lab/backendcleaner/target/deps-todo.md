# 跨模块待办（从已重写模块移出、待目标模块建立的关注点）

> 重写某模块时，把不属于它的关注点移走后记在这里；到目标模块那一轮去建立或判定，确保不丢。

## 来自波次 0 · M0.1 第一轮（reqctx / idgen / pagination）

| 移出内容 | 原位置（问题） | 去向 | 备注 |
|---|---|---|---|
| model override ctx | `reqctx/modeloverride.go`（🔴 曾让 reqctx → `domain/model` 反向依赖） | model（M1.3） | `WithModelOverride`/`GetModelOverride`；在 model 模块重建其 ctx 透传 |
| agent state ctx | `reqctx/agentstate.go` | agent/loop（M2.2/M3.4） | `WithAgentState`/`GetAgentState` + `pkg/agentstate` 去留判定 |
| 对话/执行标识 ctx | `reqctx/agentrun.go` | chat/loop/messages（M2.2/M5.2） | conversationID·messageID·toolCallID·parentBlockID·subagentDepth；服务 messages 流递归(`Open.ParentID` 嵌套)；判定是否仍走 ctx 透传、放哪一层 |
| ID 前缀 → 实体类型 | `idgen/prefix.go` | **仅 relation（M1.4）** | `KindByPrefix`/`KindForID`；值 = `relationdomain.EntityKind*`。wikilink 已剥离 Kind（R0005），不再是消费者 |
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
| 前缀 → EntityKind 映射 + `KindForID` | wikilink（曾经经 idgen.KindByPrefix） | relation domain（M1.4） | relation 持 `EntityKind` 常量 + 前缀映射 + `KindForID(id)(EntityKind,bool)` |
| 未知前缀过滤 + Kind 解析 | `wikilink.Parse` | document（M1.10） | document 拿 wikilink 的 ID → `relation.KindForID` 解析 Kind + 过滤 + 跳过自链，再建 `SyncEdge` |
| Kind 映射测试用例 | wikilink_test（`DropsUnknownPrefix` / `AllSupportedPrefixes`） | relation（M1.4）测试 | 验前缀→EntityKind 全集；wikilink 侧已用 `ReturnsAllIdShapedTokens` 固定「不过滤」新语义 |

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
