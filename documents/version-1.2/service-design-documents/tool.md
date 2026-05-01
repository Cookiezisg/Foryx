# tool domain — 详细设计文档 v2

**所属 Phase**：Phase 3
**状态**：✅ 已实现（2026-04-26）
**职责**：管理用户锻造的 Python 工具全生命周期——CRUD、版本历史、pending 变更确认、测试用例、沙箱执行、导入导出；并向 ReAct Agent 提供 5 个 System Tool（search / get / create / edit / run）

**依赖**：
- `infra/db`（GORM + modernc.org/sqlite）
- `infra/sandbox`（Python subprocess 沙箱）
- `infra/llm`（create_tool / edit_tool 内部 LLM 调用 + GenerateTestCases）
- `pkg/reqctx`（userID 读取）
- `domain/events`（SSE 事件推送）

**被依赖**：
- `app/agent/forge.go`（System Tool 实现，由 app/chat 组装注入 ReAct Agent）
- Phase 4 workflow 节点

---

## 1. 核心决策

| 决策 | 选择 | 理由 |
|---|---|---|
| pending 与 version 的关系 | **合并为一张表** `tool_versions`，用 `status` 区分 | pending 和 version 形状完全一样，都是完整工具快照，无需两张表 |
| 版本快照内容 | **完整快照**：name + description + code + parameters + returnSchema + tags | 只存 code 的版本无法完整回滚，也无法看到历史状态 |
| pending 触发条件 | **所有 LLM 发起的变更**（code + 元数据）统一走 pending | 用户直接操作（HTTP PATCH / revert）立即生效，不走 pending |
| 工具搜索 | **LLM 排序**：SearchTool 把全部工具名+描述发给 LLM，LLM 返回按相关度排好的 ID + score 列表 | 比向量搜索准确（LLM 完整理解语义）；工具数量少（20-200），一次 prompt 能全放进去；无需 embedding API 或本地向量库，任何 LLM provider 都能用 |
| System Tool 位置 | `app/agent/forge.go`，组装留在 `app/chat` | 避免 app/chat 成为巨无霸；未来 web/workflow/knowledge 各加一个文件 |
| resolveAttachments | **RunTool（System Tool）调用前完成**，不进 Service | tool Service 不感知 att_id 概念，保持纯粹 |
| GenerateTestCases | Service 方法 + **callback（emit func）** 解耦 HTTP | Service 不导入 net/http；emit 函数由 Handler 注入 |
| LLM 注入 | **LLMClient 接口注入 Service** | GenerateTestCases 是 tool domain 自己的能力，不是 chat 触发的 |
| 代码生成方式 | **One-shot**，LLM 一次生成完整函数 | 工具是单函数，全量重写比 patch 更可靠 |
| 沙箱隔离 | **subprocess + 30s timeout** | 本地单用户；Docker 是过度工程 |
| AST 解析 | **Python subprocess + Google-style docstring** | 可靠提取 parameters（含 required）和 returnSchema |
| 归档 | **不做**，只有软删除 | 本地单用户，工具数量有限 |
| LLM 能否删除工具 | **不能** | 删除是破坏性操作，只走 HTTP API |

---

## 2. 多租户准备

- 所有表带 `user_id TEXT NOT NULL`
- Store 方法首行 `reqctx.GetUserID(ctx)`，缺失返错（接线 bug）
- Phase 3 仍硬编码 `"local-user"`

---

## 3. 领域模型

### 3.1 Tool（主实体）

```go
type Tool struct {
    ID           string         `gorm:"primaryKey;type:text"           json:"id"`
    UserID       string         `gorm:"not null;index;type:text"       json:"-"`
    Name         string         `gorm:"not null;type:text"             json:"name"`
    Description  string         `gorm:"not null;type:text;default:''"  json:"description"`
    Code         string         `gorm:"not null;type:text"             json:"code"`
    Parameters   string         `gorm:"type:text;default:'[]'"         json:"parameters"`   // JSON: [{name,type,required,description,default?}]
    ReturnSchema string         `gorm:"type:text;default:'{}'"         json:"returnSchema"` // JSON: {type,description}
    Tags         string         `gorm:"type:text;default:'[]'"         json:"tags"`          // JSON: ["tag1"]
    VersionCount int            `gorm:"not null;default:0"             json:"versionCount"`  // 当前最大 accepted version 号
    CreatedAt    time.Time      `json:"createdAt"`
    UpdatedAt    time.Time      `json:"updatedAt"`
    DeletedAt    gorm.DeletedAt `gorm:"index"                          json:"-"`
}

func (Tool) TableName() string { return "tools" }
```

| 字段 | 说明 |
|---|---|
| `ID` | `t_<16hex>` |
| `Name` | 工具库内唯一（partial UNIQUE：`UNIQUE(user_id, name) WHERE deleted_at IS NULL`）|
| `Code` | 当前 active 代码（最新 accepted version 的代码）|
| `Parameters` | `[{"name":"x","type":"str","required":true,"description":"...","default":null}]` |
| `ReturnSchema` | `{"type":"list","description":"..."}` |
| `VersionCount` | 最新 accepted version 号，从 1 开始 |

### 3.2 ToolVersion（版本历史 + pending 变更，合并表）

```go
type ToolVersion struct {
    ID           string    `gorm:"primaryKey;type:text"           json:"id"`
    ToolID       string    `gorm:"not null;index;type:text"       json:"toolId"`
    UserID       string    `gorm:"not null;type:text"             json:"-"`
    Version      *int      `gorm:"type:integer"                   json:"version"`      // pending/rejected 时为 nil
    Status       string    `gorm:"not null;type:text"             json:"status"`       // "pending"|"accepted"|"rejected"

    // 完整工具快照
    Name         string    `gorm:"not null;type:text"             json:"name"`
    Description  string    `gorm:"type:text;default:''"           json:"description"`
    Code         string    `gorm:"not null;type:text"             json:"code"`
    Parameters   string    `gorm:"type:text;default:'[]'"         json:"parameters"`
    ReturnSchema string    `gorm:"type:text;default:'{}'"         json:"returnSchema"`
    Tags         string    `gorm:"type:text;default:'[]'"         json:"tags"`

    Message      string    `gorm:"type:text;default:''"           json:"message"` // LLM instruction | "manual edit" | "reverted to v{N}" | "initial"
    CreatedAt    time.Time `json:"createdAt"`
    UpdatedAt    time.Time `json:"updatedAt"`
}

func (ToolVersion) TableName() string { return "tool_versions" }
```

**状态流转**：
```
pending → accepted  （用户确认）→ 分配 version 号，更新 Tool 主表
pending → rejected  （用户拒绝）→ version 保持 nil
```

**版本号分配**：accepted 时 `version = tool.VersionCount + 1`，同时 `tool.VersionCount++`

**上限**：每工具最多保留 50 条 `status='accepted'` 记录，超限硬删最旧的 accepted 版本。rejected/pending 不计入上限。

### 3.3 ToolTestCase（测试用例定义）

```go
type ToolTestCase struct {
    ID             string    `gorm:"primaryKey;type:text"        json:"id"`
    ToolID         string    `gorm:"not null;index;type:text"    json:"toolId"`
    UserID         string    `gorm:"not null;type:text"          json:"-"`
    Name           string    `gorm:"not null;type:text"          json:"name"`
    InputData      string    `gorm:"type:text;default:'{}'"      json:"inputData"`      // JSON object
    ExpectedOutput string    `gorm:"type:text;default:''"        json:"expectedOutput"` // JSON，空=不断言
    CreatedAt      time.Time `json:"createdAt"`
    UpdatedAt      time.Time `json:"updatedAt"`
}

func (ToolTestCase) TableName() string { return "tool_test_cases" }
```

### 3.4 ToolRunHistory（运行历史）

每次 `:run` 写一条，不管成功失败。

```go
type ToolRunHistory struct {
    ID          string    `gorm:"primaryKey;type:text"     json:"id"`
    ToolID      string    `gorm:"not null;index;type:text" json:"toolId"`
    UserID      string    `gorm:"not null;type:text"       json:"-"`
    ToolVersion int       `gorm:"not null"                 json:"toolVersion"` // 执行时的 accepted version 号
    Input       string    `gorm:"type:text;default:'{}'"   json:"input"`
    Output      string    `gorm:"type:text;default:''"     json:"output"`
    OK          bool      `gorm:"not null"                 json:"ok"`
    ErrorMsg    string    `gorm:"type:text;default:''"     json:"errorMsg"`
    ElapsedMs   int64     `gorm:"not null;default:0"       json:"elapsedMs"`
    CreatedAt   time.Time `json:"createdAt"`
}

func (ToolRunHistory) TableName() string { return "tool_run_history" }
```

### 3.5 ToolTestHistory（测试历史）

每次测试用例执行写一条（单跑和批跑都写）。

```go
type ToolTestHistory struct {
    ID          string    `gorm:"primaryKey;type:text"       json:"id"`
    ToolID      string    `gorm:"not null;index;type:text"   json:"toolId"`
    UserID      string    `gorm:"not null;type:text"         json:"-"`
    ToolVersion int       `gorm:"not null"                   json:"toolVersion"`
    TestCaseID  string    `gorm:"not null;index;type:text"   json:"testCaseId"`
    BatchID     string    `gorm:"type:text;default:'';index" json:"batchId"` // 批跑时共享，单跑时为空
    Input       string    `gorm:"type:text;default:'{}'"     json:"input"`
    Output      string    `gorm:"type:text;default:''"       json:"output"`
    OK          bool      `gorm:"not null"                   json:"ok"`
    Pass        *bool     `gorm:"type:integer"               json:"pass"` // nil=无 expected_output
    ErrorMsg    string    `gorm:"type:text;default:''"       json:"errorMsg"`
    ElapsedMs   int64     `gorm:"not null;default:0"         json:"elapsedMs"`
    CreatedAt   time.Time `json:"createdAt"`
}

func (ToolTestHistory) TableName() string { return "tool_test_history" }
```

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
    VersionStatusPending  = "pending"
    VersionStatusAccepted = "accepted"
    VersionStatusRejected = "rejected"

    MaxAcceptedVersions   = 50
    MaxRunHistoryPerTool  = 100
    MaxTestHistoryPerTool = 200
    SandboxTimeout        = 30 * time.Second
)
```

---

## 5. Sentinel 错误

```go
var (
    ErrNotFound         = errors.New("tool: not found")
    ErrDuplicateName    = errors.New("tool: name already exists")
    ErrVersionNotFound  = errors.New("tool: version not found")
    ErrPendingNotFound  = errors.New("tool: no pending change found")
    ErrPendingConflict  = errors.New("tool: already has a pending change")
    ErrTestCaseNotFound = errors.New("tool: test case not found")
    ErrRunFailed        = errors.New("tool: execution failed")
    ErrASTParseError    = errors.New("tool: code AST parse failed")
    ErrImportInvalid    = errors.New("tool: import data invalid")
)
```

---

## 6. Repository 接口

```go
type Repository interface {
    // Tool CRUD
    SaveTool(ctx context.Context, t *Tool) error
    GetTool(ctx context.Context, id string) (*Tool, error)
    GetToolsByIDs(ctx context.Context, ids []string) ([]*Tool, error) // LLM 排序后按 ID 批量拉完整对象
    ListTools(ctx context.Context, filter ListFilter) ([]*Tool, string, error)
    ListAllTools(ctx context.Context) ([]*Tool, error) // 供 search_tools 把全量工具发给 LLM 排序
    DeleteTool(ctx context.Context, id string) error

    // Versions（含 pending）
    SaveVersion(ctx context.Context, v *ToolVersion) error
    GetVersion(ctx context.Context, toolID string, version int) (*ToolVersion, error)
    GetActivePending(ctx context.Context, toolID string) (*ToolVersion, error) // status='pending'
    ListAcceptedVersions(ctx context.Context, toolID string) ([]*ToolVersion, error) // status='accepted', version DESC
    UpdateVersionStatus(ctx context.Context, id, status string, version *int) error
    DeleteOldestAcceptedVersion(ctx context.Context, toolID string) error

    // Test cases
    SaveTestCase(ctx context.Context, tc *ToolTestCase) error
    GetTestCase(ctx context.Context, id string) (*ToolTestCase, error)
    ListTestCases(ctx context.Context, toolID string) ([]*ToolTestCase, error)
    DeleteTestCase(ctx context.Context, id string) error

    // Run history
    SaveRunHistory(ctx context.Context, h *ToolRunHistory) error
    ListRunHistory(ctx context.Context, toolID string, limit int) ([]*ToolRunHistory, error)
    CountRunHistory(ctx context.Context, toolID string) (int64, error)
    DeleteOldestRunHistory(ctx context.Context, toolID string) error

    // Test history
    SaveTestHistory(ctx context.Context, h *ToolTestHistory) error
    ListTestHistory(ctx context.Context, toolID string, limit int) ([]*ToolTestHistory, error)
    ListTestHistoryByBatch(ctx context.Context, batchID string) ([]*ToolTestHistory, error)
    CountTestHistory(ctx context.Context, toolID string) (int64, error)
    DeleteOldestTestHistory(ctx context.Context, toolID string) error
}

type ListFilter struct {
    Cursor string
    Limit  int
}
```

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
    repo    tooldomain.Repository
    sandbox Sandbox
    llm     LLMClient // GenerateTestCases 使用
    log     *zap.Logger
}

type Sandbox interface {
    Run(ctx context.Context, code string, input map[string]any, timeout time.Duration) (*tooldomain.ExecutionResult, error)
}
// ExecutionResult 定义在 domain/tool/tool.go，避免 infra/sandbox ↔ app/tool 循环依赖

// LLMClient 非流式调用，等待完整 JSON 响应。
// 实现层复用 ChatModelFactory + KeyProvider，对 Service 透明。
type LLMClient interface {
    Generate(ctx context.Context, prompt string) (string, error)
}

// GenerateEvent 是 GenerateTestCases 通过 emit callback 推送的事件。
type GenerateEvent struct {
    Type     string                   // "test_case" | "done" | "not_supported"
    TestCase *tooldomain.ToolTestCase // Type="test_case" 时有值
    Count    int                      // Type="done" 时有值
    Reason   string                   // Type="not_supported" 时有值
}
```

### 8.2 Input / Output 类型

```go
type CreateInput struct {
    Name        string
    Description string
    Code        string
    Tags        []string // 可为空
}

type UpdateInput struct {
    Name        *string   // nil = 不改
    Description *string
    Tags        *[]string
    Code        *string   // nil = 不改代码
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

### 8.3 CRUD

```go
func (s *Service) Create(ctx context.Context, in CreateInput) (*tooldomain.Tool, error)
// CreateInput: {Name, Description, Code, Tags}
// 1. parseToolCode(code) → parameters, returnSchema
// 2. repo.SaveTool（UNIQUE 冲突 → ErrDuplicateName）
// 3. repo.SaveVersion(status='accepted', version=1, message="initial")
// 4. tool.VersionCount = 1

func (s *Service) Get(ctx context.Context, id string) (*tooldomain.Tool, error)

func (s *Service) GetDetail(ctx context.Context, id string) (*ToolDetail, error)
// 供 get_tool System Tool 使用：Get + 聚合最近 test history 摘要

type ToolDetail struct {
    *tooldomain.Tool
    TestSummary TestSummary
}

type TestSummary struct {
    Total        int    // 当前测试用例总数
    LastPassRate string // 最近一次 :test 批跑的结果，格式 "3/3" | "2/3" | "" (无记录)
    LastRunAt    string // 最近一次批跑时间，ISO 8601 或 ""
}

func (s *Service) List(ctx context.Context, filter tooldomain.ListFilter) ([]*tooldomain.Tool, string, error)

func (s *Service) ListAll(ctx context.Context) ([]*tooldomain.Tool, error)
// 供 SearchTool 使用：返回当前用户全部活跃工具（无分页），仅取 name+description 即可

func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*tooldomain.Tool, error)
// UpdateInput: Name? / Description? / Tags? / Code?（用户直接操作，立即生效）
// 若 Code != nil:
//   1. 检查有无 active pending → 自动 reject
//   2. parseToolCode(newCode) → parameters, returnSchema
// 3. 更新 Tool 主表所有变更字段
// 4. tool.VersionCount++，repo.SaveVersion(status='accepted', version=VersionCount, message="manual edit")
// 5. 若 accepted count > 50 → DeleteOldestAcceptedVersion

func (s *Service) Delete(ctx context.Context, id string) error
// repo.DeleteTool（软删）
```

### 8.4 版本管理

```go
func (s *Service) ListVersions(ctx context.Context, toolID string) ([]*tooldomain.ToolVersion, error)
// repo.ListAcceptedVersions（status='accepted', version DESC）

func (s *Service) GetVersion(ctx context.Context, toolID string, version int) (*tooldomain.ToolVersion, error)

func (s *Service) RevertToVersion(ctx context.Context, toolID string, version int) (*tooldomain.Tool, error)
// 1. GetVersion → 拿完整快照（name/description/code/parameters/returnSchema/tags）
// 2. 检查有无 active pending → 自动 reject
// 3. 更新 Tool 主表为快照内容
// 4. tool.VersionCount++，SaveVersion(status='accepted', version=VersionCount, message="reverted to v{N}")
// 5. 若 accepted count > 50 → DeleteOldestAcceptedVersion
```

### 8.5 Pending 管理

```go
func (s *Service) GetActivePending(ctx context.Context, toolID string) (*tooldomain.ToolVersion, error)
// repo.GetActivePending → ErrPendingNotFound if nil

func (s *Service) AcceptPending(ctx context.Context, toolID string) (*tooldomain.Tool, error)
// 1. repo.GetActivePending(toolID) → ErrPendingNotFound if none
// 2. 分配 version = tool.VersionCount + 1
// 3. 更新 Tool 主表为 pending 快照（name/description/code/parameters/returnSchema/tags）
// 4. tool.VersionCount = version
// 5. repo.UpdateVersionStatus(pv.ID, 'accepted', &version)
// 6. 若 accepted count > 50 → DeleteOldestAcceptedVersion

func (s *Service) RejectPending(ctx context.Context, toolID string) error
// repo.GetActivePending(toolID) → UpdateVersionStatus(pv.ID, 'rejected', nil)
```

### 8.6 执行

```go
func (s *Service) RunTool(ctx context.Context, toolID string, input map[string]any) (*ExecutionResult, error)
// input 已由调用方预处理（att_id 解析在 RunTool System Tool 内完成；HTTP 调用者直接传真实路径）
// 1. GetTool → code
// 2. sandbox.Run(code, input, SandboxTimeout)
// 3. 写 ToolRunHistory（无论成功失败）
// 4. 若 count > MaxRunHistoryPerTool → DeleteOldestRunHistory
```

### 8.7 测试用例

```go
func (s *Service) CreateTestCase(ctx context.Context, toolID string, in TestCaseInput) (*tooldomain.ToolTestCase, error)
func (s *Service) ListTestCases(ctx context.Context, toolID string) ([]*tooldomain.ToolTestCase, error)
func (s *Service) DeleteTestCase(ctx context.Context, id string) error

func (s *Service) RunTestCase(ctx context.Context, testCaseID string, batchID string) (*TestRunResult, error)
// sandbox.Run + 若 ExpectedOutput != "" 则断言 pass/fail
// 写 ToolTestHistory
// 若 count > MaxTestHistoryPerTool → DeleteOldestTestHistory

func (s *Service) RunAllTests(ctx context.Context, toolID string) ([]*TestRunResult, error)
// 生成 batchID → 逐条 RunTestCase(id, batchID) → 汇总返回

func (s *Service) GenerateTestCases(ctx context.Context, toolID string, count int, emit func(GenerateEvent)) error
// 1. GetTool → code + parameters + returnSchema
// 2. llm.Generate(ctx, prompt) — 等完整 JSON
//    prompt：分析函数，若依赖外部状态输出 {"not_supported":true,"reason":"..."}
//            否则输出 {"test_cases":[{name,input,expected_output},...]}
// 3. 解析结果：
//    not_supported → emit({Type:"not_supported", Reason:...})
//    test_cases    → 逐条 SaveTestCase + emit({Type:"test_case", TestCase:tc})
//    完成          → emit({Type:"done", Count:N})
// 注意：追加到现有测试集
```

### 8.8 导入导出

```go
func (s *Service) Export(ctx context.Context, toolID string) ([]byte, error)
// JSON: {name, description, code, tags, testCases:[]}

func (s *Service) Import(ctx context.Context, data []byte) (*tooldomain.Tool, error)
// 解析 → Create → 若有 testCases 则 CreateTestCase
```

### 8.9 AST 解析（私有，app/tool/ast.go）

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

// parseToolCode 启动 Python subprocess 解析代码结构。
// 要求 Google-style docstring；Description 字段解析失败时为空字符串，不报错。
func parseToolCode(code string) (*ParsedCode, error)
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

## 10. System Tools（app/agent/forge.go）

```go
func ForgeTools(
    toolSvc      *toolapp.Service,
    attachRepo  chatdomain.Repository,
    modelPicker modeldomain.ModelPicker,
    keyProvider apikeydomain.KeyProvider,
    llmFactory  *llminfra.Factory,        // 自有 LLM 流式客户端工厂
    bridge      eventsdomain.Bridge,
) []agentapp.Tool
// 返回 5 个 System Tool：search / get / create / edit / run（实现 agentapp.Tool 接口）
```

### search_tools

```
参数：{ "query": string, "limit"?: int（默认 3，最大 5）}
返回：[{
  id, name, description,
  parameters: [{name, type, required, description, default}],
  returnSchema: {type, description},
  similarity: float   // LLM 给出的相关度评分 0~1
}]

实现（SearchTool 内部）：
  1. toolSvc.ListAll(ctx) → 全部工具（name + description）
  2. llm.Generate(ctx, rankPrompt) → "[{\"id\":\"t_xxx\",\"score\":0.95},...]"
     rankPrompt：列出所有工具 + query，要求返回最相关的 limit 个 ID+score
  3. 解析 JSON → 按 score DESC 取前 limit 条
  4. repo.GetToolsByIDs(ids) → 完整 Tool 对象
  5. 组装返回，score 填入 similarity 字段

LLM 使用指引：
- similarity >= 0.8：高度相关，可直接 get_tool 确认后使用
- similarity 0.5~0.8：可能相关，建议 get_tool 读代码判断
- 返回空或全部低分：工具库无合适工具，考虑 create_tool
```

### get_tool

```
参数：{ "tool_id": string }
返回：{
  id, name, description, code,
  parameters: [{name, type, required, description, default}],
  returnSchema: {type, description},
  tags,
  versionCount,
  testSummary: {           // 最近一批测试的摘要，帮助 LLM 判断工具可靠性
    total: int,            // 测试用例总数
    lastPassRate: string,  // "3/3" | "2/3" | ""（无记录）
    lastRunAt: string      // ISO 8601 或 ""
  }
}
实现：toolSvc.GetDetail(tool_id)
说明：LLM 在 search_tools 拿到候选后，对不确定的工具调此接口读完整代码再决定是否使用
```

### create_tool

```
参数：{ "name": string, "description": string, "instruction": string }
返回：{ "tool_id": string, "name": string, "parameters": [...] }
流程：
  1. llm.Stream(createPrompt + instruction) → 逐 token 推 tool.code_streaming{actionType:"create"}
  2. toolSvc.Create({name, description, code})
  3. 推 tool.created
  4. 返回 {tool_id, name, parameters}
```

### edit_tool

```
参数：{
  "tool_id": string,
  "instruction"?: string,    // 有 → LLM 生成新代码（流式）
  "name"?: string,
  "description"?: string,
  "tags"?: [string]
}
// instruction 和其余字段至少提供一个

返回：{ "pending_id": string, "tool_id": string }
流程：
  1. GetTool → 当前完整状态
  2. 检查 active pending → ErrPendingConflict（先检查，避免白做工作）
  3. 若有 instruction：llm.Stream(editPrompt + currentCode + instruction)
     → 逐 token 推 tool.code_streaming{actionType:"edit"}
     → 生成新代码，parseToolCode → parameters, returnSchema
  4. 构建完整 pending 快照（合并当前状态 + 参数中的变更）
  5. repo.SaveVersion(status='pending', message=instruction or "metadata update")
  6. 推 tool.pending_created
  7. 返回 {pending_id, tool_id}
```

### run_tool

```
参数：{ "tool_id": string, "input": object }
返回：{ "ok": bool, "output": any, "error"?: string, "elapsed_ms": int }
流程：
  1. resolveAttachments(ctx, input)
  2. toolSvc.RunTool(ctx, tool_id, resolvedInput)
注意：执行失败返回 ok=false，不是 HTTP 错误
```

---

## 11. HTTP API（22 个端点，get_tool 仅为 System Tool，无对应 HTTP 端点）

| Method | Path | 用途 | 状态码 |
|---|---|---|---|
| POST | `/api/v1/tools` | 创建（直接传 code，不走 LLM）| 201 |
| GET | `/api/v1/tools` | 列表（分页 + `?q=` 语义搜索）| 200 |
| GET | `/api/v1/tools/{id}` | 详情 | 200 |
| PATCH | `/api/v1/tools/{id}` | 更新（直接生效，任意字段）| 200 |
| DELETE | `/api/v1/tools/{id}` | 软删 | 204 |
| POST | `/api/v1/tools/{id}:run` | 执行工具 | 200 |
| POST | `/api/v1/tools/{id}:export` | 导出 JSON | 200 |
| POST | `/api/v1/tools:import` | 导入 JSON | 201 |
| GET | `/api/v1/tools/{id}/versions` | accepted 版本列表 | 200 |
| GET | `/api/v1/tools/{id}/versions/{version}` | 单版本详情（含完整快照）| 200 |
| POST | `/api/v1/tools/{id}:revert` | 回滚到指定版本 | 200 |
| GET | `/api/v1/tools/{id}/pending` | 当前 pending（无则 404）| 200/404 |
| POST | `/api/v1/tools/{id}/pending:accept` | 接受 | 200 |
| POST | `/api/v1/tools/{id}/pending:reject` | 拒绝 | 204 |
| GET | `/api/v1/tools/{id}/test-cases` | 测试用例列表 | 200 |
| POST | `/api/v1/tools/{id}/test-cases` | 创建测试用例 | 201 |
| DELETE | `/api/v1/tools/{id}/test-cases/{tcId}` | 删除测试用例 | 204 |
| POST | `/api/v1/tools/{id}/test-cases/{tcId}:run` | 运行单个测试用例 | 200 |
| POST | `/api/v1/tools/{id}:test` | 运行全部测试用例 | 200 |
| POST | `/api/v1/tools/{id}:generate-test-cases` | LLM 生成测试用例（SSE）| 200 SSE |
| GET | `/api/v1/tools/{id}/run-history` | 运行历史 | 200 |
| GET | `/api/v1/tools/{id}/test-history` | 测试历史（`?batchId=` 过滤）| 200 |

**关键说明**：
- `POST /tools` 和 `PATCH /tools/{id}` 是用户直接操作，立即生效，创建 accepted version
- `edit_tool`（System Tool）是 LLM 发起的变更，统一走 pending，用户审核后生效
- `:run` 执行失败是业务结果（200 + `ok:false`），不是 HTTP 错误

---

## 12. 错误码

| Code | HTTP | Sentinel | 场景 |
|---|---|---|---|
| `TOOL_NOT_FOUND` | 404 | `ErrNotFound` | id 查不到 |
| `TOOL_NAME_DUPLICATE` | 409 | `ErrDuplicateName` | 创建/改名撞名 |
| `TOOL_VERSION_NOT_FOUND` | 404 | `ErrVersionNotFound` | revert / get version 时版本不存在 |
| `TOOL_PENDING_NOT_FOUND` | 404 | `ErrPendingNotFound` | accept/reject 时无 pending |
| `TOOL_PENDING_CONFLICT` | 409 | `ErrPendingConflict` | edit_tool 时已有未处理 pending |
| `TOOL_TEST_CASE_NOT_FOUND` | 404 | `ErrTestCaseNotFound` | 测试用例找不到 |
| `TOOL_RUN_FAILED` | 422 | `ErrRunFailed` | sandbox 内部错误（≠ 执行失败，执行失败是 ok=false）|
| `TOOL_AST_PARSE_FAILED` | 422 | `ErrASTParseError` | 代码无法被 Python AST 解析 |
| `TOOL_IMPORT_INVALID` | 400 | `ErrImportInvalid` | 导入 JSON 格式错误 |

---

## 13. SSE 事件（6 个新增，追加到 domain/events/types.go）

```go
// ToolCodeStreaming 在 create_tool / edit_tool 的 LLM 代码生成阶段逐 token 推送。
// ToolCodeStreaming 在 create_tool / edit_tool 的 LLM 代码生成阶段逐 token 推送。
// MessageID + ToolCallID 把流绑定到触发它的对话轮次，前端据此关联代码面板更新。
// create_tool 期间 ToolID 为空（工具尚未创建）。
type ToolCodeStreaming struct {
    ConversationID string `json:"conversationId"`
    MessageID      string `json:"messageId"`  // 触发此 tool call 的 assistant 消息 id
    ToolCallID     string `json:"toolCallId"` // LLM 分配的 tool call id
    ToolID         string `json:"toolId"`     // edit 时为现有 ID；create 时为空
    ActionType     string `json:"actionType"` // "create" | "edit"
    Delta          string `json:"delta"`
}
func (ToolCodeStreaming) EventName() string { return "tool.code_streaming" }

// ToolCreated 在 create_tool 成功保存工具后推送。
type ToolCreated struct {
    ConversationID string `json:"conversationId"`
    MessageID      string `json:"messageId"`
    ToolCallID     string `json:"toolCallId"`
    ToolID         string `json:"toolId"`
    ToolName       string `json:"toolName"`
}
func (ToolCreated) EventName() string { return "tool.created" }

// ToolPendingCreated 在 edit_tool 保存 pending 变更后推送。
type ToolPendingCreated struct {
    ConversationID string `json:"conversationId"`
    MessageID      string `json:"messageId"`
    ToolCallID     string `json:"toolCallId"`
    ToolID         string `json:"toolId"`
    PendingID      string `json:"pendingId"`  // status='pending' 的 ToolVersion id
    Instruction    string `json:"instruction"`
}
func (ToolPendingCreated) EventName() string { return "tool.pending_created" }

// ToolTestCaseGenerated 在 generate-test-cases 每生成一条完整用例时推送（完整对象，非 token）。
type ToolTestCaseGenerated struct {
    ToolID         string `json:"toolId"`
    TestCaseID     string `json:"testCaseId"`
    Name           string `json:"name"`
    InputData      string `json:"inputData"`
    ExpectedOutput string `json:"expectedOutput"`
}
func (ToolTestCaseGenerated) EventName() string { return "tool.test_case_generated" }

// ToolTestCasesDone 在 generate-test-cases 全部完成后推送。
type ToolTestCasesDone struct {
    ToolID string `json:"toolId"`
    Count  int    `json:"count"`
}
func (ToolTestCasesDone) EventName() string { return "tool.test_cases_done" }

// ToolTestCasesNotSupported 在 LLM 判断工具不可自动测试时推送。
type ToolTestCasesNotSupported struct {
    ToolID string `json:"toolId"`
    Reason string `json:"reason"` // LLM 解释，直接展示给用户
}
func (ToolTestCasesNotSupported) EventName() string { return "tool.test_cases_not_supported" }
```

---

## 14. 端到端调用链

### 链 1：LLM 创建工具

```
用户："帮我写一个解析 CSV 的工具"
  → LLM 调 create_tool({name, description, instruction})
  → CreateTool.InvokableRun
      → llm.Stream → 推 tool.code_streaming tokens
      → toolSvc.Create → SaveTool + SaveVersion(v1, accepted)
      → 推 tool.created
      → return {tool_id, name, parameters}
```

### 链 2：LLM 编辑工具（代码 + 元数据）

```
用户："帮 parse_csv 加 delimiter 参数，顺便改个好听的名字"
  → LLM 调 edit_tool({tool_id, instruction:"add delimiter param", name:"csv_parser"})
  → EditTool.InvokableRun
      → llm.Stream(currentCode + instruction) → 推 tool.code_streaming tokens
      → 构建完整 pending 快照（name="csv_parser", 新代码, 新 parameters...）
      → repo.SaveVersion(status='pending')
      → 推 tool.pending_created
      → return {pending_id, tool_id}
```

### 链 3：用户接受 pending

```
POST /api/v1/tools/t_xxx/pending:accept
  → toolSvc.AcceptPending
      → 分配 version = VersionCount + 1
      → 更新 Tool 主表为 pending 快照（名字也改了）
      → UpdateVersionStatus → 'accepted'
      → vectorDB.Upsert（新 name+description）
  → 200 updatedTool
```

### 链 4：LLM 搜索并执行工具

```
用户："帮我处理这段 CSV"
  → LLM 调 search_tools({query:"csv"}) → [{id, name, parameters, returnSchema, similarity:0.91}]
  → LLM 调 run_tool({tool_id, input:{csv_text:"..."}})
  → RunTool.InvokableRun
      → resolveAttachments（无 att_ 字段，直接透传）
      → toolSvc.RunTool → sandbox.Run → 写 ToolRunHistory
      → return {ok:true, output:[...], elapsed_ms:35}
```

### 链 5：LLM 执行工具处理附件

```
用户上传 report.csv → att_abc123
用户："用工具处理这个文件"
  → LLM 调 run_tool({tool_id, input:{file_path:"att_abc123"}})
  → RunTool.InvokableRun
      → resolveAttachments → 查 chat_attachments → {file_path:"/data/.../original.csv"}
      → toolSvc.RunTool → sandbox.Run
```

### 链 6：用户点击"AI 生成测试用例"

```
POST /api/v1/tools/t_xxx:generate-test-cases  (SSE)
  → handler 调 toolSvc.GenerateTestCases(ctx, id, 5, emit)
      → llm.Generate(prompt) — 等完整 JSON

      情况 A（可测）：
        → 逐条 SaveTestCase + emit({Type:"test_case", TestCase})  → SSE: tool.test_case_generated
        → emit({Type:"done", Count:5})                            → SSE: tool.test_cases_done

      情况 B（不可测，如依赖文件路径）：
        → emit({Type:"not_supported", Reason:"..."})              → SSE: tool.test_cases_not_supported
```

---

## 15. 数据库表总览

| 表 | 主键前缀 | 说明 |
|---|---|---|
| `tools` | `t_` | 主实体，当前 active 状态 |
| `tool_versions` | `tv_` | 版本历史 + pending 变更（status 字段区分），accepted 最多保留 50 条 |
| `tool_test_cases` | `tc_` | 测试用例定义 |
| `tool_run_history` | `trh_` | 每次 `:run` 记录，最多 100 条/工具 |
| `tool_test_history` | `tth_` | 每次测试用例执行记录，最多 200 条/工具 |

`schema_extras.go` 追加：`UNIQUE(user_id, name) WHERE deleted_at IS NULL`（tools 表）

向量索引由 chromem-go 管理，路径 `{dataDir}/vectordb/tools`，不经过 SQLite。

---

## 16. infra/sandbox/python.go

```go
// internal/infra/sandbox/python.go

type PythonSandbox struct{ pythonPath string }

func New(pythonPath string) *PythonSandbox

func (s *PythonSandbox) Run(
    ctx context.Context,
    code string,
    input map[string]any,
    timeout time.Duration,
) (*toolapp.ExecutionResult, error)
// 1. 拼接驱动代码（读 stdin JSON → 调函数 → 输出 JSON）写临时文件
// 2. JSON 序列化 input → stdin
// 3. subprocess python3，超时 kill
// 4. stdout → output，stderr → errorMsg
// 5. 清理临时文件
```

工具约定只定义函数，sandbox 追加驱动：

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

# sandbox 自动追加：
# if __name__ == "__main__":
#     import json, sys
#     input_data = json.load(sys.stdin)
#     result = parse_csv(**input_data)
#     print(json.dumps(result))
```

---

## 17. 实现清单

- [x] 详设计完成（本文档）
- [x] `domain/tool/tool.go` — 5 个 entity + `ExecutionResult` + 常量 + 9 个 sentinel + Repository 接口 + ListFilter
- [ ] `domain/events/types.go` — 追加 6 个 SSE 事件
- [ ] `infra/sandbox/python.go` — PythonSandbox + 测试
- [ ] `infra/db/schema_extras.go` — partial UNIQUE（tools 表）
- [ ] `infra/store/tool/tool.go` — Repository 实现 + 集成测试
- [ ] `app/tool/ast.go` — parseToolCode（Python subprocess）
- [ ] `app/tool/tool.go` — Service 实现 + 单测
- [ ] `app/agent/forge.go` — 5 个 System Tool（search/get/create/edit/run）+ resolveAttachments
- [ ] `handlers/tool.go` — 22 个端点 + errmap
- [ ] `router/deps.go` + `router/router.go` — 装配
- [ ] `cmd/server/main.go` — 注入 toolService + ForgeTools → chatService
- [ ] service-contract-documents 同步更新
