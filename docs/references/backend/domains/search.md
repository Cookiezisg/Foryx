---
id: DOC-120
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-06
review-due: 2026-09-01
audience: [human, ai]
---
# Search Tools — 文件导航三件套(LS / Glob / Grep)

> **核心地位**:`tool/search` 是 LLM 在整台机器上**找文件**的导航能力——三工具对应人找文件的三个动作:**打开文件夹看一眼**(LS)/ **按名字找**(Glob)/ **按内容找**(Grep)。叶子工具适配器(无 domain / store / handler / DDL / HTTP 端点),三者皆只读、共享注入的 `PathGuard`、永不碰 AgentState。
>
> **设计哲学**:桌面 agent **没有项目根、没有当前目录(cwd)**。它像人用 Finder + Spotlight 那样,从 `~` 出发、看一眼→下钻→撒网→读,全程用**绝对路径**(`~` 由 `pkg/fspath` 展开)。不像 IDE 里的 coding agent 钉死在一个 repo 里。

---

## 1. 物理布局

```
backend/internal/app/tool/search/
├── search.go        # SearchTools(pathGuard, log) → [LS, Glob, Grep] + 探测 rg
├── ls.go            # LS    — 列一层目录,目录优先
├── glob.go          # Glob  — 按名字模式找(doublestar ** 递归)
├── grep.go          # Grep  — 主入口 + 双后端分派
├── grep_rg.go       # Grep ripgrep 后端
└── grep_stdlib.go   # Grep 纯 Go 兜底后端
```

无 domain / store / handler / DDL / HTTP 端点。装配器 `SearchTools(pathGuard, log)` 返三件套,由 host 装入 `Toolset.Resident`。

---

## 2. 为什么没有 cwd(设计哲学)

| | Claude Code | Forgify 桌面 agent |
|---|---|---|
| 形态 | CLI,`cd ~/proj && claude` | 常驻桌面 app,服务整台机器 |
| 工作根 | 启动即在项目里(隐含 cwd) | **无**——全电脑范围,靠用户交互定位 |
| 工具路径 | 相对 cwd 寻址 | **永远绝对**,`~` 由 `fspath` 展开 |
| 导航方式 | 项目内相对路径 | 看→下钻→撒网→读(像人用 Finder) |

照搬 CLI 的"隐含 cwd"在桌面 agent 上是错的:进程 cwd 是 app 安装目录,对用户毫无意义。Forgify 干脆**废弃 cwd 这个概念**——工具无状态,每次调用都是独立的绝对路径。Agent 的"我现在在看哪"只活在它自己的推理链里,不是系统记的状态。

---

## 3. 三件工具

每件按 `app/tool` 的 **5 方法接口**。三个标准字段(`summary` / `danger` / `execution_group`)由 framework 注入 schema、从 args 剥离——**工具代码不碰 danger**,危险由 LLM 逐次自报(M2.1 纯信任)。

| 工具 | 人的动作 | 业务参数 | 逻辑 | 输出 |
|---|---|---|---|---|
| **LS** | 打开文件夹看一眼 | `path`(必填)· `limit?` | `fspath.Expand` → `Allow` → `os.ReadDir` **一层** → 目录排前 + 名字序 → 截断 | 轻结构化文本(含隐藏文件) |
| **Glob** | 按名字找 | `pattern`(必填)· `path`(根,必填)· `limit?` | `Expand` → `Allow` → `doublestar.Glob`(`**` 递归)→ stat → mtime 降序 → 截断 | JSON `{root,matches,total,truncated}` |
| **Grep** | 按内容找 | `pattern` · `path`(根,必填)· `glob?` · `type?` · `output_mode?` · `-A/-B/-C` · `-n` · `-i` · `multiline?` · `head_limit?` | `Expand` → `Allow` → 双后端(rg / stdlib) | 纯文本(对齐 rg `--no-heading`) |

**LS 是新增工具**(旧实现用 `Glob "*"` 兼任)。在全电脑导航模型里,"打开文件夹看一眼再下钻"是最高频动作,值得当一等工具——LLM 见到 `LS` 天然知道是列目录,比 `Glob "*"` 的模式匹配语义更直白。三者职责正交:LS 看一层(目录优先,为导航优化)、Glob 递归模式撒网、Grep 内容撒网。

---

## 4. 工作流 = 看 → 下钻 → 撒网 → 读

三工具的真正价值在**怎么串起来找文件**,跟人用 Finder + Spotlight 一模一样:

```
用户:"我那个处理 Excel 的 Python 脚本,忘了在哪"
  LS   ~                → 顶层有 projects/ Documents/ Desktop/ ...
  LS   ~/projects       → 下钻,看到十几个项目目录
  Grep ~/projects "pandas.*excel|openpyxl" --type py   → 缩到 3 个文件
  Read 命中文件(filesystem)                            → 读懂
```

起点 = `~` + OS 标准目录(`~/Desktop` `~/Downloads` `~/Documents` 是 OS 常识,Agent 天然会用)。`search` 找到路径后,交给 `filesystem` 的 Read/Edit 读改。

---

## 5. 路径:绝对 + `~`(`fspath`)

三工具的 `path` 都过 `pkg/fspath.Expand`(与 filesystem 共用):

- `path` **必填**——没有 cwd 可作默认(`ValidateInput` 拦空,`Expand` 是绝对性的最终裁判)
- 支持 `~` / `~/sub`,工具层 `os.UserHomeDir()` 展开(Agent 不知道你 home 是谁,这事工具替它干)
- 展开后必须绝对,否则返 `path must be absolute ...`

---

## 6. Grep 双后端

```
装配时 exec.LookPath("rg")
  ├─ 有 rg → execRg(rg 子进程)  失败 → 回退 execStdlib + log.Warn
  └─ 无 rg → execStdlib(纯 Go regexp + WalkDir)
```

| | ripgrep 后端 | stdlib 兜底后端 |
|---|---|---|
| 实现 | `exec.CommandContext` 翻译 CLI flag | `regexp` + `filepath.WalkDir` + `bufio` |
| 退出码 | 0 匹配 / 1 无匹配(非错)/ 2 真错 | — |
| 噪音目录 | rg 原生尊重 `.gitignore` | `noiseDirs` 手动跳 `.git/node_modules/.venv/__pycache__/.forgify` |
| type 过滤 | `--type go` | `typeExtensions` 26 语言→扩展名 map |
| 单行 / 文件上限 | rg 自管 | 单行 8 MiB · multiline 整文件 32 MiB |

**为什么必须双后端**:Forgify 是 Wails 桌面 app,分发给最终用户——不能假设用户机器装了 rg(且项目一贯"不代装外部二进制",同 sandbox docker 决策)。stdlib 是**零依赖兜底**(保证"总能 Grep");rg 是**有则更快 + 尊重 gitignore + PCRE**。两后端共享 args / 三种 `output_mode`(content / files_with_matches / count)/ cap 语义,使 LLM 不论是否装 rg 都看到一致行为。rg 失败静默回退 stdlib(PCRE-only 正则可能行为漂移,`log.Warn` 已坦白)。

---

## 7. 先缩范围纪律

工具描述硬性引导"**先缩范围,绝不无脑全盘**":`Glob ~ "**/*"` 会扫几十万文件、又慢又呛 token——就像人不会"在整个电脑里 grep",而是先 LS 看顶层、下钻到合理的区,再撒网。这条纪律写进 Glob / Grep 的 description,逼 Agent 学人的节奏。

---

## 8. 不做什么 — 边界

- ❌ **任何 cwd / 当前目录状态** — 全局废弃(见 §2);path 永远绝对
- ❌ **网络搜索** — 归 `tool/web`(M2.3 下一个);search 只搜本机物理文件
- ❌ **实体搜索**(`search_function` 等) — 那是 catalog 两段式的第二段,搜 Forgify 实体,波次 3 各实体域提供;`tool/search` 搜的是**物理文件**,两者正交
- ❌ **HTTP 端点 / DDL / 错误码** — 工具失败永不冒泡 HTTP,只回 tool-result 串供 LLM 自纠

---

## 9. 跨域接线(实接在后续波次)

| 接线 | 当下 | 实接 |
|---|---|---|
| 三工具装入 `Toolset.Resident` | host 调 `SearchTools(pathGuard, log)` | chat M5.2 host 组装 |
| `PathGuard` 实例 | 默认 `pathguardpkg.NewDefault()` | server boot M7 |
| `rg` 二进制 | `exec.LookPath` 探测,无则 stdlib | 不代装(用户自备) |

---

## 10. 测试矩阵

全离线 / `t.TempDir()`:

- **ls_test**:`ValidateInput`(空 path / 负 limit) · 目录优先(`zsub` 排在 `afile` 前)+ 隐藏文件显示 · 非目录 / 不存在 / 空目录 / 截断 / pathguard 拒
- **glob_test**:`ValidateInput`(空 pattern / 空 path / 负 limit) · `**` 递归 / 非递归 · mtime 降序(`Chtimes` 控制) · limit 截断 + truncated · JSON 形状 · root 不存在 / 非目录 / pathguard 拒
- **grep_test**:`ValidateInput` · **stdlib 后端**全覆盖(files_with_matches / content + `-n` / 上下文 `-A`/`-B` / count / type 过滤 / `-i` / multiline / 无匹配 / head_limit / 跳 noiseDirs / pathguard 拒)· `buildRgArgs` flag 翻译 · **rg 后端集成**(装了 rg 则跑真实 rg,验证两后端一致)

---

## 11. 决策快照

- **没有 cwd**:桌面 agent 全电脑范围、靠交互定位,永远绝对路径;删掉 cwd 比保留更简单更对(这是重建相对照搬 Claude Code 的核心纠正)
- **LS 独立成工具**:列目录是导航最高频动作,扶正为一等工具而非 `Glob "*"` 兼任
- **双后端保留**:桌面分发不能假设有 rg + 不代装二进制 → stdlib 兜底必须有;rg 有则更快。464 行 stdlib 是"对齐 rg 功能集"的必要代价,不是脂肪
- **path 必填**:没有 cwd 可作默认,逼 Agent 显式说从哪搜(它会用 `~`)
- **`~` 展开在工具层**:Agent 不知道你 home 是谁,`fspath.Expand` 替它解析(`os.UserHomeDir`)
- **danger 工具侧零逻辑**:三工具皆只读,danger 由 LLM 逐次自报,工具代码不碰
- **`Glob "*"` 与 `noiseDirs` 不冲突**:pathguard 守搜索根(防越权);noiseDirs 是 stdlib 遍历时跳噪音子目录(性能),两者正交
