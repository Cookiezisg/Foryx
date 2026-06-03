# Round 0013 — SSE 三流统一协议 domain（波次 0 · M0.4）

类型 / 目标：把三条 SSE 流从「各长各的异构事件协议」重构为「统一的流式树协议」的 domain 层。改名 + 语义升级 + 统一数据结构。设计蓝本 = `stream-protocol.md`。

考古发现（重构动机）：
- 三流数据结构异构（eventlog 5 强类型 / forge 4 强类型 / notifications 弱类型），但骨子里都是「对渲染树的增量操作流（open→delta→child→close）」，notifications 是退化（瞬时单点）。
- 三个 infra Bridge 95% 同构三抄 → M0.5 收敛单一 Bus。
- pkg/* 是 producer 辅助层（非残留）；旧 forge→eventlog 边 = 共享 Scope。

设计（与作者深度讨论定稿，含 2 次 review 修正）：
- **传输/语义正交**：domain 只管传输（信封 + 四动词 Frame），语义（节点是什么）交业务。
- 信封：`Envelope{Seq; Event{Scope, ID, Frame}}`——**ID 升信封层**（节点身份 universal，与 `Open.ParentID`「挂哪」正交）；Event/Envelope 两层 = seq 所有权边界（producer 草稿 vs bus 成品）。
- Frame 四动词封闭联合 Open/Delta/Close/Signal + `Durable()` **可丢性分级**：delta=ephemeral（不入 buffer）/ open·close·非ephemeral signal=durable（入 buffer，`Close.Result` 带快照）→ token delta 永不撑爆 replay。
- **Node 通用化（review 修正）**：`Node{Type, Content json.RawMessage}` 替代判别联合。domain **不**枚举 node 类型/字段——词表下放各 producer 业务（反校验剧场 + 零包袱：domain 不该猜它不知道的词表；曾标 PROVISIONAL 本身即此信号）。Type 自由字符串、可编码层级；Content 不透明 JSON、domain 从不解析。
- **砍三流 domain 包（review 修正）**：node 通用 + Bridge 通用 → messages/entities/notifications domain 包无内容，删。三流区别落到 infra 实例 + scope kind + wiring。`stream.Bridge`(Publish/Subscribe) 通用；`stream.ListReader`(+List) 给 notifications（无 DB 落盘读内存 buffer）。
- **error 内聚 domain/errors（review）**：stream 的 `ErrSeqTooOld`（上 HTTP 410）→ `KindGone`+SEQ_TOO_OLD、`ErrInvalidEvent`（producer bug）→ `KindInternal`+STREAM_INVALID_EVENT，从标准库 sentinel 升结构化——遵守 R0012「错误码内聚 domain、transport 零特例 statusForKind」；连带给 errors 加 `KindGone`(410)。

落地（仅 1 包 `domain/stream`，6 源 + 3 测试）：
- event/scope/node/frame/bridge/validate.go，纯 stdlib。
- node = `{Type, Content}`；frame `Close.Result = *Node`（可选快照）；validate 只校验骨架（scope kind / 节点 id / open·signal 的 node.Type / close 终态），不碰 Content。
- scope kind 全集（9 种实体）留 stream（与 relation.EntityKind 重叠，M1.4 收口）。

测试：6 — frame 可丢性分级（5 例）、ValidateEvent 通用不变量（合法 7 / 非法 7，含 close result 校验，errors.Is ErrInvalidEvent）、scope String + IsValidKind。

验证：`gofmt -l` 空 / `go build ./...` / `go vet` / `go test` 全绿。

是否更干净：✅✅ 经 2 次设计 review 收敛——先统一三流为流式树，再把 node 从「domain 替业务穷举判别联合」简化为「domain 只给 `{Type,Content}` 信封、词表下放业务」。domain 从 4 包 21 文件收到 **1 包 9 文件**。marshal/线缆形状留 M0.7（domain 不碰序列化）。

覆盖状态：三流 domain = 单一 `domain/stream`。infra bus + producer `streamemit` + **type 常量下沉各业务** + ~20 目录 emit 改造 + messages DB 落盘随 chat → deps-todo（R0013 节）。

下一步：M0.5 `infra/stream`（单一 `Bus`）+ `infra/chat`。
