# I2 · 审批工作流 — 技术设计文档

**切片**：I2  
**状态**：待 Review

---

## 1. 审批详情组件

```tsx
// components/inbox/ApprovalDetailView.tsx
export function ApprovalDetailView({ message, onResolved }: {
    message: MailboxMessage
    onResolved: () => void
}) {
    const payload = JSON.parse(message.payload) as {
        approvalId: string
        workflowId?: string
        toolName?: string
    }
    const [approval, setApproval] = useState<Approval | null>(null)
    const [editMode, setEditMode] = useState(false)
    const [params, setParams] = useState<Record<string, any>>({})
    const [result, setResult] = useState<string | null>(null)

    useEffect(() => {
        if (payload.approvalId) {
            GetApproval(payload.approvalId).then(a => {
                setApproval(a)
                setParams(a.params ?? {})
            })
        }
    }, [payload.approvalId])

    if (!approval) return null
    if (result) return <ResolvedBadge status={result} />

    const handleApprove = async () => {
        if (editMode) {
            await ApproveWithModifiedParams(approval.id, params)
        } else {
            await ApproveAction(approval.id)
        }
        await MailboxService.Resolve(message.id)
        setResult('approved')
        onResolved()
    }

    const handleReject = async () => {
        await RejectAction(approval.id)
        await DismissMailboxMessage(message.id)
        setResult('rejected')
        onResolved()
    }

    const isPermission = message.type === 'permission_required'

    return (
        <div className="p-6 space-y-5">
            <div className="flex items-center gap-2">
                <span className="text-xl">{isPermission ? '🔐' : '⏳'}</span>
                <h2 className="text-base font-semibold">{message.title}</h2>
            </div>

            <p className="text-sm text-neutral-300">{approval.message}</p>

            <div className="space-y-2">
                <p className="text-xs text-neutral-500 font-medium">参数</p>
                {Object.entries(params).map(([k, v]) => (
                    <div key={k} className="flex gap-2 items-start">
                        <span className="text-xs text-neutral-500 w-24 flex-shrink-0">{k}</span>
                        {editMode ? (
                            <input value={String(v)}
                                onChange={e => setParams(p => ({ ...p, [k]: e.target.value }))}
                                className="flex-1 px-2 py-1 bg-neutral-800 rounded text-sm" />
                        ) : (
                            <span className="text-sm text-neutral-300 break-all">{String(v)}</span>
                        )}
                    </div>
                ))}
            </div>

            <div className="flex gap-2 justify-end pt-4 border-t border-neutral-800">
                <button onClick={handleReject}
                    className="px-4 py-2 text-sm rounded bg-neutral-700 hover:bg-neutral-600">
                    拒绝
                </button>
                {!editMode && (
                    <button onClick={() => setEditMode(true)}
                        className="px-4 py-2 text-sm rounded bg-neutral-700 hover:bg-neutral-600">
                        修改后批准
                    </button>
                )}
                <button onClick={isPermission && !editMode ? handleGrantAlways : handleApprove}
                    className="px-4 py-2 text-sm rounded bg-blue-600 hover:bg-blue-500">
                    {editMode ? '确认修改并批准' : isPermission ? '始终允许' : '批准'}
                </button>
            </div>
        </div>
    )

    async function handleGrantAlways() {
        await ApproveAction(approval!.id)
        await GrantPermission(payload.toolName!)
        await DismissMailboxMessage(message.id)
        setResult('granted')
        onResolved()
    }
}
```

---

## 2. 已解决徽章

```tsx
function ResolvedBadge({ status }: { status: string }) {
    const config = {
        approved: { icon: '✅', label: '已批准', color: 'text-green-400' },
        rejected: { icon: '❌', label: '已拒绝', color: 'text-red-400' },
        granted:  { icon: '🔓', label: '已授权（始终允许）', color: 'text-blue-400' },
    }[status] ?? { icon: '✓', label: '已处理', color: 'text-neutral-400' }

    return (
        <div className="flex flex-col items-center justify-center h-full gap-3 text-center">
            <span className="text-4xl">{config.icon}</span>
            <p className={`text-sm font-medium ${config.color}`}>{config.label}</p>
        </div>
    )
}
```

---

## 3. HTTP API 路由扩展

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/approvals/{id}", s.getApproval)
mux.HandleFunc("POST /api/permissions/{toolName}/grant", s.grantPermission)
mux.HandleFunc("PATCH /api/mailbox/{id}/resolve", s.resolveMailboxMessage)
```

---

## 4. InboxDetailView 分发

```tsx
// components/inbox/InboxDetailView.tsx
export function InboxDetailView({ message, onResolved }) {
    switch (message.type) {
    case 'approval_required':
    case 'permission_required':
        return <ApprovalDetailView message={message} onResolved={onResolved} />
    case 'run_failed':
        return <RunFailedDetailView message={message} />
    default:
        return <GenericDetailView message={message} />
    }
}
```

---

## 5. 验收测试

```
1. 点击 approval_required 消息 → ApprovalDetailView 显示参数
2. 点击"批准" → ApproveAction() + ResolveMailboxMessage() → 显示 ResolvedBadge
3. 点击"拒绝" → RejectAction() + DismissMailboxMessage() → 显示 ResolvedBadge
4. 点击"修改后批准" → 参数可编辑 → 修改 → ApproveWithModifiedParams()
5. permission_required → "始终允许" → ApproveAction() + GrantPermission()
6. 已解决的 approval → 显示正确的 ResolvedBadge
```
