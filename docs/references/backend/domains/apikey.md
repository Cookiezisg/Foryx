---
id: DOC-101
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-04
review-due: 2026-09-01
audience: [human, ai]
---
# APIKey Domain — 加密保险箱 / 哑探针 / 按 id 发钥匙

> **核心地位**：APIKey 是 Forgify 与外部世界（LLM / Search Provider）的通信令牌底座。它**只管钥匙本身**的生命周期——存、加密、探测连通、按 id 发放——刻意**不持任何 provider 语义**：「该用哪把钥匙」「钥匙背后有哪些模型 / 能力」由别的模块决定（LLM 选择 → model；搜索选择 → 未来搜索配置；模型理解 → model）。这是一次有意的职责收窄。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `APIKey` 实体（纯 struct + db tag，已去 GORM）
```go
type APIKey struct {
    ID           string     `db:"id,pk" json:"id"`                 // aki_<16hex>
    WorkspaceID  string     `db:"workspace_id,ws" json:"-"`        // 隔离：orm 写时自动填、读时自动过滤
    Provider     string     `db:"provider" json:"provider"`
    DisplayName  string     `db:"display_name" json:"displayName"` // workspace 内唯一
    KeyEncrypted string     `db:"key_encrypted" json:"-"`          // 密文，永不出进程
    KeyMasked    string     `db:"key_masked" json:"keyMasked"`     // 脱敏展示
    BaseURL      string     `db:"base_url" json:"baseUrl,omitempty"`
    APIFormat    string     `db:"api_format" json:"apiFormat,omitempty"`
    TestStatus   string     `db:"test_status" json:"testStatus"`   // pending|ok|error
    TestError    string     `db:"test_error" json:"testError,omitempty"`
    TestResponse string     `db:"test_response" json:"-"`          // 探测的上游原始返回，原样存；由 model/search 解析，apikey 从不解析
    LastTestedAt *time.Time `db:"last_tested_at" json:"lastTestedAt,omitempty"`
    CreatedAt    time.Time  `db:"created_at,created" json:"createdAt"`
    UpdatedAt    time.Time  `db:"updated_at,updated" json:"updatedAt"`
    DeletedAt    *time.Time `db:"deleted_at,deleted" json:"-"`
}
```

- **`WorkspaceID` 做隔离**：apikey 是首个真正吃 orm 自动隔离的业务表——store 不再手写 `WHERE workspace_id = ?`，orm 写时 stamp、读时过滤。
- **`DisplayName` 按 workspace 唯一**（`UNIQUE INDEX ... WHERE deleted_at IS NULL` partial）；重名由 `pkg/orm` 的 `ErrConflict` 翻译为 `ErrDisplayNameConflict`，store 不碰 SQLite 字符串。
- **`TestResponse` 是探测档案的核心**：存上游原始返回（成功时），**apikey 不解析**——这是哑探针哲学的落点（见 §3）。
- **去掉了 `models_found`（解析后列表）与 `is_default`（默认选择）**：前者 → `TestResponse` 原始 + 下游解析；后者随「选择策略」下放（见 §6）。

### 1.2 Provider 注册表 (`providers.go`)
硬编码的 provider 元数据，只含 apikey **校验 / 连接 / 探测** 所需：`Name / DisplayName / DefaultBaseURL / BaseURLRequired / TestMethod / Category`。
- **LLM**：openai, anthropic, google, deepseek, openrouter, qwen, zhipu, moonshot, doubao, ollama, custom, mock。
- **Search**：brave, serper, tavily, bocha。
- `GET /api/v1/providers` 暴露此表（onboarding 配 key 用）。Category 仅供前端分组，**不再挂任何选择逻辑**。

---

## 2. 加解密原理 (Encryption & Masking)

防止 SQLite 被拷走导致明文泄漏，凭证用 `crypto.Encryptor`（M0.3）加密落库。

- **算法**：AES-256-GCM；密钥由宿主机指纹 + 固定 Salt 经 SHA-256 拉伸出 32 字节主密钥。
- **持久化约束**：`KeyEncrypted` 落库，JSON `json:"-"` 强制过滤，**永不传前端**。
- **内存脱敏** `KeyMasked`（创建 / 换钥匙时算，前端只展示它）：`<8 → ****`；`≤20 → 前3…后4`；`>20 → 前7…后4`。

---

## 3. 健康探测：哑探针 (Dumb Probe)

`POST /api/v1/api-keys/{id}:test` 触发。**探针只回答「这把钥匙活没活」，把上游原始返回原样存档，绝不解析。**

### 3.1 怎么探（留 apikey）
按 provider 的 `TestMethod` 选**最轻能验证 key 的端点**发请求，各家认证不同：
- `get_models`：GET `/models`，`Authorization: Bearer`（OpenAI 系）。
- `anthropic_ping`：发 1-token `/v1/messages`（Anthropic **无 /models 端点**），`x-api-key` + `anthropic-version`。
- `google_list_models`：GET `/v1beta/models?key=`（key 在 query）。
- `ollama_tags`：GET `/api/tags`。
- `search_ping`：1 条结果的探测搜索（brave `X-Subscription-Token` / serper `X-API-KEY` / tavily key 在 body / bocha `Bearer`）。
- `custom`：按 `apiFormat` 走 anthropic_ping 或 get_models。

### 3.2 怎么判 + 怎么存
- **连通判断**：统一看 **HTTP 200** = 活；非 200 / 传输错误 = 失败。
- **存**：成功 → `TestStatus=ok` + `TestResponse=原始 body`（限 64KB）；失败 → `TestStatus=error` + `TestError`，`TestResponse` 留空。
- **不解析**：返回体里有没有模型、有哪些——apikey 一概不看。OpenAI 的 `/models` 返回恰好含模型列表，那是 model 模块的免费红利，不是 apikey 的事。

### 3.3 异步写回 (Detached Persist)
探测可能耗时数秒，落库用 **Detached Context**（§S9，`reqctxpkg.SetWorkspaceID(context.Background(), ws)`），即便原请求被 Cancel 也不丢结果。

---

## 4. 凭证发放 (Key Resolution)

LLM / Search 发请求时**绝不直接访问 Repository**，而是通过跨模块端口 `KeyProvider`——**永远按显式 id**（由上游 model / 搜索配置选定），绝无启发式。

### 4.1 `KeyProvider`（收窄到 2 个，全按 id）
```go
type KeyProvider interface {
    ResolveCredentialsByID(ctx, apiKeyID) (Credentials, error) // 按 id 解密递出明文凭证包
    MarkInvalidByID(ctx, apiKeyID, reason) error               // 调用方撞 401/403 → 按 id 标 error（detached）
}
```
`Credentials{Provider, Key(明文！), BaseURL(空则补 provider 默认), APIFormat}`。`Get` 按 workspace 隔离，跨 workspace 的 id 天然走 `ErrNotFound`。

### 4.2 `ProbeReader`（给 model 解析）
```go
type ProbeReader interface {
    ListProbed(ctx) ([]ProbedKey, error) // ProbedKey{Provider, TestStatus, TestResponse}，不含密钥
}
```
model 模块通过它取**探测档案**（原始返回）→ 自己解析可用模型 + 静态目录兜底。**apikey 持原始数据，model 持解读。**

---

## 5. 删除保护 (Reference Scanner)

`Service.Delete` 执行前遍历已注入的 `RefScanner`（DIP 端口）；任一报告「仍被引用」即抛 `ErrInUse` 拒删，防级联崩溃。引用方（model_config / 对话 override / workflow 节点 override）的具体实现住在各自模块，**装配时注入**——apikey 只持端口、不依赖它们。

---

## 6. 不负责的（边界）

| 不做 | 归谁 |
|---|---|
| 选哪把 key（LLM）| **model 模块**（per-scenario api_key_id + 对话 / 节点 override）|
| 选哪把 key（搜索）| **未来搜索配置模块**（显式配 api_key_id，不启发式，防乱烧钱）|
| 模型理解（有哪些 / 上下文 / effort / option）| **model 模块**（解析 `TestResponse` + 静态目录兜底）|
| 搜索返回理解 | 搜索模块 |

---

## 7. 错误字典 (Clinical Sentinels)

全部经 `errorsdomain.New(kind, code, msg)` 构造（§S20），transport 直接读 Kind/Code。

| Sentinel | HTTP | Wire Code | 场景 |
|---|---|---|---|
| `ErrNotFound` | 404 | `API_KEY_NOT_FOUND` | id 错误 / 跨 workspace |
| `ErrInvalidProvider` | 400 | `API_KEY_INVALID_PROVIDER` | 不在注册表的 Provider |
| `ErrKeyRequired` | 400 | `API_KEY_VALUE_REQUIRED` | 空秘钥 |
| `ErrBaseURLRequired` | 400 | `API_KEY_BASE_URL_REQUIRED` | Ollama / Custom 必填 URL |
| `ErrAPIFormatRequired` | 400 | `API_KEY_API_FORMAT_REQUIRED` | Custom 必填协议族 |
| `ErrDisplayNameConflict` | 409 | `API_KEY_DISPLAY_NAME_CONFLICT` | workspace 内显示名重复 |
| `ErrInUse` | 422 | `API_KEY_IN_USE` | 被引用，先解绑 |
| (handler) | 422 | `API_KEY_TEST_FAILED` | `:test` 探测失败（非 sentinel）|
