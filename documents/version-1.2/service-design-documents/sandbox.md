# Sandbox — V1.2 详设计（PluginSandbox v2 统一架构）

**Phase**：Phase 4 准备件（提前到位，与 mcp/skill/forge 整合）
**状态**：📐 设计完成（2026-05-05）— 待实施
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/database-design.md`](../service-contract-documents/database-design.md) — `sandbox_runtimes` + `sandbox_envs` 两表（待加）
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — sandbox ×8（待加）
- [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) — install 进度通过 chat.message tool_call/tool_result 走，不新增事件
- 关联设计：[`forge.md`](./forge.md)（现有 forge sandbox 升级为本设计的 first consumer）/ [`mcp.md`](./mcp.md)（MCP server install 走本服务）/ [`skill.md`](./skill.md)（未来 skill 带 deps 时复用）

---

## 1. 一句话

**统一的 PluginRuntime 抽象**——给 forge / mcp / skill / **每个对话**（agent scratch）/ 未来任何 plugin 提供"安装 runtime → 建独立 env → 装 deps → spawn 子进程"的一站式服务。**Bootstrap 仅 mise + 几个 helper 脚本（~10 MB）**，所有语言运行时（Python / Node / Rust / Java / Go / Ruby / PHP / 长尾）lazy install 到 `~/.forgify/sandbox/runtimes/`，每个 plugin 实例的依赖隔离到 `~/.forgify/sandbox/envs/`。

**Bash 自动路由**：LLM 通过 Bash tool 跑 `pip install pandas` / `python script.py` / `npm install` 等命令时，sandbox 检测命令意图，**自动路由到该对话的 scratch env**——LLM 完全不知道沙箱存在，但所有动作都被收口（不污染用户系统、不跨对话扩散）。详见 §9.6。

---

## 2. 端到端推演

### 启动期

```
main.go → sandboxapp.NewService(deps).Start(ctx)
  → 加载 ~/.forgify/sandbox/manifest.json 等价 SQLite 表（sandbox_runtimes + sandbox_envs）
  → 检测 bootstrap 是否就位（~/.forgify/sandbox/bootstrap/mise 等）
      ✅ 在 → 继续
      ❌ 缺 → 从 embed.FS 解出 bundled binary 到 bootstrap/
  → 准备好接 plugin 调用
```

**注意：启动时不预装任何 runtime 或 env**——什么都不做，等第一个 plugin 来要。

### 运行期 — 用户装 Playwright MCP

```
mcpapp.InstallFromRegistry(ctx, "playwright")
  ↓
sandboxapp.EnsureEnv(ctx, Owner{Kind:"mcp", ID:"playwright"}, EnvSpec{
  Runtime: RuntimeSpec{Kind:"node", Version:""},   // "" = 用 default
  Deps:    []string{"@playwright/mcp"},
  Extras:  []string{"browsers/chromium"},
})
  ↓
Step 1: EnsureRuntime("node", "")
  → 查 sandbox_runtimes 表：node 类是否有 default 行
  → 没有 → 跑 bootstrap/mise install node@22  → 等 ~50 MB 下载
  → 写一行 sandbox_runtimes：kind=node, version=22.5.0, path=runtimes/node/22.5.0, isDefault=true
  → 推 SSE 进度（"Installing Node.js 22.5.0..."）
  ↓
Step 2: 创建 envs/mcp/playwright/
  → 写 package.json with @playwright/mcp
  → 跑 <node>/pnpm install --prefix=envs/mcp/playwright（共享 global store）
  → 推 SSE 进度（"Installing @playwright/mcp..."）
  ↓
Step 3: 处理 Extras: browsers/chromium
  → 跑 envs/mcp/playwright/node_modules/.bin/playwright install chromium
  → 推 SSE 进度（"Downloading Chromium browser (~150MB)..."）
  → 若 chromium 已装（共享 PLAYWRIGHT_BROWSERS_PATH=runtimes/browsers/）跳过
  ↓
Step 4: 写一行 sandbox_envs：
  ownerKind=mcp, ownerID=playwright, runtimeID=<node-22.5.0 row id>,
  deps=["@playwright/mcp"], extras=["browsers/chromium"],
  path=envs/mcp/playwright, sizeBytes=...
  ↓
Step 5: 返 EnvHandle 给 mcpapp.Service
  → mcpapp 拿 EnvHandle.Spawn(...) 起 MCP server 子进程
```

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
mcpapp.Connect(ctx, "playwright")
  ↓
sandboxapp.Spawn(ctx, Owner{Kind:"mcp", ID:"playwright"}, SpawnOpts{
  Cmd: "node",
  Args: []string{"node_modules/@playwright/mcp/dist/index.js"},
  Env: map[string]string{
    "PLAYWRIGHT_BROWSERS_PATH": runtimes/browsers/  // 强制本地化
  },
  LongLived: true,
})
  ↓
Service 用 envs/mcp/playwright/.runtime/node/bin/node 跑
  → 返 LongLivedHandle{Stdin, Stdout, Stderr, Wait, Kill}
  → mcpapp 用这个 handle 走 JSON-RPC over stdio
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
    Kind        string         `gorm:"not null;type:text;index:idx_sr_kind_def,priority:1" json:"kind"`     // "python" / "node" / "rust" / "java" / "browsers" / ...
    Version     string         `gorm:"not null;type:text" json:"version"`            // "3.12.5" / "22.5.0" / "stable" / "chromium-1234"
    Path        string         `gorm:"not null;type:text" json:"path"`               // 相对 ~/.forgify/sandbox/runtimes/，如 "python/3.12.5"
    SizeBytes   int64          `json:"sizeBytes"`
    IsDefault   bool           `gorm:"index:idx_sr_kind_def,priority:2" json:"isDefault"` // 该 kind 的"默认那个"
    InstalledAt time.Time      `json:"installedAt"`
    UpdatedAt   time.Time      `json:"updatedAt"`
}

func (Runtime) TableName() string { return "sandbox_runtimes" }
```

复合 UNIQUE：`(kind, version)`——同 kind 下版本号唯一。
复合索引 `idx_sr_kind_def`：按 kind 找 default 的那个。

### Env（持久化，同文件）

```go
type Env struct {
    ID          string         `gorm:"primaryKey;type:text" json:"id"`              // se_<16hex>
    OwnerKind   string         `gorm:"not null;type:text;index:idx_se_owner,priority:1" json:"ownerKind"` // "forge" / "mcp" / "skill" / "conversation"
    OwnerID     string         `gorm:"not null;type:text;index:idx_se_owner,priority:2" json:"ownerID"`   // f_xxx / "playwright" / etc.
    OwnerName   string         `gorm:"type:text" json:"ownerName,omitempty"`        // UI display
    RuntimeID   string         `gorm:"not null;type:text;index" json:"runtimeId"`   // FK → sandbox_runtimes.id
    Deps        []string       `gorm:"serializer:json" json:"deps"`                 // ["pandas==2.0", "numpy"]
    Extras      []string       `gorm:"serializer:json" json:"extras,omitempty"`     // ["browsers/chromium"]
    Path        string         `gorm:"not null;type:text" json:"path"`              // 相对 envs/，如 "mcp/playwright"
    SizeBytes   int64          `json:"sizeBytes"`
    Status      string         `gorm:"not null;type:text;default:ready" json:"status"` // "installing" / "ready" / "failed"
    ErrorMsg    string         `gorm:"type:text" json:"errorMsg,omitempty"`
    CreatedAt   time.Time      `json:"createdAt"`
    LastUsedAt  time.Time      `gorm:"index" json:"lastUsedAt"`                      // GC 用
    UpdatedAt   time.Time      `json:"updatedAt"`
}

func (Env) TableName() string { return "sandbox_envs" }
```

复合 UNIQUE：`(owner_kind, owner_id)`——一个 plugin 实例同时只能有一份 env（避免重复）。
复合索引 `idx_se_owner`：按 owner 查"我的 env 是哪份"。
索引 `runtime_id`：runtime GC 时反查"还有人用吗"。
索引 `last_used_at`：按时间 GC。

### Owner / Spec / Handle 等支持类型

```go
type Owner struct {
    Kind string  // "forge" / "mcp" / "skill" / "conversation" / 未来扩展
    ID   string  // 唯一标识，按 Kind 不同：
                 //   forge:        EnvID（同 deps 多 forge 版本共享）
                 //   mcp:          server 名（如 "playwright"）
                 //   skill:        skill 名
                 //   conversation: "<conv_id>:<runtime_kind>"，如 "cv_abc:python"
    Name string  // UI display 用
}

type RuntimeSpec struct {
    Kind    string  // "python" / "node" / "rust" / "java" / 任何 mise 支持的
    Version string  // "" = default；">=3.10" / "==3.10.5" / "stable" / etc.
}

type EnvSpec struct {
    Runtime  RuntimeSpec
    Deps     []string  // 包名（按 runtime 语言习俗：pip / npm / cargo / etc.）
    Extras   []string  // 额外 install 步骤的引用，如 "browsers/chromium"
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

### Sentinel 错误（10 个）

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
    // Docker-specific (added 2026-05-08 alongside Docker runtime support).
    // Forgify cannot install Docker for the user (system service); these
    // surface clear platform-specific install URLs to the caller.
    // Docker 专用（2026-05-08 与 Docker runtime 同期加）。Forgify 不替用户装
    // Docker（系统服务）；返清晰平台对应安装链接给调用方。
    ErrDockerNotInstalled = errors.New("sandbox: docker not installed")
    ErrDockerDaemonDown   = errors.New("sandbox: docker daemon not responding")
)
```

---

## 5b. Docker runtime（2026-05-08 加，准备 marketplace V2 接入官方 MCP registry 中的 oci/docker package）

### 为什么 Docker 不像其他 runtime 那样能"装"

| 维度 | node / python / 等 mise 管的 | docker |
|---|---|---|
| 装 runtime | mise 拉 binary 到 ~/.local | **不能装**——系统服务 + 要 root/admin |
| 平台基础设施 | 跨平台一致 | Mac/Win 要 Docker Desktop（~1.2 GB GUI + 内嵌 Linux VM）；Linux 要 dockerd systemd |
| Forgify 能干啥 | 自动装 + 缓存 + 复用 | **只能探活**（`docker version --format {{.Server.Version}}`）+ 缺时返清晰安装链接 |

### DockerInstaller 行为

`Install()` 不真装——跑 `docker version` 探 daemon：
- CLI 不在 PATH → 返 `ErrDockerNotInstalled` + 平台对应安装链接（Mac/Win → Docker Desktop；Linux → Docker Engine + `usermod -aG docker $USER`）
- CLI 在但 daemon 不响应 → 返 `ErrDockerDaemonDown` + 平台对应启动指令（Mac/Win → 启 Docker Desktop；Linux → `sudo systemctl start docker`）
- 都 OK → 写 marker 文件到 `<sandboxRoot>/docker-marker` 记 server 版本

`Locate()` 始终返 `"docker"`（系统 PATH）。`ListAvailable()` 返 nil（不枚举）。`ResolveDefault()` 返 ""（无版本概念）。

### DockerEnvManager 行为

- `CreateEnv(envPath)`：mkdir 一个 host 目录，将作容器 `/workspace` 的 bind 挂载源
- `InstallDeps([image])`：`docker pull <image>`；多 deps 取首项 + warn log（一容器一 image）
- `InstallExtras`：no-op
- `EnvBin()`：返 `"docker"`（系统 PATH）；真正的 `docker run -i --rm -v ...` 命令由调用方经 `BuildDockerRunArgs` helper 拼

### BuildDockerRunArgs（helper）

`mcp` adapter（marketplace V2 加进来）从 registry 装 docker package 时，调此 helper 拼 `ServerConfig.Args`：

```go
args := BuildDockerRunArgs(envPath, image, []string{"API_KEY=xxx"}, []string{"--db-path", "/workspace/db"})
// → ["run", "-i", "--rm", "-v", envPath+":/workspace", "-e", "API_KEY=xxx", image, "--db-path", "/workspace/db"]
```

### 安全 + 默认

- **bind 挂载**：仅 envPath（per-server 隔离）；**绝不**自动挂 host home/root 目录（避免 LLM 通过 docker MCP server 偷文件）
- **network**：默认 docker bridge（容器有外网，不能访问 host 内网）
- **资源限制**：V1 无默认 `--memory` / `--cpus`（信任 marketplace 精选条目）
- **容器生命周期**：`--rm` 自动清；`-i` 保持 stdin（stdio MCP transport）；进程结束 = 容器死
- **image 缓存**：拉到 docker daemon 系统级 cache，**永不自动删**；卸 server 不删 image（可能复用）；用户想清空跑 `docker image prune`

### 跨平台

- Mac/Win：通过 Docker Desktop 提供的 socket，自动找
- Linux：`/var/run/docker.sock`（用户加 docker group 才能不 sudo 跑）
- 国内镜像加速：用户在 Docker Desktop 设置里配（Forgify 不接管）

### 关键限制（**用户必须做一次性配置**）

1. **必须自己装 Docker Desktop / Docker Engine**——Forgify 给清晰链接但不替装
2. **Mac/Win 用 Docker Desktop 商业 license 注意**——>250 员工的公司付费

### 与 marketplace V2 的衔接（下一轮）

V2 marketplace adapter 处理"registry entry 是 docker package"时：
1. 调 `sandbox.EnsureEnv(owner=mcp/<alias>, spec={Runtime:{Kind:"docker"}, Deps:[image_ref]})`
2. 拿到 envPath
3. 用 `BuildDockerRunArgs(envPath, image, env, serverArgs)` 拼 ServerConfig.Args
4. 调 `mcp.AddServer(ServerConfig{Command:"docker", Args:<拼好的>, ...})`
5. mcp 内部 `exec.Command("docker", args...)` 起 stdio 容器作 MCP server

---

## 6. Repository 接口（`internal/domain/sandbox/sandbox.go`）

```go
type Repository interface {
    // Runtime CRUD
    CreateRuntime(ctx context.Context, r *Runtime) error
    GetRuntime(ctx context.Context, id string) (*Runtime, error)
    FindDefaultRuntime(ctx context.Context, kind string) (*Runtime, error)         // kind 默认那个
    FindRuntime(ctx context.Context, kind, version string) (*Runtime, error)        // 精确版本
    ListRuntimes(ctx context.Context) ([]*Runtime, error)
    DeleteRuntime(ctx context.Context, id string) error

    // Env CRUD
    CreateEnv(ctx context.Context, e *Env) error
    GetEnv(ctx context.Context, id string) (*Env, error)
    FindEnvByOwner(ctx context.Context, ownerKind, ownerID string) (*Env, error)
    ListEnvsByRuntime(ctx context.Context, runtimeID string) ([]*Env, error)        // GC 用
    ListEnvsByOwnerKind(ctx context.Context, ownerKind string) ([]*Env, error)      // UI 用
    UpdateEnv(ctx context.Context, e *Env) error
    DeleteEnv(ctx context.Context, id string) error

    // 聚合查询
    TotalSizeBytes(ctx context.Context) (int64, error)                              // UI 显示磁盘占用
    ListEnvsLastUsedBefore(ctx context.Context, t time.Time) ([]*Env, error)        // GC 候选
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

    // ListAvailable 给 UI 展示"我能装哪些版本"用（可选，返 nil 表示不支持枚举）
    ListAvailable(ctx context.Context) ([]string, error)

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

// EnvManager 知道怎么在 runtime 上建包隔离环境。需要支持工具的实现
// （Python/uv、Node/pnpm、Java/maven、Ruby/bundler、PHP/composer）构造时
// 接 ToolRegistry，操作时按需 EnsureTool 拿路径。无支持工具的实现
// （Rust、Go、Dotnet、Static、Generic、Playwright wrapper）构造无参数。
type EnvManager interface {
    Kind() string  // 同 RuntimeInstaller.Kind()

    // CreateEnv 在 envPath 建立独立 env（venv / node_modules / 等）
    // runtimePath 是 RuntimeInstaller.Locate 返的解释器目录
    CreateEnv(ctx context.Context, runtimePath, envPath string) error

    // InstallDeps 在 env 里装 deps（uv pip install / pnpm install / cargo install / etc.，统一走包管理器原生共享机制）
    InstallDeps(ctx context.Context, runtimePath, envPath string, deps []string, stream ProgressFunc) error

    // InstallExtras 跑额外 install 步骤（如 Playwright Chromium 下载）
    InstallExtras(ctx context.Context, runtimePath, envPath string, extras []string, stream ProgressFunc) error

    // EnvBin 返 env 内某 binary 的绝对路径（用于 Spawn）
    // 例：python kind + "python" → "<envPath>/.venv/bin/python"
    EnvBin(envPath, binName string) string

    // EnvDir 返 env 主目录（spawn 时 cwd 候选）
    EnvDir(envPath string) string
}
```

### v1 ship 的 Installer + EnvManager（全部 D2 实施，**无延后到 v2**）

§S12 平铺规则：所有实现按 `installer_<name>.go` / `envmanager_<name>.go` 命名，
直接放 `internal/infra/sandbox/` 下，不分子目录。

#### Installer

| Installer | Kind | 文件 |
|---|---|---|
| **mise generic** | `python` / `node` / `rust` / `java` / `go` / `ruby` / `php` / 其他长尾 | `installer_mise.go` —— 一个 struct 通配，按 kind 路由 |
| **Playwright** | `browsers` | `installer_playwright.go` —— 跑 `playwright install <browser>` |
| **dotnet** | `dotnet` | `installer_dotnet.go` —— 微软官方 `dotnet-install.sh` / `.ps1` 封装 |
| **static-binary** | `static` | `installer_static.go` —— 直接下载二进制（如 GitHub MCP）|

#### EnvManager（一种语言一个，因包管理器互不兼容）

| EnvManager | Kind | 文件 | 隔离机制 |
|---|---|---|---|
| **Python** | `python` | `envmanager_python.go` | `uv venv` + `uv pip install`（uv hardlink 全局 wheel cache） |
| **Node** | `node` | `envmanager_node.go` | `pnpm install --prefix=<env_path>`（pnpm content-addressable global store + symlink）|
| **Rust** | `rust` | `envmanager_rust.go` | `cargo install --root=<env_path>` + `CARGO_HOME=<env>/.cargo` |
| **Go** | `go` | `envmanager_go.go` | `GOPATH=<env_path>/gopath` + `go install` |
| **Java** | `java` | `envmanager_java.go` | **方案 A**：每 env 独立 Maven local repo，`MAVEN_OPTS=-Dmaven.repo.local=<env>/m2`。每 env 独立下载所有 jar，磁盘最大但隔离最干净——跟 venv 哲学一致 |
| **Ruby** | `ruby` | `envmanager_ruby.go` | Bundler `BUNDLE_PATH=<env>/bundle` |
| **PHP** | `php` | `envmanager_php.go` | Composer `--working-dir=<env>` |
| **Playwright** | `browsers` | `envmanager_playwright.go` | 委托给 Node 的 pnpm 装 playwright npm 包；二进制 chromium 路径独立 |
| **dotnet** | `dotnet` | `envmanager_dotnet.go` | `dotnet add package` + per-env `nuget.config` |
| **Static binary** | `static` | `envmanager_static.go` | 无包管理；env 仅持二进制 + 启动脚本 |
| **Generic fallback** | `*` | `envmanager_generic.go` | 兜底 EnvManager：仅 mkdir env 目录，让用户/LLM 在 cwd 自己跑包管理器。给 mise 长尾 600+ 语言（Erlang / Elixir / Lua / Zig / Deno / etc.）和未来 plugin 用 |

#### D2-3 实施顺序（5 子任务，按消费方/复杂度分组）

| 子任务 | 内容 | 主要消费方 |
|---|---|---|
| **D2-3a**（已完成）| MiseInstaller + PythonEnvManager | Forge / MarkItDown / DuckDuckGo MCP / Skill |
| **D2-3b** | NodeEnvManager + PlaywrightInstaller + PlaywrightEnvManager | Playwright MCP / Context7 MCP / conv |
| **D2-3c** | GenericEnvManager + StaticBinaryInstaller + StaticBinaryEnvManager | conv 长尾语言 + 未来纯静态二进制 plugin |
| **D2-3d** | RustEnvManager + GoEnvManager | conv `cargo build` / `go run` |
| **D2-3e** | JavaEnvManager + RubyEnvManager + PHPEnvManager | conv 三种传统语言（流程相似） |
| **D2-3f** | DotnetInstaller + DotnetEnvManager | conv .NET |

**Java EnvManager 决策**：选**方案 A**（每 env 独立 Maven local repo），代价是磁盘大但 demo 阶段不显著。pnpm/uv 共享缓存的优雅在 Maven 上做要重写 jar 解析，不值——v2 视用户反馈再优化。

### main.go 装配示例

```go
// main.go
sandboxService := sandboxapp.New(repo, dataDir, log)
if err := sandboxService.Bootstrap(ctx); err != nil {
    log.Warn("sandbox degraded mode", zap.Error(err)) // 不致命
}
miseBin := sandboxService.MiseBin()

// 1. RuntimeInstaller —— 7 种主流 mise 通配 + 4 种支持工具（uv/pnpm/maven/
//    bundler/composer 也是 mise 装的 runtime）+ Playwright/Static/Dotnet 专用
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "python", "3.12"))
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "node", "22"))
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "rust", "stable"))
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "java", "21"))
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "go", "1.22"))
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "ruby", "3.3"))
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "php", "8.3"))
// 支持工具（用 mise 装但不直接给 plugin 用，给 EnvManager 用）
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "uv", ""))
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "pnpm", ""))
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "maven", ""))
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "bundler", ""))
sandboxService.RegisterInstaller(sandboxinfra.NewMiseInstaller(miseBin, "composer", ""))
// 专用 installer
sandboxService.RegisterInstaller(sandboxinfra.NewPlaywrightInstaller(/* playwright cli path */))
sandboxService.RegisterInstaller(sandboxinfra.NewDotnetInstaller("8.0"))
sandboxService.RegisterInstaller(sandboxinfra.NewStaticBinaryInstaller("github-mcp", log))

// 2. EnvManager —— 需要支持工具的传 sandboxService 作 ToolRegistry；
//    无支持工具的（rust/go/dotnet/static/generic/playwright wrapper）直接构造
sandboxService.RegisterEnvManager(sandboxinfra.NewPythonEnvManager(sandboxService))
sandboxService.RegisterEnvManager(sandboxinfra.NewNodeEnvManager(sandboxService))
sandboxService.RegisterEnvManager(sandboxinfra.NewRustEnvManager())
sandboxService.RegisterEnvManager(sandboxinfra.NewGoEnvManager())
sandboxService.RegisterEnvManager(sandboxinfra.NewJavaEnvManager(sandboxService))
sandboxService.RegisterEnvManager(sandboxinfra.NewRubyEnvManager(sandboxService))
sandboxService.RegisterEnvManager(sandboxinfra.NewPHPEnvManager(sandboxService))
sandboxService.RegisterEnvManager(sandboxinfra.NewDotnetEnvManager())
sandboxService.RegisterEnvManager(sandboxinfra.NewStaticBinaryEnvManager("github-mcp", sandboxService.SandboxRoot()))
sandboxService.RegisterEnvManager(sandboxinfra.NewPlaywrightEnvManager(
    sandboxinfra.NewNodeEnvManager(sandboxService), sandboxService.SandboxRoot()))
// 长尾语言（Erlang / Elixir / Lua / Zig / Deno / etc.）：MiseInstaller 已通配
// 装 runtime；env 走 GenericEnvManager 兜底（mkdir + 让 LLM/用户在 cwd 自己跑
// 该语言包管理器）。按需注册：
sandboxService.RegisterEnvManager(sandboxinfra.NewGenericEnvManager("elixir"))
sandboxService.RegisterEnvManager(sandboxinfra.NewGenericEnvManager("zig"))
// ... 仅当深度集成（写专用 EnvManager 类似 Python/Node）才用专门实现
```

**注**：构造 EnvManager 不触发任何装机——支持工具（uv/pnpm/etc.）首次
 `EnsureEnv` 调 `tools.EnsureTool(ctx, "uv", "")` 时才装。boot 极快。

---

## 8. Service 层（`internal/app/sandbox/sandbox.go`）

```go
type Service struct {
    repo         sandboxdomain.Repository
    installers   map[string]sandboxdomain.RuntimeInstaller
    envManagers  map[string]sandboxdomain.EnvManager
    bootstrapDir string  // ~/.forgify/sandbox/bootstrap/
    runtimesDir  string  // ~/.forgify/sandbox/runtimes/
    envsDir      string  // ~/.forgify/sandbox/envs/
    log          *zap.Logger
    mu           sync.Mutex  // 保护 RegisterInstaller 启停 + install 锁
    installLocks sync.Map    // map[runtimeKind]*sync.Mutex 防并发同 runtime install
}

// 装配（main.go 调）。Installer 与 EnvManager 分两次注册——一种 runtime kind
// 可有 installer 但无 EnvManager（如支持工具 uv/pnpm/maven 只给其他 EnvManager
// 用，自己不当 plugin runtime）。
func (s *Service) RegisterInstaller(i RuntimeInstaller)
func (s *Service) RegisterEnvManager(m EnvManager)

// EnsureTool 实现 sandboxdomain.ToolRegistry——给 EnvManager 懒解析支持工具
// 路径用。内部链 EnsureRuntime + Installer.Locate。
func (s *Service) EnsureTool(ctx context.Context, kind, version string) (binPath string, err error)

// EnsureRuntime 保证某 runtime 装好；返 Runtime 行
func (s *Service) EnsureRuntime(ctx context.Context, spec RuntimeSpec, stream ProgressFunc) (*Runtime, error)

// EnsureEnv 为某 plugin 实例建独立 env（含装 runtime + 装 deps + 装 extras）
func (s *Service) EnsureEnv(ctx context.Context, owner Owner, spec EnvSpec, stream ProgressFunc) (*Env, error)

// Spawn 在 env 里起子进程（一次性 / 长生命周期）
// owner 必须已 EnsureEnv 过
func (s *Service) Spawn(ctx context.Context, owner Owner, opts SpawnOpts) (*ExecutionResult, error)
func (s *Service) SpawnLongLived(ctx context.Context, owner Owner, opts SpawnOpts) (LongLivedHandle, error)

// Destroy 删 env（plugin 卸载时调用）
func (s *Service) Destroy(ctx context.Context, owner Owner) error

// 查询 / 管理
func (s *Service) ListRuntimes(ctx context.Context) ([]*Runtime, error)
func (s *Service) ListEnvs(ctx context.Context, ownerKind string) ([]*Env, error)
func (s *Service) TotalDiskUsage(ctx context.Context) (int64, error)
func (s *Service) GCEnvs(ctx context.Context, olderThan time.Duration) (int, error)  // 清理超 N 天未用
```

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
        IsDefault:   spec.Version == "",  // 没指定版本视为 default
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
    envPath := filepath.Join(s.envsDir, owner.Kind, owner.ID)  // envs/mcp/playwright/
    
    // 4. 先写"installing"状态（让 UI 知道在装）
    env := &Env{
        ID: envID, OwnerKind: owner.Kind, OwnerID: owner.ID, OwnerName: owner.Name,
        RuntimeID: runtime.ID, Deps: spec.Deps, Extras: spec.Extras,
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
    if len(spec.Extras) > 0 {
        if err := em.InstallExtras(ctx, runtimePath, envPath, spec.Extras, stream); err != nil {
            env.Status = "failed"
            env.ErrorMsg = err.Error()
            s.repo.UpdateEnv(ctx, env)
            return nil, fmt.Errorf("sandboxapp.EnsureEnv extras: %w", err)
        }
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

| 语言 | env 内文件 | spawn 时强制本地化的机制 |
|---|---|---|
| **Python** | `.venv/bin/python` + `.venv/lib/python3.x/site-packages/` | venv shim python 的 `sys.path` 钉死本 venv |
| **Node** | `node_modules/<package>/` | Node `require()` resolution 算法找最近 `node_modules`（cwd 上溯）|
| **Rust** | `target/` + `Cargo.lock` | `cargo --target-dir=<env_path>/target` |
| **Java** | `lib/*.jar` | classpath 显式指 `<env_path>/lib/*` |
| **Go** | `pkg/` | env var `GOPATH=<env_path>` + `GOMODCACHE=<env_path>/cache` |
| **Ruby** | `vendor/bundle/` | env var `BUNDLE_PATH=<env_path>/vendor/bundle` |
| **PHP** | `vendor/` | Composer 自动找 cwd 的 vendor |
| **Browsers** | `browsers/chromium/` | env var `PLAYWRIGHT_BROWSERS_PATH=<runtimes_path>/browsers` |

### 9.3 关键约束："永远不装到全局"

- **禁止** `pip install --user`
- **禁止** `npm install -g` / `pnpm add -g`
- **禁止** `cargo install` 不带 `--root`
- **禁止** `gem install` 不带 `BUNDLE_PATH`

所有 EnvManager 的 InstallDeps 实现**必须**走 prefix/local 模式，绝不污染全局环境。

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

**Playwright MCP** 装 `playwright-core@1.40`，**Context7 MCP** 间接依赖 `playwright-core@1.50`：

```
envs/mcp/playwright/node_modules/playwright-core/   ← 1.40
envs/mcp/context7/node_modules/playwright-core/     ← 1.50
```

Spawn 各自时 `cwd` 是各自 env 目录，Node `require()` 只找本目录 node_modules。零冲突。

### 9.5 Bash 自动路由 + 对话 scratch env（核心收口机制）

**问题**：LLM 通过 Bash tool 跑 `pip install pandas` 或 `python script.py`——如果 Bash 直接走系统 PATH，会污染用户系统 Python，且不可控。

**简单方案（已废弃）**：在 Bash tool 加 denylist 拦截 `pip install`、`python` 等命令——告诉 LLM 用 forge tool。**问题**：denylist 不可能穷举（`/usr/bin/python3` / `python3.12` / `pipx` / `python -m pip` ...）；且对 LLM "我就想试个小脚本"的合理需求过于严苛。

**采用方案**：**Bash 自动路由到 conversation scratch env**——sandbox 检测命令意图，把执行环境替换为该对话的 scratch env。LLM 自由 install/run，**所有副作用都被收口到沙箱里**。

#### Conversation env 概念

每个对话按需为每种 runtime 起一个独立 scratch env：

```
~/.forgify/sandbox/envs/conversation/
├── cv_abc:python/        ← cv_abc 用了 Python
├── cv_abc:node/          ← cv_abc 也用了 Node
├── cv_xyz:python/        ← cv_xyz 只用了 Python
└── cv_some:rust/         ← 另一对话用了 Rust
```

`(conversation_id, runtime_kind)` 一对一，**绝不会两个对话共享**。

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
            ID:   fmt.Sprintf("%s:%s", convID, runtimeKind),
            Name: fmt.Sprintf("Conv %s scratch (%s)", convID, runtimeKind),
        }
        env, err := b.sandbox.EnsureEnv(ctx, owner, sandboxdomain.EnvSpec{
            Runtime: sandboxdomain.RuntimeSpec{Kind: runtimeKind},
            Deps:    nil,  // 空 env，让 LLM 自由装
        }, progressFn)
        if err != nil { return "", err }
        
        // Spawn 时 PATH 加上 conversation env 的 bin 优先
        return b.sandbox.SpawnShell(ctx, owner, cmd)
    }
    
    // 非 runtime 命令（git/ls/cat 等）→ 普通 shell（仅 /usr/bin /bin PATH）
    return b.runShellPlain(ctx, cmd)
}
```

#### detectRuntime — 命令到 runtime 的映射

实现走 **AST 解析**（`mvdan.cc/sh/v3/syntax`）而非 first-token regex——`shfmt` 的同一 parser，pure Go，跨平台 0 依赖。流程：parse 命令 → `syntax.Walk` 遍历每个 `CallExpr` → 对每个 call 经 `classifyCallExpr`（剥路径前缀 / 处理 wrapper / env / which）→ 匹配下面的 runtime 表，**首次命中胜**。pattern 匹配的是规范化后的*裸命令名*（无路径、无 env 前缀、无 flag）：

```go
var runtimeDetectors = []runtimeDetector{
    {Kind: "python", Pattern: regexp.MustCompile(`^(?:python3?(?:\.\d+)?|pip3?|uv|virtualenv|pipenv|poetry)$`)},
    {Kind: "node",   Pattern: regexp.MustCompile(`^(?:node|npm|npx|yarn|pnpm)$`)},
    {Kind: "rust",   Pattern: regexp.MustCompile(`^(?:cargo|rustc|rustup)$`)},
    {Kind: "go",     Pattern: regexp.MustCompile(`^go$`)},
    {Kind: "ruby",   Pattern: regexp.MustCompile(`^(?:ruby|gem|bundle|bundler|rake)$`)},
    {Kind: "php",    Pattern: regexp.MustCompile(`^(?:php|composer)$`)},
    {Kind: "java",   Pattern: regexp.MustCompile(`^(?:java|javac|mvn|gradle)$`)},
    {Kind: "dotnet", Pattern: regexp.MustCompile(`^dotnet$`)},
    // 未来加新 runtime 1 行
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

#### SpawnShell 实现

```go
// SpawnShell 在 env 里跑 shell 命令，PATH 自动加 env bin 在前
func (s *Service) SpawnShell(ctx context.Context, owner Owner, command string) (*ExecutionResult, error) {
    env, err := s.repo.FindEnvByOwner(ctx, owner.Kind, owner.ID)
    if err != nil { return nil, err }
    
    em := s.envManagers[env.Runtime.Kind]
    binDirs := em.EnvBinDirs(env.Path)  // venv/bin / node_modules/.bin / etc.
    
    augmentedPath := strings.Join(binDirs, ":") + ":/usr/bin:/bin"
    
    cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
    cmd.Env = append(os.Environ(), "PATH="+augmentedPath)
    cmd.Dir = s.envsDir + "/" + env.Path  // cwd 在 env 目录
    
    // ...spawn + capture output...
}
```

#### LLM 视角的体验

```
LLM: "我想用 pandas 处理 CSV"
LLM: Bash("pip install pandas")
  ↓ sandbox 检测：pip → python kind
  ↓ EnsureEnv(owner=conversation/cv_abc:python)
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
| Playwright Chromium cache | 默认 `~/.cache/ms-playwright` | spawn 时强制 `PLAYWRIGHT_BROWSERS_PATH=<runtimes>/browsers` |

---

## 10. SSE 事件 — 不新增

sandbox install 进度**通过调用方的 chat.message 工具结果机制推**——比如 mcp install 是 LLM 调 `mcp_install` system tool 触发，进度 stream 体现为该 tool_call block 的 stdout 累积。**sandbox 不发独立 SSE 事件**。

ProgressFunc 由调用方传入，sandbox 只负责 invoke 它：

```go
// mcpapp 调 sandbox 时：
progressFn := func(stage, message string, percent int) {
    // 拼成 LLM-friendly 文本
    text := fmt.Sprintf("[%s] %s (%d%%)\n", stage, message, percent)
    // 这段会进 mcp_install tool 的 stdout，自然走 chat.message 流
    progressBuffer.WriteString(text)
    publishToolResultStreaming(progressBuffer.String())  // chat 层负责
}
sandboxService.EnsureEnv(ctx, owner, spec, progressFn)
```

---

## 11. HTTP API

### Sandbox 状态 / 管理（debug + UI）

| Method + Path | 用途 |
|---|---|
| `GET /api/v1/sandbox/runtimes` | 列所有已装 runtime（kind/version/size/谁在用） |
| `GET /api/v1/sandbox/envs?ownerKind=mcp` | 列指定 owner kind 的所有 env |
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

## 12. 错误码

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

---

## 13. 测试覆盖（计划）

| 层 | 文件 | 测试数（计划）| 覆盖 |
|---|---|---|---|
| domain | `internal/domain/sandbox/sandbox_test.go` | 5 | Runtime/Env JSON / Sentinel / Owner valid |
| store | `internal/infra/store/sandbox/sandbox_test.go` | 12 | CRUD + UNIQUE 约束 + ListByOwnerKind + ListByRuntime + GC 候选查询 |
| installer/mise | `internal/infra/sandbox/installer/mise/mise_test.go` | 10 | 多 kind 路由 / Install python / Install node / Locate / ListAvailable |
| installer/playwright | `internal/infra/sandbox/installer/playwright/playwright_test.go` | 5 | Install chromium 流程 / 用 fake mise / progress 推送 |
| envmanager 各语言 | 多文件 | ~22 | venv + uv pip install / pnpm install --prefix（验证 store 共享）/ 各种 Bundle/GOPATH/etc. + 跨 env hardlink/symlink 共享验证 |
| app/sandbox | `internal/app/sandbox/sandbox_test.go` | 18 | EnsureRuntime 锁防并发 / EnsureEnv 复用 / Destroy / Spawn timeout / GC 流程 / 失败 status 转 |
| pipeline | `test/sandbox/sandbox_test.go` | 6 | 真起 Python venv 装 markitdown / 真起 Node 装某 pnpm 包 / Spawn 真子进程 / Destroy 干净 / 多 plugin 隔离验证 / **多 conv 共装 pandas 验证 hardlink 共享生效**（du 测实际磁盘 ≈ 1×）|

总计 ~75 测 + 5 pipeline 场景。

**fake mise 注入点**：installer 里 mise binary 路径可注入；测试用 mock binary 模拟 install/locate 行为，避免真下载。

---

## 14. 与其他 domain 的关系

| 关系 | 说明 |
|---|---|
| **forge** | forgeapp 通过 `forgesandboxport.Sandbox` 接口（在 forge domain 定义）调本服务；现有 `infra/sandbox` 升级为本设计的 first consumer |
| **mcp** | mcpapp 装 server 时调 `sandboxapp.EnsureEnv`；spawn 时调 `SpawnLongLived` 拿 stdio handle 走 JSON-RPC |
| **skill** | v1 skill 不需 runtime；未来 skill 带 deps 时复用本服务 |
| **chat / Bash tool** | **app/tool/shell/Bash 改造**：detectRuntime + 自动路由到 conversation scratch env；非 runtime 命令走普通 shell |
| **conversation** | 软删/硬删 conversation 触发对应 scratch env 的标记可清 / 立即 Destroy（详 §9.5）|
| **events** | install 进度通过调用方 tool_call 输出走 chat.message，不发独立 sandbox SSE |

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
1. logger / DB / events bridge
2. apikey / model / conversation / chat / forge / task / ask（已有）
3. NEW: sandboxapp.Service.New(deps)
4. NEW: 注册 v1 ship 的 installers + env managers（10+ kind）
5. NEW: subagentapp / mcpapp / skillapp / catalogapp（新设计）
   - 各自 New() 时注入 sandboxapp.Service 作 PluginSandbox 接口
6. forge service 重构：注入 sandboxapp.Service 替代旧 infra/sandbox
7. 注册 system tools 到 chat
8. router / http listen
```

Bootstrap 阶段不阻塞——sandboxapp.Start 仅检查 bootstrap binary 是否在位（不在则从 embed.FS 解出），不预装任何 runtime。

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
func (s *Service) BootstrapStatus() (ok bool, err string)
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
| MCP **Python 类**（MarkItDown / DuckDuckGo / SQLite）| ✅ | mise 装 Python 在 Windows OK |
| MCP **Node 类**（Playwright / Context7 / everything）| ✅ | mise 装 Node 在 Windows OK |
| MCP **Java server**（v1 暂无内置 Java 写的 server；JDK 装 + Maven local repo 隔离基础设施已 v1 ready，等社区出 Java MCP server 即可启用）| ⚠️ 部分 | Adoptium Windows JDK 能装；mvn/gradle wrapper 路径可能有坑 |
| MCP **Ruby / PHP / Erlang / Elixir / Lua / Crystal / Zig / 长尾** | ❌ | mise 这些用 bash plugin，Windows 无 bash。RegistryEntry 标 `UnsupportedPlatforms: ["windows"]` → marketplace 在 Windows 隐藏 |
| Bash tool | ⚠️ 改 | Windows 用 PowerShell 替代 sh；命令兼容性大部分一致；shell 差异 LLM 自适应 |
| Forge Python venv | ✅ | uv 跨平台原生 |
| Playwright Chromium | ✅ | 自动下 Windows 版 Chromium |
| 子进程 cancel | ⚠️ 改 | Windows Job Object 替代 SIGTERM（防 grandchild orphan）|
| fsnotify | ⚠️ 测 | ReadDirectoryChangesW 后端，行为略异；不支持网络盘 |

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
