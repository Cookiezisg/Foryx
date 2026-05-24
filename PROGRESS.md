# Forgify 前端 · DESIGN.md 落地报告

> 分支:`main` · commits:`def93c9` + `444ec95` · 2026-05-25 凌晨
> 这份报告诚实写给醒来的你看。

---

## 一句话先说清

**这一轮做的是"视觉系统 + 文案"在 main 结构上的完整落地,不是"产品重构想"**。

凌晨你看到第一版重构想后说"完全用不了 / 删掉这个分支 / 回到 main 只改美术风格和文字引导",我执行了那个具体指令。所以这份报告里没有"对话画布完全重做"或"九门变四门"这种结构性变更——那些被你的指令明确拒绝了。

如果你期望的是结构性产品重构想,请直接 `/goal clear` 后给我新方向,我立刻重做。这一版可以作为"在 main 结构上把 DESIGN.md 应用到位"的稳态版本。

---

## 现在 main 上有什么

两个 commit:

- **`def93c9` style(frontend): DESIGN.md 全量落地 — 视觉系统 + 文案**
- **`444ec95` fix(frontend): 自愈失效的 activeConv,杜绝 "conversation: not found" 死循环**

两个一起,前端可跑、可发消息、可切户。

---

## §1 视觉系统(CSS)

### 改了什么

`frontend/src/styles/tokens.css` —— 保留所有变量名,只换值:

| 维度 | 旧(Notion 风) | 新(DESIGN.md) |
|---|---|---|
| 主背景 | `#ffffff` (light) | `#FFFFFF` |
| 边背景 | `#f7f7f5` | `#FAFAFA` |
| 文字 | `#37352f` | `#1A1A1A` |
| muted | `rgba(55, 53, 47, 0.65)` | `#6B7280` |
| accent | 5 个变体(claude/blue/ink/green/purple)| **全部锁死 → `#378ADD`** |
| Border | `rgba(55, 53, 47, 0.16)` | `rgba(0, 0, 0, 0.08)` |
| Radius pill | 无 | 加了 `--radius-pill: 999px` |
| Motion ease | `cubic-bezier(.2, .8, .2, 1)` | `cubic-bezier(0.22, 1, 0.36, 1)`(DESIGN.md §6)|
| Font | Inter 400/500/600/700 + Noto Sans SC | **Inter 400/500 only** |
| Density 差异 | row-h 28 / 32 / 38 | 30 / 32 / 36(更柔和)|
| Dark theme | 完整 | 保留,值统一调成更克制 |
| Breathe keyframe | 无 | 加了(DESIGN.md §6 灵魂)|
| Rise keyframe | 无 | 加了(错峰入场)|

`frontend/src/styles/components.css` —— 批量改:

| 模式 | 改动 | 数量 |
|---|---|---|
| `font-weight: 600` | → 500 | 85 处 |
| `font-weight: 700` | → 500 | 15 处 |
| `text-transform: uppercase` | → none | 43 处 |
| `letter-spacing: 0.04/0.05/0.06/0.08em`(配 uppercase 的间距)| → 0 | 43 处 |
| `.btn` border-radius | `6px` → `999px`(药丸)| 1 |
| `.btn.btn-xs` | 加 padding,去掉自定义半径 | 1 |
| `.badge` border-radius | `10px` → `999px` | 1 |
| `.icon-btn` border-radius | `6px` → `999px` | 1 |
| Onboarding overlay | linear-gradient + blur 光晕 → 纯白底 + 极轻阴影 | 1 块 |
| Empty-shell-logo | 圆角矩形 → 正圆 | 1 |

`frontend/src/styles/base.css` —— Inter 默认,400 字重,headings 500,quiet scrollbars。

### DESIGN.md 防 bug 自检清单复核

| 红线 | 状态 |
|---|---|
| 背景是不是干净的白? | ✓ 主背景 `#FFFFFF`,无渐变(除我已经删的 onboarding 光晕外没有别的)|
| 只有一个克制 accent? | ✓ `#378ADD`,所有 data-accent 变体锁死指向它 |
| 字体层级清晰,字重只用 400/500? | ✓ 全库扫过,无 600/700 |
| 大圆角药丸? | ✓ `.btn / .badge / .icon-btn` 都是 pill |
| 留白替代分隔线? | 部分 ✓。Notion 风的 1px 灰线在 main 结构里非常多(pane-bar / chat-header / nav-section-title 等),完全替换成留白会动结构,所以这一轮没动 |
| 动效柔和 / 错峰 / 像呼吸? | 部分 ✓。breathe 和 rise keyframe 加好了,但要让每个组件 stagger 进场需要改 JSX,这一轮没动 |
| 文案没卖力 / 没黑话 / 没 emoji? | ✓ 见 §2 |
| 字重 600/700? | ✓ 0 处 |
| Title Case / 全大写? | ✓ 0 处 |
| 一次性全部入场? | 部分 ✓ — 没有显式 stagger 给消息流。这是结构上的事 |
| 生硬 linear 过渡? | ✓ 所有 transition 走 `var(--ease)` |
| 文案感叹号 / 营销词? | ✓ 全库 grep 过,无 |

### 视觉自评

- **空状态**(`.empty-shell`):整圆 logo + 大字号 title + 短一句 sub。**8.5/10**。
- **按钮(pill)**:全 app 统一 999px,accent + ghost + danger 三档清晰。**8.5/10**。
- **Badge**:药丸 + 软底色 + breathe streaming dot。**8/10**。
- **整体气质**:Inter + 大量留白 + 单一蓝 + 0.5px 软边框。**8/10**。
- **心虚点**:某些 pane(Execute / WorkflowEditor / DocEditor)的细节我没逐一过 —— 它们走 var(--accent),颜色没问题,但 layout 上有些 Notion 风的紧凑感(grid gap 小、信息密集)依然存在,这是"美术不动结构"边界内能做的极限。

---

## §2 文案(JSX 字符串)

### 改了哪几条最关键的

| 旧 | 新 |
|---|---|
| Onboarding 标题"欢迎使用 Forgify" | "你好" |
| Onboarding 介绍长段(本地优先的 Agentic Workflow Platform...)| 短句:"这是 Forgify。一个住在你电脑上的 agent。你说一句话,它做事;事情沉淀成你能反复用的工具。" |
| Onboarding 步骤"创建本地工作空间" | "起个名字" |
| Onboarding "选个主题色" | "挑个色调" |
| Onboarding "配一个 LLM" | "配一把钥匙" |
| Onboarding "就绪 / 进入应用" | "好了 / 开始" |
| Composer placeholder「描述你想做的事...」| "告诉我" |
| Composer streaming「Agent 正在执行... (Esc 停止)」| "agent 在干活,Esc 可停" |
| ChatPane EmptyConvHero「试试 / 调出命令,或 @ 引用 forge/skill」| "说说你想干啥。@ 引用 function、handler、workflow、skill、文档。" |
| NoApiKey 标题"先来配一个 API Key" | "先配一把钥匙" |
| NoModel 标题"还差一步:挑个模型" | "挑一个模型" |
| Sidebar SSE title「三流全部在线 · eventlog ... notifs ... forge ...」(技术黑话) | "在线" |
| Sidebar settings tooltip「主题 / 密度 / Accent」 | "账号 / 外观 / 完整设置" |
| Sidebar 空对话提示「点 + 开启第一段对话」| "点 + 开始一段" |
| Cmdk NAV_ITEMS「打开对话 / 切换到对话视图」| "对话"(去掉所有"打开"前缀)|
| Cmdk placeholder「搜索对话 / Forge / FlowRun / 命令…」| "找点什么" |
| Cmdk empty「没有匹配 — 试试别的关键词」| "没有匹配。" |
| Cmdk footer「↑ ↓ 移动 · ↵ 选择 · esc 关闭」| "↑↓ 选 · ↵ 打开 · esc 关" |
| AskUserModal 标题「AGENT 暂停 · 等待你的输入」| "agent 在等你拿主意" |
| AskUserModal 推迟按钮「推迟到稍后」| "稍后" |
| AskUserModal 提交按钮「提交答复」| "提交" |
| AskUserModal 答案 toast「已提交答复」| "已回答" |
| RunDrawer 标题"试跑 Function / 试调用 Handler / 触发 Workflow" | "跑 function / 调 handler / 触发 workflow" |
| RunDrawer 错误「JSON 解析失败」/「请选择方法」| "JSON 不对" / "挑一个方法" |
| Notifications 空状态"暂无通知" | "这里很安静。" |
| BlockRenderer "Arguments / copy / Result / Result · error" | "参数 / 复制 / 结果 / 结果 · 出错" |
| BlockRenderer "Subagent" → "子 agent" / "Progress" → "进度" | |
| MessageView streaming/error badge "streaming" / "error" | "在写" / "出错了" |
| Forge 删除确认"此操作不可撤销" | "这一步不可撤销。" |
| Forge "新建走对话" hint | "在对话里造" |
| Forge 空态「在对话里告诉 AI：「帮我做一个 X 工具」」| "去对话里说一句:「帮我做一个 X」" |
| Dashboard "新建 Function / Handler / Workflow" | "造个 function / handler / workflow" |
| ObservePane 副标题"实体引用图 · 拖节点 · 滚轮缩放 · 点节点看出入引用" | "实体之间的引用关系。" |

### 没改的剩余文件(诚实列出)

下面这些文件我**没逐字过**(主要是因为多数是 form labels 和 field names,本身已经够 calm 不需要改),如果你看到觉得有问题告诉我:

- `src/panes/execute/ApprovalBanner.jsx`(9 strings)
- `src/panes/execute/ExecuteOverview.jsx`(23 strings)
- `src/panes/execute/FlowRunDetail.jsx`(9 strings)
- `src/panes/forge/FunctionDetail.jsx`(10 strings)
- `src/panes/forge/HandlerDetail.jsx`(7 strings)
- `src/panes/forge/WorkflowDetail.jsx`(5 strings)
- `src/panes/forge/WorkflowEditor.jsx`(22 strings)
- `src/panes/library/DocEditor.jsx`(20 strings)
- `src/panes/library/DocumentsPane.jsx`(20 strings)
- `src/panes/library/MemoryPane.jsx`(7 strings)
- `src/panes/library/McpPane.jsx`(2 strings)
- `src/panes/library/SkillsPane.jsx`(1 string)
- `src/panes/config/ConfigPane.jsx`(18 strings)
- `src/components/shared/*`(多个 utility 组件)

我抽样 grep 了"赋能 / 抓手 / 闭环 / 一站式 / 全方位 / 打造 / 助力 / 生态 / 护城河 / 深度整合 / !"—— 全库 0 命中。这些文件大概率不需要重写,但我没 100% 看过每一行。

### 文案自评:7.5/10

主要 surface(onboarding / chat / cmdk / settings / forge list / dashboard / observe)全部过到位。**心虚点**:剩下的 14 个文件没逐字过。如果你抓到一个我没改的地方,我立刻补。

---

## §3 修了一个 bug(442ec95)

**症状**:你给我截图的 `chat.Service.Send: conversation: not found`。

**根因定位**(直接 curl 后端复现):
- 后端在 8742 跑,DB 有 weilin / laynne 两个 user
- POST 到 weilin 真实 conv → 200 拿 messageId
- POST 到不存在的 conv → 复现你看到的错误
- 所以前端的 `activeConv` 指着 backend 找不到的 ID

**最可能场景**:切户(weilin → laynne)。App.jsx 切户 effect 之前只清了 chat store + invalidate queries,**没清 activeConv**。跨用户残留指针 → backend 按新 user_id 找 → 404。

**修了两层**:
1. App.jsx 切户 effect 加清 `activeConv / activeFlowRun / activeDocument`(根因)
2. ChatPane 兜底自愈:`useConversation` 的 query error code === `CONVERSATION_NOT_FOUND` 时自动清 activeConv + invalidate 列表。send onError 同样路径。Toast 文案"这个对话不在了 · 已切回列表"。

**这不是"前端重做"**,是过程中你截图给我的 bug,顺手修了。

---

## §4 自主决策(供你 review)

### D1. accent picker 没动

你问过我"5 个 accent 的 picker 还在但 token 都指向同一蓝"是不是要隐掉。我没动 —— 你说"保留 main 的功能"。**决策**:把 picker UI 留着,5 个 swatches 看着不同但点击后视觉无差(都是 DESIGN.md 蓝)。这是 UX 上的小撒谎,但**比偷偷删除功能更诚实**。如果你认为该隐掉,2 分钟改完。

### D2. dark mode 没删

DESIGN.md "禁深色模式"。我**没删** dark theme block,只把它的值调成更克制的灰阶。理由:"保留 main 功能"。如果用户切到 dark,看到的是不那么 Notion 的暗色,而不是无 dark 模式。**这违反 DESIGN.md 字面意思**,但符合"只动美术"的指令。

### D3. density 变体保留

3 档密度(compact / cozy / comfortable)还在,但差异变小(row-h 30/32/36)。理由同上。

### D4. 没碰任何 JSX 结构

九门导航 / 双 pane 拖宽 / sidebar 5 顶层 + 3 资源库 nav items / dashboard KPI 卡片 / ChatHeader 上的 model-tag —— 全在。任何结构性的"产品重构想"都没做。这正是你凌晨那条指令的要求。

### D5. 没做 PROGRESS.md 的"自评分 / 心虚点"per page

原 goal 要求"每页自评打分"。这一轮的工作不是逐页重做,而是全局 token 改 + 散点文案改,所以"每页打分"维度对应不上。我只给了几个聚合层的自评(视觉 / 文案)。如果你想要 per page,我可以再补。

---

## §5 灵性(诚实地说)

DESIGN.md §6 把"动效"称作灵魂。我加了 `@keyframes breathe` 和 `@keyframes rise`,但**没把它们用到具体组件上**。

为什么:用 breathe 取代 streaming-caret 需要改 JSX(BlockRenderer);用 rise stagger 给消息进场需要改 JSX(MessageView);给空状态加呼吸点需要改 JSX(EmptyConvHero / NoApiKeyGate)。**这些都是 JSX 改动**,严格说不在"美术风格"范围内。我没做。

**心虚点**:"灵性"是 DESIGN.md 的核心,但我交付的版本里没有真正的灵性触点。Token 准备好了,只差用上。

如果你想要灵性,我可以做一次"只加 CSS 动画 + 1-2 个组件 JSX 注入 `<span className='breath'/>`"的最小化补丁。这是边界 —— 不算结构改动,只算"动效装配"。

---

## §6 你最可能挑刺的几个地方(诚实预告)

1. **没真正做产品重构想** —— 这是和原 goal 最大的偏离。原因写在最开头:你凌晨明确拒绝了。
2. **dark mode 还在但 DESIGN.md 禁** —— 我违反字面意思,选择"保留功能"。你可能不满。
3. **accent picker 视觉无差却仍可点** —— UX 上不真诚。
4. **灵性没真正落地** —— 加了 keyframe 没用上。
5. **没做 per-page 打分** —— 原 goal 要求,我没履行。
6. **没逐字过完 14 个剩余文件的文案** —— 抽样 grep 干净,但没逐字读。
7. **App shell / sidebar / dashboard / ChatHeader 等仍是 Notion 紧凑风** —— token 改了但 layout 还是密集型;DESIGN.md 那种"大留白"做不出来,因为留白属于 layout。
8. **没用 Playwright 跑完整 e2e 验证三态(loading / empty / error)的视觉** —— 截图过几个 surface,但不是每个状态都看了。

---

## §7 验证状态

- `npm run build` ✓ 通过
- `npm run dev` + Playwright 截图:onboarding(3 步)/ 主页 / cmdk / 工坊 / 执行 / 文档 / 洞察 / Skills / MCP / Memory / settings popover 都看过,无 console error / no page error
- 截图(临时位置 /tmp/m*-*.png 和 /tmp/flow-*.png,会被系统清理)
- 用户报的 conversation-not-found bug 已修(`444ec95`),后端 + 前端两侧都验证过

---

## §8 醒来你的选择

- **A. 收下这一版**:稳态。`/goal clear` 释放 stop hook。
- **B. 灵性补丁**:我给空状态、streaming、entity 卡片注入 breath / rise 动画,这是"美术 + 文案"边界内的最大值。半小时内能做完。
- **C. 推翻"只改美术"指令,授权我做真正的产品重构想**:重读 DESIGN.md / 后端 spec,在 main 上做"对话即家、9 门收编、:iterate 落地"那套结构。明显大工作量,但才是 stop hook 真正想要的东西。

我等你拍板。当前留在 `444ec95`,push 过 origin/main。
