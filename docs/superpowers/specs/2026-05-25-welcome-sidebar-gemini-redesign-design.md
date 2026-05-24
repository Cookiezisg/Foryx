# 欢迎页 + 侧边栏 Gemini-style 重做 — 设计文档

> 把 Google/Gemini 的克制视觉语言固化为 Forgify 的官方风格。本文档只覆盖 **欢迎页(Dashboard) + 侧边栏(Sidebar) + footer**;pane 内部不动。

## 1. 动机

- 现侧边栏图标视觉重量不齐、对齐松散;"工具类" 项(Skills/MCP/Memory/洞察)平铺占行,层级感弱
- 现 Dashboard 重 KPI 卡 + 双 section,违背 DESIGN.md "大留白、克制" 心智
- footer 的 "本地 / SSE" 文案对用户无意义
- 没有视觉锚:第一眼分不出 Forgify 和任意 SaaS 产品

目标:让欢迎页 + 侧边栏成为 Forgify 的 visual signature。

## 2. 范围

**In:** `Sidebar.jsx`、`Dashboard.jsx`、相关 styles、新建 `greetings.js`、footer 模式、用户名来源。

**Out:** 各 pane 内部内容(ChatPane、ForgePane、ObservePane 等);Phase 5 智能化;narrow 模式专属调整(沿用现状)。

## 3. 核心决策

### 3.1 欢迎页(Dashboard)

| 区域 | 决策 |
|---|---|
| 整体布局 | Gemini-empty:单列居中,大问候 + pill 输入框 + 可选智能上下文条 |
| KPI 卡 | **删** |
| "继续对话" / "开始新的" section | **删**(最近对话挪入侧边栏 "最近" 段) |
| 输入框 placeholder | `Ask Forgify… or forge something` |
| 输入框 Enter 行为 | (1) `POST /conversations` 拿 id → (2) `POST /conversations/{id}/messages` 把输入当首条 user message → (3) `setActiveConv(id)` → (4) `openPane("chat")` 顺序串行,一气呵成 |

### 3.2 智能上下文条

**优先级:** 取最重要那一条;都没就 **整条隐藏**(绝不出现 "0 · 0 · 0")。

| 优先级 | 触发 | 显示文案 | 点击 |
|---|---|---|---|
| P1 | 有 `waiting_approval` flowrun | "N 个流程等你确认 · <link>{flowName}</link>" | open execute pane,focus that flowrun |
| P2 | 有 `failed` flowrun | "N 个流程卡住了 · <link>查看</link>" | open execute pane,filter failed |
| P3 | 有 `running` flowrun | "N 个流程在跑 · 最近一次 {relTime} 前启动" | open execute pane |
| P4 | 有最近 conv(updatedAt < 24h) | "继续 · <link>{convTitle}</link> · {relTime}" | setActiveConv + open chat |

*(本表 `{flowName}` / `{convTitle}` / `{relTime}` 是渲染时填充字段,和 §3.3 greeting 里 `{name}` 是用户 displayName 是两回事。)*
| P5 | 啥都没 | (隐藏) | — |

实现:`useContextStrip()` hook,内部 useFlowRuns + useConversations,返 `{kind, payload}` 或 `null`。

### 3.3 问候语(Greetings)

- 池子大小 **360 句**,15 类别(A 硬核 / B 主题 / C 行动 / D 问句 / E 续接 / F 短令 / G 时间感 / H 高压 / I 冷幽默 / J 温柔 / K 自我引述 / L 谚语 / M 带名 / N Forgify 自称 / O 杂)
- 占位符:含名字的句子用 `{name}`(不硬编码 "Weilin"),substitution 时用 `displayName`;无 `displayName` 时只从 name-free 子集抽
- 选用逻辑(`useGreeting()` hook,顺序判断,**先匹配的赢**):
  1. 当地小时 ≥ 22 或 < 6 → 50% 从 G 的"夜班"子集(`Working late, Weilin.` 类)抽
  2. 当地小时 ≥ 6 且 < 11 → 50% 从 G 的"早班"子集(`Morning, Weilin.` 类)抽
  3. 有最近 conv(updatedAt < 24h)→ 50% 从 E(续接)抽
  4. 否则 / fallback → 从全池随机
  5. 无 displayName 时,所有抽签限定在 name-free 子集(过滤含 `{name}` 的)
- **mount 一次,不会刷字**(避免心理上"花") — `useMemo` 锁定 + 依赖数组只放 displayName / recent existence
- 数据文件:`frontend/src/panes/dashboard/greetings.js`,导出 `GREETINGS` 数组,每项 `{ text, tags: [...] }`

### 3.4 侧边栏(Sidebar)结构

```
[ logo (hover→PanelLeftClose) ]   Forgify              ← 展开 260
[ logo (hover→PanelLeftOpen)  ]                        ← 收起 64

[ ✎ 新对话 ]                                            ← primary pill,bg #F4F4F2
[ ⌕ 搜索 或 跳转 ]

[ 💬 对话 ]                                             ← workbench 块
[ 🔨 工坊 ]                                             ← 锻造 → 工坊(UI label)
[ ▶  执行 ]
[ 📄 文档 ]

── 工具(hover→ ▾,collapsible) ─────
[ 📊 洞察 ]
[ ✨ Skills ]
[ 🔌 MCP ]
[ 🧠 Memory ]

── 最近(hover→ ▾,collapsible,仅展开态显示) ─────
RAG 数据准备流程详解
Transformer 长度限制
...

[ avatar(badge corner) ]  {displayName}  hover→ ⚙
```

**几个关键约束:**

- **logo 即 toggle:** 顶部 24×24 槽位 hover 180ms morph(logo → panel-toggle),click 触发 collapse/expand。**不占额外行。**
- **平行翻译动画:** 展开 ↔ 收起时,所有 nav item 的垂直 y 坐标 **完全不变**;变的只是宽度(260 ↔ 64)和 label 出现/隐藏(opacity)。Framer Motion spring 同步驱动。
- **工具段在两态都存在:** 收起态 "工具" 段标题降级为 18px 短横线(hover 浮出 ▾);折叠状态写 localStorage `sidebar.toolsExpanded`,两态共享。
- **最近段仅展开态显示:** chat 项无图标,收起态隐藏整个 section。
- **icon 统一:** 全部 Lucide outline,18×18,stroke 2,色 `--fg-strong` (active) / `--fg-muted` (idle)。

### 3.5 icon mapping

| 项 | Lucide name |
|---|---|
| logo | (自绘 anvil+spark SVG) |
| panel-toggle close | PanelLeftClose |
| panel-toggle open | PanelLeftOpen |
| 新对话 | SquarePen |
| 搜索 | Search |
| 对话 | MessageSquare |
| 工坊 | Hammer |
| 执行 | Play |
| 文档 | FileText |
| 洞察 | BarChart3 |
| Skills | Sparkles |
| MCP | Plug |
| Memory | Brain |
| 段折叠 | ChevronDown |
| 设置 | Settings |
| 头像 fallback | (首字母 / 1 字符) |

### 3.6 footer

- **删** "本地" 文字 + "SSE" 文字标签(SSE 状态降级:头像右下角 2px dot,色由 `useSSEHealth().overall` 决定;dot 仅在 `err` / `warn` 时显示)
- **删** 独立 HelpCircle 按钮 + 独立 Bell 按钮
- **新:**
  - 头像槽 28×28,内嵌真实用户首字母(取自 `displayName`,空 fallback "?"),**头像本身右上角** 6px 红 dot 在 `unreadHelp + unreadBell > 0` 时显示
  - **点头像** → 打开统一 NotificationsDrawer(两 tab:**待办** = Help/Ask,**通知** = Bell;打开后 mark all as read)
  - **hover footer 整行** → 名字右侧 slide-in 一个 24×24 ⚙ Settings 按钮(在 180ms 内 width:0→24 + opacity:0→1)
  - **收起态 footer:** 仅头像槽居中;Settings 按钮变成头像 **正上方** 浮出的 24×24 icon(同样 hover-only)
- 名字:`displayName` 来自新增的偏好字段(见 3.8)

### 3.7 锻造 → 工坊(rename)

- **仅 UI label** 改:`Sidebar.jsx` 中 `"锻造"` → `"工坊"`;PRD §8 / DESIGN.md 等中文文档对应改
- **内部不动:** pane key 仍是 `"forge"`、entity prefix 仍是 `f_`、`forgetool` 包名 / domain 名 / store 名 / API 路径 `/forges` / SSE channel `forge` 全保留
- 影响文件: `Sidebar.jsx`、`PaneFrame.jsx` 的 `PANE_META`、`AppShell.jsx`(label 引用)、PRD §5/§6/§8 中文档对应行

### 3.8 用户名来源(displayName)

- **新偏好字段:** 后端无需改;前端用 localStorage key `forgify.user.displayName`(单用户本地)
- 默认 `""`(空)→ footer 显示头像 + 不显示文字行;greetings 走 name-free 子集
- 用户可在 SettingsPopover 一个新 input 设
- 头像首字母:`displayName?.[0]?.toUpperCase() || "?"`
- **不依赖后端用户表**(V1.2 单用户本地;backend 的 user 概念是 multi-user 预留,不展示)

### 3.9 动画规约(DESIGN.md 落地)

| 元素 | 时长 | 缓动 |
|---|---|---|
| 侧边栏宽度切换 | spring (stiffness 280, damping 28) | Framer Motion |
| label opacity fade | 180ms | `--ease` |
| logo / 头像 hover morph | 180ms | `--ease` |
| 段折叠展开 | 220ms height auto | `--ease` |
| 智能上下文条入场 | 700ms rise(从下方+8px) | `--ease`,延 200ms 错峰 |
| 问候语入场 | 700ms rise | `--ease`,延 0 (主轴) |
| 输入框入场 | 700ms rise | `--ease`,延 120ms |
| `prefers-reduced-motion` | 全部降为 1ms | — |

## 4. 涉及文件

### 新建

- `frontend/src/panes/dashboard/greetings.js` — 360 句 + tag
- `frontend/src/panes/dashboard/useGreeting.js` — 选用 hook,基于 recent conv + 时间 + name 可用性
- `frontend/src/panes/dashboard/useContextStrip.js` — 智能条优先级 hook
- `frontend/src/panes/dashboard/WelcomeInput.jsx` — pill 输入框 + Enter 触发新建对话(从 Composer 抽出 send 流程)
- `frontend/src/components/layout/SidebarSection.jsx` — 可折叠段组件(hover-▾,localStorage 持久化)
- `frontend/src/hooks/useDisplayName.js` — 读写 localStorage,React 同步

### 重写

- `frontend/src/components/layout/Sidebar.jsx` — 全重做(布局、图标、互动)
- `frontend/src/panes/dashboard/Dashboard.jsx` — 全重做(KPI 块全删,变 Gemini-empty)
- `frontend/src/styles/components.css` — `.sidebar` / `.sidebar-*` / `.nav-*` / `.dash-*` 段全改

### 微改

- `frontend/src/components/primitives/Icon.jsx` — 确认 SquarePen / BarChart3 / Plug / Brain / PanelLeftClose / PanelLeftOpen / Hammer 都从 lucide-react 正确 re-export
- `frontend/src/components/overlays/NotificationsDrawer.jsx` — 增 "待办" tab(吸收 AskUserModal 的列表;现 AskUserModal 保留为 push-style modal,但同时进 drawer 列表)
- `frontend/src/components/overlays/SettingsPopover.jsx` — 新 displayName input
- `frontend/src/store/ui.js` — 加 `toolsExpanded`、`recentExpanded`(初值 true,localStorage 同步)
- `frontend/src/App.jsx` — 移除 narrow 模式下对老 sidebar 的特殊 fallback(如有)

### 不动

- `ChatListItem.jsx`(数据形状不变,仅样式由父 CSS 控制)
- 所有 pane 内部(ChatPane / ForgePane / ExecutePane / DocumentsPane / etc.)
- 所有 API 客户端 / SSE 协议

## 5. 数据流

### 5.1 欢迎页 Enter → 新对话

```
WelcomeInput onSubmit(text)
  → useCreateConversation().mutateAsync({}) 等返回
    → useUIStore.setActiveConv(created.id)
    → useUIStore.openPane("chat")
    → 同步调 useSendMessage(created.id).mutate({ text })
      → ChatPane 已挂载,SSE 接收后续 events
```

错误处理:第一步 POST /conversations 失败 → 维持欢迎页 + toast 红色;第二步 send 失败 → 已切到 chat pane,显示 message 错误状态(沿用 Composer 已有错误流)。

### 5.2 侧边栏收起 + 工具段折叠

```
LayoutRoot mount
  → 读 localStorage 'sidebar.collapsed' / 'sidebar.toolsExpanded' / 'sidebar.recentExpanded'
  → 初始化 zustand 状态
点击 logo (hover→toggle visible)
  → setCollapsed(!collapsed)
  → Framer Motion 触发 width spring (260↔64) + label opacity
  → localStorage 写入
点击 "工具" hover-▾
  → setToolsExpanded(!toolsExpanded)
  → 子项 height animate (auto ↔ 0)
  → localStorage 写入
  → 收起态下同样响应(段标题降级成短线,hover 仍出 ▾)
```

### 5.3 头像 → notif

```
unreadCount = unreadHelp + unreadBell  (来自 useSSEHealth + Help store)
点头像
  → setNotifsOpen(true)
  → NotificationsDrawer 渲染两 tab,default 选 "待办" 若 unreadHelp > 0 否则 "通知"
  → drawer 打开同步 markAllRead → 红 dot 消失
hover footer (任一位置)
  → ⚙ 按钮 slide-in
点 ⚙
  → setSettingsPopOpen(true)
```

## 6. S14 文档同步

| 文档 | 更新点 |
|---|---|
| `documents/version-1.2/frontend-prd.md` §8 | Sidebar 结构图重画(从 5+3 改为 4+4 工具段) |
| `documents/version-1.2/frontend-prd.md` §8.2 | sidebar collapse 行为细化(logo morph) |
| `documents/version-1.2/frontend-prd.md` §16 | 加几条 "已修" 记录:icon 对齐 / footer 标签去除 / Dashboard 简化 |
| `documents/version-1.2/frontend-prd.md` §15 | Phase 6 (welcome+sidebar redesign) 项打勾顺序 |
| `documents/version-1.2/frontend-prd.md` §17 | 不动(API 没变) |
| `DESIGN.md` | 新增 §10 "问候语调性"小节:硅谷腔、英文、不喊、无 emoji、不强行带名 |
| `documents/version-1.2/progress-record.md` | 每个子任务完工日志(P0 prep / P1 sidebar / P2 dashboard / P3 polish) |
| 锻造 → 工坊 改名 | PRD 中所有 "锻造" UI 引用改 "工坊";内部代码 / API / 数据库 / contract 引用不动 |

## 7. 测试

| 文件 | 重点 |
|---|---|
| `Sidebar.test.jsx`(重写) | logo hover morph;collapse toggle;tools section hover-▾;tools state persist;footer avatar badge with unread > 0;hover footer → gear slide-in;display name fallback |
| `Dashboard.test.jsx`(重写) | greeting picked once on mount;name-free path when displayName empty;context strip P1>P2>P3>P4>hidden order;input Enter → 双 POST 顺序 |
| `greetings.test.js`(新) | 池子 360 不重复;每条都英文 ASCII;`{name}` 替换功能;过滤 name-free 子集大小 |
| `useContextStrip.test.js`(新) | 优先级映射;hidden 状态 |
| `useGreeting.test.js`(新) | 时间分布;recent conv 时 E 抽中频率 ~50% |

跑:`cd frontend && npm test`(沿用 vitest 现配置)。

## 8. 阶段拆分(给 writing-plans 参考)

- **P0** 基础设施:displayName hook、icon 确认、greetings.js 数据文件、useGreeting / useContextStrip hook(写 + 测,不接 UI)
- **P1** Sidebar 重写:layout + 折叠段 + logo morph + footer avatar/notif + ⚙ 浮出。锻造→工坊文案改。先在主页面看效果,无需 Dashboard 配套。
- **P2** Dashboard 重写:Welcome layout + WelcomeInput + smart strip;接 hook。
- **P3** S14 三件套同步:PRD / DESIGN.md / progress-record;打勾 phase。

## 9. 非目标 / 推迟

- ObservePane 实质实现(Phase 5 工作)
- MemoryPane 完整 UI(78 行骨架的修复,留独立 spec)
- 后端 user / displayName 表(单用户本地用 localStorage 已够;multi-user 时再说)
- 主题切换 / accent 切换(DESIGN.md 锁死单一蓝)
- 侧边栏宽度拖拽(boilerplate 没有,YAGNI)

## 10. 风险

1. **Lucide icon 视觉不齐:** 需在实现时逐 icon 像素级目检;若某个(如 Hammer)视觉重量明显大于其他,考虑改 stroke 1.75 或换替代 icon
2. **localStorage 持久化跨 reload 漂移:** 加 schema versioning(`sidebar.v2.collapsed`)避免老版本残留状态污染
3. **NotificationsDrawer 吸收 Help/Ask 后,原 AskUserModal 弹窗的可见性下降:** 保留 push-style modal,drawer 是入口;modal 出现时同步加 drawer 列表
4. **greeting `{name}` 替换边角:** `displayName` 含空格或 emoji 时显示丑陋;实现里 trim + 长度 ≤ 24 截断
