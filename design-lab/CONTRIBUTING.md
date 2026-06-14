# design-lab 并发协作约定（多 AI 同时打磨，不打架）

> 多个 AI 各自从同一个 `main` fork、各自打磨一个模块。本文件是**主人地图 + 边界规则**。
> 核心一句话：**一文件一主人 + 共享内核只读 + 海洋自包含**——做到这三点，
> 大家改的永远是不同文件，`main` 上 `git` 永不冲突（本仓库 main-only、不开分支）。

---

## 一、目录 = 模块边界

```
design-lab/
├── shared/                 # 🔒 内核（设计系统）。只读消费；要改找内核负责人
│   ├── tokens.css          #    颜色 / 圆角 / 间距 / 字体 / --ease-spring
│   ├── shell.css           #    窗口框架 + 全站通用原语（.win/.body/.main/.sea/.ibtn/.chev）
│   ├── icons.js            #    图标集（APPEND-ONLY）→ icon('chat',16)
│   └── shell.js            #    外壳：搭框、开三槽(#left/#sea/.body)、海洋挂载 API
├── sidebar/                # 🧩 左侧栏模块（一人）。自挂载进 #left
│   ├── sidebar.css
│   └── sidebar.js
├── onboarding/             # 🧩 Onboarding 模块（一人）。独立整页
│   └── onboarding.html
├── oceans/                 # 🌊 每个海洋 = 一个文件夹（一人负责整个文件夹）
│   └── chat/               #    Chat 海洋：含「很多设计块」，都在本文件夹内
│       ├── chat.html       #      预览 harness
│       ├── chat.css        #      本海洋全部样式
│       ├── chat.js         #      编排：对话流 + 工具块 + composer + 信号交互
│       └── entity-card.js  #      右岛块（实体卡）—— 右岛属于海洋
│   └── …                   #    后续：entities/ · scheduler/ · documents/（各一个文件夹）
├── reference.html          # 📐 设计参考页（组件规格）。内核负责人维护
├── PHILOSOPHY.md           # 📖 形态事实源。内核负责人维护
└── index.html              # 🚪 入口画廊。内核负责人维护
```

**单独的模块（各一个主人）**：内核 `shared/` · 侧栏 `sidebar/` · Onboarding `onboarding/` · **每个海洋** `oceans/<海>/`。

---

## 二、谁能改什么（主人地图）

| 模块 | 主人 | 别人可以 | 别人不可以 |
|---|---|---|---|
| `shared/*` | 内核 | import / 读 token·icon·Shell | 改任何一行（要加图标也走内核） |
| `sidebar/*` | 侧栏 AI | — | 改 sidebar 文件 |
| `onboarding/*` | Onboarding AI | — | 改 onboarding 文件 |
| `oceans/chat/*` | Chat AI | — | 改 chat 文件夹 |
| `oceans/<新海>/*` | 该海洋 AI | — | 改别的海洋文件夹 |
| `reference.html`·`PHILOSOPHY.md`·`index.html` | 内核 | 提议 | 直接改 |

铁律：**你只改自己模块文件夹里的文件**。需要别的模块配合 → 提给那个主人，别自己伸手。

---

## 三、共享契约（改这些 = 牵动所有人，必须走内核）

海洋 / 侧栏只**依赖**下面这套契约，不依赖内核内部实现：

- **`Shell`（shell.js 暴露）**
  - `Shell.left` — 侧栏槽（侧栏 `innerHTML` 填它）
  - `Shell.sea` — 海洋中区槽（海洋 `build(sea)` 填它）
  - `Shell.body` — 海洋把**自己的右岛** `appendChild` 到这（带 `data-ocean-right="<id>"`，切海洋时外壳据此清理）
  - `Shell.headExtra(html)` — 海洋往主区头**右侧**加按钮
  - `Shell.headLead` — 主区头**最左**的中性布局槽（面包屑之前），与右侧 `#head-extra` 物理隔离。侧栏把「展开侧栏」按钮 `append` 进这（收起后岛全隐、按钮需岛外有家）；**海洋勿碰**。
  - `Shell.sideWidth`（get，**optional**）— 只读当前侧栏像素宽（收起返回 0）。仅给内部用 canvas/绝对定位、无法靠 CSS flex 自动回流的海洋兜底重测量；**纯 flex/DOM 海洋勿用**（误读布局态反而泄漏）。
  - `Shell.crumb(text)` — 设面包屑
  - `Shell.registerOcean(id, { crumb?, build(sea) })` + `Shell.mount(id)`
- **`icon(name, size?, stroke?)`（icons.js）** — APPEND-ONLY：只加 key，**永不改名/删**已有（别人在用）。
- **CSS token**：颜色/度量一律走 `tokens.css` + `shell.css` 的 `--cc-*`、`--ink*`、`--accent`、`--ease-spring`。**禁内联硬编码颜色**。
- **侧栏宽度/收起（shell ↔ sidebar 契约）**：
  - CSS 变量 `--side-w`（挂 `<html>`，默认 240px）= 侧栏宽，**全仓库只在 shell.css 定义一次**；sidebar 拖拽时写 `<html>` 行内值，sidebar.css **不得再声明 `.side` 的 width**（杜绝「槽 vs 岛谁定宽」打架）。
  - data 属性 `html[data-side]`(`on`|`off`) + `html[data-side-dragging]` — sidebar 写、shell.css 据此回流/关过渡。收起规约由 shell.css 用 `html[data-side="off"] .side{…}`（属性选择器稳压 `.side`，不用 `!important`），**只碰布局量**（width/opacity/margin/overflow/pointer-events），不碰岛皮肤（那是 sidebar 的）。
  - **给海洋的一句话**：你是 `.body` 里 `flex:1` 的 `.main`，侧栏收起/调宽只会让你自动伸缩、flex 平滑回流，**什么都不用做**；布局状态不会也不该泄漏给你（除非内部 canvas 无法 CSS 回流，才读 optional 的 `Shell.sideWidth`）。
  - 备忘：48px 图标 rail 收起态当前**不做**（窄岛会读成竖直 divider、违「禁分割线」）；**留待侧栏长出 Forge 四实体竖导航时再评估**。

> 契约要变（加槽、改签名）= 提给内核，内核改 `shared/` + 本表 + 同步通知，**绝不各自在海洋里硬塞**。

---

## 四、加一个新海洋（零冲突套路）

1. 新建文件夹 `oceans/<id>/`（如 `oceans/scheduler/`）。
2. 拷 `oceans/chat/chat.html` 作 harness，把 `chat.*` 引用换成你的文件名。
3. 写你的块：`<id>.css` + `<id>.js`（`Shell.registerOcean('<id>', { build(sea){…} })`）+ 需要右岛就加 `right-*.js`（`append` 到 `Shell.body`，带 `data-ocean-right="<id>"`）。
4. 缺图标 → **提给内核**往 `icons.js` append；别在海洋里内联 SVG。
5. 完事在 `index.html` 加一行入口（这是唯一需要内核配合的共享触点，内核串行合）。

**全程你只新增自己文件夹的文件** → 和别的海洋/侧栏/onboarding 物理零交叠。

---

## 五、设计纪律（沿用 PHILOSOPHY.md，落地必须守）

- **海与岛同色、靠海岸线分**（1px 描边 + 圆角 + 窄缝 + 微阴影，**不靠颜色差**）；用户气泡灰药丸是唯一例外。
- **禁横向分割线**（留白 + 分组代替）。
- **少强调色**：`--accent` 只点「正在发生」；其余中性灰；语义色仅用于真实状态。
- **动效走 `--ease-spring`**，与全站一致。
- **视觉参考 Claude Code**（结构/密度），**颜色用干净白/近黑、字体用系统字**（非米白/衬线）。

---

## 六、预览 & 提交

- 预览：`python3 -m http.server 4178 --directory design-lab` → 开 `oceans/<海>/<海>.html`。
- 提交：**只 `git add` 自己模块的文件**（main-only、不开分支、不 `git add -A`）；commit 后推 origin/main。
- design-lab **不连后端、不进文档门禁、不进 Flutter 工程**——它是「形态事实源」，定稿后才在 `frontend/` 落 Flutter。
