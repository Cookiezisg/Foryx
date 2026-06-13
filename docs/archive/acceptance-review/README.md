---
id: WRK-012
type: working
status: archived
owner: @weilin
created: 2026-06-12
reviewed: 2026-06-12
review-due: 2026-09-12
expires: 2026-09-12
landed-into: ""
audience: [human, ai]
---

# acceptance-review —— 全产品真机验收 + 体验审查（2026-06-12）

## 定位

第四种审查：前三种（实现正确性 / 设计自洽 / 闭环配对）都是读码推演，本轮**真开机真打**——全部 feature × 全部情况 × 涟漪面，三列判定（用户面 / 产品逻辑 / LLM 面），外加六视角×六状态体验审查。完整计划见 [PLAN.md](PLAN.md)。

> **换 agent 接手？先读 [HANDOFF.md](HANDOFF.md)** —— 操作手册 + 方法论 + harness/llmmock API 速查 + bug 模式图谱 + W5 逐步接手指南。读它即可无缝续跑、标准不变。

## 规则

- 分支 `acceptance-review`；场景即 go test（testend/scenarios，黑盒零 backend import）；发现 PR-N 亲验落 [findings.md](findings.md)；能修顺手修、产品裁决留 [DECISIONS-PENDING.md](DECISIONS-PENDING.md)；每波 verify+testend 双绿收口提交。
- 永久资产：testend/ 验收套件（make testend）+ 金标套件（make evals）+ promptdump。

## 波次

| 波 | 范围 | 状态 |
|---|---|---|
| W0-W8 | 首轮（环境/锻造/编排/集成/对话/平台/柱B/柱C 快扫 + 首收口） | ✅ 首验（24 条 finding） |
| R1-R7 | **高标准重验**（用户裁定重开）：A7 整面 / A4 整域 / A8 缺格 / A9 协议面 / A10 矩阵 / 柱B 四状态两视角 / 柱C 后 5 旅程 | ✅ 逐格全绿（新增 AC-25..30） |
| R8 | 终收口：全量回归 95 场景 + 全套 evals 12 旅程 + verify | ✅ |

**程序完成（2026-06-13）**：首轮 W0-W8 后用户裁定 A7 起标准滑坡、重开 R1-R8 按 [R-PLAN.md](R-PLAN.md) 缺口矩阵逐格补课——三柱最终全绿。30 条 AC 全处置（R 轮新抓 AC-26 🔴 三面同死 / AC-27 🟡 mcp ref 死链 / AC-29 🟡 通知族哑火 / AC-30 🟡 窗口未知压缩盲禁）。终报：[TERMINAL-REPORT.md](TERMINAL-REPORT.md)。永久资产：`make testend`（95 黑盒场景）+ `make evals`（12 金标旅程）+ promptdump 体验审计。
