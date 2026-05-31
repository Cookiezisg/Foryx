# 测试流水线迭代 1：harness 修复 + Fake LLM 注入 + 全场景覆盖

**日期**：2026-05-03
**状态**：🔄 设计中（未开工）
**阶段定位**：Phase 3 后优化轮的一项；非 Phase 4 阻塞前置
**关联**：
- 现状代码：[`backend/test/`](../../../../backend/test/)（5 文件、6 测试场景，目前编译失败）
- 沙箱迭代 1：[`../sandbox-iteration-documents/01-uv-bundled-python-per-forge-venv.md`](../sandbox-iteration-documents/01-uv-bundled-python-per-forge-venv.md)
- chat 详设：[`../../service-design-documents/chat.md`](../../service-design-documents/chat.md)
- forge 详设：[`../../service-design-documents/forge.md`](../../service-design-documents/forge.md)

---

## 0. 背景与目标

### 0.1 当前状态

`backend/test/` pipeline 测试套件只有 6 个场景（1 smoke + 5 chat），且**当前编译失败**——沙箱迭代 1（2026-05-03）改了 `sandboxinfra.New` 签名（`string` → `Config`），main.go 跟齐了但 harness.go 没跟齐：

```
test/harness.go:146:20: cannot use "python3" (untyped string constant)
                       as sandbox.Config value in argument to sandboxinfra.New
```

progress-record 自己也承认了（2026-05-03 [refactor] 重复实现摸排条）：
> 当前 build 状态：cmd/server/main.go + test/harness.go 因 sandbox iteration A in-flight ... 暂时无法整体编译——本轮 8 项改动与 sandbox 互不耦合，**待 sandbox iteration A 收尾后整体绿**。

后来沙箱迭代 1 落地了，main.go 修了，**harness.go 漏了**。

### 0.2 现有覆盖盘点

| 层 | 覆盖情况 | 测试数（约） |
|---|---|---|
| `pkg/*`（pagination / reqctx / idgen） | ✅ 充分 | 32 |
| `infra/db / crypto / events / llm`（含 LLM SSE 解析）| ✅ 充分 | 35（含 5 个 LLM 集成测试 env-gated） |
| `infra/sandbox`（uv + python lifecycle）| ✅ 充分（Phase 3 后大量补） | ~80 |
| `domain/*` | ✅ 够用（多数是纯类型，3 个 model 测试） | 3 |
| `app/apikey / model / conversation` | ✅ Service unit + httptest E2E | ~40 |
| `app/chat`（history / stream） | ⚠️ 仅单测 helper（assembleBlocks / parseToolArgs / blocksToLLM） | ~25 |
| `app/forge` | ⚠️ Service unit 用 **fake sandbox**；真 sandbox lifecycle 不覆盖 | ~19 |
| `app/tool` | ✅ 注入 / 剥除标准字段 | ~12 |
| `app/tool/forge`（5 个 LLM tool）| ❌ **零测试** | 0 |
| `transport/httpapi/middleware` | ✅ 充分 | 27 |
| `transport/httpapi/router` | ✅ 充分 | 6 |
| `transport/httpapi/handlers/apikey/model/conversation` | ✅ E2E httptest | ~30 |
| `transport/httpapi/handlers/forge`（22 端点）| ❌ **零 handler 测试** | 0 |
| `transport/httpapi/handlers/chat`（5 端点）| ❌ 零 handler 测试，仅 pipeline 间接 | 0 |
| `transport/httpapi/handlers/dev` | ❌ 零（dev-only，不必测） | 0 |
| **`backend/test/` 真端到端 pipeline** | ⚠️ **6 场景 + 编译失败** | 6 |

**最大缺口**：
1. **forge 22 个 HTTP 端点** 没有 handler-level 测试
2. **5 个 forge LLM tool**（search/get/create/edit/run_forge）没有任何测试
3. **沙箱迭代 1 lifecycle**（draft→pending→accept、N=3 LRU、evicted 重建、sync 失败 entity-state SSE 推送）只有 fake-sandbox 的 service 单测，**真 sandbox + 真 SSE 流**完全没覆盖
4. **chat × forge 交集**（LLM 在对话中调 create_forge/edit_forge/run_forge）零覆盖
5. **错误码 sweep**——32 个 sentinel 都有 errmap 映射，但很多没有 HTTP 真实路径验证

### 0.3 测试流水线的目标（用户原话："cover 到所有场景，让我们的系统稳定发挥"）

**用户不痛**——以下事故 pipeline 测试必须在 commit 前抓到：
- 沙箱迭代后 harness 跟不上，pipeline 测试静默失活几天没人发现
- forge 22 端点 handler 改了响应形状，前端 dev console 看到才发现
- `app/forge/forge_test.go` 用 fake sandbox，真 uv sync 失败的错误处理路径从来没跑过
- LLM 调 create_forge → 流式生成代码 → entity-state SSE 推送代码增长 → AcceptPending 这条核心 user journey，靠用户在 testend 手动触发才能验证

---

## 1. 设计基准

### 1.1 用户动线（pipeline 测试要支持的开发循环）

```
[1] 工程师改 backend 代码（任意 domain）
[2] 在 devbox shell 里跑 make test-pipeline
        ↓
        ~30 秒：fake-LLM 套件全跑（apikey / model / conversation / chat 基础 / forge HTTP CRUD / forge lifecycle / errcodes sweep）
        失败立刻定位到具体 Test*_*

[3] 改完后跑 make test-pipeline-live （opt-in）
        ↓
        ~3 分钟：含真 DeepSeek API 的端到端测试（reasoner / 多步 ReAct / chat × forge LLM 调用）

[4] 准备 commit 时 make doctor（已有）跑全套：unit + pipeline + staticcheck
```

**关键质量约束**：
- 默认 `make test-pipeline` **不依赖外部网络**——离线 CI 能跑
- 真 LLM 测试通过环境变量 opt-in，跳过时打印 skip 原因
- 真 sandbox 测试若 `$FORGIFY_DEV_RESOURCES` 设了就跑（用真 uv），没设就跳过
- 所有测试**幂等**（§T5 硬要求）：每个 test 起新 harness（in-memory SQLite + 新 httptest server），互不污染
- 所有测试**自描述**（§T1）：`Test<Function>_<Scenario>` 命名，scenario 描述被测条件而非动作

### 1.2 fake-LLM vs 真 LLM 的边界

**真 LLM 的不可替代价值**：捕获真 wire 协议变化（OpenAI SSE chunk 格式 / DeepSeek-R1 reasoning_content / Anthropic tool_use 块边界）。这部分必须用真 DeepSeek。

**fake-LLM 的不可替代价值**：精准控制 LLM 行为（"返回这个 tool_call args" / "中途切断" / "返 401"），让 forge lifecycle 等结构性测试不依赖 LLM 心情。

**结论：分两层**——
| 测试组 | LLM | 触发条件 | 用途 |
|---|---|---|---|
| `make test-pipeline` | **Fake LLM** | 默认；无外部依赖 | 结构 / 协议 / 状态机 / 错误路径 / 历史重建 / 跨 domain 协作 |
| `make test-pipeline-live` | **真 DeepSeek** | `DEEPSEEK_API_KEY` 设了 + 用户主动触发 | wire 协议变化捕获 / reasoner 真实输出 / 真实 token 计数 |

两套测试**共享同一个 harness**——只是注入不同的 `llminfra.Factory`。

---

## 2. 核心实现概览

### 2.1 Fake LLM：httptest server 模拟 OpenAI SSE

不去改 `*llminfra.Factory` 的接口，**让真 Factory 指向 httptest server**——通过 apikey 的 `BaseURL` 字段做覆盖。

```go
// backend/test/fake_llm.go
//
// FakeLLMServer is an httptest.Server that speaks just enough OpenAI Chat
// Completions API to drive the chat runner end-to-end without touching the
// real DeepSeek/OpenAI network.
//
// 优势：
//   - 不改任何 production code
//   - 复用真 openAIClient SSE 解析（一旦 wire 协议改，FakeLLMServer 也得跟改）
//   - apikey.BaseURL 是天然的注入点（Service.ResolveCredentials 会把它透传给 Factory）

type FakeLLMServer struct {
    server  *httptest.Server
    scripts map[string]Script  // 按 messageID 或 "default" 匹配
}

// Script 描述一次 chat completions 调用应该流出什么。
type Script struct {
    // Sequence 是按 SSE chunk 顺序的一系列动作。
    Sequence []ChunkAction
    // FinishReason / InputTokens / OutputTokens 在最后一个 chunk 用。
    FinishReason string
    InputTokens  int
    OutputTokens int
}

type ChunkAction struct {
    Kind         string  // "text" | "reasoning" | "tool_call_start" | "tool_call_args" | "delay" | "abort"
    Content      string  // text/reasoning 内容；tool_call_args 的 args JSON 片段
    ToolID       string
    ToolName     string
    ToolIndex    int
    DelayMs      int     // delay 用：等待多久后再发下个 chunk
    HTTPStatus   int     // abort 用：返非 200 状态码
}

func NewFakeLLMServer(t *testing.T) *FakeLLMServer { ... }
func (f *FakeLLMServer) URL() string { return f.server.URL + "/v1" }  // 与 OpenAI 兼容路径
func (f *FakeLLMServer) PushScript(scenario string, s Script)
func (f *FakeLLMServer) PushDefault(s Script)
```

**注入路径**：
```go
// 测试里：
fake := NewFakeLLMServer(t)
h := New(t, WithFakeLLMBaseURL(fake.URL()))   // harness option：注入到 SeedDeepSeek 创建的 apikey.BaseURL
h.SeedDeepSeek(t, "fake-test-key")            // 走 fake server
fake.PushDefault(Script{Sequence: []ChunkAction{
    {Kind: "text", Content: "Hello "},
    {Kind: "text", Content: "world"},
}, FinishReason: "stop", InputTokens: 10, OutputTokens: 2})

// 后续 chat.Send → runner → llmclient.Resolve → factory.Build →
// openAIClient.Stream(req with BaseURL=fake.URL()) → 命中 fake server
```

**预设脚本工厂**（让测试可读性高）：
```go
// backend/test/fake_llm_scripts.go
func ScriptText(content string) Script
func ScriptReasoningThenText(reasoning, text string) Script
func ScriptSingleToolCall(name string, args map[string]any) Script
func ScriptParallelToolCalls(calls []ToolCallSpec) Script
func ScriptToolCallThenText(call ToolCallSpec, text string) Script
func ScriptStreamThenAbort(textPrefix string, statusCode int) Script
func ScriptStreamThenSlow(text string, perChunkDelay time.Duration) Script  // 给 cancel 测试留窗口
```

### 2.2 真 sandbox vs fake sandbox

参考沙箱迭代 1 §11.1 "punt 给 AI 自救" 的精神——**不构建 fake sandbox，直接用真 uv**：

| 场景 | 决策 |
|---|---|
| 单测层（`app/forge/forge_test.go`）| **保留** fake sandbox（已实现），快、确定 |
| pipeline 层 | **真 sandbox**——`$FORGIFY_DEV_RESOURCES` 设了就跑、没设就 skip（与现有 `infra/sandbox/integration_test.go` 一致） |

理由：
- pipeline 测试本来就是"验证全栈端到端能否工作"，沙箱是端到端的一部分
- 沙箱迭代 1 已经把真 uv 测试做得很扎实（13 个 integration_test.go），pipeline 复用即可
- fake sandbox 在 pipeline 层会让"sync 失败的 entity-state SSE 推送是否对"这种关键测试流于形式

**注意**：sandbox 测试需要 ~5-10 秒/次（uv sync 真跑），所以 forge lifecycle 测试是**慢测试**——单独分文件，跑次数控制在 < 10 次。

### 2.3 Harness 改造（核心 drift 修复）

```go
// backend/test/harness.go（修复后）
import (
    "os"
    sandboxinfra "github.com/sunweilin/forgify/backend/internal/infra/sandbox"
    forgedomain  "github.com/sunweilin/forgify/backend/internal/domain/forge"
)

func New(t *testing.T, opts ...Option) *Harness {
    cfg := buildOptions(opts)
    // ... in-memory SQLite + migrations 不变 ...

    // ── 1. Sandbox（沙箱迭代 1 签名）──
    sandbox := sandboxinfra.New(sandboxinfra.Config{
        DataDir:       t.TempDir(),  // 每 test 独立目录，自动清理
        DefaultPython: forgedomain.DefaultPythonVersion,
        Logger:        log,
    })
    if resourceDir := os.Getenv("FORGIFY_DEV_RESOURCES"); resourceDir != "" {
        if err := sandbox.Bootstrap(context.Background(), resourceDir); err != nil {
            t.Logf("sandbox.Bootstrap failed: %v (forge ops will return ErrSandboxUnavailable)", err)
        }
    }
    // 注：未 Bootstrap 也允许装配——沙箱触达的测试自己 t.Skip 即可

    // ... 其他 service 装配 ...

    // ── 2. forge service 接新 sandbox 接口 ──
    forgeService := forgeapp.NewService(
        forgestore.New(gdb),
        sandbox,         // 6 方法接口
        forgeLLM,
        bridge,
        log,
    )

    // 其他不变
    return &Harness{ ... Sandbox: sandbox, ... }
}

// ── Options pattern ──
type Option func(*options)
type options struct {
    fakeLLMBaseURL string  // 非空时 SeedDeepSeek 用此 BaseURL
}

func WithFakeLLMBaseURL(url string) Option { return func(o *options) { o.fakeLLMBaseURL = url } }
```

**变化点**：
1. ✅ `sandboxinfra.New(Config{...})` 跟齐 main.go
2. ✅ `Bootstrap` 调用：`FORGIFY_DEV_RESOURCES` 设了就跑，没设就让 sandbox 处于 unavailable 状态（forge sandbox 测试会 t.Skip）
3. ✅ `t.TempDir()` 替代固定 dataDir，自动清理 + per-test 隔离
4. ✅ Options pattern 支持 fake LLM 注入

### 2.4 Helpers 抽取

`chat_pipeline_test.go::postMessage` / `extractTextFromBlocks` 这些重复出现的函数搬到 `helpers.go`：

```go
// backend/test/helpers.go
func PostMessage(t *testing.T, h *Harness, convID, content string) string
func PostMessageWithAttachments(t *testing.T, h *Harness, convID, content string, attIDs []string) string
func WaitForFinalAssistant(t *testing.T, sub *SSESub, timeout time.Duration) *chatdomain.Message
func ExtractTextFromBlocks(blocks []chatdomain.Block) string
func ExtractToolCallByName(blocks []chatdomain.Block, name string) (id string, found bool)
func ExtractToolResultByCallID(blocks []chatdomain.Block, callID string) (data map[string]any, found bool)
func MustQueryRow[T any](t *testing.T, h *Harness, sql string, args ...any) T
func DBCount(t *testing.T, h *Harness, table string, where string, args ...any) int64
```

这些 helper 让 test body 关注业务断言，不被样板代码淹没。

---

## 3. 文件夹结构

### 3.1 现状

```
backend/test/
├── harness.go              ← drift（编译失败）
├── seed.go                 ← 简单 fixtures
├── sse.go                  ← SSE collector
├── harness_smoke_test.go   ← smoke
└── chat_pipeline_test.go   ← 5 chat scenarios
```

### 3.2 提议结构

```
backend/test/
├── README.md                          ← 测试流水线指南（怎么跑、怎么加新测试）
│
├── ── 基础设施 ──
├── harness.go                         ← DI 装配（修 drift + Options + Bootstrap）
├── seed.go                            ← Domain fixtures（apikey/model/conv/forge）
├── sse.go                             ← SSE 订阅器（保留）
├── helpers.go                         ← HTTP / 断言 / DB 共享 helper
├── fake_llm.go                        ← Fake OpenAI SSE httptest server
├── fake_llm_scripts.go                ← 预设脚本工厂
│
├── ── 单 domain HTTP 端到端 ──
├── apikey_pipeline_test.go            ← 5 端点 + connectivity test + 跨用户 + cursor
├── model_pipeline_test.go             ← 2 端点 + idempotent + 跨用户
├── conversation_pipeline_test.go      ← 4 端点 + 软删 + 跨用户 + cursor
│
├── ── chat（按场景簇分）──
├── chat_basic_pipeline_test.go        ← 现有 5 + streaming snapshot 单调性 + DB 落库
├── chat_react_pipeline_test.go        ← 多步 ReAct + 并行 tool / 串行混合 / history rebuild
├── chat_attachment_pipeline_test.go   ← upload + text/image extraction + 解析失败软跳过
├── chat_queue_pipeline_test.go        ← STREAM_IN_PROGRESS / 跨对话并行 / Cancel 排空队列
├── chat_autotitle_pipeline_test.go    ← 第一轮完成后 conversation SSE 含新 title
│
├── ── forge ──
├── forge_http_pipeline_test.go        ← 22 端点 CRUD/versions/test_cases/executions
├── forge_lifecycle_pipeline_test.go   ← draft→pending→accept / N=3 LRU / evicted 重建 / revert / 真 sandbox sync
│
├── ── 跨 domain ──
├── chat_forge_pipeline_test.go        ← chat 调 5 forge tool（fake LLM 触发 + 真 sandbox 执行）
│
├── ── 横切 ──
├── errcodes_pipeline_test.go          ← 32 个 sentinel sweep（每个 sentinel 一行短测）
└── isolation_pipeline_test.go         ← 跨用户隔离 / SQL injection 抗性 / 越权访问
```

**总计 ~14 文件、~80 测试用例**。

**命名约定**（§T1 + 本文项目惯例）：
- 文件：`<scope>_pipeline_test.go`（明示这是 pipeline 而非 unit）
- 测试函数：`Test<Domain>_<Scenario>`
  - `<Scenario>` 描述被测条件，**不是动作**
  - 例：`TestForge_AcceptPending_FailedEnvReturns422` ✅
  - 反例：`TestForge_ShouldRejectAcceptOnFailedEnv` ❌
- 真 LLM 必须的测试：`Test<Domain>_Live_<Scenario>` 前缀 `Live_`，便于 `go test -run` 筛选

---

## 4. Coverage Matrix（现状 → 目标）

按 domain × 测试粒度二维展开。✅ = 已覆盖，⚠️ = 部分覆盖，❌ = 缺失，🎯 = 本迭代要新增。

### 4.1 apikey

| 场景 | 现单测 | 现 store | 现 handler | 现 pipeline | 目标 pipeline |
|---|---|---|---|---|---|
| Create + List 往返 | ✅ | ✅ | ✅ | ❌ | 🎯 |
| Create invalid provider → 400 | ✅ | — | ✅ | ❌ | 🎯 |
| Create connectivity test → 200/422 | ✅ | — | ✅ | ❌ | 🎯（fake openai-compat server）|
| 跨用户隔离（user-1 create / user-2 list 空）| ⚠️ | ✅ | — | ❌ | 🎯 |
| Cursor 分页 100 条 | — | ✅ | — | ❌ | 🎯 |
| GetByProvider 选 ok 优先 | ✅ | ✅ | — | ❌ | 🎯 |
| MarkInvalid 在并发 chat 调用下竞态 | — | — | — | ❌ | 🎯（fake LLM 返 401 多次）|
| Update BaseURL → re-test 用新 URL | — | — | — | ❌ | 🎯 |

### 4.2 model

| 场景 | 现 | 目标 |
|---|---|---|
| Upsert + List 往返 | ✅ | 🎯 |
| Upsert 同 scenario 二次 → ID 保持 | ✅ | 🎯 |
| Upsert invalid scenario → 400 | ✅ | 🎯 |
| 跨用户隔离 | ✅ store | 🎯 |
| PickForChat 未配 → 422 MODEL_NOT_CONFIGURED 经 chat 真路径 | — | 🎯（chat_basic 已含）|

### 4.3 conversation

| 场景 | 现 | 目标 |
|---|---|---|
| Create + List + Patch + Delete 往返 | ✅ | 🎯 |
| 软删后 List 不出现 | ✅ store | 🎯 |
| 跨用户隔离（user-1 创建 / user-2 删 → 404）| ✅ store | 🎯 |
| Cursor 分页 100 条 | — | 🎯 |
| Empty title allowed | ✅ | 🎯 |
| autoTitled 默认 false / 自动改写后 true | — | 🎯（chat_autotitle 含）|

### 4.4 chat

| 场景 | 现单测 | 现 pipeline | 目标 pipeline |
|---|---|---|---|
| 简单文本流 + token 单调生长 + DB 落库 | ⚠️ stream_test | ✅ | 保留 |
| Pre-LLM error: missing model_config | — | ✅ | 保留 |
| Pre-LLM error: missing api key | — | ❌ | 🎯 |
| Pre-LLM error: LLM build fail | — | ❌ | 🎯 |
| LLM 中途 401 → status=error errorCode=LLM_STREAM_ERROR | — | ❌ | 🎯（fake LLM 中途 abort）|
| Cancel mid-stream + DB cancelled | — | ✅ | 保留 |
| Cancel during tool execution | — | ❌ | 🎯 |
| Reasoner（DeepSeek-R1）reasoning + text blocks 分流 | — | ✅ Live | 保留（Live） |
| 单 tool call（search_forges 只读，不写 forge_executions）| — | ✅ | 保留 |
| 多步 ReAct（read → process → write 链）| — | ❌ | 🎯（fake LLM 编排 3 步）|
| 并行 tool call（2 read-only 进同一 batch）| — | ❌ | 🎯 |
| 混合并行 + 串行（safe → unsafe → safe 分 3 batch）| — | ❌ | 🎯 |
| History rebuild：快速连发 user1 → user2 → assistant 顺序正确 | — | ❌ | 🎯（fake LLM 慢响应）|
| History rebuild：tool_call + tool_result 多轮注入 | — | ❌ | 🎯 |
| Attachment text 上传 + 内容提取入 LLM context | — | ❌ | 🎯 |
| Attachment image 上传 + base64 入 vision content part | — | ❌ | 🎯（fake server 验 image_url）|
| Attachment 解析失败 → 软跳过 + warn log + 其他 parts 仍发 | — | ❌ | 🎯 |
| Auto-title：第一轮完成 → conversation SSE 出现新 title | — | ❌ | 🎯 |
| Queue 满（5 个未处理时第 6 个）→ 409 STREAM_IN_PROGRESS | — | ❌ | 🎯 |
| 跨对话并行（A 流文本 + B 调 tool）独立 worker | — | ❌ | 🎯 |
| Cancel 排空队列（send 3 后 cancel → 后 2 不处理）| — | ❌ | 🎯 |
| SSE keep-alive ping 15s 不断连 | — | ❌ | 🎯（短测：开订阅 16s 看是否仍活）|
| Worker 5 分钟空闲后退出（这条难测，可省）| — | ❌ | 跳过 |

### 4.5 forge HTTP（22 端点）

| 端点 | Method | 现 handler 测试 | 目标 pipeline |
|---|---|---|---|
| `/forges` | POST | ❌ | 🎯 含 deps + sync 完成断言 EnvStatus |
| `/forges` | GET | ❌ | 🎯 cursor 分页 + 含 Pending |
| `/forges/{id}` | GET | ❌ | 🎯 含计算字段 EnvStatus |
| `/forges/{id}` | PATCH | ❌ | 🎯 metadata only / code only / deps 不接受 |
| `/forges/{id}` | DELETE | ❌ | 🎯 软删 + sandbox.Destroy |
| `/forges/{id}:run` | POST | ❌ | 🎯 真 sandbox 执行 + ForgeExecution 写 |
| `/forges/{id}:export` | POST | ❌ | 🎯 |
| `/forges:import` | POST | ❌ | 🎯 round-trip |
| `/forges/{id}:revert` | POST | ❌ | 🎯 含 evicted 重建分支 |
| `/forges/{id}:test` | POST | ❌ | 🎯 batch all tests |
| `/forges/{id}:generate-test-cases` | POST | ❌ | 🎯（用 fake LLM 返 test_cases JSON） |
| `/forges/{id}/versions` | GET | ❌ | 🎯 |
| `/forges/{id}/versions/{v}` | GET | ❌ | 🎯 |
| `/forges/{id}/pending` | GET | ❌ | 🎯 |
| `/forges/{id}/pending:accept` | POST | ❌ | 🎯 含 EnvStatus 守卫（ready 放行 / failed 422 / 其他 422） |
| `/forges/{id}/pending:reject` | POST | ❌ | 🎯 含 draft 整体删除分支 |
| `/forges/{id}/test-cases` | POST/GET | ❌ | 🎯 |
| `/forges/{id}/test-cases/{tc}:run` | POST | ❌ | 🎯 |
| `/forges/{id}/test-cases/{tc}` | DELETE | ❌ | 🎯 |
| `/forges/{id}/executions` | GET | ❌ | 🎯 ?kind / ?batchId 过滤 + cursor |

### 4.6 forge lifecycle（沙箱迭代 1）

| 场景 | 现 service unit（fake sb）| 目标 pipeline（真 sb） |
|---|---|---|
| HTTP POST /forges 直接 v1 + 真 uv sync 完成 | ⚠️ | 🎯 |
| HTTP POST /forges 但 deps 错（uv 解析失败）→ EnvStatus=failed + EnvError 含 stderr | — | 🎯 |
| LLM create_forge：CreateDraft → stream code → ParseCode → Create | ✅ | 🎯（fake LLM 推进 + 真 sandbox） |
| LLM create_forge → AST parse fail → tool_result error / 草稿留 | — | 🎯 |
| LLM edit_forge 改 deps → 新 EnvID → 真 sync | ✅ | 🎯 |
| LLM edit_forge 仅改 code → EnvID 不变 → 不重 sync（用 WriteCodeFile）| ✅ | 🎯 |
| LLM edit_forge 已有 pending → 自动 reject + 创新 | ✅ | 🎯 |
| HTTP AcceptPending EnvStatus=ready → 放行 + version 分配 | ✅ | 🎯 |
| HTTP AcceptPending EnvStatus=failed → 422 FORGE_ENV_FAILED（含 envError） | ✅ | 🎯 |
| HTTP AcceptPending EnvStatus=pending/syncing/evicted → 422 FORGE_ENV_NOT_READY | ✅ | 🎯 |
| HTTP RejectPending（draft 阶段）→ 整个 forge 删除 + sandbox.Destroy | ✅ | 🎯 |
| HTTP RejectPending（已 active）→ pending 标 rejected，forge 保留 | ✅ | 🎯 |
| HTTP revert 到非 evicted 版本 → 不 sync | ✅ | 🎯 |
| HTTP revert 到 evicted 版本 → 触发 SyncEnvForVersion → 真 sync 重建 | ✅ | 🎯 |
| AcceptPending 触发 trimEnvBuffer：N=4 个 EnvID 时淘汰最旧 + DestroyEnv | ✅ | 🎯 |
| Soft-delete forge → DestroyForge 清整个 forge 目录 | — | 🎯 |
| 跨用户隔离（user-1 forge / user-2 GET → 404）| — | 🎯 |
| Sandbox unavailable（无 Bootstrap）→ 所有 sandbox 触达端点 503 | — | 🎯 |
| ForgeExecution kind=run，HTTP 触发 → TriggeredBy=http | — | 🎯 |
| ForgeExecution kind=run，chat 触发 → TriggeredBy=chat + ConversationID/MessageID/ToolCallID 填入 | — | 🎯 |
| ForgeExecution 300 条上限 → 第 301 条插入时最旧硬删 | — | 🎯（构造小批）|

### 4.7 forge SSE entity-state 推送

| 场景 | 现 | 目标 pipeline |
|---|---|---|
| LLM create_forge 流 N 帧 forge 快照（code 增长）| — | 🎯 |
| LLM create_forge sync 阶段 3 帧（resolving / preparing / installing）| — | 🎯 |
| LLM create_forge 终态：EnvStatus=ready 帧 | — | 🎯 |
| LLM create_forge sync 失败：EnvStatus=failed + EnvError 帧 | — | 🎯 |
| LLM edit_forge 同上但 .Pending 子对象生长 | — | 🎯 |
| HTTP AcceptPending 后：Pending=null + ActiveVersionID 切换 帧 | — | 🎯 |
| HTTP revert 到 evicted：syncing 3 帧 + ready 帧 | — | 🎯 |

### 4.8 chat × forge 交集（5 个 LLM tool）

| Tool | 场景 | 目标 |
|---|---|---|
| search_forges | LLM 调 → ListAll + Rank → 返 ranked JSON；**不写 ForgeExecution** | 🎯 |
| get_forge | LLM 调 → 返完整 Forge + TestSummary | 🎯 |
| create_forge | LLM 调 → 流式生成 + 真 sandbox sync + 草稿入 DB | 🎯 |
| edit_forge | LLM 调 → 流式改写 + CreatePending + 新 sync | 🎯 |
| run_forge | LLM 调 → resolveAttachments + RunForge → 写 ForgeExecution（TriggeredBy=chat 完整 IDs）| 🎯 |
| destructive 字段 | fake LLM 在 args 加 `"destructive": true` → ToolCallData.Destructive=true → SSE chat.message 含 | 🎯 |
| run_forge 输出 50KB 截断 | LLM 调 forge 输出大量数据 → tool_result.output 替为 notice | 🎯 |

### 4.9 错误码 sweep（32 sentinel）

每个 sentinel 至少 1 个 pipeline 测试触发其 HTTP 状态码 + envelope code 字段：

```
INVALID_REQUEST                400  derrors.ErrInvalidRequest
INTERNAL_ERROR                 500  derrors.ErrInternal / reqctxpkg.ErrMissingUserID / cryptoinfra.ErrUnsupportedVersion
NOT_FOUND                      404  middleware（路由未匹配）
API_KEY_NOT_FOUND              404  apikey.ErrNotFound
API_KEY_PROVIDER_NOT_FOUND     404  apikey.ErrNotFoundForProvider（chat 路径间接）
INVALID_PROVIDER               400  apikey.ErrInvalidProvider
BASE_URL_REQUIRED              400  apikey.ErrBaseURLRequired
API_FORMAT_REQUIRED            400  apikey.ErrAPIFormatRequired
KEY_REQUIRED                   400  apikey.ErrKeyRequired
API_KEY_TEST_FAILED            422  apikey.ErrTestFailed
API_KEY_INVALID                401  apikey.ErrInvalid
MODEL_NOT_CONFIGURED           422  model.ErrNotConfigured
INVALID_SCENARIO               400  model.ErrInvalidScenario
PROVIDER_REQUIRED              400  model.ErrProviderRequired
MODEL_ID_REQUIRED              400  model.ErrModelIDRequired
CONVERSATION_NOT_FOUND         404  conversation.ErrNotFound
MESSAGE_NOT_FOUND              404  chat.ErrMessageNotFound
STREAM_NOT_FOUND               404  chat.ErrStreamNotFound
STREAM_IN_PROGRESS             409  chat.ErrStreamInProgress
LLM_PROVIDER_ERROR             502  chat.ErrProviderUnavailable
ATTACHMENT_TOO_LARGE           413  chat.ErrAttachmentTooLarge
ATTACHMENT_TYPE_UNSUPPORTED    415  chat.ErrAttachmentTypeUnsupported
ATTACHMENT_PARSE_FAILED        422  chat.ErrAttachmentParseFailed
VISION_NOT_SUPPORTED           422  chat.ErrVisionNotSupported
TOOL_NOT_FOUND                 404  forge.ErrNotFound（实际 sentinel 是 forgedomain）
TOOL_NAME_DUPLICATE            409  forge.ErrDuplicateName
TOOL_VERSION_NOT_FOUND         404  forge.ErrVersionNotFound
TOOL_PENDING_NOT_FOUND         404  forge.ErrPendingNotFound
TOOL_PENDING_CONFLICT          409  forge.ErrPendingConflict
TOOL_TEST_CASE_NOT_FOUND       404  forge.ErrTestCaseNotFound
TOOL_RUN_FAILED                422  forge.ErrRunFailed
TOOL_AST_PARSE_FAILED          422  forge.ErrASTParseError
TOOL_IMPORT_INVALID            400  forge.ErrImportInvalid
FORGE_ENV_NOT_READY            422  forge.ErrEnvNotReady
FORGE_ENV_FAILED               422  forge.ErrEnvFailed
FORGE_SANDBOX_UNAVAILABLE      503  forge.ErrSandboxUnavailable
FORGE_DEPENDENCY_RESOLUTION    422  forge.ErrDependencyResolution
```

`errcodes_pipeline_test.go` 用 table-driven 一次扫完。

### 4.10 横切

| 场景 | 现 | 目标 |
|---|---|---|
| 跨用户隔离全 domain（apikey / model / conv / forge）| ⚠️ store | 🎯 真 HTTP 切 user header |
| SSE keep-alive ping 15s 心跳 | — | 🎯 |
| SSE filter key（订阅 conv-A 不收 conv-B 事件）| — | 🎯 |
| InjectUserID 中间件缺失 → 接线 bug 检测 | ⚠️ middleware unit | 跳过（已有 unit） |
| Recover middleware：handler panic → 500 + envelope | ✅ middleware unit | 🎯（pipeline 触发真 panic 路径）|

---

## 5. Phase 划分（独立可交付）

每个 Phase 完成时**全栈编译 + 测试绿** 是 acceptance criteria。

### Phase A — 修 drift + Fake LLM 基础设施（~2h）

**目标**：harness 能跑、fake LLM 注入工作、smoke test 全绿。

子任务：
1. `harness.go` 改 `sandboxinfra.New(Config{...})` + `Bootstrap` 调用 + Options pattern
2. `fake_llm.go` + `fake_llm_scripts.go`：实现 OpenAI SSE 兼容的 httptest server，覆盖最少功能：text 流 / tool_call 流 / abort / delay
3. `helpers.go`：抽 `PostMessage` / `ExtractTextFromBlocks` / `WaitForFinalAssistant` / `DBCount`
4. `seed.go`：扩展 `SeedDeepSeek` 接受 `WithFakeLLMBaseURL` 注入
5. 现有 `harness_smoke_test.go` 改用 fake LLM（去掉 `RequireDeepSeekKey`）
6. 现有 `chat_pipeline_test.go` 5 个 case 拆为 fake-LLM 部分（4 个）+ Live 部分（1 个 reasoner）

验收：
- `go build -tags=pipeline ./test/...` ✅
- `make test-pipeline`（无 .env）→ smoke + 4 chat 全绿，~5s
- `make test-pipeline-live`（含 .env DEEPSEEK_API_KEY）→ + 1 reasoner = 全绿，~10s

### Phase B — 三个轻 domain 的 HTTP 端到端（~1h）

**目标**：apikey / model / conversation 每个 HTTP 端点都有 pipeline 真路径覆盖。

子任务：
1. `apikey_pipeline_test.go`：8 个测试（CRUD / connectivity 用 fake openai-compat / 跨用户 / cursor 100 条）
2. `model_pipeline_test.go`：4 个测试（CRUD / idempotent / 跨用户 / chat 路径触发 MODEL_NOT_CONFIGURED）
3. `conversation_pipeline_test.go`：5 个测试（CRUD / 软删 / 跨用户 / cursor）

验收：
- 17 个新测试全绿
- 总耗时 < 3s（无 LLM、无 sandbox）

### Phase C — chat 场景全覆盖（~3h）

**目标**：chat 域所有 ReAct / 并行 / cancel / queue / autotitle / attachment 场景。

子任务：
1. `chat_basic_pipeline_test.go`：迁移现有 4 个 + 新增 5 个（pre-LLM API key 缺 / 中途 401 / cancel during tool / DB streaming checkpoint）
2. `chat_react_pipeline_test.go`：5 个测试（多步 ReAct / 并行 safe batch / 混合 batch / history rebuild 顺序 / max_steps 退出）
3. `chat_attachment_pipeline_test.go`：4 个测试（text upload + 入 prompt / image upload + base64 / 解析失败软跳过 / 50MB 超限 413）
4. `chat_queue_pipeline_test.go`：4 个测试（queue 满 409 / 跨对话独立 worker / cancel 排空 / SSE keep-alive 15s）
5. `chat_autotitle_pipeline_test.go`：2 个测试（title 自动生成 / 已 autoTitled 不重生）

验收：
- 20 个新测试全绿
- 总耗时 < 15s（fake LLM + 内存 SQLite）

### Phase D — forge HTTP + lifecycle（~3h）

**目标**：forge 22 端点 + sandbox iteration 1 lifecycle 全覆盖（真 sandbox）。

**前置**：`$FORGIFY_DEV_RESOURCES` 资源就绪（`cd backend && go run ./cmd/resources`）。

子任务：
1. `forge_http_pipeline_test.go`：~15 个测试，覆盖 22 端点（含 cursor / executions filter / test cases batch）
2. `forge_lifecycle_pipeline_test.go`：~10 个测试，覆盖 lifecycle 路径（a-f）+ N=3 LRU + evicted 重建 + sandbox.Destroy
3. SSE 推送验证：每个 lifecycle test 顺带断言 forge SSE 帧序列（用 SSESub.RawEvents()）

验收：
- 25 个新测试，需要 `FORGIFY_DEV_RESOURCES` 资源
- 总耗时 ~60-90s（真 uv sync 是慢的）
- 无资源时 `t.Skip` 优雅跳过

### Phase E — chat × forge 交集（~2h）

**目标**：5 个 forge LLM tool 在 chat 场景下端到端跑通。

子任务：
1. `chat_forge_pipeline_test.go`：~7 个测试
   - LLM 调 search_forges → ranked 结果 + 不写 forge_executions
   - LLM 调 get_forge → 返 detail 含 TestSummary
   - LLM 调 create_forge → fake LLM 推代码 + 真 sandbox sync + draft + pending
   - LLM 调 edit_forge 改 code → 同 EnvID 不重 sync
   - LLM 调 edit_forge 改 deps → 新 EnvID 真 sync
   - LLM 调 run_forge → 写 ForgeExecution(TriggeredBy=chat) + 跨表 join 找回原 chat 消息
   - destructive 字段端到端流转（fake LLM args + ToolCallData + chat.message SSE）

验收：
- 7 个新测试全绿
- 总耗时 ~30-60s（混合 fake LLM + 真 sandbox）

### Phase F — 错误码 sweep + 跨用户隔离（~1h）

**目标**：32 个 sentinel 全覆盖 + 跨 domain 越权访问验证。

子任务：
1. `errcodes_pipeline_test.go`：table-driven 一次扫完所有 sentinel
2. `isolation_pipeline_test.go`：~5 个测试（apikey / model / conv / forge 跨用户 + 越权 GET 返 404 不是 403）

验收：
- ~37 个新测试全绿
- 总耗时 < 5s

### Phase G — 文档 + Makefile + 全套 doctor（~1h）

**目标**：测试流水线文档化、跑流程清晰。

子任务：
1. `backend/test/README.md`：写"怎么跑、怎么加新测试、fake LLM 用法"
2. `Makefile`：
   - `make test-pipeline`：默认 fake LLM，不需 DEEPSEEK_API_KEY；需 `FORGIFY_DEV_RESOURCES`（forge sandbox 测试 skip 友好）
   - `make test-pipeline-live`：source .env 自动注入 DEEPSEEK_API_KEY，跑含 `Live_` 前缀的测试
   - `make test-pipeline-quick`：跳过真 sandbox（`-skip Sandbox|Live_`），秒级回归
3. `make doctor` 加跑 `test-pipeline`
4. CLAUDE.md §T 系列加 T6（pipeline 测试用 fake LLM 默认；Live 测试 opt-in）
5. progress-record.md 加本迭代条目
6. 跑一遍全套确认绿

验收：
- 全 ~110 测试全绿（fake-LLM 模式）
- `make test-pipeline-live` 全绿（真 LLM 模式）
- 文档同步

---

## 6. 决策表

| 决策 | 选择 | 理由 |
|---|---|---|
| Fake LLM vs 真 LLM | **分两层**：默认 fake；live opt-in | 速度 / 离线友好 / 真协议捕获三者兼顾 |
| Fake LLM 实现方式 | **httptest server emit OpenAI SSE** | 不改 production 代码；复用真 openAIClient 解析；wire 协议变化能被抓到 |
| Fake LLM 注入点 | **apikey.BaseURL** 字段 | 天然存在的覆盖机制，零侵入 |
| Fake sandbox vs 真 sandbox | pipeline 层**真 sandbox**（`$FORGIFY_DEV_RESOURCES` 设了就跑） | 单测层 fake 已够；pipeline 是端到端验证 |
| 文件夹分块粒度 | **按场景簇**（~14 文件 / 80 测试）| 太粗失败定位难；太细文件管理累 |
| 命名约定 | `Test<Domain>_<Scenario>` + `Live_` 前缀标识 | §T1 + 让 `go test -run` 筛选清晰 |
| Harness Options pattern | **functional options**（`WithFakeLLMBaseURL` / `WithoutBootstrap`）| 向后兼容现有 `New(t)` 调用 |
| 跨用户测试 | 在 helpers 提供 `LocalCtxAs(userID)` 切换 | InjectUserID middleware 在 HTTP 层 hard-code，pipeline 直接调 service 时手动切 ctx |
| 真 LLM 测试位置 | 与 fake 同文件，`Live_` 前缀区分 | 测试场景相邻可读；运行时按前缀筛 |
| 测试 timeout | 单测 60s / 集成 180s（reasoner 慢）| 留余地避免 flaky |
| Bootstrap 失败行为 | harness 仍构造完成，sandbox 处于 unavailable，触达测试 t.Skip | 不阻塞无关测试 |
| dataDir 隔离 | 用 `t.TempDir()` per harness | 内置 cleanup + 隔离 |
| LLM SSE 协议覆盖范围 | fake server 仅模拟 OpenAI 兼容；Anthropic 不模拟 | DeepSeek/Qwen/Moonshot 都走 OpenAI 兼容；Anthropic 真实流由 Live 测试补 |
| Cursor 测试 dataset 大小 | 10-20 条够（不必 100）| 验证 cursor 编解码 + ORDER BY 稳定即可，量级无关正确性 |
| forge_executions 300 上限测试 | 构造小批（如 hard-set MaxExecutions=5 via test override 或就插 305 条）| 真插 300 条慢；hard-set 需 export 常量为变量——避免，就插 |
| SSE 长连接验证 | 用 `time.Sleep(16s)` 验 keep-alive | 慢但精确；放在专门的 chat_queue 文件 |

---

## 7. 不做的事（明确范围）

- ❌ **Anthropic wire 协议 fake**——只模拟 OpenAI 兼容，Anthropic 协议变化由 `infra/llm/anthropic_test.go`（unit）+ Live 测试覆盖
- ❌ **多用户真实 auth 流程**（Phase 5 才接 JWT/session）——pipeline 仍 hard-code `local-user`，跨用户测试通过 ctx 切换
- ❌ **真 OpenAI / Anthropic 测试**——仅真 DeepSeek（项目唯一活跃 API key）
- ❌ **Bootstrap 失败的具体诊断测试**（缺 uv / python tarball 损坏 / mac codesign 失败）——这些已在 `infra/sandbox/preflight_test.go` 单测覆盖
- ❌ **HTML/CSS/JS testend 前端测试**——dev console 是 dev-only，不进 pipeline
- ❌ **可视化测试报告 / coverage 报告**——`go test -v` 输出 + 失败时 `FormatRawEvents()` 已够
- ❌ **fuzz 测试**——pipeline 是端到端确定性测试，fuzz 是另一类工具（可后续独立加）
- ❌ **性能 benchmark**——pipeline 不做 benchmark，性能用 unit benchmark + 真生产监控
- ❌ **MCP 测试**（Phase 5 才有 MCP）
- ❌ **workflow 测试**（Phase 4 才有 workflow）

---

## 8. 风险与已知 hairy 点

### 8.1 fake LLM 与真 OpenAI 协议漂移

**风险**：fake server 实现可能漏掉某个 OpenAI SSE 边角行为，导致 fake 测试通过但真 LLM 跑不动。

**缓解**：
- Phase A 必须包含 1 个 chat_basic Live 测试（reasoner），保留 wire 协议兜底
- 真 openAIClient 如果改 SSE 解析逻辑，对应 `openai_test.go` unit 必须先跟改（unit 是契约层）
- 文档 fake 仅模拟"happy path + abort + delay"，复杂 corner case（多 choice / function_call legacy 格式 / streaming finish 边界）不模拟

### 8.2 真 sandbox 测试的 flakiness

**风险**：uv sync 跑 10s+，CI 偶尔 timeout；Python 子进程在 mac sandbox 偶发 SIGKILL。

**缓解**：
- Phase D 测试单独 timeout 提到 180s
- 全部 forge_lifecycle 测试用同一个 forge 名前缀（`f_pipeline_test_*`）让失败时易于清理
- Bootstrap 失败时 t.Skip 而非 fail
- mac codesign 失败已在 sandbox preflight 处理（沙箱迭代 1）

### 8.3 SSE 测试的时序敏感

**风险**：SSESub 用 `WaitForX(timeout)` 模式，慢机器可能 timeout / 顺序判断假阳性。

**缓解**：
- 默认 60s timeout（chat）/ 180s timeout（reasoner）
- entity-state 模型本身是"最终态"导向，中间帧丢失不影响最终断言
- 时序敏感测试（如 token 单调生长）显式拿 RawEvents() 看完整 history，断言失败时打印全部帧（FormatRawEvents 已实现）

### 8.4 测试间状态污染

**风险**：每 test 都 `harness.New(t)` 起新实例本应隔离，但若忘了或共用全局变量会污染。

**缓解**：
- §T5 硬要求"幂等"——任何 test 不得依赖前一次运行残留
- 文档 README.md 强调 + Phase G 加 doctor 检查（重复跑 3 次结果一致）
- t.TempDir() 自动清理，不需要测试自己 rm
- fake LLM server 每个 harness 独立实例，scripts 不跨 test 复用

### 8.5 测试代码量爆炸

**预估**：~3000-4500 行 pipeline 测试代码（80 测试 × 平均 40 行）+ 600 行 helpers/fake_llm。

**缓解**：
- helpers + fake_llm scripts 工厂大幅减少样板
- table-driven 哪里能用就用（errcodes sweep 必用）
- 命名清晰，让 `go test -run` grep 友好

---

## 9. 实施顺序图

```
                Phase A（基础设施 2h）
                      │
        ┌─────────────┼─────────────────┐
        ↓             ↓                 ↓
   Phase B        Phase C            Phase D
   3 轻 domain    chat 场景           forge HTTP + lifecycle
      1h           3h                   3h
                                          │
        └─────────────┼─────────────────┘
                      ↓
                 Phase E（chat × forge 交集 2h）
                      │
                      ↓
                 Phase F（errcodes + isolation 1h）
                      │
                      ↓
                 Phase G（文档 + Makefile + doctor 1h）
```

**总工时估算**：~13 小时（与沙箱迭代 1 同量级）

**MVP 截止线**：Phase A + B + C + F = ~7h，能让 fake-LLM 测试套覆盖 chat 全场景 + 3 轻 domain + 错误码扫描，已经是巨大提升。Phase D + E（真 sandbox / chat × forge）可后续独立做。

---

## 10. 验收标准（全 Phase 完成）

- [ ] `make test-pipeline`（默认）全绿，~30-60s
- [ ] `make test-pipeline-live`（含 .env）全绿，~3-5min
- [ ] `make test-pipeline-quick`（跳过真 sandbox）全绿，~10-15s
- [ ] 所有 32 个 sentinel 都有 pipeline 触发路径
- [ ] forge 22 端点都有 pipeline 测试
- [ ] 5 个 forge LLM tool 都有 chat 场景测试
- [ ] entity-state SSE 3 事件（chat.message / forge / conversation）每个都有触发点测试
- [ ] 跨用户隔离 4 个 domain 都验
- [ ] `staticcheck ./...` 0 warnings（默认 + pipeline tag 双跑）
- [ ] `staticcheck -tags=pipeline ./test/...` 0 warnings
- [ ] `make doctor` 加跑 pipeline，绿
- [ ] `backend/test/README.md` 文档化
- [ ] CLAUDE.md §T 系列加 T6 + 沙箱测试约定
- [ ] progress-record.md 加本迭代完工条目
- [ ] 同一 test 反复跑 3 次结果一致（§T5 幂等性）

---

**End of Spec**
