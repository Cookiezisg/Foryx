# Testend Dev Console — 设计文档

**关联**：[`backend-design.md`](../../backend-design.md) / [`progress-record.md`](../../progress-record.md) / [`testend-rewrite/testend-rewrite-backend-issues.md`](./testend-rewrite/testend-rewrite-backend-issues.md)

---

## V3 当前形态（2026-05-27 React 重写完工）

V3 是 React 18.3 + TypeScript strict + TanStack Query v5 + Zustand v5 + Vite 6 + react-router-dom 6 hash 的多 view SPA，取代了 V2 的 Vue 3 版本。核心驱动：通过 vite path alias 共享 frontend entity TS 类型，从根本上消除 2 周一次的 type drift；同时彻底清理后端 dev 设施（删除已废弃的 3 个 handler + 2 个字段 + 1 个 flag）。

### 布局

4 列固定栏 + 顶栏（延续 V2 设计）：

```
┌─────────────────────────────────────────────────────────────────────────┐
│ TopBar: build · port · git · 3 SSE pills (EL/NF/FG) · ⌘K · expand    │
├──────────────┬─────────────────────┬──────────────┬─────────────────────┤
│ Col1 conv    │ Col2 chat panel     │ Col3 tab nav │ Col4 tab content    │
│ list         │ ─────────────       │ 6 sections × │ <Route />           │
│ + filter     │ messages + blocks   │ 44 items     │ (44 view 路由)      │
│ + new btn    │ recursive BlockView │ collapsible  │                     │
│ + context    │ Composer (drag drop │              │                     │
│   menu       │ + Enter to send)    │              │                     │
└──────────────┴─────────────────────┴──────────────┴─────────────────────┘
```

三栏宽度均可拖拽（ResizableSplit），localStorage 持久化；⌘K 切换 CommandPalette；expand 模式把 col1/3 折叠成 40px 图标栏让 col4 内容铺开。

### 技术栈

- React 18.3.1（严格 pin 与 frontend 同版本，避免 Multiple React instances 错误）
- TypeScript strict（`strict: true`）
- TanStack Query v5（`@tanstack/react-query`）
- Zustand v5（7 个 store）
- Vite 6 + `@frontend` path alias（→ `../frontend/src`）
- react-router-dom 6 hash mode（`/dev/#/...`，不与后端 SPA fallback 冲突）
- reactflow（WorkflowDetail DAG 渲染，lazy chunk）
- monaco-editor（MonacoEditor 组件，lazy chunk）
- lucide-react（图标，pin 与 frontend 同版本）
- vanilla CSS + CSS 变量（`--bg-0..3` / `--fg-1..3` / `--accent` / `--status-*`），自动 dark/light

### Type 共享机制

```
testend/vite.config.ts
  resolve.alias["@frontend"] → "../frontend/src"

testend 源码中:
  import type { Conversation } from "@frontend/entities/conversation/model/types"
  import type { Block } from "@frontend/entities/conversation/model/types"
```

**type-only 深引**（直接 import `model/types`，不经 `index.ts` barrel）——避免把 React hook 运行时拉入 testend bundle，同时自动跟随后端 §S15 ID 前缀定义。

### 44 View 清单（6 section）

#### dev（8 view）
SQL · Info · Routes · BackendLogs · Processes · Metrics · Errors · Prompts

#### current（9 view）
EventlogRaw · WireTrace · Notifications · SubAgents · ToolCalls · Todos · AsksPending · Attachments · Compaction

#### config（10 view）
ApiKeys · ModelConfigs · Skills · MCPServers · Sandbox · Memory · Documents · Permissions · LLMHealth · Profile

#### forge（7 view）
Functions · FunctionDetail · Handlers · HandlerDetail · Workflows · WorkflowDetail（reactflow） · ToolsRegistry

#### execute（5 view）
Triggers · FlowRuns · FlowRunDetail · ApprovalsQueue · Executions

#### observe（5 view）
LiveSSE · NotificationHistory · Catalog · Usage · MockLLM

**已删除**：TestCollections（V2 时期 YAML 集合执行器；后端 `/dev/collections` + `--collections-dir` 同步删除）。

### 目录结构（V3）

```
testend/
├── package.json / vite.config.ts / tsconfig.json / CLAUDE.md
├── src/
│   ├── main.tsx / App.tsx / router.tsx / style.css
│   ├── api/
│   │   ├── devClient.ts         ← request<T>/envelope unwrap/pagination
│   │   ├── sse.ts               ← 3 stream fan-out subscribe()
│   │   ├── logs.ts / sql.ts / mockllm.ts / trace.ts / info.ts / routes.ts
│   ├── stores/                  ← Zustand
│   │   ├── users / ui / conv / chat / notifications / forge / catalog
│   ├── hooks/
│   │   ├── queryKeys.ts         ← TanStack Query key factory
│   │   └── useNormalizedBlock.ts ← attrs parseMaybeJSON workaround (#4)
│   ├── components/
│   │   ├── layout/              ← TopBar / UserPicker / ConvSidebar / ChatPanel / TabNav / ResizableSplit
│   │   └── ui/                  ← RawJsonModal / ToastTray / EmptyView / RelTime / KindChip /
│   │                               StatusBadge / Pill / Kbd / BlockView / MonacoEditor / CommandPalette
│   └── views/
│       ├── dev/                 ← 8 view
│       ├── current/             ← 9 view
│       ├── config/              ← 10 view
│       ├── forge/               ← 7 view
│       ├── execute/             ← 5 view
│       └── observe/             ← 5 view
└── dist/                        ← Vite 产物，后端 --testend-dir 指向此目录
```

### SSE 订阅模式

`api/sse.ts` 提供 3 个 channel 的 fan-out `subscribe()`，`app/sse/SSEProvider.tsx`（对应 frontend 的 SSEProvider 同名组件）在 mount 时启动 eventlog / notifications / forge 三条流，所有 view 共享同一个 `EventSource`。

- `eventlog`：5 events × 7 block types（text/reasoning/tool_call/tool_result/progress/message/compaction）
- `notifications`：entity 状态变更，开放词表
- `forge`：4 events × 3 kinds（function/handler/workflow），封闭枚举

### 7 个 Zustand Store

| Store | 职责 |
|---|---|
| `users` | 用户列表 + 当前用户 |
| `ui` | col widths / expanded / toasts / palette / rawJsonModal |
| `conv` | 对话列表 + 选中 conv |
| `chat` | 消息树 + block 状态（SSE 实时更新） |
| `notifications` | 通知队列 |
| `forge` | 锻造流事件（function/handler/workflow 进度） |
| `catalog` | tool catalog |

### 后端 Dev 设施（V3 清理后现状）

仅 `--dev` 模式挂载。端点不走 errmap，直接返回 JSON / SSE。

| Method | Path | 用途 |
|---|---|---|
| GET | `/dev/logs` | 后端日志 SSE 流（ring buffer 回放 + 持续推送） |
| POST | `/dev/sql` | 只读 SQL 查询（`SELECT` 前缀强制） |
| GET | `/dev/info` | 实例信息（build / port / git / uptime） |
| GET | `/dev/runtime` | Go 运行时指标（goroutines / heap / GC） |
| GET | `/dev/forgify-home` | `~/.forgify/` 目录快照 |
| GET | `/dev/bash-processes` | 后端活跃 bash shell 进程列表 |
| GET | `/dev/mock-llm` | mock LLM 状态 + 请求历史 |
| GET | `/dev/llm/trace` | LLM 调用 trace（请求/响应 payload 列表） |
| GET | `/dev/routes` | **reflection-based via `router.Recorder`**；自动收录 HandleFunc/Handle 调用；drift-proof（2026-05-27 根治） |
| GET | `/api/v1/dev/prompts` | prompt 总览（见 api-design.md） |

**V3 清理记录（2026-05-27 commits bdf9dd3–f96685d）**：
- 删除：`/dev/collections` handler（YAML 集合列举）+ `--collections-dir` flag + `Deps.CollectionsDir` 字段
- 删除：`/dev/tools` handler（system tool 列表）+ `/dev/invoke` handler（直接调 tool）+ `DevHandler.tools` 字段 + `Deps.Tools []agentapp.Tool` 字段
- 删除：`ServeIndex` 中 `tester.html` fallback（现仅读 `index.html`；V2 tester.html 兼容代码随 V3 迁移一并清理）
- 重命名：`--integration-dir` → `--testend-dir`（语义更准）
- 新增：`router.Recorder` 包装 `*http.ServeMux`，HandleFunc/Handle 调底层 + 记录 Route；`/dev/routes` 读 `Recorder.List()`

### Build 流

```bash
cd testend && npm run build    # 产物 → testend/dist/
```

后端启动：

```bash
go run ./backend/cmd/server \
  --dev \
  --port 8742 \
  --data-dir /tmp/forgify-dev \
  --testend-dir testend/dist
```

Makefile target `make testend` 封装以上流程（先检查 dist/ 是否存在），浏览器自动打开 `http://localhost:8742/dev/`。

开发期热重载：`cd testend && npm run dev`（Vite dev server 5174；vite proxy 已配 `/api` 和 `/dev` → 后端 8742）。

### Verification（每次 commit 前）

| 层 | 命令 | 通过条件 |
|---|---|---|
| 静态 | `cd testend && npm run typecheck` | 0 error |
| 静态 | `cd testend && npm run build` | 0 error / 0 warning |
| 动态 | `make testend` + 浏览器开修改的 view | 无 console error，数据/UI 正确 |

### 约定

- 所有 API 通过 `request<T>` 的 envelope 解包，不绕过去裸 `fetch`
- 所有 toast 走 `ui` store，无 `alert()`
- 所有 raw JSON 调试走 `ui` store `showRaw(title, obj)`
- 不走 i18n（dev tool，开发者用，中英混排即可）
- 不写单元测试（门禁是 typecheck + build + 浏览器 smoke）
- 错误展示原始 `error.code` + `error.message`，不走 errorMap（debug 视角需要看原码）

---

## V2 历史（2026-05-14, 已淘汰）

V2 是 Vue 3 + Vite + TypeScript + Pinia + vue-router(hash) 的多 view SPA，取代了 V1 单文件 Alpine.js 控制台。Plan 04-06 后端 trinity 重构后，V1 几乎完全失效；V2 全面对齐当时 90+ HTTP 路由 + 3 SSE 流的现实形态。V2 于 2026-05-27 被 V3 React 版本取代（Vue 源码 + package.json + vite.config.ts + tsconfig.json 全部删除）。

**布局**：4 列固定栏 + 顶栏（V3 继承）
```
┌─────────────────────────────────────────────────────────────────────────┐
│ TopBar: build · port · git · 3 SSE pills (EL/NF/FG) · ⌘K · expand    │
├──────────────┬─────────────────────┬──────────────┬─────────────────────┤
│ Col1 conv    │ Col2 chat panel     │ Col3 tab nav │ Col4 tab content    │
│ list         │                     │ 6 sections × │ <RouterView />      │
│ + filter     │ messages + blocks   │ ~32 items    │ (33 个 view 路由)   │
│ + new btn    │ recursive BlockView │ collapsible  │                     │
│ + context    │ Composer            │              │                     │
│   menu       │                     │              │                     │
└──────────────┴─────────────────────┴──────────────┴─────────────────────┘
```

**技术栈（V2）**：Vue 3.5 + `<script setup>` / Pinia / vue-router 4 hash / vanilla CSS / cytoscape.js 3.30（WorkflowDetail） / vue-tsc

**33 view 路由**（V2）：
- current（8）：WireTrace · EventlogRaw · Notifications · SubAgents · ToolCalls · Todos · AsksPending · Attachments
- forge（8）：Functions+Detail · Handlers+Detail · Workflows+Detail · ToolsRegistry · TestCollections
- execute（5）：Triggers · FlowRuns+Detail · ApprovalsQueue · Executions
- observe（4）：LiveSSE · NotificationHistory · Catalog · MockLLM
- config（5）：ApiKeys · ModelConfigs · Skills · MCPServers · Sandbox
- dev（7）：SQL · Info · Routes · BackendLogs · Processes · Metrics · Errors

**目录（V2，已删）**：
```
testend/
├── package.json / vite.config.ts / tsconfig.json
├── src/
│   ├── main.ts / App.vue / router.ts / style.css
│   ├── types/{api,domain}.ts            ← 镜像 backend Go struct（drift 根因）
│   ├── api/                             ← domain client 文件
│   ├── stores/                          ← Pinia（ui/conv/chat/notifications/forge/catalog）
│   ├── components/
│   └── views/
└── dist/
```

**V2 已知问题**（见 testend-rewrite-backend-issues.md）：
- #1 ServeIndex 硬编码 tester.html（V2 修补为双 fallback，V3 彻底删 fallback）
- #2 in-memory DB 多连接 → 空 schema（已修 MaxOpenConns=1）
- #3 dev_routes.go 手维护清单 drift（V3 根治：router.Recorder）
- #4 Block.Attrs REST/SSE 双形态（V3 客户端 workaround：useNormalizedBlock；后端长期 fix 待 chat domain 重写）

**V2 收尾总结（2026-05-14）**：33 view 路由、6 nav section、4 列布局、3 SSE 流、`npm run typecheck` + build 通过、产物 151 KB（gzip 55 KB）+ WorkflowDetail 450 KB（含 cytoscape）。

---

## V1 历史（2025-Q1, 已淘汰）

> 以下章节描述的 V1 testend（单文件 Alpine.js 控制台）在 V2 重写后已废弃。V1 文件（`testend/tester.html` / `testend/js/*` / `testend/style.css`）已删除；V1 collections（`testend/collections/`）随 V3 清理一并移除（后端 `/dev/collections` + `--collections-dir` 同步删）。

V1 目录结构（已删）：

```
testend/
├── tester.html                Dev Console UI（单文件，Alpine.js CDN，无构建）
├── style.css
├── js/
│   ├── app.js                 顶层 Alpine root + 全局状态
│   ├── chat.js                聊天区（消息渲染 + 流式 token 累积）
│   ├── sidebar.js             对话列表
│   ├── drag.js                左右栏拖拽分割
│   ├── tab-config.js          Config tab：API Keys / Model 配置
│   ├── tab-sse.js             SSE tab：Stream / Raw 双视图
│   ├── tab-logs.js            Logs tab：后端日志 EventSource
│   ├── tab-sql.js             SQL tab：只读查询
│   ├── tab-tests.js           Tests tab：YAML 集合执行（前端跑）
│   └── tab-tools.js           Tools tab：System / User Tools 调用
└── collections/               测试集合配置文件（YAML）
    ├── 01-infra-setup.yaml
    ├── 02-conversation-lifecycle.yaml
    ├── 03-chat-messages.yaml
    └── ...（12 个集合）
```

V1 后端新增（`--dev` 模式）：

```
backend/internal/
├── infra/logger/
│   └── broadcast.go           LogBroadcaster（zapcore.Core + SSE 扇出）
└── transport/httpapi/handlers/
    └── dev.go                 /dev/* 路由组的全部 handler
```

V1 时期 `/dev/*` 端点（已部分删除）：`/dev/logs` / `/dev/sql` / `/dev/collections`（已删） / `/dev/tools`（已删） / `/dev/invoke`（已删） / `/dev/static/...` / `/dev/...`（兜底返 tester.html，已删 fallback）。

`testend/` 通过 `--integration-dir` 启动参数（V3 已改名 `--testend-dir`）传入后端。
