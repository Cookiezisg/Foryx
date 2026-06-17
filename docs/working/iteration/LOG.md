---
id: WRK-028
type: working
status: active
owner: @weilin
created: 2026-06-18
reviewed: 2026-06-18
review-due: 2026-09-16
audience: [human, ai]
landed-into:
---

# Iteration Loop —— Finding 索引（一行一条，永不写成 essay）

> **规范（强制）**：一个 finding = **一行**，每格一个短语。证据→轨迹 dump；修法详情→commit；本表只做索引。
> 状态：`open` 待修 · `confirmed` 已复现待修 · `fixed` 已修+验+回归 · `watch` 观察 · `not-bug` 判断后非 bug（成本/性能/可恢复且行为正确——不算）· `dup` 被他条覆盖。
> 新发现追加在表末。**别删行**（同 D1 Log 语义）。

| ID | 状态 | 问题（一句话） | 范围 | 修法（定位） | 验证（前→后） | commit |
|---|---|---|---|---|---|---|
| F1 | fixed | lazy 工具概览不点名 id 参数 → 模型瞎猜参数名（`query`/`function_name`…） | **系统性 49/50** | 地基：`toolset.Overview` 浮出必填参数名 + `prompt` 渲 `name(args)` + preamble id→search 解析 | function+handler 修前 4/4 错 → 修后 4/4 一次对、零 error；79/91 工具现渲参数 | dfe2a361 |
| F2 | not-bug | "resident vs searchable" 措辞被半误读——但 agent 行为本就正确，非 bug | — | — | — | — |
| F3 | not-bug | 简单任务 ~75K input token（冗长 schema 重发）——成本/性能，**非 bug**（作者明示不算） | — | — | — | — |
| F4 | watch | `run_function` 首调 args 平铺非 `{"args":{…}}`（修 F1 后未复现，疑被 F1 一并修掉） | 待 CONFIRM | — | — | — |
| F5 | open | 模型用无效字段类型 `"integer"`（schema 只认 number）→ 一次失败调用 + 恢复 | 疑系统性（`pkg/schema` 共享） | 倾向宽容：`integer→number` 等别名归一 | — | — |
| F6 | fixed | edit 带 set_meta 不更新实体行 name/desc/tags（只移版本指针）→ agent 以为改了名、后端没改 | function+handler（workflow 本就对；agent/control/approval 无 set_meta op） | `Edit` 把 draft meta 带回行 + `SaveVersionAndActivate(v, f)` 同事务 Save 整行（6 文件） | `:edit set_meta` 重命名后 GET 真变；零 token 回归 `Test{Function,Handler}_EditPersistsMeta` 绿；make verify 绿 | _pending_ |

## 元注（一次性，非 finding）
- **为什么这 loop 值得**：F1 那条轨迹 `golden J5` 只断言"版本>1"是绿的；轨迹判官却抓到模型把 `get_function` 调错绕一圈——终态测试瞎、判官看见。
- 永久回归 test：`selfiter_confirm_f1_*`、`selfiter_confirm_f1batch_*`（守 F1）。
