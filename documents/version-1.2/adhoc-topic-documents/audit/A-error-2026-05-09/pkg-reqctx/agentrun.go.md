# File audit: backend/internal/pkg/reqctx/agentrun.go

LOC: 154. Per-agent-run ID ctx ferries（conversationID / messageID / toolCallID / parentBlockID / subagentDepth / subagentRunID）+ ErrMissingConversationID sentinel。

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | agentrun.go:21 | `var ErrMissingConversationID = errors.New("reqctx: missing conversation id in context")` | A.4 / A.5 | OK | 同 ErrMissingUserID pattern——前缀 `reqctx:`，已登记 errmap.go:186 → 500 INTERNAL_ERROR。 | — | — | — | — |
| 2 | agentrun.go:23-26 | `type conversationIDKey struct{}`<br>`type messageIDKey struct{}`<br>`type toolCallIDKey struct{}`<br>`type parentBlockIDKey struct{}` | A.1 | OK | 4 个私有 empty-struct ctx key。SA1029 合规。 | — | — | — | — |
| 3 | agentrun.go:31-33 | `func WithConversationID(ctx context.Context, id string) context.Context {`<br>`	return context.WithValue(ctx, conversationIDKey{}, id)`<br>`}` | A.2 | OK | ctx setter，无错误路径。 | — | — | — | — |
| 4 | agentrun.go:38-41 | `func GetConversationID(ctx context.Context) (string, bool) {`<br>`	id, ok := ctx.Value(conversationIDKey{}).(string)`<br>`	return id, ok && id != ""`<br>`}` | A.1 | OK | 同 reqctx.go:GetUserID 的 ctx 探针 pattern。type assertion 的 ok-bool 不是 error。 | — | — | — | — |
| 5 | agentrun.go:46-51 | `func RequireConversationID(ctx context.Context) (string, error) {`<br>`	if id, ok := GetConversationID(ctx); ok {`<br>`		return id, nil`<br>`	}`<br>`	return "", ErrMissingConversationID`<br>`}` | A.4 | OK | 同 RequireUserID pattern——直接返 sentinel，§S16 example 行 88 形式。 | — | — | — | — |
| 6 | agentrun.go:56-58 | `func WithMessageID(ctx context.Context, id string) context.Context {`<br>`	return context.WithValue(ctx, messageIDKey{}, id)`<br>`}` | A.2 | OK | ctx setter，无错误路径。 | — | — | — | — |
| 7 | agentrun.go:63-66 | `func GetMessageID(ctx context.Context) (string, bool) {`<br>`	id, ok := ctx.Value(messageIDKey{}).(string)`<br>`	return id, ok && id != ""`<br>`}` | A.1 | OK | 同 site#4 pattern。godoc 行 11-12 显式说"missing values are not bugs"——messageID 缺失是合法 branch（不是每个调用路径都有 in-flight assistant message），**没有 RequireMessageID** 是有意（events filter key 为空时静默 drop）。 | — | — | — | — |
| 8 | agentrun.go:71-73 | `func WithToolCallID(ctx context.Context, id string) context.Context {`<br>`	return context.WithValue(ctx, toolCallIDKey{}, id)`<br>`}` | A.2 | OK | ctx setter，无错误路径。 | — | — | — | — |
| 9 | agentrun.go:78-81 | `func GetToolCallID(ctx context.Context) (string, bool) {`<br>`	id, ok := ctx.Value(toolCallIDKey{}).(string)`<br>`	return id, ok && id != ""`<br>`}` | A.1 | OK | 同 site#7 pattern——godoc 注解（行 11-13）显式说明 missing 非 bug。 | — | — | — | — |
| 10 | agentrun.go:91-93 | `func WithParentBlockID(ctx context.Context, id string) context.Context {`<br>`	return context.WithValue(ctx, parentBlockIDKey{}, id)`<br>`}` | A.2 | OK | ctx setter，无错误路径。 | — | — | — | — |
| 11 | agentrun.go:99-102 | `func GetParentBlockID(ctx context.Context) (string, bool) {`<br>`	id, ok := ctx.Value(parentBlockIDKey{}).(string)`<br>`	return id, ok && id != ""`<br>`}` | A.1 | OK | 同 site#7 pattern——godoc 行 95-98 说"absent or empty (top-level emit)"——缺失语义合法。 | — | — | — | — |
| 12 | agentrun.go:112-113 | `type subagentDepthKey struct{}`<br>`type subagentRunIDKey struct{}` | A.1 | OK | empty-struct key。 | — | — | — | — |
| 13 | agentrun.go:120-122 | `func WithSubagentDepth(ctx context.Context, depth int) context.Context {`<br>`	return context.WithValue(ctx, subagentDepthKey{}, depth)`<br>`}` | A.2 | OK | ctx setter，无错误路径。 | — | — | — | — |
| 14 | agentrun.go:129-134 | `func GetSubagentDepth(ctx context.Context) int {`<br>`	if d, ok := ctx.Value(subagentDepthKey{}).(int); ok {`<br>`		return d`<br>`	}`<br>`	return 0`<br>`}` | A.1 | OK | 总返 int（preference 模式同 GetLocale）；godoc 行 124-128 显式说"absent means depth=0"——缺失语义合法。 | — | — | — | — |
| 15 | agentrun.go:142-144 | `func WithSubagentRunID(ctx context.Context, id string) context.Context {`<br>`	return context.WithValue(ctx, subagentRunIDKey{}, id)`<br>`}` | A.2 | OK | ctx setter，无错误路径。 | — | — | — | — |
| 16 | agentrun.go:151-154 | `func GetSubagentRunID(ctx context.Context) (string, bool) {`<br>`	id, ok := ctx.Value(subagentRunIDKey{}).(string)`<br>`	return id, ok && id != ""`<br>`}` | A.1 | OK | 同 site#7 pattern——godoc 行 146-150 说"absent or empty (we are not inside a subagent loop)"——缺失合法。 | — | — | — | — |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: not present
  - notes: 5 个 Get-form helper（GetConversationID / GetMessageID / GetToolCallID / GetParentBlockID / GetSubagentRunID）都返 (string, bool)，调用方决定是否 require；GetSubagentDepth 总返 int（preference 模式）。godoc 行 11-13 / 95-98 / 124-128 / 146-150 都有"missing 合法"显式说明——非吞错误

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: 无（本文件是 ctx ferry，不写 DB / 不发事件）
  - violations: N/A: package doesn't do terminal writes (pure ctx ferry)
  - 备注: 同 reqctx.go——本包提供 SetUserID / WithConversationID 等让业务代码构建 detached ctx，本身非写

A.3 §S15 ID 生成:
  - ID generation calls: 无（all IDs are ctx-ferry pass-through, generated upstream by `idgen.New("cv")` / `idgen.New("msg")` etc.）
  - violations: N/A: package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present
  - notes: site#5 RequireConversationID 直接返 sentinel——§S16 example 行 88 形式，无 wrap，errors.Is 完整可达

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ErrMissingConversationID (line 21)
  - 已登记 errmap: ErrMissingConversationID 登记在 errmap.go:186 → 500 INTERNAL_ERROR
  - missing: all registered
