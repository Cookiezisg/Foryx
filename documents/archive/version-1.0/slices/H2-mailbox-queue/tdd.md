# H2 · Mailbox 队列 — 技术设计文档

**切片**：H2  
**状态**：待 Review

---

## 1. 数据库

```sql
-- 011_mailbox.sql
CREATE TABLE IF NOT EXISTS mailbox_messages (
    id           TEXT PRIMARY KEY,
    type         TEXT NOT NULL,   -- approval_required, permission_required, run_failed, ...
    priority     INTEGER NOT NULL DEFAULT 1,  -- 1=low, 2=medium, 3=high
    title        TEXT NOT NULL,
    body         TEXT NOT NULL,
    payload      JSON,            -- 关联数据（approval_id, run_id, workflow_id 等）
    status       TEXT NOT NULL DEFAULT 'pending'
                 CHECK(status IN ('pending','read','resolved','dismissed')),
    created_at   DATETIME DEFAULT (datetime('now')),
    updated_at   DATETIME DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_mailbox_status ON mailbox_messages(status);
CREATE INDEX IF NOT EXISTS idx_mailbox_created ON mailbox_messages(created_at DESC);
```

---

## 2. MailboxService

```go
// service/mailbox.go
package service

type MailboxMessage struct {
    ID        string          `json:"id"`
    Type      string          `json:"type"`
    Priority  int             `json:"priority"`
    Title     string          `json:"title"`
    Body      string          `json:"body"`
    Payload   json.RawMessage `json:"payload"`
    Status    string          `json:"status"`
    CreatedAt time.Time       `json:"createdAt"`
}

type MailboxService struct{}

func (s *MailboxService) Send(msg MailboxMessage) error {
    // 同类型同工作流消息合并（upsert 最新）
    if msg.Type == "run_failed" {
        var payload struct{ WorkflowID string `json:"workflowId"` }
        json.Unmarshal(msg.Payload, &payload)
        storage.DB().Exec(`
            DELETE FROM mailbox_messages
            WHERE type='run_failed' AND json_extract(payload,'$.workflowId')=?
            AND status IN ('pending','read')`, payload.WorkflowID)
    }

    msg.ID = uuid.NewString()
    _, err := storage.DB().Exec(`
        INSERT INTO mailbox_messages (id, type, priority, title, body, payload, status)
        VALUES (?, ?, ?, ?, ?, ?, 'pending')`,
        msg.ID, msg.Type, msg.Priority, msg.Title, msg.Body, string(msg.Payload))
    return err
}

func (s *MailboxService) List(statusFilter string) ([]*MailboxMessage, error) {
    q := `SELECT id, type, priority, title, body, payload, status, created_at
          FROM mailbox_messages`
    if statusFilter != "" {
        q += ` WHERE status = '` + statusFilter + `'`
    }
    q += ` ORDER BY priority DESC, created_at DESC LIMIT 100`
    // ... scan rows ...
}

func (s *MailboxService) MarkRead(id string) error {
    _, err := storage.DB().Exec(
        `UPDATE mailbox_messages SET status='read', updated_at=datetime('now') WHERE id=?`, id)
    return err
}

func (s *MailboxService) Resolve(id string) error {
    _, err := storage.DB().Exec(
        `UPDATE mailbox_messages SET status='resolved', updated_at=datetime('now') WHERE id=?`, id)
    return err
}

func (s *MailboxService) Dismiss(id string) error {
    _, err := storage.DB().Exec(
        `UPDATE mailbox_messages SET status='dismissed', updated_at=datetime('now') WHERE id=?`, id)
    return err
}

func (s *MailboxService) UnreadCount() (int, error) {
    var count int
    storage.DB().QueryRow(
        `SELECT COUNT(*) FROM mailbox_messages WHERE status='pending'`).Scan(&count)
    return count, nil
}
```

---

## 3. 消息发送时机

```go
// 各模块在关键事件后调用 mailboxSvc.Send()

// F3 Approval 节点：
mailboxSvc.Send(service.MailboxMessage{
    Type: "approval_required", Priority: 3,
    Title: "需要确认：" + title,
    Body:  message,
    Payload: json.Marshal(map[string]any{"approvalId": approval.ID, "workflowId": wf.ID}),
})

// F2 运行失败：
mailboxSvc.Send(service.MailboxMessage{
    Type: "run_failed", Priority: 2,
    Title: "工作流运行失败：" + wf.Name,
    Body:  "节点 " + nodeID + " 执行出错",
    Payload: json.Marshal(map[string]any{"runId": runID, "workflowId": wf.ID, "nodeId": nodeID}),
})

// G2 部署成功：
mailboxSvc.Send(service.MailboxMessage{
    Type: "workflow_deployed", Priority: 1,
    Title: "工作流已部署：" + wf.Name,
    Body:  "自动触发已开启",
    Payload: json.Marshal(map[string]any{"workflowId": wf.ID}),
})
```

---

## 4. 推送到前端

```go
// 发送消息后立即通知前端更新 Inbox
func (s *MailboxService) SendAndNotify(msg MailboxMessage, bridge *events.Bridge) error {
    if err := s.Send(msg); err != nil { return err }
    count, _ := s.UnreadCount()
    bridge.Emit(events.MailboxUpdated, map[string]any{"unreadCount": count})
    return nil
}
```

---

## 5. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/mailbox", s.listMailbox)
mux.HandleFunc("PATCH /api/mailbox/{id}/read", s.markMailboxRead)
mux.HandleFunc("PATCH /api/mailbox/{id}/dismiss", s.dismissMailboxMessage)
```

---

## 6. 验收测试

```
1. Send(approval_required) → DB 插入 → MailboxUpdated 事件 → 前端红点出现
2. Send(run_failed) 两次（同工作流）→ 第一条被删除，只保留最新
3. MarkRead() → status=read → UnreadCount 减少
4. Resolve() → status=resolved
5. List() 按 priority DESC, created_at DESC 排序
6. UnreadCount() 准确反映 pending 数量
```
