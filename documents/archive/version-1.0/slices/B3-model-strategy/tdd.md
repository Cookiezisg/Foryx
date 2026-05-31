# B3 · 模型选择策略 — 技术设计文档

**切片**：B3  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 配置存储 | `app_config` 表（key-value）| 无需单独建表，配置简单 |
| 模型列表获取 | 静态 Map + 动态从提供商 API 查询 | 本切片先用静态 Map，避免每次启动都查 API |
| 降级判断 | 捕获特定错误类型触发降级 | 不是所有错误都应该降级（如网络断开不降级，余额不足才降级）|

---

## 2. 目录结构

```
internal/model/
├── gateway.go       # 已有，本切片完善
├── selector.go      # ModelSelector：场景→模型的路由逻辑
└── catalog.go       # 各提供商支持的模型列表（静态）

internal/storage/migrations/
└── （无新迁移，复用 app_config 表）

frontend/src/components/settings/
└── ModelAssignment.tsx   # 模型分配 UI
```

---

## 3. Go 层

### 3.1 `internal/model/catalog.go` — 静态模型目录

```go
package model

// ProviderModels 各提供商的可用模型（静态）
var ProviderModels = map[string][]ModelInfo{
    "anthropic": {
        {ID: "claude-opus-4-7",          Name: "Claude Opus 4.7",    Tier: "powerful"},
        {ID: "claude-sonnet-4-6",        Name: "Claude Sonnet 4.6",  Tier: "balanced"},
        {ID: "claude-haiku-4-5-20251001",Name: "Claude Haiku 4.5",   Tier: "cheap"},
    },
    "openai": {
        {ID: "gpt-4o",       Name: "GPT-4o",       Tier: "powerful"},
        {ID: "gpt-4o-mini",  Name: "GPT-4o mini",  Tier: "cheap"},
        {ID: "o3-mini",      Name: "o3-mini",       Tier: "balanced"},
    },
    "deepseek": {
        {ID: "deepseek-chat",    Name: "DeepSeek V3", Tier: "balanced"},
        {ID: "deepseek-reasoner",Name: "DeepSeek R1", Tier: "powerful"},
    },
    // ...其他提供商
}

type ModelInfo struct {
    ID   string
    Name string
    Tier string // "powerful" | "balanced" | "cheap"
}
```

### 3.2 `internal/model/selector.go` — 选择逻辑

```go
package model

import (
    "forgify/internal/storage"
)

// Purpose 表示 LLM 的使用场景
type Purpose string
const (
    PurposeConversation Purpose = "conversation"
    PurposeCodegen      Purpose = "codegen"
    PurposeCheap        Purpose = "cheap"
)

// ModelConfig 用户为每个场景配置的模型
type ModelConfig struct {
    ConversationModelID string
    CodegenModelID      string // 空则同 ConversationModelID
    CheapModelID        string
    FallbackModelID     string // 空则不降级
}

func LoadModelConfig() (*ModelConfig, error) {
    get := func(key, def string) string {
        var v string
        storage.DB().QueryRow(
            "SELECT value FROM app_config WHERE key=?", key,
        ).Scan(&v)
        if v == "" { return def }
        return v
    }
    return &ModelConfig{
        ConversationModelID: get("model.conversation", ""),
        CodegenModelID:      get("model.codegen", ""),
        CheapModelID:        get("model.cheap", ""),
        FallbackModelID:     get("model.fallback", ""),
    }, nil
}

func SaveModelConfig(cfg *ModelConfig) error {
    pairs := map[string]string{
        "model.conversation": cfg.ConversationModelID,
        "model.codegen":      cfg.CodegenModelID,
        "model.cheap":        cfg.CheapModelID,
        "model.fallback":     cfg.FallbackModelID,
    }
    for k, v := range pairs {
        storage.DB().Exec(`
            INSERT INTO app_config(key,value) VALUES(?,?)
            ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=datetime('now')
        `, k, v)
    }
    return nil
}
```

### 3.3 完善 `gateway.go` — 带降级的模型获取

```go
package model

import (
    "context"
    "errors"
    "forgify/internal/events"

    "github.com/cloudwego/eino/components/model"
)

type ModelGateway struct {
    bridge *events.Bridge
}

// GetModel 根据场景返回 ChatModel，主模型失败时降级
func (g *ModelGateway) GetModel(ctx context.Context, purpose Purpose) (model.ChatModel, string, error) {
    cfg, _ := LoadModelConfig()

    primaryID := resolveModelID(cfg, purpose)
    if primaryID == "" {
        return nil, "", ErrNoModelConfigured
    }

    m, err := buildModel(primaryID)
    if err == nil {
        return m, primaryID, nil
    }

    // 判断是否应该降级
    if !shouldFallback(err) || cfg.FallbackModelID == "" {
        return nil, "", err
    }

    // 降级
    fallback, ferr := buildModel(cfg.FallbackModelID)
    if ferr != nil {
        return nil, "", errors.Join(err, ferr)
    }
    return fallback, cfg.FallbackModelID, ErrUsedFallback{Primary: primaryID, Fallback: cfg.FallbackModelID}
}

func resolveModelID(cfg *ModelConfig, purpose Purpose) string {
    switch purpose {
    case PurposeCodegen:
        if cfg.CodegenModelID != "" { return cfg.CodegenModelID }
        return cfg.ConversationModelID
    case PurposeCheap:
        return cfg.CheapModelID
    default:
        return cfg.ConversationModelID
    }
}

// shouldFallback 判断错误类型是否应触发降级
// 额度耗尽、模型不可用 → 降级；网络超时 → 不降级（可能很快恢复）
func shouldFallback(err error) bool {
    msg := err.Error()
    return contains(msg, "insufficient_quota") ||
        contains(msg, "model_not_found") ||
        contains(msg, "overloaded")
}

// ErrUsedFallback 降级成功时返回此错误，调用方可选择性提示用户
type ErrUsedFallback struct {
    Primary  string
    Fallback string
}
func (e ErrUsedFallback) Error() string {
    return "used fallback model: " + e.Fallback
}
```

### 3.4 在 ChatService 中处理降级提示

```go
// service/chat.go
m, modelID, err := gateway.GetModel(ctx, model.PurposeConversation)

var fallbackErr model.ErrUsedFallback
if errors.As(err, &fallbackErr) {
    // 在助手消息下方注入小字提示（通过特殊事件）
    bridge.Emit(events.Notification, events.NotificationPayload{
        Title: "已自动切换模型",
        Body:  fmt.Sprintf("主模型不可用，已切换到 %s", fallbackErr.Fallback),
        Level: "info",
    })
}
```

---

## 4. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/model-config", s.getModelConfig)
mux.HandleFunc("POST /api/model-config", s.saveModelConfig)
mux.HandleFunc("GET /api/models", s.listAvailableModels)
```

---

## 5. 前端 `ModelAssignment.tsx`

```tsx
// 展示四行 Select：主对话、代码生成、低成本、备用
// 选项来自 GET /api/models（已配置 Key 的提供商的模型）
// 保存调用 POST /api/model-config
```

---

## 6. 验收测试

```
1. 设置主对话=Claude Sonnet，发消息 → 日志显示用了 claude-sonnet-4-6
2. 设置代码生成=DeepSeek，触发工具锻造 → 日志显示用了 deepseek-chat
3. 模拟主模型返回 overloaded 错误 → 自动切换备用，前端出现提示
4. 主模型+备用都失败 → 前端显示错误，不崩溃
5. 保存配置，重启 Forgify → 配置不丢失
6. 未配置任何模型时 → GetModel 返回 ErrNoModelConfigured，前端引导去设置
```
