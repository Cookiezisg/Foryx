---
id: WRK-026
type: working
status: active
owner: @weilin
created: 2026-06-18
reviewed: 2026-06-18
review-due: 2026-09-16
audience: [human, ai]
landed-into:
---

# Iteration Loop —— AI 操作手册（START HERE）

> AI：读完这个文件夹你就能跑这个 loop。本文件 = 怎么跑 + 怎么判 + 铁律。
> [`ARCHIVE.md`](ARCHIVE.md) = **探过什么 + 还有什么**（覆盖归档 + frontier，EXPLORE 起手读它）。[`TASKS.md`](TASKS.md) = 跑什么（索引表）。[`LOG.md`](LOG.md) = 已发现什么（finding 索引）。
> 仓库根 `/Users/SP14921/Documents/Personal/PersonalCodeBase/Foryx`；真模型 key 在根 `.env`（`DEEPSEEK_API_KEY`，deepseek-v4-flash）；运行器 `testend/golden/selfiter_*_test.go`。

## 一句话

让真 agent 在真模型上跑真任务 → 抓整条轨迹 → 你（Claude）当判官判「工具用得对不对 + 整套工程转得对不对」 → **有就都修** → 重跑同测看前后 → 记一行 → commit。一条延绵不断、线性的迭代线。

**两个灵魂：**
- **① 多轮对话，不是一问一答。** **你（Claude）亲自扮演用户**，带一个目标跟 agent 聊 **6-9 轮**——它做错你纠正、它问你答、它绕了你引导，综合判整段。（当前标准目标驱动用户即可，暂不做多人设。）
- **② 判要进后端 query。** 回复说"已触发"不算，查 flowrun 真跑上没——后端状态才是 ground truth。

## 架构：EXPLORE 并发 / EXPLOIT 串行
- **EXPLORE**（找问题）= 多 agent 并发，各探一个没覆盖的方向（每个 agent 也亲自演用户跑多轮）。
- **EXPLOIT**（CONFIRM→GENERALIZE→FIX→VERIFY→LOG→COMMIT）= 回主 agent，串行，一个问题彻底解决（含同类一起解决）再开下一个。
- 线性的是骨干（LOG 一行一行长大）；并发只在发现的爆发期。

## 循环 = 8 拍

| 拍 | 名 | 干什么 |
|---|---|---|
| 1 | **REVIEW** | 起手先读本 README，刷新操作模型（防漂移、加深印象）。 |
| 2 | **EXPLORE** | **开放式发现引擎**（下节）：看 [`ARCHIVE.md`](ARCHIVE.md) → 想（脑暴新探针）→ 挑（novelty × value）→ 多 agent 并发跑**多轮对话** probe + 后端查询 → 报候选。 |
| 3 | **CONFIRM** | 多变体各跑 1-2 次，**跨变体一致复现才算真**，出现一次不算。 |
| 4 | **GENERALIZE** ★ | **修任何东西前的第一反射**：独有还是**范问题**（不止这里）？读共享层 / 同类工具确认范围。 |
| 5 | **FIX** | 直接修，**有就都修**（不设"值不值得"闸）。范 → 批量 + 修地基一处；独有 → 只修这处。定位到层。 |
| 6 | **VERIFY** | 重跑确认它的那些测 + 后端 ground-truth，前后对比。**能转成零 token 结构断言的就转**（回归守得便宜）。改进必须可见，否则回拍 5。 |
| 7 | **LOG** | 在 [`LOG.md`](LOG.md) 表里追一行（索引，不写 essay）。 |
| 8 | **COMMIT** | **一个 fix = 一个 commit**：fix + 回归 test + LOG 行同提交，专用分支。 |

回拍 1，开拓新方向。**永不"做完"**——见下节的 NEVER-DONE 不变式；唯一外部上限是 API 没额度（见「停止信号」）。

## EXPLORE 引擎 —— 开放式发现（看 → 想 → 挑 → 探 → 归档）

> 拍 2 的展开。**心智：这题无限发散，loop 不是填一张 checklist，是一台"永远看测了什么 → 想还有什么 → 挑最值得的 → 探 → 归档"的发现引擎。** 

**三层记忆（都不是 checklist）：**
1. **回归套件 = 硬记忆**：锁死的格、零 token 免费复查、**探针永不回碰**。
2. [`ARCHIVE.md`](ARCHIVE.md) **= 软记忆**：探过的格（含**绿格**：探了没缺陷）+ 结局签名 + frontier。命中和未命中都记——失败也是覆盖记忆，免重问死问题。
3. **开放描述符网格**（ARCHIVE §1）：轴随时可加；空格/薄格 = backlog。

**每轮 select 仪式：**
1. **看** —— 读 ARCHIVE（哪里覆盖、哪格薄、frontier 在哪）+ 回归套件。
2. **想（生成）** —— 脑暴一批候选探针，**双偏置**：(a) **价值** = 朝「一个真 agent 在哪儿最可能卡/被误导/白烧/找不到/恢复不了/等，不要局限」——`promise≠reality` 是最锋利的镜头，但**不止它**；(b) **多样** = 朝 ARCHIVE 的空格/薄格 + 没碰过的 arity/镜头。**并允许发明新轴**（元新颖）。
3. **挑（过滤三段）** —— ① 砍掉"需要不存在的功能"和"必绿无信息"的（Minimal Criterion）；② 砍掉与归档**结局签名**太近的（换皮不算新）；③ Claude 当兴趣模型按 **novelty × value** 排序，取本轮 fanout 几条。无聊的降权不删（rare re-probe 仍可能）。
4. **探** —— 多 agent 并发，各演用户跑多轮 + 查后端（拍 2 原样）。
5. **归档** —— 写结局签名 + 一句教训进 ARCHIVE；缺陷 → EXPLOIT（拍 3-8）修 + shrink 最小复现 + 锁零 token 回归（填格，可能长新轴）。

**主裁判是 Claude，不是机械 diff（这是 agent 产品）：** 价值是**判断**——从 agent 视角 holistic 看「这算不算真的痛」，镜头开放可加（ARCHIVE §1）。两条护栏：
- **事实永远锚 DB ground-truth**：value 的*证据*必须引后端真相（到底成没成、真实状态），不是浮空的 LLM 软分（《AI Scientist》教训：软兴趣分任意、没人用）。**Claude 提议/排序，DB 裁决事实。**
- **绝不优化单标量**："最多 bug"是欺骗性目标、必坍缩到反复找同一个（Lehman-Stanley）。novelty 与 ground-truth value 联合；新轴/空格自动优先（AFLFast 反频）。

**跨domain迁移：** 任一实体确诊一个缺陷类，**立刻把同类探针迁移到其余实体**（F20 meta-in-row 跨 5 实体即此），改思路不止是实体，其他东西也类似。

**NEVER-DONE 不变式：** loop 只在**没有任何候选探针同时"对 ARCHIVE 新颖"且"可锁成回归"**时才停——对一个还在长的产品，这永不发生（DeepMind 形式化 open-endedness）。"做完了"在构造上不存在。

## DOC-ALIGN —— 里程碑全量文档对齐（per-fix sync 的安全网）

每个 fix 已在 COMMIT 同提交做 1:1 doc-sync（铁律 #6 / CLAUDE.md #9）——第一道防线。但一批改动里**跨面的 drift 会漏**（如某轮把 `api.md` 漏了、靠对抗审查才逮到）。故**里程碑**（一批 fix 后 / 周期性）跑一次**全量对齐**：

- **审计扇出**（多 agent 并发、每面一个、只读）：`api.md` / `database.md` / `error-codes.md` / `events.md` / `domains/*` / `foundation/*` / `overview.md` 各自 diff 文档 vs 权威代码——端点 / 表·列·CHECK·索引 / 错误码 + **计数** / SSE 事件 / 实体字段 / op / 文档化行为，逐项核（**计数必须真去数代码**，别信文档自报）。本轮改最多的面（trigger / workflow 并发 / control·approval / function / mcp / scheduler / cel）**优先审**。每 agent 报 confirmed drift（doc 位置 + doc 说什么 + 代码真相 `file:line` + 精确修法）。
- **修**：回主 agent **串行**修 drift（high→low），`make docs` 绿，`docs(loop): align <面> 到代码` 精确 commit。
- **判据**：reference 文档 = 代码的**精确投影**（rule #9），任何事实不符 = bug（与编译失败同级）；只修事实 drift、不碰文案偏好。
- **机制**：审计与修都是 Workflow 扇出（EXPLORE 并发审 / EXPLOIT 串行修），与主 loop 同骨架。

## 怎么操作（具体 —— 照着做就能跑）

**① 一次性 · 起真后端 + 配 deepseek**（用我们自己的后端，不是测试 harness）
```bash
make server                  # 真后端：端口 8742、数据 /tmp/anselm-dev（持久）；停用 make stop
bash testend/loop/setup.sh   # 等 health → 建 workspace + deepseek api-key + 默认模型 → 写 serve.json
```

**② 每个 probe · 一段多轮对话，你（Claude）亲手演用户**
> **关键：每一轮都是你亲手发的。** 你看 agent 这回合干了啥，自己 decide 下一句。`turn.sh` 只是把「发一句 + 等回合到终态 + 放行危险门 + 打印 agent 干了啥」这套 curl 收成一个命令——**说什么是你定，不是脚本、不是另一个模型。**
```bash
testend/loop/turn.sh new                            # 开新对话 → 打印 CONV=cv_xxx
testend/loop/turn.sh cv_xxx "<你的开场，来自任务目标>"   # 打印 agent 这回合：CALL/RSLT/TEXT
#  ↑ 你读完，亲手写第 2 轮（纠正它 / 澄清 / 推进 / 说 done）：
testend/loop/turn.sh cv_xxx "<你的下一句>"
#  … 6-9 轮，直到目标达成或卡住
```

**③ 判 · 整段 + 后端 ground-truth**（回复说的不算，查后端真相）
```bash
B=$(jq -r .baseURL /tmp/anselm_selfiter/serve.json); W=$(jq -r .workspaceId /tmp/anselm_selfiter/serve.json); H="X-Anselm-Workspace-ID: $W"
curl -s "$B/api/v1/functions"      -H "$H" | jq '.data'                                              # 实体真建了没
curl -s "$B/api/v1/functions/<id>" -H "$H" | jq '.data | {name,description,tags,activeVersion:.activeVersion.version}'  # 名/meta/版本真对没
curl -s "$B/api/v1/flowruns/<id>"  -H "$H" | jq '.data'                                              # flowrun 真 advance / 恢复没
```
> **⚠️ N1 envelope：真相在 `.data`，先 unwrap**（忘了就全 null，会错判）。
你对整段轨迹按 RUBRIC 出判词 + finding。**F6 就是这步逮到的**：agent 说改了名，`.data.name` 没变。

**④ 回归**：多轮里你的用户各轮消息**脚本化**进一个 Go test（用户侧固定、agent 侧真模型），`testend/golden/selfiter_*_test.go`，`EVALS=1 … go test … ./golden/...` 后台跑；结构性 finding 优先转**零 token 断言**。

## RUBRIC（判官，判整段对话非单轮）
六维度各 1-5，**每分必引证据**（轮次/block seq / tool_call 内容 / 后端查询结果）：工具选择 · 参数 · 顺序 · **恢复**（容忍 double-check，**跨轮收敛即满分**）· 效率（含 token）· **系统终态**（query 后端，确定性事实，最硬）。
混合判官：确定性事实用 code 判；模糊质量用 LLM 判。**finding** = 「第 N 轮/步做了 X，该 Y，因为 Z；坏在<哪层>；建议<怎么改>」——定位到层才知道改哪个文件。

## 铁律
1. **GENERALIZE 先于单修**（拍 4）：修任何东西前先问"范问题吗"、读共享层。修一类别一个个磨。
2. **有问题就都修**（不设值不值得闸）。
3. **防自欺靠机制**：CONFIRM 跨变体 + VERIFY 前后 + 后端 ground-truth，三者皆客观。判官 ≠ 被测。
4. **绝不改预期/断言刷绿**；预期错了改预期并在 LOG 注明。
5. **无回归 test 的修 = 没修完**（拍 8 进永久集；能零 token 结构断言就优先用它）。
6. `make verify` 必绿；改文案触动契约 → 同提交改 `references/`（+ Co-Authored-By 尾注）。
7. **commit 在专用分支**（不动 `main`）；消息 `fix(loop): F<n> <一句话> [范围]`。

## 停止信号
loop **结构上永不"做完"**（NEVER-DONE 不变式，见 EXPLORE 引擎节）——**唯一外部停止信号 = deepseek API 报额度/余额耗尽**（model 调用持续因无额度失败）。此时**不撂挑子**：把当前卡在一半的修做到**干净态**（`make verify` 绿 + 该 commit 的 commit 掉），再收工等下一步。**绝不留半坏状态。**

## 文档规范（强制 —— 这些表会无限增长）
LOG / TASKS 是**索引表非 essay**：一条 = 一行，每格一短语；详情进 commit/test，不进表。违反（写成段落、重复别处已有事实）= 文档腐烂，立刻压回一行。

## 文件 / 已知 gap
文件：`README.md`（手册 + EXPLORE 引擎）· [`ARCHIVE.md`](ARCHIVE.md)（覆盖归档 + frontier）· [`TASKS.md`](TASKS.md) · [`LOG.md`](LOG.md) · 操作脚本 `testend/loop/{setup,turn}.sh`（起后端后用）· 回归 `testend/golden/selfiter_*_test.go`。
已知 gap：判官是裸的你 + 单模型（自评偏差，靠后端 ground-truth + 前后对比兜）；回归套真模型成本（尽量转零 token 结构断言）；ARCHIVE 描述符/结局签名目前**手工维护**（靠纪律，无自动 embedding 去重——单人规模够用，量大再自动化）。
