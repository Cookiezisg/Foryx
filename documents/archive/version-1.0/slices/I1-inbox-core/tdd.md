# I1 · Inbox 核心 — 技术设计文档

**切片**：I1  
**状态**：待 Review

---

## 1. 目录结构

```
frontend/src/
├── pages/InboxPage.tsx        # Inbox 主页面（左列表 + 右详情）
└── components/inbox/
    ├── InboxList.tsx          # 消息列表
    ├── InboxListItem.tsx      # 单条消息
    └── InboxBadge.tsx         # 侧边栏未读数量徽章
```

---

## 2. InboxPage

```tsx
// pages/InboxPage.tsx
export function InboxPage() {
    const [messages, setMessages] = useState<MailboxMessage[]>([])
    const [selectedId, setSelectedId] = useState<string | null>(null)

    const reload = () => ListMailbox().then(setMessages)
    useEffect(() => { reload() }, [])

    useEffect(() => {
        return onEvent(EV.MailboxUpdated, reload)
    }, [])

    const handleSelect = async (msg: MailboxMessage) => {
        setSelectedId(msg.id)
        if (msg.status === 'pending') {
            await MarkMailboxRead(msg.id)
            reload()
        }
    }

    const handleMarkAllRead = async () => {
        await Promise.all(
            messages.filter(m => m.status === 'pending').map(m => MarkMailboxRead(m.id))
        )
        reload()
    }

    const selectedMsg = messages.find(m => m.id === selectedId)

    return (
        <SplitView
            leftWidth="320px"
            left={
                <InboxList
                    messages={messages.filter(m => m.status !== 'dismissed')}
                    selectedId={selectedId}
                    onSelect={handleSelect}
                    onDismiss={id => DismissMailboxMessage(id).then(reload)}
                    onMarkAllRead={handleMarkAllRead}
                />
            }
            right={selectedMsg ? <InboxDetailView message={selectedMsg} onResolved={reload} /> : <EmptyDetail />}
        />
    )
}
```

---

## 3. InboxList（含分组）

```tsx
// components/inbox/InboxList.tsx
function groupByDate(messages: MailboxMessage[]) {
    const today = new Date().toDateString()
    const yesterday = new Date(Date.now() - 86400000).toDateString()
    return {
        today: messages.filter(m => new Date(m.createdAt).toDateString() === today),
        yesterday: messages.filter(m => new Date(m.createdAt).toDateString() === yesterday),
        earlier: messages.filter(m => {
            const d = new Date(m.createdAt).toDateString()
            return d !== today && d !== yesterday
        }),
    }
}

export function InboxList({ messages, selectedId, onSelect, onDismiss, onMarkAllRead }) {
    const groups = groupByDate(messages)

    return (
        <div className="flex flex-col h-full">
            <div className="flex items-center justify-between px-4 py-3 border-b border-neutral-800">
                <span className="text-sm font-medium">Inbox</span>
                <button onClick={onMarkAllRead} className="text-xs text-neutral-400 hover:text-white">
                    全部已读
                </button>
            </div>
            <div className="flex-1 overflow-y-auto">
                {[['今天', groups.today], ['昨天', groups.yesterday], ['更早', groups.earlier]].map(([label, items]) =>
                    (items as MailboxMessage[]).length > 0 && (
                        <div key={label as string}>
                            <div className="px-4 py-2 text-xs text-neutral-600">{label}</div>
                            {(items as MailboxMessage[]).map(msg => (
                                <InboxListItem key={msg.id} message={msg}
                                    selected={msg.id === selectedId}
                                    onSelect={() => onSelect(msg)}
                                    onDismiss={() => onDismiss(msg.id)} />
                            ))}
                        </div>
                    )
                )}
            </div>
        </div>
    )
}
```

---

## 4. InboxListItem

```tsx
// components/inbox/InboxListItem.tsx
const TYPE_ICONS: Record<string, string> = {
    approval_required: '⏳',
    permission_required: '🔐',
    run_failed: '❌',
    run_completed: '✅',
    workflow_deployed: '🚀',
}

export function InboxListItem({ message, selected, onSelect, onDismiss }) {
    return (
        <div onClick={onSelect}
            className={`relative flex items-start gap-3 px-4 py-3 cursor-pointer hover:bg-neutral-800
                ${selected ? 'bg-neutral-800' : ''}`}>
            {message.status === 'pending' && (
                <div className="absolute left-1 top-4 w-1.5 h-1.5 rounded-full bg-red-500" />
            )}
            <span className="text-base">{TYPE_ICONS[message.type] ?? '💬'}</span>
            <div className="flex-1 min-w-0">
                <p className="text-sm truncate">{message.title}</p>
                <p className="text-xs text-neutral-500 truncate">{message.body}</p>
            </div>
            <div className="flex flex-col items-end gap-1 flex-shrink-0">
                <span className="text-xs text-neutral-600">{formatRelativeTime(message.createdAt)}</span>
                <button onClick={e => { e.stopPropagation(); onDismiss() }}
                    className="text-xs text-neutral-600 hover:text-white">×</button>
            </div>
        </div>
    )
}
```

---

## 5. InboxBadge（侧边栏）

```tsx
// components/inbox/InboxBadge.tsx
export function InboxBadge() {
    const [count, setCount] = useState(0)

    useEffect(() => {
        const update = ({ unreadCount }: { unreadCount: number }) => setCount(unreadCount)
        return onEvent(EV.MailboxUpdated, update)
    }, [])

    if (count === 0) return null
    return (
        <div className="absolute -top-1 -right-1 min-w-[16px] h-4 px-1 bg-red-500 rounded-full
            text-[10px] text-white flex items-center justify-center">
            {count > 99 ? '99+' : count}
        </div>
    )
}
```

---

## 6. 验收测试

```
1. ListMailbox() 返回按 priority DESC, created_at DESC 排序的消息
2. MailboxUpdated 事件触发 → InboxList 刷新
3. 点击 pending 消息 → MarkMailboxRead() → 红点消失
4. onMarkAllRead → 所有 pending 变 read
5. onDismiss → 消息从列表消失
6. InboxBadge 正确显示未读数量，99+ 上限
```
