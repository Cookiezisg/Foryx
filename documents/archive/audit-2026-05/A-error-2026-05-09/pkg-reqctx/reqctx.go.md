# File audit: backend/internal/pkg/reqctx/reqctx.go

LOC: 55. UserID ctx ferry — 包含 §S9 detached pattern 的核心 helper `SetUserID`。

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | reqctx.go:19 | `var ErrMissingUserID = errors.New("reqctx: missing user id in context")` | A.4 / A.5 | OK | 错误消息含 `<pkg>:` 前缀（"reqctx:"），符合 §S16 sentinel-at-innermost 形式。已登记 errmap.go:185（Cross-cutting 段）→ 500 INTERNAL_ERROR。 | — | — | — | — |
| 2 | reqctx.go:31-33 | `func SetUserID(ctx context.Context, id string) context.Context {`<br>`	return context.WithValue(ctx, userIDKey{}, id)`<br>`}` | A.2 | OK | **§S9 detached pattern 的实现核心**。被业务代码用 `reqctxpkg.SetUserID(context.Background(), uid)` 派生 detached ctx。本函数无错误处理职责——只是 WithValue 包装，按设计无 err。 | — | — | — | — |
| 3 | reqctx.go:39-42 | `func GetUserID(ctx context.Context) (string, bool) {`<br>`	id, ok := ctx.Value(userIDKey{}).(string)`<br>`	return id, ok && id != ""`<br>`}` | A.1 | OK | 经典 ctx-key 探针。type assertion `_, ok` 的 ok-bool 是 Go 习惯（不是 error）。返 false 表"未设值或为空"是合法 branch，调用方决定是否返 ErrMissingUserID（site#4）。 | — | — | — | — |
| 4 | reqctx.go:48-54 | `func RequireUserID(ctx context.Context) (string, error) {`<br>`	id, ok := GetUserID(ctx)`<br>`	if !ok {`<br>`		return "", ErrMissingUserID`<br>`	}`<br>`	return id, nil`<br>`}` | A.4 | OK | 直接返 sentinel（最里层无需 wrap）—— 符合 §S16 example 行 88 "直接返 sentinel" pattern。没有 wrap 信息丢失，errors.Is 完整可达。 | — | — | — | — |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: not present
  - notes: site#3 是 ctx 探针的 ok-bool 模式，不是 error 吞错

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: 无（本文件是 ctx ferry，不写 DB / 不发事件）
  - 各自 ctx 来源: N/A
  - violations: N/A: package doesn't do terminal writes (it provides the SetUserID helper that callers use to build detached ctx; not itself a write site)
  - 备注: site#2 SetUserID 是 §S9 detached pattern 的**实现源**——CLAUDE.md §S9 显式举例 `reqctxpkg.SetUserID(context.Background(), uid)`。本函数实现正确（直接 WithValue，无副作用）

A.3 §S15 ID 生成:
  - ID generation calls: 无
  - violations: N/A: package doesn't generate business IDs (handles existing IDs only)

A.4 §S16 错误 wrap 格式:
  - violations: not present
  - notes: site#4 直接返 sentinel（§S16 example 行 88 允许的"最里层"形式）；本包不上抛包装错误

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: ErrMissingUserID (line 19)
  - 已登记 errmap: ErrMissingUserID 登记在 errmap.go:185 → 500 INTERNAL_ERROR
  - missing: all registered
