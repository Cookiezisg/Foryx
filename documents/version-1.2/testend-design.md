# Testend Dev Console — 设计文档

**关联**：[`backend-design.md`](./backend-design.md) / [`progress-record.md`](./progress-record.md)

---

## 概述

`testend/` 是面向开发者的本地调试面板。在项目根目录 `make testend` 一键启动后端（dev 模式）+ 浏览器自动打开调试 UI：配置凭证、真实聊天、观察流式响应、查后端日志、查数据库、跑系统工具、跑测试集合。

**不是前端应用，不进生产——纯 dev 工具。**

---

## 目录结构

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
