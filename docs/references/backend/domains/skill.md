---
id: DOC-018
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
audience: [human, ai]
---

# skill —— 文件式 Agent Skill（指令载体）

## 1. 定位 + 心智模型

skill 是**指令载体、非构建实体**——memory 的近亲（文件式注入物），不是 function 的近亲（执行实体）。**name(slug) 即身份**：无生成 id、无版本（编辑即覆盖文件）、零 DB 表、零 LLM 依赖；无 execution log、无 LLM 搜索（与文件式指令载体的抽象错配，故不提供）。

**持久化 = 文件**（`infra/fs/skill`，复用 memory 的文件式范式）：每 skill 一个目录 `~/.anselm/workspaces/<ws>/skills/<name>/SKILL.md`（目录而非扁平文件——未来 references/assets 可同住）。**纯按需**：每次 List 现扫目录，无缓存/无 watcher；坏文件跳过不连坐。slug 正则（`^[a-z][a-z0-9_-]{0,63}$`）**既是身份校验也是路径穿越守卫**（合法 name 1:1 映射目录）。

**Frontmatter 逐字镜像 Anthropic SKILL.md spec**（跨厂字段全留以便无缝导入）+ Anselm 自有扩展 `source: user|ai`（谁创作）。护栏：body ≤32KB、description ≤1024 字符。

## 2. 行为

- **激活两模式**（`Activate(name, args)`）：`inline` = 渲染正文（`$ARGUMENTS`/`$1..$n`/命名占位/`${CLAUDE_SESSION_ID}` 替换；刻意**不支持** `!`cmd`` shell 注入——任意执行面，拒绝）注入当前对话 + **把 allowed-tools 记为本次运行的预授权**（active skill，危险确认流消费——预授权非限制白名单）；`fork` = 把渲染正文派给隔离 subagent（frontmatter.agent 必填，否则 `SKILL_FORK_REQUIRES_AGENT`）。
- **`Guide(name)`**（agent 挂载路径）：只渲染正文（展开 `${CLAUDE_SESSION_ID}`，不接 `$ARGUMENTS`/位置参数）、**不**设 active-skill、**不** fork——见 [agent.md](agent.md)#3。
- **创作**：Create（同名 → `SKILL_NAME_CONFLICT`）/ Replace（缺失 → 404）/ Delete；同步 relation 边（allowed-tools → equip 出边、构建对话 → 入边）。

## 3. 契约（引用）

端点 → [api.md](../api.md)（CRUD + `:activate`）· 无 DB 表（文件式）· 码 `SKILL_*` 7+1 → [error-codes.md](../error-codes.md) · 通知 `skill.{created,updated,deleted}`。LLM 工具 5 个：activate/get/create/edit/delete_skill（无 search——catalog 概览已曝光全部 skill）。消费方：chat loop（active skill 预授权）、agent（Guide 挂载）、catalog（name+desc）。
