---
id: WRK-029
type: working
status: active
owner: @weilin
created: 2026-06-18
reviewed: 2026-06-18
review-due: 2026-09-16
audience: [human, ai]
landed-into:
---

# Iteration Loop —— 覆盖归档（看测了什么 + 想还有什么）

> **这不是 checklist。** 探索空间无限发散（轴本身也在长），所以这里记的不是"哪个能力测完了"，而是**探过哪些"格子"、各激发了什么结局**——让 frontier（空格/薄格）可读，喂给 EXPLORE 的"想还有什么"。方法论见 [`README.md`](README.md) 的「EXPLORE 引擎」。
> **覆盖的单位 = 结局签名，不是措辞。** 两个探针"算同一个"当且仅当它们在 ground-truth 检查上激发**同一组通过/失败**——换皮不算新。details 进 [`LOG.md`](LOG.md) / 回归 test，本表只做坐标 + 指针。
> **价值是判断、不是机械 diff**（这是 agent 产品）：`promise≠reality` 是最锋利的一面镜子，但**主裁判是 Claude**——从「一个真 agent 在这儿会不会卡 / 被误导 / 白烧 turn / 找不到路 / 恢复不了」holistic 判。镜头列表开放、随时可加。

## §1 描述符轴（松散标签、可生长——不是封闭分类法）

仅用来**保持广度可见 + 让空格现形**，不是给探针套牢笼。任一轴随时可加新值；新值入场即 frontier。

- **target（打哪）**：function · handler · agent · workflow · trigger · control · approval · mcp · document · skill · search · memory · conversation · chat · durable-engine · **ai-ops（:iterate/:triage）** · all others, everything…
- **arity（几方协作）**：单工具 · 多工具组合 · 跨实体 · 多轮迭代 · 并发 · 任何你想到的。
- **regime（什么处境）**：happy · 报错 · 崩溃恢复 · 并发 · 边界/大输入 · 任何。
- **镜头（哪种 agent 之痛——价值轴、Claude 判、开放）**：
  - `promise≠reality`（隐形契约：描述/文档/schema 说 X、运行时做 Y）
  - `假成功`（让 agent 以为成了、其实没——还回 ok:true）
  - `不可发现`（agent 需要的藏着、找不到）
  - `选错工具`（多工具里分不清该用哪个 / 静默用错）
  - `恢复无门`（出错后 agent 回不到正轨）
  - `组合摩擦`（多工具/多实体串不起来）
  - `能力缺口`（合理诉求、却无路径）
  - `白烧`（能做但绕远、耗 turn/推理）
  - `静默降级`（质量悄悄掉、无感知）
  - `脆弱`（agent 的合法使用把产品搞崩）
  - …（新镜头随时加——发明新镜头 = 元新颖，是好事）

## §2 已探（covered cells）

### 已修缺陷（每条都是一个填满的格 + 锁了/待锁回归；details→LOG）
| 探针 | target | arity / regime | 主镜头 | 状态 |
|---|---|---|---|---|
| F1 工具概览不点名 id 参数 | 全 build 工具 | 单工具 / happy | promise≠reality + 选错工具 | fixed·locked |
| F5 无效字段类型 integer | pkg/schema | 单工具 / 边界 | promise≠reality | fixed·locked |
| F6 edit set_meta 不更新行 | function·handler | 多轮迭代 / happy | 假成功 | fixed·locked |
| F7 tool 错误丢 Details 对 LLM 不透明 | 全 tool 边界 | 单工具 / 报错 | 恢复无门 | fixed·locked |
| F8 workflow CEL 错不列可用标识 | workflow | 单工具 / 报错 | 恢复无门 + 不可发现 | fixed·locked |
| F10 invoke_agent input 非必填→空跑还 ok | agent | 单工具 / happy | 假成功 + promise≠reality | fixed |
| F11 嵌入 >512 token 静默退 lexical | search | 单工具 / 大输入 | 静默降级 | fixed |
| F13 control 描述说 payload、运行时绑 input | control | 单工具 / happy→报错 | promise≠reality（主动误导） | fixed |
| F14 author-time 宽容 env 不校命名空间 | control·approval·sensor | 单工具 / 报错 | promise≠reality | fixed |
| F15 approval result 非透传 input | approval | 跨实体 / happy | promise≠reality + 组合摩擦 | fixed |
| F16 节点 input CEL 读 nodeId 非 payload | workflow | 跨实体 / happy | promise≠reality（接线总文档） | fixed |
| F18 入口解析选中缩进 def | function | 单工具 / 边界 | promise≠reality + 假成功 | fixed·locked |
| F19 描述不说首顶层 def 即入口 | function | 单工具 / happy | promise≠reality + 不可发现 | fixed |
| F20 revert 不还原 name/desc/tags | fn·hd·ctl·apf·ag（5） | 多轮迭代 / happy | 假成功 | fixed |
| F21 move_document 非移位绝对索引 | document | 多工具（reorder） / 边界 | promise≠reality + 不可发现 | fixed·locked |
| F22 MCP ref name 形 vs id 形 | mcp→workflow | 跨实体 / 报错 | promise≠reality + 组合摩擦 | fixed |
| W1 cron 无 nextFireAt | trigger(cron) | 单工具 / happy | 不可发现 | fixed·locked |
| W3 runQueue 无 recover→一 turn panic 崩全进程 | chat/runner | 单工具 / 崩溃 | 脆弱 | fixed·locked |
| F24 trigger fire-payload 键隐形 | trigger（3 源）→下游 | 跨实体 / happy | promise≠reality + 不可发现 | fixed |
| F25 fsnotify eventKind 大写 vs 配置小写 | trigger(fsnotify) | 单工具 / happy | promise≠reality | fixed·locked |
| F26 webhook 真 URL 不可发现 | trigger(webhook) | 单工具 / happy | 不可发现 | fixed |
| F27 replay 后端对、但 agent 无工具 | durable-engine | 单工具 / 崩溃恢复 | 能力缺口 | fixed |
| F28 自累加循环 cel-go 缺省根崩 | durable-engine | 多轮迭代（loop） / happy | 能力缺口（+引擎） | fixed·locked |
| F29 buffer_one/all 广告但未实现 | workflow(并发) | 并发 / 并发 | promise≠reality（+引擎） | fixed·locked |
| F30 trigger outputs 自由手填不校验 | trigger | 单工具 / happy | promise≠reality | fixed·locked |
| F31 create_agent 禁 ag_ 无指路 | agent | 跨实体(agent→agent) / happy | 能力缺口/组合摩擦/不可发现 | fixed·locked |
| F32 schema-less 节点输出静默键 .text | 全节点 kind | 跨实体 / happy→报错 | promise≠reality/不可发现/白烧 | fixed·locked |
| F33（=F12）keep-alive 流困死 scanSSELines→message 卡 streaming | chat·llm-stream（共享 ~11 provider） | 单工具 / 崩溃·robustness | 脆弱/恢复无门/假成功 | fixed·locked |
| F34 LLM 流错空消息→message finalize 无因无恢复提示 | chat-loop | 单工具 / 报错 | 恢复无门/静默降级 | fixed |
| F35 capability_check 不查 dataflow→绿检查超额承诺、运行时崩 | workflow(静态校验) | 跨实体 / happy→报错 | promise≠reality/白烧/假成功 | fixed |
| F42 edit_workflow 静默吞无效 concurrency（Create 校 Edit 不校） | workflow | 单工具 / happy | promise≠reality/假成功 | fixed·locked |
| F49 CEL 混类型算术（double+int）裸 `no such overload`、capability 放行、agent 烧 4 版本 | pkg/cel（每条节点 input/条件 eval） | 单工具 / 报错 | promise≠reality/不可发现/白烧 | fixed·locked |
| F36 :iterate 对不存在 id 返 202 而非 404（aispawn 不校、Triage 校的不对称） | ai-ops(aispawn) | 单工具 / happy | promise≠reality/假成功 | fixed·locked |
| F50 空 function 名误报 INVALID_CODE（误导查代码） | function | 单工具 / 边界 | promise≠reality/选错工具 | fixed·locked |
| F54 无搜索后端引导广告不存在的 keyless duckduckgo MCP | web(搜索引导) | 单工具 / happy | promise≠reality/能力缺口 | fixed·locked |
| F60 approval `0s`/零时长 timeout 校验过但永不触发→run 永 park | approval | 单工具 / 报错 | promise≠reality/静默降级 | fixed·locked |
| F47 agent 无工具决策 parked approval（人在环半边不可达）→ 加 `decide_approval` | approval·durable-engine | 跨实体 / happy（人在环） | 能力缺口/组合摩擦 | fixed·locked |
| F63 handler 多行 body 非 flush-left→双重缩进 IndentationError→不透明 crash | handler(assemble) | 单工具 / 报错 | promise≠reality/脆弱 | fixed·locked |
| F66 agent invoke 失败记不透明 "agent loop error"、丢真因（Result 不带 ErrMsg） | agent·loop | 跨实体 / 报错 | 恢复无门/静默降级 | fixed·locked |
| F69 author-time control/approval/sensor CEL 编译错丢真因（裸 sentinel）→ agent 猜 | control·approval·trigger | 单工具 / 报错 | promise≠reality/不可发现 | fixed·locked |
| F70 add_node 顶层误放 input 静默丢弃→节点无接线运行时崩 | workflow(ops) | 单工具 / 报错 | promise≠reality/静默降级 | fixed·locked |
| F64 handler import-time 错（语法/缩进/坏import）不透明 crash→import 移进 init try、走 init_error 带 traceback | handler(driver) | 单工具 / 报错·崩溃 | 恢复无门/不可发现 | fixed·locked |
| F68 agent 无配置工具→grep FS 泄露明文 key+臆造审计→建只读 get_model_config（脱敏） | model·安全 | 跨实体 / happy | 不可发现/能力缺口/安全 | fixed·locked |
| F52 MCP 工具 chat 席不可调（DynamicTools 死码）→ 接 per-request provider 进 search_tools 池 | mcp·chat | 跨实体 / happy | 不可发现/能力缺口 | fixed·locked |
| F74 嵌套 MCP 结果 {text:json} 不进 CEL→mcpResultMap 把 JSON 对象穿成字段 | mcp·workflow | 跨实体 / happy | promise≠reality/组合摩擦 | fixed·locked |
| F83 function 无墙钟 timeout→runaway 钉死 worker→RunFunction 套 FunctionRunSec 外层 ctx deadline | function·durable | 单工具 / 崩溃·并发 | 脆弱/白烧 | fixed·locked |
| F61 并发同父 create_document position 竞态→InsertAtNextPosition 单 tx 原子赋 position（Create+Duplicate 根） | document | 并发 | 组合摩擦/脆弱 | fixed·locked |
| F73 并发 :edit 版本碰撞泄露泛 ORM_CONFLICT→6 域各加 <E>_VERSION_CONFLICT 翻译 | 6 版本化实体 | 并发 | 静默降级 | fixed·locked |
| F80 语义搜索无相关性下限→无匹配 query 灌全 workspace→cosineFloor=0.7（:8743 实测校准） | search | happy/边界 | 静默降级/假成功 | fixed·locked |
| F82 handler 注入 secret 经 call-log 泄露→Instance.SecretValues + recordCall scrubSecrets（防御纵深） | handler | 安全 | 静默降级/安全 | fixed·locked |
| F40 declared outputs advisory→agent invoke 终答回解析(coerce/loud-fail)，fn/hd 保 advisory+文档 | agent·workflow | 跨实体/数据传递 | promise≠reality | fixed(agent半)·locked |
| F57 skill allowed-tools preauth 挂 agent 不生效=对的(无人值守安全)→只改误导措辞(build.go:40+agent.go:79) | agent·skill | 单工具/授权 | promise≠reality | fixed(措辞)·locked |
| F65 sensor level-trigger 风暴被并发策略中和→工具描述+trigger.md 写清节奏(+targetKind 补 mcp) | trigger | 脆弱/白烧 | promise≠reality | fixed(措辞)·locked |
| F41 concurrency=skip 退化疑→对抗复核前提证伪(overlap 信号 DB-durable)、唯同步 Advance niche 吞吐 | workflow·引擎 | 并发 | 系统性→降级 | 评估关闭(非问题) |
| F55 compaction trigger/gate 两尺非对称→刻意+自愈(懒加载压 schema)→不动 | chat·loop | 边界 | 设计议题 | 评估关闭(非问题) |
| F62 search_conversations 跨会话泄露疑→误读(有界片段+工作区隔离、单用户召回即价值)→不动 | search | happy | promise≠reality | 评估关闭(误读) |
| F51 capability_check 校 mcp server 不校 tool→ServerToolNames 灌 RefInfo.MCPToolNames + mcpTool 校验(镜像 handler .method) | workflow·mcp | 不可发现 | promise≠reality | fixed·locked |
| F46 agent 读不回 subagent 子树→get_subagent_trace 工具(列runs+全trace、复用 LoadThread 内存滤 SubagentID) | subagent·messages | 不可发现 | 能力缺口 | fixed·locked |
| F37 agent 读不回上传 attachment→list_attachments+read_attachment 工具(Kind 分流文本/binary、第 11 catalog source) | attachment | 不可发现 | 能力缺口 | fixed·locked |
| F71 capability_check 不校必填 input 接线→Option A「声明即必填」(RefInfo.DeclaredInputs+resolver 灌+check)、不动 schema | workflow·校验 | 跨实体/happy→报错 | promise≠reality/不可发现 | fixed·locked |
| F45 无工作区级坏链接体检工具→运行时 fail-fast 已兜、relation 边硬删无法 cheap 扫→产品设计不做 | workflow·relation | 能力缺口 | 用户判定非问题 | 评估关闭(产品设计) |
| F38 agent 无会话管理工具→编造 compact UI 按钮→manage_conversation 工具(archive/pin·复用 Service.Update)+prompt 声明压缩自动 | conversation·chat | happy | promise≠reality/能力缺口 | fixed·locked |
| F39 todo 全完成后 reminder 抑制+无读回→agent 编造→加常驻 todo_read(含已完成项·复用 ReadRendered)、不动抑制 | todo | happy | 假成功/能力缺口 | fixed·locked |
| F44 构建 turn 0-block+null-error+孤儿实体→两面皆已覆盖(FACE1=F34 填非空因·FACE2=per-tool durable commit 有意)→补 F34 守测 | loop·build | 报错 | 恢复无门 | 评估关闭(非问题·已覆盖) |
| F48 delete 结果不报依赖数+边被同 op 删→地基 CountDependents(入向 equip/link)+8 delete 工具删前读注入(避 Delete 签名级联) | relation·8 实体 delete | happy | 静默降级 | fixed·locked |
| F87 手动 trigger_workflow 绕过并发策略(StartRun 不走 overlapDecision)但工具不说→agent 困惑→描述+workflow.md 点明仅真 fire 受策略 | workflow·trigger | happy | promise≠reality/不可发现 | fixed(措辞)·locked |
| F88 capability_check 不校引用上游不存在的声明 output 字段→**static 校 unsound**(F40 声明 output 对 fn/hd advisory、toResultMap 直通真 dict)·假阳+假阴比诚实不校更糟→运行时 fail-fast+needsAttention 是正解 | workflow·capability | happy→报错 | promise≠reality | 评估关闭(非问题·unsound) |
| F89 LLM tool 错误泄露内部 Go 包.方法路径(`functionapp.RunFunction:`)→llmErrText 基于 sentinel `de.Message`+Details 渲染、非 `err.Error()` 整链（一处净化全工具错误面，忠 S20） | 全 tool 错误边界 | 单工具 / 报错 | promise≠reality/契约卫生 | fixed·locked |
| F90 trigger_workflow payload 不给 per-kind 形状→agent 试错（猜平铺非 webhook `{body:{}}`）→描述列各 kind fire-payload 形状（双 lane CONFIRM） | workflow(手动 run) | 单工具/happy | 不可发现/白烧 | fixed(措辞)·locked |
| F91 list_mcp_marketplace 倾倒全 ~96 server inline 撑爆 context→加 `query` 能力过滤（filterMarketViews 纯函数、向后兼容） | mcp | 单工具/happy | 不可发现/白烧 | fixed·locked |
| F92 handler 方法调用无墙钟（opt-in 默认无界、钉死常驻管道）→加全局 `HandlerCallSec` 默认兜底(call.go methodCallTimeout)+暴露 per-method timeout 旋钮（对称 F83、闭 fn/hd 不对称、function.md 旧 doc-ahead 成真） | handler·durable·systems | 单工具/崩溃·并发 | 脆弱/promise≠reality | fixed·locked |
| F93 LLM 流无总墙钟→idle-timer 每事件重置、病态流永困钉 CPU 25min+阻塞 graceful shutdown+泄漏子进程→加 `LLMStreamMaxSec` 不重置总墙钟（provider.go 第二计时器、区分 total-budget 错） | chat·llm-stream·systems | 单工具/崩溃·robustness | 脆弱/假成功 | fixed·locked |
| F94 fire_trigger 描述诱导传 body 但只发 {manual:true} 丢之→描述点明不带自定义 payload、指向 trigger_workflow | trigger | 单工具/happy | 不可发现/promise≠reality | fixed(措辞)·locked |
| F96 create/edit_agent 不校 skill 存在→dangling 名建 dead-on-arrival agent、只 invoke 才报→Create/Edit eager 校验（同 invoke SkillGuide、新码 AGENT_SKILL_NOT_FOUND） | agent·校验 | 单工具/happy→报错 | 假成功/promise≠reality | fixed·locked |
| F97 F83 函数墙钟封顶但误分类：deadline-SIGKILL→ExitError 先被 errors.As 捕获、status=failed 非 timeout（:triage 不可辨）→ spawn.go 先查 ctx.Err() 返 ErrSpawnTimeout（adapter→run.go 链已对）；真路径测替假信心 fakeRunner | function·sandbox·systems | 单工具/崩溃 | promise≠reality/leaked-internals | fixed·locked |
| F99 create_document 描述假称 >1MB 自动拆子文档（实硬拒 413）→改描述为硬拒+手动拆、document.md 同步（caps 本身全绿） | document | 单工具/边界 | promise≠reality | fixed(措辞)·locked |
| F98 F96 未泛化→knowledge dangling **静默丢弃**(invoke 报 ok·比 DOA 更糟)+tool dangling DOA→BuildKnowledgePrefix 缺 doc 返 AGENT_KNOWLEDGE_NOT_FOUND + Create/Edit eager 校验 knowledge/tool(复用 invoke resolver) | agent·校验·systems | 单工具/happy→报错 | 假成功/静默降级 | fixed·locked |
| F100 chat 回合无 per-turn 墙钟→流/工具卡过每步守卫则 detached ctx 永跑、卡 isGenerating+阻塞 graceful shutdown(SIGTERM 需 SIGKILL)→加 ChatTurnSec 兜底(同 F83/F92/F93 族)；F101=深层不查-ctx busy-loop 待 pprof。**F100 round-5 实测验证：同负载 round-4 钉 180%+阻塞、round-5 全清 0% CPU/0 stuck** | chat·runner·systems | 单工具/崩溃·robustness | 脆弱/假成功 | fixed·locked(F101 open) |
| F105 F97 函数 timeout 持久消息仍是泄露的 "spawn process timeout"(暗示启动失败、误导 :triage)→run.go timeout 分支改"run exceeded Ns wall-clock limit"(镜像 handler RPC timeout) | function·systems | 单工具/崩溃 | promise≠reality/leaked-internals | fixed·locked |
### 已探·无缺陷（绿格——探过、当前行为正确；记下免重挖。details→LOG 元注 0618 + round-1）
| 绿格 | target | regime |
|---|---|---|
| workflow build + durable 逐节点记忆化 | workflow·durable-engine | happy |
| handler 常驻进程跨调用保态（counter） | handler | happy |
| control first-true-wins 路由 + emit 透传 | control | happy |
| sensor 每 5s 自主 fire→spawn flowrun | trigger(sensor) | happy |
| 多轮迭代编辑（v1→edit v2、边界对、流断恢复） | function | 多轮 / 报错恢复 |
| approval park→decide→resume→completed | approval·durable-engine | happy（人在环） |
| fsnotify/webhook/cron 端到端触发 | trigger（3 源） | happy |
| durable replay（清 failed、保记忆化、replayCount++） | durable-engine | 崩溃恢复（HTTP 直验，**非 agent 席**） |
| 结构化累加循环（count 0→1→2→3 + done 终止） | durable-engine | 多轮 loop |
| concurrency serial/skip | workflow(并发) | 并发 |
| concurrency replace/buffer_one（真 webhook fire：replace 取消在途 run·buffer_one defer 留最新 superseded 旧的）+ 手动 :trigger 绕过策略=设计 | workflow(并发)·trigger | 并发（真 fire 席） |
| **kill-9 硬崩溃恢复 from agent 席**（agent 建+trigger parked run→kill -9→重启 run 存活[记忆化+版本 pin]→agent resume 到 completed） | durable-engine | 真崩溃恢复 |
| 广告配置选项 use-time 真生效（agent modelOverride[negative-control 坐实]/挂载 fn/knowledge doc·handler config-update→新 instance 重启非 stale；零 accepted-but-no-op） | agent·handler·model | happy（多选项） |
| code-bug 失败 run 恢复（agent get_flowrun 诊断→edit_function 修→正确选 fresh trigger·知 replay 用旧 pin；replay gotcha 写在工具描述；无 :rerun-with-latest=D1 record-once 有意） | durable-engine·function | 报错→恢复 |
| 多节点 dataflow 契约（order-processing：接线 <nodeId>.field 全对·声明 object output 穿下游 CEL 非 .text 吞·control 两分支两值终态全对·capability_check 缺 output 校但 fail-fast+needsAttention 兜=F88 by-design） | workflow·durable | happy（真实世界） |
| skill allowed-tools 预授权免确认 | skill | happy |
| agent knowledge（doc）挂载 | agent·document | happy |
| MCP 真 server 运行时（echo，name 形接 workflow） | mcp→workflow | happy |
| 失败 run 诊断+恢复（get_flowrun 暴露 failed-node 错+payload、盲诊可行、修后 fresh-run 转绿） | durable-engine·ai-ops(:triage 向) | 报错→崩溃恢复 |
| 多实体组合（agent 同挂 doc-knowledge + function-tool；invoke 时双 mount 各自生效、doc 实时改也反映） | agent·document·function | happy |
| 检索可发现性（相似名里按义挑对，连"重格式化"诱饵也对） | search·function | happy |
| 跨任务记忆（write/read/forget 全可、跨会话真召回、forget 后不臆造） | memory | happy |
| 工具选择可发现性（cron vs fsnotify 按义选对、kind 不可变→delete+recreate、整图合法） | cross-tool | happy |
| agent→agent 嵌套（经 workflow agent 节点真嵌套、结果由 specialist 真输出背书） | agent | 跨实体 |
| :iterate AI-编辑 happy path（edit 真落 DB v1→v2→v3、honest、turn.sh 可驱动） | ai-ops(:iterate) | happy |
| todo_write 机制（持久、逐步状态、整写保历史、真 fsnotify fire） | todo | happy |
| attachment 经 message.attachmentIds 喂 LLM（读真 CSV、零幻觉） | attachment | happy |
| @mention 文档冻结快照注入（注入可见、freeze vs fresh 各对） | document·chat | happy |
| 会话 auto-title 质量 + archive 真实性 | conversation | happy |
| relation 图发现+推理（get_relations 自发现、what-uses-X/反向/传递依赖全对、删影响推理对） | relation | happy |
| subagent 真嵌套 spawn（subagent 执行真跑、parentBlockId 子消息真存） | subagent | happy（读回缺口见 F46） |
| version/revert 语义（v1→v4 编辑、revert 到旧版 + 再编辑、回退码真跑、版本号/active/实跑码全一致） | function | 多轮 / 版本交织 |
| function/handler 名校验对边界/恶意输入健壮（emoji/SQL注入/超长全干净拒、无 500、无脏行） | function·handler | 边界/malformed |
| function 入参类型运行时不强制（鸭子类型、文档化设计、坏类型干净 traceback 不崩） | function | 边界 |
| handler crash/restart 韧性（崩溃丢实例、config-edit 重启、状态语义、agent 能推理） | handler | 报错→崩溃恢复 |
| WebSearch 无后端**诚实降级**（不假装搜过、报可操作信息、转 keyless WebFetch） | web | happy |
| approval **durable timer**（1ms–5s reject-timeout 全正确解析 parked→{decision:no,reason:timeout}+run completed；first-wins race 码验对） | approval·durable-engine | 报错→超时 |
| document 树深操作（嵌套 create 落对父、内容 edit 持久、跨父 move 级联 path、cascade delete 不留孤儿、reorder 连续、环 move 拒） | document | happy |
| 大复杂图（11 节点/13 边、多 control 分支、并行 re-join、durable 全节点记忆化、agent 不丢失） | workflow·durable-engine | happy |
| 多会话隔离（B 见 A 的 workspace 实体+memory、不见 A 的聊天记录；隔离 vs 共享边界正确） | conversation | happy |
| mcp 运行时错路径（分层可操作错、双落 flowrun_nodes+mcp_calls 交叉链、offline→MCP_SERVER_NOT_FOUND、agent 轻松诊断） | mcp→workflow | 报错 |
| 最大实体组合（一 agent 挂 fn+hd+doc+skill 全生效、handler 跨 invoke 保态、doc 实时注入） | agent·多实体 | happy |
| 多分支 control 路由（4+ 分支 first-true-wins 全对、各 input 路由正确） | control | happy |
| approval timeout approve/fail 行为（approve→yes 支 honest{decision:yes,reason:timeout}；fail→整 run failed；补全三件套） | approval·durable-engine | 报错→超时 |
| handler config 工具（update 重启带新 init-args、merge-patch 保键、clear 停实例、重启清内存态=有意 durable） | handler | happy |
| function 运行错透明（自定义异常/ImportError/非JSON返回 真 traceback 逐字穿 run_function+flowrun节点+执行记录） | function | 报错 |
| trigger 失败路径可追（webhook 坏 body、cron/fsnotify 坏配、sensor 探错 各有可追因） | trigger | 报错 |
| 深 durable 循环（25 迭代累加器 + 双体节点循环 per-iteration 全对、按真条件退、远低于 MaxIterations、scopeFor 多体验对） | durable-engine | 多轮 loop |
| **D2 workspace 隔离边界**（跨 ws 读/写/run 全 404/401、无泄露、缺头 401；ORM workspace 过滤兜底） | 安全·全实体 | 边界/恶意 |
| 删被引用实体级联（删 fn→消费 workflow/agent run 干净报 ref-not-found、链可恢复、capability_check 报 dangling、pin 闭包 run 不受影响） | 跨实体·durable | 报错→恢复 |
| 大规模（15-25 节点图 build+run、多 input/output、长内容、版本 cap-50 trim 不丢 active）无截断·腐败 | workflow·全实体 | 大输入/scale |
| create_function 名竞态（DB 唯一索引兜底、并发同名 1×201+N×409 DUPLICATE）· serial-trigger firing 路径串行（单 ticker drain） | function·workflow | 并发 |
| **tool-pick 准确性**（5 相似 fn + 4 相邻 agent：每次选对、无则建新、有完美匹配则复用不重建——零误选/零静默近似/零冗余重建） | 全实体·选错工具镜 | happy（多相似实体） |
| skill 深用（danger gate 恰为 dangerous 调用触发·精确 scoped、body 32KB cap 干净、sequential activate=替换非并、entity-by-name 不建边） | skill | 报错/边界 |
| document 块编辑（markdown-tree 模型、各块型 round-trip 字节精确、单块编辑 siblings 不动、reorder 位置连续、1MB guard 精确 413/201 无截断、200 项大文档不腐） | document | happy/边界 |
| :triage AI 诊断（正确诊断真失败因 + 提可操作修、eagerly 校验 execution 存在无幻影会话、pin-replay 须 fresh trigger 才拾修=有意 pin 语义） | ai-ops(:triage)·durable | 报错→恢复 |
| notification / needsAttention 生命周期（run_failed→点亮、replay completed→熄灭、approval park→pending、completed/cancelled 不误报、workspace 正确 scoped） | notification·SSE | happy/报错 |
| **e2e 系统编排**（一句话目标→agent 搭 webhook→classify(fn)→urgency(control)→approval gate→reply(fn)→log 全系统：选型/dataflow/capability_check/真 webhook POST 三路径全对——除工具描述漏 merge 规则 F76 致首建漏分支汇聚） | 全实体·组合 | happy（真实世界目标） |
| webhook auth（auth 强制 / body-size cap / method gate / dedup idx_trf_dedup 防重放双触发 全按广告——仅 HMAC 验证 header 不可发现 F79） | trigger(webhook)·安全 | 边界/恶意 |
| memory 深用（12 写全召回无 cap、按名 slug upsert 替换不重、长内容完整、矛盾按名去重、forget 真删·无幻影召回、workspace 隔离 401） | memory | happy/边界 |
| conversation 管理（rename 持久不被 auto-title 覆盖、archive/unarchive 正确、soft-delete→404、分页 cursor 0重0漏 recent/pinned-first、usage 与逐条 token 和精确） | conversation | happy/边界 |
| chat 体验深用（cancel→cancelled+partial 存·无 streaming 孤儿、danger confirm/deny=副作用前中断、并发 2nd→409 STREAM_IN_PROGRESS 干净终态、6 轮上下文连贯、空→400 EMPTY_CONTENT） | chat·conversation | 崩溃恢复/并发 |
| i18n/locale（CJK desc/tags/content 零 mojibake、locale 软指令=有意、name 拒 CJK=有意 slug 约束、搜索跨非 ASCII 工作） | 全实体·i18n | 边界 |
| relation 图深用（transitive/reverse/impact 至 depth、diamond 去重不双算、删中链更新边、cycle 处理、14+ 实体规模准确；引擎 BFS edgesSeen/visited 去重） | relation | 边界/规模 |
| replay/durable 恢复深用（record-once 记忆化早节点不重跑、仅失败节点重跑、pinned-version replay、approval 存活、replay_count 幂等、completed run 拒 replay） | durable-engine | 崩溃恢复 |
| handler sensitive config 加密/掩码（config_enc AES-GCM 整 blob、GET/list/versions 不序列化、/config 掩 ********、rotate 重加密、__init__ 收解密值——仅 call-log 投影泄露 F82） | handler·安全 | 边界/安全 |
| **chaos 鲁棒性**（深JSON/病态CEL/unicode·RTL·控制符/mem-bomb/inf-loop/环图/并发dup/SQLi头 全干净降级——零 500·panic·腐败·sandbox逃逸；inf-loop 经 pgroup-SIGKILL 容器化） | 全实体·安全 | 边界/恶意 |
| **第二新域 e2e 编排**（内容审核管线：webhook→toxicity-fn→3分支control→publish-handler/approval/reject-log + cron stats，8 实体三分支全对、agent 自恢复——编排泛化到新域） | 全实体·组合 | happy（真实世界目标） |
| 语义搜索召回+排序（"send mail"→email_dispatcher 真语义命中、best-match-first、per-type scoping 对——**仅无匹配 query 无下限灌全 workspace** F80） | search | happy |

## §3 Frontier（空格 / 薄格——"想还有什么"的起点）

> 这是 backlog，不是 TODO 清单：选哪个由 EXPLORE 的 select 仪式按 novelty × value 判（README）。新轴入场即排这里。

> round-1（0618）填：ai-ops 诊断向、多实体组合、检索可发现、memory、工具选择、agent 嵌套。
> round-2（0618）填：:iterate happy、todo 机制、attachment(attachmentIds 路径)、@mention 注入、auto-title/archive 真实性。
> round-3（0618）填：relation 图推理、subagent 嵌套 spawn（全转绿）；并确诊 F40–F48（F42 已修，F40/F41 HIGH 待 wind-down）。
> round-4（0618）填：version/revert 语义、名校验健壮、入参鸭子类型（全转绿）；确诊 F49（已修）+ F50–F53。F52（chat 调 mcp）= 设计判断。
> round-5（0618）填：handler crash/restart 韧性、WebSearch 诚实降级（绿）；确诊 F54（已修）+ F55–F59。
> round-6（0619）填：approval durable timer、document 树深操作、大复杂图、多会话隔离（全转绿）；确诊 F60（已修）+ F61/F62 + **F47 双 lane 重confirm**（无 decide_approval 工具）。**收敛信号**：真 clean bug 产出率降、not-bug/设计议题升、产品在硬化。
> round-7（0619）填：mcp 运行时错路径、最大实体组合、多分支 control（3/4 lane 全绿）；仅 handler-build 面有料：F63（已修）+ F64（HIGH 队，import 错不透明）+ F65（sensor level-trigger，设计）。**产品高度硬化**。
> round-8（0619）填：approval approve/fail timeout、handler config 工具（全绿）；仅 modelOverride 面有料：F66（HIGH 修，执行记录真因）+ F67/F68（队）。**8 轮收敛**：HIGH 多为透明度族（流错 F33/F34/F66、handler F63/F64）。
> round-9（0619）透明度轴 sweep：function 运行错、trigger 失败路径全绿；ctlerr 逮 F69（author-time CEL 丢因，已修）。透明度轴大体硬化（F7/F8/F33/F34/F35/F49/F63/F66/F69），剩 F64/F67 同族。
> round-10（0619）：深 durable 循环引擎全绿、D2 隔离边界全程守住（无泄露）；逮 F70（add_node 静默丢 input，已修）+ F71（capability dataflow，=F35 深层）+ F72（跨ws messages 200-空 vs 404 一致性，low）。
> round-11（0619）≈收敛完成：删级联 + 大规模 全绿；concur2 仅重确认 F61（仅外部并行客户端触发）+ F73(low)。**本轮零新 clean fix——产品高度硬化**。
> round-补薄格（0620）：两薄格补绿——kill-9 真崩溃恢复 from agent 席 GREEN、concurrency replace/buffer_one 真 webhook fire VERIFIED；逮 F87（手动 trigger 绕策略，已修措辞）。
> round-explore（0620，3 lane 并发真 deepseek）：广告配置选项 use-time 真生效 + code-bug 失败恢复 + 多节点 dataflow 契约 **三格全绿/by-design**；逮 F88（capability_check 不校声明 output——读码判 static 校 unsound、by-design）+ 一模型行为非后端料。**零可修 bug——续高度硬化**。
> **round-1（0621，5 lane 并发真 deepseek）**：skill **创作席全链**（create→activate→edit→rename·活化同会话真改行为·零假成功）· ergo **白烧**（webhook→approval 一轮建对、F13/F15/F16/F24/F26 复验 hold）· rename **by-id 引用跨改名 sound** · tooload **6 相似 fn 选择满分** · ask 澄清/恢复——**四格绿**；逮 **F89**（llmErrText 泄露 Go 包.方法路径，系统性）+ **F90**（trigger_workflow payload 形状双 lane 试错）+ **F91**（list_mcp_marketplace 倾倒 96 server）。
> **round-2（0621，5 lane 并发真 deepseek）**：gdelete **粒度删 node/edge/method 原子整图校验+回滚**（探最毒两 corruption case 均精确拒、零残留）· agnest **agent 嵌套深度 3 + trace 可读**（writer→researcher 真嵌套非编造）· longhaul **17 轮 compaction 强绿**（3 callback 全对 DB、零遗忘/重复）· vision **vision 能力诚实 + F37 读文本**——**绿格丰**；逮**两 HIGH**：**F92**（handler 调用无墙钟·延 F83）+ **F93**（LLM 流无总墙钟→病态流钉 CPU+阻塞 shutdown+孤儿，systems-correctness）+ low **F94**（fire_trigger 丢 payload）+ open **F95**（capability_check 不校 trigger canonical outputs，sound 候选）。**watch**：deepseek 在 attachment+tool-history 投影上病态生成的深层因（BlocksToAssistantLLM 嫌疑、model-specific）。
> **round-3（0621，5 lane 并发真 deepseek）**：stagewf **one-shot 武装→触发→自动解除端到端绿**（边界 WORKFLOW_ALREADY_ACTIVE 清晰）· webfetch **WebFetch 真抓取 example.com 逐字、WebSearch 诚实降级**（零幻觉）· skillagent **装备 skill 指南真注入嵌套 run**（净控对照证因）· errstorm **TOOL_ERROR_STORM 熔断干净终态+跨轮恢复**（F93 挂起类的反例锚点）· resource **F92 handler per-method timeout live 确认(3000→实测 2943 status=timeout)+F83 函数墙钟真封顶+并发隔离**——**绿格丰**；逮 **F96**（create/edit_agent 不校 skill→dead-on-arrival）+ **F97**（F83 函数墙钟封顶但误分类 status=failed 非 timeout）；by-design low：stagewf 2nd-POST-404 model-side 措辞、**webfetch Bash+curl 可达 loopback 绕 WebFetch SSRF 守**（本地单用户威胁模型下可接受、判定非 bug）。
> **round-4（0621，5 lane 并发真 deepseek）**：concstress **常驻 handler 单 mutex 管道 16 并发调用全序列化、state n=1..34 严格连续、okCount 61/0 failed、14 flowrun 全 completed**（系统正确性核心绿）· bigio **三类 cap(tool_result/doc 1MB/log) 全显式信号截断、DB-is-truth**——**绿格丰**；逮 **HIGH F98**（knowledge dangling 静默丢弃·F96 未泛化）+ **HIGH F100**（chat 回合无 per-turn 墙钟→卡 isGenerating+阻塞 shutdown，连带 **F101** open 深层 busy-loop 待 pprof）+ **HIGH F99**（create_document 假拆分契约）。**判断/设计项（记录待定，未 now-fix）**：concstress 手动 :trigger **同步阻塞但返 202 "dispatched"**（durable 引擎下同步可辩护、doc 略偏，medium）· memagent **nested agent 无法用 workspace memory**（mount resolver 只认 fn_/hd_/mcp、system prompt 无 MemoryProvider——能力缺口 vs 产品决策）· iterwf **:iterate steer "Do NOT create_*" 软提示令"加引用新实体的节点"型重构 dead-end**（编辑单实体 scope vs workflow 节点引用他实体的张力，设计）。
> **round-5（0621，5 lane 并发真 deepseek）**：**🎯 F100 实测验证**——同 5-lane 负载下 round-4(无 F100) 钉 180% CPU + SIGTERM 阻塞 shutdown，round-5(含 F100) **全清 0% CPU / 0 stuck conv**。approvaldeep **双 approval 串联端到端全绿**（both decisions threaded、reject-port 路由、has() 守卫）· triagedeep **:triage 区分 crash vs timeout 全绿 + F97 status live 确认**（function_executions.status=timeout）· convmgmt **archive/pin/unpin/compaction 诚实全绿**——**绿格丰**；修 **F105**（F97 timeout 消息泄露 spawn 措辞）；**confirmed backlog**（精确 fix sketch 已记 LOG，排期下次）：**F102** trigger sensor 目标无 eager 校验（校验家族泛化）· **F103** cron @every 契约谎言+对齐隐患 · **F104** flowrun 错误列泄 Go 路径（F89 兄弟）· **F106** auto-unarchive 隐形契约(high)· **F107** manage_conversation 无 rename→UI 臆造(F38 类)。by-design：approval 未接 no-port=有意终端分支、wfnodeval activate 不预检 capability_check=advisory 有意。

**确诊待修 backlog（"想还有什么"已变"该修什么"，= LOG）：**
- **HIGH（wind-down careful 修）：** F40 declared-outputs 静默 no-op（标量返回忽略声明名、落 .text）· F41 concurrency=skip 对阻塞工作流退化成 serial（同步 Advance 蒸发 overlap 信号）。
- **round-2：** F36 :iterate 不校实体存在 · F37 无 attachment 读工具 · F38 无会话管理工具+编造 UI · F39 todo 完成后无读回。
- **round-3 其余：** F44 错 turn 留孤儿实体 · F45 无工作区 health 审计 · F46 无 subagent trace 读 · F48 delete 无守卫+删依赖边。（F43 查实 not-bug——Edit 保留 lifecycle、是 agent 没 :activate 的误读。）
- **round-4：** F51 capability_check 不校 mcp tool 存在(medium) · **F52 chat 不可调 mcp（DynamicTools 死代码）= 设计判断(HIGH)**。（F50 已修）
- **round-5：** F55 compaction trigger/gate 量纲不一致→触发后静默不压(medium) · F57 skill allowed-tools 挂 agent 不授权(medium 待判) · F58 无 intra-loop context 窗守卫(low)。
- **deepseek 没额度时的收尾 pass 清这批（fixing 是代码工不需 deepseek；零 token 回归守）。**

**整列没碰（target 维空白）：**
- **websearch**（toolpick/convo lane 见 workspace 未配 search backend）· **relation 写/删边的 agent 面**（读已绿）。

**薄格（碰过但只在某 regime / 某席位）——两格 0620 补绿：**
- ~~**kill-9 真崩溃恢复 from agent 席**（T3）~~ **GREEN（0620，独立 :8749 + 持久 data dir）**：agent 建+trigger 的 parked durable run 硬扛 `kill -9` 重启（记忆化 + 版本 pin 存活）、agent 崩后 resume 到 completed。见 LOG 元注。
- ~~**concurrency `replace`/`buffer_one` from agent 席**~~ **VERIFIED（0620，真 webhook fire）**：逮 F87（手动 trigger_workflow 绕过策略——已修）后经真 webhook fire 实测——replace 取消在途 run、buffer_one defer 留最新。手动 `:trigger` 路径**设计上不走策略**（仅真 fire 受治）。见 LOG 元注 + F87。

**镜头 / 能力缺口（待判/待探）：**
- ~~**代码-bug 失败 run 无原地恢复**（triage latent）~~ **GREEN/by-design（0620 explore recovery lane）**：replay 按原 pin、改 code 要 fresh trigger，gotcha 写在 `replay_flowrun` 描述里 agent 读得到；无 `:rerun-with-latest` 是 D1 record-once 有意（历史 run 不可变）；agent 实测正确诊断+修+选 fresh trigger 恢复。
- ~~**capability_check 静态 dataflow 校验**（F35 深层）~~ **CLOSED by-design（0620 explore dataflow lane→F88）**：真做静态 node-input 字段校验**unsound**——声明 output 对 fn/hd 是 advisory（F40，`toResultMap` 直通真 dict），按声明 output 校下游引用会假阳+假阴、比诚实不校(F35)更糟；运行时 fail-fast + needsAttention 是正解。input 可校(F71)≠output(advisory) 不对称是有意。
- **无原生 email/通知投递工具**（toolpick lane）：疑产品设计取舍（同 F23 待拍板），非 bug。
- **`白烧` 直接猎**：round-2 wasted lane 撞到 F35（绿检查骗人）；纯 ergonomics（绕远/耗 turn）仍可深挖。
- **跨轴迁移未尽**（沿 pattern 轴扫）：F33「非-data 行跳 ctx」已扫 anthropic（对）；F32/F35 的 schema-less/dataflow 已跨 fn/hd/mcp/agent；「广告选项是否真实现」「数据传递隐形契约」等轴待续。
