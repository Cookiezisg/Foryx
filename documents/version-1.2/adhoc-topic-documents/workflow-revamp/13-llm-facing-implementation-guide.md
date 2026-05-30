# 13 — LLM-Facing 实施指南(workflow-revamp:照这个做)

> **这是一篇"你该做什么"的文档。** revamp 的 LLM-facing 层(给 deepseek-v4-flash 看的工具描述 / schema / 系统 prompt / 后端容错)按本文实现,不用再猜。
> 每条改动都标 **现状(revamp 草案,00-12 文档) → 改成(已实测验证) → 为什么**。
> 实测证据 + 全部实验在 [`14-research-record.md`](./14-research-record.md);原研究稿归档在 [`research-archive/`](./research-archive/)。
> 被测模型:**deepseek-v4-flash**(平台默认便宜模型)。

---

## §0 一句话 + 必做清单

**净结论:revamp 这套设计配 deepseek-v4-flash 是 work 的。** 但 LLM-facing 层动工前,**5 件 schema/后端的事必须做**,否则复杂场景会悄悄出错;再加 1 条产品形态认知。每件下面都有 before/after + 证据。

| # | 必做 | 一句话 | 优先级 |
|---|---|---|---|
| **A** | **case 节点改 per-branch `when:` 守卫** | 现 04 设计"expression 值==分支名"对布尔路由 0-18%;改每分支一个布尔 CEL → ~100% | 🔴 动土前(schema)|
| **B** | **forge 的 ops / node.config 在 schema 里 pin 形状** | 裸 `{}` / 无类型 → 模型猜字段名、丢判别字段;pin → 收敛 | 🔴 动土前(schema)|
| **C** | **后端解析 tool 参数前跑 JSON-repair** | deepseek ~4-8%(复杂 agent ~17%)吐畸形 JSON,Go 默认拒 | 🔴 动土前(后端)|
| **D** | **case 求值 fail-to-false** | guard 出错当 false 落兜底,免模型背 null-safety | 🟡 建议(后端)|
| **E** | **forge 后必跑 capability_check(真查 ref)+ 结构 lint + 错误回喂让模型修** | first-draft 有缺陷,靠"试→报错→修"回路兜 | 🔴 动土前(运行期)|
| **F** | **产品做成"建→审→`:iterate` 改"的迭代对话** | 复杂自动化无法一次建完美;模型给骨架+一半细节,迭代精修 | 🔴 产品形态认知 |

> 其中 A / B / D 是同一主题:**把 LLM 易错的契约 pin 在 schema/平台,别靠模型猜或靠教学补**。

---

## §1 五件机制改动(逐条 before/after + 为什么)

### A. case 节点:`expression+分支名匹配` → `per-branch when: 布尔守卫` 🔴

**现状(04-case-node.md 草案):**
```yaml
type: case
config:
  expression: <CEL>            # 求值出一个"值"
  branches:
    invoice: { to: ... }       # 分支名必须 == expression 的值
    inquiry: { to: ... }
    _default: { to: ... }
```

**改成(实测验证):**
```yaml
type: case
config:
  branches:                    # 每分支一个布尔 CEL 守卫,首个为真胜出
    fast:    { when: "payload.vip || payload.amount >= 5000", to: ... }
    normal:  { when: "true", to: ... }     # 最后一条 when:"true" = 兜底(取代 _default)
```

**为什么:** "expression 求值出的值,必须正好等于某个分支名"这个契约对 LLM **根本性脆弱**。
- **分类场景能用**(agent enum 输出 → `expression: payload.category` → 分支名 = enum 值):实测 ~100%。这是 04 设计跑得通的场景。
- **但布尔路由就崩**:`expression: payload.attempt > 5` 求值出 `true/false`,可分支却叫语义名(fast/normal/escalate)→ 模型写不对这个映射。**实测布尔条件 + 语义分支名 = 0-18%**。
- 改成**每分支一个布尔 CEL(when)**,分类和布尔**统一 ~100%**(模型对"每个分支写一个布尔条件"是强项)。实测在隔离 CEL、完整 create_workflow、edit_workflow、全新场景四处都验证(0-18% → ~100%)。
- 回边计数仍按 04 的 emit 机制:`emit: { attempt: "(has(payload.attempt) ? payload.attempt : 0) + 1" }`;守卫读 `(has(payload.attempt) ? payload.attempt : 0) < 3`(**不要** `has(x) && x<3` —— 首次 unset→false→永不重试,这是实测踩过的坑)。

### B. forge 的 ops / node.config 在 schema 里 pin 形状(G10)🔴

**现状:** forge 工具的 `ops` 都是 `items: {type: object}`(无类型),op 的形状写在描述 prose 里;workflow 的 `node.config` 也无类型。

**改成:** **逐 op / 逐 node-type 在 JSON schema 里写出 payload 形状 + 判别字段**,别只放在描述。尤其这几个易错点:

| 易错点 | 现状 | 改成 | 实测 |
|---|---|---|---|
| agent 的 `set_output_schema` | `value: {}` 无类型 | `{kind:'json_schema'\|'enum'\|'free_text', schema:<JSON Schema>}`(放 `schema` 键)| 0% → 87% 正确产出判别字段 |
| trigger 的 cron 字段 | `config: {}` 无类型 | `{kind:'cron', cron:'<5段表达式>'}`(字段名写死 `cron`)| 73% 模型放进 `schedule` 等错字段 → pin 后 100% |
| edit_* 的 `update_code` | ops 裸 `{}` | `{op:'update_code', code:'<新代码>'}`(键写死 `code` 非 value)| 46/30% → 66/77% |

**为什么:** 无类型的 payload 让模型**猜内层形状、丢判别字段、字段名乱起**(cron 串见过 `cron`/`schedule`/`expression`/`cronExpr`… 6 种写法)。把形状写进 schema(或至少描述里 spell 死字段名),模型就收敛。**后端字段名定死哪个都行(cron 还是 schedule),关键是 schema 里唯一。** 注:把形状写进描述 prose 已能到 ~87%(现状 forge 描述其实写得挺详细);**写进 schema typed 能更稳**。tool 节点的 `{ref, args}` 模型本来就一致(无需改),trigger / output_schema / edit-ops 是重灾区。

### C. 后端解析 tool 参数前跑 JSON-repair(G1)🔴

**现状:** 后端用 Go `encoding/json` 直接解析模型吐的 tool 参数。

**改成:** `infra/llm` 解析前先跑一遍 repair——**容忍多行字符串里的字面控制字符 + 括号配平**。

**为什么:** deepseek-v4-flash 在**复杂/多行 tool 参数**里 **~4-8%**(复杂 agent 嵌套 outputSchema 高达 **~17%**)产出严格解析器会拒的 JSON:① 多行 prompt 里塞未转义换行;② 深嵌套 ops 漏一个 `}`。Go 默认全拒 → 复杂 forge 调用直接失败。实测 `json_repair` 类括号配平**恢复 100%**。**这是解析层硬需求,prompt 救不了。**(第三类:code 里 docstring 过度转义成 JSON 合法但 Python 非法——这类靠 run_function 试跑抓,见 E。)

### D. case 求值 fail-to-false(G9)🟡

**现状:** case CEL 求值若访问缺失字段(标准 CEL 抛 no-such-key),会中断 flowrun。

**改成:** **求值器把"某分支 guard 求值出错"当 `false`**(落到下一分支,最终 `when:"true"` 兜底),不中断。cel-go 一行:`out,_,err := prg.Eval(act); match := err==nil && out==types.True`。

**为什么:** 实测模型写 when 守卫**布尔逻辑 ~全对,但防御性 `has()` 只 ~50% 一致**。平台 fail-to-false 后,省略 `has()` 的守卫也安全 → null-safety 不再是模型的负担(教学降为双保险)。配合 E 的 capability_check 用样例 payload 干跑各分支,避免逻辑错被静默掩盖。

### E. forge 后必跑 capability_check(真查 ref)+ 结构 lint + 错误回喂(G8)🔴

**现状:** capability_check_workflow 在 revamp 里**还没建**(后端无此工具)。

**改成:** 建 capability_check,且必须**真查 ref**:遍历 workflow 各节点引用的 `fn_/hd_/ag_` id,**有引用了但不存在的就报缺失** `{error, missing:[...], next_step:"建 X 或改 ref"}`;同时结构 lint(查悬空分支、空 payload、缺 fetch 步、不可达节点)。**强制:create/edit workflow 后、activate 前必跑;错误以带 `next_step` 的 envelope 回喂,让模型修。** 同理 function/handler 用 run_function/call_handler 试跑抓代码缺陷。

**为什么:** forge first-draft 有 ~15-45% 缺陷,**都由"建→试跑/校验→错误回喂→模型修"恢复**(实测令牌桶 17%→88%、capability_check 恢复 3/3)。**关键:capability_check 必须真查 ref 并报缺失——若永远返回 ok,模型不知道接错了、没法修**(端到端实测:无反馈版 workflow 接线常空;真查 ref 版 23/24 接对)。错误 envelope 必须带 `next_step`(具体下一步)——实测有 next_step 模型能自纠,裸 prose 错误则乱试。

---

## §2 系统 prompt(完整组装 + 教学守则)

**现状:** revamp 没定最终系统 prompt。

**改成:** 段顺序固定,**最关键守则放最后一段**(deepseek 对 prompt 末尾遵守度最高,与 OpenAI 相反)。骨架:
```
[identity]            你是 Forgify 的 AI 自动化工程师;能力只来自 forge 实体,无平台逃生舱
[how-to]              先设计后决定性调用(一次给完整参数);引用实体用 id;summary 必填
[tool_conventions]    summary/destructive/execution_group 三注入字段讲一次(省 ~13k token)
[capabilities]        运行时渲染:用户的资产菜单 + 当前激活的工具组
[critical rules — 殿后] ↓ 这些放最后
```

**[critical rules] 段(每条已实测验证):**
1. **worker 工具限制**:agent / workflow 节点只能挂 fn/hd/mcp 当工具;**agent 绝不调 agent**;绝不给 worker fs/shell/web/memory。
2. **选型消歧**:"分类/判断/抽取/路由" = agent(create_agent),不是 function;"知识库" = document,不是本地文件。
3. **不可能能力禁令**(实测把陷阱 17%→95%):绝不给 agent 写"它没有工具支撑的能力"的 prompt;要外部数据走 `{{payload.*}}` 或挂 forge fn。
4. **可满足性检查(紧条件措辞 — 实测必读)**:**仅当**需求自相矛盾、逻辑无法同时满足时(如"全自动无人值守"且"每笔人工审批"),才点明冲突+提一个折衷请用户确认;**信息不全(缺邮箱/数据源)不算矛盾,按默认直接建,别多问**。⚠️ 宽措辞("建造前先检查")会把正常请求也过度反问(daily_report 建成率 100%→47%),**务必用这条紧措辞**(矛盾识别 ~85% + 正常请求 100% 不误伤)。
5. **commit-after-recon**:search/read 过目标实体一次后,**直接执行**请求的操作(edit/run/delete),别反复重查(实测模型对"改已有实体"recon 过度甚至循环;这句把 revert 35%→100%)。
6. **构图守则**:cron/manual 触发不带业务数据 → 触发后第一个节点必须先 fetch;case 用 when 守卫不用 add_edge;重试回边 emit 自增计数且有界;终止节点省略 `to`;一次 create_workflow 把完整图建全。

**为什么:** 上面每条都对应一类实测失败(见 doc 14)。整段"教学有效"是可复现的(不可能能力 17→95、lazy 过度激活 4→1、矛盾识别 0→85)。

---

## §3 Forge 工具描述 before/after(关键工具)

> 大多数读取/检索工具(get/search/list/read,~60 个)描述**现状即可用**(实测工具选择 82%,多数 95-100%),不用改。下面是需要特定教学的 forge + 元工具。

### create_agent(revamp 还没建 — 照这个建)
- **要点**:agent = 配置好的 LLM worker;挂 prompt / skill(0-1)/ knowledge(挂文档**引用**别粘正文)/ tools(**只 fn/hd/mcp,绝不平台工具/另一个 agent**)/ outputSchema(`enum|json_schema|free_text`)/ model。
- **schema 必须 pin**:`set_output_schema.value = {kind, schema}`(见 §1-B);`set_knowledge.value = [doc 引用]`;`set_tools.value = [callable refs]`。
- **描述里写死不可能能力禁令**(§2-3)。outputSchema=enum 的 agent 天然喂给 case 节点(意图识别)。

### create_function(含 polling)
- **现状描述已不错**(set_meta/set_code/set_parameters)。**加 polling 教学**:`kind=polling` 的函数 `poll(last_cursor) → {events, next_cursor}`,**只 emit 比 cursor 新的、cursor 前进、不重复、首次 last_cursor=None 要处理**。
- **外部 IO**(调真实 API/密钥)说明从哪来(handler 注入 / 函数 config),否则模型硬编码占位(实测发现)。

### create_handler
- **现状描述已很好**(BARE-NAMES 契约:`__init__`/method 收裸命名参数非 dict;init_schema/methods_schema 声明参数名)。实测 handler 是已解决表面(复杂状态机 code-exec 93%)。**不用大改,保留 bare-names 教学。**

### create_workflow / edit_workflow
- **case 改 when:**(§1-A);**node.config 逐 type pin 形状**(§1-B):trigger `{kind,cron}`、tool `{ref,args}`、agent `{ref}`、case `{branches:{when,to}}`、approval `{prompt,branches,timeoutSeconds?,onTimeout?}`。
- 构图守则进系统 prompt(§2-6),不必塞满工具描述(实测中等教学 > 全套教学)。

### activate_tools(lazy 分组 — 保持 domain-6)
- **现状描述够用**,保留这句(实测把过度激活 4 组→1 组):*"激活**单个**当前任务需要的组——想清楚是哪个(发 Slack→mcp);别投机激活多个组,浪费 token。"*
- **🆕 实测确认:domain-6 分组(function/handler/workflow/mcp/document/skill)优于 11-组细分**(激活对组 62% vs 46%,domain-6 从不激活错组)——**保持现行 domain-6,别拆 edit/use**。
- **🆕 两个激活摩擦(建议后端兜)**:① skill 命名撞车(用户说"激活技能"→ 模型想直接 `activate_skill` 却够不着未激活的 skill 组);② 模型 search 完想直接 edit/run 够不着 lazy 工具。**最省心修法:模型调一个还没激活的组里的工具时,后端自动激活该组并执行**(而非报错)。

---

## §4 产品形态:迭代对话,不是一次入魂(最重要认知)

**实测端到端 24 个"从零帮我建这套自动化"复杂 episode:一次性全对 = 0/24,每项设计决策对 ≈ 53%。**

- 模型**走得完整个流程**(search→forge→accept→接线→check→activate)、**接得对 id**(配 E 的真查-ref capability_check 时 23/24 接对),但**架构选择**(该 agent 还是 function、路由怎么分、polling 还是 cron、多字段守卫)首发只对一半,**6-8 项设计全中就接近 0**(连乘)。
- **→ 复杂自动化无法 one-shot。产品必须做成"建 → 用户审 → `:iterate` 改 → G8 兜结构错"的迭代对话**(正好是 N5 的 `:iterate` 流)。模型给骨架 + 一半细节,剩下靠迭代精修。
- **简单自动化高得多**(实测 composite onboarding/daily-report 配真反馈 4/4)。**复杂端 = 首草稿需精修,这是产品形态,不是模型缺陷。** 别把 UI 设计成"期待一次生成完美 workflow"。

---

## §4.5 API-only 把复杂建首发推更高(Round-4 实测,不自部署/不微调)

复杂建首发的差距**全在语义架构决策**(模型的结构已 ~95-100% 对)。纯 API 手段,按性价比叠加:

- **🥇 系统 prompt 里放 1 个 gold 示例**(展示 case+when+retry+approval+cron-fetch 的完整正确 workflow)→ 实测 **~+11pt**,最便宜。**演示 > 说教。**
- **🥈 加"架构决策守则"教学**(agent-vs-function / polling-vs-cron / case 别当分析师 / 每路径有动作 / 多字段守卫用 && 组合)→ **+10pt**,与 gold 示例互补。
- **🥉 复杂建时采 N 个候选挑"众数结构"(自一致性)**→ **+7pt**;或 **forge 后加一轮自审**(对照需求查实体类型/悬空/capability_check)→ **+7pt**。
- **❌ 别上更强模型**:deepseek-reasoner(R1)只 +3pt 却 **10× 成本**——差距是 Forgify 约定不是原始智能,强通用模型不更懂 Forgify。**模型分层对领域 forge 不划算。**
- **结构侧**:DeepSeek API 有 `strict:true` 函数调用(beta 端点,服务端约束 args 匹配 schema)= API 版约束解码,配 §1-C 的 JSON-repair 兜它的畸形-JSON bug。
- **叠加预期**:gold 示例 + 架构守则进 prompt,复杂建时自一致性采样 + 自审 → 复杂建可上 85%+;剩下靠 §4 的 `:iterate` 对话兜(实测改一次正确 67%、96% 不破坏其余)。

> ⚠️ 别被单次判官分误导:绝对正确率 run-to-run 抖 ±15pt(LLM-judge 宽严方差);上面是实验内部 paired lift。

---

## §5 实施清单(改哪个文件 / 建什么)

| 改动 | 落到哪 |
|---|---|
| A case → when: | 改 `04-case-node.md` 设计 + case 节点 schema/求值器 |
| B ops/node pin 形状 | 各 forge 工具的 `Parameters()`(逐 op/type typed)|
| C JSON-repair | `internal/infra/llm` 解析 tool 参数处 |
| D case fail-to-false | case 节点求值器(cel-go Eval err→false)|
| E capability_check 真查 ref + lint + 回喂 | **新建** capability_check_workflow + workflow accept/activate 路径 |
| §2 系统 prompt | `internal/app/chat` system prompt 组装 + tool_conventions 段 |
| §3 forge 描述 | 各 create/edit 工具的 `Description()`;**create_agent 整个新建** |
| §3 activate_tools / domain-6 | 保持现行 6 组;后端加"调未激活组工具自动激活"|
| F 迭代产品形态 | `:iterate` 对话流(N5)+ 编辑器 UI 不期待一次完美 |

> **净结论重申**:revamp 设计 + deepseek-v4-flash work;A-E 五件(契约 pin 在 schema/平台 + test/check 回喂)+ F 产品形态认知做了,就放心动工。
