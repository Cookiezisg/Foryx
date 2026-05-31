# 04 — Claude Code 记忆系统

## 信息来源与局限

主要参考：
- https://code.claude.com/docs/en/memory (官方)
- https://www.deeplearning.ai/the-batch/claude-codes-source-code-leaked-exposing-potential-future-features-kairos-and-autodream/
- https://claudefa.st/blog/guide/mechanics/auto-dream
- https://milvus.io/blog/claude-code-memory-memsearch.md
- https://thoughts.jock.pl/p/claude-code-source-leak-what-to-learn-ai-agents-2026
- https://joseparreogarcia.substack.com/p/how-claude-code-rules-actually-work
- https://code.claude.com/docs/en/skills (官方)
- https://github.com/anthropics/skills/blob/main/skills/skill-creator/SKILL.md

---

## 1. CLAUDE.md 机制

### 1.1 加载层级（5 层）

✅ Claude Code 在 session 启动时**bottom-up 合并**多个 CLAUDE.md，越具体的越后加载、覆盖更宽泛的：

| 层级 | 路径 | 优先级 | 范围 |
|---|---|---|---|
| Enterprise Policy | macOS: `/Library/Application Support/ClaudeCode/CLAUDE.md` <br> Linux/WSL: `/etc/claude-code/CLAUDE.md` <br> Windows: `C:\ProgramData\ClaudeCode\CLAUDE.md` | 最高（不可被项目覆盖） | 整个组织/机器 |
| User Memory | `~/.claude/CLAUDE.md` | 高 | 当前用户跨所有项目 |
| Project Memory | `<project_root>/CLAUDE.md` | 中 | 仓库共享（commit 进 git） |
| Local Project | `<project_root>/CLAUDE.local.md` | 中（同 project，覆盖 project） | 个人本地（gitignored） |
| Subdirectory | `<project>/<sub>/CLAUDE.md` | **按需** | 进入该目录工作时才载入 |

✅ 启动时算法：
1. 从 cwd 向上遍历到 git root（或 home）；遇到 CLAUDE.md 即收集
2. 加上 `~/.claude/CLAUDE.md` 和 enterprise CLAUDE.md
3. 合并：拼接为单一 markdown，每段加 `<!-- source: <path> -->` 注释帮助 LLM 知道来源
4. 写入 system prompt 的**静态段**（DYNAMIC_BOUNDARY 之前）

✅ Subdirectory CLAUDE.md：进入子目录工作时**lazy load**——具体触发是 `InstructionsLoaded` hook 上的 `nested_traversal` 来源，意思是 Claude 第一次 Read 子目录里的文件时附加加载该目录的 CLAUDE.md。

### 1.2 注入位置

✅ CLAUDE.md 内容注入到 **system prompt 的静态段**（不是作为单独的 user message）。这意味着可以走 prompt cache，每次 turn 不重新算。

### 1.3 Compaction 后重新读盘

✅ autoCompact 完成后，CLAUDE.md / MEMORY.md / 已激活的 skills 和 rules 会**重新读盘并注入新历史**（防止被压缩丢失）。这点由 `SessionStart` 钩子的 `compact` matcher 也能利用——开发者写 hook 在 compact 后再注入额外 reminder。

### 1.4 Compact Instructions

✅ CLAUDE.md 里可以写 `## Compact Instructions` 小节，内容会作为 autoCompact subagent 的"特别保留"指令——对应"重要的项目特定规则不会被压缩丢"。

---

## 2. MEMORY.md 三层架构

### 2.1 设计哲学（来自泄漏分析）

✅ Claude Code 的"自动记忆"系统目标：**让 Claude 在多 session 之间累积学到的东西，但不让记忆文件膨胀拖慢每次 session start**。

类比：MEMORY.md 是**目录索引（sticky note index）**，不是档案柜——每条记录是 ~150 字的"在哪里能找到 X"指针。

### 2.2 三层结构

✅ 三层文件存于 `~/.claude/projects/<projectHash>/memory/`（每个项目独立）：

```
memory/
├─ MEMORY.md                  # Layer 1: 索引，永远 ≤ 200 行 / 25KB
├─ topics/                    # Layer 2: topic 文件，按需加载
│   ├─ user.md                # 用户偏好（"你喜欢用 bun"）
│   ├─ feedback.md            # 用户给的反馈（"上次说 prefer functional style"）
│   ├─ project.md             # 项目结构（build cmd, conventions）
│   ├─ reference.md           # 各种 reference (URL、命令)
│   └─ <topic>.md             # 自定义
└─ sessions/                  # Layer 3: session 全文 transcript
    ├─ 2026-04-29-abc123.jsonl
    └─ ...
```

### 2.3 加载策略

✅ **session start 时**：
- MEMORY.md 全量读（≤25KB / 200 行硬上限），注入 system prompt 静态段
- topic 文件**不读**——只在 MEMORY.md 索引里看到指针

✅ **session 进行中**：
- agent 判断"相关"时通过特定 tool 读 topic 文件——具体判断方式（关键词匹配？LLM 判断？）⚠️ 公开分析倾向于"LLM 在 system prompt 里被告知有这些 topics，自己决定调 ReadMemory tool 拉取"
- session log 仅在 autoDream 时被批量扫描，不参与正常对话

### 2.4 200 行 / 25KB 限制

✅ 硬上限 enforcement 由 autoDream 在 prune 阶段保证（详见 §3）。超出时新写入会触发 prune。

---

## 3. autoDream 后台整合 ⭐

### 3.1 触发条件

✅（来自 deeplearning.ai / claudefa.st 分析）autoDream 触发条件：
- **24 小时内未跑过** AND
- **本机已累积至少 5 个 session** AND
- **当前没有 active session 持锁**（`memory/.dream.lock` 文件不存在）
- 用户配置 `autoDream: enabled` (v2.1+ 默认 enabled，未来可能改为 opt-out)

✅ 触发位置：session **结束时**（SessionEnd 钩子），如果上面条件全 true，spawn 后台进程跑 autoDream，主 CLI 立即退出。autoDream 自己拿锁。

### 3.2 四阶段 Pipeline

✅ 四阶段（orient → gather → consolidate → prune）：

**Phase 1: Orient**
- 扫描 `memory/topics/` 已有的 topic 文件
- 读取 MEMORY.md 现有 index
- 列出最近 5 个 session 的 jsonl
- 输出：本次要处理的 session 列表 + 已知 topics 列表

**Phase 2: Gather**
- 对每个未处理 session 跑 LLM 分析（small model, 例如 Haiku），提取候选记忆条目
- 每条带 metadata：(topic, content, evidence_session_id, confidence)
- 输出：candidate facts 列表（去重前）

**Phase 3: Consolidate**
- 把 candidate 按 topic 归类
- 对每个 topic：merge 进现有 topic 文件，去重、合并矛盾
  - 例如 user.md 已有"喜欢 npm"，新发现"用户改用 bun" → 用户偏好变更，旧条目标记 deprecated 或删
- 把每个 topic 文件的 1-2 句 summary 更新到 MEMORY.md index
- 输出：更新后的 topic 文件 + 新 MEMORY.md 草稿

**Phase 4: Prune**
- 检查 MEMORY.md 是否 >200 行或 >25KB
- 超出时：删最旧 / 最少引用的 topic pointer；或者把 topic 文件本身 archive 到 `memory/archive/`
- 释放锁

### 3.3 作为 forked subagent 跑

✅ autoDream 在**独立子进程**跑（不是 thread），这样：
- 独立 context，不污染主对话
- 受限工具集：只能 Read/Write 在 `~/.claude/projects/<id>/memory/` 目录、只能读 sessions/、不能 Bash、不能 Edit 工程代码
- 失败也不影响主 session（异常打到 `~/.claude/logs/autodream.log`）

✅ 锁机制：`memory/.dream.lock` 文件 + flock 系统调用（Linux/macOS）。文件存在即"另一个 dream 在跑"，本次 skip。文件含 PID，可手动检测 stale lock。

---

## 4. Rules files

### 4.1 .claude/rules/*.md

✅ Rules 是 **project-scoped 的可分文件 CLAUDE.md**——把 instructions 拆到多个文件，按需加载：

```
.claude/rules/
├─ code-style.md          # 总是加载（无 paths 字段）
├─ api-conventions.md     # path-scoped
├─ testing.md             # path-scoped
└─ security.md
```

✅ 加载规则：
- 默认所有 `.claude/rules/*.md` 在 session start 全量加载到 system prompt
- 文件可加 YAML frontmatter `paths:` 改为 path-scoped

### 4.2 Path-scoped activation

✅ 用 YAML frontmatter：

```markdown
---
paths:
  - "src/api/**/*.ts"
  - "src/handlers/**/*.ts"
---

# API Design Rules

When writing code in `src/api/`...
```

✅ 触发逻辑：
- session start 时**不加载**这些 path-scoped rule
- agent 第一次 Read 一个匹配的文件时，rule 内容**附在 Read 结果后**（同 skill 落地的位置），相当于一次 just-in-time 注入
- 后续 Read 同样路径不重复注入（去重 by file hash）

⚠️ 已知 issue（#16853 / #38487）path-scoped rules 在 Write/Edit 触发时**不一定**自动加载——只对 Read 可靠。

### 4.3 与 CLAUDE.md 的差别

| 维度 | CLAUDE.md | rules/ |
|---|---|---|
| 文件数 | 单一 | 多文件 |
| 加载粒度 | 全部 / 子目录全部 | 按 path-scoped frontmatter |
| 适用场景 | 项目级总规则、persona | 模块级具体规则 |
| 目录探测 | 向上遍历 | 仅 `.claude/rules/` 一处 |

---

## 5. Skills 系统

### 5.1 SKILL.md 文件格式

✅ Skill 是带 YAML frontmatter 的 markdown 文件，存于：
- `~/.claude/skills/<skill-name>/SKILL.md`（用户级）
- `<project>/.claude/skills/<skill-name>/SKILL.md`（项目级）

```markdown
---
name: my-skill
description: What this skill does AND when to use it. (★ description 是 LLM 唯一能看到的"该不该用"的依据，写好它最重要)
disable-model-invocation: false   # true = 用户必须显式 /skill-name 调用，LLM 不能自动调用
allowed-tools: Read Grep          # 可选：调用此 skill 时 agent 只能用这些 tool
---

# Skill 内容

具体的 procedure 文档（markdown）。包含步骤、约束、示例等。

可以引用同目录其他文件：见 ./reference.md
```

✅ Skill 同目录可放任意辅助文件（python 脚本、reference markdown），SKILL.md 用相对路径引用。

### 5.2 加载与触发

✅ session start：
- 扫描所有 skill 目录，**只读 frontmatter** 的 `name` 和 `description`
- 把 "skill name + description" 列表注入 system prompt（动态段）
- SKILL.md **正文不读**

✅ 调用：
- LLM 决定用 skill → 调 `Skill` tool with `skill: "my-skill"`
- Skill tool 执行：读 SKILL.md 全文 → 注入到 conversation 作为新 system 上下文（保持到 session 结束）
- `allowed-tools` 生效（限制此后能用的 tool）

### 5.3 Skill 与 Slash Command

✅ Skills 自动暴露成 slash command：`/my-skill <args>` 等价于 `Skill(skill: "my-skill", args: "<args>")`。

✅ 用户直接打 `/<skill-name>` 时**绕过 LLM 决策**——直接进入 skill flow（这就是为什么 `disable-model-invocation: true` 仍能让用户用）。

---

## 6. 对 Forgify 的改进建议

> 现状：
> - `conversation.systemPrompt`（domain/conversation）是用户手填的字段，不是 agent 学到的
> - 无 CLAUDE.md 等价物
> - 无 MEMORY.md / autoDream
> - Forgify 的 `forge_*` 工具系列已经是"用户自定义工具"，类似 skills 的能力但是工具粒度

| # | 改进 | 优先级 | 实施要点（Go） |
|---|---|---|---|
| 1 | **FORGIFY.md 等价 CLAUDE.md** | P0 | 在 `runner.go:289 buildSystemPrompt` 之后插入：从 cwd 起向上遍历 git root，收集所有 `FORGIFY.md`；加上 `~/.forgify/FORGIFY.md`；按层级 merge。新文件 `chat/memory.go` 提供 `LoadProjectMemory(ctx) string`。注入到 system prompt 静态段（参考报告 03 §3）。Go 实现 ~50 行 |
| 2 | **`read_memory` / `write_memory` 工具** | P1 | 新文件 `agent/memory.go`：两个 tool。`read_memory(topic string)` → 读 `~/.forgify/projects/<projectHash>/memory/topics/<topic>.md`；`write_memory(topic, content, mode {append|replace})` → 写。对 path 做 sanitize（不允许 `..`）。这一步让 agent 能**手动**记笔记 |
| 3 | **MEMORY.md 索引** | P2 | session start 自动读 `memory/MEMORY.md` 注入 system prompt（动态段，因为它会变）。200 行 / 25KB 限制 enforce 由 write_memory 工具检测溢出时报错让 agent 自己 prune。比 autoDream 简单很多 |
| 4 | **autoDream 简化版** | P3 | `chat/autodream.go`：每 24h 触发一次，spawn `goroutine` 跑（Forgify 后端是 daemon，不需要单独子进程）。简化版只做"orient + consolidate"，不 fork subagent，单次 LLM call。失败回滚即可 |
| 5 | **Rules 系统** | P2 | 复用 #1 的加载流程，扫描 `.forgify/rules/*.md`；解析 YAML frontmatter `paths` 字段；session start 加载所有无 paths 的；带 paths 的标 lazy。lazy 触发点放在 `read_file` tool execute 后（见报告 02 read_file 的扩展） |
| 6 | **Skills（远期）** | P3 | Forgify 已有 forge_tool 系列（用户自定义工具）。Skills 是**高于 tool 的 procedure 模板**。可后期把 `forge_tool` 升级为支持 markdown skill 文件。优先级低 |

最小可行第一步：**#1（FORGIFY.md）+ #2（read_memory / write_memory tool）**，这俩做完 agent 立刻能跨 session 累积知识，约 1-2 天工作量。

