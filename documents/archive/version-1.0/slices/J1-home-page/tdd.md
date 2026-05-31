# J1 · Home Page — 技术设计文档

**切片**：J1  
**状态**：待 Review

---

## 1. 目录结构

```
frontend/src/
├── pages/HomePage.tsx
└── components/home/
    ├── StatsPanel.tsx       # 状态摘要
    └── RecentActivity.tsx   # 最近活动列表
```

---

## 2. HomePage

```tsx
// pages/HomePage.tsx
export function HomePage() {
    const [conversationId, setConversationId] = useState<string | null>(null)

    // 如果没有默认对话，创建一个（不绑定资产）
    useEffect(() => {
        CreateConversation().then(conv => setConversationId(conv.id))
    }, [])

    return (
        <SplitView
            leftWidth="320px"
            left={<HomeSidebar />}
            right={
                conversationId
                    ? <ChatInterface conversationId={conversationId} placeholder="和 Forgify 助手聊聊，告诉我你想自动化什么..." />
                    : <div className="h-full flex items-center justify-center text-neutral-500">加载中...</div>
            }
        />
    )
}

function HomeSidebar() {
    return (
        <div className="flex flex-col gap-4 p-4">
            <StatsPanel />
            <RecentActivity />
        </div>
    )
}
```

---

## 3. StatsPanel

```tsx
// components/home/StatsPanel.tsx
export function StatsPanel() {
    const [stats, setStats] = useState({ todayRuns: 0, deployedWorkflows: 0, pendingApprovals: 0 })

    useEffect(() => {
        GetHomeStats().then(setStats)
    }, [])

    // 刷新：工作流运行或邮件更新时
    useEffect(() => {
        const off1 = onEvent(EV.RunCompleted, () => GetHomeStats().then(setStats))
        const off2 = onEvent(EV.RunFailed, () => GetHomeStats().then(setStats))
        const off3 = onEvent(EV.MailboxUpdated, () => GetHomeStats().then(setStats))
        return () => { off1(); off2(); off3() }
    }, [])

    return (
        <div className="space-y-2">
            <p className="text-xs text-neutral-500 font-medium">今日概览</p>
            <div className="grid grid-cols-1 gap-1">
                <StatRow label="今日运行" value={stats.todayRuns} />
                <StatRow label="已部署工作流" value={stats.deployedWorkflows} />
                <StatRow label="待审批" value={stats.pendingApprovals} highlight={stats.pendingApprovals > 0}
                    onClick={() => navigateTo('inbox')} />
            </div>
        </div>
    )
}

function StatRow({ label, value, highlight, onClick }: {
    label: string; value: number; highlight?: boolean; onClick?: () => void
}) {
    return (
        <div onClick={onClick} className={`flex justify-between items-center px-3 py-2 rounded
            ${onClick ? 'cursor-pointer hover:bg-neutral-800' : ''}`}>
            <span className="text-xs text-neutral-400">{label}</span>
            <span className={`text-sm font-medium ${highlight ? 'text-red-400' : 'text-white'}`}>{value}</span>
        </div>
    )
}
```

---

## 4. RecentActivity

```tsx
// components/home/RecentActivity.tsx
export function RecentActivity() {
    const [runs, setRuns] = useState<RecentRun[]>([])

    useEffect(() => {
        GetRecentRuns(10).then(setRuns)
    }, [])

    useEffect(() => {
        const off = onEvent(EV.RunCompleted, () => GetRecentRuns(10).then(setRuns))
        return off
    }, [])

    return (
        <div className="space-y-2">
            <p className="text-xs text-neutral-500 font-medium">最近活动</p>
            <div className="space-y-1">
                {runs.map(run => (
                    <button key={run.id}
                        onClick={() => navigateToAsset(run.workflowId, run.id)}
                        className="w-full flex items-center gap-2 px-2 py-2 rounded text-left hover:bg-neutral-800">
                        <span className="text-sm">
                            {run.status === 'success' ? '✅' : run.status === 'failed' ? '❌' : '⏳'}
                        </span>
                        <span className="text-xs text-neutral-300 flex-1 truncate">{run.workflowName}</span>
                        <span className="text-xs text-neutral-600">{formatRelativeTime(run.startedAt)}</span>
                    </button>
                ))}
                {runs.length === 0 && (
                    <p className="text-xs text-neutral-600 px-2">还没有运行记录</p>
                )}
            </div>
        </div>
    )
}
```

---

## 5. Go 层：HomeStats

```go
// service/stats.go
type HomeStats struct {
    TodayRuns         int `json:"todayRuns"`
    DeployedWorkflows int `json:"deployedWorkflows"`
    PendingApprovals  int `json:"pendingApprovals"`
}

func (s *StatsService) GetHomeStats() (*HomeStats, error) {
    stats := &HomeStats{}
    storage.DB().QueryRow(
        `SELECT COUNT(*) FROM runs WHERE started_at >= date('now')`).Scan(&stats.TodayRuns)
    storage.DB().QueryRow(
        `SELECT COUNT(*) FROM workflows WHERE status='deployed'`).Scan(&stats.DeployedWorkflows)
    storage.DB().QueryRow(
        `SELECT COUNT(*) FROM mailbox_messages WHERE status='pending'`).Scan(&stats.PendingApprovals)
    return stats, nil
}

type RecentRun struct {
    ID           string    `json:"id"`
    WorkflowID   string    `json:"workflowId"`
    WorkflowName string    `json:"workflowName"`
    Status       string    `json:"status"`
    StartedAt    time.Time `json:"startedAt"`
}

func (s *StatsService) GetRecentRuns(limit int) ([]*RecentRun, error) {
    rows, _ := storage.DB().Query(`
        SELECT r.id, r.workflow_id, w.name, r.status, r.started_at
        FROM runs r JOIN workflows w ON r.workflow_id = w.id
        ORDER BY r.started_at DESC LIMIT ?`, limit)
    // ... scan ...
}
```

---

## 6. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/home/stats", s.getHomeStats)
mux.HandleFunc("GET /api/home/recent-runs", s.getRecentRuns)
```

---

## 7. 验收测试

```
1. GetHomeStats() 返回正确的今日运行、已部署、待审批数量
2. GetRecentRuns(10) 返回最新 10 条（跨工作流）
3. StatsPanel 实时更新：RunCompleted 事件触发后数字刷新
4. 待审批 > 0 时显示红色，点击跳转 Inbox
5. RecentActivity 点击 → navigateToAsset 调用正确
6. ChatInterface 在 Home Page 正常工作
```
