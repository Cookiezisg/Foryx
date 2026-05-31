# H1 · 权限门控 — 技术设计文档

**切片**：H1  
**状态**：待 Review

---

## 1. 数据库

```sql
-- 010_permissions.sql
CREATE TABLE IF NOT EXISTS tool_permissions (
    tool_name   TEXT PRIMARY KEY,
    granted     BOOLEAN NOT NULL DEFAULT 0,  -- write 级别：始终允许
    granted_at  DATETIME
);
```

---

## 2. 权限级别解析

工具元数据中已有 `permission` 字段（D6 定义，`@permission read|write|execute`），权限门控在工具节点执行前读取。

```go
// runner/node_runners/tool.go

type PermissionLevel string

const (
    PermRead    PermissionLevel = "read"
    PermWrite   PermissionLevel = "write"
    PermExecute PermissionLevel = "execute"
)
```

---

## 3. PermissionGate

```go
// internal/permission/gate.go
type Gate struct {
    toolSvc    *service.ToolService
    permSvc    *service.PermissionService
    approvalSvc *service.ApprovalService
    bridge     *events.Bridge
}

type GateResult struct {
    Allowed bool
    Params  map[string]any // 可能被用户修改的参数
}

func (g *Gate) Check(ctx context.Context, toolName string, params map[string]any, runCtx *runner.RunContext) (GateResult, error) {
    tool, _ := g.toolSvc.GetByName(toolName)
    if tool == nil { return GateResult{Allowed: false}, fmt.Errorf("工具不存在: %s", toolName) }

    switch PermissionLevel(tool.Permission) {
    case PermRead, "":
        return GateResult{Allowed: true, Params: params}, nil

    case PermWrite:
        granted, _ := g.permSvc.IsGranted(toolName)
        if granted { return GateResult{Allowed: true, Params: params}, nil }

        // 发出权限确认请求，通过 UI 确认
        result, err := g.requestPermission(ctx, toolName, params, runCtx, false)
        return result, err

    case PermExecute:
        // 每次都要确认，路由到 Inbox Approval
        return g.requestPermission(ctx, toolName, params, runCtx, true)
    }

    return GateResult{Allowed: true, Params: params}, nil
}

func (g *Gate) requestPermission(ctx context.Context, toolName string, params map[string]any, runCtx *runner.RunContext, alwaysAsk bool) (GateResult, error) {
    approval, _ := g.approvalSvc.Create(service.CreateApprovalInput{
        RunID:      runCtx.RunID,
        NodeID:     runCtx.CurrentNodeID,
        WorkflowID: runCtx.WorkflowID,
        Title:      "权限确认：" + toolName,
        Message:    fmt.Sprintf("工具 %s 需要执行权限", toolName),
        Params:     params,
        RejectMode: "stop",
        ExpiresAt:  time.Now().Add(24 * time.Hour),
    })

    g.bridge.Emit(events.PermissionRequired, map[string]any{
        "approvalId": approval.ID,
        "toolName":   toolName,
        "alwaysAsk":  alwaysAsk,
    })

    result, err := g.approvalSvc.Wait(ctx, approval.ID)
    if err != nil || result.Status != "approved" {
        return GateResult{Allowed: false}, nil
    }

    // write 级别：记住"始终允许"选择
    if !alwaysAsk {
        g.permSvc.Grant(toolName)
    }

    finalParams := params
    if result.ModifiedParams != nil { finalParams = result.ModifiedParams }
    return GateResult{Allowed: true, Params: finalParams}, nil
}
```

---

## 4. PermissionService

```go
// service/permission.go
type PermissionService struct{}

func (s *PermissionService) IsGranted(toolName string) (bool, error) {
    var granted bool
    err := storage.DB().QueryRow(
        `SELECT granted FROM tool_permissions WHERE tool_name = ?`, toolName).Scan(&granted)
    if err == sql.ErrNoRows { return false, nil }
    return granted, err
}

func (s *PermissionService) Grant(toolName string) error {
    _, err := storage.DB().Exec(`
        INSERT INTO tool_permissions (tool_name, granted, granted_at)
        VALUES (?, 1, datetime('now'))
        ON CONFLICT(tool_name) DO UPDATE SET granted=1, granted_at=datetime('now')
    `, toolName)
    return err
}

func (s *PermissionService) Revoke(toolName string) error {
    _, err := storage.DB().Exec(
        `UPDATE tool_permissions SET granted=0 WHERE tool_name=?`, toolName)
    return err
}

func (s *PermissionService) List() ([]*ToolPermission, error) {
    rows, _ := storage.DB().Query(
        `SELECT tool_name, granted, granted_at FROM tool_permissions WHERE granted=1`)
    // ... scan ...
}
```

---

## 5. 集成到工具节点执行

```go
// runner/node_runners/tool.go
func (r *ToolRunner) Run(ctx context.Context, node FlowNode, input map[string]any, rc *RunContext) (any, error) {
    toolName := node.Config["tool_name"].(string)

    gateResult, err := r.gate.Check(ctx, toolName, input, rc)
    if err != nil { return nil, err }
    if !gateResult.Allowed {
        return nil, &RunError{Type: "permission_denied", NodeID: node.ID, Message: "权限被拒绝"}
    }

    return r.sandbox.Execute(ctx, toolName, gateResult.Params)
}
```

---

## 6. HTTP API 路由（设置页）

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/permissions", s.listGrantedPermissions)
mux.HandleFunc("DELETE /api/permissions/{toolName}", s.revokePermission)
```

---

## 7. 验收测试

```
1. read 工具 → Gate.Check() 直接返回 Allowed=true
2. write 工具首次 → 发出 PermissionRequired 事件 → 等待 Approval
3. 批准 write 工具 → permSvc.Grant() → 下次直接通过
4. execute 工具 → 每次都发出 PermissionRequired，不记住
5. 拒绝 → GateResult{Allowed:false} → RunError{Type:"permission_denied"}
6. RevokePermission() → 下次 write 工具重新需要确认
```
