# Search Tools — V1.2 详设计

**Phase**：5（System Tool 第二代 search 批次）
**状态**：✅ 实现完成（2026-05-04，S1-S3）
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../../../CLAUDE.md`](../../../CLAUDE.md) §S18 — Tool 接口规约
- [`./chat.md`](./chat.md) §4.4 — 系统工具完整目录
- [`./filesystem.md`](./filesystem.md) — 共享 PathGuard 守卫
- 实现包：`backend/internal/app/tool/search/`

---

## 1. 一句话

LLM 的代码搜索两件套：**Grep**（regex 内容搜索，rg + stdlib 双后端）/ **Glob**（文件查找，doublestar + 按 mtime 降序 + JSON 富信息）。设计决策 D3：Glob 用 `pattern: "*"` **替代独立 LS 工具**——单工具覆盖 list dir + glob match。

---

## 2. 端到端推演（设计原则 #5）

### Grep 路径

```
触发源：LLM 调 Grep
  → app 层：app/tool/search.Grep.Execute
      → ValidateInput: pattern 非空 / output_mode 合法 / -A/-B/-C/head_limit ≥0 / path 绝对
      → pathguard.Allow(path)
      → os.Stat(path)                 // 决定单文件 vs 目录走法
      → 后端分派：
          rgPath != "" → execRg(ctx, args)         // shell out 到 ripgrep
          rgPath == "" → execStdlib(ctx, args)     // bufio.Scanner + regexp.Regexp
      → rg 异常失败时再 fallback execStdlib（surface 同）
  → 响应：
    内容 / files-with-matches / count 三种格式 + head_limit 截断
    无匹配 → "No matches for X in Y."
    PathGuard 拒 → 友好字符串
    regex 编译错（仅 stdlib 可见）→ "Invalid regex pattern: <err>"
```

### Glob 路径

```
触发源：LLM 调 Glob
  → ValidateInput: pattern 非空 / path 绝对（如有）/ limit ≥0
  → pathguard.Allow(path)
  → os.Stat(root)                                 // 必须是目录
  → doublestar.Glob(os.DirFS(root), pattern)      // ** 递归支持
  → 对每个 match 调 os.Lstat 取 type/size/mtime    // Lstat：symlink 报 symlink
  → 按 mtime 降序排（同时间用 path 字典序兜底）
  → 截到 limit（默认 100，硬上限 1000）
  → JSON: {root, matches[{path,type,size,mtime}], total, truncated}
```

**端到端跨 domain 依赖**：
- `pkg/pathguard`：路径黑名单（共享 `fstool` 那份 PathGuard 指针，main.go 装一次）
- `os/exec.LookPath("rg")`：构造时一次检测 rg 是否在 PATH，缓存到 `Grep.rgPath`
- 第三方：`github.com/bmatcuk/doublestar/v4`（**唯一**第三方依赖）
- 无 DB / Service / SSE 事件 / HTTP API

---

## 3. 关键决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 后端策略 | **rg 优先 + stdlib 兜底**，构造时锁定（不每次重查 PATH）| rg 大树 10×–100× 加速 + .gitignore-aware；缺 rg 时 stdlib 也能跑（surface 一致），不强制用户装 rg |
| LS 工具 | **不实现** — Glob `pattern: "*"` 替代 | 详 02-tools-deep/02-search.md 决策 D3；少一个工具表面，LLM 也少一种选择疲劳 |
| Glob 排序 | **mtime 降序**（最新在前）| 大仓库里"我刚改的"通常是 LLM 想要的；同时间用 path 字典序兜底保确定性 |
| Glob 输出 | **JSON 含 type/size/mtime** | 替代 LS 时仍能给 LLM 文件元信息；mtime 走 RFC 3339 让 LLM 不需自定义解析 |
| Symlink 处理 | **Lstat**（报 symlink 类型）| Stat 会跟随 symlink 报目标类型，掩盖事实；Lstat 让 LLM 看到真实结构 |
| Grep schema 字段名 | **镜像 rg CLI flag**（`-A`/`-B`/`-C`/`-n`/`-i`）| 跟 CC 对齐，让 LLM 直觉迁移；不常规但实用 |
| Multiline 模式 | **可选 flag** + Go regex `(?s)` 内联 | 默认按行扫（典型场景）；跨行 pattern 用户显式开启 |
| 噪声目录（stdlib） | 跳过 `.git` / `node_modules` / `.venv` / `venv` / `__pycache__` / `.forgify` | 避免在 stdlib 后端浪费时间；rg 走 .gitignore 自然跳 |
| 单文件读上限（multiline）| 32 MiB | 防意外扫到大二进制 OOM |
| 单行长度上限 | 8 MiB | 与 Read 一致 |
| Type 过滤词汇 | 内置 25+ 常见语言扩展名 map（`go`/`py`/`ts`/...）| 粗略对齐 rg `--type` 词汇；未知 type 静默零结果（不报错）|
| head_limit 应用 | rg 后端：post-hoc 截 N 行；stdlib：循环里早 break | rg 的 `--max-count` 是 per-file 不是全局；都按"全局首 N"语义 |
| 错误返回模式 | 文件系统 / regex 错 → 友好字符串；ValidateInput 错 → Go err | §S18 规约 |

---

## 4. 工具规约

### 4.1 Grep（`backend/internal/app/tool/search/grep.go`）

**Args**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `pattern` | string | ✅ | 正则；字面 `{}` 等需转义 |
| `path` | string | | 文件或目录绝对路径；缺省走 cwd |
| `glob` | string | | 文件名过滤（`"*.go"` / `"**/*.tsx"`）|
| `type` | string | | 语言过滤（`"go"`/`"py"`/`"ts"`/`"rust"` 等 25+）|
| `output_mode` | enum | | `"content"` / `"files_with_matches"`(默认) / `"count"` |
| `-A` | number | | content 模式 trailing 上下文行数 |
| `-B` | number | | content 模式 leading 上下文行数 |
| `-C` | number | | content 模式同时设 -A 和 -B |
| `-n` | bool | | content 模式显示行号 |
| `-i` | bool | | 大小写不敏感 |
| `multiline` | bool | | 允许跨行匹配（`.` 跨 `\n`）|
| `head_limit` | number | | 截前 N 项 |

**返回**：
- **content** 模式：`<path>:<lineno>:<text>`（匹配行）/ `<path>-<lineno>-<text>`（上下文行）；单文件 root 省 path 前缀
- **files_with_matches** 模式：每行一个 path
- **count** 模式：每行 `<path>:<count>`
- 无匹配 → `No matches for "<pattern>" in <path>.`
- 截断 → 末尾追加 `... [truncated at N matches/files; raise head_limit to see more]`

**静态元数据**：`IsReadOnly=true` / `NeedsReadFirst=false` / `RequiresWorkspace=true`

**ValidateInput** sentinels：
- `ErrEmptyPattern` — pattern 缺 / 空 / 仅空白
- `ErrInvalidOutputMode` — 不在 enum
- 数值 < 0 / path 非绝对 → `errors.New(...)` 描述

### 4.2 Glob（`glob.go`）

**Args**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `pattern` | string | ✅ | glob，如 `**/*.go` / `*.md` / `*`（LS 模式）|
| `path` | string | | 搜索 root 绝对路径；缺省 cwd |
| `limit` | number | | 默认 100；硬上限 1000 |

**返回**（JSON）：
```json
{
  "root": "/Users/x/proj",
  "matches": [
    {"path": "/Users/x/proj/main.go", "type": "file", "size": 1234, "mtime": "2026-05-04T10:00:00Z"}
  ],
  "total": 12,
  "truncated": true
}
```

- `type`：`"file"` / `"dir"` / `"symlink"`
- `mtime`：RFC 3339
- 排序：mtime 降序，同时间 path 字典序
- 单文件 root：返友好错（`Search root must be a directory: <path>`）
- 不存在：`Search root not found: <path>`
- pattern 错：`Invalid glob pattern "<pat>": <doublestar err>`

**静态元数据**：`IsReadOnly=true` / `NeedsReadFirst=false` / `RequiresWorkspace=true`

**ValidateInput** sentinels：
- 共享 grep.go 的 `ErrEmptyPattern`
- path 非绝对 / limit < 0 → `errors.New(...)`

### 4.3 SearchTools 工厂

```go
// app/tool/search/search.go
func SearchTools(pathGuard pathguardpkg.PathGuard) []toolapp.Tool {
    return []toolapp.Tool{
        newGrep(pathGuard),  // 构造时锁定 rgPath（PATH 上 LookPath("rg")）
        newGlob(pathGuard),
    }
}
```

调用方按 §S13 嵌套子包别名规则导入为 `searchtool`。

---

## 5. 实现要点

### 5.1 双后端设计

```
Grep.Execute(ctx, args)
  ├─ ValidateInput / PathGuard / Stat
  ├─ rgPath != "" → execRg
  │   └─ 失败（rg exit ≠ 0,1）→ fallback execStdlib（防止 rg 偶发故障让搜索全跪）
  └─ rgPath == "" → execStdlib 直接走
```

**rg 优先**：构造时 `exec.LookPath("rg")` 一次性检测，缓存到 `Grep.rgPath` struct 字段。运行时**不**再重查（PATH 改动不会被识别——重启即可）。

**Surface 等价性**：两后端共享 `grepArgs` struct + 输出格式约定 + head_limit 语义 + `noMatchesMessage()` helper。Pipeline 测试在不区分后端的前提下验证行为。

### 5.2 rg 后端要点（`grep_rg.go`）

- `--color=never` + `--no-heading`：让输出确定可解析（无 ANSI 干扰）
- `--multiline --multiline-dotall`：仅当 LLM 显式 `multiline:true` 时附加
- `-e <pattern>`：用 `-e` 显式指定 pattern 避免歧义（pattern 以 `-` 开头时不被误解析为 flag）
- `head_limit` 走 post-hoc `capLines` 切（rg 的 `--max-count` 是 per-file，不符合"全局首 N"语义）
- exit code 1 = 无匹配（**非错误**）→ 返 `noMatchesMessage(args)` + nil err
- exit code 2+ → 真错误 → 返 Go err（让 Execute fallback 到 stdlib 救场）
- stderr 截 512 字节防 rg 异常时灌爆日志

### 5.3 stdlib 后端要点（`grep_stdlib.go`）

- `compileGrepRegex`：内联 flag `(?i)` / `(?s)` 拼到 pattern 前；**不**开 `(?m)`（按行扫已有 ^/$ 语义）
- `collectCandidates`：filepath.WalkDir + noiseDirs 跳过 + glob/type 过滤
- 单文件 vs 目录两条路径
- **multiline 模式整文件读**（受 32 MiB 限）；line 模式按 bufio.Scanner 流（受 8 MiB 单行限）
- `scanFileContentLineMode` 跟 multiline 版**对称**预算 `matchLines` map，让多匹配重叠时 match 行始终用 `:` 分隔符（**Tool 自检 batch 1 修的真 bug** — 详 progress-record 2026-05-05）

### 5.4 Glob 实现要点

- `os.DirFS(root)` 让 doublestar 在 fs.FS 抽象上 Glob，pattern 用正斜杠（doublestar 要求）
- `Lstat` 而非 Stat：symlink 报真类型
- 单遍 Walk + 单遍 Sort + 单遍截 limit；`Truncated` flag 反映是否被切
- ctx.Err() 在每文件循环开头查 → 长 walk 可被取消

---

## 6. 安全边界

| 防线 | 覆盖 | 局限 |
|---|---|---|
| **PathGuard 黑名单** | 共享 fstool 的 `pathguard.NewDefault()` 实例（cross-platform）| Bash 不走（一致 trade-off）|
| **必须绝对路径** | 防 cwd 误解析 + 跨目录越权 | 用户希望相对路径需先 `Bash cd`，但 search tool 不读 AgentState.Cwd |
| **rg 走用户 PATH 上的 rg**（cmd.Output）| 不取决用户系统 rg 版本 | 若用户 PATH 被改成恶意 rg：本地单用户场景不在威胁模型；详 02-tools-deep/02-search.md |
| **stdlib 跳 noise dirs** | 防扫 node_modules 等浪费时间 + 误曝 secret | 不读 .gitignore（要 .gitignore 装 rg）|
| **单行 8 MiB / 单文件 32 MiB（multiline）** | 防 OOM | 超大 minified 文件会被 Scanner 报错（友好返）|
| **head_limit 默认与硬上限** | 默认 100、硬上限 1000，防 LLM 索取百万项撑爆响应 | 用户真需要更多需要分批查 |

---

## 7. 测试覆盖

| 层 | 文件 | 测试数 | 覆盖 |
|---|---|---|---|
| Grep | `backend/internal/app/tool/search/grep_test.go` | 28 | identity / 静态 metadata / schema / Validate × 5 / normalize / 3 输出模式 / type+glob 过滤 × 4 / -i / -A / -B / 上下文分隔符 / multiline / head_limit content + files / PathGuard / 不存在 / regex 错 / 单文件 root 省 path / **多行连续匹配 match-not-context 回归**（batch 1）/ 混合匹配上下文重叠（batch 1） |
| Glob | `glob_test.go` | 19 | identity / 静态 metadata / schema / Validate × 4 / normalize × 2 / `*` LS 替代 / `**/*.go` 递归 / type 字段 file/dir/symlink / size+mtime / mtime 降序排 / limit truncated / PathGuard / 不存在 / 是文件 / 零结果 |
| Pipeline | `backend/test/search/` | 3 场景 | LLM ↔ tool 端到端：GrepFindsMatches / GlobListsDirectoryWithMetadata / GrepPathGuardDeniesSensitivePath（40s） |

合计 **47 单测 + 3 pipeline 场景**。

---

## 8. 与其他 domain 的关系

| 关系 | 说明 |
|---|---|
| **filesystem** | **共享 PathGuard 实例**（main.go 装一次 `pathguardpkg.NewDefault()` 传给 fstool 和 searchtool）|
| **shell (Bash)** | Bash 不共享 PathGuard（设计 trade-off）；但 LLM 通过 Grep 找代码 + Bash 跑命令是常见组合 |
| **chat** | search tool 通过 chat ReAct loop 调度；与其他 read-only tool（如 forge.search）可同 execution_group 并行 |
| **agentstate** | 不依赖 — search 是 stateless（不写 SeenFiles，也不读 Cwd）|
| **events / SSE** | 无 — 结果通过 chat.message 的 tool_result block 推流 |
| **errmap** | 无登记 — 错误以友好字符串返 LLM |

---

## 9. 演化方向

- **rg 版本探测**：构造时 `rg --version` 输出记到日志，让用户知道用的是哪个 rg
- **--gitignore 显式开关**：当前 stdlib 不读 .gitignore，未来加可选支持（但实现复杂度抵消"轻量 fallback"价值，谨慎）
- **Type 词汇扩展**：当前 25+ 语言；新语言按需加（修 `typeExtensions` map）
- **Multiline 大文件流式**：当前 multiline 整文件读（32 MiB 上限）；未来若需扫超大文件可改流式（带前后向 buffer 的窗口算法）
- **结果 caching**：完全不做 — 文件可能改，缓存是 footgun
- **Fuzzy file finder（fzf 集成）**：可作为 Glob 的补充 tool，但本地单用户先用 Glob mtime 降序就够了
