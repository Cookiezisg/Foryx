# Memory — 跨对话长期记忆

**Phase**：V1.2 §2 final-sweep（与 compaction 同批落地，可独立 ship）
**状态**：📐 设计期
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/database-design.md`](../service-contract-documents/database-design.md) — `memories` 表
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — memory ×3 sentinel
- [`../service-contract-documents/api-design.md`](../service-contract-documents/api-design.md) — memory CRUD 端点
- [`./compaction.md`](./compaction.md) — 与 §1 联动的"逃生通道"
- 参考：[Anthropic Memory tool](https://platform.claude.com/docs/en/agents-and-tools/tool-use/memory-tool)（spec 参考；我们 SQLite 后端不走 filesystem）

---

## 1. 一句话

**跨对话持久的事实仓库**。一张全局 `memories` 表，存"用户是谁 / 偏好 / 当前在做啥 / 外部系统指针"四类条目。AI 通过 3 个 system tool（`read_memory` / `write_memory` / `forget_memory`）自管，用户可以在 testend 面板审查、pin、编辑、删。

**没有 conversation 维度**——所有 memory 都是用户级，跨所有对话共享。**没有 AGENTS.md / CLAUDE.md 文件**——用户的"全局指令"就是一条 `pinned=true` 的 user 类型 memory。

---

## 2. 端到端推演（设计原则 #5）

### 启动 / 新对话期

```
用户开新对话 → chat.Service.Create → 跳转 chat 界面
  ↓
用户发第一条消息 → chat.Service.Send → runner.processTask
  ↓
runner.buildSystemPrompt(ctx, conv) 拼装：
  [STATIC]                                       ← cache_control: ephemeral
    - Base prompt
    - Tool defs (含 read_memory / write_memory / forget_memory)
    - Catalog summary
    ──── Pinned memories（完整内容）────
    memoryService.ListPinned(ctx) → 全文拼进去
    ──── Memory Index（≤200 行）────
    memoryService.ListIndex(ctx) → 每条 `- [type] name: description`
  [DYNAMIC]
    - locale / now / task budget
  [CONV OVERRIDE]
    - conversation.SystemPrompt（如果非空）
```

### 运行期 — AI 调 read_memory

```
LLM 看到 system prompt 里的 index → 决定"我需要 user_role 那条"
  ↓
tool_use: read_memory({"name": "user_role"})
  ↓
app/tool/memory.ReadMemory.Execute
  ↓
memoryService.Get(ctx, "user_role")
  ├─ 命中 → 返 content，accessed_at + access_count 更新
  └─ 未命中 → 返 friendly error "Memory 'user_role' not found"
  ↓
tool_result → LLM 继续
```

### 运行期 — AI 调 write_memory

```
LLM 在对话中学到"用户用 Python 3.12"
  ↓
tool_use: write_memory({
  "name": "user_python_version",
  "type": "user",
  "description": "User's Python version is 3.12",
  "content": "User explicitly mentioned using Python 3.12 with type hints..."
})
  ↓
app/tool/memory.WriteMemory.Execute
  ↓
memoryService.Upsert(ctx, ...):
  ├─ name 已存在 → update content/description/updated_at
  └─ 不存在 → insert with source="ai"
  ↓
推 SSE notification: {type:"memory", id:"<memId>", data:{action:"created"/"updated"}}
  ↓
testend memory 面板实时刷新
```

### 运行期 — 用户在 testend 手动编辑

```
testend 用户在 memories 面板点 [+ New] / [✏️ Edit] / [🗑 Delete] / [📌 Pin]
  ↓
HTTP POST/PATCH/DELETE /api/v1/memories/...
  ↓
handler → memoryService → repo
  ↓
推 SSE notification（同上）
```

**端到端跨 domain 依赖**：
- `chat/runner.go::buildSystemPrompt` 注入 pinned + index（**唯一改造点**）
- `pkg/notifications.Publisher` 推 `memory` entity 通知
- 无 conv 依赖、无 user_id 复杂度（单用户）

---

## 3. 设计原则

| 原则 | 落地 |
|---|---|
| **全局，无 scope** | 一张表，所有对话共享。conv id 不进 schema |
| **AI + 用户共写** | `source` 字段区分；行为无差异，仅 UI 显示徽章 |
| **4 类 categorizing**（CoALA 框架）| `type` 字段：user/feedback/project/reference |
| **pinned 决定可见度** | pinned=true 全文进 system prompt；false 只进 ≤200 行 index |
| **Index 上限 200 行** | 超出按 `accessed_at DESC + access_count DESC` 排，最不常用的不进 index（但 AI 知道 name 仍能 read）|
| **SQLite 后端**（不是文件）| AI 视角是"文件"（tool 接口），后端是 GORM 行——0 fs 复杂度 |
| **3 tool 而非 6**（Anthropic spec 简化）| read/write/forget 够用；str_replace/insert 等 line-level 编辑对 AI 是负担 |
| **不做向量搜索（V1）** | 4 类 + index + AI 自取 name 已够；语义搜索 Phase 5 RAG 时一起做 |

---

## 4. 领域模型

### Memory entity（`internal/domain/memory/memory.go`）

```go
type Memory struct {
    ID          string         `gorm:"primaryKey;type:text" json:"id"`           // mem_<16hex>
    Name        string         `gorm:"not null;type:text" json:"name"`           // LLM-facing identifier
    Type        string         `gorm:"not null;type:text;check:type IN ('user','feedback','project','reference')" json:"type"`
    Description string         `gorm:"not null;type:text" json:"description"`     // 1 line，进 MEMORY index
    Content     string         `gorm:"not null;type:text" json:"content"`         // markdown 完整内容
    Pinned      bool           `gorm:"not null;default:false" json:"pinned"`
    Source      string         `gorm:"not null;type:text;check:source IN ('user','ai')" json:"source"`
    Metadata    map[string]any `gorm:"serializer:json;type:text;default:'{}'" json:"metadata,omitempty"`  // 扩展位
    
    CreatedAt   time.Time      `json:"createdAt"`
    UpdatedAt   time.Time      `json:"updatedAt"`
    AccessedAt  *time.Time     `json:"accessedAt,omitempty"`                       // AI 上次 read_memory 时间
    AccessCount int            `gorm:"not null;default:0" json:"accessCount"`
    DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Memory) TableName() string { return "memories" }
```

### 4 个 Type 常量

```go
const (
    TypeUser      = "user"       // 关于用户本人（角色 / 专业 / 长期事实）
    TypeFeedback  = "feedback"   // 偏好 / 纠正（"don't do X"）
    TypeProject   = "project"    // 当前在搞的事（变更频繁）
    TypeReference = "reference"  // 外部系统指针（"bugs in Linear FORGE"）
)

func IsValidType(t string) bool {
    switch t {
    case TypeUser, TypeFeedback, TypeProject, TypeReference:
        return true
    }
    return false
}
```

### Source 常量

```go
const (
    SourceUser = "user"  // testend / HTTP API 用户写
    SourceAI   = "ai"    // LLM 通过 write_memory tool 写
)
```

### Sentinel（3 个）

```go
var (
    ErrNotFound       = errors.New("memory: not found")
    ErrNameConflict   = errors.New("memory: name already exists")  // 仅 Create 路径（write_memory 是 upsert，不返）
    ErrInvalidName    = errors.New("memory: invalid name format")  // 必须 [a-z][a-z0-9_]{0,63}
)
```

**故意没有** `ErrInvalidType` / `ErrInvalidSource`——枚举校验在 DB CHECK 层，错了就是 GORM error 上抛 500（producer bug，不该到用户）。

### Index 行格式

`memoryService.ListIndex` 返一段渲染好的 markdown，最多 200 行：

```markdown
- [user] my_role: 我是 Go 后端工程师
- [user] python_version: Python 3.12
- [feedback] no_emoji: Don't use emojis
- [feedback] verbose_logging: 测试时多打 log
- [project] forgify_v12: 当前 Phase 4 收尾
- [reference] linear_forge: Bug tracker 在 Linear FORGE project
... (按 accessed_at DESC + access_count DESC 取前 200)
```

---

## 5. 持久化

### `memories` 表

主键 `mem_<16hex>`；软删；`UNIQUE(name) WHERE deleted_at IS NULL`（schema_extras partial unique 因为 GORM tag 不能表达条件 unique）。

字段已在 §4 列出。索引：

```sql
CREATE UNIQUE INDEX idx_memories_name ON memories (name) WHERE deleted_at IS NULL;
CREATE INDEX idx_memories_type_pinned ON memories (type, pinned);
CREATE INDEX idx_memories_accessed ON memories (accessed_at DESC, access_count DESC);
```

**没有 user_id 列**——单用户场景按 §设计原则 #6"反校验剧场"省。

---

## 6. Service 层（`internal/app/memory/`）

### 文件结构

```
app/memory/
  memory.go    ← Service + New + 接口断言
  crud.go      ← Get / List / ListByType / Search / Upsert / Pin / Unpin / Delete
  index.go     ← ListIndex + ListPinned + 索引渲染
  notify.go    ← publishChanged（推 `memory` notification）
```

### Service struct

```go
type Service struct {
    repo   memorydomain.Repository
    notif  notificationspkg.Publisher
    log    *zap.Logger
}

func New(repo memorydomain.Repository, notif notificationspkg.Publisher, log *zap.Logger) *Service {
    if log == nil { panic("memoryapp.New: nil logger") }
    if notif == nil { notif = notificationspkg.New(nil, log) }  // noop fallback
    return &Service{repo: repo, notif: notif, log: log}
}
```

### 主要方法

```go
// 给 chat runner 用
func (s *Service) ListPinned(ctx context.Context) ([]*Memory, error)   // 全文进 system prompt 的
func (s *Service) ListIndex(ctx context.Context, maxLines int) (string, error)  // markdown 索引段

// 给 tool 层用（read_memory / write_memory / forget_memory）
func (s *Service) Get(ctx context.Context, name string) (*Memory, error)
func (s *Service) Upsert(ctx context.Context, in UpsertInput) (*Memory, error)
func (s *Service) Delete(ctx context.Context, name string) error

// 给 HTTP / testend 用
func (s *Service) ListAll(ctx context.Context) ([]*Memory, error)
func (s *Service) ListByType(ctx context.Context, typ string) ([]*Memory, error)
func (s *Service) Pin(ctx context.Context, name string) error
func (s *Service) Unpin(ctx context.Context, name string) error
```

**`Get` 副作用**：更新 `accessed_at = now()` + `access_count++`。让 ListIndex 的"按 AI 访问频率排"自然 work。

### UpsertInput

```go
type UpsertInput struct {
    Name        string
    Type        string  // user / feedback / project / reference
    Description string
    Content     string
    Pinned      *bool   // nil = 保持现状（不在 patch 字段）
    Source      string  // "user" (HTTP) or "ai" (tool)
    Metadata    map[string]any
}
```

### 校验

- `Name`: regex `^[a-z][a-z0-9_]{0,63}$`（lowercase + underscore + digits）
- `Type`: 4 枚举之一
- `Description`: 非空，≤ 200 chars（保证 index 一行能装）
- `Content`: 非空（空内容没意义，删了就完）

校验失败返 `ErrInvalidName` / `ErrInvalidType`（domain sentinel） → errmap 翻 422。

---

## 7. 3 个 System Tool（`internal/app/tool/memory/`）

按 §S18 标准 9 方法实现。

### 7.1 `read_memory`

```go
func (t *ReadMemory) Description() string {
    return "Retrieve a specific memory by name. " +
           "Memories are persistent facts about the user, their preferences, " +
           "current projects, or external references. " +
           "Check the memory index in your system prompt to see what's available."
}

func (t *ReadMemory) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "name": {"type": "string", "description": "Memory name (from index)"}
        },
        "required": ["name"]
    }`)
}

func (t *ReadMemory) Execute(ctx context.Context, args string) (string, error) {
    var p struct{ Name string }
    json.Unmarshal([]byte(args), &p)
    
    mem, err := t.svc.Get(ctx, p.Name)
    if errors.Is(err, memorydomain.ErrNotFound) {
        return fmt.Sprintf("Memory '%s' not found. Available memories are listed in your system prompt index.", p.Name), nil
    }
    if err != nil {
        return "", err
    }
    return fmt.Sprintf("# %s (%s)\n\n%s", mem.Name, mem.Type, mem.Content), nil
}
```

### 7.2 `write_memory`

```go
func (t *WriteMemory) Description() string {
    return "Save a fact to long-term memory. Use when you learn something " +
           "worth remembering across conversations: user preferences, " +
           "their role/expertise, current project state, or external references.\n\n" +
           "Memory types:\n" +
           "- user: about the user themselves (role, expertise, long-term facts)\n" +
           "- feedback: their preferences or corrections ('don't use emojis')\n" +
           "- project: what they're currently working on\n" +
           "- reference: pointers to external systems (Linear projects, etc.)"
}

func (t *WriteMemory) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "name":        {"type": "string", "description": "Stable identifier, lowercase_with_underscores"},
            "type":        {"type": "string", "enum": ["user","feedback","project","reference"]},
            "description": {"type": "string", "description": "One-line summary (≤200 chars), shown in memory index"},
            "content":     {"type": "string", "description": "Full content in markdown"}
        },
        "required": ["name","type","description","content"]
    }`)
}
```

**默认 pinned=false**——AI 不能自己 pin（只用户能 pin）。理由：pin 影响每次 system prompt，不能让 AI 自管，否则 context bloat。

### 7.3 `forget_memory`

```go
func (t *ForgetMemory) Description() string {
    return "Delete a memory by name. Use when a memory is outdated, " +
           "incorrect, or the user explicitly asks you to forget something."
}
```

Args: `{"name": "..."}`。soft delete + 推 notification。

### Tool 静态元数据

| Tool | IsReadOnly | NeedsReadFirst | RequiresWorkspace |
|---|---|---|---|
| `read_memory` | ✅ | ❌ | ❌ |
| `write_memory` | ❌ | ❌ | ❌ |
| `forget_memory` | ❌ | ❌ | ❌ |

全都不依赖文件系统（SQLite 后端，不走 PathGuard）。

---

## 8. System prompt 注入（**Memory 跟 chat 的唯一接口**）

`chat/runner.go::buildSystemPrompt` 改造：

```go
func (r *runner) buildSystemPrompt(ctx context.Context, conv *Conversation) string {
    var sb strings.Builder
    
    // 1. 基础 prompt
    sb.WriteString(r.basePrompt)
    
    // 2. Tool defs (含 read/write/forget_memory)
    // ... tool def 拼装由 LLM client 处理 ...
    
    // 3. Catalog summary
    if r.catalog != nil {
        sb.WriteString(r.catalog.GetForSystemPrompt())
    }
    
    // 4. ★ Memory pinned 全文 ★
    if r.memory != nil {
        pinned, _ := r.memory.ListPinned(ctx)
        if len(pinned) > 0 {
            sb.WriteString("\n\n──── Pinned memories ────\n")
            for _, m := range pinned {
                fmt.Fprintf(&sb, "\n## %s (%s)\n%s\n", m.Name, m.Type, m.Content)
            }
        }
        
        // 5. ★ Memory index ★
        index, _ := r.memory.ListIndex(ctx, 200)
        if index != "" {
            sb.WriteString("\n\n──── Memory index ────\n")
            sb.WriteString(index)
            sb.WriteString("\n\nUse read_memory(name) to load specific entries when relevant.\n")
            sb.WriteString("Use write_memory(...) when you learn something worth keeping.\n")
        }
    }
    
    // 6. conversation override (per-conv)
    if conv.SystemPrompt != "" {
        sb.WriteString("\n\n──── This conversation ────\n")
        sb.WriteString(conv.SystemPrompt)
    }
    
    // 7. Dynamic (locale / now / task budget)
    sb.WriteString(r.buildDynamic(ctx))
    
    return sb.String()
}
```

**Pin 数量上限**：建议 ≤ 5 条（user 自己控制，超了 testend 警告但不强制）。原因：pinned 内容是 every-turn 成本，太多伤性能。

---

## 9. HTTP API（testend / 用户用）

| Method | Path | 用途 |
|---|---|---|
| GET | `/api/v1/memories` | 列表（`?type=user&pinned=true` filter）|
| GET | `/api/v1/memories/{name}` | 单条详情 |
| POST | `/api/v1/memories` | 创建（source 自动设为 user）|
| PATCH | `/api/v1/memories/{name}` | 编辑（partial: type/description/content/pinned）|
| DELETE | `/api/v1/memories/{name}` | 软删 |
| POST | `/api/v1/memories/{name}:pin` | Pin |
| POST | `/api/v1/memories/{name}:unpin` | Unpin |

**没有 cursor 分页**——memory 数量量级 ≤ 几百，全返足够。

### POST `/api/v1/memories` body

```json
{
  "name": "my_role",
  "type": "user",
  "description": "我是 Go 后端工程师，做 Forgify",
  "content": "<完整 markdown>",
  "pinned": true
}
```

`source` 自动 `"user"`（HTTP 进入的都视作用户写）。

---

## 10. Notifications

复用现有 `notifications` SSE bridge（不加新 SSE 流）：

```json
{
  "type": "memory",
  "id":   "<memId>",
  "data": {
    "action": "created" | "updated" | "deleted" | "pinned" | "unpinned",
    "name":   "<memName>",
    "memType": "<type>",
    "source":  "user" | "ai"
  }
}
```

**触发点**：每次 Service.Upsert / Delete / Pin / Unpin。slim payload（per §D-redo-6）——UI 拿通知后调 `GET /memories/{name}` 取全文。

---

## 11. 错误码

| Sentinel | HTTP | Wire Code |
|---|---|---|
| `memorydomain.ErrNotFound` | 404 | `MEMORY_NOT_FOUND` |
| `memorydomain.ErrNameConflict` | 409 | `MEMORY_NAME_CONFLICT` |
| `memorydomain.ErrInvalidName` | 400 | `MEMORY_INVALID_NAME` |

`ErrInvalidType` / `ErrInvalidSource` 不暴露 sentinel——DB CHECK 兜底，触发即 producer bug 上抛 500（用户不该撞到）。

---

## 12. testend UI（一个面板）

```
┌──────────────────────────────────────────────────────────────┐
│ Memories                                              [+ New] │
│ [All ▾] [user] [feedback] [project] [reference]              │
│ ┌────────────────────────────────────────────────────────┐   │
│ │ 📌 my_role            user      by user    2 days ago  │   │
│ │    我是 Go 工程师...                                    │   │
│ ├────────────────────────────────────────────────────────┤   │
│ │ 📌 no_emojis          feedback  by user    2 days ago  │   │
│ │    Don't use emojis                                     │   │
│ ├────────────────────────────────────────────────────────┤   │
│ │    pandas_over_polars feedback  by AI 🤖   1 hour ago  │   │
│ │    用 pandas 不用 polars                                │   │
│ │    [📌 Pin] [✏️ Edit] [🗑 Delete]                       │   │
│ └────────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────┘
```

特性：
- 📌 = pinned 图标
- 来源徽章（by user / by AI 🤖）
- 4 个 type filter + "All"
- 单条 hover 出现 [Pin/Unpin] [Edit] [Delete]
- 点条目展开看完整 content（monaco markdown editor）
- 新建按钮 → 一个 form：name + type 下拉 + description + content（markdown）+ pin checkbox

---

## 13. 测试覆盖

| 层 | 文件 | 覆盖 |
|---|---|---|
| domain | `internal/domain/memory/memory_test.go` | Type / Source 枚举 / sentinel 唯一性 / Memory JSON round-trip / Name regex 校验 |
| store | `internal/infra/store/memory/memory_test.go` | CRUD / Upsert（name 冲突）/ ListPinned 顺序 / ListByType filter / soft-delete partial unique reuse |
| app | `internal/app/memory/{memory,crud,index}_test.go` | Service.Get 副作用（accessed_at + access_count）/ ListIndex 200 行 cap + 排序 / Pin/Unpin 推通知 |
| tool | `internal/app/tool/memory/memory_test.go` | 3 tool × 9 方法 / ValidateInput / Execute friendly error |
| transport | `internal/transport/httpapi/handlers/memory_test.go` | 7 endpoints happy + error 分支 + JSON validation |
| pipeline | `backend/test/memory/memory_test.go` | E2E: 用户 POST 创 memory → chat 启动看到 system prompt 含它 / AI 调 write_memory → 持久化 + notification / pin/unpin 影响 system prompt |

---

## 14. 与其他 domain 的关系

| domain | 关系 |
|---|---|
| **chat** | `runner.buildSystemPrompt` 注入 pinned 内容 + index（**唯一接口**）|
| **conversation** | 完全无关——memory 不依赖 conv，conv 不依赖 memory |
| **compaction** (§1) | 压缩时识别"该长期记住的事实" → 推 suggestion / 自动 write_memory（详 compaction.md） |
| **catalog** | 不实现 CatalogSource——memory 不是"capability"，AI 通过 tool 调用，跟 forge/skill/mcp 性质不同 |
| **notifications** | 经 `notificationspkg.Publisher` 推 `memory` entity 通知 |
| **agentstate** | 无关系——memory 是跨对话，agentstate 是对话内 |

### 包依赖

```
internal/domain/memory/          (memory.go: Memory + Type/Source 枚举 + sentinel + Repository)
        ↑
internal/app/memory/             (Service: ListPinned/ListIndex/Get/Upsert/Pin/Unpin/Delete)
        ↑
internal/app/tool/memory/        (3 tools: read/write/forget)
internal/transport/httpapi/handlers/memory.go  (HTTP CRUD)
internal/app/chat/runner.go      (注入 system prompt)
```

无循环依赖。

---

## 15. 演化方向

- **语义搜索**（Phase 5 RAG 时一起做）：memory.content 加向量列，search_memory tool 按 query 召回。当前 4 类 + index + 按 name read 已够。
- **跨设备同步**（远期）：用户多设备共享 memory。需要 user_id 真化 + 远程存储。**v1.2 不做**。
- **Memory diff history**：每次 Upsert 留旧 content 一段时间，让用户能"undo"。**v1.2 不做**。
- **重要性自动排序**：超 200 行索引时，目前按 `accessed_at + access_count` 排。未来可加更复杂的"AI 评估重要性"机制。**v1.2 不做**。
- **Pinned 数量软上限警告**：testend 在 pinned ≥ 5 条时显示警告"system prompt 会变长，建议精简"。**v1.2 可做**。
- **Slash command**（如果 §10 加了 slash command）：`/memory list` / `/memory pin xxx` 等。

---

## 16. 关键决策记录

| 决策 | 选项 | 选了 | 理由 |
|---|---|---|---|
| 后端存储 | filesystem / SQLite | **SQLite** | Forgify 已有 DB，backup/搜索/事务全免费 |
| Scope | conv-level / user-level / 双层 | **user-level 全局** | 单用户单机 chat app，conv-scoped 增加复杂度无收益 |
| Tool 数量 | Anthropic 6 命令 / 简化 3 命令 | **3 命令** | str_replace/insert 对 AI 是负担，read/write/forget 够覆盖 |
| Type 数量 | 自由标签 / 固定枚举 | **固定 4 类**（CoALA）| 行业共识，AI 训练数据里 user/feedback/project/reference 概念清晰 |
| AGENTS.md 文件 | 单独文件 / merge 进 memory | **merge** | 一种心智模型，pinned 是同一开关 |
| AI 能 pin 吗 | 能 / 不能 | **不能** | pinned 影响 every-turn 成本，只用户能控制 |
| 索引上限 | 100 / 200 / 500 行 | **200 行** | Claude Code 实践数字，cache 友好 |

---

## 17. 历史

- 2026-05-15 设计完成（与 §1 compaction 同批）。AGENTS.md 文件方案被否——用户明确"Forgify 没项目概念"。落地为 pinned 类型 memory。
