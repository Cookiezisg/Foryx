# Round 0026 — sandbox（波次 1 · M1.8）三 runtime 隔离运行时

类型 / 目标：M1.8 sandbox 重写——三 runtime（Python+Node+Docker）隔离运行时。骨架照搬（旧实现质量高、本质复杂度无脂肪），换 GORM→orm + ResolveExec 抽象 + Docker 新写 + notification.Emitter + 重写烂文档。

## 核心方针（一句话）
**sandbox = 三 runtime 统一在「Installer + EnvManager」双接口下（image = docker 的 runtime、容器 = env），系统级 manifest 不分桶，spawn 委托 ResolveExec，env 变更发 notification。**

## runtime 矩阵调研（前置，决定技术方案）
GitHub MCP registry API（`api.mcp.github.com/v0/servers`）98 server 全量：remote 45（不吃本地 runtime）/ 本地 package 61；Python+Node+remote 覆盖 **90/98 = 92%**，缺口 8（**7 Docker-only + 1 .NET**，只发镜像/nuget）。→ 拍板 **Python+Node+Docker 三件套**。

## 关键设计决策（经讨论拍板）
1. **三 runtime 统一双接口**：image = docker runtime、容器 = env，零特例共用 manifest/锁/Ensure 流程。`EnvBin/EnvDir` → `ResolveExec(runtimeRef, envPath, opts) → (cmd, args, cwd)`，spawn 层不持 runtime 知识。
2. **workspace 隔离例外**：两表系统级（无 workspace_id，orm `meta.ws==nil` 自动跳过隔离）、磁盘 `~/.forgify/sandbox/` 不分桶——runtime 全机共享决定，相对 memory/skills 分桶的合理例外。
3. **Docker = 探测 + pull + docker run**：不代装（需 root）；`ErrDockerNotInstalled`/`ErrDockerDaemonDown` 从"残留疑似"转正为预留实装（调研证实 7/98 真需求——修正考古时的误判）。
4. **去 GORM**：domain 剥 tag → 纯 struct + db tag；store 基于 `pkg/orm` 重写；**硬删**（无 deleted 列）。
5. **notification.Emitter**（R0024）：env 变更发 `sandbox.env_status_changed`/`sandbox.env_deleted`。
6. **路由清理**：hacky `POST /sandbox/{action}`（前导冒号）→ RESTful + N5 字面（`DELETE /runtimes|envs/{id}`、`POST /sandbox:gc`）。

## 考古发现（旧实现评估）
- **骨架质量高**（故照搬）：双接口注册表正交（Docker 无缝插入）、bootstrap 踩坑细节（SHA256/幂等/degraded/codesign/attestation）、进程组杀、僵尸扫描、错误分层——本质复杂度无脂肪。
- **真账**：文档严重腐烂（`MiseSpec`/`BootstrapOK` 虚构字段、错误码全旧）；`EnvBin` 写死 venv；Docker 错误码悬空没实装。

## 新实现
- **domain**：Runtime/Env 纯 struct + db tag；3 接口（RuntimeInstaller / EnvManager(ResolveExec) / ToolRegistry）；错误集 + `ErrRuntimeNotFound`（替 `gorm.ErrRecordNotFound`）。
- **infra/store**：orm 两表 CRUD + manifest + 僵尸 PID + `TotalSizeBytes`（Go 层 sum，单机数量小）。
- **infra/sandbox**：mise/codesign/proc/exec_helper/embed 照搬；python/node 改 ResolveExec；**docker.go 新写**（DockerInstaller 探测+pull、DockerEnvManager docker run + `-e` env 排序）；spawn 清历史 § 注释 + `isBareCommand`。
- **app**：Service 编排照搬（Bootstrap / Ensure* / Spawn / GC / Shutdown / RestoreOnBoot / owner.ID 校验）+ Emitter 替 notifications pkg + prepareSpawn 调 ResolveExec（返回 args，docker 已包装）。
- **handler**：13 端点（RESTful + N5）；mise 二进制 cp（generated artifact，git 不入库）。

## 测试（全离线）
store 9（orm CRUD / FindByOwner / TotalSize / RunningPID / LastUsedBefore）；infra：mise 解压幂等 + 纯函数、spawn echo/cat/sleep + 进程组 + dangling symlink、python/node ResolveExec、**docker ResolveExec 命令组装 + env 排序 + Installer 纯函数**；app：owner.ID 校验、Spawn/LongLived/Shutdown handle 注册、RestoreOnBoot 真 kill。

## 验证
`gofmt -l` 干净 · `go build ./...` 0 · `go vet ./...` 0 · `go test ./...sandbox... -race` 全 ok（store 2.1s / infra 4.2s 含真 mise 解压+codesign / app 2.0s）。

## 契约
domains/sandbox.md 整篇重写（三 runtime 模型）；api.md 路由重构（RESTful + N5）；error-codes +`ErrRuntimeNotFound` + 改备注（去 nix）；events.md sandbox 事件升 notification（`sandbox.env_*`）；database.md 登记已对（不动）；contract-changes #6。

## 遗留 / 下一步
- **M1.9 permissions/hooks**（波次 1 续）。
- **Docker 精细化留 M3.6（mcp 那轮）**：容器优雅停止（`docker stop` + container-id 追踪）、孤儿回收、stdio MCP 长连接 e2e——docker spawn 真正被消费验证处。
- **留 M7**：cmd/server 注册三 installer/envmanager + `~/.forgify` base + `notification.Emitter` 注入 + `make fetch-mise` cmd（embed 二进制生成）。
