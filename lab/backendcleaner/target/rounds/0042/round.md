# R0042 — M3.5 三处留口补做 + sandbox 物理 runtime-tool 真机跑通

> 波次 3 第三站收尾。R0041 mcp 重写逻辑完整、测试全绿，但留了 3 处「需后续/需真实环境」的留口（见 R0041 round.md §留口 + deps-todo）。本轮全补完，**重头戏是 sandbox 物理 runtime-tool 在真实机器上端到端跑通**——离线测不出来的 npx/uvx/dnx 真解析 + PATH 注入，靠一台 darwin/arm64 真机 e2e 把 context7 MCP server 真起来验证。

## 三留口补完

### 留口2 — handler catalog 列方法名（已 commit 071e8425）
catalog 渲染早支持 `Item.Members`（R0041 为 mcp 容器实体加的字段），但 handler source 之前没填。本轮 handler catalog_source 填 active version 的方法名：`repo.GetVersion(ActiveVersionID).Methods[].Name`（ListAll 不附 Methods，故 source 内逐查 active 版本）。handler 与 mcp 同款「容器实体」范式——catalog item 报实体名 + 把内部成员名（mcp=工具名 / handler=方法名）摊平给 LLM。测试 `TestCatalogSource_ListsMethodNames`。

### 留口3 — trigger sensor 绑 mcp.tool（已 commit 071e8425）
回改 trigger R0039：sensor target 从「function / handler」扩成第三类 **`SensorTargetMCP`**（`domain/trigger/config.go`）——`TargetID`=mcp server 名、`Method`=工具名。校验：mcp 与 handler 同样**要求 `Method` 非空**（function 整体触发、无 method）。relation 适配器 `syncSensorBinding` 加 mcp 分支 → trigger 指向 mcp server 的 **`equip` 出向边**（与 function/handler 同 `KindEquip`）。测试 `TestValidateConfig_SensorTargets`（三 target kind：function 无 method / handler 有 method / mcp 有 method）。

**留 M7 装配**：sensor 实际 Invoke 路由（`SensorInvoker` 实现）随 M7 装配——function/handler/mcp 三条路由现在**都还没接**（trigger 监听器有回调口子、实现体待装）。

## sandbox 物理 runtime-tool（5 点回改，本轮 commit 待提）

R0041 stdio transport 用 sandbox `SpawnLongLived` 起 MCP server 进程（go-sdk `IOTransport` 接协议），但 sandbox（R0026）的 `ResolveExec` 只认 env 内依赖（venv/node_modules），**不认 MCP server 的启动方式 `npx -y <pkg>` / `uvx <pkg>` / `dnx <pkg>`**——这些是 runtime **自带的包运行器**、不在 per-owner env 里。本轮补全：

1. **node `ResolveExec`**（`infra/sandbox/node.go`）：`isNodeRuntimeTool` 认 `npx/npm/node/corepack` → `runtimeToolPath` = `<runtimeRef>/bin/<cmd>`（runtimeRef 现在是**绝对** install dir）；其余裸名仍走 env 的 `node_modules/.bin`。
2. **python `ResolveExec`**（`infra/sandbox/python.go`）：认 `uvx/uv` → `tools.EnsureTool("uv")` 同目录（`filepath.Dir(uvBin)`，uvx 随 uv 同装）。**重要修正**：R0041 留口写「python uvx 需装 uv」是**错的**——sandbox 的 python 本就是 **uv-backed**（`CreateEnv` 跑 `uv venv`、`InstallDeps` 跑 `uv pip`，都经 `EnsureTool("uv")`，uv 经 aqua 装）。**uv 早就在**，uvx 随 uv 自带，`EnsureTool` 只是快速查找、不触发新安装。
3. **dotnet 新增**（`infra/sandbox/dotnet.go`，**第 4 runtime**）：`DotnetEnvManager`，`CreateEnv`/`InstallDeps` 为 **no-op**（dnx 运行时拉包即跑、无 per-owner env）；`ResolveExec` 认 `dnx/dotnet` → `<runtimeRef>/dnx`（**顶层**，不在 `bin/`）。
4. **`prepareSpawn`**（`app/sandbox/spawn.go`）：① **空-Cmd 检查移到 runtime 查之后**——docker 豁免（`dockerRuntimeKind`：image entrypoint 跑、无命令），其余 runtime 仍要求 Cmd；② **非-docker 传绝对 runtimeRef**（`filepath.Join(sandboxRoot, rt.Path)`，使 EnvManager 能在其下解析 npx/uvx/dnx）；docker 的 `rt.Path` 是镜像 ref、原样传。
5. **PATH 注入 `prependPATH`**（`app/sandbox/spawn.go`）：非-docker spawn 把 `<runtimeRef>/bin` + `<runtimeRef>` 塞到 env 的 PATH 最前。**这是端到端真跑逼出的硬集成点**——npx 的 shebang 是 `#!/usr/bin/env node`，运行时必须能在 PATH 找到 node 才能起；dnx（顶层）同理调 dotnet。对 venv/绝对 cmd 无害。

## 真机验证（用户机器 darwin/arm64）

- **dotnet**：`mise install dotnet@10.0.300` 真装成功，确认 **dnx 在 install dir 顶层**（`<install>/dnx`，不在 `bin/`）——dotnet.go 的路径假设据此钉死（注释留了「verified on a real machine」凭据）。
- **e2e**（`infra/mcp/e2e_test.go`，build tag `e2e`）：整条 stdio 链真跑——embed mise → 装 node 22.22.3 → `ResolveExec` 把 `npx` 翻成 `<runtime>/bin/npx` → `SpawnLongLived`（PATH 注入 node）→ go-sdk `Initialize` + `ListTools` → **context7 v3.1.0 真起来，列出 2 工具（resolve-library-id / query-docs），PASS 17.91s**。

## 测试清单

- `infra/sandbox/resolveexec_test.go`：`TestNodeResolveExec_RuntimeToolVsEnvDep` / `TestPythonResolveExec_UvRunner` / `TestDotnetResolveExec_Dnx`（三 runtime 的 ResolveExec：runtime-tool vs env-dep 分流）。
- `app/sandbox/path_test.go`：`TestPrependPATH`（PATH 前插，有无 PATH 条目两路）。
- `infra/mcp/e2e_test.go`：`TestE2E_Context7ViaNpx`（build tag `e2e`，真机才跑、离线 CI 不跑）。
- 留口2/3 测试：`app/handler/catalog_source_test.go::TestCatalogSource_ListsMethodNames` + `domain/trigger/config_test.go::TestValidateConfig_SensorTargets`（随 071e8425）。

## 注册留 M7

dotnet 的 `RuntimeInstaller`（mise `dotnet@`）+ `DotnetEnvManager` 注册，以及 node/python/docker/dotnet **全部** runtime 的注册，统一**留 M7 装配**（`cmd/server`）——现在**没有人注册**任何 runtime（sandbox `Service.envManagers` 由 boot 装配填）。本轮只把四套 EnvManager/Installer 实现就位 + 单测，不接 wiring。

## 两个 commit

1. **071e8425**（已提）：handler catalog 列方法名 + trigger sensor 绑 mcp.tool（留口 2/3，对齐容器实体）。
2. **sandbox 物理 runtime-tool**（待提）：node/python ResolveExec 认 runtime-tool + dotnet.go 第 4 runtime + prepareSpawn 空-Cmd/绝对 runtimeRef + prependPATH + resolveexec/path 单测 + mcp e2e。

## 契约同步

- `lab/backendcleaner/target/contract-changes.md`：新条目（sandbox runtime-tool 解析 + PATH 注入 + dotnet 第 4 runtime + docker 空-Cmd；装配影响 = M7 注册）。
- `docs/references/backend/domains/sandbox.md`（DOC-118）：runtime 矩阵补 dotnet 第 4 列 + ResolveExec runtime-tool 解析（npx/uvx/dnx）+ PATH 注入小节 + docker 空 Cmd 走 image entrypoint。
- deps-todo / STATE：用户自改（本轮不碰，避冲突）。
