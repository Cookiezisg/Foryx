# C2 · 消息流 UI — 技术设计文档

**切片**：C2  
**状态**：待 Review

---

## 1. 目录结构

```
internal/
├── storage/migrations/
│   └── 004_messages.sql
└── service/
    └── message.go

frontend/src/
└── components/chat/
    ├── MessageList.tsx          # 消息列表 + 滚动逻辑
    ├── MessageItem.tsx          # 分发各消息类型
    ├── cards/
    │   ├── ToolCreatedCard.tsx  # AI 操作卡片（工具）
    │   ├── WorkflowUpdatedCard.tsx
    │   ├── ToolTestResultCard.tsx
    │   └── FlowRunCard.tsx
    ├── ChatInput.tsx            # 底部输入框（B2 已有，本切片完善）
    └── EmptyChat.tsx            # 空白状态
```

---

## 2. 数据库迁移

### 004_messages.sql

```sql
CREATE TABLE IF NOT EXISTS messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL CHECK(role IN ('user','assistant','system')),
    content         TEXT NOT NULL DEFAULT '',
    content_type    TEXT NOT NULL DEFAULT 'text'
                        CHECK(content_type IN ('text','tool_created','workflow_updated',
                                               'tool_test_result','flow_run')),
    metadata        JSON,          -- 卡片类消息的结构化数据
    model_id        TEXT,          -- AI 消息用了哪个模型
    created_at      DATETIME DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_messages_conversation
    ON messages(conversation_id, created_at);
```

---

## 3. Go 服务层

### `internal/service/message.go`

```go
package service

import (
    "encoding/json"
    "forgify/internal/storage"
    "time"

    "github.com/google/uuid"
)

type Message struct {
    ID             string          `json:"id"`
    ConversationID string          `json:"conversationId"`
    Role           string          `json:"role"`
    Content        string          `json:"content"`
    ContentType    string          `json:"contentType"`
    Metadata       json.RawMessage `json:"metadata,omitempty"`
    ModelID        string          `json:"modelId,omitempty"`
    CreatedAt      time.Time       `json:"createdAt"`
}

// 各卡片类型的 Metadata 结构
type ToolCreatedMeta struct {
    ToolID   string `json:"toolId"`
    ToolName string `json:"toolName"`
    Status   string `json:"status"`
}
type ToolTestResultMeta struct {
    ToolID   string `json:"toolId"`
    Passed   bool   `json:"passed"`
    Duration int64  `json:"duration"` // ms
    Output   any    `json:"output"`
}
type FlowRunMeta struct {
    RunID      string `json:"runId"`
    WorkflowID string `json:"workflowId"`
    Status     string `json:"status"` // "running"|"success"|"failed"
    Progress   int    `json:"progress"`
    Total      int    `json:"total"`
}

type MessageService struct{}

func (s *MessageService) Save(msg *Message) error {
    meta, _ := json.Marshal(msg.Metadata)
    _, err := storage.DB().Exec(`
        INSERT INTO messages (id, conversation_id, role, content, content_type, metadata, model_id)
        VALUES (?, ?, ?, ?, ?, ?, ?)
    `, msg.ID, msg.ConversationID, msg.Role, msg.Content,
        msg.ContentType, string(meta), msg.ModelID)
    return err
}

func (s *MessageService) List(conversationID string) ([]*Message, error) {
    rows, err := storage.DB().Query(`
        SELECT id, conversation_id, role, content, content_type, metadata, model_id, created_at
        FROM messages WHERE conversation_id = ? ORDER BY created_at ASC
    `, conversationID)
    if err != nil { return nil, err }
    defer rows.Close()
    var msgs []*Message
    for rows.Next() {
        m := &Message{}
        var meta string
        rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content,
            &m.ContentType, &meta, &m.ModelID, &m.CreatedAt)
        m.Metadata = json.RawMessage(meta)
        msgs = append(msgs, m)
    }
    return msgs, nil
}
```

### 在 ChatService 中保存消息（接 B2）

```go
// service/chat.go — 在 stream 完成后保存
func (s *ChatService) SendMessage(convID, userMsg string) error {
    // 1. 保存用户消息
    s.msgSvc.Save(&Message{
        ID: uuid.NewString(), ConversationID: convID,
        Role: "user", Content: userMsg,
    })

    // 2. 调用 LLM，收集完整回复
    var fullContent string
    // ... stream loop (同 B2)，accumulate tokens into fullContent

    // 3. 保存 AI 回复
    s.msgSvc.Save(&Message{
        ID: uuid.NewString(), ConversationID: convID,
        Role: "assistant", Content: fullContent, ModelID: modelID,
    })

    // 4. 触发自动命名（如果是第一轮对话）
    s.convSvc.MaybeAutoTitle(convID, userMsg, fullContent)
    return nil
}
```

---

## 4. 前端组件

### `MessageList.tsx` — 滚动逻辑

```tsx
export function MessageList({ conversationId }: { conversationId: string }) {
    const { messages, isStreaming } = useStreaming(conversationId)
    const bottomRef = useRef<HTMLDivElement>(null)
    const containerRef = useRef<HTMLDivElement>(null)
    const [userScrolled, setUserScrolled] = useState(false)

    // 自动滚到底部
    useEffect(() => {
        if (!userScrolled) {
            bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
        }
    }, [messages, userScrolled])

    // 检测用户上滚
    const onScroll = () => {
        const el = containerRef.current
        if (!el) return
        const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
        setUserScrolled(!atBottom)
    }

    return (
        <div ref={containerRef} onScroll={onScroll}
             className="flex-1 overflow-y-auto px-4 py-4 space-y-4">
            {messages.length === 0 && <EmptyChat />}
            {messages.map(m => <MessageItem key={m.id} message={m} />)}
            <div ref={bottomRef} />

            {/* 回到最新按钮 */}
            {userScrolled && (
                <button
                    onClick={() => {
                        setUserScrolled(false)
                        bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
                    }}
                    className="fixed bottom-24 right-8 px-3 py-1.5 text-sm
                               bg-neutral-700 rounded-full shadow-lg hover:bg-neutral-600"
                >
                    ↓ 回到最新
                </button>
            )}
        </div>
    )
}
```

### `MessageItem.tsx` — 消息类型分发

```tsx
export function MessageItem({ message }: { message: Message }) {
    switch (message.contentType) {
        case 'tool_created':
            return <ToolCreatedCard meta={message.metadata as ToolCreatedMeta} />
        case 'workflow_updated':
            return <WorkflowUpdatedCard meta={message.metadata} />
        case 'tool_test_result':
            return <ToolTestResultCard meta={message.metadata as ToolTestResultMeta} />
        case 'flow_run':
            return <FlowRunCard meta={message.metadata as FlowRunMeta} />
        default:
            return (
                <div className={`flex gap-3 ${message.role === 'user' ? 'justify-end' : ''}`}>
                    {message.role === 'assistant' && (
                        <span className="text-xs text-neutral-500 mt-1">{message.modelId}</span>
                    )}
                    <div className={`max-w-[80%] rounded-xl px-4 py-3 text-sm ${
                        message.role === 'user'
                            ? 'bg-blue-600 text-white'
                            : 'bg-transparent text-neutral-100'
                    }`}>
                        <MarkdownContent content={message.content} />
                    </div>
                </div>
            )
    }
}
```

### `cards/ToolCreatedCard.tsx` 示例

```tsx
export function ToolCreatedCard({ meta }: { meta: ToolCreatedMeta }) {
    return (
        <div className="border border-neutral-700 rounded-xl p-4 max-w-sm">
            <div className="flex items-center gap-2 mb-2">
                <span>✅</span>
                <span className="text-sm font-medium">已创建工具</span>
            </div>
            <p className="text-sm text-neutral-300 mb-1">📦 {meta.toolName}</p>
            <p className="text-xs text-neutral-500 mb-3">状态：{meta.status}</p>
            <button className="text-xs text-blue-400 hover:underline">
                打开工具详情 →
            </button>
        </div>
    )
}
```

---

## 5. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/conversations/{id}/messages", s.listMessages)
```

---

## 6. 验收测试

```
1. 发消息 → AI 回复 → 刷新页面 → 消息仍在（持久化验证）
2. 快速发 3 条消息，顺序正确，不乱序
3. AI 输出长内容（500字+）时，自动滚到底部
4. 手动上滚 → "回到最新"按钮出现 → 点击回到底部
5. 切换到另一个对话，再切回来，消息列表不重置
6. 模拟触发 tool_created 事件 → 操作卡片正常渲染
7. 空对话显示占位内容，发消息后占位消失
```
