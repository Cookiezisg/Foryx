# Anselm demo — 原语 / Pattern 覆盖登记

> **单一事实源**：产品每个 UI 范式都必须在此有归宿。**不在册的范式 = 造轮子警报**——先登记、归类，再动手。
> 来源：跨 6 界面 × 后端契约 × design 旧组件的范式覆盖审计（covered 4 · compose 60 · GAP ~20 去重）。
> 配套：[`CAPABILITY.md`](CAPABILITY.md)（后端能力→界面覆盖）· [`README.md`](README.md)（三层强制）。

**状态图例**：✅ covered（已建原语直接是它）· 🔧 compose（拼现有原语、无新件）· 🧩 pattern（专门件）· 🪂 escape-hatch（自绘空间排布、仍吃 token）· ⬚ planned · 🔨 building（Phase 2.5 在建）

---

## 一、原语层（已建，covered）

| 原语 | 是什么 |
|---|---|
| `status-dot` · `badge` | 5 态状态点（状态翻译单源）· 分类/状态药丸 |
| `button` · `input` · `field` · `kv` | 按钮 4 变体 · 输入/多行 · 键值大行（`editable`/`wrap` 就地编辑+多行）· 紧凑键值列（`wrap` 长 value 多行自适应） |
| `section` · `row` · `row-detail` · `page` · `info-card` · `group-label` | 段（`grid` → 响应式 2 列，内化原 render.js 手搓网格）· 核心行（三列网格；`hint` 多行 wrap；`emphatic` accent 选中皮肤[软底+左 accent 条] · `mono` 等宽标签 · `collapsible` 非 passive 行点 chevron 派 an-toggle/点标题派 an-select——树节点分流）· 可展开详情行（点行展开下方详情面板，内化原 render.js 手搓 panel+toggle）· 记录页骨架（`:host` flex:1/min-height:0 自填海面，sea 直返即可滚）· 无边信息卡 · uppercase-meta 小标题单源 |
| 地基 `base.js` | AnElement 基类 + 共享糖：`anEsc`(转义) · `anLabel`(标识符人性化) · **`el(tag,attrs,…kids)`**（元素工厂，attrs 支持 on*/html/prop；全 feature/画廊经 `window.el` 复用，不再各抄——原散在 scheduler/documents/reference 三处） |
| `tabs` · `segmented` | 页级视图切换（隐藏不销毁；实体页概览/版本）· 就地紧凑选项 |
| `floating`(模块) · `menu`(模块) · `mention`(模块) | 锚定浮层引擎 · 菜单 · @ 提及 picker（contenteditable 上 `@`→边打边滤→内联插 `an-ref-pill`；doc-editor / composer 同源，复用 AnMenu/AnFloating/ref-pill） |
| `action-group` · `toolbar` · `ocean-header` | 动作组（`end`/`block`/`stack`/`compact` + `footer` 底部独立动作区变体[尾部带间距，替代各处手搓 margin 裸 div]）· 三段工具条（`bordered` 顶栏 variant）· 海洋页头（`editable` 标题就地改名，派 an-title-change） |
| `right-island` · `sidebar-list` | 右岛内容壳（皮肤与左岛同源 `--shadow-float`/`--r-chip`；`headless` 不画头、交 slot 自绘——entity-workspace 用）· 左岛列表（New[`no-new` 可隐]+域内垂搜[实时过滤·命中祖先链自动展开]+排序+**可折叠大组** chat 式头 + **headless 类型**[无标题省头·scheduler]+**嵌套行树**[row.children 递归·点 chevron 折叠/点标题选中·`add`/`more` 行尾动作·documents 文档树复用此件而非另造]） |
| `code-editor` · `json-tree` | 编辑器块（高亮单源 `AnCodeEditor.highlight`；编辑→保存/取消）· 结构化树 |
| 配置 `config/entity-kinds`(9 kind + `kindIconOf`) · `config/state-model`(`anState`/`anTone`) | 实体类型/图标/动词单源 + 引用→kind 图标派生 · 状态翻译单源 + 状态→徽 tone 单源 |
| schema `schema/{kind-schema,render}` | 声明式实体页（字段型 text/kv/code/json/rows/card + 段 layout:grid + 块 span） |

## 二、Pattern 层（共享件，Phase 2.5）

> **✅ 13 件已全部落地** `demo/core/primitives/`（lint 绿 · reference.html 活体规格台已展示 · 0 console 错误 · 0 missing icon）。

| Pattern | 状态 | 归宿 / 来源 | 哪需要 |
|---|---|---|---|
| `an-dropdown` | ✅ | 移植 design `dropdown.js`（= field + AnMenu） | models/providers/settings/onboarding/workspace |
| `an-ref-pill` | ✅ | 移植 `ref-pill.js`（点击 → Intent.select） | chat @提及 · docs wikilink · mount-health · search |
| `an-tags` | ✅ | 移植 `tags.js`（可增删 chip + health 点） | 实体 tags · skill allowed-tools · agent 挂载 |
| `an-thin-table` | ✅ | 移植 `thin-table.js`（发丝表） | 执行/调用日志 · runtimes · provider 列表 |
| `an-callout` | ✅ | 移植 `attention.js`（警示条 + tone） | workflow attention · env 失败 · 错误态承载 |
| `an-state` + `an-skeleton` | ✅ | 新建（空/加载/错误占位 + shimmer 骨架） | **全 surface（最普遍缺口）** |
| `AnToast` · `AnDialog` | ✅ | 新建命令式模块（floating/menu 族） | 非阻塞反馈 · 确认/表单弹窗 |
| `an-approval-gate` | ✅ | `approval-gate.js` 三 flavor：chat(danger 批准/拒绝) · ask(ask_user 提交/跳过 + options) · durable(flowrun :decide，仅 scheduler) | chat 危险确认 / ask 提问 · flowrun 审批门 |
| `an-run-terminal` | ✅ | 移植 `run-debug.js`（args→流式 stdout→结果） | fn/hd/agent/mcp 试运行 |
| `an-block-tree` | ✅ | `block-kit.js` → 9 块型 transcript（text/reasoning/tool_call/tool_result/progress/compaction/turnEnd/todo/subtree E3）；结果按形态分派(终端/列表/JSON/error 标红) · turnEnd 按 stopReason 分态 · pokeText/pokeLog 逐帧 Delta 流式 | **chat 核心** · agent transcript |
| `an-entity-workspace` | ✅ 🧩 | `entity-workspace.js`（v2，chat 右岛实体工作台 = **entities SSE 流的实体面板镜像**，与对话流 block-tree[messages] 并行双写，**跟着对话长出来**）：自绘头[an-status-dot+真名+下拉钮]（宿主 `an-right-island[headless]` 只给皮肤）+ body 双态[item 态=该 item canonical 全量 facet 的 an-tabs / picker 态=an-segmented 状态筛 + **复用 an-sidebar-list** 分类列表，仅非空分类+搜索]；每种 item（5 实体 kind + Todo + Subagent）一套固定 facet，未触及 facet 显 **an-state 空态**；facet 按 key 分派——code/prompt/graph→code-editor · versions→version-diff(+thin-table) · run→run-terminal · flowrun→node-gantt · trace→block-tree · history/firings→thin-table · overview/config/mounts→info-card+kv(+callout)；命令式 ensure(id)/setActive(id)/focus(id,facet)/setItemStatus/setTodo（auto attr=静态全 ensure）供 sea 持 timer 流式驱动（首个 ensure→sea setRight=出现点，active 跟随最新触发） | chat 右岛（动态多 item 面板 + 下拉分类切换 + 状态机/搜索/筛选 + Todo/Subagent） |

## 三、逃生舱 + 海洋专属 pattern（Phase 3 随海洋建）

| 范式 | 状态 | 归宿 / 来源 | 哪需要 |
|---|---|---|---|
| `an-graph-canvas` | ✅ 🪂 | 已落 `core/primitives/graph-canvas.js`（移植 `graph-lab/`，一骨多态：edit/run × LR/TB；Sugiyama-lite 布局 + 浮动正交布线 + 回边弧；图标走 NODE_ICON、色走 tokens；后端对齐：5 节点 kind、回边只控制/审批发、(node,iteration) 记忆化、:edit ops）。**外设内化**：`framed`(定高 card 框 `--h-graph-preview`)/`toolbar`(悬浮缩放组)/`enterable`(进入编辑器)——render.js graph leaf 退化成一行、编辑器不再重拼缩放。**伴生 `an-kind-legend`**（5 类节点色图例，自 `window.AnGraph` 取数、零属性；`divided` 脚位变体自带 border-top+padding，rail 不再手描；图编辑器 rail + reference 画廊同用，内化原 rail 手搓 flex+拼色） | workflow 图（实体页**定义图**框 edit + 图编辑器海洋 edit·纯编辑无运行态）· scheduler 活运行图（run 态）· relation 邻域图 |
| `an-outline` | ✅ 🧩 | 已落 `core/primitives/outline.js`（文档大纲/ToC：左导引线 + 层级缩进 + 当前节高亮；items/active 属性注入；点条目 emit an-outline-pick → 消费方滚到标题） | documents 右岛大纲 |
| `an-doc-editor` | ✅ 🪂 | 已落 `core/primitives/doc-editor.js`（contenteditable 块编辑，全 demo 唯一自画像素区，**对齐产品密度**：13px 正文/标题阶 t-h3·t-strong·t-body/16px 待办框，非 Notion 放大）：blocks 属性注入（h1/h2/h3·p[spans 含 @ref]·bullet·todo·quote·code·callout[tone]·divider）；四能力——斜杠「/」→ 块类型菜单(AnMenu) · 「@」→ 实体/文档 picker 边打边滤 → 内联插 an-ref-pill · 悬停 ref-pill → 浮信息卡(AnFloating) · 悬停块 → 左槽 ＋ 手柄插块；`scrollToHeading(i)` 供大纲跳转 | documents 所见即所得 |
| `an-heatmap` | 🪂 ⬚ | 新建（日历网格，mock 驱动；后端无聚合端点） | 个人/主页活动 |
| `an-chart` / `an-sparkline` | 🪂 ⬚ | 新建（同件两 mode：有轴/无轴） | 用量/指标趋势 · 实体行内联 |
| `an-version-diff` | ✅ 🧩 | 已落 `core/primitives/version-diff.js`（移植 design `version-diff.js` 的 LCS 纯函数；单框 unified 红绿 diff，行内着色复用 `AnCodeEditor.highlight`；before/after + lang/range/note/bare） | 实体版本 tab（左 an-row 版本轨 + 右本件）· chat 代码 diff |
| `an-wire-list` | ✅ 🧩 | 已落 `core/primitives/wire-list.js`（key→expr 可增删接线行组，复用 an-input；focusout 收集 field→CEL map 派 an-wire-change） | 图编辑器节点 input 映射 · control when→port（an-branch-editor 复用其底座）|
| `an-run-board` · `an-node-gantt` | ✅ 🪂 | 已落（scheduler 专属）：运行看板**只自画 2 列外壳 + 左右同步**，左列每条 run **复用 an-row**（dot 状态 + mono id + trigger·when hint + ↻replay meta + emphatic 选中），右 = 选中 run 的节点甘特；点行 emit an-run-pick · 节点甘特（self-drawn：单 run 逐节点时段条 + 循环 iters 多条 + ×N 徽 + parked 等待框 + future 占位、点行 emit an-node-pick） | scheduler 执行驾驶舱（选 workflow → run 看板 → 运行图 + 节点调试）|
| `an-flowrun` | 🧩 ⬚ | 移植 `flowrun.js`（记忆化条 + park 挂审批） | scheduler durable 节点 |
| `an-branch-editor` | 🧩 ⬚ | 新 pattern（复用 `an-wire-list` + code-editor[cel] + segmented） | control 的 CEL when→port 分支组 |
| `an-search-results` | 🧩 ⬚ | 新 pattern（hit 行 + `<mark>` 高亮安全注入 + 折叠） | search 综搜/垂搜结果 |
| `an-block-kit`（search） | 🧩 ⬚ | 新 pattern（积木接线单元，refHint→填节点） | workflow 编排挑可接线单元 |
| `an-notification-inbox` | 🧩 ⬚ | 新 pattern + **通知类型→{图标,可操作} 单源表** | 需要你 / FYI 两段收件箱 |
| `an-composer` | ✅ 🧩 | 已落 `core/primitives/composer.js`（多行 contenteditable + @ 提及内联药丸[复用地基 `AnMention`] + 附件 chip[可删] + Enter 发送 / Shift+Enter 换行 / generating 切停止；派 an-send{text,html,refs,attachments}/an-stop/an-attach） | chat 输入条 |
| `an-stepper` | 🧩 ⬚ | 新建（线性多步外壳） | onboarding 向导 |

## 四、Compose（无需新件，拼现有原语——约 60 范式，节选拼装规则）

- 实体页主体 = `page` + `section` + `schema/render`（声明 KIND_SCHEMA 一行）
- 版本列表 = `sidebar-list` / `row` + `action-group`(revert)
- 运行历史 / 单条 logs = `thin-table`/`row` + `status-dot` + `badge`(triggeredBy) + `code-editor`(只读 logs)
- mount-health / capability-check = `row` + `status-dot` + `ref-pill`（per-ref 问题行）
- 实体头多状态徽阵列（version/env/config/runtime/lifecycle）= `ocean-header` + `status-dot` + `badge` + `action-group`
- handler config 表单 = `field` + `input`（按 init_args_schema 驱动）+ sensitive 掩码
- 模型/工作区/APIKey 设置 = `field` + `dropdown` + `row` + `menu`(danger 删)
- search 框 = `input`(q) + `segmented`/`tabs`(综搜↔垂搜) + `menu`(类型/标签/时间)
- MCP server 列表 = `row` + `status-dot`(连接态映射 state-model) + `section`
- memory / 文档树 / 通知行 = `sidebar-list` / `row`(label+hint+dot)
- 面包屑 / 大纲 TOC / 反链 = `action-group`(crumb) · `right-island` + `row`(depth)
- 仪表盘 KPI 卡 = `info-card`(大数字 + badge 趋势) + section layout:grid

## 五、登记纪律

1. **任何新 UI 范式动手前**先在此登记 + 标状态/归宿。
2. **不在册 = 造轮子警报**：停，先归类——能 compose 就 compose、该共享就建 pattern、只有空间自绘才 escape-hatch。
3. **compose 范式禁止落新文件**（用现有原语拼，归装配层）；**pattern/escape-hatch 才允许新件**，且必须复用底座：行内高亮走 `AnCodeEditor.highlight`、弹层走 `AnFloating`、状态走 `state-model`、图标走 `entity-kinds`、节点态走 `status-dot`——不重抄。
4. 状态推进：⬚/🔨 建完即改 ✅/🧩 并标"已落 `demo/core/primitives/<x>.js`"。

## 六、已知 token 微调（不阻塞 · 全 lint 绿 · 现用最近 token）

> 移植/新建时遇到无精确 token 的值，已就近用现有 token（lint 通过、视觉可用）。下列若要像素级还原 design，可在 `tokens.css` 补登记后一处替换：

- `--scrim`（dialog 遮罩，现为 `rgba(0,0,0,.28)` 裸 rgba，与 tokens 内 line/shadow 同写法）
- `--d-shimmer: 1.5s`（block-tree / run-terminal 流光，现借 `--d-breath` 1.8s）
- `--line-bold: 1.5px`（thin-table 表头粗线，现借 `--line-2` 2px）
- segmented 轨道 `--inset` / 段缝 `--gap-hair: 2px` / 内距 `--pad-hair: 3px`（现借 `--island-3` / `--line-2` / `--focus-ring`）
- pill 垂直内距 `--pad-pill-y: 2px`（现 `calc(--grid/2)`）
- z-index 阶梯 `--z-float/--z-toast/--z-dialog`（现各模块裸整数 40/60/80，靠人工约定层级）
