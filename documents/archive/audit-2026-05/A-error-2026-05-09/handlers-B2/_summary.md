# Package audit summary: transport/httpapi/handlers (B2 — 5 medium files)

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: B2 5 文件中 chat.go / dev_routes.go / eventlog.go / apikey.go **0 violation**——错误路径全部走 envelope helper。dev_info.go **5 EDGE-LOW**——dev-only graceful：`os.UserHomeDir() _` / `entry.Info() continue` / `walkHomeTree _` 等失败时静默跳过，dev tester 体验合理但缺 inline 注释钉住 §S3 例外身份。
- **§S9 detached ctx 终态写**: B2 5 文件 **0 violation**——handler 全部是单步 CRUD / 触发器 / 读路径 / 静态 manifest，r.Context() 是正确选择。chat.go SendMessage 是 stream 触发器（异步 goroutine 在 chatapp.Service 内启动用 detached ctx，归 chat-app audit batch）；apikey.go Test 内部探测写 result 由 apikeyapp.Service.Test 处理（spec extracts §S9 已注 "前一轮已修"）。**任务描述中预期 dev_routes.go 可能有 fire-and-forget 问题——实际不存在，文件是纯静态 manifest**。
- **§S15 ID 生成**: B2 5 文件 **0 violation**——handler 是 transport pure shell，全部 ID 由 service 层 idgenpkg.New(prefix) 生成；handler 仅消费返回值。
- **§S16 错误 wrap 格式**: B2 **5 EDGE-LOW**——chat.go 3 处 `%w: <text>` 形式缺 `<pkg>.<Method>:` 前缀（site 1/2/4），sentinel 链通但缺位置；apikey.go decodeJSON site 8 同问题（B1 _summary 已识别 cross-cutting）；dev_info.go site 2 直接拼 err.Error() 进 envelope（dev-only 故意暴露原文）。**没有破坏 unwrap 链 / errors.Is 的情况**——客户端 wire 行为全部正确。
- **§S17 errmap 单一事实源**: B2 5 文件 **0 violation**——chat.go 透传 8 个 chatdomain sentinel 全部已登记；eventlog.go 唯一域 sentinel `ErrSeqTooOld` 是 handler 自翻 410 不走 errmap（同 B1 notifications.go pattern §S17 例外）；apikey.go 所有 8 个 apikeydomain sentinel + errorsdomain.ErrInvalidRequest 全登记。dev_info.go / dev_routes.go 不消费任何 sentinel。

## Files audited

| File | LOC | Sites | OK | EDGE | VIOLATION |
|---|---|---|---|---|---|
| chat.go | 148 | 9 | 6 | 3 | 0 |
| dev_info.go | 161 | 6 | 0 | 6 | 0 |
| eventlog.go | 177 | 8 | 8 | 0 | 0 |
| dev_routes.go | 187 | 3 | 3 | 0 | 0 |
| apikey.go | 231 | 9 | 8 | 1 | 0 |
| **TOTAL** | **904** | **35** | **25** | **10** | **0** |

> EDGE-classified rows surfaced as LOW severity per spec — 0 strict violations.

## Severity breakdown

| Severity | Count | Sites |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 10 | chat.go site 1/2/4 (§S16 `%w: <text>` 缺 pkg.Method 前缀, attachment err 路径) ; dev_info.go site 1 (`home, _`) / site 2 (raw err.Error 进 envelope) / site 4 (entry.Info() continue) / site 5 (walkHomeTree `_`) / site 6 (`_ = fs.ErrNotExist` 占位) ; apikey.go site 8 (decodeJSON `decode body: %w` 缺 pkg.Method 前缀, 跨 6 handler 共享) |

**Net: 10 LOW (all EDGE per §S3 dev-only graceful / §S16 prefix lacking but unwrap-able)**

## Status (post-fix)

| site | severity | status | commit |
|---|---|---|---|
| chat.go site 1 | LOW | FIXED | this batch — `handlers.UploadAttachment:` 前缀 |
| chat.go site 2 | LOW | FIXED | this batch — 同上模式 |
| chat.go site 4 | LOW | FIXED | this batch — 加前缀 + `%v` 注解保留 io err |
| dev_info.go site 1 | LOW | FIXED-doc | this batch — 加 dev-only graceful inline 注释 |
| dev_info.go site 2 | LOW | WAIVED | dev-only 故意暴露原 err（tester triage）；prod 经 dev-mode flag 不暴露 |
| dev_info.go site 4 | LOW | FIXED-doc | this batch — 加 dev-only graceful inline 注释 |
| dev_info.go site 5 | LOW | FIXED-doc | this batch — 加 dev-only graceful inline 注释 |
| dev_info.go site 6 | LOW | FIXED | this batch — 删 io/fs import + 删 `_ = fs.ErrNotExist` 占位 |
| apikey.go site 8 | LOW | FIXED | this batch — `handlers.decodeJSON: %w` 前缀（**HIGH ROI**：覆盖 chat/apikey/conv/forge/model/sandbox/answers 全部 handler decodeJSON 路径） |

## Cross-cutting

### 1. apikey.go decodeJSON site 8 — highest-ROI 1-line fix

`decodeJSON` 是本包共享 JSON helper，14+ handler 调用点全部经它。`fmt.Errorf("decode body: %w", ...)` 缺 §S16 `<pkg>.<Method>:` 前缀——B1 _summary §S16 cross-cutting note 已 flag 此点。

**1 行修复**：
```go
return fmt.Errorf("handlers.decodeJSON: %w", joinInvalidRequest(err))
```

修了之后 chat / apikey / conversation / forge / model / sandbox / answers 全部 handler 错误流的 prefix 一致性自动解决。**强烈建议合入下个 fix-batch**。

### 2. chat.go 3 处 `%w: <text>` 形式

chat.go site 1/2/4 全部用 `fmt.Errorf("%w: <static text>", sentinel)` 或 `%w: <inline static>`——把 sentinel 接到内层、把 detail 静态字符串拼上下文。**§S16 严格说**：`<pkg>.<Method>:` 前缀缺位 + site 4 还把原 IO err（io.ReadAll 失败）丢出链外。

**两种修法**：
- 简洁：直接传 sentinel `responsehttpapi.FromDomainError(w, h.log, chatdomain.ErrAttachmentXxx)` 丢弃 detail
- 保留：`fmt.Errorf("handlers.UploadAttachment.<step>: %w", chatdomain.ErrXxx)` + zap.Error(originalErr) 单独打 log

site 4 推荐第二种保留 IO err 信息（disk full / conn drop 调试线索）。

### 3. dev_info.go silent fallback 集中

dev_info.go 5/6 sites 是 **EDGE 集中地**——全部是 dev-only graceful skip 设计：home 拿不到、entry stat 失败、子目录 walk 失败 → 都 silent skip。**§S3 例外要求"行内注释"**——3 处缺注释。建议这批一起补 1-line inline comment + 删 `io/fs` keep-import 占位。

dev_info.go 没有 logger 字段——若注入 `log *zap.Logger`，site 4/5 的 silent continue 可改为 `log.Warn` 给 tester 调试线索。这是 §S3 推荐的"dev/test handler 应让失败可见"模式（同 B1 _summary cross-cutting #4 providers.go 的 logger 缺失）。

### 4. dev_routes.go — 任务描述误判

任务描述写"dev_routes.go 可能起 fire-and-forget goroutine（live-reload 风格），需 detached ctx 检查"。**实际**：dev_routes.go 是手维护的 manifest（`devRoutes []devRoute` 静态数组），handler `Routes` 只 `copy + sortRoutes + Success`。文件 187 LOC 中 158 行是 manifest 字面量。**0 goroutine / 0 IO / 0 ctx use** —— 是 B2 五文件中最简单的一个。fire-and-forget concern 不适用，**可能任务描述把 dev_routes.go 与 dev_runtime.go / dev_processes.go 混淆**（B1 已审 dev_processes.go 也无 goroutine）。

### 5. eventlog.go vs B1 notifications.go pattern alignment

B1 _summary §A.5 footnote 6 + cross-cutting #6 已识别：eventlogdomain.ErrSeqTooOld + notificationsdomain.ErrSeqTooOld **都是 handler 自翻 410 不走 errmap** —— §S17 例外。本审 eventlog.go 进一步验证此 pattern 在双 SSE 协议 (§E1) 是统一惯例。未来加新 SSE channel (mcp_server / build_done / system_warning) 时 copy-paste 此 pattern。

### 6. apikey.go test() 设计 — sentinel + envelope 重叠合规

apikey.go:181-210 的 `test()` 函数：当 `res.OK == false` 时，**不**把 svc 错抛 sentinel，而是直接 `responsehttpapi.Error(... "API_KEY_TEST_FAILED" ...)` 加 details map（latencyMs）。errmap.go:54 的 `ErrTestFailed` entry 仍存在为 svc 上抛路径兜底。**这是合规的**——details map 注入是 wire-shape 关注（不能塞 sentinel + errmap 路径），handler 负责"业务结果转 wire" 含 details 是 §S6 推荐形态。

### 7. handler 层 §S6 一致性 — strong but with 2 hot files

B2 5 文件中 chat.go / apikey.go / eventlog.go / dev_routes.go 全部是 §S6 "解 JSON → 调 service → 写 envelope" 模型；唯一例外是 dev_info.go 内联 walkHomeTree 业务（递归 + filter + sort）——但这本是 dev-only metadata 端点，业务逻辑放 handler 比专门起 service 更合理（同 B1 dev_processes.go 推理）。

## Recommended fix priorities

按 §S20 + §S14 优先级 — 本批 **0 HIGH / 0 MED**，所有 LOW 都是 EDGE，**修复非 §S20 强制**。但有 1 个 **HIGH ROI** 候选：

1. **apikey.go site 8 (decodeJSON `%w` prefix)** — 1 行修复覆盖 7 个 handler 14+ 调用点的 §S16 一致性。**强建议合入下批 fix**——这是 B1 _summary 已识别的 cross-cutting 顶部条目。
2. **chat.go site 1/2/4 (attachment %w prefix + IO err preservation)** — 3 行修复，sentinel 链通仅是 log 可读性增强；可与 site 8 同批修。
3. **dev_info.go 5 sites (dev-only inline comment + io/fs cleanup)** — 修文档/清理，每处 1-2 行。注入 log 字段是 medium-effort 改动（构造 + main.go DI），**强建议但非必需**——给 tester 调试线索。
4. **dev_routes.go / eventlog.go** — 0 fix needed.

## Out-of-scope notes

1. `_test.go` 文件按 fork 约束未读（apikey_test.go / eventlog_test.go 等）。
2. apikeyapp.Service.Test 内部 detached ctx 终态写——属 apikey-app audit batch，spec extracts §S9 已注"前一轮已修"。
3. chatapp.Service.Send 内 fire-and-forget goroutine（启动 chat stream，detached ctx 写 assistant final message）——属 chat-app audit batch。
4. eventlogdomain.Bridge 内部 buffer 管理 + Subscribe 实现——属 eventlog-pkg / chat-app audit batch。
5. dev_runtime.go / dev_processes.go / dev.go / dev_mock_llm.go — 已被 B1 部分覆盖；剩余 dev.go / dev_mock_llm.go 待后续 batch。
6. forge.go (~365 LOC) / mcp.go / sandbox.go / skills.go — 待后续 B3 batch。
