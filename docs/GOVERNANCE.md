---
id: DOC-000
type: concept
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# Forgify 文档规范（Documentation Governance）

> 本文件定义本仓库**全部**文档如何被创建、组织、同步、淘汰。**它是强制规范，与代码纪律（`CLAUDE.md`）同级。**
> 本文件自身遵守它定义的一切规则（frontmatter / 类型 / 生命周期），作为标准的活样板。

---

## 0. 强制性与执行设计（先读这条）

文档规范若不被执行，等于不存在。本规范用**三层冗余**确保 Claude（及人）完整遵循——任一层兜住，三层叠加几乎不漏：

| 层 | 机制 | 作用 |
|---|---|---|
| **① 常驻** | `CLAUDE.md`（每次会话**自动加载**、工程纪律唯一事实源）内嵌「文档纪律」节：核心条款 + §7 触发表精要 + §12 收尾清单 | 使 Claude **每次会话都已读到**这些规则——**无「不知道」借口** |
| **② 规范** | 本文件：具体到 **if-then** 的触发表（§7）、可逐条勾的收尾清单（§12）、机械可判的门禁项（§11） | 把「文档要同步」从空泛原则变成**可机械执行**的指令 |
| **③ 门禁** | `make lint-docs`（§11，CI 兜底）机械校验 frontmatter / 必填 / immutable / 孤儿链接 | 捕捉人或 AI 的疏漏，**让违规无法静默通过** |

**三条铁律（违反 = 严重 Bug，与编译失败同级）：**

1. **文档与代码物理同步**：改了代码却没在**同一提交**同步对应文档 → 这次改动**未完成**。文档落后于代码 = Bug。
2. **触发即停**：任何时候发现文档与代码不符，**立刻停下修文档**（记 `[doc-fix]` dev log），再继续原任务。
3. **不确定就查本规范**；本规范没覆盖的 → 按 §1 核心原则推导，并**回头给本规范补一条**（规范随项目生长）。

---

## 1. 核心原则

1. **文档-代码物理同步（doc-code parity）**：`reference` 类文档必须与代码**逐字**对得上。代码是事实，文档是其精确投影。
2. **单一事实源**：每个事实只在**一处**权威记录（见 §10 权威层级）；他处引用、不复制。
3. **零历史包袱**：项目未上线。文档**只描述当前物理事实**，禁止「曾经如何、后来改成」的演化叙述（历史在 git 与 `archive/`）。
4. **写 Why 不写 What**：What 看代码/结构即知；文档的价值在解释**为什么这样设计**、有哪些取舍与边界。
5. **高密度**：表格优先、要点优先、删一切 fluff（「本节将介绍…」之类）。本规范自身即范例。
6. **中文**：所有文档正文用**中文**；代码标识符、路径、wire code、frontmatter 字段名保持原文。
7. **状态即重述（state 文档整体重述、非追加）**：描述「当前状态」的 `concept` 文档（`architecture.md` / `CLAUDE.md` / 本规范）每次变更**必须整体重述到当前事实**——改一个状态/事实 = 重写相关部分，使全文读起来像「一直如此」，旧状态**不留痕迹**（历史在 git）。**绝不在旧内容旁追加**；只增不删 = 文档腐烂之源。两种更新 mode 不混：`reference` 文档按 **§1.1 精确同步**（投影代码），`concept`/state 文档按**本条整体重述**。

---

## 2. 文档类型（6 类）

每篇文档必须在 frontmatter 声明**唯一** `type`，它决定写入规则、审阅周期、淘汰协议。

| `type` | 用途 | 可变性 | 审阅周期 | 目录 |
|---|---|---|---|---|
| `concept` | 架构解释、设计理念、心智模型 | 随系统演进 | 季度 | `concepts/` |
| `reference` | 必须与代码精确一致的契约/规格 | **每次代码改动即同步** | 每次代码改动 | `references/` |
| `how-to` | 分步操作手册 | 流程变更时更新 | 半年 | `how-to/` |
| `decision` | ADR——为何选 X 不选 Y | **不可变**（只新建 supersede、绝不编辑） | 永不 | `decisions/` |
| `log` | 时间序进度/决策日志 | **仅追加** | 永不 | `references/changelog.md` 等 |
| `working` | 在研、临时、过程性 | 落地前活跃 | **90 天上限** | `working/` |

---

## 3. Frontmatter 标准

除 `INDEX.md`（入口，豁免）与 `archive/`（只读墓地，豁免）外，**每篇 `.md` 必须**以下列 frontmatter 开头：

```yaml
---
id: DOC-NNN          # 唯一编号，创建时分配（见 §4）
type: concept        # §2 六类之一
status: active       # draft | active | superseded | deprecated | archived
owner: @weilin
created: YYYY-MM-DD
reviewed: YYYY-MM-DD  # 最近一次人工审阅
review-due: YYYY-MM-DD # 下次到期审阅（= reviewed + 该类型周期）
audience: [human, ai]  # 读者：human / ai / 二者
superseded-by:        # status→superseded 时填，指向取代它的 DOC-id/路径
landed-into:          # 仅 working：结论提取进哪篇 concepts/references 后填
---
```

`status` 缺失或非法值、`type` 非六类之一、必填字段为空 → `lint-docs` 失败（§11）。

---

## 4. 命名与 ID

- **ID**：`DOC-NNN`，三位递增、全仓唯一、创建即分配、**永不复用**。`DOC-000` = 本规范。
- **文件名**：`kebab-case.md`。
  - `reference` 域文档名 = 其对应代码资源域（如 `domains/function.md` 对应 function 模块），便于 1:1 对照。
  - `decision` 文档：`NNNN-简短标题.md`（如 `0021-durable-vs-eventsourcing.md`），`NNNN` 为 ADR 序号、与 `DOC-NNN` 独立。
- **目录归属由 `type` 决定**（§2 表末列），不得错放。

---

## 5. 目录地图（canonical）

```
docs/
├── INDEX.md              ← AI 会话入口（≤50 行，§11 强制）
├── GOVERNANCE.md         ← 本规范
├── concepts/             ← 稳定的架构解释（concept）
├── references/           ← 必须与代码同步的契约（reference）
│   ├── backend/          ← api.md · database.md · events.md · error-codes.md · changelog.md
│   │   └── domains/      ← 每个后端域一篇 <domain>.md
│   └── frontend/         ← fsd-layers.md · entity-types.md · cross-cutting.md
│       └── slices/       ← 每个 FSD slice 一篇 <slice>.md
├── decisions/            ← ADR，仅追加、不可变（decision）
├── how-to/               ← 操作手册（how-to）
├── working/              ← 在研，≤90 天（working）
└── archive/              ← 只读墓地（被取代/终止的文档，豁免 frontmatter）
```

- 每个文件夹放且仅放其声明类型的文档；空文件夹用 `.gitkeep`（内含一行职责说明）占位。
- 新增类别 = 先在本 §5 + §2 登记，再建目录。

---

## 6. 生命周期

```
draft → active → superseded → archived
              └→ deprecated → archived
```

| 状态 | 含义 | 规则 |
|---|---|---|
| `draft` | 起草中、未生效 | 非权威，不得被他处依赖 |
| `active` | 权威、单一事实源 | 唯一可被依赖的状态 |
| `superseded` | 被更新文档取代 | 填 `superseded-by`，随后移 `archive/` |
| `deprecated` | 主动淘汰中 | 标记后移 `archive/` |
| `archived` | 只读，住 `archive/` | **不可再改**；历史/参考用 |

`decision` 不走「superseded→改」——ADR 不可变，被推翻时**新建**一篇 ADR 并把旧篇 `status` 置 `superseded`、`superseded-by` 指向新篇。

---

## 7. 同步触发表（★ doc-code parity 的执行点）

**改了左列代码 → 必须在同一提交更新右列文档。** 这是「文档=代码精确投影」落地为可机械执行的 if-then：

| 代码改动 | 必须同步的文档 |
|---|---|
| 新增/改 API 端点 | `references/backend/api.md` + 对应 `domains/<域>.md` |
| 新增/改 DB 表或列 | `references/backend/database.md` + 对应 `domains/<域>.md` |
| 新增/改 error code | `references/backend/error-codes.md` + 对应 `domains/<域>.md` |
| 新增/改 SSE 事件 | `references/backend/events.md` + 对应 `domains/<域>.md` |
| 架构决策（选型/取舍） | `decisions/` 新建一篇 ADR |
| 架构 / 分层 / 实体 / 引擎 / 路线状态变更 | **整体重述** `concepts/architecture.md` 相关节（§1.7，非追加） |
| 工程规则 / 设计原则 / 契约宪法（N·D·E·S·T）变更 | **整体重述** `CLAUDE.md` 相关节（§1.7，非追加） |
| 前端实体类型变更 | `references/frontend/entity-types.md` |
| FSD 层级规则变更 | `references/frontend/fsd-layers.md` + `CLAUDE.md` 前端节 |

**两种更新 mode 不混（§1.1 vs §1.7）**：`reference` 文档行 = **精确同步**（增量改到逐字吻合代码）；`architecture.md` / `CLAUDE.md` 行 = **整体重述**（把相关节重写到当前状态、删尽旧状态，绝不在旁追加）。表为高频清单、非穷举——「代码改了而某文档因此失真」一律适用。

---

## 8. 写作规约

- **语言**：正文中文；标识符/路径/wire code/frontmatter 键保持原文。
- **密度**：表格 > 列表 > 段落。删除 meta 废话、礼貌性过渡、「显然」「众所周知」。
- **只写 Why**：解释设计动机、取舍、边界、坑。What（有哪些字段/端点）让结构自述或链向 reference。
- **零历史叙述 + 重述维护**：不写「原来…后来改成…」，当前事实 only（演化进 git / `decisions/` / `archive/`）。维护 state 文档（architecture.md / CLAUDE.md / 本规范）时**整体重述、非追加**（§1.7）——改个状态就把相关节重写到当前，**不在旁边堆新句、不留旧状态**。
- **交叉引用**：用相对链接指向权威源（`[api.md](../references/backend/api.md)`），**不复制**内容——复制即制造第二事实源、必然 drift。
- **删/移文档**：必须同时修掉所有指向它的链接（`INDEX.md` 及他处），不留孤儿链接（§11 校验）。

---

## 9. working 文档协议

`working/` 文档**最长 90 天**。落地时：

1. 把结论提取进对应的 `concepts/` 或 `references/` 文档（那才是权威源）。
2. frontmatter 填 `landed-into:` = 目标文档路径。
3. `git mv` 该文件到 `archive/`。
4. 若 `INDEX.md` 引用过它，更新 `INDEX.md`。

超过 90 天且 `landed-into` 为空的 working 文档由 `make lint-docs` 标记。

---

## 10. 权威层级

文档冲突时，**高者胜**：

```
CLAUDE.md  >  references/  >  concepts/  >  working/  >  archive/
```

`CLAUDE.md` 是工程纪律最高法；`reference` 因「必须等于代码」而高于解释性的 `concept`；`archive/` 最低（仅历史）。

---

## 11. 质量门禁（`make lint-docs`）

`lint-docs` 应作为 `make verify` 的一环，机械强制：

1. 所有非 `archive/`、非 `INDEX.md` 的 `.md` 都有合法 frontmatter，且必填字段齐。
2. `type` ∈ §2 六类；`status` ∈ §6 五态。
3. `review-due` 已过期 → **警告**（不阻断）。
4. `working/` 文档 >90 天且 `landed-into` 空 → **失败**。
5. `decisions/` 文档创建后被改（git blame）→ **失败**（ADR 不可变）。
6. `INDEX.md` ≤ 50 行。
7. 无孤儿链接（文档内相对链接指向的文件都存在）。

> **现状**：`lint-docs` 尚未在 backend-new 实现（构建体系重置中）。在它落地前，§12 收尾清单是 Claude 的人肉门禁；落地后二者并行。本节即其规格。

---

## 12. Claude 收尾清单（★★ 每次代码改动「完成」前逐条勾）

声明任何代码改动**完成**之前，逐条自检——任一项未过 = 改动**未完成**，回去补：

1. ☐ 这次改动碰了 §7 触发表里的东西吗（API / DB / error / SSE / 架构决策 / 架构·实体·引擎状态 / 工程规则·N·D·E·S·T / 前端类型 / FSD）？→ 对应文档**同一提交**更新了吗？
2. ☐ 改的是 `reference` 文档吗？它和代码**逐字**对得上吗（端点/字段/码/事件 一一吻合）？
3. ☐ 改的是**状态文档**（`architecture.md` / `CLAUDE.md` / 本规范）吗？→ 是**整体重述到当前状态**吗（没在旧内容旁追加、没留旧状态痕迹，§1.7）？
4. ☐ 新建文档有合法 frontmatter（§3）吗？`type`/`status`/`id` 对吗？放对目录（§5）了吗？
5. ☐ 删/移过文档吗？→ 所有指向它的链接（`INDEX.md` 及他处）都修了吗（无孤儿链接）？
6. ☐ 动过 `decisions/` 里的 ADR 吗？→ **禁止**（只能新建 supersede）。
7. ☐ working 文档落地了吗（提取进 concepts/references + 填 `landed-into` + 移 `archive/`）？

---

## 13. 修改本规范

本规范是 `concept` 文档，随项目演进。改它需：① 更新 `reviewed`/`review-due`；② 若改了执行机制（§0/§7/§12），**同步更新 `CLAUDE.md` 的「文档纪律」节**——二者必须一致，否则常驻层与规范层冲突、执行即失效。
