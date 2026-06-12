---
id: WRK-015
type: working
status: active
owner: @weilin
created: 2026-06-12
reviewed: 2026-06-12
review-due: 2026-09-12
expires: 2026-09-12
landed-into: ""
audience: [human, ai]
---

# DECISIONS-PENDING —— 验收期产品裁决台账

| 编号 | 问题 | 选项/建议 | 状态 |
|---|---|---|---|
| AC-PD-1 | function/handler 创建与 edit **同步阻塞 env 物化**（冷启动 26s、运行时缓存命中仍 3-6s；带依赖时 pip 可达分钟级）——编辑器场景创建应秒回 | A：异步物化（创建即返、envStatus=installing 列已支撑、面板/通知报就绪；run 时未就绪则等待或快速失败）；B：维持同步（语义最简：返回即可跑） | ⬜ 待裁 |
