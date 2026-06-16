# demo/ 契约与协作宪法

> demo/ 是 Foryx 形态事实源的**低耦合·高内聚重做**（取代 design-lab）。纯静态、不连后端、不进任何门禁。
> 本文件是**法律**：六契约 + 文件归属 + 防撞铁律。多个 AI 各自从同一 `main` fork、各打磨一块，遵此则永不冲突。

---

## 一、三句话架构

1. **manifest 装配**（防打架）：`manifest.js` 是唯一注册表；`app.html` / `index.html` / 四导航 / 画廊全由它生成。加/改一个海洋 = **append manifest 一行 + 只动自己文件夹**，永不手改共享文件。
2. **组件库引擎**（杀复制）：`core/components/*` 一组件一文件 = 一份实现 = 一个主人；海洋只**组合**（调 `window.X` 工厂），不重造像素。
3. **六契约接线**（低耦合）：令牌 / 外壳槽 / 组件 API / Intent 选中 / Live 实时 / 数据 DTO——海洋只依赖这六个显式缝，不碰内部、不碰彼此。也是未来 Flutter 对接边界。

## 二、目录 = 归属

```
demo/
├── manifest.js          # 注册表（唯一可被功能作者编辑的共享文件，且只 append 一行）
├── app.html · index.html # 生成式 host（永不按 feature 编辑；core 主人维护）
├── core/                # 🔒 内核（主人 = core）。features 只读消费
│   ├── tokens.css       #   令牌契约：唯一调色板（语义名 + --cc-* 别名）
│   ├── reset.css · shell.css · sidebar.css  # 重置 + 布局框架 + 侧栏 chrome
│   ├── shell.js         #   外壳槽契约：Shell.registerOcean/mount/headExtra/headLead/crumb/body/returnTo
│   ├── sidebar.js       #   四导航(manifest 生成) + 头像/铃铛两轴 + 收起拖拽 + peek
│   ├── intent.js        #   选中契约：Intent.select/on/act/back
│   ├── live.js          #   实时契约：Live.on（messages/entities/notifications 三流）
│   ├── loader.js · boot.js · gallery.js · cssload.js · dom.js · icons.js
│   ├── config/          #   归一表：entity-kinds(9 类) · state-model(状态)
│   └── components/      #   🧩 组件库（21 件，一组件一文件一主人，自载 .css）
├── features/<id>/       # 🌊 海洋（一文件夹一主人，SEALED）：sea.js · rail.js · *.css (· 局部辅助如 editor.js)
└── mock/<域>.js         # 📊 数据层（DTO 镜像后端 references/）：window.MOCK_<域>
```

---

## 三、六契约

### ① 令牌契约 `core/tokens.css`
自定义属性**只能**在此定义。语义名为真（`--sea/--island*/--ink*/--accent*/--ok/--warn/--danger/--line*/--r-*/--s*/--t-*/--ease-*/--cd-*/--shadow-*`）；`--cc-*` = 旧交互别名（保 design-lab 值字节级不漂移）。**禁裸 hex/rgb**（features/components 一律走令牌；唯二例外：红绿灯三色、阴影 token 内部 rgba）。圆角走尺度：块 `--r-card` · 菜单 `--r-chip` · 钮/行 `--r-btn` · 小标签 `--r-tag` · 药丸 `--r-pill`。

### ② 外壳槽契约 `core/shell.js`（+ shell.css）
三槽：`#left`（侧栏填）· `#sea`（海洋海面填）· `#body`（海洋把右岛 append 进来，带 `data-ocean-right=<id>`，mount 时据此清理）。
API：`Shell.registerOcean(id,{crumb,build(sea)})` · `Shell.mount(id)`（清 sea/head-extra/右岛，记 `Shell._ocean`）· `Shell.headExtra(html)` · `Shell.headLead`（getter，中性领位槽）· `Shell.crumb(t)` · `Shell.toOcean(id)`（=侧栏 nav，返回 Promise）· `Shell.returnTo(id)`。

### ③ 组件 API 契约 `core/components/*`
工厂律：`X.method(host?, opts) → html | {el, ...handle}`。每个组件：(a) 一文件 (b) 首行 `if(window.cssNextTo) cssNextTo(document.currentScript)` 自载同名 .css (c) **只读令牌/config/icons** (d) 类名 `fg-<name>-*` 前缀 (e) 点击实体/run/doc 引用**发 `Intent.select`**、绝不调 feature。21 件 + API 见 `reference.html`（活体规格台）：
`StatusDot RefPill ThinTable KV Tabs Attention Tags Segmented Dropdown RunChip IterationStepper`（叶子）·
`CodeEditor Floating RightIsland ApprovalGate RunDebug BlockKit RunGraph VersionDiff Flowrun EntityCard`（复合）。

### ④ 选中/意图契约 `core/intent.js`
`Intent.select({kind,id,source?})` kind∈{entity,document,workflow,run,node,conversation,notification} → 据 `manifest.owns` 解析归属海洋 → 切到它（同海洋内只派发不重挂，保侧栏高亮）→ 派发给 `Intent.on(kind)` 处理器。
`Intent.on(kind,fn)` 海洋订阅自己的 kind · `Intent.act({verb,...})` 非导航动作（verb 对齐 N5 :run/:call/:invoke/:trigger/:iterate/:decide）· `Intent.push/back` 真返回栈。
**ref-pill / 侧栏行 / 图节点 / 文档反链 全走 Intent.select——一个前门、零跨海洋 import。**

### ⑤ 实时契约 `core/live.js`
`Live.on(scope,fn)→{close()}`，scope∈{messages,entities,notifications}（E1 铁律：仅三流，workspace 级，bus 做 demux）。ephemeral 帧 `seq=0`（flowrun tick / AI 编辑 delta）只改瞬时态；durable `seq>0` 才进耐久缓存。现为 mock 发射器，接后端整体换 SseGateway、订阅契约不变。

### ⑥ 数据 DTO 契约 `mock/<域>.js`
一域一文件，暴露 `window.MOCK_<域>`，camelCase 镜像后端 `references/` DTO（9 实体 kind、run 形 `{nodes,edges,loopbacks,st,taken,ghost,live,iters,ports,memo}`、document/notification/model 等）。海洋经 `manifest.data` 声明依赖，loader 在 sea/rail 前加载。

### manifest 契约 `manifest.js`
`window.MANIFEST = [...]` append-only，一海洋一对象：`{id,label,icon,nav,default?,axis?,owns[],data[],sea?,rail?,standalone?,gallery?,desc?}`。`boot.js`/`gallery.js`/侧栏导航**据它生成**。

---

## 四、防撞铁律（多 AI 不打架）

1. **一文件一主人**：`features/<id>/**` = 该海洋作者；`core/components/<x>.{js,css}` = 该组件作者；`core/{tokens,shell,sidebar,intent,live,loader,boot,...}` + `manifest` = core 主人。
2. **唯一共享写 = manifest append 一行**：`app.html`/`index.html`/四导航/CSS 清单全生成。加海洋 = append 一行 + 建自己文件夹；改自己海洋 = 只动自己那行 + 自己文件夹。
3. **自载 CSS**：组件/海洋经 `cssNextTo` 各自注入，无中央 `<head>` 样式清单可争；任意入口都能挂任意海洋（根治「海面待接入」）。组件库全量常驻 `app.html`（core 库、非 feature，加组件 = core 加一行）。
4. **禁跨 feature import**：海洋不引 `features/<别的>/`、不读别人的全局/CSS。跨海洋只走 `Intent` / `Live`。
5. **一概念一份实现**：重复 UI 落 `core/components/`（一次），不在海洋里抄。
6. **令牌只写一次** + **裸色禁令**：仅 `tokens.css` 定义属性；features/components CSS 禁裸 hex。
7. **CSS 命名律**：组件类 `fg-*`；海洋类带海洋前缀（`chat-/ent-/sch-/doc-/set-/onb-/ntf-`），只活在自己 .css。
8. **通道非全局**：海洋间只经 `Intent`/`Live`（append-only 订阅），绝不写彼此全局；`icons.js` 与 `manifest.js` 也 append-only。

---

## 五、加一个新海洋（零冲突套路）

1. `manifest.js` append 一行 `{id,label,icon,nav/axis,owns,data,sea,rail,gallery,desc}`。
2. 建 `features/<id>/sea.js`（`Shell.registerOcean` + `Intent.on(owned-kind)`）+ `rail.js`（`SideBar.register`）+ `*.css`（自载、海洋前缀）。
3. 建 `mock/<域>.js`（`window.MOCK_<域>`）。
4. 海面/侧栏**只调组件库**；缺组件 → 提给 core 落 `core/components/`，别在海洋内造。
5. 完事：四导航/画廊/app 自动有它——零额外触点。

## 六、预览 & 门禁

- 预览：起静态 server 服务 `demo/`（建议带 `Cache-Control: no-store`），开 `app.html`（整体）/ `reference.html`（组件库活体规格）/ `index.html`（画廊）。
- 提交：**只 `git add demo/`**（main-only、不开分支）；commit 后推 origin/main。
- 自检：组件/海洋 CSS 零裸色（`grep` 验）、JS 全 `cssNextTo`、海洋零跨 feature、活壳 0 报错。
