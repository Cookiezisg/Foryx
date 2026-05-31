# File audit: backend/internal/pkg/reqctx/agentstate.go

LOC: 28. AgentState ctx ferry — 把 `*agentstatepkg.AgentState` 通过 ctx 传递。

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | agentstate.go:12 | `type agentStateKey struct{}` | A.1 | OK | 私有 empty-struct ctx key（避免 SA1029 违规——string-key 会触发 staticcheck）。设计正确。 | — | — | — | — |
| 2 | agentstate.go:17-19 | `func WithAgentState(ctx context.Context, s *agentstatepkg.AgentState) context.Context {`<br>`	return context.WithValue(ctx, agentStateKey{}, s)`<br>`}` | A.2 | OK | ctx setter，无错误路径。 | — | — | — | — |
| 3 | agentstate.go:25-28 | `func GetAgentState(ctx context.Context) (*agentstatepkg.AgentState, bool) {`<br>`	s, ok := ctx.Value(agentStateKey{}).(*agentstatepkg.AgentState)`<br>`	return s, ok && s != nil`<br>`}` | A.1 | OK | 返双值 (*, bool)；nil 也归 ok=false（双重 guard：type assert ok && s != nil）。godoc 显式说"调用方自决 fail 或跳过"——非吞错误。 | — | — | — | — |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: not present

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: 无
  - 各自 ctx 来源: N/A
  - violations: N/A: package doesn't do terminal writes (pure ctx ferry)

A.3 §S15 ID 生成:
  - ID generation calls: 无
  - violations: N/A: package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present (no error returns)

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: 无
  - 已登记 errmap: N/A
  - missing: N/A: file defines no sentinels
