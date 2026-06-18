---
id: WRK-028
type: working
status: active
owner: @weilin
created: 2026-06-18
reviewed: 2026-06-18
review-due: 2026-09-16
audience: [human, ai]
landed-into:
---

# Iteration Loop —— Finding 索引（一行一条，永不写成 essay）

> **规范（强制）**：一个 finding = **一行**，每格一个短语。证据→轨迹 dump；修法详情→commit；本表只做索引。
> 状态：`open` 待修 · `confirmed` 已复现待修 · `fixed` 已修+验+回归 · `watch` 观察 · `not-bug` 判断后非 bug（成本/性能/可恢复且行为正确——不算）· `dup` 被他条覆盖。
> 新发现追加在表末。**别删行**（同 D1 Log 语义）。

| ID | 状态 | 问题（一句话） | 范围 | 修法（定位） | 验证（前→后） | commit |
|---|---|---|---|---|---|---|
| F1 | fixed | lazy 工具概览不点名 id 参数 → 模型瞎猜参数名（`query`/`function_name`…） | **系统性 49/50** | 地基：`toolset.Overview` 浮出必填参数名 + `prompt` 渲 `name(args)` + preamble id→search 解析 | function+handler 修前 4/4 错 → 修后 4/4 一次对、零 error；79/91 工具现渲参数 | dfe2a361 |
| F2 | not-bug | "resident vs searchable" 措辞被半误读——但 agent 行为本就正确，非 bug | — | — | — | — |
| F3 | not-bug | 简单任务 ~75K input token（冗长 schema 重发）——成本/性能，**非 bug**（作者明示不算） | — | — | — | — |
| F4 | watch | `run_function` 首调 args 平铺非 `{"args":{…}}`（修 F1 后未复现，疑被 F1 一并修掉） | 待 CONFIRM | — | — | — |
| F5 | fixed | 模型用无效字段类型 `"integer"`（schema 只认 number）→ 一次失败调用 + 恢复 | 系统性（`pkg/schema` 所有 build 实体共享） | `ValidateFields` 归一别名（integer/int/float→number, str→string, bool, dict→object, list→array）原地写 + `FromJSONSchema` 同 | 零 token 单测 `TestValidateFields_NormalizesAliases` 绿；make verify 绿 | _pending_ |
| F6 | fixed | edit 带 set_meta 不更新实体行 name/desc/tags（只移版本指针）→ agent 以为改了名、后端没改 | function+handler（workflow 本就对；agent/control/approval 无 set_meta op） | `Edit` 把 draft meta 带回行 + `SaveVersionAndActivate(v, f)` 同事务 Save 整行（6 文件） | `:edit set_meta` 重命名后 GET 真变；零 token 回归 `Test{Function,Handler}_EditPersistsMeta` 绿；make verify 绿 | e356cf2f |
| F7 | fixed | tool 错误对 LLM 不透明：`Error()` 只给 Message、丢 `Details`，而 workflow 校验把违例节点+真实 CEL 错放在 `Details.reason` → agent 见 "workflow graph is invalid" 盲猜 CEL ~8 次卡死 | **系统性**（tool-error→LLM 边界丢所有工具的 Details） | `loop/tools.go` 加 `llmErrText`，在 executeTool 把 Details 渲进 LLM 可见错（一处修全部工具，原则 #8） | 零 token 单测 `TestLLMErrText` 绿；make verify 绿；agent 重跑见详错、自纠建成 workflow、turn completed（前 ERROR） | 0a6c6986 |
| F8 | fixed | workflow CEL 错说 "undeclared reference to 'X'" 但不列**可用**标识符 → agent 试 payload/trigger/celsius/input/receive 5 次才中 | workflow-only（control/approval/trigger 用固定 env payload/ctx/input，无此问题） | `crud.go` compileGraphCEL 首层错附 "this node may read: [祖先节点 id]" | 零 token 回归 `TestWorkflow_InvalidCELListsAvailableNodes` 绿；make verify 绿 | 10c2e343 |
| F10 | fixed | `invoke_agent` 的 `input` 非 required → 概览只显 `invoke_agent(agentId)`，agent 猜 `prompt`（静默丢）→ 空 input 跑出通用问候**却 ok:true**（误导成功）；search_tools 后用对得正解（30C=86F） | invoke_agent | `input` 设 required：概览现显 `invoke_agent(agentId, input)` + ValidateInput 缺失即 `AGENT_INPUT_REQUIRED`（`{}` 允许 self-contained）+ 描述点名「无 prompt 字段」 | 单测 `TestInvokeAgent_RequiresAgentID` 更新（缺 input 报错）；make verify 绿 | 9f0fc39a |
| F9 | not-bug | `get_flowrun` "not found"——查实：模型把 `trigger_workflow` 返回的 id `fr_…b4a` **截成 `fr_…b4`**（漏末位）后端正确报无；用全 id 重试即中。后端正确、模型复制错+恢复 | — | — | — | — |
| F11 | fixed | 嵌入 provider 对 >512-token chunk 报 "input (590 tokens) too large, physical batch 512" → 语义搜索**静默退化 lexical**（仅 INFO 日志，质量降级隐形） | 嵌入 server llama-server 启动 flags（`search/engine/engine.go`） | `engine.go` 加 `--ubatch-size 2048`（=ctx-size，整块一批嵌） | 重启后 ~600-token function 描述索引：embed server 起、"input too large" 计数 **0**（前反复）、不再退 lexical；make verify 绿 | _pending_ |
| F12 | open | chat 回合卡 `streaming` 0-block >5min（`updatedAt`==`createdAt`=无事件到达）。idle-timer(`provider.go:62`,150s)对无事件流应 150s 后 cancel，但 message 未 finalize 到 failed（仍 streaming）→ **疑** idle-cancel 未 finalize message（真 bug）**或** deepseek 对超大 system prompt(ws_dc44 退化)hang | 待聚焦 CONFIRM | 查 LLM 流生命周期 finalize 路径 | — | 触发是退化超大 workspace，**带着思考先不抢修** |
| F13 | fixed | `create_control` 描述说 when/emit CEL 读 `payload`/`ctx`——但运行时只绑 `input`（dispatch.go:159），field schema + 域模型 + docs 三方全说 `input.*`。描述自相矛盾、**主动诱导** agent 写 `payload.temperature`（运行时未声明 → 控制节点必崩）→ 模型为此耗 ~120 行推理纠结 payload/input、终写错 → `TOOL_ERROR_STORM` 中断整轮 | control 工具描述（与自身 field schema + runtime + docs 矛盾） | `tool/control/lifecycle.go` create 描述重写为「读 `input.*` ONLY、payload/ctx 不在域内」+ 点明两层喂入（workflow 节点 input 映射→喂 `input`，branch 再读 `input.*`） | 重建重跑**同任务**：前=TOOL_ERROR_STORM/45 块/写 `payload.*`/未建成；后=completed、写 `input.temperature`、durable 路由 35→`__port:hot`→"hot warning"·10→`__port:normal`→"normal"（flowrun result ground-truth 验） | _pending_ |
| F14 | watch | create/edit 时 `celpkg.Compile` 用共享**宽容 env**（声明 payload+ctx+input 三根）→ control 写 `payload.x` 编译通过、运行时崩（活化只绑 input）；author-time 不做命名空间校验（对比 workflow 节点 input 走 ScopedEnv 按 node-id 收紧、能报清错） | 设计取舍（`pkg/cel` 单一宽容 env、调用方各绑所需根——有意为之且文档化） | — | — | F13 修描述后诱因已除；按命名空间收紧 author-time 校验=改地基设计，留焦点决策、不 drive-by（带着思考） |
| F15 | fixed | approval 节点的下游 result 只是 `{decision, reason}`（**非透传 input**）——但 `create_approval` 描述只讲 yes/no 出口、从不说 result 形状 → agent 把下游 action 接 `human_gate.amount`（approval result 无此键）→ park/decide 续跑后 action 运行时崩、run **failed** | **系统性**（任何 approval→下游需原数据的工作流） | 中央修 `opsDoc`（见 F16）列 approval result=`{decision,reason}` only + `create_approval` 描述补「不透传、下游从上游节点读原数据」 | 重跑同任务：前=action 读 `human_gate.amount` 崩、run failed；后=0 error、读 `start.amount`、approve 后 run **completed**（`record_approved` result ground-truth 验） | _pending_ |
| F16 | fixed | `opsDoc`（create/edit_workflow 共享）说节点 input CEL 读「payload/ctx for a trigger's signal, input for node-fed data」——实际读**上游节点 result 按 node id**（`start.amount`）、无 payload/ctx/input 根 → 误导 agent 写 `payload.x` 节点 input（F13/F15 两轮都先撞此、靠 F8 错误提示才纠） | **系统性**（所有工作流节点接线的总入口文档） | `tool/workflow/workflow.go` opsDoc 重写：明示 input CEL 读 `<nodeId>.<field>`（无 payload/ctx/input 根）+ 加 **NODE RESULT SHAPES** 块（trigger/action/control/approval/agent 各 result 形状） | 同 F15 重跑：agent 一次写对所有节点 input（`start.amount`/`approval_node.reason`）、0 error、run completed；docs 本就对（domains/workflow.md:20、scheduler-flowrun.md:37）故 code-only | _pending_ |

| F17 | not-bug | 多轮改 fn 后 agent 自称「active 仍 v1」——查实：turn-2 `get_function` 在 edit **之前**调（正确返 v1 快照），agent 回看该**前置快照**误判 active=v1；后端实际 edit→v2、run 返 v2 输出、当前 active=v2 全对（F6 激活正常运作）。同 F9 模型时序误读、**非 bug** | — | — | — | 同轮 `LLM_STREAM_ERROR`：THINK 截于半句 = deepseek 流瞬断，后端**正确 finalize 到 error**（status=error/code=`LLM_STREAM_ERROR`）→ 证流错 finalize 路径可用（**区别于 F12 卡 streaming**）；turn-4 对话优雅恢复、agent 正确报 v2 + "Smith, Jane"（无 middle 边界对） |

| F18 | fixed | `entryFuncName` 先 `TrimSpace` 再判 `def ` → 选中**缩进的** def（类方法/嵌套）作入口、与 `validateFinal` 列-0 规则矛盾 → driver 按名调缩进函数 → 运行时 NameError；validateFinal 放行（列-0 def 存在）故 create 期不挡 | 系统性（类/嵌套 def 在入口前；多 agent fanout 之 skeptic 变体复现，driver 写成 `multiply(**_input)`） | `function/validate.go` `entryFuncName` 改判 `HasPrefix(line,"def ")`（不 trim、要求列 0） | 零 token 单测 `TestEntryFuncName_TopLevelOnly`（类方法在前→返列-0 入口）绿；make verify 绿 | _pending_ |
| F19 | fixed | `create_function`/`edit_function` 描述只示 `def main(...)`/`def(**kwargs)`、从不说**第一个顶层 def 即入口**（按 **kwargs 调、名无意义、helper 须置后）→ helper-first 的常规 Python 风格 → run 调错 def、opaque TypeError | 系统性（任何 helper-first 写法；skeptic 变体复现 `_avg() got unexpected kwarg`） | `tool/function/build.go` CreateFunction 描述加 ENTRY POINT 段（首列-0 def 是入口、**kwargs、helper 置后/内嵌） | make verify 绿；同 F18 簇重建再行为验 | _pending_ |
| F20 | fixed | `revert_*` 只移版本指针，但 name/desc/tags **不在版本快照**（在实体行、F6 确立）→ revert 不还原改名 → agent 误报「已改回原名」（result 也不含 name 无从自查）。**F6 同类、在 revert 路径** | **系统性 5 实体**（function/handler/control/approval/agent 的 meta 都在行；workflow meta 在 graph 版本内、不受影响——已核 6 个 Version 结构） | 5 个 revert 工具描述加 honest-contract 句（name/desc/tags 非版本化、revert 不动、用 set_meta 改）。值班判 Option B：meta 是身份非行为快照、不应被 revert 改名——只澄清契约+不改行为 | skeptic 变体确诊（revert 到 v1 后 code 还原但 name 仍 `temp_converter`）；make verify 绿 | _pending_ |

| F21 | fixed | `move_document` 的 position 是**非移位绝对索引**：`Service.Move` 裸赋 `d.Position=*in.Position` 只更一行、不挪同级 → 插入已占用槽（如 0）→ 位置撞 + `ListByParent` 的 `created_at` tiebreak 静默按创建序排 → reorder 静默失败 + 同级 position 重复（实测两兄弟都 pos=1）；且 position **只写**：list/read 工具从不返回、agent 看不到顺序只能盲猜 | 系统性（任何 reorder N 个子文档；skeptic 变体复现 pos=1/pos=1 碰撞） | `app/document/document.go` Move 改稳定按索引插入（取同级 ListByParent、剔自身、splice 到请求 idx、0..N-1 重排、UpdateBatch 原子写）；`tool/document/list.go` slim 加 `position` + 描述 | 零 token 单测 `TestMove_StableReorder`（移末位→0 得 C,A,B、position 0/1/2 无撞）绿；make verify 绿 | _pending_ |

## 元注（一次性，非 finding）
- **为什么这 loop 值得**：F1 那条轨迹 `golden J5` 只断言"版本>1"是绿的；轨迹判官却抓到模型把 `get_function` 调错绕一圈——终态测试瞎、判官看见。
- **workflow + durable 子系统验证通过**（2026-06-18）：F7+F8 修后，agent 建成 workflow（trigger→convert→classify）、`trigger_workflow` 跑通；durable 引擎逐节点记忆化、结果正确（celsius=100 → convert `{fahrenheit:212}` → classify `{label:"hot"}`，三节点 completed）。"整套工程"在此方向确认能转。
- **handler 子系统验证通过**（2026-06-18）：agent 建 counter handler、add(5)→5、add(3)→**8**（resident 进程跨调用保态）、get()→8。有状态常驻"灵魂"正常。
- **control 路由子系统验证通过**（2026-06-18，F13 修后）：agent 建 trigger→control→{hot_label,normal_label} 工作流，durable 引擎 first-true-wins 路由：35→`__port:hot`→"hot warning"、10→`__port:normal`→"normal"（两条 flowrun 各节点 result ground-truth 验）。control 分支结构化路由 + emit 透传在此确认。
- **trigger sensor 自主触发子系统验证通过**（2026-06-18）：agent 一次 0-error 建 handler(level)+sensor(每 5s 探 `read_level`、condition `payload.value>100`、output `{level:payload.value}`)+alert workflow+activate。set level=150 后 sensor 每 5s **自主 fire→spawn flowrun→alert** 节点产 `{alert:true,level:150,message:"Level 150 exceeded threshold of 100"}`（flowrun 节点 result ground-truth 验）。**带着思考纠偏**：firing 记录全 `status:started`、`matched/firedAt` 为 null，初看似「卡住未 finalize」——读 `domain/trigger/firing.go` 知 `started`=「claimed+flowrun created」**即终态-ok**（单 status 枚举、本就无 matched/firedAt 列，jq 取空字段才显 null）→ **非 bug**，自主 fire 全链路正常。差点把非 bug 当 bug，读码纠偏（反自欺铁律）。
- **多轮迭代编辑子系统验证通过**（2026-06-18）：4 轮对话 agent 建 fn v1→改加 middleName/改格式 v2（`edit_function` 改同实体非新建、改前 `get_function` 读态、改后 run 测）→ 边界（无 middle 不留尾空格/句点）正确 → 经一次 deepseek 流瞬断后 turn-4 优雅恢复、正确报 v2。多轮上下文承接 + edit 路径（F6 域）在多轮压力下稳。
- **approval park/resume 子系统验证通过**（2026-06-18，F15/F16 修后）：agent 建 trigger→control→approval→action 退款工作流，trigger(250)→control 路由 `over_100`→approval 节点 **park**（run 续 running）→ 经 `POST /flowruns/{id}/approvals/{node}:decide {decision:yes}` 决策→ run **resume**→ 下游 action 读 `start.amount`(250)+`approval_node.reason`→ 产 "Refund of \$250.00: ✅ Approved by manager"→ run **completed**（全节点 result ground-truth 验）。durable 人在环 park/resume/decide 全链路在此确认。
- **harness 修**：`testend/loop/setup.sh` 固定 workspace 名 "loop" → 重跑 `WORKSPACE_NAME_CONFLICT` → `ws=null` 卡住 loop 自身；改唯一名（`loop-时间戳-RANDOM`）。属"harness 工程"维度。
- 永久回归 test：`selfiter_confirm_f1_*`、`_f1batch_*`（F1）· `Test{Function,Handler}_EditPersistsMeta`（F6）· `TestLLMErrText`（F7）· `TestWorkflow_InvalidCELListsAvailableNodes`（F8）。
