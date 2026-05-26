# entities/workflow — 前端 slice 详细设计

**所属层**：entities（对位后端 domain/workflow + Phase 4 执行引擎）
**状态**：✅ 已实现
**职责**：管理 Workflow（DAG 图 + 调度配置）的 CRUD + 版本管理 + 图形化编辑（ops）+ 手动触发 + 能力检查。Graph 以 JSON 字符串存储，前端解析为 `Graph` 对象展示。

**关联文档**：
- [`../frontend-design.md`](../frontend-design.md) — FSD 总规范
- 后端 `service-design-documents/workflow.md`

---

## 1. 职责边界

- 列表 / 详情 / 版本历史
- pending 版本审批（accept / reject）
- 图形编辑（edit ops — 差异 → ops 数组 → POST :edit）
- 手动触发（:trigger）
- 能力检查（:capability-check）
- PATCH 更新调度配置（enabled / concurrency）
- 软删除

FlowRun 执行记录由 entities/flowrun 管理。

---

## 2. 类型（`model/types.ts`）

```ts
type VersionStatus = "pending" | "accepted" | "rejected";

interface VariableSpec { name; type?; description?; default? }
interface EdgeSpec { id; from; fromPort?; to; toPort? }
interface NodeSpec { id; type; label?; config? }
interface Graph {
  name; description?; tags?; variables?: VariableSpec[];
  nodes: NodeSpec[]; edges: EdgeSpec[];
}

interface WorkflowVersion {
  id; workflowId; status: VersionStatus; version?;
  graph: string;          // JSON 字符串（后端存储格式）
  graphParsed?: Graph;    // 前端反序列化后的结构
  changeReason; forgedInConversationId?; createdAt; updatedAt;
}

interface Workflow {
  id; userId; name; description; tags;
  enabled: boolean; concurrency: string;
  needsAttention: boolean; attentionReason: string;
  activeVersionId; createdAt; updatedAt;
  // 服务端计算字段
  pending?: WorkflowVersion; liveRuns?: number;
  lastFiredAt?: string; nextFireAt?: string;
}

interface WorkflowEditOp { op: string; [key: string]: unknown }
interface EditWorkflowVars { ops: WorkflowEditOp[]; changeReason?: string }
interface RunWorkflowVars { id: string; input?: Record<string,unknown> }
interface CapabilityIssue { nodeId; kind; ref; reason }
interface CapabilityCheckResult { ok: boolean; issues: CapabilityIssue[] }
```

---

## 3. API hooks（`api/workflow.ts`）

| Hook | 方法 + 端点 | 说明 |
|---|---|---|
| `useWorkflows()` | GET `/workflows?limit=200` | 列表 |
| `useWorkflow(id)` | GET `/workflows/{id}` | 详情含 pending + liveRuns |
| `useWorkflowVersions(id)` | GET `/workflows/{id}/versions` | 版本历史 |
| `useAcceptWorkflow()` | POST `/workflows/{id}/pending:accept` | 接受 pending |
| `useRejectWorkflow()` | POST `/workflows/{id}/pending:reject` | 拒绝 pending |
| `useDeleteWorkflow()` | DELETE `/workflows/{id}` | invalidate workflows |
| `useUpdateWorkflow(id)` | PATCH `/workflows/{id}` | 更新调度配置（enabled/concurrency） |
| `useRunWorkflow()` | POST `/workflows/{id}:trigger` body `{input}` | 手动触发，返 flowrun |
| `useEditWorkflow(id)` | POST `/workflows/{id}:edit` body `{ops, changeReason}` | 编辑器 autosave，invalidate workflow + versions |
| `useCapabilityCheck()` | POST `/workflows/{id}:capability-check` | 检查节点依赖是否满足 |

---

## 4. 端到端数据流

### 4.1 编辑器 autosave

```
WorkflowEditor 检测图变更
  → 计算 diff → ops 数组
  → useEditWorkflow(id).mutate({ops, changeReason: "manual edit"})
      → POST /workflows/{id}:edit  {ops, changeReason}
      → 后端产/迭代 pending 版本
      → onSuccess: invalidate workflow(id) + workflowVersions(id)
      → VersionRail 自动刷新，显示 pending 角标
```

### 4.2 手动触发

```
用户点击"运行"
  → useRunWorkflow().mutate({id, input})
      → POST /workflows/{id}:trigger  {input}
      → 后端 scheduler.StartRun(kind=manual)
      → 返回 flowrun id → 跳转到 flowrun 详情
```

---

## 5. 实现清单

| 文件 | 说明 |
|---|---|
| `frontend/src/entities/workflow/model/types.ts` | Workflow / WorkflowVersion / Graph / Edit* 类型 |
| `frontend/src/entities/workflow/api/workflow.ts` | 10 个 hooks |
| `frontend/src/entities/workflow/index.ts` | public API |
