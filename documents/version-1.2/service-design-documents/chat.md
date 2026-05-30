# chat domain — 详细设计文档

**所属 Phase**：Phase 2 起（每个 Phase 都会升级）
**状态**：✅ 已实现到 Phase 3（含事件日志协议统一 2026-05-08 + loop 引擎抽离 2026-05）；Phase 4-5 时再升级
**地位**：**全系统最核心的 domain**——用户的每一次对话都从这里进入，一切能力都通过这里编排。

> **🔧 限制优化（2026-05-31，limits-optimization）**：ReAct `maxSteps`（20→150）/ `maxTurnDuration`（10→30min）现读 `limits.Current()`；撞顶写诚实终态 `StopReasonMaxSteps`+`MAX_STEPS_REACHED`（不再冒充 completed）；`maxHistoryMessages` 200→2000 + `buildHistory` 对 archived 消息统一投影（含 user）；LLM idle 死连接超时替代 120s 总墙钟。详 [`../adhoc-topic-documents/limits-optimization/`](../adhoc-topic-documents/limits-optimization/)。

**关联文档**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../event-log-protocol.md`](../event-log-protocol.md) — 事件日志协议事实源（5 events × 6 block types）
- [`../service-contract-documents/api-design.md`](../service-contract-documents/api-design.md) — API 索引
- [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) — 事件契约（双协议）
- [`./subagent.md`](./subagent.md) — Subagent system tool；嵌套 sub-run 走同一事件协议

---

## 1. 核心思想：一切都是 Tool Call

### 1.1 为什么

Forgify 的终极形态是：用户一句话，AI 自主完成"创建工具→测试→组建工作流→挂知识库→部署"的完整链路，中间多次迭代，用户实时看到每一步。

这本质上是一个**自主 Agent 循环**，而不是简单的"识别意图→路由→执行一次"。

### 1.2 是什么

从 LLM 的视角，它只有两种输出：
- **直接回复**（= 任务完成）
- **调一个或多个 Tool**（= 还有事情要做）

所有 Forgify 的能力——创建工具、运行沙箱、搜知识库、创建工作流——对 LLM 都是 Tool。Agent 每轮决策（调哪些 Tool 或直接回复），拿到结果后再想下一步，直到认为任务完成。

这就是 **ReAct 循环**（Reasoning + Acting），和 Claude Code 的工作方式完全一致。

### 1.3 关键约束

**每一步都可观测、可中断、可追溯。** 一个 turn 内允许多个并行 tool call（LLM 自报 `execution_group` 决定并行 batch），但每个 Tool 的 start/delta/stop 都通过事件流推给前端。

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
│  System Tools（永远在 context，~21 个）              │
│  ┌────────────┐ ┌──────────┐ ┌────────────────────┐ │
│  │ create_forge│ │ edit_forge│ │     run_forge(id)    │ │
│  └────────────┘ └──────────┘ └────────────────────┘ │
│  ┌─────────────┐ ┌──────────────────────────────┐   │
│  │ search_forges│ │ get_forge / Subagent / ...    │   │
│  └─────────────┘ └──────────────────────────────┘   │
│  ... filesystem / search / web / shell / task / ask  │
└─────────────────────────────────────────────────────┘

用户工具库（不在 context，通过 search_forges 发现，run_forge 执行）
┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│ email_parser │ │ csv_processor│ │  ...（数百个）│
└──────────────┘ └──────────────┘ └──────────────┘
```

**System Tools** 是 meta-tools：用来创建/管理其他工具和工作流。永远可见。

**User Tools** 不直接注入 context。Agent 通过：
1. `search_forges(query)` → 语义搜索工具库，得到相关工具列表
2. `run_forge(id, input)` → 通用执行器，执行任意用户工具

这本质上是 **Tool RAG**——与知识库 RAG 同一个思路，检索对象是工具描述。

### 2.3 System Tools 完整目录

| 家族 | Phase | Tool | 描述 | 对接的 domain |
|---|---|---|---|---|
| forge | 3 | `search_forge` / `get_forge` / `create_forge` / `edit_forge` / `run_forge` | 用户工具库 CRUD + 执行 | forge sandbox |
| filesystem | 5 | `Read` / `Write` / `Edit` | 文件读写编辑（PathGuard 守敏感路径，Edit 走 must-Read-first 守卫 + 原子写）| `pkg/agentstate.SeenFiles` |
| search | 5 | `Grep` / `Glob` | 内容搜索 + 文件查找（rg 优先、stdlib 兜底；Glob 输出 type/size/mtime 替代 LS）| 文件系统 |
| web | 5 | `WebFetch` / `WebSearch` | URL 抓 + 摘要（Jina + 直 GET fallback）/ BYOK → MCP 两层搜索 | `llmclient.ResolveUtility`（utility scenario）|
| shell | 5 | `Bash` / `BashOutput` / `KillShell` | 前后台 shell（cwd 状态机走 AgentState；后台 ProcessManager 注册 256 KB 环形缓冲）| `pkg/agentstate.Cwd` |
| task | 5 | `TaskCreate` / `TaskList` / `TaskGet` / `TaskUpdate` | 对话级 to-do 列表 | task domain（mini-domain，详 task.md）|
| ask | 5 | `AskUserQuestion` | 暂停 agent loop 等用户回答 | app/ask（in-memory 会合，POST /answers）|
| subagent | 4 | `Subagent` | 隔离 context spawn 子 agent loop | subagent（详 subagent.md）|
| workflow | 4 | `create_workflow` / `edit_workflow` / `run_workflow` | 创建/执行工作流 | workflow + flowrun（未实现）|
| knowledge | 5 | `search_knowledge` | RAG 检索知识库 | knowledge（未实现）|
| mcp | 5 | `mcp_call` | 调用 MCP 服务器方法 | mcpserver（未实现）|

**当前在线**：5 forge + 15 phase-5 + 1 Subagent = **21 个** system tool（Phase 4 前）。装配点：`cmd/server/main.go::tools = append(tools, ...)` 链。

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
    └── providerClient     ← 把一个 Provider 适配成 Client，跑共享传输铁律
            ↓ 持有
        Provider           ← 一种 wire 方言（BuildRequest / ParseStream / Name / DefaultBaseURL）
            ├── openAICompatProvider  ← 9 个 OpenAI-compat provider 共用一份 body/SSE 逻辑，仅身份不同
            └── anthropicProvider     ← Anthropic 原生 /v1/messages 协议
```

**P2.0 重构（Provider 接口 + 共享传输 + 注册表）**：`Client` 实现从「两个共享 wire client + 薄 adapter」改为「N 个 Provider 注册项，背后共享一份 compat / anthropic wire 逻辑」。`providerRegistry` 按 name 索引；`buildProviderRegistry` 从 `adapters` 列表派生（name + DefaultBaseURL 单一来源）；mock 不入 registry（Factory 直接短路到 MockClient）。SSE 解析 / tool-call index 合成 / reasoning round-trip / sanitize / retry 全部逐字保留。per-provider 请求微调（deepseek 剥 reasoning、ollama 关流）仍在 `adapter.go` 的 `adapterWrappedClient` 钩子里，wrapping 顺序不变。

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

### 3.3 OpenAI 兼容 Provider（`infra/llm/openai.go`）

`openAICompatProvider{name, defaultBaseURL}` 覆盖所有 OpenAI-compat provider：openai / deepseek / qwen / zhipu / moonshot / doubao / openrouter / google(compat) / ollama / custom。**body/SSE 逻辑只此一份**，9 个 provider 仅 name + base URL 不同。

- `BuildRequest`：`buildOpenAIBody` → POST `<baseURL>/chat/completions` + `Authorization: Bearer`
- `ParseStream`：`req.DisableStream` 时走 `parseOpenAINonStreaming`（Ollama+tools），否则 `parseOpenAISSE`
- 自写 SSE line reader（`data: {...}\n\n` 格式）；解析 `choices[0].delta.content` / `reasoning_content`（DeepSeek-R1）/ `tool_calls`；`toolCallState` 对不填 index 的 provider 按 ID 合成 index
- 畸形 chunk → emit EventError，不 panic

### 3.4 Anthropic 原生 Provider（`infra/llm/anthropic.go`）

`anthropicProvider` 使用 Anthropic 原生 `/v1/messages` 协议（SSE 格式）：
- `BuildRequest`：`buildAnthropicBody` → POST `<baseURL>/v1/messages` + `x-api-key` + `anthropic-version`；system / 最后一个 tool 带 `cache_control`
- `ParseStream`：`parseAnthropicSSE` —— `content_block_start` 识别 text / thinking / tool_use；`content_block_delta` 分发 EventText / EventReasoning / EventToolDelta
- tool result 消息格式与 OpenAI 不同：将连续 tool results 合并为一条 `role="user"` 消息（`content = [{type:"tool_result", tool_use_id, content}...]`）

### 3.5 共享传输（`infra/llm/transport.go`）

`providerClient.Stream` 跑共享「铁律」：`BuildRequest` → `doRequest`（共享 `*http.Client`，120s timeout）→ status→sentinel（`classifyHTTPError` 区分 401/429/400/404/5xx）→ `ParseStream`。ctx 取消静默终止（不发 EventError）。所有 Provider 共用这一份请求/响应管道。

### 3.6 Factory（`infra/llm/factory.go`）

```go
// Factory.Build 按 Config 解析 Provider，包成 Client
func (f *Factory) Build(cfg Config) (Client, string, error) {
    // mock                              → MockClient（短路，不入 registry）
    // custom + APIFormat=anthropic-compatible → anthropicProvider
    // 其余（含 anthropic / 未知）       → lookupProvider 查 registry，未知回落 openai-compat
    //   → providerClient → adapterWrappedClient（BeforeRequest 微调）→ recordingClient（若 tracer）
}
```

Provider 基础 URL 由 `resolveBaseURL` 经 `lookupAdapter(provider).DefaultBaseURL()` 给出（adapter 仍是 base-url 权威源，含 mock=`mock://in-process` / custom=空），调用方传入的 `BaseURL` 覆盖默认值。

---

## 4. Tool 接口 & 标准字段注入（`app/tool/tool.go`）

完整规约见 [`CLAUDE.md §S18`](../../../CLAUDE.md)。本节只讲 chat 层视角的关键交互。

### 4.1 Tool 接口（9 方法全必填）

```go
type Tool interface {
    // Identity（3 个）
    Name() string
    Description() string
    Parameters() json.RawMessage   // JSON Schema；禁止含 "summary" / "destructive" / "execution_group"

    // 静态元数据（3 个固有属性）
    IsReadOnly() bool              // 仅文档/语义参考；不再驱动并发调度
    NeedsReadFirst() bool          // Phase 5 Edit/Write 用 + 走 AgentState.SeenFiles
    RequiresWorkspace() bool       // PathGuard 守卫开关（Phase 5）

    // 钩子（args-dependent，2 个）
    ValidateInput(args json.RawMessage) error
    CheckPermissions(args json.RawMessage, mode PermissionMode) PermissionResult

    // 主入口（args 已剥除 summary / destructive / execution_group）
    Execute(ctx context.Context, argsJSON string) (string, error)
}
```

### 4.2 标准字段注入机制（summary + destructive + execution_group）

```
ToLLMDef(tool)
  → injectStandardFields(tool.Parameters())
    → properties 加 "summary"（必填 string）/ "destructive"（可选 bool 默认 false）
                  / "execution_group"（可选 int ≥1）
    → required 把 "summary" 插到第一位
  → 返回发给 LLM 的 ToolDef（含三个标准字段）

runOneTool(ctx, t, tc)  -- 实际位于 app/loop/tools.go
  → ValidateInput(args) — 失败转失败 tool_result
  → CheckPermissions(args, PermissionModeDefault) — Deny 转失败；Ask 当前阶段当 Allow
  → Execute(ctx, argsJSON) — 此时 args 已剥除三个标准字段
  → 在 ctx 上挂 WithParentBlockID(tc.ID)，让 tool 内调用的 emit 自动挂在
    tool_call block 下

parseToolArgs(rawArgs)  -- 实际位于 app/loop/stream.go
  → toolapp.StripStandardFields(rawArgs)
    → (StandardFields{Summary, Destructive, ExecutionGroup}, stripped)
  → 填进 ToolCallData 的三个一等字段；剩余 args 作为 Arguments map
```

**destructive 设计**：per-call AI 自报，比静态 IsDestructive() 精准（同一 tool 不同 args 可不同）。存进 `ToolCallData.Destructive` 一等字段，前端从 block_start 的 attrs 渲染警示徽章。

**execution_group 设计**：LLM 自报的并行 batch 提示（≥1）。partition 层用它取代旧 `IsConcurrencySafe`：同 group 并行、不同 group 升序串行；缺失（≤0）的 call 自动分配 ≥1000 的唯一 group（fail-safe 默认 = 独自串行，排在所有显式 group 之后）。详见 §5.4。

### 4.3 Context Helpers（`pkg/reqctx/agentrun.go`）

agent-run 标识符 helpers 不再属于 tool 包，统一归 `pkg/reqctx`（与 user 身份 / locale 同包）：

```go
// pkg/reqctx/agentrun.go
func WithConversationID(ctx, id) context.Context
func GetConversationID(ctx) (string, bool)
func WithMessageID(ctx, id) context.Context
func GetMessageID(ctx) (string, bool)
func WithToolCallID(ctx, id) context.Context
func GetToolCallID(ctx) (string, bool)
func WithParentBlockID(ctx, id) context.Context
func GetParentBlockID(ctx) (string, bool)
func WithAgentState(ctx, *AgentState) context.Context
func GetAgentState(ctx) (*AgentState, bool)
func WithSubagentRunID(ctx, id) context.Context
func WithSubagentDepth(ctx, depth) context.Context
func GetSubagentDepth(ctx) int
```

`chat/runner.go::processTask` 在 agent 循环开始注入 ConversationID + AgentState + emitter + msgID；`loop/tools.go::runOneTool` 在 Execute 前进一步注入 ParentBlockID（= tool_call ID）；tool 内部读 ctx 用对应 emit。

**Phase 5 新增**：runner 还在 ctx 注入每对话独立的 `*agentstatepkg.AgentState`（`reqctxpkg.WithAgentState`），由 filesystem (Read/Write/Edit) 读 SeenFiles 做 must-Read-first 守卫，由 shell (Bash) 读 Cwd 做 cd 状态机；queue idle 时与 conversation 一起 GC。详 [`task.md §10`](task.md) 与 `pkg/agentstate/agentstate.go` 包 doc。

### 4.4 System Tools 完整目录

详细家族表见 §2.3。当前在线 21 个 system tools；Phase 4 起继续扩展（workflow/knowledge/mcp）。**Subagent** 工具（`Spawn` 隔离 sub-runner）见 [`subagent.md`](./subagent.md) 详设计。

---

## 5. Pipeline 架构（`app/chat/` + `app/loop/`）

### 5.1 文件结构

ReAct 引擎 2026-05 拆为两个包：
- **`app/loop/`** — 通用 ReAct 引擎（chat / subagent / Skill fork / Phase 4 workflow LLM 节点共享）
- **`app/chat/`** — chat 特有的队列、附件、autoTitle、system prompt

```
app/loop/
  loop.go      ← Host 接口 + Result + Run（ReAct 主循环）
  stream.go    ← streamLLM（iter.Seq）+ 实时 emit text/reasoning/tool_call block_start/delta/stop
  tools.go     ← runTools（按 execution_group 分批）+ runOneTool（注入 ParentBlockID + emit tool_result）+ partitionByExecutionGroup
  history.go   ← extendHistory + BlocksToAssistantLLM（DB 加载与循环内拼装共用）+ ExtractTextContent

app/chat/
  chat.go      ← 公开 API（Send / Cancel / ListMessages / UploadAttachment）+ Service struct + queue 类型 + emitUserMessage
  runner.go    ← getOrCreateQueue / runQueue / processTask（→ loop.Run）/ emitFatalError / buildSystemPrompt / autoTitle
  host.go      ← chatHost 实现 loop.Host：LoadHistory / Tools / WriteFinalize（终态写库 + emit message_stop）+ buildMessage + mapEventLogStatus
  history.go   ← buildHistory（从 DB 加载） + buildUserLLMMessage（含 @-mention 块渲染）+ attachmentToPart
  mention.go   ← renderMentionsXML（@ 引用 → <mention> 块；Send 解析存 Attrs["mentions"]，详见 mention.md）
  util.go      ← ID 生成器（newMsgID / newBlockID / newAttachmentID）+ readAndEncode + truncate
```

**chat 不再持有 stream.go / tools.go**——这些是循环内部细节，统一在 loop 包。chat 通过 `chatHost` 接 loop。

### 5.2 ReAct Loop（`loop.Run`）

**入口**：`chat/runner.go::processTask` 构造 `chatHost` → `loop.Run(ctx, host, client, baseReq, maxSteps=20, log)` → 返回 `loop.Result{Blocks, Status, StopReason, TokensIn, TokensOut, Steps, LastMessage}`。

```
loop.Run（伪码，详见 app/loop/loop.go）:
  history = host.LoadHistory(ctx)               // chat: buildHistory(convID, userMsgID)
  for step < maxSteps:
      tools = host.Tools(ctx)                    // 每步重算：activate_tools 可能已扩张集合
      req.Tools = ToLLMDefs(tools); byName = toolsByName(tools)
      aBlocks, toolCalls, sr, errMsg, in, out = streamLLM(ctx, client, req with history)
          // streamLLM 实时 emit 每个 LLM event：
          //   text/reasoning EventText/EventReasoning →
          //     首次 → em.EmitBlockStart(blk_, parent=msgID, msgID, BlockTypeText/Reasoning)
          //     之后 → em.DeltaBlock(blkID, delta)
          //   EventToolStart → em.EmitBlockStart(tc.ID, parent=msgID, msgID, BlockTypeToolCall, attrs={tool:name})
          //   EventToolDelta → em.DeltaBlock(tc.ID, argsDelta)
          //   流结束/切换 → em.StopBlock(blkID, status=completed/error/cancelled)
      allBlocks += aBlocks
      totalIn += in; totalOut += out
      if cancelled / error → host.WriteFinalize(error/cancelled, ...) → break
      if len(toolCalls) == 0 → host.WriteFinalize(completed, ...) → break  // 最终答案
      rBlocks = runTools(ctx, toolCalls, byName, log)
          // partition by execution_group → 同 group 并行、不同 group 串行升序
          // 每个 tool: runOneTool 在 ctx 挂 WithParentBlockID(tc.ID)
          // → ValidateInput → CheckPermissions → Execute
          // → em.EmitBlockStart(blk_result, parent=tc.ID, msgID, BlockTypeToolResult)
          //   em.DeltaBlock(blk_result, output)
          //   em.StopBlock(blk_result, status, errStr)
      allBlocks += rBlocks
      history, err = extendHistory(log, history, aBlocks, rBlocks)
      if err → host.WriteFinalize(error/HISTORY_EXTEND_FAILED) → break

  if !finalWritten → host.WriteFinalize(completed, max_tokens, ...)
  return Result{...}
```

**关键设计**：
- **block 实时 emit**：每个 LLM event（text token / reasoning token / tool call start / tool args delta）就推一次 block_start/delta/stop——前端逐字渲染，无 60fps 节流，无 entity-snapshot。Block 同步落 `message_blocks` 表（emitter 内部 dual-write）。
- **chatHost.WriteFinalize 是终态唯一写入点**：成功 / 失败 / 取消 / max_steps 都通过它落 `messages` 行 + emit message_stop。Block 在循环里已实时落库，WriteFinalize 不再处理 blocks 参数（仅保留满足 loop.Host 接口）。
- **fatal 写盘走 detached context**：`chatHost.WriteFinalize` 内部 `saveCtx = reqctxpkg.SetUserID(context.Background(), uid)` 再 `WithConversationID`，保证用户 cancel 不阻断终态落库 + message_stop。
- **`Result.Status` hardcode 为 Completed**：loop 不区分细分终态，只通过 `StopReason` 让调用方判断；subagent 在 spawn.go 重新映射成 `StatusMaxTurns / StatusFailed` 等独立桶。

### 5.3 streamLLM（`app/loop/stream.go`）

每个 LLM stream event 直接 emit 一次（没有快照路径，没有节流），消费 `iter.Seq[StreamEvent]`：

```go
em := eventlogpkg.From(ctx)
msgID, _ := reqctxpkg.GetMessageID(ctx)
var textBlockID, reasonBlockID string
toolBlockIDs := map[int]string{}

for ev := range client.Stream(ctx, req) {
    switch ev.Type {
    case EventText:
        if textBlockID == "" {
            textBlockID = idgenpkg.New("blk")
            em.EmitBlockStart(ctx, textBlockID, msgID, msgID, BlockTypeText, nil)
        }
        em.DeltaBlock(ctx, textBlockID, ev.Delta)
    case EventReasoning: // 同上，BlockTypeReasoning
    case EventToolStart:
        // 复用 LLM 自带的 tool-call ID（tc_xxx）作 block ID（§S21 例外）
        toolBlockIDs[ev.ToolIndex] = ev.ToolID
        em.EmitBlockStart(ctx, ev.ToolID, msgID, msgID, BlockTypeToolCall,
            map[string]any{"tool": ev.ToolName})
    case EventToolDelta:
        em.DeltaBlock(ctx, toolBlockIDs[ev.ToolIndex], ev.ArgsDelta)
    case EventFinish:
        // 累计 token
    case EventError:
        // ctx.Err != nil → cancelled；否则 error + errMsg
    }
}
// 流结束 / 切换：所有 open block 关闭（status=completed / cancelled / error）
```

`assembleBlocks` 把累积的 buffer 转为内存 `chatdomain.Block` slice 给 `extendHistory` 用（**仅作内存中转**——已经实时 emit 过了，事件已到前端，DB 已写过；这一步只是为了 history 回拼成 LLM wire）。

`extractToolCalls` 从 blocks 抽 `ToolCallData` 列表给 runTools。`parseToolArgs` 通过 `toolapp.StripStandardFields` 剥三个标准字段 + JSON 损坏兜底 `args["raw"]`。

### 5.4 Tool Call 分批执行（`app/loop/tools.go`）

按 LLM 自报的 `execution_group` 字段分批（详 CLAUDE.md §S18）：
- 同 group 号的 calls = 并行 batch（LLM 担保它们之间无依赖、无共享可变状态）
- 不同 group 号 = 升序串行（前 group 全跑完才进下 group）
- 缺省 / ≤0 = 自动分配唯一 group ≥1000，每个独自串行 batch，**排在所有显式 group 之后**——fail-safe 默认（LLM 不主动声明并行就不并行）

```go
func runTools(ctx, calls, byName, log) []chatdomain.Block {
    batches := partitionByExecutionGroup(calls)
    blocks := make([]chatdomain.Block, len(calls))
    var mu sync.Mutex
    for _, b := range batches {
        if len(b.items) > 1 {
            var wg sync.WaitGroup
            for _, item := range b.items {
                wg.Add(1)
                go func(it indexedCall) {
                    defer wg.Done()
                    blk := runOneTool(ctx, byName[it.tc.Name], it.tc, it.idx, log)
                    mu.Lock(); blocks[it.idx] = blk; mu.Unlock()
                }(item)
            }
            wg.Wait()
        } else {
            blk := runOneTool(ctx, byName[b.items[0].tc.Name], b.items[0].tc, b.items[0].idx, log)
            mu.Lock(); blocks[b.items[0].idx] = blk; mu.Unlock()
        }
    }
    return blocks
}
```

**例**：LLM 同 turn 发 `[A:1, B:1, C:0, D:2, E:0]`
→ 自动分配后 `[A:1, B:1, C:1000, D:2, E:1001]`（maxExplicit=2，autoBase=max(3, 1000)=1000）
→ 排序 `[1, 2, 1000, 1001]`
→ 4 个 batches: `[A, B 并行] [D 单跑] [C 单跑] [E 单跑]`

`runOneTool` 在调 `Execute` 前在 ctx 挂 `WithParentBlockID(tc.ID)` 让 tool 内部 emit 自动挂在 tool_call block 下。**`ValidateInput → CheckPermissions(default) → Execute`** 三步钩子链：Validate 失败 / Permission Deny 直接转失败 tool_result（不进 Execute）。每个 tool 跑完 emit 一次 `tool_result` block_start + delta(output) + block_stop，无快照推送。

### 5.5 终态写入与取消安全（`chatHost.WriteFinalize`）

chat 通过 `app/chat/host.go::chatHost` 实现 `loop.Host` 接口。WriteFinalize 是 loop 在循环结束（成功 / 错误 / 取消 / 步数到顶）时调用的唯一终态钩子：

```go
func (h *chatHost) WriteFinalize(ctx, blocks, status, stopReason, errCode, errMsg, in, out) {
    // Detached ctx：cancelled stream 不能阻止终态写或 message_stop emit
    saveCtx := reqctxpkg.SetUserID(context.Background(), h.uid)
    saveCtx = reqctxpkg.WithConversationID(saveCtx, h.convID)

    msg := buildMessage(h.msgID, h.convID, h.uid, status, stopReason, errCode, errMsg, in, out)
    if err := h.svc.repo.SaveMessage(saveCtx, msg); err != nil {
        h.svc.log.Error("CRITICAL: final assistant message persist failed — message lost", ...)
        // 覆盖 status 为 error 给后续 message_stop
        msg.Status = StatusError; msg.StopReason = StopReasonError
        msg.ErrorCode = "INTERNAL_ERROR"; msg.ErrorMessage = "failed to save assistant message to database"
    }
    h.svc.emitter.StopMessage(saveCtx, h.msgID, mapEventLogStatus(msg.Status),
        msg.StopReason, msg.ErrorCode, msg.ErrorMessage,
        msg.InputTokens, msg.OutputTokens)
    _ = blocks // unused — blocks 已经经 emit 实时落 message_blocks
}
```

`emitFatalError`（`runner.go`）走类似形态——给 LLM 调用前的失败（model 未配置 / key 解析失败）产生一条 stub assistant message 并 emit message_stop，让前端能正确收尾。

取消流程：`Cancel()` → ctx cancelled → `streamLLM` break → `loop.Run` 走 cancelled 分支 → `chatHost.WriteFinalize(StatusCancelled, ...)` → 终态必然落库 + message_stop 必然推达。

---

## 6. 消息存储（Message + Block 模型）

**事件日志协议统一后（2026-05-08）**，messages 与 message_blocks 表的 schema 与递归事件协议 1:1 对齐——每个 emit (block_start / block_delta / block_stop) 写或更新一行。

### 6.1 messages 表

```go
type Message struct {
    ID             string         `gorm:"primaryKey;type:text" json:"id"`
    ConversationID string         `gorm:"not null;index;type:text" json:"conversationId"`
    UserID         string         `gorm:"not null;type:text" json:"-"`
    ParentBlockID  string         `gorm:"type:text;index" json:"parentBlockId,omitempty"` // 嵌套消息（subagent sub-run）才填
    Role           string         `gorm:"not null;type:text" json:"role"`            // user | assistant
    Status         string         `gorm:"not null;type:text;default:'completed'" json:"status"`
    StopReason     string         `gorm:"type:text;default:''" json:"stopReason,omitempty"`
    ErrorCode      string         `gorm:"type:text;default:''" json:"errorCode,omitempty"`
    ErrorMessage   string         `gorm:"type:text;default:''" json:"errorMessage,omitempty"`
    InputTokens    int            `gorm:"default:0" json:"inputTokens,omitempty"`
    OutputTokens   int            `gorm:"default:0" json:"outputTokens,omitempty"`
    Attrs          map[string]any `gorm:"type:text;serializer:json" json:"attrs,omitempty"` // 自由稀疏 map(GORM serializer 透明序列化为 TEXT)
    CreatedAt      time.Time      `json:"createdAt"`
    UpdatedAt      time.Time      `json:"updatedAt"`
    DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`

    Blocks         []Block        `gorm:"-" json:"blocks"` // store 层 attachBlocks 填，不存这列
}
```

**字段语义**：
- `ParentBlockID`：仅当本消息嵌套在另一消息的某 block 下时设——subagent sub-run 用（指向父对话消息中 type=message 的 placeholder block）。顶层 user / assistant 消息为空。
- `Attrs`：自由 JSON map，常见键：
  - `attachments: [{attachmentId, fileName, mimeType}]`（user message 的附件引用）
  - `kind: "subagent_run", type, runId, maxTurns`（subagent sub-run 元数据）

**Role 值**：`user` | `assistant`（`tool` 角色已移除——tool result 是 assistant 消息内的 block）。

**Status 常量**：`pending` | `streaming` | `completed` | `error` | `cancelled`

**StopReason**：`end_turn` | `max_tokens` | `cancelled` | `error`

### 6.2 message_blocks 表（事件日志协议事实源）

```go
type Block struct {
    ID             string    `gorm:"primaryKey;type:text" json:"id"`
    ConversationID string    `gorm:"not null;type:text;uniqueIndex:idx_blocks_conv_seq,priority:1" json:"conversationId"`
    MessageID      string    `gorm:"not null;type:text;index" json:"messageId"`
    ParentBlockID  string    `gorm:"type:text;index" json:"parentBlockId,omitempty"`
    Seq            int64     `gorm:"not null;uniqueIndex:idx_blocks_conv_seq,priority:2" json:"seq"`
    Type           string    `gorm:"not null;type:text;check:type IN ('text','reasoning','tool_call','tool_result','progress','message')" json:"type"`
    Attrs          map[string]any `gorm:"type:text;serializer:json" json:"attrs,omitempty"` // 元数据 map(GORM serializer 透明 ↔ TEXT)
    Content        string    `gorm:"not null;type:text;default:''" json:"content"` // 累积内容，AppendDelta 走 SQL `content || ?` 原子拼
    Status         string    `gorm:"not null;type:text;check:status IN ('streaming','completed','error','cancelled')" json:"status"`
    Error          string    `gorm:"type:text" json:"error,omitempty"`           // block_stop 时填
    CreatedAt      time.Time `json:"createdAt"`
    UpdatedAt      time.Time `json:"updatedAt"`
}
```

**索引（GORM tag 声明）**：
- `idx_blocks_conv_seq` UNIQUE `(conversation_id, seq)` — §S21 单调 seq 强制
- `idx_blocks_message_id` `(message_id)` — 按 message 拉 blocks
- `idx_blocks_parent_block_id` `(parent_block_id)` — 递归子 block 查询

**CHECK 约束（GORM tag 声明）**：
- `type IN ('text','reasoning','tool_call','tool_result','progress','message')` — 6 类穷举
- `status IN ('streaming','completed','error','cancelled')` — 4 终态枚举

**Attrs 字段语义（2026-05 #4 修）**：DB 列形态是 `TEXT`，但 Go 侧用 `map[string]any` + `gorm:"serializer:json"`——GORM 自动 marshal/unmarshal。**对外 wire 形态**：JSON 响应里 `attrs` 是 object（如 `{"toolName":"Read","executionGroup":1}`），**不再是 JSON-encoded string**。修前 store 自己 `json.Marshal` 成 string 存进 string 列，handler/SSE 透传那个 string，前端拿到的是 `"attrs":"{...}"` 字符串——每个消费者都得多解一次。改 serializer 后 service / store / SSE / handler 全用 map 直接传，前端 `attrs.toolName` 即可访问。

**Block 类型与内容形态**（与 [`event-log-protocol.md` §2](../event-log-protocol.md) 1:1）：

| Type | Content 形态 | Attrs 字段 | 子 block？ |
|---|---|---|---|
| `text` | 裸文本（无 JSON 包装），流式 append | — | ❌ |
| `reasoning` | 同上（DeepSeek-R1 等的 thinking） | — | ❌ |
| `tool_call` | LLM tool args JSON 字符串，流式 append | `{tool: <name>}` | ✅（子 block：progress / nested LLM 文本 / tool_result） |
| `tool_result` | result 字符串 | — | ❌ |
| `progress` | 流式进度文字 | `{stage?: <free text>}` | ❌ |
| `message` | （无内容） | `{messageId: <nested msg ID>, ...}` | ✅（递归到下层 message） |

**ID 规则**：
- `text` / `reasoning` / `tool_result` / `progress` / `message` blocks → `blk_<16hex>`（pkg/idgen.New("blk")）
- `tool_call` blocks → 复用 LLM 自带的 tool-call ID（如 `tc_xxx`）作主键，不走 §S15 prefix（详 §S21 例外）。这让 tool_result.parent_block_id 与对应 tool_call.ID 直接配对，无需查表。

### 6.3 ReplayEnvelope（HTTP refetch wire shape）

`GET /api/v1/conversations/{id}/eventlog?from=N` 走 HTTP 而非 SSE，wire shape 用 `chatdomain.ReplayEnvelope`：

```go
type ReplayEnvelope struct {
    Type    string         `json:"type"`     // "block_start" | "block_delta" | "block_stop" | ...
    Seq     int64          `json:"seq"`
    Payload map[string]any `json:"payload"`  // 事件 payload（不重 type/seq）
}
```

`Repository.ReplayEventsAfter(ctx, conversationID, fromSeq)` 把 `seq > fromSeq` 的 blocks 重构成 3 envelope（`block_start` + `block_delta`(content) + `block_stop`）流，共享行 seq——前端 SSE 与 HTTP refetch 拿到 wire 形状完全一致。

### 6.4 Repository 接口（12 方法）

```go
type Repository interface {
    // ── Message ──
    SaveMessage(ctx, *Message) error                                    // upsert 行（不写 block）
    GetMessage(ctx, id) (*Message, error)                               // 含 Blocks，按 ctx 用户过滤；缺失返 ErrMessageNotFound
    ListMessagesByConversation(ctx, convID, ListFilter) ([]*Message, nextCursor, error)

    // ── Block ──
    SaveBlock(ctx, *Block) error                                        // upsert 行，emitter 在 block_start + block_stop 时调
    AppendDelta(ctx, blockID, delta) error                              // SQL `content || ?` 原子拼；行不存在返 ErrBlockNotFound
    FinalizeStop(ctx, blockID, status, errStr) error                    // 设终态；行不存在返 ErrBlockNotFound
    GetBlock(ctx, blockID) (*Block, error)                              // 缺失返 ErrBlockNotFound
    ListBlocksByConversation(ctx, convID) ([]*Block, error)             // 按 seq ASC
    ListBlocksByMessage(ctx, messageID) ([]*Block, error)               // 按 seq ASC
    ReplayEventsAfter(ctx, convID, fromSeq) ([]ReplayEnvelope, error)   // 历史回放给 HTTP refetch

    // ── Attachment ──
    SaveAttachment(ctx, *Attachment) error
    GetAttachment(ctx, id) (*Attachment, error)
}
```

**用户过滤**：Message + Attachment 方法按 ctx 用户过滤；Block 方法不过滤（auth 在父 Message 上——block 写由 emitter server-side 可信）。

**ON CONFLICT 保护**：`SaveMessage` 用显式 ON CONFLICT upsert，`DoUpdates` 列表为 `[status, stop_reason, error_code, error_message, input_tokens, output_tokens, attrs, updated_at]`——`created_at` 不在更新列里，保证首次 INSERT 的时间戳不被 status 更新覆盖（避免 GORM `Save()` 把零值写回 DB）。

### 6.5 attachments 表（Phase 5 重命名 + 加软删）

```go
type Attachment struct {
    ID          string         `gorm:"primaryKey;type:text" json:"id"`       // att_<16hex>
    UserID      string         `gorm:"not null;index;type:text" json:"-"`
    FileName    string         `gorm:"not null;type:text" json:"fileName"`
    MimeType    string         `gorm:"not null;type:text" json:"mimeType"`
    SizeBytes   int64          `gorm:"not null" json:"sizeBytes"`
    StoragePath string         `gorm:"not null;type:text" json:"-"`
    CreatedAt   time.Time      `json:"createdAt"`
    UpdatedAt   time.Time      `json:"updatedAt"`
    DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}
```

文件存 `{dataDir}/attachments/{att_id}/original.{ext}`，50MB 限制（`MaxAttachmentBytes`）。
**Phase 5 重命名**：表名从 `chat_attachments` 改为 `attachments`，并加 `UpdatedAt` + `DeletedAt`（软删）。理由：用户删附件后旧对话的 `Message.Attrs.attachments` 仍持引用，软删保留行让解引用不变 dangling。

### 6.6 历史重建（`app/chat/history.go`）

#### buildHistory

```go
func (s *Service) buildHistory(ctx, convID, currentUserMsgID string) ([]llminfra.LLMMessage, error)
```

扫描所有非 streaming/pending 消息，跳过 `currentUserMsgID`，末尾显式追加当前用户消息。

**为什么要追加末尾**：同一对话快速连发两条消息时，第二条 user 消息的 `created_at` 可能早于第一条 assistant 回复（队列中并发写入），导致历史排序错乱。追加保证 LLM 看到的最末是当前 user。

历史 message → LLM wire 转换走 `loopapp.BlocksToAssistantLLM(log, blocks)`（loop 包导出），与循环内部的 `extendHistory` 共用同一个转换器——blocks → LLM wire 形状只有一个事实源。

#### buildUserLLMMessage

读 `Message.Attrs` JSON 的 `attachments` 数组解析附件引用（**不**从 Block 读——Phase 5 起附件不再产生 attachment_ref block）：

```go
parts := [content text part]
for each attachment in msg.Attrs.attachments:
    if isImage(mime):
        parts += {type:"image_url", imageURL:"data:<mime>;base64,..."}
    else if extracted, _ := chatinfra.Extract(storagePath, mimeType); extracted != "":
        parts += {type:"text", text:"[附件: <fileName>]\n<extracted>"}
    else:
        log.Warn 跳过
msg.Parts = parts  // 多 parts 时；单 text 走 msg.Content
```

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

附件从 `Message.Attrs.attachments` 解析（不是 block）：

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

## 8. SSE 事件（事件日志协议）

chat domain 通过 **事件日志协议** 推流——5 events × 6 block types 的递归 SSE 流。完整设计 → [`../event-log-protocol.md`](../event-log-protocol.md)；契约表格 → [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md)。本节只讲 chat 作为 producer 的视角。

### 8.1 双 SSE 协议

后端只有两个 SSE 流（CLAUDE.md §E1）：

| 协议 | 路径 | 用途 |
|---|---|---|
| **eventlog**（per-conversation）| `GET /api/v1/eventlog?conversationId=<id>` | chat 流式内容（5 events × 6 block types）|
| **notifications**（global broadcast）| `GET /api/v1/notifications` | entity 状态更新（autoTitle / todo / 等开放词表）|

chat 的 producer 出口分别经：
- `s.emitter` (`eventlogpkg.Emitter`) → eventlog 流（对话内的所有 token / tool / sub-run）
- `s.notifications` (`notificationspkg.Publisher`) → notifications 流（autoTitle 完成时推 `conversation` entity snapshot）

### 8.2 5 个事件类型 + 6 个 block 类型

详见 [`events-design.md` §1-§3](../service-contract-documents/events-design.md)：

| 事件 | 用途 | DB 写入 |
|---|---|---|
| `message_start` | 开新 message（user / assistant / subagent）| ✅ → `messages` 行（终态时 SaveMessage）|
| `message_stop` | 关 message（终态）| ✅ → 同上 |
| `block_start` | 开新 block | ✅ → `message_blocks` SaveBlock |
| `block_delta` | append 内容到 block | ✅ → AppendDelta 原子 SQL `content || ?` |
| `block_stop` | 关 block | ✅ → FinalizeStop |

6 block 类型穷举：`text` / `reasoning` / `tool_call` / `tool_result` / `progress` / `message`（详 §6.2）。

### 8.3 chat producer 视角

```
HTTP Send → Service.Send 同步部分:
  1. 落 user message（含 text block，attrs.attachments）
  2. emitUserMessage(ctx, userMsg)：
     - EmitMessageStart(msgID, "user", "", nil)
     - 每个 block: EmitBlockStart + DeltaBlock(content) + StopBlock(completed)
     - StopMessage(msgID, completed, "", "", "", 0, 0)
     一次性 burst，user message 不走流式
  3. 入队 → 立刻返 202

worker → processTask → loop.Run → chatHost:
  4. 预分配 assistant msgID + EmitMessageStart(msgID, "assistant", "", nil)  // 打开 assistant 槽
  5. loop.Run 内 streamLLM 实时 emit：
     - text/reasoning：首次 EmitBlockStart(blk_, parent=msgID, msgID, BlockType*)，之后 DeltaBlock(blk_, delta)
     - EventToolStart：EmitBlockStart(tc.ID, parent=msgID, msgID, BlockTypeToolCall, attrs={tool:name})
     - EventToolDelta：DeltaBlock(tc.ID, argsDelta)
     - 流结束 / 切换：StopBlock(blkID, status)
  6. runOneTool 内 ctx 已挂 WithParentBlockID(tc.ID)：
     - tool 内可调 eventlogpkg.From(ctx) 推 progress block 等子 block（子 block parentId = tc.ID）
     - 返后 runOneTool: EmitBlockStart(blk_result, parent=tc.ID, msgID, BlockTypeToolResult)
                     + DeltaBlock(blk_result, output)
                     + StopBlock(blk_result, status, errStr)
  7. chatHost.WriteFinalize：终态 SaveMessage + StopMessage(msgID, mappedStatus, ...)

autoTitle：异步 goroutine 起 → 完成时 s.notifications.Publish(titleCtx, "conversation", conv.ID, conv)
（不走 eventlog；走 notifications 流，前端按 type=conversation 路由）

pre-LLM 失败（resolve 失败）：
  emitFatalError → SaveMessage(stub, status=error) + StopMessage(msgID, error, code, msg, 0, 0)
```

### 8.4 §S21 invariants 实现要点（chat 视角）

- **`block_start.parentId` 必须是先于本事件出现过的 ID**：chat 每次 EmitBlockStart 的 parent 都是同 conversation 内已 emit 过的 message ID（顶层 block）或 block ID（嵌套）。runOneTool 在 tool 内部调用前挂 `WithParentBlockID(tc.ID)` 让 tool emit 自动选对父。
- **Block status 单向 streaming → terminal**：StopBlock 后不再 emit 任何同 ID 事件；emitter 内部不允许重复 stop。
- **Per-conversation seq 严格全局单调**：`message_blocks` 的 `idx_blocks_conv_seq` UNIQUE 强制；emitter 在 producer 端原子分配 seq。
- **同 block 的 deltas 按 seq 顺序 append-only**：DB 走 SQL `content || ?` 原子拼；前端永不重写。
- **tool_call block 的 ID 复用 LLM 自带的 `tc_xxx`**：streamLLM 在 EventToolStart 时 `EmitBlockStart(event.ToolID, ...)` 传 LLM 的 ID。这是 §S21 例外：LLM 不知道我们的 ID 体系，复用让 tool_result.parent_block_id 与 tool_call.ID 直接配对。

### 8.5 历史回放与重连

- **正常重连**：客户端断线后 `Last-Event-ID: <seq>` header 重连，Bridge 从 buffer 内 `seq+1` 起 replay。
- **超 buffer**：返 `410 Gone` + `code=SEQ_TOO_OLD`，客户端经 `GET /api/v1/conversations/{id}/eventlog?from=<seq>` 经 `Repository.ReplayEventsAfter` 重构事件流（见 §6.3 ReplayEnvelope）。前端 renderer 不区分 SSE 实时 vs HTTP refetch——同一 wire 形状。

---

## 9. HTTP API

### 9.1 端点

| Method | Path | 用途 | 状态码 |
|---|---|---|---|
| `POST` | `/api/v1/attachments` | 上传附件（multipart）| 201 |
| `POST` | `/api/v1/conversations/{id}/messages` | 发送消息，触发 Agent | 202 |
| `DELETE` | `/api/v1/conversations/{id}/stream` | 取消正在运行的 Agent | 204 |
| `GET` | `/api/v1/conversations/{id}/messages` | 消息历史（cursor 分页，含 blocks）| 200 |

`handlers/chat.go` 注册以上 4 个路由。SSE 端点（`/api/v1/eventlog` / `/api/v1/conversations/{id}/eventlog` / `/api/v1/notifications`）在 `handlers/eventlog.go` + `handlers/notifications.go` 各自注册——非 chat handler 责任。

### 9.2 GET /conversations/{id}/messages 响应格式

```json
{
  "data": [
    {
      "id": "msg_xxx", "role": "user", "status": "completed",
      "attrs": "{\"attachments\":[{\"attachmentId\":\"att_xxx\",\"fileName\":\"...\",\"mimeType\":\"...\"}]}",
      "createdAt": "...",
      "blocks": [
        {"id":"blk_1","conversationId":"cv_xxx","messageId":"msg_xxx","seq":1,"type":"text","content":"帮我...","status":"completed","createdAt":"..."}
      ]
    },
    {
      "id": "msg_yyy", "role": "assistant", "status": "completed",
      "stopReason": "end_turn", "inputTokens": 1024, "outputTokens": 256,
      "createdAt": "...",
      "blocks": [
        {"id":"blk_2","seq":2,"type":"reasoning","content":"思考...","status":"completed",...},
        {"id":"tc_abc","seq":3,"type":"tool_call","attrs":"{\"tool\":\"datetime\"}","content":"{...args...}","status":"completed",...},
        {"id":"blk_3","parentBlockId":"tc_abc","seq":4,"type":"tool_result","content":"...","status":"completed",...},
        {"id":"blk_4","seq":5,"type":"text","content":"当前时间是…","status":"completed",...}
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

**错误**：404 `CONVERSATION_NOT_FOUND` / 409 `STREAM_IN_PROGRESS` / pre-LLM 失败（API_KEY_PROVIDER_NOT_FOUND / MODEL_NOT_CONFIGURED / LLM_PROVIDER_ERROR）经 `emitFatalError` 推 stub message + emit message_stop。

---

## 10. Service 设计

### 10.1 Struct

```go
// app/chat/chat.go
type Service struct {
    repo          chatdomain.Repository      // messages + blocks + attachments
    convRepo      convdomain.Repository      // 对话 CRUD
    modelPicker   modeldomain.ModelPicker    // 拿 (provider, modelID)
    keyProvider   apikeydomain.KeyProvider   // 拿 (key, baseURL)
    llmFactory    *llminfra.Factory          // 自有 LLM 流式客户端工厂
    toolset       toolapp.Toolset            // Resident + Lazy 组（SetToolset 注入）；host.Tools(ctx) 按激活组返
    emitter       eventlogpkg.Emitter        // event-log emit (chat / block 生命周期)
    notifications notificationspkg.Publisher // global notifications (autoTitle 等 entity 更新)
    dataDir       string                     // 附件存储根目录
    log           *zap.Logger
    queues        sync.Map                   // conversationID → *convQueue
    catalog       catalogdomain.SystemPromptProvider // 可选；SetSystemPromptProvider 注入
}
```

ctor `NewService(repo, convRepo, modelPicker, keyProvider, llmFactory, emitter, notifications, dataDir, log)`：
- `log==nil` 立即 panic
- `emitter` / `notifications` 接 nil 时回退到 `From(context.Background())` 的 no-op 实现（单测便利）
- `dataDir==""` 兜底 `filepath.Join(os.TempDir(), "forgify")`

后置注入：
- `SetToolset(ts toolapp.Toolset)` — 全局 tool 列表建好后拆分注入（Resident + Lazy 组）；`host.Tools(ctx)` 返 Resident + ctx 上 `ActivatedGroups()` 对应的 Lazy 组（activate_tools 按需加载）
- `SetSystemPromptProvider(p catalogdomain.SystemPromptProvider)` — Capability Catalog（避免 catalog import chat 的循环依赖）

### 10.2 Send 流程

```
HTTP 入口（同步部分）:
  1. convRepo.Get(conversationID)            → 验证对话存在
  2. 解析附件 → attrs.attachments JSON       → Marshal 失败仅 log 不阻断
  3. 建 user Message（含单 text block 或仅附件场景空 blocks）
  4. repo.SaveMessage(userMsg)               → DB（user message + blocks 一次写）
  5. emitUserMessage(emitCtx, userMsg)       → 一次性 burst：MessageStart → 每 block start/delta/stop → MessageStop
  6. 入队 queuedTask{userMsgID}（agentCtx detached + uid + locale）
  7. 立刻返回 202 { messageId: userMsgID }；队列满返 ErrStreamInProgress

worker goroutine（runner.go::processTask）:
  8. ctx 注入 ConversationID + AgentState（per-queue 共享）+ emitter + msgID
  9. EmitMessageStart(msgID, "assistant", "", nil)  // 开 assistant slot
 10. llmclientpkg.ResolveDialogueWithOverride(ctx, conv.ModelOverride, picker, keys, factory)
       → Bundle{Client, APIKeyID, Provider(从 key 派生), ModelID, Key, BaseURL}
       conv.ModelOverride 非 nil → 直接 finishResolve(override.APIKeyID, override.ModelID, ...)
       否则 → picker.PickForDialogue(ctx) → (apiKeyID, modelID) → finishResolve
       失败 → emitFatalError 推 stub error message + StopMessage（pre-LLM 失败映射：
          ErrPickModel → MODEL_NOT_CONFIGURED
          ErrResolveCreds → API_KEY_NOT_FOUND（按 id 解析失败；含跨用户隔离）
          其他 → LLM_PROVIDER_ERROR）
 10a. agentCtx = reqctxpkg.WithModelOverride(agentCtx, conv.ModelOverride)
        让 subagent spawn 工具读出 effective override 透传给 Spawn 的 parentModelOverride 参数
 11. baseReq{ModelID, Key, BaseURL, System}（loop.Run 内每步填 req.Tools = ToLLMDefs(host.Tools(ctx))）
 12. chatHost{svc, convID, uid, msgID, userMsgID}
 13. loop.Run(agentCtx, host, bc.Client, baseReq, maxSteps=20, log) → loop.Result

 14. 触发 auto-title goroutine（conv.Title 为空且 !AutoTitled）
        → s.notifications.Publish(titleCtx, "conversation", conv.ID, conv)
```

### 10.3 并发控制与取消

每个 conversationID 拥有一个 `convQueue`（buffered channel cap=5 + 单 worker goroutine + per-queue `*agentstatepkg.AgentState`），保证同对话消息按序逐条执行；不同对话间并行。worker 在 5 分钟空闲后自行退出，下次 Send 时按需重建。

```go
type convQueue struct {
    ch         chan queuedTask
    mu         sync.Mutex
    cancel     context.CancelFunc           // nil when idle
    agentState *agentstatepkg.AgentState    // 跨 task 共享（SeenFiles / Cwd）
}
```

- **Send**：队列满 → 409 `STREAM_IN_PROGRESS`；否则入队后立即 202 返回
- **Cancel**：`q.cancel()` → ctx cancelled → streamLLM break → loop.Run 走 cancelled 分支 → `chatHost.WriteFinalize(StatusCancelled, ...)` 推最终 message_stop（detached ctx 保证落库）+ 清空 channel 中尚未处理的 task

### 10.4 System Prompt 组装

每次调用 Agent 前，`SystemPromptSections(ctx, conv)`（`runner.go`）按 cache-friendly 顺序（静态前 / 动态后）返回以下命名段：

```
identity        — 身份一句话（代码写死，静态可缓存）
how_to_work     — 操作原则 7 条（复用优先/先验证/先核查/审慎/提问/精炼/并行；静态可缓存）
tools           — 工具模型 + 三个标准注入字段讲一次（静态可缓存）
capabilities    — 工具组索引 + catalog 资产菜单（半动态；catalog.GetForSystemPrompt 拼）
memory          — 长期记忆（动态）
documents       — @-mention 文档注入（动态）
user_system_prompt — conversation.system_prompt（可选，动态）
environment     — date + 回复语言（动态；替代原 locale_hint）
```

`capabilities` 段由 `buildCapabilitiesSection` 拼装：
1. **工具组索引**（仅 Lazy 非空时渲染）：列出各 category 名 + 工具数，提示 `call activate_tools(category)` 按需加载。
2. **catalog 资产菜单**：调 `catalog.GetForSystemPrompt(ctx)` 取 function/handler/workflow/skill/mcp/document 实体列表（`- name [invokeTool]: desc`，desc 截 48 字符）。

`activate_tools` 是 RESIDENT 工具，不需 activate 即可调用；调用后当轮及后续轮 `host.Tools(ctx)` 返回 Resident + 对应 Lazy 组。

`conversation.system_prompt` 字段存在 `conversations` 表（由 conversation domain 管理），chat.Service 通过 `convRepo.Get(id)` 读取。

### 10.5 自动命名（Auto-Titling）

第一轮对话完成后（assistant 消息 status=completed），异步起一个 goroutine 调轻量模型生成标题：

```
条件：conversation.title == "" AND conversation.auto_titled == false
  → llmclientpkg.ResolveUtility(ctx, picker, keys, factory)
       → utility scenario，不接 conv override（autoTitle 是工具内部活儿，用户主对话挑 Opus 不必让 autoTitle 也用 Opus 烧 token）
  → System: "Generate a short conversation title (5 words or fewer). Reply with ONLY the title, no punctuation."
  → Input: result.LastMessage（assistant 最后文字，truncate 300 字）
  → 写回 conversations.title + conversations.auto_titled = true
  → s.notifications.Publish(titleCtx, "conversation", conv.ID, conv)  // 全局通知所有 UI 窗口
```

**非阻塞**：标题生成失败静默忽略，不影响主流程。10s 超时硬限。

**Goroutine 竞态修复（2026-05-30）**：原 `go s.autoTitle(...)` 脱离 `WaitGroup`，DB 关闭后仍可能查 DB（`-race` 下偶发 `sql: database is closed`）。修复：`runQueue` goroutine 追踪进 `s.wg`（`sync.WaitGroup`），同时监听 `s.shutdown` channel；`chatService.Wait()` 关 shutdown channel + `wg.Wait()`，哈内斯 teardown 在 `t.Cleanup` 中 LIFO 顺序注册 `chatService.Wait()`，先于 DB close，保证 autoTitle goroutine 正常退出后 DB 才关。

---

## 11. 完整调用链（当前形态）

### 11.1 用户发消息

```
POST /api/v1/conversations/cv_xxx/messages  body={content, attachmentIds}
  → middleware 链
  → ChatHandler.SendMessage
      → convRepo.Get(conversationID)              → 验证对话存在
      → 解析附件 refs → attrs JSON
      → user Message{ID=msgID, Status=completed, Attrs=attrsJSON, Blocks=[text]}
      → repo.SaveMessage(userMsg)                 → DB
      → emitUserMessage(emitCtx, userMsg)         → burst 推 user message 全套事件
      → getOrCreateQueue(conversationID).ch <- queuedTask{userMsgID}
      → response 202 {messageId: userMsgID}

--- worker goroutine（runner.go::processTask）---
  → agentCtx 注入 ConversationID + AgentState + emitter + assistant msgID
  → emitter.EmitMessageStart(assistantMsgID, "assistant", "", nil)  // 打开 assistant slot
  → llmclient.ResolveDialogueWithOverride(ctx, conv.ModelOverride, picker, keys, factory) → Bundle{Client, APIKeyID, Provider, ModelID, Key, BaseURL}
      ↳ 失败映射 ErrPickModel → MODEL_NOT_CONFIGURED
                ErrResolveCreds → API_KEY_NOT_FOUND（按 id 解析 + 跨用户隔离）
                其他 → LLM_PROVIDER_ERROR
      ↳ emitFatalError(...) → SaveMessage(stub) + StopMessage(error, code, msg)
  → reqctxpkg.WithModelOverride(agentCtx, conv.ModelOverride)  // subagent spawn 走同一 override
  → baseReq{System=buildSystemPrompt(...), ...}
  → host = chatHost{svc, convID, uid, msgID, userMsgID}
  → result = loopapp.Run(agentCtx, host, bc.Client, baseReq, 20, log)
       host.LoadHistory  → buildHistory(convID, userMsgID) → [...LLMMessage]
       for step < 20:
         streamLLM(ctx, client, req)   // 每 token 实时 emit text/reasoning/tool_call block_start/delta/stop
         if cancelled/error → host.WriteFinalize(...) → break
         if no toolCalls   → host.WriteFinalize(completed, ...) → break
         runTools          // partition by execution_group；每 tool runOneTool 实时 emit tool_result block_start/delta/stop
         extendHistory     // BlocksToAssistantLLM(blocks)
       if !finalWritten → host.WriteFinalize(completed, max_tokens, ...)
  → if conv.Title=="" && !AutoTitled: go autoTitle(...)
       → notifications.Publish("conversation", conv.ID, conv)  // 触发全局 UI rename
```

### 11.2 前端收事件（事件日志协议）

```
GET /api/v1/eventlog?conversationId=cv_xxx
  → EventLogHandler.Stream
      → Bridge.Subscribe(conversationID, fromSeq)
      → 持续 write SSE wire（详 event-log-protocol.md §3）：
          event: message_start  data: {seq, id, conversationId, role, parentBlockId?, attrs?}
          event: block_start    data: {seq, id, conversationId, parentId, messageId, blockType, attrs?}
          event: block_delta    data: {seq, id, conversationId, delta}
          event: block_stop     data: {seq, id, conversationId, status, error?}
          event: message_stop   data: {seq, id, conversationId, status, stopReason?, errorCode?, errorMessage?, inputTokens?, outputTokens?}
      → 断线重连：Last-Event-ID: <seq> header → server replay buffer 内事件；超 buffer 返 410 + SEQ_TOO_OLD

GET /api/v1/notifications  // 全局通知（autoTitle / todo 等）
  → NotificationsHandler.Stream
      → 推 envelope: {type: "conversation", id, data, conversationId?}
      → 前端按 type 字符串过滤路由
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
- 长对话 context compaction：超长时压缩历史，保留关键消息——这是 Claude Code 调研 [`claude-code-research-documents/03-context.md`](../claude-code-research-documents/03-context.md) 的吸收。Phase 5 实施时再在 `app/loop/` 加钩子点（当前代码无占位）

---

## 13. 错误码

| Code | HTTP | Sentinel | 场景 |
|---|---|---|---|
| `MESSAGE_NOT_FOUND` | 404 | `chat.ErrMessageNotFound` | GetMessage 未命中 |
| `STREAM_NOT_FOUND` | 404 | `chat.ErrStreamNotFound` | 取消不存在的流 |
| `STREAM_IN_PROGRESS` | 409 | `chat.ErrStreamInProgress` | 同一对话队列已满 |
| `LLM_PROVIDER_ERROR` | 502 | `chat.ErrProviderUnavailable`（与 `llminfra.ErrProviderError` 共用同一 wire code）| 上游 LLM 故障（非 401）|
| `ATTACHMENT_TOO_LARGE` | 413 | `chat.ErrAttachmentTooLarge` | 附件超过 50MB |
| `ATTACHMENT_TYPE_UNSUPPORTED` | 415 | `chat.ErrAttachmentTypeUnsupported` | 无法处理的文件格式 |
| `ATTACHMENT_PARSE_FAILED` | 422 | `chat.ErrAttachmentParseFailed` | 文件损坏或解析失败 |
| `ATTACHMENT_NOT_FOUND` | 404 | `chat.ErrAttachmentNotFound` | attachmentIds 中包含未知 ID（或属于其他 user）|
| `EMPTY_CONTENT` | 400 | `chat.ErrEmptyContent` | content 为空/空白且无附件 — burn-in v2 加入避免无意义 LLM 调用 |
| `VISION_NOT_SUPPORTED` | 422 | `chat.ErrVisionNotSupported` | 当前 provider 不支持图片 |

**未到 handler 的 sentinel**：
- `chat.ErrBlockNotFound` — 内部 sentinel，由 emitter/AppendDelta/FinalizeStop/GetBlock 在 race / 协议错误时返；不到 handler，故不在 errmap。

**Message.errorCode 字段值**（`status="error"` 时填，前端从 message_stop 事件读）：`MODEL_NOT_CONFIGURED` / `API_KEY_PROVIDER_NOT_FOUND` / `LLM_PROVIDER_ERROR` / `LLM_STREAM_ERROR` / `HISTORY_EXTEND_FAILED` / `INTERNAL_ERROR`。详见 `error-codes.md` chat 域。

**401 路径**：LLM 流响应中遇 401 → streamLLM 返 stopReason=error + errMsg → loop.Run 走 error 分支 → `chatHost.WriteFinalize(status=error, errCode="LLM_STREAM_ERROR", errMsg=err.Error())`。前端从 message_stop 事件的 `errorCode` / `errorMessage` 读。

---

## 14. 为什么这样设计（关键决策总结）

| 决策 | 选择 | 理由 |
|---|---|---|
| 用 ReAct Agent 还是固定 Graph | **ReAct Agent** | 任务序列是运行时 LLM 决定的，不能提前写死；Phase 2-5 的工具列表动态增长 |
| tools 全部注入 vs Tool RAG | **System Tools 注入 + Tool RAG** | System Tools 数量固定（~21 个）可全注入；用户工具无上限，靠 search_forges 动态检索 |
| 202 + SSE vs 直接 stream response | **202 + 独立 SSE** | Agent 跑多步需要持久连接；POST 语义是"接受请求"不是"等待结果"；事件日志 Bridge 已就绪 |
| messages 存哪 | **chat domain 自己管** | 消息历史是 chat 专有数据，不应跨 domain 共享；conversation domain 只管线程元数据 |
| LLM 客户端用现成框架 vs 自实现 | **自实现 `infra/llm`** | 流式 SSE 解析 / tool call 累积 / 错误分类需要完全可见可控 |
| 不同 provider 协议适配 | **各自独立 client** | OpenAI 兼容 / Anthropic 原生协议差异（消息格式、tool result 包装、stream 边界），强行统一会失真 |
| ReAct 引擎拆 chat / loop | **2026-05 拆出 `app/loop`** | chat / subagent / Skill fork / Phase 4 workflow 节点共享同一 ReAct 实现；chat 仅持有"chat 特有"的部分 |
| SSE 协议形态 | **5 events × 6 block types 递归事件日志** | 替代旧 entity-snapshot 模型——单一 wire 描述任意嵌套（subagent / progress / tool 内 LLM 文字 / 等），前端纯 append 不覆盖。详 [`event-log-protocol.md`](../event-log-protocol.md) |
| 终态写 detached context | **`chatHost.WriteFinalize` 用 `context.Background()` + WithUserID + WithConversationID** | 用户取消时 ctx 已 cancelled，但终态消息必须落库 + message_stop 必须推达——detached ctx 让 SaveMessage + StopMessage 都不被上游 cancel 阻断 |
| autoTitle 走 notifications 而非 eventlog | **`notifications.Publish("conversation", ...)`** | autoTitle 是 entity 状态更新（不是对话内流式内容），应走全局 entity broadcast 流让所有打开的 UI 窗口同步 rename |
| tool_call block ID 复用 LLM ID | **直接用 `tc_xxx` 作主键** | LLM 不知道我们的 ID 体系；复用让 tool_result.parent_block_id 与对应 tool_call.ID 直接配对，无需查表（§S15 例外，§S21 documented） |

---

## 15. 实现清单 ✅

### infra/llm 层（自有 LLM 流式客户端）
- [x] `infra/llm/llm.go` — StreamEvent / LLMMessage / ToolDef / Client 接口 / Generate helper
- [x] `infra/llm/openai.go` — OpenAI-compat SSE 客户端（iter.Seq），覆盖 OpenAI/DeepSeek/Qwen/Moonshot/Ollama 等
- [x] `infra/llm/anthropic.go` — Anthropic 原生 `/v1/messages` 客户端（content_block_start/delta/stop）
- [x] `infra/llm/factory.go` — Factory.Build(Config) provider dispatch + resolveBaseURL

### app/tool 层（framework + 7 家族子包，§S12 例外允许嵌套）
- [x] `app/tool/tool.go` — Tool 接口（9 方法）+ PermissionMode/Result + injectStandardFields（注入 summary/destructive/execution_group）+ StripStandardFields + ToLLMDef/ToLLMDefs
- [x] `app/tool/forge/{forge,search,get,create,edit,run}.go` — 5 个 forge system tool + 共享工厂 + streamCode helper + resolveAttachments
- [x] `app/tool/filesystem/` — Read / Write / Edit；must-Read-first 守卫走 AgentState.SeenFiles；原子写 CreateTemp+Rename
- [x] `app/tool/search/` — Grep（rg + stdlib 双后端）+ Glob（doublestar + mtime 降序 + JSON enrichment）
- [x] `app/tool/web/` — WebFetch（Jina + 直 GET fallback）+ WebSearch（SearXNG/Bing/Bing CN 三层）
- [x] `app/tool/shell/` — Bash（前后台双模式 + cd 状态机）+ BashOutput + KillShell + ProcessManager
- [x] `app/tool/task/` — TaskCreate / TaskList / TaskGet / TaskUpdate
- [x] `app/tool/ask/ask.go` — AskUserQuestion；与 `app/ask` Service 配合（in-memory rendezvous）
- [x] `app/tool/subagent/agent.go` — Subagent system tool（详 [`subagent.md`](./subagent.md)）

### domain/chat 层
- [x] `domain/chat/chat.go` — Message（含 ParentBlockID + Attrs JSON + errorCode/errorMessage）+ Block 实体（ConversationID + ParentBlockID + Seq int64 + Type CHECK + Attrs JSON + Content + Status CHECK + Error + UpdatedAt）+ ToolCallData（含 Summary/Destructive/ExecutionGroup 一等字段）+ ToolResultData（含 ErrorMsg/ElapsedMs）+ AttachmentRef + Attachment（软删）+ ReplayEnvelope + 9 个 sentinel + Repository 接口（12 方法）

### infra/db 层
- [x] `infra/db/db.go` — modernc.org/sqlite 驱动；DSN 走 `_pragma=...` 语法
- [x] `infra/db/schema_extras.go` — 按 table 分组的 extraGroup 结构（partial 索引 / 触发器 / 等 GORM tag 表达不了的）

### infra/store/chat 层
- [x] `infra/store/chat/chat.go` — Store struct + 12 个 Repository 方法实现：SaveMessage（ON CONFLICT 保护 created_at）/ GetMessage（attachBlocks）/ ListMessagesByConversation（cursor 分页 + 批量取 blocks 避 N+1，按 seq ASC）/ SaveBlock / AppendDelta（SQL `content || ?`）/ FinalizeStop / GetBlock / ListBlocksByConversation / ListBlocksByMessage / ReplayEventsAfter（构造 ReplayEnvelope 流）/ SaveAttachment / GetAttachment

### infra/chat 层
- [x] `infra/chat/extractor.go` — Extract(storagePath, mimeType)：text/pdf/docx/xlsx/pptx/html 提取；IsImage 分派 Vision 路径

### app/loop 层（共享 ReAct 引擎）
- [x] `app/loop/loop.go` — Host 接口（LoadHistory / Tools / WriteFinalize 共 3 方法）+ Result struct（7 字段）+ Run（ReAct 主循环，maxSteps 上限）+ ExtractTextContent
- [x] `app/loop/stream.go` — streamLLM（iter.Seq 驱动）+ 实时 emit text/reasoning/tool_call block_start/delta/stop + 流结束所有 open block 关闭
- [x] `app/loop/tools.go` — runTools（按 execution_group 分批）+ runOneTool（注入 ParentBlockID + ValidateInput → CheckPermissions → Execute + emit tool_result）+ partitionByExecutionGroup
- [x] `app/loop/history.go` — extendHistory + BlocksToAssistantLLM（与 chat.buildHistory 共用）

### app/chat 层（5 文件）
- [x] `app/chat/chat.go` — Service struct + NewService + Send / Cancel / ListMessages / UploadAttachment + SetToolset（持 Resident + Lazy 组）+ SetSystemPromptProvider + emitUserMessage + queueCapacity + convQueue（含 agentState）/ queuedTask 类型
- [x] `app/chat/runner.go` — getOrCreateQueue / runQueue（5 分钟空闲 GC）/ processTask（→ loop.Run）/ emitFatalError / buildSystemPrompt（含 Capability Catalog 段）/ autoTitle（→ notifications.Publish）+ maxSteps 常量
- [x] `app/chat/host.go` — chatHost 实现 loop.Host：LoadHistory / Tools / WriteFinalize（detached saveCtx + SaveMessage + StopMessage）+ buildMessage + mapEventLogStatus（Warn fallback for unknown status）
- [x] `app/chat/history.go` — buildHistory(currentUserMsgID) + buildUserLLMMessage（读 Attrs.attachments）+ attachmentToPart
- [x] `app/chat/util.go` — newMsgID / newBlockID / newAttachmentID + readAndEncode + truncate

### transport 层
- [x] `handlers/chat.go` — 4 端点：POST attachments / POST messages / DELETE stream / GET messages
- [x] `handlers/eventlog.go` — GET /eventlog SSE + GET /conversations/{id}/eventlog refetch
- [x] `handlers/notifications.go` — GET /notifications SSE

### 配套
- [x] `errmap.go` — chat sentinel 映射（含 `MESSAGE_NOT_FOUND` / `STREAM_NOT_FOUND` / `STREAM_IN_PROGRESS` / `LLM_PROVIDER_ERROR` / 4 attachment / `VISION_NOT_SUPPORTED`）
- [x] `router/deps.go` — ChatService / EventlogBridge / NotificationsBridge 字段
- [x] `main.go` — chatRepo 共享变量；llmFactory；PathGuard；7 家族工厂装配链 ForgeTools → FilesystemTools → SearchTools → WebTools → NewShellTools（含 ProcessManager.Stop defer）→ TaskTools → AskTools → SubagentTools → buildToolset(tools) + 注入 activate_tools(RESIDENT) → chatService.SetToolset(ts)；Migrate messages + message_blocks + attachments + tasks
