# Ask — V1.2 详设计

**Phase**：5（System Tool 第二代 ux 批次 / 与 task 同 batch 落地）
**状态**：✅ 实现完成（2026-05-04，U2-U3）
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/api-design.md`](../service-contract-documents/api-design.md) — `POST /api/v1/conversations/{id}/answers` 端点
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — ASK_NO_PENDING_QUESTION ×3
- [`./chat.md`](./chat.md) §4.4 — 系统工具完整目录
- 实现包：`backend/internal/app/ask/`（Service）+ `backend/internal/app/tool/ask/`（Tool）+ `backend/internal/transport/httpapi/handlers/answers.go`（HTTP）

---

## 1. 一句话

LLM 中途**暂停 agent loop 等用户回答**的交互机制。无 entity / 无持久化——`app/ask.Service` 持有 in-memory 会合 map（`toolCallID → channel`），工具 Wait 阻塞 + 用户走 `POST /answers` 投递时 Resolve 原子摘条目唤醒。问题本身**坐 chat.message SSE 流**（决策 D11：不新建事件家族），AskUserQuestion tool_call block 的 arguments 含 `question` + `options`。

> **历史**：v1 设计时 ask 与 task 同 batch 完成，最初一并写在 [`task.md §10`](./task.md)。Tool 自检 batch 5 把它单独抽出来对照其它 5 个 tool 家族（filesystem / search / web / shell / ask）的独立 design doc 模式。

---

## 2. 端到端推演（设计原则 #5）

```
触发源：LLM 在 chat agent 循环里调 AskUserQuestion(question, options?)
  → transport 层（tool 调用本身不走 HTTP；但答案投递走）
    → app 层：app/tool/ask.AskUserQuestion.Execute
        → reqctxpkg.GetToolCallID(ctx) → callID（chat/tools.go::runOneTool 已注入）
        → svc.Wait(ctx, callID, 5min)
            ↓ 注册 chan
            ↓ 阻塞（agent loop 暂停）

  -- 同时 --
  chat.message SSE 流已经把 tool_call block 推给前端
  （block.arguments 含 question + options，UI 渲染问题）

  ┌────────── 用户在 UI 点选/输入答案 ──────────┐
  ↓                                              ↑
  POST /api/v1/conversations/{id}/answers         ↑
  body: {toolCallId, answer}                      ↑
    → handlers/answers.go::AnswerHandler.Submit   ↑
    → askapp.Service.Resolve(toolCallID, answer)  ↑
        ↓ 持锁原子地：delete(map, ID) + send to chan ↑
        ↓ 返回 nil                                ↑
    → 204 No Content                              ↑

  ↑ chan 被填充 → svc.Wait 解锁返答案 ↑
  → 工具 Execute 返答案为 tool_result
  → chat.message SSE 推 tool_result block（agent loop 继续）
```

**端到端跨 domain 依赖**：
- `pkg/reqctx.GetToolCallID` — chat/tools.go::runOneTool 在 Execute 前注入
- `transport/httpapi/router/deps.AskService` — main.go 装 Service 单例
- handler **不**校验 path 中 conv 存在 / 不校验 callID 属于该 conv（**反校验剧场**：单用户场景下过度设计——会合 key 是 callID，conv 仅 routing 分组）
- 无 DB / SSE 自有事件 / Repository — Service 是纯 in-memory

---

## 3. 关键决策

| 决策 | 选择 | 理由 |
|---|---|---|
| **D11：问题坐 chat.message** | 不新建 ask 事件家族 | AskUserQuestion 的 tool_call block arguments 已经含 `question` + `options`；chat.message 已经流转 UI 需要的一切；新事件家族 = 前端多一种渲染逻辑 + wire 协议复杂化，无收益 |
| 持久化 | **无** — 内存 map | backend 重启后所有 pending 会丢(用户视角:agent 卡住的话刷新页面也救不回来,需要重新触发);in-memory 会合最简单 |
| 双答防护 | Resolve **持锁原子**地 `delete(map, ID) + send to chan` | buffered chan cap=1 + 删条目让第二次 Resolve 必走 `ErrNoPendingQuestion`（无竞态）。**Tool 自检 batch 1 之前**用过 select default 兜底但有 race window，已重构为原子摘条目 |
| 重复注册防护 | Wait 检查 `pending[toolCallID]` 已存在 → 报 caller bug | 同 callID 注册两次会静默覆盖前一个 chan；显式报错让接线 bug 暴露 |
| **超时(2026-05 #6 重构)** | **7 天** (基本 = 不限,只防 zombie 进程) | 真用户 UX **不**该依赖超时——用户可能离屏过夜。frontend Composer 状态机 + sticky 横幅负责"等答题"感知 + 把新消息自动 route 成答案,而不是靠 timeout 让 agent 自救。原 5min 太短(burn-in #6 撞坑)。**Tool description 已说明超时不该被业务依赖** |
| **options 字段(2026-05 #6)** | 可选 | 有 options → 结构化按钮;无 options → 开放式问,Composer 给自由输入。LLM tool description 显式说明两种用法 |
| **skipped 语义(2026-05 #6)** | `POST /answers` body 加 `skipped:bool` | 用户主动 skip → backend 把答案替换成哨兵 `"(user skipped)"` 喂给 agent,agent 自决合理 default。frontend Composer 的 "Skip" 按钮即此 |
| Cancel 语义 | ctx.Done → 返 ctx.Err() | 用户在对话里点取消时 LLM 看到"问题已取消（对话被打断）"|
| Tool IsReadOnly | true | 不修改任何持久化状态；从 LLM 视角看是只读（拿到答案）；从世界视角看也无副作用 |
| RequiresWorkspace | false | 不碰文件系统 |
| HTTP 端点 RESTful | `POST /api/v1/conversations/{id}/answers` body 含 toolCallId + answer | path 里 conv-id 用于 routing 分组 / 未来日志审计；callID 是真正会合 key（**反校验剧场**：当前不强制校验 callID 属于该 conv，单用户场景过度设计）|

---

## 4. Service 层（`backend/internal/app/ask/ask.go`）

```go
type Service struct {
    mu      sync.Mutex
    pending map[string]chan string   // toolCallID → buffered chan(cap 1)
}

func NewService() *Service
func (s *Service) Wait(ctx context.Context, toolCallID string, timeout time.Duration) (string, error)
func (s *Service) Resolve(toolCallID, answer string) error
```

### 4.1 Wait 语义

```go
func (s *Service) Wait(ctx, toolCallID, timeout) (string, error) {
    ch := make(chan string, 1)  // buffered: Resolve 永不阻塞 send

    s.mu.Lock()
    if _, exists := s.pending[toolCallID]; exists {
        s.mu.Unlock()
        return "", errors.New("ask: tool_call_id already pending — caller bug")
    }
    s.pending[toolCallID] = ch
    s.mu.Unlock()

    defer s.cleanup(toolCallID)  // 兜底删条目（即便 Resolve 已删也幂等）

    timer := time.NewTimer(timeout)
    defer timer.Stop()

    select {
    case ans := <-ch:
        return ans, nil
    case <-timer.C:
        return "", ErrTimeout
    case <-ctx.Done():
        return "", ctx.Err()
    }
}
```

### 4.2 Resolve 语义（**原子摘条目**）

```go
func (s *Service) Resolve(toolCallID, answer string) error {
    s.mu.Lock()
    ch, ok := s.pending[toolCallID]
    if ok {
        delete(s.pending, toolCallID)  // ← 持锁原子摘条目
    }
    s.mu.Unlock()
    if !ok {
        return ErrNoPendingQuestion
    }
    ch <- answer  // buffered cap=1 + 已删条目 → send 永不阻塞，无第二个 Resolve 能 race
    return nil
}
```

**关键正确性**：双答防护**不**靠"chan 满 → select default"那种 race 窗口模式。**先持锁删 + 再 send**——第二个 Resolve 看到 map 里没条目，直接走 `ErrNoPendingQuestion`。无竞态。

### 4.3 Sentinel 错误

```go
var (
    ErrNoPendingQuestion = errors.New("ask: no pending question for that tool_call_id")
    ErrTimeout           = errors.New("ask: user did not respond within the timeout")
)
```

- `ErrNoPendingQuestion` → 404 / `ASK_NO_PENDING_QUESTION` — 涵盖"未注册 / 已超时 / 已答 / 拼错"四种情况；二次 Resolve 因原子摘条目也走此路径
- `ErrTimeout` → 504 / `ASK_TIMEOUT` — 仅 Service 内部抛；当前工具 Execute 转友好字符串而非上抛 handler，因此**实际不到 handler**

> 历史 `ErrAlreadyAnswered` sentinel 已删——双答场景被 `ErrNoPendingQuestion` 完全覆盖（Resolve 原子摘条目后第二次必无条目可见）。

---

## 5. 工具规约（`backend/internal/app/tool/ask/ask.go`）

### 5.1 AskUserQuestion

**Args**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `question` | string | ✅ | 问题文本 |
| `options` | string[] | | 建议选项（UI 可渲染按钮；用户**不**受限于这些）|

**返回**（成功）：用户答案字符串。

**返回**（失败 — 全部友好字符串，**不**上抛 Go err）：
- ctx 缺 toolCallID → `Cannot ask the user: tool runtime is not properly initialized.`（调用方 defect；operator 经 executeTool warn log 看到原栈）
- 超时 → `User did not respond within the timeout.`
- ctx cancel → `Question cancelled by the user.`
- 其他 err → `Asking the user failed: <err>`（err.Error() 经 framework boundary 清洗 §S16 wrap 前缀）

**静态元数据**：`IsReadOnly=true` / `NeedsReadFirst=false` / `RequiresWorkspace=false`

**ValidateInput** sentinel：
- `ErrEmptyQuestion` — question 缺 / 空 / 仅空白

**默认超时**(2026-05 #6 重构):`defaultTimeout = 7 * 24 * time.Hour`(7 天,基本=不限,只防 zombie 进程)。可在 Service 构造时通过 `tool.timeout` 字段在测试中覆盖到 500ms 加速测试。Frontend Composer 状态机负责"等答题"UX,不依赖此 timeout 自救。

### 5.2 AskTools 工厂

```go
// app/tool/ask/ask.go
func AskTools(svc *askapp.Service) []toolapp.Tool {
    return []toolapp.Tool{
        &AskUserQuestion{svc: svc, timeout: defaultTimeout},
    }
}
```

调用方按 §S13 嵌套子包别名规则导入为 `asktool`。

---

## 6. HTTP API

### `POST /api/v1/conversations/{id}/answers`

**Request body**：
```json
{
  "toolCallId": "call_xyz",
  "answer": "yes please"
}
```

**Response**：
- `204 No Content`（成功投递）
- `400 INVALID_REQUEST`（缺字段）
- `404 ASK_NO_PENDING_QUESTION`（toolCallId 未在 Wait — 已超时 / 已答 / 拼错）

**Handler**（`backend/internal/transport/httpapi/handlers/answers.go`）：

```go
func (h *AnswerHandler) Submit(w, r) {
    // path 含 {id} 但 handler 不读不校验（反校验剧场——单用户 + 同人写前后端）。
    var req answerRequest // {ToolCallID, Answer}
    if err := decodeJSON(r, &req); err != nil {
        responsehttpapi.FromDomainError(w, h.log, err)
        return
    }
    if req.ToolCallID == "" {
        responsehttpapi.Error(w, http.StatusBadRequest, "INVALID_REQUEST",
            "toolCallId is required", nil)
        return
    }
    if err := h.svc.Resolve(req.ToolCallID, req.Answer); err != nil {
        responsehttpapi.FromDomainError(w, h.log, err)  // ErrNoPendingQuestion → 404
        return
    }
    responsehttpapi.NoContent(w)
}
```

### Wire shape 注脚

**Path 里 conv-id 但 body 里 callID** — 这是 RESTful 路由分组的需要（routing / 未来审计），但实际会合 key 是 callID。**当前不校验 callID 属于该 conv**——单用户场景下过度设计。LLM 不知道这层 callID-conv 弱关联，永远在自己的 chat.message context 里看到 tool_call_id 直接用即可。

---

## 7. 错误码（`transport/httpapi/response/errmap.go`）

| Sentinel | HTTP | Wire Code |
|---|---|---|
| `askapp.ErrNoPendingQuestion` | 404 | `ASK_NO_PENDING_QUESTION` |
| `askapp.ErrTimeout` | 504 | `ASK_TIMEOUT` |

> 仅 `ASK_NO_PENDING_QUESTION` 真正会到达 handler（覆盖 4 种缺失场景）；`ASK_TIMEOUT` 保留登记仅为未来若改语义时不需重排映射。`ErrAlreadyAnswered`（历史 sentinel）已删——双答被 `ErrNoPendingQuestion` 完全覆盖。

---

## 8. 实现要点

### 8.1 Tool ↔ Service 解耦

Tool 层**只**调 `svc.Wait(ctx, callID, timeout)`——不接触 map / chan / mutex。Service 层**只**做会合（Wait + Resolve + cleanup）——不知道 chat / SSE / HTTP。这种分层让：
- Tool 层可单独测试（`tool.timeout` 缩到 500ms，goroutine Resolve）
- Service 层可单独测试（直接 Wait + Resolve 不经过 tool）
- HTTP handler 可单独测试（mock svc）

### 8.2 Cleanup 幂等性

```go
func (s *Service) cleanup(toolCallID string) {
    s.mu.Lock(); defer s.mu.Unlock()
    delete(s.pending, toolCallID)  // 不存在也无副作用
}
```

`Wait` 用 `defer s.cleanup(toolCallID)`：
- 答案到达 → Resolve 已经 delete，cleanup 是 no-op ✓
- 超时 / cancel → cleanup 删条目 ✓
- 重复注册被拒（早 return）→ cleanup **没注册** 时也无害 ✓

### 8.3 与 chat.message 协同

agent 层 `runOneTool`（chat/tools.go）流程：
1. 流出 tool_call block（含 `question` / `options` 在 arguments map 里）→ chat.message 推给前端
2. 进入 `executeTool` → `Validate → CheckPermissions → Execute`
3. Execute 内 `svc.Wait` 阻塞 → agent goroutine 暂停（不再生成 tool_result）
4. 用户答 → Resolve → Wait 返答案 → tool_result block 流出 → chat.message 再推一帧
5. agent loop 继续

**关键**：tool_call block 的 chat.message 推送在 `svc.Wait` 之**前**。前端拿到 chat.message 看到 tool_call.name=AskUserQuestion → 渲染问题 UI。

---

## 9. 测试覆盖

| 层 | 文件 | 测试数 | 覆盖 |
|---|---|---|---|
| Service | `backend/internal/app/ask/ask_test.go` | 7 | Wait/Resolve happy round-trip / Wait timeout / Wait ctx cancel / Resolve unknown ID 报 ErrNoPendingQuestion / **双答必报 ErrNoPendingQuestion**（原子摘条目）/ 同 callID 重复 Wait 拒 / 50 并发 Wait+Resolve 注册表清理 |
| Tool | `backend/internal/app/tool/ask/ask_test.go` | 12 | identity / 静态 metadata / schema / AskTools 工厂返单工具 / Validate × 2 / Execute 缺 toolCallID 友好 / 答案到达 / 超时（~500ms 元）/ ctx cancel |
| Handler | （在 `transport/httpapi/handlers/` 集成测试覆盖）| — | 见 transport 层契约测试 |
| Pipeline | `backend/test/uxtodo/uxtodo_test.go::TestUxTodo_AskUserQuestionAnswerDelivered` + `_AnswerEndpoint_UnknownCallID_404` | 2 场景 | 端到端旁路 goroutine POST 答案验真实接线 + 404 |

合计 **19 单测 + 2 pipeline 场景**。

---

## 10. 与其他 domain 的关系

| 关系 | 说明 |
|---|---|
| **chat** | tool_call block 在 chat.message 流里携带 question / options；执行流暂停 / 恢复都由 chat ReAct loop 调度 |
| **task** | 同 batch 落地（U2-U3）；task.md §10 旧版完整描述本 domain，本批 5A 重构期已迁出 + task.md 改为指向 ask.md |
| **events / SSE** | **不发自有事件**（决策 D11）；信息全走 chat.message |
| **transport** | 独立 HTTP 端点 `/answers`；handler 在 `answers.go`；router/deps.go 加 `AskService *askapp.Service` 字段 |
| **errmap** | 3 sentinel 全登记（详 §7）|
| **agentstate** | 不依赖 — Wait/Resolve 跟对话级状态无关，仅按 toolCallID 会合 |

---

## 11. 演化方向

- **持久化历史**：当前问题超过 5 分钟即丢；用户回头看历史对话时看不到"曾问过 X"。可加把 question + answer 落到 chat.message 持久 store（已经在了，因为 tool_call + tool_result block 落库），无需新表
- **多选 / 单选 schema**：当前 `options` 是普通 string array，UI 自由渲染；未来可加 `kind: "single"|"multi"|"text"` 字段让前端区分 radio / checkbox / textarea
- **答案 schema 校验**：当前用户能传任意字符串；未来若 LLM 想限定（如"只接受数字"）需在 schema 加 type
- **同 turn 多 ask 并发**：当前 LLM 同 turn 起多个 AskUserQuestion 时各走独立 callID 的 Wait（互不干扰）；前端 UI 需要能同时渲染多个问题（v1 假设单问题，可改进）
- **Cancel 单 question**：当前 ctx cancel 会取消所有 pending Wait（整 turn 取消）；未来加 `POST /answers:cancel` 端点让用户单独 dismiss 一个问题
- **超时调整**：当前固定 5 分钟；未来可让 LLM 通过 schema 字段指定（"等 30 分钟" / "等到结束"）
