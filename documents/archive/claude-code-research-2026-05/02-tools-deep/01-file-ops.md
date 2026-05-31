# 01 — File Operations Tools 深挖

> 02-tools-deep 系列首篇。覆盖 **Read / Write / Edit**。
> ❌ 不含 MultiEdit（v2.1.126 已下线，详见末节）。
> ⚠️ 不含 NotebookEdit（Forgify ⚠️ P2，远期再做）。

## 信息源与置信度

- **主源**：[Piebald-AI/claude-code-system-prompts](https://github.com/Piebald-AI/claude-code-system-prompts)（每个 description 文件 header 标 `ccVersion`，本篇引用版本如下）
  - `tool-description-readfile.md` — ccVersion **2.1.121**
  - `tool-description-write.md` — ccVersion **2.1.97**
  - `tool-description-write-read-existing-file-first.md` — ccVersion **2.1.120**（Read-First 强制变体）
  - `tool-description-edit.md` — ccVersion **2.1.91**
- **副源**：[code.claude.com tools-reference](https://code.claude.com/docs/en/tools-reference)、[anthropics/claude-code GitHub Issues](https://github.com/anthropics/claude-code/issues)、上一轮 deep-dive agent 调研
- **写作日期**：2026-05-03

⚠️ **关于 description 的 template 形式**：Piebald-AI 原文是 **template** 含 `${VARIABLE}` 占位符（例如 `${MAX_LINES_CONSTANT}`、`${LINE_NUMBER_PREFIX_FORMAT}`、`${CAT_DASH_N_NOTE}`）。实际下发到 LLM 时这些占位符会被环境运行时值替换。Forgify 不需要复刻 template 系统——直接 hard-code 数值即可，本文给出的"最终下发文本"已展开占位符。

⚠️ **MultiEdit 已下线确认**：经独立 GitHub API 验证，Piebald-AI 的 system-prompts 目录在 v2.1.126 时间点**不再含 multiedit.md**；issue #11125 关闭为 "not planned"。Forgify 原 inventory 中标 ⚠️ 待校准的位置在此**正式定论：跳过**，理由见末节。

---

## Read（读文件）

### 1. Description 原文（Piebald-AI v2.1.121）

> Reads a file from the local filesystem. You can access any file directly by using this tool.
> Assume this tool is able to read all files on the machine. If the User provides a path to a file assume that path is valid. It is okay to read a file that does not exist; an error will be returned.
>
> Usage:
> - The file_path parameter must be an absolute path, not a relative path
> - By default, it reads up to **${MAX_LINES_CONSTANT}** lines starting from the beginning of the file**${CONDITIONAL_LENGTH_NOTE}**
> - **${CAT_DASH_N_NOTE}**
> - **${READ_FULL_FILE_NOTE}**
> - This tool allows Claude Code to read images (eg PNG, JPG, etc). When reading an image file the contents are presented visually as Claude Code is a multimodal LLM.
> - **${CAN_READ_PDF_FILES_FN()}** *(conditional)*: This tool can read PDF files (.pdf). For large PDFs (more than 10 pages), you MUST provide the pages parameter to read specific page ranges (e.g., pages: "1-5"). Reading a large PDF without the pages parameter will fail. Maximum 20 pages per request.
> - This tool can read Jupyter notebooks (.ipynb files) and returns all cells with their outputs, combining code, text, and visualizations.
> - This tool can only read files, not directories. To list files in a directory, use the registered shell tool.
> - You will regularly be asked to read screenshots. If the user provides a path to a screenshot, ALWAYS use this tool to view the file at the path. This tool will work with all temporary file paths.
> - If you read a file that exists but has empty contents you will receive a system reminder warning in place of file contents.
> - **${HAS_ADDITIONAL_READ_NOTE_FN()}** *(conditional)*: **${ADDITIONAL_READ_NOTE}**

#### 占位符典型值

| 占位符 | 典型值 | 备注 |
|---|---|---|
| `MAX_LINES_CONSTANT` | `2000` | 默认行数上限 |
| `CONDITIONAL_LENGTH_NOTE` | `When you already know which part of the file you need, only read that part. This can be important for larger files.` | 大文件提示，常注入 |
| `CAT_DASH_N_NOTE` | `Results are returned using cat -n format, with line numbers starting at 1` | line-numbering 说明 |
| `READ_FULL_FILE_NOTE` | （多数情况下空） | 特殊环境用 |
| `CAN_READ_PDF_FILES_FN()` | true/false 视宿主能力 | 决定 PDF 段是否注入 |
| `HAS_ADDITIONAL_READ_NOTE_FN()` | 常 false | MCP 注入 hook |

### 2. JSON Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "additionalProperties": false,
  "required": ["file_path"],
  "properties": {
    "file_path": {
      "type": "string",
      "description": "The absolute path to the file to read"
    },
    "offset": {
      "type": "number",
      "description": "The line number to start reading from. Only provide if the file is too large to read at once"
    },
    "limit": {
      "type": "number",
      "description": "The number of lines to read. Only provide if the file is too large to read at once"
    },
    "pages": {
      "type": "string",
      "description": "Page range for PDF files (e.g., \"1-5\", \"3\", \"10-20\"). Only applicable to PDF files. Maximum 20 pages per request."
    }
  }
}
```

✅ 多源确认（bgauryy gist + wong2 gist 一致）。

### 3. 算法行为

**行号格式** ✅
- 模板：`%5d\t%s` —— 5 位右对齐数字 + tab + 行内容
- 行号从 1 开始
- LLM 看到的形如：
  ```
       1	package main
       2	
       3	import "fmt"
  ```

**默认值**
- `offset`：缺省 1（从第 1 行开始）
- `limit`：缺省 2000 行
- 总 token 软上限 ~25,000；超出会触发 token cap 错误，提示 LLM 用 offset/limit 翻页 ✅ ([#4002](https://github.com/anthropics/claude-code/issues/4002))

**多模态处理** ✅
- **图片**：PNG/JPG/GIF/WebP 等 → base64 inline 注入 LLM（multimodal）；无明确 size cap 公开
- **PDF**：≥10 页强制要求 `pages` 参数；上限 20 页/次；按页提取文本 + 视觉缩略图
- **Jupyter notebook**：所有 cell + 输出连成一段；无结构化字段

**v2.1.86 优化**（compact format + dedup） ⚠️ 单源 changelog
- 行号格式 token 占用减少
- **同 session 内重读未变文件不会重发内容**——LLM 看到 system reminder 提示"file unchanged since last read"。机制推测：每个 path 记 (size, mtime, hash) 三元组

**v2.1.105 / v2.1.126 移除 malware-assessment 提示** ✅
- 老模型遇到此提示会无故拒绝读敏感文件名
- 行为上无变化，仅 prompt 净化

**Edge cases**
- 文件不存在 → 错误（不 panic）
- 权限不足 → 错误
- 二进制文件 → 截断 + 转义输出
- Symlink → 解析后读真实路径
- 0 字节 → 系统提示替代 content
- 文件 > limit → 截断 + 暗示用 offset/limit 翻页

**并发性**
- `IsReadOnly() = true`
- 并发安全：**是**（多个并发 Read 不会互相影响）—— LLM 同 turn 多个 Read 应放同 `execution_group` 加速

### 4. 已知 bugs / edge cases

| Issue | 状态 | 详情 |
|---|---|---|
| [#36654](https://github.com/anthropics/claude-code/issues/36654) | Closed dup | Read 输出的行号前缀占视觉 ~8 列；LLM 估算行宽时偏小，Edit 替换时容易错位 wrap |
| [#20223](https://github.com/anthropics/claude-code/issues/20223) | Open | 行号格式 token 开销 ~70%；v2.1.86 compact format 只解决一部分 |
| [#34304](https://github.com/anthropics/claude-code/issues/34304) | Open (feature) | 社区呼吁 AST-aware Read（按符号读，省 context） |

### 5. 输出格式给 LLM

普通文本文件返回字符串：
```
     1	<line 1 content>
     2	<line 2 content>
...
  2000	<line 2000 content>
```

无额外 header / footer / metadata（无 file size / mtime 等）。空文件返回：
```
<system-reminder>File exists but has empty contents.</system-reminder>
```

### 6. Forgify Go 实现要点

#### 6.1 Tool 接口（10 方法）

```go
type Read struct {
    workspace PathGuard // Phase 5
    state     *AgentState        // 用于 MarkRead，配 Edit/Write 必读约束
}

func (t *Read) Name() string                  { return "Read" }
func (t *Read) Description() string           { return readDesc }  // 占位符已展开
func (t *Read) Parameters() json.RawMessage   { return readSchema }

func (t *Read) IsReadOnly() bool              { return true }
func (t *Read) NeedsReadFirst() bool          { return false }
func (t *Read) RequiresWorkspace() bool       { return true }   // Phase 5 强约束

func (t *Read) ValidateInput(args json.RawMessage) error { /* 见 6.3 */ }
func (t *Read) CheckPermissions(_ json.RawMessage, _ toolapp.PermissionMode) toolapp.PermissionResult {
    return toolapp.PermissionAllow
}

func (t *Read) Execute(ctx context.Context, argsJSON string) (string, error) { /* 见 6.4 */ }
```

#### 6.2 依赖

- v1：纯 stdlib `os` + `bufio.Scanner`
- v2 加 PDF：`github.com/ledongthuc/pdf` 或 `gen2brain/go-fitz`（fitz 有 CGO 依赖，权衡）
- 图片：`net/http` 检测 MIME + base64 inline
- Jupyter：标准 JSON 解析（.ipynb 是 JSON），按 cell 类型遍历

#### 6.3 Validate 错误码（进 errmap）

| Sentinel | HTTP 等价 | 触发条件 |
|---|---|---|
| `ErrInvalidPath` | 400 | file_path 非绝对路径 |
| `ErrInvalidOffset` | 400 | offset < 1 |
| `ErrInvalidLimit` | 400 | limit < 1 |
| `ErrPathOutsideWorkspace` | 422 | Phase 5 workspace 守卫 |

#### 6.4 Execute 失败返回（作为 tool_result）

| 错误 | 给 LLM 的 message |
|---|---|
| 文件不存在 | `"File not found: /abs/path"` |
| 权限不足 | `"Permission denied: /abs/path"` |
| 路径在 workspace 外 | `"Path outside allowed workspace: /abs/path"` |
| 文件超 limit | 仅返回 limit 行 + 提示 `"... [truncated; use offset/limit to read more]"` |

#### 6.5 关键代码片段

```go
func (t *Read) Execute(ctx context.Context, argsJSON string) (string, error) {
    var args struct {
        FilePath string `json:"file_path"`
        Offset   int    `json:"offset"`
        Limit    int    `json:"limit"`
    }
    if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
        return "", fmt.Errorf("Read.Execute: %w", err)
    }
    if args.Offset == 0 { args.Offset = 1 }
    if args.Limit == 0 { args.Limit = 2000 }

    f, err := os.Open(args.FilePath)
    if err != nil {
        return fmt.Sprintf("File not found or unreadable: %s", args.FilePath), nil
    }
    defer f.Close()

    info, _ := f.Stat()
    if info.Size() == 0 {
        // 标记为已读（让 Write/Edit 通过约束），但提示空
        t.state.MarkRead(args.FilePath, 0)
        return "<system-reminder>File exists but has empty contents.</system-reminder>", nil
    }

    var sb strings.Builder
    scanner := bufio.NewScanner(f)
    scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024) // 8MB single-line ceiling
    lineNum := 0
    written := 0
    for scanner.Scan() {
        lineNum++
        if lineNum < args.Offset {
            continue
        }
        if written >= args.Limit {
            sb.WriteString(fmt.Sprintf("... [truncated at line %d; use offset+limit to read more]\n", lineNum-1))
            break
        }
        fmt.Fprintf(&sb, "%5d\t%s\n", lineNum, scanner.Text())
        written++
    }
    if err := scanner.Err(); err != nil {
        return "", fmt.Errorf("Read.Execute: scan: %w", err)
    }
    t.state.MarkRead(args.FilePath, info.Size())
    return sb.String(), nil
}
```

#### 6.6 测试要点

- 常规文本文件（.go / .md / .yaml）
- 空文件
- 缺失文件
- 权限不足
- Symlink → 读真实文件
- offset > 文件总行数 → 空结果
- limit < 文件总行数 → 截断 + 提示
- 并发 N 个 Read 同文件 → 全成功
- 单行 > 8MB → ErrLineTooLong（应优雅退出而非崩溃）
- workspace 外路径（Phase 5）→ ErrPathOutsideWorkspace

---

## Write（写文件）

### 1. Description 原文

Claude Code 有**两个**版本的 Write description，选哪个由环境决定：

#### 默认版（Piebald v2.1.97）

> Writes a file to the local filesystem.
>
> Usage:
> - This tool will overwrite the existing file if there is one at the provided path.**${GET_NEW_FILE_NOTE_FN()}**
> - Prefer the Edit tool for modifying existing files — it only sends the diff. Only use this tool to create new files or for complete rewrites.
> - NEVER create documentation files (*.md) or README files unless explicitly requested by the User.
> - Only use emojis if the user explicitly requests it. Avoid writing emojis to files unless asked.

#### Read-First 强制版（Piebald v2.1.120）

> Writes a file to the local filesystem. Overwrites if the file exists.
>
> - If the file already exists, you must **${READ_TOOL_NAME}** it first in this conversation or the call will fail.
> - Prefer Edit for modifying existing files — it only sends the diff.

**Forgify 用哪个？** 强制版——一致性优于宽容；让 LLM 习惯"先 Read 再 Write"流。

### 2. JSON Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "additionalProperties": false,
  "required": ["file_path", "content"],
  "properties": {
    "file_path": {
      "type": "string",
      "description": "The absolute path to the file to write (must be absolute, not relative)"
    },
    "content": {
      "type": "string",
      "description": "The content to write to the file"
    }
  }
}
```

### 3. 算法行为

**Must-Read-First 强制** ✅
- 已存在的文件，没经 Read 工具读过的话 Write 失败
- 跟踪：in-memory per-session map（绝对路径 → size at read time）
- session 结束清空
- v2.1.89 起 Bash `cat / sed / head / tail` 也算"已 Read"（patten matching；后述）

**CRLF 处理** ✅
- v2.1.89 修复了 Windows 上 Write 把 `\r\n` 双重写成 `\r\r\n` 的 bug
- v2.1.89 修复了 Markdown 硬换行（行尾两个空格）被静默剥除的 bug
- 当前（v2.1.126）：CRLF 原样保留；不强制平台换行符

**原子写** ⚠️
- 推测使用 temp file + rename pattern（不是直接 truncate + write）
- 半写入状态不会出现：要么完全写入新内容、要么原文件保留

**File mode** ⚠️
- 推测 0644（owner 可写、所有人可读）

**Parent directory**
- **不**自动创建——missing parent 直接报错
- LLM 需主动 `Bash mkdir -p ...` 或 `LS` 验证后再 Write

**Edge cases**
- 文件已存在 + 没 Read 过 → fail with enforcement error
- Parent 不存在 → fail
- 权限不足 → fail
- 磁盘满 → fail（atomic rollback）
- Path traversal（`../etc/passwd`）→ workspace 守卫拦下

**并发性**
- `IsReadOnly() = false`
- 并发安全：**否**（多个并发 Write 同文件会损坏）—— LLM 应给每个 Write 独立 `execution_group` 串行

### 4. 已知 bugs / edge cases

| Issue | 状态 | 详情 |
|---|---|---|
| [#4462](https://github.com/anthropics/claude-code/issues/4462) | Open | subagent Write 报成功但磁盘没文件——agent 清理 race |
| [#12805](https://github.com/anthropics/claude-code/issues/12805) | Closed (not planned) | Windows MINGW 下 Edit/Write fail with "File has been unexpectedly modified" |
| [#2805](https://github.com/anthropics/claude-code/issues/2805) | Open | Linux 下创建文件用 Windows 行尾——跨平台一致性 |
| [#23478](https://github.com/anthropics/claude-code/issues/23478) | Open | `.claude/rules/` 的 path-based rule 只对 Read 生效，Write 不读 |

### 5. 输出格式

```
File successfully written to /absolute/path/to/file
```

无 size / checksum / 行数确认。

### 6. Forgify Go 实现要点

#### 6.1 Tool 接口

```go
type Write struct {
    workspace PathGuard
    state     *AgentState
}

func (t *Write) Name() string                  { return "Write" }
func (t *Write) IsReadOnly() bool              { return false }
func (t *Write) NeedsReadFirst() bool          { return true }   // 已存在文件需先 Read
func (t *Write) RequiresWorkspace() bool       { return true }
```

#### 6.2 Validate 错误码

| Sentinel | 触发 |
|---|---|
| `ErrInvalidPath` | 非绝对路径 |
| `ErrEmptyContent` | content == ""（防护性，LLM 应知道） |
| `ErrPathOutsideWorkspace` | Phase 5 workspace 守卫 |

#### 6.3 Execute 失败返回

| 错误 | 给 LLM message |
|---|---|
| 已存在文件未 Read | `"File must be read first before overwriting: /abs/path"` |
| 父目录不存在 | `"Parent directory does not exist: /parent/dir"` |
| 权限不足 | `"Permission denied: /abs/path"` |
| 磁盘满 | `"Write failed: no space left on device"` |

#### 6.4 关键代码片段

```go
func (t *Write) Execute(ctx context.Context, argsJSON string) (string, error) {
    var args struct {
        FilePath string `json:"file_path"`
        Content  string `json:"content"`
    }
    if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
        return "", fmt.Errorf("Write.Execute: %w", err)
    }

    // 已存在 → 必须 Read 过
    if _, err := os.Stat(args.FilePath); err == nil {
        if _, ok := t.state.WasRead(args.FilePath); !ok {
            return fmt.Sprintf("File must be read first before overwriting: %s", args.FilePath), nil
        }
    } else if !os.IsNotExist(err) {
        return fmt.Sprintf("Cannot stat path: %s (%v)", args.FilePath, err), nil
    }

    // 父目录不自动创建
    parent := filepath.Dir(args.FilePath)
    if _, err := os.Stat(parent); os.IsNotExist(err) {
        return fmt.Sprintf("Parent directory does not exist: %s", parent), nil
    }

    // Atomic write: temp file + rename
    tmp, err := os.CreateTemp(parent, ".forgify-write-*")
    if err != nil {
        return "", fmt.Errorf("Write.Execute: create temp: %w", err)
    }
    tmpPath := tmp.Name()
    if _, err := tmp.WriteString(args.Content); err != nil {
        tmp.Close()
        os.Remove(tmpPath)
        return "", fmt.Errorf("Write.Execute: write temp: %w", err)
    }
    if err := tmp.Close(); err != nil {
        os.Remove(tmpPath)
        return "", fmt.Errorf("Write.Execute: close temp: %w", err)
    }
    if err := os.Rename(tmpPath, args.FilePath); err != nil {
        os.Remove(tmpPath)
        return "", fmt.Errorf("Write.Execute: rename: %w", err)
    }

    // 写后自动标记为"已读"（避免 LLM 再 Read 一次）
    t.state.MarkRead(args.FilePath, int64(len(args.Content)))
    return fmt.Sprintf("File successfully written to %s", args.FilePath), nil
}
```

#### 6.5 测试要点

- 新建文件
- 覆写已 Read 的文件
- 覆写未 Read 的文件 → 拒绝
- 父目录缺失 → 错误
- 路径在 workspace 外 → 拒绝
- 权限不足 → 错误
- atomic：写入中途 panic 不损坏原文件（用 `t.Cleanup` 模拟）
- 并发 2 个 Write 同文件 → 应被 runTools 串行（不在本 tool 测试范围，是 chat tool 调度测试）

---

## Edit（精确字符串替换）

### 1. Description 原文（Piebald v2.1.91）

> Performs exact string replacements in files.
>
> Usage:**${MUST_READ_FIRST_FN()}**
> - When editing text from Read tool output, ensure you preserve the exact indentation (tabs/spaces) as it appears AFTER the line number prefix. The line number prefix format is: **${LINE_NUMBER_PREFIX_FORMAT}**. Everything after that is the actual file content to match. Never include any part of the line number prefix in the old_string or new_string.
> - ALWAYS prefer editing existing files in the codebase. NEVER write new files unless explicitly required.
> - Only use emojis if the user explicitly requests it. Avoid adding emojis to files unless asked.**${ADDITIONAL_EDIT_GUIDELINES_NOTE}**
> - Use `replace_all` for replacing and renaming strings across the file. This parameter is useful if you want to rename a variable for instance.

#### 占位符典型值

| 占位符 | 典型值 |
|---|---|
| `MUST_READ_FIRST_FN()` | 一段话："You must use your `Read` tool at least once in the conversation before editing. This tool will error if you attempt an edit without reading the file." |
| `LINE_NUMBER_PREFIX_FORMAT` | `"line number + tab"` 或更详细的 `"5-space-padded line number followed by a tab"` |
| `ADDITIONAL_EDIT_GUIDELINES_NOTE` | 通常空，是注入点 |

### 2. JSON Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "additionalProperties": false,
  "required": ["file_path", "old_string", "new_string"],
  "properties": {
    "file_path": {
      "type": "string",
      "description": "The absolute path to the file to modify"
    },
    "old_string": {
      "type": "string",
      "description": "The text to replace"
    },
    "new_string": {
      "type": "string",
      "description": "The text to replace it with (must be different from old_string)"
    },
    "replace_all": {
      "type": "boolean",
      "default": false,
      "description": "Replace all occurrences of old_string (default false)"
    }
  }
}
```

✅ 字段名 confirmed `old_string` / `new_string`（不是 `from`/`to`）。

### 3. 算法行为

**匹配算法** ✅
- 字面量匹配（`String.indexOf` 等价物，无 regex）
- 大小写敏感、空格敏感、缩进敏感
- old_string 可跨行（含 `\n`）

**0 / 1 / N 匹配的处理**

| 匹配数 | replace_all=false | replace_all=true |
|---|---|---|
| 0 | 错：`"The string to replace was not found in the file."` | 同左 |
| 1 | 替换那一处 | 替换那一处 |
| N (>1) | 错：`"Found N matches of the string to replace, but replace_all is false."` | 替换全部 |

**🚨 #51986 关键 bug** ❌（CC 侧）
- **状态**：v2.1.126 仍 open
- **症状**：`replace_all=true` 在某些 markdown 模式（如 ` **[WEAK]**` 即"加粗结尾 + 空格"前后）会**静默跳过部分匹配**且**消耗邻接 newline**，但 tool 仍报告 "All occurrences replaced"
- **重现**：5 处 ` **[WEAK]**` 的 markdown，只替换 2 处，3 行被合并成 1 行
- **影响**：用户信任成功消息，不去 grep 验证
- **workaround（Claude Code 用户视角）**：避免 replace_all，改用多次 single edit
- **Forgify 视角**：bug 根源在 JS 实现（推测 V8 indexOf 边缘行为）；Go 的 `strings.ReplaceAll` 语义是良好定义的（替换所有非重叠出现），不会复现此 bug。Forgify 信任 stdlib 不加额外校验。**仍要做的事**：在 success message 里**显式报告替换 N 次**（比 CC 多一层透明），让用户看到数字而非笼统的 "All replaced"

**v2.1.89 needsRead 放宽** ✅
- Edit 现在接受"通过 Bash `sed -n` / `cat` / `head` / `tail` 看过"的文件
- 实现机制：framework parse Bash 调用，识别 read pattern → 标记为 seen
- Forgify Phase 5 加这个

**v2.1.121 PostToolUse hook integration** ✅
- hooks 可以通过 `hookSpecificOutput.updatedToolOutput` **改写** Edit 的返回内容
- 用途：format-on-save、lint-then-respond
- Forgify 暂不做 hooks 系统，但要预留接口位

**v2.1.113 file state tracking 修复**
- 老问题：format-on-save hook 改文件后，下一次 Edit 误报 "File has been modified" 拒绝
- v2.1.113 后能识别 hook-driven 改动，允许继续

**Edge cases**
- old_string == new_string → 拒绝（no-op edit；schema description 已 hint）
- old_string 跨行 → 支持（newlines 算 match 一部分）
- 文件被外部改动 → fail with "File has been modified"（用户需重 Read）
- Symlink → 解析后 edit 真实文件

**并发性**
- `IsReadOnly() = false`
- 并发安全：**否**（同文件并发 Edit 会冲突）—— LLM 应给每个 Edit 独立 `execution_group` 串行

### 4. 已知 bugs / edge cases

| Issue | 状态 | 详情 |
|---|---|---|
| [#51986](https://github.com/anthropics/claude-code/issues/51986) | **OPEN** 🚨 | replace_all 静默跳过 + 吃 newline，见 §3 |
| [#3513](https://github.com/anthropics/claude-code/issues/3513) | Open | 新建文件即便重 Read 后 Edit 仍报 "File has been modified"——false positive |
| [#7443](https://github.com/anthropics/claude-code/issues/7443) | Open | Linux 下 #3513 变种 |
| [#13456](https://github.com/anthropics/claude-code/issues/13456) | Open | CRLF 文件 Edit 失败——v2.1.89 修了 Write 但 Edit 还有 |
| [#23741](https://github.com/anthropics/claude-code/issues/23741) | Open | Opus 4.6 处理空格/tab 时混乱（diff 分隔符歧义） |

### 5. 输出格式

成功（single）：
```
The file /abs/path has been updated. Here's the result of running `cat -n` on a snippet of the edited file:
   42	<line containing new_string>
   43	<adjacent lines for context>
```

成功（replace_all）：
```
The file /abs/path has been updated. All N occurrences of the string have been replaced.
```

⚠️ Claude Code **不显式报告 N**——这正是 #51986 容易蒙混过关的根因之一。Forgify 显式报告 N（透明度优先，不是为了防 bug——Go stdlib 不会有这个 bug）。

### 6. Forgify Go 实现要点

#### 6.1 Tool 接口

```go
type Edit struct {
    workspace PathGuard
    state     *AgentState
}

func (t *Edit) Name() string                  { return "Edit" }
func (t *Edit) IsReadOnly() bool              { return false }
func (t *Edit) NeedsReadFirst() bool          { return true }
func (t *Edit) RequiresWorkspace() bool       { return true }
```

#### 6.2 Validate 错误码

| Sentinel | 触发 |
|---|---|
| `ErrInvalidPath` | 非绝对路径 |
| `ErrEditNoOp` | old_string == new_string |
| `ErrEmptyOldString` | old_string == ""（防护） |
| `ErrPathOutsideWorkspace` | Phase 5 |

#### 6.3 Execute 失败返回

| 错误 | 给 LLM message |
|---|---|
| 文件未 Read | `"File must be read first before editing: /abs/path"` |
| 文件不存在 | `"File not found: /abs/path"` |
| 0 匹配 | `"The string to replace was not found in the file."` |
| N>1 匹配 + replace_all=false | `"Found N matches of the string to replace, but replace_all is false."` |
| 文件外部改动 | `"File has been modified since last read; please Read again before editing: /abs/path"` |

#### 6.4 关键代码片段

```go
func (t *Edit) Execute(ctx context.Context, argsJSON string) (string, error) {
    var args struct {
        FilePath   string `json:"file_path"`
        OldString  string `json:"old_string"`
        NewString  string `json:"new_string"`
        ReplaceAll bool   `json:"replace_all"`
    }
    if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
        return "", fmt.Errorf("Edit.Execute: %w", err)
    }

    // 已 Read 守卫
    seenSize, ok := t.state.WasRead(args.FilePath)
    if !ok {
        return fmt.Sprintf("File must be read first before editing: %s", args.FilePath), nil
    }

    // 读当前内容
    raw, err := os.ReadFile(args.FilePath)
    if err != nil {
        return fmt.Sprintf("File not found or unreadable: %s", args.FilePath), nil
    }

    // 检查文件是否被外部改动
    info, _ := os.Stat(args.FilePath)
    if info.Size() != seenSize {
        return fmt.Sprintf("File has been modified since last read; please Read again before editing: %s", args.FilePath), nil
    }

    content := string(raw)

    // 计数前
    occurrences := strings.Count(content, args.OldString)
    if occurrences == 0 {
        return "The string to replace was not found in the file.", nil
    }
    if occurrences > 1 && !args.ReplaceAll {
        return fmt.Sprintf("Found %d matches of the string to replace, but replace_all is false.", occurrences), nil
    }

    // 替换（信任 stdlib：strings.ReplaceAll 替换所有非重叠出现，语义明确）
    var newContent string
    var replacements int
    if args.ReplaceAll {
        newContent = strings.ReplaceAll(content, args.OldString, args.NewString)
        replacements = occurrences
    } else {
        newContent = strings.Replace(content, args.OldString, args.NewString, 1)
        replacements = 1
    }

    // 原子写
    tmp, err := os.CreateTemp(filepath.Dir(args.FilePath), ".forgify-edit-*")
    if err != nil {
        return "", fmt.Errorf("Edit.Execute: create temp: %w", err)
    }
    tmpPath := tmp.Name()
    if _, err := tmp.WriteString(newContent); err != nil {
        tmp.Close()
        os.Remove(tmpPath)
        return "", fmt.Errorf("Edit.Execute: write temp: %w", err)
    }
    tmp.Close()
    if err := os.Rename(tmpPath, args.FilePath); err != nil {
        os.Remove(tmpPath)
        return "", fmt.Errorf("Edit.Execute: rename: %w", err)
    }

    // 更新 seen 状态（size 变了）
    newInfo, _ := os.Stat(args.FilePath)
    t.state.MarkRead(args.FilePath, newInfo.Size())

    // 显式报告 N（比 Claude Code 多一层透明，让用户知道实际替换次数）
    if args.ReplaceAll {
        return fmt.Sprintf("The file %s has been updated. Replaced %d occurrence(s).", args.FilePath, replacements), nil
    }
    return fmt.Sprintf("The file %s has been updated.", args.FilePath), nil
}
```

#### 6.5 测试要点

- 单匹配 + replace_all=false → 替换那一处
- 0 匹配 → 错误消息
- N>1 + replace_all=false → 错误消息含计数
- N>1 + replace_all=true → 全替换 + success message 报告 N
- old_string == new_string → 拒绝
- old_string 跨行（含 `\n`）→ 支持
- old_string 含 regex 元字符（`.+*$`）→ 字面量匹配，不当 regex
- new_string 含 old_string（如 `x` → `xy` / `x` → `xx`）→ stdlib 行为正确
- 文件被外部改动 → 错误
- 工具未 Read 直接 Edit → 错误
- 并发 2 个 Edit 同文件 → runTools 串行（不在本 tool 测试范围）

---

## MultiEdit 现状（不抄）

经多源验证：

1. **Piebald-AI 不再含 multiedit.md**（GitHub API 列目录直接确认）
2. **Issue #11125 关闭为 "not planned"**——social proof 证实下线
3. 老 issue（#5234 / #2154 / #7197 / #2396）描述的多个 bug 与下线决策一致

**Forgify 替代方案**：
- LLM 想批量改？同一 turn 内调多次 Edit
- LLM 同 turn 内调多次 Edit 时给每个独立 `execution_group` 即天然串行——给到 batch 的"原子性 + 可见性"，缺点是不能 rollback
- 真要 atomic？写前快照 → 任一失败 restore——但仪式感重，单用户场景**不值得**

**inventory 修订**：本结论同步回 `00-inventory.md` 把 MultiEdit 从 P1 移到"已下线，跳过"。

---

## 总结：本批实施估时

| 工具 | 估时 | 难点 |
|---|---|---|
| Read | 0.5 天 | 多模态 v1 跳过；行号格式严格匹配 |
| Write | 0.3 天 | atomic 写 + Read-First 跨 tool 状态 |
| Edit | 0.5 天 | Read-First + 跨 tool 状态共用（信任 stdlib，不另加 #51986 防御） |

**合计 ~1.5 天**（含测试）。

## 跨 tool 共享：AgentState.SeenFiles

Read / Write / Edit 都依赖一个 conversation-scoped 的"已读文件"状态。完整设计如下。

### 1. AgentState 本体（独立 pkg，避免循环依赖）

放在 `internal/pkg/agentstate/`（**不**放 `app/chat/`，否则 `pkg/reqctx` 引它会循环）。

```go
// internal/pkg/agentstate/agentstate.go
package agentstate

import "sync"

type AgentState struct {
    SeenFiles sync.Map // path string → size int64 at read time
}

func (s *AgentState) MarkRead(path string, size int64) {
    s.SeenFiles.Store(path, size)
}

func (s *AgentState) WasRead(path string) (int64, bool) {
    v, ok := s.SeenFiles.Load(path)
    if !ok {
        return 0, false
    }
    return v.(int64), true
}
```

### 2. 挂在 convQueue 上（per-conversation 一份，自动 GC）

```go
// internal/app/chat/chat.go::convQueue 加字段
type convQueue struct {
    ch         chan queuedTask
    mu         sync.Mutex
    cancel     context.CancelFunc
    agentState *agentstatepkg.AgentState  // ← 新加
}

// getOrCreateQueue 创建时一并初始化
func (s *Service) getOrCreateQueue(conversationID string) *convQueue {
    q := &convQueue{
        ch:         make(chan queuedTask, queueCapacity),
        agentState: &agentstatepkg.AgentState{},  // ← 新加
    }
    actual, loaded := s.queues.LoadOrStore(conversationID, q)
    // ...
}
```

conversation idle 5 分钟后 queue 被清，AgentState 一并 GC。

### 3. processTask 注入 ctx

```go
// internal/app/chat/runner.go::processTask
func (s *Service) processTask(conversationID string, q *convQueue, task queuedTask) {
    ctx := task.ctx
    // ... 已有的 cancel + uid + locale 注入
    ctx = reqctxpkg.WithConversationID(ctx, conversationID)  // 已有
    ctx = reqctxpkg.WithAgentState(ctx, q.agentState)        // ← 新加
    // ...
    s.agentRun(ctx, ...)
}
```

### 4. reqctxpkg helper（~15 行）

```go
// internal/pkg/reqctx/agentstate.go (新文件)
package reqctx

import (
    "context"
    agentstatepkg "github.com/sunweilin/forgify/backend/internal/pkg/agentstate"
)

type agentStateKey struct{}

func WithAgentState(ctx context.Context, s *agentstatepkg.AgentState) context.Context {
    return context.WithValue(ctx, agentStateKey{}, s)
}

func GetAgentState(ctx context.Context) (*agentstatepkg.AgentState, bool) {
    s, ok := ctx.Value(agentStateKey{}).(*agentstatepkg.AgentState)
    return s, ok
}
```

### 5. Tool Execute 里读

Tool struct **不持有** AgentState 引用——通过 ctx 拿（保持 Tool stateless，跟现有 forge tool 一致）：

```go
// Read 写入 SeenFiles
func (t *Read) Execute(ctx context.Context, argsJSON string) (string, error) {
    // ... 读文件 ...
    state, _ := reqctxpkg.GetAgentState(ctx)
    if state != nil {
        state.MarkRead(args.FilePath, info.Size())
    }
    return out, nil
}

// Edit / Write 检查 SeenFiles
func (t *Edit) Execute(ctx context.Context, argsJSON string) (string, error) {
    state, _ := reqctxpkg.GetAgentState(ctx)
    if state == nil {
        return "", fmt.Errorf("Edit: agent state missing in context")
    }
    if _, seen := state.WasRead(args.FilePath); !seen {
        return "File must be read first before editing: " + args.FilePath, nil
    }
    // ... 改文件 ...
}
```

### 6. 关键设计决策

- ✅ **包归属在 `pkg/`**——避免 `pkg/reqctx` ⇄ `app/chat` 循环
- ✅ **per-conversation 粒度**——挂在 convQueue 上跟现有 chat 队列模型完全契合；不需要单独管理生命周期
- ✅ **ctx 注入而非 Tool 字段**——tool struct 保持 stateless；跨 tool 共享天然实现（Read 写、Edit/Write 读，互不知对方存在）
- ✅ **lifecycle 自动**——conversation idle GC 时 AgentState 一并清，无内存泄露

### 7. 未来 Phase 5 Bash sed/cat bypass

CC v2.1.89 起 Bash `sed -n '...' /abs/path` / `cat /abs/path` / `head /abs/path` 等命令也算"Read 过"。Forgify Phase 5 加这个：

```go
// Bash.Execute 末尾，cwd 持久化之后
if state, _ := reqctxpkg.GetAgentState(ctx); state != nil {
    for _, p := range parseReadFilesFromBashCmd(args.Command) {
        if info, err := os.Stat(p); err == nil {
            state.MarkRead(p, info.Size())
        }
    }
}
```

`parseReadFilesFromBashCmd` 识别常见 read pattern（cat / head / tail / sed -n / less / more 等）+ 后续路径参数。Phase 5 推进时实现。

---

## 信任度总结

- ✅ **多源确认**：Read/Write/Edit description 原文（Piebald-AI 直接 fetch）/ Read 行号格式 / Edit 字面量匹配 / replace_all bug #51986 / Write must-Read-first
- ⚠️ **单源 / 推测**：Write atomic 实现、File mode 0644、v2.1.86 dedup 算法细节、Read 25K token cap 上限精确值
- ❌ **无法验证 / 已弃用**：MultiEdit 现行 schema、`v2.1.89 Bash sed/cat` 检测的具体 pattern 列表

deep-dive 期间补强 ⚠️ 项；❌ 项遇到再单独研究。
