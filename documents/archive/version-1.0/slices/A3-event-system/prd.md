# A3 · 事件系统 — 产品需求文档

**切片**：A3  
**状态**：待 Review  
**依赖**：A1  
**下游**：所有需要实时推送的切片（B2、E2、F2、F3、G4、I4）

---

## 1. 这块做什么

定义 Forgify 所有从后端到前端的实时推送事件。用户不直接感知这一层，但它是流式对话、工作流节点状态、审批通知等所有实时体验的基础。

---

## 2. 设计原则

**事件是单向的**：Go 后端 → 前端。前端触发操作通过 HTTP REST 完成，不走事件。

**事件是窄的**：每个事件只携带它自己需要的数据，不传递大 payload。

**事件名称格式**：`domain.action`，全小写，点分隔。

---

## 3. 全量事件清单

### 对话类

| 事件名 | 触发时机 | 关键 payload |
|---|---|---|
| `chat.token` | AI 流式输出每个 token | `conversationId`, `token`, `done` |
| `chat.done` | 本轮 AI 输出完成 | `conversationId`, `messageId` |
| `chat.error` | AI 调用报错 | `conversationId`, `error` |
| `chat.compacted` | 上下文压缩发生 | `conversationId`, `level`(micro/auto/full) |

### 工作流执行类

| 事件名 | 触发时机 | 关键 payload |
|---|---|---|
| `flow.node.status` | 节点状态变化 | `runId`, `nodeId`, `status`(pending/running/done/error) |
| `flow.node.output` | 节点执行完成 | `runId`, `nodeId`, `output` |
| `flow.run.done` | 整个工作流跑完 | `runId`, `status`, `duration` |
| `flow.run.error` | 工作流执行失败 | `runId`, `nodeId`, `error` |

### 审批类

| 事件名 | 触发时机 | 关键 payload |
|---|---|---|
| `approval.request` | Mailbox 收到执行级操作需审批 | `requestId`, `toolId`, `description`, `params` |
| `approval.expired` | 审批超时（默认 10 分钟） | `requestId` |

### 画布类

| 事件名 | 触发时机 | 关键 payload |
|---|---|---|
| `canvas.updated` | 工作流 FlowDefinition 被后端修改 | `workflowId`, `summary` |

### 主题记忆类

| 事件名 | 触发时机 | 关键 payload |
|---|---|---|
| `topic.memory.updated` | autoDream 整理完成 | `topicId`, `diff` |

### 系统类

| 事件名 | 触发时机 | 关键 payload |
|---|---|---|
| `tray.first-hide` | 首次点关闭按钮 | 无 |
| `notification` | 需要展示桌面或站内通知 | `title`, `body`, `level`(info/warn/error) |

---

## 4. 不在本切片范围内

- 具体事件的业务实现（各自所属切片负责）
- WebSocket（用不到，SSE 已够用）

---

## 5. 验收标准

- [ ] Go 层可以 emit 任意事件，前端收到
- [ ] 事件名拼写错误时有编译期或运行期提示（通过常量定义）
- [ ] TypeScript 侧所有事件有类型定义，不需要 any
- [ ] 同一事件多个前端 listener 可以共存
