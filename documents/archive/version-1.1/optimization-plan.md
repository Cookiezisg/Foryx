# Forgify 优化计划（V1.1 收官 + 跨版本质量地基）

**创建于**：2026-04-22
**目标**：V1.1 收官 + 把代码质量拉到能扛 Tier 5/6 工作流引擎复杂度的水平
**总工时估算**：~29–41h（15 项）

> ⚠️ **2026-04-22 更新**：Stage 2-5 已作废，被更彻底的后端重写替代。
> 详见 [`Documents/BACKEND_REWRITE.md`](../BACKEND_REWRITE.md)。
> Stage 1（V1.1 收官）仍有效且已完成。

---

## 状态标记

```
⬜ 未开始
🔄 进行中
✅ 已完成
⏸ 暂缓
❌ 决定不做
```

---

## Stage 1 — V1.1 收官（P0，必做）

| # | 任务 | 涉及文件 | 工作量 | 依赖 | 状态 | 备注 |
|---|---|---|---|---|---|---|
| 1 | Chat+Tool 布局下隐藏代码块测试/保存按钮 | `TabContext.tsx`（+`useCurrentLayout` hook）、`ForgeCodeBlock.tsx`（早返判断） | 2–4h | — | ✅ | 2026-04-22 完成 |
| 2 | 测试失败"让 AI 修复"链路 | `TestTab`、`ChatContent`、`ChatToolLayout`、i18n | 4–6h | #1 | ✅ | 2026-04-22 完成；纯前端改动，复用 Eino 现有对话管道 |

**验收**：V1.1 PRD 验收清单全部 ✅；Chat+Tool 完整闭环跑一遍。

---

## Stage 2 — 后端质量地基（P1）— ❌ 作废，被重写替代

| # | 任务 | 涉及文件 | 工作量 | 依赖 | 状态 | 备注 |
|---|---|---|---|---|---|---|
| 3 | 后端错误处理统一（消除裸 Exec 不检查错、`_` 静默吞错） | `routes_chat.go:44/69/71/286` 等 | 2–3h | — | ⬜ | |
| 4 | Handler 中残留 SQL 下沉到 service 层 | `routes_chat.go`、对应 service | 3–4h | #3 | ⬜ | |
| 5 | 全局 panic recover middleware | `server.go` | 30min | — | ⬜ | |
| 6 | service 层 + forge 解析器单元测试 | 新 `*_test.go` | 4–6h | #3、#4 | ⬜ | |

**验收**：`go vet` + `go test ./...` 零报错；handler 不再出现裸 `storage.DB().Exec()`。

---

## Stage 3 — 安全收紧(P1）— ❌ 作废，被重写替代

| # | 任务 | 涉及文件 | 工作量 | 依赖 | 状态 | 备注 |
|---|---|---|---|---|---|---|
| 7 | Python 沙箱命令/参数白名单校验 | `sandbox/process.go` | 1–2h | — | ⬜ | |
| 8 | AST 解析输出验证（函数名/参数名按 Python 标识符规则） | `forge/ast_parser.go` | 1–2h | — | ⬜ | |

**验收**：构造非法函数名/恶意参数测试用例不入库；沙箱拒绝执行非 `python` 二进制。

---

## Stage 4 — 前端质量（P2）— ⏸ 暂缓，下轮前端 iteration 再做

| # | 任务 | 涉及文件 | 工作量 | 依赖 | 状态 | 备注 |
|---|---|---|---|---|---|---|
| 9 | 消除 `Record<string, any>` 与无类型 `JSON.parse` | `TestParamsModal.tsx:32`、`ToolMainView.tsx:685`、`MessageItem.tsx:78/89` | 2h | — | ⬜ | |
| 10 | i18n 补齐（`ForgeCodeBlock` 硬编码中文 + 后端系统提示） | 两个 locale 文件、`routes_chat.go` | 2h | — | ⬜ | |
| 11 | 设计系统统一（颜色常量、抽样式常量） | 新 `lib/styles.ts`、批量替换 | 3–4h | — | ⬜ | |
| 12 | `ToolMainView` / `ChatToolLayout` 性能优化（memo / useCallback） | 上述两个组件 | 2h | — | ⬜ | |
| 13 | `MessageItem → ForgeCodeBlock` prop drilling 收敛为 ForgeContext | `context/`、两个组件 | 2h | #1 | ⬜ | |

**验收**：`tsc --noEmit` 零报错；前端无明显重复渲染（React DevTools profile 一遍）。

---

## Stage 5 — 收尾杂项（P2）— ❌ 作废，被重写替代

| # | 任务 | 涉及文件 | 工作量 | 依赖 | 状态 | 备注 |
|---|---|---|---|---|---|---|
| 14 | 验证 `idx_msg_conv` 实际查询计划，必要时补索引 | `001_schema.sql`、新 migration | 1h | — | ⬜ | |
| 15 | 决定 `routes_conversations.go:168` 的 TODO 归属 Tier | PRD/PROGRESS 更新 | 15min | — | ⬜ | |

---

## 总览

| Stage | 任务数 | 总工时 | 完成后效果 |
|---|---|---|---|
| 1 | 2 | 6–10h | V1.1 验收 100% |
| 2 | 4 | 9–13h | 后端能扛 Tier 5/6 复杂度 |
| 3 | 2 | 2–4h | 沙箱与 AST 不留攻击面 |
| 4 | 5 | 11–13h | 前端类型安全 + 设计统一 |
| 5 | 2 | ~1h | 杂项清零 |
| **合计** | **15** | **~29–41h** | 可干净进入 Tier 5（工作流画布）|

---

## 进度日志

| 日期 | 内容 |
|---|---|
| 2026-04-22 | 计划创建：基于代码审计的 15 项优化任务，分 5 个 Stage |
| 2026-04-22 | Stage 1 #1 完成：TabContext 新增 `useCurrentLayout` hook，ForgeCodeBlock 在 chat-tool/chat-workflow 布局下整体早返 null（代码块本体由父组件 SyntaxHighlighter 保留）。tsc + build 通过 |
| 2026-04-22 | Stage 1 #2 完成：测试失败一键"让 AI 修复"。右侧 TestTab 加按钮，window.dispatchEvent 广播 `forge:fix-requested`；ChatContent 按 conversationId 匹配后 sendMessage 进 Eino 对话管道。复用现有 ForgeMiddleware/ForgeCodeUpdated 链路自动刷新右侧代码。后端零改动 |
