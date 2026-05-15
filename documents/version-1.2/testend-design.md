# Testend Dev Console — 设计文档

**关联**：[`backend-design.md`](./backend-design.md) / [`progress-record.md`](./progress-record.md) / [`testend-rewrite-backend-issues.md`](./testend-rewrite-backend-issues.md)

---

## V2 当前形态(2026-05 重写完工)

V2 是 Vue 3 + Vite + TypeScript + Pinia + vue-router(hash) 的多 view SPA, 取代了 v1 单文件 Alpine.js 控制台。Plan 04-06 后端 trinity 重构后,v1 几乎完全失效;V2 全面对齐当前 90+ HTTP 路由 + 3 SSE 流的现实形态。

**布局**:**4 列固定栏 + 顶栏**(用户指定)
```
┌─────────────────────────────────────────────────────────────────────────┐
│ TopBar: build · port · catalog · 3 SSE pills (EL/NF/FG) · ⌘K · expand │
├──────────────┬─────────────────────┬──────────────┬─────────────────────┤
│ Col1 conv    │ Col2 chat panel     │ Col3 tab nav │ Col4 tab content    │
│ list         │ ─────────────       │ 6 sections × │ <RouterView />      │
│ + filter     │ messages + blocks   │ ~32 items    │ (33 个 view 路由)   │
│ + new btn    │ recursive BlockView │ collapsible  │                     │
│ + context    │ Composer (drag drop │              │                     │
│   menu       │ + Enter to send)    │              │                     │
└──────────────┴─────────────────────┴──────────────┴─────────────────────┘
```
三栏宽度均可拖拽(ResizableSplit),lokastorage 持久化;⌘K 切换 CommandPalette;expand 模式把 col1/3 折叠成 40px 图标栏让 col4 内容铺开。

**技术栈**:
- Vue 3.5 + `<script setup>` Composition API
- Pinia stores (UI / conv / chat / notifications / forge / catalog)
- vue-router 4 hash mode (`/dev/#/...`, 单页路由不与后端 SPA fallback 冲突)
- vanilla CSS + CSS 变量(`--bg-0..3` / `--fg-1..3` / `--accent` / `--status-*`),自动 dark/light(`prefers-color-scheme`)
- JetBrains Mono + Inter 字体
- cytoscape.js 3.30 用于 workflow DAG 渲染(只在 WorkflowDetail 路由 lazy chunk)
- 完整 typecheck (`vue-tsc --noEmit`),生产 build 通过 `npm run build`

**目录结构**(v2):
```
testend/
├── package.json / vite.config.ts / tsconfig.json
├── src/
│   ├── main.ts / App.vue / router.ts / style.css
│   ├── types/{api,domain}.ts            ← 镜像 backend Go struct
│   ├── api/                             ← 每个 domain 一个 client 文件
│   │   ├── client.ts                    ← request<T>/envelope unwrap/pagination
│   │   ├── sse.ts                       ← 3 stream fan-out subscribe()
│   │   ├── conversations / functions / handlers / workflows / flowruns
│   │   ├── resources                    ← apikey / model / skill / mcp / sandbox / catalog
│   │   ├── dev                          ← /dev/* endpoints
│   │   └── misc                         ← attachments
│   ├── stores/                          ← Pinia
│   │   ├── ui                           ← col widths / expanded / toasts / palette
│   │   ├── conv / chat / notifications / forge / catalog
│   ├── components/
│   │   ├── layout/                      ← TopBar / ConvSidebar / ChatPanel / TabNav / ResizableSplit
│   │   ├── chat/                        ← MessageView / BlockView(recursive) / Composer / SystemPromptEditor
│   │   ├── forge/                       ← GraphView(cytoscape)
│   │   └── common/                      ← Pill / RawJsonModal / ToastTray / CommandPalette / SubTabBar / ViewHeader / EmptyView
│   ├── views/
│   │   ├── current/    (8 view: WireTrace, EventlogRaw, Notifications, SubAgents, ToolCalls, Todos, AsksPending, Attachments)
│   │   ├── forge/      (8 view: Functions+Detail, Handlers+Detail, Workflows+Detail, ToolsRegistry, TestCollections)
│   │   ├── execute/    (5 view: Triggers, FlowRuns+Detail, ApprovalsQueue, Executions)
│   │   ├── observe/    (4 view: LiveSSE, NotificationHistory, Catalog, MockLLM)
│   │   ├── config/     (5 view: ApiKeys, ModelConfigs, Skills, MCPServers, Sandbox)
│   │   └── dev/        (7 view: SQL, Info, Routes, BackendLogs, Processes, Metrics, Errors)
│   └── utils/format.ts                  ← timeAgo/timestamp/bytes/duration/shortID/pretty/truncate/statusClass
└── dist/                                ← Vite 产物, /dev/static/ 服务
```

**SSE 订阅模式**:`api/sse.ts` 提供 3 个 channel 的 fan-out `subscribe()`,App.vue 在 mount 时启动 eventlog / notifications / forge 三条流,所有 view 共享同一个 `EventSource`(浏览器 per-origin 6 连接预算稀有);单一 store 累积事件,各 view 经 computed 筛选(by convId / type / scope.kind)。`Last-Event-ID` 自动重连由浏览器 EventSource 处理;手动 close+reopen 时接受小 gap(view 主动 REST refetch)。

**约定**:
- 所有 API 通过 `request<T>` 的 envelope 解包,不绕过去裸 `fetch`
- 所有 toast 走 `useUIStore().toast(...)`,无 `alert()`
- 所有 raw JSON 调试走 `useUIStore().showRaw(title, obj)`(顶层 modal)
- 列表分页一律 `getPage<T>(path, query)` cursor 模式

**已知契约**:每个 view 头部 doc 注释列出 source endpoint + 数据依赖的 store。开发新 view 时,先在 `dev/routes` 验证后端真有这个路由,再写 client + view。

---

## V1 历史 (2025-Q1, 已淘汰)

> 以下章节描述的 v1 testend(单文件 Alpine.js 控制台)在 V2 重写后已废弃。v1 文件 (`testend/tester.html` / `testend/js/*` / `testend/style.css`) 已删除;v1 collections (`testend/collections/`) 暂保留作 dev 测试集合参考,可由 v2 的 Forge › Test collections view 加载。

V1 的目录结构(已删):

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
    ├── 04-tool-crud-versions.yaml
    ├── 05-tool-test-cases.yaml
    ├── 06-tool-run-edge-cases.yaml
    ├── 07-tool-import-export.yaml
    ├── 08-error-validation.yaml
    ├── 09-pagination-cursor.yaml
    ├── 10-tool-test-case-with-assertions.yaml
    ├── 11-full-e2e-workflow.yaml
    └── phase2-smoke.yaml
```

后端新增（仅 `--dev` 模式挂载）：

```
backend/internal/
├── infra/logger/
│   └── broadcast.go           LogBroadcaster（zapcore.Core + SSE 扇出）
└── transport/httpapi/handlers/
    └── dev.go                 /dev/* 路由组的全部 handler
```

`testend/` 整个目录通过 `--integration-dir` 启动参数传入后端，由 dev.go 以静态文件形式对外提供。

---

## UI 布局

```
┌─────────────────────────────────────────────────────────────┐
│  Forgify Dev Console                                         │
├───────────────┬────────────────────────┬────────────────────┤
│               │                        │  Config│SSE│Logs│SQL│Tests│Tools │
│  对话列表     │      聊天区            │                              │
│  sidebar      │   (流式渲染)           │  右侧工具面板（6 个 Tab）    │
│               │                        │                              │
│  + 新对话     │  输入框 + Send / Cancel│                              │
└───────────────┴────────────────────────┴────────────────────┘
```

三栏宽度可拖拽（drag.js）。

---

## 后端变更

### 修改文件

| 文件 | 变更 |
|---|---|
| `cmd/server/main.go` | 解析 `--dev` / `--data-dir` / `--collections-dir` / `--integration-dir` 等启动参数；dev 模式构建 LogBroadcaster，接入 zap tee core |
| `infra/logger/zap.go` | `New()` 接受可选额外 `zapcore.Core`（dev 时传 broadcaster） |
| `infra/logger/broadcast.go` | **新增** LogBroadcaster：实现 `zapcore.Core`，作为 tee 的第二路 |
| `transport/httpapi/router/deps.go` | 加 dev-only 字段：`Dev bool`、`DB *gorm.DB`、`LogBroadcaster *LogBroadcaster`、`CollectionsDir`、`IntegrationDir`、`Port`、`Tools []agentapp.Tool` |
| `transport/httpapi/router/router.go` | `if deps.Dev { devHandler.Register(mux) }` |
| `transport/httpapi/handlers/dev.go` | **新增** DevHandler，注册所有 `/dev/*` 端点 |

### LogBroadcaster（`infra/logger/broadcast.go`）

实现 `zapcore.Core`，作为 tee core 的第二路，仅 dev 模式启用。

```
LogBroadcaster
├── ring buffer（最近 N 条，供新订阅者连接时回放）
├── subs []*logSub
└── Write(entry) → 追加 buffer + 非阻塞扇出
```

LogEntry 结构（SSE data payload）：
```json
{
  "time": "2026-04-26T10:00:00Z",
  "level": "info",
  "msg": "chat task done",
  "fields": { "conversation_id": "cv_xxx", "stop_reason": "end_turn" }
}
```

设计与 `infra/events/memory/bridge.go` 对称：RLock 快照 subs → 释放锁 → 发送，慢订阅者非阻塞丢弃。

---

## Dev Endpoints（`handlers/dev.go`）

仅 `--dev` 模式注册。dev 端点不走 errmap，直接返回 JSON / SSE。

| Method | Path | 用途 |
|---|---|---|
| `GET` | `/dev/logs` | 后端日志 SSE 流 |
| `POST` | `/dev/sql` | 只读 SQL 查询 |
| `GET` | `/dev/collections` | 列出可运行的 YAML 测试集合 |
| `GET` | `/dev/tools` | 列出已注册的 system tool |
| `POST` | `/dev/invoke` | 直接调用 system tool（绕过 LLM）|
| `GET` | `/dev/static/...` | testend 静态文件（CSS/JS） |
| `GET` | `/dev/...` | 兜底返回 `tester.html`（SPA-style） |

> **没有** `POST /dev/collections/{name}/run` 端点。测试集合由**前端 JS** (`testend/js/tab-tests.js`) 自己按步骤逐个 fetch 执行，后端只负责提供 YAML 内容。

### `GET /dev/logs` — SSE 日志流
1. 先把 ring buffer 历史全量推一遍（replay）
2. 订阅新条目，持续推送
3. 每 15s 发 `: keep-alive`

### `POST /dev/sql` — SQL 查询
```
Request:  { "sql": "SELECT ..." }
Response: { "columns": ["id", "role", ...], "rows": [["msg_x", "user"], ...] }
        | { "error": "只允许 SELECT 语句" }
```
- 前缀必须是 `SELECT`（`strings.ToUpper(TrimSpace(sql))` 检查）
- `db.Raw(sql).Rows()` + `(*sql.Rows).Columns()` 动态扫描返回

### `GET /dev/collections` — 列出 YAML 集合
扫描 `--collections-dir` 指向的目录中所有 `*.yaml`，返回每个集合的完整内容（含 name / description / steps），由前端直接消费。

### `GET /dev/tools` — 列出已注册 system tool
```
Response: [{"name": "web_search", "desc": "..."}, ...]
```
依赖 `Deps.Tools []agentapp.Tool`（main.go 把全部 system tool 注入）。

### `POST /dev/invoke` — 直接调用 system tool
绕过 LLM agent 直接执行指定 system tool，供调试使用。
```
Request:  { "tool": "web_search", "args": "{\"query\": \"go generics\"}" }
Response: { "output": "...", "ok": true, "elapsedMs": 342 }
        | { "output": "", "ok": false, "elapsedMs": 12, "error": "..." }
```
- `args` 为 JSON 字符串，原样传给 tool 的 `Execute(ctx, argsJSON)`；省略时默认 `"{}"`
- `tool` 名称查不到 → 404
- tool 内部 panic / Execute 返 error → `ok=false`、200 状态码（dev 端点风格）

---

## 右侧工具面板（6 个 Tab）

实际 Tab 顺序见 `tester.html`：**Config / SSE / Logs / SQL / Tests / Tools**。

### Config（默认 tab）
- API Keys CRUD + connectivity test
- Chat scenario 的 Model 配置（PUT `/api/v1/model-configs/chat`）

### SSE(2026-05-08 重构后:递归事件日志协议 + 2026-05-12 三流统一)
- **三流模型**(C3/C4):
  - `/api/v1/eventlog`(per-user)— 5 events × 6 block types,chat 内容
  - `/api/v1/notifications`(per-user)— entity 状态变更(瘦身 payload)
  - `/api/v1/forge`(per-user)— 4 events × 3 kinds,trinity 锻造进度
- testend 经共享 Alpine store(chatBus / notifBus / forgeBus)单 SSE 连接 fanout 给多 tab
- chat panel 改订 chatBus + 客户端按 `payload.conversationId` demux(D-redo-2);不再 `?conversationId=` query
- Forge tab(C5)显示 4 类锻造事件 + kind/type 过滤

### Logs
- 连接 `GET /dev/logs` EventSource
- 每行：`[时间] LEVEL  msg  {fields JSON}`
- level 颜色：`INFO`=绿 / `WARN`=黄 / `ERROR`=红 / `DEBUG`=灰
- 启动时回放历史
- Auto-scroll + Clear + 关键词过滤输入框

### SQL
- textarea 输入 SQL → Run → `POST /dev/sql`
- 结果渲染为可横向滚动的 HTML table，含行数统计
- 快捷按钮：messages / message_blocks / conversations / api_keys / model_configs / chat_attachments / tools / tool_versions / tool_test_cases / tool_run_history / tool_test_history

### Tests
- 左侧：集合列表（来自 `GET /dev/collections`）
- 右侧：选中集合的 step 列表 + Run 按钮
- 顶部环境变量输入框（`KEY=VALUE` 多行格式，注入 `{{env.KEY}}`）
- 点 Run 后**前端**逐步 fetch：每步实时更新状态 ⏳ pending / ⟳ running / ✓ pass / ✗ fail
- 每步可展开：请求详情 + 响应体 + status 断言结果 + latencyMs
- 结果存内存，刷新后清空

### Tools
两个子面板通过 [System][User Tools] 切换：

**System 子面板**（`/dev/invoke`）
- 下拉选择 tool（`GET /dev/tools` 填充）
- JSON textarea 填入参数（Ctrl+Enter 运行）
- 展示 output、ok/error、elapsedMs

**User Tools 子面板**（`/api/v1/tools`）
- 可搜索的工具列表（最多 200 条）
- 点击工具 → 展示代码预览 + JSON input 表单
- Run 按钮 → `POST /api/v1/tools/{id}:run` → 展示 ExecutionResult

---

## 测试集合格式（`collections/*.yaml`）

```yaml
name: "Phase 2 冒烟测试"
description: "apikey / model / conversation / chat 基础流程"
steps:
  - name: "创建 API Key"
    method: POST
    path: /api/v1/api-keys
    body:
      provider: "openai"
      key: "{{env.TEST_API_KEY}}"
      displayName: "smoke-test"
    expect:
      status: 201

  - name: "设置模型"
    method: PUT
    path: /api/v1/model-configs/chat
    body:
      provider: "openai"
      modelId: "gpt-4o-mini"
    expect:
      status: 200

  - name: "创建对话"
    method: POST
    path: /api/v1/conversations
    body:
      title: "smoke test conv"
    expect:
      status: 201
    capture:
      convId: "$.data.id"

  - name: "发消息"
    method: POST
    path: /api/v1/conversations/{{convId}}/messages
    body:
      content: "Hello"
    expect:
      status: 202
```

**字段说明**：

| 字段 | 必填 | 说明 |
|---|---|---|
| `name` | ✅ | 步骤名 |
| `method` | ✅ | HTTP 方法 |
| `path` | ✅ | 路径，可含 `{{变量}}` 或 `{{env.KEY}}` |
| `body` | ❌ | JSON 对象，可含 `{{变量}}` |
| `expect.status` | ❌ | 期望状态码，不填则不断言 |
| `capture` | ❌ | 从响应捕获变量，支持 JSONPath 子集（如 `$.data.id`）|

**执行模型**：前端按顺序执行，每步等上一步完成（含变量捕获）才发下一步。step 之间无并发。

---

## 启动方式（项目根 Makefile）

`testend/` 不再持有自己的 Makefile，全部纳入项目根 `Makefile`：

```makefile
BACKEND_DATA_DIR ?= /tmp/forgify-dev
LOG_FILE         := /tmp/forgify-dev.log
PORT             ?= 8742

testend:
	@lsof -ti :$(PORT) | xargs kill 2>/dev/null || true
	@sleep 0.3
	@cd backend && \
	  go run ./cmd/server \
	    --dev \
	    --port $(PORT) \
	    --data-dir $(BACKEND_DATA_DIR) \
	    --collections-dir $(shell pwd)/testend/collections \
	    --integration-dir $(shell pwd)/testend \
	  > $(LOG_FILE) 2>&1 &
	@echo "→ Waiting for backend..."
	@while ! curl -sf http://localhost:$(PORT)/api/v1/health > /dev/null 2>&1; do sleep 0.5; done
	@echo "→ http://localhost:$(PORT)/dev/  (logs → $(LOG_FILE))"
	@open http://localhost:$(PORT)/dev/

stop:
	@lsof -ti :$(PORT) | xargs kill 2>/dev/null || true

logs:
	@tail -f $(LOG_FILE)

clear:
	@lsof -ti :$(PORT) | xargs kill 2>/dev/null || true
	@rm -rf $(BACKEND_DATA_DIR)
	@rm -f  $(LOG_FILE)
```

**注意**：自从 SQLite 驱动迁移至 `modernc.org/sqlite`（2026-05-01）后，`CGO_CFLAGS="-DSQLITE_ENABLE_FTS5"` 已不再需要——modernc 内置 FTS5、纯 Go、无需 C 工具链。

---

## 验证步骤

```bash
# 启动 dev 模式
make testend
# 浏览器自动打开 http://localhost:8742/dev/

# 1. Config Tab → 填 API Key + Model → 保存
#    Logs Tab：看到 "apikey created" + "model config upserted"
# 2. 左侧新建对话 → 中间输入框发一条消息
#    聊天区：流式 token 逐字出现，标题自动生成后 sidebar 更新
# 3. SSE Tab：看到 chat.token * N 条 + chat.done
# 4. Logs Tab：chat task enqueued → chat task done（含 stop_reason）
# 5. SQL Tab → 快捷按钮 messages：出现 user + assistant 两行
# 6. SQL Tab → 快捷按钮 message_blocks：可见对应 blocks（text / tool_call / tool_result 等）
# 7. Tests Tab → 选 phase2-smoke.yaml → 填 TEST_API_KEY → Run → 全绿
# 8. Tools Tab → System 子面板：下拉选 datetime → Run → 看到当前时间

# 验证非 dev 模式下路由不暴露：
go run ./backend/cmd/server   # 不加 --dev
curl http://localhost:8742/dev/logs   # 应返回 404
```

```bash
# 回归测试（CGO 已不需要）
cd backend && go test -count=1 -race ./...
```
