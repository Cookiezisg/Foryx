# backendcleaner — STATE（单一状态源）

> 进度的**唯一**事实源。原 `CONCLUSIONS.md` / `backlog.json` 已删（避免三处状态漂移）：结论并入 `SPEC.md`/`criteria.md`，下一步在本文末，轮次索引在 `ROUNDS.md`。

## 当前

- **阶段**：Phase 0 计划 — lab 已定稿，**等确认进 Phase 1**。
- **分支**：`main`（backend-new 平行重写不需要分支）。
- **策略**：在 `backend-new/` 平行重建 → 覆盖回 `backend/` → 调前端/testend 兼容。

## 已定的关键决策

- 全量重写，**无任何保留**（含本地 SQLite 数据 → schema 可激进重定）。
- 契约**可改**：每次改对外 API/SSE/error 都 take note 到 `contract-changes.md`；前端/testend 也是 AI 写的，覆盖后一并兼容。
- 架构按 `module-template.md` 统一，但**按需取层**（不强行套层）。
- 重写单元 = 垂直切片（domain+store+app+tool+handler）；顺序见 `order.md`，判据见 `criteria.md`。

## 模块进度（编号见 order.md）

状态：⬜ pending ｜ 🔧 doing ｜ ✅ done ｜ ⏭️ 判定删除/合并

- **波次0 地基**：M0.1 pkg ⬜ · M0.2 db ⬜ · M0.3 logger/crypto ⬜ · M0.4 errors/eventlog/notif(domain) ⬜ · M0.5 infra eventlog/notif/chat ⬜ · M0.6 llm ⬜
- **波次1 叶子域**：M1.1 user ⬜ · M1.2 apikey ⬜ · M1.3 model ⬜ · M1.4 relation ⬜ · M1.5 catalog ⬜ · M1.6 mention ⬜ · M1.7 memory ⬜ · M1.8 sandbox ⬜ · M1.9 permissions/hooks ⬜ · M1.10 document ⬜ · M1.11 todo ⬜(待判定)
- **波次2 tool+原语**：M2.1 tool ⬜ · M2.2 loop ⬜ · M2.3 tool/filesystem·search·web·toolset ⬜
- **波次3 Quadrinity 执行体**：M3.1 function ⬜ · M3.2 handler ⬜ · M3.3 subagent ⬜ · M3.4 agent ⬜ · M3.5 skill ⬜ · M3.6 mcp ⬜ · M3.7 tool 适配器组 ⬜
- **波次4 编排核心**：M4.1 workflow ⬜ · M4.2 flowrun ⬜ · M4.3 scheduler 🔴⬜ · M4.4 trigger ⬜ · M4.5 tool/workflow ⬜
- **波次5 对话**：M5.1 conversation ⬜ · M5.2 chat ⬜ · M5.3 contextmgr ⬜ · M5.4 tool/permissionsgate ⬜
- **波次6 顶层编排**：M6.1 askai ⬜ · M6.2 ask+tool/ask ⬜(强残留嫌疑)
- **波次7 transport+wiring**：M7.1 response/middleware/router ⬜ · M7.2 handlers×36 ⬜ · M7.3 cmd/server ⬜ · M7.4 cmd/desktop+工具 ⬜

## 下一步

- 等确认 → **Phase 1**：建 `backend-new/go.mod`（最终 module path）+ 波次0 地基 + 最小 smoke（启动 / `/api/v1/health` / 用户初始化）。
