# Foryx 布局语法规范（Layout Grammar）

> 版本 0.1 · 草案,待你研究后定稿。
> 本文是 Foryx 桌面端 UI 的**宪法**:所有面板、海洋、组件、未来的 Flutter 实现都从它派生。
> 写法:**铁律 + 正例(✅) + 反例(❌)**。每条规则尽量同时给出"值/原语"(怎么强制)。

---

## 目录

- §0 哲学与三件套(为什么这样写)
- §1 不变的地基(海洋 + 三岛 + 六契约)——只确认,不改
- §2 Token 层:密度 / 字阶 / 间距 / 圆角 / 动效 / 色(值 + 用哪个的铁律)
- §3 页面骨架:海洋结构 / 三岛 / 内容宽 / 滚动 / 页头
- §4 原语库:Page / Section / Field / Row / SidebarList / Toolbar / Button / Input / Menu / Tabs / Badge / RightIsland
- §5 内容 schema 与「文档边界」:记录模型 / 逃生舱
- §6 交互规约:hover / selected / focus / disabled + 两个同槽互换 + 字色铁律
- §7 命名与第七契约:强制怎么落 + 绑 Flutter
- §8 `design/` 目录规划 + 重做路线

---

## §0 哲学与三件套

**核心命题**:一致性 = 约束的副产品。自由度越少,越不可能做丑。Foryx 好看不靠每个面精雕,靠"丑布局根本表达不出来"。

**三件套(缺一不可)**:

| 层 | 角色 | 落点 |
|---|---|---|
| **文档**(本 SPEC) | 解释 why + 正反例。给人看,但不强制。 | `design/SPEC.md` |
| **token** | 所有数值的**唯一出处**。想硬编都没地方写。 | `design/core/tokens.css` |
| **原语 / 组件** | 把规则**烧进** `Page/Section/Row/…`。拼它就自动对,想错都难。**真正的强制层。** | `design/core/primitives/` |

> 反例(❌):只写一份规范 PDF,靠自觉遵守 → 必然漂(`demo/` 里同一种"行"写了 5 遍,行高 32/34、间距 4/5/8/9/11/15 全在飘,就是因为没有原语兜底)。

**边界(很重要)**:规范覆盖**记录型 / 列表型 / 表单型**界面(实体页、设置、文档、通知、对话流)——这些是"文档型",80% 的面。**空间型界面**(运行图、编辑器画布)走**逃生舱**:自定义实现,但**必须吃同一套 token + 活在标准页骨架里**,所以照样"是 Foryx 的味道"。规范不追求"能表达一切"——那会膨胀成又一个 CSS,丢掉约束本身。

---

## §1 不变的地基(只确认)

`demo/` 这套大骨架**保留,不重新发明**:

- **海洋(ocean)** = 中央激活的工作面。
- **三岛**:**左岛**=侧栏(可被 settings/notifications 接管)· **中央**=海洋 · **右岛**=跟着海洋走的抽屉(海洋切走即清)。
- **manifest 装配**(append-only,一文件一主人)· **Intent**(选中路由)· **Live**(messages/entities/notifications 三流)· **组件库**(`fg-` 前缀,自载 CSS)· **六契约**。

> 本规范是在它**之上**新增的第 7 层(布局语法),不是架构重写。
> 但设计 token/原语时**会顺手优化地基的洞**(见 §4.13、§7):`onUnmount` 生命周期、`headExtra` 改作 `OceanHeader` 原语、右岛 `oceanId === feature id`、废弃 `--cc-*`、抽公共件(esc/diff/动画)。

---

## §2 Token 层

> 原则:`demo` 不缺**值**,缺的是**用哪个值的铁律**。所以本节 = 值 + 决策表。
> 铁律:**features/primitives 里禁止任何裸数值(px/hex/ms/cubic-bezier)**;一律 `var(--…)`。新值只能加在 `tokens.css`。

### §2.1 密度与行解剖(一种密度,定死)

> 📌 **值的唯一源 = [`core/tokens.css`](core/tokens.css)**(每个值旁有数学注释)。本节给规则与数学出处;数值以 tokens.css 为准。可视化见 [`tokens-preview.html`](tokens-preview.html)。

**密度阶梯 = 纯 2 的幂**(基 u=4px = 2²):

```
--grid:  4px;   /* 2² 基础网格 */
--gap:   8px;   /* 2³ 行内间距(图标↔文字 / 点↔文字) */
--icon: 16px;   /* 2⁴ 标准图标 = 行首槽 */
--lead: 16px;   /* = --icon,行首槽零死白 */
--row:  32px;   /* 2⁵ 标准行高(唯一) */
--ctl:  28px;   /* 7u 控件高 = row − grid(button/input/segmented) */
--icon-sm: 12px;  /* 3u 密集图标(meta 内联) */
--indent:  20px;  /* 5u 树每级缩进 */
--pad-row:  8px;  /* 2³ 行内边距 = 尾槽内嵌 */
```

**行解剖铁律**(所有 Row / ListRow / 类型头 一律长这样):

```
[ 行首槽 --lead ] --gap [ 标签 flex:1 省略号 ] [ 尾槽:meta 常驻 / 动作 hover ]
```

- 行首槽**恰好 = 图标尺寸**(16=2⁴),不留空盒(`demo` 早期 20px 盒有死白 → 现定为铁律)。
- ✅ `height:var(--row); gap:var(--gap); padding:0 var(--pad-row)`
- ❌ 任何 `height:34px` / `gap:11px` / `gap:5px` 这种手调魔数。

> 📐 4·8·16·32 = grid·gap·icon·row 四个最关键密度量,**正好是 2² 2³ 2⁴ 2⁵ 二进制阶梯**。这套密度对标 **Linear**(13px/32 行,我们的密度孪生)+ **macOS HIG**(10 源实测见 §3.3 末)。

### §2.2 字阶(值已在,补"用哪个 + 行高")

**角色命名**(非尺寸命名——逼组件按语义选,不硬编 px):

```
--t-meta:12  --t-body:13  --t-strong:16  --t-h3:20  --t-h2:24  --t-h1:32   (px)
模数 ≈大三度;display(16/20/24/32)落 4 网格(16=2⁴·20=5u·24=6u·32=2⁵);body 13 是锚、不入 2 幂。
```

| 场景 | token | 字重 | 行高(**永远显式**) |
|---|---|---|---|
| meta / caption / 时间 / 版本 | `--t-meta` 12 | 400 | 1.3 |
| 行/正文 body | `--t-body` 13 | 400/500 | `--lh-ui` 1.4 |
| 段标题 / 强调 | `--t-strong` 16 | 600(段标题大写 .04em) | `--lh-tight` 1.25 |
| 小标题 | `--t-h3` 20 | 600 | 1.25 |
| 标题 | `--t-h2` 24 | 600 | 1.25 |
| 页标题 | `--t-h1` 32 | 700 letter-spacing -.02em | `--lh-tight` 1.25 |
| 长文 prose | `--t-body` 13 | 400 | `--lh-prose` 1.6 |
| 代码 | 12 mono | — | 1.5 |

```
--lh-tight: 1.25;  --lh-ui: 1.4;  --lh-prose: 1.6;
```

> **双语铁律(血泪)**:① 打包**一族覆盖中英的字体**(MiSans);② **永远显式写 line-height**——`normal` 下 CJK 行更高,中英混排会顿挫。只换字体不够。
> ❌ 任何组件硬编 `font-size:13px`/`12px`(`demo` 的 code-editor、block-kit 都犯了);一律 `var(--t-*)`。
> **下限铁律**:UI 文字 **≥12px**(meta = `--t-meta` 12,对齐 Atlassian/无障碍下限;不设更小 token)。meta 与正文(13)只差 1px——层次靠**字色**(`--ink-3` vs `--ink-2`)拉开,不靠字号。

### §2.3 纵向间距(4px 网格)

```
--sp-1:4  --sp-2:8  --sp-3:12  --sp-4:16  --sp-6:24  --sp-8:32  --sp-12:48  --sp-16:64   (4 网格 ×1,2,3,4,6,8,12,16)
```

| 关系 | 值 |
|---|---|
| 行与行(列表内) | 0(贴)或 `--sp-0` |
| 字段与字段(表单) | `--sp-2` 8 |
| 段与段(section) | `--sp-6` 24 |
| 段内块(prose 块) | `--sp-3` 12 |
| 页面内边距 | 上下 `--sp-6` / 左右按内容宽 |

> 一个数一个用途。❌ `demo` 现状:段距 24/26/28/32 混用、行高 1.5/1.55/1.6/1.65/1.7 混用 → 全部收敛到上表。

### §2.4 圆角

```
--r-tag:4  --r-btn:8  --r-chip:12  --r-card:16  --r-island:20  --r-pill:999   (4 网格 ×1..5)
```
| 用途 | token |
|---|---|
| 大浮岛/窗 | `--r-island` / `--r-card` |
| 菜单/弹层 | `--r-chip` |
| 行/按钮/输入 | `--r-btn` |
| 小标签/角标 | `--r-tag` |
| 状态药丸 | `--r-pill` |

### §2.5 动效

```
--d-fast:120ms  --d-mid:240ms  --d-slow:340ms
--ease-out: cubic-bezier(.16,1,.3,1);  --ease-spring: cubic-bezier(.2,.9,.25,1);
```

| 动作 | 时长/曲线 |
|---|---|
| **hover 显隐/同槽互换(icon↔chevron、meta↔action)** | **0ms 即时** |
| hover 背景/字色 | `--d-fast` |
| 抽屉滑入/折叠/段展开 | `--d-mid` + `--ease-spring` |
| 环境/呼吸 | `--d-slow` |

> **即时切换铁律**:任何"默认态必须正确渲染"的 `opacity/color/width/transform` **不许进 transition**。原因双重:① 更脆(crisp);② 渲染器对未完成过渡会冻在起点(`demo` 无头预览里灰默认变黑、箭头不转、动作常驻——都是这个坑)。hover 揭示一律瞬时。

### §2.6 色与层次

```
墨   --ink(主) --ink-2(次,列表项默认) --ink-3(三级/meta)
面   --island(基) --island-2(嵌套) --island-3(hover 底) --island-4(选中底)
线   --line  --line-strong
强调 --accent  --accent-soft(10%)  --accent-line(30%)
语义 --ok --warn --danger（各带 -soft）
```

**用色铁律**:
- 文字三级:主 `--ink` / 次 `--ink-2` / meta `--ink-3`。
- **侧栏/列表字色铁律**:项默认 `--ink-2`(灰),**只有 hover / 选中才 `--ink`(黑)**。段标题/meta 用 `--ink-3`。(你之前要的"不全黑"= 此条)
- 面用深度阶梯(island→4),不用阴影堆叠表达层次。
- **accent 极度克制**:只落在①主 CTA ②实时态(live)③选中点。其余一律中性。
- ❌ **废弃 `--cc-*` 整套遗留别名**(`--cc-win/side/hover/active/bubble`);新代码只用语义名。`--cc-bubble` 那唯一灰填充 → 改 `--island-3` 或具名 `--bubble-user`。

---

## §3 页面骨架

### §3.1 三岛布局(契约)

```
┌───────────┬────────────────────────────┬──────────────┐
│ 左岛 侧栏  │        中央 海洋             │  右岛(可选)  │
│ #left     │        #sea                │  跟海洋走     │
│ 240–420   │        flex:1              │  token 宽     │
└───────────┴────────────────────────────┴──────────────┘
```
- 左岛宽 `--side-w: 240`(2u,默认;拖 240–420),收起态 0。
- 右岛宽用 **token**,不再每海洋手编(`demo`:chat 384/sch 360/doc 300/默认 372 → 收敛到谐波):
  ```
  --island-w:      360px;   /* 3u 标准右岛 */
  --island-w-wide: 480px;   /* 4u 宽(实体卡/深读) */
  ```
- 右岛**幂等键 = feature id**(`demo` chat 用 'entity-card' 是 bug → 一律 oceanId=本海洋 id)。
- 谐波平铺:侧栏 2u + 内容 6u + 右岛(3u/4u);整窗 1440 = 12u(2+6+4 恰好平铺)。

### §3.2 海洋结构(纵向三段)

```
OceanHeader (可选, flex:none)   ← 面包屑/标题/动作,由原语统一(见 §4.2)
OceanBody   (flex:1, scroll)    ← 内容,唯一滚动区
OceanFooter (可选, flex:none)   ← 仅 chat 的 composer 之类
```
- **唯一滚动区 = OceanBody**;统一用 `.scroll-fade`(上下缘柔化)。❌ 不再各海洋自定义 overflow。
- header 不再是裸 `headExtra` 共享槽 → 升级为 `OceanHeader` 原语(自带清理,见 §4.2、§7)。

### §3.3 内容宽(统一,不手编)

**读宽 = 对话宽 = 一个值**(都是"中央内容",分开没价值还破整数比):

```
--w-content: 720px;   /* 6u · 实体页/文档/设置/对话流 全部用它 */
--w-full:    100%;    /*      逃生:宽代码块/表/运行图 局部铺满 */
```
| 面 | 宽 |
|---|---|
| 实体页 / 文档 / 设置 / 对话流 | `--w-content` |
| 个别宽块(宽代码/表/图) | `--w-full`(那一块铺满,而非整列变宽——同 Notion full-width 块) |

> ❌ `demo`:chat 860 / 设置 680 / 实体 720 无据、且读/对话乱分 → 收敛到 **一个 720**。
> 📐 **行业基准(2026-06-16 实测 10 源)**:720 正中文档系共识(Obsidian ~700 · Craft 720–760 · macOS HIG 680–750 · Baymard 研究 680–750);右岛 360 = Linear 360 / Notion 400。行业全员**单一内容宽**(Notion/Obsidian/Linear),宽块按需 full-bleed。

> 📐 **整体密度基准**:Foryx 全套(13px 字 / 32 行 / 240 侧栏 / 4px 网格 / 16 图标)精准落在 **Linear + macOS HIG** 的"紧凑桌面"阵营——本地优先、键盘驱动、桌面生产力工具的同一条线。Notion/Primer/Material/Atlassian 偏宽偏大(16px 字、48–56 行)是因其为 web/触摸/读重产品,**不照抄**。值得抄的:密度→Linear/macOS,读宽→Obsidian/Craft,间距·图标 token→Primer/Atlassian。

---

## §4 原语库

> 这是**强制层**。每个面板 = 调原语,几乎不写自有 CSS。每个原语:**职责 / 解剖 / API / 正反例**。
> 命名 `fy-<name>`(Foryx 前缀,区别于 demo 的 fg-)。

> **🔒 对齐铁律(模版化,错位结构上不可能)**:凡"行类"元素(Row / New / 过滤 / 类型头 / 分组标签),一律走**同一三列网格** `grid-template-columns: var(--lead) 1fr auto`(行首固定列 / 标签 / 尾槽),**绝不靠 padding/width 手量对齐**。行首内容(状态点 / 图标 / 折叠箭头)统一**叠放居中**(`grid-area:1/1; place-self:center`)→ 7px 点与 16px 图标**同心**。图标墨迹画在**居中艺术板**(光学中心 ≈ 12,12),与点同心。于是 New 的 `+`、Search 的 🔍、类型图标、实体点 **永远同列对齐**(已测:同级 leading-center 与 label-left 像素相等)。❌ demo 的"每个 rail 各自摆 leading"是错位之源。

### §4.1 `Page`(记录页骨架)—— 杀掉 sec()/foldSec() 手搓
- **职责**:一个"记录/文档型"海洋 = `Page(header, sections[], rightIsland?)`。
- **解剖**:`OceanHeader` + 居中 `--w-content` 列 + `Section` 堆叠。
- **API**:`Page.mount(sea, { title, crumb, actions, sections, width })`
- 取代:entities 的 `sec/prose/foldSec`、documents 的 `.doc-root`、scheduler 的 `.sch-col`(三份互不兼容 → 一份)。

### §4.2 `OceanHeader`—— 杀掉裸 headExtra
- **职责**:面包屑 + 标题 + 右侧动作区,海洋切换自动清理(`data-ocean-head` 框架托管,不靠各海洋自觉)。
- **API**:`OceanHeader.set(sea, { crumb:[], title, actions:[Button…] })`

### §4.3 `Section`(段)
- **职责**:`section-label`(可选,大写灰)+ 内容岛 + 可折叠。
- **解剖**:label(`--t-meta` 12 · 600 · 大写 · `--ink-3`)/ `--island` 卡 / 折叠态走 §2.5。
- **API**:`Section({ label, fold?:'open'|'closed', body })`
- 折叠默认态走**参数**,不再 magic(`demo` 实体页"版本恒开、关系恒关"是硬编 → 显式 `fold`)。

### §4.4 `Field` / `KV`(键值行)
- **职责**:左 label(+hint)/ 右 控件或值。设置、实体 meta、信息块通用。
- **API**:`Field({ label, hint?, control })` · `KV.defs(host, [[k,v,opt]])`

### §4.5 `Row` / `ListRow`(**核心原语**)—— 杀掉 5 份 rail 行
- **职责**:唯一的"一行"。承载 chat 会话 / 实体 / workflow / 文档树 / 通知 / 设置类目。
- **解剖**(= §2.1 铁律):
  ```
  [leading: dot | icon↔chevron] gap [label flex:1 ⋯省略] [trailing: meta常驻 ↔ actions hover]
  ```
- **行首 icon↔chevron 同槽**(可折叠行):默认图标,hover 图标淡出/箭头淡入(同一 `--lead` 格,leaf 无箭头)。收起=右、展开=下。
- **行尾 meta↔action 同槽**:meta(时间/版本/计数)靠右常驻;动作(⋯/+)**绝对定位、零占位**,hover 浮现于同槽、meta 同步 `opacity:0` 让位(不重排)。两者永不抢空间。
- **API**:`Row({ leading, label, meta?, actions?, collapsible?, selected? })`
- ✅ 所有侧栏行用它 → 行高/间距/hover 物理一致,漂移不可能。
- ❌ `demo` 现状:`.ent-r`/`.chat-cv`/`.doc-rail-row`/`.sch-wf`/`.ntf-row` 五份近似拷贝。

### §4.6 `SidebarList`(侧栏列表)—— 杀掉 5 份过滤/排序菜单
- **职责**:New 按钮 + 过滤输入 + 排序/显示 sliders 菜单 + 分段(可折叠)+ `Row` 列表。
- **API**:`SidebarList.mount(host, { newLabel, filters, sortOpts, displayToggles, sections:[{label, rows}] })`
- 取代 chat/entities/documents/settings/notifications 各写一遍的"列表+过滤+菜单"。

### §4.7 `Toolbar` / `ActionGroup` · §4.8 `Button` · §4.9 `Input`
- **Button** 变体:`ghost`(默认,中性)/ `primary`(accent CTA)/ `danger`/ `icon`(28 方钮)。统一 hover/active/focus/disabled(§6)。取代 settings 手搓的 `btn/icbtn`、各处重写的 `.ibtn`。
- **Input / Textarea**:统一高 `--ctl`、focus 环、token 化。
- ❌ 不再有"局部小工厂"画按钮/输入。

### §4.10 `Menu` / `Floating` · §4.11 `Tabs` / `Segmented`
- **Floating**:统一锚定弹层 + Escape 栈(**按 ocean 隔离命名空间**,修 `demo` 全局 Escape 冲突)。
- **Tabs vs Segmented**:`demo` 两套 API 打架(Tabs 用 string key+回调、Segmented 用 numeric index)→ **统一一套**:`{ items:[{key,label,render?}], value, onPick }`。

### §4.12 `Badge` / `StatusDot` · `Tag` · `RefPill`
- 保留(已是好原语),但:`StatusDot` 状态归一只走 `config/state-model`(单一翻译路径,修 `demo` 双实现);`Badge` 升级为可挂点击的真组件(不再裸 HTML 串)。

### §4.13 `RightIsland`(右岛)
- 保留,但:宽走 `--island-w*` token;`oceanId === feature id`;width 改 token 驱动(不再 JS 内联 px)。

> **上帝组件拆解**:`EntityCard`(410 行/9 kind if 级联)、`BlockKit`(9 种块塞一个)→ 改为**由上述原语 + §5 schema 数据驱动组合**。`EntityCard` = `Page` + `Section` + 数据驱动字段;`BlockKit` 拆 `Calls / Output / Reasoning / Subtree`。

---

## §5 内容 schema 与「文档边界」

> 你已同意:**选择性文档化 + 声明数据 schema**。

### §5.1 记录模型(文档型面的统一表达)
**一个记录/文档型面 = `Page` ← `Section[]` ← `Field`/`Block`/`Row`,由声明式 schema 驱动渲染**(不是 if 级联)。

```
KIND_SCHEMA[entityKind] = { title, sections:[{ label, fields:[…], fold? }] }
BEAT_SCHEMA[beatType]   = { … }   // 对话流块
```
- 加一种实体/块 = 加一行 schema,不动核心组件。取代 `EntityCard.buildBody()` 的 9 分支级联、对话 beat 的无 schema。

### §5.2 「万物皆文档」的边界
| 面 | 归属 | 实现 |
|---|---|---|
| 实体页 / 设置 / 文档 / 通知卡 | **文档型** | `Page` + schema,近零 bespoke |
| 对话流 | 文档型(块流)| `Page` 变体 + `BEAT_SCHEMA` + 块原语 |
| **运行图(2D DAG)** | **逃生舱** | 自定义 `RunGraph`,但吃同套 token、活在标准 `Page`/`Island` 骨架 |
| **文档编辑器画布** | **逃生舱** | 自定义,token 绑定 |

> 逃生舱不是"随便写":它**只许**自定义"内部空间排布",外壳(页骨架/右岛/字色/密度/动效)全部吃 token。所以图和画布"照样是 Foryx"。

### §5.3 统一 DTO
- **Relation**:三处格式(documents backlinks / entities rel / conversations inline)→ 一个 `Relation { kind, id, label, snippet? }`。
- **State**:`DOT(idle/run/wait/err/done)` + 各域 ALIAS,单一翻译。
- mock/ 下加 `_schema.js` 声明每域规范形,作为前后端对齐的事实源。

---

## §6 交互规约

| 态 | 规则 |
|---|---|
| **default** | 列表/导航文字 `--ink-2`;面 `--island`。 |
| **hover** | 背景 `--island-3`(`--d-fast`);文字 → `--ink`;揭示动作/箭头(**0ms**)。 |
| **selected/on** | 背景 `--island-4`;文字 `--ink`;选中点 `--accent`。 |
| **focus** | `:focus-visible` 统一 `--accent-line` 2px 环(键盘可达,**必须有**——`demo` 缺)。 |
| **disabled** | `opacity:.4; pointer-events:none`。 |
| **active/press** | 轻微 `scale(.98)` 或加深底,`--d-fast`。 |

**两个同槽互换**(全局统一,见 §4.5):
- **行首 icon↔chevron**:默认图标 → hover 折叠箭头(可折叠行)。
- **行尾 meta↔action**:meta 常驻靠右 → hover 让位给 ⋯/+。

**字色铁律**:灰默认、hover/选中才黑(§2.6)。
**无障碍底线**:交互件用真 `<button>`、有 `:focus-visible`、可键盘操作(`demo` 多处 `div[role=button]` → 纠正)。

---

## §7 命名与第七契约

- **第 7 契约 = 布局语法**:本 SPEC 进 `contracts.md`,与 token/shell/组件/Intent/Live/DTO 并列。
- **强制 = 原语 + 命名 + (后续)lint**:照规范做(调原语)比不照做(手写 CSS)更省事,这才是真强制。命名 `fy-*`(组件)/ 海洋前缀(feature CSS),后续接 stylelint 守"无裸值/无裸 hex/前缀合规"。
- **绑 Flutter**:token 1:1 映射 Flutter `ThemeExtension`;原语解剖 1:1 映射 Flutter Widget。SPEC 是 web 原型与 Flutter 的共同事实源——所以现在把它定死,app 才不返工。
- **地基洞顺手补**(设计时优化):`Shell` 加 `onUnmount` 生命周期钩子(替代 chat 的 runId 补丁);公共件抽核(`esc/lineDiff/syntaxTokenize/shimmer/pulse` 各只一份);`headExtra`→`OceanHeader`;废 `--cc-*`。

---

## §8 `design/` 目录规划 + 重做路线

### 目录(规范定稿后落)
```
design/
├── README.md · SPEC.md            # 本规范
├── core/
│   ├── tokens.css                 # §2 全部 token(唯一值源)
│   ├── shell.* sidebar.* intent.* live.* loader.*   # 地基(移植 demo + 补洞)
│   ├── primitives/                # §4 原语(强制层):page/section/row/sidebar-list/button/input/menu/tabs/badge/right-island…
│   ├── config/                    # state-model / entity-kinds / *_schema(§5)
│   └── contracts.md               # 含第 7 契约
├── features/<ocean>/              # 薄组合:调原语,几乎不写 CSS
├── mock/<域>.js + _schema.js
└── app.html · index.html · reference.html   # 装配 + 原语活体规格台
```

### 路线(grammar-first,切片推进,不大爆炸)
1. **Phase 1|焊地基+原语**:`tokens.css`(§2)+ 核心原语(`Page/Section/Row/SidebarList/Button/Input/OceanHeader`)+ 补地基洞 + `reference.html` 活体规格台。
2. **Phase 2|拉通样板面**:用原语重建**实体页**(最典型记录面、最压榨原语),证明近零 bespoke CSS、漂移不可能。**这是大铺开前的验证关卡。**
3. **Phase 3|铺开**:其余海洋逐个瘦成薄组合;运行图/编辑器走逃生舱。
4. **Phase 4|锁契约**:进 `contracts.md` 第 7 契约,绑 Flutter。

---

> 待你研究后,我们逐节定稿;有异议的地方直接标,我改。定稿即落 Phase 1。
