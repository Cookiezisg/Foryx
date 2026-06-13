---
id: WRK-021
type: working
status: active
owner: @weilin
created: 2026-06-13
reviewed: 2026-06-13
review-due: 2026-09-13
expires: 2026-09-13
landed-into: ""
audience: [human, ai]
---

# 符合性矩阵 + 归一化波次（第 2 轮产物,2026-06-13）

> 以批准的 [CHARTER.md](CHARTER.md) 当尺子,14 切片叶子穷尽对标 682 项:**167 项已符合 / 153 条原始偏差 → 去重 25 条净偏差(ST-1..25)**。high 13 / medium 9 / low 3;**改线缆契约 17 条**(前端可见,前端正重建→优先改)/ 纯内部·文档 8 条。聚成 **9 个执行波(S1-S9)**,排序:线缆形状在前、内部重命名+文档勘误殿后。

## 偏差总账（ST-1..25,全部 file:line 亲验）

| ST | 波 | 轴 | 级 | 面 | 偏差 → 修法 |
|---|---|---|---|---|---|
| ST-1 | S1 | envelope | high | 线缆 | Create `{entity,version}`/`{trigger}` wrapper → 裸实体 + 内嵌 `currentVersion`(7 端点) |
| ST-2 | S2 | action | high | 线缆 | 执行/异步动作新 id 键统一 `{id}`(messageId/flowrunId/conversationId/triggerId→id) |
| ST-3 | S2 | action | med | 线缆 | trigger `:fire` 异步动作收单产物 id：去 `fired`(被 202 蕴含)+ `triggerId`(在 URL 路径)，留 `{id:activationId}`(MD3，执行落定见 S2 ✅) |
| ST-4 | S3 | action | high | 线缆 | `:stage`/`:kill` 返实体后置快照,禁 `{staged:true}`/`{killed:N}`(MD4) |
| ST-5 | S3 | action | med | 线缆 | handler `:restart`/`Call`、skill `:activate`、mcp `:call` 去 data 内层多余包裹 |
| ST-6 | S3 | action | med | 线缆 | chat `:resolve-interaction` → 204 NoContent |
| ST-7 | S3 | action | med | 线缆 | search `:reindex` → 204 NoContent |
| ST-8 | S3 | action | med | 线缆 | document `:delete` `{id,deletedCount}` → 204 NoContent |
| ST-9 | S4 | action | high | 线缆 | notification `/{id}/read`·`/read-all` → `:mark-read`·`:mark-all-read`(MD5) |
| ST-10 | S4 | action | high | 线缆 | chat `DELETE /{id}/stream` → `POST /{id}:cancel`(GET /stream 订阅不动) |
| ST-11 | S4 | naming | high | 线缆 | 路径占位 `{conversationID}` → `{conversationId}` |
| ST-12 | S4 | identity | high | 线缆 | Log 单读路径变量 `execId/callId/actId` → `{id}`(5 handler 10 点位) |
| ST-13 | S5 | pagination | high | 线缆 | search 自解析 limit → `ParsePage` |
| ST-14 | S5 | pagination | high | 线缆 | 执行/调用/搜索 List 用 `Paged`、aggregates 进 data 子对象(修唯一埋 data 偏离) |
| ST-15 | S6 | errors | high | 线缆 | transport 21 处裸 `Error`/`http.NotFound` → sentinel 化经 `FromDomainError` |
| ST-16 | S6 | errors | high | 线缆 | 补登 error-codes.md 漏登码 + standard_test 加裸 Error AST 守卫 |
| ST-17 | S7 | sse | high | 线缆 | `Signal` 加 `ephemeral` 形参 + flowrun tick/trigger fire 置 true(MD-sse1) |
| ST-18 | S9 | sse | med | 内部 | events.md 登记 node.type 词表 + `conversation.compacted` |
| ST-19 | S9 | identity | high | 内部 | database.md ID 前缀勘误+补登(noti_/aki_/sr_/se_/sig_/bsh_/subagt_) |
| ST-20 | S8 | internal | med | 内部 | Service 构造器 12 处裸 `New` → `NewService` |
| ST-21 | S8 | skeleton | high | 内部 | 7 个 `search_<entity>`+contentsearch 本地 slim struct 抽 domain 级 `EntitySlim` |
| ST-22 | S8 | skeleton | med | 线缆 | `ComputeAggregates` → `Compute<Entity>Aggregates` 统一 |
| ST-23 | S8 | action | med | 线缆 | `list_documents`/`search_documents` tool prose → `ToJSON`(MD7) |
| ST-24 | S8 | internal | med | 内部 | `agent.List` 签名 raw int/string → `ListFilter` 结构体(与 control 一致) |
| ST-25 | S9 | envelope | low | 内部 | mcp `:install` 用 `Created` helper / flowrun Inbox key 核准 |
| ST-26 | S1 | naming | high | 线缆 | **trigger domain 结构体缺 json tag → 序列化 PascalCase(`ID`/`Name`/`Config`)且 `WorkspaceID` 上线缆(D2 漏)**。S1 执行时由「裸实体 + 大小写敏感解包」暴露(此前被 Go 大小写不敏感 unmarshal 掩盖)。审计 naming 轴误判"基本达标"的反例。全量复扫确认 trigger 是唯一缺 tag 实体。已随 S1 补全双 tag |

## 九波执行计划（每波独立提交;exit = make verify 绿 + 涉线缆补 testend + api/error-codes/events/domains 1:1 同步）

**S1 [改线缆·中险] Create 响应裸实体化**(ST-1)：6 版本实体 domain 结构体加 `CurrentVersion` 双 tag 字段;function/handler/agent/approval/control/workflow `Created(w, {entity,version})` → `Created(w, entity)`;trigger `{trigger:t}` → `Created(w, t)`。

**S2 [改线缆·低险] 执行/异步动作 id 键统一 `{id}`**(ST-2/3)：chat.go:77/workflow.go:229/aispawn.go:35,82 → `{id}`;agent `:invoke` 改 202 `{id:execID}`(全量走 GET);trigger `:fire` 去 `fired` 保多键。

**S3 [改线缆·中险] 状态变更/无产物动作收口**(ST-4/5/6/7/8)：workflow `:stage`/`:kill` 改 Service 签名返 wf 快照;handler `:restart`/`Call`、skill `:activate`、mcp `:call` 裸返;chat `:resolve`/search `:reindex`/document delete → 204。

**S4 [改线缆·中险] URL 语法+路径占位归一**(ST-9/10/11/12)：notification `/read`·`/read-all`→`:mark-read`·`:mark-all-read`;chat `/stream` DELETE→`:cancel`;`{conversationID}`→`{conversationId}`;5 个 Log 单读 `{execId/callId/actId}`→`{id}`;flowrun `{nodeAction}`→`{idAction}`。集中改一次 testend 路径黑盒重写。

**S5 [改线缆·中险] 分页统一**(ST-13/14)：search 删自解析 limit 改 ParsePage + `Paged(Hits)`;function/agent/handler/mcp 执行调用日志 `{<plural>,aggregates}` 容器进 data + `Paged` 顶层。

**S6 [改线缆·中险] 错误 sentinel 化**(ST-15/16)：pkg/errors 新增 `ErrNotFound/ErrInternal/ErrStreamingUnsupported`;auth/notfound/recover/sse/mcp 复用既有 sentinel;document/attachment/sandbox/apikey/skill/workspace 校验类定义域 sentinel 经 FromDomainError;补登 error-codes.md;standard_test 加 `TestTransportErrorsUseFromDomainError`(transport 裸 `Error(`/`http.NotFound(` 计数==0)。

**S7 [改线缆·低险] SSE ephemeral 修正**(ST-17)：`entitystream.Signal` 加末位 `ephemeral bool`;scheduler advance(flowrun tick)/trigger report(fire)补 true;核验 chat interaction(已 true)、notification(durable)不动。

**S8 [内部/LLM 面·低险] 内部归一**(ST-20/21/22/23/24)：12 处 `New`→`NewService`+装配点;新建 `domain/search.EntitySlim` 替 8 处本地 slim;`Compute<Entity>Aggregates` 统一;document list/search tool prose→ToJSON;`agent.List` 改 `ListFilter`。

**S9 [文档·零险] 文档勘误殿后**(ST-18/19/25)：database.md 前缀 key_→aki_/ntf_→noti_/补 sr_·se_·sig_·bsh_·subagt_;events.md node.type 词表节 + compacted;mcp `:install` 用 Created;flowrun Inbox key 核准。

## 状态

- 第 1 轮(宪章)✅ / 第 2 轮(本矩阵)✅。
- **第 3 轮 = 执行 S1-S9**(用户批准自主跑完)。
  - **S1 ✅**(ST-1 + 新抓 ST-26):7 Create handler 裸实体化 + trigger json tag 补全;testend 11 文件解析点全转 `Field(t,"id")`;api.md 响应形状铁律;verify+testend 绿。
  - **S2 ✅**(ST-2/3):异步动作返新 id 统一 `{id}`——chat send(messageId→id)、workflow :trigger(flowrunId→id)、aispawn :iterate/:triage(conversationId→id)、trigger :fire(去 fired/triggerId,留 {id:activationId})。同步执行(:run/:invoke/:call)不在此列(返完整结果)。3 处 send helper 转 Field("id");api.md 加异步动作返 id 铁律。verify+testend 绿。
  - **S3 ✅**(ST-4/5/6/7/8):状态变更动作 :stage/:kill 返实体后置快照(经 Get,对齐 activate/deactivate)、handler :restart 同;Call/mcp :call/skill :activate 去 {result}/{output} 包裹返裸结果;chat :resolve / search :reindex / document DELETE → 204。testend handler/mcp result 解析转裸;bootstrap search 集成测试 reindex 断言改 204;api.md 加状态变更/无产物铁律。verify+testend 绿。
  - **S4 ✅**(ST-9/10/11/12):notification `/{id}/read`·`/read-all`→`:mark-read`·`:mark-all-read`(N5/MD5)、chat `DELETE /{id}/stream`→`POST /{id}:cancel`;路径占位归一(todo `{conversationID}`→`{conversationId}` camelCase、5 个 Log 单读 `{execId/callId/actId}`→`{id}` MD-id4)。flowrun `{nodeAction}` 保留(node-keyed 动作派发,与 mcp `{nameAction}` 同属 `{xAction}` 约定、更准确)。testend 5 处 URL 消费方更新;api.md 路由同步。verify+testend 绿。
  - **S5 ✅**(ST-13/14):分页统一——执行/调用列表(function/agent/handler/mcp)从 `Success(整 result 埋 data)` 改 `Paged({entities,aggregates} 进 data、nextCursor/hasMore 顶层)`;search 同(`{hits,total}` 进 data、分页顶层),limit 走新 `ParsePageBounded(20,50)`(取 ParsePage 错误语义、保 search 专属界)。harness `Resp` 加顶层 `NextCursor`/`HasMore`;searchQ/searchPageOf 从顶层取游标。api.md search/mcp-calls 响应形状同步。verify+testend 绿。
  - **S6 ✅**(ST-15/16):错误 sentinel 化——pkg/errors 新增跨域 `ErrNotFound`(404)/`ErrInternal`(500)/`ErrStreamingUnsupported`(500);全 transport(~21 handler + notfound/auth/recover 中间件)裸 `responsehttpapi.Error(...)`/`http.NotFound(w,r)` 一律改 `FromDomainError(w,log,sentinel)`——未知 :action / 路由未命中 → `ErrNotFound`(mcp `:explode` 等 400→404 归一)、recover panic → `ErrInternal`、SSE 非流式 → `ErrStreamingUnsupported`;真校验类新增定义域 sentinel:sandbox(`SANDBOX_OWNER_KIND_REQUIRED`/`SANDBOX_INVALID_OWNER_KIND` 400)、attachment(`ATTACHMENT_BAD_UPLOAD` 400、:60 复用既有 `ErrTooLarge` 413)、apikey(`API_KEY_TEST_FAILED` 422 带 latencyMs+reason details)。error-codes.md 补 7 码、头计数 256→263;standard_test 加 `TestTransportErrorsUseFromDomainError`(按 import 别名解析、transport handler+中间件裸 `Error(`/`http.NotFound(`/`http.Error(` 计数==0)。testend mcp `:explode` 断言 400→404。verify+targeted testend(mcp/apikey/sandbox/attachment/chat 错误路径)绿。
  - **S7 ✅**(ST-17):SSE ephemeral 修正——`entitystreamapp.Signal` 加末位 `ephemeral bool` 形参、置入 `streamdomain.Signal{Ephemeral}`;两处"DB 行才是真相"点状广播补 `true`:scheduler advance flowrun 节点 tick(flowrun_nodes 行真相)、trigger report fire 信号(Activation/Firing 行真相)——不再被当 durable 占 replay 环(E2)。chat interaction 已 `Ephemeral:true`(正面模板,未动);notification 信号必达、保持 durable(未动)。entitystream 单测加 ephemeral/!Durable 断言。events.md 加「Frame 协议与 node.type 词表」中央登记节(四动词 durable 分级 + Signal 硬规则 + node.type 当前全集·标非穷举,MD-sse2)+ flowrun/trigger 挂载行标注 ephemeral。verify+targeted testend(SSE/workflow/trigger 路径)绿。
  - **S8 ✅**(ST-20/21/22/23/24):内部/LLM 面归一。**ST-20** 12 个 `*Service` 构造器 `func New`→`NewService`(entitystream→Writer/humanloop→Broker/2 个 tool→Tool 不在列、非 Service);全 bootstrap 装配点 + 7 个包内单测 + 1 个跨包单测(tool/document)call site 同步;负向 lookbehind 仅改裸 `New(`、不碰 `pkg.New(`。**ST-21** 抽共享 `searchdomain.EntitySlim{id,name,description}`——置于 `domain/search`(紧邻 `Hit`,该域本就持 json-tagged 结果 DTO 的既有先例);6 处纯形(contentsearch + function/handler/agent/control/approval)直用,trigger/workflow 内嵌它 + 加 kind·refCount·listening / lifecycleState·active。**ST-22** `ComputeAggregates`→`ComputeExecutionAggregates`(agent+function,含域接口/store impl/app caller/单测/错误串)——选 `Compute<LogType>Aggregates` 而非 `<Entity>`,与既有 `ComputeCallAggregates`(handler/mcp)对齐。**ST-23** `list_documents`/`search_documents` 散文→`toolapp.ToJSON({count,documents})`;search 双路径(内容引擎/原名字)共用 `docHit`(path/snippet omitempty);document_test 断言改 JSON 形状。**ST-24** agent 仓库 `List(limit,cursor)`→`ListAgents(ListFilter)`、新增 `agentdomain.ListFilter`、service `List(ListFilter)`、transport 传 `ListFilter{Cursor,Limit}`——与 function/handler/control 仓库形状完全对齐。search.md L29 同步(8 垂搜全 JSON、共享 EntitySlim)。verify + targeted testend(SearchLLM 垂搜/会话搜、chat 工具往返、agent 版本、search 分页、并行工具批)绿。
  - **S9 ✅**(ST-18/19/25):文档勘误殿后。**ST-18** node.type 词表已随 S7(MD-sse2)落 events.md;本波补 `conversation.{created,deleted,archived,pinned,auto_titled,model_override,compacted}` 全族(原仅登 auto_titled——经 grep s.emit 核出 7 个全为通知事件),`compacted` 标 payload {coversUpToSeq,summaryBytes}。**ST-19** database.md ID 前缀对齐代码全量(全库 `idgenpkg.New` 实扫 40 前缀 vs 登记表):勘误 `key_`→`aki_`、`ntf_`→`noti_`;补 sandbox 两表 `sr_`/`se_`;新增「运行时/infra 前缀(无表)」登记节 `sig_`(entitystream 信号帧)·`bsh_`(shell bash 进程)·`subagt_`(subagent run)·`hdi_`(handler 实例)。**ST-25** mcp `:install` `Success(w,201,st)`→`Created(w,st)` helper(全库唯一裸 201);flowrun Inbox key **核准通过**——FiringInbox 以 durable firing id(`trf_`)为键 + 单事务 claim(ADR-021)+ dedup `idx_trf_dedup`(D3),审批 inbox 的 parked `FlowRunNode` 序列化全 camelCase·`WorkspaceID json:"-"`(D2 不泄),无偏差、无须改。verify 全绿;`:install` testend(TestMCP_ImportAndRegistry)绿。
  - **三轮收口**:S1-S9 九波全绿、24 项偏差(ST-1..26)全部归一。下一步:working 文档结论提取 + 终版报告。

> **遗留(非本审计范畴)**:`TestMCP_OfficialFilesystemServer` 在 S8 提交态即失败(与 S9 无关、已 stash 复验)——真 npx 拉的官方 `@modelcontextprotocol/server-filesystem` 升级改了 `read_text_file` 返回形状(结果仅 28 字节、不含文件内容)。属上游 npm 版本漂移、网络依赖测试(不在 `make verify`),已另起后台任务跟。
