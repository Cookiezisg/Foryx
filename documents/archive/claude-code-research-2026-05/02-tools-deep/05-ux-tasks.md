# 05 — UX + Task Tools 深挖

> 02-tools-deep 系列**末篇**。
> ✅ **AskUserQuestion** —— LLM 主动问用户（agentic UX 杀手级特性）
> ✅ **TaskCreate / TaskList / TaskGet / TaskUpdate** —— LLM 自管理 todo（4-in-1）
> ⚠️ **TaskStop** —— P2 简评（与 Bash background 配套）
> ⚠️ **EnterPlanMode / ExitPlanMode** —— P2 简评（"草稿审批"模式）
> ❌ **TodoWrite** —— legacy；仅作 prompt engineering 演化对照
> ❌ **SendUserMessage** —— 与 Forgify chat.message SSE 重叠

## 信息源

- **主源**：
  - [`tool-description-askuserquestion.md`](https://github.com/Piebald-AI/claude-code-system-prompts/blob/main/system-prompts/tool-description-askuserquestion.md) — ccVersion **2.1.47**
  - [`tool-description-askuserquestion-preview-field.md`](https://github.com/Piebald-AI/claude-code-system-prompts/blob/main/system-prompts/tool-description-askuserquestion-preview-field.md) — ccVersion **2.1.69**（HTML preview 子规范）
  - [`tool-description-taskcreate.md`](https://github.com/Piebald-AI/claude-code-system-prompts/blob/main/system-prompts/tool-description-taskcreate.md) — ccVersion **2.1.84**
  - [`tool-description-tasklist-teammate-workflow.md`](https://github.com/Piebald-AI/claude-code-system-prompts/blob/main/system-prompts/tool-description-tasklist-teammate-workflow.md) — ccVersion **2.1.38**
  - [`tool-description-todowrite.md`](https://github.com/Piebald-AI/claude-code-system-prompts/blob/main/system-prompts/tool-description-todowrite.md) — ccVersion **2.1.84**（legacy 完整版）
  - [`tool-description-enterplanmode.md`](https://github.com/Piebald-AI/claude-code-system-prompts/blob/main/system-prompts/tool-description-enterplanmode.md) — ccVersion **2.1.63**
  - [`tool-description-exitplanmode.md`](https://github.com/Piebald-AI/claude-code-system-prompts/blob/main/system-prompts/tool-description-exitplanmode.md) — ccVersion **2.1.14**
- **副源**：[code.claude.com tools-reference](https://code.claude.com/docs/en/tools-reference)
- **写作日期**：2026-05-03

---

## AskUserQuestion

### 1. Description 原文（Piebald v2.1.47）

> Use this tool when you need to ask the user questions during execution. This allows you to:
> 1. Gather user preferences or requirements
> 2. Clarify ambiguous instructions
> 3. Get decisions on implementation choices as you work
> 4. Offer choices to the user about what direction to take.
>
> Usage notes:
> - Users will always be able to select "Other" to provide custom text input
> - Use `multiSelect: true` to allow multiple answers to be selected for a question
> - If you recommend a specific option, make that the first option in the list and add "(Recommended)" at the end of the label
>
> Plan mode note: In plan mode, use this tool to clarify requirements or choose between approaches BEFORE finalizing your plan. Do NOT use this tool to ask "Is my plan ready?" or "Should I proceed?" - use **${EXIT_PLAN_MODE_TOOL_NAME}** for plan approval. IMPORTANT: Do not reference "the plan" in your questions (e.g., "Do you have feedback about the plan?", "Does the plan look good?") because the user cannot see the plan in the UI until you call **${EXIT_PLAN_MODE_TOOL_NAME}**. If you need plan approval, use **${EXIT_PLAN_MODE_TOOL_NAME}** instead.

### 2. Preview field 子描述（Piebald v2.1.69）

> Preview feature:
> Use the optional `preview` field on options when presenting concrete artifacts that users need to visually compare:
> - HTML mockups of UI layouts or components
> - Formatted code snippets showing different implementations
> - Visual comparisons or diagrams
>
> Preview content must be a self-contained HTML fragment (no `<html>`/`<body>` wrapper, no `<script>` or `<style>` tags — use inline style attributes instead). Do not use previews for simple preference questions where labels and descriptions suffice. Note: previews are only supported for single-select questions (not multiSelect).

### 3. JSON Schema

⚠️ **schema 来自社区推断 + 行为反推**（CC 没暴露完整 schema）：

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "additionalProperties": false,
  "required": ["question", "options"],
  "properties": {
    "question": {
      "type": "string",
      "description": "The question text shown to the user"
    },
    "options": {
      "type": "array",
      "minItems": 2,
      "items": {
        "type": "object",
        "required": ["label"],
        "properties": {
          "label": {
            "type": "string",
            "description": "Option label; add '(Recommended)' suffix on the recommended one"
          },
          "description": {
            "type": "string",
            "description": "Optional longer explanation shown under the label"
          },
          "preview": {
            "type": "string",
            "description": "HTML fragment for visual preview (single-select only; no <html>/<body>/<script>/<style>; use inline style attrs)"
          }
        }
      }
    },
    "multiSelect": {
      "type": "boolean",
      "default": false,
      "description": "Allow multiple answers"
    }
  }
}
```

**自动注入**："Other" option 由 framework 默认追加，**LLM 不需要传**——用户永远可以"自定义文本"。

### 4. 算法行为

**阻塞执行** ✅
- 调用 AskUserQuestion 后，**LLM agent loop 挂起等用户输入**
- 不是 polling——是 turn-level pause；用户在 UI 选完点确认才解锁
- 解锁后用户的回答以 tool_result 形式回到 LLM context

**单选 vs 多选**
- 默认 `multiSelect=false` 单选；用户只能选一个 option（或"Other"自填）
- `multiSelect=true` 多选；用户可勾若干 options + 可附自填
- 多选不允许 preview field（preview 是单选独享）

**"(Recommended)" 约定**
- 推荐选项放第一位 + label 后缀 "(Recommended)"
- prompt-side 约定，framework 不强制

**Plan mode 特殊规则** ✅
- plan mode 下用 AskUserQuestion 澄清需求 / 选择方案——但**不能用来求 plan 批准**
- "Should I proceed?" / "Is my plan ready?" 这类问题应该用 ExitPlanMode 而非 AskUserQuestion
- IMPORTANT 子规则：plan mode 下问题里**不能引用 "the plan"**——用户在 ExitPlanMode 之前看不到 plan，问"plan 看起来怎样"会让用户困惑

### 5. 输出格式给 LLM

用户回答以 tool_result 字符串形式回 LLM，推测格式：

```
User selected: Option B (Recommended)
[+ Custom: "actually I want both A and B"]
```

或多选：
```
User selected:
- Option A
- Option C
+ Custom: "and also do X"
```

或全自定义：
```
User selected: Other
Custom: "I want a totally different approach: ..."
```

### 6. Forgify Go 实现要点

#### 6.1 设计：复用 chat.message 快照，不开新 SSE 事件族

> **关键洞察**：question 不需要单独的 SSE 事件——它**就是一个 tool_call block**，已经会被现有 `chat.message` 快照机制推送。answer **就是对应的 tool_result block**。整个交互天然嵌进现有 block 模型，前端只需特化处理 `name=AskUserQuestion` 的 tool_call block 渲染问答 UI。

**完整流程**：

```
1. LLM emit tool_call (name="AskUserQuestion", args={question, options, multiSelect})
2. streamLLM 推 chat.message 快照（已含 tool_call block）→ 前端看到
3. 前端识别 block.data.name === "AskUserQuestion" 且无对应 tool_result → 渲染问答 UI
4. runTools 调 AskUserQuestion.Execute → 创建 waiter channel + 阻塞等
5. 用户在 UI 选完点确认 → 前端 POST /chat/{convID}:answer-question
6. handler 找到 waiter channel signal answer → tool 返回 answer 文本
7. runTools 写 tool_result block → publishMessageSnapshot 推 chat.message
8. 前端看到 tool_result，问答 UI 切到"已答"态
9. ReAct loop 继续下一轮，把 answer 喂回 LLM
```

**为何这个设计干净**：
- ❌ **不引入** `domain/events` 新事件类型（chat 大类一个 SSE 事件族搞定）
- ❌ **不引入** 新 domain entity（question 数据藏在 tool_call block.args，answer 藏在 tool_result block.result）
- ❌ **不引入** 新 DB 表（blocks 本来就落库——question + answer 自动持久化）
- ✅ **新增 1 个 HTTP endpoint** + **1 个 in-memory channel store**（仅当 backend 进程内的 agent 等答案时活）

#### 6.2 Tool 接口

```go
type AskUserQuestion struct {
    pending PendingQuestionStore  // in-memory waiter channels（无持久化层）
}

func (t *AskUserQuestion) Name() string                  { return "AskUserQuestion" }
func (t *AskUserQuestion) IsReadOnly() bool              { return false }
func (t *AskUserQuestion) NeedsReadFirst() bool          { return false }
func (t *AskUserQuestion) RequiresWorkspace() bool       { return false }
```

#### 6.3 单一新 endpoint

```
POST /chat/{convID}:answer-question
Body: {
  toolCallId: "tc_<id>",          // 即 question 的 tool_call block.data.id
  selectedLabels: ["Option B"],
  customText?: "实际我想…"
}
→ 200 + { ok: true }
```

handler 调 `chatService.AnswerQuestion(ctx, convID, toolCallID, answer)`：
1. 找到 in-memory waiter channel by toolCallID → signal
2. 不写额外 DB（tool 返回时 runTools 自然会写 tool_result block 落库）
3. 回 200

如果找不到 waiter（比如 backend 重启后老 conversation 的等待 channel 没了）→ 返 410 Gone + "Question expired; resubmit chat to retry"。

#### 6.4 挂起机制（关键代码）

```go
// app/chat/pending_question.go
type PendingQuestionStore struct {
    mu      sync.Mutex
    waiters map[string]chan QuestionAnswer  // toolCallID → 等待 channel
}

type QuestionAnswer struct {
    SelectedLabels []string
    CustomText     string
}

func (s *PendingQuestionStore) Register(toolCallID string) <-chan QuestionAnswer {
    s.mu.Lock()
    defer s.mu.Unlock()
    ch := make(chan QuestionAnswer, 1)
    s.waiters[toolCallID] = ch
    return ch
}

func (s *PendingQuestionStore) Cleanup(toolCallID string) {
    s.mu.Lock()
    defer s.mu.Unlock()
    delete(s.waiters, toolCallID)
}

func (s *PendingQuestionStore) Signal(toolCallID string, ans QuestionAnswer) error {
    s.mu.Lock()
    ch, ok := s.waiters[toolCallID]
    s.mu.Unlock()
    if !ok {
        return ErrQuestionNotPending
    }
    select {
    case ch <- ans:
        return nil
    default:
        return ErrQuestionAlreadyAnswered
    }
}

// Tool 实现
func (t *AskUserQuestion) Execute(ctx context.Context, argsJSON string) (string, error) {
    // ✅ 从 ctx 拿 toolCallID（reqctxpkg 已经注入）
    toolCallID, ok := reqctxpkg.GetToolCallID(ctx)
    if !ok {
        return "", fmt.Errorf("AskUserQuestion: missing toolCallID in context")
    }

    answerCh := t.pending.Register(toolCallID)
    defer t.pending.Cleanup(toolCallID)

    // 不需要主动 publish——question 已经在 tool_call block 里，
    // streamLLM 的 publishMessageSnapshot 已经把它推给前端了

    select {
    case answer := <-answerCh:
        return formatAnswerForLLM(answer), nil
    case <-ctx.Done():
        return "", fmt.Errorf("AskUserQuestion: cancelled before answer")
    case <-time.After(30 * time.Minute):
        return "User did not respond within 30 minutes.", nil
    }
}

func formatAnswerForLLM(a QuestionAnswer) string {
    var sb strings.Builder
    if len(a.SelectedLabels) > 0 {
        sb.WriteString("User selected:\n")
        for _, l := range a.SelectedLabels {
            fmt.Fprintf(&sb, "- %s\n", l)
        }
    }
    if a.CustomText != "" {
        fmt.Fprintf(&sb, "+ Custom text: %q\n", a.CustomText)
    }
    if sb.Len() == 0 {
        return "User confirmed but provided no specific answer."
    }
    return sb.String()
}
```

#### 6.5 持久化与 backend 重启

- **进行中的 question**：waiter channel 在内存；backend 重启 → channel 丢，挂起 agent 被 ctx-cancel → tool 返 error → assistant message 标 `error` 状态
- **已答的 question + answer**：tool_result block 已在 DB；前端刷新照样能看到完整问答历史
- **跨 session 的"补答"**（用户隔天回来想答之前的 question）→ **不支持**：question 过期就过期，让 LLM 重问。要做也是 Phase 6+ 复杂度

#### 6.6 前端契约

前端处理 `chat.message` 事件时，对每个 tool_call block：

```ts
if (block.type === 'tool_call' && block.data.name === 'AskUserQuestion') {
    const args = block.data.arguments;  // {question, options, multiSelect}
    const hasAnswer = message.blocks.some(b =>
        b.type === 'tool_result' && b.data.toolCallId === block.data.id
    );
    if (hasAnswer) {
        renderAnsweredState(args, hasAnswer);
    } else {
        renderQuestionUI(args, async (selectedLabels, customText) => {
            await fetch(`/chat/${convID}:answer-question`, {
                method: 'POST',
                body: JSON.stringify({ toolCallId: block.data.id, selectedLabels, customText }),
            });
        });
    }
}
```

#### 6.7 实施代价（修订）

- 后端：~**0.5 天**（PendingQuestionStore + endpoint + tool Execute + 测试）——比之前估的 1 天少，因为没新 SSE 类 / 新表
- 前端：在前端 chat 渲染逻辑里加 ~30 行特化（识别 AskUserQuestion 这个 tool_call 类型）；前端工作不在本轮
- 没前端时的体验：用户看到 LLM 调用了 AskUserQuestion，但 UI 不会显示问答框；30 分钟后 tool timeout——**不理想但不会卡死**，user 可重发 chat 让 LLM 换路径

**v1 决策**：**做后端**——0.5 天投入，POST endpoint + waiter store + tool 实现就绪。前端跟着 Phase 5 桌面化时配。无前端阶段 user 不要主动让 LLM 用 AskUserQuestion 即可（LLM 一般不会主动调；description 说"问用户" → user 没问题不会触发）。

---

## TaskCreate 族（4-in-1）

### 1. Description 原文（Piebald v2.1.84）

> Use this tool to create a structured task list for your current coding session. This helps you track progress, organize complex tasks, and demonstrate thoroughness to the user.
> It also helps the user understand the progress of the task and overall progress of their requests.
>
> ## When to Use This Tool
>
> Use this tool proactively in these scenarios:
>
> - Complex multi-step tasks - When a task requires 3 or more distinct steps or actions
> - Non-trivial and complex tasks - Tasks that require careful planning or multiple operations**${CONDTIONAL_TEAMMATES_NOTE}**
> - Plan mode - When using plan mode, create a task list to track the work
> - User explicitly requests todo list - When the user directly asks you to use the todo list
> - User provides multiple tasks - When users provide a list of things to be done (numbered or comma-separated)
> - After receiving new instructions - Immediately capture user requirements as tasks
> - When you start working on a task - Mark it as in_progress BEFORE beginning work
> - After completing a task - Mark it as completed and add any new follow-up tasks discovered during implementation
>
> ## When NOT to Use This Tool
>
> Skip using this tool when:
> - There is only a single, straightforward task
> - The task is trivial and tracking it provides no organizational benefit
> - The task can be completed in less than 3 trivial steps
> - The task is purely conversational or informational
>
> NOTE that you should not use this tool if there is only one trivial task to do. In this case you are better off just doing the task directly.
>
> ## Task Fields
>
> - **subject**: A brief, actionable title in imperative form (e.g., "Fix authentication bug in login flow")
> - **description**: What needs to be done
> - **activeForm** (optional): Present continuous form shown in the spinner when the task is in_progress (e.g., "Fixing authentication bug"). If omitted, the spinner shows the subject instead.
>
> All tasks are created with status `pending`.
>
> ## Tips
>
> - Create tasks with clear, specific subjects that describe the outcome
> - After creating tasks, use TaskUpdate to set up dependencies (blocks/blockedBy) if needed
> **${CONDITIONAL_TASK_NOTES}**- Check TaskList first to avoid creating duplicate tasks

### 2. JSON Schemas（4 个 tool）

#### TaskCreate

```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["subject"],
  "properties": {
    "subject":     { "type": "string",  "description": "Brief, actionable title in imperative form" },
    "description": { "type": "string",  "description": "What needs to be done" },
    "activeForm":  { "type": "string",  "description": "Present continuous form for spinner" },
    "metadata":    { "type": "object",  "description": "Free-form metadata key-value pairs" },
    "blockedBy":   { "type": "array", "items": { "type": "string" }, "description": "Task IDs this task depends on" }
  }
}
```

返回：`{ "taskId": "tsk_..." }`

#### TaskList

```json
{
  "type": "object",
  "additionalProperties": false,
  "properties": {}
}
```

无参；返回精简列表（只 subject + status + id；防 context bloat）。

#### TaskGet

```json
{
  "type": "object",
  "required": ["taskId"],
  "properties": {
    "taskId": { "type": "string" }
  }
}
```

返回完整 task 对象（含 description / metadata / blockedBy / 时间戳）。

#### TaskUpdate

```json
{
  "type": "object",
  "required": ["taskId"],
  "properties": {
    "taskId":        { "type": "string" },
    "status":        { "type": "string", "enum": ["pending", "in_progress", "completed"] },
    "owner":         { "type": "string", "description": "Teammate name or agent id (agent-team mode)" },
    "subject":       { "type": "string" },
    "description":   { "type": "string" },
    "activeForm":    { "type": "string" },
    "addBlockedBy":  { "type": "array", "items": { "type": "string" } },
    "removeBlockedBy": { "type": "array", "items": { "type": "string" } }
  }
}
```

### 3. 算法行为

**Task 状态机** ✅
```
pending → in_progress → completed
```
no rollback ；completed 后不能改回。

**核心规矩** ✅（来自 TodoWrite legacy 描述）
- "Exactly ONE task in_progress at a time"——并行 in_progress 是反 pattern
- "Mark complete IMMEDIATELY after finishing"——不要批量；防止 LLM 一口气标五个完成（有的实际没干完）
- "ONLY mark completed when FULLY accomplished"——若有 error / blocker，保持 in_progress，新建一个 task 描述阻塞原因

**存储** ✅
- `~/.claude/tasks/<sessionId>/<taskId>.json`（一个 session 一个文件夹）
- session 重启可恢复（`--resume` 时同步 tasks）
- 上限 50 task / session

**TaskList 设计巧思** ✅
- TaskList 只返**精简 summary**——避免 context 爆炸
- 想看完整内容才调 TaskGet——按需 hydrate
- 这是 **lazy loading 模式**：让 LLM 主动问深，而非每次全量塞

**Teammate workflow** （TaskList 子描述）

> ## Teammate Workflow
> When working as a teammate:
> 1. After completing your current task, call TaskList to find available work
> 2. Look for tasks with status 'pending', no owner, and empty blockedBy
> 3. **Prefer tasks in ID order** (lowest ID first) when multiple tasks are available, as earlier tasks often set up context for later ones
> 4. Claim an available task using TaskUpdate (set `owner` to your name), or wait for leader assignment
> 5. If blocked, focus on unblocking tasks or notify the team lead

⚠️ Forgify 单 agent，没 teammate workflow——这段不用。仍然保留 owner 字段以备 Phase 5+ 多 agent 时复用。

### 4. 已知 edge cases

- **删除 task**：description 说"Remove tasks that are no longer relevant from the list entirely"——TaskUpdate 应支持 `delete` action 或独立 `TaskDelete`？现行 schema 不明，Forgify 加 `TaskUpdate({status: "deleted"})` 即可
- **跨 session task**：`--resume` 恢复全部 pending；in_progress 保持（让 LLM 自己决定继续 or abandon）
- **50 上限**：超出会拒绝？降级警告？Forgify 加 ErrTaskLimitReached + 友好消息

### 5. 输出格式给 LLM

**TaskCreate** 返回：
```
Task created: tsk_abc123 ("Fix authentication bug in login flow")
```

**TaskList** 返回：
```
Tasks (3 total):
- tsk_001 [pending]    Fix authentication bug
- tsk_002 [in_progress] Add unit tests for auth flow
- tsk_003 [pending]    Update documentation
```

**TaskGet** 返回 JSON：
```json
{
  "id": "tsk_abc123",
  "subject": "Fix authentication bug in login flow",
  "description": "...",
  "activeForm": "Fixing authentication bug",
  "status": "in_progress",
  "blockedBy": [],
  "createdAt": "...",
  "updatedAt": "..."
}
```

**TaskUpdate** 返回：
```
Task updated: tsk_abc123 (status: pending → in_progress)
```

### 6. Forgify Go 实现要点

#### 6.1 数据存储：SQLite（不是 JSON 文件）

CC 用 `~/.claude/tasks/<sessionId>/<id>.json`——文件存。Forgify 已有 SQLite 基础设施，应该用 table。

```sql
CREATE TABLE chat_tasks (
    id              TEXT PRIMARY KEY,           -- "tsk_<16hex>"
    user_id         TEXT NOT NULL,
    conversation_id TEXT NOT NULL,
    subject         TEXT NOT NULL,
    description     TEXT,
    active_form     TEXT,
    status          TEXT NOT NULL CHECK (status IN ('pending', 'in_progress', 'completed', 'deleted')),
    owner           TEXT,                       -- Phase 5+ teammate
    metadata        TEXT,                       -- JSON
    blocked_by      TEXT,                       -- JSON array of taskIds
    created_at      DATETIME NOT NULL,
    updated_at      DATETIME NOT NULL,
    deleted_at      DATETIME                    -- 软删除（D1 规范）
);
CREATE INDEX idx_chat_tasks_conv_status ON chat_tasks(conversation_id, status) WHERE deleted_at IS NULL;
```

#### 6.2 SSE 事件家族

```go
type TaskCreated struct {
    Task *chatdomain.Task `json:"task"`
}
type TaskUpdated struct {
    Task        *chatdomain.Task `json:"task"`
    PrevStatus  string           `json:"prevStatus,omitempty"`
}
```

每次 TaskCreate / TaskUpdate 后 publish。前端实时渲染 todo panel。

#### 6.3 Tool 接口（4 个，结构相同）

```go
type TaskCreate struct {
    repo  chatdomain.TaskRepository
    bridge eventsdomain.Bridge
}
type TaskList struct  { repo chatdomain.TaskRepository }
type TaskGet struct   { repo chatdomain.TaskRepository }
type TaskUpdate struct {
    repo  chatdomain.TaskRepository
    bridge eventsdomain.Bridge
}

// All 4: IsReadOnly only（IsConcurrencySafe 已删，按 execution_group 调度）
// TaskList / TaskGet: ReadOnly=true, ConcurrencySafe=true
// TaskCreate / TaskUpdate: ReadOnly=false, ConcurrencySafe=false
```

#### 6.4 关键代码片段（TaskCreate）

```go
func (t *TaskCreate) Execute(ctx context.Context, argsJSON string) (string, error) {
    var args struct {
        Subject     string         `json:"subject"`
        Description string         `json:"description"`
        ActiveForm  string         `json:"activeForm"`
        BlockedBy   []string       `json:"blockedBy"`
        Metadata    map[string]any `json:"metadata"`
    }
    if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
        return "", fmt.Errorf("TaskCreate: %w", err)
    }
    if args.Subject == "" {
        return "Task subject is required.", nil
    }

    convID, _ := reqctxpkg.GetConversationID(ctx)
    uid, _    := reqctxpkg.GetUserID(ctx)

    // 50 上限
    n, err := t.repo.CountByConversation(ctx, convID)
    if err != nil { return "", fmt.Errorf("TaskCreate: count: %w", err) }
    if n >= 50 {
        return "Task limit reached (50 per conversation). Delete some tasks before creating new ones.", nil
    }

    task := &chatdomain.Task{
        ID:             newTaskID(),
        UserID:         uid,
        ConversationID: convID,
        Subject:        args.Subject,
        Description:    args.Description,
        ActiveForm:     args.ActiveForm,
        Status:         "pending",
        BlockedBy:      args.BlockedBy,
        Metadata:       args.Metadata,
        CreatedAt:      time.Now().UTC(),
        UpdatedAt:      time.Now().UTC(),
    }
    if err := t.repo.Save(ctx, task); err != nil {
        return "", fmt.Errorf("TaskCreate: save: %w", err)
    }
    t.bridge.Publish(ctx, convID, eventsdomain.TaskCreated{Task: task})

    return fmt.Sprintf(`Task created: %s ("%s")`, task.ID, task.Subject), nil
}
```

#### 6.5 实施估时

| 工具 | 估时 |
|---|---|
| 4 个 tool（schema + Execute） | 0.5 天 |
| domain + store + repository (含 50 上限) | 0.3 天 |
| SSE 事件家族 + 前端契约 | 0.2 天 |
| 测试 (4 tool × 主路径 + edge case) | 0.3 天 |
| **合计** | **~1.3 天** |

注意：前端 todo panel UI 不在本估时。

---

## TodoWrite —— legacy，仅供对照

CC v2.1.16+ 用 5 个细粒度 Task* tool 取代了 1 个 TodoWrite。但 TodoWrite description 仍保留供 **Agent SDK / 非交互模式**用。

为何研究这个 legacy？因为：

1. **TodoWrite 描述含 4 个详细 example + 4 反 example + reasoning** —— Anthropic 自己的"何时该用 todo / 何时不该用"教材，是 **prompt engineering 范本**
2. TaskCreate 描述继承了 TodoWrite 的核心规矩（One-in-progress / IMMEDIATELY mark / FULLY accomplished）但**砍掉了所有 example** —— 说明 v2.1.84 起 Anthropic 觉得 example 不再必要（模型够强）
3. Forgify 自己写 TaskCreate description 时，可以**酌情借用 TodoWrite 的 example**——尤其针对小模型用户群

TodoWrite 的 4 example 主题：
- 多步骤 feature（dark mode 实施）
- 跨文件 rename
- 多 feature 并行（user reg + 商品 + 购物车 + 结账）
- 性能优化清单

TodoWrite 的 4 反 example 主题：
- 单步任务（"How do I print Hello World?"）
- 信息查询（"What does git status do?"）
- 单文件单点改动（加一个注释）
- 单条命令执行（npm install）

**Forgify 决策**：用 Task* 系列细粒度（不是 TodoWrite 一锅）；description 借 TodoWrite 的"何时该用 / 何时不该用"清单，**不抄 example**——保持 description 紧凑。

---

## EnterPlanMode / ExitPlanMode（P2 简评）

### EnterPlanMode 核心要点

> Use this tool **proactively** when you're about to start a non-trivial implementation task. Getting user sign-off on your approach before writing code prevents wasted effort and ensures alignment.

**何时用**（CC 列了 7 个场景）：
1. 新 feature 实施
2. 多种合理实现方案
3. 改动现有行为
4. 架构决策
5. 改 2-3+ 文件
6. 需求模糊需先探索
7. 用户偏好相关

**plan mode 行为**：
- 进入后变只读：禁 Edit / Write / Bash 等写操作
- 仍可 Read / Glob / Grep / Agent 探索
- 探索完毕用 ExitPlanMode 提交 plan 求批准
- 用户拒绝 → 留在 plan mode；用户批准 → 出 plan mode 开始执行

### ExitPlanMode 核心要点

> Use this tool when you are in plan mode and have finished writing your plan to the plan file and are ready for user approval.

- **不接收 plan 内容作为参数**——读 plan mode 系统消息里指定的 plan 文件
- 调用 = 信号"我准备好了"，等用户审批

### Forgify 决策：P2 不实施

**理由**：
- Plan mode 是为"代码实施型对话"特化的——CC 的目标用户是 coder
- Forgify 是 workflow orchestration——"建 workflow" / "改 forge"已经有自己的 pending/accept lifecycle（详见 forge.md）
- 引入 plan mode 会跟 forge pending 概念重叠 / 冲突
- v1 不做；Phase 5+ 视用户需求评估

如未来要做：可以做"workflow draft mode"——LLM 出工作流方案，用户在 UI 编辑+批准，再开始执行。这是更符合 Forgify 定位的版本。

---

## TaskStop / TaskOutput / SendUserMessage / TodoWrite —— 跳过

| 工具 | 跳过理由 |
|---|---|
| TaskStop | 配 Bash `run_in_background` 用；Forgify Bash 简化版的 background 也只是 fire-and-forget 写日志，没"任务"实体可 stop。要 cancel 直接 `cancel(ctx)` |
| TaskOutput | CC 自己 deprecated，推荐改 Read on output file path |
| TodoWrite | legacy；用 Task* 系列代替 |
| SendUserMessage | Forgify 已有 chat.message SSE 双向通道；这个 tool 只是单向 push 通知，跟现有事件流重叠 |

---

## 跨 tool 共享

- **AskUserQuestion** 阻塞机制 + **TaskCreate 族** 的 SSE 事件家族都需要扩展现有 `domain/events/types.go`——可以一并设计
- 二者都 `RequiresWorkspace = false`（不操作文件系统）
- TaskCreate 的 50 上限 + AskUserQuestion 的 30 分钟 timeout 是反"agent 失控"的护栏

---

## 总结：本批实施估时

| 工具 | 估时 | 难点 |
|---|---|---|
| AskUserQuestion | **0.5 天**（仅后端：endpoint + 挂起 channel + tool；复用 chat.message SSE，无新事件类） | 挂起 channel 生命周期 + 前端契约（前端实施不在本轮） |
| TaskCreate 族（4 in 1） | **1.3 天**（含 domain/store/SSE/test） | 50 上限 + 状态机 + dependencies 解析 |
| Plan mode（P2，不做） | 0 | — |

**合计 ~2.3 天**——是 5 篇里第二大的（仅次 shell 1.5 天）。

---

## 信任度总结

- ✅ **多源确认**：AskUserQuestion description + preview field + plan mode 子规则；TaskCreate description 完整 + Task 状态机 + One-in-progress 规矩；TodoWrite 4+4 example 完整；EnterPlanMode 7 use case + ExitPlanMode 行为
- ⚠️ **单源 / 推断**：AskUserQuestion 的 schema 字段精确名（社区推断；可能 CC 内部叫 `multi_select` 而非 `multiSelect`）；TaskCreate 的 metadata / blockedBy 是 array vs string 的具体格式；TaskList 输出格式精确 wire；Task* 50 上限是否硬性
- ❌ **无法验证**：teammate workflow 的实际同步机制（CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS 才能开）；plan 文件的精确位置（"plan 文件" 内部细节）

---

## 02-tools-deep 系列总结

5 篇 deep-dive 写完。覆盖：

| 篇 | 工具 | 实施估时 |
|---|---|---|
| 01-file-ops | Read / Write / Edit | 1.5 天 |
| 02-search | Grep / Glob / LS（如保留） | 0.65–0.85 天 |
| 03-shell | Bash + Monitor 设计走查 | 1.5 天 |
| 04-web | WebFetch / WebSearch | 0.9 天 |
| 05-ux-tasks | AskUserQuestion + TaskCreate 族 | 2.3 天 |
| **合计** | **15 个 tool** | **~7 天** |

跳过的（已下线 / off-mission / P2）：MultiEdit / NotebookEdit / TodoWrite / TaskOutput / TaskStop / Skill / EnterPlanMode / ExitPlanMode / SendUserMessage / EnterWorktree / ExitWorktree / Computer / BrowserBatch / LSP / TeamCreate / TeamDelete / SendMessage / PowerShell / Cron* / Agent / mcp__\* / ToolSearch / ListMcpResourcesTool / ReadMcpResourceTool / Monitor（设计已记录但不实现）

**下一步建议**：
- 不再继续 deep-dive（剩下的 tool 已经 P2 / Skip）
- 转入实施阶段——按 01 → 02 → 04 → 03 → 05 顺序更稳（小 → 大、低风险 → 高风险）
- 或者：先建 Phase 5 的 `workspace whitelist` + `AgentState.SeenFiles` 等基础设施，再开始铺 tool

实施前最后一步：把 inventory + 5 篇 deep-dive 一起翻一遍，对照现有代码定 milestone。
