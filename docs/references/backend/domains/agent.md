---
id: DOC-129
type: reference
status: active
owner: @weilin
created: 2026-06-01
reviewed: 2026-06-08
review-due: 2026-09-01
audience: [human, ai]
---
# Agent — 配置好的 LLM Worker（Quadrinity 第四元）

> **核心地位**：Agent 是 Forgify「四项全能」(Quadrinity) 的第四元——**配置好的 LLM worker**。它**不写一行代码**，靠**按引用挂载**（一个 system prompt、0-1 个 skill 名、若干文档作知识、`fn_`/`hd_`/`mcp` 工具白名单、声明的 `inputs`/`outputs` 字段、一个可选 model 覆盖）定义能力，以 **ReAct loop** 运行。与对话里临时生成的 subagent 不同，本域的 Agent 是**持久化、版本化、可重用**的专业 worker：可独立 `:invoke` 试跑，也可作为 Workflow 的 agent 节点被引用。

---

## 1. 版本模型：线性历史 + 自由指针（无 accept）

与 function / handler 完全一致——版本号、版本内容、active 指针三者正交：

| 概念 | 语义 | 谁能动 |
|---|---|---|
| **版本号 `version`** | 写入顺序（单调计数器，只增不改） | 写新版本时 = `max+1` |
| **版本内容** | 不可变快照（append-only，无 `updated_at`） | 永不修改既有版本 |
| **active 指针 `active_version_id`** | 「现在用哪个」 | edit 前移 / revert 自由移动 |

- **create** = 写 v1，立即生效。
- **edit** = **全量替换** Config → 写 `v(max+1)` → 指针前移。立即生效、无断点（edit 是替换、非合并）。
- **revert(N)** = **只挪指针**到 vN，不产生版本、不删「更新的」版本。active 号可能小于某些历史号（前端诚实显示「当前 vN，之后还有 …」）。
- 历史保留供 revert / 审计；超 `AcceptedVersionCap=50` 裁最老的——**但绝不裁 active 版本**（revert 后它可能很老）。

`ActiveVersion` 是 `Agent` 上的 **computed 字段**（非列），由 `Service.Get` 附上，使读者一趟拿到当前配置。

---

## 2. 物理模型

### 2.1 `agents`（`ag_`，软删）
`id` · `workspace_id`(orm 自动隔离) · `name`(workspace 内 partial-UNIQUE，软删后释放) · `description` · `tags`(json) · **`active_version_id`**(指针) · 时间戳 · `deleted_at`。

### 2.2 `agent_versions`（`agv_`，append-only + cap 裁剪，**不可变、无 `updated_at`**）
`id` · `workspace_id` · `agent_id` · **`version`**(单调号，**无 status**) · `prompt`(system prompt) · `skill`(单个 skill 名) · `knowledge`(json，文档 ID 列表) · `tools`(json，`ToolRef{ref,name}` 列表) · **`inputs`**(json，`[]schema.Field`，`TEXT NOT NULL DEFAULT '[]'`) · **`outputs`**(json，`[]schema.Field`，同上) · `model_override`(json，`ModelRef` nullable) · `change_reason` · `forged_in_conversation_id`(relation 边用) · `created_at`。`UNIQUE(agent_id, version)`。

> **I/O 统一**：`inputs`/`outputs` 均为共享 `[]schema.Field`（`internal/pkg/schema`），取代旧 `output_schema` 列与三态 `OutputSchema`/`OutputSchemaKind`(free_text/enum/json_schema) 类型——后者连同 enum 硬约束 + coercion 一并删除。

### 2.3 `agent_executions`（`agx_`，append-only log，**无软删/无硬删** D1）
`id` · `workspace_id` · `agent_id` · `version_id` · `model_id`(实际 resolve 出的) · `status`(ok/failed/cancelled/timeout，CHECK) · **`triggered_by`**(chat/workflow/manual，CHECK) · `input`(json) · `output`(json) · `error_message` · `elapsed_ms` · `started_at` · `ended_at` · `conversation_id` · `message_id` · `tool_call_id` · `flowrun_id` · `flowrun_node_id` · `created_at`。

**`triggered_by` = 执行体**（「谁在跑」，非「请求怎么来的」）：`chat`(对话里 LLM 调 invoke_agent) / `workflow`(工作流 agent 节点) / `manual`(REST `:invoke` 手动跑)。**无 `agent` 触发**——员工不调员工（agent tools 禁 `ag_` ref）。

---

## 3. 挂载架构（Mounts）

Agent 不编写逻辑，而是「挂载」其它领域的实体来定义能力，全部存在 active 版本上：

- **Prompt**：system prompt（worker 身份）。`buildSystemPrompt` 拼 agent 身份 + worker 纪律（「只用给你的工具」）+ outputs 指令。
- **Skill**：0-1 个 skill 名（预激活）。
- **Knowledge**：文档 ID 列表。invoke 时经 `KnowledgeProvider.BuildKnowledgePrefix` 渲染成 prompt 前缀，拼到 user message 头部。
- **Tools**：显式授权的 callable 白名单（`fn_` / `hd_…method` / `mcp:server/tool`）。**禁 `ag_` ref**（员工不调员工，`ValidateTools` 拦 `ag_` 与空 ref）。invoke 时 `filterToolsByWhitelist` 把全局工具池按白名单 `Name()` 过滤；空白名单 = 纯 prompt worker。
- **Inputs / Outputs**：均 `[]schema.Field`（`{name,type,description}`）。`inputs` 声明 workflow 节点喂给本 agent 的字段。`outputs` 非空时，`outputsInstruction` 把这些字段渲成 system prompt 硬约束——指示 LLM 最终答案是**仅含这些字段的单个 JSON object**（output only the JSON）；空 outputs = **自由文本**作答。取代旧三态 `free_text`/`enum`/`json_schema`——enum 硬约束 + `coerceEnumOutput` 规整已删；下游 workflow 路由现由 `control` 实体读 agent 输出字段决定。
- **Model Override**：`*ModelRef`（apiKeyId + modelId + options）。nil = 默认 `agent` scenario 模型。缺 apiKeyId/modelId 在 create/edit 即被 `ErrInvalidModelOverride` 拦下。execution 记录实际 resolve 出的 `model_id`。

---

## 4. 执行（Invoke）：唯一入口 + 端口注入（DIP）

`InvokeAgent` 是**唯一执行方法**——invoke_agent 工具 / HTTP `:invoke` / workflow agent 节点都经它，每次跑完写一条 `agent_executions`（对标 function 的 `RunFunction`）。

agent 自己**不拥有** LLM / 工具池 / 知识渲染，三个外部依赖经 **`InvokeDeps` 端口注入**（构造后 `SetInvokeDeps`，避 init 环；M7 装配注入真实、测试注入 fake）：

| 端口 | 职责 | 实现（boot 时） |
|---|---|---|
| `LLMResolver` | `(nil = 默认 agent 场景) model 覆盖 → 可运行 LLMBundle`（client + 预填 Request）| model-picker + apikey + llm-factory |
| `Tools func() []Tool` | 返回**全局工具池**；invoke 按 agent 白名单过滤 | 全局 toolset |
| `KnowledgeProvider` | 文档 ID → prompt 前缀字符串 | document 渲染器 |

`InvokeAgent` 取 active（或指定）版本 → 渲染知识前缀 + 拼 input → 过滤工具白名单 → resolve LLM bundle → 跑 `app/loop.Run`（ReAct loop，默认 10 turns）→ 经 detached ctx（保留 workspace，使被取消的运行仍落库）写一行 `Execution` 审计。

**SSE 白捡**：loop 的 emitter 把 block 推到 ctx 携带的 stream scope——在 chat 里调用即 messages 流，渲染成**嵌套 subagent 子树**（E3）。**agent 零 stream 代码**。

**Workflow 子步重放（ADR-010）**：agent 作为 workflow 节点时，一个回合可能含 N 次工具调用；崩溃重启不应重消耗已完成的子步。`InvokeInput.ReplaySteps`（已完成步前置）+ `Recorder`（记新步到绝对回合下标）透传，重放快进到最后一个未完成子步。standalone chat/manual invoke 时这些字段全空。

---

## 5. 锻造（Forge）：全量 Config 替换

create/edit 携带完整 `Config`（`prompt` / `skill` / `knowledge` / `tools` / `inputs` / `outputs` / `modelOverride` / `changeReason`）——edit 是**全量替换**、非增量 ops（agent 配置小、整体替换最清爽，区别于 function/handler 的 ops 草稿）。落版本前 `ValidateTools`（禁 `ag_` + 非空 ref）+ `validateModelOverride`（override 设了则 apiKeyId/modelId 都必填）+ `schema.ValidateFields`（inputs/outputs 字段名唯一 + 类型合法）。

---

## 6. LLM 工具（9，懒加载）

`search_agent`（子串找）· `get_agent`（含 active 版配置）· `create_agent`（立即生效、非 pending）· `edit_agent`（全量替换写 max+1）· `revert_agent`（按号移指针）· `delete_agent`（软删）· **`invoke_agent`**（真跑 ReAct loop，落一条 `agent_executions`）· `search_agent_executions`（分页 + ok/failed 汇总）· `get_agent_execution`（单条详情）。

全 S18 五方法接口、danger 由 LLM 逐次自报；进 `Toolset.Lazy`，经 `search_tools` 浮现。**无 accept 工具**（create/edit 立即生效，无 pending/accept，同 function/handler）。

---

## 7. HTTP 端点

`POST /agents`（扁平创建）· `GET /agents`（分页）· `GET|PATCH|DELETE /agents/{id}`（PATCH=UpdateMeta，改 name/description/tags 不升版本）· `POST /agents/{id}:edit|:invoke|:revert` · `GET /agents/{id}/versions` · `GET /agents/{id}/versions/{version}`（整数号或 version id）· `GET /agents/{id}/executions`（分页 + filter）· `GET /agent-executions/{execId}`。

> **删**：`/{id}/pending`、`pending:accept`、`pending:reject`（无 accept 状态机）。`:iterate`(AI 编辑→conversationId) 随 askai 波次 6。

---

## 8. 跨域集成

- **relation**：`Service` 实现 `SetRelationSyncer`。Create/Edit/Revert（active 版本变 → 挂载可能变）重算**出向 equip 边**——active 版本挂载的 ref 各推一条 `KindEquip`，`OtherKind` 区分目标：`fn_` → Function、`hd_…method` → Handler（剥 `.method`）、`mcp:server/tool` → MCP（剥 `/tool`）、每个 knowledge 文档 → Document、`skill` → Skill。**5 出边、无 agent→agent**（员工不挂员工）。另按 `ForgedInConversationID` 写**入向**对话边（`KindCreate` v1 / `KindEdit` v>1，分 kind-scope 故共存）。Delete 级联 `PurgeEntity`。
- **catalog**：`AsCatalogSource` 把 agent 库暴露给能力 catalog（名 + 描述）。agent **不是容器实体**——挂载工具是内部白名单、非可调子单元，故**不报 Members**（区别于 mcp/handler）。
- **workflow**：`agent` 节点经 `InvokeAgent` 执行（`triggeredBy=workflow` + `flowrunId/flowrunNodeId`），落 `agent_executions`；ADR-010 子步重放经 `ReplaySteps`+`Recorder`。
- **document / skill / function / handler / mcp**：被 active 版本按引用挂载（弱引用，relation 出边记录）。
- **notification**：`agent.created/edited/reverted/meta_updated/deleted` 经 `Emitter`。

---

## 9. 错误字典

| Sentinel | Wire Code | HTTP |
|---|---|---|
| `ErrNotFound` | `AGENT_NOT_FOUND` | 404 |
| `ErrNameConflict` | `AGENT_NAME_CONFLICT` | 409 |
| `ErrVersionNotFound` | `AGENT_VERSION_NOT_FOUND` | 404 |
| `ErrNoActiveVersion` | `AGENT_NO_ACTIVE_VERSION` | 422 |
| `ErrToolsAgentRef` | `AGENT_TOOLS_AGENT_REF` | 422 |
| `ErrToolRefBlank` | `AGENT_TOOL_REF_BLANK` | 422 |
| `ErrInvalidModelOverride` | `AGENT_INVALID_MODEL_OVERRIDE` | 422 |
| `ErrExecutionNotFound` | `AGENT_EXECUTION_NOT_FOUND` | 404 |

> 工具失败软返 tool-result 串（不冒泡 HTTP）；上表是 HTTP 端点冒泡的 domain 错误。
