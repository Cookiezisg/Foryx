# 沙箱迭代 1：uv + 捆绑 Python + 每 EnvID 一个独立 venv

**日期**：2026-05-02
**状态**：🔄 设计中（未开工）
**阶段定位**：Phase 3 后优化轮的一项；非 Phase 4 阻塞前置
**关联**：
- 现状代码：[`backend/internal/infra/sandbox/python.go`](../../../../backend/internal/infra/sandbox/python.go)
- 现状代码：[`backend/internal/app/forge/ast.go`](../../../../backend/internal/app/forge/ast.go)
- 桌面打包大方向：[`../desktop-packaging-notes.md`](../desktop-packaging-notes.md) §五
- forge domain 详设：[`../../service-design-documents/forge.md`](../../service-design-documents/forge.md)

---

## 0. 用户动线（设计基准）

整个迭代要支持的 LLM-用户协作流程：

```
[1] 用户："帮我做个处理本地 CSV 的工具"
       LLM 调 search_forges → 没找到合适的

[2] LLM 调 create_forge：
       前端流式看到框框框打字：
         name:        csv_parser
         description: Parse local CSV files
         dependencies: [pandas>=2.0]
         code:        def parse_csv(path): ...

       代码定型 → 进入装环境阶段
       前端看到环境进度框框框：
         ⏳ Resolving dependencies...
         ⏳ Downloading numpy...
         ❌ Failed: numpy>=2.0 incompatible with python 3.12

[3] LLM 看到错误，调 edit_forge 改 deps：
       前端流式看到 deps 在变：[pandas>=2.0, numpy>=1.24]
       重新装环境：
         ⏳ Resolving...
         ✅ Ready

[4] 用户审核：[Accept] [Reject]
       点 Accept → forge 激活 v1
       表里：forges.active_version_id 指向这个版本，sandbox 知道用哪个 venv

[5] 之后 LLM 调 run_forge → ~50ms 启动 → 跑 pandas 代码
```

整个沙箱迭代的设计就是为这条动线服务。所有不在这条线上的复杂度（异步队列、安全隔离、超时限制、Forge 静态属性等）一律不进。

---

## 1. 核心实现概览

### 1.1 EnvID：venv 的 identity = deps，不是 version

不同 ForgeVersion 如果依赖集 + Python 版本完全相同，物理上**共用同一个 venv**。EnvID 是这个共享键：

```go
func ComputeEnvID(deps []string, pythonVersion string) string {
    normalized := make([]string, len(deps))
    for i, d := range deps {
        // 去首尾空格 + 包名小写（保留版本约束符号和数字）
        normalized[i] = normalizeSpecifier(d)
    }
    sort.Strings(normalized)
    payload := strings.Join(normalized, "\n") + "\n" + pythonVersion
    h := sha256.Sum256([]byte(payload))
    return "env_" + hex.EncodeToString(h[:6])  // 12 hex chars
}
```

效果：

| Deps | EnvID |
|---|---|
| `[pandas]` | env_aaa |
| `[pandas>=2.0]` | env_bbb（specifier 字符串不同 = 不同 hash） |
| `[pandas>=2.0, numpy]` | env_ccc |
| `[numpy, pandas>=2.0]` | env_ccc（标准化排序后相同） |
| `[Pandas>=2.0, numpy]` | env_ccc（包名小写后相同） |
| `[pandas==2.1.3]` | env_ddd（精确版本是另一个 specifier） |

**关键性质**：同 specifier 必然得同 EnvID；EnvID hit 时**复用现有 venv**，跳过 uv sync——同 specifier 的多个版本零代价共享环境。venv 内的 uv.lock 锁住的精确版本是第一次装时 uv 解析出来的，后续命中的版本继承这套锁——可重现性自动保证。

如果用户/LLM 要"升级到最新 pandas"，唯一方式是**改 specifier**（比如 `pandas>=2.5`）→ EnvID 变 → 装新 venv → 新 lock。这逼显式表达"我要新版本"。

### 1.2 文件磁盘布局

```
<dataDir>/forges/<forge_id>/
├── envs/
│   ├── env_aaa/                    ← venv 按 EnvID 命名（多版本可共用）
│   │   ├── pyproject.toml
│   │   ├── uv.lock                 ← 锁定精确版本（uv sync 产物）
│   │   └── .venv/
│   ├── env_bbb/
│   └── env_ccc/
└── versions/
    ├── fv_<v1-id>/main.py          ← 代码跟 ForgeVersion 一一对应
    ├── fv_<v2-id>/main.py
    └── fv_<pending-id>/main.py
```

跑代码：

```go
// sandbox.Run 接 (forgeID, versionID, envID) 元组
envDir := <dataDir>/forges/<forgeID>/envs/<envID>/
codeFile := <dataDir>/forges/<forgeID>/versions/<versionID>/main.py
exec.Command(uv, "run", "--no-sync", "--project", envDir,
             "python", codeFile)
```

`--no-sync` 是关键 flag——告诉 uv "别检查 lock 是否最新、别动包，直接跑"，把 sync 时机完全交给 service 层。

### 1.3 N=3 EnvID 缓冲

每个 forge 的 envs/ 下最多保留 3 个不同 EnvID 的 venv（按最近使用时间）。超过时删最旧那个目录。

老 ForgeVersion 行不删——它们的 EnvStatus 转 `"evicted"`，revert 到那时触发即时 sync 重建。

---

## 2. 数据模型（3 个 entity 的字段分布）

### 2.1 `Forge` entity

```go
type Forge struct {
    // 既有：id / userId / name / description / code / parameters /
    //       returnSchema / tags / versionCount / createdAt / updatedAt /
    //       deletedAt
    
    // ── 本迭代新增：DB 列 ──
    
    // ActiveVersionID 指向当前活跃版本的 ForgeVersion.ID。
    // 草稿期为空（forge 已建但还没 accept 任何版本）；
    // accept 后指向 accepted 版本。sandbox.Run 据此选 venv。
    //
    // ActiveVersionID 指向当前活跃版本的 ForgeVersion.ID。草稿期为空。
    ActiveVersionID string `gorm:"type:text;default:''" json:"activeVersionId"`
    
    // ── 既有计算字段（gorm:"-"）──
    Pending *ForgeVersion `gorm:"-" json:"pending,omitempty"`  // attachPending 填
    
    // ── 本迭代新增计算字段（gorm:"-"，attachActiveEnv 填）──
    // 从 ActiveVersion 拷过来，让 GET /forges/{id} 直接含当前活跃环境状态。
    EnvStatus     string     `gorm:"-" json:"envStatus"`
    EnvError      string     `gorm:"-" json:"envError"`
    EnvSyncedAt   *time.Time `gorm:"-" json:"envSyncedAt"`
    EnvSyncStage  string     `gorm:"-" json:"envSyncStage"`
    EnvSyncDetail string     `gorm:"-" json:"envSyncDetail"`
}
```

`attachActiveEnv` 在 Get / List 后调用（同 `attachPending` 模式），把 `ActiveVersion` 的 env 字段值拷到 forge 上。`forge.Pending` 子对象自带它自己的 env 状态（pending 的 sync 进度）。

### 2.2 `ForgeVersion` entity

```go
type ForgeVersion struct {
    // 既有：id / forgeId / userId / version / status / name / description /
    //       code / parameters / returnSchema / tags / changeReason /
    //       createdAt / updatedAt
    
    // ── 本迭代新增：依赖配置（随版本快照）──
    
    // Dependencies 是 PEP 508 specifier 列表，例 ["pandas>=2.0", "requests"]。
    // LLM 在 create_forge / edit_forge 时根据代码 import 申报；空数组 = 仅 stdlib。
    //
    // Dependencies 是 PEP 508 specifier 列表，由 LLM 申报。
    Dependencies string `gorm:"type:text;default:'[]'" json:"dependencies"`
    
    // PythonVersion 形如 ">=3.12"，为空时用 sandbox 默认。
    //
    // PythonVersion 形如 ">=3.12"。
    PythonVersion string `gorm:"type:text;default:''" json:"pythonVersion"`
    
    // EnvID 是该版本对应的 venv 物理目录名，由 ComputeEnvID(deps, python) 算出。
    // 多个 ForgeVersion 如果 EnvID 相同，共用 envs/<EnvID>/.venv/。
    //
    // EnvID 是该版本对应的 venv 目录名。同 EnvID 的版本共用 venv。
    EnvID string `gorm:"type:text;index" json:"envId"`
    
    // ── 本迭代新增：环境运行时态（每版本独立）──
    
    // EnvStatus："pending" | "syncing" | "ready" | "failed" | "evicted"
    EnvStatus     string     `gorm:"type:text;default:'pending'" json:"envStatus"`
    EnvError      string     `gorm:"type:text;default:''" json:"envError"`
    EnvSyncedAt   *time.Time `json:"envSyncedAt"`
    EnvSyncStage  string     `gorm:"type:text;default:''" json:"envSyncStage"`
    EnvSyncDetail string     `gorm:"type:text;default:''" json:"envSyncDetail"`
}
```

### 2.3 `ForgeExecution`

Phase 5 已落地，本迭代不动。run_forge / 测试用例都写一行 forge_executions（含 chat 触发上下文）。

### 2.4 表里"哪个 forge 用哪个 uv 环境"

```sql
SELECT 
    f.id          AS forge_id,
    fv.id         AS active_version_id,
    fv.env_id     AS env_id,
    fv.dependencies,
    fv.env_status,
    fv.env_synced_at
FROM forges f
JOIN forge_versions fv ON fv.id = f.active_version_id
WHERE f.id = 'f_xxx';

-- venv 在磁盘：<dataDir>/forges/<forge_id>/envs/<env_id>/.venv/
```

---

## 3. 端到端调用链

### 链 1：create_forge（流式生成 + 同步装环境）

```
LLM tool_call create_forge {name, description, instruction, dependencies}

forgetool.CreateForge.Execute
  → svc.Create(ctx, CreateInput{...})
      → repo.SaveForge → forges 表落库（ActiveVersionID="" 草稿期）
      → repo.SaveVersion → forge_versions 表落库
            status="pending", version=nil,
            EnvID=ComputeEnvID(deps, pythonVersion),
            EnvStatus="pending"
      → publishForgeSnapshot（前端看到 forge.pending 出现）
  
  → streamCode(...) → LLM 逐 token 流出代码
        每个 token：repo.UpdateVersionCode(versionID, partialCode)
                   + publishForgeSnapshot（前端看 forge.pending.code 在长）
  
  → svc.ParseCode(code) AST dry-run
        失败 → 返 LLM tool error 让其重试，不进 sync
  
  → svc.SyncEnvForVersion(ctx, versionID)        ← 同步等
      → repo.UpdateVersionEnvStatus(versionID, "syncing")
      → publishForgeSnapshot（pending.envStatus=syncing）
      → sandbox.Sync(ctx, SyncRequest{
            ForgeID, VersionID, EnvID, Dependencies, PythonVersion,
            OnProgress: func(stage, detail) {
                repo.UpdateVersionEnvProgress(versionID, stage, detail)
                publishForgeSnapshot
            },
        })
      → 成功 → repo.UpdateVersionEnvStatus(versionID, "ready", syncedAt=now)
      → 失败 → repo.UpdateVersionEnvStatus(versionID, "failed", error=stderr)
      → publishForgeSnapshot（终态）
  
  → 返 tool_result {forge_id, version_id, env_status: "ready" | "failed"}
```

### 链 2：edit_forge（草稿期 / 激活期统一入口）

```
LLM tool_call edit_forge {forge_id, instruction?, dependencies?, name?, description?}

forgetool.EditForge.Execute
  → forge := svc.Get(forge_id)
  
  → if forge.Pending != nil:
        // 草稿期 / 已有 pending → 修同一个 ForgeVersion
        target = forge.Pending
    else:
        // 激活后改进 → 基于 active 复制出新 pending
        target = svc.CreatePendingFromActive(forge_id)
  
  → if instruction != "":
        streamCode(target.Code, instruction) → publishForgeSnapshot 流帧
        ParseCode dry-run
  
  → newEnvID = ComputeEnvID(newDeps, newPython)
  
  → if newEnvID != target.EnvID:
        // deps / python 改了：新 EnvID
        repo.UpdateVersionEnvID(target.ID, newEnvID)
        repo.UpdateVersionEnvStatus(target.ID, "pending")
        svc.SyncEnvForVersion(ctx, target.ID)        ← 同步装新 venv
                                                       （命中已有 EnvID 则跳过）
    else:
        // deps 没变：venv 复用，只改 main.py
        sandbox.WriteCodeFile(forgeID, target.ID, code)
        repo.UpdateVersionEnvStatus(target.ID, "ready")
  
  → 返 tool_result {pending_id, env_status}
```

### 链 3：用户 Accept

```
POST /api/v1/forges/{id}/pending:accept

handler → svc.AcceptPending(ctx, forgeID)
  → pending := repo.GetActivePending(forgeID)
  → 校验 pending.EnvStatus == "ready"（环境没装好不能 accept）
  → repo.UpdateVersionStatus(pending.ID, "accepted", version=N+1)
  → repo.UpdateForge(forgeID, ActiveVersionID=pending.ID,
                     Code/Parameters/.../=同步 pending 字段)
  → svc.trimEnvBuffer(forgeID)    ← 超 N=3 EnvID 删最旧
  → publishForgeSnapshot（pending 字段消失，active 切换）
  → 200
```

### 链 4：run_forge（热路径）

```
LLM tool_call run_forge {forge_id, input}

forgetool.RunForge.Execute
  → resolveAttachments
  → svc.RunForge(ctx, forgeID, input)
      → forge := repo.GetForge → forge.ActiveVersionID
      → activeVer := repo.GetVersion(forge.ActiveVersionID)
      → 检查 activeVer.EnvStatus
            "ready"   → 走第 4 步
            "evicted" → 同步触发懒重建 sync → 5-15s → 走第 4 步
            其他      → 返 ErrEnvNotReady
      → sandbox.Run(ctx, RunRequest{
            ForgeID:   forgeID,
            VersionID: activeVer.ID,
            EnvID:     activeVer.EnvID,
            Code:      activeVer.Code,
            Input:     input,
        })
      → repo.SaveExecution（既有 forge_executions 表）
  → 返 tool_result {ok, output, ...}
```

### 链 5：revert + 删除

```
POST /api/v1/forges/{id}:revert {version: 1}
  → v1 := repo.GetVersion(forgeID, 1)
  → if v1.EnvStatus == "evicted":
        repo.UpdateVersionEnvStatus(v1.ID, "pending")
        svc.SyncEnvForVersion(ctx, v1.ID)    ← 同步重建
  → repo.UpdateForge(forgeID, ActiveVersionID=v1.ID, Code/.../=v1 快照)
  → publishForgeSnapshot
  → 200

DELETE /api/v1/forges/{id}
  → repo.DeleteForge（软删，沿用既有逻辑）
  → sandbox.Destroy(forgeID)（rm -rf forges/<id>/）
  → 204
```

---

## 4. Sandbox 模块（infra 层）

### 4.1 文件结构（§S12 平铺）

```
internal/infra/sandbox/
├── sandbox.go        ← Package doc + Sandbox struct + Bootstrap + Run + Destroy
├── sync.go           ← Sync()：跑 uv sync + 调 OnProgress
├── progress.go       ← 解析 uv stderr 行 → (stage, detail)
├── paths.go          ← 路径解析 + ComputeEnvID + normalizeSpecifier
├── pyproject.go      ← 渲染 pyproject.toml
└── sandbox_test.go
```

类型名 `PythonSandbox` → `Sandbox`（不只跑 Python，还管 uv venv）。包别名按 §S13 仍 `sandboxinfra`。

### 4.2 接口

```go
type Config struct {
    DataDir       string  // <dataDir>
    UVPath        string  // 已就绪的 uv 二进制路径（Bootstrap 之后填）
    PythonPath    string  // 捆绑 Python 解释器路径（Bootstrap 之后填）
    DefaultPython string  // ">=3.12"
    Logger        *zap.Logger
}

type Sandbox struct { ... }

func New(cfg Config) *Sandbox

// Bootstrap 启动期解压 uv + python 到 dataDir，幂等。
func (s *Sandbox) Bootstrap(ctx context.Context) error

// Sync 物化某 ForgeVersion 对应的 venv 目录（按 EnvID 命名）。
// EnvID 已存在则跳过；不存在则跑 uv sync。OnProgress 在每行 stderr 解析后调用。
func (s *Sandbox) Sync(ctx context.Context, req SyncRequest) error

// Run 在 ready 的 venv 中执行代码。无 timeout——只随上游 ctx 取消终止。
func (s *Sandbox) Run(ctx context.Context, req RunRequest) (*forgedomain.ExecutionResult, error)

// WriteCodeFile 只更新 main.py 不动 venv（用于 deps 没变只改代码）。
func (s *Sandbox) WriteCodeFile(forgeID, versionID, code string) error

// Destroy 删整个 forge 目录（forge 软删时调）。
func (s *Sandbox) Destroy(ctx context.Context, forgeID string) error

// DestroyEnv 删某 EnvID 的 venv 目录（N=3 缓冲超出时调）。
func (s *Sandbox) DestroyEnv(ctx context.Context, forgeID, envID string) error

type SyncRequest struct {
    ForgeID       string
    VersionID     string
    EnvID         string
    Dependencies  []string
    PythonVersion string
    OnProgress    func(stage, detail string)  // 每行 stderr 解析后调，可为 nil
}

type RunRequest struct {
    ForgeID   string
    VersionID string
    EnvID     string
    Code      string
    Input     map[string]any
}
```

**沙箱不知道 forge 概念**——只接 `(forgeID, versionID, envID)` 元组操作目录。也不直接调 `bridge.Publish`——通过 `OnProgress` callback 把进度数据流给 forgeapp，由 forgeapp 写库 + 推 forge 快照。这跟 Phase 6 chat.runner 是 chat.message 唯一发布事实源同模式。

### 4.3 Bootstrap

```
1. 确保 <dataDir>/{bin,forges} 目录
2. 从 cmd/desktop 的 embed.FS 提取 uv → <dataDir>/bin/uv（chmod +x）
   dev 模式：从 $FORGIFY_DEV_RESOURCES/bin/ 拷
   hash 一致跳过
3. 同样方式解压 python-build-standalone → <dataDir>/bin/python/
4. mac 早期阶段（未公证）：
   xattr -dr com.apple.provenance <python dir>
   walk <python dir> 对所有可执行文件（mode & 0111 != 0）跑 codesign --force --sign -
   失败 fail loud（Bootstrap 失败 → sandbox unavailable）
   ── 这是 issue uv#16726 的 fix 模式：python-build-standalone 二进制带
   com.apple.provenance xattr + 仅 ad-hoc 签，会被内核 SIGKILL（无日志）。
   uv 自己的 install 路径里也跑这套 codesign，我们走 embed.FS 解压不经过
   uv install，得自己跑。
5. mac v1.0+（已公证）：上面这步可省——公证 ticket 已经覆盖 .app 内所有
   嵌入二进制（含 uv + python）。但 .app entitlements 必须含
   `com.apple.security.cs.disable-library-validation`，否则 forge 装新依赖
   后 dlopen wheel 里的 .so 会被 Hardened Runtime 拦。详见
   desktop-packaging-notes.md 第六节。
6. 跑 `uv --version` 校验
7. 跑 `<bundled-python> -c "import sys"` 校验
8. 设 UV_PYTHON 环境变量 = <bundled-python-path>，所有 sandbox 子进程默认
   用我们捆绑的 Python（见 §4.4 withUVEnv 实现）
9. 填 cfg.UVPath / cfg.PythonPath
```

`cmd/server` 启动时调 Bootstrap；失败不阻断 backend 启动，但 sandbox 状态记 `unavailable`，后续 forge 操作返 `ErrSandboxUnavailable`。

**关于 mac 公证的两阶段路线**（按 desktop-packaging-notes.md 第六节）：

| 阶段 | 状态 | Bootstrap 步 4 |
|---|---|---|
| 早期 / v0.x | 没掏 $99 Apple Developer 账号 | 跑 codesign ad-hoc 重签 + 剥 com.apple.provenance（上面步 4）|
| **v1.0+** | 已公证 | 跳过步 4——公证覆盖所有嵌入二进制；但 `.app` 必须配 `disable-library-validation` entitlement 让运行时下载的 wheel `.so` 也能 dlopen |

### 4.4 Sync 内部

```go
func (s *Sandbox) Sync(ctx context.Context, req SyncRequest) error {
    unlock := s.syncMu.Lock(req.ForgeID)  // per-forge 串行
    defer unlock()
    
    envDir := filepath.Join(s.cfg.DataDir, "forges", req.ForgeID, "envs", req.EnvID)
    
    // 已存在 → 跳过（简单 stat 判断；半成品 venv 让 uv run 时自然报错给 LLM 自救）
    if _, err := os.Stat(envDir + "/.venv"); err == nil {
        return nil
    }
    
    os.MkdirAll(envDir, 0700)
    
    // 写 pyproject.toml（atomic rename 防半成品）
    writeAtomic(envDir+"/pyproject.toml", renderPyproject(req))
    
    cmd := exec.CommandContext(ctx, s.cfg.UVPath,
        "sync",
        "--project", envDir,
        "--python", s.cfg.PythonPath,
        "--no-progress",
    )
    cmd.Env = withUVEnv(s.cfg.DataDir, s.cfg.PythonPath)
    
    // stderr 双路：进度行调 OnProgress；无法识别的行（错误链 / 警告 / 散文）
    // 收集进 errBuf。成功时 errBuf 丢弃；失败时连同 cmd error 一并返回，
    // 让上层 forgeapp 把内容塞进 forge_versions.env_error 字段——LLM 能看到
    // 真实失败原因（如"numpy>=2.0 conflicts with python<3.12"），调 edit_forge
    // 改 deps 自救。
    //
    // stderr 双路：进度行 → OnProgress；非进度行 → errBuf。失败时 errBuf 透传
    // 到 EnvError，让 LLM 看到具体错误自救。
    stderrPipe, _ := cmd.StderrPipe()
    var errBuf bytes.Buffer
    go scanProgress(stderrPipe, req.OnProgress, &errBuf)
    
    if err := cmd.Run(); err != nil {
        return &SyncError{Cause: err, Stderr: errBuf.String()}
    }
    return nil
}

// SyncError 包装 uv sync 失败 + 完整 stderr 文本。
// forgeapp.SyncEnvForVersion 收到后写入 ForgeVersion.EnvError。
type SyncError struct {
    Cause  error
    Stderr string
}

func (e *SyncError) Error() string { return e.Stderr }  // 整段 stderr 当 message
func (e *SyncError) Unwrap() error { return e.Cause }
```

### 4.5 Progress 解析

`progress.go::parseUVLine` 把 uv stderr 行识别成 `(stage, detail)`，**uv 真实输出三大阶段总结行**（按上游 `uv sync` 行为）：

```
"Resolved 12 packages in 1.5s"     → ("resolving",  "Resolved 12 packages in 1.5s")
"Prepared 12 packages in 800ms"    → ("preparing",  "Prepared 12 packages in 800ms")
"Installed 12 packages in 200ms"   → ("installing", "Installed 12 packages in 200ms")
```

下载发生在 `Prepared` 阶段内部——大型包可能有 sub-progress 行（"Downloading numpy"），但总结行就这三个。

```go
func scanProgress(r io.Reader, onProgress func(stage, detail string), errBuf *bytes.Buffer) {
    scanner := bufio.NewScanner(r)
    for scanner.Scan() {
        line := scanner.Text()
        if u := parseUVLine(line); u != nil && onProgress != nil {
            onProgress(u.Stage, u.Detail)
            continue
        }
        // 无法识别 → 不调 OnProgress，但收集到 errBuf。
        // 成功路径：errBuf 内容 sync 完丢弃。
        // 失败路径：errBuf 内容塞进 EnvError 给 LLM 看错误自救。
        errBuf.WriteString(line)
        errBuf.WriteByte('\n')
    }
}
```

### 4.6 Run 内部

```go
func (s *Sandbox) Run(ctx context.Context, req RunRequest) (*forgedomain.ExecutionResult, error) {
    envDir := filepath.Join(s.cfg.DataDir, "forges", req.ForgeID, "envs", req.EnvID)
    versionDir := filepath.Join(s.cfg.DataDir, "forges", req.ForgeID, "versions", req.VersionID)
    
    os.MkdirAll(versionDir, 0700)
    fullCode := req.Code + buildDriver(extractFuncName(req.Code))
    writeAtomic(versionDir+"/main.py", fullCode)
    
    cmd := exec.CommandContext(ctx, s.cfg.UVPath,
        "run",
        "--project", envDir,
        "--no-sync",  // 关键：跳过 lock 检查
        "python", versionDir+"/main.py",
    )
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}  // 进程组便于 kill
    
    inputJSON, _ := json.Marshal(req.Input)
    cmd.Stdin = strings.NewReader(string(inputJSON))
    
    start := time.Now()
    var stderr bytes.Buffer
    cmd.Stderr = &stderr
    stdout, runErr := cmd.Output()
    elapsed := time.Since(start).Milliseconds()
    
    // 失败时 stderr 透传到 ExecutionResult.ErrorMsg → tool_result。
    // 涵盖：venv 不存在（被 N=3 缓冲清掉了）/ Python 抛异常 / ctx 取消 等。
    // **不在 sandbox 层做"venv 不存在 → 转 evicted → 触发 sync"自愈**——uv 自然
    // 报错给 LLM，LLM 看到 "No virtual environment found" 这种错误，自决调
    // edit_forge / :resync 自救。MVP 哲学：punt 给 AI，不在 backend 写恢复
    // 逻辑（详 §11）。
    return parseExecutionResult(stdout, stderr.String(), runErr, elapsed), nil
}
```

**无 timeout**——只靠 ctx-cancel 终止子进程；windows 用 Job Object（详 §12 风险表）。

---

## 5. SSE 推送（Phase 6 entity-state 模型）

### 5.1 不引入新事件类

所有 sync 进度通过现有 `forge` 事件推送：

```
事件名：forge
载荷：完整 Forge entity（含 .Pending 子对象 + 计算字段 EnvStatus / EnvSyncStage / ...）
形状：跟 GET /api/v1/forges/{id} 一致
```

### 5.2 触发点表（既有 + 本迭代新增）

| # | 触发动作 | 来源 |
|---|---|---|
| 1 | CRUD（Create / PATCH / Delete / `:revert`） | Phase 6 已有 |
| 2 | Pending 生命周期（创建 / accept / reject） | Phase 6 已有 |
| 3 | create_forge 流：预 stub + 逐 token + 末尾定型 | Phase 6 已有 |
| 4 | edit_forge 代码流：预 draft + 逐 token + 末尾定型 | Phase 6 已有 |
| 5 | edit_forge 仅元数据：单帧最终快照 | Phase 6 已有 |
| 6 | **EnvStatus 状态转换**：pending→syncing→ready/failed/evicted | **本迭代新增** |
| 7 | **每行 uv stderr 解析**：EnvSyncStage / EnvSyncDetail 变化 | **本迭代新增** |

### 5.3 唯一发布事实源

forgeapp.Service 内部一个 helper（按 chat.runner.publishMessageSnapshot 同模式）：

```go
// app/forge/forge.go
func (s *Service) publishForgeSnapshot(ctx context.Context, forgeID string) {
    f, err := s.repo.GetForge(ctx, forgeID)
    if err != nil { return }
    s.attachPending(f)
    s.attachActiveEnv(f)
    s.bridge.Publish(ctx, "", events.Forge{Forge: f})
}
```

sandbox.OnProgress callback 由 forgeapp 实现，调用 `publishForgeSnapshot`——sandbox 自己不直接调 bridge。

### 5.4 装包过程的实际推送

装 pandas+numpy 大约 6 帧 forge 完整快照：

```
t=0    forge { pending: {envStatus:"syncing", envSyncStage:""} }
t=1.5s forge { pending: {envSyncStage:"resolving",  envSyncDetail:"Resolved 12 packages in 1.5s"} }
t=5.4s forge { pending: {envSyncStage:"preparing",  envSyncDetail:"Prepared 12 packages in 800ms"} }
t=5.6s forge { pending: {envSyncStage:"installing", envSyncDetail:"Installed 12 packages in 200ms"} }
t=8s   forge { pending: {envStatus:"ready", envSyncedAt:"...", envSyncStage:"", envSyncDetail:""} }
```

每帧都是 Forge 完整快照——前端按 forge.id 替换本地拷贝即可，不用追 delta。

---

## 6. EnvStatus 状态机

```
                     [forge create / dep change]
                              │
                              ▼
                       ┌─────────┐
                       │ pending │
                       └────┬────┘
                            │
                            ▼
                       ┌─────────┐
                       │ syncing │ ←─────────────────┐
                       └────┬────┘                   │
                            │                        │
                  ┌─────────┴─────────┐              │
                  ▼                   ▼              │
              ┌───────┐           ┌────────┐         │
              │ ready │           │ failed │         │
              └───┬───┘           └────┬───┘         │
                  │                    │             │
            [N=3 缓冲超出]      [edit 改 deps 重试]   │
                  │                    │             │
                  ▼                    └─────────────┘
            ┌─────────┐
            │ evicted │ ──[revert / run 触发懒重建]──► syncing
            └─────────┘
```

DB CHECK 约束（按 §D3 走 schema_extras）：

```sql
CHECK (env_status IN ('pending','syncing','ready','failed','evicted'))
```

---

## 7. 跟现行架构对接

| 现行约束 | 本迭代如何遵守 |
|---|---|
| Phase 6 entity-state 模型（3 个事件，载荷 = REST GET 形状） | 不引入新事件，沿用 `forge` 事件；扩 entity 字段自然扩载荷 |
| Phase 6 chat.runner 是 chat.message 唯一发布事实源 | 同模式：forgeapp.publishForgeSnapshot 是 forge 事件唯一发布事实源；sandbox 不直接调 bridge |
| Phase 5 ForgeExecution 合并 run/test history | 不变；run_forge 仍写 forge_executions（含 chat 上下文） |
| 现状 create_forge 直接落 accepted v1 | **本迭代改**：create_forge 进 pending → user accept 才 v1（跟 edit_forge 走统一审核入口） |
| §S18 Tool 接口 destructive per-call AI 自报 | 不在 forge entity 加静态字段；保留既有机制 |
| §S9 detached context 终态写模式 | 不需要——sync 是同步链路，ctx 来自 LLM tool call |
| 30s SandboxTimeout 常量 | 删除；run / sync 都不设硬限，只靠 ctx-cancel |

---

## 8. 错误模型

```go
// domain/forge/forge.go 新增 sentinel：
var (
    ErrEnvNotReady          = errors.New("forge: env not ready")
    ErrEnvFailed            = errors.New("forge: env failed")
    ErrSandboxUnavailable   = errors.New("forge: sandbox unavailable")
    ErrDependencyResolution = errors.New("forge: dependency resolution failed")
)
```

errmap 新增 4 行（按 §S17）：

| Code | HTTP | Sentinel |
|---|---|---|
| `FORGE_ENV_NOT_READY` | 422 | `ErrEnvNotReady` |
| `FORGE_ENV_FAILED` | 422 | `ErrEnvFailed` |
| `FORGE_SANDBOX_UNAVAILABLE` | 503 | `ErrSandboxUnavailable` |
| `FORGE_DEPENDENCY_RESOLUTION` | 422 | `ErrDependencyResolution` |

Run 时 ctx-cancel → 走通用 `context.Canceled`，不需要 forge 专门 sentinel。

---

## 9. Phase 划分（~4 天独立交付，不阻塞 Phase 4）

### Phase A：sandbox 内部（~1.5 天）

- [ ] 6 个文件骨架（按 §4.1）
- [ ] Bootstrap：embed.FS / dev resources 双路径解压 + 跨平台路径处理
- [ ] Sync / Run / WriteCodeFile / Destroy / DestroyEnv 实现
- [ ] progress.go uv stderr 行解析 + 单测
- [ ] paths.go ComputeEnvID + normalizeSpecifier + 单测
- [ ] 集成测试（FORGIFY_TEST_UV + FORGIFY_TEST_PYTHON 环境门控，按 §T3）

### Phase B：domain + service 扩展（~1 天）

- [ ] `domain/forge`：
  - `Forge` 加 `ActiveVersionID` 列 + 4 个计算字段
  - `ForgeVersion` 加 `Dependencies / PythonVersion / EnvID + 5 env 字段`
  - 4 个新 sentinel + EnvStatus 5 值常量
- [ ] `infra/db/schema_extras` forge_versions 加 `CHECK(env_status IN ...)`
- [ ] `infra/store/forge` 适配新字段；加 `(forge_id, env_id)` 复合索引
- [ ] `app/forge`：
  - `Sandbox` 接口（5 方法）
  - `SyncEnvForVersion(ctx, versionID)` — 同步包装
  - `attachActiveEnv(forge)` — 计算字段填充
  - `publishForgeSnapshot(ctx, forgeID)` — 唯一发布点
  - `Create / Update / AcceptPending / RevertToVersion / Delete` 接 sync 触发 + EnvID 计算
  - `trimEnvBuffer` — N=3 EnvID 缓冲清理
- [ ] `app/forge/ast.go` 改成 `ASTParser` 类型接收 pythonPath（用捆绑 Python 替代系统 python3）

### Phase C：tool 层 + HTTP（~半天）

- [ ] `app/tool/forge/create.go`：流程改成进 pending（不直接 accepted v1）
- [ ] `app/tool/forge/edit.go`：草稿期 / 激活期统一入口（找 pending 或基于 active 复制）
- [ ] LLM-facing schema 加 `dependencies` 字段（PEP 508 string array，optional）
- [ ] prompt 加"声明 non-stdlib import 进 dependencies"指引
- [ ] 错误码 + errmap 4 行
- [ ] `forgehandler` `:revert` 端点接 evicted 重建路径
- [ ] AcceptPending 校验 EnvStatus="ready" 才允许 accept

### Phase D：装配（~半小时）

- [ ] `cmd/server/main.go` Bootstrap + Sandbox 注入
- [ ] dev 模式从 `FORGIFY_DEV_RESOURCES` 拷资源
- [ ] `backend/cmd/resources/main.go` 一次性下 uv + python-build-standalone（`cd backend && go run ./cmd/resources`）

### Phase E：文档同步（~1 天，按 §S14）

见 §10。

### Phase F：testend UI（~半天，并行）

- [ ] forge 详情页 envStatus 徽章 + 进度区（按 forge.envSyncStage / Detail 渲染）
- [ ] resync 按钮（清 evicted / 修复损坏）
- [ ] forge entity-state 事件接进现有 sse 视图

---

## 10. 文档同步清单（§S14）

### 必改

- [ ] `service-design-documents/forge.md`
  - §3.1 `Forge` 加 `ActiveVersionID` + 4 计算字段
  - §3.2 `ForgeVersion` 加 `Dependencies / PythonVersion / EnvID` + 5 env 字段
  - §3.4 ForgeExecution 不变
  - §4 常量加 EnvStatus 5 值 + N=3 缓冲常量 + DefaultPythonVersion
  - §5 sentinel 加 4 个
  - §6 Repository 接口加 `UpdateVersionEnvStatus / UpdateVersionEnvProgress / UpdateVersionEnvID / ListEnvIDsForForge` 等方法
  - §8 Service 加 `SyncEnvForVersion / publishForgeSnapshot / attachActiveEnv / trimEnvBuffer`
  - §10 system tool：create_forge 改进 pending；edit_forge 草稿/激活统一
  - §11 HTTP API 加 `:revert` 路径
  - §12 错误码加 4 行
  - §13 SSE 触发点表加 #6 / #7
  - §14 调用链全量改写（按本文档 §3 五条链）
  - §16 sandbox 章节大改（PythonSandbox → Sandbox + 接口 5 方法）

- [ ] `service-contract-documents/database-design.md`
  - forges 表加 active_version_id
  - forge_versions 表加 8 字段（deps / python_version / env_id / 5 env 字段）+ env_status CHECK + (forge_id, env_id) 复合索引

- [ ] `service-contract-documents/error-codes.md`
  - 加 4 行 FORGE_ENV_* / FORGE_SANDBOX_*

- [ ] `service-contract-documents/events-design.md`
  - `forge` 事件触发点表加 #6 / #7 两行（不改事件名，不加事件类型）

- [ ] `service-contract-documents/api-design.md`
  - create_forge / edit_forge args 加 `dependencies` 字段说明

- [ ] `progress-record.md`
  - dev log 加本迭代条目

- [ ] `desktop-packaging-notes.md` §五
  - 方案表：本迭代落定 C+B 混合（uv 管 venv，捆绑 python-build-standalone）

- [ ] `CLAUDE.md` 项目特殊性段
  - "infra/sandbox 用 subprocess 跑 Python" → "infra/sandbox 捆绑 uv + Python，每 EnvID 独立 venv"

---

## 11. 不做的事（明确划界）

### 11.1 MVP 哲学：punt 给 AI 自救

LLM agent 系统跟传统 backend 不一样的核心红利：**很多边界 case 可以让 AI 看错自救**。传统 backend 想方设法防的事，在 agent loop 里报错回去就行——LLM 调 edit_forge / `:resync` 就能自愈。

所以**砍掉一票"自动恢复"机制**，前提条件是错误信息能可靠传到 LLM（详 §4.4 Sync 的 errBuf 收集 + §4.6 Run 的 stderr 透传）：

| 砍掉的"自动修复"机制 | 自然怎么处理 |
|---|---|
| 启动期 reconcile `EnvStatus="syncing"` 残留 | LLM `get_forge` 看到状态卡住，调 `:resync` 端点；前端 UI 也给个"重新装"按钮 |
| venv 完整性严密校验 | `stat .venv` 简单判断够 99%；半成品让 uv run 自然报错→LLM 看错误自救 |
| Run 时 evicted 自检 + eager 状态同步 | uv run 找不到 venv 直接报错→透传到 tool_result→LLM 调 `:resync` |
| 孤儿 venv 目录定期 GC | 不做；磁盘多占点而已，后续 feature |
| 进程退出前清半成品文件 | 不做；下次 sync uv 会自己处理已存在文件 |

**只保留两个真必须的兜底**：
- mac codesign（不修就内核 SIGKILL 无日志，LLM 也救不了）
- 错误信息收集到 EnvError（不收集 LLM 看不到错就没法救自己）

这与设计原则 #6 "反校验剧场" 一脉相承——不预先防 LLM/用户能自然修复的事。

### 11.2 不做的具体清单

- ❌ **异步 sync worker**：同步等更简单，create/edit 等 sync 完是合理 LLM tool call 时长
- ❌ **Run timeout**：工具可能合理跑很久，死循环是 LLM/用户问题
- ❌ **安全隔离**（filesystem / network / cgroups）：本地单用户单作者
- ❌ **Forge 静态 Destructive / IsReadOnly / IsConcurrencySafe 字段**：destructive 走 §S18 既有 per-call AI 自报模式
- ❌ **多 Python 版本并存**：仅锁一个 3.12.x
- ❌ **Pyodide / WASM 路线**
- ❌ **Pre-warmed Python 进程池**：启动 ~50ms 可接受
- ❌ **自动 venv 过期重新解析**：要新版本就改 specifier，显式表达意图
- ❌ **新 SSE 事件类**：复用现有 forge entity-state 事件
- ❌ **PEP 440 等价 specifier 语义合并**（如 `pandas>=2.0` vs `pandas>=2.0.0`）：不强求；多一份 venv 几 MB metadata 可接受
- ❌ **重启状态 reconcile / venv 完整性校验 / evicted 自愈 / 孤儿 GC**（见 §11.1）：punt 给 AI 自救

---

## 12. 风险 / 未决项

| 风险 | 影响 | 缓解 |
|---|---|---|
| python-build-standalone 在 mac quarantine 触发 Gatekeeper | 中-高 | preflight 跑 xattr -dr；做 mac 公证流水线时实测 |
| uv 跨 minor 版本破坏行为 | 中 | 锁 minor 版本，升级走集成回归 |
| Run 时 ctx-cancel 未杀干净子孙进程 | 中 | process group + 跨平台测试覆盖（mac/linux Setpgid + Kill -pgid；win taskkill /T /F） |
| 同 forge 并发 Run 写同一 main.py 文件 | 低 | atomic rename `main.py.tmp` → `main.py` |
| 大型依赖（torch、playwright + browsers）单次 sync 1+ 分钟 | 中 | EnvSyncStage / EnvSyncDetail 进度推送让用户看到在装啥 |
| 同 EnvID 共用 venv 但用户期待"新装"获取最新版本 | 中 | 文档明确：要新版本就改 specifier；提供"删 EnvID 强制重建"端点（v2，本迭代不做） |

未决（写代码前要确认）：

- [ ] mac 上 uv 装的 python-build-standalone 解释器实际 quarantine 行为
- [ ] uv 0.5.x 在 windows 子进程信号处理（taskkill /T /F vs Cmd.Cancel）
- [ ] python-build-standalone 跨平台目录结构差异（mac/linux 有 `bin/`，win 直接根目录 `python.exe`）

---

## 一句话总结

把 sandbox 从"调系统 python3 跑临时文件"升级为**自带 Python + uv 管 venv + 每 EnvID 独立环境**：venv 按依赖集 hash 命名（不是按 version），同 deps 的多版本零代价共享；create/edit 同步等 sync 完成；ForgeVersion 持有依赖配置 + 环境状态；forge.ActiveVersionID 指当前活跃版本；sync 进度通过现有 `forge` entity-state 事件推送。不引入新事件类、不引入异步 worker、不设 timeout。
