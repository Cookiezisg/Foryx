---
id: WRK-045
type: working
status: active
owner: @weilin
created: 2026-06-26
reviewed: 2026-06-26
review-due: 2026-09-24
audience: [human, ai]
landed-into: references/frontend/architecture.md
---

> **✅ STEP 0–7 全部落地(2026-06-26,commit `0c1c0d69`..`5e519546`)**:契约/net/SSE 自 main PORT+加固 ·
> sidecar 监督器(`core/process`)· 后端 loopback 三改(绑 127.0.0.1 + bearer + host,`make verify` 绿)·
> Riverpod 装配 + 错误边界 + 启动门控 MERGE 进 AnApp · L2 流式合并原语(`core/perf`,200 deltas→1 重建门禁)。
> 每步过 floor gate、独立提交、超强测试覆盖(契约 12 / net 8 / SSE 14 / process 6 / bearer+host 16 /
> 门控 3 / 错误边界 1 / 性能 3)。结论已提取进 [`architecture.md`](../../references/frontend/architecture.md) §2。
> **⚠ 遗留(非本篇引入,基线即存在,4.1 前需查)**:`/api/v1/entities/stream` 后端返 500(git stash 干净对照
> 确认与 STEP 5 无关),会卡 4.1 Entities 实时通道。**实施纠正 vs 规范**:`StateProvider` 是 Riverpod 3.x
> legacy → activeWorkspace 改用现代 `Notifier`;`explicit_to_json` 须开(嵌套 ModelRef 往返);SSE 续传需
> full-jitter(规范已记);`KindForbidden`→403 须新增(S6 元测试禁硬编 response.Error)。本篇留作建造存档。

# WRK-045 — Phase 4.0 运行时骨干 建造规范(✅ 已落地)

> **一句话**:把 Flutter 客户端的"承重水电"建起来——契约/net/SSE/进程托管/Riverpod 装配 + loopback 安全 + 错误边界,让 features(Entities 起)能安全对接后端。三轮调研合并:**(a) 后端契约理解** `w8q011l67` + **(b) 解决方案 best-practice** `wm42o1h1d` + **(c) 流式渲染性能** `wlup4cbx4`(回应用户"SSE 会不会每帧重渲染整页"的担忧)。落地后结论提取进 `references/frontend/architecture.md` + 填 `landed-into`。

## 0. 执行摘要
- **不是从零建,是 PORT + ADD + MERGE**:`main` 分支已有完整、贴后端、带 file:line 注释的 `core/{contract,net,sse}` + `app/backend_controller.dart`;`frontend-rebuild` 当年精简时把它们和依赖删了。**4.0 = 从 main 取回这四套 + 重新加依赖 + 把错误边界/启动门控织进现有 `AnApp`**(原则 #8:不重造已验证的抽象)。
- **⚠ 最大风险 = MERGE 不是 OVERWRITE**:rebuild 的 `main.dart`/`app.dart` 与 main **不同**(有 scaled_app/WindowZoom/AnApp/AnOverlayHost)。**绝不能整文件覆盖**,否则丢掉窗口/缩放/浮层成果。错误处理器加进现有 `main()`;`ErrorWidget.builder` 加进现有 `builder`;启动门控包在现有 `AnShell` 外。**逐行 diff,不 overwrite。**
- **后端要改三件**(你已授权,同提交守后端纪律):默认绑 `127.0.0.1` + `ANSELM_AUTH_TOKEN` bearer 中间件 + `RequireLoopbackHost` 防 DNS rebinding。
- **性能(你的担忧)已有硬方案**:7 层规则保证"一帧只重画变化的叶子、绝不整页",其中**地基原语(网关 demux + ephemeral/durable 分流 + 每帧合并器 + 重建计数门禁)在 4.0 就建**,真正的叶子 widget 写法在 Chat 4.2 落地。见 §5。

## 1. 集成契约(客户端 1:1 镜像,源:后端读码 w8q011l67,带 file:line)
- **Envelope**:成功 `{data:<裸实体|裸结果>}`;错误 `{error:{code,message,details?}}`;分页 `{data:[...],nextCursor?(末页省略),hasMore:bool}`,`data` 永不为 null。分页参数 `?cursor=<opaque>&limit`(默认 50/上限 200;搜索 20/50)。
- **状态/Kind**:400/401/404/409/422/413/415/429/502/503/504 + 202(异步动作回 `{data:{id}}`)+ 204 + 499 + **410(=SSE SEQ_TOO_OLD)** + 500。~261(代码里约 487 处字面量)wire code,**客户端 OPEN 集 + unknown 兜底**。
- **workspace 轴**:header `X-Anselm-Workspace-ID`(SSE 用 `?workspaceID=` 查询);缺/未知 → 401 `UNAUTH_NO_WORKSPACE`。
- **SSE(最硬)**:3 条流 `GET /api/v1/{messages,entities,notifications}/stream`。线缆:`event: stream\n` +(仅 durable)`id: <seq>\n` + `data: <json>\n\n`(**冒号后有空格**)。载荷 `{seq,scope,id,frame:{kind: open|delta|close|signal,...}}`。**seq=0=ephemeral**(delta/tick,瞬时,不进缓存不进游标);**seq>0=durable**(patch 缓存 + 进 Last-Event-ID)。close 帧带快照。陈旧续传→410→**REST 重取真相再重订**。workspace 级、**服务端不过滤,客户端按 scope 自滤**。messages 支持 `parentBlockId` 嵌套(subagent 树)。**铁律:DB 行是真相,流只为实时。**
- **进程**:Dart 预抢 loopback 端口 → spawn `cmd/server`(env `ANSELM_ADDR=127.0.0.1:<port>` + `ANSELM_DATA_DIR` + `ANSELM_AUTH_TOKEN`)→ 轮询 `/api/v1/health`(将要求 token)到 200 → `BackendStatus` 门控 UI;退出 SIGTERM;有界崩溃重启。`ANSELM_BACKEND_URL`=dev 挂已跑后端。

## 2. 分层建造方案(PORT 哪些 / ADD 哪些)
| 层 | 做法 | 细节 |
|---|---|---|
| `core/contract` | **PORT** `{workspace,page,api_error}.dart` 逐字 + **ADD** | `page.dart`/`api_error.dart` 已契约精确零改;`Page.isLastPage` 已兼容 nextCursor 省略 + hasMore:false。ADD `AnselmErr.unauthBadToken`(401→重启后端横幅,非重选 workspace)+ `seqTooOld`(410,此处登记、SSE 层处置)+ 新 DTO/enum(sealed + unknown 兜底)。 |
| `core/net` | **PORT** `api_client.dart` 逐字 + **ADD 1 拦截器** | ADD `Authorization: Bearer <ANSELM_AUTH_TOKEN>` 拦截器(token 回调、非空才挂)。workspace-id 拦截器已在。 |
| `core/sse` | **PORT** `{frame,sse_connection,sse_parser,sse_gateway}.dart` 逐字 + **1 修** | **加 full-jitter 退避**(原注释"单本地连接无需 jitter"对我们拓扑是错的,唯一真缺陷)。durable=seq>0 才进 Last-Event-ID(已对齐 stream.go)。 |
| `core/process` | **PORT** `backend_controller.dart` → 移到 `core/process/`(ADR 0004 归位) | 预抢端口 + spawn + health 门控 + SIGTERM + 有界重启 + 排空 stderr。 |
| Riverpod 装配 | **MERGE 进现有 AnApp** | `ProviderScope` override(workspace/baseUrl/authToken);401/410 拦截;3 条 keepAlive SSE。**不覆盖** scaled_app/WindowZoom/AnOverlayHost。 |
| 错误边界 + bootstrap | **MERGE 进现有 main()/builder** | `runZonedGuarded`+`FlutterError.onError`+`PlatformDispatcher.onError` 三处汇一;`ErrorWidget.builder` 可恢复错误屏;启动门控(BackendStatus 未就绪显启动屏)包在现有 home 外。 |
| codegen | **RE-ADD 工具链** | `build.yaml` 限 `generate_for: contract/** + features/**/data/**`(slang 不进 build_runner);`analysis_options` 忽略 `invalid_annotation_target` + 排除 generated。 |

## 3. 重新加的依赖(都是 main 上有、rebuild 删掉的,版本 2026-06 现行、桌面三平台已验)
```
dependencies:
  dio: ^5.9.2                      # REST + 长连 SSE(ResponseType.stream)。SSE 拒 eventflux(停滞)/flutter_client_sse(黑盒,non-200 即死)
  freezed_annotation: ^3.1.0
  json_annotation: ^4.12.0
  go_router: ^17.3.0               # 非严格 4.0 骨干,随最小 AppShell 路由一起(Entities 4.1 要)
  fast_immutable_collections: ^11.2.0   # 推迟到 BlockTreeReducer/GraphModel 落地再用
dev_dependencies:
  build_runner: ^2.15.0
  freezed: ^3.2.5                  # 3.x 需 abstract/sealed 关键字
  json_serializable: ^6.14.0       # unknownEnumValue 真实存在
```

## 4. 后端改动(loopback 加固,同提交守 N/D/E/S/T + make verify + 文档 1:1)
- **A 默认绑 loopback**:`ANSELM_ADDR` 空 fallback 从 `:8080`(全网卡)→ `127.0.0.1:8080`(`build.go:109-115`)。纯默认值改,`ANSELM_ADDR` 仍可覆盖。低风险。
- **B `RequireBearerToken` 中间件**(新 `middleware/bearer.go`):`crypto/subtle.ConstantTimeCompare` 比对 `Authorization: Bearer` 与新 env `ANSELM_AUTH_TOKEN`;**path 前缀跳过表**(webhooks 豁免——它们 HMAC 自鉴权;**health 要 token**)。新 sentinel `ErrUnauthorizedBadToken`。中风险(chain 签名涉及)。
- **C `RequireLoopbackHost` 中间件**(新 `middleware/host.go`):校验 `r.Host` 仅允 `127.0.0.1[:port]`/`localhost[:port]`/`[::1]`,否则**硬编 403 `FORBIDDEN_BAD_HOST`**(不新增 Kind,避免 errmap 涟漪)。**常开**(loopback 下零代价、dev make-server 经 127.0.0.1/localhost 仍通)。防 DNS rebinding。
- 三者经 Chain 包裹整 mux → **SSE GET + health 都被覆盖**(已验证)。
- **不改**:端口发现保持客户端预抢(零后端改);stdin-EOF 孤儿守护**推迟**到 fast-follow(supervisor 已 SIGTERM,孤儿只在硬崩溃)。

## 5. 🔥 流式渲染性能(回应"会不会每帧重渲染整页"——答案:不会,靠 7 层 AND 起来兜住)
> 你的直觉对:**裸做会卡**(页面层 watch 流 → 每个 token 重建整页)。下面 7 层任缺其一都会漏帧进整页;**全做到 = 一帧只重画那个叶子**。

- **L0 网关 demux**(plain Dart,Riverpod 之下):订阅者拿到的流已只含自己的帧(O(帧),非 O(帧×订阅者));**禁** Riverpod/build 里逐帧 `.where`。【4.0】
- **L1 ephemeral/durable 分流**(ingest 边界):seq=0 走瞬时 holder(不写缓存/不进游标/可合并)、seq>0 立即 patch 缓存 + 进游标(不丢不合)。两条码路、两个 state holder。【4.0】
- **L2 每帧合并器**(每 scope 一个,demux 兄弟):同步把每个 delta 累进内存(StringBuffer/Map),`_flushScheduled` 守一帧只 notify 一次(`schedulerPhase==idle ? 立刻 : addPostFrameCallback`)。**几百帧/秒 → ≤1 重建/帧**。复用 `core/` 原语,不每 feature 重抄。【4.0 原语】
- **L3 family provider per blockId/nodeId**:X 的帧只 invalidate `blockProvider(X)`,兄弟不脏。【feature 时用,原语 4.0 备】
- **L4 叶子 `.select((s)=>slice)`**:只在变化字段重建(token 改 text 不动 role badge/缩进/折叠箭头)。【feature 时】
- **L5 叶子 Consumer + ValueListenable + RepaintBoundary**:Consumer 放在流式 `Text`/单节点;页面是 `const` StatelessWidget 建 `ListView.builder`,**绝不** watch 流;RepaintBoundary 隔离热点光栅。【feature 时】
- **L6 列表/树虚拟化**:`ListView.builder`/`SliverList.builder` + 稳定 `ValueKey`(插/改第 N 行不重建 0..N-1)+ cacheExtent;`reverse:true` chat 底部增长。【feature 时】

**禁(review/lint 守,每条都把一帧变整页重绘)**:A1 页面层 watch 整集合/整流 AsyncValue(页面只许 watch 扁平 ID 列表)· A2 叶子 watch 整对象不 select · A3 逐帧 `.where` · A4 一 token 一 notify · A5 ephemeral 灌进 durable 缓存 · A6 helper 函数建行(不能 const 短路)。

**性能门禁(`make fe-verify`,经真网关 + 假传输)**:`BuildSpy` 埋 3 高度(① 流式叶子 ② 行 ③ 页面),`fake.emitDelta` 灌 200 帧 + `tester.pump()`(非 pumpWidget),断言 **叶子≈190、行≤1、页面==0**;durable 帧另断言进缓存。这是"绝不整页重绘"的红绿证明。【harness 4.0 备,真断言 feature 时】

**4.0 vs feature 时**:4.0 建 L0/L1/L2 原语 + 门禁 harness + reducer 的家(BlockTreeReducer/GraphModel 框架无关层);L3–L6 的真叶子写法在 Chat 4.2 / Scheduler 4.3 落地——因 4.0 还没有流式 UI 可优化。

## 6. 有序建造步骤(每步 gate 绿才进下一步;floor=`make fe-verify` / 后端 `make verify` -race + `make testend`)
- **STEP 0 工具链+依赖**:re-add deps + `build.yaml` + analysis 忽略项。Gate:`pub get` 净 + `slang && build_runner build` 成 + analyze 净 + 既有 UI kit 测仍绿。
- **STEP 1 PORT contract + ADD 码/DTO**。Gate(强):每 DTO `fromJson/toJson` 往返 golden(录真后端响应进 `test/fixtures`)+ `Page/PageWithAggregate.fromBody`(空页/末页/聚合变体)+ enum unknown 兜底 + `ApiException.fromEnvelope`。
- **STEP 2 PORT net + bearer 拦截器**。Gate:mock dio 断言 unwrap/分页/错误映射/401·410/token 挂载。
- **STEP 3 PORT sse + jitter**。Gate:`sse_parser` 喂录制 line fixture(含空格冒号/多 data/keep-alive 注释)+ 重连状态机(EOF 重连、410 重取、durable 才进游标)+ jitter 范围。
- **STEP 4 core/process sidecar**。Gate:假二进制测端口预抢/health 门控/SIGTERM→超时→kill/有界重启;跨平台信号注意。
- **STEP 5 后端 loopback 三改**(同提交文档)。Gate:`make verify` -race + `make testend`(bearer 跳过表/host 校验/health 要 token)。
- **STEP 6 Riverpod 装配 + 错误边界 + 启动门控 MERGE 进 AnApp**。Gate:启动门控 widget 测(BackendStatus 各态)+ 错误边界吞异常不白屏 + override 注入。
- **STEP 7 性能原语 L0–L2 + 门禁 harness**。Gate:`BuildSpy` 200 帧 → 叶子≈190/行≤1/页面==0。
- **交付**:空壳 app 能拉起后端、健康门控、三流连上、错误优雅降级、流式更新只动叶子——**Entities 4.1 可直接接真后端开建**。

## 7. 待你拍板(多已带推荐,点头即默认)
1. **后端 loopback 三改进 4.0**(需后端提交)——确认?(推荐:是,它是 loopback P0)
2. **health 要不要 token**?(推荐:**要**——门控的是同进程、它有 token;豁免反而留个未鉴权面)
3. **go_router 现在加还是推迟**?(推荐:随最小 AppShell 现在加,Entities 4.1 要路由)
4. **PORT+MERGE 路线**确认(不重建、不覆盖 main.dart/app.dart)?
5. stdin-EOF 孤儿守护推迟到 fast-follow——可否?

## 8. 数据/复核
- 三轮 digest:`scratchpad/wf4_digest.json`(后端契约)· `wf5b_digest.json`(解决方案)· `wf5perf_digest.json`(性能)。原始全文:`tasks/{w8q011l67,wm42o1h1d,wlup4cbx4}.output`。
- 验证:后端契约对抗验证 verdict=READY-TO-SPEC;解决方案 verify=APPROVE(包全真/桌面/现行,SSE 续传对齐 stream.go 实锤);性能 gate 即其自证(BuildSpy 红绿)。
