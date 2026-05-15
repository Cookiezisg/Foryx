# Sandbox — V1.2 详设计（PluginSandbox v2 统一架构）

**Phase**：Phase 4 准备件（提前到位，与 mcp/skill/forge 整合）
**状态**：✅ Marketplace V3 collapse（2026-05-08）：仅 Python + Node 2 EnvManagers + 3 RuntimeInstaller（python / node / uv via mise）；原 11 EnvManager 矩阵 + Docker / Playwright / Dotnet / Static / Generic / Rust / Go / Java / Ruby / PHP installer 全部删除（无消费方）。Layer A/B leak prevention：Service.Shutdown / RestoreOrCleanupOnBoot + Env.RunningPID manifest 追踪 spawn 子进程。`sandbox_env` per-env notification on every state transition。
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/database-design.md`](../service-contract-documents/database-design.md) — `sandbox_runtimes` + `sandbox_envs` 两表（含 running_pid 列）
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — sandbox ×11（已接 errmap）
- [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) — `sandbox_env` per-env entity-state 通知 + install 进度通过 ctx eventlog Emitter 推 progress block 到调用方 tool_call 父下（详 §10）
- 关联设计：[`forge.md`](./forge.md)（forge sandbox 是 first consumer）/ [`mcp.md`](./mcp.md)（MCP server install 走本服务）/ [`skill.md`](./skill.md)（未来 skill 带 deps 时复用）

---

## 1. 一句话

**统一的 PluginRuntime 抽象**——给 forge / mcp / skill / **每个对话**（agent scratch）/ 未来任何 plugin 提供"安装 runtime → 建独立 env → 装 deps → spawn 子进程"的一站式服务。**Bootstrap 仅 mise binary（~25 MB go:embed 进 binary）**，**Python + Node** runtime + uv 工具 lazy install 到 `<dataDir>/sandbox/runtimes/`，每个 plugin 实例的依赖隔离到 `<dataDir>/sandbox/envs/`。

**V3 collapse**（2026-05-08）：原 11 EnvManager 矩阵（Rust / Go / Java / Ruby / PHP / .NET / Playwright / Docker / Generic / Static）已全部删除——marketplace V3 curated 21 条只用 npm + pypi，forge/skill/conversation 也只跑 Python/Node 脚本。详 §4。

**Bash 自动路由**：LLM 通过 Bash tool 跑 `pip install pandas` / `python script.py` / `npm install` 等命令时，sandbox 检测命令意图，**自动路由到该对话的 scratch env**——LLM 完全不知道沙箱存在，但所有动作都被收口（不污染用户系统、不跨对话扩散）。详见 §9.5。

---

## 2. 端到端推演

### 启动期

```
main.go → sandboxapp.NewService(deps).Start(ctx)
  → 加载 SQLite manifest 表（sandbox_runtimes + sandbox_envs）
  → 检测 mise binary 是否在 <dataDir>/sandbox/bin/mise
      ✅ 在 → 继续
      ❌ 缺 → 从 go:embed 解出当前平台 mise binary 到 <dataDir>/sandbox/bin/，chmod +x，darwin 跑 codesign
  → 跑 RestoreOrCleanupOnBoot（Layer B 泄漏防护）：
      → 扫所有 envs.RunningPID != 0 的行
      → 该 PID 还活着且 cmdline 含 forgify env 路径 → 重新 attach 到 activeHandles
      → 否则清掉 RunningPID（前次进程已死，遗留状态）
  → 启 mise 全局配置写入（disable 所有 attestation backend）
  → 准备好接 plugin 调用
```

**注意：启动时不预装任何 runtime 或 env**——什么都不做，等第一个 plugin 来要。

### 运行期 — 用户装 Node-类 MCP server（如 github）

```
mcpapp.InstallFromRegistry(ctx, "github")
  ↓
sandboxapp.EnsureEnv(ctx, Owner{Kind:"mcp", ID:"github"}, EnvSpec{
  Runtime: RuntimeSpec{Kind:"node", Version:""},   // "" = 用 default
  Deps:    []string{"@modelcontextprotocol/server-github"},
})
  ↓
Step 1: EnsureRuntime("node", "")
  → 查 sandbox_runtimes 表：node 类是否有 default 行
  → 没有 → 跑 mise install node@22  → 等 ~50 MB 下载
  → 写一行 sandbox_runtimes：kind=node, version=22.x.y, path=mise-data/installs/node/22.x.y, isDefault=true
  → 通过 ctx eventlog Emitter 推 progress block delta（详 §10）
  ↓
Step 2: 创建 envs/mcp/github/
  → 写 package.json
  → 跑 npm install --prefix=envs/mcp/github @modelcontextprotocol/server-github
  → 通过 ctx eventlog Emitter 推 progress delta
  ↓
Step 3: 写一行 sandbox_envs：
  ownerKind=mcp, ownerID=github, runtimeID=<node row id>,
  deps=["@modelcontextprotocol/server-github"], path=mcp/github,
  sizeBytes=..., status=ready
  ↓ publish notification {type:"sandbox_env", id:"se_xxx", data:{status:"ready", ...}}
  ↓
Step 4: 返 *Env 给 mcpapp.Service
  → mcpapp 拿 envID 调 SpawnLongLived 起 MCP server 子进程
```

> Python-类 server（如 sentry / figma）流程相同，把 RuntimeSpec.Kind 改 `"python"`，`uv pip install <pkg>` 替代 `npm install`。

### 运行期 — Forge 跑代码（一次性 spawn）

```
forgeapp.RunForge(ctx, forgeID, input)
  ↓
sandboxapp.Spawn(ctx, Owner{Kind:"forge", ID:forgeVersionEnvID}, SpawnOpts{
  Cmd: "python", Args: []string{"-c", "<forge code>"},
  Stdin: input, Timeout: 30s,
})
  ↓
Service 查 sandbox_envs 找匹配 env（已存在）
  → 找到 path=envs/forge/<env-id>
  → exec.CommandContext 用 envs/forge/<env-id>/.venv/bin/python 跑
  → 取 stdout/stderr/exitCode
  → 更新 last_used_at（GC 用）
  → 返 ExecutionResult
```

### 运行期 — MCP server 长生命周期 spawn

```
mcpapp.Connect(ctx, "github")
  ↓
sandboxapp.SpawnLongLived(ctx, Owner{Kind:"mcp", ID:"github"}, SpawnOpts{
  Cmd: "npx",
  Args: []string{"-y", "@modelcontextprotocol/server-github"},
  Env: map[string]string{
    "GITHUB_PERSONAL_ACCESS_TOKEN": "ghp_...",
  },
})
  ↓
Service 解析 npx 路径到 envs/mcp/github/.runtime/node/bin/npx
  → 写 RunningPID 到 sandbox_envs（Layer B leak prevention）
  → trackedHandle 加进 activeHandles map（Layer A graceful shutdown 用）
  → 返 LongLivedHandle{Stdin, Stdout, Stderr, Wait, Kill, PID}
  → mcpapp 用这个 handle 走 JSON-RPC over stdio
  ↓
进程退出（正常 / kill / crash）→ Wait goroutine 触发：
  → 清 RunningPID = 0
  → 从 activeHandles 移除
  → 错误时 publish sandbox_env 通知 status="failed"
```

---

## 3. 设计原则

| 原则 | 落地 |
|---|---|
| **统一 PluginRuntime 抽象** | forge / mcp / skill / conversation scratch / 未来任何 plugin 类型走同一套 EnsureEnv / Spawn / Destroy 接口 |
| **对话 scratch env + Bash 自动路由** | 每对话按需起独立 scratch env（per runtime kind）；Bash tool 检测命令路由到对应 conversation env；LLM 自由 install/run，**所有动作自动收口到沙箱**——不靠 denylist 靠基础设施 |
| **Bootstrap 极小** | bundled 仅 mise（~10 MB）+ 几个 helper 脚本；其他 lazy 装 |
| **共享 runtime + 隔离 env** | 一个 Python 解释器服务所有 Python 类 plugin（venv 隔离 deps）；Node/Rust/Java 同理 |
| **Default 单版本，例外多版本** | 默认 Python 3.12 / Node 22 / etc.；plugin 显式 version pin 才装额外版本 |
| **代码层全语言覆盖** | RuntimeInstaller 接口 open/closed——v1 ship mise 一个 installer 通配大部分语言；新加语言 = 1 行注册 |
| **Lazy install + 按需缓存** | 第一次用某 runtime 才下载；安装好留在磁盘下次复用 |
| **SQLite manifest** | 所有 runtime/env 元数据走 sandbox_runtimes + sandbox_envs 两表，不另起 JSON 文件 |
| **Per-plugin env 隔离** | 各 plugin 自己的 venv/node_modules/etc.，不同版本 deps 不冲突 |
| **强制本地化** | 所有包管理器调用都带 `--prefix` / `BUNDLE_PATH` / etc.，不允许装到全局 |
| **跨 env 共享靠包管理器原生机制** | uv hardlink wheel cache / pnpm content-addressable store / Maven `~/.m2` / Cargo registry——多 conv 装同包磁盘 ≈ 装一次。**venv 数量 ≠ 磁盘占用** |
| **GC 默认手动** | sandbox_envs.last_used_at 用于 UI 显示和手动清理；**v1 不开 auto-GC**（共享机制让磁盘开销极小）；plugin 卸载和软删→硬删时按需触发 Destroy |
| **Owner 反向追溯** | 每 env 记 ownerKind + ownerID，"Python 3.12 被谁用着 / 这 forge 占多少磁盘"一查即得 |

---

## 4. 覆盖语言矩阵（V3 — 仅 npm + pypi）

Marketplace V3（curated 21）+ forge / skill / conversation 4 类 owner 都只用 **Python + Node**——故 sandbox 现在仅注册这两条 runtime。原 11 EnvManager 矩阵（Rust / Go / Java / Ruby / PHP / .NET / Playwright / Docker / Generic / Static）已删除（2026-05-08）。

| 语言 | 默认版本 | mise 装机 | Env 隔离 |
|---|---|---|---|
| **Python** | 3.12.x | python-build-standalone（mise 内置）| `uv venv` + `uv pip install`（uv 也由 mise 装，pin 0.11.4）|
| **Node.js** | 22.x（LTS）| nodejs.org tarball（mise 装 node）| `npm install --prefix=<env_path>` |

**为什么砍**：先前为"未来扩展"预留的 7 个 EnvManager 没有真实消费者——MCP marketplace 现在 curated 化只剩 npm/pypi，forge/skill/conversation 也只跑 Python/Node 脚本。删掉 ~700 行无 caller 代码 + ~200MB 不会被装的语言 runtime，遵守 §S20"留下次"= bug。

未来真要加 Rust/Go MCP server 时再恢复对应 EnvManager（git history 还在），不预先架空中楼阁。

### Python 的"二级火箭"

mise 装的 Python 自带 pip——慢。所以 sandbox 在 Python runtime 装好后**自动再装一次 uv**：

```
1. mise install python@3.12          → ~30s（下解释器）
2. mise install uv@latest            → ~5s（下 uv binary）
3. 之后所有 Python env 用 uv 操作   → uv pip install / uv venv 都比 pip/venv 快 10x
```

**Python uv 也存为 sandbox_runtimes 一行**（kind="python-tool", version="uv@x.y.z"），便于 manifest 追溯。

---

## 5. 领域模型

### Runtime（持久化，`internal/domain/sandbox/sandbox.go`）

```go
type Runtime struct {
    ID          string         `gorm:"primaryKey;type:text" json:"id"`               // sr_<16hex>
    Kind        string         `gorm:"not null;type:text" json:"kind"`               // "python" / "node" / "uv" / 任何 mise 支持的
    Version     string         `gorm:"not null;type:text" json:"version"`            // "3.12.5" / "22.5.0" / "stable"
    Path        string         `gorm:"not null;type:text" json:"path"`               // 相对 ~/.forgify/sandbox/runtimes/，如 "python/3.12.5"
    SizeBytes   int64          `json:"sizeBytes"`
    InstalledAt time.Time      `json:"installedAt"`
    UpdatedAt   time.Time      `json:"updatedAt"`
}

func (Runtime) TableName() string { return "sandbox_runtimes" }
```

复合 UNIQUE：`(kind, version)`——同 kind 下版本号唯一。
默认版本由 `RuntimeInstaller.ResolveDefault` 拥有（构造时固化的常量）；无 `is_default` 列 / `FindDefaultRuntime` 查询。

### Env（持久化，同文件）

```go
type Env struct {
    ID                string     `gorm:"primaryKey;type:text" json:"id"`              // se_<16hex>
    OwnerKind         string     `gorm:"not null;type:text;index:idx_se_owner,priority:1" json:"ownerKind"` // "forge" / "mcp" / "skill" / "conversation"
    OwnerID           string     `gorm:"not null;type:text;index:idx_se_owner,priority:2" json:"ownerID"`   // f_xxx / "github" / etc.（**用 `_` 不用 `:`**，详 §5）
    OwnerName         string     `gorm:"type:text" json:"ownerName,omitempty"`        // UI display
    RuntimeID         string     `gorm:"not null;type:text;index" json:"runtimeId"`   // FK → sandbox_runtimes.id
    Deps              []string   `gorm:"serializer:json" json:"deps"`                 // ["pandas==2.0", "numpy"]
    Path              string     `gorm:"not null;type:text" json:"path"`              // 相对 envs/，如 "mcp/github"
    SizeBytes         int64      `json:"sizeBytes"`
    Status            string     `gorm:"not null;type:text;default:ready;check:status IN ('installing','ready','failed','destroyed')" json:"status"`
    ErrorMsg          string     `gorm:"type:text" json:"errorMsg,omitempty"`
    // Layer B leak prevention（详 §8 Service.RestoreOrCleanupOnBoot）
    RunningPID        int        `gorm:"column:running_pid;default:0;index" json:"runningPid,omitempty"`  // 当前 spawn 出去的子进程 PID；0 = 未在 spawn
    CreatedAt         time.Time  `json:"createdAt"`
    LastUsedAt        time.Time  `gorm:"index" json:"lastUsedAt"`                     // GC 用
    UpdatedAt         time.Time  `json:"updatedAt"`
}

func (Env) TableName() string { return "sandbox_envs" }
```

复合 UNIQUE：`(owner_kind, owner_id)`——一个 plugin 实例同时只能有一份 env（避免重复）。
复合索引 `idx_se_owner`：按 owner 查"我的 env 是哪份"。
索引 `runtime_id`：runtime GC 时反查"还有人用吗"。
索引 `last_used_at`：按时间 GC。
索引 `running_pid`：boot scan 时按 `WHERE running_pid != 0` 过滤"上次还在 spawn 的 envs"。

CHECK 约束：`status IN ('installing','ready','failed','destroyed')`。

### Owner / Spec / Handle 等支持类型

```go
type Owner struct {
    Kind string  // "forge" / "mcp" / "skill" / "conversation" / 未来扩展
    ID   string  // 唯一标识，按 Kind 不同（**禁 `:` 等系统 PATH 元字符**，违反返 ErrInvalidOwnerID）：
                 //   forge:        EnvID（同 deps 多 forge 版本共享）
                 //   mcp:          server 名（如 "github"）
                 //   skill:        skill 名
                 //   conversation: "<conv_id>_<runtime_kind>"，如 "cv_abc_python"（**用 `_` 分隔，不用 `:`**）
    Name string  // UI display 用
}

type RuntimeSpec struct {
    Kind    string  // "python" / "node" / "rust" / "java" / 任何 mise 支持的
    Version string  // "" = default；">=3.10" / "==3.10.5" / "stable" / etc.
}

type EnvSpec struct {
    Runtime  RuntimeSpec
    Deps     []string  // 包名（按 runtime 语言习俗：pip / npm / cargo / etc.）
}

type SpawnOpts struct {
    Cmd       string            // 可执行名（"python" / "node" / 绝对路径）
    Args      []string
    Env       map[string]string // 额外环境变量
    Stdin     []byte            // 一次性 spawn 用
    Timeout   time.Duration     // 一次性 spawn 用；0 = 长生命周期
    LongLived bool              // true → 返 LongLivedHandle 不取 stdout
}

type ExecutionResult struct {
    Ok       bool
    Stdout   []byte
    Stderr   []byte
    ExitCode int
    Duration time.Duration
}

type LongLivedHandle interface {
    Stdin() io.WriteCloser
    Stdout() io.ReadCloser
    Stderr() io.ReadCloser
    Wait() error
    Kill() error
    PID() int
}

type ProgressFunc func(stage, message string, percent int)
```

### Sentinel 错误（11 个，V3 后）

```go
var (
    ErrRuntimeNotSupported  = errors.New("sandbox: runtime kind not registered")
    ErrRuntimeInstallFailed = errors.New("sandbox: runtime install failed")
    ErrEnvNotFound          = errors.New("sandbox: env not found")
    ErrEnvCreateFailed      = errors.New("sandbox: env create failed")
    ErrDepInstallFailed     = errors.New("sandbox: dependency install failed")
    ErrSpawnFailed          = errors.New("sandbox: spawn process failed")
    ErrSpawnTimeout         = errors.New("sandbox: spawn process timeout")
    ErrEnvInUse             = errors.New("sandbox: env in use; cannot destroy")
    // Owner.ID 校验（PATH-meta 字符守卫——禁 `:` 等会被系统 PATH 拼接咬到的字符）
    ErrInvalidOwnerID       = errors.New("sandbox: invalid owner id")
    // SpawnOpts.Cmd 必填守卫
    ErrCmdRequired          = errors.New("sandbox: cmd required for spawn")
    // Bootstrap 失败 → degraded mode 入口返此（详 §15.1）
    ErrSandboxUnavailable   = errors.New("sandbox: bootstrap not ready")
)
```

**已删（V3 collapse）**：`ErrDockerNotInstalled` / `ErrDockerDaemonDown` 与 Docker runtime 同期移除。

---

## 5b. Docker runtime — 已删除（V3，2026-05-08）

> Marketplace V3 collapse 时把 Docker runtime 全部移除（DockerInstaller / DockerEnvManager / BuildDockerRunArgs helper / `ErrDockerNotInstalled` / `ErrDockerDaemonDown`）——curated 21 条全部 npm + pypi，无 docker package。
>
> 原设计：MCP marketplace V2 准备接 OCI/docker package 时加的探活 + bind-mount 包装。
>
> 未来真要加（社区 docker MCP server 多到不可忽视）再恢复，git history 还在。

---

## 6. Repository 接口（`internal/domain/sandbox/sandbox.go`）

```go
type Repository interface {
    // Runtime CRUD
    CreateRuntime(ctx context.Context, r *Runtime) error
    GetRuntime(ctx context.Context, id string) (*Runtime, error)
    FindRuntime(ctx context.Context, kind, version string) (*Runtime, error)        // 精确版本（默认版本由 Installer.ResolveDefault 解析）
    ListRuntimes(ctx context.Context) ([]*Runtime, error)
    UpdateRuntime(ctx context.Context, r *Runtime) error                             // 路径 / size 更新
    DeleteRuntime(ctx context.Context, id string) error

    // Env CRUD
    CreateEnv(ctx context.Context, e *Env) error
    GetEnv(ctx context.Context, id string) (*Env, error)
    FindEnvByOwner(ctx context.Context, ownerKind, ownerID string) (*Env, error)
    ListEnvsByRuntime(ctx context.Context, runtimeID string) ([]*Env, error)        // GC 用
    ListEnvsByOwnerKind(ctx context.Context, ownerKind string) ([]*Env, error)      // UI 用
    UpdateEnv(ctx context.Context, e *Env) error
    DeleteEnv(ctx context.Context, id string) error

    // Layer B leak prevention（详 §8 Service.RestoreOrCleanupOnBoot）
    SetEnvRunningPID(ctx context.Context, envID string, pid int) error
    ClearEnvRunningPID(ctx context.Context, envID string) error
    ListEnvsWithRunningPID(ctx context.Context) ([]*Env, error)                      // boot 扫描入口

    // 聚合查询
    TotalSizeBytes(ctx context.Context) (int64, error)                               // UI 显示磁盘占用
    ListEnvsLastUsedBefore(ctx context.Context, t time.Time) ([]*Env, error)         // GC 候选
}
```

实现在 `internal/infra/store/sandbox/sandbox.go`，标准 GORM。

---

## 7. RuntimeInstaller / EnvManager 接口（开闭设计）

```go
// internal/domain/sandbox/installer.go

// RuntimeInstaller 知道怎么装 + 定位某种 runtime。
// 加新 runtime 类型 = 写一个 Installer 实现 + main.go 注册一行，
// sandbox 核心逻辑 0 修改。
type RuntimeInstaller interface {
    Kind() string                                                          // "python" / "node" / "java" / 任何

    // Install 把指定版本装到 sandboxRoot（<dataDir>/sandbox/ 的绝对路径）
    // 下某个位置，返回相对 sandboxRoot 的安装目录路径——该相对路径会存进
    // Runtime.Path。Installer 自己选 layout（如 mise 让所有 kind + version
    // 共享单个 MISE_DATA_DIR，返 "mise-data/installs/<kind>/<version>"）。
    // stream 是进度推流（前端 SSE 用）。
    Install(ctx context.Context, version, sandboxRoot string, stream ProgressFunc) (relPath string, err error)

    // Locate 返回已装 (version, sandboxRoot) runtime 主可执行文件的绝对路径
    // （如 "<sandboxRoot>/mise-data/installs/python/3.12.5/bin/python"）。
    // 实现通常调用底层 installer 的查找机制（mise where 等）保持同步。
    Locate(version, sandboxRoot string) (binPath string, err error)

    // ResolveDefault 返回该 kind 的默认版本（"3.12.5" 之类）
    ResolveDefault(ctx context.Context) (string, error)
}

// ToolRegistry 让 EnvManager 懒解析支持工具（uv / pnpm / mvn / bundler /
// composer）的二进制路径，而不耦合 mise / boot 顺序 / Service 内部。
// app/sandbox/Service 实现 ToolRegistry——main.go 构造 EnvManager 时把
// Service 自己作 registry 注入。
type ToolRegistry interface {
    // EnsureTool 返 (kind, version) 对应的绝对二进制路径，缺则懒装 runtime。
    // version="" 请求该 kind 默认。无 installer 注册返 ErrRuntimeNotSupported。
    EnsureTool(ctx context.Context, kind, version string) (binPath string, err error)
}

// EnvManager 知道怎么在 runtime 上建包隔离环境。V3 仅 2 个实现：
// PythonEnvManager（用 ToolRegistry 懒解析 uv 路径）+ NodeEnvManager（用 npm，
// runtimePath 直接拿 node 解释器即可，不需要支持工具）。
type EnvManager interface {
    Kind() string  // 同 RuntimeInstaller.Kind()

    // CreateEnv 在 envPath 建立独立 env（venv / node_modules / 等）
    // runtimePath 是 RuntimeInstaller.Locate 返的解释器目录
    CreateEnv(ctx context.Context, runtimePath, envPath string) error

    // InstallDeps 在 env 里装 deps（uv pip install / npm install --prefix，统一走包管理器原生共享机制）
    InstallDeps(ctx context.Context, runtimePath, envPath string, deps []string, stream ProgressFunc) error

    // EnvBin 返 env 内某 binary 的绝对路径（用于 Spawn）
    // 例：python kind + "python" → "<envPath>/.venv/bin/python"
    EnvBin(envPath, binName string) string

    // EnvDir 返 env 主目录（spawn 时 cwd 候选）
    EnvDir(envPath string) string
}
```

### v1 ship 的 Installer + EnvManager（V3 — 仅 Python + Node）

§S12 平铺规则：所有实现按 `installer_<name>.go` / `envmanager_<name>.go` 命名，
直接放 `internal/infra/sandbox/` 下，不分子目录。

#### RuntimeInstaller（3 个）

| Installer | Kind | 文件 | 备注 |
|---|---|---|---|
| **mise python** | `python` | `installer_mise.go`（参数化）| python-build-standalone via mise |
| **mise node** | `node` | 同上 | nodejs.org tarball via mise |
| **mise uv** | `uv` | 同上 | Python 二级火箭，pin 0.11.4（详 §4）|

`MiseInstaller` 是单一 struct，构造时传 `kind` + `defaultVersion` 参数化注册同一类型 3 次（python / node / uv）。

#### EnvManager（2 个，V3 collapse 后）

| EnvManager | Kind | 文件 | 隔离机制 |
|---|---|---|---|
| **Python** | `python` | `envmanager_python.go` | `uv venv` + `uv pip install`（uv hardlink 全局 wheel cache） |
| **Node** | `node` | `envmanager_node.go` | `npm install --prefix=<env_path>`（不用 pnpm；npm 内置够用 + 全平台稳） |

**已删（V3 collapse 2026-05-08）**：原 9 个 EnvManager（Rust / Go / Java / Ruby / PHP / .NET / Playwright / Static / Generic）+ 3 个 Installer（Playwright / Dotnet / Static）共 ~700 行无 caller 代码移除。详 §4。

### main.go 装配（实际形态，cmd/server/main.go::registerSandboxStack）

```go
sandboxService := sandboxapp.NewSandbox(sandboxapp.Config{
    DataDir:  dataDir,
    Repo:     sandboxstore.NewRepository(db),
    Notif:    notifPublisher,        // sandbox_env per-env 通知（详 §10）
    Log:      log,
})

if err := sandboxService.Bootstrap(ctx); err != nil {
    // degraded mode（详 §15.1）—— 不 fatal，banner + 后续入口返 ErrSandboxUnavailable
    log.Error("sandbox bootstrap failed; entering degraded mode", zap.Error(err))
}

miseBin := sandboxService.MiseBin()  // <dataDir>/sandbox/bin/mise（degraded 时返空字符串）

// V3 仅注册 3 RuntimeInstaller + 2 EnvManager
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "python", "3.12"))
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "node",   "22"))
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "uv",     "0.11.4"))

sandboxService.RegisterEnvManager(sandboxinfra.NewPythonEnvManager(sandboxService))
sandboxService.RegisterEnvManager(sandboxinfra.NewNodeEnvManager(sandboxService))

// Layer B 启动扫描（详 §8 Service.RestoreOrCleanupOnBoot）
sandboxService.RestoreOrCleanupOnBoot(ctx)

// 注入下游 service
mcpService    := mcpapp.New(...)    // 持 sandboxapp.Service 作 PluginSandbox
forgeService  := forgeapp.New(...)  // 同上
chatService   := chatapp.New(...)   // Bash tool 持 sandboxapp.Service 作 ConversationSandbox
```

**注**：构造 EnvManager 不触发任何装机——uv 首次 `EnsureEnv` 调 `Service.EnsureTool(ctx, "uv", "")` 时才 mise install。boot 极快。

未来真要加 Rust/Go/Java/etc. 时按相同模式扩 1 Installer + 1 EnvManager + main.go 多 2 行注册。

---

## 8. Service 层（`internal/app/sandbox/sandbox.go`）

```go
type Service struct {
    repo          sandboxdomain.Repository
    installers    map[string]sandboxdomain.RuntimeInstaller
    envManagers   map[string]sandboxdomain.EnvManager
    sandboxRoot   string                              // <dataDir>/sandbox/
    runtimesDir   string                              // <dataDir>/sandbox/runtimes/
    envsDir       string                              // <dataDir>/sandbox/envs/
    miseBin       string                              // <dataDir>/sandbox/bin/mise（go:embed 解出）
    notif         notificationspkg.Publisher          // 每次 env 状态变化 publish "sandbox_env" 通知
    log           *zap.Logger
    mu            sync.Mutex                          // 保护 RegisterInstaller / EnvManager 启停
    installLocks  sync.Map                            // map[runtimeKind]*sync.Mutex 防并发同 runtime install
    activeHandles map[handleID]*trackedHandle         // Layer A graceful shutdown：spawn 出去的长生命 handle
    nextHandleID  atomic.Uint64
    bootstrapped  atomic.Bool                         // false → degraded 模式（详 §15.1）
    bootstrapErr  atomic.Pointer[error]
}

// === 装配 ===
func (s *Service) RegisterInstaller(i RuntimeInstaller)
func (s *Service) RegisterEnvManager(m EnvManager)

// === Bootstrap / 状态 ===
func (s *Service) Bootstrap(ctx context.Context) error      // 解 mise binary + 写 mise 全局配置
func (s *Service) IsReady() bool                             // bootstrapped.Load()
func (s *Service) BootstrapError() error                     // bootstrapErr.Load()
func (s *Service) RetryBootstrap(ctx context.Context) error  // UI 重试（详 §15.1）
func (s *Service) SandboxRoot() string                       // 给装配用（Static EnvManager 等）
func (s *Service) MiseBin() string                           // 给 MiseInstaller 用

// === ToolRegistry（实现 sandboxdomain.ToolRegistry）===
// 给 EnvManager 懒解析支持工具（Python EnvManager 调它拿 uv binary 路径）。
func (s *Service) EnsureTool(ctx context.Context, kind, version string) (binPath string, err error)

// === Runtime / Env CRUD ===
func (s *Service) EnsureRuntime(ctx context.Context, spec RuntimeSpec, stream ProgressFunc) (*Runtime, error)
func (s *Service) EnsureEnv(ctx context.Context, owner Owner, spec EnvSpec, stream ProgressFunc) (*Env, error)
func (s *Service) GetEnv(ctx context.Context, id string) (*Env, error)              // GET /sandbox/envs/{id}
func (s *Service) Destroy(ctx context.Context, owner Owner) error                    // plugin 卸载
func (s *Service) DestroyEnvByID(ctx context.Context, id string) error               // POST /sandbox/envs/{id}:destroy
func (s *Service) DeleteRuntime(ctx context.Context, id string) error                // POST /sandbox/runtimes/{id}:destroy（先 reject 若有 env 用）

// === Spawn ===
func (s *Service) Spawn(ctx context.Context, owner Owner, opts SpawnOpts) (*ExecutionResult, error)         // 一次性，timeout 守卫
func (s *Service) SpawnLongLived(ctx context.Context, owner Owner, opts SpawnOpts) (LongLivedHandle, error) // 长生命，写 RunningPID + activeHandles 跟踪

// === Layer A leak prevention ===
func (s *Service) Shutdown(ctx context.Context) error
//   遍历 activeHandles → SIGTERM → 5s 超时 → SIGKILL；保证进程退出前杀干净所有 spawn 子进程

// === Layer B leak prevention ===
func (s *Service) RestoreOrCleanupOnBoot(ctx context.Context)
//   启动时扫所有 envs.RunningPID != 0 的行：
//     - PID 还活 + cmdline 含 forgify env 路径 → 重新 attach 进 activeHandles（如 mcp server 在 reboot 之间存活）
//     - PID 死 / cmdline 不匹配 → 清掉 RunningPID（前次进程残留状态）

// === 查询 / 管理 ===
func (s *Service) ListRuntimes(ctx context.Context) ([]*Runtime, error)
func (s *Service) ListEnvs(ctx context.Context, ownerKind string) ([]*Env, error)
func (s *Service) TotalDiskUsage(ctx context.Context) (int64, error)
func (s *Service) GC(ctx context.Context, olderThan time.Duration) (int, error)   // 清理超 N 天未用（v1 默认手动；HTTP `:gc` 触发）
```

**Layer A vs Layer B leak prevention**（重要）：

| 层 | 防范 | 机制 |
|---|---|---|
| **Layer A** — 进程内 | Forgify 退出时遗留 spawn 子进程 | `Service.Shutdown` 遍历 `activeHandles` graceful kill；main.go 在 ctx cancel 后调一次 |
| **Layer B** — 重启之间 | Forgify 崩溃 / OS reboot 后 sandbox_envs 表里仍标 RunningPID 但进程已死 | `Env.RunningPID` 列做 manifest；启动时 `RestoreOrCleanupOnBoot` 扫一遍按 PID + cmdline 双确认重 attach 或清空 |

二者**都不是 GC**——GC 是磁盘清理；Leak prevention 是进程 / 状态一致性。

### EnsureRuntime 关键逻辑

```go
func (s *Service) EnsureRuntime(ctx context.Context, spec RuntimeSpec, stream ProgressFunc) (*Runtime, error) {
    installer, ok := s.installers[spec.Kind]
    if !ok {
        return nil, ErrRuntimeNotSupported
    }
    
    // 解析版本
    version := spec.Version
    if version == "" {
        var err error
        version, err = installer.ResolveDefault(ctx)
        if err != nil { return nil, err }
    }
    
    // 查 DB 看是否已装
    if existing, err := s.repo.FindRuntime(ctx, spec.Kind, version); err == nil {
        return existing, nil  // 已装，直接返
    }
    
    // 没装 → 按 kind 加锁防并发同 install
    lock := s.getInstallLock(spec.Kind)
    lock.Lock()
    defer lock.Unlock()
    
    // 双重检查（lock 期间别人可能装好了）
    if existing, err := s.repo.FindRuntime(ctx, spec.Kind, version); err == nil {
        return existing, nil
    }
    
    // 真去装。Installer 自己决定文件布局；返回的 relPath 是 install
    // 目录相对 sandboxRoot 的相对路径，存到 Runtime.Path。
    relPath, err := installer.Install(ctx, version, s.sandboxRoot, stream)
    if err != nil {
        return nil, fmt.Errorf("sandboxapp.EnsureRuntime: %w", err)
    }
    
    // 记 DB
    runtime := &Runtime{
        ID:          newRuntimeID(),
        Kind:        spec.Kind,
        Version:     version,
        Path:        relPath,                                       // 由 installer 决定
        SizeBytes:   computeDirSize(filepath.Join(s.sandboxRoot, relPath)),
        InstalledAt: time.Now(),
    }
    if err := s.repo.CreateRuntime(ctx, runtime); err != nil { return nil, err }
    
    return runtime, nil
}
```

### EnsureEnv 关键逻辑

```go
func (s *Service) EnsureEnv(ctx context.Context, owner Owner, spec EnvSpec, stream ProgressFunc) (*Env, error) {
    // 1. 查 DB 是否已存在该 owner 的 env
    if existing, err := s.repo.FindEnvByOwner(ctx, owner.Kind, owner.ID); err == nil {
        // 已存在 → 检查 deps 是否一致
        if depsEqual(existing.Deps, spec.Deps) && existing.Status == "ready" {
            return existing, nil  // 完全一致，直接复用
        }
        // 不一致 → 删旧建新（隐式更新）
        if err := s.Destroy(ctx, owner); err != nil { return nil, err }
    }
    
    // 2. 装 runtime
    runtime, err := s.EnsureRuntime(ctx, spec.Runtime, stream)
    if err != nil { return nil, err }
    runtimePath := filepath.Join(s.runtimesDir, runtime.Path)
    
    // 3. 创建 env 目录
    envID := newEnvID()
    envPath := filepath.Join(s.envsDir, owner.Kind, owner.ID)  // 例: envs/mcp/github/
    
    // 4. 先写"installing"状态（让 UI 知道在装）
    env := &Env{
        ID: envID, OwnerKind: owner.Kind, OwnerID: owner.ID, OwnerName: owner.Name,
        RuntimeID: runtime.ID, Deps: spec.Deps,
        Path: filepath.Join(owner.Kind, owner.ID), Status: "installing",
        CreatedAt: time.Now(), LastUsedAt: time.Now(),
    }
    if err := s.repo.CreateEnv(ctx, env); err != nil { return nil, err }
    
    // 5. 调 EnvManager
    em := s.envManagers[spec.Runtime.Kind]
    if err := em.CreateEnv(ctx, runtimePath, envPath); err != nil {
        env.Status = "failed"
        env.ErrorMsg = err.Error()
        s.repo.UpdateEnv(ctx, env)
        return nil, fmt.Errorf("sandboxapp.EnsureEnv create: %w", err)
    }
    if err := em.InstallDeps(ctx, runtimePath, envPath, spec.Deps, stream); err != nil {
        env.Status = "failed"
        env.ErrorMsg = err.Error()
        s.repo.UpdateEnv(ctx, env)
        return nil, fmt.Errorf("sandboxapp.EnsureEnv deps: %w", err)
    }
    
    // 6. 标 ready
    env.Status = "ready"
    env.SizeBytes = computeDirSize(envPath)
    s.repo.UpdateEnv(ctx, env)
    
    return env, nil
}
```

---

## 9. 隔离机制详解

### 9.1 共享层 vs 隔离层

```
共享（runtime binary 无状态）：runtimes/<lang>/<version>/
  └── 只是解释器/编译器二进制——所有 plugin 共享
  
隔离（每 plugin 一份）：envs/<owner_kind>/<owner_id>/
  └── 这个 plugin 装的所有包，外人看不到
```

### 9.2 各语言隔离机制

V3 仅支持 Python + Node 两种 EnvManager（详 §4 / §7 ship list）。

| 语言 | env 内文件 | spawn 时强制本地化的机制 |
|---|---|---|
| **Python** | `.venv/bin/python` + `.venv/lib/python3.x/site-packages/` | venv shim python 的 `sys.path` 钉死本 venv |
| **Node** | `node_modules/<package>/` | Node `require()` resolution 算法找最近 `node_modules`（cwd 上溯）|

> 已删（V3 collapse）：Rust / Go / Java / Ruby / PHP / Browsers 行——9 个 EnvManager 已移除（详 §4 / §7）。未来恢复时按相同模式 1 行加回。

### 9.3 关键约束："永远不装到全局"

- **禁止** `pip install --user`
- **禁止** `npm install -g`

Python EnvManager / Node EnvManager 的 InstallDeps 实现**必须**走 prefix/local 模式（`uv pip install` 自带 venv 隔离；`npm install --prefix=<env_path>` 强制本地），绝不污染全局环境。

### 9.4 包冲突真实案例

**forge A** 装 `pandas==1.5`：
```
envs/forge/<envid_A>/.venv/lib/python3.12/site-packages/pandas/  ← 1.5 版本
```

**forge B** 装 `pandas==2.0`：
```
envs/forge/<envid_B>/.venv/lib/python3.12/site-packages/pandas/  ← 2.0 版本
```

跑 A 用 `envs/forge/<envid_A>/.venv/bin/python`——这个 python shim 的 sys.path 第一个就是自己 venv 的 site-packages，**根本看不到 B 的 pandas**。零冲突。

**github MCP** 装 `@modelcontextprotocol/server-github@1.0`，**slack MCP** 间接依赖同一 npm transitive 但版本不同：

```
envs/mcp/github/node_modules/@modelcontextprotocol/server-github/  ← 1.0
envs/mcp/slack/node_modules/<transitive-pkg>/                       ← 不同版本
```

Spawn 各自时 `cwd` 是各自 env 目录，Node `require()` 只找本目录 node_modules。零冲突。

### 9.5 Bash 自动路由 + 对话 scratch env（核心收口机制）

**问题**：LLM 通过 Bash tool 跑 `pip install pandas` 或 `python script.py`——如果 Bash 直接走系统 PATH，会污染用户系统 Python，且不可控。

**简单方案（已废弃）**：在 Bash tool 加 denylist 拦截 `pip install`、`python` 等命令——告诉 LLM 用 forge tool。**问题**：denylist 不可能穷举（`/usr/bin/python3` / `python3.12` / `pipx` / `python -m pip` ...）；且对 LLM "我就想试个小脚本"的合理需求过于严苛。

**采用方案**：**Bash 自动路由到 conversation scratch env**——sandbox 检测命令意图，把执行环境替换为该对话的 scratch env。LLM 自由 install/run，**所有副作用都被收口到沙箱里**。

#### Conversation env 概念

每个对话按需为每种 runtime 起一个独立 scratch env：

```
<dataDir>/sandbox/envs/conversation/
├── cv_abc_python/        ← cv_abc 用了 Python
├── cv_abc_node/          ← cv_abc 也用了 Node
└── cv_xyz_python/        ← cv_xyz 只用了 Python
```

`(conversation_id, runtime_kind)` 一对一，**绝不会两个对话共享**。

> **owner.ID 用 `_` 分隔不用 `:`** —— `:` 是 POSIX PATH separator + Windows path drive letter delimiter，混进路径或 env var 会咬到下游；统一 `_` 安全。Service 入口校验时拒 `:`，违反返 `ErrInvalidOwnerID`。

#### Bash tool 改造（自动路由）

```go
// app/tool/shell/bash.go
func (b *Bash) Execute(ctx context.Context, argsJSON string) (string, error) {
    convID := reqctxpkg.RequireConversationID(ctx)
    cmd := parseCommand(argsJSON)
    
    runtimeKind := detectRuntime(cmd)   // 见下表
    
    if runtimeKind != "" {
        // Lazy create conversation env if not exists
        owner := sandboxdomain.Owner{
            Kind: "conversation",
            ID:   fmt.Sprintf("%s_%s", convID, runtimeKind),  // `_` 分隔，不能 `:`（详 §9.5 owner.ID 约定）
            Name: fmt.Sprintf("Conv %s scratch (%s)", convID, runtimeKind),
        }
        env, err := b.sandbox.EnsureEnv(ctx, owner, sandboxdomain.EnvSpec{
            Runtime: sandboxdomain.RuntimeSpec{Kind: runtimeKind},
            Deps:    nil,  // 空 env，让 LLM 自由装
        }, progressFn)
        if err != nil { return "", err }

        // Bash 自己拼 envBin path + PATH（.venv/bin / node_modules/.bin），调 Spawn 起子进程
        return b.sandbox.Spawn(ctx, owner, sandboxdomain.SpawnOpts{
            Cmd:     "/bin/sh",
            Args:    []string{"-c", cmd},
            Env:     bashEnvWithPATH(env),  // PATH 加上 .venv/bin / node_modules/.bin
            Timeout: bashTimeout,
        })
    }

    // 非 runtime 命令（git/ls/cat 等）→ 普通 shell（仅 /usr/bin /bin PATH，不走 sandbox）
    return b.runShellPlain(ctx, cmd)
}
```

> **注**：早期文档提到 `SpawnShell` / `EnvBinDirs` 方法——实际不存在。Bash tool 自己用 `EnvManager.EnvBin(envPath, binName)` 单 binary 解析 + 自己拼 PATH，**不另设 SpawnShell 入口**。

#### detectRuntime — 命令到 runtime 的映射

实现走 **AST 解析**（`mvdan.cc/sh/v3/syntax`）而非 first-token regex——`shfmt` 的同一 parser，pure Go，跨平台 0 依赖。流程：parse 命令 → `syntax.Walk` 遍历每个 `CallExpr` → 对每个 call 经 `classifyCallExpr`（剥路径前缀 / 处理 wrapper / env / which）→ 匹配下面的 runtime 表，**首次命中胜**。pattern 匹配的是规范化后的*裸命令名*（无路径、无 env 前缀、无 flag）：

```go
// V3 collapse：仅 python + node 注册了 EnvManager；命中即建 conversation env
// 并真路由。其他 runtime（rust / go / ruby / php / java / dotnet）detector
// 已删——这些命令在 detectRuntime 不触发 → 落系统 shell（与 `git status` 同
// 路径，不进 sandbox；想 isolate 时由用户自己装 mise / docker）。
var runtimeDetectors = []runtimeDetector{
    {Kind: "python", Pattern: regexp.MustCompile(`^(?:python3?(?:\.\d+)?|pip3?|uv|virtualenv|pipenv|poetry)$`)},
    {Kind: "node",   Pattern: regexp.MustCompile(`^(?:node|npm|npx|yarn|pnpm)$`)},
}
```

**AST 走查覆盖的复杂命令**（first-token regex 全部漏掉）：

| 写法 | AST 怎么处理 |
|---|---|
| `cd /tmp && python script.py` | BinaryCmd 的两支都走，`python` CallExpr 命中 |
| `cd a && cd b && npm test` | 任意层 `cd` 链都走，最终 `npm` 命中 |
| `pip install foo \| tee log` | pipe 两端 CallExpr 都走，第一个命中即停 |
| `(pip install pandas)` | Subshell 下钻到 inner CallExpr |
| `$(pip install pandas)` / 反引号 | CmdSubst 下钻 |
| `bash -c "pip install pandas"` | `classifyCallExpr` 识别 wrapper，对 `-c` 后字面量递归调 `detectRuntime` |
| `bash -lc "pip install x"` / `-cl` | 单短横组合 flag 含 `c` 当 `-c` 处理 |
| `env PYTHONPATH=. python ...` | `env` wrapper 跳过赋值 / flag，首个非赋值 arg 是真命令 |
| `FOO=bar python script.py` | AST 把前导赋值放 `Assigns`，`Args[0]` 直接是 `python` |
| `/usr/bin/python3 script.py` | `stripPath` 取 basename 后匹配 |
| `which python3` / `type pip` / `command -v cargo` | 自省命令按 argument 路由 |

**parser 看不见的静态逃逸**（fail-open，靠 LLM 自觉避免；Bash tool description 已警告）：

| 写法 | 为何看不见 |
|---|---|
| `eval "pip install pandas"` | eval 字符串是 Lit，AST 不下钻字面量 |
| `source ./install.sh` / `. ./install.sh` | 脚本路径不透明，无法 introspect 文件内容 |
| `$(<动态字符串>)` 含变量扩展 | 变量值运行时才有，static skeleton 看不到 |

malformed shell（罕见）→ `detectRuntimeFirstToken` fallback：取首 token 直接匹配，至少不静默丢掉 `pip install ...` 这种简单形态。

#### Bash 端拼 PATH（替代不存在的 SpawnShell）

实际架构：`Service.Spawn` 通用入口接 SpawnOpts；Bash tool 自己用 `EnvManager.EnvBin(envPath, binName)` 拿目标二进制路径 + 自己拼 PATH 加进 SpawnOpts.Env。

```go
// app/tool/shell/bash.go::bashEnvWithPATH
func bashEnvWithPATH(env *sandboxdomain.Env) map[string]string {
    em := pythonOrNodeEnvManager(env.RuntimeKind)
    venvBin := em.EnvBin(env.Path, "python")    // .venv/bin/python（python kind）
    nodeMods := filepath.Join(env.Path, "node_modules", ".bin")  // node kind

    binDir := filepath.Dir(venvBin)              // .venv/bin
    augmented := strings.Join([]string{binDir, nodeMods, "/usr/bin", "/bin"}, ":")

    return map[string]string{"PATH": augmented}
}

// 然后 Bash 调 b.sandbox.Spawn(ctx, owner, SpawnOpts{
//     Cmd: "/bin/sh", Args: []string{"-c", cmd}, Env: bashEnvWithPATH(env), ...
// })
```

> 早期文档的 `Service.SpawnShell` / `EnvManager.EnvBinDirs` 方法**不存在**——Service 只暴露通用 Spawn，PATH 拼装由调用方负责。

#### LLM 视角的体验

```
LLM: "我想用 pandas 处理 CSV"
LLM: Bash("pip install pandas")
  ↓ sandbox 检测：pip → python kind
  ↓ EnsureEnv(owner=conversation/cv_abc_python)
  ↓ 第一次：lazy 装 Python 3.12 + uv + 建 venv
  ↓ uv pip install pandas
  ↓ 返："Successfully installed pandas-2.0..."

LLM: 装好了，现在跑代码
LLM: Bash("python -c 'import pandas; ...')
  ↓ EnsureEnv 已存在 → 直接复用
  ↓ 用 conversation env 的 venv python 跑（pandas 在那里）
  ↓ 返代码执行结果

(用户系统 Python 0 影响；其他对话 0 影响)
```

#### 真实收益

| 之前担心 | 现在的现实 |
|---|---|
| LLM `pip install pandas` 污染系统 | ✅ 自动落进 conversation env |
| LLM 跑 `python script.py` 用系统 Python | ✅ 自动用 conversation env 的 venv python |
| 要不要 denylist `pip install`？ | ❌ **不需要**——基础设施层面已经收口 |
| LLM 跨对话污染 | ✅ 不可能——每对话独立 env |
| 演示前需手动准备 Python 环境 | ✅ 用户机器 0 配置——agent 用啥自动建啥 |

#### Conversation env 生命周期

| 触发 | 行为 |
|---|---|
| LLM 首次在该对话用某 runtime | Lazy 起 conversation env（包括装 runtime 如未装）|
| 同对话第二次用同 runtime | 复用，0 等待 |
| 用户软删对话 | env 保留（恢复对话仍可用）|
| 用户硬删对话（DB 真删）| 立即 Destroy 释放磁盘 |
| 用户主动"重置对话环境"（UI）| 立即 Destroy + 下次 LLM 用时重建 |
| 后台 GC | **v1 不开 auto-GC**——uv/pnpm 共享让 conv env 磁盘占用极小；用户想清就 UI 点 |

**为什么 v1 不开 auto-GC**：100 个 conv 都装 pandas → uv hardlink → 实际磁盘 = 1 份 pandas + 100 个空 venv（每个几 KB）。GC 几乎没东西可省。**真到 disk quota 极端用户场景再加**（v2 演化）。

### 9.6 边界情况（罕见但要 watchout）

| 情况 | 风险 | sandbox 对策 |
|---|---|---|
| Python C 扩展链接系统库（numpy 链 BLAS）| 系统级 libBLAS 共享 | 现代 wheel 把依赖打包进 wheel；不会冲突 |
| npm 包写 `~/.config/<name>` | 全局 user 目录 | 个别恶意包可能干，**正常包不会**；遇到就提 plugin 作者 |
| `--user` / `-g` 强行污染 | 全局 site-packages | sandbox 实施时**禁用这些 flag** |
| 端口冲突 | 端口是机器全局 | MCP 走 stdio 不监听端口；Forgify HTTP 自己一个端口；不冲突 |

---

## 10. Notifications + Install Progress

### 10.1 `sandbox_env` per-env state notification

每次 env 状态转换（installing → ready / failed → ready / ready → destroyed）经 `notificationspkg.Publisher` 推 `sandbox_env` 通知，前端按 envID 局部更新。**不发全量快照**。

```json
{
  "type": "sandbox_env",
  "id":   "se_<16hex>",         // env ID
  "data": {                      // Env payload
    "id":         "se_xxx",
    "ownerKind":  "mcp",
    "ownerID":    "github",
    "runtimeId":  "sr_xxx",
    "deps":       ["@modelcontextprotocol/server-github"],
    "path":       "mcp/github",
    "sizeBytes":  12345678,
    "status":     "ready",       // installing / ready / failed / destroyed
    "errorMsg":   ""
  }
}
```

**触发点**（详 `app/sandbox/sandbox.go::publishEnv` / `publishEnvDeleted`）：
- `EnsureEnv` 写 `installing` 后立刻 publish
- `EnsureEnv` 装机成功 → `ready` publish
- `EnsureEnv` 装机失败 → `failed` publish
- `Destroy` / `DestroyEnvByID` → `destroyed` 单条最终 publish + 行删

### 10.2 Install 进度 → eventlog Emitter（不走 sandbox notification）

Install 进度（`Installing Node.js 22.5.0...` / `Downloading @playwright/mcp...`）**不**经 sandbox notification——通过调用方的 ctx eventlog Emitter 推 `progress` block 到该 tool_call 父下。

`pkg/installprogress/Run` helper 把 ProgressFunc 适配成 eventlog block：

```go
// mcpapp.install_mcp_server tool 调 sandbox 时：
err := installprogresspkg.Run(ctx, "Installing MCP server", func(progressFn ProgressFunc) error {
    return sandboxService.EnsureEnv(ctx, owner, spec, progressFn)
})
// helper 内部：
//   - StartBlock(progress) 挂在 install_mcp_server tool_call 父下
//   - progressFn 每次调用 → DeltaBlock("...stage... 45%\n")
//   - Run 完成 → StopBlock(completed)
```

详 §S18 推流约定（`pkg/eventlog`）+ [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) eventlog 协议。

---

## 11. HTTP API

### Sandbox 状态 / 管理（debug + UI）

| Method + Path | 用途 |
|---|---|
| `GET /api/v1/sandbox/runtimes` | 列所有已装 runtime（kind/version/size/谁在用） |
| `GET /api/v1/sandbox/envs?ownerKind=mcp` | 列指定 owner kind 的所有 env。**强制 ownerKind**(避免无界响应),缺 → 400 OWNER_KIND_REQUIRED;非白名单(`function`/`handler`/`mcp`/`skill`/`conversation`)→ 400 INVALID_OWNER_KIND(#16,2026-05) |
| `GET /api/v1/sandbox/envs/{id}` | 单 env 详情 |
| `GET /api/v1/sandbox/disk-usage` | 总磁盘占用 + 按类目分 |
| `POST /api/v1/sandbox/envs/{id}:destroy` | 强制删某 env（debug 用，正常通过 plugin 卸载触发） |
| `POST /api/v1/sandbox/runtimes/{id}:destroy` | 强制删 runtime（先确认无 env 用）|
| `POST /api/v1/sandbox:gc?olderThanDays=30` | 用户主动触发 GC：清理超 N 天未用的 env（v1 默认不自动跑，靠用户点）|

### Bootstrap 状态 / 重试（degraded mode 用）

| Method + Path | 用途 |
|---|---|
| `GET /api/v1/sandbox/bootstrap-status` | 当前 bootstrap 状态（ok / 失败原因）|
| `POST /api/v1/sandbox:retry-bootstrap` | 触发重试，返新状态 |

### Conversation scratch env（per 对话）

| Method + Path | 用途 |
|---|---|
| `GET /api/v1/conversations/{id}/sandbox-envs` | 列该对话的 scratch envs（哪些 runtime / 装了什么 / 多大）|
| `POST /api/v1/conversations/{id}/sandbox-envs/{kind}:reset` | 清掉某 runtime 的 scratch env（让 LLM 重新干净开始）|
| `POST /api/v1/conversations/{id}/sandbox-envs:reset-all` | 清掉所有 scratch（核选项）|

### `:install-runtime` 手动安装（罕见，UI 不暴露）

| Method + Path | 用途 |
|---|---|
| `POST /api/v1/sandbox/runtimes:install` | 手动装某 runtime（body: kind + version）；通常 plugin install 自动触发，无需直接调 |

---

## 12. 错误码（11 个，V3 后）

| Sentinel | HTTP | Wire Code |
|---|---|---|
| `sandboxdomain.ErrRuntimeNotSupported` | 422 | `SANDBOX_RUNTIME_NOT_SUPPORTED` |
| `sandboxdomain.ErrRuntimeInstallFailed` | 502 | `SANDBOX_RUNTIME_INSTALL_FAILED` |
| `sandboxdomain.ErrEnvNotFound` | 404 | `SANDBOX_ENV_NOT_FOUND` |
| `sandboxdomain.ErrEnvCreateFailed` | 502 | `SANDBOX_ENV_CREATE_FAILED` |
| `sandboxdomain.ErrDepInstallFailed` | 502 | `SANDBOX_DEP_INSTALL_FAILED` |
| `sandboxdomain.ErrSpawnFailed` | 502 | `SANDBOX_SPAWN_FAILED` |
| `sandboxdomain.ErrSpawnTimeout` | 504 | `SANDBOX_SPAWN_TIMEOUT` |
| `sandboxdomain.ErrEnvInUse` | 409 | `SANDBOX_ENV_IN_USE` |
| `sandboxdomain.ErrInvalidOwnerID` | 400 | `SANDBOX_INVALID_OWNER_ID` |
| `sandboxdomain.ErrCmdRequired` | 400 | `SANDBOX_CMD_REQUIRED` |
| `sandboxdomain.ErrSandboxUnavailable` | 503 | `SANDBOX_UNAVAILABLE` |

**已删（V3 collapse）**：`ErrDockerNotInstalled` / `ErrDockerDaemonDown` 与 Docker runtime 同期移除（详 §5b）。

---

## 13. 测试覆盖（V3 后）

| 层 | 文件 | 测试数 | 覆盖 |
|---|---|---|---|
| domain | `internal/domain/sandbox/sandbox_test.go` | 5 | Runtime/Env JSON / Sentinel / Owner valid（含 `_` vs `:` 校验）|
| store | `internal/infra/store/sandbox/sandbox_test.go` | 12 | CRUD + UNIQUE 约束 + ListByOwnerKind + ListByRuntime + GC 候选 + RunningPID set/clear/list |
| infra/mise | `internal/infra/sandbox/mise_test.go` | 8 | 多 kind 路由（python / node / uv）/ Install / Locate / 全局配置写入 |
| infra/exec | `internal/infra/sandbox/exec_helper_test.go` + `proc_<goos>_test.go` | ~6 | RunWithStderrCapture / 跨平台 process kill |
| envmanager | `internal/infra/sandbox/envmanager_python_test.go` + `envmanager_node_test.go` | ~10 | uv venv + uv pip install / npm install --prefix（验证 store 共享）/ EnvBin 单 binary 解析 |
| app/sandbox | `internal/app/sandbox/sandbox_test.go` + `restore_test.go` + `spawn_test.go` | 18 | EnsureRuntime 锁防并发 / EnsureEnv 复用 / Destroy / Spawn timeout / GC / 失败 status 转 / RestoreOrCleanupOnBoot 双路径 / Shutdown graceful |
| pipeline | `test/sandbox/sandbox_test.go` | 6 | 真起 Python venv 装 markitdown / 真起 Node 装某 npm 包 / Spawn 真子进程 / Destroy 干净 / 多 plugin 隔离 / **多 conv 共装 pandas 验证 uv hardlink 共享**（du 测实际磁盘 ≈ 1×）|

总计 ~67 单测 + 6 pipeline 场景。

**fake mise 注入点**：infra/sandbox/mise 的 binary 路径可注入；测试用 mock binary（`backend/test/sandbox/fakemise/`）模拟 install/locate 行为，避免真下载。

**已删（V3 collapse）**：installer/playwright_test.go / envmanager_{rust,go,java,ruby,php,dotnet,playwright,static,generic}_test.go 全部移除——9 个 EnvManager + 3 个 Installer 不存在了。

---

## 14. 与其他 domain 的关系

| 关系 | 说明 |
|---|---|
| **forge** | forgeapp 通过 `forgesandboxport.Sandbox` 接口（在 forge domain 定义）调本服务；现有 `infra/sandbox` 升级为本设计的 first consumer |
| **mcp** | mcpapp 装 server 时调 `sandboxapp.EnsureEnv`；spawn 时调 `SpawnLongLived` 拿 stdio handle 走 JSON-RPC |
| **skill** | v1 skill 不需 runtime；未来 skill 带 deps 时复用本服务 |
| **chat / Bash tool** | **app/tool/shell/Bash 改造**：detectRuntime + 自动路由到 conversation scratch env；非 runtime 命令走普通 shell |
| **conversation** | 软删/硬删 conversation 触发对应 scratch env 的标记可清 / 立即 Destroy（详 §9.5）|
| **notifications** | 经 `notificationspkg.Publisher` 推 `sandbox_env` per-env 通知（详 §10.1）|
| **eventlog** | install 进度通过 `pkg/installprogress.Run` 适配 ProgressFunc → 调用方 ctx eventlog Emitter 推 progress block（详 §10.2）|

### 反向接口防循环依赖

**forge / mcp / skill 不直接 import sandboxapp**——通过 domain 接口：

```go
// internal/domain/sandbox/sandbox.go
type PluginSandbox interface {
    EnsureEnv(ctx context.Context, owner Owner, spec EnvSpec, stream ProgressFunc) (*Env, error)
    Spawn(ctx context.Context, owner Owner, opts SpawnOpts) (*ExecutionResult, error)
    SpawnLongLived(ctx context.Context, owner Owner, opts SpawnOpts) (LongLivedHandle, error)
    Destroy(ctx context.Context, owner Owner) error
}

// forgeapp / mcpapp / skillapp 持有 PluginSandbox 接口字段
// main.go 装配时注入 sandboxapp.Service（实现 PluginSandbox）
```

---

## 15. 装配（main.go 顺序）

```
1. logger / DB / notifications publisher（事件日志 Bridge 也在这一层起）
2. apikey / model / conversation / chat / forge / task / ask（已有）
3. sandboxapp.NewSandbox(deps) → Bootstrap（degraded fail-open）→ RegisterInstaller × 3 + RegisterEnvManager × 2 → RestoreOrCleanupOnBoot
4. subagentapp / mcpapp / skillapp / catalogapp
   - mcpapp / skillapp / forge service 各自 New() 时注入 sandboxapp.Service 作 PluginSandbox 接口
5. forge service 重构：注入 sandboxapp.Service 替代旧 infra/sandbox（V1 → V2 切换；D2-5 完成）
6. 注册 system tools 到 chat
7. router / http listen
8. main.go defer：ctx cancel → sandboxapp.Service.Shutdown(ctx) graceful kill activeHandles
```

Bootstrap 阶段不阻塞——`sandboxapp.Bootstrap` 仅 (a) 解 mise binary 从 go:embed 到 `<dataDir>/sandbox/bin/mise`、(b) 写 mise 全局配置（disable attestation），不预装任何 runtime。失败进 degraded mode（详 §15.1）+ `IsReady()` 返 false 让下游 entry 早返 `ErrSandboxUnavailable`。

---

## 15.1 Bootstrap 失败 + Degraded Mode

### 失败可能的 6 类原因

| 故障 | 概率 | 真实场景 |
|---|---|---|
| **A. embed.FS 没当前平台 mise** | 中 | 用户在 Forgify 不支持的 OS / arch（详 §17）|
| **B. 解出到 bootstrap/ 时磁盘写入失败** | 低 | ENOSPC（盘满）/ 权限拒（read-only mount / NFS / SMB）|
| **C. chmod +x 失败** | 罕见 | 受限 fs 不支持 POSIX mode bits |
| **D. mise binary 解出后 corrupt** | 罕见 | go:embed 字节坏 / 部分写入 / 杀软隔离 |
| **E. mise binary 启动跑不起来** | 中 | Linux: glibc 版本 / Alpine musl 不兼容；macOS: Gatekeeper / SIP；SELinux/AppArmor 拦 exec；Windows: SmartScreen / 杀软 |
| **F. bootstrap 后 binary 被破坏** | 低 | 杀软误删 / 用户手动 rm |

### Degraded Mode 设计原则

**Sandbox bootstrap 失败 → Forgify 仍正常启动**——只把"需要 runtime 的功能"标 unavailable，**纯文本聊天 + 不需要 runtime 的工具仍能用**。

```go
type Service struct {
    ...
    bootstrapped atomic.Bool
    bootstrapErr atomic.Value  // string，最后失败原因
}

func (s *Service) Start(ctx context.Context) error {
    if err := s.tryBootstrap(ctx); err != nil {
        s.bootstrapped.Store(false)
        s.bootstrapErr.Store(err.Error())
        s.log.Error("sandbox bootstrap failed; degraded mode", zap.Error(err))
        return nil  // ⚡ 不 return err——app 继续启动
    }
    s.bootstrapped.Store(true)
    return nil
}

// EnsureRuntime / EnsureEnv / Spawn 等所有入口加守卫
func (s *Service) EnsureRuntime(...) (*Runtime, error) {
    if !s.bootstrapped.Load() {
        return nil, fmt.Errorf("sandbox bootstrap failed: %v", s.bootstrapErr.Load())
    }
    ...
}

// HTTP / SSE 暴露状态供 UI 展示
func (s *Service) IsReady() bool             // 替代旧 BootstrapStatus()——bool only
func (s *Service) BootstrapError() error      // 失败原因（success → nil）
func (s *Service) RetryBootstrap(ctx context.Context) error  // 用户点 retry
```

### Degraded Mode 下"啥能用 啥不能"矩阵

| 功能 | bootstrap fail 时 | 原因 |
|---|---|---|
| Chat 跟 LLM 聊文字 | ✅ 能 | 不需要 sandbox |
| `Read` / `Write` / `Edit` / `Glob` / `Grep` | ✅ 能 | 不需要 runtime |
| `Bash` 跑 `ls` / `cat` / `git status` 等 | ✅ 能 | plain shell 不走 sandbox |
| `Bash` 跑 `pip install` / `python ...` / `npm ...` | ❌ 报错 | sandbox auto-route 起不来 |
| Forge create / run | ❌ 报错 | 需要 Python venv |
| MCP install / call | ❌ 报错 | 需要 runtime |
| Skill `search_skills` / `activate_skill`（无 fork）| ✅ 能 | 不调 runtime |
| Skill activate `context: fork` | ✅ 能 | fork 走 subagent，subagent 跑 LLM 不需 sandbox |
| Subagent (`Task` 工具) | ✅ 能 | 子 LLM loop 不需 sandbox |
| Catalog | ✅ 能 | 纯逻辑 + LLM |
| Conversation env 自动建（Bash auto-route）| ❌ 报错 | EnsureEnv fail |

### UI 表现

```
┌─ Forgify Banner（顶部） ────────────────────────────┐
│ ⚠️ Sandbox 未就绪：mise binary 解压失败             │
│   reason: permission denied: ~/.forgify/sandbox/    │
│                       [重试 bootstrap] [详情]       │
└─────────────────────────────────────────────────────┘
```

- Plugin marketplace / forge UI 上对应卡片置灰 + tooltip "Sandbox unavailable — see banner"
- 所有需 runtime 的 system tool 调用返友好错让 LLM 看到："Sandbox is not initialized; this operation requires runtime support which is currently unavailable."

### Recovery 路径

| 触发 | 行为 |
|---|---|
| 用户点 "重试 bootstrap" 按钮 | `RetryBootstrap(ctx)` → 再试一次 → 成功则 banner 消失，所有功能解锁 |
| 用户调 `POST /api/v1/sandbox:retry-bootstrap` | 同上（HTTP 端点）|
| App 重启 | Start 自然再试一次 |
| 自动重试 | **v1 不做**——bootstrap 失败一般是环境问题（权限/磁盘/杀软），重试无用，spam log 不友好 |

### 对应新增 HTTP 端点

| Method + Path | 用途 |
|---|---|
| `GET /api/v1/sandbox/bootstrap-status` | 当前状态：`{ok: true}` 或 `{ok: false, error: "permission denied: ..."}` |
| `POST /api/v1/sandbox:retry-bootstrap` | 触发重试，返新状态 |

---

## 16. 演化方向

- **GC 自动化**：v1 全 owner kind 默认 manual GC（uv/pnpm 共享让多 conv 磁盘开销极小）；v2 视用户反馈加 disk quota 触发 + cron 后台清
- **Conversation env disk quota**：极端用户场景下加"sandbox 总占用上限 X GB，超就拒新 install"（v1 不必，包管理器共享够顶）
- **Runtime 升级流程**：default Python 从 3.12 → 3.13 时怎么 migrate 已有 env？方案：保留旧 runtime + 新装的用新版 + 逐步 migrate（用户主动触发）
- **共享 deps 缓存**（v1 已就位）：uv hardlink + pnpm content-addressable store + Maven/Cargo/Go global cache——多 env 装同包磁盘 ≈ 1×。未来若加新语言，**EnvManager 实现要保证沿用该语言原生共享机制**
- **Disk quota**：UI 让用户设"sandbox 总占用上限 5GB"，超了拒绝新 install
- **远程 plugin 仓库**：未来 plugin manager 系统跑用户社区 plugin，全部走本 sandbox，与 forge/mcp/skill 平级
- **GPU runtime**：未来 ML 类 plugin 需要 CUDA/Metal——再加 RuntimeKind="cuda" 之类
- **WASM runtime**：浏览器外跑 WASM 模块，作为更轻量的 plugin 执行选项
- **Multi-Python-package-manager**：除 uv 外可选 pixi / poetry，作为不同 EnvManager 实现


---

## 17. 跨平台支持矩阵

### v1 平台清单

| 平台 | 状态 | mise binary embed | 限制 |
|---|---|---|---|
| **macOS arm64** (Apple Silicon, M1+) | ✅ 全功能 | ✅ embed | 无 |
| **macOS amd64** (Intel) | ✅ 全功能 | ✅ embed | 系统 ≥ 10.15 (Catalina) |
| **Linux amd64** (glibc：Ubuntu/Debian/Fedora/CentOS/RHEL) | ✅ 全功能 | ✅ embed | glibc ≥ 2.31 |
| **Linux arm64** (RPi 4+, AWS Graviton, glibc) | ✅ 全功能 | ✅ embed | 同上 |
| **Windows amd64** (10/11) | ⚠️ 限制版 | ✅ embed (mise.exe) | 长尾语言不支持，详下 |
| Linux musl (Alpine) | ❌ 不支持 | — | mise glibc binary，启动失败进 degraded |
| 32-bit / 其他架构 | ❌ 不支持 | — | mise 不 ship |
| FreeBSD / OpenBSD | ❌ 不支持 | — | mise 不 ship |

### Windows 限制矩阵

| 功能 | Windows v1 | 备注 |
|---|---|---|
| Chat / Forge / Skill / Catalog / Subagent | ✅ | 行为与 macOS/Linux 100% 一致 |
| 内置 system tools (Read/Write/Edit/Glob/Grep/WebFetch/WebSearch/Task/Ask/TaskCreate/...) | ✅ | 路径用 \，代码已 filepath.Join |
| MCP **Python 类**（sentry / figma / e2b / etc.）| ✅ | mise 装 Python 在 Windows OK |
| MCP **Node 类**（github / gitlab / playwright / context7 / etc.）| ✅ | mise 装 Node 在 Windows OK |
| MCP **其他语言 server**（Java / Ruby / PHP / Rust / Go / .NET / Erlang / Elixir / etc.）| ❌ | V3 collapse 后 sandbox 仅 Python + Node EnvManager；非 Python/Node 的 MCP server 暂不支持。未来恢复时按§7 ship list 模式扩 1 EnvManager 即可 |
| Bash tool | ⚠️ 改 | Windows 用 PowerShell 替代 sh；命令兼容性大部分一致；shell 差异 LLM 自适应 |
| Forge Python venv | ✅ | uv 跨平台原生 |
| 子进程 cancel | ⚠️ 改 | Windows Job Object 替代 SIGTERM（防 grandchild orphan）|

**核心**：**Python + Node 解决 99% 用户需求**，长尾在 Windows 上 "看不见" 即可。

### 跨平台代码统一原则

1. **路径**：全代码 `filepath.Join`，禁止字符串拼 `/`。
2. **可执行后缀**：`.exe` 在 Windows，POSIX 无后缀。helper：
   ```go
   func ExeName(base string) string {
       if runtime.GOOS == "windows" { return base + ".exe" }
       return base
   }
   ```
3. **进程取消**：跨平台用 `exec.CommandContext`，Windows 额外用 `golang.org/x/sys/windows` Job Object 包裹保 grandchild kill。
4. **文件锁**：`golang.org/x/sys/unix.Flock` + `golang.org/x/sys/windows.LockFileEx`，封装到 sandbox 内部 `fileLock` helper 跨平台一致 API。
5. **Shell auto-route**：Bash tool 内部按平台选 shell —— POSIX 用 `/bin/sh -c`，Windows 用 `powershell -NoProfile -Command`。detectRuntime 模式跨平台一致。
6. **路径长度**：Windows 默认 260 char 限——sandbox 路径设计避深嵌套，所有测试验证 path < 200 char。
7. **大小写敏感**：macOS HFS+ 默认不敏感，Linux/Windows-NTFS 敏感（NTFS 实际复杂）——代码不依赖大小写区分。
8. **行尾**：写文件强制 `
`，读文件用 normalize；不依赖平台默认。

### Build 策略：Per-Platform binary

每个 mise binary 文件用 build tag 拒绝其他平台 embed：

```go
//go:build darwin && arm64

package bootstrap

import _ "embed"

//go:embed binaries/mise-darwin-arm64
var Mise []byte
```

5 份 .go 文件（darwin-arm64 / darwin-amd64 / linux-amd64 / linux-arm64 / windows-amd64），每份对应 build tag——`go build` 自动只 include 当前平台。

**Build 命令**（Makefile / cmd/desktop / Wails）：

```bash
GOOS=darwin GOARCH=arm64 go build -o forgify-darwin-arm64 ./cmd/server
GOOS=darwin GOARCH=amd64 go build -o forgify-darwin-amd64 ./cmd/server
GOOS=linux GOARCH=amd64 go build -o forgify-linux-amd64 ./cmd/server
GOOS=linux GOARCH=arm64 go build -o forgify-linux-arm64 ./cmd/server
GOOS=windows GOARCH=amd64 go build -o forgify-windows-amd64.exe ./cmd/server
```

每平台 binary ~25 MB + ~10 MB embed mise = ~35 MB。

### Bootstrap 检测平台 + 友好失败

Sandbox.Start 跑 `mise --version` 验证：
- 成功 → bootstrapped=true
- 失败 → 进 degraded mode（详 §15.1）+ banner 显示具体平台限制（如 "Linux musl is not supported"）

### 跨平台测试

| 测试场景 | 平台 |
|---|---|
| Pipeline test | macOS arm64（CI 主力）|
| Smoke test | Linux amd64（CI 次要）|
| Manual test | Windows amd64（D14-D15 你自己 Windows 机器跑）|
| Skip test | Linux musl（标 `t.Skip("musl not supported")`）|
