---
id: WRK-010
type: working
status: archived
owner: @weilin
created: 2026-06-12
reviewed: 2026-06-12
review-due: 2026-09-12
expires: 2026-09-12
landed-into: ""
audience: [human, ai]
---

# DECISIONS-PENDING —— 产品级裁决台账

> R1 四条已全部裁决（2026-06-12，用户批「完全同意建议」）。本文件保留台账；新增裁决追加于此。

| 编号 | 问题 | 裁决 | 状态 |
|---|---|---|---|
| PD-A | `pkg/limits` 空壳（自述可调实未接线，仅 1/20 字段被消费） | **A：真做配置面 + 全量接线**（settings 面 + 各模块硬编码常量改读 `limits.Current()`，注意 limits 默认值与现实常量的漂移要对齐现实） | ✅ 已落地（PR-3 fixed） |
| PD-B | Ollama embedder baseURL/model 硬编码 | **A：settings 扩展** | ✅ 已落地（PR-4 fixed） |
| PD-C | 桌面 app 无日志文件 | **A 最小版：文件 + 轮转** | ✅ 已落地（PR-5 fixed） |
| PD-D | 跨机迁移密文不可解、无文档 | **B 先行：文档声明，export/import 进 roadmap** | ✅ 已落地（PR-6 doc-fix） |
| PD-E | 对话历史对 LLM 不可检索（PR-11）——人有综搜、LLM 只能靠 memory 萃取 | **批：加 `search_conversations`**（只返指针不返全文） | ✅ 已落地（PR-11 fixed） |
| PD-F | relation 图对 LLM 不可查（PR-12）——删除/改造前无法答「谁在用它」 | **批：加 `get_relations`** | ✅ 已落地（PR-12 fixed） |

