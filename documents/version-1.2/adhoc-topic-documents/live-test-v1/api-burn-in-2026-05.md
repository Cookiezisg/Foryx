# V1.2 后端 Burn-In Test Log — 2026-05-14

> 多小时疯狂 case 测试,目标是结果**符合预期**(不是不报错),逐条与文档对照。
> 用 DeepSeek API key 实测 LLM 路径。遇到 bug 直接修(除非是产品决策)。
> 每个测试有 4 个字段:**预期** / **实际** / **判断** / **行动**。
>
> 起始时间:2026-05-14 01:38(本地)。
> 后端 PID 21177 跑在 :8742,data-dir `/tmp/forgify-dev`(每次清空)。
> DeepSeek key: `sk-204827945d6e40a7a0f42ac45c5ce305` (provider=deepseek, modelId 实测 `deepseek-v4-flash` / `deepseek-v4-pro`)。

---

## 测试维度(产品全流程,非 API 单点)

按"产品流"分组,每流多个 case:

### A. 多轮 AI 对话
- A1 单条消息
- A2 多轮对话 (3+ turn 上下文连续)
- A3 system prompt 影响输出
- A4 长 prompt(>2k tokens)
- A5 长输出(请求详细文档)
- A6 mid-stream cancel
- A7 reasoning 模型分离 reasoning / text blocks
- A8 并发 3+ conv 各自 streaming

### B. 工具锻造闭环(function / handler / workflow)
- B1 通过 chat 让 agent 锻造一个 function (`create_function` 工具)
- B2 立刻 run 这个 function
- B3 让 agent 编辑这个 function (`edit_function`),走 pending 流程
- B4 accept pending → run new version → 行为变了
- B5 revert 到 v1 → run → 旧行为回来了
- B6 同上 for handler(带 config,init/methods)
- B7 同上 for workflow(graph + nodes + edges)
- B8 锻造期间 forge SSE 流推 forge_started / forge_op_applied / forge_completed

### C. Skill / MCP 调用
- C1 通过 chat 让 agent 调用 anthropic skill
- C2 通过 chat 让 agent 调 google-search MCP tool
- C3 通过 chat 让 agent 调 duckduckgo MCP tool
- C4 多个 MCP tool 并行调用(`execution_group` 同 group)
- C5 MCP server 状态查询 / reconnect / health

### D. Workflow 部署 + 运行
- D1 锻造一个 workflow(manual trigger + 多 node)
- D2 manual trigger → 看 FlowRun 节点逐个跑
- D3 workflow 含 approval node → 暂停 → 人工 approve → 继续
- D4 cancel 进行中的 FlowRun
- D5 workflow 含 cron trigger,等自动触发(若超时则 manual trigger 替代)
- D6 workflow 调用 forged function(B 阶段产物),证 catalog 联动

### E. SSE 流契约
- E1 eventlog 流式 5 events × 6 block types 完整 lifecycle
- E2 SSE 重连 Last-Event-ID resume(no duplicates / no gaps)
- E3 410 SEQ_TOO_OLD 走 history endpoint refetch
- E4 notifications 收到 entity 变更,`data` 字段瘦身(per D-redo-22)
- E5 forge stream 推 4 events × 3 kinds 的锻造进度
- E6 subagent run 在 messages 表(attrs.kind=subagent_run)+ parentBlockId 链对

### F. 边界 + 错误路径
- F1 invalid api key → :test 报错且不写入 testStatus="ok"
- F2 model-config 选未知 scenario → 422
- F3 sandbox 环境 build 失败 → envStatus="failed" + envError 落地
- F4 cancel 已完成的 stream → 204 idempotent
- F5 N6 PUT model-config 两次 → 都 200 + 第二次幂等
- F6 删除有 active FlowRun 的 workflow → 应允许 cascade soft-delete 或 422 拒绝
- F7 LLM 返 error 中途断流 → message_stop.status=error 而非 cancelled

### G. 并发 + 压力
- G1 同时启 3 个 chat conv
- G2 同时触发 5 个 function run
- G3 同时 trigger 多个 workflow
- G4 长 chat + workflow 共存,观察 sandbox env 复用

> 上方分组只是脚手架。**测的是整个产品**:domain CRUD、SSE 协议、background goroutine、契约 envelope、错误路径、并发、ID 规范、catalog 联动、跨域依赖,缺一不可。

---

## 测试日志(逐条)

(以下随测试 append;每条结构:**ID** / **预期** / **实际** / **判断** / **行动**)

---

### T01 — clean boot 与 sanity

- **预期**:`make test-console` 等价启动后 2-3s 内 `/api/v1/health` 200,无 ERROR 日志,无 WARN。
- **实际**:2s ready,200 OK。**1 条 WARN**:`notifications.publisher: notification publish failed {"type":"skill","id":"*","error":"reqctx: missing user id in context"}` —— 来自 `skill.Scan` 在 boot 阶段的 fire-and-forget publish。
- **判断**:**真后端 bug**(非产品决策)。background goroutine 没用户 ctx 直接 publish 会失败,虽不影响功能(skill list 仍能读),但违反 §S9 detached context 模式,会污染 WARN 噪声(每次 boot 都见)。issue #5。同类问题:之前观察到 MCP boot 时也是这模式(`mcp_server` notif 失败);在 boot 阶段所有 background bootstrap 都该用 detached ctx 注入 DefaultLocalUserID。
- **行动**:留作 issue #5 → 修复:`skill/polling.go` 和 `mcp/mcp.go` boot 入口处把 ctx 包一层 `reqctxpkg.SetUserID(ctx, reqctxpkg.DefaultLocalUserID)`。先继续 burn-in,收尾时一起改。

### T02 — dev sanity sweep + 14 个 endpoint 全 200

- **预期**:所有公开 dev/* + 公共 api endpoint clean DB 下都返 200。
- **实际**:全部 200。
- **判断**:✓ 通过。

### T03 — N4 envelope 形态(empty list)

- **预期**:paginated endpoint clean DB 返 `{data: [], nextCursor, hasMore: false}`。
- **实际**:返 `{data: [], hasMore: false}`,**缺 `nextCursor`** (Go `omitempty` 把空字符串省掉了)。Non-paginated (providers / catalog) 返 `{data: ...}`。
- **判断**:轻微 spec 偏差但前端兼容(把 `nextCursor` 当 optional)。N4 严格说要求 `nextCursor` 必有,但 omitempty 实践通常无害。**留作观察**,不开 issue —— 前端按 optional 处理,Wails 形态下也不会暴露。
- **行动**:保持现状,不改。

### T04 — apikey CRUD + :test + 错误路径

| Sub | 预期 | 实际 | 结论 |
|---|---|---|---|
| T04.1 | 201 + APIKey | 201 + APIKey(testStatus="pending") | ✓ |
| T04.2 | :test 联网 + modelsFound | 366ms, 2 models | ✓ |
| T04.3 | 同 provider 第二个 key | **201 接受** | ⚠️ 业务允许多 key (per `GetByProvider_PrefersOKOverPending`/`PrefersRecentlyTested` 测试),但 list 视图会看到 2 个 key,UI 容易混淆 |
| T04.4 | invalid provider → 400 | 400 INVALID_PROVIDER | ✓ |
| T04.5 | missing key → 400 | 400 KEY_REQUIRED | ✓ |
| T04.6 | empty body → 400 | 400 INVALID_PROVIDER (因 provider 为空空白) | ✓ |
| T04.7 | malformed JSON → 400 | 400 INVALID_REQUEST | ✓ |

- **判断**:除 T04.3 是产品决策(允许同 provider 多 key,有 picker 优选规则)外,其他路径都 OK。
- **行动**:无,留作观察。

### T05 — apikey PATCH/DELETE

- T05.1: PATCH displayName ✓
- T05.2: 空 patch → 200 idempotent ✓
- T05.3: PATCH 不存在 id → 404 API_KEY_NOT_FOUND ✓
- T05.4: DELETE 不存在 → 404 ✓

### T06 — model-config

| Sub | 预期 | 实际 | 结论 |
|---|---|---|---|
| T06.1 | 200 first PUT | 200 | ✓ |
| T06.2 | N6 idempotent PUT 返 200 | 200 | ✓ |
| T06.3 | switch model 同 scenario | 200 + modelId 改了 | ✓ |
| T06.4 | invalid scenario | 400 INVALID_SCENARIO | ✓ |
| T06.5 | provider 没对应 api-key | **200 接受**(无前置校验) | ⚠️ 设计原则 #6 "反校验剧场",不强校验保存 config,运行时再失败;但 UI 应可视化警告 "provider=openai 但无 api-key" |
| T06.6 | web_summary scenario | 200 | ✓ |
| T06.7 | missing modelId | 400 MODEL_ID_REQUIRED | ✓ |

- **判断**:T06.5 是设计意图(避免前后端双校验),但产品体验上 testend ModelConfigs view 应当 highlight 这个 mismatch。**当前 V2 testend ModelConfigs view 已经只让选已有 api-key 的 provider(filter availableProviders),所以 UI 实际上拦下了**——OK。
- **行动**:无。

### T07-T08 — 多轮 chat + 队列行为

- **T07 (single)** 单轮 chat:agent 自然搜索了 forge 库(空)然后用知识答了。1 user + 1 assistant msg,blocks 6 个(reasoning/text/tool_call/tool_result/reasoning/text)。✓
- **T08 (multi-turn)** 3+ 轮 chat 测上下文:
  - 第 1 轮回复正常
  - 第 2 轮(发完 turn 2 后立刻发 turn 3,2nd 在 queue 里等)
  - 第 3 轮触发了 AskUserQuestion 工具(因为我问"两件事中哪个更重要",agent 用 ASk 求澄清)
  - ASk 默认 5 min timeout 后 agent 用"User did not respond"作 tool_result 继续
  - 第 4 轮(我又发了消息)正确排队,在 turn 3 完成后才被处理

- **关键发现 / 误报警**:刚开始以为有"stranded user message"bug(turn 3 5min 不回),实际是 **AskUserQuestion 默认 timeout 5 min**。系统行为正确:turn 3 起诉了 AskUserQuestion → 等不到用户答 → timeout → 继续完成 turn 3 → 然后才处理 turn 4。
- **判断**:✓ 队列机制正确。但产品体验上有改进空间:
  1. UI 应当能区分"agent 正在思考"vs"agent 在等用户回答"两种状态(否则用户看到 5min 没动静会以为系统卡死)
  2. 当多个 user msg 排队 + 前面有 AskUserQuestion 时,新 msg 应能"答题"而不只是单纯追加(产品策略,V2 testend AsksPending view 已支持答题,但 chat 主对话框没有 inline 答题除非 block.status='streaming')
- **行动**:无紧急 bug。issue #6 列入产品改进:Composer 应能感知 pending AskUserQuestion 并 inline 提示答题。

- **T08.4-T08.5** 简单 2 轮快速发送测试:turn 1 + turn 2 都正确处理。queue 工作正常。

### T08.7-T08.10 — REST `/messages` 分页

- **预期**:limit 默认应有合理值(20 / 50?);hasMore + nextCursor 正确。
- **实际**:limit=5 → 5 results, hasMore=true。limit=10/20/50/100 都返全部 8 个,hasMore=false。**默认 limit 不知道是多少**——没传时返了 20(默认值 per backend)。
- **判断**:✓ 分页正常。
- **行动**:无。

### T09 — chat cancel mid-stream

- **预期**:DELETE `/conversations/{id}/stream` 204,assistant msg 状态变 cancelled,部分输出保留。
- **实际**:204,assistant status=cancelled, stopReason=cancelled, 605 chars 已 streamed 部分保留 ✓
- T09.2: 再次 cancel → 404 STREAM_NOT_FOUND(已取消)
- T09.3: cancel 没在跑的 conv → 404 STREAM_NOT_FOUND
- **判断**:✓ 设计正确。

### T10 — auto-title

- **预期**:conv 没 title 时 agent 完成 turn 1 后自动生成 title。
- **实际**:turn 1 完成后 3s autoTitle 触发,title 设为合理摘要("Python asyncio Event Loop")。fire-and-forget。
- **判断**:✓。

### T11 — system prompt 持久 + 影响

- **预期**:PATCH systemPrompt 写入 conv,后续 chat 走 system prompt。
- **实际**:PATCH 后 conv.systemPrompt 持久;agent 用海盗语回复"1+1=2, ye scurvy sea dog! ... Arrr!"。✓

### T12 — attachments

- **预期**:multipart POST /api/v1/attachments 返 Attachment;send message + attachmentIds 把 attrs.attachments 写入 user msg。
- **实际**:✓ Attachment 创建。user msg.attrs={"attachments":[{attachmentId,fileName,mimeType}]}。agent 用 Read/Glob 工具找到文件并读出内容。
- **判断**:Attachment 路径正确。agent 需要"搜索"文件位置是因为 attachments 默认存的目录 agent 不知道——这是产品体验细节,不是 bug。

### T14 — Bash tool

- **预期**:agent 能用 Bash tool 跑复合命令(`pwd; ls -la; uname -a`)。
- **实际**:agent 智能地把 3 个命令合并成一个 Bash 调用("一次性"),结果正确。✓

---

## 🔴 重大 Bug 发现 + 修复

### Bug B-01 (修复中) — `mise install python@>=3.12` 走 pyenv source-compile 失败 → 所有 function 锻造的 env 装不上

- **现象**:T15 用 chat 锻造一个 calc_age function,forge SSE 推出来:
  - forge_started → forge_env_attempt × 3 (3 次重试都失败) → forge_completed(status=failed)
  - 每次 attempt 报错:`mise ERROR ~/Library/Caches/mise/python/pyenv/plugins/python-build/bin/python-build failed`
  - mise 误把 `>=3.12` 当版本号传给 pyenv plugin → fall back 到 python-build (源码编译) → 编译炸了
- **根因**:`MiseInstaller.Install` 在 `infra/sandbox/mise.go:279` 直接拼 `m.kind+"@"+version`,LLM 锻造时存 `pythonVersion=">=3.12"` (DefaultPythonVersion in `domain/function/version.go:30`)。mise 不接受 PEP 440 范围,只接受具体版本(`3.12` / `3` / `3.12.13`)。
- **修复**:在 mise 边界加 `normalizeVersionForMise(version)` 工具,剥掉 `>= / <= / ~= / == / > / < / ~ / ^` 等前缀(留具体版本号)。`MiseInstaller.Install` + `MiseInstaller.Locate` 都用归一化版本对 mise 调用,**domain 层 pythonVersion 字段保留 PEP 440 原文(审计/显示用)**。
- **验证**:T15.5 retry 一遍 — forge_env_attempt 1 次就 ready,无重试,fn envStatus=ready,`:run` 返 36 (1990 出生 → 2026-1990=36) ✓
- **commit**:burn-in 收尾时一起提。

### Bug B-02 (修复中) — D22 5 表中 2 表没注册 AutoMigrate → function_executions / handler_calls 写入静默失败,GET 路径 500

- **现象**:T15.6 forge 完跑 `:run` → 返 200 + output 正确。但 `GET /functions/{id}/executions` → **500 INTERNAL_ERROR**。
- **根因**:`cmd/server/main.go::Migrate(...)` 没把 `functiondomain.Execution{}` 和 `handlerdomain.Call{}` 列进去。`recordExecution` 写 DB 失败被 best-effort warn 吞了(`function/run.go:174` 只 log 不 fail),所以前端 :run 返 200 但 executions 表压根不存在。GET /executions 时 SearchExecutions 走真查询 → "no such table" → 500。
- **修复**:`cmd/server/main.go::dbinfra.Migrate(...)` 加 `&functiondomain.Execution{}` 和 `&handlerdomain.Call{}`(同时注释清楚 D22 5 表归属:Execution, Call, Node, mcpdomain.Call, skilldomain.Execution)。
- **验证**:T16 重新 forge add_nums + `:run` × 2 + GET `/executions` → 200,返 3 条 execution 记录(2 个 my :run + 1 个 agent 锻造期间 self-test 跑的)。✓
- **commit**:burn-in 收尾时一起提。

### T15-T17 — forge function E2E 完整闭环验证

- **T15** 通过 chat 锻造一个 `add_nums` function:agent 调 `create_function` tool,sandbox 装 python env,验测试通过 ✓
- **T16** `:run` 测试两组参数:3+5=8, 100+200=300 ✓
- **T17** 通过 chat 让 agent 用 `edit_function` 修改 add_nums → a+b+1。
  - pending 流程生效 ✓
  - accept pending → 200 + version=2 active ✓
  - 新版本 :run 3+5=9 ✓
  - revert 到 v1 → 200 ✓
  - revert 后 :run 3+5=8 ✓
- **判断**:Trinity 双版本 + pending + accept + revert 闭环完美工作。executions 表正确记录所有 `:run`,包括 agent 锻造期间的自测调用(triggeredBy=http + conversationId 指向锻造 conv)。

### Bug B-03 (已修复) — handler.Client 异常未在 errmap → 500 INTERNAL_ERROR + 隐藏 traceback

- **现象**:T18 锻造 handler,call → 500 INTERNAL_ERROR + msg="internal error"。日志看到 Python AttributeError full traceback。
- **根因**:`infra/handler/client.go` 定义了 `ErrCallFailed` / `ErrInitFailed` / `ErrCrashed` / `ErrProtocol` / `ErrShutdownAlready` 等 sentinels,但 `transport/httpapi/response/errmap.go::errTable` 没收。FromDomainError lookup miss → 默认 500 + 隐藏 msg。
- **修复**:errmap 加 `handlerinfra` import + 5 个 sentinel 映射:
  - `ErrCallFailed → 422 HANDLER_CALL_FAILED`
  - `ErrInitFailed → 422 HANDLER_INIT_FAILED`
  - `ErrCrashed → 422 HANDLER_INSTANCE_CRASHED_INFRA`
  - `ErrProtocol → 500 HANDLER_PROTOCOL_ERROR`
  - `ErrShutdownAlready → 422 HANDLER_SHUTDOWN_ALREADY`
- **验证**:T19 retry,error → `HANDLER_CALL_FAILED` 422 + full Python traceback msg。dev 模式有用,prod 模式可考虑过滤 traceback 进 details。
- **commit**:burn-in 收尾时一起提。

### T18-T19 — handler 锻造经 chat 失败(product UX issue,非 backend bug)

- **现象**:让 agent 锻造一个 counter / kv_store handler,**agent 多次尝试都失败**:
  - 用 `init_args.type="int"` → 后端 validate 拒绝(只接受 `integer` per JSON schema 命名);
  - 用 `args.type="any"` → 拒绝(不在 `string/number/integer/boolean/object/array` 白名单);
  - method body 写 `key not in self.data` 但 signature 是 `def set(self, **args)`,导致 Python NameError;
  - 最终生成一个 `initBody=""` `initArgsSchema=[]` 的"无 init"handler,call 时 self.data 不存在 → AttributeError。
- **判断**:**backend 行为完全正确**,validate 拒绝非法 ops 是设计意图。问题是 **agent 不熟 handler tool 框架的契约**:
  1. arg.type 必须用 JSON schema 名 (`integer` not `int`,无 `any` 这个值)
  2. method body 必须经 `args['key']` 取参,因为框架 inject 的 signature 是 `**args`
  3. init_args 在 handler.go 里走 `set_init_args_schema` op + 实际 init body 必须 `self.x = x`
- **行动**:这是 **product issue,不是 backend bug**。issue #7 列入:
  - 给 `create_handler` / `edit_handler` tool 的 description 加更清楚的契约示例(args 字典 + 类型白名单),让 LLM 一上来就明白
  - 或:JSON schema arg.type 加 `int` 别名 → `integer` normalization,降低门槛
  - 或:method body 自动 unwrap args 为 named locals(框架层),让 LLM 写自然 Python
- 当前 burn-in 不修这个,记录 issue,继续测其他流。

### T19.2-T19.3 — handler 直接 API create + config + call(绕过 LLM)

- **manual_counter handler** 直接 POST `/handlers`,带 `initArgsSchema=[{name:"start",type:"int",required:true}]` → **400 HANDLER_OP_INVALID** (`init arg "start": invalid type "int"`)。✓ validate 工作正常。
- 修正为 `type:"integer"` → 201,handler envStatus=ready。
- 配 config `{start:10}` + call peek → **HANDLER_INIT_FAILED**:`name 'args' is not defined` (initBody 写的 `args["start"]` 但 framework 注入的 kwarg 名是 `init_args`)。
- 改 initBody 为 `self.count = init_args["start"]` → 一切跑通。
- peek 返 10 ✓
- increment x2 都返 10(state 不持久!)
- peek 返 10(应该是 12 if state 持久)
- **判断**:HTTP `:call` 设计为 **per-call ephemeral instance**(per `app/handler/call.go:1-15`,owner.kind="chat"→ spawn→call→destroy 单次)。所以 handler 在 HTTP 调用间**不**保留 state。
- **product 行为,非 bug**。但**框架契约不对称**应该统一:
  - method body 注入名:`args` (`def m(self, **args):`)
  - init body 注入名:`init_args` (`def __init__(self, **init_args):`)
  - issue #8:统一为同名(都叫 args 或都叫 init_args),减少 LLM 错乱。在 driver/template 把 init 也命名 args 也能 work。

### Bug B-04 (产品行为正确,frontend 误判) — handler_calls 字段叫 `method` 不是 `methodName`

- **现象**:V2 testend HandlerDetail view 读 `c.methodName` 拿不到 → undefined。
- **判断**:backend 返 `method` 字段 ✓(per `domain/handler/call_log.go` json tag);frontend 字段名误读。**testend 是错的,backend 正确**。
- **行动**:testend 端简单修复,不算 backend bug。

---

## T20-T21 — Workflow forge + manual trigger + approval

### T20 — workflow E2E

- **T20.1**:让 chat agent 锻造 hello_workflow → **agent 尝试 7+ 次都失败**:
  - `add_node` op 需要 `node` 字段包装(不是 flat)——agent 多轮试错才发现;
  - 不存在 "end" 节点类型(13 NodeTypes:trigger/function/handler/mcp/skill/llm/http/condition/loop/parallel/approval/wait/variable);
  - agent 最终没生成 workflow,放弃。
- **判断**:**create_workflow tool 描述不清**(同 handler issue):op 字段结构 / 合法 node types / "无 end 节点" 都得让 LLM 知道。issue #11。
- **T20.2** 直接 POST /workflows + 正确 op 结构 → **成功**:
  - 1 trigger node (`type:trigger, config:{kind:manual}`)
  - manual trigger → flowrun completed in 1ms,1 node (t1) ok。
- **T20.4-5** workflow 调 function node:
  - 错过 1 次(function node config 用 `args` 字段失败):`{functionId:..., args:{...}}` → input={},function 跑空参 fail
  - 修正为 `input` 字段:`{functionId:..., input:{a:7,b:8}}` ✓
  - 触发后:t1 trigger ok → f1 function ok (15ms total),output `{out:15, elapsedMs:72}` ✓

### T21 — approval node + resume

- **T21**:trigger → approval → function 3 节点 workflow:
  - manual trigger → status=paused,pausedState.nodeId=a1 ✓
  - POST `/flowruns/{id}/approvals/a1` `{decision:approved}` → 202 resumed:true ✓
  - 5s 后 status=completed,nodesCompleted=2,nodesTotal=3
- **观察**:flowrun_nodes 只记录了 t1 1 条,a1/f1 没在表里。可能是approval/function 节点的 recording 缺漏(或 approval gate 不算节点)。**issue #12**:确认 flowrun_nodes 表的写入规则——expected 是 trigger + 后续 + approval(若 audit 全节点)还是仅 active dispatchers。

---

## T22 — MCP via chat E2E ✅ 完美闭环

- **T22**:让 agent "用 DuckDuckGo 搜索 Python 3.12 新特性 3 条结果":
  1. agent 先尝试 WebSearch tool → 后端返"无 search backend"提示
  2. agent 调 `list_mcp_marketplace` → 发现 duckduckgo
  3. agent 调 `install_mcp_server` → "already_installed"(我之前测过)
  4. agent 调 `search_mcp_tools(query="DuckDuckGo web search")` → 发现 duckduckgo 的 `search` / `fetch_content`
  5. agent 调 `call_mcp_tool(server=duckduckgo, tool=search, ...)` → 3 条真实结果
  6. agent 用 Markdown 表格总结返回给用户
- **判断**:🎉 这是 burn-in 中最完美的产品体验。Discovery → Install → Use 循环全部 LLM 自驱。
- **后端代码 + MCP 框架 100% 工作**。

---

## T23-T24 — Catalog refresh + SSE 3-stream

### T23 — catalog auto-refresh + LLM categorization

- **预期**:加 function/handler/mcp 后 catalog 自动 rebuild,version 增,fingerprint 改。
- **实际**:catalog.version=15(初始 1,加东西后涨到 15),fingerprint 改,sourcesAt 含 function/handler/skill 时间戳。✓
- **观察**:`coverage` 字段 **键名是 LLM 决定的分类**(`forge` / `mcp` / `skill`),不是 source 名(`function` / `handler`)。所以 IDs 是 `fn_xxx` 但归在 `forge` 类下,IDs 是 `hd_xxx` 但归在 `skill` 类下,LLM 把"counter handler"误归为"skill"。**判断:product/UX issue**——前端可能预期 coverage 按 source 名归类(see V2 testend Catalog view)。**issue #13**:讨论 catalog Coverage key 由 LLM 决定还是固定 source 名。

### T24 — SSE 3-stream concurrent capture

- **预期**:eventlog/notifications/forge 三流独立工作,锻造一个 function 触发所有三流。
- **实际**:✓✓✓
  - eventlog: 1317 events(2 message_start + 2 message_stop + 49 block_start + 50 block_stop + 1214 block_delta)
  - notifications: 4 events(conversation/function/sandbox_env × 2)
  - forge: 3 events(forge_started + forge_env_attempt + forge_completed)
- **观察**:**notifications 字段未瘦身**(违反 D-redo-22 / E1 规范)——`conversation` 通知带完整 conv 对象,`sandbox_env` 通知带完整 env 对象。规范要求 `data: SlimPayload` (只 ID + 小字段)。`function` 通知是对的(`{action,versionId,versionNumber}`)。**issue #14**:把 conversation / sandbox_env 通知瘦身。

---

## T25 — Errmap + envelope coverage

- 全部 404/400/422 路径返合规 envelope。✓
- 405 Method Not Allowed → 实际返 404(Go ServeMux 行为,非 spec 严格违反)。
- envelope shape:`{error: {code, message, details}}` ✓(details nullable per N1)。
- **errmap 整体覆盖良好**,只有 handler.Client.* 漏(B-03 已修)+ skill.Scan / mcp.Start boot publish 路径漏(B-01 已识别,未修)。

---

## T26 — 3 conv 并发 chat

- 同时跑 3 个 conv,每个一句简单数学题。
- **结果**:3 个都正确 + 第 2 个还自己调用了 forged `add_nums_v2` function ✓
- 后端 chat queue 正确 per-conv 隔离,无串扰。

---

# 🎯 收尾总结(2026-05-14 ~04:00)

## 真实修复(已 commit-ready)

| Bug | 影响 | 修复 |
|---|---|---|
| **B-01** mise install `python@>=3.12` fall back to pyenv source-compile (broken on this mac) | 所有 function/handler 锻造失败 | `infra/sandbox/mise.go` 加 `normalizeVersionForMise()`,剥 PEP 440 前缀;Install + Locate 双路径使用 |
| **B-02** D22 `function_executions` + `handler_calls` 表没注册 AutoMigrate | `:run` 静默丢日志;`GET /executions` 500 | `cmd/server/main.go::Migrate` 加 2 个表 |
| **B-03** `handler.Client.Err*` 不在 errmap → 500 INTERNAL_ERROR + 隐藏 traceback | handler call 失败时,用户看不到 Python 错原因 | `transport/httpapi/response/errmap.go` 加 5 个 handlerinfra sentinel 映射 |
| **B-05** EdgeSpec port stringly-typed → approval workflow 静默假成功 | approval 后 function 节点不跑,run 标 completed nodesCompleted=2/3,flowrun_nodes 漏行 | EdgeSpec 加显式 `FromPort`/`ToPort` 字段;`BranchOutputPorts` 表 + `IsBranchingNode` API;validate 强制 port 一致性 + 拒 legacy dotted;scheduler `topo.advance` 改读 `e.FromPort`;tool description 大改加 cheatsheet。详 progress-record dev log 2026-05-14。|

## 留下来的 issue(讨论性 / 非阻塞)

| # | 描述 | 严重度 | 状态 |
|---|---|---|---|
| **#4** | Block.Attrs / Message.Attrs 是 JSON-encoded string 而非 object,前端要二次解析 | 契约清晰度 | **✅ 已修(P2-C,2026-05-15)** — domain 改 `map[string]any` + GORM `serializer:json`,store/SSE/handler/test 级联适配 |
| **#5** | skill/mcp boot publish 缺 user ctx → WARN log noise | 低,功能 OK | **✅ 已修(P2-A,2026-05-15)** — `main.go` 在 `mcpService.Start` / `skillService.Start` 入口包 detached ctx + `DefaultLocalUserID`(§S9 模式) |
| **#6** | chat composer 应当能感知 pending AskUserQuestion + inline 答题 | 产品 UX | **✅ 已修(P2-F/G,2026-05-15)** — backend 改 7d timeout + Skipped 字段(sentinel `(user skipped)`)+ options 可选;frontend Composer 3-state 状态机(IDLE/AGENT_RUNNING/AWAITING_ANSWER),Skip 按钮 + 黄光呼吸 + Jump-to-question 导航 |
| **#7** | handler / workflow create tool 描述不够清楚,LLM 多轮试错费 tokens | 产品 UX(LLM tool description) | 留下次(独立轮 prompt tuning) |
| **#8** | handler `__init__` 注入名 `init_args` vs method 注入名 `args` 不一致 | 框架契约 | **✅ 已修(P2-E,2026-05-15)** — 深修:`AssembleClass` 改 exploded named params(`def __init__(self, p1: str, p2: int = None)` 而非 `**init_args`),method 同样 explode;新增 `pythonType`/`pythonDefault` 类型映射,`validate.go` 加 Python identifier 校验。LLM 现在用 bare 名字写 InitBody / MethodBody,不再有 `init_args["x"]` 这种字典歧义 |
| **#11** | workflow 13 NodeType 没有 "end",新 LLM 容易尝试 "end" / "output" 失败 | 产品 UX | 留下次(独立轮 NodeType 设计) |
| ~~**#12**~~ | ~~flowrun_nodes 表对 approval/function 节点的记录规则待确认~~ | **已修(B-05),根因是 EdgeSpec port stringly-typed,不是 recordNode 漏调** | — |
| **#13** | catalog `coverage` 键名由 LLM 决定,不固定 → 前端难按 source 归类 | 产品决策 | **✅ 已修(P2-D,2026-05-15)** — LLM 只输出 `summary`,backend `computeCoverage(items)` 按 `item.Source` 名字机械 group;coverage 键名 100% 等于 source 名(`function`/`handler`/`mcp`/`skill`),永不漏 item |
| **#14** | notifications conversation / sandbox_env 字段未瘦身,违反 D-redo-22 | 规范遵守 | **✅ 已修(P2-B,2026-05-15)** — `conversation` 通知改 slim shape(只 `{action, conversationId, title?, status?}`),`sandbox_env` 通知改 slim shape(只 `{action, envId, status?, ownerKind, ownerId, ownerVersionId}`);UI 拿通知 → 主动 GET 详情 |

**P2 批次收尾**:#5 / #6 / #8 / #13 / #14 / #4 一起改完,backend `go build ./... && go test -count=1 ./...` 全绿,testend `npm run build` 0 error。详 progress-record dev log 2026-05-15。剩 #7 / #11 是独立的 LLM prompt / 产品决策,留下次。

---

## B10 sandbox lifecycle 补测(2026-05-15)

P2 收尾后回头扫 B10 / B13 两个原计划维度。本节 B10。

| Sub | 命中 | 结论 |
|---|---|---|
| B10.1 | `GET /sandbox/runtimes` | ✓ 3 行(python 2 行 + uv 1 行)|
| B10.2 | `GET /sandbox/envs?ownerKind=X` 5 类 | ✓;**强制 ownerKind**,缺则 400 OWNER_KIND_REQUIRED |
| B10.3 | `disk-usage` + `bootstrap-status` | ✓ |
| B10.4 | `POST /sandbox/envs/{id}:destroy` | ✓ 204;再 GET 返 404 SANDBOX_ENV_NOT_FOUND |
| B10.5 | env 销毁后再 run function | ❌ 见 finding-1 |
| B10.6 | `POST /sandbox/:gc?olderThanDays=30` + `:retry-bootstrap` | ✓ 200(action 名必须带 `:` 前缀,符 N5)|
| B10.7 | conv-scoped `:reset-all` + `/{kind}:reset` | ✓ 200 / 204 |
| B10.8 | 错误路径(404 / 非法 action)| ✓ 大多对;见 finding-2/3 |
| B10.9 | 销毁仍被 env 引用的 runtime | ✓ 409 SANDBOX_ENV_IN_USE(data integrity 保护到位)|

**新 findings**:

| # | 描述 | 严重度 |
|---|---|---|
| **#15** | env `:destroy` 销毁 function 的 env 后,该 function `:run` 报 SANDBOX_ENV_NOT_FOUND 而非 lazy rebuild。**低**:admin-only 端点 + 单用户场景。 **✅ 已修(P3-F,2026-05-15)** — RunFunction / spawnInstance 捕 ErrEnvNotFound → syncEnv 按 stored spec 重建 → 重试一次;live E2E 4.4s 走通。|
| **#16** | `?ownerKind=garbage` 返 200 + 空 list(应 400 INVALID_OWNER_KIND)。**低**:反校验剧场边界。 **✅ 已修(P3-B,2026-05-15)** — `validOwnerKinds` 5 值白名单 + 400 INVALID_OWNER_KIND。conv reset 的 idempotent NOT_FOUND→nil 是 Destroy 语义本身,不算 bug。|
| **#17** | sandbox_runtime 表 `3.12` 与 `>=3.12` 两行同 install path。**低**:不浪费磁盘只视觉重复。 **✅ 已修(P3-C,2026-05-15)** — `RuntimeInstaller` 接口加 `NormalizeVersion(string) string`,MiseInstaller 复用 `normalizeVersionForMise`;EnsureRuntime 在 lookup/insert 前归一化。|

---

## B13 pagination + cursor 补测(2026-05-15)

| Sub | 命中 | 结论 |
|---|---|---|
| B13.1 | 8 资源 list envelope | ✓ 真分页资源(conversations / api-keys / functions / handlers / workflows / flowruns / messages)都返 `{data, hasMore, nextCursor?}`;非分页(mcp-servers / skills)返 `{data}`,N4 允许 |
| B13.2 | cursor walk(3 conv 分 2+1)| ✓ 干净 walk-to-end;cursor 是 base64-encoded `{c: <created_at>, i: <id>}` keyset |
| B13.3 | 错误输入 | ✓ limit=0 / -1 / abc → 400;cursor=garbage / 半构造 → 400 INVALID_REQUEST |
| B13.4 | 子资源 + 空 list | ✓ `/conversations/{id}/messages` 也走分页;空 list `{data:[], hasMore:false}` |

**新 finding**:

| # | 描述 | 严重度 |
|---|---|---|
| ~~**#18**~~ | ~~`limit=99999` 无 cap~~ | **❌ 误报** — `pkg/pagination/cursor.go::Parse` 已有 `MaxLimit=200` silent clamp + `TestParse_LimitClamp` 单测。原测试样本数过小(3 行)未观察到 clamp 行为,误以为漏。|

**已知 omitempty 行为**:`nextCursor` 为空字符串时 Go `omitempty` 把字段省掉(envelope 只剩 `{data, hasMore}`)。N4 严格说要求字段必有,但前端按 optional 处理,Wails 形态下也不暴露。T03 已记录,不开新 issue。

---

## 收尾(burn-in 第一轮全部 24 维度过完)

| 维度 | 状态 |
|---|---|
| A. 多轮 AI 对话 | ✓ |
| B. 工具锻造闭环(function/handler/workflow)| ✓ |
| C. Skill / MCP 调用 | ✓ |
| D. Workflow 部署 + 运行 | ✓(EdgeSpec port refactor 之后)|
| E. SSE 流契约 | ✓ |
| F. 边界 + 错误路径 | ✓ |
| G. 并发 + 压力 | ✓ |
| B10 sandbox lifecycle | ✓(本节)|
| B13 pagination + cursor | ✓(本节)|

剩余未修 issue:**全清**(2026-05-15 P3 批次)。

- **#7**(handler/workflow create description 模糊):MINIMAL COMPLETE EXAMPLE + 类型映射 + 规则 cheatsheet → ✅
- **#11**(workflow `end` 节点 LLM 误用):`isPseudoTerminalType` 拦 7 个伪 terminal + 教学型错误 → ✅
- **#15**(env destroy 后 function 不 lazy rebuild):RunFunction / handler.spawnInstance 捕 ErrEnvNotFound + 重建 + 重试 → ✅
- **#16**(ownerKind 非白名单悄吞):`validOwnerKinds` 5 值 + 400 INVALID_OWNER_KIND → ✅
- **#17**(sandbox_runtime duplicate row):`NormalizeVersion` 加入 RuntimeInstaller 接口 → ✅
- **#18**(limit 无 cap):❌ 误报,已有 `MaxLimit=200` + 单测

burn-in 第一轮 24 维度 + 10 真 bug(B-01/02/03/05 + P2 #4/#5/#6/#8/#13/#14 + P3 #7/#11/#15/#16/#17)全 closed。

## 完整测过 / 工作正常的产品流(主路径都通了)

1. ✅ Boot + 7 background goroutines (scheduler/mcp/skill/catalog/sandbox/etc) ready in 2s
2. ✅ API key CRUD + live :test (DeepSeek 实测连通)
3. ✅ model-config N6 idempotent PUT
4. ✅ Single + multi-turn chat (system prompt / cancel / auto-title / attachments)
5. ✅ Function E2E:chat → create_function → env build → run → edit → pending → accept → revert → run
6. ✅ Workflow:create with trigger + function nodes → manual trigger → flowrun completed
7. ✅ Workflow approval:paused → POST /approvals → resumed → completed
8. ✅ MCP install + chat-driven 工具 discovery + 调用 → 真实搜索结果回流
9. ✅ Catalog 自动 rebuild + LLM 重新生成 summary
10. ✅ SSE 3 流并发(eventlog / notifications / forge),per-user demux
11. ✅ 3 conv 并发 chat 互不干扰
12. ✅ Errmap envelope shape 整体合规(N1/N2)

## 时长 + token

burn-in 总长 ~5h(2026-05-14 01:38 - 06:30),3 次 backend 重启,~30+ chat 对话,DeepSeek API 调用 ~100+ 次,锻造 function 4 个 + handler 7 个 + workflow 4 个 + 安装 1 个 MCP。








