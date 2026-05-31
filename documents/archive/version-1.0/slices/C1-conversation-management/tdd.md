# C1 · 对话管理 — 技术设计文档

**切片**：C1  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 绑定关系存储 | conversations 表的 asset_id + asset_type | 1:1 关系，直接字段比关联表简单 |
| 文件夹 | 不做（MVP 范围外）| 资产 mini-sidebar 已经提供了资产维度的对话组织 |
| 自动命名 | goroutine + PurposeCheap 模型 | 首次回复完成后异步触发，不阻塞主流程 |
| 绑定事件 | EventBridge emit chat.bound | 前端监听后更新绑定指示条 |

---

## 2. 目录结构

```
internal/
├── storage/migrations/
│   └── 003_conversations.sql
└── service/
    └── conversation.go

frontend/src/
├── components/chat/
│   ├── ConversationSidebar.tsx   # 对话 Tab 内的左侧列表
│   ├── ConversationItem.tsx      # 单条对话
│   ├── BindingIndicator.tsx      # 对话顶部的绑定指示条
│   └── AssetPicker.tsx           # 资产选择浮层
└── components/common/
    └── ContextMenu.tsx
```

---

## 3. 数据库迁移

### `003_conversations.sql`

```sql
CREATE TABLE IF NOT EXISTS conversations (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL DEFAULT '新对话',
    asset_id    TEXT,                    -- 绑定的资产 ID（工具或工作流）
    asset_type  TEXT                     -- 'tool' | 'workflow'
                CHECK(asset_type IN ('tool','workflow') OR asset_type IS NULL),
    status      TEXT NOT NULL DEFAULT 'active'
                CHECK(status IN ('active','archived')),
    created_at  DATETIME DEFAULT (datetime('now')),
    updated_at  DATETIME DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_conversations_status  ON conversations(status);
CREATE INDEX IF NOT EXISTS idx_conversations_updated ON conversations(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_conversations_asset   ON conversations(asset_id) WHERE asset_id IS NOT NULL;
```

---

## 4. Go 服务层

### `internal/service/conversation.go`

```go
package service

import (
    "context"
    "strings"
    "time"

    "forgify/internal/events"
    "forgify/internal/model"
    "forgify/internal/storage"

    "github.com/cloudwego/eino/schema"
    "github.com/google/uuid"
)

type Conversation struct {
    ID        string     `json:"id"`
    Title     string     `json:"title"`
    AssetID   string     `json:"assetId,omitempty"`
    AssetType string     `json:"assetType,omitempty"` // "tool" | "workflow"
    Status    string     `json:"status"`
    CreatedAt time.Time  `json:"createdAt"`
    UpdatedAt time.Time  `json:"updatedAt"`
}

type ConversationService struct {
    gateway *model.ModelGateway
    bridge  *events.Bridge
}

func (s *ConversationService) Create() (*Conversation, error) {
    id := uuid.NewString()
    _, err := storage.DB().Exec(
        `INSERT INTO conversations (id) VALUES (?)`, id)
    if err != nil { return nil, err }
    return s.Get(id)
}

func (s *ConversationService) Get(id string) (*Conversation, error) {
    convs, err := s.scan(
        `SELECT id, title, asset_id, asset_type, status, created_at, updated_at
         FROM conversations WHERE id = ?`, id)
    if err != nil || len(convs) == 0 { return nil, err }
    return convs[0], nil
}

func (s *ConversationService) List() ([]*Conversation, error) {
    return s.scan(`
        SELECT id, title, asset_id, asset_type, status, created_at, updated_at
        FROM conversations
        WHERE status = 'active'
        ORDER BY updated_at DESC
    `)
}

func (s *ConversationService) ListByAsset(assetID string) ([]*Conversation, error) {
    return s.scan(`
        SELECT id, title, asset_id, asset_type, status, created_at, updated_at
        FROM conversations
        WHERE asset_id = ? AND status = 'active'
        ORDER BY updated_at DESC
    `, assetID)
}

func (s *ConversationService) Rename(id, title string) error {
    _, err := storage.DB().Exec(
        `UPDATE conversations SET title=?, updated_at=datetime('now') WHERE id=?`,
        title, id)
    return err
}

func (s *ConversationService) Bind(id, assetID, assetType string) error {
    _, err := storage.DB().Exec(
        `UPDATE conversations SET asset_id=?, asset_type=?, updated_at=datetime('now') WHERE id=?`,
        nullStr(assetID), nullStr(assetType), id)
    if err != nil { return err }
    // 通知前端绑定状态变化
    s.bridge.Emit(events.ChatBound, map[string]any{
        "conversationId": id,
        "assetId":        assetID,
        "assetType":      assetType,
    })
    return nil
}

func (s *ConversationService) Unbind(id string) error {
    return s.Bind(id, "", "")
}

func (s *ConversationService) Archive(id string) error {
    _, err := storage.DB().Exec(
        `UPDATE conversations SET status='archived', updated_at=datetime('now') WHERE id=?`, id)
    return err
}

func (s *ConversationService) Delete(id string) error {
    _, err := storage.DB().Exec(`DELETE FROM conversations WHERE id=?`, id)
    return err
}

func (s *ConversationService) Search(query string) ([]*Conversation, error) {
    q := "%" + query + "%"
    return s.scan(`
        SELECT DISTINCT c.id, c.title, c.asset_id, c.asset_type, c.status, c.created_at, c.updated_at
        FROM conversations c
        LEFT JOIN messages m ON m.conversation_id = c.id
        WHERE c.status = 'active'
          AND (c.title LIKE ? OR m.content LIKE ?)
        ORDER BY c.updated_at DESC
    `, q, q)
}

// AutoTitle 在首次 AI 完整回复后异步生成标题
func (s *ConversationService) AutoTitle(ctx context.Context, convID, firstExchange string) {
    go func() {
        m, _, err := s.gateway.GetModel(ctx, model.PurposeCheap)
        if err != nil || m == nil { return }
        resp, err := m.Generate(ctx, []*schema.Message{
            schema.UserMessage("根据以下对话内容，生成一个简洁的标题（最多15字，不加引号）：\n\n" + firstExchange),
        })
        if err != nil { return }
        title := strings.TrimSpace(resp.Content)
        if len([]rune(title)) > 15 {
            title = string([]rune(title)[:15])
        }
        if title != "" {
            s.Rename(convID, title)
            s.bridge.Emit(events.ChatTitleUpdated, map[string]any{
                "conversationId": convID, "title": title,
            })
        }
    }()
}

func (s *ConversationService) scan(query string, args ...any) ([]*Conversation, error) {
    rows, err := storage.DB().Query(query, args...)
    if err != nil { return nil, err }
    defer rows.Close()
    var convs []*Conversation
    for rows.Next() {
        c := &Conversation{}
        var assetID, assetType *string
        rows.Scan(&c.ID, &c.Title, &assetID, &assetType,
            &c.Status, &c.CreatedAt, &c.UpdatedAt)
        if assetID != nil  { c.AssetID = *assetID }
        if assetType != nil { c.AssetType = *assetType }
        convs = append(convs, c)
    }
    return convs, nil
}

func nullStr(s string) any {
    if s == "" { return nil }
    return s
}
```

---

## 5. 事件扩展（接 A3）

```go
// events/events.go 新增
const (
    ChatBound        = "chat.bound"         // 对话绑定了资产
    ChatTitleUpdated = "chat.title_updated" // AI 自动命名完成
)
```

TypeScript 侧：
```ts
interface ChatBoundPayload {
    conversationId: string
    assetId: string
    assetType: 'tool' | 'workflow'
}
interface ChatTitleUpdatedPayload {
    conversationId: string
    title: string
}
```

---

## 6. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/conversations", s.listConversations)
mux.HandleFunc("POST /api/conversations", s.createConversation)
mux.HandleFunc("GET /api/conversations/search", s.searchConversations)
mux.HandleFunc("GET /api/conversations/by-asset/{assetId}", s.listConversationsByAsset)
mux.HandleFunc("PATCH /api/conversations/{id}/rename", s.renameConversation)
mux.HandleFunc("PATCH /api/conversations/{id}/bind", s.bindConversation)
mux.HandleFunc("PATCH /api/conversations/{id}/unbind", s.unbindConversation)
mux.HandleFunc("PATCH /api/conversations/{id}/archive", s.archiveConversation)
mux.HandleFunc("DELETE /api/conversations/{id}", s.deleteConversation)
```

---

## 7. 前端组件

### `ConversationSidebar.tsx`

```tsx
export function ConversationSidebar() {
    const [conversations, setConversations] = useState<Conversation[]>([])
    const [query, setQuery] = useState('')
    const [activeId, setActiveId] = useState<string | null>(null)

    const load = () => fetch(`http://127.0.0.1:${port}/api/conversations`).then(r => r.json()).then(setConversations)
    useEffect(() => { load() }, [])

    // 监听 chat.title_updated 事件，实时更新标题
    useEffect(() => {
        return onEvent(EV.ChatTitleUpdated, ({ conversationId, title }) => {
            setConversations(prev =>
                prev.map(c => c.id === conversationId ? { ...c, title } : c))
        })
    }, [])

    const displayed = query
        ? conversations.filter(c => c.title.toLowerCase().includes(query.toLowerCase()))
        : conversations

    return (
        <div className="flex flex-col h-full">
            <div className="px-3 py-2">
                <input value={query} onChange={e => setQuery(e.target.value)}
                    placeholder="搜索对话..."
                    className="w-full px-3 py-1.5 text-sm bg-neutral-800 rounded-lg
                               text-neutral-200 placeholder-neutral-500 outline-none" />
            </div>

            <button
                onClick={async () => {
                    const conv = await CreateConversation()
                    setConversations(prev => [conv, ...prev])
                    setActiveId(conv.id)
                }}
                className="mx-3 mb-2 px-3 py-1.5 text-sm text-neutral-300
                           hover:bg-neutral-800 rounded-lg text-left">
                + 新建对话
            </button>

            <div className="flex-1 overflow-y-auto">
                {displayed.map(c => (
                    <ConversationItem
                        key={c.id} conv={c}
                        active={activeId === c.id}
                        onClick={() => setActiveId(c.id)}
                        onRename={title => RenameConversation(c.id, title)}
                        onArchive={() => ArchiveConversation(c.id).then(load)}
                        onDelete={() => DeleteConversation(c.id).then(load)}
                    />
                ))}
            </div>
        </div>
    )
}
```

### `ConversationItem.tsx`

```tsx
const ASSET_BADGE = {
    workflow: { icon: '⚡', color: 'text-yellow-400' },
    tool:     { icon: '📦', color: 'text-blue-400' },
}

export function ConversationItem({ conv, active, onClick, onRename, onArchive, onDelete }:
    { conv: Conversation; active: boolean; onClick: () => void;
      onRename: (t: string) => void; onArchive: () => void; onDelete: () => void }) {

    const badge = conv.assetType ? ASSET_BADGE[conv.assetType as keyof typeof ASSET_BADGE] : null

    return (
        <div onClick={onClick}
            className={`group flex items-center gap-1 px-3 py-2 cursor-pointer text-sm
                ${active ? 'bg-neutral-800 text-white' : 'text-neutral-400 hover:bg-neutral-800/50'}`}>
            <span className="flex-1 truncate">{conv.title}</span>
            {badge && (
                <span className={`text-xs flex-shrink-0 ${badge.color}`}>{badge.icon}</span>
            )}
            <ContextMenu
                items={[
                    { label: '重命名', onClick: () => promptRename(conv.title, onRename) },
                    { label: '调整绑定', onClick: () => {/* 打开 AssetPicker */} },
                    { type: 'separator' },
                    { label: '归档', onClick: onArchive },
                    { label: '删除', onClick: () => confirm('确认删除此对话？') && onDelete(), danger: true },
                ]}
            />
        </div>
    )
}
```

### `BindingIndicator.tsx`

```tsx
interface Props {
    conversationId: string
    assetId: string
    assetType: 'tool' | 'workflow'
    assetName: string
    onUnbind: () => void
    onSwitch: (newAssetId: string, newAssetType: string) => void
}

export function BindingIndicator({ conversationId, assetId, assetType, assetName, onUnbind, onSwitch }: Props) {
    const [pickerOpen, setPickerOpen] = useState(false)

    return (
        <div className="flex items-center gap-2 px-4 py-2 bg-neutral-800/50 border-b border-neutral-700
                        text-xs text-neutral-400 animate-fade-in">
            <span className="text-neutral-500">📎 已关联到：</span>
            <span className="text-neutral-200 font-medium">{assetName}</span>
            <button onClick={() => setPickerOpen(true)}
                className="text-blue-400 hover:underline">切换 ▼</button>
            <button onClick={onUnbind}
                className="text-neutral-500 hover:text-neutral-300">解除关联</button>

            {pickerOpen && (
                <AssetPicker
                    currentAssetId={assetId}
                    onSelect={(id, type) => {
                        onSwitch(id, type)
                        setPickerOpen(false)
                    }}
                    onClose={() => setPickerOpen(false)}
                />
            )}
        </div>
    )
}
```

### `AssetPicker.tsx`

```tsx
export function AssetPicker({ currentAssetId, onSelect, onClose }:
    { currentAssetId: string; onSelect: (id: string, type: string) => void; onClose: () => void }) {

    const [workflows, setWorkflows] = useState<Workflow[]>([])
    const [tools, setTools] = useState<Tool[]>([])

    useEffect(() => {
        Promise.all([ListWorkflows(), ListTools('', '')]).then(([wf, tl]) => {
            setWorkflows(wf)
            setTools(tl)
        })
    }, [])

    return (
        <div className="absolute top-8 left-0 z-50 w-64 bg-neutral-800 border border-neutral-700
                        rounded-xl shadow-xl py-2">
            <div className="flex items-center justify-between px-3 pb-2 border-b border-neutral-700">
                <span className="text-xs font-medium text-neutral-300">选择关联资产</span>
                <button onClick={onClose} className="text-neutral-500 hover:text-neutral-300 text-xs">关闭</button>
            </div>
            <div className="px-2 pt-2">
                <p className="text-xs text-neutral-500 px-1 mb-1">工作流</p>
                {workflows.map(w => (
                    <button key={w.id} onClick={() => onSelect(w.id, 'workflow')}
                        className={`w-full text-left px-2 py-1.5 text-sm rounded hover:bg-neutral-700
                            ${w.id === currentAssetId ? 'text-blue-400' : 'text-neutral-200'}`}>
                        ⚡ {w.name}
                    </button>
                ))}
                <p className="text-xs text-neutral-500 px-1 mt-2 mb-1">工具</p>
                {tools.map(t => (
                    <button key={t.id} onClick={() => onSelect(t.id, 'tool')}
                        className={`w-full text-left px-2 py-1.5 text-sm rounded hover:bg-neutral-700
                            ${t.id === currentAssetId ? 'text-blue-400' : 'text-neutral-200'}`}>
                        📦 {t.displayName}
                    </button>
                ))}
            </div>
        </div>
    )
}
```

---

## 8. 验收测试

```
1. 点击"新建对话"→ 列表顶部出现新条目，进入空白对话
2. AI 回复后约 5 秒内，标题从"新对话"自动更新
3. 对话列表按最后活跃时间倒序排列
4. 绑定了工作流的对话显示 ⚡ 徽章，工具显示 📦 徽章
5. 操作菜单：重命名即时生效；归档后消失，归档区可见；删除需确认
6. AI 创建工具/工作流后，对话顶部 BindingIndicator 自动出现
7. 点击"切换" → AssetPicker 弹出，选择新资产 → 指示条更新
8. 点击"解除关联" → 指示条消失，列表徽章消失
9. 搜索"邮件"只显示匹配的对话，清空后恢复完整列表
```
