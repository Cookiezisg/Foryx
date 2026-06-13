---
id: DOC-020
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# mcp —— MCP server 集成（外部工具生态网桥）

## 1. 定位 + 心智模型

MCP server 是**容器实体**：持 N 个可调工具、以常驻进程（stdio 经 sandbox）或远程连接（SSE / streamable-HTTP + header 鉴权）运行。**生命周期镜像 handler**：`map[mcp_id]` 单例池、Boot per-workspace 并发连接（best-effort）、`reconnect`（"重置按钮"——救活着但坏了的连接；并发连接注册时换出旧 client+进程并关闭，后写者赢）、优雅 Shutdown。

**状态机（进程内、永不落盘）**：disconnected → connecting → ready；连续 3 次调用失败 → **degraded**（仍可服务、软警告）；连不上 → failed（reconnect 可救）。`IsCallable` = ready|degraded。

**密文**：`Env`（stdio 注入子进程）+ `Headers`（remote 鉴权）落盘加密在 `config_enc` 单列（domain 持明文、store Save 加密/Get 解密）——Env/Headers 本身**不是列**。

## 2. 关键行为

- **三条安装路径**：`InstallFromRegistry`（市场 curated 条目——择最优可跑 package（按 runtime 可用性）或 remote、缺必填 env 报 `MCP_ENV_MISSING`、物化 runtime env）/ `AddServer`（手动 PUT，同名替换）/ `Import`（Claude Desktop mcp.json 片段，overwrite 控制同名）。`Source` 记录来源（registry/manual/import），`RegistryID` 供"查更新"。
- **stdio 链**：sandbox `EnsureEnv`（node/python/docker/dotnet 四 runtime）→ `SpawnLongLived`（进程归 sandbox 管）→ infra client 只把管道接进 go-sdk `IOTransport` 走 MCP 握手——**本包不碰进程生命周期**。stderr 进 256KB ring buffer（`StderrTail` 供 triage）。
- **工具面**：连接后 `tools/list` 缓存为 `ToolDef`（InputSchema **原样**复用为包装工具的 Parameters——我们不造 schema）；全局注册表里每个工具包成 `mcp__<server>__<tool>` 动态工具（`tool/mcp/dynamic.go`）。
- **进度关联**：go-sdk 的 progress handler 是 session 级全局——per-call token（progSeq 铸造）→ sink 映射把 server 的进度通知关联回发起那次 CallTool，转发到该调用的 sink（chat 的 tool_call progress + entities run 终端 + 限长 logtail 三写）。
- **调用记账**（`recordCall`）：mcp_calls Log 表，溯源 5 列与其它执行单元对齐（conversation/message/toolCall 从 ctx + **flowrun 2 列**——调度器派发注入）；`logs` 列存 progress 通知、**失败时附 server stderr 尾**（8KiB，server 级、标注可能早于本次调用）；默认 call 超时 180s（MCP 工具可能调 LLM/爬虫，长顶棚把控制权还给 agent）。

## 3. 关键设计决策

进程归 sandbox（mcp 不碰 PATH/进程）；状态不落盘（无 health-history 表——重启即重连）；比 handler_calls 精简（无 version_id——server 无版本；无 instance_id——stderr 自有 ring）；degraded 仍服务（外部 server 抖动不该立刻禁用）。

## 4. 契约（引用）

端点（servers CRUD + `:reconnect` + `stderr` + `tools/{tool}:invoke` 直调 + registry 浏览/安装 + import + calls 列表/详情——列表带 ok/failed 聚合、详情带 logs，与 handler 同形）→ [api.md](../api.md) · 表 `mcp_servers`（config_enc）+ `mcp_calls`（Log）→ [database.md](../database.md) · 码 `MCP_*` 11+3 → [error-codes.md](../error-codes.md) · ID：`mcp_`/`mcl_`。LLM 工具：动态 `mcp__*__*` 全家 + `install_mcp_server` 等系统工具 + 调用日志两查询（`search_mcp_calls`/`get_mcp_call`，与 fn/hd/ag 的执行日志查询面对齐）。通知：`mcp.{installed,updated,removed,reconnected}` 族（`SetNotifier` 注入 Emitter；曾缺线整族哑火，AC-29）→ [events.md](../events.md)。消费方：chat loop（动态工具）、agent 挂载（`mcp:server/tool`）、workflow action 节点（dispatcher 直调 CallTool）。
