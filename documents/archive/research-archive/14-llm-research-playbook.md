# 14 — LLM Prompt Forging Playbook(死结论,直接抄)

**模型**:DeepSeek V4-flash(Forgify 默认)。**方法**:真 LLM 迭代淬炼(读 trace→根因→改→再测),非跑分。
**数据/迭代日志**:[`13-llm-research-report.md`](./13-llm-research-report.md)。**计划**:[`docs/superpowers/specs/2026-05-29-llm-prompt-forging-plan.md`](../../../../docs/superpowers/specs/2026-05-29-llm-prompt-forging-plan.md)。

**状态:Round 1+2+3 完成,全 LLM-facing 表面 🟢**(~¥8.8 / ¥200,~6000 runs)。7 个 master 发现(G1-G7)。所有死结论有实测 N 支撑。R3 补全了 edit ops / 结构化参数 / 注入字段 / catalog / lazy 重淬 / 4 rerank / subagent / section-order。

---

## §0 全局发现(跨所有 surface,最高优先级)

这几条是淬炼中反复验证、影响**每一个** LLM-facing 调用的结论。先讲一次,各 surface 不重复。

> ⚠️ **指标修正(重要)**:本研究多数 surface 用"单-turn 一次答对"测。对 **多轮 agent 循环这是错的尺子** —— 它把"模型先思考 / 先反问 / 先 search 确认"误判成失败,而这些在真实对话里是**正常甚至正确**的。受影响最大的是 G1(thinking)。看本报告任何数字时,先问:"这个'失败'是真错,还是只是模型多轮里合理地没在第一 turn 直接调?" 下面 G1 已按此修正;其余 surface 的"配方"里凡写 thinking-off 的,除复杂长 ops 外均非必需。

### G1.(已修正)thinking ≠ 元凶 —— 别全局关;仅复杂长 ops 输出有 ~15% 真畸形

⚠️ **本条第一版结论("所有结构化调用关 thinking")错了,经隔离实验推翻。** 诚实记录:

**初版误判**:单-turn 测 thinking-ON 时 CEL 50% / agent 47%,关 thinking 涨到 100%/95%。我据此说"关 thinking"。
**真相(隔离实验)**:那些"失败"几乎全是 **`called None` —— 模型先思考 / 先反问 / 先解释**。这在**多轮对话里完全正常**(想清楚 / 问一句 → 下一轮再调)。单-turn 指标把"这轮没直接调"误判成失败。

隔离验证:
- **CEL 补全场景 + thinking-ON = 92%**(其中"失败"8 个是 trap 里模型**正确解释"case 不能做情感分析"**,实属对的行为 → 真实正确率 ~99%)。即 50→100 的 thinking-off"大涨"是**纯指标幻觉**。
- workflow thinking-ON max_tokens=12000(排除截断):valid 84%,**malformed-COMPLETE 15%**(JSON 完整生成但语法坏)—— 这才是 thinking 的**真**代价。

**修正后的死结论**:
1. **不要全局关 thinking。** 对话循环 / CEL / agent / 简单工具:thinking 正常且有益(帮模型想清楚),保留。"called None = 先想/先问"是正常多轮行为,不是失败。
2. **唯一真例外:复杂长 ops 结构化输出(workflow create/edit)**,thinking-ON 有 ~15% 畸形 JSON(非截断)。这里两条路任选:**(a)** 该次调用 thinking-off;**(b)** 保留 thinking + **parse 错误自动重试**(真实 agent loop 本来就该有,畸形→报错→下一轮修)。倾向 (b),更通用。
3. 实现:`infra/llm` adapter **支持** per-call thinking 开关(给 (a) 留口),但默认开;tool-call JSON parse 失败要能优雅重试。

(教训:**单-turn "一次答对"指标对多轮 agent 是错的尺子** —— 见 §0 头部"指标修正"。)

### G2. 🔴 复杂结构输出:**max_tokens 必须够高(≥8000)**

workflow ops 数组长(6 节点全功能图实测输出 7900+ tokens)。`max_tokens=3000` 时复杂图被**截断成半个 JSON** → 解析失败,看着像"LLM 不会"实则是"被腰斩"。

| max_tokens | workflow 编排一次对 |
|---|---|
| 3000 | 78%(branch/full 因截断掉到 40-50%) |
| **8000** | **97%** |

**死结论**:create_workflow / edit_workflow / 任何产长 ops 的调用,后端 `max_tokens ≥ 8000`。否则复杂 workflow 静默截断。

### G3. callable ref 允许下划线 + 用真实 id

ref 语法 `fn_xxx`/`hd_xxx.method`/`ag_xxx`/`mcp:server/tool` 的 entity-id 部分**可含下划线**(Forgify hex id `fn_a3f2…` + 人类命名 `fn_send_email` 都合法)。validator/正则别把下划线判非法。

### G4. 教学过度有害(concise > verbose)

workflow:中等教学(节点类型 enum + op-key,V2)= 97%;**加全套 CEL 教学 + 完整 example(V3)反降到 90%**。长描述增加截断/干扰。**每个 surface 给"刚好够"的教学,不堆**。(与早期 tool-desc 研究的 V5-combined 结论一致方向。)

### G5. 🔴 `search_X` 是按-ID 操作的**通用 on-ramp**(53 工具 sweep 实证)

53 工具单-turn 体检发现:**31/53 工具"首-turn 没调对",但几乎全是因为 LLM 先调了 `search_X`**:

| 用户说 | LLM turn 1 | turn 2 |
|---|---|---|
| "上线 wf_report" | `search_workflows` | activate_workflow |
| "接受 fn_x 待定版本" | `search_functions` | accept_pending_function |
| "试跑 ag_x" | `search_agents` | run_agent |
| "取消 fr_abc" | `get_flowrun` | cancel_flowrun |

**规律:任何"按 id 操作一个已存在实体"的工具(accept/revert/delete/run/activate/trigger/cancel/get/edit/call/update_config/replay/...),LLM 都先 search/get 确认实体,再操作。** 这是**正确行为**(不瞎编 id),不是描述缺陷。

**死结论(三条产品含义)**:
1. **单-turn "首工具对不对"指标对这 31 个工具无意义** —— 必须多-turn 测(见 §3)。
2. **`search_*` / `get_*` 必须随时可达**(常驻 or lazy 但 LLM 知道存在)—— 它们是所有 entity-anchored 操作的入口。印证早先 lazy 研究的 "search-first" 结论。
3. **search_X 的返回必须含 id**,否则 LLM turn 2 没法填 id 操作。

多-turn sweep 结果:**41/53 工具 100%(干净)**,整体 675/795 (84%)。残留 10 个 LOW 见 G6。

### G6. 🔴 按实体类型的工具需"类型消歧" guard + 足够 turn 预算

多-turn sweep 后仍 LOW 的 10 个工具,两类根因:

**(a) 实体类型混淆** —— LLM 默认走 function 家族。例:"调 hd_db 的 query 方法" → LLM 走 `search_functions→get_function→run_function`(无视 `hd_` 前缀),不调 call_handler。

> **修法(已证)**:handler/skill/memory 系工具的 description 加一行类型 guard:
> `call_handler: "Call a method on a HANDLER (hd_ entity, stateful class). For functions (fn_) use run_function."`
> 实测 accept_pending_handler **0/15 → 12/12**。

**(b) search→get→act 链需 5+ turn** —— call_handler(search_handlers→get_handler 看方法→call)是 3-4 步,4 turn 偶尔不够。生产环境无 turn 上限,非问题。

**(c) canned-result 假象(call_mcp_tool/activate_skill/forget_memory 仍 0)** —— 读 trace:LLM 调 search_skills/read_memory/list_mcp,canned 结果**不含**请求的实体(harness echo 只覆盖 fn_/hd_/ag_/wf_ id,不覆盖 skill 名/mcp server/memory query),LLM **正确地报"没找到该实体、拒绝操作不存在的东西"** —— 这是**对的安全行为**,不是失败。真实环境 search 会返回该实体 → LLM 就 activate/call/forget。

> **G6 验证 + 全 roster 复测**:加类型 guard + 系统 prompt 类型图例 + 真实 canned echo + 5 turns 后,**多-turn 84%→88%,45/53 工具 100%**。剩 8 个 LOW 三类:类型 guard 救活的(accept_pending_handler 0→12)、obscure 工具名被合理替代的(capability_check_workflow → LLM 用 get_workflow 看 refs,语义对)、全-roster 竞争干扰的。**无根本 LLM 失败**。
> **(d) obscure 工具名**:capability_check_workflow / install_mcp_from_registry 等名字不直观的工具,LLM 倾向用更熟的近义工具替代。修法:这类工具 description 首句点明独特用途 +"无替代"(如 "Pre-flight check that a workflow's referenced callables still exist & match — do this BEFORE activate; get_workflow does NOT validate refs")。
> **死结论**:① 每个按"特定实体类型"操作的工具,description 一行写明 `for hd_/sk_/mem_ entities`(默认 fn_ 家族会抢);② 系统 prompt 教 `fn_=function, hd_=handler, sk_=skill, mem_=memory`;③ 别给 multi-step chain 设过紧 turn 上限;④ search_* 返回真实含 id —— LLM 不会瞎编、不存在就拒(安全)。

### G7. 🔴 关键规则放系统 prompt **末尾**(recency 主导,反 "rules-first")

实测:一条 critical 守则("never run_agent on ag_sum02")放系统 prompt **开头 vs 末尾**:

| 位置 | 遵守率 |
|---|---|
| rules-first(放最顶,被 base prompt 压住)| **0/20**(全违规,照样调 ag_sum02)|
| rules-last(放末尾,贴近 user turn)| **20/20**(全遵守)|

DeepSeek V4-flash 是 **recency 主导** —— 离 user 消息越近的指令越被遵守。**与 OpenAI 的 "rules-first" 启发相反**(模型差异)。

**死结论**:Forgify 系统 prompt 里**最关键的守则 / 安全约束放最后一段**(紧贴 user turn),不要埋在 identity/how_to_work 开头。`buildSystemPrompt` 的段顺序:基础信息在前,critical 约束殿后。

---

## §0.1 总 scorecard

| Surface | baseline | 淬炼后 | 配方 | verdict |
|---|---|---|---|---|
| **CEL case 表达式** | 40% | **100%** | full CEL 教学 + **thinking-off** | 🟢🟢 |
| **function 锻造** | 0%* | **100%** | V5(antipattern+polling cursor 例)+ thinking-off | 🟢🟢 |
| **handler 锻造** | — | **98%** | bare-names contract + 例 + thinking-off(counter/db/oauth 全满分) | 🟢 |
| **workflow 编排** | 0% | **97%** | V2 node-types 教学 + thinking-off + max_tok 8000 | 🟢 |
| **agent forging** | 0% | **95%** | V3 教学(员工思维)+ thinking-off | 🟢 |
| **callable ref** | 52% | **93%** | ref 语法表 + 正则允许下划线 | 🟢 |
| **Utility 场景** | — | **100%** | 简洁 prompt + thinking-off(title/rerank/envfix/compact/websum 全满分)| 🟢🟢 |
| **error-envelope 恢复** | 45%(prose)| **100%**(sentinel+next_step)| 结构化错误 `{error,field,got,expected,next_step}` | 🟢 |
| **89 工具体检** | 44%(单-turn 假象)| **多-turn 88%**(45/53 工具 100%,G6 guard 后)| search-first + thinking-off + 类型 guard(G6) | 🟢(45 个)/ 🟡(8 个 obscure,见 G6) |
| **edit_workflow**(最恶心 case)| — | **100%** | get→edit + thinking-off + is_edit 校验 | 🟢🟢 |
| **edit_function/handler/agent ops** | — | **~99%**(enum ops;eh "加方法" 用 update_code 也对)| enum discriminated ops + thinking-off | 🟢 |
| **结构化参数工具**(trigger/call_handler/call_mcp/run_*/update_config/replay)| — | **99%**(给定 id 时参数填对)| thinking-off;sweep 0/15 纯 search-first 假象,非参数质量 | 🟢🟢 |
| **注入字段**(summary/destructive/execution_group)| — | **100%**(含/不含 tool_conventions 都对)| schema 字段描述 + thinking-off 即够 | 🟢🟢 |
| **catalog 渲染** | — | 两种格式都选对实体;descriptive 更谨慎(缺输入会问)| 系统 prompt asset 菜单 | 🟢 |
| **lazy 分组**(forging 框架重淬)| 0%(有 resident search)| **83%**(V4:11 组 + search 移进 lazy)| 无 resident search,activate-first | 🟢 |
| **4 rerank**(fn/hd/skill/mcp)| — | **100%**(4×15/15)| "Output ONLY JSON id array, no prose" + thinking-off | 🟢🟢 |
| **subagent prompt**(explorer/forger)| — | **100%**(explorer 守只读、forger 正确建)| 角色约束 + thinking-off | 🟢🟢 |
| **section-order**(critical 规则位置)| 0%(rules-first)| **100%**(rules-last,G7 recency)| 关键守则放系统 prompt 末尾 | 🟢 |

\* function baseline 0% 指无教学的复杂 polling;简单 normal function 一直 ~100%。

**核心结论(已修正)**:之前说"thinking-off 是万能钥匙"是**错的**(见 G1)—— 那些大涨是单-turn 误判"先想/先问"为失败。真正稳健的杠杆是 **M2 max_tokens 够高 + M5 ref 正则 + M6 类型 guard + M7 关键规则殿后 + 好的描述/schema/教学**。thinking 只在复杂长 ops 输出上需特殊处理(off 或 parse-重试)。

---

## §1 Forge 实体工具死结论

### §1.1 `create_workflow` / `edit_workflow` — `[🟢 97% · ~1850 out-tok · lazy组: workflow-edit · 依赖: accept_pending_workflow]`

#### 就这么写 `Description()`(直接贴 `backend/internal/app/tool/workflow/create.go`):

```
Create a workflow by applying graph ops.
NODE TYPES (only these 5 exist):
  trigger  — workflow entry. config: {kind: cron|fsnotify|webhook|polling|manual, payloadSchema?}
  agent    — LLM step. config: {agentRef: "ag_xxx"}  (thin wrapper; prompt/tools live on the agent entity)
  tool     — call a forge callable. config: {callable: <ref>, args: {...}}
             callable ref: "fn_xxx" | "hd_xxx.method" | "mcp:server/tool" | "ag_xxx"
  case     — route by CEL. config: {expression: <CEL>, branches: {<name>: {to: <nodeId>, emit?: {<field>: <CEL>}}}}
             a branch's `to` may point to an UPSTREAM node → forms a loop (cyclic graph is allowed).
  approval — wait for user yes/no. config: {prompt: <markdown>, branches: {approved:{to}, rejected:{to}}}
```

(`edit_workflow` 描述:`Edit a workflow by applying graph ops (same op shapes as create_workflow).`)

#### 就这么写 `Parameters()`:

```json
{"type":"object","required":["name","ops"],"additionalProperties":false,"properties":{
  "name":{"type":"string"},
  "ops":{"type":"array",
    "description":"Graph ops. Each op.op ∈ {add_node, remove_node, connect, disconnect, update_config}.",
    "items":{"type":"object","required":["op"],"properties":{
      "op":{"type":"string","enum":["add_node","remove_node","connect","disconnect","update_config"]},
      "node":{"type":"object","description":"for add_node: {id, type∈[trigger,agent,tool,case,approval], config}"},
      "from":{"type":"string"},"to":{"type":"string"},
      "nodeId":{"type":"string"},"config":{"type":"object"}}}}}}
```

#### 调用约束(后端/客户端,见 §0):
- **thinking 关**(G1):94%→97% + 消除畸形 JSON
- **max_tokens ≥ 8000**(G2):否则 6 节点图截断

#### 为什么这么写(逐轮 Δ,真实数据):

| 轮 | 改动 | 一次对 | 消灭的失败模式(读 trace) |
|---|---|---|---|
| v1 | 泛型 `ops: array of object` | **0%** | LLM 退回 React-flow `data` 字段 / `type` 当 discriminator / camelCase `addNode` / 老节点 `function`+`condition` |
| v2 | + `op` key enum + 5-node-types 教学 | 78% | 消灭 discriminator 错 + 老节点类型(trap 18/20 正确用 tool+case) |
| v3 | + max_tokens 3000→8000 | 94% | 消灭复杂图截断(branch 10→20,full 8→17) |
| v4 | + thinking OFF | **97%** | 消灭重度 reasoning 后的畸形 JSON(loop 17→19,trap 19→20) |

(注:再加全套 CEL 教学 + example = V3-full,反降到 90% —— 过度教学,见 G4。)

#### 别这么写(top 3 致命反例):

- ❌ 泛型 ops 无结构 → **0%**,LLM 必退回 React-flow / 老节点格式
- ❌ max_tokens < 8000 → 复杂图静默截断,假装成"LLM 不会"(实则被腰斩)
- ❌ 堆全套 CEL 教学 + 长 example → 过度教学,97%→90%

#### 🔴 edit_workflow(你点名的"最恶心" case)实测 **100/100 (100%)**:

改已有图的 5 类操作各 20/20:**插中间节点**(fetch→ag_clean→save)/ **加 case 分支**(+approval)/ **加回边重试**(loop-back + attempt+1)/ **重连边**(insert validate)/ **删节点改接**(remove save → ag_report)。

流程:LLM 先 `get_workflow` 看现图(search-first),再 emit edit ops 引用**已有节点 id**。配方同 create(V2 描述 + thinking-off)。

⚠️ **产品要点(capability check)**:edit_workflow 的校验**不能要求 ops 里有 trigger 节点**(trigger 已在现图里)。`validate(ops, is_edit=true)` —— create 要 trigger,edit 不要。否则误拒所有合法 edit。

#### 残留 / 已知限制:

- create `wf-full`(6 节点全功能 cron→tool→agent→case→approval→tool)18/20 = 90%;`wf-loop` 19/20。最复杂的从零建图偶有边连接小错。已达 🟢 bar。
- edit_workflow 无残留(100%)—— 改已有图比从零建更稳(有现图 id 锚点)。
- case 节点的 CEL 正确性由 CEL surface(§见 CEL 段)单独保证;case-heavy workflow 建议 description 里补 CEL null-safety 一行(详 CEL 段),但**别补全套**(过度教学)。

### §1.2 `create_agent` / `edit_agent` — `[🟢 95% · thinking-off · lazy组: agent-edit · 依赖: accept_pending_agent]`

#### 就这么写 `Description()`(贴 `backend/internal/app/tool/agent/create.go`):

```
Create a new agent (forge entity = configured LLM ReAct loop, a "worker") via ops.
An agent is a forge entity = a configured LLM ReAct loop (a "worker").
Mounts (set via ops):
  prompt        — one instruction block (NOT split system/user); supports {{ payload.* }} / {{ ctx.* }}
  skill         — 0 or 1 skill name (pre-activated methodology)
  knowledge     — list of document refs (injected directly, no RAG)
  tools         — forge callables only: fn_xxx | hd_xxx.method | mcp:server/tool | ag_xxx
                  (NEVER platform tools — no fs/shell/web/memory/ask/subagent)
  outputSchema  — enum | json_schema | free_text
  model         — {apiKeyId, modelId} (optional; falls back)
DO NOT mount platform tools (fs/shell/web/memory/ask/subagent) — agents are workers, not bosses.
DO NOT split prompt into system/user — one block only.

Example:
  create_agent(name="classifier", ops=[
    {"op":"set_prompt","value":"Classify the email in {{ payload.text }} as invoice|inquiry|spam."},
    {"op":"set_output_schema","value":{"kind":"enum","values":["invoice","inquiry","spam"]}},
    {"op":"set_tools","value":["fn_fetch_sender_history"]}
  ])
```

#### `Parameters()`:ops 数组,`op.op ∈ {set_meta,set_prompt,set_skill,set_knowledge,set_tools,set_output_schema,set_model}`(enum)+ `value`。

#### 调用约束:**thinking 关**(G1)—— 决定性。

#### 为什么(逐轮 Δ):
| 轮 | 改动 | 一次对 | 消灭 |
|---|---|---|---|
| v1 | 泛型 ops | 0% | 无结构,乱编 |
| v2 | + 教学(mounts + 员工思维 + 例) | 47% | classify/knowledge 满分,但 tools 因正则 bug=0,trap `called None` |
| v3 | + 正则允许下划线(G3) | 82% | tools 0→20, full 0→18(ref 误判修复) |
| v4 | + thinking OFF | **95%** | trap-platform-tools 4→15(thinking 时模型反问/拒,关后直接正确建 agent 不挂平台工具) |

#### 别这么写(top 3):
- ❌ 泛型 ops → 0%
- ❌ 不写"NEVER platform tools"→ 用户要"读文件/上网/记笔记"时 LLM 会挂 fs/web/memory(违反员工思维)
- ❌ thinking on → 模型反问而非建 agent

#### 残留:trap-platform-tools。Round 2 在教学里加一句"用户要文件/上网/记笔记 → 改 create_function 锻造再挂 fn_xxx,绝不挂裸 filesystem/web/memory" → **15/20 → 18/20**。剩 2/20 是长 ops 截断(非语义错)。死结论描述已含这句(见上方 Description 倒数第 3 行)。

---

## §1.4 `create_function`(含 polling)— `[🟢 100% · thinking-off]`

#### `Description()`(贴 `function/create.go`):
```
Create a new function (stateless Python callable).

DO NOT use for stateful classes (use create_handler).

kind:
  normal  — on-demand callable (workflow tool node / agent tool).
  polling — system runs on an interval (requires polling_interval, e.g. "60s").
            A polling function MUST accept last_cursor and return {events: [...], next_cursor: ...}.
            Cursor pattern (copy this):
              def poll(last_cursor):
                  items = fetch_since(last_cursor)
                  return {"events": items, "next_cursor": items[-1].ts if items else last_cursor}
```
#### 为什么:V5(antipattern + polling cursor 完整例)+ thinking-off → add/time/poll-gmail/**poll-cursor**/trap-webhook **全 20/20**。cursor 例直接消灭"polling 漏 cursor"。
#### 反例:❌ 无 cursor 例 → polling 函数漏 last_cursor/next_cursor(早期 tool-desc 研究 V1-terse 仅 70%)。

## §1.5 `create_handler` — `[🟢 98%(真实场景)· thinking-off]`

#### `Description()`(贴 `handler/create.go`):
```
Create a handler (stateful Python class).
A handler is a stateful Python class. Forgify uses a BARE-NAMES body contract:
  - __init__ receives init args as BARE parameters (not a dict): def __init__(self, db_url): ...
  - each method receives its args as BARE parameters: def query(self, sql): ...
  - DO NOT access args via dict (no args["sql"] / init_args["db_url"]).
  - init_args_schema declares the init params; methods_schema declares each method's params.
Example:
  create_handler(name="db", code="class DB:\n    def __init__(self, db_url):\n        self.conn = connect(db_url)\n    def query(self, sql):\n        return self.conn.run(sql)",
    init_args_schema={"db_url":"string"}, methods_schema={"query":{"sql":"string"}})
```
#### 为什么:bare-names contract + 例 + thinking-off → counter/db/oauth 全 ~20/20(db 19/20)。bare-names 例消灭 `args["sql"]` 字典访问。
#### 测试注记:`hd-trap-stateless` 0/20 是**测试设计 bug**(hd 模式只 offer 了 create_handler,LLM 无 create_function 可选)—— 不计入。真实 3 场景 59/60。

---

## §1.6 CEL case 表达式 — `[🟢🟢 100% · thinking-off]`

#### 就这么写(case 节点 `expression`/`branches` 教学,放 create_workflow 的 case 段 + set_case_branches):
```
CEL quick reference:
  payload.x        field access      ctx.triggerKind   metadata
  has(payload.x)   presence check    x.size()          length of string/list
  &&  ||  !        boolean           ==  !=  <  >  <=  >=   compare
  x in [a,b]       membership        "s" in x          substring/contains
Branches: each branch has {to: "<nodeId>", emit?: {<field>: <expr>}}.

NULL-SAFETY (critical — unguarded nested access errors at runtime):
  BAD :  payload.user.email.size() > 5
  GOOD:  has(payload.user) && has(payload.user.email) && payload.user.email.size() > 5

BOUNDARY: case is a dealer, not an analyst. It ROUTES on data already present.
  It CANNOT analyze/compute (no sentiment, no summarization). If routing needs
  analysis, that must be done by an upstream agent node first; case only reads its output.

LOOP: a branch `to` may point upstream. On a retry branch, emit the incremented counter:
  {to: "solve", emit: {attempt: "payload.attempt + 1"}}
```
#### 为什么(逐轮 Δ):
| 轮 | 改动 | 一次对 |
|---|---|---|
| v1 | 无教学 | 40%(numeric/contains 极低) |
| v2 | + full CEL 教学(null-safety + boundary + loop) | 50%(仍大量 `called None`) |
| v3 | + thinking OFF | 88%(simple/nullsafe/contains/loop 全 20/20;numeric 仍 7/20 反问目标节点) |
| v4 | + 场景补全分支目标 ID(真实建图时下游 ID 已知) | **100%** |
#### 关键洞察:CEL 表达式本身 LLM 写得**完美**(`has(payload.user) && ...` 一次对);失败全是 thinking 导致的"不发 tool_call"或场景缺目标 ID 导致的反问。**修这俩,CEL 100%**。
#### 反例:❌ thinking on(numeric 16/20 光想不调);❌ 场景给的路由目标不明确 → LLM 正确地反问(不瞎编 nodeId)。

---

## §2 callable ref 死结论 — `[🟢 93%]`

#### 就这么写(tool 节点 `callable` 字段教学,放 create_workflow + agent set_tools 描述里):

```
Callable ref syntax (the `callable` field):
  function       → "fn_<id>"              e.g. fn_send_email
  handler method → "hd_<id>.<method>"     e.g. hd_db.query
  mcp tool       → "mcp:<server>/<tool>"  e.g. mcp:slack/post
  agent          → "ag_<id>"              e.g. ag_summarize
```

#### 为什么(逐轮 Δ):
| 轮 | 改动 | 一次对 |
|---|---|---|
| v1 | 无 ref 语法表 | 52%(function/mcp 全 0 — LLM 不知前缀格式) |
| v2 | + ref 语法表 | 80% |
| v3 | + 正则允许下划线(G3,validator 修正)| **93%** |

#### 残留:handler/mcp 偶尔(17-19/20)把 method/server 拆错。

---

## §3 Utility 场景死结论 — `[🟢🟢 100%]`

全部 thinking-off + **简洁 prompt + "Output ONLY X, no prose"**:

| 场景 | 一次对 | 死结论 prompt 要点 |
|---|---|---|
| auto-title | 15/15 | `"Output ONLY the title, max 6 words, no quotes, no punctuation at end."` |
| rerank ×2 | 15/15 | `"Output ONLY a JSON array of candidate ids, most relevant first. No prose."` |
| env-fix ×2 | 15/15 | `"Output ONLY a JSON object {\"deps\": [...]}. No prose."` |
| compaction | 15/15 | `"Summarize ... into <= N chars, preserving key decisions and open questions."` |
| web-summary | 15/15 | `"Summarize ... in <= N chars, plain text."` |

铁律:**"Output ONLY … No prose." + thinking-off** —— 杜绝模型加解释/markdown fence 包裹。

## §4 error-envelope 死结论 — `[🟢 sentinel 100% vs prose 45%]`

工具失败时,返回**结构化 sentinel**让 LLM 一轮恢复;prose 让它重复同样的错。

#### 就这么返(HTTP error body / tool_result 错误):
```json
{
  "error": "INVALID_KIND",
  "field": "kind",
  "got": "async",
  "expected": ["normal", "polling"],
  "next_step": "Use 'polling' for scheduled jobs (needs polling_interval), or 'normal' for on-demand. Re-call with a valid kind."
}
```

#### 数据:同一个 bad call(`kind=async`),返回错误后下一轮恢复率:
| 错误形式 | 恢复率 |
|---|---|
| prose `"the kind value is not allowed"` | **9/20 (45%)** — 11/20 重复 `kind=async` |
| sentinel + `next_step` | **20/20 (100%)** |

#### 实施:`backend/internal/transport/httpapi` error envelope 在现有 `{code,message,details}` 基础上,details 里带 `field`/`got`/`expected`/`next_step`。**`next_step` 是关键字段** —— 它直接告诉 LLM 下一步怎么做。

## §5 非工具 artifact:catalog / lazy 分组

由 §0 G5(search-first)+ G3 + 早先 lazy 研究(doc 13 旧版 V4:search 移 lazy / 11 组)共同覆盖,本轮无新增矛盾。死结论:**search_* 随时可达 + 返回含 id;activate_tools 11 组(详早先研究)**。

## §6 复合端到端场景 — `[🟢 ~100%(用收敛描述串联)]`

用**收敛后的全部描述 + thinking-off** 跑真实多步任务(多-turn,喂 canned 结果):

| 场景 | 一次完成 | 说明 |
|---|---|---|
| 诊断链(查挂掉 flowrun → events → 死信 → replay) | **15/15** | 多步诊断流畅,~6 turn |
| 多实体锻造(建 agent + function + 接进 workflow + 路由) | **15/15** | ~8 turn,长链完整串起 |
| 上线(accept pending + activate) | **15/15** | ~4 turn,search→accept→activate |
| **合计** | **45/45 (100%)** | |

**关键证据**:LLM 用收敛描述能把皇冠级复杂任务端到端做完。**初版 44% 是 harness 假象**(canned search 结果不回显请求的实体 id → LLM 死循环搜索;max_turns 太少)。修 canned 回显 id + turns 8 后 → ~100%。**真实产品里 search 会返对应实体,所以这是真能力**。

⚠️ **harness 教训(也是产品教训)**:多-turn 链里 search_X 的返回**必须含被请求的 id**,否则 LLM(正确地)不瞎编、持续重搜 → 卡死。印证 G5 第 3 条。

## §7 实施 roadmap(贴哪个文件)

| 改动 | 文件 | 来自 |
|---|---|---|
| tool-call JSON parse 失败 → 优雅重试(下一轮修)| `infra/llm` / chat loop | G1(修正版,通用兜底)|
| DeepSeek adapter **支持** per-call thinking 开关(默认开;仅复杂长 ops 可选 off)| `internal/infra/llm/deepseek*.go` | G1 |
| create/edit_workflow 后端 max_tokens≥8000(或不设硬 cap)| workflow dispatch / llm 调用处 | G2 |
| 各工具 `Description()` 用 **§8 全目录** 文本 | `app/tool/*/` | §1 + §8 |
| `edit_*` ops schema enum discriminated | edit.go | §3 schema |
| error envelope 加 `field/got/expected/next_step` | `transport/httpapi` + `domain/errors` | §4 |
| callable ref 正则允许下划线 | ref 校验处 | G3/M5 |
| 系统 prompt 加 id-前缀→工具家族图例 + 关键规则殿后 | `chat/runner.go::buildSystemPrompt` | G6/G7 |
| Utility prompt 用 "Output ONLY … no prose" | chat infra | §3 |
| search_* 返回含 id + 移进 lazy 组(非 Resident)| catalog / lazy 装配 | G5 |

---

## §8 全工具描述目录(直接抄 `Description()`)

应用淬炼原则:**简洁动作句 + 实体类型 guard(G6)+ 必要处给 example/反例 + 关键 schema 提示**。深淬工具(create/edit_X)给完整文本(见 §1),其余给最终一行描述(多-turn sweep 88% 验证过 + G6 guard)。

> 心智:工具名 + 一句"做什么 + 何时用 + 用哪类实体"。**id 前缀决定工具家族**(fn_/hd_/ag_/wf_)—— 系统 prompt 须有此图例(G6)。

### function 家族
```
search_functions   : "Search functions (fn_, stateless callables) by name/tag/desc."
get_function       : "Get a function's (fn_) active code + signature."
get_function_versions: "List a function's (fn_) version history."
create_function    : <见 §1.4 完整文本 —— antipattern guard + kind + polling cursor 例>
edit_function      : "Edit a function (fn_) via ops. op ∈ {rename, update_code, update_kind, update_description}. 加方法/改逻辑用 update_code。"
accept_pending_function: "Promote a function's (fn_) pending version to active."
revert_function    : "Revert a function (fn_) to a prior version number."
delete_function    : "Delete a function (fn_)."
run_function       : "Test-run a function (fn_) with args. NOT for handlers (hd_ use call_handler)."
search_function_executions: "List a function's (fn_) past execution records."
get_function_execution: "Get one function execution's detail."
```

### handler 家族(G6:全部点明 hd_,与 function 区分)
```
search_handlers    : "Search handlers (hd_, stateful classes)."
get_handler        : "Get a handler's (hd_) class def + init/methods schema."
create_handler     : <见 §1.5 完整文本 —— bare-names body contract + 例>
edit_handler       : "Edit a handler (hd_) via ops. op ∈ {rename, update_code, update_method, update_init_schema}."
accept_pending_handler: "Promote a handler's (hd_) pending version. (handler, not function.)"
revert_handler     : "Revert a handler (hd_) to a prior version."
delete_handler     : "Delete a handler (hd_)."
call_handler       : "Call a method on a HANDLER (hd_, stateful class) with args. For functions (fn_) use run_function."
update_handler_config: "Update a handler's (hd_) init args/secrets (AES-encrypted)."
search_handler_calls: "List a handler's (hd_) past method-call records."
get_handler_call   : "Get one handler call's detail."
```

### agent 家族
```
search_agents      : "Search agents (ag_, configured LLM workers)."
get_agent          : "Get an agent's (ag_) config (prompt/skill/knowledge/tools/model/outputSchema)."
create_agent       : <见 §1.2 完整文本 —— mounts + 员工思维(no platform tools)+ 例>
edit_agent         : "Edit an agent (ag_) via ops. op ∈ {set_prompt, set_skill, set_knowledge, set_tools, set_output_schema, set_model}. tools 只填 forge callable ref。"
accept_pending_agent: "Promote an agent's (ag_) pending version."
revert_agent       : "Revert an agent (ag_) to a prior version."
delete_agent       : "Delete an agent (ag_)."
run_agent          : "Test-run an agent (ag_) with a payload; returns output + tokens + latency."
search_agent_executions: "List an agent's (ag_) past run records."
get_agent_execution: "Get one agent run's detail."
```

### workflow 家族 + 生命周期
```
search_workflows   : "Search workflows (wf_). Filter by active state if asked."
get_workflow       : "Get a workflow's (wf_) graph (nodes + edges + active state)."
get_workflow_versions: "List a workflow's (wf_) version history."
create_workflow    : <见 §1.1 完整文本 —— 5 node types + callable ref + CEL>
edit_workflow      : <见 §1.1 —— 同 op 形状;改已有图,引用现有 node id;校验用 is_edit(不要求 ops 含 trigger)>
accept_pending_workflow: "Promote a workflow's (wf_) pending version to active."
revert_workflow    : "Revert a workflow (wf_) to a prior version."
delete_workflow    : "Delete a workflow (wf_)."
capability_check_workflow: "Pre-flight: verify a workflow's (wf_) referenced callables still exist & kinds match — do this BEFORE activate. get_workflow does NOT validate refs."  ← G6(d):名字不直观,首句点明独特用途防被 get_workflow 替代
activate_workflow  : "Activate a workflow (wf_): register its listener triggers so it runs automatically."
deactivate_workflow: "Deactivate a workflow (wf_): unregister listeners (manual trigger still works)."
trigger_workflow   : "Manually trigger a workflow (wf_) at a specific trigger node, with a payload matching that node's schema."
```

### 运行时 + 诊断
```
search_flowruns    : "List a workflow's (wf_) flowrun history (filter by status/time)."
get_flowrun        : "Get a flowrun's (fr_) overview (status/trigger/duration)."
get_flowrun_trace  : "Get a flowrun's (fr_) message causality trace (parent_id chain) — for debugging where it went wrong."
get_flowrun_nodes  : "Get per-node status of a flowrun (fr_) (running/done/failed/approval-pending)."
cancel_flowrun     : "Cancel a stuck/running flowrun (fr_)."
query_events       : "Query a workflow's (wf_) event stream — types: handler_crash / trigger_exhausted / dead_letter_created / etc."
list_dead_letters  : "List a workflow's (wf_) dead-letter messages (retry-exhausted)."
get_dead_letter    : "Get a dead-letter's (msg_) payload + ctx + failure stack."
replay_message     : "Replay a dead-letter message (msg_) — from its node or the whole flowrun."
clear_dead_letters : "Bulk-clear a workflow's (wf_) dead letters."
```

### 资产:mcp / skill / document / memory(G6:各点明实体类型)
```
search_mcp_tools   : "Search the installed MCP tool catalog."
call_mcp_tool      : "Call an installed MCP tool: needs server + tool + args (ref form mcp:server/tool). Find servers via list_mcp_servers first."
list_mcp_servers   : "List installed MCP servers."
install_mcp_from_registry: "Install a NEW MCP server from the registry (use when the server isn't installed yet — distinct from call_mcp_tool)."
health_check_mcp   : "Health-check an installed MCP server's connectivity."
search_skills      : "Search skills (methodology playbooks, by name)."
get_skill          : "Get a skill's content (by name)."
activate_skill     : "Activate a skill (by name) for this conversation."
search_documents   : "Search documents (knowledge base) by content."
list_documents     : "List documents (tree view)."
read_document      : "Read a document's content (by path)."
create_document    : "Create a document at a path with content."
edit_document      : "Edit a document's content (by path)."
move_document      : "Move/rename a document (from_path → to_path)."
delete_document    : "Delete a document (by path)."
read_memory        : "Read user memory entries (by query)."
write_memory       : "Write a user memory entry (name + content)."
forget_memory      : "Delete a user memory entry (by name). Find it via read_memory first."
```

**注**:`<见 §X>` 的几个深淬工具(create/edit_X)用 §1 的完整多行 Description(含 example + antipattern),不要压成一行 —— 它们的复杂度需要那些教学。其余一行式已够(sweep 88% + G6 guard 验证)。

