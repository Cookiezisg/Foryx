# 能力披露层重构 — Capability Disclosure Redesign

> 状态:设计稿(brainstorming 产物),待评审 → writing-plans → 实现
> 日期:2026-05-25
> 主题:根治"每轮请求 ~28k input token"的工具/能力上下文膨胀;把 tool 与 catalog 统一为单一「能力披露层」。
> 范围:**仅 token 治理**。workflow 执行引擎(`app/scheduler`,~2587 行)**已交付**(Phase 4,2026-05-13);本设计只在 tools 层补一个 `trigger_workflow` 薄工具(包 `scheduler.StartRun`),见 §10。

---

## 0. 问题(实测根因)

一条 "Hello" 实测 **28,044** input token。逐层归因:

| 组成 | ~token | 占比 |
|---|---|---|
| 用户消息 + system prompt 静态段(identity/how_to_work/tools/environment) | ~430 | 1.5% |
| catalog / memory(新号为空) | ~0 | 0% |
| **64 个工具(`tools` 参数)** | **~28,200** | **98%** |
| └─ `injectStandardFields` 重复注入(destructive+execution_group ×64) | ~14,900 | 53% |
| └─ Parameters JSON schema | ~8,000 | 28% |
| └─ Description(其中 4 个 forge 巨头 create_workflow/create_handler/edit_workflow/edit_handler 占 ~2,400) | ~5,300 | 19% |

四个结构性问题:

1. **64 个工具全量静态下发**:无论用户说什么,每轮都把 64 把工具的完整 schema 发给 LLM。
2. **`injectStandardFields` 重复放大**:`destructive`(~280 字)+ `execution_group`(~520 字)的说明对 64 把工具逐一复制,~13k token 是纯重复。
3. **catalog 无上限**:用户资产(function/handler/...)全量列入 system prompt,随资产线性增长 —— 深度用户会形成"第二个、且无封顶的 28k"。
4. **无 prompt caching**:以上每轮按全价计费,无法摊薄。

业界参照:Berkeley function-calling 榜显示工具 4→51 个时模型选对工具的准确率 43%→2%;Anthropic 内测 58 工具 ≈ 55k token。即「工具多」不仅费钱,还降准确率。

---

## 1. 目标 / 非目标

**目标**
- 常驻输入 28k → **~4k 且封顶**(不随用户资产增长)。
- 保留"让模型快速感知有哪些能力"(少一层 search 调度)。
- 把 tool 与 catalog 统一为单一心智模型。
- 启用 prompt caching,常驻前缀可整段缓存。
- toB 可规模化的单位经济。

**非目标**
- 不采用 Anthropic "Code Execution with MCP" 范式(64 把里 57 把是原生 tool,非 MCP;为原生工具上代码执行属过度工程)。
- 不做向后兼容、不分阶段:**完整重写**到目标态。
- 不碰 workflow 执行引擎(`app/scheduler` 已交付,见 §10);仅补 `trigger_workflow` 薄工具入口。

---

## 2. 核心概念:动作 vs 对象(两层正交)

Forgify 的"能力"是两类本质不同的东西,分两层披露:

| | 动作(Action) | 对象(Object) |
|---|---|---|
| 是什么 | agent 能执行的操作 | 可被操作的实体 |
| 例子 | `run_function` / `call_handler` / `create_handler` / `Read` / `Bash` | 用户的 function / handler / workflow / skill / 外部 MCP 工具 |
| 协议位置 | LLM `tools` 参数(function-calling) | system prompt 文本 |
| 治理机制 | **常驻 + 按需 `activate_tools`**(§4.3) | **统一能力菜单**(§4.2) |

关键认知:`function` 不是工具,是 `run_function` 的参数;`handler` 不是工具,是 `call_handler` 的参数。"我有哪些 function"属对象层(catalog),不该塞进 tool schema。

---

## 3. 架构总览

改后,每次请求发给 LLM 的内容 = **system prompt 文本** + **tools 参数**。三个机制:

- **M1 · catalog 统一能力菜单**(对象层,§4.2):平铺报菜名,小库全报 / 大库退计数。
- **M2 · tools 常驻 + 按需**(动作层,§4.3):~16 把高频常驻,长尾按 `activate_tools(category)` 激活。
- **M3 · 横切**:`injectStandardFields` 去重(§4.4)+ prompt caching(§4.5)。

skill / mcp 本来就是按需的(`activate_skill` / `call_mcp_tool` 代理 + `search_mcp_tools`),机制不动,仅纳入 M1 统一菜单一起报。

---

## 4. 详细设计

### 4.1 system prompt 完整结构

按 cache-friendly 顺序(静态前 / 动态后)装配,沿用现有 `SystemPromptSections` 命名段机制:

```text
<section name="identity">               身份一句话  ~25t  [静态·可缓存]
<section name="how_to_work">            操作原则 7 条  ~180t  [静态·可缓存]
<section name="tools">                  工具模型 + 三个标准字段讲一次  ~140t  [静态·可缓存]
<section name="capabilities">           统一能力菜单(对象层)  ~200t–1k  [半动态]
<section name="memory">                 长期记忆  0–?  [动态]
<section name="documents">              @-mention 文档注入  0–?  [动态]
<section name="user_system_prompt">     对话级(可选)  [动态]
<section name="environment">            date + 回复语言  ~20t  [动态]
```

`identity` / `how_to_work` / `tools` 三个静态段排在 `capabilities` 之前,与常驻 tool 定义共同构成稳定可缓存前缀(§4.5)。

> **段内容已于 2026-05-27 重写** —— 见 [`chat-prompt-redesign.md`](chat-prompt-redesign.md) §1(高密度操作性英文 prompt 全文)。下方原中文示例仅存历史参考,**段名/内容以新文档为准**。

**完整示例(历史参考;现已被 chat-prompt-redesign.md 取代)**:

```text
<section name="base">
你是 Forgify，帮用户构建工具、自动化工作流、处理数据的 AI 助手。
</section>

<section name="tool_conventions">
每次工具调用都接受这三个标准字段（不在各工具 schema 里重复）：
- summary（必填）：一句话说明你在做什么、为什么。
- destructive（可选）：若本次调用可能不可逆（删除、强推、写外部状态）设 true，用户会看到警告。
- execution_group（可选，整数）：同组并行，不同组按升序串行。只给彼此无依赖、无共享状态的调用编同组；拿不准就省略。
</section>

<section name="capabilities">
## 工具组
你手边已备常用工具（在本次请求的 tools 里）。需要某类操作时，用 activate_tools(类别) 取对应组：
- function — 造/改/删/查 函数（7 把）
- handler  — 造/改/删/查 处理器（8 把）
- workflow — 造/改/删/触发 工作流（8 把）
- mcp      — 接入/调用外部 MCP（6 把）
- document — 文档增删改查（7 把）
- skill    — 技能执行记录（2 把）
优先用手边的；只在确实需要某类操作时才 activate。

## 你的能力（用 [ ] 内的工具调用；完整详情用 get_*）
Functions [run_function]：
  · normalize_address — 清洗标准化地址
  · fx_rate — 取实时汇率
  …（共 30 个，更多用 search_function）
Handlers [call_handler]：
  · order_webhook — 接收校验订单
  …（共 12 个，用 search_handler）
Workflows [trigger_workflow]：nightly_report — 夜间报表 …（共 5 个）
Skills [activate_skill]：pdf_extract — 抽取 PDF 文本 …（共 8 个，search_skills）
MCP tools [call_mcp_tool]：github.create_issue …（来自 3 个 server，search_mcp_tools）
</section>

<section name="multi_agent_forging">
遇到涉及 3+ 个独立可锻造模块的请求，考虑并行派 general-purpose 子代理（execution_group=1），各锻造一个 function/handler 并自测；工作流由你自己组装。简单编辑自己做 —— 子代理 token 成本翻 N 倍。
</section>

<section name="memory">
（新用户为空）
</section>

<section name="locale_hint">
除非用户用其他语言，否则请用简体中文回复。
</section>
```

### 4.2 catalog → 统一能力菜单(M1)

**职责变更**:catalog 从"全量列 function/handler 描述"升级为"统一报所有可调用实体的菜名"。

**报菜名格式**(每行:名字 + 一句话 + 用哪个工具调):

```text
## 你的能力(用 [ ] 内的工具调用;完整详情 get_*)
Functions [run_function]:
  · normalize_address — 清洗并标准化地址
  · fx_rate — 取实时汇率
  …(共 30 个,更多用 search_function)
Handlers [call_handler]:
  · order_webhook — 接收并校验 Shopify 订单
  …(共 12 个,search_handler)
Workflows [trigger_workflow]: nightly_report — 夜间报表 …(5)
Skills [activate_skill]: pdf_extract — 抽取 PDF 文本 …(8,search_skills)
MCP tools [call_mcp_tool]: github.create_issue …(来自 3 个 server,search_mcp_tools)
```

**描述长度治理**(替代退化机制 —— 不搞"大库退计数"的分级):
- **源头**:`create_*` / `edit_*` 工具的 `description` 参数说明改为"一句话、十来字概括",不再放任 AI 写长篇(现状:创建时对 description 长度零引导)。
- **兜底**:catalog 渲染时 `truncate(desc, ~48 字符)`,防历史/超长数据击穿。
- 每行 = 名字 + 十来字,资产再多平铺全报也可控(100 实体 ≈ +1k token);`catalog.Generator` 缝本次不启用。

**接口扩展**:`catalogdomain.CatalogSource` 增 `InvokeTool() string`(返回该类实体的调用工具名,如 `"run_function"`),供 assemble 渲染 `[ ]` 标注。

**补齐 source(顺带修 §4.6 bug)**:
- 新增 `internal/app/workflow/catalog_source.go`,实现 `AsCatalogSource()`,`InvokeTool()="trigger_workflow"`。
- `main.go` 注册 workflow source + 补注册已存在的 document source。

### 4.3 tools:常驻 + 按需 activate(M2)

**常驻集(~28 把,始终在 `tools` 参数中)** —— 高频通用 + 各域的检索/执行入口:

| 类 | 工具 |
|---|---|
| 检索(发现已建实体) | `search_function` `search_handler` `search_workflow` `search_skills` `search_mcp_tools` |
| 执行现成实体 | `run_function` `call_handler` |
| 文件 | `Read` `Write` `Edit` `Grep` `Glob` |
| shell | `Bash` `BashOutput` `KillShell` |
| web | `WebSearch` `WebFetch` |
| 交互 | `AskUserQuestion` |
| 任务 | `TodoCreate` `TodoUpdate` `TodoList` `TodoGet` |
| 记忆 | `read_memory` `write_memory` `forget_memory` |
| 技能 / 子代理 | `activate_skill` `Subagent` |
| 元 | `activate_tools` |

**长尾分组(按 `activate_tools(category)` 激活)**:

| category | 工具 |
|---|---|
| `function` | `create_function` `edit_function` `delete_function` `revert_function` `get_function` `get_function_execution` `search_function_executions` |
| `handler` | `create_handler` `edit_handler` `delete_handler` `revert_handler` `get_handler` `update_handler_config` `get_handler_call` `search_handler_calls` |
| `workflow` | `create_workflow` `edit_workflow` `delete_workflow` `revert_workflow` `get_workflow` `get_workflow_execution` `search_workflow_executions` `trigger_workflow`(见 §10) |
| `mcp` | `call_mcp_tool` `install_mcp_server` `uninstall_mcp_server` `list_mcp_marketplace` `get_mcp_call` `search_mcp_calls` |
| `document` | `create_document` `edit_document` `delete_document` `move_document` `read_document` `list_documents` `search_documents` |
| `skill` | `get_skill_execution` `search_skill_executions` |

**`activate_tools` meta-tool**:
- 参数:`{ category: enum[function, handler, workflow, mcp, document, skill] }`。
- `Execute`:把 category 写入当前 run 的 `AgentState.ActivatedGroups`;返回该组工具名清单(让模型确认激活了什么)。
- `IsReadOnly=true`,常驻。

**激活集状态**:落在已存在的 per-conversation `agentstatepkg.AgentState`(随 `convQueue` 生命周期),仿照现有 `ActiveSkill` 的 mutex 模式新增:
```go
ActivateGroup(cat string)        // activate_tools 调用
ActivatedGroups() []string       // loop 每轮读
```

**loop 改造(核心)**:现状 `loop.Run` 在循环外 `baseReq.Tools = ToLLMDefs(host.Tools())` 设置一次。改为**每轮重算**:
- `host.Tools(ctx)` 依据 `AgentState.ActivatedGroups()` 返回「常驻 + 已激活组」。
- 循环内每个 step:`req.Tools = ToLLMDefs(host.Tools(ctx))`。
- 效果:step N 调 `activate_tools("forge")` → step N+1 的 `req.Tools` 即含 forge 组。

**工具分组落点**:不改 `Tool` 接口(66 把工具零改动)。在注册层(`main.go` 或新 `internal/app/tool/registry.go`)把工具分为 `resident []Tool` + `lazy map[string][]Tool`,注入 `chat.Service`;`host.Tools(ctx)` 据此组装。

### 4.4 schema 与描述瘦身(M3-a)

**(a) injectStandardFields 去重** —— 省 ~13–14k token(占 28k 的 53%,最大单项)

**实现状态（已落地）**：`injectStandardFields` 注入三字段的**精简壳（slim shells）**，不是全部移除：
- `summary` 壳：`{"type":"string","description":"One sentence: what you're doing and why."}` — 保留约束 LLM 输出格式；仍进每把 schema 的 `required`。
- `destructive` 壳：`{"type":"boolean","default":false,"description":"true if this call may be irreversible; see the tools section."}` — 保留字段以解析调用结果；long guidance 移至 `tools` 段。
- `execution_group` 壳：`{"type":"integer","minimum":1,"description":"Parallel-batch id; see the tools section."}` — 同上；long guidance 移至 `tools` 段。

三个字段**全部保留在 schema**（slim shells），仅去除了冗长的 inline 说明。`StripStandardFields` 侧零改动，照常解析。

**(b) 工具 Description() 瘦身** —— 省 ~2k token
实测工具 Description 合计 ~5.3k token,中位仅 87 字符(合格),但 4 个 forge 巨无霸占 ~2.4k:`create_workflow`(3594 字符)、`create_handler`(3469)、`edit_workflow`(1470)、`edit_handler`(1122)。瘦法(描述层的渐进披露):`Description()` 只留一句"它是干嘛的"(菜名,≤ ~80 字符),把"怎么写 / 参数约定 / DAG 语法 / 示例"等长指引(菜谱)移到 `activate_tools("forge")` 的返回引导 —— 模型激活 forge 时才给、平时不占,且 create/edit 共享一份(不再各抄)。第二梯队(`Edit`/`Read`/`Write`/`write_memory`/`list_mcp_marketplace` 等 500–700 字符)可顺手精简,收益小、非必须。

### 4.5 prompt caching(M3-b)

- 稳定可缓存前缀 = `identity` + `how_to_work` + `tools` 段 + 常驻 tool 定义。这些跨请求不变。
- Anthropic 路径:在 `internal/infra/llm/anthropic.go` 给最后一个常驻 tool / system 段打 `cache_control:{type:"ephemeral"}`。
- OpenAI / DeepSeek 路径:自动前缀缓存,只需保证前缀字节稳定(顺序固定、不在前缀里塞动态内容)——§4.1 的段顺序已满足。
- 注意:`capabilities` 菜单随资产变,排在可缓存前缀**之后**,其变动不击穿 tool 缓存。
- token 数不变,但缓存命中部分单价降至 ~10%。

### 4.6 顺带修的 bug

1. **document catalog source 未注册** → `main.go` 补 `RegisterSource(documentService.AsCatalogSource())`。
2. **workflow 无 catalog source** → 新增 `workflow/catalog_source.go` + 注册(§4.2)。
3. **`trigger_workflow` 缺聊天工具** → 执行引擎(`scheduler.StartRun`)已存在,本次补一个薄工具注册即可(§10);prompt 与菜单照常引用 `trigger_workflow`。

---

## 5. 端到端数据流推演

**场景 A · 简单对话("帮我看下这个报错")**
触发 → chat handler → loop step1:`req.Tools` = 常驻 16 把 → LLM 用 `Read`/`Grep` 即可作答 → end_turn。
全程常驻 ~4k,零额外调度。

**场景 B · 改实体("把订单 handler 超时改成 30s")**
step1:模型看 `capabilities` 菜单知有 `order_webhook` handler,但手上无 `edit_handler` → 调 `activate_tools("forge")`。
step2:`host.Tools(ctx)` 返回 常驻 + forge 组 → 模型调 `edit_handler` → 改完 → end_turn。
比现状多一次 activate 往返,但避免每轮背 42 把 forge schema。

**场景 C · 大库找 function(用户有 300 个 function)**
`capabilities` 菜单退化为计数("functions: 300,search_function 查")→ 模型调 `search_function("invoice")` 拿候选 → `run_function(id)`。
菜单封顶在计数,system prompt 不随 300 个 function 膨胀。

---

## 6. 涉及文件与改动（已落地）

| 文件 | 改动 |
|---|---|
| `internal/app/tool/tool.go` | `injectStandardFields` 改为注入 slim shells（三字段保留，去长 guidance）(§4.4) |
| `internal/app/chat/multi_agent_prompt.go` | 精简;保留 `trigger_workflow` 引用 |
| `internal/app/chat/runner.go` | `tool_conventions` + `capabilities` 段 + `SystemPromptSections` 顺序（**实现在 runner.go，不是 chat.go**）|
| `internal/app/chat/host.go` `runner.go` | `Tools()` → `Tools(ctx)` 按激活集组装;loop 接线 |
| `internal/app/loop/loop.go` | 每轮重算 `req.Tools`(§4.3) |
| `internal/pkg/agentstate/` | 新增 `ActivatedGroups` 状态 + 方法 |
| `internal/app/tool/toolset/activate.go` | 新增 `activate_tools` meta-tool（**路径为 `toolset/activate.go`**）|
| `internal/app/tool/toolset.go` | `Toolset{Resident []Tool, Lazy map[string][]Tool}`;工具分组；注册 66 工具 |
| `internal/domain/catalog/source.go` | `CatalogSource` 加 `InvokeTool()` |
| `internal/app/catalog/mechanical.go` | 报菜名（`name [invokeTool]: desc`，desc 截 48 字符）|
| `internal/app/workflow/catalog_source.go`(新) | workflow source（`InvokeTool()="trigger_workflow"`）|
| `cmd/server/main.go` | 注册 workflow + document source;工具分组接线 |
| `internal/infra/llm/anthropic.go` | `cache_control:{type:"ephemeral"}` 打在最后一个常驻 tool + system 段(§4.5) |

---

## 7. token 账本

| | 现状 | 改后(常驻，实测) |
|---|---|---|
| system prompt 文本 | base + multi_agent + **全量 catalog(每条无约束)** | system ~2.9k bytes；catalog 随实体数线性增长但**每条十来字**(100 实体 ≈ +1k) |
| tools 参数 | 64 把全展开 + 每把重复注入 ≈ 28k | **28 把常驻 tool slim schemas ≈ 17.5k bytes**（实测）|
| **合计** | **~28k，且无上限** | **实测 ~5.1k token**（system ~2.9k + 28 常驻 tools ~17.5k ≈ 20.4k bytes ≈ 5.1k token）|
| 计费 | 全价 | 稳定前缀可缓存 → 命中再 −90% 单价 |
| **工具数** | 64 | **66**（workflow + document catalog source 补齐；activate_tools 新增）|

---

## 8. 测试策略

- **单测**:`injectStandardFields` 去重后 schema 不含 destructive/execution_group(`tool_test.go`);catalog 报菜名格式 + 分级阈值(`mechanical_test.go`);`activate_tools` 写入 AgentState(新);`host.Tools(ctx)` 按激活集返回正确集合。
- **pipeline**(`test/<domain>_pipeline_test.go`,fake LLM):场景 B 端到端 —— 模型 `activate_tools("forge")` 后下一轮能见到 `edit_handler`;场景 A 不激活也能用常驻工具作答。
- **token 回归**:新增断言「新号一条 hello 的 system prompt + 常驻 tools 字节 < 阈值」防回归膨胀。
- 幂等:复用 `harness.New(t)`,新内存 SQLite 天然隔离。

---

## 9. 文档同步(§S14)

- `service-design-documents/catalog.md`:统一菜单 + 分级 + `InvokeTool` + 补 source。
- `service-design-documents/chat.md`:`tool_conventions` / `capabilities` 段;`activate_tools`;loop 每轮重算 tools。
- 新建 `service-design-documents/` 中 tool-disclosure 或并入现有 tool 文档:常驻/长尾分组、`activate_tools` 契约。
- `service-contract-documents/`:`activate_tools` 工具契约;catalog 菜单格式。
- `backend-design.md`:新增"能力披露层"跨域模式。
- `progress-record.md`:本重构 dev log(含修掉的 3 个 bug)。
- `CLAUDE.md`:若 `Tool` 接口/注册模式有变动则更新 §S18。

---

## 10. workflow 执行类工具:已就绪(post-audit 用户补全)

**现状(2026-05-26 核实)**:
- workflow 执行引擎 `app/scheduler`(~2587 行)早已交付(Phase 4 / 2026-05-13)。
- `trigger_workflow` 薄工具**已补**(`workflow/trigger.go`,包 `scheduler.StartRunWithOptions`,支持 dryRun;commit 28793d6),description 已精简(~280 字符,best practice 达标,**不用重写**)。
- `get_workflow_execution` / `search_workflow_executions` **已注册**(`WorkflowExecutionTools`,main.go:543)。

**本次只需**:把这三个工具归进 `activate_tools("workflow")` 长尾组(§4.3 已含);catalog 的 workflow source `InvokeTool()="trigger_workflow"`。**无需新建任何工具**。
