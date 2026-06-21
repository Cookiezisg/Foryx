---
id: DOC-026
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# memory —— 跨对话长期记忆（文件式）

## 1. 定位 + 心智模型

文件式注入物（skill 的同族，文件式范式）：每条记忆一个 markdown 文件（`~/.anselm/workspaces/<ws>/memories/<name>.md`，frontmatter：description/source/pinned），**name(slug) 即身份**。`ForSystemPrompt` 把记忆目录注入 system prompt——**pinned 全文、其余 name+description 列表**（目录而非全文，控 token）；LLM 经 `read_memory` 按需加载非 pinned 全文。写侧 `write_memory` 一律 source=ai、不 pinned（pinned 是用户的策展动作）。**`Upsert` 更新保留策展**（F147）：编辑既有记忆只改 content/description，**保留**现有 `pinned`（仅经专用 pin/unpin 端点改）+ `source`（不可变作者归属）——否则 LLM 的 write_memory（永远发 source=ai、从不设 pinned）每次编辑都静默把用户置顶的逐字注入规则降级成懒加载目录行。纯按需扫描、无缓存。

## 2. 契约（引用）

端点（CRUD + pin）→ [api.md](../api.md) · 无 DB 表（文件式）· 码 `MEMORY_*` 4+3 → [error-codes.md](../error-codes.md)。LLM 工具：read/write/forget_memory（lazy）。被消费：chat system prompt（MemoryProvider 端口）。
