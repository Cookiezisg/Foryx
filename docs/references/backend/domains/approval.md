---
id: DOC-306
type: reference
status: active
owner: @weilin
created: 2026-06-08
reviewed: 2026-06-08
review-due: 2026-09-08
audience: [human, ai]
---
# Approval Form — 审批渲染实体（workflow approval 节点引用）

> **核心地位**：Approval Form 是一个**面向 AI 的工作实体**——一个 markdown prompt 模板（含 `{{ CEL }}` 插值）+ 决策规则（`allowReason` / `timeout` / `timeoutBehavior`）。workflow 的 `approval` 节点按 id 引用它，把数据**渲染成人能看懂的审批点**；图把它固定的 `yes`/`no` 出口连到下游。它是**纯配置**——渲染 + park 是 durable 解释器（波次 4）的运行时事，**绝非 activity**，无 sandbox/env/executions。设计源头见 [`18-graph-model-redesign.md`](../../../working/workflow-revamp/18-graph-model-redesign.md) §3.2。

> **🔑 前缀分层（必读）**：本实体前缀 **`apf_`/`apfv_`**（approval **form**），**不是** `apv_`。`apv_` 属 `approvals` **运行时表**（per-flowrun 的 parked/approved/timed_out 记录，波次 4，17 §1）。**表（配置）与运行时记录是两回事**——对位 trigger 实体（`trg_`）vs `trigger_firings`（运行时）。

---

## 1. 版本模型：线性 + 自由指针（无 accept，无沙箱）

与 control / function 同构的最轻版本化——版本是 **pin 所必需**，但无 sandbox/env/executions：

- **create** = 写 v1，立即 active。**edit** = 用一组**完整新 template + 规则**写 `v(max+1)` → 指针前移。**revert(N)** = 只挪指针。
- 历史超 `VersionCap=50` 裁最老——**绝不裁 active**。**无 pending/accept**。

---

## 2. 物理模型（两表）

### 2.1 `approval_forms`（`apf_`，软删）
`id` · `workspace_id`(orm 自动隔离) · `name`(workspace 内 partial-UNIQUE，软删后释放) · `description` · **`active_version_id`** · 时间戳 · `deleted_at`。

### 2.2 `approval_form_versions`（`apfv_`，append-only + cap 裁剪，无软删）
`id` · `workspace_id` · `approval_id` · **`version`**(单调号) · **`template`**(markdown，含 `{{ CEL }}`) · **`allow_reason`**(bool) · **`timeout`**("" = 永不超时) · **`timeout_behavior`**(reject/approve/fail) · `change_reason` · `forged_in_conversation_id` · 时间戳。`UNIQUE(approval_id, version)`。

---

## 3. 校验（结构 domain + CEL 模板 app）

| 层 | 管什么 | 何时 |
|---|---|---|
| **① domain** (`ValidateForm`) | `template` 非空（无说明的审批无意义——用户看到孤零零按钮）· `timeout` 非空时 `timeoutBehavior` ∈ {reject,approve,fail} **且** `timeout` 是合法 duration（`ParseTimeout` 支持 `30d`/`2w`） | create/edit |
| **② app CEL 模板** (`pkg/cel.CompileTemplate`) | 提取 `template` 里的 `{{ expr }}` 段，各自编译；语法错 / 未知函数（`now()`）→ `ErrInvalidTemplate` | create/edit（快速失败） |

> `{{ CEL }}` 模板支持是本轮给 `pkg/cel` 加的**地基**（`Template` 类型：`CompileTemplate` 编译校验 + `Render` 运行时渲染）；agent.prompt（波次 4）复用同一套。CEL **不在 domain 编译**（domain 不准 import cel-go，原则 #3）。

---

## 4. 锻造：全量 template + 规则（无 ops）

create/edit 直接传**完整**的 template + allowReason + timeout + timeoutBehavior——表是一个原子整体，无 `ops` 框架。`template` 里 `{{ CEL }}` 引用的字段与具体 workflow 的 payload 形状耦合，故复用主要在同一 workflow 内（同 control）；价值在「编排-锻造分离 + 统一节点心智」。

---

## 5. 跨域集成

- **catalog**：进（name + description；AI 工作实体，靠 AI 写清楚 name/description 区分）。
- **relation**：approval 是第 **11** 个 EntityKind（前缀 `apf_`）；`create`/`edit` 边由被锻造实体自述（对话 → 版本）。**不产出边**——template 的 CEL 只读 payload。
- **mention**：不进（配置/逻辑实体，同 trigger/control）。
- **notification**：`approval.created/edited/reverted/updated/deleted` 经 `Emitter`。
- **生命周期**：**独立孤儿**——删 workflow 不级联（同 function/control/agent）。

---

## 6. LLM 工具（6，懒加载）

`search_approval`（子串找）· `get_approval`（含 active 版 template + 规则）· `create_approval` · `edit_approval`（整组替换）· `revert_approval`（按号移指针）· `delete_approval`。

全 S18 五方法接口、danger 由 LLM 逐次自报；进 `Toolset.Lazy`，经 `search_tools` 浮现。**无 `run`、无 `executions`**——审批表被 workflow 解释器渲染 + park，绝不独立调用。

---

## 7. HTTP 端点

`POST /approvals` · `GET /approvals`(分页) · `GET|PATCH|DELETE /approvals/{id}` · `POST /approvals/{id}:edit|:revert` · `GET /approvals/{id}/versions` · `GET /approvals/{id}/versions/{version}`。

> **无 `:run`**（不独立执行）、**无 pending 端点**（无 accept 状态机）。`:iterate`(AI 编辑) 随 askai 波次 6。

---

## 8. 在 workflow 里的角色（波次 4 消费）

`approval` 节点 config = `{ approvalRef, yes→下游, no→下游 }`（出口固定 `yes`/`no`，**不由实体定义**）。scheduler 走到 approval 节点：

```
取 apf_ 的 pin 版本 → Service.Resolve(id, versionId) 拿 template + 规则
  → pkg/cel.CompileTemplate(template).Render(已记账 payload/ctx) → 渲染出 markdown
  → park：写 signal_awaited 事件 + approvals 运行时行（apv_，status=parked）→ flowrun awaiting_signal
  → 用户决策 / timeout（durable timer，timeoutBehavior 决定 reject/approve/fail）→ 沿 yes/no 出口继续
```

**确定性**：插值读已记账值、禁墙钟（`now()`）；渲染结果落 approvals 行，重放直接读、不重算。详 [`17-execution-contract.md`](../../../working/workflow-revamp/17-execution-contract.md) §9。

---

## 9. 错误字典

| Sentinel | Wire Code | HTTP |
|---|---|---|
| `ErrNotFound` | `APPROVAL_NOT_FOUND` | 404 |
| `ErrDuplicateName` | `APPROVAL_NAME_DUPLICATE` | 409 |
| `ErrVersionNotFound` | `APPROVAL_VERSION_NOT_FOUND` | 404 |
| `ErrNoActiveVersion` | `APPROVAL_NO_ACTIVE_VERSION` | 422 |
| `ErrInvalidName` | `APPROVAL_INVALID_NAME` | 422 |
| `ErrInvalidTemplate` | `APPROVAL_INVALID_TEMPLATE` | 422 |
| `ErrInvalidTimeout` | `APPROVAL_INVALID_TIMEOUT` | 422 |

> 工具失败软返 tool-result 串（不冒泡 HTTP）；上表是 HTTP 端点冒泡的 domain 错误。
