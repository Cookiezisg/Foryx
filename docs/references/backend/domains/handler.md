---
id: DOC-111
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Handler Domain — 有状态 Python 类与长连接服务

> **核心地位**：Handler 是 Forgify “四项全能” (Quadrinity) 的第二元。它代表了 **“常驻的、有状态的服务”**。与 Function 不同，Handler 的实例在多次调用间保持存活，适用于数据库连接、外部 API 会话维持等场景。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `Handler` (实体主表)
```go
type Handler struct {
    ID              string         `gorm:"primaryKey;type:text" json:"id"` // hd_<16hex>
    UserID          string         `gorm:"not null;index" json:"userId"`
    Name            string         `gorm:"not null;type:text" json:"name"`
    
    // 初始化配置: 加密存储的 JSON 载荷
    ConfigEncrypted string         `gorm:"type:text;default:''" json:"-"`
    
    ActiveVersionID string         `gorm:"type:text;default:''" json:"activeVersionId"`
    CreatedAt       time.Time      `json:"createdAt"`
    UpdatedAt       time.Time      `json:"updatedAt"`
    DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}
```

### 1.2 `Version` (类定义表)
```go
type Version struct {
    ID            string          `gorm:"primaryKey;type:text" json:"id"` // hdv_<16hex>
    HandlerID     string          `gorm:"not null;index" json:"handlerId"`
    Status        string          `gorm:"not null;default:'pending'" json:"status"`
    
    // 结构化代码块
    Imports       string          `gorm:"type:text" json:"imports"`
    InitBody      string          `gorm:"type:text" json:"initBody"`     // __init__ 逻辑
    ShutdownBody  string          `gorm:"type:text" json:"shutdownBody"` // __del__ 或清理逻辑
    Methods       []MethodSpec    `gorm:"serializer:json;type:text" json:"methods"` // 公开方法索引
    
    Dependencies  []string        `gorm:"serializer:json;type:text" json:"dependencies"`
    EnvID         string          `gorm:"index;type:text" json:"envId"`
    EnvStatus     string          `gorm:"type:text;default:'pending'" json:"envStatus"`
    
    CreatedAt     time.Time       `json:"createdAt"`
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Instance-Keep-Alive (实例常驻)
Handler 运行在独立的长连接子进程中：
1. **实例化**：调用方首次调 `:call` 时，后端在沙箱内拉起进程，执行 `InitBody`。
2. **状态驻留**：进程不退出，类成员变量 (`self.xxx`) 在多次调用间保持。
3. **RPC 通信**：后端与子进程通过 Unix Domain Socket 或 StdIO 运行轻量级 JSON-RPC。
4. **懒加载销毁**：若 15 分钟无调用，系统会自动发送 `SHUTDOWN` 信号并执行 `ShutdownBody`。

### 2.2 双层配置架构
- **类定义 (Version)**：固化了方法的逻辑和入参 Schema。
- **实例配置 (Config)**：存储了秘钥、端点等。
- **逻辑隔离**：同一套 `Version` 代码可以被不同用户实例化为不同的 `Config`，实现“代码与凭证”的分离。

### 2.3 自动方法探测
当用户编辑 Handler 代码时，系统会自动利用 Python `ast` 模块扫过整个类定义：
- 识别所有 `def` 开头的方法。
- 排除 `_` 开头的私有方法。
- 自动提取 `async def` 的协程标识。

---

## 3. 生命周期 (Lifecycle)

1. **定义 (Defining)**：编写类逻辑和 `__init__` 参数 Schema。
2. **同步 (Syncing)**：Sandbox 准备 venv 环境。
3. **实例化测试 (Probing)**：用户填入 `config` 运行 `:call`。系统拉起进程。
4. **服务运行 (Serving)**：进入 Stable 态。
5. **停机回收 (Grooming)**：超时销毁或用户执行 `:reconnect` 强制重启。

---

## 4. 跨域集成 (Interactions)

- **APIKey**：Handler 的 `config` 经常引用 `aki_` ID，通过解密后注入子进程。
- **Workflow**：`tool` 节点若 `kind=handler`，则会触发该 Handler 的方法调用。
- **Sandbox**：Handler 拥有独立的进程管理生命周期，不参与 Function 的瞬时 GC。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrInstanceCrashed` | 502 | `HANDLER_CRASHED` | 子进程 Exception 退出。 |
| `ErrMethodNotFound` | 404 | `HANDLER_METHOD_NOT_FOUND` | 调用了一个不存在的 def。 |
| `ErrConfigIncomplete` | 422 | `HANDLER_CONFIG_MISSING` | 没配初始化参数。 |
| `ErrInstanceRPCTimeout`| 504 | `HANDLER_TIMEOUT` | 子进程死锁或响应超 30s。 |
| `ErrEnvFailed` | 422 | `HANDLER_ENV_FAILED` | 环境损毁无法拉起。 |
