# 02 — Claude Code 工具完整度（Tool 系统）

## 信息来源与局限

主要参考：
- https://gist.github.com/wong2/e0f34aac66caf890a332f7b6f9e2ba8f （tool 清单 & schema）
- https://gist.github.com/bgauryy/0cdb9aa337d01ae5bd0c803943aa36bd
- https://github.com/Piebald-AI/claude-code-system-prompts （所有 tool description 原文）
- https://code.claude.com/docs/en/tools-reference （官方）
- https://www.aifreeapi.com/en/posts/claude-code-tool-search
- https://www.callsphere.tech/blog/claude-code-tool-system-explained

**局限**：tool 实现的内部 TS 类是 minified 名（`g7`、`U1` 等），公开分析多围绕 description / parameters schema 而非实现代码。

---

## 1. Tool 接口设计

### 1.1 类型签名

✅ Claude Code 的 Tool 在 TS 里大致结构（参考 Anthropic Agent SDK 反推 + 反编译笔记）：

```ts
interface Tool<Input, Output = string> {
  // 必须字段
  name: string                                  // LLM 看到的工具名
  description: string                           // 一段长 prompt（许多带"important note"小节）
  inputSchema: JSONSchema                       // 标准 JSON Schema object

  // 元数据字段（影响 orchestration 与 permission）
  readOnlyHint?: boolean                        // ★ 决定 isConcurrencySafe
  destructiveHint?: boolean                     // 提示用户慎用
  needsReadFirst?: boolean                      // Edit/Write 强制要求先 Read
  requiresWorkspace?: boolean                   // tool 是否要求 cwd 在某个允许目录

  // 钩子
  isConcurrencySafe?(input: Input): boolean     // 默认走 readOnlyHint
  checkPermissions?(input: Input, mode): PermissionResult
  validateInput?(input: Input): string | null

  // 主入口
  call(
    input: Input,
    ctx: ToolContext                            // abortSignal, sessionId, cwd, addContext()...
  ): AsyncGenerator<ToolEvent, Output>          // 流式产出 progress/text，最后 yield 一次结果
}
```

✅ 重点元字段：`readOnlyHint`（驱动并行）、`isConcurrencySafe`（细粒度覆盖）、`needsReadFirst`（Edit 必须先 Read 的强约束）。

⚠️ Plain prompt 的 `description` 文本里大量塞"WHEN TO USE"、"IMPORTANT NOTES"、"CONSTRAINTS"——这是 Claude Code 把"工具使用策略"放在 description 里、而不是放在 system prompt 里的设计选择。

### 1.2 注册机制

✅ Tool 在 session start 时被 **静态收集** 到一个 registry（一个 Map<string, Tool>），同时 MCP tool 在 lazy 模式下只放 stub（详见报告 09）。registry 是 in-memory，进程重启即消失。

---

## 2. 完整工具清单

### 2.1 内置 14 个核心 tool

✅ 来自 Piebald-AI 收集的完整列表（v2.x）：

| 类别 | Tool | 关键 schema | 关键实现细节 |
|---|---|---|---|
| 文件读 | `Read` | `file_path`, `offset?`, `limit?` | 默认 2000 行，line 前缀 `<lineno>\t`；image 转 base64；PDF/notebook 特殊处理；Edit 前必须 Read |
| 文件写 | `Write` | `file_path`, `content` | 已存在文件必须先 Read；覆盖式写 |
| 文件改 | `Edit` | `file_path`, `old_string`, `new_string`, `replace_all?` | **精确字符串替换**；详见 §2.2 |
| 批改 | `MultiEdit` | `file_path`, `edits[]` | 原子：所有 edits 成功才提交；任一失败全部回滚 |
| 笔记本 | `NotebookEdit` | `notebook_path`, `cell_number/cell_id`, `new_source`, `cell_type?`, `edit_mode?` | 0-indexed cell；replace/insert/delete 三模式 |
| 搜索 | `Glob` | `pattern`, `path?` | 标准 glob；返回**按修改时间倒序** |
| 搜索 | `Grep` | `pattern`, `path?`, `include?`, `output_mode?`, `multiline?`, `-A/-B/-C` | 底层 ripgrep；并行；默认 `files_with_matches`；multiline 必须显式开 |
| 列表 | `LS` | `path`, `ignore?` | path 必须绝对路径；ignore 是 glob 列表 |
| 执行 | `Bash` | `command`, `timeout?`, `description?`, `run_in_background?` | 持久 shell session；详见 §2.4 |
| 网页 | `WebFetch` | `url`, `prompt` | HTML→Markdown；15 分钟缓存；用小模型解析；HTTP→HTTPS 自动升级 |
| 网页 | `WebSearch` | `query`, `allowed_domains?`, `blocked_domains?` | 仅美国可用 |
| 任务 | `TodoWrite` | `todos[]` | 详见报告 07 |
| 用户 | `AskUserQuestion` | `questions[]`, `multiSelect?` | 详见报告 07 |
| 元 | `exit_plan_mode` | `plan` | 仅在 plan mode 退出时调用 |

✅ **额外的 orchestration 类**：`Agent` / `Task`（subagent，详见报告 05）、`SendMessage`（teammate inbox）、`EnterWorktree`、`Skill`（启动 skill）、`TaskCreate` / `TaskUpdate` / `TaskStop`（v2.1.16+ 替代 TodoWrite）、`Monitor`（背景流事件）、`PermissionDenied` 等扩展 tool。

⚠️ v2.1.117 起，macOS/Linux native build **移除了 Glob/Grep**，改成嵌入 bfs/ugrep 通过 Bash tool 调用——少一次 round-trip。

### 2.2 Edit 的精确替换算法（重点）

✅ Claude Code Edit 不用 regex，是**纯字符串字面量匹配**：

```
1. 校验：file_path 必须绝对；该文件在本 session 已被 Read（needsReadFirst）
2. 校验：old_string ≠ new_string
3. 读文件全部内容 fileText
4. 计算 occurrences = countSubstring(fileText, old_string)
5. 决策：
   - replace_all=false（默认）：
     - occurrences === 0 → 抛错 "old_string not found"
     - occurrences  >  1 → 抛错 "old_string is not unique, found N occurrences. Provide more context or set replace_all"
     - occurrences === 1 → 替换那一处
   - replace_all=true → 全部替换
6. 校验：写入后再读一次确认结果（diff 给到 LLM 作为 tool_result）
```

✅ **缩进保留**：Read tool 输出格式是 `<行号>\t<内容>`，Edit 校验 old_string 必须**只含正文不含行号前缀**。如果 LLM 误把 `42\tconst x = 1` 当 old_string，会 fail uniqueness 或 0 matches。

❌ 没在公开分析里看到 Edit 用什么算法做大文件 substring search（应该就是 V8 的 indexOf，O(n)）。

⚠️ **已知 bug**（issue #51986）：`replace_all=true` 在某些 edge case 下会静默跳过部分匹配并破坏邻接 newline 而 reportSuccess。社区建议慎用 replace_all。

### 2.3 Glob 的修改时间排序

✅ Glob 返回按 mtime 倒序。底层使用文件系统索引调 fast-glob 或类似库，对每个 match 调 `fs.stat` 拿 mtime。**性能影响**：在 monorepo 跑 `**/*.ts` 时，stat 调用是主要开销。

### 2.4 Grep / ripgrep

✅ 默认使用 `@vscode/ripgrep` npm 包里的 binary（带 Node.js wrapper 开销）。设 `USE_BUILTIN_RIPGREP=0` 可切到系统 PATH 的 rg，5-10× 加速。

✅ Grep 的 `output_mode`：
- `files_with_matches` (默认)：只返回路径
- `content`：返回匹配行
- `count`：每文件计数
- `-A/-B/-C` 上下文行数仅在 content 模式下生效；`multiline: true` 才能跨行匹配

### 2.5 Bash：持久 shell session

✅ Bash tool 维护 **同会话内的工作目录持久**；但**环境变量不持久**——每条命令实际在新 shell 进程跑。已知 bug（issue #2508、#20503）官方文档自相矛盾。

✅ 关键参数：
- `timeout`：默认 120s，最大 600s
- `description`：5-10 字描述（用于 UI 显示）
- `run_in_background`：异步执行，不阻塞，可用 BashOutput tool 拉日志
- 输出截断在 30000 字符

✅ 工作目录的实现：维护一个 in-memory `cwd` 字段，每条 Bash 命令前 prepend `cd <cwd> && `。`cd` 命令解析后更新 `cwd`。

### 2.6 WebFetch 的独立 context window

✅ WebFetch 不会把 HTML 全文倒进主对话 context——它**用一个小快模型（很可能是 Haiku）单独评估 HTML→Markdown 后的内容**，只把最终回答（按用户传的 `prompt` 做摘要）返回主 agent。
- 15 分钟内同一 URL 走缓存
- HTTP 自动升级 HTTPS
- 跨 host 重定向时不自动跟，要求 LLM 重新发 WebFetch 到新 URL（防 SSRF）

❌ 缓存的具体实现位置（disk 还是 in-memory）未在公开分析中找到。

### 2.7 LSP / Code Intelligence

⚠️ Code intelligence（跳定义、查引用、call hierarchy）通过 **plugin** 而不是核心 tool 提供，需要装 `code-intelligence` plugin。背后接 LSP server。具体实现细节未在泄漏分析中找到。

### 2.8 MCP Tool 映射

✅ MCP tool 名格式：`mcp__<server>__<tool>`，例如 `mcp__github__search_repositories`。注册到同一个 Tool registry，但走 lazy loading（详见报告 09）。

---

## 3. tool 输出格式

✅ tool 返回 LLM 的格式是 Anthropic API 的 `tool_result` content block，本质是字符串（或 image content array）。Claude Code 内部把任何 tool 的输出都先 normalize 成 `string`（structured 数据用 JSON.stringify 或定义 markdown 模板）。

✅ 大输出处理：
- Bash：30000 字符硬截断
- Read：默认 2000 行 / 100KB（用 offset/limit 翻页）
- WebFetch：先用小模型摘要，结果通常<5KB
- 通用层：`applyToolResultBudget()` 在每次 LLM call 前对**历史**里的旧 tool result 做"oversized → 替换为 content reference"（详见报告 03）

---

## 4. 对 Forgify 的改进建议

> 现状（共 13 个 tool，`agent/tool.go`/`system.go`/`web.go`/`forge.go`）：
> - read_file 100KB 截断，无 offset/limit
> - write_file 一次性覆盖
> - 无 Edit/MultiEdit/Glob/Grep/LS（只有 list_dir）
> - run_shell：`exec.CommandContext(ctx, "sh", "-c", cmd)` 单次进程，无持久 cwd，无 timeout 配置
> - 无 readOnly/concurrencySafe 元字段

| # | 改进 | 优先级 | 实现要点（Go） |
|---|---|---|---|
| 1 | **新增 `Edit` tool** | P0 | 新文件 `agent/edit.go`：参数 `path, old_string, new_string, replace_all bool`。算法直译 §2.2：`occurrences := strings.Count(content, old_string)`；返回 `unified diff`。需要"先读"约束：在 ctx 维护 `seenFiles map[string]int64`（path→size when read），`Read` 写入、`Edit` 校验。失败错误码：`OLD_STRING_NOT_FOUND` / `OLD_STRING_NOT_UNIQUE`(返回 N 次出现) / `FILE_NOT_READ_FIRST` |
| 2 | **新增 `Glob` tool** | P1 | 用 `doublestar/v4` 或 `bmatcuk/doublestar` 库；遍历 + `os.Stat` 拿 ModTime；按时间倒序；上限 1000 项 |
| 3 | **新增 `Grep` tool** | P1 | 优先尝试调用本机 `rg`（`exec.LookPath`），fallback 到 `bufio.Scanner` + `regexp`；支持 `output_mode`/`include`/`-C` |
| 4 | **新增 `LS` tool**（取代 list_dir） | P2 | 接近 `list_dir` 但加 `ignore []string` glob 列表 + 返回排序更稳定 |
| 5 | **Tool 接口加元字段** | P0 | `tool.go` 接口新增 `IsReadOnly() bool` 和可选 `IsConcurrencySafe(argsJSON string) bool`。所有现有 tool 实现：`read_file`/`list_dir`/`web_search`/`fetch_url`/`datetime` → readOnly=true；其余 false。`runTools` 用此分批（见报告 01 改进 #4） |
| 6 | **Bash 持久 cwd** | P1 | `RunShellTool` 加状态：在 `ctx` 里关联 `*shellState{ cwd string; mu sync.Mutex }`（service 启动时建一个，整个 session 共享）。Execute 时 `cmd := "cd "+cwd+" && "+args.Command`；解析 `cd` 命令更新 cwd |
| 7 | **超时配置 + 后台运行** | P1 | RunShell 参数加 `timeout int` 和 `run_in_background bool`。后台运行实现：起 goroutine + 把 stdout/stderr 写到 `~/.forgify/bg/<id>.log`；新增 `bg_output` tool 拉日志 |
| 8 | **read_file 加 offset/limit** | P2 | `args` 加 `offset int, limit int`；按行切片 |
| 9 | **WebFetch（HTML→Markdown）** | P3 | 现 fetch_url 已经用 Jina Reader（很好），加 `prompt` 参数 + 调小模型摘要，结构上像 Claude Code |
| 10 | **MultiEdit** | P3 | 实现可选；先 Edit 做好再考虑 |

最先做：**#1 + #5 + #6**（Edit 是最大缺口；元字段是 #4 报告 01 的前提；Bash cwd 持久是用户体验大改善）。

