# Forgify — 技术设计文档

**版本**：v0.4  
**日期**：2026-04-19  
**配套 PRD**：PRD_1.0.md（v0.3）

> **⚠️ V1.1 迭代说明**（2026-04-20）
> 
> 本文档定义了 V1.0 的技术架构，Tier 1-4 已按此实现。V1.1 对前端架构做了重大变更：
> 
> - **前端导航模型** → V1.1 引入 TabContext + TabBar + Layout 组件替代现有 SidebarNav + 固定页面切换
> - **目录结构** → V1.1 新增 `components/layouts/` 目录，拆分 `pages/ChatPage.tsx` 和 `pages/AssetsPage.tsx`
> - **状态管理** → V1.1 新增 TabContext（Tab 生命周期），现有 ChatContext 保留不变
> - **后端 API** → V1.1 新增工具元数据、标签、版本、测试用例相关端点，现有端点全部不变
> 
> 详见 `Documents/V1.1/TDD_1.1.md`。后续 Tier 5-9 的前端开发应基于 V1.1 架构。

---

## 0. 技术选型总览

| 层级 | 技术 | 理由 |
|---|---|---|
| 桌面框架 | **Electron** | Chromium 内核，渲染行为标准，systray 原生支持 |
| 后端语言 | **Go 1.23+** | 性能、并发，以 HTTP subprocess 形式运行 |
| 前后端通信 | **HTTP REST + SSE** | REST 用于调用，SSE 用于 Go→前端推送 |
| LLM 编排 | **Eino (CloudWeGo)** | Go 原生，Graph/ReAct/Interrupt-Resume |
| 前端 | **React + TypeScript + Vite** | 运行在 Electron Chromium 渲染进程 |
| 工作流画布 | **ReactFlow** | 节点/边拖拽，成熟生态 |
| 代码编辑器 | **Monaco Editor** | VS Code 同款，语法高亮、代码补全 |
| 本地数据库 | **SQLite (modernc)** | 零 CGO，纯 Go |
| Python 环境管理 | **uv** (Astral) | 单二进制，自带 Python 版本管理，venv 创建 35ms |
| Python 运行时 | **subprocess + uv venv** | 每工具独立环境，无需 Docker，无需用户预装 Python |
| LLM 调用统一 | **Eino ChatModel 抽象** | 支持 OpenAI / Anthropic / Ollama |
| 文件系统监听 | **fsnotify** | 跨平台文件事件监听（G3 trigger_file）|
| Cron 调度 | **robfig/cron v3** | 5 字段标准 Cron，动态注册/注销 |

**不用 Docker**：本地桌面 app 要求安装 Docker 门槛太高，uv venv + subprocess 足够。  
**HTTP 仅监听 127.0.0.1**：Go 后端绑定随机本地端口，端口号由 Electron 读取后通过 URL query param 传递给渲染进程。  
**Electron 而非 Wails**：Chromium 内核渲染行为可预测，原生缩放、DevTools、扩展生态均优于系统 WebView。

---

## 1. 整体架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Electron Desktop App                          │
│                                                                       │
│  ┌────────────────────────────────────┐   ┌────────────────────────┐ │
│  │   Chromium Renderer Process        │   │   Main Process (Node)  │ │
│  │                                    │   │                        │ │
│  │  React + TypeScript + Vite         │   │  BrowserWindow         │ │
│  │                                    │   │  Tray                  │ │
│  │  Home | 对话 | 资产 | Inbox        │   │  spawn Go subprocess   │ │
│  │  ├─ ChatInterface (流式)           │   │  IPC: fullscreen-change│ │
│  │  ├─ WorkflowCanvas (ReactFlow)     │   └────────────────────────┘ │
│  │  ├─ ToolMainView (Monaco)          │                               │
│  │  └─ InboxPage                      │   ↑ HTTP REST (前端→Go)       │
│  │                                    │   ↑ SSE  (Go→前端，实时推送)  │
│  └────────────────────────────────────┘                               │
│                                                                       │
│  ┌─────────────────────────────────────────────────────────────────┐  │
│  │              Go Backend (127.0.0.1:随机端口)                      │  │
│  │                                                                   │  │
│  │  ┌────────────────────┐  ┌─────────────────────────────────────┐│  │
│  │  │    Eino 层         │  │            业务层                    ││  │
│  │  │  ConversationAgent │  │  WorkflowService / RunService        ││  │
│  │  │  ├─ ChatModel      │  │  ToolService / PermissionService     ││  │
│  │  │  ├─ ContextManager │  │  ApprovalService / MailboxService    ││  │
│  │  │  └─ 三层压缩       │  │  CronScheduler / EventTriggerManager ││  │
│  │  │  FlowCompiler      │  │  PermissionGate                      ││  │
│  │  └────────────────────┘  └─────────────────────────────────────┘│  │
│  │                                                                   │  │
│  │  ┌────────────────────┐  ┌─────────────────────────────────────┐│  │
│  │  │    数据层           │  │         Python 沙箱                  ││  │
│  │  │  SQLite (modernc)  │  │  subprocess + uv venv                ││  │
│  │  └────────────────────┘  └─────────────────────────────────────┘│  │
│  └───────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 2. 目录结构

```
forgify/
├── package.json                 # Electron 根配置
├── electron/
│   ├── main.ts                  # Electron 主进程：窗口、托盘、启动 Go
│   └── preload.ts               # contextBridge：fullscreen-change IPC
│
├── backend/                     # Go HTTP 服务
│   ├── main.go                  # 监听随机端口，打印 FORGIFY_PORT=xxxx
│   ├── go.mod
│   └── internal/
│       ├── server/              # HTTP handler + SSE broker
│       ├── events/              # 事件常量 + Bridge
│       └── storage/             # SQLite 初始化 + 迁移
│
├── frontend/                    # React 渲染进程
│   ├── src/
│   │   ├── main.tsx             # 读 URL ?port= 初始化 SSE
│   │   ├── App.tsx
│   │   ├── lib/events.ts        # EventSource 封装，替代 @wailsio/runtime
│   │   └── ...
│   ├── package.json
│   └── vite.config.ts
│
└── internal/                    # Go 业务逻辑（backend/ 下）
│   ├── compiler/                # F1：FlowDefinition → Eino Graph
│   │   ├── compiler.go
│   │   ├── graph_builder.go
│   │   ├── validator.go
│   │   └── resolver.go
│   │
│   ├── runner/                  # F2：工作流执行引擎
│   │   ├── runner.go
│   │   ├── context.go
│   │   ├── retry.go
│   │   └── node_runners/
│   │       ├── tool.go
│   │       ├── condition.go
│   │       ├── llm.go
│   │       ├── agent.go
│   │       ├── approval.go
│   │       └── loop.go
│   │
│   ├── scheduler/               # G1：Cron 定时调度
│   │   └── scheduler.go
│   │
│   ├── triggers/                # G3：事件触发
│   │   ├── manager.go
│   │   ├── file_watcher.go
│   │   └── webhook.go
│   │
│   ├── permission/              # H1：权限门控
│   │   └── gate.go
│   │
│   ├── service/                 # 业务服务层
│   │   ├── conversation.go      # C1
│   │   ├── chat.go              # B2 + E2（含上下文注入）
│   │   ├── tool.go              # D1-D3
│   │   ├── workflow.go          # E1
│   │   ├── flow_parser.go       # E2（提取 flow-definition）
│   │   ├── run.go               # F2
│   │   ├── approval.go          # F3
│   │   ├── mailbox.go           # H2
│   │   ├── permission.go        # H1
│   │   ├── settings.go          # K1-K2
│   │   └── stats.go             # J1
│   │
│   ├── sandbox/                 # D3：Python 沙箱
│   │   ├── runner.go
│   │   ├── venv.go
│   │   └── scanner.go
│   │
│   ├── storage/                 # A2：数据层
│   │   ├── db.go
│   │   └── migrations/
│   │       ├── 001_conversations.sql
│   │       ├── 002_messages.sql
│   │       ├── 003_tools.sql
│   │       ├── 004_api_keys.sql
│   │       ├── 005_conversations_asset_bind.sql
│   │       ├── 006_tools_builtin.sql
│   │       ├── 007_workflows.sql
│   │       ├── 008_runs.sql
│   │       ├── 009_approvals.sql
│   │       ├── 010_permissions.sql
│   │       ├── 011_mailbox.sql
│   │       └── 012_settings.sql
│   │
│   └── events/                  # A3：事件系统
│       └── bridge.go
│
├── tools/                       # D6：内置工具
│   ├── email/
│   │   ├── gmail_read.py
│   │   ├── gmail_send.py
│   │   └── outlook_read.py
│   ├── file/
│   │   ├── read_file.py
│   │   ├── write_file.py
│   │   └── read_excel.py
│   ├── web/
│   │   ├── http_request.py
│   │   └── web_scrape.py
│   └── ...
│
└── frontend/                    # React 前端
    ├── src/
    │   ├── App.tsx              # A1：主布局（侧边栏 + 主区）
    │   ├── pages/
    │   │   ├── HomePage.tsx     # J1
    │   │   ├── ConversationsPage.tsx  # C1
    │   │   ├── AssetsPage.tsx   # D1 / E1
    │   │   └── InboxPage.tsx    # I1
    │   ├── components/
    │   │   ├── chat/            # C2：消息 UI
    │   │   ├── workflow/        # E1-E5：画布 + 节点
    │   │   │   ├── nodes/
    │   │   │   ├── edges/
    │   │   │   └── panels/
    │   │   ├── tool/            # D4：工具视图
    │   │   └── inbox/           # I1-I3：Inbox
    │   ├── hooks/
    │   │   └── useChat.ts       # B2：流式消息
    │   └── lib/
    │       ├── backend.ts       # 后端端口管理
    │       └── events.ts        # SSE EventSource 封装
    ├── package.json
    └── vite.config.ts
```

---

## 3. 前后端通信

### 3.1 两种通信方式

**HTTP REST（前端→Go）**：前端用 `fetch` 调用 Go HTTP API，Go 返回 JSON。

```typescript
const wf = await fetch(`http://127.0.0.1:${port}/api/workflows/${id}`).then(r => r.json())
await fetch(`http://127.0.0.1:${port}/api/workflows/${id}`, {
  method: 'PUT', body: JSON.stringify(def)
})
```

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/workflows/{id}", s.getWorkflow)
mux.HandleFunc("PUT /api/workflows/{id}", s.updateWorkflow)
```

**SSE（Go→前端，实时推送）**：前端订阅 `/events` SSE 端点，Go 通过 `events.Bridge.Emit()` 推送。

```go
srv.Events.Emit(events.NodeStatusChanged, map[string]any{"runId": runID, "nodeId": nodeID, "status": "running"})
```

```typescript
import { onEvent, EventNames } from './lib/events'
onEvent(EventNames.NodeStatusChanged, ({ nodeId, status }) => { ... })
```

**端口传递**：Electron 启动 Go 后读取 stdout 中的 `FORGIFY_PORT=xxxx`，加载前端时附在 URL 上：`http://localhost:5173?port=xxxx`。前端在 `main.tsx` 里读取并初始化 EventSource 连接。

### 3.2 Event 类型清单

| Event 名 | 方向 | 说明 |
|---|---|---|
| `chat.token` | Go→前端 | 流式 token |
| `chat.done` | Go→前端 | 本轮对话完成 |
| `chat.compacted` | Go→前端 | 上下文已压缩，显示提示 |
| `canvas.updated` | Go→前端 | AI 更新工作流，画布刷新 |
| `node.status_changed` | Go→前端 | 节点运行状态变化 |
| `run.completed` | Go→前端 | 工作流运行完成 |
| `run.failed` | Go→前端 | 工作流运行失败 |
| `approval.pending` | Go→前端 | Approval 节点等待审批 |
| `permission.required` | Go→前端 | 权限门控等待确认 |
| `mailbox.updated` | Go→前端 | Inbox 有新消息，含未读数 |
| `workflow.deployed` | Go→前端 | 工作流部署成功 |
| `open.conversation` | Go→前端 | 切换到指定对话（G4 错误修复）|

---

## 4. Eino 层设计

### 4.1 ConversationAgent

```
用户输入 + 隐藏上下文注入（工作流状态 / 最近错误）
    │
    ▼
┌──────────────────────────────────┐
│       ConversationAgent           │
│                                   │
│  ChatModel（K1 可配置）           │
│  ContextManager（三层压缩）       │
│                                   │
│  工具调用（非必需，按需配置）     │
│  ├─ 内置 Go 工具（调用速度快）   │
│  └─ 用户工具（Python 沙箱）      │
└──────────────────────────────────┘
    │
    ▼
流式 token → EventBridge → 前端（chat.token / chat.done）
    │
    ▼
OnAssistantMessage() 检查是否含 flow-definition 块
    └─ 是：提取 → AutoLayout → 更新 DB → 发 canvas.updated
```

**状态注入（E2）**：每次用户发消息前，`BuildContextInjection()` 检查绑定的工作流，将当前状态（节点数、最近错误）作为隐藏系统消息插入。

### 4.2 FlowCompiler（F1）

```
FlowDefinition (JSON)
    │
    ▼  validate()：检查节点类型、边引用、变量引用、环路、无触发节点
    │
    ▼  buildGraph()：节点类型 → Eino 组件映射
    │
    ▼  Eino Graph（可执行）
```

| NodeType | Eino 组件 |
|---|---|
| `trigger_*` | 入口节点 |
| `tool` | ToolNode（调用 Python sandbox）|
| `condition` | BranchNode |
| `approval` | ApprovalRunner（阻塞，等待 F3 审批）|
| `loop` | LoopNode（子图）|
| `parallel` | FanOutNode + JoinNode |
| `llm` | LLMNode（单次调用）|
| `agent` | AgentNode（ReAct 循环）|
| `subworkflow` | 递归编译 |

---

## 5. 数据库 Schema

迁移文件按编号顺序执行（`storage/db.go` 在启动时自动运行）：

```sql
-- 001 conversations
CREATE TABLE conversations (
    id         TEXT PRIMARY KEY,
    title      TEXT NOT NULL DEFAULT '新对话',
    asset_id   TEXT,
    asset_type TEXT CHECK(asset_type IN ('tool','workflow') OR asset_type IS NULL),
    status     TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived')),
    created_at DATETIME DEFAULT (datetime('now')),
    updated_at DATETIME DEFAULT (datetime('now'))
);

-- 002 messages
CREATE TABLE messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT REFERENCES conversations(id),
    role            TEXT CHECK(role IN ('user','assistant','system')),
    content         TEXT NOT NULL,
    token_count     INTEGER,
    metadata        JSON,
    created_at      DATETIME DEFAULT (datetime('now'))
);

-- 003 tools
CREATE TABLE tools (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description TEXT,
    status      TEXT NOT NULL DEFAULT 'draft'
                CHECK(status IN ('draft','testing','ready','deprecated')),
    code        TEXT NOT NULL DEFAULT '',
    builtin     BOOLEAN NOT NULL DEFAULT 0,
    version     TEXT,
    requires_key TEXT,
    permission  TEXT NOT NULL DEFAULT 'read'
                CHECK(permission IN ('read','write','execute')),
    created_at  DATETIME DEFAULT (datetime('now')),
    updated_at  DATETIME DEFAULT (datetime('now'))
);

-- 004 api_keys
CREATE TABLE api_keys (
    id         TEXT PRIMARY KEY,
    service    TEXT NOT NULL UNIQUE,
    key_value  TEXT NOT NULL,
    created_at DATETIME DEFAULT (datetime('now'))
);

-- 007 workflows
CREATE TABLE workflows (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL DEFAULT '新工作流',
    definition JSON NOT NULL DEFAULT '{"nodes":[],"edges":[]}',
    status     TEXT NOT NULL DEFAULT 'draft'
               CHECK(status IN ('draft','ready','deployed','paused','archived')),
    created_at DATETIME DEFAULT (datetime('now')),
    updated_at DATETIME DEFAULT (datetime('now'))
);

-- 008 runs
CREATE TABLE runs (
    id           TEXT PRIMARY KEY,
    workflow_id  TEXT NOT NULL REFERENCES workflows(id),
    status       TEXT NOT NULL DEFAULT 'running'
                 CHECK(status IN ('running','success','failed','cancelled')),
    trigger_type TEXT NOT NULL DEFAULT 'manual',
    params       JSON,
    started_at   DATETIME DEFAULT (datetime('now')),
    finished_at  DATETIME,
    error        TEXT
);

CREATE TABLE run_node_results (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES runs(id),
    node_id     TEXT NOT NULL,
    status      TEXT NOT NULL CHECK(status IN ('running','success','failed','skipped')),
    input       JSON,
    output      JSON,
    started_at  DATETIME,
    finished_at DATETIME,
    error       TEXT
);

-- 009 approvals
CREATE TABLE approvals (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES runs(id),
    node_id     TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    title       TEXT NOT NULL,
    message     TEXT NOT NULL,
    params      JSON,
    status      TEXT NOT NULL DEFAULT 'pending'
                CHECK(status IN ('pending','approved','rejected','timeout')),
    reject_mode TEXT NOT NULL DEFAULT 'skip'
                CHECK(reject_mode IN ('skip','stop','notify')),
    expires_at  DATETIME NOT NULL,
    decided_at  DATETIME,
    modified_params JSON
);

-- 010 tool_permissions
CREATE TABLE tool_permissions (
    tool_name  TEXT PRIMARY KEY,
    granted    BOOLEAN NOT NULL DEFAULT 0,
    granted_at DATETIME
);

-- 011 mailbox_messages
CREATE TABLE mailbox_messages (
    id         TEXT PRIMARY KEY,
    type       TEXT NOT NULL,
    priority   INTEGER NOT NULL DEFAULT 1,
    title      TEXT NOT NULL,
    body       TEXT NOT NULL,
    payload    JSON,
    status     TEXT NOT NULL DEFAULT 'pending'
               CHECK(status IN ('pending','read','resolved','dismissed')),
    created_at DATETIME DEFAULT (datetime('now')),
    updated_at DATETIME DEFAULT (datetime('now'))
);

-- 012 settings
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
```

---

## 6. 权限系统

### 6.1 工具权限级别

工具元数据中声明 `permission` 字段：

| 级别 | 触发行为 |
|---|---|
| `read` | 直接执行，不拦截 |
| `write` | 首次执行弹确认，记住"始终允许" |
| `execute` | 每次执行都路由到 Inbox 审批 |

### 6.2 Approval 等待机制

```go
// ApprovalService 使用 channel 实现阻塞等待
type ApprovalService struct {
    waiters map[string]chan ApprovalResult
    mu      sync.Mutex
}

func (s *ApprovalService) Wait(ctx context.Context, approvalID string) (ApprovalResult, error) {
    ch := make(chan ApprovalResult, 1)
    s.mu.Lock()
    s.waiters[approvalID] = ch
    s.mu.Unlock()

    select {
    case result := <-ch:
        return result, nil
    case <-time.After(timeout):
        return ApprovalResult{Status: "timeout"}, nil
    case <-ctx.Done():
        return ApprovalResult{}, ctx.Err()
    }
}
```

Approval 节点只阻塞自身 goroutine，工作流中无依赖该节点的其他分支继续运行（非阻塞）。

---

## 7. Python 沙箱

### 7.1 uv 管理 Python 环境

```
工具首次使用
    ↓
uv python install 3.11      # 下载预编译 Python，仅首次，~秒级
    ↓
uv venv .venv/              # 35ms，比 python -m venv 快 ~60x
    ↓
uv pip install -r deps.txt  # 全局缓存，比 pip 快 10-100x
    ↓
subprocess 执行（timeout=30s）
    ├── stdout → JSON output
    ├── stderr → 错误日志
    └── exitcode != 0 → RunError
```

### 7.2 跨平台 subprocess

```go
uvBin := filepath.Join(forgifyDataDir, "uv")
venvPython := filepath.Join(toolDir, ".venv", "bin", "python")
if runtime.GOOS == "windows" {
    venvPython = filepath.Join(toolDir, ".venv", "Scripts", "python.exe")
}

cmd := exec.Command(venvPython, "main.py")
cmd.Dir = toolDir
cmd.Env = append(os.Environ(),
    "FORGIFY_INPUT="+inputJSON,
    "FORGIFY_TOOL_ID="+toolID,
)
```

始终用 Args 列表，不拼 shell 字符串，Windows 路径有空格完全安全。

### 7.3 内置工具注册（D6）

```go
//go:embed tools/**/*.py
var toolsFS embed.FS

func Register(toolSvc *service.ToolService) error {
    return fs.WalkDir(toolsFS, "tools", func(path string, d fs.DirEntry, err error) error {
        // 解析文件头注释中的 @builtin @version @category 等元数据
        // 版本相同则跳过，否则 upsert（builtin=true）
    })
}
```

---

## 8. 上下文压缩（三层）

```go
func (cm *ContextManager) MaybeCompress(messages []Message, limit int) []Message {
    usage := estimateTokens(messages)
    switch {
    case usage > limit*88/100:
        return cm.autoCompact(messages)   // LLM 摘要历史
    case usage > limit*75/100:
        return cm.microCompact(messages)  // 本地裁剪工具调用详情
    default:
        return messages
    }
}

// FullCompact：用户手动触发，完整重新生成摘要
func (cm *ContextManager) FullCompact(messages []Message) []Message { ... }
```

压缩后发送 `chat.compacted` 事件，前端显示"部分历史已压缩，点击查看摘要"提示。

---

## 9. 调度与触发

### 9.1 Cron 调度（G1）

```go
func (s *CronScheduler) Register(wf *service.Workflow) {
    cronExpr := s.extractCron(wf.Definition)
    id, _ := s.c.AddFunc(cronExpr, func() {
        s.runner.Run(context.Background(), wf.ID, nil)
    })
    s.entryIDs[wf.ID] = id
}
```

不补偿错过的触发（关机期间的定时任务直接跳过）。

### 9.2 文件/Webhook 触发（G3）

- **文件监听**：`fsnotify` + 5 秒防抖，匹配 glob pattern 后调用 `runner.Run()`
- **Webhook**：`net/http` 监听指定 port+path，收到 POST 后立即返回 `{"status":"accepted","runId":"..."}` 并异步运行

---

## 10. 前端关键设计

### 10.1 VS Code 三栏布局

```
┌──────┬──────────────────────────────────────────────────────┐
│ 64px │                     主区                              │
│ 导航 │  二级面板（320px）  │       主内容区                  │
│侧边栏│  ─────────────────  │  ────────────────────────────  │
│      │  对话列表            │  ChatInterface                  │
│ Home │  /资产列表           │  ／WorkflowCanvas               │
│ 对话 │  /Inbox列表          │  ／ToolMainView                 │
│ 资产 │                      │  ／InboxDetailView              │
│Inbox │                      │                                 │
└──────┴──────────────────────┴─────────────────────────────────┘
```

每个 tab 管理自己的内部分栏，复用 `SplitView` 组件：

```tsx
export function SplitView({ left, right, leftWidth = '300px' }) {
    return (
        <div className="flex h-full overflow-hidden">
            <div style={{ width: leftWidth }} className="flex-shrink-0 border-r border-neutral-800 overflow-y-auto">{left}</div>
            <div className="flex-1 overflow-hidden">{right}</div>
        </div>
    )
}
```

### 10.2 流式消息接收

```typescript
// hooks/useChat.ts
export function useChat(conversationId: string) {
    const [messages, setMessages] = useState<Message[]>([])

    useEffect(() => {
        const off1 = onEvent(EV.ChatToken, (e) => {
            if (e.conversationId !== conversationId) return
            setMessages(prev => appendToken(prev, e.token))
        })
        const off2 = onEvent(EV.ChatDone, (e) => {
            if (e.conversationId !== conversationId) return
            setMessages(prev => finalize(prev, e.messageId))
        })
        return () => { off1(); off2() }
    }, [conversationId])

    return { messages }
}
```

### 10.3 工作流状态注入（替代 canvas echo）

用户在画布上的操作（拖动节点、修改配置）静默保存到 DB，不产生对话消息。

在用户**下一次发送消息**时，`BuildContextInjection()` 读取当前工作流状态作为隐藏系统消息注入：

```go
func (s *ChatService) BuildContextInjection(convID string) string {
    // 工作流状态：节点列表、边数量、最近失败信息
    return "[当前工作流状态]\n名称：...\n节点：...\n[最近一次运行失败]\n..."
}
```

AI 读到的是完整当前状态，不是变更日志。

---

## 11. 打包与分发

```bash
# 1. 编译 Go 后端
cd backend && go build -o ../dist-electron/forgify-backend .

# 2. 构建前端
npm --prefix frontend run build

# 3. 编译 Electron 主进程
tsc -p electron/tsconfig.json

# 4. 打包（electron-builder）
npx electron-builder --mac --win --linux
```

Go 后端二进制随 Electron 应用打包为 `extraResources`，Electron 在 `process.resourcesPath` 目录下找到它并 spawn。内置工具（`tools/` 目录）通过 `//go:embed` 打包进 Go 二进制，版本升级时通过 `Register()` 检查版本号自动更新。

---

## 12. 技术决策记录

| # | 决策 | 结论 | 理由 |
|---|---|---|---|
| 1 | Python 版本管理 | 打包 uv 二进制 | uv 自带版本管理，无需用户操作 |
| 2 | venv 初始化耗时 | uv venv 35ms | 比 python -m venv 快 60x |
| 3 | 桌面框架 | Electron | Chromium 内核，缩放/DevTools/渲染行为标准，systray 完善 |
| 4 | AI 操作工作流 | AI 输出 flow-definition JSON，Go 解析 | 不让 AI 直接操作 DB，有验证层 |
| 5 | 工作流状态感知 | 状态注入（不是 diff/changelog）| AI 读完整当前状态，简单可靠 |
| 6 | 审批阻塞问题 | Mailbox 模式（channel 等待）| 只阻塞自身 goroutine，其他分支继续 |
| 7 | 对话-资产关系 | 1:1 绑定，可切换/解除 | 简单清晰，避免多对多的复杂性 |
| 8 | 内置工具分发 | go:embed + 版本检查 | 零用户感知，随应用更新 |
| 9 | Windows 路径 | exec.Cmd Args 列表 | 原生 CreateProcess，路径空格安全 |
| 10 | 上下文压缩触发 | 75%/88% 双阈值 | MicroCompact 零成本，AutoCompact 仅在必要时调 API |

---

**文档结束**
