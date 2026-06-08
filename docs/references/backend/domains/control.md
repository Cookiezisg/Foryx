---
id: DOC-305
type: reference
status: active
owner: @weilin
created: 2026-06-08
reviewed: 2026-06-08
review-due: 2026-09-08
audience: [human, ai]
---
# Control Logic — 路由逻辑实体（workflow control 节点引用）

> **核心地位**：Control 是一个**面向 AI 的工作实体**——一组命名、版本化的**有序路由分支**（`when` 守卫 + `emit` 重塑）。workflow 的 `control` 节点按 id 引用它，图把每个分支的 `port` 连到下游。它是**纯控制流逻辑**：由 durable 解释器（波次 4）求值，**绝非 activity**——无 sandbox、无 env、无执行记录。设计源头见 [`18-graph-model-redesign.md`](../../../working/workflow-revamp/18-graph-model-redesign.md)。

---

## 1. 版本模型：线性 + 自由指针（无 accept，无沙箱）

与 function 同构的最轻版本化——**版本是 pin 所必需**（在途 flowrun 不漂移），但去掉 function 的 sandbox/env/executions：

| 概念 | 语义 | 谁能动 |
|---|---|---|
| **版本号 `version`** | 写入顺序（单调计数器，只增不改） | 写新版本时 = `max+1` |
| **版本内容** | 不可变的 branches 快照（append-only） | 永不修改既有版本 |
| **active 指针 `active_version_id`** | 「现在用哪个」 | edit 前移 / revert 自由移动 |

- **create** = 写 v1，立即 active。
- **edit** = 用一组**完整新 branches** 写 `v(max+1)` → 指针前移。立即生效、无断点。
- **revert(N)** = 只挪指针到 vN，不产生版本、不删「更新的」版本。
- 历史超 `VersionCap=50` 裁最老——**但绝不裁 active**（revert 后它可能很老）。
- **无 pending/accept 状态机**（与 function/handler/agent 一致）。

---

## 2. 物理模型（两表）

### 2.1 `control_logics`（`ctl_`，软删）
`id` · `workspace_id`(orm 自动隔离) · `name`(workspace 内 partial-UNIQUE，软删后释放) · `description` · **`active_version_id`**(指针) · 时间戳 · `deleted_at`。

### 2.2 `control_logic_versions`（`ctlv_`，append-only + cap 裁剪，无软删）
`id` · `workspace_id` · `control_id` · **`version`**(单调号) · **`input_schema`**(json，`[]schema.Field`，`TEXT NOT NULL DEFAULT '[]'`) · **`branches`**(json) · `change_reason` · `forged_in_conversation_id`(relation 边用) · 时间戳。`UNIQUE(control_id, version)`。

> **`input_schema`（统一 I/O 入参）**：`[]schema.Field`（共享 `internal/pkg/schema` 类型，全锻造实体统一），声明 workflow 节点喂给本 control 的输入字段。`branches[].when`/`emit` 的 CEL 据此读 **`input.*`**（见 §3）。

### 2.3 `Branch`（分支结构，存于 `branches` json）
```go
type Branch struct {
    Port string            `json:"port"`           // 具名结局：workflow 图据此路由（fromPort==port 的边把本臂 emit 输出带到其下游；可连回上游=循环）
    When string            `json:"when"`            // 布尔 CEL 守卫，读 input.*（first-true-wins；末条恒 "true" 兜底）
    Emit map[string]string `json:"emit,omitempty"`  // 字段→读 input.* 的 CEL（重塑下游 payload；空=透传 input）
}
```
> **Port = 路由结局，不是实体内部出口**：control = `(input) → (Port, emit 数据)`。control 实体**永不知道** Port 连到哪个下游节点——`fromPort==此 Port` 的图边把本臂 emit 输出带给它的下游，**路由住在 workflow**。

---

## 3. 校验（结构 domain + CEL app；无运行时类型层）

| 层 | 管什么 | 何时 |
|---|---|---|
| **① domain 结构** (`ValidateBranches` + `schema.ValidateFields`) | branches 非空 · 每个 port 非空且**唯一**（图要可区分寻址每个出口）· 末条 `when:"true"` **兜底**（防全 false 无路）· inputSchema 字段名唯一 + 类型合法 | create/edit |
| **② app CEL** (`pkg/cel` 编译) | 每个 `when` / `emit` 表达式语法 + 未知函数（如 `now()`）→ `ErrInvalidCEL`；表达式根变量 = **`input`**（节点喂入，由 inputSchema 声明） | create/edit（快速失败） |

> CEL **不在 domain 编译**（domain 不准 import cel-go，原则 #3）——app 层编译校验。运行期的 CEL **求值**是波次 4 解释器的事（按 flowrun pin 版本、缓存 program）；本实体只存 CEL **源串** + accept 期编译校验。

---

## 4. 锻造：全量 branches（无 ops）

create/edit 直接传 **`inputSchema` + 完整 branch 组**——两者是一个原子整体，function 那套增量 `ops` 框架对它无价值（全量替换），故**刻意简化**为整组传入。`when`/`emit` 引用的 `input.*` 字段（由 `inputSchema` 声明）与具体 workflow 的 payload 形状耦合，故 control 实体的**复用主要在同一 workflow 内**，价值在「编排-锻造分离 + 统一节点心智」而非跨 workflow 共享。

---

## 5. 跨域集成

- **catalog**：进（name + description）。强制每个 control 节点都引用实体 ⇒ catalog 比 function 多很多一次性小实体，**靠 AI 写清楚 name/description** 区分（不设特殊过滤机制）。
- **relation**：control 是第 **10** 个 EntityKind（前缀 `ctl_`）；`create`/`edit` 边由被锻造实体自述（对话 → 版本）。**ctl 不产出边**——`when`/`emit` 只读 `input`，不引用任何其他实体；`workflow → ctl` 监听/引用边由 workflow 侧在波次 4 产。
- **mention**：不进（配置/逻辑实体，非内容快照——同 trigger）。
- **notification**：`control.created/edited/reverted/updated/deleted` 经 `Emitter`。
- **生命周期**：**独立孤儿**——删 workflow 不级联删它引用的 ctl_（同 function/agent）；孤儿（relation `refCount=0`）可被识别、按需清理。

---

## 6. LLM 工具（6，懒加载）

`search_control`（子串找）· `get_control`（含 active 版分支）· `create_control` · `edit_control`（整组替换）· `revert_control`（按号移指针）· `delete_control`。

全 S18 五方法接口、danger 由 LLM 逐次自报；进 `Toolset.Lazy`，经 `search_tools` 浮现。**无 `run`、无 `executions`**——control 逻辑被 workflow 解释器求值，绝不独立调用。

---

## 7. HTTP 端点

`POST /controls`（扁平创建）· `GET /controls`（分页）· `GET|PATCH|DELETE /controls/{id}` · `POST /controls/{id}:edit|:revert` · `GET /controls/{id}/versions`(分页) · `GET /controls/{id}/versions/{version}`(整数号或 version id)。

> **无 `:run`**（不独立执行）、**无 pending 端点**（无 accept 状态机）。`:iterate`(AI 编辑) 随 askai 波次 6。

---

## 8. 在 workflow 里的角色（波次 4 消费）

`control` 节点 config 仅 `{ controlRef, 各 port → 下游 }`。scheduler 走到 control 节点：

```
取 ctl_ 的 pin 版本（pinned_callables）→ Service.Resolve(id, versionId) 拿 inputSchema + branches
  → 按 inputSchema 把节点入参喂成 input → 逐分支求 when（读 input.*，first-true-wins）
  → 命中分支算 emit（读 input.*）→ 记 branch_taken{port, payload}
  → 沿 fromPort==该 port 的下游边走（port 连回上游 = 结构化循环，emit 携带循环状态如 attempt+1）
```

**确定性**（durable replay 硬约束）：`when`/`emit` 只读已记账的 `input`、禁墙钟（`now()`），故 `branch_taken` 记下的 emit 后 payload 在重放时抄账、不分叉。详 [`17-execution-contract.md`](../../../working/workflow-revamp/17-execution-contract.md) §3/§4。

---

## 9. 错误字典

| Sentinel | Wire Code | HTTP |
|---|---|---|
| `ErrNotFound` | `CONTROL_NOT_FOUND` | 404 |
| `ErrDuplicateName` | `CONTROL_NAME_DUPLICATE` | 409 |
| `ErrVersionNotFound` | `CONTROL_VERSION_NOT_FOUND` | 404 |
| `ErrNoActiveVersion` | `CONTROL_NO_ACTIVE_VERSION` | 422 |
| `ErrInvalidName` | `CONTROL_INVALID_NAME` | 422 |
| `ErrInvalidBranches` | `CONTROL_INVALID_BRANCHES` | 422 |
| `ErrNoCatchAll` | `CONTROL_NO_CATCHALL` | 422 |
| `ErrInvalidCEL` | `CONTROL_INVALID_CEL` | 422 |

> 工具失败软返 tool-result 串（不冒泡 HTTP）；上表是 HTTP 端点冒泡的 domain 错误。
