# Testend — Claude 工作守则

> 本文件是 testend 子项目的工程纪律事实源。项目根 [`../CLAUDE.md`](../CLAUDE.md) 仍生效，本文件**补充**而非覆盖；冲突时本文件赢（仅 testend 范围）。

---

## 一句话

testend 是 Forgify 的**开发调试控制台**：React 18.3 + TanStack Query v5 + Zustand v5 + Vite 6 + react-router-dom 6 hash + reactflow + monaco。通过 vite path alias 共享 `frontend/src/` 的 entity TS 类型 + shared/api/errorCodes 常量。V3 完工于 2026-05-27。

---

## 必读

| 用途 | 路径 |
|---|---|
| 设计文档 | [`../documents/version-1.2/adhoc-topic-documents/testend/testend-design.md`](../documents/version-1.2/adhoc-topic-documents/testend/testend-design.md) |
| issue log | [`../documents/version-1.2/adhoc-topic-documents/testend/testend-rewrite/testend-rewrite-backend-issues.md`](../documents/version-1.2/adhoc-topic-documents/testend/testend-rewrite/testend-rewrite-backend-issues.md) |
| V3 rewrite design / plan | `2026-05-27-react-rewrite-{design,plan}.md`（同 issue log 目录） |

---

## 改代码前必做

1. 读对应 view 的源（`testend/src/views/<section>/<View>.tsx`）
2. 如果 entity 数据契约变更，先确认 frontend `src/entities/<x>/model/types.ts` 已对齐 backend；testend 跟随，无独立 type 副本
3. 改完跑 `cd testend && npm run typecheck && npm run build`（硬门禁）
4. 同步文档（§F1 testend 部分）

---

## 9 条 testend 纪律

1. **不进 FSD layered 架构**。testend 是 tool，扁平 view-driven。`src/views/<section>/<View>.tsx` 直接读 stores + 调 hooks + 渲染。不要拆 entities / features / widgets / pages 这种 layer。
2. **共享 frontend 只通过 type-only 深引**：`import type { X } from "@frontend/entities/<x>/model/types"`。**不经 barrel `index.ts`**（barrel 会拉 React hook 运行时，污染 testend bundle）。
3. **deps 版本号严格 sync frontend**。testend/package.json 中 React / TanStack Query / Zustand / Vite / TypeScript / lucide-react 的版本号必须与 frontend/package.json 一致。每次 frontend 升级，testend 同步。验证：两边 `npm ls react`，版本号需一字不差。否则会出 "Multiple React instances" 运行时错误。
4. **不写单元测试**。门禁是 `npm run typecheck` + `npm run build` + 浏览器手动 44 路由 smoke + 真 LLM E2E。testend 是 dev tool，加单测过度工程。
5. **不引 i18n**。testend 用户是开发者（单人），中英混排即可。不要复用 frontend 的 react-i18next。
6. **commit 粒度 = 一 view 一 commit**（或邻近 2-3 简单 view 一 commit）。push 跟每次 commit。
7. **不在 commit message 加 `Co-Authored-By: Claude`**（per project memory）。
8. **不开分支**（直接 commit 到 main）。
9. **错误展示原码，不走 errorMap**。frontend 走 errorCodes → errorMap → i18n key → t()。testend 直接显示 backend 返回的 `error.code` + `error.message`，debug 视角需要看原始码。

---

## 文档同步触发表（§F1 testend 部分）

| testend 代码变动 | 必改文档 |
|---|---|
| 新 view / 删 view | testend-design.md view inventory + progress-record.md dev log |
| 新 dev 后端 endpoint 消费 | api-design.md + testend-design.md + progress-record.md |
| 改共享 import 模式（alias / type 路径） | testend/CLAUDE.md（本文件）+ frontend/CLAUDE.md（若动到 frontend） |
| 后端 dev/* 端点删除/新增 | api-design.md + testend-design.md + progress-record.md |
| 发现 testend 影响产品核心思想的 bug | progress-record.md + testend-rewrite-backend-issues.md V3 段 |

发现文档与代码不符 → **立刻停下修文档**，记 `[doc-fix]` dev log。

---

## 跑起来

```bash
make testend       # 浏览器自动打开 http://localhost:8742/dev/
make stop          # 关
```

Makefile testend target 内部：`go run ./backend/cmd/server --dev --port 8742 --testend-dir testend/dist`，先 build testend dist 若不存在。

开发期热重载：`cd testend && npm run dev`（单独跑 vite dev 5174；后端 8742；vite proxy 已配 /api 和 /dev）。

---

## Verification 三层（每次 commit 前）

| 层 | 命令 | 通过条件 |
|---|---|---|
| 静态 | `cd testend && npm run typecheck` | 0 error |
| 静态 | `cd testend && npm run build` | 0 error / 0 warning |
| 动态 | `make testend` + 浏览器开当前修改的 view | 无 console error / 数据/UI 正确 |

整体回归（大变动后）：`make test-backend`（所有包） + 跑 frontend `npm test -- --run`（确保没误碰 frontend shared）。
