---
id: DOC-301
type: reference
status: active
owner: @weilin
created: 2026-06-06
reviewed: 2026-06-06
review-due: 2026-09-01
audience: [human, ai]
---
# Messages Domain — 对话回合的内容模型

> **核心地位**：messages 是「一个 assistant 回合**由什么组成**」的中立内容模型——`Block` 树（reasoning / text / tool_call / tool_result）+ 流式 tool call 解析出的 `ToolCallData`。
>
> **与 `domain/stream` 正交**：stream 是**传输**（一帧怎么到前端：`Envelope/Frame/Node`），messages 是**内容**（回合由什么块组成）。共享 ReAct 引擎 `app/loop` 产 `Block` 并依赖**本包**、而非依赖 `chat` 这种具体消费者——故 `chat / agent / subagent / workflow-agent` 共享一个中立内容模型。这修正了旧架构「共享引擎 `loop` 依赖 `domain/chat`」的耦合反向。

---

## 1. 物理模型 (Data Anatomy)

```go
type Block struct {
    ID             string         `db:"id,pk"`             // blk_<16hex>
    ConversationID string         `db:"conversation_id"`
    WorkspaceID    string         `db:"workspace_id,ws"`   // D2 物理隔离；orm 自动填/过滤（落盘时设，loop 内存态不填）
    MessageID      string         `db:"message_id"`        // 所属回合；block 的 stream parentId
    ParentBlockID  string         `db:"parent_block_id"`   // tool_result → 其 tool_call
    Seq            int64          `db:"seq"`               // 落盘时分配（loop 不设）
    Type           string         `db:"type"`              // BlockType*
    Attrs          map[string]any `db:"attrs,json"`        // tool_call: {tool,summary,danger}; reasoning: {signature}
    Content        string         `db:"content"`
    Status         string         `db:"status"`            // Status*
    Error          string         `db:"error"`
    ContextRole    string         `db:"context_role"`      // 压缩器投影（contextmgr M5.3）；落盘默认 hot
    CreatedAt      time.Time      `db:"created_at,created"`
    UpdatedAt      time.Time      `db:"updated_at,updated"`
}

type ToolCallData struct {        // 内存解析形态，不原样落库（转成 tool_call Block）
    ID             string         `json:"id"`
    Name           string         `json:"name"`
    Summary        string         `json:"summary"`         // LLM 自报：本次调用意图
    Danger         string         `json:"danger"`          // LLM 自报：safe/cautious/dangerous（纯字符串，不引 app/tool）
    ExecutionGroup int            `json:"executionGroup"`  // 并行批键
    Arguments      map[string]any `json:"arguments"`       // 已剥 3 标准字段的业务 args
}

type Message struct {                 // 一个对话回合（user 发言 / assistant 生成），拥有 Block 树；落 `messages` 表（msg_）
    ID             string         `db:"id,pk"`              // msg_<16hex>
    ConversationID string         `db:"conversation_id"`
    WorkspaceID    string         `db:"workspace_id,ws"`    // D2 物理隔离；orm 自动填/过滤
    SubagentID     string         `db:"subagent_id"`        // ""=顶层；非空=subagent run（R0058）。chat LoadHistory 排除 != ""（不污染父历史）；ListMessages 仍返回（reload 重建子树，锚 = Attrs.parentBlockId）
    Role           string         `db:"role"`               // user | assistant（无 system/tool 行）
    Status         string         `db:"status"`             // Status*（assistant 回合开始前为 pending）
    StopReason     string         `db:"stop_reason"`
    ErrorCode      string         `db:"error_code"`
    ErrorMessage   string         `db:"error_message"`
    InputTokens    int            `db:"input_tokens"`       // 支撑 /usage 与对话 tokensUsed 富化
    OutputTokens   int            `db:"output_tokens"`
    Provider       string         `db:"provider"`           // 溯源：产此回合的 provider（多模型混对成本核算）
    ModelID        string         `db:"model_id"`           // 溯源：产此回合的模型
    Attrs          map[string]any `db:"attrs,json"`         // attachments / mentions 快照（freeze-on-send）
    CreatedAt      time.Time      `db:"created_at,created"`
    UpdatedAt      time.Time      `db:"updated_at,updated"`
    Blocks         []Block        `db:"-"`                  // 内容树，store 读时 hydrate / 写时 caller 给——非列
}
```

`Block` 落 `message_blocks` 表（`blk_` 前缀），但**store / 落盘 / History 查询留 chat M5.2**——本轮（loop M2.2）只立类型契约 + 词表，loop 内存产 `Block`、经 `host.WriteFinalize` 外包落盘，自身不碰表。`Danger` 在 domain 存为**纯字符串**：`tool.DangerLevel` 是 app 层概念，domain 不能反向依赖 app，故 loop 在 `collectToolCalls` 做 `DangerLevel`→`string` 转换。

---

## 2. 词表 (Vocabularies)

| 词表 | 取值 | 说明 |
|---|---|---|
| `BlockType*` | `text` `reasoning` `tool_call` `tool_result` `compaction` | loop 发的内容树节点种类。旧 eventlog 的 `progress`/`message` **已砍**——更深层级（subagent 子树）经 stream `Open.ParentID` 表达，不靠新增块型。 |
| `Status*` | `pending` `streaming` `completed` `error` `cancelled` | message 与 block 共用一套。message 回合开始前为 `pending`；block 在 open↔close 间隐含 `streaming`；三终态与 `stream.Close` 状态 1:1。 |
| `StopReason*` | `end_turn` `max_tokens` `max_steps` `cancelled` `error` | 回合结束原因。`max_steps` 是**非成功**终态——loop 撞步数上限，诚实暴露使 UI 提供「继续」（不冒充 completed end_turn）。 |
| `ContextRole*` | `hot` `warm` `cold` `archived` | 压缩器（contextmgr M5.3）投影 block 如何进 LLM 历史而**不改写**落库 Content：hot 全文 / warm 截断预览 / cold 省略带标记 / archived 丢弃（并入 conversation.summary）。 |

---

## 3. messages 流的 Node content 形状（producer 那一份词表）

messages 流有**两个 producer**：**chat（R0055）发 message 级** node（一整个回合，message_start/stop）、**loop 发 block 级** node（回合内的 text/reasoning/tool_call/tool_result，嵌在 message 下）。各自定义所发 node 的 `Node.Content` 形状（「词表下放 producer」）。`open` 帧带最小元数据；**`close` 帧带完整快照**——`delta` 是 ephemeral（不入 replay buffer），buffer 内重连只见 open/close，故 close 的 `Result` 必须能重建内容。

| node.type | producer | open content | delta | close result |
|---|---|---|---|---|
| `message` | **chat** | `{role}` | —（无 delta，block 子节点流式） | `{role, status, stopReason?, inputTokens, outputTokens, errorCode?, errorMessage?}`（assistant 终态元数据）/ `{role, content, attachmentIds?}`（user 回显快照） |
| `text` | loop | —（空） | token 文本 | `{content}` |
| `reasoning` | loop | —（空） | token 文本 | `{content, signature?}` |
| `tool_call` | loop | `{name}` | args JSON 增量 | `{name, arguments, summary?, danger?}` |
| `tool_result` | loop | `{content}`（一次性产出，无 delta） | — | —（close 只带 status/error） |

**message 节点是回合的顶层父**：id=`msg_<id>`、`scope=conversation:<id>`、`Open.ParentID` 空（挂 scope 根）；loop 的 block 节点 `Open.ParentID = msgID`（挂其下）。**token/终态进 message 的 `Close.Result`**（回合元数据，不进任何 block 快照）。E3 递归：subagent 的 message 节点 `ParentID = 父 tool_call id`。

**danger 纯标记**（M2.2「纯信任」）：LLM 自报的 `danger`/`summary` 随 `tool_call` 节点上行（close result + 落库 `Attrs`），前端据此显示一句话摘要、标记 `cautious`/`dangerous` 调用。**本轮不阻塞执行**——`dangerous` 调用的确认暂停在 loop 层留接口位，待 ask 通道就绪（波次 6）接入。

---

## 4. 与 stream / loop 的关系

```
              produces                streams (transport)
  loop  ───────────────►  Block  ──────────────────────►  stream.Bridge (messages)
   │                        │                                   open / delta / close
   │  host.WriteFinalize    │  collectToolCalls
   ▼                        ▼
  message_blocks 表      ToolCallData (内存，loop 用 danger/group 决策)
```

- **stream = 怎么推**：`Envelope{seq,scope,id,frame}`，scope 锚 `conversation:<id>`，frame ∈ open/delta/close/signal。
- **messages = 由什么组成**：`Block` + 词表 + node content 形状。
- loop 产 `Block` → 一路经 `stream.Bridge` 实时推前端（Node.Content 装 block 内容）、一路经 `host.WriteFinalize` 落 `message_blocks` 表。两条路都保留：推流在 loop 内（best-effort，无 bridge/conv 自禁用），落盘外包给 host。

---

## 5. 契约边界（本轮 vs 后续）

| 范围 | 归属 |
|---|---|
| `Block` / `ToolCallData` / 词表 / node content 形状 | **messages domain（M2.2 R0031）** |
| `Message` 实体 + `Repository` + `messages`/`message_blocks` 表 store / DDL / workspace 列 / History 查询 | **R0054 ✅（落 domain/messages，见 §6）** |
| `ContextRole` 的写入（压缩） | contextmgr M5.3 |
| chat runner 编排（拼历史/Host/SSE message_start·stop/convQueue/System Prompt） | chat R0055-56 |
| 前端按 frame + node.type 重渲（events.md 全量重写） | 覆盖阶段（见 contract-changes #2 / #11） |

> **`Message` 归 `domain/messages`、不复活 `domain/chat`**：Message 是「拥有 Block 树的回合」、与 Block 同属中立内容模型，agent/subagent/chat 共享。旧 `domains/chat.md` 在 chat R0055-56 重写时清理为只剩 chat runner（ReAct 编排），Message/Block 契约全在本文。

---

## 6. 持久化（R0054）

`messages`（`msg_`，回合记录）+ `message_blocks`（`blk_`，Block 树）两表，**皆 append-only**（无 `deleted_at`，D1：内容日志永不删）、按 workspace 隔离（orm `,ws` 自动填/过滤）。DDL 详见 `database.md` §2.2。

**`Repository`（5 法）—— 两段式写对应 loop.Host 契约**：

| 方法 | 语义 |
|---|---|
| `CreateMessage(m, blocks)` | 一个事务内 insert 回合行 + （分配 seq 的）blocks。user 回合（role=user/completed/单 text block）；**开** assistant 回合（streaming/blocks=nil，host 在 `loop.Run` 前调拿 msgID 喂 reqctx + SSE `message_start`）。 |
| `FinalizeMessage(m, blocks)` | 一个事务内 update 已存在回合终态列（status/stopReason/error/tokens/provider/modelId）+ 追加 blocks。chat `WriteFinalize` 用；缺失行 → `ErrMessageNotFound`。 |
| `GetMessage(id)` | 单回合 + hydrate Blocks；缺失 `ErrMessageNotFound`。 |
| `ListMessages(convID, cursor, limit)` | 一页 keyset，**最新在前**（orm Page DESC）；REST 历史（N4 分页），前端反转一页按时序渲染。 |
| `LoadThread(convID)` | 整条线程、**最旧在前**（不分页）；chat `LoadHistory` 据此组装 LLM 历史。 |

- **seq 单调分配**：`message_blocks` 落盘时按对话 `MAX(seq)+1` 分配（`idx_blocks_conv_seq` UNIQUE 兜底）；正确性靠 chat convQueue per-conversation 串行写（每对话一个 AI 协程）、非 DB 序列。新对话从 seq=1 重起。
- **block id 落盘补全**：loop 内存 block 中 text/reasoning 无 id（落盘 mint `blk_`）、tool_call/tool_result 已带 id（LLM 给 / loop 设 parent）——store `insertBlocks` 原地补 id/seq/conv/message + 默认 status=completed/context_role=hot，caller 拿回填充后的 block。
- **错误**：`MESSAGE_NOT_FOUND`（404）——`GetMessage`/`FinalizeMessage` 命中未知 id。
