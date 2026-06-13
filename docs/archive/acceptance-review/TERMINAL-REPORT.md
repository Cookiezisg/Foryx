---
id: WRK-017
type: working
status: archived
owner: @weilin
created: 2026-06-13
reviewed: 2026-06-13
review-due: 2026-09-13
expires: 2026-09-13
landed-into: ""
audience: [human, ai]
---

# 终报 —— 全产品真机验收 + 体验审查（2026-06-12～13，含 R 波次高标准重验）

> **程序完成**：首轮 W0-W8 后经用户裁定「A7 起标准滑坡」**重开为 R1-R8 高标准重验**，按 PLAN.md 逐格穷尽补课后三柱全绿。细节：[findings.md](findings.md)（30 条 AC 台账 + R 波次逐节）；缺口矩阵与波次记录：[R-PLAN.md](R-PLAN.md)；裁决：[DECISIONS-PENDING.md](DECISIONS-PENDING.md)；接手：[HANDOFF.md](HANDOFF.md)。

**一句话**：两轮（首轮快扫 + R 轮逐格高标准）真开机真打共抓 **9 条 🔴 + 一串 🟡**（核心是 **12 例「设计完整、接线缺失/哑火」**——最重的 AC-26 三个功能生产从未工作、AC-27 mcp 可接线 ref 物理死链、AC-29 整族通知哑火都在 R 轮被逐格机械扫抓出），全部修复；真模型（deepseek-v4-flash）**计划 12 旅程全数跑齐全绿**；沉淀 95 黑盒场景 + 12 金标旅程（约 7900 行）两套可重跑永久资产。

## 1. 定位：第四种审查 + 第二轮标准重校

前三种审查（实现正确性 / 设计自洽 / 闭环配对）都是读码推演。本程序**真开机、真打请求、真跑模型**：独立 Go module `testend/`（零 backend import）编译并拉起真 `cmd/server`，讲纯 HTTP/SSE。首轮 W0-W8 跑完后用户裁定 A7 起覆盖不足，重开 R1-R8：**以 PLAN.md 的"必验情况"逐格勾稽为完成标准**（缺格 = 未完成），正反都断言、三列判定（用户面/产品逻辑/LLM 面）、不可达格记 N/A 并写明亲验原因。

**核心命题两轮都被证实**：黑盒压力抓到读码审查结构性抓不到的 bug——且 R 轮证明**首轮的"全绿"并不蕴含覆盖完整**：A7-A10/柱B/柱C 的缺格里藏着三条全新 🔴/🟡（AC-26/27/29），全部属于"单测 fake 绿、文档有承诺、唯独真线缆死"的家族。

## 2. 方法与永久资产

| 资产 | 作用 | 命令 |
|---|---|---|
| `testend/scenarios`（95 场景） | 真二进制全功能黑盒，函数名即验收台账行 | `make testend`（llmmock 零 token） |
| `testend/golden`（12 旅程） | 真模型端到端（deepseek-v4-flash，provider=deepseek） | `make evals`（EVALS=1 门控，自动 source `.env`） |
| `harness/`（llmmock+PromptDump+SSE+multipart+续传订阅） | OpenAI 兼容假模型 + 模型线缆视角抓包 + 三流帧采集 | （随 testend） |
| 文档体系（R-PLAN 缺口矩阵 / findings 台账 / HANDOFF） | 换 agent 无缝接手、标准不漂移 | — |

## 3. 发现总账（AC-1..AC-30）

- **🔴 功能不可用/语义错（9）**：首轮 AC-4/9/10/11/16/17/18/21 + **R1 AC-26**（`Factory.Build` 第二返回值 baseURL 被三处手抄链误作 modelID 上线缆——search 精度链/envfix 自愈/WebFetch 摘要**生产从未工作**；收敛为 `app/modelclient` 唯一解析链）。全部修复。
- **🟡 体验/一致性（11）**：首轮 AC-2/5/6/7/12/19/20/22/24 + **R1 AC-27**（mcp 投影按行 id 键控 → refHint `mcp:msv_…` 挂载永远解析不了——"ref 直填"契约对 mcp 物理死链；投影改 server name）+ **R4 AC-29**（events.md 承诺的 `mcp.{installed,updated,removed,reconnected}` 通知族从未接 Emitter——11 域机械扫独缺 mcp 当场现形；补线）+ **R7 AC-30**（openai 兼容路线接 deepseek → 窗口表未命中 → 压缩按设计盲禁——体验陷阱，前端提示素材，与 AC-20 同族）。
- **🟠（2）**：AC-13/14（首轮）。**🟢 / by-design**：AC-1/3/8/15/23/25/28 等（含 Retrieve 休眠口、垂搜 7+1 渲染差异等定界记录）。
- 全程 N/A 格皆有亲验原因落档（findings 各节）：黑盒不可达（需第二真嵌入引擎/直改 DB 文件）或物理不存在（mention 不产边、trigger 无生命周期通知、name 即 id 无改名）。

## 4. 贯穿 bug 模式图谱（最可迁移）

1. **设计完整、接线缺失/哑火（12 例，两轮最高产）**：端口/实现/单测/文档全在，唯独一条线没接——AC-9/10/13/21（首轮）+ **AC-26（地基 API 设陷 + 三处复制传播）** + **AC-27（投影键与挂载键不一致）** + **AC-29（Emitter 全场标配独漏一域）** + 产品审查期 5 例。抓法品类：①断言"协作方必须被咨询过"（sifter 零 dump 现形 AC-26）；②跨域机械扫齐全性（11 域通知独缺 mcp 现形 AC-29）；③契约字面直填回路（refHint 直接喂挂载现形 AC-27）。
2. **契约名义存在、物理失效**（AC-16）；**provider 线缆触雷**（AC-11/17）；**不变量半覆盖**（AC-18）；**生命周期绑错 ctx**（AC-4）；**锁序锁死可见性**（AC-14）；**护盾缺失**（AC-5/6）；**校验漏洞**（AC-7/19）；**热换未贯通**（AC-22）；**显式设置不驱动**（AC-24）；**能力目录未命中的静默降级**（AC-20/30 家族）。

## 5. R 波次结论（高标准重验，每波独立收口提交）

| 波 | 范围 | 结论 |
|---|---|---|
| R1 | A7 Search 17/17 | 12 实体投影全周期 / 粒度锚点 / LLM 口整面（8 垂搜 + blocks 三段链逐档 + search_conversations）/ boot 对账 / 隔离 / 密文红线；**抓 AC-26 🔴 + AC-27 🟡** |
| R2 | A4 Agent 6/6（首轮零覆盖） | 三类挂载真合成（恰为挂载、系统工具零泄漏）+ 四处台账 / 改名重解析 / fail-fast 三态 / prompt 组装 / modelOverride 队列物证 / 三入口（含 E3 嵌套实证）/ 版本生效 |
| R3 | A8 Chat 10/10 | 附件三路按能力门控（PDF sandbox 真抽取）/ skill 两路 + 预授权免确认 / memory 两段式 / mention 冻结 / 删除取消在途 / 并行批 / Subagent 树 + 深度守卫 / SSE 重连 replay（delta 不重放）/ utility 降级 |
| R4 | A9 平台 5/5 | SSE 协议面（410 环淘汰实证）/ limits 九字段热换 / 通知 11 域机械扫（**抓 AC-29 🟡**）/ sandbox 治理（删除守卫）/ 级联逐资产零残留 |
| R5 | A10 涟漪 3/3 | 12×3×6 矩阵逐格台账（建/改/删 × 搜索/catalog/通知/关系/挂载方/引用方）；同名重建不救（ref 按 id）实证 |
| R6 | 柱B 6/6 | Subagent/前端开发者两视角 + 规模（200 实体 <3×）/降级/崩溃恢复（历史恰一次）/压缩后四状态 + tool 配对零孤儿 + usage 逐数对账 |
| R7 | 柱C 5/5 | 计划 12 旅程收齐：J4 搓图到 parked / J6 mcp 真调 / J8 跨对话回忆 / J10 skill 遵循 / J12b 跨压缩召回；**抓 AC-30 🟡** |
| R8 | 终收口 | 全量回归（95 场景）+ 全套 evals（12 旅程）+ verify 全绿；本报整体重述 |

## 6. 裁决台账

- **AC-PD-1**：function/handler 同步阻塞 env 物化 = by-design（可见性实测成立）。✅
- **AC-PD-2**：locale 权威 = workspace.language（用户裁决，已实现）。✅

## 7. 后续（非本程序范围）

前端重建对接已验证契约；AC-20/AC-30 家族的设置页提示（模型窗口未知→压缩不可用、key 未探测→能力保守）；`make testend` 入回归门禁的取舍（9 分钟级）。
