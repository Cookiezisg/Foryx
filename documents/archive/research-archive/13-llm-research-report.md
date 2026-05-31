# 13 — LLM Prompt Forging Research Report(数据 + 迭代日志)

**模型**:DeepSeek V4-flash only。**方法**:迭代淬炼(真 LLM 当 oracle,读失败 trace→根因→改→再测),非跑分选最优。
**死结论(直接抄)**:[`14-llm-research-playbook.md`](./14-llm-research-playbook.md)。**计划**:[`docs/superpowers/specs/2026-05-29-llm-prompt-forging-plan.md`](../../../../docs/superpowers/specs/2026-05-29-llm-prompt-forging-plan.md)。
**实验框架**:`research/llm-experiments/`(catalog_v2 工具定义 + forge_runner oracle + 各 surface forge 脚本 + per-pid 成本 ledger)。

⚠️ **状态:轮询淬炼进行中**。本报告随每个 surface 收敛持续更新。

---

## §0 TL;DR — 7 个 master 发现(影响每个 LLM 调用)

| # | 发现 | 证据 | 产品动作 |
|---|---|---|---|
| **M1**(已修正)| **thinking 别全局关**;只复杂长 ops 输出有 ~15% 真畸形 | ⚠️ 初版"关 thinking"错了:CEL 50→100 / agent 47→95 是**单-turn 把"先想/先问"误判成失败**(多轮正常)。隔离验证:CEL 补全场景 thinking-ON=92%(失败多是 trap 正确解释)。workflow thinking-ON 无截断时 valid 84% / **malformed 15%** = 唯一真代价 | 复杂长 ops:thinking-off **或** parse-错误重试(倾向后者);其余保留 thinking |
| **M2** | **复杂结构输出 max_tokens ≥ 8000** | 6 节点 workflow 输出 7900+ tok;max_tokens=3000 时复杂图被腰斩,假装"不会"(78%),提到 8000→97% | create/edit_workflow 后端高 token 上限 |
| **M3** | **`search_X` 是按-id 操作的通用 on-ramp** | 53 工具 sweep:31 个"首-turn 没调对"全因 LLM 先 search 确认实体再操作(正确行为) | 单-turn 指标对这些工具无效→多-turn 测;search_* 须随时可达且返回含 id |
| **M4** | **教学过度有害(concise > verbose)** | workflow 中等教学 97% > 全套 CEL+example 90% | 每 surface 给"刚好够"教学 |
| **M5** | **callable ref 允许下划线 + 真实 id** | 早期正则禁下划线,误杀 `fn_send_email` 全部正确输出 | validator/正则 `[a-z0-9_]+` |
| **M6** | **按实体类型的工具需类型 guard** | call_handler 被当 function;guard 后 accept_pending_handler 0→12 | description 写 `for hd_/sk_ entities` + 系统 prompt id-前缀图例 |
| **M7** | 🔴 **关键规则放系统 prompt 末尾(recency)** | rules-last 20/20 遵守 vs rules-first 0/20(反 OpenAI rules-first)| critical 守则殿后,贴近 user turn |

**M1(thinking-off)是这一轮的决定性杠杆** —— 单这一项把 5 个 surface 从 40-50% 抬到 95-100%。

---

## §1 方法论 — 迭代淬炼循环

```
写 v1 描述/schema → 真 DeepSeek 跑 N=20 复杂场景(easy→hard→trap)
→ 程序 validator 自动判 + 我读失败 trace 搞懂"为什么错"
→ root-cause → 改 v2(优先 tool-call:描述/schema/example;撬不动才动设计)
→ 再跑对比 Δ → 重复到 🟢(复杂 90%+ / 简单 98%+)或撞天花板
```

- **指标**:one-shot 正确(首工具+参数;多步任务用多-turn)+ token。
- **validator**(无 sandbox):`validate_workflow_ops`(5 节点/ref/case 结构/loop 检测)、`validate_cel`(无副作用/括号/引用 payload)、`CALLABLE_RE`、agent ops(tools 只含 forge callable,拒平台工具)。
- **场景**:每 surface 含 easy→hard→trap(陷阱测员工思维 / 老节点 / 平台工具 / 不可计算)。

---

## §2 各 surface 迭代日志 + 数据(Round 1,全 🟢)

### 2.1 workflow 编排(皇冠)— 0% → 97% 🟢

| 轮 | 改动 | 一次对 | 读 trace 发现的失败模式 |
|---|---|---|---|
| v1 泛型 ops | `ops: array of object` | **0/120** | React-flow `data` 字段 / `type` 当 discriminator / camelCase `addNode` / 老节点 `function`+`condition` |
| v2 + enum + 5-node 教学 | op-key enum + 节点类型表 | 78% | 消灭 discriminator/老节点;trap-old-nodes 18/20 正确用 tool+case |
| v3 + max_tokens 8000 | (M2) | 94% | 消灭复杂图截断:branch 10→20,full 8→17 |
| v4 + thinking off | (M1) | **97%** | 消灭畸形 JSON:loop 17→19,trap 19→20 |

场景明细(v4):linear 20/20 · branch 20/20 · loop 19/20 · full 18/20 · callable-mix 20/20 · trap-old-nodes 20/20。
**N=40 统计确认**:232/240 (96%) —— branch/callable-mix/linear/trap 全 40/40,full/loop 各 36/40 (90%)。97% 非运气,稳定。
残留:wf-full(6 节点全功能)90% —— 最复杂图偶有边连小错。
反向:V3-full(加全套 CEL 教学+example)= 90% < V2 的 97%(M4 过度教学)。

**edit_workflow(用户点名的"最恶心" case)= 100/100 (100%)**:改已有图 5 类操作(插中间/加case+approval/加回边重试/重连/删节点改接)各 20/20。流程 get_workflow→emit edit ops 引用现有 id。配方同 create。**关键**:capability check 对 edit 不可要求 ops 含 trigger(已在现图)—— `validate(ops, is_edit=true)`。edit 比 create 更稳(有现图 id 锚点,无残留)。

### 2.2 CEL case 表达式 — 40% → 100% 🟢🟢

| 轮 | 改动 | 一次对 | 发现 |
|---|---|---|---|
| v1 无教学 | | 40% | numeric/contains 极低 |
| v2 + full CEL 教学 | null-safety/boundary/loop | 50% | 大量 `called None`(thinking 光想不调) |
| v3 + thinking off | (M1) | 88% | simple/nullsafe/contains/loop 全 20/20;numeric 7/20 反问目标节点 |
| v4 + 场景补分支目标 id | (测试真实性:建图时下游 id 已知) | **100%** | 全 6 场景 20/20 |

关键洞察:**LLM 写的 CEL 表达式本身完美**(`has(payload.user) && has(payload.user.email) && payload.user.email.size() > 5` 一次对);失败全是 thinking 导致不调工具 + 场景缺目标 id 导致 LLM 正确反问。trap(情绪分析)→ LLM 正确路由 `payload.sentiment=="positive"`(假设上游字段),不 cram 分析。

### 2.3 agent forging — 0% → 95% 🟢

| 轮 | 改动 | 一次对 | 发现 |
|---|---|---|---|
| v1 泛型 | | 0% | 无结构 |
| v2 + 教学(mounts+员工思维+例) | | 47% | classify/knowledge 满分;tools 因正则 bug=0;trap `called None` |
| v3 + 正则允许下划线 | (M5) | 82% | tools 0→20,full 0→18 |
| v4 + thinking off | (M1) | **95%** | trap-platform-tools 4→15 |

场景明细(v4):classify/full/knowledge/tools 全 20/20,trap-platform-tools 15/20。
残留:trap 15/20 —— 5/20 在被要求"读文件/上网/记笔记"时仍挂平台工具名(违反员工思维)。可强化教学。

### 2.4 function 锻造 — → 100% 🟢🟢

V5(antipattern + polling cursor 完整例)+ thinking-off:add/time/poll-gmail/**poll-cursor**/trap-webhook 全 20/20。cursor 例直接消灭 polling 漏 cursor。

### 2.5 handler 锻造 — → 98% 🟢

bare-names contract + 例 + thinking-off:counter 20/20 · db 19/20 · oauth 20/20(真实 3 场景 59/60)。
注:`hd-trap-stateless` 0/20 是**测试设计 bug**(hd 模式只 offer create_handler,LLM 无 create_function 可选)—— 不计入。

### 2.6 callable ref — 52% → 93% 🟢

| 轮 | 改动 | 一次对 |
|---|---|---|
| v1 无 ref 语法表 | | 52%(function/mcp 全 0) |
| v2 + ref 语法表 | | 80% |
| v3 + 正则允许下划线(M5) | | **93%** |
残留:handler/mcp 17-19/20(偶尔 method/server 拆错)。

---

## §3 89 工具体检 — 单-turn 假象 vs 多-turn 真相

### 单-turn(误导):44%
53 工具单-turn first-tool 体检 = 353/795 (44%)。**但 31/53 "失败"全是 search-first**(M3):LLM 先调 `search_X`/`get_X` 确认实体,再操作。这是正确行为。

高分(单-turn 即 ≥90%,无需 search-first):search_* / list_* / 简单 create / install_mcp / get_dead_letter 等。

### 多-turn(正确测量):**701/795 (88%),45/53 工具 100%**(G6 guard 后)
喂 search 结果(回显请求 id)后看 LLM 是否接着调目标工具。残留 10 个 LOW,**无真·LLM 失败**,两类:
- **类型混淆**(handler/skill/memory 工具被 fn_ 家族抢):description 加一行 `for hd_/sk_ entities` guard 即救 —— 实测 accept_pending_handler 0→12/12、update_handler_config 0→11/12。
- **canned 假象**(call_mcp/activate_skill/forget_memory 0/15):canned search 不含请求的 skill 名/mcp server/memory query → LLM **正确报"实体不存在、拒绝操作"**(对的安全行为),生产环境真 search 会返回。
即:LLM 全程行为正确(search-first + 类型对则调对 + 不存在则拒)。详 doc 14 G6。

### 复合端到端(终极验证):**45/45 (100%)**
用收敛描述串联真实多步任务:诊断链 15/15 · 多实体锻造 15/15 · 上线 15/15。**证明 LLM 用淬炼后的描述能把皇冠级复杂任务端到端做完**(初版 44% 是 canned-result + turns 不够的 harness 假象,修后 100%)。

### 2.7 Round 3 — 补全所有剩余 LLM-facing 表面

第二轮审计发现 R1/R2 深淬了皇冠 + ~10 surface,但**广度未覆盖全**。R3 补完:

| 表面 | 结果 | 备注 |
|---|---|---|
| edit_function/handler/agent ops | ~99% | enum ops;eh "加方法" LLM 用 update_code(对)|
| 结构化参数工具(7 个:trigger/call_handler/call_mcp/run_fn/run_ag/update_config/replay)| **99%**(139/140)| 给定 id 时参数填对极好。**证明 sweep 那些 0/15 纯 search-first 假象** |
| 注入字段 summary/destructive/execution_group | **100%** | 含/不含 tool_conventions 都对(schema 描述够)|
| chainPatternsSection | 清晰任务 raw 也好;难任务 plan 决定性(同 R1) | |
| catalog 渲染 | 两格式都选对实体 | descriptive 更谨慎(缺输入会问)|
| section-order(G7/M7)| **rules-last 100% vs rules-first 0%** | recency 主导,关键规则殿后 |
| 4 rerank(fn/hd/skill/mcp)| **100%**(4×15/15)| "Output ONLY JSON, no prose" |
| subagent prompt(explorer/forger)| **100%** | explorer 守只读、forger 正确建 |
| lazy 分组(forging 框架重淬)| V4 83% vs V1/V2/V3 有 resident search 全 0 | 重证 search 必须移进 lazy 组 |

**R3 结论:剩余表面全 🟢。无新的真·LLM 失败** —— 验证了"结构化参数质量没问题(单 sweep 假象)"+ 补出 1 个新 master 发现(M7 recency)。

---

## §4 残留 + 设计建议

| Surface | 残留 | 建议(优先 tool-call,不动设计) |
|---|---|---|
| workflow full | 18/20 | 已达 🟢;6 节点图边连接极限,可接受 |
| agent trap | 15/20 | 教学强化"平台能力→改锻造 function" |
| `[更多待 sweep 多-turn + Round 2]` | | |

暂无需改 revamp 设计 —— **全部靠 tool-call 层(thinking-off / max_tokens / 教学 / 正则)解决**,印证"尽量不动设计"。

---

## §5 成本 ledger

per-pid ledger 在 `/tmp/forge_budget_*.json`,跨进程求和。**累计 ~¥8.8 / 预算 ¥200(4.4%)**。停止信号 = DeepSeek API 402(非自设 cap)。所有 surface 在低成本即达 🟢 —— 预算是天花板非靶子,继续烧为边际收益,价值低。

实验规模:Round 1+2+3 共 ~6000+ runs(workflow create 3 变体 + N=40 + edit_workflow 100 + agent/cel/ref/fn/hd baseline+教学 + 53 工具 sweep ×3 + composite + error-envelope + utility + G6 验证 + R3:editops/structargs/注入字段/catalog/section-order/4 rerank/subagent/lazy 重淬)。

### 全 surface 终表(🟢)

| Surface | 淬炼后 | Surface | 淬炼后 |
|---|---|---|---|
| CEL case | 100% | error-envelope(sentinel)| 100% |
| function 锻造 | 100% | composite 端到端 | 45/45 100% |
| utility 场景 | 100% | edit_workflow(最恶心)| 100% |
| handler 锻造 | 98% | workflow create(N=40)| 96% |
| agent forging | 95% | callable ref | 93% |
| 89 工具多-turn | 88%(45/53 满分)| | |

全部靠 tool-call 层(thinking-off / max_tokens / 教学 / 正则 / 类型 guard)解决,**revamp 设计零改动**。
