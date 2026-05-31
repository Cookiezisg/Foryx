# Chat 基础设施重构设计文档

> **性质**：临时重构文档。重构完成后，将更新 `service-design-documents/chat.md`、`service-contract-documents/database-design.md`、`service-contract-documents/events-design.md`，本文档归档。

---

## 一、为什么要重构

当前 chat 管线有三处设计债，互相关联：

### 1.1 DB schema 拍扁了 LLM 的内容结构

LLM 的输出天然是"若干个有类型的 block"的序列：

```
reasoning block   → "我需要先查一下天气..."
tool_call block   → get_weather(city="Beijing")
tool_result block → "晴，25°C"
text block        → "北京今天晴，25度，适合出门。"
```

但现在 `messages` 表把它们拍扁成多个列：

```sql
content TEXT,               -- 文字内容
reasoning_content TEXT,     -- 思考内容
tool_calls TEXT,            -- JSON blob
tool_call_id TEXT,          -- tool 角色消息专用
attachment_ids TEXT,        -- JSON 数组
token_usage TEXT,           -- JSON blob
```

**问题**：同维度的东西变成了不同列；新增内容类型就要改表结构；`tool` 角色行本质上是 tool_result block，和 tool_call 的关系只靠 `tool_call_id` 软链接。

### 1.2 Eino 作为黑盒框架引入

整个 app 层渗透了 Eino 的类型：`schema.Message`、`tool.BaseTool`、`schema.ToolInfo`。实际上只用了 Eino 的三个功能：HTTP client、SSE 解析、schema 类型定义。

Eino 的 `react.Agent` 因 callback bug 已被废弃，自己重写了 ReAct loop。`BuiltModel.Checker` 字段（`safeStreamChecker`）仍然存在，是死代码。

### 1.3 Pipeline 先攒完整个流再处理

`collectStream` 把整个 LLM 流读完，组装成完整的 `schema.Message`，然后才判断有没有 tool call。这和"收到什么推什么"的思路违背。

另有一个 bug：mid-stream 取消/错误时走 `saveFinalMessage`（固定写 `status=completed`），状态不对。

---

## 二、重构目标

1. **DB**：`messages` 表精简为纯元数据，新增 `message_blocks` 表，每种内容独立一行
2. **infra/llm**：自主实现 LLM 流式 HTTP 客户端，完全移除 Eino 依赖；用 `iter.Seq` 代替 channel
3. **Tool 接口**：4 个方法，框架统一注入 `summary` 字段，tool 实现者无感知
4. **Pipeline**：事件驱动，收到什么处理什么；LLM 提供 tool call summary，框架提取并推送 SSE

---

## 三、DB Schema 新设计

### 3.1 `messages` 表（精简）

**移除的列**：`content`、`reasoning_content`、`tool_calls`、`tool_call_id`、`attachment_ids`、`token_usage`

**新增的列**：`input_tokens INT`、`output_tokens INT`

**`role` 值变化**：移除 `tool`（tool result 变成 block，不再单独成行）

```go
// domain/chat/chat.go
type Message struct {
    ID             string         `gorm:"primaryKey;type:text" json:"id"`
    ConversationID string         `gorm:"not null;index;type:text" json:"conversationId"`
    UserID         string         `gorm:"not null;type:text" json:"-"`
    Role           string         `gorm:"not null;type:text" json:"role"`  // "user" | "assistant"
    Status         string         `gorm:"not null;type:text" json:"status"`
    StopReason     string         `gorm:"type:text;default:''" json:"stopReason,omitempty"`
    InputTokens    int            `gorm:"default:0" json:"inputTokens,omitempty"`
    OutputTokens   int            `gorm:"default:0" json:"outputTokens,omitempty"`
    CreatedAt      time.Time      `json:"createdAt"`
    DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`

    // Blocks 不是 DB 列，由 store 层查询后填充
    Blocks []Block `gorm:"-" json:"blocks"`
}
```

### 3.2 `message_blocks` 表（新建）

```go
// domain/chat/chat.go
type Block struct {
    ID        string    `gorm:"primaryKey;type:text" json:"id"`
    MessageID string    `gorm:"not null;index;type:text" json:"-"`
    Seq       int       `gorm:"not null" json:"seq"`
    Type      string    `gorm:"not null;type:text" json:"type"`
    Data      string    `gorm:"not null;type:text" json:"data"` // JSON
    CreatedAt time.Time `json:"createdAt"`
}

func (Block) TableName() string { return "message_blocks" }
```

### 3.3 Block 类型与 Data JSON 结构

| Type | Data 结构 | 说明 |
|------|-----------|------|
| `text` | `{"text":"..."}` | 普通文字回复 |
| `reasoning` | `{"text":"..."}` | 思考内容（DeepSeek-R1 等） |
| `tool_call` | `{"id":"call_1","name":"get_weather","summary":"Checking Beijing weather","arguments":{"city":"Beijing"}}` | LLM 发起的工具调用（含 LLM 写的 summary） |
| `tool_result` | `{"toolCallId":"call_1","ok":true,"result":"晴，25°C"}` | 工具执行结果 |
| `attachment_ref` | `{"attachmentId":"att_abc","fileName":"report.pdf","mimeType":"application/pdf"}` | 用户上传的附件引用 |

`tool_call` 的 `summary` 字段由 LLM 在调用 tool 时填写，框架从 arguments 中提取后存入 block 并推送 SSE，执行时剥除，不传给 tool 的 Execute。

### 3.4 存储样例

**用户："北京和上海今天天气怎么样？"（含附件）**

```
messages: id=msg_001, role=user, status=completed

message_blocks:
  seq=0, type=text,           data={"text":"北京和上海今天天气怎么样？"}
  seq=1, type=attachment_ref, data={"attachmentId":"att_abc","fileName":"weather_context.txt","mimeType":"text/plain"}
```

```
messages: id=msg_002, role=assistant, status=completed, stop_reason=end_turn, input_tokens=150, output_tokens=80

message_blocks:
  seq=0, type=reasoning,   data={"text":"我需要分别查两个城市的天气。"}
  seq=1, type=tool_call,   data={"id":"call_1","name":"get_weather","summary":"Checking Beijing weather","arguments":{"city":"Beijing"}}
  seq=2, type=tool_call,   data={"id":"call_2","name":"get_weather","summary":"Checking Shanghai weather","arguments":{"city":"Shanghai"}}
  seq=3, type=tool_result, data={"toolCallId":"call_1","ok":true,"result":"晴，25°C"}
  seq=4, type=tool_result, data={"toolCallId":"call_2","ok":true,"result":"阴，18°C"}
  seq=5, type=text,        data={"text":"北京今天晴天25度，上海阴天18度。"}
```

### 3.5 重建 LLM 历史时的格式转换

从 DB 取出 Message+Blocks 后，转成 OpenAI 协议格式：

```
user message (blocks: text + attachment_ref)
→ {role:"user", content:[{type:"text",text:"..."},{type:"image_url",...}]}

assistant message (blocks: reasoning + tool_call×N + tool_result×N + text)
→ 拆成两段：
  1. {role:"assistant", content:"<reasoning>", tool_calls:[{id,name,arguments},...]}  ← arguments 里不含 summary
  2. 每个 tool_result → {role:"tool", tool_call_id:"...", content:"..."}
```

注意：`summary` 在 DB 里随 tool_call block 存储，但重建 LLM 历史时剥除，不回传给 LLM。

---

## 四、`internal/infra/llm/` — 自主 LLM 客户端

完全取代 `internal/infra/eino/`。

### 4.1 文件结构

```
internal/infra/llm/
├── types.go      — 核心类型（StreamEvent、LLMMessage、ToolDef、Client 接口）
├── openai.go     — OpenAI 兼容客户端（覆盖 openai/deepseek/qwen/ollama 等所有 compat provider）
├── anthropic.go  — Anthropic 原生客户端（/v1/messages，content block 格式）
└── factory.go    — 按 provider dispatch，返回对应 Client
```

Anthropic 单独实现原生客户端的原因：
- 原生 SSE 格式（`content_block_start/delta/stop`）明确标记每个 block 的结束，和我们的 block 模型天然对应
- `content_block_stop` 信号可用于提前确认 tool call arguments 完整，为并行执行创造条件
- thinking block 是一等公民类型，不是旁路字段

### 4.2 `types.go` — 核心类型

```go
package llm

import (
    "context"
    "encoding/json"
    "iter"
)

// ── 流式事件 ──────────────────────────────────────────────────────────────────

type StreamEventType string

const (
    EventText      StreamEventType = "text"        // 普通文字 delta
    EventReasoning StreamEventType = "reasoning"   // 思考内容 delta
    EventToolStart StreamEventType = "tool_start"  // tool 名字确定，立刻可推 SSE
    EventToolDelta StreamEventType = "tool_delta"  // arguments 片段
    EventFinish    StreamEventType = "finish"      // 流结束，携带 usage + finish_reason
    EventError     StreamEventType = "error"       // 流中出错
)

// StreamEvent 是 LLM 流式响应的一个事件，各字段按 Type 按需填充。
type StreamEvent struct {
    Type StreamEventType

    // EventText / EventReasoning
    Delta string

    // EventToolStart
    ToolIndex int
    ToolID    string
    ToolName  string

    // EventToolDelta（ToolIndex 复用上面字段）
    ArgsDelta string

    // EventFinish
    FinishReason string
    InputTokens  int
    OutputTokens int

    // EventError
    Err error
}

// ── LLM 请求 / 消息类型 ───────────────────────────────────────────────────────

type Role string

const (
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleTool      Role = "tool"
)

// LLMMessage 是发给 LLM 的一条消息（已转换为协议格式）。
type LLMMessage struct {
    Role             Role
    Content          string        // 纯文字
    Parts            []ContentPart // 多模态（user 消息含图片时）
    ToolCalls        []LLMToolCall // assistant 消息发起的 tool call
    ToolCallID       string        // tool 角色消息：对应的 call id
    ReasoningContent string        // thinking-mode API 回传用
}

type ContentPart struct {
    Type     string // "text" | "image_url"
    Text     string
    ImageURL string // base64 data URL 或 http URL
}

type LLMToolCall struct {
    ID        string
    Name      string
    Arguments string // JSON 字符串（不含 summary）
}

// ToolDef 是发给 LLM 的工具描述（JSON Schema 格式）。
type ToolDef struct {
    Name        string
    Description string
    Parameters  json.RawMessage // {"type":"object","properties":{...},"required":[...]}
}

// Request 是一次完整的 LLM 调用请求。
type Request struct {
    ModelID  string
    Key      string
    BaseURL  string
    System   string
    Messages []LLMMessage
    Tools    []ToolDef
}

// ── Client 接口 ───────────────────────────────────────────────────────────────

// Client 是 LLM 流式客户端接口。
// Stream 返回 iter.Seq[StreamEvent]，调用方用 for range 消费。
// 调用方 break 或 ctx 取消时，迭代自动终止，无需额外清理。
type Client interface {
    Stream(ctx context.Context, req Request) iter.Seq[StreamEvent]
}
```

**为什么用 `iter.Seq` 而不是 channel：**
- 拉式（pull-based），consumer 读一个才产生一个，天然背压
- 调用方直接 `break` 即可退出，无需 drain channel 或担心 goroutine 泄漏
- 无需管理 goroutine 生命周期和 channel 关闭
- 错误作为 `EventError` 事件流出，和其他事件处理方式一致

**为什么去掉 `Generate()`：**

非流式调用（如 `search_tools` 内部排序）直接消费 `Stream()` 的 text delta 拼接即可，不需要独立接口。减少接口面，实现更简单。

### 4.3 `openai.go` — OpenAI 兼容 SSE 解析

覆盖：OpenAI、DeepSeek、Qwen、Moonshot、Ollama、Custom(openai-compat)，以及 Anthropic 的 OpenAI compat 端点（Anthropic 优先走原生客户端，compat 作为兜底）。

**iter.Seq 实现模式**：

```go
func (c *openAIClient) Stream(ctx context.Context, req Request) iter.Seq[StreamEvent] {
    return func(yield func(StreamEvent) bool) {
        body, err := buildOpenAIRequest(req)
        if err != nil {
            yield(StreamEvent{Type: EventError, Err: err})
            return
        }

        httpReq, _ := http.NewRequestWithContext(ctx, "POST",
            req.BaseURL+"/chat/completions", bytes.NewReader(body))
        httpReq.Header.Set("Authorization", "Bearer "+req.Key)
        httpReq.Header.Set("Content-Type", "application/json")

        resp, err := c.http.Do(httpReq)
        if err != nil {
            yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm: %w", err)})
            return
        }
        defer resp.Body.Close()

        if resp.StatusCode != http.StatusOK {
            body, _ := io.ReadAll(resp.Body)
            yield(StreamEvent{Type: EventError, Err: classifyHTTPError(resp.StatusCode, body)})
            return
        }

        toolNameSent := map[int]bool{}
        scanner := bufio.NewScanner(resp.Body)
        for scanner.Scan() {
            if ctx.Err() != nil {
                return
            }
            // 解析 SSE line → StreamEvent → yield
            // 首次出现 tool name → EventToolStart
            // arguments delta → EventToolDelta
            // 文字 → EventText
            // reasoning_content → EventReasoning
            // finish_reason → EventFinish
            if !yield(event) {
                return // consumer break 了
            }
        }
    }
}
```

关键点：`yield` 返回 `false` 表示 consumer 不再需要数据（break），立刻 return，HTTP 响应 body 由 defer 关闭。

### 4.4 `anthropic.go` — Anthropic 原生 SSE 解析

Anthropic 原生格式的 SSE 事件流：

```
event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"我需要..."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call_1","name":"get_weather","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}   ← tool call arguments 此处确认完整
```

映射到我们的 StreamEvent：

| Anthropic 事件 | StreamEvent |
|---|---|
| `content_block_start` (thinking) | — |
| `content_block_delta` (thinking_delta) | `EventReasoning` |
| `content_block_start` (tool_use) | `EventToolStart`（name 已知） |
| `content_block_delta` (input_json_delta) | `EventToolDelta` |
| `content_block_stop` (tool_use index) | — （pipeline 可利用此信号提前确认 args 完整） |
| `content_block_start` (text) | — |
| `content_block_delta` (text_delta) | `EventText` |

Anthropic 原生请求格式与 OpenAI 的差异：
- `system` 是独立的顶层字段，不是 messages 里的一条
- tool result 在 user message 的 content 数组里（`{type:"tool_result", tool_use_id:"...", content:"..."}`），不是独立的 role=tool 消息
- thinking block 需要在 API 参数里开启 `thinking: {type:"enabled", budget_tokens: N}`

### 4.5 `factory.go` — Provider dispatch

```go
type Factory struct {
    openai    *openAIClient
    anthropic *anthropicClient
}

func NewFactory() *Factory {
    return &Factory{
        openai:    newOpenAIClient(),
        anthropic: newAnthropicClient(),
    }
}

type Config struct {
    Provider  string
    APIFormat string // custom provider: "openai-compatible" | "anthropic-compatible"
    ModelID   string
    Key       string
    BaseURL   string
}

func (f *Factory) Build(cfg Config) (Client, string, error) {
    baseURL, err := resolveBaseURL(cfg)
    if err != nil {
        return nil, "", err
    }
    switch cfg.Provider {
    case "anthropic":
        return f.anthropic, baseURL, nil
    case "custom":
        if cfg.APIFormat == "anthropic-compatible" {
            return f.anthropic, baseURL, nil
        }
        return f.openai, baseURL, nil
    default:
        return f.openai, baseURL, nil
    }
}
```

默认 BaseURL（按 provider）：

| Provider | Default BaseURL |
|----------|----------------|
| openai | `https://api.openai.com/v1` |
| anthropic | `https://api.anthropic.com` （原生端点，不加 /v1，anthropic client 自己拼路径） |
| ollama | `http://localhost:11434/v1` |
| deepseek | `https://api.deepseek.com/v1` |
| custom | 必须手动提供 |

---

## 五、`internal/domain/agent/tool.go` — 自主 Tool 接口

取代 Eino 的 `tool.BaseTool` + `schema.ToolInfo`。

```go
// domain/agent/tool.go
package agent

import (
    "context"
    "encoding/json"

    llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// Tool 是每个系统 tool 必须实现的接口。
// summary 字段由框架统一注入到 Parameters schema，tool 实现者无需关心。
type Tool interface {
    Name()        string
    Description() string
    Parameters()  json.RawMessage // 业务参数 schema，不含 summary 字段
    Execute(ctx context.Context, argsJSON string) (string, error)
}

// ToLLMDef 把 Tool 转成发给 LLM 的 ToolDef，自动注入 summary 字段。
// summary 字段要求 LLM 在调用时填写一句话描述，框架提取后推 SSE，执行前剥除。
func ToLLMDef(t Tool) llminfra.ToolDef {
    return llminfra.ToolDef{
        Name:        t.Name(),
        Description: t.Description(),
        Parameters:  injectSummaryField(t.Parameters()),
    }
}

// ToLLMDefs 批量转换。
func ToLLMDefs(tools []Tool) []llminfra.ToolDef {
    defs := make([]llminfra.ToolDef, len(tools))
    for i, t := range tools {
        defs[i] = ToLLMDef(t)
    }
    return defs
}

// injectSummaryField 在 parameters schema 的 properties 里注入 summary 字段，
// 并添加到 required 列表首位（引导 LLM 优先生成，便于早期 SSE 推送）。
func injectSummaryField(params json.RawMessage) json.RawMessage {
    // 解析 → 注入 summary → 重新序列化
    // 如果 schema 已包含 summary 字段则不重复注入
}

// ── 执行前剥除 summary ────────────────────────────────────────────────────────

// StripSummary 从 argsJSON 中取出 summary 值并返回剥除后的 JSON。
// pipeline 在执行 tool 前调用，summary 值用于 SSE 推送。
func StripSummary(argsJSON string) (summary string, strippedJSON string) {
    var m map[string]any
    if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
        return "", argsJSON
    }
    if s, ok := m["summary"].(string); ok {
        summary = s
        delete(m, "summary")
    }
    b, _ := json.Marshal(m)
    return summary, string(b)
}

// ── Context helpers ───────────────────────────────────────────────────────────

type contextKey int

const convIDKey contextKey = iota

func WithConversationID(ctx context.Context, id string) context.Context {
    return context.WithValue(ctx, convIDKey, id)
}

func GetConversationID(ctx context.Context) (string, bool) {
    v, ok := ctx.Value(convIDKey).(string)
    return v, ok && v != ""
}
```

---

## 六、更新 `internal/domain/chat/chat.go`

**新增实体**：`Block`

**`Message` 变化**：移除内容列，新增 `InputTokens`、`OutputTokens`、`Blocks []Block`（`gorm:"-"`）

**Block data 结构体**（供 app 层序列化/反序列化）：

```go
const (
    BlockTypeText          = "text"
    BlockTypeReasoning     = "reasoning"
    BlockTypeToolCall      = "tool_call"
    BlockTypeToolResult    = "tool_result"
    BlockTypeAttachmentRef = "attachment_ref"
)

type TextData          struct { Text string `json:"text"` }
type ReasoningData     struct { Text string `json:"text"` }
type ToolCallData      struct {
    ID        string         `json:"id"`
    Name      string         `json:"name"`
    Summary   string         `json:"summary"`           // LLM 填写的调用描述
    Arguments map[string]any `json:"arguments"`         // 不含 summary
}
type ToolResultData    struct {
    ToolCallID string `json:"toolCallId"`
    OK         bool   `json:"ok"`
    Result     string `json:"result"`
}
type AttachmentRefData struct {
    AttachmentID string `json:"attachmentId"`
    FileName     string `json:"fileName"`
    MimeType     string `json:"mimeType"`
}
```

---

## 七、更新 `internal/infra/store/chat/chat.go`

### 7.1 Save — 同时写 message + blocks（事务）

```go
func (s *Store) Save(ctx context.Context, m *chatdomain.Message) error {
    return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Save(m).Error; err != nil {
            return fmt.Errorf("chatstore.Save message: %w", err)
        }
        if len(m.Blocks) > 0 {
            if err := tx.Where("message_id = ?", m.ID).
                Delete(&chatdomain.Block{}).Error; err != nil {
                return fmt.Errorf("chatstore.Save delete old blocks: %w", err)
            }
            if err := tx.Create(&m.Blocks).Error; err != nil {
                return fmt.Errorf("chatstore.Save blocks: %w", err)
            }
        }
        return nil
    })
}
```

### 7.2 ListByConversation — 附带 blocks（N+1 避免）

```go
// 一次额外查询取所有相关 blocks，按 message_id 分组，避免 N+1
ids := make([]string, len(rows))
for i, m := range rows { ids[i] = m.ID }

var blocks []*chatdomain.Block
s.db.Where("message_id IN ?", ids).Order("message_id, seq ASC").Find(&blocks)

blockMap := make(map[string][]chatdomain.Block)
for _, b := range blocks {
    blockMap[b.MessageID] = append(blockMap[b.MessageID], *b)
}
for _, m := range rows {
    m.Blocks = blockMap[m.ID]
}
```

---

## 八、更新 `internal/infra/db/migrate.go`

在 `db.Migrate` 调用里新增 `&chatdomain.Block{}`，放在 `&chatdomain.Message{}` 之后。

`schema_extras.go` 补充：

```sql
CREATE INDEX IF NOT EXISTS idx_message_blocks_msg_seq ON message_blocks(message_id, seq);
```

---

## 九、新的 `app/chat/pipeline.go`

### 9.1 整体结构

```
processTask()
  ├─ 1. PickForChat + ResolveCredentials
  ├─ 2. llmFactory.Build(cfg) → (client, baseURL)
  ├─ 3. buildLLMHistory() → []LLMMessage
  ├─ 4. runReactLoop() — 最多 20 步
  └─ 5. Publish(ChatDone) + autoTitle()

runReactLoop()
  └─ for step:
       runStep() → done bool
         ├─ client.Stream(req)  → iter.Seq[StreamEvent]
         ├─ for event := range stream:
         │    EventText      → textBuf + Publish(ChatToken)
         │    EventReasoning → reasoningBuf + Publish(ChatReasoningToken)
         │    EventToolStart → accums[idx] + Publish(ChatToolCallStart)
         │    EventToolDelta → accums[idx].args += delta
         │    EventFinish    → record usage
         │    EventError     → set error, break
         │
         ├─ assembleAndSaveAssistantMsg()
         ├─ if no tool calls → done=true, return
         └─ executeToolCalls() — 并行执行（见 9.3）
              for each toolCall（goroutine）:
                summary, strippedArgs = StripSummary(args)
                Publish(ChatToolCall{summary})
                result = tool.Execute(strippedArgs)
                Publish(ChatToolResult)
              saveToolResultBlocks()
              return done=false
```

### 9.2 iter.Seq 消费

```go
for event := range client.Stream(ctx, req) {
    switch event.Type {
    case llm.EventText:
        textBuf.WriteString(event.Delta)
        s.bridge.Publish(ctx, convID, events.ChatToken{Delta: event.Delta, MessageID: msgID})

    case llm.EventReasoning:
        reasoningBuf.WriteString(event.Delta)
        s.bridge.Publish(ctx, convID, events.ChatReasoningToken{Delta: event.Delta, MessageID: msgID})

    case llm.EventToolStart:
        accums[event.ToolIndex] = &toolAccum{id: event.ToolID, name: event.ToolName}
        s.bridge.Publish(ctx, convID, events.ChatToolCallStart{
            MessageID: msgID, ToolCallID: event.ToolID, ToolName: event.ToolName,
        })

    case llm.EventToolDelta:
        if a := accums[event.ToolIndex]; a != nil {
            a.args.WriteString(event.ArgsDelta)
        }

    case llm.EventFinish:
        usage = event
        if event.FinishReason == "length" {
            stopReason = chatdomain.StopReasonMaxTokens
        }

    case llm.EventError:
        streamErr = event.Err
        // for range 在 yield 返回 false 后停止，这里直接 break
    }
}
```

### 9.3 并行 tool call 执行

LLM 单次响应可能包含多个 tool call（parallel tool calls）。串行执行浪费等待时间，改为并发：

```go
func (s *Service) executeToolCalls(
    ctx context.Context,
    accums map[int]*toolAccum,
    convID, msgID string,
) []chatdomain.Block {
    type result struct {
        index int
        block chatdomain.Block
    }
    results := make(chan result, len(accums))
    var wg sync.WaitGroup

    for idx, a := range accums {
        wg.Add(1)
        go func(idx int, a *toolAccum) {
            defer wg.Done()

            summary, strippedArgs := agent.StripSummary(a.args.String())

            s.bridge.Publish(ctx, convID, events.ChatToolCall{
                MessageID:  msgID,
                ToolCallID: a.id,
                ToolName:   a.name,
                Summary:    summary,
            })

            output, ok := s.executeTool(ctx, a.name, strippedArgs)

            s.bridge.Publish(ctx, convID, events.ChatToolResult{
                ToolCallID: a.id, Result: output, OK: ok,
            })

            results <- result{index: idx, block: makeToolResultBlock(a.id, output, ok)}
        }(idx, a)
    }

    wg.Wait()
    close(results)

    // 按原始 index 排序，保证 block seq 稳定
    blocks := make([]chatdomain.Block, len(accums))
    for r := range results {
        blocks[r.index] = r.block
    }
    return blocks
}
```

### 9.4 取消/错误的正确状态写入

```go
// mid-stream 取消或错误时，写正确的 status，而不是 completed
switch {
case ctx.Err() != nil:
    stopReason = chatdomain.StopReasonCancelled
    saveAssistantMsg(status=StatusCancelled)
case streamErr != nil:
    stopReason = chatdomain.StopReasonError
    saveAssistantMsg(status=StatusError)
default:
    saveAssistantMsg(status=StatusCompleted)
}
```

### 9.5 buildLLMHistory — 从 DB 重建 LLM 消息列表

```go
func buildAssistantLLMMessages(m *chatdomain.Message) []llm.LLMMessage {
    var assistantMsg llm.LLMMessage
    assistantMsg.Role = llm.RoleAssistant
    var toolResults []llm.LLMMessage

    for _, b := range m.Blocks {
        switch b.Type {
        case chatdomain.BlockTypeReasoning:
            var d chatdomain.ReasoningData
            json.Unmarshal([]byte(b.Data), &d)
            assistantMsg.ReasoningContent = d.Text

        case chatdomain.BlockTypeText:
            var d chatdomain.TextData
            json.Unmarshal([]byte(b.Data), &d)
            assistantMsg.Content = d.Text

        case chatdomain.BlockTypeToolCall:
            var d chatdomain.ToolCallData
            json.Unmarshal([]byte(b.Data), &d)
            // summary 不回传给 LLM
            argsJSON, _ := json.Marshal(d.Arguments)
            assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, llm.LLMToolCall{
                ID: d.ID, Name: d.Name, Arguments: string(argsJSON),
            })

        case chatdomain.BlockTypeToolResult:
            var d chatdomain.ToolResultData
            json.Unmarshal([]byte(b.Data), &d)
            toolResults = append(toolResults, llm.LLMMessage{
                Role: llm.RoleTool, Content: d.Result, ToolCallID: d.ToolCallID,
            })
        }
    }

    return append([]llm.LLMMessage{assistantMsg}, toolResults...)
}
```

---

## 十、更新 `app/agent/system.go`、`web.go`、`forge.go`

### 10.1 统一改法（4 个方法，无 Summary）

```go
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string { return "read_file" }
func (t *ReadFileTool) Description() string {
    return "Read the content of a local file and return it as text."
}
func (t *ReadFileTool) Parameters() json.RawMessage {
    return json.RawMessage(`{
        "type": "object",
        "properties": {
            "path": {"type": "string", "description": "Absolute or relative path to the file"}
        },
        "required": ["path"]
    }`)
}
func (t *ReadFileTool) Execute(ctx context.Context, argsJSON string) (string, error) {
    // 实现，argsJSON 里已无 summary（框架在上游剥除）
}
```

移除所有 `github.com/cloudwego/eino` import。

### 10.2 `forge.go` — 内部 LLM 调用切到 infra/llm

`streamCode` 从 `built.Model.Stream(schema.Message)` 切到 `client.Stream(llm.Request)`：

```go
// 消费 iter.Seq，拼接文字 token，推 ToolCodeStreaming SSE
for event := range client.Stream(ctx, req) {
    if event.Type == llm.EventText {
        buf.WriteString(event.Delta)
        bridge.Publish(ctx, convID, events.ToolCodeStreaming{Delta: event.Delta})
    }
}
```

非流式排序调用（`search_tools`）同样通过 `Stream()` 消费后拼接，不再需要 `Generate()` 方法。

### 10.3 `web.go` — 移除 duckduckgo eino 包

`web_search` 直接实现，不再依赖 `github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2`：

```go
func (t *WebSearchTool) Execute(ctx context.Context, argsJSON string) (string, error) {
    // 调 DuckDuckGo Lite HTML endpoint，解析结果，返回 JSON
}
```

---

## 十一、更新 SSE 事件（`domain/events/types.go`）

### 11.1 新增：`chat.tool_call_start`

```go
type ChatToolCallStart struct {
    ConversationID string `json:"conversationId"`
    MessageID      string `json:"messageId"`
    ToolCallID     string `json:"toolCallId"`
    ToolName       string `json:"toolName"`
}
func (e ChatToolCallStart) EventName() string { return "chat.tool_call_start" }
```

### 11.2 更新：`chat.tool_call`（新增 Summary 字段）

```go
type ChatToolCall struct {
    ConversationID string `json:"conversationId"`
    MessageID      string `json:"messageId"`
    ToolCallID     string `json:"toolCallId"`
    ToolName       string `json:"toolName"`
    ToolInput      string `json:"toolInput"`   // stripped argsJSON（无 summary）
    Summary        string `json:"summary"`     // LLM 写的一句话描述
}
func (e ChatToolCall) EventName() string { return "chat.tool_call" }
```

### 11.3 完整事件时序

```
chat.reasoning_token  ← EventReasoning，边流边推
chat.token            ← EventText，边流边推
chat.tool_call_start  ← EventToolStart，tool name 一出现就推（新增）
[... arguments delta 累积中，不推 ...]
chat.tool_call        ← arguments 完整，StripSummary 后推（含 summary）
chat.tool_result      ← 执行完成
[... 多个 tool call 并行执行，结果顺序按完成时间 ...]
chat.done             ← 所有步骤完成
```

---

## 十二、更新 `app/chat/chat.go`

**Service struct**：

```go
type Service struct {
    repo        chatdomain.Repository
    convRepo    convdomain.Repository
    modelPicker modeldomain.ModelPicker
    keyProvider apikeydomain.KeyProvider
    llmFactory  *llminfra.Factory   // 替代 einoinfra.ChatModelFactory
    tools       []agentdomain.Tool  // 替代 []tool.BaseTool
    bridge      events.Bridge
    dataDir     string
    log         *zap.Logger
    queues      sync.Map
}
```

---

## 十三、更新 `cmd/server/main.go`

- 移除 `einoinfra` + `"github.com/cloudwego/eino/schema"` import
- 新增 `llminfra "…/internal/infra/llm"` + `agentdomain "…/internal/domain/agent"`
- `llmFactory := llminfra.NewFactory()`
- `toolLLMClientAdapter.Generate()` 重写为消费 `client.Stream()` 拼接结果
- `db.Migrate` 新增 `&chatdomain.Block{}`
- `agentpkg.WebTools()` 签名变化（移除 context 参数，不再需要 duckduckgo 初始化）

---

## 十四、删除 `internal/infra/eino/`

删除：`factory.go`、`openai.go`、`anthropic.go`、`ollama.go`

---

## 十五、清理 `go.mod`

`go mod tidy` 后消失的依赖：
- `github.com/cloudwego/eino`
- `github.com/cloudwego/eino-ext/components/model/openai`
- `github.com/cloudwego/eino-ext/components/model/ollama`
- `github.com/cloudwego/eino-ext/components/tool/duckduckgo`
- `github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2`
- 所有 Eino 间接依赖（`bytedance/sonic`、`cloudwego/base64x` 等）

---

## 十六、实施顺序

```
1. internal/infra/llm/        → 新建（types.go, openai.go, anthropic.go, factory.go）
2. internal/domain/agent/     → 新建（tool.go）
3. internal/domain/chat/      → 改（Block 实体，data 结构体）
4. internal/infra/db/         → 改（message_blocks 表）
5. internal/infra/store/chat/ → 改
6. internal/app/agent/        → 改（system.go, web.go, forge.go）
7. internal/app/chat/chat.go  → 改（Service struct）
8. internal/app/chat/pipeline.go → 全部重写
9. internal/app/chat/util.go  → 改
10. internal/domain/events/   → 改（新增 ChatToolCallStart，更新 ChatToolCall）
11. cmd/server/main.go        → 改
12. 删除 internal/infra/eino/
13. go mod tidy && go build ./...
```

---

## 十七、Review — 本次迭代的设计决策与质量评估

### ✅ 本次做对的事

**DB 模型**：block-per-row 和 LLM 的内容结构完全对齐，新增内容类型无需改表，前端渲染逻辑也更清晰。

**iter.Seq 替代 channel**：Go 1.25 的拉式迭代器，无 goroutine 泄漏风险，consumer break 即干净退出，背压天然。

**Tool 接口极简**：4 个方法，`summary` 完全由框架处理，tool 实现者只关心业务逻辑。这是正确的关注点分离。

**LLM 提供 summary**：比后端用参数硬拼更有表达力。LLM 知道上下文，能写出"为了回答用户的问题，查询北京天气"这样有意义的描述。

**并行 tool call 执行**：正确利用了 LLM 的 parallel tool calls 能力，不再串行浪费时间。

**Anthropic 原生客户端**：block 格式天然对应我们的 block 模型，`content_block_stop` 信号未来可用于更精细的流控。

### ⚠️ 需要关注的点

**summary 字段的 required**：LLM 不 100% 遵守 required。建议在 `StripSummary` 里做好兜底——`summary` 缺失时用 tool name 作为默认值，不要让前端收到空 summary。

**并行 tool result 的 SSE 顺序**：并行执行时 `chat.tool_result` 事件的到达顺序不固定（谁先执行完谁先推），前端需要用 `toolCallId` 对应，不能假设顺序。

**Anthropic tool result 格式**：Anthropic 原生 API 的 tool result 放在 user message 的 content 数组里，`buildLLMHistory` 里 Anthropic 和 OpenAI 的重建逻辑需要分开处理。

**`injectSummaryField` 的冲突检测**：如果某个 tool 的业务参数里已经有 `summary` 字段，注入会产生冲突。建议注入时检测，冲突则 panic（开发期发现）而不是静默覆盖。

### 🔮 后续可做的优化

**LLM provider 错误分类**：目前错误统一为字符串。可定义 `LLMError` 类型，区分 `ErrAuth`（401）、`ErrRateLimit`（429）、`ErrContextTooLong`（400）、`ErrServerError`（5xx），前端可以分别展示不同的引导信息。

**Anthropic content_block_stop 用于提前执行**：当收到 `content_block_stop` for tool_use block 时，该 tool call 的 arguments 已完整，可以立刻启动执行，不等整个流结束。这需要在 pipeline 里维护"已完整的 tool call"集合，复杂度增加，可作为后续优化。

**消息 FTS5 全文索引**：旧 schema 有 `content` 列可以直接建 FTS5。新 schema 的文字内容在 `message_blocks.data` 的 JSON 里，FTS5 索引需要用生成列或触发器提取。可以在 `schema_extras.go` 里加一个 `text_content TEXT GENERATED ALWAYS AS (json_extract(data, '$.text')) VIRTUAL` 列然后建 FTS5。

---

## 十八、验证清单

- [ ] `go build ./...` 通过，Eino import 全部消失
- [ ] 现有测试套件全部通过（~170 tests）
- [ ] 手工：普通对话（text only）
- [ ] 手工：单步 tool call（含 summary 显示）
- [ ] 手工：多步 ReAct
- [ ] 手工：parallel tool calls（验证并行执行 + SSE 顺序）
- [ ] 手工：取消流（status=cancelled）
- [ ] 手工：mid-stream 网络断开（status=error）
- [ ] 手工：DeepSeek-R1 reasoning block
- [ ] 手工：附件上传后发消息
- [ ] 手工：Anthropic 原生 API（tool call、thinking）
- [ ] SSE 时序：`tool_call_start` → `tool_call`（含 summary） → `tool_result`
- [ ] DB：`message_blocks` 数据完整，summary 存入 tool_call block
- [ ] `go mod tidy` 后 Eino 包从 go.sum 消失

---

## 十九、详细执行计划

> 本计划遵循 `backend-design.md` 全套规范。执行过程中每完成一步必须即时更新 `progress-record.md`（S14，最高优先级）。

### 前置：开工前必做

**1. 端到端推演（设计原则 #5）**

开工前先用文字走一遍完整调用链，确认每层职责清晰，无遗漏依赖：

```
POST /api/v1/conversations/{id}/messages
  → ChatHandler.SendMessage（handler ≤ 20 行，S6）
    → chatapp.Service.Send
        → convRepo.Get → Conversation
        → chatdomain.Message{role:user} → repo.Save（含 user message blocks）
        → getOrCreateQueue → enqueue queuedTask
  ← 202 {"messageId": "msg_xxx"}

Worker goroutine:
  → modelPicker.PickForChat → (provider, modelID)
  → keyProvider.ResolveCredentials → (key, baseURL)
  → llmFactory.Build(Config{provider, ...}) → (Client, baseURL)
  → agent.ToLLMDefs(s.tools) → []ToolDef（含注入的 summary 字段）
  → buildLLMHistory(messages, provider) → []LLMMessage
  → runReactLoop:
      for step < 20:
        client.Stream(ctx, Request{...}) → iter.Seq[StreamEvent]
        for event := range stream:
          EventText      → bridge.Publish(ChatToken)
          EventReasoning → bridge.Publish(ChatReasoningToken)
          EventToolStart → bridge.Publish(ChatToolCallStart)
          EventToolDelta → accum
          EventFinish    → record usage
        assembleAssistantBlocks → save message+blocks（事务）
        if no tool calls → break
        executeToolCalls（并行）:
          StripSummary(args) → summary + strippedArgs
          bridge.Publish(ChatToolCall{summary})
          tool.Execute(strippedArgs) → result
          bridge.Publish(ChatToolResult)
          save tool_result blocks（追加到 assistant message）
  → bridge.Publish(ChatDone)
  → autoTitle（异步）

GET /api/v1/events?conversationId=xxx
  → ChatHandler.EventsSSE
    → bridge.Subscribe → ch
    → for event := range ch: write SSE + flush
```

**2. 文档预检（S14）**

在动第一行代码前，确认以下文档状态均已知晓，后续改动将同步更新：
- `service-design-documents/chat.md` — 将大幅更新
- `service-contract-documents/database-design.md` — 新增 message_blocks 表
- `service-contract-documents/events-design.md` — 新增 ChatToolCallStart，更新 ChatToolCall
- `progress-record.md` — 每步完成后追加 dev log

---

### Step 1：新建 `internal/infra/llm/`

**目标**：自主 LLM 流式客户端，完全替代 `infra/eino/`。

**文件**：
```
internal/infra/llm/
├── llm.go        ← 包 doc + 核心类型（StreamEvent, LLMMessage, ToolDef, Client 接口）
├── openai.go     ← openAIClient：OpenAI-compat SSE，iter.Seq 实现
├── anthropic.go  ← anthropicClient：Anthropic 原生 /v1/messages，content block 格式
└── factory.go    ← Factory：按 provider dispatch + resolveBaseURL
```

**规范检查**：
- S11：所有导出符号双语 godoc（英文先，空行，中文后）
- S12：主文件用包名（`llm.go`），平铺按概念拆，无子目录
- S13：包名 `llm`，调用方别名 `llminfra`
- S5：每个函数 ≤ 60 行；`openai.go` + `anthropic.go` 各自 ≤ 500 行

**关键实现点**：
- `Client.Stream()` 返回 `iter.Seq[StreamEvent]`，`yield` 返回 false 时立即终止（consumer break）
- openai.go 的 SSE 解析：`EventToolStart` 在首次出现 tool name 时 emit，不等 arguments 完整
- anthropic.go 的 SSE 解析：`content_block_start` (tool_use) → `EventToolStart`；`content_block_stop` → 可选的 `EventToolComplete`（pipeline 未来可利用）
- `factory.go` 的 `resolveBaseURL`：custom provider 无 BaseURL 时返回明确错误，不 panic（S3：错误不吞）
- HTTP 错误分类：401 → 认证失败，429 → 限流，400 → 请求格式错误，5xx → provider 内部错误；返回有类型的 error

**完成后文档同步（S14）**：
- `progress-record.md` 追加：`[refactor] Step 1 完成：新建 infra/llm，实现 OpenAI compat + Anthropic 原生 SSE 客户端，iter.Seq 替代 channel`

---

### Step 2：`internal/app/agent/` — 新增 Tool 接口

**目标**：在 `app/agent/` 包内定义 Tool 接口，取代 Eino `tool.BaseTool`。放在 app 层而非 domain 层，因为 Tool 接口仅被 `app/chat` 和 `app/agent` 消费，不涉及 domain 层职责。

**文件**：`internal/app/agent/tool.go`（新建）

```go
// 导出：
type Tool interface { Name(), Description(), Parameters(), Execute() }
func ToLLMDef(t Tool) llminfra.ToolDef       // 注入 summary 字段
func ToLLMDefs(tools []Tool) []llminfra.ToolDef
func StripSummary(argsJSON string) (summary, stripped string)
func injectSummaryField(params json.RawMessage) json.RawMessage
func WithConversationID / GetConversationID   // context helpers（从 forge.go 迁移）
```

**规范检查**：
- S12：文件名 `tool.go`，概念内聚（Tool 接口 + 相关工具函数），放在 app/agent 包根
- S5：`injectSummaryField` 函数 ≤ 60 行；如 JSON 操作复杂可拆 helper
- E1：`summary` 字段注入时若已存在则 panic（开发期快速失败，不静默覆盖）

**完成后文档同步（S14）**：
- `progress-record.md` 追加：`[refactor] Step 2 完成：app/agent/tool.go 定义 Tool 接口，summary 框架注入`

---

### Step 3：`internal/domain/chat/chat.go` — 更新实体

**目标**：新增 Block 实体，精简 Message。

**改动**：
- `Message` struct：移除 `Content`、`ReasoningContent`、`ToolCalls`、`ToolCallID`、`AttachmentIDs`、`TokenUsage`；新增 `InputTokens int`、`OutputTokens int`、`Blocks []Block`（`gorm:"-"`）；`role` 常量移除 `RoleTool`
- 新增 `Block` struct（见 §三.3.2）
- 新增 block 类型常量和 data 结构体（`TextData`、`ReasoningData`、`ToolCallData`、`ToolResultData`、`AttachmentRefData`）
- 更新 `Repository` 接口（`Save` 语义扩展为同时写 blocks）

**规范检查**：
- S11：`Block` 和新常量需双语 godoc
- S12：若 chat.go 超过 500 行，按概念拆出 `block.go`（Block 实体 + 类型 + data 结构体）
- D1：Block 无 `deleted_at`（blocks 不软删除，message 删除时 blocks 跟随删除）
- D2：Block 有 `created_at`，无 `updated_at`（不可变）
- D4：Block.MessageID 声明外键（GORM tag `gorm:"not null;index;type:text;references:id"`）

**完成后文档同步（S14）**：
- `service-contract-documents/database-design.md`：新增 message_blocks 表行（列说明、索引、外键）
- `service-design-documents/chat.md`：更新 §领域模型，新增 Block 实体描述
- `progress-record.md` 追加 dev log

---

### Step 4：`internal/infra/db/` — 新增 message_blocks 表

**目标**：清空旧 messages 表，重建新 schema。

**改动**：
- `migrate.go`：`db.Migrate` 调用里在 `&chatdomain.Message{}` 后面加 `&chatdomain.Block{}`
- `schema_extras.go`：新增复合索引 SQL

```sql
-- message_blocks 的 (message_id, seq) 复合索引，GORM 单字段 index tag 不够
CREATE INDEX IF NOT EXISTS idx_mb_msg_seq ON message_blocks(message_id, seq);
```

**关于清空重来**：
- 在 `migrate.go` 或 `schema_extras.go` 里加 `DROP TABLE IF EXISTS messages` + `DROP TABLE IF EXISTS message_blocks`，确保每次启动以新 schema 重建。
- 这是一次性操作，生产前移除 DROP 语句。

**规范检查**：
- S8：所有 SQL 只在 `infra/db/` 里（DROP、CREATE INDEX 均在此）
- D2：`created_at` GORM 自动维护，Block 无 `updated_at`（不可变）
- D4：`message_blocks.message_id` FK 在 schema_extras 里用 `FOREIGN KEY` 语句显式声明（GORM tag 不足以生成 FK 约束）

**完成后文档同步（S14）**：
- `service-contract-documents/database-design.md`：message_blocks 表行加索引信息
- `progress-record.md` 追加 dev log

---

### Step 5：`internal/infra/store/chat/chat.go` — 更新 store

**目标**：Save 同时写 blocks（事务），ListByConversation 附带 blocks（避免 N+1）。

**改动**：
- `Save`：用 GORM 事务包裹 message upsert + blocks delete+insert（见 §七.7.1）
- `ListByConversation`：分页查 messages 后，一次 `WHERE message_id IN (...)` 批量取 blocks（见 §七.7.2）
- 移除旧的 `content`、`reasoning_content` 等字段相关逻辑

**规范检查**：
- S8：SQL 只在 store 层，不上浮到 app 层
- S9：所有方法传 `ctx`，且从 `ctx` 读 `userID`（`reqctx.GetUserID`）
- S3：事务失败时 `return fmt.Errorf("chatstore.Save: %w", err)`，不吞错误
- S5：`Save` 函数 ≤ 60 行；若超过，拆出 `saveBlocks(tx, blocks)` helper

**完成后文档同步（S14）**：
- `service-design-documents/chat.md`：更新 §存储层，描述 Save 事务语义
- `progress-record.md` 追加 dev log

---

### Step 6：`internal/domain/events/types.go` — 更新 SSE 事件

**目标**：新增 `ChatToolCallStart`，更新 `ChatToolCall`（加 `Summary` 字段）。

**改动**：
```go
// 新增
type ChatToolCallStart struct { ConversationID, MessageID, ToolCallID, ToolName string }
func (e ChatToolCallStart) EventName() string { return "chat.tool_call_start" }

// 更新（加 Summary）
type ChatToolCall struct {
    ConversationID, MessageID, ToolCallID, ToolName, ToolInput, Summary string
}
```

**规范检查**：
- E1：`ChatToolCallStart` 必须有真实发布点（Step 9 的 pipeline 里会发）；禁止 `map[string]any`
- E2：`"chat.tool_call_start"` 符合 `domain.action` snake_case 格式
- S11：新增结构体需双语 godoc

**完成后文档同步（S14）**：
- `service-contract-documents/events-design.md`：新增 `chat.tool_call_start` 行，更新 `chat.tool_call` 的 payload 字段
- `service-design-documents/chat.md`：更新 §SSE 事件，描述新事件时序
- `progress-record.md` 追加 dev log

---

### Step 7：`internal/app/agent/system.go`、`web.go`、`forge.go` — 更新 tool 实现

**目标**：所有 tool 实现新的 4 方法 `Tool` 接口，移除 Eino import。

**system.go 改动**：
- 每个 tool struct 实现 `Name()`、`Description()`、`Parameters()`、`Execute()`
- 移除 `Info()`、`CoreInfo()`、`InvokableRun()` 方法
- 移除 `github.com/cloudwego/eino` import
- `SystemTools()` 返回类型从 `[]tool.BaseTool` 改为 `[]Tool`

**web.go 改动**：
- `FetchURLTool` 实现新接口
- `web_search` 直接实现，移除 `github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2`
- `WebTools()` 签名：移除 `context.Context` 参数（不再需要 duckduckgo 初始化），返回 `[]Tool`

**forge.go 改动**：
- 5 个 forge tool 实现新接口
- `ForgeTools()` 返回类型改为 `[]Tool`，参数 `modelFactory` 从 `einoinfra.ChatModelFactory` 改为 `*llminfra.Factory`
- `buildModel` → `buildClient`，返回 `(llminfra.Client, string, error)`
- `streamCode`：消费 `iter.Seq[StreamEvent]`，推 `ToolCodeStreaming` SSE
- 非流式 LLM 调用（`search_tools` 排序）：消费 `Stream()` 拼接 EventText delta

**规范检查**：
- S11：`Parameters()` 返回的 JSON Schema 字符串内嵌在函数体里时，加注释说明字段含义
- S5：`forge.go` 当前 699 行，改完后若超过 500 行应按概念拆文件（如 `forge_stream.go` 放 streamCode helper）
- S3：`streamCode` 里的流式错误（`EventError`）必须返回，不静默忽略
- 移除 `ExtractFallbackSummary`（不再需要，summary 由 LLM 提供）

**完成后文档同步（S14）**：
- `progress-record.md` 追加 dev log

---

### Step 8：`internal/app/chat/chat.go` — 更新 Service

**目标**：替换 Eino 依赖，更新 `Service` struct 和公共方法签名。

**改动**：
- `modelFactory einoinfra.ChatModelFactory` → `llmFactory *llminfra.Factory`
- `tools []tool.BaseTool` → `tools []agent.Tool`
- `SetTools(tools []agent.Tool)`
- 移除 `einoinfra` import，新增 `llminfra`、`agent`（均在 app 层内）
- `NewService` 签名更新

**规范检查**：
- S13：`llminfra` 是 `infra/llm` 的别名，`agent` 是 `app/agent`（同层，直接用包名不需别名，或用 `agentpkg` 区分）
- S6：`Send()`、`Cancel()`、`ListMessages()` 等公共方法保持简洁，业务逻辑下沉到 pipeline

**完成后文档同步（S14）**：
- `service-design-documents/chat.md`：更新 §Service struct 依赖列表
- `progress-record.md` 追加 dev log

---

### Step 9：`internal/app/chat/pipeline.go` — 全部重写

**目标**：事件驱动 pipeline，iter.Seq，并行 tool 执行，正确的 status 写入。

**文件拆分**（S5，预计代码量较大）：

```
app/chat/
├── chat.go       ← Service struct + 公共方法（Send, Cancel, ListMessages, UploadAttachment）
├── pipeline.go   ← processTask, runReactLoop, runStep（主流程）
├── stream.go     ← iter.Seq 消费逻辑（consumeStream, assembleAssistantBlocks）
├── tools.go      ← executeToolCalls（并行执行），executeTool
├── history.go    ← buildLLMHistory, buildUserLLMMessage, buildAssistantLLMMessages
└── util.go       ← newMsgID, newBlockID, makeBlock, imageToInputPart, truncate 等
```

**关键实现规范**：

`runStep` ≤ 60 行，拆出 `consumeStream`：
```go
// consumeStream iterates the event stream, publishing SSE and accumulating tool calls.
// Returns assembled blocks and any stream error.
//
// consumeStream 消费事件流，推送 SSE，累积 tool call。返回组装好的 blocks 和流错误。
func (s *Service) consumeStream(ctx context.Context, stream iter.Seq[StreamEvent], ...) (blocks, error)
```

`executeToolCalls` 并行执行：
- `sync.WaitGroup` + `chan result`
- 每个 tool 在独立 goroutine 中：`StripSummary` → `Publish(ChatToolCall)` → `executeTool` → `Publish(ChatToolResult)`
- 结果按原始 `accum` index 排序后写 blocks，保证 seq 稳定

取消/错误状态写入（修复旧 bug）：
```go
switch {
case ctx.Err() != nil:
    status, stopReason = StatusCancelled, StopReasonCancelled
case streamErr != nil:
    status, stopReason = StatusError, StopReasonError
default:
    status, stopReason = StatusCompleted, StopReasonEndTurn
}
```

**规范检查**：
- S5：每个函数 ≤ 60 行，每个文件 ≤ 500 行（stream.go 和 tools.go 分开正是为此）
- S9：所有函数传 `ctx`
- S10：`executeToolCalls` 里的 tool 执行失败需 `s.log.Warn(...)`（异步操作必须打 log）
- S3：`executeTool` 找不到工具时返回 `(result="tool not found", ok=false)`，不 panic，不静默忽略

**完成后文档同步（S14）**：
- `service-design-documents/chat.md`：更新 §调用链 + §pipeline 流程图 + §SSE 事件时序
- `progress-record.md` 追加详细 dev log（文件拆分方式、并行执行决策、iter.Seq 理由）

---

### Step 10：`cmd/server/main.go` — 更新 wiring

**目标**：移除 Eino 相关依赖，接入新的 llmFactory 和 Tool 接口。

**改动**：
- 移除：`einoinfra`、`"github.com/cloudwego/eino/schema"` import
- 新增：`llminfra "…/internal/infra/llm"`
- `einoFactory` → `llmFactory := llminfra.NewFactory()`
- `ForgeTools(...)` 参数：`einoFactory` → `llmFactory`
- `WebTools()` 调用：移除 `context.Background()` 参数
- `chatService.SetTools(allTools)`：`allTools` 类型从 `[]tool.BaseTool` 改为 `[]agent.Tool`
- `toolLLMClientAdapter.Generate`：从 Eino 切到消费 `llmFactory.Build(cfg).Stream(req)` 拼接 text delta
- `db.Migrate` 新增 `&chatdomain.Block{}`（放在 `&chatdomain.Message{}` 之后）

**规范检查**：
- S9：`WebTools()` 移除 ctx 参数后，main 里对应调用也更新
- S3：`toolLLMClientAdapter.Generate` 里 Stream 的 EventError 必须返回，不吞

**完成后文档同步（S14）**：
- `progress-record.md` 追加 dev log

---

### Step 11：删除 `internal/infra/eino/`

```bash
rm internal/infra/eino/factory.go
rm internal/infra/eino/openai.go
rm internal/infra/eino/anthropic.go
rm internal/infra/eino/ollama.go
rmdir internal/infra/eino
```

确认无任何文件仍 import `github.com/cloudwego/eino`：

```bash
grep -r "cloudwego/eino" --include="*.go" internal/ cmd/
# 期望：0 行输出
```

---

### Step 12：`go mod tidy` + 编译验证

```bash
go mod tidy
go build ./...
go test -count=1 -race ./...
```

期望：
- `go.sum` 中 `cloudwego/eino*` 全部消失
- `go build` 零错误
- 测试套件零失败

---

### Step 13：手工测试（按验证清单）

按 §十八 验证清单逐条测试。

SSE 事件时序重点验证（用 `testend` 的 SSE monitor）：

```
chat.tool_call_start   ← tool name 出现后立刻，arguments 尚未完整
chat.tool_call         ← arguments 完整后，执行前，含 summary
chat.tool_result       ← 执行完成
```

---

### Step 14：重构完成后文档全量同步（S14）

**这是最后一步，也是 S14 规定必须做的。**

- [ ] `service-design-documents/chat.md`：全量更新，逐字段匹配新代码（实体、方法签名、pipeline 流程、SSE 事件、调用链）
- [ ] `service-contract-documents/database-design.md`：messages 表更新，message_blocks 表新增
- [ ] `service-contract-documents/events-design.md`：chat.tool_call_start 新增，chat.tool_call payload 更新
- [ ] `service-contract-documents/api-design.md`：ListMessages 响应结构变化（blocks 数组）
- [ ] `progress-record.md`：重构完工日志，说明 Eino 移除、新架构决策、iter.Seq 理由

---

### 规范速查卡（执行中随时对照）

| 规范 | 要点 |
|------|------|
| **S3** | 错误不吞；`_ = err` 必须注释；找不到工具返 `ok=false`，不 panic |
| **S5** | 函数 ≤ 60 行；文件 ≤ 500 行（概念内聚可到 500）|
| **S6** | Handler ≤ 20 行：只解析 / 调用 / 序列化 |
| **S8** | SQL 只在 `infra/store/` 和 `infra/db/` |
| **S9** | 每个跨层调用传 `ctx` |
| **S10** | 异步/fire-and-forget 必须打 zap log；同步原语（store）由调用者打 |
| **S11** | 导出符号双语 godoc；英文先，空行，中文后；不写"做什么"，写"为什么" |
| **S12** | 包平铺，按概念拆文件，主文件用包名（`chat.go`、`llm.go`）|
| **S13** | 三层同名；调用方 `<name><role>` 别名（`llminfra`、`agentpkg`）|
| **S14** | 每步完成→即时更新 `progress-record.md`；涉及 API/DB/事件→同步 contract docs |
| **E1** | SSE struct 必须有真实发布点；禁 `map[string]any` |
| **E2** | 事件名 `domain.action` snake_case；必带 `conversationId` |
| **D4** | 外键显式声明（GORM tag + schema_extras SQL）|
