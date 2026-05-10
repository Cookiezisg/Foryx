# Audit trace — `transport/httpapi/handlers/dev_mock_llm.go` (250 LOC)

Phase A audit fork (B3) — §S3 / §S9 / §S15 / §S16 / §S17.
Dev-only file — `--dev` 启动 + `llmFactory` 已 wired 时才注册 (router.go gate)。
5 endpoints under `/dev/mock-llm/`：push scripts / queue / clear / last-prompt / llm-trace。

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | `dev_mock_llm.go:93` | `if m.Error != "" { ev.Err = errors.New(m.Error) }` | A.4 | OK | `errors.New` 在最里层构造 user-supplied EventError 文本，不是 wrap。无 `%v` / 套娃问题。§S16 例外（最里层 sentinel/literal 构造）。 | N-A | — | — | — |
| 2 | `dev_mock_llm.go:96` | `return ev, fmt.Errorf("unknown event type %q (want text/reasoning/...)", m.Type)` | A.4 | EDGE | 无 `%w`——但本是叶子 error（无 inner err 可 unwrap），且**不**经 errmap（直接被 site 5 catch 转 envelope detail）。无 `<pkg>.<Method>:` prefix，但本路径无 sentinel chain 维护需求。dev-only triage 错误，前端 testend 直接显示 detail 文本即可。EDGE：dev-only 容忍。 | LOW | dev tester 看不到包前缀，仅看到 type 字符串 | 改为 `fmt.Errorf("handlers.toStreamEvent: unknown event type %q (want ...)", m.Type)` 一致 §S16 全 prefix 风格 | FIXED (this commit — site 2 加 `handlers.toStreamEvent:` 前缀；site 3 改用 decodeJSON helper) |
| 3 | `dev_mock_llm.go:116-119` | `if err := json.NewDecoder(r.Body).Decode(&body); err != nil { responsehttpapi.Error(w, ..., "INVALID_REQUEST", "failed to parse JSON body: "+err.Error(), nil); return }` | A.4 | EDGE | 直接 `+err.Error()` 拼字符串入 envelope。**未走** `decodeJSON` helper（apikey.go decodeJSON site 8 在 B2 已 fix 加 prefix）——本 handler 自行 inline。`err.Error()` 进 envelope detail 是 dev-only debug 友好，但不走 sentinel chain。两个问题同源：(a) 未走 decodeJSON helper（**重复 code，缺一致性**）；(b) 错误未带 §S16 prefix。EDGE—dev-only。**注**：B2 _summary 已注 decodeJSON 是 B1/B2 cross-cutting fix 候选，本文件应改用 `decodeJSON(r, &body)` 复用。 | LOW | dev tester 误格式 JSON 时直接看到 stdlib decode err 文本（OK，dev 友好）；但 prod 路径已 gate（router.go 仅 --dev 注册），无信息泄漏 | 改用 `if err := decodeJSON(r, &body); err != nil { responsehttpapi.FromDomainError(w, h.log, err); return }`——一致 B1/B2 已修 pattern | FIXED (this commit — site 2 加 `handlers.toStreamEvent:` 前缀；site 3 改用 decodeJSON helper) |
| 4 | `dev_mock_llm.go:121-124` | `if len(body.Scripts) == 0 { responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST", "no scripts in payload (expect {scripts: [...]})", nil); return }` | A.4 | OK | 不是错误 wrap，是 handler 直接判定空 payload + 写 envelope（§S6 handler 业务校验合法 — payload-shape 校验留 handler 一致）。`INVALID_REQUEST` wire code 与 errorsdomain.ErrInvalidRequest errmap 行一致。 | N-A | — | — | — |
| 5 | `dev_mock_llm.go:135-140` | `ev, err := eIn.toStreamEvent(); if err != nil { responsehttpapi.Error(w, ..., "INVALID_REQUEST", fmt.Sprintf("script[%d].events[%d]: %s", i, j, err.Error()), nil); return }` | A.4 | OK | catch site 2 错，转入 envelope detail 加 path 索引。`fmt.Sprintf` 进 detail string 是 wire shape（不维护 sentinel chain），不算 §S16 wrap 违规。dev-only triage 友好。 | N-A | — | — | — |
| 6 | `dev_mock_llm.go:127` | `mock := h.llmFactory.Mock()` | A.1 | OK | `Mock()` 是 dev factory 的 singleton getter，无 error 返回，no-op when factory 未 init（router 已 gate `--dev` + factory wired，此 handler 不会注册）。无吞错风险。 | N-A | — | — | — |
| 7 | `dev_mock_llm.go:144` | `mock.PushScript(s)` | A.1 | OK | `PushScript` 是 in-memory append 无 error 返回（singleton MockClient 内部 lock），handler 不能也无需检查 err。 | N-A | — | — | — |
| 8 | `dev_mock_llm.go:163-164` | `mock := h.llmFactory.Mock(); queue := mock.Queue()` | A.1 | OK | 与 site 6 同；`Queue()` 是 in-memory snapshot getter 无 error。 | N-A | — | — | — |
| 9 | `dev_mock_llm.go:191` | `dropped := h.llmFactory.Mock().Clear()` | A.1 | OK | `Clear()` 返 count，无 error。dev-only 立即操作。 | N-A | — | — | — |
| 10 | `dev_mock_llm.go:208` | `req := h.llmFactory.Mock().LastRequest()` | A.1 | OK | `LastRequest()` 返最近 Request snapshot（可能 zero-value if no calls yet），handler 透传，无 error 路径。dev-only LLM 调试。 | N-A | — | — | — |
| 11 | `dev_mock_llm.go:233-238` | `tracer := h.llmFactory.Tracer(); if tracer == nil { responsehttpapi.Error(w, http.StatusServiceUnavailable, "TRACER_DISABLED", "LLM trace recorder not enabled (only available in --dev)", nil); return }` | A.5 | OK | wire code `TRACER_DISABLED` **不**走 errmap (无对应 sentinel)——本 handler 自翻 503，与 eventlog.go::ErrSeqTooOld 自翻 410 同 pattern (B2 _summary cross-cutting #5 已记)。dev-only 端点的 wire code 通常不入 errTable（域 sentinel 才入），合规。 | N-A | — | — | — |
| 12 | `dev_mock_llm.go:241-244` | `if convID == "" { responsehttpapi.Success(w, ..., map[string]any{"conversations": tracer.Conversations()}); return }` | A.1 | OK | empty query param → list mode（合理 fallback；非吞错而是无 param 默认行为）。 | N-A | — | — | — |
| 13 | `dev_mock_llm.go:118` `dev_mock_llm.go:138` | wire code "INVALID_REQUEST" | A.5 | OK | 非 sentinel-driven response（直接 inline 错误文本）；errmap.go:44 `errorsdomain.ErrInvalidRequest` entry 仍在为 sentinel 路径保留。本文件 wire code 字面量与 errmap 行一致。 | N-A | — | — | — |
| 14 | `dev_mock_llm.go:230-249` | `LLMTrace` overall — 无 SSE 推流 | A.1 / N7 | OK | 任务描述提示"如果 mock LLM 模拟 LLM 流式响应，wire format 必须严格 §N7"——**实际**：本 handler 不推 SSE，仅返 JSON envelope（traces 是 finished snapshots）。SSE 推流由 chatapp.Service 经 eventlog Bridge 进行，本 dev handler 是 read-only 检查器，不参与 wire-format。 | N-A | — | — | — |

## Sub-check 模板

A.1 §S3 错误吞没:
  - violations: not present
  - 备注：mock LLM/tracer factory 调用全无 error 返回（singleton getter 设计）；JSON parse / toStreamEvent 错都通过 envelope 返。

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: 0
  - 各自 ctx 来源: N/A
  - violations: N/A — 本文件 0 长跑流程 / 0 落库写 / 0 fire-and-forget goroutine。所有操作都在 in-memory MockClient 单例上同步执行（PushScript / Clear / Queue / LastRequest / Conversations），handler 立即返 envelope。**任务描述担忧"mock-LLM scripted response 加载是否 silent fail"——不存在，全部 sync in-memory，无 IO，无落库**。

A.3 §S15 ID 生成:
  - ID generation calls: 0（无 idgenpkg.New / crypto rand 调用）
  - violations: N/A — 本文件不生成业务 ID。MockScript 是 in-memory 队列项，无持久 ID；Tracer trace ID（如有）由 llminfra 内部管理。

A.4 §S16 错误 wrap 格式:
  - violations: site 2 (`fmt.Errorf` 无 `<pkg>.<Method>:` prefix), site 3 (inline JSON decode err 拼字符串入 envelope，未复用 decodeJSON helper)
  - 备注：两处都是 EDGE-LOW（dev-only，不破坏 errors.Is 链——本就无 sentinel chain 需维护）。site 3 是 cross-cutting 候选（与 B1/B2 已识别 decodeJSON helper 复用 fix 同源）。

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: 0（本文件不定义任何 var Err...）
  - 已登记 errmap: N/A
  - missing: N/A — 本文件不定义 sentinel；handler 用的 wire codes 都是字面量（"INVALID_REQUEST" / "TRACER_DISABLED"），无 sentinel 经 FromDomainError 路径冒泡。

## Cross-cutting

1. **decodeJSON helper 复用缺失**：site 3 inline 写 `json.NewDecoder(r.Body).Decode(&body)` + `+err.Error()` 拼字符串。B1/B2 _summary 多次提到 decodeJSON 是包共享 helper，14+ handlers 已用。本 dev handler 应一致改用——**EDGE-LOW**，dev-only 路径，但风格一致性是 §S6 handler 薄度的延伸。
2. **dev handler 风格**：本文件与 B2 dev_info.go 同属 `--dev` gated；与 dev_info.go 不同的是，本文件 handler 全部 thin (~10-30 LOC each)，没有 dev_info.go 那种 "walkHomeTree silent skip" §S3 EDGE 集中。整体最干净的 dev handler 之一。
3. **§N7 SSE 不适用**：任务描述提示 §N7 wire format check——但本 handler 0 SSE 端点。所有 SSE 推流由 chatapp.Service / eventlog Bridge 完成，dev_mock_llm 是 in-memory queue 检查器。task spec 误把 mock-llm 与 chat stream 混淆。
