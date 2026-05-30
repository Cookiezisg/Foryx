# Compaction — 对话上下文压缩

**Phase**：V1.2 §1 final-sweep（与 memory 同批落地）

> **🔧 限制优化（2026-05-31，limits-optimization）**：nil `CapabilityResolver` 时一次性 WARN（防大模型被按 32K 兜底窗口过早压缩——文档自标的"严重 bug"）；上下文边界改由 token 预算 + compaction 投影主导，`chat.buildHistory` 对 archived 消息（含 user）统一跳过。详 [`../adhoc-topic-documents/limits-optimization/`](../adhoc-topic-documents/limits-optimization/)。
**状态**：✅ 部分实现（2026-05-30：ContextManager 骨架 + 窗口感知压缩 + chat goroutine 竞态修复；fullCompact LLM 调用 + 块降级 + SSE 推流为设计期，待后续完整实现）
**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../event-log-protocol.md`](../event-log-protocol.md) — **本设计加 1 种 block type（`compaction`，第 7 种）+ 给 block.Content 改写加豁免条款** ⚠️
- [`../service-contract-documents/database-design.md`](../service-contract-documents/database-design.md) — conversations 加 2 列，message_blocks 加 1 列
- [`../service-contract-documents/error-codes.md`](../service-contract-documents/error-codes.md) — compaction ×1 sentinel
- [`./memory.md`](./memory.md) — Memory 作为"逃生通道"
- [`./chat.md`](./chat.md) — `runner.buildSystemPrompt` + `loop/history.BlocksToAssistantLLM` 双改造点

---

## 1. 一句话

对话变长时，**用一段持续维护的摘要（`conversation.summary`）替代老的 block**，让 LLM 看到的 history 始终在阈值内。**老 block DB 里永远完整保留**，只是不发给 LLM。

**核心 schema 改动 = 2 列 + 1 种新 block type**：
- `conversations.summary` + `conversations.summary_covers_up_to_seq`（持续维护的摘要）
- `message_blocks.context_role`（hot/warm/cold/archived）
- 新 block type `compaction`（事件日志协议第 7 种）

**核心 service = 1 个 ContextManager**（~150 行），AI turn 完成后跑一次，按规则做"降级 + 必要时叫 cheap LLM 写摘要"。

---

## 2. 端到端推演（设计原则 #5）

### 平时（70% 以下）

```
LLM turn 跑完（chat/host.go::WriteFinalize 末尾）
  ↓
contextmgr.Manager.MaybeCompact(ctx, convID)
  ↓
estimate tokens（tiktoken-go + 实际 usage 校准）
  ↓
< 70% → return 啥也不做
```

### 70-85%（规则降级）

```
> 70% → demoteOldBlocks（纯规则，0 LLM）
  ↓
倒序扫所有 tool_result block：
  • 最近 5 个 → 不动（hot）
  • 第 6-15 个 → context_role = "warm"
  • 第 16+ 个 → context_role = "cold"
  ↓
text / reasoning / tool_call 不主动降级
  ↓
重算 token，若降到阈值下 → return
```

### 85%+（真压缩，叫 LLM）

```
> 85% → fullCompact
  ↓
1. emit compaction block_start（type=compaction，status=streaming）
   ──→ 前端立刻收到 SSE，渲染 "Compacting..." spinner
  ↓
2. 收集要 archive 的 blocks：seq <= (latest_seq - recentTurnsKeep)
   且未被之前的 compaction 覆盖（seq > summary_covers_up_to_seq）
  ↓
3. 构造 prompt：previous summary + new blocks → anchored merge
  ↓
4. cheap LLM call（utility scenario：autoTitle / search rerank 等共享一档；典型配 Haiku / 4o-mini / DeepSeek 小快省档）
   返回 newSummary
  ↓
5. 一个事务里：
   • UPDATE conversations SET summary=?, summary_covers_up_to_seq=?
   • UPDATE message_blocks SET context_role='archived' WHERE id IN (...)
   • UPDATE message_blocks SET content=?, status='completed' WHERE id=<compaction_block_id>
  ↓
6. emit compaction block_stop（completed）
   ──→ 前端 spinner 消失，summary 内容渲染到 compaction 卡片
```

### 拼 LLM history（每次 LLM 调用前）

```
loop/history.go::BlocksToAssistantLLM(blocks)
  ↓
按 context_role 投影：
  conversation.summary    → 作为一条 system-like 消息塞在历史最前
  archived blocks         → 跳过（不发）
  cold blocks             → 仅元数据 "Read foo.py (4.2KB) at 14:23"
  warm blocks             → 前 200 字符 + "[truncated, NN KB total]"
  hot blocks              → 完整 content
```

**端到端跨 domain 依赖**：
- `chat/runner.go` 在 turn 完成后调 `ContextManager.MaybeCompact`
- `loop/history.go::BlocksToAssistantLLM` 改造按 context_role 投影
- `pkg/eventlog.Emitter` 用现有协议推 compaction block（新 type 加进白名单即可）
- `app/memory/Service` 作为逃生通道：压缩前扫"该写进 memory 的事"（详 §10）

---

## 3. 设计原则

| 原则 | 落地 |
|---|---|
| **Anchored append, never rewrite** | summary 是累积式，每次 LLM 调用只**追加新 bullets**，旧 bullets 保留 |
| **DB 永远完整** | block 内容永不删，只改 context_role 标记。GET /blocks/{id} 永远返完整 |
| **触发时机：turn 完成后** | 不阻断 LLM mid-turn；用户当前对话流畅 |
| **不阻塞用户** | "Compacting..." 是 block streaming 状态，对话 queue 自然 serial 等 |
| **Reversible vs Lossy** 区分 | tool_result（可 re-call）放心剔；text/decision 必须先入 summary 才能 archive |
| **Recent K turns 永远 hot** | 保 LLM "节奏"（Anthropic 推荐）：最近 3 轮全 hot，最近 5 个 tool_result 全 hot |
| **Pinned 永远 hot** | 用户首条 message / active Skill activate / 当前 todo 状态变更 → 自动 pin → 永不降级 |
| **Memory 是逃生通道** | 压缩前扫 archive 候选，识别"用户偏好/长期事实"提议写进 §2 memory |
| **单 LLM 调用**（不重试）| 失败就退回纯规则降级；下一轮再试。**没有 retry loop**——下一次 turn 自然就是 retry |
| **不分多 tier**（之前砍掉 5-tier 设计）| 就两路径：规则降级 + 真压缩。心智模型简单 |

---

## 4. 数据模型变更

### 4.1 `conversations` 加 2 列

```sql
ALTER TABLE conversations ADD COLUMN summary TEXT DEFAULT '';
ALTER TABLE conversations ADD COLUMN summary_covers_up_to_seq INTEGER DEFAULT 0;
```

| 字段 | 说明 |
|---|---|
| `summary` | 持续维护的对话摘要（markdown bullets，anchored append）。覆盖 archived blocks。空 = 还没压缩过 |
| `summary_covers_up_to_seq` | 该 summary 覆盖到哪个 block seq。下次压缩从 seq+1 开始算 |

### 4.2 `message_blocks` 加 1 列

```sql
ALTER TABLE message_blocks ADD COLUMN context_role TEXT DEFAULT 'hot'
    CHECK (context_role IN ('hot','warm','cold','archived'));
CREATE INDEX idx_mb_conv_role ON message_blocks (conversation_id, context_role);
```

| 值 | 发给 LLM | 谁会是这个 role |
|---|---|---|
| **hot** | 完整 Content | 默认；最近 3 轮的所有 block；pinned 的 block |
| **warm** | 前 200 字符 + 截断标记 | 中段 tool_result（reversible 类） |
| **cold** | 仅元数据（tool 名 + 大小 + 时间）| 老一点的 tool_result |
| **archived** | **完全不发**（summary 已覆盖）| 已被 ContextManager 摘要进 conversation.summary |

### 4.3 新 block type：`compaction`

**§S21 invariants 影响**：现有 6 种 block type 是封闭枚举（text/reasoning/tool_call/tool_result/progress/message）。本设计加第 7 种 `compaction`。

**改动列表**（必须同 PR 完成）：
1. [`event-log-protocol.md`](../event-log-protocol.md) §block-types 加 `compaction` 行
2. `domain/eventlog/eventlog.go::IsValidBlockType` switch 加 `BlockTypeCompaction`
3. `domain/chat/chat.go::Block` 的 `Type` CHECK 约束加值
4. `domain/eventlog/eventlog.go` 加常量 `BlockTypeCompaction = "compaction"`
5. testend 前端 BlockView 加 `compaction` 渲染分支

**compaction block 形态**：

| 字段 | 值 |
|---|---|
| `ID` | `blk_<16hex>` 标准 ID |
| `ConversationID` | 所属对话 |
| `MessageID` | 一个"虚拟" message（type=system？或者就挂在最近一个 assistant message 下）|
| `ParentBlockID` | 空（顶层 block）|
| `Type` | `"compaction"` |
| `Content` | 摘要的 markdown 文本（生成时流式 delta 进来）|
| `Attrs` | JSON：`{coversFromSeq, coversToSeq, blocksArchived, generatedBy, tokensSaved}` |
| `Status` | `streaming` → `completed` / `error` |
| `Seq` | 全局 seq（跟其他 block 共用）|

**为啥用 block 而不是新 SSE 事件**：保持 §E1"SSE 上限三条永远不再加"——compaction 走现有 eventlog 流，前端无需新订阅。

### 4.4 §S21 invariants 豁免条款

当前规则：`block.Content` / `block.Attrs` 是 append-only，`block.Status` 单向流转。

**新增豁免**：

```
ContextManager 可以**改写**已 completed block 的 Content 字段（用于
"老 tool_result 替成 placeholder 字符串"）—— 这是 server-side 投影优化，
不属于"内容流转"，不破坏 §S21 invariants:
  • status 不变（仍 completed）
  • parent/message/seq 不变
  • 改写是单向的（hot → warm → cold → archived，不会反向）

实际实现里，**永远不真的改 DB 的 Content**——而是在 BlocksToAssistantLLM
投影时按 context_role 替换给 LLM 看的字符串。DB Content 始终是原文。

✅ 这条满足 §S21 不变性：DB 永远是 single source of truth + append-only。
```

---

## 5. ContextManager（`internal/app/contextmgr/`）

### 5.1 文件结构

```
app/contextmgr/
  contextmgr.go   ← Manager struct + New + MaybeCompact 入口
  demote.go       ← demoteOldBlocks（纯规则降级）
  compact.go      ← fullCompact（叫 LLM）
  estimate.go     ← Token 估算 + 校准（基于 tiktoken-go + 上次 usage）
  prompt.go       ← compactPromptTemplate（anchored merge prompt）
```

### 5.2 Manager struct

```go
type Manager struct {
    repo                chatdomain.Repository
    convRepo            convdomain.Repository
    cheapLLM            llmclientpkg.Resolver  // 装配阶段注入：闭包包 ResolveUtility(picker, keys, factory)（utility scenario）
    em                  eventlogpkg.Emitter    // 推 compaction block 用
    memory              memoryapp.Service      // 逃生通道（§10）
    capabilityResolver  CapabilityResolver     // 2026-05-30：注入 per-model 真实窗口；nil 时 fallback 全局常量
    log                 *zap.Logger
    
    // 阈值（per-conv 可配，默认全局）
    softThreshold float64  // 0.70  → demote
    hardThreshold float64  // 0.85  → fullCompact
    recentKeep    int      // 3     → 最近 N 轮永远 hot
}
```

### 5.3 主入口：`MaybeCompact`

每次 AI turn 跑完后调一次（**唯一入口**）：

```go
func (m *Manager) MaybeCompact(ctx context.Context, convID string) error {
    usable, used, err := m.estimate(ctx, convID)
    if err != nil { return err }
    ratio := float64(used) / float64(usable)
    
    if ratio < m.softThreshold {
        return nil  // 啥也不做（90% 的 turn 走这里）
    }
    
    // 规则降级（先跑，可能省 LLM）
    saved := m.demoteOldBlocks(ctx, convID)
    if saved > 0 {
        ratio = float64(used-saved) / float64(usable)
    }
    
    if ratio < m.hardThreshold {
        return nil  // demote 够了
    }
    
    // 真压缩
    return m.fullCompact(ctx, convID)
}
```

### 5.4 `demoteOldBlocks`（纯规则，0 LLM）

```go
func (m *Manager) demoteOldBlocks(ctx context.Context, convID string) int64 {
    blocks, _ := m.repo.ListBlocksByConversation(ctx, convID)
    
    // 倒序遍历，per-type 计数
    toolResultIdx := 0
    var savedBytes int64
    
    for i := len(blocks) - 1; i >= 0; i-- {
        b := blocks[i]
        
        // Pinned / 已 archived 不动
        if b.Attrs["pinned"] == true || b.ContextRole == "archived" {
            continue
        }
        
        // 最近 K turns 不动（看 message position，不是 block position）
        if m.isWithinRecentTurns(blocks, i, m.recentKeep) {
            continue
        }
        
        if b.Type == "tool_result" {
            toolResultIdx++
            var newRole string
            switch {
            case toolResultIdx <= 5:
                continue  // 最近 5 个 tool_result 不动
            case toolResultIdx <= 15:
                newRole = "warm"
            default:
                newRole = "cold"
            }
            if b.ContextRole != newRole {
                m.repo.UpdateBlockRole(ctx, b.ID, newRole)
                savedBytes += estimateSaving(b, newRole)
            }
        }
        // text / reasoning / tool_call 不主动降级
    }
    return savedBytes
}
```

### 5.5 `fullCompact`（叫 LLM）

```go
func (m *Manager) fullCompact(ctx context.Context, convID string) error {
    conv, _ := m.convRepo.Get(ctx, convID)
    
    // 1. emit compaction block_start
    msgID := m.ensureSystemMessage(ctx, convID)
    blockID := m.em.StartBlock(ctx, "compaction", map[string]any{
        "coversFromSeq": conv.SummaryCoversUpToSeq + 1,
    })
    
    // 2. 收集 archive 候选
    candidates := m.collectArchiveCandidates(ctx, convID, conv.SummaryCoversUpToSeq)
    if len(candidates) == 0 {
        m.em.StopBlock(ctx, blockID, "completed", nil)
        return nil
    }
    
    // 3. ★ Memory escape hatch（详 §10）★
    m.maybePromoteToMemory(ctx, candidates)
    
    // 4. 构造 anchored merge prompt
    prompt := buildCompactPrompt(conv.Summary, candidates)
    
    // 5. cheap LLM call
    bundle, err := m.cheapLLM.Resolve(ctx)  // 闭包内部 = llmclientpkg.ResolveUtility(ctx, picker, keys, factory)
    if err != nil {
        m.em.StopBlock(ctx, blockID, "error", err)
        return err
    }
    
    newSummary, usage, err := llminfra.Generate(ctx, bundle.Client, prompt)
    if err != nil {
        m.em.StopBlock(ctx, blockID, "error", err)
        return err
    }
    
    // 6. 流式 delta 进 compaction block（让前端看着摘要"打字"出来，UX 好）
    for _, chunk := range chunkText(newSummary, 200) {
        m.em.DeltaBlock(ctx, blockID, chunk)
    }
    
    // 7. 一个事务里完成所有 DB 写
    lastSeq := candidates[len(candidates)-1].Seq
    err = m.repo.WithTx(ctx, func(tx) error {
        // 更新 conversation summary
        tx.UpdateConv(convID, map[string]any{
            "summary": newSummary,
            "summary_covers_up_to_seq": lastSeq,
        })
        // 标 archive
        for _, b := range candidates {
            tx.UpdateBlockRole(b.ID, "archived")
        }
        return nil
    })
    if err != nil {
        m.em.StopBlock(ctx, blockID, "error", err)
        return err
    }
    
    // 8. stop block
    m.em.StopBlock(ctx, blockID, "completed", nil)
    
    // 9. 推 notification（slim）
    m.notif.Publish(ctx, "compaction", convID, map[string]any{
        "blocksArchived": len(candidates),
        "coversToSeq": lastSeq,
        "summaryLength": len(newSummary),
    })
    
    return nil
}
```

### 5.6 `buildCompactPrompt`（anchored merge 是关键）

```go
const compactPromptTemplate = `You are maintaining a running summary of an ongoing conversation between a user and an AI assistant.

PREVIOUS SUMMARY (covering everything up to block %d):
%s

NEW CONTENT (blocks %d to %d, %d items):
%s

TASK:
1. Append concise bullets describing what happened in the NEW CONTENT.
2. PRESERVE all bullets from the PREVIOUS SUMMARY unchanged (unless a new bullet directly contradicts it; then mark old one as ~~struck~~ and add new).
3. Mark new bullets with [later] for context.
4. Use these sections:
   - User's original request (PRESERVE)
   - Files touched
   - Tools called (high-level)
   - Errors and fixes
   - Decisions made
   - Current state
5. Keep total summary ≤ 1500 tokens.
6. Output the FULL updated summary (previous + new appended). No commentary.
`
```

**关键**：prompt 强调 **preserve previous + append new**。LLM 不会重写老内容。

---

## 6. 投影规则（`loop/history.go::BlocksToAssistantLLM`）

现有方法负责"从 DB block 拼 LLM history"。改造为按 `context_role` 投影：

```go
func BlocksToAssistantLLM(ctx context.Context, blocks []*Block, conv *Conversation) []LLMMessage {
    var msgs []LLMMessage
    
    // 1. 摘要（如果有）放历史最前，作为 system-like assistant message
    if conv.Summary != "" {
        msgs = append(msgs, LLMMessage{
            Role:    "assistant",
            Content: "<conversation_summary>\n" + conv.Summary + "\n</conversation_summary>",
        })
    }
    
    // 2. 按 context_role 投影 blocks
    for _, b := range blocks {
        switch b.ContextRole {
        case "archived":
            continue  // 完全跳过
            
        case "cold":
            // 仅元数据
            msgs = append(msgs, LLMMessage{
                Role: blockRoleToLLMRole(b),
                Content: fmt.Sprintf("[%s tool: %s, output cleared (%s)]",
                    b.Type, b.Attrs["toolName"], humanizeBytes(len(b.Content))),
            })
            
        case "warm":
            preview := b.Content
            if len(preview) > 200 {
                preview = preview[:200] + fmt.Sprintf(
                    "\n...\n[truncated, %s total]", humanizeBytes(len(b.Content)))
            }
            msgs = append(msgs, LLMMessage{
                Role: blockRoleToLLMRole(b),
                Content: preview,
            })
            
        case "hot":
            msgs = append(msgs, LLMMessage{
                Role: blockRoleToLLMRole(b),
                Content: b.Content,
            })
        }
    }
    
    return msgs
}
```

---

## 7. Token 估算

### 7.1 估算策略（2026-05-30：窗口感知，真实 per-model 窗口）

```go
func (m *Manager) estimate(ctx context.Context, convID string, provider, modelID string) (usable, used int, err error) {
    conv := m.convRepo.Get(ctx, convID)
    blocks := m.repo.ListBlocksForLLMHistory(ctx, convID)
    
    // 算每个 block 投影后的字符数（用 tiktoken-go 转 token）
    used = countTokens(systemPromptStatic)  // 缓存的
    used += countTokens(conv.Summary)
    for _, b := range blocks {
        used += countTokensForRole(b)
    }
    
    // 2026-05-30：通过注入的 CapabilityResolver 拿真实 per-model 窗口
    // 之前 hardcoded 调 modelmeta.Lookup("","") 始终返兜底 ~4K，
    // 导致 200K/1M 窗口大模型被按 4K 压缩，属于严重 bug。
    usable = m.capabilityResolver(ctx, provider, modelID)  // UsableInput = ContextWindow - MaxOutput - SafetyBuffer
    
    return
}
```

**`capabilityResolver` 签名**（装配时注入）：
```go
type CapabilityResolver func(ctx context.Context, provider, modelID string) int // 返 UsableInput
```

`Manager` 字段 `capabilityResolver CapabilityResolver`；`main.go` 注入：
```go
capRes := func(ctx context.Context, provider, modelID string) int {
    cap := capabilityService.ResolveCapabilities(ctx, provider, modelID)
    return cap.UsableInput
}
contextMgr = contextmgr.New(..., capRes)
```

**`MaybeCompact` 和 `ForceCompact` 签名**（2026-05-30 扩展）：调用方（`chat/runner.go`）在调 `MaybeCompact` 时传入当前轮次的 `provider` + `modelID`（来自 `bundle.Provider` / `bundle.ModelID`），使 estimate 能选对窗口。

### 7.2 Model registry — `pkg/modelmeta` 已删除（2026-05-30）

原有 `internal/pkg/modelmeta/` 包（硬编码 ModelMeta 注册表）**已在 2026-05-30 删除**。唯一消费方 `contextmgr/estimate.go` 已迁移到注入的 `CapabilityResolver`（详上节）。

新的能力目录在 `internal/pkg/modelcaps/`，按 family 规则 + per-model 精确覆盖，详见：
[`documents/version-1.2/adhoc-topic-documents/llm-providers/04-capability-catalog.md`](../adhoc-topic-documents/llm-providers/04-capability-catalog.md)

### 7.3 校准

每次 LLM 调用返 `usage.input_tokens` → 真实值。算 `calibrationRatio = actual / estimated`，缓存到 `Conversation.LastCalibration`（new column）。后续估算乘以 ratio。

5 次调用后误差 < 2%。

---

## 8. 用户感知（UI / SSE）

### 8.1 进度条（chat 顶部常驻）

```
[██████░░░░] 38K / 54K · 12 hot · 8 warm · 5 cold · 23 archived
```

走现有 `notifications` SSE bridge，新 event type `context_stats`：

```json
{
  "type": "context_stats",
  "id": "<convId>",
  "data": {
    "usedTokens": 38000,
    "usableTokens": 54000,
    "zone": "yellow",  // green < 70% / yellow 70-85% / orange 85-95% / red > 95%
    "breakdown": {
      "hot":      {"count": 12, "tokens": 28000},
      "warm":     {"count": 8,  "tokens": 4000},
      "cold":     {"count": 5,  "tokens": 1500},
      "archived": {"count": 23, "tokens": 0}
    },
    "summaryTokens": 4500
  }
}
```

ContextManager 在每次状态变更后 publish。

### 8.2 Compaction block 卡片

UI 渲染 `type=compaction` 的 block 成特殊卡片：

```
─────────────────────────────────────────────
  📦 Compacted at 14:32 · covers 47 blocks
  
  ## User's original request
  - 帮我修 foo.py 测试挂的 bug
  
  ## Files touched
  - foo.py (read 3 times, edited line 42)
  - tests/test_foo.py (read, ran 4 times)
  
  ## Errors and fixes
  - npm test failed: "TypeError..."
    → fixed by None check at foo.py:42
  
  [▶ Show 47 archived blocks]
─────────────────────────────────────────────
```

streaming 状态时显示 spinner "⏳ Compacting...";completed 后渲染完整 markdown。点 `[Show ...]` 展开看被 archive 的 block 原文（从 DB 读，永远完整）。

### 8.3 Block role 视觉标记

每条 block 旁边小色块：
- 🟢 hot — 默认无标记
- 🟡 warm — 黄色细边
- 🔵 cold — 蓝色细边 + 透明度 70%
- ⚫ archived — 灰色 + 透明度 40% + "已归档"小标签

hover 显示提示："LLM 看到的是 [完整/preview/元数据/不可见]"。

### 8.4 "Compacting..." 阻塞提示

当 LLM 即将调用但 ContextManager 正在跑 fullCompact 时：
- chat queue 自然 serial 等
- testend 输入框灰显
- 顶部 progress 条变红 + 显示 "⏳ Compacting conversation..."
- compaction block 完成后自动恢复

**用户不需要做任何事，几秒钟自动过去**。

---

## 9. HTTP API

### 9.1 用户面向

| Method | Path | 用途 |
|---|---|---|
| POST | `/api/v1/conversations/{id}:compact` | 手动触发压缩（强制 fullCompact）|
| GET | `/api/v1/conversations/{id}/context-stats` | 当前 stats（首次加载用，之后 SSE 推）|
| GET | `/api/v1/conversations/{id}/summary` | 拿当前 summary 内容 |
| POST | `/api/v1/blocks/{id}:pin` | 手动 pin（永远 hot）|
| POST | `/api/v1/blocks/{id}:unpin` | unpin |

### 9.2 testend 内部

- 配置阈值（softThreshold / hardThreshold / recentKeep）走 settings API
- 看 archived blocks 列表：`GET /conversations/{id}/blocks?role=archived`

---

## 10. Memory 逃生通道（与 §2 联动）

**痛点**：压缩可能丢"用户偏好"这种该长期记住的事。

**机制**：`fullCompact` 在 archive 候选 blocks 前，调 `maybePromoteToMemory`：

```go
func (m *Manager) maybePromoteToMemory(ctx context.Context, candidates []*Block) {
    // 规则扫描（0 LLM）
    for _, b := range candidates {
        if b.Type != "text" || b.Role != "user" {
            continue
        }
        
        // 启发式：用户表达偏好 / 长期声明
        if matchesPreferencePattern(b.Content) {
            // 高置信 → 自动写 memory（标 source=ai 表示自动）
            // 低置信 → 推 suggestion 让用户决定
            m.memory.Upsert(ctx, memoryapp.UpsertInput{
                Name:        deriveNameFromText(b.Content),
                Type:        memorydomain.TypeFeedback,
                Description: firstSentence(b.Content),
                Content:     b.Content,
                Source:      memorydomain.SourceAI,
                Pinned:      false,  // 用户在 testend 决定要不要 pin
            })
            // toast：UI 收到 memory created notification 提示用户"已记到 memory"
        }
    }
}
```

**正则示例**（启发式，宁可漏报不误报）：
- "我喜欢 X" / "我用 X" / "I prefer X" / "I use X"
- "以后都 / 永远 / 别 / 不要"
- "记住" / "remember"
- "我是 X 工程师" / "I'm a X engineer"

---

## 11. 错误码

| Sentinel | HTTP | Wire Code |
|---|---|---|
| `contextmgrdomain.ErrCompactFailed` | 503 | `COMPACTION_FAILED` |

仅 1 个 sentinel——其他错误（DB / LLM）走通用包装。`ErrCompactFailed` 在用户 POST `:compact` 时上抛；自动触发的失败仅 log 不暴露（下次 turn 自然 retry）。

---

## 12. 与其他 domain 的关系

| domain | 关系 |
|---|---|
| **chat** | runner 在 turn 结束后调 `MaybeCompact`；`BlocksToAssistantLLM` 按 context_role 投影（**双改造点**）|
| **conversation** | 加 `summary` + `summary_covers_up_to_seq` 列 |
| **memory** (§2) | 压缩前调 `memory.Upsert` 写"该长期记住的事"（逃生通道）|
| **eventlog** | 加 1 种 block type (`compaction`)；走现有 SSE 协议 |
| **notifications** | 加 `context_stats` event type 推进度条 + `compaction` event type 推完成事件 |
| **subagent** | sub-conversation 也跑同一套 ContextManager（subagent 的 messages 也是 message_blocks 行）|
| **catalog** | 无依赖 |
| **agentstate** | 无依赖——compaction 与对话内 state 无关 |

### 包依赖

```
internal/app/contextmgr/         (Manager: MaybeCompact / demoteOldBlocks / fullCompact / estimate)
  ↓ 消费                          ↓ 消费                   ↓ 消费
chatdomain.Repository       memoryapp.Service       llmclientpkg.Resolver
convdomain.Repository       eventlogpkg.Emitter     pkg/modelmeta
```

无循环依赖。

---

## 13. 测试覆盖

| 层 | 文件 | 覆盖 |
|---|---|---|
| domain | 无独立 domain 包（contextmgr 是 app 层）| — |
| pkg/modelmeta | `pkg/modelmeta/registry_test.go` | 已知 model lookup / unknown fallback / Usable 计算 |
| pkg/tokencount | `pkg/tokencount/count_test.go` | tiktoken-go 集成 / 误差校准 |
| app/contextmgr | `internal/app/contextmgr/{demote,compact,estimate}_test.go` | demoteOldBlocks 规则 / fullCompact 全流程 mock LLM / estimate 阈值判断 / anchored merge prompt |
| loop/history | `internal/app/loop/history_test.go` | 增加 context_role 投影测试（4 种 role 各一例）|
| transport/handlers | `internal/transport/httpapi/handlers/compaction_test.go` | 4 端点 happy + error + role transitions |
| pipeline | `backend/test/compaction/compaction_test.go` | E2E: 长对话超阈值自动触发 / 手动 :compact / archived block 不发给 LLM / memory promotion 端到端 |

---

## 14. 关键决策记录

| 决策 | 选项 | 选了 | 理由 |
|---|---|---|---|
| 触发时机 | LLM 前 / LLM 后 | **LLM 后** | 不阻断当前 turn；下次 turn 前压缩已完成 |
| 阻塞 vs 后台 | 阻塞下一 turn / 完全后台 | **阻塞下一 turn** | chat queue 自然 serial，简单可靠；用户感知 spinner |
| Summary 形态 | paragraph / bullets / JSON | **markdown bullets** | LLM 友好 + 用户可读 + anchored append 容易 |
| Summary 增量 | append / 全重写 | **append**（anchored）| Factory 36K session 验证 anchored > full rewrite |
| 摘要存哪 | conversation column / 独立表 | **conversation column** | 一对话一 summary，多版本无需，简化 |
| Block 角色 | enum / int / bool | **enum 4 值** | hot/warm/cold/archived 清晰；未来加新值不破坏 |
| Compaction 事件载体 | 新 SSE event / 新 block type | **新 block type** | 复用 SSE 协议 + 永久持久化（用户能翻历史看摘要）|
| LLM 摘要重试 | retry 3 次 / 单次失败回退 | **单次** | catalog generator 同思路（屎山拯救计划 #7）：失败下轮 turn 自然 retry |
| Recent keep 数 | 3 turns / 5 turns / N tokens | **3 turns + 5 tool_results** | Anthropic 推荐保 LLM rhythm |
| 用户感知 | 完全透明 / 状态条 + 卡片 | **状态条 + 卡片** | 透明（用户看得见）+ 不打扰（自动跑）|

---

## 15. 演化方向

- **Per-conv 阈值**：高级用户在 conv settings 调 softThreshold / hardThreshold。**v1.2 可做**。
- **Anthropic 服务端 `compact_20260112`**：Anthropic provider 直接走他们 API，省 LLM 一次调用。**v1.5**（需 beta header + flow 适配）。
- **Anthropic 服务端 `clear_tool_uses_20250919`**：tool_result 自动清。**v1.5** 可选启用（client demote 已经做这事，但服务端能省 round-trip）。
- **LLM 见到 token budget**：system prompt dynamic 段加 "you have X tokens remaining"。**v1.5** 评估 ROI。
- **Subagent 用 forked context 跑摘要**：当前直接 cheap LLM，未来若摘要质量不够可考虑。**v2**。
- **Compaction history / diff**：用户能看"这次压缩前后 LLM 看到的差异"。**testend feature**。
- **手动 pin/unpin UI**：右键 block "pin to context"。**v1.2 可做**。
- **TaskBudget per-tool**：让 Bash / Read 等 tool 知道剩余预算自我约束。**v2**。

---

## 16. 历史

- 2026-05-15 设计完成。从早期"Claude Code 5-tier cascade 移植"演化成 Forgify-native "2 列 schema + 1 ContextManager + 1 新 block type" 极简版本。anchored append 借鉴 Factory 评测、Progressive Note-Taking 论文（arxiv 2510.06677）、Cline /smol 实践。
