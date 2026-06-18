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
| skill allowed-tools 预授权免确认 | skill | happy |
| agent knowledge（doc）挂载 | agent·document | happy |
| MCP 真 server 运行时（echo，name 形接 workflow） | mcp→workflow | happy |
| 失败 run 诊断+恢复（get_flowrun 暴露 failed-node 错+payload、盲诊可行、修后 fresh-run 转绿） | durable-engine·ai-ops(:triage 向) | 报错→崩溃恢复 |
| 多实体组合（agent 同挂 doc-knowledge + function-tool；invoke 时双 mount 各自生效、doc 实时改也反映） | agent·document·function | happy |
| 检索可发现性（相似名里按义挑对，连"重格式化"诱饵也对） | search·function | happy |
| 跨任务记忆（write/read/forget 全可、跨会话真召回、forget 后不臆造） | memory | happy |
| 工具选择可发现性（cron vs fsnotify 按义选对、kind 不可变→delete+recreate、整图合法） | cross-tool | happy |
| agent→agent 嵌套（经 workflow agent 节点真嵌套、结果由 specialist 真输出背书） | agent | 跨实体 |

## §3 Frontier（空格 / 薄格——"想还有什么"的起点）

> 这是 backlog，不是 TODO 清单：选哪个由 EXPLORE 的 select 仪式按 novelty × value 判（README）。新轴入场即排这里。

> round-1（0618）填掉了：ai-ops 诊断向、多实体组合、检索可发现、memory、工具选择、agent 嵌套（全转绿，移上 §2）。

**整列没碰（target 维空白）：**
- **ai-ops `:iterate`（AI 对话式编辑实体）** —— round-1 只探了 :triage 诊断向（已绿）；`:iterate` 仍空。
- **conversation 管理**（archive/compact/auto-title）· **attachment** 上传/取/content · **todo** · **relation 图** · **subagent 嵌套树渲染**（parentBlockId）。

**薄格（碰过但只在某 regime / 某席位）：**
- **kill-9 真崩溃恢复 from agent 席**（T3）：round-1 triage lane 验了"诊断+修+fresh-run 恢复"（绿），但**真杀进程→重启→resume** 仍未从 agent 席验（故意未入并发轮——会连累共享后端，留**串行单跑**）。
- **concurrency `replace`/`buffer_one` from agent 席**：F29 仅单测；agent 多轮真触发顶替/收敛没验。
- **@mention 聊天注入冻结快照**：compose lane 验了 doc 作 agent knowledge 挂载（绿）；聊天里 @mention 的冻结快照语义没碰。

**镜头 / 能力缺口（round-1 新冒出，待判/待探）：**
- **代码-bug 失败 run 无原地恢复**（triage latent）：replay 按原 pin 重走、改 code 要 fresh trigger（新 id）；按 id 跟踪的用户无 `:rerun-with-latest`。待判是否值得做。
- **capability_check 不校 schema-less 节点的 input-CEL 字段引用**（F32 深层）：doc 已补、但 check 仍放行坏 key、第一次跑才崩——可加 check 期校验（比 doc 更硬）。
- **无原生 email/通知投递工具**（toolpick lane）：'email me' 无一等路径、要手搓 SMTP——疑产品设计取舍（同 F23 待你拍板），非 bug。
- **`白烧` 直接猎**：仍没**直接**以"能做但绕远/耗 turn"为目标探。
- **跨轴迁移未尽**（沿 pattern 轴扫）：F33 的"非-data 行跳过 ctx"已扫到 anthropic（它对的）；F32 的 schema-less `.text` 已跨 fn/hd/mcp/agent；「广告选项是否真实现」「数据传递点隐形契约」等轴待续。
