---
id: DOC-123
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# Subagent Domain — 递归派生与子智能体通信原理

> **核心职责**：Subagent 是 Forgify 实现 **“分治策略”** 的核心。它允许一个对话中的 Agent 作为一个特殊的工具，递归地“派生”出一个新的、临时的子智能体来处理特定的、可拆解的子任务，从而在不污染主对话历史的情况下完成重度任务。

---

## 1. 物理模型 (Data Anatomy)

Subagent 不设独立实体表，它是对 `Conversation` 域和 `Chat` 域的 **“特殊投影”**。

### 1.1 `SubagentRun` (逻辑 DTO)
```typescript
interface SubagentRun {
    id: string;              // 同 msg_ID，作为子回合标识
    conversationId: string;  // 指向一个隐式的、临时的子对话 ID
    parentBlockId: string;   // 物理锚点：指向父对话中的 tool_call 块
    type: string;            // 子智能体预设名（如 "researcher"）
    status: string;          // running|completed|failed
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Virtual Conversation Isolation (虚拟对话隔离)
当 LLM 调 `Subagent(task)` 时：
1. **静默开窗**：系统在后端创建一个隐式的、不出现在列表中的 `cv_xxx` 对话。
2. **上下文隔离**：子智能体只接收 `task` 描述，不感知父对话的完整历史（除非显式传入）。
3. **副作用隔离**：子智能体可以调工具、写临时文件，但所有操作都在其专属的子 Context 下。

### 2.2 Parent-Child Anchoring (父子锚点关联)
这是 Forgify V1.2 的关键架构改进：
- **物理引用**：子智能体的每一个消息，其 `parent_block_id` 均指向父对话中那个发起的 `tool_call`。
- **渲染递归**：前端通过 SSE 收到消息时，若发现 `parentBlockId` 非空，会自动将其嵌套在对应的工具调用气泡中展现。

### 2.3 Inheritance Policy (设置承袭)
子智能体不是“白板”，它会自动继承父对话的特定属性：
- **Model Override**：若父对话指定了用 GPT-4o，子任务自动跟进。
- **Locale**：回复语言自动保持一致。
- **AgentState**：子任务拥有独立的临时沙箱环境。

---

## 3. 生命周期 (Lifecycle)

1. **派生 (Spawning)**：LLM 调 `Subagent` 工具。
2. **初始化 (Seeding)**：后端创建子 Ctx，注入 Subagent 专用 System Prompt。
3. **自主执行 (Acting)**：子智能体在其独立循环内运行 N 回合。
4. **合流 (Summarizing)**：子任务终结，将最终结论作为 `tool_result` 返回给父 Agent。
5. **销毁 (GC)**：子对话标记为隐式，在父对话删除或超时后彻底物理清理。

---

## 4. 跨域集成 (Interactions)

- **Chat**：作为 `Subagent` 工具的底层驱动。
- **Eventlog**：驱动层级化、嵌套式的内容推流。
- **Model**：决定子智能体的“智力分布”。
- **Todo**：子智能体可以被授权读写父对话的任务清单。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 场景 |
|---|---|---|---|
| `ErrRecursionTooDeep`| 400 | `RECURSION_LIMIT` | 尝试在子智能体里再开子智能体（深度默认限 2）。 |
| `ErrSubagentCrash` | 502 | `SUBAGENT_ERROR` | 子进程崩溃或 LLM 停止响应。 |
| `ErrTaskAmbiguous` | 422 | `INVALID_REQUEST` | 给子智能体的指令不足以开辟任务。 |
| `ErrToolAccessDenied`| 403 | `PERM_DENIED` | 子智能体尝试调用未授权的高危工具。 |
