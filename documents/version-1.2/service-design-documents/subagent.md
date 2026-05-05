# Subagent — V1.2 详设计

**Phase**：Phase 4 准备件（提前到位，本周交付）
**状态**：📐 设计完成（2026-05-04）— 待实施
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/database-design.md`](../service-contract-documents/database-design.md) — `subagent_runs` + `subagent_messages` 两表（待加）
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — subagent ×3（待加）
- [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) — `chat.message` 事件加 `subagentRunId` + `parentConversationId` + `subagentRun` 三字段（subagent 不发独立 SSE 事件）
- 关联设计文档：[`skill.md`](./skill.md)（`context: fork` 复用本服务）/ [`chat.md`](./chat.md)（chat.message 事件 schema 承载 subagent 信息）

---

## 1. 一句话

LLM 通过 **`Subagent(prompt, subagent_type)` 一个 system tool**，在主对话之外起一个 **独立 context、独立 messages、过滤后 tool registry** 的子 LLM loop；跑完只回 last assistant message 给主 LLM 当 tool_result。**复用主 chat runner 的 `agentRun`，不复制流式/重试/工具调度逻辑**。

> **注**：v1 改名了——原本和 Claude Code 看齐叫 `Task`，但 Forgify 已有 `task` mini-domain（TaskCreate/List/Get/Update 管 TODO），LLM 看到 `Task` + `TaskCreate/List/...` 五个会以为同族 → 改名 `Subagent` 明确区分。

---

## 2. 端到端推演（设计原则 #5）

```
触发源：LLM 在 chat agent 循环里调 Subagent 工具
  → transport 层：无（system tool 不走 HTTP）
    → app 层：app/tool/subagent.SubagentTool.Execute
        → reqctxpkg.GetSubagentDepth(ctx) > 上限 → 立即返 ErrRecursionAttempt 字符串
        → subagentapp.Service.Spawn(ctx, type, prompt, opts)
            → 取 SubagentType（找不到 → ErrTypeNotFound）
            → 过滤 tool registry（**物理排除 SubagentTool 自身**；只保留 type.AllowedTools）
            → 创建 SubagentRun 入库（status=running）
            → 解析 model（opts.Model ?? type.DefaultModel ?? pickModel）
            → ctx 注入 SubagentRunID + SubagentDepth+1
            → 构造 subagentHost{run, repo, bridge, filteredTools, systemPrompt}
            → loop.Run(ctx, subagentHost, client, baseReq, maxSteps)
                → host.LoadHistory（首次只有 user prompt）
                → host.OnInitialPublish（推 chat.message status=streaming + subagentRun stub）
                → ReAct 循环：stream + tool dispatch + extendHistory
                    · 每 step：host.OnStepComplete 累 token + repo.Update SubagentRun
                    · streaming checkpoint：host.OnStreamCheckpoint 写 SubagentMessage + 推 chat.message
                · 终止条件：finish_reason = stop / max_turns / ctx.cancel
                → host.OnFinalize（写终态 SubagentMessage + 推 chat.message status=completed/cancelled/max_turns）
            → loop.Result 含 LastMessage
            → 写 SubagentRun（status=终态，EndedAt，含 token 总计）
        → 返回 LastMessage string 给主 LLM 作为 tool_result
  → 主 LLM 收到 tool_result，继续主 loop
```

**端到端跨 domain 依赖**：
- `app/loop`（**新包**）：通用 ReAct 引擎；从 chat 迁出 streamLLM/runTools/partitionByExecutionGroup/extendHistory；定义 `Host` 接口 + `Run` 函数
- `app/chat`：`agentRun` 改写为构造 `chatHost` → `loop.Run`（无对外 API 变化，纯内部重构）
- `app/subagent`（**新包**）：`Service.Spawn` 内部构造 `subagentHost` → `loop.Run`
- `pkg/reqctx`：新增 `SubagentDepth` / `SubagentRunID` ctx key
- `pkg/agentstate`：新增 `SubagentTokenLog` 字段（per-conversation 累计，UI 调用 conversation 详情时可拉）
- `domain/events`：`ChatMessage` struct 加 3 个 omitempty 字段（subagentRunId / parentConversationId / subagentRun）；不新增独立事件类型
- `infra/store/subagent`：新增 GORM 实现

---

## 3. 领域模型

两个持久化实体：
- **`SubagentRun`** — 每次 spawn 一条总账（status / token 累计 / model / 时间戳等）
- **`SubagentMessage`** — subagent 内部每条消息一行（流式 UI 渲染 + 回放历史用）

### SubagentType（注册表项，`internal/domain/subagent/subagent.go`）

```go
type SubagentType struct {
    Name            string   `json:"name"`            // "Explore" / "Plan" / "general-purpose"
    Description     string   `json:"description"`     // LLM 用来 match
    SystemPrompt    string   `json:"systemPrompt"`    // sub-runner 的 system prompt
    AllowedTools    []string `json:"allowedTools"`    // tool 白名单（按 Tool.Name() 匹配）
    DefaultModel    string   `json:"defaultModel"`    // 可空 → fallback PickForChat
    DefaultMaxTurns int      `json:"defaultMaxTurns"` // 默认 25
}
```

**注册表**位于 `internal/app/subagent/registry.go`：v1 三个内置类型（Explore / Plan / general-purpose），实现按 §4。**未来**可加文件加载（`~/.forgify/subagents/<name>.md` 类似 Skill），但 v1 内置足够。

### SubagentRun（每次 spawn 的总账，同文件）

```go
type SubagentRun struct {
    // ── 持久化字段（落库 subagent_runs 表）──
    ID                   string         `gorm:"primaryKey;type:text" json:"id"`
    ParentConversationID string         `gorm:"not null;index;type:text" json:"parentConversationId"`
    ParentMessageID      string         `gorm:"type:text;index" json:"parentMessageId,omitempty"`
    ParentToolCallID     string         `gorm:"type:text" json:"parentToolCallId,omitempty"`
    Type                 string         `gorm:"not null;type:text" json:"type"`
    Prompt               string         `gorm:"type:text" json:"prompt"`
    Result               string         `gorm:"type:text" json:"result,omitempty"`
    Status               string         `gorm:"not null;type:text;default:running" json:"status"`
    TotalTokensIn        int            `json:"totalTokensIn"`
    TotalTokensOut       int            `json:"totalTokensOut"`
    StepsUsed            int            `json:"stepsUsed"`
    Model                string         `gorm:"type:text" json:"model,omitempty"`
    StartedAt            time.Time      `json:"startedAt"`
    EndedAt              *time.Time     `json:"endedAt,omitempty"`
    ErrorMsg             string         `gorm:"type:text" json:"errorMsg,omitempty"`
    CreatedAt            time.Time      `json:"createdAt"`
    UpdatedAt            time.Time      `json:"updatedAt"`

    // ── 流式 UI 瞬时字段（仅 in-memory，gorm:"-"，不落库）──
    // run 跑完这些字段就过期了；服务重启时丢失也无关紧要（run 本身已结束）；
    // 通过 chat.message 嵌套 subagentRun 推给前端做"小窗状态条 / lastTool 提示"。
    LastToolCalled       string         `gorm:"-" json:"lastToolCalled,omitempty"`
    LastToolArgsBrief    string         `gorm:"-" json:"lastToolArgsBrief,omitempty"`
    LastToolResultBrief  string         `gorm:"-" json:"lastToolResultBrief,omitempty"`
    LastStepDurationMs   int            `gorm:"-" json:"lastStepDurationMs,omitempty"`
    LastStepAt           *time.Time     `gorm:"-" json:"lastStepAt,omitempty"`
}

func (SubagentRun) TableName() string { return "subagent_runs" }
```

### SubagentMessage（subagent 内部消息细粒度持久化，同文件）

```go
type SubagentMessage struct {
    ID               string                  `gorm:"primaryKey;type:text" json:"id"`
    SubagentRunID    string                  `gorm:"not null;index:idx_smm_run_seq,priority:1;type:text" json:"subagentRunId"`
    Seq              int                     `gorm:"not null;index:idx_smm_run_seq,priority:2" json:"seq"`
    Role             string                  `gorm:"not null;type:text" json:"role"`            // user / assistant / tool / system
    Blocks           []chatdomain.Block      `gorm:"serializer:json" json:"blocks"`             // 复用 chat 的 Block 类型，前端渲染零成本
    PromptTokens     int                     `json:"promptTokens,omitempty"`                    // assistant role 才有
    CompletionTokens int                     `json:"completionTokens,omitempty"`                // assistant role 才有
    CreatedAt        time.Time               `json:"createdAt"`
}

func (SubagentMessage) TableName() string { return "subagent_messages" }
```

### 字段说明

| 字段 | 说明 |
|---|---|
| `SubagentRun.ID` | `sar_<16hex>`（per §S15）|
| `SubagentRun.Parent*ID` | 调用方 conversation/message/toolCall 的反向引用（审计 "哪条消息触发的"）|
| `SubagentRun.Status` | 5 值：`running` / `completed` / `cancelled` / `max_turns` / `failed` |
| `SubagentRun.TotalTokens*` | 累计；每 message 末由 sub-runner 更新 |
| `SubagentRun.LastTool*` | 流式小窗 UI"当前在干啥"用；每 step 末由 sub-runner 更新；通过 chat.message 嵌套 subagentRun 推前端（§10）|
| `SubagentMessage.ID` | `smm_<16hex>`（per §S15，prefix=subagent message） |
| `SubagentMessage.Seq` | run 内顺序，0 起，写入即定 |
| `SubagentMessage.Blocks` | 复用 `chatdomain.Block` —— text / tool_call / tool_result / reasoning / attachment_ref 五类 |

### 复合索引

```
INDEX idx_smm_run_seq (subagent_run_id, seq)
```
按 run 拉全部消息 + 按 seq 排序，单 query 命中。

### Sentinel 错误（4 个）

```go
var (
    ErrTypeNotFound      = errors.New("subagent: type not found")
    ErrRecursionAttempt  = errors.New("subagent: nested spawn not allowed")
    ErrMaxTurnsExceeded  = errors.New("subagent: max turns exceeded")
    ErrCancelled         = errors.New("subagent: cancelled")
)
```

`ErrMaxTurnsExceeded` / `ErrCancelled` 不视为"错误"——sub-runner 终态、SubagentRun.Status 反映即可。`Tool.Execute` 把它们转成 `tool_result` 文本（"I ran out of turns; here's what I found so far: ..."）让主 LLM 知情，**不**抛 handler。

---

## 4. 内置 SubagentType 三种

### 4.1 `Explore`（参考 Claude Code）

```go
{
    Name: "Explore",
    Description: "Fast read-only search agent for locating code/files. Use to find files, grep symbols, answer 'where is X defined'. Don't use for analysis or design — its tool list excludes mutation.",
    SystemPrompt: "You are Explore, a code reconnaissance agent. ...",
    AllowedTools: []string{"read", "glob", "grep", "ls", "search_forges"},  // read-only
    DefaultModel: "",  // → fallback PickForChat
    DefaultMaxTurns: 30,
}
```

### 4.2 `Plan`（参考 Claude Code）

```go
{
    Name: "Plan",
    Description: "Software architect agent for designing implementation plans. Use when you need to plan strategy for a task, identify critical files, consider trade-offs.",
    SystemPrompt: "You are Plan, an architectural advisor. ...",
    AllowedTools: []string{"read", "glob", "grep", "ls", "web_fetch", "web_search"},
    DefaultModel: "",
    DefaultMaxTurns: 25,
}
```

### 4.3 `general-purpose`

```go
{
    Name: "general-purpose",
    Description: "General-purpose agent for researching complex questions, searching for code, executing multi-step tasks. Use when you're not confident a single search will succeed.",
    SystemPrompt: "You are a general-purpose subagent. ...",
    AllowedTools: []string{},  // 空 = 继承父 registry（除 Task 自身）
    DefaultModel: "",
    DefaultMaxTurns: 25,
}
```

**`AllowedTools` 空数组的语义**：继承父 registry **去掉 SubagentTool 自身**——这是 `general-purpose` 唯一与具体类型不同的地方。其他类型用显式白名单。

---

## 5. Repository 接口

```go
// internal/domain/subagent/subagent.go
type Repository interface {
    // SubagentRun
    CreateRun(ctx context.Context, r *SubagentRun) error
    GetRun(ctx context.Context, id string) (*SubagentRun, error)
    UpdateRun(ctx context.Context, r *SubagentRun) error
    ListRunsByConversation(ctx context.Context, conversationID string) ([]*SubagentRun, error)

    // SubagentMessage
    AppendMessage(ctx context.Context, m *SubagentMessage) error            // Seq 由 store 自增分配
    UpdateMessage(ctx context.Context, m *SubagentMessage) error            // 流式更新同条 message 的 blocks
    ListMessagesByRun(ctx context.Context, runID string) ([]*SubagentMessage, error)
}
```

实现在 `internal/infra/store/subagent/subagent.go`，标准 GORM。`AppendMessage` 内部 `SELECT COALESCE(MAX(seq), -1) + 1` 取下一个 seq（同事务）。

---

## 6. Service 层（`internal/app/subagent/subagent.go`）

```go
type Service struct {
    repo     subagentdomain.Repository
    registry map[string]subagentdomain.SubagentType
    tools    []toolapp.Tool          // 全局 tool 注册表（每次 Spawn 内部按 type 过滤一份子集）
    bridge   eventsdomain.Bridge
    pickModel func(ctx context.Context) (llminfra.Client, llminfra.Request, error)  // 复用 chat 的 model picker（仅函数依赖，不依赖 chat 整体）
    log      *zap.Logger
}

type SpawnOpts struct {
    MaxTurns int    // 0 = 用 type 默认
    Model    string // "" = 用 type 默认
}

type SpawnResult struct {
    Run    *subagentdomain.SubagentRun
    Result string  // last assistant message
}

func (s *Service) Spawn(ctx context.Context, typeName, prompt string, opts SpawnOpts) (*SpawnResult, error)
func (s *Service) Cancel(ctx context.Context, runID string) error
func (s *Service) Get(ctx context.Context, runID string) (*subagentdomain.SubagentRun, error)
func (s *Service) ListTypes() []subagentdomain.SubagentType
func (s *Service) ListByConversation(ctx context.Context, conversationID string) ([]*subagentdomain.SubagentRun, error)
```

### ReAct 循环复用：`internal/app/loop/`（不再有 SubRunner 接口）

V1.2 把通用 ReAct loop 抽到独立包 `internal/app/loop/`——**chat 主对话和 subagent 都是 loop 的调用方**，不再相互依赖。文档前期版本设计的 `SubRunner` port 已废弃。

`loop` 包导出：

```go
// internal/app/loop/loop.go
type Host interface {
    LoadHistory(ctx context.Context) ([]llminfra.Message, error)
    Tools() []toolapp.Tool                                            // 已过滤的 tool 列表（chat 返全局；subagent 返按 type 过滤）
    SystemPrompt(ctx context.Context) string                          // loop 拼到 history 前

    OnInitialPublish(ctx context.Context)                             // 创建 stub message slot（chat 推 chat.message status=streaming；subagent 推带 subagentRun 的 chat.message）
    OnStreamCheckpoint(ctx context.Context, blocks []chatdomain.Block, status, stopReason string, in, out int)   // 每 step 末非致命落盘 + 推快照
    OnFinalize(ctx context.Context, blocks []chatdomain.Block, status, stopReason, errCode, errMsg string, in, out int)  // 终态落盘 + 推快照
    OnStepComplete(ctx context.Context, step, deltaIn, deltaOut int)  // 给 token accounting hook（subagent 写 SubagentRun；chat 可空实现）
}

type Result struct {
    Blocks      []chatdomain.Block
    Status      string         // completed / cancelled / error / max_turns
    StopReason  string
    TokensIn    int
    TokensOut   int
    Steps       int
    LastMessage string         // 提取自最后一个 assistant text block，subagent 用作返主 LLM 的 tool_result
}

func Run(ctx context.Context, host Host, client llminfra.Client, baseReq llminfra.Request, maxSteps int) Result
```

**`loop.Run` 内部职责**：
1. `host.LoadHistory()` 取历史 → `host.SystemPrompt()` 拼前
2. `host.OnInitialPublish()` 开槽位
3. ReAct 循环（最多 maxSteps 步）：
   - `streamLLM`（loop 内部函数，原 chat/stream.go 迁入）→ 拿 blocks + tool_calls + token deltas
   - `host.OnStepComplete()` 实时累 token
   - 无 tool_calls → `host.OnFinalize(status=completed)` break
   - 有 tool_calls → `runTools`（loop 内部，原 chat/tools.go 迁入；按 execution_group 分批；从 host.Tools() 取注册表）→ append 结果 → `host.OnStreamCheckpoint(status=streaming)`
4. 步数上限 → `host.OnFinalize(status=completed, stopReason=max_tokens)`
5. cancel/error 路径同样落到 `host.OnFinalize`

**chat 侧实现**：
- `internal/app/chat/host.go` 新增 `chatHost{convID, msgID, store, bridge, log}` —— `agentRun` 改写为：构造 chatHost → `loop.Run(...)` → `autoTitle`
- 现有 `streamLLM` / `runTools` / `partitionByExecutionGroup` / `extendHistory` 全部从 chat 迁到 loop 包

**subagent 侧实现**：
- `internal/app/subagent/host.go` 新增 `subagentHost{run, repo, bridge, log}` —— Service.Spawn 内构造 subagentHost → `loop.Run(...)` → 取 Result.LastMessage 返调用方

**包依赖方向**：
```
loop（不依赖 chat / subagent）
 ↑
chat → loop
subagent → loop
events → chatdomain + subagentdomain（见 §10）
```

无循环、无 port、无依赖注入接口——纯单向依赖。Workflow（Phase 4）/ Skill `context: fork` 未来直接接 `loop.Host` 即可，无需扩接口。

---

## 7. Tool 实现（`internal/app/tool/subagent/agent.go`）

```go
type SubagentTool struct {
    svc *subagentapp.Service
}

func (t *SubagentTool) Name() string { return "Subagent" }

func (t *SubagentTool) Description() string {
    return "Spawn a specialized subagent to handle a focused subtask in isolation. " +
           "The subagent has its own context window and a curated tool list. " +
           "Returns the subagent's final message as a string. " +
           "Use for: searching large codebases (Explore), planning multi-step work (Plan), " +
           "or any task where isolating context from your main conversation is valuable."
}

func (t *SubagentTool) Parameters() json.RawMessage {
    return json.RawMessage(`{
      "type": "object",
      "properties": {
        "subagent_type": {
          "type": "string",
          "description": "Which subagent to spawn. Available: Explore, Plan, general-purpose."
        },
        "prompt": {
          "type": "string",
          "description": "Task description for the subagent. Be specific — subagent doesn't see your conversation."
        },
        "max_turns": {
          "type": "integer",
          "description": "Optional cap on subagent's ReAct turns. Default per type (25-30)."
        }
      },
      "required": ["subagent_type", "prompt"]
    }`)
}

// 9 方法接口实现
func (t *SubagentTool) IsReadOnly() bool                     { return false }  // subagent 可能写
func (t *SubagentTool) NeedsReadFirst() bool                 { return false }
func (t *SubagentTool) RequiresWorkspace() bool              { return false }
func (t *SubagentTool) ValidateInput(args json.RawMessage) error
func (t *SubagentTool) CheckPermissions(args json.RawMessage, mode toolapp.PermissionMode) toolapp.PermissionResult
func (t *SubagentTool) Execute(ctx context.Context, argsJSON string) (string, error)
```

`Execute` 主体：

```go
func (t *SubagentTool) Execute(ctx context.Context, argsJSON string) (string, error) {
    // 1. 防嵌套（双保险，registry filter 外加 ctx 检查）
    if depth := reqctxpkg.GetSubagentDepth(ctx); depth >= 1 {
        return "", subagentdomain.ErrRecursionAttempt
    }
    
    // 2. 解析参数
    var args struct{ SubagentType, Prompt string; MaxTurns int }
    json.Unmarshal([]byte(argsJSON), &args)
    
    // 3. Spawn
    result, err := t.svc.Spawn(ctx, args.SubagentType, args.Prompt,
        subagentapp.SpawnOpts{MaxTurns: args.MaxTurns})
    if err != nil {
        return "", err  // 上层走 errmap
    }
    
    // 4. 终态文案
    switch result.Run.Status {
    case "max_turns":
        return result.Result + "\n\n[note: subagent hit max turns]", nil
    case "cancelled":
        return result.Result + "\n\n[note: subagent was cancelled]", nil
    default:
        return result.Result, nil
    }
}
```

---

## 8. 防递归机制（双保险）

### 保险 1：tool registry 物理排除（结构性）

`Service.Spawn` 构建 `SubRunOpts.Tools` 时：

```go
filteredTools := make([]toolapp.Tool, 0, len(allTools))
for _, tool := range allTools {
    if tool.Name() == "Subagent" {
        continue  // 子 agent 永远看不到 Subagent 工具
    }
    if len(typ.AllowedTools) > 0 && !contains(typ.AllowedTools, tool.Name()) {
        continue  // 类型白名单进一步过滤
    }
    filteredTools = append(filteredTools, tool)
}
```

**这是主要防线**——LLM 看不到工具就不可能调用它。OpenCode 47-session 嵌套事故的反面教训。

### 保险 2：ctx depth 检查（运行时兜底）

`SubagentTool.Execute` 第一行 `reqctxpkg.GetSubagentDepth(ctx) >= 1` → 拒绝。**理论上不会触发**（保险 1 应已挡住），但兜底捕获 bridge bug 或测试场景。

### 为什么不用单纯 max_depth 计数

OpenCode 教训：单纯计数允许"depth=2 但每个 200 turns"的等效灾难。计数器是**辅助**，结构性排除才是**主要**。

---

## 8.5. 失败 / 取消 / 并发控制

### Cancellation 级联（parent → subagent）

**问题**：主对话被用户 cancel → subagent 不知道继续烧 token。

**设计**：
- Service.Spawn 内部 `subCtx, cancel := context.WithCancel(parentCtx)`——parent ctx cancel 自动级联
- sub-runner 检测到 ctx.Done → 终止 ReAct 循环
- SubagentRun.Status = "cancelled" + EndedAt 写盘
- 已发出的 tool 调用走各自 tool 的 cancel 链（如 MCP 走 `notifications/cancelled`）

### Subagent 总超时

**问题**：tool call 卡住（如 MCP server 死锁）→ subagent 永不退出 + 无限烧 token。

**设计**：每 subagent 起一个**总超时**（默认 5 分钟，可 SubagentType 内 override）：
```go
runCtx, cancel := context.WithTimeout(subCtx, 5*time.Minute)
defer cancel()
```
超时 → status="failed" + ErrorMsg="total run timeout"。

### Panic 恢复

**问题**：sub-runner 内部某 tool 实现 panic → run 卡 running 永不终态。

**设计**：sub-runner 入口 `defer recover()`：
```go
defer func() {
    if r := recover(); r != nil {
        run.Status = "failed"
        run.ErrorMsg = fmt.Sprintf("panic: %v", r)
        run.EndedAt = ptr(time.Now())
        s.repo.UpdateRun(ctx, run)
        s.bridge.Publish(ctx, "", eventsdomain.Subagent{SubagentRun: run})
        zap.L().Error("subagent panic recovered", zap.Any("panic", r), zap.String("runID", run.ID))
    }
}()
```

### 并发 subagent 隔离

**问题**：同 turn LLM 起 3 个 sibling subagent，token 累计 / SubagentTokenLog 别撞车。

**设计**：
- agentstate.SubagentTokenLog 按 RunID 分桶存储——每个 sibling 独立计数
- 各 subagent 用独立 SubagentRun.ID + 独立 ctx（虽共享 parent ctx，但有自己的 cancel）
- chat runner 的 partitionByExecutionGroup 已经能正确并行调度

### Conversation 删除时数据处理

**决策**：`subagent_runs` 与 `subagent_messages` **不级联删**——保留独立审计：
- 用户在 UI 删 conversation → 只软删 conversation 行
- subagent_runs 通过 ListByConversation 自然过滤掉（因为 conversation 找不到了）
- 但数据库里仍能查到（DBA / 历史回查可用）
- 演化方向：未来加"超 N 天硬清"配置，分两步删

---

## 9. Token Accounting（业界血泪共识）

### 实时累计

`SubRunOpts.OnTokens` 回调由 sub-runner 每 step 末调用：

```go
// app/chat/subrunner.go 内
opts.OnTokens(streamEvent.PromptTokens, streamEvent.CompletionTokens)
```

`Service.Spawn` 实现该回调：

```go
opts.OnTokens = func(in, out int) {
    run.TotalTokensIn += in
    run.TotalTokensOut += out
    s.repo.Update(ctx, run)  // 实时落库
    
    // agentstate 累计（用于 conversation 详情显示）
    if state := agentstatepkg.From(ctx); state != nil {
        state.AddSubagentTokens(typeName, in, out)
    }
}
```

### 主对话内 agentstate 累计

`pkg/agentstate/agentstate.go` 新增：

```go
type SubagentTokenEntry struct {
    TypeName  string
    TokensIn  int
    TokensOut int
    RunID     string
}

func (s *AgentState) AddSubagentTokens(typeName string, in, out int) { ... }
func (s *AgentState) SubagentTokenLog() []SubagentTokenEntry        { ... }
```

### 日志（zap）

每 SubagentRun 终态时 INFO 级日志：

```
{"level":"info","msg":"subagent.run.completed",
 "run_id":"sar_abc...","type":"Explore","status":"completed",
 "tokens_in":4523,"tokens_out":1278,"steps":7,"duration_ms":3421}
```

**v1 不强制预算上限**，但日志使任何 runaway loop 都可被发现 + retro 加 budget 简单（log 行就是 budget 决策依据）。

---

## 10. SSE 事件 — 全合到 chat.message 一条流

subagent **不发自己的 SSE 事件**。所有信息（消息内容 + run 元数据 + 流式 lifecycle）**全部通过 `chat.message` 推**——subagent 上下文的 chat.message 载荷里嵌套一个完整 `subagentRun` 快照。前端拿一个事件就有所有信息。

### 唯一事件：`chat.message`，subagent 上下文时载荷"加料"

```json
event: chat.message
data: {
  // ── 标准 Message 快照（与主对话同 schema）──
  "id": "smm_a1b2c3...",
  "role": "assistant",
  "blocks": [
    {"type":"reasoning", "content":"Let me search..."},
    {"type":"tool_call", "name":"Read", "args":{...}},
    {"type":"tool_result", "content":"Read 250 lines"},
    {"type":"text", "content":"Found 3 matches in foo.go"}
  ],
  "status": "streaming",
  "promptTokens": 642,
  "completionTokens": 89,
  "createdAt": "2026-05-05T10:42:01Z",
  "updatedAt": "2026-05-05T10:42:01.4Z",

  // ── subagent 上下文专属字段（仅 subagent 内 sub-runner 推时携带）──
  "subagentRunId": "sar_xyz",                          ← 标识 + 路由 key
  "parentConversationId": "cv_main",                   ← 让前端知道往哪挂
  "subagentRun": {                                     ← ⚡完整 SubagentRun 快照
    "id": "sar_xyz",
    "parentConversationId": "cv_main",
    "parentMessageId": "msg_main",
    "parentToolCallId": "tc_xxx",
    "type": "Explore",
    "prompt": "Find all files mentioning 'progressive disclosure'",
    "result": "",
    "status": "running",
    "totalTokensIn": 4523,
    "totalTokensOut": 1278,
    "stepsUsed": 3,
    "model": "deepseek-chat",
    "startedAt": "2026-05-05T10:42:00Z",
    "lastToolCalled": "Grep",
    "lastToolArgsBrief": "{pattern:'progressive disclosure'}",
    "lastToolResultBrief": "Found 7 matches in 4 files",
    "lastStepDurationMs": 421,
    "lastStepAt": "2026-05-05T10:42:01.4Z"
  }
}
```

**主对话消息**则**不**携带 `subagentRunId` / `parentConversationId` / `subagentRun` 字段（omitempty）——wire 完全向后兼容。

### 触发点

每个 chat.message 事件触发都同时刷新嵌入的 `subagentRun` 快照：
- subagent sub-runner 每 token / 每 tool_call 开始 / 每 tool_result 完成 / message complete
- 每次推都带**当前最新**的 SubagentRun 状态（含累计 token、stepsUsed、lastTool* 等）

**spawn / 终态也推**——spawn 时第一个 message stub + status=running；终态时 last message complete + status=completed/cancelled/max_turns/failed。

### 过滤 key

`parentConversationId`——前端订阅一个 conversation 同时收到主对话 + 该对话所有 subagent 的全部消息。

### 前端怎么用

```javascript
sse.on('chat.message', (data) => {
  if (data.subagentRunId) {
    // subagent 消息
    // - data.blocks 是当前 subagent 在说什么（流式更新）
    // - data.subagentRun 是当前 run 的全部状态（token / status / lastTool / 等）
    // 按 runId 分组渲染到对应小窗
    smallWindows[data.subagentRunId].render({
      message: data,                    // 流式内容
      run: data.subagentRun,            // 状态条 / token 计数 / 当前在干啥
    });
  } else {
    // 主对话消息
    mainChat.render(data);
  }
});
```

**前端只懂一种事件 = chat.message**。subagent 的"炫酷小窗"也是渲染这同一种事件，只是按 `subagentRunId` 字段分流，并额外读 `subagentRun` 子对象做 lifecycle UI。

### 设计带来的好处

1. **后端 0 新事件类型**——直接复用 chat.message
2. **前端 0 新协议**——chat.message 渲染逻辑直接复用，仅按 subagentRunId 分流 + 读 subagentRun 子对象
3. **单事件单事实源**——message 状态 + run 状态总在同一帧推，不会出现"chat.message 已到 subagent 事件还没到"的对齐问题
4. **多 subagent 并发**——天然按 runId 分组
5. **Skill `context: fork`**——自动获得流式 UI，因为 fork 走 subagent 服务

### 后端实现要点

`chat.message` 事件 struct 在 `domain/events/types.go`：

```go
type ChatMessage struct {
    *chatdomain.Message
    SubagentRunID        string                       `json:"subagentRunId,omitempty"`
    ParentConversationID string                       `json:"parentConversationId,omitempty"`
    SubagentRun          *subagentdomain.SubagentRun  `json:"subagentRun,omitempty"`
}
```

主对话推时三个 subagent 字段都 nil/empty → omitempty 序列化时不出现。
subagent 推时三字段全填 → 前端拿到完整 message + 完整 run。

---

## 11. HTTP API

| Method + Path | 用途 | 响应 |
|---|---|---|
| `GET /api/v1/conversations/{id}/subagent-runs` | 列对话下所有 run（UI 历史 / cost analysis）| `{data: [SubagentRun...]}` |
| `GET /api/v1/subagent-runs/{id}` | 单 run 详情（prompt + result）| `{data: SubagentRun}` |
| `GET /api/v1/subagent-runs/{id}/messages` | run 内全部 messages（**流式小窗回放用**）| `{data: [SubagentMessage...]}` |
| `GET /api/v1/subagent-types` | 列所有可用 subagent 类型（UI 下拉）| `{data: [SubagentType...]}` |
| `POST /api/v1/subagent-runs/{id}:cancel` | **v1 不实现**——主对话 cancel 间接 cancel 所有 sub | — |

`GET /messages` 让前端能"回放"历史 subagent run——UI 打开某条 SubagentRun 详情时拉全部消息，按 seq 渲染，再现当时的流式过程（不带 token 节奏，但内容完整）。

---

## 12. 错误码（`transport/httpapi/response/errmap.go`）

| Sentinel | HTTP | Wire Code |
|---|---|---|
| `subagentdomain.ErrTypeNotFound` | 404 | `SUBAGENT_TYPE_NOT_FOUND` |
| `subagentdomain.ErrRecursionAttempt` | 422 | `SUBAGENT_RECURSION` |
| `subagentdomain.ErrMaxTurnsExceeded` | (不到 handler，tool 内吞) | — |
| `subagentdomain.ErrCancelled` | (不到 handler，tool 内吞) | — |

仅前两个上抛 handler。后两个由 Tool.Execute 转友好字符串返 LLM。

---

## 13. 测试覆盖（实际，截至 D4-5 收尾 2026-05-06）

| 层 | 文件 | 测试数 | 覆盖 |
|---|---|---|---|
| store | `internal/infra/store/subagent/subagent_test.go` | 14 | Run CRUD（含 NotFound、状态迁移、跨对话隔离、按 StartedAt 倒序 List）/ Message AppendSeq 单调（含 12-goroutine 并发竞争）+ UpdatePreservesSeq + chatdomain.Block 复用 round-trip + transient gorm:"-" 字段不落库 |
| events | `internal/domain/events/types_test.go` | 5 | ChatMessage MarshalJSON wire 兼容（零 subagent 字段时字节级与原 Message 一致）+ subagent 三字段 set 时正确合并到顶层 + omitempty 半填场景 + EventName 稳定 |
| agentstate | `internal/pkg/agentstate/agentstate_test.go` | 9 | MarkRead/WasRead + Cwd round-trip + SubagentTokenLog 顺序保留 + 32-goroutine 并发 append 求和不丢条目 + 返 copy 非别名 |
| app/subagent | `internal/app/subagent/subagent_test.go` | 14 | Registry Get/List 排序 + 内置 3 类型契约（Explore 白名单纯只读 / general-purpose AllowedTools=nil 标记） + filterTools 防递归（白名单含 Subagent 也强制剥）+ composeSystemPrompt locale 行为 |
| app/tool/subagent | `internal/app/tool/subagent/agent_test.go` | 18 | Identity / 静态元数据 / 必填字段声明 / ValidateInput 5 路径 / CheckPermissions 跨 mode 全 Allow / 运行时递归守卫（depth≥1 拒绝）/ Execute 解析失败 / appendNote 三 body 形态 |
| handlers/subagent | `internal/transport/httpapi/handlers/subagent_test.go` | 8 | ListRuns 过滤+排序 + 空结果 / GetRun 200+404 / ListMessages 按 Seq 排序 + 空 run / ListTypes 内置 3 个按 alpha |
| pipeline | `backend/test/subagent/subagent_test.go` | 3 | Spawn 端到端（parent → SubagentTool → Service.Spawn → loop.Run → tool_result 回 parent，DB 行 + message 落实）/ SSE 携带 subagentRun 快照（chat.message 带 subagentRunId + parentConversationId + 完整 SubagentRun JSON）/ max_turns 触发 + parent tool_result 含 "[note: hit max turns]" 注脚 |

**总计 71 测试**（68 单测 + 3 pipeline）。

> V1 范围说明：结构性防递归（filterTools 剥 SubagentTool）已在 app/subagent 单测覆盖；嵌套 spawn 在 sub-runner 内会降级为 "tool not found"（layer-1 设计中），加 pipeline 重复证明无价值。运行时递归守卫（depth ≥ 1 拒绝）由 app/tool/subagent 单测覆盖。

---

## 14. 与其他 domain 的关系

| 关系 | 说明 |
|---|---|
| **loop** | 共享 ReAct 引擎；subagent 直接调 `loop.Run(ctx, subagentHost{...}, client, req, maxSteps)` |
| **chat** | **无直接依赖**——双方都是 loop 的调用方；Skill `context: fork` 时 chatHost 内部可触发 spawn，但两个服务之间不互相 import |
| **events** | events.ChatMessage 嵌入 `*subagentdomain.SubagentRun`（subagent 上下文 message 推送时携带，主对话推送时 omitempty）|
| **reqctx** | 新增 `SubagentDepth` / `SubagentRunID` ctx key |
| **agentstate** | 新增 `SubagentTokenLog` 字段（per-conversation accumulator）|
| **skill** | Skill 的 `context: fork` 字段调本 Service.Spawn 实现复用 |
| **catalog** | **不实现 CatalogSource**——Subagent tool 自身 description 已覆盖 subagent 类型说明，catalog 不重复 |

### 包依赖方向（无循环 import 设计）

```
internal/app/loop/           （通用 ReAct 引擎，不依赖任何业务 service）
        ↑                    ↑
        ├── chat 调用         ├── subagent 调用
        │                     │
internal/app/chat/      internal/app/subagent/
   (chatHost 实现)          (subagentHost 实现 + Service.Spawn)
        ↓                          ↓
        └─── 共同依赖：loop / domain/* / infra/store/* / events bridge
```

无 port 接口、无 DI 注入：两个 service 各自构造自己的 Host 实现，调同一个 `loop.Run` 函数。chat 不知道 subagent 存在；subagent 不知道 chat 存在。Workflow（Phase 4）/ Skill `context: fork` 未来同样直接接 `loop.Host`。

---

## 15. 演化方向

- **跨厂 subagent 定义**：从代码内置改文件加载（`~/.forgify/subagents/<name>.md` YAML frontmatter，类似 Skill）
- **token budget 强约束**：基于 `agentstate.SubagentTokenLog` 实时累计 + 用户配的对话级上限触发拒绝
- **并发 subagent**：当前一次只能 spawn 一个（同 group 排队）；未来允许同 ExecutionGroup 并发 spawn 多个 subagent
- **subagent 内嵌套**：当前禁；未来如有强需求（罕见），加可配的 `MaxDepth=N`，但默认仍是 1
- **SubagentMessage 软删归档**：当前 run 删除时 message 不级联删（保留独立审计）；后续可加"归档周期"配置定期清理超 N 天历史
