---
id: DOC-027
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
audience: [human, ai]
---

# todo —— 对话工作清单（LLM 自管 + 实时呈现）

## 1. 定位 + 心智模型

每执行作用域一份 LLM 自管的工作清单（对话级；subagent run 内另起独立一份。≤64 项——超出是规划异味；上限同时给 reminder 注入设界）。**整表替换写**（TodoWrite 语义：LLM 每次重写全清单，状态机在 LLM 脑中、存储只管快照）。两条呈现路径：**reminder**（chat host 每步前注入 live 清单为临时 `<system-reminder>`——清单顶在模型眼前、不污染持久历史）+ **messages 流**（写入即推 todo 信号，前端实时渲染面板）。

## 2. 契约（引用）

表 `todos`（每执行作用域一行——PK `scope_id` = subagent run 内取 subagent id、否则对话 id；列含 conversation_id + subagent_id + items json）→ [database.md](../database.md) · 码 `TODO_*` 4 → [error-codes.md](../error-codes.md)。LLM 工具：todo_write + todo_read（均 resident）——todo_read 无参、读回当前作用域整张清单**含已完成项**（复用 `Service.ReadRendered`=`Get`+`render`，空清单软返 cleared 串、无新码）。它补的缺口：**reminder 抑制全完成清单**（`reminder()` 在 0-open 时 return false——刻意不让完成清单每轮注入），故没 todo_read 时 agent 完成后被问列清单凭记忆**编造**；常驻（非懒）使读回无需 search_tools 跳转。被消费：chat host（实现 loop 的 `ReminderProvider` 端口）+ messages 流 + 只读看板 `GET /conversations/{id}/todos`（`?subagentId=` 可选）→ [api.md](../api.md)。
