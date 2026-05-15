# File audit: backend/internal/pkg/installprogress/installprogress.go

LOC: 200 (1 file in package).

## Trace table

| site# | file:line | snippet | category | classification | reasoning | severity | user_impact | suggested_fix | status |
|---|---|---|---|---|---|---|---|---|---|
| 1 | installprogress.go:75-79 | `progressCb := newProgressCallback(ctx, attrs)`<br>`progressCb.emitStartLine(attrs)`<br>`out, err := fn(progressCb.cb)`<br>`progressCb.close(ctx, err)`<br>`return out, err` | A.1 | OK | err 原样返回给调用者，未吞。fn 的 err 通过 close 渲染到 progress block ([error] line) 同时仍由 Run 上抛——双向都到调用者。godoc 行 69 显式说明"err 原样返回；helper 不加 wrapping"。 | — | — | — | — |
| 2 | installprogress.go:97-99 | `em := eventlogpkg.From(ctx)`<br>`if em == nil {`<br>`	return &progressCallback{}`<br>`}` | A.1 | OK | `eventlogpkg.From` 文档承诺 never-nil（在 ctx 无 emitter 时返 no-op emitter），这里多一层 nil-guard 是 defensive；返 zero-blockID progressCallback 让后续所有方法走 no-op 分支，不是"静默吞错误"——本来就没 error。这是 `inChatFlow` 已经守卫过的二次防御。 | — | — | — | — |
| 3 | installprogress.go:101 | `blockID := em.StartBlock(ctx, eventlogdomain.BlockTypeProgress, attrs)` | A.1 | OK | StartBlock 返 blockID（string），不返 error——SSE bridge 推流是 fire-and-forget，pipeline 内部已有 logger 兜底（属 §S10 异步必打 log 范畴）。无 error 可吞。 | — | — | — | — |
| 4 | installprogress.go:106-107 | `if p.blockID == "" {`<br>`	return // no-op when not in chat flow`<br>`}` | A.1 | OK | 行内注释说明为什么 short-circuit："非 chat flow" 是 well-defined branch 不是错误吞。 | — | — | — | — |
| 5 | installprogress.go:109-115 | `// DeltaBlock signature: (ctx, blockID, delta). Use Background ctx`<br>`// because progressCallback doesn't carry the original — emitter`<br>`// only uses ctx for logging, not control.`<br>`p.em.DeltaBlock(context.Background(), p.blockID, formatProgressLine(stage, message, percent))` | A.2 | OK | 用 `context.Background()` 是 detached pattern 的轻量变体——progressCallback 是 fn 收到的 callback，可能在 install 完成后/上游 cancel 后被调；emitter 已用 ctx 仅做 logging（注释行 110-114 显式说明）。这不是终态写入（block 终态在 close()），是中间 delta。即便 ctx 被 cancel 这条 delta 也该尝试推。**不算严格 §S9 终态写**，但 detached pattern 选择是对的。注意：未通过 `reqctxpkg.SetUserID(context.Background(), uid)` 注入 userID——但 emitter 内部不靠 userID 路由（按 conversationId routing），故无影响。 | — | — | — | — |
| 6 | installprogress.go:115 | `p.em.DeltaBlock(context.Background(), ...)` | A.1 | OK | DeltaBlock 不返 error；fire-and-forget。无可吞。 | — | — | — | — |
| 7 | installprogress.go:133-134 | `rt, _ := attrs["runtime"].(string)`<br>`srv, _ := attrs["server"].(string)` | A.1 | EDGE | 类型断言 `_` 丢弃失败 boolean——但**这不是 error**（type assertion 的 ok 不是 error）。失败时返零值（空字符串），下面 switch 显式处理空串 case (default branch)。逻辑上等价于"attrs 里没有 runtime/server 时走通用 [starting] sandbox install"——这是设计意图。归类为 EDGE 因为 `_ = err` 模式形似 §S3 但 `_` 丢的是 ok-bool 不是 error；正常 Go 习惯。 | LOW | — | — | — |
| 8 | installprogress.go:144 | `p.em.DeltaBlock(context.Background(), p.blockID, line)` | A.1 / A.2 | OK | 同 site#5/6 — fire-and-forget delta，detached ctx 是有意，无 error 可吞。 | — | — | — | — |
| 9 | installprogress.go:147-159 | `func (p *progressCallback) close(ctx context.Context, err error) {`<br>`	if p.blockID == "" {`<br>`		return`<br>`	}`<br>`	status := eventlogdomain.StatusCompleted`<br>`	if err != nil {`<br>`		p.em.DeltaBlock(context.Background(), p.blockID, fmt.Sprintf("[error] %v\n", err))`<br>`		status = eventlogdomain.StatusError`<br>`	} else {`<br>`		p.em.DeltaBlock(context.Background(), p.blockID, "[done] install complete\n")`<br>`	}`<br>`	p.em.StopBlock(ctx, p.blockID, status, err)`<br>`}` | A.2 | OK | 关键的终态写。**StopBlock 用调用方传入的 ctx**（不是 Background）——这是因为 close 是同步在 Run 里调（Run 的 ctx），如果 Run 的 ctx 已被 cancel，StopBlock 也会拿到 cancel 信号。但仔细看 emitter 的 StopBlock 内部行为决定是否符合 §S9：若 ctx-cancel 时 StopBlock 会 abort，则违规；若仅用 ctx 做 logging（同 DeltaBlock 注释承诺），则 OK。emitter 行为承诺在注释 110-114 是"only uses ctx for logging, not control"——按此承诺 OK。**但这是 emitter 一致性约定，不在本文件**——仍要标 EDGE 以提示审查 emitter 时验证此承诺。`fmt.Sprintf("[error] %v", err)` 用 `%v` 渲染——这是**写到 progress block delta 的 UI 文本**，非 error wrap，`%v` 是渲染语义不是 §S16 wrap 语义。OK。 | LOW | progress block 在 cancel 路径可能不关停（若 emitter 实际 abort on ctx-cancel）；UI 看到 dangling streaming block | 用 `reqctxpkg.SetUserID(context.Background(), uid)` 派生 detached ctx 给 StopBlock；或在 emitter 文档 / 实现层显式承诺 ctx-cancel 不影响 stop 路径 | FOUND |
| 10 | installprogress.go:153 | `p.em.DeltaBlock(context.Background(), p.blockID, fmt.Sprintf("[error] %v\n", err))` | A.4 | OK | `%v` 不是 §S16 wrap，是 progress block delta 的 UI 渲染（一行人读文本，不参与 errors.Is）。fn 的 err 仍由 Run 原样上抛（site#1 已确认）。 | — | — | — | — |
| 11 | installprogress.go:170-178 | `func inChatFlow(ctx context.Context) bool {`<br>`	if convID, ok := reqctxpkg.GetConversationID(ctx); !ok || convID == "" {`<br>`		return false`<br>`	}`<br>`	if parent, ok := reqctxpkg.GetParentBlockID(ctx); !ok || parent == "" {`<br>`		return false`<br>`	}`<br>`	return true`<br>`}` | A.1 | OK | 经典 ctx-key probe pattern——`!ok || convID == ""` 双重 guard 是 defensive。返 false 是合法 branch（"非 chat flow → no-op"）非吞错误。 | — | — | — | — |

## Sub-checks

A.1 §S3 错误吞没:
  - violations: not present
  - notes: site#7 是 type assertion 的 `_, _` 丢弃 ok-bool（不是 error），Go 习惯写法；switch default 显式处理零值，归 EDGE 而非 violation

A.2 §S9 detached ctx 终态写:
  - terminal-state writes identified: site#9 (StopBlock 在 close())
  - 各自 ctx 来源:
    - close() 接收的 ctx 来自 Run 的调用方传入的 ctx（即 fn 的执行 ctx，可能被 cancel）
    - DeltaBlock 全部用 `context.Background()`（site#5/8/10），是 detached（一致）
    - StopBlock 用 close() 收到的 ctx（site#9），**不一致**——若 emitter StopBlock 在 ctx-cancel 时 abort 会丢终态
  - violations: 1 LOW (site#9 — StopBlock 没用 detached ctx)
  - 备注: emitter 注释承诺"ctx 只用于 logging 不控流"——若信任此承诺则非违规；若审查 emitter 实现层不一致则该层应修。本包级 audit 标 LOW 提示交叉验证

A.3 §S15 ID 生成:
  - ID generation calls: 无（blockID 来自 emitter.StartBlock 返回，不是本包生成）
  - violations: N/A: package doesn't generate business IDs

A.4 §S16 错误 wrap 格式:
  - violations: not present
  - notes: site#1 godoc 显式声明"err 原样返回；helper 不加 wrapping"——这是**有意不 wrap**，因为本包是中间 helper（不是接 LLM/IO 边界），wrap 反而污染 sentinel chain。fn 的 err 是上层 sandbox 调用的 err，已带 `<pkg>.<Method>:` 前缀，本包透传是正确的

A.5 §S17 sentinel 登记 errmap:
  - sentinels defined: none
  - 已登记 errmap: N/A
  - missing: N/A: file defines no sentinels (helper package; consumes sandboxdomain / eventlogdomain types only)
