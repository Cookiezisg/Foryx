# 14 — LLM 验证研究记录(workflow-revamp:我都研究了什么)

> **这是一篇"研究做了什么 + 结论是什么"的标准化记录。** 配套 [`13-llm-facing-implementation-guide.md`](./13-llm-facing-implementation-guide.md)(那篇是"你该做什么")。
> 目的:在 revamp 动 20+ 天工程前,用平台默认便宜模型 **deepseek-v4-flash** 把每个 LLM-facing 表面(工具描述 / schema / 系统 prompt)真刀真枪验证一遍,de-risk。
> 原始研究稿 + 逐轮迭代日志归档在 [`research-archive/`](./research-archive/);实验框架代码在 `research/llm-experiments/`。
> 累计花费 ~¥43 / ¥204 预算(缓存极省);~8000+ 次真实 deepseek 调用 + 数千次 Claude 判官/驱动 agent。

---

## §0 一句话结论

**revamp 设计配 deepseek-v4-flash 是 work 的。模型能力不是瓶颈——契约设计(把易错的东西 pin 在 schema/平台)+ test/check 回喂机制才是。** 动土前 5 件事(case 改 when:、ops/node pin 形状、后端 JSON-repair、case 求值 fail-to-false、forge 后真查-ref 的 capability_check + 回喂)+ 1 条产品形态认知(复杂自动化是迭代对话不是一次入魂)。详见 doc 13。

---

## §1 方法论(为什么这些数能信)

1. **真组装 prompt**:完整系统 prompt(identity/how-to/tool_conventions/能力菜单/critical-rules)+ 真工具 schema,不是塞单条命令。
2. **真 ReAct 多轮**:Claude subagent 当后端(动态返真实结果 + 注错)+ 用户(澄清)+ 裁判,不喂 canned 结果。
3. **code 真执行**:function/handler 生成的 Python 子进程实跑(mock 外部依赖),分 clean/runtime_error/wrong_output 三档。
4. **语义裁决**:每个产物 3 个 Claude 判官对抗式审(默认怀疑)+ 多数表决。**不靠结构对蒙混过关。**
5. **生产真实度**:temp = 默认(≈ 生产);大样本(per-表面 n=50-200);95% CI。

---

## §2 三轮实验概览

| 轮 | 测什么 | 规模 |
|---|---|---|
| **Round-1** | 皇冠 forge 表面(单决策)+ 多轮 ReAct + 91 工具选择 + utility + 难度梯度;迭代教学 prompt | 数百调用,n=6-20 |
| **Round-2** | 大样本复验(生产温度 + n=50-100 + CI)+ 复杂场景 + 多轮复杂 + 新维度 + G8 恢复曲线 + 矛盾需求 | ~2500 调用 |
| **Round-3** | **全 91 工具 × ≥50 个不同场景**完整覆盖(5202 场景)+ 复杂批 300 + 端到端全修复 + lazy 6vs11 + edit-ops + 多轮端到端 at scale | ~5000 调用 |

**Round-3 核心方法**:Claude 按工具 author **5202 个互不相同的场景**(不是重复跑同一个),deepseek 2-4 轮 ReAct 每个跑一次,Claude 语义判官逐场景判。这才是"覆盖/泛化",不是"可靠性"。

---

## §3 全部数字

### 3.1 逐表面正确率(Round-1/2,of-attempts = 剔除合理澄清)
| 表面 | 正确率 | 验证模式 |
|---|---|---|
| create_handler(复杂状态机)| 简单 100% / 复杂 code-exec 93% | CODE 真执行 |
| create_function(复杂算法)| 88-90%(code-exec 58/60 clean)| CODE 真执行 |
| create_agent | 90%(陷阱教学 17→95)| ARTIFACT |
| CEL case(布尔路由)| **旧设计 0-18% → when: 设计 ~100%** | ARTIFACT |
| create_workflow(大图 10-20 节点)| 42-52% | ARTIFACT |
| 多轮 ReAct(诊断/跨实体/恢复)| 5-6/7 | 真多轮 |
| 端到端 composite(简单自动化)| 4/4 | 真多轮 |
| Utility/CONTENT | 100% | CONTENT |

### 3.2 Round-2 大样本 robustness(n=50,temp=默认,±95%CI)
- **100%**:expense_approval · ag_router · when_compound · when_3way · fn_dedup · fn_validate_email
- **92-98%**:nullguard · fn_workdays · cache_ttl · branch_signup · backup_retry · oauth
- **62-70%(真弱区)**:lead_scoring 70 · ag_extract_invoice 68 · fp_status_poll 67 · **hd_ratelimit 62**(令牌桶算法)

### 3.3 Round-3 全 91 工具覆盖(5202 场景,语义判官)
- **覆盖达标 100%**:91/91 工具各 ≥50 个不同场景。
- **SELECTION(选对工具)82%**:lifecycle/runtime/diagnosis 100、agent 94、document 88、workflow 84、function 83、handler 79、mcp 74、skill 72、memory/base 67。
- **USAGE(用法正确)**:create/read/discovery 类 83%(44/62 ≥85%);act-简单(revert/delete/lifecycle)98-100%;act-复杂(edit 产 ops/accept/call)~47%(真偏低 → G10 pin edit-ops + G8 兜)。

### 3.4 Round-3 复杂批(300 难场景,code-exec + 判官)
| | 正确率 |
|---|---|
| create_function(复杂算法)| 88%(58/60 clean)|
| create_handler(滑窗/令牌桶/连接池)| 93%(56/60)|
| cel_when(复杂守卫)| 97% |
| create_agent | 73%(被 G1 malformed JSON ~17% 拖累)|
| create_workflow(10-20 节点大图)| 52% |

### 3.5 Round-3 端到端 at scale(24 个全链路 build episode)
- **一次性全对(all-AND)= 0/24** · **每项设计决策(per-check)= 53%**
- 模型走完全链路结构 + 接对 ref(真查-ref capability_check 版 23/24),失败全在语义架构决策(实体类型/路由/polling/多字段守卫)。**复杂自动化无法 one-shot → 产品是迭代对话。**

### 3.6 三个专项 A/B
| 专项 | 结果 |
|---|---|
| **Lazy domain-6 vs 11-edituse** | domain-6 激活对组 62%(从不激活错组)> 11-edituse 46%(细分搞混)→ **保持 domain-6** |
| **G8 恢复曲线**(temp=默认,令牌桶)| first-draft 17% → 1轮 71% → 2轮 88% → 3轮 88%(plateau);**~2 轮收敛,残留 ~12% 需 escalation** |
| **矛盾需求修复** | baseline 0% 识别 → 宽规则 100% 但过度反问正常请求(不可上线)→ 紧条件规则 ~85% + 正常 100%(可上线)|

---

## §4 全部死结论(G0-G10)

| # | 结论 | 证据 |
|---|---|---|
| **G0** | 改教学 prompt 能可复现推高语义率 | 不可能能力 17→95;lazy 4→1;矛盾 0→85 |
| **G1** 🔴 | deepseek ~4-8%(复杂 agent ~17%)吐畸形 JSON,Go 默认拒 → **后端必须 JSON-repair** | json_repair 配平恢复 100% |
| **G2** | 复杂锻造 max_tokens ≥ 16000(8k 截断)| 8k→16k 截断 1/96→0/320 |
| **G3** | thinking 别全局关,按任务;多轮回传 reasoning_content | 复杂任务 on 更好 |
| **G4** | search-first 是模型默认且正确;单轮指标会冤枉它 | 多次自查纠正假阴性 |
| **G5** | 实体消歧靠描述(91 全集 ~82-91%);分类器→agent / 知识库→document 需补强 | — |
| **G6** | 关键守则殿后(recency,与 OpenAI 相反)| 角色禁令殿后有效 |
| **G7** | 错误带 `next_step` 模型能自纠,裸 prose 则乱试 | recover 3/3 |
| **G8** 🔴 | forge first-draft 有缺陷,靠 test/check-before-accept 回喂恢复;**capability_check 必须真查 ref** | 令牌桶 17→88;端到端真查-ref 23/24 接对 |
| **G9** 🟡 | case guard null-safety 应平台 fail-to-false,非 LLM 负担 | 布尔逻辑~全对但 has() 仅 ~50% 一致 |
| **G10** 🔴 | ops/node payload 必须 pin 形状(schema 或描述),禁裸无类型 | set_output 0→87;trigger cron 23→100;edit-ops 46→66 |

**核心机制需求(动土前)**:case 改 when:(消除 #1 脆弱)+ G1 + G10 + G9 + G8。**统一主题:把 LLM 易错的契约 pin 在 schema/平台,别靠模型猜或教学补。**

**额外真发现(Round-3)**:
- **Lazy domain-6 优于细分** —— "6 组不行要 11"被推翻(当年真变量是 search_* 位置)。
- **Subagent / AskUserQuestion 欠选择**(模型倾向自己干 / 散文列取舍,不委派/不调结构化工具)—— 若产品依赖,需教学触发或后端兜。
- **recon-over-commitment** —— 模型改已有实体前反复核实甚至循环;"信息够了就执行"nudge 大幅改善。
- **复杂自动化无法 one-shot**(端到端 per-check 53%)—— 产品必须迭代对话。

---

## §5 方法论严谨:自查纠正 10+ 个测量 artifact

**很多"低分"其实是测试脚手架的锅,不是模型差——靠"下结论前读原始输出 + 回归测副作用 + faithful 重测"逐个抓出:**
- wave-3 工具选择 35%(假阴性,实为正确 search-first)→ 纠正 91%。
- 复杂 CEL timewindow/multifield 初判 17%/30%(判官过严要 has()/嫌 `||`≢`in`)→ 逻辑导向重判 100%。
- km_knowledge 50%(漏给 search 工具 → 模型正确反问)→ 非弱点。
- fp_status_poll 67%(转换逻辑实正确,harness 跑不了外部 HTTP)→ 非弱点。
- **合成 id 污染**:act-on-existing USAGE 低分,因合成 recon 返通用 id 被模型采纳(59%)→ faithful 重测(echo 场景真 id)后 revert 13→100。
- 矛盾修复宽措辞"0→100%"看似干净 → 回归暴露过度反问正常请求(100→47)→ 收窄成紧条件。

**结论:假信心(假阳/假阴)比没信心更坏;本研究的数经得起自查。**

---

## §5.5 Round-4:API-only 怎么把复杂建推更高(7 实验)

> 约束:deepseek 是直接 API(不能自部署约束解码、不能微调 v4-flash)。靶子:复杂 create_workflow(R3 首发 52%)。过程在 `research-archive/round4-api-optimization-notes.md`。

**枢纽发现:模型的"结构"已做对 ~95-100%**(when 守卫/不悬空/终止分支/重试 emit);复杂建那 ~50% 差距**全在语义架构决策**(agent-vs-function、polling-vs-cron、case 别当分析师、多字段守卫、每路径有动作)。

**7 个 API-only 杠杆,按 paired lift × ROI:**
| 杠杆 | paired lift | 成本 |
|---|---|---|
| few-shot gold 示例(1 例进 prompt)| **~+11pt** | ~免费 🥇 |
| GEPA 架构守则教学(我当 mutator 进化教学)| **+10pt**(n=20)| ~免费 🥈 |
| 自一致性(采 N 挑众数结构)| **+7pt** | N× 采样 |
| reflexion 自审一轮 | **+7pt** | 1 轮 |
| best-of-N(结构选择器)| +3pt | N× 采样 |
| 模型分层(reasoner R1)| +3pt | **10× 成本 ❌ 不值** |
| `:iterate` 回路(建→改)| **67% 正确 / 96% 不破坏** | 1 轮 |

**结论**:① **能动语义的杠杆都便宜(示例/守则/采样/自审 各 +7~11pt);贵的更强模型反而没用(+3@10×)**——差距是 Forgify 约定不是原始智能,强通用模型不更懂 Forgify。② **DeepSeek API 有 `strict:true`**(beta,服务端约束 args 匹配 schema,有畸形-JSON bug → 配 JSON-repair)= 结构侧的 API 版约束解码。③ **绝对判官分 run-to-run 抖 ±15pt(LLM-judge 宽严方差,文献证实)→ 只信 paired lift。** ④ 又抓 1 个 artifact(我的结构验证器误判终止分支)→ 累计 11 个自查纠正。

---

## §6 怎么复现(框架文件)

实验框架在 `research/llm-experiments/`:
- **驱动**:`deepseek_client.py`(带预算 ledger)/ `ds_turn.py`(单轮原语)/ `wave1_gen.py`(系统 prompt + parse_args)。
- **catalog**:`catalog_v2.py` / `spec_catalog.py`(91 工具 schema)。
- **Round-2**:`round2_*.py`(robustness / complex / 矛盾 / G8 曲线)+ `wf_judge_r2*.js`。
- **Round-3 覆盖**:`wf_gen_r3.js`(生成 5202 场景)+ `round3_run.py`(2-4 轮 ReAct)+ `wf_judge_r3.js` + `r3_coverage_result.json`(91 工具逐项)。
- **Round-3 专项**:`round3_lazy_ab.py` / `round3_editops_ab.py` / `round3_e2e_run.py` + `wf_judge_e2e.js` / `round3_pinned_wf.py`(G10 合规 schema 样例)。
- 跑法:`DEEPSEEK_API_KEY` 设好,`python3 <script>.py`;判官是 Claude workflow。

> 详细逐轮迭代日志(每个发现怎么找到的)在 `research-archive/` 的旧 13/14 稿;本文是收口的标准化结论。
