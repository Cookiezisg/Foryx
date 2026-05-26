# entities/function — 前端 slice 详细设计

**所属层**：entities（对位后端 domain/function + domain/askai）
**状态**：✅ 已实现
**职责**：管理 FunctionEntity（Python 函数）的 CRUD + 版本管理（pending accept/reject/revert）+ 运行。与后端 "trinity" 的 function 部分对应。

**关联文档**：
- [`../frontend-design.md`](../frontend-design.md) — FSD 总规范
- 后端 `service-design-documents/function.md`

---

## 1. 职责边界

- 列表 / 详情 / 版本历史查询
- pending 版本审批（accept / reject）
- 版本回滚（revert）
- 手动执行（run）
- 软删除

不含 AI 迭代（`:iterate` 走 features/forge-iterate）。

---

## 2. 类型（`model/types.ts`）

```ts
type EnvStatus = "pending" | "syncing" | "ready" | "failed" | "evicted";
type VersionStatus = "pending" | "accepted" | "rejected";

interface ParameterSpec {
  name: string; type: string; description?: string;
  required: boolean; default?: unknown; enum?: unknown[];
}

interface FunctionVersion {
  id: string; functionId: string; status: VersionStatus; version?: number;
  code: string; parameters: ParameterSpec[]; returnSchema: Record<string,unknown>;
  dependencies: string[]; pythonVersion: string;
  envId: string; envStatus: EnvStatus; envError: string;
  envSyncedAt?: string; envSyncStage: string; envSyncDetail: string;
  changeReason: string; forgedInConversationId?: string;
  createdAt: string; updatedAt: string;
}

interface FunctionEntity {
  id: string; userId: string; name: string; description: string;
  tags: string[]; activeVersionId: string;
  createdAt: string; updatedAt: string;
  // 服务端计算字段
  pending?: FunctionVersion; envStatus?: EnvStatus;
  envError?: string; envSyncedAt?: string; envSyncStage?: string; envSyncDetail?: string;
}

interface RunFunctionVars { id: string; inputs: Record<string,unknown> }
interface RunFunctionResult { output: unknown; elapsedMs: number }
```

JS 内置 `Function` 冲突 → 实体命名 `FunctionEntity`。字段对齐后端 json tag（camelCase）。

---

## 3. API hooks（`api/function.ts`）

| Hook | 方法 + 端点 | 说明 |
|---|---|---|
| `useFunctions()` | GET `/functions?limit=200` | 列表；select pickList |
| `useFunction(id)` | GET `/functions/{id}` | 详情 |
| `useFunctionVersions(id)` | GET `/functions/{id}/versions` | 版本历史 |
| `useAcceptFunction()` | POST `/functions/{id}/pending:accept` | 接受 pending 版本；invalidate 函数 + 版本列表 |
| `useRejectFunction()` | POST `/functions/{id}/pending:reject` | 拒绝 pending 版本；同上 invalidate |
| `useRevertFunction()` | POST `/functions/{id}:revert` | 回滚到上一个 accepted；invalidate 函数 |
| `useRunFunction()` | POST `/functions/{id}:run` body `{inputs}` | 手动执行，不 invalidate |
| `useDeleteFunction()` | DELETE `/functions/{id}` | invalidate functions |

注意路由区别：accept/reject 走 `/pending:accept`（专属前缀），revert 走 `/{id}:revert`（action 后缀）。

---

## 4. 端到端数据流

```
用户点击 forge 界面"接受"按钮
  → features/forge-review → useAcceptFunction().mutate(id)
      → POST /functions/{id}/pending:accept
      → onSuccess: invalidateQueries([functions] + [function,id] + [function-versions,id])
      → useFunctions() / useFunction(id) 自动重取 → UI 刷新

用户手动运行函数
  → features/run-function → useRunFunction().mutate({id, inputs})
      → POST /functions/{id}:run  {inputs}
      → 返回 {output, elapsedMs} → 直接展示结果（不写入 store）
```

---

## 5. 实现清单

| 文件 | 说明 |
|---|---|
| `frontend/src/entities/function/model/types.ts` | FunctionEntity / FunctionVersion / ParameterSpec / Run* 类型 |
| `frontend/src/entities/function/api/function.ts` | 8 个 hooks |
| `frontend/src/entities/function/index.ts` | public API |
