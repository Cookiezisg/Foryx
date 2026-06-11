# findings —— 评审中发现的偏差（不合理 / 冗余 / 产品问题）

> docswriter 真正的产出。一条 `F-N`：模块 · 类型 · 描述 · 对照的标准 · 建议修法（**标准化、不打补丁**）· 严重度 · 状态（open / 待裁 / 已修 / 转ADR / wontfix）。
> 流程：我列 → **用户裁决修哪些、怎么修** → 修 + 文档 → 下一模块。修法默认走"统一到标准"，不加特例。

| 类型 | 含义 |
|---|---|
| 不合理 | 设计讲不通 / 海拔错 / 该是 A 却 B |
| 冗余 | 同一概念两处实现 / 重复样板（标准>冗余：统一掉） |
| 产品 | 功能本身建模错 / 缺失 / 不一致 |
| 真bug | 代码确实错 |

---

## F-1 错误构造分裂（std vs errorsdomain）→ 全量统一 ✅ 已修

- **原现状**：errorsdomain.New 只用于"会冒泡 HTTP"的 domain 错误；tool 错误（todo/shell/web/filesystem/search/toolset）+ pkg/infra 原语用 std `errors.New`。"按出口分情况"是认知税（todo 为此写 9 行注释、易踩坑）+ 脆弱（一加写端点就漏成 500）。
- **裁决（用户 2026-06-11）**：真正全量统一。把错误**类型**从 `domain/errors` 移到 `pkg/errors`（纯机制、全层可用、无反向依赖）；**所有命名 sentinel 一律 `errorspkg.New`**——无"是否冒泡 HTTP"之分（出口不同：HTTP 读 Kind/Code、LLM tool 读 Message）。
- **已做**：类型搬迁（39 文件重命名 `errorsdomain`→`errorspkg`）+ 37 个命名 sentinel（tool 22 + pkg/infra 15）全转 `errorspkg.New`；web 的 auth/ratelimit/upstream 配真 HTTP 语义（502/429）。build + 全测试绿。
- **状态**：**已修**（type relocation + 全 sentinel；ADR `decisions/0002`）。

## F-2 websearch 错误无 Kind → 随 F-1 一并修 ✅

- **原现状**：`app/tool/web` 的 `ErrAuthFailed`/`ErrRateLimited`/`ErrUpstreamHTTP` 是 std error，丢了它们本有的 HTTP 语义。
- **已做**：转为 `errorspkg.New`——`WEBSEARCH_AUTH_FAILED`/`WEBSEARCH_UPSTREAM_HTTP`(KindBadGateway 502)、`WEBSEARCH_RATE_LIMITED`(KindRateLimited 429)。语义编码正确、未来若经 HTTP 冒出即对。
- **状态**：**已修**（随 F-1）。

## F-3 内联 validation 错误重复样板（Phase 3，open）

- **现状**：~22 处内联 `return errors.New("x is required")`（memory/ask/document/filesystem/search/shell 等 tool 的 ValidateInput）——非命名 sentinel，是逐工具重复的输入校验样板。
- **已做（7 agent 并行读码理解业务 + 去重，2026-06-11）**：22 处全转 `errorspkg.New`，**先去重不盲配码**——shell/kill 复用 `ErrEmptyBashID`、search 3 处 "limit must be non-negative" → 1 个 `ErrNegativeLimit`、document 4 处 "id is required" → 1 个 `ErrIDRequired`、memory 3 处共享 `ErrEmpty*`；ask 的 malformed-JSON 对齐全库 `fmt.Errorf("…: %w")` 包裹惯例（**不配码**——它是包裹非 sentinel）；顺带把 `shell.ErrInvalidTimeout`（还在 fmt.Errorf）也转了。
- **故意保留**：`web/fetch.go:163` "stopped after 10 redirects" 是 `http.Client.CheckRedirect` 回调的内部控制流、非面向 LLM 的 sentinel → 留 std `errors.New`（全库唯一一处）。
- **状态**：**✅ 已修**（Phase 3，全量统一彻底收尾）

## F-4 `orm.Page` + 自定义 `Order` ~~脚雷~~ → 撤回（误判，是有意特性）

- **模块**：P1 orm（`pkg/orm/select.go`）
- **我的误判**：以为「`Page` 只在 `order==""` 时设 keyset 序」是脚雷（自定义 Order 会与游标失配），建议"改成无条件强制默认序"。**没先 grep 调用方**。
- **真相（make verify 拦下）**：`conversation` 列表**故意** `.Order("pinned DESC, created_at DESC")` + `Page()` 做**置顶优先**——我的改动让 `TestList_PinnedFirstThenNewest` 立刻挂。这是**被用的特性**：`Page` 给默认序、可被覆盖；置顶少、都在首页，游标按 created 键够用。
- **处置**：回退；转成**解释性注释**（写明「别强制默认序、会破置顶优先」），防下个人重蹈。净收益 = 多了一处设计注释。
- **教训**：建议改地基行为前必须先 grep 全部调用方；机械门禁是安全网、不该靠它兜判断。
- **状态**：**撤回**（非 finding；orm 仍无实质 findings，STD-2 成立）

## F-7 agent 挂载运行时断裂（工具拿不到 + skill 没接线）✅ 已修（完整功能收尾）

- **模块**：P2 agent · **类型**：真 bug / 半成品（用户证实 agent 执行面曾 in-flight）
- **现状（3 agent 评审 + 我亲验 crux）**：① 旧 `filterToolsByWhitelist` 拿挂载 ref（`fn_`/`hd_`/`mcp:`）匹配全局工具 `Name()`（动词 `run_function`…）——永不相等 → **agent mount 的工具运行时全拿不到、裸跑**；唯一过的测试是 rigged 的（令 Name()==Ref）。② `Version.Skill` 存了、关系连了，但 **invoke 从不读**——skill 挂载完全没接线。③ knowledge（文档）是五类挂载中唯一真生效的。
- **设计裁决（用户 2026-06-11）**：agent 可接 function / handler 方法 / mcp call + 挂 skill（激活为执行指南）+ 挂 document；**不能**挂普通系统 tool call。"过滤全局表"思路本身就错。
- **修（完整功能实现）**：新 `app/tool/mount` 包——per-mount **合成绑定工具**（fn→以 function 命名、hd→`name__method`、mcp→`mcp__server__tool`；目标自己的 desc+inputs schema；Execute 走实体标准执行方法、TriggeredBy=agent；DIP 三窄端口可 fake）；`skillapp.Guide`（渲染执行指南，不设 active-skill、不 fork）注入 system prompt；InvokeDeps 重塑（删 `Tools func()`，加 Mounts/Skill；需要却 nil = 大声失败）；挂载解析 fail-fast + 撞名检测（`AGENT_MOUNT_INVALID`）；`schemapkg.ToJSONSchema`（地基补正向转换）；rigged 测试改真实路径 + mount 包全合成单测。**评审报告纠偏**：agent 的 recordExecution 其实早已填 toolCallID/flowrunID/flowrunNodeID（X1 不含 agent）。
- **状态**：**✅ 已修**（build + 全测试 0 FAIL；`domains/agent.md` 落定）

## F-8 P2 亲审 findings 批（用户裁决 2026-06-11：X1 选 A、其余全修）✅ 已修

我亲读三实体全栈（~9000 行）重审（用户不信 subagent 报告），证实/纠正/新增如下，全部修复：

- **X1（A 案）审计列三实体悬空**：`tool_call_id/flowrun_id/flowrun_node_id` 列+索引+过滤器+HTTP param 齐全但无写入供给（根因：scheduler.Dispatcher 接口不带 flowrun 上下文；之前"agent 已全"只对一半——有管道但 dispatcher 不喂）。修：`reqctx` 加 `SetFlowrunID/SetFlowrunNodeID`（横切，与 conversation/toolCall id 同级）、调度器 `runNode` 派发前注入 ctx、三实体 recordX 读 ctx（agent 的 InvokeInput 显式字段优先供 ADR-010）。派发端口保持 `(ctx, ref, input)` 不穿透。
- **H3（真 bug）config 孤儿 key 炸 spawn**：`__init__` 命名参数 + driver `**args`，schema 删过的 arg 留在 config → TypeError → 永久 spawn 失败。修：`spawnInstance` 唯一咽喉点按 active schema 过滤 config。
- **H2 `MethodSpec.Timeout` 死字段 + 注释撒谎**（亲验 0 读取、"client default"不存在）。修：**实现**——Call 先解析 method spec（不存在 → `HANDLER_METHOD_NOT_FOUND` 早失败，不进 RPC）+ Timeout>0 加 ctx deadline（防卡死 method 堵死单例串行管道）；domain 注释重述。
- **H1 `Client.Call` 死代码**（亲验 0 调用方）。修：删接口方法+实现，统一 StreamCall。
- **X5 事件名分叉**：UpdateMeta 通知 fn/hd 发 `updated`、agent 发 `meta_updated`。修：统一 `updated`。
- **X2 UpdateMeta 错码**：fn+hd 用 `*_OP_INVALID`（forge 语义）报 PATCH meta 的 name 非法。修：各加 `FUNCTION_INVALID_NAME`/`HANDLER_INVALID_NAME`(400)。
- **Count 字段误导**（三实体一致 Count=页大小）。修：删字段（数组长度自明；前端重建中零迁移成本）；测试断言改 len()。
- **Create 查名预检冗余**（TOCTOU；store 的 partial-UNIQUE + ErrConflict 翻译才是真守卫）。修：删三实体预检，唯一性单源化到 DB 约束。
- **F-3 漏网 67 处**：tool 层 `fmt.Errorf("<tool>: x is required")` 静态校验（当时只扫了 errors.New 形式）。修：9 包各加 `sentinels.go`（共享 sentinel、同物理违例一码，31 新码）；trigger 的 kind 校验**复用** domain `ErrInvalidKind` 不造新码。错误码 213→246，机械重抽取保 parity。
- **wontfix（记录为设计事实，写进 domains 文档）**：ctx 取消标 crashed 重启单例（协议正确性）；`HANDLER_CLIENT_*` 冒泡 HTTP（502 语义对）；Search 全表载入（本地规模取舍）；agent tokens/steps 不持久化（无 ALTER 机制，留观测议题）；ForgedInConversationID 类型微分叉（不值得动 DDL）。
- **文档**：`domains/{function,handler,agent}.md` 三篇 0 障碍级详解 + seed `api.md`/`database.md`/`events.md` 三索引（三实体条目）+ reqctx.md 补 flowrun 行。

## F-9 P3 亲审批（durable 引擎六模块，用户裁决 2026-06-11：全修）✅ 已修

亲读 ~11000 行（flowrun/workflow domain+store、scheduler 全 7 文件、trigger 全链含 infra 4 listener、control/approval、bootstrap、transport）。引擎核心设计是全库最好的一块（record-once / first-wins 全链一致 / claim 单事务 / walk 统一 join 规则 / kill 先标后取消）。Findings：

- **P3-1（🔴 致命真 bug，实证锤死）后台路径裸 ctx → 自动化链路全死**：`a.Boot(context.Background())` + drainLoop（Background 派生）调 ws-scoped 查询（ListPendingFirings / ListParkedNodes / ListAllHandlers / ListActiveWorkflows）→ `MISSING_WORKSPACE_ID`。后果：DrainFirings 每 5s 失败（**cron/webhook/fsnotify/sensor 自动触发完全不工作**）、CheckTimeouts 同死（**审批超时永不结算**）、ReattachActive 死（**重启后 active workflow 监听全丢**）、handler/mcp Boot 预热死。实证：临时单测证明 `ListParkedNodes(Background)` 报错而显式 CrossWorkspace 的 `ListRunningRuns` 正常。为何测试绿：全部测试用带 ws 的 ctx；真实 Boot 路径无覆盖。讽刺：同一约定在 Recover（per-run 播种）/onReport（Detached(wsID)）做对了，drainLoop/Boot 漏了。**修**：`bootstrap.forEachWorkspace`（workspaces 是全局表、裸 ctx 可列）——Boot 的 handler/mcp/ReattachActive 与 drainLoop 每 tick 的 DrainFirings/CheckTimeouts 全部逐 workspace 以 `Detached(wsID)` 重放；**守护测试** `bootstrap/background_ctx_test.go` 锁死契约（裸 ctx 必败 + Detached 必通）。
- **P3-2（🟡）webhook/fsnotify/sensor DedupKey 全空**（只有 cron 有）→ `idx_trf_dedup`（D3）对三源形同虚设。**修**：webhook=sha256(body)+分钟桶（秒级网络重试折叠、跨分钟合法重复放行——UNIQUE 永久故键必须含时间）；fsnotify=path+op+秒桶（编辑器保存突发折叠）；sensor=probe 秒时刻（一探至多一条/工作流）。
- **P3-3（🟡）Concurrency 线缆值大小写混杂**（'serial','Skip','BufferOne','BufferAll','AllowAll'）。**修**：统一 snake（serial/skip/buffer_one/buffer_all/allow_all）+ DDL CHECK + 全注释同步（未上线零包袱，本地库重建）。
- **小项**：FlowRun.Status 注释漏 cancelled（修）；buffer_* v2 占位按 allow_all（注释+文档写明）；5s 轮询收件箱是单进程本地的合理取舍（文档记）。
- **文档**：`foundation/scheduler-flowrun.md`（引擎 0 障碍级——记忆化模型/walk 算法/run 生命周期/后台播种铁律）+ `domains/{workflow,trigger,control,approval}.md` 四篇 + 三索引 P3 增量（修正 workflow `:kill`/`:capability-check` 端点遗漏）。

## F-5 detached context 散手搓 → 加 `reqctx.Detached` helper ✅ 已修

- **模块**：P1 reqctx（横切：~15 个 app/infra 站点）
- **类型**：冗余（标准化）
- **现状**：detached 异步（finalize / 后台写 / 自动标题）的"防孤儿 ctx"惯用法 `SetWorkspaceID(context.Background(), wsID)` 散在 ~15 处手搓，靠每作者记得重埋 ws（忘 → 运行时 `MISSING_WORKSPACE_ID`）。
- **修法（用户裁：加 helper）**：`reqctx.Detached(wsID)` 命名并集中惯例（Background 非 WithoutCancel = 脱离已取消请求；重埋 ws = orm 隔离最低要求）；15 站点统一改用，conv 子集照常链 `SetConversationID`。
- **状态**：**✅ 已修**（helper + 15 站点 + `foundation/reqctx.md` 写明惯例）

## F-6 `ErrMissingWorkspaceID` Kind 401 与自己注释/镜像/架构三重矛盾 ✅ 已修（真 bug）

- **模块**：P1 reqctx（`pkg/reqctx/workspace.go`）· **类型**：真 bug
- **现状**：码 `KindUnauthorized`(401)，但 ① 自己注释写"wiring bug(500), not 401" ② 镜像 `ErrMissingConversationID` 是 500 且注释"Mirrors ErrMissingWorkspaceID (500, not 401)" ③ 客户端 401 已由中间件 `UNAUTH_NO_WORKSPACE` 兜。是我之前错误统一时挑错 Kind——全项目视角（读注释 + 镜像 + 中间件）才暴露。
- **验证（吸取 F-4 教训）**：三重证据一致；测试断言 error 身份非 kind，改后 0 FAIL。
- **修法**：`KindUnauthorized` → `KindInternal`(500)；error-codes.md 同步（401→500 + 补"两个 no-workspace 错误之分"）。
- **状态**：**✅ 已修**
