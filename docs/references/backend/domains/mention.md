---
id: DOC-114
type: reference
status: active
owner: @weilin
created: 2026-04-22
reviewed: 2026-06-02
review-due: 2026-09-01
audience: [human, ai]
---
# @-Mention Domain — 实体引用、内容快照与注入原理

> **核心职责**：Mention 是对话中实体的 **“即时通讯员”**。它允许用户在对话中使用 `@` 符号引用系统内的 Document, Function, Handler, Workflow 等实体，并确保在发送瞬间捕捉其内容快照注入 LLM 上下文。

---

## 1. 物理模型 (Data Anatomy)

### 1.1 `MentionInput` (前端载荷)
```typescript
interface MentionInput {
    type: "document" | "function" | "handler" | "workflow";
    id: string; // 实体物理 ID
}
```

### 1.2 `Reference` (持久化快照元数据)
存储在 `messages.attrs` 的 `mentions` 数组中。
```go
type Reference struct {
    Type string `json:"type"`
    ID   string `json:"id"`
    Name string `json:"name"`
}
```

---

## 2. 核心原理 (Principles)

### 2.1 Freeze-on-Send (发送即冻结)
Forgify 不支持动态引用（即：LLM 生成时再去查实体最新内容）。
- **原理**：当用户点击“发送”按钮时，`chat.Service` 会立即调用各个域注册的 `Resolver`。
- **快照行为**：系统获取实体当前的完整内容（代码、文本、图定义），并将其作为一个隐藏的 `system` 消息段附着在该回合的输入中。
- **优点**：即使实体在 10 分钟后被用户修改或删除，对话历史中的“当时所见”依然保持一致。

### 2.2 Content-Specific Rendering (差异化渲染)
Mention 注入上下文时的表现形态因类型而异：
- **Document**：渲染为 `<document>` XML 标签包围的纯文本。
- **Function/Handler**：渲染为 `<code_snippet>` 及其依赖声明。
- **Workflow**：渲染为 `<graph_definition>` JSON 片段。

### 2.3 Registry-Based Extension (基于注册表的扩展)
`Mention` 域本身不持业务逻辑，它定义了一个 `Resolver` 接口：
```go
type Resolver interface {
    Type() MentionType
    Resolve(ctx context.Context, id string) (*Reference, error)
}
```
各业务域（如 `functionapp`）在启动时调用 `chatService.RegisterMentionResolver(this)` 进行挂载。

---

## 3. 生命周期 (Lifecycle)

1. **选点 (Picking)**：用户在前端编辑器输入 `@`，调 `/api/v1/catalog` 搜索并选中。
2. **提交 (Submitting)**：`POST /messages` body 包含 `mentions` 数组。
3. **解析 (Resolving)**：后端 `chat.Service` 遍历数组，同步调各域 `Resolver` 抓取 `Reference` 元数据。
4. **注入 (Injecting)**：`SystemPromptSections` 模块将抓取到的实体内容拼装进 wire prompt。
5. **归档 (Journaling)**：`Relation` 域同步记录一条 `message_mentions_entity` 边。

---

## 4. 跨域集成 (Interactions)

- **Chat**：主要的消费方和流程控制器。
- **Catalog**：为前端提供 `@` 自动补全的备选项。
- **Relation**：利用 Mention 建立实体的活跃度热力图。

---

## 5. 错误字典 (Sentinels)

| Sentinel | HTTP | Wire Code | 场景 |
|---|---|---|---|
| `ErrResolverNotFound`| 500 | `INTERNAL_ERROR` | 尝试引用一个后端未注册 Resolver 的类型。 |
| `ErrEntityNotFound` | - | - | 解析失败，自动回退到名为 `(无法加载)` 的 Stub，不中断消息发送。 |
| `ErrInvalidInput` | 400 | `INVALID_REQUEST` | Mention 数组格式错。 |
