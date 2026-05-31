# Forgify 完成度 / 契约一致性审计 — 报告

> 配套指令:[`completeness-audit.md`](./completeness-audit.md)
> 审计日期:2026-05-26
> 方法:5 个并行只读 agent 分区扫描(prompt/工具描述 · 端口接口接线 · 契约文档 · HTTP 路由 · Phase 完成度/执行引擎);全部 🔴 由人工亲自复核(claim 片段 + 实际片段并列 + 全局 grep 取证)。
> 纪律:**本次为只读审计,未修改任何被审计的代码或文档。**

---

## 严重度汇总

- **🔴 2**(均为新发现、均已人工复核)
- **🟡 ~14**(分 7 个主题)

### 已知线索(核对仍成立,不重复计入)

1. `trigger_workflow` 工具不存在 —— `internal/app/chat/multi_agent_prompt.go:35` 教学引用,全局无 `Name()` 返回它。
2. catalog 仅接 function/handler/skill/mcp —— `cmd/server/main.go:410-413`;document 有 `AsCatalogSource()` 未注册、workflow 连方法都没有。

---

## 🔴 高 severity(调用必失败 / 能力用不了)

### 🔴-1 `edit_forge` 是所有「AI 迭代 / AI Triage」流程的核心指令,但这个工具不存在

- **类型**:幽灵引用(ghost reference)
- **位置**:`internal/app/askai/forge_context.go:51`(function)、`:99`(handler)、`:140`(workflow);`internal/app/askai/triage_context.go:99`(triage)
- **声称**:四个 iterate/triage system prompt 把 `edit_forge` 作为**唯一必做动作**下发给 agent:
  - `forge_context.go:51-53`:`...then call edit_forge with functionId=%q + ops... After edit_forge succeeds, summarize...`
  - `forge_context.go:100`:`...call edit_forge with handlerId=%q + ops...`
  - `forge_context.go:140-141`:`...call edit_forge with workflowId=%q + ops (add_node / update_node / ...)`
  - `triage_context.go:99`:`...call edit_forge (function/handler/workflow) or edit_document...`
- **实际**:全局无任何工具 `Name()` 返回 `"edit_forge"`(`grep -rn 'return "edit_forge"' backend` → 空,exit 1)。真实工具是 `edit_function` / `edit_handler` / `edit_workflow`,且参数名是 `id`(不是 `functionId`/`handlerId`/`workflowId`)。
  - **铁证旁证**:同文件正下方 `BuildDocumentContext`(`:165-166`)正确写 `call edit_document with id=%q` —— document 路径用真名 + 真参数,forge 三条路径却用幽灵名 + 错参数。典型「`edit_forge` 拆分成 `edit_*` 后这几个 prompt 没跟着改」的陈旧残留(`subagent_test.go:69`、`domain/function/version.go:42` 也还留着 `edit_forge` 字样)。
- **撞墙场景**:用户在某个 function/handler/workflow 上点「AI 迭代」或在 flowrun 上点「AI Triage」→ 系统起对话、注入此 system prompt → agent 被指令调 `edit_forge`,而它的 `tools` 数组里只有 `edit_function` 等、根本没有 `edit_forge`。强模型**或许**能猜到替代工具,但 prompt 同时把工具名和参数名都说错;弱模型 / 照字面执行会卡在整个功能唯一有价值的那一步。这是平台旗舰级「AI 改你的 forge」功能。
- **修复方向**:把三处 `edit_forge` 换成对应的 `edit_function` / `edit_handler` / `edit_workflow`,并把 `functionId`/`handlerId`/`workflowId` 改成 `id`。**小修**(2 文件 ~5 处字符串)。
- **置信度**:high —— 工具名全局缺席已证;调用链(handler `:iterate` → `askai.Spawn` → `chat.Send` → 主 chat 工具注册表)清晰;document 路径的正确写法构成同文件反证。

### 🔴-2 `set_dependencies` op 教 LLM 用 `"dependencies"` 键,但代码只读 `"deps"` —— 依赖被静默丢弃

- **类型**:文档-代码漂移(prompt 教学 ↔ apply 实现)
- **位置**:`internal/app/tool/handler/create.go:73`(教学示例) ↔ `internal/app/handler/apply.go:206`(实际解析);function 侧同构(`internal/app/function/apply.go:117`)
- **声称**:`create_handler` 描述里的 OPS 速查表写 `{"op":"set_dependencies", "dependencies":["psycopg2-binary"]}`
- **实际**:`apply.go:204-211` 用 `struct{ Deps []string ` + "`json:\"deps\"`" + ` }` 解析此 op(键是 `deps`),`state.Dependencies = p.Deps`。JSON 反序列化**静默忽略**未知的 `"dependencies"` 键 → `Deps` 为空 → 依赖列表丢失。
  - `json:"dependencies"` 只出现在 HTTP 请求结构体(`handlers/handler.go:59`、`function.go:59`,前端那条路)和 domain 模型(`version.go`)—— 也就是说**全仓只有 op-apply 这一处用 `deps`,是个异类**,而教学示例、HTTP API、领域模型一律用 `dependencies`。框架内部的 env-fix 重试倒是正确用了 `{"deps":...}`(`create.go:146`)。
- **撞墙场景**:agent 按速查表照抄 `{"op":"set_dependencies","dependencies":["psycopg2-binary"]}` → op「成功」但依赖被丢 → venv 装不上包(空 deps 不会触发 env-fix 重试)→ 直到 `call_handler` 时 `import psycopg2` 在运行期 `ModuleNotFoundError`。无创建期报错;agent 再按同一速查表重试仍丢键 → 易陷死循环。凡是需要第三方包的 function/handler 都会中招。
- **修复方向**:把速查表改成 `{"op":"set_dependencies", "deps":[...]}`,或让 apply.go 同时接受 `dependencies`/`deps`(更稳,消除全仓异类)。**小修**。
- **置信度**:high。
- **备注**:子 agent 评 🟡,人工上调到 🔴 —— 理由:这是**静默**数据丢失(比报错更危险)、命中常见「需要 pip 包」场景、且速查表会让重试持续踩坑,符合「承诺的能力根本用不了」。

---

## 🟡 中 severity

### 🟡-A〔影响最大,优先看〕工作流执行引擎已建成并可达,但设计文档/记忆/路线表都说它「未实现 / Phase 4 未启动」

- **类型**:文档-代码 + 文档-文档 + 记忆 漂移(**反向**:文档严重**低报**了已有代码)
- **位置**:`backend-design.md:87`(路线表 `| 4 | 工作流 | 20h | ⬜ |`)、`capability-disclosure-design.md:6/46/320/324`、项目记忆 `workflow-execution-engine-missing`
- **声称**:capability-disclosure §10「`trigger_workflow` 与 workflow 执行引擎(`flowrun` app 层)**当前未实现**,属 Phase 4 独立工程」;记忆「flowrun app + trigger_workflow planned but unbuilt」;路线表 Phase 4 = `⬜`。
- **实际**:`schedulerapp` 是 ~2587 行真实编排。
  - `scheduler.go:101 StartRun` 落 `flowruns` 行并驱动 DAG;14 个 NodeType dispatcher 全部调真实服务(`dispatch_function.go` → `functionapp.RunFunction`、`dispatch_handler.go` → `handlerapp.Call` …);pause/resume(`pause.go` 337 行)、retry(`retry.go`)、`RehydrateOnBoot`(`rehydrate.go`)俱全。
  - **可达**:`POST /api/v1/workflows/{id}:trigger`(`workflow.go:164` → `:170 FireManual` → `trigger.go` → `scheduler.StartRun`),路由在 `router.go` 注册;e2e 测试 `test/scheduler/scheduler_test.go`、`approval_e2e_test.go` 存在。
  - **同一份 `backend-design.md` 自相矛盾**:`:87` 表格说 ⬜,`:115` 正文却写「### Phase 4 — 工作流能力 ✅(已交付,2026-05-13)」,`progress-record.md:23` 亦记 Phase 4 ✅。
  - 唯一真缺口是 `trigger_workflow` **聊天工具**(LLM 入口)未注册 —— 它要调的引擎早已建好。
- **撞墙场景**:这是「功能能 work、但文档误导」故按规则评 🟡 —— 然而**影响是本次审计最严重的**:一份 2026-05-25 写的设计稿正建立在「引擎不存在」的假前提上(把 `trigger_workflow`/catalog 标记当「Phase 4 以后再建引擎」推迟),可能导致有人去「实现 Phase 4」而重造一套已在跑的 ~2587 行引擎,或按「20h」误估工作量。
- **修复方向**:
  1. 改文档 —— `backend-design.md:87` 表格行改 ✅(与本文件 `:115` 对齐)、订正 `capability-disclosure-design.md` §10/§6/§4.6、**更正项目记忆 `workflow-execution-engine-missing`**。
  2. 唯一真缺口可选补:加一个 `trigger_workflow` 薄工具(包一层已存在的 `scheduler.StartRun`/`trigger.FireManual`)。
  3. 文档为小修,补工具为小修。
- **置信度**:high —— 多条代码路径读取 + HTTP 链路验证 + 文档自相矛盾(`:87` vs `:115` vs progress `:23`)+ `trigger_workflow` 缺席全局 grep 取证。

### 🟡-B `make e2e` 编译就过不了 —— 与「~315 测试全绿」基线矛盾,旗舰引擎 e2e 跑不起来

- **类型**:沉默半成品(测试腐烂)
- **位置**:`test/document/workflow_attach_test.go:33`、`test/scheduler/approval_e2e_test.go:16`、`test/workflow/workflow_test.go:138`
- **声称**:`progress-record.md:26`「~315 单元/集成测试全绿」;`CLAUDE.md`「make e2e … 缺 key 优雅 skip」「~170 单测全绿;5 个集成测试因 env skip」。
- **实际**:亲跑 `go vet -tags=pipeline ./test/...` → 编译失败:
  - `th.LocalCtxAs` 现签名 `(string)`,但 3 处调用传 `(*testing.T, string)`(`too many arguments`)。
  - `reqctxpkg.DefaultLocalUserID` 已 `undefined`。
  - pipeline suite **build 失败 = 硬失败**,不是「缺 key 优雅 skip」(skip 只发生在能编译过、再撞 key-gate 的测试)。守护(已确认真实的)执行引擎的那些 e2e 因此跑不了。
  - **注**:非 pipeline 的 `make test-backend` 单测是另一套、仍绿;腐烂仅限 `pipeline` tag 套件。
- **修复方向**:`LocalCtxAs(t,"x")` → `LocalCtxAs("x")`;`reqctxpkg.DefaultLocalUserID` 换成现 harness 的等价物;跑通后重新校准 progress 里的测试数。**小修**。
- **置信度**:high —— 亲自复现,error 原文 + file:line 在手。

### 🟡-C Prompt 里一簇陈旧/错写的工具名(多为重命名后未同步;均有真名替代,撞墙弱)

同一类「prompt 文本引用了不存在或错写的工具名」,因函数调用只允许 `tools` 数组内的工具、且现场都有正确替代,撞墙弱,合并为一条:

- `internal/app/subagent/registry.go:19` —— Explore 的 `AllowedTools` 含 `"search_forges"`(无此工具;真名 `search_function/handler/workflow`)。`filterTools` 按白名单严筛 → Explore 实得 forge 检索能力**为零**(只剩 Read/Glob/Grep)。因 multi-agent step 1 标注「(Optional)」,不致命。
- `internal/app/tool/mcp/search.go:22` —— `searchMCPDescription` 让 agent 调 `call_mcp`,真名 `call_mcp_tool`(与已知 `call.go:41` 的 `search_mcp` 同类简写漂移)。
- `internal/app/subagent/registry.go:18,24` —— Explore/Plan prompt 写「Use Read / Glob / Grep / LS」,无 `LS` 工具(Glob 描述自己就说「covers what a separate LS tool would」)。
- `internal/app/chat/multi_agent_prompt.go:24-28` —— 教 agent「`get_function` … check configState」,但 configState 是 handler 专属概念,function 无此字段(分支永不触发,自解)。
- 已知 2 项一并归此簇:`revert_function` 描述引用 `list_function`(`function/revert.go:20`,旁边给了真工具 `get_function`);`call_mcp_tool` 参数描述引用 `search_mcp`(`mcp/call.go:41`)。
- **修复方向**:逐处改成真名 / 删除。**小修**。**置信度**:high(逐名全局 grep 取证)。

### 🟡-D op 速查表 ↔ apply.go 键名漂移(响亮失败版,与 🔴-2 同根)

- **位置**:`internal/app/tool/workflow/edit.go:34-35` ↔ `internal/app/workflow/apply.go:222,245`
- **声称**:`{"op":"update_edge","id":"<edgeId>",...}` / `{"op":"delete_edge","id":"<edgeId>"}`
- **实际**:apply.go 读 `json:"edgeId"`(不是 `id`);node op 用 `id` 正确,只有 edge op 错。照抄会得 `"empty edgeId"` / `edge "" not found`(响亮报错,error 文案点名 `edgeId`,可自纠)。
- **意义**:这与 🔴-2 的 `set_dependencies` 是**同一根模式**(op 速查表键名 ≠ apply.go 解析键)。建议把所有 op 速查表与 apply.go 做一次键名对账。**小修**。**置信度**:high。

### 🟡-E 契约文档错误码缺失(error-codes.md 自称「全仓一眼索引」却漏多条)

- 6 条已在 errmap 注册却未进文档:`UNAUTH_NO_USER`(401)、`HANDLER_CALL_FAILED`/`HANDLER_INIT_FAILED`/`HANDLER_INSTANCE_CRASHED_INFRA`(422)、`HANDLER_PROTOCOL_ERROR`(500)、`HANDLER_SHUTDOWN_ALREADY`(422)(`errmap.go:52,126-130`)。
- 5 条 handler 内联码未进文档:`ASKAI_NOT_AVAILABLE`(503,**挂在已文档化的 `:iterate`/`:triage` 端点上**,最该补)、`CONTEXT_STATS_UNAVAILABLE`、`MCP_COMMAND_REQUIRED`、`TRACER_DISABLED`、`UNKNOWN_ACTION`(404)。
- `INVALID_CONFIG`(`error-codes.md:329`,标 ✅/400)实际**不在 errTable**,wire 字符串全仓 0 处产出 → 若真到 handler 会落 500 而非 400;且其唯一产出路径(trigger webhook 注册)目前无非测试调用方。
- **修复方向**:补/订正 error-codes.md 对应行(至少 `ASKAI_NOT_AVAILABLE`,因它背书契约端点)。**小修**。**置信度**:high。

### 🟡-F 契约文档事件/路由漂移

- **block type「6」vs 代码「7」**:`events-design.md:25/89/302`、`api-design.md:89/140` 反复说「6 block types」、§9 说「CHECK in 6 值」,但代码枚举 **7** 种(多 `compaction`,`domain/chat/chat.go:90` 的 DB CHECK + `eventlog.go` + `contextmgr/compact.go:63` 真在发)。`events-design.md` §2 表格自己列了 7、概述行却说 6 —— 文档自相矛盾。
- `document` 通知类型在发(`app/document/document.go:471`)但不在 §11.2 实体表;`env_rebuilt` 通知 action 在发(`function/crud.go:357`、`handler/crud.go:389`)但 §11.2 已声明移除该类 action。(通知是开放词表,前端不识别就静默忽略,不破协议。)
- 4 条已注册路由未进 api-design.md:`GET .../export`、`.../llm-trace`、`.../context-stats`、`GET /metrics/tools`。
- **修复方向**:订正文档数字/补行。**小修**。**置信度**:high。

### 🟡-G `POST /api/v1/functions/{id}:resync` 文档仍列,但端点已被删除

- **位置**:`api-design.md:157` ↔ 代码缺席(`functions/{id}` 的 action 只有 run/revert/edit/iterate;`resync` 全仓 0 处)。
- **实际**:`progress-record.md:111` 明记 Plan 03(2026-05-12)「**删 :resync**/env_synced/env_failed」—— 是**有意删除**,但 api-design.md 未同步。调它会 404。重建 venv 能力仍可经 `edit`(`ops=[]` 强制重建 active env)达成。
- **修复方向**:删 api-design.md 该行(§S14 文档滞后)。**小修**。**置信度**:high。
- **备注**:子 agent 评 🔴,人工下调 🟡 —— 因系有意删除 + 能力有替代路径,属纯文档滞后而非能力缺失。

---

## ✅ 复核为「干净」的区域(增强结论可信度)

- **端口接口接线(main.go 枢纽)**:`go build ./...` 通过、`staticcheck ./...` 无 U1000;14 个 NodeType 全有 dispatcher;chat 的 7 个 `Set*`/`Register*` 注入目标均真实非 stub;7 个 `SetRelationSyncer` + 7 个 reader 全被消费;function/handler/workflow/relation 的 Repository 端口零未实现/零 panic-stub;`CatalogSource.InvokeTool()`(设计稿拟新增)确认**未半加入**(接口仍是 Name/Granularity/ListItems)。
- **HTTP 路由层**:~130 条路由全部 trace 到真实 service,无返假数据 stub;`metrics`/`usage`/`context_stats`/`flowrun`/`workflow`/`relation`/`catalog`/`health` 逐个读过均真实;nil-依赖路径返诚实的 `503 *_NOT_AVAILABLE` 而非伪造数据。唯一 ⚪ 级:`dev_routes.go:114` 的 `/dev/routes` 清单残留一条从未注册的 `catalog:refresh`(dev-only,调它诚实 404)。

---

## 最担心的 Top 3

1. **🔴-1 `edit_forge`** —— 平台旗舰「AI 迭代 / Triage」四条流程的唯一核心动作指向不存在的工具(还配错参数名),照字面执行直接卡死。**最该先修。**
2. **🟡-A 执行引擎「低报」(影响最大)** —— 一份 2026-05-25 写的设计稿建立在「工作流引擎不存在」的假前提上,而引擎(~2587 行)其实已建成、可达、有测试;根因是 `backend-design.md` 路线表(`:87`=⬜)与自身正文(`:115`=✅)+ progress(`:23`=✅)+ 项目记忆 自相矛盾。不修会导致**重造已有引擎 / 误估工作量**。
3. **🔴-2 `set_dependencies` 静默丢依赖 + 🟡-B `make e2e` 编译挂掉** —— 前者让「带 pip 包的 function/handler」静默坏掉且按速查表重试持续踩坑;后者使本应守护已确认真实的执行引擎的 e2e 测试**根本跑不起来**,且「~315 全绿」基线失真。

---

## 附:超出本次范围、建议尽快做的动作

项目记忆 `workflow-execution-engine-missing` 已被本审计证伪(见 🟡-A),会误导后续 session,建议择机更正。
