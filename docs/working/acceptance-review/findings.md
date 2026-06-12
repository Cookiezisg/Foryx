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

