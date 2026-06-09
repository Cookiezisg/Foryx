# Round 0058 — subagent（波次 5 · M5.2+ 递归子对话）

类型 / 目标：建 **subagent**——LLM 用 `Subagent` 工具派一个隔离的子 agent 跑一段任务、同步拿回结果。「**≈ 递归的 chat**：无自己的表（写父对话 messages、SubagentID 标记）、承袭父 effective model、E3 嵌在派它的 tool_call 下」。消费者 = skill fork（`SubagentRunner` 端口）+ `tool/subagent`（Task 工具，LLM 直接派）。**chat 模块收官后的递归能力**。

依赖扫描（三路 Explore 考察 2026-06-09 + 源码核实）：
- **消费契约**：`skilldomain.SubagentRunner.Spawn(ctx, agentType, prompt) (result string, err error)`（domain/skill/skill.go:123）——同步跑完返最终文本。skill fork（activate.go:49）+ Task 工具共用。
- **host 范式**：**混血**——LoadHistory=任务 prompt（像 agentHost）/ Tools=静态白名单（像 agentHost）/ WriteFinalize=落 sub-message + message_stop（像 chatHost）。比纯 agentHost 多「落盘 + SSE」（因选了持久化 B）。
- **reqctx**：`SubagentID` 种子已埋（R0029，防递归 + todo 作用域）；`ToolCallID` **需新增**（loop 当前不把 tc.ID 给 Execute，见下）。reqctx **不依赖 domain**（只 context+errors）——故 model override 不放 reqctx。
- **持久化（用户拍板 B）**：sub-message 落父对话 messages 表、带 `SubagentID` 标记；blocks 经 E3 嵌父 tool_call；chat LoadHistory 过滤 `SubagentID!=""`（父只见 tool_call + 最终结果作 tool_result，不污染）；ListMessages 返回它们（刷新可重建 subagent 树）。
- **考古**：旧 `app/subagent`（spawn/host/registry）+ `tool/subagent` + DOC-123。**必留**：Spawn 同步派发、registry 3 类型、Task 工具形状、parentBlock 锚点、防递归。**必废**（DOC-123 腐烂）：cv_xxx 子对话（虚构，实写父对话）、深度限 2（实为 1）、AgentState 沙箱（虚构）、4 个虚构错误码。

设计要点：

1. **持久化（B）**：`domain/messages` Message += `SubagentID string`（db `subagent_id`，""=顶层回合）。sub-message 落父对话（role=assistant、SubagentID=run id、`Attrs["parentBlockId"]`=派它的 tool_call id 供前端重建树）。chat `LoadHistory` 加 `if m.SubagentID != "" { continue }`（排除 LLM 历史）。LoadThread/ListMessages 不变（返全量，REST/reload 用）。store DDL 加列。**spawn 的最终输出 = 派它的 tool_call 的 tool_result**（父 LLM 只见这个，由 Task 工具 Execute 返回串、loop 落成 tool_result）。

2. **E3 流嵌套（live/reload 一致）**：subagent 发**自己的 message 节点**：message_start=`Open{type:"message", ParentID:<spawning tool_call id>}` → 挂 tool_call 下；blocks 经 loop（`reqctx.SetMessageID(subMsgID)`）挂 sub-message 下；message_stop=`Close`。同一 Bridge（subagent 跑在 Task 工具 Execute 的 ctx 内，已带父 loop 的 `WithBridge` + 同 conversation scope）。

3. **loop ToolCallID 种子**（gap）：`loop/tools.go` `runOneTool` 执行工具前 `ctx = reqctxpkg.SetToolCallID(ctx, tc.ID)`，使 Task 工具 Execute 知道自己的 tool_call id（→ 传 Spawn 作 E3 锚）。reqctx 加 `SetToolCallID`/`GetToolCallID`（纯 string，无 domain）。

4. **model 承袭**：subagent 自有 `ModelResolver.Resolve(ctx) (Bundle, error)`（Bundle{Client,Request,Provider} 自包含、不引 chatapp）；M7 适配器 = `model.Resolve(ScenarioDialogue, nil, picker)→creds→factory.Build`（workspace dialogue 默认 = 父常见无 override 时的 effective model）。**真·conv.ModelOverride 承袭延后**（跨 pkg→domain 层成本不值，单用户 override 罕见）。

5. **registry 3 类型**（硬编码，照旧）：**Explore**（侦察：Read/Glob/Grep/search_*）、**Plan**（架构顾问：Read/Glob/Grep/WebFetch/WebSearch）、**general-purpose**（父全集减 Subagent）。各 = system prompt + AllowedTools 白名单 + DefaultMaxTurns。`filterTools(type, all)` = 白名单交集 ∖ 黑名单（Subagent + 写类 create/edit/delete/trigger——防递归 + 防写）。

6. **Spawn 实现**（`subagent.Service.Spawn(ctx, agentType, prompt)`）：registry.Get(type) → `Resolve(ctx)` 拿 Bundle → mint subMsgID（msg_）+ subagentRunID → `CreateMessage`(开 sub-message streaming, SubagentID, Attrs parentBlockId=GetToolCallID) + emit message_start(ParentID=toolCallID) → 建 subagentHost + 子 ctx（`SetSubagentID(runID)` + `WithAgentState(New())` + `SetMessageID(subMsgID)`，保留父 Bridge/conv/ws）→ `loop.Run` → host.WriteFinalize 落终态 sub-message + emit message_stop → 返 `result.LastMessage`。

7. **Task 工具**（`app/tool/subagent`）：5 方法，参数 `{subagent_type enum[Explore,Plan,general-purpose], prompt, max_turns?}`。Execute：**防递归**（`GetSubagentID(ctx)` 在 → 拒 `已在 subagent 内不可再派`）+ 读 `GetToolCallID` + 调 `SubagentRunner.Spawn` → 返 result（软失败返串、不冒泡）。danger=safe（LLM 自报）。注入 `SubagentRunner`（subagent.Service）。

8. **防递归双保险**：①Task 工具不进 subagent 的 filterTools 白名单（registry 黑名单含 Subagent）②Task Execute 见 `GetSubagentID` 拒。深度 = **1**（subagent 内不能再派；诚实标，废 DOC-123「限 2」）。

强化地基：无（messages 加列 + reqctx 加 ToolCallID 种子 + loop 加 1 行 seed 是本轮自带）。subagent **无 catalog/relation/REST**（非 Quadrinity 实体、无表、是运行时机制；3 类型内置）。

修改后完整逻辑：
- **domain/messages/messages.go**：Message += `SubagentID`（+ doc）。
- **infra/store/messages/messages.go**：DDL `messages` 加 `subagent_id TEXT NOT NULL DEFAULT ''`。
- **pkg/reqctx/conversation.go**（或新 toolcall.go）：`SetToolCallID`/`GetToolCallID`。
- **app/loop/tools.go**：`runOneTool` 执行工具前 seed ToolCallID。
- **app/chat/history.go**：LoadHistory 过滤 sub-message。
- **app/subagent/**（新）：registry.go / subagent.go（Service+Deps+Spawn+ports+Bundle）/ host.go / emit.go。
- **app/tool/subagent/subagent.go**（新）：Task 工具。

删除 / 合并：无（纯增 + 1 列 + 1 过滤）。

契约变更（→ contract-changes #40）：domains/subagent.md DOC-123 **整篇重写**（砍 cv_xxx 子对话 / 深度限 2→诚实标 1 / AgentState 沙箱 / 4 虚构错误码；改 Parent-Child Message Anchoring〔SubagentID + parentBlock〕 + Spawn 同步 + registry 3 类型 + Task 工具 + E3）；messages.md §1 Message + `SubagentID`（+ §6 LoadHistory 过滤注记）；chat.md §4.1 LoadHistory 过滤 sub-message；database.md §2.2 messages 加 subagent_id；reqctx 注记 ToolCallID 种子。**无新 REST**；**无新 error-code**（防递归/失败软返 tool-result 串、不冒泡，对齐旧「工具失败不冒泡」）。

新测试（全离线，fake LLM）：
- registry：3 类型存在 + filterTools（Explore 只读集、general-purpose 父全集减 Subagent、黑名单剔 Subagent/写类）。
- Spawn 端到端（fake client + 真 messages store）：派 → sub-message 落盘（SubagentID set、Attrs parentBlockId、role=assistant、blocks 在）+ 返 result.LastMessage；message_start/stop 经 fake Bridge（节点 ParentID=toolCallID）。
- chat LoadHistory 过滤：父对话含一条 SubagentID sub-message → LoadHistory 跳过（父 LLM 历史不含子 trace）。
- 防递归：Task 工具 Execute 在 `SetSubagentID` 的 ctx 下 → 拒。
- Task 工具：Execute 调 Spawn（fake runner）返 result；ValidateInput（缺 type/prompt）。
- loop ToolCallID seed：执行工具时 ctx 带 tc.ID（一个读 ctx 的 fake 工具验证）。

验证：gofmt / build ./... / vet / test 全绿。

是否更干净（自证）：① subagent 复用 loop（共享 ReAct）+ chat 的落盘/SSE 模式（混血 host，不重写）；② 持久化 B 用 1 列 SubagentID（顶层过滤 + reload 树），不建 cv_xxx 子对话（废旧虚构）；③ model 自有 resolver（不耦合 chatapp，workspace 默认 = 常见 effective model，override 承袭按需）；④ 防递归用 SubagentID 存在性（种子已埋）+ 白名单剔除双保险，深度 1 诚实；⑤ 无 catalog/relation/REST（非实体、运行时机制）；⑥ ToolCallID 种子是通用能力（工具知自己 call id）、E3 锚点。

遗留 / 下一步：**波次 5 仅剩 contextmgr**（M5.3 压缩：写 message_blocks 的 context_role + conversation.summary/summaryCoversUpToSeq，LoadHistory 的 hot/warm/cold/archived 投影已在 R0055 接好）。真·conv.ModelOverride 承袭 + subagent 内部 trace 的更细 UX（如逐 subagent 折叠）按需。M7 装配：skill 注入 subagent runner（`s.subagent` 当前 nil 降级）+ Task 工具入 Toolset + subagent Deps 注真。
