# 14 — LLM-Facing 设计规格(workflow-revamp 最终交付物)

> **这是发给 deepseek-v4-flash 的每一句话的最终规格。** 工程师照此实现,不需再猜任何 prompt / 描述 / schema。
> 每条都标:**最终文本 + 为什么 + 实测语义正确率 + 验证模式**。验证证据见 [`13-validation-report.md`](./13-validation-report.md)。
>
> 验证方法(真·端到端):真组装 prompt + 真 ReAct(Claude 当后端/用户)+ code 真执行 + 3-judge 对抗式语义判 + 多数表决。被测模型 deepseek-v4-flash(thinking on;R1 temp 0,**R2 temp=默认 = 生产真实温度**)。
> 状态:2026-05-30 ~¥19/¥204 预算;Round-1 waves 1-7 + **Round-2 大样本复验(n=50-100,temp=默认,95%CI:robustness 20场景 + 复杂9 + 多轮3 + 新维度8)** 已完成。

---

## §0 全局 LLM 设计原则(governing rules,每条已实测验证)

按重要性。这些**凌驾于**单个工具描述之上。

### G0 总命题(已证)
改 tool 描述 / 教学 prompt **能显著、可复现地推高** deepseek-v4-flash 的语义正确率。实测:create_agent 的"不可能能力"陷阱 **17% → 95%**(同一场景,只改教学);lazy over-activation **4 次激活 → 1 次**(只改 activate_tools 描述);**矛盾需求识别 0% → ~85%**(殿后"可满足性检查"规则)。**淬炼有效,不是玄学——但也有边界**:矛盾规则的**宽措辞**虽 0→100% flag,却 OVER-flag 正常请求(daily_report 100→47%);**收窄成条件措辞**(仅真矛盾才 flag、信息不全不算)才两全:矛盾 ~85% + 正常 100%(见 §1 + §0.1b)。**→ 教学改动必须回归测试副作用(假信心教训)。**

### G1 📌 工具参数解析:后端必须容错(生产级硬需求)
deepseek-v4-flash 在**复杂/多行 tool-call 参数**里 ~**4-8%** 产出严格解析器会拒的输出,三子类:
- **(a) 字面控制字符**:多行 prompt/markdown 里塞未转义换行 → `strict` JSON 拒。
- **(b) brace-undercount**:深嵌套长 ops 数组里**漏一个 `}`**(approval 节点该 5 个 `}` 只吐 4 个),输出完整非截断但结构坏。
- **(c) 过度转义引号(code 参数)**:code 里 docstring 写成 `\"\"\"`(JSON 里 `\\"` 而非 `\"`)→ **JSON 合法但 Python 非法**(~25% 含 docstring 的 code)。
- **Go `encoding/json` 默认拒 (a)(b)** → 复杂 create_workflow/create_agent ~4-8% 调用直接失败。**R3 复杂 agent(嵌套 json_schema outputSchema)规模重现:60 个难场景里 malformed JSON 21×(of-fails 35%)→ 复杂 agent 受 G1 影响比基线更重(~17%)。后端 JSON-repair 是硬刚需,再确认。**
- **规格要求**:
  - (a)(b):transport/llm 层解析前**必跑 repair**——容忍控制字符 + 括号配平(实测 `json_repair` 类配平**恢复 100%**)。**解析层硬需求,prompt 救不了。**
  - (c):JSON-repair 救不了(JSON 合法),由 **run_function/call_handler 试跑**抓 SyntaxError(见 G8 统一原则);+ 教学"code 里别双重转义引号"。

### G2 max_tokens:复杂锻造 ≥ 16000
thinking 吃 completion 预算 → 复杂 workflow/handler 在 8000 截断(finish_reason=length)。实测 8k→16k 把 wave-1 截断率从 1/96 降到 0/320。**规格:forge 类调用(create/edit workflow·handler·agent)max_tokens=16000;简单/utility 4000 够。**

### G3 thinking:别全局关,按任务
- thinking ON:复杂锻造(workflow/handler/agent)、诊断推理、CEL — **保留**(质量明显更好,reasoning 帮模型自检)。
- thinking OFF:纯结构化短输出(rerank / env-fix JSON / auto-title / 路由分类)— 省 output 成本、不掉质量。
- **多轮必须回传 `reasoning_content`**(thinking on 时),否则 DeepSeek 下一轮 400。
- 规格:per-scenario 配置;forge/diagnose 默认 on,utility-JSON 默认 off。

### G4 search-first 是模型默认且正确
全 91 工具集下,模型面对"对 X 做 Y"**默认先 search/get 找实体再动手**(实测 wave-3:绝大多数"未直接调终端工具"其实是合理的 search-first)。**规格**:工具描述明确"改/删/上线前先 search/get 拿真实 id";不要在评测或 UX 上惩罚这个行为。

### G5 实体类型消歧靠描述(基本到位,2 处需补)
91 全集下家族路由 ~91% reasonable:workflow 任务→search_workflows、知识库 doc→search_documents(**不是本地 Read**)、本地文件→Read。两处需在 system prompt + 描述补强:
- **"分类器/classifier/判断类" → create_agent,不是 create_function**(LLM 判断=agent;确定性逻辑=function)。
- **"知识库/knowledge base" → create_document**,不是本地文件工具。

### G6 关键守则殿后(recency,与 OpenAI 相反)
deepseek-v4-flash 对 prompt **末尾**的约束遵守度最高。系统 prompt 的**最关键守则放最后一段**(员工思维禁令、agent.tools 限制、case 路由规则)。

### G7 错误信息必须 actionable(envelope + next_step)
模型收到带 `next_step` 的结构化错误能**正确自纠**(实测 recover_capability_check 3/3:建缺失 fn 再 recheck);收到裸 prose 错误则乱试。**规格**:所有到达 LLM 的工具错误用 `{error:{code, message, next_step}}`,next_step 写"下一步具体怎么做"。

### G8 复杂 workflow:case 路由 3 根因(可教)+ create→check→fix(兜底)
create_workflow 难度梯度(wave-7,n=12):**不是节点数,是 3 个具名根因**——加教学修复后 BEFORE→AFTER 大幅抬升(全梯度均值 **58%→90%**):
| | g1 线性 | g2 单case | g3 双case | g4 循环 | g5 approval | g6 复杂 |
|---|---|---|---|---|---|---|
| 修前 | 100% | 33% | 25% | 67% | 33% | 83% |
| **修后** | 100% | **67%** | **92%** | 83% | **100%** | **100%** |

3 个根因(均已教学根治,实测):
1. **case 表达式↔分支键不匹配(#1 高频)**:布尔表达式配字符串键 → 永不匹配(详 §5)。→ 教学后 g2/g3 大涨。
2. **终止分支留 `to:null`/悬空**:应省略 `to` 或指明确 end 节点。→ g3 25→92%。
3. **approval timeout 配置漏**。→ g5 33→100%。

⚠️ **诚实(教学的能与不能)**:上面梯度是**专门围绕 case-contract 的场景**。在**原始多样场景**上,完整强化教学(case-contract + 终止 + fetch)把它们拉到 **clear_triage 58% · branch_signup 92% · retry 82% · vague 75%**(均值 ~77%,远好于原始 23-50%,但非统一 90%)。残留主因:clear_triage 的 redundant-connect-on-case + 空payload(缺fetch步)。**而 `when:` 设计(§5,实测 100%)能从结构上消除分支路由这一最大类失败——比教学更彻底。** 教学拉升 + `when:` 设计 + G8 lint 三者叠加 → workflow 达放心。
**→ 兜底机制因此是必须,不是可选**:create_workflow 后 capability_check + 结构 lint(查 3 类 + **空payload/缺fetch 步**)→ G7 envelope 回喂;模型给 verification 能自修(实测多场景自检并修)。**create_workflow 达放心 = case-contract 教学 + G8 check/fix,缺一不可。**

**★ 统一原则(test-before-accept,贯穿所有 forge 实体)**:forge first-draft 都有 ~15-45% 缺陷,**都由"建→试跑/校验→错误回喂→模型修"恢复**:
- function/handler → **run_function / call_handler 试跑**抓 syntax(含 G1-c 过度转义)/wrong-output → 修 → accept。**实测 G8 恢复曲线(temp=默认,精确 oracle,n=24,terser 令牌桶 prompt):first-draft 17% → 1 轮 71% → 2 轮 88% → 3 轮 88%(plateau)。** Round-1 的"50→100%"是 temp=0 乐观值;**生产温度下:G8 强力(17→88)但有上限——预算 ~2 轮 check/fix(第 3 轮零增益),硬状态码 post-恢复 ~88%,残留 ~12% 需 escalation(更强模型/人工)。**
- workflow → **capability_check + 结构 lint** 抓 case 路由/悬空/空 payload → 修 → activate。**实测:recover_capability_check 3/3、edit_wf 注错后恢复 3/3。**
- **铁律:绝不盲目 accept/activate forge 产物。** 这是把所有 forge 表面从 first-draft 率拉到"放心"的统一机制——不是靠单条 prompt,是靠 forge 实体内建的 test/check + 回喂回路。**两半(code + workflow)均已实测验证。**
- **规格(强制)**:create_workflow 后**必跑 capability_check + 结构 lint**(查:悬空/null 分支目标、case 节点多余 connect 边、节点收空 payload),错误以 G7 envelope 回喂让模型修。
- **规格(split-tools,A/B 实测后)**:提供 **split-tools**(`add_workflow_node` / `connect_workflow_nodes` / `set_case_branches` 分开调)。**A/B(n=3/场景)确证:split 把 brace-undercount 畸形从 ~4% 降到 0/9(G1 缓解);语义上 branch_signup 50%→3/3 明显升,但 retry_loop 反降、clear_triage 持平 → split 治 JSON 有效性,不是 case-routing 语义银弹**。**主力仍是上面的 check/fix 回路;split 与 check/fix 互补,复杂图两者都上。**

### G9 case-node CEL 求值:平台 fail-to-false(降 LLM 负担,建议)
Round-2(n=30,temp=默认)实测:模型写 CEL `when:` guard 的**布尔逻辑~全对**(timewindow/multifield 逻辑导向重判 **30/30 = 100%**),唯一不稳的是**防御性 `has()` 只 ~50% 一致**(一半 `payload.dow>=1`,一半 `has(payload.dow)&&payload.dow>=1`)。
- **规格(平台侧)**:case 求值器把"某分支 guard 求值出错"(标准 CEL 访问缺失 key 抛 no-such-key)当 **`false`** 处理 → 落到下一分支,最终 `when:"true"` 兜底;**绝不让 guard 求值错误中断 flowrun**。cel-go 实现一行:`out,_,err:=prg.Eval(act); match := err==nil && out==types.True`。
- **效果**:省略 `has()` 的 guard 也安全 → null-safety 从"LLM 必须记得"降为 belt-and-suspenders。**符合项目原则"下游能自然 handle 就别让上游背"。**
- **教学仍保留**(§5 已有 `has()` 示例)作双保险,但不再 load-bearing。
- **⚠️ 边界**:fail-to-false 意味着"写错的 guard 静默走兜底分支"——配合 G8 capability_check 在 activate 前用样例 payload 干跑各分支命中,避免逻辑错被 fail-to-false 掩盖。

### G10 📌 ops/node 的 payload(`value`/`config`)必须 pin 形状(schema 契约硬需求,**两个 A/B 验证;与 case-contract 同级**)
所有 forge 用 **ops 数组**(`ops:[{op, value}]`)+ workflow 的 **node.config**。**payload 绝不能无类型(`value:{}` / `node.config` 不 pin)** —— 模型会猜内层形状/字段名并丢判别字段。两个隔离 A/B(temp=默认,n=30):

**A/B 1 — set_output_schema 内层**(round2_pinshape.py):
| set_output_schema 的 value | canonical `{kind, schema}` |
|---|---|
| 无类型 `value:{}` | **0/30 = 0%**(73% 裸塞 JSON Schema、**丢 `kind` 判别字段** → 后端无法区分 json_schema/enum/free_text)|
| pin:`{kind, schema}`(schema 放 `schema` 键)| **26/30 = 87%**(发该 op 的 26 个 100% 对)|

**A/B 2 — create_workflow trigger 节点**(round2_trigger_ab.py,**皇冠工具,隔离测**,两臂都有 node.type 枚举,只差 config 是否 pin):
| 变体 | cron 串放对 `cron` 字段 |
|---|---|
| typed_only(= 现 V3:type 枚举有、config 不 pin)| **7/30 = 23%**(73% 放进 `schedule`、还有 `expression`)|
| pin config 形状("cron 串放 `cron` 键,非 schedule/expression")| **30/30 = 100%** |

- **与 case-contract 同类(契约歧义杀一致性,pin 死即修)。** 这是 ag_extract_invoice 68% + **content_mod 86% "cron 残留"** 的真根因(现有 reps:trigger config 见过 9 种形状、cron 串字段名 `cron`/`schedule`/`expression`/`cronExpr`/`cronExpression`/嵌套)。approval 节点反而稳(`{branches,prompt}`)因 `when:` 教学强。
- **规格(强制)**:ops/node 工具的 schema 描述**逐 op / 逐 node-type 列出 payload 形状 + 判别字段**:
  - agent ops:set_prompt→string、set_model→string、set_skill→string、set_knowledge→array(doc refs,禁正文)、set_tools→array(callable refs)、set_output_schema→`{kind:'json_schema'|'enum'|'free_text', schema:<JSON Schema>}`。
  - workflow node.config:trigger→`{kind, cron(放 cron 键), payloadSchema?}`、tool→`{ref, args}`、agent→`{ref}`、case→`{branches:{name:{when,to}}}`、approval→`{prompt, branches}`。
  - **后端选定字段名后必须 pin(cron 还是 schedule 都行,关键是 schema 里写死)。禁裸 `value:{}` / `node.config:{}` 上生产。**
- **逐 node-type 风险画像(现有 reps 实测)**:风险与"该字段是否有唯一显然约定"相关——**trigger cron(`cron`/`schedule`/`expression` 多约定)+ set_output_schema wrapper = 分裂,必 pin**;**tool `{callable,args}` 300/300、approval `{prompt,branches}` = 模型自收敛**(概念唯一)。**⚠️ catch:self-consistency ≠ correctness** —— tool 节点 100% 用 `callable`,若后端字段叫 `ref`,就是"100% 一致地错"。故即便自收敛的字段也要 pin/对齐到后端真实字段名(或后端接受模型的自然命名 `callable`)。

---

## §0.1 Scorecard(实测语义正确率,of-attempts = 剔除合理澄清)

| Surface | of-attempts | 验证模式 | 判定 |
|---|---|---|---|
| create_handler | **简单 100% / 复杂 state 58%** | CODE 真执行 | 🟡 bare-names 稳;复杂状态(令牌桶补充)靠 call_handler 试跑+回喂兜(G8) |
| create_agent | **90%** (n=60) | ARTIFACT | ✅ 强(陷阱教学 17→95%) |
| create_function | **90%** (n=80) | CODE 真执行 | ✅ 强(fn_csv ~25% code 缺陷待观察) |
| CEL case | **阈值类 100% / 布尔条件 0-18%** | ARTIFACT | 🔴 branch-key 契约 LLM 脆弱 → **建议改 `when:` 分支设计**(§5) |
| create_workflow | **~80-82%**(`when:`设计;迭代场景~80% + **全新场景 82% 防过拟合确认**;expense 3级路由 100%;原始 23-50%)| ARTIFACT | 🟡 **`when:` 治 case 路由类**(§5,robust generalize);残留=retry计数/数据流 → G8 兜 |
| 多轮 ReAct(edit/诊断/跨实体/恢复/lazy) | **5-6/7**(×3 跑)| 真多轮 Claude-后端 | ✅ 强;edit_wf 经 `when:`+恢复 0→3/3 |
| **端到端 composite(从零搭完整自动化)** | **4/4** | 真多轮 | ✅ **全产品用例 work**(recon→forge→accept→wire→check→activate)|
| 工具选择(91 全集) | **~91% reasonable** | USAGE | ✅ 非风险点 |
| Utility/CONTENT (×8) | **100%** (n=64) | CONTENT | ✅ 强项 |

**一句话(breadth 验证后,诚实)**:agent / 简单 function / utility / 工具选择 / 多轮编排(诊断·跨实体·恢复)= **88-100% 且多样场景 generalize**(放心)。**唯一系统性脆弱 = case 节点"表达式值==分支键"契约**:阈值类 100%,但**布尔条件 + 语义分支名 0-18%**(模型写布尔表达式配语义键→不匹配),教学只部分扭得动 → **建议改 `when:` 分支设计(§5,动土前改)**。其余残留(复杂 handler state 58% / 难 polling / workflow 缺fetch步)= first-draft 缺陷,靠 **G8 forge test/check-before-accept 回喂**恢复。**净结论(Round-2 大样本 + 生产温度复验后):设计大体 work;动土前 schema/平台层四件 + 运行期一件 ——(1) case-node 改 `when:` 分支(消除 #1 脆弱,0-18%→100%)、(2) G1 后端 JSON-repair、(3) G10 ops/node payload 逐 op/type pin 形状(禁裸 `value:{}`/`config:{}`;set_output_schema 0→87、trigger 23→100)、(4) G9 case 求值 fail-to-false;(5) 运行期 G8 test/check-before-accept(查不可达节点)。(1)(3)(4) 同一主题:把 LLM 易错契约 pin 在 schema/平台,别靠模型猜或教学补——均已 A/B 验证。**

### §0.1b Round-2 大样本 scorecard(n=50-100,temp=默认,±95%CI)
> 生产真实温度 + 大样本 + CI 复验。完整数据见 `13-validation-report.md §R2`。

| 分层 | of-attempts | surface |
|---|---|---|
| **100% ±0** | 满分 | expense_approval · ag_router · ag_trap_pdf · when_compound · when_3way · celw_5way · **celw_timewindow** · **celw_multifield** · fn_dedup · fn_validate_email · hd_sliding · hd_connpool · lc_pick(长上下文消歧) · inj 注入字段语义 |
| **92-98%** | 强 | nullguard 98 · fn_workdays 98 · cache_ttl 98 · fp_multisource 97 · km_skill 97 · branch_signup 96 · backup_retry 94 · bigwf_ecommerce/support 93 · hd_oauth 92 |
| **83-88%** | 良 | order_fulfill 88 · content_mod 86 · clear_triage 83 |
| **62-70%(真弱区)** | 弱 → G8 兜 | bigwf_etl 70(重试回边)· lead_scoring 70 · **ag_extract_invoice 68**(ops `value:{}` 无类型→内层 `schema` vs `value` 猜 ~33%)· fp_status_poll 67 · **hd_ratelimit 62**(令牌桶算法)|
| **artifact(非弱点)** | 表观低实为合理 | km_knowledge 表观 50% = 漏给 search 工具→模型正确反问要 doc id(澄清);真调用都正确挂引用 |

**Round-2 关键结论**(并入 §0 G-table):
- **温度敏感**:temp=0 高估;生产温度下 ag_extract_invoice 92→68、CEL `has()` 一致性降 → **结论按生产温度复验后仍成立,但弱区更明显**。
- **复杂≠差**:复杂 CODE(滑窗/连接池)100%,简单令牌桶 62% → 脆的是**特定算法**非复杂度。
- **矛盾需求 → 可教但需调校(完整故事)**:baseline **0/20** 识别(善意重解、静默建废图)。**宽规则** 0→100% flag 但 OVER-flag 正常(daily_report 100→47、onboarding 100→60,不可上线);**紧条件规则**(仅真矛盾 flag、信息不全按默认建)→ 矛盾 **~85% flag** + 正常 **100% 恢复**(daily/onboarding/threshold/support)。残留 ~15% → G8 lint 双保险。**教训:教学改动必须回归测试副作用。**
- **artifact 自查**:celw timewindow/multifield 初判 17%/30% 经读原始输出修正为 100%(判官过严要 `has()`/嫌 `||`≢`in`)——逻辑实为全对,佐证 G9 平台 fail-to-false。
- **大型 workflow**:无重试回边 93%,有重试回边(etl)70% → G8 lint 查"不可达节点"。

### §0.1c Round-3 全 91 工具 × ≥50 不同场景 覆盖(完整数据见 `13 §R3`)
**5202 个互不相同的场景**(每工具 53-66,全 ≥50)× 2-4 轮 ReAct × 语义判官。
- **SELECTION(选对工具)全局 82%**;lifecycle/runtime/diagnosis 100、agent 94、document 88、workflow 84、function 83、handler 79、mcp 74、skill 72、memory/base 67。**USAGE(create/read 类可信)83%,44/62 工具 ≥85%。**
- **真发现(进教学)**:① **Subagent 仅 11%** —— 用户说"派个人"模型却自己干,不委派;② **AskUserQuestion 仅 21%** —— 模型散文列取舍,不调结构化提问工具;③ **recon-over-commitment**(act-on-existing 类)—— recon 本能过强,有时循环不下手;"已有足够信息就执行"nudge 把 revert 35→100、accept 4→75。④ G10 在 edit-ops 重现。
- **act-on-existing USAGE(修 harness 后 faithful 重测,两层)**:让合成 recon echo 场景真 id/版本后污染消除——**act-简单(凭 id:revert/delete/lifecycle/replay/cancel)98-100%**(revert 全 100、delete 98-100);**act-复杂(edit 产 ops/accept/call/run/update)~47%(真信号)**:edit_agent 69·run_agent 75·update 53·accept 45-63·call 36·edit_workflow 35·edit_function 24·edit_handler 5。根因 = **G10 edit-ops 形状 + 产精确 edit 难 + 有状态类改写谨慎 → G10(pin edit-ops)+ G8(test-before-accept)叠加领域**(R1/R2 真后端 edit 3/3 = 配 test/check 后达标)。**合成单轮忠实测 SELECTION + create/read/简单-act;复杂-edit 真偏低,靠 G10+G8。**

**R3 三新方向(用户追加,详 `13 §R3`)**:
- **Lazy domain-6 优于 11-edituse**(激活对组 62% vs 46%,domain-6 从不激活错组)→ 保持 domain-6(见 §2)。摩擦:skill 命名撞车 + search-first-resident,建议后端"调未激活组工具自动激活"。
- **edit-ops 也要 pin(G10)**:edit_function/handler 的 ops 裸 `{type:object}` → pin 后 canonical code-key 46→66 / 30→77。
- **🎯 多轮端到端最重要产品洞察**:复杂多实体自动化首发 **all-perfect 0/24、per-check 53%** —— 模型走完全链路结构 + 接对 ref(G8 反馈版 23/24),但语义架构决策(实体类型/路由模式/polling/多字段守卫)各步 ~一半对,**连乘 → 一次性全对 ≈ 0**。**复杂自动化无法 one-shot:产品必须做成"建→审→`:iterate` 改→G8 兜结构"的迭代对话(N5),非一发入魂**;简单端 R1/R2 真反馈 composite 4/4。**capability_check 必须真查 ref 报缺失(不能恒 ok),否则模型无从修。**

**R3 复杂批(300 难场景)+ R3-C 端到端全修复(详 `13 §R3`)**:复杂 forge 选对工具 ~100%;3-judge + code-exec:**create_function 88%(58/60 clean)· create_handler 93%(56/60)· cel_when 97% —— 复杂 CODE/CEL 是强项**;**create_agent 73%** 被 **malformed JSON 21×(G1 在复杂 agent 规模重现)**拖累;**create_workflow 52%**(10-20 节点大图,失败=wiring 非字段名)。**R3-C(pinned schema:when:+per-node-config 写死)对比 baseline:cel_when 97→93(等效,when: 已解决)、create_workflow 52→42(CI 重叠,无抬升)。关键:G10 schema-pin 治字段名/分支键/config-shape + 选择(robustness A/B 0→87、23→100 已证),但救不了大图组合 wiring —— 那是 G8 check/fix 专属(首发 ~50%→回喂 recover);两类修复治不同病,缺一不可。**

---

## §1 系统 prompt 规格(完整组装文本)

> 段顺序固定;**关键守则殿后**(G6)。以下为最终文本骨架(`<...>` 为运行时插值)。

```
[identity]
You are Forgify's chat agent — the user's personal AI automation engineer. You forge automation
entities (functions / handlers / agents) and orchestrate them into workflows. You are the BOSS: you
explore, design, forge, debug. The entities you forge are workers.

[how_to_work]
- Capabilities come ONLY from forge entities. There is NO platform escape hatch for workers: a
  workflow/agent never gets raw web/file/shell — you FORGE a function for any external capability.
- To CHANGE or ACT ON an existing entity, first search/get it (so you use real ids and don't clobber).
- To DIAGNOSE, investigate broad→specific (list failed runs → trace → dead letter) before concluding.
- Plan multi-step work before executing (forge → accept → wire → activate).

[tool_conventions]   ← 见 §S18;summary/destructive/execution_group 三注入字段解释一次
Every tool call includes: summary (one sentence what+why); destructive (true if irreversible);
execution_group (int; same group runs parallel, ascending groups serial).

[catalog]            ← 运行时渲染:用户的 asset 菜单(已有 fn/hd/ag/wf/doc 摘要)
<rendered asset catalog>

[chainPatternsSection]
Common chains: forge→run→accept→wire→activate; diagnose = search_flowruns→get_flowrun_trace→
get_dead_letter→(fix)→replay; capability gap = create_function→accept→edit_agent/edit_workflow.

[critical rules — LAST, by G6]
- Workers (agents/workflow nodes) get tools ONLY from fn/hd/mcp. Agents NEVER call agents. Never hand
  a worker fs/shell/web/memory/ask/subagent.
- A "classifier / 判断 / 抽取 / 路由" capability = an AGENT (create_agent), not a function.
- "知识库 / knowledge" = documents (create_document); the user's own disk files = Read/Write.
- After create_workflow, ALWAYS capability_check before activate; fix what it reports.
- Satisfiability (TIGHT, conditional — 见下方实测警告) — ONLY when the request is genuinely
  self-contradictory / impossible to satisfy together (e.g. "fully automatic, unattended" AND "every
  item needs manual approval") do you NAME the conflict + propose one workable compromise (e.g. a
  threshold) for the user to confirm. **Incomplete info (missing email / data source / format) is NOT a
  contradiction — build with sensible defaults, do NOT refuse or over-ask.** Never silently build a
  self-contradictory graph.
  ⚠️ **实测(必读):宽措辞"建造前先检查可满足性"虽把矛盾识别 0→100%,但 OVER-flag 正常请求(daily_report built 100→47%、onboarding 100→60%)——不可上线。上面这条 TIGHT 条件措辞:矛盾仍 flag ~85%(built 15%)且正常请求 built 全恢复 100%(daily/onboarding/threshold/support)。残留 ~15% 矛盾由 G8 capability_check/lint 双保险兜。** 见 §0 G0 + `13 §R2`。
- Commit-after-recon (R3 实测) — once you've searched/read the target entity ONCE, PROCEED to the requested action (edit/accept/revert/delete/run/call). Do NOT re-search/re-get the same entity repeatedly. (实测:模型对 act-on-existing 操作 recon 过度甚至循环;"已有足够信息就执行"nudge 把 revert 35→100、accept 4→75。)
- Delegation & decisions (R3 实测,按产品意图取舍) — 若希望模型委派,需强化:盘点/审计/并行子任务 → 用 Subagent(实测模型默认自己干,Subagent 选择率仅 11%)。真正属于用户的决策(不可逆策略/法务/业务权衡)→ 用 AskUserQuestion 结构化提问,而非仅散文列取舍(实测仅 21% 调该工具)。
```
- **为什么**:identity/how-to 定角色;tool_conventions 讲一次省 ~13k token(§S18);critical rules 殿后(G6 recency)+ 编码 G5 消歧 + G8。

---

## §2 Lazy 分组 + activate_tools 规格

- **Resident(始终 offer,~28)**:每实体的 search/get/create/edit + workflow lifecycle 高频 + 主对话基础 + `activate_tools`。
- **Lazy(6 组,按需激活)**:`function` / `handler` / `workflow` / `mcp` / `document` / `skill` 的低频深工具(versions/revert/executions/run/call、mcp 调用、doc/skill/memory CRUD)。
- **activate_tools 最终描述**(已验证修复 over-activation):
  > "Load a lazy tool group into your available tools (categories: function/handler/workflow/mcp/document/skill). Call BEFORE using a tool from a group that isn't available yet. **Activate ONLY the single group the immediate task needs — reason about which one (e.g. send a Slack message → mcp). Do NOT speculatively activate multiple groups; that wastes tokens.**"
- **实测**:加粗那句把"投机激活 4 组"压到"精准激活 1 组",lazy_mcp 场景 0/3→3/3。
- **search-in-lazy**:search_* 留在 resident(模型先 search 才知道要不要激活该组)。
- **🆕 R3 实测:domain-6 分组优于 11-edituse 细分(回答"6 组够不够")**。72 场景 × 两种枚举:**domain-6 激活对组 62%(剔除 skill 命名撞车后 73%)、且从不激活错组**;11-edituse 仅 **46%**(把 function-edit/function-use 等细分搞混,主动激活错子组)。**→ 保持 domain-6;粗分组更清晰,模型只判 domain 不纠结 edit/use。当年"6 不行要 11"被推翻。**
- **🆕 R3 实测的两个激活摩擦(正交,需修)**:① **skill 命名撞车**——"激活技能"→ 模型想直接调 `activate_skill`(在未激活的 skill 组里够不着),不知要先 `activate_tools("skill")`(skill 激活率因此最低)。② **search-first-resident**——模型 search(resident)后想直接 edit/run,够不着 lazy 组里的工具又没回头激活。**修法二选一**:(a) 教学加一句"调一个还没 offer 的工具前,先 `activate_tools(它的组)`;尤其用 skill 前先激活 skill 组";(b) **后端容错**:模型调用未激活组的工具时,自动激活该组并执行(或返回 `{error, next_step:"activate X first"}`)—— 推荐 (b),最省心。

---

## §3 全 91 工具规格(逐个 Description + Parameters)

> **全 91 工具的最终 Description + 参数见 [`14a-tool-catalog.md`](./14a-tool-catalog.md)**(由 `render_spec.py` 从 `spec_catalog.py` 可执行 source-of-truth 渲染,改 schema 重跑即同步)。
>
> families:function 11 / handler 12 / agent 11 / workflow 9 / lifecycle 3 / runtime 5 / diagnosis 5 / mcp 5 / skill 3 / document 7 / memory 3 / base 17 = **91**。
> 验证模式:CODE(function/handler create-edit,真执行)· ARTIFACT(workflow/agent/CEL,判结构)· CONTENT(document/memory/utility,判内容)· USAGE(读/版本/生命周期/资产/基础,91 全集选择 ~91% reasonable)。
> 描述按 §0 原则写:简洁 > 冗长、search-first 提示、实体类型消歧、何时用。

---

## §4 Forge 实体锻造完整规格(深教学 prompt + ops schema)

源:`catalog_v2.py`(已迭代验证)。以下教学已在 create/edit × workflow/agent/function/handler 验证。

### §4.1 create_workflow / edit_workflow
- 工具描述含 **NODE TYPES 教学**(5 类:trigger/agent/tool/case/approval + 各 config 形状)+ **CEL 教学** + **DATA FLOW & ROUTING 铁律** + 一个完整 example。
- **DATA FLOW & ROUTING(实测最关键,虽未根治 case 弱点,降低错误率)**:
  1. case/approval 节点**只**走 `branches`(每 branch 的 `to` 就是边);**绝不**对 case/approval 再加 `connect` 边(重复/矛盾路由)。
  2. 节点只见上游 emit 的消息;数据必须**先产出**——要外部数据先加 tool 节点 fetch,**别假设下游 agent 凭空有数据**(空 payload 给 classifier = 跑不通)。
  3. 无悬空分支:每个 `to` 指向真实节点 id;终止路径省略 branch 或指明确 end,**绝不 `to:null`**。
- ops schema:`op ∈ {add_node, remove_node, connect, disconnect, update_config}`。
- **G8 提醒**:强化教学后原始场景 ~77%;`when:` 设计(§5)根治路由类失败;残余(空payload/精确配置)→ 必配 capability_check + lint + 回喂。

### §4.2 create_agent / edit_agent
- **AGENT 教学**(已验证 90%):agent = 配置好的 LLM worker;挂载 prompt / skill(0-1)/ knowledge(docs)/ tools(**只 fn/hd/mcp,绝不平台工具,绝不另一个 agent**)/ outputSchema(enum|json_schema|free_text)/ model。
- **关键守则(实测把陷阱 17→95%)**:
  > "CRITICAL — never write a prompt that assumes a capability the agent has no tool for. An agent with no web/db tool CANNOT fetch live data; telling it to 'look up the current rate' makes it HALLUCINATE. If the task needs external data: (a) take it as a {{payload.*}} input, or (b) mount a forge fn that provides it. Always reference inputs via {{payload.*}}."
- **edit_agent**:`set_tools` **替换**整个列表 → 加工具必须带上已有的(先 get_agent)。实测 anti-clobber 正确。
- **knowledge 挂载(Round-2 实测,artifact 修正)**:真调用的 rep **都正确**用 `set_knowledge` 挂**文档引用**(`["《退款政策》",...]`),无人粘正文。km_knowledge 表观 50% 是**我测试漏给 `search_documents` 工具**→ 模型正确反问要 doc id(而非幻觉 id),是合理澄清非弱点。**真设计提示**:lazy 激活锻造带 knowledge 的 agent 时应 **co-offer `search_documents`**,模型才能 search-first 拿 doc id 再挂。(skill 挂载 km_skill 97% 已稳。)

### §4.3 create_function(含 polling)/ edit_function
- 教学:kind=normal | polling;**polling 必须** `poll(last_cursor)→{events,next_cursor}`,**只 emit 比 cursor 新的、cursor 前进、不重复、首次 last_cursor=None 要处理**(cursor 模板已给,实测 of-attempts 89%)。
- edit:`update_code` 替换整体;保留原行为。

### §4.4 create_handler / edit_handler
- 教学:stateful class;**BARE-NAMES 契约**(`__init__` 和每个 method 收裸命名参数,非 dict);init_schema/methods_schema 声明参数名。**实测 100%**(40/40 code clean)——handler 是已解决 surface。

---

## §5 CEL / callable-ref / 注入字段规格

### CEL(case 表达式,实测 82%)
- 只读 `payload`/`ctx`;无副作用。
- **★🔴 表达式↔分支键契约 = #1 根因,且教学只部分有效 → 建议改设计**:case `expression` 的 VALUE 必须精确等于某个 branch KEY。
  - 教学要求:布尔条件 → 键 `"true"`/`"false"` 或 ternary 返回键;分类 → expression=字段、键=enum 值。
  - **但 wave-9 breadth 实测:教学只在"阈值→自然 ternary"(cel_3way 100%)生效;布尔条件 + 语义分支名(`vip||amount>=5000`→fast/normal、`has&&email`→notify/skip)模型坚持写布尔表达式配语义键 → 0-18%。** LLM 强先验"表达式=条件、分支名=语义标签"扭不过来。
  - **🚩 设计建议(动土前改,优于教学补丁)**:case 分支改为带 `when: <布尔CEL>` 守卫,而非 key 匹配:
    ```
    case: { branches: {
      fast:   { when: "payload.vip || payload.amount >= 5000", to: f },
      normal: { when: "true", to: n } } }   // 每分支一个布尔条件 = LLM 强项
    ```
    **🎯 实测验证(wave-10,非假设)**:同样的布尔条件场景,key-match 版 0-18% → `when:` 版 **~100%**——模型对"每分支一个 when 守卫"处理完美(when_compound 全 rep `fast:{when:"payload.vip||payload.amount>=5000"}, normal:{when:"true"}`;nullguard 甚至自动双重 null-safe)。**这是对 `04-case-node.md` 的设计修正建议,且已实测确认能根治 #1 脆弱。** 若保留 key 匹配契约,布尔路由必须靠 G8 lint 兜底(检测"布尔表达式 + 非true/false键")。
- **null-safe**:deref 前 `has(payload.x)`(教学保留作双保险)。**⚠️ Round-2(temp=默认,n=30)实测:模型布尔逻辑 100% 对,但 `has()` 仅 ~50% 一致 → 真正的修法是 G9 平台 fail-to-false(case 求值器把 guard 出错当 false 落兜底),不靠 LLM 记得写 `has()`。** 见 §0 G9。
- **重试计数 canonical(实测修了"首次失败永不重试"bug)**:`(has(payload.attempt) ? payload.attempt : 0) < 3` ✓;**不要** `has(payload.attempt) && payload.attempt < 3`(首次 unset→false→不重试)✗。
- emit 进回边累加:`attempt: "(has(payload.attempt) ? payload.attempt : 0) + 1"`。
- 残留弱点:branch 的 `to` 目标映射偶错 + **大型图重试回边致下游不可达(bigwf_etl 70%)**(均并入 G8 lint:查不可达节点)。

### callable-ref
`fn_xxx | hd_xxx.method | mcp:server/tool | ag_xxx`;正则容忍下划线 id(`fn_send_email`)。

### 注入字段(§S18,三 slim shell)
`summary`(必填,一句话)/ `destructive`(bool,危险操作)/ `execution_group`(int,并行分组)。**注意**:工具 schema 若 `additionalProperties:false` 且未列 summary,模型无法 emit(实测发现)→ framework 注入这三字段,schema 不重复列名但 framework 接受。

---

## §6 错误信息 envelope 规格

```json
{"error": {"code": "CAPABILITY_MISSING",
           "message": "tool node references fn_send_sms which does not exist",
           "next_step": "create fn_send_sms (create_function) or change the callable ref, then re-activate"}}
```
- 每个到达 LLM 的工具错误都带 `next_step`(具体下一步)。实测:有 next_step → 模型正确自纠;无 → 乱试。
- 配合 G8:capability_check / lint 的错误走此格式回喂。

---

## §7 Utility prompts 规格

> 源 `wave4_gen.py`;最终 prompt 文本如下。**实测全部 100%(8 surface × 8 reps × 3 judge);utility/CONTENT 是 deepseek-v4-flash 强项。**

- **auto-title**(thinking off):"Output ONLY the title: ≤6 words, no quotes, no trailing punctuation, no markdown."
- **rerank**(thinking off):"Output ONLY a JSON array of the top N candidate ids, most relevant first. No prose."
- **compaction**(thinking off):"Preserve: key decisions, current task state, open questions, important ids. Drop chit-chat. Concise structured summary."
- **env-fix**(thinking off):"Return ONLY {\"deps\": [pip package names]}. No prose."(关键:bs4→beautifulsoup4 的 pip 名映射)
- **web-summary**(thinking off):"3-4 sentences, use ONLY facts present in the page, no outside info."

**实测(每项 8 reps × 3 judge,全 100%)**:auto-title 简短切题无引号 ✓;rerank valid JSON+top-1 对+无 prose ✓;compaction 保住 id/任务态/开放问题/根因不编造 ✓;env-fix `bs4→beautifulsoup4` 映射对 ✓;web-summary 准确无幻觉 ✓;doc-create 4 段齐全可用 ✓;mem-write JSON 三事实全捕获 ✓。**这些 prompt 即最终文本,照抄。**

---

## §8 Subagent system prompts 规格(已验证)

主对话 boss 可 spawn 三类 subagent。最终角色 prompt(已实测,explorer/verifier 角色遵守 24/24 零越权):

- **explorer**(只读调查;实测 12/12 零 mutation,即便诊断诱惑也不修):
  > "You are an EXPLORER subagent. Your ONLY job is to INVESTIGATE and REPORT. You may search/get/read/list/trace. You MUST NOT create, edit, delete, accept, revert, activate, replay, or mutate ANYTHING. End with a findings summary; never change state."
- **forger**(建实体到 accept;起手正确 = create 或 search-first 查重;完整 create→run→accept 链在 wave-2 cross/recover 场景已 3/3 验证):
  > "You are a FORGER subagent. BUILD the requested entity end to end: create it, test-run it, then accept the pending version. Stay focused on the one entity; do not touch unrelated things."
- **verifier**(审而不改;实测 12/12 零 mutation,不 edit/activate):
  > "You are a VERIFIER subagent. REVIEW the target for correctness/problems and REPORT findings. You may search/get/read/run read-only checks. You MUST NOT edit/fix/accept/activate anything — only report what you find and recommend."

**实测**:全 91 工具集下,explorer/verifier **零越权**(24/24 episode 无任何 mutation 调用);forger 正确起手。角色 prompt 的"MUST NOT ..."殿后表述(G6)有效约束。

---

## §9 实施 roadmap(每个 artifact 贴哪个文件)

| Artifact | 贴哪里 |
|---|---|
| §0 G1 JSON repair | `internal/infra/llm/`(解析 tool 参数处)+ `internal/transport/httpapi`(错误 envelope)|
| §0 G2 max_tokens / G3 thinking | `internal/app/chat/runner.go`(组装 req)+ per-scenario config |
| §1 系统 prompt 各段 | `internal/app/chat/` system prompt 组装;tool_conventions 段 |
| §2 lazy 分组 + activate_tools | `internal/app/tool/toolset/`(Resident/Lazy 装配 + activate.go)|
| §3 各工具 Description/Parameters | 各 `internal/app/tool/<family>/` 的 Tool 实现 |
| §4 forge 教学 | 对应 create/edit 工具的 Description() |
| §5 CEL/ref/注入字段 | case 节点校验 + `injectStandardFields` |
| §6 错误 envelope | `errmap.go` + 各 sentinel 的 next_step |
| §8 G8 capability_check + lint(查不可达节点)| workflow accept/activate 路径 + 新 lint |
| **§0 G9 case 求值 fail-to-false** | case 节点求值器(cel-go `Eval` 返 err → 该分支判 false,落 `when:true` 兜底)|
| **§0 G10 ops/node payload pin 形状** | 各 forge 工具 `Parameters()`:逐 op / 逐 node-type 写死 value/config 形状 + 判别字段(禁裸 `{}`)|
| **§1 可满足性检查规则(矛盾需求)** | system prompt critical-rules 段(殿后,G6)|

### §9.1 动土前 checklist(按"先 schema/平台、后教学"排序;均已 A/B 实测)
**Schema/平台层(消除系统性脆弱,优先):**
- [ ] case-node 改 `when:<布尔CEL>` 分支(弃"表达式值==分支键"契约)— 0-18%→100%
- [ ] G10:全 ops/node payload 逐 op/type pin 形状 + 判别字段,删所有裸 `value:{}`/`node.config:{}` — set_output_schema 0→87、trigger 23→100
- [ ] G9:case 求值器 fail-to-false(guard 出错→false→兜底)— 免 LLM 背 null-safety
- [ ] G1:`infra/llm` tool 参数解析加 JSON-repair(控制字符 + 括号配平)— 救 ~4-8% 畸形
- [ ] G2/G3:forge 调用 max_tokens=16k;forge/诊断 thinking on、utility-JSON off

**运行期机制:**
- [ ] G8:forge accept/activate 前必跑 test/check(run_function/capability_check + 结构 lint 含不可达节点),G7 envelope 回喂;**预算 ~2 轮**(温度默认 17→71→88 plateau,残留 ~12% 需 escalation)

**教学层(系统 prompt,殿后 G6):**
- [ ] agent 不可能能力禁令(17→95)· classifier→agent / 知识库→document 消歧(G5)· worker tools 仅 fn/hd/mcp + agent 不调 agent · **可满足性检查(矛盾需求 0→~85%,用 TIGHT 条件措辞——宽措辞会 over-flag 正常请求 100→47%,务必用 §1 的条件版)** · polling cursor 规则 · set_knowledge 挂引用禁粘正文 · 锻造带 knowledge 的 agent co-offer search_documents

---

> **持续迭代**:本规格随 eval 推进更新;数据见 `13-validation-report.md` + `research/llm-experiments/wave1_findings.md`。
