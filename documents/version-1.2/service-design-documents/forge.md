# forge domain — 详细设计文档 v3

**所属 Phase**：Phase 3（沙箱迭代 1 完成于 2026-05-03）
**状态**：✅ 已实现（2026-04-26 初版；2026-05-02 Phase 3 后优化轮重命名 tool→forge + Tool 接口重构 + 子包重组；2026-05-03 沙箱迭代 1：捆绑 uv + python-build-standalone + per-EnvID venv 共享 + 同步 sync + draft→pending→accept lifecycle + entity-state 嵌入 env 字段）
**职责**：管理用户锻造的 Python 工具全生命周期——CRUD、版本历史、pending 变更确认、测试用例、沙箱执行、导入导出；并向 ReAct Agent 提供 5 个 System Tool（search / get / create / edit / run）

**依赖**：
- `infra/db`（GORM + modernc.org/sqlite）
- `infra/sandbox`（捆绑 uv + python-build-standalone，per-EnvID venv，N=3 LRU buffer；6 个方法：Sync / Run / Destroy / DestroyEnv / WriteCodeFile / PythonPath）
- `infra/llm`（create_forge / edit_forge 内部 LLM 调用 + GenerateTestCases）
- `pkg/reqctx`（userID 读取，agent-run IDs 读取）
- `domain/events`（SSE 事件推送）

**被依赖**：
- `app/tool/forge/`（5 个 system tool 实现的子包，由 app/chat 组装注入 ReAct Agent；`forgetool.ForgeTools()` 工厂返回 `[]toolapp.Tool`）
- Phase 4 workflow 节点

**Tool 接口规约**：所有 5 个 forge system tool 实现 `app/tool.Tool` 接口（10 方法全必填，详见 [`CLAUDE.md §S18`](../../../CLAUDE.md)）。

---

## 1. 核心决策

| 决策 | 选择 | 理由 |
|---|---|---|
| pending 与 version 的关系 | **合并为一张表** `forge_versions`，用 `status` 区分 | pending 和 version 形状完全一样，都是完整工具快照，无需两张表 |
| 版本快照内容 | **完整快照**：name + description + code + parameters + returnSchema + tags | 只存 code 的版本无法完整回滚，也无法看到历史状态 |
| pending 触发条件 | **所有 LLM 发起的变更**（code + 元数据）统一走 pending | 用户直接操作（HTTP PATCH / revert）立即生效，不走 pending |
| 工具搜索 | **LLM 排序**：SearchForge 把全部工具名+描述发给 LLM，LLM 返回按相关度排好的 ID + score 列表 | 比向量搜索准确（LLM 完整理解语义）；工具数量少（20-200），一次 prompt 能全放进去；无需 embedding API 或本地向量库，任何 LLM provider 都能用 |
| System Tool 位置 | `app/tool/forge/` 嵌套子包（每文件一 tool：search/get/create/edit/run.go），工厂 `ForgeTools()` 返 `[]toolapp.Tool`；组装留在 `cmd/server/main.go` | §S12 例外：tool framework meta-namespace 允许嵌套子包；Style B 命名 `SearchForge / GetForge / CreateForge / EditForge / RunForge` 显式不重叠 |
| resolveAttachments | **RunForge（System Tool）调用前完成**，不进 Service | forge Service 不感知 att_id 概念，保持纯粹 |
| GenerateTestCases | Service 方法**同步返回 `*GenerateResult`** | LLM 是非流式调用（`llm.Generate`），逐条流式推送只是化妆——直接整批返回更清晰 |
| LLM 注入 | **LLMClient 接口注入 Service** | GenerateTestCases 是 tool domain 自己的能力，不是 chat 触发的 |
| 代码生成方式 | **One-shot**，LLM 一次生成完整函数 | 工具是单函数，全量重写比 patch 更可靠 |
| 沙箱隔离 | **subprocess + 30s timeout** | 本地单用户；Docker 是过度工程 |
| AST 解析 | **Python subprocess + Google-style docstring** | 可靠提取 parameters（含 required）和 returnSchema；解析器使用 sandbox 捆绑的 Python（不依赖系统 Python）|
| 归档 | **不做**，只有软删除 | 本地单用户，工具数量有限 |
| LLM 能否删除工具 | **不能** | 删除是破坏性操作，只走 HTTP API |
| 危险操作提示 | LLM 在 tool_call args 自报 `destructive: true` | per-call 标注比静态 IsDestructive() 精准（同一 tool 不同 args 可能不同）；UI 据此显示警示徽章；存进 ToolCallData 一等字段 + ChatToolCall SSE 字段 |
| AST dry-run | CreateForge / EditForge 在 streamCode 后调 `forgeapp.Service.ParseCode(code)` 验证，失败立刻返 LLM 重试信号 | 不进 svc.Create 的存储 I/O；干净的错误路径 |
| RunForge 输出 | 50KB 截断（`maxOutputBytes`）| 防失控 forge 撑爆 LLM context；超限替换为 notice 字符串而非裁剪 |
| **沙箱迭代 1：Python 解释器** | **捆绑 python-build-standalone**（不依赖系统 Python）| 桌面端零依赖；用户解压 .app 即用；每平台 ~50MB |
| **沙箱迭代 1：依赖管理** | **捆绑 uv** 跑 `uv pip install` 装第三方包 | 比 venv+pip 快一个量级；resolver 给清晰错误；wheel 缓存共享 |
| **沙箱迭代 1：venv 粒度** | **per-EnvID** = `sha256(normalize_and_sort(deps).join("\n") + "\n" + pythonVersion).hex[:6]`；每 forge 最多 N=3 个 EnvID（LRU 淘汰）| 同 deps 不同 ForgeVersion 复用 venv（节省磁盘 + 装包）；保留 N 让用户切换历史版本时有概率命中已 ready 的 venv；超 N 淘汰最旧的 |
| **沙箱迭代 1：sync 时机** | **同步**（svc.Create / CreatePending / RevertToVersion 调用 SyncEnvForVersion）| 与"创建/编辑就立刻知道能不能用"心智匹配；错误归属清晰（用户立刻看到 envError）；无 worker 调度复杂度 |
| **沙箱迭代 1：错误模型** | **punt-to-AI**：sync/run 失败把 stderr 装进 `EnvError` 不主动恢复；LLM 看到 tool_result 中的 env_status/env_error 决定下一步（edit_forge 改 deps 重试 / 放弃 / 提示用户）| MVP 阶段不构建复杂回退/状态修复；让 AI 处理 |
| **沙箱迭代 1：lifecycle** | **draft（内存）→ pending（DB row, status='pending', envStatus 同步流转）→ user accept（status='accepted')**；只在 LLM 工具调用走 pending；HTTP CRUD 直接写 accepted | accept 守卫 envStatus（仅 ready 放行）保护用户不接受坏依赖 |
| **沙箱迭代 1：ASTParser** | 改造为 `forgeapp.ASTParser{ pythonPath string }`，pythonPath 从 `sandbox.PythonPath()` 取 | 不依赖系统 Python；与 sandbox runtime 一致 |
| **沙箱迭代 1：超时** | **删除 30s 超时** | 用户写的工具有可能合法长跑（爬数据 / 训练）；ctx cancel + 进程组 kill 已能终止失控进程，硬超时反而误杀 |

---

## 2. 多租户准备

- 所有表带 `user_id TEXT NOT NULL`
- Store 方法首行 `reqctx.GetUserID(ctx)`，缺失返错（接线 bug）
- Phase 3 仍硬编码 `"local-user"`

---

## 3. 领域模型

### 3.1 Forge（主实体）

```go
type Forge struct {
    ID           string         `gorm:"primaryKey;type:text"           json:"id"`
    UserID       string         `gorm:"not null;index;type:text"       json:"-"`
    Name         string         `gorm:"not null;type:text"             json:"name"`
    Description  string         `gorm:"not null;type:text;default:''"  json:"description"`
    Code         string         `gorm:"not null;type:text"             json:"code"`
    Parameters   string         `gorm:"type:text;default:'[]'"         json:"parameters"`   // JSON: [{name,type,required,description,default?}]
    ReturnSchema string         `gorm:"type:text;default:'{}'"         json:"returnSchema"` // JSON: {type,description}
    Tags         string         `gorm:"type:text;default:'[]'"         json:"tags"`          // JSON: ["tag1"]
    VersionCount int            `gorm:"not null;default:0"             json:"versionCount"`  // 当前最大 accepted version 号

    // ActiveVersionID 指向当前 active 的 ForgeVersion 行（status='accepted'）。
    // 沙箱迭代 1 引入：env state 字段挂在 ForgeVersion 行而非 Forge 主行——切版本时不用迁
    // env state，envID 自然跟着 ForgeVersion 走。Forge 主表只存"指针"。
    ActiveVersionID string `gorm:"type:text;default:''"           json:"-"`

    CreatedAt    time.Time      `json:"createdAt"`
    UpdatedAt    time.Time      `json:"updatedAt"`
    DeletedAt    gorm.DeletedAt `gorm:"index"                          json:"-"`

    // Pending 是当前活跃的 pending 变更（如有）。计算字段——序列化前由
    // handler/service 填充，不是 DB 列。nil 表示无 pending。
    Pending *ForgeVersion `gorm:"-" json:"pending,omitempty"`

    // 计算字段（gorm:"-"）——服务序列化前由 attachActiveEnv 从 ActiveVersionID 指向的
    // ForgeVersion 行抄过来。让 GET /forges/{id} 响应同时含 active 代码 + active env 状态，
    // 前端不用二次查 versions/{v}。
    EnvStatus     string     `gorm:"-" json:"envStatus,omitempty"`     // pending|syncing|ready|failed|evicted（空 = 无 active）
    EnvError      string     `gorm:"-" json:"envError,omitempty"`      // failed/evicted 时的错误摘要（uv stderr tail）
    EnvSyncedAt   *time.Time `gorm:"-" json:"envSyncedAt,omitempty"`   // 最后一次 ready 时间
    EnvSyncStage  string     `gorm:"-" json:"envSyncStage,omitempty"`  // resolving|preparing|installing（仅 syncing 期间有值）
    EnvSyncDetail string     `gorm:"-" json:"envSyncDetail,omitempty"` // uv 当前在做的具体动作（如 "Resolved 8 packages"）
}

func (Forge) TableName() string { return "forges" }
```

| 字段 | 说明 |
|---|---|
| `ID` | `f_<16hex>` |
| `Name` | forge 库内唯一（partial UNIQUE：`UNIQUE(user_id, name) WHERE deleted_at IS NULL`）|
| `Code` | 当前 active 代码（最新 accepted version 的代码）|
| `Parameters` | `[{"name":"x","type":"str","required":true,"description":"...","default":null}]` |
| `ReturnSchema` | `{"type":"list","description":"..."}` |
| `VersionCount` | 最新 accepted version 号，从 1 开始；create_forge 期间 stub 是 0 |
| `ActiveVersionID` | 沙箱迭代 1 新增：指向 active ForgeVersion 行的 ID（`fv_<16hex>`）。Run / 运行测试用例都根据它取 EnvID。无 active 版本时为空（draft 阶段）|
| `Pending` | **计算字段**（gorm:"-"），由 service 层 `attachPending` 在 GET / List 后填充。entity-state SSE `forge` 事件的载荷依赖此字段——edit_forge 期间 draft pending 挂在此上，前端 forge 面板从 `Forge.Pending.Code` 读流式生长的代码 |
| `EnvStatus / EnvError / EnvSyncedAt / EnvSyncStage / EnvSyncDetail` | **计算字段**（gorm:"-"），沙箱迭代 1 新增。由 `attachActiveEnv` 从 ActiveVersionID 指向的 ForgeVersion 行抄过来。`EnvSyncStage` 在 syncing 期间根据 uv stderr 解析（resolving / preparing / installing），其他状态为空。这些字段同时随 `forge` SSE 事件流出——前端 forge 面板直接渲染状态徽章 + 进度条 + 错误面板，无需额外订阅 |

### 3.2 ForgeVersion（版本历史 + pending 变更 + env 状态，合并表）

```go
type ForgeVersion struct {
    ID           string    `gorm:"primaryKey;type:text"           json:"id"`
    ForgeID      string    `gorm:"not null;index;type:text"       json:"forgeId"`
    UserID       string    `gorm:"not null;type:text"             json:"-"`
    Version      *int      `gorm:"type:integer"                   json:"version"`      // pending/rejected 时为 nil
    Status       string    `gorm:"not null;type:text"             json:"status"`       // "pending"|"accepted"|"rejected"

    // 完整 forge 快照
    Name         string    `gorm:"not null;type:text"             json:"name"`
    Description  string    `gorm:"type:text;default:''"           json:"description"`
    Code         string    `gorm:"not null;type:text"             json:"code"`
    Parameters   string    `gorm:"type:text;default:'[]'"         json:"parameters"`
    ReturnSchema string    `gorm:"type:text;default:'{}'"         json:"returnSchema"`
    Tags         string    `gorm:"type:text;default:'[]'"         json:"tags"`

    // 沙箱迭代 1 新增：依赖声明 + 解释器版本（不可变快照）。每条 ForgeVersion 永久绑定
    // 一份 deps + pyver 组合——同 ForgeID 不同 ForgeVersion 可以有不同 deps（用户编辑加包了）。
    Dependencies  string `gorm:"type:text;default:'[]'"        json:"dependencies"`  // JSON array of PEP 508 strings
    PythonVersion string `gorm:"type:text;default:''"          json:"pythonVersion"` // PEP 440 spec, 如 ">=3.12"
    EnvID         string `gorm:"type:text;index;default:''"    json:"envId"`         // sha256(normalize+sort(deps).join+\n+pyver)[:6]，6 hex 字符

    // 沙箱迭代 1 新增：env 状态机（5 态）+ 上下文。Service 层在 sync 各阶段写入。
    EnvStatus     string     `gorm:"type:text;default:'pending'"   json:"envStatus"`              // pending|syncing|ready|failed|evicted
    EnvError      string     `gorm:"type:text;default:''"          json:"envError"`               // failed 时填 uv stderr tail（最多 ~2KB）
    EnvSyncedAt   *time.Time `gorm:"type:datetime"                 json:"envSyncedAt,omitempty"`  // ready 时刻；evicted 后保留作历史参考
    EnvSyncStage  string     `gorm:"type:text;default:''"          json:"envSyncStage,omitempty"` // resolving|preparing|installing（syncing 期间有值，sync 完清空）
    EnvSyncDetail string     `gorm:"type:text;default:''"          json:"envSyncDetail,omitempty"` // 当前 stage 的具体内容（如 "Installed 12 packages in 3.4s"）

    // ChangeReason 记录此版本的变更意图（Phase 5 改名 from `Message`：
    // LLM 指令 | "manual edit" | "reverted to v{N}" | "initial"）
    ChangeReason string    `gorm:"type:text;default:''"           json:"changeReason"`
    CreatedAt    time.Time `json:"createdAt"`
    UpdatedAt    time.Time `json:"updatedAt"`
}

func (ForgeVersion) TableName() string { return "forge_versions" }
```

**版本状态流转**：
```
pending → accepted  （用户确认；envStatus 必须 ready，否则 422）→ 分配 version 号，更新 Forge 主表 ActiveVersionID
pending → rejected  （用户拒绝）→ version 保持 nil；venv 不立刻销毁（让其他 ForgeVersion 复用）
```

**EnvStatus 状态机（沙箱迭代 1）**：
```
pending（刚创建，未 sync）
  → syncing（Service 调 sandbox.Sync）
      → ready（uv 成功，写 EnvSyncedAt）
      → failed（uv 失败，写 EnvError）

ready
  → evicted（被 trimEnvBuffer 淘汰；venv 目录已删，但 ForgeVersion 行保留）
      → 用户/AI 调 :revert 或 RevertToVersion 时检测 evicted，重新触发 sandbox.Sync → syncing → ready
```

`EnvSyncStage / EnvSyncDetail` 仅在 syncing 期间有值，sync 完毕（ready / failed）清空 stage、保留 detail 作 success/error 摘要。

**版本号分配**：accepted 时 `version = forge.VersionCount + 1`，同时 `forge.VersionCount++`，并写 `forge.ActiveVersionID = pv.ID`

**上限**：
- 每 forge 最多保留 `MaxAcceptedVersions=50` 条 `status='accepted'` 记录，超限硬删最旧的 accepted 版本（连带 venv）。rejected/pending 不计入上限。
- 每 forge 同时保留的不同 EnvID 最多 `MaxEnvIDsPerForge=3` 个；超限时 `trimEnvBuffer` 把最旧的 EnvID 涉及的 ForgeVersion 行标记 `EnvStatus='evicted'`，并调 `sandbox.DestroyEnv(forgeID, envID)` 删 venv 目录。被 evicted 的 ForgeVersion 行**不**删除——下次切回会自动重 sync。

### 3.3 ForgeTestCase（测试用例定义）

```go
type ForgeTestCase struct {
    ID             string    `gorm:"primaryKey;type:text"        json:"id"`
    ForgeID        string    `gorm:"not null;index;type:text"    json:"forgeId"`
    UserID         string    `gorm:"not null;type:text"          json:"-"`
    Name           string    `gorm:"not null;type:text"          json:"name"`
    InputData      string    `gorm:"type:text;default:'{}'"      json:"inputData"`      // JSON object
    ExpectedOutput string    `gorm:"type:text;default:''"        json:"expectedOutput"` // JSON，空=不断言
    CreatedAt      time.Time `json:"createdAt"`
    UpdatedAt      time.Time `json:"updatedAt"`
}

func (ForgeTestCase) TableName() string { return "forge_test_cases" }
```

### 3.4 ForgeExecution（执行历史，Phase 5 统一表）

**Phase 5 重构**（2026-05-02）：原 `ForgeRunHistory` + `ForgeTestHistory` 两表合并为单一 `ForgeExecution`，用 `Kind` 区分 `"run"` / `"test"`。新增 chat 触发上下文 4 字段（`TriggeredBy` + `ConversationID` + `MessageID` + `ToolCallID`），让 LLM 在 chat 中调用 run_forge 后，可从 chat 消息追溯到对应执行行。

每次 `:run` / 测试用例执行 / LLM run_forge 调用都写一条。

```go
type ForgeExecution struct {
    ID           string `gorm:"primaryKey;type:text"                                            json:"id"`
    ForgeID      string `gorm:"not null;index:idx_fe_forge_created,priority:1;type:text"        json:"forgeId"`
    UserID       string `gorm:"not null;type:text"                                              json:"-"`
    ForgeVersion int    `gorm:"not null"                                                        json:"forgeVersion"`

    // Discriminator + 结果
    Kind      string `gorm:"not null;type:text"     json:"kind"`     // "run" | "test"
    Input     string `gorm:"type:text;default:'{}'" json:"input"`    // JSON
    Output    string `gorm:"type:text;default:''"   json:"output"`
    OK        bool   `gorm:"not null"               json:"ok"`
    ErrorMsg  string `gorm:"type:text;default:''"   json:"errorMsg"`
    ElapsedMs int64  `gorm:"not null;default:0"     json:"elapsedMs"`

    // test 专属字段（Kind="run" 时空）
    TestCaseID string `gorm:"type:text;default:'';index" json:"testCaseId,omitempty"`
    BatchID    string `gorm:"type:text;default:'';index" json:"batchId,omitempty"`
    Pass       *bool  `gorm:"type:integer"               json:"pass,omitempty"` // nil=无断言

    // 触发上下文
    TriggeredBy    string `gorm:"not null;type:text;default:'http'"     json:"triggeredBy"`     // "chat" | "http"
    ConversationID string `gorm:"type:text;default:'';index:idx_fe_msg" json:"conversationId,omitempty"`
    MessageID      string `gorm:"type:text;default:'';index:idx_fe_msg" json:"messageId,omitempty"`
    ToolCallID     string `gorm:"type:text;default:''"                  json:"toolCallId,omitempty"`

    CreatedAt time.Time `gorm:"index:idx_fe_forge_created,priority:2" json:"createdAt"`
}

func (ForgeExecution) TableName() string { return "forge_executions" }
```

**复合索引 2 个**：
- `idx_fe_forge_created (forge_id, created_at)` — 单 forge 历史按时间倒序检索（最常用）
- `idx_fe_msg (conversation_id, message_id)` — 一次 chat 消息触发的所有 forge 调用追溯

**保留上限**：`MaxExecutionsPerForge = 300` 条/forge（合并上限，原 100 + 200 = 300），超限硬删最旧。

### 3.6 ExecutionResult（domain 层共享类型）

定义在 `domain/tool` 避免 `infra/sandbox` 和 `app/tool` 相互依赖。

```go
type ExecutionResult struct {
    OK        bool   `json:"ok"`
    Output    any    `json:"output"`
    ErrorMsg  string `json:"errorMsg"`
    ElapsedMs int64  `json:"elapsedMs"`
}
```

---

## 4. 常量

```go
const (
    // VersionStatus values for ForgeVersion.Status.
    VersionStatusPending  = "pending"
    VersionStatusAccepted = "accepted"
    VersionStatusRejected = "rejected"

    // ExecutionKind values for ForgeExecution.Kind.
    ExecutionKindRun  = "run"  // 临时运行 / LLM 调用
    ExecutionKindTest = "test" // 测试用例

    // TriggeredBy values for ForgeExecution.TriggeredBy.
    TriggeredByChat = "chat" // LLM 在 chat 中调用
    TriggeredByHTTP = "http" // 用户直接调 HTTP

    // EnvStatus values for ForgeVersion.EnvStatus（沙箱迭代 1）.
    EnvStatusPending  = "pending"  // 刚创建，sandbox.Sync 还没启动
    EnvStatusSyncing  = "syncing"  // uv 进程跑中（resolving/preparing/installing）
    EnvStatusReady    = "ready"    // venv 就绪可执行
    EnvStatusFailed   = "failed"   // uv 报错，stderr 已存 EnvError
    EnvStatusEvicted  = "evicted"  // venv 已被 trimEnvBuffer 淘汰；切回此 ForgeVersion 时会重 sync

    // Retention.
    MaxAcceptedVersions   = 50  // 每 forge accepted 版本上限
    MaxExecutionsPerForge = 300 // 每 forge 执行历史上限（合并 run+test）
    MaxEnvIDsPerForge     = 3   // 每 forge 同时保留的不同 EnvID 上限（沙箱迭代 1）；超限 LRU 淘汰

    // DefaultPythonVersion 是 ForgeVersion.PythonVersion 为空时的兜底值。
    // 用 PEP 440 spec，让 uv 选 sandbox bundle 里满足条件的版本。
    DefaultPythonVersion = ">=3.12"
)
```

**沙箱迭代 1 改动**：
- 删除原 `SandboxTimeout = 30 * time.Second`——见 §1 决策表"删除 30s 超时"
- 新增 `EnvStatus*` 5 态 + `MaxEnvIDsPerForge` + `DefaultPythonVersion`

---

## 5. Sentinel 错误

```go
var (
    ErrNotFound         = errors.New("forge: not found")
    ErrDuplicateName    = errors.New("forge: name already exists")
    ErrVersionNotFound  = errors.New("forge: version not found")
    ErrPendingNotFound  = errors.New("forge: no pending change found")
    ErrPendingConflict  = errors.New("forge: already has a pending change")
    ErrTestCaseNotFound = errors.New("forge: test case not found")
    ErrRunFailed        = errors.New("forge: execution failed")
    ErrASTParseError    = errors.New("forge: code AST parse failed")
    ErrImportInvalid    = errors.New("forge: import data invalid")

    // 沙箱迭代 1 新增。
    ErrEnvNotReady          = errors.New("forge: env not ready")           // run / accept 时 EnvStatus 不是 ready（pending / syncing / evicted）
    ErrEnvFailed            = errors.New("forge: env sync failed")         // run / accept 时 EnvStatus = failed
    ErrSandboxUnavailable   = errors.New("forge: sandbox unavailable")     // sandbox.Bootstrap 没成功（资源缺失 / 解压失败 / 重签失败）
    ErrDependencyResolution = errors.New("forge: dependency resolution failed") // pyproject 渲染或 PEP 508 校验阶段就报错（罕见，多数错误归 ErrEnvFailed）
)
```

---

## 6. Repository 接口

```go
type Repository interface {
    // Forge CRUD
    SaveForge(ctx context.Context, f *Forge) error
    GetForge(ctx context.Context, id string) (*Forge, error)
    GetForgesByIDs(ctx context.Context, ids []string) ([]*Forge, error) // LLM 排序后按 ID 批量拉完整对象
    ListForges(ctx context.Context, filter ListFilter) ([]*Forge, string, error)
    ListAllForges(ctx context.Context) ([]*Forge, error) // 供 search_forges 把全量 forge 发给 LLM 排序
    DeleteForge(ctx context.Context, id string) error

    // Versions（含 pending）
    SaveVersion(ctx context.Context, v *ForgeVersion) error
    GetVersion(ctx context.Context, forgeID string, version int) (*ForgeVersion, error)
    GetVersionByID(ctx context.Context, id string) (*ForgeVersion, error)            // 沙箱迭代 1 新增：按 ForgeVersion.ID 取（attachActiveEnv 用）
    GetActivePending(ctx context.Context, forgeID string) (*ForgeVersion, error)     // status='pending'
    ListAcceptedVersions(ctx context.Context, forgeID string) ([]*ForgeVersion, error) // status='accepted', version DESC
    UpdateVersionStatus(ctx context.Context, id, status string, version *int) error
    CountAcceptedVersions(ctx context.Context, forgeID string) (int64, error)
    DeleteOldestAcceptedVersion(ctx context.Context, forgeID string) error

    // Env state（沙箱迭代 1 新增 6 个方法）
    UpdateVersionEnvID(ctx context.Context, id, envID string, deps []string, pythonVersion string) error
    // 仅 status != 'accepted' 的行允许改 EnvID/deps/pythonVersion；accepted 行拒绝改（保护历史快照不可变）

    UpdateVersionEnvStatus(ctx context.Context, id, status, errorMsg string, syncedAt *time.Time) error
    // 写 EnvStatus / EnvError / EnvSyncedAt；status 转换无 DB 层校验（Service 层负责）

    UpdateVersionEnvProgress(ctx context.Context, id, stage, detail string) error
    // 仅写 EnvSyncStage / EnvSyncDetail；syncing 期间高频调用

    ListEnvIDsForForge(ctx context.Context, forgeID string) ([]string, error)
    // 返回该 forge 下所有不同 EnvID（按最近 updated_at DESC 排序），用于 trimEnvBuffer LRU 判断
    // 实现注意：modernc.org/sqlite 不能把 MAX(updated_at) 聚合列直接 scan 到 *time.Time，
    // 用 GORM Pluck 只取 env_id 列、MAX(updated_at) 仅用于 ORDER BY

    EvictEnvForVersions(ctx context.Context, forgeID, envID string) error
    // 把 (forgeID, envID) 匹配的所有 ForgeVersion 行的 EnvStatus 设为 'evicted'

    GetActiveVersion(ctx context.Context, forgeID string) (*ForgeVersion, error)
    // 通过 forge.ActiveVersionID 查 active 版本；无 active（draft 阶段）返 ErrVersionNotFound

    // Test cases
    SaveTestCase(ctx context.Context, tc *ForgeTestCase) error
    GetTestCase(ctx context.Context, id string) (*ForgeTestCase, error)
    ListTestCases(ctx context.Context, forgeID string) ([]*ForgeTestCase, error)
    DeleteTestCase(ctx context.Context, id string) error

    // Executions（Phase 5 统一表，9 个 history 方法 → 4 个）
    SaveExecution(ctx context.Context, e *ForgeExecution) error
    ListExecutions(ctx context.Context, filter ExecutionFilter) ([]*ForgeExecution, string, error)
    CountExecutions(ctx context.Context, forgeID string) (int64, error)
    DeleteOldestExecution(ctx context.Context, forgeID string) error
}

type ListFilter struct {
    Cursor string
    Limit  int
}

// ExecutionFilter 是 Repository.ListExecutions 接受的查询形状。所有字段可选；
// 空 filter 列出全部（按 ctx 用户过滤）。常用模式：
//   - {ForgeID, Limit:20}                        某 forge 最近 20 条执行
//   - {ForgeID, Kind:"test", BatchID:"..."}      一次 :test 批次的所有行
//   - {MessageID}                                 某 chat 消息触发的所有 forge 执行
//   - {ConversationID, Limit:100}                一个对话中所有执行
type ExecutionFilter struct {
    ForgeID        string
    Kind           string // "" | "run" | "test"
    BatchID        string
    TestCaseID     string
    ConversationID string
    MessageID      string
    ToolCallID     string
    Cursor         string // base64url(paginationpkg.Cursor); "" = first page
    Limit          int    // 0 → store default (50)
}
```

**ListExecutions 排序约定**：BatchID 指定时按 `created_at ASC`（单批次按运行顺序展示）；其他情况按 `created_at DESC`（最新在前）。Cursor 谓词随排序方向反转。

---

## 7. Store 实现要点

### 7.1 SQLite（GORM）

- Partial UNIQUE：`UNIQUE(user_id, name) WHERE deleted_at IS NULL`，在 `schema_extras.go` 追加
- `ListAcceptedVersions`：`WHERE tool_id=? AND status='accepted' ORDER BY version DESC`
- `GetActivePending`：`WHERE tool_id=? AND status='pending' LIMIT 1`
- `DeleteOldestAcceptedVersion`：硬删 `WHERE tool_id=? AND status='accepted' ORDER BY version ASC LIMIT 1`

### 7.2 工具搜索（LLM 排序）

搜索逻辑完全在 `SearchTool`（`app/agent/forge.go`）中实现，不在 Service 层，不依赖向量库。

**流程**：
1. `toolSvc.ListAllTools(ctx)` → 拿全部工具（仅 name + description，轻量）
2. 构建 prompt：列出所有工具 + query，要求 LLM 返回 `[{"id":"t_xxx","score":0.95},...]`
3. LLM 非流式调用（等完整 JSON）→ 解析 ID + score 列表，取前 limit 条
4. `repo.GetToolsByIDs(ids)` → 取完整 Tool 对象
5. 按 score 排序后返回

**为什么比向量搜索准确**：LLM 完整理解语义，能推理 "处理表格" → parse_csv；20-200 个工具一次 prompt 全放进去，不丢失信息；无需 embedding API，任何 provider 都支持。

---

## 8. Service 层（app/tool/tool.go）

### 8.1 Struct

```go
type Service struct {
    repo    forgedomain.Repository
    sandbox Sandbox
    llm     LLMClient            // GenerateTestCases 使用
    bridge  eventsdomain.Bridge  // optional — 仅 chat agent 路径使用；HTTP forge handler 路径传 nil 时 PublishSnapshot no-op
    log     *zap.Logger
}

// PublishSnapshot 把 forge entity-state 事件发到 bridge；create_forge / edit_forge
// 通过它推流（不直连 bridge），与 chat.runner.publishMessageSnapshot 同模式。
// bridge 为 nil 或 convID 空时静默返回。
func (s *Service) PublishSnapshot(ctx context.Context, convID string, f *forgedomain.Forge)

// Sandbox 接口（沙箱迭代 1 从 1 个方法扩到 6 个）。
// 实现：infra/sandbox.Sandbox（捆绑 uv + python-build-standalone）。
type Sandbox interface {
    // Sync 为 (forgeID, envID, deps, pyver) 创建/复用 venv。同步阻塞直到 ready 或失败。
    // ProgressFn 在 uv stderr 每行解析后回调，Service 据此更新 EnvSyncStage / EnvSyncDetail
    // 并 PublishSnapshot——前端进度条从这条信号生长。
    Sync(ctx context.Context, req SyncRequest) error

    // Run 在指定 (forgeID, envID) 的 venv 跑 code（已由 WriteCodeFile 写到 forge 目录）。
    // input 通过 stdin JSON 传；stdout = output，stderr = errorMsg。失败用 SyncError 包装。
    Run(ctx context.Context, req RunRequest) (*forgedomain.ExecutionResult, error)

    // Destroy 删整个 forge 目录（含所有 venv + code 文件）。Service.Delete 调用。
    Destroy(ctx context.Context, forgeID string) error

    // DestroyEnv 仅删指定 (forgeID, envID) 的 venv，保留 forge 目录下其他 venv 与 code 文件。
    // trimEnvBuffer LRU 淘汰时调用。
    DestroyEnv(ctx context.Context, forgeID, envID string) error

    // WriteCodeFile 把 forge code 写到 forge 目录的入口文件（main.py）。
    // Run 之前调用一次；多次调用幂等覆盖。
    WriteCodeFile(ctx context.Context, forgeID, code string) error

    // PythonPath 返回当前平台 sandbox 捆绑的 Python 解释器路径。
    // ASTParser 用此路径跑 AST 解析；与 sandbox runtime 一致，不依赖系统 Python。
    PythonPath() string
}

// SyncRequest 是 Sandbox.Sync 的输入。
type SyncRequest struct {
    ForgeID       string
    EnvID         string
    Dependencies  []string                          // PEP 508 strings
    PythonVersion string                            // PEP 440 spec
    ProgressFn    func(stage, detail string) error // uv stderr 每行回调；返 ctx.Err 中止
}

// RunRequest 是 Sandbox.Run 的输入。
type RunRequest struct {
    ForgeID string
    EnvID   string
    Input   map[string]any
}

// SyncError 包装 sandbox sync/run 错误，含 cause + 原始 stderr，便于 Service 写 EnvError。
type SyncError struct {
    Cause  error
    Stderr string
}

// ExecutionResult 定义在 domain/forge/forge.go，避免 infra/sandbox ↔ app/forge 循环依赖

// LLMClient 非流式调用，等待完整 JSON 响应。
// cmd/server/main.go 的 forgeLLMClientAdapter 通过 pkg/llmclient.Resolve 实现这个接口。
type LLMClient interface {
    Generate(ctx context.Context, prompt string) (string, error)
}

// GenerateResult 是 GenerateTestCases 同步返回的形状。
// 要么 NotSupported=true（含 Reason），要么 TestCases 含已保存的用例。
type GenerateResult struct {
    NotSupported bool                       `json:"notSupported"`
    Reason       string                     `json:"reason,omitempty"`
    TestCases    []*tooldomain.ForgeTestCase `json:"testCases,omitempty"`
}
```

### 8.1.1 attachActiveEnv（沙箱迭代 1 新增）

```go
// attachActiveEnv 在 GET / List / PublishSnapshot 调用前，把 forge.ActiveVersionID 指向的
// ForgeVersion 行的 5 个 env 字段抄到 Forge 计算字段（EnvStatus / EnvError / EnvSyncedAt /
// EnvSyncStage / EnvSyncDetail）。无 active version（draft 阶段）保持空。
//
// 与 attachPending 同模式——都是把 DB 关系表达成"展平到主 entity"的便利字段。
func (s *Service) attachActiveEnv(ctx context.Context, f *forgedomain.Forge) error
```

### 8.1.2 SyncEnvForVersion + trimEnvBuffer（沙箱迭代 1 新增）

```go
// SyncEnvForVersion 驱动一个 ForgeVersion 的 EnvStatus 状态机：
//
//   pending/evicted → syncing → (ready | failed)
//
// 流程：
//   1. UpdateVersionEnvStatus(id, "syncing", "", nil) → PublishSnapshot 首帧
//   2. 解 Dependencies / PythonVersion / EnvID
//   3. sandbox.Sync(ctx, SyncRequest{...ProgressFn: func(stage, detail) {
//          UpdateVersionEnvProgress(id, stage, detail)
//          PublishSnapshot（带最新 EnvSyncStage/Detail）
//      }})
//   4. 成功 → UpdateVersionEnvStatus(id, "ready", "", &now) → 最终快照
//      失败 → UpdateVersionEnvStatus(id, "failed", stderrTail, nil) → 错误快照
//   5. 成功后调 trimEnvBuffer(forgeID) — 超 MaxEnvIDsPerForge 时 evict 最旧 EnvID
//
// 不返 sync 失败错误——错误已写进 EnvError，调用方只关心"DB 状态已经更新到终态"。
// 真正会上抛的错误：DB I/O 失败 / context cancel。
func (s *Service) SyncEnvForVersion(ctx context.Context, versionID string) error

// trimEnvBuffer 是 LRU 淘汰：若当前 forge 不同 EnvID 数 > MaxEnvIDsPerForge，
// 把最旧 EnvID 涉及的所有 ForgeVersion 行设为 evicted + DestroyEnv 删 venv 目录。
func (s *Service) trimEnvBuffer(ctx context.Context, forgeID string) error
```

### 8.2 Input / Output 类型

```go
type CreateInput struct {
    Name          string
    Description   string
    Code          string
    Tags          []string // 可为空
    Dependencies  []string // 沙箱迭代 1：PEP 508 strings；空 = 仅 stdlib
    PythonVersion string   // 沙箱迭代 1：PEP 440 spec；空 = DefaultPythonVersion
}

type UpdateInput struct {
    Name        *string   // nil = 不改
    Description *string
    Tags        *[]string
    Code        *string   // nil = 不改代码
    // 沙箱迭代 1：UpdateInput 不接 deps 改动——deps 改走 edit_forge LLM tool / pending
    // → accept 流程。HTTP PATCH 仅改元数据。Code 改时新 ForgeVersion 继承 active 版本的 deps + pyver。
}

type TestCaseInput struct {
    Name           string
    InputData      string // JSON object string
    ExpectedOutput string // JSON string，空 = 不断言
}

type TestRunResult struct {
    TestCaseID     string
    Name           string
    Input          string // 实际执行的 input JSON
    Output         string // 实际输出 JSON
    OK             bool   // sandbox 执行是否成功
    Pass           *bool  // nil=无 expected_output；true/false=断言结果
    ErrorMsg       string
    ElapsedMs      int64
}
```

### 8.3 CRUD（沙箱迭代 1 改造）

```go
func (s *Service) Create(ctx context.Context, in CreateInput) (*forgedomain.Forge, error)
// CreateInput: {Name, Description, Code, Tags, Dependencies, PythonVersion}
// 1. fillEnvFields — pyver 兜底 DefaultPythonVersion；ComputeEnvID(deps, pyver)
// 2. ASTParser.Parse(code) — 用 sandbox.PythonPath() 跑解析；失败 → ErrASTParseError
// 3. repo.SaveForge（UNIQUE 冲突 → ErrDuplicateName）
// 4. repo.SaveVersion(status='accepted', version=1, message="initial",
//                    Dependencies=in.Deps, PythonVersion=pyver, EnvID=envID, EnvStatus="pending")
// 5. forge.VersionCount = 1，forge.ActiveVersionID = pv.ID
// 6. svc.SyncEnvForVersion(ctx, pv.ID) — 同步阻塞至 ready/failed
// 7. attachActiveEnv → 返回的 forge 含完整 envStatus/envError 等计算字段

func (s *Service) CreateDraft(ctx context.Context, in CreateInput) (*forgedomain.Forge, error)
// 沙箱迭代 1 新增：create_forge LLM tool 的 internal 入口。
// 1. 预分配 ForgeID，构建内存 stub Forge（code 空），不进 DB
// 2. 不跑 ASTParser、不跑 sandbox.Sync——纯内存对象
// 3. 调用方（CreateForge tool）逐 token 更新 stub.Code 并 PublishSnapshot
// 4. 流式结束后调 svc.Create(...) 真正落库 + sync

func (s *Service) Get(ctx context.Context, id string) (*forgedomain.Forge, error)
// attachPending + attachActiveEnv 后返回

func (s *Service) GetDetail(ctx context.Context, id string) (*ToolDetail, error)
// 供 get_forge System Tool 使用：Get + 聚合最近 test history 摘要

type ToolDetail struct {
    *forgedomain.Forge
    TestSummary TestSummary
}

type TestSummary struct {
    Total        int    // 当前测试用例总数
    LastPassRate string // 最近一次 :test 批跑的结果，格式 "3/3" | "2/3" | "" (无记录)
    LastRunAt    string // 最近一次批跑时间，ISO 8601 或 ""
}

func (s *Service) List(ctx context.Context, filter forgedomain.ListFilter) ([]*forgedomain.Forge, string, error)
// 每条结果都跑 attachPending + attachActiveEnv

func (s *Service) ListAll(ctx context.Context) ([]*forgedomain.Forge, error)
// 供 SearchForge 使用：返回当前用户全部活跃 forge（无分页），仅取 name+description 即可

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*forgedomain.Forge, error)
// UpdateInput: Name? / Description? / Tags? / Code?（用户直接操作，立即生效）
// 沙箱迭代 1 改造：Code 改时不接受 deps 改动——继承 active 版本的 deps + pyver
// 若 Code != nil:
//   1. 检查有无 active pending → 自动 reject
//   2. ASTParser.Parse(newCode) — 用 sandbox.PythonPath()
//   3. 取 active 版本的 Dependencies / PythonVersion / EnvID 继承到新版本
// 3. 更新 Forge 主表所有变更字段
// 4. forge.VersionCount++，repo.SaveVersion(status='accepted', deps/pyver/envID 继承, EnvStatus 继承)
// 5. 因 deps 不变 → 不重新 sync；ActiveVersionID = 新 pv.ID 后 attachActiveEnv 拷贝同 EnvID 行的 env 状态
// 6. 若 accepted count > MaxAcceptedVersions → DeleteOldestAcceptedVersion

func (s *Service) Delete(ctx context.Context, id string) error
// 1. repo.DeleteForge（软删）
// 2. sandbox.Destroy(forgeID) — 删整个 forge 目录（含所有 venv）；本地存储释放
//    失败仅 warn log，软删已成功
```

### 8.4 版本管理（沙箱迭代 1 改造）

```go
func (s *Service) ListVersions(ctx context.Context, forgeID string) ([]*forgedomain.ForgeVersion, error)
// repo.ListAcceptedVersions（status='accepted', version DESC）

func (s *Service) GetVersion(ctx context.Context, forgeID string, version int) (*forgedomain.ForgeVersion, error)

func (s *Service) RevertToVersion(ctx context.Context, forgeID string, version int) (*forgedomain.Forge, error)
// 1. GetVersion → 拿完整快照（name/description/code/parameters/returnSchema/tags + deps/pyver/envID/EnvStatus）
// 2. 检查有无 active pending → 自动 reject
// 3. 更新 Forge 主表为快照内容；ActiveVersionID = 目标版本 ID
// 4. forge.VersionCount++，SaveVersion(status='accepted', version=VersionCount, message="reverted to v{N}",
//                                       继承目标版本的 deps/pyver/envID/EnvStatus）
// 5. 若 accepted count > MaxAcceptedVersions → DeleteOldestAcceptedVersion
// 沙箱迭代 1 关键：检测目标版本 EnvStatus == "evicted" → 触发 SyncEnvForVersion 重建 venv
//                  其他 EnvStatus（ready/failed/pending）保持原状，由后续 :run 时再触发；
//                  EnvStatus 继承让 evict→revert 闭环干净
```

### 8.5 Pending 管理（沙箱迭代 1 改造）

```go
func (s *Service) GetActivePending(ctx context.Context, forgeID string) (*forgedomain.ForgeVersion, error)
// repo.GetActivePending → ErrPendingNotFound if nil

func (s *Service) CreatePending(ctx context.Context, forgeID string, snap PendingSnap) (*forgedomain.ForgeVersion, error)
// 沙箱迭代 1：edit_forge LLM tool 调用入口。
// PendingSnap: {Name, Description, Code, Tags, Dependencies, PythonVersion, ChangeReason, ID（预分配）}
// 1. 拒绝已有 active pending → ErrPendingConflict
// 2. fillEnvFields — pyver 兜底；ComputeEnvID
// 3. ASTParser.Parse(code) — 用 sandbox.PythonPath()
// 4. deps 继承策略：snap.Dependencies 显式提供时用 snap，否则继承 active 版本
// 5. repo.SaveVersion(status='pending', EnvStatus='pending')
// 6. svc.SyncEnvForVersion(ctx, pv.ID) — 同步阻塞至 ready/failed
//    （即便 deps 没变，新 ForgeVersion 也是新 envID 数据点，但 sandbox.Sync 内部会通过 EnvID
//    复用已存在的 venv 目录，不重新装包；同 deps 的 sync 是秒级返回）
// 7. 返回 pv（含 envStatus 终态）

func (s *Service) AcceptPending(ctx context.Context, forgeID string) (*forgedomain.Forge, error)
// 1. repo.GetActivePending(forgeID) → ErrPendingNotFound if none
// 2. 沙箱迭代 1 守卫：检查 pv.EnvStatus
//    - "ready" → 放行
//    - "failed" → ErrEnvFailed (handler 422 FORGE_ENV_FAILED)
//    - 其他（pending/syncing/evicted）→ ErrEnvNotReady (handler 422 FORGE_ENV_NOT_READY)
// 3. 分配 version = forge.VersionCount + 1
// 4. 更新 Forge 主表为 pending 快照；ActiveVersionID = pv.ID
// 5. forge.VersionCount = version
// 6. repo.UpdateVersionStatus(pv.ID, 'accepted', &version)
// 7. 若 accepted count > MaxAcceptedVersions → DeleteOldestAcceptedVersion

func (s *Service) RejectPending(ctx context.Context, forgeID string) error
// 1. repo.GetActivePending(forgeID) → UpdateVersionStatus(pv.ID, 'rejected', nil)
// 2. 重读 Forge：若 ActiveVersionID == ""（即 create_forge 首份代码被拒，
//    forge 从未有过 accepted 版本）→ 调用 svc.Delete(ctx, forgeID) 把整个
//    forge 一并删掉（含 sandbox.Destroy 清 venv 目录）。否则只把 pending 标
//    rejected，forge 保留之前的 active 代码。
// 沙箱迭代 1：rejected 不立刻销毁 venv（同 EnvID 可能其他 ForgeVersion 在用）；
//             trimEnvBuffer LRU 时再清理。但"draft 首拒"场景走 Delete 整体清。
```

### 8.6 执行（沙箱迭代 1 改造）

```go
func (s *Service) RunForge(ctx context.Context, forgeID string, input map[string]any) (*forgedomain.ExecutionResult, error)
// input 已由调用方预处理（att_id 解析在 RunForge System Tool 内完成；HTTP 调用者直接传真实路径）
// 沙箱迭代 1：通过 ActiveVersionID 找到 active ForgeVersion，从中取 EnvID
// 1. repo.GetForge(forgeID) → forge
// 2. forge.ActiveVersionID == "" → ErrVersionNotFound（draft 阶段不能运行）
// 3. repo.GetVersionByID(forge.ActiveVersionID) → activeVersion
// 4. 守卫 activeVersion.EnvStatus：
//    - "ready" → 放行
//    - "evicted" → 触发 SyncEnvForVersion 重建后再 run；
//                  返回 ErrEnvNotReady 给调用方让其下一帧再 run（非阻塞策略；同步 Sync 会延迟）
//      实现选择：当前是同步 Sync 然后继续 run（用户/AI 等同一次 run 完成）
//    - "failed" → ErrEnvFailed（handler 422）
//    - 其他 → ErrEnvNotReady（handler 422）
// 5. sandbox.WriteCodeFile(forgeID, forge.Code) — 入口文件可能因 revert/edit 改了，先同步
// 6. sandbox.Run(RunRequest{ForgeID, EnvID: activeVersion.EnvID, Input}) — 无超时
// 7. 写 ForgeExecution（kind="run"，无论成功失败；记录 forge 当前 VersionCount + 触发上下文）
// 8. 若 count > MaxExecutionsPerForge → DeleteOldestExecution
```

### 8.7 测试用例（沙箱迭代 1 改造）

```go
func (s *Service) CreateTestCase(ctx context.Context, forgeID string, in TestCaseInput) (*forgedomain.ForgeTestCase, error)
func (s *Service) ListTestCases(ctx context.Context, forgeID string) ([]*forgedomain.ForgeTestCase, error)
func (s *Service) DeleteTestCase(ctx context.Context, id string) error

func (s *Service) RunTestCase(ctx context.Context, testCaseID string, batchID string) (*TestRunResult, error)
// 沙箱迭代 1：与 RunForge 同样路径——ActiveVersionID → EnvID → sandbox.Run
// + 若 ExpectedOutput != "" 则断言 pass/fail
// 写 ForgeExecution（kind="test"）

func (s *Service) RunAllTests(ctx context.Context, forgeID string) ([]*TestRunResult, error)
// 生成 batchID → 逐条 RunTestCase(id, batchID) → 汇总返回

func (s *Service) GenerateTestCases(ctx context.Context, forgeID string, count int) (*GenerateResult, error)
// 1. GetForge → code + parameters + returnSchema
// 2. llm.Generate(ctx, prompt) — 等完整 JSON
//    prompt：分析函数，若依赖外部状态输出 {"not_supported":true,"reason":"..."}
//            否则输出 {"test_cases":[{name,input,expected_output},...]}
// 3. 解析结果：
//    not_supported → return &GenerateResult{NotSupported:true, Reason:...}
//    test_cases    → 逐条 SaveTestCase 累积 → return &GenerateResult{TestCases:saved}
// 注意：追加到现有测试集
```

### 8.8 导入导出

```go
func (s *Service) Export(ctx context.Context, toolID string) ([]byte, error)
// JSON: {name, description, code, tags, testCases:[]}

func (s *Service) Import(ctx context.Context, data []byte) (*tooldomain.Tool, error)
// 解析 → Create → 若有 testCases 则 CreateTestCase
```

### 8.9 AST 解析（私有，app/forge/ast.go；沙箱迭代 1 改造为 struct 形态）

```go
type ParsedCode struct {
    FuncName   string
    Parameters []ParsedParam
    Return     ParsedReturn
    Docstring  string
}

type ParsedParam struct {
    Name        string
    Type        string
    Required    bool    // true = 无默认值
    Description string  // Google-style docstring Args: 段
    Default     *string
}

type ParsedReturn struct {
    Type        string // 返回类型注解
    Description string // Google-style docstring Returns: 段
}

// ASTParser 用 sandbox 捆绑的 Python 解析 forge code。
// 沙箱迭代 1：从函数 parseForgeCode 改为 struct，pythonPath 在装配时从 sandbox.PythonPath() 取
// —— 不依赖系统 Python，与 sandbox runtime 一致。
//
// 要求 Google-style docstring；Description 字段解析失败时为空字符串，不报错。
type ASTParser struct {
    pythonPath string
}

func NewASTParser(pythonPath string) *ASTParser

func (p *ASTParser) Parse(ctx context.Context, code string) (*ParsedCode, error)
// 启动 Python subprocess（p.pythonPath）解析代码结构。
// pythonPath 为空（开发期 sandbox bootstrap 失败）→ 仍尝试用系统 python3 兜底，便于 dev 流程不卡。
```

---

## 9. 文件交互（att_id 解析）

`RunTool`（System Tool，`app/agent/forge.go`）在调用 `toolSvc.RunTool` 前做 att_id 解析：

```go
// resolveAttachments 遍历 input 所有 string 值，
// 若以 "att_" 开头则查 chat_attachments 表，替换为绝对路径。
func resolveAttachments(ctx context.Context, attachRepo chatdomain.Repository, input map[string]any) (map[string]any, error)
```

HTTP 直接调用 `:run` 的用户传真实文件路径，不需要解析。

---

## 10. System Tools（`app/tool/forge/` 嵌套子包）

5 个 forge system tool 各自一个文件（Phase 3 后优化轮重组），实现 `app/tool.Tool` 接口（10 方法全必填，详见 [`CLAUDE.md §S18`](../../../CLAUDE.md)）。包别名 `forgetool`（§S13 嵌套子包规则 `<sub><parent>`）。

```
internal/app/tool/forge/
├── forge.go        ← ForgeTools() 工厂 + 共享 helpers（streamCode / extractCode / resolveAttachments / prompt builders）
├── search.go       ← SearchForge struct
├── get.go          ← GetForge struct
├── create.go       ← CreateForge struct
├── edit.go         ← EditForge struct
└── run.go          ← RunForge struct
```

```go
package forge

func ForgeTools(
    svc        *forgeapp.Service,
    attachRepo chatdomain.Repository,
    picker     modeldomain.ModelPicker,
    keys       apikeydomain.KeyProvider,
    factory    *llminfra.Factory,
) []toolapp.Tool
// 返回 5 个 forge system tool（实现 toolapp.Tool 接口的 10 方法）。
// 注：bridge 不再传给 ForgeTools——CreateForge / EditForge 通过
// svc.PublishSnapshot(ctx, convID, *Forge) 推 forge entity-state 事件，
// bridge 装配在 forgeapp.Service 上（与 chat.runner.publishMessageSnapshot 同模式）。
```

**统一注入 / 钩子链**（每个 forge tool 都走）：framework 自动注入 `summary` (必填) + `destructive` (可选) 字段进 Parameters；`runOneTool` 在 Execute 前跑 `ValidateInput` + `CheckPermissions`；推流时 tool 调 `t.svc.PublishSnapshot(ctx, convID, forge)`（不直连 bridge），从 `pkg/reqctx` 读 convID/msgID/toolCallID。

**LLM 客户端解析**：`streamCode` / `search_forges` 通过 `pkg/llmclient.Resolve(ctx, picker, keys, factory) (*Bundle, error)` 取 `Bundle{Client, ModelID, Key, BaseURL}`，不再用本地 `buildClient` / `builtClient` helper（已删除）；JSON 提取通过 `pkg/llmparse.ExtractJSON(s) (string, bool)`，不再用本地 `extractJSON` / `isLikelyJSON`。

| Tool | IsReadOnly | IsConcurrencySafe | 推 SSE | 备注 |
|---|---|---|---|---|
| SearchForge | true | true | — | LLM 排序，并发安全 |
| GetForge | true | true | — | 单条查询，并发安全 |
| CreateForge | false | false | `forge`（draft → pending → accepted 全程多帧）| 沙箱迭代 1：draft→pending→user accept；schema 加 dependencies + python_version；tool_result 含 env_status / env_error |
| EditForge | false | false | `forge`（draft pending → DB pending 全程多帧）| 沙箱迭代 1：拒绝已有 pending；schema 加 dependencies + python_version；tool_result 含 env_status / env_error |
| RunForge | false | false | — | sandbox 执行；输出 50KB 截断 |

### search_forges

```
参数：{ "query": string, "limit"?: int（默认 3，最大 5）}
返回：[{
  id, name, description,
  parameters: [{name, type, required, description, default}],
  returnSchema: {type, description},
  score: float   // LLM 给出的相关度评分 0~1（注意：原 "similarity" 在 Phase 3 改名 score——更诚实，不是向量 cosine）
}]

实现（SearchForge 内部）：
  1. svc.ListAll(ctx) → 全部 forge（name + description）
  2. llm.Generate(ctx, rankPrompt) → "[{\"id\":\"f_xxx\",\"score\":0.95},...]"
     rankPrompt：列出所有 forge + query，要求返回最相关的 limit 个 ID+score
  3. extractJSON 兼容 markdown fence 优先（` ```json ... ``` `）+ bracket fallback
  4. svc.GetForgesByIDs(ids) → 完整 Forge 对象
  5. 组装返回，score 填入

LLM 使用指引：
- score >= 0.8：高度相关，可直接 get_forge 确认后使用
- score 0.5~0.8：可能相关，建议 get_forge 读代码判断
- 返回空或全部低分：forge 库无合适工具，考虑 create_forge
```

### get_forge

```
参数：{ "forge_id": string }
返回：{
  id, name, description, code,
  parameters, returnSchema, tags, versionCount,
  testSummary: { total, lastPassRate, lastRunAt }
}
实现：svc.GetDetail(forge_id)
说明：LLM 在 search_forges 拿到候选后，对不确定的 forge 调此接口读完整代码再决定是否使用
```

### create_forge（沙箱迭代 1 改造）

```
参数：{
  "name": string,
  "description": string,
  "instruction": string,
  "dependencies"?: string[],   // 沙箱迭代 1 新增：PEP 508 strings；空/不传 = 仅 stdlib
  "python_version"?: string,   // 沙箱迭代 1 新增：PEP 440 spec；不传 = DefaultPythonVersion (">=3.12")
}
返回：{
  "forge_id": string,
  "name": string,
  "parameters": [...],
  "env_status": string,        // 沙箱迭代 1：终态 ready / failed
  "env_error"?: string,        // 沙箱迭代 1：env_status="failed" 时填 uv stderr tail
}
流程（沙箱迭代 1：draft → pending → user accept）：
  1. svc.CreateDraft(name, description) — 内存 stub，预分配 forgeID + pendingID
     PublishSnapshot 首帧（draft Forge 含空的 Pending）
  2. streamCode(createPrompt + instruction) → 逐 token 更新 stub.Pending.Code 并 PublishSnapshot
  3. ASTParser.Parse(code) — 失败 → 错误 tool_result，LLM 重试
  4. svc.CreatePending(forgeID, PendingSnap{ID:pendingID, Code, Deps, Pyver, ...})
     CreatePending 内部调 SyncEnvForVersion 同步阻塞至 ready/failed，每条 uv stderr 解析后
     UpdateVersionEnvProgress + PublishSnapshot
  5. tool_result 含 env_status / env_error
  6. LLM 看到结果后决定下一步：
     - env_status=ready → 通常引导用户 accept_pending
     - env_status=failed → 调 edit_forge 改 deps 重试（重新产生 pending）
```

### edit_forge（沙箱迭代 1 改造）

```
参数：{
  "forge_id": string,
  "instruction"?: string,        // 有 → LLM 生成新代码（流式）；无 → 仅改元数据
  "name"?: string,
  "description"?: string,
  "dependencies"?: string[],     // 沙箱迭代 1 新增：PEP 508 strings；不传 = 继承 active 版本
  "python_version"?: string,     // 沙箱迭代 1 新增：PEP 440 spec；不传 = 继承 active 版本
}
// instruction 和其余字段至少提供一个

返回：{
  "pending_id": string,
  "forge_id": string,
  "env_status": string,          // 沙箱迭代 1：终态 ready / failed
  "env_error"?: string,          // 沙箱迭代 1：env_status="failed" 时填
}
流程（沙箱迭代 1）：
  1. 守卫：repo.GetActivePending(forge_id) != nil → 错误 tool_result（含已有 pending 信息），
     提示 LLM 让用户先 accept/reject 当前 pending 再编辑。**统一入口**——无论 forge 处于
     draft 还是已 active 状态，edit_forge 都按此守卫。
  2. 若有 instruction：
     a. 预分配 pendingID = forgeapp.NewVersionID()，构建 draftPending 挂到 forge.Pending
        PublishSnapshot 首帧
     b. svc.Get(forge_id) → 当前 code
     c. streamCode(editPrompt + currentCode + instruction) → 逐 token 更新
        forge.Pending.Code 并 PublishSnapshot（draft pending 在内存增长，未落库）
     d. ASTParser.Parse(newCode) — AST dry-run，失败 → 错误 tool_result
  3. 仅元数据：跳过 streamCode；snap.Code = forge.Code 不变
  4. svc.CreatePending(forge_id, PendingSnap{ID:pendingID, Code, Deps, Pyver, ...})
     CreatePending 内部调 SyncEnvForVersion；若 deps 与 active 同 → sandbox.Sync 内部命中
     已存在的 venv 目录秒级返回 ready；变了 → 跑完整 uv pip install
  5. tool_result 含 env_status / env_error；LLM 同 create_forge 路径决策下一步
```

### run_forge

```
参数：{ "forge_id": string, "input": object }
返回：{ "ok": bool, "output": any, "error"?: string, "elapsed_ms": int }
流程（沙箱迭代 1）：
  1. resolveAttachments(ctx, input) — 顶层 string 字段 "att_xxx" → storage path
  2. svc.RunForge(ctx, forge_id, resolvedInput) — 内部按 ActiveVersionID 取 EnvID 跑 sandbox.Run
     - active 版本 EnvStatus=evicted → 同步 sync 后再 run（用户/AI 等同一次 run 完成）
     - EnvStatus=failed/pending/syncing → 错误 tool_result（含 env_error）
  3. 输出 JSON 编码后超 50KB 截断为 notice 字符串
注意：sandbox 执行失败（forge code 异常）返回 ok=false，不是 Go error；env 不就绪是 Go error
```

---

## 11. HTTP API（21 个端点，get_forge 仅为 System Tool，无对应 HTTP 端点）

| Method | Path | 用途 | 状态码 |
|---|---|---|---|
| POST | `/api/v1/forges` | 创建（直接传 code，不走 LLM）| 201 |
| GET | `/api/v1/forges` | 列表（分页；响应每个 forge 含 `pending` 字段）| 200 |
| GET | `/api/v1/forges/{id}` | 详情（响应含 `pending` 字段）| 200 |
| PATCH | `/api/v1/forges/{id}` | 更新（直接生效，任意字段）| 200 |
| DELETE | `/api/v1/forges/{id}` | 软删 | 204 |
| POST | `/api/v1/forges/{id}:run` | 执行 forge | 200 |
| POST | `/api/v1/forges/{id}:export` | 导出 JSON | 200 |
| POST | `/api/v1/forges:import` | 导入 JSON | 201 |
| GET | `/api/v1/forges/{id}/versions` | accepted 版本列表 | 200 |
| GET | `/api/v1/forges/{id}/versions/{version}` | 单版本详情（含完整快照）| 200 |
| POST | `/api/v1/forges/{id}:revert` | 回滚到指定版本 | 200 |
| GET | `/api/v1/forges/{id}/pending` | 当前 pending（无则 404）| 200/404 |
| POST | `/api/v1/forges/{id}/pending:accept` | 接受 | 200 |
| POST | `/api/v1/forges/{id}/pending:reject` | 拒绝 | 204 |
| GET | `/api/v1/forges/{id}/test-cases` | 测试用例列表 | 200 |
| POST | `/api/v1/forges/{id}/test-cases` | 创建测试用例 | 201 |
| DELETE | `/api/v1/forges/{id}/test-cases/{tcId}` | 删除测试用例 | 204 |
| POST | `/api/v1/forges/{id}/test-cases/{tcId}:run` | 运行单个测试用例 | 200 |
| POST | `/api/v1/forges/{id}:test` | 运行全部测试用例 | 200 |
| POST | `/api/v1/forges/{id}:generate-test-cases` | LLM 生成测试用例（一次性返回 JSON 批量）| 200 |
| GET | `/api/v1/forges/{id}/executions` | 执行历史（统一端点，`?kind=run\|test &batchId=&cursor=&limit=` 过滤；分页 envelope；Phase 5 替代 run-history + test-history）| 200 |

**关键说明**：
- `POST /forges` 和 `PATCH /forges/{id}` 是用户直接操作，立即生效，创建 accepted version
- `edit_forge`（System Tool）是 LLM 发起的变更，统一走 pending，用户审核后生效
- `:run` 执行失败是业务结果（200 + `ok:false`），不是 HTTP 错误

**沙箱迭代 1（2026-05-03）改动**：

`POST /forges` 请求体新增可选字段：
```json
{
  "name": "...", "description": "...", "code": "...", "tags": [...],
  "dependencies": ["pandas>=2.0", "..."],   // 沙箱迭代 1：PEP 508 strings；空/不传 = 仅 stdlib
  "pythonVersion": ">=3.12"                  // 沙箱迭代 1：PEP 440 spec；不传 = DefaultPythonVersion
}
```
service 同步等 `sandbox.Sync` 完成才返；响应的 forge 对象计算字段含 `envStatus` / `envError` / `envSyncedAt` / `envSyncStage` / `envSyncDetail` / `activeVersionId`（attachActiveEnv 填充）。

`PATCH /forges/{id}` **不接 deps 改动** —— deps 改走 `edit_forge` LLM tool / pending → accept 流程。HTTP PATCH 仅改元数据 + 可选 code（继承 active 版本 deps）。

`POST /forges/{id}/pending:accept` 沙箱迭代 1 守卫 pending 的 EnvStatus：
- `ready` → 放行（200）
- `failed` → 422 `FORGE_ENV_FAILED`
- 其他（pending / syncing / evicted）→ 422 `FORGE_ENV_NOT_READY`

`POST /forges/{id}:revert` 自动检测目标版本 `EnvStatus="evicted"` → 触发同步 sync 重建后再返；其他状态保持原状由后续 :run 时再触发。

---

## 12. 错误码

| Code | HTTP | Sentinel | 场景 |
|---|---|---|---|
| `TOOL_NOT_FOUND` | 404 | `ErrNotFound` | id 查不到 |
| `TOOL_NAME_DUPLICATE` | 409 | `ErrDuplicateName` | 创建/改名撞名 |
| `TOOL_VERSION_NOT_FOUND` | 404 | `ErrVersionNotFound` | revert / get version 时版本不存在 |
| `TOOL_PENDING_NOT_FOUND` | 404 | `ErrPendingNotFound` | accept/reject 时无 pending |
| `TOOL_PENDING_CONFLICT` | 409 | `ErrPendingConflict` | edit_forge 时已有未处理 pending |
| `TOOL_TEST_CASE_NOT_FOUND` | 404 | `ErrTestCaseNotFound` | 测试用例找不到 |
| `TOOL_RUN_FAILED` | 422 | `ErrRunFailed` | sandbox 内部错误（≠ 执行失败，执行失败是 ok=false）|
| `TOOL_AST_PARSE_FAILED` | 422 | `ErrASTParseError` | 代码无法被 Python AST 解析 |
| `TOOL_IMPORT_INVALID` | 400 | `ErrImportInvalid` | 导入 JSON 格式错误 |
| `FORGE_ENV_NOT_READY` | 422 | `ErrEnvNotReady` | 沙箱迭代 1：accept_pending 时 EnvStatus 不是 ready（pending/syncing/evicted）；run_forge 时同样 |
| `FORGE_ENV_FAILED` | 422 | `ErrEnvFailed` | 沙箱迭代 1：EnvStatus = failed；调用方该看 envError 决定是改 deps 重建还是放弃 |
| `FORGE_SANDBOX_UNAVAILABLE` | 500 | `ErrSandboxUnavailable` | 沙箱迭代 1：sandbox.Bootstrap 没成功（资源缺失 / 解压失败 / 重签失败）|
| `FORGE_DEPENDENCY_RESOLUTION` | 422 | `ErrDependencyResolution` | 沙箱迭代 1：pyproject 渲染或 PEP 508 校验阶段就报错（罕见，多数错误归 FORGE_ENV_FAILED）|

**TOOL_* vs FORGE_***：旧 TOOL_* 代码字符串前缀（domain 重命名前的客户端约定）保留作 wire-compat，新增的 env 相关错误用 FORGE_* 前缀。errmap 中两套同时存在。

---

## 13. SSE 事件（Phase 6 重构 · entity-state 模型）

forge domain 现只用 **1 个 SSE 事件 `forge`**——载荷 = 完整 Forge 实体（含 `pending` 字段）的 GET 形状。详见 [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md)。

```go
// Forge carries a full Forge snapshot, including the .Pending field when a
// pending change exists.
type Forge struct {
    *forgedomain.Forge
}

func (Forge) EventName() string { return "forge" }

// MarshalJSON 委托给嵌入的 *forgedomain.Forge——wire shape 严格 = GET /api/v1/forges/{id}。
func (e Forge) MarshalJSON() ([]byte, error) {
    if e.Forge == nil {
        return []byte("null"), nil
    }
    return json.Marshal(e.Forge)
}
```

### 触发点

| 触发场景 | 时机 |
|---|---|
| `create_forge` 进入 | 预分配 `forgeID = forgeapp.NewForgeID()`，构建 stub Forge（code 空），发首帧快照 |
| `create_forge` 流式 | LLM 每 token，更新 stub `Code` 并发快照 |
| `create_forge` 完成 | `svc.Create(ID=forgeID)` 落库后发最终快照（含 parsed parameters / version_count=1 / activeVersionId）|
| `edit_forge` 进入（含 instruction）| 预分配 `pendingID = forgeapp.NewVersionID()`，构建 draft pending 挂在 `Forge.Pending`，发首帧快照 |
| `edit_forge` 流式（含 instruction）| LLM 每 token，更新 `Pending.Code` 并发快照 |
| `edit_forge` 完成（含 instruction）| `svc.CreatePending(ID=pendingID)` 落库后发最终快照 |
| `edit_forge` 仅元数据 | 无流式；`svc.CreatePending` 落库后发一次最终快照 |
| **沙箱迭代 1：EnvStatus 状态转换** | `pending → syncing` / `syncing → ready` / `syncing → failed` / `ready → evicted` / `evicted → syncing → ready` 每次 `UpdateVersionEnvStatus` 后立刻发快照——前端徽章颜色随之变 |
| **沙箱迭代 1：uv stderr 进度** | `SyncEnvForVersion` 把 `ProgressFn` 注入 `sandbox.Sync`，每行 uv stderr 解析后调 `UpdateVersionEnvProgress(stage, detail)` + PublishSnapshot；前端进度条随 `envSyncStage` (resolving/preparing/installing) 与 `envSyncDetail` 文字生长 |
| accept_pending / reject_pending | （**TODO** Phase 6+）：HTTP handler 在状态变更后调 bridge.Publish |
| HTTP CRUD（POST/PATCH/DELETE）| **MVP 暂不广播**——单用户单窗口；多窗口同步留待后续 |

### 失败时的语义

**LLM 流失败**：stub / draft pending **从不落库直到 LLM 流成功 + AST 验证通过**。订阅方观察到的最后一帧是错误前的部分快照，但 DB 没有对应行——前端可显示"创建/编辑失败"，并在下次 list/refresh 时清掉。

**沙箱迭代 1：sync 失败**：与 LLM 流失败不同——sync 失败时 ForgeVersion 行**已落库**（status='pending', envStatus='failed', envError 含 stderr）。订阅方收到的最后一帧 forge 快照含 `pending.envStatus='failed'` + `pending.envError`，前端 forge 面板显示错误原因，引导用户操作（手工 reject pending / 让 AI 调 edit_forge 改 deps 重试）。

---

## 14. 端到端调用链

### 链 1：LLM 创建工具（沙箱迭代 1）

```
用户："帮我写一个用 pandas 分析 CSV 的工具"
  → LLM 调 create_forge({name, description, instruction, dependencies:["pandas>=2.0"]})
  → CreateForge.Execute
      → svc.CreateDraft → 预分配 forgeID/pendingID，stub Forge 进内存
      → PublishSnapshot 首帧（forge entity，empty code）
      → llm.Stream → 逐 token 更新 stub.Pending.Code + PublishSnapshot
      → ASTParser.Parse(code) → 通过
      → svc.CreatePending(forgeID, snap{deps, pyver})
          → SyncEnvForVersion
              → UpdateVersionEnvStatus(syncing) + PublishSnapshot
              → sandbox.Sync(SyncRequest{...ProgressFn:onLine})
                  → uv stderr 每行 → onLine(stage, detail)
                      → UpdateVersionEnvProgress + PublishSnapshot（多帧）
              → UpdateVersionEnvStatus(ready, syncedAt) + PublishSnapshot 终帧
              → trimEnvBuffer（若 EnvID 数 > 3 → DestroyEnv 最旧）
      → tool_result {forge_id, name, parameters, env_status:"ready"}
```

### 链 2：LLM 编辑工具（沙箱迭代 1，新增 deps）

```
用户："给 csv_parser 加上 numpy 支持"
  → LLM 调 edit_forge({forge_id, instruction:"...", dependencies:["pandas>=2.0", "numpy"]})
  → EditForge.Execute
      → 守卫：当前无 active pending（否则错误 tool_result）
      → 预分配 pendingID，构建 draft pending 挂到 forge.Pending + PublishSnapshot 首帧
      → llm.Stream → 逐 token 更新 forge.Pending.Code + PublishSnapshot
      → ASTParser.Parse(code) → 通过
      → svc.CreatePending(forgeID, snap{deps, pyver})
          → SyncEnvForVersion → sandbox.Sync（新 envID 首次装 → 完整跑 uv pip install）
          → 多帧 progress → ready
      → tool_result {pending_id, env_status:"ready"}
```

### 链 3：用户接受 pending（沙箱迭代 1：ENV 守卫）

```
POST /api/v1/forges/f_xxx/pending:accept
  → forgeSvc.AcceptPending
      → repo.GetActivePending → pv
      → 沙箱迭代 1 守卫：
          - pv.EnvStatus="ready" → 放行
          - pv.EnvStatus="failed" → 422 FORGE_ENV_FAILED（含 envError）
          - 其他 → 422 FORGE_ENV_NOT_READY
      → 分配 version = VersionCount + 1
      → 更新 Forge 主表 → ActiveVersionID = pv.ID
      → UpdateVersionStatus(pv.ID, 'accepted', &version)
  → 200 updatedForge（含 envStatus、envSyncedAt 等计算字段）
```

### 链 4：LLM 搜索并执行工具（沙箱迭代 1：通过 ActiveVersionID 取 EnvID）

```
用户："帮我处理这段 CSV"
  → LLM 调 search_forges({query:"csv"}) → [{id, name, parameters, returnSchema, score:0.91}]
  → LLM 调 run_forge({forge_id, input:{csv_text:"..."}})
  → RunForge.Execute
      → resolveAttachments
      → forgeSvc.RunForge
          → forge.ActiveVersionID → repo.GetVersionByID → activeVersion
          → 守卫 activeVersion.EnvStatus（ready 放行 / evicted 触发 sync 再 run）
          → sandbox.WriteCodeFile(forgeID, forge.Code) — 入口文件同步
          → sandbox.Run(RunRequest{ForgeID, EnvID:activeVersion.EnvID, Input}) — 无超时
          → 写 ForgeExecution(kind="run", forge_version=N)
      → return {ok:true, output:[...], elapsed_ms:35}
```

### 链 5：LLM 执行工具处理附件

```
用户上传 report.csv → att_abc123
用户："用工具处理这个文件"
  → LLM 调 run_forge({forge_id, input:{file_path:"att_abc123"}})
  → RunForge.Execute
      → resolveAttachments → 查 chat_attachments → {file_path:"/data/.../original.csv"}
      → forgeSvc.RunForge → 同链 4 路径
```

### 链 7：用户 revert 到 evicted 版本（沙箱迭代 1 新增）

```
用户在 testend 看到 v3 是 evicted 状态，想回到 v3
POST /api/v1/forges/f_xxx:revert  body={version:3}
  → forgeSvc.RevertToVersion
      → repo.GetVersion(forgeID, 3) → targetVersion (EnvStatus="evicted")
      → 检查 active pending → 自动 reject
      → 分配 version = VersionCount + 1
      → 更新 Forge 主表（继承 v3 的 deps/pyver/envID/EnvStatus="evicted"）
      → SaveVersion(status='accepted', ChangeReason="reverted to v3", EnvStatus="evicted")
      → 检测 EnvStatus=="evicted" → SyncEnvForVersion 重建 venv
          → 多帧 progress → ready（同 deps 命中已存在的 venv 目录则秒级 ready）
  → 200 updatedForge（envStatus="ready"）
```

### 链 6：用户点击"AI 生成测试用例"

```
POST /api/v1/forges/t_xxx:generate-test-cases  (200 JSON)
  → handler 调 toolSvc.GenerateTestCases(ctx, id, 5)
      → llm.Generate(prompt) — 等完整 JSON

      情况 A（可测）：
        → 逐条 SaveTestCase 累积进 saved
        → return &GenerateResult{TestCases:saved}

      情况 B（不可测，如依赖文件路径）：
        → return &GenerateResult{NotSupported:true, Reason:"..."}

  ← 200 envelope: {data: {testCases: [...]} 或 {notSupported:true, reason:"..."}}
```

---

## 15. 数据库表总览

| 表 | 主键前缀 | 说明 |
|---|---|---|
| `forges` | `f_` | 主实体，当前 active 状态。沙箱迭代 1 新增列 `active_version_id` |
| `forge_versions` | `fv_` | 版本历史 + pending 变更（status 字段区分），accepted 最多保留 50 条。沙箱迭代 1 新增 8 列：dependencies / python_version / env_id（带 index）/ env_status / env_error / env_synced_at / env_sync_stage / env_sync_detail |
| `forge_test_cases` | `tc_` | 测试用例定义 |
| `forge_executions` | `fe_` | Phase 5 统一表（合并原 run_history + test_history），最多 300 条/forge |

`schema_extras.go` 追加：`UNIQUE(user_id, name) WHERE deleted_at IS NULL`（forges 表）

LLM-排序搜索不依赖向量库，无 chromem-go 索引。

---

## 16. infra/sandbox（沙箱迭代 1 完全重写）

详见 [`../adhoc-topic-documents/sandbox-iteration-documents/01-uv-bundled-python-per-forge-venv.md`](../adhoc-topic-documents/sandbox-iteration-documents/01-uv-bundled-python-per-forge-venv.md)。

### 16.1 包文件结构

```
internal/infra/sandbox/
├── sandbox.go        ← Config{DataDir, DefaultPython, Logger} + Sandbox struct + withUVEnv
├── paths.go          ← ComputeEnvID + normalizeSpecifier + bundledPythonPath / UVPath + forgeMutexMap
├── pyproject.go      ← renderPyproject（PEP 621 格式 + strconv.Quote 转义）
├── progress.go       ← parseUVLine（识别 resolving/preparing/installing 三阶段）+ scanProgress
├── preflight.go      ← Bootstrap（解压 uv tarball + python tar.gz + macOS xattr+codesign 重签）
├── sync.go           ← Sync + SyncRequest + SyncError{Cause, Stderr} + writeAtomic（pyproject 原子写）
├── run.go            ← Run + RunRequest + WriteCodeFile + buildDriver + extractFuncName
├── destroy.go        ← Destroy + DestroyEnv（per-forge mutex 防并发竞态）
├── proc_unix.go      ← Setpgid + Kill(-pid, SIGKILL) — 进程组 kill
└── proc_windows.go   ← taskkill /T /F /PID — 进程树 kill
```

### 16.2 Sandbox 构造与 Bootstrap

```go
type Config struct {
    DataDir       string       // 数据根目录（如 ~/Library/Application Support/Forgify/sandbox）
    DefaultPython string       // 兜底 Python 路径（资源缺失时用系统 python3，仅 dev）
    Logger        *zap.Logger
}

type Sandbox struct {
    cfg     Config
    bootHash string // .bootstrap-hash 文件内容；幂等判断
    log     *zap.Logger
}

func New(cfg Config) (*Sandbox, error)

// Bootstrap 解压资源 + macOS 重签 + 写 .bootstrap-hash。
// 资源目录从 $FORGIFY_DEV_RESOURCES（dev）或 cmd/desktop embed.FS（prod）取。
// 已 bootstrap 过（hash 一致）→ 跳过；变了 → 全部重做。
//
// macOS 关键步骤：解压完后 xattr -dr com.apple.provenance + codesign --force --sign -
// 重签整个 python 目录——绕过 issue uv#16726 的内核 SIGKILL。
func (s *Sandbox) Bootstrap(ctx context.Context) error
```

### 16.3 6 个核心方法

详见 §8.1 中 Sandbox 接口定义。Service 层通过这 6 个方法驱动整个 forge env 生命周期：

| 方法 | 何时调 | 关键行为 |
|---|---|---|
| `Sync` | `forgeapp.Service.SyncEnvForVersion` | 启 uv subprocess，stderr 实时 callback；同 EnvID 命中已 ready venv 秒级返；新装包按 deps 跑完整 `uv pip install`；失败包装 `SyncError{Cause, Stderr}` |
| `Run` | `forgeapp.Service.RunForge` / `RunTestCase` | exec venv 里的 `python3 main.py`；stdin 喂 input JSON；stdout = output JSON；stderr = errorMsg；ctx cancel → 进程组 kill |
| `Destroy` | `forgeapp.Service.Delete`（forge 软删后） | 删整个 forge 目录（venv + code + lockfile）；先持 forge mutex 防并发 sync |
| `DestroyEnv` | `forgeapp.Service.trimEnvBuffer` LRU | 仅删指定 (forgeID, envID) 的 venv 子目录；其他 venv + code 文件保留 |
| `WriteCodeFile` | `forgeapp.Service.RunForge` 前 | 把 forge code 写到 `<forgeDir>/main.py`；幂等覆盖；多次 Run 仅在 code 变化时写 |
| `PythonPath` | `forgeapp.NewASTParser` 装配时 | 返回 `bundledPythonPath()` —— 跨平台路径抽象（mac/linux 是 `bin/python3`，windows 是根目录 `python.exe`）|

### 16.4 EnvID 算法

```go
// ComputeEnvID 把 (deps, pythonVersion) 标准化后哈希成 6 hex 字符。
// 同 deps 不同顺序、空白、大小写差异 → 同一个 EnvID（共享 venv）。
//
// 步骤：
//   1. 对每条 dep：strings.TrimSpace + strings.ToLower（normalizeSpecifier）
//   2. 排序去空：sort.Strings + filter ""
//   3. 拼接：strings.Join(deps, "\n") + "\n" + pythonVersion
//   4. sha256 → hex → 取前 6 字符
//
// 6 hex = 16M 组合空间；单 forge MaxEnvIDsPerForge=3 远低于碰撞概率。
func ComputeEnvID(deps []string, pythonVersion string) string
```

### 16.5 进程隔离

- **Unix**: `Setpgid: true` 启子进程独立 process group → ctx.Done() 时 `syscall.Kill(-pid, SIGKILL)` 杀整组（uv 自身 + python + 用户 code 启的所有 subprocess）
- **Windows**: 用 `taskkill /T /F /PID` 杀进程树（Go 1.26+ 计划改用 Job Object 更可靠）
- **Per-forge mutex**: `forgeMutexMap` 防 sync / destroy / run 并发触发同 forge——同 forge 操作 serialized，不同 forge 并行

### 16.6 工具代码约定

工具约定**只定义函数**，sandbox 在 `WriteCodeFile` 时追加驱动：

```python
def parse_csv(csv_text: str, delimiter: str = ',') -> list:
    """解析 CSV 文本。

    Args:
        csv_text: 要解析的 CSV 文本内容
        delimiter: 字段分隔符

    Returns:
        解析后的行列表，每行是字符串列表
    """
    import csv, io
    return list(csv.reader(io.StringIO(csv_text), delimiter=delimiter))

# sandbox 自动追加（buildDriver 拼接）：
# if __name__ == "__main__":
#     import json, sys
#     input_data = json.load(sys.stdin)
#     result = parse_csv(**input_data)
#     print(json.dumps(result, default=str))
```

`extractFuncName` 用正则在 code 第一行找 `def <name>`——单函数约定让 driver 自动接线。

### 16.7 集成测试门控

`integration_test.go` 有 13 个测试，环境变量 `FORGIFY_TEST_UV` + `FORGIFY_TEST_PYTHON` 都设了才跑（指向真实 uv / python-build-standalone 二进制）。CI / 离线机器跳过——保持 §T3 原则。

---

## 17. 实现清单

### Phase 3 初版（2026-04-26 完成）+ Phase 3 优化轮（2026-05-02 完成）

- [x] 详设计完成（本文档）
- [x] `domain/forge/forge.go` — 4 个 entity（Forge, ForgeVersion, ForgeTestCase, ForgeExecution）+ ExecutionResult + 常量 + 9 个 sentinel + Repository 接口 + ListFilter / ExecutionFilter
- [x] `domain/events/types.go` — entity-state forge 事件
- [x] `infra/db/schema_extras.go` — partial UNIQUE（forges 表）
- [x] `infra/store/forge/forge.go` — Repository 实现 + 集成测试
- [x] `app/forge/ast.go` — parseForgeCode（Python subprocess）
- [x] `app/forge/forge.go` — Service 实现 + 单测
- [x] `app/tool/forge/` — 5 个 System Tool（search/get/create/edit/run）+ resolveAttachments
- [x] `handlers/forge.go` — 21 个端点 + errmap
- [x] `router/deps.go` + `router/router.go` — 装配
- [x] `cmd/server/main.go` — 注入 forgeSvc + ForgeTools → chatService
- [x] service-contract-documents 同步更新

### 沙箱迭代 1（2026-05-03 完成）

- [x] `infra/sandbox/{sandbox,paths,pyproject,progress,preflight,sync,run,destroy,proc_*}.go` — 完整重写（13 个集成测试）
- [x] `domain/forge/forge.go` — Forge 加 ActiveVersionID + 5 计算字段；ForgeVersion 加 8 列；EnvStatus 5 态 + MaxEnvIDsPerForge=3 + DefaultPythonVersion；4 个新 sentinel
- [x] `infra/store/forge/forge.go` — 6 个新方法（GetVersionByID / UpdateVersionEnvID / UpdateVersionEnvStatus / UpdateVersionEnvProgress / ListEnvIDsForForge / EvictEnvForVersions / GetActiveVersion）
- [x] `app/forge/forge.go` — Sandbox 接口 1→6；attachActiveEnv；SyncEnvForVersion；trimEnvBuffer；CreateDraft；Create/Update/Delete/RevertToVersion/CreatePending/AcceptPending lifecycle 改造（19 个单测）
- [x] `app/forge/ast.go` — ASTParser struct{pythonPath}，pythonPath 从 sandbox.PythonPath() 取
- [x] `app/tool/forge/{create,edit}.go` — schema 加 dependencies + python_version；tool_result 含 env_status/env_error；EditForge 守卫已有 pending
- [x] `transport/httpapi/response/errmap.go` — 4 个 FORGE_* 映射
- [x] `cmd/server/main.go` — sandboxinfra.New + Bootstrap from $FORGIFY_DEV_RESOURCES（fail-soft warn）
- [x] `Makefile` + `scripts/download-sandbox-resources.sh` — download-resources 目标
- [x] `testend/` — forge-env-badge + envSyncDetail progress + envError 显示
- [x] 8 份文档同步（forge.md / database-design / error-codes / events / api-design / desktop-packaging-notes / CLAUDE / progress-record）
