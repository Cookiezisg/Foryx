# 02 — Search Tools 深挖

> 02-tools-deep 系列第二篇。
> ✅ **Grep** — Claude Code 标准 tool（v2.1.126 在）
> ✅ **Glob** — Claude Code 标准 tool（v2.1.126 在）；Forgify 升级为 **"找 + 列"统一 tool**（输出加 type/size/mtime）
> ❌ **LS** — **跟 CC 一起砍**（Glob 覆盖目录列出场景，详 §下面）

## 信息源与置信度

- **主源**：
  - **Grep** — [Piebald-AI `tool-description-grep.md`](https://github.com/Piebald-AI/claude-code-system-prompts/blob/main/system-prompts/tool-description-grep.md)（ccVersion **2.0.14**——CC v2.0.14 起基本未变，描述很成熟）
  - **Glob** — Piebald-AI 不含独立描述（仅在 `tool-description-bash-alternative-file-search.md` 提到 `${GLOB_TOOL_NAME}` 占位符，说明仍是 standalone tool 名）；schema 来自 [wong2 gist 历史快照](https://gist.github.com/wong2/e0f34aac66caf890a332f7b6f9e2ba8f)与 [code.claude.com tools-reference](https://code.claude.com/docs/en/tools-reference)
- **副源**：[GitHub Issues](https://github.com/anthropics/claude-code/issues)、`bash-alternative-*.md` 系列（侧面证据）
- **写作日期**：2026-05-03

### 三个 tool 在 v2.1.126 当前状态对照

| 工具 | v2.1.126 状态 | Piebald 描述 | 官方 tools-reference 列表 | Forgify 处置 |
|---|---|---|---|---|
| `Grep` | ✅ 标准 tool | ✅ 完整 (ccVersion 2.0.14) | ✅ 在 | 完整复刻 |
| `Glob` | ✅ 标准 tool | ⚠️ 无独立描述（仅 placeholder `${GLOB_TOOL_NAME}`） | ✅ 在 | 复刻 + **输出升级**（每条带 type/size/mtime；既覆盖"找文件"也覆盖"列目录"，吃掉 LS 的需求） |
| `LS` | ❌ **已下线** | ❌ 无 | ❌ 不在 | ❌ **跟着砍**——CC 把 ls 收并到 Glob 是更好设计（少一个 tool，一个 pattern 解决两件事）；Forgify 直接借鉴 |

> 旁证：`tool-description-bash-alternative-file-search.md` 原文 `"File search: Use ${GLOB_TOOL_NAME} (NOT find or ls)"` —— CC 把 `ls` 视为 Bash 命令，不是 dedicated tool。Forgify 进一步：让 Glob 输出 type/size/mtime，**结构化得比 CC 还彻底**，非编程用户也能从 Glob 拿到"这个目录里有什么"的清晰答案。

---

## Grep（按内容搜索）

### 1. Description 原文（Piebald v2.0.14）

> A powerful search tool built on ripgrep
>
> Usage:
> - ALWAYS use **${GREP_TOOL_NAME}** for search tasks. NEVER invoke `grep` or `rg` as a **${BASH_TOOL_NAME}** command. The **${GREP_TOOL_NAME}** tool has been optimized for correct permissions and access.
> - Supports full regex syntax (e.g., `"log.*Error"`, `"function\s+\w+"`)
> - Filter files with `glob` parameter (e.g., `"*.js"`, `"**/*.tsx"`) or `type` parameter (e.g., `"js"`, `"py"`, `"rust"`)
> - Output modes: `"content"` shows matching lines, `"files_with_matches"` shows only file paths (default), `"count"` shows match counts
> - Use **${TASK_TOOL_NAME}** tool for open-ended searches requiring multiple rounds
> - Pattern syntax: Uses ripgrep (not grep) - literal braces need escaping (use `interface\{\}` to find `interface{}` in Go code)
> - Multiline matching: By default patterns match within single lines only. For cross-line patterns like `struct \{[\s\S]*?field`, use `multiline: true`

#### 占位符典型值

| 占位符 | 值 |
|---|---|
| `GREP_TOOL_NAME` | `Grep` |
| `BASH_TOOL_NAME` | `Bash` |
| `TASK_TOOL_NAME` | `Agent`（v2.1.x 之后；早期是 `Task`） |

> ⚠️ **`glob` 参数 vs `Glob` tool 同名**——Grep 自带 `glob` 字段做"文件名预筛"，跟独立的 Glob tool 无关，但名字重叠容易让 LLM 混淆。Forgify 实现时建议在描述里加 disambiguation。

### 2. JSON Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "additionalProperties": false,
  "required": ["pattern"],
  "properties": {
    "pattern": {
      "type": "string",
      "description": "The regex pattern to search for"
    },
    "path": {
      "type": "string",
      "description": "File or directory to search in (defaults to cwd). Must be absolute."
    },
    "glob": {
      "type": "string",
      "description": "Glob pattern to filter files (e.g. \"*.js\", \"**/*.tsx\")"
    },
    "type": {
      "type": "string",
      "description": "File type filter (e.g. \"js\", \"py\", \"rust\"); ripgrep --type"
    },
    "output_mode": {
      "type": "string",
      "enum": ["content", "files_with_matches", "count"],
      "default": "files_with_matches"
    },
    "-A": {
      "type": "number",
      "description": "Lines after match (content mode only)"
    },
    "-B": {
      "type": "number",
      "description": "Lines before match (content mode only)"
    },
    "-C": {
      "type": "number",
      "description": "Lines around match (content mode only)"
    },
    "-n": {
      "type": "boolean",
      "description": "Show line numbers (content mode only)"
    },
    "-i": {
      "type": "boolean",
      "description": "Case-insensitive"
    },
    "multiline": {
      "type": "boolean",
      "default": false,
      "description": "Allow patterns to match across line boundaries"
    },
    "head_limit": {
      "type": "number",
      "description": "Cap result to first N matches/files"
    }
  }
}
```

✅ 字段名 (含 `-A`/`-B`/`-C`/`-n`/`-i`) 来自 wong2 gist + 官方 docs 交叉确认。注意字段名带短横线是 unusual JSON Schema 风格——CC 直接采用 ripgrep CLI flag 命名。

### 3. 算法行为

**底层引擎** ✅
- `@vscode/ripgrep` 捆绑的 binary（Node.js wrapper 开销）
- 设 `USE_BUILTIN_RIPGREP=0` 切到系统 PATH 的 `rg`，**5-10× 加速**（捆绑版慢）

**Pattern 语法** ✅
- ripgrep 风格（不是 POSIX grep）
- 字面量 `{}` `()` `[]` 需转义：`interface\{\}` 才能匹配 Go 代码的 `interface{}`
- 默认 single-line；跨行匹配必须显式 `multiline: true`

**Output mode 行为差异**

| mode | 输出形态 |
|---|---|
| `files_with_matches`（默认） | 一行一个绝对路径，命中文件去重 |
| `content` | `path:lineno:content` 形式（`-n` 控制是否带行号），`-A`/`-B`/`-C` 决定上下文行 |
| `count` | `path:count` 形式，每文件一行 |

`-A` / `-B` / `-C` 与上下文行**仅在 `content` 模式生效**；其他模式忽略。

**File filter**
- `glob`: 直接传给 ripgrep `--glob`
- `type`: 传给 ripgrep `--type`（ripgrep 内置常见语言别名）
- 二者可以叠加（求交集）

**并发**
- ripgrep 自身已并行扫文件
- `IsReadOnly() = true`
- 并发安全：**是**——LLM 同 turn 多个 Grep 应放同 `execution_group` 加速

### 4. 已知 bugs / edge cases

- **bundled ripgrep 慢**——v2.1.126 仍未默认指向 system `rg`；用户得自己设环境变量
- **pattern 转义错**：LLM 经常忘记 `{}` 转义（写 `interface{}` 而不是 `interface\{\}`）→ 0 匹配。description 里专门提醒
- **multiline 默认关闭**：跨行 pattern 不报错只是 0 匹配，新手容易踩坑
- **大型 monorepo 性能**：`**/*.ts` 在大 repo 里会扫几万文件——靠 `head_limit` 限流

### 5. 输出格式给 LLM

`files_with_matches`（默认）：
```
/abs/path/to/foo.go
/abs/path/to/bar.go
... [N more files; use head_limit to refine]
```

`content` + `-n`：
```
/abs/path/foo.go:42:func ParseConfig(path string) (*Config, error) {
/abs/path/foo.go:58:func ParseConfigFromBytes(b []byte) (*Config, error) {
/abs/path/bar.go:13:func parseInternal(...)
```

`count`：
```
/abs/path/foo.go:5
/abs/path/bar.go:2
```

### 6. Forgify Go 实现要点

#### 6.1 Tool 接口

```go
type Grep struct {
    workspace PathGuard
    rgPath    string  // 系统 rg 路径（exec.LookPath("rg")），空则 fallback 到 stdlib
}

func (t *Grep) Name() string                   { return "Grep" }
func (t *Grep) IsReadOnly() bool               { return true }
func (t *Grep) NeedsReadFirst() bool           { return false }
func (t *Grep) RequiresWorkspace() bool        { return true }
```

#### 6.2 依赖

- **首选**：用户系统装的 `rg`（`exec.LookPath("rg")`）。装了就 shell out
- **fallback**：`bufio.Scanner` + `regexp.Regexp`，遍历目录手写
- 不要捆绑 ripgrep binary——用户体验差（CC 自己也吃这个亏）

#### 6.3 关键代码片段（rg backend）

```go
func (t *Grep) Execute(ctx context.Context, argsJSON string) (string, error) {
    var args struct {
        Pattern    string `json:"pattern"`
        Path       string `json:"path"`
        Glob       string `json:"glob"`
        Type       string `json:"type"`
        OutputMode string `json:"output_mode"`
        After      int    `json:"-A"`
        Before     int    `json:"-B"`
        Around     int    `json:"-C"`
        ShowLines  bool   `json:"-n"`
        IgnoreCase bool   `json:"-i"`
        Multiline  bool   `json:"multiline"`
        HeadLimit  int    `json:"head_limit"`
    }
    if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
        return "", fmt.Errorf("Grep.Execute: %w", err)
    }
    if args.OutputMode == "" { args.OutputMode = "files_with_matches" }
    if args.Path == "" { args.Path = "." } // workspace cwd

    // workspace 守卫
    if !t.workspace.Allowed(args.Path) {
        return fmt.Sprintf("Path outside allowed workspace: %s", args.Path), nil
    }

    if t.rgPath != "" {
        return t.execRg(ctx, args)
    }
    return t.execStdlib(ctx, args)  // fallback
}

func (t *Grep) execRg(ctx context.Context, args /*…*/) (string, error) {
    rgArgs := []string{}
    switch args.OutputMode {
    case "content":
        rgArgs = append(rgArgs, "--no-heading", "--with-filename")
        if args.ShowLines { rgArgs = append(rgArgs, "-n") }
        if args.After > 0  { rgArgs = append(rgArgs, "-A", fmt.Sprint(args.After)) }
        if args.Before > 0 { rgArgs = append(rgArgs, "-B", fmt.Sprint(args.Before)) }
        if args.Around > 0 { rgArgs = append(rgArgs, "-C", fmt.Sprint(args.Around)) }
    case "count":
        rgArgs = append(rgArgs, "-c")
    case "files_with_matches":
        rgArgs = append(rgArgs, "-l")
    }
    if args.Glob != ""    { rgArgs = append(rgArgs, "--glob", args.Glob) }
    if args.Type != ""    { rgArgs = append(rgArgs, "--type", args.Type) }
    if args.IgnoreCase    { rgArgs = append(rgArgs, "-i") }
    if args.Multiline     { rgArgs = append(rgArgs, "--multiline") }
    if args.HeadLimit > 0 { rgArgs = append(rgArgs, "-m", fmt.Sprint(args.HeadLimit)) }
    rgArgs = append(rgArgs, "--", args.Pattern, args.Path)

    cmd := exec.CommandContext(ctx, t.rgPath, rgArgs...)
    out, err := cmd.Output()
    if err != nil {
        var exitErr *exec.ExitError
        if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
            return "No matches found.", nil
        }
        return "", fmt.Errorf("Grep.execRg: %w", err)
    }
    if len(out) == 0 {
        return "No matches found.", nil
    }
    return string(out), nil
}
```

#### 6.4 Validate 错误码

| Sentinel | 触发 |
|---|---|
| `ErrEmptyPattern` | pattern == "" |
| `ErrInvalidOutputMode` | output_mode 非三选一 |
| `ErrPathOutsideWorkspace` | Phase 5 |
| `ErrInvalidRegex` | 编译 regex 失败（仅 stdlib fallback 时检测） |

#### 6.5 测试要点

- 基本字面量匹配（`foo`）
- regex 匹配（`func\s+\w+`）
- glob filter（`*.go`）→ 缩限范围
- type filter（`go`）→ 缩限范围  
- output_mode 三选一各跑一遍
- 0 匹配 → "No matches found."
- 大量匹配 + head_limit → 截断
- multiline pattern + multiline=false → 0 匹配（不报错）
- multiline pattern + multiline=true → 命中
- pattern 含 `{}` 不转义 → 0 匹配（验证错误传播）
- pattern 编译失败（fallback 模式）→ ErrInvalidRegex
- workspace 外路径 → 拒绝

---

## Glob（统一的"找 + 列"工具）

### 1. Description（自补，参考官方 docs + Forgify 升级）

⚠️ **Piebald-AI 没有独立描述文件**。官方 docs 仅一句 "Finds files based on pattern matching"。Forgify 自补版本（mimick CC 风格 + 加上**统一"列目录"语义** + **结构化输出**说明）：

> Finds files and directories matching a glob pattern. Also covers "list this directory" usage (use pattern `*` for one level, `**/*` for recursive).
>
> Usage:
> - ALWAYS use **${GLOB_TOOL_NAME}** for filename-based file search and directory listing. NEVER invoke `find` or `ls` as a **${BASH_TOOL_NAME}** command.
> - Supports standard glob syntax: `*` (any chars except `/`), `**` (any path), `?` (single char), `[abc]` (char class), `{a,b}` (alternation)
> - Returns a JSON array of entries, each with: `path`, `type` (`file`/`dir`/`symlink`), `size` (bytes; null for directories), `modified_at` (ISO 8601)
> - Sorted by **modification time, descending** (most recently modified first)
> - For content search inside files, use **${GREP_TOOL_NAME}** instead

**Forgify 与 CC 的关键区别**：CC Glob 只返回路径列表（plain text），仅覆盖"找文件"；Forgify Glob 返回结构化 JSON（含 type/size/mtime），既覆盖"找文件"也覆盖"列目录"，让 LS tool 的需求被吃掉。

### 2. JSON Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "additionalProperties": false,
  "required": ["pattern"],
  "properties": {
    "pattern": {
      "type": "string",
      "description": "The glob pattern to match files against (e.g. \"**/*.ts\", \"src/*.go\")"
    },
    "path": {
      "type": "string",
      "description": "The directory to search in (absolute path). If not specified, the current working directory will be used."
    }
  }
}
```

✅ 字段名来自 wong2 gist。

### 3. 算法行为

**Pattern 语法** ✅
- 标准 glob：`*`/`?`/`[abc]`/`{a,b}`/`**`（递归）
- 不是 regex
- 区分大小写（OS 默认）

**排序** ✅
- 按 **mtime 倒序**（最近修改的在前）
- 实现要点：每个 match 都要 `os.Stat` 拿 mtime——大 repo 上是主要开销

**返回数量上限** ⚠️
- CC 没明确公开；社区观察约 **1000 个**
- Forgify 取 1000 同等实现

**性能特性** ✅
- 大 monorepo 跑 `**/*.ts`：扫文件本身快，但 stat 每个 match 拿 mtime 慢——是真实瓶颈
- 用户体感"Glob 慢"基本都是 mtime 排序锅

**Edge cases**
- 不存在的 path → 错误
- 权限不足的子目录 → skip + 不报错
- Symlink → 跟随（fast-glob 默认）；返回结构里 `type: "symlink"`
- 隐藏文件（`.git/`）→ 默认跳（gitignore 风格）；除非 pattern 显式 `.*`
- pattern `*` + path → 列该目录一层（覆盖原 LS 用例，含 dir 和 file）
- pattern `**/*` + path → 递归列所有

**并发**
- `IsReadOnly() = true`
- 并发安全：**是**——LLM 同 turn 多个 Glob 应放同 `execution_group` 加速

**Forgify 升级（vs CC）**
- 输出形态：CC 是 plain text path list；Forgify 是 JSON array 每条带 `path` / `type` / `size` / `modified_at`
- 包含 dir：CC 默认 `WithFilesOnly()` 只返文件；Forgify 不加这个 filter，dir 也返（让 pattern 自然决定，`*.go` 自然只匹文件，`*` 同时匹两者）

### 4. 已知 bugs / edge cases

- 大 repo `**/*` 性能（stat 调用密集）
- `**/.*`（隐藏文件 explicit）行为依实现库不一致
- Symlink 循环——doublestar 应该有保护，但要测

### 5. 输出格式给 LLM

JSON 数组，按 mtime 倒序：

```json
[
  {"path": "/abs/path/internal", "type": "dir", "size": null, "modified_at": "2026-05-03T10:23:45Z"},
  {"path": "/abs/path/main.go", "type": "file", "size": 1234, "modified_at": "2026-05-02T14:11:08Z"},
  {"path": "/abs/path/README.md", "type": "file", "size": 540, "modified_at": "2026-04-22T09:00:00Z"}
]
```

超过 1000 条时截断 + 加追加提示行（不破坏 JSON）：
```json
{"results": [...], "truncated": true, "total_matched": 4321, "limit": 1000}
```

0 匹配：
```
No entries matching pattern "<pattern>" in <path>.
```

### 6. Forgify Go 实现要点

#### 6.1 Tool 接口

```go
type Glob struct {
    workspace PathGuard
}

func (t *Glob) Name() string                   { return "Glob" }
func (t *Glob) IsReadOnly() bool               { return true }
func (t *Glob) NeedsReadFirst() bool           { return false }
func (t *Glob) RequiresWorkspace() bool        { return true }
```

#### 6.2 依赖

- **`github.com/bmatcuk/doublestar/v4`**——Go 圈最常用的 glob 库，支持 `**`
- 不用 stdlib `path/filepath.Glob`（不支持 `**`）

#### 6.3 关键代码片段

```go
import "github.com/bmatcuk/doublestar/v4"

type globEntry struct {
    Path       string `json:"path"`
    Type       string `json:"type"`        // file | dir | symlink
    Size       *int64 `json:"size"`        // null for dir
    ModifiedAt string `json:"modified_at"` // RFC3339
}

func (t *Glob) Execute(ctx context.Context, argsJSON string) (string, error) {
    var args struct {
        Pattern string `json:"pattern"`
        Path    string `json:"path"`
    }
    if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
        return "", fmt.Errorf("Glob.Execute: %w", err)
    }
    if args.Path == "" {
        args.Path, _ = os.Getwd()
    }
    if !t.workspace.Allowed(args.Path) {
        return fmt.Sprintf("Path outside allowed workspace: %s", args.Path), nil
    }

    fsys := os.DirFS(args.Path)
    // 不加 WithFilesOnly()——让 dir 也返，覆盖 LS 用例
    matches, err := doublestar.Glob(fsys, args.Pattern)
    if err != nil {
        return fmt.Sprintf("Invalid glob pattern: %s (%v)", args.Pattern, err), nil
    }
    if len(matches) == 0 {
        return fmt.Sprintf("No entries matching pattern %q in %s.", args.Pattern, args.Path), nil
    }

    // 拉 stat + mtime 倒序排
    type withTime struct{ entry globEntry; mtime time.Time }
    items := make([]withTime, 0, len(matches))
    for _, m := range matches {
        full := filepath.Join(args.Path, m)
        info, err := os.Stat(full)
        if err != nil { continue } // permission / race，跳过
        e := globEntry{
            Path:       full,
            ModifiedAt: info.ModTime().UTC().Format(time.RFC3339),
        }
        switch {
        case info.IsDir():
            e.Type = "dir"
            // size 留 nil
        case info.Mode()&os.ModeSymlink != 0:
            e.Type = "symlink"
            sz := info.Size(); e.Size = &sz
        default:
            e.Type = "file"
            sz := info.Size(); e.Size = &sz
        }
        items = append(items, withTime{entry: e, mtime: info.ModTime()})
    }
    sort.Slice(items, func(i, j int) bool {
        return items[i].mtime.After(items[j].mtime)
    })

    const limit = 1000
    if len(items) > limit {
        results := make([]globEntry, limit)
        for i := range items[:limit] { results[i] = items[i].entry }
        out, _ := json.Marshal(map[string]any{
            "results":       results,
            "truncated":     true,
            "total_matched": len(items),
            "limit":         limit,
        })
        return string(out), nil
    }

    results := make([]globEntry, len(items))
    for i := range items { results[i] = items[i].entry }
    out, _ := json.Marshal(results)
    return string(out), nil
}
```

#### 6.4 Validate 错误码

| Sentinel | 触发 |
|---|---|
| `ErrEmptyPattern` | pattern == "" |
| `ErrPathOutsideWorkspace` | Phase 5 |

#### 6.5 测试要点

- 基本 `*.go` → 仅文件
- 递归 `**/*.go`
- 多扩展 `**/*.{go,md}`
- 字符类 `[abc]*`
- **`*` + path（"列目录"用例）**→ 同时返 dir + file，dir size=null
- **`**/*` + path（"递归列出"用例）**→ 全部 entry
- 0 匹配 → 友好消息
- mtime 倒序正确（造 3 个时间不同的文件验证）
- 1001 个匹配 → 截断 + JSON 含 `truncated: true` + `total_matched`
- Symlink → 返 `type: "symlink"` + size
- 路径不存在 → 错误
- 权限不足子目录 → skip 不崩
- Symlink 循环 → 不死循环
- workspace 外 → 拒绝
- JSON 输出可被标准 `json.Unmarshal` 解析（结构稳定）

---

## LS 决策记录

**已定：跟 CC 一起砍 LS**。理由：CC v2.1.126 的 Glob 在能力上严格 ⊃ LS（pattern `*` 等价 `ls`，pattern `**/*` 是递归列出）。Forgify 进一步把 Glob 输出从纯路径升级为 JSON（每条带 type/size/mtime），结构化得比 CC 还彻底——既覆盖"找文件"也覆盖"列目录"，LS tool 自然多余。

inventory 同步：LS 从 P0 移到 Skip。

---

## 跨 tool 共享

- 二者都 `IsReadOnly() = true`——LLM 把同 turn 的 Grep + Glob 都放同 `execution_group` 即并行
- 都 Phase 5 `RequiresWorkspace = true`
- 都不需要 `NeedsReadFirst`（不依赖 SeenFiles 状态）
- workspace 守卫共用 `PathGuard` 接口（Phase 5 设计）

---

## 总结：本批实施估时

| 工具 | 估时 | 难点 |
|---|---|---|
| Grep | 0.5 天 | rg 是否捆绑决策 + stdlib fallback 完整度 |
| Glob | 0.3 天 | doublestar 库 + mtime 排序性能 + JSON 输出（结构化升级） |

**合计** ~0.8 天（含 Glob 的"统一找+列"职责）。

---

## 信任度总结

- ✅ **多源确认**：Grep description 原文 / schema / 6 字段名 / output_mode 行为；Glob schema {pattern, path}；LS 已从 v2.1.126 移除；mtime 倒序排序约定
- ⚠️ **单源 / 推测**：Glob 实际描述文本（CC 没公开 verbatim）/ Glob 1000 cap 数值 / Grep `head_limit` 字段是否真存在
- ❌ **无法验证**：Glob 隐藏文件默认行为（依实现库）/ LS 在 v2.1.126 之前的精确移除版本号

deep-dive 期间补强 ⚠️ 项；实施时一律按 ✅ 项落地，⚠️ 项设计上保留扩展位。
