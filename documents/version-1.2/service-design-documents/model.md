# model domain — 详细设计文档

**所属 Phase**：Phase 2（基础对话能力，第 2 个 domain）
**状态**：✅ 已实现（2026-04-25）
**职责**：为每个"场景"（scenario）记录用户选定的 `(provider, modelID)`；给 chat / workflow / knowledge 提供"我该用谁"的策略层
**依赖**：
- `infra/db`（GORM 底层）+ `pkg/reqctx`（userID ctx 读取）
- **不依赖** `domain/crypto`（无敏感数据）
- **不依赖** `domain/apikey`（provider 字符串由用户传，不做交叉校验，见核心决策 Q2）

**被依赖**：`chat.Service` / 未来的 `workflow` LLM 节点 / `knowledge` embedding 层，**全部通过 `modeldomain.ModelPicker` 接口**

**关联文档**：
- [`../backend-design.md`](../backend-design.md) — 总规范（设计原则 #5 端到端推演先行 + #6 反校验剧场）
- [`../service-contract-documents/api-design.md`](../service-contract-documents/api-design.md) — API 索引
- [`../service-contract-documents/database-design.md`](../service-contract-documents/database-design.md) — 表索引
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — 错误码索引

---

## 1. 为什么要这个 domain

chat 发消息时要回答"该调 OpenAI 的 gpt-4o 还是 Anthropic 的 claude-3-5-sonnet？"——**谁该决定这件事**？

当前三方零件分工：

| domain | 管 | 不管 |
|---|---|---|
| **apikey** | 凭证存储（"我是谁"）| "该用谁" |
| **model**（本 domain）| **策略**（"这个场景用谁"）| 怎么调 |
| **chat** / workflow / knowledge | 编排（"跑 LLM 调用"）| "该用谁" |

没有 model domain，"provider 从哪来"就没有归属——这个坑是在推演 chat 端到端调用链时发现的，立下了 **"端到端推演先行"** 设计原则（backend-design.md §设计原则 #5）。

---

## 2. 核心决策（已敲定）

| 决策 | 选择 | 理由 |
|---|---|---|
| Scenario 粒度 | **一个 scenario 最多 1 条活跃配置**（`UNIQUE(user_id, scenario)`）| 防止用户意外存两条互斥 |
| Scenario 白名单位置 | **app 层 `IsValidScenario()`**，**DB 不 CHECK** | 白名单会随 Phase 扩张（Phase 4 加 workflow_llm，Phase 5 加 embedding / intent），改 DB CHECK 成本高 |
| HTTP 路径形状（Q1）| **`/api/v1/model-configs/{scenario}`**（复数 + path param）| 单数 `/model-config` 把 Phase 4+ 扩展堵死；复数是 N5 规范 |
| 是否校验 provider 在 apikey 白名单（Q2）| **不校验** | 前端 dropdown 已筛；下游 `apikey.ResolveCredentials` 自然报 `API_KEY_PROVIDER_NOT_FOUND`（见设计原则 #6 反校验剧场）|
| 是否校验用户真有该 provider 的 key（Q3）| **不校验** | 用户"先设 model 后加 key"是合法流程；chat 时报错即可；model 不必 import apikey 接口 |
| DELETE 端点？| **不做** | 删 = 未配置 = chat 报 `MODEL_NOT_CONFIGURED`；用户要改直接 PUT 新值即可 |
| PATCH 端点？| **不做** | provider + modelId 强耦合（换 provider 必换 modelId），PATCH 分开改会造非法组合 |
| GET 单条 `/{scenario}`？| **不做（Phase 2）** | Phase 2 最多 1 条，GET 列表够；未来 scenario 多了再加 |
| 事件 | **无**（Phase 2 不推） | 配置类资源由用户主动改，无异步通知需求 |

---

## 3. 多租户准备

继承项目级约定（同 apikey）：

- 表带 `user_id TEXT NOT NULL`
- 方法首次动作：`reqctx.GetUserID(ctx)` 取值；缺失返 `fmt.Errorf("modelstore: missing user id in context")` —— 接线 bug，不是 401
- Phase 2 ctx 注入 `"local-user"`

---

## 4. Scenario 白名单

代码位置：`internal/domain/model/model.go`

### Phase 2 清单（1 个）

| Scenario 常量 | 值 | 含义 | 典型模型 |
|---|---|---|---|
| `ScenarioChat` | `"chat"` | 用户主对话（`POST /chat/messages` 走的）| GPT-4o / Claude Sonnet / DeepSeek Chat |

### 演化（其他 Phase 再加 const）

| Phase | 可能新增 | 说明 |
|---|---|---|
| Phase 3 | `ScenarioForgeCode`（**待定**）| 锻造工具时代码生成模型；也可能复用 `chat`，到时候定 |
| Phase 4 | `ScenarioWorkflowLLM` | 工作流 LLM 节点（常跑批量，用户可能想挑便宜/快的模型）|
| Phase 5 | `ScenarioEmbedding` | 知识库向量化（属于另一类模型：text-embedding-3-small / bge 等）|
| Phase 5 | `ScenarioIntent` | 意图识别（Haiku / gpt-4o-mini 等小模型省钱）|

**扩展方式**：新增一个 const + 在 `IsValidScenario()` 返回 true + `ModelPicker` 接口加相应方法（如 `PickForWorkflow`）+ errmap 保持不变。**API 形状不变**。

### 工具函数（代码设计）

```go
// internal/domain/model/model.go
const (
    ScenarioChat = "chat"
    // Phase 3+: 随 Phase 加 const
)

func IsValidScenario(s string) bool {
    switch s {
    case ScenarioChat:
        return true
    default:
        return false
    }
}

func ListScenarios() []string {
    return []string{ScenarioChat}
}
```

---

## 5. 领域模型

### ModelConfig struct（`internal/domain/model/model.go`）

```go
// internal/domain/model/model.go

type ModelConfig struct {
    ID        string         `gorm:"primaryKey;type:text" json:"id"`
    UserID    string         `gorm:"not null;type:text;uniqueIndex:idx_mc_user_scenario,priority:1" json:"-"`
    Scenario  string         `gorm:"not null;type:text;uniqueIndex:idx_mc_user_scenario,priority:2" json:"scenario"`
    Provider  string         `gorm:"not null;type:text" json:"provider"`
    ModelID   string         `gorm:"not null;type:text" json:"modelId"`
    CreatedAt time.Time      `json:"createdAt"`
    UpdatedAt time.Time      `json:"updatedAt"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (ModelConfig) TableName() string { return "model_configs" }
```

### 字段说明

| 字段 | 说明 |
|---|---|
| `ID` | `mc_<16hex>` 格式（8 字节 crypto/rand，与 apikey 的 `aki_` 一致）|
| `UserID` | JSON 不输出（`json:"-"`，与前端无关）|
| `Scenario` | 白名单常量（Phase 2 仅 `"chat"`）|
| `Provider` | 11 白名单之一，**但不在 model 层校验**（反剧场原则）|
| `ModelID` | 字符串，如 `"gpt-4o"` / `"claude-3-5-sonnet-latest"`；**不校验**（不同 provider 的 model 命名无统一白名单）|
| 时间戳 | GORM 自动维护 |
| `DeletedAt` | 软删，GORM 内置 |

### 唯一约束

```
UNIQUE(user_id, scenario) WHERE deleted_at IS NULL
```

**注意**：GORM tag 里的 `uniqueIndex:idx_mc_user_scenario` 只产生全索引（不带 WHERE）。要做 partial UNIQUE，需要在 `infra/db/schema_extras.go` 追加 raw SQL：

```sql
-- schema_extras.go 需要加
DROP INDEX IF EXISTS idx_mc_user_scenario;
CREATE UNIQUE INDEX idx_mc_user_scenario
  ON model_configs(user_id, scenario)
  WHERE deleted_at IS NULL;
```

或者干脆**放弃软删**（model_configs 是设置类数据，审计价值低），硬删也可以。**开发时决定**（见 §18 遗留）。

### Sentinel 错误（4 个）

```go
// internal/domain/model/model.go
var (
    ErrNotConfigured    = errors.New("model: not configured for scenario")
    ErrInvalidScenario  = errors.New("model: invalid scenario")
    ErrProviderRequired = errors.New("model: provider is required")
    ErrModelIDRequired  = errors.New("model: model id is required")
)
```

映射见 §13 错误码。

---

## 6. 对外 API vs 对内函数（速查表）

### 6.1 对外两类消费者

| 消费者 | 接口 | 位置 | 方法数 |
|---|---|---|---|
| 🌐 **前端 / curl** | HTTP REST | `/api/v1/model-configs/*` | **2 个端点** |
| 🧩 **其他 domain**（chat / workflow / knowledge）| `modeldomain.ModelPicker` 接口 | `internal/domain/model/model.go` | **1 个方法**（Phase 2，随 Phase 加）|

### 6.2 HTTP REST（详见 §10）

```
GET  /api/v1/model-configs              列出当前用户所有 scenario 的配置（200）
PUT  /api/v1/model-configs/{scenario}   upsert 指定 scenario（200）
```

无 POST / PATCH / DELETE / GET-by-scenario（见 §2 核心决策）。

### 6.3 `ModelPicker` 接口（跨 domain 唯一入口）

```go
// domain/model/model.go

type ModelPicker interface {
    // PickForChat returns the (provider, modelID) for the user's main
    // chat scenario. Returns ErrNotConfigured if never set.
    //
    // PickForChat 返回当前用户主对话的 (provider, modelID)。
    // 用户未设置过返回 ErrNotConfigured。
    PickForChat(ctx context.Context) (provider, modelID string, err error)

    // Phase 4+ 按需追加方法（不泛化成 Pick(ctx, scenario) 的理由见下）
    // PickForWorkflow(ctx, nodeType string) (provider, modelID string, err error)
    // PickForEmbedding(ctx) (provider, modelID string, err error)
    // PickForIntent(ctx)    (provider, modelID string, err error)
}
```

**为什么不用通用 `Pick(ctx, scenario string)` 方法**：
- **类型安全**：拼错 `"cht"` 编译期抓不到；方法名拼错编译期立刻炸
- **演化独立**：`PickForWorkflow` 可能要 `nodeType` 参数，`PickForEmbedding` 可能不同返回值
- **调用点自文档**：chat 代码里写 `mp.PickForChat(ctx)` 一眼就懂

实现：`app/model.Service`（有 `var _ modeldomain.ModelPicker = (*Service)(nil)` 编译期守护）。

### 6.4 对内类型速查

| 类别 | 名字 | 位置 | 谁用 |
|---|---|---|---|
| Repository 接口 | `Repository` | `domain/model/model.go` | Service；其他 domain 不许 import |
| Repository 实现 | `Store` | `infra/store/model/store.go`（别名 modelstore） | main.go DI |
| Service（CRUD + ModelPicker 实现）| `Service` | `app/model/service.go`（别名 modelapp） | handler + main.go |
| ModelPicker 实现 | 同 `Service` | `app/model/modelpicker.go` | 其他 domain（通过接口） |
| Scenario 工具 | `ScenarioChat`, `IsValidScenario`, `ListScenarios` | `domain/model/model.go` | Service + handler 校验 |

---

## 7. Repository 接口

```go
// internal/domain/model/model.go

type Repository interface {
    // GetByScenario fetches the active config for (current user, scenario).
    // Returns ErrNotConfigured if none.
    //
    // GetByScenario 返回 (当前用户, scenario) 的活跃配置；无则返 ErrNotConfigured。
    GetByScenario(ctx context.Context, scenario string) (*ModelConfig, error)

    // List returns all active configs for the current user. No pagination
    // (Phase 2 has at most 1 entry; future phases ≤ 6).
    //
    // List 返回当前用户所有活跃配置；不分页（Phase 2 ≤ 1 条，未来 ≤ 6）。
    List(ctx context.Context) ([]*ModelConfig, error)

    // Upsert creates a new row or updates the existing (user_id, scenario)
    // row. Caller must have set m.UserID + m.Scenario before calling.
    //
    // Upsert 按 (user_id, scenario) 创建或更新。调用方须先填 m.UserID + m.Scenario。
    Upsert(ctx context.Context, m *ModelConfig) error
}
```

**注意**：无 `Delete` / `Get(id)` 方法 —— Phase 2 用不上，按需增加。

### Store 实现细节（`infra/store/model/model.go`）

- 每个方法前 `reqctx.GetUserID(ctx)` 取 uid，缺失返 wrapped 错误
- `GetByScenario`: `WHERE user_id=? AND scenario=? AND deleted_at IS NULL`
- `List`: `WHERE user_id=? AND deleted_at IS NULL ORDER BY scenario`
- `Upsert`: 尝试 `WHERE user_id=? AND scenario=?` 拿现有行 → 有则更新 ID 保持 + 字段改 + `Save()`；无则 `INSERT`
  - 并发安全靠 `UNIQUE(user_id, scenario) WHERE deleted_at IS NULL`
  - 或者走 GORM 的 `ON CONFLICT DO UPDATE` 语法（SQLite 支持）

---

## 8. Service 层

### Struct + 构造

```go
// app/model/service.go

type Service struct {
    repo modeldomain.Repository
    log  *zap.Logger
}

func NewService(repo modeldomain.Repository, log *zap.Logger) *Service {
    if log == nil {
        panic("model.NewService: logger is nil")
    }
    return &Service{repo: repo, log: log}
}
```

### Inputs

```go
// app/model/service.go

type UpsertInput struct {
    Provider string
    ModelID  string
}
```

（scenario 不放 UpsertInput 里，它来自 HTTP path param，由 handler 透传给 Service 的独立参数。）

### 方法签名

```go
// 对前端（HTTP handler 调）
func (s *Service) List(ctx context.Context) ([]*modeldomain.ModelConfig, error)
func (s *Service) Upsert(ctx context.Context, scenario string, in UpsertInput) (*modeldomain.ModelConfig, error)

// ModelPicker 接口实现（在 modelpicker.go）
func (s *Service) PickForChat(ctx context.Context) (provider, modelID string, err error)
```

### Upsert 流程

```
1. 校验 scenario：
   !modeldomain.IsValidScenario(scenario) → ErrInvalidScenario
2. 校验 body：
   strings.TrimSpace(in.Provider) == "" → ErrProviderRequired
   strings.TrimSpace(in.ModelID)  == "" → ErrModelIDRequired
3. reqctx.GetUserID(ctx) → uid（缺失 = 接线 bug，上抛）
4. 查现有：existing, err := repo.GetByScenario(ctx, scenario)
   err == ErrNotConfigured → 新建流程：
     m := &ModelConfig{ID: newID(), UserID: uid, Scenario: scenario, Provider: ..., ModelID: ...}
     repo.Upsert(ctx, m)
   err == nil → 更新流程：
     existing.Provider = in.Provider
     existing.ModelID  = in.ModelID
     existing.UpdatedAt = time.Now().UTC()
     repo.Upsert(ctx, existing)
5. log.Info("model config upserted", user_id, scenario, provider, model_id)
6. 返回最新的 *ModelConfig
```

### PickForChat 流程

```
1. m, err := repo.GetByScenario(ctx, ScenarioChat)
   err == ErrNotConfigured → 向上抛 ErrNotConfigured
2. return m.Provider, m.ModelID, nil
```

### ID 生成

```go
func newID() string {
    var b [8]byte
    if _, err := rand.Read(b[:]); err != nil {
        panic(fmt.Sprintf("model: crypto/rand failed: %v", err))
    }
    return "mc_" + hex.EncodeToString(b[:])
}
```

---

## 9. ConnectivityTester？

**不存在**。model domain 没有"测试"语义 —— 真实验证发生在 chat 调 LLM 时，上游返错才真能暴露"model 不存在"或"provider 拒绝"。"测试模型可用"不是 model 的职责。

---

## 10. HTTP API 详细

### 通用约定

- 前缀：`/api/v1/model-configs`
- 中间件链：同 apikey
- 响应走 envelope（N1）

### 端点清单（2 个）

#### 10.1 `GET /api/v1/model-configs` — 列表（200）

**Request**：无 body，无 query（不分页，最多 5-6 条）。

**Response 200**：
```json
{
  "data": [
    {
      "id": "mc_abc123",
      "scenario": "chat",
      "provider": "openai",
      "modelId": "gpt-4o",
      "createdAt": "2026-04-24T07:30:00Z",
      "updatedAt": "2026-04-24T07:30:00Z"
    }
  ]
}
```

从未配过 → `{"data": []}`（不是 null、不是 404）。

#### 10.2 `PUT /api/v1/model-configs/{scenario}` — upsert（200）

**Path param**：`scenario` ∈ `{"chat"}`（Phase 2 白名单）

**Request body**：
```json
{
  "provider": "openai",
  "modelId": "gpt-4o"
}
```

**Response 200**：完整的 `ModelConfig`（同 GET 单条形状）

**错误**：
- 400 `INVALID_REQUEST` — JSON 畸形 / 含未知字段（`DisallowUnknownFields`）
- 400 `INVALID_SCENARIO` — path scenario 不在白名单
- 400 `PROVIDER_REQUIRED` — body `provider` 空或仅空白
- 400 `MODEL_ID_REQUIRED` — body `modelId` 空或仅空白

**注意**：无 201（upsert 语义，既可创建也可覆盖，统一 200）。

### Handler 设计（`transport/httpapi/handlers/model.go`）

```go
type ModelConfigHandler struct {
    svc *modelapp.Service
    log *zap.Logger
}

func (h *ModelConfigHandler) Register(mux *http.ServeMux) {
    mux.HandleFunc("GET /api/v1/model-configs", h.List)
    mux.HandleFunc("PUT /api/v1/model-configs/{scenario}", h.Upsert)
}
```

---

## 11. 数据库表

```sql
CREATE TABLE model_configs (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL,
    scenario    TEXT NOT NULL,                    -- 白名单由 app 层校验
    provider    TEXT NOT NULL,                    -- 不 CHECK（反校验剧场）
    model_id    TEXT NOT NULL,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  DATETIME
);

-- 通过 GORM tag 生成（全索引，不带 WHERE）：
CREATE UNIQUE INDEX idx_mc_user_scenario ON model_configs(user_id, scenario);
CREATE INDEX        idx_mc_deleted_at    ON model_configs(deleted_at);

-- 由 schema_extras.go 追加（partial UNIQUE）：
DROP INDEX IF EXISTS idx_mc_user_scenario;
CREATE UNIQUE INDEX idx_mc_user_scenario
  ON model_configs(user_id, scenario)
  WHERE deleted_at IS NULL;
```

**迁移**：`cmd/server/main.go` 的 `db.Migrate(gdb, &modeldomain.ModelConfig{})` 末尾追加。

---

## 12. 事件

**Phase 2 不推送事件**。ModelConfig 是用户主动改的设置型资源，无需异步通知前端；前端操作完立刻 GET 列表刷新就行。

---

## 13. 错误码（4 个）

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `MODEL_NOT_CONFIGURED` | 422 | `model.ErrNotConfigured` | chat 调 `PickForChat` 时用户从未配过 | ⬜ |
| `INVALID_SCENARIO` | 400 | `model.ErrInvalidScenario` | PUT path `scenario` 不在白名单 | ⬜ |
| `PROVIDER_REQUIRED` | 400 | `model.ErrProviderRequired` | PUT body `provider` 空 | ⬜ |
| `MODEL_ID_REQUIRED` | 400 | `model.ErrModelIDRequired` | PUT body `modelId` 空 | ⬜ |

errmap 条目（新增）：
```go
// internal/transport/httpapi/response/errmap.go
modeldomain.ErrNotConfigured:    {http.StatusUnprocessableEntity, "MODEL_NOT_CONFIGURED"},
modeldomain.ErrInvalidScenario:  {http.StatusBadRequest, "INVALID_SCENARIO"},
modeldomain.ErrProviderRequired: {http.StatusBadRequest, "PROVIDER_REQUIRED"},
modeldomain.ErrModelIDRequired:  {http.StatusBadRequest, "MODEL_ID_REQUIRED"},
```

---

## 14. 消费方如何用（跨 domain 示例）

### chat.Service 调 LLM 时

```go
// internal/app/chat/chat.go（精简版）
type Service struct {
    modelPicker modeldomain.ModelPicker          // 只见接口
    keyProvider apikeydomain.KeyProvider         // 只见接口
    llmFactory  *llminfra.Factory                // 自有 LLM 流式客户端工厂
    // ...
}

func (s *Service) processTask(ctx context.Context, ...) {
    // 1. model domain 决定 (provider, modelID)
    provider, modelID, err := s.modelPicker.PickForChat(ctx)
    if err != nil { /* 422 MODEL_NOT_CONFIGURED → SSE chat.error */ }

    // 2. apikey domain 拿凭证
    creds, err := s.keyProvider.ResolveCredentials(ctx, provider)
    if err != nil { /* 404 API_KEY_PROVIDER_NOT_FOUND → SSE chat.error */ }

    // 3. 构造 LLM Client + 消费流
    client, _, err := s.llmFactory.Build(llminfra.Config{
        Provider: provider, ModelID: modelID,
        Key: creds.Key, BaseURL: creds.BaseURL,
    })
    // ... ReAct loop 消费 client.Stream(ctx, req) → iter.Seq[StreamEvent]
}
```

**注意**：chat 只 import `modeldomain`（接口），**不** import `modelapp`（Service struct）。main.go 把 `*modelapp.Service` 作为 `modeldomain.ModelPicker` 传进 chat 的构造函数。

---

## 15. 完整调用链（S5 "端到端推演先行"）

### 15.1 GET /api/v1/model-configs（列出）

```
前端 GET /api/v1/model-configs
  → middleware 链（Recover / Logger / CORS / InjectLocale / InjectUserID）
      → reqctx.SetUserID(ctx, "local-user")
  → mux 匹配 "GET /api/v1/model-configs"
  → ModelConfigHandler.List
      → svc.List(ctx)
          → repo.List(ctx)                 [infra/store/model]
              SELECT * FROM model_configs
              WHERE user_id = ? AND deleted_at IS NULL
              ORDER BY scenario
      → response.Success(200, items)       ← items 可能是 []，不是 nil
```

### 15.2 PUT /api/v1/model-configs/chat（upsert）

```
前端 PUT /api/v1/model-configs/chat  body={provider, modelId}
  → middleware 链（同上）
  → mux 匹配 "PUT /api/v1/model-configs/{scenario}"
  → ModelConfigHandler.Upsert
      → r.PathValue("scenario") → "chat"
      → decodeJSON → UpsertRequest{Provider, ModelID}
          畸形 → 400 INVALID_REQUEST
      → svc.Upsert(ctx, "chat", UpsertInput{...})
          → IsValidScenario("chat")?
              false → 400 INVALID_SCENARIO
          → TrimSpace(Provider) == ""?
              → 400 PROVIDER_REQUIRED
          → TrimSpace(ModelID) == ""?
              → 400 MODEL_ID_REQUIRED
          → reqctx.GetUserID → uid
          → repo.GetByScenario(ctx, "chat")
              ErrNotConfigured → 新建分支
              nil → 更新分支
          → repo.Upsert(ctx, m)            [infra/store/model]
          → log.Info("model config upserted")
      → response.Success(200, m)
```

### 15.3 chat.Send 调 PickForChat（跨 domain）

```
chat.Service.Send(ctx)
  → models.PickForChat(ctx)     【本 domain 对外入口】
      → repo.GetByScenario(ctx, "chat")
          用户从未配 → ErrNotConfigured → 向上抛
              → chat 的 handler → response.FromDomainError → 422 MODEL_NOT_CONFIGURED
      → return m.Provider, m.ModelID, nil
  → apikey.ResolveCredentials(ctx, provider) ...
```

---

## 16. 安全考虑

model domain 不涉及明文凭证，安全面比 apikey 小。唯一关注：

| 点 | 设计 |
|---|---|
| `user_id` 隔离 | Repository 方法全都 `WHERE user_id=?` 过滤（store 里强制）|
| `user_id` 响应不泄漏 | ModelConfig struct 里 `UserID` 有 `json:"-"` 标签（与 APIKey 保持一致）|
| nil logger | `NewService(..., nil)` panic；单测守护 |

---

## 17. 实现清单（✅ 已全部完成，2026-04-25）

> 注：文件命名遵循 S12 更新后的规范——所有层主文件统一用包名（`model.go`，不再叫 `service.go` / `store.go`）。

### domain 层 ✅
- [x] `internal/domain/model/model.go` — ModelConfig struct + TableName + 4 sentinel + ScenarioChat + IsValidScenario + ListScenarios + Repository（3 方法）+ ModelPicker（1 方法）
- [x] `internal/domain/model/model_test.go` — 3 个单测（valid/invalid 校验 + ListScenarios 一致性守卫）

### infra 层 ✅
- [x] `internal/infra/store/model/model.go` — Store 实现 Repository（GetByScenario / List / Upsert）
  - Upsert = GORM `Save()`（INSERT on new PK / UPDATE on existing PK）
  - Service 负责"查现有 → 决定 insert or update"逻辑，store 只做最终持久化
  - **partial UNIQUE 暂缓**：GORM 全索引在当前 Upsert 模式下等价（无 delete+recreate 路径）
- [x] `internal/infra/store/model/model_test.go` — 9 个集成测试（CRUD / 跨用户隔离 / 唯一约束守卫）

### app 层 ✅
- [x] `internal/app/model/model.go` — Service（List / Upsert + 校验 + ID 生成 + PickForChat + nil logger 守护）
  - **`modelpicker.go` 取消**：PickForChat 3 行，合并入 `model.go`；`var _ modeldomain.ModelPicker = (*Service)(nil)` 守护保留
- [x] `internal/app/model/model_test.go` — 12 个单测（fake repo）

### transport 层 ✅
- [x] `internal/transport/httpapi/handlers/model.go` — ModelConfigHandler + GET + PUT + Register
- [x] `internal/transport/httpapi/handlers/model_test.go` — 7 个 E2E 契约测试（真 SQLite + Service + InjectUserID）

### 配套基础设施 ✅
- [x] `internal/transport/httpapi/response/errmap.go` — 4 条 model sentinel 映射
- [x] `internal/transport/httpapi/router/deps.go` — `ModelService *modelapp.Service` 字段
- [x] `internal/transport/httpapi/router/router.go` — 条件注册（nil-tolerant）
- [x] `cmd/server/main.go` — `modelstore.New(gdb)` → `modelapp.NewService(...)` → `router.Deps`；`db.Migrate` 追加 `&modeldomain.ModelConfig{}`

### 验收 ✅
- [x] 全仓 `go test -count=1 -race ./...` 零失败
- [x] `go vet ./...` 零警告
- [x] `go build ./...` 通过
- [x] curl 冒烟全通：GET 空 / PUT chat → 200 / GET 验 1 条 / PUT 覆盖 anthropic / GET 验 provider 变 / PUT workflow_llm → 400 INVALID_SCENARIO

---

## 18. 遗留 / 未来

### 设计决定（已落定）

- **软删保留**：ModelConfig 保持 `gorm.DeletedAt` 软删，与 D1 规范一致。
- **Upsert 方式**：Service 先 `GetByScenario` 判断存在性，再调 `repo.Upsert(m)`（GORM `Save()`）。比 `OnConflict` 更可控，Service 层显式决定 insert vs update 路径。
- **partial UNIQUE 暂缓**：当前 Upsert 模式无 "delete+recreate 同一 scenario" 路径，GORM 全索引已足够。待有删除场景时在 `schema_extras.go` 追加。

### backlog

- `PickForWorkflow` / `PickForEmbedding` / `PickForIntent` 方法：Phase 4/5 按需加到 ModelPicker 接口
- `Pick(ctx, scenario)` 通用方法：**不做**（类型安全 > DRY，见 §6.3 理由）
- GET /{scenario} 单条接口：Phase 2 不做，scenario 多了再加
- DELETE 接口：暂不做，用户直接 PUT 新值即可

- **一键自动配置** `POST /api/v1/model-configs:auto-configure`：按每个 scenario 的 provider 偏好列表，匹配用户已有的活跃 key，批量 Upsert。触发时机：新增/更新 key 后提示用户、key 失效后提示重新跑。**Phase 5 全部 scenario 定义完后 revisit**。

---

## 19. 与其他 domain 的协作图

```
         ┌─────────────────────────────┐
         │  chat / workflow / knowledge │   ← 消费方（只见 ModelPicker 接口）
         │  (Phase 2-5 陆续实现)        │
         └──────────┬──────────────────┘
                    │ PickForChat(ctx) → (provider, modelID)
                    │ [Phase 4+ 再加 PickForWorkflow / PickForEmbedding]
                    ↓
            ┌──────────────────┐
            │  model.Service    │ ← ModelPicker 唯一实现
            └───┬──────────────┘
                │ GetByScenario / List / Upsert
                ↓
            ┌──────────────────┐
            │  Repository 实现  │ ← infra/store/model.Store
            └──────────────────┘

model domain **不依赖** apikey，反之亦然 —— 它俩通过 chat 这条消费链横向串起来。
```
