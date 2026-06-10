# Round 0062 — SSE-B：tool 流式 emit（机制 + 全 B 组逐 tool）

类型 / 目标：SSE 收尾的 B 层。R0061 后 SSE-A（StreamHandler 三流订阅）+ P1（改名对账）已收；messages 流的 ① 请求（tool_call args，含代码）+ thinking/主文本 + ③ tool_result 早在发。**B = 补 tool_call 的 ② 中间过程**（tool 内部的内层 LLM / 长进程 / 子循环实时流），用户拍板「全 B 组都做」。

**现状（亲验）**：`emitter`（open/delta/close）是 loop 私有、tool 够不着；`progress` 块型 events.md §1 早列、domain 里漏了；subagent 是唯一先例（block 树经 `Open.ParentID` 嵌在 tool_call 下，E3）。

## 机制 keystone（B0，所有 tool 地基）

1. **domain/messages**：补 `BlockTypeProgress = "progress"`——**stream-only、永不持久化**（tool 中间过程不入 message_blocks / 不入 LLM 历史）→ **不进 IsValidBlockType、不动 DDL CHECK**（那是持久化块的闸）。
2. **loop 开 tool-facing 口**：public `loop.ToolProgress(ctx) *ToolProgressWriter`
   - 复用 `newEmitter(ctx)`（bridge from `WithBridge` + conversation scope）+ `reqctx.GetToolCallID`（parentID）
   - 实现 `io.Writer`：首 Write 懒开 `progress` block（`Open.ParentID = toolCallID`）+ delta；`Close()` 发快照（delta 可丢、快照是重连真相）
   - **nil 安全**：无 bridge / 无 toolCallID（REST / 测试 / 非流 host）→ 全 no-op，tool 在任何路径都正确
3. 测试：fake bridge 验 open(parentID=toolCall)/delta/close 序列 + 无 bridge no-op + io.Writer 语义。

## 逐 tool 接（B1–B6，全复用 B0）

| Phase | tool | 接什么 → ToolProgress |
|---|---|---|
| B1 | **Bash** | 实时 stdout/stderr（最显眼、端到端验 B0） |
| B2 | create/edit_function · create/edit_handler | forgeSink（`OnAttempt`/`OnFixing`）= 装依赖 + 内层 LLM 改依赖 log |
| B3 | call_handler | `OnProgress`（method `yield`） |
| B4 | WebFetch | 内层 LLM 摘要链（边生成边流） |
| B5 | invoke_agent | 子 ReAct loop block 树嵌 tool_call 下（**E3，复用 subagent**，非 progress block——②③） |
| B6 | run_function · mcp 动态 · install_mcp_server | 代码 stdout / MCP progress notif / 装包进度 |

每 phase：建 + 测 + 独立 commit。

## 验证 + 文档（PLAYBOOK ④）
- `go build ./...` + loop/各 tool 测试 + 全模块 0 FAIL。
- messages.md（progress 块 stream-only）+ events.md §1 as-built + contract-changes（progress 块型 + 逐 tool 流式，前端渲在 tool_call 下）+ 各 domain doc 注流式。**database 不变**（progress 不持久化）。

## C 层（B 后，独立轮）
forge 双写 entities 生产侧——锻造工具进度同写 entities 流（给前端实体面板），entities 流不再空。
