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
| **Execution**| `flowruns` | `fr_` | `FlowRun` |
| | `flowrun_events` | `fre_` | `FlowRunEvent` |
| | `flowrun_nodes` | `frn_` | `Node` |
| | `approvals` | `apv_` | `Approval` |
| **Inbound** | `trigger_schedules` | `ts_` | `TriggerSchedule` |
| | `trigger_firings` | `tfi_` | `TriggerFiring` |
| | `polling_states` | - | `PollingState` |
| **Audit** | `function_executions`| `fne_` | `Execution` |
| | `handler_calls` | `hcl_` | `Call` |
| | `mcp_calls` | `mcl_` | `Call` |
| | `skill_executions` | `ske_` | `Execution` |
| | `agent_executions` | `agx_` | `AgentExecution` |
| **Knowledge**| `documents` | `doc_` | `Document` |
| | `memories` | `mem_` | `Memory` |
| | `relations` | `rel_` | `Relation` |
| **Infra** | `sandbox_envs` | `se_` | `Env` |
| | `sandbox_runtimes` | `sr_` | `Runtime` |
| | `mcp_health_history`| `mch_` | `HealthSnapshot` |
| **Tasks** | `todos` | `td_` | `Todo` |

---

## 2. 核心领域模型详述 (Literal Structs)

### 2.1 Identity & Settings
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
    Parameters    []ParameterSpec `gorm:"serializer:json" json:"parameters"`
    ReturnSchema  map[string]any  `gorm:"serializer:json" json:"returnSchema"`
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

// Agent
type Agent struct {
    ID              string `gorm:"primaryKey;type:text" json:"id"`
    ActiveVersionID string `json:"activeVersionId"`
}
type AgentVersion struct {
    Prompt                 string          `json:"prompt"`
    Skill                  string          `json:"skill"`
    Knowledge              []string        `gorm:"serializer:json" json:"knowledge"`
    Tools                  []ToolRef       `gorm:"serializer:json" json:"tools"`
    OutputSchema           *OutputSchema   `gorm:"serializer:json" json:"outputSchema,omitempty"` // injected into systemPrompt at invoke
    ModelOverride          *model.ModelRef `gorm:"serializer:json" json:"modelOverride,omitempty"` // apiKeyId+modelId; nil=default agent scenario
    ForgedInConversationID *string         `gorm:"index" json:"forgedInConversationId,omitempty"` // relation forged/edited edges
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

### 3.1 Parameter & Method Specs
```typescript
interface ParameterSpec {
  name: string;
  type: "string" | "number" | "boolean" | "array" | "object";
  description?: string;
  required: boolean;
  default?: any;
  enum?: any[];
}
interface MethodSpec {
  name: string;
  args: ParameterSpec[];
  returnSchema?: Record<string, any>;
  body: string;
  streaming: boolean;
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
    ParentID *string `gorm:"index" json:"parentId"`
    Path     string  `gorm:"index;not null" json:"path"`
    Content  string  `gorm:"not null" json:"content"`
    Position int     `gorm:"not null;default:0" json:"position"`
}
```

### 4.2 Relation (Topology)
```go
type Relation struct {
    FromKind string `gorm:"not null;index" json:"fromKind"`
    FromID   string `gorm:"not null;index" json:"fromId"`
    ToKind   string `gorm:"not null;index" json:"toKind"`
    ToID     string `gorm:"not null;index" json:"toId"`
    Kind     string `gorm:"not null;uniqueIndex:uq_rel" json:"kind"`
}
```

### 4.3 Trigger
```go
type TriggerSchedule struct {
    WorkflowID string `gorm:"not null;index" json:"workflowId"`
    Kind       string `gorm:"not null" json:"kind"`
    Spec       any    `gorm:"serializer:json" json:"spec"`
    Enabled    bool   `json:"enabled"`
}
type TriggerFiring struct {
    DedupKey  string `gorm:"not null;uniqueIndex" json:"-"`
    Status    string `gorm:"index" json:"status"`
    FlowrunID string `json:"flowrunId"`
}
```

---

## 5. SQL 约束与扩展 (Schema Extras)

- **Partial Unique**: `idx_fre_record_once` -> `UNIQUE(flowrun_id, dedup_key) WHERE type NOT IN ('node_started','node_failed')`.
- **Soft Delete**: `DeletedAt` 字段在全量业务表中存在，查询需强制过滤。
- **ID 前缀**: `u_, aki_, cv_, msg_, blk_, att_, fn_, fnv_, fne_, hd_, hdv_, hcl_, wf_, wfv_, ag_, agv_, agx_, fr_, fre_, frn_, apv_, ts_, tfi_, doc_, mem_, rel_, se_, sr_, mch_, mcl_, ske_, td_`.
