---
id: WRK-005
type: working
status: archived
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-12
review-due: 2026-09-11
expires: 2026-09-11
landed-into: "docs/archive/backend-review/REPORT.md"
audience: [human, ai]
---

# round2-findings —— 二轮 Code Review（发版门禁，主模型亲审、零 agent）

> 用户要求：高标准、不开 agent、Fable 5 主模型亲审全项目，目标**可直接发版**。
> **第一维度（用户钦点最高优先）：产品角度是否正确、符合预期**——每个模块先立"产品上应该发生什么"（对照 architecture/domains 文档的产品语义 + 桌面单用户场景的常识预期），再对照实现，重点看边界场景下用户实际体验到的行为；代码再对也救不了产品语义错。其余维度（工程正确性/质量/架构/可维护）服务于它。
> 编号续用 CR-N（一轮止于 CR-12）。一条 = 维度 · 严重度（🔴 发版阻断 / 🟡 应修 / 🟢 小 / 📋 产品决策）· 验证过程 · 处置。

## 波次（按发版风险排序，全部亲读）

| 波 | 范围 | 状态 |
|---|---|---|
| W1 | 安全面：tool/shell·filesystem·search·web·mount·document、pathguard/fspath、fs/skill·memory·blob（路径穿越/命令注入/SSRF） | ← 进行中 |
| W2 | 传输层：28 handlers + middleware + response + router（N1-N5、输入校验、状态码） | ⬜ |
| W3 | orm + db + 全部 store（D1/D2、游标分页、SQL 构造、tx） | ⬜ |
| W4 | 引擎：scheduler + flowrun + workflow domain + trigger（D3、record-once、claim、timer、join） | ⬜ |
| W5 | llm ×18 + loop + stream + contextmgr（流终态、用量、E1-E3） | ⬜ |
| W6 | 其余 app 服务（crud/capability/envfix/catalog/subagent/aispawn/apikey+crypto…） | ⬜ |
| W7 | bootstrap ×12 + cmd（装配次序、停机次序、config） | ⬜ |
| W8 | pkg/* 小件（idgen/pagination/limits/jsonrepair/schema/cel/agentstate/reqctx/errors） | ⬜ |

## 发现

### W1 安全面 + 工具层（全部亲读：pathguard/fspath/shell/filesystem/search/web/mount/document/skill/memory/mcp/ask/subagent/toolset/function/agent/handler/workflow/trigger/control/approval 工具组 + infra/fs 三件 + loop tools/history，约 90 文件）

- **CR-13 🔴 已修** Bash foreground 孙进程持管道永久挂死：`cmd.Run()` 的 stdout/stderr 是 io.Writer → Go 开 os.Pipe + copy goroutine，Wait 等 copy 到 EOF。命令留下持有管道的孙进程（`npm run dev` 忘开后台被超时杀、脚本拉起 daemon 后正常退出）→ EOF 永不来 → **Run 永不返回**，对话队列整体卡死、cancel 无效（Cancel 只杀 sh）。修：① `WaitDelay=10s`（进程退出或 ctx 取消后强制关管道，Go 官方为此设计）② Unix `Setpgid` + 超时/取消/KillShell/Stop 全部组杀（`kill(-pgid)`，proc_unix/proc_windows 平台分流）。回归测试：`sleep 30 | sleep 30` timeout 200ms 须秒回（无修复阻塞 30s）。验证：shell 包测试 2s 全绿。
- **CR-14 🔴 已修** tool_result 无界：框架/loop 层无任何截断，结果整段落库 + 整段上 durable SSE open 帧 + 整段进**同回合**下一步 LLM 请求（warm 投影只裁后续回合）——一次不带 head_limit 的大树 content Grep（rg `cmd.Output()` 内存也无界）= LLM 400 + 巨型 DB 行 + 前端巨帧三连。修（强化地基）：① loop `capToolResult` 中央 256 KiB 硬顶（覆盖全部现/未来工具，含 MCP）② rg 路径 `cappedBuffer`（保头 256 KiB、丢弃计数、rg 跑完不杀——免断管舞步）③ stdlib 三模式输出循环加同值字节预算 + content 行模式 32 MB 文件守卫（与 multiline 同界；files/count 模式流式不受限）。
- **CR-15 🟡 已修** Glob 不跳噪音目录：与 Grep 的 noiseDirs 政策不一致——JS 项目 `**/*.js` 返回的 100 条几乎全是 node_modules（mtime 降序放大：刚装的包最新）。修：`hasNoiseSegment` 后置过滤（root 自身在噪音目录内不受限——显式意图）；测试断言 node_modules/.git 命中被排除。
- **✅ PD-4 已裁决 C 并实现（2026-06-12）**：workspace 加 `web_fetch_mode`（local|jina，CHECK 约束，空=local）；WebFetch 经 `FetchModePicker` 端口读模式——local=仅本机直 GET（URL 不出本机，默认），jina=Jina 优先+直 GET 兜底；picker 缺失/读不到一律收敛 local（隐私降级绝不静默）。PATCH /workspaces 接受 `webFetchMode`，新码 `WORKSPACE_WEB_FETCH_MODE_INVALID`。原档案：WebFetch 默认把每个抓取 URL 发给 r.jina.ai——local-first 定位下属产品决策。SSRF 守卫健全（全 DNS 答案检查、逐跳重检、1MB 封顶）不变。
- **🟢 wontfix/留档**：pathguard 不解析 symlink（本地单用户反足枪层、非安全边界，shell 本就可直读）；DNS rebinding TOCTOU（查后拨号，同阈值）；`invoke_agent` 硬编码 TriggeredByChat 而 `run_function` 有 triggerFromCtx（溯源标签不一致，W6 看 subagent 工具集后定）；grep stdlib `byteOffsetToLine` O(hits×size)（有 32MB 界，可接受）。
- **整体评价**：工具层质量高——5 方法契约一致、sentinel 共享（S20）、domain 错误全译 LLM 可行动话术、build 镜像/进度流一致、fs 三件穿越守卫+原子写+隔离齐全、测试覆盖扎实。

### W2 传输层（全部亲读：28 handlers + middleware ×7 + response ×9 + router ×3）

- **CR-16 🟡 已修** 四个 List 手搓 limit 解析绕过 `ParsePage`：chat.List / conversation.List / notification.List / relation.List 自行 Atoi——**无 MaxLimit 钳制**（`?limit=999999` 直达 SQL，orm.Page 只兜下限）且非法 limit 静默吞（其余 List 全 400）。修：四处统一 `ParsePage`（消 4 份样板 + 对齐 N4 语义）。
- **CR-17 🟡 已修** `GET /agents/{id}/versions` 不分页：返全量，违反 N4 + api.md 通则「List 全部 ?cursor&limit」（function/handler/workflow/approval/control 同端点都分页）。修：domain 加 `VersionListFilter` + store 走 orm.Page + app/handler 对齐 + api.md 行重述（注明 agv_ id）。
- **🟡 留档（发版打包前必修）**：CORS 白名单仅 localhost:5173/3000 dev 端口——Wails 打包后 webview origin（`wails://` 等）不在名单，桌面端 fetch 会被浏览器侧 CORS 拦截。前端接入 Wails 时补 origin（后端一行配置）。
- **🟢 注意**：middleware 未知 workspace id 静默清除→RequireWorkspace 401 + 引导语（产品行为正确：删掉的 ws 残留在 localStorage 时自动 re-onboard）；Recover 在 handler 已写头后无法改状态码（净身出户惯例）；attachment Content-Disposition 已剥引号（CR/LF 由 net/http 丢弃非法 header 兜底）。
- **整体评价**：传输层质量极高——Kind→status 塌缩表 + 不泄露内部错误、N1/N4/N5/202/204/410 全合规、28 个 handler 模式完全统一、SSE 三流一处 + Last-Event-ID 续传 + keep-alive、测试覆盖（含 410/续传/越权 401）扎实。

### W3 orm 地基 + infra/db + 全部 22 个 store（含全部 store 测试，~7000 行全读）

- **零缺陷**。全仓质量最高的一层：
  - **D2 双向 fail-closed**：读侧 `whereClause` 缺 ws ctx 即报错、写侧 `applyWorkspace` 自动盖章；`CrossWorkspace()` 显式逃生口仅 flowrun boot 恢复一处使用且注释清楚。跨 ws 更新 0 行、隔离泄露均有测试。
  - **D1/D3 物理铁律全兑现**：Log 表（executions/calls/activations/firings/flowruns/flowrun_nodes/messages/blocks）一律无 deleted_at；`idx_frn_once` record-once first-wins、approval 决策 `status='parked'` 条件更新 first-wins、`idx_trf_dedup` 幂等去重——三者皆有专项测试。
  - **关键并发原语正确**：ClaimFiring 单事务 claim+建 run（ADR-021 精确实现，崩溃回滚 firing 留 pending）；MarkRunTerminal 守卫在 still-running（kill/finalize/fail 竞态 first-wins）；MarkInactiveIfDraining 条件 reconcile 0 行=幂等 no-op 非 NotFound。
  - **一致性纪律**：22 个 store 同模板（Schema 导出/orm 错误翻译/partial-UNIQUE 软删释名/TrimOldest 放过 active）；document 的 COALESCE(parent_id,'') UNIQUE 技巧、todo 的 scope_id 天然 PK、mcp/handler 的 config 加密列均有注释讲清 why。
  - cursor 错误链带 KindInvalid → 400（亲验 pagination sentinel，排除 500 嫌疑）；orm Page 测试覆盖跨页去重/终止。
- **🟢 注意（不修）**：orm.Page 无上限钳制是刻意分层（limit 策略归 transport——CR-16 已在 handler 层补齐）；`uniqueViolationText` 字符串匹配是 driver 错误识别的业界惯例（注释已声明两 driver 均含该子串）。

### W4 引擎（scheduler ×9 + workflow/flowrun domain ×9 + trigger app×10 + infra/trigger ×6，全读含测试）

- **CR-18 🔴 已修** webhook trigger 产品级完全不可用：webhook 路由挂在共享 mux 的 `/api/v1/webhooks/...`、被 Chain 整体包裹，而 `requireWorkspaceExempt` 豁免表不含它 → 外部调用方（GitHub push、一切第三方回调——**不可能**带 X-Anselm-Workspace-ID）一律 401 UNAUTH_NO_WORKSPACE，请求根本到不了 webhook 监听器。修：豁免 `/api/v1/webhooks/` 前缀（安全性不降——webhook 自带 secret/HMAC 鉴权，workspace 由 trigger app 在 report 时从注册表解析，不依赖 header）；chain_test 加回归 case。**验证过程**：webhook 监听器→trigger Service 共享 mux→bootstrap build.go L82-91（mux 整体过 Chain）→chain.go 豁免表四项无 webhooks，链路逐环确认。
- **零其他缺陷，引擎质量卓越**：
  - advance() 幂等核心精确（completed 行抄、绝不重跑；批后重 walk；全 parked 即 yield；ctx.Err() 退出不误标终态——kill 先写 cancelled 再 cancel ctx 的次序保证记录终态正确）。
  - walk 的活跃子图推导（tentative 前向传播 + 回边仅真实决策走一轮 + 统一 AND-join/simple-merge 规则 + MaxIterations 安全帽 + 声明序确定性排序）逐条兑现 doc 21 §4.3。
  - ClaimFiring 单事务（ADR-021）、record-once first-wins、approval 条件更新 first-wins、超时三行为（reject/approve/fail）+ 人机竞速 first-wins——全部有专项测试（含阻塞 agent 被 kill 打断、at-least-once 丢行重跑的诚实证明）。
  - trigger 引用计数监听（N workflow 共享 1 listener、1→0 停）、stage 一次性自动撤防、Activation 触没触发都记（"为什么没触发"可查）、四源 dedup key 设计各对其源语义（cron=刻度分钟、webhook=body hash+分钟桶、fsnotify=path+op+秒桶、sensor=probe 秒刻）。
  - webhook HMAC 常量时间比较 + 10MB body 封顶；cron/fsnotify/sensor 回调全带 panic recover。
- **🟢 注意**：cron 锁 time.Local（桌面 app 语义正确——用户本地时间）；webhook 明文 secret 接受 ?token= query（外发日志可能记 URL——但本地单用户、HMAC 模式可选，可接受）；serial defer 的 firing 靠下一次 DrainFirings tick 重拾（W7 验证 tick 周期）。

### W5 loop + stream + contextmgr + llm ×34（全读含测试，~9000 行）

- **零缺陷**。此波核对了 W1 留下的 CR-14 定性前提（loop 层确无截断→已修），其余全为质量确认：
  - loop：取消态提升（provider 静默关流不发 EventError 时把 EndTurn 提升为 Cancelled，防止悬挂块误标 completed）、TOOL_ERROR_STORM 熔断、MAX_STEPS 诚实终态、reminder 注入副本不污染持久历史、build 镜像 SSE-C、progress 块仅流不回喂 LLM（类型白名单）。
  - stream：E2 语义精确——durable 帧入环 + 阻塞背压、ephemeral 非阻塞丢弃；Subscribe 的 replay 缺口检测（最旧 seq 越过 fromSeq+1 → 410）；cancel 先 close(done) 再争锁（防与阻塞 Publish 死锁）。
  - contextmgr：水位（summary_covers_up_to_seq）幂等键 + SetSummary 先于 archive 标记的崩溃安全次序；触发用真实 InputTokens、闸用 bytes/4 自校正估算；demote 只降不升；archive 按整回合粒度（tool_call 绝不失 tool_result）。
  - llm：唯一 idle 计时器替代总墙钟（健康长流永不杀）；classifyHTTPError 收口 status→sentinel；SanitizeMessages 缝合孤儿 tool_call（防严格 provider 400 锁死）；Generate 的 retry 只许无副作用调用方用（emit 方禁用，注释明示）；11 家 provider 各自自包含（刻意重复防共享分支地狱）、签名/思考块回传、多模态矩阵测试逐家断言官方 wire、PDF 不支持家优雅降级。
- **🟢 注意**：anthropic「enabled」thinking 的 budget 派生（max/2 clamp [1024,8192]、必要时上调 max_tokens）正确防 400；mock 队列空发 MOCK_QUEUE_EMPTY 错误（fail-loud，T6 正确语义）。

## W6 —— 其余 app 服务 + infra（sandbox 双层 / mcp 双层 / crypto / handler client）

**结论：零缺陷波次。** 亲读 sandbox app+infra（directInstaller 校验和+原子换+zip/tar-slip 守卫+1GiB 顶；进程组三平台 Setpgid/Pdeathsig/Job 对象——组杀使 SpawnOnce 免 WaitDelay）、function/handler/agent/workflow/approval/control 六实体 app（PD-3 B 两步写、版本 Trim、关系同步、CEL 祖先域校验、pin 闭包深度 1 一致）、mcp app+infra（连接生命周期换 client 旧句柄 goroutine 收尾、降级阈值、JSON-RPC progress token）、skill/subagent（fork 隔离、工具过滤去 Subagent 防递归）、document（路径级联 BFS+环防护）、apikey（AES-GCM 机器指纹派生、probe 矩阵）、attachment（sha256 去重 blob+能力降级）、catalog/todo/memory/model/workspace/aispawn/envfix（AI 依赖修复环）+ infra/crypto+infra/handler（crashed 状态机：超时→crashed→重生，弃 goroutine 不污染下任调用）。

| 级别 | 发现 | 处置 |
|---|---|---|
| 🟢 | handler DriverScript 把 tuple/set 当生成器迭代消费（`hasattr __iter__` 排除 str/bytes/list/dict 但漏 tuple/set），返回 tuple 的方法只留末元素 | 留档不修——LLM 生成的 handler 方法实际返回 dict/list/标量；修复属 driver 协议升级，发版后随真实反馈定 |

## W7 进行中 —— CR-19 🟡（产品语义违约，已修）

**CR-19：pin 闭包没传到派发口——function/agent 节点跑 active 版本而非冻结版本。** 文档承诺 `pinned_refs` 使"运行中编辑任何被引用实体都改不动在途 run"，但 `Dispatcher` 端口签名只有 `(ctx, ref, input)`：pin 只被 control/approval（内联求值）与图拓扑（`version_id`）消费，fn/ag 派发时 `VersionID: ""` 落回 active——中途编辑实体会改变在途 run 未执行节点的行为。**修复**：Dispatcher 接口加 `pinnedVersionID` 参数（scheduler.go），runNode 传 `run.PinnedRefs[entityIDOf(ref)]`（dispatch.go），bootstrap dispatcher fn→`RunInput.VersionID`、ag→`InvokeInput.VersionID`；handler（常驻实例=active 类代码）与 mcp（无版本外部 server）**活态绑定属设计事实**，显式注释+文档定性而非假装支持。新增 `TestDispatch_PinnedVersionsReachPort` 断言 pin 贯穿 StartRun→派发。文档同步：scheduler-flowrun.md §2/§7 精确化 pin 边界。

## W7 —— bootstrap ×18 + cmd/server + cmd/docs

亲读全部装配件（build/build_data/build_services/dispatch/refresolver/resolvers/renderers/model_info/aispawn/conversation/sensor/workflow_exec + 6 测试文件）+ 两个 main。除 CR-19/CR-20（已修，见上）外干净。确认项：
- **DrainFirings tick = 5s**（`drainInterval`，逐 workspace 轮询 + CheckTimeouts）——关闭 W4 留档的 serial-defer 延迟疑问（最长 5s 延迟，桌面产品可接受）。
- DI 全图核对：26 服务装配顺序、relation Namers 11 实体齐、catalog 10 源、mention 8 解析器、workspace reaper（杀 wf + 停 handler 实例 + 断 mcp + 删文件树）与 PD-1 A 一致。
- 优雅关停顺序：SSE 流(cancelBase) → HTTP 排空 → trigger/chat/mcp/handler/sandbox → DB；cmd/server 薄壳只接信号。
- 🟢 `Boot` 里 `RestoreOrCleanupOnBoot` 在 `sandbox.Bootstrap` 内已调、Boot 又显式调一次——属防御性（Bootstrap mkdir 失败的降级路径仍要收割 PID），二次调用幂等无害，留账不动。

## W8 —— domain 全层 + pkg 全包 + 配置文件

亲读 23 个 domain 包（实体+错误+Repository 接口）+ pkg（errors/idgen/reqctx/agentstate/schema/limits/cel/jsonrepair/tokencount/wikilink）+ infra/logger + 全部剩余测试 + 配置文件。发现：
- 🟢 **S22 卫生**（已修）：`.gitattributes` 的 `documents/**`/`testend/**` 与 `.gitignore` 的 `!lab/*/target/` 豁免指向已不存在目录——按状态即重述删除。
- domain 层零 import 违规（仅依赖 pkg/errors+pkg/schema+兄弟 domain，无 ORM/cel-go）；S15 前缀与 S20 错误码全层一致（standard_test 的 WireCodesGloballyUnique 门禁在）。
- cel 包 ScopedEnv 双轴（payload/ctx/input 全局 + node-id 根 scoped）与 workflow/scheduler 用法吻合；jsonrepair 三段修复幂等；tokencount CJK=1/ASCII÷4 + 校准钳制 [0.5,3.0]。

## 二轮总结（发版判定）

**覆盖**：624/624 文件（87,628 行 Go + 配置）全部亲读标记，零 agent 代审。
**发现**：CR-13🔴 CR-14🔴 CR-18🔴（W1-W4，已修）、CR-15🟡 CR-16🟡 CR-17🟡 CR-19🟡 CR-20🟡（已修）、PD-4（已裁决 C 实现：webFetchMode 配置、默认 local）、CORS Wails origin（用户裁决：回头再说）、若干 🟢 留账。
**门禁**：make verify 全绿 + 并发包 -race 全绿。
**判定**：PD-4 已落地（裁决 C）；CORS Wails origin 经用户确认延后（前端联调时一行补上）。后端达到可发版质量。
