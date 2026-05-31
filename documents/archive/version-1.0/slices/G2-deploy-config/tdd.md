# G2 · 部署配置 — 技术设计文档

**切片**：G2  
**状态**：待 Review

---

## 1. WorkflowService 扩展

```go
// service/workflow.go
func (s *WorkflowService) SetStatus(id, status string) error {
    _, err := storage.DB().Exec(
        `UPDATE workflows SET status=?, updated_at=datetime('now') WHERE id=?`, status, id)
    return err
}
```

---

## 2. 部署前检查

```go
// service/workflow.go
type DeployCheck struct {
    OK     bool
    Errors []string
}

func (s *WorkflowService) PreDeployCheck(id string, toolSvc *ToolService, apiKeySvc *APIKeyService) DeployCheck {
    wf, _ := s.Get(id)
    var errs []string

    // 1. 编译检查
    result := compiler.Compile(wf.Definition, toolSvc)
    for _, e := range result.Errors {
        errs = append(errs, e.Message)
    }

    // 2. API Key 依赖检查
    var fd struct {
        Nodes []struct {
            Config map[string]any `json:"config"`
        } `json:"nodes"`
    }
    json.Unmarshal(wf.Definition, &fd)
    for _, n := range fd.Nodes {
        if toolName, ok := n.Config["tool_name"].(string); ok {
            tool, _ := toolSvc.GetByName(toolName)
            if tool != nil && tool.RequiresKey != "" {
                if !apiKeySvc.Has(tool.RequiresKey) {
                    errs = append(errs, "工具 "+toolName+" 需要 API Key: "+tool.RequiresKey)
                }
            }
        }
    }

    return DeployCheck{OK: len(errs) == 0, Errors: errs}
}
```

---

## 3. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("POST /api/workflows/{id}/set-ready", s.setWorkflowReady)
mux.HandleFunc("POST /api/workflows/{id}/deploy", s.deployWorkflow)
mux.HandleFunc("POST /api/workflows/{id}/pause", s.pauseWorkflow)
mux.HandleFunc("POST /api/workflows/{id}/archive", s.archiveWorkflow)
```

---

## 4. 前端：工具栏状态控件

```tsx
// WorkflowToolbar.tsx
const statusLabels: Record<string, string> = {
    draft: '草稿', ready: '就绪', deployed: '已部署', paused: '已暂停', archived: '已归档'
}

function WorkflowActions({ status, workflowId, onStatusChange }) {
    const [deployErrors, setDeployErrors] = useState<string[]>([])

    const handleDeploy = async () => {
        const result = await DeployWorkflow(workflowId)
        if (!result.ok) { setDeployErrors(result.errors); return }
        onStatusChange('deployed')
    }

    return (
        <div className="flex items-center gap-2">
            <span className="text-xs text-neutral-500 px-2 py-1 rounded bg-neutral-800">
                {statusLabels[status]}
            </span>
            {status === 'draft' && <>
                <button onClick={() => SetWorkflowReady(workflowId).then(() => onStatusChange('ready'))}
                    className="text-xs px-3 py-1 rounded bg-neutral-700">设为就绪</button>
            </>}
            {status === 'ready' && <>
                <button onClick={handleDeploy} className="text-xs px-3 py-1 rounded bg-blue-600">部署</button>
            </>}
            {status === 'deployed' && <>
                <button onClick={() => PauseWorkflow(workflowId).then(() => onStatusChange('paused'))}
                    className="text-xs px-3 py-1 rounded bg-neutral-700">暂停</button>
            </>}
            {status === 'paused' && <>
                <button onClick={handleDeploy} className="text-xs px-3 py-1 rounded bg-blue-600">重新部署</button>
            </>}

            {deployErrors.length > 0 && (
                <DeployErrorModal errors={deployErrors} onClose={() => setDeployErrors([])} />
            )}
        </div>
    )
}
```

---

## 5. 验收测试

```
1. SetWorkflowReady() → 编译通过 → status=ready
2. SetWorkflowReady() → 编译失败 → 返回错误，status 不变
3. DeployWorkflow() → PreDeployCheck 通过 → status=deployed → scheduler.Register 调用
4. DeployWorkflow() → 检查失败 → 返回 DeployCheck{OK:false, Errors:[...]}，前端弹出错误列表
5. PauseWorkflow() → status=paused → scheduler.Deregister 调用
6. 工具栏根据 status 显示对应按钮
```
