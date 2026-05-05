# Filesystem Tools — V1.2 详设计

**Phase**：5（System Tool 第二代 file-ops 批次）
**状态**：✅ 实现完成（2026-05-04，O1-O4）
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../../../CLAUDE.md`](../../../CLAUDE.md) §S18 — Tool 接口规约 + 静态元数据对照表
- [`./chat.md`](./chat.md) §4.4 — 系统工具完整目录
- [`./task.md`](./task.md) §10 — AgentState 跨 tool 共享状态生命周期
- 实现包：`backend/internal/app/tool/filesystem/`
- 共享守卫：`pkg/pathguard`（路径黑名单）/ `pkg/agentstate`（must-Read-first 状态）

---

## 1. 一句话

LLM 操作用户本机文件系统的三件套：**Read**（cat -n 行号格式读）/ **Write**（原子覆写）/ **Edit**（字面量字符串替换）。三者**故意**不依赖任何 service / repository——纯粹 stdlib + 两个 cross-tool 共享原语：`pkg/pathguard`（敏感路径黑名单）+ `pkg/agentstate.SeenFiles`（must-Read-first 守卫）。

---

## 2. 端到端推演（设计原则 #5）

### Read 路径

```
触发源：LLM 在 chat agent 循环里调 Read 工具
  → transport 层：无（system tool 不走 HTTP）
    → app 层：app/tool/filesystem.Read.Execute
        → pathguard.Allow(file_path)        // 黑名单守卫
        → os.Stat / os.Open + bufio.Scanner
        → reqctxpkg.GetAgentState(ctx).MarkRead(path, size)  // 标 SeenFiles
      → infra 层：直接 os 系统调用，无 sandbox
  → 响应路径：
    成功 → cat -n 格式字符串 → tool_result
    PathGuard 拒绝 / 文件不存在 / 权限不足 → 友好字符串 + nil err（LLM 可恢复）
    Validate 失败（路径非绝对 / offset 负） → Go err（chat.executeTool 转失败 tool_result）
```

### Edit 路径（must-Read-first + 外部修改检测）

```
LLM 调 Edit
  → ValidateInput：file_path 绝对 / old_string 非空 / new_string ≠ old_string
  → Execute:
      pathguard.Allow                          → 拦敏感路径
      os.Stat(file)                            → 必须存在（Edit 不创建）
      reqctxpkg.GetAgentState(ctx).WasRead(path) → 必须本对话 Read 过
      info.Size() == seenSize                  → 检测外部修改（best-effort）
      strings.Count + strings.ReplaceAll       → 字面量替换（信任 stdlib）
      atomic write: CreateTemp + Chmod + Rename
      state.MarkRead(path, len(new))           → 更新 SeenFiles 让链式 Edit 通过
  → 返 "Successfully replaced N occurrence(s) in <path>."
```

### Write 路径

类似 Edit，但：
- 文件不存在时**直接创建**，不需 must-Read-first（新建无可破坏目标）
- 文件存在时走 must-Read-first（覆写需先 Read）
- 父目录必须存在（**不**主动 mkdir，让 LLM 显式 `Bash mkdir -p`）

**端到端跨 domain 依赖**：
- `pkg/pathguard`：路径黑名单守卫（详 [`pathguard.go` 包注释](../../../backend/internal/pkg/pathguard/pathguard.go)）
- `pkg/agentstate`：`SeenFiles` map（must-Read-first）；通过 `pkg/reqctx.WithAgentState` ctx 注入
- `pkg/reqctx.AgentState`：chat/runner.go::processTask 注入到每个 task 的 ctx
- 无 DB / Service / Repository / SSE 事件——纯 stateless tool

---

## 3. 关键决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 路径要求 | **必须绝对路径** | LLM 容易混淆 cwd 概念，相对路径靠每次解析 cwd 是 footgun；强制绝对避免歧义。`Bash cd` 改 AgentState.Cwd，但 Read/Write/Edit 不读 Cwd |
| 防越权策略 | **PathGuard 黑名单** + **must-Read-first** 双层 | 黑名单挡敏感路径（~/.ssh 等）；Read-first 挡"LLM 没看代码就盲改"；详 02-tools-deep/03-shell.md 决策 D5 |
| 外部修改检测 | **size 失配检测**（best-effort） | hash 检测过重；size 对意外覆盖足够；用户改文件后 LLM 必须重 Read |
| Edit 替换语义 | **字面量** + 唯一性守卫 + replace_all | 不引 regex（避免 LLM 写错 regex 大范围误改）；信任 stdlib `strings.ReplaceAll`，比 CC `#51986` 防御性 count-after 更透明（显式报 N 次）|
| 原子写 | **CreateTemp + Rename**（同目录）| Rename 在同 fs 是原子 syscall；读者永远看不到半成品；中途 panic 也只留原文件 |
| 文件 mode 保留 | 覆写时保留原 mode；新建用 0o644 | CreateTemp 默认 0o600 会静默收紧权限——显式 chmod 防意外 |
| Read 默认 limit | 2000 行 | 跟 CC 公开值对齐 |
| Read 单行上限 | 8 MiB（`bufio.Scanner` 缓冲）| minified JS / 大 JSON dump 都覆盖；超出报友好错让 LLM 知情 |
| 错误返回模式 | 文件系统失败（not found / 权限 / 已存在目录）→ 友好字符串 + nil err；ValidateInput 失败 → Go err | 让 LLM 看到可恢复信号；§S18 规约 |
| Image / PDF / Notebook | **v1 不支持** | description 不写未实现内容（写了反而误导 LLM）；Phase 5+ 再加 |
| LS 单独工具 | **不做** | Glob 用 `pattern: "*"` 替代（决策 D3，详 02-tools-deep/02-search.md）|

---

## 4. 工具规约

### 4.1 Read（`backend/internal/app/tool/filesystem/read.go`）

**Args** (LLM-facing schema)：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `file_path` | string | ✅ | 必须绝对路径 |
| `offset` | number | | 1-based 起始行；默认 1 |
| `limit` | number | | 读多少行；默认 2000 |

**返回**（成功）：cat -n 格式字符串
```
    1	first line
    2	second line
... [truncated at line 2000; use offset+limit to read more]
```

**返回**（特殊情况）：
- 空文件 → `<system-reminder>File exists but has empty contents.</system-reminder>` + 仍 MarkRead（让 Edit/Write 能通过守卫）
- 路径是目录 → `Path is a directory, not a file: <path>. Use Glob with pattern "*" to list a directory.`
- 不存在 → `File not found: <path>`
- 权限不足 → `Permission denied: <path>`
- PathGuard 拒 → `path is denied by safety guard: <rule>`
- 单行超 8 MiB → `Failed to read <path>: bufio.Scanner: token too long`

**静态元数据**：`IsReadOnly=true` / `NeedsReadFirst=false` / `RequiresWorkspace=true`

**ValidateInput** sentinels（`backend/internal/app/tool/filesystem/read.go`）：
- `ErrEmptyFilePath` — 缺 / 空 / 仅空白
- `ErrPathNotAbsolute` — 不以 `/` 开头
- `ErrNegativeOffset` — offset < 0（0 = 用默认）
- `ErrNegativeLimit` — limit < 0（0 = 用默认）

### 4.2 Write

**Args**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `file_path` | string | ✅ | 必须绝对 |
| `content` | string | ✅ | 允许空串（创建空文件）|

**Schema 用 `*string`** 检测 content 字段缺失（区分"缺 key"与"空字符串"）——LLM 必须显式表达意图。

**返回**：
- 成功 → `File successfully written to <path>`
- PathGuard 拒 → 标准黑名单字符串
- 父目录不存在 → `Parent directory does not exist: <parent>. Use Bash 'mkdir -p' to create it first.`
- 父路径不是目录 → `Parent path exists but is not a directory: <parent>`
- 目标是目录 → `Path is a directory, not a file: <path>`
- 已存在 + 没 Read 过 → `File must be read first before overwriting: <path>. Use the Read tool first.`
- AgentState 缺失（接线 bug）+ 已存在 → `Cannot verify Read-first guard: agent state missing. Read the file first.` **拒绝覆写**（守卫不可绕过）
- AgentState 缺失 + 新文件 → 允许（无可保护目标）

**静态元数据**：`IsReadOnly=false` / `NeedsReadFirst=true` / `RequiresWorkspace=true`

**ValidateInput** sentinels：
- `ErrEmptyFilePath` / `ErrPathNotAbsolute`（共享 read.go）
- 缺 `content` key → `errors.New("content field is required (use empty string to create an empty file)")`

### 4.3 Edit

**Args**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `file_path` | string | ✅ | 必须绝对 |
| `old_string` | string | ✅ | 字面量（非 regex），非空 |
| `new_string` | string | ✅ | 允许空串（删除 old_string）|
| `replace_all` | bool | | 默认 false；多匹配时必须显式 true |

**返回**：
- `Successfully replaced 1 occurrence in <path>.`
- `Successfully replaced N occurrences in <path>.`（replace_all=true 且 N>1）
- 不存在 → `File not found: <path>. Edit can only modify existing files; use Write to create new ones.`
- 没 Read → `File must be read first before editing: <path>. Use the Read tool first.`
- 外部修改 → `File has been modified since last read (current size N, expected M): <path>. Read it again before editing.`
- 0 匹配 → `old_string not found in the file. Verify the exact text (whitespace and case matter).`
- N>1 + replace_all=false → `Found N matches of old_string in <path>, but replace_all is false. Either provide more surrounding context to make old_string unique, or set replace_all: true.`

**静态元数据**：`IsReadOnly=false` / `NeedsReadFirst=true` / `RequiresWorkspace=true`

**ValidateInput** sentinels：
- 共享 read.go 的路径校验
- `ErrEmptyOldString` — old_string 空 / 缺
- `ErrEditNoOp` — old_string == new_string（拒绝空操作浪费 tool 调用）
- 缺 `new_string` key → `errors.New(...)`

### 4.4 Filesystem Tools 工厂

```go
// app/tool/filesystem/filesystem.go
func FilesystemTools(pathGuard pathguardpkg.PathGuard) []toolapp.Tool {
    return []toolapp.Tool{
        &Read{pathGuard: pathGuard},
        &Write{pathGuard: pathGuard},
        &Edit{pathGuard: pathGuard},
    }
}
```

调用方按 §S13 嵌套子包别名规则导入为 `fstool`。

---

## 5. 实现要点

### 5.1 PathGuard 集成

每个 tool 在 Execute 第一步调 `t.pathGuard.Allow(args.FilePath)`，拒绝则返友好字符串 + nil err。**不**走 Validate——Validate 只管参数结构正确性，PathGuard 是运行时策略。

`DefaultDenyList` 跨平台覆盖见 [`pathguard.go`](../../../backend/internal/pkg/pathguard/pathguard.go) 包注释 + Tool 自检 batch 4 增强（Linux runtime / Windows credential store / 浏览器 logins / kube+docker config）。

### 5.2 must-Read-first 守卫（AgentState.SeenFiles）

`AgentState` 是 per-conversation 的 `*agentstatepkg.AgentState`，由 `chat/runner.go::processTask` 通过 `reqctxpkg.WithAgentState(ctx, state)` 注入到每个 task 的 ctx。

```go
// pkg/agentstate/agentstate.go
type AgentState struct {
    SeenFiles sync.Map // string (abs path) → int64 (size at Read time)
    ...
}
func (s *AgentState) MarkRead(path string, size int64)
func (s *AgentState) WasRead(path string) (int64, bool)
```

**调用规约**：
- **Read 成功** → `MarkRead(path, info.Size())`（空文件也标记，size=0）
- **Read 被 PathGuard 拒** → **不**标记（否则 Edit/Write 会通过守卫操作 Read 实际访问不到的 path）
- **Write 成功** → `MarkRead(path, len(content))` 让链式 Edit 通过
- **Edit 成功** → `MarkRead(path, len(newContent))` 让链式 Edit 通过

**生命周期**：跟 conversation 的 `convQueue` 同步——5 分钟空闲 GC（详 chat/runner.go::runQueue）。重启后清空——LLM 必须重 Read。这是有意行为（重启 = 全新 session）。

### 5.3 原子写

```go
tmpFile, _ := os.CreateTemp(parent, ".forgify-write-*")  // tmp 在同目录
tmpFile.WriteString(content)
tmpFile.Close()
os.Chmod(tmpPath, mode)  // 保留原 mode 或用 defaultFileMode
os.Rename(tmpPath, cleaned)  // 同 fs 内 rename 是原子 syscall
```

**为啥同目录 tmp**：`os.Rename` 跨 fs 退化为 copy + delete（不再原子）。同目录 tmp 保证 rename 是 syscall 级原子。

**cleanup**：任一步失败都调 `os.Remove(tmpPath)`，best-effort（rm 失败也只是 tmp 残留）。

### 5.4 错误返回模式（§S18 一致性）

所有 tool 严格遵守：
- **ValidateInput** 失败 → Go err（chat/tools.go::executeTool 转失败 tool_result + LLM 可重试）
- **Execute** 中文件系统 / OS 错误 → 友好字符串 + nil err（LLM 可恢复）
- **Execute** 中真正的内部 bug（Validate 通过后 args 又解析失败、scanner panic 等）→ Go err

**理由**：LLM 看到友好字符串能继续推理（"哦，文件没找到，我换个路径"），看到 Go err 框架包装的"input validation failed: <err>"知道是自己 args 写错了。两类信号 LLM 反应不同——分开传达比统一返 err 更有用。

---

## 6. 安全边界

| 防线 | 覆盖 | 局限 |
|---|---|---|
| **PathGuard 黑名单** | ~/.ssh / ~/.aws / Linux runtime / Windows credential store / 浏览器 logins / kube+docker config / Forgify 自家 dataDir | **不挡 Bash**（Bash 故意 RequiresWorkspace=false）；新攻击向量需手动加 entry |
| **必须绝对路径** | 防 cwd 误解析；防 `../../etc/passwd` 这种相对越权（filepath.Clean 标准化） | 用户希望相对路径需要先用 Bash 显式 `cd` 改 AgentState.Cwd 但 fs tool 不读 Cwd——**这是有意的**（fs tool 跟 Bash 解耦，Cwd 仅 Bash 用）|
| **must-Read-first** | 防 LLM 没看代码就盲改 / 盲覆写 | 仅 in-memory，重启清空（LLM 必须重 Read）|
| **size 失配检测** | 检测 Read 与 Edit 之间的外部修改 | 仅 size，hash 检测过重；同 size 内容互换检测不到（v1 trade-off） |
| **mode 保留** | 覆写不静默放宽权限 | chmod 失败时 cleanup tmp + 报错（不会损坏原文件）|
| **原子 rename** | 读者永远看不到半成品 | 跨 fs 的 tmp 会让 Rename 退化（CreateTemp 用同目录避免）|

---

## 7. 测试覆盖

| 层 | 文件 | 测试数 | 覆盖 |
|---|---|---|---|
| Read | `backend/internal/app/tool/filesystem/read_test.go` | 19 | identity / Validate × 4 / 基本读 / 空文件 / offset / limit truncation / 边界 limit / 无尾 \n / 不存在 / 是目录 / PathGuard 拒（不 MarkRead）/ AgentState 缺失（仍成功）/ 单行超 8 MiB / offset > EOF / 默认 limit |
| Write | `write_test.go` | 13 | identity / Validate × 4 / 新文件 / 空内容 / 覆写（先 Read）/ 覆写（没 Read）/ AgentState 缺失（拒覆写 / 允新建）/ 父目录缺 / 目标是目录 / PathGuard 拒 / mode 保留 / mode 默认 |
| Edit | `edit_test.go` | 19 | identity / Validate × 6 / 单替换 / replace_all 多替 / 跨行 old_string / regex 元字符字面量匹配 / 空 new_string 删除 / SeenFiles 链式更新 / 不存在 / 是目录 / PathGuard 拒 / AgentState 缺 / 没 Read / 外部修改检测 / 0 匹配 / N>1+replace_all=false / mode 保留 / **markdown bold #51986 回归**（5 处全替不吃 newline）/ batch 1 回归测试见 [`progress-record.md`](../progress-record.md) |
| Pipeline | `backend/test/filesystem/` | 3 场景 | LLM ↔ tool 端到端：ReadEditClosedLoop / WriteWithoutReadDenied / PathGuardDeniesSensitivePath（29.7s） |

合计 **51 单测 + 3 pipeline 场景**。

---

## 8. 与其他 domain 的关系

| 关系 | 说明 |
|---|---|
| **chat** | chat/runner.go::processTask 通过 `reqctxpkg.WithAgentState(ctx, q.agentState)` 注入；queue idle 时 GC |
| **agentstate** | 独立 pkg；SeenFiles map + Cwd cell（Cwd 是 Bash 用，fs tool 不读）|
| **pathguard** | 独立 pkg；构造时由 main.go 装 NewDefault() 一份；3 个 fs tool 共享同一 guard 指针 |
| **shell (Bash)** | **不共享 PathGuard** — Bash 故意 RequiresWorkspace=false，是设计 trade-off（详 pathguard.go 包注释）|
| **search (Grep/Glob)** | 共享 PathGuard（同样 RequiresWorkspace=true）|
| **forge** | 无直接耦合；但 forge.RunForge 内部跑 Python 沙箱时也走文件系统，那是 sandbox 自己的隔离，不走 PathGuard |
| **events / SSE** | 无——fs tool 不发 SSE；其结果通过 chat.message 的 tool_result block 推流 |
| **errmap** | **无登记**——所有错误以友好字符串返 LLM，不到 handler |

---

## 9. 演化方向

- **Image / PDF / Jupyter notebook 读取**：现 description 不提（避免误导）；Phase 5+ 加 `infra/chat/extractor.go` 集成 → Read 自动分派
- **Hash-based 外部修改检测**：当前 size-only（best-effort）；将来可换 SHA-256 但需评估 perf
- **Workspace allowlist**：当前 PathGuard 是黑名单；未来若用户希望严格沙箱（"只能在 ~/Projects/foo"），加 allowlist 模式
- **Diff 工具**：当前 Edit 信任 stdlib `strings.ReplaceAll` + 显式报次数；未来可加 `Diff` tool 让 LLM 用 unified diff 做更复杂的多块编辑
- **Move / Copy / Delete**：当前不实现，LLM 用 Bash 跑 `mv`/`cp`/`rm`；未来若 sandbox 强化，独立 tool 更可控
