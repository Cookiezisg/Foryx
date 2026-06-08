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
| | `flowrun_events` | `fre_` | `FlowRunEvent` |
| | `flowrun_nodes` | `frn_` | `Node` |
| | `approvals` | `apv_` | `Approval` |
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
type Conversation struct {
    ID                   string         `gorm:"primaryKey;type:text" json:"id"`
    UserID               string         `gorm:"not null;index" json:"-"`
    Title                string         `gorm:"not null;default:''" json:"title"`
    SystemPrompt         string         `gorm:"type:text" json:"systemPrompt"`
    Summary              string         `gorm:"type:text" json:"summary"`
    SummaryCoversUpToSeq int64          `gorm:"not null;default:0" json:"summaryCoversUpToSeq"`
    Archived             bool           `gorm:"not null;default:false;index" json:"archived"`
    Pinned               bool           `gorm:"not null;default:false" json:"pinned"`
    AttachedDocuments    []AttachedDoc  `gorm:"serializer:json" json:"attachedDocuments"`
}
type Message struct {
    ID             string         `gorm:"primaryKey;type:text" json:"id"`
    ConversationID string         `gorm:"not null;index" json:"conversationId"`
    Role           string         `gorm:"not null" json:"role"`
    Status         string         `gorm:"not null;default:completed" json:"status"`
    InputTokens    int            `json:"inputTokens"`
    OutputTokens   int            `json:"outputTokens"`
    Provider       string         `gorm:"index" json:"provider"`
    ModelID        string         `gorm:"index" json:"modelId"`
    Attrs          map[string]any `gorm:"serializer:json" json:"attrs"`
}
type Block struct {
    ID             string         `gorm:"primaryKey;type:text" json:"id"`
    ConversationID string         `gorm:"not null;uniqueIndex:idx_conv_seq" json:"conversationId"`
    Seq            int64          `gorm:"not null;uniqueIndex:idx_conv_seq" json:"seq"`
    Type           string         `gorm:"not null" json:"type"`
    Content        string         `gorm:"not null" json:"content"`
    Attrs          map[string]any `gorm:"serializer:json" json:"attrs"`
    Status         string         `gorm:"not null" json:"status"`
    ContextRole    string         `gorm:"not null;default:hot" json:"contextRole"`
}
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

### 2.4 Durable Execution (Run Plane)
```go
type FlowRun struct {
    ID              string            `gorm:"primaryKey;type:text" json:"id"`
    WorkflowID      string            `gorm:"not null;index" json:"workflowId"`
    Generation      int               `gorm:"not null;default:0" json:"generation"`
    PinnedCallables map[string]string `gorm:"serializer:json" json:"pinnedCallables"`
    Status          string            `gorm:"not null;index" json:"status"`
    StartedAt       time.Time         `gorm:"not null" json:"startedAt"`
}
type FlowRunEvent struct {
    ID           string `gorm:"primaryKey;type:text" json:"id"`
    FlowrunID    string `gorm:"not null;uniqueIndex:idx_fre_seq" json:"flowrunId"`
    Seq          int64  `gorm:"not null;uniqueIndex:idx_fre_seq" json:"seq"`
    Type         string `gorm:"not null" json:"type"`
    NodeID       string `gorm:"index" json:"nodeId"`
    IterationKey int    `gorm:"index" json:"iterationKey"`
    Generation   int    `gorm:"index" json:"generation"`
    DedupKey     string `gorm:"not null" json:"-"`
    Result       any    `gorm:"serializer:json" json:"result"`
}
type Approval struct {
    FlowrunID string `gorm:"not null;uniqueIndex:idx_apv" json:"flowrunId"`
    NodeID    string `gorm:"not null;uniqueIndex:idx_apv" json:"nodeId"`
    Status    string `gorm:"not null" json:"status"`
    Decision  string `json:"decision"`
    Deadline  *time.Time `gorm:"index" json:"deadline"`
}
```

---

## 3. 逻辑协议 DTOs (Nested Schemas)

### 3.1 Field（统一 I/O）& Method Specs
`schema.Field`（`internal/pkg/schema`）是**所有锻造实体共享的唯一 I/O 字段类型**——fn/hd/ag 的 inputs/outputs、ctl/apf 的 inputSchema、trg 的 outputs 全用它。刻意极简：无 required / default / enum / 嵌套，精确塑形交运行时 CEL。各处以 `,json` 存为 JSON 数组列（`TEXT NOT NULL DEFAULT '[]'`）。

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

### 3.2 Graph Engine
```typescript
interface Graph {
  nodes: NodeSpec[];
  edges: EdgeSpec[];
  variables: VariableSpec[];
}
interface NodeSpec {
  id: string;
  type: "trigger" | "agent" | "tool" | "case" | "approval";
  config: any;
  retry?: { maxAttempts: number, backoff: string, delay: number };
}
interface EdgeSpec {
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
| `control_logic_versions` | id(ctlv_), control_id, version, **input_schema**(`TEXT NOT NULL DEFAULT '[]'`，`[]schema.Field` 声明 workflow 节点喂入的字段；`when`/`emit` 读 `input.*`), branches(JSON `[{port,when,emit}]`), change_reason, forged_in_conversation_id | append-only + cap 50 裁剪（无 deleted_at）；`idx_ctlv_control_version` = UNIQUE(control_id, version) |

### 4.6 Approval（审批渲染实体，apf_/apfv_）
> workflow `approval` 节点引用的审批表（markdown prompt 模板 + 决策规则）。AI 工作实体，有版本但**无 sandbox/env/executions**——渲染 + park 是波次 4 运行时事。**前缀 `apf_`/`apfv_` ≠ `apv_`**（`apv_` 是 `approvals` 运行时表）。详 domains/approval.md。2 表，pkg/orm。

| 表 | 关键列 | 说明 |
|---|---|---|
| `approval_forms` | id(apf_), workspace_id, name, description, active_version_id, deleted_at | 实体本体，软删；`idx_approval_forms_ws_name` = UNIQUE(workspace_id, name) WHERE deleted_at IS NULL |
| `approval_form_versions` | id(apfv_), approval_id, version, **input_schema**(`TEXT NOT NULL DEFAULT '[]'`，`[]schema.Field` 声明 workflow 节点喂入的字段；`template` 读 `{{ input.* }}`), template(markdown `{{ CEL }}`), allow_reason(bool), timeout, timeout_behavior(reject/approve/fail), change_reason, forged_in_conversation_id | append-only + cap 50 裁剪（无 deleted_at）；`idx_apfv_approval_version` = UNIQUE(approval_id, version) |

---

## 5. SQL 约束与扩展 (Schema Extras)

- **Partial Unique**: `idx_fre_record_once` -> `UNIQUE(flowrun_id, dedup_key) WHERE type NOT IN ('node_started','node_failed')`.
- **Partial Unique**: `idx_mcp_ws_name` -> `UNIQUE(workspace_id, name) WHERE deleted_at IS NULL`（mcp server 短名工作区内唯一，故可作 HTTP path key）。
- **Encrypted Column**: `mcp_servers.config_enc` -> AES-GCM 密文，载 `{env, headers}`；加密封在 store 层，domain.Server 持明文 `Env`/`Headers`。
- **Soft Delete**: `DeletedAt` 字段在全量业务表中存在，查询需强制过滤。
- **ID 前缀**: `u_, aki_, cv_, msg_, blk_, att_, fn_, fnv_, fne_, fnenv_, hd_, hdv_, hcl_, hdenv_, hdi_, wf_, wfv_, ag_, agv_, agx_, fr_, fre_, frn_, apv_, trg_, trf_, tra_, mcp_, doc_, rel_, se_, sr_, noti_, bsh_, ctl_, ctlv_, apf_, apfv_`. （`fnenv_`/`hdenv_` = function/handler 为各版本 venv 自 mint 的 sandbox owner id；`hdi_` = handler 常驻实例 id（内存态，不入库）；`trg_`/`trf_`/`tra_` = trigger 实体 / firing 收件箱 / activation 动作日志（trigger 升为独立实体，取代旧 `ts_`/`tfi_`）；`mcp_` = mcp server 容器实体（一表 `mcp_servers`，工具不落库）；`se_` = sandbox 内部物理 env 行 id——consumer 不复用 entity id，见 shared-infra-IDs；`bsh_` = 后台 shell 进程 id（`tool/shell` 的 `ProcessManager`，内存态、不入库，性质同 `hdi_`））
> 注：memory 改文件式（`~/.forgify/workspaces/<wsID>/memories/*.md`），**无 memories 表、无 `mem_` 前缀**（文件名即标识）。
> 注：todo 改 TodoWrite 式（一行一作用域、整列替换），PK `scope_id` = 对话/subagent id 多态键，**无 `td_` 前缀**（项无 id、清单按作用域寻址）。
> 注：skill 改文件式（`~/.forgify/workspaces/<wsID>/skills/<name>/SKILL.md`），**无 skill 表、无 `skill_executions` 表（execution 审计砍）、无 `ske_`/`sk_` 前缀**（name 即标识、relation 节点用 name；R0021 预留的 `sk_` 对文件式 skill 不启用）。
> 注：mcp server 为容器实体（`mcp_` 前缀、一表 `mcp_servers`，`relation.KindForID` 已识别）。**无 `mcp_calls`/`mcp_health_history` 表、无 `mcl_`/`mch_` 前缀**（调用审计 + 健康历史砍，server 工具不落库——动态落成 `mcp__<server>__<tool>` 工具）。
> 注：`ctl_`/`ctlv_` = control 逻辑实体 / 其版本（workflow `control` 节点引用的路由逻辑 when/emit 分支组；AI 工作实体，有版本但无 sandbox/env/executions，详 domains/control.md）。
> 注：`apf_`/`apfv_` = approval **form**（审批渲染实体）/ 其版本（workflow `approval` 节点引用的 markdown 模板 + 决策规则；详 domains/approval.md）。**`apf_` ≠ `apv_`**——`apv_` 是 `approvals` 运行时表（波次 4 flowrun 的 parked 记录）。
- **作废前缀**: `sk_` 原为 skill 预留，**R0040 skill 重写为文件式后作废**——skill 无生成 id、relation 节点用 name；`ske_` 随 skill execution 审计砍而删。
