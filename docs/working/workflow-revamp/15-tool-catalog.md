---
id: WRK-001-16
type: working
status: active
owner: @weilin
created: 2026-05-20
reviewed: 2026-05-31
review-due: 2026-08-31
audience: [human, ai]
landed-into:
---
# 15 — LLM-Facing 全优化:现状 → 优化后

> **这是一篇逐项 before→after 的文档。** workflow-revamp 要给 deepseek-v4-flash 看的**每一个 LLM-facing 面**——工具调用描述 / 工具选择描述 / catalog 分组 / 系统 prompt / schema 形状 / 后端容错 / API 杠杆 / 产品形态——都按「**现状(revamp 草案 00-12 / 被测基线)→ 优化后(实测验证)→ 证据**」写一遍。
>
> - **现状**的工具签名据 [`10-ai-tool-inventory.md`](./10-ai-tool-inventory.md);case 据 [`04-case-node.md`](./04-case-node.md);被测的 91 条基线描述原文见 [`research-archive/baseline-tool-catalog.md`](./research-archive/baseline-tool-catalog.md)。
> - **优化后**的取舍优先级 + 产品认知见 [`13-llm-facing-implementation-guide.md`](./13-llm-facing-implementation-guide.md);**证据(实验 + 数字)**见 [`14-llm-validation-research-record.md`](./14-llm-validation-research-record.md)。
> - 这篇正好回答 doc 10 末尾那 5 个「待验证 best practice」问题(description 怎么写 / schema 设计 / 错误信息 actionable / chain 模式 / 高风险兜底)。
>
> ⚠️ 绝对判官分 run-to-run 抖 ±15pt(LLM-judge 宽严方差);下面凡 paired 对比(A→B)可信,跨实验绝对值仅供量级。被测:**deepseek-v4-flash**。

---

## 一句话

**revamp 这套设计配 deepseek-v4-flash 是 work 的;模型能力不是瓶颈。** 差距集中在两类:① **易错契约没 pin 在 schema/平台**(case、ops 形状、JSON 容错)② **复杂 workflow 的语义架构决策**(模型的结构已 95-100% 对,但"该 agent 还是 function、polling 还是 cron"首发只对一半)。前者靠 §A/§E/§F pin 死,后者靠 §D/§G 教学 + §H 迭代对话兜。

---

## §A 工具调用描述(改了的 forge / meta 工具)

> ~80 个读取/资产/生命周期工具的描述**现状即最终**(见 §B);下面只列**需要改**的。

### `create_workflow`
- **现状**(doc 10):`create_workflow(name, graph)` —— "造(初始 v1 auto-accept)";graph 里 case 节点用 `expression` + 分支名匹配(doc 04),node.config 无类型。
- **优化后**:① case 改 per-branch `when:` 布尔守卫(见 §E);② node.config 逐 type pin 形状(见 §E);③ 描述点明构图守则——*"cron/manual 触发不带业务数据 → 第一个节点必须先 fetch;case 用 when 守卫不用 add_edge;一次把完整图建全"*。
- **证据**:case 0-18%→~100%;复杂大图结构 95-100% 对,语义 52%(靠 §G/§H 补)。

### `edit_workflow`
- **现状**(doc 10):`edit_workflow(id, ops)` —— add_node/remove_node/connect/disconnect/update_config,ops 裸 `{}`。
- **优化后**:`update_config` 等 op 逐种 pin 形状(尤其 case/trigger);明确"引用已存在 node id"。
- **证据**:edit-ops 合规 46/30%→66/77%。

### `create_agent`(草案全新,照这个建)
- **现状**(doc 10):`create_agent(name, prompt, skill?, knowledge[], tools[], model?, outputSchema?)` —— 仅"造"一句,无教学。
- **优化后**:完整 Description——*"配置好的 LLM worker;挂 prompt / skill(0-1)/ knowledge(挂文档**引用**别粘正文)/ tools(**只 fn/hd/mcp,绝不平台工具、绝不另一个 agent**)/ outputSchema(enum|json_schema|free_text)/ model"*;`set_output_schema` pin `{kind, schema}`(§E);**描述里写死不可能能力禁令**(§D-3)。
- **证据**:不可能能力陷阱 17→95;set_output_schema 0→87;agent ARTIFACT 90%。

### `create_function`
- **现状**(doc 10):`create_function(name, kind, code, pollingInterval?)` —— kind 必填 `normal|polling`。
- **优化后**:加 **polling 教学**——*"kind=polling 的函数 `poll(last_cursor) → {events, next_cursor}`,只 emit 比 cursor 新的、cursor 前进、不重复、首次 last_cursor=None 要处理"*;**外部 IO 说明来源**(handler 注入 / 函数 config),否则模型硬编码占位。
- **证据**:复杂算法 code-exec 88%(58/60 clean);fp_status_poll 转换逻辑实正确。

### `edit_function` / `edit_handler` / `edit_agent`(update_code op)
- **现状**:ops 裸 `{}`,op 形状只在 prose。
- **优化后**:`update_code` op pin `{op:'update_code', code:'<新代码>'}`(键写死 `code` 非 `value`);`edit_agent` 的 `set_tools` 标注 **REPLACE 语义**(要保留得带上现有)。
- **证据**:46/30%→66/77%。

### `create_handler`(不大改)
- **现状**(doc 10):`create_handler(name, code, initSchema, methodsSchema)`。
- **优化后**:**保留** BARE-NAMES 教学(`__init__`/method 收裸命名参数,非 dict;init_schema/methods_schema 声明参数名)—— 已解决表面,不动。
- **证据**:复杂状态机(滑窗/令牌桶/连接池)code-exec 93%(56/60)。

### `capability_check_workflow`(草案已列,行为必须补全)
- **现状**(doc 10):`capability_check_workflow(id)` —— "预校验 callable 存在 + kind 匹配 + payload schema 流"(列了,但是 stub / 未真建)。
- **优化后**:**必须真查 ref** —— 遍历各节点引用的 `fn_/hd_/ag_` id,有引用却不存在就报 `{error, missing:[...], next_step:"建 X 或改 ref"}`;附结构 lint(悬空分支/空 payload/缺 fetch/不可达);**强制 create/edit 后、activate 前必跑,错误带 `next_step` 回喂让模型修**。
- **证据**:真查-ref 版端到端 23/24 接对(无反馈版接线常空);G8 恢复 17→88。

### `activate_tools`(不大改)
- **现状**(doc 10):`activate_tools` —— lazy 加载。
- **优化后**:**保留**这句(把过度激活 4 组压到 1 组)——*"激活**单个**当前任务需要的组;别投机激活多个,浪费 token"*;分组保持 domain-6(见 §C)。
- **证据**:过度激活 4→1;domain-6 62% vs 11-split 46%。

---

## §B 工具选择描述(模型从全集挑对工具)

- **现状**:91 全集,模型靠每个工具各自的 description 选;草案描述(doc 10 的一句话"解决问题")。
- **优化后**:**绝大多数(~80 个读取/资产/生命周期工具)现状即最终,一字不改** —— 实测全集选择 82%、多数 95-100%。**只补 3 类消歧 + 1 类欠选**:

| 项 | 现状 | 优化后 | 证据 |
|---|---|---|---|
| 分类/判断/抽取/路由 | 易选成 create_function | 描述/系统 prompt 点明 → `create_agent` | G5 消歧 |
| 知识库 | 易选成本地 `Read` | 点明 → `document`(create/read_document) | G5 消歧 |
| 实体类型消歧 | 靠描述 | 描述写清"何时用我 vs 兄弟工具" | 91 全集 82-91% |
| `Subagent` / `AskUserQuestion` **欠选** | 模型倾向自己干 / 用散文列取舍 | 若产品依赖 → 教学触发或后端兜 | under-selected |

> ~80 个"不改"工具的逐条基线描述见 [`research-archive/baseline-tool-catalog.md`](./research-archive/baseline-tool-catalog.md)(那就是它们的最终描述)。

---

## §C Catalog / Lazy 分组 / 能力菜单

- **现状**:lazy 6 组 = function/handler/workflow/mcp/document/skill(domain-6)+ resident ~28;曾设想"6 组不够,拆成 11 组(edit/use 细分)"。
- **优化后**:**保持 domain-6,别拆**;resident 28 + lazy 6;`search_*` 放 resident(当年"6 不行"的真变量其实是 search 工具位置不对,非分组粒度);**两个激活摩擦建议后端兜**:① 用户说"激活技能"→ 模型想直接 `activate_skill` 却够不着未激活的 skill 组;② 模型 search 完想直接 edit/run 够不着 lazy 工具。**最省心修法:模型调一个未激活组里的工具时,后端自动激活该组并执行(而非报错)。**
- **证据**:domain-6 激活对组 62% vs 11-split 46%(domain-6 **从不激活错组**)。

---

## §D 系统 prompt

- **现状**:revamp **没定最终系统 prompt**(草案散落)。
- **优化后**:段顺序固定,**最关键守则放最后一段**(deepseek 对 prompt 末尾遵守度最高,与 OpenAI 相反):

```
[identity]          你是 Forgify 的 AI 自动化工程师;能力只来自 forge 实体,无平台逃生舱
[how-to]            先设计后决定性调用(一次给完整参数);引用实体用 id;summary 必填
[tool_conventions]  summary/destructive/execution_group 三注入字段讲一次(省 ~13k token)
[capabilities]      运行时渲染:用户资产菜单 + 当前激活的工具组
[gold 示例 + 架构守则]  ↓ 见下,复杂建提分
[critical rules]    ↓ 殿后
```

**[critical rules] 6 条(各 before 失败 → after 规则):**

| # | 现状(无此规则时) | 优化后(加这条) | 证据 |
|---|---|---|---|
| 1 | worker 可能挂错工具 | agent/workflow 节点只挂 fn/hd/mcp;**agent 绝不调 agent**;绝不给 worker fs/shell/web/memory | 规则有效 |
| 2 | 分类塞 function、知识库塞文件 | "分类/判断/抽取/路由"=agent;"知识库"=document | G5 |
| 3 | 给 agent 写没有工具支撑的能力 | **不可能能力禁令**:要外部数据走 `{{payload.*}}` 或挂 forge fn | 17→**95** |
| 4 | 信息不全也乱建 / 或过度反问 | **可满足性检查(紧措辞)**:**仅当**需求自相矛盾才点明冲突;**信息不全按默认直接建,别多问** | 0→**85**,正常请求 100(⚠️宽措辞会 100→47,不可上线) |
| 5 | 改已有实体前反复重查甚至循环 | **commit-after-recon**:search/read 过一次就直接执行 | revert 35→**100** |
| 6 | cron 触发后直接用空 payload / case 乱连边 | **构图守则**:触发后先 fetch;case 用 when 守卫;重试 emit 自增且有界;终止节点省 `to`;一次建全 | — |

**复杂建额外两段(进 prompt,§G 详):** gold 示例(+11)+ 架构决策守则(+10)。

---

## §E Schema 形状 pin(G10 —— 把易错契约 pin 死,别靠模型猜)

- **现状**:forge 的 `ops` 都是 `items:{type:object}`(无类型);workflow `node.config` 无类型;op 形状只写在描述 prose。
- **优化后**:**逐 op / 逐 node-type 在 JSON schema 里写出 payload 形状 + 判别字段**。重灾区:

| 易错点 | 现状 | 优化后 | 证据 |
|---|---|---|---|
| agent `set_output_schema` | `value:{}` 无类型 | `{kind:'json_schema'\|'enum'\|'free_text', schema:<JSON Schema>}` | **0→87** |
| trigger cron 字段 | `config:{}` 无类型 | `{kind:'cron', cron:'<5段>'}`(字段名写死 `cron`)| 73% 放错字段(schedule/expression/cronExpr…6 种)→ **100** |
| edit `update_code` | ops 裸 `{}` | `{op:'update_code', code:'<代码>'}` | **46/30→66/77** |
| `node.config` 逐 type | 无类型 | trigger`{kind,spec}` · tool`{callable,args}` · agent`{agentRef}` · case`{branches:[{when,to}]}` · approval`{prompt,timeout?,timeoutBehavior?}`(端口 yes/no;**字段名 canon 见 [`17`](./17-execution-contract.md) §7**)| — |

> tool 节点 `{callable,args}` 模型本来就一致,不用改。**字段名 canon 统一在 [`17`](./17-execution-contract.md) §7(agent=`agentRef`、tool=`callable`、approval 端口=`yes`/`no`);schema 里唯一、不再各文档各写。**

### case 节点形态(头号脆弱点,单列)
- **现状**(doc 04):`config.expression`(CEL 求值出一个**值**)+ `branches` 按名(invoice/inquiry/_default),**分支名必须 == expression 的值**。
- **优化后**:每分支一个**布尔 CEL 守卫 `when:`**,首个为真胜出,最后一条 `when:"true"` = 兜底(取代 `_default`):
```yaml
type: case
config:
  branches:
    fast:   { when: "payload.vip || payload.amount >= 5000", to: ... }
    normal: { when: "true", to: ... }   # 兜底
```
- **为什么**:`expression==分支名`对**分类**场景能用(agent enum 输出 → `expression: payload.category`,~100%,这是 04 跑得通的场景);但**布尔路由就崩**(`expression: payload.attempt>5` 出 true/false,分支却叫 fast/normal/escalate → 映射写不对,**0-18%**)。改 per-branch `when:` 后分类+布尔统一 ~100%。
- **坑**:重试守卫写 `(has(payload.attempt)?payload.attempt:0) < 3`,**不要** `has(x)&&x<3`(首次 unset→false→永不重试,实测踩过)。
- **注**:草案 04 **已经**写了"case 是看牌发牌员不是分析师"(别在 CEL 里塞计算/调 API/多步推理)—— 这条对,保留。

---

## §F 后端容错机制(平台层兜模型易错点)

| 机制 | 现状 | 优化后 | 证据 |
|---|---|---|---|
| **JSON-repair**(G1)🔴 | Go `encoding/json` 直接解析 tool 参数 | 解析前跑 repair(容忍多行字面控制字符 + 括号配平)| ~4-8%(复杂 agent 嵌套 ~17%)畸形 → 恢复 **100%**;prompt 救不了 |
| **case fail-to-false**(G9)🟡 | CEL 访问缺失字段抛 no-such-key,中断 flowrun | guard 求值出错当 `false`(落下一分支,`when:"true"` 兜底)| 模型布尔逻辑 ~全对,但防御 `has()` 仅 ~50% 一致 |
| **capability_check 真查 ref**(G8)🔴 | 见 §A(草案 stub)| 真查 ref + 结构 lint + 错误带 `next_step` 回喂 | 23/24 接对;17→88 |
| **max_tokens**(G2)| 复杂锻造截断 | ≥ **16000** | 8k 截断 1/96 → 16k 0/320 |
| **thinking**(G3)| —— | 别全局关,按任务;多轮回传 `reasoning_content` | 复杂任务 on 更好 |
| **错误 envelope**(G7)| 裸 prose 错误 | 带 `next_step`(具体下一步)| 有 next_step 能自纠 3/3,裸 prose 乱试 |

---

## §G API-only 把复杂建首发推更高(Round-4 —— 不自部署/不微调)

- **现状**:期待一次 `create_workflow` 入魂;复杂大图首发语义判官 ~52%。
- **关键认知**:模型的**结构已 ~95-100% 对**(when 守卫/不悬空/终止分支/重试 emit);那 ~50% 差距**全在语义架构决策**(agent-vs-function、polling-vs-cron、case 别当分析师、多字段守卫、每路径有动作)。
- **优化后**:纯 API 手段,按 paired lift × ROI 叠加:

| 杠杆 | paired lift | 成本 | 评 |
|---|---|---|---|
| **gold 示例**(1 个完整正确 workflow 进 prompt)| **~+11** | ~免费 | 🥇 演示完整架构形态 > 说教 |
| **架构决策守则**(agent-vs-function / polling-vs-cron / case 别当分析师 / 每路径有动作 / 多字段守卫用 `&&`)| **+10** | ~免费 | 🥈 与 gold 示例互补 |
| **自一致性**(采 N 挑众数结构)| **+7** | N× 采样 | 众数=模型最有把握 |
| **reflexion 自审一轮**(对照需求查实体类型/悬空)| **+7** | 1 轮 | 抓部分语义错 |
| best-of-N(结构选择器)| +3 | N× | 结构已满分,选不出语义差 |
| 模型分层(deepseek-reasoner R1)| +3 | **10× 成本** | ❌ 不值;差距是 Forgify 约定非原始智能 |
| **strict:true 函数调用**(beta,服务端约束 args 匹配 schema)| 结构侧 | —— | = API 版约束解码;配 §F JSON-repair 兜其畸形-JSON bug |

- **叠加预期(未实测投影)**:gold 示例 + 架构守则进 prompt,复杂建时自一致性采样 + 自审 → **复杂建有望 85%+**(paired lift 线性叠加到 42-52% 实测基线的投影;自一致性 / 自审需 N× token);**实测地板**复杂建首发 42-52%(±15pt 方差)、一次性 0/24 → 是人在环迭代;剩下靠 §H 的 `:iterate` 对话兜。**别上更强模型。**

---

## §H 产品形态:迭代对话,不是一次入魂(最重要认知)

- **现状**:UI / 交互期待"一次生成完美 workflow"。
- **优化后**:做成 **建 → 用户审 → `:iterate` 改 → G8 兜结构错** 的迭代对话(正是 N5 的 `:iterate` 流)。模型给骨架 + 一半细节,迭代精修。
- **证据**:端到端 24 个复杂"从零建这套自动化"episode —— **一次性全对 = 0/24**,**每项设计决策对 ≈ 53%**(6-8 项连乘 → 趋近 0);但走得完整流程 + 接得对 id(真查-ref 23/24)。`:iterate` 改一次正确 67% / **96% 不破坏其余**;简单自动化(onboarding/daily-report)配真反馈 **4/4**。**复杂端首草稿需精修 = 产品形态,不是模型缺陷。**

---

## 落地清单(改哪 / 建什么)

| 面 | 落到哪 |
|---|---|
| §A 工具描述 / §B 选择消歧 | 各 create/edit 工具 `Description()`;`create_agent` 整个新建 |
| §C domain-6 + 自动激活兜底 | 保持 6 组;后端加"调未激活组工具自动激活" |
| §D 系统 prompt | `internal/app/chat` system prompt 组装 + tool_conventions 段 + gold 示例 + 架构守则 |
| §E schema pin / case→when | 各 forge `Parameters()`(逐 op/type typed)+ `04-case-node.md` 设计 + case 节点 schema/求值器 |
| §F JSON-repair / fail-to-false / capability_check / max_tokens | `internal/infra/llm` + case 求值器 + **新建** capability_check_workflow + accept/activate 路径 |
| §G API 杠杆 | system prompt(示例+守则)+ 复杂建采样/自审编排 + `strict:true` |
| §H 迭代产品形态 | `:iterate` 对话流(N5)+ 编辑器 UI 不期待一次完美 |

> **净结论**:revamp 设计 + deepseek-v4-flash work;§A-§F(契约 pin 在 schema/平台 + test/check 回喂)+ §G(便宜语义杠杆)+ §H(迭代产品形态)做了,就放心动工。
