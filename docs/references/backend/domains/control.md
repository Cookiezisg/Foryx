---
id: DOC-016
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# control —— 路由逻辑实体（图节点的"if/switch/loop 闸"）

## 1. 定位 + 心智模型

命名、版本化的**有序路由分支组**（方案 A 版本模型，同 function；无 sandbox/env/executions——纯控制流，由解释器**内联求值**、绝非 activity）。核心抽象：**control = (input) → (Port, Emit 数据)**——实体永不知道 Port 连哪个下游，那是 workflow 图的事（FromPort 匹配的边）。

**Branch**：`When`（读 `input.*` 的布尔 CEL，**first-true-wins** 自上而下）+ `Port`（具名出口，图据此路由；出口可连回上游 = 结构化循环）+ `Emit`（可选，字段→CEL 重塑下游 payload；**空 = 透传 input**）。**末条必须 `When=="true"`**（兜底，all-false 也有路由——`CONTROL_NO_CATCHALL` 强制）。Port 非空且组内唯一（`CONTROL_INVALID_BRANCHES`）。

## 2. 行为 / 求值

create/edit 时：domain `ValidateBranches`（结构）+ app 编译每条 when/emit CEL（`CONTROL_INVALID_CEL`——原则 #3，domain 不碰 cel-go）。运行时：scheduler 经 `Resolve(id, pinnedVersionID)` 取**钉死版本**的分支，对节点解析出的 `input` 求值；result = emit 字段扁平 + 保留键 `__port`（下游直接读 `gate.feedback`，解释器读 `gate.__port` 路由）。

## 3. 契约（引用）

端点（CRUD + `:edit`/`:revert`/`:iterate` + versions）→ [api.md](../api.md) · 表 `controls`/`control_versions` → [database.md](../database.md) · 码 `CONTROL_*` 8+4 → [error-codes.md](../error-codes.md) · ID：`ctl_`/`ctlv_`。catalog/mention/relation 三适配器同构；版本 cap 50 放过 active。
