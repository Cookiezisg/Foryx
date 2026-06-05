---
id: DOC-113
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-05
review-due: 2026-09-01
audience: [human, ai]
---
# Memory Domain — 按 workspace 的长期记忆（文件式）

> **核心地位**：Memory 是 agent 的**跨对话长期记忆**——每个 workspace 自己的事实/偏好/项目背景。重设计为**文件式**：每条记忆是 `~/.forgify/workspaces/<wsID>/memories/<name>.md`（markdown，用户可直接看/编辑/git），LLM 用工具自管。**无 SQLite、无 id、无向量/热度/分类**——经业界调研（MemGPT/Mem0/LangMem/Claude memory tool + 2025 综述）验证：单机单用户下简单胜复杂（LOCOMO 上全量上下文还赢过 Mem0），记忆系统买的是省 token 非准确率。

---

## 1. 物理模型（文件，非表）
一条记忆 = `<name>.md`：
```markdown
---
description: 语言版本偏好
pinned: true
source: user
---
用户不喜欢 Python 3.8，新项目一律 3.11+。
```
- **文件名（去 `.md`）= name**：slug 唯一标识，**无生成 id**。
- frontmatter：`description`（目录用）、`pinned`（注入策略）、`source`（user/ai）。
- 正文 = content；`UpdatedAt` = 文件 mtime（用户直接改文件也反映）。
- **workspace 分桶**：每个 workspace 一个 `memories/` 目录，完全隔离（路径据 ctx workspace，name slug 校验防穿越，不用 pathguard）。

## 2. 核心原理

### 2.1 两段式注入 system prompt
```
## Memory (pinned)              ← pinned 全文常驻
### no-python38 (source: user)
用户不喜欢 Python 3.8...

## Memory index — read_memory(name) to load   ← 非 pinned 只列 name+description
- api-base-url: 后端地址约定
```
- pinned 全文常驻；非 pinned 只给目录，LLM 要用 → `read_memory(name)` 加载全文。
- 砍热度排序——按 name 排。

### 2.2 LLM 自管 + 天然去重
LLM 经 `write_memory / read_memory / forget_memory`（工具，波次 2/3）自管。system prompt 已注入记忆目录 → LLM 写前看得见现有 → 自然 update 而非新建重复（天然实现 Mem0 的 add/update 决策，**无向量 pipeline**）。

### 2.3 用户可控
前端管理 UI（handler）列/编辑/删/置顶；用户也能**直接编辑文件**（Finder/git）——文件即真相，下次扫描自然反映。

### 2.4 变更发通知
每次增删改 → `notification.Emitter.Emit("memory.created/updated/deleted", {name})` → 前端通知中心/列表刷新。

## 3. 砍掉的（对照旧 SQLite 版）
SQLite 表、**mem_ id**、热度（AccessCount/AccessedAt/排序）、Type 四分类、Metadata、软删、自建通知/access、向量/图/Generator/reflection/decay——全砍（业界调研判为单机过度工程）。

## 4. 端点（前端管理）
| 端点 | 作用 |
|---|---|
| `GET /api/v1/memories` | 列表（`?pinned=` 可选） |
| `GET /api/v1/memories/{name}` | 一条全文 |
| `PUT /api/v1/memories/{name}` | upsert |
| `DELETE /api/v1/memories/{name}` | 删 |
| `POST /api/v1/memories/{name}/pin` · `/unpin` | 置顶/取消 |

## 5. 跨域集成 / 落位
- **chat（波次 5）**：经 `SystemPromptProvider.ForSystemPrompt` 注入记忆段。
- **工具（波次 2/3）**：`read/write/forget_memory` 包 app 的 Get/Upsert/Delete。
- **infra/fs/memory**：backend-new 第一个文件式 store（手写 frontmatter 解析/原子写 temp+rename/slug 防穿越），skills（波次 3）复用此范式。
- **boot（M7）**：注入 `~/.forgify` base 路径 + `notification.Emitter`。

## 6. 错误
| Sentinel | Wire Code | HTTP | 场景 |
|---|---|---|---|
| `ErrNotFound` | `MEMORY_NOT_FOUND` | 404 | 文件不存在 |
| `ErrInvalidName` | `MEMORY_INVALID_NAME` | 400 | name 非 slug |
| `ErrInvalidSource` | `MEMORY_INVALID_SOURCE` | 400 | source 非 user/ai |
| `ErrInvalidInput` | `MEMORY_INVALID_INPUT` | 400 | description/content 缺 |
