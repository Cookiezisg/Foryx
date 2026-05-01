# apikey domain — 详细设计文档

**所属 Phase**：Phase 2（基础对话能力，第 1 个完成的 domain）
**状态**：✅ 已实现（2026-04-24 全部 7 步完成 + 100+ 测试全绿 + curl 冒烟通过）
**职责**：管理用户的 LLM provider API Key 凭证（存储、加密、测试连通性、按 provider 解析）
**依赖**：
- `domain/crypto`（加密接口）+ `infra/crypto.AESGCMEncryptor`（AES-GCM 实现，`v1:` 前缀密文）
- `infra/db`（GORM 底层）+ `pkg/reqctx`（userID ctx 读取）
- `net/http`（tester 发探测请求；hand-rolled HTTP，理由见 §9 设计说明）

**被依赖**：`chat` / `model` / 工作流 LLM 节点 / 知识库 embedding 等所有需要调 LLM 的地方，**全部通过 `apikeydomain.KeyProvider` 接口**，不直接接触 Service struct 或 Repository。

**关联文档**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/api-design.md`](../service-contract-documents/api-design.md) — API 索引
- [`../service-contract-documents/database-design.md`](../service-contract-documents/database-design.md) — 表索引
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — 错误码索引

---

## 1. 为什么要这个 domain

所有 LLM 调用都需要凭证。用户使用 Forgify 前必须配一个或多个 API Key（OpenAI / Anthropic / DeepSeek / Ollama / ...）。

本 domain 负责：

1. **安全存储** Key（加密 + `v1:` 前缀 + `key_masked` 冗余展示值）
2. **列出** Key 给用户（分页，带展示掩码）
3. **测试** Key 能不能用（连通性测试，4 种 HTTP 模式 + custom 按 APIFormat 二选一）
4. **提供** Key 给其他 domain 消费（跨 domain 接口 `KeyProvider`；消费方看不到密文/Repository/Service struct）

---

## 2. 核心决策（已敲定）

| 决策 | 选择 | 理由 |
|---|---|---|
| apikey vs model 是否分 domain | **分离** | apikey 管凭证、model 管"哪个场景用哪个模型"策略，职责清晰 |
| 多租户 user_id | **从 V1 就引入** | 每表带 `user_id`，Phase 2 暂时硬编码 `local-user`；未来加 auth 只需改 middleware |
| Provider 列表 | **硬编码白名单** | 11 个；新 provider 需要代码改动（适配 base_url / 测试逻辑） |
| base_url 校验 | **app 层（Service）** | 不在 DB 层 CHECK（scenario 白名单同理，保灵活性）|
| Key 失效反馈 | **`test_status=error` 标记**（Phase 2）+ 未来可加事件 | 现阶段足够；chat 时流式响应可推 `chat.error` |
| Tester 实现方式 | **hand-rolled HTTP**（`net/http`）| 探测要最便宜（不付 LLM token 费用）；要区分 401/网络/5xx 等错误；Anthropic 1-token ping / Google query-string key / Ollama 无 auth 各有特殊性，单一抽象会丢信息 |
| 加密算法 | **AES-GCM + 机器指纹派生密钥**；密文 `v1:` 前缀 | 未来换 KMS 信封加密时可共存 |

---

## 3. 多租户准备（跨 domain 约定）

> ⚠️ **项目级约定**，不只 apikey。其他 domain 照此办理。

### 设计

- 每张业务表都有 `user_id TEXT NOT NULL`
- Phase 2 单用户阶段：middleware `InjectUserID` 把 `reqctx.DefaultLocalUserID = "local-user"` 塞入 ctx
- 未来加 auth：替换 middleware 解析 JWT / session，业务代码零改

### 实现位置

| 层 | 文件 | 做什么 |
|---|---|---|
| middleware | `internal/transport/httpapi/middleware/auth.go` | `InjectUserID(next)` 把 `local-user` 塞 ctx |
| 共享包 | `internal/pkg/reqctx/userid.go` | `SetUserID(ctx, id)` / `GetUserID(ctx) (string, bool)` |
| store | `internal/infra/store/apikey/store.go` | 每个方法 `userID(ctx)` 取值；缺失 = 接线 bug |

---

## 4. Provider 白名单（11 个）

代码位置：`internal/domain/apikey/providers.go`

| Provider | 分类 | base_url 必填 | 默认 base_url | TestMethod 枚举 |
|---|---|---|---|---|
| `openai` | 国际 | 否 | `https://api.openai.com/v1` | `TestMethodGetModels` |
| `anthropic` | 国际 | 否 | `https://api.anthropic.com` | `TestMethodAnthropicPing` |
| `google` | 国际 | 否 | `https://generativelanguage.googleapis.com` | `TestMethodGoogleListModels` |
| `deepseek` | 国产/OpenAI 兼容 | 否 | `https://api.deepseek.com` | `TestMethodGetModels` |
| `openrouter` | 国际聚合 | 否 | `https://openrouter.ai/api/v1` | `TestMethodGetModels` |
| `qwen` | 国产（阿里）| 否 | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `TestMethodGetModels` |
| `zhipu` | 国产（智谱）| 否 | `https://open.bigmodel.cn/api/paas/v4` | `TestMethodGetModels` |
| `moonshot` | 国产（Kimi）| 否 | `https://api.moonshot.cn/v1` | `TestMethodGetModels` |
| `doubao` | 国产（字节）| 否 | `https://ark.cn-beijing.volces.com/api/v3` | `TestMethodGetModels` |
| `ollama` | 本地 | **✅ 必填** | — | `TestMethodOllamaTags` |
| `custom` | 兜底 | **✅ 必填** | — | `TestMethodCustom`（测试时按 APIFormat 二选一）|

### ProviderMeta（实际代码）

```go
// domain/apikey/providers.go

type TestMethod string

const (
    TestMethodGetModels        TestMethod = "get_models"          // GET {baseURL}/models, Bearer auth
    TestMethodAnthropicPing    TestMethod = "anthropic_ping"      // POST {baseURL}/v1/messages, 1-token
    TestMethodGoogleListModels TestMethod = "google_list_models"  // GET {baseURL}/v1beta/models?key=
    TestMethodOllamaTags       TestMethod = "ollama_tags"         // GET {baseURL}/api/tags, no auth
    TestMethodCustom           TestMethod = "custom"              // dispatch by APIFormat at test time
)

type ProviderMeta struct {
    Name            string
    DisplayName     string
    DefaultBaseURL  string
    BaseURLRequired bool
    TestMethod      TestMethod
}

// 外部用的工具函数
func GetProviderMeta(name string) (ProviderMeta, bool)
func IsValidProvider(name string) bool
func ListProviders() []string
```

### `custom` provider 补充规则

`provider='custom'` 时必填：
- `base_url`
- `api_format`：`openai-compatible` / `anthropic-compatible`

测试时 HTTPTester 按 `api_format` 分派：`anthropic-compatible` → anthropic_ping；其他（含空）→ get_models。

---

## 5. 领域模型

### APIKey struct（`internal/domain/apikey/apikey.go`，实际代码）

```go
type APIKey struct {
    ID           string         `gorm:"primaryKey;type:text" json:"id"`
    UserID       string         `gorm:"not null;index:idx_api_keys_user_id;type:text" json:"userId"`
    Provider     string         `gorm:"not null;index:idx_api_keys_user_provider,priority:2;type:text" json:"provider"`
    DisplayName  string         `gorm:"not null;type:text;default:''" json:"displayName"`
    KeyEncrypted string         `gorm:"not null;type:text" json:"-"`              // 线上永不出现
    KeyMasked    string         `gorm:"not null;type:text" json:"keyMasked"`
    BaseURL      string         `gorm:"type:text;default:''" json:"baseUrl"`
    APIFormat    string         `gorm:"type:text;default:''" json:"apiFormat"`
    TestStatus   string         `gorm:"type:text;default:'pending'" json:"testStatus"`
    TestError    string         `gorm:"type:text;default:''" json:"testError"`
    LastTestedAt *time.Time     `json:"lastTestedAt"`
    CreatedAt    time.Time      `json:"createdAt"`
    UpdatedAt    time.Time      `json:"updatedAt"`
    DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

func (APIKey) TableName() string { return "api_keys" }
```

**字段说明**：

| 字段 | 说明 |
|---|---|
| `ID` | `aki_<16hex>` 格式（8 字节 crypto/rand） |
| `UserID` | Phase 2 固定 `"local-user"`；未来真 auth 时替换 |
| `Provider` | 11 白名单之一 |
| `DisplayName` | 用户自定义别名；app 层 `strings.TrimSpace` |
| `KeyEncrypted` | `v1:` 前缀 + base64(AES-GCM seal)；线上响应有 `json:"-"` tag，永不泄漏 |
| `KeyMasked` | 如 `sk-proj...abc4`；列表查询不必解密 |
| `BaseURL` | 空 = 用 provider 默认；app 层 `strings.TrimSpace` |
| `APIFormat` | 仅 custom 必填（`openai-compatible` / `anthropic-compatible`）|
| `TestStatus` | `pending` / `ok` / `error`，由 Service.Test 或 MarkInvalid 写 |
| `TestError` | 测试失败时的 human-readable 原因；成功时清空 |
| `LastTestedAt` | UTC；`UpdateTestResult` 每次调用都写 |
| `ModelsFound` | `[]string`，GORM `serializer:json` 存为 JSON 字符串；测试成功时写入 provider 返回的模型列表，测试失败或 MarkInvalid 时保持不变（传 nil 跳过更新）；API 响应序列化为 JSON 数组 |

### 常量（`internal/domain/apikey/apikey.go`，与 entity 同文件）

```go
const (
    TestStatusPending = "pending"
    TestStatusOK      = "ok"
    TestStatusError   = "error"

    APIFormatOpenAICompatible    = "openai-compatible"
    APIFormatAnthropicCompatible = "anthropic-compatible"
)
```

### Sentinel 错误（**8 个**，实际代码）

```go
// domain/apikey/apikey.go
var (
    ErrNotFound            = errors.New("apikey: not found")
    ErrNotFoundForProvider = errors.New("apikey: no key for provider")
    ErrInvalidProvider     = errors.New("apikey: invalid provider")
    ErrBaseURLRequired     = errors.New("apikey: base_url required for this provider")
    ErrAPIFormatRequired   = errors.New("apikey: api_format required for custom provider")
    ErrKeyRequired         = errors.New("apikey: key value is required")
    ErrTestFailed          = errors.New("apikey: connectivity test failed")
    ErrInvalid             = errors.New("apikey: key rejected by provider")
)
```

各 sentinel → HTTP 映射见 §14 错误码 + `service-contract-documents/error-codes.md`。

---

## 6. 对外 API vs 对内函数（速查表）

### 6.1 对外两类消费者

| 消费者 | 接口 | 位置 | 方法数 |
|---|---|---|---|
| 🌐 **前端 / curl** | HTTP REST API | `/api/v1/api-keys/*` | **5 个端点** |
| 🧩 **其他 domain**（chat / workflow / knowledge / embedding）| `apikeydomain.KeyProvider` 接口 | `internal/domain/apikey/apikey.go` | **2 个方法** |

### 6.2 HTTP REST（详见 §10）

```
POST   /api/v1/api-keys              创建（201）
GET    /api/v1/api-keys              列表（200，分页 + ?provider= 过滤）
PATCH  /api/v1/api-keys/{id}         更新 displayName / baseUrl（200）
DELETE /api/v1/api-keys/{id}         软删（204）
POST   /api/v1/api-keys/{id}:test    连通性测试（200 或 422）
```

响应**绝不**含解密后的明文 key（`json:"-"` 守护）。

### 6.3 `KeyProvider` 接口（跨 domain 唯一入口）

```go
// domain/apikey/apikey.go

type KeyProvider interface {
    ResolveCredentials(ctx context.Context, provider string) (Credentials, error)
    MarkInvalid(ctx context.Context, provider string, reason string) error
}

type Credentials struct {
    Key     string // 明文；调用方禁止 log、禁止传跨请求 goroutine
    BaseURL string // 已合并 provider 默认
}
```

实现：`app/apikey.Service`（`internal/app/apikey/keyprovider.go` 里有 `var _ apikeydomain.KeyProvider = (*Service)(nil)` 编译期守护）。

### 6.4 对内类型速查

| 类别 | 名字 | 位置 | 谁用 |
|---|---|---|---|
| Repository 接口 | `Repository` | `domain/apikey/apikey.go` | Service 调；其他 domain **不许** import |
| Repository 实现 | `Store` | `infra/store/apikey/store.go`（包名 apikey，调用方别名 apikeystore）| main.go DI |
| Service（CRUD + Test 编排） | `Service` | `app/apikey/service.go`（别名 apikeyapp）| handler + main.go |
| KeyProvider 实现 | 同 `Service` | `app/apikey/keyprovider.go` | 其他 domain（通过接口） |
| ConnectivityTester 接口 | `ConnectivityTester` | `app/apikey/tester.go` | Service.Test 内部 |
| ConnectivityTester 实现 | `HTTPTester` | `app/apikey/tester.go`（同文件）| main.go DI |
| MaskKey | `MaskKey(string) string` | `app/apikey/mask.go` | Service.Create 生成展示值 |
| ProviderMeta | `ProviderMeta`, `GetProviderMeta`, `IsValidProvider`, `ListProviders` | `domain/apikey/providers.go` | Service 校验 + Tester 分派 |

**关键界线**：
- `Repository` 是 **apikey 内部**契约（Service ↔ DB）—— 其他 domain 不该 import
- `KeyProvider` 是 **apikey 对外**契约（给 chat / workflow 等）—— 这才是跨 domain 入口
- 两个接口**都在 `domain/apikey/apikey.go`**，通过包级 godoc 区分用途

---

## 7. Repository 接口（实际代码）

```go
// domain/apikey/apikey.go
type Repository interface {
    // Get fetches a single APIKey by id, scoped to userID in ctx.
    // Returns ErrNotFound if no live row matches.
    Get(ctx context.Context, id string) (*APIKey, error)

    // List returns a page of caller's keys. Returns (rows, nextCursor, err).
    List(ctx context.Context, filter ListFilter) ([]*APIKey, string, error)

    // GetByProvider picks the most suitable live APIKey for (user, provider).
    // Ordering: test_status=ok 优先 → last_tested_at DESC → created_at DESC.
    // Returns ErrNotFoundForProvider if none.
    GetByProvider(ctx context.Context, provider string) (*APIKey, error)

    // Save upserts on primary key. Caller must have set k.UserID before calling.
    Save(ctx context.Context, k *APIKey) error

    // Delete soft-deletes by id, scoped to caller.
    Delete(ctx context.Context, id string) error

    // UpdateTestResult writes test_status / test_error / last_tested_at / models_found.
    // Pass nil for models when no model list is available (e.g. MarkInvalid).
    UpdateTestResult(ctx context.Context, id, status, errMsg string, models []string) error
}

type ListFilter struct {
    Cursor   string
    Limit    int
    Provider string  // 可选 provider 过滤
}
```

### Store 实现（`infra/store/apikey/store.go`）

- GORM 实现，每个方法前 `reqctx.GetUserID(ctx)` 取 userID，缺失返 `fmt.Errorf("apikeystore: missing user id in context")`（接线 bug，不是 401）
- `List` 用 `(created_at, id)` 元组 cursor 做稳定分页（时间戳相同也稳定）
- `GetByProvider` 排序 SQL：`CASE WHEN test_status = 'ok' THEN 0 ELSE 1 END` + `last_tested_at DESC` + `created_at DESC`
- 软删走 GORM 内置的 `DeletedAt` 字段
- `UpdateTestResult` 只写 4 列（`test_status` / `test_error` / `last_tested_at` / `models_found`），避免全表往返；`models_found` 用 `json.Marshal(models)` 序列化后存字符串

---

## 8. Service 层（`app/apikey/service.go`，实际代码）

### Struct

```go
type Service struct {
    repo      apikeydomain.Repository
    encryptor crypto.Encryptor     // domain/crypto 接口
    tester    ConnectivityTester   // apikeyapp 内部接口
    log       *zap.Logger
}

func NewService(repo apikeydomain.Repository, enc crypto.Encryptor, tester ConnectivityTester, log *zap.Logger) *Service {
    if log == nil {
        panic("apikey.NewService: logger is nil")
    }
    return &Service{repo: repo, encryptor: enc, tester: tester, log: log}
}
```

**nil logger 立刻 panic** —— 接线 bug 不应在生产 log 处才炸，有 `TestNewService_NilLogger_Panics` 守护单测。

### Inputs

```go
type CreateInput struct {
    Provider    string
    DisplayName string
    Key         string
    BaseURL     string
    APIFormat   string
}

// UpdateInput: nil 字段不改；非 nil 指向 "" 则清空该值。
// 故意不含 Key / Provider / APIFormat —— 改这些要 delete + recreate。
type UpdateInput struct {
    DisplayName *string
    BaseURL     *string
}
```

### 方法签名（6 个公开 + 2 个 KeyProvider 实现）

```go
func (s *Service) Create(ctx, in CreateInput)           (*apikeydomain.APIKey, error)
func (s *Service) Update(ctx, id string, in UpdateInput) (*apikeydomain.APIKey, error)
func (s *Service) Delete(ctx, id string)                 error
func (s *Service) Get(ctx, id string)                   (*apikeydomain.APIKey, error)
func (s *Service) List(ctx, filter apikeydomain.ListFilter) ([]*apikeydomain.APIKey, string, error)
func (s *Service) Test(ctx, id string)                  (*TestResult, error)

// KeyProvider 接口实现（在 keyprovider.go）
func (s *Service) ResolveCredentials(ctx, provider string) (apikeydomain.Credentials, error)
func (s *Service) MarkInvalid(ctx, provider, reason string) error
```

### Create 流程

```
1. validateCreate(in):
   - apikeydomain.IsValidProvider(provider) → ErrInvalidProvider
   - strings.TrimSpace(Key) == "" → ErrKeyRequired
   - meta.BaseURLRequired && BaseURL == "" → ErrBaseURLRequired
   - provider == "custom" && APIFormat == "" → ErrAPIFormatRequired
2. reqctx.GetUserID(ctx) → uid（缺失 = 接线 bug，上抛）
3. encryptor.Encrypt(Key) → ciphertext "v1:..."
4. newID() → "aki_<16hex>"
5. 拼 APIKey（TestStatus=pending）
6. repo.Save
7. log.Info("apikey created", key_id, user_id, provider)
```

### Test 流程（最复杂的方法）

```
1. repo.Get(id) → APIKey (含密文)
2. encryptor.Decrypt(KeyEncrypted) → 明文
3. tester.Test(Provider, 明文, BaseURL, APIFormat) → TestResult 或 err
4a. err != nil（程序 bug 路径）:
    repo.UpdateTestResult(id, TestStatusError, err.Error(), nil)
    上抛包装过的 err
4b. err == nil:
    status = TestStatusError; errMsg = result.Message; models = nil
    if result.OK: status = TestStatusOK; errMsg = ""; models = result.ModelsFound
    repo.UpdateTestResult(id, status, errMsg, models)
5. log.Info("apikey tested", key_id, provider, ok, latency_ms)
6. return result
```

**关键**：DB 写是 Service 的职责、不是 Tester 的。Tester 是无状态探针。

### ID 生成

```go
func newID() string {
    var b [8]byte
    if _, err := rand.Read(b[:]); err != nil {
        panic(fmt.Sprintf("apikey: crypto/rand failed: %v", err))
    }
    return "aki_" + hex.EncodeToString(b[:])
}
```

`crypto/rand` 失败意味着 OS RNG 坏 → panic 比拿不可信的 ID 继续跑更安全。

### MaskKey（`app/apikey/mask.go`）

```go
func MaskKey(key string) string
```

规则：
- 长度 < 8 → `"****"`（完全隐藏）
- 长度 8-20 → `前 3 + "..." + 后 4`
- 长度 > 20 → `前 7 + "..." + 后 4`

示例：
- `"sk-proj-abcdefg1234567890xyz"` → `"sk-proj...0xyz"`
- `"AKIA1234567890ABCDEF"` → `"AKI...CDEF"`
- `"short"` → `"****"`

回归守卫测试 `TestMaskKey_NeverLeaksMiddle` 确保掩码不含中间字节。

---

## 9. ConnectivityTester（`app/apikey/tester.go`）

### 接口 + TestResult

```go
type TestResult struct {
    OK          bool
    Message     string    // UI 可直接展示
    LatencyMs   int64     // 挂钟往返时间
    ModelsFound []string  // 仅 get_models / google_list_models / ollama_tags 填充
}

type ConnectivityTester interface {
    Test(ctx context.Context, provider, key, baseURL, apiFormat string) (*TestResult, error)
}
```

### 错误 vs TestResult 的约定（很重要）

| 情况 | 表现 |
|---|---|
| 网络不通 / DNS 失败 / ctx 取消 / 401 / 5xx / 上游返回格式错 | `*TestResult{OK:false, Message:...}`，err = nil |
| 未知 provider / 必填 baseURL 缺失（Service 应预先校验） | `nil, error`（包装 `ErrInvalidProvider` / `ErrBaseURLRequired`）|

**Service 端**看 err 决定是否上抛 500；看 `result.OK` 决定 `test_status = ok/error`。

### HTTPTester 实现

```go
type HTTPTester struct {
    client *http.Client  // 传 nil 装默认 10s Timeout
}

func NewHTTPTester(client *http.Client) *HTTPTester

var _ ConnectivityTester = (*HTTPTester)(nil)
```

`Test` 主分派按 `ProviderMeta.TestMethod` 走：

| TestMethod | 私有方法 | HTTP |
|---|---|---|
| `TestMethodGetModels` | `testGetModels` | `GET {baseURL}/models`，`Authorization: Bearer {key}` |
| `TestMethodAnthropicPing` | `testAnthropicPing` | `POST {baseURL}/v1/messages`，`x-api-key: {key}` + `anthropic-version: 2023-06-01`，body `{"model":"claude-3-5-haiku-latest","max_tokens":1,"messages":[{"role":"user","content":"hi"}]}`，**约 $0.0001/次** |
| `TestMethodGoogleListModels` | `testGoogleListModels` | `GET {baseURL}/v1beta/models?key={key}`，query 认证 |
| `TestMethodOllamaTags` | `testOllamaTags` | `GET {baseURL}/api/tags`，**无认证**（本地）|
| `TestMethodCustom` | - | `APIFormat == anthropic-compatible` → `testAnthropicPing`；其他（含空）→ `testGetModels` |

### baseURL 规范化

Tester 在分派前：`effective = strings.TrimRight(baseURL, "/")`；空则用 `meta.DefaultBaseURL`；仍空（ollama / custom）则返 `ErrBaseURLRequired`。用户传 `"http://foo.com/"` 不会造成 `//models`。

### 响应体读取

`do()` 共用 helper：`io.LimitReader(resp.Body, 64*1024)`，防止 provider 返回巨大 HTML 吃爆内存。

错误消息用 `formatHTTPError(status, body)`：`"HTTP {status}: {body 前 200 字节 trimmed}"`，UI 安全展示。

### 模型列表解析

| Provider | 解析形状 |
|---|---|
| OpenAI 兼容 | `{"data":[{"id":"..."}]}` |
| Google | `{"models":[{"name":"..."}]}` |
| Ollama | `{"models":[{"name":"..."}]}` |

JSON 畸形时 `parseXxx` 返 `nil`，**连通性仍报告成功**（只是不返模型列表）——因为 200 = provider 接受了 key，body 格式问题不影响"可用"判断。

### 为什么 Tester 不复用通用 LLM 客户端

`infra/llm` 是给 chat 调真实推理用的 LLM 流式客户端，会真发推理请求产生 token 费用。Tester 的目的是**最便宜**地验证"key 能用"：
- OpenAI 兼容：打 `GET /models`，0 token 成本
- Anthropic：1-token `messages` ping（不能避免，但成本最小）
- Google / Ollama：用各自的列表端点，认证语义都不同
- 错误分类：401（key 错）/ 网络错 / 5xx（provider 故障）/ 4xx（请求格式）必须区分清楚，给出有信息量的诊断；通用 LLM 客户端的错误抽象会丢这些差别

所以 Tester 走 `net/http` hand-rolled，与 `infra/llm` 是平行能力，互不依赖。

---

## 10. HTTP API 详细

### 通用约定

- 前缀：`/api/v1/api-keys`
- 中间件链：`Recover → RequestLogger → CORS → InjectLocale → InjectUserID → mux`
- 响应走 envelope（N1）
- 错误走 `response.FromDomainError(w, log, err)` 经 errmap.go 翻译；`:test` 非 OK 时 handler 直接 synthesize 422 envelope

### 端点清单（5 个）

#### 10.1 `POST /api/v1/api-keys` — 创建（201）

**Request**：
```json
{
  "provider": "openai",
  "displayName": "My OpenAI Main",
  "key": "sk-proj-xxxxxxxxxxxxxxxx",
  "baseUrl": "",
  "apiFormat": ""
}
```

**校验错误**（Service 层）：
- 400 `INVALID_PROVIDER` / `KEY_REQUIRED` / `BASE_URL_REQUIRED` / `API_FORMAT_REQUIRED`
- 400 `INVALID_REQUEST`（JSON 畸形，含未知字段 `DisallowUnknownFields`）

**Response 201**：完整的 APIKey 对象（**无 `keyEncrypted` 字段**，`json:"-"` 守护）

#### 10.2 `GET /api/v1/api-keys` — 列表（200）

**Query**：`?cursor=&limit=50&provider=openai`（`limit` 默认 50 最大 200）

**Response 200**：
```json
{
  "data": [ {...}, {...} ],
  "nextCursor": "<opaque base64url>",
  "hasMore": true
}
```

空列表返 `{"data": [], "hasMore": false}`（不是 null）。

#### 10.3 `PATCH /api/v1/api-keys/{id}` — 更新（200）

**Request**（nil 字段不改；非 nil 指向 `""` 清空）：
```json
{ "displayName": "...", "baseUrl": "..." }
```

**注意**：不支持改 key / provider / apiFormat（要改就 delete + recreate）。

**Response 200**：更新后的 APIKey。`updatedAt` 自动推进。

**404 `API_KEY_NOT_FOUND`**：id 不存在。

#### 10.4 `DELETE /api/v1/api-keys/{id}` — 软删（204）

无 body 响应。`deleted_at` 写入当前时间。再次 DELETE 返 404。

#### 10.5 `POST /api/v1/api-keys/{id}:test` — 连通性测试（200 / 422）

> 实现方式：路由注册 `POST /api/v1/api-keys/{idAction}` 通配符，handler `postOnID` 用 `strings.Cut(":", idAction)` 拆出 id 和 action，switch 到 `test`。未来加 `:rotate` 或 `:archive` 只改 switch case。

**Request**：无 body（从 DB 读已加密 key，解密后 test）。

**Response 200**（`result.OK == true`）：
```json
{
  "data": {
    "ok": true,
    "message": "connected, 45 models available",
    "latencyMs": 1203,
    "modelsFound": ["gpt-4o", "gpt-4-turbo", ...]
  }
}
```

`modelsFound` 永远非 nil（handler `if models == nil { models = []string{} }`）。

**Response 422**（`result.OK == false`，网络/401/5xx/ctx 取消）：
```json
{
  "error": {
    "code": "API_KEY_TEST_FAILED",
    "message": "HTTP 401: {...}",
    "details": { "latencyMs": 80 }
  }
}
```

**副作用**（Service.Test 负责）：
- `result.OK == true` → `test_status=ok`, `last_tested_at=now`, `test_error=''`
- `result.OK == false` → `test_status=error`, `test_error=result.Message`
- tester 返程序 bug err（未知 provider 等）→ `test_status=error`, `test_error=err.Error()`，上抛 500（理论不发生，Service 预先校验）

**404 `API_KEY_NOT_FOUND`**：id 不存在。

---

## 11. 数据库表

```sql
CREATE TABLE api_keys (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL,
    provider         TEXT NOT NULL,           -- CHECK 约束由 schema_extras 补
    display_name     TEXT NOT NULL DEFAULT '',
    key_encrypted    TEXT NOT NULL,
    key_masked       TEXT NOT NULL,
    base_url         TEXT NOT NULL DEFAULT '',
    api_format       TEXT NOT NULL DEFAULT '',
    test_status      TEXT NOT NULL DEFAULT 'pending',
    test_error       TEXT NOT NULL DEFAULT '',
    last_tested_at   DATETIME,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at       DATETIME
);

CREATE INDEX idx_api_keys_user_id       ON api_keys(user_id);
CREATE INDEX idx_api_keys_user_provider ON api_keys(user_id, provider);
CREATE INDEX idx_api_keys_deleted_at    ON api_keys(deleted_at);
```

**索引理由**：
- `user_id` 单索引：列表查询最常用（"当前用户的所有 key"）
- `(user_id, provider)` 复合：chat 调 `GetByProvider` 时用
- `deleted_at` 单索引：GORM 软删 filter 用

**CHECK 约束**：Phase 2 目前**没有**给 `provider` 列加 CHECK 约束（应由 `schema_extras.go` 补；当前是通过 app 层 `IsValidProvider` 校验）。同样 `test_status` 的 CHECK 约束也在 app 层。若未来要加 DB 层保险，写入 `schema_extras.go` 的 raw SQL。

**迁移**：`db.Migrate(gdb, &apikeydomain.APIKey{})`（`cmd/server/main.go` 已接）。

**类型策略**：domain struct 直接带 GORM tag（一份到底），store 层不做 entity↔row 转换。

---

## 12. 事件

**Phase 2 不推送事件**。

未来可能加（Phase 3+，未决定）：
- `apikey.test_failed`：主动推测试失败给前端（不用刷新列表）
- `apikey.invalidated`：chat 调 key 返 401 时 push（UI 标红）

现阶段通过 `test_status=error` 列 + 下次拉取时前端显示即可；暂不加事件复杂度。

---

## 13. 错误码（8 个 sentinel，全已实现 ✅）

| Code | HTTP | Sentinel | 场景 |
|---|---|---|---|
| `API_KEY_NOT_FOUND` | 404 | `apikey.ErrNotFound` | Get/Delete/Update/Test id 不存在 |
| `API_KEY_PROVIDER_NOT_FOUND` | 404 | `apikey.ErrNotFoundForProvider` | `ResolveCredentials(ctx, provider)` 当前用户无活跃 key |
| `INVALID_PROVIDER` | 400 | `apikey.ErrInvalidProvider` | 创建时 provider 不在 11 白名单 |
| `BASE_URL_REQUIRED` | 400 | `apikey.ErrBaseURLRequired` | ollama / custom 没填 baseURL |
| `API_FORMAT_REQUIRED` | 400 | `apikey.ErrAPIFormatRequired` | custom 没填 apiFormat |
| `KEY_REQUIRED` | 400 | `apikey.ErrKeyRequired` | 创建时 key 空 |
| `API_KEY_TEST_FAILED` | 422 | `apikey.ErrTestFailed` | （**handler 直接 synthesize，不经 errmap**）连通性失败 |
| `API_KEY_INVALID` | 401 | `apikey.ErrInvalid` | chat 等消费方用 key 返 401 时 |

映射位置：`internal/transport/httpapi/response/errmap.go` 的 `errTable`。

---

## 14. 消费方如何用（跨 domain 示例）

### chat domain 调 LLM 时

```go
// internal/app/chat/chat.go
type Service struct {
    modelPicker modeldomain.ModelPicker          // 只见接口
    keyProvider apikeydomain.KeyProvider         // 只见接口
    llmFactory  *llminfra.Factory                // 自有 LLM 流式客户端工厂
    // ...
}

func (s *Service) processTask(...) {
    // 1. model 决定 (provider, modelID)
    provider, modelID, err := s.modelPicker.PickForChat(ctx)
    if err != nil { /* MODEL_NOT_CONFIGURED → SSE chat.error */ }

    // 2. apikey 拿凭证
    creds, err := s.keyProvider.ResolveCredentials(ctx, provider)
    if err != nil { /* API_KEY_PROVIDER_NOT_FOUND → SSE chat.error */ }

    // 3. 构造 Client + 调 LLM 流
    client, _, err := s.llmFactory.Build(llminfra.Config{
        Provider: provider, ModelID: modelID,
        Key: creds.Key, BaseURL: creds.BaseURL,
    })
    // ... ReAct loop 消费 client.Stream(ctx, req)

    // 4. 401 → 回报失效
    if isAuthError(streamErr) {
        _ = s.keyProvider.MarkInvalid(ctx, provider, streamErr.Error())
        // → SSE chat.error LLM_PROVIDER_ERROR
    }
}
```

**关键约定**：chat 只 import `apikeydomain`（**domain 层接口**），**不** import `apikeyapp`。main.go 把 `*apikeyapp.Service` 作为 `apikeydomain.KeyProvider` 接口传进 chat 的构造函数。chat 看不到 Service struct 也看不到 Repository。

---

## 15. 完整调用链（S5 "端到端推演先行"）

### 15.1 POST /api/v1/api-keys（创建）

```
前端 fetch POST /api/v1/api-keys
  → Recover / RequestLogger / CORS / InjectLocale / InjectUserID middleware
      → reqctx.SetUserID(ctx, "local-user")
  → mux 匹配 "POST /api/v1/api-keys"
  → APIKeyHandler.Create
      → decodeJSON(body, &createRequest)
          畸形 → response.FromDomainError(derrors.ErrInvalidRequest) → 400
      → svc.Create(ctx, CreateInput{...})
          → validateCreate：白名单 / 非空 / baseURL / apiFormat
              任一失败 → 400（errmap 翻译）
          → reqctx.GetUserID(ctx)
          → encryptor.Encrypt(key) → "v1:base64(nonce+ct+tag)"
          → newID() → "aki_<16hex>"
          → repo.Save(ctx, k)                              [infra/store/apikey]
          → log.Info("apikey created")
      → response.Created(w, k) → 201 envelope
```

### 15.2 POST /api/v1/api-keys/{id}:test（连通性测试）

```
前端 POST /api/v1/api-keys/aki_xxx:test
  → middleware 链（同上）
  → mux 匹配 "POST /api/v1/api-keys/{idAction}"
  → APIKeyHandler.postOnID
      → strings.Cut(idAction, ":") → (id="aki_xxx", action="test", found=true)
      → switch action {"test": h.test(...)}
  → APIKeyHandler.test
      → svc.Test(ctx, id)
          → repo.Get(ctx, id)                              [infra/store/apikey]
              未命中 → ErrNotFound → 404 API_KEY_NOT_FOUND
          → encryptor.Decrypt(KeyEncrypted) → 明文
              密文损坏 → 上抛 500
          → tester.Test(provider, 明文, baseURL, apiFormat)  [app/apikey.HTTPTester]
              按 ProviderMeta.TestMethod 分派（§9）
              → HTTP 打上游 provider
                  成功 200 → TestResult{OK:true, Message, LatencyMs, ModelsFound}
                  失败 / 401 / 5xx / 网络错 → TestResult{OK:false, Message, LatencyMs}
                  未知 provider（不该发生）→ error
          → 根据 result.OK 决定 status
          → repo.UpdateTestResult(id, status, errMsg, models)  // models=ModelsFound(成功) | nil(失败)
          → log.Info("apikey tested")
      → handler 看 result.OK：
          true  → response.Success(200, {ok, message, latencyMs, modelsFound})
          false → response.Error(422, "API_KEY_TEST_FAILED", message, {latencyMs})
```

### 15.3 其他 domain 调 apikey（chat 的真实路径）

```
chat.Service.processTask
  → modelPicker.PickForChat(ctx)             → (provider, modelID)
  → keyProvider.ResolveCredentials(ctx, provider) 【本 domain 的对外入口】
      → repo.GetByProvider(ctx, provider)
          排序：test_status=ok 优先 → last_tested_at DESC → created_at DESC
          无 → ErrNotFoundForProvider → SSE chat.error API_KEY_PROVIDER_NOT_FOUND
      → encryptor.Decrypt(KeyEncrypted)
      → 合并 baseURL（用户填的 | meta.DefaultBaseURL）
      → 返回 Credentials{Key, BaseURL}
  → llmFactory.Build(llminfra.Config{...})  → llminfra.Client
  → client.Stream(ctx, req) 消费 iter.Seq[StreamEvent]
  → 401 分支：
      → keyProvider.MarkInvalid(ctx, provider, reason)
          → repo.GetByProvider(ctx, provider)
          → repo.UpdateTestResult(k.ID, TestStatusError, reason, nil)
          → log.Warn("apikey marked invalid")
      → SSE chat.error LLM_PROVIDER_ERROR
```

---

## 16. 安全考虑

| 点 | 设计 |
|---|---|
| 加密 | AES-GCM + 机器指纹派生密钥（`infra/crypto.MachineFingerprint → DeriveKey → AESGCMEncryptor`）|
| 密文版本 | `v1:` 前缀；未来 KMS 用 `v2:`，新旧共存 |
| 明文 Key 不落日志 | Service/handler/store 绝不 log key 内容；`log.Info` 只记 key_id / provider / user_id |
| 明文 Key 不落响应 | `KeyEncrypted` 带 `json:"-"`；契约回归测试 `TestAPIKeyHandler_Create_Success` 断言 `keyEncrypted` 字段**不存在**于响应 |
| 明文 Key 生命周期短 | 只在 Service.Test / ResolveCredentials 瞬间出现在内存，函数返回即 GC |
| DB 丢失 | `key_masked` 冗余展示值保底；key 本身不可恢复，用户需重填 |
| 软删保留审计 | 30 天物理清理**未实现**（见 §遗留 backlog）|
| nil logger 守护 | `NewService(..., nil)` 立刻 panic；有单测锁 |

---

## 17. 实现清单（全部完成 ✅）

> 文件布局遵循 `backend-design.md` S12（domain 平铺按概念拆文件）+ S13（三层同名包 + 调用方 `<name><role>` 别名）。
> 三个 apikey 包都声明 `package apikey`；调用方 import 别名为 `apikeydomain` / `apikeyapp` / `apikeystore`。

### domain 层 ✅
- [x] `internal/domain/apikey/apikey.go` — APIKey + TableName + 常量 + Credentials + ListFilter + **8 个** sentinel + Repository + KeyProvider 接口（**合并在同一文件**）
- [x] `internal/domain/apikey/providers.go` — TestMethod 枚举（5 个）+ ProviderMeta + 11 providers 白名单 + `GetProviderMeta` / `IsValidProvider` / `ListProviders`
- [x] `internal/domain/apikey/providers_test.go` — 5 个白名单完整性测试

### infra 层 ✅
- [x] `internal/infra/db/{db,migrate,schema_extras}.go` — 通用 GORM 底层（Phase 1 已就绪）
- [x] `internal/infra/store/apikey/store.go` — Store 实现 Repository（含 cursor 分页 + GetByProvider 排序）
- [x] `internal/infra/store/apikey/store_test.go` — 18 个集成测试（CRUD + 跨用户隔离 + 分页 + GetByProvider 排序）

### app 层 ✅
- [x] `internal/app/apikey/mask.go` + `mask_test.go` — MaskKey + 9 用例（含"不泄漏中间字节"回归守卫）
- [x] `internal/app/apikey/tester.go` — ConnectivityTester 接口 + HTTPTester + 4 私有方法 + 5 TestMethod 分派 + custom APIFormat 二选一
- [x] `internal/app/apikey/tester_test.go` — 21 个 httptest 用例（成功/401/5xx/网络错/ctx 取消/malformed JSON/trailing slash/query escape/默认 baseURL）
- [x] `internal/app/apikey/service.go` — Service（Create/Update/Delete/Get/List/Test + 校验 + ID 生成 + 加密编排 + nil logger 守护）
- [x] `internal/app/apikey/keyprovider.go` — Service 实现 `apikeydomain.KeyProvider` + 编译期 `var _` 守护
- [x] `internal/app/apikey/service_test.go` — 18 个单测（真 AES-GCM + fake repo + fake tester）

### transport 层 ✅
- [x] `internal/transport/httpapi/handlers/apikey.go` — 5 端点 + `POST /{idAction}` + `strings.Cut(":")` 拆分
- [x] `internal/transport/httpapi/handlers/apikey_test.go` — 15 个 E2E 契约测试（真 SQLite + 真 AES-GCM + fake tester + InjectUserID）

### 配套基础设施 ✅
- [x] `internal/transport/httpapi/middleware/auth.go` — InjectUserID（Phase 1）
- [x] `internal/pkg/reqctx/userid.go` — ctx 读写（Phase 1）
- [x] `internal/transport/httpapi/response/errmap.go` — 7 apikey sentinel 映射
- [x] `internal/transport/httpapi/router/router.go` + `deps.go` — 条件注册 + nil-tolerant
- [x] `cmd/server/main.go` — `MachineFingerprint → DeriveKey → AES-GCM → Store → HTTPTester → Service` 装配；`db.Migrate(gdb, &apikeydomain.APIKey{})`

### 验收 ✅
- [x] 全仓 `go test ./...` 零失败（apikey 相关 61 测试 + 其他 40+）
- [x] `go vet ./...` 零警告
- [x] `go build ./...` 通过
- [x] `-race` 检测通过
- [x] 4/5 端点 curl 冒烟通过（Create 201 / List paged / Patch 200 / Delete 204 / GET after delete = `[]`）
- [ ] `:test` 端点真实 provider 验证（留给用户用真 key 跑，目前 httptest 全覆盖）

---

## 18. 遗留 / 未来可能补的东西

已解决（立项时的待确认，现在已定）：
- ✅ Provider 默认 base_url 全部按官方文档核对，httptest 覆盖 5 种 TestMethod 模式
- ✅ Anthropic 测试费用约 $0.0001/次接受；无真实调用不产生
- ✅ GetByProvider 排序：`test_status=ok 优先 → last_tested_at DESC → created_at DESC`（store + 集成测试守护）

backlog（未来按需做）：
- **软删 30 天物理清理**：单用户场景不急；多租户后加定时任务
- **单用户 Key 数量上限**：目前无限制；真出现恶意场景可加 100 条 soft limit + 告警
- **provider 列 DB 层 CHECK 约束**：app 层已校验；补 DB 层是防御深度，需写 `schema_extras.go`
- **Events push**：`apikey.test_failed` / `apikey.invalidated` —— 用到再加
- **CHECK(test_status IN ('pending', 'ok', 'error'))**：同上，app 层已保

---

## 19. 与其他 domain 的协作图

```
     ┌─────────────────────────────┐
     │  chat / workflow / knowledge │   ← 消费方（通过 KeyProvider 接口）
     │  Phase 2-5 陆续实现           │
     └──────────┬──────────────────┘
                │ ResolveCredentials(provider) → Credentials
                │ MarkInvalid(provider, reason)  on 401
                ↓
        ┌──────────────────┐
        │  apikey.Service   │ ← KeyProvider 的唯一实现（app/apikey/keyprovider.go）
        └───┬──────────────┘
            │ Encrypt / Decrypt
            ↓
        ┌──────────────────┐
        │  crypto.Encryptor │ ← domain 接口（infra/crypto.AESGCMEncryptor 实现）
        └──────────────────┘

        ┌──────────────────┐
        │  Repository 实现  │ ← infra/store/apikey.Store（内部使用，其他 domain 不 import）
        └──────────────────┘
```
