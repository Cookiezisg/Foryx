# I3 · Inbox 上下文视图 — 技术设计文档

**切片**：I3  
**状态**：待 Review

---

## 1. 目录结构

```
frontend/src/components/inbox/
├── InboxDetailView.tsx         # 分发路由（I2 已定义）
├── InboxContextCanvas.tsx      # 只读 Canvas
└── RunFailedDetailView.tsx     # run_failed 消息详情
```

---

## 2. InboxContextCanvas（只读模式）

```tsx
// components/inbox/InboxContextCanvas.tsx
export function InboxContextCanvas({ workflowId, highlightNodes }: {
    workflowId: string
    highlightNodes: Record<string, 'orange' | 'red' | 'green' | 'gray'>
}) {
    const [nodes, setNodes] = useNodesState([])
    const [edges, setEdges] = useEdgesState([])

    useEffect(() => {
        GetWorkflow(workflowId).then(wf => {
            const def = JSON.parse(wf.definition)
            // 注入高亮颜色
            const styledNodes = def.nodes.map((n: any) => ({
                ...toRFNode(n),
                data: { ...n.data, runStatus: highlightNodes[n.id] ?? 'idle' }
            }))
            setNodes(styledNodes)
            setEdges(def.edges.map(toRFEdge))
        })
    }, [workflowId, highlightNodes])

    return (
        <div className="relative h-full w-full">
            {/* 只读标识 */}
            <div className="absolute top-2 right-2 z-10 flex items-center gap-2">
                <span className="text-xs text-neutral-500 px-2 py-1 bg-neutral-900 rounded">只读</span>
                <button onClick={() => navigateToAsset(workflowId)}
                    className="text-xs px-2 py-1 bg-neutral-800 rounded hover:bg-neutral-700">
                    打开完整编辑
                </button>
            </div>

            <ReactFlow
                nodes={nodes}
                edges={edges}
                nodeTypes={NODE_TYPES}
                nodesDraggable={false}
                nodesConnectable={false}
                elementsSelectable={false}
                panOnDrag={true}
                zoomOnScroll={true}
                fitView
            >
                <Background color="#333" gap={16} />
                <MiniMap nodeColor={nodeColor} />
            </ReactFlow>
        </div>
    )
}
```

---

## 3. RunFailedDetailView

```tsx
// components/inbox/RunFailedDetailView.tsx
export function RunFailedDetailView({ message }: { message: MailboxMessage }) {
    const payload = JSON.parse(message.payload) as {
        runId: string
        workflowId: string
        nodeId: string
    }
    const [runDetail, setRunDetail] = useState<RunDetail | null>(null)

    useEffect(() => {
        GetRunDetail(payload.runId).then(setRunDetail)
    }, [payload.runId])

    // 构建高亮 map：失败节点红色，成功节点绿色，跳过灰色
    const highlightNodes = useMemo(() => {
        if (!runDetail) return {}
        const map: Record<string, 'red' | 'green' | 'gray'> = {}
        Object.entries(runDetail.nodeResults).forEach(([nodeId, result]) => {
            if (result.status === 'failed') map[nodeId] = 'red'
            else if (result.status === 'success') map[nodeId] = 'green'
            else if (result.status === 'skipped') map[nodeId] = 'gray'
        })
        return map
    }, [runDetail])

    return (
        <div className="flex flex-col h-full">
            {/* 上方：消息详情 */}
            <div className="p-4 border-b border-neutral-800 space-y-2">
                <div className="flex items-center gap-2 text-red-400">
                    <span>❌</span>
                    <span className="text-sm font-medium">{message.title}</span>
                </div>
                <p className="text-xs text-neutral-400">{message.body}</p>
                {runDetail && (
                    <div className="text-xs text-neutral-500">
                        失败节点：{payload.nodeId} ·{' '}
                        {runDetail.nodeResults[payload.nodeId]?.error}
                    </div>
                )}
                <button onClick={() => OpenRepairConversation(payload.workflowId)}
                    className="text-xs px-3 py-1 rounded bg-red-900 text-red-300 hover:bg-red-800 mt-2">
                    通过对话修复
                </button>
            </div>

            {/* 下方：Canvas 上下文 */}
            <div className="flex-1">
                <InboxContextCanvas
                    workflowId={payload.workflowId}
                    highlightNodes={highlightNodes}
                />
            </div>
        </div>
    )
}
```

---

## 4. ApprovalDetailView 集成（I2 扩展）

```tsx
// ApprovalDetailView.tsx 下方追加 Canvas
return (
    <div className="flex flex-col h-full">
        {/* 上方审批操作区（I2 已定义）*/}
        <div className="flex-shrink-0 p-6 ...">...</div>

        {/* 下方 Canvas */}
        {payload.workflowId && (
            <div className="flex-1 border-t border-neutral-800">
                <InboxContextCanvas
                    workflowId={payload.workflowId}
                    highlightNodes={{ [runCtx?.nodeId ?? '']: 'orange' }}
                />
            </div>
        )}
    </div>
)
```

---

## 5. 导航到资产页面

```tsx
// 全局导航 helper（复用 App.tsx 的 tab 切换逻辑）
function navigateToAsset(workflowId: string) {
    // 通过全局状态或事件切换到资产页面并选中该工作流
    emitNavigate({ tab: 'assets', assetId: workflowId, assetType: 'workflow' })
}
```

---

## 6. 验收测试

```
1. run_failed 消息详情：上方错误说明 + 下方 Canvas，失败节点红色
2. approval_required：上方审批按钮 + 下方 Canvas，等待节点橙色
3. Canvas nodesDraggable=false，拖拽无响应
4. 点击"打开完整编辑" → 切换到资产页面对应工作流
5. 成功节点绿色，跳过节点灰色，失败节点红色的颜色逻辑正确
6. workflowId 为空的消息 → 不渲染 Canvas
```
