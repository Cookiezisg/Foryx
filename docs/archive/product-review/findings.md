---
id: WRK-009
type: working
status: archived
owner: @weilin
created: 2026-06-12
reviewed: 2026-06-12
review-due: 2026-09-12
expires: 2026-09-12
landed-into: ""
audience: [human, ai]
---

# findings —— 全部发现（PR-N，每条先亲验再定性）

> 严重度：🔴 产品级断点 / 🟡 体验或一致性缺口 / 🟢 文档·可观测性轻症。处置：fixed / pending（DECISIONS-PENDING）/ wontfix（带理由）/ doc-fix。

## R1 配置与基础设施面

### 实锤·已修

- **PR-1 🔴 workflow Edit/Revert 换入口 trigger ref 不重绑活监听 → 旧绑定泄漏 + 新 trigger 无人听**（fixed）
  验证：`Activate` 按**当时** active 图解析 entry refs 挂监听（execution.go:82-90）；`Edit`/`Revert` 移 active 指针后零 binder 调用（crud.go）；`Deactivate` 按**当前**图解析去 detach（execution.go:102+）。后果：active workflow 改入口（trg_a→trg_b）后，trg_a 永远继续触发本 workflow、trg_b 无人听、deactivate 时 detach(trg_b) no-op → trg_a 引用计数永不归零（listener 永驻）。图编译校验「至少一个 trigger」（domain/workflow/graph.go:55）挡得住「删光」、挡不住「换 ref」。
  修复：`rebindIfListening`（diff 旧/新图 refs，detach 删除者 + attach 新增者；双图缺一跳过防 refcount 重复）接入 Edit 与 Revert（Revert 补「指针移动前快照旧图」）；`TestEditRevert_RebindLiveListener` 钉死（active 换 ref 重绑 ×2 方向 + inactive 不碰 binder）。已知限界：staged（AttachOnce）武装在 binder 内部、workflow 行不可见，staged 期间编辑保留旧一次性武装——试运行态可接受，注释明示。

- **PR-2 🟢 api.md workspace/sandbox 行与代码脱节**（doc-fix）
  验证：api.md 写 `GET /sandbox/status`，实际是 `GET /sandbox/bootstrap-status`；runtimes 三端点、`GET /sandbox/disk-usage`、`POST /sandbox:gc`、workspace 的 `default-models/{scenario}`、`default-search`、`:activate` 均未登记（handlers/sandbox.go:40-49、workspaces.go:32-47）。已重述该节。

### 实锤·待裁决（详见 DECISIONS-PENDING）

- **PR-3 🔴 `pkg/limits` 是未接线的空壳**（fixed——裁决 A：schema 重述为现实投影（删 9 个无消费方字段、并入 InvokeMaxTurns、新增 ToolResultCapKB/TriggerRatio）、Default 对齐接线前常量（行为零变化、测试钉死）；新 `app/settings` 读写 `<dataDir>/settings.json` + `GET/PATCH /api/v1/limits` 热换；9 处硬编码常量改读 `limits.Current()`：chat MaxSteps、agent InvokeMaxTurns、mcp 调用超时、bash 超时+输出 cap、read 页大小、loop tool_result cap、contextmgr 触发比、attachment 上限、webhook body 上限）
  验证：包自述「用户可调运行上限的唯一来源……启动装配经 SetProvider 换成 settings.json 支持的 getter」（limits.go:1-8）。实际：①全仓无任何 settings.json 加载器；②`SetProvider` 生产代码零调用（仅测试）；③全仓唯一消费方是 `infra/llm/provider.go:59` 读 `Timeout.LLMIdleSec`——其余全部字段（MaxSteps/Subagent*/bash·mcp 超时/工具体量/attachment 上限/workflow 轮数…）无人读，真实生效的是各模块**各自的硬编码常量**（如 `loop.maxToolResultBytes`、`mcp.defaultCallTimeout`、`shell.outputCapBytes`）。「用户可调」目前是虚构。

- **PR-4 🟡 Ollama embedder 参数无配置面**（fixed——裁决 A：search_meta 补 `ollama_base_url`/`ollama_model` 两键、PATCH/GET 全接、工厂注入 + 参数变化重建适配器、域默认权威 `searchdomain.DefaultOllama*`；app/integration 双层测试）
  验证：`SetEmbeddingProviders(…, NewOllama("", ""))`（build_services.go:134）——baseURL 钉死 `127.0.0.1:11434`、model 钉死 `embeddinggemma`（engine.go NewOllama 默认分支）；`PATCH /search/settings` 只收 `embedder` 一个字段。用户切到 ollama 后无法指定地址/模型，GET 也看不到生效的 baseURL。

- **PR-5 🟡 桌面 app 日志故事缺失**（fixed——裁决 A 最小版：`<dataDir>/logs/forgify.log` 轮转 JSON（lumberjack 10MB×3×28d gzip）tee 在 stderr 控制台旁；文件 sink 测试）
  验证：zap 只出 stdout/stderr、级别仅由 `FORGIFY_DEV` 环境变量二档切换（infra/logger/zap.go:16-32、cmd/server/main.go:25）；无文件落盘/轮转/级别配置。Wails 桌面 app 用户报障无日志可交。

- **PR-6 🟡 备份/跨机迁移故事缺失**（doc-fix——裁决 B：`how-to/data-migration.md` 声明数据布局/备份/三类密文重填边界；export/import 进 roadmap）
  验证：落盘加密密钥从 `MachineFingerprint` 派生（build_data.go:155-168，CR-20 接通）——拷 `~/.forgify` 换机后 api key/handler config/mcp config 密文**全部不可解**；无任何 export/import 面；文档零说明。

### 轻症·已处置

- **PR-7 🟢 utility 模型未配时静默降级未声明**（doc-fix，随本轮文档批）
  验证：autotitle best-effort 吞错（chat/autotitle.go:29-36，无标题无提示）；contextmgr 压缩跳过；search_blocks 精选落第三档。行为本身合理（核心链路不依赖 utility），但「utility 的依赖清单 + 未配时各功能表现」无文档——用户无法把「没标题/没压缩」归因到「没配 utility 模型」。→ 已在 domains/chat.md 补一句（见提交）。
- **PR-8 🟢 env GC 无自动触发**（wontfix）
  验证：`POST /sandbox:gc` 手动口在、`Service.GC(olderThan)` 在（sandbox.go:214），无定时器。理由：本地单用户磁盘、disk-usage 可见、手动口已具备；自动 GC 引入「正在用的 env 被回收」风险大于收益（已有 ErrEnvNotFound 自愈兜底）。
- **PR-9 🟢 首启零 workspace 的 first-run 契约未文档化**（doc-fix）
  验证：bootstrap/cmd 无 workspace 播种；`forEachWorkspace` 对空集 no-op；删除守卫 `ErrCannotDeleteLast`（Count≤1 拒删）。首个 workspace 由前端 onboarding 创建——契约成立但没写下来。→ api.md workspace 行已带「守最后一个」，domains 留待前端对接预检（R5）一并补。

### 误报（agent 面扫报告，亲验驳回）

- ✗「agent 实体无 Edit 操作」——`agentapp.Edit` 在（crud.go:166），`edit_agent` 工具在（tool/agent/forge.go:107）。
- ✗「handler Edit 后活实例可能用旧代码（版本不一致）」——`Restart = Stop + Get`（manager.go:110-113），先停后起：失败时实例已不在，下次 Get 按新 active 版本 spawn；不存在 stale 实例路径。残余仅「spawn 失败留 stopped 态」且 RuntimeState 可查。
- ✗「sandbox 无清理口/env 不可见/无磁盘占用/无 boot 诊断」——十个端点俱全（handlers/sandbox.go:40-49：runtimes GET/POST/DELETE、envs GET×2/DELETE、disk-usage、bootstrap-status、:gc、:retry-bootstrap）。
- ✗「llama-server 关停缺失」——`search.Close()` 对实现 `ProviderCloser` 的 provider 调 Close（app/search/service.go:144-152），builtin 引擎杀子进程。
- ✗「mcp 改 config 不自动重连」——AddServer upsert 路径 `persistAndConnect` 自动连（install.go:98-110）。
- ✗「limits 经 settings.json limits.agent.maxSteps 可调」——把包注释当实现；见 PR-3。

## R2 实体闭环配对矩阵

### 实锤·已修

- **PR-10 🔴 todo_write 工具不存在——todo 实体零写入口、功能整体不可用**（fixed）
  验证：`domains/todo.md` 声称「LLM 工具：todo_write（resident）」；`handlers/todo.go:14` 注释「写入是 LLM 专属（TodoWrite 工具）——前端从不编辑」（HTTP 看板只读 by design）；`todoapp.Write` 完整就绪（整表替换+渲染回显+流推送，todo.go:56）。但 `tool/` 无 todo 包、toolset 零注册、全仓无 `"todo_write"` 字符串——**没有任何主体能写 todos**，chat 每步注入的 TodoReminder 永远渲染空清单。三处（文档/HTTP 注释/service 设计）都以为工具存在，是「波次 2/3」规划中漏装的接线。
  修复：`tool/todo` 包（todo_write，**resident**——规划不该先经 search_tools 发现）+ `TODO_ITEMS_REQUIRED` sentinel（nil≠[]：[] 清空、缺省是错）+ toolset 注册 + 接线测试。
- **PR-15 🟢 events.md 三处与代码脱节**（doc-fix）
  验证：`workflow.lifecycle_changed`/`attention_changed`（crud.go:241/261）、`sandbox.env_status_changed`/`env_deleted`（sandbox.go:441/450）代码发、文档无；文档写「workflow/trigger/... 生命周期族」但 trigger **无任何生命周期 publish**（活动走 activations 行 + fire 信号）。已重述。

### 实锤·待裁决（DECISIONS PD-E/PD-F）

- **PR-11 🟡 对话历史对 LLM 不可检索**（fixed——裁决 PD-E：`search_conversations` 工具，混合检索 conversation 投影，只返 conversationId/title/snippet/messageId、IncludeArchived、limit≤20；防上下文污染=指针不倾倒）
  验证：综搜（人）覆盖 conversation（12 实体投影）；LLM 面：search_blocks 限六类积木、8 个垂搜工具不含 conversation、无任何对话读取工具。「用户问：我们上次聊的那个方案」LLM 无工具可查——只能靠 memory 萃取物。
- **PR-12 🟡 relation 关系图对 LLM 不可查**（fixed——裁决 PD-F：`get_relations` 工具包 Neighborhood（kind+id+depth 1-3），relation 构造上移至 toolset 前供注入）
  验证：HTTP 有 `GET /relations/neighborhood`（依赖/影响面查询）；LLM 零工具。LLM 删除/改造实体前无法回答「谁在用它」——capability_check 只覆盖 workflow 单向。

### 轻症·已处置

- **PR-13 🟢 handler config 清空无 LLM 工具**（wontfix）：`update_handler_config` 的 merge-patch null 已可删键，HTTP DELETE 兜底；为罕见场景加工具是面积膨胀。
- **PR-14 🟢 fire_trigger 不返 activationId**（fixed，acceptance-w1 后批）：fanOut/FireManual 返 actID，HTTP `:fire` 与 fire_trigger 工具均带 `activationId`——拿 id 直查闭环。
- **PR-16 🟢 document/memory/skill List 无分页**（wontfix）：树/名列语义 + 本地规模；前端树渲染本就要全量。

### by-design 记录（矩阵空格、确认有意）

- apikey/workspace/sandbox/notification/attachment 无 LLM 工具组——安全边界（密钥/机器管理不归 LLM）或无意义（通知是人的收件箱）。
- memory/skill 无 search 工具——发现走 prompt 注入（MemoryProvider 索引 / CatalogProvider 能力概览），按名直读。
- LLM 无 approval 决策工具——human-in-loop 红线：决策永远是人的。
- 矩阵六件套（执行→记录→人查→LLM 查→诊断→过程可见）：六类可执行体全格 ✅（exec-observability + 本轮补齐）；:triage 六前缀全覆盖；:iterate 八实体（六版本实体+document+trigger）。

## R3 角色旅程走查

### 实锤·已修

- **PR-17 🔴 异步完成的唤回环整体缺失——run 失败/审批挂起不通知任何人**（fixed）
  验证：`markRunTerminal` 只写库 + drain reconcile（kill.go:99），approval park 只写 parked 行——notifications 流**零事件**；entities 流的 NodeRun Signal 只够到正开着面板的人。更实锤的是 `workflowapp.SetNeedsAttention` 注释明写「The scheduler raises this when a run fails」且 `attention_changed` 事件齐备，但**全仓零调用方**——与 todo_write/limits 同款「设计了没接线」。旅程断点：用户 activate workflow 后离开 → run 失败/等审批 → 永远不会被唤起（除非主动回查）。
  修复：scheduler 加 `SetNotifier`（best-effort，通知绝不连累 run）——failed → `workflow.run_failed` + 经 LifecycleReconciler 新口 `MarkRunAttention(true, reason)` 点灯；completed → 熄灯（**自愈语义**，免 acknowledge 端点）；cancelled 两不做（手动终止非故障）；approval park → `workflow.approval_pending`（at-least-once，重复唤起优于静默卡死）。workflowapp.MarkRunAttention 幂等（旗标一致不写不发）。`TestRunTerminal_NotifyAndAttention` + `TestApprovalPark_Notifies` 钉死。

### 旅程走查台账（断点已清面）

- LLM 搓 workflow 全旅程：search_blocks → create/edit（ops+capability_check）→ trigger_workflow → **get_flowrun**（R2 补）→ :triage/edit 循环 ✅。
- 用户调试失败 function：执行历史 → 详情+**logs**（exec-observability 补）→ :triage → :iterate ✅。
- 装 MCP 排错：marketplace（requiredEnv 显式）→ install 自动连 → reconnect 重置 → 调用失败附 stderr 尾 ✅。
- 配模型/密钥并验证：CRUD → `:test` probe → capabilities 聚合 → 场景配置即时生效（R1 验）✅。
- 审批人在环：park → **通知唤回**（本轮）→ inbox → decide first-wins → 续跑 ✅。
- boot/崩溃/恢复/退出：Recover 重走 + SweepOrphans + 优雅关停逆序（二轮 review 验）✅。

### 轻症

- **PR-18 🟢 function/handler env 构建失败无通知**（closed——复验已覆盖）：sandbox 层 `env_status_changed` 对 failed 同样 emit 且带 errorMsg（sandbox.go:415+publishEnvStatus），唤回在场；当初定性时只看了 function 层事件、漏了 sandbox 层。

## R4 横向一致性

### 实锤·已修

- **PR-19 🟡 Activation/Firing 整 struct 无 json tags——HTTP 线缆 PascalCase，违 N3**（fixed）
  验证：`GET /triggers/{id}/activations` 已在暴露 Activation，序列化字段为 `ID/TriggerID/Fired/FiringCount`（Go 字段名），全系统唯一违 N3 的线缆面；Firing 同病（彼时未暴露）。已补全两 struct 的 camelCase json tags（workspace_id → `json:"-"`）。
- **PR-20 🟡 firing 收件箱零可见性——「触发了为什么没跑」答不了**（fixed）
  验证：trf_ 表有完整处置状态机（pending/started/skipped/superseded/shed——overlap 政策与资源 shed 的判决），但无任何读取口；activation 只记 firingCount，skip/shed 的去向用户不可见。补 `SearchFirings`（domain filter + store + service + `GET /triggers/{id}/firings?status=`）+ store 测试。
- **PR-21 🟡 mcp_calls 无 ok/failed 聚合——四个执行体里唯一缺徽标数据的**（fixed）
  验证：fn/hd/agent 的执行历史都带 ComputeAggregates，mcp 没有——面板一致性断档。补 domain `CallAggregates` + store（filter 复用重构）+ app `SearchCalls`（镜像 handlerapp 形状）+ HTTP/工具双面换装。
- **PR-22 🟢 执行类工具互导参差**（fixed）
  验证：trigger_workflow→get_flowrun、search/get 系列已互导（前几轮补），但 invoke_agent 只说「recorded in history」不点名工具、call_handler 完全不提、run_function 不提。三处描述补点名（search/get_*_executions / calls）——LLM 不读文档，工具描述就是它的全部地图。

### 一致性台账（核过、齐）

- 六版本实体（fn/hd/ag/ctl/apf/wf）：CRUD+versions+revert+iterate+事件族对齐 ✅（handler 特有 restart/config、wf 特有 lifecycle 是合理差异）。
- 执行四件套（fn/hd/ag/mcp）：溯源 5 列 + logs/transcript + get/search 工具 + 聚合（本轮补齐 mcp）✅。
- 分页（N4）：全 List 走 ParsePage ✅（document/memory/skill 树/名列豁免，R2 已记）。
- 错误码（S20）：机械守卫在 verify 内，无需重扫。
- entities 流 run 终端：fn/hd/ag/mcp + wf 节点信号 + trigger fire 信号 ✅。

## R5 前端对接预检

### 实锤·已修

- **PR-23 🟢 flowrun 列表无 status 过滤**（fixed）：「看所有失败的 run」此前要全拉自滤。ListFilter+store 谓词+HTTP `?status=`+search_flowruns 工具同步。

### 面板反推台账（全核齐）

- 实体列表/详情/版本页：list+get+versions+聚合徽标（执行四件套）✅。chat 页：messages 流全块系+重连重水合 ✅。设置页：models(:test+capabilities)/keys/limits(GET/PATCH)/search settings(含 ollama)/webFetchMode/sandbox(runtimes/envs/disk-usage/gc/bootstrap-status) ✅。通知中心：list/unread/mark + run_failed/approval_pending 唤回 ✅。审批收件箱：flowrun-inbox+decide ✅。trigger 面板：activations+firings(R4)+fire 信号 ✅。综搜/关系图/catalog/workspace 切换器 ✅。

### 轻症·记录

- **PR-24 🟢 实体列表页无执行聚合摘要**（wontfix）：卡片想显示 ok/failed 徽标需逐实体查 executions 聚合——本地 app N 请求廉价 + 前端可懒加载可见区；列表带聚合是 N+1 JOIN 复杂度，不值。
- **PR-25 🟢 无全局活动总览端点**（wontfix）：「dashboard 首页」可由通知中心+各 list 组合；后端聚合首页是前端形态定型前的过早抽象。

