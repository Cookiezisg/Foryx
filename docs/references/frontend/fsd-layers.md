---
id: DOC-200
type: reference
status: active
owner: @weilin
created: 2026-05-27
reviewed: 2026-05-31
review-due: 2026-06-30
audience: [human, ai]
---
# FSD 层契约 — 一眼索引

**关联**：
- [`../frontend-design.md`](../frontend-design.md) — 架构愿景 + 设计理由
- [`../frontend-prd.md`](../frontend-prd.md) — 产品需求 / UI 细节
- [`./cross-cutting.md`](./cross-cutting.md) — DIP / errorMap / SSE / queryKeys
- [`./entity-types.md`](./entity-types.md) — 12 entity TS 类型 ↔ 后端端点映射

**定位**：**一眼看到每层是什么、能 import 谁、slice 清单是什么**。架构理由 / 演进历史去 `frontend-design.md`。

---

## 6 层职责表

| FSD 层 | 职责 | 后端对位 | 可 import |
|---|---|---|---|
| **`app`** | 应用组装：入口、providers、全局 store、SSE 单例、identity boot、主题 | `transport` 组装 + `main.go` wire | 全部下层 |
| **`pages`** | 完整屏幕（一个 pane = 一个 page） | HTTP handler 入口（路由式） | widgets / features / entities / shared |
| **`widgets`** | 自包含组合 UI 块（组合多个 feature/entity） | 无直接对位（组合层） | features / entities / shared |
| **`features`** | 用户用例 / 交互（带业务价值） | `app/service`（用例层） | entities / shared |
| **`entities`** | 单个业务实体（数据 + 模型 + 展示卡） | `domain`（实体层） | shared（+ `@x` 跨 slice） |
| **`shared`** | 零业务：传输底座、UI kit、工具函数 | `infra` + `pkg` | 仅自身 |

---

## 依赖规则

```
app → pages → widgets → features → entities → shared
```

- 反向依赖（下层引上层）**严格禁止**。
- 同层 slice 默认**不互引**；entity 间真需共享走 `@x` 机制（见下）。
- 导航意图由 `shared/lib/navigation` DIP 解：widgets/features 调 `navigate.*`，不 import `app`。
- 机器强制：`eslint-plugin-boundaries`（见"§ 机器强制"节）。

### @x 跨 slice 机制

```
entities/<X>/@x/<Y>.ts   →  给相邻 entities/<Y> 使用的专用 public 片
```

当前已有：
- `entities/session/@x/user.ts` — session 向 user slice 暴露 currentUserId 工具
- `entities/conversation/@x/workflow.ts` — conversation 向 workflow slice 暴露 `ModelRef`（含 `ThinkingSpec`）类型，供 WorkflowEditor 的节点 modelOverride 检查器使用（**2026-05-30**）

---

## Slice 清单

### app（单一 slice，内部按 segment 组织）

| segment | 内容 |
|---|---|
| `app/` root | `App.tsx`（root 组件）/ `AppShell.tsx`（主布局）/ `main.tsx`（Vite 入口）/ `index.ts` |
| `app/model/` | `useSessionBootstrap.ts`（boot gate + DIP 注入）/ `paneStore.ts`（pane 编排）/ `overlayStore.ts`（modal/overlay）/ `sidebarStore.ts`（sidebar 展开收起）|
| `app/sse/` | `SSEProvider.tsx`（三流 single mount）/ `useEventLog.ts` / `useForge.ts` / `useNotifications.ts` |
| `app/shell/` | `PaneFrame.tsx` / `PaneResize.tsx` / `NarrowSwitch.tsx`（窄屏适配）|
| `app/lib/` | `useKeyboardShortcuts.ts` |

### pages（6 个 slice）

| slice | 对应 UI pane |
|---|---|
| `pages/chat` | 对话主视图（消息树 + 输入框）|
| `pages/forge` | 锻造视图（function/handler/workflow 编辑 + AI 迭代）|
| `pages/execute` | 执行视图（flowrun 触发 + 列表）|
| `pages/library` | 资产库（文档 / memory / skill / mcp）|
| `pages/dashboard` | 仪表盘（概览）|
| `pages/observe` | 观察视图（flowrun 节点级监控）|

### widgets（10 个 slice）

| slice | 功能 |
|---|---|
| `widgets/sidebar` | 左侧导航 + 对话列表 |
| `widgets/notifications-drawer` | 通知抽屉（展示 SSE notification 历史）|
| `widgets/command-palette` | Cmd+K 命令面板 |
| `widgets/rel-graph` | 实体关系图（D3 force）|
| `widgets/version-rail` | 版本轨道（function/handler/workflow 版本历史）|
| `widgets/ask-ai-trigger` | AI 触发按钮（喂当前上下文给 chat）|
| `widgets/entity-link` | 实体内联引用卡片 |
| `widgets/entity-rel-meta` | 实体关系元信息面板 |
| `widgets/toaster` | Toast 托盘渲染（消费 `shared/ui/toastStore`）|
| `widgets/action-menu` | 上下文动作菜单 |

### features（8 个 slice）

| slice | 用例 |
|---|---|
| `features/send-message` | 发送消息（输入 → POST /conversations/{id}/messages:send）|
| `features/forge-iterate` | 锻造迭代（AI 编辑 function/handler/workflow）|
| `features/forge-review` | 锻造审查（diff 查看 + accept/reject）|
| `features/workflow-edit` | 工作流编辑（节点图 CRUD）；palette 已按 revamp 设计折叠为 **5 种**：trigger / agent / tool（→function） / case（→condition） / approval；旧 14 种仍在 canvas 渲染（向后兼容），仅 palette 新增入口关闭 |
| `features/onboarding` | 首次启动流程（创建 user + 配置 API key）|
| `features/settings` | 用户偏好设置 UI（主题/语言/API key 管理）；**2026-05-30 新增**：`ui/ModelCapOverrideEditor.tsx`（stale-catalog 逃生舱，允许手动覆盖 thinkingShape / contextWindow / maxOutput）；**2026-05-31 新增**：`ui/AdvancedCapabilitiesSection.tsx`（「高级能力」运行上限区，编辑 settings.json `limits` 块）|
| `features/ask-user` | ask_user tool 响应（approval/input 弹窗）|
| `features/entity-link` | 实体链接解析（wikilink → 内联卡片）|

### entities（15 个 slice）

| slice | 核心类型 | 主键 |
|---|---|---|
| `entities/conversation` | `Conversation` / `Message` / `Block` / `ChatStore` | `id`（`cv_` / `msg_` / `blk_` 前缀）|
| `entities/function` | `FunctionEntity` / `FunctionVersion` | `id`（`fn_` / `fnv_` 前缀）|
| `entities/handler` | `Handler` / `HandlerVersion` | `id`（`hd_` / `hdv_` 前缀）|
| `entities/workflow` | `Workflow` / `WorkflowVersion` | `id`（`wf_` / `wfv_` 前缀）|
| `entities/flowrun` | `FlowRun` / `FlowRunNode` | `id`（`fr_` / `frn_` 前缀）|
| `entities/document` | `Document` / `DocTreeNode` | `id`（`doc_` 前缀）|
| `entities/skill` | `Skill` | `name`（主键无 ID 前缀）|
| `entities/mcp` | `McpServer` / `ToolDef` | `name`（主键无 ID 前缀）|
| `entities/memory` | `Memory` | `id`（`mem_` 前缀）|
| `entities/apikey` | `ApiKey` | `id`（`aki_` 前缀）|
| `entities/relation` | `Relation` / `Neighborhood` | `id`（`rel_` 前缀）|
| `entities/session` | `SessionState`（zustand + persist）| `currentUserId`（非 REST entity）|
| `entities/settings` | `SettingsState`（zustand + persist，前端偏好）+ **2026-05-31** `api/limits`（`useLimits`/`useUpdateLimits`）+ `model/limits.ts`（`Limits`）↔ `/settings/limits` | 偏好单例（非 REST）；limits 为 REST 运行上限 |
| `entities/user` | `User` | `id`（`u_` 前缀）|
| `entities/model-config` | `ModelConfig` / `Provider` / `Scenario` / `ThinkingSpec` / `ModelCapability` | `id`（`mc_` 前缀）；**2026-05-30 新增**：`ui/ThinkingControl.tsx`（capability-driven：none/toggle/effort/budget 四态）、`api/useModelCapabilities.ts` + `useSetModelCapabilityOverride.ts` + `useClearModelCapabilityOverride.ts`、`capabilityFor` 辅助函数 |

### shared（按 segment 组织，不按 slice）

| segment | 内容 |
|---|---|
| `shared/api/` | `httpClient.ts` / `authProvider.ts` / `errorMap.ts` / `queryKeys.ts` / `sse.ts` |
| `shared/bridge/` | `wails.ts`（Wails `apiUrl` 适配，生产走 localhost:PORT，Wails 走 `window.go`）|
| `shared/lib/` | `navigation.ts`（Navigator DIP）/ `motion.ts`（动效参数）/ `i18n/`（react-i18next 配置 + 词典）/ `highlight/`（代码高亮）/ `useCollapsible.ts` |
| `shared/model/` | `forgeProgress.ts`（forge SSE 实时投影 zustand）|
| `shared/ui/` | `toastStore.ts` + UI kit（Badge / Button / Icon / Select / StatusBadge / MarkdownView / HighlightedCode / RelTime / KindChip / Spinner / Kbd / BottomSheet / FloatingInspector / PaneCollapseToggle）|

---

## Slice Public API 约定（index.ts barrel）

每个 slice 的 `index.ts` 是**唯一合法 import 入口**。外部代码必须 `import { X } from "@entities/conversation"`，**禁止深引内部**（`@entities/conversation/model/chatStore` 禁）。

违反深引 = 制造隐式耦合，等价后端绕过 port 直接调 `infra/store`。

---

## 机器强制点

### eslint-plugin-boundaries

配置文件：`frontend/eslint.config.js`

| 规则 | 禁止方向 |
|---|---|
| `shared` | 不得 import `entities / features / widgets / pages / app` |
| `entities` | 不得 import `features / widgets / pages / app` |
| `features` | 不得 import `widgets / pages / app` |
| `widgets` | 不得 import `pages / app` |
| `pages` | 不得 import `app` |

**跨 entity `@x` 豁免**：eslint-boundaries 无法感知 `@x` 目录语义，同层 slice 间结构违规由 `steiger` 负责；eslint 守粗粒度单向。

### steiger（细粒度 FSD lint）

负责：同层 slice 禁止互引、`@x` 协议正确使用、slice 内 segment 顺序。

### TypeScript strict

`frontend/tsconfig.json` 开启 `"strict": true`；`tsc --noEmit` 零错误。`@typescript-eslint/no-explicit-any` 保持 `off`——剩余 ~31 处 `any` 全部在 Tiptap / floating-ui 第三方边界（`DocEditor.tsx`、`CodeBlockNode.tsx`、`ActionMenu.tsx`），上游未导出所需类型，带内联注释说明，无业务逃逸。
