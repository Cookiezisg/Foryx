---
id: WRK-011
type: working
status: active
owner: @weilin
created: 2026-06-12
reviewed: 2026-06-12
review-due: 2026-09-12
expires: 2026-09-12
landed-into: ""
audience: [human, ai]
---

# REPORT —— 全产品完整性审查终报（2026-06-12）

## 结论

第三种审查视角（「旅程走不走得完、配不配得齐」）一日五波收口：**25 条 finding（13 修复 + 5 doc-fix + 5 wontfix 带理由 + 2 待办）+ 6 条产品裁决全部落地 + 误报亲验驳回 6 条**。方法：agent 机械枚举供线索、每条 finding 亲自 grep/读码定性、修复随轮提交、轮轮 verify+race 全绿。

## 核心发现：「设计完整、接线缺失」模式 ×4

本轮最大价值是抓住了一类前两种审查（正确性 review、设计评审）结构性盲区的病：**每一行代码都对、设计也通，但 produce/consume 没配对**——

1. **`pkg/limits` 空壳**（PR-3 🔴）：自述「用户可调、settings.json 装配」，实际无加载器、SetProvider 零调用、20 字段仅 1 个被消费。→ 重述 schema 为现实投影 + app/settings + GET/PATCH /limits + 9 处接线。
2. **`todo_write` 工具不存在**（PR-10 🔴）：文档声称存在、HTTP 看板只读 by design、service 万事俱备——实体零写入口、功能整体死。→ 落地工具。
3. **异步唤回环缺失**（PR-17 🔴）：SetNeedsAttention 注释明写「scheduler raises this」但零调用；run 失败/审批挂起不通知任何人。→ run_failed/approval_pending 通知 + attention 自愈（失败点灯/成功熄灯）。
4. **workflow 活监听不重绑**（PR-1 🔴）：Edit/Revert 换入口 trigger 后旧绑定泄漏、新 trigger 无人听。→ diff 重绑。

共性：都是规划波次里掉的「最后一根线」。建议固化为收口检查项：**新能力交付前过一遍 produce/consume 配对**（返回的每个 id 有消费口吗？写口存在吗？事件有人发吗？）。

## 逐波摘要

| 波 | 焦点 | 实锤 | 关键产出 |
|---|---|---|---|
| R1 | 配置与基础设施面 | 9 | limits 真配置面、Ollama 参数面、日志文件落盘、迁移文档、workflow 重绑 |
| R2 | 实体闭环配对矩阵 | 7 | todo_write、search_conversations、get_relations、events.md 对齐 |
| R3 | 角色旅程走查 | 2 | run_failed/approval_pending 唤回环 + attention 自愈 |
| R4 | 横向一致性 | 4 | Activation/Firing N3 线缆修复、firing 收件箱可见、mcp 聚合补齐、工具互导 |
| R5 | 前端对接预检 | 3 | flowrun ?status 过滤；列表聚合/全局总览 wontfix 留档 |

## 裁决台账（6/6 落地）

PD-A limits 接线 · PD-B Ollama 参数 · PD-C 日志文件 · PD-D 迁移文档 · PD-E search_conversations · PD-F get_relations——全部当轮实现（见 DECISIONS-PENDING.md）。

## 待办（后续批）

- PR-14 🟢 fire_trigger 返 activationId（需动 FireManual 签名）。
- PR-18 🟢 env 构建失败通知（与 run_failed 同构小补）。

## 明细

全部 finding（验证过程 + 处置 + 误报驳回）见 [findings.md](findings.md)。
