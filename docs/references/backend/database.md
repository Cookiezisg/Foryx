---
id: DOC-012
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Database Design — 全量存储 Schema 与物理模型契约 (100% Coverage)

> **法律级声明**：本文档是 Forgify 存储层的**权威规格说明书**。通过对 `internal/domain` 进行深度扫描生成，涵盖 130+ 个结构体及其物理/逻辑标签。

---

## 1. 物理表全索引 (The Tables)

| Domain | Table | Prefix | GORM Model |
|---|---|---|---|
| **Identity** | `workspaces` | `ws_` | `Workspace` |
| | `api_keys` | `aki_` | `APIKey` |
| **Messaging**| `conversations` | `cv_` | `Conversation` |
| | `messages` | `msg_` | `Message` |
| | `message_blocks` | `blk_` | `Block` |
| | `attachments` | `att_` | `Attachment` |
| **Forge** | `functions` | `fn_` | `Function` |
| | `function_versions` | `fnv_` | `Version` |
| | `handlers` | `hd_` | `Handler` |
| | `handler_versions` | `hdv_` | `Version` |
| | `workflows` | `wf_` | `Workflow` |
| | `workflow_versions` | `wfv_` | `Version` |
| | `agents` | `ag_` | `Agent` |
| | `agent_versions` | `agv_` | `AgentVersion` |
| | `control_logics` | `ctl_` | `ControlLogic` |
| | `control_logic_versions` | `ctlv_` | `Version` |
| | `approval_forms` | `apf_` | `ApprovalForm` |
| | `approval_form_versions` | `apfv_` | `Version` |
| | `mcp_servers` | `mcp_` | `Server` |
| **Execution**| `flowruns` | `fr_` | `FlowRun` |
| | `flowrun_nodes` | `frn_` | `FlowRunNode` |
| **Inbound** | `trigger_schedules` | `ts_` | `TriggerSchedule` |
| | `trigger_firings` | `tfi_` | `TriggerFiring` |
| | `polling_states` | - | `PollingState` |
| **Audit** | `function_executions`| `fne_` | `Execution` |
| | `handler_calls` | `hcl_` | `Call` |
| | `agent_executions` | `agx_` | `AgentExecution` |
| **Knowledge**| `documents` | `doc_` | `Document` |
| | `relations` | `rel_` | `Relation` |
| **Infra** | `sandbox_envs` | `se_` | `Env` |
| | `sandbox_runtimes` | `sr_` | `Runtime` |
| **Tasks** | `todos` | `-` | `List` |
| **Notification**| `notifications` | `noti_` | `Notification` |

---

## 2. 核心领域模型详述 (Literal Structs)

### 2.1 Identity (Workspace)
```go
// workspaces — the isolation root: the one business table with NO workspace_id.
// backend-new style: plain struct + lightweight db tags (GORM removed).
// default_dialogue/utility/agent — per-scenario default model selection (ModelRef
// JSON, nullable); selection lives here as workspace preferences, not in a table.
type Workspace struct {
    ID              string                `db:"id,pk" json:"id"`
    Name            string                `db:"name" json:"name"`                          // free-form display label; UNIQUE(name) WHERE deleted_at IS NULL
    AvatarColor     string                `db:"avatar_color" json:"avatarColor,omitempty"`
    Language        string                `db:"language" json:"language"`                  // CHECK IN ('zh-CN','en'), default 'zh-CN'
    DefaultDialogue *modeldomain.ModelRef `db:"default_dialogue,json" json:"defaultDialogue,omitempty"` // TEXT, JSON ModelRef, nullable
    DefaultUtility  *modeldomain.ModelRef `db:"default_utility,json" json:"defaultUtility,omitempty"`   // TEXT, JSON ModelRef, nullable
    DefaultAgent    *modeldomain.ModelRef `db:"default_agent,json" json:"defaultAgent,omitempty"`       // TEXT, JSON ModelRef, nullable
    DefaultSearchKeyID string             `db:"default_search_key_id" json:"defaultSearchKeyId,omitempty"` // TEXT NOT NULL DEFAULT '', "" = unconfigured; WebSearch's single explicit search key (provider implied by the key)
    LastUsedAt      *time.Time            `db:"last_used_at" json:"lastUsedAt,omitempty"`
    CreatedAt       time.Time             `db:"created_at,created" json:"createdAt"`
    UpdatedAt       time.Time             `db:"updated_at,updated" json:"updatedAt"`
    DeletedAt       *time.Time            `db:"deleted_at,deleted" json:"-"`
}
// api_keys — workspace-scoped credentials. The probe archives the upstream's raw
// response verbatim (test_response, parsed downstream by model/search); no
// models_found / is_default — selection lives in model / search-config modules.
type APIKey struct {
    ID           string     `db:"id,pk" json:"id"`
    WorkspaceID  string     `db:"workspace_id,ws" json:"-"`
    Provider     string     `db:"provider" json:"provider"`
    DisplayName  string     `db:"display_name" json:"displayName"`              // UNIQUE per workspace (partial, active)
    KeyEncrypted string     `db:"key_encrypted" json:"-"`
    KeyMasked    string     `db:"key_masked" json:"keyMasked"`
    BaseURL      string     `db:"base_url" json:"baseUrl,omitempty"`
    APIFormat    string     `db:"api_format" json:"apiFormat,omitempty"`
    TestStatus   string     `db:"test_status" json:"testStatus"`                // pending|ok|error
    TestError    string     `db:"test_error" json:"testError,omitempty"`
    TestResponse string     `db:"test_response" json:"-"`                       // raw probe body; parsed by model/search
    LastTestedAt *time.Time `db:"last_tested_at" json:"lastTestedAt,omitempty"`
    CreatedAt    time.Time  `db:"created_at,created" json:"createdAt"`
    UpdatedAt    time.Time  `db:"updated_at,updated" json:"updatedAt"`
    DeletedAt    *time.Time `db:"deleted_at,deleted" json:"-"`
}
```

### 2.2 Chat & Messaging
```go
// conversations (cv_) — 线程容器 + 配置（M5.1 ✅ as-built，pkg/orm；详见 domains/conversation.md DOC-106）。
// 业务表软删（deleted_at，D1）。summary/summaryCoversUpToSeq 由 contextmgr(M5.3) 写、auto_titled 由 chat(M5.2) 写。
type Conversation struct {
    ID                   string                            `db:"id,pk"`                    // cv_<16hex>
    WorkspaceID          string                            `db:"workspace_id,ws"`
    Title                string                            `db:"title"`
    AutoTitled           bool                              `db:"auto_titled"`
    SystemPrompt         string                            `db:"system_prompt"`
    Summary              string                            `db:"summary"`
    SummaryCoversUpToSeq int64                             `db:"summary_covers_up_to_seq"`
    AttachedDocuments    []documentdomain.AttachedDocument `db:"attached_documents,json"`
    Archived             bool                              `db:"archived"`
    Pinned               bool                              `db:"pinned"`
    ModelOverride        *modeldomain.ModelRef             `db:"model_override,json"`      // nil=用默认；结构校验 only
    CreatedAt            time.Time                         `db:"created_at,created"`
    UpdatedAt            time.Time                         `db:"updated_at,updated"`
    DeletedAt            *time.Time                        `db:"deleted_at,deleted"`
}
// idx_conversations_ws_list = (workspace_id, pinned DESC, created_at DESC, id DESC) WHERE deleted_at IS NULL
//
// messages (msg_) + message_blocks (blk_) — 一个对话的内容日志（R0054 ✅，归 domain/messages，
// 详见 domains/messages.md §6）。两表 append-only（无 deleted_at，D1：内容日志永不删）、workspace
// 隔离（,ws）。回合两段式写（CreateMessage 开 / FinalizeMessage 收 assistant 回合）对应 loop.Host。
type Message struct {
    ID             string         `db:"id,pk"`              // msg_<16hex>
    ConversationID string         `db:"conversation_id"`
    WorkspaceID    string         `db:"workspace_id,ws"`
    SubagentID     string         `db:"subagent_id"`       // ''=顶层；非空=subagent run（R0058）；chat LoadHistory 排除非空（不污染父历史）
    Role           string         `db:"role"`              // CHECK(user|assistant)；无 system/tool 行
    Status         string         `db:"status"`            // CHECK(5 态) DEFAULT 'completed'
    StopReason     string         `db:"stop_reason"`
    ErrorCode      string         `db:"error_code"`
    ErrorMessage   string         `db:"error_message"`
    InputTokens    int            `db:"input_tokens"`      // /usage + 对话 tokensUsed 富化
    OutputTokens   int            `db:"output_tokens"`
    Provider       string         `db:"provider"`          // 溯源：产此回合的 provider
    ModelID        string         `db:"model_id"`          // 溯源：产此回合的模型
    Attrs          map[string]any `db:"attrs,json"`        // attachments / mentions 快照（freeze-on-send）
    CreatedAt      time.Time      `db:"created_at,created"`
    UpdatedAt      time.Time      `db:"updated_at,updated"`
    // Blocks []Block — 内容树，store hydrate（非列）
}
// idx_messages_conv = (workspace_id, conversation_id, created_at, id)
type Block struct {
    ID             string         `db:"id,pk"`             // blk_<16hex>
    ConversationID string         `db:"conversation_id"`
    WorkspaceID    string         `db:"workspace_id,ws"`
    MessageID      string         `db:"message_id"`        // 所属回合；block 的 stream parentId
    ParentBlockID  string         `db:"parent_block_id"`   // tool_result → 其 tool_call
    Seq            int64          `db:"seq"`               // 对话内单调；落盘时分配（MAX+1）
    Type           string         `db:"type"`              // CHECK(text|reasoning|tool_call|tool_result|compaction)
    Attrs          map[string]any `db:"attrs,json"`        // tool_call:{tool,summary,danger}; reasoning:{signature}
    Content        string         `db:"content"`
    Status         string         `db:"status"`            // CHECK(5 态)
    Error          string         `db:"error"`
    ContextRole    string         `db:"context_role"`      // CHECK(hot|warm|cold|archived) DEFAULT 'hot'；压缩器（M5.3）投影
    CreatedAt      time.Time      `db:"created_at,created"`
    UpdatedAt      time.Time      `db:"updated_at,updated"`
}
// idx_blocks_conv_seq = UNIQUE(conversation_id, seq)   ← seq 单调保证（D3 风格幂等键）
// idx_blocks_message  = (message_id, seq)              ← hydrate 按回合取 block
```

### 2.2b Attachment (att_) — 多模态附件（R0051 ✅，详见 domains/attachment.md DOC-307）
```go
// attachments (att_) — 上传文件元数据；字节在 CAS blob 存储（~/.forgify/workspaces/<ws>/blobs/<sha[:2]>/<sha>），
// 绝不进 SQLite。业务表软删 D1；sha256 不唯一（多行共享一 blob = 内容寻址 dedup）。
type Attachment struct {
    ID          string     `db:"id,pk"`              // att_<16hex>
    WorkspaceID string     `db:"workspace_id,ws"`
    SHA256      string     `db:"sha256"`             // CAS 键
    Filename    string     `db:"filename"`
    MimeType    string     `db:"mime_type"`
    SizeBytes   int64      `db:"size_bytes"`
    Kind        string     `db:"kind"`               // image|document|text|audio|video|other
    CreatedAt   time.Time  `db:"created_at,created"`
    DeletedAt   *time.Time `db:"deleted_at,deleted"`
}
// idx_attachments_ws_sha = (workspace_id, sha256) WHERE deleted_at IS NULL（GC 保留集 / dedup-ref）
```

### 2.3 The Quadrinity Entities (Forge)
```go
// Function
type Function struct {
    ID              string   `gorm:"primaryKey;type:text" json:"id"`
    UserID          string   `gorm:"not null;index" json:"userId"`
    Name            string   `gorm:"not null" json:"name"`
    ActiveVersionID string   `json:"activeVersionId"`
    Tags            []string `gorm:"serializer:json" json:"tags"`
}
type Version struct {
    ID            string          `gorm:"primaryKey;type:text" json:"id"`
    Code          string          `json:"code"`
    Inputs        []schema.Field  `db:"inputs,json" json:"inputs"`   // TEXT NOT NULL DEFAULT '[]'
    Outputs       []schema.Field  `db:"outputs,json" json:"outputs"` // TEXT NOT NULL DEFAULT '[]'
    Dependencies  []string        `gorm:"serializer:json" json:"dependencies"`
    EnvStatus     string          `json:"envStatus"`
    ChangeReason  string          `json:"changeReason"`
}

// Handler
type Handler struct {
    ID              string `gorm:"primaryKey;type:text" json:"id"`
    ActiveVersionID string `json:"activeVersionId"`
    ConfigEncrypted string `gorm:"type:text" json:"-"`
}

// Workflow
type Workflow struct {
    Enabled     bool   `gorm:"not null" json:"enabled"`
    Concurrency string `gorm:"not null" json:"concurrency"`
}
type Version struct {
    Graph string `gorm:"type:text;not null" json:"-"` // Resolved as Graph DTO
}

// Agent — the 4th Quadrinity element: a configured LLM worker (no code; mounts capabilities
// by reference, runs a ReAct loop). Linear append-only versions + free-moving active pointer;
// NO pending/accept. backend-new style: plain struct + lightweight db tags (GORM removed),
// workspace_id isolation via ,ws. partial-UNIQUE(workspace_id, name).
type Agent struct {
    ID              string     `db:"id,pk"`
    WorkspaceID     string     `db:"workspace_id,ws"`
    Name            string     `db:"name"`              // workspace-scoped partial-UNIQUE (freed on soft-delete)
    Description     string     `db:"description"`
    Tags            []string   `db:"tags,json"`
    ActiveVersionID string     `db:"active_version_id"` // pointer; edit moves forward / revert moves freely
    CreatedAt       time.Time  `db:"created_at,created"`
    UpdatedAt       time.Time  `db:"updated_at,updated"`
    DeletedAt       *time.Time `db:"deleted_at,deleted"`
    // ActiveVersion is computed (non-column), attached by Service.Get.
}

// AgentVersion (agv_) — immutable config snapshot; no status, no updated_at. UNIQUE(agent_id, version).
type AgentVersion struct {
    ID                     string          `db:"id,pk"`
    WorkspaceID            string          `db:"workspace_id,ws"`
    AgentID                string          `db:"agent_id"`
    Version                int             `db:"version"`                  // monotonic max+1 (no status)
    Prompt                 string          `db:"prompt"`
    Skill                  string          `db:"skill"`                    // single skill name to pre-activate
    Knowledge              []string        `db:"knowledge,json"`           // document IDs attached as context
    Tools                  []ToolRef       `db:"tools,json"`               // fn_/hd_/mcp refs (no ag_)
    Inputs                 []schema.Field  `db:"inputs,json"`              // declared task inputs (workflow feeds these); TEXT NOT NULL DEFAULT '[]'
    Outputs                []schema.Field  `db:"outputs,json"`             // declared result fields; empty = free-form; instructed to answer as JSON object; TEXT NOT NULL DEFAULT '[]'
    ModelOverride          *model.ModelRef `db:"model_override,json"`      // apiKeyId+modelId; nil=default agent scenario
    ChangeReason           string          `db:"change_reason"`
    ForgedInConversationID string          `db:"forged_in_conversation_id"`// relation create(v1)/edit(v>1) edges
    CreatedAt              time.Time        `db:"created_at,created"`
}

// AgentExecution (agx_) — append-only log of one InvokeAgent run; NO deleted_at (D1).
// status CHECK(ok|failed|cancelled|timeout); triggered_by CHECK(chat|workflow|manual) — no
// "agent" trigger (an agent cannot invoke another agent).
type AgentExecution struct {
    ID             string         `db:"id,pk"`
    WorkspaceID    string         `db:"workspace_id,ws"`
    AgentID        string         `db:"agent_id"`
    VersionID      string         `db:"version_id"`
    ModelID        string         `db:"model_id"`        // which model actually ran
    Status         string         `db:"status"`          // ok|failed|cancelled|timeout
    TriggeredBy    string         `db:"triggered_by"`    // chat|workflow|manual
    Input          map[string]any `db:"input,json"`
    Output         any            `db:"output,json"`
    ErrorMessage   string         `db:"error_message"`
    ElapsedMs      int64          `db:"elapsed_ms"`
    StartedAt      time.Time      `db:"started_at"`
    EndedAt        time.Time      `db:"ended_at"`
    ConversationID string         `db:"conversation_id"`
    MessageID      string         `db:"message_id"`
    ToolCallID     string         `db:"tool_call_id"`
    FlowrunID      string         `db:"flowrun_id"`
    FlowrunNodeID  string         `db:"flowrun_node_id"`
    CreatedAt      time.Time      `db:"created_at,created"`
}
```

### 2.4 Durable Execution (Run Plane) — 节点结果记忆化（M4.2/M4.3 落地）

> **取代旧事件溯源模型**：真相是 `flowrun_nodes` 行表（**记忆化**）——**无 `flowrun_events` 日志、无 generation 代数、无 `approvals` 投影表、无 GORM、无 user_id**（workspace_id）。崩溃恢复 = 重走图、completed 行抄不重跑。详 domains/flowrun.md + domains/scheduler.md + workflow-revamp/21。

```go
type FlowRun struct { // flowruns — 执行头：钉死的拓扑 + pin 闭包 + 状态机（Log 表，无 deleted_at，D1）
    ID          string            `db:"id,pk"`              // fr_
    WorkspaceID string            `db:"workspace_id,ws"`
    WorkflowID  string            `db:"workflow_id"`
    VersionID   string            `db:"version_id"`         // 钉死的 wfv_（图拓扑，确定性锁之一）
    PinnedRefs  map[string]string `db:"pinned_refs,json"`   // BuildPinClosure {entity_id: active_version_id}（确定性锁之二）
    TriggerID   string            `db:"trigger_id"`         // 起点 trg_（手动 :trigger 时空）
    FiringID    string            `db:"firing_id"`          // 来源 trf_（firing 单事务 claim 写）
    Status      string            `db:"status"`             // running|completed|failed（CHECK）
    ReplayCount int               `db:"replay_count"`       // :replay 自增；非 generation
    Error       string            `db:"error"`
    StartedAt   time.Time         `db:"started_at,created"`
    CompletedAt *time.Time        `db:"completed_at"`
    UpdatedAt   time.Time         `db:"updated_at,updated"`
}
type FlowRunNode struct { // flowrun_nodes — ★真相表（记忆化）；每个 (节点,轮次) 一行（Log 表，无 deleted_at，D1）
    ID          string         `db:"id,pk"`              // frn_
    WorkspaceID string         `db:"workspace_id,ws"`
    FlowRunID   string         `db:"flowrun_id"`
    NodeID      string         `db:"node_id"`            // 图内局部 id（= 下游引用名）
    Iteration   int            `db:"iteration"`          // 循环轮次（回边 +1）
    Kind        string         `db:"kind"`               // trigger|action|agent|control|approval
    Ref         string         `db:"ref"`                // pin 的实体 ref（审计）
    Status      string         `db:"status"`             // completed|failed|parked（CHECK；只写终态/parked，无 running 行）
    Result      map[string]any `db:"result,json"`        // per-kind：trigger=payload / action·agent=返回 / control=emit 字段扁平+保留键 __port / approval=parked{rendered,allowReason}→{decision,reason}
    Error       string         `db:"error"`
    CreatedAt   time.Time      `db:"created_at,created"` // 终态写 / park 时间
    CompletedAt *time.Time     `db:"completed_at"`       // parked 期间 nil
    UpdatedAt   time.Time      `db:"updated_at,updated"`
}
```

- **`idx_frn_once` = UNIQUE(flowrun_id, node_id, iteration)**（D3 record-once，取代旧 `idx_fre_record_once`）：写终态一律 `INSERT OR IGNORE` first-wins（重放抄、不重跑）；approval 决策是条件 `UPDATE … WHERE status='parked'`（同 first-wins）。
- 索引：`idx_fr_running`（`WHERE status='running'` 部分，boot 跨 ws 恢复扫）· `idx_fr_ws_created`/`idx_fr_ws_workflow`（列表）· `idx_frn_run`（flowrun_id，重走拉全 run）· `idx_frn_parked`（`WHERE status='parked'` 部分，**审批收件箱 = parked 行，无 apv_ 投影**）。
- **`:replay`** = 物理删该 run 的 `status='failed'` 行（`DeleteFailedNodes`，Log 表上唯一允许的物理删——failed 是非结果）+ `replay_count++` + status 回 running，再重走（completed 行复用）。**取代旧 generation 代数。**
- **删**（旧 backend 残留）：`flowrun_events`(fre_ 事件日志) / `approvals`(apv_ parked 投影) / `Generation` / `PinnedCallables`(改名 `pinned_refs`) / GORM tag。`frs_`（agent 子步）**不引入**（agent 粗粒度，详 21 §3.3）。

---

## 3. 逻辑协议 DTOs (Nested Schemas)

### 3.1 Field（统一 I/O）& Method Specs
`schema.Field`（`internal/pkg/schema`）是**所有锻造实体共享的唯一 I/O 字段类型**——fn/hd/ag 的 inputs/outputs、ctl/apf 的 inputs（输出派生：ctl 读分支 emit、apf 恒为 `{decision,reason}`）、trg 的 outputs 全用它。刻意极简：无 required / default / enum / 嵌套，精确塑形交运行时 CEL。各处以 `,json` 存为 JSON 数组列（`TEXT NOT NULL DEFAULT '[]'`）。

> **CEL env**：`pkg/cel` 除固定三根变量 env（`payload`/`ctx`：trigger sensor；`input`：ctl/apf 的 when/emit/template）外，现也暴露 **`ScopedEnv`/`NewScopedEnv`**——根为调用方给的一组名字（各 DynType）+ 恒有的 `ctx`，专供 workflow 接线：节点 `input` CEL 按一张图的 **node id** 编译（`reviewer.score`），引用集合外名字即编译失败（白送「只引用存在节点」校验）。

```typescript
interface Field {                                  // pkg/schema.Field — 双向、处处通用
  name: string;
  type: "string" | "number" | "boolean" | "object" | "array";  // 粗类型提示（CEL 动态类型，不硬约束）
  description?: string;
}
interface MethodSpec {                             // handler 类方法（存于 handler_versions.methods JSON blob）
  name: string;
  inputs: Field[];                                 // 旧 args（ParameterSpec[]）→ 统一 Field[]
  outputs?: Field[];                               // 旧 returnSchema（map）→ 统一 Field[]
  body: string;
  streaming: boolean;
}
interface InitArgSpec {                            // handler __init__ 配置（≠ method I/O，保留自有形状）
  name: string;
  type: string;
  description?: string;
  required: boolean;
  default?: any;
  sensitive: boolean;                              // true → 加密存盘、读时掩码
}
```

### 3.2 Graph Engine（workflow_versions.graph）
> workflow 版本的图 blob（详 domains/workflow.md §2.3/§3）。node `kind` ∈ trigger/action/agent/control/approval（**非**旧 tool/case）；node `input` 是 `field → 裸 CEL` 接线（按 **node id** 读上游结果，`reviewer.score`），无全图 `variables`；边只携控制（`fromPort`：control 源=Branch.Port / approval 源=yes\|no / 其它=空），不携数据。

```typescript
interface Graph {
  nodes: Node[];
  edges: Edge[];                                   // 无 variables：数据走 node.input 的 node-id CEL
}
interface Node {
  id: string;                                      // 图内局部 id；也是下游 input CEL 引用本节点结果的名字
  kind: "trigger" | "action" | "agent" | "control" | "approval";
  ref: string;                                     // trg_ / fn_·hd_<id>.method·mcp:server/tool / ag_ / ctl_ / apf_
  input?: { [field: string]: string };             // field → 读上游结果的裸 CEL（trigger 无）
  retry?: { maxAttempts: number, backoff?: string, delayMs?: number };
  pos?: { x: number, y: number };                  // 画布坐标（执行忽略）
  notes?: string;
}
interface Edge {
  id: string;
  from: string;
  fromPort?: string;
  to: string;
}
```

---

## 4. 其它业务模型 (Support)

### 4.1 Document (Knowledge)
```go
type Document struct {
    ID          string     `db:"id,pk"`              // doc_<16hex>
    WorkspaceID string     `db:"workspace_id,ws"`
    ParentID    *string    `db:"parent_id"`          // nil = root-level
    Name        string     `db:"name"`
    Description string     `db:"description"`
    Content     string     `db:"content"`           // markdown, ≤ 1 MB
    Tags        []string   `db:"tags,json"`
    Position    int        `db:"position"`
    Path        string     `db:"path"`              // "/Parent/Child" dotted path
    SizeBytes   int64      `db:"size_bytes"`
    CreatedAt   time.Time  `db:"created_at,created"`
    UpdatedAt   time.Time  `db:"updated_at,updated"`
    DeletedAt   *time.Time `db:"deleted_at,deleted"` // 软删（删除子树留墓碑）
}
```
- `idx_documents_ws_parent_name` UNIQUE(workspace_id, COALESCE(parent_id,''), name) WHERE deleted_at IS NULL — 同父名唯一（根级 NULL→'' 兜住，否则 SQLite 放过根级重名）、软删后名可复用。
- `idx_documents_ws_parent` / `idx_documents_ws_path` (workspace_id, …) WHERE deleted_at IS NULL — 子节点列举 + path 排序。

### 4.2 Relation (Topology)
跨实体有向边。**无 name 列**（显示名读时内存查、不入库）、**无 deleted_at**（边随实体硬删）。
4 个边动词 `create/edit/equip/link`；两端类型在 from_kind/to_kind，故 kind 只需动词。详见 `domains/relation.md`。

```go
type Relation struct {
    ID          string         `db:"id,pk"`              // rel_<16hex>
    WorkspaceID string         `db:"workspace_id,ws"`
    Kind        string         `db:"kind"`               // CHECK IN ('create','edit','equip','link')
    FromKind    string         `db:"from_kind"`
    FromID      string         `db:"from_id"`
    ToKind      string         `db:"to_kind"`
    ToID        string         `db:"to_id"`
    Attrs       map[string]any `db:"attrs,json"`
    CreatedAt   time.Time      `db:"created_at,created"`
    UpdatedAt   time.Time      `db:"updated_at,updated"`
}
```
- `idx_rel_dedup` UNIQUE(workspace_id, from_id, to_id, kind) — 幂等重同步。
- `idx_rel_from` / `idx_rel_to` (workspace_id, from_id|to_id) — 方向性邻域遍历。

### 4.3 Trigger（独立实体，trg_/trf_/tra_）
> trigger 从 workflow 图节点提升为**独立实体**（详见 domains/trigger.md）。3 表，去 GORM（pkg/orm）。

| 表 | 关键列 | 说明 |
|---|---|---|
| `triggers` | id(trg_), workspace_id, name, kind CHECK(cron/webhook/fsnotify/sensor), config(JSON), **outputs**(`TEXT NOT NULL DEFAULT '[]'`，`[]schema.Field` 声明扇给监听 workflow 的 payload 字段), deleted_at | 实体本体，软删；`idx_triggers_ws_name` = UNIQUE(workspace_id, name) WHERE deleted_at IS NULL |
| `trigger_firings` | id(trf_), trigger_id, workflow_id, activation_id, payload, dedup_key, status, flowrun_id | durable 收件箱（persist-before-act）；`idx_trf_dedup` = UNIQUE(workflow_id, trigger_id, dedup_key)（**D3 幂等**）；status pending→claimed→started→{skipped,superseded,shed}；单事务 claim（ADR-021）留波次 4 scheduler |
| `trigger_activations` | id(tra_), trigger_id, kind, fired, return_value, payload, error, detail, firing_count | 动作日志，只增（**D1 不删**）；fired=false 也记 return_value/detail——"为什么没触发"可查 |

### 4.4 Todo (Working Checklist)
Agent 工作记忆清单（TodoWrite 式）。整列替换、无逐项 id；作用域 = 一个执行上下文（对话，或嵌入其中的 subagent run）。`scope_id` 是多态 owner 键（= `subagent_id ?? conversation_id`，kind ∈ {conversation, subagent}，同 relation 的 `from_id`），**无 `td_` 生成前缀**。

```go
type List struct {
    ScopeID        string     `db:"scope_id,pk"`         // = subagent_id ?? conversation_id（全局唯一）
    WorkspaceID    string     `db:"workspace_id,ws"`
    ConversationID string     `db:"conversation_id"`     // subagent 行 = 父对话（清理/分组）
    SubagentID     *string    `db:"subagent_id"`         // nil = 主对话清单
    Items          []Item     `db:"items,json"`          // 整张清单作 JSON 列；整体替换
    CreatedAt      time.Time  `db:"created_at,created"`
    UpdatedAt      time.Time  `db:"updated_at,updated"`
    DeletedAt      *time.Time `db:"deleted_at,deleted"`  // 软删（对话级联，留后波次）
}
type Item struct {
    Content    string `json:"content"`    // 祈使标题 "Run tests"
    ActiveForm string `json:"activeForm"` // 进行时 "Running tests"（in_progress 时展示）
    Status     string `json:"status"`     // pending | in_progress | completed（无 deleted）
}
```
- 每 `(workspace, conversation, subagent?)` 作用域恰一行；`scope_id` 是 PK（两种 id 都全局唯一，故无需 surrogate / COALESCE 唯一技巧）。整列替换 = 单行 upsert。
- `idx_todos_ws_conversation` (workspace_id, conversation_id) WHERE deleted_at IS NULL — 未来「某对话所有清单」级联清理查询。

### 4.5 Control（路由逻辑实体，ctl_/ctlv_）
> workflow `control` 节点引用的路由逻辑（when/emit 分支组）。AI 工作实体，有版本（pin 必需）但**无 sandbox/env/executions**——纯控制流，由 durable 解释器（波次 4）求值，绝非 activity。详 domains/control.md。2 表，pkg/orm。

| 表 | 关键列 | 说明 |
|---|---|---|
| `control_logics` | id(ctl_), workspace_id, name, description, active_version_id, deleted_at | 实体本体，软删；`idx_control_logics_ws_name` = UNIQUE(workspace_id, name) WHERE deleted_at IS NULL |
| `control_logic_versions` | id(ctlv_), control_id, version, **inputs**(`TEXT NOT NULL DEFAULT '[]'`，`[]schema.Field` 声明 workflow 节点喂入的字段；`when`/`emit` 读 `input.*`), branches(JSON `[{port,when,emit}]`；输出由各臂 emit 的 keys 描述，无独立 outputs 列), change_reason, forged_in_conversation_id | append-only + cap 50 裁剪（无 deleted_at）；`idx_ctlv_control_version` = UNIQUE(control_id, version) |

### 4.6 Approval（审批渲染实体，apf_/apfv_）
> workflow `approval` 节点引用的审批表（markdown prompt 模板 + 决策规则）。AI 工作实体，有版本但**无 sandbox/env/executions**——渲染 + park 是 flowrun 运行时事。**前缀 `apf_`/`apfv_`**（运行时 parked 状态 = `flowrun_nodes` 行，**无独立 `apv_` 表**）。详 domains/approval.md。2 表，pkg/orm。

| 表 | 关键列 | 说明 |
|---|---|---|
| `approval_forms` | id(apf_), workspace_id, name, description, active_version_id, deleted_at | 实体本体，软删；`idx_approval_forms_ws_name` = UNIQUE(workspace_id, name) WHERE deleted_at IS NULL |
| `approval_form_versions` | id(apfv_), approval_id, version, **inputs**(`TEXT NOT NULL DEFAULT '[]'`，`[]schema.Field` 声明 workflow 节点喂入的字段；`template` 读 `{{ input.* }}`), template(markdown `{{ CEL }}`；输出恒为 `{decision,reason}` 常量，无独立 outputs 列), allow_reason(bool), timeout, timeout_behavior(reject/approve/fail), change_reason, forged_in_conversation_id | append-only + cap 50 裁剪（无 deleted_at）；`idx_apfv_approval_version` = UNIQUE(approval_id, version) |

### 4.7 Workflow（静态编排图实体，wf_/wfv_）
> Quadrinity 的编排者：一张静态「DAG + 回边」typed 图，按 id 引用其它实体并用每个节点 CEL 接线 I/O。本模块**只 STORE + VALIDATE + PIN 图，不执行**——执行（解释器/scheduler/flowrun）是后续波次，import 同一批纯 helper 走 pin 版本。线性版本 + 自由 active 指针，无 pending/accept。详 domains/workflow.md。2 表，pkg/orm。

| 表 | 关键列 | 说明 |
|---|---|---|
| `workflows` | id(wf_), workspace_id, name, description, tags(json), active(bool，镜像 lifecycle==active), **lifecycle_state**(active\|draining\|inactive), **concurrency**(serial\|Skip\|BufferOne\|BufferAll\|AllowAll), needs_attention(bool), attention_reason, last_action_by(user\|system), active_version_id, deleted_at | 实体本体，软删；`idx_workflows_ws_name` = UNIQUE(workspace_id, name) WHERE deleted_at IS NULL；**CHECK** lifecycle_state / concurrency 各限上列枚举；`idx_workflows_ws_active` = (workspace_id, active) WHERE deleted_at IS NULL AND active = 1 |
| `workflow_versions` | id(wfv_), workflow_id, version, **graph**(JSON `{nodes,edges}`，`TEXT NOT NULL DEFAULT '{}'`；node=`{id,kind,ref,input:{field→CEL},retry}`、edge=`{id,from,fromPort,to}`), change_reason, forged_in_conversation_id | append-only + cap 50 裁剪（**无 deleted_at**，超 cap 硬删、始终放过 active）；`idx_wfv_workflow_version` = UNIQUE(workflow_id, version) |

---

## 5. SQL 约束与扩展 (Schema Extras)

- **Partial Unique / Record-once**: `idx_frn_once` -> `UNIQUE(flowrun_id, node_id, iteration)`（D3：flowrun 节点结果记忆化的 first-wins 去重键；写终态 `INSERT OR IGNORE`、approval 决策条件 `UPDATE WHERE status='parked'`。取代旧 journal `idx_fre_record_once`）。
- **Partial Unique**: `idx_mcp_ws_name` -> `UNIQUE(workspace_id, name) WHERE deleted_at IS NULL`（mcp server 短名工作区内唯一，故可作 HTTP path key）。
- **Encrypted Column**: `mcp_servers.config_enc` -> AES-GCM 密文，载 `{env, headers}`；加密封在 store 层，domain.Server 持明文 `Env`/`Headers`。
- **Soft Delete**: `DeletedAt` 字段在全量业务表中存在，查询需强制过滤。
- **ID 前缀**: `u_, aki_, cv_, msg_, blk_, att_, fn_, fnv_, fne_, fnenv_, hd_, hdv_, hcl_, hdenv_, hdi_, wf_, wfv_, ag_, agv_, agx_, fr_, frn_, trg_, trf_, tra_, mcp_, doc_, rel_, se_, sr_, noti_, bsh_, ctl_, ctlv_, apf_, apfv_`. （`fnenv_`/`hdenv_` = function/handler 为各版本 venv 自 mint 的 sandbox owner id；`hdi_` = handler 常驻实例 id（内存态，不入库）；`trg_`/`trf_`/`tra_` = trigger 实体 / firing 收件箱 / activation 动作日志（trigger 升为独立实体，取代旧 `ts_`/`tfi_`）；`mcp_` = mcp server 容器实体（一表 `mcp_servers`，工具不落库）；`se_` = sandbox 内部物理 env 行 id——consumer 不复用 entity id，见 shared-infra-IDs；`bsh_` = 后台 shell 进程 id（`tool/shell` 的 `ProcessManager`，内存态、不入库，性质同 `hdi_`））
> 注：memory 改文件式（`~/.forgify/workspaces/<wsID>/memories/*.md`），**无 memories 表、无 `mem_` 前缀**（文件名即标识）。
> 注：todo 改 TodoWrite 式（一行一作用域、整列替换），PK `scope_id` = 对话/subagent id 多态键，**无 `td_` 前缀**（项无 id、清单按作用域寻址）。
> 注：skill 改文件式（`~/.forgify/workspaces/<wsID>/skills/<name>/SKILL.md`），**无 skill 表、无 `skill_executions` 表（execution 审计砍）、无 `ske_`/`sk_` 前缀**（name 即标识、relation 节点用 name；R0021 预留的 `sk_` 对文件式 skill 不启用）。
> 注：mcp server 为容器实体（`mcp_` 前缀、一表 `mcp_servers`，`relation.KindForID` 已识别）。**无 `mcp_calls`/`mcp_health_history` 表、无 `mcl_`/`mch_` 前缀**（调用审计 + 健康历史砍，server 工具不落库——动态落成 `mcp__<server>__<tool>` 工具）。
> 注：`ctl_`/`ctlv_` = control 逻辑实体 / 其版本（workflow `control` 节点引用的路由逻辑 when/emit 分支组；AI 工作实体，有版本但无 sandbox/env/executions，详 domains/control.md）。
> 注：`apf_`/`apfv_` = approval **form**（审批渲染实体）/ 其版本（workflow `approval` 节点引用的 markdown 模板 + 决策规则；详 domains/approval.md）。审批的运行时 parked 状态 = `flowrun_nodes` 行（status=parked），**无独立 `apv_` 投影表**（M4.2 落地：parked frn 行即审批收件箱）。
> 注：`fr_`/`frn_` = flowrun 执行头 / 节点结果记忆化真相表（M4.2/M4.3，详 domains/flowrun.md + domains/scheduler.md）。**旧 `fre_`（事件日志）/`apv_`（parked 投影）已删**（记忆化模型无事件日志）；`frs_`（agent 子步）**不引入**（agent 粗粒度，详 workflow-revamp/21 §3.3）。
> 注：`wf_`/`wfv_` = workflow 静态编排图实体 / 其版本（按 id 引用 trg_/fn_·hd_·mcp_/ag_/ctl_/apf_ 的 typed 图，graph 以 JSON 存于版本行；只 STORE+VALIDATE+PIN，不执行；详 domains/workflow.md）。**本轮仅 `wf_`/`wfv_` 落地**——执行面前缀 **`fr_`/`fre_`/`apv_`**（flowrun / flowrun-event / approval 运行时记录）**尚未建**（durable scheduler 波次产；上表 §1 Execution 段 + §2.4 Run Plane struct 是该波次的前瞻设计，未落物理表）。
- **作废前缀**: `sk_` 原为 skill 预留，**R0040 skill 重写为文件式后作废**——skill 无生成 id、relation 节点用 name；`ske_` 随 skill execution 审计砍而删。
