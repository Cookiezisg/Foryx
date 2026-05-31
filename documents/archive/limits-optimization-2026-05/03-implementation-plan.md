# 03 — 实施计划(P0–P3)

> **For agentic workers:** REQUIRED SUB-SKILL: `superpowers:subagent-driven-development` 或 `superpowers:executing-plans`。步骤用 `- [ ]` 跟踪。
> **每个 task 执行前必读**:对应裁决与"为什么"在 [`01-optimize-decisions.md`](./01-optimize-decisions.md)(§①–⑤);可配项的存储/UI 在 [`02-advanced-settings-ui.md`](./02-advanced-settings-ui.md);原则在 [`00`](./00-overview.md)。

**Goal:** 把桶 3 的不合理 hardcode 优化掉,统一到「高 ceiling + 诚实失败态 + 用户可中断 + 真实信号驱动」;可调项做成前端「高级能力」区;顺带修 5 个真 bug。

**Architecture:** 4 阶段,**先把配置底座 + 止血放一起 → 再诚实 → 再换机制 → 最后接 settings/前端**,依赖自下而上。关键:**`Limits` getter 在 P0 就装上(先返回默认)**,所以 P0–P2 改的每处都**一次写成"读 getter"的最终形态**(只改一遍),P3 只把 getter 数据源换成 `settings.json` + 端点 + 前端。**超时不再限健康活——in-progress 工作靠 ctx + stop**(见 §⑤)。

**Tech Stack:** Go 1.25 / GORM / modernc sqlite;React 18 TS strict / FSD / TanStack Query。测试 `make unit` `make mock` `make web` `make lint-frontend`,`staticcheck ./...`,发布门禁 `make verify`。

> **每阶段收尾**:`cd backend && go build ./... && staticcheck ./...` + `make unit` + `make mock` 绿;改了 errcode/endpoint/event/status 跑 `make matrix`;碰前端跑 `make lint-frontend` + `make test-frontend`;按各阶段 doc-sync 同步 + `progress-record.md` dev log(§S14/§F1)。

---

> **🟢 实施增量（as-built，2026-05-31，commits `52095f6`→`b863935`）—— 与初稿不同处以此为准：**
> - **「继续」按钮砍掉**：诚实失败态做满（后端 `StopReasonMaxSteps`+`StatusError`+`MAX_STEPS_REACHED` + `ErrorCard` 可见 + errMsg "continue to resume"），但专用一键续跑按钮 / `:continue` 端点**经评估不做**——ceiling 抬到 150 后撞顶罕见，用户再发一条消息即续跑（composer 本就是续跑路径）。
> - **§② 输出兜底纠正**：`modelcaps` fallback `MaxOutput` **未抬**（抬到 64000 会让 `UsableInput = 32768 − 64000` 触底、压缩失灵）；真·输出截断元凶是 **Gemini 不发 `maxOutputTokens`**（已修：始终发模型真上限）；Anthropic `8096` 近 dead，未动。
> - **节点超时是"删"非"归 0"**：`scheduler/retry.go` 的 `defaultTimeouts` 整表删除，只留显式 `node.Timeout` 覆盖。
> - **mcp 180s / Bash 120s** 为 const（非 getter）；subagent maxTurns 保留 per-type 注册表（只 timeout 接 `limits.Current`）。
> - **Anthropic 4.7/4.8 effort-thinking + live capability overlay** 延后（见 [`00`](./00-overview.md) §4）。

## 文件结构(decomposition)

**新建**
- `backend/internal/infra/settings/limits.go` — `Limits` 结构 + `DefaultLimits` + zero→默认 填充 + 校验(**P0 落 getter**;P3 接文件读写)
- `backend/internal/transport/httpapi/handlers/settings.go` — `GET/PUT /settings/limits`(P3)
- `frontend/src/entities/settings/api/{useLimits,useUpdateLimits}.ts` + `model/types.ts` 加 `Limits`(P3)
- `frontend/src/features/settings/ui/AdvancedCapabilitiesSection.tsx`(P3)

**修改(关键)**
- `infra/llm/transport.go`(SSE buffer P0;`Client.Timeout=0`+Transport 分段超时 P2)、`provider.go`(idle 网 P2)、`anthropic.go`(兜底 P0;thinking P1;SSE buffer P0)、`gemini.go`(thinking P1)
- `pkg/modelcaps/modelcaps.go`(兜底 MaxOutput P0,读 getter)
- `app/tool/{function,handler,mcp,skill,workflow,document}/search*.go`(top-N 统一常量 P0)、`function/get_execution.go`、`handler/get_call.go`(get_* 统一 P0)、`function/search_executions.go`(`truncateJSON` P0)
- `domain/chat/chat.go`(`StopReasonMaxSteps`/状态 P1)、`app/loop/loop.go`(诚实失败态 P1)、`app/chat/runner.go`(读 getter P0)、`chat/history.go`(history 200→2000 + user 投影 P2)
- `app/contextmgr/estimate.go`(nil-resolver WARN P1)
- `app/scheduler/retry.go`(**删 defaultTimeouts** P2)、各 `dispatch_*.go`(审计 ctx P2)、`dispatch_agent.go`(workflow 例外失败态 P1)
- `infra/handler/client.go`(确认 ctx.Done 已 select;`MethodSpec.Timeout` 可选 P2)、`app/mcp/mcp.go`(CallTool 读 getter P2)、`app/tool/shell/bash.go`(默认超时读 getter P2)
- `app/subagent/spawn.go`(StopReason 映射 P1;超时读 getter)
- `cmd/server/main.go` + `harness`(P0 装 getter)
- 前端:`SettingsModal`、`shared/api/queryKeys.ts`、chat 页(撞顶「继续」按钮 P1;「stop run」按钮 P2)

---

# P0 — 配置底座 + 止血(低风险;注意:抬 cap/top-N 是行为变化但安全)

### Task P0.0: `Limits` getter 骨架(#2 前置,关键)
**Files:** `infra/settings/limits.go`、`cmd/server/main.go`、`harness`
- [ ] 定义 `Limits` 结构(按 [`02`](./02-advanced-settings-ui.md) §1)+ `DefaultLimits` + zero→默认 填充
- [ ] 暴露 `func() Limits` getter;P0 阶段**只返回 `DefaultLimits`**(不读文件)
- [ ] `main.go`/`harness` 把 getter 注入下面各消费方(见 [`02`](./02-advanced-settings-ui.md) §2 表)
- [ ] Verify:`make unit`
> 之后 P0–P2 所有"可调"改动都**读 `limits().X`**,不写裸字面量。

### Task P0.1: SSE 单行 buffer 抬高(bug #3)
**Files:** `infra/llm/transport.go`、`anthropic.go`
- [ ] `const maxSSELineBytes = 8 << 20`;两处 `bufio.NewScanner` 后 `sc.Buffer(make([]byte,0,64*1024), maxSSELineBytes)`
- [ ] 测试:>64KB 的 `data:` 行不 abort、事件完整解析

### Task P0.2: 输出兜底 → `limits().Output.UnknownModelMaxTokens`(默认 64000,§②)
**Files:** `pkg/modelcaps/modelcaps.go`、`infra/llm/anthropic.go`
- [ ] `modelcaps.go:131` fallback `MaxOutput` 与 `anthropic.go:21` 兜底改读 getter(默认 64000);`ContextWindow 32768` 兜底不变
- [ ] golden/modelcaps 测试更新

### Task P0.3: `truncateJSON` 修非法 JSON(bug #2)+ 更新现有断言(#9)
**Files:** `app/tool/function/search_executions.go`(helper)+ 7 处调用
- [ ] 共享 helper:截**结构内字符串值**保 envelope 合法,或明确标纯文本 snippet + `truncated:true`;7 处改用
- [ ] **更新现有 pipeline 测试**里 pin 旧(非法)输出的断言
- [ ] 测试:所有预览/详情输出是合法 JSON 或明确纯文本

### Task P0.4: search top-N 统一规格(§④)
**Files:** 共享常量 + `tool/{function,handler,mcp,skill,workflow,document}/search*.go`
- [ ] 加共享常量 `DefaultSearchTopN`(读 `limits().Tools.SearchTopN`,默认 10)/ `MaxSearchTopN = 50`
- [ ] 4 个 LLM-rerank(function/handler/mcp/skill)默认/max 全改用之(删散落 `3`/`5`/`20`/`10`)
- [ ] workflow 补 max `50`;documents 本就 `10/50` 对齐确认 ——「是否 LLM 重排」是另一根轴,**不动**
- [ ] **更新现有测试**(pin 旧 top-3/5 的断言);测试:默认 ≤10、可请求到 50

### Task P0.5: `get_*_execution` 去语义截断 + 统一(§④)
**Files:** `tool/function/get_execution.go`、`handler/get_call.go`(+ workflow/mcp/skill 对齐)
- [ ] function/handler 的 `4096B` → 统一 `256KB` 防御上限;超出按 JSON 边界优雅截 + `offset`/取回提示;5 个 `get_*` 同策略

**P0 收尾**:`go build ./... && staticcheck ./...`、`make unit`、`make mock`(含更新后的断言)绿。Doc-sync:`forge_redesign/08-executions.md §7.1` + `service-design-documents/{function,handler}.md` + `progress-record.md`(`[opt] P0`)。

---

# P1 — 诚实失败态(全量,不拆 #4)

### Task P1.1: 循环撞顶不再谎报 +「继续」(bug #1,§①;**全套铺开**)
**Files:** `domain/chat/chat.go`、`infra/db/schema_extras.go`、`cmd/coverage-matrix/sse_truth.go`、`app/loop/loop.go`、`app/subagent/spawn.go`、`app/scheduler/dispatch_agent.go`、前端 chat 页 + `chatStore`
- [ ] `domain/chat`:加 `StopReasonMaxSteps`(+ 必要时新 `StatusIncomplete`);**DB CHECK 约束随之迁移**(`schema_extras.go`)
- [ ] **SSE 协议**:`sse_truth.go` 枚举 + `eventlog-redesign/` 协议文档加该 status/stop_reason(封闭枚举,E2)
- [ ] `loop/loop.go:182` 步数耗尽:写**非成功终态** + `stop_reason=max_steps` + errCode `MAX_STEPS_REACHED`(不再冒充 `max_tokens`/`completed`)
- [ ] `subagent/spawn.go:167` 映射切到新 reason;`dispatch_agent.go` workflow 节点撞顶 → flowrun 节点 `failed`/`incomplete`(例外:cap 保留,失败态诚实)
- [ ] **前端**:`chatStore` 识别新终态 → 大声提示 + 一键「继续」;**「继续」小设计**:不加新用户消息,后端提供续跑入口(re-enqueue 同会话,历史 + summary 完整重入)
- [ ] 测试:`make mock` 步数耗尽 → 状态非 `completed`、stop_reason=`max_steps`;workflow 节点同验

### Task P1.2: 模型自报 hit max_tokens → surface(§②缺口)
**Files:** `app/loop/{loop.go,stream.go}`
- [ ] `stop_reason==max_tokens` 时 emit 通知/UI 徽章;可选 auto-continue ≤2 轮
- [ ] 测试:fake LLM 返回 `max_tokens` finish → surface

### Task P1.3: 压缩 nil-resolver 告警(bug #5)
**Files:** `app/contextmgr/estimate.go`、`main.go`/`harness`
- [ ] `capFor == nil` → WARN + 启动断言(防大模型按 32K 压)

### Task P1.4: thinking budget 修(bug #4,§②)
**Files:** `infra/llm/anthropic.go`、`gemini.go`
- [ ] Anthropic:去 `8192` thinking 顶(从 `BudgetMax` 派生);**Opus 4.7/4.8 走 adaptive `effort`,不发手填 budget**
- [ ] Gemini:fallback `8192 → -1`(动态);golden 测试更新

**P1 收尾**:上 + `make matrix`(新 errcode/status)+ **`make lint-frontend` + `make test-frontend`**(继续按钮)。Doc-sync:`chat.md`/`subagent.md`/`compaction.md`/`error-codes.md`/`events-design.md`(新 status/stop_reason)/`frontend-design-documents/feature-chat`(继续按钮)+ `progress-record.md`(`[opt] P1`)。

---

# P2 — 换机制(ctx + idle + history)

### Task P2.1: LLM 删总墙钟 + 留 idle 死连接网(§⑤,#5)
**Files:** `infra/llm/transport.go`、`provider.go`
- [ ] `newSharedHTTPClient`:`Timeout = 0`;`Transport = &http.Transport{ DialContext:(&net.Dialer{Timeout:10s,KeepAlive:30s}).DialContext, TLSHandshakeTimeout:10s, ResponseHeaderTimeout:60s }`
- [ ] `providerClient.Stream`:`timer := time.AfterFunc(idle, cancel)`,每 `range ParseStream` 事件 `timer.Reset(idle)`;`idle = limits().Timeout.LLMIdleSec`(默认 150s,纯死连接探测)
- [ ] 测试:httptest"持续吐 token 总时长 >150s"→ 不 kill;"开流后静默 >idle"→ cancel
- [ ] Doc-sync:**新建** `llm-providers/` 流式超时设计注

### Task P2.2: history 抬上限 + 统一 ContextRole 投影(§③,#3)
**Files:** `app/chat/history.go`
- [ ] `maxHistoryMessages 200 → 2000`(纯 I/O 上限,读 getter `agent.maxSteps`? 否——单列 `limits` 或常量;注释说明语义边界归 token 预算+compaction)
- [ ] **`buildUserLLMMessage` / `buildHistory` 对 user 消息也按 `ContextRole` 投影**(archived 丢 / cold 省),与 assistant 侧一致——否则抬 cap 会把归档 user 消息全文重塞
- [ ] 测试:`make mock` 造 >200 条短消息(未触压缩)→ 早期 user 轮次仍在;造已归档 user 消息 → 不与 summary 重复全文

### Task P2.3: 删 workflow 节点墙钟超时 + 审计 ctx 传播 + stop-run(§⑤,#1)
**Files:** `app/scheduler/retry.go`、各 `dispatch_*.go`、`infra/handler/client.go`、flowrun cancel 端点 + 前端
- [ ] **删整张 `defaultTimeouts`**(及 `nodeTimeoutDuration` 墙钟逻辑)
- [ ] **审计每个 dispatcher**:确认 ctx 透传到叶子操作(function/handler 子进程、mcp、http、llm、skill)且叶子 honor ctx
- [ ] `scheduler.Cancel` → run-level ctx 取消;**确认/补 `POST /flowruns/{id}:cancel` 端点 + 前端「stop run」按钮**
- [ ] handler RPC:`readMessage` 已 select `ctx.Done`(`client.go:239`),**无需为安全实现 timeout**;`MethodSpec.Timeout` 留作可选 per-method 便利(本阶段可不做)
- [ ] 测试:启动一个长跑 function 节点 → `scheduler.Cancel` 能即时杀掉(`make sandbox`)

### Task P2.4: mcp/Bash 工具超时改可配高默认 + tool_result 可恢复(§⑤/③)
**Files:** `app/mcp/mcp.go`、`app/tool/shell/bash.go`、`app/loop/history.go`
- [ ] mcp `defaultCallTimeout`、Bash 默认超时改读 getter(`timeout.mcpCallSec`/`bashDefaultTimeoutSec`,默认 180/120;Bash 硬顶 600 常量留)
- [ ] `projectToolResultContent`:warm/cold 占位符塞 `block.<id>`("full result at block …, fetch to retrieve")

**P2 收尾**:`make unit` + `make mock` + `make sandbox`(节点 cancel / handler RPC)+ `staticcheck`。Doc-sync:`chat.md`/`scheduler.md`/`handler.md`/`mcp.md`/`compaction.md`/`api-design.md`(cancel 端点)+ `progress-record.md`(`[opt] P2`)。

---

# P3 — 配置化 + 优雅化(settings 数据源 + 前端)

### Task P3.1: `settings.json` `limits` 持久化 + 读写端点(§[`02`](./02-advanced-settings-ui.md))
**Files:** `infra/settings/{limits.go,settings.go}`、`handlers/settings.go`、router/deps
- [ ] `settings.Limits()` 从 `settings.json` 读(缺省 `DefaultLimits`);**`UpdateLimits` 用 read-modify-write 只改 `limits` 段、保留 permissions/hooks**,原子写 + 内存即更
- [ ] `GET/PUT /api/v1/settings/limits`(envelope,camelCase,N6 upsert 200,非法 400)+ errmap
- [ ] **P0 的 getter 改为返回 `settings.Limits()`**(数据源切换;消费方零改动)
- [ ] 测试:`api/settings/settings_pipeline_test.go` happy + 非法 + read-modify-write 不冲 hooks;`// covers: GET|PUT /api/v1/settings/limits` + `make matrix`

### Task P3.2: 前端「高级能力」区(§[`02`](./02-advanced-settings-ui.md) §3)
**Files:** `entities/settings/{api/*,model/types.ts}`、`features/settings/ui/AdvancedCapabilitiesSection.tsx`、`SettingsModal`、`shared/api/queryKeys.ts`、`i18n/locales/{zh,en}/settings.json`
- [ ] `useLimits`/`useUpdateLimits` + `Limits` 类型 + `qk.settingsLimits()`
- [ ] `AdvancedCapabilitiesSection`(默认折叠 + 警示 + 分组输入 + 单项/全部恢复默认),挂底部;组件零业务决策
- [ ] i18n `settings.advanced.*` 全量 zh/en
- [ ] Verify:`make lint-frontend` + `make test-frontend`
- [ ] Doc-sync:`frontend-prd.md §17` + `entity-types.md` + `cross-cutting.md` + `fsd-layers.md` + `frontend-design-documents/feature-settings.md`

### Task P3.3(可选): guards + perScenario override(#6)
- [ ] `guards`(附件/HTTP节点/webhook MB)、`output.perScenarioOverride` 接进 getter + UI;不做则从 UI 暂隐(别留空 knob)

### Task P3.4(大/可延后): live capability overlay(§②,#10)
- [ ] 接通 `llm-providers/06-implementation-plan.md P5.4`:Anthropic `/v1/models`、Gemini `models.get`、OpenRouter `/api/v1/models`、Ollama `/api/show` 读真 `max_tokens` 叠加 modelcaps。**这是独立子工程,可独立排期**

### Task P3.5(可选): budget-based 终止 / Bash head+tail
- [ ] per-turn token/cost 预算作为 maxSteps 更优替代;Bash `capOutput` 改 head+tail 中段截。后续增强,不阻塞

**P3 收尾**:`make verify` + `make lint-frontend` + `make test-frontend` + `wails dev` 冒烟。Doc-sync 全量(上各 task)+ `CLAUDE.md`(若"限制可配/超时哲学"成新规范,加一行)+ `progress-record.md`(`[opt] P3`)。

---

## 验证矩阵(每阶段门禁)

| 阶段 | 命令 |
|---|---|
| P0 | `go build ./... && staticcheck ./...` · `make unit` · `make mock`(含更新断言) |
| P1 | 上 + `make matrix`(新 errcode/status)+ `make lint-frontend` · `make test-frontend`(继续按钮) |
| P2 | 上 + `make sandbox`(节点 cancel / handler RPC) |
| P3 | `make verify` · `make lint-frontend` · `make test-frontend` · `wails dev` 冒烟 |
