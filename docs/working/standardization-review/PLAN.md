---
id: WRK-019
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

# 标准化审核 —— 计划（standardization-review，2026-06-13）

## 定位：第五种审查

前四种（实现正确性 / 设计自洽 / 闭环配对 / 真机验收）审的是**对不对、跑不跑得通**。本轮审的是**标准性/一致性**——对"同一类东西"，对前端的线缆形状与后端内部处理是否都用同一种做法。**零历史包袱**：找出"该统一却没统一"的地方，把主流/正确做法定为 canonical，少数偏离的改齐；有物理/语义理由的分化只落档不改。

**侦察基础**：9 维度并行扫全库（5 对外线缆面 + 4 内部面），47 条候选 divergence，去重合并为 **11 条种子发现（SD-1..11）**，亲验中**推翻 2 条 stale 结论**（modelclient 已统一、mcp serverRow 分离是加密设计）。

## 现状全貌

后端"线缆形状"层标准化**总体高**：N3 camelCase/snake_case 100% 净、SSE Envelope 三流统一、DIP `Set*` 注入矩阵齐、Detached Context（S9）用法一致。扎眼的"同类不同做"集中在 6 处：

1. **列表形状分裂**（SD-1）：版本列表走 `response.Paged` 出 `{data:[...],nextCursor,hasMore}`，执行/调用/搜索列表用 `response.Success` 包成 `{data:{executions:[...],...}}`——同一实体的兄弟接口前端要写两套解包。
2. **Create 响应三态**（SD-3）：版本实体 `{type,version}`、非版本扁平、trigger 卡中间 `{trigger}` 无 version。
3. **18 处 handler 硬编码 HTTP 状态+码**（SD-4），绕过 `FromDomainError→statusForKind` 唯一映射；其中 **8 个码无 sentinel 定义、未登记 error-codes.md**（SD-5），逃过全库唯一性守卫。
4. **notification 路由违 N5**（SD-6）：`PUT /{id}/read` + `POST /read-all`，全 API 唯一不用 `:action` 的动作。
5. **mcp 执行台账命名孤立**（SD-7）：`CallTriggeredBy*` / `GetCall` / `ComputeCallAggregates` 与 function/handler 多数派不齐。
6. **app 服务骨架不齐**（SD-8）：conversation/document 内联 CRUD（无 crud.go）、`New()` vs `NewService()`、`emit` vs `publish`。

## 方法论

**canonical 判定（按优先级）**：(a) **规则字面**——N1/N3/N4/N5/S20 等宪法条款直接定形状（如 N5 字面要求非 CRUD 用 `:action`，故偏离方是 notification 的 `/read`）；(b) **多数派**——同类里多数怎么做即标准（`GetXByID` 3/4、`NewService` 10/13、`publish` 11/12）；(c) **最简正确**——同形状取前端解包路径最少者（列表裸数组进 data、分页元数据进 envelope 顶层，胜过嵌一层 key）。

**normalize vs document**：有物理/语义理由的分化**只落档不改**（flowrun 编排态 vs execution 调用态是两套语义、agent 不能 trigger agent、mcp serverRow 分离为 config_enc 加密、document DELETE 返 deletedCount 因级联）；无理由的随意不同（命名、helper 选择、HTTP 状态码散布）**一律改齐**。

**纪律**：每条改前 grep 验 call-site 全集、改后 `make verify`（gofmt+vet+build+单测+docs 门禁）绿、涉线缆改动补 testend 黑盒；每波一个独立 commit `feat(standardization-rN): ...`，代码改动**同提交**带 1:1 文档同步（api.md/error-codes.md/events.md/domains/*.md）；commit 不加 Co-Authored-By、每 commit 后即推。先做对前端影响最大、风险最低的（响应/列表形状），再推内部骨架。

## 波次（每波独立收口提交）

| 波 | 范围 | 状态 |
|---|---|---|
| **S1** | 对外列表与响应形状统一（List 全出 `{data:[...],...}` + Create 分类统一）——前端线缆，最高优先、风险最低 | ⬜ 待 Q1/Q2/Q3 裁决 |
| **S2** | 错误构造统一回 sentinel + FromDomainError，补全 error-codes.md（18 处硬编码 + 8 漏登码） | ⬜ |
| **S3** | 路由动词与 SSE 事件登记对齐（notification/memory 改 `:action`；conversation 事件补登 events.md） | ⬜ 待 Q4/Q5 裁决 |
| **S4** | 内部执行台账与服务骨架命名对齐（mcp 台账名、`NewService`、`publish`、crud.go 抽取） | ⬜ 待 Q6 裁决 |
| **S5** | 有意分化落档 + 共享链路注释/登记补全（ID 前缀 S15、双语义注释、身份方案分组、HANDLER_CRASHED 转译） | ⬜ 待 Q7 裁决 |

各波 tasks/exitCriteria 详见 workflow 综合产出（本文 §种子发现 + §待裁决即其落档）。

## 种子发现台账（SD-1..11，全部 file:line 亲验）

| ID | 级 | 标记 | 标题 |
|---|---|---|---|
| SD-1 | high | | 执行/调用/搜索列表 `response.Success` 嵌包,与版本列表 `Paged` 裸数组分裂(`function.go:296`/`agent.go:292`/`handler.go:332`/`mcp.go:114`) |
| SD-2 | medium | | `search.Search` 绕过 `Paged` 且手搓 `?limit` 解析(`search.go:94-125`) |
| SD-3 | high | 需裁决 | Create 响应三态:版本 `{type,version}`/非版本扁平/trigger `{trigger}` 无 version |
| SD-4 | high | | 18 处 handler 硬编码 HTTP 状态码+码,绕过 `FromDomainError→statusForKind` |
| SD-5 | high | | 8 个 wire 码无 sentinel 定义、未登记 error-codes.md,逃过唯一性守卫 |
| SD-6 | high | 需裁决 | notification `PUT /{id}/read`+`POST /read-all` 违 N5(全 API 唯一例外) |
| SD-7 | medium | | mcp 台账命名孤立:`CallTriggeredBy*`/`IsValidCallTrigger`/`GetCall`/`ComputeCallAggregates` |
| SD-8 | medium | 需裁决 | 服务骨架不齐:conversation/document 内联 CRUD、`New()` vs `NewService()`、`emit` vs `publish` |
| SD-9 | low | 需裁决 | `conversation.compacted` 已 emit 未登记 events.md(`conversation.go:223`) |
| SD-10 | medium | byDesign·需裁决 | infra `HANDLER_CLIENT_CRASHED` vs domain `HANDLER_CRASHED` 同义不同码(命名空间分界) |
| SD-11 | medium | 需裁决 | 6 个运行时 ID 前缀(`aki_/bsh_/sig_/sr_/se_/subagt_`)未登记 database.md S15 |

**推翻的 stale 结论（不立项）**：① LLM 解析链已于 acceptance-r1/AC-26 收敛为 `app/modelclient.Resolve`、bootstrap 已 delegate；② mcp `serverRow`/`DeletedAt` 分离是 `config_enc` 加密的有意设计，非 contract 不符。

## 待裁决项（哪个做法当 canonical——需用户拍板，含推荐）

1. **Create 响应统一成哪种?**（SD-3）→ 推荐 **A**：版本实体全 `{type,version}`、非版本全扁平、trigger 归扁平组（三态变两态，不动 N1）。备选 B 全扁平 / C 全 `{data,version?}`。
2. **执行列表改 Paged 后 aggregates 放哪?**（SD-1）→ 推荐 **A**：扩展 `Paged` 接可选 `aggregates` 进 envelope 顶层（与 nextCursor/hasMore 并列）。备选 B 独立 GET / C 放弃统一。
3. **List 是否统一裸数组进 data?**（SD-1/2）→ 推荐 **B**：可分页接口统一 `Paged`；有意不分页的小集合（skill/memory/model caps）保留 `Success` 但 data 仍裸数组；search 不额外暴露 total、统一靠 hasMore。
4. **notification/memory 路由改 `:action`?**（SD-6）→ 推荐 **A**：两者都改（`POST /{id}:read`、`/memories/{name}:pin`），全 API 零例外。
5. **conversation.compacted/auto_titled 是对外事件?**（SD-9）→ 推荐 **A**：正式登记 `conversation.{created,updated,deleted,auto_titled,compacted}` 进 events.md。
6. **服务骨架统一到什么程度?**（SD-8）→ 推荐 **B**：统一构造器名 `NewService` + `publish` 命名（零风险纯重命名）；crud.go 物理抽取对非版本简单实体豁免并落档。
7. **HANDLER_CRASHED 统一还是分离?**（SD-10）→ 推荐 **A**：保持 infra 独立命名空间（合 MEMORY『Shared infra IDs』），补规则+注释：domain 捕获 infra 码后必转译,wire 上只出 domain 码。
