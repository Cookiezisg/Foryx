---
id: DOC-017
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# approval —— 审批表单实体（图节点的"人在环闸"）

## 1. 定位 + 心智模型

命名、版本化的**审批表**：markdown prompt 模板（`{{ CEL }}` 插值读 `input.*`）+ 决策规则（`allowReason` / `timeout` / `timeoutBehavior`）。方案 A 版本模型；无 sandbox/env。图把 approval 节点**固定的 yes/no 两出口**连下游。

**前缀注意**：实体是 `apf_`/`apfv_`——审批的**运行时**记录不是独立表，而是 flowrun_nodes 的 **parked 行**（parked 行即收件箱；见[引擎文档](../foundation/scheduler-flowrun.md) §4）。表（配置）与运行时（等待中的审批）是两回事，对位 trigger 实体 vs trigger_firings。

## 2. 行为 / 求值

create/edit 时：模板非空 + `{{ CEL }}` 全部可编译（`APPROVAL_INVALID_TEMPLATE`）；timeout 解析（`ParseTimeout` 扩展 `d`/`w` 粗粒度，`""`=永不超时）+ 非空 timeout 必须配 behavior（reject/approve/fail，`APPROVAL_INVALID_TIMEOUT`）。运行时：scheduler 解析钉死版本 → 渲染模板 → 写 **parked** 行（result 带 `rendered` 供收件箱 UI + `allowReason`）→ run 保持 running；决策（人工 first-wins vs 超时）见引擎文档。

## 3. 契约（引用）

端点（CRUD + `:edit`/`:revert`/`:iterate` + versions；决策端点在 flowrun 侧 `POST /flowruns/{id}/approvals/{node}:decide`）→ [api.md](../api.md) · 表 `approvals`/`approval_versions` → [database.md](../database.md) · 码 `APPROVAL_*` 7+4 → [error-codes.md](../error-codes.md) · ID：`apf_`/`apfv_`。三适配器同构；版本 cap 50 放过 active。
