# Frontend 架构设计 — TypeScript + Feature-Sliced Design

**创建于**：2026-05-27
**当前进度 / 开发日志**：[`progress-record.md`](./progress-record.md)

**本文档定位**：**前端愿景 + FSD 架构 + Revamp Phase 路线**。**所有代码规范、工程纪律、设计原则、i18n、lint 纪律、boilerplate 守则全部在项目根 [`CLAUDE.md`](../../CLAUDE.md)**——本文档只放"前端长什么样、架构为什么这么设计、怎么走"，不重复规则。

产品需求（UI/交互/数据流/SSE 处理）见 [`frontend-prd.md`](./frontend-prd.md)。

---

## Context — 为什么 Revamp

### 触发事件

起于一个生产 bug：`make clean` 后前端不刷新 → stale `activeUserId` + stale `usersQ` 缓存 → SSE/REST 401 → 多处自愈互相喂 → **401 死循环风暴**。

根因不是某一行，而是**身份自愈散在 5 处**（`App.jsx` 两个 effect + `client.js` + `shared.js` + `boot.js`）互相竞态。深查后确认：这是整个前端缺乏架构纪律的症状。

### 根因诊断（三份深度审计的共识）

- **缺纵向脊柱**：业务逻辑 100% 散在组件 `onClick`/`useEffect`（`Onboarding` 437 行是未抽出的 service；`ChatPane.onSend` 嵌着发送 + 自愈 + toast）。没有后端 `app/service` 那样的用例层。
- **物理范式与后端相反**：后端纵向按 domain 切；前端横向按技术切（`api/` / `store/` / `sse/`）+ `panes/` 纵向。两种范式并存，一条 chat 链劈在 5 个目录。
- **横切关注点散落**：身份自愈 5 处、`pushToast` 71 处、`invalidateQueries` 76 处/13 文件（forge 失效重复 3 份）、`enabled` gate 7/14 不一致、`ui.js` 43 成员 God Store、`client.js::apiFetch` 一函数干 6 件事且反向依赖 store。
- **零边界强制**：前端无任何 lint，全靠自觉（后端有 staticcheck + 别名 + port 三重护栏）。

### 目标

让前端拥有和后端 Go clean arch **对等的低耦合高内聚**：TypeScript 定型 + 完整 Feature-Sliced Design 6 层 + 横切收口 + 机器强制的边界。为长生命周期 / 持续迭代留满空间。

---

## 架构 — FSD 6 层（与后端同构）

### 层定义

依赖**严格自上而下单向**（上层 import 下层；下层永不知上层存在；同层 slice 默认不互引）。

| FSD 层 | 职责 | Forgify 内容 | 后端对位 | 可 import |
|---|---|---|---|---|
| **`app`** | 应用组装：入口、providers、全局 store、SSE 单例、identity、boot、主题 | `App.tsx` / providers / `paneStore` / `overlayStore` / `sidebarStore` / `useSessionBootstrap` / SSEProvider | `transport` 组装 + `main.go` wire | 全部下层 |
| **`pages`** | 完整屏幕（一个 pane = 一个 page） | chat / forge / execute / library / dashboard / observe | HTTP handler 入口（路由式） | widgets / features / entities / shared |
| **`widgets`** | 自包含组合 UI 块（组合多个 feature/entity） | Sidebar / NotificationsDrawer / CommandPalette / RelGraph / VersionRail / AskAiTrigger / EntityRelMeta | 组合层（无直接对位） | features / entities / shared |
| **`features`** | 用户用例 / 交互（带业务价值） | send-message / forge-iterate / forge-review / workflow-edit / onboarding / settings / ask-user / entity-link | `app/service`（用例层） | entities / shared |
| **`entities`** | 单个业务实体（数据 + 模型 + 展示卡） | conversation / function / handler / workflow / flowrun / document / skill / mcp / memory / apikey / relation / session / settings / user / model-config | `domain`（实体层） | shared（+ `@x` cross-import） |
| **`shared`** | 零业务：传输底座、UI kit、工具 | api（httpClient / queryKeys / sse / errorMap）/ bridge / ui / lib / i18n | `infra` + `pkg` | 仅自身 |

### 依赖规则

```
app → pages → widgets → features → entities → shared
```

反向或越级禁止。同层 slice 默认禁互引；entity 间真需共享走 FSD 标准 `@x` 机制（`entities/<x>/@x/<y>.ts` 暴露给邻层的专用片）。

### Slice 内部结构

每个 slice 按 segments 组织，**只建需要的段**：

```
entities/conversation/
├── api/        # TanStack Query hooks（数据访问，对位后端 store）
├── model/      # zustand store + types.ts（实体形状，TS interface）
├── ui/         # 展示卡组件（ConversationCard 等）
└── index.ts    # public API — 外部只 import 此文件，不准深引内部
```

`index.ts` 是 **FSD public API**，等价后端 port"不暴露内部"铁律。

### 后端对位精确说明

| 前端规则 | 后端对应 |
|---|---|
| `pages` 只"读 hook → 渲染 → 调 mutation"，零业务 | S6：handler 只"解 JSON → 调 service → 写 envelope"，业务不进 handler |
| `fetch` 只在 `shared/api/httpClient`，组件禁直接 fetch | S8：SQL 只在 `infra/store/`，业务层禁直接写 SQL |
| slice 经 `index.ts` 暴露契约，TS interface 强制 | port 在 domain 定义，不暴露 entity 内部 |
| `steiger` + `eslint-plugin-boundaries` 越界 CI 红灯 | staticcheck + 别名 + port 三重护栏 |

---

## 横切机制

### DIP 注入（解 shared→上层反向依赖）

`shared` 不可依赖上层（FSD 铁律）。横切关注点用**控制反转**，与后端 domain 定 port / `main.go` wire 同构：

- **userId 注入 header**：`shared/api/httpClient` 暴露 `setUserIdProvider(fn)` 注册点；`app/model/useSessionBootstrap` 启动时注入 `() => sessionStore.getState().currentUserId`。httpClient 调注入 fn 取 userId，**完全不知 session 存在**。
- **401 → 信号**：httpClient / sse 的 401 调注入的 `onAuthFailure()`；app 注入实现触发 `resolve()`，不各自清 store，**没有"清了又从 stale 喂回"的循环**。
- **导航意图**：feature hook 返回导航意图对象；page 执行导航。feature 不 import `app/model`（反向禁），page 从 props 拿编排状态。

### errorMap + 全局 onError

`shared/api/errorMap.ts` 是**后端 `errmap.go` 的前端镜像**：`ApiError.code → { i18nKey, recoveryAction }`。TanStack QueryClient 全局 `onError` 消费它 → toast，消除 71 处手写 `pushToast`。feature hook 抛 `ApiError(code)` 即可，不碰 toast。

### SSE 三流

对位后端 E1（永不超三条）：

| SSE 流 | hook | 归层 | 分发目标 |
|---|---|---|---|
| eventlog | `useEventLog` | `app/sse` | `entities/conversation/model/chatStore`（消息树） |
| notifications | `useNotifications` | `app/sse` | entity store invalidate |
| forge | `useForge` | `app/sse` | `entities/function\|handler\|workflow` 的 forge progress state |

SSEProvider 是单例，挂在 `app/providers`。三流通过 `app/sse` 分发到各 entity store，不反向依赖。

### 其余横切归属

| 关注点 | 归属 | 理由 |
|---|---|---|
| toast 队列 | `shared/ui/toastStore` | 无业务的通知原语，下层 widgets/toaster 渲染合法 |
| 用户偏好（theme/accent/density/lang/reasoningDefault）| `entities/settings/model` | 单例配置实体；下层组件直接 import（顺向） |
| UI 编排（openPanes/activeConv/overlays/sidebar）| `app/model`（paneStore 等）| **只** AppShell 读，pages 收 props，不下放 |
| queryKeys 失效映射 | `shared/api/queryKeys` | 单一"实体→失效集"映射；SSE / mutation 都查它，消除 forge 失效 3 份重复 |
| i18n | `shared/lib/i18n/`（配置）+ `entities/settings`（lang 驱动）| app 层驱动 `i18n.changeLanguage`，不由组件各自调 |

---

## 身份层（§8 最规范形态）

### 为什么独立设计

原 401 风暴的根因是身份自愈散 5 处。修复不是"加 if"而是**建单一真相源 + 唯一 writer**。

### entities/session — 身份唯一真相

`entities/session/model/sessionStore.ts`（zustand + persist）：
- `currentUserId: string | null`（persist localStorage）
- `status: 'loading' | 'onboarding' | 'ready'`

身份是业务概念（谁登录着）→ 归 **entities 层**（不是 shared，不是 app）。上层 features/widgets/pages/app 直接 import 合法（顺向）；下层 shared 不可见。

### resolve()

`entities/session/model/resolve.ts` 基于**刚 fetch 的 fresh `/users`** 定 status：
- `/users` 空 → `onboarding`
- `currentUserId` 在 fresh list → `ready`
- `currentUserId` 不在（或 null）→ users 非空则选 `users[0]` 并 `ready`；否则 `onboarding`

**永远基于 fresh，userId 不可能 stale**——根治原 bug 的机制锚点。

### boot gate

`session.status !== 'ready'` 时 app 渲染 loading / onboarding，**不挂载 AppShell / pages → user-scoped query 根本不发**（组件未挂载）。`enabled` gate 不再散在每个 entity hook。

### 删除的 5 处散落自愈

- `App.jsx` 两个 self-heal / account-switch effect → `entities/session.resolve()`（由 `app/model/useSessionBootstrap` 调）
- `httpClient` 401 清除 + `sse` 401 自愈 → 注入的 `onAuthFailure()` → resolve
- `store/boot.js` valid 判定 → **删除**，boot 直接 = `session.status`

---

## 目录结构（到 slice 级）

```
frontend/src/
├── main.tsx                          # 入口
├── app/                              # ── 第 6 层:组装 ──
│   ├── App.tsx                       # 根(boot=session.status)
│   ├── AppShell.tsx                  # composition root:读 app/model → 渲染 pages 传 props
│   ├── providers/                    # QueryProvider(全局 onError→errorMap→toast)/ SSEProvider / I18nProvider
│   ├── model/
│   │   ├── useSessionBootstrap.ts    # 启动 resolve + 注入 userId provider / onAuthFailure 到 shared/api
│   │   ├── paneStore.ts              # openPanes / activeConv / activeFlowRun / activeDocument / leftPct / focusEntity
│   │   ├── overlayStore.ts           # cmdk / notifs / ask / settings open + pendingAsk
│   │   └── sidebarStore.ts           # collapsed / tools / recent / archived expanded
│   ├── sse/                          # SSEProvider + 3 hook(分发到 entity store)
│   └── index.ts
├── pages/                            # ── 第 5 层:屏幕(= pane) ──
│   ├── chat/ forge/ execute/ library/ dashboard/ observe/
├── widgets/                          # ── 第 4 层:组合块 ──
│   ├── sidebar/ command-palette/ notifications-drawer/ entity-graph/
│   └── version-rail/ ask-ai-trigger/ entity-rel-meta/
├── features/                         # ── 第 3 层:用例 ──
│   ├── send-message/ forge-iterate/ forge-review/ workflow-edit/
│   └── onboarding/ settings/ ask-user/ entity-link/
├── entities/                         # ── 第 2 层:实体 ──
│   ├── session/       # 身份唯一真相(sessionStore + resolve + boot gate)
│   ├── settings/      # 单例配置实体(theme/accent/density/lang/reasoningDefault)
│   ├── conversation/  # chatStore + SSE 消息树 + TanStack hooks
│   ├── function/ handler/ workflow/ flowrun/
│   ├── document/ skill/ mcp/ memory/
│   ├── apikey/ model-config/ relation/ user/
│   └── (各有 api/ model/types.ts ui/ index.ts)
└── shared/                           # ── 第 1 层:基础设施 ──
    ├── api/       # httpClient.ts(setUserIdProvider/onAuthFailure 注册点)
    │              # queryKeys.ts  errorMap.ts  sse.ts
    ├── bridge/    # wails.ts(GetBackendPort binding + apiUrl())
    ├── ui/        # Button Badge Icon Kbd Spinner Select + toastStore.ts
    ├── lib/       # motion.ts(动效 tokens)  i18n/(react-i18next 配置)
    └── model/     # navigation.ts(Navigator 接口注册点)
```

---

## Revamp Phase 路线

**当前状态 / 任务细化** → [`progress-record.md`](./progress-record.md)

| Phase | 主题 | 核心交付 | 状态 |
|---|---|---|---|
| **0** | TS 地基 + 护栏 | `tsconfig.json`（allowJs 宽松）+ vite TS + `eslint-plugin-boundaries` + `steiger`（先量化违规）+ path alias + `make lint-frontend` 接入 | ✅ |
| **1** | `shared/` 层 | bridge / httpClient / sse / primitives / motion / i18n → `shared/*`；定型 `Envelope<T>` / `ApiError` / `errorMap` / `queryKeys`；建 public API | ✅ |
| **2** | `entities/` 层（逐实体，共 12 实体）| 每个 `api/*.js → entities/<x>/api/*.ts`；先定 `model/types.ts`（实体形状）；再定型 api hooks；建 `index.ts`；`store/chat.js → entities/conversation/model/chatStore.ts`；拆 `api/forge.js` | ✅ |
| **3** | `features/` 层（抽用例，共 8 feature）| 把组件里的业务编排（ChatPane.onSend / Onboarding / forge accept/reject / RelGraph 聚合）抽进 `features/*/model` hook；组件变薄 | ✅ |
| **4a** | 身份层落地 + 根治 401 风暴 | `entities/session`（sessionStore + resolve）+ `app/model/useSessionBootstrap`（DIP 注入）+ 删 5 处自愈 + boot gate + SSE 移 `app/sse` | ✅ |
| **4b** | `widgets/` + `pages/` 组件迁移 | 组合块归 widgets；pane 退化成 `pages/*` 薄容器；props 化（pages 不 import app store） | ✅ |
| **5** | 严格化 + 文档同步 | `tsconfig strict: true` 全开；消除 `any` 逃逸（tsc 零错误）；steiger / boundaries 零违规；删旧死代码；**本文档 + CLAUDE.md 前端守则同步** | ✅ |

---

## Verification（前端门禁三段）

### 段 1：类型契约

```bash
npm run typecheck     # tsc --noEmit，strict: true，零错误
```

零 `any` 逃逸；entity 形状 / SSE payload / API envelope 全有类型约束。

### 段 2：架构边界

```bash
npm run lint          # eslint src（eslint-plugin-boundaries 单向规则）
npm run fsd           # steiger src（FSD 官方 linter：层级依赖/public API/cross-import/orphan）
```

6 层 import 方向全机器强制；任何反向或越级导入 CI 红灯。

### 段 3：行为 + 冒烟

```bash
npm test              # vitest run，756 tests 全绿
wails dev             # 窗口起得来 + 能连后端（每 Phase 末尾跑）
```

vitest 是行为不变的安全网；`tsc` 是契约破裂的早期警报；steiger/boundaries 是架构破裂的护栏。

**三段并入 `make lint-frontend`，与后端 `staticcheck` 同等地位——违规 push 不过去。**

---

## 文档分册结构

本文件是**前端架构总览**（愿景 / 架构 / Phase 路线），其余按角色分组：

| 文档 | 用途 | 推进节奏 |
|---|---|---|
| [`../../CLAUDE.md`](../../CLAUDE.md) | 前端代码规范 / 工程纪律 / boilerplate 守则 / i18n / F1 文档同步 — 单一事实源 | 规则演化时改 |
| [`frontend-prd.md`](./frontend-prd.md) | 产品需求：UI/交互/SSE 数据流/API endpoint 映射 | 产品/UX 变更时改 |
| [`frontend-contract-documents/`](./frontend-contract-documents/)（待建）| 前端契约索引：entity 类型总览 / SSE payload 总览 / queryKeys 映射 | 每个实体/SSE 变更时更新 |
| [`frontend-design-documents/`](./frontend-design-documents/)（待建）| 每个复杂 slice 的详设计（对位后端 service-design-documents/）| 复杂 feature/entity 开工前写 |
| [`progress-record.md`](./progress-record.md) | 开发日志 + 当前快照 + 任务清单（前后端统一） | 实时更新 |

**工作流**：
1. **开工前** → 填 `frontend-design-documents/<slice>.md` 详设计（含数据流推演 + 实现清单）
2. **实现中** → 同步更新 `frontend-contract-documents/` 里该 slice 的契约段
3. **完成后** → 在 `progress-record.md` 加 dev log + 勾任务清单

---

## 非目标（前端 Revamp 不做）

- 业务功能变更（纯架构/类型/组织重构，行为不变，vitest 断言行为不变即证）
- UI 视觉/交互变更（像素级保持，boilerplate 定义视觉细节）
- 后端任何改动
- 引入 React Router（无 URL 路由需求，pane 编排在 `app/model`）
- Docker 容器化 / SSR / 多端（本地单用户 Wails 桌面 app，无多端需求）
- 类型生成工具链（手写对齐后端 contract 文档，简单可控）
