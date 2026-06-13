---
id: WRK-018
type: working
status: active
owner: @weilin
created: 2026-06-13
reviewed: 2026-06-13
review-due: 2026-09-13
expires: 2026-09-13
landed-into: ""
audience: [human, ai]
---

# R-PLAN —— A7 起高标准重验计划（用户裁定重开，2026-06-13）

> **用户裁定**：W0-W2（A1-A5）标准合格；**从 A7（W3 后半）起标准滑坡**——只测了 happy path + 少量错误码，没有以产品思维穷尽 PLAN.md 每格"必验情况"。重开为 R 波次，按 W1/W2 的标准补全 A7、A8（含 A4）、A9、A10、柱B、柱C。**分阶段，每阶段独立收口提交，不一口气低质量。**

## 高标准的定义（W1/W2 已立的标尺）

每个 feature = 功能本体 × **情况矩阵全格**（正常/边界/出错/并发/降级）× **涟漪面**；三列判定（用户面 HTTP 语义 / 产品逻辑状态机级联记账 / **LLM 面工具真驱动**）；正反都断言（选边要验"未选边不跑"、守卫要验"不误伤"）；以 PLAN.md 对应表格**逐格勾**，缺格 = 未完成。

## 缺口矩阵（2026-06-13 对照 PLAN.md 逐格核对）

### A7 Search（search_test.go 现状 → 缺口）

| PLAN 必验格 | 现状 |
|---|---|
| 词法：中文 2 字 LIKE / 注入 / 空查询 | ✅ |
| 词法：3+ 字 trigram 中文、中英混合、代码符号（`snake_case`/`CamelCase`） | ❌ |
| 12 实体投影：agent / control / approval / mcp（server+工具粒度）/ handler 方法粒度 | ❌（只投了 8 类） |
| 投影全周期：**改→新内容搜到→删→搜不到**；conversation 增量（新消息进索引） | ❌（只测建→搜到） |
| 同步：杀进程丢事件→**boot 对账自愈** | ❌ |
| 质量：前缀次于 exact、**折叠+matchedChunks**、workspace 隔离 | ❌ |
| **LLM 口整面**：search_blocks 三段精度链 / 8 垂搜+降级链 / search_conversations / Retrieve(MaxChars) | ❌（零覆盖） |
| ollama：真连切换错误态可见（无本机 ollama，验到"设置生效+软降级"为界） | 部分 ✅ |
| 三态互切→向量失效→**后台重嵌真完成** | ❌（只切了设置） |

### A8 Chat + A4 Agent（chat_test.go 现状 → 缺口）

| PLAN 必验格 | 现状 |
|---|---|
| 主链/工具往返/人在环/取消/409/todo/标题/压缩/错误路径 | ✅ |
| **并行工具批**（同回合多 tool_call 并发执行 execution_group 语义） | ❌ |
| progress 块、**subagent 嵌套树（E3 parentBlockId）** | ❌ |
| **附件三路**：vision 图片 / native PDF / sandbox 文本提取（上传→消息引用→模型视角真出现） | ❌ |
| **skill 两路 activate + allowed-tools 免确认**（active skill 危险工具不询问） | ❌ |
| memory llmmock 面：write_memory 工具真写 / 新对话 system prompt 注入 / 忘 | ❌ |
| @mention 实体冻结、归档 Send 自动解档、删除对话取消在途生成 | ❌ |
| 消息重连重水合（断开 SSE 重连 → close 快照 replay 补齐） | ❌ |
| utility 缺席静默降级（llmmock 面：不配 utility，标题/压缩缺席主链不伤） | ❌ |
| **A4 全部**：invoke_agent 嵌套块流 / 挂载三类（fn、hd.method、mcp）真合成专属工具真调通 / HTTP :invoke + workflow 节点入口 / transcript 落库 + executions 查询 / 挂载物被删悬空涟漪 / 输出 schema 约束 / modelOverride 优先级 | ❌（无 agent_test.go） |

### A9 平台（platform_test.go 现状 → 缺口）

| PLAN 必验格 | 现状 |
|---|---|
| workspace CRUD/校验/activate/最后拒删/级联（function+conversation 不可达） | ✅ |
| 级联**逐资产**验残留（12 类资产建齐再删 ws：行/盘/索引/关系/通知全清） | ❌ |
| apikey 全面 + model 三场景 | ✅ |
| limits **每字段** PATCH→行为真变（现只 3 字段：triggerRatio 校验/maxSteps/toolResultCapKB） | ❌ |
| 通知**全事件类型**到达（现只 function.created） | ❌ |
| sandbox：runtime 装/删/gc | ❌ |
| **SSE 三流协议面整面**：durable 重连 Last-Event-ID replay 不丢 / E2 ephemeral seq=0 不进 buffer（重连不重放 delta）/ E3 嵌套 parentBlockId / entities 流 upsert/delete 帧 / 410 Gone 淘汰 | ❌（零覆盖） |

### A10 涟漪矩阵

PLAN 要求 {创建/改名/删除} × 12 实体 → {搜索索引/关系图/catalog/通知/挂载方/引用方} 六面机械表。现状：1 实体 × 1.5 面（platform_test 的 relation ripple）。**缺口 ≈ 96%。** 需逐实体机械扫（不可达格记 N/A 并说明）。

### 柱B 体验（promptdump_test.go 现状 → 缺口）

| 计划格 | 现状 |
|---|---|
| 视角：Chat 主 / Utility / Agent 实体 / 用户(preview) | ✅ |
| 视角：**Subagent**（invoke_agent 时 agent 收到什么）/ **前端开发者**（三流 SSE 帧线缆形状审读） | ❌ |
| 状态：空态 / 正常 | ✅ |
| 状态：**规模态**（200 实体 catalog 形状/长度预算）/ **降级态**（无模型/utility 缺席的 prompt 面）/ **崩溃恢复态** / **长程压缩后态**（压缩后 system+messages 形状） | ❌ |
| 横切：prompt lint / preview 保真 / S18 / i18n | ✅ |
| 横切：**tool_result 形状**（截断标记/错误形状一致性）/ **token 成本账单**（usage 逐请求记账与 mock 对账） | ❌ |

### 柱C 金标（golden_test.go 现状 → 缺口）

已有 7：J1 自举 / J2 function / J3 handler / J5 debug / J7 search / J9 memory / J12 降级。
**缺 5**：**J4 搓三节点 workflow 真触发到 parked** / **J6 装 MCP 调工具** / **J8 回忆历史对话（search_conversations）** / **J10 激活 skill 干活** / **J11 跨压缩边界长任务**。

## R 波次（每波独立收口：测试绿 + 发现修复 + findings 记录 + 提交推送）

| 波 | 范围 | 状态 |
|---|---|---|
| R1 | A7 Search 补全（LLM 口整面 + 投影全周期 12 实体 + boot 对账 + 质量格） | ✅ 17/17（抓 AC-26 🔴 三面同死 + AC-27 🟡 mcp ref 死链 + AC-25/28；见 findings） |
| R2 | A4 Agent 整域新建 agent_test.go（三入口/三类挂载/transcript/悬空/schema） | ✅ 6/6（无新 bug——A4 全如设计；E3 嵌套/挂载合成/fail-fast 实证；见 findings） |
| R3 | A8 Chat 补全（附件三路/skill/memory/mention/归档/删除取消/并行批/subagent/重水合/utility 降级） | ✅ 10/10（无新 bug——十面全如设计；harness 增 Upload/SubscribeFrom；见 findings） |
| R4 | A9 平台补全（SSE 三流协议面/limits 每字段/通知全事件/sandbox 装删 gc/级联逐资产） | ✅ 5/5（抓 AC-29 🟡 mcp 通知族哑火并修复；410 环淘汰/limits 九字段/删除守卫实证；见 findings） |
| R5 | A10 涟漪矩阵机械表（12 实体 × 3 操作 × 6 面） | ✅ 3/3（矩阵台账逐格归口进 findings；mention 不产边/名随代码体两项 by-design 定界） |
| R6 | 柱B 补全（subagent+前端开发者视角；规模/降级/恢复/压缩后四状态；tool_result 形状+token 账单） | ✅ 6/6（无新 bug；帧 kind 判别/规模 <3×/恢复恰一次/配对零孤儿实证；见 findings） |
| R7 | 柱C 补全（J4 workflow-parked / J6 MCP / J8 历史对话 / J10 skill / J11 跨压缩） | ✅ 5/5（计划 12 旅程齐；抓 AC-30 🟡 openai 兼容路线窗口未知→压缩盲禁体验陷阱；见 findings） |
| R8 | 终收口：全量回归 + verify + 终报整体重述 | ✅ 95 场景回归 974s 零失败 + 12 旅程 evals 全绿 + verify/docs 绿；终报整体重述 |

## 执行纪律

- 每格先读对应 reference 文档（api.md / domains/<域>.md / events.md）对齐契约，再写测试——测试断言以契约为准，契约与行为不符即 finding（AC-N 续号，亲验定性）。
- 发现就修（用户 standing：遇到问题直接修复）；修复必须黑盒可验证 + 文档同步同提交。
- 不可达格（如需真 ollama/真外网 registry）记 N/A + 原因进 findings，不静默跳过。
- 每波收口跑该波文件 + 相邻波文件（防交叉破坏）；R8 跑全量。
