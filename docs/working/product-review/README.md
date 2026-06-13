---
id: WRK-008
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

# product-review —— 全产品完整性审查（2026-06-12）

## 定位

第三种审查视角。前两轮：backend-review 答「实现扛不扛得住」（正确性/并发/错误路径），docswriter 答「设计讲不讲得通」（按域设计评审）。本轮答「**产品走不走得完、配不配得齐**」——全旅程（all journey）+ 全基础设施配置面。起因：exec-observability 发现 `trigger_workflow` 返回 flowrunId 但 LLM 无工具读回——每行代码都对、设计也通，但 produce/consume 没配对，前两种视角都抓不到。

## 规则

- 分支 `product-review`；小洞顺手修、明确正确解法的也修、产品级裁决留 [DECISIONS-PENDING.md](DECISIONS-PENDING.md)。
- **每条 finding 先亲自 grep/读码验证再定性**（docswriter F-4 方法论）；finding 编号 PR-N。
- 修复随轮提交；`make verify` + `-race` 全绿才算轮收口。

## 波次

| 轮 | 范围 | 方法 | 状态 |
|---|---|---|---|
| R1 ✅ | **配置与基础设施面全检** | 旋钮清单逐个验：设置口在哪（HTTP/tool/文件）→ 读取链是否真生效 → 当前值可见吗 → 默认值合理吗 → 文档一致吗。覆盖：model 场景解析/override、api keys、workspace settings（web_fetch_mode 等全部键）、search settings/embedder、agent 模型配置、handler init-config、mcp config、trigger config、skill allowed-tools、pkg/limits 用户可调上限、加密落盘、locale | ✅ |
| R2 ✅ | **实体闭环配对矩阵** | 实体 × 能力机械扫格子：执行→记录→人可查→LLM 可查→可诊断→过程可见；每个返回的 id 必有消费口；每个异步启动必有状态查询口；CRUD+list+分页完备 | ✅ |
| R3 ✅ | **角色旅程走查** | {人, LLM} × 全任务清单端到端：从零搓 workflow 并调通 / 调试失败 function / 装 MCP 排错 / 配 trigger 看 firing / 配模型与密钥并验证生效 / 管对话与记忆 / workspace 全生命周期 / boot-崩溃-恢复-退出。每步问「此刻需要的信息/动作存在吗」 | ✅ |
| R4 ✅ | **横向一致性** | 六可执行体 + 全实体对照：错误码风格、分页、:action 动词、工具描述质量与互相引导、SSE 事件覆盖、:triage/:iterate 可达性——一处有的能力其它处缺没缺 | ✅ |
| R5 ✅ | **前端对接预检** | 以「前端要画这个面板」反推每个域：列表字段够吗、详情够吗、实时流够吗、空态/错态表达得了吗 | ✅ |
| R6 ✅ | 收尾：留档 + 终报 + 文档同步 | | ✅ |

> 安全红线/并发等二轮已深扫的维度不重做；扫描中撞见照报。

## 文件

- [findings.md](findings.md) —— 全部发现（PR-N：维度/严重度/验证过程/处置）
- [DECISIONS-PENDING.md](DECISIONS-PENDING.md) —— 等用户裁决的产品级问题
- [REPORT.md](REPORT.md) —— 终报
