# SSE 三流统一协议（流式树）— 设计蓝本

> M0.4（domain）+ M0.5（infra bus）的实现依据。一次定型，下游按此重写。
> 状态：✅ 已拍板（2026-06-03，与作者深度讨论后定稿）。

---

## 0. 一句话

三条 SSE 流（**messages / entities / notifications**）不是三个事件协议，而是**同一个「对渲染树的增量操作流」协议的三个实例**。设计的灵魂是 **传输机制与语义负载正交分解**：三流共享"怎么操作树"（信封 + 四动词），各自只定义"树上长什么"（Node 词表）。

旧名 → 新名 / 语义：

| 旧 | 新 | 语义 | 端点 | scope.kind |
|---|---|---|---|---|
| eventlog | **messages** | 聊天流式信息（消息树）；tool_call 内部"中间过程"作为子树流出 | `/api/v1/messages` | `conversation` |
| forge | **entities** | 所有实体的流式输出总线（创建/编辑流式内容 + 运行流式输出：function 终端、agent 对话） | `/api/v1/entities` | `function`/`handler`/`agent`/`workflow`/`document`/`mcp`/`skill` |
| notifications | **notifications** | 实体状态变更广播（刷列表/toast/角标） | `/api/v1/notifications` | `workspace`（全局） |

---

## 1. 信封层（三流逐字共享，一个字节不差）

```go
// domain/stream

// Scope —— 这个操作作用在哪棵树 / 哪个广播空间上。
type Scope struct {
    Kind string // conversation | function | handler | agent | workflow | document | mcp | skill | workspace
    ID   string // 锚点实体 id（workspace 流可空）
}

// Event —— producer 要发的内容（无 seq）。
type Event struct {
    Scope Scope  // 发到哪
    ID    string // 节点身份（树上节点地址）；universal —— 四个 Frame 都有
    Frame Frame  // 对树的一次操作
}

// Envelope —— bus 盖了 seq 章的 Event。
type Envelope struct {
    Seq int64 // bus 分配，每流单调；ephemeral 帧 = 0
    Event
}
```

**关键：`ID` 在信封层（节点身份，universal），不在 Frame 里。** `Scope.ID`（锚点）与 `Event.ID`（节点）语义不同，靠访问路径区分（`e.Scope.ID` vs `e.ID`）。

---

## 2. 四个动词（Frame）+ 可丢性分级

```go
type Frame interface {
    frame()        // unexported marker，封闭联合
    Durable() bool // 是否进 replay buffer（续传真相）
}

type Open   struct { ParentID string; Node Node } // 长节点；ParentID 空=顶层，非空=嵌套挂载点
type Delta  struct { Chunk string }                // 往节点流式追加（token/终端输出）
type Close  struct { Status string; Result Node; Error string } // 节点终结
type Signal struct { Node Node; Ephemeral bool }   // 退化：瞬时广播，不建树节点

func (Open) Durable() bool   { return true }   // 树结构必达
func (Close) Durable() bool  { return true }   // 终态 + 快照必达
func (Delta) Durable() bool  { return false }  // 打字机帧，丢了无所谓（close 会补最终内容）
func (s Signal) Durable() bool { return !s.Ephemeral } // entity-changed 必达；flowrun tick 可丢
```

**可丢性不能纯靠动词推断**：Delta 恒 ephemeral、Open/Close 恒 durable，唯 Signal 用 `Ephemeral` bool 区分（entity-changed=durable / flowrun-tick=ephemeral）。一个 bool 解决，不需要第五动词、不需要单独的 `PublishEphemeral` 方法。

**`ID↔ParentID` 正交**：`Event.ID`="操作哪个节点"（人人有）；`Open.ParentID`="新节点挂哪"（仅诞生时一次）。

---

## 3. Node —— 各流的词表（判别联合，强类型 + 可扩展 + 封闭）

```go
type Node interface { NodeType() string } // exported：marshal 注入 "type" 判别字段
```

灵活 ≠ `map[string]any`；灵活 = 可扩展的判别联合。加一种 UI 表现 = 加一个 type + 一个 struct。

| 流 | Node 词表（初版，可加） |
|---|---|
| **messages** | `message`(Role) · `text` · `reasoning` · `tool_call`(Name,Args) · `tool_result` · `progress` · `compaction` |
| **entities** | `forge`(Operation) · `run` · `env_attempt`(Attempt,Stage) · `terminal` · `message`(复用——agent 运行的对话双输出) |
| **notifications** | `entity_changed`(Kind,Action) · `flowrun_tick`(NodeID,Status,Ephemeral) · `flowrun_lifecycle`(Status) |

> message 也是一种 Node（type=`message`）——**一切皆节点**，无 message/block 双层；message 是顶层节点，text/tool_call 挂其下，tool_call 调的 agent 对话又是挂在 tool_call 下的 message 节点。全递归。

---

## 4. Bridge 端口

```go
// domain/stream —— 通用端口
type Bridge interface {
    Publish(ctx context.Context, e Event) (Envelope, error)            // bus 校验 + 盖 seq + 分发
    Subscribe(ctx context.Context, fromSeq int64) (<-chan Envelope, func(), error)
}
```

各流 thin 接口（DI 强类型注入，防接错流）：

```go
// domain/messages
type Bridge interface { stream.Bridge }
// domain/entities
type Bridge interface { stream.Bridge }
// domain/notifications —— 多 REST 快照拉取
type Bridge interface {
    stream.Bridge
    List(ctx context.Context, fromSeq int64, limit int) ([]stream.Envelope, bool, error)
}
```

校验 `ValidateEvent(Event) error` 在 `domain/stream`：scope.kind 合法、节点 id 非空、frame 内部一致（open 须带 node、close 须带 status）；各流 node 词表合法性由各流提供的校验补充。

---

## 5. infra bus（M0.5 实现要点）

- **单一 `Bus` 类型，非泛型**（三流共享 `stream.Envelope`），实例化三次 = E1 三条流；各自独立 buffer + seq 序列。
- **按 workspace 分流**（`reqctx.RequireWorkspaceID`）。
- **frame 分级 buffer**：`Publish` 给所有帧分发；仅 `frame.Durable()==true` 的进环形 buffer（seq>0）；ephemeral（delta / tick）实时扇出、seq=0、满则丢、不进 buffer。→ **buffer 永不被 token delta 撑爆**。
- **`Subscribe(fromSeq>0)` replay**：只回放 buffer 里的 durable 帧 → 重建**树骨架 + 每个节点最终内容**（`Close.Result` 携带）。唯一丢失=断线时正在打字节点的中间帧（其 close 后补齐）。太旧→`ErrSeqTooOld`→前端全量重取。
- **`Close` 必带节点最终内容/快照**（不是可选装饰，是续传正确性的支柱）。
- 扇出在锁内保序，slow subscriber 阻塞（durable）/ drop（ephemeral）；cancel 幂等。

---

## 6. 双输出（设计内，非冗余）

同一份内容（agent 运行的对话）发两条流，消费者不同：

```
producer 拿到 agent 一句 text delta:
  bridge.entities.Publish(Event{Scope:{agent, ag_x},        ID:nodeID, Frame:Delta{...}})  // 实体运行面板
  bridge.messages.Publish(Event{Scope:{conversation, c_x}, ID:nodeID, Frame:Delta{...}})  // chat 界面
```

- 数据**同构** → 双输出 = 同 Event 换 scope，包一个 `EmitBoth` helper 即可。
- **共享同一个节点 `ID`** → 跨流关联免费（chat ↔ 实体面板定位同一节点）。
- 代价（已接受）：2× 内存/扇出；两次独立 Publish 失败可分叉 → best-effort。

---

## 7. 已记录的关键决策与边界

1. **ID 提升信封层**（universal 节点身份；顺带白捡 ID↔ParentID 正交、跨流关联）。
2. **frame 按可丢性分级**（delta=ephemeral / open·close·signal[非tick]=durable；close 带快照）—— 化解 delta 高频撑爆 replay 的唯一大风险。
3. **三条流物理独立（E1）的真正理由**：消费者生命周期 / buffer 压力 / 背压策略三者不同——非历史教条。
4. **抽象边界**：协议假设"内容 append-only 流式生长"。将来若需"改已发节点/删中间节点/重排"，加第五动词（`Replace`/`Remove`），不破坏现有。
5. **DB 落盘只 messages 有**（chat blocks，M5.2 接线，供 410 后 `/conversations/{id}/messages` 全量重放）；entities/notifications 纯内存。
6. **待定（M0.5 定）**：scope 级订阅（`?scope=agent:ag_x` 精准订阅 vs workspace 全量推前端过滤）。v1 倾向 workspace 全量推；buffer 已按 workspace，加 scope 过滤 replay 作扩展点。

---

## 8. 下游影响（全 app 层，登记 deps-todo，各模块那一轮按新协议写）

旧 `domain/{eventlog,forge,notifications}` + `pkg/{eventlog,forge,notifications}` 被 ~20+ 目录依赖：loop · chat · scheduler · subagent · contextmgr · tool/{workflow,handler,function,agent} · workflow · handler · function · mcp · skill · ask · todo · sandbox · memory · document · conversation · installprogress · transport/handlers。

- producer 侧（`pkg/*` Emitter/Publisher）→ 统一为 `pkg/streamemit`（含 `EmitBoth`）；`pkg/eventlog.Emitter` 的 chat 双写依赖 → messages 流随 chat（M5.2）。
- reqctx 的对话/执行标识透传（conversationID/messageID/parentBlockID/subagentDepth）→ 服务 messages 流递归，随 chat/loop（M2.2/M5.2）重建。
- 对外契约（端点改名 + 数据结构全变）→ contract-changes.md 登记，覆盖阶段重写 events.md + 改前端/testend。
