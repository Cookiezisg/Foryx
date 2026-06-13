---
id: WRK-014
type: working
status: active
owner: @weilin
created: 2026-06-12
reviewed: 2026-06-12
review-due: 2026-09-12
expires: 2026-09-12
landed-into: ""
audience: [human, ai]
---

# findings —— 验收发现（AC-N，每条真机复现 + 亲验定性）

> 严重度：🔴 功能不可用/语义错 / 🟡 体验或一致性 / 🟢 轻症。处置：fixed / pending / wontfix（带理由）/ doc-fix。

## W0 环境与座架

- **AC-1 🟢 function 创建响应嵌套形 `{function, version}` 与 workspace 创建扁平形不一致**（观察，待 W1 定性）
  真机：POST /functions 返 `{"function":{...},"version":{...}}`，POST /workspaces 返扁平对象。前端两套解析。是否统一留 W1 对照全实体后定。
- **AC-2 🟡 function 创建同步阻塞 env 物化**（观察，待 W1 定性）
  真机：首建 function 的 POST 阻塞 26.3s（冷启动下载 python + venv + pip）；运行时缓存命中后仍 ~3-6s（venv 构建）。创建即编辑器场景，秒级以上同步阻塞值得裁决（异步物化 + envStatus=installing 已有列支撑）。

## W1 锻造域

- **AC-3 🟢 api.md `:run` body 写 `{input}` 实为 `{args}`**（doc-fix）
  真机：`POST /functions/{id}:run` 带 `{"input":{...}}` 被严格解码拒 400 INVALID_REQUEST；代码收 `{args, version}`（handlers/function.go:167-170），与 run_function 工具一致。api.md 已重述。黑盒按文档打、被代码拒——正是验收要抓的契约漂移。
- **AC-4 🔴 SpawnLongLived 用 CommandContext——常驻实例绑死在首个请求的 ctx 上**（fixed）
  真机：首个 `:call` 懒 spawn 实例 → 请求结束 ctx 取消 → **实例被连带杀死** → 后续调用全部撞尸体重 spawn（每次调用都付 spawn 代价，且首调后实例必死）。单测从未暴露（测试 ctx 活到断言后）。修复：`exec.Command` 替换 `CommandContext`，生命周期由 handle.Kill/Shutdown 显式拥有（infra/sandbox/spawn.go:141）。**只有真机验收能抓到的级别**。
- **AC-5 🟡 handler driver 无协议护盾——用户 print() 直接污染 JSON-RPC 判死实例**（fixed）
  真机：方法体一行 print → 协议帧解析失败 → 实例判 crash 废弃重生，用户代码「合法操作」杀进程。修复：DriverScript 启动即重定向用户态 stdout→stderr（import/__init__/method/shutdown 全程受保护），协议只经保存的真 stdout 写——与 function driver 同款护盾；print 自此变成调用日志（assemble.go）。
- **AC-6 🟡 stderr 窗口竞态——print 先写却可能后到、被 detach 关在门外**（fixed）
  真机：单跑绿、全波跑挂——stdout（return 帧）与 stderr（print）两条独立管道各自 goroutine 在读，无跨管道顺序保证。修复：detach 前 30ms stderr 宽限（call.go stderrGrace），count=2 复跑确定性绿。
- **AC-7 🟡 approval 孤 timeoutBehavior 不校验——垃圾值静默落库**（fixed）
  真机：`{timeoutBehavior:"explode"}`（无 timeout）201 落库；ValidateForm 只在 timeout 非空时校验 behavior。今天无害、补上 timeout 即毒化该行。修复：behavior 非空必合法（domain/approval ValidateForm）。
- **AC-1 复定性：创建响应嵌套形是六版本实体的统一约定**（by-design 关闭）
  真机对照：fn/hd/ctl/apf 创建一律返 `{<entity>, version}`（双对象都需要：实体头+版本体）；workspace 无版本故扁平。前端按「版本实体 vs 平实体」两类解析即可，约定一致。
- **AC-2 终定性：同步阻塞 env 物化 = by-design，可见性链实测在场**（关闭，场景钉死）
  用户裁决：同步是预期（环境要搞、崩了有 LLM 修复、再不行打回），要求是「阻塞期间用户看得到在搞」。真机实测：阻塞窗口内 notifications 流三连——`function.created` 立即落（前端可先画实体行）→ `sandbox.env_status_changed`(installing，构建开始即推) → (ready/failed 终态)；LLM 修复链 Provision 不分入口都跑、修不好落 envStatus=failed+envError（打回可见）。`TestFunction_CreateEnvVisibility` 钉死该承诺。nuance：逐行进度（pip 输出）仅 chat 锻造路径流（progress 块），HTTP 路径状态级信号——够用；将来编辑器要逐行再把 envfix Sink 接 entities 流。

## W1.5 小尾巴清账（用户指示「这种小问题都顺手做了」）

- **AC-8 🟢 HTTP 编辑器路径 env 物化无逐行进度**（fixed）：`envfix.WriterSink`+`MultiSink` 新地基，function/handler 的 ensureEnv 把尝试/修复行 tee 到 entities 流 forge 终端——不分入口（HTTP/chat/run 重建）面板都看得到逐行；状态级 `env_status_changed` 照旧。
- **PR-14 回收**（fixed）：fire_trigger/HTTP `:fire` 返 `activationId`（fanOut/FireManual 签名升级）。
- **PR-18 回收**（closed）：env 失败通知实已在场（sandbox.env_status_changed failed+errorMsg），原定性漏看 sandbox 层。

## W2 编排域

- **AC-9 🔴 并发政策无任何设置口——四个 overlap 政策三个是死配置**（fixed）
  真机：create 硬编码 `ConcurrencySerial`、PATCH meta 只收 name/description/tags、set_meta op 不含——调度器认真执行的 skip/buffer_one/buffer_all/allow_all 永远配不出来。第五个「设计完整、接线缺失」。修复：PATCH `{concurrency}`（IsValidConcurrency 校验）+ set_meta op 扩展，双面打通。
- **AC-10 🔴 ExtractMeta 零调用——set_meta op 是静默 no-op**（fixed）
  真机：applyOne 对 set_meta 只查形状、注释自信「ExtractMeta owns the header projection」，而 ExtractMeta 生产代码零调用方；workflow.md 同样声称投影存在。LLM/编辑器发 set_meta 改名 → 200+新版本+名字纹丝不动（最恶劣的静默 no-op）。修复：Create（显式 op 赢过扁平字段、并发校验）与 Edit（dirty 才 SaveWorkflow、UNIQUE 重查）接线 ExtractMeta；`TestWorkflow_SetMetaProjection` 钉死。
- **场景批（全绿）**：图校验拒（无 trigger/孤儿）、线性 run CEL 寻址+执行溯源（triggered_by=workflow+flowrun 双列）、control 真路由（选边跑/未选不跑/emit 下游可读）、approval 人在环全链（park→approval_pending 唤回→收件箱→decide yes→续跑 completed）、**kill -9 崩溃恢复**（6s action 中处决、同库重启 Recover 续跑到 completed——durable 终极考试 PASS）。
- harness 增强：Kill9/Restart（同数据目录重启）、client.Try（崩溃场景的不致命请求）。

## W2 编排域（续：trigger 真触发）

- **AC-11 🔴 nil input 的 function/handler 调用必崩——sensor 触发零参函数 100% Python TypeError**（fixed）
  真机：sensor 轮询 → `RunFunction{Input: nil}` → adapter `json.Marshal(nil)` = JSON `null` → driver `f(**None)` → TypeError；activation 记 `fired:false` + traceback，workflow 永不触发。标准 `:run` 传 `{args:{}}` 故掩盖了它。**任何 nil-input 调用方（sensor / 无 input 接线的 workflow 节点）触发零参实体都崩**——读码看不出（input 一路是 `map[string]any`，nil 与 {} 类型相同）。修复：RunFunction（含重试路径）+ handler Call 入口把 nil 归一成 `{}`，driver 收到 `{}` → `f(**{})`。function/handler 同源同修。
- **AC-12 🟢 cron `@every`/秒级被 ParseStandard 拒、错误消息不指路**（doc+UX）
  真机：`@every 2s` → 422 `invalid cron expression`（robfig ParseStandard 只认 5 段、分钟粒度，与分钟桶 dedup 一致——by-design）。但错误消息没说「为什么/该用什么」。修复：错误消息改为「use a 5-field expression (minute granularity); @every and seconds are not supported」。
- **场景批（全绿）**：webhook HMAC 正签触发+坏签 401+activation/firing 双台账；cron `* * * * *` 真等分钟边界触发（67s）；sensor CEL 真轮询（`payload` 变量、condition 真→fire、ReturnValue 入 activation）。

## W3 集成域（MCP + Search）

- **AC-13 🟠 `GET /mcp-calls/{id}` 文档有、路由无——mcp 调用 logs 经 HTTP 完全不可达**（fixed）
  真机：黑盒按 api.md 打 `GET /mcp-calls/{id}` 404。整条链 store（GetCall 带 logs/列表置空，有单测）→ app（GetCall，:triage 在用）→ LLM 工具（get_mcp_call）全在，**只缺 transport 一条路由**——而 handler 同形面 `GET /handler-calls/{callId}` 存在。前端拿不到 mcp 调用的 progress 通知与失败 stderr 尾（第八个「设计完整、接线缺失」）。修复：补路由对标 handler；同时 api.md mcp 节整体重述（registry/import/stderr/invoke 的路径全与实际漂移）+ 删除零消费方的 `GetRegistryEntry`（slug 含 `/` 本就做不成 path 参数，列表即全量）。
- **AC-14 🟠 引擎安装期间 `GET /search/settings` 阻塞 52.7 秒——Status() 与下载抢同一把锁**（fixed）
  真机：log 实锤 `GET /search/settings ... elapsed_ms=52755`。`Builtin.ensureRunning` 全程持 `b.mu`（下载 llama-server+GGUF 可达分钟级），`Status()` 也拿 `b.mu` → 设置页轮询挂到下载完——「downloading」状态本身就是为这个窗口设计的，却被锁顺序弄到看不见。修复：status/lastErr 改独立叶子锁 `stmu`，安装中 Status() 秒回 `downloading`。
- **AC-15 🟢 memory PUT 必填 source/description**（by-design）：`MEMORY_INVALID_SOURCE` 枚举校验（user/ai 写侧语义有别，LLM 工具恒 ai），报错可操作，非校验剧场。
- **testend harness：runtime 缓存 all-or-nothing 回存 bug**（fixed）：`saveRuntimeCache` 见缓存已存在即整体跳过——python 先占位后，node/llamasrv/embedmodel 永远进不了缓存（每跑重下）。改按 kind 子目录合并；现五 runtime 全缓存（python/node/uv/llamasrv/embedmodel ~640MB），RAG/MCP 场景后跑秒装。
- **场景批（全绿，9 个新场景）**：
  - MCP：脚本 stdio server 全生命周期（PUT ready+tools schema 原样透传、`:invoke` 真调、**progress 通知真落调用 logs**、连续 3 失败翻 degraded→一次成功回 ready、失败调用附 stderr 尾、stderr ring、reconnect、删净 404）；错误路径（未知工具 502 `MCP_RPC_ERROR`、坏 command 落盘 failed+lastError 可 reconnect、死 remote 同语义、down 调用 503 `MCP_SERVER_DOWN`、未知 action 400）；import skip/overwrite 语义 + registry 全列 + 装未知 404 + 缺 env 422（下载前拦截）；**官方 filesystem server npx 真装**（node runtime 真下载、真读真文件、台账记账，22s）。
  - Search：8 类实体投影同索引综搜收敛、中文短词 LIKE 回退（2 rune「天气」/1 rune「猫」）、exact-name 置顶、垂搜 types/tags/归档过滤、FTS5 注入安全（6 种元字符/SQL 片段不 500）、`<mark>` 高亮；物化窗口分页 25 行 3 页不重不漏 total 稳定、异查询 cursor 400 `SEARCH_CURSOR_INVALID`；reindex 202 后命中恢复、settings 回显/非法 embedder 400/off 词法照常/ollama 指死端口软降级/空串重置默认；**RAG 真下载真嵌入**——builtin 引擎真装真 spawn，英文查询 `rain umbrella tomorrow forecast` 经向量命中纯中文文档《出行备忘》（零词法重叠 = RRF 融合的物理证明，缓存后 20s）。

## W4 对话域（llmmock 驱动 chat 全链）

llmmock 进场（`testend/harness/llmmock.go`）：OpenAI 兼容假模型 httptest server——真走 provider HTTP 链（请求构造/SSE 流解析/tool-call 组装/usage 记账全被压到）、按 model id 路由独立 FIFO 队列（dialogue 与 utility 互不抢帧）、每请求捕获为 **PromptDump**（模型在线缆上真看到什么 = 体验断言的事实源，柱 B promptdump 同源）。

- **AC-16 🔴 `STREAM_IN_PROGRESS` 契约失效——流式中的 Send 被静默排队而非 409**（fixed）
  真机：流式中（SSE 已见 delta）第二次 Send 返 202。根因：容量 5 的 channel + 任务被取走后即不可见——409 只在「积压 5 条」时触发；chat.md 与代码注释都宣称「正在流式直接 409」，注释自身两句互斥。修复：单槽 + `q.running` 标志（生成中拒）；**finalize 即放行**——回合收尾活（同步压缩检查可达秒级真 LLM 调用）期间的 Send 进槽排队，回复刚完就接着发的消息不被弹回。chat.md 同步重述。
- **AC-17 🔴 provider tool-call id 直接当全局块 PK——撞键整回合静默丢失、行永卡 pending**（fixed）
  真机：mock 每步发 `call_1`（index 风格 provider 如 deepseek/qwen 的家常）→ `WriteFinalize: UNIQUE constraint failed: message_blocks.id` → **「turn lost from history」**：整回合内容丢失、assistant 行永远 pending（前端永转圈、boot SweepOrphans 才扫成 cancelled）。messages.md 本就声明块 id 是 `blk_`——实现违约。修复：tool_call 块 id 一律服务端铸造（provider id 只是响应内关联句柄，accums 本按 ToolIndex 键控；历史回喂用块 id 配对 assistant tool_calls 与 tool 结果，provider 照单全收）；forge scope / interaction 键 / 溯源 ctx 全链随之一致；5 个单测从硬编码线缆 id 改读 pending id。
- **AC-18 🔴 压缩水位线只投影 assistant 块——user 回合永远原文随行**（fixed）
  真机：summary 落库、watermark>0，下一请求里 `TURN1-ANCIENT-MARKER` 仍逐字在场。根因：LoadHistory 的 user 分支绕过 `unfolded()`——摘要+原文双份在场，而用户粘贴正是上下文膨胀的大头，压缩对最该压的部分形同虚设。修复：全折叠 user 回合整条跳过（与 assistant 的 isEmptyAssistant 对称）。
- **AC-19 🟢 EMPTY_CONTENT 不 trim**（fixed）：`"   "` 被 202 接受——落空白 user 回合、为空内容白付一次模型调用。TrimSpace 后再判。
- **AC-20 🟡 能力目录只来自探测档案**（观察，不修）：apikey Create 不自动探测（TestStatusPending）；从未点 `:test` 的 key：模型窗口未知 → **压缩静默禁用**、附件保守渲染。与「utility 未配静默降级」同家族——前端设置页应提示「测试一下你的 key」。
- **场景批（6 个全绿，71s）**：
  - 主链：发送→流式→reasoning/tool_call/text 三块落盘；**懒工具自动发现**（请求 1 线缆工具集 resident-only、模型直接点名 run_function 仍跑、请求 2 起它进入工具集——AutoActivator 实证）；工具结果回喂；function 执行台账 chat 溯源（triggeredBy=chat + conversationId）；usage 精确和（mock 控的 300/40）；SSE 帧；system-prompt-preview。
  - 人在环：dangerous 自报 → interactions 重同步 → approve 真跑（台账 +1）/ deny 不跑且拒绝回喂模型视角 / 重复决议 404 `NO_PENDING_INTERACTION`。
  - 在途控制：SSE 见 delta 证明在飞（DB 行至 finalize 前保持 pending——流是实时事实源）、第二 Send 409、Cancel 落 cancelled。
  - todo+标题：todo_write 落库可查、live 清单 reminder 出现在下一请求模型视角（不污染持久历史）、首回合 utility 自动起标题（用户已命名则不动）。
  - 压缩长程：PATCH limits 调 triggerRatio、4 回合 ~30KB 灌注、真实 input token 越线 → utility 摘要 → **水位线投影实证**（下一请求带滚动摘要、TURN1 原文消失、当前问题在场）。
  - 错误路径：空白 400、未知对话 404 `CONVERSATION_NOT_FOUND`、供应商连环 5xx 落 error 回合（`LLM_STREAM_ERROR`）、25 步触顶诚实报 `MAX_STEPS_REACHED`、全新未配模型 workspace 的 Send 被收下而回合以配置类错误码落地（产品的「去配模型」时刻）。

## W5 平台域（workspace / apikey / model / limits / notification / sandbox / relation）+ 涟漪矩阵 A10

- **AC-21 🔴 `API_KEY_IN_USE` 删除守卫接线于无——任何被引用的 api-key 都能被删、引用静默悬空**（fixed）
  真机：把 key 设为 workspace dialogue 默认模型后 `DELETE /api-keys/{id}` 返 **204**（应 422）。根因：apikey.Service 的 `RefScanner` 端口 + `Delete` 的 scanner 循环 + `ErrInUse`/`API_KEY_IN_USE` + 单测（`fakeScanner`）+ 文档承诺（support-services「删除前查引用」）**全在**，但 `AddRefScanner` **生产侧零调用**——scanner 列恒空 → 守卫永真放行。**第 9 个「设计完整、接线缺失」**，且最阴：单测注入 fake 故绿、code review 见守卫、doc 称其有，唯独线缆被剪；只有真删一个在用 key 拿到 204 才暴露。删后 workspace 默认/agent override 指向死 key → 下次 chat/invoke 以晦涩解析错误崩，而非删时一句干净的「key 在用」。修复：boot 注册两个真 scanner——`workspace.ReferencesAPIKey`（三 scenario 默认模型 + 默认搜索 key）与 `agent.ReferencesAPIKey`（active 版本 modelOverride），均结构上满足 RefScanner、于 build_services 后注入；三引用来源黑盒逐一验拒删。文档承诺自此为真（doc 本就对、code 补齐）。
- **AC-22 🟡 chat `maxSteps` 构造时捕获、`PATCH /limits` 不热换——唯一不实时读的 limits 字段**（fixed）
  真机：`PATCH /limits {agent:{maxSteps:2}}` 后 `GET /limits` 回读为 2（settings 已热换），但驱动一个无限点工具的对话仍跑满（未在 2 步触顶）。根因：`chat.go` 的 `New()` 把 `limitspkg.Current().Agent.MaxSteps` 一次性存进 `s.maxSteps` 字段——而 limits 包文档/CLAUDE/api.md 均承诺「消费方下次读取即生效」，其余字段（ToolResultCapKB/InvokeMaxTurns/MCPCallSec…）都在用点 `Current()` 实时读，独此一个构造时定死。修复：删 `s.maxSteps` 字段，`runner.go` 调用点改实时读 `limitspkg.Current().Agent.MaxSteps`——热换下一回合即生效，与全体 limits 消费方一致。
- **AC-23 🟢 limits 11 字段全有真消费方（非空壳）**（by-design 关闭）：逐字段 grep 消费点——MaxSteps→chat、InvokeMaxTurns→agent、TriggerRatio→contextmgr、LLMIdleSec→infra/llm、MCPCallSec→mcp、BashDefaultTimeoutSec/BashOutputCapKB→shell、ReadDefaultLines→filesystem、ToolResultCapKB→loop、AttachmentMaxMB→attachment+上传、WebhookBodyMaxMB→webhook，全部在场。product-review 已硬化过的 limits 这轮无新空壳；本轮验的是**运行时热换真生效**（AC-22 即唯一漏网）。
- **场景批（8 个全绿，73s）**：
  - workspace：CRUD + 校验四态（空名 `WORKSPACE_NAME_REQUIRED`/重名 `WORKSPACE_NAME_CONFLICT`/非法语言/非法 webFetchMode）+ `:activate` 刷 lastUsedAt + 最后一个拒删 `CANNOT_DELETE_LAST_WORKSPACE`（删到只剩 1 时精确触发）。
  - 删除级联：含 function（落盘 env）+ 对话的 workspace 删除及时 204（Reaper 跑完不挂死）→ ws 行 404 → 其下数据带旧头不可达。
  - apikey：创建校验（未知 provider/空 key）+ `:test` 两态（活 200/死 baseUrl 422）+ 重名 409 + **被引用拒删三来源全验**（dialogue 默认 / 默认搜索 key / agent override）。
  - model：scenarios 白名单（dialogue/utility/agent）+ 默认模型校验（非法 scenario `MODEL_SCENARIO_INVALID`/残缺 ref `MODEL_REF_INVALID`）+ capabilities 经探测档案聚合现身 mock 模型。
  - limits：非法 triggerRatio 400 `SETTINGS_LIMITS_INVALID` + maxSteps=2 热换真 2 步触顶 + **toolResultCapKB=1 经 promptdump 实证**模型真收到被截到 ~1KB 的 tool_result（5KB 函数返回值）。
  - notification：function.created 真落 → list/未读计数/标已读减计/全标已读归零。
  - sandbox：bootstrap-status + runtimes 列 + disk-usage 物化后 >0 + envs ownerKind 守卫（缺失/非法各 400）+ 单 env 销毁 204。
  - relation（A10 涟漪）：agent equip function → neighborhood 现 equip 边（toName hydrate）；改 function 名 → 边 toName 自动跟随（图存 id、名读时取）；relgraph 全景含 agent；删 function → PurgeEntity 清边（neighborhood 不再含 fnId）。

## W6 体验静态审查（柱 B：promptdump 审读——「模型在线缆上真看到什么」）

llmmock 的 PromptDump 把每个视角的 system prompt / 工具 schema / 请求体抓成可断言事实。`promptdump_test.go` 7 场景审读三视角（Chat 主 LLM / Utility / Agent 实体）+ 用户透明面 + 横切刀。

- **AC-24 🟡 assistant 回复语言由 `Accept-Language` 请求头决定，`workspace.language` 持久化设置不参与**（fixed——用户裁决 AC-PD-2：workspace.language 权威）
  真机：把 workspace 语言设为 `en` 后发消息，模型收到的 environment 段仍是「Reply in Chinese」。根因：locale 仅由 `InjectLocale` 中间件从 `Accept-Language` 头解析（无头默认 zh-CN），全栈无任何处把 `workspace.language` 灌进 locale ctx；chat runner 的 `t.locale` 也是头派生值。后果：用户在设置页显式选了语言，assistant 却按浏览器头（或默认 zh-CN）回复——显式设置被静默忽略。与「设计完整、接线缺失」同族（字段存在、校验齐全、却不驱动它最该驱动的东西）。**用户裁决：workspace.language 为权威**。修复：`WorkspaceResolver` 端口 `Validate→Resolve`（返 workspace 的 UI locale），`IdentifyWorkspace` 中间件在 workspace 识别后用它 `SetLocale` 压过 `InjectLocale` 设的头默认（中间件链 InjectLocale→IdentifyWorkspace，后者在内层故覆盖生效）；头仅作 pre-workspace（onboarding）兜底。chat 任务 ctx 的 `t.locale` 本就读 `GetLocale(ctx)`，故修复自动贯通到 environment 段，无需改 chat。i18n 场景收紧为 zh-CN→Chinese / en→English 实证。
- **W6 正面确认（审读通过、无回退——这些是「审了、干净」的事实，进终报）**：
  - **system prompt 结构**：identity→how_to_work→tools→capabilities→memory→documents→user_system_prompt→environment→architecture_rules→critical_rules，每段 `<section name>` 包裹；身份只现一次、无空 section 残壳、无安全剧场套话（"As an AI language model" 等四类均缺席——符合「本地单用户无 safety theater」）。
  - **lazy 工具目录浮出**：tools 段列全 lazy 工具 name+一句话（LLM 知全集不盲搜）；forged function 真进 capabilities 菜单。
  - **S18 框架字段注入齐全**：每个线缆工具 schema 的 `properties` 都含 `summary`/`danger`/`execution_group`，且工具 description 非空（LLM 选型靠它）。
  - **R0057 透明度无漂移**：`GET /system-prompt-preview` 与模型真收到的 system prompt **逐字一致**（同对话同日，二者都走 buildSystemPrompt）——用户看到的就是模型看到的。
  - **视角隔离**：Utility（首回合起标题）收到的是紧凑专用 prompt，不泄漏 chat 全 section（无 identity 段/无 Searchable tools），且确实引用了对话内容；Agent 实体 :invoke 收到「You are <name>, a workflow automation worker. Your role: …」自有身份，不漏 chat 主视角。
  - **空态自举连贯**：零 forged 实体的 workspace，核心身份/规则段仍在、无残壳。
  - **i18n 接缝在场**：environment 段带「Reply in <lang>」指令、prompt 本体保持英文（不整体翻译）——回复语言来源 AC-24 已修为 workspace.language 权威。

## W7 金标真模型旅程（柱 C：make evals / deepseek-v4-flash，真烧 token）

`golden/golden_test.go` 把 llmmock 换成**真 deepseek-v4-flash**（key 在仓库根 `.env`，`make evals` 自动 source），断言只看**结果状态**（实体建了 / function 跑了 / memory 召回了），不看逐字——真模型非确定。`say()` 每轮自动放行人在环门（danger→approve_always、ask→accept），故真模型把工具自报危险也不卡。

- **W7 正面结论（7/7 全绿，真模型，首跑即过）——真模型真驱动得了产品工具面**：
  - **J1 Bootstrap**（13.7s）：空 workspace + 完整工具面，开放问题得连贯非报错引导。
  - **J2 Build+Run Function**（20.5s，旗舰）：真模型 **create_function 再 run_function**——functions 列出实体 + 最终答复报出和 5。锻造→执行整环真模型跑通。
  - **J3 Build+Call Handler**（26.5s）：真模型 **create_handler**（有状态服务，env 物化 + 进程 spawn）再 call_handler——handlers 列出实体。常驻服务锻造+调用真模型跑通。
  - **J5 Debug Function**（24.4s）：预置引用未定义变量的 bug function，真模型诊断 + **edit_function 真落新版本**（active 版本号前进 ≥2）。AI 自愈环真模型跑通。
  - **J7 Search Building Blocks**（12.2s）：预置一个 function，真模型用**搜索**找到它并报出确切名字。积木检索（search_tools/search_blocks）真模型跑通。
  - **J9 Memory Write+Recall**（14.1s）：对话 A 真模型 **write_memory** 落库；对话 B（全新）经 system prompt 注入**召回**（命中 codename）。memory 写读环真模型跑通。
  - **J12 Degraded Main Path**（9.7s）：仅配 dialogue（utility 缺）——起标题/压缩静默降级，主问答照常完成不报错。
  - **无新 AC bug**：工具面 schema（S18 框架字段）、懒工具自动发现、danger 门、memory 注入、降级面在真模型下全部如设计工作。**柱 C 证明柱 A/B 的 llmmock 结论不是假模型的假象**。
  - 范围说明：golden 现 7 旅程覆盖 function 锻造+执行 / handler 常驻服务 / AI 自愈 / 积木检索 / memory / 自举 / 降级七条核心能力线。柱 C 计划的 workflow-to-parked / MCP 真装 / skill / 跨压缩长任务等更重旅程，已在 W2/W3/W4 用 llmmock 结构性覆盖；真模型 golden 视价值/稳定性按需增补（flash 模型搓多节点图易飘，过重项不强求）。

## R1 A7 Search 高标准补全（程序重开后首波，标尺=W1/W2）

新增 `search_lifecycle_test.go`（投影全周期 12 实体 / 粒度锚点 / 排序折叠 / 代码符号+混合查询 / 隔离 / 密文红线 / boot 对账）+ `search_llm_test.go`（8 垂搜真走内容引擎 / search_blocks 三段精度链逐档 / search_conversations 回忆窗）。缺口矩阵见 [R-PLAN.md](R-PLAN.md)。

- **AC-26** 🔴（三处生产面同死：LLM 解析链手抄误用——**第十种模式：地基 API 设陷 + 复制传播**）：`Factory.Build` 返回 `(Client, 解析后 baseURL, error)`，而 **三个**后加的 utility 消费方全把第二返回值绑成 `modelID` 喂进 `Request.ModelID`——线缆上发出 `"model": "http://127.0.0.1:.../v1"`。真 provider 必报 model 不存在 → 三个功能**生产从未工作**、且全部静默降级不报错：① `bootstrap/searchsift.go` —— search_blocks 三段精度链的 tier1/tier2 永远失败、永远落 tier3 纯索引（用户以为精选在工作）；② `app/envfix/fix.go` —— AI 依赖自愈建议永远失败；③ `app/tool/web/fetch.go` —— WebFetch 摘要永远失败。**单测全绿**（fake 链不真上线缆）、唯一正确用法在 bootstrap resolvers.go（主 chat 链没死，故 W4/W7 全过）。黑盒抓法：R1 新断言「sifter 必须被咨询 + prompt 含整目录」→ 零 utility dump → 后端日志（新补的 zap.Error）现形 `model="http://..."`。**修复（标准化非补丁）**：新建 `app/modelclient.Resolve` 唯一解析链，bootstrap 四 resolver 核 + 三病灶全部委托；顺手补两处 sift 失败错误日志（原来静默吞错，utility 断线伪装成普通排序）。文档：stream-llm.md 增"禁止手抄该链"。
- **AC-27** 🟡（mcp 可接线 ref 物理死亡：投影键与挂载键不一致）：mcp searchsource 按 **msv_ 行 id** 键控投影，refHint 渲染成 `mcp:msv_…/<tool>`；而 agent 挂载解析（`mount.mcpTool`）只按 **server name** 匹配（`mcp:<name>/<tool>`，与 HTTP `/mcp-servers/{name}`、api.md「name 即键」一致）。后果：search_blocks/综搜给 LLM 的每条 mcp ref 都挂载不上（`MCP_SERVER_NOT_FOUND`）——「ref 直填」契约对 mcp 类从未成立。skill 早已按 name 投影（代码库自身标准），mcp 是漏网。修复：mcp 投影身份换 server name（Stamps/Docs/3 处 notifySearch）；旧 id 行由 boot 对账孤儿清理自愈、零迁移。黑盒钉死：R1 GranularityAnchors 断言 `refHint == "mcp:granmcp/<tool>"`。
- **AC-28** 🟢（垂搜渲染不一致，记录不改）：8 个垂搜中 7 个经 ContentSearch 返回 slim `{count, list}` JSON，`search_documents` 引擎同源但自有散文渲染（`- name (id=…)` 列表 + 空态指路 list_documents）。LLM 面两种形状并存——按 doc 工具自身定位（path 浏览心智）保留现状，文档已改述精确计数（7+1）。
- **AC-25** 🟢（定性：休眠口 + 文档误导）：`Service.Retrieve`（RAG 内部口、search 域文档自称"四个出口"之一）**零生产消费方**——`grep -rn "\.Retrieve(" backend/internal` 仅定义处。archive 设计文档言明它为"agent 上下文注入/未来知识挂载"预留，非接线缺失 bug；但 reference 把它列为活出口有误导、PLAN 把 `Retrieve(MaxChars)` 列为必验格而黑盒不可达。处置：`domains/search.md` 已补"当前零生产消费方（休眠口）"一句；验收面记 **N/A（黑盒不可达，单测覆盖管线）**。
- **N/A 台账（黑盒不可达格，亲验定界）**：① Retrieve（上）；② 垂搜"引擎出错回退原子串路径"——黑盒无法注入引擎错误（引擎 nil 仅当 search 服务整体缺席），回退逻辑由 `contentsearch.go` 单测覆盖；③ `fts_schema_version` 不匹配→boot 清空重建——需直改 DB 文件，白盒不变量；④ ollama 真连真嵌——本机无 ollama，W3 已验"设置生效+死端口软降级"边界；⑤ 换 embedder 模型→向量失效→后台重嵌——黑盒证明需第二个真嵌入引擎（同④），`search_embeddings.model` 逐行记账的失效/重嵌逻辑由 search 单测覆盖、builtin 全链由 W3 RAG 真嵌实证。

## R2 A4 Agent 整域（agent_test.go 新建，首轮零覆盖的补课）

6/6 全绿，**无新后端 bug**——A4 在高标准黑盒下全部如设计工作：

- **挂载合成（核心格）**：fn / hd.method / mcp:server/tool 三类真合成绑定工具（名=函数现名 / `<handler>__<method>` / `mcp__<server>__<tool>`），agent 线缆工具集**恰为挂载**、四类系统工具零泄漏；三工具真调通且四处台账齐（fn 执行 / hd call / mcp call 各 TriggeredBy=agent + agent 执行行带自包含 transcript）。
- **运行时按现名重解析**：改 function 名 → 下次 invoke 工具自动换新名（ToolRef.Name 只是快照）。
- **fail-fast 三态**：挂载物被删 → run 落 failed + errorMsg 带因（HTTP 200、失败可审计——产品语义：配置坏的是"运行失败"非"请求非法"）；合成撞名（fn `greeter__hello` vs hd `greeter.hello`）→ failed 引撞名；`ag_` 挂载在 create 即拒（员工不调员工）。
- **prompt 组装**：身份段 + skill `Execution guide` 段（指南注入、非激活非 fork）+ outputs declared → JSON 硬约束 + knowledge 前缀进 user 消息；chat 主视角零泄漏。
- **modelOverride 物理证明**：override agent 的请求落自己的 mock 队列、默认 agent 队列分毫未动。
- **三入口**：HTTP :invoke（manual）/ chat invoke_agent（**E3 嵌套实证**：provider call id 重映射为 blk_ 后，agent 流式 text 节点 open 帧 parentId=持久化 tool_call 块 id、close 带完整快照；结果回喂主对话；台账 chat+conversationId）/ workflow agent 节点（active v2 配置真驱动、结果记忆化进节点行、台账 workflow+flowrunId）。
- **版本面**：:edit 全量替换 → v2 即时生效；:revert 回 v1 下次 invoke 生效。
- 线缆事实（接手须知）：InvokeResult/执行行 status 词汇 = `ok|failed`（与 function 域一致）；`GET /agents/{id}/versions` 与 `GET /api-keys` 返回**裸数组**；执行列表返回 `{executions, aggregates}`。

## R7 柱C 金标补全（golden_r7_test.go：J4/J6/J8/J10/J12b 五旅程，真模型 deepseek-v4-flash）

- **AC-30** 🟡（体验陷阱，AC-20 同族——记录待前端提示，无后端改动）：把 deepseek 以 **openai-compatible 路线**（provider=openai + 自定义 baseURL）接入时，窗口/能力静态表按 (provider, modelID) 查不到 → 预算未知 → **压缩按设计静默禁用**（`contextmgr`: "unknown budget — don't compact blind"）。J12b 首跑据此全哑。正解 = 用户选 deepseek provider（静态表 1M/384k 命中）；产品侧应在 key 探测/设置页提示「该模型窗口未知，长对话压缩不可用」。golden 套件已改为 provider=deepseek（真实用户选法）。
- **J6 溯源语义（非 bug，正面实证）**：三轮实跑 flash 各走了**三条不同合法路线**完成同一任务——chat 直调（triggeredBy=chat）/ 委托 Subagent（子运行记 agent）/ **自搓 workflow 用 mcp 节点跑**（记 workflow）。断言改为路线无关（ok + 溯源在账）；这是产品工具面组合自由度的活证据。
- J4 真模型搓三节点图 + trigger_workflow：flash 首轮常建图不触发——旅程按真实用户范式给自愈迭代回合（J5 同款）。线缆事实（接手须知）：**`parked` 是节点状态、run 行保持 `running`**——必须查 `GET /flowruns/{id}` 详情的 nodes，列表行匹配不到；flowrun 列表 data 是裸数组。
- **终绿：5/5**（J4 建图→触发→真挂 parked 44s 一轮过 / J6 mcp 真调（Subagent 委托路线）/ J8 跨对话回忆 GOLDHARBOR 命中 / J10 skill 激活且遵循 SKILLSTAMP / J12b 压缩真发生且跨边界召回 GOLDCOMPACT）。与首批 7 条合计 **12/12 旅程全绿**（provider=deepseek 切换后 J1 金丝雀复验通过）。

六视角/六状态/横切刀的首轮缺格全补，**无新后端 bug**：

- **Subagent 视角**：子请求自足（父用户原文零泄漏、只见 Subagent prompt）；Explore 工具集真是只读侦察（无 Subagent/锻造/run）；自有紧凑 system、非 chat 主 prompt。
- **前端开发者视角（三流帧线缆形状审读）**：envelope 恒 `{seq, scope:{kind,id}, id, frame}`、frame 带 `kind` 判别（open/delta/close/signal 全集）、**delta 恒 seq=0**（E2）、三流各有 durable 帧、messages scope=conversation。接手须知：帧 kind 是判别字段而非 type-key 对象。
- **规模态**：5 实体基线 vs 200 实体——system prompt 体积 <3×（懒目录不线性爆炸的物理证明）。
- **降级态**：零配置 workspace 的 system-prompt-preview 仍连贯渲染（自举调试面活着）。
- **崩溃恢复态**：kill -9 重启续聊——模型视角的灾前回合**恰一次**（重水合无重复无残缺）。
- **长程压缩后态**：滚动摘要在模型视角**恰一次**、被压回合尽出、当前回合在场（压缩保留近窗：需足量可折叠旧回合——4 回合形）。
- **tool_result 配对刀**：线缆历史里每个 assistant tool_call id 恰有一个 role=tool 回包、零孤儿（sanitizer 红线黑盒钉死）。
- **token 账单刀**：三回合（含工具回合双请求）usage 与 mock 上报逐数相等（666/49）。

## R5 A10 跨域涟漪矩阵（ripple_test.go 新建，3/3 全绿）

**矩阵台账**：{创建/改名/删除} × 12 实体 × 6 面，逐格归口（✅=黑盒已测于；N/A=物理不存在该格，亲验定界）：

| 涟漪面 | 覆盖 | N/A 格（亲验原因） |
|---|---|---|
| **搜索索引** | 建/删 ×12：R1 ProjectionLifecycle（content token）+ R5 Matrix（exact-name）。改名 ×9：R5 Matrix（标题跟名；function/handler 旧名随代码体常驻 = by design 不断言出局） | skill/memory/mcp 改名（name 即 id，无改名操作） |
| **catalog** | 建/改/删 × 6 积木类（fn/hd/ag/ctl/apf/wf）：R5 Matrix（coverage 进出 + summary 跟名） | document/conversation/trigger/skill/memory（catalog 是能力菜单、内容类不入——代码事实）；mcp 工具经动态工具面呈现（W3） |
| **通知** | created/deleted ×8 + updated 改名族：R5 Matrix；11 域 created：R4；scheduler 族（run_failed/approval_pending/lifecycle/attention）：W2 | trigger 无生命周期通知（events.md 言明：活动走 activations+fire 信号）；mcp 三态于 R4（AC-29 修复后） |
| **关系图** | agent 五类挂载出边 + 改名水化跟随 + 删除级联清：R5 RelationGraphFaces；trigger↔workflow 绑定边：R5；document wikilink link 边（按 **id** 链接）：R5；apikey 引用守卫：W5 | conversation @mention **不产边**（快照非引用，relations 仅 purge+Namer——代码事实）；锻造 create/edit 入边随 :iterate（AI 面，R7 酌情） |
| **挂载方跟随** | agent 挂载按现名重解析（改名跟、删除 fail-fast）：R2；trigger 改绑 workflow 重监听：W2 | hd/mcp 挂载跟名与 fn 同机制（mount 单测 + R2 fn 代表性实证） |
| **引用方报缺** | workflow :capability-check 删 fn 后报缺 + **同名重建不救**（ref 按 id）：R5 ReferenceRipples；apikey 三引用拒删：W5；env 在用拒删 runtime：R4 | — |

- 过程中测试侧自伤两处（已修、非产品 bug）：①把实体名写进代码体导致旧名"出不了局"——function/handler 名随 def/类名常驻正文是设计事实；②wikilink 误用名字——按 id 链接（`[[doc_xxx]]`）。

## R4 A9 平台高标准补全（platform_r4_test.go 新建，5/5 全绿）

- **AC-29** 🟡（通知族名义存在、物理哑火——模式 #1 第 11 例）：`events.md` P4 契约写明 `mcp.{installed,updated,removed,reconnected}` 通知族，但 `app/mcp` **从未持有 notification Emitter**（其余 11 个发射域都有）——整族从未发出。R4「11 域通知全到达」机械扫直接抓出（10/11、独缺 mcp）。修复：mcp Service 加 `SetNotifier` + 四事件点（AddServer 区分 installed/updated、InstallFromRegistry、RemoveServer、Reconnect；Import 经 AddServer 自然覆盖）+ bootstrap 接线。
- **SSE 协议面全绿**：entities 流 forge 镜像 + run 终端 stderr 帧真到达；notifications durable `fromSeq` 续传重放；**环淘汰真验**（>256 durable 后 fromSeq=1 → 410 `SEQ_TOO_OLD` 走 Envelope）。
- **limits 逐字段热换全验**（与 W4/W5 合计九字段全覆盖）：attachmentMaxMB（1.5MB 在 50 默认过、=1 拒）/ webhookBodyMaxMB（正签 1.5MB 默认过、=1 → 413；入站路由 = `/webhooks/{trgID}/{配置 path}`）/ bashDefaultTimeoutSec=1 真切 5s 命令 / bashOutputCapKB=1 真截 200KB 洪水 / invokeMaxTurns=2 真切死循环 agent（stopReason 诚实带 max）/ llmIdleSec=2 流中 8s 静默 → 回合错误码落地。N/A：mcpCallSec（脚本 server 无慢工具）、readDefaultLines（Read 面随 R5 涟漪酌情）。
- **通知中心**：11 发射域 created 族全到达；unread 单减 + read-all 清零（线缆：`{unread}`、`PUT /{id}/read`、`POST /read-all`）。
- **sandbox 治理**：runtimes 列表 / disk-usage / :gc；**删除守卫**（env 在用 → 409 `SANDBOX_ENV_IN_USE`，清 env 后放行）——比计划格更严的正确行为。
- **级联逐资产**：12 类全建（共享 token）→ 删 ws → keeper 隔离无恙（total=1 恰己方）、同名重生 ws 索引/列表零残留。

## R3 A8 Chat 高标准补全（chat_r3_test.go 新建，10/10 + W4 原 6 全绿）

**无新后端 bug**——首轮全缺的十个面在高标准黑盒下全部如设计工作：

- **附件三路按能力门控**（gpt-4o：vision ✓ nativeDocs ✗）：文本内联原文 / 图片成 `image_url` part / PDF 走 **sandbox 真抽取**（pdfplumber 共享 env 首用真装）后文本内联——三路同回合并存、模型视角逐一验到。
- **skill 两路**：inline activate 渲染正文回喂 + **allowed-tools 预授权实证**（active skill 下自报 dangerous 的 run_function 零询问直接执行、interactions 空）；fork activate 真派 subagent（`context=fork` 必须声明 `agent` 类型，缺则 422 `SKILL_FORK_REQUIRES_AGENT`——校验在岗）、结果同步回喂、sub-message 落库。
- **memory 两段式注入**：unpinned = name+description **索引**入 system（全文不入）+ `read_memory` 取回全文；**pinned = 全文直接入 system**；forget 后新对话彻底消失。写读忘 + pin 升级全环闭合。
- **@mention 冻结**：发送时刻快照实体内容入模型视角；实体后续改版，同对话后续回合的历史快照**保持 V1**（freeze-on-send）。
- **归档 Send 自动解档**；**删除对话取消在途生成**（stalled 流中 DELETE → 404 + 后续对话不受阻、无孤儿）。
- **并行工具批**：同回合两个 tool_call（同 execution_group）都执行、两个 tool 结果**同一请求**回喂、两条台账各一。
- **Subagent 嵌套树**：general-purpose 子运行结果同步回喂；sub-message 以 `subagentId` 落父对话（重水合源）；**子集物理剔除 Subagent 工具**（深度 1 守卫实证）。
- **SSE 重连 replay**：live 流带 ephemeral delta；`fromSeq=<durable seq>` 重连重放其后 durable 帧（close 带全文快照）、**delta 绝不重放**（E2）。线缆事实：fromSeq=0 是「仅实时」哨兵、重放语义 = seq > fromSeq。
- **utility 缺席静默降级**：未命名不起标题、压缩越线不压缩、主链三回合零错误。

