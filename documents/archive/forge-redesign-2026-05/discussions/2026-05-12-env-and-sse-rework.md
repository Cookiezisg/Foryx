# 2026-05-12 Discussion — Env model + SSE 统一(Plan 03 重构)

> **Context**:Plan 02 (handler) 收工后准备开 Plan 03 (eventlog + transport)。
> 进 Phase 1 没多久,从"加 Scope struct"这一步岔出去讨论了一大圈,最终
> **大幅修订 Plan 03 方向** + **回过头改 Plan 01/02 的 env 模型**。
> 本文落档所有决策,作为后续 commit 的依据。

---

## A. 起因 — TLS 那一步不值得

原 Plan 03 Phase 5 想加 HTTP/2 + TLS,理由是 HTTP/1.1 浏览器 6-conn/origin 限制 + 多 SSE 流可能撞。

**讨论后发现**:
- 现状真实生产 SSE 只有 2 条(`/api/v1/eventlog?conversationId=X` + `/api/v1/notifications`),远没撞 6
- D19 原计划 per-entity scope 会让条数膨胀,但有更轻的解
- TLS 给 Wails 打包带来 25MB mkcert binary + 首次 sudo 装 CA + 18 月证书轮换 — 负担不值
- 桌面端最终方案是 Wails native events 绕开 HTTP,**TLS 永远不需要**

**决策 D-redo-2026-05-12-1**:Plan 03 Phase 5 (TLS + HTTP/2 + mkcert)**永久搁置**,不进 V1.2。

---

## B. SSE 改成 3 条统一流(按 user_id key)

### 新形态(全部按 user_id 订阅,future-proof 多用户)

| Stream | 用途 | wire 关键字段 |
|---|---|---|
| `GET /api/v1/eventlog` | chat 内容流 — message/block 5 个事件(text/reasoning/tool_call/tool_result/progress/message)| payload 带 `conversationId`,client demux |
| `GET /api/v1/notifications` | entity 状态变更(全局通知)| payload `{type, id, data}`,`type` 开放词表 |
| `GET /api/v1/forge`(新)| trinity 锻造进度流(function / handler / workflow)| payload 带 `kind, entityId` |

**全部按 user_id 订阅**:
- 后台 Bridge 内部 key 从 `conversation_id` / 全局 改为 `user_id`
- 订阅端不传 `?conversationId=` / 不区分 entity — 一律每用户一条
- 每用户全局单调 `seq`,Last-Event-ID 重连仍工作
- 慢订阅者 BLOCK publisher(append-only,delta 不丢)

### 决策清单

- **D-redo-2026-05-12-2**:`/api/v1/eventlog` 去掉 `?conversationId=` 必填,按 user_id 推全部 conv 事件,client 按 `payload.conversationId` demux
- **D-redo-2026-05-12-3**:`/api/v1/notifications` Bridge 加 user_id key(现在是全局)
- **D-redo-2026-05-12-4**:新加 `/api/v1/forge` SSE — trinity 锻造进度流
- **D-redo-2026-05-12-5**:**3 条 SSE 上限**(dev 模式 +1 `/dev/logs` = 4)。所有未来"entity 详情面板独立流"需求一律走 forge 流 + client filter,**不再开新流**

### Forge stream wire 协议

```
event: forge_started
id: 12
data: {"kind":"function","entityId":"fn_x","operation":"create",
       "conversationId":"cv_a","toolCallId":"tc_1"}

event: forge_op_applied
id: 13
data: {"kind":"function","entityId":"fn_x","index":0,"op":"set_code"}

event: forge_env_attempt
id: 14
data: {"kind":"function","entityId":"fn_x","attempt":1,
       "status":"installing","stage":"resolving deps","detail":"..."}

event: forge_env_attempt
id: 15
data: {"kind":"function","entityId":"fn_x","attempt":1,
       "status":"failed","error":"No matching distribution"}

event: forge_env_attempt
id: 16
data: {"kind":"function","entityId":"fn_x","attempt":2,
       "status":"ok"}

event: forge_completed
id: 17
data: {"kind":"function","entityId":"fn_x",
       "status":"ok","versionId":"fnv_y","envStatus":"ready",
       "attemptsUsed":2}
```

**枚举**:
- `kind ∈ {function, handler, workflow}` — 封闭(workflow 现在加上避免 Plan 04 时改协议)
- `operation ∈ {create, edit, revert, delete}` — 封闭
- forge event type 4 个(started / op_applied / env_attempt / completed)— 封闭

---

## C. Notification payload 瘦身

### 原 function 9 个 action + handler 11 个 action 大盘点

每个 action 现在 data 字段塞**完整 entity 实体**(几 KB - 几十 KB),严重违背"通知 = 轻量状态变更"。

### 新规则:**所有 notification data 只送指针 + 必要小字段**

UI 拿到通知 → 主动 GET 详情。带宽 + 一致性 + 心智都更好。

### 决策清单

- **D-redo-2026-05-12-6**:function/handler 所有 action 的 `data` 字段精简为
  ```
  { action, versionId?, versionNumber?, envStatus?, envError? }
  ```
  完整 entity 不进 envelope
- **D-redo-2026-05-12-7**:**删除** `env_synced` / `env_failed` 两个 action(function + handler 各一对)
  理由:env 状态变更现在同步发生在 LLM tool 内部(见 D 节),tool result 携带 envStatus,**LLM 和 UI 都不需要异步推**

### 改完后的 function 通知

| action | data |
|---|---|
| `created` | `{action, versionId, versionNumber:1}` |
| `updated` | `{action}` |
| `pending_created` | `{action, versionId}` |
| `version_accepted` | `{action, versionId, versionNumber}` |
| `pending_rejected` | `{action, versionId}` |
| `reverted` | `{action, versionId, versionNumber}` |
| `deleted` | `{action}` |

7 个(从 9 砍到 7,删了 env_synced + env_failed)。

### 改完后的 handler 通知

| action | data |
|---|---|
| 同 function 7 个 | 同 |
| `config_updated` | `{action}` |
| `config_cleared` | `{action}` |

9 个(从 11 砍到 9,删了 env_synced + env_failed)。

---

## D. Env 模型彻底重整 — sync in tool + 每版本独立 env

### 现状的问题(多个)

1. **EnvID = sha256(deps, python_version)** — 多个 version 共享同 EnvID 时,一个 version 装包成功 ≠ 另一个 version 行的 envStatus 翻 ready(每行独立 envStatus 字段,共享时不同步)→ UI 撒谎
2. **Function 后台 fire-and-forget sync** — Accept 后不起 sync(注释说要做,代码漏了)
3. **Handler 完全没有后台 sync 入口** — 只能在 Call 时同步装(用户卡 30-60s)
4. **edit 写 pending 不装 env** — 用户 review pending 时不知道环境能不能用,accept 后才装,装失败白审

### 决策清单

- **D-redo-2026-05-12-8**:**每版本独立 venv,EnvID 在 Version 行内独立生成**(`fnenv_<16hex>` / `hdenv_<16hex>`),跟 versionID 1:1 但**不等同**。删 `ComputeEnvID(deps, python)` 哈希共享逻辑。代价:多几个 venv 目录(uv global cache 让实际磁盘开销小);收益:envStatus 字段对当前 version 状态零歧义 + EnvID 与 versionID 解耦(sandbox 是共享基础设施,handler/chat tool calls/mcp 等其他消费者各自有自己的 EnvID 命名空间,trinity 不应强迫"EnvID == 我的 entity ID"语义)
- **D-redo-2026-05-12-9**:**env sync 同步发生在 LLM tool 内部**,不再有后台 goroutine。`Service.Create` / `Service.Edit` 调用同步 `syncEnvSync`(blocking),失败时返 envStatus=failed + envError 给调用方
- **D-redo-2026-05-12-10**:**`Service.AcceptPending` 仅翻 active 指针**(env 已在 edit 阶段装好),瞬时返。修旧 bug
- **D-redo-2026-05-12-11**:**`Service.Edit` 改 "iterate same pending"**:
  - 无 pending → 在 active 之上 ApplyOps → 创建新 pending Version 行 → 装 env
  - 有 pending → 在 pending 之上 ApplyOps → **重写同 ID pending 行**(不创建新行)→ 销旧 env → 装新 env
  - 这让 LLM 在 tool 内部反复 patch 同一份 draft 直到 env ready,而不是"每次 edit 创建新行"
- **D-redo-2026-05-12-12**:**`Service.RejectPending`** 主动调 sandbox.DestroyEnv + 删 pending Version 行
- **D-redo-2026-05-12-13**:**`Service.Revert`** 翻指针;目标 version 的 env 已被 evict 时同步重建
- **D-redo-2026-05-12-14**:**删** `SyncEnvForVersion` (fire-and-forget) + `Resync` 后台路径 + handler 那边对应的入口

---

## E. Tool 内部 env-fix 自闭环 LLM loop

### 模型

`create_function` / `edit_function` / `create_handler` / `edit_handler` 4 个 LLM tool **内部跑一个小循环**:env 装失败时调主 model 让它"看 stderr 建议新 deps",retry 最多 N 次。

### 决策清单

- **D-redo-2026-05-12-15**:4 个工具加内部 env-fix loop
  ```
  for attempt := 1..maxAttempts {
      装 env
      success → break,返成功 + attemptsUsed
      fail → 内部 LLM call (用主 model, scenario="chat",DeepSeek)
            prompt:"deps=X,装失败 stderr=Y,attempts 历史=Z,建议新 deps"
            返新 deps → 覆盖 Version.dependencies → 销旧 env → loop
  }
  maxAttempts 后仍失败 → 返 envStatus=failed + envError + attemptsUsed + attemptHistory
  ```
- **D-redo-2026-05-12-16**:**maxAttempts = 3**(hardcode,V1.5 可配置)
- **D-redo-2026-05-12-17**:**内部 LLM 用主 chat scenario 的 model**(DeepSeek 反正便宜)。**不引入** `env_fix` 单独 scenario
- **D-redo-2026-05-12-18**:LLM 在内部 loop 中**只改 deps**,不改代码 / parameters / 其他 ops(per "代码一次搞好"原则)
- **D-redo-2026-05-12-19**:每次 attempt 推一个 `forge_env_attempt` 事件 + 在 chat tool_call block 下 emit progress block delta;成功/失败终态推 `forge_completed`

### 主 chat 体验

```
[user]: "建个 CSV 清洗工具用 pandas"

[assistant 一次 chat 回合,主 LLM 一次调用]:
  text:        "好,我建一个..."
  tool_call:   create_function {ops:[set_code, set_dependencies:["pandass"]]}
    progress block (挂 tool_call):
      "[Attempt 1] Resolving deps..."
      "[Attempt 1] Downloading pandass..."
      "[Attempt 1] ERROR: No matching distribution"
      "[Attempt 2] AI suggested: pandas (typo corrected)"
      "[Attempt 2] Installing pandas-2.1..."
      "[Attempt 2] Success!"
  tool_result: { functionId: fn_x, envStatus: "ready", attemptsUsed: 2 }
  text:        "建好啦 fn_x,我中途修了个 typo。"
```

主 LLM context 只看到最终 tool_result,中间 retry 是 tool 内部 self-heal,token 省。

### 修环境后期场景(env 突然坏了)

无专用 resync 工具(per 决策 D-redo-2026-05-12-14)。LLM 路径:
```
LLM → get_function(fn_x) → 看到 envStatus=failed + envError
LLM → edit_function({id, ops:[set_meta:{description:"trigger rebuild"}]})
       (无 deps 改动,但因 EnvID=versionID,新 pending 有新 envID → 触发重装)
LLM → user:"我让它重装环境就好了,review 一下点接受。"
```

略 hacky 但能跑。**V1.5 真嫌烦再加 `rebuild_function_env` 工具**,V1.2 不做。

---

## F. 改动汇总清单

```
✅ 已 commit(scope.go)— 保留,forge event payload 用其 kind/id 形态

待 commit:

1.  EnvID 每版本独立生成(`fnenv_`/`hdenv_`,与 versionID 解耦);删 ComputeEnvID hash 逻辑(C1.1 修订原 D8 措辞)  [domain + sandbox_types]
2.  Service.Create 内置同步 syncEnvSync(替 fire-and-forget)             [function + handler]
3.  Service.Edit 改 "iterate same pending":
    - 无 pending → 创建新 pending,装 env
    - 有 pending → 重写同 ID pending,销旧 env,装新 env             [function + handler]
4.  Service.AcceptPending 仅翻 active 指针,不再 sync                    [function + handler]
5.  Service.RejectPending 调 sandbox.DestroyEnv + 删 Version 行          [function + handler]
6.  Service.Revert 翻指针;evicted 时同步重建                            [function + handler]
7.  Service.Delete 已删全部 env(已在做)                                 [验证]
8.  删 SyncEnvForVersion / Resync async 路径                            [function]
9.  Handler 不再需要 SyncEnvForVersion 异步入口(同 8)                    [handler]
10. 4 个 LLM 工具加内部 env-fix loop(maxAttempts=3,主 model fix deps)  [tool/function + tool/handler]
11. eventlog Bridge 改 user_id key                                       [domain + infra/eventlog]
12. eventlog HTTP /api/v1/eventlog 去 ?conversationId= 必填                [transport/handler]
13. notifications Bridge 加 user_id key                                  [domain + infra/notifications]
14. notifications wrapper 自动从 ctx 抽 user_id                          [pkg/notifications]
15. 新 domain/forge/(Bridge interface + 4 event types + kind/operation enum)  [新建]
16. 新 infra/forge/bridge.go(user-keyed,同 eventlog pattern)            [新建]
17. 新 transport/httpapi/handlers/forge.go(GET /api/v1/forge SSE)         [新建]
18. 新 pkg/forge/Publisher wrapper 给 LLM tools 用                        [新建]
19. 4 个 LLM tool 双写 — call forge.Publish + 保留 chat eventlog progress  [tool/function + tool/handler]
20. function/handler crud 删 env_synced + env_failed publish calls       [function + handler]
21. notification payload 瘦身(去掉完整 entity inline)                    [function + handler crud]
22. HTTP /pending:accept 改瞬时返(env 不在此装)                          [transport/handler]
23. HTTP /functions/{id}:resync 端点删除                                  [transport/handler]
24. HTTP /handlers/{id}:resync 端点(若有)删除                            [transport/handler]
25. testend chatBus 新建(单 SSE,client 按 conversationId demux)         [testend]
26. testend forgeBus 新建(同 pattern)                                    [testend]
27. testend chat panel / function detail / handler detail listener 调整   [testend]
28. 文档同步:
    - service-design-documents/function.md(env 模型 + tool 行为)
    - service-design-documents/handler.md(同上)
    - service-contract-documents/api-design.md(端点 + SSE 协议)
    - service-contract-documents/events-design.md(notifications 瘦身 + forge 流)
    - service-contract-documents/error-codes.md(无变化,但 confirm)
    - event-log-protocol.md(去 conversationId 必填,加 forge 节)
    - progress-record.md dev log
    - 本 discussion 文件链接进 plans/03-eventlog-and-transport.md 头部           [docs]
29. 测试:eventlog/notifications/forge 三 bridge 单测;HTTP handler test;
    LLM tool 内部 env-fix loop 单测(fake LLM + fake sandbox);
    pipeline test 跑 create_function 端到端验证 forge 流推送               [tests]
```

---

## G. 执行顺序(commit 切分)

```
Commit 1: env 模型重整(后端)
   - 8 → 1, 2, 3, 4, 5, 6, 7, 9
   - 单测 + handler/function 现有测试调整

Commit 2: LLM tool 内部 env-fix loop
   - 10
   - tool 单测 + pipeline test 用 fake LLM 演示 retry

Commit 3: SSE 三流统一 — eventlog + notifications user_id 化
   - 11, 12, 13, 14, 20, 21, 22, 23, 24
   - bridge 单测改 + httptest 改

Commit 4: forge stream 新建
   - 15, 16, 17, 18, 19
   - forge bridge 单测 + tool 双写 pipeline test

Commit 5: testend 三 bus 改造
   - 25, 26, 27

Commit 6: 文档全套
   - 28
```

总估 6 个 commit,~1500 LOC + 测试 + 文档。

> **执行前** Plan 03 原 plan 文件(`plans/03-eventlog-and-transport.md`)需要更新头部
> 加链接到本 discussion + 标记 Phase 5 (TLS) 永久搁置 + Phase 1-4 内容大改。
> 让未来翻 Plan 03 的人能跟上当前真实方向。

---

## H. 决策一览表

| ID | 主题 | 决策 |
|---|---|---|
| D-redo-2026-05-12-1 | TLS | 永久搁置,Wails native event 解决 |
| D-redo-2026-05-12-2 | eventlog | 去 conversationId 必填,user_id 订 |
| D-redo-2026-05-12-3 | notifications | bridge 加 user_id key |
| D-redo-2026-05-12-4 | forge | 新加 /api/v1/forge 流 |
| D-redo-2026-05-12-5 | SSE 上限 | 3 + 1 dev,永远不再加 |
| D-redo-2026-05-12-6 | notif payload | 瘦身 — 只送 ID + 小字段 |
| D-redo-2026-05-12-7 | notif | 删 env_synced + env_failed action |
| D-redo-2026-05-12-8 | EnvID | 每版本独立生成(`fnenv_`/`hdenv_`),与 versionID 解耦 |
| D-redo-2026-05-12-9 | env sync | 同步在 Create/Edit tool 内 |
| D-redo-2026-05-12-10 | Accept | 仅翻指针,不 sync |
| D-redo-2026-05-12-11 | Edit | iterate same pending,不创建新行 |
| D-redo-2026-05-12-12 | Reject | 销 env + 删行 |
| D-redo-2026-05-12-13 | Revert | 翻指针 + evicted 重建 |
| D-redo-2026-05-12-14 | async path | 删 SyncEnvForVersion / Resync |
| D-redo-2026-05-12-15 | env-fix loop | 4 个 LLM tool 内部 retry |
| D-redo-2026-05-12-16 | maxAttempts | 3 |
| D-redo-2026-05-12-17 | fix-LLM model | 主 chat scenario,无 env_fix scenario |
| D-redo-2026-05-12-18 | fix 范围 | 只改 deps |
| D-redo-2026-05-12-19 | progress | 推 forge_env_attempt + chat progress block |
| D-redo-2026-05-12-20 | sandbox 不可用 | 硬拒(503 ErrSandboxUnavailable),不建 entity |
| D-redo-2026-05-12-21 | 内部 LLM 失败 | 与"装失败"同路径,记 1 attempt 后返 envError |
| D-redo-2026-05-12-22 | edit empty ops | 官方"强制重建 env"语义 |
| D-redo-2026-05-12-23 | forge payload | 用 Scope struct 嵌套,不平铺 |
| D-redo-2026-05-12-24 | CLAUDE.md §E1 | "双协议"→"三协议",必须同步更新 |

---

## I. 补漏 — 5 个边角决策(2026-05-12 续)

H 表已覆盖主干,但前面对话里有 5 个边角问题需要正式落档,避免实现期再翻。

### I.1 Sandbox 不可用时的处理路径(D-redo-2026-05-12-20)

**前提**:sandbox 是项目基石,bootstrap 在 `cmd/server/main.go` 启动期就跑,正常情况必成功(mise binary 已 `go:embed`)。**bootstrap 失败 = 项目本身瘫痪**(连 chat 也用不了),不是"个别功能 degrade"。

**决策**:
- `Service.Create` / `Service.Edit` 调 sandbox 前**先 ping**(轻量 health check)
- ping 失败 → 直接返 `domain.ErrSandboxUnavailable` sentinel
- HTTP transport 把该 sentinel 映到 **503 Service Unavailable + code=SANDBOX_UNAVAILABLE**
- LLM tool 看到 503 → 失败 tool_result,**不创建 entity 行**
- 不做"先建 Function 行,envStatus=failed 占位"这种"看起来恢复友好"的 graceful degradation —— 这种 path 会污染 DB,后续 list 显示一堆死 entity,UI 还得加"重试装环境"按钮,徒增复杂度

**触发场景**:`backend/internal/infra/sandbox/mise/<goos>-<goarch>/mise` 没拉(用户跑了 `make build` 但忘了 `make resources`)/ mise binary 损坏 / mise data dir 权限错 / 磁盘满到连 venv 都建不了。这些都是**环境异常**,不是业务异常,503 表达更准。

**errmap 影响**:`error-codes.md` 加一行 `ErrSandboxUnavailable | 503 | SANDBOX_UNAVAILABLE`;`transport/httpapi/response/errmap.go::errTable` 加对应项(per §S17)。

### I.2 内部 LLM 调用失败的处理(D-redo-2026-05-12-21)

**场景**:tool 内部 env-fix loop 第 N 轮装失败,准备调主 model 让它 suggest 新 deps,这次 LLM call 本身炸了(网络抖 / DeepSeek 5xx / API key 被吊销)。

**决策**:与"装 env 失败"完全同路径处理。
- attemptsUsed 记为**本轮失败前的次数**(若 attempt 1 装失败后调 LLM 炸 → attemptsUsed=1)
- 不对 LLM call 本身做 retry / fallback model
- 返 `envStatus=failed` + `envError="env-fix LLM call failed: <upstream err>"` + `attemptHistory`
- 主 chat 看到 tool_result 后自行决定怎么继续(通常会跟用户解释 + 让用户决定重试 / 改 deps / 放弃)

**理由**:维持 tool 内部 loop 的状态机简单 — 一个 attempt 要么完整(装失败 + LLM 改 deps 成功 = 进 attempt+1),要么半途中断(装失败 + LLM 调用失败 = 立刻退出 loop 返失败)。**不引入 LLM call 自身的 retry 机制**(那是 chat 域的事,不是 env-fix loop 的事)。

### I.3 edit 空 ops = 强制重建 env(D-redo-2026-05-12-22)

**前情**:讨论"env 后期突然坏了怎么修"时,我提了"假装编辑 set_meta:{description}"的 hack。用户场景虽然成立,但 hack 味太重 — 让 LLM 必须想"我得改个无关字段触发重装"。

**决策**:`edit_function` / `edit_handler` 显式接受 `ops=[]` 空数组,语义为**强制重建当前 active version 的 env**(不创建 pending,不改任何字段,只销旧 env + 重装)。
- ApplyOps(empty) → 不写任何字段,**直接进 env sync 流程**
- 走完整 env-fix loop(maxAttempts=3,LLM 可建议改 deps)
- 成功 → active version 的 envStatus 回 `ready`;失败 → 同 I.2 处理
- 不创建新 Version 行,不影响 pending(若有 pending 则同样路径处理 pending 的 env)

**LLM 看到的路径变简单**:
```
LLM → get_function(fn_x) → envStatus=failed
LLM → edit_function({id:"fn_x", ops:[]})  ← 明确语义,不再骗人
LLM → "我让它重装环境就好了"
```

**文档落点**:`02-function.md` / `03-handler.md` 工具规约里加一行"`ops=[]` 视为强制重建 env"。

### I.4 forge stream payload 用 Scope struct(D-redo-2026-05-12-23)

**前提**:`domain/eventlog/scope.go` 已经定义了 `Scope{Kind,ID}`,a3b7c59 commit。它本来给 eventlog 用,但 eventlog 后来改成按 user_id 订(D-redo-2)就用不上了。

**决策**:forge stream 的 payload **复用** Scope struct,嵌套形式:

```json
{
  "scope": {"kind":"function","id":"fn_x"},
  "operation": "create",
  "conversationId": "cv_a",
  "toolCallId": "tc_1"
}
```

而**不是**平铺:

```json
{
  "kind":"function",
  "entityId":"fn_x",
  "operation":"create",
  ...
}
```

**理由**:
1. Scope 已在 eventlog 域定义,跨域复用比 forge 域自己再起一套 `{kind,entityId}` 更省心智
2. JSON 嵌套 + Go struct 嵌套对应直观,前端 TS 类型 `{scope:{kind,id}, operation, ...}` 也清晰
3. 未来如果 entity 需要 path-like 复合 id(workflow 里 nested step 等),scope.id 是 string,天然可塞 `wf_x/step_3` 这种 — 平铺时只能再加字段

**枚举 kind 限定**:forge stream 中 `scope.kind ∈ {function, handler, workflow}`,**不含** `conversation` / `flowrun`(那俩不锻造)。bridge publish 时校验,非法 kind panic。

### I.5 CLAUDE.md §E1 更新(D-redo-2026-05-12-24)

**当前 §E1**(`/Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/CLAUDE.md`)写的是**双协议**:
- 事件日志(per conversation)
- 通知(global broadcast)

**新现实是三协议**:
- 事件日志(per user,demux 按 payload.conversationId)
- 通知(per user)
- forge 流(per user)

**决策**:Doc commit B 必须同步改 CLAUDE.md §E1,从"双协议"改"三协议",并把"事件日志按 conversationId 订阅"改为"按 user_id 订阅,payload 带 conversationId 客户端 demux"。

这是 CLAUDE.md 唯一一处需要在本轮改的章节(其他 S/D/N/T 规范不动)。

---

## J. 待办执行(从 H/I 同步)

I.1-I.5 5 个边角决策**已纳入 Commit 切分**:
- Commit 1 含 I.1(env model + sandbox ping 失败硬拒)
- Commit 2 含 I.2(LLM 调用失败处理)+ I.3(edit empty ops 重建)
- Commit 4 含 I.4(forge payload 用 Scope)
- Commit 6 含 I.5(CLAUDE.md §E1) + 其他文档同步

§F 的 29 项 + §G 的 6-commit 切分不需要重排 — I 节是细化,不是新增工作。
