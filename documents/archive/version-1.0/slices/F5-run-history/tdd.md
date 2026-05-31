# F5 · 运行历史 — 技术设计文档

**切片**：F5  
**状态**：待 Review

---

## 1. 数据库

运行记录已在 F2（`runs` + `run_node_results` 表）定义，本切片在其基础上添加自动清理：

```sql
-- 在 RunService.Create() 中触发清理
-- 保留最新 N 条（N 可配置，默认 100）
DELETE FROM runs
WHERE workflow_id = ?
  AND id NOT IN (
      SELECT id FROM runs WHERE workflow_id = ?
      ORDER BY started_at DESC LIMIT ?
  );
```

---

## 2. RunService 扩展

```go
// service/run.go
func (s *RunService) ListByWorkflow(workflowID string, limit int) ([]*Run, error) {
    rows, err := storage.DB().Query(`
        SELECT id, status, trigger_type, params, started_at, finished_at, error
        FROM runs WHERE workflow_id = ?
        ORDER BY started_at DESC LIMIT ?`, workflowID, limit)
    // ... scan rows ...
}

func (s *RunService) GetWithNodeResults(runID string) (*RunDetail, error) {
    run, _ := s.Get(runID)
    rows, _ := storage.DB().Query(`
        SELECT node_id, status, input, output, started_at, finished_at, error
        FROM run_node_results WHERE run_id = ?`, runID)
    // ... aggregate into RunDetail ...
}

func (s *RunService) Prune(workflowID string, keepCount int) error {
    _, err := storage.DB().Exec(`
        DELETE FROM runs WHERE workflow_id = ? AND id NOT IN (
            SELECT id FROM runs WHERE workflow_id = ? ORDER BY started_at DESC LIMIT ?
        )`, workflowID, workflowID, keepCount)
    return err
}
```

---

## 3. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/workflows/{id}/runs", s.listRunHistory)
mux.HandleFunc("GET /api/runs/{runId}/detail", s.getRunDetail)
```

---

## 4. 前端：运行历史面板

```tsx
// RunHistoryPanel.tsx
export function RunHistoryPanel({ workflowId, onSelect }: {
    workflowId: string
    onSelect: (runId: string) => void
}) {
    const [runs, setRuns] = useState<Run[]>([])

    useEffect(() => {
        fetch(`http://127.0.0.1:${port}/api/workflows/${workflowId}/runs`).then(r => r.json()).then(setRuns)
    }, [workflowId])

    return (
        <div className="border-t border-neutral-800 bg-neutral-950 h-48 overflow-y-auto">
            <div className="px-4 py-2 text-xs text-neutral-500 font-medium border-b border-neutral-800">运行历史</div>
            {runs.map(run => (
                <button key={run.id} onClick={() => onSelect(run.id)}
                    className="w-full flex items-center gap-3 px-4 py-2 text-xs hover:bg-neutral-800 text-left">
                    <span>{run.status === 'success' ? '✅' : run.status === 'failed' ? '❌' : '⏳'}</span>
                    <span className="text-neutral-400">{run.triggerType === 'manual' ? '手动运行' : '定时运行'}</span>
                    <span className="text-neutral-600">{formatTime(run.startedAt)}</span>
                    <span className="text-neutral-600 ml-auto">{formatDuration(run.startedAt, run.finishedAt)}</span>
                    {run.status === 'failed' && <span className="text-red-400 truncate max-w-[120px]">{run.error}</span>}
                </button>
            ))}
        </div>
    )
}
```

---

## 5. 历史回放模式

```tsx
// WorkflowCanvas.tsx — 历史回放
const [replayRunId, setReplayRunId] = useState<string | null>(null)
const [replayData, setReplayData] = useState<RunDetail | null>(null)

useEffect(() => {
    if (!replayRunId) return
    GetRunDetail(replayRunId).then(setReplayData)
}, [replayRunId])

// 历史回放时节点颜色
const nodesWithReplay = useMemo(() => {
    if (!replayData) return nodes
    return nodes.map(n => {
        const nodeResult = replayData.nodeResults[n.id]
        return { ...n, data: { ...n.data, runStatus: nodeResult?.status } }
    })
}, [nodes, replayData])

// 历史回放时边高亮（已执行的路径）
const edgesWithReplay = useMemo(() => {
    if (!replayData) return edges
    const executedNodes = new Set(Object.keys(replayData.nodeResults))
    return edges.map(e => ({
        ...e,
        style: executedNodes.has(e.source) && executedNodes.has(e.target)
            ? { stroke: '#22c55e', strokeWidth: 2 }
            : { stroke: '#404040' }
    }))
}, [edges, replayData])
```

---

## 6. 验收测试

```
1. ListRunHistory() 返回按时间倒序排列的运行记录
2. 运行完成后列表自动刷新（监听 RunCompleted 事件）
3. 点击历史记录 → GetRunDetail() → Canvas 节点颜色反映该次状态
4. 历史回放中点击节点 → 显示该次运行的输入/输出
5. 超过 100 条时 Prune() 被调用，旧记录删除
6. 历史回放模式下工具栏显示"历史回放"提示，运行按钮不可用
```
