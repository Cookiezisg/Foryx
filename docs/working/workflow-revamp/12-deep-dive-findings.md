---
id: WRK-001-13
type: working
status: active
owner: @weilin
created: 2026-05-20
reviewed: 2026-05-31
review-due: 2026-08-31
audience: [human, ai]
landed-into:
---
# 12 — 深挖发现(8 subagent 并行盘点 patch 11)

脑爆结论笔记(2026-05-29)。
2026-05-31 改向 durable execution(详 [`00-overview.md`](./00-overview.md))。

依赖:00-11 全部。本 doc patch [11-integration-chains.md](./11-integration-chains.md) 的初版盘点 — 深挖出 doc 11 漏掉的:**Memory 给 agent 节点的 严重员工思维漏洞 / Forge SSE 现有协议状态 / Relations 6 种新 kind / Catalog 已天然 "永远 prod" 合规 / Lazy 分组方案 / Frontend 5 新 feature slice**。

> 执行底盘已从旧的 message-queue + actor 模型改向 **durable execution**(详 [`00-overview.md`](./00-overview.md)):节点 = activity(记账步骤),执行器照图走,事件日志(journal)记每步结果,崩了确定性重放。本篇凡涉执行模型处一律按此理解;本篇结论本身**多数与执行模型无关**(forge SSE / relations / catalog / 跨域涟漪 / HTTP API / 测试基建 / frontend),改向不影响。受影响的只有 S1(lazy 分组,见下)、S7 的执行引擎 seam、综合改造规模表里执行引擎相关项 —— 已就地对齐。

---

## 1 个实现期注意点

### Agent 节点不接 memory — 产品决策,链路隔离

**产品决策(2026-05-29 拍)**:**agent 节点不支持 memory**,跟 subagent / 临场 skill search / 临场 forge 一致 — 员工思维不给"老板能力"。

实现期注意点:**不能复用 chat 老板的 `SystemPromptProvider` 注册表**(它默认带 memory + 临场 skill 等),否则 agent 跑时 memory 会自动注入。

**修法**:在 `app/agent/dispatch.go` 走**独立 system prompt 装配链**,只组装 agent.prompt + skill(挂载的)+ knowledge(挂载文档)+ tools(挂载 callables)。从根上不接 memory / subagent / 临场 skill search 这些老板能力。

跟 chat 老板系统 prompt 完全两套机制,**不靠 flag suppress,靠链路隔离**。

---

## Subagent 各自关键发现

### S1. Lazy 分组 — domain-6(旧"11 组"已被推翻)

> ⚠️ **结论已更新(doc 14 收口)**:本节原写"拆成 11 组"是**旧脑爆结论,已被 Round-3 LLM 实测推翻**。最新定论是 **按域分组(domain 原则,不拆 edit/use)**——研究验证时是 6 组(function / handler / workflow / mcp / document / skill);**本 doc 把 agent 升为第 4 个 forge 实体后是 7 组**(详 §C1 与 [`11-integration-chains.md`](./11-integration-chains.md) §C1)。详 [`14-llm-validation-research-record.md`](./14-llm-validation-research-record.md) §3.6 + §4。下面保留旧提案作为推演记录,但**不要据此施工**。

**为什么"11 组"被推翻**:当年觉得"6 组不行"的真变量不是组数,而是 **`search_*` 工具的位置**——把 7 个 `search_*` 提到 **Resident** 后,domain-6 的「激活对组率」62% 反而**优于** 11-edituse 的 46%(细分 *-edit / *-use 让模型搞混选错组)。所以正确改动是:**保持 6 组 by-domain + 把 `catalog-query`(所有 `search_*`)挪进 Resident**,而不是把每组再拆成 mutate/inspect。

**最新定论(施工以此为准)**:

| Lazy 组(按域;含 agent = 7 组) | 工具数(约) | 备注 |
|---|---|---|
| function | 7 | 锻造 + 试跑 + inspect 一组,不再拆 mutate/inspect |
| handler | 8 | 同上 |
| agent(新) | ~9 | 第 4 个 forge 域,与 function/handler 对称(本 doc 的新增) |
| workflow | 8 | 含错诊 / 观察工具(不拆独立 `workflow-debug` 组) |
| mcp | 6 | — |
| document | 7 | — |
| skill | 2 | — |

**Resident 侧**:7 个 `search_*`(`catalog-query`)+ 3 skill + 3 memory + meta(`activate_tools`)+ chat 基础工具。LLM 一开始就能搜任何 entity,锻造类工具按需 activate 对应 domain 组。

---

**↓ 以下为旧提案(已作废,仅留推演痕迹)**

当前 6 lazy group 总 ~4,400 tokens。Subagent 当时实测 + 提案(后被推翻):

| 当前 | 实测 tokens | (旧)提议拆分 |
|---|---|---|
| function (7) | ~950 | ~~forge-mutate (3) + forge-inspect (4)~~ |
| handler (8) | ~1,050 | ~~handler-mutate (4) + handler-inspect (4)~~ |
| workflow (8) | ~900 | ~~workflow-craft (3) + workflow-deploy (3) + workflow-debug (~7)~~ |
| mcp (6) | ~700 | ~~mcp-tools (3) + mcp-admin (3)~~ |
| document (7) | ~600 | document-tree(整体保留) |
| skill (2) | ~200 | 现状保留 |

旧提案里仍正确并已采纳的那一条:**`catalog-query`(所有 `search_*`)放 Resident**,LLM 一开始就能搜任何 entity。错诊场景因此从 22 工具收敛(activate workflow 组即可),省 token 的收益来自 search_* 前置 + by-domain 激活,而非细分。

### S2. Forge SSE 现状 + 改动

**好消息**:协议 `kind` 字段开放(实际验证只有 3 kind: function/handler/workflow)。扩 kind 集合就行,4 event 类型(started/op_applied/env_attempt/completed)不动。

**Kind 集合扩到 6**(function / handler / workflow / agent / document / skill;2026-05-31 **B5 定:6 —— 核心产品体验需求**):

| Kind | 现状 | 用意 |
|---|---|---|
| function | ✅ 已支持 | — |
| handler | ✅ 已支持 | — |
| workflow | ✅ 已支持 | — |
| **agent** | ❌ 新 | Quadrinity 一致(有版本/pending/accept,无 env,同 workflow)|
| **document** | ❌ 新 | 右栏 subpage 流式呈现文档编辑 |
| **skill** | ❌ 新 | 右栏 subpage 流式呈现 skill 锻造 |

> **B5(产品决策,2026-05-31):6 大锻造工具全进 forge SSE。** 前端有 **subpage** 形态——**左栏对话输出,右栏是当前在锻造/编辑的那个工具的页面,实时流式呈现它的变化**。function / handler / workflow / agent / document / skill 这 6 个都有各自的工具页、都要在右栏流式更新,所以**都必须在 forge 流上** —— 这是非常核心的产品体验。**forge SSE 因此是"右栏 subpage 的实时锻造/编辑流"通道,不只是版本/环境生命周期通道。**
>
> 注意一个概念区分:**forge SSE 的 6 kind(UI 流式通道)≠ "forge 实体"分类(quadrinity 4)**。document/skill 在分类上不是锻造实体(document=内容、skill=marketplace 装),但它们的编辑要在右栏流式呈现,所以也上 forge 流。两个 axis 不冲突。`env_attempt` 仍只 function/handler 有(其他无 Python venv)、version/accept 只 quadrinity 有,但 6 个都发 `forge_started`/`forge_op_applied`/`forge_completed` 驱动右栏。**mcp 不进**(marketplace 装、无锻造页)。

**6 kind 的 emit 点漏了一大堆**(都要补,右栏才有流):

| 事件 | function | handler | workflow | agent | document | skill |
|---|---|---|---|---|---|---|
| create | ✅ | ✅ | ✅ | ❌ 新 | ❌ | ❌ |
| edit | ✅ | ✅ | ✅ | ❌ 新 | ❌ | ❌ |
| accept_pending | ❌ | ❌ | ❌(都只 notifications) | ❌ 新 | n/a(无版本) | n/a |
| revert | ❌ | ❌ | ✅ | ❌ 新 | n/a | n/a |
| delete | ❌ | ❌ | ✅ | ❌ 新 | ❌ | ❌ |
| move | n/a | n/a | n/a | n/a | ❌ | n/a |
| 试跑结果 | ❌ | ❌ | ❌ | ❌ 新 | n/a | n/a |
| `ForgeOpApplied` 逐 op 进度 | 协议声明**从未 emit** | 同 | 同 | 同 | 同 | 同 |

**改动**:

1. `internal/infra/forge/protocol.go::IsValidScopeKind` 加 3 个 kind(`agent` / `document` / `skill`)→ 6 — 3 行
2. function/handler/**agent**/workflow 的 `create` / `accept_pending` / `revert` / `delete` 补 emit(各自锻造点;~10-12 行)
3. **document / skill 的 create / edit / delete(document 还有 move)补 emit forge SSE** —— 右栏 subpage 才能流式呈现它们的编辑;它们无版本/env,故只发 `forge_started`/`forge_op_applied`/`forge_completed`(无 `env_attempt`、无 accept/revert)。**document `move` 走 `operation=edit` + `ForgeOpApplied{op:"move"}`,不扩 `OperationMove`(C1)**
4. `ForgeOpApplied` 真 emit(每 op apply 时,~3-5 site)
5. 试跑结果 emit(已拍 Emit,详 决策 #4)

**env_attempt** 只 function/handler 有(其他 4 个无 Python venv)。

**协议本身不动**:仍 4 event 类型,**6 kind 共享**。

> **CANON-X1(扩的是共享 scope 枚举,不另立 forge 私有枚举)**:给 forge SSE 加 `agent` / `document` / `skill` kind(→ 6),本质是扩 **eventlog 的共享 scope-kind 闭枚举**——`forge.IsValidScopeKind` 复用 `eventlogdomain.Kind*`,两条 SSE 共用同一套 scope 词表。这是项目重构,扩这个共享枚举是接受的、**不另立 forge 私有枚举**。按 **E2**(闭枚举先改协议文档再加 code)先更新 `event-log-protocol.md` 再加 kind。澄清旧表述里"eventlog SSE 不动"——**实际共享 scope 枚举要扩(加 agent/document/skill)**。

### S3. Relations — 6 种新 kind + DB CHECK migration

**好消息**:`relations` 表**当前无 version_id 列** — 永远 prod 天然合规 ✅。

**坏消息**:加 agent 需要 6 种新 relation kind:

```
workflow_uses_agent              # workflow 节点 ref agent
agent_uses_function              # agent 工具挂载 fn_xxx
agent_uses_handler               # agent 工具挂载 hd_xxx.method
agent_uses_mcp                   # agent 工具挂载 mcp:server/tool
agent_uses_document              # agent knowledge 挂载
agent_uses_skill                 # agent skill 挂载
```

> **CANON-X5(chat 锻造/编辑用通用边,不为 agent 立专边)**:chat 老板锻造/编辑 agent **不加** `conversation_forged_agent` / `conversation_edited_agent`——冗余且反既有通用模式。用既有通用边 `conversation_forged_entity` / `conversation_edited_entity` + `to_kind=agent` 即可。真正新增的边类型 = `workflow_uses_agent` + 5 个 `agent_uses_*`(function / handler / mcp / document / skill),共 **6 种**(**无 `agent_uses_agent`** —— agent 不能调 agent,员工思维,C-3)。

**改动**(`backend/internal/domain/relation/relation.go`):

- 加 `EntityKindAgent = "agent"`(line 74+)
- 加 6 个 kind 常量(`workflow_uses_agent` + 5 `agent_uses_*`,无 `agent_uses_agent`)
- 改 `IsValidKind` switch (line 54)
- 改 `IsValidEntityKind` switch (line 81)
- 改 DB CHECK constraint 列举(line 26)
- DB migration 加 7 个 kind 到 CHECK

**新加 reader**(`app/relation/relation.go`):AgentReader 接口 + GetRelgraph 加 agent reader 调用。

**Sync hooks**:agent CRUD/Accept/Revert 调用 SyncOutgoing(6 个 `agent_uses_*` edge 由 agent.mounts 计算;`workflow_uses_agent` 由 workflow 侧拥有,`conversation_*_entity` 由 chat 侧拥有)。

**capability check 不走 relation**(走 workflow graph walk 已足够;relation 只服务 relgraph / UI)。

### S4. Catalog — 已天然 prod-only,只需要加新字段 + agent reader

**好消息**:catalog `Item` 结构很简洁(Source/ID/Name/Description/Category),无 version 字段 — "永远 prod" 天然合规 ✅。

**改动**:

1. 加 `internal/app/agent/catalog_source.go`(~50 LoC)
2. `Item` 加 `Kind` 字段(function 透出 normal/polling)
3. `Item` 加 `Active` 字段(workflow 透出 active 状态;mechanical 渲染加 `[INACTIVE]` 前缀)
4. `runner.go::categoryLabels` 加 `"agent": "..."` 行
5. main.go `catalog.RegisterSource(agentService.AsCatalogSource())`

**token cost**:agent 10-20 个 + function kind 字段 + workflow active 标 ≈ 增加 650-2190 tokens 进 chat 老板的 system prompt。**100+ entity 时考虑 pagination**,目前不急。

> **CANON-X8(catalog 缓存,低优)**:catalog 菜单每轮注入 chat system prompt,加 agent 后变大;且 **no-pin(永远 prod)** 要求菜单反映当前 active 状态,这段是动态的、**不好 prompt-cache**。低优做法:实体多时按相关性裁剪/分页,尽量**分离静态段与动态(active 标)段**保 cache。目前不急。

**开放问题**:agent 是否进 catalog?Subagent 提了"agent 是 system-level orchestrator 可不进 catalog 省 token"。**我倾向进**(因为 agent 是可被引用的 callable,跟 function/handler 同 lift)— **待用户拍**。

### S5. 跨域涟漪 — 7 个 domain 受影响

| Domain | 改动 | 备注 |
|---|---|---|
| **memory** | 走独立 system prompt 装配链 | agent 不接 memory(产品决策,详上方) |
| **skill** | 加 `AgentID` 到 ExecutionLog + create/edit/delete emit forge SSE | skill.Agent 字段已有 ✅;**进 forge SSE**(右栏 subpage 流式呈现 skill 锻造) |
| **document** | create/edit/delete/move 走 relation + emit forge SSE | **进 forge SSE**(右栏 subpage 流式呈现文档编辑);delete 受 PurgeEntity 自动清 |
| **mcp** | 无 schema 改 | 走 relation;uninstall 时 audit 是否还有 agent mount |
| **model** | 0 改 | `ScenarioAgent` 已就绪(line 42-44) ✅ |
| **workflow node** | 0 改 | `NodeTypeAgent` 已声明(line 58)+ `IsCapabilityNode` 包括 ✅ |
| **idgen** | 加 `ag_/agv_/agx_` | §S15 注释更新 |
| **conv** | 加 `EntityKindAgent` 到 conv 受 relation 影响 | 用于 :iterate 跟踪 |
| **sandbox** | 0 改 | agent 不在 sandbox 跑;agent 工具挂载的 function/handler 走现有 sandbox ✅ |

**好新闻**:model + workflow node domain 已经预备好了 agent — 不用大改。

### S6. HTTP API — 22 新端点 + 1 改造

**Agent domain 13 端点**:CRUD 6 + version 3 + pending action 2 + run 1 + iterate 1。文件:`backend/internal/transport/httpapi/handlers/agent.go` ~400 lines,mirror `function.go`。

**Workflow lifecycle**:
- 新 `POST /workflows/{id}:activate` / `:deactivate`
- 改造 `POST /workflows/{id}:trigger`,body 加 `triggerNodeId` **必填**(breaking)

**FlowRun**:
- 新 `GET /flowruns/{id}/trace`
- 新 `POST /flowruns/{id}:cancel`
- 已有 `GET /flowruns/{id}/nodes` ✅

**失败步诊断 / events 端点**(durable 模型,**无独立死信 store**):
- `GET /flowruns/{id}/failures` —— 列该 flowrun 事件日志里 retry 用尽的失败 activity(`node_id` / `iteration_key` / `error` / `attempts`)
- `POST /flowruns/{id}/nodes/{nodeId}:replay` —— 从事件日志确定性重放、停在该失败 activity 重跑
- `GET /events?type=...&workflowId=...&since=...`(或扩 `/eventlog`)

> durable 模型下**没有死信表 / store / `:clear`**:失败步 = 那个 activity 没记账成功,失败信息作为事件留在同一本 `flowrun_events` journal。签名按 **`flowrunId` + `nodeId`**(不是 `messageId`)。对齐 [`07-error-handling.md`](./07-error-handling.md) §"失败步"(`GET /flowruns/{id}/failures`)+ [`10`](./10-ai-tool-inventory.md) 的 `list_failed_steps` / `replay_flowrun`。

**testend 受影响**:`/workflows/{id}:trigger` body 加 triggerNodeId 是 breaking。testend 调用全要 patch。详 [`testend/CLAUDE.md`](../../../testend/CLAUDE.md)。

### S7. 测试基建 — 4 新 pipeline + 9 新 errcode + 6 新 seam

| 类型 | 新增 | 文件 |
|---|---|---|
| Pipeline test | 4 文件 ~850-1100 LoC | `api/agent/` + `api/workflow_lifecycle/` + `cross/flowrun_observe_*` + `cross/diagnosis_*` |
| Errcode sentinel | 8 个 | `AGENT_NOT_FOUND` / `AGENT_VERSION_NOT_FOUND` / `AGENT_NAME_DUPLICATE` / `CAPABILITY_CHECK_FAILED` / `TRIGGER_EXHAUSTED` / `FLOWRUN_NODE_NOT_FAILED`(replay 目标不是失败步,取代旧 `DEAD_LETTER_*`)/ `FLOWRUN_NOT_CANCELLABLE` / `INVALID_TRIGGER_NODE` |
| SSE truth | 7 个新 notif type + **3 个新 forge kind(`agent` / `document` / `skill` → 6)** | sse_truth.go forge kind 加 `agent` / `document` / `skill`(6 大锻造工具全进 forge SSE,驱动右栏 subpage 流式;mcp 不进)+ notif `workflow_activated/deactivated` / `trigger_exhausted` / `handler_crash` / `step_failed`(取代旧 `dead_letter_created`)/ `flowrun_node_status_changed` |
| Cross seam | 6 个新 | `workflow:activate_register_listener` / `:deactivate_destroy_listener` / `:trigger_sync_acceptance` / `agent:skill_mount` / `:document_mount` / `scheduler:durable_replay_driven` |

> seam `scheduler:durable_replay_driven`(原 `scheduler:message_queue_driven`)= 验证执行引擎按 **durable execution** 跑:节点作为 activity 记账进事件日志、崩溃后确定性重放命中已记账步骤不重跑、停在第一个未记账步骤续跑。对齐 [`00-overview.md`](./00-overview.md) 的"崩溃重放"段。

`make matrix` 加 1 新 agent section + workflow section 加 2 行 + flowrun section 加 1 行。

### S8. Frontend FSD — 1 新 entity + 5 新 feature + ~1660 LoC

| 类型 | 新增 / 改动 |
|---|---|
| **entities/agent/**(新) | ~300 LoC(types/api/ui card) |
| **entities/workflow/**(改) | +40 LoC,加 activate/deactivate hooks + triggerNodeId param |
| **entities/function/**(改) | +20 LoC,types 加 `kind: 'normal' \| 'polling'`,filter param |
| **entities/flowrun/**(改) | +60 LoC,加 trace / nodes / cancel hooks |
| **features/workflow-deploy/**(新) | ~120 LoC(activate/deactivate 按钮 + 状态 badge) |
| **features/workflow-trigger/**(新) | ~200 LoC(trigger node picker + payload form) |
| **features/flowrun-debug/**(新) | ~300 LoC(trace viewer + 死信 inbox + replay) |
| **features/agent-ui/**(新) | ~250 LoC(agent node config UI + case CEL + approval markdown) |
| **features/workflow-edit/**(改) | +180 LoC(palette 14→5,新节点 config UI) |
| **widgets/canvas-runtime/**(新) | ~140 LoC(画布滴答 overlay) |
| **shared/**(改) | +80 LoC(queryKeys 6 新 + errorMap 5 新 + SSE dispatcher) |
| **i18n** | ~45 新 key |
| **总计** | **~1660 LoC** |

---

## doc 11 需要 patch 的点

| doc 11 段 | 现状 | 改 |
|---|---|---|
| Lazy 划分(C1)| 提议 7 组(workflow 膨胀到 22) | **domain-6**(按 forge 实体分 6 组,不细分 mutate/inspect)+ `catalog-query`(7 个 `search_*`)入 Resident(详 S1;Round-3 实测推翻了"细分到 11"的旧结论) |
| Forge SSE(G1) | 只说 "加 agent kind" | kind 扩到 **6(加 agent / document / skill)** —— 右栏 subpage 要对 6 大锻造工具流式呈现;各 kind emit 点补漏 + ForgeOpApplied 真 emit + 试跑结果 emit |
| 错诊工具放哪 | 待用户拍 | **已答**:并入 Lazy `workflow` 组(domain-6,不单拆 `workflow-debug`) |
| Relations 改造 | **doc 11 完全没提** | 新加段落:6 种 kind(`workflow_uses_agent` + 5 `agent_uses_*`,无 `agent_uses_agent`;chat 锻造/编辑走通用边 `conversation_*_entity` + `to_kind=agent`,不立专边)+ DB migration + AgentReader |
| Catalog 改造 | doc 11 只提 source 加 reader | 补 `Kind` / `Active` 字段加进 Item + token cost 估算 |
| Memory 给 agent | **doc 11 完全没提** | 新加段:agent 不接 memory(产品决策),dispatch 走独立 system prompt 装配链 |
| 执行引擎(H 段)| doc 11 旧稿仍写 `driveLoop → message-queue-driven` + 5 节点 actor + `infra/messagequeue/` | 对齐 **durable execution**:执行器照图走 + 事件日志 + 确定性重放(详 [`00-overview.md`](./00-overview.md));`infra/messagequeue/` 这类全删,换 `flowrun_events` journal |
| categoryLabels | doc 11 提了 | ✅ 跟 S4 一致 |
| HTTP API delta | 散落各处 | 集中到一节 — 22 新 + 1 改 |
| FSD delta | 笼统说"改 workflow-edit" | 1660 LoC 拆细 |

我会回头改 doc 11 这些点(单独 commit 标 `[doc-fix]`)。

---

## 综合改造规模(修订版)

> 执行引擎相关项已从旧的 message-queue 重构换成 **durable execution** 引擎。规模**比旧估更小**:durable 模型删掉了消息 version 配对 / 前沿(frontier)计算 / 空票(void token)/ consume-emit-processed 消息状态机 / 原子认领等一大堆机制,执行器只需"照图走 + 写事件日志 + 重放跳过已记账"。

| 块 | doc 11 估时 | 修订 | 修订理由 |
|---|---|---|---|
| 1. DB schema(含 relations migration) | 1.5 天 | **2 天** | 加 relation CHECK migration + agent table 新建;flowrun 持久化塌缩成 `flowruns` + `flowrun_events`(journal)+ `approvals` 三表(比旧 messages+node_state 更少) |
| 2. Agent domain + 工具(domain-6 内 agent 组) | 2 天 | 2 天 | 不变 |
| 3. 事件日志(journal)+ 重放底座 | 1.5 天 | **1 天** | 替代旧"Message queue infra ~300 行";append-only 事件日志 + 重放跳过逻辑比消息队列简单(无版本/前沿/空票/原子认领) |
| 4. 节点执行引擎:执行器照图走 + activity 记账 + 确定性重放 | 3-4 天 | **4-6 天(B2 重估)** | 替代旧"driveLoop → message-queue + 5 节点 actor"。**拆 4 子块分别验收:① journal writer(per-flowrun 串行写 + UNIQUE,见 00 写入契约)② replay interpreter ③ control-flow reducer(fork-join / case 回边 / iteration_key)④ projection + trace API**。"确定性重放正确性"是承重不变量,按上限估 |
| 5. Lifecycle(activate/deactivate/trigger)| 2 天 | 2 天 | 不变 |
| 6. Polling 系统 + capability check | 1.5 天 | 1.5 天 | 不变 |
| 7. 教学 prompt + catalog + toolset + SSE 补 emit | 2 天 | **3 天** | + forge emit 补全(~10 处) + agent system prompt 独立链路 + domain-6 lazy 重组(search_* 入 Resident) |
| 8. Frontend(平行块 4 后)| 2-3 天 | **5-6 天** | doc 11 低估;~1660 LoC + 滴答 widget 复杂 |
| **N1. outputSchema 运行时强制**(provider-native + app 层 repair / validate / retry)| **漏列** | **2-3 天** | 跨 provider wire format + JSON-repair + schema validate + next_step retry + 错误 envelope + 测试矩阵;实测 ~12% 硬残留 —— **独立 LLM 子系统,别塞进 agent domain / 引擎顺手做**(B1)|

**总(后端 + 前端) ~22.5-26.5 天**(含 N1 单列 + 块 4 按上限重估;doc 11 原估 13-14 天纯写。这是按交叉审 A 组"按底线估非上限"修订后的诚实区间,**非承诺**)。

> **实现量重估警示(交叉审 A1/A3/A4/A6 —— 上表偏乐观,几块是从零造的承重子系统,按底线估非上限)**:
> - **A1 · outputSchema 运行时强制(N1)** —— **已单列(见上表 N1 行)**;provider-native(可选)+ app 层 JSON-repair / validate / next_step-retry 是独立子系统(实测 ~12% 硬残留),别塞进 agent domain。
> - **A3 · #3+#4 的"确定性重放正确性"是研究级不变量**(重放粒度 / 幂等 / callable 版本漂移 / CEL 确定性),不是写完 driveLoop 就完。**块 4 已按 B2 拆 4 子块验收 + 按上限重估(见上表)。**
> - **A4 · #6 的 capability-check 查深**(kind / method / 必填参数 + active-version 反向重查)**本质是个静态分析器**,不是布尔存在性查。
> - **A6 · durable 触发收件箱 / 派发器 / drain(Theme 3)** 是新表 + boot 重注册 + ack loop 的从零工程,别低估。
>
> 现实更接近"手搓 **mini-Temporal** + **schema-validate-retry 层** + **capability 静态分析器**"。**进实现前把这几块各排成独立工程块重估,别拿上表当承诺。**(纯排期诚实,不影响设计正确性。)

---

## 已拍决策(2026-05-29;2026-05-31 更新 #1)

| # | 决策点 | 结论 |
|---|---|---|
| 1 | Lazy 分组 | **按域分组**(不细分 *-edit / *-use;研究验证为 6 组,**加 agent 后 7 组**:function / handler / agent / workflow / mcp / document / skill);7 个 `search_*`(`catalog-query`)+ 3 skill + 3 memory + meta + chat 基础 入 **Resident**。〔**2026-05-31 更正**:原拍"11 组"被 Round-3 LLM 实测推翻——真变量是 search_* 位置,domain-6 激活对组率 62% > 11-edituse 46%。详 [`14-llm-validation-research-record.md`](./14-llm-validation-research-record.md) §3.6〕 |
| 2 | Agent 进 catalog | **进**(callable 同 lift,与 function/handler 一致) |
| 3 | Memory 给 agent | **不接**(产品决策,员工思维)。实现走独立 system prompt 装配链 |
| 4 | 试跑结果 emit forge SSE | **Emit**(支撑未来"试跑结果时间线"UI) |
| 5 | `ForgeOpApplied` 现在补 emit | **现在补**(协议已声明,~5 行,UI 渐进反馈直接受益) |
| 6 | Agent 带 `:triage` | **带**(对齐 flowrun,反正没坏处) |

剩下的小决策(各种 default 值 / 字段命名)我自己拍,不打扰你。
