# Package audit summary: internal/pkg/installprogress

## Spec understanding (Phase A — §S3 / §S9 / §S15 / §S16 / §S17)

- **§S3 错误不吞**: 本包是 fire-and-forget eventlog wrapper，`em.DeltaBlock` / `em.StartBlock` / `em.StopBlock` 不返 error（emitter 内部承担 logging）；`fn` 的 err 原样透传（godoc 行 69 显式承诺）。无吞错误风险。type assertion `rt, _ := attrs["runtime"].(string)` 丢弃 ok-bool（不是 error）—— Go 习惯写法，归 EDGE。
- **§S9 detached ctx 终态写**: 一处终态写 — `close()` 调 `StopBlock(ctx, ...)`（site#9）。**未严格遵循 §S9 detached pattern**——StopBlock 用调用方传入的 ctx（可能被 cancel），不是 `reqctxpkg.SetUserID(context.Background(), uid)` 派生的 detached ctx。emitter 的 ctx 使用承诺（"only for logging, not control"）写在 DeltaBlock 内联注释，但未写在 StopBlock 路径上——若 emitter 实现层 StopBlock 在 ctx-cancel 时 abort，progress block 会卡 streaming 状态（违反 §S21 status 单向流转）。1 LOW。
- **§S15 ID 生成**: N/A — blockID 由 emitter.StartBlock 返回，本包不生成业务 ID。
- **§S16 错误 wrap 格式**: 本包 helper 有意不 wrap fn 的 err（godoc 行 69 显式声明），透传上层 sandbox sentinel chain。0 violation。`fmt.Sprintf("[error] %v", err)` 是写到 progress block 的 UI delta（site#10），不是 error wrap，`%v` 渲染语义合法。
- **§S17 errmap 单一事实源**: N/A — 本包不定义 sentinel。

## Files audited

| File | LOC | Sites | OK | POST-FIX | VIOLATION | EDGE |
|---|---|---|---|---|---|---|
| installprogress.go | 200 | 11 | 9 | 0 | 1 | 1 |
| **TOTAL** | **200** | **11** | **9** | **0** | **1** | **1** |

## Severity breakdown

| Severity | Count | Status |
|---|---|---|
| HIGH | 0 | — |
| MED | 0 | — |
| LOW | 1 | site#9 §S9 — StopBlock 未用 detached ctx |

**Net: 1 LOW violation**.

## Cross-cutting

### Detached ctx 不一致 (site#5/8/10 vs site#9)

DeltaBlock 全部用 `context.Background()`（site#5/8/10），且行内注释解释清楚：

```
// DeltaBlock signature: (ctx, blockID, delta). Use Background ctx
// because progressCallback doesn't carry the original — emitter
// only uses ctx for logging, not control.
```

但 close() 的 StopBlock 行 (site#9) 反而用了**传入的 ctx**：

```go
func (p *progressCallback) close(ctx context.Context, err error) {
    ...
    p.em.StopBlock(ctx, p.blockID, status, err)
}
```

这是不对称的设计。判断逻辑：
- 若 emitter 真的"只用 ctx 做 logging"——那 DeltaBlock 用 Background 是冗余防御；StopBlock 用传入 ctx 是正确选择（保留 ctx 作 logging）
- 若 emitter ctx 影响控流——那 DeltaBlock 的 Background 是必需，但 StopBlock 应同步切换到 detached ctx，否则 cancel 后整个 progress block 卡 streaming

**判定**：注释承诺只在 DeltaBlock 行写，StopBlock 行未表态——按 §S9 字面规则（"必须落库的最后一步必须用 detached ctx"）应该用 detached。

### S20 当场修风险评估

S20（禁止"留下次"无理由）触发：本 audit 阶段不修代码，故 finding 落 LOW + FOUND 待修。修复需要：
- (a) 结构性硬约束: 否——单文件 1 行改动
- 故应纳入本轮 audit 后的 fix batch

### eventlog emitter 承诺一致性 (out-of-scope hint)

audit installprogress 时发现 emitter 的 ctx 行为承诺（"only for logging"）是隐性契约——文档化承诺只出现在 installprogress.go 的内联注释（site#5 行 110-114），不在 `pkg/eventlog/` 包的 godoc 中。这是跨包契约不显式的 smell。但属 audit pkg/eventlog 的 scope，不在本包 audit 范围。

### 已修历史 (commit 855f382)

任务 prompt 说 "[已修过] commit 855f382 加了起点/终点 delta + sandbox_env notification"——验证现有实现：
- emitStartLine (site at line 129) ✓ — 起点 delta
- close() 的 done/error delta（site#9 行 153/156）✓ — 终点 delta
- "Notifications (sandbox_env entity state changes) are NOT emitted by this helper"（包 godoc 行 20-24）✓ — 显式说明 sandbox_env notification 由各 service 自己发，本 helper 只管 progress 流

承诺现已落地。

## Recommended fix priorities

1. **LOW** — site#9: close() 的 StopBlock 用 detached ctx，对齐 §S9 终态写规则。修法：
   ```go
   func (p *progressCallback) close(ctx context.Context, err error) {
       if p.blockID == "" { return }
       // 派生 detached ctx 让终态 stop 不被上游 cancel 影响
       uid, _ := reqctxpkg.GetUserID(ctx)
       detached := reqctxpkg.SetUserID(context.Background(), uid)
       ...
       p.em.StopBlock(detached, p.blockID, status, err)
   }
   ```
   或者更简单：直接用 `context.Background()`（与 DeltaBlock 一致），因为 emitter 注释承诺 ctx 只用于 logging——对称化。

## Out-of-scope notes

1. emitter 的 ctx 使用约定（"only for logging, not control"）需要在 `pkg/eventlog/` 包的 godoc 中显式声明，目前仅以内联注释存在于 installprogress.go——属跨包契约不显式的工程债，不是本包 audit issue。
2. `attrs map[string]any` 是无类型 schema（runtime / server keys 用 type assertion 取）；若 attrs schema 收紧到 typed struct（如 sandboxdomain.ProgressAttrs），site#7 EDGE 可消除。属设计层重构，超出 §S3-S17 scope。
