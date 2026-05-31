# 测试流水线迭代 2：全面 overhaul

**日期**：2026-05-27
**状态**：📐 设计已批准（未开工）
**关联**：
- 上一份迭代（2026-05-03，70% 落地后 drift 复发）：[`01-foundation-and-coverage.md`](./01-foundation-and-coverage.md)
- 审计报告（2026-05-26）：[`../audit/completeness-audit-report.md`](../audit/completeness-audit-report.md) §🟡-B
- 现状代码：[`backend/test/`](../../../../backend/test/)
- CLAUDE.md §T 系列 + §S12 + §S14
- 并行 agent：testend 完整重写正在另一 worktree 进行

---

## 0. 背景与触发

### 0.1 上一份迭代后的状态（部分落地）

2026-05-03 的 `01-foundation-and-coverage.md` 提出"fake LLM httptest server + 真 sandbox + 双 target + `_pipeline_test.go` 命名"等方案。约 70% 落地——`harness/fake_llm.go`（317 行）+ `fake_llm_scripts.go`（118 行）+ Options pattern + per-test 隔离 + 26 个 domain 测试包均到位。

**未收尾的部分**：
1. **harness 签名又漂移了**（这次：`LocalCtxAs` 调用站签名错 + `reqctxpkg.DefaultLocalUserID` 已删但 3 处仍引用）—— 同一类事故第二次发生
2. **文件命名约定未遵守**——全是 `<domain>_test.go` 不是 `<domain>_pipeline_test.go`
3. **`make e2e` 没拆 fake / live 两档**——一锅煮，跑一次烧 token
4. **没有 `backend/test/README.md`**
5. **覆盖率矩阵从未反查**——原矩阵基于"forge 一个 domain"，trinity 重构后过期
6. **"~315 测试全绿" 基线在 pipeline 编译挂掉之前从未被验证**

### 0.2 触发本次 overhaul 的根因

`make e2e` 当前编译失败（审计报告 🟡-B），守护着 ~2587 行 workflow 执行引擎的 e2e 测试**根本跑不起来**。这是这次 overhaul 的导火索，但**目标不是修编译——目标是把这套测试模块做成"优雅、高效、有用的测试端口"**。

### 0.3 设计约束（用户明确）

- **工程完备性 > 简洁**：不追求最少代码、不追求最少 target；追求该有的都有、漂移不可能复发
- **不烧 token 为默认**：日常 driver 跑 fake LLM；真 LLM 只在显式 opt-in 时跑
- **Sandbox 默认全开**：`FORGIFY_DEV_RESOURCES` 设了就跑真 uv，没设优雅 skip
- **Makefile 单字 target**：22 个 target 全部 `make <single-word>`，无连字符复合
- **并行 agent 隔离**：本 overhaul 在 worktree 跑短命 topic branch，与 testend 重写并行不撞

---

## 1. 目标

### 1.1 系统层目标（"做完后是什么样"）

1. `make verify` 从干净 checkout 跑通，包含 `mock`（~60s 离线 pipeline 测试），无 token 消耗
2. `make e2e` 是真正的 release gate：跑 mock + sandbox + live，按 fail-fast 顺序
3. `backend/test/README.md` 5 分钟讲清"如何加一个新测试"，自带自动生成的覆盖矩阵段
4. **签名漂移在编译阶段就抓**（`make verify` 内的 `go build -tags=pipeline ./backend/test/...`）
5. **覆盖率自动反查**：加端点忘记测 → `make verify` 在 audit 步立刻挂
6. ~251 测试按 axis（api / cross / sse / lifecycle / errcodes / live / smoke）有序分布，文件命名 `<scope>_pipeline_test.go` 强制
7. 测试函数顶上 `// covers:` annotation 覆盖到 HTTP 端点 / 错误码 / SSE 事件 / 跨 domain 锈川 / 长链路 全部封闭枚举
8. 所有 close/shutdown 走 `t.Cleanup()`、所有跨用户 ctx 走 `harness.CtxAs()`、`t.Parallel()` 政策明确

### 1.2 非目标

- ❌ 单元测试改造（`make test-backend` 不在范围；本 overhaul 只动 pipeline 层）
- ❌ 前端测试改造（`make test-frontend` / `make lint-frontend` 改名，但内容不动）
- ❌ 引入新测试框架（不上 testify / ginkgo / gomega；保持 `testing` 标准库 + `t.Helper()`）
- ❌ 引入 mock 库（fake LLM 是 httptest 真实服务，不是 gomock-style）
- ❌ CI 配置（项目当前无 CI；本 overhaul 全 local-first，CI 时考虑再加）

---

## 2. 架构与目录布局

### 2.1 顶层结构

```
backend/test/
├── README.md                           5 分钟"如何加测"导览 + 自动生成矩阵段
│
├── harness/                            测试共享底座（每文件 < 200 行，职责单一）
│   ├── harness.go                      DI 装配 + Options + Harness struct + New
│   ├── factories.go                    *Harness 上的 Seed* 工厂
│   ├── http.go                         *Harness 上的 HTTP 客户端方法
│   ├── sse.go                          SSESub + 3 流订阅
│   ├── db.go                           DBCount + 泛型 QueryRow / MustQueryRow
│   ├── assertions.go                   Extract* + WaitFor* + Assert* 共享断言
│   ├── ctx.go                          CtxAs / DefaultCtx / DefaultUserID 常量
│   ├── live_gate.go                    RequireDeepSeekKey / RequireSandboxResources
│   ├── fake_llm.go                     FakeLLMServer（保留）
│   ├── fake_llm_scripts.go             预设脚本工厂（保留）
│   └── test_registry.go                LLM tool 注册表 inspector（保留）
│
├── smoke/                              启动冒烟（1 文件，2 测试）
│   └── smoke_pipeline_test.go
│
├── api/                                单 domain × HTTP 端点 happy + 主错路径（14 子包）
│   ├── apikey/        apikey_pipeline_test.go
│   ├── conversation/  conversation_pipeline_test.go
│   ├── chat/          chat_pipeline_test.go
│   ├── function/      function_pipeline_test.go
│   ├── handler/       handler_pipeline_test.go
│   ├── workflow/      workflow_pipeline_test.go
│   ├── flowrun/       flowrun_pipeline_test.go
│   ├── document/      document_pipeline_test.go
│   ├── memory/        memory_pipeline_test.go
│   ├── skill/         skill_pipeline_test.go
│   ├── mcp/           mcp_pipeline_test.go
│   ├── model/         model_pipeline_test.go
│   ├── relation/      relation_pipeline_test.go
│   └── user/          user_pipeline_test.go
│
├── cross/                              跨 domain 锈川（11 文件）
│   ├── chat_trinity_pipeline_test.go
│   ├── workflow_scheduler_pipeline_test.go
│   ├── mention_document_pipeline_test.go
│   ├── catalog_consistency_pipeline_test.go
│   ├── subagent_pipeline_test.go
│   ├── isolation_pipeline_test.go
│   ├── compaction_memory_pipeline_test.go
│   ├── askai_iterate_pipeline_test.go
│   ├── eventlog_aggregation_pipeline_test.go
│   ├── relation_sync_pipeline_test.go
│   └── tool_framework_pipeline_test.go
│
├── sse/                                3 流协议 round-trip（3 文件）
│   ├── eventlog_pipeline_test.go
│   ├── notifications_pipeline_test.go
│   └── forge_pipeline_test.go
│
├── lifecycle/                          长链路 / 真 sandbox（4 文件，env-gated）
│   ├── function_env_pipeline_test.go
│   ├── handler_config_pipeline_test.go
│   ├── workflow_dag_pipeline_test.go
│   └── sandbox_bootstrap_pipeline_test.go
│
├── errcodes/                           每 sentinel 一行 sweep（1 文件）
│   └── sweep_pipeline_test.go
│
└── live/                               真 LLM 测试（4 文件，env-gated DEEPSEEK_API_KEY）
    ├── reasoner_pipeline_test.go
    ├── multi_step_react_pipeline_test.go
    ├── chat_trinity_pipeline_test.go
    └── wire_protocol_pipeline_test.go
```

### 2.2 命名约定（强制）

| 维度 | 规则 |
|---|---|
| 测试函数 | `Test<Domain>_<Scenario>`，Scenario 描述被测条件不描述动作 |
| 跨 domain | `TestCross_<SeamName>_<Scenario>` |
| SSE 协议 | `TestSSE_<Stream>_<Scenario>` |
| 生命周期 | `TestLifecycle_<Domain>_<Scenario>` |
| 错误码 | `TestErrCode_<EXACT_SENTINEL_NAME>`（必须和 envelope code 字段逐字一致）|
| 真 LLM | `TestLive_<Domain>_<Scenario>` |
| 测试文件 | `<scope>_pipeline_test.go`（明示 pipeline tag 守门）|

### 2.3 §S12 例外条款（追加）

CLAUDE.md §S12 当前规定"平铺禁子目录"，例外允许 `app/tool/` 按家族嵌套。本 overhaul 在 §S12 追加一条例外：

> **§S12 例外（追加）**：`backend/test/` 按测试 axis 嵌套子目录（api / cross / sse / lifecycle / errcodes / live / smoke）。理由：每子目录有独立词汇体系（axis 定义），且 ≥10 文件，满足拆子包标准。

---

## 3. Harness 拆分

### 3.1 API 设计原则

- **方法**（`h.Foo`）：访问 Harness 内部状态（DB / HTTP server / SSE bus）—— Seed* / PostJSON / SubEventLog / WaitFor*
- **自由函数**（`harness.Foo`）：纯逻辑工具，不依赖 Harness 状态 —— Extract* / CtxAs

**testcode 长这样**：
```go
//go:build pipeline

// covers: POST /api/v1/functions (happy)
func TestFunction_Create_Happy_V1Active(t *testing.T) {
    t.Parallel()
    h := harness.New(t)
    user := h.SeedUser(t)
    h.SeedAPIKey(t, user.ID, "deepseek", "fake-key")
    h.SeedModelConfig(t, user.ID, "chat", "deepseek", "deepseek-chat")

    var resp struct{ Data struct{ ID string } }
    status := h.PostJSON(t, "/api/v1/functions", map[string]any{
        "name": "add_two",
        "code": "def add_two(a, b): return a + b",
    }, &resp, harness.WithCtx(h.CtxAs(user.ID)))

    if status != 201 {
        t.Fatalf("status=%d, want 201", status)
    }
    blocks := h.WaitForFunctionEnvReady(t, resp.Data.ID, 10*time.Second)
    text := harness.ExtractTextFromBlocks(blocks)
    // ... 业务断言
}
```

### 3.2 文件分家清单

| 文件 | 行数（估）| 核心导出 |
|---|---|---|
| `harness.go` | ~400 | `Harness` struct / `New(t, opts...) *Harness` / `Option` / `WithFakeLLMBaseURL(url)` / `WithRealSandbox()` / `WithFakeSandbox()` / 方法 `URL()` `HTTPClient()` `DB()` `Logger()` `Cleanup()` |
| `factories.go` | ~250 | 方法 `SeedUser(t)` / `SeedAPIKey(t, userID, provider, key)` / `SeedModelConfig(t, userID, scenario, provider, modelID)` / `SeedConversation(t, userID, title)` / `SeedFunction` / `SeedHandler` / `SeedWorkflow` / `SeedDocument` / `SeedMemory` / `SeedFakeDeepSeek(t)`（一键起 fake server + 配 apikey + 配 model） |
| `http.go` | ~150 | 方法 `DoRequest(t, method, path, body, out, opts...) int` / `PostJSON` / `GetJSON` / `Patch` / `Delete` / `UploadFile(t, filename, mime, data) string` / option `WithCtx(ctx)` |
| `sse.go` | ~480 | 方法 `SubEventLog(t, userID) *SSESub` / `SubNotifications(t, userID)` / `SubForge(t, userID)` / `SSESub` + 方法 `WaitForEvent` / `Drain` / `Close` |
| `db.go` | ~80 | 方法 `DBCount(t, table, where, args...) int64` / 泛型 `QueryRow[T any](t, sql, args...) T` / `MustQueryRow[T any]` |
| `assertions.go` | ~150 | 自由 `ExtractTextFromBlocks` / `ExtractToolCallByName` / `ExtractToolResultByCallID` + 方法 `WaitForFinalAssistant` / `WaitForFunctionEnvReady` / `WaitForFlowRunStatus` + 自由 `AssertErrCode` / `AssertBlockType` / `AssertSeqMonotonic` / `AssertNotificationOfType` / `AssertGolden` |
| `ctx.go` | ~30 | 自由 `CtxAs(userID)` / `DefaultCtx()` / 常量 `DefaultUserID` |
| `live_gate.go` | ~50 | 自由 `RequireDeepSeekKey(t)` / `RequireSandboxResources(t)` / `RequireForgifyDevResources(t)` |
| `fake_llm.go` | ~317 | `FakeLLMServer` + `NewFakeLLMServer(t)` + 方法 `URL` / `PushScript` / `PushDefault` / `LastMessages`（保留现状）|
| `fake_llm_scripts.go` | ~118 | `ScriptText` / `ScriptReasoningThenText` / `ScriptSingleToolCall` / `ScriptParallelToolCalls` / `ScriptToolCallThenText` / `ScriptStreamThenAbort` / `ScriptStreamThenSlow`（保留）|
| `test_registry.go` | ~44 | LLM tool 注册表 inspector（保留）|

### 3.3 Drift Killer：`ctx.go`

历次 drift 都是 ctx 相关工具被改坏。本 overhaul 立**测试自管常量**原则：

```go
// harness/ctx.go
//go:build pipeline

package harness

import (
    "context"
    reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// DefaultUserID is the canonical local user ID used across pipeline tests.
//
// DefaultUserID 是 pipeline 测试公用的本地用户 ID。
const DefaultUserID = "u_pipeline_default_00000000"

// CtxAs returns a context stamped with the given userID via reqctxpkg.SetUserID.
//
// CtxAs 用 reqctxpkg.SetUserID 返回打了指定 userID 的 ctx。
func CtxAs(userID string) context.Context {
    return reqctxpkg.SetUserID(context.Background(), userID)
}

// DefaultCtx is CtxAs(DefaultUserID); reads anchor across tests.
//
// DefaultCtx 等价 CtxAs(DefaultUserID)，是跨测试默认 ctx。
func DefaultCtx() context.Context { return CtxAs(DefaultUserID) }
```

**为什么这能根治 drift**：
1. 旧 `reqctxpkg.DefaultLocalUserID` 是后端代码暴露给测试的洞——后端删它是对的
2. `harness.DefaultUserID` 是测试自己定义、自己消费的常量，与后端解耦——后端 reqctxpkg 怎么改都不影响测试
3. `LocalCtxAs` 散在 helpers.go 是历史包袱——上升为顶级 `CtxAs`，且和 `DefaultUserID` 放一个文件，命运绑定

---

## 4. 测试 inventory（完整清单）

### 4.1 smoke/（2 测试）
- `TestSmoke_Boot_HarnessAssemblesAllServices`
- `TestSmoke_Health_ReturnsOK`

### 4.2 api/（14 子包，~95 测试）

每 domain happy + 主错路径 + cursor（如适用）+ 跨用户隔离（≥1 端点）。完整清单见 §3 brainstorm session 输出，按 §6 命名规则一致。代表性条目：

**api/apikey/**（10 测试）：Create_Happy / Create_InvalidProvider_Returns400 / Create_BaseURLRequiredForCustom_Returns400 / List_Happy_CrossUserIsolation / Get_NotFound_Returns404 / Patch_Happy / Patch_NotFound_Returns404 / Delete_Happy_SoftDelete / Test_OkFakeProvider_Returns200 / Test_FailedConnectivity_Returns422

**api/chat/**（8 测试）：Send_SimpleText_StreamingToCompleted / Send_MissingModelConfig_Returns422 / Send_MissingAPIKey_Returns422 / Send_LLMBuildFailure_Returns422 / Send_LLMMidStream401_StatusErrorWithCode / Cancel_MidStream_DBCancelled / Send_StreamInProgress_Returns409 / AutoTitle_FirstTurnEmitsNewTitle

**api/function/**（12 测试）：Create_Happy_V1Active / Create_DuplicateName_Returns409 / List_Cursor / Get_NotFound_Returns404 / Update_Pending / AcceptPending_EnvReady_Returns200 / RejectPending_DraftDeletes / Run_ReturnsOutput / Run_PythonError_Returns422 / Delete_SoftDelete / Versions_List / Executions_List_FiltersByKind

**api/handler/**（10 测试）：Create_Happy / Get_Happy_ContainsConfigState / AcceptPending_Happy / Config_Get_Unconfigured / Config_Update_PartialThenReady / Config_Clear_RevertsToUnconfigured / Call_UnconfiguredReturns422 / Call_ConfiguredHappy / Calls_List_FiltersByMethod / Delete_SoftDelete

**api/workflow/**（9 测试）：Create_Happy / Edit_AddNode_Pending / Edit_CycleRejection_Returns422 / CheckCapabilities_MissingFunction_Returns422 / CheckCapabilities_HandlerUnconfigured_Returns422 / AcceptPending_Happy / Run_Happy_ReturnsFlowRunID / Versions_List / Delete_SoftDelete

**api/flowrun/**（5 测试）/ **api/document/**（7 测试）/ **api/memory/**（6 测试）/ **api/skill/**（6 测试）/ **api/mcp/**（7 测试）/ **api/model/**（4 测试）/ **api/relation/**（3 测试）/ **api/user/**（4 测试）/ **api/conversation/**（6 测试）

### 4.3 cross/（11 文件，~55 测试）

| 文件 | 测试数（估）| 主题 |
|---|---|---|
| chat_trinity_pipeline_test.go | 12 | chat 调 trinity 9 LLM tool + parallel/serial execution_group |
| workflow_scheduler_pipeline_test.go | 10 | workflow 执行：trigger / approval / loop / parallel / cancel / variable / retry / timeout / rehydrate |
| mention_document_pipeline_test.go | 4 | @-mention 解析 + 快照入 message attrs |
| catalog_consistency_pipeline_test.go | 5 | function/handler/skill/mcp 都登记 catalog + 删后清 |
| subagent_pipeline_test.go | 5 | Explore/Plan/general-purpose 跑通 + 嵌套抑制 + timeout |
| isolation_pipeline_test.go | 5 | 跨用户隔离 sweep + SQL injection 抗性 |
| compaction_memory_pipeline_test.go | 5 | compaction 3 路径 + memory 跨对话写读 |
| askai_iterate_pipeline_test.go | 5 | iterate × function/handler/workflow/document + triage flowrun |
| eventlog_aggregation_pipeline_test.go | 5 | seq monotonic / parentId 不悬空 / status 单向流转 / 重连 / 410 |
| relation_sync_pipeline_test.go | 4 | 4 个 RelationKind 自动 sync + delete 清边 |
| tool_framework_pipeline_test.go | 4 | permission / NeedsReadFirst / 标准字段剥除 / execution_group dispatch |

### 4.4 sse/（3 文件，~27 测试）

**eventlog**（9 测试）：每 7 block type × 主流转 + message frame + per-user scope
**notifications**（13 测试）：13 种 type sweep（conversation / function / handler / workflow / flowrun / mcp_server / skill / memory / todo / sandbox_env / compaction / ask×2）
**forge**（5 测试）：4 events × function/handler/workflow + env_attempt + per-user scope

### 4.5 lifecycle/（4 文件，~20 测试）

**function_env**（6）：draft_to_accept_to_ready / deps_parse_failure / lru_n3 / revert_to_evicted / delete_destroys_envs / multi_package_import
**handler_config**（5）：unconfigured_to_partial_to_ready / live_instance_init_validated / call_serialized / instance_crash_recovery / config_clear_reverts
**workflow_dag**（6）：13_node_types_smoke / retry_then_fatal / pause_resume / timeout / nested_container / rehydrate_on_boot
**sandbox_bootstrap**（3）：mise_embed_unpacks / bootstrap_idempotent / destroy_env

### 4.6 errcodes/（1 文件，~40 测试）

每 errmap.go 登记的 sentinel 一行测试。完整清单以 `backend/internal/transport/httpapi/<domain>/errmap.go` 各文件的 `errTable` 为权威。代表性：API_KEY_NOT_FOUND / CONVERSATION_NOT_FOUND / STREAM_IN_PROGRESS / LLM_PROVIDER_ERROR / FUNCTION_ENV_FAILED / WORKFLOW_CYCLE / ... ~40 条

每测试格式机械化：
```go
// covers: errcode:API_KEY_NOT_FOUND
func TestErrCode_API_KEY_NOT_FOUND(t *testing.T) {
    t.Parallel()
    h := harness.New(t)
    user := h.SeedUser(t)
    var env harness.ErrEnvelope
    status := h.GetJSON(t, "/api/v1/api-keys/aki_nonexistent", &env, harness.WithCtx(h.CtxAs(user.ID)))
    harness.AssertErrCode(t, status, 404, env, "API_KEY_NOT_FOUND")
}
```

### 4.7 live/（4 文件，~12 测试）

**reasoner**（2）：DeepSeek-R1 reasoning_content 分流 + token 计数
**multi_step_react**（3）：read_process_write_full / parallel_calls_resolve / max_steps_exit
**chat_trinity**（3）：create_function_real_LLM / edit_handler_real_LLM / create_workflow_real_LLM
**wire_protocol**（3）：OpenAI SSE chunk 字段 sanity / Anthropic message_delta 格式 / OpenAI tool_call chunk assembly

### 4.8 总计

| 目录 | 文件 | 测试 |
|---|---|---|
| smoke/ | 1 | 2 |
| api/ | 14 | ~95 |
| cross/ | 11 | ~55 |
| sse/ | 3 | ~27 |
| lifecycle/ | 4 | ~20 |
| errcodes/ | 1 | ~40 |
| live/ | 4 | ~12 |
| **合计** | **38** | **~251** |

---

## 5. 测试规范

### 5.1 失败信息约定

- 所有 helper / 断言函数必须 `t.Helper()`
- 失败信息必含可调试上下文（path / body / 关键变量）
- 共享断言函数集中在 `harness/assertions.go`，覆盖常见模式：
  - `AssertErrCode(t, status, wantStatus, env, wantCode)`
  - `AssertBlockType(t, blocks, wantType, wantStatus)`
  - `AssertSeqMonotonic(t, events)`
  - `AssertDBRow[T](t, h, table, where, args, check)`
  - `AssertNotificationOfType(t, sub, type, timeout)`

模板示例：
```go
t.Fatalf("AcceptPending(funcID=%s): status=%d, want 422; envelope.code=%q want %q; body=%s",
    funcID, status, env.Error.Code, "FUNCTION_ENV_FAILED", string(rawBody))
```

### 5.2 `// covers:` annotation（强制 + 矩阵工具消费）

每个 pipeline 测试函数必须在签名上方注释 `// covers:` 行。Annotation 词汇封闭枚举：

| 前缀 | 含义 | 例 |
|---|---|---|
| `<METHOD> /<path> [(modifier)]` | HTTP 端点 | `POST /api/v1/functions (happy)` / `GET /api/v1/functions/{id} (not_found_404)` |
| `errcode:<CODE>` | 错误码 sentinel | `errcode:FUNCTION_ENV_FAILED` |
| `sse:<stream>:<event>[:<blocktype>]` | SSE 协议 | `sse:eventlog:block_delta:text` |
| `cross:<seam_id>` | 跨 domain 锈川（id 见 seams.yaml）| `cross:chat_trinity:create_function` |
| `lifecycle:<chain_id>` | 长链路（id 见 seams.yaml）| `lifecycle:function_env:lru_n3_eviction` |

多 annotation 允许：

```go
// covers: POST /api/v1/conversations/{id}/messages:send (happy)
// covers: sse:eventlog:message_start
// covers: sse:eventlog:block_start:text
// covers: sse:eventlog:block_delta
// covers: sse:eventlog:block_stop
// covers: sse:eventlog:message_stop
func TestChat_Send_SimpleText_StreamingToCompleted(t *testing.T) { ... }
```

### 5.3 testdata/ 布局

每个测试目录可有 `testdata/`（Go `go test` 自动排除编译）：

```
backend/test/
├── harness/testdata/
│   ├── fake_llm_chunks/         FakeLLMServer 用预录 SSE chunk 模板
│   └── sample_python/           典型 Python code 片段
├── api/function/testdata/       request body json fixtures
├── lifecycle/testdata/          workflow_dag_*.json 等大固定数据
└── live/testdata/               golden 文件
```

### 5.4 Golden files（snapshot）

`harness.AssertGolden(t, "name.golden.json", got)`：
- 文件存在 → 比对，不一致 fatal + 输出 diff
- 文件不存在 → fatal "golden missing; run with -update to create"
- `-update` flag → 写文件而非比对

Golden 文件路径：`testdata/golden/<test-name>.golden.json`。

### 5.5 `-race` 默认

所有 pipeline 跑 `-race`。Makefile target 内已注入。

### 5.6 `t.Parallel()` 政策

| 测试类型 | 可 Parallel? | 原因 |
|---|---|---|
| api/、errcodes/、sse/ | ✅ 应 Parallel | 各 test 独立 `harness.New(t)`，互不冲突 |
| cross/ | ✅ 应 Parallel | 同上 |
| lifecycle/ | ❌ **禁** Parallel | 真 sandbox 共享 mise 目录 / uv 缓存 |
| live/ | ❌ **禁** Parallel | 真 LLM API 限流 |
| smoke/ | ❌ 不必 | 仅 2 测试 |

### 5.7 `t.Cleanup()` 规范

- 所有 close / shutdown / destroy 用 `t.Cleanup()`
- 禁 `defer`（panic 安全性 + sub-test 隔离差）
- `harness.New(t)` 内部已 `t.Cleanup(h.shutdown)`

### 5.8 Build tag 策略

单一 `pipeline` tag + 运行时 gate：

```go
//go:build pipeline

func TestLifecycle_Function_DraftToAcceptToReady_FullChain(t *testing.T) {
    harness.RequireSandboxResources(t)  // 运行时 gate
    h := harness.New(t)
    // ...
}
```

不分 sub-tag（避免组合爆炸）。

### 5.9 Deterministic ID 策略

- **fake LLM 脚本** 用固定 ToolID（如 `tc_test_001`），便于断言
- **harness 工厂** 用真实 `idgen.New(prefix)` 不固定（覆盖真 ID 生成路径，§S15 检测）

---

## 6. 覆盖矩阵自动化工具

### 6.1 工具位置与结构

`backend/cmd/coverage-matrix/`（同 lintprompts，不是 internal/cmd）：

```
backend/cmd/coverage-matrix/
├── main.go                      ~100 行  CLI flag 解析 + 主流程
├── endpoint_scanner.go          ~150 行  AST 扫所有 router 注册的 HTTP 端点
├── errcode_scanner.go           ~120 行  AST 扫所有 errmap.go::errTable 条目
├── sse_truth.go                 ~80 行   SSE 协议封闭枚举（按 E1 hardcode + 扫 notifications type 词表）
├── seams.yaml                   ~120 行  手维 cross/lifecycle seam 清单
├── covers_parser.go             ~100 行  扫 backend/test/** 的 `// covers:` annotation
├── matcher.go                   ~80 行   truth × test annotation 双向匹配
├── renderer.go                  ~150 行  matrix markdown 输出
├── validator.go                 ~80 行   strict mode 校验所有 annotation 指向真目标
└── coverage_matrix_test.go      ~100 行  工具自己的单测
```

总规模 ~1080 行 Go。

### 6.2 真值源（truth sources）

5 处扫描：

**A. HTTP 端点**：扫 `backend/internal/transport/httpapi/router/*.go` + 各 `handlers/<domain>/*.go` 的 `Register` 函数。AST 模式：识别 `mux.HandleFunc("METHOD /path", ...)` 类似调用。

```go
type Endpoint struct {
    Method     string
    Path       string
    HandlerRef string  // file:line
}
```

**B. 错误码**：扫每个 `backend/internal/transport/httpapi/<domain>/errmap.go` 的 `errTable` 表。

```go
type ErrCode struct {
    Code       string
    HTTPStatus int
    Sentinel   string
    Source     string
}
```

**C. SSE 事件**：按 CLAUDE.md §E1 + `events-design.md` hardcode 3 流；notifications 词表额外做一次扫描验证。

**D. Cross-domain seams（手维 yaml）**：

```yaml
# seams.yaml
cross:
  - id: chat_trinity:search_function
    description: "chat 通过 search_function tool 列函数（不写 execution_log）"
  ...

lifecycle:
  - id: function_env:draft_to_accept_to_ready
    description: "Function lifecycle: draft → fake LLM stream code → AST 解析 → AcceptPending → uv sync → ready"
  ...
```

**E. 测试 annotation**：扫 `backend/test/**/*_pipeline_test.go` 的 `// covers:` 注释行。

### 6.3 输出格式

**README 矩阵段**：用 marker 包起来工具只改其间内容：

```markdown
<!-- COVERAGE-MATRIX:START (generated by `make matrix`; do not edit) -->

> Generated 2026-XX-XX by `make matrix`. Run `make audit` to verify.

## 1. HTTP endpoints (67 / 72 covered, 93%)
### apikey domain (10/10 ✅)
| Method | Path | Test |
|---|---|---|
| POST | /api/v1/api-keys | api/apikey/...::TestAPIKey_Create_Happy |
| ...

## 2. Error codes (40 / 40 covered, 100% ✅)
| Code | HTTP | Test |
| ...

## 3. SSE protocol (27 / 27 covered, 100% ✅)
...

## 4. Cross-domain seams (11 / 11 covered, 100% ✅)
...

## 5. Lifecycle chains (15 / 15 covered, 100% ✅)
...

## Summary
- Total targets: 165
- Covered: 160 (97%)
- Uncovered: 5
- Tests: 251 across 38 files

### Uncovered list (for follow-up)
- chat domain: POST /api/v1/conversations/{id}/messages/{msgId}:cancel (no test)
- ...

<!-- COVERAGE-MATRIX:END -->
```

**stdout summary**（`make matrix` 末尾自动 + `--report` 单独触发）：

```
Coverage Matrix Summary
=======================
HTTP endpoints     67/72   93% ⚠️
Error codes        40/40  100% ✅
SSE protocol       27/27  100% ✅
Cross-domain       11/11  100% ✅
Lifecycle          15/15  100% ✅
─────────────────────────────────
Total             160/165  97%
```

### 6.4 CLI

```
coverage-matrix [--update | --check | --report] [--strict] [--root <path>]

  --update    Regenerate README matrix section in place. (default)
  --check     Regenerate to temp; diff vs current README; exit 1 if differs.
  --report    Print stdout summary; exit 0 regardless.
  --strict    With --check, also fail on any uncovered target / orphan annotation.
  --root      Project root override.
```

### 6.5 Strict mode 规则（`--check --strict` = `make audit`）

违反任一 → 退出 1：
1. **README 矩阵段不新鲜**（再生成 ≠ 当前文件）
2. **任何 truth 目标 0 测试覆盖**（endpoint / errcode / sse event / seam / lifecycle）
3. **任何 `// covers:` annotation 指向不存在的 truth**（orphan）
4. **任何 `func Test*` in `backend/test/` 缺 `// covers:` annotation**

### 6.6 工具自己的 drift 防护

- 工具自带 `coverage_matrix_test.go` 单测（不带 pipeline tag，普通 `go test`）
- scanner 失败时 fatal not silent —— AST 模式不匹配时 panic 输出诊断
- `make verify` 跑 `make audit`，scanner 抓不到端点会立刻显形（数量异常下降）

---

## 7. Makefile 重构（22 单字 target）

### 7.1 完整 target 清单

```
一次性
  setup       装全部依赖
  mise        下载 mise binary

日常应用
  dev         起 desktop app
  stop        杀进程

日常测试
  unit        Go 单测（原 test-backend）
  web         vitest 前端单测（原 test-frontend）
  test        unit + web 聚合
  lint        前端 eslint + tsc + steiger（原 lint-frontend）
  mock        pipeline fake LLM 测试 ◀ 新 daily driver

Pipeline 测试加档
  sandbox     mock + 真 sandbox lifecycle
  live        only 真 LLM 测试（烧 token）
  e2e         full pipeline（mock → sandbox → live 串行）
  cover       生成 HTML coverage 报告

矩阵
  matrix      生成 README 矩阵段 + 打印摘要
  audit       矩阵严格检查（verify 内部调用；失败 = stale / uncovered / orphan）

发布
  verify      pre-push gate（vet×5 + build×5 + lintprompts + audit + mock）
  build       打 macOS .app

QA / Misc
  smoke       Playwright 前端走查
  testend     旧调试控制台
  clean       清 dev 数据
  reset       工厂重置
  help        列所有 target
```

**总计 22 个 target**。

### 7.2 改名映射（干净切换 / 无 alias）

| 旧 | 新 |
|---|---|
| test-backend | unit |
| test-frontend | web |
| lint-frontend | lint |

无 alias 保留——彻底干净。

### 7.3 新增 target 内容

```makefile
# 日常 driver。fake LLM。no env vars。<60s。
mock:
	$(AUTO_DEVBOX)
	@cd backend && go test -count=1 -race -tags=pipeline -p 1 -timeout=5m \
		./test/smoke/... ./test/api/... ./test/cross/... ./test/sse/... ./test/errcodes/...

# 加真 sandbox。要 FORGIFY_DEV_RESOURCES（缺则 t.Skip）。不烧 token。
sandbox: mock
	$(AUTO_DEVBOX)
	@$(LOAD_ENV) cd backend && go test -count=1 -race -tags=pipeline -p 1 -timeout=10m \
		./test/lifecycle/...

# 只跑真 LLM 测试。要 DEEPSEEK_API_KEY（缺则 t.Skip）。BURNS TOKENS.
live:
	$(AUTO_DEVBOX)
	@$(LOAD_ENV) cd backend && go test -count=1 -race -tags=pipeline -p 1 -timeout=10m \
		./test/live/...

# 全套：mock → sandbox → live 串行 fail-fast。Release gate。
e2e: mock sandbox live

# HTML coverage 报告。
cover:
	$(AUTO_DEVBOX)
	@mkdir -p coverage
	@cd backend && go test -count=1 -tags=pipeline -p 1 -timeout=5m \
		-coverprofile=../coverage/pipeline.out -covermode=atomic \
		-coverpkg=./internal/... \
		./test/smoke/... ./test/api/... ./test/cross/... ./test/sse/... ./test/errcodes/...
	@go tool cover -html=coverage/pipeline.out -o coverage/pipeline.html
	@echo "Coverage report: coverage/pipeline.html"

# 生成 / 刷新 README 矩阵段 + stdout 摘要。
matrix:
	@cd backend && go run ./cmd/coverage-matrix --update

# 矩阵严格检查（verify 内部调用）。
audit:
	@cd backend && go run ./cmd/coverage-matrix --check --strict
```

### 7.4 verify target 修改

```makefile
verify: vet build-all lintprompts audit mock
```

（注：vet / build-all / lintprompts 现有实现保持；新加 audit + mock 进 gate）

### 7.5 help target 更新

```makefile
help:
	@echo "Forgify"
	@echo ""
	@echo "Once:    make setup    install all dependencies"
	@echo "         make mise     download mise binaries (one-time)"
	@echo ""
	@echo "Daily:   make dev      run the desktop app"
	@echo "         make stop     kill anything we started"
	@echo "         make unit     Go unit tests"
	@echo "         make web      vitest frontend tests"
	@echo "         make test     unit + web aggregate"
	@echo "         make lint     frontend lint (eslint + tsc + steiger)"
	@echo "         make mock     pipeline fake LLM tests (~60s, no tokens)"
	@echo "         make clean    wipe dev data"
	@echo "         make reset    factory reset"
	@echo ""
	@echo "Pipeline:make sandbox  mock + real sandbox (FORGIFY_DEV_RESOURCES)"
	@echo "         make live     real LLM tests only (BURNS TOKENS)"
	@echo "         make e2e      full pipeline mock+sandbox+live"
	@echo "         make cover    HTML coverage report"
	@echo "         make matrix   regenerate README coverage matrix"
	@echo "         make audit    strict matrix check (used by verify)"
	@echo ""
	@echo "Ship:    make build    package macOS .app"
	@echo "         make verify   pre-push gate (vet/build/lint/audit/mock)"
	@echo ""
	@echo "QA:      make smoke    playwright frontend walk"
	@echo ""
	@echo "Misc:    make testend  legacy debug console"
```

---

## 8. 迁移计划（7 Phase）

每 Phase 独立 commit + push。每 Phase 完工 `make verify` 必须绿。原子可回滚。

### Phase 0：Makefile 单字化 + 3 处 compile fix（~1.5h）

**目标**：Makefile 立刻清爽 + `make e2e` 不再编译挂。

**改动**：
- `Makefile`：22 target 全部按 §7 重写
- 新加 target `mock` / `sandbox` / `live` / `cover`（实质内容）
- `matrix` / `audit` target 暂用 echo 占位（工具 Phase 4 实现）
- `verify` 加 `mock`，**暂不加 `audit`**（工具未实现）
- `e2e` 改为聚合 `mock sandbox live`
- `backend/test/document/workflow_attach_test.go:33`：`LocalCtxAs(t, "x")` → `LocalCtxAs("x")`
- `backend/test/scheduler/approval_e2e_test.go:16`：同上
- `backend/test/workflow/workflow_test.go:138`：同上 + `reqctxpkg.DefaultLocalUserID` → `harness.DefaultUserID`
- `backend/test/harness/ctx.go`：**新建**，含 `CtxAs` + `DefaultUserID` 常量（Phase 1 才正式拆 harness，但 Phase 0 先把这个文件落地以提供 `DefaultUserID`）

**验收**：
- `make unit` 绿
- `make web` 绿
- `make lint` 绿
- `make verify` 绿（含 mock 跑通；mock 跑现有 26 个 domain 测试包能编通就算 mock 绿）
- `make e2e` 不再编译挂（live / sandbox 因缺 env 优雅 skip，mock 实跑）

**Commit**：`chore: Makefile 单字化重命名 + 修 3 处 pipeline harness drift`

### Phase 1：harness/ 拆 11 文件（~2h）

**目标**：harness/ 清晰分家，向后兼容。

**改动**：按 §3.2 表格拆分。`helpers.go` 保留 alias（`LocalCtxAs = CtxAs` 等）到 Phase 3 删。

**验收**：`make verify` 绿；现有 26 包零代码改动仍 work。

**Commit**：`refactor(test/harness): 拆 11 文件分家职责 + 保留向后兼容 alias`

### Phase 2：测试按 axis 重组（~4h）

**目标**：26 包搬到新结构 + 改名 `_pipeline_test.go`。

**机械搬家**（用 Edit 工具，禁 sed/git mv）：
- `test/<domain>/<domain>_test.go` → `test/api/<domain>/<domain>_pipeline_test.go`（14 个 domain）
- `test/cross/errcodes_test.go` 暂留 cross/（Phase 4 拆到独立 errcodes/）
- `test/cross/isolation_test.go` → `test/cross/isolation_pipeline_test.go`
- `test/scheduler/scheduler_test.go` → `test/cross/workflow_scheduler_pipeline_test.go`
- `test/scheduler/approval_e2e_test.go` → `test/lifecycle/workflow_dag_pipeline_test.go`（部分）
- `test/document/workflow_attach_test.go` → `test/cross/mention_document_pipeline_test.go`
- `test/integration/d9_test.go` → 拆分到对应 axis
- 其他 domain 测试逐一归位

每搬 5-10 文件 `cd backend && go build ./...` 跑一次确认。

**验收**：`make mock` 绿 / `make e2e` 绿 / `make verify` 绿

**Commit**：`refactor(test): 按 axis 重组 26 测试包到 api/cross/sse/lifecycle/errcodes/live`

### Phase 3：删 harness 向后兼容 alias（~1h）

**改动**：
- 删 `backend/test/harness/helpers.go` 中的 `LocalCtxAs = CtxAs` 等 alias
- Grep 所有旧调用，改成新方法
- `harness/helpers.go` 删档或缩到 0 行

**验收**：`make verify` + `make e2e` 绿

**Commit**：`chore(test/harness): 删向后兼容 alias，全代码改用新方法`

### Phase 4：写 coverage-matrix 工具（~6h）

**改动**：
- 新建 `backend/cmd/coverage-matrix/` 10 文件（按 §6.1）
- 含 `seams.yaml`（cross/lifecycle id 清单，~60 个 id）
- 工具自带 `coverage_matrix_test.go` 单测
- `backend/test/README.md` 加 marker
- `Makefile`：`matrix` / `audit` target 改为真实调用工具
- `Makefile`：`verify` 加 `audit` 进 gate（注意：因 Phase 5 才填 annotation，**此 Phase 末把 audit 设为 warn-only**——`make audit` 输出"X uncovered, run `make matrix` to update README" 但 exit 0）

**验收**：
- `make matrix` 生成 README 矩阵段
- `make audit` 检查不挂（warn-only）
- 工具单测全绿

**Commit**：`feat(cmd/coverage-matrix): 自动覆盖矩阵生成工具 + seams.yaml + README 占位`

### Phase 5：补 `// covers:` annotation + 补全新测试（~12h）

**5a. annotation backfill**（先做）：
- 现有 26 个 domain 测试每个函数加 `// covers:` 行
- `make matrix` 跑过看 baseline

**5b. 新测试补全**（按 §4 inventory）：
- `errcodes/sweep_pipeline_test.go` —— ~40 测试，模板化批量
- `sse/` 三流协议测试 —— ~27 测试
- `cross/` 11 文件 ~55 测试
- `lifecycle/` 4 文件 ~20 测试
- `live/` 4 文件 ~12 测试
- `api/` 缺口补足到全端点 happy + 主错

**5c. 矩阵 audit 切到 strict**：
- `Makefile`：`audit` 加 `--strict` flag
- `make verify` 跑过 = 真正"全 covered + 无 orphan"

**验收**：
- `make matrix` 显示接近 100% covered
- `make audit` strict 通过
- `make mock` ~60s 全绿
- `make e2e` 全绿（含 sandbox + live）

**Commit**（分子 commit）：
- `test(errcodes): 全 sentinel sweep（~40 测试）`
- `test(sse): 3 流协议 round-trip 测试（~27 测试）`
- `test(cross): 跨 domain 锈川测试（~55 测试，11 文件）`
- `test(lifecycle): 长链路真 sandbox 测试（~20 测试）`
- `test(live): 真 LLM 测试（~12 测试）`
- `test(api): 补全单 domain 端点测试至完整 happy + 主错`
- `feat(test): coverage matrix --strict 启用 + audit 进 verify gate`

### Phase 6：文档同步（~1.5h）

**改动**（按 §9）：
- CLAUDE.md：§T 加 T7-T11 / §S12 例外 / §S14 触发表新增行 / 开发期工具纪律段加 audit / 测试基线段更新
- `documents/version-1.2/backend-design.md` Verification 段
- `documents/version-1.2/service-contract-documents/*.md` 加 footer "矩阵自动生成见 backend/test/README.md"
- `documents/version-1.2/progress-record.md` dev log 1-2 句

**Commit**：`docs: §S14/§F1 同步 — pipeline 测试 overhaul 完工`

### 总览

| Phase | 标题 | 预估 | 累计 |
|---|---|---|---|
| 0 | Makefile 单字化 + 3 处 compile fix | 1.5h | 1.5h |
| 1 | harness/ 拆 11 文件 | 2h | 3.5h |
| 2 | 测试按 axis 重组 | 4h | 7.5h |
| 3 | 删 harness 向后兼容 alias | 1h | 8.5h |
| 4 | coverage-matrix 工具 | 6h | 14.5h |
| 5 | annotation + 新测试 + audit strict | 12h | 26.5h |
| 6 | 文档同步 | 1.5h | 28h |
| **合计** | | | **~28h ≈ 3-4 工作日** |

---

## 9. 文档同步（§S14 / §F1 联动）

### 9.1 CLAUDE.md 改动

**§T 系列新增**：

```markdown
- **T7** Pipeline 测试约定：每个 pipeline 测试函数必须 `// covers:` annotation；
        命名 `Test<Domain>_<Scenario>`；文件 `<scope>_pipeline_test.go`。
- **T8** Harness API 单一入口：所有 test 通过 `harness.New(t)` 启 DI 图；
        ctx 通过 `harness.CtxAs(userID)` 构造；禁直接调 `reqctxpkg.SetUserID`。
- **T9** `t.Parallel()` 政策：api/cross/sse/errcodes 应 Parallel；
        lifecycle/live **禁** Parallel。
- **T10** `t.Cleanup()`：所有 close/shutdown 用 `t.Cleanup`，禁 `defer`。
- **T11** Pipeline 测试用 `-race` 跑：`make verify` 已强制。
```

**§S12 例外追加**：

```markdown
**§S12 例外（追加）**：`backend/test/` 按测试 axis 嵌套子目录
（api / cross / sse / lifecycle / errcodes / live / smoke）。
理由：每子目录有独立词汇体系，且 ≥10 文件，满足拆子包标准。
```

**§S14 触发表新增行**：

```markdown
| 新 HTTP endpoint | + 测试 + `// covers:` annotation + `make matrix` 自动 |
| 新 sentinel（进 errmap） | + errcodes/sweep 加测试 + annotation + `make matrix` |
| 新 SSE event 类型 | + sse/ 加测试 + annotation + 改 sse_truth.go + `make matrix` |
| 新跨 domain 锈川 | + seams.yaml 加 id + cross/ 加测试 + annotation + `make matrix` |
```

**"开发期工具纪律" 段新增**：

```markdown
- **`make audit` 提交前必跑**（已在 `make verify` 内）：矩阵新鲜 + 全 covered + 无 orphan annotation
- **`make mock` 日常 driver**（`make verify` 已含）：~60s pipeline 测试，零外部依赖
```

**"测试基线" 段重写**：

```markdown
- **测试基线**：`make verify`（含 `mock`）全绿——~60s 离线 pipeline 测试；
  `make sandbox` 加真 sandbox 测试（要 `FORGIFY_DEV_RESOURCES`）；
  `make live` 跑真 LLM 测试（要 `DEEPSEEK_API_KEY`，烧 token）；
  `make e2e` = mock + sandbox + live 全串。
- **覆盖矩阵**：`make matrix` 自动生成 backend/test/README.md 矩阵段；
  `make audit` 严格检查。
```

### 9.2 backend-design.md 改动

**Verification 段**：

```markdown
### Pipeline 测试
- `make mock`：日常 driver，<60s，fake LLM，无 token
- `make sandbox`：加真 sandbox lifecycle，要 FORGIFY_DEV_RESOURCES
- `make live`：只跑真 LLM 测试，要 DEEPSEEK_API_KEY，烧 token
- `make e2e`：全套，release gate
- `make audit`：矩阵严格检查

### 单测
- `make unit`：Go 单测，in-memory SQLite
- `make web`：vitest 前端单测
- `make test`：unit + web 聚合

### Coverage
- `make cover`：生成 HTML 报告到 coverage/pipeline.html
- 覆盖矩阵自动生成见 backend/test/README.md
```

### 9.3 service-contract-documents/*.md

各 contract 文档（api-design.md / error-codes.md / events-design.md）末尾加 footer：

```markdown
---
**覆盖矩阵自动生成见 `backend/test/README.md`**。文档不再手维 endpoint × test 映射。
```

### 9.4 progress-record.md dev log

按 §S19 格式（1-2 句 ~30-100 汉字），完工时加一条：

```
2026-XX-XX | [overhaul] backend/test/ 全面 overhaul：harness 拆 11 文件 / 测试按
            axis 重组（api/cross/sse/lifecycle/errcodes/live，~251 测试 / 38 文件）/
            写 coverage-matrix 工具 + audit 进 verify gate / make 22 target 单字化。
            矩阵 100% covered。spec 见 adhoc/test-pipeline-iteration-documents/02-overhaul.md。
```

---

## 10. 并行 agent 协调（worktree）

### 10.1 隔离方案

本 overhaul 在独立 worktree 短命 topic branch 跑，与 testend 完整重写并行：

```bash
git worktree add ../Forgify-e2e -b e2e-overhaul main
cd ../Forgify-e2e
# 全部 overhaul 工作在此 worktree
```

### 10.2 冲突面

| 文件 | 风险 | 处理 |
|---|---|---|
| `Makefile` | 🔴 高 | Phase 0 第一步完成 Makefile 单字化，提前 push；testend agent 之后只动 `testend` target 行 |
| `CLAUDE.md` | 🟡 中 | 我加 §T7-T11 / §S12 例外 / §S14 行；testend agent 可能动"Misc"段 |
| `progress-record.md` | 🟡 中 | append 操作，rebase 时通常 auto-merge |
| `testend/**` | ⚪ 零 | 完全不动 |
| `backend/test/**` | ⚪ 零 | testend agent 禁动 |
| `backend/cmd/coverage-matrix/` | ⚪ 零 | 新建，testend agent 禁动 |

### 10.3 rebase 节奏

每 Phase 完工：
```bash
git fetch origin
git rebase origin/main
git push origin e2e-overhaul    # topic branch force-push 允许（短命）
# 然后切回 main 做 fast-forward merge：
cd /path/to/main/worktree
git pull
git merge --ff-only e2e-overhaul
git push origin main
```

### 10.4 给 testend agent 的告知

> 一个并行 session 正在 worktree `../Forgify-e2e` 跑 backend/test/ 全面 overhaul（spec 在
> `documents/version-1.2/adhoc-topic-documents/test-pipeline-iteration-documents/02-overhaul.md`）。
> 你的工作禁区：`backend/test/**`、`backend/cmd/coverage-matrix/`。
> 你需要避免触碰的 shared file：`Makefile`（如必须改 testend target，请只改 testend target 那一段，
> 不动其他 target）。每次写 progress-record.md 时 commit + push 立刻，减少 race。

---

## 11. 风险点 / 失败回滚

1. **Phase 2 大搬家**：26 包重组容易漏 import / 包名不一致。用 Edit 工具逐文件搬，禁 sed/git mv。每 5-10 文件 `cd backend && go build ./...` 跑一次。
2. **Phase 4 工具不出 AST**：endpoint scanner 用 `go/parser` + `go/ast`，参考 lintprompts 代码风格。
3. **Phase 5 太大**：12h 是单次最大块。中途若发现某 axis 测试预想错，**先停下来更新 spec**（per S14），别硬写。
4. **跨 worktree rebase 冲突**：若 testend agent 改了 Makefile 非 testend 行，回归冲突手工解。Phase 0 提前 push 减少这种风险。

---

## 12. 验收清单（done = 全 ✅）

- [ ] `make verify` 绿（含 vet×5 / build×5 / lintprompts / audit / mock）
- [ ] `make e2e` 绿（mock → sandbox → live 全串）
- [ ] `make audit --strict` 通过——0 uncovered / 0 orphan / 0 stale README
- [ ] `backend/test/README.md` 5 分钟讲清"如何加新测试" + 自带覆盖矩阵段
- [ ] `backend/test/harness/` 11 文件每文件 < 200 行（harness.go 例外 ~400）
- [ ] ~251 测试按 axis 分布，命名约定 100% 合规
- [ ] 每个 pipeline 测试函数顶上有 `// covers:` annotation
- [ ] CLAUDE.md §T7-T11 / §S12 例外 / §S14 触发表行 / 测试基线段已更新
- [ ] `documents/version-1.2/progress-record.md` 完工 dev log 已加
- [ ] topic branch `e2e-overhaul` 已 merge 到 main，worktree 已删

---

## 附录 A：当前已知 drift 修复点（Phase 0 必修）

- `backend/test/document/workflow_attach_test.go:33` —— `LocalCtxAs(t, "x")` → `LocalCtxAs("x")`
- `backend/test/scheduler/approval_e2e_test.go:16` —— 同上
- `backend/test/workflow/workflow_test.go:138` —— 同上 + `reqctxpkg.DefaultLocalUserID` → `harness.DefaultUserID`

## 附录 B：seams.yaml 初稿大纲（Phase 4 用）

```yaml
cross:
  - id: chat_trinity:search_function
  - id: chat_trinity:get_function
  - id: chat_trinity:create_function
  - id: chat_trinity:edit_function
  - id: chat_trinity:revert_function
  - id: chat_trinity:delete_function
  - id: chat_trinity:run_function
  - id: chat_trinity:search_function_executions
  - id: chat_trinity:get_function_execution
  - id: chat_trinity:search_handler
  - id: chat_trinity:get_handler
  - id: chat_trinity:create_handler
  - id: chat_trinity:edit_handler
  - id: chat_trinity:revert_handler
  - id: chat_trinity:delete_handler
  - id: chat_trinity:call_handler
  - id: chat_trinity:update_handler_config
  - id: chat_trinity:search_handler_calls
  - id: chat_trinity:get_handler_call
  - id: chat_trinity:search_workflow
  - id: chat_trinity:get_workflow
  - id: chat_trinity:create_workflow
  - id: chat_trinity:edit_workflow
  - id: chat_trinity:revert_workflow
  - id: chat_trinity:delete_workflow
  - id: chat_trinity:trigger_workflow
  - id: chat_trinity:search_workflow_executions
  - id: chat_trinity:get_workflow_execution
  - id: workflow_scheduler:trigger_full_dag
  - id: workflow_scheduler:approval_pause_resume
  - id: workflow_scheduler:loop_iterates_body
  - id: workflow_scheduler:parallel_branches
  - id: workflow_scheduler:cancel_run
  - id: workflow_scheduler:variable_expression
  - id: workflow_scheduler:retry_up_to_max
  - id: workflow_scheduler:timeout_status
  - id: workflow_scheduler:rehydrate_on_boot
  - id: mention_document:single_doc_snapshot
  - id: mention_document:nonexistent_rejected
  - id: mention_document:trinity_entity_snapshot
  - id: mention_document:multi_doc_all_snapshotted
  - id: catalog_consistency:function_registers
  - id: catalog_consistency:handler_registers
  - id: catalog_consistency:skill_registers
  - id: catalog_consistency:mcp_registers
  - id: catalog_consistency:deleted_removed
  - id: subagent:explore_terminates
  - id: subagent:plan_terminates
  - id: subagent:general_purpose_terminates
  - id: subagent:nested_spawn_suppressed
  - id: subagent:timeout_5min_cap
  - id: isolation:cannot_list_other_user
  - id: isolation:cannot_get_other_conv
  - id: isolation:cannot_delete_other_apikey
  - id: isolation:cannot_approve_other_flowrun
  - id: isolation:sql_injection_sanitized
  - id: compaction_memory:soft_downgrades_old
  - id: compaction_memory:hard_summary_archives
  - id: memory:pinned_in_system_prompt
  - id: memory:read_tool_returns
  - id: memory:forget_tombstone
  - id: askai:iterate_function
  - id: askai:iterate_handler
  - id: askai:iterate_workflow
  - id: askai:iterate_document
  - id: askai:triage_flowrun
  - id: eventlog:seq_monotonic
  - id: eventlog:parent_id_no_dangle
  - id: eventlog:status_one_way
  - id: eventlog:last_event_id_reconnect
  - id: eventlog:seq_too_old_410
  - id: relation:create_function_edge
  - id: relation:workflow_uses_edges
  - id: relation:document_links_edge
  - id: relation:delete_purges
  - id: tool_framework:permission_plan_mode
  - id: tool_framework:needs_read_first
  - id: tool_framework:standard_fields_strip
  - id: tool_framework:execution_group_serial

lifecycle:
  - id: function_env:draft_to_accept_to_ready
  - id: function_env:deps_parse_failure
  - id: function_env:lru_n3_eviction
  - id: function_env:revert_evicted_rebuild
  - id: function_env:delete_destroys_envs
  - id: function_env:multi_package_import
  - id: handler_config:unconfigured_to_ready
  - id: handler_config:live_instance_init
  - id: handler_config:call_serialized
  - id: handler_config:instance_crash_recovery
  - id: handler_config:config_clear_reverts
  - id: workflow_dag:13_node_types_smoke
  - id: workflow_dag:retry_then_fatal
  - id: workflow_dag:pause_resume
  - id: workflow_dag:timeout
  - id: workflow_dag:nested_container
  - id: workflow_dag:rehydrate_on_boot
  - id: sandbox_bootstrap:mise_embed_unpacks
  - id: sandbox_bootstrap:bootstrap_idempotent
  - id: sandbox_bootstrap:destroy_env
```
