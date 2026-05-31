# 01 — 优化桶逐项裁决 + 业界依据

> **定位**:桶 3(优化)的逐项决策表。每项给 `现状(值@位置)→ 裁决 → 目标做法/值 → 业界依据`。
> 裁决记号:✅ **采纳优雅方案** · ➖ **去掉 / 抬到高 ceiling** · 📌 **保留+记录(已是对的)** · 🐛 **顺带真 bug**。
> 原则与分类见 [`00`](./00-overview.md);实施顺序见 [`03`](./03-implementation-plan.md)。值的"可配"指通过 [`02`](./02-advanced-settings-ui.md) 的前端「高级能力」区调,默认值即下表 ceiling。

---

## ① 循环边界

| 限制 | 现状 @ 位置 | 裁决 | 目标 |
|---|---|---|---|
| ReAct `maxSteps` | `20` @ `chat/runner.go:25` → `loop/loop.go:90,182` | ✅+🐛 | 加独立 `StopReasonMaxSteps`(别再冒充 `max_tokens`);默认 ceiling **150**(可配);撞顶写**非成功终态** + 大声报 + 一键「继续」(带历史重入) |
| `maxTurnDuration` | `10min` @ `chat/runner.go:82` | ➖ | cancel 行为已对(`status=cancelled`);默认抬到 **30min**(可配) |
| tool-error-storm | 连续 `3` 全错 @ `loop/loop.go:27` | 📌 | **好失败态(loop-detection 正解),保留**;可选升级为"相同 `(tool,args)` 调用"检测,抓"成功但无进展"的死循环 |
| subagent 总超时 | `5min` @ `subagent/spawn.go:25` | ➖ | 抬到 **10min**(子 agent 不该比父 turn 更紧);可配 |
| subagent `maxTurns` | `30`(Explore)/`25`(Plan,general)@ `subagent/registry.go` | 📌 | 值合理;**子 agent 已有独立 `StatusMaxTurns`(做对了)**;随主修把映射切到新 `StopReasonMaxSteps` |
| **workflow agent 节点 `maxTurns`** | 默认 `10`,硬顶 `50` @ `scheduler/dispatch_agent.go:69,70` | 📌(**唯一例外**) | **保留 cap(无人值守,Esc 救不了)**,但同样要修"假装 completed";`maxTurns`/token 预算可在 workflow 节点 spec 配 |
| conv 队列空闲拆除 | `5min` @ `chat/runner.go:45` | 📌 | 资源清理,非 loop bound;保留(加一行注释澄清) |

**依据**:Claude Agent SDK 默认 `max_turns`/`maxBudgetUsd` **无限**,撞限返回 `error_max_turns`/`error_max_budget_usd` 子类型、**绝不返回 success**([agent-loop docs](https://code.claude.com/docs/en/agent-sdk/agent-loop));LangGraph `recursion_limit=25` → `GraphRecursionError`;OpenAI Agents SDK `max_turns`→`MaxTurnsExceeded`;LangChain `early_stopping_method="generate"`(撞顶再做一次终答而非吐 stub);Pydantic-AI `request_limit=50`+`total_tokens_limit`。Anthropic"Building Effective Agents":停止条件配 **human checkpoint**。仓库 `claude-code-research-documents/01-agent-loop.md §1.5/§6`:CC 无 `maxSteps`,§6#8 已提议删它换 token+turn 软限。

> **本组要点**:不是挑更好的数字,是**让 agent 停止谎报成功**——这是 80% 的价值。`loop/loop.go:182-185` 当前在步数耗尽时写 `StatusCompleted`+`stop_reason=max_tokens`,与 `stream.go:115` 真·token 耗尽不可区分。修法参照 `subagent/spawn.go:165` 已有的 `StatusMaxTurns` 模式。

---

## ② 输出 token

| 限制 | 现状 @ 位置 | 裁决 | 目标 |
|---|---|---|---|
| modelcaps 未知模型兜底 `MaxOutput` | `8192` @ `modelcaps.go:131` | 📌（实测纠正） | **未抬**——抬到 64000 会让 `UsableInput = 32768 − 64000` 触底、压缩失灵（它是压缩预算用值，非输出 cap）；真·输出截断元凶是 **Gemini 不发 `maxOutputTokens`**（已修）。详见 [`03`](./03-implementation-plan.md) 顶部 as-built 注 |
| Anthropic 兜底 `max_tokens` | `8096` @ `infra/llm/anthropic.go:21` | ✅ | 接 **live `/v1/models`**(`06-impl-plan P5.4` 已 spec)读真 `max_tokens`;仍未知时兜底抬到 **64000** |
| 各模型 `MaxOutput` 表 | `modelcaps.go:88-128` | 📌 | 正确底层;上叠 live overlay(Anthropic/Gemini/OpenRouter/Ollama 可 live 读;OpenAI/DeepSeek/Qwen/Zhipu/Kimi/Doubao 只能静态表) |
| `SafetyBuffer` / `UsableInput` floor | `2000` / `1000` @ `modelcaps.go:14,43` | 📌 | 仅影响压缩触发(内部预算),非 wire 参数;保留 |
| Anthropic thinking budget | 半 `max_tokens` 夹 `[1024,8192]` @ `anthropic.go:213-224` | ✅+🐛 | 去掉 `8192` 顶(从 `BudgetMax` 派生);**Opus 4.7/4.8 走 adaptive `effort`,手填 `budget_tokens` 会 400** |
| Gemini thinking fallback | `8192` @ `gemini.go:434` | ✅ | 改 `-1`(Gemini 动态,模型自决);消除魔数,避免"thinking 吃光 output"空响应 |
| **SSE 单行扫描** | `64KB`(bufio 默认,未 override)@ `transport.go:34` + `anthropic.go` SSE | 🐛 | `scanner.Buffer(make([]byte,64*1024), maxSSELine)`,`maxSSELine=8–16MB`(命名常量);否则大 tool-call 参数单帧 → `ErrTooLong` **整条流 abort** |
| (缺口)模型自报 hit max_tokens | 静默结束 @ `loop/loop.go:183` | 🐛 | `stop_reason==max_tokens` 时 surface(UI 徽章/通知);可选 CC 式 auto-continue ≤2 轮 |

**依据**:`max_tokens` **唯一强制**的是 **Anthropic**(SDK 类型 `Required[int]`,LiteLLM 因此注入 4096);OpenAI 可省(用 model-max,o-系列须 `max_completion_tokens`);Gemini 可省但**默认 8192 且 thinking 计入同一预算**→必须显式设大;DeepSeek/Qwen/… 省则用 provider 默认。Forgify OpenAI-compat 路径**本就不发 cap**(`oaiRequest` 无该字段)——所以魔数只伤 Anthropic/Gemini 两条原生路径。robust SSE parser(`go-sse`/`r3labs/sse`)都 `scanner.Buffer` 到 MB 级。仓库 `llm-providers/04~06` 已 spec 三层解析(静态规则 ⊕ live overlay ⊕ user override),**live overlay 复选框未勾**——接通它是 #1/#2 的结构性修复。

> **本组净策略**:`输出 cap = 除非有真实数字,否则不设`,只在 API 强制处(Anthropic)填一个**大且兜底**的数。魔数兜底里只有 `ContextWindow` 偏保守是安全的;`MaxOutput` 偏小**从来不可辩护**(就是静默截断 bug)。

---

## ③ context / 历史

| 限制 | 现状 @ 位置 | 裁决 | 目标 |
|---|---|---|---|
| **`maxHistoryMessages`** | `200` @ `chat/history.go:18` | ➖ | **抬成纯 I/O 上限(如 `2000`)**,让 token 预算 + compaction 成为唯一边界。**已核实安全**:`buildHistory` 对 assistant 走 `BlocksToAssistantLLM` 按 `ContextRole` 投影(archived 丢 / cold 省 / warm 截)+ `conv.Summary` 前置——用量 <70% 按定义装得下、>70% 已压缩,计数 cap 纯冗余,唯一实害是"短消息多、未触压缩 → 开头被静默砍"。**但必须同时修**:`buildUserLLMMessage`(`chat/history.go:95`)**完全不看 `ContextRole`** → 抬 cap 会把已归档 user 消息重新全文塞入(与 summary 重复)。所以让 `buildHistory` 对 archived/cold **统一投影,含 user 消息** |
| 压缩阈值 Soft/Hard | `0.70` / `0.85` @ `contextmgr.go:47,48` | 📌 | **经查正落业界区间**(MemGPT ~70% 驱逐;CLI 95% / VSCode 75% / 服务端 ~75%=150k/200k);0.85 留 15% 收尾余量,**别往 95% 抬**。可配 |
| RecentTurns / TRKeep / WarmCutoff | `3` / `5` / `15` @ `contextmgr.go:49,50` | 📌 | 对齐 Anthropic 默认(保留近 ~3 条、keep 3 tool_use);Forgify 更保守。已有 `SetThresholds` 钩子,可配 |
| warm 预览 / cold 省略 | `200B` / 整段省略 @ `loop/history.go:22,99` | ✅ | **占位符塞 block id** 让模型能 re-fetch(Manus/Deep-Agents 可恢复截断;全文已存 DB,`GET /blocks/{id}`);可选抬到 `500–1000` 且按 token(CJK 友好)算 |
| 压缩兜底窗口 | `32768` @ `estimate.go:16` | 📌+🐛 | 值可留;加 **nil-resolver 启动 WARN/断言**,防 200K/1M 模型被按 32K 压缩(文档自标"严重 bug") |
| `perBlockBudget` | `1500B` @ `prompt.go:59` | 📌 | 仅喂摘要 LLM 的每块上限,合理,保留 |

**依据**:有 token 账本的系统都**靠 token 预算 + 摘要**,不靠消息计数(Anthropic 服务端 compaction 触发 = `input_tokens` 150k/min 50k;OpenAI Assistants 默认 `auto`=token 驱动;消息计数只是"无 token 账本时的简单代理")。Forgify 已有 modelcaps 真窗口 + 校准 + compaction——计数 cap 是**更钝的冗余第二道闸**,且砍在错误的层和时机。Manus/Deep-Agents:可恢复截断保留 URL/path/id 供按需取回。仓库 `compaction.md` 明言"老 block DB 永远完整,只是不发给 LLM";`final-sweep.md §1`/`03-context.md §6` 亲口把 200 计数 cap 标为 compaction 要替代的孤儿。

---

## ④ 工具结果截断

> 关键区分:**A 防御性**(防 tool 把 MB 倒进 context — 合法,但要优雅截 + 给续读手柄)vs **B 语义性**(砍 tool 真正的返回值/检索结果 — 基本都是错的)。

| 限制 | A/B | 现状 @ 位置 | 裁决 | 目标 |
|---|---|---|---|---|
| **`search_*` top-N(统一规格)** | B | 历史不一致:function/handler `3`/`5`(`function/search.go:42`、`handler/search.go:41`)、mcp `5`/`20`(`mcp/search.go:14`)、skill `3`/`10`(`skill/search.go:14`)、workflow `10`、documents `10`/`50` | ➖ | **全部统一为默认 `10` / 最多 `50`**(= 向 documents 现状看齐),收敛成一个共享常量 `DefaultSearchTopN`/`MaxSearchTopN`;候选仅 1 行无 token 理由。**最直接解用户痛点**,且让 [`02`](./02-advanced-settings-ui.md) 的单一 `searchTopN` knob 名副其实。**注**:统一的是"返回 N 个"规格;"是否 LLM 重排"(function/handler/mcp/skill 过、workflow/documents 不过)与"喂 reranker 的候选数"(全量)是另两根轴,不动 |
| **get_function_execution / get_handler_call** | B | input/output 砍 `4096B` @ `function/get_execution.go:60`、`handler/get_call.go:56` | ✅ | 用户明确要看这条执行——**去语义截断**;统一到 **256KB** 防御上限,超出按 **JSON 边界**优雅截 + `offset`/文件回退 |
| 5 个 `get_*_execution` 不一致 | — | workflow/mcp/skill 不截,function/handler 截 | ✅ | **统一**:全部"高防御上限(256KB)内不截" |
| search_*_executions 行预览 | A | `200B`/行 @ 5 个 `search_executions.go` | 📌 | 合法 list 预览(full 一个 `get_*` 即得),保留;仅修下方 truncateJSON bug |
| **`truncateJSON`** | — | 对 `json.Marshal` 结果 `b[:max]` 切片 → 吐**非法 JSON** @ 7 处 | 🐛 | 改成截**结构内的字符串值**(保 envelope 合法)或明确标为纯文本 snippet;一个共享 helper 修 7 处 |
| create/edit env-error UI delta | A(UI) | `240B` @ `function/create.go:249`(DB 留全量) | 📌 | UI 通道截断合法;**确认模型侧 tool_result 拿全量 envError** |
| Read 行数 | A | 默认 `2000` @ `filesystem/read.go:20`(已带"use offset+limit") | 📌 | **已是 CC 正解**,保留 |
| Glob / WebSearch / WebFetch / Bash | A | 100·1000 / 10·30 / 1MB / 256KB | 📌 | 都有续读/`truncated` 信号,保留;Bash 可选改 **head+tail** 中段截(CC 做法) |

**依据**:CC Read=2000 行 + "use offset/limit" 续读、~25k token 硬顶**报错**而非静默;CC Bash ~30k 中段截(留头尾);OpenAI Agents SDK `ToolOutputFileContent`(超大转文件引用);MCP `ResourceLink`/`structuredContent`(引用而非内联)+ "context window is for reasoning, not storage";retrieve→rerank 标准**取 15–50 → 留 5–10**,静态 top-3 的失败模式正是"相关项埋在第 8 位永不进窗口"([arXiv 2512.14313](https://arxiv.org/pdf/2512.14313))。仓库 `08-executions.md §7.1` 证实 200B 是有意的 list→detail 两级设计(预览的 bug 只是 JSON 损坏,非 200B 本身)。

> **决策规则(今后所有 cap 通用)**:cap 合法 ⟺ 它界定**用户没特意要的体量** 且 配 (a) 截断信号 + (b) 取回手柄。砍掉**用户/agent 明确请求**的值、或砍掉改变答案的检索结果 = 错误语义截断,删之。

---

## ⑤ 超时

**总原则(用户拍板)**:超时**不再用来限"健康的慢活"**。能中止 in-progress 工作的机制 = **ctx 端到端传播 + 用户随时 stop**(`scheduler.Cancel`、聊天 cancel-mid-stream 都已在;ctx 已传到 handler RPC / 子进程 / HTTP / LLM 叶子)。只保留三类 timeout:① LLM idle **死连接探测**;② 探针/setup 的 fail-fast;③ 把控制权还给 **agent** 的工具超时(Bash / mcp call,可配高默认)。成本由 agent `maxTurns`(§①)兜,与超时正交。

| 限制 | 现状 @ 位置 | 裁决 | 目标 |
|---|---|---|---|
| **LLM 传输 client** | `Timeout=120s` @ `infra/llm/transport.go:16` | ✅ | **删 120s 总墙钟**(误杀健康长流);`Client.Timeout=0`,`Transport` 只管 `DialContext 10s`/`TLSHandshakeTimeout 10s`/`ResponseHeaderTimeout 60s` + TCP keepalive。**保留一个宽松 idle 网**(`provider.go:38-56` 流循环里 `timer.Reset(idle)`,`idle≈150s`,每 token 重置 → 永不杀健康流)**纯当死连接探测**——静默挂死的 socket 不触发 ctx,无人值守节点会泄漏 goroutine |
| scheduler 节点墙钟超时 | function/handler/mcp/http `30s`、skill/llm `60s` @ `scheduler/retry.go:13-19` | ➖ | **删整张 `defaultTimeouts` 表**。workflow 节点不设墙钟——靠 **run-level ctx**(`scheduler.Cancel` / app 关停)+ 前端「stop run」。真正的活变成「**审计 ctx 传到每个 dispatcher 叶子操作**」,非调超时。(决策 #2 的无人值守护栏 = agent `maxTurns`,不是墙钟) |
| handler RPC | **无 Go 超时** @ `infra/handler/client.go:133` | 📌 | `readMessage` 已 select `ctx.Done`(`client.go:239`)→ 用户 stop / run cancel 即杀,**安全已够**。`MethodSpec.Timeout` 降为**可选 per-method 便利**(默认无),不为"安全"而做 |
| mcp `CallTool` / Bash 工具超时 | mcp `30s` @ `mcp.go:25`;Bash 默认 `120s`/max `600s` @ `bash.go:27` | ➖ | 这俩是"超时 → 把控制权还给 **agent**"(Claude Code 同款),**保留为可配高默认**(经 [`02`](./02-advanced-settings-ui.md) 调);不是限健康活,是让 agent 自决换路子 |
| 探针/ setup 超时 | mcp init 30s / AddServer 3min / HealthCheck 10s / apikey test 10s / server Read 15s·Idle 60s·WriteTimeout 0 | 📌 | **保留**:探针/setup 就该 fail-fast;`WriteTimeout=0` 为 SSE 正确 |
| python/sandbox exec | `opts.Timeout`,`0`=无 @ `sandbox/spawn.go:29` | 📌 | **已是优雅范式**(可配、`0`=无、ctx 兜),其它向它看齐 |

**依据**:Go `http.Client.Timeout` 覆盖 body 读取 → 对流式是反模式(Cloudflare net/http 超时指南;golang/go#31391)。Anthropic SDK 默认 **10min** 且按 `max_tokens` 缩放、设 TCP keepalive;OpenAI SDK 默认 **600s** 当"首 token 预算"。SSE 最佳实践 = idle 超时 + `:` 心跳(Forgify 已有 15s keep-alive)。`transport.go` 原 120s 注释把"wedged connection"(=idle)错当成"总墙钟"——bug 根源。

> **本组净策略**:**ctx + stop 管一切 in-progress 工作;timeout 只用于(a) LLM idle 死连接探测、(b) 探针 fail-fast、(c) 还控制权给 agent 的工具超时**。优先修 LLM 120s + 审计 ctx 传播。

---

## 顺带挖出的真 bug 汇总(进 [`03`](./03-implementation-plan.md) 各阶段)

| # | bug | 位置 | 阶段 |
|---|---|---|---|
| 1 | 循环撞顶谎报 `completed`+`max_tokens` | `loop/loop.go:182` | P1 |
| 2 | `truncateJSON` 吐非法 JSON(7 处) | `function/search_executions.go:137` 等 | P0 |
| 3 | SSE 单行 64KB → 整条流 abort | `transport.go:34`、`anthropic.go` SSE | P0 |
| 4 | Opus 4.7/4.8 手填 thinking budget 会 400 | `anthropic.go:213-224` | P1 |
| 5 | 压缩 nil-resolver 大模型按 32K 压 | `contextmgr/estimate.go:16` | P1 |

> 上轮审计一处更正:catalog 的"1s 轮询 / 3 次重试 / fingerprint"实际在 **skill**(`app/skill/polling.go`/`scan.go`),catalog 按需构建无后台 loop——与本优化无关,记此备查。
