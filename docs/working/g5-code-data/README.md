---
id: WRK-040
type: working
status: active
owner: @weilin
created: 2026-06-24
reviewed: 2026-06-24
review-due: 2026-09-22
audience: [human, ai]
landed-into:
---

> G5 开工前调研产物(15-agent 工作流:10 扇出逐件 demo 盘点 + kit 复用 + 联网 best-practice + 后端契约绑定 → 1 综合 → 3 镜对抗复审 → 1 硬化折入)。
> 待用户拍板 5 决策(见末 openDecisions——复审把「高亮包」「diff 包」从伪锁定升回 openDecision)。建造规范、单一作者、gallery-first、逐件提交。
> 结论随件 landed 进 `references/frontend/design-system.md` 后归档(同 WRK-037/038/039 先例)。

# WRK-040 — G5 代码与数据 建造规范

## §0 一句话

落 **3 个代码/数据原语**(`AnCodeEditor` / `AnJsonTree` / `AnVersionDiff`)+ **1 组共享地基**(语法调色板 `cd-*` token + `trail` 度量 + `highlight()` / `lineDiff()` 两**同步**纯函数 + 新 i18n keys)。
**复用优先 + 不重造确定性算法**是本组双主轴:容器/字体/滚动/折叠 100% 骑既有 kit(`AnCard`/`AnText.mono`/`AnScrollBehavior`/`AnExpandReveal`/`AnInteractive`/`AnCard` 外框),
代码高亮的引擎选型**有真坑**(见 §1+openDecision A:成熟 Flutter 高亮包整片陈旧/与本组同步契约冲突,demo 正则 tokenizer 反成最顺载体),行级 diff 经复审证伪后**回归 demo LCS 移植**为 v1 floor(顺序输出正合 unified 网格);JSON 树骑 Flutter **内置 TreeSliver**(官方虚拟化、零新依赖、解 650KB 性能悬崖,**但带已知回归须门控**)。
**两条铁律**:① **唯一高亮源**——三件全调同一 `highlight()`,VersionDiff/JsonTree 绝不另起第二套 tokenizer;② **高亮/diff 必须同步**——流式 `.value`/`.after` 立即 repaint、diff 逐行内联着色全靠它,引入异步预载的包一律否决(即便更星)。

**字体两面(复审 #20 纠正)**:本项目打包**两个**字体——`uiFamily='MiSans'`(MiSansVF.ttf,UI 文字:顶栏语言标签/note/说明文走 `AnText.meta`)与 `monoFamily='JetBrains Mono'`(JetBrainsMono.ttf,代码面/行号/+N−N/range 版本号走 `AnText.mono` / `value(mono:true)` tabular)。**代码面用 mono 正确、UI 文字勿误 mono 化**。

**已核对的 kit 现状**:`colors.dart` 已有 `ok/okSoft/danger/dangerSoft`(双 brightness:`0xFF2DA44E`/`rgba(45,164,78,.12)`/`0xFFD70015`/`rgba(215,0,21,.10)`;暗色亦全,lerp/copyWith 全字段)但**无任何 `cd-*` 语法色**;`tokens.dart` 有 `s2..s64`/`row=32`/`control=28`/`tab=34`/`icon=16`/`iconLg=20`/`hairline=1`/`gripLine=2` 但**无 `trail`(行号槽宽)度量**;`typography.dart` 有 `mono`(JetBrains Mono 已打包 / 13px / height 1.5)+ `value({mono})`(无条件 tabular)+ `meta`(MiSans)+ `metaTabular()`。`AnExpandReveal`(nested-safe,WRK-039 已验)/`AnScrollBehavior`/`AnInteractive`/`AnCard`/`AnInput`/`AnSeamlessField`/`AnRow`(已透 `expanded` 折叠态语义) 全在。`GallerySpecimen.height` 字段已于 G4.0 落地——本批滚动宿主直接复用。`gallery_matrix_test.dart` 轴 = **build / no-overflow / render + reduced**(**无 a11y-role 轴**;a11y 是逐件 dedicated 测试,同 `an_card_test` 用 `ensureSemantics`+`getSemantics`——复审 #17 纠正)。pubspec **无任何 code-editor/json/diff/highlight 包,基线为零**。

## §1 锁定决策

**用户 2026-06-24 拍板(5 决策,全采纳推荐)**:
1. **代码高亮引擎 = 移植 demo 正则 tokenizer 成同步 `highlight(code,lang)→List<TextSpan>`**(决策 1·A)。整片成熟 Flutter 高亮包陈旧/与同步契约冲突(详 §8);窄面 Python/CEL/JSON 移植已验逻辑非造轮子。封装层 `highlight()` 为**唯一入口**(隔离选型、换引擎只动一处);`highlighting` 包留作日后宽语言准度的升级路径,`syntax_highlight` 因 async 预载违 §4 同步铁律已否决。
2. **行级 diff 算法 = 移植 demo LCS 成同步 `lineDiff(before,after)→List<({DiffOp op,String text})>`**(决策 5·A)。逆向 DP+回溯直出顺序 ctx/add/del、零包、正合 unified 网格;LCS 是教科书确定性算法、非几何 footgun。退化闸按 **LCS 矩阵 cell 数 `(m+1)(n+1)>4M`**(命名 `lineDiffMaxCells`,忠实移植 demo;**G5.0 复审修订**——原研究写「m+n 行数闸」,实测证伪:m+n 封不住 m\*n 矩阵,详见末「G5.0 实施修订」)。`diffutil_dart` 留作真机压测卡顿后的升级路径(保留同一出参契约)。
3. **VersionDiff v1 = 仅单字段文本行 diff**(决策 4·A)。适配 Function.code / Agent.prompt / Control.when·emit / Approval.template;结构化多字段(inputs/outputs/deps JSON 走 JsonTree)+ **Handler 多部件**(无单一 code 文本)版本 diff 整体推迟到 entities 版本视图 feature(见 §7)。
4. **CEL 要语法上色**(决策 2·B)。因选 demo 正则路线 → 在 `highlight()` 封装层内部对 `lang==cel` 加最小正则分派(插值 `{{ }}`/`${ }` + 注释 + 字符串 + 数字),**对外仍唯一入口、绝不另起第二套 tokenizer**;CEL 专属精细 tokenizer 推迟。
5. **JsonTree 复制路径 = JSON-pointer**(决策 3·A,RFC 6901,机器寻址语义)。复制能力本身推迟到消费 feature(kit 出 `AnInteractive` 行级动作钩子),格式先定为 JSON-pointer。

**清晰推荐-即锁定**(技术上无争议,反对再议):

- **JSON 树 = Flutter 内置 TreeSliver**(`package:flutter/widgets.dart`,非第三方包,自 3.24 入库)。`treeNodeBuilder` 仅建可视行(真虚拟化、解 650KB 悬崖)、`TreeSliverIndentationType.none` 让行皮自绘贴 kit 风、`treeRowExtentBuilder` 给 `AnSize.row=32`。最星标第三方树包 baumths/flutter_tree_view 已于 2025-03 归档、作者明确改推官方 TreeSliver;所有专用 JSON 包(json_data_explorer v0.1.0/4年未更、json_explorer fork 拖依赖、flutter_json v0.1.0/5likes、easy_json_viewer 风格锁死、flutter_json_viewer 2021 死)均否决。**但 TreeSliver 带已知回归**(#153889 duration-zero 折叠冻结、#178962 全收起抛异常、#167928 cross-axis hitTest——已联网核实存在),**采用但带回归清单 + 3.41.9 实测门控**(见 §4+§9),非裸断「已稳」。
- **`cd-*` 语法调色板 = G5.0 地基件,先于三组件**。在 `AnColors` ThemeExtension 加一组 syntax 色(双 brightness),非任一件私有内联(违视觉灵魂禁内联)。明亮(One Light):com `#a0a1a7` / kw `#a626a4` / str `#50a14f` / num `#986801` / fn `#4078f2` / arg=accent;暗色(One Dark)翻转。JsonTree 用 str/num/kw(bool)/com(null),VersionDiff/CodeEditor 用全色。
- **`trail`(行号槽宽)= G5.0 地基度量**,登 `tokens.dart` `AnSize.trail`。**复审 #7 纠正:demo 的 20px 是 web 更小字号下的槽,JetBrains Mono 13px tabular 单字≈8px,20px 连 3 位数都放不下。** v1 取**动态计宽**(按内容最大行号位数 `maxLines.toString().length × 单字宽 + 右 padding`,用 `IntrinsicWidth` 或预算)而非固定 20px——「行号溢出」从 LOW 升为 §4 必处理(否则 100 行文件行号即糊)。固定占位下界 ≥`~36px`(容 4 位数)。
- **三件全单框、容器复用既有 kit**:CodeEditor/VersionDiff 框骑 `AnCard`(描边走 `AnCard` 既有实色 line border + island bg,**不用半透明 border 叠角**,见 §4),JsonTree 滚动宿主骑 `TreeSliver` in `CustomScrollView` + `AnScrollBehavior` 局部隐条。**复审 #13 纠正:CodeEditor 编辑态不骑 AnInput**(AnInput 渲可见着色文本 `color: readOnly?inkFaint:ink` + 自管 style + box chrome control=28/s12,与「透明文字叠高亮 pre」架构冲突、破 pixel-perfect)——见 §5。
- **不引 Riverpod/SSE 进 kit**:三件是 ZERO-业务纯 props widget(gallery-first 单作者)。流式 `.value`/`.after` setter、版本选择列表、revert/iterate、草稿/审批门、嵌套 block-tree/run-terminal 全推迟到 feature(见 §7)。

**高亮/diff 选型的复审证伪依据(已按上方 §1 拍板移植 demo;留此记录为何不引包)**:
- **代码高亮引擎**(决策 1,已定 A 移植 demo 正则)——草案锁 flutter_highlight 是事实错误(0.7.0/5年前/unverified/返 Widget,详 §8 已解;草案误把 diffutil 数据安到它头上)。整片成熟包均有硬伤:highlighting/flutter_highlighting 引擎 3 年陈旧+低 likes、syntax_highlight 须 async init + TextMate 主题 + 仅 15 语言。**封装层 `highlight(code,lang)→List<TextSpan>` 唯一入口必须保留**(隔离选型、吃 SyntaxColors、换引擎只动一处)。技术推荐 = **移植 demo 正则 tokenizer 成同步 `highlight()`**(唯一同时满足 同步+List<TextSpan>+cd-* 任意着色+CEL/插值原生+零包风险),交用户拍板。
- **行级 diff 算法**(openDecision B)——草案锁 diffutil_dart「~20 行薄投影」被严重低估(getUpdates() index-based + Move/Change + 批序,折成顺序 unified 网格非平凡;demo LCS 本就直出顺序 `[op,text][]`)。技术推荐 = **移植 demo LCS 成同步 `lineDiff()`** 为 v1 floor;diffutil_dart 留作压测证 LCS 在 2000 行级卡顿后的升级路径。

## §2 新共享原语(地基)

- **`SyntaxColors`**(`core/design`,AnColors ThemeExtension 扩一组):cd-com/kw/str/num/fn(5 原色)+ arg(=accent)。双 brightness。三件唯一着色源,禁内联硬编码 Color 字面量(同 G4 复审 ⑥)。
- **`AnSize.trail`**(`core/design/tokens.dart`):行号/符号槽宽。**动态计宽为主**(按最大行号位数,复审 #7);固定占位下界 ≥36px(容 4 位数,非 demo 20px)。CodeEditor 行号列 + VersionDiff 三列网格共用,右对齐 tabular。
- **`syntax_highlighter.dart` 的静态 `highlight(code, lang) → List<TextSpan>`**(`core/ui`):**必须同步纯函数**(§4 铁律)。吃 `SyntaxColors` token、缺失语言裸渲兜底(不谎报)、对 `lang==cel` 在封装层内部加最小正则分派(插值 `{{ }}`/`${ }`+注释+字符串+数字,见 openDecision A2)。VersionDiff/JsonTree 行内着色全调它,**唯一 tokenizer**。
- **`code_diff.dart` 的静态 `lineDiff(before, after) → List<({DiffOp op, String text})>`**(落 `core/model`):**必须同步纯函数**。v1 = demo LCS 移植(逆向 DP + 回溯,直出顺序 ctx/add/del)+ 退化闸(按 **LCS 矩阵 cell 数 `(m+1)(n+1)>4M`**,命名 `lineDiffMaxCells`,忠实移植 demo——**G5.0 复审修订**:原研究拟用「m+n 行数闸」,实测证伪 m+n 封不住 m\*n 矩阵、平衡型 m=n=2500 漏网撑 ~50MB,见末「G5.0 实施修订」)+ before 空→整段 ctx。脱 widget golden 单测(空 before / 全删全增 / 中段改 / **多重复行多空行** / **平衡型 cell 退化** / 行号在 del 不++、add/ctx 累进)。
- **新 i18n keys**(slang,`lib/i18n/<locale>.i18n.json`)——**复审 #19 纠正:先 grep 既有树,`action.{edit,cancel,save}` 与 `feedback.{error,success,...}` 已存在,复用之、勿重定义**:
  - **复用既有**:`action.{edit,cancel,save}` · `feedback.error`。
  - **真新增**:`action.{copy,wrap}` · `feedback.{copiedToClipboard,copyFailed}` · `editor.{line,lines,<lang 标签>}` · `tree.{expandedItem,collapsedItem,circular,moreItems}` · `diff.{added,removed,unchanged,version,largeDiff}` · `errors.{invalidJson,circularRef}`。
  - demo `code-editor.js` 硬编码"取消/保存"——Flutter 严禁复制,走 `context.t.<key>`。

## §3 跨切依赖(改/新增已发布地基,须重跑其测试)

- **AnColors 加 SyntaxColors**(地基扩展):重跑 `an_colors`/theme 测 + lerp/copyWith 全字段断言(双 brightness + lerp 不丢字段)。
- **AnSize 加 trail**(地基扩展):无行为风险,本批新引用,确认 tokens 测试通过。
- **pubspec 新包**:**先在 scratch/frontend 真实 `flutter pub add` 试装 + `flutter analyze` 冒烟核 Dart 3.11.5 / Flutter 3.41.9 resolve + null-safety**(verify-by-real-run;尤其任何 5 年前的高亮引擎包能否在 Dart 3.11 编译/分析是真实未验风险——草案凭空写「均声明 null-safe」对 flutter_highlight 是无据)。仅 demo 正则路线时无新高亮包、零此风险。`make fe-verify` 重跑 codegen+analyze+全测。
- **highlighting 语言集核验**(若选包路线):逐一核 python/json/markdown/yaml/sql/javascript(后端实际用到全集)是否在覆盖内,缺失者走兜底裸渲并在 lang 标签标注。
- **CEL 高亮缺口**:任何高亮引擎(包或正则)都不含 CEL DSL。v1 在 `highlight()` 封装层内部对 `lang==cel` 加一段最小正则分派(插值/注释/字符串/数字),**对外仍唯一入口、绝不另起第二套 tokenizer**(见 openDecision A2)。CEL 专属精细 tokenizer 推迟(§7)。

## §4 HIGH 正确性铁律(build 时必守)

- **【铁律·唯一高亮源】**:VersionDiff/JsonTree 行内着色**只**调 `highlight(code, lang)`,缺失裸转义兜底——**禁第二套 tokenizer**。VersionDiff 必须 `highlight()` 已就位才能建(建造序硬依赖,§6)。
- **【铁律·高亮/diff 必须同步】(复审 #4 新增)**:`highlight()` 与 `lineDiff()` 必须**同步纯函数**——流式 `.value`/`.after` setter 的立即 repaint、diff 逐行内联着色、纯 model 层单测全依赖此。**任何引入异步预载/异步 tokenizer 的包一律否决(即便更星)**——serverpod syntax_highlight 的 `await Highlighter.initialize([...])` per-language 预载 + 整块单 TextSpan(无法天然按行切喂三列 diff 网格)与本架构正面冲突,**显式否决**(免 build 时有人盲换,见 §8)。
- **【铁律·TreeSliver 已知回归门控】(复审 #3 新增,已联网核实)**:① **reduced-motion 折叠禁用 `Duration.zero` 的 `toggleAnimationStyle`**(flutter#153889:duration-zero 移除节点并冻结 app)——reduced 下走 `AnimationStyle.noAnimation` 或验证 3.41.9 已修后再用;② **全收起边界**(flutter#178962:所有树收到根抛异常)——压力床加「全收起→再展开」断言不抛;③ 子行 hitTest 命中区(flutter#167928,选 `IndentationType.none`+自绘缩进可部分规避)在 3.41.9 真机实测点 chevron/行不偏(§9)。
- **滚动宿主无界高必崩 + 描边叠角灰尖**:① JsonTree(TreeSliver in CustomScrollView)/ CodeEditor 只读滚 / VersionDiff 主体滚 都是滚动宿主——matrix host 无界高 → 「Vertical viewport unbounded height」一 pump 即崩。specimen 必须用 `GallerySpecimen.height`(如 360)给有界高 + 大高度无溢出滚动断言(同 G4 AnPage/AnInspector 坑)。② 单框描边**禁半透明 border**——半透明 border+圆角四角自叠成「灰尖」(Retina 2x 尤明显)。用 `AnCard` 既有实色 line border + borderRadius,bare/inline 变体隐框透明底。
- **编辑态透明 TextField 叠高亮 pre 必须 pixel-perfect 同步(复审 #13 重写)**:CodeEditor 编辑态**自绘透明 TextField**(`style: AnText.mono.copyWith(color: transparent)` + `cursorColor: accent` + `decoration: collapsed`/无 + `maxLines: null`)叠在 `RichText(highlight())` 之上——**不骑 AnInput**(AnInput 渲可见着色文本会双重文本重影、box chrome 不匹配代码逐行对齐)。透明文本层与高亮展示层**共用同一 TextStyle 引用**(含 fontFamily/fontSize/height/fontFeatures/letterSpacing)+ 相同容器 padding,稍异即滚动错位。行号列同字号同行高(含 tabular),否则行号整倍数错位。**优先评估现成编辑器包**(re_editor/flutter_code_editor 是否已解叠层对齐——原则 #8 先查,见 §6)。
- **trail 行号槽宽足够(复审 #7 升级)**:固定 20px 仅容 2 位数(<100 行即溢出),**v1 动态计宽**(按最大行号位数)或固定下界 ≥36px(容 4 位数)。行号列与代码列共用同一 TextStyle 引用,trail 宽参与三列网格 baseline 对齐验证。
- **wrap 软换行行号对齐**:Flutter wrap 初版退化为等高(wrap 是装饰特性);真需求时走 `TextPainter.computeLineMetrics()`/`getBoxesForSelection()` 逐行计高,**勿简单分行堆叠**。gallery 默认 `wrap=off` + specimen 标注 wrap 下行号不精确(免 review 误判 bug),matrix 不对 wrap 态断行号精确。
- **JsonTree 环检测 + 行高确定性(虚拟化前提)**:构 `TreeSliverNode` 树时带 `seen` 集(`Set<identityHashCode>`)+ 深度上限,命中即出 `[Circular]` leaf 不下钻(jsonDecode 产物无环,但运行时构造的 Map 可能有环→栈溢出)。`treeRowExtentBuilder` 返回确定行高(`AnSize.row=32`)——长字符串若换行破坏定高即破虚拟化;对策:沿用 demo `MAX_VAL` 截断 + 单行 ellipsis,长值走单独 detail。
- **JsonTree 类型分派按 Dart runtime type(复审 #21 新增)**:数据模型=jsonDecode 的 `dynamic`,类型分派按 Dart runtime type(`Map`→object / `List`→array / `String`→string / `num`→number(int/double 不分,统一 number 色)/ `bool`→boolean / `Null`→null),**非 demo 的 `typeof`**(Dart 无 typeof,照搬会错)。`§8 无须 unknown 兜底` 限定为「JSON 六型枚举封闭」;对手构 dynamic 可能传入的意外值(自定义对象)给 `toString` + 中性着色兜底,不 throw。
- **VersionDiff 行号槽对齐(语义铁律)**:行号是**新文件侧逻辑行号**,del 行不计号(`op==del` 不 ++ln、ln 列留空),add/ctx 累进。错位即语义错。三列网格 `[trail | trail | 1fr]` baseline 对齐、行号右对齐 tabular。
- **diff 性能悬崖 + 流式高频重算(复审 #10/#15/#24 + G5.0 复审收敛)**:① **退化闸按 LCS 矩阵 cell 数 `(m+1)(n+1)>4M`**(命名 `lineDiffMaxCells`,忠实移植 demo)——**G5.0 复审修订**:原拟「m+n 行数闸」错(m+n 是不同量纲、封不住 m\*n 矩阵真实成本,平衡型 m=n=2500 双闸全过却撑 ~50MB);v1 是 LCS、确有矩阵,cell 数才是真实成本,一个度量封顶、无平衡型漏网;② cell 闸是**保守占位**——真机压测只「上调」非「首次发现卡顿」;③ **流式重算默认就上 debounce/coalesce**(16~50ms 合并 + microtask/compute,§7),不留「待定」;④ 重算在纯 model 层。
- **JsonTree 构树峰值(复审 #15)**:TreeSliver 虚拟化解了 widget 悬崖,但构 `TreeSliverNode` 树是 upfront 全量;650KB JSON 一次 jsonDecode + 建全节点在主线程仍有 ms~数十 ms 峰值。>N 节点(如 >2000)考虑 isolate/compute 解码或分帧建树,§9 加真机峰值断言。
- **a11y 行级 merge 语义(屏读不逐 token 念)**:① diff 行内多 span RichText,a11y 整行单语义节点(「新增行: <代码>」/「删除行: …」),视觉 token span 全 `ExcludeSemantics`。② JsonTree 行 Semantics 给「<key>: <type> <折叠态>」;**复审 #16:不假设 TreeSliver 自动透 expanded(它自绘行皮 IndentationType.none)——在 `treeNodeBuilder` 返回行根显式 `Semantics(expanded: 该 branch 是否展开)`(leaf 给 null,同 AnRow 不做虚假折叠承诺)**。③ 代码块容器播报「Code block, Python, 42 lines」而非逐行,Tab 不进代码块内逐字符。须真机 VoiceOver 验(§9)。
- **AnScrollBehavior 仅局部、绝不 app-root**:仅 `ScrollConfiguration` 包住 JsonTree/CodeEditor-readonly 可滚区;全局装会碾压 AnPage 故意 overlay thumb → 海洋滚条消失(WRK-039 §2 明示)。
- **着色双色对比(明亮+暗)**:VersionDiff add=okSoft 软底(alpha .12)+ ok/cd-* 字、del=dangerSoft 软底(alpha .10)+ danger/cd-* 字——软底保通透轻盈,**绝不饱和实色填整行**。绿底+绿字可能相融,light/dark 都验 contrast,headless 不足、须真机截图验(§9)。全走 token 禁硬编码。

## §5 逐件建造计划 + 复用图

| 件 | 复用骑乘 | 要点 |
|---|---|---|
| **G5.0 地基** | `AnColors` ThemeExtension(扩 SyntaxColors)· `tokens.dart`(扩 AnSize.trail)· `GallerySpecimen.height`(G4.0 已有)· **同步 `highlight()`**(demo 正则移植 / 或经 openDecision A 定的包封装)· **同步 `lineDiff()`**(demo LCS 移植) | `SyntaxColors`(cd-* 5+arg,双 brightness)+ `AnSize.trail`(动态计宽)+ 同步 `highlight()`(吃 token / CEL 内部分派 / 缺失裸渲)+ 同步 `lineDiff()`(顺序输出/行数阈闸/before 空/del 不计号)+ i18n keys(先 grep 复用既有)。算法纯函数脱 widget 单测先行(含多重复行/全删全增/中段改 golden)。 |
| **G5.1 AnCodeEditor** | `AnCard`(line border+island bg 容器框)· `AnText.mono`(13/1.5 代码+行号)· `AnScrollBehavior`(只读滚局部隐条)· `RawScrollbar`(可读 overlay 按需)· `highlight()`(G5.0)· `AnButton`/`AnInteractive`(复制/编辑钮)· `Clipboard`(复制)· `SelectionArea`(只读三键选) | **只读=`SelectableText.rich(highlight())`**(无叠层最稳);**编辑态=透明 TextField 叠 `RichText(highlight())`**(共用同一 mono TextStyle 引用 + 同 padding,§4;**不骑 AnInput**)。**先评估 re_editor/flutter_code_editor 是否已解叠层对齐**(原则 #8)。变体:`inline`/`editable`/`compact`/`wrap`(初版等高退化)。顶栏复制/换行/编辑钮 + 保存/取消;派 `an-input`/`an-change`;`.value` setter 支持流式(立即 repaint,依同步 highlight)。语言标签走 LANG 映射规范大小写、用 `AnText.meta`(MiSans,**非 mono**)。 |
| **G5.2 AnJsonTree** | `TreeSliver`/`TreeSliverNode`/`TreeSliverController`/`IndentationType.none`· `CustomScrollView`+`AnScrollBehavior`· `AnInteractive`(行 hover/focus)· `AnText.mono`+`value()`(key/值+tabular)· `icons.dart` chevron· `SyntaxColors` | 纯只读结构:object/array 可折叠 summary 行、leaf=key+类型着色 value。**类型分派按 Dart runtime type(§4,非 typeof)**。折叠归 TreeSliver 原生 `toggleAnimationStyle`(**reduced→`AnimationStyle.noAnimation`,禁 Duration.zero**,§4 #153889);缩进靠 `node.depth` 自绘 padding。`MAX_VAL` 长值截断单行 ellipsis;`[Circular]` 环检测;`root='false'` 隐根行;open-depth 默认沿用 demo(有根=2/无根=1)。行根显式 `Semantics(expanded:)`(§4 #16)。复制路径=app 级增强(demo 无,推迟,见 §7/openDecision)。 |
| **G5.3 AnVersionDiff** | `lineDiff()`(G5.0,顺序输出)· `highlight()`(G5.0,行内唯一着色源)· `AnCard`(单框,bare 隐框)· `colors.okSoft/dangerSoft/ok/danger`(行底+字色,已存在)· `AnText.value({mono})`(+N/−N tabular + range 版本号)· `AnText.meta`(note,MiSans)· `AnScrollBehavior`(纵+横滚) | 单框 unified diff(非双栏、非 char-level):三列网格 `[行号 trail | 符号 trail | 代码 1fr]`,add=okSoft 底/del=dangerSoft 底/ctx 无底(§4 软底铁律)。顶栏 `range='v3→v4'` + `note` + +N/−N 统计。props:`before`(null/''→最早版本整段 ctx 不染)/`after`(必传)/`lang`/`range`/`note`/`bare`。**v1 仅单字段文本行 diff** 适配 Function.code / Agent.prompt / Control.when·emit / Approval.template;**Handler 因多部件组装无单一 code 文本,版本 diff 整体推迟**(复审 #23,见 §7)。 |

## §6 建造顺序(单作者、gallery-first、依赖序)

0. **G5.0 地基**:`SyntaxColors`(AnColors 扩 + 重跑 colors/theme 测)+ `AnSize.trail`(动态计宽)+ **(若选包路线)scratch 试装 `flutter pub add` + analyze 冒烟核 SDK** + 封同步 `highlight()` + 封同步 `lineDiff()`(纯函数脱 widget 单测:行数阈/before 空/多重复行/del 不计号/环检测路径)+ i18n keys(先 grep 复用既有)。**算法+token 先就位,三件才有地可踩**。
1. **G5.1 AnCodeEditor** — 通路件(schema code + entity-workspace + chat 右岛全需),且 `highlight()` 复用源、VersionDiff 硬前置,故先落。**先评估现成编辑器包解叠层**。on AnCard+透明 TextField+AnText.mono+highlight。
2. **G5.2 AnJsonTree** — on TreeSliver+CustomScrollView+AnInteractive+SyntaxColors;与 CodeEditor 无依赖、单作者顺序排此。
3. **G5.3 AnVersionDiff** — on lineDiff()+highlight()(硬依赖 G5.0+G5.1 就位);三列行网格 + 软底着色。
4. **集成测试** — 三件 gallery specimen + matrix(**build/no-overflow/render/reduced** 四轴,**a11y 是逐件 dedicated 测试 ensureSemantics+getSemantics,不入 matrix**——复审 #17)+ 压力床。

## §7 推迟(kit ≠ feature)

- **流式驱动**(streamCode/streamDiff)→ chat/entities feature 持 timer。kit 仅出 `.value`/`.after` setter + 立即 repaint(依同步 highlight/diff)+ **默认 debounce/coalesce**(§4)。
- **版本选择列表 + 版本管理**(revert/iterate 端点、版本轨、双列 versionView)→ entities feature。
- **草稿/提交/审批门** → 各 feature 策略不同。
- **嵌套 block-tree / run-terminal / approval-gate facet 拆装** → entity-workspace feature。
- **VersionDiff 结构化多字段 diff**(inputs/outputs/deps JSON 逐键走 JsonTree)→ entities 版本视图 feature。
- **Handler 版本 diff(复审 #23)**:Handler 无单一 code 文本(imports/init_body/shutdown_body/methods/init_args_schema 多部件),v1 不强求覆盖;届时决定 diff 单 method 还是 AssembleClass 整文件 → entities 版本视图 feature。
- **char-level 行内 diff** → 推迟(demo 不做、右岛窄宽行级已够)。
- **side-by-side 双栏 diff** → 推迟(窄宽不适合;v1 unified)。
- **CodeEditor wrap 软换行行号精确对齐**(逐行量高)→ 推迟(初版等高退化)。
- **CEL 专属精细 tokenizer** → 推迟(v1 走 `highlight()` 封装层内部最小正则分派 / register 路线,见 openDecision A2)。
- **JsonTree 复制路径**(JSON-pointer / 点路径)→ app 级增强(demo 无,格式待 openDecision C;与消费 feature 一起落)。
- **JsonTree 真 lazy-node 加载**(1GB+ 流式)→ 推迟(get_flowrun 封顶 80 节点/650KB 在 TreeSliver 舒适区,但 §4 构树峰值要测)。
- **CodePanel 通用基座** → 暂缓(≥2 消费者才抽)。
- **Markdown split-pane 预览 / goto-line / 编辑态语言切换 / count 徽标** → 超 demo floor 或非 G5 scope。

## §8 已解(复审核实,免 build 时再查)

- **【高亮包真实事实(复审 #1/#11/#18 纠正——草案此处全错)】**:`flutter_highlight` = **0.7.0、最后发布约 5 年前、156 likes/130 pub points、uploader unverified、对外产物是 `HighlightView` Widget(非 List<TextSpan>)**;底层 `highlight` 引擎同样 0.7.0/5年前。草案写的「5.0.0/101 likes/139k downloads/Apache-2.0/Myers/活跃」其实是 **diffutil_dart 的数据被错安到它头上**。`highlighting`/`flutter_highlighting`(Akvelon/predatorx7 fork)引擎 3 年陈旧、16/7 likes;`syntax_highlight`(serverpod verified) = 0.5.0/10月前/**15 语言(含 python 无 CEL)/须 async init/TextMate 主题**。**结论:整片成熟 Flutter 高亮包都有硬伤——选型升为 openDecision A,技术推荐 demo 正则移植(同步+List<TextSpan>+cd-* 任意着色+CEL/插值原生+零包风险)。**「拒手搓正则」措辞松绑:对 Python/CEL/JSON 这窄面,移植已验 demo 逻辑不是造轮子。
- **【syntax_highlight 显式否决理由(复审 #2)】**:`await Highlighter.initialize([...])` per-language async 预载(逼 widget Future 化、污染 ZERO-业务纯 props)+ 对一段代码返回**整块单 TextSpan**(无法天然按行切喂三列 diff 网格)+ TextMate 主题(cd-* 双 brightness 要写两份 JSON 资产)+ 仅 15 语言。与本组「同步唯一 highlight() 逐行喂 diff」架构不兼容,**否决**(免 build 时盲换)。留作「要更准多语言且接受 async」的升级路径。
- **【diff 算法(复审 #12 纠正)】**:`diffutil_dart` = 5.0.0/17天前/101 likes/Apache-2.0/纯 Dart——包本身成立。但 `getUpdates()` 是 Android RecyclerView DiffUtil 编辑脚本(index-based on NEW list + 批序 + `Insert/Remove/Change/Move`,为 AnimatedList 增量刷新设计),折成顺序 unified 网格(ctx/del/add 按行序交错 + 新侧累进行号)**远超「~20 行」**(Move 无法映射 unified、Change 须拆 del+add、index 漂移须双指针重排)。**demo LCS 逆向 DP+回溯本就直出顺序 `[op,text][]`、~10 行、已 demo 跑通**——正是 unified 要的形状。**v1 回归 demo LCS 移植**(零包、零投影、顺序输出)。5.0.0 changelog 有 BREAKING(duplicate-heavy lists 改 anchor)——若日后引 diffutil 须 `detectMoves:false` + 压力床含「多重复行/多空行」锚定呈现。`diff_match_patch` 已死(上游 2024-08 archived)——否决。
- **TreeSliver**(Flutter 内置,非包):3.24 入库;`treeNodeBuilder` 虚拟化、`IndentationType.none` 自绘行皮、`treeRowExtentBuilder` 自定行高、`TreeSliverController` 程序化展开。最星标第三方 baumths/flutter_tree_view 2025-03 归档改推它。**采用,但带已知回归清单(#153889 duration-zero 冻结 / #178962 全收起抛异常 / #167928 cross-axis hitTest)+ 3.41.9 实测门控**(§4+§9),非裸断「已稳」。专用 JSON 包全否决(同 §1)。
- **colors.dart 已有 ok/okSoft/danger/dangerSoft 双 brightness**(已核实,lerp/copyWith 全字段)——VersionDiff 行底/字色直接复用,只须新增 cd-* 语法色。
- **typography.dart 两字体(复审 #20)**:`uiFamily='MiSans'`(MiSansVF.ttf,UI)+ `monoFamily='JetBrains Mono'`(JetBrainsMono.ttf,代码),皆已打包变量字体;`mono`/`value({mono})`(tabular)/`meta`/`metaTabular()` 全在——三件代码面用 mono、UI 文字用 meta,无须加字体依赖。
- **tokens.dart 已有 row=32/control=28/tab=34/icon=16/iconLg=20/hairline=1/gripLine=2**——只须新增 `trail`(动态计宽,非 demo 20px)。
- **i18n 既有键(复审 #19)**:`action.{edit,cancel,save}` + `feedback.{error,success,warning,info,dismiss,loading,...}` **已存在**——复用,勿重定义(slang 同 namespace 重复键行为不确定);真新增见 §2。
- **GallerySpecimen.height 字段已于 G4.0 落地**——本批滚动宿主 specimen 直接复用有界高。
- **gallery_matrix_test.dart 轴 = build/no-overflow/render + reduced**(已核实,**无 a11y-role 轴**)——a11y 是逐件 dedicated 测试(`ensureSemantics`+`getSemantics`,同 an_card_test 先例)。
- **AnRow 已透 `expanded` 折叠态语义**(`collapsible? open : null`,已核实)——JsonTree 行 Semantics 沿用此习惯;但 TreeSliver 自绘行不自动透,须在行 builder 显式包(§4 #16)。
- **AnExpandReveal nested-safe**(WRK-039 已验)——但**直接锁定 TreeSliver 原生 `toggleAnimationStyle`、删去 AnExpandReveal 备选**(复审 #17):其高度补间与 `treeRowExtentBuilder` 定高(虚拟化前提)冲突,虚拟化里塞自定义 reveal 得不偿失;reduced 走 `AnimationStyle.noAnimation`。
- **SelectionArea(Flutter 内置 3.3+)**:桌面三键选(Cmd+C)透传原生复制;macOS Impeller 长按无系统菜单 → 复制钮须手搓(Clipboard.setData + toast/按钮态反馈)。Impeller 下 SelectionArea/EditableText 有活跃问题(右键复制后手势失效 3.29–3.33、selectable 光标命中区、text transform 错位)——§9 真机核。
- **后端契约字段已钉死**:Function(code Python + inputs/outputs/deps/python版本/env)/Handler(imports/init_body/shutdown_body/**methods**/init_args_schema 多部件,无单一 code)/Agent(prompt/skill/...)/Workflow/Control(when/emit CEL)/Approval(模板 markdown+{{CEL}}) 各 version 快照精确字段已绑定;CEL 变量作用域(input.* / node_id.field / sensor output / approval 模板)已明。get_flowrun LLM 工具封顶 80 节点/~650KB(REST 不限,故虚拟化是硬需求)。JSON 值是 6 标准型封闭集(类型着色可穷举无须 unknown 兜底——限定 jsonDecode 来源;手构 dynamic 意外值仍给 toString 兜底,§4 #21)。

## §9 build 时真机/核值(verify-by-real-run)

- **(选包路线)scratch 试装核 SDK**:`flutter pub add <候选>` + `flutter analyze` 在 Dart 3.11.5 / Flutter 3.41.9 下 resolve + null-safety 通过 + 一个 Python/JSON parse 冒烟(5 年前的引擎包能否编译是真实未验风险)。
- **TreeSliver 回归实测**(复审 #3):3.41.9 实测 ① reduced-motion 折叠不冻(#153889)② 全收起→再展开不抛(#178962)③ 子行 hitTest 命中区(点 chevron/行不偏,#167928)。
- **diff 防卡阈值实测**:`lineDiffMaxCells`((m+1)(n+1) cell 闸,占位 4M)真机压测定(扩 catalog-stress 海量增删到 2000 行级);保守占位、真机只上调。流式 `:iterate` 逐 chunk debounce 窗口真机调参。
- **JsonTree 构树峰值**(复审 #15):650KB / >2000 节点 jsonDecode + 建树主线程峰值断言(是否需 isolate/compute)+ 展开/折叠流畅无冻 — Impeller 截图验。
- **CodeEditor 编辑态叠层 Impeller 实测**(复审 #6 具体化):透明文字层 caret 是否对齐高亮字形、选区高亮是否错位、tab 插入后光标位、长文本(5000 行)滚动同步是否抖、IME(中文)candidate 框定位 — MacBook M3+ 逐项截图+录屏,勿 headless 声称完成。
- **VersionDiff 双色对比**(各语言 py/js/cel light+dark 着色清晰、绿底绿字不相融)— Impeller 截图验。
- **全件 a11y**(JsonTree 行 expanded/collapsed + 值行、VersionDiff 行 merge + 增删/行号、CodeEditor 容器播报)— **真机 VoiceOver 验**(逐件 dedicated 测试 + 真机)。
- **macOS 复制**:SelectionArea + native 三键选是否工作、Clipboard 异步是否卡 UI、长文本复制是否超时、复制无权限弹窗 — 真机验。

## 附:字体/着色/性能/a11y/动画 跨切约定

- **字体/排版(两面)**:代码面/行号/+N−N/range 走 `AnText.mono`(JetBrains Mono 13px/1.5/tabular);UI 文字(顶栏语言标签/note/说明)走 `AnText.meta`(MiSans)——**勿顶栏全 mono 化**(复审 #20)。tab 宽=4 空格;ligature 保留。行号列与代码区**同 TextStyle 引用**(§4)。
- **着色单源**:`SyntaxColors` ThemeExtension(cd-* 5+arg,双 brightness)登 colors.dart,三件共用、禁内联 Color 字面量。`highlight()` 是唯一 tokenizer 入口(CEL 走封装层内部最小正则分派,仍唯一入口)。
- **性能上限**:JSON 走 TreeSliver 虚拟化 + 构树峰值看护(>2000 节点考虑 isolate)· 代码 >5000 行考虑虚拟化/截断 · diff 退化闸按 LCS 矩阵 cell 数 `(m+1)(n+1)>4M`(`lineDiffMaxCells`,G5.0 复审修订)+ 流式默认 debounce。重算在纯 model 层。
- **a11y**:容器 `Semantics(label: 语言+行数)`;diff 行/树行 merge 语义(token span ExcludeSemantics)、播报折叠态/增删/行号(TreeSliver 行须**显式** `Semantics(expanded:)`,不假设自动透);桌面手搓复制钮;Tab 不进代码块内逐字符。逐件 dedicated a11y 测试(非 matrix 轴)+ 真机 VoiceOver。
- **reduced-motion**:三件本身无功能动效;装饰性动(JSON 树 chevron rotate / TreeSliver 折叠)→ **`AnimationStyle.noAnimation`(禁 Duration.zero,#153889)** + `AnMotionPref.reducedOrAssistive` 闸。gallery_matrix reduced 轴断言无动画(否则 pumpAndSettle 超时)。
- **滚动条两诉求拆清**:CodeEditor 可读=RawScrollbar(overlay 按需);JsonTree/CodeEditor-readonly=AnScrollBehavior 局部彻底隐;VersionDiff=纵滚 + 横滚(超长行 pre 不 wrap、代码列 horizontal SingleChildScrollView、行号/符号列固定,两滚向勿混进一个 viewport)。**AnScrollBehavior 绝不 app-root**(§4)。
- **测试矩阵**:每件 gallery specimen + matrix(**build / no-overflow / render / reduced** 四轴)+ 逐件 dedicated a11y(`ensureSemantics`+`getSemantics`)+ 压力床。**压力床去 demo「注入 XSS」(Flutter TextSpan 无 HTML 注入面、测不到东西——复审 #8),替为 Flutter 真风险**:超长无换行单行(横滚不撑无界宽)· 控制字符/CRLF 混用行分割 · emoji+CJK 字素簇(行号/光标/截断对齐)· 深嵌套 200 层(栈+虚拟化)· 超大/NaN/Infinity 数值 · 空/海量/[Circular]/无变更/最早版本/**全收起边界**(#178962)/**多重复行多空行**(diff 锚点)。escape-safe 断言保留但语义改为「特殊字符作纯文本正确渲染」(非「防注入」)。算法纯函数脱 widget 单测;有状态件用 `_XxxDemo` StatefulWidget 持态。codegen 产物入库。建造前重跑 AnColors/AnText/AnCard/AnScrollBehavior/AnRow 既有测试防地基波及。

## 复审纪要(3 镜对抗复审 24 项 → 折入/驳回)

**确认为真并折入(全部 HIGH + 多数 MED/LOW)**:
- **#1/#11/#18(HIGH,联网核实)**:flutter_highlight 是 0.7.0/5年前/unverified/返 Widget,草案「5.0.0/101likes/活跃/支持3.41」是 diffutil 数据误植。**高亮包从「即锁定」降为 openDecision A**,技术推荐 demo 正则同步移植;§8 重写为真实事实。
- **#2(HIGH)**:syntax_highlight async-init + 整块单 TextSpan 与同步逐行架构冲突 → §8 显式否决登记。
- **#3(HIGH,联网核实)**:TreeSliver 回归 #153889(duration-zero 冻结)/#178962(全收起抛异常)/#167928(hitTest)→ §4 新增门控铁律 + §9 实测项 + §8 改「带回归清单采用」。
- **#12(HIGH)**:diffutil getUpdates() 投影远超「~20 行」、demo LCS 本就直出顺序 → **diff 包从「即锁定」降为 openDecision B**,v1 回归 demo LCS 移植。
- **#4(MED)**:同步性升为 §4 铁律(异步包一律否决)。
- **#13(MED)**:CodeEditor 编辑态不骑 AnInput(渲可见色文本不匹配透明叠层)→ 改自绘透明 TextField,§4/§5/§6 修正。
- **#5(MED)**:diffutil 5.0.0 BREAKING dup-anchor → 压力床加多重复行/多空行;若引 diffutil 须 detectMoves:false(已记 §8)。
- **#6(MED)**:Impeller 编辑态叠层 caret/选区/IME 风险 → §9 具体化 + §5/§6 先评估现成编辑器包。
- **#7(MED)**:trail 20px 仅容 2 位数(<100 行溢出)→ 升 §4 必处理,v1 动态计宽 / 下界 ≥36px。
- **#8(MED)**:XSS 压力床是 Flutter 伪命题 → 删除,替为超长单行/CRLF/emoji-CJK/深嵌套/极值数 真风险。
- **#15(MED)**:perf 占位 4M 太乐观 + 流式 debounce 应默认开 + 构树峰值 → §4 收敛(保守下界 + debounce 默认 + 构树峰值看护)。
- **#19(MED)**:i18n action.{edit,cancel,save}/feedback.error 已存在 → §2 拆「复用既有/真新增」+ 动工前 grep。
- **#20(MED)**:两字体(MiSans UI + JetBrains Mono code)→ §0/附 纠正,UI 文字勿 mono 化。
- **#21(MED)**:JSON 按 Dart runtime type 分派(非 typeof)+ 手构意外值 toString 兜底 → §4 新增。
- **#16(LOW)**:TreeSliver 不自动透 expanded → 行 builder 显式 Semantics(expanded:),§4。
- **#17(LOW)**:a11y 非 matrix 轴(matrix=build/no-overflow/render/reduced)+ JsonTree 折叠锁 TreeSliver 原生删 AnExpandReveal 备选 → §6/§8/附 修正。
- **#9(LOW)**:换包后 CEL 仍在覆盖外,可选 registerLanguage 注入最小 CEL mode → openDecision A2。
- **#10/#24(LOW)**:LCS_CELL_CAP 对 Myers 不适用、命名误导 → 退化闸改按总行数(m+n)+ 字节闸,命名 `lineDiffMaxLines`。
- **#22(LOW)**:openDecision A(CEL B 方案)的 why 去掉「demo 通用正则同等覆盖」前提,明确 CEL 须封装层内部最小正则分派。
- **#23(LOW)**:删无据「95%」;Handler 多部件无单一 code,版本 diff 整体推迟 → openDecision D + §7。
- **#14(MED)**:syntax_highlight 三约束(async/TextMate/15语言)显式纳入 openDecision A 候选(草案只字未提是盲点)。

**部分修正(方向对、量级纠正)**:
- 草案 §8「flutter_highlight 维护活跃」整条删除改写为真实数据(否则误导 build 时 pub add 死包)。
- 草案对 diffutil「~20 行投影」的低估纠正为「数十行 + Move 不可映射 unified + Change 拆 del+add」。

**驳回/不采纳**:无明确误报需驳回——24 项均有物理价值。唯 #9 关于 highlighting 是否含 Python 的 parse 兜底行为属实现期实测细节,不阻塞建造、归 §3 核验项。

## 决策详录(已于 2026-06-24 拍板,见 §1 锁定决策;此处留选项/理由全录)

> 复审把「高亮引擎」「diff 算法」从草案的伪锁定升回决策——两者都是「移植 demo 已验确定性逻辑 vs 引第三方包」的 #8 取舍,且整片成熟 Flutter 高亮/diff 包经联网核实多陈旧/与本组同步契约冲突。**封装层 `highlight()` / `lineDiff()` 唯一入口无论选哪个都保留**(隔离选型、换引擎只动一处)。

**决策 1 — 代码高亮引擎**(复审证伪:草案锁的 `flutter_highlight` 实为 0.7.0 / ~5 年前 / unverified / 返 Widget;整片成熟包均有硬伤)
- **A**(推荐)移植 demo 正则 tokenizer 成同步 `highlight()→List<TextSpan>`:同步 + cd-* 任意着色 + CEL/插值原生 + 零包风险;窄面 Python/CEL/JSON 准度足够。
- **B** `highlighting`(Akvelon)/`flutter_highlighting`(fork)引擎:highlight.js 端口、同步 parse 返 Node 树可 map TextSpan、~190 语言含 Python——但引擎 ~3 年陈旧、低 likes,须 scratch 试装核 Dart 3.11.5 resolve。
- **C** serverpod `syntax_highlight`:verified / 返 TextSpan / 15 语言含 Python——但须 async `Highlighter.initialize()` 预载(违 §4 同步铁律)+ 整块单 TextSpan 难按行喂 diff + TextMate 主题须写两份 JSON。**已被 §4/§8 显式否决**。
- **理由**:A 是唯一同时满足本组四条硬约束(同步 / 直出 List<TextSpan> / cd-* 任意着色 / CEL 原生)的方案;窄面移植已验逻辑非「造轮子」。B 留作「日后要宽多语言准度」升级路径(仍走同一封装入口)。

**决策 2 — CEL 高亮 v1**(任何引擎都不含 CEL DSL;CEL 是后端第二种代码、高频:control when·emit / approval 模板 `{{CEL}}` / workflow Input / sensor 条件)
- **A** 走 `highlight()` 兜底裸渲(纯文本无色——但会误导「无语言支持」)。
- **B**(推荐)在 `highlight()` 封装层内部对 `lang==cel` 加最小正则分派(插值 `{{ }}`/`${ }` + 注释 + 字符串 + 数字),对外仍唯一入口、绝不另起第二套 tokenizer。
- **C** 若选决策 1 的包路线(B/C 引擎)则用包的 `registerLanguage` 注入最小自定义 CEL mode(更正规、仍在唯一入口内)。
- **理由**:approval 模板高频,A 全裸渲误导;B 实质是封装层内一段最小正则(归实现细节、非独立 tokenizer);若已走包路线则 C 更正规(#8 用包能力)。CEL 专属精细 tokenizer 推迟。

**决策 3 — JsonTree 复制路径格式**(app 级增强,demo 无;复制本身可推迟到消费 feature,但格式须先定)
- **A**(推荐)JSON-pointer(`/a/b/0`,RFC 6901、机器友好、特殊 key 标准转义、可直接喂后端/CEL `node_id.field` 寻址)。
- **B** 点路径(`a.b[0]`,人友好但含 `.`/引号的 key 转义须自创规则、易错)。
- **C** 两者皆给(复制 pointer + 复制点路径两个右键动作)。
- **理由**:本项目消费方(flowrun 结果/schema 寻址)偏机器语义、JSON-pointer 是 RFC 标准 → A 优先;若产品要给用户读路径则 C。

**决策 4 — VersionDiff v1 scope**
- **A**(推荐)v1 仅单字段文本行 diff(适配 Function.code / Agent.prompt / Control.when·emit / Approval.template;对齐 demo floor)。
- **B** v1 连结构化多字段 diff(inputs/outputs/deps JSON 逐键走 JsonTree、Handler 组件级)。
- **理由**:结构化字段不应 pretty-print 后跑文本 diff(键序/缩进噪声);须走 JsonTree 逐键比对、与真实消费场景(entities 版本视图)一起设计。**关键边界**:Handler 版本快照是多部件(imports/init_body/.../methods/init_args_schema)、无单一 code 文本——v1 单字段文本 diff 对 Handler 无从适配,其版本 diff 整体推迟,免 v1 kit 形状被 Handler 特例污染。

**决策 5 — 行级 diff 算法**(复审证伪:草案锁的 `diffutil_dart`「~20 行薄投影」被严重低估——`getUpdates()` index-based + Move/Change + 批序,折成顺序 unified 网格非平凡)
- **A**(推荐)移植 demo LCS 成同步 `lineDiff(before,after)→List<(DiffOp,String)>`:逆向 DP + 回溯直出顺序 ctx/add/del、~10 行、demo 已跑通、零包、零投影层、正合 unified 网格。
- **B** 引 `diffutil_dart`(Myers O(ND) 内存优于 LCS O(mn))+ 自写顺序重投影层(`detectMoves:false` 禁 Move、Change 拆 del+add、index 漂移双指针重排,远超 20 行)+ 边界单测。
- **理由**:line-level LCS 是教科书确定性算法、非窗口 chrome/红绿灯那类几何边界 footgun(原则 #8「手搓跌跟头」针对平台/几何边界、非纯算法);demo LCS 本就直出 unified 要的形状。退化闸按 LCS 矩阵 cell 数 `(m+1)(n+1)>4M`(命名 `lineDiffMaxCells`,忠实移植 demo;**G5.0 复审修订**:原拟 m+n 行数闸被实测证伪——见末「G5.0 实施修订」)。B 留作真机压测证 LCS 在 2000 行级卡顿后的升级路径(保留同一出参契约)。

## G5 实施修订(2026-06-24,逐件落地随建随记)

> 建造按 §6 顺序逐件落,每件「主控写 → analyze/test/截图 → 对抗复审 → 折入 → 提交」。各件落地后此处记实测偏离调研之处(WRK 是过程记录;最终随件 landed 进 design-system.md)。

**G5.0 地基 — 已落(`SyntaxColors` / `AnSize.trail` / `AnText.code` / `highlightCode` / `lineDiff`,+29 单测,fe-verify 绿)**:
- **退化闸:`m+n` 行数闸 → `(m+1)(n+1)>4M` cell 闸(忠实移植 demo)**。G5.0 对抗复审(LCS 镜)证伪原研究的 m+n 决策:m+n 与 m\*n 不同量纲、**封不住** DP 矩阵真实成本——平衡型 `m=n=2500` 仅 5000 行却双闸全过、撑 ~6.25M cell≈50MB,比 demo 的 4M cell 闸**更激进**(放行了 demo 会退化的 case),且与「保守占位 / #8 忠实移植」自相矛盾。改回 demo 的乘积闸 `lineDiffMaxCells=4_000_000`,一个度量直接封顶真实成本、无平衡型漏网;原 `lineDiffMaxTotalBytes` 字节闸(String.length 是 UTF-16 code unit 非字节、命名误导且 cell 闸已封顶真实成本)一并删除。回归测试 `code_diff_test`「BALANCED diff trips the cell cap」锁住。研究当初的 m+n 论证误把「Myers 无矩阵」套到 v1 的 LCS 上(LCS 确有矩阵)。
- **`highlightCode` 唯一入口 + CEL 天然覆盖**:demo 单正则 tokenizer 语言无关,CEL 插值 `{{}}`/`${}` 经 arg 组天然上色——决策 4「封装层内 CEL 上色」由统一 tokenizer 直接满足,无须额外 cel 分支(`lang` 参仅为 API/标签稳定)。`_followedByParen` 只认 ASCII 空白(有意窄于 JS `\s`、避 per-identifier substring 的 O(n²);代码前 Unicode 空白不存于真实源码),复审记为可接受偏离。
- **`SyntaxColors` = 独立 ThemeExtension**(非塞进 AnColors):语法子板自成一概念、不胀 280 行的 AnColors;`arg` 按值镜像 `AnColors.accent`(`single-source` 妥协,测试锁不变式)。`AnText.code` = mono 12/1.6(demo `--t-meta`/`--lh-prose`,区别于 13/1.5 的内联 `mono`)。`AnSize.trail=36`(demo 20px 在 mono 仅容 ~2 位数,升为容 4 位数的下界)。

**G5.1 AnCodeEditor — 已落(`core/ui/an_code_editor.dart`,13 单测 + matrix 四轴 + 截图验,fe-verify 绿)**:
- **编辑改用 `_HighlightController.buildTextSpan`、非透明叠层(架构升级、消解 WRK #1 HIGH)**:WRK-040 §4/§5 原拟「透明 TextField 叠 RichField(highlight)+ pixel-perfect 对齐」(demo web 的 textarea-over-pre 移植)。改用 Flutter 原生机制——自定义 `TextEditingController` 重写 `buildTextSpan` 返回高亮 span,field 自渲染着色文本:光标/选区/滚动全原生、**零叠层对齐风险**(原拟的 #1 HIGH 直接消失),更 #8。read-only 用 `SelectableText.rich(highlightCode)`,两路同走唯一 `highlightCode`。`buildTextSpan` 内读 `context.syntax` → 自动跟主题。
- **`re_editor` 已评估否决(#8 尽调)**:成熟、自绘渲染(解大文本性能+叠层),但其高亮基于 `re_highlight`(自带 tokenizer)、与决策 1「highlightCode 唯一同步源」冲突(CEL 无覆盖、cd-* 主题要另写、三件高亮源分裂)。忠实于用户决策 1,移植 demo。
- **scroll-host 双态(LayoutBuilder)**:内容高(无界父=AnPage/AnInspector 滚)/ 有界则 `Flexible(SingleChildScrollView)` body 纵滚、bar 固定——两态都不崩(解 WRK §4 滚动宿主)。read-only 非 wrap 横滚;editable soft-wrap(TextField 不能净 nowrap+hscroll,inline 短、全码编辑罕见,v1 简化)。
- **框底 = `c.surface` 白岛**(对齐全 kit + demo `--island`,非 surfaceSubtle);私有 `_codeFrame`(DecoratedBox 实色内描边 + ClipRRect)非 AnCard(AnCard 强制 pad/无 clip/无 focus 边不适配);G5.3 出现第二消费者再抽共享。
- **G5.1 对抗复审(3 镜 17 项,全探针实证零误报)折入**:**HIGH** 编辑态行号/a11y 与文本脱钩(敲换行行号冻结、丢 demo per-keystroke repaint)→ 控制器挂 listener、文本变化 `setState` 重算 gutter+a11y(光标由控制器保留)+ 回归测试。**MED**:① lang 标签 `ExcludeSemantics`(否则容器 label 念两遍 Python)② gutter `ExcludeSemantics`(否则屏读逐个念行号)③ copy 失败翻 `feedback.copyFailed` tooltip(消死键 + 诚实反馈)④ Tab 拦为插 4 空格(`CallbackShortcuts`,同 demo,非跳焦点)⑤ 钮间 4px gap。**LOW**:wrap specimen 标注 v1 行号等高 + doc 注无虚拟化/每键全量 tokenize 上限(轻编辑定位)。
- **i18n 新增**:`action.{copy,wrap}` · `feedback.{copied,copyFailed}` · `a11y.{codeBlock,codeBlockPlain}`(复用既有 `action.{edit,cancel,save}`);新图标 `AnIcons.copy`。a11y 容器播报「Code block, {lang}, N lines」。

**G5.2 AnJsonTree — 已落(`core/ui/an_json_tree.dart`,11 单测含 dedicated a11y + matrix 四轴 + 截图验,fe-verify 绿)**:
- **Flutter 内置 `TreeSliver` 虚拟化**(`treeNodeBuilder` 只建可视行解 650KB 悬崖)+ `IndentationType.none` 自绘缩进(depth*iconLg);`treeRowExtentBuilder`→`AnSize.row`;`TreeSliverController.toggleNode`;`re_highlight`/专用 JSON 包全否决(同 §1)。
- **TreeSliver 回归——SDK 源码 + 探针实测核实(纠正研究期推测)**:① **#153889 在 3.41.9 已修**(noAnimation 与 Duration.zero 两支都 skip;仍用 `AnimationStyle.noAnimation` 作更干净的 reduced 路径)② **#167928 经 `IndentationType.none` 化解**(自绘缩进→paint offset 0→hit==paint)③ **研究期写的 #178962 实为 #170757 的 duplicate、仍 OPEN**(快速双击展开中 in-flight 动画的 paint-time null;3.41.9 探针 6 轮全收起+快速 toggle 未复现,**残留真机风险**→§9 真机验快速双击,若崩则 toggle 去抖或恒 noAnimation)。
- **有界高滚动宿主(SDK 确认 TreeSliver 不支持 shrinkWrap、RenderTreeSliver 在 shrink-wrapping viewport 抛)**:AnJsonTree 须父给有界高(同 AnPage);去掉了原拟的 shrinkWrap 回退(实测崩);小树内联流入页面的非虚拟化 Column 回退**推迟**(当前消费方均有界,doc 记为已知取舍)。
- **类型按 Dart runtime type 分派**(Map/List/String/num/bool/Null,非 demo typeof)+ unknown→toString 兜底;环检测 identity seen 祖先集→`[Circular]`;节点 upfront 建 + 封顶 `_maxNodes=2000`(→`N more`)+ `_maxVal=500`。
- **G5.2 对抗复审(3 镜 16 项、SDK 源码逐字核实 + 6 探针、剔 1 误报)折入**:**HIGH** ① 缺容器级 `Semantics(label「JSON tree, N items」)`(兄弟件已有)→ 补 + 新 i18n `a11y.jsonTree` ② 无 dedicated a11y 测试 → 补(分支 `isButton`+`isExpanded` 翻转、容器 label)。**MED** ① 空 `{}`/`[]` 渲成可点假分支 → 改 leaf(`isBranch` 需 `children` 非空)② leaf 单 `Text.rich` 长 key 吞 value → 改 `Row[Flexible(flex2 key) | ': ' | Flexible(flex5 value)]` 独立省略(demo 双轨等价)。**LOW** 截断用 `inkFaint`(非 danger,只 circular 是错)· null 叶补斜体 · 注释纠正(#153889 已修 / #178962→#170757)。
- **取舍记录**:key 与 value 均 `AnText.code` mono(全等宽对齐,demo key 走 UI 字体——本组为代码面对齐取舍);a11y 测试用当前 `isSemantics` 匹配器(`hasFlag`/`containsSemantics` 已弃用)。i18n 新增 `tree.{invalidJson,circular,moreItems}` + `a11y.jsonTree`。

