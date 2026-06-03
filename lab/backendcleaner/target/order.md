# backend-new 重写顺序（依赖拓扑驱动）

> 本文是"基础 → 复杂"的执行路径，确保**不重不漏**。
> 顺序的依据是 `go list` 实测的 import 依赖图（2026-06-03 扫描），不是拍脑袋。
> 进度状态在 `STATE.md`（单一事实源），本文只定"顺序 + 模块边界 + 旗标"。

---

## 0. 已验证的依赖事实

- **domain 层干净**：28 个 domain 包里只有 4 条 domain→domain 依赖（`agent→model`、`conversation→document,model`、`forge→eventlog`、`workflow→model`），**没有任何 domain 反向依赖 app/infra**。地基层架构是对的，重写风险低。
- **app 层是 DAG，无强连通环**：`scheduler`/`trigger` 通过依赖倒置（接口注入，不 import 对方包）连接；`tool` 子包用 DIP（service 依赖 `tool` 基础接口、`tool/<x>` 适配器依赖 service）避环。**所以"一个一个、基础→复杂"可行，不会卡环。**
- 重写单元 = **垂直切片**：一个业务模块 = `domain/<m>` + `infra/store/<m>` + `app/<m>` + `app/tool/<m>`（若是工具）+ `transport/.../handlers/<m>.go`（若有 HTTP）。粒度见 `module-template.md`（按需取层）。

---

## 1. 重写波次（每波次内可并行，跨波次严格串行）

> 规则：写模块 X 时，它依赖的所有模块必须已在 `backend-new/` 就绪。
> 每波次收尾：`cd backend-new && go build ./...` + smoke（启动 / health / 用户初始化）必须绿。

### 波次 0 — 地基（无业务逻辑，所有上层的根）

| 编号 | 模块 | 说明 | 旗标 |
|---|---|---|---|
| M0.1 | `pkg/*` 纯工具 | idgen, pagination, reqctx, tokencount, pathguard, userpath, jsonrepair, wikilink, limits | ⚠️ `modelcaps`（modelcatalog 已取代，疑残留）、`forge`/`agentstate`/`envfix`/`installprogress`/`llmclient`/`llmcost`/`llmparse`/`notifications`/`eventlog` 逐个判定去留 |
| M0.2 | `infra/db` | modernc sqlite + 迁移 + schema_extras | 边界：schema 可激进重定（无需保留数据） |
| M0.3 | `infra/logger` `infra/crypto` | zap + AES-GCM | |
| M0.4 | `domain/errors` `domain/eventlog` `domain/notifications` | 横切契约 | |
| M0.5 | `infra/eventlog` `infra/notifications` `infra/chat` | SSE 三流底座（E1/E2/E3） | |
| M0.6 | `infra/llm` | 自有 provider 客户端（18 文件）+ factory | 边界：provider wire 格式冻结；`mock.go` 留给测试 |

### 波次 1 — 叶子业务域（只依赖地基）

| 编号 | 模块 | app→ 依赖 | 旗标 |
|---|---|---|---|
| M1.1 | `user` | — | |
| M1.2 | `apikey` | domain/crypto | |
| M1.3 | `model` | domain/apikey | ⚠️ `infra/store/modelcapoverride` + `pkg/modelcaps`（旧 model 范式残留） |
| M1.4 | `relation` | — | 横切（实体关系图） |
| M1.5 | `catalog` | — | 核心：被超多模块依赖（trinity catalog 源），重点审实现 |
| M1.6 | `mention` | — | |
| M1.7 | `memory` | domain/errors | |
| M1.8 | `sandbox` | infra/sandbox | 边界：mise binaries = generated |
| M1.9 | `permissions` / `hooks` | — | |
| M1.10 | `document` | catalog, relation, mention | LLM-ranked attach（**无 RAG**），Notion 树 |
| M1.11 | `todo` | — | ⚠️ 待判定：是否 Quadrinity/agent 真需要 |

### 波次 2 — tool 基础 + 执行原语

| 编号 | 模块 | 依赖 | 旗标 |
|---|---|---|---|
| M2.1 | `tool`（基础接口） | infra/llm | S18 九方法接口；framework 注入 summary/destructive |
| M2.2 | `loop`（ReAct 引擎） | tool | 被 chat/agent/subagent/skill 共享 |
| M2.3 | `tool/filesystem` `tool/search` `tool/web` `tool/toolset` | tool | 叶子工具适配器 |

### 波次 3 — Quadrinity 执行体

| 编号 | 模块 | app→ 依赖 | 旗标 |
|---|---|---|---|
| M3.1 | `function` | sandbox | |
| M3.2 | `handler` | sandbox | |
| M3.3 | `subagent` | loop, tool | |
| M3.4 | `agent` | loop, tool | 🔧 in-flight：execution 面对齐 function/skill（当前未提交改动）|
| M3.5 | `skill` | subagent | |
| M3.6 | `mcp` | infra/mcp | ⚠️ `infra/store/mcpcalls`+`mcphealth` 判定 |
| M3.7 | tool 适配器 | `tool/function` `tool/handler` `tool/agent` `tool/skill` `tool/subagent` `tool/mcp` `tool/memory` `tool/document` `tool/shell` | ⚠️ `domain/forge` 在此被依赖（SSE bridge）→ 判定 forge 角色 |

### 波次 4 — 编排核心（最复杂，重灾区）

| 编号 | 模块 | app→ 依赖 | 旗标 |
|---|---|---|---|
| M4.1 | `workflow` | document, function, handler, mcp, skill | 5-node DAG 规格 + CEL；**13→5 节点收敛在此定型** |
| M4.2 | `flowrun`（domain+store: flowrun/flowrunevent/approval） | — | Journal 真相模型 |
| M4.3 | **`scheduler`** | agent, function, handler, loop, mcp, skill, tool, workflow | 🔴 **最大重灾区**：删 topo-walk 旧链（state/pause/subdag/retry 旧半）；14 dispatcher 收 5；`LoopDispatcher` 删→结构化 loop 取代；只保留 durable interpreter |
| M4.4 | `trigger`（+ infra/trigger: cron/fsnotify/webhook/polling） | workflow | inbox 单事务 claim；⚠️ polling listener 是否真需要 |
| M4.5 | `tool/workflow` | scheduler, workflow | |

### 波次 5 — 对话与上下文

| 编号 | 模块 | app→ 依赖 | 旗标 |
|---|---|---|---|
| M5.1 | `conversation` | — | |
| M5.2 | `chat` | document, hooks, loop, tool, tool/permissionsgate | runner 庞大，重点审 |
| M5.3 | `contextmgr` | — | compaction |
| M5.4 | `tool/permissionsgate` | hooks, tool | |

### 波次 6 — 顶层智能编排

| 编号 | 模块 | app→ 依赖 | 旗标 |
|---|---|---|---|
| M6.1 | `askai` | agent, chat, conversation, document, function, handler, workflow | :iterate / :triage（N5） |
| M6.2 | `ask` + `tool/ask` | — | ⚠️ **强残留嫌疑**：疑似 askai 旧版；判定后大概率删 |

### 波次 7 — transport + wiring

| 编号 | 模块 | 说明 | 旗标 |
|---|---|---|---|
| M7.1 | `httpapi/response` `middleware` `router` | 框架级通用层 | |
| M7.2 | `httpapi/handlers/*.go`（36 文件） | 按 API 资源域，对应各 service | ⚠️ `dev_*`（5 个，dev-only 边界）；⚠️ `answers/scenarios/prompts/capabilities/context_stats/metrics/usage`（AI 加的信息端点，逐个审是否真有产品旅程用到）|
| M7.3 | `cmd/server`（DI 装配） | main.go | 装配收口；只注册 5-node |
| M7.4 | `cmd/desktop`（wails）`cmd/resources` `cmd/doc-*` `cmd/lintprompts` `cmd/coverage-matrix` | 入口 + 工具 | embed = generated |

---

## 2. 覆盖阶段（所有波次完成后）

1. `backend-new` 自证完整：`go build ./...` 全平台 + 全部新测试绿 + 能力对账表（`capability-ledger.md`）全勾。
2. 覆盖：`rm -rf backend && mv backend-new backend`（module path 已是最终值，import / Makefile 零改动）。
3. 前端 / testend 兼容：按 `contract-changes.md` 逐条施工。
4. 全链路 e2e + 全量 verification。

---

## 3. 跨模块依赖问题：就地登记

重写某模块时若发现"它依赖的下游模块设计有问题、需要等那一轮调整"，**不要当场跨界改**，在该下游模块的 `target/rounds/` 记录或 `contracts/<下游>.md` 顶部"待调整"区登记，留给那一轮处理。已知大项先列在此：

- `scheduler`（M4.3）：topo-walk 整条旧链删除、14→5 dispatcher 收敛——见独立审计（已深挖）。
- `ask`/`askai`（M6）：双实现合并决策。
- `forge`（domain/infra/pkg 三处）：判定是"SSE 锻造基础设施"（保留）还是旧实体（删）。
