---
id: DOC-040
type: decision
status: active
owner: @weilin
created: 2026-06-14
reviewed: 2026-06-14
review-due: 2099-12-31
audience: [human, ai]
---

# 0004 — 前端架构:Flutter 桌面端 + sidecar + Riverpod 三层

## 背景

后端已定型(4 层 Clean、Quadrinity、durable 引擎、32 域全通),契约高度规整(ADR [0003](0003-api-contract-standardization.md):统一 Envelope / 裸实体 / `{data,nextCursor,hasMore}` / `202{id}` / `204` / camelCase / 三条 SSE 流)。前端原计划 FSD + TypeScript(已弃,前版在 `version-0.2` 分支),**改定为 Flutter 桌面端**(macOS/Linux/Windows)。

前端复杂度的真实来源不在 CRUD,而在三处:① **三条常驻、workspace 级、后端不过滤的 SSE 流**(前端自滤 + 断线续传 + 410 重取 + ephemeral/durable 分级);② **流式 chat block 树**(text/reasoning/tool_call/tool_result/progress/compaction + parentBlockId 嵌套 subagent 子树 + danger 内存阻塞确认);③ **workflow 图编辑器**(node/edge 画布 + CEL input + 类型化端口)。架构必须在开局就把这三处的形状钉死,否则后期难回退。

决策经一轮设计评审团(4 个带不同偏见的独立提案 + 对抗批判 + 综合)压测:全部独立收敛到下列方案;评审团提出的 7 条修正经逐字核代码后采纳,2 条"最高风险修正"经核代码证伪(见取舍)。

## 决策

**Flutter 桌面端,作为既有 Go HTTP+SSE 后端的纯客户端,架构十轴定型如下。**

1. **进程模型 = sidecar**:Flutter app 把 `cmd/server` 二进制作为子进程托管(启动拉起 / 健康门控 / 崩溃有界重启 / 随 app 退出关停);后端监听 Dart 端预抢的临时端口,经既有环境变量注入 —— **零后端改动**。已核实物理事实:`ANSELM_ADDR`(空→`:8080`)、`ANSELM_DATA_DIR`、`ANSELM_DEV` 三个 env 已存在([cmd/server/main.go](../../backend/cmd/server/main.go));`GET /api/v1/health` liveness 探针已存在且豁免 workspace 鉴权([bootstrap/build.go](../../backend/internal/bootstrap/build.go))。流程:Dart `ServerSocket.bind(loopback,0)` 抢端口→关→`ANSELM_ADDR=127.0.0.1:<port>` 传子进程→启动屏轮询 `/api/v1/health` 至 200。`ANSELM_BACKEND_URL` 为 dev 逃生口(优先于自管 sidecar)。后端未就绪/崩溃由单一 `BackendStatus` provider 驱动全局横幅 + 暂停 SSE 重连。

2. **分层 = 3-tier feature-first(对齐 Clean 精神,不照搬 4 层)**:`shared/core`(跨切:contract/net、SSE gateway、design system、i18n、router、process)→ `features/<域>`(每片自管 data+state+ui)→ `app`(装配根 + shell)。**砍掉 use-case 层**——客户端零业务规则,Go 二进制即 use-case 层,repo 方法 + Riverpod Notifier 即"用例";per-call Interactor 是纯仪式。唯一保留的框架无关纯模型层:`features/chat/model/BlockTreeReducer` 与 `features/workflow/model/GraphModel`+校验器(承载性正确,须脱离 widget/socket 单测)。

3. **状态 = Riverpod 2.x(riverpod_generator + riverpod_lint),非 Bloc**:app 约九成是 server-state + 三条推流。`AsyncNotifier`(分页列表 + `loadMore()`)/ `@Riverpod(keepAlive) Stream`(三条会话级流)/ `autoDispose.family`(按实体/对话 scope)三原语 1:1 贴合本 app 的三种状态形状;编译期安全 DI 取代 get_it。**采纳 Bloc 的一个好习惯**:每个"帧→状态"应用写成**纯函数 reducer**(Riverpod 仅托管),用录制帧日志做零 mock 单测。

4. **SSE = 单 `SseGateway`(纯 Dart,app/ 持有),三连接**;每连接手写 SSE 行解析(`event: stream\nid:<seq>\ndata:<json>`),续传发 `Last-Event-ID`(回退 `?fromSeq`),seq=0 ephemeral 不进续传游标,15s keep-alive 容忍,410 → 发 `ResyncRequired(scope-kinds)`(gateway 域无关、订阅方自己走 REST 重取再从新 head 续)。**关键:在 Riverpod **下面**垫 `Map<Scope,Stream>` demux**——gateway 把帧预分桶进 per-scope broadcast controller,family provider 包预分桶流;`StreamProvider` 每帧吐新 `AsyncData`、`.select`/`.where` 无法短路,直接 family 订阅会 O(帧×订阅者) 重建。**铁律**:DB 行是真相、流只为实时——delta/run/fire/interaction tick 仅改瞬时视图态、不进耐久缓存、不推进游标;唯 `Close` 快照 + durable 通知信号触耐久态。durable 不在线缆上(`Signal.Ephemeral` 是 `json:"-"`,[frame.go](../../backend/internal/domain/stream/frame.go)),故必达/可丢从 **`stream + node.type`** 推断、非从 frame.kind。

5. **代码生成 = freezed + json_serializable,手写镜像 reference 文档(无 OpenAPI 生成器)**。**仅对真封闭集 seal**:4 frame 动词 / 6 block 型(有 store CHECK)/ 5 图节点 kind / 4 trigger 源 / model 场景。**协议级 SSE `node.type` 保持开放 `String` + `unknown` 兜底**(producer 定义、非穷举,[event.go](../../backend/internal/domain/stream/event.go));~256 错误码生成 enum + `.unknown(raw)` 兜底(仅命名 UI 分支的 ~30 个)。Dart 契约层 = 后端的又一份 doc-sync 纪律下的"reference 投影"。

6. **网络一次编码**:`EnvelopeInterceptor` 拆 `{data}` 到裸实体;`Page<T>{items,nextCursor,hasMore}` + 执行日志端点用 `PageWithAggregate<T,A>`(其 `data` 为 `{executions,aggregates}`);`ErrorInterceptor`→typed `ApiException(code,message,details,httpStatus)`;线缆已 camelCase 故无重命名表;`postAction(path)→data.id` 收口 202 异步动作律。**实体 edit 非统一**:fn/hd/wf 走 ops-list,**agent `:edit` = 全量 Config 替换 → 新版本**([api.md](../references/backend/api.md));repo 层建模此不对称。

7. **导航 = go_router + `StatefulShellRoute`**:常驻三栏桌面 shell(nav rail / list / detail + 可选右侧 inspector),三条 SSE 根流与 shell 导航期间不卸载;`redirect` 据 `workspace==null → /select-workspace`;窗口 chrome 走 `window_manager`;实体路由深链,版本/执行/iterate 嵌套子路由。

8. **DI = Riverpod provider(无 get_it/injectable)**:装配根 = `app/app.dart` 的 `ProviderScope`;`bootstrap.dart` 拉起 + 健康检查 Go 二进制,经 `overrides:[baseUrlProvider.overrideWithValue(url)]` 注入运行期发现的唯一值——镜像后端 bootstrap 的"唯一全知者"为 scope override。

9. **图编辑器 = 自绘画布 + 持久坐标**:已核实 `Node.Pos *Position{X,Y} json:"pos"` 是图一等字段、随 graph blob 落盘、`add_node`(整节点)与 `update_node`(顶层合并 patch 含 pos)均可持久化([workflow.go](../../backend/internal/domain/workflow/workflow.go) / [ops.go](../../backend/internal/domain/workflow/ops.go))。故带持久坐标的手绘画布(`Stack`+`CustomPaint`+`InteractiveViewer`)成立,节点位移经 `update_node {id,patch:{pos:{x,y}}}` 落盘;现成图库塞不下 yes/no 端口、control 分支端口、CEL input,故自绘。**图语义校验**(可达性、回边合法、ref 解析)留后端权威(`:capability-check` + `:edit` 的 `WORKFLOW_INVALID_GRAPH`);前端只做廉价稳定结构检查(id 唯一/无自环/ref 前缀配 kind)做即时红线——非门控、避双事实源漂移。

10. **纪律机械化**:层依赖规则用 `custom_lint`/import lint 守(对标后端 S13+`make verify` 门禁文化,features 互不依赖);契约漂移用前端门禁守(错误码 enum 比对 `error-codes.md` + 每端点录制响应的全键集 smoke 测,testend 精神);i18n 走 slang、禁硬编码字符串(lint 守);design token 守"明亮通透轻盈 + 32px 行"。

工具链经 devbox(nix)管理(`flutter` 加入 `devbox.json`,与 `go`/`gnumake` 同管,S22)。

## 取舍

**为何不选:**
- **FFI 把 Go 编进 Flutter**:否决。丢现成 HTTP 契约 + `testend` 黑盒测试、跨平台 CGO 地狱,收益仅省一进程。sidecar 让后端零改、契约不变。
- **Bloc + 严格 4 层 Clean(含 use-case)**:否决。本 app 是 server-state + 推流主导,Bloc 的 event/state 类爆炸是单人开发该拒的仪式;其测试性优势靠"纯 reducer + Riverpod 托管"即可收回。4 层 + per-call use-case 是为可换持久化/可换 infra 而存在的力——客户端无这些力。
- **图编辑器只用 auto-layout / 给后端加 layout 字段开 ADR**(评审团标"最高风险修正"):**证伪**。`Node.Pos` 已是契约一等字段且可经 ops 持久化(见决策 9),手绘持久坐标画布无须任何后端改动。此为最贵的"开局错判",经读码逮回。
- **健康门控打 `/limits`**(评审团修正):**部分证伪**。`/healthz` 确无,但 `GET /api/v1/health` 专用 liveness 已存在,门控打它;sidecar 端口/数据目录现成 env 全支持(见决策 1)。
- **把协议级 SSE `node.type` 也 seal**:否决。它 producer 定义、非穷举,seal 会在新 tick 类型时崩解析器——仅 seal 真封闭集。
- **Riverpod `.select` 直接自滤工作区级流**:否决(评审团修正采纳)。`select` 按值短路、流每帧吐新值无法短路 → 必须在 Riverpod 下垫 plain-Dart demux。
- **agent edit 套用"edit=ops"**:否决。agent `:edit` 是全量 Config 替换(4 实体唯一例外),repo 层建模不对称。

## 后果

- **前端一套心智**:吃 ADR 0003 契约红利,每类端点(Create/List/action/error/SSE)解构一致,Dart DTO 层成为后端契约的 doc-sync 投影。
- **三处硬面集中治理**:SSE gateway(含 demux 垫层 + 断线 reconcile:重连对 `streaming` 态块走 REST tail 补、durable 实体帧 invalidate 分页 provider)、chat block 树(跨流订阅——`conversation.compacted` 落 notifications 流回投 contextRole)、图编辑器(自绘 + 持久 pos)各有定形方案。
- **状态文档同步**:本 ADR 落地后,`CLAUDE.md` 前端开发守则节(原 FSD/TS)整体重述为 Flutter 守则;`architecture.md` 路线表"前端重建(FSD)"重述为 Flutter;`references/frontend/` 按本架构填充。
- **本 ADR 不可变**:后续调整新建 supersede 篇,不改本篇。
