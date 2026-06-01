---
id: DOC-225
type: reference
status: active
owner: @weilin
created: 2026-05-27
reviewed: 2026-05-31
review-due: 2026-06-30
audience: [human, ai]
---
# entities/flowrun — 前端 slice 详细设计

**所属层**：entities（对位后端 domain/flowrun）
**状态**：✅ 已实现
**职责**：管理 FlowRun（工作流执行记录）的只读查询 + 取消 + 人工审批（approval node）+ AI 调试（:triage）。FlowRun 是 Workflow 的运行实例，不同于 Workflow 的 entities/workflow 定义。

**关联文档**：
- [`../frontend-design.md`](../frontend-design.md) — FSD 总规范
- 后端 `references/backend/domains/flowrun.md`

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
type FlowRunStatus = "running" | "paused" | "awaiting_signal" | "completed" | "failed" | "cancelled";
type FlowRunTriggerKind = "cron" | "fsnotify" | "webhook" | "manual";
type FlowRunNodeStatus = "pending" | "running" | "ok" | "failed" | "cancelled" | "timeout" | "skipped";
type ApprovalDecision = "approved" | "rejected";
type ApprovalStatus = "parked" | "approved" | "rejected" | "timed_out" | "failed" | "cancelled";

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

// Approval — durable parked state of an approval node (后端 17 §9 投影)。
// inbox 端点返回当前用户所有 parked 行；banner 按 runId + status==="parked" 过滤。
interface Approval {
  id; userId; flowrunId; nodeId;
  prompt; payload?; status: ApprovalStatus;
  allowReason: boolean; reason?; decidedAt?;
  createdAt; updatedAt;
}

// TraceEntry — GET /flowruns/{id}/trace 的 journal 投影条目（08 §6 节点诊断）。
// 只读；seq 严格递增；loop 多轮按 iterationKey 区分；重连时全量补。
interface TraceEntry {
  seq: number; type: string; nodeId: string;
  iterationKey: number; generation: number;
  turn?: number; result?: Record<string, unknown>; at: string;
}
```

`pausedState` 在 approval node 等待时非空，包含当前执行上下文供前端展示暂停详情。

---

## 3. API hooks（`api/flowrun.ts`）

| Hook | 方法 + 端点 | 说明 |
|---|---|---|
| `useFlowRuns(params)` | GET `/flowruns?{qs}` | 列表，支持多维过滤 |
| `useFlowRun(id)` | GET `/flowruns/{id}` | 单条详情 |
| `useFlowRunNodes(id)` | GET `/flowruns/{id}/nodes` | 节点执行记录 |
| `useFlowRunTrace(runId, nodeId?)` | GET `/flowruns/{id}/trace?nodeId=X` | journal 投影给编排 UI **节点诊断**（只读；`nodeId` 过滤单节点；loop 多轮按 iterationKey 区分；重连全量补；返 `TraceEntry[]`）|
| `useApprovalInbox()` | GET `/approvals` | 当前用户所有 parked approval；**banner 数据源**，按 runId 过滤；approve/reject 后失效 |
| `useCancelFlowRun()` | DELETE `/flowruns/{id}` | 取消；invalidate flowruns + flowrun(id) |
| `useApproveNode()` | POST `/flowruns/{runId}/approvals/{nodeId}` body `{decision:"approved", reason}` | 人工审批-通过；invalidate flowruns + flowrun(id) + flowrunNodes(id) + approvals() |
| `useRejectNode()` | POST `/flowruns/{runId}/approvals/{nodeId}` body `{decision:"rejected", reason}` | 人工审批-拒绝；invalidate flowruns + flowrun(id) + approvals() |
| `useTriageFlowRun()` | POST `/flowruns/{id}:triage` | AI 调试，返回 conversationId |

注意：取消走 DELETE（非 :cancel action）；审批走 `/approvals/{nodeId}`（非 `/nodes/{nodeId}:approve`）；decision 值必须 `approved`/`rejected`（后端 canon，否则 400）。banner 不再靠节点 status（interpreter 从不发 `waiting_approval` 节点态），改读 `/approvals` inbox 投影。

---

## 4. 端到端数据流

### 4.1 人工审批节点

```
interpreter park approval 节点 → 写 approvals 投影行 (status=parked)
  → useApprovalInbox() GET /approvals 返回该用户所有 parked 行
  → pages/execute/ui/ApprovalBanner 按 runId + status==="parked" 过滤渲染
  → 用户点击"批准"
  → useApproveNode().mutate({runId, nodeId, decision:"approved"})
      → POST /flowruns/{runId}/approvals/{nodeId}  {decision:"approved"}
      → 后端 journal signal_received + 恢复执行 + 投影行翻 approved
      → onSuccess: invalidate flowruns + flowrun(runId) + flowrunNodes(runId) + approvals()
      → inbox 重取 → 该行不再 parked → banner 行消失
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
| `frontend/src/entities/flowrun/model/types.ts` | FlowRun / FlowRunNode / Approval / PausedState / Approval* 类型 |
| `frontend/src/entities/flowrun/api/flowrun.ts` | 9 个 hooks（含 `useApprovalInbox` + `useFlowRunTrace`）；`TraceEntry` 接口 |
| `frontend/src/entities/flowrun/index.ts` | public API |
