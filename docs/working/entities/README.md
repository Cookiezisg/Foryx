---
id: WRK-046
type: working
status: active
owner: @weilin
created: 2026-06-26
reviewed: 2026-06-26
review-due: 2026-09-24
audience: [human, ai]
---

# WRK-046 — Phase 4.1 Entities 建造规范(5 决策已拍板,STEP 0 已落)

> **一句话**:第一个真 feature——**Entities** 海洋。用它把"三岛布局(忠实复刻 demo)+ 运行时管道(Phase 4.0)"端到端走通。两轮调研合并:**(a) 后端契约 + demo 布局** `wd6a072aj` + **(b) 解决方案 best-practice** `wk6vkas9w`,两轮均经对抗验证(已折入纠正)。落地后结论提取进 `references/frontend/`(新 entities 域)+ 填 `landed-into`。

## 0. 执行摘要
- **范围**:**4 个可执行 Quadrinity kind 给详情页**——function(`:run`)/handler(`:call`)/agent(`:invoke`)/workflow(`:trigger`)。trigger/control/approval/mcp/skill 在左岛 rail **只显行、详情推迟**。
- **架构**:`features/entities/{data(repository+fixtures), state(Riverpod), ui(三岛 from An* kit), model}`;复用 Phase 4.0 全管道(ApiClient 全动词已有 / SseGateway demux / L2 CoalescingNotifier)。
- **契约**:字节级确认(stage-a,带 file:line);~22 个净新增 freezed DTO。ApiClient **唯一缺口** = `getPageWithAggregate`(包 `PageWithAggregate.fromBody`,page.dart 已有,client 缺动词)。
- **⚠ 对抗验证纠正(已折入)**:① **SSE 流路由**——**生命周期(created/edited/deleted…)全走 notifications 流**(events.md §38),**entities 流只承面板实时**(build 镜像/run 终端/flowrun tick);synthesis 原说"fn/hd/ag 走 entities"是错的。② 缺 `HandlerConfig` DTO。③ workflow 概览 graph = **只读 stub**(graph-canvas 是推迟原语)。④ 空/错/loading 是一等屏(G1 白屏教训)。⑤ events.md 生命周期动作词表不全 → `[doc-fix]` 一并补。

## 1. 待忠实复刻的三岛布局(概念级复刻;颜色/尺寸/动效/字形自由)
全部用现有 An* kit 组装,**绝不 bespoke**(20 个 kit 件已核存在)。

- **左岛 = 实体 rail**:`AnSidebarList`(`more` 行尾 …;model={newLabel:'New Entity', filterPlaceholder, groups})。**4 个固定可折叠超组**(rail.js:12-17):逻辑节点[function,handler,agent,trigger]/控制节点[control,approval]/工作流[workflow]/外部组件[mcp,skill] → 每 kind(≥1 实体)成可折叠 type-head(图标+标签+count,默认开)→ 实体行{id,label,meta,dot,…菜单}。空 kind/空组隐藏。New + 行内 filter。行点 → select(entity,id);… → openEntityMenu(单源动作表)。映射:`AnSidebarList`+`AnStatusDot`+`AnMenu`+`AnBadge`。
- **海洋 = AnPage 单滚动**居中 ~720;**绝对浮动 `AnOceanHeader`**(面包屑 Entities|<KindLabel> + 就地编辑 H1 + meta badge[tone=anTone(dot)] + 主 CTA=kind 动词钮[Run/Call/Invoke/Trigger]接右岛 run + … 菜单)over `AnTabs`(概览/版本/日志)+ 段(`AnField`/`AnKv`/`AnCodeEditor`/`AnVersionDiff`)。
- **右岛 = `AnInspector` run-terminal**:**仅可执行 kind 揭示**;run 前 idle(终端隐藏);动词 CTA 触发 → 实时输出。

## 2. 集成契约(stage-a,字节级;客户端 1:1 镜像)
- **Envelope/分页**:复用现成 `api_error.dart`/`page.dart`。裸实体规则:`data` 内永远裸实体、版本走嵌入 `activeVersion`(Create+Get 同形)。日志 → `PageWithAggregate`(listKey `executions`/`calls`,aggregates {okCount,failedCount})。
- **每 kind CRUD(统一)**:`POST /{plural}`(201 裸实体+activeVersion)· `GET /{plural}`(分页)· `GET /{plural}/{id}`(裸+activeVersion+computed)· `PATCH /{plural}/{id}`({name?,description?,tags?};wf 另 concurrency?)· `DELETE`(204)。版本:`GET /{plural}/{id}/versions`(分页)+ version-diff。日志:executions/calls(PageWithAggregate)。`:iterate`→conversationId。
- **执行动词**:fn `:run {args,version?}`(postBare→ExecutionResult)· hd `:call {method,args}`(postBare)· ag `:invoke {input,version?}`(postBare→InvokeResult)· wf `:trigger {payload?}`(**postForId→flowrunId,异步 202**)。
- **~22 DTO**(core/contract/entities/):Function/Handler/Agent/Workflow 的 entity+version DTO + 共享值类型 Field/ToolRef/MethodSpec/InitArgSpec/Graph/Node/Edge + ExecutionResult/InvokeResult/**HandlerConfig**(纠正补)。复用 ModelRef + Page/PageWithAggregate。**只 seal `EntityKind`+`NodeKind`(unknown 兜底)**;envStatus/configState/runtimeState/lifecycleState/concurrency/Field.type 等小集**保持 open String 常量**。

## 3. SSE 对账设计(已纠正 — 这是 stage-b verify 的 load-bearing 修正)
**唯一可丢性规则**(frame.dart):`durable == seq>0`。durable 进缓存+进游标;ephemeral(seq=0)只入 L2 CoalescingNotifier 瞬时视图(可丢,DB 行是真相)。

- **列表对账 = notifications 流**(events.md §38):所有 kind 的 `<kind>.{created,edited,reverted,updated,deleted,…}` 都在 notifications 流(fn 另 env_rebuilt;hd 另 restarted/config_updated/config_cleared/crashed;wf 另 lifecycle_changed/attention_changed/run_failed/approval_pending;mcp/skill/document 各自族)。notifications 帧 scope=notification:<id>、node.type=`<域>.<动作>`——**按 node.type 前缀过滤**(生命周期低频,此处 `.where` 可接受;demux-by-scope 对 notifications 不分 kind)。列表 notifier 据 id **就地 patch `AsyncData`**(created→插、deleted→移、其余→改),**绝不全量重取**;410 resync→`invalidateSelf` 重读 REST。
- **详情面板实时 = entities 流**(events.md §50/§70):`gw.scopeStream(StreamScope(kind.wire, id))`——build 镜像(open/delta/close 填概览)+ run 终端(fn stderr/hd yield/ag ReAct 轨迹,ephemeral→L2 coalescer)+ **workflow flowrun 节点 tick**(ephemeral Signal {flowrunId,nodeId,iteration,status})。
- **L2 coalescer 只在详情 ephemeral 路径**(run 终端/tick);列表 durable patch 不经它。

## 4. Feature 结构(stage-b)
- **data/**:每 kind 一个 `Repository` 抽象(`FunctionRepository` 等)front 现成管道(ui/state 不直碰 ApiClient/SseGateway)。Live impl 包 apiClientProvider/sseGatewayProvider;Fixture impl 返港自 demo data.js 的内存数据 + 脚本化 FakeGateway(含 empty/error/delay 态)。`Provider<XRepository>`(非 Notifier)。+ `entity_kinds.dart`(单源 kind→{label,icon,verb,plural,idPrefix} + kindIconOf)。
- **state/**(经典 Riverpod,非 codegen):`entityListProvider(EntityKind)` = `FamilyAsyncNotifier`(首页 + `loadMore` 保 hasValue + `_onSignal` 就地对账,build() 内订阅 notifications kindStream、`ref.onDispose` 撤);`railModelProvider` = `Provider<SidebarModel>` **同步派生** 4 超组(watch 各 kind list,零额外网络);`entityDetailProvider(id)` = family(实体+activeVersion + 订阅 entities scopeStream patch);版本/日志 paginated provider。
- **ui/**:`entity_rail.dart`(AnSidebarList)· `entity_sea.dart`(AnPage+AnOceanHeader+AnTabs+段)· `entity_inspector.dart`(AnInspector run-terminal)· 空/错/loading 用 `AnState`/`AnSkeleton`。
- **select/路由**:见 §7 决策。

## 5. fixtures-first 数据策略(两 rung)
- **Rung 1 仓库 override**(日常 UI 主战场,无后端):`ProviderScope(overrides: kUseFixtures ? [xRepositoryProvider.overrideWith((ref)=>FixtureX()), …] : [])`。**用 `overrideWith` 非 `overrideWithValue`**(Riverpod 3.x #4324)。Fixture 暴露 emitCreated/Deleted/Edited + empty/error/delay 模式。
- **Rung 2 传输级 fixture**(契约保真 + demux/coalescer 正确性):换 Dio `httpClientAdapter`(net 测已有 `_FakeAdapter` 范式)+ `SseGateway` 的 `connectionFactory`(STEP 3 加的 @visibleForTesting 缝)注脚本化连接。
- 录制真后端 JSON(make server + curl)→ `test/fixtures/entities/<kind>/*.json` 做 DTO 往返 golden。

## 6. 有序构建步骤(fixtures-first;每步 gate;`make fe-verify` 滚动门禁 + 真机截图里程碑)
- ✅ **STEP 0 DTO 契约 + golden**(地基,单作者,阻塞全部)— **已落**(`core/contract/entities/{values,function,handler,agent,workflow,common}.dart`,~22 DTO + codegen 入库 + 16 golden 往返,`make fe-verify` 746 测绿,投影进 [`contract.md`](../../references/frontend/contract.md))。Gate 达成:每 DTO fromJson↔toJson key-equal;NodeKind unknown 兜底;`default` 保留字 rename;bare `FunctionRunResult`/`InvokeResult` 解码;`explicit_to_json` 嵌套对象。**`HandlerConfig` DTO + `getPageWithAggregate` client 动词推迟到首个消费它们的步骤**(分别是 handler config-CRUD / STEP 1 repository)。
- ✅ **STEP 1 Repository 缝 + fixtures** — **已落**(`features/entities/data/`):单一 `EntityRepository` 缝(list/get/versions/logs/flowrun/execute/signals 全 4 kind)+ `LiveEntityRepository`(接 ApiClient[补 `getPageWithAggregate`——日志页聚合嵌 `data.aggregates`]+ SseGateway demux;生命周期走 notifications rawStream[全帧 scope.kind=notification,故按 node.type 投影]、面板实时走 entities scopeStream)+ `FixtureEntityRepository`(内存 typed 种子 + keyset 分页 + 可脚本 `emitLifecycle`/`emitPanel`)+ `entityRepositoryProvider`(单点 override)。辅型 `EntityKind`(4 可执行 kind 的 REST/scope/verb 常量)/`EntityRow`(统一 rail 行投影 + 徽标)/`EntitySignal`(生命周期投影)。Gate 达成:fixture 返 Page/PageWithAggregate + keyset loadMore;脚本信号对账(EntitySignal.fromEnvelope 动作词表 + durable);Live 传输 rung 端到端(fake adapter 验路径/请求体/嵌套聚合/flowrun 复合/裸结果)。`make fe-verify` 764 测绿。
- ✅ **STEP 2 列表 state + rail VM** — **已落**(`features/entities/state/`):`entityListProvider`(AsyncNotifier.family over EntityKind——首页 + `loadMore` keyset 追加[`loadingMore` 在 data 内,翻页不打回 spinner]+ `_onSignal` SSE 就地 patch[created 取行前插/deleted 删/edited·updated 重取替换/未载页忽略,每 await 后重读 state 防并发])+ `railModelProvider`(扇 4 kind 成有序 RailGroup + count)。build.yaml freezed scope 加 `features/**/state/**`。Gate 达成:ProviderContainer 测(首页/loadMore 全程 hasValue;created·deleted·edited durable 就地 patch;ephemeral/seq=0 不动列表;rail VM 计数)。`make fe-verify` 772 测绿。
- ✅ **STEP 3 rail UI(左岛)** — **已落**(`features/entities/ui/`):`EntityRail`(ConsumerWidget)over `AnSidebarList`——单平铺组 + 4 kind 折叠段(icon[thin Lucide]+ label[i18n `ref.<kind>`]+ count)+ 实体行(status dot:handler runtime / workflow lifecycle·attention,经 `AnStatus.fromRaw`[补 `crashed`→err])+ loading 骨架 / error(可重试,关 Riverpod 自动重试)/ empty / loaded 四态;选择写 `selectedEntityProvider`(EntityRef,STEP 6 接 go_router)。纯投影 `entity_rail_model.dart`(可脱 UI 单测)。i18n 加 `entities.*` + 复用 `ref.*`。Gate 达成:widget 测四态 + 选择;**真渲染截图核对**(`test/dev/capture_demo.dart` → PNG,肉眼验证图标/状态点/分组)。整合规范:app 与 demo 共用 `app/app_shell.dart`(rail + ocean 占位),`make demo`(fixture 零后端)= 手动验收面。**rail 完整度**:过滤(AnSidebarList 自带,已生效)+ 排序 sliders 菜单(`railSortProvider`:最近更新/名称,`sortRows` 稳定排序)。`make fe-verify` 782 测绿。
- ✅ **STEP 4 详情 sea(海洋)** — **已落**(`features/entities/{state/detail,ui/detail}/`):**文档模型(对齐 demo,用户复审纠正)**:整海洋 = 单一 `AnPage`(居中 720 阅读列 + 唯一滚动)——头 + tab 条 + 内容**一起往下滚**;`AnTabs` 加 **flow 模式**(条 + 仅选中面顺排、随内容高,不再 Expanded+IndexedStack 填充——demo an-tabs 模型);版本/日志 tab 内部从 ListView 改 Column(滚动归外层 AnPage 一个)。
`EntityOcean`(选区 null→空态)= `AnOceanHeader`(面包屑 + 名称 + 各 kind 状态徽[version/env/config/runtime/lifecycle/mount-health,经 `AnStatus.fromRaw`,补 ready/syncing/crashed/…]+ 动词 CTA[Run/Call/Invoke/Trigger,STEP 4 禁用占位]+ ⋯)+ `AnTabs(flow:true)`(概览/版本/日志)。**4 kind 概览**(`overview/*_overview.dart`,复用 AnKv[非 AnThinTable]/AnField/AnCodeEditor[只读]/AnInfoCard;handler 敏感默认遮蔽、agent 工具按 ref 显 fn/hd/mcp 图标 + mount-health、**workflow 编排图可视化 + 图编辑器入口推迟到图编辑器阶段**[概览只在 KV 给节点/边计数,不渲图])。**版本 tab** = 版本列表 + `AnVersionDiff` 相邻版本(src 按 kind:code/handler 拼接/prompt/graph json)。**日志 tab** = ok/failed 聚合头(workflow 无)+ `AnRowDetail` 行展开 + loadMore;workflow flowrun 首展开懒取 `getFlowrun` 节点列表。providers:`entityDetail`(双流订阅:durable 生命周期→重取 + 失效 version/log,ephemeral no-op[build 镜像/run 终端归 STEP 5],deleted→清选区)/`versionList`/`logList`,均关自动重试。纯助手 `entity_format`(handler 源拼接/graphOf 解码[graphParsed 生产为空]/fmtTime/prettyJson)。i18n `entities.detail.*`(tab/sec/card/kv/state/val/verb)。Gate 达成:provider 测(各 kind 解析/agent mount-health/no-retry/durable 重取/deleted 清选/ephemeral 不重取/版本 active+select+loadMore/日志聚合+toggle+workflow 懒取)+ widget 测(四态/各 kind 概览/敏感遮蔽/tab 切换)+ **真渲染截图**(`capture_demo.dart --dart-define=SEL=kind:id`:函数/智能体/工作流详情核对)。`make fe-verify` 798 测绿。
- **STEP 5 执行 + 右岛 run-terminal**:动词 CTA → run/call/invoke/trigger;ephemeral 终端经 L2 coalescer;idle 态。Gate:fixture 脚本流驱动终端,重建计数门禁(叶子≤1/帧);wf :trigger 显异步 flowrun id。
- **STEP 6 select + 路由 + 收尾**:见 §7;全 feature 五电池矩阵入 fe-verify + 真机截图终检。

## 7. 待你拍板(带推荐)
1. **范围**:4.1 = 4 可执行 kind 详情 + 其余 rail-rows-only。确认?(推荐:是)
2. **workflow 概览的 graph**:graph-canvas 是推迟原语 → 4.1 渲**只读 stub**(GraphModel 用 AnSection/AnKv/AnRow 列出节点/边,或至多简单 CustomPaint DAG)+ "进入图编辑器" nav intent(路由到未来编辑器海洋,暂 gate 成"敬请期待")。**绝不在 4.1 建交互画布**。确认?(推荐:是)
3. **select/路由**:go_router 路由(`/entities/:kind/:id`,deep-link+back 白送)**还是** 选择 provider(Notifier 持 {kind,id})?(推荐:**go_router**——已加依赖;海洋+右岛都 watch 路由参、rail 不 import 它们[跨 feature 经 shared])
4. **workflow 详情深度**:概览(graph stub)/版本/日志(flowruns)+ Trigger CTA 显异步启动态;**完整 flowrun 时间线** = 薄首切(显 flowrun id + 节点列表)还是推到 Scheduler 4.3?(推荐:薄首切,富时间线归 4.3)
5. **fixtures-first**:先港 demo data.js 做 fixtures 把 UI 全建起来,再接 seed/真后端。确认这个开发回路?(推荐:是)

## 8. 数据/复核
- digest:`scratchpad/wf6_digest.json`(stage-a 契约+布局)· `wf7_digest.json`(stage-b 方案)· brief `wf6_brief.txt`。原始:`tasks/{wd6a072aj,wk6vkas9w}.output`(含全部 file:line + AsyncNotifier/反模式)。
- 验证:stage-a verdict=needs-revision(4 缺口已折入 §0/§3)· stage-b verdict=REVISE(SSE 路由纠正已折入 §3)。
