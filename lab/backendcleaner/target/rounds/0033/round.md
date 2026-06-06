---
# Round 0033 — tool/search 三件套 + 新建 pkg/fspath + filesystem 回溯（波次 2 · M2.3#2）

类型 / 目标:M2.3 叶子工具第 2 个——`tool/search`。但设计讨论把它从"照搬"撑大成**确立桌面 agent 的"全电脑绝对路径、无 cwd"范式**:新建 `pkg/fspath` 地基、LS 独立成工具、search 三件套改 path 语义、并**回溯改上一轮已提交的 filesystem** 补 `~` 展开。

## 核心方针(一句话)
**search = 文件导航三件套(LS 看一眼 / Glob 按名找 / Grep 按内容找),对应人用 Finder + Spotlight 找文件;桌面 agent 没有项目根、没有 cwd,永远绝对路径 + `~` 工具层展开。**

## 考古发现
- 旧 search 5 源 ~1045 行 + 测试 992 行;只 2 工具(Glob + Grep,**无 LS**,用 `Glob "*"` 兼任);Grep **双后端**(rg 优先 + 纯 Go stdlib 兜底)。0 重大设计 bug。
- 旧 `glob.go`/`grep.go` 的 `normalize()` 有 `path == "" → os.Getwd()` 默认——这在桌面 app 里 = 搜 app 安装目录,**坏默认**。
- 旧 search 连 `agentstate.cwd` 都不看(用 `os.Getwd()`),跟 Bash 维护的 cwd 是**两套**——印证 cwd 模型本就混乱。
- 旧 search.md 腐烂(`grep_search`/`glob` 工具名错、`Permissions`/`protectedPaths`、"多线程扫描"、`SearchBlock` SSE、5 个虚构 sentinel)。

## 关键决策(用户拍板 — 这是重建的重要意义)
1. **cwd 概念全局废弃**:桌面 agent 是"全电脑助手",工作范围是整台机器,去哪由用户交互定位——**没有项目根、没有当前目录**。工具无状态,永远绝对路径。**比照搬 Claude Code 的项目级 cwd 更简单更对**。连带:`agentstate` 永不加 cwd 字段;shell(M3.7)也将无 cwd 持久化。
2. **新建 `pkg/fspath`**:`Expand(path)` — 展开 `~`/`~/sub`(`os.UserHomeDir`,后端进程天然知道、agent 不知道这是谁的 home)→ 必须绝对否则 err → Clean。**是"永远绝对"铁律的唯一物理执行点**,filesystem + search 六工具共用。
3. **LS 独立成工具**:列目录是全电脑导航最高频动作(看一眼→下钻),扶正为一等工具,不再用 `Glob "*"` 兼任。三件套职责正交(LS 看一层 / Glob 递归名 / Grep 内容)。
4. **双后端全保留**:桌面分发不能假设有 rg + 不代装二进制 → stdlib 兜底必须有;rg 有则更快 + 尊重 gitignore + PCRE。464 行 stdlib 是"对齐 rg 功能集"的必要代价。
5. **danger 工具侧零逻辑**:三工具皆只读,danger 由 LLM 逐次自报,工具代码一个字不碰(M2.1 纯信任 + M2.2 纯标记)。

## 新实现
- `pkg/fspath/fspath.go`:`Expand` + 3 sentinel(ErrEmptyPath/ErrNotAbsolute/ErrNoHome)。
- `app/tool/search/search.go`:`SearchTools(pathGuard, log)` 装配三件套 + `exec.LookPath("rg")` 探测。
- `app/tool/search/ls.go`:**LS 新工具**(ReadDir 一层 / 目录优先 / 隐藏文件显示 / humanBytes / 截断)。
- `app/tool/search/glob.go`:Glob(删 os.Getwd 默认 → path 必填 + `fspath.Expand`;doublestar `**`;mtime 降序;JSON)。
- `app/tool/search/grep.go` + `grep_rg.go` + `grep_stdlib.go`:Grep 双后端(改 path 语义;rg/stdlib 照搬)。

## 回溯改 filesystem(上一轮已提交 bfcd4cfa)
- `read.go`/`write.go`/`edit.go`:path 处理改走 `fspath.Expand`(支持 `~`);**删** `IsAbs` 检查 + `ErrPathNotAbsolute`(绝对性裁判下沉 fspath,工具层不重复)。
- 三 `_test.go`:删 ValidateInput 的相对路径 case,加 Execute 层 `~` 展开 + 相对拒绝测试。

## 测试(全离线,0 token)
- `pkg/fspath` 6:空/相对拒/绝对 Clean/`~`/`~/sub`/`~user` 不支持。
- search 三测试:ls 8(目录优先+隐藏/非目录/不存在/空/截断/pathguard) · glob 8(`**` 递归/非递归/mtime 降序/limit/JSON/root 错/pathguard) · grep 13(stdlib 全模式+上下文+type+`-i`+multiline+无匹配+head_limit+跳 noiseDirs+pathguard / buildRgArgs / **rg 集成 skip-if-absent**)。
- filesystem 回溯:三工具各加 `~` 展开 + 相对拒绝(共 6 新测试)。

## 验证
`gofmt -l` 干净 · `go build ./...` 0 · `go vet` 0 · `go test -race -count=1`(fspath 1.9s / search 2.5s / filesystem 2.0s,**含 rg 真实后端集成**)· `go mod tidy`(新增 `doublestar/v4 v4.10.0` 转 direct)。

## 契约
- `domains/search.md` **整篇重写**(DOC-120):三件套 + 无 cwd 哲学 + `~` 展开 + 看→钻→撒网→读工作流 + 双后端 + 先缩范围纪律。
- `domains/filesystem.md`:加 §4.1 路径解析(`fspath.Expand`)+ 清 cwd 尾巴(边界/接线表 cwd → 废弃)。
- `contract-changes.md` #13 search + #12 补注 filesystem `~` 展开。
- 无新 HTTP 端点 / 无 DB 表 / 无 error code(工具失败永不冒泡 HTTP)。

## 跨波次接线
- **三工具装入 `Toolset.Resident`** → chat M5.2 host 组装。
- **`PathGuard` 实例** → server boot M7 `pathguardpkg.NewDefault()`。
- **`rg` 二进制** → 不代装,`exec.LookPath` 探测无则 stdlib。
- **cwd 废弃连带** → shell M3.7 也无 cwd 持久化(Bash 命令若需工作目录,命令内显式 `cd /abs &&`,不跨调用记忆)。

## 波次 2 进度
M2.1 tool ✅ → M2.2 loop ✅ → M2.3 #1 filesystem ✅ → **#2 search ✅**(+ 建 pkg/fspath + cwd 全局废弃)→ #3 web / #4 toolset。
