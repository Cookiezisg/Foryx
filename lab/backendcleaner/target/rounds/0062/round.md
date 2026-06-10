# Round 0062 — SSE-B：tool 流式 emit（机制 + 全 B 组逐 tool）

类型 / 目标：SSE 收尾的 B 层。R0061 后 SSE-A（StreamHandler 三流订阅）+ P1（改名对账）已收；messages 流的 ① 请求（tool_call args，含代码）+ thinking/主文本 + ③ tool_result 早在发。**B = 补 tool_call 的 ② 中间过程**（tool 内部的内层 LLM / 长进程 / 子循环实时流），用户拍板「全 B 组都做」。

**现状（亲验）**：`emitter`（open/delta/close）是 loop 私有、tool 够不着；`progress` 块型 events.md §1 早列、domain 里漏了；subagent 是唯一先例（block 树经 `Open.ParentID` 嵌在 tool_call 下，E3）。

## 机制 keystone（B0 + B0.5，所有 tool 地基）

> **B0.5 修正（用户定调「请求/中间态/结果都要持久化」）**：progress 从最初设计的「仅流不持久」改为**一等持久块**——随回合落 message_blocks、刷新可重放；但 LLM 历史投影（`BlocksToAssistantLLM`）是类型白名单（text/reasoning/tool_call/tool_result），故 progress **持久但永不回喂模型**（无 token 爆炸）。

1. **domain/messages**：`BlockTypeProgress = "progress"` 进 `IsValidBlockType` + store DDL `type CHECK`（一等持久块）。
2. **loop 开 tool-facing 口**：public `loop.ToolProgress(ctx) *ToolProgressWriter`
   - 复用 `newEmitter(ctx)`（bridge from `WithBridge` + conversation scope）+ `reqctx.GetToolCallID`（parentID）
   - 实现 `io.Writer`：首 Write 懒开 `progress` block（`Open.ParentID = toolCallID`）+ delta；`Close()` 发快照 + **把成块交给 ctx 捕获槽**（`progressCapture`）
   - **nil 安全**：无 bridge / 无 toolCallID（REST / 测试 / 非流 host）→ 全 no-op
3. **loop 持久化**：`runOneTool` 埋 `progressCapture` 进 ctx → tool 跑完收 `[progress…, tool_result]`（progress 在前=时序）；`runTools` 拍平。progress 随 `allBlocks`→`FinalizeMessage` 落库（`parent=tc.ID`，同 tool_result 兄弟）。`extendHistory` 白名单天然排除 progress 出 LLM。
4. 测试：ToolProgress 流序 + nil no-op + io.Writer 语义；`runOneTool` 持久化 [progress,result]/沉默工具仅 result。

## 逐 tool 接（B1–B6，全复用 B0）

| Phase | tool | 接什么 → ToolProgress |
|---|---|---|
| B1 | **Bash** | 实时 stdout/stderr（最显眼、端到端验 B0） |
| B2 | create/edit_function · create/edit_handler | forgeSink（`OnAttempt`/`OnFixing`）= 装依赖 + 内层 LLM 改依赖 log |
| B3 | call_handler | `OnProgress`（method `yield`） |
| B4 | WebFetch | 内层 LLM 摘要链（边生成边流） |
| B5 | invoke_agent | 子 ReAct loop block 流式嵌 tool_call 下（`SetMessageID(ctx, toolCallID)`，用 ctx 已有 bridge）；**持久化落 agent_executions.transcript 自包含、不碰 messages 表**（用户定调）——execution 经 `ToolCallID` 关联回 tool_call，前端 reload 从 execution 重水合。**不抄 subagent 的 sub-message 模式** |
| B6 | install_mcp_server ✅ · run_function ✅ · mcp 动态 ✅ | **install**：`ensureEnv` 传 ProgressFunc 写 ToolProgress（装 runtime 实时流）。**run_function**：driver 把用户 `print()` 重定向 stderr（**顺带修了 print 破坏结果 JSON 的 bug**）+ `SpawnOptions.StreamErr` tee（三层穿透 domain/app/infra）+ adapter 接 ToolProgress。**mcp 动态**：go-sdk `ProgressNotificationHandler`（client 级）+ token→sink 注册表（atomic 计数 token）；**infra 零 app 依赖**——`WithProgress` ctx 助手传 `func(string)` 回调（DIP），dynamic 工具层建 ToolProgress + 设 ctx 回调 |

每 phase：建 + 测 + 独立 commit。

## 验证 + 文档（PLAYBOOK ④）
- `go build ./...` + loop/各 tool 测试 + 全模块 0 FAIL。
- messages.md（progress 块：持久但 LLM 白名单排除）+ **database.md**（message_blocks type CHECK 加 `progress`）+ events.md §1 as-built + contract-changes（progress 块型 + 逐 tool 流式+持久化，前端渲在 tool_call 下）+ 各 domain doc 注流式。

## C 层（B 后，独立轮）
forge 双写 entities 生产侧——锻造工具进度同写 entities 流（给前端实体面板），entities 流不再空。
