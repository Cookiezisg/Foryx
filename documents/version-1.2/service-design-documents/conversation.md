# conversation domain — 详细设计文档

**所属 Phase**：Phase 2（基础对话能力，第 3 个完成的 domain）
**状态**：✅ 已实现（2026-04-25 全部 7 步完成）
**职责**：管理对话线程的元数据（创建、列表、改名、软删）。Conversation 是 chat 消息的容器，本身不含消息内容——消息历史由 `chat` domain 管理。
**依赖**：
- `infra/db`（GORM 底层）+ `pkg/reqctx`（userID ctx 读取）
- **不依赖** `domain/apikey`、`domain/model`（conversation 是纯 CRUD）

**被依赖**：`chat` domain 在发消息时需要传入 `conversationId` 锚定线程。

**关联文档**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/api-design.md`](../service-contract-documents/api-design.md) — API 索引
- [`../service-contract-documents/database-design.md`](../service-contract-documents/database-design.md) — 表索引
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — 错误码索引

---

## 1. 为什么要这个 domain

chat 发消息时需要一个"线程 ID"表示"这条消息属于哪次对话"。Conversation domain 提供这个 ID，并管理对话的元数据（标题、创建时间等），让前端侧边栏能列出历史对话。

**职责边界**：
- Conversation domain 管：线程元数据（ID / 标题 / 时间戳）
- Chat domain 管：消息发送、流式输出、消息历史存储

---

## 2. 核心决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 消息是否存在本 domain | **不存** | 消息历史属于 chat domain，Conversation 只是容器 |
| Title 是否必填 | **不必填** | 用户可建空标题对话；首轮对话完成后由 chat domain 异步 auto-titling goroutine 回写（已实现），并推 `conversation.title_updated` SSE |
| 归档 vs 软删 | **软删统一** | 遵循 D1 规范，`deleted_at` 覆盖"归档"语义 |
| 绑定工具/工作流 | **未做** | conversation 保持纯线程容器；entity binding 推迟到 Phase 4-5 真有跨 entity 切换需求时再讨论 |
| 分页 | **cursor 分页** | 遵循 N4 规范，与 apikey.List 相同机制 |

---

## 3. 多租户准备

继承项目级约定（同 apikey / model）：
- 表带 `user_id TEXT NOT NULL`
- Phase 2 ctx 注入 `"local-user"`
- 每个 store 方法首先 `reqctx.GetUserID(ctx)` 取值；缺失返接线 bug 错误

---

## 4. 领域模型

### Conversation struct（`internal/domain/conversation/conversation.go`）

```go
type Conversation struct {
    ID           string         `gorm:"primaryKey;type:text" json:"id"`
    UserID       string         `gorm:"not null;index;type:text" json:"-"`
    Title        string         `gorm:"not null;type:text;default:''" json:"title"`
    AutoTitled   bool           `gorm:"not null;default:false" json:"autoTitled"`       // true = AI 自动生成，false = 用户手动改
    SystemPrompt string         `gorm:"type:text;default:''" json:"systemPrompt,omitempty"` // 对话级自定义系统提示词
    CreatedAt    time.Time      `json:"createdAt"`
    UpdatedAt    time.Time      `json:"updatedAt"`
    DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Conversation) TableName() string { return "conversations" }
```

**字段说明**：

| 字段 | 说明 |
|---|---|
| `ID` | `cv_<16hex>` 格式（8 字节 crypto/rand）|
| `UserID` | JSON `"-"`，前端不可见 |
| `Title` | 用户改名后填入；初始可为空；auto-titling goroutine 首轮完成后回写 |
| `AutoTitled` | `true` = AI 自动生成；用户手动改名后置 `false` |
| `SystemPrompt` | 对话级自定义系统提示词；空则只用全局基础提示词；chat.Service 读取后注入 MessageModifier |
| `DeletedAt` | 软删，GORM 内置 |

### 错误 sentinel

```go
var ErrNotFound = errors.New("conversation: not found")
```

映射：`404 CONVERSATION_NOT_FOUND`（见 `errmap.go`）。

---

## 5. Repository 接口

```go
type Repository interface {
    Save(ctx context.Context, c *Conversation) error
    Get(ctx context.Context, id string) (*Conversation, error)
    List(ctx context.Context, filter ListFilter) ([]*Conversation, string, error)
    Delete(ctx context.Context, id string) error
}

type ListFilter struct {
    Cursor string
    Limit  int
}
```

### Store 实现（`infra/store/conversation/conversation.go`）

- `Save` = GORM `Save()`（INSERT on new PK / UPDATE on existing PK）
- `Get` = `WHERE id=? AND user_id=?` + GORM 自动 `AND deleted_at IS NULL`
- `List` = `(created_at, id)` 元组 cursor 稳定分页，`ORDER BY created_at DESC, id DESC`
- `Delete` = GORM 软删 + `RowsAffected == 0` 返 `ErrNotFound`

---

## 6. Service 层（`app/conversation/conversation.go`）

### Struct + 构造

```go
type Service struct {
    repo convdomain.Repository
    log  *zap.Logger
}

func NewService(repo convdomain.Repository, log *zap.Logger) *Service {
    if log == nil { panic("conversation.NewService: logger is nil") }
    return &Service{repo: repo, log: log}
}
```

### 方法签名

```go
func (s *Service) Create(ctx context.Context, title string) (*Conversation, error)
func (s *Service) List(ctx context.Context, filter ListFilter) ([]*Conversation, string, error)
func (s *Service) Rename(ctx context.Context, id, title string) (*Conversation, error)
func (s *Service) Delete(ctx context.Context, id string) error
```

### Create 流程

```
1. reqctx.GetUserID(ctx) → uid（缺失 = 接线 bug，上抛）
2. 构造 Conversation{ID: newID(), UserID: uid, Title: TrimSpace(title), ...}
3. repo.Save(ctx, c)
4. log.Info("conversation created", conversation_id, user_id)
```

### Rename 流程

```
1. repo.Get(ctx, id) → c（未命中 → ErrNotFound → 404）
2. c.Title = TrimSpace(title)
3. c.UpdatedAt = time.Now().UTC()
4. repo.Save(ctx, c)
```

### ID 生成

```go
func newID() string  // "cv_" + 16 hex（8 字节 crypto/rand）
```

---

## 7. HTTP API 详细

### 通用约定

- 前缀：`/api/v1/conversations`
- 中间件链：同 apikey（Recover → RequestLogger → CORS → InjectLocale → InjectUserID）
- 响应走 envelope（N1）

### 端点清单（4 个）

#### 7.1 `POST /api/v1/conversations` — 创建（201）

**Request**：
```json
{ "title": "My Chat" }
```
`title` 可为空字符串或省略。

**Response 201**：完整 Conversation 对象（无 `userId` 字段）。

#### 7.2 `GET /api/v1/conversations` — 列表（200）

**Query**：`?cursor=&limit=50`（默认 50，最大 200）

**Response 200**：
```json
{
  "data": [ {...}, {...} ],
  "nextCursor": "<opaque>",
  "hasMore": true
}
```

空列表返 `{"data": [], "hasMore": false}`。

#### 7.3 `PATCH /api/v1/conversations/{id}` — 改名（200）

**Request**：
```json
{ "title": "New Name" }
```

**Response 200**：更新后的 Conversation。`updatedAt` 推进。

**404 `CONVERSATION_NOT_FOUND`**：id 不存在。

#### 7.4 `DELETE /api/v1/conversations/{id}` — 软删（204）

无 body 响应。再次 DELETE 返 404。

---

## 8. 数据库表

```sql
CREATE TABLE conversations (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  DATETIME
);

CREATE INDEX idx_conversations_user_id  ON conversations(user_id);
CREATE INDEX idx_conversations_deleted_at ON conversations(deleted_at);
```

**索引理由**：
- `user_id` 单索引：List 查询最常用（当前用户的所有对话）
- `deleted_at` 单索引：GORM 软删 filter

---

## 9. 事件

**Phase 2 不推送事件**。

未来可能加（Phase 5+）：
- `conversation.title_updated`：自动命名后推给前端（不用刷新列表）

---

## 10. 错误码（1 个）

| Code | HTTP | Sentinel | 场景 |
|---|---|---|---|
| `CONVERSATION_NOT_FOUND` | 404 | `conversation.ErrNotFound` | Get/Rename/Delete id 不存在 |

---

## 11. 完整调用链

### 11.1 POST /api/v1/conversations（创建）

```
前端 POST /api/v1/conversations  body={title}
  → middleware 链（Recover / Logger / CORS / InjectLocale / InjectUserID）
  → ConversationHandler.Create
      → decodeJSON → createConvRequest{Title}
      → svc.Create(ctx, title)
          → reqctx.GetUserID(ctx) → uid
          → newID() → "cv_<16hex>"
          → repo.Save(ctx, c)             [infra/store/conversation]
          → log.Info("conversation created")
      → response.Created(w, c) → 201
```

### 11.2 GET /api/v1/conversations（列表）

```
前端 GET /api/v1/conversations?cursor=&limit=
  → ConversationHandler.List
      → pagination.Parse(r) → {Cursor, Limit}
      → svc.List(ctx, filter)
          → repo.List(ctx, filter)        [infra/store/conversation]
              cursor 解码 → WHERE (created_at, id) < (?, ?)
              ORDER BY created_at DESC, id DESC LIMIT limit+1
      → response.Paged(w, items, next, hasMore)
```

### 11.3 PATCH /api/v1/conversations/{id}（改名）

```
前端 PATCH /api/v1/conversations/cv_xxx  body={title}
  → ConversationHandler.Rename
      → r.PathValue("id") → "cv_xxx"
      → decodeJSON → renameConvRequest{Title}
      → svc.Rename(ctx, id, title)
          → repo.Get(ctx, id)             [infra/store/conversation]
              未命中 → ErrNotFound → 404 CONVERSATION_NOT_FOUND
          → c.Title = TrimSpace(title)
          → c.UpdatedAt = now
          → repo.Save(ctx, c)
      → response.Success(200, c)
```

---

## 12. 与其他 domain 的协作图

```
         ┌───────────────────────────┐
         │  chat domain（Phase 2）   │  ← 发消息时携带 conversationId
         └──────────┬────────────────┘
                    │ conversationId 作锚点
                    ↓
         ┌──────────────────────────┐
         │  conversation.Service    │
         └──────────────────────────┘
                    │
                    ↓
         ┌──────────────────────────┐
         │  infra/store/conversation │
         └──────────────────────────┘

conversation 不依赖 apikey / model，后两者也不依赖 conversation。
```

---

## 13. 实现清单（✅ 已全部完成，2026-04-25）

### domain 层 ✅
- [x] `internal/domain/conversation/conversation.go` — Conversation struct + TableName + ListFilter + ErrNotFound + Repository

### infra 层 ✅
- [x] `internal/infra/store/conversation/conversation.go` — Store（Save / Get / List cursor 分页 / Delete 软删）
- [x] `internal/infra/store/conversation/conversation_test.go` — 11 个集成测试

### app 层 ✅
- [x] `internal/app/conversation/conversation.go` — Service（Create / List / Rename / Delete + nil logger 守护）
- [x] `internal/app/conversation/conversation_test.go` — 11 个单测（fake repo）

### transport 层 ✅
- [x] `internal/transport/httpapi/handlers/conversation.go` — ConversationHandler + 4 端点 + Register
- [x] `internal/transport/httpapi/handlers/conversation_test.go` — 6 个 E2E 契约测试

### 配套基础设施 ✅
- [x] `internal/transport/httpapi/response/errmap.go` — 1 条 conversation sentinel 映射
- [x] `internal/transport/httpapi/router/deps.go` — `ConversationService *convapp.Service` 字段
- [x] `internal/transport/httpapi/router/router.go` — 条件注册
- [x] `cmd/server/main.go` — `convstore.New(gdb)` → `convapp.NewService(...)` → `router.Deps`；`db.Migrate` 追加 `&convdomain.Conversation{}`

### 验收 ✅
- [x] 全仓 `go test -count=1 -race ./...` 零失败
- [x] `go build ./...` 通过
