# tooltuner — SPEC(技术设计:具体怎么搭)

> v0.2(广度优先,从头重写)。把 [`PRD.md`](./PRD.md) 落成可建的技术契约。复用 `research/llm-experiments/` 代码(迁移后退役)。
> 设计铁律(贯穿全篇):**测量结构服务于「验证」,绝不限制 AI 的「想象」。** 数据 schema 必须能装下:多轴、设计级建议、新维度提案、发散实验——否则就把思路逼窄了。

---

## 1. 目录布局

```
research/tooltuner/
  engine/                      # ② 零件:确定性,与 target 无关
    model_client.py            #   被测模型 client(泛化 deepseek_client;provider 可换)
    run_model.py               #   批量 ReAct(泛化 round3_run;单/多轮 per-experiment)
    score.py                   #   多轴聚合:多数表决 / CI / 弱榜(轴集可扩)
    ab.py                      #   paired-lift:base vs variant 同场景 + 跨工具回归
    gen.workflow.js            #   场景生成 Workflow(泛化 wf_gen_r3;可被 critic 质疑多样性)
    judge.workflow.js          #   判官 Workflow(泛化 wf_judge_r3;按"当前轴集"判,轴可扩)
    memory.py                  #   读写 target 记忆 + schema 守门
  target/                      # ① 记忆:被优化的 Forgify 工具集(三层,见 §2)
    ── 当前真相(人入口,大小不随轮数涨)──
    STATE.md                   #   此刻快照:每工具每轴现分 + 当前 best + 待办 top + 已转 N 轮(每轮末重生成)
    CONCLUSIONS.md             #   durable 结论:已证真理(G 式)/ known-good / 别再 re-litigate
    ROUNDS.md                  #   轮次索引:一轮一行(NNNN·日期·目标·头条·花费)
    ── 交付物 + 活状态 ──
    surfaces/                  #   当前 best LLM-facing 面:tools.json / system_prompt.md / teaching.md / examples.md / grouping.json
    axes.json                  #   当前质量维度集(起步 selection/usage;可加)
    backlog.json               #   活的待办:open[] + known_good[]
    recommendations.md         #   设计级建议(拆/并/删/加/重设计 schema)——给人,不自动改
    changelog.md               #   只记"被采纳的 surface 改动"(交付物 provenance,短)
    scores.jsonl               #   机器读时间序列:每轮每工具每轴(喂趋势,人不读)
    config.json                #   被测模型 / backend / domain hint / judges_n
    ── 过程(不可变胶囊,可 GC)──
    rounds/0001/               #   round.md(统一模板)/ scenarios.json / traces/ / verdicts.json
          0002/ ...
  PLAYBOOK.md                  # ③ 方法论文档:AI 跑一轮前读它(见 §4;不是 skill,就是 md)
  PRD.md
  SPEC.md
```

> **engine 与 target 解耦** = 配置即换靶。**`axes` / `recommendations` 两个文件是本次重写的关键** —— 它们让"新维度"和"设计级改动"有地方落,思路才不被两轴 + 文本微调框死。

---

## 2. 记忆:信息架构(转 N 轮不糊的关键)

**铁律:三个时间尺度分开,过程别淹当前真相。**

### 2.1 三层
| 层 | 文件 | 性质 |
|---|---|---|
| **当前真相**(人入口) | `STATE.md` · `CONCLUSIONS.md` · `ROUNDS.md` | 大小**不随轮数涨**;STATE 每轮重生成 |
| **交付 + 活状态** | `surfaces/` · `axes.json` · `backlog.json` · `recommendations.md` · `changelog.md` · `scores.jsonl` | 维护型(覆盖 / curated),非无限 append |
| **过程**(不可变,可 GC) | `rounds/NNNN/` | 每轮一胶囊,封存不改 |

### 2.2 各文件契约
- **`STATE.md`** —— 从数据**重生成**的快照:每工具每轴现分(取 scores 最新)+ 当前 best surfaces 指针 + backlog top + 已转 N 轮 + 累计花费。**不手改**(防漂移)。
- **`CONCLUSIONS.md`** —— durable、curated,三类:① 已证真理(G 式,带证据轮次)② known-good(查实没问题、别再碰)③ 已采纳设计变更追溯。**靠"提升"而来,去重。**
- **`ROUNDS.md`** —— 索引,一轮一行 `NNNN | 日期 | 目标 | 头条结果 | 花费`。
- **`axes.json`** —— `[{key, name, how_judged, added_round, why}]`,起步 `selection`/`usage`,可 append。
- **`backlog.json`** —— `{open:[{id, kind, target, note, ts}], known_good:[...]}`;**kind ∈** `weak/hunch/transfer/lever/coverage/reprobe/redesign/new_axis/scenario_gap`(后三个是反窄入口)。
- **`scores.jsonl`** —— 每行 `{round, ts, tool, axis, pct, n, ci, surfaces_hash}`(按轴一行;`surfaces_hash` 防跨版误比)。
- **`recommendations.md` / `changelog.md`** —— 如 §1 注(设计建议给人 / 只记采纳的 surface 改动)。
- **`rounds/NNNN/round.md`** —— 统一模板(§2.3)+ `scenarios.json` `traces/` `verdicts.json`。

### 2.3 `round.md` 统一模板(每轮读起来一个样)
```
# Round NNNN — <一句话目标>
类型: exploit | explore | meta
信号/假设: <…>      方法: <单/多轮 · n · 测哪些轴>
读 raw: <原始 trace 看到啥>      判定: 真 | 假象
动作: 改 surface <which> | known-good | 设计建议 | 立新轴
结果: <paired lift per 轴 + 跨工具回归>      下一步: <backlog 更新>
```

### 2.4 清干净的收尾铁律(一轮不算完,直到)
1. 写 `rounds/NNNN/round.md` + raw(封存,不再改)
2. **重生成 `STATE.md`**;append `scores.jsonl`;更新 `backlog.json`
3. durable 学到的 → 提进 `CONCLUSIONS.md`(去重)
4. `ROUNDS.md` += 一行
5. 有采纳的 surface 改动才 → `changelog.md` += 一行

### 2.5 GC(不肿)
- STATE/CONCLUSIONS/ROUNDS 大小恒定(ROUNDS 一行一轮,几百行也能扫)。
- 唯一会肿的是 raw `traces/` → **留最近 N 轮 + 被 backlog/争议结论引用的**;更老 `rounds/` 只留 `round.md`,traces 删。认知 + 磁盘都有界。

---

## 3. 零件接口(I/O 契约)

| 零件 | 输入 → 输出 | 职责 |
|---|---|---|
| `model_client.chat(...)` | → `{content, tool_calls, reasoning, cost}` | 单次调被测模型;并发/重试/预算账本/畸形 JSON 修复/content-leak/reasoning-echo(现成) |
| `run_model(surfaces, scenarios, config)` | → 写 `rounds/NNNN/traces/` | 批量 ReAct;**单/多轮 per-experiment**;backend 模拟;移植 faithful-echo(防合成 id 污染) |
| `score(traces, verdicts, axes)` | → `{per_tool_per_axis:{pct,n,ci}, weak[]}` | **按 axes.json 多轴**聚合(确定性) |
| `ab(base, variant, scenarios, config)` | → `{per_tool_per_axis: lift, regression[]}` | **同场景**两版对比;**改全局面时跨多工具回归** |
| `gen.workflow(catalog_subset, domain_hint, n, diversity_critique?)` | → `scenarios[]` | Claude 造 ≥n 个互不相同场景;可传"上轮 critic 指出的盲区"逼多样 |
| `judge.workflow(run_dir, rubrics, axes, judges_n, decorrelate?)` | → `verdicts` | 3 判官对抗,**按当前轴集**逐轴判,多数;**可去相关**(第二判官换模型 / 多样 prompt,让判官盲区 ≠ 优化者盲区) |

---

## 4. PLAYBOOK.md(③ 方法论 —— 皇冠,大纲)

> **不是 Claude Code skill**。就是 `research/tooltuner/PLAYBOOK.md`;**偶尔想优化时让 AI「读 research/tooltuner/,跑一轮」即可。**

段落:
- **何时调 + 记忆怎么读**(先读 PLAYBOOK + axes + backlog + scores)。
- **每轮两动作**:**① 想宽**(多信号 + 元层面)→ **② 验稳**(读 raw → A/B 或 设计建议)。
- **质量底线(血换的)**:读 raw 验信号、只信 paired-lift、判官默认怀疑、改全局面跨工具回归、11 个测量假象、G0-G10 已知坑;**真执行 > 判官**(code 跑子进程为准)、**绝对分 = Claude 审美只信相对**、**能去相关就上第二判官**(见 §9 信任边界)。
- **反窄机制(PRD §7 落地,这是本版重点)**:① 轴可生长 ② 结构/设计提案进 `recommendations.md` ③ explore vs exploit(**每若干推至少一发散**:从零重写 / 狂想 / best-of-N 框架锦标赛)④ 场景 critic ⑤ 反 Goodhart + outside view ⑥ **元认知殿后**(收尾自问:是不是在局部爬山?)。
- **怎么用 §3 零件 + 预算花完即停 + 没好题就停。**

---

## 5. 一次"推"端到端(映射到零件)

1. 读 `PLAYBOOK` + `STATE` + `axes` + `backlog`
2. **想宽** → 挑一个实验:exploit(局部改)/ explore(发散:重写/狂想/锦标赛)/ **元**(质疑某轴、某场景盲区、某工具设计)
3. `gen.workflow`(或复用 `rounds/`)→ `run_model` → `judge.workflow`(当前轴)→ `score`
4. **读 `rounds/NNNN/traces/` 原始失败**(PLAYBOOK 要求)→ 信号真假?
5. **分叉(四种都算一次成功的推)**:
   - 信号真 + 可 A/B → 草 variant → `ab`(+跨工具回归)→ 真赢 → 写回 `surfaces/` + `changelog`
   - 信号假 → `backlog.known_good` += 记录
   - **设计级**(拆/删/加/重设计)→ `recommendations.md` += 建议
   - **新维度** → `axes.json` += 新轴 + `backlog` 立项下轮测
6. **清干净收尾(§2.4)**:封存 `rounds/NNNN/` 胶囊 → 重生成 `STATE` → 提 `CONCLUSIONS` → `ROUNDS` +1 行 →(采纳才)`changelog` → **元认知收尾**(局部爬山?)

---

## 6. 第一个 target:Forgify seeding(P1)

- `surfaces/` ← `spec_catalog.py`(tools)+ `wave1_gen.SYSTEM`/doc 13 §2(system_prompt/teaching)+ doc 13 §4.5/doc 15(examples)+ domain-6(grouping)
- `axes.json` ← 起步 `selection` + `usage`
- `backlog.open` ← doc 14 弱榜,**标"待验,非定论"**(假象教训);**外加几条 `redesign`/`new_axis`/`scenario_gap` 种子,逼飞轮一开始就不只盯低分**
- `recommendations.md` ← 空

---

## 7. 复用 vs 新写

- **泛化复用**:`deepseek_client`→`model_client`;`round3_run`→`run_model`;`wf_gen_r3`→`gen`;`wf_judge_r3`→`judge`;judge agg→`score`。
- **新写**:记忆 schema + `memory.py` + `ab.py`(含跨工具回归)+ **多轴 `score`** + **`axes`/`recommendations` 机制** + `PLAYBOOK.md`(含反窄方法论)+ surfaces 移植。
- **退役**:迁移验证后删 `research/llm-experiments/`。

---

## 8. 构建阶段(增量可交付)

| P | 内容 | 验收 | 烧 token? |
|---|---|---|---|
| **P1** | 记忆三层架构(STATE/CONCLUSIONS/ROUNDS + rounds/ + axes/recommendations)+ seed Forgify target | 文件齐、parse 过、STATE 能重生成 | 否 |
| **P2** | 泛化零件 `model_client`/`run_model`/多轴`score` + `gen`/`judge` Workflow 参数化 | 对 1 工具跑通 gen→run→judge→score(两轴) | 少量 |
| **P3** | `ab.py`(跨工具回归)+ `PLAYBOOK.md`(**含反窄方法论**) | ab 出 paired-lift;playbook 可读可跟 | 否 |
| **P4** | 首轮真迭代(小预算):**至少含一个 exploit + 一个 explore/元 实验** | 各类产出各落一条(改赢 / 假象 / 设计建议 / 新轴) | 小预算 |
| **P5** | 退役 `llm-experiments/` | 删除 + 引用清理 | 否 |

---

## 9. 信任边界 + 待确认风险

**信任边界(这是闭环,诚实划线 —— 决定能宣称什么):**
- 全回路 Claude 出场景 / 判 / 改 → **绝对分是 Claude 的审美,不是生产真相**。**只信相对 / paired**,绝不宣称"到 X%"。
- **真执行是唯一硬 ground truth**:function/handler 跑子进程为准,这块**别信判官**;workflow/agent 能真跑就真跑。
- **judge 去相关**:graph 这种没法真跑的,第二判官换模型 / 多样 prompt,让判官与优化者盲区不重叠。
- **唯一外部锚 = 真实使用分布**(从 Forgify 对话日志抽场景)→ **产品有真用户后才补**;在那之前"合成 + 判官"是**已知上限,不是缺陷**。

**待确认 / 风险:**
- **backend 保真**:多轮模拟必须移植 faithful-echo(合成 id 污染是踩过的坑)。
- **预算账本**:沿用 `/tmp` per-pid ledger(macOS TCC 教训)。
- **反窄 vs 收敛的张力**:explore / 元实验更贵更慢、未必每次有 paired 赢 —— PLAYBOOK 要给"发散占比"和"何时收"的判断指引,别两头落空。
