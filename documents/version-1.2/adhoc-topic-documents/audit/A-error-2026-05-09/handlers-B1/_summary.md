# Package audit summary: transport/httpapi/handlers (B1 — 10 small/medium files)

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: handler 层正常路径全部走 `responsehttpapi.FromDomainError` / `responsehttpapi.Error` / `responsehttpapi.Success`，无 `_ = err` 吞错。**3 处 EDGE-LOW** 集中在「query-string parse silent fallback」与「registry-meta-missing silent skip」——皆为 dev/UX 可接受的 graceful degradation。`StreamSSE` 的 `_ = onEvent(...)` 是 sse.go:35-49 godoc 显式声明的契约（client disconnect-on-write 是常态），属 §S3 例外。
- **§S9 detached ctx 终态写**: 本批 handler 全部是**单步 CRUD / 读路径**（POST/PUT/PATCH/DELETE/GET）——`r.Context()` 是正确选择，cancel-on-disconnect = "用户不再想要这次操作" = 不要写。**0 violation**。§S9 detached-ctx pattern 适用于「post-cancel 终态」如 chat stream 被取消后写 assistant final message，**handler 层一般不需要**——本批未发现 fire-and-forget goroutine 起在 handler 内。
- **§S15 ID 生成**: 本批 handler **0 处** mint 业务 ID——transport 是 pure shell，ID 生成职责全部在 app/Service 层。**0 violation**。
- **§S16 错误 wrap 格式**: handler 层正常 forward via `FromDomainError`，**0 处自己 wrap**。**0 violation in scope**。Cross-cutting 发现 1 处 out-of-scope: `paginationpkg.cursor.go:55, 95, 98` 的 `fmt.Errorf("limit must be...: %w", ...)` 缺 `<pkg>.<Method>:` 前缀（不影响 unwrap 链，因仍用 `%w` + sentinel registered）；归 pkg-pagination audit 处理。
- **§S17 errmap 单一事实源**: 本批 handler **0 处**定义新 sentinel；transitive 消费的 sentinel **全部登记**（modeldomain×4 / convdomain×1 / catalogdomain×1 / askapp×3 / errorsdomain.ErrInvalidRequest / reqctxpkg.ErrMissingUserID）。`notificationsdomain.ErrSeqTooOld` **未登记**——但 handler 用 `errors.Is` 自己翻译为 410 envelope，**不走** `FromDomainError`，符合 §S17 spec extract 例外（mirrors eventlog handler pattern, see pkg-eventlog/_summary.md §A.5）。

## Files audited

| File | LOC | Sites | OK | EDGE | VIOLATION |
|---|---|---|---|---|---|
| util.go | 24 | 1 | 1 | 0 | 0 |
| dev_processes.go | 43 | 2 | 1 | 1 | 0 |
| health.go | 43 | 1 | 1 | 0 | 0 |
| dev_runtime.go | 76 | 3 | 3 | 0 | 0 |
| model.go | 76 | 3 | 3 | 0 | 0 |
| catalog.go | 85 | 3 | 3 | 0 | 0 |
| answers.go | 87 | 3 | 2 | 1 | 0 |
| providers.go | 96 | 4 | 3 | 1 | 0 |
| notifications.go | 103 | 5 | 5 | 0 | 0 |
| conversation.go | 139 | 5 | 5 | 0 | 0 |
| **TOTAL** | **772** | **30** | **27** | **3** | **0** |

> EDGE-classified rows are surfaced as LOW severity per spec — 0 strict violations.

## Severity breakdown

| Severity | Count | Sites |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 3 | dev_processes.go site 1 (query-param silent fallback to default sample size; dev-only); answers.go site 2 (handler-side `req.ToolCallID == ""` 400 check vs sentinel; wire-code clarity trade-off); providers.go site 2 (registry meta-missing silent skip; defensive-against-impossible internal invariant) |

**Net: 3 LOW (all EDGE per §6 反校验剧场 / dev-tool / defensive)**

## Status (post-fix)

| site | severity | status | commit |
|---|---|---|---|
| dev_processes.go site 1 | LOW | FIXED-doc | this batch — 加 §6 反校验剧场 inline 注释 |
| answers.go site 2 | LOW | WAIVED | input-shape 校验属 JSON-schema 层；引入 sentinel 仅 1 个调用点 = boilerplate > 价值 |
| providers.go site 2 | LOW | FIXED-doc | this batch — 加 invariant-defensive inline 注释 |

## Cross-cutting

### 1. handler 层 §S6 一致性 — exemplary

本批 10 文件 / 30 sites 中，**所有错误路径**走 envelope helper（`FromDomainError` / `Error` / `Success` / `Created` / `NoContent` / `Paged`）——零裸 `w.Write` / `json.Encode`，零 service-layer 业务逻辑泄漏到 handler。

handler 层 §S6 "解 JSON → 调 service → 写 envelope" 在本批是**真正达成**的目标形态。可以作为 model file（model.go / catalog.go / conversation.go）举例向其他 handler 展示。

### 2. dev/test handler 的 silent-fallback 策略

`dev_processes.go` (sample query-param) / `dev_runtime.go` (db.DB() 失败) 都用 silent fallback——前者无 inline comment，后者有 explicit bilingual justification。

**建议**：dev_processes.go site 1 顺手补一行 inline comment 把 §S3 例外身份钉在代码上（"// bad sample param falls back to default — dev-only convenience knob"），让 reviewer / 未来 audit 不需要 cross-reference 才知道为什么不 400。

### 3. answers.go 的 input-shape vs sentinel 边界

handler 直接用 `responsehttpapi.Error(... "INVALID_REQUEST", "toolCallId is required" ...)` 而不是创建 `askapp.ErrToolCallIDRequired` sentinel + errmap 行。**两种都合规**：

- 直接 `Error` 优势: 0 sentinel 增量，handler 自含
- sentinel 优势: 错误码统一通过 errmap（一处事实源，本批其他 handler 全是这模式）

**建议**: 保留现状，**加一行 inline comment** 解释为什么不绕 sentinel——避免下次 audit 又被当 "S6 业务逻辑泄漏" 标红。或者若想统一，加 `askapp.ErrToolCallIDRequired` + errmap 行（5 行变更）。

### 4. providers.go 缺 logger 字段

`ProvidersHandler struct{}` 完全空，site 2 的 registry-invariant silent skip 无处 log。**建议**: 哪怕只为 audit-debuggability，注入 `log *zap.Logger`，让那一行变 `log.Error("provider listed but meta missing", zap.String("name", name))`——成本极低，invariant violation 时给出唯一的 debug 线索。

不修也行——就当 invariant 永不破。

### 5. paginationpkg cross-cutting 出 audit scope

`paginationpkg.cursor.go:55, 95, 98` 的 wrap 格式 (`fmt.Errorf("limit must be a positive integer: %w", err)`) 缺 §S16 要求的 `<pkg>.<Method>:` 前缀。本批 handler 透传无影响（unwrap 链仍能匹配 ErrInvalidRequest），但 pkg-pagination audit 时应处理。

### 6. notifications.go vs eventlog.go pattern alignment

两者都做 "Last-Event-ID parse → Subscribe → 410 ErrSeqTooOld 自翻 → StreamSSE marshal+wire"。本审 notifications.go 验证了 §S17 例外（handler 自翻不走 errmap）与 eventlog 一致。这是好的——双 SSE 协议形成统一模式，未来加 SSE channel 时 copy-paste 此 pattern 即可。

## Recommended fix priorities

按 §S20 + §S14 优先级 — 本批 0 HIGH / 0 MED，所有 LOW 都是 EDGE，**修复非 §S20 强制**。可选优化：

1. **answers.go site 2** (handler-side req.ToolCallID == "" 检查) — 加 1 行 inline comment 钉住「为什么不绕 sentinel」的设计选择，避免下次 audit 重复标红。1 行变更，0 风险。

2. **providers.go site 2** (registry meta-missing silent skip) — 注入 `log *zap.Logger` 字段 + 在 silent-skip 处 `log.Error(...)`。3-5 行变更（构造函数 + 字段 + 调用方在 main.go 装配）。Invariant violation 时唯一 debug 线索，**强建议但非必需**。

3. **dev_processes.go site 1** (sample query-param silent fallback) — 加 1 行 inline comment 钉住 §S3 例外身份。1 行变更，0 风险，spec-compliance 增强。

4. **out-of-scope**: paginationpkg cursor.go 的 §S16 wrap-prefix 缺失，归 pkg-pagination audit。

## Out-of-scope notes

1. `_test.go` 文件按 fork 约束未读。
2. apikey.go (231 LOC) 含 `decodeJSON` / `joinInvalidRequest` helpers — 本批 model.go / answers.go / conversation.go 透传消费，但其本身实现归 apikey.go audit batch（model.go trace 中已注 cross-cutting 见 §S16 prefix 缺失：apikey.go:221 `fmt.Errorf("decode body: %w", ...)` 同样无 `<pkg>.<Method>:` 前缀）。
3. response/sse.go 的 `_ = onEvent(...)` 契约设计在本批被 notifications.go 透传依赖；其本身合规（godoc 显式声明），属 response 包 audit batch。
4. 本批未审 chat.go (148 LOC) / dev_info.go (161 LOC) / eventlog.go (177 LOC) / dev_routes.go (187 LOC) / apikey.go (231 LOC) / forge.go / mcp.go / sandbox.go / skills.go / dev_mock_llm.go / dev.go ——超出 B1 fork 范围，归后续 batch。
5. handler 测试覆盖（apikey_test.go / catalog_test.go / conversation_test.go / eventlog_test.go / mcp_test.go / model_test.go / providers_test.go / sandbox_test.go / skills_test.go）按 fork 约束跳过。
