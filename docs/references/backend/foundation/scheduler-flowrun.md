---
id: DOC-013
type: reference
status: active
owner: @weilin
created: 2026-06-11
reviewed: 2026-06-11
review-due: 2026-09-11
audience: [human, ai]
---

# scheduler + flowrun —— durable 执行引擎

## 1. 定位

平台的心脏：**durable workflow 解释器**（`app/scheduler`，纯编排无实体）+ **一次执行的持久化状态**（`domain/flowrun` + 两张 Log 表）。设计原则 #2 的落点：**节点结果记忆化 + 解释器幂等重走**（DBOS/Conductor 式），**非**事件溯源日志（Temporal 式已否决）——没有用户代码可重放，只有图解释器，其全部状态 = "哪些 (节点,轮次) 完成了、result 是什么"。

## 2. 心智模型（懂了这节，引擎代码就是顺着读）

**两张表讲完所有状态**：
- `flowruns`（run 头）= **冻结的拓扑**（`version_id` pin 死图）+ **冻结的引用实体版本**（`pinned_refs`：pin 闭包 `{实体id: active版本id}`）+ 状态。pin 是重放确定性的两把锁——运行中编辑 workflow 或被引用的 function/agent/control/approval 都改不动在途 run（handler 是常驻实例、永远跑 active 类代码；mcp 是无版本的外部 server——两者活态绑定、pin 不约束）。
- `flowrun_nodes`（★真相表）= 每条是一个 `(节点, 轮次)` 的**记忆化 result**。`UNIQUE(flowrun_id, node_id, iteration)`（`idx_frn_once`，D3）是 **record-once 键**：`INSERT OR IGNORE` 语义、首写赢。

**整个引擎是一个幂等函数** `Advance(runID)`：读 frn 行 + 冻结图 → 算哪些 (节点,轮次) ready → 跑/求值 → 写行 → 重复直到无人 ready。**崩溃 = 再调一遍 Advance**：completed 行被"抄"（record-once 拒绝重写）、绝不重跑。没有事件日志、没有 generation、没有 dispatcher 扇出。

**节点行只写终态**（completed/failed/parked，无瞬时 running 行）：action 在一次同步 advance 内跑完，写行前崩溃就整体重跑（**at-least-once**——副作用靠下游幂等，引擎不装 exactly-once）。`parked` 是唯一非终态：approval 挂起；"哪些 run 在等人"从 parked 行**派生**（parked 行即审批收件箱，无投影表）。

## 3. walk —— ready 计算（引擎最核心的算法，`walk.go`）

每轮 advance 在"冻结图 + 最新 frn 行"上重建一个纯派生视图：

1. **seed**：有行的 trigger 节点（run 创建时 trigger 节点行连头一起原子写入——run 绝不无 seed 存在）。
2. **可达 BFS（从已落库决策重推活跃子图）**：前向边**暂时传播**（未决 control 开放所有 port）；**completed 的 control/approval 只放行选中 port 的边**（`edgePruned`）；**回边只在源 completed 且选中该 port 时走、iteration+1**（循环每个真实决策恰进一轮）。**无 skip 信号传播**——活跃子图每轮从 result 里的 `__port`/`decision` 重新推导。
3. **ready 判定（一条规则统一 AND-join 和 simple-merge）**：节点 ready ⟺ 被 reached + 还没行 + **每条 live 入边的源都 completed**（被剪的入边忽略——等它们会让分支汇合死锁；并行扇出则两条都 live、自然 AND-join）。
4. **确定性**：ready 集按节点声明序+轮次排序；`BackEdges`（可归约回边 DFS 判定）与 workflow 校验**用同一个导出纯函数**——系统里"回边"只有一个定义。`MaxIterations=1000` 是失控循环的安全帽（真循环由自身 CEL guard 约束）。

**数据接线（model B）**：节点 `Input` 的每个字段是一条裸 CEL，根 = 祖先**节点 id**（`gate.feedback`）。`scopeFor` 取每个节点"`iteration ≤ 当前轮` 中最大且 completed"的 result——循环内祖先解析到当前轮、循环外到固定 result。control 的 result = 选中分支的 emit 字段**扁平** + 保留键 `__port`；approval = `decision`/`reason`。

## 4. run 生命周期

- **手动**（`StartRun`，HTTP `:trigger`）：读 active 版本 → pin 闭包（`BuildPinClosure`：逐 ref 解析 active 版本，**agent 递归一层**进其挂载的 fn/hd——两层是闭包天然下界）→ 选入口 trigger 节点（显式 entryNode > 按 trg_ > 唯一者；歧义 = `FLOWRUN_INVALID_ENTRY`）→ **单事务**写 run 头 + seed trigger 节点 → Advance。
- **自动**（firing 路径）：见 [trigger.md](../domains/trigger.md)。`consumeFiring` 先 overlap 决策（serial 在途则**留 pending 下个 tick 再试** / skip 标 skipped / 其余跑），读全做在事务外，然后 **`ClaimFiring` 单事务**：claim（仅当仍 pending）+ `SeedRunOnTx` 建 run + 回填 started——崩溃回滚后 firing 仍 pending，**绝无 claimed-但-无-run 残留**（ADR-021）。
- **审批**：approval 节点渲染模板后写 **parked** 行、run 保持 running。人工 `DecideApproval` / 超时 `CheckTimeouts` 都走 `ResolveParkedNode` ——**status='parked' 上的条件更新，first-wins**：人 vs 超时谁先写谁赢，输家 no-op（人工路径上呈 `FLOWRUN_APPROVAL_NOT_PARKED` 422）。超时行为 reject→no / approve→yes / fail→run 失败；timeout 支持 `30d`/`2w` 粗粒度。**这是系统唯一的 durable timer**（5 秒 tick 扫描 parked 行 vs deadline，无定时器持久化）。
- **失败与修复**：节点失败 = 写 failed 行 + run 标 failed（fail-fast，completed 兄弟行留着）。`:replay` = **物理删 failed 行**（Log 表唯一允许的删除——failed 是"非结果"，删它重试不是抹历史）+ run 翻回 running + `replay_count++` + 重走（completed 复用、被清的重跑）。
- **kill**：`KillWorkflow` 对每个 running run **先标 cancelled（守卫 WHERE running）再 cancel ctx**——顺序决定终态正确性：被打断的节点会返 ctx.Err()，若先 cancel，failNode 会把 run 写成 failed；先写 cancelled 则 failNode 的 UPDATE 匹配 0 行 no-op。`trackInflight` 给每次 advance 注册可取消 ctx，使 kill 能打断卡在长 agent 里的 run。
- **崩溃恢复**：boot 时 `Recover` 重走所有 still-running run（`ListRunningRuns` 显式跨 workspace，每个 run 在自己 workspace 的 ctx 里 advance）。
- **run 终态收口**：completed/failed 都过 `markRunTerminal`（store 层 first-wins：守卫 WHERE running，completed 绝不被刷成 cancelled）→ `afterRunSettled` 把 draining workflow 的最后一个 run 结算成 inactive（优雅排空闭环）。

## 5. 后台播种惯例（P3-1 教训，背景工作的铁律）

**后台入口（无请求 ctx）必须逐 workspace 播种**：`bootstrap.forEachWorkspace` 取全局 workspaces 表（无 ,ws 列、裸 ctx 可列），对每个 workspace 以 `reqctx.Detached(wsID)` 重放入口——Boot 的 handler/mcp/ReattachActive + drainLoop 每 5 秒 tick 的 DrainFirings/CheckTimeouts。裸 `context.Background()` 调 ws-scoped 查询会 `MISSING_WORKSPACE_ID`——**自动化链路全死、日志里却像轻微降级**。守护测试 `bootstrap/background_ctx_test.go` 锁死该契约。同族先例：Recover 的 per-run 播种、trigger onReport 的 `Detached(wsID)`、consumeFiring 的按 firing 播种。

## 6. 契约（引用）

- 表：`flowruns` / `flowrun_nodes` → [database.md](../database.md)；ID：`fr_` / `frn_`。两张都是 Log 表（D1 不删；唯一例外 = replay 清 failed 行）。
- 端点：`GET/POST /flowruns` · `GET /flowruns/{id}`（头+全节点行）· `POST /flowruns/{id}:replay` · `GET /flowrun-inbox` · `POST /flowruns/{id}/approvals/{node}:decide` → [api.md](../api.md)。LLM 读取面：`get_flowrun`（同「头+全节点行」）+ `search_flowruns`（住 app/tool/workflow，闭合 trigger_workflow → flowrunId → 检查的环）。
- 码：`FLOWRUN_*` domain 5 + 工具校验 1（`FLOWRUN_ID_REQUIRED`，住 app/tool/workflow）→ [error-codes.md](../error-codes.md)。
- 事件：advance 每节点向 entities 流 workflow scope 发进度 Signal（durable 记录是 frn 行）；终态与挂起走 notifications **唤回环**——failed → `workflow.run_failed` + 点亮 needsAttention（经 LifecycleReconciler.MarkRunAttention，completed 熄灭、cancelled 两不做），approval park → `workflow.approval_pending`（at-least-once）→ [events.md](../events.md)。

## 7. 跨域集成

派发走 4 个窄端口（bootstrap/dispatch.go），签名 `(ctx, ref, pinnedVersionID, input)`：action 按前缀 → fn `RunFunction`（执行 pin 版本）/ hd `Call`（活态绑定，pin 不适用）/ mcp `CallTool`（无版本），agent → `InvokeAgent`（执行 pin 版本；粗粒度 activity——只记忆化最终 result；子步重放是 ADR-010 预留）。派发前调度器把 `flowrunID/nodeID` 注入 ctx（执行实体的审计列就此对账）；pin 版本走显式参数（执行语义非环境身份）。control/approval 由解释器**内联求值**（resolve pin 版本 + CEL first-true-wins / 模板渲染），不是 activity。`OK=false` 转 error fail-fast 使节点行写 failed。
