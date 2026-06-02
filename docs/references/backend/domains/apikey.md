---
id: DOC-101
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# APIKey Domain — 凭证管理与加解密原理

> **核心地位**：APIKey 是 Forgify 与外部世界（LLM, Search Providers）的通信令牌底座。通过**物理加密与内存脱敏**实现本地安全的凭证管理，同时提供**BYOK (Bring Your Own Key) 健康探测**机制。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `APIKey` 实体结构
```go
type APIKey struct {
    ID           string         `gorm:"primaryKey;type:text" json:"id"`
    UserID       string         `gorm:"not null;index:idx_api_keys_user_id;index:idx_api_keys_user_provider,priority:1;type:text" json:"userId"`
    
    Provider     string         `gorm:"not null;index:idx_api_keys_user_provider,priority:2;type:text" json:"provider"`
    DisplayName  string         `gorm:"not null;type:text;default:''" json:"displayName"`
    
    // 物理层密文与脱敏明文
    KeyEncrypted string         `gorm:"not null;type:text" json:"-"`
    KeyMasked    string         `gorm:"not null;type:text" json:"keyMasked"`
    
    // 自定义接入参数
    BaseURL      string         `gorm:"type:text;default:''" json:"baseUrl"`
    APIFormat    string         `gorm:"type:text;default:''" json:"apiFormat"`
    
    // 健康探测状态
    TestStatus   string         `gorm:"type:text;default:'pending'" json:"testStatus"` // 'pending' | 'ok' | 'error'
    TestError    string         `gorm:"type:text;default:''" json:"testError"`
    LastTestedAt *time.Time     `json:"lastTestedAt"`
    ModelsFound  []string       `gorm:"serializer:json;type:text;default:'[]'" json:"modelsFound"`
    
    // Provider 类别内的默认选择
    IsDefault    bool           `gorm:"not null;default:false" json:"isDefault"`
    
    CreatedAt    time.Time      `json:"createdAt"`
    UpdatedAt    time.Time      `json:"updatedAt"`
    DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}
```

### 1.2 Provider 注册表 (The Registry)
Forgify 内部维护了一张硬编码的提供商注册表 (`providers.go`)，将提供商分为 `CategoryLLM` 和 `CategorySearch`。
- **LLM**: `openai`, `anthropic`, `google`, `deepseek`, `openrouter`, `qwen`, `zhipu`, `moonshot`, `doubao`, `ollama`, `custom`, `mock`.
- **Search**: `brave`, `serper`, `tavily`, `bocha`.

---

## 2. 加解密原理 (Encryption & Masking)

为了防止 SQLite 数据库被拷贝导致明文密钥泄漏，Forgify 实现了 `crypto.Encryptor` 接口。

### 2.1 物理加密 (`infra/crypto/AESGCMEncryptor`)
- **算法**：AES-256-GCM (IND-CPA 安全，自带认证标签)。
- **密钥推导**：基于宿主机硬件指纹 (`fingerprint`) + 固定 Salt (`forgify:aesgcm:v1:...`)，通过 SHA-256 哈希拉伸出 32 字节的主密钥 (Master Key)。
- **线缆格式**：`v1:<base64(nonce || ciphertext || tag)>`。
- **持久化约束**：`KeyEncrypted` 落库；JSON 序列化时通过 `json:"-"` 强制过滤，**永不传给前端**。

### 2.2 内存脱敏 (Masking)
在创建或更新密钥时，系统会计算 `KeyMasked` 字段并落库，前端仅展示此字段：
- `< 8 字符`：`****`
- `≤ 20 字符`：保留前 3 后 4，中间掩码（如 `sk-...abcd`）。
- `> 20 字符`：保留前 7 后 4。

---

## 3. BYOK 路由与解析机制 (Key Resolution)

LLM 或 Search 工具在需要发起网络请求时，**绝不直接访问 Repository**，而是通过跨域端口 `KeyProvider.ResolveCredentials`。

### 3.1 `Credentials` DTO (单次通话凭证)
```go
type Credentials struct {
    Provider  string
    Key       string // 解密后的明文！
    BaseURL   string // 若用户未填，自动补全 Provider 注册表中的 DefaultBaseURL
    APIFormat string // 仅 custom key 有用
}
```

### 3.2 WebSearch 路由优先级
WebSearch 工具通过 `DefaultSearchProvider()` 查找。如果没有设置 `is_default`，则按照固定优先级遍历探测：
**顺序**：`brave` -> `serper` -> `tavily` -> `bocha`。
只要发现其中一个有配置，即提取 Credentials 发起搜索。

### 3.3 失效联动 (`MarkInvalid`)
当外部调用（如 LLM 网关或 Search API）返回 **401/403** 错误时，调用方会触发 `KeyProvider.MarkInvalid(ctx, provider, reason)`。
- **机制**：通过 `reqctxpkg.SetUserID(context.Background(), uid)` 创建 **Detached Context** 更新数据库的 `test_status = 'error'`。
- **目的**：确保即便原始请求被 Cancel，数据库状态依然更新，前端 UI 能立刻将该 Key 的角标翻红。

---

## 4. 健康探测逻辑 (Connectivity Tester)

用户在前端点击“Test Connection”时触发 `POST /api/v1/api-keys/{id}:test`。

### 4.1 异步写回 (Detached Persist)
测试请求本身可能需要几秒甚至十几秒。为了防止用户刷新页面导致 `context.Canceled` 丢弃测试结果，测试完成后的落库动作使用 **Detached Context**。

### 4.2 探测模式 (Test Methods)
`Service.Test` 会根据 Provider 的种类分发不同的探测 HTTP 请求：
- `get_models`: 调 `/v1/models`，既验证 Auth 又填充 `ModelsFound` 下拉列表（OpenAI/DeepSeek 等）。
- `anthropic_ping`: 发送一个带 `max_tokens=1` 的极小生成请求。
- `search_ping`: 向搜索 API 的探测接口发一个探针 Query。

---

## 5. 删除保护 (Reference Scanner)

APIKey 是系统的底层资源。`Service.Delete` 在执行前会遍历已注入的 `RefScanner`：
- `modelConfigScan`：是否被 `/api/v1/model-configs` 关联。
- `convOverrideScan`：是否有对话线程正强制绑定该 Key (`conversations.model_override`)。
- `nodeOverrideScan`：是否有 Workflow 节点锁定了该 Key。
若命中任意引用，抛出 `ErrInUse`，拒绝删除。这是为了防止级联崩溃。

---

## 6. 错误字典 (Clinical Sentinels)

| Sentinel | HTTP | Wire Code | 场景及处理建议 |
|---|---|---|---|
| `ErrNotFound` | 404 | `API_KEY_NOT_FOUND` | ID 错误。 |
| `ErrNotFoundForProvider` | 404 | `API_KEY_PROVIDER_NOT_FOUND` | 该提供商下没有任何活跃秘钥。 |
| `ErrInvalidProvider` | 400 | `INVALID_PROVIDER` | 尝试创建不在硬编码注册表中的 Provider。 |
| `ErrBaseURLRequired` | 400 | `BASE_URL_REQUIRED` | Ollama/Custom 等私有部署必须指定 BaseURL。 |
| `ErrAPIFormatRequired` | 400 | `API_FORMAT_REQUIRED` | Custom 必须指定 `openai-compatible` 等协议族。 |
| `ErrKeyRequired` | 400 | `KEY_REQUIRED` | 空秘钥（对于不需要鉴权的 Mock/Ollama 允许填 Dummy 字符）。 |
| `ErrDisplayNameConflict` | 409 | `API_KEY_NAME_CONFLICT` | 同一用户下 `DisplayName` 唯一性冲突。 |
| `ErrInUse` | 422 | `API_KEY_IN_USE` | 被 ModelConfig 等锁定，请用户先去解绑。 |
