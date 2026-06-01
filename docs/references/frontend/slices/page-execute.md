---
id: DOC-233
type: reference
status: active
owner: @weilin
created: 2026-05-27
reviewed: 2026-05-31
review-due: 2026-06-30
audience: [human, ai]
---
# pages/execute — 前端 slice 详细设计

**所属层**：pages（聚合 entities/flowrun）
**状态**：✅ 已实现（FSD Revamp 阶段 0–4 完成）
**职责**：FlowRun 列表 ↔ 详情路由器。与 ForgePage 结构对称：`focusEntity.execute` 驱动预打开指定 run；内部维护 `openRunId`。

---

## 1. Props 接口

```ts
interface ExecutePageProps {
  focusEntity?: { execute?: string; [key: string]: unknown };
  onConsumeFocusEntity: (pane: string) => unknown;
  onOpenChat?: (convId: string) => void;   // FlowRunDetail 中"查看对话"按钮
}
```

AppShell 从 paneStore 提取 `focusEntity` / `consumeFocusEntity`，提供 `onOpenChat = setActiveConv + openPane("chat")` 后传入。

---

## 2. 路由逻辑

```
focusEntity.execute = runId
  ↓
useFlowRun(runId) probe
  ↓
data 命中 → setOpenRunId(runId) + onConsumeFocusEntity("execute")

openRunId 非 null → FlowRunDetail(runId)
openRunId null    → ExecuteOverview
```

---

## 3. UI 子组件

| 组件 | 文件 | 职责 |
|---|---|---|
| `ExecuteOverview` | `ui/ExecuteOverview.tsx` | FlowRun 列表（状态过滤 + 触发手动执行） |
| `FlowRunDetail` | `ui/FlowRunDetail.tsx` | 单次执行详情（节点树 + 状态时间线 + 日志 + 跳 chat） |
| `ApprovalBanner` | `ui/ApprovalBanner.tsx` | FlowRunDetail 顶部 sticky banner；自取 `useApprovalInbox` 按 runId 过滤 parked，每行 approve/reject + 可选 reason |

---

## 4. 数据流

```
列表:
  useFlowRuns()     → ExecuteOverview 列表数据

详情:
  useFlowRun(id)    → run 元数据 + status
  useFlowRunNodes(id)  → 节点执行树
  useApprovalInbox()   → 当前用户 parked approval；ApprovalBanner 按 runId 过滤渲染

SSE notifications:
  flowrun 类型通知 → useNotifications → qc.invalidateQueries(qk.flowruns(), qk.flowrun(id), qk.approvals())
  → 列表 + 详情 + approval banner 自动刷新
```

---

## 5. 实现清单

| 文件 | 说明 |
|---|---|
| `frontend/src/pages/execute/ExecutePage.tsx` | 主路由组件 |
| `frontend/src/pages/execute/ui/ExecuteOverview.tsx` | FlowRun 列表 |
| `frontend/src/pages/execute/ui/FlowRunDetail.tsx` | 执行详情 |
| `frontend/src/pages/execute/ui/ApprovalBanner.tsx` | parked approval banner（自取 inbox，按 runId 过滤）|
| `frontend/src/pages/execute/index.ts` | public API export |
