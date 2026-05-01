# chat domain — 详细设计文档

**所属 Phase**：Phase 2 起（每个 Phase 都会升级）
**状态**：✅ 已实现到 Phase 3（含 chat 基础设施重构 2026-04-27 + pipeline → runner 二次重构）；Phase 4-5 时再升级
**地位**：**全系统最核心的 domain**——用户的每一次对话都从这里进入，一切能力都通过这里编排。

**关联文档**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/api-design.md`](../service-contract-documents/api-design.md) — API 索引
- [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) — 事件索引

---

## 1. 核心思想：一切都是 Tool Call

### 1.1 为什么

Forgify 的终极形态是：用户一句话，AI 自主完成"创建工具→测试→组建工作流→挂知识库→部署"的完整链路，中间多次迭代，用户实时看到每一步。

这本质上是一个**自主 Agent 循环**，而不是简单的"识别意图→路由→执行一次"。

### 1.2 是什么

从 LLM 的视角，它只有两种输出：
- **直接回复**（= 任务完成）
- **调一个 Tool**（= 还有事情要做）

所有 Forgify 的能力——创建工具、运行沙箱、搜知识库、创建工作流——对 LLM 都是 Tool。Agent 每轮只做一个决策（调哪个 Tool 或直接回复），拿到结果后再想下一步，直到认为任务完成。

这就是 **ReAct 循环**（Reasoning + Acting），和 Claude Code 的工作方式完全一致。

### 1.3 关键约束

**每个小轮次只有一次 Tool Call。** 这不是限制，这是优点：
- 每一步都可观测（实时推事件给前端）
- 每一步都可中断
- LLM 的推理链清晰可追溯
- 不会一口气做完所有事情让用户措手不及

---

## 2. 两层工具体系

这是整个设计最关键的决策。

### 2.1 问题

用户最终可能创建数百个工具。如果把所有工具都塞进 LLM context，性能严重下降，LLM 会选错工具，最重要的系统工具会被淹没。

### 2.2 解法

```
┌─────────────────────────────────────────────────────┐
│                  Agent Context                       │
│                                                      │
│  System Tools（永远在 context，~8 个）               │
│  ┌────────────┐ ┌──────────┐ ┌────────────────────┐ │
│  │ create_tool│ │ edit_tool│ │     run_tool(id)    │ │
│  └────────────┘ └──────────┘ └────────────────────┘ │
│  ┌─────────────┐ ┌──────────────────────────────┐   │
│  │ search_tools│ │  create_workflow / run_workflow│   │
│  └─────────────┘ └──────────────────────────────┘   │
│  ┌──────────────────┐ ┌──────────┐                  │
│  │ search_knowledge  │ │ mcp_call │                  │
│  └──────────────────┘ └──────────┘                  │
└─────────────────────────────────────────────────────┘

用户工具库（不在 context，通过 search_tools 发现，run_tool 执行）
┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│ email_parser │ │ csv_processor│ │  ...（数百个）│
└──────────────┘ └──────────────┘ └──────────────┘
```

**System Tools** 是 meta-tools：用来创建/管理其他工具和工作流。永远可见。

**User Tools** 不直接注入 context。Agent 通过：
1. `search_tools(query)` → 语义搜索工具库，得到相关工具列表
2. `run_tool(id, input)` → 通用执行器，执行任意用户工具

这本质上是 **Tool RAG**——与知识库 RAG 同一个思路，检索对象是工具描述。

### 2.3 System Tools 完整目录

| Tool | Phase | 描述 | 对接的 domain |
|---|---|---|---|
| `create_tool` | 3 | 创建新工具（名称/描述/代码）| forge + tool |
| `edit_tool` | 3 | 修改已有工具的代码或描述 | tool |
| `run_tool` | 3 | 通用执行器，按 id 运行任意工具 | tool sandbox |
| `search_tools` | 3 | 语义搜索工具库，返回相关工具列表 | tool（FTS5）|
| `create_workflow` | 4 | 创建工作流（DAG 节点定义）| workflow |
| `edit_workflow` | 4 | 修改已有工作流 | workflow |
| `run_workflow` | 4 | 执行工作流，返回运行结果 | flowrun |
| `search_knowledge` | 5 | RAG 检索知识库 | knowledge |
| `mcp_call` | 5 | 调用 MCP 服务器的方法 | mcpserver |

**Phase 2**：tools 列表为空。Agent 就是一个没有工具的 ReAct Agent，行为等同于纯 LLM 流式对话，但架构已经是可扩展的。

---

## 3. LLM 客户端层（`infra/llm`）

> **Eino 已完全移除**（2026-04-27）。chat 管线使用完全自有的 LLM 流式客户端，
> 零框架依赖，完全掌控 SSE 解析和请求构建。

### 3.1 核心组件

```
chat.Service
    ↓ 依赖
llminfra.Factory          ← 按 provider dispatch，返回 Client
    ↓ Build(Config)
llminfra.Client           ← 唯一方法：Stream(ctx, Request) iter.Seq[StreamEvent]
    ├── openAIClient      ← 覆盖 OpenAI/DeepSeek/Qwen/Moonshot/Ollama 等 OpenAI-compat
    └── anthropicClient   ← Anthropic 原生 /v1/messages 协议
```

### 3.2 核心类型（`infra/llm/llm.go`）

```go
// StreamEvent 是 LLM 流式响应中一个带类型标签的事件
type StreamEvent struct {
    Type           StreamEventType
    Delta          string   // EventText: 文字增量
    ReasoningDelta string   // EventReasoning: 推理增量（DeepSeek-R1 等）
    ToolIndex      int      // EventToolStart / EventToolDelta
    ToolID         string   // EventToolStart: LLM 分配的 tool call id
    ToolName       string   // EventToolStart
    ArgsDelta      string   // EventToolDelta: arguments 片段
    FinishReason   string   // EventFinish
    InputTokens    int      // EventFinish
    OutputTokens   int      // EventFinish
    Err            error    // EventError
}

type StreamEventType string
const (
    EventText      StreamEventType = "text"
    EventReasoning StreamEventType = "reasoning"
    EventToolStart StreamEventType = "tool_start"  // tool name 已知，立刻可推 SSE
    EventToolDelta StreamEventType = "tool_delta"  // arguments 片段
    EventFinish    StreamEventType = "finish"
    EventError     StreamEventType = "error"
)

// Client 是唯一的 LLM 流式接口
type Client interface {
    Stream(ctx context.Context, req Request) iter.Seq[StreamEvent]
}

type Request struct {
    ModelID  string
    Key      string
    BaseURL  string
    System   string
    Messages []LLMMessage
    Tools    []ToolDef
}
```

**设计关键**：
- `iter.Seq[StreamEvent]` 替代 channel：拉式迭代，无 goroutine 泄漏，break 干净退出
- `EventToolStart` 在 tool name 首次出现时立刻 emit，不等 arguments 完整（让前端尽快展示"正在调用 X…"）
- `Generate()` helper 消费 Stream 实现非流式调用，不引入独立接口

### 3.3 OpenAI 兼容客户端（`infra/llm/openai.go`）

覆盖所有 OpenAI-compat provider：openai / deepseek / qwen / moonshot / doubao / openrouter / ollama 等。

- 自写 SSE line reader（`data: {...}\n\n` 格式）
- 解析 delta chunks：`choices[0].delta.content` / `reasoning_content`（DeepSeek-R1）/ `tool_calls`
- `classifyHTTPError` 区分 401/429/400/404/5xx 返回对应 Go error
- 畸形 chunk → emit EventError，不 panic

### 3.4 Anthropic 原生客户端（`infra/llm/anthropic.go`）

使用 Anthropic 原生 `/v1/messages` 协议（SSE 格式）：
- `content_block_start` → 识别 text / tool_use block
- `content_block_delta` → 分发 EventText / EventToolDelta
- `content_block_stop` → 关闭当前 block
- tool result 消息格式与 OpenAI 不同：按 Anthropic 协议将 tool results 合并为一条 `role="user"` 消息（`content = [{type:"tool_result", tool_use_id, content}...]`）

### 3.5 Factory（`infra/llm/factory.go`）

```go
// Factory.Build 按 provider 返回对应 Client
func (f *Factory) Build(cfg Config) (Client, string, error) {
    // anthropic → anthropicClient{baseURL}
    // 其余全部 → openAIClient{baseURL}（含 ollama 等）
}
```

Provider 基础 URL 由 `resolveBaseURL` 按 provider 名称给出，调用方传入的 `BaseURL` 会覆盖默认值。

---

## 4. Tool 接口 & summary 注入（`app/agent/tool.go`）

### 4.1 Tool 接口

```go
// Tool 是每个 system tool 必须实现的接口
type Tool interface {
    Name()        string
    Description() string
    Parameters()  json.RawMessage  // JSON Schema object，禁止包含 "summary" 字段
    Execute(ctx context.Context, argsJSON string) (string, error)
}
```

`Parameters()` 返回的 JSON Schema **不含** `summary` 字段，由框架在 `ToLLMDef` 时自动注入。

### 4.2 Summary 注入机制

```
ToLLMDef(tool)
  → injectSummaryField(tool.Parameters())
    → 在 properties 里加入 "summary": {type: string, description: "One sentence..."}
    → 在 required 里把 "summary" 插到第一位（引导 LLM 优先输出）
  → 返回发给 LLM 的 ToolDef（含 summary 字段）

runOneTool(ctx, tc)
  → StripSummary(tc.Arguments)
    → 提取 summary 字段值 → 发 ChatToolCall SSE
    → 返回去掉 summary 后的 argsJSON
  → tool.Execute(ctx, strippedArgsJSON)   ← Execute 永不看到 summary
```

**为什么**：summary 是给前端展示的人类可读摘要（如 `"$ git status"`），不是工具的真实入参。让 LLM 输出 summary 比 tool 自己生成更准确，也无需为每个 tool 写摘要逻辑。

### 4.3 Context Helpers

```go
// 6 个 context helpers，供 runner.go 注入、forge.go 中的 tools 读取
func WithConversationID(ctx, id) context.Context
func GetConversationID(ctx) (string, bool)

func WithMessageID(ctx, id) context.Context
func GetMessageID(ctx) (string, bool)

func WithToolCallID(ctx, id) context.Context
func GetToolCallID(ctx) (string, bool)
```

`runOneTool`（tools.go）在调用 `tool.Execute` 前注入 msgID 和 toolCallID，`forge.go` 中的 `streamCode` / `CreateTool.Execute` / `EditTool.Execute` 读取并填充 `ToolCodeStreaming` / `ToolCreated` / `ToolPendingCreated` SSE 事件的对应字段。

### 4.4 System Tools 完整目录

| Tool | 实现文件 | Phase | 描述 |
|---|---|---|---|
| `datetime` | system.go | 2+ | 当前日期时间 |
| `read_file` | system.go | 2+ | 读本地文件 |
| `write_file` | system.go | 2+ | 写本地文件 |
| `list_dir` | system.go | 2+ | 列目录 |
| `run_shell` | system.go | 2+ | 执行 shell 命令 |
| `run_python` | system.go | 2+ | 执行 Python 代码 |
| `web_search` | web.go | 2+ | DuckDuckGo Lite 搜索（POST 表单）|
| `fetch_url` | web.go | 2+ | Jina Reader 抓取 URL 为 Markdown |
| `search_tools` | forge.go | 3+ | 语义搜索用户工具库 |
| `get_tool` | forge.go | 3+ | 获取工具完整代码 |
| `create_tool` | forge.go | 3+ | LLM 生成代码 + 保存工具 |
| `edit_tool` | forge.go | 3+ | LLM 改写代码 + 创建 pending |
| `run_tool` | forge.go | 3+ | 运行用户工具（沙箱） |

---

## 5. Pipeline 架构（`app/chat/`）

### 5.1 文件结构（6 文件）

```
app/chat/
  chat.go     ← 公开 API（Send / Cancel / ListMessages / UploadAttachment）+ Service struct + queue 管理常量
  runner.go   ← getOrCreateQueue / runQueue / processTask / agentRun（ReAct loop）/ writeDB / stampBlocks
  stream.go   ← streamLLM（iter.Seq 驱动）+ assembleBlocks + extractToolCalls + parseToolArgs
  tools.go    ← runTools（并行）+ runOneTool + executeTool
  history.go  ← buildHistory + extendHistory + blocksToLLM + blocksToAssistantLLM + buildUserLLMMessage + attachmentToPart
  util.go     ← ID 生成器（newMsgID / newBlockID / newAttachmentID）+ readAndEncode + truncate
```

> 2026-04-27 重构后从 `app/chat/chat.go` 单文件拆为 5 文件；2026-04-27 后又把原 `pipeline.go` 替换为 `runner.go`（concept compaction 预留）。函数名也随之精简：`runReactLoop` → `agentRun`，`persistMsg` + `finalPersist` 合并为带 `fatal bool` 的 `writeDB`。

### 5.2 ReAct Loop（`agentRun`，runner.go）

```
Send(userMsg) → 保存 user message → 入队 queuedTask{userMsgID}
  ↓ worker goroutine
processTask
  → modelPicker.PickForChat → keyProvider.ResolveCredentials → llmFactory.Build
  → 组装 baseReq（System / Tools 注入）
  → agentRun(ctx, uid, conv, userMsgID, client, baseReq)
      → buildHistory(ctx, convID, userMsgID)     // 加载历史，userMsgID 末尾追加
      → for step < maxSteps (=20):
          aBlocks, toolCalls, stopReason, iT, oT = streamLLM(ctx, client, req, convID, msgID)
          allBlocks = append(allBlocks, aBlocks...)
          totalInput += iT; totalOutput += oT

          if stopReason == cancelled / error:
              writeDB(allBlocks, status=cancelled|error, fatal=true) → break

          if len(toolCalls) == 0:
              writeDB(allBlocks, status=completed, stopReason, fatal=true) → break  // 最终答案

          rBlocks = runTools(ctx, toolCalls, convID, msgID)   // 并行
          allBlocks = append(allBlocks, rBlocks...)
          writeDB(allBlocks, status=streaming, fatal=false)   // checkpoint，buildHistory 会跳过
          history = extendHistory(history, aBlocks, rBlocks)  // 把本步的 assistant + tool result 拼回历史
          // TODO: context compaction 钩子点

      if !finalWritten:                                       // 步骤上限
          writeDB(allBlocks, status=completed, stopReason=max_tokens, fatal=true)

      Publish(ChatDone)
      if conv.Title == "" && !conv.AutoTitled:
          go autoTitle(...)
```

**关键设计**：
- **allBlocks 累积**：所有步骤的 blocks 全部累积进一个 slice，最终一次性写入同一条 assistant 消息。一次用户发言对应一条完整的 DB 记录，工具调用链不丢失。
- **中间步 streaming checkpoint**：中间步用 `writeDB(fatal=false)` 写 `status=streaming`，`buildHistory` 跳过 streaming/pending 状态的消息，避免把未完成的步骤放进历史重建。失败只 warn，最终写会覆盖。
- **`fatal=true` 走 detached context**：终态写用 `reqctxpkg.SetUserID(context.Background(), uid)` 创建全新 context，不受取消影响，保证终态必然落库。

### 5.3 streamLLM（`stream.go`）

```go
// iter.Seq 拉式迭代：只要 for range 不 break，就一直消费
for event := range client.Stream(ctx, req) {
    switch event.Type {
    case EventToolStart:
        bridge.Publish(ChatToolCallStart{ToolCallID: event.ToolID, ToolName: event.ToolName})
        accum[event.ToolIndex] = newAccum(event.ToolID, event.ToolName)
    case EventToolDelta:
        accum[event.ToolIndex].args.WriteString(event.ArgsDelta)
    case EventText:
        textBuf.WriteString(event.Delta)
        bridge.Publish(ChatToken{Delta: event.Delta})
    case EventReasoning:
        reasonBuf.WriteString(event.ReasoningDelta)
        bridge.Publish(ChatReasoningToken{Delta: event.ReasoningDelta})
    case EventFinish:
        totalInput, totalOutput = event.InputTokens, event.OutputTokens
    case EventError:
        return nil, nil, StopReasonError, ...
    }
}
return assembleBlocks(textBuf, reasonBuf, accum), extractToolCalls(blocks), stopReason, totalInput, totalOutput
```

`assembleBlocks` 把 buffers 组装为 blocks：顺序为 reasoning block → tool_call blocks（按 index 排） → text block。`extractToolCalls` 从 blocks 抽出 tool_call 列表交给 runTools。`parseToolArgs` 拆分 LLM 输出的 arguments JSON 为 `{summary, args}`。

### 5.4 并行 Tool Call（`tools.go`）

```go
func (s *Service) runTools(ctx, calls, convID, msgID) []chatdomain.Block {
    ch := make(chan result, len(calls))
    var wg sync.WaitGroup
    for i, call := range calls {
        wg.Add(1)
        go func(idx int, tc ToolCallData) {
            defer wg.Done()
            ch <- result{idx, s.runOneTool(ctx, tc, convID, msgID, idx)}
        }(i, call)
    }
    wg.Wait(); close(ch)
    blocks := make([]Block, len(calls))
    for r := range ch { blocks[r.idx] = r.block }   // 还原 index 顺序，保证 block seq 确定
    return blocks
}
```

`runOneTool` 调用 `executeTool` 前注入 `WithMessageID` / `WithToolCallID` 到 ctx，供 forge.go 中的工具读取并填充 SSE 事件字段。`executeTool` 是真正调 `tool.Execute(ctx, argsJSON)` 的最内层。

### 5.5 writeDB 与取消安全

```go
func (s *Service) writeDB(ctx, msgID, convID, uid, blocks, status, stopReason, ..., fatal bool) {
    saveCtx := ctx
    if fatal {
        // Fresh context: 已取消的流不能阻止终态写入
        saveCtx = reqctxpkg.SetUserID(context.Background(), uid)
    }
    if err := s.repo.Save(saveCtx, msg); err != nil {
        if fatal {
            log.Error("CRITICAL: final assistant message persist failed")
            bridge.Publish(ChatError{Code: "INTERNAL_ERROR"})
        } else {
            log.Warn("streaming checkpoint persist failed, continuing")
        }
    }
}
```

取消流程：Cancel() → context cancelled → streamLLM break → agentRun 返回已有 blocks → writeDB(status=cancelled, fatal=true)。终态必然落库。

---

## 6. 消息存储（Block 模型）

### 6.1 messages 表（精简为纯元数据）

```go
type Message struct {
    ID             string         `gorm:"primaryKey;type:text" json:"id"`
    ConversationID string         `gorm:"not null;index;type:text" json:"conversationId"`
    UserID         string         `gorm:"not null;type:text" json:"-"`
    Role           Role           `gorm:"not null;type:text" json:"role"`  // user | assistant
    Status         Status         `gorm:"not null;type:text" json:"status"`
    StopReason     string         `gorm:"type:text" json:"stopReason,omitempty"`
    InputTokens    int            `json:"inputTokens,omitempty"`
    OutputTokens   int            `json:"outputTokens,omitempty"`
    Blocks         []Block        `gorm:"-" json:"blocks"`  // 查询时填充，不存这列
    CreatedAt      time.Time      `json:"createdAt"`
    DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}
```

**已移除**：`Content`、`ReasoningContent`、`ToolCalls`、`ToolCallID`、`AttachmentIDs`、`TokenUsage`（全部转为 `message_blocks`）。

**Role 值**：`user` | `assistant`（`tool` 角色已移除，tool result 变为 assistant 消息内的 block）。

**Status 常量**：`pending` | `streaming` | `completed` | `error` | `cancelled`

**StopReason**：`end_turn` | `max_tokens` | `cancelled` | `error`

### 6.2 message_blocks 表（新增，存所有内容）

```go
type Block struct {
    ID        string    `gorm:"primaryKey;type:text" json:"id"`   // blk_<16hex>
    MessageID string    `gorm:"not null;index;type:text" json:"-"`
    Seq       int       `gorm:"not null" json:"seq"`               // 消息内排序
    Type      BlockType `gorm:"not null;type:text" json:"type"`
    Data      string    `gorm:"not null;type:text" json:"data"`    // JSON，结构随 type
    CreatedAt time.Time `json:"createdAt"`
}
```

**Block 类型 & data 结构**：

| Type | data JSON 结构 | 说明 |
|---|---|---|
| `text` | `{"text":"..."}` | 普通文字（user 输入或 assistant 回复）|
| `reasoning` | `{"text":"..."}` | 推理型模型的 thinking 内容 |
| `tool_call` | `{"id":"call_xxx","name":"datetime","summary":"获取时间","arguments":{...}}` | LLM 决定调用某工具 |
| `tool_result` | `{"toolCallId":"call_xxx","ok":true,"result":"..."}` | 工具执行结果 |
| `attachment_ref` | `{"attachmentId":"att_xxx","fileName":"report.pdf","mimeType":"application/pdf"}` | 附件引用 |

### 6.3 chatstore.Save 的 ON CONFLICT 保护

```go
// infra/store/chat/chat.go
tx.Clauses(clause.OnConflict{
    Columns:   []clause.Column{{Name: "id"}},
    DoUpdates: clause.AssignmentColumns([]string{
        "status", "stop_reason", "input_tokens", "output_tokens",
    }),
}).Create(m)
```

`created_at` **不在** DoUpdates 列表里，保证首次 INSERT 写入的时间戳在后续 status 更新时不被覆盖。这解决了 GORM `Save()` upsert 会把零值 `created_at` 写回 DB 的问题。

### 6.4 chat_attachments 表

```go
type Attachment struct {
    ID          string    `gorm:"primaryKey;type:text" json:"id"`       // att_<16hex>
    UserID      string    `gorm:"not null;type:text" json:"-"`
    FileName    string    `gorm:"not null;type:text" json:"fileName"`
    MimeType    string    `gorm:"not null;type:text" json:"mimeType"`
    SizeBytes   int64     `gorm:"not null" json:"sizeBytes"`
    StoragePath string    `gorm:"not null;type:text" json:"-"`  // 不对外暴露
    CreatedAt   time.Time `json:"createdAt"`
}
```

文件存 `{dataDir}/attachments/{att_id}/original.{ext}`，50MB 限制。

### 6.5 历史重建（`history.go`）

#### buildHistory

```go
func (s *Service) buildHistory(ctx, convID, currentUserMsgID string) ([]llminfra.LLMMessage, error)
```

扫描所有非 streaming/pending 消息，跳过 `currentUserMsgID`，末尾显式追加当前用户消息。

**为什么要追加末尾**：同一对话快速连发两条消息时，第二条 user 消息的 `created_at` 可能早于第一条 assistant 回复（队列中并发写入），导致历史排序错乱，LLM 看到 `[user1, user2(current), assistant1]` 末尾是 assistant、无法确定回复对象。

#### extendHistory

```go
func extendHistory(history, aBlocks, rBlocks) ([]llminfra.LLMMessage, error)
```

ReAct 循环每步结束后被 `agentRun` 调一次，把本步的 assistant blocks（含 tool_call）+ tool_result blocks 转为 LLM wire 格式追加到 history，供下一步 LLM 读取。**这是 ReAct 多步对话的核心机制**——LLM 看到自己上一步调了什么、得到什么结果，才能决定下一步。

#### blocksToAssistantLLM / blocksToLLM

把一条 assistant 消息的 blocks 转为 OpenAI wire 格式：

```
[assistant{content, toolCalls, reasoningContent}] + [N × role=tool messages]
```

tool_call blocks → `assistant.ToolCalls[]`；tool_result blocks → 独立的 `role="tool"` 消息（OpenAI 协议要求）。`buildHistory` 加载历史时调用，`extendHistory` 中间步插历史时也调用——同一份转换逻辑统一在 `blocksToAssistantLLM`，避免重复实现漂移。

---

## 7. 附件与多模态支持

### 7.1 上传流程

```
POST /api/v1/attachments (multipart/form-data)
→ 写到 {dataDir}/attachments/{att_id}/original.{ext}
→ 201 { id, fileName, mimeType, sizeBytes }
```

### 7.2 内容提取（`infra/chat/extractor.go`）

`chatinfra.Extract(storagePath, mimeType)` 按 MIME 类型分派：

| 格式 | 实现 |
|---|---|
| `text/*` / `.go` / `.py` / `.json` / `.csv` 等 | `os.ReadFile` |
| `application/pdf` | `dslipak/pdf`（纯 Go）|
| `.docx` / `.odt` / `.rtf` | `lu4p/cat`（纯 Go）|
| `.xlsx` / `.xlsm` | `xuri/excelize`（纯 Go）|
| `.pptx` | stdlib zip + XML 解析 |
| `text/html` | HTML 标签剥离 |
| `image/*` | `IsImage()` → base64 Vision 路径 |

### 7.3 LLM 消息组装

```
图片附件
  → buildUserLLMMessage → attachmentToPart
      → readAndEncode(storagePath) → base64
      → ContentPart{type:"image_url", imageURL:"data:<mime>;base64,..."}

文本附件（提取成功）
  → ContentPart{type:"text", text:"[附件: report.pdf]\n{提取内容}"}

提取失败
  → 软失败：log.Warn + 跳过，其余 parts 正常发送

多 parts → msg.Parts = parts（OpenAI array content 格式）
单 text → msg.Content = text（简化格式，不用 array）
```

---

## 8. SSE 事件

### 8.1 传输机制

```
前端                                  后端
 │                                     │
 ├──GET /api/v1/events?convId=──────→  │  长连接，Bridge 订阅
 │                                     │
 ├──POST /conversations/{id}/messages→ │  202（异步），入队
 │                                     │  ↓ worker goroutine
 │←── event: chat.tool_call_start ───  │  tool name 出现即推（arguments 尚未完整）
 │←── event: chat.token ─────────────  │  每个文字 delta
 │←── event: chat.tool_call ─────────  │  arguments 完整，执行前推
 │←── event: chat.tool_result ───────  │  执行完成
 │←── event: chat.done ──────────────  │  全部结束
```

Keep-alive ping：每 15 秒推 `: keep-alive\n\n` 防代理断连。

### 8.2 Chat 事件完整列表

| 事件 | 触发时机 | 关键字段 |
|---|---|---|
| `chat.token` | 每个文字 delta | `messageId`, `delta` |
| `chat.reasoning_token` | 推理模型 thinking delta | `messageId`, `delta` |
| `chat.tool_call_start` | stream 中 tool name 首次出现 | `messageId`, `toolCallId`, `toolName` |
| `chat.tool_call` | arguments 完整、执行前 | `messageId`, `toolCallId`, `toolName`, `toolInput`, `summary` |
| `chat.tool_result` | 工具执行完成 | `toolCallId`, `result`, `ok` |
| `chat.done` | Agent 全部完成 | `messageId`, `stopReason`, `inputTokens`, `outputTokens` |
| `chat.error` | 不可恢复错误 | `code`, `message` |
| `conversation.title_updated` | auto-titling 回写 | `conversationId`, `title`, `autoTitled` |

---

## 9. HTTP API

### 9.1 端点

| Method | Path | 用途 | 状态码 |
|---|---|---|---|
| `POST` | `/api/v1/attachments` | 上传附件（multipart）| 201 |
| `POST` | `/api/v1/conversations/{id}/messages` | 发送消息，触发 Agent | 202 |
| `DELETE` | `/api/v1/conversations/{id}/stream` | 取消正在运行的 Agent | 204 |
| `GET` | `/api/v1/conversations/{id}/messages` | 消息历史（cursor 分页，含 blocks）| 200 |
| `GET` | `/api/v1/events` | SSE 事件流（`?conversationId=xxx`）| 200 |

### 9.2 GET /conversations/{id}/messages 响应格式

```json
{
  "data": [
    {
      "id": "msg_xxx", "role": "user", "status": "completed",
      "createdAt": "...",
      "blocks": [
        {"id":"blk_1","seq":0,"type":"text","data":"{\"text\":\"帮我...\"}", "createdAt":"..."},
        {"id":"blk_2","seq":1,"type":"attachment_ref","data":"{\"attachmentId\":\"att_xxx\",...}", "createdAt":"..."}
      ]
    },
    {
      "id": "msg_yyy", "role": "assistant", "status": "completed",
      "stopReason": "end_turn", "inputTokens": 1024, "outputTokens": 256,
      "createdAt": "...",
      "blocks": [
        {"id":"blk_3","seq":0,"type":"tool_call","data":"{\"id\":\"call_1\",\"name\":\"datetime\",...}","createdAt":"..."},
        {"id":"blk_4","seq":1,"type":"tool_result","data":"{\"toolCallId\":\"call_1\",\"ok\":true,\"result\":\"...\"}","createdAt":"..."},
        {"id":"blk_5","seq":2,"type":"text","data":"{\"text\":\"当前时间是…\"}","createdAt":"..."}
      ]
    }
  ],
  "nextCursor": "...",
  "hasMore": false
}
```

### 9.3 POST /conversations/{id}/messages

```json
{ "content": "帮我做一个处理 CSV 的工具", "attachmentIds": ["att_xxx"] }
```

→ 202 `{ "data": { "messageId": "msg_xxx" } }`（user 消息 ID，非 assistant）

**错误**：404 `CONVERSATION_NOT_FOUND` / 409 `STREAM_IN_PROGRESS` / SSE 推 `chat.error`（API_KEY_PROVIDER_NOT_FOUND / MODEL_NOT_CONFIGURED）

---

## 10. Service 设计

### 10.1 Struct

```go
// app/chat/chat.go
type Service struct {
    repo        chatdomain.Repository    // messages + blocks + attachments
    convRepo    convdomain.Repository    // 对话 CRUD
    modelPicker modeldomain.ModelPicker  // 拿 (provider, modelID)
    keyProvider apikeydomain.KeyProvider // 拿 (key, baseURL)
    llmFactory  *llminfra.Factory        // 自有 LLM 流式客户端工厂
    tools       []agentapp.Tool          // System Tools（实现 Tool 接口；SetTools 注入）
    bridge      eventsdomain.Bridge      // 推 SSE 事件
    dataDir     string                   // 附件存储根目录
    log         *zap.Logger
    queues      sync.Map                 // conversationID → *convQueue
}
```

### 10.2 Send 流程

```
HTTP 入口（同步部分）:
  1. convRepo.Get(conversationID)            → 验证对话存在
  2. buildUserBlocks(ctx, in)                → 从 DB 查附件完整元数据构建 user blocks
  3. repo.Save(userMsg with blocks)          → DB（user message）
  4. getOrCreateQueue(conversationID)        → 入队 queuedTask{userMsgID}
  5. 立刻返回 202 { messageId: userMsgID }

worker goroutine（runner.go::processTask）:
  6. modelPicker.PickForChat(ctx)            → (provider, modelID)；失败推 SSE chat.error MODEL_NOT_CONFIGURED
  7. keyProvider.ResolveCredentials(ctx, p)  → (key, baseURL)；失败推 SSE chat.error API_KEY_PROVIDER_NOT_FOUND
  8. llmFactory.Build(Config{...})           → Client；失败推 SSE chat.error LLM_PROVIDER_ERROR
  9. agentRun(ctx, uid, conv, userMsgID, client, baseReq)
       buildHistory → for-step ReAct → writeDB checkpoints → ChatDone

  10. 触发 auto-title goroutine（conv.Title 为空且 !AutoTitled）
```

### 10.3 并发控制与取消

每个 conversationID 拥有一个 `convQueue`（buffered channel cap=5 + 单 worker goroutine），保证同对话消息按序逐条执行；不同对话间并行。worker 在 5 分钟空闲后自行退出，下次 Send 时按需重建。

```go
type convQueue struct {
    ch     chan queuedTask
    mu     sync.Mutex
    cancel context.CancelFunc  // nil when idle
}
```

- **Send**：队列满 → 409 `STREAM_IN_PROGRESS`；否则入队后立即 202 返回
- **Cancel**：`q.cancel()` → ctx cancelled → streamLLM break → agentRun 写 `status=cancelled` → 推 ChatDone

### 10.4 System Prompt 组装

每次调用 Agent 前，`buildSystemPrompt(ctx, conv)` 按以下优先级组装：

```
[基础系统提示词（代码写死）]
+
[conversation.system_prompt（用户自定义，可为空）]
+
[locale 指令（从 reqctx 读）]
```

`conversation.system_prompt` 字段存在 `conversations` 表（由 conversation domain 管理），chat.Service 通过 `convRepo.Get(id)` 读取。

### 10.5 自动命名（Auto-Titling）

第一轮对话完成后（assistant 消息 status=completed），异步起一个 goroutine 调轻量模型生成标题：

```
条件：conversation.title == "" AND conversation.auto_titled == false
  → 调 modelFactory.Build（使用同 provider 的轻量模型，如 haiku / gpt-4o-mini）
  → System: "生成一个 5 字以内的对话标题，只返回标题本身"
  → Input: 前两条消息（user + assistant）
  → 写回 conversations.title + conversations.auto_titled = true
  → 推 conversation.title_updated SSE 事件
```

**非阻塞**：标题生成失败静默忽略，不影响主流程。`conversations` 表需新增 `auto_titled BOOLEAN NOT NULL DEFAULT false` 字段。

---

## 11. 完整调用链（Phase 3 当前形态）

### 11.1 用户发消息

```
POST /api/v1/conversations/cv_xxx/messages  body={content, attachmentIds}
  → middleware 链
  → ChatHandler.Send
      → convRepo.Get(conversationID)              → 验证对话存在
      → buildUserBlocks(ctx, in)                  → 查附件完整元数据 → []Block
      → repo.Save(userMsg with blocks)            → DB
      → getOrCreateQueue(conversationID).ch <- queuedTask{userMsgID}
      → response 202 {messageId: userMsgID}

--- worker goroutine（runner.go::processTask）---
  → modelPicker.PickForChat(ctx)                  → ("deepseek", "deepseek-chat")
  → keyProvider.ResolveCredentials(ctx, "deepseek") → (key, baseURL)
  → llmFactory.Build(Config{...})                 → Client
  → 构造 baseReq（System / Tools 注入）
  → agentRun(ctx, uid, conv, userMsgID, client, baseReq):
      buildHistory(ctx, convID, userMsgID)        → []LLMMessage（末尾追加当前 user）
      for step < maxSteps:
          aBlocks, toolCalls, sr, iT, oT = streamLLM(ctx, client, req, convID, msgID)
              EventToolStart → Publish(ChatToolCallStart)
              EventText      → Publish(ChatToken)
              EventReasoning → Publish(ChatReasoningToken)
              EventFinish    → 累计 token usage
          allBlocks += aBlocks; totalInput += iT; totalOutput += oT
          if cancelled/error → writeDB(fatal=true) → break
          if len(toolCalls)==0 → writeDB(completed, fatal=true) → break
          rBlocks = runTools(ctx, toolCalls, convID, msgID)   // 并行 sync.WaitGroup
          allBlocks += rBlocks
          writeDB(streaming, fatal=false)
          history = extendHistory(history, aBlocks, rBlocks)
      if !finalWritten → writeDB(max_tokens, fatal=true)
  → Publish(ChatDone)
  → if conv.Title=="" && !AutoTitled: go autoTitle(...)
```

### 11.2 前端收事件

```
GET /api/v1/events?conversationId=cv_xxx
  → ChatHandler.EventsSSE
      → Bridge.Subscribe(filter={conversationId: cv_xxx})
      → 每 15s 推 ": keep-alive\n\n" 防代理断连
      → 持续 write SSE:
          event: chat.tool_call_start  data: {...}
          event: chat.token            data: {...}
          event: chat.tool_call        data: {...}
          event: chat.tool_result      data: {...}
          event: chat.done             data: {...}
```

---

## 12. Phase 4-5 扩展点

chat domain 在 Phase 4-5 主要通过 **追加 system tools** + **升级 system prompt** 来扩展，Service 本身代码改动很小：

### Phase 4（workflow 完成后）
- 追加 `create_workflow` / `edit_workflow` / `run_workflow` system tool
- Agent 获得"对话中创建/运行工作流"能力
- chat.Service 代码零改动，main.go 多注入 3 个 tool

### Phase 5（智能化完成后）
- 追加 `search_knowledge`（RAG）+ `mcp_call`（MCP 服务器）system tool
- System Prompt 升级为意图引导版（"可创建工具/工作流/搜知识库，自主决策"）
- 长对话 context compaction（runner.go::agentRun 已预留 TODO 钩子点）：超长时压缩历史，保留关键消息——这是 Claude Code 调研 [`claude-code-research-documents/03-context.md`](../claude-code-research-documents/03-context.md) 的吸收

---

## 13. 错误码

| Code | HTTP | Sentinel | 场景 |
|---|---|---|---|
| `STREAM_NOT_FOUND` | 404 | `chat.ErrStreamNotFound` | 取消不存在的流 |
| `STREAM_IN_PROGRESS` | 409 | `chat.ErrStreamInProgress` | 同一对话已有 Agent 在运行 |
| `LLM_PROVIDER_ERROR` | 502 | `chat.ErrProviderUnavailable` | 上游 LLM 故障（非 401）|
| `ATTACHMENT_TOO_LARGE` | 413 | `chat.ErrAttachmentTooLarge` | 附件超过 50MB |
| `ATTACHMENT_TYPE_UNSUPPORTED` | 415 | `chat.ErrAttachmentTypeUnsupported` | 无法处理的文件格式 |
| `ATTACHMENT_PARSE_FAILED` | 422 | `chat.ErrAttachmentParseFailed` | 文件损坏或解析失败 |
| `VISION_NOT_SUPPORTED` | 422 | `chat.ErrVisionNotSupported` | 当前 provider 不支持图片 |

**401 路径**：LLM 返回 401 → `apikey.MarkInvalid` → 推 `chat.error` 事件（code: `API_KEY_INVALID`）→ Service 返回 `apikey.ErrInvalid` → errmap 翻译 → 前端 SSE 收到。

---

## 14. 为什么这样设计（关键决策总结）

| 决策 | 选择 | 理由 |
|---|---|---|
| 用 ReAct Agent 还是固定 Graph | **ReAct Agent** | 任务序列是运行时 LLM 决定的，不能提前写死；Phase 2-5 的工具列表动态增长 |
| tools 全部注入 vs Tool RAG | **System Tools 注入 + Tool RAG** | System Tools 数量固定（~8 个）可全注入；用户工具无上限，靠 search_tools 动态检索 |
| 202 + SSE vs 直接 stream response | **202 + 独立 SSE** | Agent 跑多步需要持久连接；POST 语义是"接受请求"不是"等待结果"；events Bridge 已就绪 |
| messages 存哪 | **chat domain 自己管** | 消息历史是 chat 专有数据，不应跨 domain 共享；conversation domain 只管线程元数据 |
| LLM 客户端用现成框架 vs 自实现 | **自实现 `infra/llm`** | 流式 SSE 解析 / tool call 累积 / 错误分类需要完全可见可控；framework 抽象会丢信息（实测 framework callback 对流式 ChatModel 不触发，导致 DB content 空）|
| 不同 provider 协议适配 | **各自独立 client** | OpenAI 兼容 / Anthropic 原生 / Ollama 等都有协议差异（消息格式、tool result 包装、stream 边界），强行统一会失真 |
| System Prompt 的 locale | **buildSystemPrompt 动态注入** | 每次调用前动态拼接，locale 从 reqctx 读，Agent 不需要知道 locale 逻辑 |
| Message status | **message 级别字段** | 流式过程中消息状态需持久化；失败/取消场景前端需要准确知道每条消息的最终态 |
| SSE 可靠性 | **keep-alive ping + 内存 Bridge fan-out** | 网络抖动断连不丢事件；桌面应用场景常见 |
| Auto-titling | **异步 goroutine，失败静默** | 标题生成不是核心流程；用轻量模型节省费用；失败不影响用户体验 |
| 中间步 DB checkpoint | **streaming 状态 + buildHistory 跳过** | 多步 ReAct 中间态需持久化（崩溃恢复 / SQL Tab 调试可见），但不能进 LLM 历史避免循环 |
| 终态写 detached context | **`writeDB(fatal=true)` 用 `context.Background()`** | 用户取消时 ctx 已 cancelled，但终态消息必须落库——否则下次打开对话看不到这次回复 |

---

## 15. 实现清单 ✅

### infra/llm 层（自有 LLM 流式客户端）
- [x] `infra/llm/llm.go` — StreamEvent / LLMMessage / ToolDef / Client 接口 / Generate helper
- [x] `infra/llm/openai.go` — OpenAI-compat SSE 客户端（iter.Seq），覆盖 OpenAI/DeepSeek/Qwen/Moonshot/Ollama 等
- [x] `infra/llm/anthropic.go` — Anthropic 原生 `/v1/messages` 客户端（content_block_start/delta/stop）
- [x] `infra/llm/factory.go` — Factory.Build(Config) provider dispatch + resolveBaseURL

### app/agent 层
- [x] `app/agent/tool.go` — Tool 4 方法接口 + injectSummaryField + StripSummary + ToLLMDef/ToLLMDefs + 6 个 context helpers（WithConversationID/MessageID/ToolCallID + 对应 Get）
- [x] `app/agent/system.go` — 6 个 system tool（datetime/read_file/write_file/list_dir/run_shell/run_python）
- [x] `app/agent/web.go` — web_search（DDG lite POST 表单）+ fetch_url（Jina Reader）
- [x] `app/agent/forge.go` — 5 个 forge tool（search/get/create/edit/run）

### domain/chat 层
- [x] `domain/chat/chat.go` — Message（精简纯元数据）+ Block 实体 + 5 种 BlockType + data 结构体 + Attachment + sentinels + Repository
- [x] `domain/events/types.go` — ChatToolCallStart + ChatReasoningToken；ChatDone 改为 inputTokens/outputTokens int；ToolCodeStreaming/ToolCreated/ToolPendingCreated 加 MessageID+ToolCallID

### infra/db 层
- [x] `infra/db/schema_extras.go` — 按 table 分组的 extraGroup 结构；message_blocks 索引；tools partial UNIQUE（FTS5 当前未使用）
- [x] `infra/db/db.go` — modernc.org/sqlite 驱动；DSN 走 `_pragma=...` 语法

### infra/store/chat 层
- [x] `infra/store/chat/chat.go` — Save（ON CONFLICT upsert 保护 created_at，事务写 blocks）；ListByConversation（批量取 blocks 避 N+1）；GetAttachment；SaveAttachment

### infra/chat 层
- [x] `infra/chat/extractor.go` — Extract(storagePath, mimeType)：text/pdf/docx/xlsx/pptx/html 提取；IsImage 分派 Vision 路径

### app/chat 层（6 文件）
- [x] `app/chat/chat.go` — Service struct + Send / Cancel / ListMessages / UploadAttachment + queueCapacity + convQueue / queuedTask 类型
- [x] `app/chat/runner.go` — getOrCreateQueue / runQueue / processTask / agentRun（ReAct loop，含 context compaction 钩子点）/ writeDB（fatal 模式分支）/ stampBlocks / autoTitle
- [x] `app/chat/stream.go` — streamLLM（iter.Seq）+ assembleBlocks + extractToolCalls + parseToolArgs
- [x] `app/chat/tools.go` — runTools（sync.WaitGroup 并行）+ runOneTool（注入 msgID/toolCallID）+ executeTool
- [x] `app/chat/history.go` — buildHistory(currentUserMsgID) + extendHistory + blocksToLLM + blocksToAssistantLLM + buildUserLLMMessage + attachmentToPart
- [x] `app/chat/util.go` — newMsgID / newBlockID / newAttachmentID / readAndEncode / truncate

### transport 层
- [x] `handlers/chat.go` — 5 端点：POST attachments / POST messages / DELETE stream / GET messages / GET events SSE（keep-alive ping）

### 配套
- [x] `errmap.go` — chat sentinel 映射全部覆盖
- [x] `router/deps.go` — ChatService / EventsBridge 字段
- [x] `main.go` — chatRepo 共享变量；llmFactory；WebTools / SystemTools / ForgeTools 装配；Migrate messages + message_blocks + chat_attachments
