---
id: DOC-108
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Filesystem Tools — 文件操作、保护与原子写入原理

> **核心职责**：Filesystem 是 Forgify 赋予 Agent 的 **“读写手臂”**。它允许 LLM 操作宿主机文件（代码库、配置文件等）。为了平衡效率与安全，系统实现了一套基于 **“路径保护”** 和 **“原子替换”** 的物理操作机制。

---

## 1. 核心原理 (Principles)

### 1.1 Protected Paths (物理禁区)
为了防止 LLM 越权读取系统隐私或破坏关键文件，后端在执行任何 FS 工具前都会进行 **“前置路径校验”**：
- **原理**：基于 `settings.json` 中的 `protectedPaths` 列表。
- **禁区**：默认拒绝访问 `~/.ssh/`, `/etc/`, `/var/`, 以及 Forgify 自身的数据库文件 `forgify.db`。
- **匹配算法**：物理路径归一化（Absolute Path Clean）后执行子集判定，防止利用 `../` 符号链接绕过。

### 1.2 Atomic-Atomic Rewrite (原子替换)
为了防止文件写入过程中的断电损坏或脏数据产生：
- **步骤**：`fs_edit` 或 `fs_write` 工具首先在目标目录创建一个带有随机后缀的 `.tmp` 文件。
- **同步**：内容写入完成后执行 `Sync()` 物理刷盘。
- **原子原子操作**：最后调用 `os.Rename()` 瞬间替换目标文件。
- **效果**：对前端和其它进程来说，文件要么是旧的，要么是完整的新内容，不存在中间态。

### 1.3 Read-Only Safeguard (只读模式)
- **原理**：所有的 `read_file`, `list_directory`, `grep_search` 工具在静态元数据中被标记为 `IsReadOnly: true`。
- **权限分流**：这些工具即便被 LLM 疯狂调用，也绝对不会触发磁盘写入动作，物理上消除了该层面的副作用风险。

---

## 2. 工具规约 (The Tools)

### 2.1 `read_file`
- **限制**：单次读取上限 1MiB。
- **特性**：自动检测文本/二进制。若发现是二进制文件，则仅返回 Base64 编码的头信息。

### 2.2 `write_file`
- **鉴权**：必须在 Workspace 根目录或指定的子目录下执行。
- **模式**：强制执行“完全覆盖”语义。

---

## 3. 生命周期 (Lifecycle)

1. **寻址 (Normalization)**：LLM 给出相对路径 `src/main.go`。
2. **鉴权 (Guarding)**：后端将其拼装为全路径并检查 `protectedPaths`。
3. **备份 (Buffering)**：对于修改操作，系统在内存或临时文件中生成副本。
4. **提交 (Committing)**：原子替换物理文件。
5. **通知 (Notifying)**：发布 `document` 类型的通知（若是文档库内容），提示 UI 刷新。

---

## 4. 跨域集成 (Interactions)

- **Permissions**：提供物理路径的黑白名单。
- **Sandbox**：Bash 工具运行 `ls/cat` 命令时，底层其实调用了 FS 域的逻辑。
- **Document**：Document 实体的内容同步通常依赖 FS 域进行物理落盘。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 场景 |
|---|---|---|---|
| `ErrPathForbidden` | 403 | `PERM_DENIED` | 命中保护名单（如 .ssh）。 |
| `ErrPathOutside` | 403 | `PERM_DENIED` | 尝试逃逸出 Workspace 根目录。 |
| `ErrFileSyncError` | 500 | `INTERNAL_ERROR` | 磁盘空间不足或硬件故障。 |
| `ErrNotFound` | 404 | `NOT_FOUND` | 文件路径不存在。 |
| `ErrIsDirectory` | 422 | `INVALID_REQUEST` | 尝试把文件夹当文件读。 |
