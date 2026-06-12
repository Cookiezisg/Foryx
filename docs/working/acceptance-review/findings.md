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
  - **i18n 接缝在场**：environment 段带「Reply in <lang>」指令、prompt 本体保持英文（不整体翻译）——唯回复语言的来源是 AC-24 的待裁决项。

