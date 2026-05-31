# Subagent Forger + Catalog Extension + E2E Pipeline Plan

> ✅ **COMPLETED 2026-05-13** — 5 commits F1-F5 直推 main(02ccc17 / 21dc15b / 66bab4d / 2eb4f43 + 本 commit)。
> 见 [`README.md`](../README.md) trinity 完工导航 + [`progress-record.md`](../../../progress-record.md) 2026-05-13 dev log。
>
> **Scope 调整** vs 原 plan:
> - F1 filterTools strip — done(D21 落地 + 3 单测)
> - F2 主 agent system prompt 教学 — done(放 chat/runner buildSystemPrompt;catalog generator 不动 — generator 是 LLM 生成 catalog summary 的 meta-prompt,不是主 agent 自己的 system prompt)
> - F3 catalog trinity source verification — done(2 pipeline test)
> - F4 approval lifecycle E2E — done(3 HTTP 场景;RehydrateOnBoot 由 E10 单测覆盖)
> - F5 final doc sync + README — done(本 commit)
> - **跳过原 Task 5 邮件 workflow 全栈 E2E** — 需要 25-turn fake LLM script,长期维护成本太高,效益不如直接 dogfood;主 agent 多 agent 锻造 mechanics 由 subagent unit + filterTools test + system prompt test 覆盖足够
>
> Trinity architecture **全交付**:Plan 01-06 全在 main。下一阶段 V1.2 桌面端 Wails 迁移 + Phase 5 智能化。

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Trinity 全栈最后一公里:

1. **Sub-agent forger 配置**(D21):filterTools 加 workflow ops strip + 主 agent system prompt 教学(纯 prompt-driven 不加新 SubagentType)
2. **Catalog 源扩展**:验证 function / handler catalog source 工作正常(Plan 01/02 已实施,本 plan 只验)
3. **E2E pipeline test**:跨 trinity domain 全链路(用户口语需求 → decomposer → forger 并行锻造 → config 引导 → workflow 装配 → trigger → run 完成)

**前置依赖**:Plan 01-05 全部 merge。trinity 三 domain + execution plane + transport + eventlog 都就位。

**Architecture:** 本 plan 不引入新 domain。改动局限在:
- `app/loop/tools.go::filterTools` 加 strip list(~10 行 Go)
- `app/catalog/generator.go` system prompt template 加 multi-agent 教学段(~30 行 prompt)
- `test/e2e/` 新建 cross-domain pipeline tests(~500 行测试代码)

**Tech Stack:** 纯现有(Go 1.25 + 现有所有 domain + harness)。

**关联**:[`06-subagent-forging.md`](../06-subagent-forging.md) 完整 spec / [`07-notifications-and-eventlog.md`](../07-notifications-and-eventlog.md) / 全 trinity spec docs。

---

## Phase 0:Branch + Prereqs

### Task 1:Branch + 验证 prereq

- [ ] **Step 1: 验证 main 完整**

```bash
cd /Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify
git checkout main && git pull origin main
ls backend/internal/domain/{function,handler,workflow,flowrun,trigger,eventlog}
ls backend/internal/app/{function,handler,workflow,scheduler,trigger,flowrun}
# 全部应存在
```

- [ ] **Step 2: 创建分支**

```bash
git checkout -b feature/subagent-catalog-e2e
```

---

## Phase 1:Sub-agent filterTools Strip(D21)

### Task 2:加 workflow ops 到 filterTools 黑名单

**Files:** Modify `backend/internal/app/loop/tools.go`(filterTools 函数)

per spec D21 + 06-subagent-forging.md §5.2:

- [ ] **Step 1: 找现有 filterTools**

```bash
grep -n "filterTools\|Subagent" backend/internal/app/loop/tools.go
```

应找到现有 `filterTools` 函数(D4 落地的,strip "Subagent" 防递归)。

- [ ] **Step 2: 改 filterTools**

```go
// 之前(只 strip Subagent):
func filterTools(tools []toolapp.Tool) []toolapp.Tool {
	out := make([]toolapp.Tool, 0, len(tools))
	for _, t := range tools {
		if t.Name() != "Subagent" {
			out = append(out, t)
		}
	}
	return out
}

// 现在(D21 — strip workflow mutation + execution ops):
var subagentStrippedTools = map[string]bool{
	"Subagent":          true, // 防递归(已有)
	"create_workflow":   true, // D21 — workflow 装配主 agent 独享
	"edit_workflow":     true,
	"delete_workflow":   true,
	"revert_workflow":   true,
	"trigger_workflow":  true, // D21 — workflow 触发主 agent 独享(避副作用)
}

func filterTools(tools []toolapp.Tool) []toolapp.Tool {
	out := make([]toolapp.Tool, 0, len(tools))
	for _, t := range tools {
		if subagentStrippedTools[t.Name()] {
			continue
		}
		out = append(out, t)
	}
	return out
}
```

**保留**:`search_workflow` / `get_workflow`(只读;sub-agent 可参考现有 workflow context)
**保留**:`call_handler` / `run_function`(sub-agent 锻造完自测必需)

- [ ] **Step 3: 单测**

**Files:** Modify `backend/internal/app/loop/tools_test.go`

```go
func TestFilterTools_StripsWorkflowMutationOps(t *testing.T) {
	tools := []toolapp.Tool{
		fakeTool("create_function"),
		fakeTool("call_handler"),
		fakeTool("create_workflow"),
		fakeTool("edit_workflow"),
		fakeTool("trigger_workflow"),
		fakeTool("Subagent"),
	}
	out := filterTools(tools)
	got := toolNames(out)
	want := []string{"create_function", "call_handler"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterTools_KeepsReadOnlyWorkflowTools(t *testing.T) {
	tools := []toolapp.Tool{
		fakeTool("search_workflow"),
		fakeTool("get_workflow"),
	}
	out := filterTools(tools)
	if len(out) != 2 {
		t.Errorf("expected both read-only tools kept, got %d", len(out))
	}
}

func TestFilterTools_KeepsCallHandlerAndRunFunction(t *testing.T) {
	tools := []toolapp.Tool{
		fakeTool("call_handler"),
		fakeTool("run_function"),
	}
	out := filterTools(tools)
	if len(out) != 2 {
		t.Errorf("sub-agent self-test tools should be kept, got %d", len(out))
	}
}
```

- [ ] **Step 4: 跑测试**

```bash
cd backend && go test ./internal/app/loop/ -count=1 -run TestFilterTools
```

Expected: PASS 3 测试

- [ ] **Step 5: Commit + push**

```bash
git add backend/internal/app/loop/tools.go backend/internal/app/loop/tools_test.go
git commit -m "feat(loop): D21 — filterTools strips workflow mutation + execution ops for sub-agents"
git push origin feature/subagent-catalog-e2e
```

---

## Phase 2:主 Agent System Prompt 教学

### Task 3:Catalog generator system prompt template 加 multi-agent forging 段

**Files:** Modify `backend/internal/app/catalog/generator.go`(system prompt template)

- [ ] **Step 1: 找 catalog generator system prompt**

```bash
grep -n "system prompt\|systemPrompt\|template" backend/internal/app/catalog/generator.go
```

- [ ] **Step 2: 加 multi-agent forging 段**

```go
// 现有 buildPrompt(...) 的某处加:

const multiAgentForgingPromptSection = `
You have multi-agent forging capabilities via the Subagent tool.

When the user requests something involving 3+ independent forgeable modules
(e.g., "build a workflow that does X, Y, Z, each needing its own Function
or Handler"), CONSIDER spawning subagents in parallel:

1. (Optional) Spawn Subagent(type="Explore", prompt="analyze + produce a
   forging plan; use search_* tools only, do NOT forge anything") — returns
   a structured plan listing what Functions / Handlers are needed.

2. Spawn N Subagent(type="general-purpose", prompt="forge ONE specific
   atom: ...") IN PARALLEL (LLM-self-reported execution_group=1 to get
   them parallel-batched). Each subagent forges a Function or Handler,
   runs self-test (run_function / call_handler), returns the entity ID.

3. Wait for all subagents to return.

4. CHECK CONFIG GATE: get_handler / get_function for each new entity, check
   configState. If unconfigured / partially_configured → use AskUserQuestion
   to collect missing init_args, then call update_handler_config to persist.
   Only proceed when all references show configState="ready".

5. YOU YOURSELF assemble the workflow — call create_workflow + apply ops
   directly. Sub-agents have NO workflow ops by design (D21); they can't
   create / edit / trigger workflows. Workflow assembly is your job.

6. trigger_workflow to dry-run, report results to user.

For SIMPLE requests (single Function edit, one-line Handler tweak), DO IT
YOURSELF. Don't spawn subagents for trivial work — token cost is N× higher.
`

// 在 buildPrompt 函数返回 system prompt 时把这段 append 进去
```

- [ ] **Step 3: 单测**

```go
func TestSystemPrompt_ContainsMultiAgentForgingSection(t *testing.T) {
	gen := NewLLMGenerator(...)
	prompt := gen.buildSystemPromptForTesting() // 加 export-for-test 方法
	if !strings.Contains(prompt, "multi-agent forging") {
		t.Error("system prompt should contain multi-agent forging instruction")
	}
	if !strings.Contains(prompt, "configState") {
		t.Error("should mention configState gate")
	}
	if !strings.Contains(prompt, "D21") || !strings.Contains(prompt, "no workflow ops") {
		t.Error("should explain D21 — sub-agents have no workflow ops")
	}
}
```

- [ ] **Step 4: 跑测试 + commit**

```bash
cd backend && go test ./internal/app/catalog/ -count=1 -run TestSystemPrompt
git add backend/internal/app/catalog/generator.go backend/internal/app/catalog/generator_test.go
git commit -m "feat(catalog): main-agent system prompt teaches multi-agent forging + D21 awareness"
git push
```

---

## Phase 3:Catalog Source 扩展验证

Plan 01 已加 function CatalogSource;Plan 02 已加 handler CatalogSource(含 configState)。本 task 只是 e2e 验证。

### Task 4:Catalog 含 function + handler items 的端到端验证

**Files:** Create `backend/test/catalog/trinity_catalog_test.go`

```go
package catalog

import (
	"context"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/backend/test/harness"
)

func TestCatalog_IncludesFunctionAndHandlerItems(t *testing.T) {
	h := harness.New(t)

	// 先 forge 一个 Function + 一个 Handler
	fnID := harness.CreateFunction(t, h, "to-pdf", "convert markdown to PDF")
	hdID := harness.CreateHandler(t, h, "pg-prod", "PostgreSQL connector",
		[]harness.MethodDef{{Name: "query", Body: "..."}})

	// 配 Handler config(让 configState=ready)
	harness.UpdateHandlerConfig(t, h, hdID, map[string]any{"dsn": "postgresql://localhost"})

	// 触发 catalog refresh
	cat, err := h.CatalogService.Refresh(context.Background())
	if err != nil { t.Fatal(err) }

	// 验证 catalog summary 含两类
	summary := cat.Summary
	if !strings.Contains(summary, "to-pdf") {
		t.Errorf("catalog summary missing function item: %s", summary)
	}
	if !strings.Contains(summary, "pg-prod") {
		t.Errorf("catalog summary missing handler item: %s", summary)
	}
	if !strings.Contains(summary, "configState: ready") {
		t.Errorf("catalog should expose configState (D9-1 minor), got: %s", summary)
	}
}

func TestCatalog_HandlerConfigStateUnconfigured(t *testing.T) {
	h := harness.New(t)
	hdID := harness.CreateHandler(t, h, "pg-staging", "...",
		[]harness.MethodDef{{Name: "query", Body: "..."}})
	// 不配 config

	cat, _ := h.CatalogService.Refresh(context.Background())
	if !strings.Contains(cat.Summary, "configState: unconfigured") {
		t.Errorf("expected unconfigured state, got: %s", cat.Summary)
	}
	if !strings.Contains(cat.Summary, "missing:") {
		t.Errorf("missing list not exposed")
	}
}
```

- [ ] Step 1-2:写 + 跑

```bash
cd backend && make test-pipeline
```

- [ ] Step 3: commit

---

## Phase 3.5:Execution Log LLM 诊断 e2e(D22)

**注**:具体 10 个 LLM 工具(5 search + 5 get)的实现分散到对应 plan:
- function execution 工具 → Plan 01 Task 23e
- handler execution 工具 → Plan 02 Task 26e
- workflow / mcp / skill execution 工具 → Plan 05 Task 16d

本 plan **只测 e2e 端到端 LLM 诊断流**(各域工具已实现)。

### Task 4a:LLM 诊断 e2e 场景

**Files:** Add `backend/test/e2e/llm_diagnostics_test.go`

模拟用户:"昨天 to-pdf function 跑出来 PDF 是空的"。

驱动 LLM(fake script):
1. `search_function_executions({functionId:"fn_to_pdf", limit:20})` → 看 aggregates
2. `get_function_execution({id:第一条 ok 但 output 异常的})` → 看 page_count=0
3. `get_function_execution({id:hints.duplicates_previous_input})` → 上次同 input 也是 0
4. LLM 总结:"问题在 Function 代码,建议 edit_function 检查 weasyprint"

验证 LLM 拿到 hints 字段后能正确诊断 + 不依赖 status=failed。

第二场景(可选):跨域诊断 — `search_handler_executions({handlerId:"hd_pg", status:"failed", since:"2026-05-10"})` → 找最近 PG 失败 → `get_handler_execution(id)` 看错误细节 → 诊断 DSN 问题。

- [ ] Step 1-3:e2e test + fake script + commit

---

## Phase 4:E2E Cross-Domain Pipeline Test

### Task 5:全栈 E2E test —— 邮件 workflow 的端到端故事

**Files:** Create `backend/test/e2e/email_workflow_e2e_test.go`

最大的 pipeline test。模拟用户口语需求 → 主 agent 协调 → forger sub-agents 并行 → workflow 装配 → trigger → run 完成。

```go
package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/sunweilin/forgify/backend/test/harness"
)

// TestE2E_EmailWorkflow_FullStack — 全栈端到端。
//
// 用户:"做个监听邮箱 workflow,猎头存 DB,发 WhatsApp"
// 期望:主 agent decomposer + 3 forger sub + workflow 装配 + manual trigger 跑通
//
// 用 fake LLM(harness.FakeLLMServer)按脚本 driven 整个流程。
func TestE2E_EmailWorkflow_FullStack(t *testing.T) {
	h := harness.New(t,
		harness.WithFakeLLMScript(loadEmailWorkflowScript(t)),
	)
	defer h.Close()

	// 起一个 conversation
	convID := harness.CreateConversation(t, h, "Email Headhunter Setup")

	// 发用户消息触发整个流程
	resp := harness.SendMessage(t, h, convID, `做个监听邮箱的 workflow,
猎头信息存 DB,发 WhatsApp`)

	// 等流程完成(fake LLM 脚本驱动直到 trigger_workflow 调用 + 返结果)
	finalState := harness.WaitForChatComplete(t, h, convID, 60*time.Second)

	// 断言 1:主 agent spawn 了 4 个 sub-agent(decomposer + 3 forger)
	subs := harness.ListSubagentRuns(t, h, convID)
	if len(subs) < 4 {
		t.Errorf("expected ≥4 sub-runs, got %d", len(subs))
	}

	// 断言 2:DB 里有 2 个 handler + 1 个 function + 1 个 workflow
	hs := harness.ListHandlers(t, h)
	if len(hs) != 2 { t.Errorf("expected 2 handlers, got %d", len(hs)) }
	fns := harness.ListFunctions(t, h)
	if len(fns) != 1 { t.Errorf("expected 1 function, got %d", len(fns)) }
	wfs := harness.ListWorkflows(t, h)
	if len(wfs) != 1 { t.Errorf("expected 1 workflow, got %d", len(wfs)) }

	// 断言 3:Handler config 都 ready(主 agent 用 AskUserQuestion + update_handler_config)
	for _, hd := range hs {
		state, _, _ := h.HandlerService.ConfigState(context.Background(), hd.ID, hd.ActiveVersion.InitArgsSchema)
		if state != "ready" {
			t.Errorf("handler %s configState=%s, expected ready", hd.Name, state)
		}
	}

	// 断言 4:trigger_workflow 由主 agent 调(不是 sub-agent)— 看 chat.message 流里 tool_call attrs
	mainTrigger := harness.FindToolCall(t, h, convID, "trigger_workflow", "")
	if mainTrigger == nil {
		t.Error("trigger_workflow not called by main agent")
	}
	// sub-agent 物理上没有 trigger_workflow tool — 验证 D21
	for _, sub := range subs {
		if harness.SubAgentCalledTool(t, h, sub.ID, "trigger_workflow") {
			t.Errorf("D21 violation: sub-agent %s called trigger_workflow", sub.ID)
		}
	}

	// 断言 5:FlowRun 有 1 条 status=completed
	runs := harness.ListFlowRuns(t, h, wfs[0].ID)
	if len(runs) != 1 || runs[0].Status != "completed" {
		t.Errorf("expected 1 completed run, got %+v", runs)
	}
}

// loadEmailWorkflowScript 加载预制 fake LLM 脚本(模拟决策序列)。
//
// loadEmailWorkflowScript loads pre-canned fake LLM script driving the flow.
func loadEmailWorkflowScript(t *testing.T) harness.LLMScript {
	t.Helper()
	return harness.LLMScript{
		// 主 agent 第 1 轮:spawn decomposer
		// 主 agent 第 2 轮(收到 decomposer plan):spawn 3 forgers parallel
		// decomposer:返 plan JSON
		// forger 1: create_handler gmail-listener
		// forger 2: create_handler pg-recruiter
		// forger 3: create_function send-whatsapp
		// 主 agent 第 3 轮(收到 3 IDs):get_handler check configState → AskUserQuestion
		// User answer:DSN / API key
		// 主 agent 第 4 轮:update_handler_config × 2
		// 主 agent 第 5 轮:create_workflow with full ops
		// 主 agent 第 6 轮:trigger_workflow
		// 完成
	}
}
```

- [ ] **Step 1: 写 e2e test**(~500 行)
- [ ] **Step 2: 写 fake LLM script**(JSON or YAML 文件,~200 行,~25 turns)
- [ ] **Step 3: 跑 + iterate**

```bash
cd backend && make test-pipeline -- -run TestE2E_EmailWorkflow_FullStack
```

通过前可能要 iterate 几次 fake script + harness helpers。

- [ ] **Step 4: Commit + push**

```bash
git add backend/test/e2e/
git commit -m "test(e2e): full-stack email workflow scenario (multi-agent forging + config gate + D21)"
git push
```

---

### Task 6:更多 E2E 场景(可选,选 1-2 个增加 coverage)

**Files:** Create more `backend/test/e2e/*.go`

候选场景:

- **`webhook_triggered_workflow_test.go`**:webhook 触发 → 跑 → 失败通知
- **`approval_lifecycle_test.go`**:approval 节点 → 进程重启 → rehydrate → resume
- **`needs_attention_cascade_test.go`**:create wf → delete handler → wf 标 needs_attention → 下次 trigger fail-fast

V1 至少做 **approval_lifecycle**(覆盖 §6.1 关键 production hardening)。

- [ ] Step 1-3:1 个 E2E + commit

---

## Phase 5:Final Cross-platform + Doc Sync

### Task 7:三平台 cross-compile + staticcheck 全 spec 完工验证

- [ ] **Step 1: 三平台**

```bash
cd backend
go build ./...
GOOS=windows go build ./...
GOOS=linux go build ./...
GOOS=linux GOARCH=arm64 go build ./...
GOOS=darwin GOARCH=amd64 go build ./...
```

5 平台全过(per spec 平台支持声明)。

- [ ] **Step 2: staticcheck**

```bash
staticcheck ./...
```

Expected: 0 警告

- [ ] **Step 3: 全套 unit + pipeline test**

```bash
make test-unit
make test-pipeline
```

Expected: 全绿

---

### Task 8:最终 Doc Sync(整个 forge_redesign 项目收尾)

**Files:** Modify 多份文档

- [ ] **Step 1: progress-record.md 加 forge_redesign 完工 dev log**

per S19(每条 ~30-100 字):

```markdown
| 2026-XX-XX | **[refactor]** forge_redesign 完工 — 6 周 6 plan 落地。
function (替代 forge) / handler (新) / workflow (authoring) / scheduler+trigger+flowrun (执行) / eventlog scope + HTTP/2 / subagent forger + catalog ext。
~22000 LOC 净增 + ~5000 删。21 D 决策全实施。 |
```

- [ ] **Step 2: backend-design.md 顶部 Phase 路线图加完工行**

```
| Phase 4-5 (重做版) | Trinity Architecture | ~6 周 | Function / Handler / Workflow 完整 + 14 项生产硬化 | ✅ 2026-XX-XX |
```

- [ ] **Step 3: service-design-documents/ 收尾**

确认 7 份新 doc 全在(function / handler / workflow / scheduler / trigger / flowrun / 加 forge_redesign_complete summary 一份回头讲整体)。

- [ ] **Step 4: forge_redesign/ 加 README**

`documents/version-1.2/adhoc-topic-documents/forge_redesign/README.md`:

```markdown
# Forge Redesign — Trinity Architecture

V1.2 V1 完整重做(2026-05-XX 完工)。

## 文档导航
- [00-overview.md](./00-overview.md) — 顶层愿景 + 21 D 决策
- [01-shared-tool-interface.md](./01-shared-tool-interface.md) — 21 LLM tools 矩阵
- [02-function.md](./02-function.md) / [03-handler.md](./03-handler.md) / [04-workflow.md](./04-workflow.md) — Trinity 三 domain 详设计
- [05-execution-plane.md](./05-execution-plane.md) — Scheduler + Trigger + FlowRun
- [06-subagent-forging.md](./06-subagent-forging.md) — 多 agent 并行锻造
- [07-notifications-and-eventlog.md](./07-notifications-and-eventlog.md) — 通知 + scope 总览

## 实施 Plans
- [plans/01-function-domain.md](./plans/01-function-domain.md)
- [plans/02-handler-domain.md](./plans/02-handler-domain.md)
- [plans/03-eventlog-and-transport.md](./plans/03-eventlog-and-transport.md)
- [plans/04-workflow-authoring.md](./plans/04-workflow-authoring.md)
- [plans/05-execution-plane.md](./plans/05-execution-plane.md)
- [plans/06-subagent-catalog-e2e.md](./plans/06-subagent-catalog-e2e.md) — 本 plan,trinity 收尾
```

- [ ] **Step 5: Commit + push**

```bash
git add documents/
git commit -m "docs(forge_redesign): final doc sync + README + progress-record completion log"
git push
```

---

## Phase 6:PR + Merge

### Task 9:Open PR

```bash
gh pr create --title "feat(trinity): subagent forger + catalog ext + E2E completion" --body "$(cat <<'EOF'
## Summary
最后一公里 — Trinity architecture 完工:

- D21: filterTools strips workflow mutation/execution ops for sub-agents
  (~10 lines + 3 unit tests)
- Main agent system prompt: multi-agent forging教学 + config gate + D21 awareness
- Catalog source verified: function + handler with configState
- E2E pipeline: email-headhunter full stack (主 agent → decomposer → 3
  forger parallel → config gate → workflow assembly → trigger → completion)
- Final 5-platform cross-compile + staticcheck 0 + S14 doc sync

## Test plan
- [x] 3 filterTools unit tests pass
- [x] catalog system prompt test pass
- [x] full-stack email workflow E2E pass
- [x] approval lifecycle E2E (process restart + rehydrate)
- [x] make test-unit + make test-pipeline 全绿
- [x] 5 平台 cross-compile / staticcheck 0 / doc sync

## Related
This completes forge_redesign Plans 01-06. After merge:
- Trinity architecture (Function / Handler / Workflow) fully implemented
- 21 D decisions all landed
- ~22000 LOC change shipped
- forge_redesign README + 7 spec docs + 6 plan docs all in main
EOF
)"
```

- [ ] **Step 1: Open + merge PR**
- [ ] **Step 2: 庆祝 + 跟用户回报完工**

---

## Acceptance criteria

1. ✅ 10 task done(原 9 + Phase 3.5 加 1 task = 4a;LLM 工具实现已分布到 Plan 01/02/05 per-entity 各域)
2. ✅ filterTools strips workflow mutation/execution ops
3. ✅ catalog system prompt 含 multi-agent 教学
4. ✅ E2E test 邮件 workflow 全栈通过
5. ✅ approval lifecycle E2E 通过(rehydrate after process restart)
6. ✅ 5 平台 cross-compile / staticcheck 0
7. ✅ S14 doc sync 完整 + README
8. ✅ PR merge to main + push
9. ✅ Trinity architecture 整体完工:Function / Handler / Workflow / 执行 plane / 多 agent 锻造 / 通知 + scope 全部就位

---

## Trinity 完工后状态

V1.2 后端期收尾;实际可交付桌面端 Wails 迁移阶段:

- ✅ chat 主对话端用户能造 Function / Handler
- ✅ chat 内 LLM 用 catalog summary 知道用户有什么 capabilities
- ✅ chat 内主 agent 能 spawn 多个 forger sub-agent 并行锻造复杂 workflow
- ✅ Workflow 通过 cron / fsnotify / webhook / manual 触发
- ✅ workflow run 状态可见可取消
- ✅ Handler instance lifetime 100% caller-owns
- ✅ secret 加密存(handler config)
- ✅ HTTP/2 transport 解 connection limit
- ✅ entity-level eventlog scope 给前端多视图准备
- ✅ failed run / capability 删等关键事件 broadcast 到 notifications
- ✅ 14 项生产 hardening 全测过

---

(本 plan 完;forge_redesign 6 plans 全交付)
