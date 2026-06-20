---
id: DOC-020
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-14
review-due: 2026-09-14
audience: [human, ai]
---

# mcp —— MCP server 集成（外部工具生态网桥）

## 1. 定位 + 心智模型

MCP server 是**容器实体**：持 N 个可调工具、以常驻进程（stdio 经 sandbox）或远程连接（SSE / streamable-HTTP + header 鉴权）运行。**生命周期镜像 handler**：`map[mcp_id]` 单例池、Boot per-workspace 并发连接（best-effort）、`reconnect`（"重置按钮"——救活着但坏了的连接；并发连接注册时换出旧 client+进程并关闭，后写者赢）、优雅 Shutdown。

**状态机（进程内、永不落盘）**：disconnected → connecting → ready；连续 3 次调用失败 → **degraded**（仍可服务、软警告）；连不上 → failed（reconnect 可救）。`IsCallable` = ready|degraded。

**密文**：`Env`（stdio 注入子进程）+ `Headers`（remote 静态鉴权）+ `OAuth`（remote OAuth 凭据束：DCR 注册的 client + access/refresh token 对）落盘加密在 `config_enc` 单列（domain 持明文、store Save 加密/Get 解密）——三者本身**都不是列**。

## 2. 关键行为

- **三条安装路径**：`InstallFromRegistry`（市场 curated 条目——择最优可跑 package（按 runtime 可用性）或 remote、缺**必填** env 报 `MCP_ENV_MISSING`、物化 runtime env）/ `AddServer`（手动 PUT，同名替换）/ `Import`（Claude Desktop mcp.json 片段，overwrite 控制同名）。`Source` 记录来源（registry/manual/import），`RegistryID` 供"查更新"。
- **env 必填/可选**：`EnvVar.Required` 从 registry 的 `is_required` 捕获；`missingEnv` **只强制 Required 的**——可选旋钮（如 brightdata 8 个 zone 调参）给了就传、不给不拦（此前一律必填会卡死多可选 env 的 server）。registry 偶把真需要的 key 标可选，catalog overlay 用 `upsertEnv` **升级为必填**；以参数传的凭据（`--api-key {api_key}`、`{logfire_token}`）由 `Plan` 按占位提成 env（凭据形的必填）、安装时 `expandArgs` 填好再起进程。
- **市场 = curated 白名单**（`infra/mcp` 的 `CuratedCatalog` 装饰 `GitHubRegistrySource`，数据 `catalog.json`）：只暴露**已核验「纯代码即可真正连上用」**的 server（List/Get 按 slug 过滤——非白名单 slug `Get` 即 `MCP_REGISTRY_NOT_FOUND`、不可装）。
- **事实源纪律（不误导）**：**每个 MCP 的配置以它在 registry 的 `server.json` 声明为准**——env 的名字/描述/`is_required` 一律取自 registry、**不改写**；catalog overlay 只做**最小机制增量**（不臆造）。认证按**忠实度优先级**：① 支持 DCR 的 remote 一律走 **OAuth-via-discovery**（运行时发现+注册，**零编造配置**，22 个）；② registry 声明的静态 header/env 原样用；③ 真无 OAuth/DCR 的 remote 才补**最小**静态-token 配置（env 名 + `isSecret` + 一句**事实**型 token 类型标签，**不写「去 X 网站第几步拿」的臆造步骤**，12 个）；④ 自带客户端/scope/URL 等结构化机制参数（Entra/glean/box）。overlay 注入但 registry 没声明的 env 用最小事实标签；registry 已声明的用 registry 原话。需厂商业务步骤（注册 OAuth app / 进 allowlist，如 Google/Figma/Vercel/Box）的直接缺席。每条可选 **auth 覆盖**钉死原始 registry 行缺失/写错的安装+认证，三类 `transport`：静态 token **remote** 注入 `Authorization: Bearer {TOKEN}` header + 必填 env（**故 `Plan`→`missingEnv` 结构性堵死「静默零认证连接」**）；静态 token **stdio** 把 token env 标必填、或钉死启动 package（上游裸名解析不出 runtime 时，如 Snyk CLI）；**oauth** 标 `Remote.Auth=oauth`（无静态凭据，装机走 §OAuth 流程）。覆盖永远走现有 `Env`/`Headers`/`OAuth` 加密通道——无新明文列。**判据 + 分档 + 5 个永不做的取舍**见 [ADR 0006](../../../decisions/0006-mcp-curated-whitelist.md)。
- **OAuth 装机流程**（remote OAuth 2.1 + PKCE + DCR，`app/mcp` 编排 + `infra/mcp/oauth` 纯协议）：`Plan.OAuth` 的条目装机走 `authorizeOAuth`——①探测（POST initialize 读 401 `WWW-Authenticate`）→②发现（RFC 9728 受保护资源元数据 → RFC 8414 AS 元数据）→③DCR（RFC 7591 运行时注册公共客户端、`token_endpoint_auth_method=none`，**无厂商预注册、无 client secret**）→④PKCE(S256)+state 构 authorize URL（RFC 8707 `resource` 把 token 绑死本 server）→⑤拉系统浏览器（`BrowserOpener`，sidecar 跑在用户机）+ 起 127.0.0.1 随机端口 loopback 回调（RFC 8252，**无需证书**）→⑥换 token。token 束加密存 `config_enc`；连接时 `oauthRoundTripper` 经 `TokenSource`（DIP，app 实现）每请求注入实时 Bearer、临过期用 refresh token 静默换新并重存；refresh 失败/缺失 → `MCP_OAUTH_REAUTH_REQUIRED`。受众绑定：仅采纳与 server 同 host 的 PRM `resource`（防受众改向）。**每租户端点**（模板 URL，如 Glean）：overlay 标 `Remote.URLEnv`、`Plan` 暴露成必填 env，安装时 `expandPlaceholders` 用用户填的值解析出真实 URL 再走流程。**已纳入 22 个 oauth**：原 oauth-dcr 那批 + **凡支持 DCR 的 remote 一律忠实改走 OAuth-via-discovery**（neon/todoist/monday/zapier/anima/qualityclouds/notion/supabase/sentry——此前因「先有静态-token 旁路」被标 static-token、实网 DCR POST→201 复核后改回 OAuth，省掉编造的 token header/env/步骤）。Vercel 经 register→201 且 authorize→200 复核确认非 allowlist；getguru 因无注册端点 + Guru 须白名单客户端而移除。**自带客户端**（无 DCR 的 OAuth 提供方，如 Box/Microsoft Entra）：overlay 标 `Remote.ClientIDEnv`(+`ClientSecretEnv`)，`Plan` 暴露成必填 env、`InstallPlan.OAuthClient*Env` 记其名；安装收用户**自己注册的 OAuth app** 的 client_id(+secret)，`authorizeOAuth` 据此**跳过 DCR**、用用户客户端跑授权码流程。loopback 回调首选固定端口 `47100`（使自带客户端用户能注册确定的 redirect URI），占用则退随机。**scope 覆盖**（`Remote.Scopes`）：AS 元数据不通告资源 scope 时（Entra 把受众编进 `<app-id>/.default`）由 catalog 钉死；带 scope 覆盖即视为 Entra 类、去掉多余的 RFC 8707 `resource` 参数。**本地 server**（`catalogEntry.Local`）：不在 registry、自包含、无认证的 loopback 端点（如 Figma Dev Mode `127.0.0.1:3845/mcp`，用户在 Figma 桌面开开关即用）。判据（用户自助 OK、厂商步骤不做）见 [ADR 0006](../../../decisions/0006-mcp-curated-whitelist.md)。**市场现 96 条可用（loop 实测移除连不上的 nuxt-mcp/servicebricks）**；**唯二不上**（真·厂商步骤，用户无法自助）：figma remote（需 Figma allowlist Anselm——其本地 Dev Mode 已作为 `figma-dev-mode` 上架旁路）+ getguru（Guru 须白名单客户端、无开放 DCR）。Box/MS-Enterprise/MS-sentinel 已上（用户自带 OAuth app）。
- **stdio 链**：sandbox `EnsureEnv`（node/python/docker/dotnet 四 runtime）→ `SpawnLongLived`（进程归 sandbox 管）→ infra client 只把管道接进 go-sdk `IOTransport` 走 MCP 握手——**本包不碰进程生命周期**。stderr 进 256KB ring buffer（`StderrTail` 供 triage）。
- **工具面**：连接后 `tools/list` 缓存为 `ToolDef`（InputSchema **原样**复用为包装工具的 Parameters——我们不造 schema）；全局注册表里每个工具包成 `mcp__<server>__<tool>` 动态工具（`tool/mcp/dynamic.go`）。**chat 消费**：`DynamicTools(ctx)` **per-request** 注进 chat host 的 `search_tools` 排序池（不进静态 `Toolset`/Overview，因 MCP server 是 workspace 域 + 可变）——LLM 经 search_tools 发现、host 把已 discovered 的 offer 进本回合工具集并可调（`chat.Deps.DynamicTools` provider → `NewSearchTools` + `chatHost.Tools`/`TryActivateForTool`，F52）。
- **进度关联**：go-sdk 的 progress handler 是 session 级全局——per-call token（progSeq 铸造）→ sink 映射把 server 的进度通知关联回发起那次 CallTool，转发到该调用的 sink（chat 的 tool_call progress + entities run 终端 + 限长 logtail 三写）。
- **调用记账**（`recordCall`）：mcp_calls Log 表，溯源 5 列与其它执行单元对齐（conversation/message/toolCall 从 ctx + **flowrun 2 列**——调度器派发注入）；`logs` 列存 progress 通知、**失败时附 server stderr 尾**（8KiB，server 级、标注可能早于本次调用）；默认 call 超时 180s（MCP 工具可能调 LLM/爬虫，长顶棚把控制权还给 agent）。

## 3. 关键设计决策

进程归 sandbox（mcp 不碰 PATH/进程）；状态不落盘（无 health-history 表——重启即重连）；比 handler_calls 精简（无 version_id——server 无版本；无 instance_id——stderr 自有 ring）；degraded 仍服务（外部 server 抖动不该立刻禁用）。

## 4. 契约（引用）

端点（servers CRUD + `:reconnect` + `stderr` + `tools/{tool}:invoke` 直调 + registry 浏览/安装 + import + calls 列表/详情——列表带 ok/failed 聚合、详情带 logs，与 handler 同形）→ [api.md](../api.md) · 表 `mcp_servers`（config_enc）+ `mcp_calls`（Log）→ [database.md](../database.md) · 码 `MCP_*` 17+3 → [error-codes.md](../error-codes.md) · ID：`mcp_`/`mcl_`。LLM 工具：动态 `mcp__*__*` 全家 + `install_mcp_server` 等系统工具 + 调用日志两查询（`search_mcp_calls`/`get_mcp_call`，与 fn/hd/ag 的执行日志查询面对齐）。通知：`mcp.{installed,updated,removed,reconnected}` 族（`SetNotifier` 注入 Emitter，缺则整族静默）→ [events.md](../events.md)。消费方：chat loop（动态工具）、agent 挂载、workflow action 节点。**ref-token 统一**：`mcp:<server>/<tool>` 的 server 段既可是 workspace 唯一的公开 **name**（搜索投影 / refHint 给的、agent 挂载用的），也可是 **mcp_ id**——`Service.ResolveServerID`（先按 id、再按名）在三处消费方都解析成规范 mcp_ id：agent 挂载（`mount.go` ListServers 按名）、workflow 能力检查（`refresolver.go` mcp 分支）、workflow 派发（`dispatch.go` RunAction mcp 分支 → CallTool）。故 search_blocks 投出的 name 形 ref 可直填 workflow 节点（此前 dispatcher/refresolver 只认 id、name 形静默 `MCP_SERVER_NOT_FOUND`/`ErrRefNotFound`——已修）。
