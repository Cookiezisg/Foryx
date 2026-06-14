---
id: DOC-023
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
audience: [human, ai]
---

# conversation —— 对话线程容器

## 1. 定位 + 心智模型

线程**容器**实体：身份（title/pin/archive/软删）+ 线程级配置（systemPrompt / attachedDocuments / modelOverride——用户可改，chat 运行时消费）。**消息不在这**（归 messages/chat）。三个系统写字段在记录里但**不进 PATCH 面**：`Summary`/`SummaryCoversUpToSeq`（压缩器写）、`AutoTitled`（chat 首回合自动命名后写、绝不覆盖用户标题）。

**PATCH 三态**：`ModelOverride **ModelRef`——nil=不变、&nil=清除、&(&ref)=设置（指针的指针表达三态）。List：Archived nil=排除归档（默认）/&true=仅归档/&false=仅活跃；**按最近活跃排序**靠 store 的 `pinned DESC, last_message_at DESC, id DESC` + orm `PageKeyset("last_message_at")`（游标键随排序列对齐，见 [orm.md](../foundation/orm.md)）。

**last_message_at**（最近活跃排序键）：普通列（非 `,updated` tag，故 pin/改名/换模型不重排）。创建时种为 now，chat 经 `ConversationReader.TouchLastMessage` 在每个用户回合刷新——"最近聊过"上浮，ChatGPT 式 Today/Yesterday 分组的依据。

**isGenerating**（派生只读，`db:"-"`）：List/Get 据 chat 注入的 `GeneratingQuerier` 端口逐行填——该对话当前是否有在途 assistant 回合。让刚连上 / SSE 重连的客户端冷启动活动圆点（无需等下一帧）；纯运行时状态、不落库、不进 PATCH。与 canceler 同款后注入端口破 chat↔conversation 环。

**Unarchive**：chat Send 的自动解档入口（给归档线程发消息即隐式唤回）。**Delete 连带停生成**：可选 `GenerationCanceler` 端口（chat 满足、后注入破环）——删对话先 cancel 在途生成，已删线程不再烧 LLM/推流。

## 2. 契约（引用）

LLM 工具：`search_conversations`（内容混合检索历史对话——只返 conversationId/title/snippet/messageId，绝不返全文；回忆是指针、不是上下文倾倒）。

端点（CRUD）→ [api.md](../api.md) · 表 `conversations` → [database.md](../database.md) · 码 `CONVERSATION_*` 2 个 → [error-codes.md](../error-codes.md) · ID：`cv_`。被消费：chat（每回合读配置）、relation（conversation↔实体的 create/edit 边的另一端）、aispawn（`:iterate`/`:triage` 创建）。
