# Package audit summary: internal/pkg/reqctx

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: 本包是纯 ctx ferry——所有 Get-form 返 (string, bool) / int，没有 error 概念。godoc 显式说明 missing 在不同 helper 下的语义（identity 类如 conversationID 提供 Require-form 上抛 sentinel；preference 类如 locale / subagentDepth 总返默认值；event-filter 类如 messageID / toolCallID 缺失合法静默）。0 violation。
- **§S9 detached ctx 终态写**: 本包不做终态写——但**实现了 §S9 的核心 helper**：`SetUserID(context.Background(), uid)` 是项目所有 detached ctx 的源 (CLAUDE.md §S9 example)。以及 `WithConversationID` / `WithMessageID` 等让业务代码组装完整 detached ctx。本包自身非写，0 violation。
- **§S15 ID 生成**: N/A — 本包是 ctx pass-through，所有 ID 上游用 `idgen.New(prefix)` 生成。
- **§S16 错误 wrap 格式**: `RequireUserID` / `RequireConversationID` 直接返 sentinel（§S16 example 行 88 "最里层"形式）；本包不做 wrap。0 violation。
- **§S17 errmap 单一事实源**: 2 sentinel — ErrMissingUserID（reqctx.go:19）/ ErrMissingConversationID（agentrun.go:21），**两个都已登记**在 errmap.go:185/186 cross-cutting 段（500 INTERNAL_ERROR）。0 missing。

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| reqctx.go | 55 | 4 | 4 | 0 | 0 | 0 |
| agentstate.go | 28 | 3 | 3 | 0 | 0 | 0 |
| locale.go | 44 | 4 | 4 | 0 | 0 | 0 |
| agentrun.go | 154 | 16 | 16 | 0 | 0 | 0 |
| **TOTAL** | **281** | **27** | **27** | **0** | **0** | **0** |

## Severity breakdown

| Severity | Count | Status |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 0 | — |

**Net: 0 violations**.

## Cross-cutting

### §S9 detached pattern 实现源

`SetUserID(context.Background(), uid)` (reqctx.go:31-33) 是**全项目 §S9 detached ctx 的入口**——所有终态写都靠它构建 detached ctx。本函数实现极简（直接 WithValue），无副作用，无错误路径。

实际使用 pattern（CLAUDE.md §S9 example）:
```go
detached := reqctxpkg.SetUserID(context.Background(), uid)
s.repo.Update(detached, ...)  // 终态写
```

注：本包仅提供 setter；**调用方需自己组合 conversationID / messageID 等**到 detached ctx 上（如 chat 流终态写需要保留 conversationID 给事件路由），可考虑添加 `Detach(ctx)` 一键派生 helper——但属设计层重构建议，超出 §S3-S17 scope。

### Get-form 三种语义模式

本包 6 个 Get-form helper 分三类语义，godoc 都已显式说明：

1. **Identity 类** (UserID / ConversationID): 缺失 = 接线 bug，提供 Require-form 上抛 sentinel
2. **Preference 类** (Locale / SubagentDepth): 总返默认值，无 Require-form
3. **Event-filter 类** (MessageID / ToolCallID / ParentBlockID / SubagentRunID): 缺失合法（事件 filter key 为空时静默 drop），无 Require-form

这种分类清晰避免了"什么时候该返 error 什么时候返默认值"的判断负担——godoc 直接告诉调用方该用哪种模式。

### sentinel 登记一致

errmap.go:185-186 cross-cutting 段显式注释：

```go
// Cross-cutting: explicitly registered to suppress the "unmapped domain
// error" warning while still returning 500. Both represent server-side
// state that the user can't recover from.
reqctxpkg.ErrMissingUserID:         {http.StatusInternalServerError, "INTERNAL_ERROR"},
reqctxpkg.ErrMissingConversationID: {http.StatusInternalServerError, "INTERNAL_ERROR"},
```

按 §S17 规则，pkg/ 跨层使用的 sentinel 必须登记——本包 2/2 完全合规。

### 自检触发场景：本包是否会产生新 sentinel？

未来扩展时（如增加 RequireMessageID）需要：
1. 本包加 `var ErrMissingMessageID = errors.New("reqctx: missing message id in context")`
2. errmap.go cross-cutting 段加一行
3. 设计 doc 更新（哪些路径必须 require message ID）

CLAUDE.md §S17 + §S14 双重纪律。

## Recommended fix priorities

**No fixes needed**. 本包是 §S3/S9/S15/S16/S17 textbook clean。

## Out-of-scope notes

1. **agentrun.go 命名**: 包含 conversation / message / toolCall / parentBlock / subagent 5 类 ID——文件名 `agentrun.go` 准确描述了 "per-agent-run identifiers" 但若按 §S12 "概念拆分" 严格执行可考虑拆 `subagent.go` 单独——但当前 154 行远未触及 500 行警戒线，不必拆。属设计建议，不是 audit issue。
2. **godoc 行 11-12 的"silently go nowhere"** 在 events 系统已迁移到 §E1 双协议（domain/eventlog + domain/notifications）后，需要核对 messageID / toolCallID 缺失时事件确实静默 drop——这是 audit pkg/eventlog 该验的，非本包 scope。
3. **没有 Detach(ctx) helper**: 业务代码每次构建 detached ctx 都要手动 `SetUserID(context.Background(), uid)` + `WithConversationID(...)` + `WithMessageID(...)`——可考虑提供 `Detach(ctx) (detached, error)` 一键派生（保留必要 ID + 用 Background 替换 cancel-prone parent）。但属 API 设计层，不是 §S3-S17 audit issue。
