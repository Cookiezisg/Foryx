# 审计问题修复 — 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:executing-plans（inline）或 subagent-driven-development。步骤用 `- [ ]` 勾选。
> 本计划在 `main` 上 inline 执行（用户决定不开分支）。每个 Task 末尾 commit；全绿后统一 push origin/main。提交信息**不带** AI 署名（项目规矩）。

**Goal:** 把审计报告里除 🟡-B（e2e）外的全部代码级问题改干净,并加 lintprompts 守卫根治"prompt 引用不存在工具名"这一整类。

**Architecture:** 纯 backend 改动。op 键名向全仓主流拼法归一(改 apply.go 这个孤儿);prompt 幽灵名清理;新增薄 `trigger_workflow` 工具包已有引擎;lintprompts 加一条静态守卫。

**Tech Stack:** Go;`make test-backend` 跑单测;`cd backend && go build ./... && staticcheck ./...`;`make verify`(含 lintprompts)。

**Spec:** [`../specs/2026-05-26-audit-fixes-design.md`](../specs/2026-05-26-audit-fixes-design.md)

**顺序:** Task 1 op 键归一 → Task 2 清幽灵名 → Task 3 trigger_workflow 工具 → Task 4 lintprompts 守卫(此时真实 prompt 已干净 → 直接绿)→ Task 5 稿子批注 + doc-sync + 全绿 push。

---

## Task 1: op 键名归一(🔴-2 + 🟡-D + camelCase 洁癖)

**Files:**
- Modify: `backend/internal/app/function/apply.go:117`(deps→dependencies)
- Modify: `backend/internal/app/handler/apply.go:206`(deps→dependencies)、`:131`(init_body→initBody)、`:140`(shutdown_body→shutdownBody)
- Modify: `backend/internal/app/workflow/apply.go:140,163`(node id→nodeId)、`:222,245`(edge 保持 edgeId,改校验文案)
- Modify: 内部 env-fix 发射器 `backend/internal/app/function/create.go:146`、`backend/internal/app/function/edit.go:137`(及 handler 对应处)`{"deps":…}`→`{"dependencies":…}`
- Modify: cheatsheet —— `backend/internal/app/tool/workflow/edit.go:34-35`(edge op `id`→`edgeId`、node op `id`→`nodeId`)、`backend/internal/app/tool/handler/create.go`(set_init/set_shutdown 示例 `init_body`/`shutdown_body`→`initBody`/`shutdownBody`)。(handler set_dependencies 示例本就写 `dependencies` → 改完代码即自洽,无需动)
- Test: `backend/internal/app/function/apply_test.go`、`backend/internal/app/handler/apply_test.go`、`backend/internal/app/workflow/apply_test.go`

- [ ] **Step 1: 先读现有 apply_test.go 的写法**，照同样 harness/断言风格写新测试(`Test<Func>_<Scenario>`,§T1)。读 `function/apply_test.go` 看 ParseOps/Apply 怎么调。

- [ ] **Step 2: 写失败测试** —— 每个 apply.go 加一条"照 cheatsheet 键名喂 op → 断言 state 真填上":
  - function/handler:`set_dependencies` 用键 `dependencies` → 断言 `state.Dependencies == ["psycopg2-binary"]`
  - handler:`set_init` 用键 `initBody` → 断言 InitBody 填上;`set_shutdown` 用 `shutdownBody` 同理
  - workflow:`update_node` 用键 `nodeId` → 断言命中节点;`delete_edge` 用键 `edgeId` → 断言删掉边
  （示例,function：）
  ```go
  func TestApply_SetDependencies_PopulatesFromDependenciesKey(t *testing.T) {
      st := &buildState{} // 按现有测试的真实初始态替换
      err := applyOp(st, ParsedOp{Type: "set_dependencies",
          Raw: json.RawMessage(`{"op":"set_dependencies","dependencies":["psycopg2-binary"]}`)})
      if err != nil { t.Fatalf("apply: %v", err) }
      if len(st.Dependencies) != 1 || st.Dependencies[0] != "psycopg2-binary" {
          t.Fatalf("Dependencies=%v, want [psycopg2-binary]", st.Dependencies)
      }
  }
  ```
  （函数/类型名按实际代码替换 —— Step 1 已读到真名。）

- [ ] **Step 3: 跑测试看它失败** —— `cd backend && go test ./internal/app/function/ ./internal/app/handler/ ./internal/app/workflow/ -run 'TestApply_' -v`。预期 FAIL(当前读 `deps`/`init_body`/`id`)。

- [ ] **Step 4: 改 apply.go 键名** —— 按上面 Files 逐处改 `json:` tag + 校验文案;改内部 env-fix 发射器的 `{"deps":…}`→`{"dependencies":…}`;改两处 cheatsheet。

- [ ] **Step 5: 跑测试看它通过** —— 同 Step 3 命令,预期 PASS。再 `cd backend && go build ./... && staticcheck ./internal/app/...` 干净。

- [ ] **Step 6: 全量对账** —— `grep -rn 'json:"' internal/app/{function,handler,workflow}/apply.go` 逐 op 跟各自 cheatsheet 核对,确认无其它 mismatch(发现就一并归一 + 补测试)。

- [ ] **Step 7: Commit** —— `git add backend/internal/app/{function,handler,workflow}/ && git commit -m "fix(forge): canonicalize op keys (deps→dependencies, node/edge id→nodeId/edgeId, init/shutdown→camelCase)"`

---

## Task 2: 清 prompt 幽灵 / 错写工具名(🔴-1 + 🟡-C)

**Files:**
- Modify: `backend/internal/app/askai/forge_context.go:51,53,99,100,140,141`
- Modify: `backend/internal/app/askai/triage_context.go:99`
- Modify: `backend/internal/app/subagent/registry.go:18,19,24`(去 LS、search_forges→真名)
- Modify: `backend/internal/app/tool/mcp/search.go:22`(call_mcp→call_mcp_tool)
- Modify: `backend/internal/app/tool/mcp/call.go:41`(search_mcp→search_mcp_tools)
- Modify: `backend/internal/app/chat/multi_agent_prompt.go:24-28`(configState 限定到 handler)
- Modify: `backend/internal/app/tool/function/revert.go:20`(去 list_function)

- [ ] **Step 1: edit_forge → 真实工具名** ——
  - `forge_context.go` function(51-53):`edit_forge`→`edit_function`,`functionId=%q`→`id=%q`
  - handler(99-100):`edit_forge`→`edit_handler`,`handlerId=`→`id=`
  - workflow(140-141):`edit_forge`→`edit_workflow`,`workflowId=`→`id=`(ops 列表 add_node/update_node/... 保留)
  - `triage_context.go:99`:`call `+"`edit_forge`"+` (function/handler/workflow)` → `call `+"`edit_function` / `edit_handler` / `edit_workflow`"+`(按实体)`

- [ ] **Step 2: 幽灵名簇** ——
  - `subagent/registry.go:19` Explore `AllowedTools`:`"LS", "search_forges"` → `"search_function", "search_handler", "search_workflow"`(去 LS)
  - `registry.go:18,24` Explore/Plan 两个 prompt 文案里的 `LS` 删掉(Glob 已覆盖列目录)
  - `mcp/search.go:22`:描述里 `call_mcp` → `call_mcp_tool`
  - `mcp/call.go:41`:`as returned by search_mcp` → `search_mcp_tools`
  - `multi_agent_prompt.go:24-28`:把 "get_handler / get_function … check configState" 改成只对 handler 查 configState(function 无此概念)
  - `tool/function/revert.go:20`:`Use list_function or get_function` → `Use get_function`

- [ ] **Step 3: grep 验证幽灵名清零** —— 
  ```
  cd backend && for t in edit_forge search_forges 'return "LS"' 'list_function'; do echo "== $t =="; grep -rn "$t" internal/app --include=*.go | grep -v _test; done
  grep -rn 'call_mcp\b' internal/app/tool/mcp/search.go; grep -n 'search_mcp\b' internal/app/tool/mcp/call.go
  ```
  预期:edit_forge/search_forges/list_function 在非测试代码中清零;call_mcp/search_mcp 在那两文件中变全名。(subagent_test.go 里 edit_forge/trigger_workflow 是 deny-list 测试桩,Task 3 后再看是否清。)

- [ ] **Step 4: build 干净** —— `cd backend && go build ./... && go test ./internal/app/subagent/ ./internal/app/chat/ -count=1`(确保 prompt 改动没破坏现有 prompt 相关测试,如 multi-agent prompt 断言)。若有断言旧文案的测试 → 同步更新。

- [ ] **Step 5: Commit** —— `git add backend/internal/app/{askai,subagent,tool/mcp,chat,tool/function}/ && git commit -m "fix(prompts): replace ghost tool refs (edit_forge→edit_*, search_forges/LS/call_mcp/search_mcp/list_function)"`

---

## Task 3: 新增 `trigger_workflow` 工具(🟡-A)

**Files:**
- Create: `backend/internal/app/tool/workflow/trigger.go`
- Modify: `backend/internal/app/tool/workflow/workflow.go`(`WorkflowTools` 或新 registrar 注入 trigger/scheduler)
- Modify: `backend/cmd/server/main.go`(接线:把 trigger/scheduler 服务传进 workflow 工具注册)
- Modify: `backend/internal/app/chat/multi_agent_prompt.go:34`(step 6 引用即成真;若加 dryRun 则措辞同步)
- Test: `backend/internal/app/tool/workflow/trigger_test.go`

- [ ] **Step 1: 核实引擎入口签名 + 干跑语义** —— 读:
  - `backend/internal/app/scheduler/scheduler.go`(`StartRun` 签名:入参 userID/workflowID/trigger 信息?返回 flowrunID?)
  - `backend/internal/app/trigger/trigger.go`(`FireManual` 签名)
  - `backend/internal/app/scheduler/retry.go` + `state.go`(找 dry-run mock 输出 → StartRun 是否有 dryRun 形参/选项)
  - 一个现成工具做模板:`backend/internal/app/tool/workflow/` 里 `search.go` 或 `get.go`(看 9 方法骨架 + 怎么拿 ctx user + forgePub 注入风格)
  **结论落到这里**(执行时填):StartRun 签名 = `____`;dryRun 支持 = 是/否 → dryRun 参数策略 = `____`。

- [ ] **Step 2: 写失败测试** —— `trigger_test.go`:构造带 fake/stub scheduler 的工具,`Execute({"workflowId":"wf_x","summary":"…"})` → 断言调到 StartRun/FireManual 且返回含 flowrunId 的 JSON;`ValidateInput` 缺 workflowId → err。先跑 → FAIL(工具不存在)。
  Run: `cd backend && go test ./internal/app/tool/workflow/ -run TestTriggerWorkflow -v` → FAIL。

- [ ] **Step 3: 实现工具** —— `trigger.go` 实现 §S18 九方法:
  - `Name()="trigger_workflow"`;`Description()` 一句话(≤~80 token);`Parameters()` = `{workflowId required, dryRun? bool}`(不含 summary/destructive/execution_group —— framework 注入)
  - `IsReadOnly()=false`;`RequiresWorkspace()=false`;`NeedsReadFirst()=false`
  - `Execute`:解析 args → 调 Step 1 确认的 `FireManual`/`StartRun`(从 ctx 取 userID,§S9)→ 返回 `{flowrunId, status}` JSON;失败转 tool_result 错误文本
  - 构造函数 `TriggerWorkflowTool(sched, log)` 注入依赖

- [ ] **Step 4: 注册 + 接线** —— `WorkflowTools(...)` 增参(scheduler/trigger 服务)并 append 新工具;`main.go` 调用处把已构造的 `schedulerService`/`triggerService` 传进去(注意 main.go 里 scheduler 在 workflow 工具构造之后才建 —— 可能需把 trigger_workflow 的注册挪到 scheduler 建好之后,类似 `WorkflowExecutionTools(flowrunRepo)` 在 main.go:543 的延后注册模式)。

- [ ] **Step 5: 跑测试 + build** —— `cd backend && go test ./internal/app/tool/workflow/ -run TestTriggerWorkflow -v`(PASS)+ `go build ./... && staticcheck ./internal/app/tool/workflow/ ./cmd/server/`。

- [ ] **Step 6: multi_agent_prompt step 6** —— 确认 `:34` 的 `trigger_workflow to dry-run` 现在指向真实工具;若 Step 1 结论是"无真干跑",把措辞从 "dry-run" 改成准确说法(如 "trigger a test run")。

- [ ] **Step 7: Commit** —— `git add backend/internal/app/tool/workflow/ backend/cmd/server/main.go backend/internal/app/chat/multi_agent_prompt.go && git commit -m "feat(tool): add trigger_workflow tool wrapping existing scheduler engine"`

---

## Task 4: lintprompts 防复发守卫(D4)

**Files:**
- Modify: `backend/cmd/lintprompts/main.go`(加 rule + realTools 静态扫 + roots 补 askai)
- Test: `backend/cmd/lintprompts/main_test.go`(新建)

- [ ] **Step 1: 写失败测试** —— `main_test.go`:
  - `TestGhostToolRule_FlagsUnknownTool`:喂一段含 反引号 `nonexistent_tool` 的 prompt + 一个 realTools 集(不含它)→ 期望 violated=true
  - `TestGhostToolRule_PassesRealToolAndAllowlist`:喂含 `edit_function`(real)和 `dryRun`(allowlist)的 prompt → violated=false
  Run: `cd backend && go test ./cmd/lintprompts/ -v` → FAIL(函数未定义)。

- [ ] **Step 2: 实现 realTools 静态扫** —— 在 lintprompts 加函数:walk `internal/app/tool`,正则抓 `func \([^)]*\) Name\(\) string \{\s*return "([^"]+)"` 的捕获组,build `map[string]bool`。(与审计时用的同法。)

- [ ] **Step 3: 实现 ghost-tool rule** —— 新 rule:正则抓 prompt 里 反引号包裹、形如 `^[a-z][a-z0-9_]*$`(snake)或已知 PascalCase 工具(`Read|Write|Edit|Glob|Grep|Bash|BashOutput|KillShell|WebFetch|WebSearch|Subagent|AskUserQuestion|TodoCreate|TodoUpdate|TodoList|TodoGet`)的 token;若 token ∉ realTools ∧ ∉ allowlist → 违例。allowlist = 参数/概念集(`dryRun configState summary destructive execution_group init_body initBody ...` —— 执行时按实际 prompt 里出现的非工具反引号 token 补全)。

- [ ] **Step 4: roots 补 askai** —— `main.go:141` 的 `roots` 加 `"internal/app/askai"`。

- [ ] **Step 5: 跑测试 + 跑真实** —— `cd backend && go test ./cmd/lintprompts/ -v`(PASS);再 `go run ./cmd/lintprompts`(在 backend/ 下)→ **预期 0 违例**(Task 2/3 已清干净)。若报出残留 → 那就是漏网的幽灵,回头补;若报出参数被误判 → 补 allowlist。

- [ ] **Step 6: Commit** —— `git add backend/cmd/lintprompts/ && git commit -m "feat(lint): lintprompts flags prompt refs to nonexistent tools; scan askai"`

---

## Task 5: 稿子批注 + doc-sync + 全绿 + push(D3 + §S14)

**Files:**
- Modify: `documents/version-1.2/capability-disclosure-design.md`(§6/§10/§4.6 加批注)
- Modify: `documents/version-1.2/service-design-documents/workflow.md`(trigger_workflow 工具) + `backend-design.md`(workflow 工具数 6→7) + `progress-record.md`([fix] dev log)
- 可能 Modify: `CLAUDE.md`(若工具/op 规约有需要补,预计无)

- [ ] **Step 1: capability-disclosure 批注** —— §6/§10/§4.6 顶部各加一句:"⚠️ 2026-05-26 订正:执行引擎(`scheduler` ~2587 行)已交付且可达,`trigger_workflow` 工具本轮已补;本节原'未实现/假定存在'措辞作废。"

- [ ] **Step 2: doc-sync** ——
  - `service-design-documents/workflow.md`:新增 `trigger_workflow` 工具条(入参 + 包 scheduler)
  - `backend-design.md`:架构树 `app/tool/workflow/` 的 "6 + 2 = 8 LLM tools" → 补 trigger_workflow(变 9)
  - `progress-record.md`:§2 末加 `[fix]` dev log(本轮:op 键归一 + 幽灵名清理 + trigger_workflow 工具 + lintprompts 守卫;测试 +N;指向审计报告)

- [ ] **Step 3: 全量门禁** —— 
  ```
  make test-backend    # 全绿
  cd backend && go build ./... && staticcheck ./...   # 干净
  make verify          # 含新 lintprompts 规则,0 违例
  ```
  任一红 → 修到绿。

- [ ] **Step 4: 最终 commit + push** —— 
  ```
  git add documents/ CLAUDE.md docs/superpowers/
  git commit -m "docs: sync workflow trigger tool + annotate capability-disclosure premise; audit-fix dev log"
  git push origin main
  ```
  （文档对齐那批未提交改动也一并在此前后纳入提交 —— 见下方"未提交存量"。）

---

## 未提交存量(执行前必须处理)

`main` 工作树现有**上一轮文档对齐**的未提交改动(6 改 + 4 未跟踪含审计报告 + 本 spec/plan)。这些与本次代码修复正交,建议**第一步先单独提交**它们,免得跟代码改动混在一个 commit:
```
git add CLAUDE.md documents/version-1.2/{backend-design,progress-record}.md \
        documents/version-1.2/service-contract-documents/{api-design,error-codes,events-design}.md \
        documents/version-1.2/completeness-audit*.md documents/version-1.2/tool-rewrite-catalog.md \
        documents/version-1.2/capability-disclosure-design.md docs/superpowers/
git commit -m "docs(audit): completeness audit report + post-audit doc realignment"
git push origin main
```
(capability-disclosure 的批注在 Task 5 再追加一个 commit。)

---

## Self-Review(写完计划自查)

- **Spec 覆盖:** spec §1.1→Task1;§1.2/1.3→Task2;§1.4→Task3;§1.5→Task4;§1.6→Task5;§2 测试散在各 Task;§3 doc-sync→Task5。✅ 全覆盖。
- **Placeholder:** Task1 测试的 `buildState`/`applyOp` 是占位真名(Step1 读真名替换)——已明确标注"按实际替换",非 TBD。Task3 Step1 的签名结论是"执行时填"——这是合法的"先读再写"步骤,非空泛占位。
- **类型一致:** trigger_workflow 工具名/参数(workflowId/dryRun)前后一致;realTools/allowlist 命名前后一致。
- **顺序正确:** 守卫(Task4)在幽灵清理(Task2)+ trigger_workflow(Task3)之后 → 跑真实 prompt 直接绿,green-at-every-commit。✅
