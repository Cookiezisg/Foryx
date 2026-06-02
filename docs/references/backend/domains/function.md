---
id: DOC-110
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Function Domain — 无状态 Python 函数与沙箱执行

> **核心地位**：Function 是 Forgify “四项全能” (Quadrinity) 的第一元。它代表了 **“纯粹的、无状态的逻辑”**。每一行代码都在严格隔离的独立沙箱进程中运行，调用结束后环境立即销毁。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `Function` (实体主表)
```go
type Function struct {
    ID              string         `gorm:"primaryKey;type:text" json:"id"` // fn_<16hex>
    UserID          string         `gorm:"not null;index" json:"userId"`
    Name            string         `gorm:"not null;type:text" json:"name"`
    Description     string         `gorm:"type:text;default:''" json:"description"`
    Tags            []string       `gorm:"serializer:json;type:text;default:'[]'" json:"tags"`
    ActiveVersionID string         `gorm:"type:text;default:''" json:"activeVersionId"`
    CreatedAt       time.Time      `json:"createdAt"`
    UpdatedAt       time.Time      `json:"updatedAt"`
    DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
}
```

### 1.2 `Version` (代码快照表)
```go
type Version struct {
    ID            string          `gorm:"primaryKey;type:text" json:"id"` // fnv_<16hex>
    FunctionID    string          `gorm:"not null;index" json:"functionId"`
    Status        string          `gorm:"not null;default:'pending'" json:"status"` // pending|accepted|rejected
    
    // 物理载体
    Code          string          `gorm:"type:text;default:''" json:"code"`
    Parameters    []ParameterSpec `gorm:"serializer:json;type:text" json:"parameters"`
    ReturnSchema  map[string]any  `gorm:"serializer:json;type:text" json:"returnSchema"`
    Dependencies  []string        `gorm:"serializer:json;type:text" json:"dependencies"` // pip 依赖清单
    
    // 环境锚点
    EnvID         string          `gorm:"index;type:text" json:"envId"`
    EnvStatus     string          `gorm:"type:text;default:'pending'" json:"envStatus"` // installing|ready|failed
    
    CreatedAt     time.Time       `json:"createdAt"`
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Fresh-Subprocess (瞬时沙箱)
与 Handler 的常驻进程不同，Function 采用 **“即用即弃”** 的模式：
1. **注入**：将 `Code` 写入临时 `.py` 文件。
2. **拉起**：在对应的 `se_xxx` 虚拟环境下启动 Python 解释器。
3. **IO 交换**：参数通过 `sys.stdin` 注入，结果通过 `sys.stdout` 抓取 JSON。
4. **强杀**：调用结束后，无论成功与否，立即 Terminate 进程，确保无内存残留。

### 2.2 Polling 扩展模式
Function 具备一个特殊的 `kind` 属性（V1.2 引入）：
- **Normal**：常规被动调用。
- **Polling**：作为 **Trigger** 的后端。该函数必须接受 `last_cursor` 参数并返回 `{events, next_cursor}`。

### 2.3 AST 预校验
在 `edit_function` 阶段，后端会物理拉起一个轻量级 Python 探测器：
- **语法检查**：检查 `def main()` 是否存在，是否有明显的 SyntaxError。
- **元数据提取**：自动从 Docstring 提取参数说明并同步到 `ParameterSpec`。

---

## 3. 生命周期 (Lifecycle)

1. **锻造 (Forging)**：LLM 发送 Python 代码片段。
2. **环境预热 (Syncing)**：Sandbox 模块识别 `dependencies` 变更，异步执行 `pip install`。
3. **转正 (Accepting)**：用户在 UI 侧运行 `:run` 冒烟测试通过后，调 `:accept`。
4. **执行 (Running)**：在 Chat 回合或 Workflow 节点中被调度。
5. **审计 (Execution Log)**：每次运行结果均记入 `function_executions` 表，供 D22 审计。

---

## 4. 跨域集成 (Interactions)

- **Sandbox**：提供虚拟环境 (`venv`) 和 Python 二进制路径。
- **Trigger**：`polling` 类型的触发器直接驱动 Function 执行。
- **Relation**：建立 `forged_in_conversation` 关联，让用户知道这个函数是哪次聊出来的。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 场景 |
|---|---|---|---|
| `ErrASTParseError` | 422 | `FUNCTION_AST_PARSE_FAILED` | 代码写错了。 |
| `ErrRunFailed` | 422 | `FUNCTION_RUN_FAILED` | Python 运行时 Exception。 |
| `ErrEnvNotReady` | 422 | `FUNCTION_ENV_NOT_READY` | 还在安装 pip 依赖，请稍等。 |
| `ErrDependencyResolution` | 422 | `FUNCTION_DEP_FAILED` | 依赖包装不上。 |
| `ErrSandboxUnavailable` | 503 | `SANDBOX_UNAVAILABLE` | 后端 Sandbox 守护进程故障。 |
| `ErrNoActiveVersion` | 422 | `FUNCTION_NO_ACTIVE_VERSION` | |
| `ErrNotFound` | 404 | `FUNCTION_NOT_FOUND` | |
| `ErrDuplicateName` | 409 | `FUNCTION_NAME_DUPLICATE` | |
