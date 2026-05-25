# Forgify 产品完成度 / 契约一致性审计 — Agent 任务指令

> 用法:对一个**全新的 Claude Code**(独立 session)说:「读 `documents/version-1.2/completeness-audit.md` 并执行这次审计」。它会自包含地跑完。

---

## 你是谁、要干什么

你是 Forgify 项目的资深审计员。这个项目长期单人 + AI 协作开发,**代码、文档、prompt 三者大量对不上**。你要抓的**不是**普通 bug、也不是代码风格,而是**「契约断裂」** —— 某处**声称**有某个能力(prompt 教 agent 用它 / 文档说支持它 / UI 有入口 / 接口声明了它),但实际**不存在或链路不通**。

**判断的唯一锚**:假设一个用户或 agent **相信了这个声称、真的去用**,会发生什么?
- 失败 / 报错 / 无响应 / 调到一个不存在的东西 → **🔴 这就是你要抓的**。
- 能正常 work,只是文档措辞不准 → 🟡 次要。
- 纯内部注释过时、无任何外部可见影响 → ⚪ 记录。

## 5 类契约断裂(你要找的形态)

1. **幽灵引用** — prompt/文档/代码引用了一个**不存在**的工具/函数/endpoint/实体/字段。
   - 真实案例:`internal/app/chat/multi_agent_prompt.go` 教 agent「trigger_workflow to dry-run」,但 `trigger_workflow` 工具全局无定义(`WorkflowTools()` 只注册 6 把)→ agent 照做必失败。
2. **悬空契约** — 声明了接口/能力却没接上:实现了没注册、domain 有但 app 层缺、注册表漏一项。
   - 真实案例:`documentapp` 实现了 `AsCatalogSource()` 但 `main.go` 漏 `RegisterSource` → document 永不进 catalog;`workflowapp` 连 `AsCatalogSource` 都没写;`flowrun` 有 domain+store 却没 app 执行层。
3. **文档-代码漂移** — 契约文档/设计文档说 X,代码做 Y(字段名、endpoint、error code、事件 type、行为不符)。
4. **死链路** — 入口在、中途断:endpoint 注册了但 service 是 stub / 返回假数据;前端有按钮但后端没接。
5. **沉默半成品** — 某 feature 做了骨架、核心缺,且没在显眼处标注「未完成」→ 用户以为能用。

## 严重度(只报 🔴 和 🟡,⚪ 不报)
- 🔴 **高**:用户/agent 会直接撞墙 —— 调用必失败、点了没反应、承诺的能力根本用不了。
- 🟡 **中**:文档/契约误导但功能仍 work;或数据不准但不致命。

## 怎么扫(系统性,不要随机找)

1. **交叉引用核实**:把所有 prompt(`internal/app/chat/*prompt*.go`、各 `*_context.go`、`internal/app/**/registry.go`)和文档(`documents/version-1.2/**`)里提到的**每个工具名 / endpoint / 实体 / 字段**,逐个 grep 代码,确认它真实存在且能被调到。
2. **接口实现对账**:每个端口接口(`Tool`、`CatalogSource`、各 `Repository`、mention resolver、catalog source)→ 实现了吗?在 `cmd/server/main.go` / router 注册了吗?
3. **契约文档对账**:`documents/version-1.2/service-contract-documents/`(api / error-codes / events)逐条 → grep 代码确认存在且一致。
4. **入口到落地追踪**:每个 HTTP route(`internal/transport/httpapi/`)、每个 system tool → 追到最终实现,看是不是 stub / 假数据 / 断链。
5. **Phase 完成度核对**:`backend-design.md` / `progress-record.md` 里标「完成」的项 → 抽查代码核对;特别注意 Phase 4(工作流)/5(智能化)标「未启动」,但已有半成品(如 flowrun)却被别处引用的情况。

## 项目地图
- 架构:4 层 clean arch,`transport → app → (domain ∪ infra/store) → infra/db`;后端代码在 `backend/`。
- 文档:`documents/version-1.2/` — `backend-design.md`(愿景/Phase 路线)、`progress-record.md`(进展)、`service-design-documents/<domain>.md`(详设)、`service-contract-documents/`(API/error/events 契约)。
- 纪律:`CLAUDE.md` — 尤其 §S14(文档同步)、§S15(ID 前缀表)、§S18(Tool 接口规约)。
- 已知半成品线索:`trigger_workflow` + flowrun 执行引擎属 Phase 4 未实现(已知,不用重复报);catalog 实际只接了 function/handler/skill/mcp(workflow/document 漏接)。

## 输出格式(一份报告,按严重度排序,🔴 在前)

对每个问题:
```
### [🔴/🟡] <一句话标题>
- 类型:<5 类之一>
- 位置:<file:line>
- 声称:<谁说有这能力 + 引用出处,贴关键片段>
- 实际:<grep/读码结论 + 关键片段>
- 撞墙场景:<一句话:谁、怎么用、会怎样失败>
- 修复方向:<补实现 / 删引用 / 改文档> + 工作量(小修 / Phase 级大工程)
```

## 纪律(必须遵守)
- **只审计 + 报告,绝不改任何代码或文档。**
- **先核实再下结论**:看到一处引用别急着说「没实现」—— 一定 grep **全局**(工具/函数可能在别的包、别的名字、别的注册函数里)。本审计的起因 trigger_workflow,第一次只 grep 了单个目录差点误判,全局搜才坐实。
- 宁可漏报 ⚪ 低优先级,绝不误报 🔴。每个 🔴 都要有铁证(声称片段 + 实际片段并列)。
- 跑完给一个**严重度汇总**:🔴 N 个、🟡 M 个,以及你最担心的 top 3。
