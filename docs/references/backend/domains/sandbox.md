---
id: DOC-118
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Sandbox Domain — PluginSandbox v2 隔离执行原理

> **核心职责**：Sandbox 是 Forgify 的 **“物理防线”**。它负责在宿主机上构建完全隔离的 Python/Node 环境，通过 `mise` 驱动版本管理，确保用户编写的代码（fn/hd）不会越权访问系统资源。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `Env` (虚拟环境 Manifest)
```go
type Env struct {
    ID          string         `gorm:"primaryKey;type:text" json:"id"` // se_<16hex>
    
    // 所有权: 哪个实体拥有此环境
    OwnerKind   string         `gorm:"uniqueIndex:uniq_se_owner" json:"ownerKind"` // function|handler|conversation
    OwnerID     string         `gorm:"uniqueIndex:uniq_se_owner" json:"ownerID"`
    
    RuntimeID   string         `gorm:"index" json:"runtimeId"` // sr_python_3_12 等
    
    // 物理路径: 磁盘上的实际 venv 目录
    Path        string         `gorm:"type:text" json:"path"`
    
    // 依赖清单
    Deps        []string       `gorm:"serializer:json;type:text" json:"deps"`
    
    Status      string         `gorm:"type:text" json:"status"` // installing|ready|failed
    RunningPID  int            `gorm:"index" json:"-"`          // 关联的僵尸进程监控
    
    UpdatedAt   time.Time      `json:"updatedAt"`
}
```

### 1.2 `Runtime` (运行时版本)
```go
type Runtime struct {
    ID          string    `gorm:"primaryKey" json:"id"` // sr_python_3_11
    Kind        string    `json:"kind"`                 // python|nodejs
    Version     string    `json:"version"`              // 3.11.9
    MiseSpec    string    `json:"miseSpec"`             // mise 内部标识
    BootstrapOK bool      `json:"bootstrapOk"`          // mise install 成功标记
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Embedded-Mise 驱动 (ADR-014)
Forgify 不依赖系统安装的 Python，而是内置了 `mise` 二进制进行私有化部署：
- **零依赖安装**：在 `make setup` 或首次启动时，后端会自动下载对应架构的 Python/Node 二进制。
- **物理路径隔离**：所有 Runtime 存放在 `~/.forgify/sandbox/runtimes/` 下。

### 2.2 Shared-Runtime, Private-Venv
为了平衡存储空间与隔离性：
- **Runtime 层**：同一版本的 Python 二进制在全机共享。
- **Venv 层**：每个 `se_xxx` 拥有一个独立的、基于 `venv` 或 `virtualenv` 的文件夹。
- **隔离级别**：不同 Function 之间的 `pip install` 互不干扰，禁止跨环境读写。

### 2.3 Command Auto-Routing (Bash 劫持)
当 LLM 通过 Bash 工具运行命令时：
- **原理**：后端会解析命令行字符串。
- **逻辑**：若检测到 `pip install` 或 `python xxx.py`，后端会自动将其重定向为 `/path/to/se_123/bin/python`。
- **效果**：对 LLM 来说它是直接操作系统，但物理上它始终被困在对话级的沙箱中。

---

## 3. 生命周期 (Lifecycle)

1. **引导 (Bootstrapping)**：系统检查 `mise` 是否就绪。
2. **同步 (Syncing)**：创建新实体时，后端开启 goroutine 执行 `pip install -r requirements.txt`。
3. **活跃监控 (Heartbeat)**：对于 Handler 这种常驻进程，后端记录 `RunningPID`，确保异常退出时能自动清理。
4. **垃圾回收 (GC)**：每天凌晨（或用户调 `:gc`）扫描 `updated_at` 超过 7 天的环境并物理物理删除文件夹。

---

## 4. 跨域集成 (Interactions)

- **Function / Handler**：依赖 Sandbox 提供 Python 解释器路径。
- **Chat**：对话级的临时环境在对话删除时同步销毁。
- **Infrastructure**：提供磁盘占用审计 API。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 备注 |
|---|---|---|---|
| `ErrRuntimeNotSupported`| 422 | `SANDBOX_RUNTIME_MISSING` | nix 环境初始化失败。 |
| `ErrDepInstallFailed` | 502 | `SANDBOX_PIP_FAILED` | 网速慢或包版本冲突。 |
| `ErrSpawnFailed` | 502 | `SANDBOX_EXEC_ERROR` | 无法拉起子进程。 |
| `ErrEnvNotFound` | 404 | `SANDBOX_ENV_NOT_FOUND` | 物理目录可能被手动删了。 |
| `ErrSpawnTimeout` | 504 | `SANDBOX_TIMEOUT` | 运行超过 30s。 |
