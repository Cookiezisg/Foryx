# 审计问题修复 — 设计稿（post-audit fixes）

> 状态：设计稿 → writing-plans → 实现
> 日期：2026-05-26
> 分支：`main`（用户决定直接在 main 上做，不另开分支；提交自动 push origin/main）
> 来源：[`documents/version-1.2/completeness-audit-report.md`](../../../documents/version-1.2/completeness-audit-report.md)

## 0. 背景与范围

审计报告列出 2 🔴 + ~14 🟡。其中**契约文档类已在 2026-05-26 文档对齐轮修掉**（🟡-E 错误码、🟡-F block 6→7 / document / env_rebuilt、🟡-G :resync、🟡-A 的文档面 + 项目记忆）。

本设计覆盖**剩余的代码级问题** + 两项根治/防复发。**明确不含 🟡-B**（`make e2e` pipeline 编译失败 / harness 签名漂移）——本轮不修。

核心取向（用户原话）：**改得干净清楚，可以大刀阔斧；重度洁癖**。

## 1. 修复项（方案已与用户逐条敲定）

### 1.1 op 键名归一（🔴-2 set_dependencies + 🟡-D edit_workflow + 风格洁癖）

**原则：一个概念一个键名，全仓统一；`apply.go` 的孤儿键向主流拼法看齐。** 不用"两个键都认"的兼容 hack。

- **依赖键 → `dependencies`**（干掉孤儿 `deps`；HTTP 结构体 / domain 模型 / cheatsheet / LLM 直觉全是 `dependencies`）：
  - `internal/app/function/apply.go`、`internal/app/handler/apply.go` 的 `set_dependencies`：`json:"deps"` → `json:"dependencies"`
  - 内部 env-fix 重试发的 `{"deps":…}`（`function/create.go`、`function/edit.go`，及 handler 对应处）→ `{"dependencies":…}`
- **node / edge op → 显式 `nodeId` / `edgeId`**（比裸 `id` 自解释、零歧义）：
  - `internal/app/workflow/apply.go`：`update_node`/`delete_node` 的 `json:"id"` → `json:"nodeId"`；`update_edge`/`delete_edge` 保持 `json:"edgeId"`；校验文案同步
- **camelCase 归一**（满足洁癖 + 合 N3）：handler `set_init`/`set_shutdown` 的 `init_body`/`shutdown_body` → `initBody`/`shutdownBody`（全仓仅剩这两个 snake_case payload key）
  - op 名本身（`set_meta`/`add_node` 等）是**字符串枚举值**、不是 JSON key，全仓已一致的 snake_case，N3 不管枚举值 → **不动**
- **cheatsheet 同步**：`create_function`/`create_handler`/`edit_workflow` 等工具描述里的 op 示例键名全部对齐以上
- **全量对账**：实现时逐 op 核对三个 `apply.go` ↔ 各自 cheatsheet，清掉其它潜在 mismatch

### 1.2 edit_forge 幽灵引用（🔴-1）

`internal/app/askai/forge_context.go`（function/handler/workflow 三处）+ `internal/app/askai/triage_context.go`：
- `edit_forge` → 对应的 `edit_function` / `edit_handler` / `edit_workflow`
- 参数提示 `functionId=` / `handlerId=` / `workflowId=` → `id=`
- triage 里的 `edit_forge (function/handler/workflow)` → 列出三个真实工具名

### 1.3 幽灵 / 错写工具名清理（🟡-C）

- `internal/app/subagent/registry.go`：Explore `AllowedTools` 里 `search_forges` → `search_function`/`search_handler`/`search_workflow`；去掉 `LS`；Explore/Plan 两个 system prompt 文案里的 `LS` 一并去掉
- `internal/app/tool/mcp/search.go`：描述里 `call_mcp` → `call_mcp_tool`
- `internal/app/tool/mcp/call.go`：参数描述里 `search_mcp` → `search_mcp_tools`
- `internal/app/chat/multi_agent_prompt.go`：第 4 步 "`get_function` … check configState" → 把 config 门限定到 handler（function 无 configState）
- `internal/app/tool/function/revert.go`：描述去掉不存在的 `list_function`（保留 `get_function`）

### 1.4 新增 `trigger_workflow` 工具（🟡-A，决策 A）

- 新建工具（归 `internal/app/tool/workflow/`），包一层已存在的 `trigger.FireManual` / `scheduler.StartRun`
- 入参：`workflowId`（必填）+ `dryRun`（可选 bool）+ 标准注入字段（summary/destructive/execution_group）
- **先核实引擎干跑语义**：`scheduler/retry.go` 有 dry-run mock 输出迹象——确认后决定 `dryRun` 如何落到 `StartRun`（若引擎无真干跑，则要么去掉 dryRun、要么 prompt 措辞改"试跑=真跑一次"）
- 注册：在 `WorkflowTools`（或同包新 registrar）注入 trigger/scheduler 服务；`cmd/server/main.go` 接线
- 满足 §S18 九方法；`multi_agent_prompt` 第 6 步引用即成真

### 1.5 lintprompts 防复发守卫（🟡-A，决策 D4）

扩 `backend/cmd/lintprompts/main.go`，新增一条 rule —— **prompt 引用了不存在的工具名 → 报违例、非 0 退出**：
- **权威工具名集**：静态扫 `internal/app/tool/**` 里 `func (…) Name() string { return "X" }` 的字面返回值，构建 `realTools` 集（零维护、自更新——审计时已验证此法可行）
- **提取 prompt 里的工具引用**：扫 prompt 文本里反引号包裹、且匹配工具名形（`snake_case` 或已知 PascalCase 如 `Read`/`Bash`/`Subagent`）的 token
- **排除非工具 token**：一份小 allowlist（参数 / 概念，如 `dryRun`/`configState`/`summary`/`destructive`/`execution_group`/`init_body` 等）——避免把参数键误判成工具
- **roots 补 `internal/app/askai`**（当前漏扫——edit_forge 就藏这）
- 判定：反引号 token ∈ 工具名形 ∧ ∉ realTools ∧ ∉ allowlist → 违例
- 这一条能一次性关掉本轮整类（edit_forge / trigger_workflow / search_forges / list_function / call_mcp / search_mcp），且以后 `make verify` 自动拦截

### 1.6 capability-disclosure 稿订正（🟡-A，决策 D3）

`documents/version-1.2/capability-disclosure-design.md`：在 §6 / §10 / §4.6 顶部各加一句批注——执行引擎已交付且可达，`trigger_workflow` 工具本轮已补；**不重写正文**。

## 2. 测试策略（TDD where applicable）

- **op 键名（1.1）**：每个 `apply.go` 加"照 cheatsheet 原样喂 op → 断言 state 真被填上"的测试（`set_dependencies` 真填 deps、edge op 真按 `edgeId` 命中、node op 真按 `nodeId` 命中、`initBody` 真填）。先写测试复现静默丢失 → 再改。
- **trigger_workflow（1.4）**：单测 ValidateInput / CheckPermissions / Execute（包 fake scheduler）；若引擎支持干跑，加 dryRun 路径断言
- **lintprompts 守卫（1.5）**：守卫自身单测（喂引用不存在工具名的假 prompt → 期望 fail；喂干净 prompt → pass）；并跑一遍真实 prompt 全过
- **prompt 字符串改动（1.2/1.3）**：靠 1.5 的守卫做回归保障（改完守卫必须绿）
- **全量门禁**：`make test-backend` 全绿 + `cd backend && go build ./... && staticcheck ./...` 干净 + `make verify`（含新 lintprompts 规则）

## 3. 文档同步（§S14）

- 新 `trigger_workflow` 工具 → `service-design-documents/workflow.md`（或 tool 文档）+ `backend-design.md` 工具清单 + `api-design.md`（若涉端点，预计无，走 chat 工具）+ `progress-record.md`
- op 键名改动 → `service-design-documents/{function,handler,workflow}.md` 若有 op 速查则同步
- `progress-record.md` 加 `[fix]` dev log（本轮做了什么 + 测试数）
- CLAUDE.md：§S18/§S15 若 Tool 注册模式有变则更新（预计无）

## 4. 非目标（本轮不做）

- 🟡-B `make e2e` pipeline 编译失败（harness 签名漂移）——明确不修
- 能力披露层重构本体（只订正稿子假前提，不动 token 治理工程）
- op 名（`set_meta` 等枚举值）camelCase 化——已一致、非 bug、动它纯找麻烦（YAGNI）
- 4 条未文档化的 dev-gated 路由（export/llm-trace/context-stats 等）——非误导，略

## 5. 落地顺序（建议）

1. 1.5 lintprompts 守卫**先上**（含 realTools 静态扫 + allowlist）→ 立刻把所有幽灵引用照出来当 checklist
2. 1.2 + 1.3 prompt 幽灵名清理 → 守卫转绿
3. 1.1 op 键名归一 + 测试
4. 1.4 trigger_workflow 工具（+ 干跑语义核实）→ 守卫对 `trigger_workflow` 也转绿
5. 1.6 稿子订正 + §3 文档同步
6. `make verify` + `make test-backend` 全绿 → commit（分逻辑块）+ push main
