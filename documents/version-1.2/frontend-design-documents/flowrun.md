# entities/flowrun — 前端 slice 详细设计

**所属层**：entities（对位后端 domain/flowrun）
**状态**：✅ 已实现
**职责**：管理 FlowRun（工作流执行记录）的只读查询 + 取消 + 人工审批（approval node）+ AI 调试（:triage）。FlowRun 是 Workflow 的运行实例，不同于 Workflow 的 entities/workflow 定义。

**关联文档**：
- [`../frontend-design.md`](../frontend-design.md) — FSD 总规范
- 后端 `service-design-documents/flowrun.md`

---

## 1. 职责边界

- 执行列表（按 workflowId / status / triggerKind 过滤）
- 单条执行详情 + 节点列表
- 取消执行（DELETE，非 :cancel）
- 节点人工审批（approve / reject）
- AI 调试（:triage — 创建调试对话）

---

## 2. 类型（`model/types.ts`）

```ts
type FlowRunStatus = "running" | "paused" | "completed" | "failed" | "cancelled";
type FlowRunTriggerKind = "cron" | "fsnotify" | "webhook" | "manual";
type FlowRunNodeStatus = "pending" | "running" | "ok" | "failed" | "cancelled" | "timeout" | "skipped";
type ApprovalDecision = "approve" | "reject";

interface PausedState {
  nodeId; variables: Record<string,unknown>;
  outputs: Record<string,Record<string,unknown>>;
  position: string[]; pausedAt: string;
}

interface FlowRun {
  id; userId; workflowId; versionId;
  triggerKind: FlowRunTriggerKind; triggerInput;
  status: FlowRunStatus; startedAt; endedAt?; elapsedMs;
  output?; errorCode?; errorMessage?; pausedState?;
  dryRun: boolean; createdAt; updatedAt;
}

interface FlowRunNode {
  id; userId; status: FlowRunNodeStatus;
  triggeredBy; input; output?; errorCode?; errorMessage?; elapsedMs;
  startedAt; endedAt; conversationId?; messageId?; toolCallId?;
  flowrunId; flowrunNodeId?; nodeId; nodeType;
  attempts; parentLoopNode?; iterationIndex?; createdAt;
}

interface FlowRunsParams { workflowId?; status?; triggerKind?; cursor?; limit? }
interface ApproveNodeVars { runId; nodeId; decision?: ApprovalDecision; reason? }
interface RejectNodeVars { runId; nodeId; reason? }
```

`pausedState` 在 approval node 等待时非空，包含当前执行上下文供前端展示暂停详情。

---

## 3. API hooks（`api/flowrun.ts`）

| Hook | 方法 + 端点 | 说明 |
|---|---|---|
| `useFlowRuns(params)` | GET `/flowruns?{qs}` | 列表，支持多维过滤 |
| `useFlowRun(id)` | GET `/flowruns/{id}` | 单条详情 |
| `useFlowRunNodes(id)` | GET `/flowruns/{id}/nodes` | 节点执行记录 |
| `useCancelFlowRun()` | DELETE `/flowruns/{id}` | 取消；invalidate flowruns + flowrun(id) |
| `useApproveNode()` | POST `/flowruns/{runId}/approvals/{nodeId}` body `{decision:"approve", reason}` | 人工审批-通过 |
| `useRejectNode()` | POST `/flowruns/{runId}/approvals/{nodeId}` body `{decision:"reject", reason}` | 人工审批-拒绝 |
| `useTriageFlowRun()` | POST `/flowruns/{id}:triage` | AI 调试，返回 conversationId |

注意：取消走 DELETE（非 :cancel action）；审批走 `/approvals/{nodeId}`（非 `/nodes/{nodeId}:approve`）。

---

## 4. 端到端数据流

### 4.1 人工审批节点

```
FlowRun 状态 = "paused"，pausedState.nodeId 指向 approval 节点
  → widgets/ApprovalPrompt 展示 pausedState.variables
  → 用户点击"批准"
  → useApproveNode().mutate({runId, nodeId, decision:"approve"})
      → POST /flowruns/{runId}/approvals/{nodeId}  {decision:"approve"}
      → 后端恢复执行
      → onSuccess: invalidate flowruns + flowrun(runId) + flowrunNodes(runId)
```

### 4.2 AI 调试

```
FlowRun 失败，用户点击"AI 调试"
  → useTriageFlowRun().mutate(id)
      → POST /flowruns/{id}:triage
      → 返回 {conversationId}
      → 跳转 chat pane，预加载对话
```

---

## 5. 实现清单

| 文件 | 说明 |
|---|---|
| `frontend/src/entities/flowrun/model/types.ts` | FlowRun / FlowRunNode / PausedState / Approval* 类型 |
| `frontend/src/entities/flowrun/api/flowrun.ts` | 7 个 hooks |
| `frontend/src/entities/flowrun/index.ts` | public API |
