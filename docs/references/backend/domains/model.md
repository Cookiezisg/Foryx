---
id: DOC-115
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Model Domain — 场景化模型分派与配置策略

> **核心职责**：Model 域是 Forgify 的 **“决策网关”**。它负责管理用户针对不同任务场景（如主对话、摘要提取、Agent 运行）所设定的模型偏好。通过将逻辑业务与具体 Provider/ModelID 解耦，实现了一键切源的能力。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `ModelConfig` — 场景映射表
```go
type ModelConfig struct {
    ID        string       `gorm:"primaryKey;type:text" json:"id"` // mc_<16hex>
    UserID    string       `gorm:"not null;uniqueIndex:idx_mc_user_scenario" json:"-"`
    
    // 场景枚举: dialogue (聊天) | utility (背景活) | agent (工作流)
    Scenario  string       `gorm:"not null;uniqueIndex:idx_mc_user_scenario" json:"scenario"`
    
    APIKeyID  string       `gorm:"not null" json:"apiKeyId"`
    ModelID   string       `gorm:"not null" json:"modelId"`
    
    // 高级参数: 温度、TopP 等
    Options   ModelOptions `gorm:"serializer:json;type:text" json:"options"`
}
```

### 1.2 `ModelRef` (重载引用协议)
这是在 `Conversation` 或 `NodeSpec` 中使用的 DTO，用于局部覆盖全局设置。
```go
type ModelRef struct {
    APIKeyID string `json:"apiKeyId"`
    ModelID  string `json:"modelId"`
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Scenario-Based Partitioning (场景化分区)
Forgify 将任务分为三个优先级档位：
- **Dialogue**：核心用户交互。建议使用高智力模型（Claude 3.5 Sonnet / GPT-4o）。
- **Utility**：后台任务（摘要、命名、语法检查）。建议使用低成本/极速模型（Haiku / 4o-mini）。
- **Agent**：Workflow 中的独立步骤。由用户根据具体业务复杂度配置。

### 2.2 Cascading Resolve (级联分派逻辑)
系统在获取模型实例时遵循 **“局部优先”** 策略：
1. **Explicit Override**：检查对话或 Workflow 节点是否带了 `model_override`。
2. **Global Default**：查找 `model_configs` 表中该用户设定的场景默认模型。
3. **Implicit Default**：若配置完全丢失，系统会向后端的 `PickForAgent` 逻辑报错并拦截（V1.2 强制配置决策）。

### 2.3 Key-Model Bundle (凭证闭包)
模型分派不仅返回 ModelID，还必须返回对应的 `apiKeyID`：
- **原理**：系统通过 `reqctx` 将 `apiKeyID` 透传给 `llminfra` 模块。
- **一致性**：确保调用的模型永远对应其授权过的 Key，防止跨 Provider 调用（如用 OpenAI 的 Key 调 Gemini 模型）。

---

## 3. 生命周期 (Lifecycle)

1. **设置 (Configuring)**：用户在 Settings 面板为三个场景分别选定模型。
2. **解析 (Resolving)**：应用层调 `ResolveDialogue` 或 `ResolveUtility`。
3. **构建 (Building)**：`llmclient.Factory` 整合 Key 和 Model，创建物理 Client。
4. **消耗 (Accounting)**：每轮生成结束后，利用 Resolve 到的模型标识记录 Token 账单。

---

## 4. 跨域集成 (Interactions)

- **APIKey**：提供模型所需的底层凭证。
- **Chat**：Dialogue 场景的主要消费者。
- **Compaction / Auto-title**：Utility 场景的消费者。
- **Workflow / Agent**：Agent 场景的消费者。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrNotConfigured` | 422 | `MODEL_NOT_CONFIGURED` | 用户尚未完成 Onboarding 里的模型选型。 |
| `ErrInvalidScenario`| 400 | `INVALID_SCENARIO` | 传入了未定义的场景标识符。 |
| `ErrAPIKeyRequired` | 400 | `API_KEY_ID_MISSING` | 配置记录中漏填了 Key 引用。 |
| `ErrModelIDRequired`| 400 | `MODEL_ID_MISSING` | 配置记录中漏填了模型名。 |
