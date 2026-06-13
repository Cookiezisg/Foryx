---
id: DOC-023
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# conversation —— 对话线程容器

## 1. 定位 + 心智模型

线程**容器**实体：身份（title/pin/archive/软删）+ 线程级配置（systemPrompt / attachedDocuments / modelOverride——用户可改，chat 运行时消费）。**消息不在这**（归 messages/chat）。三个系统写字段在记录里但**不进 PATCH 面**：`Summary`/`SummaryCoversUpToSeq`（压缩器写）、`AutoTitled`（chat 首回合自动命名后写、绝不覆盖用户标题）。

**PATCH 三态**：`ModelOverride **ModelRef`——nil=不变、&nil=清除、&(&ref)=设置（指针的指针表达三态）。List：Archived nil=排除归档（默认）/&true=仅归档/&false=仅活跃；置顶优先排序靠 store 的 `pinned DESC, created_at DESC` + orm Page 序覆盖（见 [orm.md](../foundation/orm.md)）。

**Unarchive**：chat Send 的自动解档入口（给归档线程发消息即隐式唤回）。**Delete 连带停生成**：可选 `GenerationCanceler` 端口（chat 满足、后注入破环）——删对话先 cancel 在途生成，已删线程不再烧 LLM/推流。

## 2. 契约（引用）

LLM 工具：`search_conversations`（内容混合检索历史对话——只返 conversationId/title/snippet/messageId，绝不返全文；回忆是指针、不是上下文倾倒）。

端点（CRUD + usage）→ [api.md](../api.md) · 表 `conversations` → [database.md](../database.md) · 码 `CONVERSATION_*` 2 个 → [error-codes.md](../error-codes.md) · ID：`cv_`。被消费：chat（每回合读配置）、relation（conversation↔实体的 create/edit 边的另一端）、aispawn（`:iterate`/`:triage` 创建）。
