---
id: DOC-108
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-06
review-due: 2026-09-01
audience: [human, ai]
---
# Filesystem Tools — 本机文件读写的三件叶子工具

> **核心地位**：`tool/filesystem` 是 LLM 操作宿主机文件的"读写手臂"——三件工具 `Read` / `Write` / `Edit`。本包是**叶子工具适配器**（无 domain / store / handler / DDL / HTTP 端点），只实现 `app/tool` 的 5 方法接口。
>
> **灵魂**：**写前必读铁律**——任何 `Write` 覆写 / `Edit` 修改的目标，本次运行内必须先被 `Read` 过；通过 `pkg/agentstate.SeenFiles` 跨工具协作实现，**fail-closed**（state 缺失时 `Write` / `Edit` 直接拒绝）。

---

## 1. 物理布局

```
backend/internal/app/tool/filesystem/
├── filesystem.go       # FilesystemTools(PathGuard) → []toolapp.Tool
├── read.go             # Read   — 只读，盖章 SeenFiles
├── write.go            # Write  — 创建/覆写，原子 tmp+rename
└── edit.go             # Edit   — 字面量替换，原子写

backend/internal/pkg/agentstate/agentstate.go
└── AgentState{SeenFiles}  — MarkRead / WasRead；本包为唯一消费者

backend/internal/pkg/reqctx/agentstate.go
└── WithAgentState / GetAgentState  — host 跑 loop 前 seed
```

无 domain / store / handler / DDL / HTTP 端点。装配器 `FilesystemTools(pathGuard)` 返三件套,由 host 装入 `Toolset.Resident`。

---

## 2. 三件工具的契约

每件工具按 `app/tool` 的 **5 方法接口**:`Name` / `Description` / `Parameters` / `ValidateInput` / `Execute`。三个标准字段(`summary` / `danger` / `execution_group`)由 framework 在 `ToLLMDef` 注入 schema、在 `StripStandardFields` 从 args 剥离——工具只声明、只收到自己的业务参数。

| 字段 | Read | Write | Edit |
|---|---|---|---|
| Name | `Read` | `Write` | `Edit` |
| 业务参数 | `file_path` · `offset?` · `limit?` | `file_path` · `content` | `file_path` · `old_string` · `new_string` · `replace_all?` |
| 守卫顺序 | ① `Allow` ② stat | ① `AllowWrite` ② 父目录是目录 ③ 写前必读(仅覆写) | ① `AllowWrite` ② 文件存在 ③ 写前必读 ④ size 漂移 |
| AgentState 副作用 | 成功后 `MarkRead(path, size)` | 成功后 `MarkRead(path, len)` | 成功后 `MarkRead(path, len)` |
| 失败语义 | 软失败返 tool-result 串(LLM 自纠) | 同左 | 同左 |
| `danger` 静态声明 | 不声明(M2.1 纯信任) | 不声明 | 不声明 |

> `danger` 三级(`safe` / `cautious` / `dangerous`)由 **LLM 逐次自报**,filesystem 工具不预设静态下限——M2.1 R0030 拍板。

---

## 3. 灵魂:写前必读铁律(write-before-read)

### 3.1 不变式

任何 `Write` 覆写已存在文件 / 任何 `Edit` 修改的目标,**本次运行内必须先被 `Read`**。否则:

- **Write 覆写**:返回 `File must be read first before overwriting: <path>`,文件不动
- **Edit 修改**:返回 `File must be read first before editing: <path>`,文件不动
- **AgentState 本身缺失**:返回 `Cannot verify Read-first guard: agent state missing`,**fail-closed**

> 静默放过没有读前守卫的 Write/Edit,会让不变式形同虚设——所以缺 state 时**显式拒绝**,要么 LLM 先 Read,要么 host 学会 seed state。
>
> Read **本身**只是只读 + 盖章,缺 state 时只跳过盖章、不影响读取(下游 Edit/Write 自会因为缺章而拒绝)。

### 3.2 实现 — `pkg/agentstate.SeenFiles`

```go
type AgentState struct {
    seenFiles sync.Map // path → size
}
func (s *AgentState) MarkRead(path string, size int64)
func (s *AgentState) WasRead(path string) (int64, bool)
```

`sync.Map` 因为同 `execution_group` 批内工具**并行跑**——多 goroutine 可能并发 `MarkRead`,并发安全是硬要求。

### 3.3 接线 — `reqctx.WithAgentState`

```go
ctx = reqctxpkg.WithAgentState(ctx, agentstatepkg.New())
```

host(chat M5.2 / subagent M3.3 / scheduler M4.3)在跑 `loop.Run` 前 seed,本包工具通过 `reqctxpkg.GetAgentState(ctx)` 取出。**接线方在波次 3-4 实接**;本轮只立 seed/get 契约。

### 3.4 size 漂移检测(Edit 专属)

Edit 在守卫链中比 Write 多一步:**当前 size 必须等于最近一次 `MarkRead` 盖章的 size**。这是廉价的外部修改检测——

- 漏:同 size 内容互换(`v1` 取舍,接受)
- 拦:外部 IDE/git 改动后 size 变化的常见场景

Edit 漂移后返:`File has been modified since last read (current size N, expected M): <path>. Read it again before editing.`,文件不动。

Write 不做这一步——Write 是**整盘覆盖**,size 不该是闸门;只查"读过"。

---

## 4. 路径解析与守卫

### 4.1 路径解析 — `fspath.Expand`(永远绝对,无 cwd)

桌面 agent 没有项目根、没有当前目录——它像人点 Finder 一样用绝对路径在整台机器导航。三工具的 `file_path` 先过 `pkg/fspath.Expand`:

- 展开开头的 `~` / `~/sub` 为系统 home(`os.UserHomeDir()`——后端进程天然知道,agent 自己并不知道这是谁的 home)
- 展开后**必须绝对**,否则返 `path must be absolute ...`(没有 cwd 可解析相对路径)
- 返回 `filepath.Clean` 后的干净绝对路径

`ValidateInput` 只查 `file_path` 非空;**绝对性的唯一裁判是 `fspath.Expand`**(在 Execute 里),不在工具层重复判断。

### 4.2 两级守卫 — `Allow` vs `AllowWrite`

由 `pkg/pathguard`(R0003)提供的两级守卫:

| 守卫 | deny 集合 | filesystem 消费者 |
|---|---|---|
| `Allow(path)` | `DefaultDenyList` — `/etc/`, `/usr/`, `~/.ssh/`, `~/.aws/`, `~/.forgify/` 等 | **Read** |
| `AllowWrite(path)` | `DefaultDenyList ∪ DefaultWriteOnlyExtras` — extras 含 `.git/`, `.env*`, `node_modules/`, `.venv/`, `__pycache__/` | **Write** / **Edit** |

写专属 extras = "AI 可读不可写"——`.git/` 看历史 OK 但不许 AI 改 hooks/refs;`.env` 调试时读 OK 但不许 AI 覆盖真 secret 为占位串。

cleaner 把"AI 永不该改写 git 历史 / 覆盖 .env"从无防护升级为**物理拦截**。这是 R0003 早铺好的地基,filesystem 这一轮消费。

---

## 5. 原子写

Write / Edit 走相同的原子写序列:

```
CreateTemp(parent, ".forgify-write-*" | ".forgify-edit-*")
  → WriteString(content)
  → Close()
  → Chmod(tmp, mode)         # 覆写时 mode = 原文件 mode；新建时 mode = 0644
  → Rename(tmp, target)      # 原子替换
  → MarkRead(target, len)    # 更新 SeenFiles 盖章
```

**为什么显式 Chmod**:`os.CreateTemp` 默认 0600。若直接 rename 上去覆写一个 0644 文件,会**静默把权限收紧**——下次别的进程读不到。所以必须 chmod 回原 mode。新建文件用 0644(类 Unix 默认)。

失败任一步:`os.Remove(tmpPath)` 清理临时文件,返软失败串给 LLM。

---

## 6. Read 的细节

### 6.1 cat -n 格式

每行 `"%5d\t<line>\n"`——5 位行号 + tab + 内容。LLM 用这个行号定位,后续 Edit 拿到的 `old_string` 必须**去掉行号前缀**(`Edit` 的 description 已声明)。

### 6.2 offset / limit

- 默认 `offset=1, limit=2000`(1-based)
- 用户/LLM 传 0 等价于默认(`ValidateInput` 已拦负数)
- 到 limit 后多 scan 一行判截断:若仍有内容,追加 `... [truncated at line N; use offset+limit to read more]`

### 6.3 单行 8 MiB 上限

`bufio.Scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)`——单行超 8 MiB 会**报错而非静默截断**。和 Claude Code 一致,足以容纳几乎任何真实源码文件,同时限内存。

### 6.4 空文件 / 目录 / 不存在 / 权限

| 场景 | 返串 |
|---|---|
| 空文件 | `<system-reminder>File exists but has empty contents.</system-reminder>` + 仍 `MarkRead(path, 0)` |
| 目录 | `Path is a directory, not a file: <path>. Use Glob with pattern "*" to list a directory.` |
| 不存在 | `File not found: <path>` |
| 无权限 | `Permission denied: <path>` |
| 其他 stat 错 | `Cannot access <path>: <err>` |

> "use Glob" 是给 LLM 的引导——`Glob` / `Grep` 工具由 **`tool/search`**(M2.3 下一个)提供,不在本包。

---

## 7. Edit 的细节

### 7.1 字面量替换(非正则)

`strings.Count(content, old)` + `strings.Replace`——空白/大小写敏感。不支持正则,故 `old_string` 必须是**完全字面**。

### 7.2 命中数分支

| 命中数 | `replace_all=false` | `replace_all=true` |
|---|---|---|
| 0 | `old_string not found in the file. Verify the exact text (whitespace and case matter).` | 同 |
| 1 | `Replaced 1 occurrence in <path>.` | `Replaced 1 occurrence in <path>.` |
| N≥2 | `Found N matches of old_string in <path>, but replace_all is false. Either provide more surrounding context to make old_string unique, or set replace_all: true.` | `Replaced N occurrences in <path>.` |

### 7.3 空操作拒绝

`ValidateInput` 拒绝 `old_string == new_string`(浪费 LLM 一步)。`new_string` 可为空(删匹配文本是合法用法)。

---

## 8. 不做什么 — 边界

- ❌ **`Glob` / `Grep` / `LS`** — 归 **`tool/search`**(M2.3 下一个)
- ❌ **任何"当前目录"/ cwd 状态** — **cwd 概念全局废弃**:桌面 agent 永远用绝对路径(`~` 由 `fspath` 展开),不维护"现在在哪"。`tool/shell`(M3.7)也将无 cwd 持久化
- ❌ **Notebook 编辑**(`.ipynb`)/ **二进制读** / **多文件 patch** — 旧版没有,新版不预建
- ❌ **HTTP 端点 / DDL / 错误码** — 工具失败永不冒泡到 HTTP,只回 tool-result 串供 LLM 自纠;无 sentinel,无 wire code,无 DB schema

---

## 9. 跨域接线(实接在后续波次)

| 接线 | 当下 | 实接 |
|---|---|---|
| `WithAgentState` seed | 本轮立契约 | chat M5.2 / subagent M3.3 / scheduler M4.3 |
| ~~`agentstate.cwd` 字段~~ | **废弃** | cwd 概念不存在;shell 也走绝对路径、无持久工作目录 |
| `agentstate.activeSkill` 字段 | 不建 | skill M3.5 |
| `agentstate.activatedGroups` 字段 | 不建 | toolset M2.3 后续 |
| 三工具装入 `Toolset.Resident` | host 调 `FilesystemTools(pathGuard)` | chat M5.2 host 组装 |
| `PathGuard` 实例 | 默认 `pathguardpkg.NewDefault()` | server boot M7 |

---

## 10. 测试矩阵

全离线 / `t.TempDir()` / 自注入 AgentState:

- **read_test**:`ValidateInput` 5 例 · 路径校验 / 边界 / 空文件 / 目录 / 不存在 / pathguard 拒 / 无 state 容忍跳过盖章 / cat -n 行号格式 / offset+limit 窗口 / 截断标记 / `MarkRead` 写入
- **write_test**:`ValidateInput` 4 例 · 新建文件 / 覆写需先 Read(fail-closed)/ Read 后覆写成功 / 无 state 时覆写拒(fail-closed)/ 父目录不存在 / 父是文件 / 目标是目录 / `AllowWrite` 拒 `.git/` / mode 保留(0600 不被 CreateTemp 0600 默认覆盖)
- **edit_test**:`ValidateInput` 8 例 · 单替换 / `replace_all` 多替换 / 多匹配无 `replace_all` 拒 / 0 匹配 / 写前必读 / 无 state 时 fail-closed / size 漂移拒 / 文件不存在 / 目录 / `AllowWrite` 拒 `.env` / mode 保留

---

## 11. 决策快照

- **danger 不静态声明**:LLM 逐次自报(M2.1 R0030 纯信任)。前端在收到 tool_call 的 `danger` ≥ cautious 时标记;dangerous 阻塞的 ask 流由 loop M2.2 后续接(波次 6)
- **写前必读 fail-closed**:静默允许会让不变式形同虚设——agentstate 缺失时显式拒比沉默放过更安全
- **size 漂移检测仅 Edit**:Write 是整盘覆盖,size 不该是闸门;Edit 是原地改,size 是廉价的"外部改动"指示器
- **`.git/` `.env` 写禁区由 pathguard 而非工具**:R0003 已铺好的地基,filesystem 消费而非重写;改默认 deny 集 → 改 pathguard 一处
- **agentstate 渐进生长**:本轮只引入 `SeenFiles`,其他字段等各自首个消费者(shell / skill / toolset)按需追加。不预留
