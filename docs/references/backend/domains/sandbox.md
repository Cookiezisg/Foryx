---
id: DOC-118
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-07
review-due: 2026-09-01
audience: [human, ai]
---
# Sandbox Domain — 隔离运行时

> **核心职责**：Sandbox 是 Forgify 的**物理防线**。它在宿主机上为用户代码（function / handler / mcp / skill / 对话）构建隔离的 **Python / Node / .NET** 环境或 **Docker** 容器，确保 LLM 编写或拉取的代码被困在 per-owner 沙箱里、互不串扰。它是 Quadrinity 执行体的地基。

---

## 1. 四 runtime 统一模型

四种 runtime 复用同一套「`RuntimeInstaller`（装 + 定位）+ `EnvManager`（建 env + 装 deps + 组装执行命令）」双接口，按 kind 注册：

| 抽象层 | Python | Node | .NET (dotnet) | Docker |
|---|---|---|---|---|
| **Runtime**（全机共享） | mise 装 `python@3.12` | mise 装 `node@22` | mise 装 `dotnet@10` | `docker pull <image>`（镜像即 runtime） |
| **Env**（per-owner） | `uv venv` + `uv pip` | `package.json` + `npm install` | no-op（dnx 运行时拉包） | no-op（镜像即环境） |
| **Spawn** | venv binary / `uvx`(uv 同目录) | `node_modules/.bin/<cmd>` / `npx`(`<rt>/bin`) | `dnx`(`<rt>/dnx` 顶层) | `docker run --rm -i <image> <cmd>` |

**关键洞察**：把 Docker 镜像看作它的 "runtime"、容器看作 "env"，四者就能零特例共用 Runtime/Env manifest + owner 锁 + 幂等 Ensure 流程。`EnvManager.ResolveExec` 负责把用户的 cmd 翻译成宿主实际命令（env 内 binary / runtime 自带运行器 / `docker run` 包装），故 spawn 层不持 runtime 知识（详见 §1.1）。

> **runtime 矩阵依据**：GitHub MCP registry 99 个 server 调研——38 remote（SSE/HTTP，不吃本地 runtime）+ 38 node + 11 python + 9 docker + 3 .NET，Node+Python+Docker+dotnet 覆盖 99/99。故定四件套（dotnet 为 .NET-only server 兜底）。

### 1.1 ResolveExec：env 依赖 vs runtime 自带运行器

`ResolveExec` 按 cmd 形态三路分流，得到宿主实际命令：

| cmd 形态 | 解析目标 | 例 |
|---|---|---|
| **runtime 自带包运行器** | runtime install dir 下的运行器路径 | node `npx`/`npm`/`node`/`corepack` → `<runtimeRef>/bin/<cmd>`；python `uvx`/`uv` → uv 工具目录同级（`EnsureTool("uv")` 同目录）；dotnet `dnx`/`dotnet` → `<runtimeRef>/dnx`（**顶层、非 bin/**） |
| **裸名（env 依赖）** | per-owner env 内 | python → `<venv>/bin/<cmd>`；node → `node_modules/.bin/<cmd>` |
| **路径式 cmd** | 原样透传 | 绝对路径 / 含 `/` / `~` / `.` 前缀 |

**自带运行器是 MCP server 的标准启动方式**（`npx -y <pkg>` / `uvx <pkg>` / `dnx <pkg>`）：拉包即跑、不落 per-owner env。python 的 uv 本就是 env 后端（`uv venv` / `uv pip` 都经 `EnsureTool("uv")`，uv 经 aqua 装），故 `uvx` 随 uv 自带、`EnsureTool` 只是快速查找。dnx 经真机 `mise install dotnet@10.0.300` 验证落在 install dir 顶层。

**PATH 注入**：非-docker spawn 把 `<runtimeRef>/bin` + `<runtimeRef>` 前插进进程 PATH。**这是自带运行器的硬约束**——`npx` 的 shebang 是 `#!/usr/bin/env node`，运行时必须能在 PATH 找到 node 才能起；`dnx`（顶层）同理调 dotnet。对 venv binary / 绝对 cmd 无害。runtimeRef 为**绝对** install dir（`<sandboxRoot>/<rt.Path>`），使运行器能在其下定位。

---

## 2. 物理模型

### 2.1 `Runtime`（已装的 kind+version）
全机共享解释器 / 镜像，`UNIQUE(kind, version)`。**无 workspace_id**——解释器/镜像是机器级资源，按 workspace 复制只会浪费。
```go
type Runtime struct {
    ID, Kind, Version, Path string // path: python/node/dotnet 为相对 sandboxRoot 的 install 目录；docker 为镜像 ref
    SizeBytes               int64
    InstalledAt, UpdatedAt  time.Time
}
```

### 2.2 `Env`（per-owner 隔离环境）
绑定一个 Runtime 的 venv / node_modules 目录，或对 Docker 镜像的逻辑绑定。`UNIQUE(owner_kind, owner_id)`，`owner_kind ∈ {function,handler,mcp,skill,conversation,attachment}`。前五种是 per-entity 跑用户代码；**attachment** 是一个固定共享 owner（`id="extractor"`）的文档抽取 env（pdfplumber / python-docx … 工具链，跨 workspace 复用），R0053 加。**无 workspace_id**——通过全局唯一的 owner id（`fn_xxx` / `mcp` owner / attachment 固定 owner）间接按 workspace 隔离（attachment env 无用户数据故全机共享）。
```go
type Env struct {
    ID, OwnerKind, OwnerID, OwnerName, RuntimeID, Path string
    Deps                            []string
    SizeBytes                       int64
    Status                          string // installing | ready | failed
    ErrorMsg                        string
    RunningPID                      int    // >0 = 长生命周期进程；启动扫描杀残留
    CreatedAt, LastUsedAt, UpdatedAt time.Time
}
```

**workspace 隔离例外**：sandbox 两表系统级、磁盘 `~/.forgify/sandbox/` 不分桶——这是相对 memory/skills（per-ws 文本资源、按 workspace 分桶）的**合理例外**，因为 runtime 本就该跨 workspace 共享（装一份所有 ws 共用），env 跟随共享 runtime。行**物理删除**（无软删墓碑）。

---

## 3. 核心机制

1. **Embedded-mise bootstrap**：按平台 `go:embed` 内置 mise 二进制（`make fetch-mise` 生成、git 不入库）→ 启动时 SHA256 钉死解压到 `<root>/bin/mise`，写 `mise.toml` 关掉所有 attestation（避 GitHub 限流），macOS 剥 `com.apple.provenance` + ad-hoc 签名。失败进 **degraded mode**（不崩，`:retry-bootstrap`）。
2. **双接口 + 按 kind 注册**：`MiseInstaller`（通用 mise 插件）→ python/node/dotnet/uv；`PythonEnvManager`（uv）/`NodeEnvManager`（npm）/`DotnetEnvManager`（dnx，env 为 no-op）/`DockerEnvManager`（docker run）。Docker 的 Installer = 探测 daemon + pull。四套 Installer/EnvManager 的注册（填 `Service` 的 manager 表）由 boot 装配负责。
3. **Ensure 幂等懒装**：per-kind / per-owner 锁 + double-check；env deps 漂移 → 销毁重建。
4. **Spawn / SpawnLongLived**：一次性命令 vs 常驻进程；独立进程组（`Setpgid`）+ 取消时 `kill(-pid)` 杀整组；LongLived 返 handle，追踪 + Shutdown 全杀。非-docker spawn 把 runtime install dir（`<rt>/bin` + `<rt>`）前插 PATH，使自带运行器（npx/dnx）能找到解释器（详见 §1.1）。
5. **owner.ID 即 PATH 段**：owner.ID 直接当目录名并进 PATH，**强校验拒 `:;= \t\n\r\x00`**；conversation 用 `<convID>_<kind>` 下划线避冒号。
6. **RunningPID 僵尸扫描**：常驻进程 PID 写进 manifest，启动时扫残留并杀（防上次崩溃留僵尸）。
7. **GC + 磁盘审计**：`LastUsedAt` 超期（默认 30 天）的 env 物理删除；`TotalSizeBytes` 汇总占用。

---

## 4. 生命周期

1. **Bootstrap**：解压 mise，degraded 兜底，顺带 boot 扫残留进程。
2. **EnsureRuntime**：缺则装（mise install / docker pull），manifest 记账。
3. **EnsureEnv**：建 venv + 装 deps（docker no-op），status `installing → ready/failed`，发 `sandbox.env_status_changed`。
4. **Spawn**：owner → env → `ResolveExec` 组装宿主命令 → 跑。docker 走 image entrypoint，**空 Cmd 合法**；其余 runtime 空 Cmd 拒（`ErrCmdRequired`）。
5. **GC / Destroy**：超期或显式销毁，发 `sandbox.env_deleted`。

---

## 5. Docker 的当前边界

已实装：探测 daemon + `docker pull` + env manifest + 基础 `docker run` spawn（`opts.Env` 经 `-e` 注入容器）。docker 是唯一 `rt.Path` 为镜像 ref（非 install dir）的 runtime——故**豁免绝对-runtimeRef 拼接、豁免 PATH 注入、豁免空-Cmd 拒绝**（docker MCP server 经 image entrypoint 运行、无命令）。**容器优雅停止（`docker stop` + container-id 追踪）、孤儿回收、docker 型 MCP 长连接 e2e 仍待后续**——那才是 docker spawn 真正被消费验证处。Forgify **不能代装 docker**（需 root/admin），故探测不可达时优雅返 `ErrDockerNotInstalled`/`ErrDockerDaemonDown`。

> stdio transport 链已在真机端到端验证（node 路径）：embed mise → 装 node → `ResolveExec` 将 `npx` 解析为 `<runtime>/bin/npx` → `SpawnLongLived`（PATH 注入 node）→ MCP `Initialize`+`ListTools` 拉起 context7。e2e 用 build tag `e2e`，仅真机跑、离线 CI 不跑。

---

## 6. 错误字典

| Sentinel | Kind | Wire Code | HTTP |
|---|---|---|---|
| `ErrRuntimeNotSupported` | Unprocessable | `SANDBOX_RUNTIME_NOT_SUPPORTED` | 422 |
| `ErrRuntimeInstallFailed` | BadGateway | `SANDBOX_RUNTIME_INSTALL_FAILED` | 502 |
| `ErrRuntimeNotFound` | NotFound | `SANDBOX_RUNTIME_NOT_FOUND` | 404 |
| `ErrEnvNotFound` | NotFound | `SANDBOX_ENV_NOT_FOUND` | 404 |
| `ErrEnvCreateFailed` | BadGateway | `SANDBOX_ENV_CREATE_FAILED` | 502 |
| `ErrDepInstallFailed` | BadGateway | `SANDBOX_DEP_INSTALL_FAILED` | 502 |
| `ErrSpawnFailed` | BadGateway | `SANDBOX_SPAWN_FAILED` | 502 |
| `ErrSpawnTimeout` | GatewayTimeout | `SANDBOX_SPAWN_TIMEOUT` | 504 |
| `ErrEnvInUse` | Conflict | `SANDBOX_ENV_IN_USE` | 409 |
| `ErrInvalidOwnerID` | Invalid | `SANDBOX_INVALID_OWNER_ID` | 400 |
| `ErrCmdRequired` | Invalid | `SANDBOX_CMD_REQUIRED` | 400 |
| `ErrDockerNotInstalled` | Unprocessable | `SANDBOX_DOCKER_NOT_INSTALLED` | 422 |
| `ErrDockerDaemonDown` | Unavailable | `SANDBOX_DOCKER_DAEMON_DOWN` | 503 |

---

## 7. 跨域集成

- **Function / Handler**：依赖 sandbox 提供 Python/Node 解释器 + venv 执行（一次性 `Spawn` / 常驻 `SpawnLongLived`）。
- **MCP**：stdio 型 server 经 `SpawnLongLived` 起常驻进程（`ResolveExec` 解析自带运行器 `npx`/`uvx`/`dnx`，进程归 sandbox 管、go-sdk 经 stdin/stdout 接协议）；docker 镜像型 server 经 sandbox 拉取 + `docker run`。
- **Chat**：对话级 scratch env，对话删除时销毁。
- **Notification**：env 状态变更经 `notification.Emitter` 发到通知中心（`sandbox.env_status_changed` / `sandbox.env_deleted`）。
