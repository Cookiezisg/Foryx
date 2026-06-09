---
id: DOC-123
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-09
review-due: 2026-09-01
audience: [human, ai]
---
# Subagent — 递归子对话 (Recursive Sub-conversation)

> **核心地位**：subagent 是 LLM 用 `Subagent`（Task）工具派出的**隔离子 agent**——给它一段自包含任务、它跑完返回结果。**≈ 递归的 chat**：复用共享 ReAct 引擎（`app/loop`），**无自己的表**（回合作为 sub-message 落**父对话**的 `messages` 表、带 `SubagentID` 标记），**承袭父的 effective model**，**不能再派 subagent**（深度 1）。
>
> **不是 Quadrinity 实体**：subagent 无 DB 实体、无 catalog/relation/REST——它是运行时机制，3 个内置类型硬编码。消费者：skill 的 fork 模式（`SubagentRunner` 端口）+ `Subagent` 工具（LLM 直接派）。
>
> **本文 = R0058 as-built**。旧 DOC-123 的 **cv_xxx 子对话 / 深度限 2 / AgentState 沙箱 / 4 个虚构错误码**全是幻想，已删（见 §6）。

---

## 1. 消费契约：`SubagentRunner.Spawn`

```go
// domain/skill.SubagentRunner（subagentapp.Service 满足）
Spawn(ctx context.Context, agentType, prompt string) (result string, err error)
```
**同步**：跑完返最终文本。两个消费者共用：
- **skill fork**：`context:fork` 的 skill 把渲染后的正文派给 subagent（`app/skill/activate.go`）。
- **`Subagent` 工具**：LLM 直接派（`app/tool/subagent`）。

`subagent==nil` 时 skill fork 降级 `SKILL_SUBAGENT_UNAVAILABLE`（M7 注入前）。

---

## 2. host：混血（agentHost + chatHost）

subagent 跑 `loop.Run`，host 是**混血**：

| loop.Host 方法 | subagentHost | 像 |
|---|---|---|
| `LoadHistory` | 只返任务 prompt 一条 user 消息（隔离运行、无父线程） | agentHost |
| `Tools` | 按类型过滤的**静态白名单**（无 lazy/search_tools/AutoActivator） | agentHost |
| `WriteFinalize` | **Detached** 落 sub-message（SubagentID）+ blocks + 推 message_stop | chatHost |

不实现 `AutoActivator`/`ReminderProvider`/`StepRecorder`。

---

## 3. 持久化：写父对话 + `SubagentID` 标记（R0058 拍板）

subagent 的回合作为 **sub-message 落父对话的 `messages` 表**（**无 cv_xxx 子对话**）：
- `Message.SubagentID` = subagent run id（`subagt_`）；顶层回合 `""`。
- `Message.Attrs["parentBlockId"]` = 派它的 tool_call 的 block id（reload 树锚点）。
- blocks 经 loop 落 `message_blocks`（`MessageID` = sub-message id；E3 流嵌套见 §4）。
- **chat `LoadHistory` 排除 `SubagentID != ""`**：subagent 内部 trace **不进父 LLM 历史**，父只见派它的 tool_call + 其 **tool_result**（= subagent 最终答案 `result.LastMessage`，由 `Subagent` 工具 Execute 返回、loop 落成 tool_result）。
- **`ListMessages`（REST）仍返回 sub-message**：刷新页面能重建 subagent 子树。

> token：sub-message 的 input/output token 计入对话 `GET /usage` 总和（subagent 成本是对话成本一部分）。

---

## 4. E3 流嵌套（live/reload 一致）

subagent 全程进 messages SSE，**复用父 loop 的 Bridge**（subagent 跑在 `Subagent` 工具 Execute 的 ctx 内、已带 `loop.WithBridge` + 同 `conversation:<id>` scope）：
- subagent **自己的 message 节点**：`Open{type:"message", ParentID:<派它的 tool_call id>}`（挂 tool_call 下）→ message_stop=`Close{Result: 终态元数据}`。content 带 `subagent:true`（前端分组）。
- subagent 的 block 节点：loop 以 `reqctx.SetMessageID(subMsgID)` 跑 → `Open.ParentID = subMsgID`（挂 sub-message 下）。
- 树：`tool_call(Subagent) → message(subagent) → reasoning/text/tool_call→tool_result`，旁边 `tool_result`（= 最终答案）。

**tool_call id 来源**：loop `runOneTool` 执行工具前 `reqctx.SetToolCallID(ctx, tc.ID)`，`Subagent` 工具内 Spawn 经 `GetToolCallID` 读出作锚（fork skill 不在 tool_call 下时为空 → 挂 conversation 根，仍合法）。

---

## 5. 类型 / model / 防递归

### 5.1 内置 3 类型（registry，硬编码）
| 类型 | system prompt | AllowedTools（白名单） | DefaultMaxTurns |
|---|---|---|---|
| **Explore** | 只读代码侦察（定位文件/定义/用法） | Read, LS, Glob, Grep | 30 |
| **Plan** | 调研 + 出实现计划（不改动） | Read, LS, Glob, Grep, WebFetch, WebSearch | 25 |
| **general-purpose** | 聚焦任务端到端 | nil = 父全集 | 25 |

`filterTools` = 白名单交集（空=全部）**且总剔 `Subagent`**（递归守卫）。

### 5.2 model 承袭
subagent 自有 `ModelResolver.Resolve(ctx)→Bundle`，M7 适配器 = `model.Resolve(ScenarioDialogue, nil, picker)`（workspace dialogue 默认 = 父常见无 override 时的 effective model）。**显式 conv.ModelOverride 承袭延后**（会越 reqctx 的 pkg→domain 边界，单用户 override 罕见）。

### 5.3 防递归（深度 1，双保险）
- ①`filterTools` 总剔 `Subagent` 工具 → subagent 的工具集不含它。
- ②`Subagent` 工具 Execute + `Spawn` 见 `reqctx.GetSubagentID(ctx)` 在 → 拒。

subagent 内**不能再派 subagent**。reqctx `SubagentID` 种子同时给 todo 作用域 `(conversation, subagent?)`。

---

## 6. 错误 / 契约边界

- **无 HTTP error-code**：派发失败（坏类型 / 模型解析错 / 递归）由 `Subagent` 工具 Execute 作 **error 半边**返回 → loop 渲成 tool_result 串给 LLM（工具级失败、不冒泡 HTTP）。
- **无 REST / catalog / relation**（运行时机制、非实体）。
- **DB**：仅 `messages.subagent_id` 列（R0058）；无新表。
- **废（旧 DOC-123 幻想）**：~~cv_xxx 子对话~~（实写父对话 + parentBlock 锚）、~~深度限 2~~（实为 1）、~~AgentState 沙箱~~（实为 ctx 隔离 + `agentstate.New()`）、~~`ErrRecursionTooDeep`/`ErrSubagentCrash`/`ErrTaskAmbiguous`/`ErrToolAccessDenied`~~（4 个虚构码，工具失败软返）。

---

## 7. 装配（M7）

`subagent.New(Deps{Messages, Resolver, Tools, Bridge})` → 注入 skill 的 `SubagentRunner`（当前 nil 降级）+ `Subagent` 工具入 Toolset。`agentstate` 子 run 独立新建（防污染父 SeenFiles）。
