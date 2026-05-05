# Todo — V1.2 详设计

**Phase**：5（System Tool 第二代 + UX 集成）
**状态**：✅ 实现完成（2026-05-04；2026-05-05 改名 Task → Todo）
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/database-design.md`](../service-contract-documents/database-design.md) — `todos` 表行
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — todo ×3 + ask ×3
- [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) — `todo` entity-state 事件

---

## 1. 一句话

LLM 在长任务里给自己用的 to-do 列表。每个 conversation 一份独立列表，4 个 system tool 操作（Create / List / Get / Update），状态变更通过 entity-state SSE 实时推给前端。**附带** AskUserQuestion 工具的 in-memory 会合服务（不持久化、无 entity，但与 todo 同 batch 提交，故合并到本文档 §10）。

---

## 2. 端到端推演（设计原则 #5）

```
触发源：LLM 在 chat agent 循环里调 TodoCreate 工具
  → transport 层：无（system tool 不走 HTTP；chat agent 直接调 tool.Execute）
    → app 层：app/tool/todo.TodoCreate.Execute
        → 调谁：app/todo.Service.Create（按 ctx 中 conversation ID 作用域）
        → 用什么：从 reqctx 取 conversation ID + user ID
      → infra 层：infra/store/todo.Store.Create（GORM insert）
  → 响应路径：
    成功 → 返新 Todo JSON 给 LLM 当 tool_result；同时通过 events.Bridge 发 `todo` SSE 事件给前端
    失败 → ErrSubjectRequired/ErrInvalidStatus → app/tool/todo.classifyTodoErr 转友好字符串返 LLM
```

**端到端跨 domain 依赖**：
- `pkg/reqctx`：取 conversation ID（Service 用）+ user ID（GORM ctx 透传）
- `domain/events`：`Todo` 事件类型（entity-state，与 forge / conversation 同模式）
- `infra/events/memory`：Bridge 实现（chat 已用，复用即可）
- 无前端 HTTP 端点：LLM 通过 system tool 操作；前端只通过 chat.message SSE + todo SSE 渲染状态

**AskUserQuestion 推演**（同 batch 完成，详 §10）：
```
LLM → AskUserQuestion 工具 → app/ask.Service.Wait（阻塞 5 分钟）
  ↑                                                          ↓
  ↑                                                  POST answers endpoint
  ↑← Resolve ← app/ask.Service.Resolve ← handler ←──┘
```

---

## 3. 领域模型

### Todo entity（`internal/domain/todo/todo.go`）

```go
type Todo struct {
    ID             string         `gorm:"primaryKey;type:text" json:"id"`
    ConversationID string         `gorm:"not null;index:idx_td_conv_status,priority:1;type:text" json:"conversationId"`
    Subject        string         `gorm:"not null;type:text" json:"subject"`
    Description    string         `gorm:"type:text" json:"description,omitempty"`
    ActiveForm     string         `gorm:"type:text" json:"activeForm,omitempty"`
    Status         string         `gorm:"not null;type:text;index:idx_td_conv_status,priority:2;default:pending" json:"status"`
    Owner          string         `gorm:"type:text" json:"owner,omitempty"`
    BlockedBy      []string       `gorm:"serializer:json" json:"blockedBy,omitempty"`
    Metadata       map[string]any `gorm:"serializer:json" json:"metadata,omitempty"`
    CreatedAt      time.Time      `json:"createdAt"`
    UpdatedAt      time.Time      `json:"updatedAt"`
    DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}
```

> **DB 表名走 GORM 默认复数化**：`Todo` struct → `todos` 表（无显式 `TableName()` 重写）。

### 字段说明

| 字段 | 说明 |
|---|---|
| `ID` | `td_<16hex>` 格式（per §S15）；8 字节 crypto/rand |
| `ConversationID` | 作用域键；todo 跨 conversation 不可移植；Service 在变更前断言匹配 |
| `Subject` | 必填、非空；imperative 一行（"Run tests"）|
| `Description` | 可选；长文上下文 |
| `ActiveForm` | 可选；present continuous（"Running tests"）；UI 在 in_progress 状态显示 spinner 文案用 |
| `Status` | 4 值白名单（见 §4），app 层校验，DB 不 CHECK——便于将来扩展 |
| `Owner` | 可选；agent 名（多 agent 协作未来）|
| `BlockedBy` | JSON 序列化的 todo ID 数组；依赖关系（不强约束，仅信息）|
| `Metadata` | JSON 自由扩展位 |

### 复合索引

```
INDEX idx_td_conv_status (conversation_id, status)
```

ListByConversation + 未来按 status 过滤（如"只显示未完成"）共用此索引。

### Sentinel 错误（4 个）

```go
var (
    ErrNotFound             = errors.New("todo: not found")
    ErrSubjectRequired      = errors.New("todo: subject is required")
    ErrInvalidStatus        = errors.New("todo: invalid status")
    ErrConversationMismatch = errors.New("todo: conversation mismatch")
)
```

`ErrConversationMismatch` 仅 domain 层定义；**Service 层把跨 conversation 访问转成 `ErrNotFound`**，避免向 LLM/前端泄漏"该 ID 在另一对话存在"的存在性信息。

---

## 4. Status 枚举

```go
const (
    StatusPending    = "pending"
    StatusInProgress = "in_progress"
    StatusCompleted  = "completed"
    StatusDeleted    = "deleted"
)

func IsValidStatus(s string) bool { ... }  // 白名单 4 值
func ListStatuses() []string      { ... }  // 同 4 值；契约测试支撑
```

**生命周期**：`pending → in_progress → completed`（终态）。任意时点可标 `deleted`（实际走 `Service.Delete` 软删 + 最终快照广播）。

**为什么 app 层校验而非 DB CHECK**：与 model_configs.scenario 同理——未来加 status 不需要 schema 迁移；扩展性优先于约束严格性。

---

## 5. Repository 接口（`internal/domain/todo/todo.go`）

```go
type Repository interface {
    Create(ctx context.Context, t *Todo) error
    Get(ctx context.Context, id string) (*Todo, error)
    ListByConversation(ctx context.Context, conversationID string) ([]*Todo, error)
    Update(ctx context.Context, t *Todo) error
    SoftDelete(ctx context.Context, id string) error
}
```

实现在 `internal/infra/store/todo/todo.go`：
- `Create` / `Update` 走 GORM `Create` / `Save`
- `Get` 把 `gorm.ErrRecordNotFound` → `ErrNotFound`
- `ListByConversation` 按 `conversation_id` 过滤，`ORDER BY created_at ASC`（LLM 看到创建顺序）
- `SoftDelete` GORM 软删 + `RowsAffected==0` 视为 not found 返规范 sentinel

**作用域**：store 仅按 `ConversationID` 过滤，**不**强制 user_id 所有权——chat-runner 印的 ctx 里有 user_id，conversation_id 来自前端 URL（已在 chat 层校验对应用户）。Service 层做 `t.ConversationID != ctxConvID` 断言。

---

## 6. Service 层（`internal/app/todo/todo.go`）

```go
type Service struct {
    repo   tododomain.Repository
    bridge eventsdomain.Bridge   // 每次变更发 entity-state SSE
    log    *zap.Logger
}

type CreateInput struct {
    Subject     string
    Description string
    ActiveForm  string
    BlockedBy   []string
    Metadata    map[string]any
}

type UpdateInput struct {
    Subject     *string         // pointer encodes "set" vs "unchanged"
    Description *string
    ActiveForm  *string
    Status      *string
    Owner       *string
    BlockedBy   *[]string
    Metadata    map[string]any  // map: nil = unchanged, non-nil = set
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*tododomain.Todo, error)
func (s *Service) Get(ctx context.Context, id string) (*tododomain.Todo, error)
func (s *Service) List(ctx context.Context) ([]*tododomain.Todo, error)
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*tododomain.Todo, error)
func (s *Service) Delete(ctx context.Context, id string) error
```

**关键行为**：
1. 每方法首句 `RequireConversationID(ctx)` 失败立即返 `ErrMissingConversationID`
2. Get/Update/Delete 加载后断言 `t.ConversationID != convID` 返 `ErrNotFound`（防泄漏）
3. Create / Update / Delete 末尾 `s.publish(ctx, t)` 广播 entity-state（best-effort）
4. Delete 把最终快照的 `Status` 设为 `"deleted"`，广播让订阅方丢本地拷贝

**ID 生成**：`newID() = idgenpkg.New("td")`（§S15）。

---

## 7. 4 个 System Tool

注入位置：`cmd/server/main.go` + `test/harness/harness.go` 末段：
```go
todoService := todoapp.NewService(todostore.New(gdb), eventsBridge, log)
tools = append(tools, todotool.TodoTools(todoService)...)
```

### 7.1 TodoCreate（`internal/app/tool/todo/create.go`）

| 字段 | 必填 | 说明 |
|---|---|---|
| `subject` | ✅ | imperative 一行 |
| `description` | | 长文 |
| `active_form` | | UI 文案 |
| `blocked_by` | | 依赖 todo ID 数组 |

返回：新 Todo 的缩进 JSON。
错误：`ErrSubjectRequired` → "Todo subject is required and must be non-empty."

### 7.2 TodoList

无参数。返回：
```json
{
  "total": 3,
  "todos": [{...}, {...}, {...}]
}
```

按 created_at ASC 排，过滤掉软删。

### 7.3 TodoGet

| 字段 | 必填 |
|---|---|
| `todo_id` | ✅ |

返回：单 Todo 的缩进 JSON。
错误：`ErrNotFound` → "Todo not found in this conversation."（含跨 conversation 防泄漏场景）。

### 7.4 TodoUpdate

| 字段 | 必填 | 说明 |
|---|---|---|
| `todo_id` | ✅ | |
| `subject` | | 非空时校验非空 |
| `description` | | 空字符串清空 |
| `active_form` | | |
| `status` | | enum 4 值 |
| `owner` | | |
| `blocked_by` | | 整体替换；空数组清空 |

特例：**`status: "deleted"` 路由到 `Service.Delete`**——软删 + 最终快照集中一处。返回 `{"deleted": true, "id": "td_..."}`。
其他更新返完整新 Todo 的 JSON。

---

## 8. SSE 事件（`internal/domain/events/types.go`）

```go
type Todo struct {
    *tododomain.Todo
}

func (Todo) EventName() string { return "todo" }

func (e Todo) MarshalJSON() ([]byte, error) {
    if e.Todo == nil { return []byte("null"), nil }
    return json.Marshal(e.Todo)
}
```

**触发点**：Service.Create / Update / Delete 内部 `bridge.Publish(ctx, t.ConversationID, eventsdomain.Todo{Todo: t})`。
**过滤 key**：`conversationId`（与 chat.message / forge / conversation 同标准）。
**Wire 形状**：与 `Todo` entity JSON 一致，无 wrapper key。

---

## 9. 测试覆盖

| 层 | 文件 | 测试数 | 覆盖 |
|---|---|---|---|
| domain | `internal/domain/todo/todo_test.go` | 4 | IsValidStatus / ListStatuses 契约 |
| store | `internal/infra/store/todo/todo_test.go` | 7 | CRUD + 跨 conv 隔离 + 软删不可见 |
| app/todo | `internal/app/todo/todo_test.go` | 13 | Create / Get / List / Update / Delete + cross-conv 防泄漏 + ID 前缀 + bridge publish 断言 |
| app/tool/todo | `internal/app/tool/todo/todo_test.go` | 17 | TodoTools factory + 每工具 identity / Validate / Execute |
| pipeline | `test/uxtodo/uxtodo_test.go::TestUxTodo_TodoCreateAndList` | 1 场景 | LLM ↔ tool 端到端 |

总计 41+ 单元/集成测试 + 1 pipeline 场景。

---

## 10. 附：ask 服务 + AskUserQuestion 工具

> **2026-05-05 batch 5A 重构**：ask 的完整设计已迁出，独立成 [`./ask.md`](./ask.md)（与 filesystem / search / web / shell 各家族一致的独立 design doc 模式）。本节保留作为"与 todo 同 V1 batch（U2-U3）落地"的历史关联指针。
>
> **快速摘要**：`app/ask.Service` 是 in-memory 会合（`toolCallID → channel`）；`AskUserQuestion` 工具调 `Wait` 阻塞 5 分钟；HTTP `POST /api/v1/conversations/{id}/answers` 调 `Resolve` 原子摘条目唤醒；问题本身坐 `chat.message` SSE 流（决策 D11，不新建事件家族）。完整设计、Service / Tool / Handler 分层、错误码、测试覆盖见 [`./ask.md`](./ask.md)。

---

## 11. 与其他 domain 的关系

| 关系 | 说明 |
|---|---|
| **chat** | LLM 在 chat agent 循环里调本 domain 的 4 工具 + AskUserQuestion；conversation_id 通过 ctx 透传 |
| **conversation** | 弱关联——todo.conversation_id 引用 conversations.id 但**不**加 FK（softdelete + 跨 conv 隔离已经够，FK 仅增加迁移负担）|
| **events** | todo 事件 entity-state 模式与 forge / conversation 同标准 |
| **reqctx** | 依赖 `RequireConversationID`（本 batch 新加 sentinel）+ `GetToolCallID`（ask 工具用）|

---

## 12. 演化方向（未来）

- **依赖图渲染**：UI 用 `BlockedBy` 画 DAG；当前后端只存不强制
- **跨 agent 协作**：Owner 字段已在 entity 里；多 agent workflow 落地后承担调度
- **持久化用户工单**：当前 todo 软删后仍能 Get（GORM 默认过滤可绕过）；将来加专门的"已归档"视图
- **HTTP CRUD 端点**（如 `GET /api/v1/conversations/{id}/todos`）：当前不暴露；前端通过 SSE `todo` 事件维护状态，无需主动拉取
