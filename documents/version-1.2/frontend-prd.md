# Forgify 前端 PRD — V1.0

> **双轨参考。** 本文档定义架构、数据流、动效规格、与 boilerplate 的差异；`boilerplate/` 目录定义视觉细节（CSS、class 名、布局像素）。两者缺一不可——PRD 说"做什么"，boilerplate 说"长什么样"。
> 实现顺序严格按 §15 的 Phase 列表，每个 Phase 完成后打勾。

---

## §0 文档使用说明

- **先读后写。** 每个 Phase 开始前重读对应章节，不靠记忆。
- **改动即同步。** 设计变更先改本文档，再改代码。
- **§15 是进度表。** 不要跳 Phase，不要并行多个 Phase。
- **视觉细节以 boilerplate 为准。** PRD 没有逐一描述 CSS rule；凡是 PRD 没有明确说"改"的地方，一律参照 boilerplate 的样子还原，不凭空发明。详见 §18。

---

## §1 技术栈（已锁定）

```
运行时：    Wails v2（macOS .app / Windows .exe）
语言：      Go 1.25（后端）+ React 18 JSX（前端）
构建：      Vite 6（前端）+ wails build（打包）
数据层：    TanStack Query v5（REST 缓存）+ Zustand v5（UI 状态）
动效：      Framer Motion v12（进出/layout/spring）+ 现有 CSS 变量（微交互）
SSE：       3 个自定义 hook（不用第三方库）
后端桥接：  1 个 Wails binding：GetBackendPort() → string
样式：      迁移 boilerplate styles.css（保留所有 CSS 变量，不引入 Tailwind）
图标：      Lucide React（对应 boilerplate 的 Icon 对象）
```

**Wails 版本：v2（不用 v3，v3 还在 alpha）**

**不引入：** TypeScript、Tailwind、Redux、React Router（无 URL 路由需要）

---

## §2 目录结构

```
frontend/
├── index.html
├── vite.config.js
├── package.json
├── src/
│   ├── main.jsx                  ← 挂载 App + QueryClient + 初始化 baseUrl
│   ├── App.jsx                   ← 根组件（shell + overlay 组装）
│   │
│   ├── bridge/
│   │   └── wails.js              ← GetBackendPort() wrapper + baseUrl 全局
│   │
│   ├── api/
│   │   ├── client.js             ← fetch wrapper（注入 baseUrl + headers）
│   │   ├── conversations.js      ← conversation 相关 query/mutation hooks
│   │   ├── forge.js              ← function/handler/workflow query/mutation hooks
│   │   ├── flowruns.js           ← flowrun query hooks
│   │   ├── config.js             ← apikey / model-configs query hooks
│   │   ├── library.js            ← skill / mcp / memory / document hooks
│   │   └── notifications.js      ← notifications REST snapshot hook
│   │
│   ├── sse/
│   │   ├── useEventLog.js        ← eventlog SSE hook（主流 chat 内容）
│   │   ├── useNotifications.js   ← notifications SSE hook（entity 状态变更）
│   │   └── useForge.js           ← forge SSE hook（trinity 锻造进度）
│   │
│   ├── store/
│   │   ├── ui.js                 ← Zustand：panes/activeConv/selection/baseUrl
│   │   ├── settings.js           ← Zustand：theme/accent/density/lang（持久化 localStorage）
│   │   └── chat.js               ← Zustand：SSE 构建的实时 message/block 树
│   │
│   ├── components/
│   │   ├── primitives/           ← 原子组件（Button/Badge/Spinner/Icon/Kbd 等）
│   │   ├── layout/               ← Sidebar / Pane / PaneResize / AppShell
│   │   ├── overlays/             ← CommandPalette / NotificationsDrawer / AskUserModal / Toast
│   │   └── shared/               ← 跨 pane 共用（EntityLink / RelTime / KindChip / StatusBadge / ActionMenu / VersionRail 等）
│   │
│   ├── panes/
│   │   ├── chat/
│   │   │   ├── ChatPane.jsx      ← 整个 chat pane
│   │   │   ├── ChatHeader.jsx
│   │   │   ├── MessageView.jsx
│   │   │   ├── BlockRenderer.jsx ← 7 种 block 递归渲染
│   │   │   └── Composer.jsx
│   │   ├── forge/
│   │   │   ├── ForgePane.jsx     ← list ↔ detail 路由
│   │   │   ├── ForgeList.jsx
│   │   │   ├── FunctionDetail.jsx
│   │   │   ├── HandlerDetail.jsx
│   │   │   └── WorkflowDetail.jsx（含 canvas + diff）
│   │   ├── execute/
│   │   │   ├── ExecutePane.jsx
│   │   │   ├── FlowRunList.jsx
│   │   │   └── FlowRunDetail.jsx（含 DAG + Gantt）
│   │   ├── config/
│   │   │   └── ConfigPane.jsx
│   │   ├── library/
│   │   │   ├── SkillsPane.jsx
│   │   │   ├── McpPane.jsx
│   │   │   ├── MemoryPane.jsx
│   │   │   └── DocumentsPane.jsx
│   │   └── dashboard/
│   │       └── Dashboard.jsx
│   │
│   └── styles/
│       ├── tokens.css            ← 从 boilerplate styles.css 提取的变量（不改内容）
│       ├── base.css              ← reset + body + 滚动条
│       ├── components.css        ← 共用组件样式（btn/badge/table/card 等）
│       └── panes.css             ← 每个 pane 的布局样式

cmd/desktop/
├── main.go                       ← Wails 入口（启 HTTP server + 开窗口）
└── app.go                        ← App struct（GetBackendPort 方法）
```

---

## §3 设计系统

### 3.1 CSS 变量体系（直接从 boilerplate 迁移，一字不改）

关键变量组：
- `--t-fast: 120ms cubic-bezier(.2,.8,.2,1)` / `--t-med: 220ms` / `--t-slow: 360ms` — 统一曲线
- `--bg-window` / `--bg-sidebar` / `--bg-paper` / `--bg-elev` / `--bg-elev-2` — 层次背景
- `--fg-strong` / `--fg-body` / `--fg-muted` / `--fg-faint` — 4 级文字
- `--border` / `--border-strong` / `--border-soft` — 3 级边框
- `--accent` / `--accent-soft` / `--accent-fg` / `--accent-ring` — 单一 accent（默认 claude-orange #d97757）
- `--status-streaming` / `--status-success` / `--status-error` / `--status-warn` — 状态色

主题切换：`document.documentElement.dataset.theme = "light" | "dark"`
密度切换：`document.documentElement.dataset.density = "compact" | "cozy" | "comfortable"`
Accent 切换：`document.documentElement.dataset.accent = "claude" | "blue" | "ink" | "green" | "purple"`

**system theme** 监听 `window.matchMedia("(prefers-color-scheme: dark)")` 自动跟随。

### 3.2 动效分工

| 动效类型 | 使用什么 | 示例 |
|---|---|---|
| hover/focus 微交互 | CSS transition（`--t-fast`）| 按钮 hover、输入框 focus |
| pane 进场/退场 | Framer Motion `AnimatePresence` + `motion.div` | 打开/关闭 pane |
| 列表项进入 | Framer Motion `layout` + `initial/animate/exit` | conversation 新增、toast |
| overlay 进入 | Framer Motion（slide up + fade）| 命令板、通知抽屉 |
| sidebar collapse | Framer Motion spring（`stiffness: 280, damping: 28`）| 侧边栏折叠 |
| streaming 打字光标 | CSS `@keyframes blink` | text block 末尾 |
| spinner / loading dot | CSS `@keyframes`（已有）| 工具调用中 |

**Framer Motion 基础参数（统一在 `tokens.js` 导出）：**
```js
export const spring = { type: "spring", stiffness: 280, damping: 28 }
export const easeOut = { duration: 0.22, ease: [0.2, 0.8, 0.2, 1] }
export const fadeIn = { initial: { opacity: 0 }, animate: { opacity: 1 }, exit: { opacity: 0 }, transition: easeOut }
export const slideUp = { initial: { opacity: 0, y: 8 }, animate: { opacity: 1, y: 0 }, exit: { opacity: 0, y: 4 }, transition: easeOut }
```

### 3.3 原子组件规格

**Button：** 变体 `default / accent / ghost / danger / xs/sm/md`，内部 spinner 替换图标（loading state）。

**Badge：** 变体 `success / error / warn / info / muted / streaming`。streaming badge 有 pulse-dot 动画。

**Spinner：** CSS `@keyframes spin`，12px/16px 两档。

**Icon：** Lucide React 直接用，在 `components/primitives/Icon.jsx` 统一 re-export，保留 boilerplate 的命名映射（`Icon.Hammer`、`Icon.Sparkles` 等）。

**EntityLink：** 文本中实体 ID 的可点击 chip。点击后调用 shell 的 `openEntity(pane, id)` 打开对应 pane 并 focus 该实体。前缀映射：
```
fn_ → forge（FunctionDetail）
hd_ → forge（HandlerDetail）
wf_ → forge（WorkflowDetail）
cv_ → chat（切换 conversation）
fr_ → execute（FlowRunDetail）
sk_ → skills
mcp_ → mcp
mem_ → memory
```

**RelTime：** `{ts}` → 中文相对时间（刚刚/N 分钟前/N 小时前/N 天前/月日），`title` 属性显示完整时间。每 30 秒重新渲染一次（`setInterval`）。

**KindChip：** `function | handler | workflow | skill | mcp` → 有色小 chip。CSS class `.kind-chip.fn/.hd/.wf/.sk/.mcp`。

**StatusBadge：** `ready / pending / draft / failed` → 对应 badge 颜色。`pending` 和 `draft` 附带 AI sparkle mark。

**ActionMenu：** 下拉菜单（portal 到 body），items 支持 divider / danger / shortcut。点击外部关闭。用 `useFloating`（floating-ui/react）定位，防出界。

**VersionRail：** 右侧版本历史栏（Function / Handler / Workflow 共用）。可折叠。pending 版本有高亮 banner + Accept/Revert 按钮。

---

## §4 Wails 启动与 baseUrl

### 4.1 Go 侧（`cmd/desktop/`）

```go
// app.go
type App struct{ port int }

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
    // 在随机端口启动 HTTP server（或固定 7788）
    // 把 port 存到 a.port
}

func (a *App) GetBackendPort() string {
    return strconv.Itoa(a.port)
}
```

### 4.2 前端侧（`bridge/wails.js`）

```js
// bridge/wails.js
import { GetBackendPort } from "../../wailsjs/go/main/App"

let _baseUrl = null

export async function initBaseUrl() {
  const port = await GetBackendPort()
  _baseUrl = `http://localhost:${port}`
}

export function getBaseUrl() {
  if (!_baseUrl) throw new Error("baseUrl not initialized")
  return _baseUrl
}
```

### 4.3 启动流程

```jsx
// main.jsx
import { initBaseUrl } from "./bridge/wails"
import { useUIStore } from "./store/ui"

async function bootstrap() {
  await initBaseUrl()
  useUIStore.getState().setBaseUrl(getBaseUrl())
  ReactDOM.createRoot(document.getElementById("root")).render(
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  )
}

bootstrap()
```

**开发期：** Vite proxy 把 `/api` 转发到 `localhost:7788`，不需要 GetBackendPort（直接用相对路径）。`vite.config.js` 用 `server.proxy` 配置。

---

## §5 状态架构

### 5.1 Zustand — `store/ui.js`

```js
{
  baseUrl: null,           // string | null，bootstrap 后设置
  openPanes: ["chat"],     // string[]，最多 2 个
  activeConv: null,        // string | null，当前对话 ID
  leftPct: 50,             // number，双 pane 左侧宽度百分比
  collapsed: false,        // boolean，sidebar 折叠
  narrow: false,           // boolean，主区域 < 1000px 时自动折叠为单 pane
  activeNarrowPane: null,  // narrow 模式下当前可见的 pane
  focusEntity: {},         // { [pane]: entityId }，各 pane 待 focus 的实体
  // actions
  togglePane, closePane, openPane, openEntity, setActiveConv, setBaseUrl
}
```

**规则：**
- `openPanes.length` 最多 2
- 超出时踢掉 index 0（最早打开的）
- narrow 模式下两个 pane 只显示 `activeNarrowPane` 指向的那个

### 5.2 Zustand — `store/settings.js`

```js
{
  theme: "system",         // "system" | "light" | "dark"
  accent: "claude",        // "claude" | "blue" | "ink" | "green" | "purple"
  density: "cozy",         // "compact" | "cozy" | "comfortable"
  lang: "zh",              // "zh" | "en"
  reasoningDefault: "collapsed",  // reasoning block 默认折叠
  // 从 localStorage 读写（zustand persist）
}
```

settings 变化时立即写 `document.documentElement.dataset.*` 并同步 localStorage。

### 5.3 Zustand — `store/chat.js`

SSE eventlog 事件推进来，chat store 负责组装 message 树。

```js
{
  // Map<conversationId, ConvState>
  convStates: {},
  // actions（由 useEventLog hook 调用）
  onMessageStart, onMessageStop,
  onBlockStart, onBlockDelta, onBlockStop,
}

// ConvState
{
  messages: Map<msgId, Message>,
  blocks: Map<blockId, Block>,
  seenSeq: number,          // 最后处理的 seq
}
```

**Block 树规则（对应后端事件协议）：**
- `block_start.parentId` 是父 block 或 message ID
- 顶层 block：parentId = messageId
- 嵌套 block（subagent 的子 block）：parentId = 外层 block 的 ID
- `block_delta`：append 到对应 block 的 `content`
- `block_stop`：设置 status + durationMs

**TanStack Query 负责什么：** conversations 列表、历史 messages（REST 拉取）、forge 列表、flowruns、config、skill/mcp/memory 等所有非 streaming 数据。

**chat store 负责什么：** 实时 streaming 的 message/block 树，SSE 事件驱动的增量更新。

**组合：** `ChatPane` 把 REST 的历史消息（TanStack）和 SSE 构建的实时消息（chat store）合并渲染，按 seq 排序。

### 5.4 TanStack Query 配置

```js
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,         // 30s 内不重新 fetch
      refetchOnWindowFocus: false, // 桌面 app，不需要
      retry: 2,
    }
  }
})
```

**invalidation 策略：** SSE notifications 收到 entity 变更时，精确 invalidate 对应 query key。例如收到 `{ type: "conversation", action: "updated" }` → `queryClient.invalidateQueries(["conversations"])`。

---

## §6 SSE 层（3 个 hook）

### 6.1 `useEventLog(userId)`

订阅 `GET /api/v1/eventlog`，驱动 `chat store` 更新。

```
连接策略：
  - App 启动后立即连接（全局单实例）
  - 断线自动重连，指数退避（1s → 2s → 4s → 最大 30s）
  - 重连时带 Last-Event-ID: {lastSeq}
  - 服务器返 410 SEQ_TOO_OLD → 清 chat store → REST 重拉当前 conv 历史 → 重连

事件处理：
  event: message_start  → chat.onMessageStart(conversationId, event)
  event: message_stop   → chat.onMessageStop(conversationId, event)
  event: block_start    → chat.onBlockStart(conversationId, event)
  event: block_delta    → chat.onBlockDelta(conversationId, event)
  event: block_stop     → chat.onBlockStop(conversationId, event)

SSE wire 格式解析：
  es = new EventSource(`${baseUrl}/api/v1/eventlog`)
  es.addEventListener("message_start", handler)   // 按 event name 订阅
  // 注意：SSE event name 就是 event type，不是一个 generic "message"

连接状态：返 { status: "connecting" | "connected" | "disconnected" | "error" }
```

### 6.2 `useNotifications(userId)`

订阅 `GET /api/v1/notifications`，驱动 TanStack Query 失效。

```
连接策略：同 useEventLog（断线重连 + Last-Event-ID）

事件处理（SSE event name 固定为 "notification"）：
  es.addEventListener("notification", (e) => {
    const { type, id, data, conversationId } = JSON.parse(e.data)
    dispatch(type, id, data, conversationId)
  })

dispatch 映射（type → invalidate）：
  "conversation"  → invalidateQueries(["conversations"]) + ["conv", id]
  "function"      → invalidateQueries(["forges"]) + ["forge", id]
  "handler"       → 同上
  "workflow"      → 同上
  "flowrun"       → invalidateQueries(["flowruns"]) + ["flowrun", id]
  "mcp_server"    → invalidateQueries(["mcp-servers"])
  "skill"         → invalidateQueries(["skills"])
  "memory"        → invalidateQueries(["memories"])
  "catalog"       → invalidateQueries(["catalog"])
  "todo"          → invalidateQueries(["todos"])
  "ask"           → ui store 中设置 pendingAsk = event（红点 + 弹出 AskUserModal）
  "compaction"    → invalidateQueries(["conv", conversationId])

未读 badge：notifications hook 维护本地 unreadCount（连接后收到的通知数，查看通知抽屉后清零）
```

### 6.3 `useForge(userId)`

订阅 `GET /api/v1/forge`，驱动锻造进度 UI。

```
事件类型（按 event name 订阅）：
  forge_started     → forgeStore.onStarted(scope, operation, convId, toolCallId)
  forge_op_applied  → forgeStore.onOpApplied(scope, index, op)
  forge_env_attempt → forgeStore.onEnvAttempt(scope, attempt, status, stage, detail, error)
  forge_completed   → forgeStore.onCompleted(scope, status, versionId, envStatus, attemptsUsed, error)
    + 完成时 invalidate ["forges"] + ["forge", scope.id]

forge store（Zustand，合并到 ui.js）：
  activeForge: Map<scopeKey, ForgeProgress>
  ForgeProgress: { scope, operation, ops: [], envAttempts: [], status, conversationId, toolCallId }
  scopeKey = `${scope.kind}:${scope.id}`

哪里消费：
  - FunctionDetail / HandlerDetail / WorkflowDetail 头部的锻造进度条
  - chat pane 的 tool_call block（progress block delta 经 eventlog 双写）
  - forge list 的 "正在锻造" 行状态
```

### 6.4 SSE hook 实现要点

```js
// 共用的 createSSEHook 工厂（sse/shared.js）
function createSSEHook(path, eventHandlers) {
  return function useSSE() {
    const baseUrl = useUIStore(s => s.baseUrl)
    const lastSeqRef = useRef(0)
    const esRef = useRef(null)
    const [status, setStatus] = useState("connecting")

    useEffect(() => {
      if (!baseUrl) return
      let retryDelay = 1000
      let retryTimer = null
      let destroyed = false

      function connect() {
        const url = new URL(`${baseUrl}${path}`)
        // 重连时带 Last-Event-ID
        const headers = lastSeqRef.current > 0 ? { "Last-Event-ID": lastSeqRef.current } : {}
        // EventSource 不支持自定义 headers，用 URL param 兜底
        if (lastSeqRef.current > 0) url.searchParams.set("lastEventId", lastSeqRef.current)

        const es = new EventSource(url.toString())
        esRef.current = es
        setStatus("connecting")

        es.onopen = () => { setStatus("connected"); retryDelay = 1000 }
        es.onerror = () => {
          setStatus("disconnected")
          es.close()
          if (!destroyed) {
            retryTimer = setTimeout(() => {
              retryDelay = Math.min(retryDelay * 2, 30_000)
              connect()
            }, retryDelay)
          }
        }

        for (const [event, handler] of Object.entries(eventHandlers)) {
          es.addEventListener(event, (e) => {
            const seq = parseInt(e.lastEventId || 0)
            if (seq > lastSeqRef.current) lastSeqRef.current = seq
            // 410 处理：在 onerror 里检查 e.status（EventSource 不直接暴露，用轮询 /health 判断）
            handler(JSON.parse(e.data), seq)
          })
        }
      }

      connect()
      return () => { destroyed = true; clearTimeout(retryTimer); esRef.current?.close() }
    }, [baseUrl])

    return { status }
  }
}
```

**注意：** EventSource 不支持自定义 request headers。后端通过 URL query `?userID=` 兜底识别用户（`api-design.md §20` 已注明）。

---

## §7 API 客户端层

### 7.1 fetch wrapper（`api/client.js`）

```js
async function apiFetch(path, options = {}) {
  const baseUrl = useUIStore.getState().baseUrl
  const res = await fetch(`${baseUrl}/api/v1${path}`, {
    headers: {
      "Content-Type": "application/json",
      "Accept": "application/json",
      // 单用户：不需要 X-Forgify-User-ID header（后端默认 local-user）
      ...options.headers,
    },
    ...options,
  })
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: { code: "UNKNOWN", message: res.statusText } }))
    throw Object.assign(new Error(err.error.message), { code: err.error.code, status: res.status })
  }
  if (res.status === 204) return null
  return res.json().then(j => j.data ?? j)  // 剥 data envelope
}
```

### 7.2 TanStack Query hooks（举例）

```js
// api/conversations.js
export function useConversations() {
  return useQuery({
    queryKey: ["conversations"],
    queryFn: () => apiFetch("/conversations?limit=50"),
  })
}

export function useConversation(id) {
  return useQuery({
    queryKey: ["conv", id],
    queryFn: () => apiFetch(`/conversations/${id}`),
    enabled: !!id,
  })
}

export function useConversationMessages(convId) {
  return useQuery({
    queryKey: ["conv-messages", convId],
    queryFn: () => apiFetch(`/conversations/${convId}/messages?limit=100`),
    enabled: !!convId,
  })
}

export function useSendMessage(convId) {
  return useMutation({
    mutationFn: ({ content, attachments }) =>
      apiFetch(`/conversations/${convId}/messages`, {
        method: "POST",
        body: JSON.stringify({ content, attachments }),
      }),
    // 不 invalidate — 等 SSE 事件驱动 UI 更新
  })
}
```

**命名规范：**
- `useXxx()` — query hook（读）
- `useXxxMutation()` 或 `useCreateXxx / useUpdateXxx / useDeleteXxx` — mutation hook（写）

---

## §8 App Shell

### 8.1 布局

```
.app（grid: sidebar | main）
├── <Sidebar>
└── <main>
    ├── （openPanes.length === 0）→ <Dashboard>
    └── （openPanes.length > 0）→
        openPanes.map((k, i) => <PaneWrapper key={k} index={i}>)
            PaneWrapper 包含 <PaneChrome> + 对应的 <XxxPane>
        （两个 pane 时中间插入 <PaneResize>）
```

**Framer Motion pane 进出：**
```jsx
<AnimatePresence mode="popLayout">
  {openPanes.map((k, i) => (
    <motion.div
      key={k}
      layout
      initial={{ opacity: 0, scale: 0.98 }}
      animate={{ opacity: 1, scale: 1 }}
      exit={{ opacity: 0, scale: 0.96 }}
      transition={easeOut}
    >
      <PaneChrome kind={k} onClose={() => closePane(k)}>
        {renderPane(k)}
      </PaneChrome>
    </motion.div>
  ))}
</AnimatePresence>
```

**PaneChrome：** chat pane 没有 pane-bar（自带 header）；其他 pane 有 pane-bar（面包屑 + 关闭按钮）。

**双 pane resize：** `PaneResize` 组件，mousedown → window mousemove 计算百分比。`leftPct` 约束在 [20, 80]。resize 时用 CSS `pointer-events: none` 防止 iframe/canvas 抢事件。

**narrow 模式（主区域 < 1000px）：** 用 ResizeObserver 检测 `<main>` 宽度。narrow 时隐藏 inactive 的 pane，底部显示 pane 切换 tab 栏。

### 8.2 Sidebar

**结构：**
```
.sidebar
├── sidebar-header
│   ├── workspace-pill（显示当前 profile 名）
│   └── cmdk-trigger（⌘K 触发命令板）
├── nav-section（主导航）
│   ├── SideNavItem "对话"（chat）
│   ├── SideNavItem "锻造"（forge）
│   ├── SideNavItem "执行"（execute）
│   ├── SideNavItem "文档"（documents）
│   └── SideNavItem "洞察"（observe — Phase 5，暂显示 disabled 状态）
├── nav-section "资源库"
│   ├── SideNavItem "Skills"
│   ├── SideNavItem "MCP"
│   └── SideNavItem "Memory"
├── nav-conv-section（flex-1，可滚动）
│   ├── "置顶" 分组
│   ├── 对话列表（ChatListItem × N）
│   └── "归档" 折叠分组（Framer Motion AnimatePresence 控制展开/折叠）
└── sidebar-footer
    ├── AgentBusyStrip（agent 在后台跑时显示）
    └── user-pill（头像 + 名字 + SSE 状态点 + Bell/Ask/Settings 图标）
```

**数据：** `useConversations()` 提供列表，notifications SSE 驱动 invalidate。

**ChatListItem 改进：**
- hover 时显示 `ConvMore` 菜单按钮（boilerplate 已有但用 window 挂载，改成正常 ref）
- `status: "streaming"` → 左侧 dot 脉动动画 + 行有 `.is-streaming` 高亮
- `status: "approval"` → warn 色 dot + `!` badge

**SideNavItem active 状态：** pane 打开时高亮（`openPanes.includes(kind)`）。

**Framer Motion sidebar collapse：**
```jsx
<motion.aside
  animate={{ width: collapsed ? 48 : 248 }}
  transition={spring}
  className="sidebar"
>
```
折叠时只显示图标，文字 fade out。

**AgentBusyStrip：** 当 `chat.convStates` 里有任何 conversation 有正在 streaming 的 message，且 chat pane 没打开时，显示底部 strip，点击打开 chat pane。

**SSE 状态点（user-pill 右侧的小圆点）：** 三流 hook 的 status 综合成一个颜色：全 connected → green，任一 connecting → yellow，任一 disconnected/error → red。

### 8.3 全局键盘快捷键

在 App 层注册（`useEffect + window.addEventListener`）：

| 快捷键 | 行为 |
|---|---|
| `⌘K` | 打开命令板 |
| `⌘B` | 折叠/展开 sidebar |
| `⌘1-9` | 切换到第 N 个 conversation（conversations 列表顺序）|
| `Esc` | 按优先级关闭：命令板 → 设置 popover → AskUserModal → NotificationsDrawer |

**输入框内的 Esc 不触发全局：** 检测 `e.target.tagName === "TEXTAREA" || e.target.tagName === "INPUT"` 则 skip。

---

## §9 Chat Pane

### 9.1 数据流

```
1. 打开 chat pane → useConversations() 已有列表
2. 切换 conversation（setActiveConv）→ 拉 useConversationMessages(convId)（历史）
3. SSE useEventLog 实时推送 → chat store 更新
4. ChatPane 合并：历史 messages（REST）+ 实时 streaming（store）
   合并算法：以 message.id 去重，streaming 消息覆盖同 id 的历史版本
5. 用户发消息 → POST /conversations/{id}/messages → 等 SSE 推回来（不直接改 store）
```

**历史 messages REST endpoint：**
`GET /api/v1/conversations/{id}/messages?limit=100`
返回 messages 数组，每条含 `blocks[]`（从 DB 读全量内容）。

### 9.2 组件树

```
ChatPane
├── ChatHeader
│   ├── 标题（conv.title，auto-title 后自动更新）
│   ├── conv.id 的 EntityRelMeta（小字：与哪些实体有关）
│   ├── model-tag（provider + modelId，点击→选择器，Phase 2 先做 display 只读）
│   ├── icon-btn：Layers（附加 Skill/Memory，Phase 5 实现）
│   ├── icon-btn：Search（对话内搜索，Phase 5 实现）
│   └── icon-btn：Settings（对话设置）
├── chat-stream（flex-1，overflow-y: auto）
│   ├── day-divider（今天 · 日期）
│   └── messages.map(msg => <MessageView key={msg.id} msg={msg} />)
│       带 scroll-to-bottom：新 message 到来时（用 ref + requestAnimationFrame 双帧）
└── Composer
```

### 9.3 MessageView

```
.msg.role-{user|assistant}
├── msg-meta（作者 / 时间 / token 数 / streaming badge）
│   ├── msg-author-avatar（用户 "Y"，AI 用 provider 前两字母）
│   ├── RelTime
│   ├── token 数（inputTokens ↓ outputTokens ↑，仅 assistant 显示）
│   ├── streaming badge（仅 message.status === "streaming"）
│   └── msg-actions（hover 显示：复制 / 重新生成 / 编辑并重发 / 分叉 / 更多）
├── msg-body
│   └── BlockList blocks={msg.blocks}
└── attachments strip（如有附件）
```

**msg-actions 改进：** boilerplate 的 actions 是固定 display。改成：`msg-meta` hover 时 actions fade in（`opacity: 0 → 1`，`transition: var(--t-fast)`）。

### 9.4 BlockRenderer（核心）

**7 种 block type：**

```
BlockList({ blocks, depth=0, defaultOpenTools=false })
└── 将 blocks 按 execution_group 分组（同 group 的 tool_call 并行展示）
    → groups.map(g => {
        if (g.eg && g.items.length > 1)
          → <ToolBatch>（左侧 vertical bar 标识并行）
        else
          → g.items.map(b => <Block type={b.type} />)
      })
```

**TextBlock：**
- 最小化 markdown 渲染：`**bold**`、`` `code` ``、`- bullet`、段落
- 实体 ID 自动变 `<EntityLink>`（正则：`\b(?:fn|hd|wf|sk|mcp|mem|cv|fr)_[a-z0-9]{2,16}\b`）
- streaming 时末尾显示打字光标（`<span className="streaming-caret">`，CSS blink 动画）

**ReasoningBlock（默认折叠）：**
```
.blk-reasoning（collapsible）
├── .blk-reasoning-head（点击展开）
│   ├── ChevronRight icon（旋转 90° 表示展开）
│   ├── Brain icon
│   ├── "已思考 Xs"
│   ├── streaming 时：dot-pulse 动画
│   └── "N chars" 字符数
└── .blk-reasoning-body（展开时显示）
```
`defaultOpen` 由 `settings.reasoningDefault === "expanded"` 控制。

**ToolCallBlock（复杂）：**
```
.blk-tool（collapsible）
├── .blk-tool-head（点击展开）
│   ├── 工具图标（根据 tool 名称映射，见 toolIcon()）
│   ├── tool 名称 + execution_group 标记
│   ├── summary（attrs.summary）
│   └── 状态（streaming spinner / 完成耗时 / error badge）
└── .blk-tool-body（展开时）
    ├── Arguments section（code-block 显示 JSON args）
    ├── Progress blocks（按序 append）
    └── tool_result block
```

**SubagentBlock（可展开的嵌套 message）：**
```
.blk-subagent
├── .blk-subagent-head
│   ├── Bot icon
│   ├── "Subagent · {agentType}"
│   └── "{N 步} · {耗时} · {tokens}"
└── .blk-subagent-body（展开）
    └── BlockList blocks={inner.blocks} depth={depth+1} defaultOpenTools
```

**CompactionBlock（新 block type，boilerplate 没有）：**
```
.blk-compaction（不折叠，inline 展示）
├── Archive icon
├── "对话已压缩"
├── "涵盖 {blocksArchived} 个 block 的摘要 · 由 {generatedBy} 生成"
└── 摘要 markdown 文本（折叠，点击展开）
```

**改进vs boilerplate：**
1. boilerplate 的 `renderInline` 有 regex bug（`\b` 在 ID 末尾不可靠），改用贪婪前后缀匹配
2. 增加 `compaction` block type 的渲染
3. ToolCallBlock 的 Arguments 支持 "copy" 按钮（写入剪贴板）

### 9.5 Composer

```
.composer-wrap
├── attached-strip（文件 + @mention 标签，Framer Motion layout 动画进出）
└── .composer-inner
    ├── .composer（主输入区）
    │   ├── SlashPopover（"/"触发，箭头键 + Enter 选择）
    │   ├── AtMentionPopover（"@"触发，搜索 forge/skill/doc 实体）
    │   ├── DropZone 指示层（drag over 时显示）
    │   └── textarea（auto-resize，最高 200px）
    └── .composer-toolbar
        ├── Paperclip（触发文件选择，原生 input[type=file]）
        ├── @（追加 @）
        ├── composer-mode（Agent 模式选择器）
        └── send-btn（Enter 发送 / Shift+Enter 换行 / Esc 停止 streaming）
```

**send-btn 状态：**
- 无文字：disabled（半透明）
- 有文字：可点击（accent 色）
- streaming：stop 按钮（方块图标）

**mentionPool：** 调 `useForgeList()` + `useSkills()` + `useDocuments()`，过滤后提供 @mention 候选。

**改进vs boilerplate：**
1. 文件 attach 用真实的 `input[type=file]`（boilerplate 只是模拟）
2. `isStreaming` 状态从 `chat store` 读取（当前 conv 是否有 streaming message），不是外部 prop
3. @mention 搜索 debounce 300ms
4. AtMentionPopover 支持 type label（"· function"、"· skill"）

### 9.6 NoApiKeyGate

当 `useApiKeys()` 返回空列表时，chat 区域显示引导卡片，提示去 Config pane 添加 API Key。

### 9.7 EntityRelMeta

boilerplate 中 `chat.jsx` 和 `execute.jsx` 引用了 `EntityRelMeta` 但没有实现。这是一个"该实体与其他实体的关系"的小展示组件。

**数据：** `GET /api/v1/relations?entityId={id}&limit=5`（relation domain）

**渲染：** inline，`font-size: 11px`，最多显示 3 个邻居实体的 EntityLink。例：
```
与 fn_xxx · wf_yyy 相关
```

---

## §10 Forge Pane

### 10.1 结构（list ↔ detail 路由）

Forge pane 内部有自己的 navigation 状态：
```
ForgePane
├── （open === null）→ ForgeList
└── （open.kind === "function"）→ FunctionDetail
    （open.kind === "handler"）→ HandlerDetail
    （open.kind === "workflow"）→ WorkflowDetail
```

状态：`const [open, setOpen] = useState(null)`

**从外部 openEntity("forge", id) 触发时：** ForgePane mount 后或 focusEntity.forge 变化时，根据 id 调 `GET /forges/{id}` 拿到 forge 信息，setOpen(forge)。

**Framer Motion detail ↔ list 过渡：**
```jsx
<AnimatePresence mode="wait">
  {open ? (
    <motion.div key="detail" {...slideUp}>...</motion.div>
  ) : (
    <motion.div key="list" {...fadeIn}>...</motion.div>
  )}
</AnimatePresence>
```

### 10.2 ForgeList

**数据：** `GET /api/v1/forges?limit=50`（后端有这个统一 list endpoint 还是分开？看 api-design.md）

**实际情况：** 后端 function/handler/workflow 是分开的 domain，没有统一 /forges endpoint。前端需要并发拉三个：
```js
const { data: functions } = useFunctions()   // GET /functions
const { data: handlers } = useHandlers()     // GET /handlers
const { data: workflows } = useWorkflows()   // GET /workflows
const forges = useMemo(() =>
  [...(functions || []).map(f => ({...f, kind:"function"})),
   ...(handlers || []).map(h => ({...h, kind:"handler"})),
   ...(workflows || []).map(w => ({...w, kind:"workflow"}))]
  .sort((a, b) => new Date(b.updatedAt) - new Date(a.updatedAt)),
  [functions, handlers, workflows]
)
```

**渲染：**
```
ForgeList
├── page-header（标题 + 新建 / 导入 按钮）
├── page-tabs（全部 / Functions / Handlers / Workflows + count badge）
├── page-toolbar（搜索 input + 排序）
├── batch-bar（多选时出现，Framer Motion slideDown）
└── table（行点击 → setOpen(forge)）
```

**table 列：** checkbox / 名称+描述 / 类型 / 版本 / 运行次数 / 状态 / 最近更新 / ActionMenu

**forge SSE 进度：** 当某 forge 正在被 AI 锻造（`activeForge` store 有对应 scope），在表格行显示进度动画（spinner 替换 status badge）。

**搜索：** 本地 filter（三个列表合并后 client-side 搜索），不发 API。

### 10.3 FunctionDetail

**数据：**
- `GET /api/v1/functions/{id}` → function 基本信息
- `GET /api/v1/functions/{id}/versions` → 所有版本

**布局（vr-shell）：**
```
FunctionDetail
├── page-header（返回按钮 / KindChip / id / 名称 / EntityRelMeta）
│   └── actions：试跑 / AskAiTrigger / 更多
│       （pending 版本时：Revert / Iterate / Accept · vN）
├── vr-shell（两栏）
│   ├── vr-main（左）
│   │   ├── （isViewingCurrent）→ FunctionFullView
│   │   └── （isViewingOther）→ FunctionDiffView
│   └── VersionRail（右，可折叠）
```

**FunctionFullView（当前版本）：**
```
├── 版本标签 + state badge（current / pending / archived）
├── FieldRow "说明" / "输入" / "输出" / "运行环境"
├── CodeView（Python 语法高亮，行号）
└── 最近试跑列表
```

**FunctionDiffView（对比历史版本）：**
```
├── Diff 标题（currentV.label ⇆ otherV.label + 变更数）
├── （descChanged）→ 说明 2 栏对比
├── （inputsChanged || outputsChanged）→ 契约 2 栏对比
└── （codeChanged）→ SplitDiff（LCS 行级 diff，高亮 add/del）
```

**CodeView：** 简单 Python 语法高亮（关键字/内置函数/字符串/注释/数字），已在 boilerplate 实现，直接移植。

**SplitDiff：** LCS-based 行级 diff，side-by-side 显示。boilerplate 已有实现，直接移植。

**AskAiTrigger：** boilerplate 引用但没有实现。这是一个触发按钮，点击后打开（或 focus）chat pane，pre-fill 消息内容（带 `@{entity.id}` 引用），或者显示 suggestions 的快捷选项。
```
AskAiTrigger({ context, suggestions })
└── 按钮 "AI · 迭代"（点击展开 suggestions popover）
    suggestions popover：
      - 每条 suggestion 点击 → openConv + 发送该 suggestion + @entity 引用
      - 或者直接 → /conversations/{id}:iterate action（返 conversationId → 打开 chat）
```

**试跑：** 点击后显示一个 modal 让用户填写 inputs JSON，发到 `POST /functions/{id}:run`，结果显示在 modal 里。（Phase 2 先做 button+placeholder，Phase 3 实现）

### 10.4 HandlerDetail

**数据：** `GET /api/v1/handlers/{id}` + `GET /api/v1/handlers/{id}/versions`

**布局：** 同 FunctionDetail（vr-shell + VersionRail）

**HandlerFullView（当前版本，3 tab）：**

- **Class tab：** 左侧 method 列表 + 右侧选中 method 的签名/描述。
- **Config tab：** key-value 表，secret 字段 masked，有 copy 按钮（`GET /api/v1/handlers/{id}/config`）。
- **Call 历史 tab：** 4 KPI stat-card（成功率/p50/p95/p99）+ 最近 calls 表（时间/方法/状态/耗时/错误）。

**HandlerDiffView（对比历史版本）：** 显示 method 变更（新增/删除/修改），config 变更。

**试调用：** 选方法 → 填 args → `POST /api/v1/handlers/{id}:call` → 显示 result。

### 10.5 WorkflowDetail

**数据：** `GET /api/v1/workflows/{id}` + `GET /api/v1/workflows/{id}/versions`

**布局（vr-shell）：**
```
WorkflowDetail
├── page-header（返回 / KindChip / id / 名称 / 描述 / EntityRelMeta）
│   └── actions：Capability check / 试跑 / AskAiTrigger
│       （isViewingCurrent）→ 保存状态点
├── vr-shell
│   ├── vr-main
│   │   ├── （isViewingCurrent）→ WorkflowEditor（可编辑画布）
│   │   └── （isViewingOther）→ WorkflowDiffView（只读 DAG + 变更清单）
│   └── VersionRail（右，含 pending banner + Deploy 按钮）
```

**WorkflowEditor（可编辑画布）：**
```
.wf-editor
├── <Palette>（左侧节点面板，拖放 or 点击 onAdd）
└── <Canvas>（主画布，pan + zoom + 节点拖动 + 连线）
    ├── SVG edges（cubic bezier 曲线 + arrowhead）
    ├── nodes（.wf-node，位置用 CSS absolute）
    ├── 连线交互（handle mousedown → 虚线跟随鼠标 → 放到目标 handle）
    └── .wf-canvas-toolbar（放大/缩小/自动布局/适配）
```

**节点类型（13 种）：** trigger / function / handler / mcp / skill / llm / http / condition / loop / parallel / approval / wait / variable

每个节点有 4 个连线 handle（top/right/bottom/left），hover 时显示。

**WorkflowDiffView：** 只读 DAG 展示两个版本的差异（added/removed/changed 节点用颜色标注）+ 左侧变更清单列表。

**画布改进vs boilerplate：**
1. 节点 config 点击节点后在 Palette 区域显示 inspector（boilerplate 没有实现）
2. Pan 用 `Space + drag`（boilerplate 是点空白处拖动），更符合 Figma 习惯
3. 自动保存：编辑后 2s debounce → `PATCH /workflows/{id}`（boilerplate 只有 dirty 状态标记）

---

## §11 Execute Pane

### 11.1 结构

```
ExecutePane
├── （openRun === null）→ ExecuteOverview
└── （openRun）→ FlowRunDetail
```

### 11.2 ExecuteOverview

**数据：**
- `useFlowRuns()` → `GET /api/v1/flowruns?limit=50`
- 实时：notifications SSE 的 `flowrun` 事件 → invalidate

**布局：**
```
ExecuteOverview
├── page-header（标题 + 刷新）
├── page-toolbar（搜索 + 状态筛选 seg）
├── page-body
│   ├── KpiStrip（今日运行 / 成功率 / 中位耗时 / 待批准，各带 sparkline）
│   ├── WorkflowHeatmap（各 workflow 最近 30 次状态 grid）
│   └── page-tabs（FlowRuns / 待批准 / 触发器）
│       ├── FlowRunsTable
│       ├── ApprovalsQueue
│       └── TriggersGrid
```

**KpiStrip：** 数据从 `GET /api/v1/flowruns/stats`（如有）或 client 端聚合 flowruns 列表。sparkline 用 SVG polyline。

**WorkflowHeatmap：** 每行一个 workflow，最近 30 次运行状态色块。点击 workflow 名 → 过滤 FlowRunsTable。

**FlowRunsTable：** 行点击 → `setOpenRun(fr)` 进 detail。列：Workflow / 状态 badge / 节点进度 mini-bar / 触发源 / 开始时间 / 耗时 / ActionMenu。

**ApprovalsQueue：** 卡片列表，每张卡片有拒绝/暂存/批准按钮。`POST /flowruns/{id}/nodes/{nodeId}:approve` 或 `:reject`。

### 11.3 FlowRunDetail

**数据：** `GET /api/v1/flowruns/{id}` + `GET /api/v1/flowruns/{id}/nodes`

**布局：**
```
FlowRunDetail
├── page-header（返回 / run id / workflow 名 / 状态 badge / 触发信息）
│   └── actions：与历史 diff / 取消 / 批准并继续 / AI 排查 / 重跑
├── （showTriage && status=failed）→ TriagePanel（inline 折叠面板）
├── （showDiff）→ RunDiffPanel（inline 折叠面板）
├── fr-shell（上方：DAG + NodeInspector 左右两栏）
└── GanttTimeline（下方）
```

**FlowRunDag：** SVG edges + absolute div nodes。节点点击 → 更新 selected → 右侧 NodeInspector 展示 input/output/log。running 节点有 spinner，failed 节点有红色。

**NodeInspector：** 选中节点的 input JSON / output JSON / 执行日志（带 level 颜色）。

**GanttTimeline：** 每行 = 一个节点，bar 宽度 = 节点耗时/总耗时，颜色 = 节点状态。

**TriagePanel（AI 排查）：**
- 按钮点击 → `POST /flowruns/{id}:triage` → 返 `{conversationId}` → 打开 chat pane 跳到该 conversation（AI 在里面实时分析排查结论）
- 如果后端没有 :triage endpoint，先 inline 显示 mock（Phase 1），Phase 2 接真实 endpoint

**改进vs boilerplate：**
- FlowRunDag 节点 tooltip（hover 显示耗时/状态/retry count）
- GanttTimeline 节点行点击 → 同步选中 DAG 中对应节点

---

## §12 Config Pane

### 12.1 布局

```
ConfigPane
├── page-header（标题 "设置"）
├── page-tabs（API Keys / Model / Sandbox / 外观 / 数据）
└── page-body
```

### 12.2 API Keys tab

**数据：** `useApiKeys()` → `GET /api/v1/api-keys`

**表格：** Provider / 名称 / 掩码 Key / 状态（verified badge）/ 最近使用 / 删除/查看

**添加 Key 流程：**
```
"+ 添加 Provider" 按钮
→ Drawer（Framer Motion slide-in from right）
  ├── Provider 选择器（从 GET /providers 获取列表，带分类）
  ├── 名称输入
  ├── Key 输入（password type）
  ├── Base URL（可选，用于自定义端点）
  └── "测试并保存"按钮
      → POST /api-keys → 成功 → invalidate → 关闭 drawer
      → POST /api-keys/{id}:test（自动）→ 显示结果
```

### 12.3 Model tab

**数据：** `useModelConfigs()` → `GET /api/v1/model-configs`

每个 scenario 一张卡片：显示 provider + modelId，"切换"按钮 → inline 编辑（选 provider → 填 modelId）→ `PUT /model-configs/{scenario}`。

### 12.4 Sandbox tab

**数据：** `useSandboxStatus()` → `GET /api/v1/sandbox/status`（如有）

显示 mise 版本、安装的 runtime（python/node/...）及版本。

### 12.5 外观 tab

theme / accent / density / lang 选择器，实时预览，写 `settings store` → localStorage。

与 SettingsPopover（sidebar footer 的快捷设置）共用同一套 controls。

### 12.6 数据 tab

**数据目录位置**：`~/.forgify/`（显示路径 + 打开文件夹按钮 → Wails binding `runtime.BrowserOpenURL`）
**存储大小**：占位，Phase 2 实现

---

## §13 Library Panes

### 13.1 SkillsPane

**数据：** `useSkills()` → `GET /api/v1/skills`

**布局：**
```
SkillsPane
├── page-header（Skills + 导入/创建 按钮）
├── page-toolbar（搜索 input）
└── 卡片 grid
    SkillCard：
    ├── 技能名（.skill-name）
    ├── 描述
    ├── 标签（from frontmatter）
    └── ActionMenu（激活 / 编辑 / 删除）
```

**技能详情（点击卡片）：** 右侧 detail panel（或新 pane），显示 SKILL.md 全文（markdown 渲染）。

**notifications 更新：** `type: "skill"` → invalidate skills。

### 13.2 McpPane

**数据：** `useMcpServers()` → `GET /api/v1/mcp-servers`

**布局：**
```
McpPane
├── page-header（MCP + 添加服务器 按钮）
└── 服务器列表
    McpServerCard：
    ├── 服务器名 + status badge（running/stopped/degraded）
    ├── 工具数量
    ├── 健康历史（最近 N 次检查的 mini 状态点）
    └── ActionMenu（停止/重连/移除）
```

**添加服务器：** Drawer，支持 stdio/SSE 两种 transport，命令行输入。

### 13.3 MemoryPane

**数据：** `useMemories()` → `GET /api/v1/memories`（支持 `?type=user|feedback|project|reference` 过滤）

**布局：**
```
MemoryPane
├── page-header（Memory + 新建 按钮）
├── page-tabs（全部 / user / feedback / project / reference）
└── 卡片列表
    MemoryCard：
    ├── name（mono 字体）
    ├── 类型 badge
    ├── description（一行摘要）
    ├── pinned 图标（pinned memory 有 accent 色 pin）
    └── ActionMenu（pin/unpin / 编辑 / 删除）
```

**详情 / 编辑：** 点击卡片 → Drawer，显示全文（markdown），可编辑（`PUT /memories/{name}`）。

### 13.4 DocumentsPane

**数据：** `useDocuments()` → `GET /api/v1/documents`

（Phase 5 能力，Phase 1 先显示 empty state + 占位 UI）

**布局：** 树形文档列表（Notion 风格，folder + page）+ 右侧预览区。

---

## §14 Overlay 系统

### 14.1 CommandPalette（⌘K）

```
CommandPalette
├── （open）→ AnimatePresence motion.div（overlay + cmdk card，slide down + fade in）
├── cmdk-input（自动 focus，过滤）
├── cmdk-list（分组：导航 / 动作 / 最近对话 / Forge）
│   每行：icon + label + desc + shortcut + Enter 选中 indicator
└── cmdk-footer（keyboard hints）
```

**数据：** 静态导航项 + `useConversations()` 前 5 条 + `useForgeList()` 前 5 条

**改进vs boilerplate：** 搜索时真实过滤 DB 数据（`conversations` + `forges`），通过 API 搜索 endpoint（`GET /conversations?q=xxx`），而不是 client-side filter mock 数据。

### 14.2 NotificationsDrawer

```
NotificationsDrawer
└── AnimatePresence motion.div（从右 slide in）
    ├── 抽屉背景遮罩（点击关闭）
    └── .drawer（position: fixed right）
        ├── drawer-head（标题 / 未读数 / 全部已读 / 关闭）
        └── drawer-list（时间分桶：现在 / 今天稍早 / 更早）
            每条 notif：icon / 标题 / 描述 / 时间 / 点击打开对应 pane
```

**数据：** SSE hook 维护的内存 list + REST snapshot（`GET /api/v1/notifications?limit=50`）初始加载。

**点击 notif 行：** 按 type 打开对应 pane，如果是 forge entity → openEntity("forge", id)。

**动效：** 新通知到来时 Framer Motion `layout` 动画将已有条目向下推。

### 14.3 AskUserModal（Agent 问题）

```
AskUserModal
└── AnimatePresence motion.div（overlay + ask-card，scale + fade）
    ├── ask-head（HelpCircle / "AGENT 暂停·等待你的输入" / 关闭）
    ├── ask-body
    │   ├── 问题文本（带 @entity 渲染）
    │   └── ask-options（单选，keyboard 1-N 快捷键）
    └── ask-footer（keyboard hints + "推迟" / "提交答复"）
```

**触发：** notifications SSE 收到 `{ type: "ask", data: { toolCallId } }` → `ui store.pendingAsk = event` → AskUserModal 自动打开。

**提交：** `POST /api/v1/conversations/{convId}/pending-questions/{toolCallId}:resolve`（带 selected answer）。

**改进vs boilerplate：** 选项内容从 SSE 推过来的 `pendingQuestion` 数据拉取，而不是 mock 硬编码的 4 个选项。

### 14.4 ApprovalBanner（固定底部条）

当有 `waiting_approval` 的 flowrun，且 execute pane 没打开时，显示底部 sticky 条。

```
ApprovalBanner（position: fixed bottom center）
└── Framer Motion（slideUp from bottom，有 waiting 时显示）
    ├── Pause icon
    ├── "{workflow 名} 等待批准"
    ├── 子信息（节点/触发时间）
    └── "查看" / "批准" 按钮
```

### 14.5 ToastTray

```
ToastTray（position: fixed bottom right）
└── AnimatePresence（Framer Motion layout）
    每条 toast（Framer Motion slideUp + auto dismiss）：
    ├── status icon（check / warning / error）
    ├── title + desc
    ├── undo 按钮（如有）
    └── 关闭 × 按钮
```

**toast 来源：**
1. 操作成功/失败的 mutation callback（如 Accept 版本）
2. `Shell.toast(...)` 全局 API（各 pane 可调用）

---

## §15 实现顺序（Phase 列表）

每个 Phase 完成后打勾。不要跳 Phase，不要并行多 Phase。

### Phase 0：脚手架 ⬜
- [ ] `wails init` 创建 Wails v2 项目骨架
- [ ] 配置 Vite 作为前端构建工具（Wails 内置支持）
- [ ] 安装依赖：React 18、Framer Motion、TanStack Query v5、Zustand v5、Lucide React
- [ ] `cmd/desktop/app.go`：`App.GetBackendPort()` 方法，启动已有 HTTP server
- [ ] `bridge/wails.js`：`initBaseUrl` + `getBaseUrl`
- [ ] Vite dev proxy：`/api → localhost:7788`（开发期不需要 GetBackendPort）
- [ ] `main.jsx`：QueryClientProvider + bootstrap + 挂载 App
- [ ] 验证：能启动，能用 DevTools 看到前端

### Phase 1：设计系统 + 原子组件 ⬜
- [ ] 迁移 `styles.css` → 拆分 `tokens.css` / `base.css` / `components.css` / `panes.css`
- [ ] `components/primitives/`：Button / Badge / Spinner / Kbd
- [ ] `components/primitives/Icon.jsx`：Lucide React 统一 re-export，映射 boilerplate 的所有 Icon 名
- [ ] `components/shared/RelTime.jsx`：30s 自动刷新
- [ ] `components/shared/KindChip.jsx`
- [ ] `components/shared/StatusBadge.jsx`
- [ ] `components/shared/ActionMenu.jsx`：floating-ui 定位 + portal
- [ ] 验证：能看到所有基础组件，样式正确，主题切换生效

### Phase 2：App Shell ⬜
- [ ] `store/ui.js`：Zustand store（panes/activeConv/etc）
- [ ] `store/settings.js`：Zustand persist（theme/accent/density）
- [ ] settings 变化 → 写 `document.documentElement.dataset.*`
- [ ] `AppShell.jsx`：grid layout（sidebar + main）
- [ ] `Pane.jsx`（chrome）+ `PaneResize.jsx`
- [ ] Framer Motion pane 进出动画（`AnimatePresence`）
- [ ] narrow 模式（ResizeObserver）
- [ ] `Sidebar.jsx`：静态骨架（无真实数据）
- [ ] 全局键盘快捷键（⌘K / ⌘B / Esc）
- [ ] `Dashboard.jsx`（空壳，显示欢迎信息）
- [ ] 验证：能打开/关闭 pane，sidebar 折叠，双 pane resize 流畅

### Phase 3：API 层 + TanStack Query 接入 ⬜
- [ ] `api/client.js`：apiFetch（带 baseUrl + error 处理）
- [ ] `api/conversations.js`：useConversations / useConversation / useConversationMessages / useSendMessage
- [ ] `api/config.js`：useApiKeys / useModelConfigs
- [ ] Sidebar 接真实 conversations 数据
- [ ] Dashboard 接真实 flowruns 数据（running/waiting/failed）
- [ ] `store/settings.js` → ConfigPane 外观 tab 可用
- [ ] 验证：sidebar 显示真实对话列表，数据与 curl 一致

### Phase 4：SSE 三流 ⬜
- [ ] `store/chat.js`：message/block 树 + 5 个 action
- [ ] `sse/useEventLog.js`：完整实现（连接/断线重连/410 处理/事件 dispatch）
- [ ] `sse/useNotifications.js`：完整实现 + TanStack Query invalidation
- [ ] `sse/useForge.js`：完整实现 + forgeProgress store
- [ ] SSE 状态点显示在 sidebar footer
- [ ] 验证：向后端发消息，能看到 SSE 事件在 DevTools 推进来

### Phase 5：Chat Pane ⬜
- [ ] `BlockRenderer.jsx`：7 种 block type 全部实现
  - TextBlock（inline render + EntityLink + streaming caret）
  - ReasoningBlock（collapsible）
  - ToolCallBlock（collapsible + Arguments + Progress + Result）
  - SubagentBlock（collapsible 嵌套 BlockList）
  - CompactionBlock
- [ ] `MessageView.jsx`：meta 行 + BlockList + attachments
- [ ] `ChatHeader.jsx`：标题 + model-tag + EntityRelMeta（基础版）
- [ ] `ChatPane.jsx`：REST 历史 + SSE 实时合并 + auto-scroll
- [ ] `Composer.jsx`：textarea auto-resize + SlashPopover + AtMentionPopover + send/stop
- [ ] `components/shared/EntityLink.jsx`：实体 ID 点击跳转
- [ ] `components/shared/EntityRelMeta.jsx`：基础版（调 relation API）
- [ ] NoApiKeyGate
- [ ] 验证：完整对话流程（发消息 → SSE 推 → 实时渲染 → 完成显示全部 block）

### Phase 6：Forge Pane ⬜
- [ ] `ForgeList.jsx`：三域并发拉取 + 合并 + tabs + 搜索 + ActionMenu
- [ ] `api/forge.js`：useFunctions / useHandlers / useWorkflows + mutation hooks
- [ ] `FunctionDetail.jsx`：FullView + DiffView + CodeView + SplitDiff + VersionRail
- [ ] `HandlerDetail.jsx`：FullView（class/config/calls tabs）+ DiffView
- [ ] `WorkflowDetail.jsx`：Editor 画布（pan/zoom/drag/connect）+ DiffView + VersionRail
- [ ] `components/shared/VersionRail.jsx`：可折叠 + pending banner + Accept/Revert
- [ ] `AskAiTrigger.jsx`：实现（调 :iterate endpoint）
- [ ] forge SSE 进度显示（list 行 + detail 头部）
- [ ] 验证：完整的 Function/Handler/Workflow 查看、diff、Accept 流程

### Phase 7：Execute Pane ⬜
- [ ] `FlowRunList.jsx`：KpiStrip + WorkflowHeatmap + FlowRunsTable + ApprovalsQueue + TriggersGrid
- [ ] `api/flowruns.js`：useFlowRuns / useFlowRun / useFlowRunNodes
- [ ] `FlowRunDetail.jsx`：DAG + NodeInspector + GanttTimeline
- [ ] `TriagePanel.jsx`：触发 :triage → 打开 chat
- [ ] `RunDiffPanel.jsx`：两次 run 节点状态对比表格
- [ ] notifications SSE 驱动 flowrun 状态实时更新
- [ ] ApprovalBanner（sticky bottom，Framer Motion）
- [ ] 验证：能看到 flowrun 列表、DAG 图、节点详情、批准流程

### Phase 8：Overlay 系统 ⬜
- [ ] `CommandPalette.jsx`：Framer Motion + 实时搜索 + 键盘导航
- [ ] `NotificationsDrawer.jsx`：Framer Motion slide-in + 分桶 + 点击跳转
- [ ] `AskUserModal.jsx`：Framer Motion + 选项 + 提交答复 API
- [ ] `ToastTray.jsx`：Framer Motion layout 动画
- [ ] `SettingsPopover.jsx`：主题/accent/密度快捷设置
- [ ] `Shell` 全局 API（openPane / openEntity / toast / openConv）
- [ ] 验证：⌘K / 通知 / ask modal / toast 全部正常

### Phase 9：Config Pane + Library Panes ⬜
- [ ] `ConfigPane.jsx`：API Keys / Model / Sandbox / 外观 / 数据 tabs
- [ ] 添加 API Key Drawer + 测试流程
- [ ] `SkillsPane.jsx`：列表 + 详情 Drawer
- [ ] `McpPane.jsx`：服务器列表 + 状态 + 健康历史
- [ ] `MemoryPane.jsx`：4 类型 tabs + pin + 编辑 Drawer
- [ ] `DocumentsPane.jsx`：empty state（Phase 5 能力未开发，展示占位）
- [ ] 验证：Config 完整可用，Skill/MCP/Memory 增删改查

### Phase 10：Dashboard ⬜
- [ ] `Dashboard.jsx`：真实数据（flowruns + conversations）
- [ ] KPI 数字动画（Framer Motion count up）
- [ ] 批准/失败卡片实际操作按钮接线
- [ ] 验证：Dashboard 显示真实状态，批准 flowrun 后实时刷新

### Phase 11：打磨 + Wails 打包 ⬜
- [ ] Framer Motion 全面审查：所有 pane 进出、list item 进出、overlay 动效一致
- [ ] 键盘可访问性：所有 modal 可 Esc 关闭，focus trap 正确
- [ ] 空状态：所有列表的 empty state（图标 + 提示文字 + 行动按钮）
- [ ] 错误状态：网络错误 / API 错误 toast 提示
- [ ] 加载状态：skeleton screen（重要列表的首次加载）
- [ ] `cmd/desktop/main.go`：完整 Wails 集成，embed frontend/dist
- [ ] `wails build` 验证产出 .app / .exe
- [ ] 验证：打包后的桌面 app 功能完整

---

## §16 已知 Boilerplate Bug / 差异

本节记录 boilerplate 中明确有问题或未实现的部分，实现时不要直接复制。

| 项目 | boilerplate 状态 | 正确实现 |
|---|---|---|
| `EntityRelMeta` | 引用但未实现 | §9.7 规格 |
| `AskAiTrigger` | 引用但未实现 | §10.3 规格 |
| `ObserveView` | 完全是 stub | Phase 5 能力，先显示 empty state |
| `MemoryView` | 78 行基础骨架 | §13.3 完整规格 |
| `relTime()` | 非组件函数，到处 copy | 统一为 `RelTime` 组件，`src/components/shared/` |
| 全局 `window.Xxx` | 模块化解决 | ES module export/import |
| `useState: useXxxState` | 模块化解决 | 直接 `import { useState } from "react"` |
| `Forgify.xxx` mock data | 模块化解决 | TanStack Query 替代 |
| pane resize 不持久 | 每次重置为 50% | `leftPct` 写入 localStorage |
| `mentionPool()` 用 mock 数据 | | 接 useForgeList + useSkills API |
| Slash popover 的 `onMouseEnter` 注释掉了 | 缺 hover 更新 idx | 补全 onMouseEnter |
| ChatListItem ConvMore 用 window.addEventListener | 有内存泄漏风险 | 用 useRef + clickOutside hook |
| WorkflowEditor Space+drag 不支持 | 点空白 = pan，误操作多 | 改为 Space+drag 或 middle click |
| CodeView 高亮 regex 有 quote 误匹配 | `'` `"` 匹配不精确 | 用 state machine 代替 split regex |
| block_delta append 性能 | 每个 delta 都 re-render 整个 BlockList | useMemo + key-based partial update |
| WorkflowEditor Palette 过宽 | `.wf-palette` 没有固定宽度，在某些窗口尺寸下会压缩或遮挡右侧 VersionRail | 自己到时候看看 |
| `.chat-title-row .chat-title-text` 块被一个孤立 `}` 切成两段 | boilerplate styles.css L520-532：第一段以 `overflow:` 结尾就关闭，第二段全是悬空孤儿属性 + 不配对 `}`，esbuild 报 css-syntax-error | 删多余 `}` + 删第二段重复 `overflow:` 行，其余属性并入 `.chat-title-text`。Phase 1 已修。 |

---

## §17 API Endpoint 映射（前端视角）

前端实际使用的 API endpoints（Phase 0-4 已实现的）：

```
# Conversation
GET    /conversations                       → useConversations
POST   /conversations                       → useCreateConversation
GET    /conversations/{id}                  → useConversation
PATCH  /conversations/{id}                  → useUpdateConversation
DELETE /conversations/{id}                  → useDeleteConversation
GET    /conversations/{id}/messages         → useConversationMessages
POST   /conversations/{id}/messages         → useSendMessage
GET    /conversations/{id}/eventlog?from=N  → (410 重连时调用)

# API Keys
GET    /api-keys                            → useApiKeys
POST   /api-keys                            → useCreateApiKey
PATCH  /api-keys/{id}                       → useUpdateApiKey
DELETE /api-keys/{id}                       → useDeleteApiKey
POST   /api-keys/{id}:test                  → useTestApiKey
GET    /providers                           → useProviders

# Model Configs
GET    /model-configs                       → useModelConfigs
PUT    /model-configs/{scenario}            → useUpsertModelConfig

# Functions
GET    /functions                           → useFunctions
POST   /functions                           → useCreateFunction
GET    /functions/{id}                      → useFunction
PATCH  /functions/{id}                      → useUpdateFunction
DELETE /functions/{id}                      → useDeleteFunction
GET    /functions/{id}/versions             → useFunctionVersions
POST   /functions/{id}:accept               → useAcceptFunction
POST   /functions/{id}:revert              → useRevertFunction
POST   /functions/{id}:run                  → useRunFunction

# Handlers
GET    /handlers                            → useHandlers
POST   /handlers                            → useCreateHandler
GET    /handlers/{id}                       → useHandler
GET    /handlers/{id}/versions              → useHandlerVersions
POST   /handlers/{id}:accept               → useAcceptHandler
POST   /handlers/{id}:call                  → useCallHandler
GET    /handlers/{id}/config               → useHandlerConfig
PUT    /handlers/{id}/config               → useUpdateHandlerConfig

# Workflows
GET    /workflows                           → useWorkflows
POST   /workflows                           → useCreateWorkflow
GET    /workflows/{id}                      → useWorkflow
PATCH  /workflows/{id}                      → useUpdateWorkflow
GET    /workflows/{id}/versions             → useWorkflowVersions
POST   /workflows/{id}:accept              → useAcceptWorkflow
POST   /workflows/{id}:deploy              → useDeployWorkflow

# FlowRuns
GET    /flowruns                            → useFlowRuns
GET    /flowruns/{id}                       → useFlowRun
GET    /flowruns/{id}/nodes                 → useFlowRunNodes
POST   /flowruns/{id}/nodes/{nodeId}:approve  → useApproveNode
POST   /flowruns/{id}/nodes/{nodeId}:reject   → useRejectNode
POST   /flowruns/{id}:cancel               → useCancelFlowRun

# SSE
GET    /eventlog                            → useEventLog SSE hook
GET    /notifications                       → useNotifications SSE hook
GET    /forge                              → useForge SSE hook

# Skills
GET    /skills                              → useSkills
GET    /skills/{id}                         → useSkill
POST   /skills/{id}:activate               → useActivateSkill

# MCP
GET    /mcp-servers                         → useMcpServers
POST   /mcp-servers                         → useAddMcpServer
POST   /mcp-servers/{id}:reconnect         → useReconnectMcpServer
DELETE /mcp-servers/{id}                    → useRemoveMcpServer

# Memory
GET    /memories                            → useMemories
POST   /memories                            → useCreateMemory
PUT    /memories/{name}                     → useUpdateMemory
DELETE /memories/{name}                     → useDeleteMemory
PATCH  /memories/{name}:pin                → usePinMemory

# Relations
GET    /relations                           → useRelations (EntityRelMeta)

# Notifications (REST snapshot)
GET    /notifications                       → useNotificationsSnapshot

# Pending Questions (AskUserModal)
POST   /conversations/{id}/pending-questions/{toolCallId}:resolve → useResolveQuestion
```

---

## §18 Boilerplate 视觉权威原则

### 18.1 两份文档的分工

| 问题 | 去哪里找答案 |
|---|---|
| 这个组件的 HTML 结构长什么样？ | boilerplate（如 `chat.jsx`、`forge.jsx`） |
| 这个 class 的 CSS 是什么？ | boilerplate `styles.css` |
| 这个交互背后的数据从哪来？ | 本 PRD |
| 这个动效用什么库、参数是什么？ | 本 PRD |
| 这里应该接哪个 API？ | 本 PRD §17 |
| boilerplate 这里有 bug，怎么修？ | 本 PRD §16 |

**原则：PRD 说"做什么"，boilerplate 说"长什么样"。二者都沉默的地方，以 boilerplate 为准。**

### 18.1.5 遇到 boilerplate 视觉 bug 时的处理原则

Boilerplate 是设计意图的参考，不是像素级的复制目标。当你实现某个组件时，如果发现 boilerplate 里有**明显的布局 bug**（元素重叠/遮挡、内容被截断、交互无法完成），按以下方式处理：

**判断标准——这是 bug 还是风格？**
- Bug（修）：元素遮挡导致内容不可见、拖拽/点击无法触达目标区域、宽度/高度导致 overflow 截断核心内容
- 风格（保留）：间距稍大或稍小、颜色稍浅或稍深、字体 weight 差一档

**修的方式：最小干预。**
- 只改导致 bug 的那一条 CSS 属性（如 width → 固定值、添加 flex-shrink: 0）
- 不借机重构整个布局
- 修完后在 §16 补一行记录（即使 §16 里没有提前列出）

**不确定时的处理：**
在实现 session 里描述一下你看到的现象（"Palette 的宽度在某些窗口下会压缩 canvas"），然后给出最小修法，等用户确认再继续。不要默默修掉，也不要因为 boilerplate 这样写就直接复制。

**§16 的角色：** §16 是已知 bug 的预先登记。**没在 §16 里的不代表没有 bug**——实现中遇到新 bug 时，先记到 §16，再修。

### 18.2 必须原封不动保留的 boilerplate 决策

以下内容在实现时**直接移植**，不改设计、不改 class 名、不改 CSS 值：

**CSS 变量体系（`styles.css` 前 ~180 行）：**
- 所有 `--t-fast / --t-med / --t-slow` 动效变量
- 所有 `--bg-*` / `--fg-*` / `--border-*` / `--accent-*` 色值
- 所有 `--shadow-*` / `--radius-*` / `--gap-*` / `--row-h` / `--pad-*` 间距
- `[data-theme]` / `[data-density]` / `[data-accent]` 三套 override
- `--font-sans` / `--font-mono` 字体栈（Inter + Noto Sans SC + JetBrains Mono）

**布局 class（`.app` → `.sidebar` / `.main` → `.pane`）：** grid 结构、尺寸，原封不动。

**组件 class 命名规范：** `.page` / `.page-header` / `.page-tabs` / `.page-toolbar` / `.page-body` / `.page-tab.is-active` / `.kpi` / `.hm-row` / `.fr-dag` 等，全部保留原名。

**表格样式（`.t` class）：** 列宽、hover 行、选中行（`.is-selected`）、`.cell-flex` / `.cell-strong` / `.cell-mono`。

**badge 变体（`.badge.success/.error/.warn/.info/.muted/.streaming`）：** 含 pulse-dot 动画，全部保留。

**diff 视图样式（`.diff` / `.split-diff` / `.sd-row.sd-add/.sd-del`）：** 绿/红高亮色，原封不动。

**workflow canvas（`.wf-canvas` / `.wf-node` / `.wf-node-handle` / `.wf-edges`）：** node 尺寸（184×76px）、handle 位置（4 个方向）、edge cubic bezier 路径算法。

**version rail（`.vr-rail` / `.vr-row` / `.vr-badge.*`）：** 颜色语义（current=green / pending=warn / deployed=accent）。

### 18.3 实现时的参考流程

每写一个新组件，执行以下步骤：

1. **找到 boilerplate 里对应的 `.jsx` 文件**，读清楚 HTML 结构和 class 名
2. **找到 `styles.css` 里对应的规则**，确认样式细节（不要猜）
3. **按本 PRD 的组件规格**替换数据来源（mock → TanStack Query / SSE store）
4. **只改 §16 列出的 bug**，其余照搬
5. **Framer Motion 动效**按 §3.2 的参数表加，不改现有 CSS transition

### 18.4 boilerplate 中已经做对的部分（不要改动）

下列设计决策在 boilerplate 里实现得很好，**不要在"重构"过程中改掉**：

- **信息密度：** `--row-h: 32px`（cozy 密度），nav-item、表格行的紧凑程度。不要因为"留白更好看"就加 padding。
- **单一 accent 原则：** 整个 UI 只有一个 accent 色（默认 claude-orange）。不要因为"想区分状态"就乱用 accent。
- **msg-actions 默认隐藏：** 只在 hover 时显示，保持消息列表的干净。
- **streaming badge：** `.badge.streaming` 有 pulse dot，传达 agent 在工作的状态感。
- **pane-bar 面包屑：** 每个 pane 顶部的 icon + breadcrumb，给用户定位感。chat pane 例外（自带 header 更节约空间）。
- **version rail 默认展开：** pending 版本存在时，rail 顶部有 banner 提示，不会让用户错过。
- **工具调用默认折叠：** `defaultOpenTools=false`，消息流保持可读，需要时才展开细节。
- **reasoning block 默认折叠：** 同上，避免推理内容占满屏幕。
- **day-divider：** 对话流里的日期分隔线，视觉呼吸点。
- **conv.status dot：** 对话列表里左侧的小点（streaming 时脉动，approval 时 warn 色），低成本传递状态。
- **sidebar footer 的运行状态 strip：** 背景有 workflow 在跑时的"▶ N · ⏸ M"提示，不打断当前工作但不漏信息。

### 18.5 写每一个 pane 之前必读的 boilerplate 文件

| Pane | 必读文件 |
|---|---|
| App Shell + Sidebar | `app.jsx`、`sidebar.jsx` |
| Chat | `chat.jsx`、`blocks.jsx` |
| Forge（列表）| `forge.jsx` |
| Function 详情 | `function-detail.jsx`、`version-rail.jsx` |
| Handler 详情 | `handler-detail.jsx`、`version-rail.jsx` |
| Workflow 详情 | `workflow.jsx`、`version-rail.jsx` |
| Execute | `execute.jsx` |
| Dashboard | `dashboard.jsx` |
| Config | `config.jsx` |
| Skills / MCP / Memory | `skills.jsx`、`mcp.jsx`、`memory.jsx` |
| 所有 Overlay | `overlays.jsx` |
| 所有 CSS | `styles.css`（全文） |

---

*最后更新：2026-05-20*
*状态：Phase 0-11 均未开始（⬜）*
