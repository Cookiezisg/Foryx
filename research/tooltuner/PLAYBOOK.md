# tooltuner — PLAYBOOK(跑一轮前读这个)

> 你(Claude Code agent)是飞轮的司机。读完这篇,你就能对一个 target 推一轮 = 一次实验。
> 不是 skill,不自动加载;用户让你「读 research/tooltuner/,跑一轮」时,你读它。
> 背景见 `PRD.md`(为啥)/ `SPEC.md`(架构 + 数据契约)。被测:`target/config.json` 的模型。

---

## 0. 信条(先记住,别违反)

**测量只约束「能 verify 什么」,不约束「能 imagine 什么」。** 你想得宽(改任何 LLM-facing 面、质疑工具设计/指标/场景、立新维度);paired-lift 只当**诚实闸门**。
**绝对分是 Claude(你)的审美,不是生产真相。只信相对 / paired。** 真执行 > 判官。没有「做完」,只有「这轮值不值」。

---

## 1. 零件(在 `engine/`,Python 用 Bash 跑,Workflow 用 Workflow 工具)

| 零件 | 怎么用 |
|---|---|
| `memory.py` | `import memory as mem`;`mem.target_dir("forgify")` / `start_round` / `record_scores` / `write_capsule` / `finalize_round`(重生成 STATE+ROUNDS)/ `promote_conclusion` / `append_changelog` / `append_recommendation` / `gc_traces` |
| `run_model.py` | `run_model.run(td, scenarios, round_dir, tool_names=[...], config=cfg)` → traces(写 `rounds/NNNN/traces/`;真执行 code)|
| `score.py` | `score.score(traces, verdicts, axes)` → `{rows, weak, per}`(真执行覆盖 usage)|
| `ab.py` | `ab.ab(base_verdicts, variant_verdicts, traces, axes)` → `{per, wins, regressions}`(McNemar 配对显著性)|
| `gen.workflow.js` | Workflow 工具跑,args `{targetDir, roundDir, tools, n}` → 各 agent 写 `roundDir/scenarios/<tool>.json` |
| `judge.workflow.js` | Workflow 工具跑,args `{targetDir, roundDir, axes, judgesN, decorrelate}` → return verdicts(**你把它写到 `roundDir/verdicts.json`**)|

> 想零 token 确认管线没坏:`python3 engine/smoke_mock.py`。

---

## 2. 一轮 = 两个动作(全篇的纲)

### ① 想得宽(imagine — 无边界)
读 `target/STATE.md` + `backlog.json` + `axes.json`。**挑一个实验**,来源不限于低分:
- `weak` 可疑低分(**先验真假**,多半是测量假象)· `hunch` 好工具能不能更好 · `reprobe` 复核旧分
- `transfer` 把某个赢搬给兄弟工具 · `lever` 没试的杠杆(few-shot / 架构守则 / 自一致 / reflexion)
- `coverage`/`scenario_gap` 盲区 · `redesign` 工具该拆/并/删/加/schema 重设计 · `new_axis` 新维度(效率/校准…)
- **explore vs exploit**:别每轮都局部微调。隔几轮强制一发散(从零重写某描述 / 狂想 / best-of-N 框架锦标赛)。

### ② 验得稳(verify — 上闸门)
1. `start_round(td)` → `(n, round_dir)`。
2. 造场景:跑 `tooltuner-gen`(或复用旧 `rounds/`)。合并 `scenarios/*.json` 成一个 list。
3. `run_model.run(...)` → traces(真执行 code 的 `exec_result` 就是硬 ground truth)。
4. 判分:跑 `tooltuner-judge` → verdicts,写 `round_dir/verdicts.json`;`score.score(traces, verdicts, axes)`。
5. **🔴 读 raw**:打开 `round_dir/traces/*.json` 看失败原文,聚类。**先问:这低分是真的,还是判官过严 / 场景不公 / search-first 假阴性?**(见 §4 的 11 假象)。
6. **分叉(四种都算一次成功的推)**:
   - **信号真 + 可 A/B** → 改一个 `surfaces/` 文件成 variant → 对同场景 run+judge variant → `ab.ab(base_v, var_v, traces, axes)` → **`wins` 非空且 `regressions` 空** → 把 variant 写回 `surfaces/` + `mem.append_changelog(...)`。改全局面(system_prompt/teaching)→ judge 要覆盖**多个工具**看回归。
   - **信号假**(其实没事)→ `backlog.known_good` += `{tool, reason, round}`,移出 open。
   - **设计级**(拆/删/加/重设计)→ `mem.append_recommendation(...)`(给人,不自动改工具集)。
   - **新维度** → 往 `axes.json` 加一条 `{key, name, how_judged, added_round, why}`(meta 也要有依据,别凭空加),立 backlog 项下轮测。
7. **清干净收尾(SPEC §2.4,缺一不可)**:`record_scores` → `write_capsule`(填 §3 模板)→ `finalize_round`(重生成 STATE/ROUNDS)→ durable 的 `promote_conclusion` → 更新 `backlog.json` → 该 GC 就 `gc_traces`。

---

## 3. `round.md` 胶囊模板(写满,每轮一个样)

```
# Round NNNN — <一句话目标>
类型: exploit | explore | meta
信号/假设: <…>            方法: <单/多轮 · n · 测哪些轴 · offer 哪些工具>
读 raw: <原始 trace 看到啥 + 聚类>
判定: 真 | 假象(为啥)
动作: 改 surface <which: before→after> | known-good | 设计建议 | 立新轴
结果: <ab per 轴: base%→var% lift,significant?,regressions?>
下一步: <backlog 增删>
```

---

## 4. 血换的质量底线(违反 = 这轮作废)

1. **下结论前读 raw**。低分先当"可疑信号"不当"真相"。R1-R4 抓出 **11 个测量假象**,典型:工具选择 35%→真 91%(正确 search-first 被冤);`fp_status_poll` 67% 其实对;`km_knowledge` 50% 是模型正确反问。**"看起来差的常常已经很牛逼了"。**
2. **只信 paired-lift,不信绝对分**(run-to-run 抖 ±15)。`ab` 的 `significant` 不为真,不算赢。
3. **真执行 > 判官**:code 跑子进程为准(`exec_result`),`score`/`ab` 已自动覆盖。
4. **判官默认怀疑 + 多数**;没法真跑的(graph)用 `decorrelate` 第二判官换框架,别让判官盲区 = 你的盲区。
5. **改全局面跨多工具回归**;`ab.regressions` 非空就别合(帮了 A 伤了 B)。
6. **改教学/描述要回归测副作用**(宽措辞修矛盾把正常请求也反问过:100→47;务必紧措辞)。
7. **meta 也要 gate**:新轴 / 设计建议必须带证据,别让 breadth 绕过 rigor。

### G0-G10 先验(已证,别重推;细节见 doc 14)
G0 教学 prompt 能可复现推高语义率 · G1🔴 deepseek ~4-8% 吐畸形 JSON(后端 JSON-repair,已在 model_client)· G2 复杂锻造 max_tokens≥16000 · G3 thinking 按任务别全关 · G4 search-first 是默认且对(单轮指标会冤)· G5 实体消歧靠描述 · G6 关键守则**殿后**(deepseek 末尾遵守度最高)· G7 错误带 next_step 才自纠 · G8🔴 first-draft 有缺陷靠 test/check 回喂(capability_check 必须真查 ref)· G9 case guard 平台 fail-to-false · G10🔴 ops/node payload 必须 pin 形状。

---

## 5. 信任边界(决定能宣称什么)

闭环:你出场景、你判、你改。所以 —— **绝对分是你的审美,不是生产真相;只报相对 / paired,绝不说"到 X%"。** 唯一硬 ground truth 是**真执行**(code);唯一外部锚是**真实使用分布**(Forgify 对话日志)—— **产品有真用户后才补**,在那之前"合成 + 判官"是已知上限,不是缺陷。

---

## 6. 何时停

预算花完即停(`model_client.cumulative_cost_rmb()` 看总花费,共享 ¥200)。或:这个 target 的弱点都验过、改动 paired 收益压平 → 在 `STATE` 标记"成熟",转 episodic(工具集有实质变动时再来)。**不为转而转。**
