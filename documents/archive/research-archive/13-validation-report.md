# 13 — LLM-Facing 设计验证报告(证据)

> 支撑 [`14-llm-facing-design-spec.md`](./14-llm-facing-design-spec.md) 的实测证据。
> 被测:**deepseek-v4-flash**(thinking on);Round-1 temp 0,**Round-2 temp=默认(生产真实温度)**;判官 + 执行环境 = Claude(Max,不计预算)。
> 方法:真组装 prompt + 真 ReAct(Claude 当后端/用户)+ code 真执行 + 3-judge 对抗式语义判 + 多数表决。
> 原始数据:`research/llm-experiments/`(wave1-4 + round2_* gen/judge + `wave1_findings.md` 运行日志)。
> 日期 2026-05-30;预算 ~¥19/¥204;**Round-2 大样本复验(n=50-100,temp=默认,95%CI)已完成**。

---

## §0 TL;DR

**结论:Forgify 的 LLM-facing 设计 + 默认模型 deepseek-v4-flash 是 work 的。** 除 create_workflow 外所有表面达 **82-100%**。**create_workflow 是失败模式相关的 33-90%**:case-contract 根因教学把分支键类失败大幅修好(梯度场景 67-100%),但原始多样场景(缺 fetch 步→空 payload、难 CEL)仍 33-82% → **必须配 G8 check/fix 回喂回路才达放心**。两条机制硬需求(不落实生产会真出事):**G1(后端 JSON-repair)、G8(forge test/check-before-accept 回喂)**。**核心方法论结论:改 tool 描述/教学能可复现推高语义率(5 次迭代:agent 陷阱 17→95、lazy 4→1、classifier 0→100、case-contract 梯度 +30pt);但教学有边界——数据流类失败靠机制(G8)而非纯 prompt。**

### Master 发现(详见 spec §0)
| # | 发现 | 证据 |
|---|---|---|
| **G0** | 改教学 prompt 能可复现推高语义率 | agent 陷阱 17%→95%;lazy 激活 4→1 |
| **G1** 🔴 | DeepSeek ~4-8% 复杂 tool 参数是畸形 JSON(控制字符 + brace-undercount);Go 默认拒 | 实测 v1 4/96;`json_repair` 配平恢复 100% |
| **G2** | 复杂锻造 max_tokens≥16k(8k 截断) | 8k:1/96 length;16k:0/320 |
| **G3** | thinking 别全局关,按任务;多轮回传 reasoning_content | 复杂任务 on 更好;JSON 类 off 省钱不掉质 |
| **G4** | search-first 是模型默认且正确 | wave-3:多数"未直接调"实为合理 search-first |
| **G5** | 实体消歧靠描述(91 全集 ~91% reasonable);2 处需补 | 分类器→agent;知识库→document |
| **G6** | 关键守则殿后(recency,反 OpenAI) | — |
| **G7** | 错误带 next_step 模型能自纠 | recover_capability_check 3/3 |
| **G8** 🔴 | forge test/check-before-accept(workflow 尤甚)| workflow 原始 23-50%→教学后 58-92%;`when:` 设计根治路由;check/fix 兜余 |
| **G9** 🟡 | **CEL guard null-safety 应平台级 fail-to-false,非 LLM 负担** | temp=默认下模型 CEL 布尔逻辑~全对,但防御性 `has()` 仅 ~50% 一致;case 求值器把"guard 出错"当 false(落 `when:true` 兜底)即根治 |
| **G10** 🔴 | **ops/node 的 payload 必须 pin 形状;禁裸 `value:{}`/`node.config:{}`** | 2 个 A/B:set_output_schema 0→87%;**create_workflow trigger config 23→100%**(隔离测,皇冠工具)。是 ag_extract 68% + content_mod cron 残留的根因,与 case-contract 同类 |

**Round-2 大样本(n=50-100,temp=默认)三条新结论:**
- **温度敏感(方法论)**:temp=0 会**高估** robustness。生产温度(默认)下抽取 agent 的 schema 选择、CEL `has()` 一致性下降(如 ag_extract_invoice 92%@temp0 → 68%@temp默认)。**结论必须按生产温度复验。**
- **矛盾/不可能需求 → 可教(已订正:原以为难教)**:baseline 给"全自动无人值守 + 每笔人工审批"矛盾需求,模型 **0/20 不识别(善意重解 + 静默建废图)**;**加一条殿后"可满足性检查"系统规则 → flag 20/20=100% + 提合理折衷 100%**(A/B n=20,temp=默认)。**又一 G0 类教学win,推翻原"教学难根治"。** 双保险仍建议 capability_check/lint 兜底。
- **"复杂≠差"**:复杂 CODE(滑动窗口、连接池)temp=默认 n=30 **100%**;反而"简单"令牌桶 ratelimit 仅 62%。**脆的是特定算法正确性(令牌桶按时间补充的数学),不是复杂度本身。**

---

## §1 方法论(为什么这次的数能信)

### 1.1 四支柱(真·端到端)
1. **真组装 prompt**:完整 system prompt(identity/how-to/tool_conventions/catalog)+ 真 tool schema(`catalog_v2` / `spec_catalog` / `wave2_build`),不是塞单条命令。
2. **真 ReAct 多轮**:Claude subagent 当后端(动态返真实结果 + 注错)+ 用户(澄清)+ 裁判;不喂 canned。
3. **code 真执行**:function/handler 生成的 Python 子进程实跑(mock 外部依赖),三档 clean/runtime_error/wrong_output。
4. **语义裁决**:每个产物 3 个 Claude judge 对抗式审(默认怀疑)+ 多数表决(≥2)。**不靠结构对蒙混。**

### 1.2 判官可靠性(已核对)
独立人工读原始输出形成 ground-truth,再比对判官:判官**精确抓到**全部——fetch-step-missing / 空 payload / 冗余 connect 边 / 截断残片 / agent 不可能能力陷阱。判官未乱判;3 票多数表决降单判官偏差。

### 1.3 指标:of-attempts(剔除合理澄清)
单决策测会惩罚"先澄清"(vague 任务下是好行为)。主指标用 **of-attempts**(剔除 clarified-not-attempted),澄清率单列。教训:**n=6 噪声过大(±33% 摆动),per-scenario 率 n≥20**。

### 1.4 ⚠️ 自我纠错记录(rigor)
- 一度把 wave-3 选择记成 35%——实为**假阴性**:打分只认终端工具当第一步,惩罚了 search-first。纠正(credit search-first)后 **91% reasonable**。**假信心包括假阴性;已纠。**
- 多个"下降"经查为 n=6 噪声或 clarified 污染,非教学退步;n=20 + of-attempts 后信号清晰。

---

## §2 逐 surface 数据 + 迭代日志

### create_handler — 100%(n=40,CODE 真执行)
hd_oauth / hd_cache_ttl 各 20/20 clean_correct。**bare-names 契约守住**,stateful 逻辑(token 过期刷新、TTL 缓存)真跑通。已解决 surface,无需迭代。

### create_agent — 90%(n=60,ARTIFACT)
- ag_enum_sentiment 100%、ag_json_extract 75%(json_schema 字段偶缺)、**ag_trap_web**:**v0 17% → 教学后 95%**。
- 迭代:`_AGENT_TEACHING` 加"绝不写假设无工具能力的 prompt;外部数据走 {{payload}} 或挂 forge fn" → 陷阱识破率 17%→95%。**最强的"教学有效"证据。**

### create_function — 90% of-attempts(n=80,CODE 真执行)
- fn_workdays 95%(19/20 clean)、fn_csv_parse 75%(5/20 runtime_error = 真 code 缺陷)、fp_dirwatch 100%、fp_rss of-attempts 89%(cursor 教学生效;clarify 率高因 RSS 源欠规格)。
- 迭代:`_POLLING_TEACHING` 强化 cursor(只 emit 新的/前进/去重/首次 None 处理)。

### CEL case — 82%(n=60,ARTIFACT)
- cel_nullsafe 100%、cel_retry 94% of-attempts、cel_vip 55%(CEL 表达式对,但 **branch `to` 目标映射偶错** → 并入 G8 lint)。
- 迭代:`_CEL_TEACHING` 加重试计数 canonical `(has(x)?x:0)<3`(修了"首次失败永不重试"bug)。

### create_workflow — 失败模式相关 33-90%(ARTIFACT)🟡 最复杂表面
3 类失败模式,各自结论不同:
- **(A) case 表达式↔分支键不匹配**(布尔表达式配字符串键→永不匹配):case-contract 教学**根治**。难度梯度场景 g2/g3/g5 修后 67-100%(修前 25-33%)。
- **(B) 数据流:缺 fetch 步→节点收空 payload**(clear_triage:cron 直连 classifier,无拉邮件步):教学强化中(cron 无业务数据→首节点必 fetch);残留靠 G8 lint。
- **(C) 难 CEL / 精确配置**(branch_signup 解析 email 域名;retry 精确 3 次上界):教学难根治,靠 G8 + 多轮自修。
- **诚实**:case-contract 在**专门设计的梯度场景**大涨(58→90),但**原始多样场景不 generalize**(clear_triage 23→33、branch_signup 50→33)——因主导失败是 (B)(C) 非 (A)。
- **→ G8 是必须(非可选)**:create_workflow 后 capability_check + 结构 lint(查悬空/空payload/缺fetch/冗余边)→ next_step 回喂;模型给 verification 能自修(见 §3 样本)。**create_workflow 达放心 = 教学(修 A)+ G8 check/fix(兜 B/C),缺一不可。**

### 多轮 ReAct — 6/7(真多轮 Claude 当后端)
edit_wf / edit_agent / edit_fn / **diag_orders_crash(诊断皇冠)** / cross_add_capability / recover_capability_check / lazy_mcp。
- 强信号:read-before-edit、诊断 broad→specific、**上游修非创可贴**、错误恢复(建缺失 fn 再 recheck)、不幻觉 id、anti-clobber set_tools 正确。
- 2 个初 FAIL 修复验证:edit_agent(补 search_functions)0/3→3/3;lazy(activate_tools 教学)0/3→3/3 over-activation 根治。
- edit_wf 曾因 case 路由 0/3 → **`when:` 设计 + G7 错误恢复后 3/3**(注入 INVALID_NODE_CONFIG 后模型用 `when:` 分支正确恢复)。3 次跑 diag/edit_fn/recover/lazy 稳定 PASS;偶发 FAIL(越界/边缘)始终 accomplished=true。

### 端到端 composite(从零搭完整自动化)— 4/4(真多轮,终极实战)
comp_onboarding ×2 + comp_daily_report ×2 全 PASS(3/3)。模型从零搭完整多实体自动化:**recon(search→empty)→ forge(agent + 多 function)→ accept → create_workflow(数据流对、case `when` 守卫)→ capability_check → activate → verify**。正确选型(总结器=agent 非 function)、fetch 在 summarize 前(无空 payload)、零幻觉 id、capability_check 都在 activate 前。**证明整条产品链端到端 work + 长程多实体连贯。**

### 工具选择(91 全集)— ~91% reasonable(USAGE)
search-first + 实体家族路由稳;消歧陷阱过(read_kb→documents 非本地 Read)。2 处小混淆:分类器→agent、知识库→document(spec §G5 已编码修法)。

### Utility / CONTENT — 100%(n=64,CONTENT)
auto-title / rerank×2 / compaction / env-fix(bs4→beautifulsoup4)/ web-summary / doc-create / mem-write 全满分。deepseek-v4-flash 强项。

---

## §3 端到端轨迹样本(看真实行为)

### 诊断皇冠(diag_orders_crash,真多轮,PASS 3/3)
```
get_workflow + search_flowruns(failed)        → 拿图 + fr_a/fr_b 两次失败
get_flowrun_trace ×2                           → 两次都挂在 process_node:KeyError 'customer_id'
get_function(fn_fetchorder) + query_events     → 确认 fetch 未返 customer_id(读 code 再动)
edit_function(加 customer_id 到 SELECT+返回)   → 显式权衡 fix-fn vs relax-agent,选上游修(非创可贴)
get_function(verify) → replay_message ×2       → 修后才 replay,两条死信 completed
final: 根因 + before/after SQL diff + 影响范围 + replay 结果
```
零幻觉 id;诊断顺序 broad→specific;上游根因修;修后才 replay。**AI 工程师能力成立。**

### 自我纠错(edit_wf_add_retry,展示 G8 自修)
模型建重试时误对 case 节点加了 connect 边 → turn6 `get_workflow` verify 时**自己发现**"冗余边,case 应走 branches" → 发 disconnect 修(可惜 max_turns 截断)。**证明:给 verification,模型能自检自修 case 路由错** → G8 check/fix 回路有效。

---

## §R2 — Round-2 大样本复验(n=50-100,temp=默认,±95%CI)

> 动机:Round-1 多为 n=20、temp=0;按"样本量太小(一般 50-100)+ 补复杂/新维度 + 用生产真实温度"复验。
> temp=默认(API 默认 ≈ 生产)、95%CI(正态近似)、of-attempts、3-judge 多数表决。原始:`research/llm-experiments/round2_*.py` + `wf_judge_r2*.js`,数据 `/tmp/r2`、`/tmp/r2c`、`/tmp/r2n`、`/tmp/r2mt`。

### R2-A 鲁棒性(n=50,temp=默认)— 统计稳的分层
| 层 | surface(of-attempts)|
|---|---|
| **100% ±0** | expense_approval · ag_router · ag_trap_pdf · when_compound · when_3way · fn_dedup · fn_validate_email(后2 50/50 code clean)|
| **92-98%** | when_nullguard 98 · fn_workdays 98 · hd_cache_ttl 98 · wf_branch_signup 96 · wf_backup_retry 94 · hd_oauth 92 |
| **83-88%** | wf_order_fulfill 88 · wf_content_mod 86(cron 配置残留)· wf_clear_triage 83 |
| **62-70%(真弱区,CI 稳)** | wf_lead_scoring 70 ±13 · ag_extract_invoice 68 ±13 · fp_status_poll 67 ±18 · **hd_ratelimit 62 ±14**(令牌桶补充算法)|

**强表面在 n=50/temp=默认下岩石般稳**(simple forge/code/agent/CEL 88-100%)。弱区性质三类:① 算法正确性(ratelimit 令牌桶、status-poll 状态转换)→ **G8 试跑兜**;② **schema 契约歧义(extract 的 json_schema kind / content_mod 的 cron 字段)→ G10 pin 死**(已 A/B 确诊 + 修);③ CEL guard null-safety(lead-scoring 的 `>=70`)→ **G9 平台 fail-to-false**。**三类弱区均已定位到机制修法,非"模型不行"。**

### R2-B 复杂单发(n=30,temp=默认)
| 类 | surface(of-attempts)|
|---|---|
| **复杂 CODE** | hd_sliding **100%** · hd_connpool **100%** · fp_multisource 97%(均真执行 30 reps)|
| **大型 workflow** | bigwf_ecommerce 93% · bigwf_support 93% · **bigwf_etl 70% ±16**(重试回边 wiring 真残留)|
| **复杂 CEL when** | celw_5way **100%** · celw_timewindow **100%** · celw_multifield **100%**(逻辑全对;见下方 artifact 修正)|

- **反直觉:复杂 CODE 反而满分**(滑动窗口/连接池 100/100),"简单"令牌桶 ratelimit 才 62%。→ **脆的是特定算法(令牌桶按时间补充的数学),不是复杂度。**(G-table 已记)
- **bigwf_etl 70%(剥 artifact 后的真残留)**:7/30 重试/load/deadletter 子图从 transform-成功分支**不可达**——大型 ETL 的有界重试回边 + load-gating wiring 是锻造最难点,~30% 错。**→ 确证 G8 结构 lint 必须查"不可达节点"。** ecommerce/support(93%)无重试回边,故更稳。
- **⚠️ artifact 修正(方法论严谨)**:celw_timewindow/multifield 初判 17%/30%,经读原始输出发现是判官过严——模型布尔逻辑 30/30 全对,失败项是"没写防御性 `has()`"(归 G9 平台修)+ `region=='US'||region=='EU'` 被判"没用 `in[]`"(**逻辑完全等价**)。逻辑导向重判 → **均 100%**。这是本研究第 5、6 个被自查抓出的判官假阴性(框架靠"判前读原始输出"持续纠偏)。

### R2-B 多轮复杂(真多轮,Claude 当后端 + 注错)
- **deep_support_system 2/2 PASS**:从零搭 6 实体相互依赖系统(handler hd_tickets + 3 fn + agent ag_support + workflow),forge→accept→test→wire→capability_check→activate;refs 全一致零幻觉;rep2 **自愈**了注入的后端不一致(读 get_handler 核源码)。**复杂多实体长程连贯成立。**
- **cascading_diag 1/2**:级联 3 错链(KeyError→TypeError→timeout-config),一个 19 轮全恢复、一个没到 error-3。**多错链恢复是上限区。**
- **dirty_contradictory:0/3 → 可教但需调校(完整故事,含回归)**:矛盾需求("全自动无人值守 + 每笔人工审批")baseline **0/20 不识别**(善意重解、静默建废图)。
  - **宽规则**("建造前先查可满足性")→ flag **0→100%**(3 判官)。**但回归暴露副作用**:OVER-flag 正常请求(daily_report built 100→**47%**、onboarding 100→**60%**——过度反问缺失细节)→ **不可上线**。
  - **紧条件规则**(仅真矛盾才 flag、信息不全按默认建、别多问)→ 矛盾 **~85% flag**(built 3/20)+ 正常 **100% 全恢复**(daily/onboarding/threshold/support 15/15)。**这是可上线版**;残留 ~15% 矛盾 → G8 capability_check/lint 双保险。
  - **教训(假信心)**:"0→100% 干净修复"经严格回归才现 40-53% 正常请求被牺牲;**任何教学改动必须回归测试副作用**。框架 `round2_contradiction_ab.py`/`round2_contra_regression.py`/`round2_contra_v2.py`。

### R2-C 新维度(n=30,temp=默认)
- **长上下文消歧(60 实体目录 + 长历史)**:lc_pick_email/wf/handler ~**100%**——在 60 个资产里精确认出对的 id(fn_send_email 非 sms/slack;wf_order_pipeline;hd_db_pool),零幻觉,不被大目录干扰。**长上下文不退化。**
- **注入字段(summary/destructive/execution_group,§S18)**:delete→`destructive=true` 30/30;3 个独立 run→**同 execution_group(并行)**30/30;get→run 依赖→**升序 group**。**§S18 注入字段语义模型拿捏准。**
- **知识/技能挂载**:km_skill(set_skill='summarization')**97%**;km_knowledge 表观 **50%** 是 **artifact**(已读原始输出核实):真调用 set_knowledge 的 rep **都正确挂文档引用**(`["《退款政策》",...]`),无人粘正文;低分全因**我测试漏给 `search_documents` 工具** → 模型正确反问要 doc id(而非幻觉)= 合理澄清。**真设计提示**(非弱点):lazy 激活锻造带 knowledge 的 agent 时 co-offer `search_documents`,让模型 search-first 拿 id 再挂。**第 7 个被"判前读原始输出"抓出的表观假阴性。**

### R2 自一致性(n=50 reps 的结构签名众数占比 → 确定性代理)
**MEAN 83%**:同一请求多次跑结构签名众数占比 83%(越高越确定 → UX 信任)。强表面(when/简单 fn/agent)95-100% 几乎确定;弱表面分散度高(与 of-attempts 弱区吻合)。

### ★ R2 弱区完整账本(每个 < 90% 表面都读到根因:机制修 or artifact)
Round-2 把每个弱表面读原始输出到根因——**没有一个是"模型能力不行"**:
| 弱表面 | 根因 | 归属 |
|---|---|---|
| hd_ratelimit 62 | 令牌桶按时间补充算法易错 | **G8** 试跑回喂(temp=默认曲线 17→71→88% plateau;~2 轮,残留 ~12% 需 escalation)|
| bigwf_etl 70 | 大型图有界重试回边致下游不可达 | **G8** lint 查不可达节点 |
| ag_extract_invoice 68 | set_output_schema `value:{}` 无类型→丢 kind | **G10** schema pin(A/B 0→87)|
| content_mod 86(cron)| trigger `node.config` 无类型→cron 串放错字段 | **G10** schema pin(A/B 23→100)|
| lead_scoring 70 | when `>=70` guard null-safety | **G9** 平台 fail-to-false |
| celw_timewindow/multifield 17/30 | 判官过严(要 `has()`/嫌 `\|\|`≢`in`)| **artifact**(重判 100%)|
| km_knowledge 50 | 漏给 search 工具→模型正确反问要 doc id | **artifact**(真调用都对)|
| fp_status_poll 67 | 转换逻辑实正确;NOCALL=合理澄清 + harness 跑不了外部 HTTP | **artifact**(+ 温和教学:外部 IO 端点/密钥来源)|

**净:3 机制(G8 算法/wiring · G9 null-safety · G10×2 schema-pin)+ 3 artifact(读原始输出纠偏,本研究累计 8 个假读数被自查抓出)+ 1 温和教学。结论:deepseek-v4-flash 能力不是瓶颈;契约设计(pin 在 schema)+ test/check 机制才是。**

### ★ G8 恢复曲线(temp=默认,精确 oracle,n=24,terser 令牌桶 prompt)
直接量化 load-bearing 机制 G8 在生产温度下的力量与上限(`round2_g8_recovery.py` + `g8_test.py` mock-clock oracle):
| 轮次 | 正确率 |
|---|---|
| first-draft(0 修)| 4/24 = **17%** |
| 1 轮 test-feedback | 17/24 = **71%** |
| 2 轮 | 21/24 = **88%** |
| 3 轮 | 21/24 = **88%**(plateau,零增益)|
- **G8 强力但有上限**:一个具体测试失败回喂就把大部分修好(17→71),两轮收敛 88%,**第 3 轮零增益**,残留 ~12% 复杂状态码三轮也修不好。
- **Round-1 的"50→100%"是 temp=0 乐观值**(已订正)。**工程含义:预算 ~2 轮 check/fix;硬状态码 post-恢复 ~88%,余 ~12% 需 escalation(更强模型/人工)。** 这也是为何 accept/activate 前必跑 test/check(G8),且回喂回路要给够回合。

---

## §R3 — 全 91 工具 × ≥50 不同场景 完整覆盖测试

> 用户硬要求:**每个 tool 至少被 50 个不同的场景调用**(不是重复跑 50 次,是 50 个不同情况)。
> 三阶段:① Claude 按工具 author **5202 个互不相同**的场景(91 工具 × 53-66 个,全 ≥50 ✓)→ ② deepseek-v4-flash **2-4 轮 ReAct** 每场景跑(给该工具所在家族工具集;诚实捕获 search-first→终端动作)→ ③ Claude 语义判官逐场景判 SELECTION(选对工具)+ USAGE(args/产物正确)。原始:`round3_*.py` + `wf_gen_r3.js` + `wf_judge_r3.js`,数据 `/tmp/r3scen|r3res` + `research/llm-experiments/r3_coverage_result.json`。

### R3 覆盖达标 + 可信结果
- **覆盖底线 100% 达标**:91/91 工具各 ≥50 不同场景(总 5202)。
- **SELECTION(选对工具,可信)全局 82%**;各族:lifecycle/runtime/diagnosis **100%** · agent 94 · document 88 · workflow 84 · function 83 · handler 79 · mcp 74 · skill 72 · memory 67 · base 67。**模型在大多数多样情况下能从族内选对工具。**
- **USAGE(create/read/discovery 类,可信)83%**;62 个非-act-on-existing 工具中 **44 个 ≥85%**(search/get/list/read/create/lifecycle/diagnosis 多在 95-100%)。

### R3 真发现(已读原始轨迹核实,非 artifact)
1. **Subagent 选择率仅 11%(真)**:用户明说"派个人/找人复核",模型却 `activate_tools` **自己干盘点/复核**,不委派 subagent。→ 若要委派,需强化 Subagent 描述/教学。
2. **AskUserQuestion 仅 21%(真)**:面对真需用户定夺的决策,模型用**散文列取舍**而非调结构化提问工具。→ 若 UI 依赖该工具,需教学触发。
3. **recon-over-commitment(act-on-existing 通病)**:edit/accept/revert/activate_skill/forget/call_mcp 类,模型 recon 本能极强,多轮 search/get **才肯下手**,单轮里甚至循环不 commit(部分正确=read-before-edit,部分过度)。**实测一句"你已有足够信息,执行吧"nudge:revert 35→100%、accept 4→75%。** → 生产建议:recon 结果要完整可信 + 可教"读过一次就执行别无限复核"。
4. **G10 在 edit-ops 重现**:edit_function 的 ops 用 `value` vs `code`/`kind` 不一致 → 再次印证 G10(ops payload 必须 pin)。
5. **call_mcp_tool 需 3 步发现**(list→search→call),且常分流到 install_mcp_from_registry —— MCP 多步链是上限区。

### ⚠️ 方法论诚实:act-on-existing 的 USAGE 本测**不可信**(harness 限制)
edit/revert/update/delete/run/call/accept 类工具的 USAGE 低分(use 2-39%)**主因是合成后端污染**:单轮合成 recon 返回**通用 id/版本/code**,模型采纳后覆盖了场景的真实 specifics(实测 **59% 的 act 终端调用用了我合成的通用 id**)→ 判官读场景意图判"操作错实体"。**这是 harness 无法镜像每场景状态所致,非模型缺陷**;这些工具的 USAGE 已在 **Round-1/2 多轮真后端**(Claude 当后端、镜像真实状态)验过:edit_wf/edit_agent/edit_fn **3/3**、recover/diag 皆 PASS。

**→ 修复 harness 后重测(让合成 recon echo 场景真 id/版本,`_extract_ctx`):污染消除,act-on-existing USAGE 现可信,且分两层:**
- **act-简单(凭 id 动作:revert/delete/lifecycle/replay/move/cancel)≈ 98-100%** —— revert_fn/hd/ag/wf 全 **100**、delete 98-100、trigger/activate/deactivate/cancel 98-100。**证实之前低分纯属合成 id 污染,faithful 后近完美。**
- **act-复杂(edit 产 ops / accept / call / run / update)≈ 47%(真信号,非 artifact)** —— edit_agent 69 · run_agent 75 · update_handler_config 53 · accept_pending 45-63 · call_handler 36 · edit_workflow 35 · edit_function 24 · **edit_handler 5**。根因:**G10 edit-ops 形状(value/code/kind 不一致)+ 产出精确 edit/config 本就难 + edit_handler 仍 recon-loop**(有状态类改写谨慎)→ **这些是 G10(pin edit-ops)+ G8(test/check)叠加的领域**,与 R1/R2"edit 需 test-before-accept"一致。
- **结论(诚实闭环)**:SELECTION + create/read/简单-act USAGE 合成可忠实测(82% / 85-100%);**复杂-edit USAGE 真的偏低(~47%),靠 G10 pin edit-ops + G8 回喂兜**(R1/R2 真后端 edit 3/3 = 配 test/check 后达标)。

### R3 复杂批(300 难场景,每皇冠表面 60 个 intricate)+ R3-C 端到端全修复
复杂批用 Claude author 300 个**难端**场景(大图/难 CEL/复杂状态机/复杂算法/复杂 agent),deepseek 单发 forge(选对 create 工具 ~100%),3-judge + code 真执行:
| 复杂表面 | of-attempts | 备注 |
|---|---|---|
| **create_function** | **88% ±8** | code-exec 58/60 clean(复杂算法强;2 个 ISO8601/空行边界)|
| **create_handler** | **93% ±6** | code-exec 56/60 clean(滑窗/令牌桶/连接池等复杂状态机强;3 个多桶 admit 边界)|
| **cel_when** | **97% ±5** | 复杂 when-guard,when: 设计**硬**(3 个 dead-branch/嵌套路径边界)|
| **create_agent** | **73% ±11** | 🔴 **被 malformed JSON `_unparseable` 21× 拖累(占失败 35%)** → G1 在复杂 agent 上规模重现 |
| **create_workflow** | **52% ±13** | 10-20 节点大图;失败 = wiring(顺序/守卫/悬空/数据流),非字段名 |

**R3-C 端到端全修复**(pinned create_workflow schema:case 用 when: 分支 + per-node-type config 写死 + 工作流教学殿后)对比 baseline(标准 V3 工具):
| 表面 | baseline | R3-C(全修复)| 解读 |
|---|---|---|---|
| cel_when | 97% | **93%**(CI 重叠,等效)| when: 设计已解决 case 路由(pin 与教学等效)|
| create_workflow | 52% | **42%**(CI 重叠,无抬升)| **schema-pin 救不了大图 wiring 复杂度** |

**关键诚实结论**:① G10 schema-pin 解决**字段名/分支键/config-shape**类错误 + 选择(已在 robustness A/B 证 0→87、23→100);② 但**超大图的组合 wiring**(10-20 节点全对)首发 ~50% 是 **G8 check/fix 专属领域**,schema-pin 不叠加于此 —— **两类修复治不同病,大图必须 G8 兜**;③ **G1 在复杂 agent 上规模重现**(malformed JSON ~17%)→ 后端 JSON-repair 必做再确认;④ 复杂 CODE(function 88/handler 93,code-exec 实证)+ 复杂 CEL(97)是模型强项。

### R3 lazy 分组:domain-6 vs 11-edituse(回答"6 组够不够")
72 个不同场景(每 domain 12)× 两种 activate_tools 枚举 × 多轮(search-first→activate):
| 方案 | 激活对组 | 备注 |
|---|---|---|
| **domain-6**(function/handler/workflow/mcp/document/skill,现行)| **62%(45/72)** | **激活时 0 错组**(剔除 skill 命名撞车后 ≈ **73%**,44/60)|
| 11-edituse(function-edit/use 等细分)| **46%(33/72)** | 主动激活**错子组**(edit vs use 混)|
- **结论:domain-6 显著优于 11-edituse(62% vs 46%),且 domain-6 从不激活错组**——粗分组更清晰,模型只需判 domain 不用纠结 edit/use。**"6 组不行要 11 组"被推翻;现行 domain-6 是对的。**(当年"11 更好"的真变量其实是 search_* 位置,非 6-vs-11。)
- **未激活的 ~38% 与 6-vs-11 无关,是两个正交可修问题**:① **skill 命名撞车**——用户说"激活技能"→ 模型想直接 `activate_skill`(但它在未激活的 skill 组里够不着),不知要先 `activate_tools("skill")`(11/12 skill 失败全因此);② **search-first-resident**——模型 search(resident)后想直接 edit/run,够不着 lazy 组里的工具又没回头激活。**修法**:教学"先 activate 组再 edit/run" 或后端"调到未激活组工具时自动激活/提示"。**这俩不影响 domain-6 优于 11 的结论。**

### R3 edit-ops G10 验证
edit_function/handler 的 ops 现为裸 `items:{type:object}`(无类型)。A/B(单发,场景给 id):
| | edit_function | edit_handler |
|---|---|---|
| 裸 ops | canonical `code` key 46% | 30% |
| **pin ops 形状**(spell 出 update_code→{op,code})| **66%** | **77%** |
**→ G10 在 edit-ops 上确认:pin 后 code-key 一致性显著抬升**(没 create 的 set_output(0→87)那么戏剧,因 edit 还要"产出正确的新代码")。**规格:edit_* 的 ops 也要逐 op pin 形状(并入 G10)。**

### ★ R3 多轮端到端 at scale(24 个全链路 build episode)—— 最重要产品洞察
deepseek 当工程师,从零搭复杂多实体自动化(recon→forge fn/hd/ag→accept→create_workflow 接线→capability_check→activate),Python 模拟后端(含 G8 反馈:capability_check 真查 ref→报缺失)。3-judge × 24 episode:
| 指标 | 结果 | 含义 |
|---|---|---|
| **all-checks 全 AND 通过** | **0/24 = 0%** | "一次性全对"——不现实的门槛 |
| **per-check 各步通过** | **53% ±5** | 复杂建每项设计决策约一半首发对 |
- **轨迹证据**:模型**走完全链路结构**(search→forge→accept→wire→check→activate,多数 episode 到 activate)+ **接对 ref(G8 反馈版 23/24 workflow 含 ref)**。失败全在**语义架构决策**:实体类型(grader 该 agent 还是 function)、3 路由带 `_default`、polling-function 当数据源 vs case 只比较、每条终态路径都写报告、多字段 when-guard(agent enum + payload 字段组合)。
- **最重要结论**:**复杂多实体自动化无法 one-shot**(0% all-AND = 6-8 项苛刻检查连乘);deepseek 首发给**骨架 + ~一半架构细节**(per-check 53%),其余靠 **`:iterate` 对话(用户审→AI 改)+ G8 兜结构错** 精修。**→ 硬验证产品必须做成"建→审→迭代"对话(N5 的 `:iterate` 流),不是一发入魂。** 简单自动化高得多(R1/R2 composite 真反馈 4/4);**复杂端 = 首草稿需精修,这是产品形态而非缺陷**。
- **G8 反馈的端到端价值**:无 capability_check 反馈版,workflow 接线常空(随机漏接);有 G8 反馈(报缺失 ref)→ 23/24 接对。**capability_check 必须真查 ref 并报缺失(不能恒 ok)——否则模型无从修。**

---

## §4 成本 + 规模

- 被测 DeepSeek 花费:截至本报告 ~¥19 / ¥204(缓存命中极便宜:wave-1 全 96 calls 仅 ~¥0.3)。
- 规模:Round-1 wave-1 n=20×16 + wave-2 7 多轮 + wave-3 34×5 + wave-4 8×8;**Round-2 robustness 20 场景×n=50 + complex 9×n=30 + 多轮 3 episode + newdim 8×n=30** ≈ 累计 **~2500+ DeepSeek calls** + 数百 Claude judge/driver agent。
- 判官/执行 Claude 侧:每 wave/round ~9-54 agent,~0.7-5M token(Max,不计 ¥204)。

---

## §5 残留 + 给 revamp 的建议

0. **🚩 改 case-node 为 `when:` 分支设计(最高价值 de-risk,已实测验证)**:wave-9 实测 case "表达式值==分支键"契约对 LLM 根本性脆弱(布尔条件 0-18%);**wave-10 实测改成每分支 `when:<布尔CEL>` 守卫 → ~100%**(0-18%→~100%,模型对每分支写布尔条件完美,甚至自动 null-safe)。`{fast:{when:"payload.vip||payload.amount>=5000", to:f}, normal:{when:"true", to:n}}`。**对 `04-case-node.md` 的设计修正建议,且已实测确认根治——本研究核心价值:动土前抓出 LLM-hostile 契约并验证修复。**
1. **G1 后端 JSON-repair(必做)**:`infra/llm` 解析 tool 参数加 repair(控制字符容忍 + 括号配平),否则 ~4-8% 复杂 forge 调用失败。
2. **G8 workflow check/fix 回路(必做)**:create_workflow → capability_check + 结构 lint(悬空分支/空payload/冗余边/**不可达节点**)→ next_step envelope 回喂。Round-2 bigwf_etl 70% 的残留(重试回边致 load/deadletter 不可达)正是靠"不可达节点"lint 抓。
2b. **G9 case-node CEL 求值 fail-to-false(建议,降 LLM 负担)**:case 求值器把"guard 求值出错"(如 no-such-key)当 **false** 处理(落到下一分支,最终 `when:"true"` 兜底),而非抛错中断 flowrun。理由:Round-2 实测 temp=默认下模型 CEL **布尔逻辑~全对,但防御性 `has()` 仅 ~50% 一致**;平台 fail-to-false 后,省略 `has()` 的 guard 也安全 → null-safety 不再是 LLM 负担(教学降为 belt-and-suspenders)。cel-go 实现一行(`prg.Eval` 返 err → 该分支判 false)。**对 `04-case-node.md` 求值语义的补充建议。**
2c. **G10 ops/node payload 逐 op/逐 type pin 形状(必做,2 个 A/B 验证)**:所有 ops/node 类锻造工具(create/edit agent·workflow·function·handler + workflow node.config)的 tool schema **逐 op / 逐 node-type 指定 payload 形状 + 判别字段**,**禁裸 `value:{}` / `node.config:{}`**。实测:① set_output_schema 无 pin → 0/30 canonical(裸塞 schema、丢 `kind`),pin 后 87%;② **create_workflow trigger config 隔离 A/B(两臂都有 type 枚举):typed_only 仅 23% 把 cron 串放对字段(73% 放进 `schedule`)→ pin config 形状后 100%**。这是 ag_extract_invoice 68% + content_mod 86% cron 残留的真根因,与 case-contract 同类(动土前在 schema 层 pin 死;后端选定字段名后写死)。
3. **split-tools A/B(已测,n=3/场景)**:split 把 brace-undercount 从 ~4% 降到 **0/9**(G1 缓解确证);语义 mixed(branch_signup 50%→3/3↑,retry_loop↓,clear_triage 持平)→ **split 治 JSON 有效性,非 case-routing 语义银弹;主力仍 G8 check/fix**。复杂图建议 split + check/fix 双上。
4. **fn_csv ~25% code 缺陷**:简单 CSV 解析也有运行时错 → 印证 §6 code 真执行 + run_function 试跑的必要(别信"看着对")。
5. **多轮留足回合**:自修需要回合预算(max_turns 太紧会截断 G8 自修)。

> 结论重申:**de-risk 成立**(Round-2 大样本 + 生产温度复验后仍立)。设计经得起真实模型 + 真实场景。**动土前 schema/平台层四件**:①case-node 改 `when:` 分支、②G1 后端 JSON-repair、③G10 ops/node payload 逐 op/type pin 形状(禁裸 `value:{}`/`config:{}`)、④G9 case 求值 fail-to-false;**运行期一件**:G8 forge test/check-before-accept 回喂(查不可达节点)。**①③④ 是同一主题——把 LLM 易错的契约 pin 在 schema/平台,而非靠模型猜或靠教学补**(三处均已 A/B 实测验证)。
