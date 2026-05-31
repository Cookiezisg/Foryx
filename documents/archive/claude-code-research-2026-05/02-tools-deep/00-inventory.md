# 00 — Claude Code v2.1.126 Tool 完整 Inventory + Forgify 优先级

> 本文件是 `02-tools-deep/` 系列的索引：先把当前 Claude Code 全工具列清楚 + 标 Forgify 是否抄，再按确定下来的顺序写每个 family 的 deep-dive。

## 信息来源与置信度

- **基准版本**：Claude Code **v2.1.126**（2026-05-01 release；当前 stable）
- **主源**：[Piebald-AI/claude-code-system-prompts](https://github.com/Piebald-AI/claude-code-system-prompts)（已更新至 v2.1.126，含 24 个 built-in tool 完整 description 原文）
- **副源**：[code.claude.com/docs/en/tools-reference](https://code.claude.com/docs/en/tools-reference)、[agent-teams](https://code.claude.com/docs/en/agent-teams)、[scheduled-tasks](https://code.claude.com/docs/en/scheduled-tasks)
- **比对基线**：[`../02-tools.md`](../02-tools.md)（v2.1.88, 2026-03-31 调研；落后 38 版本）
- **审计日期**：2026-05-03

⚠️ **关于 schema 字段名**：本表 schema/参数列为二手转录，部分字段名可能与源码不一致（如 Edit 可能是 `old_string/new_string` 而非 `from/to`，Grep 可能是 `path` 而非 `paths`）。**deep-dive 时按 Piebald-AI 原文 schema 校准**——本 inventory 仅用于优先级判断，不作为实现依据。

⚠️ **关于工具计数**：Piebald-AI 报告 24 个 built-in，加上 orchestration / agent-team / mcp / experimental 等扩展约 **38–41 个**（不同来源略有出入）。本表收 41 项，含部分 v2.1.126 公开度低的扩展工具——deep-dive 时再剔除已下线者。

---

## 优先级标注规则

| 标注 | 含义 | Forgify 处置 |
|---|---|---|
| ✅ P0 | 必抄；与 Forgify 定位高度匹配，Phase 5 范围内 | deep-dive 全文研究 + 实现 |
| ✅ P1 | 重要；agentic workflow 的关键 UX/能力 | deep-dive 研究，实现按需排期 |
| ⚠️ P2 | 待定；潜在价值但不在 Phase 5 直接 roadmap | 简评不深挖，保留观察 |
| ❌ Skip | 与 Forgify "单用户本地 Wails Agentic Workflow" 定位不匹配 | 跳过；本表注明理由 |

---

## 完整 Inventory

### 1. file — 文件操作

| 工具 | 用途 | 行为要点 | 优先级 | 理由 |
|---|---|---|---|---|
| `Read` | 读文件，支持行 offset / limit | line-numbered 输出（cat -n 格式）；支持 PDF（≤20 页）/ image / Jupyter notebook；v2.1.86 加 compact 行号 + 重读去重 | ✅ P0 | 基础能力；Forgify 当前 0 个文件 tool |
| `Write` | 完全覆写或新建文件 | 已存在文件需先 Read 才能 Write；v2.1.89 修 Windows CRLF 双重 + Markdown 硬换行剥除 | ✅ P0 | 基础能力 |
| `Edit` | 文件内字符串精确替换 | 字面量匹配；0 或 >1 次匹配失败；`replace_all` 仍存在 #51986 静默跳过 bug；v2.1.89 后支持对 Bash(sed/cat) 看过的文件直接编辑 | ✅ P0 | `02-tools.md` 标的最大缺口 |
| `MultiEdit` | 单文件原子化批量 edit | 全部成功才提交；任一失败回滚 | ❌ Skip | **v2.1.126 已下线**——Piebald-AI 清单不含 `multiedit.md`（GitHub API 直查确认），issue [#11125](https://github.com/anthropics/claude-code/issues/11125) 关闭 "not planned"。替代：同 turn 多次 Edit + runTools 串行调度天然给 batch 性。详见 [`01-file-ops.md` § MultiEdit 现状](./01-file-ops.md#multiedit-现状不抄) |
| `NotebookEdit` | Jupyter notebook cell 编辑 | replace/insert/delete 三模式；0-indexed cell；保留 metadata | ⚠️ P2 | Forgify 当前不针对数据科学 notebook 工作流 |

### 2. search — 检索

| 工具 | 用途 | 行为要点 | 优先级 | 理由 |
|---|---|---|---|---|
| `Glob` | 按 glob pattern 找文件 | 返回 mtime 倒序；**v2.1.117 macOS/Linux native build 移除独立 tool，改 embed 入 Bash via `bfs`**；Windows / npm 安装仍保留独立 tool | ✅ P0 | 基础能力 |
| `Grep` | 全文检索，底层 ripgrep | 默认 `files_with_matches`；支持 multiline、`output_mode`、上下文行；同 Glob，v2.1.117 macOS/Linux 改 embed 入 Bash via `ugrep` | ✅ P0 | 基础能力 |
| `LS` | 目录列出 | 须传绝对路径；支持 ignore glob list；返回排序稳定 | ❌ Skip | **跟 CC 一起砍**——v2.1.126 已下线；Forgify 让 Glob 输出升级为 JSON（含 type/size/mtime），既覆盖"找文件"也覆盖"列目录"，吃掉 LS 需求。详见 [`02-search.md` § LS 决策记录](./02-search.md#ls-决策记录) |

### 3. shell — 命令执行

| 工具 | 用途 | 行为要点 | 优先级 | 理由 |
|---|---|---|---|---|
| `Bash` | 持久 cwd 的 shell 执行 | cwd 跨命令持久；env vars **不**持久（已知限制 #2508/#20503）；30000 字符截断；timeout 默认 120s 最大 600s；支持 `run_in_background`；v2.1.98+ 大量权限加固（反斜杠转义 / 复合命令 / `/dev/tcp` / wildcard 空白 / `find -exec/-delete` 等漏洞全修） | ✅ P0 | `02-tools.md` 标 P1；Forgify 完全没有；优先级提升至 P0 |
| `PowerShell` | Windows 优先的 PS 执行 | 平台 gated；`CLAUDE_CODE_USE_POWERSHELL_TOOL=1`；v2.1.111+ Windows 默认渐进开启；Profiles 不加载 | ⚠️ P2 | Wails 跨平台，但 Bash via Git Bash 已能覆盖 Windows；非首要 |

### 4. web — 网络

| 工具 | 用途 | 行为要点 | 优先级 | 理由 |
|---|---|---|---|---|
| `WebFetch` | 抓 URL + LLM 摘要 | HTML→Markdown；15 分钟缓存；HTTP 自动 HTTPS；用 Haiku 类小模型摘要；**独立 context window，不污染主对话**；v2.1.119 自动剥 `<style>`/`<script>` | ✅ P0 | agentic workflow 必备 |
| `WebSearch` | 实时网搜 | **仅美国可用**；要求响应含 "Sources:" section；支持 allow/block domain | ✅ P1 | Forgify 用 **3 层 fallback** 实现（SearXNG 公共池 → Bing scrape → Bing CN scrape），全免费开源零配置，"质量一般但永远响应"。详见 `04-web.md` §6 |

### 5. task-tracking — LLM 内部任务跟踪

| 工具 | 用途 | 行为要点 | 优先级 | 理由 |
|---|---|---|---|---|
| `TaskCreate` | 创建带跟踪的任务 | session-scoped；上限 50；写到 `~/.claude/tasks/`；支持 metadata + blockedBy | ✅ P1 | TodoWrite 的进化版；agentic 拆分子任务 UX 锚 |
| `TaskList` | 列任务（仅摘要） | 完整详情用 TaskGet；防 context bloat | ✅ P1 | 配套 |
| `TaskGet` | 获取单任务详情 | 完整 task object（description / metadata / dependencies） | ✅ P1 | 配套 |
| `TaskUpdate` | 更新状态 / 依赖 / owner | in-place 原子写盘 | ✅ P1 | 配套 |
| `TaskStop` | 停 background task | 子进程 SIGTERM；output 文件保留 | ⚠️ P2 | 配套；非核心 |
| `TaskOutput` | （deprecated）取 background task 输出 | 推荐改用 Read 直读 output 文件 | ❌ Skip | 已 deprecated |
| `TodoWrite` | 老的清单 tool | Agent SDK / 非交互模式才用 | ❌ Skip | 已被 Task* 系列取代 |

### 6. ux — 用户交互

| 工具 | 用途 | 行为要点 | 优先级 | 理由 |
|---|---|---|---|---|
| `AskUserQuestion` | 单选 / 多选 / 文本问用户 | 阻塞等用户输入；可附文件 preview；支持 `multiSelect` | ✅ P1 | LLM 主动澄清需求 = agentic UX 关键 |
| `EnterPlanMode` | 进只读 plan 模式 | 阻 Edit/Write/Bash 等写操作；保留全 LSP / Read | ⚠️ P2 | 概念好，但 Forgify 当下一杆到底执行 model |
| `ExitPlanMode` | 提交 plan 求用户批准 | 拒绝则继续 plan 模式 | ⚠️ P2 | 配套 |
| `SendUserMessage` | 主动推消息给用户 | 单向，无 response | ⚠️ P2 | 与 Forgify 自有 chat.message SSE 重叠 |

### 7. orchestration — 编排

| 工具 | 用途 | 行为要点 | 优先级 | 理由 |
|---|---|---|---|---|
| `Agent` | 派 subagent 跑独立上下文 | 同 session 内派生；独立 context window；不带主对话 history | ⚠️ P2 | Phase 5+ 才考虑；与 forge 派独立 Python 进程不重叠 |
| `Skill` | 调用 reusable skill prompt | scope: project / user / cli / plugin | ❌ Skip | Forgify 的 forge 已是用户可定义扩展，与 skill 概念重叠 |
| `EnterWorktree` | 创 git worktree 隔离分支 | 隔离 fs + 独立 cwd；subagent 不可用 | ❌ Skip | git workflow，Forgify 不聚焦 |
| `ExitWorktree` | 合 worktree 回主分支 | 提交未提交变动 | ❌ Skip | 同上 |
| `Monitor` | 守 background 进程并响应输出 | 流式接收每行；同 Bash 权限；mid-conversation 触发 | ⚠️ P2 | forge 长跑可能需要；Phase 5 评估 |
| `CronCreate` | 排定时任务 | 5-field cron；session-scoped；7 天自动过期；jitter ±10% | ⚠️ P2 | 排定 forge 跑是合理远期功能 |
| `CronList` | 列定时任务 | — | ⚠️ P2 | 配套 |
| `CronDelete` | 取消定时任务 | 立即取消；不补跑漏火 | ⚠️ P2 | 配套 |

### 8. agent-team — 多 agent 协同（实验）

| 工具 | 用途 | 行为要点 | 优先级 | 理由 |
|---|---|---|---|---|
| `TeamCreate` | 派多 agent 团队共享任务 | `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1` gated；配置在 `~/.claude/teams/` | ❌ Skip | Forgify 单用户单对话 model |
| `TeamDelete` | 解散 team | 仅 lead 可清；teammates 跑着会 fail | ❌ Skip | 同上 |
| `SendMessage` | 给 teammate / subagent 发信 | 自动 deliver 到 mailbox | ❌ Skip | 同上 |

### 9. mcp — MCP 集成

| 工具 | 用途 | 行为要点 | 优先级 | 理由 |
|---|---|---|---|---|
| `mcp__<server>__<tool>` | MCP server 动态注册 tool | 命名格式 `mcp__github__search_repositories`；snake_case；v2.1.121 加 `alwaysLoad` | ⚠️ P2 | MCP 集成本身是大主题；先把 native tool 抄完再说 |
| `ListMcpResourcesTool` | 列 connected MCP server resources | 无参 | ⚠️ P2 | 同上 |
| `ReadMcpResourceTool` | 读特定 MCP resource | 参 `uri: string` | ⚠️ P2 | 同上 |
| `ToolSearch` | 搜并 lazy load deferred tool | MCP tool 数量大时启用；v2.1.116 阈值上调到 500K char | ⚠️ P2 | 同上 |

### 10. lsp — 代码智能

| 工具 | 用途 | 行为要点 | 优先级 | 理由 |
|---|---|---|---|---|
| `LSP` | 跳定义 / 查引用 / 报错 | code-intelligence plugin 启用才活；Edit/Write 后自动报错；14+ 语言 | ❌ Skip | Forgify 不是 IDE；与 forge 代码生成场景错位 |

### 11. other — 桌面 / 浏览器自动化

| 工具 | 用途 | 行为要点 | 优先级 | 理由 |
|---|---|---|---|---|
| `Computer` | Chrome DevTools 桌面 / 浏览器自动化 | screenshot + 鼠键坐标；inline 截图回传；CLI/Desktop only | ❌ Skip | 桌面 RPA 不在 Forgify Agentic Workflow 范畴 |
| `BrowserBatch` | 串行浏览器操作 | 批操作；不可并行 | ❌ Skip | 同上 |

---

## 优先级汇总

### ✅ Tier 1 — P0 必抄（7 个）
基础文件 / 检索 / 执行 / 网络能力，Forgify 当前完全空缺。

| Family | 工具 |
|---|---|
| file | Read, Write, Edit |
| search | Glob（升级为统一"找+列"），Grep |
| shell | Bash |
| web | WebFetch |

### ✅ Tier 1.5 — P1（6 个，分 3 个 deep-dive 单元）
agentic UX 关键能力。

- WebSearch
- AskUserQuestion
- TaskCreate / TaskList / TaskGet / TaskUpdate（任务跟踪算 1 个 deep-dive 单元）

### ⚠️ Tier 2 — P2 待评估（13 个）
潜在价值但不进 Phase 5 直接 roadmap：NotebookEdit, PowerShell, EnterPlanMode, ExitPlanMode, SendUserMessage, Agent, Monitor, CronCreate, CronList, CronDelete, TaskStop, mcp__pattern, ListMcpResourcesTool, ReadMcpResourceTool, ToolSearch。

### ❌ Skip（12 个）

| 工具 | 不抄理由 |
|---|---|
| MultiEdit | **v2.1.126 已下线**（Piebald-AI 不含 multiedit.md，issue #11125 not planned）；同 turn 多次 Edit 替代 |
| LS | **跟 CC 一起砍**——v2.1.126 已下线；Forgify 让 Glob 输出升级为 JSON（含 type/size/mtime）覆盖"列目录"用例 |
| TodoWrite | 已被 Task* 系列取代 |
| TaskOutput | 已 deprecated |
| Skill | Forgify 的 forge 已覆盖此概念 |
| EnterWorktree / ExitWorktree | git workflow，与 Forgify 主线无关 |
| TeamCreate / TeamDelete / SendMessage | 多 agent 团队，Forgify 单用户单对话 model |
| LSP | Forgify 不是 IDE |
| Computer / BrowserBatch | 桌面 RPA，与 Agentic Workflow 范畴错位 |

---

## Deep-dive 顺序建议

按 family 依次产出 deep-dive 文档；每篇 1500–3000 字，统一含：

1. **Tool description 原文**（完整摘自 Piebald-AI）
2. **完整 JSON Schema**（含字段类型 / 必填 / 默认值 / 描述）
3. **内部算法**（如 Edit 的字面量匹配、Glob 的 mtime 排序、Bash 的 cwd 持久化机制）
4. **Edge case + 已知 bug + workaround**
5. **输出格式与截断策略**
6. **并发安全性判定**（不只是 readOnly 一个 bit，覆盖 args-dependent 的判定）
7. **Forgify Go 实现要点**（接口落地 / 依赖库 / validate 错误码 / 测试要点）

| 顺序 | 文件 | 覆盖工具 | 估时 |
|---|---|---|---|
| 1 | `01-file-ops.md` | Read, Write, Edit（MultiEdit 已下线，独立验证后跳过） | 0.5 天 |
| 2 | `02-search.md` | Grep, Glob（升级为统一"找+列"，吃掉 LS） | 0.3 天 |
| 3 | `03-shell.md` | Bash（顺带覆盖 Monitor 的概念） | 0.5 天 |
| 4 | `04-web.md` | WebFetch, WebSearch | 0.3 天 |
| 5 | `05-ux-tasks.md` | AskUserQuestion + TaskCreate/Update/List/Get | 0.4 天 |
| (6) | `06-orchestration.md` | Agent, Cron* — 仅在前 5 篇消化吸收后再决定要不要做 | 待定 |

总估时：约 **2 天密集深挖**，覆盖 Tier 1 + 1.5。

---

## 与 v2.1.88 基线（`02-tools.md`）的关键差异

需要在 deep-dive 中**主动校验**（基线 ≠ 当前）：

1. **macOS/Linux native build 已无独立 Glob/Grep tool**（v2.1.117）——deep-dive 要写 Forgify Go 实现时按"独立 tool"方向走，但要意识到 Claude Code 自身已 inline 化
2. **Edit `replace_all` bug #51986 仍未修**——抄 Edit 时**不要复刻这个 bug**，应在文档中明确"我们的 replace_all 必须扫一遍校验匹配数 vs 替换数一致"
3. **Bash 大量权限加固**（v2.1.98 - v2.1.113）——抄 Bash 时安全措施要逐条对齐，不能只抄默认行为
4. **WebFetch 自动剥 `<style>`/`<script>`**（v2.1.119）——抄 WebFetch 时一并实现
5. **Edit 不再强制 needsRead**（v2.1.89 起，Bash sed/cat 看过也算）——抄 Edit 的 needsReadFirst 钩子时要支持这个相容路径
6. **PostToolUse hooks 可改写 Edit 输出**（v2.1.121）——记录概念，本期 Forgify 不做 hooks 系统
7. **Read 行号格式 compact + 重读去重**（v2.1.86）——deep-dive 要校准 Forgify 的 Read 输出格式

---

## 信任域总结

- ✅ **多源交叉确认**：Read / Write / Edit / Bash / Glob / Grep / WebFetch / WebSearch / Task* / AskUserQuestion / Agent / Cron* / mcp__pattern
- ⚠️ **单源或文档稀薄**：MultiEdit（v2.1.126 是否仍存在）、PowerShell schema、SendUserMessage 行为细节、ToolSearch 内部 lazy 机制
- ❌ **公开数据无法验证**：Computer / BrowserBatch 完整 schema、TeamCreate/TeamDelete 完整流程、LSP 子命令调用形态

deep-dive 期间补强 ⚠️ 项；❌ 项遇到再单独研究。
