# Round-4 过程笔记 — API-only 优化实验(7 项)

> 目标:deepseek 是直接 API(不能自部署约束解码、不能微调 v4-flash)。在此约束下,测 7 种纯 API 手段能把弱表面(复杂 create_workflow 首发 52%)推多高。
> 攻击靶子:`/tmp/r3complex/create_workflow.json`(60 个难场景,flash baseline 语义判官 52%)。复用 R3 的判官 + 场景。
> 结论收口进 doc 13/14;本文件是过程 + 原始数。框架:`research/llm-experiments/round4_*.py + wf_verify.py + wf_judge_r4*.js`。

---

## 枢纽事实(查证)
- DeepSeek API **有 `strict:true` 函数调用**(beta 端点)——服务端约束 args 匹配 schema(API 版约束解码),但有已知畸形-JSON bug → strict + JSON-repair 兜底两个一起上。`response_format` 只支持 `json_object` 不支持 `json_schema`。
- **更强模型可用**:`deepseek-reasoner`(R1)、`deepseek-chat` 都能直接 API 调 → 可做模型分层。
- 微调 v4-flash:DeepSeek 无此 API,out。

## 实验设计 + 结果

| # | 实验 | 手段 | 结果(复杂 create_workflow,baseline 52%)|
|---|---|---|---|
| ① | best-of-N + 验证 | 采 N=5,程序化结构验证器挑最优 | **n1 80% → bestN 83%(paired +3pt)**;结构选择器小幅(因结构已~满分,选不出语义差异)|
| ⑤ | 自一致性投票 | 同 N=5,众数结构签名挑 | **n1 80% → selfcon 87%(paired +7pt)**;赢过结构 best-of-N——众数=模型最有把握的结构,与正确性更相关 |
| ③ | Reflexion 自我批判 | forge 后加一轮自审→修正 | **orig 64% → revised 71%(+7pt)**;1 轮自审抓部分语义错,便宜+真实+modest |
| ④ | few-shot gold 示例 | 系统 prompt 塞 1 个正确范例 | **75%**(fresh baseline ~64%,**≈+11pt**)→ 超预期:gold 示例演示了架构形态(不只结构),连语义都带起来,最便宜杠杆之一 |

> ⚠️ **baseline 校准**:R3 的 52% 与本轮新跑有 run-to-run + 判官方差;本轮**最干净的 fresh baseline = reflexion 的 orig 64%**(无任何干预)。few-shot/tiering 的绝对 % 按此 ~64% 基线读 lift;reflexion/best-of-N 是各自内部 paired 对比(更干净)。
| ⑥ | 模型分层 | 复杂用 deepseek-reasoner(R1)| **67%(60/60 正常 tool-call),仅 +3pt vs fresh baseline 64%,却 10× 成本(¥4.65 vs ¥0.5)→ 不值!** 差距是 Forgify 约定非原始智能,强通用模型不更懂 Forgify |
| ⑦ | `:iterate` 回路 | 建→用户修改意见→模型修 | **修正正确 16/24=67%**;应用修改 75%;**保住其余没clobber 96%**(23 用 edit_workflow 增量改 ✓)→ **产品流程成立**|
| ② | mini-GEPA(我当 mutator)| 读语义失败,写"架构决策守则"教学,held-out 验 | **base 65% → GEPA-V1 75%(paired +10pt)**;针对语义架构的显式守则有效(虽降结构 100→80,语义净 +10);与 few-shot 相当 |

## 中间观察(已出的)
- **iterate**:模型 23/24 正确选 **edit_workflow 增量改**(而非重建)——选型对路,具体改对没看判官。
- **reflexion**:58 个里 21 个(~36%)自审后真改了输出——说明自审有触发修正,质量净变化看判官。
- **best-of-N 结构验证器**:n1 baseline 结构 pass 77% → bestN ?(挑最优应升);但结构是语义子集,真实 lift 看语义判官。
- **GEPA**:Claude(我)读训练集失败 trace → 提改进教学 → held-out 验。

## ★★ 枢纽发现(重构了整个 R4 的预期)
GEPA baseline 拿失败 trace 时,读原始输出发现:**模型的 workflow 结构其实几乎完美**。
- 我的结构验证器原有 bug(要求每个 case 分支都有 `to`),但**终止分支** `{"when":"true"}` 正确地省略 `to`(到此结束)——这是设计允许的。修 bug 后:**结构 pass = 100%(train 20/20)/ 95%(baseline 60)**,残留仅 2 个缺 default + 1 个 no-ops。
- **→ 模型已经把"结构"(when 守卫 / 不悬空 / 终止分支 / 重试 emit)做对了 ~95-100%。那 52% 语义判官分的差距,全在语义架构决策**:该用 agent 还是 function、路由逻辑对不对、polling 当数据源还是 case 当分析师、多字段守卫组合。**结构层 API 手段(best-of-N 结构选择器 / few-shot / GEPA-on-结构)对此基本没头部空间。**
- **能动语义的 4 个杠杆**:① 更强模型(reasoner 做更好的架构决策)② 语义选择器的 best-of-N(不是结构选择器)③ reflexion(语义自审)④ iterate(人在环修)。
- **第 11 个被 read-raw 抓出的测量 artifact**(这次是我自己的验证器 bug)。

## ★★★ R4 综合结论(7 实验全出)

**两条总命题:**
1. **deepseek-v4-flash 已把"结构"做对了 ~95-100%**(when 守卫 / 不悬空 / 终止分支 / 重试 emit)。复杂建那 ~50-65% 的差距**全在语义架构决策**(agent-vs-function、polling-vs-cron、case 别当分析师、多字段守卫、每路径有动作)。
2. **能动语义的杠杆都便宜;贵的(更强模型)反而没用。** 按 paired lift × ROI 排:

| 杠杆 | paired lift | 成本 | 评 |
|---|---|---|---|
| **few-shot gold 示例** | ~+11pt | ~免费(1 例进 prompt)| 🥇 演示完整架构形态,最佳 ROI |
| **GEPA 架构守则教学** | +10pt(n=20)| ~免费 | 🥈 显式语义守则;与 few-shot 互补(演示 + 明说)|
| **自一致性(采 N 挑众数结构)** | +7pt | N× 采样 | 🥉 众数=模型最有把握,与正确性相关 |
| **reflexion 自审一轮** | +7pt | 1 额外轮 | 便宜,抓部分语义错 |
| **best-of-N(结构选择器)** | +3pt | N× 采样 | 结构已满分→选不出语义差异;选择器要语义才有用 |
| **模型分层(reasoner R1)** | +3pt | **10× 成本** | ❌ 不值;差距是 Forgify 约定非原始智能 |
| **`:iterate` 回路** | 67% 正确 / 96% 不破坏 | 1 轮 | 产品完成机制(建→审→改),成立 |

**3. 推荐(API-only 把复杂建推到最高):叠加便宜的语义杠杆**——系统 prompt 同时放 ① gold 示例 + ② 架构守则,复杂建时 ③ 采 N 挑众数 + ④ 自审一轮 → 预期复杂建 85%+;剩下靠 ⑦ iterate 对话兜。**别上更强模型(+3@10×,不值)。结构侧另配 strict:true(API 版约束)+ JSON-repair。**

**4. 方法论 caveat(重要)**:绝对判官分 run-to-run 抖 ±15pt(R3 52% / R4 fresh 64-80%)——这是文献证实的 LLM-judge 宽严方差。**只信实验内部 paired lift,不信跨实验绝对值。** 故上表用 paired;few-shot/tiering 的 lift 按同轮 fresh baseline(~64%)估。

**5. 自查**:R4 又抓 1 个 artifact(我的结构验证器误要求终止分支带 to)——读 raw 修正后才看清"结构已满分,差距是语义"。累计 11 个测量 artifact 被自查纠正。
