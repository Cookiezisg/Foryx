# F3 · Mailbox 审批 — 技术设计文档

**切片**：F3  
**状态**：待 Review

---

## 1. 数据库

```sql
-- 009_approvals.sql
CREATE TABLE IF NOT EXISTS approvals (
    id          TEXT PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES runs(id),
    node_id     TEXT NOT NULL,
    workflow_id TEXT NOT NULL,
    title       TEXT NOT NULL,
    message     TEXT NOT NULL,
    params      JSON,           -- 节点当时的参数（可被用户修改）
    status      TEXT NOT NULL DEFAULT 'pending'
                CHECK(status IN ('pending','approved','rejected','timeout')),
    reject_mode TEXT NOT NULL DEFAULT 'skip'
                CHECK(reject_mode IN ('skip','stop','notify')),
    expires_at  DATETIME NOT NULL,
    decided_at  DATETIME,
    modified_params JSON        -- 修改后批准时用户填写的参数
);
```

---

## 2. Approval 节点执行逻辑

```go
// runner/node_runners/approval.go

type ApprovalRunner struct {
    approvalSvc *service.ApprovalService
    bridge      *events.Bridge
}

func (r *ApprovalRunner) Run(ctx context.Context, node FlowNode, input map[string]any, rc *RunContext) (any, error) {
    config := node.Config
    title := resolveString(config["title"].(string), rc.NodeOutputs)
    message := resolveString(config["message"].(string), rc.NodeOutputs)
    rejectMode, _ := config["reject_mode"].(string)
    if rejectMode == "" { rejectMode = "skip" }

    timeoutHours := 24.0
    if v, ok := config["timeout_hours"].(float64); ok { timeoutHours = v }

    approval, _ := r.approvalSvc.Create(service.CreateApprovalInput{
        RunID:      rc.RunID,
        NodeID:     node.ID,
        WorkflowID: rc.WorkflowID,
        Title:      title,
        Message:    message,
        Params:     input,
        RejectMode: rejectMode,
        ExpiresAt:  time.Now().Add(time.Duration(timeoutHours * float64(time.Hour))),
    })

    r.bridge.Emit(events.ApprovalPending, map[string]any{
        "approvalId": approval.ID,
        "runId":      rc.RunID,
        "nodeId":     node.ID,
        "workflowId": rc.WorkflowID,
    })

    // 阻塞等待审批（使用 channel + 超时）
    result, err := r.approvalSvc.Wait(ctx, approval.ID)
    if err != nil { return nil, err }

    if result.Status == "rejected" {
        switch rejectMode {
        case "stop":
            return nil, fmt.Errorf("用户拒绝了操作，工作流停止")
        case "skip":
            return map[string]any{"skipped": true}, nil
        case "notify":
            // 通知发送后继续
            return map[string]any{"rejected": true}, nil
        }
    }

    // approved or approved with modified params
    finalParams := input
    if result.ModifiedParams != nil { finalParams = result.ModifiedParams }
    return finalParams, nil
}
```

---

## 3. ApprovalService

```go
// service/approval.go
type ApprovalService struct {
    waiters map[string]chan ApprovalResult
    mu      sync.Mutex
}

func (s *ApprovalService) Wait(ctx context.Context, approvalID string) (ApprovalResult, error) {
    ch := make(chan ApprovalResult, 1)
    s.mu.Lock()
    s.waiters[approvalID] = ch
    s.mu.Unlock()

    approval, _ := s.Get(approvalID)
    timeout := time.Until(approval.ExpiresAt)

    select {
    case result := <-ch:
        return result, nil
    case <-time.After(timeout):
        s.Update(approvalID, "timeout", nil)
        return ApprovalResult{Status: "timeout"}, nil
    case <-ctx.Done():
        return ApprovalResult{}, ctx.Err()
    }
}

func (s *ApprovalService) Decide(approvalID, status string, modifiedParams map[string]any) error {
    s.Update(approvalID, status, modifiedParams)
    s.mu.Lock()
    ch, ok := s.waiters[approvalID]
    delete(s.waiters, approvalID)
    s.mu.Unlock()
    if ok {
        ch <- ApprovalResult{Status: status, ModifiedParams: modifiedParams}
    }
    return nil
}
```

---

## 4. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/approvals/pending", s.getPendingApprovals)
mux.HandleFunc("POST /api/approvals/{id}/approve", s.approveAction)
mux.HandleFunc("POST /api/approvals/{id}/reject", s.rejectAction)
mux.HandleFunc("POST /api/approvals/{id}/approve-modified", s.approveWithModifiedParams)
```

---

## 5. 前端：审批卡片（I2 会进一步完善）

```tsx
// ApprovalCard.tsx
export function ApprovalCard({ approval }: { approval: Approval }) {
    const [editMode, setEditMode] = useState(false)
    const [params, setParams] = useState(approval.params)

    return (
        <div className="bg-neutral-800 rounded-xl p-4 space-y-3">
            <div className="flex items-center gap-2">
                <span className="text-orange-400">⏳</span>
                <span className="text-sm font-medium">{approval.title}</span>
            </div>
            <p className="text-xs text-neutral-400">{approval.message}</p>

            {editMode && Object.entries(params).map(([k, v]) => (
                <div key={k}>
                    <label className="text-xs text-neutral-500 mb-1 block">{k}</label>
                    <input value={String(v)} onChange={e => setParams(p => ({ ...p, [k]: e.target.value }))}
                        className="w-full px-2 py-1 bg-neutral-700 rounded text-sm" />
                </div>
            ))}

            <div className="flex gap-2">
                <button onClick={() => RejectAction(approval.id)}
                    className="px-3 py-1 text-xs rounded bg-neutral-700">拒绝</button>
                {!editMode && (
                    <button onClick={() => setEditMode(true)}
                        className="px-3 py-1 text-xs rounded bg-neutral-700">修改后批准</button>
                )}
                <button onClick={() => editMode
                    ? ApproveWithModifiedParams(approval.id, params)
                    : ApproveAction(approval.id)}
                    className="px-3 py-1 text-xs rounded bg-blue-600">
                    {editMode ? '确认修改并批准' : '批准'}
                </button>
            </div>
        </div>
    )
}
```

---

## 6. 验收测试

```
1. ApprovalRunner.Run() 阻塞 → 发出 ApprovalPending 事件 → Inbox 出现卡片
2. ApproveAction() → Wait() channel 收到结果 → 节点继续执行
3. RejectAction() with reject_mode=skip → 节点返回 {skipped:true}，不报错
4. RejectAction() with reject_mode=stop → 节点返回 error，工作流停止
5. 超时：Wait() 自动返回 timeout → 按 reject_mode 处理
6. ApproveWithModifiedParams() → 节点使用修改后的参数继续
```
