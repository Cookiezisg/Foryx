# Foryx Design — 进度 / 背景 / 续接手册

> **开新对话先读这份 + [`SPEC.md`](SPEC.md),就有全部上下文,能直接续。**
> 本文 = 任务背景 + 关键决策 + 进度日志 + 当前状态 + 下一步 + 操作/续接须知。
> `SPEC.md` = 规范本身(宪法);本文 = "我们在哪、怎么到这、怎么继续"。

---

## 0. 一句话

把旧 `demo/` 的完整产品味道迁进 `design/`,同时用 **Foryx 布局语法 + token + 模块化组件** 约束它,让产品形态完整、入口统一、按钮/岛屿/滚动规则由规范兜底。

---

## 1. 任务背景

- **产品 = Foryx**(本地优先 agentic workflow 桌面 app,Flutter 目标 + Go sidecar)。`demo/` 作为旧参照不动;`design/` 是当前设计规范和完整 app 运行区。
- **`demo/`** = 大骨架很好(海洋+三岛+六契约+Intent/Live+组件库),但**缺统一布局语法**:行高/间距/字阶/页骨架/交互全靠各海洋手搓 → 纷繁、必漂(同一种"行"写 5 遍、`sec/foldSec` 写 3 遍、行高 32/34 间距 4/5/8/9/11/15 乱飘、上帝组件 EntityCard 410 行、无数据 schema、`--cc-*` 遗留)。一次 12-agent 全量审计证实(THEME A 无布局语法 / B 同 UI 手搓 N 遍 / C 上帝组件 / D 无 schema / E 外壳生命周期洞)。
- **视觉方向以旧 `demo/` 的产品味道为准**:白色 window 内部三岛,左/右浮岛有发丝线与轻阴影,中央海洋同白面、靠窄阅读列与留白分层。
- **`design/`** = 旧产品模块整体迁入 + 第 7 层布局语法。心法 = **三件套**:文档(解释,SPEC)+ token(唯一值源,tokens.css)+ 模块化组件/原语(强制层)。「一致性 = 约束的副产品;规范的力量在拒绝什么,不在能表达什么」。

## 2. 用户拍板的关键决策

1. `design/` = **干净重写**(不在 demo 上打补丁;demo 留作参照)。
2. **选择性"万物皆文档"**:记录/列表/表单型面走统一语法;**运行图 / 编辑器画布走逃生舱**(自定义,但吃同套 token + 活在标准页骨架)。
3. **同时声明数据 schema**(KIND_SCHEMA/BEAT_SCHEMA,杀 if 级联)。
4. **单一规范密度**(由我定)。
5. **大骨架保留**,但设计时**狂优化**地基洞。

### 数系(三层数学,已定稿,详见 SPEC §2、可视化 tokens-preview.html)
- **密度阶梯 = 纯 2 的幂**:grid 4 · gap 8 · icon/lead 16 · row 32(2²·2³·2⁴·2⁵)。
- **布局 = 谐波 2:3:6**(音乐比例,u=120):侧栏 240(2u)· 右岛 360(3u)· 内容 720(6u);1440 窗 = 12u。读宽=对话宽统一 `--w-content` 720,宽块走 full-bleed 逃生。
- **字阶 = 模数**(≈大三度;display 16/20/24/32 落 4 网格);body 13 是锚、不入 2 幂;meta 12 是 UI 下限;角色命名 `--t-meta/body/strong/h3/h2/h1`。
- 圆角 4/8/12/16/20;`--cc-*` 全废。值经 **10 源行业实测**锚定(Foryx = Linear + macOS 紧凑桌面线;Notion/Primer/Material 偏宽是 web/触摸/读重,不照抄)。
- **密度路线(13px)不为纯度牺牲信息密度——数学放进关系,不强求每数是 2 幂。**

### 🔒 对齐铁律(模版化,错位结构上不可能)
- **行 = 三列网格** `grid-template-columns: var(--lead) 1fr auto`(行首固定列 / 标签 / 尾槽),**绝不靠 padding/width 手量对齐**。
- **行首槽居中 + 尾槽 accessory slot**:① 行首 7px 点与 16px 图标**同心**;② 尾槽用 `--trail` dense accessory slot(20px = `--ctl - --sp-2`,已登记 tokens),meta 保持文本视觉但占此隐形锚位,action 是同尺寸小方形按钮,多 action `gap:0` 向左扩展,整体**右边缘锚定** → 版本号/计数/动作右端成线,hover swap 不重排。
- **图标墨迹画在居中艺术板**(光学中心 ≈ 12,12),与点同心。
- **分组标签走邻近原则**(上 `sp-2` 分隔 / 下 `sp-1` 贴附)。
- 实测:同级 leading-center 与 label-left 像素相等。(SPEC §4 有此铁律全文)

### 其他既定规约
- hover 显隐/同槽互换 = **0ms 即时**(不入 transition):更脆 + 避开无头渲染器"未完成过渡冻初值"坑。
- 字色铁律:列表项默认 `--ink-2` 灰,hover/选中才 `--ink` 黑;meta `--ink-3`;accent 只落 主CTA/实时/选中点。
- 双语:打包 MiSans(一族覆盖中英)+ **永远显式 line-height**。

## 3. 进度日志(本轮提交,新→旧读也行)

| 阶段 | 内容 |
|---|---|
| **Phase 0** | `SPEC.md`(宪法 8 节)+ `README.md`;数值经 10 源行业实测锚定 |
| **数系定稿** | `core/tokens.css`(唯一值源,每值带数学注释)+ `tokens-preview.html`(可视化) |
| **Phase 1 起步** | 管线 `reset/cssload/dom(单一 esc)/icons(单一 stroke)` + 原语 Button/StatusDot/Row/Section + `reference.html` 活体规格台 |
| **对齐模版化** | Row 改三列网格 + 行首叠放居中;新增 **SidebarList**;图标重画居中(修点/图标/New/Search 错位,实测同级 x 相等) |
| **Phase 1 续** | Field/KV · Input · Badge · Tabs |
| **swap+间距修** | 尾槽 `--trail` accessory slot(meta 占 action 隐形锚位,多 action gap=0,右端成线)+ 分组间距邻近原则 |
| **Phase 1 收官 + Phase 2 样板** | 页骨架 OceanHeader/Page/RightIsland + **`entity.html`**(三岛外壳 240/720/360 + 全程原语拼装的实体页) |
| **demo 视觉校准** | `entity.html` 改为白色 window + 左/右浮岛 + 中央文档型海洋;`Section` 新增 `plain` 变体;代码高亮色收进 `--cd-*` token |
| **岛屿开关规范** | 左/右岛只靠 `panel-left` / `panel-right` 图标按钮开合;按钮不保留加深/放大态;全收时海洋两侧按钮对称 |
| **token 守门补齐** | `Button/Input/Badge/Tabs/StatusDot/RightIsland/Section/Field/OceanHeader` 裸度量收进 `tokens.css`;primitives 扫描无裸 `px/ms/hex/scale` |
| **CodeEditor 原语** | 新增 `FyCodeEditor`(只读代码块 + 可编辑 mount + 统一 tokenizer);`entity.html` 不再手写 `.code/.kw/.str` |
| **原生文档感 + 滚动归属** | 文档型海洋默认少块/无强边界;CodeEditor 默认平铺编辑板;左/右岛可滑但隐藏滚轮,中间 Page 使用 overlay thumb |
| **JsonTree 原语** | JSON 展示态不再裸露;新增 `FyJsonTree` 把 args/output/config 解析成折叠树 + key/value 行 |
| **InfoCard + 右岛检查器密度** | 新增 `FyInfoCard`;右岛改为状态/操作 + 输入上下文 + 执行链路 + 结果摘要 + 结构化详情,不用横线补结构 |
| **Menu/Floating 原语** | 新增 `FyFloating` + `FyMenu`;侧栏 sliders 已接真实菜单;reference 增加活体菜单卡 |
| **后端能力校准** | 认真读 backend/domain + references 后重做 `entity.html`:选中 Function 页按真实能力投影当前版本/运行环境/Field/deps/运行历史/关联关系,但默认隐藏 raw ID、endpoint、SSE、execution row 等内部细节 |
| **实体 schema 起步** | 曾以独立 `entity.html` + schema 试做实体页;当前已被完整 app 迁移取代,实体入口改为 `app.html#entities` |
| **设计收敛 pass** | `Row` 增加 `hint/passive`;`KV.html` 成为紧凑摘要原语;`entity.html` 删除本地 `mini/schema/ri-kv/ri-step` 样式,字段/版本/关系/步骤统一回到 Row/KV/JsonTree;`reference.html` 清掉示例内联裸样式 |
| **实体 tab 去废话** | Version/Relation tab 删除说明文案和重复标题;Function 默认 tab 去掉“迭代”;OceanHeader 拉开 title/meta/tabs 的 token 间距 |
| **产品文案降噪** | 实体页删除重复 Run/Iterate 入口和概览运行说明;`需要输入/返回字段` 收敛为 `输入/输出`;fixture 长说明改成短事实;SPEC 增加产品文案铁律 |
| **Terminal 重构** | Function 右岛曾从静态运行检查器改为终端;当前完整 app 迁移后以 `features/entities` 的试运行右岛为准 |
| **Toolbar/ActionGroup 原语** | 新增 `FyActionGroup` + `FyToolbar`;页头动作与终端执行动作不再手写 flex 按钮组,reference 增加活体规格卡 |
| **Quadrinity 实体样板** | 旧独立样板停止作为主入口;当前以迁移后的 `features/entities` 产品海洋为准 |
| **完整 app 迁移** | 将旧 `demo` 的 `app.html` / `manifest` / `core/components` / `features` / `mock` 整体迁入 `design`;`app.html` 成为完整产品入口,`entity.html` 跳转到 `app.html#entities`;保留旧产品味道,由 `design/core/tokens.css` 提供 token compatibility |
| **规范兼容层** | `dom.js` 支持旧 product modules 与新 primitives;`icons.js` 使用旧 demo 全量图标并补 Foryx alias;`reset.css` 增加 `.ibtn/.grow/.chev/.scroll-fade`;shell/sidebar 关键尺寸回收进 token;右岛内部 X 隐藏,开合回到海洋右上 panel 按钮 |

## 4. 当前状态(已落 + 已验证)

**原语成套**(`core/primitives/`,`fy-` 前缀,自载 CSS,只读 token,`html()`+`mount()` 契约):
`Button(4 变体) · ActionGroup · Toolbar · StatusDot(5 态) · Row(核心,含 hint 二级文本/passive 信息态) · SidebarList · Section · Field/KV(含紧凑 KV.html) · Input · CodeEditor · JsonTree · InfoCard · Floating · Menu · Badge · Tabs · OceanHeader · Page · RightIsland`。

**目录**:
```
design/
├── README.md · SPEC.md · PROGRESS.md      # 规范 + 本文
├── index.html · app.html                   # 入口画廊 + 完整产品 app
├── tokens-preview.html                     # 数系可视化
├── reference.html                          # 原语活体规格台(showcase)
├── entity.html                             # 兼容入口:跳转 app.html#entities
├── manifest.js · mock/                     # app 运行清单 + mock data
├── features/                               # Chat / Entities / Scheduler / Documents / Settings / Notifications / Onboarding
└── core/
    ├── tokens.css                          # 唯一值源(数学注释)
    ├── reset.css cssload.js dom.js icons.js shell/sidebar/loader/intent/live
    ├── components/                         # 迁移后的产品组件库
    └── primitives/  *.js + *.css           # 规范原语台
```

**验证手段(每步都做)**:预览渲染 + **测量对齐**(同级 leading-center / label-left 像素相等;尾槽 meta/action 右端相等)+ 截图 + 0 console 错误。
- entity.html 实测:左岛 240 / 右岛 360 / 内容列 720 全来自 token;白窗+左右浮岛+中央白海面;4 个 `plain` section;4 字段 4 tab;0 console 错误。
- 视觉方向:实体页中央不再把每段包卡;文档型内容走 `Section({variant:'plain'})`,代码/字段组默认平铺,靠标题/留白/字色形成原生文档感。左/右岛与中央海洋颜色继承 `demo/` 的白窗三岛关系。
- 岛屿开关:无产品标志、无 `X`、无悬浮拉手;左岛展开时 `panel-left` 在左岛右上角,收起时迁到海洋左上角;右岛 `panel-right` 永远在海洋右上角;按钮不保留加深/放大态,只用布局变化 + `aria-pressed` 表示状态。
- CodeEditor:实体页 Code 走 `FyCodeEditor.html()`;reference 增加 CodeEditor 活体规格卡;语法色统一读 `--cd-*`,不再在页面内拼 `<span class="kw">`;默认无框无行号无内边距,像正文里的高亮编辑板,需要隔离时才 `variant:'boxed'`。
- JsonTree:展示态 JSON / tool args / run output / entity config 不裸露原始 JSON;实体页右岛参数与输出已改为 `FyJsonTree` 无根树(`root:false`),由 InfoCard 标题承载"输入/输出",第一层对象/数组默认展开,reference 增加 JsonTree 活体规格卡。
- InfoCard/右岛:信息分组默认走无边 `FyInfoCard`;右岛不做空抽屉。执行型实体使用 DebugTerminal,包含输入、信息、输出、日志。
- 实体终端:当前以迁移后的 `features/entities` 为准,中间编辑实体定义,右岛承载试运行/节点检视/最近运行;开合由海洋右上 panel 按钮控制,右岛内部 close 入口隐藏。
- Menu/Floating:弹层统一走 `FyFloating`;菜单统一走 `FyMenu` 行结构,支持 checked/meta/danger/disabled;SidebarList sliders 已接入真实排序/显示菜单。
- 滚动归属:左/右岛固定但允许纵向滑动,滚轮永远隐藏;中间海洋 Page 隐藏 native scrollbar gutter,使用浮动 overlay thumb;hover/滚动时出现,无溢出或空闲时消失,不挤压内容宽度。
- 实体页事实源:当前实体页由完整 app 的 `features/entities` + `mock/entities.js` 驱动;`entity.html` 只跳转到 `app.html#entities`。后续收敛要在 product module 内完成,不再恢复单页样板。
- 设计收敛:实体页局部列表和右岛摘要已回收进原语层:字段清单/版本/运行历史/关系/执行过程走 `FyRow({hint, passive:true})`,状态摘要走 `KV.html`,输入输出走 rootless `FyJsonTree`;页面 bespoke CSS 只保留三岛壳、文档栈和少量产品文案排版。
- Token 守门:原语内部度量已登记为 token(`--hairline`/`--gap-tight`/`--badge-h`/`--field-row`/`--tab-h`/`--island-head` 等);`design/core/primitives` 扫描无裸 `px/ms/hex/scale`。

## 5. 下一步(Phase 1 尾 + Phase 2/3/4)

- **补地基洞**(SPEC §7):Shell `onUnmount` 生命周期钩子(替代 chat runId 补丁)· `headExtra`→OceanHeader 已做 · 右岛 oceanId=feature id · 公共件抽核(diff/syntaxTokenize/动画)。
- **数据 schema**(SPEC §5):当前 app 运行层沿用 `core/config/entity-kinds.js`、`state-model.js` 与 feature 模块;后续 schema 化必须声明消费方,避免死 schema。
- **Phase 3 铺其余海洋**:对话流(块流)· **运行图(逃生舱:自定义但吃 token)** · 文档(编辑器逃生舱)· 设置 · 通知。
- **Phase 4**:布局语法进 `contracts.md` 第 7 契约 + 绑 Flutter(token→ThemeExtension,原语解剖→Widget)。

## 6. 操作 / 续接须知(重要)

**预览**(`design/` 静态页,无后端):
- `.claude/launch.json` 已加 `design` server(端口 **4191**,no-cache;该文件 gitignored)。
- 起:`preview_start name=design` → 开 `/app.html`(完整 app)、`/index.html`(画廊)、`/reference.html`(原语台)、`/tokens-preview.html`(数系)。`/entity.html` 等价进入 `/app.html#entities`。

**无头渲染器两个坑(踩过)**:
1. **过渡冻结**:未完成的 `opacity/color/width/transform` 过渡会冻在起点 → 默认态错乱。**对策**:凡"默认态须正确渲染"的属性不进 transition;hover 揭示一律即时。
2. **视口会塌成 1px**:eval 量布局偶尔得到全 0 宽(`innerWidth:1`)→ 不是 CSS bug。**对策**:量之前 `preview_resize 1440 900`。截图渲染正常、不受影响。
- 截图工具有时把视口钉在某区域;要看下方内容,可临时把目标卡 `insertBefore` 提到 `.wrap` 顶部再截。

**工程纪律(本仓约定)**:
- 中文回复;代码/路径/英文 commit 半句保持原样。
- commit **不加** `Co-Authored-By: Claude` 尾注。
- **每次 commit 后立刻 push origin/main**(投资人可见)。
- **只在 main 开发、不开分支**;用精确 `git add design/` 隔离(别 `git add -A`)。
- `demo/` 是别人的域 + 维持 Foryx 不动;`design/` 是本工作区、用 Foryx。
- 改 token 一处、全系统跟着变(零成本);改原语 = 改 `core/primitives/<x>.{js,css}`。

**心法复诵**:文档解释、token 定值、**原语强制**;对齐/密度**靠模版结构、不靠手量**;能用原语拼就别写自有 CSS。
