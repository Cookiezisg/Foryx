---
id: WRK-016
type: working
status: archived
owner: @weilin
created: 2026-06-12
reviewed: 2026-06-12
review-due: 2026-09-12
expires: 2026-09-12
landed-into: ""
audience: [human, ai]
---

# HANDOFF —— 换 agent 完整接手指南（先读这篇）

> **这是本目录的入口文档。** 你（接手的 agent）按以下顺序读：本篇 → [PLAN.md](PLAN.md)（全计划）→ [findings.md](findings.md)（已发现的 AC-1..AC-20 日志）→ [DECISIONS-PENDING.md](DECISIONS-PENDING.md)（产品裁决台账）。读完即可在 **W5** 无缝续跑，标准不变。本篇负责 PLAN/findings 不承载的那一层：**怎么操作、怎么判、怎么找 bug、坑在哪、下一波具体怎么开**。

---

## 0. 30 秒上手：你在干什么

这是 Forgify 后端的**第四种审查**。前三种（实现正确性 / 设计自洽 / 闭环配对，见 git log 的 `product-review-r*` 提交）全是**读码推演**。本轮不一样——**真开机、真打请求、真跑模型**：

- 用一个独立 Go module `testend/`（**零 backend import**）编译并拉起**真实** `cmd/server` 二进制，像未来前端一样讲**纯 HTTP/SSE**。场景即 `go test`，函数名即验收台账行。
- 每个 feature 的验收单元 = **功能本体 × 情况矩阵（正常/边界/出错/并发/降级）× 涟漪面（创建→可见性涟漪 / 删除→残留涟漪 / 修改→生效涟漪）**。
- **三列判定**：① 用户面（HTTP 真调成功且语义对）② 产品逻辑（状态机/级联/记账全对）③ LLM 面（工具链真驱动得了）。
- 黑盒压出来的 bug **读码审查抓不到**——已证实：`STREAM_IN_PROGRESS` 名义存在物理失效（AC-16）、provider 复用 tool-call id 撞主键整回合丢失（AC-17）、压缩水位线只折叠一半（AC-18），都需要「真 provider / 真落库 / 真流式重叠」才暴露。这就是本轮的价值。

**产出物**（永久资产，不是一次性脚本）：`testend/` 可重跑验收套件（`make testend`）+ 金标套件（`make evals`）+ promptdump 体验审计 + 终报。

**当前进度**：**程序完成（2026-06-13）**——首轮 W0-W8 + 用户裁定重开的 R1-R8 高标准重验全部收口（权威记录：[R-PLAN.md](R-PLAN.md) 波次表 + [TERMINAL-REPORT.md](TERMINAL-REPORT.md) 终报）。最终资产：95 黑盒场景（`make testend`）+ 12 金标旅程（`make evals`，provider=deepseek）。R 轮新增线缆事实（接手必读）：`SubscribeFrom`（fromSeq=0 是 live-only 哨兵、重放 = seq > fromSeq）；流帧 `{seq,scope,id,frame{kind}}`；`parked` 是节点状态（查 run 详情）；agent/api-keys/versions/notifications 列表返裸数组；skill/mcp 投影按 name 键控；InvokeResult status = ok|failed。后续工作（新会话自取）：前端重建对接、AC-20/30 设置页提示。

---

## 1. 七条铁律（违反 = 失格，全部来自用户长期指令 + 项目守则）

1. **中文回复**。代码/路径/英文 commit 半句保持原样。
2. **提交不加 AI 署名**。本仓库 commit **禁止** `Co-Authored-By: Claude` trailer（与全局默认相反——这是本项目的显式覆盖）。
3. **每次提交后立即 `git push origin <branch>`**（当前分支 `acceptance-review`）。投资人可见，不许攒着。
4. **直接在当前分支开发，不开 feature 分支**（共享 worktree）。用**精确 `git add <具体文件>`** 隔离本波改动，**绝不 `git add -A`**（工作区可能有别的 session 的在途改动）。
5. **小问题顺手修，大的产品决策才问用户**。"小" = 契约漂移、漏接线、校验漏洞、文案、护盾缺失这类有唯一正确解的；"大" = 改变产品语义/取舍的（如「env 物化要不要异步」）——记进 [DECISIONS-PENDING.md](DECISIONS-PENDING.md) 等裁决，别擅自定。
6. **先亲验再定性**。每条 finding 必须真机复现过才写进 findings.md，编号 **AC-N**（接着 AC-20 往下），标严重度（🔴 功能不可用/语义错 / 🟡 体验或一致性 / 🟢 轻症 / 🟠 介于）+ 处置（fixed / pending / wontfix（带理由）/ doc-fix / by-design）。
7. **每波收口三件套**：`make verify` 绿 + `make testend` 绿（并发/取消相关再单独 `-race`）+ **文档物理同步**（改了 API/DB/error/SSE → 同提交改对应 reference；见 CLAUDE.md 同步触发表）→ 然后 `git add <精确文件>` + commit + push。文档落后于代码 = 与编译失败同级的 Bug。

> **找 bug 的尺度**（用户原话）："可以不止局限于我们列的要测的点。你发现什么就加到测试计划里，边做边想。make it big，and full coverage。" 即：PLAN 的清单是下限不是上限；顺着真机行为发散，发现什么补什么。

---

## 2. 方法论：怎么判、怎么找 bug

### 2.1 三列判定（每个场景都问这三问）

| 列 | 问题 | 例 |
|---|---|---|
| 用户面 | HTTP 调得通吗？状态码/wire code/envelope 对吗？语义符合 api.md 吗？ | AC-3 文档写 `{input}` 实收 `{args}`；AC-13 文档有路由实际 404 |
| 产品逻辑 | 状态机走对了吗？级联/残留/记账对吗？幂等吗？ | AC-9 并发政策配不出；AC-10 set_meta 静默 no-op；AC-18 压缩只折叠一半 |
| LLM 面 | 工具真进了线缆工具集吗？模型真能驱动吗？tool_result 形状对吗？ | AC-11 nil-input 触发零参函数必崩；AC-17 provider id 撞键丢回合 |

### 2.2 贯穿 bug 模式（**最可迁移的技能——按图索骥**）

到目前为止抓到的 bug 高度同型。新场景写之前，主动按这些模式去"找茬"：

1. **设计完整、接线缺失**（出现 9+ 次，本轮最高产）：domain/store/工具全做了，**只缺一处接线**，于是功能名义存在、物理失效。已证实：AC-9（并发政策无设置口）、AC-10（`ExtractMeta` 零调用 → set_meta no-op）、AC-13（`mcp-calls/{id}` 缺路由）、**AC-21（apikey `RefScanner` 端口 + Delete 循环 + 单测 + 文档全在，但 `AddRefScanner` 生产零调用 → `API_KEY_IN_USE` 永不触发）**；产品审查期还有 limits 空壳 / todo_write / 唤回环 / 活监听重绑 / `GetRegistryEntry`。**修法永远是"接上已有的件"，不是"造新件"**。找法：顺着一条链 store→app→transport→tool 逐跳问"这跳真有调用方吗"——尤其留意"端口定义了、有单测（注入 fake）、有文档承诺，但 boot 从没注册真实现"这种最阴的接线缺失（单测+code review+doc 全绿，唯独线缆断）。
2. **契约名义存在、物理失效**：AC-16 `STREAM_IN_PROGRESS` 注释/文档都说"直接 409"，实际容量 5 channel 静默排队、注释自身两句互斥。找法：凡是"按文档应该报错/拒绝"的路径，真去触发它，别信注释。
3. **provider 线缆习惯触雷**：假模型/真模型按 OpenAI 惯例发的东西打爆后端假设。AC-11（nil→`json.Marshal`→`null`→`f(**None)` TypeError）、AC-17（每步复用 `call_1` → 撞 `message_blocks.id` 主键 → 整回合丢失、行永卡 pending）。找法：让 mock 发"index 风格 id"（`call_1` 每步复用）、发 nil input、发空 content、发多 tool_calls 同批——这些都是 deepseek/qwen 等真 provider 的家常。
4. **不变量只覆盖一半**：AC-18 压缩水位线投影只作用于 assistant 块、user 回合绕过 `unfolded()`。找法：凡有"对称两侧"的逻辑（user/assistant、create/delete、活/休眠），验两侧都覆盖。
5. **生命周期绑错 ctx**：AC-4 `SpawnLongLived` 用 `CommandContext` → 常驻实例绑死在首个请求 ctx 上、请求结束即被杀。找法：凡是"应该活过本次请求"的东西（常驻进程、detached finalize、异步物化），查它的 ctx 来源。
6. **锁顺序把可见状态锁死**：AC-14 `Status()` 与下载抢同一把 `b.mu` → 设置页轮询挂 52.7 秒，"downloading"状态本为这个窗口设计却看不见。找法：长操作（下载/构建）期间去读它的状态接口，量真实 `elapsed_ms`。
7. **协议护盾缺失 / 跨管道竞态**：AC-5（用户 `print()` 污染 JSON-RPC 判死实例）、AC-6（stderr 与 stdout 两管道无顺序保证、print 后到被关门外）。找法：用户态 stdout 会不会串进协议帧？两条独立 goroutine 读的管道有没有跨管道顺序假设？
8. **校验漏洞（校验剧场的反面）**：AC-7（approval `timeoutBehavior` 孤值不校验）、AC-19（`EMPTY_CONTENT` 不 trim、`"   "` 被收）。找法：发垃圾枚举值、发空白串、发只填一半的复合字段。

### 2.3 "只有真机能抓到"的判据

如果一个 bug 单测能抓到，它早被前三轮读码审查抓到了。本轮专攻**单测结构上抓不到的**：
- 单测的 ctx 活到断言之后 → 抓不到 AC-4（实例被请求 ctx 杀）。
- 单测用规整 `{args:{}}` → 抓不到 AC-11（nil input 必崩）。
- 单测的 fake LLM 不复用 tool-call id / 不真落库 / 不真流式 → 抓不到 AC-16/17/18。
- 所以：**让 mock 像真 provider 一样脏**，**让写路径真落库再读回**，**让两个请求真重叠**。

---

## 3. 机械操作手册（命令 / 缓存 / 坑）

### 3.1 命令

```fish
# 全功能黑盒验收（编译真二进制 + 拉起 + 打全场景）。首跑下载 sandbox 运行时，之后走缓存。
make testend            # = cd testend && go test -count=1 -timeout 30m ./scenarios/...

# 只跑一个场景（迭代时）：
cd testend && go test -count=1 -run TestChat_SendStreamToolRoundtrip ./scenarios/ -v

# 并发/取消场景加 -race：
cd testend && go test -race -count=1 -run TestChat_CancelAndStreamConflict ./scenarios/

# pre-push 门禁（gofmt 净 + vet + build + 单测 + 文档门禁）。改了 backend 必跑。
make verify

# 文档门禁单跑（frontmatter / 类型 / INDEX≤50 / 孤儿链接）：
make docs

# 金标真模型（W7 才用，烧钱，需环境变量）：
EVALS_BASE_URL=... EVALS_MODEL=... EVALS_KEY=... make evals
```

### 3.2 运行时缓存（关键，省命）

- 位置：`~/.forgify-testend-cache/sandbox/runtimes/{python,node,uv,llamasrv,embedmodel}`（暖机后 ~640MB）。
- 机制：首跑下载、`saveRuntimeCache` **按 kind 子目录**回存（曾是 all-or-nothing bug，已修——见 W3）；之后每次 `Start` 从缓存 `cp -R` 预置进临时数据目录，不再下载。
- **后果**：RAG（W3）/ MCP npx（W3）/ 任何执行代码的场景，**首跑慢（下载分钟级）、后续秒装**。CI 冷机首跑会久——别误判为挂起。

### 3.3 已知坑（踩过的，别再踩）

1. **cwd 漂移（fish shell）**：Bash 工具的 cwd 在调用间持久。`make testend` 内部 `cd testend`，但你手敲 `go test` / `grep` 时如果 cwd 已在 `testend/`，对 `backend/` 的相对路径会全错。**对策**：`grep`/`go build` 用绝对路径，或每次显式 `cd /Users/SP14921/Documents/Personal/PersonalCodeBase/Forgify/backend`。
2. **macOS `/var` symlink**：`t.TempDir()` 给 `/var/folders/...`，真实是 `/private/var/...`。MCP filesystem server 的 allowed-dir 检查按真实路径——**用 `filepath.EvalSymlinks(t.TempDir())`**（见 mcp_test.go 官方 server 场景）。
3. **gopls / LSP 噪声**：backend module 不在 LSP workspace 里 → 你会看到一堆 import 报错。**那是噪声**，`cd backend && go build ./...` 才是地面真相。别按 LSP 的红线改东西。
4. **DB 行 vs 流的事实源**：assistant 回合的 DB `status` 到 `WriteFinalize` 才从 `pending`/`streaming` 翻终态。想断言"回合在飞"**别轮询 DB 行**（它一直 pending），用 **SSE `WaitFor`** 抓 delta（流是实时事实源）。AC-16 的 Cancel 场景就是这么救活的。
5. **压缩落库 ≠ utility 请求到达**：utility 模型收到摘要请求 ≠ 摘要已写回 conversation。断言压缩生效要**等 `conversation.summary` 非空 + `summaryCoversUpToSeq > 0` 落库**（GET conversation 轮询），不是等 mock 收到请求。
6. **apikey 必须 `:test` 探活**：能力目录来自探测档案。不跑 `POST /api-keys/{id}:test` → 模型窗口未知 → 压缩静默禁用、附件保守渲染（AC-20）。`chatSetup` 已带这步，别删。
7. **mock 队列要 `Clear`**：同一测试里多个子场景共用一个 mock，多排的故障帧（`Status:500`）会毒到下个子场景。子场景间 `mock.Clear(model)`。
8. **`go vet` 对 testend 也要净**：testend 是独立 module，`make verify` 只跑 backend 的 vet——但 testend 自己 `cd testend && go vet ./...` 也该绿，提交前自查。

---

## 4. harness / llmmock API 速查（写场景就靠这套）

全在 `testend/harness/`，**故意薄**——若一个流程在 harness 里需要"体操"，那本身就是 API 的产品 finding，别藏。

### 4.1 server.go — 拉起真二进制

```go
srv := harness.Start(t)        // 编译(一次/run) + 拉起 + 等 health + 注册清理(kill+缓存回存)
srv.BaseURL                    // http://127.0.0.1:<free-port>
srv.DataDir                    // 一次性 t.TempDir，含预置的 sandbox 缓存
srv.Kill9(t)                   // SIGKILL（崩溃恢复的"崩溃"）；DataDir 幸存
srv.Restart(t)                 // 同 DataDir 新端口重启（恢复半场）；BaseURL 变了→重取 client
```

### 4.2 client.go — 带类型黑盒 HTTP

```go
c  := srv.Client(t)            // 无 workspace 身份（用于 /workspaces 本身 + health）
wc := c.WS(wsID)               // 绑 workspace（带 X-Forgify-Workspace-ID 头）

r := wc.GET(path)              // 也有 POST(path,body) / PATCH / PUT / DELETE
r := wc.POST(path, map[string]any{...})

r.OK(t, &v)                    // 断言 2xx + 反序列化 data 进 v（v=nil 丢弃）
r.Fail(t, 404, "CODE")         // 断言 status + wire code 完全一致
r.Field(t, "id")               // 从 data 顶层取 string 字段（快捷取 id）
r.Status / r.Code / r.Msg / r.Data / r.Raw

wc.Try(method, path, body)     // 不让测试失败的版本（崩溃场景 / goroutine 内用，goroutine 禁 Fatalf）

harness.Eventually(t, 5000, "what", func() bool {...})  // 轮询异步涟漪（索引/通知）直到真或超时
```

### 4.3 sse.go — 三条流订阅（E1：messages / entities / notifications）

```go
s := wc.Subscribe(t, "notifications")   // 后台收集每一帧
s.WaitFor(t, 5000, "what", "substr1", "substr2")  // 等"原始帧含全部子串"的事件
s.Never(t, 1000, "what", "substr")      // 否定断言：窗口内没有匹配帧
s.Snapshot()                            // 迄今收集的事件
```

### 4.4 llmmock.go — OpenAI 兼容假模型（W4 进场，柱 B promptdump 同源）

讲真线缆（`POST /chat/completions` SSE + `GET /models`），后端整条 provider HTTP 链全被压到。**按 model id 排独立 FIFO 队列**。

```go
mock := harness.NewLLMMock(t)           // 起 httptest server + 注册清理
mock.URL()                              // 放进 apikey 的 baseUrl

// 脚本下一批 turn（FIFO）。耗尽 → 默认发 {Text:"ok."}（场景在内容失败、不在挂起失败）
mock.Enqueue("gpt-4o", harness.LLMTurn{
    Reasoning: "...", Text: "...",
    ToolCalls: []harness.MockToolCall{{Name: "run_function", Args: map[string]any{
        "summary": "...", "danger": "safe", "execution_group": "...",  // 框架标准字段，像真 LLM 自报
        // ... 工具自身参数
    }}},
    PromptTokens: 300, CompletionTokens: 40,  // 默认 100/10；usage 记账可精确断言
    StallMS: 800,   // 先 flush 半句 text 再 stall（Cancel/在飞场景）
    Status:  500,   // 非零 → 回该 HTTP status + OpenAI error envelope（供应商故障）
})
mock.Clear("gpt-4o")                    // 丢未消费 turn（子场景间防毒）

// PromptDump = 模型在线缆上真看到什么（体验审计事实源）
ds := mock.WaitDumps(t, "gpt-4o", 2, 5000)   // 等某 model 至少 n 个请求
d  := ds[1]
d.System / d.Messages / d.Tools / d.Raw
d.HasMessage("user", "substr")          // 某 role 的消息含子串吗
mock.DumpsFor("mock-utility")           // 发给某 model 的全部请求
```

`GET /models` 返目录已知 id：`gpt-4o` / `mock-dialogue` / `mock-utility` / `mock-agent`（场景仍可用任意 id，但能力探测只认这几个）。SSE 流顺序：`reasoning → text(两片) → tool_calls → finish_reason → usage → [DONE]`。

---

## 5. 写一个 scenario 的范式（照抄 chat_test.go 的骨架）

```go
func TestDomain_Situation(t *testing.T) {
    // 1) 起环境。需要 LLM 面就用 llmmock；纯 CRUD/平台域多数不需要。
    wc, mock := chatSetup(t, true /*withUtility*/)   // chatSetup 见 chat_test.go:27
    // 或纯平台域：
    //   srv := harness.Start(t); c := srv.Client(t)
    //   wsID := c.POST("/api/v1/workspaces", map[string]any{"name":"x"}).Field(t,"id")
    //   wc := c.WS(wsID)

    // 2) 订阅相关 SSE 流（涟漪/通知断言）
    notifs := wc.Subscribe(t, "notifications")

    // 3) 脚本 LLM turn（若涉 LLM 面）
    mock.Enqueue(dlgModel, harness.LLMTurn{ToolCalls: []harness.MockToolCall{{...}}})

    // 4) 真打请求
    convID := convCreate(t, wc, "title")
    msgID  := sendMsg(t, wc, convID, "hi")

    // 5) 三列断言：
    //    用户面 — r.OK/r.Fail/状态码/wire code
    //    产品逻辑 — 轮询落库行 / Eventually 涟漪 / 记账列
    //    LLM 面 — mock.DumpsFor + d.HasMessage（模型真看到了吗）/ d.Tools（工具真进集了吗）
    waitTurn(t, wc, convID, msgID, 8000)             // 见 chat_test.go：按 msg id 轮询（顺序无关）
    notifs.WaitFor(t, 5000, "...", "entity.created")
    ds := mock.WaitDumps(t, dlgModel, 2, 5000)
    if !ds[1].HasMessage("tool", "result-substr") { t.Fatal("...") }
}
```

**铁律**：场景碰到的每个"别扭"（要写一堆 glue 才能调通、返回形状两套、文档对不上）**本身就是前端开发者体验 finding**——记进 findings.md，别在 harness 里悄悄兜掉。harness 薄是有意的。

---

## 6. 进度日志：W0–W4 已完成

完整逐条在 [findings.md](findings.md)（AC-1..AC-20）。这里给**接手摘要**：

| 波 | 范围 | 关键产出 / 大鱼 |
|---|---|---|
| **W0** | 环境+座架 | `harness/{server,client,sse}.go` + smoke（真二进制 6.9s 绿）。AC-1（创建响应嵌套形，by-design 关闭）、AC-2（同步 env 物化，裁决 by-design + 可见性钉死）。 |
| **W1** | 锻造域 function/handler/control/approval | 🔴 **AC-4**（常驻实例绑死请求 ctx，只有真机能抓）；AC-5（print 污染 JSON-RPC 护盾）、AC-6（stderr 窗口竞态）、AC-7（approval 孤值校验）、AC-3（doc-fix `:run` body）、AC-8（HTTP 路径 env 逐行进度地基）。 |
| **W2** | 编排域 workflow/trigger/flowrun | 🔴 **AC-9**（并发政策无设置口）、🔴 **AC-10**（`ExtractMeta` 零调用 set_meta no-op）、🔴 **AC-11**（nil-input 触发零参实体必崩）；AC-12（cron 错误消息指路）。**kill -9 崩溃恢复 PASS**（durable 终极考试）。harness 增 `Kill9`/`Restart`/`Try`。 |
| **W3** | 集成域 MCP+Search | 🟠 **AC-13**（`mcp-calls/{id}` 缺路由）、🟠 **AC-14**（Status() 与下载抢锁挂 52.7s）；AC-15（memory 必填 by-design）。runtime 缓存 all-or-nothing bug 修复。9 新场景：MCP 真装真调（脚本 stdio + 官方 filesystem npx）、Search 全况（8 实体投影 + 中文短词 + FTS 注入安全 + **RAG 真下载真嵌入跨语言命中**）。 |
| **W4** | 对话域 chat 全链 | **llmmock + promptdump 进场**。🔴 **AC-16**（`STREAM_IN_PROGRESS` 名义存在物理失效）、🔴 **AC-17**（provider tool-call id 撞主键整回合丢失行永卡 pending）、🔴 **AC-18**（压缩水位线只折叠 assistant、user 原文永随行）；AC-19（EMPTY_CONTENT 不 trim）、AC-20（能力目录仅来自探测，观察不修）。6 场景：主链+懒工具自动发现、人在环 approve/deny、在飞 409+Cancel、todo+起标题、压缩水位线投影、错误路径。 |
| **W5** | 平台域 A9 + 涟漪矩阵 A10 | 🔴 **AC-21**（apikey `API_KEY_IN_USE` 守卫 `AddRefScanner` 生产零调用→永不触发；修复=boot 注册 workspace+agent 两 scanner）、🟡 **AC-22**（chat `maxSteps` 构造时捕获→`PATCH /limits` 不热换；修复=runner 改实时 `Current()` 读）；AC-23（limits 11 字段全有真消费方，非空壳，by-design）。8 场景：workspace CRUD/校验/activate/最后一个拒删/删除级联、apikey 探活+被引用拒删三来源、model scenarios/caps、limits 热换（含 promptdump 验 tool_result 截断）、notification 已读流转、sandbox 治理、relation equip 边+改名跟随+删除清边。 |

**关键洞见**：W4 三条 🔴 全是读码审查抓不到的——它们需要真 index 风格 provider（AC-17）、真落库的摘要（AC-18）、真流式重叠（AC-16）。W5 AC-21 同理：端口+单测+文档全绿、唯独 boot 没注册真 scanner，只有"真删一个在用 key 拿到 204"才暴露。这验证了整个黑盒+llmmock 路线的价值。

**未决裁决**：DECISIONS-PENDING.md 目前只有 AC-PD-1（同步 env 物化，已关闭 by-design）。新发现的"大决策"往这张表加。

---

## 7. W5 接手指南（下一波，逐步开干）

**范围**：A9 平台域 + A10 跨域涟漪矩阵（PLAN.md 第 78-84 行）。

### 7.1 开工前读这些（reference 是事实源）

- `docs/references/backend/api.md` —— 搜 workspace / api-keys / default-models / limits / sandbox / notifications / relations 各端点（黑盒按文档打，对不上就是 AC-N）。
- `docs/references/backend/domains/support-services.md` —— workspace / apikey / model / limits / sandbox 多半在这（domains/ 下**没有**独立 workspace.md/apikey.md/limits.md）。
- `docs/references/backend/domains/relation.md` —— 关系图（涟漪矩阵的"关系边"列）。
- `docs/references/backend/domains/{search,notification 相关}` —— 涟漪的"搜索索引/通知"列。search.md 已在 W3 验过，notification 看 support-services 或 events.md。
- `docs/references/backend/events.md` —— SSE 三流（E1/E2/E3）的事件全集，A9 的 SSE 子项靠它。
- `docs/references/backend/database.md` —— 删除级联要逐资产验残留，先知道有哪些表带 workspace_id。

### 7.2 建场景文件 `testend/scenarios/platform_test.go`，覆盖 A9：

1. **workspace**：CRUD；**最后一个拒删**（验 wire code）；**删除级联**——建满各实体后删 workspace，逐资产验残留（D1 Log 表保留 vs 业务表软删）。
2. **模型配置**：dialogue/utility/agent 三场景 `PUT default-models/{role}` → 真生效（dialogue 影响主链、utility 影响起标题+压缩、agent 影响 invoke）。utility 缺席的静默降级面再确认一遍（AC-20 家族）。
3. **apikey**：CRUD；`:test` 真探活（成功/坏 key/坏 baseUrl 三态）；**被引用拒删**（default-models 指着它时删 → 拒，验 wire code）。
4. **limits**：每字段 PATCH → 对应行为真变。已知 `triggerRatio`（压缩，W4 用过）；其余字段逐个 PATCH 后真触发对应行为断言"变了"。
5. **sandbox runtimes**：装 / 删 / `disk-usage` / `gc`。注意缓存预置——验 disk-usage 数字真实、gc 真回收。
6. **通知**：全事件真到达（run_failed / approval_pending / env_status_changed 等，前几波已散见）+ 未读计数 + 标已读流转。
7. **SSE 三流**：重连 replay 不丢 durable 帧（E1）、ephemeral 帧（delta/tick，seq=0）不进 buffer 不背压（E2）、messages 流 `parentBlockId` 嵌套（E3，subagent 树）。

### 7.3 A10 涟漪矩阵（收口波，可机械生成）

机械表：`{创建 / 改名 / 删除} × 12 实体 → {搜索索引、关系图、catalog、通知、挂载方、引用方}` 六涟漪面全部如期变化。建议一个 table-driven 测试：每实体一行，建→改名→删，每步 `Eventually` 断言六涟漪面。前几波已零散验过一部分（search 投影 W3、关系边 W1 删除、通知 W2/W3），这里**收口成矩阵**确保无遗漏。

### 7.4 边做边找（按 §2.2 模式）

平台域最可能撞的模式：**设计完整接线缺失**（某 limits 字段 PATCH 收了但没人读 → 又一个空壳）、**校验漏洞**（apikey baseUrl 不校验格式、limits 负值/超界不拒）、**级联不全**（删 workspace 漏清某类资产 → 残留涟漪）。重点验 limits 每个字段是否真有消费方（这是 product-review 已抓过 limits 空壳的复发高危区）。

---

## 8. W6 / W7 / W8 蓝图

### W6 体验静态（柱 B，纯审读不改产品，promptdump 驱动）

资产已就位（llmmock 的 PromptDump 抓包 = 模型线缆所见全部）。要做：
- **六视角 × 六状态 dump 落盘**：视角 = Chat 主 LLM / Subagent / Agent 实体 / Utility / 用户 / 前端开发者；状态 = 空态(自举) / 正常 / 规模态(200 实体) / 降级态 / 崩溃恢复态 / 长程压缩后态。每组合跑一个最小场景、`mock.Dumps()` 落盘审读。
- **横切六刀**：prompt lint（system prompt 有没有废话/矛盾/安全剧场）、tool_result 形状（LLM 收到的结果可读吗）、安全契约体验（danger 自报链顺不顺）、token 成本账单（每视角 promptTokens 量级合理吗）、i18n 接缝、多模型矩阵（后置到 W7）。
- 产出：体验审查报告 + 发现补进 findings.md（多为 🟡 体验类，部分顺手修）。

### W7 金标旅程（柱 C，真模型，`make evals`）

- **此时才需要用户给真模型 key**（deepseek，见 memory `project_llm_research_constraints`：deepseek-v4-flash、¥200 预算）。开工前先问用户拿 `EVALS_BASE_URL/MODEL/KEY`。
- 12 条金标旅程（PLAN.md 第 93-95 行）：①空 workspace 自举 ②建 function 调通看日志 ③建 handler 配 config 调方法 ④搓三节点 workflow 触发到 parked ⑤调试埋雷 function 至修好 ⑥装 MCP 调工具 ⑦search_blocks 找积木接线 ⑧回忆历史对话 ⑨写读 memory ⑩激活 skill 干活 ⑪跨压缩边界长任务 ⑫降级态完成主链路。
- 落 `testend/golden/golden_test.go`（已存在骨架），`EVALS=1` 门控。真模型不确定性 → 断言要松（断"达成了目标状态"，不断"逐字输出"）。

### W8 修复收口 + 终报

- 清 findings.md 所有 pending → fixed/wontfix。
- 终报：一篇 concept 或 working→archive，提炼"黑盒抓到了读码抓不到的 N 类 bug"+ 永久资产清单（testend 套件 / 金标 / promptdump）。
- working 文档落地：结论提取进 concepts/references，填 `landed-into`，移 `archive/`（GOVERNANCE §7）。
- README.md 波次表全 ✅。

---

## 9. 每波收尾清单（声明"本波完成"前逐条勾）

1. ☐ `make verify` 绿（gofmt 净 + vet + build + 单测 + docs 门禁）。
2. ☐ `make testend` 绿（新场景 + 全回归；并发/取消场景另跑 `-race`）。
3. ☐ 改了 backend 契约（API/DB/error/SSE）→ 同提交改了对应 reference（api.md / database.md / error-codes.md / events.md + 对应 domains/*.md）？逐字对得上？
4. ☐ findings.md 补了本波 AC-N（亲验过 + 严重度 + 处置）。
5. ☐ 大决策记进 DECISIONS-PENDING.md（没擅自定产品语义）。
6. ☐ README.md 波次表状态更新。
7. ☐ `git add <精确文件>`（**绝不 -A**）+ commit（**无 Co-Authored-By**）+ **立即 push**。commit message 半中半英、`feat(acceptance-wN): <中文>——<要点>` 风格（照抄 git log）。
8. ☐ testend 自己也 `cd testend && go vet ./...` 净。

---

## 附：本程序的文件清单

- 计划：[PLAN.md](PLAN.md)（柱 A/B/C + A1-A10 + 波次）
- 日志：[findings.md](findings.md)（AC-1..AC-20 逐条亲验）
- 裁决：[DECISIONS-PENDING.md](DECISIONS-PENDING.md)
- 概览：[README.md](README.md)（波次表）
- 套件：`testend/harness/{server,client,sse,llmmock}.go` + `testend/scenarios/*_test.go` + `testend/golden/golden_test.go`
- 门禁：`Makefile`（testend / evals / verify / docs）
</content>
</invoke>
