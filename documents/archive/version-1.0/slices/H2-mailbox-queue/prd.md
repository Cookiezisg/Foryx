# H2 · Mailbox 队列 — 产品需求文档

**切片**：H2  
**状态**：待 Review

---

## 1. 背景

Approval 节点（F3）和权限门控（H1）都需要"将操作暂存并路由到 Inbox，等待用户决策"。H2 定义统一的 Mailbox 队列机制，是 Inbox 展示层（I2）的数据来源。

---

## 2. Mailbox 消息类型

| 类型 | 来源 | 优先级 |
|---|---|---|
| `approval_required` | F3 Approval 节点 | 高 |
| `permission_required` | H1 权限门控 | 高 |
| `run_failed` | F4 运行失败通知 | 中 |
| `run_completed` | F2 运行完成通知 | 低 |
| `workflow_deployed` | G2 部署成功 | 低 |
| `system` | 系统通知 | 低 |

---

## 3. 消息生命周期

```
created → pending → read → resolved/dismissed
```

| 状态 | 说明 |
|---|---|
| `pending` | 待处理，Inbox 图标显示红点 |
| `read` | 已查看，红点消失，消息仍在列表 |
| `resolved` | 已处理（审批完成、错误已修复）|
| `dismissed` | 用户手动关闭 |

---

## 4. 队列行为

- 新消息到达时，侧边栏 Inbox 图标显示未读数量红点
- 高优先级消息（审批）同时发送系统桌面通知（macOS 通知中心）
- 同类型消息自动合并（如同一工作流的多次运行失败，只显示最新一条）

---

## 5. 验收测试

```
1. Approval 节点触发 → Mailbox 新增 approval_required 消息
2. 运行失败 → Mailbox 新增 run_failed 消息
3. 打开 Inbox → 消息状态变 read → 红点消失
4. 审批完成 → 消息状态变 resolved
5. 同一工作流连续失败两次 → 只保留最新的 run_failed 消息
```
